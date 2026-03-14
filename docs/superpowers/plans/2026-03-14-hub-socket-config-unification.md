# Hub-Socket Config Unification Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hub-socket 模式下 `[[hosts]]` 設定讀寫透過 gRPC RPC 走 hub daemon，不再使用 spoke 本地 config。

**Architecture:** 新增 `HostConfigEntry` proto message 與 `GetHostsConfig` / `UpdateHostConfig` 兩個 RPC。Hub daemon 讀寫自己的 config 檔。UI 層透過新欄位 `hubHosts` 取代 `deps.Cfg.Hosts`，所有寫入操作改走 RPC。

**Tech Stack:** Go 1.24+, Protocol Buffers (protoc v6.33.4), gRPC, Bubble Tea TUI

**Spec:** `docs/superpowers/specs/2026-03-14-hub-socket-config-unification-design.md`

---

## File Structure

| 檔案 | 職責 | 操作 |
|------|------|------|
| `api/proto/tsm/v1/tsm.proto` | Proto 定義：新增 messages + RPCs | Modify |
| `api/proto/tsm/v1/tsm.pb.go` | Proto 生成碼 | Regenerate |
| `api/proto/tsm/v1/tsm_grpc.pb.go` | gRPC 生成碼 | Regenerate |
| `api/tsm/v1/tsm.pb.go` | Proto 生成碼（複製） | Regenerate |
| `api/tsm/v1/tsm_grpc.pb.go` | gRPC 生成碼（複製） | Regenerate |
| `internal/daemon/service.go` | Daemon RPC handler：GetHostsConfig, UpdateHostConfig | Modify |
| `internal/daemon/service_test.go` | Daemon RPC 測試 | Modify |
| `internal/client/client.go` | Client 方法：GetHostsConfig, UpdateHostConfig | Modify |
| `internal/ui/app.go` | Model struct 新增 hubHosts、Init/Update 讀取路徑、模式判斷 | Modify |
| `internal/ui/hostpicker.go` | Host picker 讀寫路徑改用 hubHosts + RPC | Modify |
| `internal/ui/items.go` | FilterActiveHosts 呼叫端改傳 hubHosts | Modify (間接，呼叫端在 app.go/hostpicker.go) |
| `internal/ui/app_test.go` | UI 測試更新 | Modify |

---

## Chunk 1: Proto + Daemon + Client

### Task 1: Proto 定義 — 新增 HostConfigEntry、RPC、Enum

**Files:**
- Modify: `api/proto/tsm/v1/tsm.proto:91-271`

- [ ] **Step 1: 在 `tsm.proto` 新增 proto 定義**

在 `ProxyMutationResponse` message 之後（第 96 行後）新增：

```protobuf
// HostConfigEntry 描述一台主機的完整設定（對應 config.HostEntry）。
message HostConfigEntry {
  string name = 1;
  string address = 2;      // 唯讀，僅由 hub daemon 端設定
  string color = 3;         // accent color（圓點、badge_bg fallback）
  bool enabled = 4;
  int32 sort_order = 5;
  string bar_bg = 6;
  string bar_fg = 7;
  string badge_bg = 8;
  string badge_fg = 9;
  bool archived = 10;
}

// GetHostsConfigRequest 取得主機設定清單。
message GetHostsConfigRequest {}

// GetHostsConfigResponse 回傳主機設定清單。
message GetHostsConfigResponse {
  repeated HostConfigEntry hosts = 1;
}

// HostConfigAction 列舉主機設定的操作類型。
enum HostConfigAction {
  HOST_CONFIG_ACTION_UNSPECIFIED = 0;  // 預留：daemon 應拒絕此值
  HOST_CONFIG_ENABLE = 1;
  HOST_CONFIG_DISABLE = 2;
  HOST_CONFIG_ARCHIVE = 3;
  HOST_CONFIG_UNARCHIVE = 4;
  HOST_CONFIG_UPDATE_COLORS = 5;
  HOST_CONFIG_REORDER = 6;
}

// UpdateHostConfigRequest 更新主機設定。
message UpdateHostConfigRequest {
  string host_name = 1;
  HostConfigAction action = 2;
  // UPDATE_COLORS 使用：
  string bar_bg = 3;
  string bar_fg = 4;
  string badge_bg = 5;
  string badge_fg = 6;
  string color = 7;
  // REORDER 使用：完整排序清單（所有非封存主機名稱，按期望順序排列）
  repeated string ordered_hosts = 8;
}

// UpdateHostConfigResponse 回傳操作後的完整主機清單。
message UpdateHostConfigResponse {
  repeated HostConfigEntry hosts = 1;
}
```

在 `service SessionManager` 區塊內（`ProxyMutation` RPC 之後）新增：

```protobuf
  // GetHostsConfig 取得 daemon 所在主機的 hosts 設定。
  rpc GetHostsConfig(GetHostsConfigRequest) returns (GetHostsConfigResponse);
  // UpdateHostConfig 更新 daemon 所在主機的 hosts 設定。
  rpc UpdateHostConfig(UpdateHostConfigRequest) returns (UpdateHostConfigResponse);
```

- [ ] **Step 2: 重新生成 proto**

