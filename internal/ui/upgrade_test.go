package ui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
	"github.com/wake/tmux-session-menu/internal/ui"
	"github.com/wake/tmux-session-menu/internal/upgrade"
)

// setupUpgradeModel 快速構建進入 ModeUpgrade 狀態的 model（含 local + air-2019 兩台主機）。
func setupUpgradeModel(t *testing.T) ui.Model {
	t.Helper()
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Address: "", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "air-2019", Address: "air-2019", Enabled: true})
	deps := ui.Deps{HostMgr: mgr, Upgrader: &upgrade.Upgrader{}}
	m := ui.NewModel(deps)
	msg := ui.CheckUpgradeMsg{
		Release:      &upgrade.Release{Version: "0.28.0"},
		HostVersions: map[string]string{"local": "0.27.0", "air-2019": "0.26.0"},
	}
	result, _ := m.Update(msg)
	return result.(ui.Model)
}

func TestUpgrade_SpaceTogglesCheck(t *testing.T) {
	m := setupUpgradeModel(t)
	items := m.UpgradeItems()
	assert.True(t, items[0].Checked, "初始應已勾選")

	// 按 space 取消勾選
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = result.(ui.Model)
	assert.False(t, m.UpgradeItems()[0].Checked, "按 space 後應取消勾選")

	// 再按 space 重新勾選
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = result.(ui.Model)
	assert.True(t, m.UpgradeItems()[0].Checked, "再按 space 應重新勾選")
}

func TestUpgrade_CursorMovement(t *testing.T) {
	m := setupUpgradeModel(t)
	itemCount := len(m.UpgradeItems()) // 2
	assert.Equal(t, 0, m.UpgradeCursor())

	// j 下移
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(ui.Model)
	assert.Equal(t, 1, m.UpgradeCursor())

	// 再按 j 到按鈕區
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(ui.Model)
	assert.Equal(t, itemCount, m.UpgradeCursor())

	// 再按 j 不超出
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(ui.Model)
	assert.Equal(t, itemCount, m.UpgradeCursor(), "不應超出按鈕區")

	// k 上移
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(ui.Model)
	assert.Equal(t, itemCount-1, m.UpgradeCursor())

	// 持續按 k 回到頂部
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(ui.Model)
	assert.Equal(t, 0, m.UpgradeCursor())

	// 再按 k 不低於 0
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(ui.Model)
	assert.Equal(t, 0, m.UpgradeCursor(), "不應低於 0")
}

func TestUpgrade_EscReturnToNormal(t *testing.T) {
	m := setupUpgradeModel(t)
	assert.Equal(t, ui.ModeUpgrade, m.Mode())

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = result.(ui.Model)
	assert.Equal(t, ui.ModeNormal, m.Mode())
}

func TestUpgrade_AToggleAll(t *testing.T) {
	m := setupUpgradeModel(t)
	// 初始：兩個都勾選
	for _, item := range m.UpgradeItems() {
		assert.True(t, item.Checked, "初始應勾選")
	}

	// 按 a → 全不選（因為目前全選）
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = result.(ui.Model)
	for _, item := range m.UpgradeItems() {
		assert.False(t, item.Checked, "全不選")
	}

	// 再按 a → 全選
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = result.(ui.Model)
	for _, item := range m.UpgradeItems() {
		assert.True(t, item.Checked, "全選")
	}
}

func TestUpgrade_CursorMovesToButtons(t *testing.T) {
	m := setupUpgradeModel(t)
	itemCount := len(m.UpgradeItems()) // 2

	// 按 down 兩次到按鈕區
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = result.(ui.Model)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = result.(ui.Model)
	assert.Equal(t, itemCount, m.UpgradeCursor(), "游標應在按鈕區")
}

