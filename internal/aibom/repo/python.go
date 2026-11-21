package repo

import (
	"path"
	"regexp"
	"strings"
)

// directReference stands in for a dependency fetched from a URL or VCS. The
// URL itself is not recorded: it may carry a token.
const directReference = "direct-reference"

// requirementPattern is a PEP 508 requirement: a name, optional extras, and
// whatever follows (a version specifier, a URL or markers).
var requirementPattern = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)\s*(?:\[[^\]]*\])?\s*(.*)$`)

// parseRequirement splits a requirement string into name and version
// specifier. Markers after ";" are dropped.
func parseRequirement(text string) (name, version string, ok bool) {
	m := requirementPattern.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return "", "", false
	}
	rest := strings.TrimSpace(m[2])
	if i := strings.Index(rest, ";"); i >= 0 {
		rest = strings.TrimSpace(rest[:i])
	}
	if strings.HasPrefix(rest, "@") {
		return m[1], directReference, true
	}
	if rest != "" && !strings.ContainsAny(rest[:1], "=<>!~(") {
		return "", "", false // not a version specifier: prose, not a requirement
	}
	return m[1], strings.Trim(rest, "()"), true
}

// isRequirementsFile reports requirements.txt and its usual variants.
func isRequirementsFile(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	ext := path.Ext(base)
	if ext != ".txt" && ext != ".in" {
		return false
	}
	return strings.Contains(base, "requirements") || strings.HasPrefix(base, "constraints") ||
		path.Base(path.Dir(strings.ToLower(rel))) == "requirements"
}

// requirementsTxt reads one requirement per line, skipping options,
// includes, comments and bare URLs.
func (s *scanner) requirementsTxt(f *textFile) {
	for i, line := range f.lines() {
		if j := strings.Index(line, " #"); j >= 0 {
			line = line[:j]
		}
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "\\"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") || strings.Contains(line, "://") && !strings.Contains(line, "@") {
			continue
		}
		if name, version, ok := parseRequirement(line); ok {
			s.library(f, dependency{eco: ecoPyPI, name: name, version: version, line: i + 1})
		}
	}
}

var quotedString = regexp.MustCompile(`"([^"]*)"|'([^']*)'`)

// requirementStrings records every quoted requirement on one line.
func (s *scanner) requirementStrings(f *textFile, text string, line int) {
	for _, m := range quotedString.FindAllStringSubmatch(text, -1) {
		value := m[1] + m[2]
		if name, version, ok := parseRequirement(value); ok {
			s.library(f, dependency{eco: ecoPyPI, name: name, version: version, line: line})
		}
	}
}

// outsideQuotes returns line with the contents of quoted strings removed,
// so brackets inside "transformers[torch]" do not end an array.
func outsideQuotes(line string) string {
	return quotedString.ReplaceAllString(line, `""`)
}

// tomlKeyValue splits a "key = value" line. Quoted keys are unquoted.
func tomlKeyValue(line string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.Trim(strings.TrimSpace(key), `"'`)
	return key, strings.TrimSpace(value), key != ""
}

// tomlSection returns the name in a "[section]" or "[[section]]" header.
func tomlSection(trimmed string) (string, bool) {
	if !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	return strings.Trim(trimmed, "[] "), true
}

// pyproject reads PEP 621 dependency arrays, PEP 735 dependency groups and
// Poetry dependency tables. It is line-based: arrays may span lines, inline
// tables may not, which covers how these files are written in practice.
func (s *scanner) pyproject(f *textFile) {
	section, inArray := "", false
	for i, raw := range f.lines() {
		trimmed := strings.TrimSpace(raw)
		if inArray {
			s.requirementStrings(f, trimmed, i+1)
			inArray = !strings.Contains(outsideQuotes(trimmed), "]")
			continue
		}
		if name, ok := tomlSection(trimmed); ok {
			section = name
			continue
		}
		key, value, ok := tomlKeyValue(trimmed)
		if !ok {
			continue
		}
		switch {
		case isDependencyArray(section, key) && strings.HasPrefix(value, "["):
			s.requirementStrings(f, value, i+1)
			inArray = !strings.Contains(outsideQuotes(value), "]")
		case strings.HasPrefix(section, "tool.poetry") && strings.HasSuffix(section, "dependencies") && key != "python":
			s.library(f, dependency{eco: ecoPyPI, name: key, version: tableVersion(value), line: i + 1})
		}
	}
}

