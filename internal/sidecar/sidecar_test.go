package sidecar

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/config"
)

var fakeEngine string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeengine")
	if err != nil {
		panic(err)
	}
	fakeEngine = filepath.Join(dir, "fakeengine")
	if runtime.GOOS == "windows" {
		fakeEngine += ".exe"
	}
	build := exec.Command("go", "build", "-o", fakeEngine, "./testdata/fakeengine")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("build fakeengine: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func newSupervisor(t *testing.T) *Supervisor {
	t.Helper()
	l := config.NewLayout(t.TempDir())
	cfg := config.Default()
	cfg.Backend = "cpu"
	cfg.Port = freePort(t)
	cfg.IdleTimeout.Duration = time.Hour
	s := New(l, cfg, fakeEngine)
	s.readyTimeout = 20 * time.Second
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	})
	return s
}

func TestArgsFromConfig(t *testing.T) {
	t.Parallel()
	l := config.NewLayout(filepath.FromSlash("/h"))
	cases := []struct {
		name    string
		mut     func(*config.Config)
		want    []string
		notWant []string
	}{
		{"cuda default", func(c *config.Config) { c.Backend = "cuda" }, []string{"--cuda", "--tensor-core-fp32", "--flash-fp32"}, nil},
		{"cuda strict", func(c *config.Config) { c.Backend = "cuda"; c.Precision.Strict = true }, []string{"--cuda"}, []string{"--tensor-core-fp32", "--flash-fp32"}},
		{"vulkan default", func(c *config.Config) { c.Backend = "vulkan" }, []string{"--vulkan", "--tensor-core-fp32"}, []string{"--flash-fp32"}},
		{"cpu", func(c *config.Config) { c.Backend = "cpu" }, []string{"--cpu"}, []string{"--tensor-core-fp32", "--flash-fp32"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := config.Default()
			tc.mut(c)
			args := Args(l, c, 45293)
			for _, w := range tc.want {
				if !contains(args, w) {
					t.Errorf("args %v missing %q", args, w)
				}
			}
			for _, nw := range tc.notWant {
				if contains(args, nw) {
					t.Errorf("args %v must not contain %q", args, nw)
				}
			}
			if !contains(args, "--port") || !contains(args, "45293") || !contains(args, "--variant") || !contains(args, c.Model) {
				t.Errorf("args %v missing port or variant", args)
			}
			if !contains(args, "--server") || !contains(args, "127.0.0.1") {
				t.Errorf("args %v must serve on loopback", args)
			}
		})
	}
}

func TestEnsureSpawnsThenAttaches(t *testing.T) {
	s := newSupervisor(t)
	ctx := context.Background()

	h1, err := s.Ensure(ctx)
	if err != nil {
		t.Fatalf("first Ensure() = %v", err)
	}
	defer h1.Close()
	if h1.PID == 0 || h1.Port != s.cfg.Port {
		t.Fatalf("handle = %+v", h1)
	}
	st, err := ReadState(s.layout)
	if err != nil || st.PID != h1.PID || st.Backend != "cpu" || st.Model != "multilingual" {
		t.Fatalf("state = %+v, %v", st, err)
	}

	h2, err := s.Ensure(ctx)
	if err != nil {
		t.Fatalf("second Ensure() = %v", err)
	}
	defer h2.Close()
	if h2.PID != h1.PID {
		t.Errorf("second Ensure spawned a new process: %d vs %d", h2.PID, h1.PID)
	}
	if !h1.Spawned || h2.Spawned {
		t.Errorf("Spawned flags: first=%v second=%v", h1.Spawned, h2.Spawned)
	}
	if n := len(LiveClients(s.layout)); n != 1 {
		t.Errorf("live clients = %d, want 1 (same process registers once)", n)
	}
}

func TestEnsureReplacesStaleLockAndState(t *testing.T) {
	s := newSupervisor(t)
	os.MkdirAll(s.layout.State, 0o755)
	// A lock and state pointing at a PID that cannot be alive.
	os.WriteFile(filepath.Join(s.layout.State, lockFile), []byte("999999"), 0o644)
	WriteState(s.layout, State{PID: 999999, Port: s.cfg.Port, Backend: "cpu", Model: "multilingual"})

	h, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() = %v", err)
	}
	defer h.Close()
	if !h.Spawned {
		t.Error("expected a fresh spawn after a stale lock")
	}
}

func TestEnsureRespawnsWhenConfigChanged(t *testing.T) {
	s := newSupervisor(t)
	h, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h.Close()
	s.cfg.Model = "english"
	h2, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer h2.Close()
	if h2.PID == h.PID || !h2.Spawned {
		t.Errorf("expected respawn after model change, got pid %d (was %d)", h2.PID, h.PID)
	}
	if !alive(h.PID) {
		return
	}
	t.Error("old engine still alive after respawn")
}

