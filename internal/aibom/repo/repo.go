// Package repo scans a source tree and produces an AI bill of materials: the
// AI libraries it depends on, the models it ships or names, the hosted AI
// services it calls, the credentials it expects (by name only), the
// datasets it loads and the agent tooling it configures, each with the file
// and line it was seen at and any risk it carries.
//
// Everything is read through os.Root, so neither a path nor a symbolic link
// can lead the scan outside the tree. Whatever a cap or an unreadable file
// stops the scan from seeing is reported as a note: a gap must never look
// like an absence.
package repo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

// Defaults for the zero Options. They are far above a hand-written project
// and low enough that a generated or hostile tree finishes promptly.
const (
	DefaultMaxFiles      = 100_000
	DefaultMaxFileBytes  = 2 << 20
	DefaultMaxComponents = 5_000
	DefaultMaxEvidence   = 50

	// notebookSizeFactor lets notebooks be larger than other text files:
	// embedded cell outputs make them big without much code in them.
	notebookSizeFactor = 4
	// maxNoteExamples is how many example paths a note lists.
	maxNoteExamples = 5
)

// Options bound one scan. A zero field takes its default.
type Options struct {
	// MaxFiles is the number of regular files visited before the walk stops.
	MaxFiles int
	// MaxFileBytes is the largest text file read. Model files are listed
	// whatever their size, because their contents are not read.
	MaxFileBytes int64
	// MaxComponents is the most components one scan records.
	MaxComponents int
	// MaxEvidence is the most locations recorded for one component.
	MaxEvidence int
	// Exclude lists paths to leave out, relative to the root, with "/" as
	// the separator. An entry matches a file or directory at exactly that
	// path, and may use the wildcards of path.Match ("*" does not cross a
	// "/"). Whatever is left out is listed in the Notes.
	Exclude []string
}

func (o Options) withDefaults() Options {
	if o.MaxFiles <= 0 {
		o.MaxFiles = DefaultMaxFiles
	}
	if o.MaxFileBytes <= 0 {
		o.MaxFileBytes = DefaultMaxFileBytes
	}
	if o.MaxComponents <= 0 {
		o.MaxComponents = DefaultMaxComponents
	}
	if o.MaxEvidence <= 0 {
		o.MaxEvidence = DefaultMaxEvidence
	}
	return o
}

// Scan walks the tree under root and returns its AI bill of materials. The
// output is sorted, so the same tree always gives the same result. Scan
// returns an error only when the root cannot be opened or ctx is done.
func Scan(ctx context.Context, root string, opts Options) (aibom.BOM, error) {
	for _, pattern := range opts.Exclude {
		if _, err := path.Match(pattern, ""); err != nil {
			return aibom.BOM{}, fmt.Errorf("aibom/repo: exclude pattern %q is malformed", pattern)
		}
	}
	if err := ctx.Err(); err != nil {
		return aibom.BOM{}, fmt.Errorf("aibom/repo: %w", err)
	}
	tree, err := os.OpenRoot(root)
	if err != nil {
		return aibom.BOM{}, fmt.Errorf("aibom/repo: open %s: %w", root, err)
	}
	defer tree.Close()

	s := &scanner{ctx: ctx, root: tree, opts: opts.withDefaults(), skipped: map[string]int{}, gaps: map[string][]string{}}
	s.out = newCollector(s.opts)
	if err := fs.WalkDir(tree.FS(), ".", s.visit); err != nil {
		return aibom.BOM{}, fmt.Errorf("aibom/repo: walk %s: %w", root, err)
	}
	return s.out.bom(filepath.Clean(root), s.notes()), nil
}

// scanner holds the state of one walk.
type scanner struct {
	ctx  context.Context
	root *os.Root
	opts Options
	out  *collector

	files      int
	hitFileCap bool
	skipped    map[string]int      // directory name -> times skipped
	gaps       map[string][]string // reason -> paths not examined
}

