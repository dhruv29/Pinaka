package repo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// Fake keys are assembled at run time so no key-shaped literal sits here.
var (
	fakeOpenAIKey = "sk-" + "proj-" + strings.Repeat("Zq8Lw3Rt", 5)
	fakeHFToken   = "hf_" + strings.Repeat("Ab9Cd8Ef7", 3)
)

func TestScanListsAILibrariesFromEveryManifestKind(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"requirements.txt":     "# deps\nopenai==1.2.3\nrequests==2.31\ntorch>=2.0\n-r other.txt\nlangchain-openai[x]~=0.1 ; python_version>'3'\n",
		"svc/pyproject.toml":   "[project]\ndependencies = [\n  \"anthropic>=0.30,<1\",\n  \"transformers[torch]\",\n]\n[project.optional-dependencies]\nvec = [\"chromadb==0.5\"]\n[tool.poetry.dependencies]\npython = \"^3.11\"\ncohere = { version = \"^5.0\" }\n",
		"p/Pipfile":            "[packages]\nmistralai = \"*\"\nflask = \"*\"\n",
		"s/setup.py":           "setup(\n  name=\"openai\",\n  install_requires=[\n    \"groq==0.9\",\n  ],\n)\n",
		"c/setup.cfg":          "[options]\ninstall_requires =\n    litellm==1.40\n[options.extras_require]\nml = xgboost==2.0\n",
		"poetry.lock":          "[[package]]\nname = \"requests\"\nversion = \"2.31\"\n\n[[package]]\nname = \"Sentence_Transformers\"\nversion = \"3.0.1\"\n",
		"u/uv.lock":            "[[package]]\nname = \"vllm\"\nversion = \"0.5.0\"\ndependencies = [\n { name = \"torch\" },\n]\n",
		"package.json":         `{"dependencies": {"@anthropic-ai/sdk": "^0.20.0", "express": "4"}, "devDependencies": {"@langchain/core": "latest", "ai": "git+https://tok@github.com/x/y"}}`,
		"go.mod":               "module x\n\nrequire github.com/tmc/langchaingo v0.1.0\nrequire (\n\tgithub.com/anthropics/anthropic-sdk-go v1.2.0 // indirect\n\tgithub.com/spf13/cobra v1.8.0\n)\n",
		"Cargo.toml":           "[dependencies]\nasync-openai = \"0.20\"\nserde = \"1\"\nmycandle = { package = \"candle-core\", version = \"0.6\" }\n[dependencies.tch]\nversion = \"0.15\"\n[target.'cfg(unix)'.dependencies]\nort = { git = \"https://x\" }\n",
		"pom.xml":              "<dependency>\n<groupId>dev.langchain4j</groupId>\n<artifactId>langchain4j</artifactId>\n<version>0.31.0</version>\n<exclusions><exclusion>\n<groupId>org.x</groupId><artifactId>y</artifactId>\n</exclusion></exclusions>\n</dependency>\n<dependency><groupId>junit</groupId>\n<artifactId>junit</artifactId>\n</dependency>\n",
		"app/build.gradle.kts": "dependencies {\n  implementation(\"com.microsoft.onnxruntime:onnxruntime:1.18.0\")\n  implementation(\"com.google.guava:guava:33.0\")\n  implementation(\"ai.djl:api:+\")\n}\n",
	})
	want := []struct{ name, version, eco string }{
		{"openai", "1.2.3", ecoPyPI}, {"torch", ">=2.0", ecoPyPI}, {"langchain-openai", "~=0.1", ecoPyPI},
		{"anthropic", ">=0.30,<1", ecoPyPI}, {"transformers", "", ecoPyPI}, {"chromadb", "0.5", ecoPyPI},
		{"cohere", "^5.0", ecoPyPI}, {"mistralai", "*", ecoPyPI}, {"groq", "0.9", ecoPyPI},
		{"litellm", "1.40", ecoPyPI}, {"xgboost", "2.0", ecoPyPI}, {"sentence-transformers", "3.0.1", ecoPyPI},
		{"vllm", "0.5.0", ecoPyPI}, {"@anthropic-ai/sdk", "^0.20.0", ecoNPM}, {"@langchain/core", "latest", ecoNPM},
		{"ai", directReference, ecoNPM}, {"github.com/tmc/langchaingo", "v0.1.0", ecoGo},
		{"github.com/anthropics/anthropic-sdk-go", "v1.2.0", ecoGo}, {"async-openai", "0.20", ecoCargo},
		{"candle-core", "0.6", ecoCargo}, {"tch", "0.15", ecoCargo}, {"ort", "", ecoCargo},
		{"dev.langchain4j:langchain4j", "0.31.0", ecoMaven}, {"com.microsoft.onnxruntime:onnxruntime", "1.18.0", ecoMaven},
		{"ai.djl:api", "+", ecoMaven},
	}
	for _, w := range want {
		c := find(bom, aibom.KindLibrary, w.name)
		if c == nil || c.Version != w.version || c.Ecosystem != w.eco {
			t.Errorf("library %s: got %+v, want version %q eco %q", w.name, c, w.version, w.eco)
		}
	}
	for _, absent := range []string{"requests", "flask", "express", "github.com/spf13/cobra", "serde", "junit:junit", "org.x:y", "com.google.guava:guava"} {
		mustNotFind(t, bom, aibom.KindLibrary, absent)
	}
	if find(bom, aibom.KindLibrary, "openai").Version == "" || find(bom, aibom.KindLibrary, "openai").Supplier != "OpenAI" {
		t.Error("openai lost its version or supplier")
	}
	unpinned := map[string]bool{"torch": true, "transformers": true, "mistralai": true, "@langchain/core": true, "ort": true, "ai.djl:api": true}
	for _, c := range bom.Components {
		if got := riskSeverity(c, "unpinned-library") != ""; got != unpinned[c.Name] {
			t.Errorf("%s unpinned-library = %v, want %v", c.Name, got, unpinned[c.Name])
		}
	}
	if strings.Contains(marshal(t, bom), "tok@github") {
		t.Error("a dependency URL was recorded")
	}
}

