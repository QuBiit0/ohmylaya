package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/QuBiit0/ohmylaya/internal/buildinfo"
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

func TestRunNoArgsPrintsUsageOnNonInteractiveStdout(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Usage: ohmylaya") {
		t.Errorf("stdout = %q, want usage", stdout.String())
	}
}
