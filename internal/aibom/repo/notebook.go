package repo

import (
	"bytes"
	"encoding/json"
	"strings"
)

// notebookCell is the part of a Jupyter cell the scan needs. Outputs are
// deliberately not read: they can hold printed secrets and large data.
type notebookCell struct {
	CellType string          `json:"cell_type"`
	Source   json.RawMessage `json:"source"`
}

// notebook runs the code rules over a notebook's code cells. Line numbers
// point at the line of the .ipynb file holding the source line, so a
// reader can jump straight to it.
func (s *scanner) notebook(f *textFile) {
	var nb struct {
		Cells []notebookCell `json:"cells"`
	}
	if err := json.Unmarshal([]byte(f.text), &nb); err != nil {
		s.gap("could not be parsed as JSON", f.path)
		return
	}
	var code []string
	for _, cell := range nb.Cells {
		if cell.CellType == "code" {
			code = append(code, cellLines(cell.Source)...)
		}
	}
	if len(code) == 0 {
		return
	}
	extracted := newTextFile(f.path, strings.Join(code, "\n"))
	extracted.lineMap = mapLines(f.text, code)
	s.codeRules(extracted)
}

// cellLines returns a cell's source as lines. nbformat allows a string or a
// list of strings.
func cellLines(raw json.RawMessage) []string {
	var parts []string
	if json.Unmarshal(raw, &parts) != nil {
		var whole string
		if json.Unmarshal(raw, &whole) != nil {
			return nil
		}
		parts = []string{whole}
	}
	return strings.Split(strings.TrimSuffix(strings.Join(parts, ""), "\n"), "\n")
}

// mapLines finds each code line, JSON-encoded as it appears on disk, in the
// raw notebook text, searching forward so repeated lines map in order. A
// line that cannot be found maps to the last line found.
func mapLines(raw string, code []string) []int {
	starts := newTextFile("", raw)
	out := make([]int, len(code))
	cursor, last := 0, 1
	for i, line := range code {
		encoded := encodeJSON(line)
		if encoded != "" {
			if at := strings.Index(raw[cursor:], encoded); at >= 0 {
				cursor += at
				last = starts.lineAt(cursor)
			}
		}
		out[i] = last
	}
	return out
}

// encodeJSON returns s as it appears inside a JSON string, without quotes.
func encodeJSON(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(s) != nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSpace(buf.String()), `"`)[1:]
}
