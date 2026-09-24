package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/QuBiit0/ohmylaya/internal/buildinfo"
	"github.com/QuBiit0/ohmylaya/internal/sidecar"
)

func TestRun(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "version subcommand prints version",
			args:       []string{"version"},
			wantCode:   0,
			wantStdout: "ohmylaya " + buildinfo.Version + "\n",
		},
		{
			name:       "version flag prints version",
			args:       []string{"--version"},
			wantCode:   0,
			wantStdout: "ohmylaya " + buildinfo.Version + "\n",
		},
		{
			name:       "help prints usage",
			args:       []string{"help"},
			wantCode:   0,
			wantStdout: "Usage: ohmylaya",
		},
		{
			name:       "unknown subcommand fails with hint",
			args:       []string{"frobnicate"},
			wantCode:   2,
			wantStderr: `unknown command "frobnicate"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := Run(tc.args, &stdout, &stderr)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", code, tc.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}

func TestAgentsListsEveryAgentInFreshHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("OHMYLAYA_HOME", home+"/.ohmylaya")
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"agents"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d stderr = %s", code, stderr.String())
	}
	for _, name := range []string{"Claude Code", "Codex", "OpenCode", "Pi"} {
		if !strings.Contains(stdout.String(), name) {
			t.Errorf("missing %s in\n%s", name, stdout.String())
		}
	}
	stdout.Reset()
	if code := Run([]string{"agents", "--json"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), `"id": "pi"`) {
		t.Errorf("json output = %s", stdout.String())
	}

	// Register then unregister Claude Code in the fresh home.
	stdout.Reset()
	if code := Run([]string{"agents", "--register", "claude"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "Registered Claude Code") {
		t.Fatalf("register: code=%d out=%s err=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	Run([]string{"agents", "--json"}, &stdout, &stderr)
	if !strings.Contains(stdout.String(), `"registered": true`) {
		t.Errorf("claude not registered: %s", stdout.String())
	}
	stdout.Reset()
	if code := Run([]string{"agents", "--unregister", "claude"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "Unregistered Claude Code") {
		t.Fatalf("unregister: code=%d out=%s", code, stdout.String())
	}
	if code := Run([]string{"agents", "--register", "emacs"}, &stdout, &stderr); code != 2 {
		t.Errorf("unknown agent exit = %d, want 2", code)
	}
}

func TestRunNoArgsPrintsStatusAndUsageOnNonInteractiveStdin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("OHMYLAYA_HOME", home+"/.ohmylaya")
	var stdout, stderr bytes.Buffer
	code := RunWithStdin(nil, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr %s)", code, stderr.String())
	}
	for _, want := range []string{"ohmylaya " + buildinfo.Version, "backend vulkan", "Usage: ohmylaya"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestReapEnginePID(t *testing.T) {
	t.Parallel()
	cases := map[string]int{"4242": 4242, "": 0, "not-a-pid": 0, "-3": 0}
	for value, want := range cases {
		getenv := func(key string) string {
			if key == sidecar.ReapPIDEnv {
				return value
			}
			return ""
		}
		if got := reapEnginePID(getenv); got != want {
			t.Errorf("reapEnginePID(%q) = %d, want %d", value, got, want)
		}
	}
}
