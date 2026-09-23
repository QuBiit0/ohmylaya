// Package skill installs the embedded ohmylaya skill into agent skill
// directories and reports the installed version.
package skill

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/QuBiit0/ohmylaya/skills"
)

// Name is the skill directory name.
const Name = "ohmylaya"

var versionLine = regexp.MustCompile(`(?m)^(\s*version:\s*)"[^"]*"`)

// Files lists the embedded skill files relative to the skill directory.
func Files() ([]string, error) {
	var out []string
	err := fs.WalkDir(skills.FS, Name, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(Name, filepath.FromSlash(p))
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

// Install writes the skill into dir (which becomes the skill directory) and
// stamps the binary version into the frontmatter. Existing files are
// overwritten; other files in dir are left alone.
func Install(dir, version string) error {
	files, err := Files()
	if err != nil {
		return err
	}
	for _, rel := range files {
		data, err := skills.FS.ReadFile(Name + "/" + rel)
		if err != nil {
			return err
		}
		if rel == "SKILL.md" && version != "" && version != "dev" {
			data = versionLine.ReplaceAll(data, []byte(`${1}"`+version+`"`))
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		tmp := dst + ".tmp"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, dst); err != nil {
			return err
		}
	}
	return nil
}

// Remove deletes the skill directory.
func Remove(dir string) error {
	if filepath.Base(dir) != Name {
		return fmt.Errorf("skill: refusing to remove %s, not a skill directory", dir)
	}
	err := os.RemoveAll(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// InstalledVersion reads metadata.version from dir/SKILL.md. It returns
// os.ErrNotExist when the skill is absent.
func InstalledVersion(dir string) (string, error) {
	f, err := os.Open(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	inFront := false
	for sc.Scan() {
		line := sc.Text()
		if line == "---" {
			if inFront {
				break
			}
			inFront = true
			continue
		}
		if !inFront {
			continue
		}
		if m := versionLine.FindStringSubmatch(line); m != nil {
			return strings.Trim(strings.TrimPrefix(line, m[1]), `"`), nil
		}
	}
	return "", errors.New("skill: SKILL.md has no metadata.version")
}

// Body returns the SKILL.md body without frontmatter, for budget checks.
func Body() ([]byte, error) {
	data, err := skills.FS.ReadFile(Name + "/SKILL.md")
	if err != nil {
		return nil, err
	}
	parts := bytes.SplitN(data, []byte("\n---\n"), 3)
	if len(parts) < 3 {
		return data, nil
	}
	return parts[2], nil
}
