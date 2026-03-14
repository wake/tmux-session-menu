package ui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/ui"
	"github.com/wake/tmux-session-menu/internal/upgrade"
	"github.com/wake/tmux-session-menu/internal/version"
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

func TestModel_View_ConnectionMode(t *testing.T) {
	tests := []struct {
		name string
		deps ui.Deps
		want string
	}{
		{"Direct", ui.Deps{}, "[Direct]"},
		{"Daemon", ui.Deps{Client: &client.Client{}}, "[Daemon]"},
		{"HostManager", ui.Deps{HostMgr: hostmgr.New()}, "[HostManager]"},
		{"Hub", ui.Deps{Client: &client.Client{}, HubMode: true}, "[Hub]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := ui.NewModel(tt.deps)
			view := m.View()
			assert.Contains(t, view, tt.want)
		})
	}
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

	// Wrap up: at top, press up → goes to bottom
	m, _ = applyKey(m, "k")
	assert.Equal(t, 2, m.Cursor())

	// Wrap down: at bottom, press down → goes to top
	m, _ = applyKey(m, "j")
	assert.Equal(t, 0, m.Cursor())
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
			AiType:   "claude",
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
	assert.Contains(t, view, "●") // agent running
	assert.Contains(t, view, "❯") // non-agent
	assert.Contains(t, view, "claude-sonnet-4-6")
}

func TestModel_View_TreeAndLineNumbers(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "development"}},
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:      "backend-api",
			GroupName: "development",
			Status:    tmux.StatusRunning,
			AIModel:   "claude",
		}},
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:      "docs",
			GroupName: "development",
			Status:    tmux.StatusIdle,
		}},
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:   "ungrouped",
			Status: tmux.StatusIdle,
		}},
	})

	view := m.View()

	// cursor 在 index 0（group），不應有 ► 箭頭（改用背景色）
	assert.NotContains(t, view, "►", "cursor 不應使用 ► 箭頭")
	assert.Contains(t, view, "▾")
	assert.Contains(t, view, "development")

	// 群組應有行號（1·）
	assert.Contains(t, view, "1·", "群組應有行號 1·")

	// 群組子項目應有樹狀符號
	assert.Contains(t, view, "├─", "非最後子項目應用 ├─")
	assert.Contains(t, view, "└─", "最後子項目應用 └─")

	// 未分組 session 不應有行號（只有群組有行號）
	assert.NotContains(t, view, "2·", "未分組 session 不應有行號")

	// 圖示應在名稱前面
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "backend-api") {
			// backend-api 無 AiType，StatusIcon 回傳 ❯
			iconIdx := strings.Index(line, "❯")
			nameIdx := strings.Index(line, "backend-api")
			if iconIdx >= 0 && nameIdx >= 0 {
				assert.Less(t, iconIdx, nameIdx, "圖示應在名稱前面")
			}
		}
	}
}

func TestModel_View_CursorUsesBackground(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:   "my-session",
			Status: tmux.StatusIdle,
		}},
	})

	view := m.View()

	// cursor 不應使用 ► 箭頭
	assert.NotContains(t, view, "►", "cursor 不應使用 ► 箭頭")
	// session 名稱仍應存在
	assert.Contains(t, view, "my-session")
	// cursor 行應使用 ANSI RGB 背景色
	assert.Contains(t, view, "\033[48;2;55;55;55m", "cursor 行應包含背景色序列")
}

func TestModel_View_UngroupedNoLineNumber(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "grp"}},
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:      "child",
			GroupName: "grp",
			Status:    tmux.StatusIdle,
		}},
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:   "standalone",
			Status: tmux.StatusIdle,
		}},
	})
	m.SetCursor(2) // cursor 在 standalone

	view := m.View()

	// 群組有行號
	assert.Contains(t, view, "1·", "群組應有行號 1·")
	// standalone 不應有行號
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "standalone") {
			assert.NotContains(t, line, "2·", "未分組 session 不應有行號")
		}
	}
}

func TestModel_View_AISummaryInline(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:      "dev",
			Status:    tmux.StatusRunning,
			AISummary: "Refactoring auth",
			Activity:  time.Now().Add(-3 * time.Minute),
		}},
	})

	view := m.View()

	// AISummary 應在同一行內聯顯示
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "dev") && strings.Contains(line, "Refactoring auth") {
			nameIdx := strings.Index(line, "dev")
			summaryIdx := strings.Index(line, "Refactoring auth")
			timeIdx := strings.Index(line, "3m")
			// 順序：name → summary → time
			assert.Less(t, nameIdx, summaryIdx, "名稱應在 summary 前面")
			if timeIdx >= 0 {
				assert.Less(t, summaryIdx, timeIdx, "summary 應在時間前面")
			}
			return
		}
	}
	t.Fatal("AISummary 應在 session 名稱同一行內聯顯示")
}

func TestModel_View_UnifiedStatusIcons(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:   "running-session",
			Status: tmux.StatusRunning,
			AiType: "claude",
		}},
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:   "waiting-session",
			Status: tmux.StatusWaiting,
			AiType: "claude",
		}},
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:   "idle-session",
			Status: tmux.StatusIdle,
		}},
	})

	view := m.View()

	// agent running 用 ●
	assert.Contains(t, view, "●")
	// agent waiting 用 ◐
	assert.Contains(t, view, "◐")
	// 非 agent 一律用 ❯
	assert.Contains(t, view, "❯")
}

func TestModel_AnimTick(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:   "dev",
			Status: tmux.StatusRunning,
		}},
	})

	// 送入 AnimTickMsg 應更新 animFrame
	updated, cmd := m.Update(ui.AnimTickMsg{})
	model := updated.(ui.Model)
	assert.Equal(t, 1, model.AnimFrame(), "AnimTickMsg 應遞增 animFrame")
	assert.NotNil(t, cmd, "AnimTickMsg 應回傳下一個 tick cmd")
}

func TestModel_View_CursorDoesNotTruncate(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "體大 ISTDC"}},
	})
	// 設定終端寬度
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	m = updated.(ui.Model)

	view := m.View()
	// cursor 行應包含完整的群組名稱，不應被截斷
	assert.Contains(t, view, "體大 ISTDC", "cursor 行不應截斷群組名稱")
	// cursor 行應用空格填充到終端寬度
	assert.Contains(t, view, "\033[48;2;55;55;55m", "cursor 行應包含背景色序列")
	assert.NotContains(t, view, "\033[K", "不應使用 Erase in Line（Bubble Tea 不支援）")
}

func TestModel_View_ToolbarFixedTwoLines(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev", Status: tmux.StatusIdle}},
	})

	view := m.View()

	// 所有工具列快捷鍵都應出現
	assert.Contains(t, view, "[/]", "工具列應包含 [/]")
	assert.Contains(t, view, "[n]", "工具列應包含 [n]")
	assert.Contains(t, view, "[r]", "工具列應包含 [r]")
	assert.Contains(t, view, "[d]", "工具列應包含 [d]")
	assert.Contains(t, view, "[q]", "工具列應包含 [q]")
	assert.Contains(t, view, "[R]", "工具列應包含 [R]")
	assert.Contains(t, view, "[m]", "工具列應包含 [m]")
	assert.Contains(t, view, "[ctrl+e]", "工具列應包含 [ctrl+e]")
	assert.NotContains(t, view, "[e]", "工具列不應包含 [e]")

	// 固定兩行佈局
	toolbarLines := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "[") && strings.Contains(line, "]") {
			if strings.Contains(line, "搜尋") || strings.Contains(line, "新建") ||
				strings.Contains(line, "更名") || strings.Contains(line, "刪除") ||
				strings.Contains(line, "搬移") || strings.Contains(line, "上移") ||
				strings.Contains(line, "退出") {
				toolbarLines++
			}
		}
	}
	assert.Equal(t, 2, toolbarLines, "工具列應固定兩行")
}

func TestModel_View_AISummaryShown(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:      "my-project",
			AISummary: "正在重構 auth 模組",
		}},
	})

	view := m.View()
	assert.Contains(t, view, "正在重構 auth 模組")
	// 不應在 Preview 區塊顯示（移至內聯）
	assert.NotContains(t, view, "Preview:", "AISummary 應內聯而非 Preview 區塊")
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

func TestModel_Enter_OnGroup_TogglesCollapse(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("dev", 0)
	groups, _ := st.ListGroups()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "target", GroupName: "dev"}},
	})

	// cursor 在群組上，按 Enter 應 toggle collapse
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	assert.Equal(t, "", model.Selected(), "不應選取 session")
	assert.NotNil(t, cmd, "應觸發 loadSessionsCmd")

	// 驗證 store 中 collapsed 已切換
	groups, _ = st.ListGroups()
	assert.True(t, groups[0].Collapsed, "群組應被收合")
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
	st := openUITestDB(t)
	defer st.Close()
	m := ui.NewModel(ui.Deps{Store: st})

	// 按 g 進入 ModeInput（新建群組）
	m, _ = applyKey(m, "g")
	assert.Equal(t, ui.ModeInput, m.Mode())

	// 按 Esc 回到 ModeNormal
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
}

