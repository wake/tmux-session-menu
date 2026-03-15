package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/version"
)

// enterInfoMode 進入連線資訊模式（唯讀）。
func (m Model) enterInfoMode() Model {
	m.mode = ModeInfo
	m.err = nil
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
	}
	return m, nil
}

// renderInfo 渲染連線資訊面板（唯讀）。
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
	b.WriteString(fmt.Sprintf("  %s  %s\n", label("Daemon:"), val(m.daemonSocketPath())))
	if hubSock := m.detectHubSocket(); hubSock != "" {
		b.WriteString(fmt.Sprintf("  %s  %s\n", label("Hub:"), val(hubSock)))
	} else {
		b.WriteString(fmt.Sprintf("  %s  %s\n", label("Hub:"), dimStyle.Render("(未偵測到 reverse tunnel)")))
	}
	b.WriteString(fmt.Sprintf("  %s  %s\n", label("連線:"), val(m.currentSocketPath())))

	b.WriteString(fmt.Sprintf("\n  %s\n", render("[Esc]", "返回")))

	return b.String()
}

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
