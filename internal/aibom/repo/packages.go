package repo

// Package ecosystems, as recorded in Component.Ecosystem.
const (
	ecoPyPI        = "pypi"
	ecoNPM         = "npm"
	ecoGo          = "go"
	ecoCargo       = "cargo"
	ecoMaven       = "maven"
	ecoHuggingFace = "huggingface"
	ecoOllama      = "ollama"
)

// Categories group AI libraries so a reader can see at a glance what kind
// of AI a project uses.
const (
	catLLMClient   = "llm-client"
	catFramework   = "llm-framework"
	catML          = "ml-framework"
	catServing     = "model-serving"
	catVectorStore = "vector-store"
	catMCP         = "mcp"
	catMLOps       = "mlops"
)

// aiPackage is one row of the curated table. A name ending in "*" matches
// every package that starts with the rest, so "langchain*" covers
// langchain-openai and langchain-community.
type aiPackage struct {
	name     string
	supplier string
	category string
}

// aiPackages lists the AI and ML packages recognised in each ecosystem.
// Names are compared after normalisation (see normalisePackage). Only
// packages whose main purpose is AI belong here: a general-purpose package
// that can talk to a model would make the inventory noisy.
var aiPackages = map[string][]aiPackage{
	ecoPyPI: {
		{"openai", "OpenAI", catLLMClient}, {"anthropic", "Anthropic", catLLMClient},
		{"google-generativeai", "Google", catLLMClient}, {"google-genai", "Google", catLLMClient},
		{"google-cloud-aiplatform", "Google", catLLMClient}, {"vertexai", "Google", catLLMClient},
		{"cohere", "Cohere", catLLMClient}, {"mistralai", "Mistral AI", catLLMClient},
		{"groq", "Groq", catLLMClient}, {"together", "Together AI", catLLMClient},
		{"replicate", "Replicate", catLLMClient}, {"huggingface-hub", "Hugging Face", catLLMClient},
		{"ollama", "Ollama", catLLMClient}, {"litellm", "BerriAI", catLLMClient},
		{"boto3-bedrock", "Amazon", catLLMClient},
		{"langchain*", "LangChain", catFramework}, {"langgraph*", "LangChain", catFramework},
		{"langsmith", "LangChain", catMLOps},
		{"llama-index*", "LlamaIndex", catFramework}, {"llama-parse", "LlamaIndex", catFramework},
		{"haystack-ai", "deepset", catFramework}, {"farm-haystack", "deepset", catFramework},
		{"pyautogen", "Microsoft", catFramework}, {"autogen*", "Microsoft", catFramework},
		{"ag2", "AG2", catFramework}, {"crewai*", "CrewAI", catFramework},
		{"semantic-kernel", "Microsoft", catFramework}, {"dspy", "Stanford NLP", catFramework},
		{"dspy-ai", "Stanford NLP", catFramework}, {"instructor", "567 Labs", catFramework},
		{"guidance", "Microsoft", catFramework}, {"pydantic-ai*", "Pydantic", catFramework},
		{"openai-agents", "OpenAI", catFramework}, {"claude-agent-sdk", "Anthropic", catFramework},
		{"smolagents", "Hugging Face", catFramework},
		{"transformers", "Hugging Face", catML}, {"diffusers", "Hugging Face", catML},
		{"sentence-transformers", "Hugging Face", catML}, {"tokenizers", "Hugging Face", catML},
		{"accelerate", "Hugging Face", catML}, {"peft", "Hugging Face", catML},
		{"trl", "Hugging Face", catML}, {"datasets", "Hugging Face", catML},
		{"safetensors", "Hugging Face", catML}, {"tiktoken", "OpenAI", catML},
		{"torch", "PyTorch Foundation", catML}, {"torchvision", "PyTorch Foundation", catML},
		{"torchaudio", "PyTorch Foundation", catML}, {"tensorflow*", "Google", catML},
		{"keras", "Google", catML}, {"jax", "Google", catML}, {"flax", "Google", catML},
		{"onnx", "LF AI & Data", catML}, {"onnxruntime*", "Microsoft", catML},
		{"scikit-learn", "scikit-learn", catML}, {"xgboost", "DMLC", catML},
		{"lightgbm", "Microsoft", catML}, {"catboost", "Yandex", catML},
		{"spacy", "Explosion", catML}, {"bitsandbytes", "bitsandbytes", catML},
		{"vllm", "vLLM", catServing}, {"llama-cpp-python", "llama.cpp", catServing},
		{"text-generation", "Hugging Face", catServing}, {"bentoml", "BentoML", catServing},
		{"ray", "Anyscale", catServing}, {"tritonclient", "NVIDIA", catServing},
		{"mlflow*", "Databricks", catMLOps}, {"wandb", "Weights & Biases", catMLOps},
		{"chromadb", "Chroma", catVectorStore}, {"pinecone*", "Pinecone", catVectorStore},
		{"weaviate-client", "Weaviate", catVectorStore}, {"qdrant-client", "Qdrant", catVectorStore},
		{"pymilvus", "Zilliz", catVectorStore}, {"faiss*", "Meta", catVectorStore},
		{"lancedb", "LanceDB", catVectorStore}, {"pgvector", "pgvector", catVectorStore},
		{"mcp", "Anthropic", catMCP}, {"fastmcp", "Prefect", catMCP},
	},
	ecoNPM: {
		{"openai", "OpenAI", catLLMClient}, {"@anthropic-ai/*", "Anthropic", catLLMClient},
		{"@google/generative-ai", "Google", catLLMClient}, {"@google/genai", "Google", catLLMClient},
		{"@google-cloud/vertexai", "Google", catLLMClient}, {"cohere-ai", "Cohere", catLLMClient},
		{"@mistralai/mistralai", "Mistral AI", catLLMClient}, {"groq-sdk", "Groq", catLLMClient},
		{"ollama", "Ollama", catLLMClient}, {"replicate", "Replicate", catLLMClient},
		{"@aws-sdk/client-bedrock*", "Amazon", catLLMClient},
		{"ai", "Vercel", catFramework}, {"@ai-sdk/*", "Vercel", catFramework},
		{"langchain", "LangChain", catFramework}, {"@langchain/*", "LangChain", catFramework},
		{"llamaindex", "LlamaIndex", catFramework}, {"@llamaindex/*", "LlamaIndex", catFramework},
		{"@openai/agents", "OpenAI", catFramework}, {"@mastra/*", "Mastra", catFramework},
		{"@huggingface/*", "Hugging Face", catML}, {"@xenova/transformers", "Hugging Face", catML},
		{"@tensorflow/*", "Google", catML}, {"onnxruntime-node", "Microsoft", catML},
		{"onnxruntime-web", "Microsoft", catML}, {"onnxruntime-react-native", "Microsoft", catML},
		{"tiktoken", "OpenAI", catML}, {"js-tiktoken", "OpenAI", catML},
		{"@pinecone-database/pinecone", "Pinecone", catVectorStore}, {"chromadb", "Chroma", catVectorStore},
		{"weaviate-client", "Weaviate", catVectorStore}, {"weaviate-ts-client", "Weaviate", catVectorStore},
		{"@qdrant/js-client-rest", "Qdrant", catVectorStore}, {"@zilliz/milvus2-sdk-node", "Zilliz", catVectorStore},
		{"@lancedb/lancedb", "LanceDB", catVectorStore}, {"vectordb", "LanceDB", catVectorStore},
		{"@modelcontextprotocol/*", "Anthropic", catMCP}, {"fastmcp", "punkpeye", catMCP},
	},
	ecoGo: {
		{"github.com/sashabaranov/go-openai", "sashabaranov", catLLMClient},
		{"github.com/openai/openai-go*", "OpenAI", catLLMClient},
		{"github.com/anthropics/anthropic-sdk-go", "Anthropic", catLLMClient},
		{"github.com/google/generative-ai-go", "Google", catLLMClient},
		{"google.golang.org/genai", "Google", catLLMClient},
		{"github.com/ollama/ollama", "Ollama", catLLMClient},
		{"github.com/tmc/langchaingo", "tmc", catFramework},
		{"github.com/firebase/genkit/go", "Google", catFramework},
		{"github.com/cloudwego/eino*", "CloudWeGo", catFramework},
		{"github.com/mark3labs/mcp-go", "mark3labs", catMCP},
		{"github.com/modelcontextprotocol/go-sdk", "Anthropic", catMCP},
		{"github.com/pinecone-io/go-pinecone*", "Pinecone", catVectorStore},
		{"github.com/qdrant/go-client", "Qdrant", catVectorStore},
		{"github.com/weaviate/weaviate-go-client*", "Weaviate", catVectorStore},
		{"github.com/pgvector/pgvector-go", "pgvector", catVectorStore},
		{"github.com/yalue/onnxruntime_go", "yalue", catML},
	},
	ecoCargo: {
		{"candle-*", "Hugging Face", catML}, {"burn", "Tracel", catML}, {"burn-*", "Tracel", catML},
		{"tch", "LaurentMazare", catML}, {"ort", "pykeio", catML}, {"tokenizers", "Hugging Face", catML},
		{"hf-hub", "Hugging Face", catML}, {"llama-cpp-2", "utilityai", catServing},
		{"async-openai", "64bit", catLLMClient}, {"rig-core", "0xPlaygrounds", catFramework},
		{"ollama-rs", "pepperoni21", catLLMClient}, {"rmcp", "Anthropic", catMCP},
		{"qdrant-client", "Qdrant", catVectorStore}, {"lancedb", "LanceDB", catVectorStore},
	},
	ecoMaven: {
		{"dev.langchain4j:*", "LangChain4j", catFramework},
		{"org.springframework.ai:*", "VMware", catFramework},
		{"com.openai:openai-java*", "OpenAI", catLLMClient},
		{"com.anthropic:anthropic-java*", "Anthropic", catLLMClient},
		{"com.theokanning.openai-gpt3-java:*", "TheoKanning", catLLMClient},
		{"com.google.genai:google-genai", "Google", catLLMClient},
		{"com.google.cloud:google-cloud-aiplatform", "Google", catLLMClient},
		{"com.google.cloud:google-cloud-vertexai", "Google", catLLMClient},
		{"io.github.ollama4j:ollama4j", "ollama4j", catLLMClient},
		{"ai.djl:*", "Amazon", catML}, {"ai.djl.*", "Amazon", catML},
		{"org.tensorflow:*", "Google", catML}, {"org.deeplearning4j:*", "Eclipse", catML},
		{"com.microsoft.onnxruntime:*", "Microsoft", catML},
		{"io.modelcontextprotocol.sdk:*", "Anthropic", catMCP},
		{"io.pinecone:*", "Pinecone", catVectorStore}, {"io.qdrant:*", "Qdrant", catVectorStore},
		{"io.milvus:*", "Zilliz", catVectorStore},
	},
}
