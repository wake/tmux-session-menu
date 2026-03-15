package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// updateHostConnection 處理中欄（連線設定）的按鍵。
func (m Model) updateHostConnection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.hostFocusCol = 0
		return m, nil
	case "left", "h":
		m.hostFocusCol = 0
		return m, nil
	case "right", "l":
		m.hostFocusCol = 2
		m.hostPanelCursor = 0
		m.hostPanelEditing = false
		return m, nil
	}
	return m, nil
}

// updateHubHostConnection 處理 hub 模式中欄的按鍵。
func (m Model) updateHubHostConnection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.hostFocusCol = 0
		return m, nil
	case "left", "h":
		m.hostFocusCol = 0
		return m, nil
	case "right", "l":
		m.hostFocusCol = 2
		m.hostPanelCursor = 0
		m.hostPanelEditing = false
		return m, nil
	}
	return m, nil
}

// renderHostConnection 渲染中欄（連線設定）。
func (m Model) renderHostConnection(hostName string, isLocal bool) string {
	var b strings.Builder
	title := fmt.Sprintf("── %s 連線 ──", hostName)
	b.WriteString("\n")
	if m.hostFocusCol == 1 {
		b.WriteString("  " + selectedStyle.Render(title) + "\n")
	} else {
		b.WriteString("  " + dimStyle.Render(title) + "\n")
	}
	b.WriteString("\n")
	if isLocal {
		b.WriteString("  " + dimStyle.Render("tmux:") + "  （待實作）\n")
	} else {
		b.WriteString("  " + dimStyle.Render("SSH:") + "     （待實作）\n")
		b.WriteString("  " + dimStyle.Render("tmux:") + "    （待實作）\n")
		b.WriteString("  " + dimStyle.Render("tunnel:") + "  （待實作）\n")
		b.WriteString("  " + dimStyle.Render("hub-sock:") + "（待實作）\n")
	}
	b.WriteString("\n")
	return b.String()
}
