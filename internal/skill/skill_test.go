package skill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuBiit0/ohmylaya/skills"
)

func TestEmbeddedSkillHasRequiredFrontmatterAndBudget(t *testing.T) {
	t.Parallel()
	data, err := skills.FS.ReadFile("ohmylaya/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"name: ohmylaya", `description: "Trigger:`, "license: MIT", "version:"} {
		if !strings.Contains(s, want) {
			t.Errorf("frontmatter missing %q", want)
		}
	}
	body, _ := Body()
	// About 4 characters per token; 1000 tokens is the hard limit.
	if n := len(body); n > 4000 {
		t.Errorf("skill body is %d chars, above the ~1000 token hard limit", n)
	}
	for _, section := range []string{"## Which tool", "## Confidence rule", "## Limits", "doctor"} {
		if !strings.Contains(string(body), section) {
			t.Errorf("body missing %q", section)
		}
	}
	files, _ := Files()
	if len(files) != 3 {
		t.Errorf("files = %v, want SKILL.md and two references", files)
	}
}

func TestInstallStampsVersionAndReportsIt(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), Name)
	if err := Install(dir, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	v, err := InstalledVersion(dir)
	if err != nil || v != "1.2.3" {
		t.Errorf("InstalledVersion = %q, %v", v, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "references", "questions.md")); err != nil {
		t.Error("references not installed")
	}
	// Foreign files survive a reinstall.
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("mine"), 0o644)
	if err := Install(dir, "dev"); err != nil {
		t.Fatal(err)
	}
	if v, _ := InstalledVersion(dir); v == "dev" {
		t.Error("dev builds must keep the source version instead of stamping dev")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.md")); err != nil {
		t.Error("reinstall removed a foreign file")
	}
}

func TestInstalledVersionMissing(t *testing.T) {
	t.Parallel()
	_, err := InstalledVersion(filepath.Join(t.TempDir(), Name))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want ErrNotExist", err)
	}
}

func TestRemoveRefusesNonSkillDir(t *testing.T) {
	t.Parallel()
	if err := Remove(t.TempDir()); err == nil {
		t.Error("must refuse to remove a directory not named ohmylaya")
	}
	dir := filepath.Join(t.TempDir(), Name)
	Install(dir, "1.0.0")
	if err := Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Error("skill dir still exists")
	}
}
