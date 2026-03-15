package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/hostcheck"
	"github.com/wake/tmux-session-menu/internal/remote"
)

// ---------------------------------------------------------------------------
// tea.Msg 類型
// ---------------------------------------------------------------------------

// hostCheckMsg 攜帶 hostcheck 結果。
type hostCheckMsg struct {
	HostID string
	Result hostcheck.Result
}

// reconnectHostMsg 攜帶 ReconnectHost 結果。
type reconnectHostMsg struct {
	HostID string
	Err    error
}

// setHubSocketMsg 攜帶 SetHubSocket 結果。
type setHubSocketMsg struct {
	HostID string
	Err    error
}

// ---------------------------------------------------------------------------
// tea.Cmd 函式
// ---------------------------------------------------------------------------

// checkHostCmd 觸發 hostcheck，address 為空表示本機。
func checkHostCmd(hostID, address string) tea.Cmd {
	return func() tea.Msg {
		c := hostcheck.DefaultChecker()
		var result hostcheck.Result
		if address == "" {
			result = c.CheckLocal()
		} else {
			result = c.CheckRemote(address)
		}
		return hostCheckMsg{HostID: hostID, Result: result}
	}
}

// reconnectHostCmd 透過 gRPC 強制重建指定主機的連線。
func reconnectHostCmd(cl *client.Client, hostID string) tea.Cmd {
	return func() tea.Msg {
		err := cl.ReconnectHost(context.Background(), hostID)
		return reconnectHostMsg{HostID: hostID, Err: err}
	}
}

// setHubSocketCmd 透過 SSH 設定遠端主機的 hub socket 路徑與 self ID。
func setHubSocketCmd(host, socketPath, selfID string) tea.Cmd {
	return func() tea.Msg {
		err := remote.SetHubSocket(host, socketPath)
		if err != nil {
			return setHubSocketMsg{HostID: host, Err: err}
		}
		err = remote.SetHubSelf(host, selfID)
		return setHubSocketMsg{HostID: host, Err: err}
	}
}

// ---------------------------------------------------------------------------
// 按鍵處理
// ---------------------------------------------------------------------------

// updateHostConnection 處理中欄（連線設定）的按鍵。
func (m Model) updateHostConnection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// hub-socket 路徑編輯模式
	if m.hostConnEditing {
		return m.updateHostConnEditing(msg)
	}

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
	case "r":
		// 觸發 hostcheck
		name, addr, _ := m.currentConnectionHost()
		if name != "" {
			return m, checkHostCmd(name, addr)
		}
		return m, nil
	case "t":
		// 重建 tunnel（僅遠端主機）
		name, _, isLocal := m.currentConnectionHost()
		if !isLocal && m.deps.Client != nil {
			return m, reconnectHostCmd(m.deps.Client, name)
		}
		return m, nil
	case "s":
		// 設定 hub-socket 路徑（僅遠端主機）
		_, _, isLocal := m.currentConnectionHost()
		if !isLocal {
			m.hostConnEditing = true
			m.textInput.SetValue("")
			m.textInput.Focus()
			return m, nil
		}
		return m, nil
	}
	return m, nil
}

// updateHubHostConnection 處理 hub 模式中欄的按鍵。
// 共用同一個邏輯（currentConnectionHost 已處理 hub/非 hub 模式）。
func (m Model) updateHubHostConnection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.updateHostConnection(msg)
}

// updateHostConnEditing 處理 hub-socket 路徑編輯模式的按鍵。
func (m Model) updateHostConnEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		sockPath := strings.TrimSpace(m.textInput.Value())
		if sockPath == "" || !strings.Contains(filepath.Base(sockPath), "tsm-hub-") {
			m.err = fmt.Errorf("必須是 hub socket（tsm-hub-*.sock）")
			m.hostConnEditing = false
			return m, nil
		}
		m.hostConnEditing = false
		name, addr, _ := m.currentConnectionHost()
		// 使用 address（SSH 目標）；若為空則用 name
		target := addr
		if target == "" {
			target = name
		}
		return m, setHubSocketCmd(target, sockPath, name)
	case "esc":
		m.hostConnEditing = false
		return m, nil
	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

// currentConnectionHost 回傳目前連線欄對應的主機資訊。
func (m Model) currentConnectionHost() (name, addr string, isLocal bool) {
	// Hub 模式
	if m.deps.HubMode {
		hosts := m.visibleHubHosts()
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			return h.Name, h.Address, h.IsLocal()
		}
		return "", "", true
	}
	// HostMgr 模式
	if m.deps.HostMgr != nil {
		hosts := m.visibleHosts()
		if m.hostPickerCursor < len(hosts) {
			cfg := hosts[m.hostPickerCursor].Config()
			return cfg.Name, cfg.Address, cfg.IsLocal()
		}
	}
	return "", "", true
}

// ---------------------------------------------------------------------------
// 渲染
// ---------------------------------------------------------------------------

