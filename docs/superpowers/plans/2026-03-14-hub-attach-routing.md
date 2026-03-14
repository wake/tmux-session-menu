# Hub-Socket Attach Routing Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** All cross-host session attach in hub-socket mode is routed through the hub, eliminating SSH from spoke to other hosts.

**Architecture:** Spoke sends `RequestAttach` RPC to hub daemon via reverse tunnel, then detaches. Hub's `runHubTUI` consumes the pending attach after `remote.Attach` returns, routing to local `switchToSession` or `remote.Attach` as appropriate. Removes all `@tsm_hub_host` / `detectLocalAddr` / `resolveHubHost` machinery.

**Tech Stack:** Go 1.24+, Protocol Buffers, gRPC, Bubble Tea TUI

---

## Chunk 1: Proto + Daemon + Client

### Task 1: Proto — 新增 RequestAttach + TakePendingAttach

**Files:**
- Modify: `api/proto/tsm/v1/tsm.proto`
- Regenerate: `api/proto/tsm/v1/tsm.pb.go`, `api/proto/tsm/v1/tsm_grpc.pb.go`
- Copy: `api/tsm/v1/tsm.pb.go`, `api/tsm/v1/tsm_grpc.pb.go`

- [ ] **Step 1: 新增 message 定義**

在 `api/proto/tsm/v1/tsm.proto` 的 `ProxyMutationResponse`（行 96）後面加入：

```protobuf
// RequestAttachRequest 由 spoke 端發送，請求 hub 切換到指定主機的 session。
message RequestAttachRequest {
  string host_id = 1;       // 目標主機 ID
  string session_name = 2;  // 目標 session 名稱
}

// RequestAttachResponse 回傳 RequestAttach 操作結果。
message RequestAttachResponse {
  bool success = 1;
}

// TakePendingAttachRequest 由 hub 端呼叫，取得並清除 pending attach。
message TakePendingAttachRequest {}

// TakePendingAttachResponse 回傳 pending attach 資訊。
// host_id 為空字串表示無 pending。
message TakePendingAttachResponse {
  string host_id = 1;
  string session_name = 2;
}
```

- [ ] **Step 2: 新增 RPC 定義**

在 `SessionManager` service（行 322 `ProxyMutation` 後面）加入：

```protobuf
  // RequestAttach 由 spoke 發送，請求 hub 切換到指定主機的 session（hub-socket 模式專用）。
  rpc RequestAttach(RequestAttachRequest) returns (RequestAttachResponse);
  // TakePendingAttach 由 hub 呼叫，取得並清除 pending attach（read-and-clear 語意）。
  rpc TakePendingAttach(TakePendingAttachRequest) returns (TakePendingAttachResponse);
```

- [ ] **Step 3: 重新生成 proto 碼**

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       api/proto/tsm/v1/tsm.proto
cp api/proto/tsm/v1/tsm.pb.go api/tsm/v1/tsm.pb.go
cp api/proto/tsm/v1/tsm_grpc.pb.go api/tsm/v1/tsm_grpc.pb.go
```

- [ ] **Step 4: 驗證編譯**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add api/
git commit -m "feat(proto): 新增 RequestAttach + TakePendingAttach RPC"
```

---

### Task 2: Daemon — HubManager pending attach 機制

**Files:**
- Modify: `internal/daemon/hub.go`
- Test: `internal/daemon/hub_test.go`

- [ ] **Step 1: 寫測試**

在 `internal/daemon/hub_test.go` 末尾加入：

```go
func TestHubManager_PendingAttach_SetAndTake(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)
	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	// 初始無 pending
	got := mgr.TakePendingAttach()
	assert.Nil(t, got)

	// 設定 pending
	err := mgr.SetPendingAttach("mlab", "dev")
	require.NoError(t, err)

	// 取得並清除
	got = mgr.TakePendingAttach()
	require.NotNil(t, got)
	assert.Equal(t, "mlab", got.HostID)
	assert.Equal(t, "dev", got.SessionName)

	// 再取應為 nil（已清除）
	got = mgr.TakePendingAttach()
	assert.Nil(t, got)
}

func TestHubManager_PendingAttach_LastWriteWins(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)
	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "mlab2", Address: "mlab2", Enabled: true})

	_ = mgr.SetPendingAttach("mlab", "dev")
	_ = mgr.SetPendingAttach("mlab2", "work")

	got := mgr.TakePendingAttach()
	require.NotNil(t, got)
	assert.Equal(t, "mlab2", got.HostID)
	assert.Equal(t, "work", got.SessionName)
}

func TestHubManager_PendingAttach_UnknownHost(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	err := mgr.SetPendingAttach("nonexistent", "dev")
	assert.Error(t, err)
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_PendingAttach
```

