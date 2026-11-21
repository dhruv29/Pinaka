package repo

import (
	"path"
	"regexp"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// modelFamily is a hosted model family: who publishes it and the shape of
// its identifiers. Patterns are matched against a whole string literal, so
// "o3" inside "pseudo3" or "claude" inside prose never counts.
type modelFamily struct {
	supplier string
	pattern  string
}

// modelFamilies is the table of recognised hosted model identifiers.
var modelFamilies = []modelFamily{
	{"OpenAI", `gpt-(?:3\.5|4|4o|4\.1|4\.5|5|oss)(?:[-.][a-z0-9]+)*`},
	{"OpenAI", `o[1-9](?:-(?:mini|pro|preview|high|medium|low|deep-research|[0-9]{4}-[0-9]{2}-[0-9]{2}))*`},
	{"OpenAI", `chatgpt-4o-latest|gpt-image-1|dall-e-[23]|whisper-1|tts-1(?:-hd)?|text-embedding-(?:3-small|3-large|ada-002)`},
	{"Anthropic", `claude-(?:instant|opus|sonnet|haiku|[0-9])[a-z0-9.@-]*`},
	{"Google", `gemini-(?:[0-9](?:\.[0-9])?|pro|ultra|nano|exp|embedding)[a-z0-9.-]*|(?:text|chat)-bison[a-z0-9@-]*|text-embedding-00[0-9]`},
	{"Google", `gemma-?[0-9][a-z0-9.:-]*`},
	{"Mistral AI", `(?:open-)?(?:mistral|mixtral|codestral|ministral|pixtral|magistral|devstral)-[a-z0-9.-]+`},
	{"Meta", `(?:meta-)?llama-?[0-9](?:\.[0-9])?[a-z0-9.:-]*|codellama[a-z0-9.:-]*`},
	{"Cohere", `command-(?:r|a|light|nightly|xlarge)[a-z0-9.-]*|embed-(?:english|multilingual)(?:-light)?-v[0-9]\.[0-9]|rerank-(?:english|multilingual|v[0-9])[a-z0-9.-]*`},
	{"xAI", `grok-(?:[0-9]|beta|vision)[a-z0-9.-]*`},
	{"DeepSeek", `deepseek-(?:chat|coder|reasoner|r1|v[0-9]|llm|math|vl)[a-z0-9.:-]*`},
	{"Alibaba", `qwen(?:[0-9](?:\.[0-9])?|-)[a-z0-9.:-]*`},
}

// compiledFamily is a family with its pattern anchored to a whole literal.
type compiledFamily struct {
	supplier string
	re       *regexp.Regexp
}

// bedrockVersion allows the "-v1:0" suffix Amazon Bedrock adds to ids.
const bedrockVersion = `(?:-v[0-9]+(?::[0-9]+)?)?`

var compiledFamilies = func() []compiledFamily {
	out := make([]compiledFamily, 0, len(modelFamilies))
	for _, fam := range modelFamilies {
		out = append(out, compiledFamily{fam.supplier, regexp.MustCompile(`(?i)^(?:` + fam.pattern + `)` + bedrockVersion + `$`)})
	}
	return out
}()

var (
	// stringLiteral is a quoted token with no spaces: the only place a
	// model identifier is looked for.
	stringLiteral = regexp.MustCompile("[\"'`]([A-Za-z0-9][A-Za-z0-9._:@/-]{1,120})[\"'`]")
	// routePrefix is a router or SDK prefix such as "openai/" (LiteLLM) or
	// "models/" (Gemini) that is not part of the model's name.
	routePrefix = regexp.MustCompile(`(?i)^(openai|anthropic|models|google|gemini|vertex_ai|mistral|groq|together_ai|openrouter|bedrock|azure|ollama|deepseek|xai|cohere|fireworks_ai)/`)
	// bedrockPrefix is the provider part of an Amazon Bedrock model id.
	bedrockPrefix = regexp.MustCompile(`(?i)^(?:(?:us|eu|apac|global)\.)?(?:anthropic|meta|mistral|cohere|deepseek)\.`)
	// providerModel is the "provider:model" form some frameworks use.
	providerModel = regexp.MustCompile(`(?i)^(openai|anthropic|google-gla|google-vertex|gemini|groq|mistral|cohere|bedrock|deepseek|xai|azure):(.+)$`)
)

// shortModelName is a model identifier so short ("o1", "o3") that the same
// two characters turn up as an ordinary key or label. It only counts in a
// file that also mentions the vendor.
var shortModelName = regexp.MustCompile(`(?i)^o[1-9]$`)

// openAIHint marks a file as using OpenAI.
var openAIHint = regexp.MustCompile(`(?i)\bopen_?ai\b`)

// hostedModel returns the canonical name and supplier of a literal that is
// a hosted model identifier.
func hostedModel(literal string) (name, supplier string, ok bool) {
	name = literal
	if m := providerModel.FindStringSubmatch(name); m != nil {
		name = m[2]
	}
	name = routePrefix.ReplaceAllString(name, "")
	bare := bedrockPrefix.ReplaceAllString(name, "")
	for _, fam := range compiledFamilies {
		if fam.re.MatchString(bare) {
			return name, fam.supplier, true
		}
	}
	return "", "", false
}

// isFloatingAlias reports names that move to a new model over time.
func isFloatingAlias(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, "-latest") || strings.HasSuffix(lower, ":latest")
}

