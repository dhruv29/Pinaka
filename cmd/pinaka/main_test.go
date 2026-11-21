package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
	"github.com/dhruv29/Pinaka/internal/aibom/kube"
)

// vllmDeployment is "kubectl get -o json" output for one model server.
const vllmDeployment = `{"apiVersion":"v1","kind":"List","items":[
 {"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"vllm-chat","namespace":"ml"},
  "spec":{"template":{"metadata":{"labels":{"app":"vllm"}},"spec":{"containers":[
   {"name":"server","image":"vllm/vllm-openai:v0.6.3","args":["--model","meta-llama/Llama-3.1-8B-Instruct"]}]}}}}
]}`

// project writes a small code tree that uses a hosted model and ships a
// pickle file, and returns its directory.
func project(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"requirements.txt": "anthropic==0.39.0\n",
		"app.py":           "import anthropic\nMODEL = \"claude-sonnet-4-5\"\n",
		"models/score.pkl": "\x80\x04pickle",
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func scan(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(context.Background(), append([]string{"scan"}, args...), strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestScanWritesAReportForARepository(t *testing.T) {
	code, stdout, stderr := scan(t, "", "-repo", project(t))

	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"# AI bill of materials", "## Look at these first", "anthropic", "claude-sonnet-4-5", "score.pkl"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the report lacks %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "component(s)") {
		t.Errorf("stderr = %q; want a one-line summary", stderr)
	}
}

func TestScanWritesJSONThatAnotherProgramCanRead(t *testing.T) {
	code, stdout, stderr := scan(t, "", "-repo", project(t), "-format", "json")

	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var bom aibom.BOM
	if err := json.Unmarshal([]byte(stdout), &bom); err != nil {
		t.Fatalf("output is not a bill: %v", err)
	}
	if len(bom.Components) == 0 || bom.Subject == "" {
		t.Fatalf("bill = %+v; want components and a subject", bom)
	}
}

func TestScanWritesCycloneDX(t *testing.T) {
	code, stdout, stderr := scan(t, "", "-repo", project(t), "-format", "cyclonedx")

	if code != exitOK || !strings.Contains(stdout, `"bomFormat": "CycloneDX"`) || !strings.Contains(stdout, "machine-learning-model") {
		t.Fatalf("exit %d, stderr %q, output:\n%s", code, stderr, stdout)
	}
}

func TestScanMergesARepositoryWithSavedClusterOutput(t *testing.T) {
	saved := filepath.Join(t.TempDir(), "cluster.json")
	if err := os.WriteFile(saved, []byte(vllmDeployment), 0o600); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := scan(t, "", "-repo", project(t), "-kube-file", saved, "-format", "json")

	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var bom aibom.BOM
	if err := json.Unmarshal([]byte(stdout), &bom); err != nil {
		t.Fatal(err)
	}
	origins := map[string]bool{}
	for _, c := range bom.Components {
		origins[c.Origin] = true
	}
	if !origins[aibom.OriginRepo] || !origins[aibom.OriginCluster] || !strings.Contains(bom.Subject, " + ") {
		t.Fatalf("subject %q, origins %v; want one bill covering code and cluster", bom.Subject, origins)
	}
}

func TestScanJoinsWhatTheCodeAndTheClusterBothHaveIntoOneComponent(t *testing.T) {
	saved := filepath.Join(t.TempDir(), "cluster.json")
	cluster := `{"kind":"Pod","apiVersion":"v1","metadata":{"name":"bot","namespace":"ml"},"spec":{"containers":[{"name":"c","image":"corp/bot:1",
	 "env":[{"name":"ANTHROPIC_API_KEY","valueFrom":{"secretKeyRef":{"name":"k","key":"k"}}},{"name":"MODEL_NAME","value":"claude-sonnet-4-5"}]}]}}`
	if err := os.WriteFile(saved, []byte(cluster), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := scan(t, "", "-repo", project(t), "-kube-file", saved, "-format", "json")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var bom aibom.BOM
	if err := json.Unmarshal([]byte(stdout), &bom); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, c := range bom.Components {
		seen[string(c.Kind)+" "+c.Name]++
		switch string(c.Kind) + " " + c.Name {
		case "provider Anthropic API", "model claude-sonnet-4-5":
			if c.Origin != aibom.OriginBoth {
				t.Errorf("%s %q origin %q, want %q", c.Kind, c.Name, c.Origin, aibom.OriginBoth)
			}
		}
	}
	for key, n := range seen {
		if n > 1 {
			t.Errorf("%s listed %d times", key, n)
		}
	}
	if seen["provider Anthropic API"] != 1 || seen["model claude-sonnet-4-5"] != 1 {
		t.Errorf("components = %v", seen)
	}
}

func TestScanReadsClusterOutputFromStandardInput(t *testing.T) {
	code, stdout, stderr := scan(t, vllmDeployment, "-kube-file", "-")

	if code != exitOK || !strings.Contains(stdout, "vllm") {
		t.Fatalf("exit %d, stderr %q, output:\n%s", code, stderr, stdout)
	}
}

func TestScanReadsALiveClusterThroughTheCollector(t *testing.T) {
	original := collectCluster
	t.Cleanup(func() { collectCluster = original })
	var asked kube.Options
	collectCluster = func(_ context.Context, opts kube.Options) (aibom.BOM, error) {
		asked = opts
		return kube.Parse(strings.NewReader(vllmDeployment), opts)
	}

	code, stdout, stderr := scan(t, "", "-kube", "-context", "prod", "-namespace", "ml")

	if code != exitOK || asked.Context != "prod" || asked.Namespace != "ml" || !strings.Contains(stdout, "vllm") {
		t.Fatalf("exit %d, asked %+v, stderr %q", code, asked, stderr)
	}
}

func TestScanReportsAClusterThatCannotBeRead(t *testing.T) {
	original := collectCluster
	t.Cleanup(func() { collectCluster = original })
	collectCluster = func(context.Context, kube.Options) (aibom.BOM, error) {
		return aibom.BOM{}, errors.New("kubectl: connection refused")
	}

	code, stdout, stderr := scan(t, "", "-kube")

	if code != exitError || stdout != "" || !strings.Contains(stderr, "connection refused") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestFailOnStopsAPipelineAtTheChosenSeverity(t *testing.T) {
	root := project(t)

	flaggedCode, _, flaggedErr := scan(t, "", "-repo", root, "-fail-on", "high")
	if flaggedCode != exitFlagged || !strings.Contains(flaggedErr, "severity high or worse") {
		t.Fatalf("exit %d, stderr %q; want the pickle file to trip -fail-on high", flaggedCode, flaggedErr)
	}

	passCode, _, passErr := scan(t, "", "-repo", root, "-fail-on", "critical")
	if passCode != exitOK {
		t.Fatalf("exit %d, stderr %q; nothing here is critical", passCode, passErr)
	}
}

func TestScanWritesAPrivateFileWhenAskedTo(t *testing.T) {
	out := filepath.Join(t.TempDir(), "ai-bom.json")

	code, stdout, stderr := scan(t, "", "-repo", project(t), "-format", "json", "-out", out)

	if code != exitOK || stdout != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != outputFileMode || info.Size() == 0 {
		t.Fatalf("mode %v, size %d; want a non-empty file only its owner can read", info.Mode().Perm(), info.Size())
	}
}

func TestScanRefusesBadUsage(t *testing.T) {
	cases := map[string][]string{
		"nothing to scan":     {},
		"two cluster sources": {"-kube", "-kube-file", "x.json"},
		"context without":     {"-repo", ".", "-context", "prod"},
		"unknown format":      {"-repo", ".", "-format", "pdf"},
		"unknown severity":    {"-repo", ".", "-fail-on", "scary"},
		"stray argument":      {"-repo", ".", "extra"},
		"unknown flag":        {"-nope"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := scan(t, "", args...)
			if code != exitUsage || stdout != "" || stderr == "" {
				t.Fatalf("exit %d, stdout %q, stderr %q; want a usage error", code, stdout, stderr)
			}
		})
	}
}

func TestScanReportsWhatItCannotRead(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	cases := map[string][]string{
		"missing repository":   {"-repo", missing},
		"missing cluster file": {"-kube-file", missing},
		"unwritable output":    {"-repo", project(t), "-out", filepath.Join(missing, "out.md")},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if code, _, stderr := scan(t, "", args...); code != exitError || stderr == "" {
				t.Fatalf("exit %d, stderr %q; want a failure with a reason", code, stderr)
			}
		})
	}
	if code, _, stderr := scan(t, "{not json", "-kube-file", "-"); code != exitError || stderr == "" {
		t.Fatalf("exit %d, stderr %q; want malformed cluster output refused", code, stderr)
	}
}

func TestTopLevelCommands(t *testing.T) {
	cases := map[string]struct {
		args []string
		code int
		want string
	}{
		"no command": {nil, exitUsage, "Usage:"},
		"help":       {[]string{"help"}, exitOK, "Usage:"},
		"version":    {[]string{"version"}, exitOK, version},
		"unknown":    {[]string{"explode"}, exitUsage, `unknown command "explode"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), tc.args, strings.NewReader(""), &stdout, &stderr)
			if code != tc.code || !strings.Contains(stdout.String()+stderr.String(), tc.want) {
				t.Fatalf("exit %d, output %q; want exit %d mentioning %q", code, stdout.String()+stderr.String(), tc.code, tc.want)
			}
		})
	}
}
