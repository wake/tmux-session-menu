package tmux

import (
	"fmt"
	"time"
)

// SessionStatus 表示 session 的狀態。
type SessionStatus int

const (
	StatusIdle    SessionStatus = iota // 閒置
	StatusRunning                      // AI 正在工作
	StatusWaiting                      // 等待人類輸入
	StatusError                        // 錯誤
)

// Session 代表一個 tmux session。
type Session struct {
	Name       string
	ID         string // tmux session id
	Path       string // 工作目錄
	Attached   bool
	Activity   time.Time // 最後活動時間
	Status     SessionStatus
	AIModel    string // 偵測到的 AI 模型（空字串表示非 AI session）
	AISummary  string // AI 摘要
	GroupName  string // 所屬群組
	SortOrder  int    // 排序順序
	CustomName string // 自訂顯示名稱（from SQLite，空字串表示使用 Name）
	AiType     string // "claude" / "" — 由 hook 生命週期決定
}

// DisplayName 回傳顯示名稱：有自訂名稱時回傳 CustomName，否則回傳 Name。
func (s Session) DisplayName() string {
	if s.CustomName != "" {
		return s.CustomName
	}
	return s.Name
}

// RelativeTime 回傳相對於現在的時間字串（例如 "30s", "5m", "3h", "2d"）。
func (s Session) RelativeTime() string {
	d := time.Since(s.Activity)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// StatusIcon 回傳對應狀態的圖示字元。
// ai_type 非空時用實心燈，空時用空心。
func (s Session) StatusIcon() string {
	if s.AiType != "" {
		if s.Status == StatusError {
			return "✗"
		}
		return "●"
	}
	return "○"
}

// Executor 定義 tmux 指令的執行介面（方便測試 mock）。
type Executor interface {
	Execute(args ...string) (string, error)
}
