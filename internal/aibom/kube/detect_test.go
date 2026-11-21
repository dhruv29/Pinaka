package kube

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// podJSON wraps a container (and optional pod-level fields) in a Pod.
func podJSON(containerJSON, podFields string) string {
	if podFields != "" {
		podFields = "," + podFields
	}
	return `{"kind":"Pod","apiVersion":"v1","metadata":{"name":"p","namespace":"n","labels":{"app":"x"}},"spec":{"containers":[` + containerJSON + `]` + podFields + `}}`
}

func imagePod(image string) string {
	return podJSON(fmt.Sprintf(`{"name":"c","image":%q}`, image), "")
}

func TestImagesAreMatchedByRepositoryNotSubstring(t *testing.T) {
	cases := []struct {
		image string
		kind  aibom.Kind
		name  string
	}{
		{"ghcr.io/huggingface/text-generation-inference:2.4.0", aibom.KindServing, "text-generation-inference"},
		{"ghcr.io/huggingface/text-embeddings-inference:1.5", aibom.KindServing, "text-embeddings-inference"},
		{"nvcr.io/nvidia/tritonserver:24.08-py3", aibom.KindServing, "triton"},
		{"nvcr.io/nim/meta/llama-3.1-8b-instruct:1.1.0", aibom.KindServing, "nvidia-nim"},
		{"seldonio/mlserver:1.6.0", aibom.KindServing, "seldon-mlserver"},
		{"bentoml/bento-server:1.2", aibom.KindServing, "bentoml"},
		{"pytorch/torchserve:0.11.0", aibom.KindServing, "torchserve"},
		{"tensorflow/serving:2.15.0", aibom.KindServing, "tensorflow-serving"},
		{"ghcr.io/ggml-org/llama.cpp:server", aibom.KindServing, "llama.cpp"},
		{"localai/localai:v2.20", aibom.KindServing, "localai"},
		{"ghcr.io/berriai/litellm:main-v1.50", aibom.KindServing, "litellm"},
		{"ghcr.io/open-webui/open-webui:main", aibom.KindServing, "open-webui"},
		{"xprobe/xinference:v0.16", aibom.KindServing, "xinference"},
		{"registry.corp.example/dockerhub/vllm/vllm-openai:v0.6.3", aibom.KindServing, "vllm"},
		{"ghcr.io/mlflow/mlflow:v2.17.0", aibom.KindPlatform, "mlflow"},
		{"kubeflownotebookswg/codeserver-python:v1.9", aibom.KindPlatform, "kubeflow"},
		{"kubeflownotebookswg/jupyter-scipy:v1.9", aibom.KindPlatform, "jupyter"},
		{"gcr.io/ml-pipeline/api-server:2.3.0", aibom.KindPlatform, "kubeflow-pipelines"},
		{"quay.io/jupyterhub/k8s-hub:3.3.8", aibom.KindPlatform, "jupyterhub"},
		{"wandb/local:0.60", aibom.KindPlatform, "wandb"},
		{"langfuse/langfuse:2", aibom.KindPlatform, "langfuse"},
		{"arizephoenix/phoenix:version-5", aibom.KindPlatform, "phoenix"},
		{"milvusdb/milvus:v2.4.13", aibom.KindDataStore, "milvus"},
		{"cr.weaviate.io/semitechnologies/weaviate:1.26.1", aibom.KindDataStore, "weaviate"},
		{"chromadb/chroma:0.5.5", aibom.KindDataStore, "chroma"},
		{"pgvector/pgvector:pg16", aibom.KindDataStore, "pgvector"},
		{"redis/redis-stack-server:7.4.0", aibom.KindDataStore, "redis-stack"},
		{"mcp/fetch:latest", aibom.KindAgentTool, "mcp/fetch"},
		{"ghcr.io/github/github-mcp-server:v0.2", aibom.KindAgentTool, "github/github-mcp-server"},
	}
	for _, tc := range cases {
		bom := mustParse(t, imagePod(tc.image))
		if _, ok := find(bom, tc.kind, tc.name); !ok {
			t.Errorf("%s: no %s %q in %+v", tc.image, tc.kind, tc.name, bom.Components)
		}
	}
	for _, image := range []string{"ghcr.io/example/ollama-exporter:1", "someone/vllm-proxy:1", "redis:7", "elasticsearch:8.15.0",
		"opensearchproject/opensearch:2", "apache/airflow:2.9.0", "nginx", "", "bad image name", "docker.io/"} {
		if bom := mustParse(t, imagePod(image)); len(bom.Components) != 0 {
			t.Errorf("%q matched: %+v", image, bom.Components)
		}
	}
}

