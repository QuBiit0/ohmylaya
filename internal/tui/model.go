// Package tui is the interactive front end. Every action maps to a
// subcommand; the TUI never has behaviour of its own.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/QuBiit0/ohmylaya/internal/agents"
	"github.com/QuBiit0/ohmylaya/internal/doctor"
)

type screen int

const (
	screenStatus screen = iota
	screenSetup
	screenAgents
	screenUpdate
	screenDoctor
	screenLogs
)

var screenNames = []string{"Status", "Setup", "Agents", "Update", "Doctor", "Logs"}

// Status is what the Status screen shows.
type Status struct {
	Version     string
	Home        string
	Backend     string
	Model       string
	Port        int
	Provider    string
	EngineOK    bool
	ModelOK     bool
	SidecarInfo string
	Agents      []agents.Status
}

// UpdateInfo is the result of an update check.
type UpdateInfo struct {
	Current   string
	Latest    string
	Available bool
	Err       error
}

// Services are the operations the TUI can trigger. Each one blocks and is
// run inside a tea.Cmd. They are injected so tests need no engine.
type Services struct {
	Status  func(ctx context.Context) Status
	Doctor  func(ctx context.Context, smoke bool) doctor.Report
	Logs    func() string
	Check   func(ctx context.Context) UpdateInfo
	Update  func(ctx context.Context, progress func(string)) error
	Setup   func(ctx context.Context, backend, model string, progress func(string)) error
	Toggle  func(ctx context.Context, agentID string, register bool) error
	Options func() (backends []string, models []string)
}

type (
	statusMsg   Status
	doctorMsg   doctor.Report
	logsMsg     string
	checkMsg    UpdateInfo
	progressMsg string
	doneMsg     struct{ err error }
)

// Model is the root Bubble Tea model.
type Model struct {
	svc      Services
	screen   screen
	width    int
	height   int
	status   Status
	report   *doctor.Report
	check    *UpdateInfo
	logs     viewport.Model
	spin     spinner.Model
	busy     bool
	progress []string
	err      error
	cursor   int
	setup    setupState
	quitting bool
}

type setupState struct {
	backends []string
	models   []string
	backend  int
	model    int
	field    int // 0 backend, 1 model, 2 apply
}

// New creates the root model.
func New(svc Services) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	m := Model{svc: svc, spin: sp, logs: viewport.New(80, 20)}
	if svc.Options != nil {
		m.setup.backends, m.setup.models = svc.Options()
	}
	return m
}

// Init loads the status.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadStatus(), m.spin.Tick)
}

func (m Model) loadStatus() tea.Cmd {
	return func() tea.Msg { return statusMsg(m.svc.Status(context.Background())) }
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.logs.Width, m.logs.Height = max(20, msg.Width-4), max(5, msg.Height-8)
		return m, nil
	case statusMsg:
		m.status = Status(msg)
		m.syncSetupFromStatus()
		return m, nil
	case doctorMsg:
		r := doctor.Report(msg)
		m.report, m.busy = &r, false
		return m, nil
	case logsMsg:
		m.logs.SetContent(string(msg))
		m.logs.GotoBottom()
		return m, nil
	case checkMsg:
		u := UpdateInfo(msg)
		m.check, m.busy = &u, false
		return m, nil
	case progressMsg:
		m.progress = append(m.progress, string(msg))
		return m, nil
	case doneMsg:
		m.busy, m.err = false, msg.err
		return m, m.loadStatus()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.busy && msg.String() != "ctrl+c" {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "tab", "right", "l":
		m.screen = screen((int(m.screen) + 1) % len(screenNames))
		return m.enter()
	case "shift+tab", "left", "h":
		m.screen = screen((int(m.screen) + len(screenNames) - 1) % len(screenNames))
		return m.enter()
	case "1", "2", "3", "4", "5", "6":
		m.screen = screen(int(msg.String()[0] - '1'))
		return m.enter()
	case "esc":
		m.screen = screenStatus
		return m, nil
	}
	switch m.screen {
	case screenSetup:
		return m.setupKey(msg)
	case screenAgents:
		return m.agentsKey(msg)
	case screenUpdate:
		if msg.String() == "enter" && m.check != nil && m.check.Available {
			return m.run(func(p func(string)) error { return m.svc.Update(context.Background(), p) })
		}
	case screenDoctor:
		if msg.String() == "s" {
			m.busy = true
			return m, func() tea.Msg { return doctorMsg(m.svc.Doctor(context.Background(), true)) }
		}
	case screenLogs:
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		return m, cmd
	}
	return m, nil
}

