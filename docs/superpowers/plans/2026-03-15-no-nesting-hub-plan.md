# 消除 tmux 嵌套 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 消除 tmux 嵌套，統一跨主機切換為 detach → 外層 tsm 接手，並加入 host preflight check 系統。

**Architecture:** popup 選到跨主機 session 時改用 `RequestAttach + tmux detach-client` 委託外層 tsm for loop 處理，取代現有的 `tmux new-window "ssh ..."`（嵌套路徑）。新增 `internal/hostcheck/` 模組做主機依賴檢測。

**Tech Stack:** Go 1.24 / gRPC / protobuf / Bubble Tea / testify

**Spec:** `docs/superpowers/specs/2026-03-15-no-nesting-hub-design.md`

---

## File Structure

| 檔案 | 職責 | 動作 |
|------|------|------|
| `internal/hostcheck/hostcheck.go` | Host preflight check 核心邏輯 | 新建 |
| `internal/hostcheck/hostcheck_test.go` | hostcheck 單元測試 | 新建 |
| `api/proto/tsm/v1/tsm.proto` | CancelPendingAttach RPC 定義 | 修改 |
| `api/tsm/v1/tsm.pb.go` | protoc 生成 | 重新生成 |
| `api/tsm/v1/tsm_grpc.pb.go` | protoc 生成 | 重新生成 |
| `internal/daemon/hub.go` | pending TTL + CancelPendingAttach | 修改 |
| `internal/daemon/hub_test.go` | pending TTL + Cancel 測試 | 修改 |
| `internal/daemon/service.go` | CancelPendingAttach handler | 修改 |
| `internal/client/client.go` | CancelPendingAttach client 方法 | 修改 |
| `internal/client/client_test.go` | CancelPendingAttach client 測試 | 修改 |
| `internal/ui/info.go` | [i] 面板持久化 @tsm_hub_socket | 修改 |
| `internal/ui/info_test.go` | [i] 面板持久化測試 | 修改 |
| `cmd/tsm/main.go` | popup 跨主機路徑改 RequestAttach+detach、handlePendingAttach helper | 修改 |

---

## Chunk 1: hostcheck 模組 + pending TTL + CancelPendingAttach

### Task 1: hostcheck 模組 — 核心邏輯與測試

**Files:**
- Create: `internal/hostcheck/hostcheck.go`
- Create: `internal/hostcheck/hostcheck_test.go`

- [ ] **Step 1: 寫 local tmux 檢測的失敗測試**

在 `internal/hostcheck/hostcheck_test.go` 寫入：

```go
package hostcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckLocal_TmuxFound(t *testing.T) {
	c := &Checker{
		LookPathFn: func(file string) (string, error) {
			if file == "tmux" {
				return "/usr/bin/tmux", nil
			}
			return "", &lookPathError{file}
		},
	}
	r := c.CheckLocal()
	assert.Equal(t, StatusReady, r.Status)
	assert.Nil(t, r.SSH, "local 不檢查 SSH")
	assert.NotNil(t, r.Tmux)
	assert.True(t, *r.Tmux)
}

func TestCheckLocal_TmuxNotFound(t *testing.T) {
	c := &Checker{
		LookPathFn: func(file string) (string, error) {
			return "", &lookPathError{file}
		},
	}
	r := c.CheckLocal()
	assert.Equal(t, StatusFailed, r.Status)
	assert.NotNil(t, r.Tmux)
	assert.False(t, *r.Tmux)
	assert.Contains(t, r.Message, "tmux")
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/hostcheck/... -v -race -run TestCheckLocal
```

預期：編譯失敗（package 不存在）。

- [ ] **Step 3: 只實作 CheckLocal（最小實作）**

在 `internal/hostcheck/hostcheck.go` 寫入（只含 types + CheckLocal，不含 CheckRemote 和 CheckAll）：

