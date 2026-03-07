package client

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
	"github.com/wake/tmux-session-menu/internal/daemon"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bufSize = 1024 * 1024

// fakeExecutor 模擬 tmux 指令執行。
type fakeExecutor struct {
	listOutput string
}

func (f *fakeExecutor) Execute(args ...string) (string, error) {
	if len(args) > 0 && args[0] == "list-sessions" {
		return f.listOutput, nil
	}
	if len(args) > 0 && args[0] == "list-panes" {
		return "", nil
	}
	if len(args) > 0 && args[0] == "capture-pane" {
		return "", nil
	}
	return "", nil
}

// setupTestClient 建立一個 bufconn-based 的測試環境，回傳 Client。
func setupTestClient(t *testing.T, exec tmux.Executor) (*Client, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	hub := daemon.NewWatcherHub()
	mgr := tmux.NewManager(exec)

	tmpDB := t.TempDir() + "/test.db"
	st, err := store.Open(tmpDB)
	require.NoError(t, err)

	sm := daemon.NewStateManager(mgr, st, config.Default(), "", hub)
	sm.Scan()

	svc := daemon.NewService(mgr, st, hub, sm)
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

	client := NewFromConn(conn)

	cleanup := func() {
		client.Close()
		srv.GracefulStop()
		hub.Close()
		st.Close()
	}

	return client, cleanup
}

func TestClient_WatchAndRecv(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:0:%d", now),
	}
	client, cleanup := setupTestClient(t, exec)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := client.Watch(ctx)
	require.NoError(t, err)

	snap, err := client.RecvSnapshot()
	require.NoError(t, err)
	require.Len(t, snap.Sessions, 1)
	assert.Equal(t, "dev", snap.Sessions[0].Name)
}

func TestClient_CreateGroup(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	client, cleanup := setupTestClient(t, exec)
	defer cleanup()

	err := client.CreateGroup(context.Background(), "work", 0)
	require.NoError(t, err)
}

func TestClient_RenameSession(t *testing.T) {
	now := time.Now().Unix()
	exec := &fakeExecutor{
		listOutput: fmt.Sprintf("dev:$1:1:/home:0:%d", now),
	}
	client, cleanup := setupTestClient(t, exec)
	defer cleanup()

	err := client.RenameSession(context.Background(), "dev", "Development", "")
	require.NoError(t, err)
}

func TestClient_RenameGroup(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	client, cleanup := setupTestClient(t, exec)
	defer cleanup()

	// 先建立群組
	err := client.CreateGroup(context.Background(), "old-name", 0)
	require.NoError(t, err)

	// 重命名群組
	err = client.RenameGroup(context.Background(), 1, "new-name")
	require.NoError(t, err)
}

func TestClient_ToggleCollapse(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	client, cleanup := setupTestClient(t, exec)
	defer cleanup()

	err := client.CreateGroup(context.Background(), "work", 0)
	require.NoError(t, err)

	err = client.ToggleCollapse(context.Background(), 1)
	require.NoError(t, err)
}

func TestClient_DaemonStatus(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	client, cleanup := setupTestClient(t, exec)
	defer cleanup()

	resp, err := client.DaemonStatus(context.Background())
	require.NoError(t, err)
	assert.True(t, resp.Pid > 0)
}

func TestClient_RecvWithoutWatch(t *testing.T) {
	exec := &fakeExecutor{listOutput: ""}
	client, cleanup := setupTestClient(t, exec)
	defer cleanup()

	_, err := client.RecvSnapshot()
	assert.Error(t, err)
}

func TestClient_DialSocket_InvalidPath(t *testing.T) {
	_, err := DialSocket("/nonexistent/path/tsm.sock")
	assert.Error(t, err)
}