func TestMirroredImagesAreMarkedAsLowerConfidence(t *testing.T) {
	bom := mustParse(t, imagePod("123.dkr.ecr.us-east-1.amazonaws.com/hub/ollama/ollama:0.3.12"))

	if c := mustFind(t, bom, aibom.KindServing, "ollama"); c.Properties["match_confidence"] != confidenceMirror {
		t.Fatalf("ollama = %+v", c)
	}
}

func TestNIMImagesNameTheirModel(t *testing.T) {
	bom := mustParse(t, imagePod("nvcr.io/nim/meta/llama-3.1-8b-instruct:1.1.0"))

	if m := mustFind(t, bom, aibom.KindModel, "meta/llama-3.1-8b-instruct"); m.Supplier != "meta" {
		t.Fatalf("model = %+v", m)
	}
}

func TestParseImageNormalisesReferences(t *testing.T) {
	cases := map[string]imageRef{
		"nginx":                             {Registry: "docker.io", Path: "library/nginx"},
		"index.docker.io/Ollama/Ollama:0.3": {Registry: "docker.io", Path: "ollama/ollama", Tag: "0.3"},
		"localhost:5000/x/y@sha256:ab":      {Registry: "localhost:5000", Path: "x/y", Digest: "sha256:ab"},
		"registry:5000/team/app:v1":         {Registry: "registry:5000", Path: "team/app", Tag: "v1"},
	}
	for in, want := range cases {
		if got, ok := parseImage(in); !ok || got != want {
			t.Errorf("parseImage(%q) = %+v, %v", in, got, ok)
		}
	}
}

func TestServersStartedByCommandAreFoundWithoutAKnownImage(t *testing.T) {
	cases := map[string]string{
		`["python3","-m","vllm.entrypoints.openai.api_server","--model","Qwen/Qwen2.5-7B"]`: "vllm",
		`["/usr/local/bin/text-generation-launcher"]`:                                       "text-generation-inference",
		`["python","-m","fastchat.serve.openai_api_server"]`:                                "fastchat",
		`["python","-m","sglang.launch_server"]`:                                            "sglang",
	}
	for cmd, name := range cases {
		bom := mustParse(t, podJSON(`{"name":"c","image":"corp/app:1","command":`+cmd+`}`, ""))
		mustFind(t, bom, aibom.KindServing, name)
	}
	if bom := mustParse(t, podJSON(`{"name":"c","image":"corp/app:1","command":["npm","run","serve"]}`, "")); len(bom.Components) != 0 {
		t.Fatalf("an ordinary command matched: %+v", bom.Components)
	}
}

func TestModelsAreReadFromFlagsCommandsEnvAndInitContainers(t *testing.T) {
	c := `{"name":"c","image":"corp/app:1","command":["sh","-c","vllm serve Qwen/Qwen2.5-7B --revision 0123abcd4567"],
	 "args":["--model-id=org/m1","--model","/models/local"],
	 "env":[{"name":"MODEL_NAME","value":"gpt-4o-mini"},{"name":"OLLAMA_MODEL","value":"phi3:mini"},{"name":"HF_MODEL_ID","value":"$(OTHER)"},{"name":"MODEL_ID","valueFrom":{"configMapKeyRef":{"name":"x","key":"y"}}}]}`
	init := `"initContainers":[{"name":"fetch","image":"kserve/storage-initializer:v0.14","args":["s3://user:pw@bucket/models/m?sig=SECRET","/mnt/models"]},
	 {"name":"dl","image":"python:3.12","command":["sh","-c","huggingface-cli download BAAI/bge-m3"]},
	 {"name":"git","image":"alpine/git","args":["clone","https://huggingface.co/google/gemma-2b/tree/main"]}]`
	bom := mustParse(t, podJSON(c, init))

	if m := mustFind(t, bom, aibom.KindModel, "Qwen/Qwen2.5-7B"); m.Version != "0123abcd4567" || len(m.Risks) != 0 {
		t.Fatalf("pinned revision not honoured: %+v", m)
	}
	mustRisk(t, mustFind(t, bom, aibom.KindModel, "BAAI/bge-m3"), riskRemoteDownload, aibom.SeverityLow)
	mustRisk(t, mustFind(t, bom, aibom.KindModel, "google/gemma-2b"), riskRemoteDownload, aibom.SeverityLow)
	if m := mustFind(t, bom, aibom.KindModel, "org/m1"); len(m.Risks) != 0 {
		t.Fatalf("a model named for an unknown app is not a download: %+v", m)
	}
	if m := mustFind(t, bom, aibom.KindModel, "gpt-4o-mini"); m.Supplier != "OpenAI" || m.Properties["source"] != "provider API" {
		t.Fatalf("model = %+v", m)
	}
	mustFind(t, bom, aibom.KindModel, "/models/local")
	if m := mustFind(t, bom, aibom.KindModel, "phi3"); m.Version != "mini" {
		t.Fatalf("model = %+v", m)
	}
	s3 := mustFind(t, bom, aibom.KindModel, "s3://bucket/models/m")
	if s3.Properties["source"] != "s3" {
		t.Fatalf("s3 model = %+v", s3)
	}
	for _, c := range bom.Components {
		if strings.Contains(c.Name, "OTHER") || strings.Contains(c.Name, "SECRET") || strings.Contains(c.Name, "pw@") {
			t.Fatalf("unsafe model name recorded: %q", c.Name)
		}
	}
}

