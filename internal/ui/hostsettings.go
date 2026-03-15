package ui

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
)

// hostDraftEntry 儲存單一主機的暫存編輯資料。
type hostDraftEntry struct {
	Enabled bool
	BarBG   string
	BarFG   string
	BadgeBG string
	BadgeFG string
	Color   string
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

// hexColorRe 驗證 #rrggbb 格式。
var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// isValidHexColor 回傳 s 是否為空或合法 #rrggbb。
func isValidHexColor(s string) bool {
	return s == "" || hexColorRe.MatchString(s)
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

// setDraftField 設定 draft 中指定欄位的值。
func setDraftField(d *hostDraftEntry, field int, val string) {
	switch field {
	case 1:
		d.BarBG = val
	case 2:
		d.BarFG = val
	case 3:
		d.BadgeBG = val
	case 4:
		d.BadgeFG = val
	}
}

// updateHostPanelOpen 處理右側面板開啟時的按鍵。
func (m Model) updateHostPanelOpen(msg tea.KeyMsg, hosts []*hostmgr.Host) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Esc 收合所有面板
		m.hostFocusCol = 0
		return m, nil
	case "left", "h":
		// 回到中欄
		m.hostFocusCol = 1
		return m, nil
	case "j", "down":
		if m.hostPanelCursor < hostPanelFieldCount-1 {
			m.hostPanelCursor++
		}
	case "k", "up":
		if m.hostPanelCursor > 0 {
			m.hostPanelCursor--
		}
	case "enter":
		if m.hostPanelCursor == 0 {
			// 啟用 toggle
			m.toggleHostPanelEnabled(hosts)
		} else {
			// 進入色彩欄位編輯
			m.enterHostPanelEdit(hosts)
		}
		return m, nil
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

// enterHostPanelEdit 進入色彩欄位編輯模式。
func (m *Model) enterHostPanelEdit(hosts []*hostmgr.Host) {
	if m.hostPickerCursor >= len(hosts) {
		return
	}
	h := hosts[m.hostPickerCursor]
	m.ensureDraft(h.ID())
	d := m.hostPanelDraft[h.ID()]
	val := draftFieldValue(d, m.hostPanelCursor)
	m.textInput.SetValue(val)
	m.textInput.Focus()
	m.hostPanelEditing = true
}

// updateHostPanelEditing 處理色彩欄位編輯中的按鍵。
func (m Model) updateHostPanelEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	hosts := m.visibleHosts()
	switch msg.Type {
	case tea.KeyEnter:
		// 確認編輯
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			d := m.hostPanelDraft[h.ID()]
			setDraftField(&d, m.hostPanelCursor, m.textInput.Value())
			m.hostPanelDraft[h.ID()] = d
		}
		m.hostPanelEditing = false
		m.textInput.Blur()
		return m, nil
	case tea.KeyEsc:
		// 取消編輯
		m.hostPanelEditing = false
		m.textInput.Blur()
		return m, nil
	default:
		// 委派給 textInput
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

// applyHostDrafts 將所有 draft 寫回 HostMgr。
func (m *Model) applyHostDrafts() {
	if m.deps.HostMgr == nil || m.hostPanelDraft == nil {
		return
	}
	for hostID, draft := range m.hostPanelDraft {
		h := m.deps.HostMgr.Host(hostID)
		if h == nil {
			continue
		}
		h.UpdateColors(draft.BarBG, draft.BarFG, draft.BadgeBG, draft.BadgeFG)
		if draft.Enabled {
			_ = m.deps.HostMgr.Enable(context.Background(), hostID)
		} else {
			_ = m.deps.HostMgr.Disable(hostID)
		}
	}
}

// persistHostsWithSync 在 persistHosts 之前同步 local 主機的顏色到 Config.Local。
func (m Model) persistHostsWithSync() {
	if m.deps.HostMgr == nil || m.deps.ConfigPath == "" {
		return
	}
	hosts := m.deps.HostMgr.Hosts()
	entries := make([]config.HostEntry, len(hosts))
	for i, h := range hosts {
		cfg := h.Config()
		cfg.SortOrder = i
		entries[i] = cfg
	}

	fileCfg := config.Default()
	cfgPath := m.deps.ConfigPath
	if data, err := os.ReadFile(cfgPath); err == nil {
		if loaded, err := config.LoadFromString(string(data)); err == nil {
			fileCfg = loaded
		}
	}
	fileCfg.Hosts = entries
	config.SyncLocalHostToConfig(&fileCfg)
	_ = config.SaveConfig(cfgPath, fileCfg)
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

	settingsActive := m.hostFocusCol == 2
	var b strings.Builder
	b.WriteString("\n")
	titleText := h.Config().Name + " 設定"
	if settingsActive {
		b.WriteString(fmt.Sprintf("  %s\n\n", selectedStyle.Render(titleText)))
	} else {
		b.WriteString(fmt.Sprintf("  %s\n\n", dimStyle.Render(titleText)))
	}

	// 五個欄位
	for i := 0; i < hostPanelFieldCount; i++ {
		cursor := "  "
		if settingsActive && i == m.hostPanelCursor {
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
			if m.hostPanelEditing && i == m.hostPanelCursor {
				line = fmt.Sprintf("  %s%-10s %s", cursor, hostPanelFieldLabels[i], m.textInput.View())
			} else {
				displayVal := val
				if displayVal == "" {
					displayVal = dimStyle.Render("（空）")
				} else {
					if isValidHexColor(displayVal) {
						swatch := lipgloss.NewStyle().
							Background(lipgloss.Color(displayVal)).
							Render("  ")
						displayVal = displayVal + " " + swatch
					}
				}
				line = fmt.Sprintf("  %s%-10s %s", cursor, hostPanelFieldLabels[i], displayVal)
				if i == 2 && val == "" {
					line += " " + dimStyle.Render("（留空由 tmux 自行決定）")
				}
			}
		}

		// 只在設定欄 active 時顯示游標底色
		if settingsActive && i == m.hostPanelCursor && !m.hostPanelEditing {
			line = m.cursorLine(line)
		}
		b.WriteString(line + "\n")
	}

	// 預覽
	b.WriteString("\n")
	cfg := h.Config()
	preview := renderColorPreview(cfg.Name, draft, cfg.Color)
	b.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#505050")).
		Padding(0, 1).
		Render(preview)))
	b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render("  ▲ 預覽")))

	return b.String()
}

