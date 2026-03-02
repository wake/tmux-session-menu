package ui_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/ui"
)

func TestModel_Init_WithDeps(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	cmd := m.Init()
	assert.NotNil(t, cmd)
}

func TestModel_View_ShowsHeader(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	view := m.View()
	assert.Contains(t, view, "tmux session menu")
}

func TestModel_Quit(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	_ = updated
	assert.NotNil(t, cmd)
}

func TestModel_Navigation(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession},
		{Type: ui.ItemSession},
		{Type: ui.ItemSession},
	})

	assert.Equal(t, 0, m.Cursor())

	m, _ = applyKey(m, "j")
	assert.Equal(t, 1, m.Cursor())

	m, _ = applyKey(m, "k")
	assert.Equal(t, 0, m.Cursor())

	// Can't go past top
	m, _ = applyKey(m, "k")
	assert.Equal(t, 0, m.Cursor())

	// Can't go past bottom
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	assert.Equal(t, 2, m.Cursor())
}

func TestModel_View_RendersSessions(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "dev"}},
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:     "my-project",
			Status:   tmux.StatusRunning,
			Activity: time.Now().Add(-3 * time.Minute),
			AIModel:  "claude-sonnet-4-6",
		}},
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:     "api-server",
			Status:   tmux.StatusIdle,
			Activity: time.Now().Add(-2 * time.Hour),
		}},
	})

	view := m.View()

	assert.Contains(t, view, "dev")
	assert.Contains(t, view, "my-project")
	assert.Contains(t, view, "api-server")
	assert.Contains(t, view, "●") // running
	assert.Contains(t, view, "○") // idle
	assert.Contains(t, view, "claude-sonnet-4-6")
}

func TestModel_View_Preview(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:      "my-project",
			AISummary: "正在重構 auth 模組",
		}},
	})

	view := m.View()
	assert.Contains(t, view, "正在重構 auth 模組")
}

func TestModel_SessionsMsg_PopulatesItems(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	msg := ui.SessionsMsg{
		Sessions: []tmux.Session{
			{Name: "alpha", Activity: time.Now()},
			{Name: "beta", Activity: time.Now()},
		},
	}

	updated, _ := m.Update(msg)
	model := updated.(ui.Model)

	assert.Equal(t, 2, len(model.Items()))
	assert.Equal(t, "alpha", model.Items()[0].Session.Name)
}

func TestModel_Init_TriggersLoad(t *testing.T) {
	mockExec := &mockExecutor{
		output: "dev:$1:1:/home:0:1700000000\nops:$2:1:/tmp:0:1700000000\n",
	}
	mgr := tmux.NewManager(mockExec)
	m := ui.NewModel(ui.Deps{TmuxMgr: mgr})

	cmd := m.Init()
	assert.NotNil(t, cmd)

	// Execute the batch and collect messages
	result := cmd()
	// tea.Batch returns a BatchMsg ([]tea.Cmd)
	// We need to find and execute the loadSessionsCmd among them
	batchMsg, ok := result.(tea.BatchMsg)
	if ok {
		for _, c := range batchMsg {
			if c == nil {
				continue
			}
			msg := c()
			if sessMsg, ok := msg.(ui.SessionsMsg); ok {
				assert.Nil(t, sessMsg.Err)
				assert.Equal(t, 2, len(sessMsg.Sessions))
				assert.Equal(t, "dev", sessMsg.Sessions[0].Name)
				return
			}
		}
		t.Fatal("no SessionsMsg found in batch")
	}
	// If not a batch, check direct
	sessMsg, ok := result.(ui.SessionsMsg)
	assert.True(t, ok)
	assert.Equal(t, 2, len(sessMsg.Sessions))
}

type mockExecutor struct {
	output string
	err    error
}

func (e *mockExecutor) Execute(args ...string) (string, error) {
	return e.output, e.err
}

func TestModel_Enter_SelectsSession(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "target"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "other"}},
	})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	assert.Equal(t, "target", model.Selected())
	assert.NotNil(t, cmd) // should return tea.Quit
}

func TestModel_Enter_OnGroup_DoesNotSelect(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "dev"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "target"}},
	})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	assert.Equal(t, "", model.Selected())
	assert.Nil(t, cmd) // should NOT quit
}

func applyKey(m ui.Model, key string) (ui.Model, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	updated, cmd := m.Update(msg)
	return updated.(ui.Model), cmd
}
