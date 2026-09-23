// Package sidecar starts, shares and stops the laya.cpp HTTP server.
package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/config"
	"github.com/QuBiit0/ohmylaya/internal/jev"
)

const (
	stateFile    = "sidecar.json"
	lockFile     = "sidecar.lock"
	logFile      = "sidecar.log"
	activityFile = "last-activity"
	clientsDir   = "clients"
	portRange    = 7
	maxLogBytes  = 5 << 20
	logTailLines = 50
)

// State describes the running engine, persisted in state/sidecar.json.
type State struct {
	PID          int       `json:"pid"`
	Port         int       `json:"port"`
	Backend      string    `json:"backend"`
	Model        string    `json:"model"`
	StartedAt    time.Time `json:"started_at"`
	EnginePath   string    `json:"engine_path"`
	EngineDigest string    `json:"engine_digest,omitempty"`
}

// BaseURL is the loopback URL of the engine.
func (s State) BaseURL() string { return "http://127.0.0.1:" + strconv.Itoa(s.Port) }

// StartError carries the engine's last log lines when it fails to start.
type StartError struct {
	Reason  string
	LogTail string
}

func (e *StartError) Error() string {
	return "sidecar: engine failed to start: " + e.Reason
}

// Supervisor owns one engine per home directory.
type Supervisor struct {
	layout       config.Layout
	cfg          *config.Config
	enginePath   string
	engineDigest string
	client       *http.Client
	readyTimeout time.Duration
	extraEnv     []string
}

// New creates a supervisor for the engine executable at enginePath.
func New(layout config.Layout, cfg *config.Config, enginePath string) *Supervisor {
	return &Supervisor{
		layout:       layout,
		cfg:          cfg,
		enginePath:   enginePath,
		client:       &http.Client{Timeout: 5 * time.Second},
		readyTimeout: 120 * time.Second,
	}
}

// SetEngineDigest records the digest of the executable in the state file.
func (s *Supervisor) SetEngineDigest(d string) { s.engineDigest = d }

// Handle is an attached client. Close unregisters it.
type Handle struct {
	PID     int
	Port    int
	Spawned bool
	layout  config.Layout
}

// BaseURL is the loopback URL of the engine.
func (h *Handle) BaseURL() string { return "http://127.0.0.1:" + strconv.Itoa(h.Port) }

// Close removes this process's client registration.
func (h *Handle) Close() {
	_ = os.Remove(filepath.Join(h.layout.State, clientsDir, strconv.Itoa(os.Getpid())))
}

// Args builds the engine command line from configuration.
func Args(l config.Layout, c *config.Config, port int) []string {
	args := []string{"--server", "--host", "127.0.0.1", "--port", strconv.Itoa(port),
		"--model", l.Models, "--variant", c.Model,
		"--max-questions", strconv.Itoa(c.Batch.MaxQuestions),
		"--max-pending-requests", strconv.Itoa(c.Batch.MaxPending),
		"--batch-wait-ms", strconv.Itoa(c.Batch.WaitMS),
	}
	switch c.Backend {
	case "cuda":
		args = append(args, "--cuda")
		if !c.Precision.Strict {
			args = append(args, "--tensor-core-fp32", "--flash-fp32")
		}
	case "vulkan":
		args = append(args, "--vulkan")
		if !c.Precision.Strict {
			args = append(args, "--tensor-core-fp32")
		}
	default:
		args = append(args, "--cpu")
	}
	return args
}

// Ensure returns a handle to a healthy engine, starting one if needed.
func (s *Supervisor) Ensure(ctx context.Context) (*Handle, error) {
	if err := os.MkdirAll(filepath.Join(s.layout.State, clientsDir), 0o755); err != nil {
		return nil, err
	}
	if st, ok := s.attachable(ctx); ok {
		return s.attach(st, false), nil
	}

	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	// Another process may have started it while we waited for the lock.
	if st, ok := s.attachable(ctx); ok {
		return s.attach(st, false), nil
	}
	// A running engine with a different configuration is replaced.
	if st, err := ReadState(s.layout); err == nil && alive(st.PID) {
		_ = s.stopState(ctx, st)
	}

	st, err := s.spawn(ctx)
	if err != nil {
		return nil, err
	}
	return s.attach(st, true), nil
}

