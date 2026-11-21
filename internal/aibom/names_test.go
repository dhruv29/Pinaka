package aibom

import "testing"

func TestRedactLocationKeepsTheLocationAndDropsWhatCanCarryACredential(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"https://u:pw@h.example/x.gguf?sig=1#f", "https://h.example/x.gguf"},
		{"s3://AKIA:secret@bucket/models/m", "s3://bucket/models/m"},
		{"http://", ""},
		{"https://%zz", ""},
		{"user:pw@registry.example/org/llama", "registry.example/org/llama"},
		{"token@models.example/org/m", "models.example/org/m"},
		{"a:b@c@d.example/m", "d.example/m"},
		{"./weights.gguf?token=x", "./weights.gguf"},
		{"llama3@sha256:abc", "llama3@sha256:abc"},
		{"registry:5000/org/m@sha256:abc", "registry:5000/org/m@sha256:abc"},
		{"meta-llama/Llama-3.1-8B", "meta-llama/Llama-3.1-8B"},
	} {
		if got := RedactLocation(c.in); got != c.want {
			t.Errorf("RedactLocation(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestProviderForHostMatchesWholeHostsOnly(t *testing.T) {
	for host, want := range map[string]string{
		"api.openai.com": "OpenAI API", "API.OpenAI.com:443": "OpenAI API", "api.openai.com.": "OpenAI API",
		"my-res.openai.azure.com": "Azure OpenAI", "us-central1-aiplatform.googleapis.com": "Google Vertex AI",
		"integrate.api.nvidia.com": "NVIDIA API Catalog", "localhost:11434": "Ollama (local)",
		"api.openai.com.evil.example": "", "myapi.openai.com": "", "vllm.ml.svc:8000": "",
	} {
		p, ok := ProviderForHost(host)
		if ok != (want != "") || p.Name != want {
			t.Errorf("ProviderForHost(%q) = %q, %v; want %q", host, p.Name, ok, want)
		}
	}
}

func TestEveryCredentialNamesAKnownProviderOrNone(t *testing.T) {
	for name, c := range Credentials {
		if c.Supplier == "" {
			t.Errorf("%s has no supplier", name)
		}
		if _, ok := ProviderNamed(c.Provider); c.Provider != "" && !ok {
			t.Errorf("%s names unknown provider %q", name, c.Provider)
		}
	}
	if _, ok := ProviderNamed("Nobody API"); ok {
		t.Error("an unknown provider was found")
	}
}

func TestIsPrivateHostTellsInHouseFromThirdPartyHosts(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost:8000": true, "10.0.0.5": true, "127.0.0.1:11434": true, "vllm": true, "vllm.ml.svc:8000": true,
		"x.ml.svc.cluster.local": true, "box.local": true, "llm.corp.internal": true,
		"llm.partner.example": false, "8.8.8.8": false, "api.openai.com": false,
	} {
		if got := IsPrivateHost(host); got != want {
			t.Errorf("IsPrivateHost(%q) = %v, want %v", host, got, want)
		}
	}
	if CustomEndpointName("VLLM.svc:80") != "custom endpoint vllm.svc:80" {
		t.Error("CustomEndpointName does not lower-case the host")
	}
}
