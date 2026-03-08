package hostmgr

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wake/tmux-session-menu/internal/config"
)

// TestManagerAddAndSnapshot 驗證新增主機後快照回傳正確順序。
func TestManagerAddAndSnapshot(t *testing.T) {
	mgr := New()

	mgr.AddHost(config.HostEntry{Name: "host-a", Address: "", Color: "#aaa"})
	mgr.AddHost(config.HostEntry{Name: "host-b", Address: "10.0.0.1", Color: "#bbb"})

	snaps := mgr.Snapshot()
	require.Len(t, snaps, 2)

	assert.Equal(t, "host-a", snaps[0].HostID)
	assert.Equal(t, "host-a", snaps[0].Name)
	assert.Equal(t, "#aaa", snaps[0].Color)
	assert.Equal(t, HostDisabled, snaps[0].Status)
	assert.Empty(t, snaps[0].Error)
	assert.Nil(t, snaps[0].Snapshot)

	assert.Equal(t, "host-b", snaps[1].HostID)
	assert.Equal(t, "host-b", snaps[1].Name)
	assert.Equal(t, "#bbb", snaps[1].Color)
	assert.Equal(t, HostDisabled, snaps[1].Status)
}

// TestManagerReorder 驗證重新排序主機。
func TestManagerReorder(t *testing.T) {
	mgr := New()

	mgr.AddHost(config.HostEntry{Name: "a"})
	mgr.AddHost(config.HostEntry{Name: "b"})
	mgr.AddHost(config.HostEntry{Name: "c"})

	mgr.Reorder([]string{"c", "a", "b"})

	snaps := mgr.Snapshot()
	require.Len(t, snaps, 3)
	assert.Equal(t, "c", snaps[0].HostID)
	assert.Equal(t, "a", snaps[1].HostID)
	assert.Equal(t, "b", snaps[2].HostID)
}

// TestManagerReorderPartial 驗證部分 ID 時未列出的主機附加到末尾。
func TestManagerReorderPartial(t *testing.T) {
	mgr := New()

	mgr.AddHost(config.HostEntry{Name: "a"})
	mgr.AddHost(config.HostEntry{Name: "b"})
	mgr.AddHost(config.HostEntry{Name: "c"})

	mgr.Reorder([]string{"b"})

	snaps := mgr.Snapshot()
	require.Len(t, snaps, 3)
	assert.Equal(t, "b", snaps[0].HostID)
	// a 和 c 保持原順序附加到末尾
	assert.Equal(t, "a", snaps[1].HostID)
	assert.Equal(t, "c", snaps[2].HostID)
}

// TestManagerRemove 驗證移除中間主機。
func TestManagerRemove(t *testing.T) {
	mgr := New()

	mgr.AddHost(config.HostEntry{Name: "a"})
	mgr.AddHost(config.HostEntry{Name: "b"})
	mgr.AddHost(config.HostEntry{Name: "c"})

	mgr.Remove("b")

	snaps := mgr.Snapshot()
	require.Len(t, snaps, 2)
	assert.Equal(t, "a", snaps[0].HostID)
	assert.Equal(t, "c", snaps[1].HostID)
}

// TestManagerRemoveUnknown 驗證移除不存在的主機不會 panic。
func TestManagerRemoveUnknown(t *testing.T) {
	mgr := New()
	mgr.AddHost(config.HostEntry{Name: "a"})

	mgr.Remove("nonexistent") // 不應 panic

	snaps := mgr.Snapshot()
	require.Len(t, snaps, 1)
}

// TestManagerClientFor 驗證查詢不存在的主機回傳 nil。
func TestManagerClientFor(t *testing.T) {
	mgr := New()
	mgr.AddHost(config.HostEntry{Name: "host-a"})

	// 已知主機但尚未連線，client 應為 nil
	assert.Nil(t, mgr.ClientFor("host-a"))

	// 不存在的主機也應回傳 nil
	assert.Nil(t, mgr.ClientFor("unknown"))
}

// TestManagerHost 驗證依 ID 查詢主機。
func TestManagerHost(t *testing.T) {
	mgr := New()
	mgr.AddHost(config.HostEntry{Name: "host-x", Address: "10.0.0.2"})

	h := mgr.Host("host-x")
	require.NotNil(t, h)
	assert.Equal(t, "host-x", h.ID())

	assert.Nil(t, mgr.Host("no-such-host"))
}

// TestManagerHosts 驗證回傳主機列表的副本。
func TestManagerHosts(t *testing.T) {
	mgr := New()
	mgr.AddHost(config.HostEntry{Name: "a"})
	mgr.AddHost(config.HostEntry{Name: "b"})

	hosts := mgr.Hosts()
	require.Len(t, hosts, 2)

	// 修改副本不影響 manager 內部
	hosts[0] = nil
	assert.NotNil(t, mgr.Hosts()[0], "修改副本不應影響 manager 內部")
}

