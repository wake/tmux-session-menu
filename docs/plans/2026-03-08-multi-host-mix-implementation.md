# Multi-Host Mix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 讓 tsm 同時連接多個 tmux daemon（本機 + 遠端），在單一 TUI 畫面中混合顯示

**Architecture:** 新增 HostManager 中間層管理多個 Client 連線，各 Host 獨立 Watch goroutine，UI 訂閱合併快照。Config 持久化主機清單，`h` 鍵開啟 HostPicker 面板。

**Tech Stack:** Go 1.24, Bubble Tea, gRPC, TOML config, SSH tunnel

---

### Task 1: Config 擴展 — HostEntry 結構與持久化

**Files:**
- Modify: `internal/config/config.go:11-28` (新增 HostEntry, Config.Hosts)
- Test: `internal/config/config_test.go`

**Step 1: 寫失敗測試**

```go
// internal/config/config_test.go
func TestHostEntryRoundTrip(t *testing.T) {
	cfg := Default()
	cfg.Hosts = []HostEntry{
		{Name: "local", Address: "", Color: "#5B9BD5", Enabled: true, SortOrder: 0},
		{Name: "dev", Address: "dev.example.com", Color: "#70AD47", Enabled: true, SortOrder: 1},
		{Name: "staging", Address: "staging.example.com", Color: "#FFC000", Enabled: false, SortOrder: 2},
	}
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFromString(buf.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hosts) != 3 {
		t.Fatalf("want 3 hosts, got %d", len(got.Hosts))
	}
	if got.Hosts[1].Address != "dev.example.com" {
		t.Errorf("want dev.example.com, got %s", got.Hosts[1].Address)
	}
	if got.Hosts[2].Enabled {
		t.Error("staging should be disabled")
	}
}

func TestDefaultHasLocalHost(t *testing.T) {
	cfg := Default()
	if len(cfg.Hosts) != 1 {
		t.Fatalf("want 1 default host, got %d", len(cfg.Hosts))
	}
	if cfg.Hosts[0].Name != "local" {
		t.Errorf("want local, got %s", cfg.Hosts[0].Name)
	}
	if cfg.Hosts[0].Address != "" {
		t.Errorf("local address should be empty, got %s", cfg.Hosts[0].Address)
	}
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/config/... -v -race -run TestHostEntry`
Expected: FAIL — HostEntry 未定義

**Step 3: 實作**

在 `internal/config/config.go` 新增：

```go
// HostEntry 定義一台主機的連線設定。
type HostEntry struct {
	Name      string `toml:"name"`
	Address   string `toml:"address"`     // 空 = 本機
	Color     string `toml:"color"`       // accent color（host title + status bar）
	Enabled   bool   `toml:"enabled"`
	SortOrder int    `toml:"sort_order"`
}

// IsLocal 判斷此主機是否為本機。
func (h HostEntry) IsLocal() bool {
	return h.Address == ""
}
```

在 `Config` struct 新增欄位：

```go
Hosts []HostEntry `toml:"hosts"`
```

在 `Default()` 回傳中新增：

```go
Hosts: []HostEntry{
	{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
},
```

**Step 4: 執行測試確認通過**

Run: `go test ./internal/config/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: 新增 HostEntry 結構與 Config.Hosts 持久化"
```

---

### Task 2: CLI 多 remote 參數解析

**Files:**
- Modify: `cmd/tsm/main.go:52-60` (parseRemoteHost → parseRemoteHosts)
- Test: `cmd/tsm/main_test.go` (若存在) 或新建

**Step 1: 寫失敗測試**

```go
// cmd/tsm/parse_test.go
package main

import (
	"testing"
)

func TestParseRemoteHosts(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"no remote", []string{"--inline"}, nil},
		{"single", []string{"--remote", "hostA"}, []string{"hostA"}},
		{"multiple", []string{"--remote", "hostA", "--remote", "hostB"}, []string{"hostA", "hostB"}},
		{"mixed flags", []string{"--inline", "--remote", "hostA", "--remote", "hostB"}, []string{"hostA", "hostB"}},
		{"trailing remote", []string{"--remote"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRemoteHosts(tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("want %v, got %v", tt.want, got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: want %s, got %s", i, tt.want[i], got[i])
				}
			}
		})
	}
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./cmd/tsm/... -v -race -run TestParseRemoteHosts`
Expected: FAIL — parseRemoteHosts 未定義

**Step 3: 實作**

在 `cmd/tsm/main.go` 中，將 `parseRemoteHost` 改為 `parseRemoteHosts`：

```go
// parseRemoteHosts 從命令列參數中提取所有 --remote <host> 的主機名稱。
func parseRemoteHosts(args []string) []string {
	var hosts []string
	for i, a := range args {
		if a == "--remote" && i+1 < len(args) {
			hosts = append(hosts, args[i+1])
		}
	}
	return hosts
}
```

保留舊函式作為相容包裝（若有其他呼叫者），或直接更新所有呼叫處。

同步更新 `main()` 中的判斷邏輯（第 80-89 行），改為呼叫 `parseRemoteHosts` 並支援多主機。