func TestScanListsModelFilesWithFormatSizeAndPickleRisk(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"m/model.gguf":         "GGUF....",
		"m/model.safetensors":  "{}",
		"m/clf.pkl":            "\x80\x04",
		"m/net.pt":             "PK",
		"hf/pytorch_model.bin": "x",
		"hf/config.json":       "{}",
		"data/blob.bin":        "x",
		"lfs/big.onnx":         lfsPointerPrefix + "\noid sha256:abc\nsize 123456\n",
		"tf/saved_model.pb":    "x",
		"proto/messages.pb":    "x",
	})
	gguf := mustFind(t, bom, aibom.KindModel, "m/model.gguf")
	if gguf.Properties["format"] != "gguf" || gguf.Properties["size"] != "8" || len(gguf.Risks) != 0 {
		t.Errorf("gguf = %+v", gguf)
	}
	if riskSeverity(mustFind(t, bom, aibom.KindModel, "m/clf.pkl"), "pickle-model-file") != aibom.SeverityHigh {
		t.Error("raw pickle is not high")
	}
	if riskSeverity(mustFind(t, bom, aibom.KindModel, "m/net.pt"), "pickle-model-file") != aibom.SeverityMedium {
		t.Error("torch checkpoint is not medium")
	}
	mustFind(t, bom, aibom.KindModel, "hf/pytorch_model.bin")
	mustFind(t, bom, aibom.KindModel, "tf/saved_model.pb")
	mustFind(t, bom, aibom.KindModel, "m/model.safetensors")
	mustNotFind(t, bom, aibom.KindModel, "data/blob.bin")
	mustNotFind(t, bom, aibom.KindModel, "proto/messages.pb")
	lfs := mustFind(t, bom, aibom.KindModel, "lfs/big.onnx")
	if lfs.Properties["storage"] != "git-lfs-pointer" || lfs.Properties["size"] != "123456" {
		t.Errorf("lfs = %+v", lfs.Properties)
	}
}

