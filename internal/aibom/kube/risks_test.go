package kube

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

func TestEveryRiskInTheTableHasASummaryAndSeverities(t *testing.T) {
	for id, def := range riskTable {
		if def.Summary == "" || def.Base == "" || def.Raised == "" {
			t.Errorf("risk %s is incomplete: %+v", id, def)
		}
	}
}

func TestExposureSeverityDependsOnWhetherTheServerIsOpenByDefault(t *testing.T) {
	svc := `{"kind":"Service","apiVersion":"v1","metadata":{"name":"s","namespace":"n"},"spec":{"type":"LoadBalancer","selector":{"app":"x"},"ports":[{"port":80}]},"status":{"loadBalancer":{"ingress":[{"ip":"203.0.113.5"}]}}}`
	open := mustParse(t, imagePod("ghcr.io/huggingface/text-generation-inference:2")+svc)
	mustRisk(t, mustFind(t, open, aibom.KindServing, "text-generation-inference"), riskExposedInference, aibom.SeverityHigh)

	closed := mustParse(t, imagePod("ghcr.io/open-webui/open-webui:main")+svc)
	ui := mustFind(t, closed, aibom.KindServing, "open-webui")
	mustRisk(t, ui, riskExposedInference, aibom.SeverityMedium)
	if ui.Properties["exposed_host"] != "203.0.113.5" {
		t.Fatalf("host = %+v", ui.Properties)
	}

	store := mustParse(t, imagePod("qdrant/qdrant:v1")+svc)
	mustRisk(t, mustFind(t, store, aibom.KindDataStore, "qdrant"), riskExposedService, aibom.SeverityHigh)
}

func TestServicesThatDoNotSelectTheWorkloadDoNotExposeIt(t *testing.T) {
	for _, svc := range []string{
		`{"kind":"Service","apiVersion":"v1","metadata":{"name":"s","namespace":"n"},"spec":{"type":"LoadBalancer","selector":{"app":"other"}}}`,
		`{"kind":"Service","apiVersion":"v1","metadata":{"name":"s","namespace":"other"},"spec":{"type":"LoadBalancer","selector":{"app":"x"}}}`,
		`{"kind":"Service","apiVersion":"v1","metadata":{"name":"s","namespace":"n"},"spec":{"type":"LoadBalancer"}}`,
		`{"kind":"Service","apiVersion":"v1","metadata":{"name":"s","namespace":"n"},"spec":{"type":"ClusterIP","selector":{"app":"x"}}}`,
	} {
		bom := mustParse(t, imagePod("ollama/ollama:0.3")+svc)
		if _, ok := riskOf(mustFind(t, bom, aibom.KindServing, "ollama"), riskExposedInference); ok {
			t.Errorf("exposed through %s", svc)
		}
	}
}

func TestHTTPRoutesExternalIPsAndDefaultBackendsExposeWorkloads(t *testing.T) {
	svc := `{"kind":"Service","apiVersion":"v1","metadata":{"name":"s","namespace":"n"},"spec":{"selector":{"app":"x"},"externalIPs":["198.51.100.1"],"ports":[{"port":80}]}}`
	route := `{"kind":"HTTPRoute","apiVersion":"gateway.networking.k8s.io/v1","metadata":{"name":"r","namespace":"n"},"spec":{"hostnames":["llm.example.com"],"rules":[{"backendRefs":[{"name":"s","port":80},{"kind":"Bucket","name":"b"}]}]}}`
	legacy := `{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"i","namespace":"n"},"spec":{"backend":{"serviceName":"s","servicePort":"http"},"rules":[{"host":"x"}]}}`
	bom := mustParse(t, imagePod("ollama/ollama:0.3")+svc+route+legacy)

	o := mustFind(t, bom, aibom.KindServing, "ollama")
	r := mustRisk(t, o, riskExposedInference, aibom.SeverityHigh)
	for _, want := range []string{"HTTPRoute n/HTTPRoute/r host llm.example.com port 80", "externalIPs", "Ingress n/Ingress/i host * port http"} {
		if !strings.Contains(r.Summary, want) {
			t.Errorf("summary lacks %q: %s", want, r.Summary)
		}
	}
}

