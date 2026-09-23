// Package doctor diagnoses an installation and tells the user how to fix it.
package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/acquire"
	"github.com/QuBiit0/ohmylaya/internal/agents"
	"github.com/QuBiit0/ohmylaya/internal/config"
	"github.com/QuBiit0/ohmylaya/internal/jev"
	"github.com/QuBiit0/ohmylaya/internal/manifest"
	"github.com/QuBiit0/ohmylaya/internal/platform"
	"github.com/QuBiit0/ohmylaya/internal/sidecar"
	"github.com/QuBiit0/ohmylaya/internal/skill"
)

// Status values.
const (
	Pass = "PASS"
	Warn = "WARN"
	Fail = "FAIL"
	Skip = "SKIP"
)

// Check is one diagnostic line.
type Check struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// Report is the full diagnosis.
type Report struct {
	Version string          `json:"version"`
	Home    string          `json:"home"`
	Checks  []Check         `json:"checks"`
	Summary map[string]int  `json:"summary"`
	Config  *config.Config  `json:"config,omitempty"`
	Agents  []agents.Status `json:"agents"`
}

// Deps are the collaborators.
type Deps struct {
	Layout   config.Layout
	Manifest *manifest.Manifest
	Probe    platform.Probe
	Env      agents.Env
	BinPath  string
	Version  string
	Client   *http.Client
	// EnginePath is where the executable should be.
	EnginePath string
	// Smoke runs one request when true.
	Smoke bool
	// LatestRelease returns the newest release tag; nil skips the check.
	LatestRelease func(ctx context.Context) (string, error)
}

// Run executes every check.
func Run(ctx context.Context, d Deps) Report {
	r := Report{Version: d.Version, Home: d.Layout.Home, Summary: map[string]int{}}
	add := func(c Check) { r.Checks = append(r.Checks, c) }

	det := platform.Detect(d.Probe)
	if det.Unsupported != "" {
		add(Check{ID: "platform", Status: Fail, Detail: det.Unsupported})
	} else {
		add(Check{ID: "platform", Status: Pass, Detail: fmt.Sprintf("%s/%s, backends: %s", det.OS, det.Arch, joinBackends(det))})
	}

	cfg, err := config.Load(d.Layout)
	switch {
	case errors.Is(err, os.ErrNotExist) || (err == nil && !exists(d.Layout.ConfigPath)):
		add(Check{ID: "home", Status: Fail, Detail: d.Layout.Home + " has no config.toml", Fix: "ohmylaya install"})
		cfg = config.Default()
	case err != nil:
		add(Check{ID: "home", Status: Fail, Detail: err.Error(), Fix: "fix or delete " + d.Layout.ConfigPath})
		cfg = config.Default()
	default:
		add(Check{ID: "home", Status: Pass, Detail: fmt.Sprintf("%s, backend %s, model %s, port %d", d.Layout.Home, cfg.Backend, cfg.Model, cfg.Port)})
	}
	r.Config = cfg

	asset, aerr := d.Manifest.EngineAsset(det.OS, det.Arch, cfg.Backend)
	switch {
	case !exists(d.EnginePath):
		add(Check{ID: "engine", Status: Fail, Detail: d.EnginePath + " missing", Fix: "ohmylaya install"})
	case aerr != nil:
		add(Check{ID: "engine", Status: Warn, Detail: aerr.Error()})
	case acquire.VerifyFile(d.EnginePath, asset.Size, asset.SHA256) != nil:
		add(Check{ID: "engine", Status: Warn, Detail: "executable does not match the pinned " + asset.Name, Fix: "ohmylaya install"})
	default:
		if out, err := runHelp(ctx, d.EnginePath); err != nil {
			add(Check{ID: "engine", Status: Fail, Detail: "cannot run: " + err.Error() + " " + out, Fix: "check runtime-deps below"})
		} else {
			add(Check{ID: "engine", Status: Pass, Detail: asset.Name + " verified and runnable"})
		}
	}

	add(runtimeDeps(d, cfg.Backend))
	add(modelCheck(d, cfg.Model))
	add(portCheck(ctx, d, cfg))
	sc, state := sidecarCheck(ctx, d, cfg)
	add(sc)
	if state != nil {
		add(Check{ID: "gpu", Status: Pass, Detail: "engine reports backend " + state.backend})
	} else {
		add(Check{ID: "gpu", Status: Skip, Detail: "engine not running; start it with a tool call or ohmylaya install"})
	}
	if d.Smoke {
		add(smokeCheck(ctx, d, cfg))
	}

	stale := false
	for _, a := range agents.All() {
		st := a.Status(d.Env, d.BinPath)
		r.Agents = append(r.Agents, st)
		if st.Registered && st.Stale {
			stale = true
		}
		if !st.Registered {
			continue
		}
		v, err := skill.InstalledVersion(a.SkillDir(d.Env))
		switch {
		case err != nil:
			add(Check{ID: "skill:" + a.ID(), Status: Warn, Detail: "registered but the skill is missing", Fix: "ohmylaya agents --register " + a.ID()})
		case d.Version != "dev" && v != d.Version:
			add(Check{ID: "skill:" + a.ID(), Status: Warn, Detail: "skill " + v + " is outdated for binary " + d.Version, Fix: "ohmylaya update"})
		default:
			add(Check{ID: "skill:" + a.ID(), Status: Pass, Detail: "skill " + v + " at " + a.SkillDir(d.Env)})
		}
	}
	registered := 0
	for _, st := range r.Agents {
		if st.Registered {
			registered++
		}
	}
	switch {
	case stale:
		add(Check{ID: "agents", Status: Fail, Detail: "a registration points at a missing binary", Fix: "ohmylaya install"})
	case registered == 0:
		add(Check{ID: "agents", Status: Warn, Detail: "no agent is registered", Fix: "ohmylaya install or ohmylaya agents --register <id>"})
	default:
		add(Check{ID: "agents", Status: Pass, Detail: fmt.Sprintf("%d registered", registered)})
	}

	if d.LatestRelease == nil {
		add(Check{ID: "version", Status: Skip, Detail: "release check disabled"})
	} else {
		vctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		latest, err := d.LatestRelease(vctx)
		cancel()
		switch {
		case err != nil:
			add(Check{ID: "version", Status: Skip, Detail: "offline or GitHub unreachable"})
		case d.Version == "dev":
			add(Check{ID: "version", Status: Pass, Detail: "development build; latest release is " + latest})
		case strings.TrimPrefix(latest, "v") != strings.TrimPrefix(d.Version, "v"):
			add(Check{ID: "version", Status: Warn, Detail: d.Version + " installed, " + latest + " available", Fix: "ohmylaya update"})
		default:
			add(Check{ID: "version", Status: Pass, Detail: d.Version + " is the latest release"})
		}
	}

	for _, c := range r.Checks {
		r.Summary[c.Status]++
	}
	return r
}

