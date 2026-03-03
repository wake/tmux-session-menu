package setup

import (
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
