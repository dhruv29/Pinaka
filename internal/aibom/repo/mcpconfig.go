package repo

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// rawServer is one MCP server entry as the common config formats write it.
type rawServer struct {
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	URL       string            `json:"url"`
	ServerURL string            `json:"serverUrl"`
	HTTPURL   string            `json:"httpUrl"`
	Env       map[string]string `json:"env"`
}

// mcpJSON reads mcpServers (Claude, Cursor, Gemini), servers (VS Code
// mcp.json) and mcp.servers (VS Code settings). Editor settings files are
// JSON with comments, so comments and trailing commas are removed first.
func (s *scanner) mcpJSON(f *textFile) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stripJSONC(f.text)), &doc); err != nil {
		s.gap("could not be parsed as JSON", f.path)
		return
	}
	for _, raw := range serverLists(doc) {
		var servers map[string]json.RawMessage
		if json.Unmarshal(raw, &servers) != nil {
			continue
		}
		names := make([]string, 0, len(servers))
		for name := range servers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			var srv rawServer
			if json.Unmarshal(servers[name], &srv) != nil {
				continue
			}
			s.addMCPServer(f, mcpServer{
				name: name, command: srv.Command, args: srv.Args, env: srv.Env, line: f.lineOfKey(name),
				url: firstNonEmpty(srv.URL, srv.ServerURL, srv.HTTPURL),
			})
		}
	}
}

func serverLists(doc map[string]json.RawMessage) []json.RawMessage {
	var out []json.RawMessage
	for _, key := range []string{"mcpServers", "servers"} {
		if raw, ok := doc[key]; ok {
			out = append(out, raw)
		}
	}
	var nested map[string]json.RawMessage
	if raw, ok := doc["mcp"]; ok && json.Unmarshal(raw, &nested) == nil {
		if raw, ok := nested["servers"]; ok {
			out = append(out, raw)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

var trailingComma = regexp.MustCompile(`,(\s*[}\]])`)

// stripJSONC removes // and /* */ comments outside strings, keeping line
// breaks so line numbers stay right, then trailing commas.
func stripJSONC(text string) string {
	var b strings.Builder
	inString, escaped := false, false
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case inString:
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '/' && i+1 < len(text) && text[i+1] == '/':
			for i < len(text) && text[i] != '\n' {
				i++
			}
			if i < len(text) {
				b.WriteByte('\n')
			}
			continue
		case c == '/' && i+1 < len(text) && text[i+1] == '*':
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				end = len(text) - i - 2
			}
			b.WriteString(strings.Repeat("\n", strings.Count(text[i:i+2+end], "\n")))
			i += end + 3
			continue
		}
		b.WriteByte(c)
	}
	return trailingComma.ReplaceAllString(b.String(), "$1")
}

var tomlArray = regexp.MustCompile(`"([^"]*)"|'([^']*)'`)

// codexToml reads [mcp_servers.NAME] tables from a Codex config.toml, and
// their [mcp_servers.NAME.env] sub-tables for credential names.
func (s *scanner) codexToml(f *textFile) {
	var cur *mcpServer
	inEnv := false
	flush := func() {
		if cur != nil {
			s.addMCPServer(f, *cur)
			cur = nil
		}
	}
	for i, raw := range f.lines() {
		trimmed := strings.TrimSpace(raw)
		if section, ok := tomlSection(trimmed); ok {
			name, isServer := strings.CutPrefix(section, "mcp_servers.")
			base, env := strings.CutSuffix(name, ".env")
			if isServer && env && cur != nil && cur.name == base {
				inEnv = true
				continue
			}
			flush()
			inEnv = false
			if isServer {
				cur = &mcpServer{name: strings.Trim(name, `"'`), line: i + 1, env: map[string]string{}}
			}
			continue
		}
		key, value, ok := tomlKeyValue(trimmed)
		if cur == nil || !ok {
			continue
		}
		unquoted := strings.Trim(value, `"'`)
		switch {
		case inEnv:
			cur.env[key] = unquoted
		case key == "command":
			cur.command = unquoted
		case key == "url":
			cur.url = unquoted
		case key == "args":
			for _, m := range tomlArray.FindAllStringSubmatch(value, -1) {
				cur.args = append(cur.args, m[1]+m[2])
			}
		}
	}
	flush()
}