**Step 4: 執行測試確認通過**

Run: `go test ./cmd/tsm/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/tsm/main.go cmd/tsm/parse_test.go
git commit -m "feat: 支援多個 --remote 參數"
```

---

### Task 3: Config 與 CLI 參數整合邏輯

**Files:**
- Create: `internal/config/hosts.go`
- Test: `internal/config/hosts_test.go`

**Step 1: 寫失敗測試**

```go
// internal/config/hosts_test.go
package config

import "testing"

func TestMergeHostsNoRemoteFlag(t *testing.T) {
	// 無 --remote：local 強制 enabled，其他依 config
	hosts := []HostEntry{
		{Name: "local", Address: "", Enabled: false, SortOrder: 0},
		{Name: "dev", Address: "dev", Enabled: true, SortOrder: 1},
	}
	got := MergeHosts(hosts, nil)
	if !got[0].Enabled {
		t.Error("local should be force-enabled when no remote flags")
	}
	if !got[1].Enabled {
		t.Error("dev should keep config enabled state")
	}
}

func TestMergeHostsWithRemoteFlags(t *testing.T) {
	// --remote dev：local disabled，dev enabled
	hosts := []HostEntry{
		{Name: "local", Address: "", Enabled: true, SortOrder: 0},
		{Name: "dev", Address: "dev", Enabled: false, SortOrder: 1},
	}
	got := MergeHosts(hosts, []string{"dev"})
	if got[0].Enabled {
		t.Error("local should be disabled when --remote is used")
	}
	if !got[1].Enabled {
		t.Error("dev should be enabled by --remote flag")
	}
}

func TestMergeHostsNewRemoteHost(t *testing.T) {
	// --remote newhost：新主機自動加入
	hosts := []HostEntry{
		{Name: "local", Address: "", Enabled: true, SortOrder: 0},
	}
	got := MergeHosts(hosts, []string{"newhost"})
	if len(got) != 2 {
		t.Fatalf("want 2 hosts, got %d", len(got))
	}
	if got[1].Name != "newhost" || got[1].Address != "newhost" {
		t.Errorf("new host not added correctly: %+v", got[1])
	}
	if !got[1].Enabled {
		t.Error("new remote host should be enabled")
	}
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/config/... -v -race -run TestMergeHosts`
Expected: FAIL

**Step 3: 實作**

```go
// internal/config/hosts.go
package config

// DefaultColors 是自動分配給新主機的顏色池。
var DefaultColors = []string{
	"#5B9BD5", "#70AD47", "#FFC000", "#FF6B6B",
	"#C678DD", "#56B6C2", "#E5C07B", "#98C379",
}

// MergeHosts 將 config 中的主機清單與 --remote 參數整合。
// remoteFlags 為 nil 表示無 --remote 參數。
func MergeHosts(hosts []HostEntry, remoteFlags []string) []HostEntry {
	if remoteFlags == nil {
		// 無 --remote：local 強制 enabled，其他依 config
		result := make([]HostEntry, len(hosts))
		copy(result, hosts)
		for i := range result {
			if result[i].IsLocal() {
				result[i].Enabled = true
			}
		}
		return result
	}

	// 有 --remote：建立 flag set
	flagSet := make(map[string]bool)
	for _, f := range remoteFlags {
		flagSet[f] = true
	}

	result := make([]HostEntry, len(hosts))
	copy(result, hosts)

	// local 若不在 flag 中 → disabled
	for i := range result {
		if result[i].IsLocal() {
			result[i].Enabled = false
		}
		// flag 中的 host → enabled
		if flagSet[result[i].Address] || flagSet[result[i].Name] {
			result[i].Enabled = true
		}
	}

	// 新主機自動加入
	existing := make(map[string]bool)
	for _, h := range result {
		existing[h.Address] = true
		existing[h.Name] = true
	}
	for _, f := range remoteFlags {
		if !existing[f] {
			colorIdx := len(result) % len(DefaultColors)
			result = append(result, HostEntry{
				Name:      f,
				Address:   f,
				Color:     DefaultColors[colorIdx],
				Enabled:   true,
				SortOrder: len(result),
			})
		}
	}

	return result
}
```

**Step 4: 執行測試確認通過**

Run: `go test ./internal/config/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/hosts.go internal/config/hosts_test.go
git commit -m "feat: MergeHosts 整合 config 與 --remote 參數"
```

---

### Task 4: HostManager 核心 — Host 結構與生命週期

**Files:**
- Create: `internal/hostmgr/host.go`
- Create: `internal/hostmgr/host_test.go`

**Step 1: 寫失敗測試**

