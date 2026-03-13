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
