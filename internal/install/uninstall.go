package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/agents"
	"github.com/QuBiit0/ohmylaya/internal/config"
	"github.com/QuBiit0/ohmylaya/internal/sidecar"
	"github.com/QuBiit0/ohmylaya/internal/skill"
)

// UninstallOptions control removal.
type UninstallOptions struct {
	KeepModels bool
}

// Uninstall stops the engine, unregisters every agent, removes the skills
// and deletes the home directory. Other settings in agent files are kept.
func Uninstall(ctx context.Context, deps Deps, opts UninstallOptions) error {
	if deps.Out == nil {
		deps.Out = io.Discard
	}
	if cfg, err := config.Load(deps.Layout); err == nil {
		stopCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		if err := sidecar.New(deps.Layout, cfg, EnginePath(deps.Layout)).Stop(stopCtx); err != nil {
			fmt.Fprintf(deps.Out, "Could not stop the engine: %v\n", err)
		}
		cancel()
	}
	for _, a := range agents.All() {
		removed, err := a.Unregister(deps.Env)
		if err != nil {
			fmt.Fprintf(deps.Out, "Could not unregister %s: %v\n", a.Name(), err)
			continue
		}
		if removed {
			fmt.Fprintf(deps.Out, "Unregistered %s\n", a.Name())
		}
		if err := skill.Remove(a.SkillDir(deps.Env)); err != nil {
			fmt.Fprintf(deps.Out, "Could not remove skill for %s: %v\n", a.Name(), err)
		}
	}
	if opts.KeepModels {
		for _, sub := range []string{deps.Layout.Bin, deps.Layout.State, deps.Layout.Backups, deps.Layout.Skills, deps.Layout.ConfigPath} {
			_ = removeAllRetry(ctx, sub)
		}
		fmt.Fprintf(deps.Out, "Removed everything under %s except models\n", deps.Layout.Home)
		return nil
	}
	if filepath.Base(deps.Layout.Home) == "" || deps.Layout.Home == string(filepath.Separator) {
		return fmt.Errorf("uninstall: refusing to remove %q", deps.Layout.Home)
	}
	if err := removeAllRetry(ctx, deps.Layout.Home); err != nil {
		return err
	}
	fmt.Fprintf(deps.Out, "Removed %s\n", deps.Layout.Home)
	return nil
}

// removeAttempts and removeRetryWait form the removal retry budget: ten
// attempts 500 ms apart, about five seconds in total.
const removeAttempts = 10

// removeAll and removeRetryWait are swapped in tests to simulate a locked
// path and count waits without waiting in real time.
var (
	removeAll       = os.RemoveAll
	removeRetryWait = func() <-chan time.Time { return time.After(500 * time.Millisecond) }
)

// removeAllRetry retries for a few seconds because Windows keeps a killed
// process's executable locked briefly after the process is gone. It stops
// early when ctx ends.
func removeAllRetry(ctx context.Context, path string) error {
	var err error
	for attempt := 1; ; attempt++ {
		if err = removeAll(path); err == nil || attempt == removeAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-removeRetryWait():
		}
	}
}