```go
// internal/hostmgr/host_test.go
package hostmgr

import (
	"testing"

	"github.com/wake/tmux-session-menu/internal/config"
)

func TestHostStatus(t *testing.T) {
	h := NewHost(config.HostEntry{
		Name: "local", Address: "", Enabled: true,
	})
	if h.Status() != HostDisabled {
		t.Error("new host should start as disabled before Start()")
	}
	if h.ID() != "local" {
		t.Errorf("want local, got %s", h.ID())
	}
	if !h.IsLocal() {
		t.Error("should be local")
	}
}

func TestHostRemote(t *testing.T) {
	h := NewHost(config.HostEntry{
		Name: "dev", Address: "dev.example.com", Enabled: true,
	})
	if h.IsLocal() {
		t.Error("should not be local")
	}
	if h.ID() != "dev" {
		t.Errorf("want dev, got %s", h.ID())
	}
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/hostmgr/... -v -race -run TestHost`
Expected: FAIL

**Step 3: 實作**

```go
// internal/hostmgr/host.go
package hostmgr

import (
	"context"
	"sync"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/remote"
)

// HostStatus 表示主機的連線狀態。
type HostStatus int

const (
	HostDisabled     HostStatus = iota // 已停用
	HostConnecting                     // 連線中
	HostConnected                      // 已連線
	HostDisconnected                   // 已斷線，重連中
)

// Host 封裝單一主機的連線與狀態。
type Host struct {
	mu       sync.RWMutex
	cfg      config.HostEntry
	status   HostStatus
	client   *client.Client
	tunnel   *remote.Tunnel
	snapshot *tsmv1.StateSnapshot
	lastErr  string
	cancel   context.CancelFunc
}

// NewHost 建立一個新的 Host。
func NewHost(entry config.HostEntry) *Host {
	return &Host{
		cfg:    entry,
		status: HostDisabled,
	}
}

func (h *Host) ID() string        { return h.cfg.Name }
func (h *Host) Config() config.HostEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

func (h *Host) IsLocal() bool     { return h.cfg.IsLocal() }

func (h *Host) Status() HostStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

func (h *Host) Client() *client.Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.client
}

func (h *Host) Snapshot() *tsmv1.StateSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.snapshot
}

func (h *Host) LastError() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastErr
}
```

**Step 4: 執行測試確認通過**

Run: `go test ./internal/hostmgr/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/hostmgr/host.go internal/hostmgr/host_test.go
git commit -m "feat: Host 結構與基本狀態管理"
```

---

### Task 5: HostManager 核心 — Manager 與快照聚合

**Files:**
- Create: `internal/hostmgr/manager.go`
- Create: `internal/hostmgr/manager_test.go`

**Step 1: 寫失敗測試**

```go
// internal/hostmgr/manager_test.go
package hostmgr

import (
	"testing"

	"github.com/wake/tmux-session-menu/internal/config"
)

func TestManagerAddAndSnapshot(t *testing.T) {
	mgr := New()
	mgr.AddHost(config.HostEntry{
		Name: "local", Address: "", Color: "#5B9BD5", Enabled: true, SortOrder: 0,
	})
	mgr.AddHost(config.HostEntry{
		Name: "dev", Address: "dev", Color: "#70AD47", Enabled: true, SortOrder: 1,
	})

	snaps := mgr.Snapshot()
	if len(snaps) != 2 {
		t.Fatalf("want 2 snapshots, got %d", len(snaps))
	}
	if snaps[0].HostID != "local" {
		t.Errorf("want local, got %s", snaps[0].HostID)
	}
	if snaps[1].HostID != "dev" {
		t.Errorf("want dev, got %s", snaps[1].HostID)
	}
}

func TestManagerReorder(t *testing.T) {
	mgr := New()
	mgr.AddHost(config.HostEntry{Name: "a", SortOrder: 0})
	mgr.AddHost(config.HostEntry{Name: "b", SortOrder: 1})
	mgr.AddHost(config.HostEntry{Name: "c", SortOrder: 2})

	mgr.Reorder([]string{"c", "a", "b"})
	snaps := mgr.Snapshot()
	if snaps[0].HostID != "c" || snaps[1].HostID != "a" || snaps[2].HostID != "b" {
		t.Errorf("reorder failed: %s, %s, %s", snaps[0].HostID, snaps[1].HostID, snaps[2].HostID)
	}
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/hostmgr/... -v -race -run TestManager`
Expected: FAIL

**Step 3: 實作**

