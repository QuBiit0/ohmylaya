// Package cfgfile edits agent configuration files by splicing text so every
// byte the user wrote, including comments, survives untouched.
package cfgfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Backup copies path to <backupsDir>/<agent>/<timestamp>-<name> and returns
// the copy's path.
func Backup(backupsDir, agent, path string) (string, error) {
	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dir := filepath.Join(backupsDir, agent)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, time.Now().UTC().Format("20060102T150405Z")+"-"+filepath.Base(path))
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return "", err
	}
	return dst, out.Close()
}

// WriteAtomic writes data through a temp file and rename, preserving the
// existing file mode (0644 for new files).
func WriteAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".ohmylaya.tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Chmod(path, mode)
}

func isWindows() bool { return runtime.GOOS == "windows" }

// ParseJSONC parses JSON that may contain comments and trailing commas.
func ParseJSONC(src []byte) (any, error) {
	clean, err := stripJSONC(src)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(clean, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// stripJSONC blanks comments and removes trailing commas without moving
// any other byte, so offsets stay aligned with the original.
func stripJSONC(src []byte) ([]byte, error) {
	out := make([]byte, len(src))
	copy(out, src)
	i := 0
	for i < len(out) {
		switch {
		case out[i] == '"':
			end, err := scanString(out, i)
			if err != nil {
				return nil, err
			}
			i = end
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '/':
			for i < len(out) && out[i] != '\n' {
				out[i] = ' '
				i++
			}
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '*':
			for i < len(out) && !(out[i] == '*' && i+1 < len(out) && out[i+1] == '/') {
				if out[i] != '\n' {
					out[i] = ' '
				}
				i++
			}
			if i+1 < len(out) {
				out[i], out[i+1] = ' ', ' '
				i += 2
			}
		case out[i] == ',':
			// Trailing comma: next token after whitespace and comments is } or ].
			j := skipWS(out, i+1)
			if j < len(out) && (out[j] == '}' || out[j] == ']') {
				out[i] = ' '
			}
			i++
		default:
			i++
		}
	}
	return out, nil
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func scanString(src []byte, i int) (int, error) {
	i++ // opening quote
	for i < len(src) {
		switch src[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1, nil
		default:
			i++
		}
	}
	return 0, errors.New("cfgfile: unterminated string")
}

// skipWS advances past whitespace and comments.
func skipWS(src []byte, i int) int {
	for i < len(src) {
		switch {
		case isSpace(src[i]):
			i++
		case src[i] == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case src[i] == '/' && i+1 < len(src) && src[i+1] == '*':
			j := bytes.Index(src[i+2:], []byte("*/"))
			if j < 0 {
				return len(src)
			}
			i += 2 + j + 2
		default:
			return i
		}
	}
	return i
}

// scanValue returns the index just past the value starting at i.
func scanValue(src []byte, i int) (int, error) {
	if i >= len(src) {
		return 0, errors.New("cfgfile: unexpected end of input")
	}
	switch src[i] {
	case '"':
		return scanString(src, i)
	case '{', '[':
		open, closeCh := src[i], byte('}')
		if open == '[' {
			closeCh = ']'
		}
		i++
		for {
			i = skipWS(src, i)
			if i >= len(src) {
				return 0, errors.New("cfgfile: unterminated container")
			}
			if src[i] == closeCh {
				return i + 1, nil
			}
			if src[i] == ',' {
				i++
				continue
			}
			end, err := scanValue(src, i)
			if err != nil {
				return 0, err
			}
			i = end
			i = skipWS(src, i)
			if i < len(src) && src[i] == ':' {
				i = skipWS(src, i+1)
				end, err := scanValue(src, i)
				if err != nil {
					return 0, err
				}
				i = end
			}
		}
	default:
		for i < len(src) && !isSpace(src[i]) && src[i] != ',' && src[i] != '}' && src[i] != ']' && src[i] != '/' {
			i++
		}
		return i, nil
	}
}

// member is one key/value pair inside an object.
type member struct {
	keyStart, valStart, valEnd int
	key                        string
}

// members lists the members of the object whose '{' is at objStart.
func members(src []byte, objStart int) ([]member, int, error) {
	if objStart >= len(src) || src[objStart] != '{' {
		return nil, 0, errors.New("cfgfile: expected object")
	}
	var out []member
	i := objStart + 1
	for {
		i = skipWS(src, i)
		if i >= len(src) {
			return nil, 0, errors.New("cfgfile: unterminated object")
		}
		if src[i] == '}' {
			return out, i, nil
		}
		if src[i] == ',' {
			i++
			continue
		}
		if src[i] != '"' {
			return nil, 0, fmt.Errorf("cfgfile: expected key at offset %d", i)
		}
		keyStart := i
		end, err := scanString(src, i)
		if err != nil {
			return nil, 0, err
		}
		var key string
		if err := json.Unmarshal(src[keyStart:end], &key); err != nil {
			return nil, 0, err
		}
		i = skipWS(src, end)
		if i >= len(src) || src[i] != ':' {
			return nil, 0, fmt.Errorf("cfgfile: expected ':' at offset %d", i)
		}
		valStart := skipWS(src, i+1)
		valEnd, err := scanValue(src, valStart)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, member{keyStart: keyStart, valStart: valStart, valEnd: valEnd, key: key})
		i = valEnd
	}
}

// SetJSONPath sets path to valueJSON inside a JSON or JSONC document,
// creating missing parent objects and touching nothing else.
func SetJSONPath(src []byte, path []string, valueJSON []byte) ([]byte, error) {
	if len(path) == 0 {
		return nil, errors.New("cfgfile: empty path")
	}
	if bytes.TrimSpace(src) == nil || len(bytes.TrimSpace(src)) == 0 {
		src = []byte("{}\n")
	}
	if _, err := ParseJSONC(src); err != nil {
		return nil, fmt.Errorf("cfgfile: not valid JSON: %w", err)
	}
	root := skipWS(src, 0)
	if root >= len(src) || src[root] != '{' {
		return nil, errors.New("cfgfile: root is not an object")
	}
	return setIn(src, root, path, valueJSON, 1)
}

func setIn(src []byte, objStart int, path []string, valueJSON []byte, depth int) ([]byte, error) {
	ms, closeIdx, err := members(src, objStart)
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		if m.key != path[0] {
			continue
		}
		if len(path) == 1 {
			return splice(src, m.valStart, m.valEnd, reindent(valueJSON, indentOf(src, m.keyStart))), nil
		}
		if m.valStart >= len(src) || src[m.valStart] != '{' {
			return nil, fmt.Errorf("cfgfile: %q is not an object", path[0])
		}
		return setIn(src, m.valStart, path[1:], valueJSON, depth+1)
	}
	// Build the missing chain as one nested value.
	value := valueJSON
	for i := len(path) - 1; i >= 1; i-- {
		k, _ := json.Marshal(path[i])
		value = []byte("{\n" + strings.Repeat("  ", 1) + string(k) + ": " + string(reindent(value, "  ")) + "\n}")
	}
	k, _ := json.Marshal(path[0])
	var indent string
	if len(ms) > 0 {
		indent = indentOf(src, ms[0].keyStart)
	} else {
		indent = indentOf(src, objStart) + "  "
	}
	entry := string(k) + ": " + string(reindent(value, indent))
	if len(ms) == 0 {
		// Empty object: {} becomes a one-member block.
		return splice(src, objStart, closeIdx+1, []byte("{\n"+indent+entry+"\n"+indentOf(src, objStart)+"}")), nil
	}
	last := ms[len(ms)-1]
	return splice(src, last.valEnd, last.valEnd, []byte(",\n"+indent+entry)), nil
}

// RemoveJSONPath deletes the member at path. It reports whether anything
// was removed. Removing the last member of an object collapses it to {}.
func RemoveJSONPath(src []byte, path []string) ([]byte, bool, error) {
	if _, err := ParseJSONC(src); err != nil {
		return nil, false, fmt.Errorf("cfgfile: not valid JSON: %w", err)
	}
	root := skipWS(src, 0)
	if root >= len(src) || src[root] != '{' {
		return nil, false, errors.New("cfgfile: root is not an object")
	}
	return removeIn(src, root, path)
}

func removeIn(src []byte, objStart int, path []string) ([]byte, bool, error) {
	ms, closeIdx, err := members(src, objStart)
	if err != nil {
		return nil, false, err
	}
	for idx, m := range ms {
		if m.key != path[0] {
			continue
		}
		if len(path) > 1 {
			if src[m.valStart] != '{' {
				return src, false, nil
			}
			return removeIn(src, m.valStart, path[1:])
		}
		switch {
		case len(ms) == 1:
			return splice(src, objStart, closeIdx+1, []byte("{}")), true, nil
		case idx == len(ms)-1:
			// Last member: cut from the previous value's end (the comma) to ours.
			return splice(src, ms[idx-1].valEnd, m.valEnd, nil), true, nil
		default:
			// Cut from our key to the next key, keeping the next key's indent.
			next := ms[idx+1]
			lineStart := lineStartOf(src, next.keyStart)
			return splice(src, lineStartOf(src, m.keyStart), lineStart, nil), true, nil
		}
	}
	return src, false, nil
}

func splice(src []byte, start, end int, replacement []byte) []byte {
	out := make([]byte, 0, len(src)-(end-start)+len(replacement))
	out = append(out, src[:start]...)
	out = append(out, replacement...)
	out = append(out, src[end:]...)
	return out
}

// indentOf returns the leading whitespace of the line containing offset.
func indentOf(src []byte, offset int) string {
	start := lineStartOf(src, offset)
	end := start
	for end < len(src) && (src[end] == ' ' || src[end] == '\t') {
		end++
	}
	return string(src[start:end])
}

func lineStartOf(src []byte, offset int) int {
	i := offset
	for i > 0 && src[i-1] != '\n' {
		i--
	}
	return i
}

// reindent re-encodes a JSON value with two-space indentation relative to
// the given base indent. Non-object values are returned compacted.
func reindent(valueJSON []byte, base string) []byte {
	var buf bytes.Buffer
	if err := json.Indent(&buf, valueJSON, base, "  "); err != nil {
		return bytes.TrimSpace(valueJSON)
	}
	return buf.Bytes()
}

// SetTOMLTable replaces or appends a [table] block. The body must end with
// a newline. Everything outside the block is untouched.
func SetTOMLTable(src []byte, table, body string) []byte {
	header := "[" + table + "]"
	start, end, found := tomlTableRange(src, table)
	block := header + "\n" + body
	if found {
		return splice(src, start, end, []byte(block))
	}
	out := src
	if len(out) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	if len(out) > 0 {
		out = append(out, '\n')
	}
	return append(out, []byte(block)...)
}

// RemoveTOMLTable deletes a [table] block and the blank line before it.
func RemoveTOMLTable(src []byte, table string) ([]byte, bool) {
	start, end, found := tomlTableRange(src, table)
	if !found {
		return src, false
	}
	if start >= 1 && src[start-1] == '\n' && start >= 2 && src[start-2] == '\n' {
		start--
	}
	return splice(src, start, end, nil), true
}

// tomlTableRange finds the byte range of the header line through the line
// before the next header or EOF.
func tomlTableRange(src []byte, table string) (int, int, bool) {
	lines := bytes.SplitAfter(src, []byte("\n"))
	offset := 0
	start := -1
	for _, line := range lines {
		trim := strings.TrimSpace(string(line))
		isHeader := strings.HasPrefix(trim, "[") && !strings.HasPrefix(trim, "[[")
		if start >= 0 && isHeader {
			return start, offset, true
		}
		if isHeader && tomlHeaderName(trim) == table {
			start = offset
		}
		offset += len(line)
	}
	if start >= 0 {
		return start, len(src), true
	}
	return 0, 0, false
}

func tomlHeaderName(line string) string {
	end := strings.Index(line, "]")
	if end < 0 {
		return ""
	}
	name := strings.TrimSpace(line[1:end])
	parts := strings.Split(name, ".")
	for i, p := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(p), `"'`)
	}
	return strings.Join(parts, ".")
}
