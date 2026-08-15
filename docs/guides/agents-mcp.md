# Driving adastra from AI agents

adastra is agent-native: JSON-first output, a built-in MCP server, and a
skills pack that teaches agents the workflows (not just the commands).

## MCP server

```
adastra mcp serve        # stdio MCP server
```

Register it:

```
# Claude Code
claude mcp add adastra -- adastra mcp serve

# Claude Desktop (claude_desktop_config.json)
{ "mcpServers": { "adastra": { "command": "adastra", "args": ["mcp", "serve"] } } }
```

~30 tools mirror the command tree (`campaigns_list`, `harvest_run`,
`aso_research`, `roas_report`, …).

**Safety contract:** mutating tools take a `confirm` boolean. Without
`confirm: true` they execute with `--dry-run` and return the would-be changes —
an agent must show you the plan before it can spend a cent. Reads run as-is.

## Skills pack

```
adastra install-skills            # → ./.claude/skills/
adastra install-skills --global   # → ~/.claude/skills/
```

Installs three playbooks any skills-aware agent can load:

- **harvest-playbook** — the search-term promotion loop, safely
- **launch-playbook** — research → scaffold → seed → track for a new app
- **audit-playbook** — find wasted spend and metadata problems, read-only

## Plain scripting

Everything is also just JSON on stdout:

```
adastra reports keywords --since 7d | jq '.[] | select(.totalInstalls == "0")'
adastra aso track alerts --drop 5 && echo "ranks healthy"
```
