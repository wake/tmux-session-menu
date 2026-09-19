# Daemon Hub 模式 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 將多主機聚合從 TUI 移入 daemon（hub 模式），搭配 SSH reverse tunnel，讓任何遠端主機的 Ctrl+Q 都能連回 hub daemon 取得完整的多主機快照。

**Architecture:** Hub daemon 管理所有 SSH tunnel（forward 取得遠端 sessions、reverse 讓遠端 TUI 連回）+ 聚合所有主機的 StateSnapshot 為 MultiHostSnapshot。遠端 TUI 透過 reverse tunnel socket 連入 hub 的 WatchMultiHost RPC，取得與 air 端一模一樣的多主機畫面。

**Tech Stack:** Go 1.24+, gRPC + protobuf, SSH tunnel (`-L` + `-R`), Bubble Tea TUI, testify

---

## 架構總覽

```
Hub Daemon (air)
  ├─ StateManager (原有，管理 air 本地 tmux)
  ├─ HubManager (新增)
  │    ├─ SSH -L → m1 daemon (forward tunnel，取得 m1 sessions)
  │    │  SSH -R → m1 reverse socket → hub daemon
  │    ├─ SSH -L → m2 daemon
  │    │  SSH -R → m2 reverse socket → hub daemon
  │    └─ SSH -L → m3 daemon
  │       SSH -R → m3 reverse socket → hub daemon
  │
  ├─ WatcherHub (原有，單機 Watch)
  ├─ MultiHostHub (新增，聚合多機 Watch)
  │
  └─ gRPC Service
       ├─ Watch()          ← 原有，單機快照
       └─ WatchMultiHost() ← 新增，多主機聚合快照

m1/m2/m3 TUI (Ctrl+Q)
  → 偵測 @tsm_hub_socket tmux option
  → 連線到 reverse tunnel socket
  → WatchMultiHost() stream → 渲染完整多主機畫面
```

## 檔案結構

### 新增檔案

| 檔案 | 職責 |
|------|------|
| `internal/daemon/hub.go` | HubManager：管理遠端 tunnel + 聚合快照 + mutation proxy |
| `internal/daemon/hub_test.go` | HubManager 單元測試 |
| `internal/daemon/multihost_hub.go` | MultiHostHub：多主機 Watch 廣播 |
| `internal/daemon/multihost_hub_test.go` | MultiHostHub 測試 |

### 修改檔案

| 檔案 | 變更 |
|------|------|
| `api/proto/tsm/v1/tsm.proto` | 新增 HostStatus enum、HostState、MultiHostSnapshot、ProxyMutationRequest/Response + WatchMultiHost/ProxyMutation RPC |
| `internal/remote/tunnel.go` | 新增 `-R` reverse tunnel 支援 |
| `internal/remote/tunnel_test.go` | reverse tunnel 測試 |
| `internal/daemon/daemon.go` | Hub 模式啟動邏輯 |
| `internal/daemon/service.go` | WatchMultiHost RPC 實作 |
| `internal/client/client.go` | WatchMultiHost client 方法 |
| `internal/ui/app.go` | Hub 模式接收 MultiHostSnapshot |
| `internal/ui/convert.go` | Proto → HostSnapshotInput 轉換 |
| `internal/remote/attach.go` | attach 前設定 @tsm_hub_socket |
| `internal/bind/bind.go` | bind block 加入 hub socket 條件判斷 |
| `cmd/tsm/main.go` | --hub-socket flag、daemon --hub 啟動 |

---

## Chunk 1: Proto — MultiHostSnapshot 定義

### Task 1: 新增 proto messages 與 RPC

**Files:**
- Modify: `api/proto/tsm/v1/tsm.proto`

- [ ] **Step 1: 新增 HostState 與 MultiHostSnapshot messages**

在 `tsm.proto` 的 `StateSnapshot` 之後新增：

```protobuf
// HostStatus 描述主機連線狀態。
enum HostStatus {
  HOST_STATUS_DISABLED     = 0;
  HOST_STATUS_CONNECTING   = 1;
  HOST_STATUS_CONNECTED    = 2;
  HOST_STATUS_DISCONNECTED = 3;
}

// HostState 描述一台主機的連線狀態與快照。
message HostState {
  string host_id = 1;
  string name = 2;
  string color = 3;
  HostStatus status = 4;
  string error = 5;
  StateSnapshot snapshot = 6;  // nil if not connected
}

// MultiHostSnapshot 聚合所有主機的狀態快照。
message MultiHostSnapshot {
  repeated HostState hosts = 1;
}
```

- [ ] **Step 2: 新增 ProxyMutation messages**

mutation routing 用的 proto 定義（遠端 TUI 透過 hub 代理操作到目標主機）：

```protobuf
// MutationType 列舉所有可被代理的操作類型。
enum MutationType {
  MUTATION_KILL_SESSION   = 0;
  MUTATION_RENAME_SESSION = 1;
  MUTATION_CREATE_SESSION = 2;
  MUTATION_MOVE_SESSION   = 3;
}

// ProxyMutationRequest 由遠端 TUI 發送，透過 hub 代理到目標主機。
message ProxyMutationRequest {
  string host_id = 1;       // 目標主機 ID
  MutationType type = 2;
  string session_name = 3;  // 目標 session 名稱
  string new_name = 4;      // rename 用
  string group_id = 5;      // move 用
}

message ProxyMutationResponse {
  bool success = 1;
  string error = 2;
}
```

- [ ] **Step 3: 新增 WatchMultiHost + ProxyMutation RPC**

在 `service SessionManager` 中新增：

```protobuf
// WatchMultiHost 回傳多主機聚合快照 stream（hub 模式專用）。
rpc WatchMultiHost(WatchMultiHostRequest) returns (stream MultiHostSnapshot);

// ProxyMutation 將操作代理到目標主機的 daemon（hub 模式專用）。
rpc ProxyMutation(ProxyMutationRequest) returns (ProxyMutationResponse);
```

