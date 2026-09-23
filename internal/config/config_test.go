package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHomeResolution(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-home")
	t.Setenv(HomeEnv, custom)
	if got := Home(); got != custom {
		t.Errorf("Home() with env = %q, want %q", got, custom)
	}

	t.Setenv(HomeEnv, "")
	got := Home()
	if !strings.HasSuffix(filepath.ToSlash(got), "/"+DefaultHomeDirName) {
		t.Errorf("Home() default = %q, want it to end with %q", got, DefaultHomeDirName)
	}
}

func TestLayoutPaths(t *testing.T) {
	t.Parallel()
	l := NewLayout(filepath.FromSlash("/tmp/oml"))
	cases := map[string]string{
		l.Bin:        "/tmp/oml/bin",
		l.Models:     "/tmp/oml/models/laya",
		l.ConfigPath: "/tmp/oml/config.toml",
		l.State:      "/tmp/oml/state",
		l.Backups:    "/tmp/oml/backups",
		l.Skills:     "/tmp/oml/skills",
	}
	for got, want := range cases {
		if filepath.ToSlash(got) != want {
			t.Errorf("layout path = %q, want %q", got, want)
		}
	}
	if got := filepath.ToSlash(l.VariantDir("multilingual")); got != "/tmp/oml/models/laya/multilingual" {
		t.Errorf("VariantDir(multilingual) = %q", got)
	}
	if got := filepath.ToSlash(l.VariantDir("english")); got != "/tmp/oml/models/laya" {
		t.Errorf("VariantDir(english) = %q, want the models root", got)
	}
}

func TestDefaultConfigMatchesSpec(t *testing.T) {
	t.Parallel()
	c := Default()
	if c.Version != 1 || c.Backend != "vulkan" || c.Model != "multilingual" || c.Port != 45292 {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.IdleTimeout.Duration != 30*time.Minute {
		t.Errorf("IdleTimeout = %v, want 30m", c.IdleTimeout.Duration)
	}
	if c.Provider != "local" || c.Precision.Strict || c.Batch.MaxQuestions != 8 || c.Batch.MaxPending != 32 || c.Batch.WaitMS != 2 {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.Tools.AutoAccept != 0.8 {
		t.Errorf("Tools.AutoAccept = %v, want 0.8", c.Tools.AutoAccept)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("default config must validate: %v", err)
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	t.Parallel()
	l := NewLayout(t.TempDir())
	c := Default()
	c.Backend = "cuda"
	c.Agents.Registered = []string{"claude", "pi"}
	c.IdleTimeout.Duration = 0
	if err := Save(l, c); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	got, err := Load(l)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if got.Backend != "cuda" || len(got.Agents.Registered) != 2 || got.IdleTimeout.Duration != 0 {
		t.Errorf("round trip lost data: %+v", got)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	t.Parallel()
	l := NewLayout(t.TempDir())
	c, err := Load(l)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if c.Backend != "vulkan" {
		t.Errorf("missing file should yield defaults, got %+v", c)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		toml string
		want string
	}{
		{"bad backend", `backend = "metal"`, "backend"},
		{"bad model", `model = "klingon"`, "model"},
		{"bad provider", `provider = "openai"`, "provider"},
		{"port too low", `port = 80`, "port"},
		{"bad duration", `idle_timeout = "soon"`, "idle_timeout"},
		{"auto_accept out of range", "[tools]\nauto_accept = 1.5", "auto_accept"},
		{"batch zero", "[batch]\nmax_questions = 0", "max_questions"},
		{"not toml", `this is not = = toml`, "parse"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := NewLayout(t.TempDir())
			os.MkdirAll(filepath.Dir(l.ConfigPath), 0o755)
			os.WriteFile(l.ConfigPath, []byte(tc.toml+"\n"), 0o644)
			_, err := Load(l)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Load() = %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("OHMYLAYA_PORT", "45300")
	t.Setenv("OHMYLAYA_PROVIDER", "typesafe")
	t.Setenv("OHMYLAYA_LOG_LEVEL", "debug")
	c := Default()
	if err := ApplyEnv(c, os.Getenv); err != nil {
		t.Fatal(err)
	}
	if c.Port != 45300 || c.Provider != "typesafe" || c.LogLevel != "debug" {
		t.Errorf("env not applied: %+v", c)
	}

	t.Setenv("OHMYLAYA_PORT", "abc")
	err := ApplyEnv(Default(), os.Getenv)
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("bad port env err = %v, want ErrInvalid", err)
	}
}

func TestDefaultTOMLGolden(t *testing.T) {
	t.Parallel()
	l := NewLayout(t.TempDir())
	if err := Save(l, Default()); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(l.ConfigPath)
	want, err := os.ReadFile(filepath.Join("testdata", "default.toml"))
	if err != nil {
		t.Fatalf("golden missing: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("default config.toml differs from golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
