package ui_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/ui"
)

// createTestSocket 建立測試用的 unix socket。
func createTestSocket(path string) (net.Listener, error) {
	return net.Listen("unix", path)
}

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

func TestInfoMode_HubReconnect(t *testing.T) {
	// 建立一個實際的 socket 檔案來通過 stat 檢查
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	// 建立 unix socket（使用 net.Listen）
	ln, err := createTestSocket(sockPath)
	if err != nil {
		t.Skip("cannot create test socket:", err)
	}
	defer ln.Close()

	deps := ui.Deps{
		Cfg:       config.Default(),
		HubSocket: sockPath,
	}
	m := ui.NewModel(deps)

	// 進入 info 模式
	m, _ = infoApplyKey(m, "i")
	assert.Equal(t, ui.ModeInfo, m.Mode())

	// 按 Enter 觸發連線（textinput 已有預設值 = sockPath）
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	assert.Equal(t, sockPath, m.HubReconnect(), "should set hub reconnect socket")
	assert.NotNil(t, cmd, "should return tea.Quit")
}

func TestInfoMode_EmptyPath(t *testing.T) {
	deps := ui.Deps{Cfg: config.Default()}
	m := ui.NewModel(deps)

	// 進入 info 模式
	m, _ = infoApplyKey(m, "i")

	// 清空輸入
	m.SetInfoHubInput("")

	// 按 Enter 應顯示錯誤，不退出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	assert.Equal(t, "", m.HubReconnect(), "should not reconnect with empty path")
	assert.Nil(t, cmd, "should not quit")
	assert.Equal(t, ui.ModeInfo, m.Mode(), "should stay in ModeInfo")
}

func TestInfoMode_NonexistentSocket(t *testing.T) {
	deps := ui.Deps{Cfg: config.Default()}
	m := ui.NewModel(deps)

	m, _ = infoApplyKey(m, "i")
	m.SetInfoHubInput("/nonexistent/path.sock")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	assert.Equal(t, "", m.HubReconnect())
	assert.Nil(t, cmd)
	assert.Equal(t, ui.ModeInfo, m.Mode())
}

func TestInfoMode_DetectHubSocket(t *testing.T) {
	// 建立假的 hub socket 供偵測
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "tsm")
	matches, _ := filepath.Glob(filepath.Join(dir, "tsm-hub-*.sock"))

	deps := ui.Deps{Cfg: config.Default()}
	m := ui.NewModel(deps)
	m, _ = infoApplyKey(m, "i")

	// 如果有 hub socket 存在，info 應該偵測到
	if len(matches) > 0 {
		assert.NotEmpty(t, m.InfoHubInputValue(), "should auto-detect hub socket")
	}
}

func TestInfoMode_RenderContainsMode(t *testing.T) {
	deps := ui.Deps{Cfg: config.Default()}
	m := ui.NewModel(deps)
	m, _ = infoApplyKey(m, "i")

	view := m.View()
	assert.Contains(t, view, "連線資訊")
	assert.Contains(t, view, "Hub 連線")
}