```go
// internal/hostmgr/manager.go
package hostmgr

import (
	"sync"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
)

// HostSnapshot 是 UI 消費用的主機快照。
type HostSnapshot struct {
	HostID   string
	Name     string
	Color    string
	Status   HostStatus
	Error    string
	Snapshot *tsmv1.StateSnapshot
}

// HostManager 管理多個主機連線。
type HostManager struct {
	mu         sync.RWMutex
	hosts      []*Host
	snapshotCh chan struct{}
}

// New 建立 HostManager。
func New() *HostManager {
	return &HostManager{
		snapshotCh: make(chan struct{}, 1),
	}
}

// AddHost 新增一台主機。
func (m *HostManager) AddHost(entry config.HostEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := NewHost(entry)
	m.hosts = append(m.hosts, h)
}

// Snapshot 回傳所有 enabled host 的當前快照（按 sort order）。
func (m *HostManager) Snapshot() []HostSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var snaps []HostSnapshot
	for _, h := range m.hosts {
		cfg := h.Config()
		snaps = append(snaps, HostSnapshot{
			HostID:   h.ID(),
			Name:     cfg.Name,
			Color:    cfg.Color,
			Status:   h.Status(),
			Error:    h.LastError(),
			Snapshot: h.Snapshot(),
		})
	}
	return snaps
}

// SnapshotCh 回傳快照更新通知 channel。
func (m *HostManager) SnapshotCh() <-chan struct{} {
	return m.snapshotCh
}

// notify 非阻塞通知有快照更新。
func (m *HostManager) notify() {
	select {
	case m.snapshotCh <- struct{}{}:
	default:
	}
}

// ClientFor 回傳指定 host 的 Client。
func (m *HostManager) ClientFor(hostID string) *client.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.hosts {
		if h.ID() == hostID {
			return h.Client()
		}
	}
	return nil
}

// Host 回傳指定 ID 的 Host。
func (m *HostManager) Host(hostID string) *Host {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.hosts {
		if h.ID() == hostID {
			return h
		}
	}
	return nil
}

// Hosts 回傳所有 Host（依 sort order）。
func (m *HostManager) Hosts() []*Host {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Host, len(m.hosts))
	copy(result, m.hosts)
	return result
}

// Reorder 按指定 ID 順序重新排列 hosts。
func (m *HostManager) Reorder(hostIDs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	index := make(map[string]*Host)
	for _, h := range m.hosts {
		index[h.ID()] = h
	}

	ordered := make([]*Host, 0, len(hostIDs))
	for _, id := range hostIDs {
		if h, ok := index[id]; ok {
			ordered = append(ordered, h)
		}
	}
	m.hosts = ordered
}

// Remove 移除指定 host。
func (m *HostManager) Remove(hostID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, h := range m.hosts {
		if h.ID() == hostID {
			m.hosts = append(m.hosts[:i], m.hosts[i+1:]...)
			return
		}
	}
}
```

**Step 4: 執行測試確認通過**

Run: `go test ./internal/hostmgr/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/hostmgr/manager.go internal/hostmgr/manager_test.go
git commit -m "feat: HostManager 快照聚合與主機管理"
```

---

### Task 6: Host 連線與 Watch goroutine

**Files:**
- Modify: `internal/hostmgr/host.go` (新增 Start/Stop/run)
- Modify: `internal/hostmgr/manager.go` (新增 Enable/Disable/StartAll/Close)
- Test: `internal/hostmgr/host_test.go`

**Step 1: 寫失敗測試**

測試 Host 的 connect 邏輯需要 mock client，先測狀態流轉：

```go
// internal/hostmgr/host_test.go 新增
func TestHostStartStop(t *testing.T) {
	h := NewHost(config.HostEntry{
		Name: "test", Address: "test.example.com", Enabled: true,
	})

	notifyCh := make(chan struct{}, 10)
	ctx, cancel := context.WithCancel(context.Background())

	// Start 會將狀態改為 Connecting
	h.start(ctx, notifyCh)
	if h.Status() != HostConnecting {
		t.Errorf("want HostConnecting, got %d", h.Status())
	}

	// Stop 會取消 context
	cancel()
	h.stop()
	if h.Status() != HostDisabled {
		t.Errorf("want HostDisabled after stop, got %d", h.Status())
	}
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/hostmgr/... -v -race -run TestHostStartStop`
Expected: FAIL

**Step 3: 實作**

在 `host.go` 新增：

```go
// start 啟動連線 goroutine。
func (h *Host) start(ctx context.Context, notifyCh chan<- struct{}) {
	h.mu.Lock()
	h.status = HostConnecting
	childCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	h.mu.Unlock()

	go h.run(childCtx, notifyCh)
}

// stop 停止連線 goroutine 並清理資源。
func (h *Host) stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	if h.tunnel != nil {
		h.tunnel.Close()
		h.tunnel = nil
	}
	if h.client != nil {
		h.client.Close()
		h.client = nil
	}
	h.snapshot = nil
	h.status = HostDisabled
}

// run 是連線 + Watch 的主迴圈。
func (h *Host) run(ctx context.Context, notifyCh chan<- struct{}) {
	backoff := time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := h.connect(ctx); err != nil {
			h.mu.Lock()
			h.status = HostDisconnected
			h.lastErr = err.Error()
			h.mu.Unlock()
			notifyNonBlock(notifyCh)

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = nextBackoff(backoff)
			continue
		}

		h.mu.Lock()
		h.status = HostConnected
		h.lastErr = ""
		h.mu.Unlock()
		backoff = time.Second
		notifyNonBlock(notifyCh)

		// Watch loop
		h.watchLoop(ctx, notifyCh)
	}
}

// connect 建立到主機的連線。
func (h *Host) connect(ctx context.Context) error {
	if h.IsLocal() {
		c, err := client.Dial(h.cfg.ToConfig())
		if err != nil {
			return err
		}
		h.mu.Lock()
		h.client = c
		h.mu.Unlock()
		return h.client.Watch(ctx)
	}

	// 遠端：建立 tunnel + dial socket
	tun := remote.NewTunnel(h.cfg.Address)
	if err := tun.Start(); err != nil {
		return fmt.Errorf("SSH tunnel: %w", err)
	}
	c, err := client.DialSocket(tun.LocalSocket())
	if err != nil {
		tun.Close()
		return fmt.Errorf("dial socket: %w", err)
	}
	h.mu.Lock()
	h.tunnel = tun
	h.client = c
	h.mu.Unlock()
	return h.client.Watch(ctx)
}

// watchLoop 持續接收快照直到錯誤。
func (h *Host) watchLoop(ctx context.Context, notifyCh chan<- struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		snap, err := h.client.RecvSnapshot()
		if err != nil {
			h.mu.Lock()
			h.status = HostDisconnected
			h.lastErr = err.Error()
			h.mu.Unlock()
			notifyNonBlock(notifyCh)
			return
		}
		h.mu.Lock()
		h.snapshot = snap
		h.mu.Unlock()
		notifyNonBlock(notifyCh)
	}
}

func notifyNonBlock(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > 30*time.Second {
		return 30 * time.Second
	}
	return next
}
```

