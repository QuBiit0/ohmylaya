package agents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/QuBiit0/ohmylaya/internal/cfgfile"
)

// codex integrates OpenAI Codex through ~/.codex/config.toml.
type codex struct{}

func (codex) ID() string   { return "codex" }
func (codex) Name() string { return "Codex" }

func (codex) Detect(env Env) (bool, string) {
	if p, ok := onPath(env, "codex"); ok {
		return true, p
	}
	if exists(filepath.Join(env.Home, ".codex")) {
		return true, "~/.codex found"
	}
	return false, "codex CLI not on PATH and ~/.codex missing"
}

func (codex) ConfigPath(env Env) string { return filepath.Join(env.Home, ".codex", "config.toml") }

// SkillDir is the shared agent-skills directory Codex scans. It must be a
// real directory; Codex does not follow a symlinked ~/.agents/skills.
func (codex) SkillDir(env Env) string {
	return filepath.Join(env.Home, ".agents", "skills", ServerID)
}

const codexTable = "mcp_servers." + ServerID

func (c codex) Register(env Env, binPath string) error {
	path := c.ConfigPath(env)
	src, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(src) > 0 {
		var probe map[string]any
		if err := toml.Unmarshal(src, &probe); err != nil {
			return fmt.Errorf("agents: %s is not valid TOML: %w", path, err)
		}
	}
	body := fmt.Sprintf("command = %s\nargs = [\"mcp\"]\n", tomlString(binPath))
	out := cfgfile.SetTOMLTable(src, codexTable, body)
	return writeWithBackup(env, c.ID(), path, out)
}

func (c codex) Unregister(env Env) (bool, error) {
	path := c.ConfigPath(env)
	src, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	out, removed := cfgfile.RemoveTOMLTable(src, codexTable)
	if !removed {
		return false, nil
	}
	return true, writeWithBackup(env, c.ID(), path, out)
}

func (c codex) Status(env Env, binPath string) Status {
	st := baseStatus(c, env)
	src, err := os.ReadFile(c.ConfigPath(env))
	if errors.Is(err, os.ErrNotExist) {
		return st
	}
	if err != nil {
		st.RegistryErr = err.Error()
		return st
	}
	var doc struct {
		Servers map[string]struct {
			Command string `toml:"command"`
		} `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(src, &doc); err != nil {
		st.RegistryErr = err.Error()
		return st
	}
	if s, ok := doc.Servers[ServerID]; ok {
		st.Registered = true
		st.Command = s.Command
		st.Stale = stale(s.Command, binPath)
	}
	return st
}

// tomlString quotes a basic string, escaping backslashes and quotes.
func tomlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
