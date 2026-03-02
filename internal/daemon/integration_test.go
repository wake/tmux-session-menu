package daemon

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_WatchReceivesOperationChanges 驗證端到端流程：
// Watch stream 連線 → 執行操作 → 收到更新快照。
func TestIntegration_WatchReceivesOperationChanges(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:0:%d", now),
	}

	lis := bufconn.Listen(bufSize)
	hub := NewWatcherHub()
	defer hub.Close()
	mgr := tmux.NewManager(exec)

	tmpDB := t.TempDir() + "/test.db"
	st, err := store.Open(tmpDB)
	require.NoError(t, err)
	defer st.Close()

	sm := NewStateManager(mgr, st, config.Default(), "", hub)
	sm.Scan()

	svc := NewService(mgr, st, hub, sm)
	srv := grpc.NewServer()
	tsmv1.RegisterSessionManagerServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	defer srv.GracefulStop()

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := tsmv1.NewSessionManagerClient(conn)

	// 建立 Watch stream
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Watch(ctx, &tsmv1.WatchRequest{})
	require.NoError(t, err)

	// 1. 接收初始快照
	snap, err := stream.Recv()
	require.NoError(t, err)
	require.Len(t, snap.Sessions, 1)
	assert.Equal(t, "dev", snap.Sessions[0].Name)
	assert.Empty(t, snap.Groups)

	// 2. 建立群組 → 應收到更新
	_, err = client.CreateGroup(ctx, &tsmv1.CreateGroupRequest{Name: "work", SortOrder: 0})
	require.NoError(t, err)

	snap, err = stream.Recv()
	require.NoError(t, err)
	require.Len(t, snap.Groups, 1)
	assert.Equal(t, "work", snap.Groups[0].Name)

	// 3. 移動 session 到群組 → 應收到更新
	_, err = client.MoveSession(ctx, &tsmv1.MoveSessionRequest{
		SessionName: "dev",
		GroupId:     snap.Groups[0].Id,
	})
	require.NoError(t, err)

	snap, err = stream.Recv()
	require.NoError(t, err)
	require.Len(t, snap.Sessions, 1)
	assert.Equal(t, "work", snap.Sessions[0].GroupName)

	// 4. 設定自訂名稱 → 應收到更新
	_, err = client.RenameSession(ctx, &tsmv1.RenameSessionRequest{
		SessionName: "dev",
		CustomName:  "開發環境",
	})
	require.NoError(t, err)

	snap, err = stream.Recv()
	require.NoError(t, err)
	require.Len(t, snap.Sessions, 1)
	assert.Equal(t, "開發環境", snap.Sessions[0].CustomName)

	// 5. 切換群組收合 → 應收到更新
	_, err = client.ToggleCollapse(ctx, &tsmv1.ToggleCollapseRequest{GroupId: snap.Groups[0].Id})
	require.NoError(t, err)

	snap, err = stream.Recv()
	require.NoError(t, err)
	require.Len(t, snap.Groups, 1)
	assert.True(t, snap.Groups[0].Collapsed)
}

// TestIntegration_MultipleWatchers 驗證多個 Watch stream 都能收到更新。
func TestIntegration_MultipleWatchers(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:0:%d", now),
	}

	lis := bufconn.Listen(bufSize)
	hub := NewWatcherHub()
	defer hub.Close()
	mgr := tmux.NewManager(exec)

	tmpDB := t.TempDir() + "/test.db"
	st, err := store.Open(tmpDB)
	require.NoError(t, err)
	defer st.Close()

	sm := NewStateManager(mgr, st, config.Default(), "", hub)
	sm.Scan()

	svc := NewService(mgr, st, hub, sm)
	srv := grpc.NewServer()
	tsmv1.RegisterSessionManagerServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	defer srv.GracefulStop()

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := tsmv1.NewSessionManagerClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 建立兩個 Watch stream
	stream1, err := client.Watch(ctx, &tsmv1.WatchRequest{})
	require.NoError(t, err)
	stream2, err := client.Watch(ctx, &tsmv1.WatchRequest{})
	require.NoError(t, err)

	// 兩者都應收到初始快照
	snap1, err := stream1.Recv()
	require.NoError(t, err)
	snap2, err := stream2.Recv()
	require.NoError(t, err)
	assert.Len(t, snap1.Sessions, 1)
	assert.Len(t, snap2.Sessions, 1)

	// 執行操作
	_, err = client.CreateGroup(ctx, &tsmv1.CreateGroupRequest{Name: "test"})
	require.NoError(t, err)

	// 兩個 stream 都應收到更新
	snap1, err = stream1.Recv()
	require.NoError(t, err)
	assert.Len(t, snap1.Groups, 1)

	snap2, err = stream2.Recv()
	require.NoError(t, err)
	assert.Len(t, snap2.Groups, 1)
}
