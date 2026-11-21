package repo

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// aiCredentials maps the environment variable names AI SDKs read their keys
// from to the supplier of the service. Only names are ever recorded.
var aiCredentials = map[string]string{
	"OPENAI_API_KEY": "OpenAI", "OPENAI_ORG_ID": "OpenAI", "AZURE_OPENAI_API_KEY": "Microsoft",
	"AZURE_OPENAI_KEY": "Microsoft", "ANTHROPIC_API_KEY": "Anthropic", "CLAUDE_API_KEY": "Anthropic",
	"HF_TOKEN": "Hugging Face", "HUGGING_FACE_HUB_TOKEN": "Hugging Face", "HUGGINGFACEHUB_API_TOKEN": "Hugging Face",
	"HF_API_TOKEN": "Hugging Face", "GOOGLE_API_KEY": "Google", "GEMINI_API_KEY": "Google",
	"COHERE_API_KEY": "Cohere", "CO_API_KEY": "Cohere", "MISTRAL_API_KEY": "Mistral AI",
	"GROQ_API_KEY": "Groq", "TOGETHER_API_KEY": "Together AI", "REPLICATE_API_TOKEN": "Replicate",
	"OPENROUTER_API_KEY": "OpenRouter", "DEEPSEEK_API_KEY": "DeepSeek", "XAI_API_KEY": "xAI",
	"FIREWORKS_API_KEY": "Fireworks AI", "PERPLEXITY_API_KEY": "Perplexity", "VOYAGE_API_KEY": "Voyage AI",
	"PINECONE_API_KEY": "Pinecone", "QDRANT_API_KEY": "Qdrant", "WEAVIATE_API_KEY": "Weaviate",
	"WANDB_API_KEY": "Weights & Biases", "LANGCHAIN_API_KEY": "LangChain", "LANGSMITH_API_KEY": "LangChain",
	"ELEVENLABS_API_KEY": "ElevenLabs",
}

// credentialName matches any table name as a whole word. Underscore counts
// as part of a word, so AZURE_OPENAI_API_KEY never also matches
// OPENAI_API_KEY.
var credentialName = func() *regexp.Regexp {
	names := make([]string, 0, len(aiCredentials))
	for name := range aiCredentials {
		names = append(names, regexp.QuoteMeta(name))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return regexp.MustCompile(`\b(` + strings.Join(names, "|") + `)\b`)
}()

// assignedValue is what follows a credential name in an assignment:
// `NAME=value`, `NAME: value`, `"NAME": "value"`, `NAME := "value"`.
var assignedValue = regexp.MustCompile(`^["']?\s*(?::=|=|:)\s*["']?([^\s"',;)}\]]*)`)

// credentialNames records each credential name and flags a line that
// assigns it a literal that looks like a real key. The value is examined
// in place and never stored.
func (s *scanner) credentialNames(f *textFile) {
	for _, loc := range credentialName.FindAllStringSubmatchIndex(f.text, -1) {
		name := f.text[loc[2]:loc[3]]
		rest := f.text[loc[3]:min(len(f.text), loc[3]+maxAssignmentBytes)]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[:nl]
		}
		s.credential(f, loc[2], name, literalAssigned(rest))
	}
}

const (
	maxAssignmentBytes = 400
	minKeyLength       = 20
	minKeyEntropy      = 3.5
)

// credential records one credential name; hardcoded adds the high risk.
func (s *scanner) credential(f *textFile, offset int, name string, hardcoded bool) {
	found := finding{
		component: aibom.Component{Kind: aibom.KindCredential, Name: name, Supplier: aiCredentials[name], Properties: map[string]string{propContext: f.context}},
		evidence:  aibom.Evidence{Path: f.path, Line: f.lineAt(offset), Detail: "credential referenced by name"},
	}
	if hardcoded {
		found.evidence.Detail = "literal value assigned here (value not recorded)"
		found.risks = []aibom.Risk{risk(riskHardcodedCred)}
	}
	s.add(found)
}

// literalAssigned reports whether rest, the text right after a credential
// name, assigns it a literal that looks like a real key.
func literalAssigned(rest string) bool {
	if strings.HasPrefix(rest, "==") || strings.HasPrefix(rest, "=\"=") {
		return false
	}
	m := assignedValue.FindStringSubmatch(rest)
	return m != nil && looksLikeKey(m[1])
}

// keyPrefixes start the keys AI vendors issue.
var keyPrefixes = []string{"sk-", "sk_", "hf_", "AIza", "gsk_", "xai-", "pcsk_", "r8_", "co-"}

// placeholderMarks make a value documentation or a lookup, not a key.
var placeholderMarks = []string{
	"example", "your", "xxx", "changeme", "change-me", "placeholder", "dummy", "fake", "sample",
	"redacted", "replace", "insert", "test", "<", "$", "{", "...", "***", "environ", "getenv",
	"process.env", "secrets.", "vars.", "config.", "settings.",
}

func looksLikeKey(value string) bool {
	lower := strings.ToLower(value)
	if len(value) < minKeyLength {
		return false
	}
	for _, mark := range placeholderMarks {
		if strings.Contains(lower, mark) {
			return false
		}
	}
	for _, prefix := range keyPrefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return strings.ContainsAny(value, "0123456789") && strings.IndexFunc(value, isLetter) >= 0 &&
		entropy(value) >= minKeyEntropy
}

func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

// entropy is the Shannon entropy of s in bits per character.
func entropy(s string) float64 {
	counts := map[rune]int{}
	for _, r := range s {
		counts[r]++
	}
	total, sum := float64(len(s)), 0.0
	for _, n := range counts {
		p := float64(n) / total
		sum -= p * math.Log2(p)
	}
	return sum
}
