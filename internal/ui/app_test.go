package ui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/config"
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

func TestModel_LoadSessions_WithHookStatus(t *testing.T) {
	// 建立暫時 status 目錄
	statusDir := t.TempDir()
	statusFile := filepath.Join(statusDir, "dev")
	data := []byte(`{"status":"running","timestamp":` + fmt.Sprintf("%d", time.Now().Unix()) + `,"event":"UserPromptSubmit"}`)
	os.WriteFile(statusFile, data, 0o644)

	mockExec := &mockExecutor{
		output: "dev:$1:1:/home:0:1700000000\n",
	}
	mgr := tmux.NewManager(mockExec)
	m := ui.NewModel(ui.Deps{TmuxMgr: mgr, StatusDir: statusDir})

	cmd := m.Init()
	msg := cmd()
	// Handle tea.Batch — drill into BatchMsg to find SessionsMsg
	batchMsg, ok := msg.(tea.BatchMsg)
	if ok {
		for _, c := range batchMsg {
			if c == nil {
				continue
			}
			inner := c()
			if sessMsg, ok := inner.(ui.SessionsMsg); ok {
				assert.Equal(t, tmux.StatusRunning, sessMsg.Sessions[0].Status)
				return
			}
		}
		t.Fatal("no SessionsMsg found in batch")
	}
	sessMsg, ok := msg.(ui.SessionsMsg)
	assert.True(t, ok)
	assert.Equal(t, tmux.StatusRunning, sessMsg.Sessions[0].Status)
}

func TestModel_Tick_ReloadsSessions(t *testing.T) {
	mockExec := &mockExecutor{
		output: "dev:$1:1:/home:0:1700000000\n",
	}
	mgr := tmux.NewManager(mockExec)

	m := ui.NewModel(ui.Deps{
		TmuxMgr: mgr,
		Cfg:     config.Config{PollIntervalSec: 2},
	})

	// 模擬 tickMsg
	updated, cmd := m.Update(ui.TickMsg{})
	_ = updated

	// tickMsg 應產生新的 Cmd（loadSessions + 下一個 tick）
	assert.NotNil(t, cmd)
}

func TestModel_LoadSessions_WithStore(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("dev", 0)
	groups, _ := st.ListGroups()
	st.SetSessionGroup("alpha", groups[0].ID, 0)

	mockExec := &mockExecutor{
		output: "alpha:$1:1:/home:0:1700000000\nbeta:$2:1:/tmp:0:1700000000\n",
	}
	mgr := tmux.NewManager(mockExec)
	m := ui.NewModel(ui.Deps{TmuxMgr: mgr, Store: st})

	cmd := m.Init()
	msg := cmd()
	// Drill into BatchMsg to find SessionsMsg
	batchMsg, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatal("expected BatchMsg")
	}
	var sessMsg ui.SessionsMsg
	found := false
	for _, c := range batchMsg {
		if c == nil {
			continue
		}
		inner := c()
		if sm, ok := inner.(ui.SessionsMsg); ok {
			sessMsg = sm
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no SessionsMsg found in batch")
	}

	assert.Len(t, sessMsg.Groups, 1)
	assert.Equal(t, "dev", sessMsg.Groups[0].Name)

	// alpha should have GroupName "dev"
	var alphaGroup string
	for _, s := range sessMsg.Sessions {
		if s.Name == "alpha" {
			alphaGroup = s.GroupName
		}
	}
	assert.Equal(t, "dev", alphaGroup)

	// beta should have no group
	var betaGroup string
	for _, s := range sessMsg.Sessions {
		if s.Name == "beta" {
			betaGroup = s.GroupName
		}
	}
	assert.Equal(t, "", betaGroup)
}

func TestModel_Tab_TogglesGroupCollapse(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("dev", 0)

	m := ui.NewModel(ui.Deps{Store: st})
	groups, _ := st.ListGroups()
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha", GroupName: "dev"}},
	})

	// cursor 在群組上，按 Tab
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_ = updated
	// 應觸發重新載入（cmd 不為 nil）
	assert.NotNil(t, cmd)

	// 驗證 store 中 collapsed 已切換
	groups, _ = st.ListGroups()
	assert.True(t, groups[0].Collapsed)
}

