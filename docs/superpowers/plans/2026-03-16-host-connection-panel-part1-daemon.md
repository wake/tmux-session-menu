# Host Connection Panel Part 1: Proto + Daemon 層 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 為連線面板提供 daemon 基礎設施：HostState 擴充 `last_connected`、ReconnectHost RPC、hubHost per-host context。

**Architecture:** 在現有 `HubManager` 上擴充 per-host cancel/done 機制，新增 `ReconnectHost` RPC 讓 UI 能觸發特定主機的 tunnel 重建。`HostState` proto 新增 `last_connected` 時間戳。

**Tech Stack:** Go 1.24 / gRPC / protobuf / testify

**Spec:** `docs/superpowers/specs/2026-03-16-host-connection-panel-design.md`

---

## File Structure

| 檔案 | 職責 | 動作 |
|------|------|------|
| `api/proto/tsm/v1/tsm.proto` | HostState + ReconnectHost 定義 | 修改 |
| `api/tsm/v1/tsm.pb.go` | protoc 生成 | 重新生成 |
| `api/tsm/v1/tsm_grpc.pb.go` | protoc 生成 | 重新生成 |
| `internal/daemon/hub.go` | hubHost 擴充、ReconnectHost、per-host context | 修改 |
| `internal/daemon/hub_test.go` | ReconnectHost 測試 | 修改 |
| `internal/daemon/hub_remote_test.go` | per-host context 測試 | 修改 |
| `internal/daemon/service.go` | ReconnectHost handler | 修改 |
| `internal/client/client.go` | ReconnectHost client 方法 | 修改 |
| `internal/client/client_test.go` | ReconnectHost client 測試 | 修改 |

---

## Chunk 1: HostState 擴充 + ReconnectHost RPC + per-host context

### Task 1: HostState proto 新增 last_connected

**Files:**
- Modify: `api/proto/tsm/v1/tsm.proto:58-66`
- Modify: `internal/daemon/hub.go:67-76` (hubHost struct)
- Modify: `internal/daemon/hub.go:126-140` (Snapshot)

- [ ] **Step 1: 新增 proto 欄位**

在 `api/proto/tsm/v1/tsm.proto` 的 `HostState` message 最後加：

```protobuf
message HostState {
  string host_id = 1;
  string name = 2;
  string color = 3;
  HostStatus status = 4;
  string error = 5;
  StateSnapshot snapshot = 6;
  string address = 7;
  google.protobuf.Timestamp last_connected = 8;
}
```

- [ ] **Step 2: 重新生成 proto**

```bash
protoc -I api/proto --go_out=api --go_opt=paths=source_relative \
       --go-grpc_out=api --go-grpc_opt=paths=source_relative \
       tsm/v1/tsm.proto
```

- [ ] **Step 3: hubHost 加 connectedAt 欄位**

在 `internal/daemon/hub.go` 的 `hubHost` struct 加：

```go
type hubHost struct {
	config       config.HostEntry
	status       tsmv1.HostStatus
	snapshot     *tsmv1.StateSnapshot
	lastErr      string
	connectedAt  time.Time           // 新增：最後連線時間
	isLocal      bool
	client       MutationClient
	remoteClient HubRemoteClient
}
```

- [ ] **Step 4: runRemote 連線成功時記錄 connectedAt**

在 `hub.go` 的 `runRemote` 中，`connectedAt := time.Now()` 已存在（約 L357），但只是局部變數。改為同時寫入 `hubHost`：

在 `m.mu.Lock()` / `m.mu.Unlock()` 區塊（L349-354）內加：

```go
m.mu.Lock()
if h, ok := m.hosts[hostID]; ok {
	h.client = rc
	h.remoteClient = rc
	h.connectedAt = time.Now()
}
m.mu.Unlock()
```

保留 `connectedAt := time.Now()` 局部變數（backoff 重置邏輯仍使用它）。

- [ ] **Step 5: Snapshot 輸出 last_connected**

在 `hub.go` 的 `Snapshot()` 方法（約 L126-140），`hs` 的建構中加：

```go
hs := &tsmv1.HostState{
	HostId:   h.config.Name,
	Name:     h.config.Name,
	Color:    h.config.Color,
	Status:   h.status,
	Error:    h.lastErr,
	Snapshot: h.snapshot,
	Address:  h.config.Address,
}
if !h.connectedAt.IsZero() {
	hs.LastConnected = timestamppb.New(h.connectedAt)
}
```

需要 import `"google.golang.org/protobuf/types/known/timestamppb"`。

- [ ] **Step 6: 寫測試**

在 `internal/daemon/hub_test.go` 加：

```go
func TestHubManager_Snapshot_LastConnected(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)
	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	// 模擬連線
	now := time.Now()
	mgr.mu.Lock()
	mgr.hosts["mlab"].connectedAt = now
	mgr.hosts["mlab"].status = tsmv1.HostStatus_HOST_STATUS_CONNECTED
	mgr.mu.Unlock()

	snap := mgr.Snapshot()
	require.NotNil(t, snap)
	require.Len(t, snap.Hosts, 1)
	assert.NotNil(t, snap.Hosts[0].LastConnected)
	assert.Equal(t, now.Unix(), snap.Hosts[0].LastConnected.AsTime().Unix())
}
```