```go
package hostcheck

import (
	"fmt"
	"os/exec"
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
	SSH       *bool     // nil=local 不適用, true=通過, false=失敗
	Tmux      *bool     // true=通過, false=失敗
	Message   string    // 失敗時的錯誤訊息
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
	ExecFn     ExecFn                             // 預設執行 exec.Command().Run()
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
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/hostcheck/... -v -race -run TestCheckLocal
```

預期：2 個測試 PASS。

- [ ] **Step 5: 寫 remote 檢測的失敗測試**

在 `hostcheck_test.go` 追加（需加 `"fmt"` 到 import）：

```go
func TestCheckRemote_AllPass(t *testing.T) {
	c := &Checker{
		ExecFn: func(name string, args ...string) error {
			return nil // 所有指令成功
		},
	}
	r := c.CheckRemote("mlab")
	assert.Equal(t, StatusReady, r.Status)
	assert.True(t, *r.SSH)
	assert.True(t, *r.Tmux)
}

func TestCheckRemote_SSHFail(t *testing.T) {
	c := &Checker{
		ExecFn: func(name string, args ...string) error {
			return fmt.Errorf("connection refused")
		},
	}
	r := c.CheckRemote("bad-host")
	assert.Equal(t, StatusFailed, r.Status)
	assert.False(t, *r.SSH)
	assert.Nil(t, r.Tmux, "SSH 失敗時不檢查 tmux")
	assert.Contains(t, r.Message, "SSH")
}

func TestCheckRemote_SSHOk_TmuxMissing(t *testing.T) {
	callCount := 0
	c := &Checker{
		ExecFn: func(name string, args ...string) error {
			callCount++
			if callCount == 1 {
				return nil // SSH 成功
			}
			return fmt.Errorf("tmux not found") // tmux 檢查失敗
		},
	}
	r := c.CheckRemote("mlab")
	assert.Equal(t, StatusFailed, r.Status)
	assert.True(t, *r.SSH)
	assert.False(t, *r.Tmux)
	assert.Contains(t, r.Message, "tmux")
}
```

- [ ] **Step 6: 執行測試確認失敗**

```bash
go test ./internal/hostcheck/... -v -race -run TestCheckRemote
```

預期：編譯失敗（`CheckRemote` 方法不存在）。

- [ ] **Step 7: 實作 CheckRemote**

在 `hostcheck.go` 追加：

```go
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
```

- [ ] **Step 8: 執行測試確認通過**

```bash
go test ./internal/hostcheck/... -v -race -run TestCheckRemote
```

預期：3 個測試 PASS。

- [ ] **Step 9: 寫 CheckAll 並行檢測的失敗測試**

在 `hostcheck_test.go` 追加（需加 `"fmt"`, `"sync"`, `"time"` 到 import）：

```go
func TestCheckAll_Parallel(t *testing.T) {
	c := &Checker{
		LookPathFn: func(file string) (string, error) { return "/usr/bin/tmux", nil },
		ExecFn:     func(name string, args ...string) error { return nil },
	}
	hosts := map[string]string{
		"local": "",
		"mlab1": "mlab1",
		"mlab2": "mlab2",
	}
	results := c.CheckAll(hosts, 5)
	assert.Len(t, results, 3)
	for name, r := range results {
		assert.Equal(t, StatusReady, r.Status, "host %s should be ready", name)
	}
}

func TestCheckAll_SemaphoreLimit(t *testing.T) {
	var mu sync.Mutex
	maxSeen := 0
	current := 0

	c := &Checker{
		LookPathFn: func(file string) (string, error) { return "/usr/bin/tmux", nil },
		ExecFn: func(name string, args ...string) error {
			mu.Lock()
			current++
			if current > maxSeen {
				maxSeen = current
			}
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			current--
			mu.Unlock()
			return nil
		},
	}
	hosts := map[string]string{}
	for i := 0; i < 10; i++ {
		hosts[fmt.Sprintf("host%d", i)] = fmt.Sprintf("host%d", i)
	}
	_ = c.CheckAll(hosts, 3)
	assert.LessOrEqual(t, maxSeen, 3, "concurrent checks should not exceed semaphore limit")
}
```

- [ ] **Step 10: 執行測試確認失敗**

