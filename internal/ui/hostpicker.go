package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

// hostPanelOpen 回傳右側面板是否展開（相容性方法）。
func (m Model) hostPanelOpen() bool { return m.hostFocusCol > 0 }

// HostPanelOpen 回傳右側面板是否開啟（供測試使用）。
func (m Model) HostPanelOpen() bool { return m.hostFocusCol > 0 }

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

// updateHostPicker 處理主機管理面板的按鍵。
func (m Model) updateHostPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// hub 模式：委派給 hub 專用 handler（支援完整編輯）
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

	// 任何按鍵清除「已儲存」提示（Ctrl+S 會重新設定）
	m.hostSavedMsg = ""

	// Ctrl+S 儲存（面板開啟或關閉皆可）
	if msg.String() == "ctrl+s" {
		m.applyHostDrafts()
		m.persistHostsWithSync()
		m.hostFocusCol = 0
		m.hostPanelEditing = false
		m.hostSavedMsg = "已儲存"
		return m, m.applyCurrentStatusBarCmd()
	}

	// 依焦點欄位分派按鍵
	switch m.hostFocusCol {
	case 0:
		return m.updateHostPickerLeft(msg, hosts)
	case 1:
		return m.updateHostConnection(msg)
	case 2:
		if m.hostPanelEditing {
			return m.updateHostPanelEditing(msg)
		}
		return m.updateHostPanelOpen(msg, hosts)
	default:
		return m, nil
	}
}

// updateHostPickerLeft 處理左側主機列表的按鍵。
func (m Model) updateHostPickerLeft(msg tea.KeyMsg, hosts []*hostmgr.Host) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "h", "q":
		m.mode = ModeNormal
		return m, nil
	case "enter", "right", "l":
		// 進入中欄（連線設定）
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			m.ensureDraft(h.ID())
			m.hostFocusCol = 1
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

// ApplyCurrentStatusBarCmd 回傳套用本機 status bar 的 tea.Cmd（exported for testing）。
func (m Model) ApplyCurrentStatusBarCmd() tea.Cmd { return m.applyCurrentStatusBarCmd() }

