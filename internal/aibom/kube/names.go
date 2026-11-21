package kube

// This file holds the name tables the detectors match against. They are
// data: extend them here.

// Credential names and hosted providers come from the tables in package
// aibom, which the repository scan shares, so the same key or service
// found in the code and in the cluster carries the same name.

// credentialPrefixes catch families such as AZURE_OPENAI_API_KEY and
// AZURE_OPENAI_KEY: a known prefix plus a credential-looking suffix.
// The value is the table entry the family belongs to.
var credentialPrefixes = map[string]string{"AZURE_OPENAI_": "AZURE_OPENAI_API_KEY"}

var credentialSuffixes = []string{"_KEY", "_TOKEN", "_SECRET"}

// credentialFlags are command-line flags whose value is a credential.
var credentialFlags = []string{"--api-key", "--hf-token"}

// aiEndpointWords mark an endpoint setting as an AI client's, so its host is
// listed even when it is no known vendor's.
var aiEndpointWords = []string{"OPENAI", "ANTHROPIC", "OLLAMA", "LLM", "GEMINI", "MISTRAL", "COHERE", "GROQ", "INFERENCE", "TGI", "MODEL"}

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

// providerModelPrefixes recognise hosted-model names used through an API,
// mapped to the supplier the repository scan gives the same models.
var providerModelPrefixes = map[string]string{
	"gpt-": "OpenAI", "o1": "OpenAI", "o3": "OpenAI", "o4": "OpenAI", "chatgpt-": "OpenAI", "text-embedding-": "OpenAI",
	"claude-": "Anthropic", "gemini-": "Google", "command-": "Cohere",
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