// enter runs the loader of the current screen.
func (m Model) enter() (tea.Model, tea.Cmd) {
	m.cursor, m.err, m.progress = 0, nil, nil
	switch m.screen {
	case screenStatus:
		return m, m.loadStatus()
	case screenDoctor:
		m.busy = true
		return m, func() tea.Msg { return doctorMsg(m.svc.Doctor(context.Background(), false)) }
	case screenLogs:
		return m, func() tea.Msg { return logsMsg(m.svc.Logs()) }
	case screenUpdate:
		m.busy = true
		return m, func() tea.Msg { return checkMsg(m.svc.Check(context.Background())) }
	}
	return m, nil
}

// run executes a long operation with progress lines.
func (m Model) run(fn func(progress func(string)) error) (tea.Model, tea.Cmd) {
	m.busy, m.progress, m.err = true, nil, nil
	return m, func() tea.Msg { return doneMsg{err: fn(func(string) {})} }
}

func (m Model) setupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.setup
	switch msg.String() {
	case "up", "k":
		s.field = max(0, s.field-1)
	case "down", "j":
		s.field = min(2, s.field+1)
	case " ", "n":
		switch s.field {
		case 0:
			if len(s.backends) > 0 {
				s.backend = (s.backend + 1) % len(s.backends)
			}
		case 1:
			if len(s.models) > 0 {
				s.model = (s.model + 1) % len(s.models)
			}
		}
	case "enter":
		if s.field == 2 && len(s.backends) > 0 && len(s.models) > 0 {
			b, mo := s.backends[s.backend], s.models[s.model]
			return m.run(func(p func(string)) error { return m.svc.Setup(context.Background(), b, mo, p) })
		}
		s.field = min(2, s.field+1)
	}
	return m, nil
}

func (m Model) agentsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.status.Agents)
	switch msg.String() {
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(max(0, n-1), m.cursor+1)
	case "enter", " ":
		if n == 0 {
			return m, nil
		}
		a := m.status.Agents[m.cursor]
		register := !a.Registered
		return m.run(func(p func(string)) error { return m.svc.Toggle(context.Background(), a.ID, register) })
	}
	return m, nil
}

func (m *Model) syncSetupFromStatus() {
	for i, b := range m.setup.backends {
		if b == m.status.Backend {
			m.setup.backend = i
		}
	}
	for i, mo := range m.setup.models {
		if mo == m.status.Model {
			m.setup.model = i
		}
	}
}

// View renders the current screen.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(styleTitle.Render("ohmylaya " + m.status.Version))
	b.WriteString("  ")
	for i, name := range screenNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if screen(i) == m.screen {
			b.WriteString(styleSelected.Render("[" + label + "]"))
		} else {
			b.WriteString(styleMuted.Render(" " + label + " "))
		}
	}
	b.WriteString("\n\n")
	switch m.screen {
	case screenStatus:
		b.WriteString(m.viewStatus())
	case screenSetup:
		b.WriteString(m.viewSetup())
	case screenAgents:
		b.WriteString(m.viewAgents())
	case screenUpdate:
		b.WriteString(m.viewUpdate())
	case screenDoctor:
		b.WriteString(m.viewDoctor())
	case screenLogs:
		b.WriteString(m.logs.View())
	}
	if m.busy {
		b.WriteString("\n" + m.spin.View() + " working...")
	}
	for _, p := range m.progress {
		b.WriteString("\n" + styleMuted.Render(p))
	}
	if m.err != nil {
		b.WriteString("\n" + styleFail.Render("error: "+m.err.Error()))
	}
	b.WriteString("\n\n" + styleMuted.Render("tab/1-6 switch  j/k move  enter select  q quit"))
	return b.String()
}

