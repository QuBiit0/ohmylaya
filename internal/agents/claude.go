package agents

import "path/filepath"

// claude integrates Claude Code through the user-scoped ~/.claude.json.
type claude struct{}

func (claude) ID() string   { return "claude" }
func (claude) Name() string { return "Claude Code" }

func (claude) Detect(env Env) (bool, string) {
	if p, ok := onPath(env, "claude"); ok {
		return true, p
	}
	if exists(filepath.Join(env.Home, ".claude")) || exists(filepath.Join(env.Home, ".claude.json")) {
		return true, "~/.claude found"
	}
	return false, "claude CLI not on PATH and ~/.claude missing"
}

func (claude) ConfigPath(env Env) string { return filepath.Join(env.Home, ".claude.json") }
func (claude) SkillDir(env Env) string {
	return filepath.Join(env.Home, ".claude", "skills", ServerID)
}

func (c claude) registry(env Env) jsonRegistry {
	return jsonRegistry{agent: c.ID(), path: c.ConfigPath(env), keys: []string{"mcpServers"}}
}

func (c claude) Register(env Env, binPath string) error {
	return c.registry(env).set(env, map[string]any{"type": "stdio", "command": binPath, "args": []string{"mcp"}})
}

func (c claude) Unregister(env Env) (bool, error) { return c.registry(env).remove(env) }

func (c claude) Status(env Env, binPath string) Status {
	st := baseStatus(c, env)
	fillJSONStatus(&st, c.registry(env), binPath)
	return st
}
