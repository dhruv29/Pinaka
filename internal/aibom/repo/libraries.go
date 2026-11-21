package repo

import (
	"regexp"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// pypiSeparators are the characters PEP 503 treats as the same.
var pypiSeparators = regexp.MustCompile(`[-_.]+`)

// normalisePackage puts a package name in the form the table uses.
func normalisePackage(eco, name string) string {
	name = strings.TrimSpace(name)
	switch eco {
	case ecoPyPI:
		return pypiSeparators.ReplaceAllString(strings.ToLower(name), "-")
	case ecoNPM, ecoCargo:
		return strings.ToLower(name)
	}
	return name
}

// lookupPackage finds name in the curated table for eco.
func lookupPackage(eco, name string) (aiPackage, bool) {
	for _, p := range aiPackages[eco] {
		if prefix, ok := strings.CutSuffix(p.name, "*"); ok {
			if strings.HasPrefix(name, prefix) {
				return p, true
			}
			continue
		}
		if name == p.name {
			return p, true
		}
	}
	return aiPackage{}, false
}

// dependency is one entry read from a manifest or lock file.
type dependency struct {
	eco     string
	name    string
	version string // as declared: "==1.2", "^1.2", "v1.2.3", "" when absent
	line    int
	// locked marks an exact version from a lock file, which is never
	// unpinned.
	locked bool
	// versionOptional marks ecosystems where a missing version means it is
	// managed elsewhere (a Maven parent or a Gradle platform), not floating.
	versionOptional bool
}

// library records dep if it is an AI package and ignores it otherwise, so
// that lock files and large manifests add nothing but AI.
func (s *scanner) library(f *textFile, dep dependency) {
	name := normalisePackage(dep.eco, dep.name)
	pkg, ok := lookupPackage(dep.eco, name)
	if !ok {
		return
	}
	version := cleanVersion(dep.version)
	detail := "dependency"
	if dep.locked {
		detail = "lock file entry"
	}
	found := finding{
		component: aibom.Component{
			Kind: aibom.KindLibrary, Name: name, Version: version, Supplier: pkg.supplier, Ecosystem: dep.eco,
			Properties: map[string]string{"category": pkg.category, propContext: f.context},
		},
		evidence: aibom.Evidence{Path: f.path, Line: dep.line, Detail: detail},
	}
	if !dep.locked && openEnded(version, dep.versionOptional) {
		found.risks = []aibom.Risk{risk(riskUnpinnedLibrary)}
	}
	s.add(found)
}

// cleanVersion removes spaces and an exact-pin operator, so "== 1.2" and
// "1.2" become the same version.
func cleanVersion(v string) string {
	v = strings.Join(strings.Fields(v), "")
	if rest, ok := strings.CutPrefix(v, "==="); ok {
		return rest
	}
	if rest, ok := strings.CutPrefix(v, "=="); ok && !strings.ContainsAny(rest, ",*<>") {
		return rest
	}
	return v
}

// floatingVersions are version words that always mean "whatever is newest".
var floatingVersions = setOf("*", "x", "latest", "release", "latest.release", "latest.integration", "next")

// openEnded reports whether a declared version lets new releases in without
// a bound: no version at all, a floating word, a ">=" with no "<", a Gradle
// "1.+" or a Maven "[1.0,)". Caret and tilde ranges have an upper bound and
// are not counted.
func openEnded(v string, optional bool) bool {
	lower := strings.ToLower(v)
	switch {
	case v == "":
		return !optional
	case floatingVersions[lower]:
		return true
	case strings.HasSuffix(v, "+"):
		return true
	case strings.HasSuffix(v, ",)"):
		return true
	case strings.Contains(v, ">") && !strings.Contains(v, "<"):
		return true
	}
	return false
}