func TestScanFindsHostedModelNamesOnlyAsWholeLiterals(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"app/llm.py": `a = "gpt-4o"
b = 'openai/gpt-4o-mini'
c = "anthropic.claude-3-5-sonnet-20240620-v1:0"
d = "gemini-1.5-pro-latest"
e = "pseudo3"
f = "command"
g = "claude-code"
h = "o3-mini"
# we tried gpt-4o once
i = "mixtral-8x7b"
j = "command-r-plus"
k = "deepseek-r1:7b"
l = "grok-2"
`,
		"README.md":         "Use `claude-sonnet-4-5` for best results.\n",
		"tests/test_llm.py": `M = "qwen2.5-coder"`,
	})
	for _, name := range []string{"gpt-4o", "gpt-4o-mini", "anthropic.claude-3-5-sonnet-20240620-v1:0", "o3-mini", "mixtral-8x7b", "command-r-plus", "deepseek-r1:7b", "grok-2"} {
		if c := mustFind(t, bom, aibom.KindModel, name); c.Properties[propContext] != contextCode {
			t.Errorf("%s context = %q", name, c.Properties[propContext])
		}
	}
	if c := mustFind(t, bom, aibom.KindModel, "gemini-1.5-pro-latest"); riskSeverity(c, "unpinned-model") != aibom.SeverityLow {
		t.Error("-latest alias is not flagged")
	}
	if c := mustFind(t, bom, aibom.KindModel, "claude-sonnet-4-5"); c.Properties[propContext] != contextDocs || c.Supplier != "Anthropic" {
		t.Errorf("docs model = %+v", c)
	}
	if c := mustFind(t, bom, aibom.KindModel, "qwen2.5-coder"); c.Properties[propContext] != contextTest {
		t.Errorf("test model = %+v", c)
	}
	for _, absent := range []string{"pseudo3", "command", "claude-code"} {
		mustNotFind(t, bom, aibom.KindModel, absent)
	}
	if ev := mustFind(t, bom, aibom.KindModel, "gpt-4o").Evidence; len(ev) != 1 || ev[0].Line != 1 {
		t.Errorf("gpt-4o evidence = %+v (the comment must not count)", ev)
	}
}

func TestScanFindsHuggingFaceModelsAndDatasetsWithRevisionAndRisks(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"local/bert/config.json": "{}",
		"train.py": `tok = AutoTokenizer.from_pretrained("bert-base-uncased", revision="a1b2c3")
model = AutoModel.from_pretrained(
    "org/remote-model",
    trust_remote_code=True,
)
loc = AutoModel.from_pretrained("local/bert")
dyn = AutoModel.from_pretrained(name, trust_remote_code=True)
skip = AutoModel.from_pretrained(path)
p = pipeline("text-generation", model="meta-llama/Llama-3.1-8B")
f = hf_hub_download(repo_id="TheBloke/x-GGUF", filename="a.gguf", revision="main")
st = SentenceTransformer("all-MiniLM-L6-v2")
ds = load_dataset("squad")
js = load_dataset("json", data_files="data/train.jsonl")
opts = {"trust_remote_code": True}
`,
	})
	bert := mustFind(t, bom, aibom.KindModel, "bert-base-uncased")
	if bert.Version != "a1b2c3" || bert.Ecosystem != ecoHuggingFace || len(bert.Risks) != 0 {
		t.Errorf("bert = %+v", bert)
	}
	remote := mustFind(t, bom, aibom.KindModel, "org/remote-model")
	if riskSeverity(remote, "remote-code-model") != aibom.SeverityHigh || riskSeverity(remote, "unpinned-model") == "" || remote.Supplier != "org" {
		t.Errorf("remote = %+v", remote)
	}
	if local := mustFind(t, bom, aibom.KindModel, "local/bert"); len(local.Risks) != 0 || local.Properties["reference"] != "local" {
		t.Errorf("local = %+v", local)
	}
	dyn := mustFind(t, bom, aibom.KindModel, "unresolved model (train.py)")
	if riskSeverity(dyn, "remote-code-model") == "" || len(dyn.Evidence) != 2 {
		t.Errorf("dynamic = %+v", dyn)
	}
	mustFind(t, bom, aibom.KindModel, "meta-llama/Llama-3.1-8B")
	if c := mustFind(t, bom, aibom.KindModel, "TheBloke/x-GGUF"); riskSeverity(c, "unpinned-model") == "" {
		t.Error("revision=main counts as pinned")
	}
	mustFind(t, bom, aibom.KindModel, "all-MiniLM-L6-v2")
	mustFind(t, bom, aibom.KindDataset, "squad")
	mustFind(t, bom, aibom.KindDataset, "data/train.jsonl")
	mustNotFind(t, bom, aibom.KindDataset, "json")
}

