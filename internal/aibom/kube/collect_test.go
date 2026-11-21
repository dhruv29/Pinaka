package kube

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestParseRejectsOversizedMalformedAndNonObjectInput(t *testing.T) {
	if _, err := Parse(strings.NewReader(vllmFixture), Options{MaxBytes: 100}); err == nil || !strings.HasPrefix(err.Error(), "aibom/kube: ") {
		t.Fatalf("byte cap: %v", err)
	}
	for _, input := range []string{`{"kind":"Pod"`, `[1,2]`, `"text"`, `{"kind":"Pod"} garbage`} {
		if _, err := Parse(strings.NewReader(input), Options{}); err == nil || !strings.HasPrefix(err.Error(), "aibom/kube: ") {
			t.Errorf("%q: %v", input, err)
		}
	}
	if _, err := Parse(nil, Options{}); err == nil {
		t.Fatal("nil reader accepted")
	}
	if _, err := Parse(errReader{}, Options{}); err == nil {
		t.Fatal("read error swallowed")
	}
}

func TestParseSkipsOddObjectsWithANoteAndKeepsTheRest(t *testing.T) {
	input := `{"kind":"List","items":[{"kind":"Pod","metadata":"oops"},7,
	 {"kind":"Deployment","apiVersion":"apps/v1","metadata":{"name":"d","namespace":"n"},"spec":{"template":"wrong"}},
	 {"kind":"Service","apiVersion":"v1","metadata":{"name":"s","namespace":"n"},"spec":{"ports":"wrong"}},
	 {"kind":"ConfigMap","apiVersion":"v1","metadata":{"name":"c","namespace":"n"},"data":{"k":1}},
	 {"kind":"List","items":[{"kind":"List","items":[{"kind":"List","items":[]}]}]},
	 {"kind":"Pod","apiVersion":"v1","metadata":{"name":"ok","namespace":"n"},"spec":{"containers":[{"name":"c","image":"ollama/ollama:0.3"}]}}]}`
	bom := mustParse(t, input)

	mustFind(t, bom, aibom.KindServing, "ollama")
	joined := strings.Join(bom.Notes, "\n")
	for _, want := range []string{"skipped list item 0", "skipped list item 1", "n/Deployment/d", "n/Service/s", "n/ConfigMap/c", "nested too deeply"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes lack %q: %v", want, bom.Notes)
		}
	}
}

