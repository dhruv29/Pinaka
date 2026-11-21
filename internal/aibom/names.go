package aibom

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

// This file holds the name tables both scanners share, so a provider or a
// credential seen in the code and in the cluster carries the same name and
// the two join into one component when the bills are merged.

// Provider is a hosted AI service recognised by its API hostname.
type Provider struct {
	// Name is the component name, such as "OpenAI API".
	Name string
	// Supplier is who runs the service.
	Supplier string
	// HostPattern is a regular expression for the API hostname. Wildcard
	// parts are limited to DNS label characters; callers anchor it.
	HostPattern string
}

// Providers is the table of recognised hosted AI services.
var Providers = []Provider{
	{"OpenAI API", "OpenAI", `api\.openai\.com`},
	{"Anthropic API", "Anthropic", `api\.anthropic\.com`},
	{"Google Gemini API", "Google", `generativelanguage\.googleapis\.com`},
	{"Google Vertex AI", "Google", `[a-z0-9-]+-aiplatform\.googleapis\.com|aiplatform\.googleapis\.com`},
	{"Azure OpenAI", "Microsoft", `[a-z0-9-]+\.openai\.azure\.com`},
	{"Azure AI Inference", "Microsoft", `[a-z0-9-]+\.services\.ai\.azure\.com|models\.inference\.ai\.azure\.com`},
	{"Amazon Bedrock", "Amazon", `bedrock-runtime(?:-fips)?\.[a-z0-9-]+\.amazonaws\.com`},
	{"Mistral AI API", "Mistral AI", `api\.mistral\.ai|codestral\.mistral\.ai`},
	{"Cohere API", "Cohere", `api\.cohere\.(?:com|ai)`},
	{"Groq API", "Groq", `api\.groq\.com`},
	{"Together AI API", "Together AI", `api\.together\.(?:xyz|ai)`},
	{"OpenRouter", "OpenRouter", `openrouter\.ai`},
	{"Replicate API", "Replicate", `api\.replicate\.com`},
	{"Hugging Face Inference", "Hugging Face", `api-inference\.huggingface\.co|router\.huggingface\.co|[a-z0-9-]+\.endpoints\.huggingface\.cloud`},
	{"xAI API", "xAI", `api\.x\.ai`},
	{"DeepSeek API", "DeepSeek", `api\.deepseek\.com`},
	{"Fireworks AI API", "Fireworks AI", `api\.fireworks\.ai`},
	{"Perplexity API", "Perplexity", `api\.perplexity\.ai`},
	{"Voyage AI API", "Voyage AI", `api\.voyageai\.com`},
	{"NVIDIA API Catalog", "NVIDIA", `integrate\.api\.nvidia\.com`},
	{"Pinecone", "Pinecone", `[a-z0-9-]+\.pinecone\.io`},
	{"Ollama (local)", "Ollama", `(?:localhost|127\.0\.0\.1|0\.0\.0\.0|ollama):11434`},
}

// Credential describes an environment variable an AI SDK reads its key from.
type Credential struct {
	// Supplier is who issues the key.
	Supplier string
	// Provider names the hosted service in Providers the key is for, or is
	// empty when the key is not for a hosted API (a self-hosted proxy's
	// master key, say).
	Provider string
}

