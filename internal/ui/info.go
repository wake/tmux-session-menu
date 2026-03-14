package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/version"
)

// enterInfoMode 進入連線資訊模式，初始化 hub socket 輸入欄位。
func (m Model) enterInfoMode() Model {
	m.mode = ModeInfo
	m.err = nil

	ti := textinput.New()
	ti.PromptStyle = selectedStyle
	ti.TextStyle = versionStyle
	ti.Cursor.Style = selectedStyle
	ti.CharLimit = 256
	ti.Width = 60

	// 預設值：Deps.HubSocket > tmux @tsm_hub_socket > 自動偵測
	defaultSock := m.deps.HubSocket
	if defaultSock == "" {
		defaultSock = m.readTmuxHubSocket()
	}
	if defaultSock == "" {
		defaultSock = m.detectHubSocket()
	}
	ti.SetValue(defaultSock)
	ti.Focus()

	m.infoHubInput = ti
	return m
}

// readTmuxHubSocket 從 tmux 全域選項讀取 @tsm_hub_socket。
func (m Model) readTmuxHubSocket() string {
	if !m.deps.Cfg.InTmux {
		return ""
	}
	exec := tmux.NewRealExecutor()
	out, err := exec.Execute("show-option", "-gqv", "@tsm_hub_socket")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// detectHubSocket 自動偵測 ~/.config/tsm/tsm-hub-*.sock 中可用的 socket。
func (m Model) detectHubSocket() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	pattern := filepath.Join(home, ".config", "tsm", "tsm-hub-*.sock")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}
	// 回傳第一個找到的
	return matches[0]
}

// daemonSocketPath 回傳本機 daemon 的 socket 路徑。
func (m Model) daemonSocketPath() string {
	dir := config.ExpandPath(m.deps.Cfg.DataDir)
	return filepath.Join(dir, "tsm.sock")
}

// updateInfo 處理 ModeInfo 的按鍵事件。
func (m Model) updateInfo(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ModeNormal
		return m, nil
	case tea.KeyEnter:
		// Enter = 連線（同 c）
		return m.infoConnect()
	}

	// 將按鍵轉發到 text input
	var cmd tea.Cmd
	m.infoHubInput, cmd = m.infoHubInput.Update(msg)
	return m, cmd
}

// infoConnect 嘗試連線到使用者輸入的 hub socket。
func (m Model) infoConnect() (tea.Model, tea.Cmd) {
	sockPath := strings.TrimSpace(m.infoHubInput.Value())
	if sockPath == "" {
		m.err = fmt.Errorf("hub socket 路徑不可為空")
		return m, nil
	}

	// 檢查 socket 檔案是否存在
	info, err := os.Stat(sockPath)
	if err != nil {
		m.err = fmt.Errorf("socket 不存在: %s", sockPath)
		return m, nil
	}
	if info.Mode()&os.ModeSocket == 0 {
		m.err = fmt.Errorf("不是 socket 檔案: %s", sockPath)
		return m, nil
	}

	// 只影響當前 session 的重連，不設定 @tsm_hub_socket
	// （@tsm_hub_socket 由 hub 端 SetHubSocket 管理，不應從 UI 覆寫）
	m.hubReconnectSock = sockPath
	m.quitting = true
	return m, tea.Quit
}

// renderInfo 渲染連線資訊面板。
func (m Model) renderInfo() string {
	var b strings.Builder
	render := func(key, desc string) string {
		return keyStyle.Render(key) + versionStyle.Render(" "+desc)
	}
	label := func(name string) string {
		return dimStyle.Render(name)
	}
	val := func(v string) string {
		return sessionNameStyle.Render(v)
	}

	b.WriteString("\n")
	b.WriteString("  " + selectedStyle.Render("── 連線資訊 ──") + "\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", label("模式:"), val(m.connectionMode())))
	b.WriteString(fmt.Sprintf("  %s  %s\n", label("版本:"), val(version.String())))
	b.WriteString(fmt.Sprintf("  %s  %s\n", label("Socket:"), val(m.currentSocketPath())))
	b.WriteString("\n")
	b.WriteString("  " + selectedStyle.Render("── Hub 連線 ──") + "\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", label("路徑:"), m.infoHubInput.View()))

	b.WriteString(fmt.Sprintf("\n  %s  %s\n",
		render("[Enter]", "連線"),
		render("[Esc]", "返回")))

	return b.String()
}

// InfoHubInputValue 回傳 hub socket 輸入欄位的目前值（供測試使用）。
func (m Model) InfoHubInputValue() string { return m.infoHubInput.Value() }

// SetInfoHubInput 設定 hub socket 輸入欄位的值（供測試使用）。
func (m *Model) SetInfoHubInput(v string) { m.infoHubInput.SetValue(v) }

// currentSocketPath 回傳目前連線使用的 socket 路徑。
func (m Model) currentSocketPath() string {
	if m.deps.HubSocket != "" {
		return m.deps.HubSocket
	}
	if m.deps.Client != nil {
		return m.daemonSocketPath()
	}
	return "(direct mode)"
}
