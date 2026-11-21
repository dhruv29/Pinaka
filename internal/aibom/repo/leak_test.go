package repo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// renderings returns every form a bill is written in.
func renderings(t *testing.T, bom aibom.BOM) map[string]string {
	t.Helper()
	data, err := json.Marshal(bom)
	if err != nil {
		t.Fatal(err)
	}
	cdx, err := aibom.CycloneDX(bom)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"json": string(data), "markdown": aibom.Markdown(bom), "cyclonedx": string(cdx)}
}

func mustNotLeak(t *testing.T, bom aibom.BOM, secret string) {
	t.Helper()
	for format, out := range renderings(t, bom) {
		if strings.Contains(out, secret) {
			t.Errorf("%s output contains the secret %q", format, secret)
		}
	}
}

func TestModelfileBaseKeepsSchemeHostAndPathButNeverUserInfoOrQuery(t *testing.T) {
	const secret = "Pw9xLEAKq7"
	bom := scanFiles(t, map[string]string{
		"a/Modelfile":   "FROM https://user:" + secret + "@models.example.com/w/x.gguf?sig=" + secret + "\n",
		"b/Modelfile":   "FROM user:" + secret + "@registry.example/library/llama3:8b\n",
		"c/x.modelfile": "FROM ./weights.gguf?token=" + secret + "\n",
	})
	mustNotLeak(t, bom, secret)
	mustFind(t, bom, aibom.KindModel, "https://models.example.com/w/x.gguf")
	mustFind(t, bom, aibom.KindModel, "registry.example/library/llama3:8b")
	mustFind(t, bom, aibom.KindModel, "./weights.gguf")
}

// TestNoPlacementOfASecretReachesAnyOutput is a property test: a secret put
// in any of the places a scanned project can hold one never appears in the
// JSON, Markdown or CycloneDX rendering of the bill.
func TestNoPlacementOfASecretReachesAnyOutput(t *testing.T) {
	secrets := []string{"sk-proj-" + strings.Repeat("Qz7Lp2Xw", 4), "hf_" + strings.Repeat("Mn4Bv8Cx", 3), "Pw9xLEAKq7zz"}
	placements := []struct{ path, template string }{
		{".env", "OPENAI_API_KEY=%s\nOPENAI_BASE_URL=https://u:%s@proxy.example.com/v1?k=%s\n"},
		{"app.py", "from openai import OpenAI\nc = OpenAI(api_key=\"%s\", base_url=\"https://u:%s@llm.corp.example/v1?t=%s\")\n"},
		{"cfg.yaml", "llm:\n  openai_api_key: %s\n  api_key: \"%s\"\n  base_url: http://10.0.0.5:8000/v1?api_key=%s\n"},
		{"src/i.ts", "import Anthropic from \"@anthropic-ai/sdk\";\nconst a = new Anthropic({ apiKey: \"%s\", baseURL: \"https://x:%s@h.example/?q=%s\" });\n"},
		{".mcp.json", `{"mcpServers":{"g":{"command":"npx","args":["-y","@modelcontextprotocol/server-github","--token=%s"],"env":{"GITHUB_TOKEN":"%s"}},"r":{"url":"https://u:%s@mcp.example.com/sse"}}}`},
		{"Modelfile", "FROM https://u:%s@h.example/x.gguf?a=%s&b=%s\n"},
		{"m.py", "from transformers import AutoModel\nAutoModel.from_pretrained(\"https://u:%s@huggingface.co/org/m?x=%s\", token=\"%s\")\n"},
	}
	for _, secret := range secrets {
		files := map[string]string{}
		for _, p := range placements {
			files[p.path] = strings.ReplaceAll(p.template, "%s", secret)
		}
		mustNotLeak(t, scanFiles(t, files), secret)
	}
}