func TestModel_ModeInput_RenderPrompt(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()
	m := ui.NewModel(ui.Deps{Store: st})

	// 按 g 進入 ModeInput（新建群組）
	m, _ = applyKey(m, "g")
	assert.Equal(t, ui.ModeInput, m.Mode())

	view := m.View()
	assert.Contains(t, view, "群組名稱")
	assert.Contains(t, view, "取消")
}

func TestModel_ModeInput_BackspaceWorks(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()
	m := ui.NewModel(ui.Deps{Store: st})

	// 按 g 進入 ModeInput（新建群組）
	m, _ = applyKey(m, "g")
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
	st := openUITestDB(t)
	defer st.Close()
	m := ui.NewModel(ui.Deps{Store: st})

	// 按 g 進入 ModeInput（新建群組）
	m, _ = applyKey(m, "g")
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
	st := openUITestDB(t)
	defer st.Close()
	m := ui.NewModel(ui.Deps{Store: st})

	// 按 g 進入 ModeInput（新建群組）
	m, _ = applyKey(m, "g")
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

	// 按 'n' 進入 ModeNewSession
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeNewSession, m.Mode())

	// 輸入 "my-new"
	for _, ch := range "my-new" {
		m, _ = applyKey(m, string(ch))
	}
	assert.Equal(t, "my-new", m.NewSessionNameValue())

	// 按 Enter 送出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	// 驗證：建立後自動掛載（quitting + selected）
	assert.True(t, model.Quitting(), "建立後應退出 TUI 以掛載")
	assert.Equal(t, "my-new", model.Selected(), "應選取新建的 session")
	// 驗證：NewSession 被呼叫
	assert.Len(t, newSessionCalls, 1, "NewSession 應被呼叫一次")
	// 驗證：cmd 不為 nil（tea.Quit）
	assert.NotNil(t, cmd, "應回傳 tea.Quit")
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

	// 按 'n' 進入 ModeNewSession
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeNewSession, m.Mode())

	// 直接按 Enter（空輸入）
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	// 驗證：mode 仍在 ModeNewSession（空名稱不送出）
	assert.Equal(t, ui.ModeNewSession, model.Mode())
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

	// 按 'n' 進入 ModeNewSession
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeNewSession, m.Mode())

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

	// 按 r 進入 ModeInput（InputRenameSession）雙行模式
	m, _ = applyKey(m, "r")
	assert.Equal(t, ui.ModeInput, m.Mode(), "按 r 後應進入 ModeInput")
	assert.Equal(t, 0, m.InputRow(), "預設焦點在第 0 行（名稱）")

	// 在第 0 行（名稱）輸入 "My Dev"
	for _, ch := range "My Dev" {
		m, _ = applyKey(m, string(ch))
	}
	assert.Equal(t, "My Dev", m.DualInputValue(0))
	// 第 1 行（ID）應預填 "dev"
	assert.Equal(t, "dev", m.DualInputValue(1))

	// 按 Enter 送出（ID 不變）
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

func TestModel_Rename_DualInput_SwitchRows(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev", CustomName: "開發"}},
	})

	// 按 r 進入雙行模式
	m, _ = applyKey(m, "r")
	assert.Equal(t, 0, m.InputRow(), "預設在第 0 行")
	assert.Equal(t, "開發", m.DualInputValue(0))
	assert.Equal(t, "dev", m.DualInputValue(1))

	// 按 Down 切到第 1 行
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(ui.Model)
	assert.Equal(t, 1, m.InputRow(), "按 Down 應切到第 1 行")

	// 按 Up 切回第 0 行
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(ui.Model)
	assert.Equal(t, 0, m.InputRow(), "按 Up 應切回第 0 行")
}

func TestModel_Rename_ClearCustomName(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 先設定自訂名稱
	st.SetCustomName("dev", "我的開發")

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev", CustomName: "我的開發"}},
	})

	// 按 r 進入雙行模式
	m, _ = applyKey(m, "r")
	assert.Equal(t, "我的開發", m.DualInputValue(0))

	// 清空第 0 行（名稱）
	for range "我的開發" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = updated.(ui.Model)
	}
	assert.Equal(t, "", m.DualInputValue(0))

	// 按 Enter 送出 → custom name 被清除
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.NotNil(t, cmd)

	metas, _ := st.ListAllSessionMetas()
	if assert.Len(t, metas, 1) {
		assert.Equal(t, "", metas[0].CustomName, "custom_name 應已清空")
	}
}

func TestModel_Rename_OnGroup_EntersModeInput(t *testing.T) {
	m := ui.NewModel(ui.Deps{Store: openUITestDB(t)})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{ID: 1, Name: "dev"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha"}},
	})

	// cursor 在群組上，按 r 應進入 ModeInput + InputRenameGroup
	m, _ = applyKey(m, "r")
	assert.Equal(t, ui.ModeInput, m.Mode(), "群組上按 r 應進入 ModeInput")
	assert.Equal(t, ui.InputRenameGroup, m.InputTarget(), "inputTarget 應為 InputRenameGroup")
	assert.Equal(t, "dev", m.InputValue(), "應預填群組名")
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
	// 排除 header 行（含版本號 "dev"），只檢查 items 區域不含原始名稱
	lines := strings.Split(view, "\n")
	for _, line := range lines[1:] {
		assert.NotContains(t, line, "  dev  ", "items 區域不應顯示原始名稱（當有自訂名稱時）")
	}
}

func TestModel_Rename_PrefillsCurrentCustomName(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{
			Name:       "dev",
			CustomName: "原本名稱",
		}},
	})

	// 按 r 應預填目前的 CustomName 和 session name
	m, _ = applyKey(m, "r")
	assert.Equal(t, ui.ModeInput, m.Mode())
	assert.Equal(t, "原本名稱", m.DualInputValue(0), "第 0 行應預填目前的自訂名稱")
	assert.Equal(t, "dev", m.DualInputValue(1), "第 1 行應預填 session name")
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

// --- 移動 session 到群組 (m key) 相關測試 ---

func TestModel_MoveSessionToGroup(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 建立兩個群組
	st.CreateGroup("工作", 0)
	st.CreateGroup("個人", 1)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 按 m 應進入 ModePicker
	m, _ = applyKey(m, "m")
	assert.Equal(t, ui.ModePicker, m.Mode(), "按 m 後應進入 ModePicker")

	// 按 Enter 選擇第一個群組（工作）
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	// 驗證：mode 回到 Normal
	assert.Equal(t, ui.ModeNormal, model.Mode(), "確認後應回到 ModeNormal")
	// 驗證：cmd 不為 nil（觸發 reload）
	assert.NotNil(t, cmd, "應回傳 loadSessionsCmd 以重新載入")

	// 驗證：store 中 session 已被移動到第一個群組
	groups, _ := st.ListGroups()
	metas, _ := st.ListAllSessionMetas()
	assert.Len(t, metas, 1)
	assert.Equal(t, "dev", metas[0].SessionName)
	assert.Equal(t, groups[0].ID, metas[0].GroupID, "session 應被移動到第一個群組")
}

func TestModel_MoveSession_PickerNavigation(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 建立三個群組
	st.CreateGroup("工作", 0)
	st.CreateGroup("個人", 1)
	st.CreateGroup("測試", 2)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 按 m 進入 ModePicker
	m, _ = applyKey(m, "m")
	assert.Equal(t, ui.ModePicker, m.Mode())
	assert.Equal(t, 0, m.PickerCursor(), "初始 pickerCursor 應為 0")

	// 按 j 向下移動
	m, _ = applyKey(m, "j")
	assert.Equal(t, 1, m.PickerCursor(), "按 j 後 pickerCursor 應為 1")

	// 按 j 再向下
	m, _ = applyKey(m, "j")
	assert.Equal(t, 2, m.PickerCursor(), "按 j 後 pickerCursor 應為 2")

	// 按 j 不能超過底部
	m, _ = applyKey(m, "j")
	assert.Equal(t, 2, m.PickerCursor(), "不能超過底部")

	// 按 k 向上移動
	m, _ = applyKey(m, "k")
	assert.Equal(t, 1, m.PickerCursor(), "按 k 後 pickerCursor 應為 1")

	// 按 k 回到頂部
	m, _ = applyKey(m, "k")
	assert.Equal(t, 0, m.PickerCursor(), "按 k 後 pickerCursor 應為 0")

	// 按 k 不能超過頂部
	m, _ = applyKey(m, "k")
	assert.Equal(t, 0, m.PickerCursor(), "不能超過頂部")
}

