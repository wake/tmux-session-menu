package daemon

import (
	"sync"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

// MultiHostHub 廣播 MultiHostSnapshot 到所有已註冊的 watcher。
// 與 WatcherHub 平行，專門用於多主機聚合快照。
// 每個 watcher 持有一個 buffer=1 的 channel，廣播時使用非阻塞寫入，
// 慢消費者會被覆蓋（丟棄舊快照、寫入新快照）。
type MultiHostHub struct {
	mu       sync.RWMutex
	watchers map[<-chan *tsmv1.MultiHostSnapshot]chan *tsmv1.MultiHostSnapshot
	closed   bool
}

// NewMultiHostHub 建立新的 MultiHostHub。
func NewMultiHostHub() *MultiHostHub {
	return &MultiHostHub{
		watchers: make(map[<-chan *tsmv1.MultiHostSnapshot]chan *tsmv1.MultiHostSnapshot),
	}
}

// Register 註冊一個新的 watcher，回傳唯讀 channel。
func (h *MultiHostHub) Register() <-chan *tsmv1.MultiHostSnapshot {
	ch := make(chan *tsmv1.MultiHostSnapshot, 1)
	h.mu.Lock()
	h.watchers[ch] = ch
	h.mu.Unlock()
	return ch
}

// Unregister 移除指定的 watcher 並關閉其 channel。
func (h *MultiHostHub) Unregister(ch <-chan *tsmv1.MultiHostSnapshot) {
	h.mu.Lock()
	if wch, ok := h.watchers[ch]; ok {
		delete(h.watchers, ch)
		close(wch)
	}
	h.mu.Unlock()
}

// Broadcast 向所有已註冊的 watcher 推送多主機快照。
// 使用非阻塞寫入：若 channel 已滿則先排空再寫入，確保不會阻塞。
func (h *MultiHostHub) Broadcast(snap *tsmv1.MultiHostSnapshot) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.closed {
		return
	}

	for _, ch := range h.watchers {
		// 非阻塞：先排空舊快照，再嘗試寫入新快照。
		// 兩步都用 select 確保不會阻塞。
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- snap:
		default:
		}
	}
}

// Count 回傳目前已註冊的 watcher 數量。
func (h *MultiHostHub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.watchers)
}

// Close 關閉 hub，關閉所有 watcher channel。
func (h *MultiHostHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for key, ch := range h.watchers {
		delete(h.watchers, key)
		close(ch)
	}
}
