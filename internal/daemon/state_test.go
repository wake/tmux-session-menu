package daemon

import (
	"context"
	"fmt"
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
	sm.scan()

	select {
	case snap := <-ch:
		require.Len(t, snap.Sessions, 1)
		assert.Equal(t, "dev", snap.Sessions[0].Name)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first broadcast")
	}

	// 無變更 — 不應廣播
	sm.scan()
	select {
	case <-ch:
		t.Fatal("should not broadcast when no change")
	case <-time.After(50 * time.Millisecond):
		// OK
	}

	// 變更 — 應廣播
	exec.listOutput = "dev:$1:1:/home:0:" + itoa(now) + "\nnew:$3:1:/tmp:0:" + itoa(now)
	sm.scan()

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

func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}
