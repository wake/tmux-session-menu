package launcher

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func applyKey(m Model, key string) (Model, tea.Cmd) {
	var msg tea.KeyMsg
	switch key {
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func TestNewModel_DefaultTab(t *testing.T) {
	m := NewModel("")
	assert.Equal(t, 0, m.ActiveTab())
	assert.Equal(t, ChoiceNone, m.Choice())
}

func TestNewModel_WithPlaceholder(t *testing.T) {
	m := NewModel("myserver")
	assert.Equal(t, "myserver", m.Placeholder())
}

func TestLeftRightSwitchTabs(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right")
	assert.Equal(t, 1, m.ActiveTab())

	m, _ = applyKey(m, "right")
	assert.Equal(t, 2, m.ActiveTab())

	// 到底不超出
	m, _ = applyKey(m, "right")
	assert.Equal(t, 2, m.ActiveTab())

	m, _ = applyKey(m, "left")
	assert.Equal(t, 1, m.ActiveTab())

	m, _ = applyKey(m, "left")
	assert.Equal(t, 0, m.ActiveTab())

	// 到頂不超出
	m, _ = applyKey(m, "left")
	assert.Equal(t, 0, m.ActiveTab())
}

func TestEnterOnRemoteTab_WithHost(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "s")
	m, _ = applyKey(m, "v")
	m, _ = applyKey(m, "r")
	m, cmd := applyKey(m, "enter")
	assert.Equal(t, ChoiceRemote, m.Choice())
	assert.Equal(t, "svr", m.Host())
	assert.NotNil(t, cmd)
}

func TestEnterOnRemoteTab_Empty_NoAction(t *testing.T) {
	m := NewModel("")
	m, cmd := applyKey(m, "enter")
	assert.Equal(t, ChoiceNone, m.Choice())
	assert.Nil(t, cmd)
}

func TestTabAutoFillsPlaceholder(t *testing.T) {
	m := NewModel("myserver")
	m, _ = applyKey(m, "tab")
	assert.Equal(t, "myserver", m.Host())
}

func TestTabNoPlaceholder_NoEffect(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "tab")
	assert.Equal(t, "", m.Host())
}

func TestBackspaceDeletesChar(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "a")
	m, _ = applyKey(m, "b")
	assert.Equal(t, "ab", m.Host())
	m, _ = applyKey(m, "backspace")
	assert.Equal(t, "a", m.Host())
}

func TestEnterOnLocalTab(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right") // tab 1
	m, cmd := applyKey(m, "enter")
	assert.Equal(t, ChoiceLocal, m.Choice())
	assert.NotNil(t, cmd)
}

func TestEnterOnFullSetupTab(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right") // tab 1
	m, _ = applyKey(m, "right") // tab 2
	m, cmd := applyKey(m, "enter")
	assert.Equal(t, ChoiceFullSetup, m.Choice())
	assert.NotNil(t, cmd)
}

func TestViewShowsAllTabs(t *testing.T) {
	m := NewModel("")
	view := m.View()
	assert.Contains(t, view, "連線遠端")
	assert.Contains(t, view, "簡易本地端")
	assert.Contains(t, view, "完整設定")
}

func TestViewRemoteTabShowsInput(t *testing.T) {
	m := NewModel("lasthost")
	view := m.View()
	assert.Contains(t, view, "SSH 主機")
	assert.Contains(t, view, "lasthost")
}

func TestViewLocalTabShowsDescription(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right")
	view := m.View()
	assert.Contains(t, view, "不啟用 daemon")
}

func TestViewFullSetupTabShowsDescription(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right")
	m, _ = applyKey(m, "right")
	view := m.View()
	assert.Contains(t, view, "hook")
}

func TestQuitReturnsNone(t *testing.T) {
	m := NewModel("")
	m, cmd := applyKey(m, "q")
	assert.Equal(t, ChoiceNone, m.Choice())
	assert.NotNil(t, cmd)
}

func TestTypingOnlyOnRemoteTab(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right") // tab 1 (local)
	m, _ = applyKey(m, "x")
	assert.Equal(t, "", m.Host(), "非遠端 tab 不應接受文字輸入")
}
