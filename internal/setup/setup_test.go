package setup

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testComponents() []Component {
	return []Component{
		{
			Label:       "Component A",
			InstallFn:   func() (string, error) { return "installed A", nil },
			UninstallFn: func() (string, error) { return "uninstalled A", nil },
			Checked:     true,
		},
		{
			Label:       "Component B",
			InstallFn:   func() (string, error) { return "installed B", nil },
			UninstallFn: func() (string, error) { return "uninstalled B", nil },
			Checked:     true,
		},
	}
}

func TestViewShowsAllComponents(t *testing.T) {
	m := NewModel(testComponents())
	view := m.View()
	assert.Contains(t, view, "Component A")
	assert.Contains(t, view, "Component B")
	assert.Contains(t, view, "tsm setup")
}

func TestDefaultAllChecked(t *testing.T) {
	m := NewModel(testComponents())
	checked := m.Checked()
	assert.True(t, checked[0])
	assert.True(t, checked[1])

	view := m.View()
	assert.Contains(t, view, "[x]")
}

func TestSpaceTogglesCheck(t *testing.T) {
	m := NewModel(testComponents())

	// cursor 在第 0 項，按 Space 取消勾選
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(Model)
	checked := m.Checked()
	assert.False(t, checked[0], "第 0 項應該被取消勾選")
	assert.True(t, checked[1], "第 1 項應該維持勾選")

	// 再按一次 Space 恢復勾選
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(Model)
	checked = m.Checked()
	assert.True(t, checked[0], "第 0 項應該恢復勾選")
}

func TestEnterExecutesAndShowsResults(t *testing.T) {
	m := NewModel(testComponents())

	// 按 Enter 開始安裝
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	assert.Equal(t, phaseRunning, m.Phase())

	// 執行 installMsg
	require.NotNil(t, cmd)
	msg := cmd()
	updated, cmd = m.Update(msg)
	m = updated.(Model)

	// 此時應該在 phaseRunning，觸發 resultMsg
	require.NotNil(t, cmd)
	msg = cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)

	assert.Equal(t, phaseDone, m.Phase())
	results := m.Results()
	assert.Len(t, results, 2)
	assert.Nil(t, results[0].err)
	assert.Equal(t, "installed A", results[0].message)

	view := m.View()
	assert.Contains(t, view, "✓")
	assert.Contains(t, view, "Component A")
}

func TestQuitCancels(t *testing.T) {
	m := NewModel(testComponents())

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(Model)
	assert.NotNil(t, cmd, "q 應該觸發 tea.Quit")
	assert.True(t, m.quitting)
}

func TestNavigationUpDown(t *testing.T) {
	m := NewModel(testComponents())
	assert.Equal(t, 0, m.cursor)

	// 按 j 向下
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	assert.Equal(t, 1, m.cursor)

	// 按 k 向上
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(Model)
	assert.Equal(t, 0, m.cursor)

	// 已在最上方，再按 k 不會超出範圍
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(Model)
	assert.Equal(t, 0, m.cursor)
}

func TestDisabledCannotToggle(t *testing.T) {
	comps := []Component{
		{
			Label:    "Disabled Item",
			Checked:  false,
			Disabled: true,
			InstallFn: func() (string, error) { return "", nil },
		},
	}
	m := NewModel(comps)
	assert.False(t, m.Checked()[0])

	// 按 Space 不應改變勾選狀態
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(Model)
	assert.False(t, m.Checked()[0], "Disabled 元件不應被切換")
}

func TestInitialCheckedState(t *testing.T) {
	comps := []Component{
		{Label: "Checked", Checked: true, InstallFn: func() (string, error) { return "", nil }},
		{Label: "Unchecked", Checked: false, InstallFn: func() (string, error) { return "", nil }},
	}
	m := NewModel(comps)
	checked := m.Checked()
	assert.True(t, checked[0], "Checked=true 元件應初始勾選")
	assert.False(t, checked[1], "Checked=false 元件應初始未勾選")
}

