package kube

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

func mustNotLeak(t *testing.T, bom aibom.BOM, secret string) {
	t.Helper()
	data, err := json.Marshal(bom)
	if err != nil {
		t.Fatal(err)
	}
	cdx, err := aibom.CycloneDX(bom)
	if err != nil {
		t.Fatal(err)
	}
	for format, out := range map[string]string{"json": string(data), "markdown": aibom.Markdown(bom), "cyclonedx": string(cdx)} {
		if strings.Contains(out, secret) {
			t.Errorf("%s output contains the secret %q", format, secret)
		}
	}
}

func TestModelReferencesKeepTheLocationButNeverUserInfoOrQuery(t *testing.T) {
	const secret = "Pw9xLEAKq7"
	c := fmt.Sprintf(`{"name":"c","image":"vllm/vllm-openai:v0.6.3","args":["--model","admin:%[1]s@registry.example/org/llama"],
	 "env":[{"name":"MODEL_ID","value":"tok%[1]s@models.example/org/mistral?sig=%[1]s"}]},
	 {"name":"o","image":"ollama/ollama:0.3.0","command":["sh","-c","ollama pull u:%[1]s@reg.example/library/llama3:8b"]}`, secret)
	bom := mustParse(t, podJSON(c, ""))
	mustNotLeak(t, bom, secret)
	mustFind(t, bom, aibom.KindModel, "registry.example/org/llama")
	mustFind(t, bom, aibom.KindModel, "models.example/org/mistral")
	mustFind(t, bom, aibom.KindModel, "reg.example/library/llama3")
}

// TestNoPlacementOfASecretReachesAnyOutput is a property test: a secret put
// in any field of a workload or ConfigMap never appears in the JSON,
// Markdown or CycloneDX rendering of the bill.
func TestNoPlacementOfASecretReachesAnyOutput(t *testing.T) {
	for _, secret := range []string{"sk-proj-" + strings.Repeat("Qz7Lp2Xw", 4), "hf_" + strings.Repeat("Mn4Bv8Cx", 3), "Pw9xLEAKq7zz"} {
		c := fmt.Sprintf(`{"name":"c","image":"vllm/vllm-openai:v0.6.3",
		 "args":["--model","u:%[1]s@r.example/o/m","--api-key=%[1]s","--hf-token","%[1]s","--served-model-name","x"],
		 "env":[{"name":"OPENAI_API_KEY","value":"%[1]s"},{"name":"HF_TOKEN","value":"%[1]s"},
		  {"name":"OPENAI_BASE_URL","value":"https://u:%[1]s@api.openai.com/v1?k=%[1]s"},
		  {"name":"LLM_BASE_URL","value":"http://u:%[1]s@vllm.ml.svc:8000/v1?k=%[1]s"},
		  {"name":"MODEL_NAME","value":"s3://u:%[1]s@bucket/m?X-Amz-Signature=%[1]s"},
		  {"name":"OLLAMA_MODEL","value":"%[1]s@reg.example/llama3"}]}`, secret)
		cm := fmt.Sprintf(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cfg","namespace":"n"},"data":{
		 "OPENAI_API_KEY":"%[1]s","app.env":"ANTHROPIC_API_KEY=%[1]s\nLLM_BASE_URL=https://x:%[1]s@api.anthropic.com/v1\nMODEL_ID=u:%[1]s@h.example/o/m"}}`, secret)
		mustNotLeak(t, mustParse(t, `{"kind":"List","items":[`+podJSON(c, "")+`,`+cm+`]}`), secret)
	}
}