Run:
```bash
cd /Users/wake/Workspace/wake/tmux-session-menu
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       api/proto/tsm/v1/tsm.proto
```

然後同步生成碼：
```bash
cp api/proto/tsm/v1/tsm.pb.go api/tsm/v1/tsm.pb.go
cp api/proto/tsm/v1/tsm_grpc.pb.go api/tsm/v1/tsm_grpc.pb.go
```

- [ ] **Step 3: 驗證編譯通過**

Run: `cd /Users/wake/Workspace/wake/tmux-session-menu && go build ./...`
Expected: 成功（新 RPC 由 `UnimplementedSessionManagerServer` 自動滿足）

- [ ] **Step 4: Commit**

```bash
git add api/
git commit -m "feat: proto — 新增 GetHostsConfig/UpdateHostConfig RPC 及 HostConfigEntry message"
```

---

### Task 2: Daemon — GetHostsConfig RPC handler

**Files:**
- Modify: `internal/daemon/service.go:22-33,306-360`
- Modify: `internal/daemon/service_test.go`

- [ ] **Step 1: 寫測試 — TestGetHostsConfig**

在 `internal/daemon/service_test.go` 新增：

```go
func TestGetHostsConfig(t *testing.T) {
	// 建立 temp config 檔案
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "local", Address: "", Color: "#00ff00", Enabled: true, SortOrder: 0, BarBG: "#111", BarFG: "#222", BadgeBG: "#333", BadgeFG: "#444"},
		{Name: "mlab", Address: "mlab", Color: "#ff0000", Enabled: true, SortOrder: 1, Archived: false},
		{Name: "old", Address: "old", Color: "#888", Enabled: false, SortOrder: 2, Archived: true},
	}
	require.NoError(t, config.SaveConfig(cfgPath, cfg))

	// 設定 env 使 loadServerConfig 讀到此路徑
	t.Setenv("TSM_CONFIG_PATH", cfgPath)

	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	resp, err := client.GetHostsConfig(context.Background(), &tsmv1.GetHostsConfigRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Hosts, 3)

	assert.Equal(t, "local", resp.Hosts[0].Name)
	assert.Equal(t, "", resp.Hosts[0].Address)
	assert.Equal(t, "#00ff00", resp.Hosts[0].Color)
	assert.True(t, resp.Hosts[0].Enabled)
	assert.Equal(t, "#111", resp.Hosts[0].BarBg)
	assert.Equal(t, "#222", resp.Hosts[0].BarFg)
	assert.Equal(t, "#333", resp.Hosts[0].BadgeBg)
	assert.Equal(t, "#444", resp.Hosts[0].BadgeFg)

	assert.Equal(t, "mlab", resp.Hosts[1].Name)
	assert.Equal(t, "mlab", resp.Hosts[1].Address)
	assert.True(t, resp.Hosts[1].Enabled)

	assert.Equal(t, "old", resp.Hosts[2].Name)
	assert.True(t, resp.Hosts[2].Archived)
	assert.False(t, resp.Hosts[2].Enabled)
}
```

- [ ] **Step 2: 確認測試失敗**

Run: `go test ./internal/daemon/... -v -race -run TestGetHostsConfig`
Expected: FAIL（`GetHostsConfig` 尚未實作，回傳 `Unimplemented`）

- [ ] **Step 3: 新增 cfgMu、loadServerConfigPath、轉換函式和 GetHostsConfig**

先在 `Service` struct（第 23-33 行）新增 mutex（Task 3 的 `UpdateHostConfig` 也會用）：

```go
type Service struct {
	tsmv1.UnimplementedSessionManagerServer

	tmuxMgr   *tmux.Manager
	store     *store.Store
	hub       *WatcherHub
	mhub      *MultiHostHub // nil = 非 hub 模式
	hubMgr    *HubManager   // nil = 非 hub 模式
	state     *StateManager
	startedAt time.Time

	cfgMu sync.Mutex // 保護 config 讀寫序列
}
```

新增 `import "sync"` 到 import 區塊。

修改 `loadServerConfig`（第 306-316 行）支援 `TSM_CONFIG_PATH` 環境變數，並新增 `loadServerConfigPath`：

```go
func loadServerConfigPath() string {
	if p := os.Getenv("TSM_CONFIG_PATH"); p != "" {
		return p
	}
	return config.ExpandPath("~/.config/tsm/config.toml")
}

func loadServerConfig() config.Config {
	cfg := config.Default()
	cfgPath := loadServerConfigPath()
	if data, err := os.ReadFile(cfgPath); err == nil {
		if loaded, err := config.LoadFromString(string(data)); err == nil {
			cfg = loaded
		}
	}
	return cfg
}
```

新增轉換函式和 `GetHostsConfig`：