func yesNo(v bool) string {
	if v {
		return styleOK.Render("yes")
	}
	return styleFail.Render("no")
}

func (m Model) viewStatus() string {
	s := m.status
	rows := []string{
		fmt.Sprintf("Home      %s", s.Home),
		fmt.Sprintf("Backend   %s", s.Backend),
		fmt.Sprintf("Model     %s", s.Model),
		fmt.Sprintf("Provider  %s", s.Provider),
		fmt.Sprintf("Port      %d", s.Port),
		fmt.Sprintf("Engine    %s", yesNo(s.EngineOK)),
		fmt.Sprintf("Weights   %s", yesNo(s.ModelOK)),
		fmt.Sprintf("Sidecar   %s", s.SidecarInfo),
	}
	var ag []string
	for _, a := range s.Agents {
		state := "not registered"
		if a.Registered {
			state = statusStyle("yes").Render("registered")
		}
		if a.Stale {
			state = statusStyle("stale").Render("stale")
		}
		ag = append(ag, fmt.Sprintf("%-12s %s", a.Name, state))
	}
	return styleBox.Render(strings.Join(rows, "\n")) + "\n\n" + styleBox.Render(strings.Join(ag, "\n"))
}

func (m Model) viewSetup() string {
	s := m.setup
	line := func(i int, label, value string) string {
		prefix := "  "
		if s.field == i {
			prefix = styleSelected.Render("> ")
		}
		return prefix + fmt.Sprintf("%-8s %s", label, value)
	}
	backend, model := "(none)", "(none)"
	if len(s.backends) > 0 {
		backend = s.backends[s.backend]
	}
	if len(s.models) > 0 {
		model = s.models[s.model]
	}
	return strings.Join([]string{
		line(0, "Backend", backend+styleMuted.Render("  (space to cycle)")),
		line(1, "Model", model+styleMuted.Render("  (space to cycle)")),
		line(2, "Apply", styleMuted.Render("download, verify, smoke test")),
	}, "\n")
}

func (m Model) viewAgents() string {
	if len(m.status.Agents) == 0 {
		return styleMuted.Render("no agents known")
	}
	var rows []string
	for i, a := range m.status.Agents {
		prefix := "  "
		if i == m.cursor {
			prefix = styleSelected.Render("> ")
		}
		mark := "[ ]"
		if a.Registered {
			mark = "[x]"
		}
		det := styleMuted.Render("not detected")
		if a.Detected {
			det = styleOK.Render("detected")
		}
		rows = append(rows, fmt.Sprintf("%s%s %-12s %s  %s", prefix, mark, a.Name, det, styleMuted.Render(a.ConfigPath)))
	}
	return strings.Join(rows, "\n") + "\n\n" + styleMuted.Render("enter toggles registration and the skill")
}

func (m Model) viewUpdate() string {
	if m.check == nil {
		return styleMuted.Render("checking...")
	}
	c := m.check
	switch {
	case c.Err != nil:
		return styleWarn.Render("could not check: " + c.Err.Error())
	case c.Available:
		return fmt.Sprintf("%s installed, %s available\n\n%s", c.Current, styleOK.Render(c.Latest), "press enter to update")
	}
	return styleOK.Render(c.Current + " is the latest release")
}

func (m Model) viewDoctor() string {
	if m.report == nil {
		return styleMuted.Render("running checks...")
	}
	var rows []string
	for _, c := range m.report.Checks {
		rows = append(rows, fmt.Sprintf("%s %-16s %s", statusStyle(c.Status).Render(fmt.Sprintf("%-4s", c.Status)), c.ID, c.Detail))
		if c.Fix != "" && c.Status != doctor.Pass {
			rows = append(rows, styleMuted.Render("     fix: "+c.Fix))
		}
	}
	rows = append(rows, "", styleMuted.Render("s runs the smoke test"))
	return strings.Join(rows, "\n")
}