func TestOfflineAndTrustRemoteCodeChangeTheDownloadRisk(t *testing.T) {
	offline := mustParse(t, podJSON(`{"name":"c","image":"vllm/vllm-openai:v1","args":["--model","a/b"],"env":[{"name":"HF_HUB_OFFLINE","value":"1"}]}`, ""))
	if m := mustFind(t, offline, aibom.KindModel, "a/b"); len(m.Risks) != 0 {
		t.Fatalf("offline model flagged: %+v", m)
	}
	trusted := mustParse(t, podJSON(`{"name":"c","image":"vllm/vllm-openai:v1","args":["--model","a/b","--trust-remote-code"]}`, ""))
	mustRisk(t, mustFind(t, trusted, aibom.KindModel, "a/b"), riskRemoteDownload, aibom.SeverityMedium)
}

func TestAcceleratorsAreFoundFromResourcesAndPlacement(t *testing.T) {
	c := `{"name":"c","image":"corp/train:1","resources":{"limits":{"amd.com/gpu":"2","habana.ai/gaudi":8,"aws.amazon.com/neuroncore":"x","cpu":"4"}}}`
	bom := mustParse(t, podJSON(c, `"tolerations":[{"key":"nvidia.com/gpu","operator":"Exists"}]`))

	for _, name := range []string{"amd.com/gpu", "habana.ai/gaudi", "aws.amazon.com/neuroncore", "nvidia.com/gpu"} {
		mustFind(t, bom, aibom.KindAccelerator, name)
	}
	if g := mustFind(t, bom, aibom.KindAccelerator, "habana.ai/gaudi"); !strings.HasSuffix(g.Evidence[0].Detail, "=8") {
		t.Fatalf("count missing: %+v", g.Evidence)
	}
	if bom := mustParse(t, podJSON(`{"name":"c","image":"corp/app:1","resources":{"limits":{"cpu":"1","memory":"1Gi"}}}`, "")); len(bom.Components) != 0 {
		t.Fatalf("cpu and memory are not accelerators: %+v", bom.Components)
	}
}

func TestGPUOdditiesAreInformational(t *testing.T) {
	noLimit := mustParse(t, podJSON(`{"name":"c","image":"corp/a:1","resources":{"requests":{"nvidia.com/gpu.shared":"1"}}}`, ""))
	g := mustFind(t, noLimit, aibom.KindAccelerator, "nvidia.com/gpu.shared")
	mustRisk(t, g, riskGPUWithoutLimits, aibom.SeverityInfo)
	mustRisk(t, g, riskSharedGPU, aibom.SeverityInfo)

	visible := mustParse(t, podJSON(`{"name":"c","image":"corp/a:1","env":[{"name":"NVIDIA_VISIBLE_DEVICES","value":"all"}]}`, `"nodeSelector":{"cloud.google.com/gke-accelerator":"nvidia-l4"}`))
	r := mustRisk(t, mustFind(t, visible, aibom.KindAccelerator, "nvidia.com/gpu"), riskGPUWithoutLimits, aibom.SeverityInfo)
	if !strings.Contains(r.Summary, "NVIDIA_VISIBLE_DEVICES=all") || !strings.Contains(r.Summary, "GPU nodes") {
		t.Fatalf("summary = %q", r.Summary)
	}
}

