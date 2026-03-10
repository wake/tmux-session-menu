package daemon

import (
	"sync"
	"time"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

// UploadState 追蹤上傳模式狀態和上傳事件。
type UploadState struct {
	mu          sync.RWMutex
	enabled     bool
	sessionName string
	events      []*tsmv1.UploadEvent
}

// NewUploadState 建立新的 UploadState。
func NewUploadState() *UploadState {
	return &UploadState{}
}

// SetMode 啟用或停用上傳模式。停用時會清除所有事件。
func (us *UploadState) SetMode(enabled bool, sessionName string) {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.enabled = enabled
	us.sessionName = sessionName
	if !enabled {
		us.events = nil
	}
}

// IsUploadMode 回傳是否處於上傳模式。
func (us *UploadState) IsUploadMode() bool {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return us.enabled
}

// SessionName 回傳目前上傳模式對應的 session 名稱。
func (us *UploadState) SessionName() string {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return us.sessionName
}

// AddEvent 新增一筆上傳事件。若 timestamp 為零值則自動填入當前時間。
func (us *UploadState) AddEvent(event *tsmv1.UploadEvent) {
	us.mu.Lock()
	defer us.mu.Unlock()
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}
	us.events = append(us.events, event)
}

// DrainEvents 回傳所有已累積的上傳事件並清空佇列。
func (us *UploadState) DrainEvents() []*tsmv1.UploadEvent {
	us.mu.Lock()
	defer us.mu.Unlock()
	events := us.events
	us.events = nil
	return events
}