// namedModels records hosted model identifiers in string literals.
func (s *scanner) namedModels(f *textFile) {
	for _, loc := range stringLiteral.FindAllStringSubmatchIndex(f.text, -1) {
		name, supplier, ok := hostedModel(f.text[loc[2]:loc[3]])
		if !ok {
			continue
		}
		if shortModelName.MatchString(name) && !openAIHint.MatchString(f.text) {
			continue
		}
		s.addModel(f, loc[0], aibom.Component{Name: name, Supplier: supplier}, "model named in a string literal")
	}
}

// addModel records a model with the file's context and the floating-alias
// risk when the name is one.
func (s *scanner) addModel(f *textFile, offset int, comp aibom.Component, detail string, risks ...aibom.Risk) {
	comp.Kind = aibom.KindModel
	comp.Properties = mergeProperties(comp.Properties, map[string]string{propContext: f.context})
	if isFloatingAlias(comp.Name) {
		risks = append(risks, risk(riskUnpinnedModel))
	}
	s.add(finding{
		component: comp,
		evidence:  aibom.Evidence{Path: f.path, Line: f.lineAt(offset), Detail: detail},
		risks:     risks,
	})
}

var (
	// ollamaModel is `model: "name:tag"` or `"model": "name:tag"`, the way
	// Ollama clients and configs name a local model.
	ollamaModel = regexp.MustCompile(`(?i)["']?\bmodel["']?\s*[:=]\s*["']([a-z0-9][a-z0-9._/-]*:[a-z0-9][a-z0-9._-]*)["']`)
	// modelfileFrom is the FROM line of an Ollama Modelfile.
	modelfileFrom = regexp.MustCompile(`(?im)^\s*FROM\s+(\S+)`)
)

// ollamaModels records name:tag models and Modelfile FROM lines.
func (s *scanner) ollamaModels(f *textFile) {
	for _, loc := range ollamaModel.FindAllStringSubmatchIndex(f.text, -1) {
		value := f.text[loc[2]:loc[3]]
		if name, supplier, ok := hostedModel(value); ok && providerModel.MatchString(value) {
			s.addModel(f, loc[0], aibom.Component{Name: name, Supplier: supplier}, "model named in configuration")
			continue
		}
		_, supplier, _ := hostedModel(value)
		s.addModel(f, loc[0], aibom.Component{Name: value, Supplier: supplier, Ecosystem: ecoOllama}, "Ollama model")
	}
	if !isModelfile(f.path) {
		return
	}
	for _, loc := range modelfileFrom.FindAllStringSubmatchIndex(f.text, -1) {
		s.modelfileBase(f, loc[0], f.text[loc[2]:loc[3]])
	}
}

func isModelfile(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	return base == "modelfile" || strings.HasSuffix(base, ".modelfile")
}

// modelfileBase records the base model of a Modelfile: either a weights file
// on disk or a model from the Ollama library. A library model without a tag
// follows "latest".
func (s *scanner) modelfileBase(f *textFile, offset int, base string) {
	// A base can be a URL or a registry reference carrying a password or a
	// signed query; only the location is kept.
	if base = aibom.RedactLocation(base); base == "" {
		return
	}
	if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "/") || strings.Contains(base, ".gguf") {
		s.addModel(f, offset, aibom.Component{Name: base, Properties: map[string]string{"reference": "file"}}, "Modelfile base")
		return
	}
	var risks []aibom.Risk
	if !strings.Contains(base, ":") {
		risks = append(risks, risk(riskUnpinnedModel))
	}
	_, supplier, _ := hostedModel(base)
	s.addModel(f, offset, aibom.Component{Name: base, Supplier: supplier, Ecosystem: ecoOllama}, "Modelfile base", risks...)
}