// recordingExecutor 記錄每次呼叫的參數，並根據指令回傳不同輸出。
type recordingExecutor struct {
	calls [][]string
	// listSessionsOutput 是 list-sessions 指令的回傳值
	listSessionsOutput string
	// capturePaneOutput 是 capture-pane 指令的回傳值
	capturePaneOutput string
}

func (e *recordingExecutor) Execute(args ...string) (string, error) {
	e.calls = append(e.calls, args)
	if len(args) > 0 {
		switch args[0] {
		case "list-sessions":
			return e.listSessionsOutput, nil
		case "capture-pane":
			return e.capturePaneOutput, nil
		}
	}
	return "", nil
}

// findSessionsMsg 從 Init() 回傳的 BatchMsg 中找出 SessionsMsg。
func findSessionsMsg(t *testing.T, cmd tea.Cmd) ui.SessionsMsg {
	t.Helper()
	result := cmd()
	batchMsg, ok := result.(tea.BatchMsg)
	if !ok {
		t.Fatal("預期為 BatchMsg")
	}
	for _, c := range batchMsg {
		if c == nil {
			continue
		}
		inner := c()
		if sm, ok := inner.(ui.SessionsMsg); ok {
			return sm
		}
	}
	t.Fatal("BatchMsg 中找不到 SessionsMsg")
	return ui.SessionsMsg{}
}

func TestLoadSessions_DetectsAIModel(t *testing.T) {
	rec := &recordingExecutor{
		listSessionsOutput: "dev:$1:1:/home:0:1700000000\n",
		capturePaneOutput:  "Using claude-sonnet-4-6 model\n> some prompt",
	}
	mgr := tmux.NewManager(rec)
	m := ui.NewModel(ui.Deps{
		TmuxMgr: mgr,
		Cfg:     config.Config{PreviewLines: 100},
	})

	cmd := m.Init()
	sessMsg := findSessionsMsg(t, cmd)

	assert.Nil(t, sessMsg.Err)
	assert.Equal(t, 1, len(sessMsg.Sessions))
	// 驗證 AI 模型從 pane 內容偵測出來
	assert.Equal(t, "claude-sonnet-4-6", sessMsg.Sessions[0].AIModel)
}

func TestLoadSessions_UsesPreviewLines(t *testing.T) {
	rec := &recordingExecutor{
		listSessionsOutput: "dev:$1:1:/home:0:1700000000\n",
		capturePaneOutput:  "some output",
	}
	mgr := tmux.NewManager(rec)
	m := ui.NewModel(ui.Deps{
		TmuxMgr: mgr,
		Cfg:     config.Config{PreviewLines: 200},
	})

	cmd := m.Init()
	_ = findSessionsMsg(t, cmd)

	// 找到 capture-pane 呼叫，驗證使用了設定的 PreviewLines 值
	found := false
	for _, call := range rec.calls {
		if len(call) > 0 && call[0] == "capture-pane" {
			found = true
			// capture-pane -t dev -p -S -200
			assert.Contains(t, call, "-200", "CapturePane 應使用設定的 PreviewLines 值 200")
		}
	}
	assert.True(t, found, "應有 capture-pane 呼叫")
}

func openUITestDB(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func applyKey(m ui.Model, key string) (ui.Model, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	updated, cmd := m.Update(msg)
	return updated.(ui.Model), cmd
}

// flexMockExecutor 使用 handler 函式處理所有 tmux 指令，方便彈性模擬。
type flexMockExecutor struct {
	handler func(args []string) (string, error)
}

func (e *flexMockExecutor) Execute(args ...string) (string, error) {
	return e.handler(args)
}

// --- Mode 相關測試 ---

func TestModel_ModeInput_EscCancels(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	// 按 n 進入 ModeInput
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeInput, m.Mode())

	// 按 Esc 回到 ModeNormal
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
}

func TestModel_ModeInput_RenderPrompt(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	// 按 n 進入 ModeInput
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeInput, m.Mode())

	view := m.View()
	assert.Contains(t, view, "Session 名稱")
	assert.Contains(t, view, "\u2588") // 游標方塊字元
}

func TestModel_ModeInput_BackspaceWorks(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	// 進入 ModeInput
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeInput, m.Mode())

	// 輸入 "abc"
	m, _ = applyKey(m, "a")
	m, _ = applyKey(m, "b")
	m, _ = applyKey(m, "c")
	assert.Equal(t, "abc", m.InputValue())

	// 按 Backspace 刪除一個字元
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model := updated.(ui.Model)
	assert.Equal(t, "ab", model.InputValue())
}