在 `manager.go` 新增：

```go
// StartAll 啟動所有 enabled host 的連線。
func (m *HostManager) StartAll(ctx context.Context) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.hosts {
		if h.Config().Enabled {
			h.start(ctx, m.snapshotCh)
		}
	}
}

// Enable 啟用指定 host 並開始連線。
func (m *HostManager) Enable(ctx context.Context, hostID string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.hosts {
		if h.ID() == hostID {
			h.mu.Lock()
			h.cfg.Enabled = true
			h.mu.Unlock()
			h.start(ctx, m.snapshotCh)
			return
		}
	}
}

// Disable 停用指定 host 並斷線。
func (m *HostManager) Disable(hostID string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.hosts {
		if h.ID() == hostID {
			h.stop()
			h.mu.Lock()
			h.cfg.Enabled = false
			h.mu.Unlock()
			m.notify()
			return
		}
	}
}

// Close 關閉所有連線。
func (m *HostManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, h := range m.hosts {
		h.stop()
	}
	close(m.snapshotCh)
}
```

**注意：** `h.cfg.ToConfig()` 需要在 config 套件新增一個 helper 回傳適合 `client.Dial` 的 `config.Config`，或者讓 local host 直接使用全域 config。這在整合時處理（Task 9）。

**Step 4: 執行測試確認通過**

Run: `go test ./internal/hostmgr/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/hostmgr/host.go internal/hostmgr/manager.go internal/hostmgr/host_test.go
git commit -m "feat: Host 連線生命週期與 Watch goroutine"
```

---

### Task 7: UI ListItem 擴展 — ItemHostTitle

**Files:**
- Modify: `internal/ui/items.go:10-65` (新增 ItemHostTitle, HostID, FlattenMultiHost)
- Test: `internal/ui/items_test.go`

**Step 1: 寫失敗測試**

```go
// internal/ui/items_test.go
package ui

import (
	"testing"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestFlattenMultiHost(t *testing.T) {
	snaps := []hostmgr.HostSnapshot{
		{
			HostID: "local", Name: "local", Color: "#5B9BD5",
			Status: hostmgr.HostConnected,
			Snapshot: &tsmv1.StateSnapshot{
				Sessions: []*tsmv1.Session{
					{Name: "dev", Id: "1"},
				},
				Groups: []*tsmv1.Group{},
			},
		},
		{
			HostID: "remote1", Name: "remote1", Color: "#70AD47",
			Status: hostmgr.HostConnected,
			Snapshot: &tsmv1.StateSnapshot{
				Sessions: []*tsmv1.Session{
					{Name: "web", Id: "2"},
					{Name: "api", Id: "3"},
				},
				Groups: []*tsmv1.Group{},
			},
		},
	}

	items := FlattenMultiHost(snaps)

	// Expected: HostTitle(local), Session(dev), HostTitle(remote1), Session(web), Session(api)
	if len(items) != 5 {
		t.Fatalf("want 5 items, got %d", len(items))
	}
	if items[0].Type != ItemHostTitle || items[0].HostID != "local" {
		t.Errorf("item 0: want HostTitle local, got %+v", items[0])
	}
	if items[1].Type != ItemSession || items[1].HostID != "local" {
		t.Errorf("item 1: want Session local, got %+v", items[1])
	}
	if items[2].Type != ItemHostTitle || items[2].HostID != "remote1" {
		t.Errorf("item 2: want HostTitle remote1, got %+v", items[2])
	}
}

func TestFlattenMultiHostDisconnected(t *testing.T) {
	snaps := []hostmgr.HostSnapshot{
		{
			HostID: "staging", Name: "staging", Color: "#FFC000",
			Status: hostmgr.HostConnecting,
			Snapshot: nil, // 尚未收到快照
		},
	}

	items := FlattenMultiHost(snaps)
	if len(items) != 1 {
		t.Fatalf("want 1 item (host title only), got %d", len(items))
	}
	if items[0].Type != ItemHostTitle {
		t.Error("disconnected host should only show title")
	}
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/ui/... -v -race -run TestFlattenMultiHost`
Expected: FAIL

