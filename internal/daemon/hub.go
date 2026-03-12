package daemon

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
)

// MutationClient 定義代理 mutation 需要的 client 介面。
type MutationClient interface {
	KillSession(ctx context.Context, name string) error
	RenameSession(ctx context.Context, sessionName, customName, newSessionName string) error
	CreateSession(ctx context.Context, name, path, command string) error
	MoveSession(ctx context.Context, sessionName string, groupID int64, sortOrder int) error
}

// HubRemoteClient 定義遠端主機連線需要的介面（Watch + Mutation + Close）。
// Close 必須是冪等的，可能被 runRemote 和 HubManager.Close 同時呼叫。
type HubRemoteClient interface {
	RecvSnapshot() (*tsmv1.StateSnapshot, error)
	MutationClient
	Close() error
}

// HubDialFn 建立到遠端主機的連線。回傳 client、cleanup 函式、錯誤。
type HubDialFn func(ctx context.Context, address string) (HubRemoteClient, func(), error)

// remoteBackoff 是遠端連線的退避參數。
const (
	remoteInitialBackoff = 1 * time.Second
	remoteMaxBackoff     = 30 * time.Second
)

// HubManager 管理多主機連線並聚合快照。
type HubManager struct {
	mu              sync.RWMutex
	hosts           map[string]*hubHost
	order           []string
	mhub            *MultiHostHub
	localMutationFn func(req *tsmv1.ProxyMutationRequest) error
	dialFn          HubDialFn // 可注入，預設為 defaultHubDial

	// attachLocal 清理用
	localHub *WatcherHub
	localCh  <-chan *tsmv1.StateSnapshot

	// remote 清理用
	remoteCancel context.CancelFunc
	remoteWg     sync.WaitGroup
}

// hubHost 記錄單台主機的設定、狀態與最新快照。
type hubHost struct {
	config       config.HostEntry
	status       tsmv1.HostStatus
	snapshot     *tsmv1.StateSnapshot
	lastErr      string
	isLocal      bool
	client       MutationClient  // 遠端主機的 mutation client，本機為 nil
	remoteClient HubRemoteClient // 遠端主機的完整 client（含 Close），用於清理
}

// NewHubManager 建立新的 HubManager。
func NewHubManager(mhub *MultiHostHub) *HubManager {
	return &HubManager{
		hosts: make(map[string]*hubHost),
		mhub:  mhub,
	}
}

// AddHost 加入一台主機。
func (m *HubManager) AddHost(entry config.HostEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := entry.Name
	m.hosts[id] = &hubHost{
		config:  entry,
		status:  tsmv1.HostStatus_HOST_STATUS_DISABLED,
		isLocal: entry.IsLocal(),
	}
	m.order = append(m.order, id)
}

// updateHostSnapshot 更新單台主機的快照並觸發廣播。
func (m *HubManager) updateHostSnapshot(
	hostID string, cfg config.HostEntry,
	status tsmv1.HostStatus, snap *tsmv1.StateSnapshot, errMsg string,
) {
	m.mu.Lock()
	h, ok := m.hosts[hostID]
	if !ok {
		h = &hubHost{config: cfg}
		m.hosts[hostID] = h
		m.order = append(m.order, hostID)
	}
	h.status = status
	h.snapshot = snap
	h.lastErr = errMsg
	m.mu.Unlock()

	// 觸發廣播
	m.mhub.Broadcast(m.Snapshot())
}

// Snapshot 產生目前的 MultiHostSnapshot。
func (m *HubManager) Snapshot() *tsmv1.MultiHostSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snap := &tsmv1.MultiHostSnapshot{}
	for _, id := range m.order {
		h := m.hosts[id]
		hs := &tsmv1.HostState{
			HostId:   id,
			Name:     h.config.Name,
			Color:    h.config.Color,
			Status:   h.status,
			Error:    h.lastErr,
			Snapshot: h.snapshot,
		}
		snap.Hosts = append(snap.Hosts, hs)
	}
	return snap
}

// attachLocal 將 daemon 自身的 WatcherHub 接入 hub。
func (m *HubManager) attachLocal(hub *WatcherHub, state *StateManager, cfg config.HostEntry) {
	// initial snapshot
	if snap := state.Snapshot(); snap != nil {
		m.updateHostSnapshot(cfg.Name, cfg,
			tsmv1.HostStatus_HOST_STATUS_CONNECTED, snap, "")
	}
	ch := hub.Register()
	m.localHub = hub
	m.localCh = ch
	go func() {
		for snap := range ch {
			m.updateHostSnapshot(cfg.Name, cfg,
				tsmv1.HostStatus_HOST_STATUS_CONNECTED, snap, "")
		}
	}()
}

// setHostClient 設定指定主機的 MutationClient（供測試注入）。
func (m *HubManager) setHostClient(hostID string, c MutationClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.hosts[hostID]; ok {
		h.client = c
	}
}

// ProxyMutation 將操作代理到目標主機的 daemon。
// 本機走 localMutationFn，遠端走 MutationClient。
func (m *HubManager) ProxyMutation(
	ctx context.Context, req *tsmv1.ProxyMutationRequest,
) (*tsmv1.ProxyMutationResponse, error) {
	m.mu.RLock()
	h, ok := m.hosts[req.HostId]
	var isLocal bool
	var mc MutationClient
	if ok {
		isLocal = h.isLocal
		mc = h.client
	}
	m.mu.RUnlock()

	if !ok {
		return &tsmv1.ProxyMutationResponse{
			Success: false, Error: "unknown host: " + req.HostId,
		}, nil
	}

	var err error
	switch {
	case isLocal:
		if m.localMutationFn != nil {
			err = m.localMutationFn(req)
		} else {
			err = fmt.Errorf("local mutation handler not configured")
		}
	case mc != nil:
		err = m.dispatchRemoteMutation(ctx, mc, req)
	default:
		return &tsmv1.ProxyMutationResponse{
			Success: false, Error: "host not connected: " + req.HostId,
		}, nil
	}

	if err != nil {
		return &tsmv1.ProxyMutationResponse{
			Success: false, Error: err.Error(),
		}, nil
	}
	return &tsmv1.ProxyMutationResponse{Success: true}, nil
}

