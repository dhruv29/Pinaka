package repo

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

type compiledProvider struct {
	aibom.Provider
	re *regexp.Regexp
}

// compiledProviders match a host with no letter, digit, dot or hyphen
// either side, so "api.openai.com.evil.example" or "myapi.openai.com" does
// not count as the vendor.
var compiledProviders = func() []compiledProvider {
	out := make([]compiledProvider, 0, len(aibom.Providers))
	for _, p := range aibom.Providers {
		re := regexp.MustCompile(`(?i)(?:^|[^a-z0-9.-])(` + p.HostPattern + `)(?:$|[^a-z0-9.-]|\.(?:$|[^a-z0-9]))`)
		out = append(out, compiledProvider{p, re})
	}
	return out
}()

// providerHosts records every recognised API host in the file.
func (s *scanner) providerHosts(f *textFile) {
	for _, p := range compiledProviders {
		for _, loc := range p.re.FindAllStringSubmatchIndex(f.text, -1) {
			host := strings.ToLower(f.text[loc[2]:loc[3]])
			s.add(finding{
				component: aibom.Component{Kind: aibom.KindProvider, Name: p.Name, Supplier: p.Supplier, Properties: map[string]string{propContext: f.context}},
				evidence:  aibom.Evidence{Path: f.path, Line: f.lineAt(loc[2]), Detail: "API host " + host},
			})
		}
	}
}

// vendorProvider returns the provider whose API host is host, if any.
func vendorProvider(host string) (aibom.Provider, bool) {
	for _, p := range compiledProviders {
		if p.re.MatchString(host) {
			return p.Provider, true
		}
	}
	return aibom.Provider{}, false
}

// baseURLOverride is a client base URL set to a literal: base_url=,
// baseURL:, api_base= or the SDKs' environment variables.
var baseURLOverride = regexp.MustCompile(
	`(?i)\b(?:base_?url|api_?base|endpoint_url|(?:OPENAI|ANTHROPIC|AZURE_OPENAI|OLLAMA)_(?:BASE_URL|API_BASE|HOST|ENDPOINT))["']?\s*[:=]\s*["']?(https?://[^"'\s,)]+)`)

// aiFileHint marks text as being about an AI client. "base_url" alone is
// common in ordinary HTTP clients, so a generic override only counts when an
// AI client is named close to it; the same file is not close enough, since a
// large file can configure both an AI client and an unrelated service.
var aiFileHint = regexp.MustCompile(`(?i)\b(?:openai|azureopenai|chatopenai|anthropic|ollama|litellm|mistral(?:ai)?|cohere|groq|gemini|langchain|llama_?index|vllm|chat\.completions)\b`)

// aiSpecificKey reports an override key that only an AI client reads.
func aiSpecificKey(match string) bool {
	upper := strings.ToUpper(match)
	return strings.HasPrefix(upper, "OPENAI_") || strings.HasPrefix(upper, "ANTHROPIC_") ||
		strings.HasPrefix(upper, "AZURE_OPENAI_") || strings.HasPrefix(upper, "OLLAMA_")
}

// baseURLs records AI clients pointed at a host other than a known
// vendor's: prompts and data then go to whoever runs that host.
func (s *scanner) baseURLs(f *textFile) {
	// A test or a document points clients at made-up hosts all the time;
	// only code and configuration say where data really goes.
	if f.context != contextCode {
		return
	}
	for _, loc := range baseURLOverride.FindAllStringSubmatchIndex(f.text, -1) {
		if !aiSpecificKey(f.text[loc[0]:loc[1]]) && !aiFileHint.MatchString(nearby(f.text, loc[0], loc[1])) {
			continue
		}
		host := hostOnly(f.text[loc[2]:loc[3]])
		if host == "" {
			continue
		}
		if _, isVendor := vendorProvider(host); isVendor {
			continue // the vendor host itself is already listed
		}
		s.redirected = true
		key := riskCustomEndpoint
		if aibom.IsPrivateHost(host) {
			key = riskLocalEndpoint
		}
		s.add(finding{
			component: aibom.Component{Kind: aibom.KindProvider, Name: aibom.CustomEndpointName(host), Properties: map[string]string{propContext: f.context, "host": host}},
			evidence:  aibom.Evidence{Path: f.path, Line: f.lineAt(loc[0]), Detail: "client base URL override"},
			risks:     []aibom.Risk{risk(key)},
		})
	}
}

// hostOnly returns host[:port] of a URL, dropping any user:password, path
// and query, which may carry credentials. Template placeholders give "".
func hostOnly(raw string) string {
	if strings.ContainsAny(raw, "${}<>%") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}

// hintWindow is how far either side of a base URL an AI client must be
// named: enough for a constructor call or a small configuration block.
const hintWindow = 400

// nearby returns the text within hintWindow bytes of [start, end).
func nearby(text string, start, end int) string {
	return text[max(0, start-hintWindow):min(len(text), end+hintWindow)]
}