**Step 3: 實作**

修改 `internal/ui/items.go`：

```go
package ui

import (
	"sort"

	"github.com/wake/tmux-session-menu/internal/hostmgr"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

type ItemType int

const (
	ItemSession   ItemType = iota
	ItemGroup
	ItemHostTitle // 主機標題列
)

// ListItem 代表列表中的一個項目。
type ListItem struct {
	Type      ItemType
	Session   tmux.Session
	Group     store.Group
	HostID    string              // 所屬主機 ID
	HostColor string              // 主機 accent color
	HostState hostmgr.HostStatus  // host title 用：主機連線狀態
	HostError string              // host title 用：錯誤訊息
}

// FlattenItems 將群組與 session 扁平化為一維列表（單主機相容）。
func FlattenItems(groups []store.Group, sessions []tmux.Session) []ListItem {
	// ... 保持原實作不變 ...
}

// FlattenMultiHost 將多主機快照扁平化為一維列表。
func FlattenMultiHost(snaps []hostmgr.HostSnapshot) []ListItem {
	var items []ListItem

	for _, hs := range snaps {
		// Host title
		items = append(items, ListItem{
			Type:      ItemHostTitle,
			HostID:    hs.HostID,
			HostColor: hs.Color,
			HostState: hs.Status,
			HostError: hs.Error,
		})

		if hs.Snapshot == nil {
			continue
		}

		sessions := ConvertProtoSessions(hs.Snapshot.Sessions)
		groups := ConvertProtoGroups(hs.Snapshot.Groups)
		hostItems := FlattenItems(groups, sessions)

		for _, item := range hostItems {
			item.HostID = hs.HostID
			item.HostColor = hs.Color
			items = append(items, item)
		}
	}

	return items
}
```

**Step 4: 執行測試確認通過**

Run: `go test ./internal/ui/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/ui/items.go internal/ui/items_test.go
git commit -m "feat: ListItem 擴展 ItemHostTitle 與 FlattenMultiHost"
```

---

### Task 8: Host Title 渲染

**Files:**
- Modify: `internal/ui/view.go` 或 `internal/ui/app.go`（View 方法中渲染 ListItem 的位置）
- Modify: `internal/ui/style.go`（若有樣式定義）

**Step 1: 確認渲染位置**

先閱讀 View() 方法中 renderItem / renderRow 的實作位置，找到渲染每一行的邏輯。

**Step 2: 新增 Host Title 渲染**

在渲染邏輯中加入 `ItemHostTitle` 的 case：

```go
case ItemHostTitle:
	titleStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(item.HostColor)).
		Foreground(lipgloss.Color("#ffffff")).
		Padding(0, 1) // 左右各一格

	title := titleStyle.Render(item.HostID)

	// 斷線時：背景改深灰，狀態文字在後方
	if item.HostState != hostmgr.HostConnected {
		titleStyle = titleStyle.Background(lipgloss.Color("#555555"))
		title = titleStyle.Render(item.HostID)
		var stateText string
		switch item.HostState {
		case hostmgr.HostConnecting:
			stateText = "連線中..."
		case hostmgr.HostDisconnected:
			stateText = "已斷線，重連中"
		case hostmgr.HostDisabled:
			stateText = "已停用"
		}
		title += " " + stateText
	}

	return title
```

**Step 3: 游標在 Host Title 時的行為**

在 `updateNormal` 中，對 `ItemHostTitle` 行禁用 session 操作（Enter、d、r 等），允許上下移動。

**Step 4: 目視驗證**

Run: `make build && bin/tsm --inline`
Expected: 畫面最上方有帶背景色的 host title

**Step 5: Commit**

```bash
git add internal/ui/
git commit -m "feat: Host title 渲染與游標行為"
```

---

### Task 9: 整合 — UI Deps 改用 HostManager

**Files:**
- Modify: `internal/ui/app.go:22-94` (Deps, Model, Init, Update)
- Modify: `internal/ui/app.go:267-315` (SnapshotMsg, watchCmd, recvSnapshotCmd)

**Step 1: 修改 Deps**

```go
type Deps struct {
	// 多主機模式
	HostMgr *hostmgr.HostManager

	// 舊模式（保留向下相容，無 HostMgr 時使用）
	Client  *client.Client
	TmuxMgr *tmux.Manager
	Store   *store.Store

	Cfg       config.Config
	StatusDir string
}
```

移除 `RemoteMode` — 多主機模式下各 host 獨立管理重連，不需要整個 TUI 退出。

**Step 2: 新增 MultiHostSnapshotMsg**

```go
type MultiHostSnapshotMsg struct {
	Snapshots []hostmgr.HostSnapshot
}

// recvMultiHostCmd 訂閱 HostManager 的快照更新。
func recvMultiHostCmd(mgr *hostmgr.HostManager) tea.Cmd {
	return func() tea.Msg {
		<-mgr.SnapshotCh()
		return MultiHostSnapshotMsg{Snapshots: mgr.Snapshot()}
	}
}
```