// ExitCode is 0 unless a check failed, or warned when failOnWarn is set.
func (r Report) ExitCode(failOnWarn bool) int {
	if r.Summary[Fail] > 0 || (failOnWarn && r.Summary[Warn] > 0) {
		return 1
	}
	return 0
}

// Print writes the human report.
func Print(w io.Writer, r Report) {
	fmt.Fprintf(w, "ohmylaya %s doctor  home %s\n", r.Version, r.Home)
	for _, c := range r.Checks {
		fmt.Fprintf(w, "  %-4s %-16s %s\n", c.Status, c.ID, c.Detail)
		if c.Fix != "" && c.Status != Pass {
			fmt.Fprintf(w, "       fix: %s\n", c.Fix)
		}
	}
	fmt.Fprintf(w, "\n%d pass, %d warn, %d fail, %d skip\n", r.Summary[Pass], r.Summary[Warn], r.Summary[Fail], r.Summary[Skip])
}

// PrintJSON writes the machine report.
func PrintJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func runtimeDeps(d Deps, backend string) Check {
	switch {
	case backend == "cpu":
		return Check{ID: "runtime-deps", Status: Pass, Detail: "cpu backend needs no GPU runtime"}
	case backend == "vulkan":
		lib := "libvulkan.so.1"
		if runtime.GOOS == "windows" {
			lib = "vulkan-1.dll"
		}
		if d.Probe.HasLibrary(lib) {
			return Check{ID: "runtime-deps", Status: Pass, Detail: lib + " found"}
		}
		return Check{ID: "runtime-deps", Status: Fail, Detail: lib + " not found", Fix: "install your GPU driver or switch to --backend cpu"}
	case backend == "cuda":
		lib := "libcuda.so.1"
		if runtime.GOOS == "windows" {
			lib = "nvcuda.dll"
		}
		if !d.Probe.HasLibrary(lib) {
			return Check{ID: "runtime-deps", Status: Fail, Detail: lib + " not found", Fix: "install the NVIDIA driver or switch to --backend vulkan"}
		}
		if runtime.GOOS == "windows" {
			var missing []string
			for _, dll := range []string{"cublas64_13.dll", "cublasLt64_13.dll"} {
				if !exists(filepath.Join(d.Layout.Bin, dll)) {
					missing = append(missing, dll)
				}
			}
			if len(missing) > 0 {
				return Check{ID: "runtime-deps", Status: Fail, Detail: "missing " + strings.Join(missing, ", ") + " in " + d.Layout.Bin, Fix: "ohmylaya install --backend cuda"}
			}
		}
		return Check{ID: "runtime-deps", Status: Pass, Detail: lib + " and cuBLAS present"}
	}
	return Check{ID: "runtime-deps", Status: Warn, Detail: "unknown backend " + backend}
}

