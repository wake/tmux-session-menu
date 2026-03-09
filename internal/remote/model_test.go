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
	assert.Equal(t, CountdownSec, m.Countdown())
}

func TestReconnectModel_Init_ReturnsCountdownTick(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	cmd := m.Init()
	assert.NotNil(t, cmd)
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
	updated, _ := m.Update(ReconnStateMsg{State: StateConnecting, Attempt: 1})
	rm := updated.(ReconnectModel)
	assert.Equal(t, StateConnecting, rm.State())
	assert.Equal(t, 1, rm.Attempt())
}

func TestReconnectModel_Connected_Quits(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	updated, cmd := m.Update(ReconnStateMsg{State: StateConnected})
	rm := updated.(ReconnectModel)
	assert.Equal(t, StateConnected, rm.State())
	assert.NotNil(t, cmd) // should be tea.Quit to return control
}

func TestReconnectModel_SessionGone_BackToMenu(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	updated, cmd := m.Update(ReconnSessionGoneMsg{})
	rm := updated.(ReconnectModel)
	assert.True(t, rm.BackToMenu())
	assert.NotNil(t, cmd)
}

// --- 倒數計時測試 ---

func TestReconnectModel_CountdownTick_Decrements(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	updated, cmd := m.Update(CountdownTickMsg{})
	rm := updated.(ReconnectModel)
	assert.Equal(t, CountdownSec-1, rm.Countdown())
	assert.NotNil(t, cmd) // 應繼續下一次 tick
}

func TestReconnectModel_CountdownZero_EntersAskContinue(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	m.countdown = 1
	updated, cmd := m.Update(CountdownTickMsg{})
	rm := updated.(ReconnectModel)
	assert.Equal(t, 0, rm.Countdown())
	assert.Equal(t, StateAskContinue, rm.State())
	assert.Nil(t, cmd) // 停止 tick
}

func TestReconnectModel_AskContinue_YRestartsCountdown(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	m.state = StateAskContinue
	m.countdown = 0
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	rm := updated.(ReconnectModel)
	assert.Equal(t, CountdownSec, rm.Countdown())
	assert.Equal(t, StateConnecting, rm.State())
	assert.NotNil(t, cmd) // 重啟 countdown tick
}

func TestReconnectModel_AskContinue_NQuits(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	m.state = StateAskContinue
	m.countdown = 0
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	rm := updated.(ReconnectModel)
	assert.True(t, rm.Quit())
	assert.NotNil(t, cmd) // tea.Quit
}

func TestReconnectModel_AskContinue_EscBackToMenu(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	m.state = StateAskContinue
	m.countdown = 0
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := updated.(ReconnectModel)
	assert.True(t, rm.BackToMenu())
	assert.NotNil(t, cmd) // tea.Quit
}

func TestReconnectModel_Connected_DuringAskContinue(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	m.state = StateAskContinue
	m.countdown = 0
	updated, cmd := m.Update(ReconnStateMsg{State: StateConnected})
	rm := updated.(ReconnectModel)
	assert.Equal(t, StateConnected, rm.State())
	assert.NotNil(t, cmd) // tea.Quit（自動接回）
}

func TestReconnectModel_AskContinue_IgnoresConnecting(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	m.state = StateAskContinue
	m.countdown = 0
	// 輪詢 goroutine 送來 StateConnecting 不應覆蓋 StateAskContinue
	updated, _ := m.Update(ReconnStateMsg{State: StateConnecting, Attempt: 5})
	rm := updated.(ReconnectModel)
	assert.Equal(t, StateAskContinue, rm.State(), "StateConnecting 不應覆蓋 StateAskContinue")
	assert.Equal(t, 5, rm.Attempt())
}

func TestReconnectModel_View_ShowsCountdown(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	m.state = StateConnecting
	view := m.View()
	assert.Contains(t, view, "60")
}

func TestReconnectModel_View_AskContinue_ShowsPrompt(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	m.state = StateAskContinue
	m.countdown = 0
	view := m.View()
	assert.Contains(t, view, "是否繼續")
}

func TestReconnectModel_View_HasBorder(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	m.width = 80
	m.height = 24
	view := m.View()
	assert.Contains(t, view, "╭")
}
