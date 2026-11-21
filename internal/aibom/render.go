package aibom

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	cycloneDXFormat  = "CycloneDX"
	cycloneDXVersion = "1.6"
	propertyPrefix   = "pinaka:"
	// maxEvidenceShown keeps the readable report short; the JSON holds all.
	maxEvidenceShown = 5
)

// kindTitles gives each kind its heading, in the order the report shows them:
// what runs and thinks first, then what it depends on, then what it needs.
var kindTitles = []struct {
	kind  Kind
	title string
}{
	{KindModel, "Models"},
	{KindServing, "Model serving"},
	{KindProvider, "Hosted AI providers"},
	{KindAgentTool, "Agent tooling"},
	{KindLibrary, "AI libraries"},
	{KindDataStore, "AI data stores"},
	{KindDataset, "Datasets"},
	{KindPlatform, "ML platforms"},
	{KindAccelerator, "Accelerators"},
	{KindCredential, "Credentials (names only)"},
}

// severityRank orders risks, worst first.
var severityRank = map[Severity]int{SeverityCritical: 0, SeverityHigh: 1, SeverityMedium: 2, SeverityLow: 3, SeverityInfo: 4}

// cycloneDXTypes maps a kind to the closest CycloneDX component type. The
// exact kind is kept as a property, so nothing is lost in the mapping.
var cycloneDXTypes = map[Kind]string{
	KindModel:       "machine-learning-model",
	KindLibrary:     "library",
	KindServing:     "application",
	KindDataStore:   "application",
	KindPlatform:    "platform",
	KindAgentTool:   "application",
	KindDataset:     "data",
	KindCredential:  "data",
	KindAccelerator: "device",
}

// Flag pairs a risk with the component it belongs to.
type Flag struct {
	Risk      Risk
	Component Component
}

// Flagged returns every risk in the bill with its component, worst first.
func Flagged(bom BOM) []Flag {
	var out []Flag
	for _, component := range bom.Components {
		for _, risk := range component.Risks {
			out = append(out, Flag{Risk: risk, Component: component})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if severityRank[a.Risk.Severity] != severityRank[b.Risk.Severity] {
			return severityRank[a.Risk.Severity] < severityRank[b.Risk.Severity]
		}
		if a.Risk.ID != b.Risk.ID {
			return a.Risk.ID < b.Risk.ID
		}
		return a.Component.Name < b.Component.Name
	})
	return out
}

// WorstSeverity returns the most serious risk severity in the bill, or the
// empty string when nothing is flagged.
func WorstSeverity(bom BOM) Severity {
	all := Flagged(bom)
	if len(all) == 0 {
		return ""
	}
	return all[0].Risk.Severity
}

// AtLeast reports whether got is as serious as threshold or worse.
func AtLeast(got, threshold Severity) bool {
	gotRank, known := severityRank[got]
	thresholdRank, valid := severityRank[threshold]
	return known && valid && gotRank <= thresholdRank
}

// Markdown renders the bill for a person: the risks first, then the
// inventory by kind, then what could not be examined.
func Markdown(bom BOM) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# AI bill of materials: %s\n\n", cell(bom.Subject))
	writeSummary(&b, bom)
	writeRisks(&b, bom)
	for _, section := range kindTitles {
		writeSection(&b, section.title, ofKind(bom, section.kind))
	}
	if len(bom.Notes) > 0 {
		b.WriteString("## Not examined\n\nThese gaps mean the inventory may be incomplete.\n\n")
		for _, note := range bom.Notes {
			fmt.Fprintf(&b, "- %s\n", cell(note))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func writeSummary(b *strings.Builder, bom BOM) {
	if len(bom.Components) == 0 {
		b.WriteString("No AI components were found.\n\n")
		return
	}
	parts := []string{}
	for _, section := range kindTitles {
		if n := len(ofKind(bom, section.kind)); n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, strings.ToLower(section.title)))
		}
	}
	fmt.Fprintf(b, "%d component(s): %s. %d risk(s) flagged.\n\n", len(bom.Components), strings.Join(parts, ", "), len(Flagged(bom)))
}

func writeRisks(b *strings.Builder, bom BOM) {
	all := Flagged(bom)
	if len(all) == 0 {
		return
	}
	b.WriteString("## Look at these first\n\nEach is an observation with its reason, not a proven vulnerability.\n\n")
	b.WriteString("| Severity | Component | What was seen | Where |\n|---|---|---|---|\n")
	for _, f := range all {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", f.Risk.Severity, cell(label(f.Component)), cell(f.Risk.Summary), cell(riskPlace(f)))
	}
	b.WriteString("\n")
}

func writeSection(b *strings.Builder, title string, components []Component) {
	if len(components) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n\n| Name | Version | Found in | Seen at |\n|---|---|---|---|\n", title)
	for _, c := range components {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", cell(c.Name), cell(orDash(c.Version)), cell(c.Origin), cell(places(c)))
	}
	b.WriteString("\n")
}