```go
// hostEntryToProto 將 config.HostEntry 轉換為 proto HostConfigEntry。
func hostEntryToProto(h config.HostEntry) *tsmv1.HostConfigEntry {
	return &tsmv1.HostConfigEntry{
		Name:      h.Name,
		Address:   h.Address,
		Color:     h.Color,
		Enabled:   h.Enabled,
		SortOrder: int32(h.SortOrder),
		BarBg:     h.BarBG,
		BarFg:     h.BarFG,
		BadgeBg:   h.BadgeBG,
		BadgeFg:   h.BadgeFG,
		Archived:  h.Archived,
	}
}

// hostEntriesToProto 將 []config.HostEntry 轉換為 []*tsmv1.HostConfigEntry。
func hostEntriesToProto(hosts []config.HostEntry) []*tsmv1.HostConfigEntry {
	result := make([]*tsmv1.HostConfigEntry, len(hosts))
	for i, h := range hosts {
		result[i] = hostEntryToProto(h)
	}
	return result
}

// GetHostsConfig 取得 daemon 所在主機的 hosts 設定。
func (s *Service) GetHostsConfig(_ context.Context, _ *tsmv1.GetHostsConfigRequest) (*tsmv1.GetHostsConfigResponse, error) {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	cfg := loadServerConfig()
	return &tsmv1.GetHostsConfigResponse{
		Hosts: hostEntriesToProto(cfg.Hosts),
	}, nil
}
```

- [ ] **Step 4: 確認測試通過**

Run: `go test ./internal/daemon/... -v -race -run TestGetHostsConfig`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/service.go internal/daemon/service_test.go
git commit -m "feat: daemon — 實作 GetHostsConfig RPC"
```

---

### Task 3: Daemon — UpdateHostConfig RPC handler

**Files:**
- Modify: `internal/daemon/service.go:22-33`
- Modify: `internal/daemon/service_test.go`

- [ ] **Step 1: 寫測試 — UpdateHostConfig 各 action**

在 `internal/daemon/service_test.go` 新增：

```go
func TestUpdateHostConfig_Enable(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: false},
	}
	require.NoError(t, config.SaveConfig(cfgPath, cfg))
	t.Setenv("TSM_CONFIG_PATH", cfgPath)

	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	resp, err := client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
		HostName: "mlab",
		Action:   tsmv1.HostConfigAction_HOST_CONFIG_ENABLE,
	})
	require.NoError(t, err)
	require.Len(t, resp.Hosts, 1)
	assert.True(t, resp.Hosts[0].Enabled)

	// 驗證寫回檔案
	reloaded := loadTestConfig(t, cfgPath)
	assert.True(t, reloaded.Hosts[0].Enabled)
}

func TestUpdateHostConfig_Disable(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: true},
	}
	require.NoError(t, config.SaveConfig(cfgPath, cfg))
	t.Setenv("TSM_CONFIG_PATH", cfgPath)

	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	resp, err := client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
		HostName: "mlab",
		Action:   tsmv1.HostConfigAction_HOST_CONFIG_DISABLE,
	})
	require.NoError(t, err)
	assert.False(t, resp.Hosts[0].Enabled)
}

func TestUpdateHostConfig_Archive(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: true},
	}
	require.NoError(t, config.SaveConfig(cfgPath, cfg))
	t.Setenv("TSM_CONFIG_PATH", cfgPath)

	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	resp, err := client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
		HostName: "mlab",
		Action:   tsmv1.HostConfigAction_HOST_CONFIG_ARCHIVE,
	})
	require.NoError(t, err)
	assert.True(t, resp.Hosts[0].Archived)
	assert.False(t, resp.Hosts[0].Enabled)
}

func TestUpdateHostConfig_Unarchive(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: false, Archived: true},
	}
	require.NoError(t, config.SaveConfig(cfgPath, cfg))
	t.Setenv("TSM_CONFIG_PATH", cfgPath)

	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	resp, err := client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
		HostName: "mlab",
		Action:   tsmv1.HostConfigAction_HOST_CONFIG_UNARCHIVE,
	})
	require.NoError(t, err)
	assert.False(t, resp.Hosts[0].Archived)
	assert.True(t, resp.Hosts[0].Enabled)
}

func TestUpdateHostConfig_UpdateColors(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: true},
	}
	require.NoError(t, config.SaveConfig(cfgPath, cfg))
	t.Setenv("TSM_CONFIG_PATH", cfgPath)

	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	resp, err := client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
		HostName: "mlab",
		Action:   tsmv1.HostConfigAction_HOST_CONFIG_UPDATE_COLORS,
		BarBg:    "#111",
		BarFg:    "#222",
		BadgeBg:  "#333",
		BadgeFg:  "#444",
		Color:    "#555",
	})
	require.NoError(t, err)
	assert.Equal(t, "#111", resp.Hosts[0].BarBg)
	assert.Equal(t, "#222", resp.Hosts[0].BarFg)
	assert.Equal(t, "#333", resp.Hosts[0].BadgeBg)
	assert.Equal(t, "#444", resp.Hosts[0].BadgeFg)
	assert.Equal(t, "#555", resp.Hosts[0].Color)
}

func TestUpdateHostConfig_Reorder(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "local", Address: "", Enabled: true, SortOrder: 0},
		{Name: "mlab", Address: "mlab", Enabled: true, SortOrder: 1},
		{Name: "air", Address: "air", Enabled: true, SortOrder: 2},
	}
	require.NoError(t, config.SaveConfig(cfgPath, cfg))
	t.Setenv("TSM_CONFIG_PATH", cfgPath)

	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	// 重新排列：air → mlab → local
	resp, err := client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
		Action:       tsmv1.HostConfigAction_HOST_CONFIG_REORDER,
		OrderedHosts: []string{"air", "mlab", "local"},
	})
	require.NoError(t, err)

	// 驗證 sort_order 被正確更新
	hostMap := make(map[string]int32)
	for _, h := range resp.Hosts {
		hostMap[h.Name] = h.SortOrder
	}
	assert.Equal(t, int32(0), hostMap["air"])
	assert.Equal(t, int32(1), hostMap["mlab"])
	assert.Equal(t, int32(2), hostMap["local"])
}

