// Package hostmgr 管理多主機的連線狀態與生命週期。
package hostmgr

import (
	"context"
	"sync"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/remote"
)

// HostStatus 代表主機的連線狀態。
type HostStatus int

const (
	HostDisabled     HostStatus = iota // 已停用
	HostConnecting                     // 連線中
	HostConnected                      // 已連線
	HostDisconnected                   // 已斷線，重連中
)

// Host 封裝單一主機的狀態與連線資源。
type Host struct {
	mu       sync.RWMutex
	cfg      config.HostEntry
	status   HostStatus
	client   *client.Client
	tunnel   *remote.Tunnel
	snapshot *tsmv1.StateSnapshot
	lastErr  string
	cancel   context.CancelFunc
}

// NewHost 建立新的 Host 實例，初始狀態為 HostDisabled。
func NewHost(entry config.HostEntry) *Host {
	return &Host{
		cfg:    entry,
		status: HostDisabled,
	}
}

// ID 回傳主機識別名稱。
func (h *Host) ID() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg.Name
}

// Config 回傳主機設定。
func (h *Host) Config() config.HostEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

// IsLocal 回傳此主機是否為本機。
func (h *Host) IsLocal() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg.IsLocal()
}

// Status 回傳目前連線狀態。
func (h *Host) Status() HostStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

// Client 回傳 gRPC 客戶端（可能為 nil）。
func (h *Host) Client() *client.Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.client
}

// Snapshot 回傳最新的狀態快照（可能為 nil）。
func (h *Host) Snapshot() *tsmv1.StateSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.snapshot
}

// LastError 回傳最後一次錯誤訊息。
func (h *Host) LastError() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastErr
}
