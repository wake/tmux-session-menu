package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
)

// hostDraftEntry 儲存單一主機的暫存編輯資料。
type hostDraftEntry struct {
	Enabled bool
	BarBG   string
	BarFG   string
	BadgeBG string
	BadgeFG string
}

// hostPanelFieldCount 是右側面板的欄位數量（0=啟用, 1=bar_bg, 2=bar_fg, 3=badge_bg, 4=badge_fg）。
const hostPanelFieldCount = 5

// hostPanelFieldLabels 對應各欄位的標籤。
var hostPanelFieldLabels = [hostPanelFieldCount]string{
	"啟用",
	"bar_bg",
	"bar_fg",
	"badge_bg",
	"badge_fg",
}

// HostPanelOpen 回傳右側面板是否開啟（供測試使用）。
func (m Model) HostPanelOpen() bool { return m.hostPanelOpen }

// HostPanelEditing 回傳右側面板是否正在編輯色彩欄位（供測試使用）。
func (m Model) HostPanelEditing() bool { return m.hostPanelEditing }

// HostPanelCursor 回傳右側面板游標位置（供測試使用）。
func (m Model) HostPanelCursor() int { return m.hostPanelCursor }

// visibleHosts 回傳非封存的主機列表（管理畫面用）。
func (m Model) visibleHosts() []*hostmgr.Host {
	if m.deps.HostMgr == nil {
		return nil
	}
	var result []*hostmgr.Host
	for _, h := range m.deps.HostMgr.Hosts() {
		if !h.Config().Archived {
			result = append(result, h)
		}
	}
	return result
}

// selectedHostForPanel 回傳目前游標所在的主機（面板用）。
func (m Model) selectedHostForPanel() *hostmgr.Host {
	hosts := m.visibleHosts()
	if m.hostPickerCursor < len(hosts) {
		return hosts[m.hostPickerCursor]
	}
	return nil
}

// ensureDraft 確保指定主機的 draft 存在，若不存在則從主機設定初始化。
func (m *Model) ensureDraft(hostID string) {
	if m.hostPanelDraft == nil {
		m.hostPanelDraft = make(map[string]hostDraftEntry)
	}
	if _, ok := m.hostPanelDraft[hostID]; ok {
		return
	}
	h := m.deps.HostMgr.Host(hostID)
	if h == nil {
		return
	}
	cfg := h.Config()
	m.hostPanelDraft[hostID] = hostDraftEntry{
		Enabled: cfg.Enabled,
		BarBG:   cfg.BarBG,
		BarFG:   cfg.BarFG,
		BadgeBG: cfg.BadgeBG,
		BadgeFG: cfg.BadgeFG,
	}
}

// draftFieldValue 回傳 draft 中指定欄位的值。
func draftFieldValue(d hostDraftEntry, field int) string {
	switch field {
	case 1:
		return d.BarBG
	case 2:
		return d.BarFG
	case 3:
		return d.BadgeBG
	case 4:
		return d.BadgeFG
	default:
		return ""
	}
}

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
	hosts := m.visibleHosts()
	if len(hosts) == 0 {
		m.mode = ModeNormal
		return m, nil
	}

	// 右側面板開啟時
	if m.hostPanelOpen {
		return m.updateHostPanelOpen(msg, hosts)
	}

	// 左側（主機列表）
	return m.updateHostPickerLeft(msg, hosts)
}

// updateHostPickerLeft 處理左側主機列表的按鍵。
func (m Model) updateHostPickerLeft(msg tea.KeyMsg, hosts []*hostmgr.Host) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "h", "q":
		m.mode = ModeNormal
		return m, nil
	case "enter", "right", "l":
		// 開啟右側面板
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			m.ensureDraft(h.ID())
			m.hostPanelOpen = true
			m.hostPanelCursor = 0
			m.hostPanelEditing = false
			m.hostSavedMsg = "" // 清除舊的 flash
		}
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
	case "n":
		// 新增主機：進入文字輸入模式
		m.mode = ModeInput
		m.inputTarget = InputNewHost
		m.inputPrompt = "主機位址"
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, nil
	case "d":
		// 封存主機（軟刪除）— local 不可封存
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			if !h.IsLocal() {
				_ = m.deps.HostMgr.Disable(h.ID())
				m.deps.HostMgr.SetArchived(h.ID(), true)
				m.persistHosts()
				// 封存後可見列表縮短，收緊游標
				newVisible := m.visibleHosts()
				if m.hostPickerCursor >= len(newVisible) && len(newVisible) > 0 {
					m.hostPickerCursor = len(newVisible) - 1
				}
			}
		}
		return m, nil
	}
	return m, nil
}

