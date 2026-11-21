package aibom

import (
	"encoding/json"
	"strings"
	"testing"
)

func sample() BOM {
	model := Component{
		ID: ComponentID(KindModel, "weights.pkl", ""), Kind: KindModel, Name: "weights.pkl", Origin: OriginRepo,
		Evidence:   []Evidence{{Path: "models/weights.pkl"}},
		Risks:      []Risk{{ID: "pickle-model-file", Severity: SeverityHigh, Summary: "A pickle file can run code when it is loaded."}},
		Properties: map[string]string{"format": "pickle"},
	}
	library := Component{
		ID: ComponentID(KindLibrary, "openai", "1.40.0"), Kind: KindLibrary, Name: "openai", Version: "1.40.0",
		Supplier: "OpenAI", Ecosystem: "pypi", Origin: OriginRepo,
		Evidence: []Evidence{{Path: "requirements.txt", Line: 3}},
		Risks:    []Risk{{ID: "unpinned-library", Severity: SeverityLow, Summary: "No exact version."}},
	}
	provider := Component{
		ID: ComponentID(KindProvider, "api.openai.com", ""), Kind: KindProvider, Name: "api.openai.com", Origin: OriginCluster,
		Evidence: []Evidence{{Path: "prod/Deployment/chat", Detail: "env OPENAI_BASE_URL"}},
	}
	return BOM{Subject: "shop", Components: []Component{model, library, provider}, Notes: []string{"Secrets were not read."}}
}

func TestComponentIDIsStableAndIgnoresNameCase(t *testing.T) {
	a := ComponentID(KindLibrary, "OpenAI", "1.0")
	if a != ComponentID(KindLibrary, "openai", "1.0") || a == ComponentID(KindLibrary, "openai", "1.1") || a == ComponentID(KindModel, "openai", "1.0") {
		t.Fatal("the ID must ignore case in the name and differ by kind and version")
	}
}

func TestJoinOriginsNamesEachPlaceOnceInAFixedOrder(t *testing.T) {
	for _, c := range []struct{ a, b, want string }{
		{OriginRepo, OriginRepo, OriginRepo}, {OriginCluster, OriginRepo, OriginBoth}, {OriginBoth, OriginCluster, OriginBoth},
		{"", OriginCluster, OriginCluster}, {"z+repo", "a", "repo+a+z"},
	} {
		if got := JoinOrigins(c.a, c.b); got != c.want {
			t.Errorf("JoinOrigins(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
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

func TestMergeJoinsTheSameComponentFromCodeAndClusterAndKeepsBothOrigins(t *testing.T) {
	fromCode := Component{ID: ComponentID(KindCredential, "OPENAI_API_KEY", ""), Kind: KindCredential, Name: "OPENAI_API_KEY",
		Supplier: "OpenAI", Origin: OriginRepo, Evidence: []Evidence{{Path: ".env", Line: 1}}}
	fromCluster := Component{ID: ComponentID(KindCredential, "OPENAI_API_KEY", ""), Kind: KindCredential, Name: "OPENAI_API_KEY",
		Origin: OriginCluster, Evidence: []Evidence{{Path: "ml/Deployment/chat"}},
		Risks: []Risk{{ID: "literal-ai-credential", Severity: SeverityHigh, Summary: "literal"}}}
	only := Component{ID: ComponentID(KindModel, "gpt-4o", ""), Kind: KindModel, Name: "gpt-4o", Origin: OriginCluster}

	merged := Merge("both", BOM{Components: []Component{fromCode}}, BOM{Components: []Component{fromCluster, only}})

	if len(merged.Components) != 2 {
		t.Fatalf("components = %+v; want the credential once and the model", merged.Components)
	}
	for _, c := range merged.Components {
		switch c.Name {
		case "OPENAI_API_KEY":
			if c.Origin != OriginBoth || len(c.Evidence) != 2 || len(c.Risks) != 1 || c.Supplier != "OpenAI" {
				t.Errorf("credential = %+v", c)
			}
		case "gpt-4o":
			if c.Origin != OriginCluster {
				t.Errorf("model origin = %q", c.Origin)
			}
		}
	}
	if empty, _ := json.Marshal(Merge("none")); !strings.Contains(string(empty), `"components":[]`) {
		t.Errorf("an empty bill is written as %s; want an empty list", empty)
	}
	twice := Merge("again", merged, BOM{Components: []Component{fromCode}})
	if twice.Components[0].Origin != OriginBoth && twice.Components[1].Origin != OriginBoth {
		t.Error("merging a merged bill lost an origin")
	}
}

func TestARiskIsShownWhereItWasRaisedNotWhereTheComponentWasFirstSeen(t *testing.T) {
	raised := Evidence{Path: "ml/Deployment/vllm", Detail: "containers[vllm].env OPENAI_API_KEY"}
	c := Component{ID: "1", Kind: KindCredential, Name: "OPENAI_API_KEY", Origin: OriginCluster,
		Evidence: []Evidence{{Path: "ml/ConfigMap/cfg", Detail: "data[OPENAI_API_KEY]"}, raised},
		Risks:    []Risk{{ID: "literal-ai-credential", Severity: SeverityHigh, Summary: "literal", Where: &raised}}}
	bom := BOM{Subject: "s", Components: []Component{c}}

	text := Markdown(bom)
	row := text[strings.Index(text, "| high |"):]
	row = row[:strings.Index(row, "\n")]
	if !strings.Contains(row, "ml/Deployment/vllm") || strings.Contains(row, "ConfigMap") {
		t.Errorf("risk row = %q", row)
	}
	cdx, err := CycloneDX(bom)
	if err != nil || !strings.Contains(string(cdx), "literal (at ml/Deployment/vllm (containers[vllm].env OPENAI_API_KEY))") {
		t.Errorf("CycloneDX lacks where the risk was raised: %s", cdx)
	}
}

func TestRiskWhereIsOmittedWhenEmptyAndExactDuplicatesAreStillDropped(t *testing.T) {
	plain, _ := json.Marshal(Risk{ID: "x", Severity: SeverityLow, Summary: "s"})
	if strings.Contains(string(plain), "where") {
		t.Errorf("empty where is written: %s", plain)
	}
	one, two := Evidence{Path: "p", Line: 2}, Evidence{Path: "p", Line: 2}
	risks := uniqueRisks([]Risk{{ID: "x", Summary: "s", Where: &one}, {ID: "x", Summary: "s", Where: &two}, {ID: "x", Summary: "s"}})
	if len(risks) != 2 {
		t.Errorf("risks = %+v; want the two with the same place joined and the one without kept", risks)
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
