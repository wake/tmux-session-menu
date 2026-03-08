package ui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
	"github.com/wake/tmux-session-menu/internal/ui"
)

// hpApplyKey 是 hostpicker 測試用的 applyKey helper。
func hpApplyKey(m ui.Model, key string) (ui.Model, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	updated, cmd := m.Update(msg)
	return updated.(ui.Model), cmd
}

func TestHostPickerNavigation(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "dev", Address: "dev", Color: "#70AD47", Enabled: true})

	deps := ui.Deps{HostMgr: mgr}
	m := ui.NewModel(deps)

	// 按 h 進入 HostPicker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode(), "should enter ModeHostPicker")
	assert.Equal(t, 0, m.HostPickerCursor(), "cursor should start at 0")

	// 按 j 向下
	m, _ = hpApplyKey(m, "j")
	assert.Equal(t, 1, m.HostPickerCursor(), "cursor should move to 1")

	// 按 j 不超過邊界
	m, _ = hpApplyKey(m, "j")
	assert.Equal(t, 1, m.HostPickerCursor(), "cursor should stay at 1 (boundary)")

	// 按 k 向上
	m, _ = hpApplyKey(m, "k")
	assert.Equal(t, 0, m.HostPickerCursor(), "cursor should move back to 0")

	// 按 k 不超過邊界
	m, _ = hpApplyKey(m, "k")
	assert.Equal(t, 0, m.HostPickerCursor(), "cursor should stay at 0 (boundary)")

	// 按 esc 回到 Normal
	m, _ = hpApplyKey(m, "esc")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "should return to ModeNormal after esc")
}

func TestHostPickerCloseWithH(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 按 h 進入
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 再按 h 關閉
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "pressing h again should close HostPicker")
}

func TestHostPickerNoHostMgr(t *testing.T) {
	// 無 HostMgr 時按 h 不進入 HostPicker
	m := ui.NewModel(ui.Deps{})
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "should stay ModeNormal without HostMgr")
}

func TestHostPickerAddHost(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 按 a 進入新增主機輸入模式
	m, _ = hpApplyKey(m, "a")
	assert.Equal(t, ui.ModeInput, m.Mode(), "pressing a should enter ModeInput")
	assert.Equal(t, ui.InputNewHost, m.InputTarget(), "input target should be InputNewHost")
}

func TestHostPickerDeleteLocalProtected(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker，游標在 local 主機上
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 按 d 不應進入確認模式（local 不可刪除）
	m, _ = hpApplyKey(m, "d")
	assert.Equal(t, ui.ModeHostPicker, m.Mode(), "should stay in ModeHostPicker when deleting local")
}

func TestHostPickerDeleteRemote(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "dev", Address: "dev.example.com", Color: "#70AD47", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker，移到第二個主機（遠端）
	m, _ = hpApplyKey(m, "h")
	m, _ = hpApplyKey(m, "j")
	assert.Equal(t, 1, m.HostPickerCursor())

	// 按 d 應進入確認模式
	m, _ = hpApplyKey(m, "d")
	assert.Equal(t, ui.ModeConfirm, m.Mode(), "should enter ModeConfirm for remote host delete")
}

func TestHostPickerReorder(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "aaa", Color: "#111111", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "bbb", Color: "#222222", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "ccc", Color: "#333333", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, 0, m.HostPickerCursor())

	// 按 J（shift+j）下移第一個主機
	m, _ = hpApplyKey(m, "J")
	assert.Equal(t, 1, m.HostPickerCursor(), "cursor should follow moved item to position 1")

	// 驗證順序：bbb, aaa, ccc
	hosts := mgr.Hosts()
	assert.Equal(t, "bbb", hosts[0].ID())
	assert.Equal(t, "aaa", hosts[1].ID())
	assert.Equal(t, "ccc", hosts[2].ID())

	// 按 K（shift+k）上移回去
	m, _ = hpApplyKey(m, "K")
	assert.Equal(t, 0, m.HostPickerCursor(), "cursor should follow moved item back to position 0")

	hosts = mgr.Hosts()
	assert.Equal(t, "aaa", hosts[0].ID())
	assert.Equal(t, "bbb", hosts[1].ID())
	assert.Equal(t, "ccc", hosts[2].ID())
}

func TestHostPickerRenderContainsHosts(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "staging", Address: "staging.example.com", Color: "#70AD47", Enabled: false})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker 並渲染
	m, _ = hpApplyKey(m, "h")
	view := m.View()

	assert.Contains(t, view, "主機管理", "should show panel title")
	assert.Contains(t, view, "local", "should show local host name")
	assert.Contains(t, view, "staging", "should show staging host name")
	assert.Contains(t, view, "已停用", "should show disabled status for staging")
}

func TestHostPickerToolbarVisible(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})
	view := m.View()
	assert.Contains(t, view, "主機管理", "toolbar should show host management shortcut in multi-host mode")
}

func TestHostPickerToolbarHiddenWithoutHostMgr(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	view := m.View()
	// [h] 主機管理 should not appear without HostMgr
	assert.NotContains(t, view, "主機管理", "toolbar should not show host management shortcut without HostMgr")
}
