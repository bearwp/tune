package main

import (
	"github.com/bear/charmkit"
	"github.com/charmbracelet/lipgloss"
)

// Palette aliases from the shared charmkit scheme (terminal 0-15, so it
// inherits the active Omarchy/pywal theme).
var (
	accent = charmkit.Colors.Accent
	muted  = charmkit.Colors.Muted
	subtle = charmkit.Colors.Subtle
	fg     = charmkit.Colors.Text
)

var (
	accentStyle = lipgloss.NewStyle().Foreground(accent)
	mutedStyle  = lipgloss.NewStyle().Foreground(muted)
	subtleStyle = lipgloss.NewStyle().Foreground(subtle)
	titleStyle  = lipgloss.NewStyle().Foreground(fg).Bold(true)
	artistStyle = lipgloss.NewStyle().Foreground(muted)
	box         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(subtle).Padding(0, 1)
)