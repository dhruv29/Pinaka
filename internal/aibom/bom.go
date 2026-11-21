// Package aibom describes an AI bill of materials: an inventory of the
// models, AI libraries, model-serving software, data stores, providers,
// agent tooling and credentials-by-name that a codebase or a running cluster
// contains, each with the evidence for it and any risk it carries.
//
// The scanners live in sub-packages (repo, kube) and all produce the types
// here, so one report can combine what the code declares with what is
// actually running.
package aibom

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Kind says what a component is.
type Kind string

const (
	// KindModel is a model: a weights file, or a model named in code,
	// configuration or a serving command.
	KindModel Kind = "model"
	// KindLibrary is an AI or machine-learning library a project depends on.
	KindLibrary Kind = "library"
	// KindProvider is a hosted AI service the code or workload calls.
	KindProvider Kind = "provider"
	// KindServing is software that serves or routes model inference.
	KindServing Kind = "serving"
	// KindDataStore is a vector database or other store built for AI data.
	KindDataStore Kind = "data_store"
	// KindDataset is a dataset the code loads.
	KindDataset Kind = "dataset"
	// KindAgentTool is agent tooling: a tool server, an agent framework
	// configuration, or instructions an agent reads.
	KindAgentTool Kind = "agent_tool"
	// KindCredential is the NAME of a credential for an AI service. The
	// value is never recorded.
	KindCredential Kind = "credential"
	// KindAccelerator is a hardware request such as a GPU.
	KindAccelerator Kind = "accelerator"
	// KindPlatform is machine-learning platform software: training,
	// pipelines, notebooks, experiment tracking.
	KindPlatform Kind = "platform"
)

// Severity ranks a risk. The values match the finding severities used
// elsewhere, so reports can sort them the same way.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Evidence is one place a component was seen. For code, Path is relative to
// the scanned root and Line is 1-based (0 when not applicable). For a
// cluster, Path is "namespace/Kind/name" and Detail names the field.
type Evidence struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Risk is something about a component that a security team should look at.
// It is an observation with its reason, not a proven vulnerability.
type Risk struct {
	// ID is a short stable slug such as "pickle-model-file".
	ID       string   `json:"id"`
	Severity Severity `json:"severity"`
	// Summary is one sentence saying what was seen and why it matters.
	Summary string `json:"summary"`
	// Where is the place the risk was raised, when that is more exact
	// than the component's evidence as a whole.
	Where *Evidence `json:"where,omitempty"`
}

// Component is one entry in the bill of materials.
type Component struct {
	// ID is stable for the same kind, name, version and origin.
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
	// Name is the canonical name: a package name, a model identifier, a
	// provider name, an image repository.
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	// Supplier is who publishes it, when that can be told from the name.
	Supplier string `json:"supplier,omitempty"`
	// Ecosystem is the package ecosystem or registry: pypi, npm, go, cargo,
	// maven, huggingface, oci, ollama.
	Ecosystem string `json:"ecosystem,omitempty"`
	// Origin is where the component was seen: "repo", "cluster", or
	// "repo+cluster" when a merged bill found it in both.
	Origin   string     `json:"origin"`
	Evidence []Evidence `json:"evidence"`
	Risks    []Risk     `json:"risks,omitempty"`
	// Properties holds extra facts, for example "format": "gguf" or
	// "exposed": "LoadBalancer".
	Properties map[string]string `json:"properties,omitempty"`
}

// BOM is the result of one scan, or of several merged.
type BOM struct {
	// Subject names what was scanned: a directory or a cluster context.
	Subject    string      `json:"subject"`
	Components []Component `json:"components"`
	// Notes say what could not be examined, so a gap is never mistaken for
	// an absence.
	Notes []string `json:"notes,omitempty"`
}

const (
	// OriginRepo marks a component found in source code.
	OriginRepo = "repo"
	// OriginCluster marks a component found in a running cluster.
	OriginCluster = "cluster"
	// OriginBoth marks a component a merged bill found in the code and in
	// the cluster.
	OriginBoth = OriginRepo + originSeparator + OriginCluster

	originSeparator = "+"

	idBytes = 8
)

// ComponentID returns the stable identifier for a component. It does not
// depend on where the component was seen, so the same model or key found
// in the code and in the cluster is one component in a merged bill.
func ComponentID(kind Kind, name, version string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{string(kind), strings.ToLower(name), version}, "\x00")))
	return hex.EncodeToString(sum[:idBytes])
}