func modelCheck(d Deps, model string) Check {
	v, err := d.Manifest.Variant(model)
	if err != nil {
		return Check{ID: "model", Status: Fail, Detail: err.Error()}
	}
	dir := d.Layout.VariantDir(model)
	var bad []string
	for _, f := range v.Files {
		if err := acquire.VerifyFile(filepath.Join(dir, filepath.FromSlash(f.Path)), f.Size, f.SHA256); err != nil {
			bad = append(bad, f.Path)
		}
	}
	if len(bad) > 0 {
		return Check{ID: "model", Status: Fail, Detail: model + " missing or corrupt: " + strings.Join(bad, ", "), Fix: "ohmylaya install --model " + model}
	}
	return Check{ID: "model", Status: Pass, Detail: fmt.Sprintf("%s verified (%d files)", model, len(v.Files))}
}

func portCheck(ctx context.Context, d Deps, cfg *config.Config) Check {
	addr := "127.0.0.1:" + strconv.Itoa(cfg.Port)
	l, err := net.Listen("tcp", addr)
	if err == nil {
		l.Close()
		return Check{ID: "port", Status: Pass, Detail: addr + " free"}
	}
	h, herr := jev.NewLocal("http://"+addr, d.Client, cfg.Batch.MaxQuestions).Health(ctx)
	if herr == nil && h.Ready() {
		return Check{ID: "port", Status: Pass, Detail: addr + " owned by a healthy engine"}
	}
	return Check{ID: "port", Status: Warn, Detail: addr + " in use by another service; the sidecar will use the next free port", Fix: "set port in " + d.Layout.ConfigPath + " to avoid the scan"}
}

type sidecarState struct{ backend string }

func sidecarCheck(ctx context.Context, d Deps, cfg *config.Config) (Check, *sidecarState) {
	st, err := sidecar.ReadState(d.Layout)
	if err != nil {
		return Check{ID: "sidecar", Status: Skip, Detail: "not running"}, nil
	}
	h, herr := jev.NewLocal(st.BaseURL(), d.Client, cfg.Batch.MaxQuestions).Health(ctx)
	if herr != nil || !h.Ready() {
		return Check{ID: "sidecar", Status: Warn, Detail: fmt.Sprintf("state records pid %d on port %d but it does not answer", st.PID, st.Port), Fix: "it will be restarted on the next call; see " + sidecar.LogPath(d.Layout)}, nil
	}
	return Check{ID: "sidecar", Status: Pass, Detail: fmt.Sprintf("pid %d on port %d, %s, pending %d queued %d", st.PID, st.Port, h.Backend, h.PendingRequests, h.QueuedRequests)}, &sidecarState{backend: h.Backend}
}

func smokeCheck(ctx context.Context, d Deps, cfg *config.Config) Check {
	sup := sidecar.New(d.Layout, cfg, d.EnginePath)
	h, err := sup.Ensure(ctx)
	if err != nil {
		var se *sidecar.StartError
		if errors.As(err, &se) {
			return Check{ID: "smoke", Status: Fail, Detail: se.Reason + "\n" + se.LogTail, Fix: "ohmylaya install with another --backend"}
		}
		return Check{ID: "smoke", Status: Fail, Detail: err.Error()}
	}
	defer func() {
		h.Close()
		if h.Spawned {
			stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = sup.Stop(stopCtx)
		}
	}()
	started := time.Now()
	out, err := jev.NewLocal(h.BaseURL(), d.Client, cfg.Batch.MaxQuestions).Predict(ctx, []jev.Request{{State: "Please refund the duplicate charge.", Questions: jev.Questions{jev.Q("refund", jev.Question{Type: "noul", Instructions: "Does the customer ask for a refund?"})}}})
	if err != nil || len(out) != 1 {
		return Check{ID: "smoke", Status: Fail, Detail: fmt.Sprintf("request failed: %v", err)}
	}
	return Check{ID: "smoke", Status: Pass, Detail: fmt.Sprintf("refund p=%.3f in %d ms", out[0].Answers["refund"].Noul, time.Since(started).Milliseconds())}
}

func runHelp(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--help").CombinedOutput()
	s := strings.TrimSpace(string(out))
	if len(s) > 200 {
		s = s[:200]
	}
	return s, err
}

func joinBackends(det platform.Detection) string {
	var out []string
	for _, o := range det.Options {
		out = append(out, o.Backend)
	}
	return strings.Join(out, ", ")
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
