package daemon

import (
	"sync"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
)

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

// Close 關閉所有連線。
func (m *HubManager) Close() {
	// 目前只管理 state，tunnel/client 由後續 chunk 加入
}