```bash
go test ./internal/hostcheck/... -v -race -run TestCheckAll
```

預期：編譯失敗（`CheckAll` 方法不存在）。

- [ ] **Step 11: 實作 CheckAll**

在 `hostcheck.go` 追加（需加 `"sync"` 到 import）：

```go
// CheckAll 並行檢測多台主機。hosts 為 map[name]address，address 為空表示 local。
// 最多 maxConcurrent 個並行。
func (c *Checker) CheckAll(hosts map[string]string, maxConcurrent int) map[string]Result {
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
```

- [ ] **Step 12: 執行測試確認通過**

```bash
go test ./internal/hostcheck/... -v -race
```

預期：全部 PASS。

- [ ] **Step 9: Commit**

```bash
git add internal/hostcheck/
git commit -m "feat: 新增 hostcheck 模組 — 主機依賴檢測（TDD）"
```

---

### Task 2: pending TTL — pendingAttach 加 SetAt 欄位

**Files:**
- Modify: `internal/daemon/hub.go:59-63` (pendingAttach struct)
- Modify: `internal/daemon/hub.go:237-255` (SetPendingAttach, TakePendingAttach)
- Modify: `internal/daemon/hub_test.go` (新增 TTL 測試)

- [ ] **Step 1: 寫 TTL 過期測試**

在 `internal/daemon/hub_test.go` 追加：

```go
func TestHubManager_PendingAttach_TTL_Expired(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)
	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	err := mgr.SetPendingAttach("mlab", "dev")
	require.NoError(t, err)

	// 手動設定 SetAt 為 61 秒前
	mgr.mu.Lock()
	mgr.pending.SetAt = time.Now().Add(-61 * time.Second)
	mgr.mu.Unlock()

	got := mgr.TakePendingAttach()
	assert.Nil(t, got, "expired pending should be discarded")
}

func TestHubManager_PendingAttach_TTL_Fresh(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)
	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	err := mgr.SetPendingAttach("mlab", "dev")
	require.NoError(t, err)

	got := mgr.TakePendingAttach()
	require.NotNil(t, got, "fresh pending should be returned")
	assert.Equal(t, "mlab", got.HostID)
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_PendingAttach_TTL
```

預期：編譯失敗（`pendingAttach` 沒有 `SetAt` 欄位）。

- [ ] **Step 3: 實作 pending TTL**

修改 `internal/daemon/hub.go`：

pendingAttach struct 加 SetAt：
```go
type pendingAttach struct {
	HostID      string
	SessionName string
	SetAt       time.Time
}
```

`SetPendingAttach` 加 `SetAt: time.Now()`：
```go
func (m *HubManager) SetPendingAttach(hostID, sessionName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.hosts[hostID]; !ok {
		return fmt.Errorf("unknown host: %s", hostID)
	}
	m.pending = &pendingAttach{HostID: hostID, SessionName: sessionName, SetAt: time.Now()}
	return nil
}
```

`TakePendingAttach` 加 TTL 檢查：
```go
const pendingTTL = 60 * time.Second

func (m *HubManager) TakePendingAttach() *pendingAttach {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pending
	m.pending = nil
	if p != nil && time.Since(p.SetAt) > pendingTTL {
		return nil // 過期丟棄
	}
	return p
}
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_PendingAttach
```

預期：全部 PASS（含原有的 SetAndTake、LastWriteWins、UnknownHost、新的 TTL 測試）。

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/hub.go internal/daemon/hub_test.go
git commit -m "feat: pending attach 加 TTL — 60 秒過期自動丟棄"
```

---

### Task 3: CancelPendingAttach — proto + daemon + client

**Files:**
- Modify: `api/proto/tsm/v1/tsm.proto`
- Regenerate: `api/tsm/v1/tsm.pb.go`, `api/tsm/v1/tsm_grpc.pb.go`
- Modify: `internal/daemon/hub.go` (CancelPendingAttach method)
- Modify: `internal/daemon/hub_test.go` (Cancel tests)
- Modify: `internal/daemon/service.go` (CancelPendingAttach handler)
- Modify: `internal/client/client.go` (CancelPendingAttach method)
- Modify: `internal/client/client_test.go` (Cancel client test)

- [ ] **Step 1: 寫 HubManager.CancelPendingAttach 的失敗測試**

在 `internal/daemon/hub_test.go` 追加：

```go
func TestHubManager_CancelPendingAttach(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)
	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	// 設定後取消
	_ = mgr.SetPendingAttach("mlab", "dev")
	mgr.CancelPendingAttach()
	got := mgr.TakePendingAttach()
	assert.Nil(t, got, "cancelled pending should be nil")
}