func TestModel_ModeConfirm_YConfirms(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	// 進入 ModeConfirm
	called := false
	m.SetConfirm("確定要刪除？", func() tea.Cmd {
		called = true
		return nil
	})
	assert.Equal(t, ui.ModeConfirm, m.Mode())

	// 按 y 確認
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.True(t, called, "confirmAction 應被呼叫")
}

func TestModel_ModeConfirm_EscCancels(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	// 進入 ModeConfirm
	called := false
	m.SetConfirm("確定要刪除？", func() tea.Cmd {
		called = true
		return nil
	})
	assert.Equal(t, ui.ModeConfirm, m.Mode())

	// 按 Esc 取消
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.False(t, called, "confirmAction 不應被呼叫")
}

func TestModel_ModeInput_EnterReturnsToNormal(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	// 進入 ModeInput
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeInput, m.Mode())

	// 輸入一些文字再按 Enter
	m, _ = applyKey(m, "t")
	m, _ = applyKey(m, "e")
	m, _ = applyKey(m, "s")
	m, _ = applyKey(m, "t")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.Equal(t, "", model.InputValue()) // 輸入值應被清空
}

func TestModel_ModeConfirm_RenderPrompt(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetConfirm("確定要刪除？", func() tea.Cmd { return nil })

	view := m.View()
	assert.Contains(t, view, "確定要刪除？")
	assert.Contains(t, view, "確認")
	assert.Contains(t, view, "取消")
}

func TestModel_ModeInput_EscDoesNotQuit(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	// 按 n 進入 ModeInput
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeInput, m.Mode())

	// 按 Esc 不應退出程式，只應回到 ModeNormal
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Nil(t, cmd, "ModeInput 中按 Esc 不應回傳 tea.Quit")
}

func TestModel_NewSession_CreatesAndReloads(t *testing.T) {
	// 追蹤 new-session 呼叫
	var newSessionCalls [][]string
	flex := &flexMockExecutor{
		handler: func(args []string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "new-session":
				newSessionCalls = append(newSessionCalls, args)
				return "", nil
			case "list-sessions":
				return "my-new:$1:1:/home:0:1700000000\n", nil
			case "list-panes":
				return "", nil
			case "capture-pane":
				return "", nil
			}
			return "", nil
		},
	}
	mgr := tmux.NewManager(flex)
	m := ui.NewModel(ui.Deps{TmuxMgr: mgr})

	// 按 'n' 進入 ModeInput
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeInput, m.Mode())

	// 輸入 "my-new"
	for _, ch := range "my-new" {
		m, _ = applyKey(m, string(ch))
	}
	assert.Equal(t, "my-new", m.InputValue())

	// 按 Enter 送出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	// 驗證：mode 回到 Normal
	assert.Equal(t, ui.ModeNormal, model.Mode())
	// 驗證：inputValue 被清空
	assert.Equal(t, "", model.InputValue())
	// 驗證：NewSession 被呼叫
	assert.Len(t, newSessionCalls, 1, "NewSession 應被呼叫一次")
	// 驗證：cmd 不為 nil（觸發 reload）
	assert.NotNil(t, cmd, "應回傳 loadSessionsCmd 以重新載入")
	// 驗證：無錯誤
	assert.Nil(t, model.Err())
}

func TestModel_NewSession_EmptyInput_NoOp(t *testing.T) {
	var newSessionCalls [][]string
	flex := &flexMockExecutor{
		handler: func(args []string) (string, error) {
			if len(args) > 0 && args[0] == "new-session" {
				newSessionCalls = append(newSessionCalls, args)
			}
			return "", nil
		},
	}
	mgr := tmux.NewManager(flex)
	m := ui.NewModel(ui.Deps{TmuxMgr: mgr})

	// 按 'n' 進入 ModeInput
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeInput, m.Mode())

	// 直接按 Enter（空輸入）
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	// 驗證：mode 回到 Normal
	assert.Equal(t, ui.ModeNormal, model.Mode())
	// 驗證：NewSession 未被呼叫
	assert.Len(t, newSessionCalls, 0, "空輸入不應呼叫 NewSession")
	// 驗證：cmd 為 nil（不觸發 reload）
	assert.Nil(t, cmd, "空輸入不應觸發 reload")
}

