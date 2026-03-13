package daemon

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
)

// --- fake 測試雙 ---

// fakeRemoteClient 模擬遠端 gRPC 連線，透過 channel 驅動快照推送。
type fakeRemoteClient struct {
	snapCh   chan *tsmv1.StateSnapshot
	closeCh  chan struct{}
	closed   atomic.Bool
	killFn   func(ctx context.Context, name string) error
	renameFn func(ctx context.Context, sessionName, customName, newSessionName string) error
	createFn func(ctx context.Context, name, path, command string) error
	moveFn   func(ctx context.Context, sessionName string, groupID int64, sortOrder int) error
}

func newFakeRemoteClient() *fakeRemoteClient {
	return &fakeRemoteClient{
		snapCh:  make(chan *tsmv1.StateSnapshot, 10),
		closeCh: make(chan struct{}),
	}
}

func (f *fakeRemoteClient) RecvSnapshot() (*tsmv1.StateSnapshot, error) {
	select {
	case snap, ok := <-f.snapCh:
		if !ok {
			return nil, fmt.Errorf("stream closed")
		}
		return snap, nil
	case <-f.closeCh:
		return nil, fmt.Errorf("client closed")
	}
}

func (f *fakeRemoteClient) Close() error {
	if f.closed.CompareAndSwap(false, true) {
		close(f.closeCh)
	}
	return nil
}

func (f *fakeRemoteClient) KillSession(ctx context.Context, name string) error {
	if f.killFn != nil {
		return f.killFn(ctx, name)
	}
	return nil
}

func (f *fakeRemoteClient) RenameSession(ctx context.Context, sessionName, customName, newSessionName string) error {
	if f.renameFn != nil {
		return f.renameFn(ctx, sessionName, customName, newSessionName)
	}
	return nil
}

func (f *fakeRemoteClient) CreateSession(ctx context.Context, name, path, command string) error {
	if f.createFn != nil {
		return f.createFn(ctx, name, path, command)
	}
	return nil
}

func (f *fakeRemoteClient) MoveSession(ctx context.Context, sessionName string, groupID int64, sortOrder int) error {
	if f.moveFn != nil {
		return f.moveFn(ctx, sessionName, groupID, sortOrder)
	}
	return nil
}

// --- 測試 ---

// TestHubManager_StartRemoteHosts_SetsConnecting 驗證啟動後遠端主機進入 CONNECTING 狀態。
func TestHubManager_StartRemoteHosts_SetsConnecting(t *testing.T) {
	mhub := NewMultiHostHub()
	defer mhub.Close()
	mgr := NewHubManager(mhub)

	// 使用永遠阻塞的 dialFn，讓狀態停留在 CONNECTING
	blockCh := make(chan struct{})
	defer close(blockCh)
	mgr.dialFn = func(ctx context.Context, address string) (HubRemoteClient, func(), error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}

	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartRemoteHosts(ctx)
	defer mgr.Close()

	// 等待 goroutine 啟動
	time.Sleep(50 * time.Millisecond)

	snap := mgr.Snapshot()
	require.Len(t, snap.Hosts, 1)
	assert.Equal(t, tsmv1.HostStatus_HOST_STATUS_CONNECTING, snap.Hosts[0].Status)
}

// TestHubManager_StartRemoteHosts_SkipsLocalAndDisabled 驗證不啟動 local 和 disabled 的主機。
func TestHubManager_StartRemoteHosts_SkipsLocalAndDisabled(t *testing.T) {
	mhub := NewMultiHostHub()
	defer mhub.Close()
	mgr := NewHubManager(mhub)

	var dialCount atomic.Int32
	mgr.dialFn = func(ctx context.Context, address string) (HubRemoteClient, func(), error) {
		dialCount.Add(1)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}

	mgr.AddHost(config.HostEntry{Name: "local", Address: "", Enabled: true})        // local → 跳過
	mgr.AddHost(config.HostEntry{Name: "off", Address: "off-host", Enabled: false}) // disabled → 跳過

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartRemoteHosts(ctx)
	defer mgr.Close()

	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, int32(0), dialCount.Load(), "不應對 local 或 disabled 主機發起連線")

	// 狀態應維持 DISABLED
	snap := mgr.Snapshot()
	for _, h := range snap.Hosts {
		assert.Equal(t, tsmv1.HostStatus_HOST_STATUS_DISABLED, h.Status)
	}
}