- [ ] **Step 7: 執行測試**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_Snapshot_LastConnected
```

- [ ] **Step 8: Commit**

```bash
git add api/ internal/daemon/hub.go internal/daemon/hub_test.go
git commit -m "feat: HostState 新增 last_connected 欄位

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: hubHost per-host context（ReconnectHost 前提）

**Files:**
- Modify: `internal/daemon/hub.go:67-76` (hubHost struct)
- Modify: `internal/daemon/hub.go:278-301` (StartRemoteHosts)
- Modify: `internal/daemon/hub.go:304-390` (runRemote)
- Modify: `internal/daemon/hub_remote_test.go`

- [ ] **Step 1: hubHost 加 cancel 和 done**

```go
type hubHost struct {
	config       config.HostEntry
	status       tsmv1.HostStatus
	snapshot     *tsmv1.StateSnapshot
	lastErr      string
	connectedAt  time.Time
	isLocal      bool
	client       MutationClient
	remoteClient HubRemoteClient
	cancel       context.CancelFunc // 新增：per-host cancel
	done         chan struct{}       // 新增：goroutine 結束信號
}
```

- [ ] **Step 2: StartRemoteHosts 改為 per-host context**

現有（約 L278-301）：

```go
func (m *HubManager) StartRemoteHosts(ctx context.Context) {
	m.mu.Lock()
	rctx, cancel := context.WithCancel(ctx)
	m.remoteCancel = cancel
	// ...
	for _, id := range m.order {
		// ...
		m.remoteWg.Add(1)
		go m.runRemote(rctx, cfg, dialFn)
	}
}
```

改為：

```go
func (m *HubManager) StartRemoteHosts(ctx context.Context) {
	m.mu.Lock()
	rctx, cancel := context.WithCancel(ctx)
	m.remoteCancel = cancel
	dialFn := m.dialFn
	m.mu.Unlock()

	if dialFn == nil {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, id := range m.order {
		h := m.hosts[id]
		if h.isLocal || !h.config.Enabled {
			continue
		}
		cfg := h.config
		hctx, hcancel := context.WithCancel(rctx)
		h.cancel = hcancel
		h.done = make(chan struct{})
		m.remoteWg.Add(1)
		go m.runRemote(hctx, h.done, cfg, dialFn)
	}
}
```

- [ ] **Step 3: runRemote 加 done channel 參數**

簽名改為：

```go
func (m *HubManager) runRemote(ctx context.Context, done chan struct{}, cfg config.HostEntry, dialFn HubDialFn) {
	defer m.remoteWg.Done()
	defer close(done)
	// ... 其餘不變
}
```

- [ ] **Step 4: 更新 hub_remote_test.go 的 mock**

`hub_remote_test.go` 中所有呼叫 `m.runRemote(ctx, cfg, dialFn)` 的地方改為 `m.runRemote(ctx, make(chan struct{}), cfg, dialFn)`。搜尋所有 `go m.runRemote` 呼叫點。

- [ ] **Step 5: 執行測試**

```bash
go test ./internal/daemon/... -v -race
```

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/hub.go internal/daemon/hub_remote_test.go
git commit -m "refactor: hubHost per-host context — cancel + done channel

為 ReconnectHost RPC 做準備：每台遠端主機有獨立的 context 和
done channel，可單獨 cancel 和等待 goroutine 結束。

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: ReconnectHost — HubManager 方法 + 測試

**Files:**
- Modify: `internal/daemon/hub.go` (ReconnectHost method)
- Modify: `internal/daemon/hub_test.go` (tests)

- [ ] **Step 1: 寫測試**

在 `internal/daemon/hub_test.go` 加：

```go
func TestHubManager_ReconnectHost_Local(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)
	mgr.AddHost(config.HostEntry{Name: "local", Enabled: true})

	err := mgr.ReconnectHost(context.Background(), "local")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "local")
}

func TestHubManager_ReconnectHost_Unknown(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	err := mgr.ReconnectHost(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestHubManager_ReconnectHost_NoDialFn(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)
	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	err := mgr.ReconnectHost(context.Background(), "mlab")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dial")
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_ReconnectHost
```

預期：編譯失敗（方法不存在）。

- [ ] **Step 3: 實作 ReconnectHost**

在 `hub.go` 加（在 `CancelPendingAttach` 後面）：