func TestHubManager_CancelPendingAttach_NoPanic(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)
	// 無 pending 時呼叫 Cancel 不應 panic
	mgr.CancelPendingAttach()
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_CancelPendingAttach
```

預期：編譯失敗（方法不存在）。

- [ ] **Step 3: 實作 HubManager.CancelPendingAttach**

在 `internal/daemon/hub.go` 的 `TakePendingAttach` 方法後追加：

```go
// CancelPendingAttach 清除 pending attach（popup 異常關閉時呼叫）。
func (m *HubManager) CancelPendingAttach() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending = nil
}
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_CancelPendingAttach
```

預期：PASS。

- [ ] **Step 5: 新增 proto 定義**

在 `api/proto/tsm/v1/tsm.proto` 的 message 區塊（`TakePendingAttachResponse` 後面）加入：

```protobuf
// CancelPendingAttachRequest 取消尚未消費的 pending attach。
message CancelPendingAttachRequest {}

// CancelPendingAttachResponse 回傳取消結果。
message CancelPendingAttachResponse {}
```

在 service 區塊的 `TakePendingAttach` RPC 後面加入：

```protobuf
  // CancelPendingAttach 取消尚未消費的 pending attach（popup 異常關閉時呼叫）。
  rpc CancelPendingAttach(CancelPendingAttachRequest) returns (CancelPendingAttachResponse);
```

- [ ] **Step 6: 重新生成 proto**

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       api/proto/tsm/v1/tsm.proto
```

- [ ] **Step 7: 實作 daemon service handler**

在 `internal/daemon/service.go` 的 `TakePendingAttach` handler 後追加：

```go
// CancelPendingAttach 取消尚未消費的 pending attach。
func (s *Service) CancelPendingAttach(
	ctx context.Context, req *tsmv1.CancelPendingAttachRequest,
) (*tsmv1.CancelPendingAttachResponse, error) {
	if s.hubMgr != nil {
		s.hubMgr.CancelPendingAttach()
	}
	return &tsmv1.CancelPendingAttachResponse{}, nil
}
```

- [ ] **Step 8: 實作 client 方法**

在 `internal/client/client.go` 的 `TakePendingAttach` 方法後追加：

```go
// CancelPendingAttach 取消尚未消費的 pending attach。
func (c *Client) CancelPendingAttach(ctx context.Context) error {
	_, err := c.rpc.CancelPendingAttach(ctx, &tsmv1.CancelPendingAttachRequest{})
	return err
}
```

- [ ] **Step 9: 寫 client 端的測試**

在 `internal/client/client_test.go` 追加：

```go
func TestCancelPendingAttach(t *testing.T) {
	exec := &fakeExecutor{}
	c, cleanup := setupTestClient(t, exec)
	defer cleanup()

	// Cancel 在無 hub mode（nil hubMgr）時不應報錯
	err := c.CancelPendingAttach(context.Background())
	assert.NoError(t, err)
}
```

注意：`setupTestClient` 建立的 Service 使用 nil hubMgr，此測試僅驗證「非 hub mode 時的 graceful no-op」路徑。實際的 cancel 邏輯由 `hub_test.go` 中的 `TestHubManager_CancelPendingAttach` 覆蓋。

- [ ] **Step 10: 執行全部測試確認通過**

```bash
go test ./internal/daemon/... ./internal/client/... -v -race
```

預期：全部 PASS。

