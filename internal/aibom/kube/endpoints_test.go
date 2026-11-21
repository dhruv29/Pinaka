package kube

import (
	"strings"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

func TestPlaceholderKeysForASelfHostedServerAreNotCredentialsAtRisk(t *testing.T) {
	c := `{"name":"c","image":"corp/chat:1","args":["--api-key","sk-no-key-required"],"env":[{"name":"OPENAI_API_KEY","value":"EMPTY"},
	 {"name":"ANTHROPIC_API_KEY","value":"dummy"},{"name":"GROQ_API_KEY","value":"none"},{"name":"MISTRAL_API_KEY","value":"ollama"},
	 {"name":"COHERE_API_KEY","value":"abc123"}]}`
	cm := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cfg","namespace":"n"},"data":{"OPENAI_API_KEY":"EMPTY","app.env":"HF_TOKEN=none"}}`
	bom := mustParse(t, `{"kind":"List","items":[`+podJSON(c, "")+`,`+cm+`]}`)
	for _, comp := range bom.Components {
		if len(comp.Risks) != 0 {
			t.Errorf("%s %q flagged for a placeholder: %+v", comp.Kind, comp.Name, comp.Risks)
		}
	}
}

func TestAClientPointedAtAnInClusterServerIsNotListedAsTheVendor(t *testing.T) {
	c := `{"name":"chat","image":"corp/chat:1","env":[{"name":"OPENAI_API_KEY","value":"EMPTY"},
	 {"name":"OPENAI_BASE_URL","value":"http://u:pw@vllm.ml.svc:8000/v1?k=x"}]}`
	bom := mustParse(t, podJSON(c, ""))
	ep := mustFind(t, bom, aibom.KindProvider, "custom endpoint vllm.ml.svc:8000")
	if ep.Properties["scope"] != "private" {
		t.Errorf("endpoint = %+v", ep)
	}
	for _, comp := range bom.Components {
		if comp.Kind == aibom.KindProvider && comp.Name != ep.Name {
			t.Errorf("vendor %q inferred although the client points elsewhere", comp.Name)
		}
	}
	mustFind(t, bom, aibom.KindCredential, "OPENAI_API_KEY")

	external := mustParse(t, podJSON(`{"name":"c","image":"corp/x:1","env":[{"name":"LLM_BASE_URL","value":"https://llm.partner.example/v1"},
	 {"name":"DATABASE_URL","value":"postgres://db.example/x"},{"name":"API_URL","value":"https://api.shop.example.com"}]}`, ""))
	if mustFind(t, external, aibom.KindProvider, "custom endpoint llm.partner.example").Properties["scope"] != "external" {
		t.Error("an outside AI endpoint is not marked external")
	}
	if len(external.Components) != 1 {
		t.Errorf("non-AI URLs listed: %+v", external.Components)
	}
}

func TestACredentialRiskPointsAtTheWorkloadThatRaisedIt(t *testing.T) {
	c := `{"name":"c","image":"corp/x:1","env":[{"name":"OPENAI_API_KEY","value":"` + secretValue + `"}]}`
	cm := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"aaa","namespace":"a"},"data":{"OPENAI_API_KEY":"${FROM_ENV}"}}`
	bom := mustParse(t, `{"kind":"List","items":[`+cm+`,`+podJSON(c, "")+`]}`)
	r := mustRisk(t, mustFind(t, bom, aibom.KindCredential, "OPENAI_API_KEY"), riskLiteralCredential, aibom.SeverityHigh)
	if r.Where == nil || *r.Where != (aibom.Evidence{Path: "n/Pod/p", Detail: "containers[c].env OPENAI_API_KEY"}) {
		t.Errorf("where = %+v", r.Where)
	}
}

func TestParseNotesEachExpectedKindMissingFromTheInputAndWhatItHides(t *testing.T) {
	bom := mustParse(t, podJSON(`{"name":"c","image":"ollama/ollama:0.3.0"}`, ""))
	joined := strings.Join(bom.Notes, "\n")
	for _, want := range []string{
		"no Service objects", "reachable through a LoadBalancer, NodePort or externalIPs could not be judged",
		"no Ingress objects", "no HTTPRoute objects", "no ConfigMap objects", "no Deployment objects",
		"no InferenceService objects", "no RayCluster objects", "no Notebook objects",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes lack %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "no Pod objects") {
		t.Error("a kind that is present is noted as missing")
	}

	all := mustParse(t, vllmFixture)
	if strings.Contains(strings.Join(all.Notes, "\n"), "no Service objects") {
		t.Errorf("notes = %v", all.Notes)
	}
}

func TestLongCommandLinesAndConfigMapValuesAreCutWithANote(t *testing.T) {
	words := strings.TrimSuffix(strings.Repeat(`"x",`, maxTokens+1), ",")
	long := strings.Repeat("A=b\n", maxConfigLines+1)
	cm := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"big","namespace":"n"},"data":{"app.env":"` + strings.ReplaceAll(long, "\n", `\n`) + `"}}`
	bom := mustParse(t, `{"kind":"List","items":[`+podJSON(`{"name":"c","image":"corp/x:1","args":[`+words+`]}`, "")+`,`+cm+`]}`)
	joined := strings.Join(bom.Notes, "\n")
	if !strings.Contains(joined, "n/Pod/p containers[c]: the command line has more than 4096 words") ||
		!strings.Contains(joined, "n/ConfigMap/big data[app.env]: only the first 5000 lines were read") {
		t.Errorf("notes = %v", bom.Notes)
	}
}