func TestScanFlagsKeysPassedAsClientArgumentsAndInLowerCaseNames(t *testing.T) {
	anthropicKey := "sk-ant-api03-" + strings.Repeat("Hw4Jk7Pq", 4)
	bom := scanFiles(t, map[string]string{
		"app.py":     "from openai import OpenAI\nclient = OpenAI(api_key=\"" + fakeOpenAIKey + "\")\n",
		"src/b.ts":   "const a = new Anthropic({ apiKey: \"" + anthropicKey + "\" });\n",
		"conf.py":    "openai_api_key = \"" + fakeOpenAIKey + "\"\n",
		"ok.py":      "client = OpenAI(api_key=os.environ[\"OPENAI_API_KEY\"])\nstripe.api_key = \"" + "sk_" + "live_" + strings.Repeat("Aa1Bb2Cc3", 3) + "\"\n",
		"generic.js": "const c = new Client({ apiKey: \"" + strings.Repeat("Ab3Cd5Ef7", 3) + "\" });\n",
	})
	openai := mustFind(t, bom, aibom.KindCredential, "OpenAI API key (literal)")
	if riskSeverity(openai, "hardcoded-ai-credential") != aibom.SeverityHigh || openai.Supplier != "OpenAI" {
		t.Errorf("openai = %+v", openai)
	}
	if riskSeverity(mustFind(t, bom, aibom.KindCredential, "Anthropic API key (literal)"), "hardcoded-ai-credential") == "" {
		t.Error("an Anthropic key passed as apiKey is not flagged")
	}
	if riskSeverity(mustFind(t, bom, aibom.KindCredential, "OPENAI_API_KEY"), "hardcoded-ai-credential") == "" {
		t.Error("a lower-case openai_api_key assignment is not flagged")
	}
	if c := find(bom, aibom.KindCredential, "AI API key (literal)"); c != nil {
		t.Errorf("a key for a non-AI client is listed: %+v", c)
	}
	mustNotLeak(t, bom, fakeOpenAIKey)
	mustNotLeak(t, bom, anthropicKey)
}

func TestScanInfersTheHostedProviderFromTheSDKAndTheCredentialName(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"package.json": `{"dependencies":{"openai":"^4.20.0","@anthropic-ai/sdk":"0.24.0","express":"4"}}`,
		"src/i.ts":     "const o = new OpenAI({ apiKey: process.env.OPENAI_API_KEY });\n",
		"conf.py":      "key = os.environ[\"GROQ_API_KEY\"]\n",
		"README.md":    "Set MISTRAL_API_KEY to use Mistral.\n",
	})
	openai := mustFind(t, bom, aibom.KindProvider, "OpenAI API")
	details := []string{}
	for _, e := range openai.Evidence {
		details = append(details, e.Detail)
	}
	if !strings.Contains(strings.Join(details, ";"), "inferred from the dependency openai") ||
		!strings.Contains(strings.Join(details, ";"), "inferred from the credential name OPENAI_API_KEY") || openai.Supplier != "OpenAI" {
		t.Errorf("OpenAI API evidence = %v", details)
	}
	mustFind(t, bom, aibom.KindProvider, "Anthropic API")
	mustFind(t, bom, aibom.KindProvider, "Groq API")
	mustNotFind(t, bom, aibom.KindProvider, "Mistral AI API")
}

func TestScanDoesNotInferTheVendorWhenTheClientIsPointedElsewhere(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"requirements.txt": "openai==1.40.0\n",
		"app.py":           "from openai import OpenAI\nc = OpenAI(base_url=\"http://vllm.ml.svc:8000/v1\", api_key=os.environ[\"OPENAI_API_KEY\"])\n",
	})
	mustFind(t, bom, aibom.KindProvider, "custom endpoint vllm.ml.svc:8000")
	mustNotFind(t, bom, aibom.KindProvider, "OpenAI API")
}

func TestScanPointsARiskAtTheLineThatRaisedIt(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"app.py": "key = os.environ[\"OPENAI_API_KEY\"]\n",
		"z.env":  "\nOPENAI_API_KEY=" + fakeOpenAIKey + "\n",
	})
	c := mustFind(t, bom, aibom.KindCredential, "OPENAI_API_KEY")
	if len(c.Risks) != 1 || c.Risks[0].Where == nil || c.Risks[0].Where.Path != "z.env" || c.Risks[0].Where.Line != 2 {
		t.Fatalf("risks = %+v", c.Risks)
	}
	if !strings.Contains(aibom.Markdown(bom), "| z.env:2 (literal value assigned here (value not recorded)) |") {
		t.Errorf("the report does not point at z.env:2:\n%s", aibom.Markdown(bom))
	}
}