// renderColorPreview 渲染一行模擬 status bar 的預覽。
func renderColorPreview(name string, draft hostDraftEntry, color string) string {
	badgeBG := draft.BadgeBG
	if badgeBG == "" && color != "" {
		badgeBG = color
	}

	// 建構 badge：色塊 + 主機名
	badgeStyle := lipgloss.NewStyle().Bold(true)
	if isValidHexColor(badgeBG) && badgeBG != "" {
		badgeStyle = badgeStyle.Background(lipgloss.Color(badgeBG))
	}
	if isValidHexColor(draft.BadgeFG) && draft.BadgeFG != "" {
		badgeStyle = badgeStyle.Foreground(lipgloss.Color(draft.BadgeFG))
	}

	// 建構 bar：背景 + 範例文字
	barStyle := lipgloss.NewStyle()
	if isValidHexColor(draft.BarBG) && draft.BarBG != "" {
		barStyle = barStyle.Background(lipgloss.Color(draft.BarBG))
	}
	if isValidHexColor(draft.BarFG) && draft.BarFG != "" {
		barStyle = barStyle.Foreground(lipgloss.Color(draft.BarFG))
	}

	badge := badgeStyle.Render(" " + name + " ")
	bar := barStyle.Render(" 0:zsh*  1:vim ")
	return badge + bar
}

// updateHubHostPanelOpen 處理 hub 模式右側面板開啟時的按鍵。
func (m Model) updateHubHostPanelOpen(msg tea.KeyMsg, hosts []config.HostEntry) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Esc 收合所有面板
		m.hostFocusCol = 0
		return m, nil
	case "left", "h":
		// 回到中欄
		m.hostFocusCol = 1
		return m, nil
	case "j", "down":
		if m.hostPanelCursor < hostPanelFieldCount-1 {
			m.hostPanelCursor++
		}
	case "k", "up":
		if m.hostPanelCursor > 0 {
			m.hostPanelCursor--
		}
	case "enter":
		if m.hostPanelCursor == 0 {
			m.toggleHubHostPanelEnabled(hosts)
		} else {
			m.enterHubHostPanelEdit(hosts)
		}
		return m, nil
	case " ":
		if m.hostPanelCursor == 0 {
			m.toggleHubHostPanelEnabled(hosts)
		}
		return m, nil
	}
	return m, nil
}

// toggleHubHostPanelEnabled 切換 hub 面板中「啟用」的 draft 值。
func (m *Model) toggleHubHostPanelEnabled(hosts []config.HostEntry) {
	if m.hostPickerCursor >= len(hosts) {
		return
	}
	h := hosts[m.hostPickerCursor]
	m.ensureDraftFromEntry(h)
	d := m.hostPanelDraft[h.Name]
	d.Enabled = !d.Enabled
	m.hostPanelDraft[h.Name] = d
}

// enterHubHostPanelEdit 進入 hub 面板色彩欄位編輯模式。
func (m *Model) enterHubHostPanelEdit(hosts []config.HostEntry) {
	if m.hostPickerCursor >= len(hosts) {
		return
	}
	h := hosts[m.hostPickerCursor]
	m.ensureDraftFromEntry(h)
	d := m.hostPanelDraft[h.Name]
	val := draftFieldValue(d, m.hostPanelCursor)
	m.textInput.SetValue(val)
	m.textInput.Focus()
	m.hostPanelEditing = true
}

