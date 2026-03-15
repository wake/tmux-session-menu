package ui

import (
	"context"
	"fmt"
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
	return m.updateHostConnection(msg)
}

// ---------------------------------------------------------------------------
// 渲染
// ---------------------------------------------------------------------------

// renderHostConnection 渲染中欄（連線設定）。
func (m Model) renderHostConnection(hostName string, hostAddr string, isLocal bool) string {
	var b strings.Builder
	title := fmt.Sprintf("── %s 連線 ──", hostName)
	b.WriteString("\n")
	if m.hostFocusCol == 1 {
		b.WriteString("  " + selectedStyle.Render(title) + "\n")
	} else {
		b.WriteString("  " + dimStyle.Render(title) + "\n")
	}
	b.WriteString("\n")

	label := func(name string) string { return dimStyle.Render(name) }

	checkResult, hasCheck := m.hostCheckResults[hostName]

	if !isLocal {
		// SSH 檢測狀態
		b.WriteString(fmt.Sprintf("  %s  %s\n", label("SSH:"), renderCheckBool(checkResult.SSH, hasCheck)))
	}
	// tmux 檢測狀態
	b.WriteString(fmt.Sprintf("  %s  %s\n", label("tmux:"), renderCheckBool(checkResult.Tmux, hasCheck)))

	if !isLocal {
		// tunnel 狀態（從 hub snapshot 或 HostMgr 取得）
		tunnelStatus, tunnelErr := m.hostTunnelStatus(hostName)
		b.WriteString(fmt.Sprintf("  %s  %s\n", label("tunnel:"), renderTunnelStatus(tunnelStatus)))

		// hub-socket 狀態
		b.WriteString(fmt.Sprintf("  %s  %s\n", label("hub-sock:"), renderHubSockStatus(tunnelStatus)))

		// 最後連線時間
		if lc := m.hostLastConnected(hostName); lc != "" {
			b.WriteString(fmt.Sprintf("  %s  %s\n", label("最後連線:"), versionStyle.Render(lc)))
		}

		// 錯誤訊息
		if tunnelErr != "" {
			b.WriteString(fmt.Sprintf("  %s  %s\n", label("錯誤:"), errorStyle.Render(tunnelErr)))
		}
	}

	// hostcheck 失敗訊息
	if hasCheck && checkResult.Message != "" {
		b.WriteString(fmt.Sprintf("  %s  %s\n", label("訊息:"), dimStyle.Render(checkResult.Message)))
	}

	b.WriteString("\n")

	// 操作提示（只在焦點在中欄時顯示）
	if m.hostFocusCol == 1 {
		hints := keyStyle.Render("[r]") + dimStyle.Render(" 檢測")
		if !isLocal {
			hints += "  " + keyStyle.Render("[t]") + dimStyle.Render(" 重建 tunnel")
			hints += "  " + keyStyle.Render("[s]") + dimStyle.Render(" 設定 hub-sock")
		}
		b.WriteString("  " + hints + "\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// 渲染輔助函式
// ---------------------------------------------------------------------------

// renderCheckBool 渲染布林檢測結果。
func renderCheckBool(val *bool, hasCheck bool) string {
	if !hasCheck {
		return dimStyle.Render("—")
	}
	if val == nil {
		return dimStyle.Render("—")
	}
	if *val {
		return successStyle.Render("✓")
	}
	return errorStyle.Render("✗")
}

// renderTunnelStatus 渲染 tunnel 連線狀態。
func renderTunnelStatus(status tsmv1.HostStatus) string {
	switch status {
	case tsmv1.HostStatus_HOST_STATUS_CONNECTED:
		return successStyle.Render("✓ 已連線")
	case tsmv1.HostStatus_HOST_STATUS_CONNECTING:
		return dimStyle.Render("⠋ 連線中...")
	case tsmv1.HostStatus_HOST_STATUS_DISCONNECTED:
		return errorStyle.Render("✗ 已斷線")
	default:
		return dimStyle.Render("—")
	}
}

// renderHubSockStatus 渲染 hub-socket 設定狀態。
func renderHubSockStatus(status tsmv1.HostStatus) string {
	if status == tsmv1.HostStatus_HOST_STATUS_CONNECTED {
		return successStyle.Render("✓ 已設定")
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
