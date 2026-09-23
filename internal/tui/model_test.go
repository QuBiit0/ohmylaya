package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/QuBiit0/ohmylaya/internal/agents"
	"github.com/QuBiit0/ohmylaya/internal/doctor"
)

type calls struct {
	toggled []string
	setup   []string
	updated int
}

func fakeServices(c *calls) Services {
	return Services{
		Status: func(context.Context) Status {
			return Status{Version: "1.0.0", Home: "/h", Backend: "vulkan", Model: "multilingual", Port: 45292, Provider: "local", EngineOK: true, ModelOK: true, SidecarInfo: "not running",
				Agents: []agents.Status{{ID: "claude", Name: "Claude Code", Detected: true, Registered: true, ConfigPath: "/c"}, {ID: "pi", Name: "Pi", Detected: true}}}
		},
		Doctor: func(_ context.Context, smoke bool) doctor.Report {
			r := doctor.Report{Checks: []doctor.Check{{ID: "engine", Status: doctor.Pass, Detail: "ok"}, {ID: "agents", Status: doctor.Warn, Detail: "none", Fix: "ohmylaya install"}}}
			if smoke {
				r.Checks = append(r.Checks, doctor.Check{ID: "smoke", Status: doctor.Pass, Detail: "160 ms"})
			}
			return r
		},
		Logs: func() string { return "line1\nline2" },
		Check: func(context.Context) UpdateInfo {
			return UpdateInfo{Current: "1.0.0", Latest: "1.1.0", Available: true}
		},
		Update: func(context.Context, func(string)) error { c.updated++; return nil },
		Setup: func(_ context.Context, b, m string, _ func(string)) error {
			c.setup = append(c.setup, b+"/"+m)
			return nil
		},
		Toggle: func(_ context.Context, id string, reg bool) error {
			c.toggled = append(c.toggled, id)
			if id == "pi" {
				return errors.New("no mcp.json")
			}
			return nil
		},
		Options: func() ([]string, []string) {
			return []string{"vulkan", "cuda", "cpu"}, []string{"multilingual", "english"}
		},
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// drive applies a key and runs any returned command synchronously.
func drive(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		var cmd tea.Cmd
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
		for cmd != nil {
			out := cmd()
			if out == nil {
				break
			}
			if batch, ok := out.(tea.BatchMsg); ok {
				cmd = nil
				for _, c := range batch {
					if c != nil {
						if r := c(); r != nil {
							next, _ = m.Update(r)
							m = next.(Model)
						}
					}
				}
				break
			}
			next, cmd = m.Update(out)
			m = next.(Model)
		}
	}
	return m
}

func loaded(t *testing.T, c *calls) Model {
	t.Helper()
	m := New(fakeServices(c))
	m = drive(m, statusMsg(m.svc.Status(context.Background())), tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func TestStatusScreenShowsConfigAndAgents(t *testing.T) {
	m := loaded(t, &calls{})
	v := m.View()
	for _, want := range []string{"ohmylaya 1.0.0", "vulkan", "multilingual", "45292", "Claude Code", "registered", "Pi"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestNavigationByTabAndNumbers(t *testing.T) {
	m := loaded(t, &calls{})
	m = drive(m, key("tab"))
	if m.screen != screenSetup {
		t.Errorf("tab -> %v", m.screen)
	}
	m = drive(m, key("5"))
	if m.screen != screenDoctor || m.report == nil {
		t.Errorf("5 -> %v, report loaded %v", m.screen, m.report != nil)
	}
	if !strings.Contains(m.View(), "fix: ohmylaya install") {
		t.Errorf("doctor view missing fix:\n%s", m.View())
	}
	m = drive(m, key("esc"))
	if m.screen != screenStatus {
		t.Error("esc must return to status")
	}
	m = drive(m, key("q"))
	if !m.quitting {
		t.Error("q must quit")
	}
}

func TestDoctorSmokeKey(t *testing.T) {
	m := loaded(t, &calls{})
	m = drive(m, key("5"), key("s"))
	if !strings.Contains(m.View(), "smoke") || m.busy {
		t.Errorf("smoke not run or still busy:\n%s", m.View())
	}
}

func TestAgentsToggleCallsServiceAndShowsError(t *testing.T) {
	c := &calls{}
	m := loaded(t, c)
	m = drive(m, key("3"), key("enter"))
	if len(c.toggled) != 1 || c.toggled[0] != "claude" {
		t.Errorf("toggled = %v", c.toggled)
	}
	m = drive(m, key("j"), key("enter"))
	if len(c.toggled) != 2 || c.toggled[1] != "pi" {
		t.Errorf("toggled = %v", c.toggled)
	}
	if m.err == nil || !strings.Contains(m.View(), "no mcp.json") {
		t.Errorf("service error must be shown:\n%s", m.View())
	}
}

func TestSetupCyclesAndApplies(t *testing.T) {
	c := &calls{}
	m := loaded(t, c)
	m = drive(m, key("2"))
	if m.setup.backend != 0 || m.setup.model != 0 {
		t.Errorf("setup must preselect the current config: %+v", m.setup)
	}
	m = drive(m, key(" "), key(" "), key("j"), key(" "), key("j"), key("enter"))
	if len(c.setup) != 1 || c.setup[0] != "cpu/english" {
		t.Errorf("setup calls = %v", c.setup)
	}
	if m.busy {
		t.Error("done message must clear busy")
	}
}

func TestUpdateScreenChecksThenApplies(t *testing.T) {
	c := &calls{}
	m := loaded(t, c)
	m = drive(m, key("4"))
	if !strings.Contains(m.View(), "1.1.0 available") {
		t.Errorf("update view:\n%s", m.View())
	}
	m = drive(m, key("enter"))
	if c.updated != 1 || m.busy {
		t.Errorf("updated = %d busy = %v", c.updated, m.busy)
	}
}

func TestLogsScreenLoadsTail(t *testing.T) {
	m := loaded(t, &calls{})
	m = drive(m, key("6"))
	if !strings.Contains(m.View(), "line2") {
		t.Errorf("logs view:\n%s", m.View())
	}
}

func TestKeysIgnoredWhileBusy(t *testing.T) {
	m := loaded(t, &calls{})
	m.busy = true
	next, _ := m.Update(key("2"))
	if next.(Model).screen != screenStatus {
		t.Error("navigation must be ignored while busy")
	}
}