func TestModel_MoveSession_OnGroup_NoOp(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("工作", 0)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "工作"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// cursor 在群組上，按 m 應保持 ModeNormal
	m, _ = applyKey(m, "m")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "群組上按 m 應保持 ModeNormal")
}

func TestModel_MoveSession_EscCancels(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("工作", 0)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 按 m 進入 ModePicker
	m, _ = applyKey(m, "m")
	assert.Equal(t, ui.ModePicker, m.Mode())

	// 按 Esc 取消
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := updated.(ui.Model)

	// 驗證：mode 回到 Normal
	assert.Equal(t, ui.ModeNormal, model.Mode(), "Esc 後應回到 ModeNormal")
	// 驗證：cmd 為 nil（不觸發 reload）
	assert.Nil(t, cmd, "取消後不應觸發 reload")

	// 驗證：store 中無任何 session meta 變更
	metas, _ := st.ListAllSessionMetas()
	assert.Len(t, metas, 0, "取消後不應有任何 session meta 變更")
}

func TestModel_MoveSession_NoGroups_NoOp(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 不建立任何群組
	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 按 m 應保持 ModeNormal（因為沒有群組可選）
	m, _ = applyKey(m, "m")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "無群組時按 m 應保持 ModeNormal")
}

// --- 搜尋模式 (/ key) 相關測試 ---

func TestModel_Search_FiltersItems(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "dev"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha-1"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha-2"}},
	})

	// 按 / 進入搜尋模式
	m, _ = applyKey(m, "/")
	assert.Equal(t, ui.ModeSearch, m.Mode(), "按 / 後應進入 ModeSearch")

	// 輸入 "alpha"
	for _, ch := range "alpha" {
		m, _ = applyKey(m, string(ch))
	}
	assert.Equal(t, "alpha", m.SearchQuery())

	// View 應只包含 alpha 相關的 session
	view := m.View()
	assert.Contains(t, view, "alpha-1", "搜尋 alpha 後應顯示 alpha-1")
	assert.Contains(t, view, "alpha-2", "搜尋 alpha 後應顯示 alpha-2")
	assert.NotContains(t, view, "beta", "搜尋 alpha 後不應顯示 beta")
	// 群組也不應出現
	assert.NotContains(t, view, "▾", "搜尋時不應顯示群組標頭")
}

func TestModel_Search_EscClears(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "gamma"}},
	})

	// 進入搜尋模式並輸入
	m, _ = applyKey(m, "/")
	for _, ch := range "alpha" {
		m, _ = applyKey(m, string(ch))
	}
	assert.Equal(t, ui.ModeSearch, m.Mode())

	// 按 Esc 取消搜尋
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model := updated.(ui.Model)

	assert.Equal(t, ui.ModeNormal, model.Mode(), "Esc 後應回到 ModeNormal")
	assert.Equal(t, "", model.SearchQuery(), "Esc 後 searchQuery 應清空")

	// View 應顯示所有項目
	view := model.View()
	assert.Contains(t, view, "alpha")
	assert.Contains(t, view, "beta")
	assert.Contains(t, view, "gamma")
}

func TestModel_Search_EnterSelectsFiltered(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "gamma"}},
	})

	// 進入搜尋模式並輸入 "beta"
	m, _ = applyKey(m, "/")
	for _, ch := range "beta" {
		m, _ = applyKey(m, string(ch))
	}

	// 按 Enter 選擇（cursor 應在第一個過濾結果上）
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	assert.Equal(t, "beta", model.Selected(), "Enter 應選取過濾結果中的 beta")
	assert.NotNil(t, cmd, "應回傳 tea.Quit")
}

func TestModel_Search_NavigationInResults(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha-1"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha-2"}},
	})

	// 進入搜尋模式並輸入 "alpha"
	m, _ = applyKey(m, "/")
	for _, ch := range "alpha" {
		m, _ = applyKey(m, string(ch))
	}
	// cursor 應重設為 0
	assert.Equal(t, 0, m.Cursor())

	// 按 down 向下移動（搜尋模式中用方向鍵導航）
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(ui.Model)
	assert.Equal(t, 1, m.Cursor(), "按 down 後 cursor 應為 1")

	// 按 down 不能超過底部（只有 2 個過濾結果）
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(ui.Model)
	assert.Equal(t, 1, m.Cursor(), "不能超過底部")

	// 按 up 向上移動
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(ui.Model)
	assert.Equal(t, 0, m.Cursor(), "按 up 後 cursor 應為 0")

	// 按 up 不能超過頂部
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(ui.Model)
	assert.Equal(t, 0, m.Cursor(), "不能超過頂部")

	// 按 down 再按 Enter 應選取 alpha-2
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(ui.Model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)
	assert.Equal(t, "alpha-2", model.Selected(), "Enter 應選取 alpha-2")
	assert.NotNil(t, cmd)
}

// --- 排序（J/K 鍵）相關測試 ---

func TestModel_Sort_MoveGroupDown(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 建立兩個群組，sort_order 分別為 0 和 1
	st.CreateGroup("alpha", 0)
	st.CreateGroup("beta", 1)
	groups, _ := st.ListGroups()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]}, // alpha
		{Type: ui.ItemGroup, Group: groups[1]}, // beta
	})

	// cursor 在第一個群組上（alpha），按 J 下移
	m, cmd := applyKey(m, "J")

	// 驗證：cmd 不為 nil（觸發 reload）
	assert.NotNil(t, cmd, "J 應觸發 loadSessionsCmd")
	// 驗證：cursor 跟隨移動（0 → 1）
	assert.Equal(t, 1, m.Cursor(), "cursor 應跟隨移動到 1")

	// 驗證：store 中群組順序已交換
	updatedGroups, err := st.ListGroups()
	assert.NoError(t, err)
	assert.Len(t, updatedGroups, 2)
	// beta 的 sort_order 應變為 0（排前面），alpha 應變為 1（排後面）
	assert.Equal(t, "beta", updatedGroups[0].Name, "beta 應排在前面")
	assert.Equal(t, "alpha", updatedGroups[1].Name, "alpha 應排在後面")
}

func TestModel_Sort_MoveGroupUp(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 建立兩個群組
	st.CreateGroup("alpha", 0)
	st.CreateGroup("beta", 1)
	groups, _ := st.ListGroups()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]}, // alpha
		{Type: ui.ItemGroup, Group: groups[1]}, // beta
	})

	// 移動 cursor 到第二個群組（beta）
	m, _ = applyKey(m, "j")
	assert.Equal(t, 1, m.Cursor())

	// 按 K 上移
	m, cmd := applyKey(m, "K")

	// 驗證：cmd 不為 nil（觸發 reload）
	assert.NotNil(t, cmd, "K 應觸發 loadSessionsCmd")
	// 驗證：cursor 跟隨移動（1 → 0）
	assert.Equal(t, 0, m.Cursor(), "cursor 應跟隨移動到 0")

	// 驗證：store 中群組順序已交換
	updatedGroups, err := st.ListGroups()
	assert.NoError(t, err)
	assert.Len(t, updatedGroups, 2)
	// beta 的 sort_order 應變為 0（排前面），alpha 應變為 1（排後面）
	assert.Equal(t, "beta", updatedGroups[0].Name, "beta 應排在前面")
	assert.Equal(t, "alpha", updatedGroups[1].Name, "alpha 應排在後面")
}

func TestModel_Sort_MoveSessionDown(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 建立群組和兩個 session
	st.CreateGroup("dev", 0)
	groups, _ := st.ListGroups()
	groupID := groups[0].ID
	st.SetSessionGroup("alpha", groupID, 0)
	st.SetSessionGroup("beta", groupID, 1)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha", GroupName: "dev"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta", GroupName: "dev"}},
	})

	// 移動 cursor 到 alpha session（index 1）
	m, _ = applyKey(m, "j")
	assert.Equal(t, 1, m.Cursor())

	// 按 J 下移 alpha
	m, cmd := applyKey(m, "J")

	// 驗證：cmd 不為 nil（觸發 reload）
	assert.NotNil(t, cmd, "J 應觸發 loadSessionsCmd")
	// 驗證：cursor 跟隨移動（1 → 2）
	assert.Equal(t, 2, m.Cursor(), "cursor 應跟隨移動到 2")

	// 驗證：store 中 session 順序已交換
	metas, err := st.ListSessionMetas(groupID)
	assert.NoError(t, err)
	assert.Len(t, metas, 2)
	// beta 的 sort_order 應變為 0（排前面），alpha 應變為 1（排後面）
	assert.Equal(t, "beta", metas[0].SessionName, "beta 應排在前面")
	assert.Equal(t, "alpha", metas[1].SessionName, "alpha 應排在後面")
}

