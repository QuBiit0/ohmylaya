package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/acquire"
	"github.com/QuBiit0/ohmylaya/internal/agents"
	"github.com/QuBiit0/ohmylaya/internal/config"
	"github.com/QuBiit0/ohmylaya/internal/manifest"
	"github.com/QuBiit0/ohmylaya/internal/skill"
)

var (
	fakeEngineBytes []byte
	timeZero        = time.Unix(0, 0)
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeengine-install")
	if err != nil {
		panic(err)
	}
	bin := filepath.Join(dir, "fakeengine")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "../sidecar/testdata/fakeengine")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("build fakeengine: " + err.Error())
	}
	fakeEngineBytes, _ = os.ReadFile(bin)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type fakeProbe struct{}

func (fakeProbe) OS() string                  { return runtime.GOOS }
func (fakeProbe) Arch() string                { return runtime.GOARCH }
func (fakeProbe) HasLibrary(name string) bool { return false }
func (fakeProbe) GLibCVersion() string        { return "2.40" }

func digest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// fixture serves a fake engine and a tiny model tree and returns a manifest
// pointing at it.
func fixture(t *testing.T) (*httptest.Server, *manifest.Manifest) {
	t.Helper()
	files := map[string][]byte{
		"/engine": fakeEngineBytes,
		"/laya/resolve/rev/multilingual/model.safetensors":    []byte("weights"),
		"/laya/resolve/rev/multilingual/rl_agent_config.json": []byte(`{"a":1}`),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			return
		}
		http.ServeContent(w, r, "", timeZero, bytes.NewReader(b))
	}))
	t.Cleanup(srv.Close)
	m := &manifest.Manifest{Schema: 1}
	m.Engine.Project, m.Engine.Tag = "test/engine", "r0"
	m.Engine.Assets = []manifest.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, Backend: "cpu", Name: "engine", URL: srv.URL + "/engine", Size: int64(len(fakeEngineBytes)), SHA256: digest(fakeEngineBytes), SupportsCPU: true}}
	m.Models.Repo, m.Models.Revision = "laya", "rev"
	m.Models.Variants = map[string]*manifest.Variant{"multilingual": {Prefix: "multilingual/", Context: 1024, HeadMaxLen: 256, Files: []manifest.File{
		{Path: "model.safetensors", Size: 7, SHA256: digest([]byte("weights"))},
		{Path: "rl_agent_config.json", Size: 7, SHA256: digest([]byte(`{"a":1}`))},
	}}}
	// FileURL builds huggingface URLs; override through a client transport.
	return srv, m
}

// rewrite sends huggingface.co requests to the fixture server.
type rewrite struct{ base string }

