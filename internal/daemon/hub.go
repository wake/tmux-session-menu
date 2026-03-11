package daemon

import (
	"context"
	"fmt"
	"strconv"
	"sync"

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

// HubManager 管理多主機連線並聚合快照。
type HubManager struct {
	mu              sync.RWMutex
	hosts           map[string]*hubHost
	order           []string
	mhub            *MultiHostHub
	localMutationFn func(req *tsmv1.ProxyMutationRequest) error
}

// hubHost 記錄單台主機的設定、狀態與最新快照。
type hubHost struct {
	config   config.HostEntry
	status   tsmv1.HostStatus
	snapshot *tsmv1.StateSnapshot
	lastErr  string
	isLocal  bool
	client   MutationClient // 遠端主機的 gRPC client，本機為 nil
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
	m.mu.RUnlock()

	if !ok {
		return &tsmv1.ProxyMutationResponse{
			Success: false, Error: "unknown host: " + req.HostId,
		}, nil
	}

	var err error
	switch {
	case h.isLocal:
		if m.localMutationFn != nil {
			err = m.localMutationFn(req)
		} else {
			err = fmt.Errorf("local mutation handler not configured")
		}
	case h.client != nil:
		err = m.dispatchRemoteMutation(ctx, h.client, req)
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
		return c.RenameSession(ctx, req.SessionName, req.NewName, req.NewName)
	case tsmv1.MutationType_MUTATION_CREATE_SESSION:
		return c.CreateSession(ctx, req.SessionName, "", "")
	case tsmv1.MutationType_MUTATION_MOVE_SESSION:
		// GroupId 為 proto string，MoveSession 需要 int64，需解析
		gid, _ := strconv.ParseInt(req.GroupId, 10, 64)
		return c.MoveSession(ctx, req.SessionName, gid, 0)
	default:
		return fmt.Errorf("unsupported mutation type: %v", req.Type)
	}
}

// Close 關閉所有連線。
func (m *HubManager) Close() {
	// 目前只管理 state，tunnel/client 由後續 chunk 加入
}
