// Package install turns a bare machine into a working ohmylaya setup: engine,
// checkpoint, configuration, smoke test, skill and agent registration.
package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

// Options are the user's choices. Empty strings mean "ask or default".
type Options struct {
	Backend string   // auto, vulkan, cuda, cpu
	Model   string   // multilingual, english, typed-decisions
	Agents  []string // agent ids, or "all", or "none"
	Yes     bool
	NoSkill bool
	NoStart bool
}

// Choice is one selectable option shown to the user.
type Choice struct {
	Value       string
	Label       string
	Description string
	Selected    bool
}

// Prompter asks the user. A nil Prompter means non-interactive.
type Prompter interface {
	Select(title string, choices []Choice) (string, error)
	MultiSelect(title string, choices []Choice) ([]string, error)
	Confirm(title string) (bool, error)
}

// Deps are the collaborators, injectable for tests.
type Deps struct {
	Layout   config.Layout
	Manifest *manifest.Manifest
	Probe    platform.Probe
	Client   *http.Client
	Env      agents.Env
	BinPath  string
	Version  string
	Out      io.Writer
	Prompt   Prompter
	Progress acquire.Progress
	// Smoke runs one request against a started engine. Nil uses the default.
	Smoke func(ctx context.Context, baseURL string) error
}

// Report summarises what happened.
type Report struct {
	Backend        string
	Model          string
	Downloaded     []string
	Skipped        []string
	Registered     []string
	SkippedAgents  map[string]string
	SkillDirs      []string
	SmokeRan       bool
	SmokeLatencyMS int64
	Changed        bool
}

// Plan is the resolved set of choices and downloads before any change.
type Plan struct {
	Backend   string
	Model     string
	Agents    []string
	Engine    *manifest.Asset
	Variant   *manifest.Variant
	Downloads []acquire.Spec
	Bytes     int64
}

// EnginePath is the executable's destination for the platform.
func EnginePath(l config.Layout) string {
	name := "laya-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(l.Bin, name)
}

// Resolve turns options into a plan, asking through the prompter when a
// choice is open and one is available.
func Resolve(deps Deps, opts Options) (*Plan, error) {
	det := platform.Detect(deps.Probe)
	if det.Unsupported != "" {
		return nil, fmt.Errorf("install: %s", det.Unsupported)
	}
	p := &Plan{}

	backend := opts.Backend
	if backend == "" || backend == "auto" {
		backend = det.Default
		if opts.Backend == "" && deps.Prompt != nil && !opts.Yes {
			var choices []Choice
			for _, o := range det.Options {
				a, _ := deps.Manifest.EngineAsset(det.OS, det.Arch, o.Backend)
				size := ""
				if a != nil {
					size = fmt.Sprintf(" (%s)", humanBytes(a.Size))
				}
				choices = append(choices, Choice{Value: o.Backend, Label: o.Backend + size, Description: o.Reason, Selected: o.Backend == det.Default})
			}
			v, err := deps.Prompt.Select("Engine backend", choices)
			if err != nil {
				return nil, err
			}
			backend = v
		}
	}
	if !containsBackend(det, backend) {
		return nil, fmt.Errorf("install: backend %q is not available on this machine (options: %s)", backend, strings.Join(backendNames(det), ", "))
	}
	p.Backend = backend

	model := opts.Model
	if model == "" {
		model = "multilingual"
		if deps.Prompt != nil && !opts.Yes {
			var choices []Choice
			for _, name := range []string{"multilingual", "english", "typed-decisions"} {
				v, err := deps.Manifest.Variant(name)
				if err != nil {
					continue
				}
				choices = append(choices, Choice{Value: name, Label: fmt.Sprintf("%s (%s)", name, humanBytes(v.TotalSize())), Description: modelBlurb(name), Selected: name == "multilingual"})
			}
			v, err := deps.Prompt.Select("Model", choices)
			if err != nil {
				return nil, err
			}
			model = v
		}
	}
	variant, err := deps.Manifest.Variant(model)
	if err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	p.Model, p.Variant = model, variant

	agentIDs, err := resolveAgents(deps, opts)
	if err != nil {
		return nil, err
	}
	p.Agents = agentIDs

	asset, err := deps.Manifest.EngineAsset(det.OS, det.Arch, backend)
	if err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	p.Engine = asset
	p.Downloads = append(p.Downloads, acquire.Spec{URL: asset.URL, Size: asset.Size, SHA256: asset.SHA256, Dest: EnginePath(deps.Layout)})
	if asset.Extra != nil {
		p.Downloads = append(p.Downloads, acquire.Spec{URL: asset.Extra.URL, SHA256: asset.Extra.SHA256, Dest: filepath.Join(deps.Layout.Bin, asset.Extra.Name)})
	}
	dir := deps.Layout.VariantDir(model)
	for _, f := range variant.Files {
		p.Downloads = append(p.Downloads, acquire.Spec{URL: deps.Manifest.FileURL(variant, f), Size: f.Size, SHA256: f.SHA256, Dest: filepath.Join(dir, filepath.FromSlash(f.Path))})
	}
	for _, d := range p.Downloads {
		if acquire.VerifyFile(d.Dest, d.Size, d.SHA256) != nil {
			p.Bytes += d.Size
		}
	}
	return p, nil
}