func TestDisabledView(t *testing.T) {
	comps := []Component{
		{
			Label:    "Conflict Item",
			Disabled: true,
			Note:     "路徑衝突",
			InstallFn: func() (string, error) { return "", nil },
		},
	}
	m := NewModel(comps)
	view := m.View()
	assert.Contains(t, view, "[-]")
	assert.Contains(t, view, "Conflict Item")
	assert.Contains(t, view, "路徑衝突")
}

func TestRunInstallSkipsDisabled(t *testing.T) {
	called := false
	comps := []Component{
		{
			Label:   "Disabled",
			Checked: true,
			Disabled: true,
			InstallFn: func() (string, error) {
				called = true
				return "should not run", nil
			},
		},
		{
			Label:   "Enabled",
			Checked: true,
			InstallFn: func() (string, error) { return "installed", nil },
		},
	}
	m := NewModel(comps)

	// 按 Enter 開始安裝
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	require.NotNil(t, cmd)

	msg := cmd()
	updated, cmd = m.Update(msg)
	m = updated.(Model)
	require.NotNil(t, cmd)

	msg = cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)

	assert.False(t, called, "Disabled 元件的 InstallFn 不應被呼叫")
	results := m.Results()
	assert.Len(t, results, 1)
	assert.Equal(t, "Enabled", results[0].label)
}

func TestNoteRendered(t *testing.T) {
	comps := []Component{
		{
			Label:   "With Note",
			Checked: true,
			Note:    "這是附加說明",
			InstallFn: func() (string, error) { return "", nil },
		},
	}
	m := NewModel(comps)
	view := m.View()
	assert.Contains(t, view, "這是附加說明")
}

// driveToPhraseDone 驅動 model 到 phaseDone 狀態。
func driveToPhaseDone(t *testing.T, m Model) Model {
	t.Helper()
	// Enter → phaseRunning
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	require.NotNil(t, cmd)

	// installMsg → runInstall
	msg := cmd()
	updated, cmd = m.Update(msg)
	m = updated.(Model)
	require.NotNil(t, cmd)

	// resultMsg → phaseDone
	msg = cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	require.Equal(t, phaseDone, m.Phase())
	return m
}

func TestRestartPromptY(t *testing.T) {
	called := false
	m := NewModel(testComponents())
	m.SetDaemonHint("daemon 需要重新啟動")
	m.SetRestartFn(func() error {
		called = true
		return nil
	})

	m = driveToPhaseDone(t, m)

	// phaseDone 時 View 應顯示 (Y/n) 提示
	view := m.View()
	assert.Contains(t, view, "重新啟動？(Y/n)")
	assert.NotContains(t, view, "✓ daemon 已重新啟動")

	// 按 Y → 執行 restartFn 並自動退出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	m = updated.(Model)
	assert.NotNil(t, cmd, "應觸發 tea.Quit")
	assert.True(t, called, "restartFn 應被呼叫")
	assert.True(t, m.quitting)

	// 最終 View 應顯示成功結果
	view = m.View()
	assert.Contains(t, view, "✓ daemon 已重新啟動")
	assert.NotContains(t, view, "(Y/n)")
}

func TestRestartPromptEnter(t *testing.T) {
	called := false
	m := NewModel(testComponents())
	m.SetDaemonHint("daemon 需要重新啟動")
	m.SetRestartFn(func() error {
		called = true
		return nil
	})

	m = driveToPhaseDone(t, m)

	// 按 Enter → 也應觸發重啟並自動退出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	assert.True(t, called)
	assert.NotNil(t, cmd, "應觸發 tea.Quit")
	assert.True(t, m.quitting)
	assert.Contains(t, m.View(), "✓ daemon 已重新啟動")
}

