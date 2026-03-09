// Package hostmgr 管理多主機的連線狀態與生命週期。
package hostmgr

import (
	"context"
	"sync"
	"time"

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

// String 回傳 HostStatus 的字串表示。
func (s HostStatus) String() string {
	switch s {
	case HostDisabled:
		return "disabled"
	case HostConnecting:
		return "connecting"
	case HostConnected:
		return "connected"
	case HostDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

// maxBackoff 是指數退避的上限。
const maxBackoff = 30 * time.Second

// initialBackoff 是初始退避間隔。
const initialBackoff = 1 * time.Second

// Host 封裝單一主機的狀態與連線資源。
type Host struct {
	mu        sync.RWMutex
	cfg       config.HostEntry
	globalCfg config.Config // 本機 Dial 時需要完整 config
	status    HostStatus
	client    *client.Client
	tunnel    *remote.Tunnel
	snapshot  *tsmv1.StateSnapshot
	lastErr   string
	cancel    context.CancelFunc
	gen       uint64 // 世代計數器，每次 start() 遞增，run() 用來辨識自己是否為最新
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

// SetGlobalConfig 設定本機 Dial 所需的全域設定。
func (h *Host) SetGlobalConfig(cfg config.Config) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.globalCfg = cfg
}

// start 啟動連線 goroutine。取消舊的 goroutine 並遞增世代計數器，
// 舊 goroutine 在下次檢查世代時會自行退出，避免兩個 run() 同時修改狀態。
func (h *Host) start(ctx context.Context, notifyCh chan<- struct{}) {
	h.mu.Lock()
	if h.cancel != nil {
		h.cancel()
	}
	childCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	h.status = HostConnecting
	h.gen++
	myGen := h.gen
	h.mu.Unlock()

	go h.run(childCtx, notifyCh, myGen)
}

// stop 停止連線 goroutine，關閉 tunnel 和 client，狀態設為 Disabled。
// 遞增世代計數器使舊 goroutine 不再修改狀態。
func (h *Host) stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.gen++ // 使舊 goroutine 的世代失效
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	if h.tunnel != nil {
		h.tunnel.Close()
		h.tunnel = nil
	}
	if h.client != nil {
		_ = h.client.Close()
		h.client = nil
	}
	h.snapshot = nil
	h.status = HostDisabled
}

// run 是主連線迴圈：嘗試連線 → watchLoop → 斷線後指數退避重連。
// myGen 是啟動時的世代編號，若 h.gen 已遞增表示有更新的 goroutine 取代自己，應靜默退出。
func (h *Host) run(ctx context.Context, notifyCh chan<- struct{}, myGen uint64) {
	backoff := initialBackoff

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 檢查世代：若已被 stop/start 取代，靜默退出
		h.mu.RLock()
		stale := h.gen != myGen
		h.mu.RUnlock()
		if stale {
			return
		}

		// 嘗試連線
		if err := h.connect(ctx); err != nil {
			h.mu.Lock()
			if h.gen == myGen { // 僅當仍為最新世代時才更新狀態
				h.status = HostDisconnected
				h.lastErr = err.Error()
			}
			h.mu.Unlock()
			notifyNonBlock(notifyCh)

			// 指數退避等待
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = nextBackoff(backoff)
			continue
		}

		// 連線成功
		h.mu.Lock()
		if h.gen != myGen {
			// 已被取代，關閉剛建立的連線
			if h.client != nil {
				_ = h.client.Close()
			}
			if h.tunnel != nil {
				h.tunnel.Close()
			}
			h.mu.Unlock()
			return
		}
		h.status = HostConnected
		h.lastErr = ""
		h.mu.Unlock()
		backoff = initialBackoff
		notifyNonBlock(notifyCh)

		// 進入 watch 迴圈
		h.watchLoop(ctx, notifyCh, myGen)

		// watchLoop 返回表示斷線，回到 connect 重試
	}
}

// connect 建立 gRPC 連線與 Watch stream。
// 本機使用 client.Dial(globalCfg)，遠端透過 SSH tunnel 再 DialSocket。
func (h *Host) connect(ctx context.Context) error {
	h.mu.RLock()
	isLocal := h.cfg.IsLocal()
	address := h.cfg.Address
	globalCfg := h.globalCfg
	h.mu.RUnlock()

	var (
		c   *client.Client
		tun *remote.Tunnel
		err error
	)

	if isLocal {
		c, err = client.Dial(globalCfg)
		if err != nil {
			return err
		}
	} else {
		tun = remote.NewTunnel(address)
		if err = tun.Start(); err != nil {
			return err
		}
		c, err = client.DialSocket(tun.LocalSocket())
		if err != nil {
			tun.Close()
			return err
		}
	}

	// 啟動 Watch stream
	if err = c.Watch(ctx); err != nil {
		_ = c.Close()
		if tun != nil {
			tun.Close()
		}
		return err
	}

	// 儲存到 Host 欄位
	h.mu.Lock()
	h.client = c
	h.tunnel = tun
	h.mu.Unlock()

	return nil
}

// watchLoop 持續接收快照。收到錯誤時設為 Disconnected 並回傳，讓外層 run 重連。
// myGen 用於判斷是否已被 stop/start 取代，避免覆蓋 HostDisabled 狀態。
func (h *Host) watchLoop(ctx context.Context, notifyCh chan<- struct{}, myGen uint64) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		h.mu.RLock()
		c := h.client
		h.mu.RUnlock()

		if c == nil {
			return
		}

		snap, err := c.RecvSnapshot()
		if err != nil {
			h.mu.Lock()
			if h.gen != myGen {
				// 已被 stop/start 取代，不更新狀態（stop 已設為 HostDisabled）
				h.mu.Unlock()
				return
			}
			h.status = HostDisconnected
			h.lastErr = err.Error()
			// 清理斷線的 client 和 tunnel
			if h.client != nil {
				_ = h.client.Close()
				h.client = nil
			}
			if h.tunnel != nil {
				h.tunnel.Close()
				h.tunnel = nil
			}
			h.mu.Unlock()
			notifyNonBlock(notifyCh)
			return
		}

		h.mu.Lock()
		h.snapshot = snap
		h.mu.Unlock()
		notifyNonBlock(notifyCh)
	}
}

// notifyNonBlock 以非阻塞方式送出通知信號。
func notifyNonBlock(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// nextBackoff 回傳下一個退避間隔（倍增至上限）。
func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > maxBackoff {
		return maxBackoff
	}
	return next
}