func TestScanFindsOllamaModelsAndModelfileBases(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"config.yaml": "model: \"llama3.1:8b\"\nother: \"openai:gpt-4o\"\nagent:\n  model: 'openai:gpt-4.1'\n",
		"Modelfile":   "FROM llama3\nSYSTEM hi\n",
		"x.modelfile": "FROM ./weights/q4.gguf\n",
	})
	if c := mustFind(t, bom, aibom.KindModel, "llama3.1:8b"); c.Ecosystem != ecoOllama || c.Supplier != "Meta" {
		t.Errorf("ollama = %+v", c)
	}
	mustFind(t, bom, aibom.KindModel, "gpt-4.1")
	mustNotFind(t, bom, aibom.KindModel, "openai:gpt-4.1")
	if c := mustFind(t, bom, aibom.KindModel, "llama3"); riskSeverity(c, "unpinned-model") == "" {
		t.Error("untagged Modelfile base is not unpinned")
	}
	mustFind(t, bom, aibom.KindModel, "./weights/q4.gguf")
}

func TestScanFindsProvidersAndCustomEndpoints(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"client.ts": `const a = "https://api.openai.com/v1";
const b = "https://myres.openai.azure.com/";
const c = "https://bedrock-runtime.us-east-1.amazonaws.com";
const d = "https://api.openai.com.evil.example/";
const e = "https://myapi.openai.com/";
const o = new OpenAI({ baseURL: "https://user:pw@llm.proxy.example/v1?k=1" });
const l = new OpenAI({ baseURL: "http://localhost:8000/v1" });
const v = new OpenAI({ baseURL: "https://api.anthropic.com" });
const t = new OpenAI({ baseURL: "${BASE}" });
fetch("http://localhost:11434/api/chat")
`,
	})
	openai := mustFind(t, bom, aibom.KindProvider, "OpenAI API")
	if len(openai.Evidence) != 1 || openai.Evidence[0].Line != 1 {
		t.Errorf("openai evidence = %+v (look-alike hosts must not count)", openai.Evidence)
	}
	mustFind(t, bom, aibom.KindProvider, "Azure OpenAI")
	mustFind(t, bom, aibom.KindProvider, "Amazon Bedrock")
	mustFind(t, bom, aibom.KindProvider, "Ollama (local)")
	mustFind(t, bom, aibom.KindProvider, "Anthropic API")
	if c := mustFind(t, bom, aibom.KindProvider, "custom endpoint llm.proxy.example"); riskSeverity(c, "custom-endpoint") != aibom.SeverityLow {
		t.Errorf("custom = %+v", c)
	}
	if c := mustFind(t, bom, aibom.KindProvider, "custom endpoint localhost:8000"); riskSeverity(c, "custom-endpoint") != aibom.SeverityInfo {
		t.Errorf("local = %+v", c)
	}
	mustNotFind(t, bom, aibom.KindProvider, "custom endpoint api.anthropic.com")
	if out := marshal(t, bom); strings.Contains(out, "pw@") || strings.Contains(out, "k=1") {
		t.Error("URL credentials or query leaked")
	}
}

