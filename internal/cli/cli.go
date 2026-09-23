// Package cli routes command-line arguments to subcommands.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"text/tabwriter"

	"github.com/QuBiit0/ohmylaya/internal/agents"
	"github.com/QuBiit0/ohmylaya/internal/app"
	"github.com/QuBiit0/ohmylaya/internal/buildinfo"
	"github.com/QuBiit0/ohmylaya/internal/mcpserver"
	"github.com/QuBiit0/ohmylaya/internal/tools"
)

const usage = `Usage: ohmylaya <command> [flags]

Commands:
  mcp         Serve the tools over MCP on stdin/stdout (used by agents)
  ask <tool>  Call one tool with JSON input on stdin: decide, classify, check, screen, rerank
  agents      Show detection and registration status per agent
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
		fmt.Fprint(stdout, usage)
		return exitOK
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
	case "ask":
		return runAsk(args[1:], stdin, stdout, stderr)
	case "agents":
		return runAgents(args[1:], stdout, stderr)
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

func runAgents(args []string, stdout, stderr io.Writer) int {
	asJSON := len(args) > 0 && args[0] == "--json"
	rt, err := app.LoadRuntime()
	if err != nil {
		fmt.Fprintln(stderr, "ohmylaya:", err)
		return exitFailure
	}
	bin, _ := os.Executable()
	env := agents.HostEnv(rt.Layout.Backups, exec.LookPath)
	var rows []agents.Status
	staleFound := false
	for _, a := range agents.All() {
		st := a.Status(env, bin)
		staleFound = staleFound || st.Stale
		rows = append(rows, st)
	}
	if asJSON {
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
