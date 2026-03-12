package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/version"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bufSize = 1024 * 1024

// setupTestService 建立一個 bufconn-based 的測試環境。
func setupTestService(t *testing.T, exec tmux.Executor) (tsmv1.SessionManagerClient, *StateManager, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	hub := NewWatcherHub()
	mgr := tmux.NewManager(exec)

	// 使用 temp DB
	tmpDB := t.TempDir() + "/test.db"
	st, err := store.Open(tmpDB)
	require.NoError(t, err)

	sm := NewStateManager(mgr, st, config.Default(), "", hub)
	// 立即做一次 scan，確保有初始快照
	sm.Scan()

	svc := NewService(mgr, st, hub, nil, nil, sm)
	srv := grpc.NewServer()
	tsmv1.RegisterSessionManagerServer(srv, svc)

	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	client := tsmv1.NewSessionManagerClient(conn)

	cleanup := func() {
		conn.Close()
		srv.GracefulStop()
		hub.Close()
		st.Close()
	}

	return client, sm, cleanup
}

func TestService_Watch_InitialSnapshot(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:0:%d\ntest:$2:1:/tmp:1:%d", now, now),
	}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Watch(ctx, &tsmv1.WatchRequest{})
	require.NoError(t, err)

	// 應收到初始快照
	snap, err := stream.Recv()
	require.NoError(t, err)
	require.Len(t, snap.Sessions, 2)
	assert.Equal(t, "dev", snap.Sessions[0].Name)
	assert.Equal(t, "test", snap.Sessions[1].Name)
}

func TestService_Watch_PushOnChange(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:0:%d", now),
	}
	client, sm, cleanup := setupTestService(t, exec)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Watch(ctx, &tsmv1.WatchRequest{})
	require.NoError(t, err)

	// 初始快照
	snap, err := stream.Recv()
	require.NoError(t, err)
	require.Len(t, snap.Sessions, 1)

	// 模擬變更
	exec.listOutput = fmt.Sprintf("dev:$1:1:/home:0:%d\nnew:$3:1:/tmp:0:%d", now, now)
	sm.Scan()

	// 應收到變更快照
	snap, err = stream.Recv()
	require.NoError(t, err)
	require.Len(t, snap.Sessions, 2)
}

func TestService_CreateGroup(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	ctx := context.Background()
	_, err := client.CreateGroup(ctx, &tsmv1.CreateGroupRequest{Name: "work", SortOrder: 0})
	require.NoError(t, err)
}

func TestService_MoveSession(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:0:%d", now),
	}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	ctx := context.Background()

	// 建立群組
	_, err := client.CreateGroup(ctx, &tsmv1.CreateGroupRequest{Name: "work"})
	require.NoError(t, err)

	// 移動 session 到群組
	_, err = client.MoveSession(ctx, &tsmv1.MoveSessionRequest{
		SessionName: "dev",
		GroupId:     1,
	})
	require.NoError(t, err)
}

func TestService_ToggleCollapse(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	ctx := context.Background()

	// 建立群組
	_, err := client.CreateGroup(ctx, &tsmv1.CreateGroupRequest{Name: "work"})
	require.NoError(t, err)

	// 切換收合
	_, err = client.ToggleCollapse(ctx, &tsmv1.ToggleCollapseRequest{GroupId: 1})
	require.NoError(t, err)
}

func TestService_RenameSession(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:0:%d", now),
	}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	ctx := context.Background()
	_, err := client.RenameSession(ctx, &tsmv1.RenameSessionRequest{
		SessionName: "dev",
		CustomName:  "Development",
	})
	require.NoError(t, err)
}

func TestService_RenameSession_WithNewSessionName(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:0:%d", now),
	}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	ctx := context.Background()

	// 先設定自訂名稱
	_, err := client.RenameSession(ctx, &tsmv1.RenameSessionRequest{
		SessionName: "dev",
		CustomName:  "開發環境",
	})
	require.NoError(t, err)

	// 使用 new_session_name 重命名 tmux session
	_, err = client.RenameSession(ctx, &tsmv1.RenameSessionRequest{
		SessionName:    "dev",
		CustomName:     "新名稱",
		NewSessionName: "dev-renamed",
	})
	require.NoError(t, err)

	// 驗證 tmux rename-session 有被呼叫
	found := false
	for _, call := range exec.calls {
		if len(call) >= 4 && call[0] == "rename-session" {
			found = true
			assert.Equal(t, "dev", call[2])
			assert.Equal(t, "dev-renamed", call[3])
		}
	}
	assert.True(t, found, "應呼叫 tmux rename-session")
}