func TestUpdateHostConfig_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: true},
	}
	require.NoError(t, config.SaveConfig(cfgPath, cfg))
	t.Setenv("TSM_CONFIG_PATH", cfgPath)

	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	_, err := client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
		HostName: "nonexist",
		Action:   tsmv1.HostConfigAction_HOST_CONFIG_ENABLE,
	})
	require.Error(t, err)
	// 應回傳 NotFound
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestUpdateHostConfig_Unspecified(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: true},
	}
	require.NoError(t, config.SaveConfig(cfgPath, cfg))
	t.Setenv("TSM_CONFIG_PATH", cfgPath)

	exec := &fakeExecutor{listOutput: ""}
	client, _, cleanup := setupTestService(t, exec)
	defer cleanup()

	_, err := client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
		HostName: "mlab",
		Action:   tsmv1.HostConfigAction_HOST_CONFIG_ACTION_UNSPECIFIED,
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// loadTestConfig 從指定路徑讀取 config（測試輔助）。
func loadTestConfig(t *testing.T, path string) config.Config {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	cfg, err := config.LoadFromString(string(data))
	require.NoError(t, err)
	return cfg
}
```

- [ ] **Step 2: 確認測試失敗**

Run: `go test ./internal/daemon/... -v -race -run TestUpdateHostConfig`
Expected: FAIL（`UpdateHostConfig` 尚未實作）

- [ ] **Step 3: 實作 UpdateHostConfig**

`cfgMu`、`loadServerConfigPath`、`loadServerConfig` 已在 Task 2 中新增。實作 `UpdateHostConfig`：

```go
func (s *Service) UpdateHostConfig(_ context.Context, req *tsmv1.UpdateHostConfigRequest) (*tsmv1.UpdateHostConfigResponse, error) {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()

	cfgPath := loadServerConfigPath()
	cfg := loadServerConfig()

	// REORDER 不需要 host_name
	if req.Action == tsmv1.HostConfigAction_HOST_CONFIG_REORDER {
		for i, name := range req.OrderedHosts {
			for j := range cfg.Hosts {
				if cfg.Hosts[j].Name == name {
					cfg.Hosts[j].SortOrder = i
					break
				}
			}
		}
		if err := config.SaveConfig(cfgPath, cfg); err != nil {
			return nil, status.Errorf(codes.Internal, "save config: %v", err)
		}
		return &tsmv1.UpdateHostConfigResponse{Hosts: hostEntriesToProto(cfg.Hosts)}, nil
	}

	if req.Action == tsmv1.HostConfigAction_HOST_CONFIG_ACTION_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "action unspecified")
	}

	idx := -1
	for i, h := range cfg.Hosts {
		if h.Name == req.HostName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, status.Errorf(codes.NotFound, "host %q not found", req.HostName)
	}

	switch req.Action {
	case tsmv1.HostConfigAction_HOST_CONFIG_ENABLE:
		cfg.Hosts[idx].Enabled = true
	case tsmv1.HostConfigAction_HOST_CONFIG_DISABLE:
		cfg.Hosts[idx].Enabled = false
	case tsmv1.HostConfigAction_HOST_CONFIG_ARCHIVE:
		cfg.Hosts[idx].Archived = true
		cfg.Hosts[idx].Enabled = false
	case tsmv1.HostConfigAction_HOST_CONFIG_UNARCHIVE:
		cfg.Hosts[idx].Archived = false
		cfg.Hosts[idx].Enabled = true
	case tsmv1.HostConfigAction_HOST_CONFIG_UPDATE_COLORS:
		cfg.Hosts[idx].BarBG = req.BarBg
		cfg.Hosts[idx].BarFG = req.BarFg
		cfg.Hosts[idx].BadgeBG = req.BadgeBg
		cfg.Hosts[idx].BadgeFG = req.BadgeFg
		cfg.Hosts[idx].Color = req.Color
	}

	config.SyncLocalHostToConfig(&cfg)
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		return nil, status.Errorf(codes.Internal, "save config: %v", err)
	}
	return &tsmv1.UpdateHostConfigResponse{Hosts: hostEntriesToProto(cfg.Hosts)}, nil
}
```

- [ ] **Step 4: 確認測試通過**

Run: `go test ./internal/daemon/... -v -race -run TestUpdateHostConfig`
Expected: 8 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/service.go internal/daemon/service_test.go
git commit -m "feat: daemon — 實作 UpdateHostConfig RPC（enable/disable/archive/colors/reorder）"
```

---

### Task 4: Client — GetHostsConfig / UpdateHostConfig 方法

**Files:**
- Modify: `internal/client/client.go:1-170`

- [ ] **Step 1: 新增轉換函式和 client 方法**

在 `internal/client/client.go` 新增：