func (s *Supervisor) attach(st State, spawned bool) *Handle {
	_ = os.WriteFile(filepath.Join(s.layout.State, clientsDir, strconv.Itoa(os.Getpid())), []byte(strconv.Itoa(os.Getpid())), 0o644)
	Touch(s.layout)
	return &Handle{PID: st.PID, Port: st.Port, Spawned: spawned, layout: s.layout}
}

// attachable reports whether the recorded engine is alive, healthy and
// matches the current configuration.
func (s *Supervisor) attachable(ctx context.Context) (State, bool) {
	st, err := ReadState(s.layout)
	if err != nil || !alive(st.PID) {
		return st, false
	}
	if st.Backend != s.cfg.Backend || st.Model != s.cfg.Model || st.EnginePath != s.enginePath {
		return st, false
	}
	h, err := jev.NewLocal(st.BaseURL(), s.client, s.cfg.Batch.MaxQuestions).Health(ctx)
	return st, err == nil && h.Ready() && h.Variant == s.cfg.Model
}

func (s *Supervisor) lock() (func(), error) {
	path := filepath.Join(s.layout.State, lockFile)
	for attempt := 0; attempt < 100; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprint(f, os.Getpid())
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("sidecar: lock: %w", err)
		}
		if b, rerr := os.ReadFile(path); rerr == nil {
			if pid, perr := strconv.Atoi(strings.TrimSpace(string(b))); perr != nil || !alive(pid) {
				_ = os.Remove(path)
				continue
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("sidecar: lock held by another live process for too long")
}

func (s *Supervisor) spawn(ctx context.Context) (State, error) {
	port, err := s.choosePort()
	if err != nil {
		return State{}, err
	}
	logPath := filepath.Join(s.layout.State, logFile)
	rotateLog(logPath)
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return State{}, err
	}
	defer logf.Close()

	cmd := exec.Command(s.enginePath, Args(s.layout, s.cfg, port)...)
	cmd.Dir = s.layout.Home
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.Env = append(os.Environ(), s.extraEnv...)
	cmd.Env = append(cmd.Env, "PATH="+s.layout.Bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	detach(cmd)
	fmt.Fprintf(logf, "\n[ohmylaya] %s starting %s %s\n", time.Now().Format(time.RFC3339), s.enginePath, strings.Join(cmd.Args[1:], " "))
	if err := cmd.Start(); err != nil {
		return State{}, &StartError{Reason: err.Error(), LogTail: tail(logPath)}
	}
	st := State{PID: cmd.Process.Pid, Port: port, Backend: s.cfg.Backend, Model: s.cfg.Model, StartedAt: time.Now(), EnginePath: s.enginePath, EngineDigest: s.engineDigest}
	// Reap the child in the background so it never becomes a zombie while we
	// live; once we exit the detached child is reparented by the OS.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	deadline := time.Now().Add(s.readyTimeout)
	local := jev.NewLocal(st.BaseURL(), s.client, s.cfg.Batch.MaxQuestions)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			return State{}, &StartError{Reason: "exited before becoming ready: " + errString(err), LogTail: tail(logPath)}
		case <-ctx.Done():
			_ = terminate(cmd.Process)
			return State{}, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		if h, err := local.Health(ctx); err == nil && h.Ready() {
			if err := WriteState(s.layout, st); err != nil {
				return State{}, err
			}
			return st, nil
		}
	}
	_ = terminate(cmd.Process)
	return State{}, &StartError{Reason: "not ready within " + s.readyTimeout.String(), LogTail: tail(logPath)}
}

// choosePort returns the configured port or the next free one in range.
func (s *Supervisor) choosePort() (int, error) {
	for p := s.cfg.Port; p <= s.cfg.Port+portRange; p++ {
		l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
		if err == nil {
			l.Close()
			return p, nil
		}
	}
	return 0, fmt.Errorf("sidecar: no free port in %d..%d", s.cfg.Port, s.cfg.Port+portRange)
}

