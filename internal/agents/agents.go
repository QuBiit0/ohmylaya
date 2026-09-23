// Package agents registers the ohmylaya MCP server in each supported coding
// agent using only that agent's own MCP registry file. No agent framework is
// read or written.
package agents

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/QuBiit0/ohmylaya/internal/cfgfile"
)

// ServerID is the key under which ohmylaya appears in every registry.
const ServerID = "ohmylaya"

// ErrNoRegistry is returned when the agent has no MCP registry to write.
var ErrNoRegistry = errors.New("agents: no MCP registry available")

// Env abstracts the host so adapters are testable.
type Env struct {
	// Home is the user's home directory.
	Home string
	// ConfigHome is $XDG_CONFIG_HOME or Home/.config.
	ConfigHome string
	// BackupsDir receives a copy of every file before it is written.
	BackupsDir string
	// LookPath resolves an executable name, like exec.LookPath.
	LookPath func(string) (string, error)
}

// Status describes one agent's integration state.
type Status struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Detected    bool   `json:"detected"`
	Detail      string `json:"detail,omitempty"`
	ConfigPath  string `json:"config_path"`
	Registered  bool   `json:"registered"`
	Command     string `json:"command,omitempty"`
	Stale       bool   `json:"stale"`
	SkillDir    string `json:"skill_dir"`
	RegistryErr string `json:"registry_error,omitempty"`
}

// Adapter integrates one agent.
type Adapter interface {
	ID() string
	Name() string
	Detect(env Env) (bool, string)
	ConfigPath(env Env) string
	SkillDir(env Env) string
	Register(env Env, binPath string) error
	Unregister(env Env) (bool, error)
	Status(env Env, binPath string) Status
}

// All returns the adapters in display order.
func All() []Adapter {
	return []Adapter{claude{}, codex{}, opencode{}, pi{}}
}

// ByID returns the adapter for an id.
func ByID(id string) (Adapter, bool) {
	for _, a := range All() {
		if a.ID() == id {
			return a, true
		}
	}
	return nil, false
}

// HostEnv builds the Env for the real host.
func HostEnv(backupsDir string, lookPath func(string) (string, error)) Env {
	home, _ := os.UserHomeDir()
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	return Env{Home: home, ConfigHome: cfg, BackupsDir: backupsDir, LookPath: lookPath}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func onPath(env Env, name string) (string, bool) {
	if env.LookPath == nil {
		return "", false
	}
	p, err := env.LookPath(name)
	return p, err == nil
}

// writeWithBackup backs up path when it exists and writes data atomically.
func writeWithBackup(env Env, agent, path string, data []byte) error {
	if exists(path) {
		if _, err := cfgfile.Backup(env.BackupsDir, agent, path); err != nil {
			return err
		}
	}
	return cfgfile.WriteAtomic(path, data)
}

// jsonRegistry handles registries that are JSON or JSONC objects.
type jsonRegistry struct {
	agent string
	path  string
	keys  []string
}

func (r jsonRegistry) set(env Env, value any) error {
	src, err := os.ReadFile(r.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	v, err := json.Marshal(value)
	if err != nil {
		return err
	}
	out, err := cfgfile.SetJSONPath(src, append(append([]string{}, r.keys...), ServerID), v)
	if err != nil {
		return err
	}
	return writeWithBackup(env, r.agent, r.path, out)
}

func (r jsonRegistry) remove(env Env) (bool, error) {
	src, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	out, removed, err := cfgfile.RemoveJSONPath(src, append(append([]string{}, r.keys...), ServerID))
	if err != nil || !removed {
		return removed, err
	}
	return true, writeWithBackup(env, r.agent, r.path, out)
}

// entry returns the registered entry as a generic map, if any.
func (r jsonRegistry) entry() (map[string]any, string, error) {
	src, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	v, err := cfgfile.ParseJSONC(src)
	if err != nil {
		return nil, "", err
	}
	cur := v
	for _, k := range append(append([]string{}, r.keys...), ServerID) {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, "", nil
		}
		cur, ok = m[k]
		if !ok {
			return nil, "", nil
		}
	}
	m, _ := cur.(map[string]any)
	return m, commandOf(m), nil
}

// commandOf extracts the executable from either a string command or a
// command array.
func commandOf(entry map[string]any) string {
	switch c := entry["command"].(type) {
	case string:
		return c
	case []any:
		if len(c) > 0 {
			if s, ok := c[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

// stale reports whether a registered command no longer matches the binary
// or no longer exists.
func stale(command, binPath string) bool {
	if command == "" {
		return true
	}
	if samePath(command, binPath) {
		return false
	}
	return !exists(command)
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func baseStatus(a Adapter, env Env) Status {
	det, detail := a.Detect(env)
	return Status{ID: a.ID(), Name: a.Name(), Detected: det, Detail: detail, ConfigPath: a.ConfigPath(env), SkillDir: a.SkillDir(env)}
}

func fillJSONStatus(st *Status, r jsonRegistry, binPath string) {
	entry, cmd, err := r.entry()
	if err != nil {
		st.RegistryErr = err.Error()
		return
	}
	if entry == nil {
		return
	}
	st.Registered = true
	st.Command = cmd
	st.Stale = stale(cmd, binPath)
}