func TestParseStopsAtTheObjectCapWithANote(t *testing.T) {
	bom, err := Parse(strings.NewReader(vllmFixture), Options{MaxObjects: 1, Subject: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if bom.Subject != "test" || len(bom.Notes) != 1 || !strings.Contains(bom.Notes[0], "stopped after 1 objects") {
		t.Fatalf("bom = %+v", bom)
	}
}

func TestParseNotesEmptyInputAndIgnoredSecrets(t *testing.T) {
	if bom := mustParse(t, ""); len(bom.Notes) != 1 || !strings.Contains(bom.Notes[0], "no Kubernetes objects") {
		t.Fatalf("notes = %v", bom.Notes)
	}
	bom := mustParse(t, `{"kind":"Secret","metadata":{"name":"s"},"data":{"a":"b"}}`)
	if !strings.Contains(strings.Join(bom.Notes, " "), "ignored 1 Secret objects") {
		t.Fatalf("notes = %v", bom.Notes)
	}
}

// fakeKubectl answers by resource name and records every argv.
type fakeKubectl struct {
	calls   [][]string
	answers map[string]string
	errs    map[string]error
}

func (f *fakeKubectl) run(_ context.Context, args []string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if err := f.errs[args[1]]; err != nil {
		return nil, err
	}
	if out, ok := f.answers[args[1]]; ok {
		return []byte(out), nil
	}
	return []byte(`{"apiVersion":"v1","kind":"List","items":[]}`), nil
}

func TestCollectOnlyEverIssuesGetAndNeverAsksForSecrets(t *testing.T) {
	for _, opts := range []Options{{}, {Namespace: "ai"}, {Context: "arn:aws:eks:us-east-1:123:cluster/prod"}} {
		f := &fakeKubectl{}
		if _, err := Collect(context.Background(), f.run, opts); err != nil {
			t.Fatal(err)
		}
		if len(f.calls) != len(collectResources) {
			t.Fatalf("%d calls", len(f.calls))
		}
		for _, args := range f.calls {
			if args[0] != "get" {
				t.Fatalf("verb %q issued", args[0])
			}
			for _, a := range args[1:] {
				if !strings.HasPrefix(a, "-") && a != "json" && !isCollectResource(a) {
					t.Fatalf("unexpected positional argument %q in %v", a, args)
				}
				if strings.Contains(a, "secret") {
					t.Fatalf("secrets requested: %v", args)
				}
			}
		}
	}
	if _, err := KubectlRunner(0)(context.Background(), []string{"delete", "pods"}); err == nil {
		t.Fatal("the runner accepted a verb other than get")
	}
	if _, err := KubectlRunner(0)(context.Background(), nil); err == nil {
		t.Fatal("the runner accepted an empty argv")
	}
}

func isCollectResource(name string) bool {
	for _, r := range collectResources {
		if r.name == name {
			return true
		}
	}
	return false
}

func TestCollectBuildsNamespaceAndContextFlags(t *testing.T) {
	args := strings.Join(kubectlArgs("pods", Options{Namespace: "ai", Context: "kind-dev"}), " ")
	if args != "get pods -o json --request-timeout=60s --namespace=ai --context=kind-dev" {
		t.Fatalf("args = %q", args)
	}
	if got := collectSubject(Options{Namespace: "ai", Context: "kind-dev"}); got != "kubectl context kind-dev, namespace ai" {
		t.Fatalf("subject = %q", got)
	}
}

func TestCollectRejectsHostileContextAndNamespaceNames(t *testing.T) {
	for _, opts := range []Options{{Context: "--kubeconfig=/tmp/x"}, {Context: "a b"}, {Context: "x;rm"}, {Context: strings.Repeat("a", 300)},
		{Namespace: "Bad_NS"}, {Namespace: "-n"}} {
		f := &fakeKubectl{}
		if _, err := Collect(context.Background(), f.run, opts); err == nil || len(f.calls) != 0 {
			t.Errorf("%+v accepted", opts)
		}
	}
	for _, ok := range []string{"gke_proj_us-central1_c1", "kind-dev", "user@cluster", "arn:aws:eks:eu-west-1:1:cluster/x"} {
		if err := validateOptions(Options{Context: ok}); err != nil {
			t.Errorf("%q rejected", ok)
		}
	}
}

func TestCollectTurnsForbiddenAndMissingResourcesIntoNotes(t *testing.T) {
	f := &fakeKubectl{
		answers: map[string]string{"pods": vllmFixture},
		errs: map[string]error{
			"configmaps":                          errors.New(`Error from server (Forbidden): configmaps is forbidden: User "x" cannot list resource`),
			"rayclusters.ray.io":                  errors.New(`error: the server doesn't have a resource type "rayclusters"`),
			"ingresses":                           errors.New(`the server could not find the requested resource`),
			"inferenceservices.serving.kserve.io": errors.New(`Error from server (NotFound): x`),
		},
	}
	bom, err := Collect(context.Background(), f.run, Options{})
	if err != nil {
		t.Fatal(err)
	}
	mustFind(t, bom, aibom.KindServing, "vllm")
	joined := strings.Join(bom.Notes, "\n")
	for _, want := range []string{"could not list configmaps: access is forbidden", "rayclusters.ray.io is not installed",
		"could not list ingresses: the resource type was not found", "inferenceservices.serving.kserve.io is not installed"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes lack %q: %v", want, bom.Notes)
		}
	}
	if bom.Subject != "current kubectl context" {
		t.Fatalf("subject = %q", bom.Subject)
	}
}

func TestCollectFailsOnRealErrorsBadOutputAndCancellation(t *testing.T) {
	cases := []*fakeKubectl{
		{errs: map[string]error{"pods": errors.New("Unable to connect to the server: dial tcp: connection refused")}},
		{answers: map[string]string{"pods": "not json"}},
		{answers: map[string]string{"pods": vllmFixture}},
	}
	for i, f := range cases {
		opts := Options{}
		if i == 2 {
			opts.MaxBytes = 10
		}
		if _, err := Collect(context.Background(), f.run, opts); err == nil || !strings.HasPrefix(err.Error(), "aibom/kube: ") {
			t.Errorf("case %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeKubectl{}
	if _, err := Collect(ctx, f.run, Options{}); !errors.Is(err, context.Canceled) || len(f.calls) != 0 {
		t.Fatalf("cancelled: %v, %d calls", err, len(f.calls))
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancelling := func(_ context.Context, _ []string) ([]byte, error) {
		cancel()
		return nil, errors.New("signal: killed")
	}
	if _, err := Collect(ctx, cancelling, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled mid-run: %v", err)
	}
	//lint:ignore SA1012 checking the nil-context guard on purpose
	if _, err := Collect(nil, f.run, Options{}); err == nil { //nolint:staticcheck
		t.Fatal("nil context accepted")
	}
}

// writeFakeKubectl puts a shell script named kubectl on PATH.
func writeFakeKubectl(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("KUBE_SECRET_SHOULD_NOT_PASS", "x")
}

func TestKubectlRunnerPassesOnlyAMinimalEnvironmentAndTheArgs(t *testing.T) {
	writeFakeKubectl(t, `printf '{"kind":"List","items":[],"env":"%s","args":"%s"}' "$(env | sort | tr '\n' ' ')" "$*"`)

	out, err := KubectlRunner(0)(context.Background(), []string{"get", "pods", "-o", "json"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "KUBE_SECRET_SHOULD_NOT_PASS") || !strings.Contains(s, "PATH=") || !strings.Contains(s, `"args":"get pods -o json"`) {
		t.Fatalf("output = %s", s)
	}
}

func TestKubectlRunnerCapsOutputAndReportsStderr(t *testing.T) {
	writeFakeKubectl(t, `i=0; while [ $i -lt 200 ]; do printf '0123456789'; i=$((i+1)); done`)
	if _, err := KubectlRunner(100)(context.Background(), []string{"get", "pods"}); err == nil || !strings.Contains(err.Error(), "larger than 100 bytes") {
		t.Fatalf("cap: %v", err)
	}

	writeFakeKubectl(t, `echo 'Error from server (Forbidden): pods is forbidden' >&2; exit 1`)
	_, err := KubectlRunner(0)(context.Background(), []string{"get", "pods"})
	if err == nil || !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("stderr: %v", err)
	}
	if note, ok := failureNote(collectResource{name: "pods"}, err); !ok || !strings.Contains(note, "forbidden") {
		t.Fatalf("note = %q", note)
	}
}

func TestKubectlSeesCloudSignInVariablesButNotUnrelatedSecrets(t *testing.T) {
	kept := signInEnv([]string{
		"PATH=/usr/bin", "KUBECONFIG=/k", "AWS_PROFILE=prod", "GOOGLE_APPLICATION_CREDENTIALS=/g.json",
		"AZURE_CONFIG_DIR=/a", "HTTPS_PROXY=http://proxy:3128",
		"SOME_SERVICE_API_KEY=do-not-pass", "KUBE_EDITOR=vi", "malformed",
	})

	got := strings.Join(kept, " ")
	for _, want := range []string{"PATH=", "KUBECONFIG=", "AWS_PROFILE=", "GOOGLE_APPLICATION_CREDENTIALS=", "AZURE_CONFIG_DIR=", "HTTPS_PROXY="} {
		if !strings.Contains(got, want) {
			t.Errorf("%s was not passed to kubectl", want)
		}
	}
	if strings.Contains(got, "do-not-pass") || strings.Contains(got, "KUBE_EDITOR") || strings.Contains(got, "malformed") {
		t.Fatalf("kubectl would see %q", got)
	}
}