func TestService_DaemonStatus(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	ctx := context.Background()
	resp, err := client.DaemonStatus(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	assert.True(t, resp.Pid > 0)
	assert.NotNil(t, resp.StartedAt)
	assert.Equal(t, version.Version, resp.Version)
}

func TestService_Reorder_Group(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	ctx := context.Background()
	_, err := client.CreateGroup(ctx, &tsmv1.CreateGroupRequest{Name: "a", SortOrder: 0})
	require.NoError(t, err)
	_, err = client.CreateGroup(ctx, &tsmv1.CreateGroupRequest{Name: "b", SortOrder: 1})
	require.NoError(t, err)

	// 把 group 1 的排序改成 10
	_, err = client.Reorder(ctx, &tsmv1.ReorderRequest{
		Target: &tsmv1.ReorderRequest_Group{
			Group: &tsmv1.GroupReorder{GroupId: 1, NewSortOrder: 10},
		},
	})
	require.NoError(t, err)
}

// --- Upload RPC 測試 ---

func TestService_GetUploadTarget_Default(t *testing.T) {
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	svc := NewService(nil, nil, hub, nil, nil, sm)

	resp, err := svc.GetUploadTarget(context.Background(), &tsmv1.GetUploadTargetRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsRemote {
		t.Error("expected local session")
	}
	if resp.UploadMode {
		t.Error("expected upload mode off")
	}
	if resp.UploadPath != "/tmp/iterm-upload" {
		t.Errorf("got upload_path %q, want /tmp/iterm-upload", resp.UploadPath)
	}
}

func TestService_GetUploadTarget_UploadMode(t *testing.T) {
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	svc := NewService(nil, nil, hub, nil, nil, sm)

	sm.uploadState.SetMode(true, "my-session")

	resp, err := svc.GetUploadTarget(context.Background(), &tsmv1.GetUploadTargetRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.UploadMode {
		t.Error("expected upload mode on")
	}
	if resp.SessionName != "my-session" {
		t.Errorf("got session %q, want my-session", resp.SessionName)
	}
}

func TestService_SetUploadMode(t *testing.T) {
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	svc := NewService(nil, nil, hub, nil, nil, sm)

	_, err := svc.SetUploadMode(context.Background(), &tsmv1.SetUploadModeRequest{
		Enabled:     true,
		SessionName: "test-sess",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !sm.uploadState.IsUploadMode() {
		t.Error("expected upload mode on")
	}
	if sm.uploadState.SessionName() != "test-sess" {
		t.Errorf("got %q, want test-sess", sm.uploadState.SessionName())
	}
}

func TestService_ReportUploadResult(t *testing.T) {
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	svc := NewService(nil, nil, hub, nil, nil, sm)

	// 啟用上傳模式，BuildSnapshot 才會 drain 事件
	sm.uploadState.SetMode(true, "test-sess")

	_, err := svc.ReportUploadResult(context.Background(), &tsmv1.ReportUploadResultRequest{
		SessionName: "test-sess",
		Files: []*tsmv1.UploadedFile{
			{LocalPath: "/tmp/a.png", RemotePath: "/data/a.png", SizeBytes: 512},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Scan() 已被呼叫，事件透過 BuildSnapshot 被 drain 到 snapshot 中
	snap := sm.Snapshot()
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if len(snap.UploadEvents) != 1 {
		t.Fatalf("got %d events, want 1", len(snap.UploadEvents))
	}
	if len(snap.UploadEvents[0].Files) != 1 {
		t.Fatalf("got %d files, want 1", len(snap.UploadEvents[0].Files))
	}
}

func TestService_ReportUploadResult_Error(t *testing.T) {
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	svc := NewService(nil, nil, hub, nil, nil, sm)

	// 啟用上傳模式，BuildSnapshot 才會 drain 事件
	sm.uploadState.SetMode(true, "test-sess")

	_, err := svc.ReportUploadResult(context.Background(), &tsmv1.ReportUploadResultRequest{
		SessionName: "test-sess",
		Error:       "scp timeout",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Scan() 已被呼叫，事件透過 BuildSnapshot 被 drain 到 snapshot 中
	snap := sm.Snapshot()
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if len(snap.UploadEvents) != 1 {
		t.Fatalf("got %d events, want 1", len(snap.UploadEvents))
	}
	if snap.UploadEvents[0].Error != "scp timeout" {
		t.Errorf("got error %q, want scp timeout", snap.UploadEvents[0].Error)
	}
}

// TestService_GetUploadTarget_AiType 驗證：hook 狀態含 ai_type=claude 時，
// 即使 status=idle，IsClaudeActive 仍應為 true。
func TestService_GetUploadTarget_AiType(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:1:%d", now),
	}

	hub := NewWatcherHub()
	mgr := tmux.NewManager(exec)
	statusDir := t.TempDir()

	// 寫入 hook 狀態：ai_type=claude，status=idle
	statusFile := filepath.Join(statusDir, "dev")
	content := fmt.Sprintf(`{"status":"idle","timestamp":%d,"event":"Stop","ai_type":"claude"}`, now)
	require.NoError(t, os.WriteFile(statusFile, []byte(content), 0644))

	sm := NewStateManager(mgr, nil, config.Default(), statusDir, hub)
	sm.Scan()

	svc := NewService(mgr, nil, hub, nil, nil, sm)
	resp, err := svc.GetUploadTarget(context.Background(), &tsmv1.GetUploadTargetRequest{})
	require.NoError(t, err)

	assert.True(t, resp.IsClaudeActive, "ai_type=claude 應讓 IsClaudeActive=true，即使 status=idle")
	assert.Equal(t, "dev", resp.SessionName)
}

// TestService_GetUploadTarget_NoAiType 驗證：無 hook 狀態時，IsClaudeActive 應為 false。
func TestService_GetUploadTarget_NoAiType(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:1:%d", now),
	}

	hub := NewWatcherHub()
	mgr := tmux.NewManager(exec)

	sm := NewStateManager(mgr, nil, config.Default(), "", hub)
	sm.Scan()

	svc := NewService(mgr, nil, hub, nil, nil, sm)
	resp, err := svc.GetUploadTarget(context.Background(), &tsmv1.GetUploadTargetRequest{})
	require.NoError(t, err)

	assert.False(t, resp.IsClaudeActive, "無 ai_type 時 IsClaudeActive 應為 false")
}
