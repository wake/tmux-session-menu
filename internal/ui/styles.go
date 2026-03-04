package ui

import "github.com/charmbracelet/lipgloss"

// Claude Code dark mode 配色
var (
	headerStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D77757"))  // claude terracotta
	selectedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#B1B9F9")).Bold(true)   // periwinkle
	dimStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))              // inactive
	subtleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#505050"))              // subtle
	statusRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4EBA65"))              // success green
	statusWaitingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC107"))              // warning yellow
	statusIdleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))              // inactive（可見）
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80"))              // error red
	sessionNameStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))              // bright white
	groupNameStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)    // group header: white bold
	keyStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("#D77757"))              // shortcut key: terracotta
	previewBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), true, false, false, false).
				BorderForeground(lipgloss.Color("#505050")) // subtle
)
