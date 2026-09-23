package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/QuBiit0/ohmylaya/internal/install"
)

// linePrompter is a plain-text prompter for terminals without the TUI.
type linePrompter struct {
	in  *bufio.Reader
	out io.Writer
}

func (p *linePrompter) Select(title string, choices []install.Choice) (string, error) {
	fmt.Fprintf(p.out, "\n%s\n", title)
	def := 1
	for i, c := range choices {
		mark := " "
		if c.Selected {
			mark = "*"
			def = i + 1
		}
		fmt.Fprintf(p.out, " %s %d) %s\n      %s\n", mark, i+1, c.Label, c.Description)
	}
	for {
		fmt.Fprintf(p.out, "Choose [%d]: ", def)
		line, err := p.readLine()
		if err != nil {
			return "", err
		}
		if line == "" {
			return choices[def-1].Value, nil
		}
		n, err := strconv.Atoi(line)
		if err == nil && n >= 1 && n <= len(choices) {
			return choices[n-1].Value, nil
		}
		fmt.Fprintln(p.out, "Enter a number from the list.")
	}
}

func (p *linePrompter) MultiSelect(title string, choices []install.Choice) ([]string, error) {
	fmt.Fprintf(p.out, "\n%s\n", title)
	var defaults []string
	for i, c := range choices {
		mark := " "
		if c.Selected {
			mark = "*"
			defaults = append(defaults, strconv.Itoa(i+1))
		}
		fmt.Fprintf(p.out, " %s %d) %s\n      %s\n", mark, i+1, c.Label, c.Description)
	}
	for {
		fmt.Fprintf(p.out, "Choose numbers separated by commas, or 'none' [%s]: ", strings.Join(defaults, ","))
		line, err := p.readLine()
		if err != nil {
			return nil, err
		}
		if line == "" {
			line = strings.Join(defaults, ",")
		}
		if line == "none" || line == "" {
			return nil, nil
		}
		var out []string
		ok := true
		for _, part := range strings.Split(line, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 1 || n > len(choices) {
				ok = false
				break
			}
			out = append(out, choices[n-1].Value)
		}
		if ok {
			return out, nil
		}
		fmt.Fprintln(p.out, "Enter numbers from the list.")
	}
}

func (p *linePrompter) Confirm(title string) (bool, error) {
	fmt.Fprintf(p.out, "%s [Y/n]: ", title)
	line, err := p.readLine()
	if err != nil {
		return false, err
	}
	line = strings.ToLower(line)
	return line == "" || line == "y" || line == "yes", nil
}

func (p *linePrompter) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
