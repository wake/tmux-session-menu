package ui

import "github.com/charmbracelet/lipgloss"

// Claude Code dark mode 配色
var (
	headerStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D77757")) // claude terracotta
	selectedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#B1B9F9")).Bold(true) // periwinkle（用於 input prompt）
	dimStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))            // dim text
	statusRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#91E200"))            // running green
	statusWaitingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E8D969"))            // waiting yellow
	statusIdleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))            // idle gray
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80"))            // error red
	sessionNameStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))            // bright white
	groupNameStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1E174"))            // group name: warm yellow
	keyStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("#D77757")).Bold(true) // shortcut key: terracotta bold
	versionStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#C1C1C1"))            // version/hints
	summaryStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#ABB5F7"))            // AI summary
	successStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#91E200"))            // success green
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B80"))            // error red
	cursorBgStyle      = lipgloss.NewStyle().Background(lipgloss.Color("#373737"))            // cursor row background
	activeTabStyle     = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#1a1b26")).
				Background(lipgloss.Color("#7aa2f7")).
				Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#787fa0")).
				Background(lipgloss.Color("#24283b")).
				Padding(0, 1)
	previewBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), true, false, false, false).
				BorderForeground(lipgloss.Color("#505050")) // subtle
)

// VisibleWidth 回傳字串的可見寬度（排除 ANSI 跳脫碼）。
func VisibleWidth(s string) int {
	return lipgloss.Width(s)
}