**Step 3: 修改 Init()**

```go
func (m Model) Init() tea.Cmd {
	if m.deps.HostMgr != nil {
		return recvMultiHostCmd(m.deps.HostMgr)
	}
	// ... 保持原有 Client / 舊模式邏輯 ...
}
```

**Step 4: 修改 Update() 的 MultiHostSnapshotMsg 處理**

```go
case MultiHostSnapshotMsg:
	m.items = FlattenMultiHost(msg.Snapshots)
	if m.cursor >= len(m.items) && len(m.items) > 0 {
		m.cursor = len(m.items) - 1
	}
	var cmds []tea.Cmd
	cmds = append(cmds, recvMultiHostCmd(m.deps.HostMgr))
	if m.hasRunning() {
		cmds = append(cmds, animTickCmd())
	}
	return m, tea.Batch(cmds...)
```

**Step 5: 修改操作轉發**

在所有 session 操作（Enter、d、r、m 等）中，根據 `m.items[m.cursor].HostID` 取得對應 Client：

```go
func (m Model) clientForCursor() *client.Client {
	if m.deps.HostMgr != nil && m.cursor >= 0 && m.cursor < len(m.items) {
		return m.deps.HostMgr.ClientFor(m.items[m.cursor].HostID)
	}
	return m.deps.Client
}
```

將現有操作中的 `m.deps.Client` 替換為 `m.clientForCursor()`。

**Step 6: 執行測試**

Run: `go test ./internal/ui/... -v -race`
Expected: PASS（既有測試不應被破壞，因為保留了 Client 模式相容）

**Step 7: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat: UI Deps 整合 HostManager，支援多主機快照"
```

---

### Task 10: ModeHostPicker 面板

**Files:**
- Modify: `internal/ui/mode.go` (新增 ModeHostPicker)
- Create: `internal/ui/hostpicker.go`
- Test: `internal/ui/hostpicker_test.go`

**Step 1: mode.go 新增常數**

```go
const (
	ModeNormal     Mode = iota
	ModeInput
	ModeConfirm
	ModePicker
	ModeSearch
	ModeHostPicker // 主機管理面板
)
```

**Step 2: 寫失敗測試**

```go
// internal/ui/hostpicker_test.go
package ui

import "testing"

func TestHostPickerNavigation(t *testing.T) {
	// 測試上下移動、space 切換
}
```

**Step 3: 實作 hostpicker.go**

```go
// internal/ui/hostpicker.go
package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wake/tmux-session-menu/internal/config"
)

// updateHostPicker 處理主機管理面板的按鍵。
func (m Model) updateHostPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	hosts := m.deps.HostMgr.Hosts()
	switch msg.String() {
	case "esc":
		// 關閉面板，寫回 config
		m.mode = ModeNormal
		return m, m.saveHostsConfig()
	case "j", "down":
		if m.pickerCursor < len(hosts)-1 {
			m.pickerCursor++
		}
	case "k", "up":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
	case " ":
		// 切換啟用/停用
		h := hosts[m.pickerCursor]
		if h.Config().Enabled {
			m.deps.HostMgr.Disable(h.ID())
		} else {
			m.deps.HostMgr.Enable(m.hostCtx, h.ID())
		}
	case "shift+down", "shift+j", "J":
		// 下移排序
		m.moveHostDown()
	case "shift+up", "shift+k", "K":
		// 上移排序
		m.moveHostUp()
	case "a":
		// 新增主機（切到 ModeInput）
		m.mode = ModeInput
		m.inputTarget = InputNewHost
		m.inputPrompt = "主機位址"
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, nil
	case "d":
		// 刪除（local 不可刪除）
		h := hosts[m.pickerCursor]
		if !h.IsLocal() {
			m.mode = ModeConfirm
			m.confirmPrompt = "刪除主機 " + h.ID() + "？"
			m.confirmAction = func() tea.Cmd {
				m.deps.HostMgr.Disable(h.ID())
				m.deps.HostMgr.Remove(h.ID())
				return nil
			}
		}
	}
	return m, nil
}

// renderHostPicker 渲染主機管理面板。
func (m Model) renderHostPicker() string {
	// 浮動面板，列出所有 host
	// ● enabled / ○ disabled
	// 游標行顯示 ▲▼
	// 底部顯示按鍵提示
}
```

**Step 4: 在 updateNormal 中加入 `h` 鍵觸發**

```go
case "h":
	if m.deps.HostMgr != nil {
		m.mode = ModeHostPicker
		m.pickerCursor = 0
		return m, nil
	}
```

**Step 5: 在 Update() 的 KeyMsg switch 中加入 ModeHostPicker**

```go
case ModeHostPicker:
	return m.updateHostPicker(msg)
