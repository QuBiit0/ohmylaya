package cfgfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const claudeSample = `{
  "numStartups": 12,
  "mcpServers": {
    "engram": {
      "type": "stdio",
      "command": "engram",
      "args": ["mcp"]
    }
  },
  "theme": "dark"
}
`

const opencodeSample = `{
  // OpenCode config with comments
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "context7": {
      "type": "remote",
      "url": "https://mcp.context7.com/mcp", // trailing comment
    },
  },
  "model": "anthropic/claude-sonnet-5",
}
`

func TestSetJSONPathInsertsIntoExistingObjectAndRemoveRestores(t *testing.T) {
	t.Parallel()
	value := []byte(`{
      "type": "stdio",
      "command": "C:\\bin\\ohmylaya.exe",
      "args": ["mcp"]
    }`)
	out, err := SetJSONPath([]byte(claudeSample), []string{"mcpServers", "ohmylaya"}, value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"engram": {`) || !strings.Contains(string(out), `"ohmylaya": {`) || !strings.Contains(string(out), `"theme": "dark"`) {
		t.Errorf("merged = %s", out)
	}
	got, err := ParseJSONC(out)
	if err != nil {
		t.Fatalf("merged output is not valid JSON: %v\n%s", err, out)
	}
	servers := got.(map[string]any)["mcpServers"].(map[string]any)
	if servers["ohmylaya"].(map[string]any)["command"] != `C:\bin\ohmylaya.exe` || servers["engram"] == nil {
		t.Errorf("servers = %+v", servers)
	}

	back, removed, err := RemoveJSONPath(out, []string{"mcpServers", "ohmylaya"})
	if err != nil || !removed {
		t.Fatalf("remove = %v, %v", removed, err)
	}
	if string(back) != claudeSample {
		t.Errorf("round trip differs:\n--- got ---\n%s\n--- want ---\n%s", back, claudeSample)
	}
}

func TestSetJSONPathReplacesExistingValue(t *testing.T) {
	t.Parallel()
	first, _ := SetJSONPath([]byte(claudeSample), []string{"mcpServers", "ohmylaya"}, []byte(`{"command": "old"}`))
	second, err := SetJSONPath(first, []string{"mcpServers", "ohmylaya"}, []byte(`{"command": "new"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(second), "old") || strings.Count(string(second), `"ohmylaya"`) != 1 {
		t.Errorf("replace failed: %s", second)
	}
}

func TestSetJSONPathCreatesMissingParents(t *testing.T) {
	t.Parallel()
	cases := []string{"{}", "{\n}", `{"theme": "dark"}`, "", "  \n"}
	for _, src := range cases {
		out, err := SetJSONPath([]byte(src), []string{"mcpServers", "ohmylaya"}, []byte(`{"command": "x"}`))
		if err != nil {
			t.Fatalf("src %q: %v", src, err)
		}
		got, err := ParseJSONC(out)
		if err != nil {
			t.Fatalf("src %q produced invalid JSON: %v\n%s", src, err, out)
		}
		m := got.(map[string]any)
		if m["mcpServers"].(map[string]any)["ohmylaya"].(map[string]any)["command"] != "x" {
			t.Errorf("src %q: %s", src, out)
		}
		if strings.Contains(src, "theme") && m["theme"] != "dark" {
			t.Errorf("src %q lost sibling: %s", src, out)
		}
	}
}

func TestJSONCCommentsAndTrailingCommasSurvive(t *testing.T) {
	t.Parallel()
	out, err := SetJSONPath([]byte(opencodeSample), []string{"mcp", "ohmylaya"}, []byte(`{"type": "local", "command": ["/usr/local/bin/ohmylaya", "mcp"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "// OpenCode config with comments") || !strings.Contains(string(out), "// trailing comment") {
		t.Errorf("comments lost: %s", out)
	}
	got, err := ParseJSONC(out)
	if err != nil {
		t.Fatalf("invalid JSONC after merge: %v\n%s", err, out)
	}
	mcp := got.(map[string]any)["mcp"].(map[string]any)
	if mcp["context7"] == nil || mcp["ohmylaya"] == nil {
		t.Errorf("mcp = %+v", mcp)
	}
	back, removed, err := RemoveJSONPath(out, []string{"mcp", "ohmylaya"})
	if err != nil || !removed || string(back) != opencodeSample {
		t.Errorf("round trip differs (removed=%v err=%v):\n%s", removed, err, back)
	}
}

func TestRemoveMissingKeyIsNoop(t *testing.T) {
	t.Parallel()
	back, removed, err := RemoveJSONPath([]byte(claudeSample), []string{"mcpServers", "nope"})
	if err != nil || removed || string(back) != claudeSample {
		t.Errorf("noop remove changed content or reported removal")
	}
}

func TestSetJSONPathRejectsUnparseable(t *testing.T) {
	t.Parallel()
	if _, err := SetJSONPath([]byte(`{"a": `), []string{"a"}, []byte("1")); err == nil {
		t.Error("truncated JSON must be rejected")
	}
	if _, err := SetJSONPath([]byte(`[1,2]`), []string{"a"}, []byte("1")); err == nil {
		t.Error("non-object root must be rejected")
	}
}

const codexSample = `# Codex configuration
model = "gpt-5"

[mcp_servers.context7]
command = "npx"
args = ["-y", "@upstash/context7-mcp"]

[mcp_servers.context7.env]
DEBUG = "1"

[features]
fast = true
`

func TestTOMLTableSetReplaceRemove(t *testing.T) {
	t.Parallel()
	body := "command = \"/usr/local/bin/ohmylaya\"\nargs = [\"mcp\"]\n"
	out := SetTOMLTable([]byte(codexSample), "mcp_servers.ohmylaya", body)
	if !strings.Contains(string(out), "[mcp_servers.ohmylaya]\ncommand = \"/usr/local/bin/ohmylaya\"") || !strings.Contains(string(out), "# Codex configuration") || !strings.Contains(string(out), "[features]\nfast = true") {
		t.Errorf("set = %s", out)
	}
	again := SetTOMLTable(out, "mcp_servers.ohmylaya", "command = \"new\"\n")
	if strings.Count(string(again), "[mcp_servers.ohmylaya]") != 1 || strings.Contains(string(again), "/usr/local/bin") {
		t.Errorf("replace = %s", again)
	}
	back, removed := RemoveTOMLTable(out, "mcp_servers.ohmylaya")
	if !removed || string(back) != codexSample {
		t.Errorf("round trip differs (removed=%v):\n%s", removed, back)
	}
	if _, removed := RemoveTOMLTable([]byte(codexSample), "mcp_servers.nope"); removed {
		t.Error("missing table must not report removal")
	}
}

func TestTOMLTableInsertedBeforeSubtablesOfOtherServers(t *testing.T) {
	t.Parallel()
	out := SetTOMLTable([]byte("[mcp_servers.a]\ncommand = \"a\"\n\n[mcp_servers.a.env]\nX = \"1\"\n"), "mcp_servers.ohmylaya", "command = \"o\"\n")
	if !strings.HasSuffix(string(out), "[mcp_servers.ohmylaya]\ncommand = \"o\"\n") {
		t.Errorf("out = %s", out)
	}
	if strings.Contains(string(out), "[mcp_servers.a.env]\nX = \"1\"\n\n[mcp_servers.ohmylaya]") == false {
		t.Errorf("existing subtables must stay intact: %s", out)
	}
}

func TestBackupAndWriteAtomic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "settings.json")
	os.WriteFile(target, []byte("{}"), 0o600)
	bk, err := Backup(filepath.Join(dir, "backups"), "claude", target)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(bk); string(b) != "{}" {
		t.Errorf("backup content = %q", b)
	}
	if !strings.Contains(bk, filepath.Join("backups", "claude")) || !strings.HasSuffix(bk, "-settings.json") {
		t.Errorf("backup path = %s", bk)
	}
	if err := WriteAtomic(target, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(target)
	if b, _ := os.ReadFile(target); string(b) != `{"a":1}` || st.Mode().Perm() != 0o600 && !isWindows() {
		t.Errorf("content = %q mode = %v", b, st.Mode())
	}
	if _, err := Backup(filepath.Join(dir, "backups"), "claude", filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file backup must fail")
	}
}