```go
// protoToHostEntry 將 proto HostConfigEntry 轉換為 config.HostEntry。
func protoToHostEntry(h *tsmv1.HostConfigEntry) config.HostEntry {
	return config.HostEntry{
		Name:      h.Name,
		Address:   h.Address,
		Color:     h.Color,
		Enabled:   h.Enabled,
		SortOrder: int(h.SortOrder),
		BarBG:     h.BarBg,
		BarFG:     h.BarFg,
		BadgeBG:   h.BadgeBg,
		BadgeFG:   h.BadgeFg,
		Archived:  h.Archived,
	}
}

// protoToHostEntries 將 []*tsmv1.HostConfigEntry 轉換為 []config.HostEntry。
func protoToHostEntries(hosts []*tsmv1.HostConfigEntry) []config.HostEntry {
	result := make([]config.HostEntry, len(hosts))
	for i, h := range hosts {
		result[i] = protoToHostEntry(h)
	}
	return result
}

// GetHostsConfig 取得 daemon 所在主機的 hosts 設定。
func (c *Client) GetHostsConfig(ctx context.Context) ([]config.HostEntry, error) {
	resp, err := c.rpc.GetHostsConfig(ctx, &tsmv1.GetHostsConfigRequest{})
	if err != nil {
		return nil, err
	}
	return protoToHostEntries(resp.Hosts), nil
}

// UpdateHostConfig 更新 daemon 所在主機的 hosts 設定，回傳更新後的完整清單。
func (c *Client) UpdateHostConfig(ctx context.Context, req *tsmv1.UpdateHostConfigRequest) ([]config.HostEntry, error) {
	resp, err := c.rpc.UpdateHostConfig(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToHostEntries(resp.Hosts), nil
}
```

- [ ] **Step 2: 驗證全部編譯通過**

Run: `go build ./...`
Expected: 成功

- [ ] **Step 3: Commit**

```bash
git add internal/client/client.go
git commit -m "feat: client — 新增 GetHostsConfig/UpdateHostConfig 方法"
```

---

## Chunk 2: UI 層讀寫路徑改造

### Task 5: UI — Model 新增 hubHosts 欄位 + 初始載入

**Files:**
- Modify: `internal/ui/app.go:80-131,737-840`

- [ ] **Step 1: 在 Model struct 新增 hubHosts 欄位**

在 `internal/ui/app.go` 第 124 行 `hubHostSnap` 之後新增：

```go
	hubHosts         []config.HostEntry          // hub-socket 模式：從 hub daemon 取得的主機設定
```

- [ ] **Step 2: 新增 helper 方法 isHubSocketMode**

在 `app.go` 新增：

```go
// isHubSocketMode 判斷是否為 hub-socket 模式（spoke 透過 reverse tunnel 連到 hub daemon）。
// 此模式下 hosts 設定從 hub daemon 讀取，而非本地 config。
func (m Model) isHubSocketMode() bool {
	return m.deps.HubMode && m.deps.HostMgr == nil
}
```

- [ ] **Step 3: 新增 Bubble Tea Cmd 和 Msg 用於載入 hubHosts**

```go
// hubHostsConfigMsg 攜帶從 hub daemon 取得的主機設定。
type hubHostsConfigMsg struct {
	Hosts []config.HostEntry
	Err   error
}

// fetchHubHostsCmd 從 hub daemon 非同步載入主機設定。
func fetchHubHostsCmd(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		hosts, err := c.GetHostsConfig(context.Background())
		return hubHostsConfigMsg{Hosts: hosts, Err: err}
	}
}
```

- [ ] **Step 4: 在 Init() 中 hub-socket 模式發起載入**

修改 `Init()`（第 737-758 行），在 hub 模式分支加入 `fetchHubHostsCmd`：

```go
func (m Model) Init() tea.Cmd {
	if m.deps.HubMode && m.deps.Client != nil {
		cmds := []tea.Cmd{watchHubCmd(m.deps.Client)}
		if m.isHubSocketMode() {
			cmds = append(cmds, fetchHubHostsCmd(m.deps.Client))
		}
		return tea.Batch(cmds...)
	}
	// ... 其餘不變
}
```

- [ ] **Step 5: 在 Update() 中處理 hubHostsConfigMsg**

在 `Update` 函式的 `switch msg := msg.(type)` 中新增 case（在 `HubSnapshotMsg` case 之後）：

```go
	case hubHostsConfigMsg:
		if msg.Err == nil {
			m.hubHosts = msg.Hosts
			// 若已有 hubHostSnap，用新的 hubHosts 重建列表
			if m.hubHostSnap != nil {
				m.rebuildHubItems()
			}
		}
		return m, nil
```

- [ ] **Step 6: 修改 HubSnapshotMsg handler 的 FilterActiveHosts 呼叫**

在 `app.go` 第 828 行，將 `FilterActiveHosts` 的第二個參數從 `m.deps.Cfg.Hosts` 改為 `m.hubHosts`（hub-socket 模式）：

```go
	case HubSnapshotMsg:
		// ... 錯誤處理不變 ...
		m.hubHostSnap = msg.Snapshot
		if !m.isHubSocketMode() {
			m.syncHubHostsToConfig()
		}
		inputs := ConvertMultiHostSnapshot(msg.Snapshot)
		if m.isHubSocketMode() {
			inputs = FilterActiveHosts(inputs, m.hubHosts)
		} else {
			inputs = FilterActiveHosts(inputs, m.deps.Cfg.Hosts)
		}
		m.items = FlattenMultiHost(inputs)
		// ... 其餘不變
```

