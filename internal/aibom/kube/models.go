package kube

import (
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

const maxModelLen = 256

var (
	modelNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]*$`)
	hfIDPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9._-]+$`)
	pinnedRevision   = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
)

// candidate is a model mention found in a container.
type candidate struct {
	value, detail string
	ollama        bool // pulled from the Ollama registry
	download      bool // explicitly downloaded at start
}

// modelContext is what a container says about how its models are fetched.
type modelContext struct {
	product  *product
	revision string
	trust    bool
	offline  bool
	served   string
}

func (a *analyzer) scanModels(s *scan, c container, tokens []string, prod *product, name string) {
	mc := modelContext{product: prod, trust: hasToken(tokens, trustRemoteCodeFlag)}
	for _, f := range revisionFlags {
		for _, v := range flagValues(tokens, f) {
			mc.revision = v
		}
	}
	for _, e := range c.Env {
		if e.ValueFrom != nil || e.Value == "" {
			continue
		}
		switch {
		case revisionEnvNames[e.Name]:
			mc.revision = e.Value
		case offlineEnvNames[e.Name]:
			mc.offline = e.Value == "1" || strings.EqualFold(e.Value, "true")
		}
	}
	mc.served = strings.Join(flagValues(tokens, servedNameFlag), ",")
	for _, cand := range modelCandidates(c, tokens, strings.HasPrefix(name, "initContainers")) {
		a.addModel(s, mc, cand, name)
	}
}

// modelCandidates gathers model mentions from flags, `vllm serve X`,
// `ollama pull X`, Hugging Face downloads and model-id environment values.
func modelCandidates(c container, tokens []string, init bool) []candidate {
	var out []candidate
	for _, f := range modelFlags {
		for _, v := range flagValues(tokens, f) {
			out = append(out, candidate{value: v, detail: "args " + f})
		}
	}
	for i := 0; i+2 < len(tokens); i++ {
		verb := path.Base(tokens[i]) + " " + tokens[i+1]
		arg := tokens[i+2]
		switch {
		case strings.HasPrefix(arg, "-"):
		case verb == "vllm serve":
			out = append(out, candidate{value: arg, detail: "command vllm serve"})
		case verb == "ollama pull" || verb == "ollama run":
			out = append(out, candidate{value: arg, detail: "command " + verb, ollama: true, download: true})
		case verb == "huggingface-cli download" || verb == "hf download":
			out = append(out, candidate{value: arg, detail: "command " + verb, download: true})
		}
	}
	for _, t := range tokens {
		if isHFURL(t) || (init && isModelURI(t)) {
			out = append(out, candidate{value: t, detail: "args model location", download: true})
		}
	}
	for _, e := range c.Env {
		if modelEnvNames[e.Name] && e.ValueFrom == nil && e.Value != "" {
			out = append(out, candidate{value: e.Value, detail: "env " + e.Name, ollama: e.Name == "OLLAMA_MODEL"})
		}
	}
	return out
}

func isHFURL(t string) bool {
	u, err := url.Parse(t)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host == "huggingface.co"
}

func isModelURI(t string) bool {
	for _, scheme := range modelURISchemes[:5] { // object stores and hf://, not plain web URLs
		if strings.HasPrefix(t, scheme) {
			return true
		}
	}
	return false
}

func (a *analyzer) addModel(s *scan, mc modelContext, cand candidate, name string) {
	ref, ok := classifyModel(cand, mc)
	if !ok {
		return
	}
	props := map[string]string{"source": ref.source, "served_as": mc.served}
	if mc.product != nil {
		props["served_by"] = mc.product.Name
	}
	if mc.trust {
		props["trust_remote_code"] = "true"
	}
	id := a.b.add(found{kind: aibom.KindModel, name: ref.name, version: ref.version, supplier: ref.supplier, ecosystem: ref.ecosystem, props: props},
		s.path, name+"."+cand.detail)
	s.models = append(s.models, modelHit{id: id, remote: ref.remote && !mc.offline, trust: mc.trust})
}

func (a *analyzer) addStorageModel(s *scan, m storageModel) {
	ref, ok := classifyModel(candidate{value: m.uri}, modelContext{product: catalogProduct("kserve")})
	if !ok {
		return
	}
	props := map[string]string{"source": ref.source, "served_by": "kserve", "format": m.format}
	id := a.b.add(found{kind: aibom.KindModel, name: ref.name, version: ref.version, supplier: ref.supplier, ecosystem: ref.ecosystem, props: props},
		s.path, m.field)
	s.models = append(s.models, modelHit{id: id, remote: ref.remote})
}

func catalogProduct(name string) *product {
	for i := range products {
		if products[i].Name == name {
			return &products[i]
		}
	}
	return nil
}

type modelRef struct {
	name, version, supplier, ecosystem, source string
	remote                                     bool
}

// classifyModel names a model and says where its weights come from. Values
// that do not look like a model id are dropped: silence beats noise.
func classifyModel(cand candidate, mc modelContext) (modelRef, bool) {
	v := strings.TrimSpace(cand.value)
	if v == "" || len(v) > maxModelLen || strings.HasPrefix(v, "$") {
		return modelRef{}, false
	}
	if strings.Contains(v, "://") {
		return classifyURI(v, cand, mc)
	}
	// "user:token@registry/model" or "path?sig=…" can carry a credential;
	// only the location is kept.
	if v = aibom.RedactLocation(v); v == "" {
		return modelRef{}, false
	}
	if strings.HasPrefix(v, "/") || strings.HasPrefix(v, "./") {
		return modelRef{name: clean(v), source: "local path"}, true
	}
	if !modelNamePattern.MatchString(v) {
		return modelRef{}, false
	}
	if cand.ollama || (mc.product != nil && mc.product.Name == "ollama") {
		name, tag, _ := strings.Cut(v, ":")
		return modelRef{name: name, version: tag, ecosystem: "ollama", source: "Ollama registry", remote: cand.download}, true
	}
	if !strings.Contains(v, "/") {
		for prefix, provider := range providerModelPrefixes {
			if strings.HasPrefix(v, prefix) {
				return modelRef{name: v, supplier: provider, source: "provider API"}, true
			}
		}
	}
	if hfIDPattern.MatchString(v) {
		return hfModel(v, mc.revision, cand.download || (mc.product != nil && mc.product.FetchesModels)), true
	}
	return modelRef{name: v, source: "named"}, true
}

func hfModel(id, revision string, fetched bool) modelRef {
	ref := modelRef{name: id, version: revision, supplier: strings.Split(id, "/")[0], ecosystem: "huggingface", source: "Hugging Face"}
	if fetched {
		ref.source = "Hugging Face (downloaded at start)"
		ref.remote = !pinnedRevision.MatchString(revision)
	}
	return ref
}

// classifyURI handles storage URIs. User info, query and fragment are
// dropped: they can carry credentials such as SAS tokens.
func classifyURI(v string, cand candidate, mc modelContext) (modelRef, bool) {
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return modelRef{}, false
	}
	if u.Scheme == "hf" || u.Host == "huggingface.co" {
		id := strings.Trim(u.Host+u.Path, "/")
		if u.Host == "huggingface.co" {
			id = strings.Join(firstN(strings.Split(strings.Trim(u.Path, "/"), "/"), 2), "/")
		}
		id, rev, _ := strings.Cut(id, ":")
		if !hfIDPattern.MatchString(id) {
			return modelRef{}, false
		}
		return hfModel(id, firstNonEmpty(rev, mc.revision), true), true
	}
	safe := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}
	return modelRef{name: clean(safe.String()), source: u.Scheme}, true
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