// JoinOrigins returns the origin of a component seen in both a and b:
// "repo", "cluster" or "repo+cluster". Origins it does not know are kept,
// sorted, after the known ones.
func JoinOrigins(a, b string) string {
	seen := map[string]bool{}
	for _, origin := range []string{a, b} {
		for _, part := range strings.Split(origin, originSeparator) {
			if part != "" {
				seen[part] = true
			}
		}
	}
	var known, other []string
	for _, origin := range []string{OriginRepo, OriginCluster} {
		if seen[origin] {
			known = append(known, origin)
			delete(seen, origin)
		}
	}
	for origin := range seen {
		other = append(other, origin)
	}
	sort.Strings(other)
	return strings.Join(append(known, other...), originSeparator)
}

// Merge combines boms into one. Components with the same ID are joined:
// their evidence, risks and properties are united. The result is sorted, so
// the same inputs always give the same output. The inputs are not changed.
func Merge(subject string, boms ...BOM) BOM {
	byID := map[string]Component{}
	out := BOM{Subject: subject, Components: []Component{}}
	for _, bom := range boms {
		out.Notes = append(out.Notes, bom.Notes...)
		for _, component := range bom.Components {
			byID[component.ID] = join(byID[component.ID], component)
		}
	}
	for _, component := range byID {
		out.Components = append(out.Components, component)
	}
	Sort(out.Components)
	return out
}

// join returns a new component holding everything in a and b.
func join(a, b Component) Component {
	if a.ID == "" {
		a = Component{ID: b.ID, Kind: b.Kind, Name: b.Name, Version: b.Version, Supplier: b.Supplier, Ecosystem: b.Ecosystem, Origin: b.Origin}
	}
	merged := a
	merged.Origin = JoinOrigins(a.Origin, b.Origin)
	merged.Supplier = smallestNonEmpty(a.Supplier, b.Supplier)
	merged.Ecosystem = smallestNonEmpty(a.Ecosystem, b.Ecosystem)
	merged.Evidence = uniqueEvidence(append(append([]Evidence{}, a.Evidence...), b.Evidence...))
	merged.Risks = uniqueRisks(append(append([]Risk{}, a.Risks...), b.Risks...))
	merged.Properties = map[string]string{}
	// When the scans disagree on a value the smaller wins, so the result
	// does not depend on the order of the inputs.
	for _, source := range []map[string]string{a.Properties, b.Properties} {
		for key, value := range source {
			merged.Properties[key] = smallestNonEmpty(merged.Properties[key], value)
		}
	}
	if len(merged.Properties) == 0 {
		merged.Properties = nil
	}
	return merged
}

func smallestNonEmpty(a, b string) string {
	if a == "" || (b != "" && b < a) {
		return b
	}
	return a
}

func uniqueEvidence(all []Evidence) []Evidence {
	seen := map[Evidence]bool{}
	out := []Evidence{}
	for _, e := range all {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Detail < out[j].Detail
	})
	return out
}

// uniqueRisks drops exact repeats only. Two scans can raise the same kind of
// risk about one component for different reasons, and both reasons are kept.
func uniqueRisks(all []Risk) []Risk {
	seen := map[riskKey]bool{}
	out := []Risk{}
	for _, r := range all {
		if k := keyOf(r); !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := keyOf(out[i]), keyOf(out[j])
		if a.id != b.id {
			return a.id < b.id
		}
		if a.summary != b.summary {
			return a.summary < b.summary
		}
		if a.where != b.where {
			return place(a.where) < place(b.where)
		}
		return a.severity < b.severity
	})
	return out
}

// riskKey compares risks by value: Where is a pointer, and two risks raised
// at equal places are the same risk.
type riskKey struct {
	id, summary string
	severity    Severity
	hasWhere    bool
	where       Evidence
}

func keyOf(r Risk) riskKey {
	k := riskKey{id: r.ID, summary: r.Summary, severity: r.Severity}
	if r.Where != nil {
		k.hasWhere, k.where = true, *r.Where
	}
	return k
}

// Sort orders components by kind, then name, version and origin.
func Sort(components []Component) {
	sort.Slice(components, func(i, j int) bool {
		a, b := components[i], components[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Version != b.Version {
			return a.Version < b.Version
		}
		return a.Origin < b.Origin
	})
}
