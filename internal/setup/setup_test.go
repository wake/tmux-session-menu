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
		},
		{
			Label:       "Component B",
			InstallFn:   func() (string, error) { return "installed B", nil },
			UninstallFn: func() (string, error) { return "uninstalled B", nil },
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
