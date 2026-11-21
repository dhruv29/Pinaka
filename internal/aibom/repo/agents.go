package repo

import (
	"path"
	"regexp"
	"strings"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

const (
	propType          = "type"
	typeInstructions  = "agent-instructions"
	typeAgentDef      = "agent-definition"
	typeSkill         = "agent-skill"
	typeCommand       = "agent-command"
	typeSystemPrompt  = "system-prompt"
	typeMCPServer     = "mcp-server"
	detailAgentFile   = "agent tooling file"
	detailMCPServer   = "MCP server configuration"
	supplierAnthropic = "Anthropic"
)

// instructionFiles are files an AI coding agent reads as standing
// instructions, with the agent's publisher. AGENTS.md is a shared format.
var instructionFiles = map[string]string{
	"claude.md": supplierAnthropic, "claude.local.md": supplierAnthropic, "agents.md": "",
	"gemini.md": "Google", ".cursorrules": "Anysphere", ".windsurfrules": "Windsurf",
	".clinerules": "Cline", "copilot-instructions.md": "GitHub",
}

// agentDirs are directories whose files define agents, skills or commands.
var agentDirs = []struct{ dir, kind, supplier string }{
	{".claude/agents/", typeAgentDef, supplierAnthropic},
	{".claude/skills/", typeSkill, supplierAnthropic},
	{".claude/commands/", typeCommand, supplierAnthropic},
	{".cursor/rules/", typeInstructions, "Anysphere"},
	{".github/instructions/", typeInstructions, "GitHub"},
	{".github/prompts/", typeSystemPrompt, "GitHub"},
}

// systemPromptName matches files named as system prompts. It is matched on
// the base name only, so it stays quiet about ordinary files.
var systemPromptName = regexp.MustCompile(`(?i)^(?:system[-_ ]?prompts?|sys[-_]prompt)(?:[-_.][\w.-]*)?\.(?:txt|md|j2|jinja2?|tmpl|tpl|ya?ml|json|prompt)$|\.prompty$`)

// agentPath records rel if its name or location marks it as agent tooling.
// Only the path is used; the file is not read for this.
func (s *scanner) agentPath(rel string) {
	slashed := "/" + strings.ToLower(rel)
	base := path.Base(slashed)
	kind, supplier := "", ""
	if sup, ok := instructionFiles[base]; ok {
		kind, supplier = typeInstructions, sup
	}
	for _, d := range agentDirs {
		if strings.Contains(slashed, "/"+d.dir) {
			kind, supplier = d.kind, d.supplier
		}
	}
	if kind == "" && systemPromptName.MatchString(base) {
		kind = typeSystemPrompt
	}
	if kind == "" {
		return
	}
	s.add(finding{
		component: aibom.Component{Kind: aibom.KindAgentTool, Name: rel, Supplier: supplier,
			Properties: map[string]string{propType: kind, propContext: contextOf(rel)}},
		evidence: aibom.Evidence{Path: rel, Detail: detailAgentFile},
	})
}

// mcpConfigNames are files that hold MCP server lists.
var mcpConfigNames = setOf(".mcp.json", "mcp.json", "mcp_config.json", "mcp_servers.json", "claude_desktop_config.json")

// agentSettingsDirs hold editor or agent settings that may list MCP servers.
var agentSettingsDirs = setOf(".vscode", ".cursor", ".claude", ".gemini", ".windsurf", ".roo", ".kiro")

// agentConfigParser returns the MCP configuration reader for rel, or nil.
func agentConfigParser(rel string) parser {
	lower := strings.ToLower(rel)
	base, dir := path.Base(lower), path.Base(path.Dir(lower))
	switch {
	case mcpConfigNames[base]:
		return (*scanner).mcpJSON
	case agentSettingsDirs[dir] && strings.HasPrefix(base, "settings") && strings.HasSuffix(base, ".json"):
		return (*scanner).mcpJSON
	case dir == ".codex" && base == "config.toml":
		return (*scanner).codexToml
	}
	return nil
}

// mcpServer is what is recorded of one configured server. Header and env
// values are never kept; env names are, because they say which credentials
// the server is given.
type mcpServer struct {
	name    string
	command string
	args    []string
	url     string
	env     map[string]string
	line    int
}

// fetchers run code they download at start-up.
var fetchers = setOf("npx", "bunx", "uvx", "pipx", "dlx")

// shells run whatever command line they are given.
var shells = setOf("sh", "bash", "zsh", "cmd", "powershell", "pwsh")

// addMCPServer records one server, the credentials passed to it by name,
// and the remote-or-shell risk.
func (s *scanner) addMCPServer(f *textFile, srv mcpServer) {
	props := map[string]string{propType: typeMCPServer, propContext: f.context}
	command := path.Base(strings.ReplaceAll(srv.command, `\`, "/"))
	if command != "." && command != "" {
		props["command"] = command
	}
	if pkg := firstOperand(srv.args); pkg != "" && (fetchers[command] || command == "docker" || command == "node" || command == "python" || command == "python3") {
		props["package"] = pkg
	}
	if host := hostOnly(srv.url); host != "" {
		props["url"] = host
	}
	found := finding{
		component: aibom.Component{Kind: aibom.KindAgentTool, Name: "mcp:" + srv.name, Properties: props},
		evidence:  aibom.Evidence{Path: f.path, Line: srv.line, Detail: detailMCPServer},
	}
	if isRemoteOrShell(command, srv) {
		found.risks = []aibom.Risk{risk(riskMCPRemote)}
	}
	s.add(found)
	for name, value := range srv.env {
		if _, known := aiCredentials[name]; known {
			s.credentialAt(f, srv.line, name, looksLikeKey(value))
		}
	}
}

func isRemoteOrShell(command string, srv mcpServer) bool {
	if srv.url != "" || fetchers[command] {
		return true
	}
	for _, arg := range srv.args {
		if (command == "pnpm" || command == "yarn") && arg == "dlx" {
			return true
		}
		if shells[command] && (arg == "-c" || strings.EqualFold(arg, "/c") || strings.EqualFold(arg, "-command")) {
			return true
		}
	}
	return false
}

// firstOperand returns the first argument that is not a flag: for npx and
// uvx, the package that is fetched.
func firstOperand(args []string) string {
	for _, arg := range args {
		if arg != "" && !strings.HasPrefix(arg, "-") && arg != "run" && arg != "dlx" {
			return arg
		}
	}
	return ""
}

// credentialAt records a credential name seen at a known line.
func (s *scanner) credentialAt(f *textFile, line int, name string, hardcoded bool) {
	found := finding{
		component: aibom.Component{Kind: aibom.KindCredential, Name: name, Supplier: aiCredentials[name], Properties: map[string]string{propContext: f.context}},
		evidence:  aibom.Evidence{Path: f.path, Line: line, Detail: "credential passed to an MCP server"},
	}
	if hardcoded {
		found.evidence.Detail = "literal value passed to an MCP server (value not recorded)"
		found.risks = []aibom.Risk{risk(riskHardcodedCred)}
	}
	s.add(found)
}
