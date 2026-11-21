package kube

// This file holds the name tables the detectors match against. They are
// data: extend them here.

// credentialNames maps environment variable names that hold AI-provider
// credentials to the provider.
var credentialNames = map[string]string{
	"OPENAI_API_KEY":           "openai",
	"ANTHROPIC_API_KEY":        "anthropic",
	"HF_TOKEN":                 "huggingface",
	"HUGGING_FACE_HUB_TOKEN":   "huggingface",
	"HUGGINGFACEHUB_API_TOKEN": "huggingface",
	"HF_API_TOKEN":             "huggingface",
	"GOOGLE_API_KEY":           "google-gemini",
	"GEMINI_API_KEY":           "google-gemini",
	"COHERE_API_KEY":           "cohere",
	"CO_API_KEY":               "cohere",
	"MISTRAL_API_KEY":          "mistral",
	"GROQ_API_KEY":             "groq",
	"TOGETHER_API_KEY":         "together",
	"REPLICATE_API_TOKEN":      "replicate",
	"DEEPSEEK_API_KEY":         "deepseek",
	"XAI_API_KEY":              "xai",
	"PERPLEXITY_API_KEY":       "perplexity",
	"FIREWORKS_API_KEY":        "fireworks",
	"OPENROUTER_API_KEY":       "openrouter",
	"VOYAGE_API_KEY":           "voyage",
	"PINECONE_API_KEY":         "pinecone",
	"LANGCHAIN_API_KEY":        "langsmith",
	"LANGSMITH_API_KEY":        "langsmith",
	"WANDB_API_KEY":            "wandb",
	"NGC_API_KEY":              "nvidia-ngc",
	"LITELLM_MASTER_KEY":       "litellm",
}

// credentialPrefixes catch families such as AZURE_OPENAI_API_KEY and
// AZURE_OPENAI_KEY: a known prefix plus a credential-looking suffix.
var credentialPrefixes = map[string]string{"AZURE_OPENAI_": "azure-openai"}

var credentialSuffixes = []string{"_KEY", "_TOKEN", "_SECRET"}

// credentialFlags are command-line flags whose value is a credential.
var credentialFlags = []string{"--api-key", "--hf-token"}

// providerHosts maps API host names (exact or as a parent domain) to the
// provider, for base-URL settings.
var providerHosts = map[string]string{
	"api.openai.com":                    "openai",
	"api.anthropic.com":                 "anthropic",
	"openai.azure.com":                  "azure-openai",
	"generativelanguage.googleapis.com": "google-gemini",
	"aiplatform.googleapis.com":         "google-vertex",
	"api.cohere.ai":                     "cohere",
	"api.cohere.com":                    "cohere",
	"api.mistral.ai":                    "mistral",
	"api.groq.com":                      "groq",
	"api.together.xyz":                  "together",
	"api.deepseek.com":                  "deepseek",
	"api.x.ai":                          "xai",
	"openrouter.ai":                     "openrouter",
	"api-inference.huggingface.co":      "huggingface",
	"router.huggingface.co":             "huggingface",
	"api.fireworks.ai":                  "fireworks",
	"api.perplexity.ai":                 "perplexity",
	"integrate.api.nvidia.com":          "nvidia-nim-api",
}

// urlNameSuffixes say an environment variable holds an endpoint, so its
// value may be parsed for a host. Other values are never looked at.
var urlNameSuffixes = []string{"_URL", "_BASE", "_ENDPOINT", "_API_BASE", "_HOST", "_URI"}

// modelEnvNames hold a model identifier.
var modelEnvNames = map[string]bool{"MODEL_ID": true, "MODEL_NAME": true, "HF_MODEL_ID": true, "OLLAMA_MODEL": true}

// modelFlags take a model identifier or location.
var modelFlags = []string{"--model", "--model-id", "--model-name", "--model_name", "--model-path"}

const servedNameFlag = "--served-model-name"

var revisionFlags = []string{"--revision"}

var revisionEnvNames = map[string]bool{"REVISION": true, "HF_REVISION": true, "MODEL_REVISION": true}

// offlineEnvNames, set to a true value, stop Hugging Face libraries from
// downloading anything.
var offlineEnvNames = map[string]bool{"HF_HUB_OFFLINE": true, "TRANSFORMERS_OFFLINE": true}

const trustRemoteCodeFlag = "--trust-remote-code"

// providerModelPrefixes recognise hosted-model names used through an API.
var providerModelPrefixes = map[string]string{
	"gpt-": "openai", "o1": "openai", "o3": "openai", "o4": "openai", "chatgpt-": "openai", "text-embedding-": "openai",
	"claude-": "anthropic", "gemini-": "google-gemini", "command-": "cohere",
}

// modelURISchemes are storage locations for weights.
var modelURISchemes = []string{"hf://", "s3://", "gs://", "pvc://", "oci://", "https://", "http://"}

// jupyterTokenFlags and jupyterPasswordFlags configure notebook auth; an
// empty token with no password means no authentication at all.
var jupyterTokenFlags = []string{"--NotebookApp.token=", "--ServerApp.token=", "--IdentityProvider.token="}

var jupyterPasswordFlags = []string{"--NotebookApp.password=", "--ServerApp.password=", "--PasswordIdentityProvider.hashed_password="}

// acceleratorRules recognise extended resources for accelerators.
var acceleratorRules = []struct {
	Resource string
	Prefix   bool
	Supplier string
}{
	{"nvidia.com/gpu", false, "NVIDIA"},
	{"nvidia.com/gpu.", true, "NVIDIA"},
	{"nvidia.com/mig-", true, "NVIDIA"},
	{"amd.com/gpu", false, "AMD"},
	{"habana.ai/gaudi", false, "Intel Habana"},
	{"gpu.intel.com/", true, "Intel"},
	{"aws.amazon.com/neuron", true, "AWS"},
	{"google.com/tpu", false, "Google"},
}

// gpuNodeKeys are node-selector and toleration keys that steer a pod onto
// accelerator nodes, mapped to the accelerator they imply.
var gpuNodeKeys = map[string]string{
	"nvidia.com/gpu":                       "nvidia.com/gpu",
	"nvidia.com/gpu.present":               "nvidia.com/gpu",
	"nvidia.com/gpu.product":               "nvidia.com/gpu",
	"nvidia.com/gpu.count":                 "nvidia.com/gpu",
	"cloud.google.com/gke-accelerator":     "nvidia.com/gpu",
	"k8s.amazonaws.com/accelerator":        "nvidia.com/gpu",
	"cloud.google.com/gke-tpu-accelerator": "google.com/tpu",
	"amd.com/gpu":                          "amd.com/gpu",
	"habana.ai/gaudi":                      "habana.ai/gaudi",
	"aws.amazon.com/neuron":                "aws.amazon.com/neuron",
}

const (
	visibleDevicesEnv = "NVIDIA_VISIBLE_DEVICES"
	sharedGPUSuffix   = ".shared"
)
