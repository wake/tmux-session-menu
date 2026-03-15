package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeExecutor 模擬 tmux 指令執行。
type fakeExecutor struct {
	listOutput string
	listErr    error
	calls      [][]string
}

func (f *fakeExecutor) Execute(args ...string) (string, error) {
	f.calls = append(f.calls, args)
	if len(args) > 0 && args[0] == "list-sessions" {
		return f.listOutput, f.listErr
	}
	if len(args) > 0 && args[0] == "list-panes" {
		return "", nil
	}
	if len(args) > 0 && args[0] == "capture-pane" {
		return "", nil
	}
	return "", nil
}

func TestBuildSnapshot_BasicSessions(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: "dev:$1:1:/home/user:0:" + itoa(now) + "\ntest:$2:1:/tmp:1:" + itoa(now),
	}
	mgr := tmux.NewManager(exec)
	hub := NewWatcherHub()
	defer hub.Close()

	sm := NewStateManager(mgr, nil, config.Default(), "", hub)
	snap := sm.BuildSnapshot()

	require.NotNil(t, snap)
	require.Len(t, snap.Sessions, 2)
	assert.Equal(t, "dev", snap.Sessions[0].Name)
	assert.Equal(t, "test", snap.Sessions[1].Name)
	assert.False(t, snap.Sessions[0].Attached)
	assert.True(t, snap.Sessions[1].Attached)
}

func TestBuildSnapshot_NoSessions(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	mgr := tmux.NewManager(exec)
	hub := NewWatcherHub()
	defer hub.Close()

	sm := NewStateManager(mgr, nil, config.Default(), "", hub)
	snap := sm.BuildSnapshot()

	require.NotNil(t, snap)
	assert.Empty(t, snap.Sessions)
}

func TestBuildSnapshot_NilManager(t *testing.T) {
	hub := NewWatcherHub()
	defer hub.Close()

	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	snap := sm.BuildSnapshot()

	require.NotNil(t, snap)
	assert.Empty(t, snap.Sessions)
}

func TestStateManager_BroadcastOnChange(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: "dev:$1:1:/home:0:" + itoa(now),
	}
	mgr := tmux.NewManager(exec)
	hub := NewWatcherHub()
	defer hub.Close()

	ch := hub.Register()
	defer hub.Unregister(ch)

	sm := NewStateManager(mgr, nil, config.Default(), "", hub)

	// 首次掃描應廣播
	sm.Scan()

	select {
	case snap := <-ch:
		require.Len(t, snap.Sessions, 1)
		assert.Equal(t, "dev", snap.Sessions[0].Name)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first broadcast")
	}

	// 無變更 — 不應廣播
	sm.Scan()
	select {
	case <-ch:
		t.Fatal("should not broadcast when no change")
	case <-time.After(50 * time.Millisecond):
		// OK
	}

	// 變更 — 應廣播
	exec.listOutput = "dev:$1:1:/home:0:" + itoa(now) + "\nnew:$3:1:/tmp:0:" + itoa(now)
	sm.Scan()

	select {
	case snap := <-ch:
		require.Len(t, snap.Sessions, 2)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for change broadcast")
	}
}

func TestStateManager_RunStopsOnCancel(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	mgr := tmux.NewManager(exec)
	hub := NewWatcherHub()
	defer hub.Close()

	sm := NewStateManager(mgr, nil, config.Default(), "", hub)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sm.Run(ctx)
		close(done)
	}()

	// 等一小段讓 Run 啟動
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

func TestToProtoSnapshot(t *testing.T) {
	now := time.Now()
	sessions := []tmux.Session{
		{
			Name:       "s1",
			ID:         "$1",
			Path:       "/home",
			Attached:   true,
			Activity:   now,
			Status:     tmux.StatusRunning,
			AIModel:    "claude-opus-4-6",
			GroupName:  "work",
			SortOrder:  1,
			CustomName: "My Session",
		},
	}
	groups := []store.Group{
		{ID: 1, Name: "work", SortOrder: 0, Collapsed: false},
	}

	snap := toProtoSnapshot(sessions, groups)

	require.Len(t, snap.Sessions, 1)
	require.Len(t, snap.Groups, 1)

	s := snap.Sessions[0]
	assert.Equal(t, "s1", s.Name)
	assert.Equal(t, "$1", s.Id)
	assert.Equal(t, "/home", s.Path)
	assert.True(t, s.Attached)
	assert.Equal(t, tsmv1.SessionStatus_SESSION_STATUS_RUNNING, s.Status)
	assert.Equal(t, "claude-opus-4-6", s.AiModel)
	assert.Equal(t, "work", s.GroupName)
	assert.Equal(t, int32(1), s.SortOrder)
	assert.Equal(t, "My Session", s.CustomName)

	g := snap.Groups[0]
	assert.Equal(t, int64(1), g.Id)
	assert.Equal(t, "work", g.Name)
}

