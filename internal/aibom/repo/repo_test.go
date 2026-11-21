package repo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// writeTree creates the files under a new temporary directory and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func mustScan(t *testing.T, root string, opts Options) aibom.BOM {
	t.Helper()
	bom, err := Scan(context.Background(), root, opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return bom
}

func scanFiles(t *testing.T, files map[string]string) aibom.BOM {
	t.Helper()
	return mustScan(t, writeTree(t, files), Options{})
}

func find(bom aibom.BOM, kind aibom.Kind, name string) *aibom.Component {
	for i, c := range bom.Components {
		if c.Kind == kind && c.Name == name {
			return &bom.Components[i]
		}
	}
	return nil
}

func mustFind(t *testing.T, bom aibom.BOM, kind aibom.Kind, name string) aibom.Component {
	t.Helper()
	c := find(bom, kind, name)
	if c == nil {
		t.Fatalf("no %s %q in %s", kind, name, names(bom))
	}
	return *c
}

func mustNotFind(t *testing.T, bom aibom.BOM, kind aibom.Kind, name string) {
	t.Helper()
	if find(bom, kind, name) != nil {
		t.Errorf("unexpected %s %q", kind, name)
	}
}

func names(bom aibom.BOM) string {
	var out []string
	for _, c := range bom.Components {
		out = append(out, string(c.Kind)+":"+c.Name+"@"+c.Version)
	}
	return strings.Join(out, ", ")
}

func riskSeverity(c aibom.Component, id string) aibom.Severity {
	for _, r := range c.Risks {
		if r.ID == id {
			return r.Severity
		}
	}
	return ""
}

func hasNote(bom aibom.BOM, substring string) bool {
	for _, n := range bom.Notes {
		if strings.Contains(n, substring) {
			return true
		}
	}
	return false
}

func marshal(t *testing.T, bom aibom.BOM) string {
	t.Helper()
	data, err := json.Marshal(bom)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestScanDoesNotFollowSymbolicLinksOutOfTheTree(t *testing.T) {
	outside := writeTree(t, map[string]string{"leak.py": `key = os.environ["OPENAI_API_KEY"]`, "dir/m.gguf": "GGUF"})
	root := writeTree(t, map[string]string{"main.py": "print(1)\n"})
	if err := os.Symlink(filepath.Join(outside, "leak.py"), filepath.Join(root, "link.py")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	if err := os.Symlink(filepath.Join(outside, "dir"), filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	bom := mustScan(t, root, Options{})
	if len(bom.Components) != 0 {
		t.Errorf("symlinked content was scanned: %s", names(bom))
	}
}

func TestScanNotesSymbolicLinksAndSpecialFilesItDidNotRead(t *testing.T) {
	root := writeTree(t, map[string]string{"main.py": "print(1)\n"})
	if err := os.Symlink("/etc", filepath.Join(root, "etc-link")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe.py"), 0o600); err != nil {
		t.Skip("named pipes unsupported:", err)
	}
	bom := mustScan(t, root, Options{})
	if !hasNote(bom, "symbolic links and were not followed: etc-link") || !hasNote(bom, "not regular files") || !hasNote(bom, "pipe.py") {
		t.Errorf("notes = %v", bom.Notes)
	}
}

func TestScanStopsAtTheFileCapAndSaysSo(t *testing.T) {
	bom := mustScan(t, writeTree(t, map[string]string{"a.py": "", "b.py": "", "c.py": `m = "gpt-4o"`}), Options{MaxFiles: 2})
	if !hasNote(bom, "file cap of 2") {
		t.Errorf("notes = %v", bom.Notes)
	}
	mustNotFind(t, bom, aibom.KindModel, "gpt-4o")
}

func TestScanCapsComponentsAndEvidenceWithNotes(t *testing.T) {
	root := writeTree(t, map[string]string{"a.py": "a = \"gpt-4o\"\nb = \"gpt-4o\"\nc = \"claude-3-opus\"\n"})
	bom := mustScan(t, root, Options{MaxComponents: 1, MaxEvidence: 1})
	if len(bom.Components) != 1 || len(bom.Components[0].Evidence) != 1 {
		t.Fatalf("components = %s", names(bom))
	}
	if !hasNote(bom, "component cap of 1") || !hasNote(bom, "evidence cap of 1") {
		t.Errorf("notes = %v", bom.Notes)
	}
}

func TestScanNotesFilesTooLargeOrBinaryToRead(t *testing.T) {
	root := writeTree(t, map[string]string{
		"big.py":   strings.Repeat("#", 200) + `"gpt-4o"`,
		"bin.json": "\x00\x01OPENAI_API_KEY",
	})
	bom := mustScan(t, root, Options{MaxFileBytes: 100})
	if !hasNote(bom, "larger than the 100-byte cap") || !hasNote(bom, "look binary") {
		t.Errorf("notes = %v", bom.Notes)
	}
	if len(bom.Components) != 0 {
		t.Errorf("components = %s", names(bom))
	}
}

func TestScanSkipsDependencyDirectoriesAndVirtualenvsWithANote(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"node_modules/x/package.json": `{"dependencies": {"openai": "^4"}}`,
		".venv/lib/a.py":              `m = "gpt-4o"`,
		"myenv/pyvenv.cfg":            "home = /usr",
		"myenv/lib/b.py":              `m = "claude-3-opus"`,
		".claude/agents/reviewer.md":  "review",
	})
	if len(bom.Components) != 1 {
		t.Errorf("components = %s", names(bom))
	}
	if !hasNote(bom, "node_modules x1") || !hasNote(bom, "virtualenv (myenv)") {
		t.Errorf("notes = %v", bom.Notes)
	}
}

func TestScanIsDeterministic(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.py":             "import openai\nclient = OpenAI(base_url=\"https://llm.example.net/v1\")\nm = \"gpt-4o\"\nOPENAI_API_KEY\n",
		"b.py":             "AutoModel.from_pretrained(\"org/model\")\nm = \"gpt-4o\"\n",
		"requirements.txt": "openai\ntorch==2.1\n",
		"w.pkl":            "x",
	})
	first, second := marshal(t, mustScan(t, root, Options{})), marshal(t, mustScan(t, root, Options{}))
	if first != second {
		t.Errorf("scans differ:\n%s\n%s", first, second)
	}
}

func TestScanReturnsAnErrorForACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, t.TempDir(), Options{}); err == nil || !strings.HasPrefix(err.Error(), "aibom/repo:") {
		t.Errorf("err = %v", err)
	}
}

func TestScanStopsWhenTheContextIsCancelledDuringTheWalk(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &scanner{ctx: ctx, opts: Options{}.withDefaults()}
	cancel()
	if err := s.visit("a.py", nil, nil); err == nil {
		t.Error("visit ignored a cancelled context")
	}
}

func TestScanReturnsAnErrorForAMissingRoot(t *testing.T) {
	_, err := Scan(context.Background(), filepath.Join(t.TempDir(), "nope"), Options{})
	if err == nil || !strings.HasPrefix(err.Error(), "aibom/repo:") {
		t.Errorf("err = %v", err)
	}
}

func TestEveryRiskInTheTableIsComplete(t *testing.T) {
	for key, r := range riskTable {
		if r.ID == "" || r.Severity == "" || r.Summary == "" {
			t.Errorf("risk %s is incomplete: %+v", key, r)
		}
	}
}

func TestContextOfClassifiesDocsTestsAndCode(t *testing.T) {
	cases := map[string]string{
		"README.md": contextDocs, "docs/setup.py": contextDocs, "app/main.py": contextCode,
		"tests/x.py": contextTest, "pkg/x_test.go": contextTest, "requirements.txt": contextCode,
		"web/a.spec.ts": contextTest,
	}
	for p, want := range cases {
		if got := contextOf(p); got != want {
			t.Errorf("contextOf(%q) = %q, want %q", p, got, want)
		}
	}
}

// TestSmokeScanOfARealTree scans the directory in AIBOM_SMOKE_ROOT, when
// set, and logs a summary. It is skipped in normal runs.
func TestSmokeScanOfARealTree(t *testing.T) {
	root := os.Getenv("AIBOM_SMOKE_ROOT")
	if root == "" {
		t.Skip("AIBOM_SMOKE_ROOT not set")
	}
	bom := mustScan(t, root, Options{})
	counts := map[aibom.Kind]int{}
	for _, c := range bom.Components {
		counts[c.Kind]++
		risks := []string{}
		for _, r := range c.Risks {
			risks = append(risks, r.ID)
		}
		t.Logf("%s %q %s ctx=%s ev=%d first=%s:%d risks=%v", c.Kind, c.Name, c.Version, c.Properties[propContext], len(c.Evidence), c.Evidence[0].Path, c.Evidence[0].Line, risks)
	}
	t.Logf("counts=%v notes=%v", counts, bom.Notes)
}
