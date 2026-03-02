package daemon

import (
	"sync"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

// WatcherHub 管理所有 Watch streaming clients。
// 每個 watcher 持有一個 buffer=1 的 channel，廣播時使用非阻塞寫入，
// 慢消費者會被覆蓋（丟棄舊快照、寫入新快照）。
type WatcherHub struct {
	mu       sync.RWMutex
	watchers map[<-chan *tsmv1.StateSnapshot]chan *tsmv1.StateSnapshot
	closed   bool
}

// NewWatcherHub 建立新的 WatcherHub。
func NewWatcherHub() *WatcherHub {
	return &WatcherHub{
		watchers: make(map[<-chan *tsmv1.StateSnapshot]chan *tsmv1.StateSnapshot),
	}
}

// Register 註冊一個新的 watcher，回傳唯讀 channel。
func (h *WatcherHub) Register() <-chan *tsmv1.StateSnapshot {
	ch := make(chan *tsmv1.StateSnapshot, 1)
	h.mu.Lock()
	h.watchers[ch] = ch
	h.mu.Unlock()
	return ch
}

// Unregister 移除指定的 watcher 並關閉其 channel。
func (h *WatcherHub) Unregister(ch <-chan *tsmv1.StateSnapshot) {
	h.mu.Lock()
	if wch, ok := h.watchers[ch]; ok {
		delete(h.watchers, ch)
		close(wch)
	}
	h.mu.Unlock()
}

// Broadcast 向所有已註冊的 watcher 推送快照。
// 使用非阻塞寫入：若 channel 已滿則先排空再寫入，確保不會阻塞。
func (h *WatcherHub) Broadcast(snap *tsmv1.StateSnapshot) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.closed {
		return
	}

	for _, ch := range h.watchers {
		// 非阻塞：若 buffer 已滿，先排空再寫入新快照
		select {
		case <-ch:
		default:
		}
		ch <- snap
	}
}

// Count 回傳目前已註冊的 watcher 數量。
func (h *WatcherHub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.watchers)
}

// Close 關閉 hub，關閉所有 watcher channel。
func (h *WatcherHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for key, ch := range h.watchers {
		delete(h.watchers, key)
		close(ch)
	}
}