func TestProvidersAreFoundFromBaseURLsButNotOtherURLs(t *testing.T) {
	c := `{"name":"c","image":"corp/app:1","env":[{"name":"OPENAI_BASE_URL","value":"https://user:pw@my-res.openai.azure.com/openai?key=1"},
	 {"name":"VERTEX_ENDPOINT","value":"https://us-central1-aiplatform.googleapis.com"},{"name":"BACKEND_URL","value":"https://api.corp.example"},
	 {"name":"NOTES","value":"https://api.anthropic.com"},{"name":"AZURE_OPENAI_API_KEY","valueFrom":{"secretKeyRef":{"name":"az","key":"k"}}},
	 {"name":"AZURE_OPENAI_DEPLOYMENT","value":"gpt4"},{"name":"bad name!","value":"x"},{"name":"COHERE_API_KEY","value":""}]}`
	bom := mustParse(t, podJSON(c, ""))

	if p := mustFind(t, bom, aibom.KindProvider, "Azure OpenAI"); p.Properties["host"] != "my-res.openai.azure.com" {
		t.Fatalf("provider = %+v", p)
	}
	mustFind(t, bom, aibom.KindProvider, "Google Vertex AI")
	if _, ok := find(bom, aibom.KindProvider, "Anthropic API"); ok {
		t.Fatal("a non-endpoint variable was read as a provider URL")
	}
	if _, ok := find(bom, aibom.KindCredential, "AZURE_OPENAI_DEPLOYMENT"); ok {
		t.Fatal("a non-credential Azure setting was called a credential")
	}
	if c := mustFind(t, bom, aibom.KindCredential, "COHERE_API_KEY"); len(c.Risks) != 0 || c.Properties["reference"] != "empty value" {
		t.Fatalf("empty credential = %+v", c)
	}
	mustFind(t, bom, aibom.KindCredential, "AZURE_OPENAI_API_KEY")
}

func TestAgentToolsAreFoundFromPackagesButNotOrdinaryArgs(t *testing.T) {
	bom := mustParse(t, podJSON(`{"name":"c","image":"node:22","command":["npx","-y","@modelcontextprotocol/server-github@1.0.0"]}`, ""))
	mustFind(t, bom, aibom.KindAgentTool, "@modelcontextprotocol/server-github")

	if bom := mustParse(t, podJSON(`{"name":"c","image":"node:22","command":["npx","-y","serve"]}`, "")); len(bom.Components) != 0 {
		t.Fatalf("matched: %+v", bom.Components)
	}
}

func TestRayAndKubeflowCustomResourcesAreRecognisedOnlyInTheirAPIGroup(t *testing.T) {
	rayService := `{"kind":"RayService","apiVersion":"ray.io/v1","metadata":{"name":"rs","namespace":"r"},"spec":{"rayClusterConfig":{"headGroupSpec":{"template":{"spec":{"containers":[{"name":"head","image":"corp/ray:1"}]}}},"workerGroupSpecs":[{"template":{"spec":{"containers":[{"name":"w","image":"corp/ray:1","resources":{"limits":{"nvidia.com/gpu":"1"}}}]}}}]}}}`
	notebook := `{"kind":"Notebook","apiVersion":"kubeflow.org/v1","metadata":{"name":"nb","namespace":"k"},"spec":{"template":{"spec":{"containers":[{"name":"nb","image":"corp/nb:1"}]}}}}`
	bom := mustParse(t, rayService+notebook)

	mustFind(t, bom, aibom.KindServing, "ray-serve")
	mustFind(t, bom, aibom.KindPlatform, "ray")
	mustFind(t, bom, aibom.KindPlatform, "kubeflow")
	mustFind(t, bom, aibom.KindAccelerator, "nvidia.com/gpu")

	other := `{"kind":"Notebook","apiVersion":"example.com/v1","metadata":{"name":"nb","namespace":"k"},"spec":{}}`
	if bom := mustParse(t, other); len(bom.Components) != 0 {
		t.Fatalf("foreign Notebook kind matched: %+v", bom.Components)
	}
}
