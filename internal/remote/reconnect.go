package remote

import "time"

// ReconnState 表示重連狀態。
type ReconnState int

const (
	// StateDisconnected 表示已斷線。
	StateDisconnected ReconnState = iota
	// StateConnecting 表示正在嘗試連線。
	StateConnecting
	// StateConnected 表示已連線。
	StateConnected
)

// String 回傳狀態名稱。
func (s ReconnState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	default:
		return "unknown"
	}
}

// MaxBackoff 是重連延遲的上限。
const MaxBackoff = 30 * time.Second

// backoff 根據嘗試次數（0 起算）回傳延遲時間。
// 使用指數退避：1s, 2s, 4s, 8s, 16s，不超過 max。
func backoff(attempt int, max time.Duration) time.Duration {
	if attempt >= 63 {
		return max
	}
	d := time.Second << uint(attempt)
	if d <= 0 || d > max {
		return max
	}
	return d
}