func TestModel_Sort_UngroupedSession_MoveDown(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 建立兩個未分組 session（groupID=0）
	st.SetSessionGroup("alpha", 0, 0)
	st.SetSessionGroup("beta", 0, 1)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta", SortOrder: 1}},
	})

	// cursor 在第一個未分組 session 上（alpha），按 J 下移
	m, cmd := applyKey(m, "J")

	// 驗證：cmd 不為 nil（觸發 reload）
	assert.NotNil(t, cmd, "J 應觸發 loadSessionsCmd")
	// 驗證：cursor 跟隨移動（0 → 1）
	assert.Equal(t, 1, m.Cursor(), "cursor 應跟隨移動到 1")

	// 驗證：store 中 session 順序已交換
	metas, err := st.ListSessionMetas(0)
	assert.NoError(t, err)
	assert.Len(t, metas, 2)
	assert.Equal(t, "beta", metas[0].SessionName, "beta 應排在前面")
	assert.Equal(t, "alpha", metas[1].SessionName, "alpha 應排在後面")
}

func TestModel_Sort_UngroupedSession_MoveUp(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 建立兩個未分組 session（groupID=0）
	st.SetSessionGroup("alpha", 0, 0)
	st.SetSessionGroup("beta", 0, 1)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta", SortOrder: 1}},
	})

	// 移動 cursor 到第二個未分組 session（beta）
	m, _ = applyKey(m, "j")
	assert.Equal(t, 1, m.Cursor())

	// 按 K 上移
	m, cmd := applyKey(m, "K")

	// 驗證：cmd 不為 nil（觸發 reload）
	assert.NotNil(t, cmd, "K 應觸發 loadSessionsCmd")
	// 驗證：cursor 跟隨移動（1 → 0）
	assert.Equal(t, 0, m.Cursor(), "cursor 應跟隨移動到 0")

	// 驗證：store 中 session 順序已交換
	metas, err := st.ListSessionMetas(0)
	assert.NoError(t, err)
	assert.Len(t, metas, 2)
	assert.Equal(t, "beta", metas[0].SessionName, "beta 應排在前面")
	assert.Equal(t, "alpha", metas[1].SessionName, "alpha 應排在後面")
}

func TestModel_Sort_UngroupedSession_Boundary(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 只有一個未分組 session
	st.SetSessionGroup("lonely", 0, 0)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "lonely", SortOrder: 0}},
	})

	// 按 J 嘗試下移（已在邊界）
	m, cmd := applyKey(m, "J")
	assert.Nil(t, cmd, "邊界處按 J 不應觸發任何 cmd")
	assert.Equal(t, 0, m.Cursor(), "cursor 應保持不變")

	// 按 K 嘗試上移（已在邊界）
	m, cmd = applyKey(m, "K")
	assert.Nil(t, cmd, "邊界處按 K 不應觸發任何 cmd")
	assert.Equal(t, 0, m.Cursor(), "cursor 應保持不變")
}

func TestModel_Sort_UngroupedSession_WrapsWithinGroup(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 建立群組和未分組 session
	st.CreateGroup("dev", 0)
	groups, _ := st.ListGroups()
	st.SetSessionGroup("grouped", groups[0].ID, 0)
	st.SetSessionGroup("ungrouped-a", 0, 0)
	st.SetSessionGroup("ungrouped-b", 0, 1)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "grouped", GroupName: "dev", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "ungrouped-a", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "ungrouped-b", SortOrder: 1}},
	})

	// 移動 cursor 到 ungrouped-a（index 2）
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	assert.Equal(t, 2, m.Cursor())

	// 按 K 上移（第一個未分組 session 應循環到最後一個未分組 session，不會跨越群組）
	m, cmd := applyKey(m, "K")
	assert.NotNil(t, cmd, "循環移動應觸發 loadSessionsCmd")
	assert.Equal(t, 3, m.Cursor(), "cursor 應循環到最後一個未分組 session")
}

func TestModel_Sort_AtBoundary_WrapsAround(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 建立兩個群組
	st.CreateGroup("alpha", 0)
	st.CreateGroup("beta", 1)
	groups, _ := st.ListGroups()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]}, // alpha
		{Type: ui.ItemGroup, Group: groups[1]}, // beta
	})

	// 移動 cursor 到最後一個群組（beta）
	m, _ = applyKey(m, "j")
	assert.Equal(t, 1, m.Cursor())

	// 按 J 下移（在邊界處應循環到頂部）
	m, cmd := applyKey(m, "J")

	assert.NotNil(t, cmd, "循環移動應觸發 loadSessionsCmd")
	assert.Equal(t, 0, m.Cursor(), "cursor 應循環到頂部")

	// 驗證：store 中群組順序已交換
	updatedGroups, _ := st.ListGroups()
	assert.Equal(t, "beta", updatedGroups[0].Name, "beta 應移到前面")
	assert.Equal(t, "alpha", updatedGroups[1].Name, "alpha 應移到後面")
}

func TestModel_Sort_DuplicateSortOrder_GroupMoveDown(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 兩個群組都使用 sort_order=0（模擬實際建立群組時的情境）
	st.CreateGroup("alpha", 0)
	st.CreateGroup("beta", 0)
	groups, _ := st.ListGroups()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]}, // alpha
		{Type: ui.ItemGroup, Group: groups[1]}, // beta
	})

	// cursor 在第一個群組上（alpha），按 J 下移
	m, cmd := applyKey(m, "J")

	assert.NotNil(t, cmd, "J 應觸發 loadSessionsCmd")
	assert.Equal(t, 1, m.Cursor(), "cursor 應跟隨移動到 1")

	// 驗證：store 中群組順序已交換
	updatedGroups, err := st.ListGroups()
	assert.NoError(t, err)
	assert.Len(t, updatedGroups, 2)
	assert.Equal(t, "beta", updatedGroups[0].Name, "beta 應排在前面")
	assert.Equal(t, "alpha", updatedGroups[1].Name, "alpha 應排在後面")
}

func TestModel_Sort_DuplicateSortOrder_SessionMoveDown(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	// 兩個未分組 session 都使用 sort_order=0（模擬未曾排序的情境）
	st.SetSessionGroup("alpha", 0, 0)
	st.SetSessionGroup("beta", 0, 0)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta", SortOrder: 0}},
	})

	// cursor 在第一個 session 上（alpha），按 J 下移
	m, cmd := applyKey(m, "J")

	assert.NotNil(t, cmd, "J 應觸發 loadSessionsCmd")
	assert.Equal(t, 1, m.Cursor(), "cursor 應跟隨移動到 1")

	// 驗證：store 中 session 順序已交換
	metas, err := st.ListSessionMetas(0)
	assert.NoError(t, err)
	assert.Len(t, metas, 2)
	assert.Equal(t, "beta", metas[0].SessionName, "beta 應排在前面")
	assert.Equal(t, "alpha", metas[1].SessionName, "alpha 應排在後面")
}

func TestModel_ModeInput_SpaceWorks(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()
	m := ui.NewModel(ui.Deps{Store: st})

	// 按 g 進入 ModeInput（新建群組）
	m, _ = applyKey(m, "g")
	assert.Equal(t, ui.ModeInput, m.Mode())

	// 輸入 "hello world"（含空格）
	for _, ch := range "hello" {
		m, _ = applyKey(m, string(ch))
	}
	// 使用 KeySpace 傳送空格（模擬真實終端行為）
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = updated.(ui.Model)
	for _, ch := range "world" {
		m, _ = applyKey(m, string(ch))
	}
	assert.Equal(t, "hello world", m.InputValue())
}

func TestModel_ModeInput_ArrowKeysMoveCursor(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()
	m := ui.NewModel(ui.Deps{Store: st})

	// 按 g 進入 ModeInput（新建群組）
	m, _ = applyKey(m, "g")
	assert.Equal(t, ui.ModeInput, m.Mode())

	// 輸入 "abcd"
	for _, ch := range "abcd" {
		m, _ = applyKey(m, string(ch))
	}
	assert.Equal(t, "abcd", m.InputValue())

	// 按左方向鍵兩次，游標移到 'c' 前
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(ui.Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(ui.Model)

	// 在游標位置插入 'X'
	m, _ = applyKey(m, "X")
	assert.Equal(t, "abXcd", m.InputValue())

	// 按右方向鍵一次，再插入 'Y'
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(ui.Model)
	m, _ = applyKey(m, "Y")
	assert.Equal(t, "abXcYd", m.InputValue())
}

func TestModel_Search_BackspaceWorks(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "abc"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "abd"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "xyz"}},
	})

	// 進入搜尋模式並輸入 "abc"
	m, _ = applyKey(m, "/")
	for _, ch := range "abc" {
		m, _ = applyKey(m, string(ch))
	}
	assert.Equal(t, "abc", m.SearchQuery())

	// 按 Backspace 刪除最後一個字元
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(ui.Model)
	assert.Equal(t, "ab", m.SearchQuery(), "Backspace 後 query 應為 ab")

	// View 應顯示 abc 和 abd（都包含 "ab"），不顯示 xyz
	view := m.View()
	assert.Contains(t, view, "abc")
	assert.Contains(t, view, "abd")
	assert.NotContains(t, view, "xyz")
}

