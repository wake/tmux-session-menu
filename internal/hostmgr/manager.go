package hostmgr

import (
	"context"
	"fmt"
	"sync"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
)

// HostSnapshot 是面向 UI 的單一主機快照。
type HostSnapshot struct {
	HostID   string
	Name     string
	Color    string
	Status   HostStatus
	Error    string
	Snapshot *tsmv1.StateSnapshot // 未連線時為 nil
}

// HostManager 管理多主機列表與快照聚合。
type HostManager struct {
	mu         sync.RWMutex
	hosts      []*Host
	snapshotCh chan struct{} // 通知 UI 任一主機快照更新
	closeOnce  sync.Once
}

// New 建立新的 HostManager。
func New() *HostManager {
	return &HostManager{
		snapshotCh: make(chan struct{}, 1),
	}
}

// AddHost 依 HostEntry 建立 Host 並加入列表。
func (m *HostManager) AddHost(entry config.HostEntry) {
	h := NewHost(entry)
	m.mu.Lock()
	m.hosts = append(m.hosts, h)
	m.mu.Unlock()
}

// Snapshot 回傳所有主機的當前快照（依列表順序）。
func (m *HostManager) Snapshot() []HostSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snaps := make([]HostSnapshot, len(m.hosts))
	for i, h := range m.hosts {
		cfg := h.Config()
		snaps[i] = HostSnapshot{
			HostID:   h.ID(),
			Name:     cfg.Name,
			Color:    cfg.Color,
			Status:   h.Status(),
			Error:    h.LastError(),
			Snapshot: h.Snapshot(),
		}
	}
	return snaps
}

// SnapshotCh 回傳唯讀的通知通道，任一主機快照更新時會收到信號。
func (m *HostManager) SnapshotCh() <-chan struct{} {
	return m.snapshotCh
}

// notify 以非阻塞方式送出通知信號。
func (m *HostManager) notify() {
	select {
	case m.snapshotCh <- struct{}{}:
	default:
	}
}

// ClientFor 依主機 ID 回傳其 gRPC 客戶端，找不到時回傳 nil。
func (m *HostManager) ClientFor(hostID string) *client.Client {
	h := m.Host(hostID)
	if h == nil {
		return nil
	}
	return h.Client()
}

// Host 依 ID 查詢主機，找不到時回傳 nil。
func (m *HostManager) Host(hostID string) *Host {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.hosts {
		if h.ID() == hostID {
			return h
		}
	}
	return nil
}

// Hosts 回傳主機列表的副本。
func (m *HostManager) Hosts() []*Host {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make([]*Host, len(m.hosts))
	copy(cp, m.hosts)
	return cp
}

// Reorder 依 hostIDs 順序重排主機列表。
// 未列出的主機會依原順序附加到末尾。
func (m *HostManager) Reorder(hostIDs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 建立 ID → Host 索引
	index := make(map[string]*Host, len(m.hosts))
	for _, h := range m.hosts {
		index[h.ID()] = h
	}

	// 依指定順序放入結果
	seen := make(map[string]bool, len(hostIDs))
	result := make([]*Host, 0, len(m.hosts))
	for _, id := range hostIDs {
		if h, ok := index[id]; ok && !seen[id] {
			result = append(result, h)
			seen[id] = true
		}
	}

	// 附加未列出的主機（保持原順序）
	for _, h := range m.hosts {
		if !seen[h.ID()] {
			result = append(result, h)
		}
	}

	m.hosts = result
}

// Remove 依 ID 移除主機。ID 不存在時為 no-op。
func (m *HostManager) Remove(hostID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, h := range m.hosts {
		if h.ID() == hostID {
			m.hosts = append(m.hosts[:i], m.hosts[i+1:]...)
			return
		}
	}
}

// StartAll 啟動所有已啟用的主機連線。
func (m *HostManager) StartAll(ctx context.Context) {
	m.mu.RLock()
	hosts := make([]*Host, len(m.hosts))
	copy(hosts, m.hosts)
	ch := m.snapshotCh
	m.mu.RUnlock()

	for _, h := range hosts {
		if h.Config().Enabled {
			h.start(ctx, ch)
		}
	}
}

// Enable 啟用指定主機並開始連線。
func (m *HostManager) Enable(ctx context.Context, hostID string) error {
	h := m.Host(hostID)
	if h == nil {
		return fmt.Errorf("host %q not found", hostID)
	}

	// 設定 cfg.Enabled
	h.mu.Lock()
	h.cfg.Enabled = true
	h.mu.Unlock()

	// 啟動連線（不持有 m.mu）
	m.mu.RLock()
	ch := m.snapshotCh
	m.mu.RUnlock()
	h.start(ctx, ch)

	return nil
}

// Disable 停止指定主機並停用。
func (m *HostManager) Disable(hostID string) error {
	h := m.Host(hostID)
	if h == nil {
		return fmt.Errorf("host %q not found", hostID)
	}

	// 先 stop（不持有 m.mu），再更新 cfg
	h.stop()

	h.mu.Lock()
	h.cfg.Enabled = false
	h.mu.Unlock()

	m.notify()
	return nil
}

// Close 停止所有主機並關閉通知通道。可安全重複呼叫。
func (m *HostManager) Close() {
	m.mu.RLock()
	hosts := make([]*Host, len(m.hosts))
	copy(hosts, m.hosts)
	m.mu.RUnlock()

	for _, h := range hosts {
		h.stop()
	}

	m.closeOnce.Do(func() {
		close(m.snapshotCh)
	})
}
