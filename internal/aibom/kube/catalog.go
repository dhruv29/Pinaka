package kube

import "github.com/dhruv29/Pinaka/internal/aibom"

// product is one kind of AI software that can be recognised in a cluster.
// The table below is the whole detection catalogue for images, commands and
// custom resources; extend it here rather than adding code.
type product struct {
	Name     string
	Kind     aibom.Kind
	Supplier string
	// Open is true when the software answers requests without
	// authentication unless an operator turns it on, so exposing it is worse.
	Open bool
	// FetchesModels is true when, given a model id, it pulls weights from
	// Hugging Face at start.
	FetchesModels bool
	// ModelInImage is true when the image path after the prefix names the
	// model (NVIDIA NIM images are one per model).
	ModelInImage bool
	// NameFromImage uses the image path as the component name, for families
	// such as the Docker MCP catalogue where each image is a different tool.
	NameFromImage bool
	// Images are exact repositories, registry included (docker.io for Hub).
	Images []string
	// Prefixes match the start of a repository; the longest prefix wins.
	Prefixes []string
	// Commands are token sequences in command+args; the first token is
	// compared by its base name.
	Commands [][]string
	// Kinds are custom resource kinds that imply the product.
	Kinds []string
}

var products = []product{
	// Model serving.
	{Name: "vllm", Kind: aibom.KindServing, Supplier: "vLLM project", Open: true, FetchesModels: true,
		Images:   []string{"docker.io/vllm/vllm-openai", "docker.io/rocm/vllm"},
		Commands: [][]string{{"vllm", "serve"}, {"-m", "vllm.entrypoints.openai.api_server"}}},
	{Name: "ollama", Kind: aibom.KindServing, Supplier: "Ollama", Open: true,
		Images: []string{"docker.io/ollama/ollama"}, Commands: [][]string{{"ollama", "serve"}}},
	{Name: "text-generation-inference", Kind: aibom.KindServing, Supplier: "Hugging Face", Open: true, FetchesModels: true,
		Images: []string{"ghcr.io/huggingface/text-generation-inference"}, Commands: [][]string{{"text-generation-launcher"}}},
	{Name: "text-embeddings-inference", Kind: aibom.KindServing, Supplier: "Hugging Face", Open: true, FetchesModels: true,
		Images: []string{"ghcr.io/huggingface/text-embeddings-inference"}, Commands: [][]string{{"text-embeddings-router"}}},
	{Name: "triton", Kind: aibom.KindServing, Supplier: "NVIDIA", Open: true,
		Images: []string{"nvcr.io/nvidia/tritonserver"}, Commands: [][]string{{"tritonserver"}}},
	{Name: "nvidia-nim", Kind: aibom.KindServing, Supplier: "NVIDIA", Open: true, ModelInImage: true,
		Prefixes: []string{"nvcr.io/nim/"}},
	{Name: "kserve", Kind: aibom.KindServing, Supplier: "KServe", FetchesModels: true, Kinds: []string{"InferenceService"},
		Images: []string{"docker.io/kserve/sklearnserver", "docker.io/kserve/xgbserver", "docker.io/kserve/lgbserver",
			"docker.io/kserve/pmmlserver", "docker.io/kserve/paddleserver", "docker.io/kserve/huggingfaceserver"}},
	{Name: "seldon-mlserver", Kind: aibom.KindServing, Supplier: "Seldon", Open: true,
		Images: []string{"docker.io/seldonio/mlserver"}, Commands: [][]string{{"mlserver", "start"}}},
	{Name: "bentoml", Kind: aibom.KindServing, Supplier: "BentoML", Open: true,
		Prefixes: []string{"docker.io/bentoml/"}, Commands: [][]string{{"bentoml", "serve"}}},
	{Name: "torchserve", Kind: aibom.KindServing, Supplier: "PyTorch",
		Images: []string{"docker.io/pytorch/torchserve", "docker.io/pytorch/torchserve-kfs"}, Commands: [][]string{{"torchserve"}}},
	{Name: "tensorflow-serving", Kind: aibom.KindServing, Supplier: "Google", Open: true,
		Images: []string{"docker.io/tensorflow/serving"}, Commands: [][]string{{"tensorflow_model_server"}}},
	{Name: "llama.cpp", Kind: aibom.KindServing, Supplier: "ggml.ai", Open: true,
		Images: []string{"ghcr.io/ggerganov/llama.cpp", "ghcr.io/ggml-org/llama.cpp"}, Commands: [][]string{{"llama-server"}}},
	{Name: "localai", Kind: aibom.KindServing, Supplier: "LocalAI", Open: true,
		Images: []string{"docker.io/localai/localai", "quay.io/go-skynet/local-ai"}, Commands: [][]string{{"local-ai"}}},
	{Name: "litellm", Kind: aibom.KindServing, Supplier: "BerriAI", Open: true,
		Images: []string{"ghcr.io/berriai/litellm", "ghcr.io/berriai/litellm-database", "ghcr.io/berriai/litellm-non_root"}},
	{Name: "open-webui", Kind: aibom.KindServing, Supplier: "Open WebUI", Images: []string{"ghcr.io/open-webui/open-webui"}},
	{Name: "ray-serve", Kind: aibom.KindServing, Supplier: "Anyscale", Open: true, Kinds: []string{"RayService"}},
	{Name: "xinference", Kind: aibom.KindServing, Supplier: "Xorbits", Open: true,
		Images: []string{"docker.io/xprobe/xinference"}, Commands: [][]string{{"xinference-local"}}},
	{Name: "fastchat", Kind: aibom.KindServing, Supplier: "LMSYS", Open: true,
		Commands: [][]string{{"-m", "fastchat.serve.openai_api_server"}, {"-m", "fastchat.serve.model_worker"}}},
	{Name: "sglang", Kind: aibom.KindServing, Supplier: "SGLang project", Open: true, FetchesModels: true,
		Images: []string{"docker.io/lmsysorg/sglang"}, Commands: [][]string{{"-m", "sglang.launch_server"}}},

	// Platforms.
	{Name: "kubeflow", Kind: aibom.KindPlatform, Supplier: "Kubeflow", Kinds: []string{"Notebook"},
		Prefixes: []string{"ghcr.io/kubeflow/", "docker.io/kubeflownotebookswg/"}},
	{Name: "kubeflow-pipelines", Kind: aibom.KindPlatform, Supplier: "Kubeflow", Prefixes: []string{"gcr.io/ml-pipeline/"}},
	{Name: "mlflow", Kind: aibom.KindPlatform, Supplier: "MLflow", Open: true,
		Images:   []string{"ghcr.io/mlflow/mlflow", "docker.io/bitnami/mlflow"},
		Commands: [][]string{{"mlflow", "server"}, {"mlflow", "ui"}}},
	{Name: "jupyter", Kind: aibom.KindPlatform, Supplier: "Project Jupyter",
		Prefixes: []string{"docker.io/jupyter/", "quay.io/jupyter/", "docker.io/kubeflownotebookswg/jupyter"},
		Images:   []string{"quay.io/jupyterhub/k8s-singleuser-sample", "docker.io/jupyterhub/k8s-singleuser-sample", "docker.io/jupyterhub/singleuser"},
		Commands: [][]string{{"jupyter", "lab"}, {"jupyter", "notebook"}, {"jupyter-lab"}, {"start-notebook.sh"}, {"start-notebook.py"}, {"jupyterhub-singleuser"}}},
	{Name: "jupyterhub", Kind: aibom.KindPlatform, Supplier: "Project Jupyter",
		Images: []string{"quay.io/jupyterhub/k8s-hub", "docker.io/jupyterhub/k8s-hub"}, Commands: [][]string{{"jupyterhub"}}},
	{Name: "ray", Kind: aibom.KindPlatform, Supplier: "Anyscale", Open: true, Kinds: []string{"RayCluster", "RayService"},
		Images:   []string{"docker.io/rayproject/ray", "docker.io/rayproject/ray-ml", "docker.io/rayproject/ray-llm"},
		Commands: [][]string{{"ray", "start"}}},
	{Name: "seldon-core", Kind: aibom.KindPlatform, Supplier: "Seldon",
		Images: []string{"docker.io/seldonio/seldon-core-operator", "docker.io/seldonio/seldon-core-executor"}},
	{Name: "wandb", Kind: aibom.KindPlatform, Supplier: "Weights & Biases", Images: []string{"docker.io/wandb/local"}},
	{Name: "langfuse", Kind: aibom.KindPlatform, Supplier: "Langfuse",
		Images: []string{"docker.io/langfuse/langfuse", "docker.io/langfuse/langfuse-worker", "ghcr.io/langfuse/langfuse", "ghcr.io/langfuse/langfuse-worker"}},
	{Name: "phoenix", Kind: aibom.KindPlatform, Supplier: "Arize AI", Open: true,
		Images: []string{"docker.io/arizephoenix/phoenix"}, Commands: [][]string{{"-m", "phoenix.server.main"}}},

	// Vector and AI data stores.
	{Name: "qdrant", Kind: aibom.KindDataStore, Supplier: "Qdrant", Open: true, Images: []string{"docker.io/qdrant/qdrant"}},
	{Name: "milvus", Kind: aibom.KindDataStore, Supplier: "Zilliz", Open: true, Images: []string{"docker.io/milvusdb/milvus"}},
	{Name: "weaviate", Kind: aibom.KindDataStore, Supplier: "Weaviate", Open: true,
		Images: []string{"cr.weaviate.io/semitechnologies/weaviate", "docker.io/semitechnologies/weaviate"}},
	{Name: "chroma", Kind: aibom.KindDataStore, Supplier: "Chroma", Open: true,
		Images: []string{"docker.io/chromadb/chroma", "ghcr.io/chroma-core/chroma"}},
	{Name: "pgvector", Kind: aibom.KindDataStore, Supplier: "pgvector", Images: []string{"docker.io/pgvector/pgvector", "docker.io/ankane/pgvector"}},
	{Name: "redis-stack", Kind: aibom.KindDataStore, Supplier: "Redis", Open: true,
		Images: []string{"docker.io/redis/redis-stack", "docker.io/redis/redis-stack-server"}},

	// Agent tooling.
	{Name: "mcp-server", Kind: aibom.KindAgentTool, Open: true, NameFromImage: true,
		Prefixes: []string{"docker.io/mcp/"}, Images: []string{"ghcr.io/github/github-mcp-server"}},
}
