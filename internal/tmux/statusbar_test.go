package tmux_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

func TestStatusBarArgs_NoBarBG(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "",
		BadgeBG: "#5f8787",
		BadgeFG: "#c0caf5",
	}
	cmds := tmux.StatusBarArgs(colors)

	// bar_bg 為空時應 unset status-style（清除 remote 殘留）
	foundUnset := false
	for _, cmd := range cmds {
		if len(cmd) >= 3 && cmd[0] == "set-option" && cmd[1] == "-gu" && cmd[2] == "status-style" {
			foundUnset = true
		}
	}
	assert.True(t, foundUnset, "should unset status-style when bar_bg is empty")

	// 應包含 status-left
	hasStatusLeft := false
	for _, cmd := range cmds {
		for _, arg := range cmd {
			if arg == "status-left" {
				hasStatusLeft = true
			}
		}
	}
	assert.True(t, hasStatusLeft)
}

func TestStatusBarArgs_WithBarBG(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "#1a2b2b",
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}
	cmds := tmux.StatusBarArgs(colors)

	// 應包含 status-style bg
	found := false
	for _, cmd := range cmds {
		if len(cmd) >= 4 && cmd[2] == "status-style" {
			assert.Contains(t, cmd[3], "bg=#1a2b2b")
			found = true
		}
	}
	assert.True(t, found, "should contain status-style with bg color")
}

func TestStatusBarArgs_BadgeInStatusLeft(t *testing.T) {
	colors := config.ColorConfig{
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}
	cmds := tmux.StatusBarArgs(colors)

	for _, cmd := range cmds {
		if len(cmd) >= 4 && cmd[2] == "status-left" {
			assert.Contains(t, cmd[3], "#73daca")
			assert.Contains(t, cmd[3], "#1a1b26")
			assert.Contains(t, cmd[3], "@tsm_name")
		}
	}
}

// statusBarMockExec 用於測試 ApplyStatusBar，記錄所有呼叫。
type statusBarMockExec struct {
	calls [][]string
	err   error
}

func (m *statusBarMockExec) Execute(args ...string) (string, error) {
	m.calls = append(m.calls, args)
	return "", m.err
}

func TestApplyStatusBar_Success(t *testing.T) {
	mock := &statusBarMockExec{}
	colors := config.ColorConfig{
		BarBG:   "#1a2b2b",
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}

	err := tmux.ApplyStatusBar(mock, colors)
	assert.NoError(t, err)
	// 應有 3 組指令：status-style + status-left + status-left-length
	assert.Len(t, mock.calls, 3)
}

func TestApplyStatusBar_NoBarBG(t *testing.T) {
	mock := &statusBarMockExec{}
	colors := config.ColorConfig{
		BarBG:   "",
		BadgeBG: "#5f8787",
		BadgeFG: "#c0caf5",
	}

	err := tmux.ApplyStatusBar(mock, colors)
	assert.NoError(t, err)
	// unset status-style + status-left + status-left-length = 3 組
	assert.Len(t, mock.calls, 3)
	// 第一組應為 unset
	assert.Equal(t, []string{"set-option", "-gu", "status-style"}, mock.calls[0])
}

func TestApplyStatusBar_Error(t *testing.T) {
	mock := &statusBarMockExec{err: fmt.Errorf("tmux not running")}
	colors := config.ColorConfig{
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}

	err := tmux.ApplyStatusBar(mock, colors)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tmux not running")
}

// TestStatusBarArgs_WithBarFG 驗證同時設定 BarBG 與 BarFG 時，status-style 包含 bg= 與 fg=。
func TestStatusBarArgs_WithBarFG(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "#1a2b2b",
		BarFG:   "#e0e0e0",
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}
	cmds := tmux.StatusBarArgs(colors)

	found := false
	for _, cmd := range cmds {
		if len(cmd) >= 4 && cmd[2] == "status-style" {
			assert.Contains(t, cmd[3], "bg=#1a2b2b", "應包含 bg 色")
			assert.Contains(t, cmd[3], "fg=#e0e0e0", "應包含 fg 色")
			found = true
		}
	}
	assert.True(t, found, "應找到 status-style 指令")
}

// TestStatusBarArgs_EmptyBarFG 驗證 BarBG 有值、BarFG 為空時，status-style 只含 bg=，不含 fg=。
func TestStatusBarArgs_EmptyBarFG(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "#1a2b2b",
		BarFG:   "",
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}
	cmds := tmux.StatusBarArgs(colors)

	for _, cmd := range cmds {
		if len(cmd) >= 4 && cmd[2] == "status-style" {
			assert.Contains(t, cmd[3], "bg=#1a2b2b", "應包含 bg 色")
			assert.NotContains(t, cmd[3], "fg=", "BarFG 為空時不應設定 fg")
		}
	}
}

// TestStatusBarArgs_OnlyBarFG 驗證 BarBG 為空、BarFG 有值時，status-style 只含 fg=。
func TestStatusBarArgs_OnlyBarFG(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "",
		BarFG:   "#ffffff",
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}
	cmds := tmux.StatusBarArgs(colors)

	found := false
	for _, cmd := range cmds {
		if len(cmd) >= 4 && cmd[2] == "status-style" {
			assert.Equal(t, "fg=#ffffff", cmd[3], "只設定 BarFG 時 style 應為 fg=...")
			assert.NotContains(t, cmd[3], "bg=", "BarBG 為空時不應設定 bg")
			found = true
		}
	}
	assert.True(t, found, "應找到 status-style 指令（僅含 fg）")
}
