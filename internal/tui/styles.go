package tui

import "github.com/charmbracelet/lipgloss"

var (
	borderColor = lipgloss.Color("62")
	accent      = lipgloss.Color("212")
	dim         = lipgloss.Color("240")
	okColor     = lipgloss.Color("42")
	errColor    = lipgloss.Color("196")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			Align(lipgloss.Center)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(1, 4)

	okStyle  = lipgloss.NewStyle().Foreground(okColor)
	errStyle = lipgloss.NewStyle().Foreground(errColor)
	dimStyle = lipgloss.NewStyle().Foreground(dim)

	// rawFocusedStyle highlights the focused row on the Raw VCP screen.
	rawFocusedStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
)
