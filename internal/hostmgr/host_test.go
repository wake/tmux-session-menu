package hostmgr

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
)

// TestNewHost 驗證新建主機的初始狀態為 HostDisabled。
func TestNewHost(t *testing.T) {
	entry := config.HostEntry{
		Name:    "test-host",
		Address: "192.168.1.1",
		Enabled: true,
	}

	h := NewHost(entry)

	require.NotNil(t, h)
	assert.Equal(t, HostDisabled, h.Status(), "新建主機應為 HostDisabled")
	assert.Nil(t, h.Client(), "新建主機的 client 應為 nil")
	assert.Nil(t, h.Snapshot(), "新建主機的 snapshot 應為 nil")
	assert.Empty(t, h.LastError(), "新建主機的 lastErr 應為空")
}

// TestHostID 驗證 ID() 回傳 cfg.Name。
func TestHostID(t *testing.T) {
	tests := []struct {
		name     string
		hostName string
	}{
		{"一般名稱", "my-server"},
		{"本機", "local"},
		{"空字串", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHost(config.HostEntry{Name: tt.hostName})
			assert.Equal(t, tt.hostName, h.ID())
		})
	}
}

// TestHostIsLocal 驗證本機與遠端的判斷邏輯。
func TestHostIsLocal(t *testing.T) {
	t.Run("本機（Address 為空）", func(t *testing.T) {
		h := NewHost(config.HostEntry{Name: "local", Address: ""})
		assert.True(t, h.IsLocal())
	})

	t.Run("遠端（Address 非空）", func(t *testing.T) {
		h := NewHost(config.HostEntry{Name: "remote", Address: "10.0.0.1"})
		assert.False(t, h.IsLocal())
	})
}

// TestHostGetters 驗證所有 getter 的執行緒安全性及正確性。
func TestHostGetters(t *testing.T) {
	entry := config.HostEntry{
		Name:      "getter-host",
		Address:   "10.0.0.5",
		Color:     "#ff0000",
		Enabled:   true,
		SortOrder: 3,
	}

	h := NewHost(entry)

	// 手動設定內部狀態以測試 getter
	h.mu.Lock()
	h.status = HostConnected
	h.lastErr = "some error"
	h.snapshot = &tsmv1.StateSnapshot{}
	h.mu.Unlock()

	// Config() 應回傳完整的 HostEntry
	assert.Equal(t, entry, h.Config())

	// Status() 應回傳手動設定的狀態
	assert.Equal(t, HostConnected, h.Status())

	// LastError() 應回傳手動設定的錯誤
	assert.Equal(t, "some error", h.LastError())

	// Snapshot() 應回傳非 nil
	assert.NotNil(t, h.Snapshot())
}

// TestHostGettersConcurrency 驗證 getter 在併發存取下不會 race。
func TestHostGettersConcurrency(t *testing.T) {
	h := NewHost(config.HostEntry{Name: "concurrent", Address: "1.2.3.4"})

	var wg sync.WaitGroup
	const goroutines = 50

	// 同時讀取所有 getter
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.ID()
			_ = h.Config()
			_ = h.IsLocal()
			_ = h.Status()
			_ = h.Client()
			_ = h.Snapshot()
			_ = h.LastError()
		}()
	}

	// 同時寫入以測試 race
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h.mu.Lock()
			h.status = HostStatus(i % 4)
			h.lastErr = "err"
			h.mu.Unlock()
		}(i)
	}

	wg.Wait()
}

// TestHostStatusConstants 驗證狀態常數的值。
func TestHostStatusConstants(t *testing.T) {
	assert.Equal(t, HostStatus(0), HostDisabled)
	assert.Equal(t, HostStatus(1), HostConnecting)
	assert.Equal(t, HostStatus(2), HostConnected)
	assert.Equal(t, HostStatus(3), HostDisconnected)
}

// --- Task 6: 連線生命週期相關測試 ---

// TestNotifyNonBlock 驗證 notifyNonBlock 不會在通道滿時阻塞。
func TestNotifyNonBlock(t *testing.T) {
	ch := make(chan struct{}, 1)
	// 填滿通道
	ch <- struct{}{}

	// 不應阻塞
	done := make(chan struct{})
	go func() {
		notifyNonBlock(ch)
		close(done)
	}()

	select {
	case <-done:
		// 成功，沒有阻塞
	case <-time.After(1 * time.Second):
		t.Fatal("notifyNonBlock 在通道滿時阻塞了")
	}

	// 空通道也不應阻塞
	ch2 := make(chan struct{}, 1)
	notifyNonBlock(ch2)
	assert.Len(t, ch2, 1)
}

// TestNextBackoff 驗證指數退避邏輯與上限。
func TestNextBackoff(t *testing.T) {
	tests := []struct {
		current  time.Duration
		expected time.Duration
	}{
		{1 * time.Second, 2 * time.Second},
		{2 * time.Second, 4 * time.Second},
		{4 * time.Second, 8 * time.Second},
		{8 * time.Second, 16 * time.Second},
		{16 * time.Second, 30 * time.Second}, // 超過上限，回傳上限
		{30 * time.Second, 30 * time.Second}, // 已在上限
		{60 * time.Second, 30 * time.Second}, // 超過上限
	}

	for _, tt := range tests {
		got := nextBackoff(tt.current)
		assert.Equal(t, tt.expected, got, "nextBackoff(%v)", tt.current)
	}
}

// TestHostStartSetsConnecting 驗證 start() 同步設定狀態為 Connecting。
// 使用已取消的 context 讓 run() goroutine 立即退出，以確保狀態維持在 Connecting。
func TestHostStartSetsConnecting(t *testing.T) {
	h := NewHost(config.HostEntry{
		Name:    "test-host",
		Address: "remote.example.com",
		Enabled: true,
	})
	require.Equal(t, HostDisabled, h.Status())

	// 使用已取消的 parent context，run() 進入後會在 ctx.Done() 立即退出
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	ch := make(chan struct{}, 1)
	h.start(ctx, ch)

	// start() 同步設定 Connecting；run() goroutine 偵測到 ctx.Done() 後立即退出，不會改狀態
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, HostConnecting, h.Status())
}

// TestHostStopSetsDisabled 驗證 stop() 會將狀態設為 Disabled 並清理資源。
func TestHostStopSetsDisabled(t *testing.T) {
	h := NewHost(config.HostEntry{
		Name:    "test-host",
		Address: "remote.example.com",
		Enabled: true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan struct{}, 1)
	h.start(ctx, ch)
	time.Sleep(50 * time.Millisecond)

	h.stop()

	assert.Equal(t, HostDisabled, h.Status())
	assert.Nil(t, h.Client())
	assert.Nil(t, h.Snapshot())
}

// TestHostStopIdempotent 驗證對未啟動的 host 呼叫 stop 不會 panic。
func TestHostStopIdempotent(t *testing.T) {
	h := NewHost(config.HostEntry{
		Name:    "test-host",
		Address: "remote.example.com",
		Enabled: true,
	})

	assert.NotPanics(t, func() {
		h.stop()
	})
	assert.Equal(t, HostDisabled, h.Status())
}