// renderHostConnection 渲染中欄（連線設定）。
func (m Model) renderHostConnection(hostName string, hostAddr string, isLocal bool) string {
	active := m.hostFocusCol == 1
	var b strings.Builder

	// 標題：比照設定面板的樣式
	b.WriteString("\n")
	titleText := hostName + " 連線"
	if active {
		b.WriteString(fmt.Sprintf("  %s\n", selectedStyle.Render(titleText)))
	} else {
		b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render(titleText)))
	}
	b.WriteString("\n")

	// 標籤和值的樣式：active 時正常顯色，非 active 時 dimmed
	label := func(name string) string { return dimStyle.Render(name) }
	val := func(s string) string {
		if active {
			return sessionNameStyle.Render(s)
		}
		return dimStyle.Render(s)
	}

	checkResult, hasCheck := m.hostCheckResults[hostName]

	if !isLocal {
		b.WriteString(fmt.Sprintf("  %s  %s\n", label("SSH:"), renderCheckBoolStyled(checkResult.SSH, hasCheck, active)))
	}
	b.WriteString(fmt.Sprintf("  %s  %s\n", label("tmux:"), renderCheckBoolStyled(checkResult.Tmux, hasCheck, active)))

	if !isLocal {
		tunnelStatus, tunnelErr := m.hostTunnelStatus(hostName)
		b.WriteString(fmt.Sprintf("  %s  %s\n", label("tunnel:"), renderTunnelStatusStyled(tunnelStatus, active)))
		b.WriteString(fmt.Sprintf("  %s  %s\n", label("hub-sock:"), renderHubSockStatusStyled(tunnelStatus, active)))

		if lc := m.hostLastConnected(hostName); lc != "" {
			b.WriteString(fmt.Sprintf("  %s  %s\n", label("最後連線:"), val(lc)))
		}

		if tunnelErr != "" {
			b.WriteString(fmt.Sprintf("  %s  %s\n", label("錯誤:"), errorStyle.Render(tunnelErr)))
		}
	}

	if hasCheck && checkResult.Message != "" {
		b.WriteString(fmt.Sprintf("  %s  %s\n", label("訊息:"), dimStyle.Render(checkResult.Message)))
	}

	b.WriteString("\n")

	// hub-socket 路徑編輯模式
	if m.hostConnEditing && active {
		b.WriteString("  " + label("路徑:") + " " + m.textInput.View() + "\n")
		b.WriteString("\n")
	}

	// 操作提示：每行一個，避免橫向過寬
	if active && !m.hostConnEditing {
		b.WriteString("  " + keyStyle.Render("[r]") + dimStyle.Render(" 檢測") + "\n")
		if !isLocal {
			b.WriteString("  " + keyStyle.Render("[t]") + dimStyle.Render(" 重建 tunnel") + "\n")
			b.WriteString("  " + keyStyle.Render("[s]") + dimStyle.Render(" 設定 hub-sock") + "\n")
		}
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// 渲染輔助函式
// ---------------------------------------------------------------------------

// renderCheckBoolStyled 渲染布林檢測結果（active 時顯色，非 active 時 dim）。
func renderCheckBoolStyled(val *bool, hasCheck, active bool) string {
	if !hasCheck || val == nil {
		return dimStyle.Render("—")
	}
	if *val {
		if active {
			return successStyle.Render("✓")
		}
		return dimStyle.Render("✓")
	}
	return errorStyle.Render("✗")
}

// renderTunnelStatusStyled 渲染 tunnel 連線狀態（active 時顯色，非 active 時 dim）。
func renderTunnelStatusStyled(status tsmv1.HostStatus, active bool) string {
	switch status {
	case tsmv1.HostStatus_HOST_STATUS_CONNECTED:
		if active {
			return successStyle.Render("✓ 已連線")
		}
		return dimStyle.Render("✓ 已連線")
	case tsmv1.HostStatus_HOST_STATUS_CONNECTING:
		return dimStyle.Render("⠋ 連線中...")
	case tsmv1.HostStatus_HOST_STATUS_DISCONNECTED:
		return errorStyle.Render("✗ 已斷線")
	default:
		return dimStyle.Render("—")
	}
}

// renderHubSockStatusStyled 渲染 hub-socket 設定狀態（active 時顯色，非 active 時 dim）。
func renderHubSockStatusStyled(status tsmv1.HostStatus, active bool) string {
	if status == tsmv1.HostStatus_HOST_STATUS_CONNECTED {
		if active {
			return successStyle.Render("✓ 已設定")
		}
		return dimStyle.Render("✓ 已設定")
	}
	return dimStyle.Render("—")
}

// hostTunnelStatus 從 hub snapshot 取得主機的 tunnel 狀態和錯誤。
func (m Model) hostTunnelStatus(hostName string) (tsmv1.HostStatus, string) {
	if m.hubHostSnap != nil {
		for _, hs := range m.hubHostSnap.Hosts {
			if hs.HostId == hostName || hs.Name == hostName {
				return hs.Status, hs.Error
			}
		}
	}
	// 非 hub 模式：嘗試從 HostMgr 取得
	if m.deps.HostMgr != nil {
		if h := m.deps.HostMgr.Host(hostName); h != nil {
			st := h.Status()
			switch st {
			case 0: // HostDisabled
				return tsmv1.HostStatus_HOST_STATUS_DISABLED, ""
			case 1: // HostConnecting
				return tsmv1.HostStatus_HOST_STATUS_CONNECTING, ""
			case 2: // HostConnected
				return tsmv1.HostStatus_HOST_STATUS_CONNECTED, ""
			case 3: // HostDisconnected
				return tsmv1.HostStatus_HOST_STATUS_DISCONNECTED, h.LastError()
			}
		}
	}
	return tsmv1.HostStatus_HOST_STATUS_DISABLED, ""
}

// hostLastConnected 從 hub snapshot 取得最後連線的相對時間字串。
func (m Model) hostLastConnected(hostName string) string {
	if m.hubHostSnap == nil {
		return ""
	}
	for _, hs := range m.hubHostSnap.Hosts {
		if (hs.HostId == hostName || hs.Name == hostName) && hs.LastConnected != nil {
			t := hs.LastConnected.AsTime()
			if t.IsZero() {
				return ""
			}
			return relativeTime(t)
		}
	}
	return ""
}

// relativeTime 回傳相對時間字串。
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "剛剛"
	case d < time.Hour:
		return fmt.Sprintf("%d 分鐘前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小時前", int(d.Hours()))
	default:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
}