func TestScanRecordsCredentialNamesButNeverTheirValues(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"app.py":                   "key = os.environ[\"OPENAI_API_KEY\"]\nOPENAI_API_KEY = os.getenv(\"X\")\n",
		"settings.py":              "OPENAI_API_KEY = \"" + fakeOpenAIKey + "\"\n",
		".env.example":             "ANTHROPIC_API_KEY=your-key-here\nHF_TOKEN=\nAZURE_OPENAI_API_KEY=${AZURE}\n",
		"compose.yaml":             "environment:\n  HF_TOKEN: " + fakeHFToken + "\n  GROQ_API_KEY: changeme-placeholder-value\n",
		".github/workflows/ci.yml": "env:\n  GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}\n",
		"other.py":                 "MY_OPENAI_API_KEYS = 1\nif OPENAI_API_KEY == \"" + fakeOpenAIKey + "x\": pass\n",
	})
	openai := mustFind(t, bom, aibom.KindCredential, "OPENAI_API_KEY")
	if riskSeverity(openai, "hardcoded-ai-credential") != aibom.SeverityHigh {
		t.Errorf("openai = %+v", openai)
	}
	if riskSeverity(mustFind(t, bom, aibom.KindCredential, "HF_TOKEN"), "hardcoded-ai-credential") == "" {
		t.Error("yaml literal not flagged")
	}
	for _, name := range []string{"ANTHROPIC_API_KEY", "AZURE_OPENAI_API_KEY", "GROQ_API_KEY", "GEMINI_API_KEY"} {
		if c := mustFind(t, bom, aibom.KindCredential, name); len(c.Risks) != 0 {
			t.Errorf("%s flagged: %+v", name, c.Risks)
		}
	}
	out := marshal(t, bom)
	if strings.Contains(out, fakeOpenAIKey) || strings.Contains(out, fakeHFToken) || strings.Contains(out, "Zq8Lw3Rt") {
		t.Error("a credential value appears in the output")
	}
}

func TestScanFlagsPickleBasedModelLoads(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"load.py": `a = torch.load("ckpt/model.pt")
b = torch.load("ckpt/safe.pt", weights_only=True)
c = pickle.load(open("clf.pkl", "rb"))
d = joblib.load(model_path)
e = pickle.load(cache_file)
`,
	})
	if c := mustFind(t, bom, aibom.KindModel, "ckpt/model.pt"); riskSeverity(c, "insecure-deserialisation") != aibom.SeverityMedium {
		t.Errorf("torch.load = %+v", c)
	}
	mustNotFind(t, bom, aibom.KindModel, "ckpt/safe.pt")
	mustFind(t, bom, aibom.KindModel, "clf.pkl")
	if c := mustFind(t, bom, aibom.KindModel, "unresolved model (load.py)"); len(c.Evidence) != 1 || c.Evidence[0].Line != 4 {
		t.Errorf("joblib = %+v (unrelated pickle.load must not count)", c.Evidence)
	}
}

func TestScanReadsNotebookCodeCellsOnly(t *testing.T) {
	nb := `{
 "cells": [
  {"cell_type": "markdown", "source": ["We compare \"gpt-4o\" here"]},
  {"cell_type": "code", "source": [
    "import os\n",
    "m = AutoModel.from_pretrained(\"org/nb-model\", revision=\"v1\")"
   ],
   "outputs": [{"text": "OPENAI_API_KEY=` + fakeOpenAIKey + `"}]},
  {"cell_type": "code", "source": "x = \"claude-3-haiku\""}
 ]
}`
	bom := scanFiles(t, map[string]string{"analysis.ipynb": nb, "broken.ipynb": "{"})
	if c := mustFind(t, bom, aibom.KindModel, "org/nb-model"); c.Evidence[0].Line != 6 {
		t.Errorf("notebook line = %d, want 6", c.Evidence[0].Line)
	}
	mustFind(t, bom, aibom.KindModel, "claude-3-haiku")
	mustNotFind(t, bom, aibom.KindModel, "gpt-4o")
	mustNotFind(t, bom, aibom.KindCredential, "OPENAI_API_KEY")
	if !hasNote(bom, "could not be parsed as JSON: broken.ipynb") {
		t.Errorf("notes = %v", bom.Notes)
	}
}