func ofKind(bom BOM, kind Kind) []Component {
	var out []Component
	for _, c := range bom.Components {
		if c.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

func label(c Component) string {
	if c.Version == "" {
		return c.Name
	}
	return c.Name + " " + c.Version
}

func place(e Evidence) string {
	out := e.Path
	if e.Line > 0 {
		out += fmt.Sprintf(":%d", e.Line)
	}
	if e.Detail != "" {
		out += " (" + e.Detail + ")"
	}
	return out
}

// riskPlace is where a risk was raised, or else where its component was
// first seen.
func riskPlace(f Flag) string {
	if f.Risk.Where != nil {
		return place(*f.Risk.Where)
	}
	return firstPlace(f.Component)
}

func firstPlace(c Component) string {
	if len(c.Evidence) == 0 {
		return ""
	}
	return place(c.Evidence[0])
}

func places(c Component) string {
	shown := []string{}
	for i, e := range c.Evidence {
		if i == maxEvidenceShown {
			shown = append(shown, fmt.Sprintf("and %d more", len(c.Evidence)-maxEvidenceShown))
			break
		}
		shown = append(shown, place(e))
	}
	return strings.Join(shown, "; ")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// cell makes text safe inside a Markdown table: what was scanned is not
// trusted, and a name must not be able to add rows, links or markup.
func cell(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	return strings.NewReplacer(`\`, `\\`, "|", `\|`, "<", "&lt;", ">", "&gt;", "[", `\[`, "]", `\]`, "`", "\\`", "*", `\*`, "_", `\_`).Replace(s)
}

type cdxProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type cdxComponent struct {
	Type       string        `json:"type"`
	BOMRef     string        `json:"bom-ref"`
	Name       string        `json:"name"`
	Version    string        `json:"version,omitempty"`
	Supplier   *cdxSupplier  `json:"supplier,omitempty"`
	Properties []cdxProperty `json:"properties,omitempty"`
}

type cdxSupplier struct {
	Name string `json:"name"`
}

type cdxService struct {
	BOMRef     string        `json:"bom-ref"`
	Name       string        `json:"name"`
	Properties []cdxProperty `json:"properties,omitempty"`
}

type cdxDocument struct {
	BOMFormat   string         `json:"bomFormat"`
	SpecVersion string         `json:"specVersion"`
	Version     int            `json:"version"`
	Metadata    cdxMetadata    `json:"metadata"`
	Components  []cdxComponent `json:"components"`
	Services    []cdxService   `json:"services,omitempty"`
}

type cdxMetadata struct {
	Component  cdxComponent  `json:"component"`
	Properties []cdxProperty `json:"properties,omitempty"`
}

// CycloneDX renders the bill as a CycloneDX document, the format other
// inventory and compliance tools read. Hosted providers become services;
// everything else becomes a component. Kinds, evidence and risks that the
// format has no field for are carried as properties named "pinaka:...".
func CycloneDX(bom BOM) ([]byte, error) {
	doc := cdxDocument{
		BOMFormat: cycloneDXFormat, SpecVersion: cycloneDXVersion, Version: 1,
		Metadata:   cdxMetadata{Component: cdxComponent{Type: "application", BOMRef: "subject", Name: bom.Subject}},
		Components: []cdxComponent{},
	}
	for _, note := range bom.Notes {
		doc.Metadata.Properties = append(doc.Metadata.Properties, cdxProperty{Name: propertyPrefix + "note", Value: note})
	}
	for _, c := range bom.Components {
		if c.Kind == KindProvider {
			doc.Services = append(doc.Services, cdxService{BOMRef: c.ID, Name: c.Name, Properties: properties(c)})
			continue
		}
		component := cdxComponent{Type: cycloneDXTypes[c.Kind], BOMRef: c.ID, Name: c.Name, Version: c.Version, Properties: properties(c)}
		if component.Type == "" {
			component.Type = "application"
		}
		if c.Supplier != "" {
			component.Supplier = &cdxSupplier{Name: c.Supplier}
		}
		doc.Components = append(doc.Components, component)
	}
	return json.MarshalIndent(doc, "", "  ")
}

func properties(c Component) []cdxProperty {
	out := []cdxProperty{{Name: propertyPrefix + "kind", Value: string(c.Kind)}, {Name: propertyPrefix + "origin", Value: c.Origin}}
	if c.Ecosystem != "" {
		out = append(out, cdxProperty{Name: propertyPrefix + "ecosystem", Value: c.Ecosystem})
	}
	keys := make([]string, 0, len(c.Properties))
	for key := range c.Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, cdxProperty{Name: propertyPrefix + key, Value: c.Properties[key]})
	}
	for _, e := range c.Evidence {
		out = append(out, cdxProperty{Name: propertyPrefix + "evidence", Value: place(e)})
	}
	for _, r := range c.Risks {
		value := string(r.Severity) + ": " + r.Summary
		if r.Where != nil {
			value += " (at " + place(*r.Where) + ")"
		}
		out = append(out, cdxProperty{Name: propertyPrefix + "risk:" + r.ID, Value: value})
	}
	return out
}