func resolveAgents(deps Deps, opts Options) ([]string, error) {
	var detected []string
	for _, a := range agents.All() {
		if ok, _ := a.Detect(deps.Env); ok {
			detected = append(detected, a.ID())
		}
	}
	if len(opts.Agents) == 1 && opts.Agents[0] == "none" {
		return nil, nil
	}
	if len(opts.Agents) == 1 && opts.Agents[0] == "all" {
		return detected, nil
	}
	if len(opts.Agents) > 0 {
		for _, id := range opts.Agents {
			if _, ok := agents.ByID(id); !ok {
				return nil, fmt.Errorf("install: unknown agent %q (claude, codex, opencode, pi)", id)
			}
		}
		return opts.Agents, nil
	}
	if deps.Prompt == nil || opts.Yes {
		return detected, nil
	}
	var choices []Choice
	for _, a := range agents.All() {
		ok, detail := a.Detect(deps.Env)
		choices = append(choices, Choice{Value: a.ID(), Label: a.Name(), Description: detail, Selected: ok})
	}
	return deps.Prompt.MultiSelect("Register in agents", choices)
}

// Run executes the plan.
func Run(ctx context.Context, deps Deps, opts Options) (*Report, error) {
	if deps.Out == nil {
		deps.Out = io.Discard
	}
	plan, err := Resolve(deps, opts)
	if err != nil {
		return nil, err
	}
	rep := &Report{Backend: plan.Backend, Model: plan.Model, SkippedAgents: map[string]string{}}
	fmt.Fprintf(deps.Out, "Backend: %s\nModel: %s\nAgents: %s\nDownload: %s\n", plan.Backend, plan.Model, orNone(plan.Agents), humanBytes(plan.Bytes))
	if deps.Prompt != nil && !opts.Yes {
		ok, err := deps.Prompt.Confirm("Proceed?")
		if err != nil || !ok {
			return nil, errors.New("install: cancelled")
		}
	}

	for _, dir := range []string{deps.Layout.Bin, deps.Layout.Models, deps.Layout.State, deps.Layout.Backups, deps.Layout.Skills} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	for _, d := range plan.Downloads {
		if acquire.VerifyFile(d.Dest, d.Size, d.SHA256) == nil {
			rep.Skipped = append(rep.Skipped, filepath.Base(d.Dest))
			continue
		}
		if d.Size == 0 {
			// The extra archive has no pinned size; digest only.
			if err := downloadUnsized(ctx, deps, d); err != nil {
				return nil, err
			}
		} else {
			fmt.Fprintf(deps.Out, "Downloading %s (%s)\n", filepath.Base(d.Dest), humanBytes(d.Size))
			if err := acquire.Download(ctx, deps.Client, d, deps.Progress); err != nil {
				return nil, err
			}
		}
		rep.Downloaded = append(rep.Downloaded, filepath.Base(d.Dest))
		rep.Changed = true
	}
	if err := os.Chmod(EnginePath(deps.Layout), 0o755); err != nil && runtime.GOOS != "windows" {
		return nil, err
	}
	if plan.Engine.Extra != nil {
		zipPath := filepath.Join(deps.Layout.Bin, plan.Engine.Extra.Name)
		if err := acquire.ExtractMembers(zipPath, plan.Engine.Extra.Members, deps.Layout.Bin); err != nil {
			return nil, err
		}
		_ = os.Remove(zipPath)
	}

	cfg, err := config.Load(deps.Layout)
	if err != nil {
		return nil, err
	}
	if cfg.Backend != plan.Backend || cfg.Model != plan.Model {
		rep.Changed = true
	}
	cfg.Backend, cfg.Model = plan.Backend, plan.Model
	cfg.Agents.Registered = plan.Agents
	if cfg.Agents.Registered == nil {
		cfg.Agents.Registered = []string{}
	}
	if err := config.Save(deps.Layout, cfg); err != nil {
		return nil, err
	}

	if !opts.NoStart {
		fmt.Fprintln(deps.Out, "Starting the engine for a smoke test (the first start compiles shaders and may take a while)")
		lat, err := smoke(ctx, deps, cfg)
		if err != nil {
			return nil, fmt.Errorf("install: smoke test failed: %w", err)
		}
		rep.SmokeRan, rep.SmokeLatencyMS = true, lat
		fmt.Fprintf(deps.Out, "Smoke test passed in %d ms\n", lat)
	}

	for _, id := range plan.Agents {
		a, _ := agents.ByID(id)
		if !opts.NoSkill {
			dir := a.SkillDir(deps.Env)
			if err := skill.Install(dir, deps.Version); err != nil {
				return nil, fmt.Errorf("install: skill for %s: %w", id, err)
			}
			if !contains(rep.SkillDirs, dir) {
				rep.SkillDirs = append(rep.SkillDirs, dir)
			}
		}
		before, _ := os.ReadFile(a.ConfigPath(deps.Env))
		if err := a.Register(deps.Env, deps.BinPath); err != nil {
			if errors.Is(err, agents.ErrNoRegistry) {
				rep.SkippedAgents[id] = err.Error()
				fmt.Fprintf(deps.Out, "Skipped %s: %v\n", a.Name(), err)
				continue
			}
			return nil, fmt.Errorf("install: register %s: %w", id, err)
		}
		after, _ := os.ReadFile(a.ConfigPath(deps.Env))
		if string(before) != string(after) {
			rep.Changed = true
		}
		rep.Registered = append(rep.Registered, id)
	}
	if err := skill.Install(filepath.Join(deps.Layout.Skills, skill.Name), deps.Version); err != nil {
		return nil, err
	}
	if rep.Changed {
		fmt.Fprintln(deps.Out, "Done. Restart your agent to pick up the new tools.")
	} else {
		fmt.Fprintln(deps.Out, "Already up to date.")
	}
	return rep, nil
}