// TestManagerNotify 驗證多次 notify 不會阻塞。
func TestManagerNotify(t *testing.T) {
	mgr := New()

	// 連續呼叫多次不應阻塞
	for i := 0; i < 100; i++ {
		mgr.notify()
	}

	// 通道應有信號（至少一個）
	select {
	case <-mgr.SnapshotCh():
		// 預期收到信號
	default:
		t.Fatal("SnapshotCh 應有信號")
	}
}

// TestManagerNotifyDrain 驗證 notify 為非阻塞式。
func TestManagerNotifyDrain(t *testing.T) {
	mgr := New()

	// 先填滿通道
	mgr.notify()
	// 再呼叫一次不應阻塞
	mgr.notify()

	// 排空
	<-mgr.SnapshotCh()

	// 再次讀取應為空
	select {
	case <-mgr.SnapshotCh():
		t.Fatal("排空後不應有信號")
	default:
		// 預期
	}
}

// TestHostStatusString 驗證所有狀態的字串表示。
func TestHostStatusString(t *testing.T) {
	tests := []struct {
		status HostStatus
		want   string
	}{
		{HostDisabled, "disabled"},
		{HostConnecting, "connecting"},
		{HostConnected, "connected"},
		{HostDisconnected, "disconnected"},
		{HostStatus(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.String())
		})
	}
}

// --- Task 6: Manager 生命週期方法測試 ---

// TestManagerStartAll 驗證只啟動 Enabled 的主機。
func TestManagerStartAll(t *testing.T) {
	mgr := New()
	mgr.AddHost(config.HostEntry{Name: "enabled-host", Address: "10.0.0.1", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "disabled-host", Address: "10.0.0.2", Enabled: false})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr.StartAll(ctx)

	// 給 goroutine 啟動時間
	time.Sleep(50 * time.Millisecond)

	snaps := mgr.Snapshot()
	require.Len(t, snaps, 2)

	// enabled 的應為 Connecting（連不上遠端，但 run 會設定 Connecting）
	assert.Equal(t, HostConnecting, snaps[0].Status, "enabled 的主機應已啟動")
	// disabled 的應仍為 Disabled
	assert.Equal(t, HostDisabled, snaps[1].Status, "disabled 的主機不應啟動")

	// 清理
	mgr.Close()
}

// TestManagerEnable 驗證 Enable 啟用並啟動主機。
func TestManagerEnable(t *testing.T) {
	mgr := New()
	mgr.AddHost(config.HostEntry{Name: "host-a", Address: "10.0.0.1", Enabled: false})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Enable(ctx, "host-a")
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	h := mgr.Host("host-a")
	require.NotNil(t, h)
	assert.True(t, h.Config().Enabled, "Enable 後 cfg.Enabled 應為 true")
	assert.Equal(t, HostConnecting, h.Status(), "Enable 後應進入 Connecting")

	mgr.Close()
}

// TestManagerEnableUnknownHost 驗證啟用不存在的主機回傳錯誤。
func TestManagerEnableUnknownHost(t *testing.T) {
	mgr := New()

	err := mgr.Enable(context.Background(), "nonexistent")
	assert.Error(t, err)
}

// TestManagerDisable 驗證 Disable 停止並停用主機。
func TestManagerDisable(t *testing.T) {
	mgr := New()
	mgr.AddHost(config.HostEntry{Name: "host-a", Address: "10.0.0.1", Enabled: true})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr.StartAll(ctx)
	time.Sleep(50 * time.Millisecond)

	err := mgr.Disable("host-a")
	require.NoError(t, err)

	h := mgr.Host("host-a")
	require.NotNil(t, h)
	assert.False(t, h.Config().Enabled, "Disable 後 cfg.Enabled 應為 false")
	assert.Equal(t, HostDisabled, h.Status(), "Disable 後應為 Disabled")

	mgr.Close()
}

// TestManagerDisableUnknownHost 驗證停用不存在的主機回傳錯誤。
func TestManagerDisableUnknownHost(t *testing.T) {
	mgr := New()

	err := mgr.Disable("nonexistent")
	assert.Error(t, err)
}

// TestManagerClose 驗證 Close 停止所有主機並關閉通道。
func TestManagerClose(t *testing.T) {
	mgr := New()
	mgr.AddHost(config.HostEntry{Name: "host-a", Address: "10.0.0.1", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "host-b", Address: "10.0.0.2", Enabled: true})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr.StartAll(ctx)
	time.Sleep(50 * time.Millisecond)

	mgr.Close()

	// 所有主機應為 Disabled
	for _, h := range mgr.Hosts() {
		assert.Equal(t, HostDisabled, h.Status(), "%s 應為 Disabled", h.ID())
	}

	// snapshotCh 應已關閉
	_, open := <-mgr.SnapshotCh()
	assert.False(t, open, "snapshotCh 應已關閉")
}