// Credentials maps the environment variable names AI SDKs read their keys
// from to what they are for. Only names are ever recorded. Hugging Face
// tokens name no provider: they mostly download weights from the Hub, which
// is not a call to a hosted inference API.
var Credentials = map[string]Credential{
	"OPENAI_API_KEY":           {"OpenAI", "OpenAI API"},
	"OPENAI_ORG_ID":            {"OpenAI", "OpenAI API"},
	"AZURE_OPENAI_API_KEY":     {"Microsoft", "Azure OpenAI"},
	"AZURE_OPENAI_KEY":         {"Microsoft", "Azure OpenAI"},
	"ANTHROPIC_API_KEY":        {"Anthropic", "Anthropic API"},
	"CLAUDE_API_KEY":           {"Anthropic", "Anthropic API"},
	"HF_TOKEN":                 {"Hugging Face", ""},
	"HUGGING_FACE_HUB_TOKEN":   {"Hugging Face", ""},
	"HUGGINGFACEHUB_API_TOKEN": {"Hugging Face", ""},
	"HF_API_TOKEN":             {"Hugging Face", ""},
	"GOOGLE_API_KEY":           {"Google", "Google Gemini API"},
	"GEMINI_API_KEY":           {"Google", "Google Gemini API"},
	"COHERE_API_KEY":           {"Cohere", "Cohere API"},
	"CO_API_KEY":               {"Cohere", "Cohere API"},
	"MISTRAL_API_KEY":          {"Mistral AI", "Mistral AI API"},
	"GROQ_API_KEY":             {"Groq", "Groq API"},
	"TOGETHER_API_KEY":         {"Together AI", "Together AI API"},
	"REPLICATE_API_TOKEN":      {"Replicate", "Replicate API"},
	"OPENROUTER_API_KEY":       {"OpenRouter", "OpenRouter"},
	"DEEPSEEK_API_KEY":         {"DeepSeek", "DeepSeek API"},
	"XAI_API_KEY":              {"xAI", "xAI API"},
	"FIREWORKS_API_KEY":        {"Fireworks AI", "Fireworks AI API"},
	"PERPLEXITY_API_KEY":       {"Perplexity", "Perplexity API"},
	"VOYAGE_API_KEY":           {"Voyage AI", "Voyage AI API"},
	"PINECONE_API_KEY":         {"Pinecone", "Pinecone"},
	"NGC_API_KEY":              {"NVIDIA", "NVIDIA API Catalog"},
	"QDRANT_API_KEY":           {"Qdrant", ""},
	"WEAVIATE_API_KEY":         {"Weaviate", ""},
	"WANDB_API_KEY":            {"Weights & Biases", ""},
	"LANGCHAIN_API_KEY":        {"LangChain", ""},
	"LANGSMITH_API_KEY":        {"LangChain", ""},
	"ELEVENLABS_API_KEY":       {"ElevenLabs", ""},
	"LITELLM_MASTER_KEY":       {"LiteLLM", ""},
}

// anchoredProviders match a whole hostname, for callers that already hold
// one (a parsed URL) rather than free text.
var anchoredProviders = func() []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(Providers))
	for i, p := range Providers {
		out[i] = regexp.MustCompile(`^(?:` + p.HostPattern + `)$`)
	}
	return out
}()

// ProviderForHost returns the hosted provider whose API host is host, which
// may carry a port. The match is on the whole name, case-insensitively.
func ProviderForHost(host string) (Provider, bool) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	bare := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		bare = h
	}
	for i, re := range anchoredProviders {
		if re.MatchString(host) || re.MatchString(bare) {
			return Providers[i], true
		}
	}
	return Provider{}, false
}

// ProviderNamed returns the provider with the given Name.
func ProviderNamed(name string) (Provider, bool) {
	for _, p := range Providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// CustomEndpointName is the component name for an AI client pointed at a
// host that is not a known vendor's.
func CustomEndpointName(host string) string {
	return "custom endpoint " + strings.ToLower(host)
}

// IsPrivateHost reports loopback, private and single-label hosts, and
// cluster-internal names: a model server run by the same organisation
// rather than a third party. host may carry a port.
func IsPrivateHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(host)
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified()
	}
	return host == "localhost" || !strings.Contains(host, ".") || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".svc") || strings.HasSuffix(host, ".cluster.local")
}

// RedactLocation returns a model location or reference with anything that
// can carry a credential removed: user information, query and fragment.
// A URL keeps its scheme, host and path. A reference without a scheme,
// such as "user:token@registry/model", loses everything up to an "@" that
// comes before the first "/", since that part can only be user information. It
// returns "" when nothing safe is left, or when a URL cannot be parsed.
func RedactLocation(v string) string {
	if strings.Contains(v, "://") {
		u, err := url.Parse(v)
		if err != nil || u.Host == "" {
			return ""
		}
		return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
	}
	if i := strings.IndexAny(v, "?#"); i >= 0 {
		v = v[:i]
	}
	head := v
	if slash := strings.IndexByte(v, '/'); slash >= 0 {
		head = v[:slash]
	}
	// "model@sha256:…" is a digest, not user information: strip only when
	// what precedes the "@" has a colon (user:password) or a host and path
	// follow it (token@host/model).
	if at := strings.LastIndexByte(head, '@'); at >= 0 && (strings.Contains(head[:at], ":") || strings.Contains(v[at+1:], "/")) {
		v = v[at+1:]
	}
	return v
}
