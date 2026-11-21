package repo

import "github.com/dhruv29/Pinaka/internal/aibom"

// sdkProviders maps a vendor SDK, by its normalised package name, to the
// hosted provider it calls by default. An app that uses the OpenAI SDK
// talks to the OpenAI API even though the host is never written down.
var sdkProviders = map[string]string{
	"openai":                                 "OpenAI API",
	"langchain-openai":                       "OpenAI API",
	"@langchain/openai":                      "OpenAI API",
	"github.com/sashabaranov/go-openai":      "OpenAI API",
	"github.com/openai/openai-go":            "OpenAI API",
	"async-openai":                           "OpenAI API",
	"com.openai:openai-java":                 "OpenAI API",
	"anthropic":                              "Anthropic API",
	"@anthropic-ai/sdk":                      "Anthropic API",
	"langchain-anthropic":                    "Anthropic API",
	"@langchain/anthropic":                   "Anthropic API",
	"github.com/anthropics/anthropic-sdk-go": "Anthropic API",
	"com.anthropic:anthropic-java":           "Anthropic API",
	"google-generativeai":                    "Google Gemini API",
	"google-genai":                           "Google Gemini API",
	"@google/generative-ai":                  "Google Gemini API",
	"@google/genai":                          "Google Gemini API",
	"langchain-google-genai":                 "Google Gemini API",
	"mistralai":                              "Mistral AI API",
	"@mistralai/mistralai":                   "Mistral AI API",
	"cohere":                                 "Cohere API",
	"cohere-ai":                              "Cohere API",
	"groq":                                   "Groq API",
	"groq-sdk":                               "Groq API",
	"together":                               "Together AI API",
	"together-ai":                            "Together AI API",
	"replicate":                              "Replicate API",
}

// inferProvider remembers that the code uses providerName, because of
// what. Inferences are added only at the end of the scan, and only when no
// AI client in the code is pointed at another address: a project that
// sends its OpenAI client to a self-hosted server does not use the OpenAI
// API, whatever its dependencies say.
func (s *scanner) inferProvider(f *textFile, line int, providerName, because string) {
	p, ok := aibom.ProviderNamed(providerName)
	if !ok || f.context != contextCode {
		return
	}
	s.inferred = append(s.inferred, finding{
		component: aibom.Component{Kind: aibom.KindProvider, Name: p.Name, Supplier: p.Supplier, Properties: map[string]string{propContext: f.context}},
		evidence:  aibom.Evidence{Path: f.path, Line: line, Detail: "inferred from " + because},
	})
}

// addInferred records the inferred providers unless a client is redirected.
func (s *scanner) addInferred() {
	if s.redirected {
		return
	}
	for _, f := range s.inferred {
		s.add(f)
	}
}
