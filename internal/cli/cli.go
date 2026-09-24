// Package cli routes command-line arguments to subcommands.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	"github.com/QuBiit0/ohmylaya/internal/agents"
	"github.com/QuBiit0/ohmylaya/internal/app"
	"github.com/QuBiit0/ohmylaya/internal/buildinfo"
	"github.com/QuBiit0/ohmylaya/internal/config"
	"github.com/QuBiit0/ohmylaya/internal/doctor"
	"github.com/QuBiit0/ohmylaya/internal/install"
	"github.com/QuBiit0/ohmylaya/internal/manifest"
	"github.com/QuBiit0/ohmylaya/internal/mcpserver"
	"github.com/QuBiit0/ohmylaya/internal/platform"
	"github.com/QuBiit0/ohmylaya/internal/sidecar"
	"github.com/QuBiit0/ohmylaya/internal/skill"
	"github.com/QuBiit0/ohmylaya/internal/tools"
	"github.com/QuBiit0/ohmylaya/internal/update"
)

const usage = `Usage: ohmylaya <command> [flags]

With no command on an interactive terminal, ohmylaya opens its TUI.

Commands:
  install     Download the engine and model, verify, register agents
              [--backend auto|vulkan|cuda|cpu] [--model multilingual|english|typed-decisions]
              [--agents claude,codex,opencode,pi|all|none] [--yes] [--no-skill] [--no-start]
  uninstall   Remove the engine, model, skills and agent entries [--keep-models]
  mcp         Serve the tools over MCP on stdin/stdout (used by agents)
  ask <tool>  Call one tool with JSON input on stdin: decide, classify, check, screen, rerank
  doctor      Diagnose the installation [--json] [--smoke] [--fail-on warn]
  update      Update the binary, then the engine, model and skill [--check]
  agents      Show detection and registration status per agent
              [--json] [--register id,...] [--unregister id,...]
  version     Print the version and exit
  help        Show this help

Run 'ohmylaya help <command>' for details on a command.
`

// Exit codes follow the convention used by most Go CLIs.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

// Run executes the command line and returns the process exit code.
// It never calls os.Exit so it stays testable.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithStdin(args, os.Stdin, stdout, stderr)
}

// RunWithStdin is Run with an explicit stdin for commands that read it.
func RunWithStdin(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		if isTerminal(stdin) && os.Getenv("TERM") != "dumb" {
			return runTUI(stdin, stdout, stderr)
		}
		code := printStatus(stdout, stderr)
		fmt.Fprint(stdout, "\n", usage)
		return code
	}

	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "ohmylaya %s\n", buildinfo.Version)
		return exitOK
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return exitOK
	case "mcp":
		return runMCP(stderr)
	case "reap":
		// Hidden: spawned detached after every engine start.
		rt, err := app.LoadRuntime()
		if err != nil {
			return exitFailure
		}
		pid, _ := strconv.Atoi(os.Getenv(sidecar.ReapPIDEnv))
		rt.Reap(context.Background(), pid)
		return exitOK
	case "ask":
		return runAsk(args[1:], stdin, stdout, stderr)
	case "agents":
		return runAgents(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "update":
		return runUpdate(args[1:], stdout, stderr)
	case "install":
		return runInstall(args[1:], stdin, stdout, stderr)
	case "uninstall":
		return runUninstall(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ohmylaya: unknown command %q\n\n", args[0])
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
}

func runMCP(stderr io.Writer) int {
	rt, err := app.LoadRuntime()
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya:", err)
		return exitFailure
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	engine := app.NewEngine(rt)
	defer engine.Close()
	go engine.RunReaper(ctx)
	if err := mcpserver.Run(ctx, engine, buildinfo.Version); err != nil && ctx.Err() == nil {
		fmt.Fprintln(stderr, "ohmylaya mcp:", err)
		return exitFailure
	}
	return exitOK
}

func runAsk(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: ohmylaya ask <decide|classify|check|screen|rerank> < input.json")
		return exitUsage
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya ask: read stdin:", err)
		return exitFailure
	}
	rt, err := app.LoadRuntime()
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya:", err)
		return exitFailure
	}
	engine := app.NewEngine(rt)
	defer engine.Close()
	ctx := context.Background()
	svc, err := engine.Service(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "ohmylaya ask: engine unavailable: %v\n%s\n", err, mcpserver.DoctorHint)
		return exitFailure
	}
	engine.Touch()
	out, err := callTool(ctx, svc, args[0], raw)
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya ask:", err)
		return exitFailure
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return exitFailure
	}
	return exitOK
}

