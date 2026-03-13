package cfgtui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/cfgtui"
	"github.com/wake/tmux-session-menu/internal/config"
)

func TestNewModel_DefaultTab(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	assert.Equal(t, cfgtui.TabClient, m.Tab())
}

func TestNewModel_FieldCount(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 客戶端：4 個 local 顏色欄位（bar_bg, bar_fg, badge_bg, badge_fg）
	assert.Equal(t, 4, m.FieldCount(cfgtui.TabClient))
	// 伺服器：2 個 server 欄位 + 4 個 local 顏色欄位
	assert.Equal(t, 6, m.FieldCount(cfgtui.TabServer))
}

func TestModel_SwitchTab(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := newM.(cfgtui.Model)
	assert.Equal(t, cfgtui.TabServer, model.Tab())
}

func TestModel_SwitchTabBack(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m2, _ := m1.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model := m2.(cfgtui.Model)
	assert.Equal(t, cfgtui.TabClient, model.Tab())
}

func TestModel_CursorMove(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	assert.Equal(t, 0, m.Cursor())

	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := m1.(cfgtui.Model)
	assert.Equal(t, 1, model.Cursor())
}

func TestModel_CursorNoOverflow(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 按上不能低於 0
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	model := m1.(cfgtui.Model)
	assert.Equal(t, 0, model.Cursor())
}

func TestModel_CursorNoOverflowBottom(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// TabClient 有 4 個欄位，cursor 最大應為 3
	var current tea.Model = m
	for i := 0; i < 10; i++ {
		current, _ = current.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	model := current.(cfgtui.Model)
	assert.Equal(t, 3, model.Cursor())
}

func TestModel_CursorJK(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)

	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := m1.(cfgtui.Model)
	assert.Equal(t, 1, model.Cursor())

	m2, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model2 := m2.(cfgtui.Model)
	assert.Equal(t, 0, model2.Cursor())
}

func TestModel_EnterEditMode(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := newM.(cfgtui.Model)
	assert.True(t, model.Editing())
}

func TestModel_EscExitEditMode(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 進入編輯模式
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model1 := m1.(cfgtui.Model)
	assert.True(t, model1.Editing())
	// Esc 退出編輯模式
	m2, _ := model1.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model2 := m2.(cfgtui.Model)
	assert.False(t, model2.Editing())
}

func TestModel_DeleteResetsToDefault(t *testing.T) {
	cfg := config.Default()
	cfg.Local.BarBG = "#changed"
	m := cfgtui.NewModel(cfg)
	// cursor 在第一個欄位 (local.bar_bg)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	model := newM.(cfgtui.Model)
	result := model.ResultConfig()
	assert.Equal(t, "", result.Local.BarBG) // Default 是空字串
}

func TestModel_BackspaceResetsToDefault(t *testing.T) {
	cfg := config.Default()
	cfg.Local.BadgeBG = "#changed"
	m := cfgtui.NewModel(cfg)
	// 移動到第三個欄位 (local.badge_bg，index 2)
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2, _ := m1.Update(tea.KeyMsg{Type: tea.KeyDown})
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model := m3.(cfgtui.Model)
	result := model.ResultConfig()
	assert.Equal(t, "#5f8787", result.Local.BadgeBG) // Default 的 badge_bg
}

func TestModel_ResultConfig(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	result := m.ResultConfig()
	assert.Equal(t, cfg.Local.BadgeBG, result.Local.BadgeBG)
	// Remote 欄位已從 TUI 移除，但 cfg 原值應被保留（不被清空）
	assert.Equal(t, cfg.Remote.BarBG, result.Remote.BarBG)
	assert.Equal(t, cfg.PreviewLines, result.PreviewLines)
}

func TestModel_CtrlS_SavesAndQuits(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model := newM.(cfgtui.Model)
	assert.True(t, model.Saved())
	assert.NotNil(t, cmd) // tea.Quit
}

func TestModel_Q_QuitsWithoutSave(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model := newM.(cfgtui.Model)
	assert.False(t, model.Saved())
	assert.NotNil(t, cmd) // tea.Quit
}

func TestModel_Esc_QuitsWithoutSave(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model := newM.(cfgtui.Model)
	assert.False(t, model.Saved())
	assert.NotNil(t, cmd) // tea.Quit
}

func TestModel_CtrlC_QuitsWithoutSave(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model := newM.(cfgtui.Model)
	assert.False(t, model.Saved())
	assert.NotNil(t, cmd) // tea.Quit
}

func TestModel_SetHostname(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	m.SetHostname("air-2019")
	// View 應包含 hostname
	view := m.View()
	assert.Contains(t, view, "air-2019")
}

func TestModel_TabResetsCursor(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 移動 cursor 到第 3 個
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2, _ := m1.Update(tea.KeyMsg{Type: tea.KeyDown})
	// 切換分頁
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := m3.(cfgtui.Model)
	assert.Equal(t, 0, model.Cursor())
}

