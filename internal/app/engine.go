// Package app composes configuration, manifest, sidecar and providers into
// the runtime the subcommands share.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/config"
	"github.com/QuBiit0/ohmylaya/internal/jev"
	"github.com/QuBiit0/ohmylaya/internal/manifest"
	"github.com/QuBiit0/ohmylaya/internal/sidecar"
	"github.com/QuBiit0/ohmylaya/internal/tools"
)

// Runtime holds the loaded configuration and paths.
type Runtime struct {
	Layout   config.Layout
	Config   *config.Config
	Manifest *manifest.Manifest
}

// LoadRuntime reads the home layout, config.toml and the embedded manifest.
func LoadRuntime() (*Runtime, error) {
	l := config.NewLayout(config.Home())
	cfg, err := config.Load(l)
	if err != nil {
		return nil, err
	}
	if err := config.ApplyEnv(cfg, os.Getenv); err != nil {
		return nil, err
	}
	m, err := manifest.Load()
	if err != nil {
		return nil, err
	}
	return &Runtime{Layout: l, Config: cfg, Manifest: m}, nil
}

// EnginePath is where the installer places the executable for the platform.
func (r *Runtime) EnginePath() string {
	name := "laya-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(r.Layout.Bin, name)
}

// Engine lazily provides a tool service backed by the configured provider.
type Engine struct {
	rt      *Runtime
	mu      sync.Mutex
	svc     *tools.Service
	handle  *sidecar.Handle
	sup     *sidecar.Supervisor
	retried bool
}

// NewEngine creates the lazy engine.
func NewEngine(rt *Runtime) *Engine {
	return &Engine{rt: rt}
}

// Service returns a ready tool service. For the hosted provider it needs no
// sidecar. For the local provider it ensures the engine is running; if the
// engine died since the last call it is restarted once.
func (e *Engine) Service(ctx context.Context) (*tools.Service, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := e.rt.Config
	if cfg.Provider == "typesafe" {
		if e.svc == nil {
			key := os.Getenv("TYPESAFE_API_KEY")
			if key == "" {
				return nil, errors.New("provider is typesafe but TYPESAFE_API_KEY is not set")
			}
			p := jev.NewTypeSafe("", key, &http.Client{Timeout: 30 * time.Second})
			e.svc = tools.NewService(p, "jev", cfg.Tools.AutoAccept)
		}
		return e.svc, nil
	}
	if e.sup == nil {
		if _, err := os.Stat(e.rt.EnginePath()); err != nil {
			return nil, fmt.Errorf("engine not installed at %s", e.rt.EnginePath())
		}
		e.sup = sidecar.New(e.rt.Layout, cfg, e.rt.EnginePath())
		e.sup.SetReaper(ReaperCommand())
	}
	if e.handle != nil && e.svc != nil {
		if st, err := sidecar.ReadState(e.rt.Layout); err == nil && st.PID == e.handle.PID {
			return e.svc, nil
		}
		// The engine went away; rebuild once.
		e.handle.Close()
		e.handle, e.svc = nil, nil
		if e.retried {
			return nil, errors.New("engine stopped twice; not restarting again in this session")
		}
		e.retried = true
	}
	h, err := e.sup.Ensure(ctx)
	if err != nil {
		var se *sidecar.StartError
		if errors.As(err, &se) && se.LogTail != "" {
			return nil, fmt.Errorf("%w\n--- engine log ---\n%s", err, se.LogTail)
		}
		return nil, err
	}
	e.handle = h
	p := jev.NewLocal(h.BaseURL(), &http.Client{Timeout: 120 * time.Second}, cfg.Batch.MaxQuestions)
	e.svc = tools.NewService(p, cfg.Model, cfg.Tools.AutoAccept)
	return e.svc, nil
}

// Touch records activity for the idle reaper.
func (e *Engine) Touch() { sidecar.Touch(e.rt.Layout) }

// Close unregisters this process as a client.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle != nil {
		e.handle.Close()
		e.handle = nil
	}
}

// RunReaper runs the in-process idle reaper for the local provider until
// ctx ends. Long-lived sessions keep it as a fallback for engines whose
// detached reaper never started or was killed.
func (e *Engine) RunReaper(ctx context.Context) {
	if e.rt.Config.Provider != "local" {
		return
	}
	sidecar.New(e.rt.Layout, e.rt.Config, e.rt.EnginePath()).RunReaper(ctx, time.Minute)
}

// ReaperCommand is the detached command that stops an idle engine. It is
// this binary running the hidden reap subcommand. It returns nil when the
// executable cannot be resolved; engines then rely on RunReaper.
func ReaperCommand() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return []string{exe, "reap"}
}

// reapPollInterval is short so the reaper notices a stopped engine and
// exits well within uninstall's retry window, releasing its executable.
const reapPollInterval = 2 * time.Second

// Reap runs the idle reaper until engine enginePID is gone or replaced.
func (r *Runtime) Reap(ctx context.Context, enginePID int) {
	sidecar.New(r.Layout, r.Config, r.EnginePath()).RunReaperUntilGone(ctx, reapPollInterval, enginePID)
}