func (s *scanner) visit(rel string, d fs.DirEntry, walkErr error) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if walkErr != nil {
		if rel == "." {
			return walkErr
		}
		s.gap("could not be read", rel)
		if d != nil && d.IsDir() {
			return fs.SkipDir
		}
		return nil
	}
	if s.excluded(rel) {
		s.gap("were excluded on request", rel)
		if d.IsDir() {
			return fs.SkipDir
		}
		return nil
	}
	if d.IsDir() {
		return s.enterDir(rel, d.Name())
	}
	// A symbolic link is not followed; its target is scanned under its own
	// path if it lies inside the tree.
	if !d.Type().IsRegular() {
		return nil
	}
	if s.files++; s.files > s.opts.MaxFiles {
		s.hitFileCap = true
		return fs.SkipAll
	}
	s.scanFile(rel)
	return nil
}

// excluded reports whether the operator asked for rel to be left out.
func (s *scanner) excluded(rel string) bool {
	for _, pattern := range s.opts.Exclude {
		if matched, err := path.Match(path.Clean(pattern), rel); err == nil && matched {
			return true
		}
	}
	return false
}

func (s *scanner) enterDir(rel, name string) error {
	if rel == "." {
		return nil
	}
	if skippedDirNames[name] {
		s.skipped[name]++
		return fs.SkipDir
	}
	// A virtualenv may have any name; its marker file gives it away.
	if _, err := s.root.Lstat(path.Join(rel, "pyvenv.cfg")); err == nil {
		s.skipped["virtualenv ("+name+")"]++
		return fs.SkipDir
	}
	return nil
}

// scanFile runs every detector that applies to one file. Detectors that need
// only the name run first; the text is read once, and only if some
// detector wants it.
func (s *scanner) scanFile(rel string) {
	s.agentPath(rel)
	if s.modelFile(rel) {
		return
	}
	parsers := parsersFor(rel)
	if len(parsers) == 0 {
		return
	}
	limit := s.opts.MaxFileBytes
	if strings.HasSuffix(strings.ToLower(rel), ".ipynb") {
		limit *= notebookSizeFactor
	}
	text, err := readText(s.root, rel, limit)
	switch {
	case errors.Is(err, errTooLarge):
		s.gap(fmt.Sprintf("are larger than the %d-byte cap and were not read", limit), rel)
		return
	case errors.Is(err, errBinary):
		s.gap("look binary and were not read", rel)
		return
	case err != nil:
		s.gap("could not be read", rel)
		return
	}
	file := newTextFile(rel, text)
	for _, parse := range parsers {
		parse(s, file)
	}
}

// add records a finding; detectors call it rather than the collector so the
// evidence path is always the slash-separated path relative to the root.
func (s *scanner) add(f finding) {
	f.evidence.Path = filepath.ToSlash(f.evidence.Path)
	s.out.add(f)
}

// gap records a file or directory that was not examined, with why.
func (s *scanner) gap(reason, rel string) {
	s.gaps[reason] = append(s.gaps[reason], filepath.ToSlash(rel))
}

// notes turns what the walk could not see into sentences for the report.
func (s *scanner) notes() []string {
	var notes []string
	if s.hitFileCap {
		notes = append(notes, fmt.Sprintf(
			"file cap of %d reached: the walk stopped and the rest of the tree was not examined", s.opts.MaxFiles))
	}
	for reason, paths := range s.gaps {
		sort.Strings(paths)
		notes = append(notes, fmt.Sprintf("%d files or directories %s: %s", len(paths), reason, examples(paths)))
	}
	if len(s.skipped) > 0 {
		names := make([]string, 0, len(s.skipped))
		for name, n := range s.skipped {
			names = append(names, fmt.Sprintf("%s x%d", name, n))
		}
		sort.Strings(names)
		notes = append(notes, "dependency, build and tool directories were skipped, so AI declared or stored only there is not listed: "+
			strings.Join(names, ", "))
	}
	return notes
}

func examples(paths []string) string {
	if len(paths) <= maxNoteExamples {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(paths[:maxNoteExamples], ", "), len(paths)-maxNoteExamples)
}

// parser reads one file's text and records what it finds.
type parser func(s *scanner, f *textFile)

// parsersFor picks the parsers for a path by its name and extension.
func parsersFor(rel string) []parser {
	var out []parser
	if p := manifestParser(rel); p != nil {
		out = append(out, p)
	}
	if p := agentConfigParser(rel); p != nil {
		out = append(out, p)
	}
	if strings.HasSuffix(strings.ToLower(rel), ".ipynb") {
		return append(out, (*scanner).notebook)
	}
	if wantsCodeRules(rel) {
		out = append(out, (*scanner).codeRules)
	}
	return out
}