// applyCurrentStatusBarCmd 回傳套用本機 status bar 的 tea.Cmd。
func (m Model) applyCurrentStatusBarCmd() tea.Cmd {
	if m.deps.HostMgr != nil {
		for _, h := range m.deps.HostMgr.Hosts() {
			if h.IsLocal() {
				cc := h.Config().ToColorConfig()
				return func() tea.Msg {
					exec := tmux.NewRealExecutor()
					_ = tmux.ApplyStatusBar(exec, cc)
					return nil
				}
			}
		}
		return nil
	}
	// Hub-socket 模式：從 hubHosts 找 self host（HubSelf = spoke 的 host name）
	if m.isHubSocketMode() && m.deps.HubSelf != "" && len(m.hubHosts) > 0 {
		for _, h := range m.hubHosts {
			if h.Name == m.deps.HubSelf {
				cc := h.ToColorConfig()
				return func() tea.Msg {
					exec := tmux.NewRealExecutor()
					_ = tmux.ApplyStatusBar(exec, cc)
					return nil
				}
			}
		}
	}
	// Hub mode fallback：從 deps.Cfg.Hosts 找 local host
	for _, h := range m.deps.Cfg.Hosts {
		if h.IsLocal() {
			cc := h.ToColorConfig()
			return func() tea.Msg {
				exec := tmux.NewRealExecutor()
				_ = tmux.ApplyStatusBar(exec, cc)
				return nil
			}
		}
	}
	return nil
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
	if !m.hostPanelOpen() {
		return leftPanel
	}

	// 取得當前選中主機的資訊
	hostName := ""
	hostAddr := ""
	isLocal := true
	if m.hostPickerCursor < len(hosts) {
		h := hosts[m.hostPickerCursor]
		hostName = h.Config().Name
		hostAddr = h.Config().Address
		isLocal = h.Config().IsLocal()
	}

	// 三欄並排：左側列表 + 中欄連線 + 右側設定
	midPanel := m.renderHostConnection(hostName, hostAddr, isLocal)
	rightPanel := m.renderHostPickerRight(hosts)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, midPanel, rightPanel)
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
			if m.hostPanelOpen() {
				cursor = dimStyle.Render("► ")
			} else {
				cursor = selectedStyle.Render("► ")
			}
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
		if i == m.hostPickerCursor && !m.hostPanelOpen() {
			line = m.cursorLine(line)
		}
		b.WriteString(line + "\n")
	}

	if m.hostPanelOpen() {
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

// syncHubHostsToConfig 將 hubHostSnap 中尚未存在於 deps.Cfg.Hosts 的主機自動加入設定。
// 這確保 hub 模式下新連線的遠端主機能立即出現在主機管理面板中。
func (m *Model) syncHubHostsToConfig() {
	if m.hubHostSnap == nil {
		return
	}
	existing := make(map[string]bool, len(m.deps.Cfg.Hosts))
	for _, h := range m.deps.Cfg.Hosts {
		existing[h.Name] = true
	}
	changed := false
	for _, h := range m.hubHostSnap.Hosts {
		name := h.Name
		if name == "" {
			name = h.HostId
		}
		if existing[name] {
			continue
		}
		// 自動挑色
		color := h.Color
		if color == "" {
			color = config.PickColorForHosts(m.deps.Cfg.Hosts)
		}
		entry := config.HostEntry{
			Name:      name,
			Address:   h.Address,
			Color:     color,
			Enabled:   true,
			SortOrder: len(m.deps.Cfg.Hosts),
		}
		m.deps.Cfg.Hosts = append(m.deps.Cfg.Hosts, entry)
		existing[name] = true
		changed = true
	}
	if changed {
		m.persistHubHosts()
	}
}

// rebuildHubItems 從目前的 hubHostSnap 重建 session 列表（套用 host 過濾）。
func (m *Model) rebuildHubItems() {
	if m.hubHostSnap == nil {
		return
	}
	inputs := ConvertMultiHostSnapshot(m.hubHostSnap)
	if m.isHubSocketMode() {
		inputs = FilterActiveHosts(inputs, m.hubHosts)
	} else {
		inputs = FilterActiveHosts(inputs, m.deps.Cfg.Hosts)
	}
	m.items = FlattenMultiHost(inputs)
	if len(m.items) == 0 {
		m.cursor = 0
	} else if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
}

// visibleHubHosts 回傳 hub 模式下非封存的主機列表（來自 deps.Cfg.Hosts）。
func (m Model) visibleHubHosts() []config.HostEntry {
	hosts := m.deps.Cfg.Hosts
	if m.isHubSocketMode() {
		hosts = m.hubHosts
	}
	var result []config.HostEntry
	for _, h := range hosts {
		if !h.Archived {
			result = append(result, h)
		}
	}
	return result
}

// ensureDraftFromEntry 確保指定主機的 draft 存在，若不存在則從 HostEntry 初始化。
func (m *Model) ensureDraftFromEntry(entry config.HostEntry) {
	if m.hostPanelDraft == nil {
		m.hostPanelDraft = make(map[string]hostDraftEntry)
	}
	if _, ok := m.hostPanelDraft[entry.Name]; ok {
		return
	}
	m.hostPanelDraft[entry.Name] = hostDraftEntry{
		Enabled: entry.Enabled,
		BarBG:   entry.BarBG,
		BarFG:   entry.BarFG,
		BadgeBG: entry.BadgeBG,
		BadgeFG: entry.BadgeFG,
		Color:   entry.Color,
	}
}

// hubHostOriginalIndex 在 deps.Cfg.Hosts 中查找 name 的原始索引。
func (m Model) hubHostOriginalIndex(name string) int {
	hosts := m.deps.Cfg.Hosts
	if m.isHubSocketMode() {
		hosts = m.hubHosts
	}
	for i, h := range hosts {
		if h.Name == name {
			return i
		}
	}
	return -1
}

// updateHubHostPicker 處理 hub 模式下 host picker 的按鍵（完整編輯支援）。
func (m Model) updateHubHostPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	hosts := m.visibleHubHosts()
	if len(hosts) == 0 {
		m.mode = ModeNormal
		return m, nil
	}

	// 任何按鍵清除「已儲存」提示（Ctrl+S 會重新設定）
	m.hostSavedMsg = ""

	// Ctrl+S 儲存（面板開啟或關閉皆可）
	if msg.String() == "ctrl+s" {
		if m.isHubSocketMode() {
			drafts := make(map[string]hostDraftEntry, len(m.hostPanelDraft))
			for k, v := range m.hostPanelDraft {
				drafts[k] = v
			}
			c := m.deps.Client
			m.hostFocusCol = 0
			m.hostPanelEditing = false
			m.hostSavedMsg = "已儲存"
			return m, func() tea.Msg {
				var updated []config.HostEntry
				var firstErr error
				for hostName, draft := range drafts {
					result, err := c.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
						HostName: hostName,
						Action:   tsmv1.HostConfigAction_HOST_CONFIG_UPDATE_COLORS,
						BarBg:    draft.BarBG,
						BarFg:    draft.BarFG,
						BadgeBg:  draft.BadgeBG,
						BadgeFg:  draft.BadgeFG,
						Color:    draft.Color,
					})
					if err != nil {
						if firstErr == nil {
							firstErr = err
						}
					} else {
						updated = result
					}
				}
				return hubHostConfigUpdatedMsg{Hosts: updated, Err: firstErr}
			}
		}
		m.applyHubHostDrafts()
		m.persistHubHosts()
		m.rebuildHubItems()
		m.hostFocusCol = 0
		m.hostPanelEditing = false
		m.hostSavedMsg = "已儲存"
		return m, m.applyCurrentStatusBarCmd()
	}

	// 依焦點欄位分派按鍵
	switch m.hostFocusCol {
	case 1:
		return m.updateHubHostConnection(msg)
	case 2:
		if m.hostPanelEditing {
			return m.updateHubHostPanelEditing(msg)
		}
		return m.updateHubHostPanelOpen(msg, hosts)
	}

	// 左側主機列表（hostFocusCol == 0）
	switch msg.String() {
	case "esc", "h", "q":
		m.mode = ModeNormal
		return m, nil
	case "enter", "right", "l":
		// 進入中欄（連線設定）
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			m.ensureDraftFromEntry(h)
			m.hostFocusCol = 1
			m.hostPanelCursor = 0
			m.hostPanelEditing = false
			m.hostSavedMsg = ""
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
			if m.isHubSocketMode() {
				action := tsmv1.HostConfigAction_HOST_CONFIG_DISABLE
				if !h.Enabled {
					action = tsmv1.HostConfigAction_HOST_CONFIG_ENABLE
				}
				return m, func() tea.Msg {
					updated, err := m.deps.Client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
						HostName: h.Name,
						Action:   action,
					})
					return hubHostConfigUpdatedMsg{Hosts: updated, Err: err}
				}
			}
			origIdx := m.hubHostOriginalIndex(h.Name)
			if origIdx >= 0 {
				m.deps.Cfg.Hosts[origIdx].Enabled = !m.deps.Cfg.Hosts[origIdx].Enabled
				m.persistHubHosts()
				m.rebuildHubItems()
			}
		}
	case "J", "shift+down":
		// 下移
		if m.hostPickerCursor < len(hosts)-1 {
			if m.isHubSocketMode() {
				reordered := make([]string, len(hosts))
				for i, h := range hosts {
					reordered[i] = h.Name
				}
				cur := m.hostPickerCursor
				reordered[cur], reordered[cur+1] = reordered[cur+1], reordered[cur]
				m.hostPickerCursor++
				return m, func() tea.Msg {
					updated, err := m.deps.Client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
						Action:       tsmv1.HostConfigAction_HOST_CONFIG_REORDER,
						OrderedHosts: reordered,
					})
					return hubHostConfigUpdatedMsg{Hosts: updated, Err: err}
				}
			}
			curName := hosts[m.hostPickerCursor].Name
			nextName := hosts[m.hostPickerCursor+1].Name
			curIdx := m.hubHostOriginalIndex(curName)
			nextIdx := m.hubHostOriginalIndex(nextName)
			if curIdx >= 0 && nextIdx >= 0 {
				m.deps.Cfg.Hosts[curIdx], m.deps.Cfg.Hosts[nextIdx] = m.deps.Cfg.Hosts[nextIdx], m.deps.Cfg.Hosts[curIdx]
				m.hostPickerCursor++
				m.persistHubHosts()
			}
		}
	case "K", "shift+up":
		// 上移
		if m.hostPickerCursor > 0 {
			if m.isHubSocketMode() {
				reordered := make([]string, len(hosts))
				for i, h := range hosts {
					reordered[i] = h.Name
				}
				cur := m.hostPickerCursor
				reordered[cur], reordered[cur-1] = reordered[cur-1], reordered[cur]
				m.hostPickerCursor--
				return m, func() tea.Msg {
					updated, err := m.deps.Client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
						Action:       tsmv1.HostConfigAction_HOST_CONFIG_REORDER,
						OrderedHosts: reordered,
					})
					return hubHostConfigUpdatedMsg{Hosts: updated, Err: err}
				}
			}
			curName := hosts[m.hostPickerCursor].Name
			prevName := hosts[m.hostPickerCursor-1].Name
			curIdx := m.hubHostOriginalIndex(curName)
			prevIdx := m.hubHostOriginalIndex(prevName)
			if curIdx >= 0 && prevIdx >= 0 {
				m.deps.Cfg.Hosts[curIdx], m.deps.Cfg.Hosts[prevIdx] = m.deps.Cfg.Hosts[prevIdx], m.deps.Cfg.Hosts[curIdx]
				m.hostPickerCursor--
				m.persistHubHosts()
			}
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
				if m.isHubSocketMode() {
					return m, func() tea.Msg {
						updated, err := m.deps.Client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
							HostName: h.Name,
							Action:   tsmv1.HostConfigAction_HOST_CONFIG_ARCHIVE,
						})
						return hubHostConfigUpdatedMsg{Hosts: updated, Err: err}
					}
				}
				origIdx := m.hubHostOriginalIndex(h.Name)
				if origIdx >= 0 {
					m.deps.Cfg.Hosts[origIdx].Archived = true
					m.deps.Cfg.Hosts[origIdx].Enabled = false
					m.persistHubHosts()
					m.rebuildHubItems()
					// 封存後可見列表縮短，收緊游標
					newVisible := m.visibleHubHosts()
					if m.hostPickerCursor >= len(newVisible) && len(newVisible) > 0 {
						m.hostPickerCursor = len(newVisible) - 1
					}
				}
			}
		}
		return m, nil
	}
	return m, nil
}