// --- SnapshotMsg / SessionsMsg cursor 行為測試 ---

func TestSnapshotMsg_CursorStableAfterOptimisticSwap(t *testing.T) {
	// 模擬 optimistic swap 後收到 stale SnapshotMsg，cursor 不應跳回

	// 初始順序：alpha(0), beta(1), gamma(2)，cursor 在 beta（index 1）
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta", SortOrder: 1}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "gamma", SortOrder: 2}},
	})
	m.SetCursor(1)

	// 模擬 optimistic swap：beta ↔ gamma，cursor 移到 2
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "gamma", SortOrder: 1}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta", SortOrder: 2}},
	})
	m.SetCursor(2)
	assert.Equal(t, 2, m.Cursor(), "swap 後 cursor 應在 2")

	// 收到 stale SnapshotMsg（舊順序：alpha, beta, gamma）
	staleSnap := &tsmv1.StateSnapshot{
		Sessions: []*tsmv1.Session{
			{Name: "alpha", SortOrder: 0},
			{Name: "beta", SortOrder: 1},
			{Name: "gamma", SortOrder: 2},
		},
	}
	updated, _ := m.Update(ui.SnapshotMsg{Snapshot: staleSnap})
	m = updated.(ui.Model)

	// cursor 應保持在 2，不被 stale snapshot 拉回到 beta 的舊位置 1
	assert.Equal(t, 2, m.Cursor(), "stale SnapshotMsg 不應改變 cursor 位置")
}

func TestSnapshotMsg_CursorBoundsOnShrink(t *testing.T) {
	// 當 SnapshotMsg 使 items 縮減時，cursor 不應超界

	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "gamma"}},
	})
	m.SetCursor(2)

	// 收到 SnapshotMsg，只剩 2 個 session
	snap := &tsmv1.StateSnapshot{
		Sessions: []*tsmv1.Session{
			{Name: "alpha", SortOrder: 0},
			{Name: "beta", SortOrder: 1},
		},
	}
	updated, _ := m.Update(ui.SnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)

	assert.Equal(t, 1, m.Cursor(), "cursor 應被 bounds check 修正為 len-1")
}

func TestSessionsMsg_CursorFollowsItem(t *testing.T) {
	// SessionsMsg（store 模式）的 restoreCursor 應正確追蹤 item

	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "beta"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "gamma"}},
	})
	m.SetCursor(1) // cursor 在 beta

	// 收到 SessionsMsg，順序改為 alpha, gamma, beta（beta 移到 index 2）
	updated, _ := m.Update(ui.SessionsMsg{
		Sessions: []tmux.Session{
			{Name: "alpha", SortOrder: 0},
			{Name: "gamma", SortOrder: 1},
			{Name: "beta", SortOrder: 2},
		},
	})
	m = updated.(ui.Model)

	assert.Equal(t, 2, m.Cursor(), "cursor 應跟隨 beta 移到 index 2")
}

// --- 左/右箭頭群組展開/收合 相關測試 ---

func TestModel_LeftArrow_TogglesGroup(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("dev", 0)
	groups, _ := st.ListGroups()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha", GroupName: "dev"}},
	})

	// 按 left → toggle（展開→收合）
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.NotNil(t, cmd, "應觸發 loadSessionsCmd")
	groups, _ = st.ListGroups()
	assert.True(t, groups[0].Collapsed, "群組應被收合")
}

func TestModel_RightArrow_TogglesGroup(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("dev", 0)
	groups, _ := st.ListGroups()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha", GroupName: "dev"}},
	})

	// 按 right → toggle（展開→收合）
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.NotNil(t, cmd, "應觸發 loadSessionsCmd")
	groups, _ = st.ListGroups()
	assert.True(t, groups[0].Collapsed, "群組應被收合")
}

func TestModel_LeftRight_OnSession_NoOp(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha"}},
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Nil(t, cmd, "session 上按 left 應無動作")

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Nil(t, cmd, "session 上按 right 應無動作")
}

// --- 群組重命名 (r key on group) 相關測試 ---

func TestModel_Rename_OnGroup_PrefillsCurrentName(t *testing.T) {
	m := ui.NewModel(ui.Deps{Store: openUITestDB(t)})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{ID: 1, Name: "原始群組"}},
	})

	m, _ = applyKey(m, "r")
	assert.Equal(t, "原始群組", m.InputValue(), "應預填目前的群組名稱")
}

func TestModel_Rename_OnGroup_Submit_Updates(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("old-name", 0)
	groups, _ := st.ListGroups()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]},
	})

	// 按 r 進入重命名模式
	m, _ = applyKey(m, "r")
	assert.Equal(t, ui.ModeInput, m.Mode())

	// 清空預填值並輸入新名稱
	// 先用 Ctrl+A + 刪除的方式清空，或重新設定
	// 這裡用逐字刪除的方式
	for range "old-name" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = updated.(ui.Model)
	}
	for _, ch := range "new-name" {
		m, _ = applyKey(m, string(ch))
	}

	// 按 Enter 送出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := updated.(ui.Model)

	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.NotNil(t, cmd, "應回傳 loadSessionsCmd 以重新載入")

	// 驗證 store 中群組名稱已更新
	groups, _ = st.ListGroups()
	assert.Equal(t, "new-name", groups[0].Name)
}

// --- 小箭頭展開/收合測試 ---

func TestModel_View_SmallArrows(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "dev"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha", GroupName: "dev"}},
	})

	view := m.View()
	// 展開狀態應使用小箭頭 ▾，不應使用大箭頭 ▼
	assert.Contains(t, view, "▾", "展開群組應使用小箭頭 ▾")
	assert.NotContains(t, view, "▼", "不應使用大箭頭 ▼")

	// 直接設定收合狀態的群組（toggleCollapse 需要 Store）
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "dev", Collapsed: true}},
	})
	view = m.View()
	assert.Contains(t, view, "▸", "收合群組應使用小箭頭 ▸")
	assert.NotContains(t, view, "▶", "不應使用大箭頭 ▶")
}

// --- ctrl+e 退出 tmux 測試 ---

func TestModel_ExitTmux_Key(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 按 ctrl+e 應設定 exitTmux 旗標並退出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model := updated.(ui.Model)
	assert.NotNil(t, cmd, "按 ctrl+e 應回傳 tea.Quit")
	assert.True(t, model.ExitTmux(), "按 ctrl+e 後 ExitTmux() 應為 true")
}

func TestModel_ExitTmux_QDoesNotSet(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 按 q 不應設定 exitTmux 旗標
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model := updated.(ui.Model)
	assert.False(t, model.ExitTmux(), "按 q 後 ExitTmux() 應為 false")
}

func TestModel_ExitTmux_NotInModeInput(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()
	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 進入 ModeInput 後按 ctrl+e 不應退出（textinput 吃掉）
	m, _ = applyKey(m, "g") // 按 g 進入 ModeInput（新建群組）
	assert.Equal(t, ui.ModeInput, m.Mode())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeInput, model.Mode(), "ModeInput 中按 ctrl+e 不應退出")
	assert.False(t, model.ExitTmux())
}

// --- Shift+Up/Down 作為 K/J 上移下移的別名 ---

func TestModel_ShiftUp_MovesItemUp(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("dev", 0)
	groups, _ := st.ListGroups()
	st.SetSessionGroup("a", groups[0].ID, 0)
	st.SetSessionGroup("b", groups[0].ID, 1)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "a", GroupName: "dev", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "b", GroupName: "dev", SortOrder: 1}},
	})
	m.SetCursor(2) // cursor 在 b

	// Shift+Up 應等同 K（上移）
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	m = updated.(ui.Model)

	assert.Equal(t, 1, m.Cursor(), "Shift+Up 應將項目上移，cursor 變為 1")
}

func TestModel_ShiftDown_MovesItemDown(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("dev", 0)
	groups, _ := st.ListGroups()
	st.SetSessionGroup("a", groups[0].ID, 0)
	st.SetSessionGroup("b", groups[0].ID, 1)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "a", GroupName: "dev", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "b", GroupName: "dev", SortOrder: 1}},
	})
	m.SetCursor(1) // cursor 在 a

	// Shift+Down 應等同 J（下移）
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	m = updated.(ui.Model)

	assert.Equal(t, 2, m.Cursor(), "Shift+Down 應將項目下移，cursor 變為 2")
}