func TestModel_NewSession_Error_SetsErr(t *testing.T) {
	flex := &flexMockExecutor{
		handler: func(args []string) (string, error) {
			if len(args) > 0 && args[0] == "new-session" {
				return "", fmt.Errorf("tmux: duplicate session")
			}
			return "", nil
		},
	}
	mgr := tmux.NewManager(flex)
	m := ui.NewModel(ui.Deps{TmuxMgr: mgr})

	// 按 'n' 進入 ModeInput
	m, _ = applyKey(m, "n")

	// 輸入名稱
	for _, ch := range "dup" {
		m, _ = applyKey(m, string(ch))
	}

	// 按 Enter 送出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	// 驗證：mode 回到 Normal
	assert.Equal(t, ui.ModeNormal, model.Mode())
	// 驗證：err 被設定
	assert.NotNil(t, model.Err(), "NewSession 失敗時應設定 err")
	assert.Contains(t, model.Err().Error(), "duplicate session")
	// 驗證：cmd 為 nil（不觸發 reload）
	assert.Nil(t, cmd, "錯誤時不應觸發 reload")
}

// --- 刪除 session (d key) 相關測試 ---

func TestModel_DeleteSession_WithConfirm(t *testing.T) {
	// 追蹤 kill-session 呼叫
	var killSessionCalls [][]string
	flex := &flexMockExecutor{
		handler: func(args []string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "kill-session":
				killSessionCalls = append(killSessionCalls, args)
				return "", nil
			case "list-sessions":
				return "other:$2:1:/tmp:0:1700000000\n", nil
			case "list-panes":
				return "", nil
			case "capture-pane":
				return "", nil
			}
			return "", nil
		},
	}
	mgr := tmux.NewManager(flex)
	m := ui.NewModel(ui.Deps{TmuxMgr: mgr})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "other"}},
	})

	// cursor 在 session 上，按 d 應進入 ModeConfirm
	m, _ = applyKey(m, "d")
	assert.Equal(t, ui.ModeConfirm, m.Mode(), "按 d 後應進入 ModeConfirm")

	// 按 y 確認刪除
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model := updated.(ui.Model)

	// 驗證：mode 回到 Normal
	assert.Equal(t, ui.ModeNormal, model.Mode(), "確認後應回到 ModeNormal")
	// 驗證：KillSession 被呼叫
	assert.Len(t, killSessionCalls, 1, "KillSession 應被呼叫一次")
	assert.Contains(t, killSessionCalls[0], "dev", "應刪除名為 dev 的 session")
	// 驗證：cmd 不為 nil（觸發 reload）
	assert.NotNil(t, cmd, "應回傳 loadSessionsCmd 以重新載入")
}

func TestModel_DeleteSession_CancelWithEsc(t *testing.T) {
	// 追蹤 kill-session 呼叫
	var killSessionCalls [][]string
	flex := &flexMockExecutor{
		handler: func(args []string) (string, error) {
			if len(args) > 0 && args[0] == "kill-session" {
				killSessionCalls = append(killSessionCalls, args)
			}
			return "", nil
		},
	}
	mgr := tmux.NewManager(flex)
	m := ui.NewModel(ui.Deps{TmuxMgr: mgr})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 按 d 進入 ModeConfirm
	m, _ = applyKey(m, "d")
	assert.Equal(t, ui.ModeConfirm, m.Mode(), "按 d 後應進入 ModeConfirm")

	// 按 Esc 取消
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := updated.(ui.Model)

	// 驗證：mode 回到 Normal
	assert.Equal(t, ui.ModeNormal, model.Mode(), "取消後應回到 ModeNormal")
	// 驗證：KillSession 未被呼叫
	assert.Len(t, killSessionCalls, 0, "取消後 KillSession 不應被呼叫")
}

func TestModel_DeleteSession_OnGroup_NoOp(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "dev"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha"}},
	})

	// cursor 在群組上，按 d 不應進入 ModeConfirm
	m, _ = applyKey(m, "d")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "群組上按 d 應保持 ModeNormal")
}

// --- 重命名 session (r key) 相關測試 ---