func TestSyncUserOptions_UsesSessionID(t *testing.T) {
	now := time.Now().Unix()
	// session name 為純數字 "0"，session ID 為 "$5"
	exec := &fakeExecutor{
		listOutput: "0:$5:1:/home:0:" + itoa(now),
	}
	mgr := tmux.NewManager(exec)
	hub := NewWatcherHub()
	defer hub.Close()

	// 建立含 custom name 的 store
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	defer st.Close()
	require.NoError(t, st.SetCustomName("0", "我的專案"))

	sm := NewStateManager(mgr, st, config.Default(), "", hub)

	// 執行掃描，觸發 syncUserOptions
	sm.Scan()

	// 檢查 fakeExecutor 記錄的 set-option 呼叫
	var setOptionCalls [][]string
	for _, call := range exec.calls {
		if len(call) > 0 && call[0] == "set-option" {
			setOptionCalls = append(setOptionCalls, call)
		}
	}

	require.Len(t, setOptionCalls, 1, "應有一次 set-option 呼叫")
	// 關鍵：target 必須是 session ID "$5"，而非 session name "0"
	assert.Equal(t, []string{"set-option", "-t", "$5", "@tsm_name", "我的專案"}, setOptionCalls[0])
}

func TestSyncUserOptions_UnsetsRemovedCustomName(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: "myapp:$3:1:/home:0:" + itoa(now),
	}
	mgr := tmux.NewManager(exec)
	hub := NewWatcherHub()
	defer hub.Close()

	st, err := store.Open(":memory:")
	require.NoError(t, err)
	defer st.Close()

	sm := NewStateManager(mgr, st, config.Default(), "", hub)

	// 先設定 custom name 再掃描
	require.NoError(t, st.SetCustomName("myapp", "App"))
	sm.Scan()

	// 清除 custom name 再掃描
	require.NoError(t, st.SetCustomName("myapp", ""))
	exec.calls = nil // 重設記錄
	sm.Scan()

	var unsetCalls [][]string
	for _, call := range exec.calls {
		if len(call) >= 2 && call[0] == "set-option" && len(call) >= 4 && call[3] == "-u" {
			unsetCalls = append(unsetCalls, call)
		}
	}

	require.Len(t, unsetCalls, 1, "應有一次 unset 呼叫")
	assert.Equal(t, []string{"set-option", "-t", "$3", "-u", "@tsm_name"}, unsetCalls[0])
}

func TestDetectStatus_AiType_FromHook(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "cc-sess")
	content := fmt.Sprintf(`{"status":"idle","timestamp":%d,"event":"Stop","ai_type":"claude"}`, time.Now().Unix())
	require.NoError(t, os.WriteFile(statusFile, []byte(content), 0644))

	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), dir, hub)

	result := sm.detectStatus("cc-sess", "", "")
	assert.Equal(t, tmux.StatusIdle, result.Status)
	assert.Equal(t, "claude", result.AiType)
}

func TestDetectStatus_AiType_ExpiredWithPresence(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "cc-sess")
	content := fmt.Sprintf(`{"status":"idle","timestamp":%d,"event":"Stop","ai_type":"claude"}`, time.Now().Add(-2*time.Hour).Unix())
	require.NoError(t, os.WriteFile(statusFile, []byte(content), 0644))

	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), dir, hub)

	result := sm.detectStatus("cc-sess", "", "some output\nclaude-sonnet-4-20250514\n> ")
	assert.Equal(t, "claude", result.AiType)
}

func TestDetectStatus_AiType_ExpiredNoPresence(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "cc-sess")
	content := fmt.Sprintf(`{"status":"idle","timestamp":%d,"event":"Stop","ai_type":"claude"}`, time.Now().Add(-2*time.Hour).Unix())
	require.NoError(t, os.WriteFile(statusFile, []byte(content), 0644))

	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), dir, hub)

	result := sm.detectStatus("cc-sess", "", "user@host:~$ ls\nfile1 file2\nuser@host:~$ ")
	assert.Equal(t, "", result.AiType)
}

func TestDetectStatus_AiType_ContentFallback(t *testing.T) {
	// 無 hook 狀態檔，但 pane content 有 Claude Code 提示符
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)

	result := sm.detectStatus("no-hook-sess", "", "some output\n> ")
	assert.Equal(t, "claude-code", result.AiType)
}

func TestDetectStatus_AiType_ContentFallbackInterrupt(t *testing.T) {
	// 無 hook 狀態檔，但 pane content 有 Claude Code 工作中指示
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)

	result := sm.detectStatus("no-hook-sess", "", "Working on task...\nesc to interrupt")
	assert.Equal(t, "claude-code", result.AiType)
}

func TestDetectStatus_AiType_NoContentFallbackForPlainShell(t *testing.T) {
	// 無 hook 狀態檔，pane content 是一般 shell → aiType 應為空
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)

	result := sm.detectStatus("plain-sess", "", "user@host:~$ ls\nfile1 file2\nuser@host:~$ ")
	assert.Equal(t, "", result.AiType)
}

func TestDetectStatus_AiType_ValidHookEmptyAiType_NoFallback(t *testing.T) {
	// 有效 hook 且 ai_type 為空 → hook 已明確表示非 AI session，content fallback 不應覆蓋
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "non-ai-sess")
	content := fmt.Sprintf(`{"status":"idle","timestamp":%d,"event":"Stop","ai_type":""}`, time.Now().Unix())
	require.NoError(t, os.WriteFile(statusFile, []byte(content), 0644))

	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), dir, hub)

	// pane content 有 Claude Code 提示符，但有效 hook 說不是 AI → 不應 fallback
	result := sm.detectStatus("non-ai-sess", "", "some output\n> ")
	assert.Equal(t, "", result.AiType)
}

func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}
