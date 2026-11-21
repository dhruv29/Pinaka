package repo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

// binarySniffBytes is how much of a file is checked for a NUL byte before it
// is treated as text.
const binarySniffBytes = 8 << 10

var (
	errTooLarge = errors.New("larger than the file size cap")
	errBinary   = errors.New("binary")
)

// skippedDirNames hold dependencies, build output or tool caches rather than
// the project's own code. What they declare is the dependencies' business,
// and they are large. Hidden directories such as .github, .claude and
// .cursor are deliberately not here: agent tooling lives there.
var skippedDirNames = map[string]bool{
	".git": true, ".hg": true, ".svn": true, "node_modules": true, "bower_components": true,
	"vendor": true, "dist": true, "build": true, "target": true,
	".venv": true, "venv": true, "__pycache__": true, "site-packages": true,
	".tox": true, ".nox": true, ".mypy_cache": true, ".pytest_cache": true, ".ruff_cache": true,
	".next": true, ".nuxt": true, ".gradle": true, ".terraform": true, ".eggs": true,
}

// codeExtensions are read with the code rules: model names, providers,
// credential names, datasets and risky loader calls.
var codeExtensions = setOf(
	".py", ".pyi", ".js", ".mjs", ".cjs", ".jsx", ".ts", ".mts", ".cts", ".tsx",
	".go", ".rs", ".java", ".kt", ".kts", ".scala", ".rb", ".php", ".cs", ".swift",
	".sh", ".bash", ".zsh", ".ps1", ".r", ".jl", ".lua", ".ex", ".exs", ".dart",
	".json", ".jsonc", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".properties",
	".tf", ".hcl", ".env", ".sql", ".xml", ".gradle",
)

// docExtensions are read with the code rules too, but what they mention is
// marked as docs.
var docExtensions = setOf(".md", ".mdx", ".markdown", ".rst", ".txt", ".adoc")

// codeNames are files without a telling extension that hold configuration.
var codeNames = setOf("dockerfile", "containerfile", "modelfile", "makefile", "procfile", "justfile")

// lockNames are dependency lock files. Only their AI packages matter, and
// their hashes and URLs would only make noise for the code rules.
var lockNames = setOf("poetry.lock", "uv.lock")

func setOf(items ...string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

// isDotEnv reports .env, .env.example, .env.local and similar names.
func isDotEnv(base string) bool {
	return base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env")
}

// wantsCodeRules reports whether the file is source, configuration or docs.
func wantsCodeRules(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	if lockNames[base] {
		return false
	}
	ext := path.Ext(base)
	return codeExtensions[ext] || docExtensions[ext] || codeNames[base] ||
		strings.HasSuffix(base, ".modelfile") || strings.HasPrefix(base, "dockerfile") || isDotEnv(base)
}

// contextOf classifies a path as docs, test or code, from its name alone.
func contextOf(rel string) string {
	lower := strings.ToLower(rel)
	base := path.Base(lower)
	if isTestPath(lower, base) {
		return contextTest
	}
	if docExtensions[path.Ext(base)] && !isRequirementsFile(rel) {
		return contextDocs
	}
	for _, dir := range strings.Split(path.Dir(lower), "/") {
		if dir == "docs" || dir == "doc" || dir == "examples" || dir == "example" {
			return contextDocs
		}
	}
	return contextCode
}

func isTestPath(lower, base string) bool {
	for _, dir := range strings.Split(path.Dir(lower), "/") {
		switch dir {
		case "test", "tests", "__tests__", "testdata", "spec", "fixtures", "e2e":
			return true
		}
	}
	return strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(base, "_test.py") || strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") || base == "conftest.py"
}

// readText reads a regular file through root, refusing anything over limit
// or anything that looks binary. root confines the read to the tree even if
// a path component is a symbolic link.
func readText(root *os.Root, rel string, limit int64) (string, error) {
	info, err := root.Lstat(rel)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", rel)
	}
	if info.Size() > limit {
		return "", errTooLarge
	}
	f, err := root.Open(rel)
	if err != nil {
		return "", err
	}
	defer f.Close()
	// One byte over the limit is read to notice a file that grew after Lstat.
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > limit {
		return "", errTooLarge
	}
	if bytes.IndexByte(data[:min(len(data), binarySniffBytes)], 0) >= 0 {
		return "", errBinary
	}
	return string(data), nil
}

// textFile is a file's text with what the detectors need to report lines.
type textFile struct {
	path    string
	text    string
	context string
	starts  []int // byte offset at which each line begins
	// lineMap, when set, maps a line of text to the line of the file on
	// disk it came from; notebooks use it because their code is extracted.
	lineMap []int
}

func newTextFile(rel, text string) *textFile {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &textFile{path: rel, text: text, context: contextOf(rel), starts: starts}
}

// lineAt returns the 1-based line holding byte offset.
func (f *textFile) lineAt(offset int) int {
	line := sort.Search(len(f.starts), func(i int) bool { return f.starts[i] > offset })
	if f.lineMap != nil && line-1 < len(f.lineMap) {
		return f.lineMap[line-1]
	}
	return line
}

// lines returns the text split into lines without their endings.
func (f *textFile) lines() []string {
	out := strings.Split(f.text, "\n")
	for i, line := range out {
		out[i] = strings.TrimSuffix(line, "\r")
	}
	return out
}

// callArgs returns the text from offset up to the parenthesis that closes
// the call the offset is inside, so keyword arguments split over several
// lines are still seen. It is bounded to keep a hostile file cheap.
func callArgs(text string, offset int) string {
	const maxCallBytes = 2000
	end := min(len(text), offset+maxCallBytes)
	depth := 0
	for i := offset; i < end; i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return text[offset:i]
			}
			depth--
		}
	}
	return text[offset:end]
}