func isDependencyArray(section, key string) bool {
	switch section {
	case "project.optional-dependencies", "dependency-groups":
		return true
	case "project", "tool.uv", "tool.pdm.dev-dependencies":
		return strings.Contains(key, "dependencies")
	}
	return false
}

var (
	inlineVersion = regexp.MustCompile(`\bversion\s*=\s*["']([^"']*)["']`)
	pinnedSource  = regexp.MustCompile(`\b(rev|tag|path)\s*=`)
)

// tableVersion reads the version from `"^1.0"` or `{ version = "^1.0" }`.
// A VCS dependency pinned to a rev or tag counts as a direct reference; one
// that follows a branch has no version.
func tableVersion(value string) string {
	if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, `'`) {
		return strings.Trim(value, `"' `)
	}
	if m := inlineVersion.FindStringSubmatch(value); m != nil {
		return m[1]
	}
	if pinnedSource.MatchString(value) {
		return directReference
	}
	return ""
}

// pipfile reads the [packages] and [dev-packages] tables of a Pipfile.
func (s *scanner) pipfile(f *textFile) {
	section := ""
	for i, raw := range f.lines() {
		trimmed := strings.TrimSpace(raw)
		if name, ok := tomlSection(trimmed); ok {
			section = name
			continue
		}
		if section != "packages" && section != "dev-packages" {
			continue
		}
		if key, value, ok := tomlKeyValue(trimmed); ok && !strings.HasPrefix(key, "#") {
			s.library(f, dependency{eco: ecoPyPI, name: key, version: tableVersion(value), line: i + 1})
		}
	}
}

// setupPy reads quoted requirements inside install_requires, extras_require
// and the like, tracking brackets so strings elsewhere (such as the
// project's own name) are not mistaken for dependencies.
func (s *scanner) setupPy(f *textFile) {
	depth, active := 0, false
	for i, raw := range f.lines() {
		if !active && strings.Contains(raw, "_require") {
			active, depth = true, 0
		}
		if !active {
			continue
		}
		s.requirementStrings(f, raw, i+1)
		bare := outsideQuotes(raw)
		depth += strings.Count(bare, "[") + strings.Count(bare, "{") - strings.Count(bare, "]") - strings.Count(bare, "}")
		if depth <= 0 && strings.ContainsAny(bare, "]}") {
			active = false
		}
	}
}

// setupCfg reads install_requires in [options] and every entry of
// [options.extras_require]; values continue on indented lines.
func (s *scanner) setupCfg(f *textFile) {
	section, inValue := "", false
	for i, raw := range f.lines() {
		trimmed := strings.TrimSpace(raw)
		indented := trimmed != "" && (strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t"))
		if name, ok := tomlSection(trimmed); ok {
			section, inValue = name, false
			continue
		}
		value := ""
		switch {
		case indented && inValue:
			value = trimmed
		case section == "options" && !indented:
			key, rest, _ := tomlKeyValue(trimmed)
			inValue, value = key == "install_requires", rest
		case section == "options.extras_require" && !indented:
			_, rest, ok := tomlKeyValue(trimmed)
			inValue, value = ok, rest
		}
		if inValue && value != "" && !strings.HasPrefix(value, "#") {
			if name, version, ok := parseRequirement(value); ok {
				s.library(f, dependency{eco: ecoPyPI, name: name, version: version, line: i + 1})
			}
		}
	}
}

// pythonLock reads the [[package]] entries of poetry.lock and uv.lock. Only
// AI packages are kept; the rest of the lock file is not reported.
func (s *scanner) pythonLock(f *textFile) {
	inPackage, name, nameLine := false, "", 0
	for i, raw := range f.lines() {
		trimmed := strings.TrimSpace(raw)
		if header, ok := tomlSection(trimmed); ok {
			inPackage, name = header == "package" && strings.HasPrefix(trimmed, "[["), ""
			continue
		}
		key, value, ok := tomlKeyValue(trimmed)
		if !inPackage || !ok {
			continue
		}
		switch key {
		case "name":
			name, nameLine = strings.Trim(value, `"'`), i+1
		case "version":
			if name != "" {
				s.library(f, dependency{eco: ecoPyPI, name: name, version: strings.Trim(value, `"'`), line: nameLine, locked: true})
				name = ""
			}
		}
	}
}