// updateHubHostPanelEditing 處理 hub 模式色彩欄位編輯中的按鍵。
func (m Model) updateHubHostPanelEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	hosts := m.visibleHubHosts()
	switch msg.Type {
	case tea.KeyEnter:
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			d := m.hostPanelDraft[h.Name]
			setDraftField(&d, m.hostPanelCursor, m.textInput.Value())
			m.hostPanelDraft[h.Name] = d
		}
		m.hostPanelEditing = false
		m.textInput.Blur()
		return m, nil
	case tea.KeyEsc:
		m.hostPanelEditing = false
		m.textInput.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

// applyHubHostDrafts 將所有 draft 寫回 deps.Cfg.Hosts。
func (m *Model) applyHubHostDrafts() {
	if m.hostPanelDraft == nil {
		return
	}
	for hostName, draft := range m.hostPanelDraft {
		for i := range m.deps.Cfg.Hosts {
			if m.deps.Cfg.Hosts[i].Name == hostName {
				m.deps.Cfg.Hosts[i].BarBG = draft.BarBG
				m.deps.Cfg.Hosts[i].BarFG = draft.BarFG
				m.deps.Cfg.Hosts[i].BadgeBG = draft.BadgeBG
				m.deps.Cfg.Hosts[i].BadgeFG = draft.BadgeFG
				m.deps.Cfg.Hosts[i].Enabled = draft.Enabled
				break
			}
		}
	}
}

// persistHubHosts 讀取設定檔，替換 Hosts 區段後寫回。
func (m Model) persistHubHosts() {
	if m.deps.ConfigPath == "" {
		return
	}
	// 更新 SortOrder
	visible := 0
	for i := range m.deps.Cfg.Hosts {
		if !m.deps.Cfg.Hosts[i].Archived {
			m.deps.Cfg.Hosts[i].SortOrder = visible
			visible++
		}
	}

	fileCfg := config.Default()
	if data, err := os.ReadFile(m.deps.ConfigPath); err == nil {
		if loaded, err := config.LoadFromString(string(data)); err == nil {
			fileCfg = loaded
		}
	}
	fileCfg.Hosts = m.deps.Cfg.Hosts
	config.SyncLocalHostToConfig(&fileCfg)
	_ = config.SaveConfig(m.deps.ConfigPath, fileCfg)
}

// renderHubHostPickerRight 渲染 hub 模式右側設定面板。
func (m Model) renderHubHostPickerRight(hosts []config.HostEntry) string {
	if m.hostPickerCursor >= len(hosts) {
		return ""
	}
	h := hosts[m.hostPickerCursor]
	draft, ok := m.hostPanelDraft[h.Name]
	if !ok {
		return ""
	}

	settingsActive := m.hostFocusCol == 2
	var b strings.Builder
	b.WriteString("\n")
	titleText := h.Name + " 設定"
	if settingsActive {
		b.WriteString(fmt.Sprintf("  %s\n\n", selectedStyle.Render(titleText)))
	} else {
		b.WriteString(fmt.Sprintf("  %s\n\n", dimStyle.Render(titleText)))
	}

	// 五個欄位
	for i := 0; i < hostPanelFieldCount; i++ {
		cursor := "  "
		if settingsActive && i == m.hostPanelCursor {
			cursor = selectedStyle.Render("► ")
		}

		var line string
		if i == 0 {
			check := " "
			if draft.Enabled {
				check = "x"
			}
			line = fmt.Sprintf("  %s[%s] %s", cursor, check, hostPanelFieldLabels[i])
		} else {
			val := draftFieldValue(draft, i)
			if m.hostPanelEditing && i == m.hostPanelCursor {
				line = fmt.Sprintf("  %s%-10s %s", cursor, hostPanelFieldLabels[i], m.textInput.View())
			} else {
				displayVal := val
				if displayVal == "" {
					displayVal = dimStyle.Render("（空）")
				} else {
					if isValidHexColor(displayVal) {
						swatch := lipgloss.NewStyle().
							Background(lipgloss.Color(displayVal)).
							Render("  ")
						displayVal = displayVal + " " + swatch
					}
				}
				line = fmt.Sprintf("  %s%-10s %s", cursor, hostPanelFieldLabels[i], displayVal)
				if i == 2 && val == "" {
					line += " " + dimStyle.Render("（留空由 tmux 自行決定）")
				}
			}
		}

		if settingsActive && i == m.hostPanelCursor && !m.hostPanelEditing {
			line = m.cursorLine(line)
		}
		b.WriteString(line + "\n")
	}

	// 預覽
	b.WriteString("\n")
	preview := renderColorPreview(h.Name, draft, h.Color)
	b.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#505050")).
		Padding(0, 1).
		Render(preview)))
	b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render("  ▲ 預覽")))

	return b.String()
}
