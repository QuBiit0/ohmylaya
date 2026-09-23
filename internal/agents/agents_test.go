package agents

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuBiit0/ohmylaya/internal/cfgfile"
)

func testEnv(t *testing.T) Env {
	t.Helper()
	home := t.TempDir()
	return Env{
		Home:       home,
		ConfigHome: filepath.Join(home, ".config"),
		BackupsDir: filepath.Join(home, "backups"),
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const bin = "/opt/ohmylaya/bin/ohmylaya"

func TestAllAdaptersRoundTrip(t *testing.T) {
	samples := map[string]string{
		"claude":   "{\n  \"numStartups\": 3,\n  \"mcpServers\": {\n    \"engram\": {\n      \"type\": \"stdio\",\n      \"command\": \"engram\",\n      \"args\": [\"mcp\"]\n    }\n  }\n}\n",
		"codex":    "model = \"gpt-5\"\n\n[mcp_servers.codegraph]\ncommand = \"codegraph\"\nargs = [\"serve\", \"--mcp\"]\n",
		"opencode": "{\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"mcp\": {\n    \"codegraph\": { \"type\": \"local\", \"command\": [\"codegraph\", \"serve\", \"--mcp\"], \"enabled\": true }\n  }\n}\n",
		"pi":       "{\n  \"mcpServers\": {\n    \"codegraph\": { \"command\": \"codegraph\", \"args\": [\"serve\", \"--mcp\"] }\n  }\n}\n",
	}
	for _, a := range All() {
		t.Run(a.ID(), func(t *testing.T) {
			env := testEnv(t)
			path := a.ConfigPath(env)
			write(t, path, samples[a.ID()])

			before := a.Status(env, bin)
			if before.Registered {
				t.Fatal("must not be registered before Register")
			}
			if err := a.Register(env, bin); err != nil {
				t.Fatalf("Register() = %v", err)
			}
			after := a.Status(env, bin)
			if !after.Registered || after.Stale || after.Command != bin {
				t.Errorf("status after register = %+v", after)
			}
			content := read(t, path)
			if !strings.Contains(content, "codegraph") && !strings.Contains(content, "engram") {
				t.Errorf("existing servers lost:\n%s", content)
			}
			if entries, _ := os.ReadDir(filepath.Join(env.BackupsDir, a.ID())); len(entries) != 1 {
				t.Errorf("backups = %d, want 1", len(entries))
			}

			// Registering again is idempotent.
			if err := a.Register(env, bin); err != nil {
				t.Fatal(err)
			}
			if strings.Count(read(t, path), ServerID) != strings.Count(content, ServerID) {
				t.Error("second Register duplicated the entry")
			}

			removed, err := a.Unregister(env)
			if err != nil || !removed {
				t.Fatalf("Unregister() = %v, %v", removed, err)
			}
			if got := read(t, path); got != samples[a.ID()] {
				t.Errorf("round trip differs:\n--- got ---\n%s\n--- want ---\n%s", got, samples[a.ID()])
			}
			if removed, _ := a.Unregister(env); removed {
				t.Error("second Unregister must be a no-op")
			}
		})
	}
}

func TestStaleDetection(t *testing.T) {
	env := testEnv(t)
	a, _ := ByID("claude")
	write(t, a.ConfigPath(env), `{"mcpServers":{"ohmylaya":{"type":"stdio","command":"/gone/ohmylaya","args":["mcp"]}}}`)
	st := a.Status(env, bin)
	if !st.Registered || !st.Stale {
		t.Errorf("status = %+v, want registered and stale", st)
	}
	live := filepath.Join(env.Home, "other-ohmylaya")
	write(t, live, "x")
	write(t, a.ConfigPath(env), `{"mcpServers":{"ohmylaya":{"type":"stdio","command":"`+strings.ReplaceAll(live, `\`, `\\`)+`","args":["mcp"]}}}`)
	if st := a.Status(env, bin); st.Stale {
		t.Errorf("an existing binary at another path is not stale: %+v", st)
	}
}

func TestOpenCodeV2SchemaGoesUnderServers(t *testing.T) {
	env := testEnv(t)
	a, _ := ByID("opencode")
	write(t, a.ConfigPath(env), "{\n  \"mcp\": {\n    \"servers\": {\n      \"x\": { \"type\": \"local\", \"command\": [\"x\"] }\n    }\n  }\n}\n")
	if err := a.Register(env, bin); err != nil {
		t.Fatal(err)
	}
	v, _ := cfgfile.ParseJSONC([]byte(read(t, a.ConfigPath(env))))
	mcp := v.(map[string]any)["mcp"].(map[string]any)
	servers := mcp["servers"].(map[string]any)
	entry, ok := servers[ServerID].(map[string]any)
	if !ok {
		t.Fatalf("entry not under mcp.servers: %+v", mcp)
	}
	if _, has := entry["enabled"]; has {
		t.Error("v2 entries must not carry enabled")
	}
	if mcp[ServerID] != nil {
		t.Error("entry must not also appear at mcp.ohmylaya")
	}
	st := a.Status(env, bin)
	if !st.Registered || st.Command != bin {
		t.Errorf("status = %+v", st)
	}
}

func TestOpenCodePrefersExistingJSONC(t *testing.T) {
	env := testEnv(t)
	a, _ := ByID("opencode")
	jsonc := filepath.Join(env.ConfigHome, "opencode", "opencode.jsonc")
	write(t, jsonc, "{\n  // comment\n  \"mcp\": {},\n}\n")
	if a.ConfigPath(env) != jsonc {
		t.Fatalf("ConfigPath = %s, want the jsonc file", a.ConfigPath(env))
	}
	if err := a.Register(env, bin); err != nil {
		t.Fatal(err)
	}
	if got := read(t, jsonc); !strings.Contains(got, "// comment") || !strings.Contains(got, ServerID) {
		t.Errorf("jsonc = %s", got)
	}
}

func TestPiWithoutAdapterIsSkippedWithHint(t *testing.T) {
	env := testEnv(t)
	a, _ := ByID("pi")
	os.MkdirAll(filepath.Join(env.Home, ".pi", "agent"), 0o755)
	err := a.Register(env, bin)
	if !errors.Is(err, ErrNoRegistry) || !strings.Contains(err.Error(), "pi-mcp-adapter") {
		t.Errorf("err = %v, want ErrNoRegistry mentioning pi-mcp-adapter", err)
	}
	if exists(a.ConfigPath(env)) {
		t.Error("mcp.json must not be created")
	}
	st := a.Status(env, bin)
	if !st.Detected || st.Registered || !strings.Contains(st.Detail, "pi-mcp-adapter") {
		t.Errorf("status = %+v", st)
	}
}

func TestCreatesMissingClaudeAndCodexFiles(t *testing.T) {
	env := testEnv(t)
	for _, id := range []string{"claude", "codex"} {
		a, _ := ByID(id)
		if err := a.Register(env, bin); err != nil {
			t.Fatalf("%s Register on missing file = %v", id, err)
		}
		if st := a.Status(env, bin); !st.Registered || st.Stale {
			t.Errorf("%s status = %+v", id, st)
		}
	}
}

func TestUnparseableRegistryIsRefused(t *testing.T) {
	env := testEnv(t)
	for _, id := range []string{"claude", "codex"} {
		a, _ := ByID(id)
		write(t, a.ConfigPath(env), "{{{ not valid ]]]")
		if err := a.Register(env, bin); err == nil {
			t.Errorf("%s must refuse an unparseable file", id)
		}
		if read(t, a.ConfigPath(env)) != "{{{ not valid ]]]" {
			t.Errorf("%s changed an unparseable file", id)
		}
		if st := a.Status(env, bin); st.RegistryErr == "" {
			t.Errorf("%s status must report the parse error", id)
		}
	}
}

func TestDetectUsesPathThenDirectories(t *testing.T) {
	env := testEnv(t)
	env.LookPath = func(name string) (string, error) {
		if name == "claude" {
			return "/usr/bin/claude", nil
		}
		return "", errors.New("no")
	}
	a, _ := ByID("claude")
	if ok, detail := a.Detect(env); !ok || detail != "/usr/bin/claude" {
		t.Errorf("Detect = %v %q", ok, detail)
	}
	c, _ := ByID("codex")
	if ok, _ := c.Detect(env); ok {
		t.Error("codex must not be detected without PATH or ~/.codex")
	}
	os.MkdirAll(filepath.Join(env.Home, ".codex"), 0o755)
	if ok, _ := c.Detect(env); !ok {
		t.Error("codex must be detected from ~/.codex")
	}
}

func TestSkillDirsFollowEachAgent(t *testing.T) {
	env := testEnv(t)
	want := map[string]string{
		"claude":   filepath.Join(env.Home, ".claude", "skills", ServerID),
		"codex":    filepath.Join(env.Home, ".agents", "skills", ServerID),
		"opencode": filepath.Join(env.ConfigHome, "opencode", "skills", ServerID),
		"pi":       filepath.Join(env.Home, ".agents", "skills", ServerID),
	}
	for _, a := range All() {
		if got := a.SkillDir(env); got != want[a.ID()] {
			t.Errorf("%s SkillDir = %s, want %s", a.ID(), got, want[a.ID()])
		}
	}
}