```go
// ReconnectHost 強制重建指定主機的連線。
// cancel 現有 goroutine → 等待結束 → 重新啟動。
func (m *HubManager) ReconnectHost(ctx context.Context, hostID string) error {
	m.mu.Lock()
	h, ok := m.hosts[hostID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown host: %s", hostID)
	}
	if h.isLocal {
		m.mu.Unlock()
		return fmt.Errorf("local 主機不需要重連")
	}
	if m.dialFn == nil {
		m.mu.Unlock()
		return fmt.Errorf("dial function not configured")
	}
	dialFn := m.dialFn

	// Cancel 現有 goroutine
	if h.cancel != nil {
		h.cancel()
	}
	done := h.done
	m.mu.Unlock()

	// 等待 goroutine 結束
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// 重新啟動
	m.mu.Lock()
	rctx := context.Background()
	if m.remoteCancel != nil {
		// 使用 parent context 派生（如果 Close 被呼叫會一起取消）
		// 但 remoteCancel 本身對應的 ctx 可能已取消，改用新的 background
	}
	cfg := h.config
	hctx, hcancel := context.WithCancel(rctx)
	h.cancel = hcancel
	h.done = make(chan struct{})
	h.status = tsmv1.HostStatus_HOST_STATUS_CONNECTING
	h.lastErr = ""
	m.remoteWg.Add(1)
	m.mu.Unlock()

	go m.runRemote(hctx, h.done, cfg, dialFn)

	return nil
}
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_ReconnectHost
```

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/hub.go internal/daemon/hub_test.go
git commit -m "feat: HubManager.ReconnectHost — 強制重建特定主機連線

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: ReconnectHost RPC — proto + service + client

**Files:**
- Modify: `api/proto/tsm/v1/tsm.proto`
- Regenerate: `api/tsm/v1/*.pb.go`
- Modify: `internal/daemon/service.go`
- Modify: `internal/client/client.go`
- Modify: `internal/client/client_test.go`

- [ ] **Step 1: 新增 proto 定義**

在 `tsm.proto` 的 message 區塊（`CancelPendingAttachResponse` 後面）加：

```protobuf
// ReconnectHostRequest 強制重建指定主機的連線。
message ReconnectHostRequest {
  string host_id = 1;
}

// ReconnectHostResponse 回傳重建結果。
message ReconnectHostResponse {
  bool success = 1;
  string error = 2;
}
```

在 service 區塊的 `CancelPendingAttach` RPC 後加：

```protobuf
  // ReconnectHost 強制重建指定主機的 SSH tunnel（hub 模式專用）。
  rpc ReconnectHost(ReconnectHostRequest) returns (ReconnectHostResponse);
```

- [ ] **Step 2: 重新生成 proto**

```bash
protoc -I api/proto --go_out=api --go_opt=paths=source_relative \
       --go-grpc_out=api --go-grpc_opt=paths=source_relative \
       tsm/v1/tsm.proto
```

- [ ] **Step 3: 實作 service handler**

在 `internal/daemon/service.go` 加：

```go
// ReconnectHost 強制重建指定主機的連線。
func (s *Service) ReconnectHost(
	ctx context.Context, req *tsmv1.ReconnectHostRequest,
) (*tsmv1.ReconnectHostResponse, error) {
	if s.hubMgr == nil {
		return nil, status.Error(codes.Unavailable, "not in hub mode")
	}
	if err := s.hubMgr.ReconnectHost(ctx, req.HostId); err != nil {
		return &tsmv1.ReconnectHostResponse{
			Success: false, Error: err.Error(),
		}, nil
	}
	return &tsmv1.ReconnectHostResponse{Success: true}, nil
}
```

- [ ] **Step 4: 實作 client 方法**

在 `internal/client/client.go` 加：

```go
// ReconnectHost 強制重建指定主機的連線。
func (c *Client) ReconnectHost(ctx context.Context, hostID string) error {
	resp, err := c.rpc.ReconnectHost(ctx, &tsmv1.ReconnectHostRequest{HostId: hostID})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}
```

- [ ] **Step 5: 寫 client 測試**

在 `internal/client/client_test.go` 加：

```go
func TestReconnectHost(t *testing.T) {
	exec := &fakeExecutor{}
	c, cleanup := setupTestClient(t, exec)
	defer cleanup()

	// nil hubMgr → gRPC error
	err := c.ReconnectHost(context.Background(), "mlab")
	assert.Error(t, err)
}
```

- [ ] **Step 6: 執行全部測試**

```bash
go test ./internal/daemon/... ./internal/client/... -v -race
```

- [ ] **Step 7: Commit**

```bash
git add api/ internal/daemon/service.go internal/client/client.go internal/client/client_test.go
git commit -m "feat: 新增 ReconnectHost RPC — 強制重建特定主機 tunnel

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: 版本推進

**Files:**
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: 推進版本號**

```bash
echo "0.46.0" > VERSION
```

- [ ] **Step 2: 更新 CHANGELOG**

在最上方加入：
- HostState 新增 `last_connected` 欄位
- hubHost per-host context 重構
- 新增 `ReconnectHost` RPC

- [ ] **Step 3: 確認建置成功**

```bash
make build && make test
```

- [ ] **Step 4: Commit**

```bash
git add VERSION CHANGELOG.md
git commit -m "chore: 版本推進至 0.46.0、更新 CHANGELOG

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```