func TestScanIgnoresBaseURLsInFilesThatAreNotAboutAI(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"http.go":  `c := Client{BaseURL: "https://api.shop.example"}`,
		"env.yaml": "OPENAI_BASE_URL: https://gateway.example.org/v1\n",
	})
	mustNotFind(t, bom, aibom.KindProvider, "custom endpoint api.shop.example")
	mustFind(t, bom, aibom.KindProvider, "custom endpoint gateway.example.org")
}

func TestScanIgnoresABaseURLFarFromAnyAIClient(t *testing.T) {
	filler := strings.Repeat("# unrelated setting\n", 80)
	bom := scanFiles(t, map[string]string{
		"settings.py": "import anthropic\n" + filler + "payments = Client(base_url=\"https://pay.partner.example/v1\")\n",
	})

	mustNotFind(t, bom, aibom.KindProvider, "custom endpoint pay.partner.example")
}

func TestScanIgnoresCustomEndpointsInTestsAndDocuments(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"client_test.py": "client = OpenAI(base_url=\"https://fake.test/v1\")\n",
		"README.md":      "Use `OpenAI(base_url=\"https://docs.example/v1\")` to try it.\n",
		"client.py":      "client = OpenAI(base_url=\"https://gateway.partner.example/v1\")\n",
	})

	mustNotFind(t, bom, aibom.KindProvider, "custom endpoint fake.test")
	mustNotFind(t, bom, aibom.KindProvider, "custom endpoint docs.example")
	mustFind(t, bom, aibom.KindProvider, "custom endpoint gateway.partner.example")
}

func TestScanCountsATwoCharacterModelNameOnlyBesideItsVendor(t *testing.T) {
	plain := scanFiles(t, map[string]string{"labels.py": "ORDER = [\"o1\", \"o2\"]\n"})
	mustNotFind(t, plain, aibom.KindModel, "o1")

	real := scanFiles(t, map[string]string{"llm.py": "from openai import OpenAI\nMODEL = \"o1\"\n"})
	mustFind(t, real, aibom.KindModel, "o1")
}

func TestScanLeavesOutWhatTheOperatorExcludesAndSaysSo(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"app.py":              "MODEL = \"claude-sonnet-4-5\"\n",
		"fixtures/sample.py":  "MODEL = \"gpt-4o\"\n",
		"docs/demo.js":        "const m = \"gemini-1.5-pro\";\n",
		"docs/guide/setup.py": "MODEL = \"mistral-large-latest\"\n",
	} {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	bom := mustScan(t, root, Options{Exclude: []string{"fixtures", "docs/*.js"}})

	mustFind(t, bom, aibom.KindModel, "claude-sonnet-4-5")
	mustFind(t, bom, aibom.KindModel, "mistral-large-latest")
	mustNotFind(t, bom, aibom.KindModel, "gpt-4o")
	mustNotFind(t, bom, aibom.KindModel, "gemini-1.5-pro")
	if !hasNote(bom, "excluded on request") {
		t.Fatalf("notes = %v; want the exclusions listed", bom.Notes)
	}
}

func TestScanRefusesAMalformedExcludePattern(t *testing.T) {
	if _, err := Scan(context.Background(), t.TempDir(), Options{Exclude: []string{"[unclosed"}}); err == nil {
		t.Fatal("a malformed pattern was accepted")
	}
}
