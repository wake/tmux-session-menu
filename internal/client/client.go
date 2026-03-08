package client

import (
	"context"
	"fmt"
	"net"
	"os"
	osexec "os/exec"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/daemon"
)

// Client 封裝 gRPC client，提供與 daemon 通訊的方法。
type Client struct {
	conn   *grpc.ClientConn
	rpc    tsmv1.SessionManagerClient
	cfg    config.Config
	stream tsmv1.SessionManager_WatchClient
}

// Dial 連線到 daemon。如果連不上會嘗試自動啟動 daemon。
func Dial(cfg config.Config) (*Client, error) {
	sockPath := daemon.SocketPath(cfg)

	conn, err := tryConnect(sockPath)
	if err != nil {
		// 嘗試自動啟動 daemon
		if startErr := autoStartDaemon(sockPath); startErr != nil {
			return nil, fmt.Errorf("connect failed and auto-start failed: connect=%w, start=%v", err, startErr)
		}
		// 重新連線
		conn, err = tryConnect(sockPath)
		if err != nil {
			return nil, fmt.Errorf("connect after auto-start: %w", err)
		}
	}

	return &Client{
		conn: conn,
		rpc:  tsmv1.NewSessionManagerClient(conn),
		cfg:  cfg,
	}, nil
}

// DialSocket 連線到指定的 unix socket（用於 remote 模式，不觸發 auto-start daemon）。
func DialSocket(sockPath string) (*Client, error) {
	conn, err := tryConnect(sockPath)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: conn,
		rpc:  tsmv1.NewSessionManagerClient(conn),
	}, nil
}

// NewFromConn 從已有的 gRPC 連線建立 Client（用於測試）。
func NewFromConn(conn *grpc.ClientConn) *Client {
	return &Client{
		conn: conn,
		rpc:  tsmv1.NewSessionManagerClient(conn),
	}
}

// Close 關閉連線。
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Watch 開始監聽 Watch stream。
func (c *Client) Watch(ctx context.Context) error {
	stream, err := c.rpc.Watch(ctx, &tsmv1.WatchRequest{})
	if err != nil {
		return err
	}
	c.stream = stream
	return nil
}

// RecvSnapshot 從 Watch stream 接收下一個快照。
func (c *Client) RecvSnapshot() (*tsmv1.StateSnapshot, error) {
	if c.stream == nil {
		return nil, fmt.Errorf("watch stream not started")
	}
	return c.stream.Recv()
}

// CreateSession 建立新的 tmux session。
func (c *Client) CreateSession(ctx context.Context, name, path string) error {
	_, err := c.rpc.CreateSession(ctx, &tsmv1.CreateSessionRequest{Name: name, Path: path})
	return err
}

// KillSession 刪除指定的 tmux session。
func (c *Client) KillSession(ctx context.Context, name string) error {
	_, err := c.rpc.KillSession(ctx, &tsmv1.KillSessionRequest{Name: name})
	return err
}

// RenameSession 設定 session 的自訂顯示名稱，並可選擇重命名 tmux session。
func (c *Client) RenameSession(ctx context.Context, sessionName, customName, newSessionName string) error {
	_, err := c.rpc.RenameSession(ctx, &tsmv1.RenameSessionRequest{
		SessionName:    sessionName,
		CustomName:     customName,
		NewSessionName: newSessionName,
	})
	return err
}

// CreateGroup 建立新的群組。
func (c *Client) CreateGroup(ctx context.Context, name string, sortOrder int) error {
	_, err := c.rpc.CreateGroup(ctx, &tsmv1.CreateGroupRequest{
		Name:      name,
		SortOrder: int32(sortOrder),
	})
	return err
}

// RenameGroup 重命名群組。
func (c *Client) RenameGroup(ctx context.Context, groupID int64, newName string) error {
	_, err := c.rpc.RenameGroup(ctx, &tsmv1.RenameGroupRequest{
		GroupId: groupID,
		NewName: newName,
	})
	return err
}

// MoveSession 將 session 移動到指定群組。
func (c *Client) MoveSession(ctx context.Context, sessionName string, groupID int64, sortOrder int) error {
	_, err := c.rpc.MoveSession(ctx, &tsmv1.MoveSessionRequest{
		SessionName: sessionName,
		GroupId:     groupID,
		SortOrder:   int32(sortOrder),
	})
	return err
}

// ReorderGroup 重新排序群組。
func (c *Client) ReorderGroup(ctx context.Context, groupID int64, newSortOrder int) error {
	_, err := c.rpc.Reorder(ctx, &tsmv1.ReorderRequest{
		Target: &tsmv1.ReorderRequest_Group{
			Group: &tsmv1.GroupReorder{GroupId: groupID, NewSortOrder: int32(newSortOrder)},
		},
	})
	return err
}

// ReorderSession 重新排序群組內的 session。
func (c *Client) ReorderSession(ctx context.Context, sessionName string, groupID int64, newSortOrder int) error {
	_, err := c.rpc.Reorder(ctx, &tsmv1.ReorderRequest{
		Target: &tsmv1.ReorderRequest_Session{
			Session: &tsmv1.SessionReorder{
				SessionName:  sessionName,
				GroupId:      groupID,
				NewSortOrder: int32(newSortOrder),
			},
		},
	})
	return err
}

// ToggleCollapse 切換群組的展開/收合狀態。
func (c *Client) ToggleCollapse(ctx context.Context, groupID int64) error {
	_, err := c.rpc.ToggleCollapse(ctx, &tsmv1.ToggleCollapseRequest{GroupId: groupID})
	return err
}

// DaemonStatus 取得 daemon 狀態。
func (c *Client) DaemonStatus(ctx context.Context) (*tsmv1.DaemonStatusResponse, error) {
	return c.rpc.DaemonStatus(ctx, &emptypb.Empty{})
}

// GetConfig 取得遠端設定。
func (c *Client) GetConfig(ctx context.Context) (*tsmv1.GetConfigResponse, error) {
	return c.rpc.GetConfig(ctx, &tsmv1.GetConfigRequest{})
}

// SetConfig 設定遠端設定值。
func (c *Client) SetConfig(ctx context.Context, values []*tsmv1.ConfigValue) error {
	_, err := c.rpc.SetConfig(ctx, &tsmv1.SetConfigRequest{Values: values})
	return err
}

// tryConnect 嘗試連線到 unix socket。
func tryConnect(sockPath string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		"unix://"+sockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial unix socket %s: %w", sockPath, err)
	}

	// 驗證連線是否可用（socket 是否存在）
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := tsmv1.NewSessionManagerClient(conn).DaemonStatus(ctx, &emptypb.Empty{}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("connect to daemon at %s: %w", sockPath, err)
	}

	return conn, nil
}

// autoStartDaemon fork 一個背景 daemon 程序，並等待它就緒。
func autoStartDaemon(sockPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	cmd := osexec.Command(exe, "daemon", "start")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = nil // 使用預設，讓子程序在背景執行

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	// 釋放子程序，不等待它結束
	go func() { _ = cmd.Wait() }()

	// 等待 socket 就緒（最多 3 秒）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", sockPath, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("daemon did not become ready within 3 seconds")
}
