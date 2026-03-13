package tmux

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// hookStatusTTL 是 hook 狀態檔案的有效期限。
const hookStatusTTL = 2 * time.Minute

// HookStatus 代表從 hook 狀態檔案讀取的狀態資訊。
type HookStatus struct {
	Status           SessionStatus `json:"-"`
	RawStatus        string        `json:"status"`
	Timestamp        int64         `json:"timestamp"`
	Event            string        `json:"event"`
	AiType           string        `json:"ai_type"`
	NotificationType string        `json:"notification_type"`
}

// IsValid 檢查 hook 狀態是否仍在有效期限內。
func (h HookStatus) IsValid() bool {
	return time.Since(time.Unix(h.Timestamp, 0)) < hookStatusTTL
}

// IsAiPresent 檢查 AI agent 是否在運行（不受 TTL 限制）。
func (h HookStatus) IsAiPresent() bool {
	return h.AiType != ""
}

// ReadHookStatus 從狀態目錄讀取指定 session 的 hook 狀態檔案。
func ReadHookStatus(statusDir, sessionName string) (HookStatus, error) {
	path := filepath.Join(statusDir, sessionName)
	data, err := os.ReadFile(path)
	if err != nil {
		return HookStatus{}, fmt.Errorf("read hook status: %w", err)
	}

	var hs HookStatus
	if err := json.Unmarshal(data, &hs); err != nil {
		return HookStatus{}, fmt.Errorf("parse hook status: %w", err)
	}

	switch hs.RawStatus {
	case "running":
		hs.Status = StatusRunning
	case "waiting":
		hs.Status = StatusWaiting
	case "error":
		hs.Status = StatusError
	default:
		hs.Status = StatusIdle
	}

	return hs, nil
}