// hubHostStatus 從 hubHostSnap 取得指定主機的即時連線狀態。
func (m Model) hubHostStatus(name string) (tsmv1.HostStatus, string) {
	if m.hubHostSnap == nil {
		return tsmv1.HostStatus_HOST_STATUS_DISABLED, ""
	}
	for _, h := range m.hubHostSnap.Hosts {
		if h.Name == name || h.HostId == name {
			return h.Status, h.Error
		}
	}
	return tsmv1.HostStatus_HOST_STATUS_DISABLED, ""
}

// renderHubHostPicker 渲染 hub 模式的主機管理面板（含編輯功能）。
func (m Model) renderHubHostPicker() string {
	hosts := m.visibleHubHosts()

	leftPanel := m.renderHubHostPickerLeft(hosts)

	if !m.hostPanelOpen() {
		return leftPanel
	}

	// 取得當前選中主機的資訊
	hostName := ""
	hostAddr := ""
	isLocal := true
	if m.hostPickerCursor < len(hosts) {
		hostName = hosts[m.hostPickerCursor].Name
		hostAddr = hosts[m.hostPickerCursor].Address
		isLocal = hosts[m.hostPickerCursor].IsLocal()
	}

	// 三欄並排：左側列表 + 中欄連線 + 右側設定
	midPanel := m.renderHostConnection(hostName, hostAddr, isLocal)
	rightPanel := m.renderHubHostPickerRight(hosts)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, midPanel, rightPanel)
}

