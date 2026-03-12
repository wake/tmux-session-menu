package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

// updateHostPicker 處理主機管理面板的按鍵。
func (m Model) updateHostPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// hub 模式：唯讀 host picker
	if m.deps.HubMode && m.hubHostSnap != nil {
		return m.updateHubHostPicker(msg)
	}

	if m.deps.HostMgr == nil {
		m.mode = ModeNormal
		return m, nil
	}
	hosts := m.deps.HostMgr.Hosts()
	if len(hosts) == 0 {
		m.mode = ModeNormal
		return m, nil
	}

	switch msg.String() {
	case "esc", "h", "q":
		m.mode = ModeNormal
		return m, nil
	case "j", "down":
		if m.hostPickerCursor < len(hosts)-1 {
			m.hostPickerCursor++
		}
	case "k", "up":
		if m.hostPickerCursor > 0 {
			m.hostPickerCursor--
		}
	case " ":
		// 切換啟用/停用
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			if h.Config().Enabled {
				_ = m.deps.HostMgr.Disable(h.ID())
			} else {
				_ = m.deps.HostMgr.Enable(context.Background(), h.ID())
			}
			m.persistHosts()
		}
	case "J", "shift+down":
		// 下移
		if m.hostPickerCursor < len(hosts)-1 {
			ids := make([]string, len(hosts))
			for i, h := range hosts {
				ids[i] = h.ID()
			}
			ids[m.hostPickerCursor], ids[m.hostPickerCursor+1] = ids[m.hostPickerCursor+1], ids[m.hostPickerCursor]
			m.deps.HostMgr.Reorder(ids)
			m.hostPickerCursor++
			m.persistHosts()
		}
	case "K", "shift+up":
		// 上移
		if m.hostPickerCursor > 0 {
			ids := make([]string, len(hosts))
			for i, h := range hosts {
				ids[i] = h.ID()
			}
			ids[m.hostPickerCursor], ids[m.hostPickerCursor-1] = ids[m.hostPickerCursor-1], ids[m.hostPickerCursor]
			m.deps.HostMgr.Reorder(ids)
			m.hostPickerCursor--
			m.persistHosts()
		}
	case "a":
		// 新增主機：進入文字輸入模式
		m.mode = ModeInput
		m.inputTarget = InputNewHost
		m.inputPrompt = "主機位址"
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, nil
	case "d":
		// 刪除主機（local 不可刪除）
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			if !h.IsLocal() {
				hostID := h.ID()
				m.mode = ModeConfirm
				m.confirmReturnMode = ModeHostPicker
				m.confirmPrompt = fmt.Sprintf("確定要刪除主機 %q？", hostID)
				m.confirmAction = func() tea.Cmd {
					_ = m.deps.HostMgr.Disable(hostID)
					m.deps.HostMgr.Remove(hostID)
					m.persistHosts()
					return nil
				}
			}
		}
		return m, nil
	}
	return m, nil
}

// updateHubHostPicker 處理 hub 模式下唯讀 host picker 的按鍵。
func (m Model) updateHubHostPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	hostCount := len(m.hubHostSnap.Hosts)
	if hostCount == 0 {
		m.mode = ModeNormal
		return m, nil
	}

	switch msg.String() {
	case "esc", "h", "q":
		m.mode = ModeNormal
		return m, nil
	case "j", "down":
		if m.hostPickerCursor < hostCount-1 {
			m.hostPickerCursor++
		}
	case "k", "up":
		if m.hostPickerCursor > 0 {
			m.hostPickerCursor--
		}
	}
	return m, nil
}

// renderHostPicker 渲染主機管理浮動面板。
func (m Model) renderHostPicker() string {
	// hub 模式：從 hubHostSnap 渲染
	if m.deps.HubMode && m.hubHostSnap != nil {
		return m.renderHubHostPicker()
	}

	if m.deps.HostMgr == nil {
		return ""
	}
	hosts := m.deps.HostMgr.Hosts()

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n", selectedStyle.Render("主機管理")))

	for i, h := range hosts {
		cfg := h.Config()
		cursor := "  "
		if i == m.hostPickerCursor {
			cursor = selectedStyle.Render("► ")
		}

		// 啟用狀態指示
		status := dimStyle.Render("○")
		if cfg.Enabled {
			status = lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Color)).Render("●")
		}

		// 主機名稱
		name := cfg.Name
		if cfg.Address != "" && cfg.Address != cfg.Name {
			name += dimStyle.Render(" (" + cfg.Address + ")")
		}

		// 連線狀態
		var stateStr string
		hostStatus := int(h.Status())
		switch hostStatus {
		case HostStateConnecting:
			stateStr = dimStyle.Render(" 連線中...")
		case HostStateDisconnected:
			stateStr = statusErrorStyle.Render(" 連線中斷，重連中...")
		case HostStateDisabled:
			stateStr = dimStyle.Render(" 已停用")
		case HostStateConnected:
			stateStr = dimStyle.Render(" ✓")
		}

		line := fmt.Sprintf("  %s%s %s%s", cursor, status, name, stateStr)
		if i == m.hostPickerCursor {
			line = m.cursorLine(line)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		dimStyle.Render("[space] 啟用/停用  [a] 新增  [d] 刪除  [⇧+↑/⇧+↓] 排序  [esc/h] 關閉")))

	return b.String()
}

// renderHubHostPicker 渲染 hub 模式的唯讀主機狀態面板。
func (m Model) renderHubHostPicker() string {
	hosts := m.hubHostSnap.Hosts

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n", selectedStyle.Render("主機管理")))

	for i, h := range hosts {
		cursor := "  "
		if i == m.hostPickerCursor {
			cursor = selectedStyle.Render("► ")
		}

		// 狀態指示
		status := dimStyle.Render("○")
		if h.Status == tsmv1.HostStatus_HOST_STATUS_CONNECTED {
			color := h.Color
			if color == "" {
				color = "#5B9BD5"
			}
			status = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("●")
		} else if h.Status != tsmv1.HostStatus_HOST_STATUS_DISABLED {
			color := h.Color
			if color == "" {
				color = "#5B9BD5"
			}
			status = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("◐")
		}

		name := h.Name

		// 連線狀態
		var stateStr string
		switch h.Status {
		case tsmv1.HostStatus_HOST_STATUS_CONNECTING:
			stateStr = dimStyle.Render(" 連線中...")
		case tsmv1.HostStatus_HOST_STATUS_DISCONNECTED:
			errMsg := " 連線中斷，重連中..."
			if h.Error != "" {
				errMsg = fmt.Sprintf(" 連線中斷: %s", h.Error)
			}
			stateStr = statusErrorStyle.Render(errMsg)
		case tsmv1.HostStatus_HOST_STATUS_DISABLED:
			stateStr = dimStyle.Render(" 已停用")
		case tsmv1.HostStatus_HOST_STATUS_CONNECTED:
			stateStr = dimStyle.Render(" ✓")
		}

		line := fmt.Sprintf("  %s%s %s%s", cursor, status, name, stateStr)
		if i == m.hostPickerCursor {
			line = m.cursorLine(line)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		dimStyle.Render("[esc/h] 關閉")))

	return b.String()
}