func TestModel_Rename_SetsCustomName(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 按 r 進入 ModeInput（InputRenameSession）
	m, _ = applyKey(m, "r")
	assert.Equal(t, ui.ModeInput, m.Mode(), "按 r 後應進入 ModeInput")

	// 輸入 "My Dev"
	for _, ch := range "My Dev" {
		m, _ = applyKey(m, string(ch))
	}
	assert.Equal(t, "My Dev", m.InputValue())

	// 按 Enter 送出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	// 驗證：mode 回到 Normal
	assert.Equal(t, ui.ModeNormal, model.Mode())
	// 驗證：cmd 不為 nil（觸發 reload）
	assert.NotNil(t, cmd, "應回傳 loadSessionsCmd 以重新載入")
	// 驗證：store 中 custom_name 已設定
	metas, err := st.ListAllSessionMetas()
	assert.NoError(t, err)
	if assert.Len(t, metas, 1) {
		assert.Equal(t, "My Dev", metas[0].CustomName)
	}
}

func TestModel_Rename_OnGroup_NoOp(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "dev"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha"}},
	})

	// cursor 在群組上，按 r 應保持 ModeNormal
	m, _ = applyKey(m, "r")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "群組上按 r 應保持 ModeNormal")
}

func TestModel_View_ShowsCustomName(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:       "dev",
			CustomName: "開發伺服器",
			Status:     tmux.StatusIdle,
		}},
	})

	view := m.View()
	assert.Contains(t, view, "開發伺服器", "View 應顯示自訂名稱")
	assert.NotContains(t, view, "  dev  ", "View 不應顯示原始名稱（當有自訂名稱時）")
}

func TestModel_Rename_PrefillsCurrentCustomName(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:       "dev",
			CustomName: "原本名稱",
		}},
	})

	// 按 r 應預填目前的 CustomName
	m, _ = applyKey(m, "r")
	assert.Equal(t, ui.ModeInput, m.Mode())
	assert.Equal(t, "原本名稱", m.InputValue(), "應預填目前的自訂名稱")
}

// --- 新建群組 (g key) 相關測試 ---

func TestModel_CreateGroup(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	m := ui.NewModel(ui.Deps{Store: st})

	// 按 g 進入 ModeInput（InputNewGroup）
	m, _ = applyKey(m, "g")
	assert.Equal(t, ui.ModeInput, m.Mode(), "按 g 後應進入 ModeInput")

	// 輸入 "工作"
	for _, ch := range "工作" {
		m, _ = applyKey(m, string(ch))
	}
	assert.Equal(t, "工作", m.InputValue())

	// 按 Enter 送出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	// 驗證：mode 回到 Normal
	assert.Equal(t, ui.ModeNormal, model.Mode())
	// 驗證：inputValue 被清空
	assert.Equal(t, "", model.InputValue())
	// 驗證：store 中已建立群組
	groups, err := st.ListGroups()
	assert.NoError(t, err)
	if assert.Len(t, groups, 1) {
		assert.Equal(t, "工作", groups[0].Name)
	}
	// 驗證：cmd 不為 nil（觸發 reload）
	assert.NotNil(t, cmd, "應回傳 loadSessionsCmd 以重新載入")
	// 驗證：無錯誤
	assert.Nil(t, model.Err())
}

func TestModel_CreateGroup_NoStore_NoOp(t *testing.T) {
	// 無 Store 的 deps
	m := ui.NewModel(ui.Deps{})

	// 按 g 應保持 ModeNormal（因為沒有 Store）
	m, _ = applyKey(m, "g")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "無 Store 時按 g 應保持 ModeNormal")
}

func TestLoadSessions_UsesLayer2PaneTitle(t *testing.T) {
	flex := &flexMockExecutor{
		handler: func(args []string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "list-sessions":
				return "dev:$1:1:/home:0:1700000000\n", nil
			case "list-panes":
				// pane title 包含 Braille 旋轉指標
				return "dev:⠋ Working on feature\n", nil
			case "capture-pane":
				// pane 內容為空（無第三層指標）
				return "", nil
			}
			return "", nil
		},
	}
	mgr := tmux.NewManager(flex)
	m := ui.NewModel(ui.Deps{TmuxMgr: mgr})

	cmd := m.Init()
	sessMsg := findSessionsMsg(t, cmd)

	assert.Nil(t, sessMsg.Err)
	assert.Equal(t, 1, len(sessMsg.Sessions))
	// 第二層偵測：pane title 含 Braille 旋轉指標 → StatusRunning
	assert.Equal(t, tmux.StatusRunning, sessMsg.Sessions[0].Status,
		"pane title 含 Braille 旋轉指標，應偵測為 StatusRunning")
}