// TestHubManager_StartRemoteHosts_ConnectsAndBroadcasts 驗證連線成功後廣播快照。
func TestHubManager_StartRemoteHosts_ConnectsAndBroadcasts(t *testing.T) {
	mhub := NewMultiHostHub()
	defer mhub.Close()
	mgr := NewHubManager(mhub)

	fakeClient := newFakeRemoteClient()
	mgr.dialFn = func(ctx context.Context, address string) (HubRemoteClient, func(), error) {
		return fakeClient, func() {}, nil
	}

	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	// 註冊接收廣播
	ch := mhub.Register()
	defer mhub.Unregister(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartRemoteHosts(ctx)
	defer mgr.Close()

	// 等待連線完成（dialFn 立即成功）
	// 第一個廣播：CONNECTING 狀態
	waitBroadcast(t, ch, 2*time.Second)

	// 推送一個快照
	fakeClient.snapCh <- &tsmv1.StateSnapshot{
		Sessions: []*tsmv1.Session{{Name: "dev", Id: "$1"}},
	}

	// 第二個廣播：CONNECTED + 快照
	snap := waitBroadcast(t, ch, 2*time.Second)

	var mlabHost *tsmv1.HostState
	for _, h := range snap.Hosts {
		if h.HostId == "mlab" {
			mlabHost = h
			break
		}
	}
	require.NotNil(t, mlabHost)
	assert.Equal(t, tsmv1.HostStatus_HOST_STATUS_CONNECTED, mlabHost.Status)
	require.NotNil(t, mlabHost.Snapshot)
	require.Len(t, mlabHost.Snapshot.Sessions, 1)
	assert.Equal(t, "dev", mlabHost.Snapshot.Sessions[0].Name)
}

// TestHubManager_StartRemoteHosts_HandleDialFailure 驗證 dial 失敗後設為 DISCONNECTED。
func TestHubManager_StartRemoteHosts_HandleDialFailure(t *testing.T) {
	mhub := NewMultiHostHub()
	defer mhub.Close()
	mgr := NewHubManager(mhub)

	mgr.dialFn = func(ctx context.Context, address string) (HubRemoteClient, func(), error) {
		return nil, nil, fmt.Errorf("ssh: connection refused")
	}

	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	ch := mhub.Register()
	defer mhub.Unregister(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartRemoteHosts(ctx)
	defer mgr.Close()

	// 等待 CONNECTING 廣播
	waitBroadcast(t, ch, 2*time.Second)

	// 等待 DISCONNECTED 廣播（dial 失敗後）
	snap := waitBroadcast(t, ch, 2*time.Second)

	var mlabHost *tsmv1.HostState
	for _, h := range snap.Hosts {
		if h.HostId == "mlab" {
			mlabHost = h
			break
		}
	}
	require.NotNil(t, mlabHost)
	assert.Equal(t, tsmv1.HostStatus_HOST_STATUS_DISCONNECTED, mlabHost.Status)
	assert.Contains(t, mlabHost.Error, "connection refused")
}

// TestHubManager_StartRemoteHosts_ReconnectsAfterStreamError 驗證 watch stream 斷線後重連。
func TestHubManager_StartRemoteHosts_ReconnectsAfterStreamError(t *testing.T) {
	mhub := NewMultiHostHub()
	defer mhub.Close()
	mgr := NewHubManager(mhub)

	var dialCount atomic.Int32
	var mu sync.Mutex
	var clients []*fakeRemoteClient

	mgr.dialFn = func(ctx context.Context, address string) (HubRemoteClient, func(), error) {
		dialCount.Add(1)
		fc := newFakeRemoteClient()
		mu.Lock()
		clients = append(clients, fc)
		mu.Unlock()
		return fc, func() {}, nil
	}

	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartRemoteHosts(ctx)
	defer mgr.Close()

	// 等待第一次連線
	require.Eventually(t, func() bool {
		return dialCount.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	// 關閉第一個 client 的 snapCh 模擬 stream 斷線
	mu.Lock()
	close(clients[0].snapCh)
	mu.Unlock()

	// 應該重連（第二次 dial）
	require.Eventually(t, func() bool {
		return dialCount.Load() >= 2
	}, 5*time.Second, 50*time.Millisecond, "應在斷線後重連")
}

// TestHubManager_StartRemoteHosts_SetsMutationClient 驗證連線後可透過 ProxyMutation 操作。
func TestHubManager_StartRemoteHosts_SetsMutationClient(t *testing.T) {
	mhub := NewMultiHostHub()
	defer mhub.Close()
	mgr := NewHubManager(mhub)

	var killedSession string
	fakeClient := newFakeRemoteClient()
	fakeClient.killFn = func(ctx context.Context, name string) error {
		killedSession = name
		return nil
	}

	mgr.dialFn = func(ctx context.Context, address string) (HubRemoteClient, func(), error) {
		return fakeClient, func() {}, nil
	}

	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartRemoteHosts(ctx)
	defer mgr.Close()

	// 推送一個快照使狀態變為 CONNECTED
	fakeClient.snapCh <- &tsmv1.StateSnapshot{
		Sessions: []*tsmv1.Session{{Name: "dev"}},
	}

	// 等待 CONNECTED 狀態
	require.Eventually(t, func() bool {
		snap := mgr.Snapshot()
		for _, h := range snap.Hosts {
			if h.HostId == "mlab" && h.Status == tsmv1.HostStatus_HOST_STATUS_CONNECTED {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	// ProxyMutation 應能透過 fakeClient 操作
	resp, err := mgr.ProxyMutation(context.Background(), &tsmv1.ProxyMutationRequest{
		HostId:      "mlab",
		Type:        tsmv1.MutationType_MUTATION_KILL_SESSION,
		SessionName: "dev",
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "dev", killedSession)
}

// TestHubManager_Close_StopsRemoteGoroutines 驗證 Close 停止遠端連線 goroutine。
func TestHubManager_Close_StopsRemoteGoroutines(t *testing.T) {
	mhub := NewMultiHostHub()
	defer mhub.Close()
	mgr := NewHubManager(mhub)

	fakeClient := newFakeRemoteClient()
	mgr.dialFn = func(ctx context.Context, address string) (HubRemoteClient, func(), error) {
		return fakeClient, func() {}, nil
	}

	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartRemoteHosts(ctx)

	// 推送快照使 CONNECTED
	fakeClient.snapCh <- &tsmv1.StateSnapshot{}

	require.Eventually(t, func() bool {
		snap := mgr.Snapshot()
		for _, h := range snap.Hosts {
			if h.HostId == "mlab" && h.Status == tsmv1.HostStatus_HOST_STATUS_CONNECTED {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	mgr.Close()

	// Close 後 client 應被關閉
	assert.True(t, fakeClient.closed.Load(), "Close 後 fakeClient 應已被關閉")
}

// TestHubManager_Close_DuringDial 驗證 Close 在 dial 阻塞時不會死鎖。
func TestHubManager_Close_DuringDial(t *testing.T) {
	mhub := NewMultiHostHub()
	defer mhub.Close()
	mgr := NewHubManager(mhub)

	dialStarted := make(chan struct{})
	mgr.dialFn = func(ctx context.Context, address string) (HubRemoteClient, func(), error) {
		close(dialStarted)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}

	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartRemoteHosts(ctx)

	// 等待 dial 開始阻塞
	select {
	case <-dialStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("dial 未開始")
	}

	// Close 不應死鎖（限時 3 秒）
	done := make(chan struct{})
	go func() {
		mgr.Close()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("Close 在 dial 阻塞時死鎖")
	}
}

// --- 輔助函式 ---

func waitBroadcast(t *testing.T, ch <-chan *tsmv1.MultiHostSnapshot, timeout time.Duration) *tsmv1.MultiHostSnapshot {
	t.Helper()
	select {
	case snap := <-ch:
		return snap
	case <-time.After(timeout):
		t.Fatal("timeout waiting for broadcast")
		return nil
	}
}
