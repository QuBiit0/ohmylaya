package agents

import (
	"fmt"
	"path/filepath"
)

// pi integrates Pi through ~/.pi/agent/mcp.json, which belongs to the
// pi-mcp-adapter package. When that file is absent Pi has no MCP support and
// registration is skipped with an explanation.
type pi struct{}

func (pi) ID() string   { return "pi" }
func (pi) Name() string { return "Pi" }

func (p pi) Detect(env Env) (bool, string) {
	if bin, ok := onPath(env, "pi"); ok {
		return true, bin
	}
	if exists(p.agentDir(env)) {
		return true, "~/.pi/agent found"
	}
	return false, "pi CLI not on PATH and ~/.pi/agent missing"
}

func (pi) agentDir(env Env) string { return filepath.Join(env.Home, ".pi", "agent") }

func (p pi) ConfigPath(env Env) string { return filepath.Join(p.agentDir(env), "mcp.json") }

// SkillDir is the shared agent-skills directory Pi scans.
func (pi) SkillDir(env Env) string {
	return filepath.Join(env.Home, ".agents", "skills", ServerID)
}

func (p pi) registry(env Env) jsonRegistry {
	return jsonRegistry{agent: p.ID(), path: p.ConfigPath(env), keys: []string{"mcpServers"}}
}

func (p pi) Register(env Env, binPath string) error {
	if !exists(p.ConfigPath(env)) {
		return fmt.Errorf("%w: %s does not exist; install the pi-mcp-adapter package in Pi first", ErrNoRegistry, p.ConfigPath(env))
	}
	return p.registry(env).set(env, map[string]any{"command": binPath, "args": []string{"mcp"}, "lifecycle": "lazy"})
}

func (p pi) Unregister(env Env) (bool, error) { return p.registry(env).remove(env) }

func (p pi) Status(env Env, binPath string) Status {
	st := baseStatus(p, env)
	if !exists(p.ConfigPath(env)) {
		st.Detail = st.Detail + "; no mcp.json (pi-mcp-adapter not installed)"
		return st
	}
	fillJSONStatus(&st, p.registry(env), binPath)
	return st
}