// renderHubHostPickerLeft 渲染 hub 模式左側主機列表。
func (m Model) renderHubHostPickerLeft(hosts []config.HostEntry) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n", selectedStyle.Render("主機管理")))

	for i, h := range hosts {
		cursor := "  "
		if i == m.hostPickerCursor {
			if m.hostPanelOpen() {
				cursor = dimStyle.Render("► ")
			} else {
				cursor = selectedStyle.Render("► ")
			}
		}

		// 從 hubHostSnap 取得即時狀態，合併 config 的 Enabled
		liveStatus, liveErr := m.hubHostStatus(h.Name)

		// 啟用狀態指示
		status := dimStyle.Render("○")
		if h.Enabled {
			color := h.Color
			if color == "" {
				color = "#5B9BD5"
			}
			if liveStatus == tsmv1.HostStatus_HOST_STATUS_CONNECTED {
				status = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("●")
			} else if liveStatus != tsmv1.HostStatus_HOST_STATUS_DISABLED {
				status = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("◐")
			}
		}

		name := h.Name
		if h.Address != "" && h.Address != h.Name {
			name += dimStyle.Render(" (" + h.Address + ")")
		}

		// 連線狀態
		var stateStr string
		if !h.Enabled {
			stateStr = dimStyle.Render(" 已停用")
		} else {
			switch liveStatus {
			case tsmv1.HostStatus_HOST_STATUS_CONNECTING:
				stateStr = dimStyle.Render(" 連線中...")
			case tsmv1.HostStatus_HOST_STATUS_DISCONNECTED:
				errMsg := " 連線中斷，重連中..."
				if liveErr != "" {
					errMsg = fmt.Sprintf(" 連線中斷: %s", liveErr)
				}
				stateStr = statusErrorStyle.Render(errMsg)
			case tsmv1.HostStatus_HOST_STATUS_DISABLED:
				stateStr = dimStyle.Render(" 已停用")
			case tsmv1.HostStatus_HOST_STATUS_CONNECTED:
				stateStr = dimStyle.Render(" ✓")
			}
		}

		line := fmt.Sprintf("  %s%s %s%s", cursor, status, name, stateStr)
		if i == m.hostPickerCursor && !m.hostPanelOpen() {
			line = m.cursorLine(line)
		}
		b.WriteString(line + "\n")
	}

	if m.hostPanelOpen() {
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