```

**Step 6: 在 View() 中，ModeHostPicker 時疊加渲染面板**

**Step 7: 執行測試**

Run: `go test ./internal/ui/... -v -race`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/ui/mode.go internal/ui/hostpicker.go internal/ui/hostpicker_test.go internal/ui/app.go
git commit -m "feat: h 鍵開啟主機管理面板 (ModeHostPicker)"
```

---

### Task 11: CLI 入口整合

**Files:**
- Modify: `cmd/tsm/main.go`

**Step 1: 重構 main() 中的 --remote 處理**

```go
// main() 中：
if containsFlag(args, "--remote") {
	remoteHosts := parseRemoteHosts(args)
	if len(remoteHosts) == 0 {
		fmt.Fprintln(os.Stderr, "Error: --remote 需要指定主機名稱")
		os.Exit(1)
	}
	runMultiHost(remoteHosts)
	return
}
```

**Step 2: 新增 runMultiHost()**

```go
func runMultiHost(remoteFlags []string) {
	cfg := loadConfig()
	cfg.InTmux = os.Getenv("TMUX") != ""
	cfg.InPopup = os.Getenv("TSM_IN_POPUP") == "1"

	// 整合 hosts 清單
	merged := config.MergeHosts(cfg.Hosts, remoteFlags)
	cfg.Hosts = merged
	_ = config.SaveConfig(configPath(cfg), cfg)

	// 建立 HostManager
	mgr := hostmgr.New()
	for _, h := range merged {
		mgr.AddHost(h)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartAll(ctx)
	defer mgr.Close()

	// TUI loop
	for {
		deps := ui.Deps{HostMgr: mgr, Cfg: cfg}
		m := ui.NewModel(deps)
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fm := finalModel.(ui.Model)
		if fm.Selected() == "" {
			return
		}

		// Attach
		item := fm.SelectedItem()
		host := mgr.Host(item.HostID)
		if host.IsLocal() {
			// 本機 attach（現行邏輯）
			attachLocal(fm.Selected(), fm.ReadOnly())
		} else {
			// 遠端 attach
			remote.Attach(host.Config().Address, fm.Selected())
		}
		// 回到多主機選單繼續
	}
}
```

**Step 3: 修改 runTUI() 使用 HostManager**

將現有 `runTUI()` / `runTUIWithClient()` 也改為建立 HostManager（只含 local），統一程式碼路徑。

**Step 4: 目視驗證**

Run: `make build && bin/tsm --inline`
Run: `make build && bin/tsm --remote hostA --remote hostB`
Expected: 多主機畫面

**Step 5: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat: CLI 入口整合多主機模式"
```

---

### Task 12: Status Bar 顏色整合

**Files:**
- Modify: `internal/ui/app.go` 或 `internal/ui/view.go`（status bar 渲染位置）
- Modify: `cmd/tsm/main.go`（attach 後設定 tmux status bar 顏色）

**Step 1: 追蹤現有 status bar 色彩設定邏輯**

找到 `runRemote` / `runTUIWithClient` 中設定 tmux status bar 的程式碼。

**Step 2: 改為從 HostEntry.Color 讀取**

Attach 到 session 時，根據該 session 所屬 host 的 color 設定 tmux status bar 背景色。

**Step 3: Detach 後恢復**

回到多主機選單時恢復 status bar 為預設顏色。

**Step 4: 目視驗證**

Attach 到不同 host 的 session，確認 status bar 顏色不同。

**Step 5: Commit**

```bash
git add internal/ui/ cmd/tsm/main.go
git commit -m "feat: status bar 顏色依 host accent color 設定"
```

---

### Task 13: 端到端測試與版本推進

**Files:**
- Modify: `VERSION`
- Run: 全套測試

**Step 1: 執行全部測試**

Run: `make test`
Expected: ALL PASS

**Step 2: 執行 lint**

Run: `make lint`
Expected: 無錯誤

**Step 3: 推進版本**

新功能 → MINOR 版本推進。讀取 `VERSION` 並推進 MINOR。

**Step 4: 建置驗證**

Run: `make build && bin/tsm --version`
Expected: 顯示新版本號

**Step 5: Commit + Tag**

```bash
git add VERSION
git commit -m "feat: v0.22.0 — multi-host mix 多主機混合模式"
git tag v0.22.0
```

---

## 任務依賴關係

```
Task 1 (Config HostEntry)
  ├─→ Task 2 (CLI 多 remote 解析)
  ├─→ Task 3 (MergeHosts 整合)
  └─→ Task 4 (Host 結構)
        └─→ Task 5 (HostManager)
              └─→ Task 6 (Watch goroutine)
                    ├─→ Task 7 (ListItem 擴展)
                    │     └─→ Task 8 (Host Title 渲染)
                    ├─→ Task 9 (UI Deps 整合)
                    │     └─→ Task 10 (HostPicker 面板)
                    └─→ Task 11 (CLI 入口整合)
                          └─→ Task 12 (Status Bar 顏色)
                                └─→ Task 13 (測試 + 版本)
```

Tasks 2, 3, 4 可平行執行（都只依賴 Task 1）。
Tasks 7, 9, 11 可部分平行。
