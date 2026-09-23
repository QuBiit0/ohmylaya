package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/QuBiit0/ohmylaya/internal/acquire"
	"github.com/QuBiit0/ohmylaya/internal/agents"
	"github.com/QuBiit0/ohmylaya/internal/buildinfo"
	"github.com/QuBiit0/ohmylaya/internal/config"
	"github.com/QuBiit0/ohmylaya/internal/doctor"
	"github.com/QuBiit0/ohmylaya/internal/install"
	"github.com/QuBiit0/ohmylaya/internal/jev"
	"github.com/QuBiit0/ohmylaya/internal/platform"
	"github.com/QuBiit0/ohmylaya/internal/sidecar"
	"github.com/QuBiit0/ohmylaya/internal/skill"
	"github.com/QuBiit0/ohmylaya/internal/tui"
	"github.com/QuBiit0/ohmylaya/internal/update"
)

// runTUI opens the interactive interface, or prints the status when the
// terminal is not interactive.
func runTUI(stdin io.Reader, stdout, stderr io.Writer) int {
	if !isTerminal(stdin) || os.Getenv("TERM") == "dumb" {
		return printStatus(stdout, stderr)
	}
	deps, err := installDeps(io.Discard)
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya:", err)
		return exitFailure
	}
	p := tea.NewProgram(tui.New(hostServices(deps)), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(stderr, "ohmylaya:", err)
		return exitFailure
	}
	return exitOK
}

func printStatus(stdout, stderr io.Writer) int {
	deps, err := installDeps(io.Discard)
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya:", err)
		return exitFailure
	}
	s := hostServices(deps).Status(context.Background())
	fmt.Fprintf(stdout, "ohmylaya %s\nhome %s\nbackend %s  model %s  port %d  provider %s\nengine %v  weights %v  sidecar %s\n", s.Version, s.Home, s.Backend, s.Model, s.Port, s.Provider, s.EngineOK, s.ModelOK, s.SidecarInfo)
	for _, a := range s.Agents {
		fmt.Fprintf(stdout, "%-12s detected=%v registered=%v stale=%v\n", a.Name, a.Detected, a.Registered, a.Stale)
	}
	return exitOK
}

// hostServices binds the TUI to the real subcommand code paths.
func hostServices(deps install.Deps) tui.Services {
	return tui.Services{
		Status: func(ctx context.Context) tui.Status {
			cfg, err := config.Load(deps.Layout)
			if err != nil {
				cfg = config.Default()
			}
			s := tui.Status{Version: buildinfo.Version, Home: deps.Layout.Home, Backend: cfg.Backend, Model: cfg.Model, Port: cfg.Port, Provider: cfg.Provider, SidecarInfo: "not running"}
			det := platform.Detect(deps.Probe)
			if a, err := deps.Manifest.EngineAsset(det.OS, det.Arch, cfg.Backend); err == nil {
				s.EngineOK = acquire.VerifyFile(install.EnginePath(deps.Layout), a.Size, a.SHA256) == nil
			}
			if v, err := deps.Manifest.Variant(cfg.Model); err == nil {
				s.ModelOK = true
				for _, f := range v.Files {
					if acquire.VerifyFile(deps.Layout.VariantDir(cfg.Model)+"/"+f.Path, f.Size, f.SHA256) != nil {
						s.ModelOK = false
					}
				}
			}
			if st, err := sidecar.ReadState(deps.Layout); err == nil {
				hctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				if h, err := jev.NewLocal(st.BaseURL(), nil, cfg.Batch.MaxQuestions).Health(hctx); err == nil && h.Ready() {
					s.SidecarInfo = fmt.Sprintf("running on %d (%s)", st.Port, h.Backend)
				}
				cancel()
			}
			for _, a := range agents.All() {
				s.Agents = append(s.Agents, a.Status(deps.Env, deps.BinPath))
			}
			return s
		},
		Doctor: func(ctx context.Context, smoke bool) doctor.Report {
			return doctor.Run(ctx, doctor.Deps{Layout: deps.Layout, Manifest: deps.Manifest, Probe: deps.Probe, Env: deps.Env, BinPath: deps.BinPath, Version: buildinfo.Version, Client: &http.Client{Timeout: 5 * time.Second}, EnginePath: install.EnginePath(deps.Layout), Smoke: smoke, LatestRelease: update.LatestRelease})
		},
		Logs: func() string { return sidecar.LogTail(deps.Layout) },
		Check: func(ctx context.Context) tui.UpdateInfo {
			p, err := update.Check(ctx, &http.Client{Timeout: 10 * time.Second}, buildinfo.Version)
			info := tui.UpdateInfo{Current: buildinfo.Version}
			switch {
			case errors.Is(err, update.ErrUpToDate):
				info.Latest = p.Latest
			case err != nil:
				info.Err = err
			default:
				info.Latest, info.Available = p.Latest, true
			}
			return info
		},
		Update: func(ctx context.Context, progress func(string)) error {
			var out bytes.Buffer
			if code := runUpdate(nil, &out, &out); code != 0 {
				return errors.New(out.String())
			}
			return nil
		},
		Setup: func(ctx context.Context, backend, model string, progress func(string)) error {
			d := deps
			d.Out = io.Discard
			_, err := install.Run(ctx, d, install.Options{Backend: backend, Model: model, Agents: []string{"keep"}, Yes: true})
			return err
		},
		Toggle: func(ctx context.Context, id string, register bool) error {
			a, ok := agents.ByID(id)
			if !ok {
				return fmt.Errorf("unknown agent %s", id)
			}
			if register {
				if err := skill.Install(a.SkillDir(deps.Env), buildinfo.Version); err != nil {
					return err
				}
				return a.Register(deps.Env, deps.BinPath)
			}
			if _, err := a.Unregister(deps.Env); err != nil {
				return err
			}
			return skill.Remove(a.SkillDir(deps.Env))
		},
		Options: func() ([]string, []string) {
			det := platform.Detect(deps.Probe)
			var backends []string
			for _, o := range det.Options {
				backends = append(backends, o.Backend)
			}
			return backends, []string{"multilingual", "english", "typed-decisions"}
		},
	}
}