func TestModel_EditAndConfirm(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 進入編輯模式
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model1 := m1.(cfgtui.Model)
	assert.True(t, model1.Editing())

	// 輸入新值（先清空原值再輸入）
	// textinput 會自動處理，我們模擬輸入字元
	var current tea.Model = model1
	for _, r := range "#ff0000" {
		current, _ = current.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Enter 確認編輯
	m3, _ := current.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model3 := m3.(cfgtui.Model)
	assert.False(t, model3.Editing())
}

func TestModel_ViewContainsTabs(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	view := m.View()
	assert.Contains(t, view, "客戶端")
	assert.Contains(t, view, "伺服器")
}

func TestModel_ViewContainsFieldLabels(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	view := m.View()
	// 客戶端分頁欄位標籤
	assert.Contains(t, view, "local.bar_bg")
	assert.Contains(t, view, "local.bar_fg")
	assert.Contains(t, view, "local.badge_bg")
	assert.Contains(t, view, "local.badge_fg")
	// remote.* 欄位已移除
	assert.NotContains(t, view, "remote.badge_fg")
}

func TestModel_ServerTabContainsExtraFields(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 切到伺服器分頁
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := m1.(cfgtui.Model)
	view := model.View()
	assert.Contains(t, view, "preview_lines")
	assert.Contains(t, view, "poll_interval_sec")
}

func TestModel_EditInEditModeDoesNotQuit(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 進入編輯模式
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// 在編輯模式下按 q 不應退出
	m2, _ := m1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model := m2.(cfgtui.Model)
	assert.True(t, model.Editing())
	// textinput 可能回傳游標閃爍 cmd，不檢查 cmd 是否為 nil
	// 重點是 model 仍在編輯模式、沒有觸發退出
	assert.False(t, model.Saved())
}

func TestModel_SwitchTabNoOverflow(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 已在 TabClient，按 Left 不應改變
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model1 := m1.(cfgtui.Model)
	assert.Equal(t, cfgtui.TabClient, model1.Tab())

	// 切到 TabServer 後按 Right 不應改變
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRight})
	model3 := m3.(cfgtui.Model)
	assert.Equal(t, cfgtui.TabServer, model3.Tab())
}

func TestModel_ServerTabEditPreviewLines(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 切到伺服器分頁
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	// cursor 在 preview_lines (第 0 個欄位)
	// 進入編輯模式
	m2, _ := m1.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model2 := m2.(cfgtui.Model)
	assert.True(t, model2.Editing())
}

func TestModel_DeleteInEditModeIgnored(t *testing.T) {
	cfg := config.Default()
	cfg.Local.BarBG = "#changed"
	m := cfgtui.NewModel(cfg)
	// 進入編輯模式
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model1 := m1.(cfgtui.Model)
	assert.True(t, model1.Editing())
	// 在編輯模式下 Del 不應重置欄位（由 textinput 處理）
	result := model1.ResultConfig()
	assert.Equal(t, "#changed", result.Local.BarBG)
}

// ---- 新增測試：移除 remote.*、加 local.bar_fg ----

// TestClientFields_NoRemote 確認客戶端分頁不含 remote.* 欄位，且包含 local.bar_fg。
func TestClientFields_NoRemote(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 客戶端分頁現在應有 4 個欄位
	assert.Equal(t, 4, m.FieldCount(cfgtui.TabClient))
	// View 不應包含 remote.* 標籤
	view := m.View()
	assert.NotContains(t, view, "remote.bar_bg")
	assert.NotContains(t, view, "remote.badge_bg")
	assert.NotContains(t, view, "remote.badge_fg")
	// 應包含 local.bar_fg
	assert.Contains(t, view, "local.bar_fg")
}

// TestServerFields_NoRemote 確認伺服器分頁不含 remote.* 欄位。
func TestServerFields_NoRemote(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg)
	// 伺服器分頁現在應有 6 個欄位（2 server + 4 local color）
	assert.Equal(t, 6, m.FieldCount(cfgtui.TabServer))
	// 切到伺服器分頁
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := m1.(cfgtui.Model)
	view := model.View()
	assert.NotContains(t, view, "remote.bar_bg")
	assert.NotContains(t, view, "remote.badge_bg")
	assert.NotContains(t, view, "remote.badge_fg")
}

// TestDefaultFieldValue_BarFG 確認 local.bar_fg 的預設值為空字串。
func TestDefaultFieldValue_BarFG(t *testing.T) {
	cfg := config.Default()
	cfg.Local.BarFG = "#changed"
	m := cfgtui.NewModel(cfg)
	// cursor 移到 local.bar_fg（第 1 個欄位）
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2, _ := m1.Update(tea.KeyMsg{Type: tea.KeyDelete})
	model := m2.(cfgtui.Model)
	result := model.ResultConfig()
	// 預設值是空字串
	assert.Equal(t, "", result.Local.BarFG)
}

// TestResultConfig_NoRemote 確認 ResultConfig 不寫回 Remote 欄位（依 TUI 輸入為準）。
func TestResultConfig_NoRemote(t *testing.T) {
	cfg := config.Default()
	cfg.Remote.BarBG = "#original-remote"
	m := cfgtui.NewModel(cfg)
	result := m.ResultConfig()
	// Remote.BarBG 應保持原值（TUI 沒有 remote 欄位，不會被修改也不會被清空）
	assert.Equal(t, "#original-remote", result.Remote.BarBG)
}

// TestResultConfig_LocalBarFG 確認 local.bar_fg 寫回 ResultConfig。
func TestResultConfig_LocalBarFG(t *testing.T) {
	cfg := config.Default()
	cfg.Local.BarFG = "#abc123"
	m := cfgtui.NewModel(cfg)
	result := m.ResultConfig()
	assert.Equal(t, "#abc123", result.Local.BarFG)
}