以及 request message：

```protobuf
message WatchMultiHostRequest {}
```

- [ ] **Step 4: 重新產生 Go 程式碼**

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       api/proto/tsm/v1/tsm.proto
```

- [ ] **Step 5: 編譯驗證**

```bash
go build ./...
```
Expected: 編譯通過（新 RPC 回傳 `UnimplementedSessionManagerServer` 預設值）

- [ ] **Step 6: Commit**

```bash
git add api/
git commit -m "proto: 新增 HostStatus enum、MultiHostSnapshot、ProxyMutation 與 WatchMultiHost RPC"
```

---

## Chunk 2: Reverse Tunnel 支援

### Task 2: Tunnel 新增 `-R` 參數

**Files:**
- Modify: `internal/remote/tunnel.go`
- Modify: `internal/remote/tunnel_test.go`

- [ ] **Step 1: 寫失敗測試 — reverse tunnel 參數建構**

在 `tunnel_test.go` 新增：

```go
func TestTunnelArgs_WithReverse(t *testing.T) {
	args := tunnelArgs("mlab", "/tmp/tsm-fwd.sock", "/home/user/.config/tsm/tsm.sock")
	// 預設只有 -L，沒有 -R
	assert.NotContains(t, args, "-R")

	args = tunnelArgsWithReverse(
		"mlab",
		"/tmp/tsm-fwd.sock",
		"/home/user/.config/tsm/tsm.sock",
		"/tmp/tsm-reverse-abc.sock",
		"/home/air/.config/tsm/tsm.sock",
	)
	// 應包含 -L 和 -R
	assert.Contains(t, args, "-L")
	assert.Contains(t, args, "-R")

	// 找到 -R 的值
	for i, a := range args {
		if a == "-R" {
			assert.Equal(t, "/tmp/tsm-reverse-abc.sock:/home/air/.config/tsm/tsm.sock", args[i+1])
			break
		}
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/remote/... -v -race -run TestTunnelArgs_WithReverse
```
Expected: FAIL — `tunnelArgsWithReverse` undefined

- [ ] **Step 3: 實作 tunnelArgsWithReverse**

在 `tunnel.go` 新增：

```go
// tunnelArgsWithReverse 建構含 forward (-L) 和 reverse (-R) 的 SSH tunnel 參數。
func tunnelArgsWithReverse(host, localSock, remoteSock, reverseRemoteSock, reverseLocalSock string) []string {
	args := []string{
		"ssh", "-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=5",
		"-o", "ServerAliveCountMax=3",
		"-L", localSock + ":" + remoteSock,
		"-R", reverseRemoteSock + ":" + reverseLocalSock,
		host,
	}
	return args
}
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/remote/... -v -race -run TestTunnelArgs_WithReverse
```
Expected: PASS

### Task 3: Tunnel struct 支援 reverse tunnel 選項

**Files:**
- Modify: `internal/remote/tunnel.go`
- Modify: `internal/remote/tunnel_test.go`

- [ ] **Step 1: 寫失敗測試 — ReverseSocketPath**

```go
func TestReverseSocketPath(t *testing.T) {
	path := ReverseSocketPath("mlab")
	assert.Contains(t, path, "tsm-hub-")
	assert.True(t, strings.HasPrefix(path, os.TempDir()))
	// 同一 host 應產生相同路徑
	assert.Equal(t, path, ReverseSocketPath("mlab"))
	// 不同 host 應產生不同路徑
	assert.NotEqual(t, path, ReverseSocketPath("air"))
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/remote/... -v -race -run TestReverseSocketPath
```
Expected: FAIL — `ReverseSocketPath` undefined

- [ ] **Step 3: 實作 ReverseSocketPath**

```go
// ReverseSocketPath 為指定 host 產生確定性的 reverse tunnel socket 路徑。
// 格式：/tmp/tsm-hub-{hash}.sock（hash 基於 host 名稱）。
func ReverseSocketPath(host string) string {
	h := sha256.Sum256([]byte("hub:" + host))
	return filepath.Join(os.TempDir(), fmt.Sprintf("tsm-hub-%x.sock", h[:8]))
}
```

- [ ] **Step 4: 執行測試確認通過**

- [ ] **Step 5: 寫失敗測試 — Tunnel 支援 WithReverse option**

```go
func TestNewTunnel_WithReverse(t *testing.T) {
	tun := NewTunnel("mlab", WithReverse("/home/air/.config/tsm/tsm.sock"))
	assert.True(t, tun.hasReverse)
	assert.Equal(t, "/home/air/.config/tsm/tsm.sock", tun.reverseLocalSock)
	assert.Equal(t, ReverseSocketPath("mlab"), tun.reverseRemoteSock)
}
```

- [ ] **Step 6: 執行測試確認失敗**

- [ ] **Step 7: 實作 WithReverse TunnelOption**

在 `Tunnel` struct 新增欄位：

```go
type Tunnel struct {
	host              string
	localSock         string
	hasReverse        bool   // 是否建立 reverse tunnel
	reverseLocalSock  string // 本地端（hub daemon socket）
	reverseRemoteSock string // 遠端上的 reverse socket
	proc              Process
	cmdRun            CmdRunFunc
	cmdStart          CmdStartFunc
}

// WithReverse 設定 reverse tunnel，讓遠端能透過 socket 連回 hub daemon。
func WithReverse(hubSocketPath string) TunnelOption {
	return func(t *Tunnel) {
		t.hasReverse = true
		t.reverseLocalSock = hubSocketPath
		t.reverseRemoteSock = ReverseSocketPath(t.host)
	}
}
```

修改 `Start()` 內的 SSH 命令建構，若 `hasReverse` 則使用 `tunnelArgsWithReverse()`。

- [ ] **Step 8: 執行全部 tunnel 測試確認通過**

```bash
go test ./internal/remote/... -v -race
```

- [ ] **Step 9: Commit**

```bash
git add internal/remote/
git commit -m "feat: SSH tunnel 支援 -R reverse tunnel"
```

---

## Chunk 3: MultiHostHub — 多主機廣播器

### Task 4: MultiHostHub 結構

**Files:**
- Create: `internal/daemon/multihost_hub.go`
- Create: `internal/daemon/multihost_hub_test.go`

- [ ] **Step 1: 寫失敗測試 — Register/Broadcast/Unregister**

```go
package daemon

func TestMultiHostHub_RegisterAndBroadcast(t *testing.T) {
	hub := NewMultiHostHub()
	defer hub.Close()

	ch := hub.Register()
	snap := &tsmv1.MultiHostSnapshot{
		Hosts: []*tsmv1.HostState{
			{HostId: "local", Name: "air", Status: 2},
		},
	}

	hub.Broadcast(snap)

	select {
	case got := <-ch:
		assert.Equal(t, "local", got.Hosts[0].HostId)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestMultiHostHub_Unregister(t *testing.T) {
	hub := NewMultiHostHub()
	defer hub.Close()

	ch := hub.Register()
	hub.Unregister(ch)

	// channel 應已關閉
	_, ok := <-ch
	assert.False(t, ok)
}
```

- [ ] **Step 2: 執行測試確認失敗**

- [ ] **Step 3: 實作 MultiHostHub**

```go
package daemon

// MultiHostHub 廣播 MultiHostSnapshot 到所有已註冊的 watcher。
// 與 WatcherHub 平行，專門用於多主機聚合快照。
type MultiHostHub struct {
	mu       sync.RWMutex
	watchers map[<-chan *tsmv1.MultiHostSnapshot]chan *tsmv1.MultiHostSnapshot
	closed   bool
}

func NewMultiHostHub() *MultiHostHub { ... }
func (h *MultiHostHub) Register() <-chan *tsmv1.MultiHostSnapshot { ... }
func (h *MultiHostHub) Unregister(ch <-chan *tsmv1.MultiHostSnapshot) { ... }
func (h *MultiHostHub) Broadcast(snap *tsmv1.MultiHostSnapshot) { ... }
func (h *MultiHostHub) Close() { ... }
```

實作模式與 `WatcherHub` 完全一致（非阻塞 drain + send）。

- [ ] **Step 4: 執行測試確認通過**

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/multihost_hub.go internal/daemon/multihost_hub_test.go
git commit -m "feat: 新增 MultiHostHub 多主機快照廣播器"
```

---

## Chunk 4: HubManager — 聚合引擎

### Task 5: HubManager 結構與聚合邏輯

**Files:**
- Create: `internal/daemon/hub.go`
- Create: `internal/daemon/hub_test.go`

- [ ] **Step 1: 寫失敗測試 — 聚合快照**

```go
func TestHubManager_BuildMultiHostSnapshot(t *testing.T) {
	mgr := NewHubManager(NewMultiHostHub())

	// 模擬兩台主機的快照
	localSnap := &tsmv1.StateSnapshot{
		Sessions: []*tsmv1.Session{{Name: "main", Id: "$0"}},
	}
	remoteSnap := &tsmv1.StateSnapshot{
		Sessions: []*tsmv1.Session{{Name: "dev", Id: "$1"}},
	}

	mgr.updateHostSnapshot("local", config.HostEntry{
		Name: "air", Color: "#5f8787",
	}, tsmv1.HostStatus_HOST_STATUS_CONNECTED, localSnap, "")

	mgr.updateHostSnapshot("mlab", config.HostEntry{
		Name: "mlab", Color: "#73daca",
	}, tsmv1.HostStatus_HOST_STATUS_CONNECTED, remoteSnap, "")

	snap := mgr.Snapshot()

	require.Len(t, snap.Hosts, 2)
	assert.Equal(t, "local", snap.Hosts[0].HostId)
	assert.Equal(t, "mlab", snap.Hosts[1].HostId)
	assert.Len(t, snap.Hosts[0].Snapshot.Sessions, 1)
	assert.Len(t, snap.Hosts[1].Snapshot.Sessions, 1)
}
```

- [ ] **Step 2: 執行測試確認失敗**

- [ ] **Step 3: 實作 HubManager 核心**

```go
package daemon

// HubManager 管理多主機連線並聚合快照。
// 在 hub daemon 中取代 TUI 的 hostmgr.HostManager。
type HubManager struct {
	mu              sync.RWMutex
	hosts           map[string]*hubHost // key: hostID
	order           []string            // host 順序
	mhub            *MultiHostHub       // 多主機廣播器
	localMutationFn func(req *tsmv1.ProxyMutationRequest) error // local host mutation 處理
	tunnelFactory   TunnelFactory   // 測試注入
	clientFactory   ClientFactory   // 測試注入
}

type hubHost struct {
	config   config.HostEntry
	status   tsmv1.HostStatus
	snapshot *tsmv1.StateSnapshot
	lastErr  string
	isLocal  bool              // true = hub daemon 自身
	tunnel   *remote.Tunnel    // nil for local
	client   MutationClient    // 遠端用 *client.Client，local 為 nil
	cancel   context.CancelFunc
}

// 注入型別：允許測試替換 tunnel/client 建立邏輯
type TunnelFactory func(host string, opts ...remote.TunnelOption) *remote.Tunnel
type ClientFactory func(socketPath string) (*client.Client, error)

func NewHubManager(mhub *MultiHostHub, opts ...HubOption) *HubManager { ... }

// HubOption 設定 HubManager 的可選參數。
type HubOption func(*HubManager)

// WithTunnelFactory 注入自訂的 tunnel 建立函式（測試用）。
func WithTunnelFactory(fn TunnelFactory) HubOption { ... }

// WithClientFactory 注入自訂的 client 建立函式（測試用）。
func WithClientFactory(fn ClientFactory) HubOption { ... }

// updateHostSnapshot 更新單台主機的快照並觸發廣播。
func (m *HubManager) updateHostSnapshot(
	hostID string, cfg config.HostEntry,
	status tsmv1.HostStatus, snap *tsmv1.StateSnapshot, errMsg string,
) { ... }

// Snapshot 產生目前的 MultiHostSnapshot（供 Service 推送 initial snapshot）。
func (m *HubManager) Snapshot() *tsmv1.MultiHostSnapshot { ... }

// AddHost 加入一台主機。
func (m *HubManager) AddHost(entry config.HostEntry) { ... }

// StartAll 啟動所有已啟用主機的連線（forward + reverse tunnel）。
func (m *HubManager) StartAll(ctx context.Context, hubSocketPath string) { ... }

// Close 關閉所有連線。
func (m *HubManager) Close() { ... }
```

- [ ] **Step 4: 執行測試確認通過**

### Task 6: HubManager 遠端連線與 watch loop

**Files:**
- Modify: `internal/daemon/hub.go`
- Modify: `internal/daemon/hub_test.go`

- [ ] **Step 1: 寫失敗測試 — 遠端連線建立**

測試 `connectRemote()` 使用注入的 tunnel/client factory。

```go
func TestHubManager_ConnectRemote(t *testing.T) {
	// 使用 mock tunnel + client 驗證連線流程
	// ...詳見實作時的具體 mock 設計
}
```

- [ ] **Step 2: 實作遠端連線**

每台遠端主機的 goroutine 流程：
1. 建立 SSH tunnel：`remote.NewTunnel(host, remote.WithReverse(hubSocketPath))`
2. `tunnel.Start()` — 同時建立 forward 和 reverse tunnel
3. `client.DialSocket(tunnel.LocalSocket())` — 透過 forward tunnel 連線
4. `client.Watch(ctx)` — 訂閱遠端 daemon 的快照 stream
5. Watch loop：收到 snapshot → `updateHostSnapshot()` → `MultiHostHub.Broadcast()`
6. 斷線重連：指數退避（1s → 2s → 4s → max 30s）

> **重用注意**：`hostmgr.Host` 已有完整的 generation-based staleness detection + 指數退避重連邏輯。
> 實作時應考慮抽取 `hostmgr.Host` 的重連核心為共用 helper（如 `internal/reconnect/backoff.go`），
> 或直接在 HubManager 中複製同樣的退避參數（initialDelay=1s, maxDelay=30s, factor=2）。
> HubManager 還需要保存每台遠端主機的 `*client.Client` 引用，供 ProxyMutation 使用。

- [ ] **Step 3: 實作 local host 連線**

Local host 直接使用 daemon 自身的 WatcherHub。

注意：必須先推送 initial snapshot（與 Watch() 行為一致），否則在第一次 tmux 狀態變化前客戶端看不到本機 sessions。

```go
func (m *HubManager) attachLocal(hub *WatcherHub, state *StateManager, cfg config.HostEntry) {
	// initial snapshot — 不等待第一次 Watch broadcast
	if snap := state.Snapshot(); snap != nil {
		m.updateHostSnapshot(cfg.Name, cfg,
			tsmv1.HostStatus_HOST_STATUS_CONNECTED, snap, "")
	}
	ch := hub.Register()
	go func() {
		for snap := range ch {
			m.updateHostSnapshot(cfg.Name, cfg,
				tsmv1.HostStatus_HOST_STATUS_CONNECTED, snap, "")
		}
	}()
}
```

- [ ] **Step 4: 執行全部測試確認通過**

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/hub.go internal/daemon/hub_test.go
git commit -m "feat: HubManager 多主機連線管理與快照聚合"
```

---

## Chunk 5: WatchMultiHost Service 實作

### Task 7: gRPC WatchMultiHost 端點

**Files:**
- Modify: `internal/daemon/service.go`

- [ ] **Step 1: 在 Service struct 加入 MultiHostHub**

```go
type Service struct {
	tsmv1.UnimplementedSessionManagerServer
	tmuxMgr  *tmux.Manager
	store    *store.Store
	hub      *WatcherHub
	mhub     *MultiHostHub  // nil = non-hub mode
	hubMgr   *HubManager    // nil = non-hub mode（用於 initial snapshot + proxy mutation）
	state    *StateManager
	startedAt time.Time
}
```

- [ ] **Step 2: 實作 WatchMultiHost（含 initial snapshot）**

與 `Watch()` 一致，必須先推送 initial snapshot，否則客戶端在第一次變化前看不到任何內容。

```go
func (s *Service) WatchMultiHost(
	req *tsmv1.WatchMultiHostRequest,
	stream tsmv1.SessionManager_WatchMultiHostServer,
) error {
	if s.mhub == nil {
		return status.Error(codes.Unavailable, "not in hub mode")
	}

	// initial snapshot — 不等待第一次 broadcast
	if s.hubMgr != nil {
		if snap := s.hubMgr.Snapshot(); snap != nil && len(snap.Hosts) > 0 {
			if err := stream.Send(snap); err != nil {
				return err
			}
		}
	}

	ch := s.mhub.Register()
	defer s.mhub.Unregister(ch)

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case snap, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(snap); err != nil {
				return err
			}
		}
	}
}
```

- [ ] **Step 3: 編譯驗證**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/daemon/service.go
git commit -m "feat: WatchMultiHost gRPC 端點實作"
```

---

## Chunk 6: Mutation Routing — 遠端操作代理（P0）

> **關鍵問題**：遠端 TUI 使用者在 hub 模式下進行操作（kill/create/rename session），
> 這些 mutation 必須路由到**目標主機的 daemon**，而非 hub daemon 的本機 tmux。
> 若不處理，使用者在 m1 畫面上 kill mlab 的 session 會誤殺 air 本機的 session。

### Task 7.5: HubManager 代理 mutation 到目標主機

**Files:**
- Modify: `internal/daemon/hub.go`
- Modify: `internal/daemon/hub_test.go`

- [ ] **Step 1: 寫失敗測試 — ProxyMutation 路由**

```go
func TestHubManager_ProxyMutation_RoutesToCorrectHost(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	// 建立 mock client，記錄呼叫
	var killedOn string
	mockClient := &mockMutationClient{
		killFn: func(ctx context.Context, name string) error {
			killedOn = name
			return nil
		},
	}

	// 註冊遠端主機並注入 mock client
	mgr.setHostClient("mlab", mockClient)

	// 代理 kill session 到 mlab
	resp, err := mgr.ProxyMutation(context.Background(), &tsmv1.ProxyMutationRequest{
		HostId:      "mlab",
		Type:        tsmv1.MutationType_MUTATION_KILL_SESSION,
		SessionName: "dev",
	})

	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "dev", killedOn)
}

func TestHubManager_ProxyMutation_LocalHost(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	// local host 的 mutation 使用 hub daemon 自己的 tmux
	var killedLocal string
	mgr.localMutationFn = func(req *tsmv1.ProxyMutationRequest) error {
		killedLocal = req.SessionName
		return nil
	}

	resp, err := mgr.ProxyMutation(context.Background(), &tsmv1.ProxyMutationRequest{
		HostId:      "local",
		Type:        tsmv1.MutationType_MUTATION_KILL_SESSION,
		SessionName: "main",
	})

	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "main", killedLocal)
}

func TestHubManager_ProxyMutation_UnknownHost(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	resp, err := mgr.ProxyMutation(context.Background(), &tsmv1.ProxyMutationRequest{
		HostId:      "nonexistent",
		Type:        tsmv1.MutationType_MUTATION_KILL_SESSION,
		SessionName: "x",
	})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "unknown host")
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_ProxyMutation
```
Expected: FAIL — `ProxyMutation` undefined

- [ ] **Step 3: 實作 HubManager.ProxyMutation**

```go
// MutationClient 定義代理 mutation 需要的 client 介面。
// 簽章對齊 internal/client/client.go 的實際方法，使 *client.Client 可直接實作。
type MutationClient interface {
	KillSession(ctx context.Context, name string) error
	RenameSession(ctx context.Context, sessionName, customName, newSessionName string) error
	CreateSession(ctx context.Context, name string) error
	MoveSession(ctx context.Context, name, groupID string) error
}

// setHostClient 供測試注入 mock MutationClient（unexported 即可）。
func (m *HubManager) setHostClient(hostID string, c MutationClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.hosts[hostID]; ok {
		h.client = c
	}
}

// ProxyMutation 將操作路由到目標主機的 daemon。
func (m *HubManager) ProxyMutation(
	ctx context.Context, req *tsmv1.ProxyMutationRequest,
) (*tsmv1.ProxyMutationResponse, error) {
	m.mu.RLock()
	h, ok := m.hosts[req.HostId]
	m.mu.RUnlock()

	if !ok {
		return &tsmv1.ProxyMutationResponse{
			Success: false, Error: "unknown host: " + req.HostId,
		}, nil
	}

	var err error
	switch {
	case h.isLocal:
		err = m.localMutationFn(req)
	case h.client != nil:
		err = m.dispatchRemoteMutation(ctx, h.client, req)
	default:
		return &tsmv1.ProxyMutationResponse{
			Success: false, Error: "host not connected: " + req.HostId,
		}, nil
	}

	if err != nil {
		return &tsmv1.ProxyMutationResponse{
			Success: false, Error: err.Error(),
		}, nil
	}
	return &tsmv1.ProxyMutationResponse{Success: true}, nil
}

func (m *HubManager) dispatchRemoteMutation(
	ctx context.Context, c MutationClient, req *tsmv1.ProxyMutationRequest,
) error {
	switch req.Type {
	case tsmv1.MutationType_MUTATION_KILL_SESSION:
		return c.KillSession(ctx, req.SessionName)
	case tsmv1.MutationType_MUTATION_RENAME_SESSION:
		return c.RenameSession(ctx, req.SessionName, req.NewName, req.NewName)
	case tsmv1.MutationType_MUTATION_CREATE_SESSION:
		return c.CreateSession(ctx, req.SessionName)
	case tsmv1.MutationType_MUTATION_MOVE_SESSION:
		return c.MoveSession(ctx, req.SessionName, req.GroupId)
	default:
		return fmt.Errorf("unsupported mutation type: %v", req.Type)
	}
}
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_ProxyMutation
```
Expected: PASS

### Task 7.6: Service 實作 ProxyMutation gRPC 端點

**Files:**
- Modify: `internal/daemon/service.go`

- [ ] **Step 1: 實作 ProxyMutation**

```go
func (s *Service) ProxyMutation(
	ctx context.Context, req *tsmv1.ProxyMutationRequest,
) (*tsmv1.ProxyMutationResponse, error) {
	if s.hubMgr == nil {
		return nil, status.Error(codes.Unavailable, "not in hub mode")
	}
	return s.hubMgr.ProxyMutation(ctx, req)
}
```

- [ ] **Step 2: 編譯驗證**

```bash
go build ./...
```

### Task 7.7: Client 新增 ProxyMutation 方法

**Files:**
- Modify: `internal/client/client.go`

- [ ] **Step 1: 新增 ProxyMutation**

```go
func (c *Client) ProxyMutation(
	ctx context.Context, hostID string, mutType tsmv1.MutationType,
	sessionName, newName, groupID string,
) error {
	resp, err := c.rpc.ProxyMutation(ctx, &tsmv1.ProxyMutationRequest{
		HostId:      hostID,
		Type:        mutType,
		SessionName: sessionName,
		NewName:     newName,
		GroupId:     groupID,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("proxy mutation failed: %s", resp.Error)
	}
	return nil
}
```

- [ ] **Step 2: TUI Hub 模式 mutation 路由**

在 `internal/ui/app.go` 中，當 `Deps.HubMode = true` 時，所有 mutation 操作需透過 `Client.ProxyMutation()` 路由，並攜帶 `host_id`。

現有 mutation 呼叫位置（KillSession、RenameSession 等）需判斷 HubMode：

```go
if m.deps.HubMode {
	err := m.deps.Client.ProxyMutation(ctx, targetHostID,
		tsmv1.MutationType_MUTATION_KILL_SESSION, sessionName, "", "")
} else {
	// 原有直接呼叫
}
```

> **HostPicker 行為**：Hub 模式下 HostPicker 不需要獨立運作。
> 多主機畫面由 hub 的 MultiHostSnapshot 提供，session 列表已含 host_id。
> TUI 只需從 session 的 host_id 決定 mutation 路由目標。

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/ internal/client/ internal/ui/
git commit -m "feat: ProxyMutation — 遠端 TUI 操作代理到目標主機"
```

---

## Chunk 7: Daemon Hub 模式啟動

### Task 8: Daemon 啟動加入 hub 模式

**Files:**
- Modify: `internal/daemon/daemon.go`
- Modify: `cmd/tsm/main.go`

- [ ] **Step 1: Daemon.Run() 支援 hub 模式**

在 `Daemon` struct 新增：

```go
type Daemon struct {
	cfg       config.Config
	server    *grpc.Server
	hub       *WatcherHub
	mhub      *MultiHostHub  // nil = normal mode
	hubMgr    *HubManager    // nil = normal mode
	state     *StateManager
	store     *store.Store
	cancelRun context.CancelFunc
}
```

在 `Run()` 中，建立 gRPC service 後：

```go
// Hub 模式：若 config 中有非 local 且 enabled 的 host，啟動 HubManager
if hasRemoteHosts(d.cfg.Hosts) {
	d.mhub = NewMultiHostHub()
	d.hubMgr = NewHubManager(d.mhub)
	for _, h := range d.cfg.Hosts {
		d.hubMgr.AddHost(h)
	}
	hubSockPath := SocketPath(d.cfg)
	go d.hubMgr.StartAll(ctx, hubSockPath)
}
```

- [ ] **Step 2: 新增 hasRemoteHosts helper**

```go
func hasRemoteHosts(hosts []config.HostEntry) bool {
	for _, h := range hosts {
		if !h.IsLocal() && h.Enabled {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: 傳遞 mhub 到 Service**

```go
svc := NewService(mgr, st, d.hub, d.mhub, d.hubMgr, d.state)
```

- [ ] **Step 4: Shutdown 清理 HubManager**

```go
func (d *Daemon) Shutdown() {
	if d.hubMgr != nil {
		d.hubMgr.Close()
	}
	if d.mhub != nil {
		d.mhub.Close()
	}
	// ... existing cleanup
}
```

- [ ] **Step 5: CLI — daemon start 傳入 host config**

在 `cmd/tsm/main.go` 的 daemon start 路徑中，載入 config（含 hosts），傳入 `Daemon`。

目前 daemon start 用的 `config.Default()` 只有 local host。需改為載入 `~/.config/tsm/config.toml`（如存在），讓 daemon 知道有哪些遠端主機。

- [ ] **Step 6: 編譯並執行測試**

```bash
go build ./... && go test ./... -race
```

- [ ] **Step 7: Commit**

```bash
git add internal/daemon/ cmd/tsm/
git commit -m "feat: daemon hub 模式 — 自動偵測並啟動多主機聚合"
```

---

## Chunk 8: Client WatchMultiHost + TUI 整合

### Task 9: Client 新增 WatchMultiHost 方法

**Files:**
- Modify: `internal/client/client.go`

- [ ] **Step 1: 新增 WatchMultiHost 和 RecvMultiHostSnapshot**

```go
func (c *Client) WatchMultiHost(ctx context.Context) error {
	stream, err := c.rpc.WatchMultiHost(ctx, &tsmv1.WatchMultiHostRequest{})
	if err != nil {
		return err
	}
	c.mhStream = stream
	return nil
}

func (c *Client) RecvMultiHostSnapshot() (*tsmv1.MultiHostSnapshot, error) {
	if c.mhStream == nil {
		return nil, fmt.Errorf("multi-host watch not started")
	}
	return c.mhStream.Recv()
}
```

在 Client struct 新增 `mhStream tsmv1.SessionManager_WatchMultiHostClient`。

- [ ] **Step 2: Commit**

### Task 10: TUI 支援 hub 模式

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/convert.go`

- [ ] **Step 1: 新增 HubSnapshotMsg**

```go
type HubSnapshotMsg struct {
	Snapshot *tsmv1.MultiHostSnapshot
	Err      error
}
```

- [ ] **Step 2: 新增 recvHubSnapshotCmd**

```go
func recvHubSnapshotCmd(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		snap, err := c.RecvMultiHostSnapshot()
		return HubSnapshotMsg{Snapshot: snap, Err: err}
	}
}
```

- [ ] **Step 3: 新增 ConvertMultiHostSnapshot**

在 `convert.go`：

```go
func ConvertMultiHostSnapshot(mhs *tsmv1.MultiHostSnapshot) []HostSnapshotInput {
	var inputs []HostSnapshotInput
	for _, hs := range mhs.Hosts {
		input := HostSnapshotInput{
			HostID: hs.HostId,
			Name:   hs.Name,
			Color:  hs.Color,
			Status: int(hs.Status), // HostStatus enum → int for TUI layer
			Error:  hs.Error,
		}
		if hs.Snapshot != nil {
			input.Sessions = ConvertProtoSessions(hs.Snapshot.Sessions)
			input.Groups = ConvertProtoGroups(hs.Snapshot.Groups)
		}
		inputs = append(inputs, input)
	}
	return inputs
}
```

- [ ] **Step 4: Update handler 處理 HubSnapshotMsg**

與現有 `MultiHostSnapshotMsg` 處理邏輯一致，使用 `FlattenMultiHost()`。

- [ ] **Step 5: Deps 新增 HubMode**

```go
type Deps struct {
	// ...existing fields...
	HubMode bool // true = 使用 WatchMultiHost
}
```

TUI init 時根據 `HubMode` 選擇：
- `HubMode = true` → `c.WatchMultiHost(ctx)` + `recvHubSnapshotCmd`
- `HubMode = false` → 現有邏輯

- [ ] **Step 6: 執行測試**

```bash
go test ./internal/ui/... -v -race
```

- [ ] **Step 7: Commit**

```bash
git add internal/client/ internal/ui/
git commit -m "feat: TUI 支援 hub 模式 — WatchMultiHost 接收多主機快照"
```

---

## Chunk 9: Attach 設定 + Ctrl+Q Bind 整合

### Task 11: Attach 時設定遠端 tmux option

**Files:**
- Modify: `internal/remote/attach.go`
- Modify: `internal/remote/attach_test.go`

- [ ] **Step 1: 寫失敗測試 — SetHubSocket / ClearHubSocket**

```go
func TestSetHubSocket(t *testing.T) {
	var captured []string
	orig := sshRunFn
	sshRunFn = func(args ...string) error {
		captured = args
		return nil
	}
	defer func() { sshRunFn = orig }()

	SetHubSocket("mlab", "/tmp/tsm-hub-abc.sock")

	assert.Contains(t, strings.Join(captured, " "), "@tsm_hub_socket")
	assert.Contains(t, strings.Join(captured, " "), "/tmp/tsm-hub-abc.sock")
}
```

- [ ] **Step 2: 執行測試確認失敗**

- [ ] **Step 3: 實作 SetHubSocket / ClearHubSocket**

```go
// SetHubSocket 在遠端主機的 tmux 設定 hub socket 路徑。
func SetHubSocket(host, socketPath string) error {
	return sshRunFn("ssh", host, "tmux", "set-option", "-g", "@tsm_hub_socket", socketPath)
}

// ClearHubSocket 清除遠端主機的 hub socket 設定。
func ClearHubSocket(host string) error {
	return sshRunFn("ssh", host, "tmux", "set-option", "-gu", "@tsm_hub_socket")
}
```

- [ ] **Step 4: 執行測試確認通過**

- [ ] **Step 5: 在 attach 流程中呼叫**

在 `cmd/tsm/main.go` 的 attach 流程（選擇遠端 session 後）：

```go
// attach 前：設定 hub socket
reverseSock := remote.ReverseSocketPath(hostCfg.Address)
remote.SetHubSocket(hostCfg.Address, reverseSock)

// attach
result := remote.Attach(hostCfg.Address, sessionName)

// detach 後：清除（可選，下次 attach 會覆寫）
```

- [ ] **Step 6: Commit**

### Task 12: Ctrl+Q Bind 條件判斷

**Files:**
- Modify: `internal/bind/bind.go`
- Modify: `internal/bind/bind_test.go`

- [ ] **Step 1: 寫失敗測試 — bindBlock 含 hub 判斷**

```go
func TestBindBlock_HasHubSocketCheck(t *testing.T) {
	if !strings.Contains(bindBlock, "@tsm_hub_socket") {
		t.Error("bindBlock 應包含 @tsm_hub_socket 條件判斷")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

- [ ] **Step 3: 更新 bindBlock**

```go
bindBlock = `# [tsm] begin
bind-key -n C-q run-shell 'HUB=$(tmux show-option -gqv @tsm_hub_socket); \
  if [ -S "$HUB" ]; then \
    tmux display-popup -E -w "80%" -h "80%" "tsm --hub-socket $HUB --inline"; \
  else \
    tmux display-popup -E -w "80%" -h "80%" "tsm $(tmux show-option -gqv @tsm_popup_args) --inline"; \
  fi'
# [tsm] end`
```

邏輯：
1. 讀取 `@tsm_hub_socket`
2. 若 socket 檔案存在（`-S`）→ 使用 `--hub-socket` 連線到 hub
3. 否則 → 使用原有的 `@tsm_popup_args` 本地模式

- [ ] **Step 4: 執行測試確認通過**

- [ ] **Step 5: 全部測試**

```bash
go test ./... -race
```

- [ ] **Step 6: Commit**

```bash
git add internal/bind/ internal/remote/ cmd/tsm/
git commit -m "feat: Ctrl+Q 自動偵測 hub socket，支援遠端多主機畫面"
```

---

## Chunk 10: CLI --hub-socket Flag + 端到端整合

### Task 13: --hub-socket CLI flag

**Files:**
- Modify: `cmd/tsm/main.go`
- Modify: `cmd/tsm/parse_test.go`

- [ ] **Step 1: 寫失敗測試 — parseHubSocket**

```go
func TestParseHubSocket(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "無 hub-socket", args: []string{"--inline"}, want: ""},
		{name: "有 hub-socket", args: []string{"--hub-socket", "/tmp/tsm-hub-abc.sock", "--inline"}, want: "/tmp/tsm-hub-abc.sock"},
		{name: "尾端缺值", args: []string{"--hub-socket"}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHubSocket(tt.args)
			if got != tt.want {
				t.Errorf("parseHubSocket(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

- [ ] **Step 3: 實作 parseHubSocket**

```go
func parseHubSocket(args []string) string {
	for i, a := range args {
		if a == "--hub-socket" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			return args[i+1]
		}
	}
	return ""
}
```

- [ ] **Step 4: 執行測試確認通過**

- [ ] **Step 5: runTUI() 整合 hub socket**

在 `runTUI()` 中：

```go
hubSocket := parseHubSocket(args)

if hubSocket != "" {
	// Hub 模式：透過 reverse tunnel socket 連線到 hub daemon
	// 連線失敗時 graceful degradation 到 local 模式（而非 exit 1）
	c, err := client.DialSocket(hubSocket)
	if err == nil {
		if err := c.WatchMultiHost(ctx); err == nil {
			deps := ui.Deps{
				Client:  c,
				Cfg:     cfg,
				HubMode: true,
			}
			// ... 啟動 TUI
			return
		}
		c.Close()
	}
	// graceful degradation: hub socket 不可用 → 降級為 local 模式
	log.Printf("hub socket 連線失敗，降級為 local 模式: %v", err)
	// 繼續往下走 local 模式路徑
}
```

- [ ] **Step 6: 全套測試**

```bash
go test ./... -race
```

- [ ] **Step 7: Commit**

```bash
git add cmd/tsm/
git commit -m "feat: --hub-socket flag 支援遠端 TUI 連線 hub daemon"
```

---

## Chunk 11: Daemon 重啟策略 + TUI HostManager 降級 + Graceful Degradation

### Task 14: TUI 保留 HostManager 作為降級路徑

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `cmd/tsm/main.go`

當 daemon 尚未升級到 hub 模式（或 hub 不可用）時，TUI 仍可透過原有的 HostManager 直接管理連線。

- [ ] **Step 1: runTUI() 決策邏輯**

```go
hubSocket := parseHubSocket(args)

if hubSocket != "" {
	// 路徑 A：Hub 模式（遠端 Ctrl+Q）
	// → DialSocket(hubSocket) + WatchMultiHost
} else if hostMode {
	// 路徑 B：嘗試 daemon hub 模式
	c, err := client.Dial(cfg)
	if err == nil {
		if err := c.WatchMultiHost(ctx); err == nil {
			// daemon 支援 hub → 使用 hub 模式
			deps := ui.Deps{Client: c, Cfg: cfg, HubMode: true}
			// ...
			return
		}
	}
	// 降級：daemon 無 hub → 原有 HostManager 路徑
	// ...existing HostManager code...
} else {
	// 路徑 C：純 local 模式
	// ...existing code...
}
```

- [ ] **Step 2: 測試降級行為**

確認 daemon 回傳 `codes.Unavailable` 時，TUI 正確降級到 HostManager。

- [ ] **Step 3: 測試 --hub-socket graceful degradation**

```go
func TestHubSocket_GracefulDegradation(t *testing.T) {
	// 給定不存在的 socket path
	// 預期：不 panic、不 exit，而是降級到 local 模式
	// 實際上這需要在 runTUI integration test 或 e2e 測試中驗證
}
```

場景驗證：
| 情況 | 預期行為 |
|------|----------|
| hub socket 存在且可連線 | 使用 WatchMultiHost hub 模式 |
| hub socket 檔案不存在 | bindBlock `[ -S "$HUB" ]` 不成立，直接走 local |
| hub socket 存在但連線失敗 | DialSocket 失敗 → 降級為 local 模式 |
| hub socket 連線後 stream 斷開 | TUI 應顯示 reconnecting modal 或降級 |

- [ ] **Step 4: Commit**

### Task 15: Daemon 重啟保留 hub 狀態

**Files:**
- Modify: `internal/daemon/daemon.go`

- [ ] **Step 1: Hub 模式自動偵測**

Daemon 啟動時：
1. 載入 `config.toml`
2. 若有 enabled 的遠端 host → 自動進入 hub 模式
3. 無需額外 CLI flag

config 的寫入已由 `tsm --host <name>` 在第一次執行時完成（MergeHosts + SaveConfig），daemon 只需要讀取。

- [ ] **Step 2: 重啟自動重建 tunnel**

`HubManager.StartAll()` 已處理重連邏輯（指數退避），daemon 重啟時會自動重建所有 tunnel。

- [ ] **Step 3: Commit**

---

## 驗收標準

完成後應通過以下場景：

| 場景 | 預期行為 |
|------|----------|
| air: `tsm` | 純 local，Ctrl+Q 也是 local |
| air: `tsm --host` | hub 模式，顯示 air + 所有遠端 sessions |
| air: Ctrl+Q | 與上次啟動模式相同 |
| m1: Ctrl+Q（從 air attach 進入） | 透過 reverse tunnel 連回 air hub，顯示完整多主機畫面 |
| m1: hub 畫面上 kill mlab 的 session | ProxyMutation 路由到 mlab daemon，不影響 air/m1 |
| m1: hub 畫面上 kill 自己的 session | ProxyMutation 路由到 m1 daemon |
| m1: 直接 SSH 進入，Ctrl+Q | m1 本地 tsm，只顯示 m1 sessions |
| air 斷線 | m1 Ctrl+Q 偵測 socket 不存在 → 降級為 m1 local |
| hub socket 存在但連線失敗 | 降級為 local 模式（不 crash） |
| daemon 重啟 | 自動重建 hub + tunnel |

## 遷移與相容

- 現有 `hostmgr.HostManager` 保留作為降級路徑，不刪除
- Proto 變更為純新增（新 message + 新 RPC），不影響舊版客戶端
- Bind block 變更需要使用者執行 `tsm setup` 升級
- 無 breaking changes — 無 hub 的環境行為完全不變
