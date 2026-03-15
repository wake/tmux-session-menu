package hostcheck

import (
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// Status 表示主機檢測狀態。
type Status int

const (
	StatusUnknown  Status = iota // 未檢測
	StatusChecking               // 檢測中
	StatusReady                  // 全部通過
	StatusFailed                 // 有項目失敗
)

// Result 記錄主機的檢測結果。
type Result struct {
	Status    Status
	SSH       *bool  // nil=local 不適用, true=通過, false=失敗
	Tmux      *bool  // true=通過, false=失敗
	Message   string // 失敗時的錯誤訊息
	CheckedAt time.Time
}

// lookPathError 用於測試中模擬 exec.LookPath 失敗（同 package 測試用）。
type lookPathError struct{ file string }

func (e *lookPathError) Error() string { return fmt.Sprintf("%s: not found", e.file) }

// ExecFn 定義可注入的指令執行函式。
type ExecFn func(name string, args ...string) error

// Checker 執行主機依賴檢測。所有外部依賴可注入，方便測試。
type Checker struct {
	LookPathFn func(file string) (string, error) // 預設 exec.LookPath
	ExecFn     ExecFn                            // 預設執行 exec.Command().Run()
}

// DefaultChecker 回傳使用真實系統呼叫的 Checker。
func DefaultChecker() *Checker {
	return &Checker{
		LookPathFn: exec.LookPath,
		ExecFn: func(name string, args ...string) error {
			return exec.Command(name, args...).Run()
		},
	}
}

func boolPtr(v bool) *bool { return &v }

// CheckLocal 檢測本機 tmux 是否已安裝。
func (c *Checker) CheckLocal() Result {
	_, err := c.LookPathFn("tmux")
	if err != nil {
		return Result{
			Status:    StatusFailed,
			Tmux:      boolPtr(false),
			Message:   "tmux 未安裝",
			CheckedAt: time.Now(),
		}
	}
	return Result{
		Status:    StatusReady,
		Tmux:      boolPtr(true),
		CheckedAt: time.Now(),
	}
}

// CheckRemote 檢測遠端主機的 SSH 連線和 tmux 安裝狀態。
func (c *Checker) CheckRemote(host string) Result {
	if err := c.ExecFn("ssh", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", host, "echo", "ok"); err != nil {
		return Result{
			Status:    StatusFailed,
			SSH:       boolPtr(false),
			Tmux:      nil,
			Message:   fmt.Sprintf("SSH 連線失敗: %v", err),
			CheckedAt: time.Now(),
		}
	}
	if err := c.ExecFn("ssh", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", host, "which", "tmux"); err != nil {
		return Result{
			Status:    StatusFailed,
			SSH:       boolPtr(true),
			Tmux:      boolPtr(false),
			Message:   "遠端主機未安裝 tmux",
			CheckedAt: time.Now(),
		}
	}
	return Result{
		Status:    StatusReady,
		SSH:       boolPtr(true),
		Tmux:      boolPtr(true),
		CheckedAt: time.Now(),
	}
}

// CheckAll 並行檢測多台主機。hosts 為 map[name]address，address 為空表示 local。
// 最多 maxConcurrent 個並行。
func (c *Checker) CheckAll(hosts map[string]string, maxConcurrent int) map[string]Result {
	if maxConcurrent <= 0 {
		maxConcurrent = len(hosts)
	}
	if maxConcurrent == 0 {
		return nil // 無主機
	}
	results := make(map[string]Result, len(hosts))
	var mu sync.Mutex
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for name, addr := range hosts {
		wg.Add(1)
		go func(name, addr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var r Result
			if addr == "" {
				r = c.CheckLocal()
			} else {
				r = c.CheckRemote(addr)
			}
			mu.Lock()
			results[name] = r
			mu.Unlock()
		}(name, addr)
	}
	wg.Wait()
	return results
}