func TestEnsureReportsCrashWithLogTail(t *testing.T) {
	s := newSupervisor(t)
	s.extraEnv = []string{"FAKEENGINE_CRASH=1"}
	_, err := s.Ensure(context.Background())
	if err == nil {
		t.Fatal("expected error when the engine exits")
	}
	var se *StartError
	if !errors.As(err, &se) {
		t.Fatalf("err = %T %v, want *StartError", err, err)
	}
	if se.LogTail == "" || !containsStr(se.LogTail, "simulated crash") {
		t.Errorf("LogTail = %q, want the engine's stderr", se.LogTail)
	}
}

func TestPortFallbackWhenBusy(t *testing.T) {
	s := newSupervisor(t)
	// Occupy the configured port with something that is not laya.cpp.
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(s.cfg.Port))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	h, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() = %v", err)
	}
	defer h.Close()
	if h.Port == s.cfg.Port || h.Port > s.cfg.Port+portRange {
		t.Errorf("port = %d, want a fallback within %d..%d", h.Port, s.cfg.Port+1, s.cfg.Port+portRange)
	}
}

func TestStopAndIdleReaper(t *testing.T) {
	s := newSupervisor(t)
	s.cfg.IdleTimeout.Duration = 200 * time.Millisecond
	h, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pid := h.PID
	// While a client is attached the reaper must not stop the engine.
	if stopped, _ := s.ReapIfIdle(context.Background()); stopped {
		t.Fatal("reaper stopped the engine while a client was attached")
	}
	h.Close()
	time.Sleep(300 * time.Millisecond)
	stopped, err := s.ReapIfIdle(context.Background())
	if err != nil || !stopped {
		t.Fatalf("ReapIfIdle = %v, %v; want stopped", stopped, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for alive(pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if alive(pid) {
		t.Error("engine still alive after reap")
	}
	if _, err := ReadState(s.layout); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state should be removed after stop, got %v", err)
	}
}

func TestTouchPostponesReaper(t *testing.T) {
	s := newSupervisor(t)
	s.cfg.IdleTimeout.Duration = 300 * time.Millisecond
	h, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h.Close()
	time.Sleep(200 * time.Millisecond)
	Touch(s.layout)
	time.Sleep(200 * time.Millisecond)
	if stopped, _ := s.ReapIfIdle(context.Background()); stopped {
		t.Error("recent activity must postpone the idle stop")
	}
}

func TestSpawnLaunchesDetachedReaperWithHome(t *testing.T) {
	s := newSupervisor(t)
	marker := filepath.Join(t.TempDir(), "reaper-ran")
	s.SetReaper([]string{fakeEngine, "-touch", marker})
	h, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(marker); err == nil {
			parts := strings.SplitN(string(b)+"\n\n", "\n", 4)
			home, wd, pid := parts[0], parts[1], parts[2]
			if pid != strconv.Itoa(h.PID) {
				t.Errorf("reaper OHMYLAYA_REAP_PID = %q, want %d", pid, h.PID)
			}
			if home != s.layout.Home {
				t.Errorf("reaper OHMYLAYA_HOME = %q, want %q", home, s.layout.Home)
			}
			// Windows cannot delete a process's current directory, so the
			// reaper must not sit inside the home that uninstall removes.
			if rel, err := filepath.Rel(s.layout.Home, wd); err == nil && !strings.HasPrefix(rel, "..") {
				t.Errorf("reaper working directory %q is inside home %q", wd, s.layout.Home)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("reaper command was not started")
}

func TestRunReaperUntilGoneExitsWhenEngineReplaced(t *testing.T) {
	s := newSupervisor(t)
	s.cfg.IdleTimeout.Duration = time.Hour
	h, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	done := make(chan struct{})
	go func() {
		// A reaper spawned for an earlier engine must not keep watching
		// the live replacement, or reapers pile up across restarts.
		s.RunReaperUntilGone(context.Background(), 50*time.Millisecond, h.PID+1)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("reaper kept running for an engine it did not spawn")
	}
	if !alive(h.PID) {
		t.Error("reaper stopped an engine it did not spawn")
	}
}

func TestRunReaperUntilGoneStopsIdleEngineAndReturns(t *testing.T) {
	s := newSupervisor(t)
	s.cfg.IdleTimeout.Duration = 100 * time.Millisecond
	h, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pid := h.PID
	h.Close()
	done := make(chan struct{})
	go func() {
		s.RunReaperUntilGone(context.Background(), 150*time.Millisecond, pid)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("reaper did not return")
	}
	if alive(pid) {
		t.Error("engine still alive after the reaper returned")
	}
}

func TestLogRotationAtSpawn(t *testing.T) {
	s := newSupervisor(t)
	os.MkdirAll(s.layout.State, 0o755)
	big := make([]byte, maxLogBytes+1)
	os.WriteFile(filepath.Join(s.layout.State, logFile), big, 0o644)
	h, err := s.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if _, err := os.Stat(filepath.Join(s.layout.State, logFile+".1")); err != nil {
		t.Errorf("expected rotated log: %v", err)
	}
	st, _ := os.Stat(filepath.Join(s.layout.State, logFile))
	if st.Size() > maxLogBytes {
		t.Errorf("fresh log is %d bytes, want small", st.Size())
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func containsStr(hay, needle string) bool {
	return len(needle) == 0 || (len(hay) >= len(needle) && indexOf(hay, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