func TestPrivilegedWorkloadsAreRankedByHowMuchOfTheHostTheyGet(t *testing.T) {
	medium := mustParse(t, podJSON(`{"name":"c","image":"ollama/ollama:0.3"}`, `"hostNetwork":true,"hostIPC":true,"volumes":[{"name":"v","hostPath":{"path":"/var/lib/models"}}]`))
	r := mustRisk(t, mustFind(t, medium, aibom.KindServing, "ollama"), riskPrivileged, aibom.SeverityMedium)
	if !strings.Contains(r.Summary, "hostPath /var/lib/models") {
		t.Fatalf("summary = %q", r.Summary)
	}

	high := mustParse(t, podJSON(`{"name":"c","image":"ollama/ollama:0.3","securityContext":{"privileged":true,"runAsUser":0}}`, `"hostPID":true`))
	mustRisk(t, mustFind(t, high, aibom.KindServing, "ollama"), riskPrivileged, aibom.SeverityHigh)

	gpuOnly := mustParse(t, podJSON(`{"name":"c","image":"corp/x:1","securityContext":{"privileged":true},"resources":{"limits":{"nvidia.com/gpu":"1"}}}`, ""))
	mustRisk(t, mustFind(t, gpuOnly, aibom.KindAccelerator, "nvidia.com/gpu"), riskPrivileged, aibom.SeverityHigh)

	if plain := mustParse(t, podJSON(`{"name":"c","image":"nginx","securityContext":{"privileged":true}}`, "")); len(plain.Components) != 0 {
		t.Fatalf("a non-AI privileged pod was reported: %+v", plain.Components)
	}
}

func TestJupyterWithATokenIsMediumWhenExposedAndSilentOtherwise(t *testing.T) {
	nb := podJSON(`{"name":"c","image":"jupyter/base-notebook:lab-4","args":["--ServerApp.token=","--ServerApp.password=sha1:abc"]}`, "")
	quiet := mustParse(t, nb)
	if c := mustFind(t, quiet, aibom.KindPlatform, "jupyter"); len(c.Risks) != 0 {
		t.Fatalf("password-protected unexposed notebook flagged: %+v", c.Risks)
	}

	svc := `{"kind":"Service","apiVersion":"v1","metadata":{"name":"s","namespace":"n"},"spec":{"type":"NodePort","selector":{"app":"x"}}}`
	mustRisk(t, mustFind(t, mustParse(t, nb+svc), aibom.KindPlatform, "jupyter"), riskNotebookExposed, aibom.SeverityMedium)

	open := podJSON(`{"name":"c","image":"jupyter/base-notebook:lab-4","args":["--NotebookApp.token=''"]}`, "")
	r := mustRisk(t, mustFind(t, mustParse(t, open), aibom.KindPlatform, "jupyter"), riskNotebookExposed, aibom.SeverityHigh)
	if !strings.Contains(r.Summary, "empty token") {
		t.Fatalf("summary = %q", r.Summary)
	}
}

func TestLiteralAndConfigMapCredentialReferencesAreRisks(t *testing.T) {
	c := `{"name":"c","image":"corp/bot:1","env":[{"name":"ANTHROPIC_API_KEY","valueFrom":{"configMapKeyRef":{"name":"cfg","key":"k"}}},
	 {"name":"GROQ_API_KEY","valueFrom":{"fieldRef":{"fieldPath":"x"}}},{"name":"OPENAI_API_KEY","value":"${FROM_ELSEWHERE}"}]}`
	bom := mustParse(t, podJSON(c, ""))

	mustRisk(t, mustFind(t, bom, aibom.KindCredential, "ANTHROPIC_API_KEY"), riskConfigMapCred, aibom.SeverityHigh)
	for _, name := range []string{"GROQ_API_KEY", "OPENAI_API_KEY"} {
		if c := mustFind(t, bom, aibom.KindCredential, name); len(c.Risks) != 0 {
			t.Errorf("%s flagged: %+v", name, c.Risks)
		}
	}
}

func TestUnpinnedImagesAreLowRiskAndLatestCounts(t *testing.T) {
	bom := mustParse(t, imagePod("qdrant/qdrant:latest"))
	mustRisk(t, mustFind(t, bom, aibom.KindDataStore, "qdrant"), riskUnpinnedImage, aibom.SeverityLow)

	bom = mustParse(t, imagePod("qdrant/qdrant:v1.11.0"))
	if _, ok := riskOf(mustFind(t, bom, aibom.KindDataStore, "qdrant"), riskUnpinnedImage); ok {
		t.Fatal("a versioned tag was called unpinned")
	}
}

func TestRiskSummariesListAtMostAFewWorkloads(t *testing.T) {
	var docs []string
	for i := 0; i < maxListedWorkloads+3; i++ {
		docs = append(docs, strings.Replace(imagePod("ollama/ollama"), `"name":"p"`, fmt.Sprintf(`"name":"p%d"`, i), 1))
	}
	r := mustRisk(t, mustFind(t, mustParse(t, strings.Join(docs, "")), aibom.KindServing, "ollama"), riskUnpinnedImage, aibom.SeverityLow)

	if !strings.Contains(r.Summary, "and 3 more") {
		t.Fatalf("summary = %q", r.Summary)
	}
}