func runUpdate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	check := fs.Bool("check", false, "only report whether an update exists")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	deps, err := installDeps(stdout)
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya update:", err)
		return exitFailure
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	client := &http.Client{Timeout: 10 * time.Minute}
	plan, err := update.Check(ctx, client, buildinfo.Version)
	switch {
	case errors.Is(err, update.ErrUpToDate):
		fmt.Fprintf(stdout, "ohmylaya %s is the latest release.\n", buildinfo.Version)
	case err != nil:
		fmt.Fprintln(stderr, "ohmylaya update:", err)
		return exitFailure
	case *check:
		fmt.Fprintf(stdout, "ohmylaya %s installed, %s available.\n", buildinfo.Version, plan.Latest)
		return exitOK
	default:
		fmt.Fprintf(stdout, "Updating ohmylaya %s to %s\n", buildinfo.Version, plan.Latest)
		if err := update.Apply(ctx, client, plan, deps.BinPath); err != nil {
			fmt.Fprintln(stderr, "ohmylaya update:", err)
			return exitFailure
		}
		// The new binary carries the new manifest; let it reconcile.
		cmd := exec.CommandContext(ctx, deps.BinPath, "install", "--yes", "--agents", "none")
		cmd.Stdout, cmd.Stderr = stdout, stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(stderr, "ohmylaya update: reconcile with the new binary failed:", err)
			return exitFailure
		}
		return reregister(stdout, stderr)
	}
	if *check {
		return exitOK
	}
	// Same binary: reconcile engine, model and skill against the manifest.
	if _, err := install.Run(ctx, deps, install.Options{Backend: "auto", Agents: []string{"none"}, Yes: true, NoStart: true}); err != nil {
		fmt.Fprintln(stderr, "ohmylaya update:", err)
		return exitFailure
	}
	return reregister(stdout, stderr)
}

// reregister refreshes the skill and absolute path for every registered agent.
func reregister(stdout, stderr io.Writer) int {
	rt, err := app.LoadRuntime()
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya update:", err)
		return exitFailure
	}
	if len(rt.Config.Agents.Registered) == 0 {
		return exitOK
	}
	return runAgents([]string{"--register", strings.Join(rt.Config.Agents.Registered, ",")}, stdout, stderr)
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "machine-readable output")
	smoke := fs.Bool("smoke", false, "start the engine and run one request")
	failOn := fs.String("fail-on", "fail", "exit non-zero on: fail, warn")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	ideps, err := installDeps(io.Discard)
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya doctor:", err)
		return exitFailure
	}
	d := doctor.Deps{
		Layout: ideps.Layout, Manifest: ideps.Manifest, Probe: ideps.Probe, Env: ideps.Env,
		BinPath: ideps.BinPath, Version: buildinfo.Version, Client: &http.Client{Timeout: 5 * time.Second},
		EnginePath: install.EnginePath(ideps.Layout), Smoke: *smoke, LatestRelease: update.LatestRelease,
	}
	r := doctor.Run(context.Background(), d)
	if *asJSON {
		_ = doctor.PrintJSON(stdout, r)
	} else {
		doctor.Print(stdout, r)
	}
	if r.ExitCode(*failOn == "warn") != 0 {
		return exitFailure
	}
	return exitOK
}

func installDeps(stdout io.Writer) (install.Deps, error) {
	l := config.NewLayout(config.Home())
	m, err := manifest.Load()
	if err != nil {
		return install.Deps{}, err
	}
	bin, err := os.Executable()
	if err != nil {
		return install.Deps{}, err
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		bin = resolved
	}
	return install.Deps{
		Layout:   l,
		Manifest: m,
		Probe:    platform.HostProbe{},
		Client:   &http.Client{},
		Env:      agents.HostEnv(l.Backups, exec.LookPath),
		BinPath:  bin,
		Version:  buildinfo.Version,
		Out:      stdout,
		Progress: progressPrinter(stdout),
	}, nil
}

func runInstall(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opts install.Options
	var agentList string
	fs.StringVar(&opts.Backend, "backend", "", "auto, vulkan, cuda or cpu")
	fs.StringVar(&opts.Model, "model", "", "multilingual, english or typed-decisions")
	fs.StringVar(&agentList, "agents", "", "comma-separated agent ids, all, or none")
	fs.BoolVar(&opts.Yes, "yes", false, "do not prompt")
	fs.BoolVar(&opts.NoSkill, "no-skill", false, "do not install the skill")
	fs.BoolVar(&opts.NoStart, "no-start", false, "skip the smoke test")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if agentList != "" {
		opts.Agents = strings.Split(agentList, ",")
	}
	deps, err := installDeps(stdout)
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya install:", err)
		return exitFailure
	}
	if !opts.Yes && isTerminal(stdin) {
		deps.Prompt = &linePrompter{in: bufio.NewReader(stdin), out: stdout}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if _, err := install.Run(ctx, deps, opts); err != nil {
		fmt.Fprintln(stderr, "ohmylaya install:", err)
		return exitFailure
	}
	return exitOK
}

