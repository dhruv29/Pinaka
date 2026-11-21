package repo

import (
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"
)

// manifestParsers maps a manifest's file name to its reader.
var manifestParsers = map[string]parser{
	"pyproject.toml":   (*scanner).pyproject,
	"pipfile":          (*scanner).pipfile,
	"setup.py":         (*scanner).setupPy,
	"setup.cfg":        (*scanner).setupCfg,
	"poetry.lock":      (*scanner).pythonLock,
	"uv.lock":          (*scanner).pythonLock,
	"package.json":     (*scanner).packageJSON,
	"go.mod":           (*scanner).goMod,
	"cargo.toml":       (*scanner).cargoToml,
	"pom.xml":          (*scanner).pomXML,
	"build.gradle":     (*scanner).gradle,
	"build.gradle.kts": (*scanner).gradle,
}

// manifestParser returns the dependency reader for rel, or nil.
func manifestParser(rel string) parser {
	if isRequirementsFile(rel) {
		return (*scanner).requirementsTxt
	}
	return manifestParsers[strings.ToLower(path.Base(rel))]
}

// packageJSONSections are the package.json fields that list dependencies.
var packageJSONSections = []string{"dependencies", "devDependencies", "peerDependencies", "optionalDependencies"}

func (s *scanner) packageJSON(f *textFile) {
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal([]byte(f.text), &manifest); err != nil {
		s.gap("could not be parsed as JSON", f.path)
		return
	}
	for _, section := range packageJSONSections {
		var deps map[string]string
		if raw, ok := manifest[section]; !ok || json.Unmarshal(raw, &deps) != nil {
			continue
		}
		names := make([]string, 0, len(deps))
		for name := range deps {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			s.library(f, dependency{eco: ecoNPM, name: name, version: npmVersion(deps[name]), line: f.lineOfKey(name)})
		}
	}
}

// npmVersion keeps ranges and tags but hides URLs, which may carry tokens.
func npmVersion(v string) string {
	if strings.Contains(v, ":") || strings.Contains(v, "/") {
		return directReference
	}
	return v
}

// lineOfKey returns the first line on which "key" appears as a JSON key,
// or 0 when it cannot be found.
func (f *textFile) lineOfKey(key string) int {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:`)
	if loc := pattern.FindStringIndex(f.text); loc != nil {
		return f.lineAt(loc[0])
	}
	return 0
}

// goMod reads require directives, single-line and block form.
func (s *scanner) goMod(f *textFile) {
	inBlock := false
	for i, raw := range f.lines() {
		// "// indirect" changes nothing about what is in the build.
		line, _, _ := strings.Cut(raw, "//")
		fields := strings.Fields(line)
		switch {
		case len(fields) == 2 && fields[0] == "require" && fields[1] == "(":
			inBlock = true
			continue
		case inBlock && len(fields) == 1 && fields[0] == ")":
			inBlock = false
			continue
		case !inBlock && len(fields) == 3 && fields[0] == "require":
			fields = fields[1:]
		case !inBlock:
			continue
		}
		if len(fields) != 2 {
			continue
		}
		s.library(f, dependency{eco: ecoGo, name: fields[0], version: fields[1], line: i + 1})
	}
}

// isCargoDepSection reports [dependencies], [dev-dependencies],
// [build-dependencies], [workspace.dependencies] and their
// [target.'cfg'.dependencies] forms.
func isCargoDepSection(section string) bool {
	for _, suffix := range []string{"dependencies", "dev-dependencies", "build-dependencies"} {
		if section == suffix || strings.HasSuffix(section, "."+suffix) {
			return true
		}
	}
	return false
}

var cargoRename = regexp.MustCompile(`\bpackage\s*=\s*"([^"]+)"`)

// cargoToml reads `name = "1.0"`, `name = { version = ... }` and
// `[dependencies.name]` tables.
func (s *scanner) cargoToml(f *textFile) {
	section := ""
	var pending *dependency // a [dependencies.name] table waiting for its version
	flush := func() {
		if pending != nil {
			s.library(f, *pending)
			pending = nil
		}
	}
	for i, raw := range f.lines() {
		trimmed := strings.TrimSpace(raw)
		if name, ok := tomlSection(trimmed); ok {
			flush()
			section = name
			if dot := strings.LastIndex(name, "."); dot > 0 && isCargoDepSection(name[:dot]) {
				pending = &dependency{eco: ecoCargo, name: strings.Trim(name[dot+1:], `"'`), line: i + 1}
			}
			continue
		}
		key, value, ok := tomlKeyValue(trimmed)
		switch {
		case !ok:
		case pending != nil && key == "version":
			pending.version = strings.Trim(value, `"'`)
		case pending == nil && isCargoDepSection(section):
			if m := cargoRename.FindStringSubmatch(value); m != nil {
				key = m[1]
			}
			s.library(f, dependency{eco: ecoCargo, name: key, version: cargoVersion(value), line: i + 1})
		}
	}
	flush()
}

func cargoVersion(value string) string {
	if strings.Contains(value, "workspace") && strings.Contains(value, "true") {
		return "workspace"
	}
	return tableVersion(value)
}

var (
	pomGroup    = regexp.MustCompile(`<groupId>\s*([^<]+?)\s*</groupId>`)
	pomArtifact = regexp.MustCompile(`<artifactId>\s*([^<]+?)\s*</artifactId>`)
	pomVersion  = regexp.MustCompile(`<version>\s*([^<]+?)\s*</version>`)
)

// pomXML reads <dependency> blocks line by line, ignoring the coordinates
// inside <exclusions>.
func (s *scanner) pomXML(f *textFile) {
	var dep *dependency
	group, excluded := "", false
	for i, line := range f.lines() {
		switch {
		case strings.Contains(line, "<dependency>"):
			dep, group, excluded = &dependency{eco: ecoMaven, versionOptional: true}, "", false
		case dep == nil:
			continue
		case strings.Contains(line, "<exclusions>"):
			excluded = true
		case strings.Contains(line, "</exclusions>"):
			excluded = false
		case excluded:
		case strings.Contains(line, "</dependency>"):
			if group != "" && dep.name != "" {
				dep.name = group + ":" + dep.name
				s.library(f, *dep)
			}
			dep = nil
		}
		if dep == nil || excluded {
			continue
		}
		if m := pomGroup.FindStringSubmatch(line); m != nil {
			group = m[1]
		}
		if m := pomArtifact.FindStringSubmatch(line); m != nil {
			dep.name, dep.line = m[1], i+1
		}
		if m := pomVersion.FindStringSubmatch(line); m != nil {
			dep.version = m[1]
		}
	}
}

// gradleDependency matches `implementation("group:artifact:version")` and
// the Groovy `implementation 'group:artifact:version'` form.
var gradleDependency = regexp.MustCompile(
	`^\s*[A-Za-z]*(?:[Ii]mplementation|[Aa]pi|[Cc]ompile|[Cc]ompileOnly|[Rr]untimeOnly|kapt|ksp)\s*\(?\s*(?:platform\(\s*)?["']([^"':\s]+):([^"':\s]+)(?::([^"'@\s]+))?["']`)

func (s *scanner) gradle(f *textFile) {
	for i, line := range f.lines() {
		m := gradleDependency.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		s.library(f, dependency{eco: ecoMaven, name: m[1] + ":" + m[2], version: m[3], line: i + 1, versionOptional: true})
	}
}