// updateHostPanelOpen 處理右側面板開啟時的按鍵。
func (m Model) updateHostPanelOpen(msg tea.KeyMsg, hosts []*hostmgr.Host) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "left", "h":
		// 關閉面板（不儲存）
		m.hostPanelOpen = false
		return m, nil
	case "j", "down":
		if m.hostPanelCursor < hostPanelFieldCount-1 {
			m.hostPanelCursor++
		}
	case "k", "up":
		if m.hostPanelCursor > 0 {
			m.hostPanelCursor--
		}
	case " ":
		if m.hostPanelCursor == 0 {
			m.toggleHostPanelEnabled(hosts)
		}
		return m, nil
	}
	return m, nil
}

// toggleHostPanelEnabled 切換面板中「啟用」的 draft 值。
func (m *Model) toggleHostPanelEnabled(hosts []*hostmgr.Host) {
	if m.hostPickerCursor >= len(hosts) {
		return
	}
	h := hosts[m.hostPickerCursor]
	m.ensureDraft(h.ID())
	d := m.hostPanelDraft[h.ID()]
	d.Enabled = !d.Enabled
	m.hostPanelDraft[h.ID()] = d
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
	hosts := m.visibleHosts()

	// 渲染左側主機列表
	leftPanel := m.renderHostPickerLeft(hosts)

	// 若面板未開啟，直接回傳左側
	if !m.hostPanelOpen {
		return leftPanel
	}

	// 渲染右側設定面板
	rightPanel := m.renderHostPickerRight(hosts)

	// 左右並排
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

// renderHostPickerLeft 渲染左側主機列表。
func (m Model) renderHostPickerLeft(hosts []*hostmgr.Host) string {
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
		if i == m.hostPickerCursor && !m.hostPanelOpen {
			line = m.cursorLine(line)
		}
		b.WriteString(line + "\n")
	}

	if m.hostPanelOpen {
		b.WriteString(fmt.Sprintf("\n  %s\n",
			dimStyle.Render("[Enter/→] 設定  [Ctrl+S] 儲存  [esc/h] 關閉")))
	} else {
		b.WriteString(fmt.Sprintf("\n  %s\n",
			dimStyle.Render("[Enter/→] 設定  [space] 啟用/停用  [n] 新增  [d] 封存  [⇧+↑/⇧+↓] 排序  [esc/h] 關閉")))
	}

	// 顯示 flash message
	if m.hostSavedMsg != "" {
		b.WriteString(fmt.Sprintf("  %s\n", successStyle.Render(m.hostSavedMsg)))
	}

	return b.String()
}

// renderHostPickerRight 渲染右側設定面板。
func (m Model) renderHostPickerRight(hosts []*hostmgr.Host) string {
	if m.hostPickerCursor >= len(hosts) {
		return ""
	}
	h := hosts[m.hostPickerCursor]
	hostID := h.ID()
	draft, ok := m.hostPanelDraft[hostID]
	if !ok {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n", selectedStyle.Render(h.Config().Name+" 設定")))

	// 五個欄位
	for i := 0; i < hostPanelFieldCount; i++ {
		cursor := "  "
		if i == m.hostPanelCursor {
			cursor = selectedStyle.Render("► ")
		}

		var line string
		if i == 0 {
			// 啟用 toggle
			check := " "
			if draft.Enabled {
				check = "x"
			}
			line = fmt.Sprintf("  %s[%s] %s", cursor, check, hostPanelFieldLabels[i])
		} else {
			// 色彩欄位
			val := draftFieldValue(draft, i)
			displayVal := val
			if displayVal == "" {
				displayVal = dimStyle.Render("（空）")
			}
			line = fmt.Sprintf("  %s%-10s %s", cursor, hostPanelFieldLabels[i], displayVal)
			// bar_fg 空值提示
			if i == 2 && val == "" {
				line += " " + dimStyle.Render("（留空由 tmux 自行決定）")
			}
		}

		if i == m.hostPanelCursor {
			line = m.cursorLine(line)
		}
		b.WriteString(line + "\n")
	}

	return b.String()
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
