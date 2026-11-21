package kube

import (
	"path"
	"sort"
	"strings"
	"sync"
)

const (
	maxImageLen    = 512
	dockerHub      = "docker.io"
	confidenceHigh = "high"
	// confidenceMirror marks an image whose path matches a known one on a
	// different registry: very likely a mirror, but not certain.
	confidenceMirror = "medium"
)

// imageRef is a parsed container image reference.
type imageRef struct {
	Registry, Path, Tag, Digest string
}

func (r imageRef) repo() string { return r.Registry + "/" + r.Path }

// unpinned is true for ":latest" or no tag, without a digest.
func (r imageRef) unpinned() bool {
	return r.Digest == "" && (r.Tag == "" || r.Tag == "latest")
}

// parseImage splits an image reference into registry, path, tag and digest,
// normalising Docker Hub short names the way the container runtime does.
func parseImage(s string) (imageRef, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxImageLen || strings.ContainsAny(s, " \t\r\n") {
		return imageRef{}, false
	}
	var ref imageRef
	if at := strings.Index(s, "@"); at >= 0 {
		s, ref.Digest = s[:at], s[at+1:]
	}
	if colon := strings.LastIndex(s, ":"); colon > strings.LastIndex(s, "/") {
		s, ref.Tag = s[:colon], s[colon+1:]
	}
	first, rest, found := strings.Cut(s, "/")
	if found && (strings.ContainsAny(first, ".:") || first == "localhost") {
		ref.Registry, ref.Path = strings.ToLower(first), rest
	} else {
		ref.Registry, ref.Path = dockerHub, s
	}
	switch ref.Registry {
	case "index.docker.io", "registry-1.docker.io":
		ref.Registry = dockerHub
	}
	if ref.Registry == dockerHub && !strings.Contains(ref.Path, "/") {
		ref.Path = "library/" + ref.Path
	}
	ref.Path = strings.ToLower(ref.Path)
	if ref.Path == "" || strings.HasSuffix(ref.Path, "/") {
		return imageRef{}, false
	}
	return ref, true
}

type prefixEntry struct {
	prefix string
	p      *product
}

type catalogIndex struct {
	exact    map[string]*product
	paths    []prefixEntry // exact entries keyed by path, for mirrors
	prefixes []prefixEntry // longest first
	kinds    map[string][]*product
}

var catalog = sync.OnceValue(buildCatalog)

func buildCatalog() catalogIndex {
	ix := catalogIndex{exact: map[string]*product{}, kinds: map[string][]*product{}}
	for i := range products {
		p := &products[i]
		for _, img := range p.Images {
			ix.exact[img] = p
			_, pth, _ := strings.Cut(img, "/")
			ix.paths = append(ix.paths, prefixEntry{pth, p})
		}
		for _, pre := range p.Prefixes {
			ix.prefixes = append(ix.prefixes, prefixEntry{pre, p})
		}
		for _, k := range p.Kinds {
			ix.kinds[k] = append(ix.kinds[k], p)
		}
	}
	sort.SliceStable(ix.prefixes, func(i, j int) bool { return len(ix.prefixes[i].prefix) > len(ix.prefixes[j].prefix) })
	sort.SliceStable(ix.paths, func(i, j int) bool { return ix.paths[i].prefix < ix.paths[j].prefix })
	return ix
}

// matchImage finds the product an image belongs to. Repositories are
// matched exactly or by a known prefix, never by substring: an image called
// "ollama-exporter" is not Ollama.
func matchImage(ref imageRef) (*product, string, bool) {
	ix := catalog()
	repo := ref.repo()
	if p, ok := ix.exact[repo]; ok {
		return p, confidenceHigh, true
	}
	for _, e := range ix.prefixes {
		if strings.HasPrefix(repo, e.prefix) {
			return e.p, confidenceHigh, true
		}
	}
	for _, e := range ix.paths {
		if ref.Path == e.prefix || strings.HasSuffix(ref.Path, "/"+e.prefix) {
			return e.p, confidenceMirror, true
		}
	}
	return nil, "", false
}

// matchCommand finds products whose command signature appears in tokens.
func matchCommand(tokens []string) []*product {
	var out []*product
	for i := range products {
		p := &products[i]
		for _, sig := range p.Commands {
			if containsSequence(tokens, sig) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func containsSequence(tokens, sig []string) bool {
	for i := 0; i+len(sig) <= len(tokens); i++ {
		if path.Base(tokens[i]) != sig[0] {
			continue
		}
		match := true
		for j := 1; j < len(sig); j++ {
			if tokens[i+j] != sig[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
