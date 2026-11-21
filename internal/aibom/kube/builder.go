package kube

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

const (
	// maxListedWorkloads keeps risk summaries readable on big clusters.
	maxListedWorkloads = 5
	// maxPropertyValues bounds a property that many workloads contribute to.
	maxPropertyValues = 10
)

// found describes a component before it has evidence.
type found struct {
	kind                               aibom.Kind
	name, version, supplier, ecosystem string
	props                              map[string]string
}

type riskHit struct {
	raised  bool
	reasons map[string]map[string]bool // workload path → reasons
}

type entry struct {
	base     aibom.Component
	evidence map[aibom.Evidence]bool
	props    map[string]map[string]bool
	risks    map[string]*riskHit
}

// builder accumulates components. Everything is kept as sets and rendered
// sorted, so the output does not depend on input order.
type builder struct {
	entries map[string]*entry
}

func newBuilder() *builder { return &builder{entries: map[string]*entry{}} }

// add records a component seen at path/detail and returns its ID.
func (b *builder) add(f found, path, detail string) string {
	f.name, f.version = clean(f.name), clean(f.version)
	id := aibom.ComponentID(f.kind, f.name, f.version, aibom.OriginCluster)
	e, ok := b.entries[id]
	if !ok {
		e = &entry{evidence: map[aibom.Evidence]bool{}, props: map[string]map[string]bool{}, risks: map[string]*riskHit{}}
		e.base = aibom.Component{ID: id, Kind: f.kind, Name: f.name, Version: f.version, Origin: aibom.OriginCluster}
		b.entries[id] = e
	}
	// Names that differ only in case share an ID; keep the smallest so the
	// result does not depend on which was seen first.
	e.base.Name = minNonEmpty(e.base.Name, f.name)
	e.base.Supplier = minNonEmpty(e.base.Supplier, f.supplier)
	e.base.Ecosystem = minNonEmpty(e.base.Ecosystem, f.ecosystem)
	e.evidence[aibom.Evidence{Path: path, Detail: clean(detail)}] = true
	b.setProps(id, f.props)
	return id
}

func minNonEmpty(a, b string) string {
	if a == "" || (b != "" && b < a) {
		return b
	}
	return a
}

func (b *builder) setProps(id string, props map[string]string) {
	e := b.entries[id]
	for k, v := range props {
		if v == "" {
			continue
		}
		if e.props[k] == nil {
			e.props[k] = map[string]bool{}
		}
		e.props[k][clean(v)] = true
	}
}

// risk attaches a risk observation for one workload to a component.
func (b *builder) risk(id, riskID string, raised bool, path, reason string) {
	e := b.entries[id]
	h := e.risks[riskID]
	if h == nil {
		h = &riskHit{reasons: map[string]map[string]bool{}}
		e.risks[riskID] = h
	}
	h.raised = h.raised || raised
	if h.reasons[path] == nil {
		h.reasons[path] = map[string]bool{}
	}
	if reason != "" {
		h.reasons[path][clean(reason)] = true
	}
}

func (b *builder) bom(subject string, notes []string) aibom.BOM {
	out := aibom.BOM{Subject: subject, Components: []aibom.Component{}, Notes: sortedUnique(notes)}
	for _, e := range b.entries {
		out.Components = append(out.Components, e.render())
	}
	aibom.Sort(out.Components)
	return out
}

func (e *entry) render() aibom.Component {
	c := e.base
	for ev := range e.evidence {
		c.Evidence = append(c.Evidence, ev)
	}
	sort.Slice(c.Evidence, func(i, j int) bool {
		if c.Evidence[i].Path != c.Evidence[j].Path {
			return c.Evidence[i].Path < c.Evidence[j].Path
		}
		return c.Evidence[i].Detail < c.Evidence[j].Detail
	})
	if len(e.props) > 0 {
		c.Properties = map[string]string{}
		for k, set := range e.props {
			c.Properties[k] = joinLimited(sortedKeys(set), maxPropertyValues)
		}
	}
	for _, id := range sortedKeys(e.risks) {
		def := riskTable[id]
		h := e.risks[id]
		sev := def.Base
		if h.raised {
			sev = def.Raised
		}
		c.Risks = append(c.Risks, aibom.Risk{ID: id, Severity: sev, Summary: def.Summary + " Seen on " + h.where() + "."})
	}
	return c
}

func (h *riskHit) where() string {
	paths := sortedKeys(h.reasons)
	var parts []string
	for i, p := range paths {
		if i == maxListedWorkloads {
			parts = append(parts, fmt.Sprintf("and %d more", len(paths)-i))
			break
		}
		if reasons := sortedKeys(h.reasons[p]); len(reasons) > 0 {
			p += " (" + strings.Join(reasons, "; ") + ")"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, "; ")
}

func joinLimited(values []string, max int) string {
	if len(values) > max {
		return strings.Join(values[:max], ", ") + fmt.Sprintf(" and %d more", len(values)-max)
	}
	return strings.Join(values, ", ")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedUnique(values []string) []string {
	set := map[string]bool{}
	for _, v := range values {
		set[v] = true
	}
	if len(set) == 0 {
		return nil
	}
	return sortedKeys(set)
}
