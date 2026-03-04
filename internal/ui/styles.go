package ui

import "github.com/charmbracelet/lipgloss"

// Claude Code dark mode 配色
var (
	headerStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D77757"))  // claude terracotta
	selectedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#B1B9F9")).Bold(true)   // periwinkle
	dimStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))              // inactive
	statusRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4EBA65"))              // success green
	statusWaitingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC107"))              // warning yellow
	statusIdleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#505050"))              // subtle
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80"))              // error red
	sessionNameStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))              // bright white
	previewBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), true, false, false, false).
				BorderForeground(lipgloss.Color("#505050")) // subtle
)