// Stop terminates the recorded engine if it is alive.
func (s *Supervisor) Stop(ctx context.Context) error {
	st, err := ReadState(s.layout)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.stopState(ctx, st)
}

func (s *Supervisor) stopState(ctx context.Context, st State) error {
	defer os.Remove(filepath.Join(s.layout.State, stateFile))
	if !alive(st.PID) {
		return nil
	}
	proc, err := os.FindProcess(st.PID)
	if err != nil {
		return nil
	}
	local := jev.NewLocal(st.BaseURL(), s.client, s.cfg.Batch.MaxQuestions)
	drain := time.Now().Add(10 * time.Second)
	for time.Now().Before(drain) {
		h, err := local.Health(ctx)
		if err != nil || (h.PendingRequests == 0 && h.QueuedRequests == 0) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := terminate(proc); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for alive(st.PID) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if alive(st.PID) {
		return proc.Kill()
	}
	return nil
}

// ReapIfIdle stops the engine when no client is attached and no activity
// happened within the idle timeout. It reports whether it stopped it.
func (s *Supervisor) ReapIfIdle(ctx context.Context) (bool, error) {
	if s.cfg.IdleTimeout.Duration == 0 {
		return false, nil
	}
	if len(LiveClients(s.layout)) > 0 {
		return false, nil
	}
	st, err := ReadState(s.layout)
	if err != nil {
		return false, nil
	}
	last := lastActivity(s.layout)
	if last.IsZero() {
		last = st.StartedAt
	}
	if time.Since(last) < s.cfg.IdleTimeout.Duration {
		return false, nil
	}
	return true, s.stopState(ctx, st)
}

// RunReaper checks idleness on an interval until ctx ends.
func (s *Supervisor) RunReaper(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = s.ReapIfIdle(ctx)
		}
	}
}

// Touch records activity so the reaper keeps the engine alive.
func Touch(l config.Layout) {
	p := filepath.Join(l.State, activityFile)
	now := time.Now()
	if err := os.Chtimes(p, now, now); err != nil {
		_ = os.MkdirAll(l.State, 0o755)
		_ = os.WriteFile(p, []byte(now.Format(time.RFC3339)), 0o644)
	}
}

func lastActivity(l config.Layout) time.Time {
	st, err := os.Stat(filepath.Join(l.State, activityFile))
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

// LiveClients returns the PIDs of attached processes that are still alive,
// removing registrations of dead ones.
func LiveClients(l config.Layout) []int {
	dir := filepath.Join(l.State, clientsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var live []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || !alive(pid) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
			continue
		}
		live = append(live, pid)
	}
	return live
}

// ReadState loads state/sidecar.json.
func ReadState(l config.Layout) (State, error) {
	var st State
	b, err := os.ReadFile(filepath.Join(l.State, stateFile))
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, fmt.Errorf("sidecar: parse state: %w", err)
	}
	return st, nil
}

// WriteState saves state/sidecar.json atomically.
func WriteState(l config.Layout, st State) error {
	if err := os.MkdirAll(l.State, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	p := filepath.Join(l.State, stateFile)
	if err := os.WriteFile(p+".tmp", b, 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// LogPath returns the engine log path.
func LogPath(l config.Layout) string { return filepath.Join(l.State, logFile) }

// LogTail returns the last lines of the engine log.
func LogTail(l config.Layout) string { return tail(LogPath(l)) }

func rotateLog(path string) {
	st, err := os.Stat(path)
	if err != nil || st.Size() <= maxLogBytes {
		return
	}
	_ = os.Remove(path + ".2")
	_ = os.Rename(path+".1", path+".2")
	_ = os.Rename(path, path+".1")
}

func tail(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > logTailLines {
		lines = lines[len(lines)-logTailLines:]
	}
	return strings.Join(lines, "\n")
}

func errString(err error) string {
	if err == nil {
		return "exit status 0"
	}
	return err.Error()
}