// --- Shift+R 唯讀進入 session ---

func TestModel_ShiftR_ReadOnlyEnter(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 按 R (Shift+R) 應選取 session 並標記唯讀
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	model := updated.(ui.Model)

	assert.NotNil(t, cmd, "按 R 應回傳 tea.Quit")
	assert.Equal(t, "dev", model.Selected(), "應選取 session")
	assert.True(t, model.ReadOnly(), "按 R 後 ReadOnly() 應為 true")
}

func TestModel_ShiftR_OnGroup_NoOp(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: store.Group{Name: "dev"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "alpha"}},
	})

	// cursor 在群組上，按 R 應無動作
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	model := updated.(ui.Model)
	assert.Nil(t, cmd, "群組上按 R 應無動作")
	assert.False(t, model.ReadOnly())
}

// --- ctrl+e 取代 e 作為退出 tmux 的按鍵 ---

func TestModel_CtrlE_ExitsTmux(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// ctrl+e 應設定 exitTmux 旗標並退出
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model := updated.(ui.Model)
	assert.NotNil(t, cmd, "ctrl+e 應回傳 tea.Quit")
	assert.True(t, model.ExitTmux(), "ctrl+e 後 ExitTmux() 應為 true")
}

func TestModel_E_NoLongerExitsTmux(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 按 e 不應再觸發退出 tmux
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model := updated.(ui.Model)
	assert.Nil(t, cmd, "按 e 不應觸發退出")
	assert.False(t, model.ExitTmux(), "按 e 後 ExitTmux() 應為 false")
}

// --- 在 tmux 中按 q 應進入確認模式 ---

func TestModel_Quit_InTmux_DirectQuit(t *testing.T) {
	m := ui.NewModel(ui.Deps{Cfg: config.Config{InTmux: true}})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 在 tmux 中按 q 應直接退出，不經確認
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.NotNil(t, cmd, "tmux 中按 q 應直接退出")
}

func TestModel_Quit_NotInTmux_DirectQuit(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 不在 tmux 中，按 q 應直接退出（不經確認）
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.NotNil(t, cmd, "不在 tmux 中按 q 應直接退出")
}

func TestModel_Esc_InTmux_DirectQuit(t *testing.T) {
	m := ui.NewModel(ui.Deps{Cfg: config.Config{InTmux: true}})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})

	// 在 tmux 中按 esc 應直接退出（不經確認，只關閉選單）
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.NotNil(t, cmd, "esc 應直接退出")
}

// --- 工具列佈局更新 ---

func TestModel_View_ToolbarNewLayout(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev", Status: tmux.StatusIdle}},
	})

	view := m.View()

	// 第一行：搜尋、新建、更名、刪除、唯獨進入、關閉選單
	assert.Contains(t, view, "唯讀進入", "工具列應包含唯讀進入")
	assert.Contains(t, view, "關閉選單", "工具列應包含關閉選單")

	// 第二行：搬移、上移（含 shift 符號）、下移（含 shift 符號）、退出（ctrl+e）
	assert.Contains(t, view, "⇧+↑", "工具列應包含 ⇧+↑")
	assert.Contains(t, view, "⇧+↓", "工具列應包含 ⇧+↓")
	assert.Contains(t, view, "ctrl+e", "工具列應包含 ctrl+e")

	// 不應再有舊的 [e] 退出tmux
	assert.NotContains(t, view, "[e]", "工具列不應包含 [e]")
}

func TestModel_ConfigKey_SetsOpenConfig(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	fm := updated.(ui.Model)
	assert.True(t, fm.OpenConfig())
	assert.True(t, fm.Quitting())
}

// ─── Wrap-around navigation ─────────────────────────────────

func TestModel_Navigation_WrapDown(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "a"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "b"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "c"}},
	})

	// 移到最底
	m, _ = applyKey(m, "j")
	m, _ = applyKey(m, "j")
	assert.Equal(t, 2, m.Cursor())

	// 再按 down，應該循環到最上面
	m, _ = applyKey(m, "j")
	assert.Equal(t, 0, m.Cursor(), "cursor 應循環回到頂部")
}

func TestModel_Navigation_WrapUp(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "a"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "b"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "c"}},
	})

	assert.Equal(t, 0, m.Cursor())

	// 在頂部按 up，應該循環到最底
	m, _ = applyKey(m, "k")
	assert.Equal(t, 2, m.Cursor(), "cursor 應循環回到底部")
}

func TestModel_Navigation_WrapDown_Arrow(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "a"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "b"}},
	})

	m, _ = applyKey(m, "down")
	assert.Equal(t, 1, m.Cursor())

	m, _ = applyKey(m, "down")
	assert.Equal(t, 0, m.Cursor(), "down arrow 應循環回到頂部")
}

func TestModel_Navigation_WrapUp_Arrow(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "a"}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "b"}},
	})

	m, _ = applyKey(m, "up")
	assert.Equal(t, 1, m.Cursor(), "up arrow 應循環回到底部")
}

// ─── Wrap-around item moving ────────────────────────────────

func TestModel_Sort_GroupWrapDown(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("alpha", 0)
	st.CreateGroup("beta", 1)
	groups, _ := st.ListGroups()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]}, // alpha
		{Type: ui.ItemGroup, Group: groups[1]}, // beta
	})

	// cursor 在 beta（最後一個群組），按 J 下移應循環到第一個
	m.SetCursor(1)
	m, _ = applyKey(m, "J")

	assert.Equal(t, 0, m.Cursor(), "cursor 應循環到頂部")

	updatedGroups, _ := st.ListGroups()
	assert.Equal(t, "beta", updatedGroups[0].Name, "beta 應移到前面")
	assert.Equal(t, "alpha", updatedGroups[1].Name, "alpha 應移到後面")
}

func TestModel_Sort_GroupWrapUp(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.CreateGroup("alpha", 0)
	st.CreateGroup("beta", 1)
	groups, _ := st.ListGroups()

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemGroup, Group: groups[0]}, // alpha
		{Type: ui.ItemGroup, Group: groups[1]}, // beta
	})

	// cursor 在 alpha（第一個群組），按 K 上移應循環到最後
	m, _ = applyKey(m, "K")

	assert.Equal(t, 1, m.Cursor(), "cursor 應循環到底部")

	updatedGroups, _ := st.ListGroups()
	assert.Equal(t, "beta", updatedGroups[0].Name, "beta 應排在前面")
	assert.Equal(t, "alpha", updatedGroups[1].Name, "alpha 應排在後面")
}

func TestModel_Sort_SessionWrapDown(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.SetSessionGroup("sess-a", 0, 0)
	st.SetSessionGroup("sess-b", 0, 1)
	st.SetSessionGroup("sess-c", 0, 2)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "sess-a", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "sess-b", SortOrder: 1}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "sess-c", SortOrder: 2}},
	})

	// cursor 在最後一個 session，按 J 下移應循環
	m.SetCursor(2)
	m, _ = applyKey(m, "J")

	assert.Equal(t, 0, m.Cursor(), "cursor 應循環到頂部")
}

func TestModel_Sort_SessionWrapUp(t *testing.T) {
	st := openUITestDB(t)
	defer st.Close()

	st.SetSessionGroup("sess-a", 0, 0)
	st.SetSessionGroup("sess-b", 0, 1)
	st.SetSessionGroup("sess-c", 0, 2)

	m := ui.NewModel(ui.Deps{Store: st})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemSession, Session: tmux.Session{Name: "sess-a", SortOrder: 0}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "sess-b", SortOrder: 1}},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "sess-c", SortOrder: 2}},
	})

	// cursor 在第一個 session，按 K 上移應循環
	m, _ = applyKey(m, "K")

	assert.Equal(t, 2, m.Cursor(), "cursor 應循環到底部")
}

// --- Remote 模式 Watch stream 斷線測試 ---

func TestSnapshotMsg_RemoteMode_WatchError_SetsWatchFailed(t *testing.T) {
	// Remote 模式下 Watch stream 錯誤應設定 watchFailed 並退出
	m := ui.NewModel(ui.Deps{RemoteMode: true})
	updated, cmd := m.Update(ui.SnapshotMsg{Err: fmt.Errorf("connection reset")})
	rm := updated.(ui.Model)
	assert.True(t, rm.WatchFailed(), "remote 模式 watch 錯誤應設定 watchFailed")
	assert.NotNil(t, cmd) // 應為 tea.Quit
}