- [ ] **Step 11: Commit**

```bash
git add api/ internal/daemon/hub.go internal/daemon/hub_test.go internal/daemon/service.go internal/client/client.go internal/client/client_test.go
git commit -m "feat: 新增 CancelPendingAttach RPC — popup 異常關閉時清除 pending"
```

---

## Chunk 2: [i] 面板持久化 + popup 跨主機路徑 + handlePendingAttach

### Task 4: [i] 面板持久化 @tsm_hub_socket

**Files:**
- Modify: `internal/ui/info.go:94-118` (infoConnect)
- Modify: `internal/ui/app.go` (Deps 增加 TmuxExecFn)
- Modify: `internal/ui/info_test.go` (持久化測試)

- [ ] **Step 1: 寫持久化的失敗測試**

在 `internal/ui/info_test.go` 追加：

```go
func TestInfoMode_PersistsHubSocket(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	ln, err := createTestSocket(sockPath)
	if err != nil {
		t.Skip("cannot create test socket:", err)
	}
	defer ln.Close()

	var tmuxCalls [][]string
	deps := ui.Deps{
		Cfg: func() config.Config {
			c := config.Default()
			c.InTmux = true
			return c
		}(),
		HubSocket: sockPath,
		TmuxExecFn: func(args ...string) (string, error) {
			tmuxCalls = append(tmuxCalls, args)
			return "", nil
		},
	}
	m := ui.NewModel(deps)

	m, _ = infoApplyKey(m, "i")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// 應該有一次 set-option 呼叫
	found := false
	for _, call := range tmuxCalls {
		if len(call) >= 4 && call[0] == "set-option" && call[1] == "-g" && call[2] == "@tsm_hub_socket" {
			found = true
			assert.Equal(t, sockPath, call[3])
		}
	}
	assert.True(t, found, "should persist hub socket to tmux option")
}

func TestInfoMode_NoPersistOutsideTmux(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	ln, err := createTestSocket(sockPath)
	if err != nil {
		t.Skip("cannot create test socket:", err)
	}
	defer ln.Close()

	var tmuxCalls [][]string
	deps := ui.Deps{
		Cfg: func() config.Config {
			c := config.Default()
			c.InTmux = false // 不在 tmux 內
			return c
		}(),
		HubSocket: sockPath,
		TmuxExecFn: func(args ...string) (string, error) {
			tmuxCalls = append(tmuxCalls, args)
			return "", nil
		},
	}
	m := ui.NewModel(deps)

	m, _ = infoApplyKey(m, "i")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Empty(t, tmuxCalls, "should not call tmux outside tmux")
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/ui/... -v -race -run TestInfoMode_Persist
```

預期：編譯失敗（`Deps` 沒有 `TmuxExecFn` 欄位）。

- [ ] **Step 3: 在 Deps 加入 TmuxExecFn**

在 `internal/ui/app.go` 的 `Deps` struct 追加：

```go
TmuxExecFn func(args ...string) (string, error) // 可注入的 tmux 執行函式（測試用）
```

- [ ] **Step 4: 修改 infoConnect 加入持久化邏輯**

修改 `internal/ui/info.go` 的 `infoConnect` 方法，在 `m.hubReconnectSock = sockPath` 之前插入：

```go
// 持久化到 tmux 選項，讓下次 Ctrl+Q 自動走 hub-socket 模式
if m.deps.Cfg.InTmux {
	execFn := m.deps.TmuxExecFn
	if execFn == nil {
		exec := tmux.NewRealExecutor()
		execFn = exec.Execute
	}
	execFn("set-option", "-g", "@tsm_hub_socket", sockPath)
}
```

移除舊的註解（`// 只影響當前 session 的重連，不設定 @tsm_hub_socket`）。

- [ ] **Step 5: 執行測試確認通過**

```bash
go test ./internal/ui/... -v -race -run TestInfoMode
```

