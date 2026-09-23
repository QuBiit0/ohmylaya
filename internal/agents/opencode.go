package agents

import (
	"os"
	"path/filepath"

	"github.com/QuBiit0/ohmylaya/internal/cfgfile"
)

// opencode integrates OpenCode through opencode.json or opencode.jsonc.
// Version 1 configs keep servers directly under "mcp"; version 2 configs
// keep them under "mcp.servers".
type opencode struct{}

func (opencode) ID() string   { return "opencode" }
func (opencode) Name() string { return "OpenCode" }

func (o opencode) Detect(env Env) (bool, string) {
	if p, ok := onPath(env, "opencode"); ok {
		return true, p
	}
	if exists(o.dir(env)) {
		return true, o.dir(env) + " found"
	}
	return false, "opencode CLI not on PATH and config directory missing"
}

func (opencode) dir(env Env) string { return filepath.Join(env.ConfigHome, "opencode") }

// ConfigPath prefers whichever file exists, defaulting to opencode.json.
func (o opencode) ConfigPath(env Env) string {
	jsonc := filepath.Join(o.dir(env), "opencode.jsonc")
	if exists(jsonc) && !exists(filepath.Join(o.dir(env), "opencode.json")) {
		return jsonc
	}
	return filepath.Join(o.dir(env), "opencode.json")
}

func (o opencode) SkillDir(env Env) string { return filepath.Join(o.dir(env), "skills", ServerID) }

// keys detects the schema version from the file content.
func (o opencode) keys(env Env) []string {
	src, err := os.ReadFile(o.ConfigPath(env))
	if err == nil {
		if v, err := cfgfile.ParseJSONC(src); err == nil {
			if m, ok := v.(map[string]any); ok {
				if mcp, ok := m["mcp"].(map[string]any); ok {
					if _, ok := mcp["servers"].(map[string]any); ok {
						return []string{"mcp", "servers"}
					}
				}
			}
		}
	}
	return []string{"mcp"}
}

func (o opencode) registry(env Env) jsonRegistry {
	return jsonRegistry{agent: o.ID(), path: o.ConfigPath(env), keys: o.keys(env)}
}

func (o opencode) Register(env Env, binPath string) error {
	r := o.registry(env)
	entry := map[string]any{"type": "local", "command": []string{binPath, "mcp"}}
	if len(r.keys) == 1 {
		entry["enabled"] = true
	}
	return r.set(env, entry)
}

func (o opencode) Unregister(env Env) (bool, error) { return o.registry(env).remove(env) }

func (o opencode) Status(env Env, binPath string) Status {
	st := baseStatus(o, env)
	fillJSONStatus(&st, o.registry(env), binPath)
	return st
}