// dispatchRemoteMutation 根據 MutationType 呼叫對應的 client 方法。
func (m *HubManager) dispatchRemoteMutation(
	ctx context.Context, c MutationClient, req *tsmv1.ProxyMutationRequest,
) error {
	switch req.Type {
	case tsmv1.MutationType_MUTATION_KILL_SESSION:
		return c.KillSession(ctx, req.SessionName)
	case tsmv1.MutationType_MUTATION_RENAME_SESSION:
		return c.RenameSession(ctx, req.SessionName, req.NewName, "")
	case tsmv1.MutationType_MUTATION_CREATE_SESSION:
		return c.CreateSession(ctx, req.SessionName, "", "")
	case tsmv1.MutationType_MUTATION_MOVE_SESSION:
		// GroupId 為 proto string，MoveSession 需要 int64，需解析
		gid, err := strconv.ParseInt(req.GroupId, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid group_id %q: %w", req.GroupId, err)
		}
		return c.MoveSession(ctx, req.SessionName, gid, 0)
	default:
		return fmt.Errorf("unsupported mutation type: %v", req.Type)
	}
}

// StartRemoteHosts 為每台已啟用的非 local 主機啟動連線 goroutine。
func (m *HubManager) StartRemoteHosts(ctx context.Context) {
	m.mu.Lock()
	rctx, cancel := context.WithCancel(ctx)
	m.remoteCancel = cancel
	dialFn := m.dialFn
	m.mu.Unlock()

	if dialFn == nil {
		return // 無 dialFn 則不啟動遠端連線（測試中應注入）
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, id := range m.order {
		h := m.hosts[id]
		if h.isLocal || !h.config.Enabled {
			continue
		}
		cfg := h.config
		m.remoteWg.Add(1)
		go m.runRemote(rctx, cfg, dialFn)
	}
}

// runRemote 是單台遠端主機的連線迴圈：dial → watchRemote → 斷線重連。
func (m *HubManager) runRemote(ctx context.Context, cfg config.HostEntry, dialFn HubDialFn) {
	defer m.remoteWg.Done()
	hostID := cfg.Name
	backoff := remoteInitialBackoff

	// 設為 CONNECTING
	m.updateHostSnapshot(hostID, cfg,
		tsmv1.HostStatus_HOST_STATUS_CONNECTING, nil, "")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		rc, cleanup, err := dialFn(ctx, cfg.Address)
		if err != nil {
			// dial 失敗：可能是 context 取消
			if ctx.Err() != nil {
				return
			}
			m.updateHostSnapshot(hostID, cfg,
				tsmv1.HostStatus_HOST_STATUS_DISCONNECTED, nil, err.Error())

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = remoteNextBackoff(backoff)
			continue
		}

		// 連線成功：設定 client，用 sync.Once 確保 close 冪等
		var closeOnce sync.Once
		closeRC := func() {
			closeOnce.Do(func() {
				rc.Close()
				if cleanup != nil {
					cleanup()
				}
			})
		}

		m.mu.Lock()
		if h, ok := m.hosts[hostID]; ok {
			h.client = rc
			h.remoteClient = rc
		}
		m.mu.Unlock()

		// 進入 watch 迴圈
		connectedAt := time.Now()
		m.watchRemote(ctx, cfg, rc)

		// watch 返回 = 斷線
		closeRC()

		// 清除 client
		m.mu.Lock()
		if h, ok := m.hosts[hostID]; ok {
			h.client = nil
			h.remoteClient = nil
		}
		m.mu.Unlock()

		if ctx.Err() != nil {
			return
		}

		m.updateHostSnapshot(hostID, cfg,
			tsmv1.HostStatus_HOST_STATUS_DISCONNECTED, nil, "connection lost")

		// 連線持續超過 30 秒才重置 backoff，避免快速連線-斷線循環以 1s 間隔重試
		if time.Since(connectedAt) > 30*time.Second {
			backoff = remoteInitialBackoff
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = remoteNextBackoff(backoff)
	}
}

// watchRemote 持續接收遠端快照並更新 hub。
func (m *HubManager) watchRemote(ctx context.Context, cfg config.HostEntry, rc HubRemoteClient) {
	hostID := cfg.Name
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		snap, err := rc.RecvSnapshot()
		if err != nil {
			return
		}

		m.updateHostSnapshot(hostID, cfg,
			tsmv1.HostStatus_HOST_STATUS_CONNECTED, snap, "")
	}
}

// remoteNextBackoff 回傳下一個退避間隔（倍增至上限）。
func remoteNextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > remoteMaxBackoff {
		return remoteMaxBackoff
	}
	return next
}

// Close 關閉所有連線並清理 goroutine。
func (m *HubManager) Close() {
	// 停止遠端連線 goroutine
	if m.remoteCancel != nil {
		m.remoteCancel()
	}

	// 關閉所有遠端 client（讓阻塞中的 RecvSnapshot 立即返回）
	m.mu.RLock()
	for _, h := range m.hosts {
		if !h.isLocal && h.remoteClient != nil {
			h.remoteClient.Close()
		}
	}
	m.mu.RUnlock()

	m.remoteWg.Wait()

	// 清理 local
	if m.localHub != nil && m.localCh != nil {
		m.localHub.Unregister(m.localCh)
	}
}