- [ ] **Step 3: 實作**

在 `internal/daemon/hub.go` 加入：

`pendingAttach` 結構（在 `hubHost` 結構後面，約行 67）：

```go
// pendingAttach 記錄 spoke 請求的跨主機 session 切換目標。
type pendingAttach struct {
	HostID      string
	SessionName string
}
```

`HubManager` 結構新增欄位（行 53 `remoteWg` 後面）：

```go
	pending *pendingAttach // 受 mu 保護
```

新增方法（在 `ProxyMutation` 方法後面）：

```go
// SetPendingAttach 設定 pending attach 目標（後寫覆蓋）。
// hostID 必須存在於已註冊的 hosts 中。
func (m *HubManager) SetPendingAttach(hostID, sessionName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.hosts[hostID]; !ok {
		return fmt.Errorf("unknown host: %s", hostID)
	}
	m.pending = &pendingAttach{HostID: hostID, SessionName: sessionName}
	return nil
}

// TakePendingAttach 取得並清除 pending attach（read-and-clear）。
// 無 pending 時回傳 nil。
func (m *HubManager) TakePendingAttach() *pendingAttach {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pending
	m.pending = nil
	return p
}
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/daemon/... -v -race -run TestHubManager_PendingAttach
```

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/hub.go internal/daemon/hub_test.go
git commit -m "feat(daemon): HubManager pending attach 機制"
```

---

### Task 3: Daemon — Service RPC handlers

**Files:**
- Modify: `internal/daemon/service.go`
- Test: `internal/daemon/service_test.go`

- [ ] **Step 1: 寫測試**

在 `internal/daemon/service_test.go` 的現有 service test 後面加入（參考現有 test 模式，使用 `fakeExecutor`）：

```go
func TestService_RequestAttach(t *testing.T) {
	svc, cleanup := setupHubService(t)
	defer cleanup()

	// 成功
	resp, err := svc.RequestAttach(context.Background(), &tsmv1.RequestAttachRequest{
		HostId: "local", SessionName: "dev",
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// 驗證 pending 已設定
	tResp, err := svc.TakePendingAttach(context.Background(), &tsmv1.TakePendingAttachRequest{})
	require.NoError(t, err)
	assert.Equal(t, "local", tResp.HostId)
	assert.Equal(t, "dev", tResp.SessionName)

	// 再取應為空
	tResp, err = svc.TakePendingAttach(context.Background(), &tsmv1.TakePendingAttachRequest{})
	require.NoError(t, err)
	assert.Empty(t, tResp.HostId)
}

func TestService_RequestAttach_NotHubMode(t *testing.T) {
	svc := &Service{} // hubMgr == nil
	_, err := svc.RequestAttach(context.Background(), &tsmv1.RequestAttachRequest{
		HostId: "mlab", SessionName: "dev",
	})
	assert.Error(t, err)
}

func TestService_RequestAttach_UnknownHost(t *testing.T) {
	svc, cleanup := setupHubService(t)
	defer cleanup()

	_, err := svc.RequestAttach(context.Background(), &tsmv1.RequestAttachRequest{
		HostId: "nonexistent", SessionName: "dev",
	})
	assert.Error(t, err)
}
```

注意：`setupHubService` 可能需要建立。若現有測試已有建立含 `hubMgr` 的 Service 的 helper，則複用。否則在測試檔中新增：

```go
func setupHubService(t *testing.T) (*Service, func()) {
	t.Helper()
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)
	mgr.AddHost(config.HostEntry{Name: "local", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	svc := &Service{hubMgr: mgr, mhub: mhub}
	return svc, func() {}
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/daemon/... -v -race -run TestService_RequestAttach
```

- [ ] **Step 3: 實作**

在 `internal/daemon/service.go` 的 `ProxyMutation` 方法（行 306）後面加入：

```go
// RequestAttach 接收 spoke 端的 attach 請求，暫存到 HubManager（hub 模式專用）。
func (s *Service) RequestAttach(
	ctx context.Context, req *tsmv1.RequestAttachRequest,
) (*tsmv1.RequestAttachResponse, error) {
	if s.hubMgr == nil {
		return nil, status.Error(codes.Unavailable, "not in hub mode")
	}
	if err := s.hubMgr.SetPendingAttach(req.HostId, req.SessionName); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &tsmv1.RequestAttachResponse{Success: true}, nil
}

// TakePendingAttach 取得並清除 pending attach（hub 模式專用）。
func (s *Service) TakePendingAttach(
	ctx context.Context, req *tsmv1.TakePendingAttachRequest,
) (*tsmv1.TakePendingAttachResponse, error) {
	if s.hubMgr == nil {
		return &tsmv1.TakePendingAttachResponse{}, nil
	}
	p := s.hubMgr.TakePendingAttach()
	if p == nil {
		return &tsmv1.TakePendingAttachResponse{}, nil
	}
	return &tsmv1.TakePendingAttachResponse{
		HostId:      p.HostID,
		SessionName: p.SessionName,
	}, nil
}
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/daemon/... -v -race -run TestService_RequestAttach
```

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/service.go internal/daemon/service_test.go
git commit -m "feat(daemon): RequestAttach + TakePendingAttach RPC handlers"
```

---

### Task 4: Client — RequestAttach + TakePendingAttach

**Files:**
- Modify: `internal/client/client.go`

- [ ] **Step 1: 實作**

在 `internal/client/client.go` 的 `ProxyMutation` 方法（行 162）後面加入：

```go
// RequestAttach 請求 hub 切換到指定主機的 session。
func (c *Client) RequestAttach(ctx context.Context, hostID, sessionName string) error {
	resp, err := c.rpc.RequestAttach(ctx, &tsmv1.RequestAttachRequest{
		HostId:      hostID,
		SessionName: sessionName,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("request attach failed")
	}
	return nil
}

// TakePendingAttach 取得並清除 hub daemon 的 pending attach。
// 回傳空 hostID 表示無 pending。
func (c *Client) TakePendingAttach(ctx context.Context) (hostID, sessionName string, err error) {
	resp, err := c.rpc.TakePendingAttach(ctx, &tsmv1.TakePendingAttachRequest{})
	if err != nil {
		return "", "", err
	}
	return resp.HostId, resp.SessionName, nil
}
```

- [ ] **Step 2: 驗證編譯**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/client/client.go
git commit -m "feat(client): RequestAttach + TakePendingAttach 方法"
```

---

## Chunk 2: Spoke + Hub 路由變更 + 清理

### Task 5: Spoke 端 — RequestAttach + detach 路由

**Files:**
- Modify: `cmd/tsm/main.go` (runHubTUI 的 `hctx != nil` 路徑，約行 1089-1109)

- [ ] **Step 1: 修改 spoke 端路由**

將 `runHubTUI` 中 hub-socket 模式的 session 選取邏輯（行 1089-1109）從：

```go
		// hub-socket 模式：路由由 resolveHubHost 決定，config 只用於顏色合併
		if hctx != nil {
			routed := resolveHubHost(hctx, item)
			if hostEntry != nil {
				routed.Color = hostEntry.Color
				routed.BarBG = hostEntry.BarBG
				routed.BarFG = hostEntry.BarFG
				routed.BadgeBG = hostEntry.BadgeBG
				routed.BadgeFG = hostEntry.BadgeFG
			}
			hostEntry = routed
		}

		if hostEntry == nil || hostEntry.IsLocal() {
			// 本機 session
			switchToSession(selected, fm.ReadOnly())
			if cfg.InPopup {
				return hubResultExit
			}
			continue
		}
```

改為：

```go
		// hub-socket 模式：spoke 只處理自己的 session，其他一律委託 hub
		if hctx != nil {
			if hctx.HubSelf != "" && item.HostID == hctx.HubSelf {
				// spoke 自己的 session → 本機切換
				switchToSession(selected, fm.ReadOnly())
				if cfg.InPopup {
					return hubResultExit
				}
				continue
			}
			// 跨主機 session → 請求 hub 處理，然後 detach 回到 hub
			if err := c.RequestAttach(context.Background(), item.HostID, selected); err != nil {
				fmt.Fprintf(os.Stderr, "request attach: %v\n", err)
				continue
			}
			_ = osexec.Command("tmux", "detach-client").Run()
			return hubResultExit
		}

		if hostEntry == nil || hostEntry.IsLocal() {
			// 本機 session（非 hub-socket 模式）
			switchToSession(selected, fm.ReadOnly())
			if cfg.InPopup {
				return hubResultExit
			}
			continue
		}
```

- [ ] **Step 2: 驗證編譯**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat: spoke 端跨主機 attach 改走 RequestAttach + detach"
```

---

### Task 6: Hub 端 — 消費 pending attach

**Files:**
- Modify: `cmd/tsm/main.go` (runHubTUI 的 AttachDetached 處理，約行 1119-1124；hubReconnectLoop，約行 829-835)

- [ ] **Step 1: 修改 runHubTUI AttachDetached 處理**

將 `runHubTUI` 中（行 1119-1124）：

```go
		if result == remote.AttachDetached {
			if remote.CheckAndClearExitRequested(hostEntry.Address) {
				return hubResultExit
			}
			continue
		}
```

改為：

```go
		if result == remote.AttachDetached {
			if remote.CheckAndClearExitRequested(hostEntry.Address) {
				return hubResultExit
			}
			// 檢查 spoke 是否請求了跨主機切換
			if pHostID, pSession, _ := c.TakePendingAttach(context.Background()); pHostID != "" {
				pHost := findHostEntry(cfg.Hosts, pHostID)
				if pHost == nil || pHost.IsLocal() {
					switchToSession(pSession, false)
					if cfg.InPopup {
						return hubResultExit
					}
				} else {
					// 遠端 spoke → 更新目標，由外層迴圈重新 attach
					hostEntry = pHost
					selected = pSession
					continue // 回到迴圈頂端重新進入 attach 路徑
				}
			}
			continue
		}
```

注意：`continue` 會回到 `for` 迴圈頂端，重新建立 TUI。但如果 pending 是遠端主機，我們不想回到 TUI，而是直接 attach。因此需要用一個 flag 跳過 TUI：

在 `for` 迴圈前宣告：

```go
	var pendingHost *config.HostEntry
	var pendingSession string
```

在 `for` 迴圈開頭（`deps := ui.Deps{` 之前）加入：

```go
		// 有 pending attach 時跳過 TUI，直接進入 attach
		if pendingHost != nil {
			hostEntry := pendingHost
			selected := pendingSession
			pendingHost = nil
			pendingSession = ""
			goto remoteAttach
		}
```

然後將遠端 attach 區塊加上 label：

```go
	remoteAttach:
		// 遠端 session — attach（使用 per-host 四色）
		applyHostBar := func() { ... }
```

**或者更簡潔的做法**：將 AttachDetached 中的遠端 pending 改為直接 re-enter attach（不 goto）：

```go
			if pHostID, pSession, _ := c.TakePendingAttach(context.Background()); pHostID != "" {
				pHost := findHostEntry(cfg.Hosts, pHostID)
				if pHost == nil || pHost.IsLocal() {
					switchToSession(pSession, false)
					if cfg.InPopup {
						return hubResultExit
					}
					continue
				}
				// 遠端 spoke → 直接 attach
				pApply := func() {
					_ = tmux.ApplyStatusBar(exec, pHost.ToColorConfig())
				}
				pApply()
				pResult := remote.Attach(pHost.Address, pSession)
				_ = tmux.ApplyStatusBar(exec, cfg.Local)
				if pResult == remote.AttachDetached {
					if remote.CheckAndClearExitRequested(pHost.Address) {
						return hubResultExit
					}
				} else {
					newC, newCtx, newCancel := hubReconnectLoop(cfg, pHost, pSession, hubSocket, pApply, exec, cfg.Local)
					if newC != nil {
						hubCancel()
						c.Close()
						c = newC
						hubCtx = newCtx
						hubCancel = newCancel
					}
				}
				continue
			}
```

這樣比 goto 更清楚。但注意 pending attach 後的 remote.Attach 返回 detached 時不再遞迴檢查 pending（一次只消費一筆，下次 detach 後才再檢查）。

- [ ] **Step 2: 修改 hubReconnectLoop**

在 `hubReconnectLoop`（行 829-835）中，`reResult == remote.AttachDetached` 的處理：

```go
		if reResult == remote.AttachDetached {
			if remote.CheckAndClearExitRequested(hostEntry.Address) {
				cancel()
				newC.Close()
				return nil, nil, nil
			}
			return newC, ctx, cancel
		}
```

`hubReconnectLoop` 只能回傳 `*client.Client`，無法回傳 pending 資訊。因此在外層 `runHubTUI` 收到 `hubReconnectLoop` 回傳的 newC 後，inline 相同的 pending 檢查邏輯：

```go
		newC, newCtx, newCancel := hubReconnectLoop(cfg, hostEntry, selected, hubSocket, applyHostBar, exec, cfg.Local)
		if newC != nil {
			hubCancel()
			c.Close()
			c = newC
			hubCtx = newCtx
			hubCancel = newCancel
			// reconnect 成功後檢查 pending attach
			if pHostID, pSession, _ := c.TakePendingAttach(context.Background()); pHostID != "" {
				pHost := findHostEntry(cfg.Hosts, pHostID)
				if pHost == nil || pHost.IsLocal() {
					switchToSession(pSession, false)
					if cfg.InPopup {
						return hubResultExit
					}
					continue
				}
				// 遠端 spoke → 直接 attach
				pApply := func() {
					_ = tmux.ApplyStatusBar(exec, pHost.ToColorConfig())
				}
				pApply()
				pResult := remote.Attach(pHost.Address, pSession)
				_ = tmux.ApplyStatusBar(exec, cfg.Local)
				if pResult == remote.AttachDetached {
					if remote.CheckAndClearExitRequested(pHost.Address) {
						return hubResultExit
					}
				} else {
					newC2, newCtx2, newCancel2 := hubReconnectLoop(cfg, pHost, pSession, hubSocket, pApply, exec, cfg.Local)
					if newC2 != nil {
						hubCancel()
						c.Close()
						c = newC2
						hubCtx = newCtx2
						hubCancel = newCancel2
					}
				}
			}
		}
		continue
```

- [ ] **Step 3: 驗證編譯**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat: hub 端消費 pending attach — 跨主機路由統一由 hub 執行"
```

---

### Task 7: 清理 — 移除 v0.41.1 遺留程式碼

**Files:**
- Modify: `cmd/tsm/main.go`
- Modify: `cmd/tsm/main_test.go`
- Modify: `internal/remote/attach.go`

- [ ] **Step 1: 移除 cmd/tsm/main.go 中的程式碼**

1. 移除 `"net"` import（行 6）
2. 移除 `resolveSSHHostname()` 函式（行 1794-1807）
3. 移除 `detectLocalAddr()` 函式（行 1809-1831）
4. 移除 `resolveHubHost()` 函式（行 1140-1158）
5. 簡化 `hubContext` 結構（行 943-948）：移除 `HubHost` 欄位

```go
type hubContext struct {
	HubSelf string // spoke 在 hub 中的 host ID
}
```

6. 簡化 `readHubContext()`（行 117-126）：只讀 `@tsm_hub_self`

```go
func readHubContext() *hubContext {
	hubSelf := strings.TrimSpace(string(mustOutput(osexec.Command("tmux", "show-option", "-gqv", "@tsm_hub_self"))))
	if hubSelf == "" {
		return nil
	}
	return &hubContext{HubSelf: hubSelf}
}
```

7. 簡化 `makeHubDialFn()`：移除 `hostname`、`detectLocalAddr`、`SetHubHost`、所有 `ClearHubHost` 呼叫。清理後的完整函式：

```go
func makeHubDialFn(hubSocketPath string, hosts []config.HostEntry) daemon.HubDialFn {
	nameByAddr := make(map[string]string)
	for _, h := range hosts {
		if h.Address != "" {
			nameByAddr[h.Address] = h.Name
		}
	}

	return func(ctx context.Context, address string) (daemon.HubRemoteClient, func(), error) {
		tun := remote.NewTunnel(address, remote.WithReverse(hubSocketPath))
		if err := tun.Start(); err != nil {
			return nil, nil, err
		}
		reverseSock := remote.ReverseSocketPath(address)
		if err := remote.SetHubSocket(address, reverseSock); err != nil {
			log.Printf("warn: set hub socket on %s failed: %v", address, err)
		}
		if selfName, ok := nameByAddr[address]; ok {
			if err := remote.SetHubSelf(address, selfName); err != nil {
				log.Printf("warn: set hub self on %s failed: %v", address, err)
			}
		}

		c, err := client.DialSocketCtx(ctx, tun.LocalSocket())
		if err != nil {
			_ = remote.ClearHubSocket(address)
			_ = remote.ClearHubSelf(address)
			tun.Close()
			return nil, nil, err
		}
		if err := c.Watch(ctx); err != nil {
			c.Close()
			_ = remote.ClearHubSocket(address)
			_ = remote.ClearHubSelf(address)
			tun.Close()
			return nil, nil, err
		}
		cleanup := func() {
			c.Close()
			_ = remote.ClearHubSocket(address)
			_ = remote.ClearHubSelf(address)
			tun.Close()
		}
		return c, cleanup, nil
	}
}
```

- [ ] **Step 2: 移除 cmd/tsm/main_test.go 中的測試**

移除 `TestDetectLocalAddr_Loopback`、`TestDetectLocalAddr_StripUser`、`TestDetectLocalAddr_Unreachable`（行 87-105）。

移除 `"net"` import。

- [ ] **Step 3: 移除 internal/remote/attach.go 中的函式**

移除 `SetHubHost()` 和 `ClearHubHost()`（行 83-93）。

- [ ] **Step 4: 執行全部測試**

```bash
make test
```

- [ ] **Step 5: Commit**

```bash
git add cmd/tsm/main.go cmd/tsm/main_test.go internal/remote/attach.go
git commit -m "refactor: 移除 detectLocalAddr / resolveHubHost / @tsm_hub_host 相關程式碼"
```

---

### Task 8: 版本推進 + 全部測試

**Files:**
- Modify: `VERSION`

- [ ] **Step 1: 版本推進**

`VERSION` 改為 `0.42.0`（新功能：attach 路由機制變更）

- [ ] **Step 2: 全部測試**

```bash
make test
make build
```

- [ ] **Step 3: Commit**

```bash
git add VERSION
git commit -m "chore: 版本推進至 0.42.0"
```

---

## 關鍵檔案

| 檔案 | 修改內容 |
|------|---------|
| `api/proto/tsm/v1/tsm.proto` | 新增 RequestAttach + TakePendingAttach |
| `api/proto/tsm/v1/*.pb.go` | 重新生成 |
| `api/tsm/v1/*.pb.go` | 同步生成 |
| `internal/daemon/hub.go` | pendingAttach struct + Set/Take 方法 |
| `internal/daemon/hub_test.go` | pending attach 測試 |
| `internal/daemon/service.go` | RequestAttach + TakePendingAttach handlers |
| `internal/daemon/service_test.go` | RPC handler 測試 |
| `internal/client/client.go` | RequestAttach + TakePendingAttach 方法 |
| `cmd/tsm/main.go` | spoke 路由、hub 消費、清理 |
| `cmd/tsm/main_test.go` | 移除 detectLocalAddr 測試 |
| `internal/remote/attach.go` | 移除 SetHubHost/ClearHubHost |
| `VERSION` | 0.41.1 → 0.42.0 |

## 驗證

1. `make test` 全部通過
2. `make build` 成功
3. 手動測試：mac-air → `tsm --host` → mlab → attach → Ctrl+Q → 選擇 hub local session → 應退回 mac-air 並切換
4. 手動測試：選擇其他 spoke session → 應退回 mac-air 並 SSH 到目標 spoke
