package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Ref points at content ohmylaya reads itself so it never passes through the
// agent's context as tool arguments.
type Ref struct {
	Path string `json:"path,omitempty" jsonschema:"Read this file (up to 2 MB)"`
	URL  string `json:"url,omitempty" jsonschema:"Fetch this http(s) URL (10 second timeout, HTML reduced to text)"`
}

// Item is one candidate or classification item.
type Item struct {
	ID   string `json:"id"`
	Text string `json:"text,omitempty"`
}

const (
	maxRefBytes  = 2 << 20
	fetchTimeout = 10 * time.Second
	excerptRunes = 200
)

// Reader resolves references.
type Reader struct {
	client *http.Client
}

// NewReader creates a reader; a nil client uses a default with timeout.
func NewReader(client *http.Client) *Reader {
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout}
	}
	return &Reader{client: client}
}

// Resolve returns inline content when given, else reads the reference.
func (r *Reader) Resolve(ctx context.Context, inline any, ref Ref) (any, error) {
	if inline != nil {
		if s, ok := inline.(string); !ok || s != "" {
			return inline, nil
		}
	}
	switch {
	case ref.Path != "":
		return r.ReadFile(ref.Path)
	case ref.URL != "":
		return r.Fetch(ctx, ref.URL)
	}
	return nil, fmt.Errorf("%w: provide content inline or a path or url reference", ErrInput)
}

// ReadFile reads a bounded text file.
func (r *Reader) ReadFile(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInput, err)
	}
	if st.IsDir() {
		return "", fmt.Errorf("%w: %s is a directory", ErrInput, path)
	}
	if st.Size() > maxRefBytes {
		return "", fmt.Errorf("%w: %s is %d bytes, limit is %d", ErrInput, path, st.Size(), maxRefBytes)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var (
	scriptStyle = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	tags        = regexp.MustCompile(`(?s)<[^>]+>`)
	blankLines  = regexp.MustCompile(`\n\s*\n+`)
)

// Fetch downloads a URL and reduces HTML to text.
func (r *Reader) Fetch(ctx context.Context, url string) (string, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("%w: url must start with http:// or https://", ErrInput)
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInput, err)
	}
	req.Header.Set("User-Agent", "ohmylaya")
	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tools: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tools: fetch %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxRefBytes))
	if err != nil {
		return "", err
	}
	text := string(b)
	if strings.Contains(resp.Header.Get("Content-Type"), "html") || strings.Contains(strings.ToLower(text[:min(len(text), 512)]), "<html") {
		text = HTMLToText(text)
	}
	return text, nil
}

// HTMLToText strips scripts, styles and tags and collapses whitespace.
func HTMLToText(html string) string {
	t := scriptStyle.ReplaceAllString(html, " ")
	t = tags.ReplaceAllString(t, "\n")
	t = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(t)
	lines := strings.Split(t, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	t = strings.Join(lines, "\n")
	return strings.TrimSpace(blankLines.ReplaceAllString(t, "\n"))
}

// ItemsFromGlob turns matching files into items keyed by path.
func (r *Reader) ItemsFromGlob(pattern string, limit int) ([]Item, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: glob %q: %v", ErrInput, pattern, err)
	}
	// Support ** by walking when the pattern contains it.
	if strings.Contains(pattern, "**") {
		matches, err = doubleStar(pattern)
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(matches)
	var items []Item
	for _, m := range matches {
		if st, err := os.Stat(m); err != nil || st.IsDir() || st.Size() > maxRefBytes {
			continue
		}
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		items = append(items, Item{ID: filepath.ToSlash(m), Text: string(b)})
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: glob %q matched no readable files", ErrInput, pattern)
	}
	return items, nil
}

// ItemsFromPaths reads the given files into items keyed by path.
func (r *Reader) ItemsFromPaths(paths []string) ([]Item, error) {
	items := make([]Item, 0, len(paths))
	for _, p := range paths {
		text, err := r.ReadFile(p)
		if err != nil {
			return nil, err
		}
		items = append(items, Item{ID: filepath.ToSlash(p), Text: text})
	}
	return items, nil
}

func doubleStar(pattern string) ([]string, error) {
	idx := strings.Index(pattern, "**")
	root := filepath.Clean(pattern[:idx])
	if root == "" || root == "." {
		root = "."
	}
	suffix := strings.TrimPrefix(pattern[idx+2:], "/")
	suffix = strings.TrimPrefix(suffix, string(filepath.Separator))
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if suffix == "" {
			out = append(out, p)
			return nil
		}
		if ok, _ := filepath.Match(suffix, filepath.Base(p)); ok {
			out = append(out, p)
		}
		return nil
	})
	return out, err
}

// Excerpt returns the first runes of text on one line.
func Excerpt(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	r := []rune(t)
	if len(r) > excerptRunes {
		return string(r[:excerptRunes]) + "…"
	}
	return t
}
