package doctor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/QuBiit0/ohmylaya/internal/agents"
	"github.com/QuBiit0/ohmylaya/internal/config"
	"github.com/QuBiit0/ohmylaya/internal/manifest"
	"github.com/QuBiit0/ohmylaya/internal/skill"
)

var fakeEngine string

func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "fakeengine-doctor")
	fakeEngine = filepath.Join(dir, "fakeengine")
	if runtime.GOOS == "windows" {
		fakeEngine += ".exe"
	}
	b := exec.Command("go", "build", "-o", fakeEngine, "../sidecar/testdata/fakeengine")
	b.Stderr = os.Stderr
	if err := b.Run(); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type probe struct{ libs map[string]bool }

func (probe) OS() string                    { return runtime.GOOS }
func (probe) Arch() string                  { return runtime.GOARCH }
func (p probe) HasLibrary(name string) bool { return p.libs[name] }
func (probe) GLibCVersion() string          { return "2.40" }

func testDeps(t *testing.T) Deps {
	t.Helper()
	home := t.TempDir()
	l := config.NewLayout(filepath.Join(home, ".ohmylaya"))
	data, _ := os.ReadFile(fakeEngine)
	sum := sha256.Sum256(data)
	m := &manifest.Manifest{Schema: 1}
	m.Engine.Project, m.Engine.Tag = "t", "r0"
	m.Engine.Assets = []manifest.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, Backend: "cpu", Name: "engine", URL: "http://x", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), SupportsCPU: true}}
	m.Models.Repo, m.Models.Revision = "laya", "rev"
	w := sha256.Sum256([]byte("weights"))
	m.Models.Variants = map[string]*manifest.Variant{"multilingual": {Prefix: "multilingual/", Context: 1024, HeadMaxLen: 256, Files: []manifest.File{{Path: "model.safetensors", Size: 7, SHA256: hex.EncodeToString(w[:])}}}}
	userHome := filepath.Join(home, "user")
	os.MkdirAll(userHome, 0o755)
	engine := filepath.Join(l.Bin, "laya-cli")
	if runtime.GOOS == "windows" {
		engine += ".exe"
	}
	return Deps{
		Layout:     l,
		Manifest:   m,
		Probe:      probe{},
		Env:        agents.Env{Home: userHome, ConfigHome: filepath.Join(userHome, ".config"), BackupsDir: l.Backups, LookPath: func(string) (string, error) { return "", errors.New("no") }},
		BinPath:    filepath.Join(home, "ohmylaya"),
		Version:    "1.0.0",
		Client:     &http.Client{},
		EnginePath: engine,
	}
}

func byID(r Report, id string) Check {
	for _, c := range r.Checks {
		if c.ID == id {
			return c
		}
	}
	return Check{}
}

func TestFreshMachineFailsWithFixes(t *testing.T) {
	d := testDeps(t)
	r := Run(context.Background(), d)
	for _, id := range []string{"home", "engine", "model"} {
		c := byID(r, id)
		if c.Status != Fail || !strings.Contains(c.Fix, "ohmylaya install") {
			t.Errorf("%s = %+v, want FAIL with install fix", id, c)
		}
	}
	if byID(r, "agents").Status != Warn || byID(r, "version").Status != Skip || byID(r, "sidecar").Status != Skip {
		t.Errorf("checks = %+v", r.Checks)
	}
	if r.ExitCode(false) != 1 {
		t.Error("exit must be 1 with failures")
	}
	var buf bytes.Buffer
	Print(&buf, r)
	if !strings.Contains(buf.String(), "fix: ohmylaya install") {
		t.Errorf("print = %s", buf.String())
	}
}

func TestHealthyInstallPassesAndSmokeRuns(t *testing.T) {
	d := testDeps(t)
	cfg := config.Default()
	cfg.Backend = "cpu"
	cfg.Port = 45600
	if err := config.Save(d.Layout, cfg); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(d.Layout.Bin, 0o755)
	data, _ := os.ReadFile(fakeEngine)
	os.WriteFile(d.EnginePath, data, 0o755)
	os.MkdirAll(d.Layout.VariantDir("multilingual"), 0o755)
	os.WriteFile(filepath.Join(d.Layout.VariantDir("multilingual"), "model.safetensors"), []byte("weights"), 0o644)
	a, _ := agents.ByID("claude")
	os.WriteFile(d.BinPath, []byte("x"), 0o755)
	a.Register(d.Env, d.BinPath)
	skill.Install(a.SkillDir(d.Env), "0.9.0")
	d.Smoke = true
	d.LatestRelease = func(context.Context) (string, error) { return "v1.0.0", nil }

	r := Run(context.Background(), d)
	for _, id := range []string{"platform", "home", "engine", "runtime-deps", "model", "port", "agents", "version", "smoke"} {
		if c := byID(r, id); c.Status != Pass {
			t.Errorf("%s = %+v, want PASS", id, c)
		}
	}
	if c := byID(r, "skill:claude"); c.Status != Warn || !strings.Contains(c.Detail, "outdated") {
		t.Errorf("skill:claude = %+v, want outdated WARN", c)
	}
	if r.ExitCode(false) != 0 || r.ExitCode(true) != 1 {
		t.Errorf("exit codes = %d/%d", r.ExitCode(false), r.ExitCode(true))
	}
	var buf bytes.Buffer
	if err := PrintJSON(&buf, r); err != nil || !strings.Contains(buf.String(), `"checks"`) {
		t.Errorf("json = %v %s", err, buf.String())
	}
}

func TestVulkanWithoutLoaderFails(t *testing.T) {
	d := testDeps(t)
	cfg := config.Default()
	cfg.Backend = "vulkan"
	config.Save(d.Layout, cfg)
	r := Run(context.Background(), d)
	c := byID(r, "runtime-deps")
	if c.Status != Fail || !strings.Contains(c.Fix, "cpu") {
		t.Errorf("runtime-deps = %+v", c)
	}
}

func TestVersionOfflineSkips(t *testing.T) {
	d := testDeps(t)
	d.LatestRelease = func(context.Context) (string, error) { return "", errors.New("dial tcp: no route") }
	r := Run(context.Background(), d)
	if c := byID(r, "version"); c.Status != Skip {
		t.Errorf("version = %+v, want SKIP offline", c)
	}
}
