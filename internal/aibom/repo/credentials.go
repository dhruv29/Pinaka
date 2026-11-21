package repo

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// credentialName matches any name in the shared table as a whole word, in
// any case, so `openai_api_key = "…"` counts as well as OPENAI_API_KEY.
// Underscore counts as part of a word, so AZURE_OPENAI_API_KEY never also
// matches OPENAI_API_KEY.
var credentialName = func() *regexp.Regexp {
	names := make([]string, 0, len(aibom.Credentials))
	for name := range aibom.Credentials {
		names = append(names, regexp.QuoteMeta(name))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return regexp.MustCompile(`(?i)\b(` + strings.Join(names, "|") + `)\b`)
}()

// credentialSupplier returns who issues the key a credential name is for.
func credentialSupplier(name string) string {
	return aibom.Credentials[name].Supplier
}

// assignedValue is what follows a credential name in an assignment:
// `NAME=value`, `NAME: value`, `"NAME": "value"`, `NAME := "value"`.
var assignedValue = regexp.MustCompile(`^["']?\s*(?::=|=|:)\s*["']?([^\s"',;)}\]]*)`)

// credentialNames records each credential name and flags a line that
// assigns it a literal that looks like a real key. The value is examined
// in place and never stored.
func (s *scanner) credentialNames(f *textFile) {
	for _, loc := range credentialName.FindAllStringSubmatchIndex(f.text, -1) {
		name := strings.ToUpper(f.text[loc[2]:loc[3]])
		rest := f.text[loc[3]:min(len(f.text), loc[3]+maxAssignmentBytes)]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[:nl]
		}
		s.credential(f, loc[2], name, literalAssigned(rest))
	}
	s.keyArguments(f)
}

const (
	maxAssignmentBytes = 400
	minKeyLength       = 20
	minKeyEntropy      = 3.5
)

// credential records one credential name; hardcoded adds the high risk.
func (s *scanner) credential(f *textFile, offset int, name string, hardcoded bool) {
	found := finding{
		component: aibom.Component{Kind: aibom.KindCredential, Name: name, Supplier: credentialSupplier(name), Properties: map[string]string{propContext: f.context}},
		evidence:  aibom.Evidence{Path: f.path, Line: f.lineAt(offset), Detail: "credential referenced by name"},
	}
	if hardcoded {
		found.evidence.Detail = "literal value assigned here (value not recorded)"
		found.risks = []aibom.Risk{risk(riskHardcodedCred)}
	}
	s.add(found)
	s.inferProvider(f, found.evidence.Line, aibom.Credentials[name].Provider, "the credential name "+name)
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

// keyArgument is a key passed to a client as a quoted literal:
// `OpenAI(api_key="…")`, `new Anthropic({ apiKey: "…" })`, `"api_key": "…"`.
var keyArgument = regexp.MustCompile(`(?i)\b(?:api[_-]?key)["']?\s*[:=]\s*["']([^"'\s]{20,400})["']`)

// keyVendor is a key prefix that names who issued the key.
type keyVendor struct {
	prefix, supplier string
}

// keyVendors are checked in order, so the longer "sk-ant-" wins over "sk-".
var keyVendors = []keyVendor{
	{"sk-ant-", "Anthropic"}, {"sk-or-", "OpenRouter"}, {"sk-", "OpenAI"}, {"hf_", "Hugging Face"},
	{"AIza", "Google"}, {"gsk_", "Groq"}, {"xai-", "xAI"}, {"pcsk_", "Pinecone"}, {"r8_", "Replicate"},
	{"nvapi-", "NVIDIA"},
}

// genericKeyName names a literal key whose issuer cannot be told from its
// prefix but that is passed to an AI client on the same line.
const genericKeyName = "AI API key (literal)"

// keyArguments flags keys written into a client call or a configuration
// entry under a generic name such as api_key. The credential is recorded
// under the issuer's name; the value is examined in place and never stored.
func (s *scanner) keyArguments(f *textFile) {
	for _, loc := range keyArgument.FindAllStringSubmatchIndex(f.text, -1) {
		value := f.text[loc[2]:loc[3]]
		if !looksLikeKey(value) {
			continue
		}
		supplier := vendorOfKey(value)
		name := supplier + " API key (literal)"
		if supplier == "" {
			// Without a vendor prefix the key is only about AI when an AI
			// client is named on the same line; api_key is a common name.
			if !aiFileHint.MatchString(lineAround(f.text, loc[0])) {
				continue
			}
			name = genericKeyName
		}
		s.add(finding{
			component: aibom.Component{Kind: aibom.KindCredential, Name: name, Supplier: supplier, Properties: map[string]string{propContext: f.context}},
			evidence:  aibom.Evidence{Path: f.path, Line: f.lineAt(loc[0]), Detail: "literal key passed as an argument (value not recorded)"},
			risks:     []aibom.Risk{risk(riskHardcodedCred)},
		})
	}
}

func vendorOfKey(value string) string {
	for _, v := range keyVendors {
		if strings.HasPrefix(value, v.prefix) {
			return v.supplier
		}
	}
	return ""
}

// lineAround returns the line holding offset.
func lineAround(text string, offset int) string {
	start := strings.LastIndexByte(text[:offset], '\n') + 1
	end := strings.IndexByte(text[offset:], '\n')
	if end < 0 {
		return text[start:]
	}
	return text[start : offset+end]
}
