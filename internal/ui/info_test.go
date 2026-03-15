package ui_test

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/ui"
)

func infoApplyKey(m ui.Model, key string) (ui.Model, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	updated, cmd := m.Update(msg)
	return updated.(ui.Model), cmd
}

func TestInfoMode_EnterAndExit(t *testing.T) {
	deps := ui.Deps{Cfg: config.Default()}
	m := ui.NewModel(deps)

	// 按 i 進入 ModeInfo
	m, _ = infoApplyKey(m, "i")
	assert.Equal(t, ui.ModeInfo, m.Mode())

	// 按 Esc 返回 ModeNormal
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, m.Mode())
}

func TestInfoMode_DetectHubSocket(t *testing.T) {
	// 檢查是否有 hub socket 存在供偵測
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "tsm")
	matches, _ := filepath.Glob(filepath.Join(dir, "tsm-hub-*.sock"))

	deps := ui.Deps{Cfg: config.Default()}
	m := ui.NewModel(deps)
	m, _ = infoApplyKey(m, "i")

	view := m.View()
	// 如果有 hub socket 存在，info 面板應顯示 Hub 資訊
	if len(matches) > 0 {
		assert.Contains(t, view, "Hub:")
	}
}

func TestInfoMode_RenderContainsMode(t *testing.T) {
	deps := ui.Deps{Cfg: config.Default()}
	m := ui.NewModel(deps)
	m, _ = infoApplyKey(m, "i")

	view := m.View()
	assert.Contains(t, view, "連線資訊")
	// Hub 連線區段已移除，不應出現
	assert.NotContains(t, view, "Hub 連線")
}
