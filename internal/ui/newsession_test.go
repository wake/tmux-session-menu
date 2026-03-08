package ui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/ui"
)

func TestNewSession_EnterMode(t *testing.T) {
	cfg := config.Default()
	cfg.Agents = []config.AgentEntry{
		{Name: "Claude", Command: "claude", Group: "Coding", Enabled: true},
	}
	m := ui.NewModel(ui.Deps{Cfg: cfg})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	fm := updated.(ui.Model)
	assert.Equal(t, ui.ModeNewSession, fm.Mode())
}

func TestNewSession_EscCancels(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	fm := m3.(ui.Model)
	assert.Equal(t, ui.ModeNormal, fm.Mode())
}

func TestNewSession_EmptyNameNoOp(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	// Press Enter with empty name
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := m3.(ui.Model)
	// Should stay in ModeNewSession
	assert.Equal(t, ui.ModeNewSession, fm.Mode())
}

func TestNewSession_ViewRenders(t *testing.T) {
	cfg := config.Default()
	cfg.Agents = []config.AgentEntry{
		{Name: "Claude Code", Command: "claude", Group: "Coding", Enabled: true},
		{Name: "Codex", Command: "codex", Group: "Other", Enabled: true},
	}
	m := ui.NewModel(ui.Deps{Cfg: cfg})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	fm := m2.(ui.Model)
	view := fm.View()
	assert.Contains(t, view, "Session 名稱")
	assert.Contains(t, view, "啟動路徑")
	assert.Contains(t, view, "啟動 Agent")
	assert.Contains(t, view, "Shell")
	assert.Contains(t, view, "Claude Code")
	assert.Contains(t, view, "Codex")
}

func TestNewSession_DisabledAgentHidden(t *testing.T) {
	cfg := config.Default()
	cfg.Agents = []config.AgentEntry{
		{Name: "Claude Code", Command: "claude", Group: "Coding", Enabled: true},
		{Name: "Disabled", Command: "disabled", Group: "Other", Enabled: false},
	}
	m := ui.NewModel(ui.Deps{Cfg: cfg})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	fm := m2.(ui.Model)
	view := fm.View()
	assert.Contains(t, view, "Claude Code")
	assert.NotContains(t, view, "Disabled")
}