func TestRestartPromptN(t *testing.T) {
	called := false
	m := NewModel(testComponents())
	m.SetDaemonHint("daemon 需要重新啟動")
	m.SetRestartFn(func() error {
		called = true
		return nil
	})

	m = driveToPhaseDone(t, m)

	// 按 n → 直接退出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	assert.False(t, called, "restartFn 不應被呼叫")
	assert.NotNil(t, cmd, "應觸發 tea.Quit")
	assert.True(t, m.quitting)
}

// --- InstallMode 模式選擇器測試 ---

func fullClientComponents() []Component {
	return []Component{
		{
			Label:     "安裝 tsm binary",
			InstallFn: func() (string, error) { return "installed", nil },
			Checked:   true,
		},
		{
			Label:     "安裝 tmux 快捷鍵",
			InstallFn: func() (string, error) { return "installed", nil },
			Checked:   true,
			FullOnly:  true,
		},
		{
			Label:     "安裝 hooks",
			InstallFn: func() (string, error) { return "installed", nil },
			Checked:   true,
			FullOnly:  true,
		},
	}
}

func TestInstallMode_DefaultFull(t *testing.T) {
	m := NewModel(fullClientComponents())
	assert.Equal(t, ModeFull, m.InstallMode())
}

func TestInstallMode_SetInitialMode(t *testing.T) {
	m := NewModel(fullClientComponents())
	m.SetInstallMode(ModeClient)
	assert.Equal(t, ModeClient, m.InstallMode())
}

func TestInstallMode_RightArrowToggles(t *testing.T) {
	m := NewModel(fullClientComponents())
	assert.Equal(t, ModeFull, m.InstallMode())

	// → 切到 client
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	assert.Equal(t, ModeClient, m.InstallMode())

	// → 再按一次回到 full
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	assert.Equal(t, ModeFull, m.InstallMode())
}

func TestInstallMode_LeftArrowToggles(t *testing.T) {
	m := NewModel(fullClientComponents())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(Model)
	assert.Equal(t, ModeClient, m.InstallMode())
}

func TestInstallMode_ViewShowsMode(t *testing.T) {
	m := NewModel(fullClientComponents())
	view := m.View()
	assert.Contains(t, view, "完整模式")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	view = m.View()
	assert.Contains(t, view, "純客戶端")
}

func TestInstallMode_ClientDisablesFullOnly(t *testing.T) {
	m := NewModel(fullClientComponents())

	// 切到 client 模式
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)

	checked := m.Checked()
	assert.True(t, checked[0], "binary 仍應勾選")
	assert.False(t, checked[1], "FullOnly 元件應取消勾選")
	assert.False(t, checked[2], "FullOnly 元件應取消勾選")
}

func TestInstallMode_BackToFullRestoresChecked(t *testing.T) {
	m := NewModel(fullClientComponents())

	// → client
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)

	// → full
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)

	checked := m.Checked()
	assert.True(t, checked[0])
	assert.True(t, checked[1], "FullOnly 元件切回 full 應恢復勾選")
	assert.True(t, checked[2], "FullOnly 元件切回 full 應恢復勾選")
}

func TestInstallMode_ClientViewShowsDisabled(t *testing.T) {
	m := NewModel(fullClientComponents())

	// 切到 client
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)

	view := m.View()
	// FullOnly 元件在 client 模式應以 [-] 顯示
	assert.Contains(t, view, "[-]")
}

func TestRestartPromptError(t *testing.T) {
	m := NewModel(testComponents())
	m.SetDaemonHint("daemon 需要重新啟動")
	m.SetRestartFn(func() error {
		return fmt.Errorf("connection refused")
	})

	m = driveToPhaseDone(t, m)

	// 按 Y → restartFn 回傳 error，仍自動退出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	m = updated.(Model)
	assert.NotNil(t, cmd, "應觸發 tea.Quit")
	assert.True(t, m.quitting)

	view := m.View()
	assert.Contains(t, view, "✗ daemon 重啟失敗: connection refused")
	assert.NotContains(t, view, "(Y/n)")
}