func downloadUnsized(ctx context.Context, deps Deps, d acquire.Spec) error {
	fmt.Fprintf(deps.Out, "Downloading %s\n", filepath.Base(d.Dest))
	resp, err := deps.Client.Head(d.URL)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.ContentLength <= 0 {
		return fmt.Errorf("install: cannot determine size of %s", d.URL)
	}
	d.Size = resp.ContentLength
	return acquire.Download(ctx, deps.Client, d, deps.Progress)
}

// smoke starts the engine, sends one fixed request, verifies the answer
// shape and stops the engine again.
func smoke(ctx context.Context, deps Deps, cfg *config.Config) (int64, error) {
	sup := sidecar.New(deps.Layout, cfg, EnginePath(deps.Layout))
	h, err := sup.Ensure(ctx)
	if err != nil {
		var se *sidecar.StartError
		if errors.As(err, &se) {
			return 0, fmt.Errorf("%s\n--- engine log ---\n%s", se.Reason, se.LogTail)
		}
		return 0, err
	}
	defer func() {
		h.Close()
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = sup.Stop(stopCtx)
	}()
	run := deps.Smoke
	if run == nil {
		run = defaultSmoke
	}
	started := time.Now()
	if err := run(ctx, h.BaseURL()); err != nil {
		return 0, err
	}
	return time.Since(started).Milliseconds(), nil
}

func defaultSmoke(ctx context.Context, baseURL string) error {
	p := jev.NewLocal(baseURL, &http.Client{Timeout: 120 * time.Second}, 8)
	out, err := p.Predict(ctx, []jev.Request{{
		State: "I was charged twice. Please refund the duplicate charge today.",
		Questions: jev.Questions{
			jev.Q("refund", jev.Question{Type: "noul", Instructions: "Does the customer ask for a refund?"}),
			jev.Q("dept", jev.Question{Type: "choice", Instructions: "Which department?", Criteria: jev.Options{jev.Opt("billing", "payments and refunds"), jev.Opt("technical", "bugs")}}),
		},
	}})
	if err != nil {
		return err
	}
	if len(out) != 1 || len(out[0].Answers) != 2 {
		return fmt.Errorf("unexpected answer shape: %+v", out)
	}
	a := out[0].Answers["refund"]
	if a.Noul < 0 || a.Noul > 1 {
		return fmt.Errorf("refund probability out of range: %v", a.Noul)
	}
	return nil
}

func containsBackend(det platform.Detection, b string) bool {
	for _, o := range det.Options {
		if o.Backend == b {
			return true
		}
	}
	return false
}

func backendNames(det platform.Detection) []string {
	var out []string
	for _, o := range det.Options {
		out = append(out, o.Backend)
	}
	return out
}

func modelBlurb(name string) string {
	switch name {
	case "english":
		return "English text, 512-token context, larger model"
	case "typed-decisions":
		return "fine-tuned for agent-trace, support, invoice and security workflows"
	}
	return "100+ languages, 1024-token context, fastest; recommended"
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, ", ")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
