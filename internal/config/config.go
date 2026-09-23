// Package config owns $OHMYLAYA_HOME, its layout and config.toml.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	// HomeEnv overrides the home directory.
	HomeEnv = "OHMYLAYA_HOME"
	// DefaultHomeDirName is created under the user's home directory.
	DefaultHomeDirName = ".ohmylaya"
	// DefaultPort spells LAYA on a phone keypad.
	DefaultPort = 45292
)

// ErrInvalid marks configuration values that fail validation.
var ErrInvalid = errors.New("config: invalid value")

// Home returns $OHMYLAYA_HOME or ~/.ohmylaya.
func Home() string {
	if h := os.Getenv(HomeEnv); h != "" {
		return h
	}
	base, err := os.UserHomeDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, DefaultHomeDirName)
}

// Layout holds every path under the home directory.
type Layout struct {
	Home       string
	Bin        string
	Models     string
	ConfigPath string
	State      string
	Backups    string
	Skills     string
}

// NewLayout derives all paths from home.
func NewLayout(home string) Layout {
	return Layout{
		Home:       home,
		Bin:        filepath.Join(home, "bin"),
		Models:     filepath.Join(home, "models", "laya"),
		ConfigPath: filepath.Join(home, "config.toml"),
		State:      filepath.Join(home, "state"),
		Backups:    filepath.Join(home, "backups"),
		Skills:     filepath.Join(home, "skills"),
	}
}

// VariantDir mirrors the Hugging Face layout: english at the root, the
// others in a subdirectory.
func (l Layout) VariantDir(variant string) string {
	if variant == "english" {
		return l.Models
	}
	return filepath.Join(l.Models, variant)
}

// Duration is a time.Duration that serialises as a human string.
type Duration struct{ time.Duration }

// MarshalText renders "30m", "1h", or the full Go form for odd values.
func (d Duration) MarshalText() ([]byte, error) {
	switch {
	case d.Duration == 0:
		return []byte("0s"), nil
	case d.Duration%time.Hour == 0:
		return []byte(strconv.Itoa(int(d.Duration/time.Hour)) + "h"), nil
	case d.Duration%time.Minute == 0:
		return []byte(strconv.Itoa(int(d.Duration/time.Minute)) + "m"), nil
	}
	return []byte(d.Duration.String()), nil
}

// UnmarshalText parses any Go duration string.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("%w: idle_timeout %q", ErrInvalid, string(b))
	}
	d.Duration = v
	return nil
}

// Config is the user configuration.
type Config struct {
	Version     int      `toml:"version"`
	Backend     string   `toml:"backend"`
	Model       string   `toml:"model"`
	Port        int      `toml:"port"`
	IdleTimeout Duration `toml:"idle_timeout"`
	Provider    string   `toml:"provider"`
	LogLevel    string   `toml:"log_level"`
	Precision   struct {
		Strict bool `toml:"strict"`
	} `toml:"precision"`
	Batch struct {
		MaxQuestions int `toml:"max_questions"`
		MaxPending   int `toml:"max_pending"`
		WaitMS       int `toml:"wait_ms"`
	} `toml:"batch"`
	Tools struct {
		AutoAccept float64 `toml:"auto_accept"`
	} `toml:"tools"`
	Agents struct {
		Registered []string `toml:"registered"`
	} `toml:"agents"`
}

// Default returns the configuration the installer writes on a fresh machine.
func Default() *Config {
	c := &Config{
		Version:     1,
		Backend:     "vulkan",
		Model:       "multilingual",
		Port:        DefaultPort,
		IdleTimeout: Duration{30 * time.Minute},
		Provider:    "local",
		LogLevel:    "info",
	}
	c.Batch.MaxQuestions = 8
	c.Batch.MaxPending = 32
	c.Batch.WaitMS = 2
	c.Tools.AutoAccept = 0.8
	c.Agents.Registered = []string{}
	return c
}

var (
	backends  = map[string]bool{"vulkan": true, "cuda": true, "cpu": true}
	models    = map[string]bool{"multilingual": true, "english": true, "typed-decisions": true}
	providers = map[string]bool{"local": true, "typesafe": true}
	logLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
)

// Validate checks enumerations and ranges.
func (c *Config) Validate() error {
	switch {
	case c.Version != 1:
		return fmt.Errorf("%w: version %d", ErrInvalid, c.Version)
	case !backends[c.Backend]:
		return fmt.Errorf("%w: backend %q (vulkan, cuda, cpu)", ErrInvalid, c.Backend)
	case !models[c.Model]:
		return fmt.Errorf("%w: model %q (multilingual, english, typed-decisions)", ErrInvalid, c.Model)
	case !providers[c.Provider]:
		return fmt.Errorf("%w: provider %q (local, typesafe)", ErrInvalid, c.Provider)
	case !logLevels[c.LogLevel]:
		return fmt.Errorf("%w: log_level %q", ErrInvalid, c.LogLevel)
	case c.Port < 1024 || c.Port > 65535:
		return fmt.Errorf("%w: port %d must be between 1024 and 65535", ErrInvalid, c.Port)
	case c.IdleTimeout.Duration < 0:
		return fmt.Errorf("%w: idle_timeout must not be negative", ErrInvalid)
	case c.Batch.MaxQuestions < 1 || c.Batch.MaxQuestions > 4096:
		return fmt.Errorf("%w: batch.max_questions %d", ErrInvalid, c.Batch.MaxQuestions)
	case c.Batch.MaxPending < 1 || c.Batch.MaxPending > 256:
		return fmt.Errorf("%w: batch.max_pending %d", ErrInvalid, c.Batch.MaxPending)
	case c.Batch.WaitMS < 0 || c.Batch.WaitMS > 1000:
		return fmt.Errorf("%w: batch.wait_ms %d", ErrInvalid, c.Batch.WaitMS)
	case c.Tools.AutoAccept < 0 || c.Tools.AutoAccept > 1:
		return fmt.Errorf("%w: tools.auto_accept %v must be within 0 and 1", ErrInvalid, c.Tools.AutoAccept)
	}
	return nil
}

// Load reads config.toml, applying defaults for missing keys. A missing file
// yields the defaults. Environment overrides are not applied here.
func Load(l Layout) (*Config, error) {
	c := Default()
	data, err := os.ReadFile(l.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", l.ConfigPath, err)
	}
	if err := toml.Unmarshal(data, c); err != nil {
		var derr *toml.DecodeError
		if errors.As(err, &derr) {
			row, col := derr.Position()
			return nil, fmt.Errorf("config: parse %s at %d:%d: %s", l.ConfigPath, row, col, derr.Error())
		}
		if errors.Is(err, ErrInvalid) {
			return nil, err
		}
		return nil, fmt.Errorf("config: parse %s: %w", l.ConfigPath, err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Save writes config.toml atomically.
func Save(l Layout, c *Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(l.ConfigPath), 0o755); err != nil {
		return err
	}
	tmp := l.ConfigPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, l.ConfigPath)
}

// ApplyEnv overlays OHMYLAYA_PORT, OHMYLAYA_PROVIDER and OHMYLAYA_LOG_LEVEL.
// The API key for the hosted provider is read where it is used and never
// stored in the configuration.
func ApplyEnv(c *Config, getenv func(string) string) error {
	if v := getenv("OHMYLAYA_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%w: OHMYLAYA_PORT %q", ErrInvalid, v)
		}
		c.Port = p
	}
	if v := getenv("OHMYLAYA_PROVIDER"); v != "" {
		c.Provider = v
	}
	if v := getenv("OHMYLAYA_LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}
	return c.Validate()
}
