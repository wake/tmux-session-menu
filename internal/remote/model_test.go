package remote

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func applyKeyToReconn(m ReconnectModel, key string) ReconnectModel {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(ReconnectModel)
}

func TestReconnectModel_InitialState(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	assert.Equal(t, StateDisconnected, m.State())
	assert.Equal(t, 0, m.Attempt())
	assert.False(t, m.Quit())
	assert.False(t, m.BackToMenu())
}

func TestReconnectModel_EscGoesBackToMenu(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := updated.(ReconnectModel)
	assert.True(t, rm.BackToMenu())
	assert.NotNil(t, cmd) // should be tea.Quit
}

func TestReconnectModel_QKeyQuits(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	rm := applyKeyToReconn(m, "q")
	assert.True(t, rm.Quit())
}

func TestReconnectModel_View_ContainsHost(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	view := m.View()
	assert.Contains(t, view, "user@host")
}

func TestReconnectModel_StateTransition(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	updated, _ := m.Update(reconnStateMsg{state: StateConnecting, attempt: 1})
	rm := updated.(ReconnectModel)
	assert.Equal(t, StateConnecting, rm.State())
	assert.Equal(t, 1, rm.Attempt())
}

func TestReconnectModel_Connected_Quits(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	updated, cmd := m.Update(reconnStateMsg{state: StateConnected})
	rm := updated.(ReconnectModel)
	assert.Equal(t, StateConnected, rm.State())
	assert.NotNil(t, cmd) // should be tea.Quit to return control
}

func TestReconnectModel_SessionGone_BackToMenu(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	updated, cmd := m.Update(reconnSessionGoneMsg{})
	rm := updated.(ReconnectModel)
	assert.True(t, rm.BackToMenu())
	assert.NotNil(t, cmd)
}