func (rw rewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "huggingface.co" {
		u := rw.base + r.URL.Path
		nr := r.Clone(r.Context())
		var err error
		nr.URL, err = nr.URL.Parse(u)
		if err != nil {
			return nil, err
		}
		nr.Host = nr.URL.Host
		return http.DefaultTransport.RoundTrip(nr)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func deps(t *testing.T, srv *httptest.Server, m *manifest.Manifest) Deps {
	t.Helper()
	home := t.TempDir()
	userHome := filepath.Join(home, "user")
	os.MkdirAll(filepath.Join(userHome, ".claude"), 0o755)
	os.MkdirAll(filepath.Join(userHome, ".pi", "agent"), 0o755)
	os.WriteFile(filepath.Join(userHome, ".pi", "agent", "mcp.json"), []byte("{\n  \"mcpServers\": {}\n}\n"), 0o644)
	l := config.NewLayout(filepath.Join(home, ".ohmylaya"))
	return Deps{
		Layout:   l,
		Manifest: m,
		Probe:    fakeProbe{},
		Client:   &http.Client{Transport: rewrite{base: srv.URL}},
		Env:      agents.Env{Home: userHome, ConfigHome: filepath.Join(userHome, ".config"), BackupsDir: l.Backups, LookPath: func(string) (string, error) { return "", errors.New("no") }},
		BinPath:  filepath.Join(home, "ohmylaya"),
		Version:  "9.9.9",
		Out:      &bytes.Buffer{},
	}
}

func TestInstallEndToEndThenIdempotentThenUninstall(t *testing.T) {
	srv, m := fixture(t)
	d := deps(t, srv, m)
	ctx := context.Background()

	rep, err := Run(ctx, d, Options{Backend: "auto", Agents: []string{"all"}, Yes: true})
	if err != nil {
		t.Fatalf("Run() = %v\n%s", err, d.Out.(*bytes.Buffer).String())
	}
	if rep.Backend != "cpu" || rep.Model != "multilingual" || !rep.SmokeRan || !rep.Changed {
		t.Errorf("report = %+v", rep)
	}
	if len(rep.Downloaded) != 3 {
		t.Errorf("downloaded = %v, want engine and two model files", rep.Downloaded)
	}
	if err := acquire.VerifyFile(EnginePath(d.Layout), int64(len(fakeEngineBytes)), digest(fakeEngineBytes)); err != nil {
		t.Errorf("engine not placed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.Layout.VariantDir("multilingual"), "model.safetensors")); err != nil {
		t.Error("model not placed")
	}
	cfg, _ := config.Load(d.Layout)
	if cfg.Backend != "cpu" || len(cfg.Agents.Registered) != 2 {
		t.Errorf("config = %+v", cfg)
	}
	// claude and pi were detected (directories exist); codex and opencode were not.
	if !contains(rep.Registered, "claude") || !contains(rep.Registered, "pi") || len(rep.Registered) != 2 {
		t.Errorf("registered = %v", rep.Registered)
	}
	a, _ := agents.ByID("claude")
	if v, err := skill.InstalledVersion(a.SkillDir(d.Env)); err != nil || v != "9.9.9" {
		t.Errorf("skill version = %q, %v", v, err)
	}
	if st := a.Status(d.Env, d.BinPath); !st.Registered || st.Stale {
		t.Errorf("claude status = %+v", st)
	}
	if _, err := os.Stat(filepath.Join(d.Layout.State, "sidecar.json")); !errors.Is(err, os.ErrNotExist) {
		t.Error("smoke test must stop the engine afterwards")
	}

	// Second run: nothing downloads, configs unchanged.
	before, _ := os.ReadFile(a.ConfigPath(d.Env))
	d.Out = &bytes.Buffer{}
	rep2, err := Run(ctx, d, Options{Backend: "auto", Agents: []string{"all"}, Yes: true, NoStart: true})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(a.ConfigPath(d.Env))
	if rep2.Changed || len(rep2.Downloaded) != 0 || len(rep2.Skipped) != 3 || string(before) != string(after) {
		t.Errorf("second run = %+v; config changed = %v", rep2, string(before) != string(after))
	}
	if !strings.Contains(d.Out.(*bytes.Buffer).String(), "Already up to date") {
		t.Errorf("out = %s", d.Out.(*bytes.Buffer).String())
	}

	if err := Uninstall(ctx, d, UninstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.Layout.Home); !errors.Is(err, os.ErrNotExist) {
		t.Error("home not removed")
	}
	if st := a.Status(d.Env, d.BinPath); st.Registered {
		t.Error("claude still registered after uninstall")
	}
	if _, err := os.Stat(a.SkillDir(d.Env)); !errors.Is(err, os.ErrNotExist) {
		t.Error("skill still present after uninstall")
	}
	if got, _ := os.ReadFile(a.ConfigPath(d.Env)); !strings.Contains(string(got), "mcpServers") {
		t.Errorf("claude config after uninstall = %s", got)
	}
}

func TestRerunKeepsPriorChoicesAndAgentsNoneKeepsRegistrations(t *testing.T) {
	srv, m := fixture(t)
	d := deps(t, srv, m)
	cfg := config.Default()
	cfg.Backend, cfg.Model = "cpu", "multilingual"
	cfg.Agents.Registered = []string{"claude"}
	config.Save(d.Layout, cfg)
	p, err := Resolve(d, Options{Backend: "auto", Agents: []string{"none"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Backend != "cpu" || p.Model != "multilingual" || len(p.Agents) != 0 {
		t.Errorf("plan = %+v", p)
	}
	if _, err := Run(context.Background(), d, Options{Backend: "auto", Agents: []string{"none"}, Yes: true, NoStart: true}); err != nil {
		t.Fatal(err)
	}
	after, _ := config.Load(d.Layout)
	if len(after.Agents.Registered) != 1 {
		t.Errorf("agents none must not clear the recorded registrations: %+v", after.Agents)
	}
	p, _ = Resolve(d, Options{Agents: []string{"keep"}, Yes: true})
	if len(p.Agents) != 1 || p.Agents[0] != "claude" {
		t.Errorf("keep = %v", p.Agents)
	}
}

func TestInstallRejectsUnavailableBackendAndUnknownAgent(t *testing.T) {
	srv, m := fixture(t)
	d := deps(t, srv, m)
	if _, err := Resolve(d, Options{Backend: "cuda"}); err == nil || !strings.Contains(err.Error(), "cuda") {
		t.Errorf("err = %v", err)
	}
	if _, err := Resolve(d, Options{Agents: []string{"emacs"}}); err == nil || !strings.Contains(err.Error(), "emacs") {
		t.Errorf("err = %v", err)
	}
	if _, err := Resolve(d, Options{Model: "klingon"}); err == nil {
		t.Error("unknown model must fail")
	}
}

func TestInstallSmokeFailureStopsBeforeRegistering(t *testing.T) {
	srv, m := fixture(t)
	d := deps(t, srv, m)
	d.Smoke = func(context.Context, string) error { return errors.New("bad answer") }
	_, err := Run(context.Background(), d, Options{Agents: []string{"claude"}, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "smoke test failed") {
		t.Fatalf("err = %v", err)
	}
	a, _ := agents.ByID("claude")
	if st := a.Status(d.Env, d.BinPath); st.Registered {
		t.Error("agents must not be registered after a failed smoke test")
	}
}

func TestInstallSkipsPiWithoutRegistry(t *testing.T) {
	srv, m := fixture(t)
	d := deps(t, srv, m)
	os.Remove(filepath.Join(d.Env.Home, ".pi", "agent", "mcp.json"))
	rep, err := Run(context.Background(), d, Options{Agents: []string{"pi"}, Yes: true, NoStart: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Registered) != 0 || !strings.Contains(rep.SkippedAgents["pi"], "pi-mcp-adapter") {
		t.Errorf("report = %+v", rep)
	}
	p, _ := agents.ByID("pi")
	if _, err := skill.InstalledVersion(p.SkillDir(d.Env)); err != nil {
		t.Error("skill should still be installed for pi")
	}
}

type fakePrompt struct {
	selects []string
	multi   []string
	confirm bool
	titles  []string
}

func (f *fakePrompt) Select(title string, c []Choice) (string, error) {
	f.titles = append(f.titles, title)
	v := f.selects[0]
	f.selects = f.selects[1:]
	return v, nil
}
func (f *fakePrompt) MultiSelect(title string, c []Choice) ([]string, error) {
	f.titles = append(f.titles, title)
	return f.multi, nil
}
func (f *fakePrompt) Confirm(title string) (bool, error) { return f.confirm, nil }

func TestInteractiveFlowAsksAndCanCancel(t *testing.T) {
	srv, m := fixture(t)
	d := deps(t, srv, m)
	fp := &fakePrompt{selects: []string{"cpu", "multilingual"}, multi: []string{"claude"}, confirm: false}
	d.Prompt = fp
	_, err := Run(context.Background(), d, Options{})
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(fp.titles, ",") != "Engine backend,Model,Register in agents" {
		t.Errorf("titles = %v", fp.titles)
	}
	if _, err := os.Stat(d.Layout.Bin); !errors.Is(err, os.ErrNotExist) {
		t.Error("cancelled install must not create the home")
	}
}
