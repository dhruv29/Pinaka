package repo

import (
	"fmt"
	"sort"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// Contexts say what kind of file a component was seen in. A model named in
// a README is still worth listing, but a reader should be able to tell it
// apart from one the code actually calls.
const (
	contextCode = "code"
	contextTest = "test"
	contextDocs = "docs"

	propContext = "context"
)

// contextRank orders contexts from weakest to strongest. A component seen
// in code and in docs is reported as code.
var contextRank = map[string]int{contextDocs: 1, contextTest: 2, contextCode: 3}

// finding is one detector result: the component as far as the detector
// knows it, where it was seen, and any risks that apply.
type finding struct {
	component aibom.Component
	evidence  aibom.Evidence
	risks     []aibom.Risk
}

// collector gathers findings into components, joining repeats and applying
// the component and evidence caps.
type collector struct {
	maxComponents int
	maxEvidence   int

	byID              map[string]*entry
	componentsDropped int
	evidenceCapped    map[string]bool
}

type entry struct {
	component aibom.Component
	seen      map[aibom.Evidence]bool
	risks     map[string]aibom.Risk
}

func newCollector(opts Options) *collector {
	return &collector{
		maxComponents:  opts.MaxComponents,
		maxEvidence:    opts.MaxEvidence,
		byID:           map[string]*entry{},
		evidenceCapped: map[string]bool{},
	}
}

// add records one finding. The ID is computed here so that every detector
// gets the same identity for the same kind, name and version.
func (c *collector) add(f finding) {
	comp := f.component
	comp.Origin = aibom.OriginRepo
	comp.ID = aibom.ComponentID(comp.Kind, comp.Name, comp.Version, comp.Origin)
	e, ok := c.byID[comp.ID]
	if !ok {
		if len(c.byID) >= c.maxComponents {
			c.componentsDropped++
			return
		}
		e = &entry{component: aibom.Component{
			ID: comp.ID, Kind: comp.Kind, Name: comp.Name, Version: comp.Version, Origin: comp.Origin,
		}, seen: map[aibom.Evidence]bool{}, risks: map[string]aibom.Risk{}}
		c.byID[comp.ID] = e
	}
	c.merge(e, comp, f)
}

func (c *collector) merge(e *entry, comp aibom.Component, f finding) {
	if e.component.Supplier == "" {
		e.component.Supplier = comp.Supplier
	}
	if e.component.Ecosystem == "" {
		e.component.Ecosystem = comp.Ecosystem
	}
	e.component.Properties = mergeProperties(e.component.Properties, comp.Properties)
	for _, r := range f.risks {
		e.risks[r.ID] = strongerRisk(e.risks[r.ID], r)
	}
	if e.seen[f.evidence] {
		return
	}
	if len(e.seen) >= c.maxEvidence {
		c.evidenceCapped[e.component.ID] = true
		return
	}
	e.seen[f.evidence] = true
}

// mergeProperties returns a new map with the keys of both. The first value
// seen for a key wins, except context, where the strongest wins.
func mergeProperties(have, add map[string]string) map[string]string {
	if len(add) == 0 {
		return have
	}
	out := make(map[string]string, len(have)+len(add))
	for k, v := range have {
		out[k] = v
	}
	for k, v := range add {
		old, ok := out[k]
		switch {
		case !ok:
			out[k] = v
		case k == propContext && contextRank[v] > contextRank[old]:
			out[k] = v
		}
	}
	return out
}

var severityRank = map[aibom.Severity]int{
	aibom.SeverityInfo: 1, aibom.SeverityLow: 2, aibom.SeverityMedium: 3,
	aibom.SeverityHigh: 4, aibom.SeverityCritical: 5,
}

// strongerRisk keeps the more severe of two risks with the same ID, so a
// raw pickle beside a torch checkpoint of the same name reports high.
func strongerRisk(have, add aibom.Risk) aibom.Risk {
	if severityRank[add.Severity] > severityRank[have.Severity] {
		return add
	}
	return have
}

// bom returns the sorted bill of materials and the notes the caps produced.
func (c *collector) bom(subject string, notes []string) aibom.BOM {
	out := aibom.BOM{Subject: subject, Components: []aibom.Component{}}
	for _, e := range c.byID {
		out.Components = append(out.Components, e.finish())
	}
	aibom.Sort(out.Components)
	out.Notes = append(append([]string{}, notes...), c.capNotes()...)
	sort.Strings(out.Notes)
	return out
}

func (e *entry) finish() aibom.Component {
	comp := e.component
	comp.Evidence = make([]aibom.Evidence, 0, len(e.seen))
	for ev := range e.seen {
		comp.Evidence = append(comp.Evidence, ev)
	}
	sort.Slice(comp.Evidence, func(i, j int) bool {
		a, b := comp.Evidence[i], comp.Evidence[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Detail < b.Detail
	})
	for _, r := range e.risks {
		comp.Risks = append(comp.Risks, r)
	}
	sort.Slice(comp.Risks, func(i, j int) bool { return comp.Risks[i].ID < comp.Risks[j].ID })
	return comp
}

func (c *collector) capNotes() []string {
	var notes []string
	if c.componentsDropped > 0 {
		notes = append(notes, fmt.Sprintf(
			"component cap of %d reached: %d further findings for new components were not recorded, so the inventory is incomplete",
			c.maxComponents, c.componentsDropped))
	}
	if n := len(c.evidenceCapped); n > 0 {
		notes = append(notes, fmt.Sprintf(
			"evidence cap of %d locations per component reached for %d components: only the first locations are listed",
			c.maxEvidence, n))
	}
	return notes
}