func runUninstall(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opts install.UninstallOptions
	yes := fs.Bool("yes", false, "do not prompt")
	fs.BoolVar(&opts.KeepModels, "keep-models", false, "keep the downloaded checkpoints")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	deps, err := installDeps(stdout)
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya uninstall:", err)
		return exitFailure
	}
	if !*yes && isTerminal(stdin) {
		p := &linePrompter{in: bufio.NewReader(stdin), out: stdout}
		ok, _ := p.Confirm("Remove " + deps.Layout.Home + " and the ohmylaya entries from your agents?")
		if !ok {
			fmt.Fprintln(stdout, "Cancelled.")
			return exitOK
		}
	}
	if err := install.Uninstall(context.Background(), deps, opts); err != nil {
		fmt.Fprintln(stderr, "ohmylaya uninstall:", err)
		return exitFailure
	}
	return exitOK
}

// progressPrinter renders a single-line progress indicator per file.
func progressPrinter(w io.Writer) func(done, total int64) {
	var lastPct int64 = -1
	return func(done, total int64) {
		if total <= 0 {
			return
		}
		pct := done * 100 / total
		if pct/5 == lastPct/5 && pct != 100 {
			return
		}
		lastPct = pct
		fmt.Fprintf(w, "\r  %3d%%", pct)
		if pct == 100 {
			fmt.Fprintln(w)
		}
	}
}

func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func runAgents(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agents", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "machine-readable output")
	register := fs.String("register", "", "comma-separated agent ids to register")
	unregister := fs.String("unregister", "", "comma-separated agent ids to unregister")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	rt, err := app.LoadRuntime()
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya:", err)
		return exitFailure
	}
	bin, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		bin = resolved
	}
	env := agents.HostEnv(rt.Layout.Backups, exec.LookPath)
	for _, id := range splitIDs(*register) {
		a, ok := agents.ByID(id)
		if !ok {
			fmt.Fprintf(stderr, "ohmylaya agents: unknown agent %q\n", id)
			return exitUsage
		}
		if err := skill.Install(a.SkillDir(env), buildinfo.Version); err != nil {
			fmt.Fprintf(stderr, "ohmylaya agents: skill for %s: %v\n", id, err)
			return exitFailure
		}
		if err := a.Register(env, bin); err != nil {
			fmt.Fprintf(stderr, "ohmylaya agents: register %s: %v\n", id, err)
			return exitFailure
		}
		fmt.Fprintf(stdout, "Registered %s\n", a.Name())
	}
	for _, id := range splitIDs(*unregister) {
		a, ok := agents.ByID(id)
		if !ok {
			fmt.Fprintf(stderr, "ohmylaya agents: unknown agent %q\n", id)
			return exitUsage
		}
		if _, err := a.Unregister(env); err != nil {
			fmt.Fprintf(stderr, "ohmylaya agents: unregister %s: %v\n", id, err)
			return exitFailure
		}
		_ = skill.Remove(a.SkillDir(env))
		fmt.Fprintf(stdout, "Unregistered %s\n", a.Name())
	}
	var rows []agents.Status
	staleFound := false
	for _, a := range agents.All() {
		st := a.Status(env, bin)
		staleFound = staleFound || st.Stale
		rows = append(rows, st)
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rows)
	} else {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "AGENT\tDETECTED\tREGISTERED\tCONFIG")
		for _, st := range rows {
			reg := "no"
			switch {
			case st.RegistryErr != "":
				reg = "error: " + st.RegistryErr
			case st.Registered && st.Stale:
				reg = "stale (" + st.Command + ")"
			case st.Registered:
				reg = "yes"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", st.Name, yesNo(st.Detected), reg, st.ConfigPath)
		}
		tw.Flush()
		if staleFound {
			fmt.Fprintln(stdout, "\nStale entries point at a missing binary. Run 'ohmylaya install' to repair.")
		}
	}
	if staleFound {
		return exitFailure
	}
	return exitOK
}

func splitIDs(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func callTool(ctx context.Context, svc *tools.Service, name string, raw []byte) (any, error) {
	switch name {
	case "decide":
		var in tools.DecideInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		return svc.Decide(ctx, in)
	case "classify":
		var in tools.ClassifyInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		return svc.Classify(ctx, in)
	case "check":
		var in tools.CheckInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		return svc.Check(ctx, in)
	case "screen":
		var in tools.ScreenInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		return svc.Screen(ctx, in)
	case "rerank":
		var in tools.RerankInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		return svc.Rerank(ctx, in)
	}
	return nil, fmt.Errorf("unknown tool %q", name)
}
