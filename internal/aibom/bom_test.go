package aibom

import (
	"encoding/json"
	"strings"
	"testing"
)

func sample() BOM {
	model := Component{
		ID: ComponentID(KindModel, "weights.pkl", "", OriginRepo), Kind: KindModel, Name: "weights.pkl", Origin: OriginRepo,
		Evidence:   []Evidence{{Path: "models/weights.pkl"}},
		Risks:      []Risk{{ID: "pickle-model-file", Severity: SeverityHigh, Summary: "A pickle file can run code when it is loaded."}},
		Properties: map[string]string{"format": "pickle"},
	}
	library := Component{
		ID: ComponentID(KindLibrary, "openai", "1.40.0", OriginRepo), Kind: KindLibrary, Name: "openai", Version: "1.40.0",
		Supplier: "OpenAI", Ecosystem: "pypi", Origin: OriginRepo,
		Evidence: []Evidence{{Path: "requirements.txt", Line: 3}},
		Risks:    []Risk{{ID: "unpinned-library", Severity: SeverityLow, Summary: "No exact version."}},
	}
	provider := Component{
		ID: ComponentID(KindProvider, "api.openai.com", "", OriginCluster), Kind: KindProvider, Name: "api.openai.com", Origin: OriginCluster,
		Evidence: []Evidence{{Path: "prod/Deployment/chat", Detail: "env OPENAI_BASE_URL"}},
	}
	return BOM{Subject: "shop", Components: []Component{model, library, provider}, Notes: []string{"Secrets were not read."}}
}

func TestComponentIDIsStableAndIgnoresNameCase(t *testing.T) {
	a := ComponentID(KindLibrary, "OpenAI", "1.0", OriginRepo)
	if a != ComponentID(KindLibrary, "openai", "1.0", OriginRepo) || a == ComponentID(KindLibrary, "openai", "1.0", OriginCluster) {
		t.Fatal("the ID must ignore case in the name and differ by origin")
	}
}

func TestMergeJoinsTheSameComponentAndLeavesItsInputsAlone(t *testing.T) {
	first := sample()
	second := sample()
	second.Components = second.Components[1:2]
	second.Components[0].Evidence = []Evidence{{Path: "pyproject.toml", Line: 9}}
	second.Components[0].Properties = map[string]string{"group": "main"}

	merged := Merge("both", first, second)

	if len(merged.Components) != 3 || merged.Subject != "both" || len(merged.Notes) != 2 {
		t.Fatalf("merged = %+v; want three components and both notes", merged)
	}
	for _, c := range merged.Components {
		if c.Name != "openai" {
			continue
		}
		if len(c.Evidence) != 2 || len(c.Risks) != 1 || c.Properties["group"] != "main" || c.Supplier != "OpenAI" {
			t.Fatalf("openai = %+v; want evidence from both scans, one risk and the property", c)
		}
	}
	if len(first.Components[1].Evidence) != 1 {
		t.Fatal("Merge changed its input")
	}
	again, _ := json.Marshal(Merge("both", second, first))
	once, _ := json.Marshal(merged)
	if string(again) != string(once) {
		t.Fatal("the order of the inputs changed the result")
	}
}

func TestMarkdownPutsTheWorstRiskFirstAndListsTheGaps(t *testing.T) {
	text := Markdown(sample())

	high, low := strings.Index(text, "pickle file"), strings.Index(text, "No exact version")
	if high < 0 || low < 0 || high > low {
		t.Fatalf("risks are missing or out of order:\n%s", text)
	}
	for _, want := range []string{"## Look at these first", "## Models", "## AI libraries", "## Hosted AI providers", "## Not examined", "Secrets were not read.", "requirements.txt:3", "3 component(s)"} {
		if !strings.Contains(text, want) {
			t.Errorf("the report lacks %q", want)
		}
	}
}

func TestMarkdownSaysSoWhenNothingWasFound(t *testing.T) {
	text := Markdown(BOM{Subject: "empty"})
	if !strings.Contains(text, "No AI components were found.") || strings.Contains(text, "Look at these first") {
		t.Fatalf("report = %q", text)
	}
}

func TestMarkdownNeutralisesMarkupInScannedNames(t *testing.T) {
	bom := BOM{Subject: "x", Components: []Component{{
		ID: "1", Kind: KindModel, Name: "evil|[click](http://bad)<script>\n| injected | row |", Origin: OriginRepo,
		Evidence: []Evidence{{Path: "a`b"}},
	}}}

	text := Markdown(bom)

	if strings.Contains(text, "<script>") || strings.Contains(text, "[click](") || strings.Contains(text, "\n| injected") {
		t.Fatalf("scanned text reached the report as markup:\n%s", text)
	}
}

func TestSeverityHelpers(t *testing.T) {
	if got := WorstSeverity(sample()); got != SeverityHigh {
		t.Fatalf("worst = %q; want high", got)
	}
	if WorstSeverity(BOM{}) != "" {
		t.Fatal("an empty bill has no worst severity")
	}
	if !AtLeast(SeverityHigh, SeverityMedium) || AtLeast(SeverityLow, SeverityMedium) || AtLeast("", SeverityLow) || AtLeast(SeverityHigh, "bogus") {
		t.Fatal("AtLeast compares severities wrongly")
	}
}

func TestCycloneDXCarriesEveryComponentRiskAndNote(t *testing.T) {
	raw, err := CycloneDX(sample())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		BOMFormat   string `json:"bomFormat"`
		SpecVersion string `json:"specVersion"`
		Components  []struct {
			Type, Name string
			Supplier   *struct{ Name string }
		}
		Services []struct{ Name string }
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.BOMFormat != "CycloneDX" || doc.SpecVersion != "1.6" || len(doc.Components) != 2 || len(doc.Services) != 1 {
		t.Fatalf("document = %+v", doc)
	}
	if doc.Components[0].Type != "machine-learning-model" || doc.Components[1].Type != "library" || doc.Components[1].Supplier.Name != "OpenAI" || doc.Services[0].Name != "api.openai.com" {
		t.Fatalf("components = %+v, services = %+v", doc.Components, doc.Services)
	}
	for _, want := range []string{"pinaka:risk:pickle-model-file", "pinaka:evidence", "requirements.txt:3", "pinaka:note", "pinaka:format"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the document lacks %q", want)
		}
	}
}