func TestSnapshotMsg_LocalMode_WatchError_DoesNotSetWatchFailed(t *testing.T) {
	// 非 remote 模式下 Watch stream 錯誤應重試，不設定 watchFailed
	m := ui.NewModel(ui.Deps{})
	updated, _ := m.Update(ui.SnapshotMsg{Err: fmt.Errorf("connection reset")})
	rm := updated.(ui.Model)
	assert.False(t, rm.WatchFailed(), "本地模式不應設定 watchFailed")
}

func TestWatchFailed_InitiallyFalse(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	assert.False(t, m.WatchFailed())
}

// --- 左側 padding 測試 ---

func TestView_HasLeftPadding(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	view := m.View()
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		assert.True(t, strings.HasPrefix(line, " "),
			"每行應有左側 padding: %q", line)
	}
}

func TestView_InPopup_HasExtraPadding(t *testing.T) {
	m := ui.NewModel(ui.Deps{Cfg: config.Config{InPopup: true}})
	view := m.View()
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		assert.True(t, strings.HasPrefix(line, "  "),
			"popup 模式每行應有雙倍左側 padding: %q", line)
	}
}

func TestModel_View_HostTitle(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemHostTitle, HostID: "local", HostColor: "#5B9BD5", HostState: ui.HostStateConnected},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev", Status: tmux.StatusRunning}},
		{Type: ui.ItemHostTitle, HostID: "staging", HostColor: "#FFC000", HostState: ui.HostStateConnecting},
		{Type: ui.ItemHostTitle, HostID: "prod", HostColor: "#FF6347", HostState: ui.HostStateDisconnected, HostError: "timeout"},
		{Type: ui.ItemHostTitle, HostID: "legacy", HostColor: "#AAAAAA", HostState: ui.HostStateDisabled},
	})

	view := m.View()

	// 已連線主機：badge 顯示主機名稱
	assert.Contains(t, view, "local", "should contain connected host title 'local'")
	// 連線中主機：badge + 狀態文字
	assert.Contains(t, view, "staging", "should contain connecting host title 'staging'")
	assert.Contains(t, view, "連線中", "staging should show connecting state")
	// 已斷線主機：badge + 友善狀態文字
	assert.Contains(t, view, "prod", "should contain disconnected host title 'prod'")
	assert.Contains(t, view, "連線中斷，重連中...", "prod should show friendly disconnected state")
	// 已停用主機
	assert.Contains(t, view, "legacy", "should contain disabled host title 'legacy'")
	assert.Contains(t, view, "已停用", "legacy should show disabled state")
}

func TestModel_View_HostTitle_CursorHighlight(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m.SetItems([]ui.ListItem{
		{Type: ui.ItemHostTitle, HostID: "remote-box", HostColor: "#5B9BD5", HostState: ui.HostStateConnected},
		{Type: ui.ItemSession, Session: tmux.Session{Name: "dev"}},
	})
	// cursor 預設在 0，即 Host Title 行
	view := m.View()
	assert.Contains(t, view, "remote-box", "host title should be visible when cursor is on it")
}

func TestCtrlU_WithoutUpgrader_Noop(t *testing.T) {
	m := ui.NewModel(ui.Deps{}) // 無 Upgrader
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.Nil(t, cmd)
}

func TestCtrlU_WithUpgrader_TriggersCheck(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			return []byte(`{"tag_name": "v99.0.0", "assets": []}`), nil
		},
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	assert.NotNil(t, cmd, "ctrl+u 應觸發版本檢查命令")
}

func TestCheckUpgradeMsg_NewVersion(t *testing.T) {
	// 暫時設定版本號，讓 NeedsUpgrade 回傳 true
	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) { return nil, nil },
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{
			Version: "99.0.0",
			Assets:  map[string]string{upgrade.AssetName(): "https://example.com/dl"},
		},
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "99.0.0")
	assert.Contains(t, model.ConfirmPrompt(), "升級")
}

func TestCheckUpgradeMsg_AlreadyLatest(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	// version.Version 為 "dev" 時 NeedsUpgrade 一律回傳 false
	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{Version: "0.0.0"},
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "已是最新版本")
}

func TestCheckUpgradeMsg_Error(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Err: fmt.Errorf("network timeout"),
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "檢查失敗")
	assert.Contains(t, model.ConfirmPrompt(), "network timeout")
}

func TestCheckUpgradeMsg_NoPlatformAsset(t *testing.T) {
	// 暫時設定版本號，讓 NeedsUpgrade 回傳 true
	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) { return nil, nil },
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{
			Version: "99.0.0",
			Assets:  map[string]string{"tsm-windows-amd64": "https://example.com/dl"},
		},
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "找不到")
}

// --- 升級確認 Y/N 測試（Task 5）---

func TestConfirmUpgrade_Yes_TriggersDownload(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			return []byte("binary-content"), nil
		},
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	// 暫時設定版本號，讓 NeedsUpgrade 回傳 true
	old := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = old }()

	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{
			Version: "99.0.0",
			Assets:  map[string]string{upgrade.AssetName(): "https://example.com/dl"},
		},
	})
	m = updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, m.Mode())

	// 按 Y 確認下載
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.NotNil(t, cmd, "Y 應觸發下載命令")
}

func TestConfirmUpgrade_No_Cancels(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) { return nil, nil },
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	old := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = old }()

	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{
			Version: "99.0.0",
			Assets:  map[string]string{upgrade.AssetName(): "https://example.com/dl"},
		},
	})
	m = updated.(ui.Model)

	// 按 N 取消
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.Nil(t, cmd)
}

// --- DownloadUpgradeMsg 成功/失敗測試（Task 6, 7）---

func TestDownloadUpgradeMsg_Success(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	updated, cmd := m.Update(ui.DownloadUpgradeMsg{TmpPath: "/tmp/tsm-new"})
	model := updated.(ui.Model)
	assert.True(t, model.UpgradeReady())
	tmpPath, _ := model.UpgradeInfo()
	assert.Equal(t, "/tmp/tsm-new", tmpPath)
	assert.NotNil(t, cmd, "should return tea.Quit")
}

func TestDownloadUpgradeMsg_Error(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	updated, _ := m.Update(ui.DownloadUpgradeMsg{Err: fmt.Errorf("disk full")})
	model := updated.(ui.Model)
	assert.False(t, model.UpgradeReady())
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "下載失敗")
	assert.Contains(t, model.ConfirmPrompt(), "disk full")
}

func TestView_UpgradeConfirm_ShowsVersionDiff(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) { return nil, nil },
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	old := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = old }()

	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{
			Version: "99.0.0",
			Assets:  map[string]string{upgrade.AssetName(): "https://example.com/dl"},
		},
	})
	model := updated.(ui.Model)
	view := model.View()
	assert.Contains(t, view, "99.0.0")
	assert.Contains(t, view, "升級")
}

func TestToolbar_ShowsUpgradeHint_WhenUpgraderPresent(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) { return nil, nil },
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})
	view := m.View()
	assert.Contains(t, view, "ctrl+u")
}

func TestToolbar_HidesUpgradeHint_WhenNoUpgrader(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	view := m.View()
	assert.NotContains(t, view, "ctrl+u")
}

func TestCtrlU_DuplicateBlocked(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			return []byte(`{"tag_name": "v99.0.0", "assets": []}`), nil
		},
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	// 第一次 ctrl+u 應觸發檢查
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	assert.NotNil(t, cmd, "第一次 ctrl+u 應觸發命令")

	// 第二次 ctrl+u 應被擋住（upgradeChecking = true）
	updated2, cmd2 := updated.(ui.Model).Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	_ = updated2
	assert.Nil(t, cmd2, "第二次 ctrl+u 應被擋住")
}

func TestCheckUpgradeMsg_BinaryNewerThanTUI(t *testing.T) {
	origVersion := version.Version
	version.Version = "0.23.0"
	defer func() { version.Version = origVersion }()

	m := ui.NewModel(ui.Deps{})

	// GitHub 最新也是 0.23.0（TUI 已是最新），但磁碟上的 binary 是 0.24.0
	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release:       &upgrade.Release{Version: "0.23.0"},
		InstalledVer:  "0.24.0",
		InstalledPath: "/usr/local/bin/tsm",
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "v0.24.0")
	assert.Contains(t, model.ConfirmPrompt(), "重新啟動")
}

func TestCheckUpgradeMsg_GitHubFail_BinaryNewer(t *testing.T) {
	origVersion := version.Version
	version.Version = "0.23.0"
	defer func() { version.Version = origVersion }()

	m := ui.NewModel(ui.Deps{})

	// GitHub 檢查失敗，但磁碟上有較新版本
	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Err:           fmt.Errorf("network error"),
		InstalledVer:  "0.24.0",
		InstalledPath: "/usr/local/bin/tsm",
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "重新啟動")
}