func TestUpgrade_ButtonFocusNavigation(t *testing.T) {
	m := setupUpgradeModel(t)
	itemCount := len(m.UpgradeItems())

	// 移到按鈕區
	for i := 0; i < itemCount; i++ {
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = result.(ui.Model)
	}
	assert.Equal(t, itemCount, m.UpgradeCursor())
	assert.Equal(t, 0, m.UpgradeBtnFocus(), "預設焦點在升級按鈕")

	// l 切到取消按鈕
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = result.(ui.Model)
	assert.Equal(t, 1, m.UpgradeBtnFocus(), "焦點應在取消按鈕")

	// 再按 l 不超出
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = result.(ui.Model)
	assert.Equal(t, 1, m.UpgradeBtnFocus(), "焦點不應超出")

	// h 切回升級按鈕
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = result.(ui.Model)
	assert.Equal(t, 0, m.UpgradeBtnFocus(), "焦點應回升級按鈕")

	// tab 切到取消
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(ui.Model)
	assert.Equal(t, 1, m.UpgradeBtnFocus(), "tab 應切到取消")

	// shift+tab 切回升級
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(ui.Model)
	assert.Equal(t, 0, m.UpgradeBtnFocus(), "shift+tab 應切回升級")

	// 在 item 區域按 l 不應影響 btnFocus
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}) // 回到 item 區
	m = result.(ui.Model)
	assert.True(t, m.UpgradeCursor() < itemCount)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = result.(ui.Model)
	assert.Equal(t, 0, m.UpgradeBtnFocus(), "在 item 區按 l 不應改變 btnFocus")
}

func TestUpgrade_EnterOnUpgradeButton(t *testing.T) {
	m := setupUpgradeModel(t)
	itemCount := len(m.UpgradeItems())

	// 移到按鈕區（升級按鈕，btnFocus=0）
	for i := 0; i < itemCount; i++ {
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = result.(ui.Model)
	}
	assert.Equal(t, itemCount, m.UpgradeCursor())
	assert.Equal(t, 0, m.UpgradeBtnFocus())

	// 按 Enter 觸發升級
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(ui.Model)
	assert.True(t, m.UpgradeRunning(), "按升級按鈕應啟動升級")
}

func TestUpgrade_EnterOnCancelButton(t *testing.T) {
	m := setupUpgradeModel(t)
	itemCount := len(m.UpgradeItems())

	// 移到按鈕區
	for i := 0; i < itemCount; i++ {
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = result.(ui.Model)
	}
	// 切到取消按鈕
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = result.(ui.Model)
	assert.Equal(t, 1, m.UpgradeBtnFocus())

	// 按 Enter 應返回 ModeNormal
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(ui.Model)
	assert.Equal(t, ui.ModeNormal, m.Mode(), "取消按鈕應返回 ModeNormal")
}

func TestUpgrade_CtrlUStartsUpgrade(t *testing.T) {
	m := setupUpgradeModel(t)
	assert.False(t, m.UpgradeRunning())

	// ctrl+u 直接觸發升級
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = result.(ui.Model)
	assert.True(t, m.UpgradeRunning(), "ctrl+u 應啟動升級")

	// 遠端項目應標記為 UpgradeRunning_
	items := m.UpgradeItems()
	for _, item := range items {
		if !item.IsLocal && item.Checked {
			assert.Equal(t, ui.UpgradeRunning_, item.Status, "遠端已勾選項目應標記為 UpgradeRunning_")
		}
	}
}

func TestUpgrade_LockedDuringRunning(t *testing.T) {
	m := setupUpgradeModel(t)

	// 觸發升級
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = result.(ui.Model)
	assert.True(t, m.UpgradeRunning())

	// 記錄目前游標
	cursor := m.UpgradeCursor()

	// j 應被鎖定
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(ui.Model)
	assert.Equal(t, cursor, m.UpgradeCursor(), "running 時 j 應被鎖定")

	// k 應被鎖定
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(ui.Model)
	assert.Equal(t, cursor, m.UpgradeCursor(), "running 時 k 應被鎖定")

	// space 應被鎖定
	checkedBefore := m.UpgradeItems()[0].Checked
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = result.(ui.Model)
	assert.Equal(t, checkedBefore, m.UpgradeItems()[0].Checked, "running 時 space 應被鎖定")

	// 模式不應改變（非 Esc 按鍵）
	assert.Equal(t, ui.ModeUpgrade, m.Mode(), "running 時模式不應改變")

	// Esc 應設定 cancelled
	assert.False(t, m.UpgradeCancelled())
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = result.(ui.Model)
	assert.True(t, m.UpgradeCancelled(), "running 時 Esc 應設定 cancelled")
	assert.Equal(t, ui.ModeUpgrade, m.Mode(), "running 時 Esc 不應離開 ModeUpgrade")
}