- [ ] **Step 7: 驗證編譯通過**

Run: `go build ./...`
Expected: 成功

- [ ] **Step 8: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat: ui — hub-socket 模式新增 hubHosts 欄位及初始載入"
```

---

### Task 6: UI — hostpicker 讀取路徑改用 hubHosts

**Files:**
- Modify: `internal/ui/hostpicker.go:605-740`

- [ ] **Step 1: 修改 rebuildHubItems 使用 hubHosts**

在 `hostpicker.go` 第 645-658 行的 `rebuildHubItems`：

```go
func (m *Model) rebuildHubItems() {
	if m.hubHostSnap == nil {
		return
	}
	inputs := ConvertMultiHostSnapshot(m.hubHostSnap)
	if m.isHubSocketMode() {
		inputs = FilterActiveHosts(inputs, m.hubHosts)
	} else {
		inputs = FilterActiveHosts(inputs, m.deps.Cfg.Hosts)
	}
	m.items = FlattenMultiHost(inputs)
	if len(m.items) == 0 {
		m.cursor = 0
	} else if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
}
```

- [ ] **Step 2: 修改 visibleHubHosts 使用 hubHosts**

在 `hostpicker.go` 第 660-669 行的 `visibleHubHosts`：

```go
func (m Model) visibleHubHosts() []config.HostEntry {
	hosts := m.deps.Cfg.Hosts
	if m.isHubSocketMode() {
		hosts = m.hubHosts
	}
	var result []config.HostEntry
	for _, h := range hosts {
		if !h.Archived {
			result = append(result, h)
		}
	}
	return result
}
```

- [ ] **Step 3: 修改 hubHostOriginalIndex 使用 hubHosts**

在 `hostpicker.go` 第 732-740 行：

```go
func (m Model) hubHostOriginalIndex(name string) int {
	hosts := m.deps.Cfg.Hosts
	if m.isHubSocketMode() {
		hosts = m.hubHosts
	}
	for i, h := range hosts {
		if h.Name == name {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 4: 修改 ensureDraftFromEntry — 無需改動**

`ensureDraftFromEntry` 接收 `config.HostEntry` 參數，呼叫端傳入的是 `visibleHubHosts()` 的結果，已自動使用 `hubHosts`，不需修改。

- [ ] **Step 5: 修改 host picker 進入時刷新 hubHosts**

在 `app.go` 中按 `h` 進入 host picker 的邏輯（約第 1127-1131 行），hub-socket 模式下發起 `fetchHubHostsCmd` 刷新：

```go
	case "h":
		if m.deps.HostMgr != nil || (m.deps.HubMode && m.hubHostSnap != nil) {
			m.mode = ModeHostPicker
			m.hostPickerCursor = 0
			if m.isHubSocketMode() {
				return m, fetchHubHostsCmd(m.deps.Client)
			}
			return m, nil
		}
```

- [ ] **Step 6: 修改 [n] handler 的 host tab 來源**

在 `app.go` 第 1059-1067 行，hub 模式的 host tab 建立已經從 `hubHostSnap` 讀取（不從 `deps.Cfg.Hosts`），但需確保只顯示 `hubHosts` 中 enabled 的主機：

```go
		} else if m.deps.HubMode && m.hubHostSnap != nil {
			// hub-socket 模式：過濾 hubHosts 中已啟用的主機
			enabledHosts := make(map[string]bool)
			hostSource := m.deps.Cfg.Hosts
			if m.isHubSocketMode() {
				hostSource = m.hubHosts
			}
			for _, h := range hostSource {
				if h.Enabled && !h.Archived {
					enabledHosts[h.Name] = true
				}
			}
			for _, h := range m.hubHostSnap.Hosts {
				if h.Status != tsmv1.HostStatus_HOST_STATUS_CONNECTED {
					continue
				}
				if !enabledHosts[h.Name] {
					continue
				}
				hosts = append(hosts, hostTabInfo{
					ID: h.HostId, Name: h.Name, Color: h.Color,
				})
			}
		}
```

- [ ] **Step 7: 驗證編譯通過**

Run: `go build ./...`
Expected: 成功

- [ ] **Step 8: Commit**

```bash
git add internal/ui/hostpicker.go internal/ui/app.go
git commit -m "feat: ui — host picker 讀取路徑改用 hubHosts（hub-socket 模式）"
```

---

### Task 7: UI — hostpicker 寫入路徑改走 RPC

**Files:**
- Modify: `internal/ui/hostpicker.go:742-865`
- Modify: `internal/ui/app.go:1280-1340`

- [ ] **Step 1: 新增 Bubble Tea Cmd 和 Msg 用於 UpdateHostConfig**

在 `app.go` 新增：

```go
// hubHostConfigUpdatedMsg 攜帶 UpdateHostConfig RPC 的結果。
type hubHostConfigUpdatedMsg struct {
	Hosts []config.HostEntry
	Err   error
}
```

- [ ] **Step 2: 修改 space（toggle enable/disable）**

在 `hostpicker.go` 第 798-808 行的 space handler：

```go
	case " ":
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			if m.isHubSocketMode() {
				action := tsmv1.HostConfigAction_HOST_CONFIG_DISABLE
				if !h.Enabled {
					action = tsmv1.HostConfigAction_HOST_CONFIG_ENABLE
				}
				return m, func() tea.Msg {
					updated, err := m.deps.Client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
						HostName: h.Name,
						Action:   action,
					})
					return hubHostConfigUpdatedMsg{Hosts: updated, Err: err}
				}
			}
			origIdx := m.hubHostOriginalIndex(h.Name)
			if origIdx >= 0 {
				m.deps.Cfg.Hosts[origIdx].Enabled = !m.deps.Cfg.Hosts[origIdx].Enabled
				m.persistHubHosts()
				m.rebuildHubItems()
			}
		}
```

- [ ] **Step 3: 修改 d（archive）**

在 `hostpicker.go` 第 843-862 行的 `d` handler：

```go
	case "d":
		if m.hostPickerCursor < len(hosts) {
			h := hosts[m.hostPickerCursor]
			if !h.IsLocal() {
				if m.isHubSocketMode() {
					return m, func() tea.Msg {
						updated, err := m.deps.Client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
							HostName: h.Name,
							Action:   tsmv1.HostConfigAction_HOST_CONFIG_ARCHIVE,
						})
						return hubHostConfigUpdatedMsg{Hosts: updated, Err: err}
					}
				}
				origIdx := m.hubHostOriginalIndex(h.Name)
				if origIdx >= 0 {
					m.deps.Cfg.Hosts[origIdx].Archived = true
					m.deps.Cfg.Hosts[origIdx].Enabled = false
					m.persistHubHosts()
					m.rebuildHubItems()
					newVisible := m.visibleHubHosts()
					if m.hostPickerCursor >= len(newVisible) && len(newVisible) > 0 {
						m.hostPickerCursor = len(newVisible) - 1
					}
				}
			}
		}
		return m, nil
```

- [ ] **Step 4: 修改 J/K（reorder）**

在 `hostpicker.go` 第 809-834 行的 J/K handler，hub-socket 模式改走 RPC：

```go
	case "J", "shift+down":
		if m.hostPickerCursor < len(hosts)-1 {
			if m.isHubSocketMode() {
				// 交換順序：建立新排序清單
				reordered := make([]string, len(hosts))
				for i, h := range hosts {
					reordered[i] = h.Name
				}
				cur := m.hostPickerCursor
				reordered[cur], reordered[cur+1] = reordered[cur+1], reordered[cur]
				m.hostPickerCursor++
				return m, func() tea.Msg {
					updated, err := m.deps.Client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
						Action:       tsmv1.HostConfigAction_HOST_CONFIG_REORDER,
						OrderedHosts: reordered,
					})
					return hubHostConfigUpdatedMsg{Hosts: updated, Err: err}
				}
			}
			// 原有邏輯不變
			curName := hosts[m.hostPickerCursor].Name
			nextName := hosts[m.hostPickerCursor+1].Name
			curIdx := m.hubHostOriginalIndex(curName)
			nextIdx := m.hubHostOriginalIndex(nextName)
			if curIdx >= 0 && nextIdx >= 0 {
				m.deps.Cfg.Hosts[curIdx], m.deps.Cfg.Hosts[nextIdx] = m.deps.Cfg.Hosts[nextIdx], m.deps.Cfg.Hosts[curIdx]
				m.hostPickerCursor++
				m.persistHubHosts()
			}
		}
	case "K", "shift+up":
		if m.hostPickerCursor > 0 {
			if m.isHubSocketMode() {
				reordered := make([]string, len(hosts))
				for i, h := range hosts {
					reordered[i] = h.Name
				}
				cur := m.hostPickerCursor
				reordered[cur], reordered[cur-1] = reordered[cur-1], reordered[cur]
				m.hostPickerCursor--
				return m, func() tea.Msg {
					updated, err := m.deps.Client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
						Action:       tsmv1.HostConfigAction_HOST_CONFIG_REORDER,
						OrderedHosts: reordered,
					})
					return hubHostConfigUpdatedMsg{Hosts: updated, Err: err}
				}
			}
			// 原有邏輯不變
			curName := hosts[m.hostPickerCursor].Name
			prevName := hosts[m.hostPickerCursor-1].Name
			curIdx := m.hubHostOriginalIndex(curName)
			prevIdx := m.hubHostOriginalIndex(prevName)
			if curIdx >= 0 && prevIdx >= 0 {
				m.deps.Cfg.Hosts[curIdx], m.deps.Cfg.Hosts[prevIdx] = m.deps.Cfg.Hosts[prevIdx], m.deps.Cfg.Hosts[curIdx]
				m.hostPickerCursor--
				m.persistHubHosts()
			}
		}
```

- [ ] **Step 5: 在 hostDraftEntry 新增 Color 欄位**

在 `hostpicker.go` 第 20-26 行的 `hostDraftEntry` struct 新增 `Color` 欄位：

```go
type hostDraftEntry struct {
	Enabled bool
	BarBG   string
	BarFG   string
	BadgeBG string
	BadgeFG string
	Color   string // accent color
}
```

更新 `ensureDraftFromEntry`（第 672-686 行），初始化新增的 `Color`：

```go
	m.hostPanelDraft[entry.Name] = hostDraftEntry{
		Enabled: entry.Enabled,
		BarBG:   entry.BarBG,
		BarFG:   entry.BarFG,
		BadgeBG: entry.BadgeBG,
		BadgeFG: entry.BadgeFG,
		Color:   entry.Color,
	}
```

- [ ] **Step 6: 修改 Ctrl+S（色彩儲存）— 非同步 RPC**

在 `hostpicker.go` 第 754-762 行的 Ctrl+S handler。注意：所有 RPC 必須在 `tea.Cmd` 中非同步執行，不能在 `Update()` 中同步呼叫：

```go
	if msg.String() == "ctrl+s" {
		if m.isHubSocketMode() {
			// hub-socket 模式：收集 draft，在 tea.Cmd 中非同步發送 RPC
			drafts := make(map[string]hostDraftEntry, len(m.hostPanelDraft))
			for k, v := range m.hostPanelDraft {
				drafts[k] = v
			}
			c := m.deps.Client
			m.hostPanelOpen = false
			m.hostPanelEditing = false
			m.hostSavedMsg = "已儲存"
			return m, func() tea.Msg {
				var updated []config.HostEntry
				var lastErr error
				for hostName, draft := range drafts {
					result, err := c.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
						HostName: hostName,
						Action:   tsmv1.HostConfigAction_HOST_CONFIG_UPDATE_COLORS,
						BarBg:    draft.BarBG,
						BarFg:    draft.BarFG,
						BadgeBg:  draft.BadgeBG,
						BadgeFg:  draft.BadgeFG,
						Color:    draft.Color,
					})
					if err != nil {
						lastErr = err
					} else {
						updated = result
					}
				}
				return hubHostConfigUpdatedMsg{Hosts: updated, Err: lastErr}
			}
		}
		m.applyHubHostDrafts()
		m.persistHubHosts()
		m.rebuildHubItems()
		m.hostPanelOpen = false
		m.hostPanelEditing = false
		m.hostSavedMsg = "已儲存"
		return m, nil
	}
```

- [ ] **Step 7: 在 Update() 中處理 hubHostConfigUpdatedMsg**

在 `app.go` 的 `Update` 函式中新增 case：

```go
	case hubHostConfigUpdatedMsg:
		if msg.Err == nil && msg.Hosts != nil {
			m.hubHosts = msg.Hosts
			m.rebuildHubItems()
			// 收緊 host picker 游標
			visible := m.visibleHubHosts()
			if m.hostPickerCursor >= len(visible) && len(visible) > 0 {
				m.hostPickerCursor = len(visible) - 1
			}
		}
		return m, nil
```

- [ ] **Step 8: 修改新增主機（InputNewHost）走 RPC**

在 `app.go` 第 1300-1340 行的 `InputNewHost` handler，hub-socket 模式不再操作本地 `deps.Cfg.Hosts`：

```go
			} else if m.deps.HubMode {
				if m.isHubSocketMode() {
					// hub-socket 模式：新增主機由 hub daemon 端 HubManager.AddHost 自動處理
					// 若有同名封存主機，嘗試解封存（NotFound 錯誤可忽略）
					if found, _ := config.FindArchivedHost(m.hubHosts, value); found {
						return m, func() tea.Msg {
							updated, err := m.deps.Client.UpdateHostConfig(context.Background(), &tsmv1.UpdateHostConfigRequest{
								HostName: value,
								Action:   tsmv1.HostConfigAction_HOST_CONFIG_UNARCHIVE,
							})
							return hubHostConfigUpdatedMsg{Hosts: updated, Err: err}
						}
					}
					// 非封存主機：不做任何操作，hub daemon 自動管理
					m.mode = ModeHostPicker
					return m, nil
				}
				// tsm --host 模式：原有邏輯不變
				// ...
```

- [ ] **Step 9: 驗證編譯通過**

Run: `go build ./...`
Expected: 成功

- [ ] **Step 10: 執行全部測試**

Run: `go test ./... -race`
Expected: 全部 PASS

- [ ] **Step 11: Commit**

```bash
git add internal/ui/app.go internal/ui/hostpicker.go
git commit -m "feat: ui — hub-socket 模式 host picker 寫入路徑改走 UpdateHostConfig RPC"
```

---

### Task 8: 版本推進 + 最終驗證

**Files:**
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: 執行完整測試**

Run: `make test`
Expected: 全部 PASS

- [ ] **Step 2: 執行 lint**

Run: `make lint`
Expected: 無錯誤

- [ ] **Step 3: 執行建置**

Run: `make build`
Expected: `bin/tsm` 產生成功

- [ ] **Step 4: 更新 VERSION**

將 `VERSION` 從 `0.40.0` 更新為 `0.41.0`（新增功能 → MINOR）。

- [ ] **Step 5: 更新 CHANGELOG.md**

在 CHANGELOG.md 頂部新增 v0.41.0 條目。

- [ ] **Step 6: Commit**

```bash
git add VERSION CHANGELOG.md
git commit -m "chore: bump version to 0.41.0"
```
