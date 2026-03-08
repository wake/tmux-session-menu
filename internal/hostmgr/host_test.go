package hostmgr

import (
	"sync"
	"testing"

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
