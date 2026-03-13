package ui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
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
	assert.Contains(t, view, "名稱")
	assert.Contains(t, view, "ID")
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

func TestNewSession_IDOnlyCreatesSession(t *testing.T) {
	// 只填 ID 不填名稱，應以 ID 為 tmux session name
	m := ui.NewModel(ui.Deps{})
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeNewSession, m.Mode())

	// 按 down 移到 ID 欄位
	m, _ = applyKey(m, "down")
	for _, ch := range "my-id" {
		m, _ = applyKey(m, string(ch))
	}

	// Enter 應觸發退出（但無 TmuxMgr 也無 Client，所以只驗證 quitting）
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)
	assert.True(t, model.Quitting())
	assert.Equal(t, "my-id", model.Selected())
}

func TestNewSession_InvalidNameChars(t *testing.T) {
	// session name 含 '.' 或 ':' 應報錯
	m := ui.NewModel(ui.Deps{})
	m, _ = applyKey(m, "n")
	for _, ch := range "bad.name" {
		m, _ = applyKey(m, string(ch))
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)
	assert.NotNil(t, model.Err())
	assert.Contains(t, model.Err().Error(), "'.' 或 ':'")
}

func TestNewSession_HubModeHostTabsFromSnapshot(t *testing.T) {
	// Hub 模式下應從 hubHostSnap 提取主機 tab
	tsmv1Snap := &tsmv1.MultiHostSnapshot{
		Hosts: []*tsmv1.HostState{
			{HostId: "air", Name: "air", Color: "#5f8787", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
			{HostId: "mlab", Name: "mlab", Color: "#73daca", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
		},
	}

	m := ui.NewModel(ui.Deps{HubMode: true})
	// 透過 HubSnapshotMsg 設定 hubHostSnap
	m2, _ := m.Update(ui.HubSnapshotMsg{Snapshot: tsmv1Snap})
	m = m2.(ui.Model)

	m, _ = applyKey(m, "n")
	view := m.View()
	assert.Contains(t, view, "air")
	assert.Contains(t, view, "mlab")
	assert.Contains(t, view, "◄ ► 切換") // 多主機才有
}

func TestNewSession_HubModeFiltersDisconnectedHosts(t *testing.T) {
	// Hub 模式下 DISCONNECTED 的主機不應出現在 new session tab 中
	// 只有一台 CONNECTED → 不顯示主機切換提示
	tsmv1Snap := &tsmv1.MultiHostSnapshot{
		Hosts: []*tsmv1.HostState{
			{HostId: "air", Name: "air", Color: "#5f8787", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
			{HostId: "mlab", Name: "mlab", Color: "#73daca", Status: tsmv1.HostStatus_HOST_STATUS_DISCONNECTED},
		},
	}

	m := ui.NewModel(ui.Deps{HubMode: true})
	m2, _ := m.Update(ui.HubSnapshotMsg{Snapshot: tsmv1Snap})
	m = m2.(ui.Model)

	m, _ = applyKey(m, "n")
	view := m.View()
	// 只有一台可用主機，不應出現切換提示（表示 tab 區域未渲染多主機切換）
	assert.NotContains(t, view, "◄ ► 切換")
}

func TestNewSession_HostTabsShownForMultiHost(t *testing.T) {
	// 多主機模式下應顯示主機 tab
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#00ff00", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "remote-1", Color: "#ff0000", Enabled: true})
	m := ui.NewModel(ui.Deps{HostMgr: mgr})
	m, _ = applyKey(m, "n")
	view := m.View()
	assert.Contains(t, view, "local")
	assert.Contains(t, view, "remote-1")
}

func TestNewSession_SingleHostNoTabs(t *testing.T) {
	// 單主機模式不應顯示 tab
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#00ff00", Enabled: true})
	m := ui.NewModel(ui.Deps{HostMgr: mgr})
	m, _ = applyKey(m, "n")
	view := m.View()
	// 只有一台主機，不應出現 "◄ ► 切換" 提示
	assert.NotContains(t, view, "◄ ► 切換")
}
