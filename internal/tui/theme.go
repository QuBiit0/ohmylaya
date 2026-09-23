package tui

import "github.com/charmbracelet/lipgloss"

// Theme colours adapt to light and dark terminals.
var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#7A2E8E", Dark: "#D7A0F0"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#6B6B6B", Dark: "#9A9A9A"}
	colorOK     = lipgloss.AdaptiveColor{Light: "#1E7F3A", Dark: "#7BE495"}
	colorWarn   = lipgloss.AdaptiveColor{Light: "#9A6A00", Dark: "#F2C86B"}
	colorFail   = lipgloss.AdaptiveColor{Light: "#A8231F", Dark: "#F28B82"}

	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleOK       = lipgloss.NewStyle().Foreground(colorOK)
	styleWarn     = lipgloss.NewStyle().Foreground(colorWarn)
	styleFail     = lipgloss.NewStyle().Foreground(colorFail)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorMuted).Padding(0, 1)
)

func statusStyle(s string) lipgloss.Style {
	switch s {
	case "PASS", "yes", "ok":
		return styleOK
	case "WARN", "stale":
		return styleWarn
	case "FAIL", "error":
		return styleFail
	}
	return styleMuted
}