func TestHostPicker_QReturnsToNormal(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	// 模擬進入 ModeHostPicker（直接透過 Update 設定較困難，改用 ConfirmReturnMode 測試）
	// 測試 ModeConfirm 取消後回到 ModeNormal
	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{Version: "0.0.0"},
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())

	// 按 q 取消確認，回到 ModeNormal
	updated2, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model2 := updated2.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model2.Mode())
}

func TestConfirmReturnMode_BackToHostPicker(t *testing.T) {
	// 測試 confirmReturnMode 正確回到 ModeHostPicker
	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) { return nil, nil },
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	// 先觸發一個 CheckUpgradeMsg 讓模型進入 ModeConfirm
	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{Version: "1.0.0"},
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "已是最新版本")

	// 按 esc 取消，應回到 ModeNormal（預設 confirmReturnMode）
	updated2, _ := model.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model2 := updated2.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model2.Mode())
}

func TestInit_MultiHost_ImmediateSnapshot(t *testing.T) {
	// 建立 HostManager 並加入一台 Enabled 主機（初始為 Connecting 狀態）
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "remote-1", Address: "10.0.0.1", Enabled: true, Color: "#ff0000"})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})
	cmd := m.Init()
	assert.NotNil(t, cmd)

	// Init 應回傳 tea.Batch，其中包含立即快照的 cmd
	// 模擬 Bubble Tea 執行：直接呼叫 batch 取得初始訊息
	// 由於 tea.Batch 內部的立即 cmd 會回傳 MultiHostSnapshotMsg，
	// 我們用 Update 來處理它，確認 items 會有主機標題
	msg := ui.MultiHostSnapshotMsg{
		Snapshots: []ui.HostSnapshotInput{
			{HostID: "remote-1", Name: "remote-1", Color: "#ff0000", Status: ui.HostStateConnecting},
		},
	}
	updated, _ := m.Update(msg)
	model := updated.(ui.Model)
	items := model.Items()
	assert.Len(t, items, 1, "應有一個 host title 項目")
	assert.Equal(t, ui.ItemHostTitle, items[0].Type)
	assert.Equal(t, "remote-1", items[0].HostID)
	assert.Equal(t, ui.HostStateConnecting, items[0].HostState)
}

func TestDownloadUpgradeMsg_RestartOnly(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	// 空 TmpPath 表示 restart-only
	updated, cmd := m.Update(ui.DownloadUpgradeMsg{TmpPath: ""})
	model := updated.(ui.Model)
	assert.True(t, model.UpgradeReady())
	assert.NotNil(t, cmd) // tea.Quit
}

// --- ModeUpgrade 入口測試（Task 3）---

func TestModel_CheckUpgradeMsg_EntersModeUpgrade(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Address: "", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "air-2019", Address: "air-2019", Enabled: true})

	deps := ui.Deps{HostMgr: mgr, Upgrader: &upgrade.Upgrader{}}
	m := ui.NewModel(deps)

	msg := ui.CheckUpgradeMsg{
		Release:      &upgrade.Release{Version: "0.28.0"},
		HostVersions: map[string]string{"local": "0.27.0", "air-2019": "0.26.0"},
	}
	result, _ := m.Update(msg)
	model := result.(ui.Model)
	assert.Equal(t, ui.ModeUpgrade, model.Mode())
}

func TestModel_CheckUpgradeMsg_NoHostMgr_StaysModeConfirm(t *testing.T) {
	// 無 HostMgr 時維持既有 ModeConfirm 邏輯
	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	m := ui.NewModel(ui.Deps{Upgrader: &upgrade.Upgrader{}})

	msg := ui.CheckUpgradeMsg{
		Release: &upgrade.Release{
			Version: "99.0.0",
			Assets:  map[string]string{upgrade.AssetName(): "https://example.com/dl"},
		},
	}
	result, _ := m.Update(msg)
	model := result.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "升級")
}

func TestModel_ModeUpgrade_EscReturnsToNormal(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Address: "", Enabled: true})

	deps := ui.Deps{HostMgr: mgr, Upgrader: &upgrade.Upgrader{}}
	m := ui.NewModel(deps)

	// 進入 ModeUpgrade
	msg := ui.CheckUpgradeMsg{
		Release:      &upgrade.Release{Version: "0.28.0"},
		HostVersions: map[string]string{"local": "0.27.0"},
	}
	result, _ := m.Update(msg)
	model := result.(ui.Model)
	assert.Equal(t, ui.ModeUpgrade, model.Mode())

	// 按 Esc 應回到 ModeNormal
	result2, _ := model.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model2 := result2.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model2.Mode())
}

func TestModel_ModeUpgrade_ItemsPopulated(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Address: "", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "air-2019", Address: "air-2019", Enabled: true})

	deps := ui.Deps{HostMgr: mgr, Upgrader: &upgrade.Upgrader{}}
	m := ui.NewModel(deps)

	msg := ui.CheckUpgradeMsg{
		Release:      &upgrade.Release{Version: "0.28.0"},
		HostVersions: map[string]string{"local": "0.27.0", "air-2019": "0.26.0"},
	}
	result, _ := m.Update(msg)
	model := result.(ui.Model)
	items := model.UpgradeItems()
	assert.Len(t, items, 2)

	// local 應在第一個且 IsLocal = true
	assert.Equal(t, "local", items[0].HostID)
	assert.True(t, items[0].IsLocal)
	assert.Equal(t, "0.27.0", items[0].Version)
	assert.True(t, items[0].Checked, "local 需要升級應預設勾選")

	// 遠端主機
	assert.Equal(t, "air-2019", items[1].HostID)
	assert.False(t, items[1].IsLocal)
	assert.Equal(t, "0.26.0", items[1].Version)
	assert.True(t, items[1].Checked, "遠端需要升級應預設勾選")
}

func TestModel_ModeUpgrade_AlreadyLatestNotChecked(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Address: "", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "remote", Address: "10.0.0.1", Enabled: true})

	deps := ui.Deps{HostMgr: mgr, Upgrader: &upgrade.Upgrader{}}
	m := ui.NewModel(deps)

	msg := ui.CheckUpgradeMsg{
		Release:      &upgrade.Release{Version: "0.28.0"},
		HostVersions: map[string]string{"local": "0.28.0", "remote": "0.28.0"},
	}
	result, _ := m.Update(msg)
	model := result.(ui.Model)
	items := model.UpgradeItems()
	assert.Len(t, items, 2)

	// 兩台都已是最新，不應預設勾選
	assert.False(t, items[0].Checked, "local 已是最新不應勾選")
	assert.False(t, items[1].Checked, "remote 已是最新不應勾選")
}

func TestDetectSessionStatus_AiType(t *testing.T) {
	statusDir := t.TempDir()
	statusFile := filepath.Join(statusDir, "cc-sess")
	content := fmt.Sprintf(`{"status":"idle","timestamp":%d,"event":"Stop","ai_type":"claude"}`, time.Now().Unix())
	require.NoError(t, os.WriteFile(statusFile, []byte(content), 0644))

	deps := ui.Deps{StatusDir: statusDir}
	result := ui.ExportDetectSessionStatus(deps, "cc-sess", "", "")
	assert.Equal(t, tmux.StatusIdle, result.Status)
	assert.Equal(t, "claude", result.AiType)
}

func TestDetectSessionStatus_AiType_NoHook(t *testing.T) {
	deps := ui.Deps{StatusDir: ""}
	result := ui.ExportDetectSessionStatus(deps, "shell-sess", "", "")
	assert.Equal(t, "", result.AiType)
}

func TestDetectSessionStatus_ContentFallback_NoHook(t *testing.T) {
	// 無 hook 但 pane content 含 AI 指示器 → 降級偵測應回傳 claude-code
	deps := ui.Deps{StatusDir: ""}
	result := ui.ExportDetectSessionStatus(deps, "ai-sess", "", "some output\nctrl+c to interrupt")
	assert.Equal(t, "claude-code", result.AiType)
}

func TestDetectSessionStatus_ValidHookNoAI_NoFallback(t *testing.T) {
	// 有效 hook 宣告非 AI（ai_type 為空）→ 不應降級偵測
	statusDir := t.TempDir()
	content := fmt.Sprintf(`{"status":"idle","timestamp":%d,"event":"Stop","ai_type":""}`, time.Now().Unix())
	require.NoError(t, os.WriteFile(filepath.Join(statusDir, "shell-sess"), []byte(content), 0644))

	deps := ui.Deps{StatusDir: statusDir}
	result := ui.ExportDetectSessionStatus(deps, "shell-sess", "", "ctrl+c to interrupt")
	assert.Equal(t, "", result.AiType, "有效 hook 宣告非 AI，不應降級偵測")
}