預期：全部 PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/ui/app.go internal/ui/info.go internal/ui/info_test.go
git commit -m "fix: [i] 面板持久化 hub socket 到 @tsm_hub_socket（Bug 2）"
```

---

### Task 5: popup 跨主機路徑 — RequestAttach + detach 取代 new-window

**Files:**
- Modify: `cmd/tsm/main.go:1145-1153` (popup remote attach 分支)
- Modify: `cmd/tsm/main.go:1131-1138` (popup local 分支加 pending check)

這個 task 修改 `runHubTUI` 核心邏輯。由於 `runHubTUI` 深度依賴 tmux、SSH、daemon 等外部系統，不適合寫純粹的單元測試。改動以程式碼審查 + 現有測試不 break 為驗證方式。

- [ ] **Step 1: 確認現有測試全部通過（基準線）**

```bash
go test ./... -v -race 2>&1 | tail -5
```

- [ ] **Step 2: 修改 popup remote 路徑**

在 `cmd/tsm/main.go` 找到 `runHubTUI` 中的 popup remote 分支（約 L1145-1153），將：

```go
if cfg.InPopup {
	applyHostBar()
	cmd := remote.AttachShellCommand(hostEntry.Address, selected)
	if err := osexec.Command("tmux", "new-window", "-n", selected, cmd).Run(); err != nil {
		_ = tmux.ApplyStatusBar(exec, cfg.Local)
	}
	return hubResultExit
}
```

改為：

```go
if cfg.InPopup {
	// 跨主機切換：委託外層 tsm for loop 處理，避免 tmux 嵌套
	if err := c.RequestAttach(context.Background(), item.HostID, selected); err != nil {
		fmt.Fprintf(os.Stderr, "request attach: %v\n", err)
		return hubResultExit // popup 模式下顯示錯誤後直接退出
	}
	pendingRequested = true
	_ = osexec.Command("tmux", "detach-client").Run()
	return hubResultExit
}
```

- [ ] **Step 3: 在 for loop 開頭加 pendingRequested 旗標和 defer cleanup**

在 `runHubTUI` 的 `for {` 之後（TUI 建立之前）加入：

```go
pendingRequested := false
```

在 `runHubTUI` 函式的 `defer func() { hubCancel(); c.Close() }()` 中加入 pending cleanup：

```go
defer func() {
	hubCancel()
	c.Close()
}()
```

改為：

```go
defer func() {
	if pendingRequested {
		_ = c.CancelPendingAttach(context.Background())
	}
	hubCancel()
	c.Close()
}()
```

注意：`pendingRequested` 在 `RequestAttach` 成功後設為 true，在 `tmux detach-client` + `return hubResultExit` 路徑上，defer 會觸發 `CancelPendingAttach`。但此時外層 tsm 已透過 `TakePendingAttach` 消費了 pending，`CancelPendingAttach` 對已消費的 nil pending 是 no-op。如果 detach 失敗導致外層未消費，`CancelPendingAttach` 正確清除殘留 pending。

spoke 模式（`hctx != nil`）的跨主機路徑也需要同樣的 `pendingRequested = true`（在既有的 `c.RequestAttach` 成功後）。

- [ ] **Step 4: 執行測試確認不 break**

```bash
go test ./... -race 2>&1 | tail -5
```

預期：全部 PASS。

- [ ] **Step 5: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat: popup 跨主機切換改用 RequestAttach+detach — 消除 tmux 嵌套"
```

---

### Task 6: takePendingTarget helper + local attach 後的 pending 檢查

**Files:**
- Modify: `cmd/tsm/main.go` (提取 helper、加檢查點)

Helper 設計原則：helper 只負責「讀取 + 驗證 pending」，回傳目標資訊。複雜的 remote attach（含 reconnect、c/hubCtx/hubCancel 更新）保留在呼叫端，因為 helper 無法存取這些 for loop 局部變數。

- [ ] **Step 1: 提取 takePendingTarget helper**

在 `cmd/tsm/main.go` 中（`findHostEntry` 函式附近）加入：

```go
// pendingTarget 記錄已驗證的 pending attach 目標。
type pendingTarget struct {
	HostID      string
	SessionName string
	Host        *config.HostEntry
}

// takePendingTarget 讀取並驗證 pending attach。回傳 nil 表示無有效 pending。
func takePendingTarget(c *client.Client, cfg config.Config) *pendingTarget {
	pHostID, pSession, err := c.TakePendingAttach(context.Background())
	if err != nil || pHostID == "" {
		return nil
	}
	pHost := findHostEntry(cfg.Hosts, pHostID)
	if pHost == nil {
		return nil // 主機不存在或已停用，忽略
	}
	return &pendingTarget{HostID: pHostID, SessionName: pSession, Host: pHost}
}
```

- [ ] **Step 2: 在 local attach 返回後加 pending 檢查**

在 `runHubTUI` 的非 popup、非 hub-socket local 分支（`switchToSession` 後、`continue` 前）加入：

```go
if hostEntry == nil || hostEntry.IsLocal() {
	switchToSession(selected, fm.ReadOnly())
	if cfg.InPopup {
		return hubResultExit
	}
	// 檢查 popup 是否發了跨主機切換請求
	if p := takePendingTarget(c, cfg); p != nil {
		if p.Host.IsLocal() {
			switchToSession(p.SessionName, false)
		} else {
			_ = tmux.ApplyStatusBar(exec, p.Host.ToColorConfig())
			pResult := remote.Attach(p.Host.Address, p.SessionName)
			_ = tmux.ApplyStatusBar(exec, cfg.Local)
			if pResult == remote.AttachDisconnected {
				newC, newCtx, newCancel := hubReconnectLoop(cfg, p.Host, p.SessionName, hubSocket, func() {
					_ = tmux.ApplyStatusBar(exec, p.Host.ToColorConfig())
				}, exec, cfg.Local)
				if newC != nil {
					hubCancel()
					c.Close()
					c = newC
					hubCtx = newCtx
					hubCancel = newCancel
				}
			}
		}
	}
	continue
}
```

- [ ] **Step 3: 將現有 remote.Attach 後的 TakePendingAttach 區塊用 takePendingTarget 簡化**

在 `runHubTUI` 中找到 `result == remote.AttachDetached` 後的 `TakePendingAttach` 區塊（約 L1160-1203），用 `takePendingTarget` 替換 `c.TakePendingAttach` 呼叫和 `findHostEntry` 驗證邏輯。保留完整的 remote attach + reconnect + c/hubCtx/hubCancel 更新邏輯不變，只替換讀取和驗證部分。

對 `hubReconnectLoop` 成功後的同類區塊做同樣替換（約 L1215-1252）。

**注意**：popup 相關路徑（`if cfg.InPopup { return hubResultExit }`）在這些區塊中可以移除，因為 popup 模式下遠端 attach 已改為 `RequestAttach + detach`（Task 5），不會走到這裡。在程式碼中加註解說明移除原因。

- [ ] **Step 4: 執行測試確認不 break**

```bash
go test ./... -race 2>&1 | tail -5
```

預期：全部 PASS。

- [ ] **Step 5: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "refactor: 提取 handlePendingAttach helper、local attach 後加 pending 檢查"
```

---

### Task 7: 版本推進與 CHANGELOG

**Files:**
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: 推進版本號**

```bash
echo "0.45.0" > VERSION
```

（新功能 → MINOR 推進）

- [ ] **Step 2: 更新 CHANGELOG**

在 `CHANGELOG.md` 最上方加入新版本的變更紀錄，包含：
- 消除 tmux 嵌套：跨主機切換改用 RequestAttach + detach
- 新增 hostcheck 模組
- 新增 CancelPendingAttach RPC
- pending attach TTL（60 秒過期）
- [i] 面板持久化 @tsm_hub_socket（Bug 2 修復）
- 提取 handlePendingAttach helper 減少 runHubTUI 重複

- [ ] **Step 3: 確認建置成功**

```bash
make build && make test
```

- [ ] **Step 4: Commit**

```bash
git add VERSION CHANGELOG.md
git commit -m "chore: 版本推進至 0.45.0、更新 CHANGELOG"
```
