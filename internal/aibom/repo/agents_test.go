package repo

import (
	"strings"
	"testing"

	"github.com/dhruv29/Pinaka/internal/aibom"
)

func TestScanListsAgentInstructionSkillAndPromptFiles(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		"CLAUDE.md":                       "be careful",
		"sub/AGENTS.md":                   "rules",
		".cursorrules":                    "x",
		".github/copilot-instructions.md": "x",
		".claude/agents/reviewer.md":      "x",
		".claude/skills/deploy/SKILL.md":  "x",
		".cursor/rules/style.mdc":         "x",
		"prompts/system_prompt.txt":       "You are",
		"README.md":                       "nothing",
		"docs/prompting.md":               "nothing",
	})
	want := map[string]string{
		"CLAUDE.md": typeInstructions, "sub/AGENTS.md": typeInstructions, ".cursorrules": typeInstructions,
		".github/copilot-instructions.md": typeInstructions, ".claude/agents/reviewer.md": typeAgentDef,
		".claude/skills/deploy/SKILL.md": typeSkill, ".cursor/rules/style.mdc": typeInstructions,
		"prompts/system_prompt.txt": typeSystemPrompt,
	}
	for name, kind := range want {
		if c := mustFind(t, bom, aibom.KindAgentTool, name); c.Properties[propType] != kind {
			t.Errorf("%s type = %q, want %q", name, c.Properties[propType], kind)
		}
	}
	mustNotFind(t, bom, aibom.KindAgentTool, "README.md")
	mustNotFind(t, bom, aibom.KindAgentTool, "docs/prompting.md")
	if c := mustFind(t, bom, aibom.KindAgentTool, "CLAUDE.md"); c.Supplier != "Anthropic" {
		t.Errorf("supplier = %q", c.Supplier)
	}
}

func TestScanListsMCPServersWithoutTheirSecrets(t *testing.T) {
	bom := scanFiles(t, map[string]string{
		".mcp.json": `{
  "mcpServers": {
    "github": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"],
               "env": {"OPENAI_API_KEY": "` + fakeOpenAIKey + `", "OTHER": "v"}},
    "local": {"command": "/usr/local/bin/my-server", "args": ["--port", "1"]},
    "remote": {"url": "https://user:secret@mcp.example.com/sse?token=abc"}
  }
}`,
		".vscode/settings.json": `{
  // editor settings
  "mcp": {"servers": {"shell": {"command": "bash", "args": ["-c", "run.sh"],}}},
  /* block
     comment */
  "url": "http://x//y"
}`,
		".codex/config.toml": "[mcp_servers.docs]\ncommand = \"uvx\"\nargs = [\"docs-mcp\"]\n[mcp_servers.docs.env]\nHF_TOKEN = \"" + fakeHFToken + "\"\n[other]\nx = 1\n",
		".cursor/mcp.json":   "{ not json",
	})
	gh := mustFind(t, bom, aibom.KindAgentTool, "mcp:github")
	if gh.Properties["command"] != "npx" || gh.Properties["package"] != "@modelcontextprotocol/server-github" ||
		riskSeverity(gh, "mcp-server-remote-or-shell") != aibom.SeverityMedium || gh.Evidence[0].Line != 3 {
		t.Errorf("github = %+v", gh)
	}
	if local := mustFind(t, bom, aibom.KindAgentTool, "mcp:local"); len(local.Risks) != 0 || local.Properties["command"] != "my-server" {
		t.Errorf("local = %+v", local)
	}
	if remote := mustFind(t, bom, aibom.KindAgentTool, "mcp:remote"); remote.Properties["url"] != "mcp.example.com" || len(remote.Risks) != 1 {
		t.Errorf("remote = %+v", remote)
	}
	if sh := mustFind(t, bom, aibom.KindAgentTool, "mcp:shell"); len(sh.Risks) != 1 {
		t.Errorf("shell = %+v", sh)
	}
	if docs := mustFind(t, bom, aibom.KindAgentTool, "mcp:docs"); docs.Properties["command"] != "uvx" || len(docs.Risks) != 1 {
		t.Errorf("docs = %+v", docs)
	}
	for _, name := range []string{"OPENAI_API_KEY", "HF_TOKEN"} {
		if riskSeverity(mustFind(t, bom, aibom.KindCredential, name), "hardcoded-ai-credential") == "" {
			t.Errorf("%s literal in MCP env not flagged", name)
		}
	}
	out := marshal(t, bom)
	for _, leaked := range []string{fakeOpenAIKey, fakeHFToken, "secret@", "token=abc"} {
		if strings.Contains(out, leaked) {
			t.Errorf("output contains %q", leaked)
		}
	}
	if !hasNote(bom, ".cursor/mcp.json") {
		t.Errorf("notes = %v", bom.Notes)
	}
}
