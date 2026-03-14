# Hub-Socket Config Unification Design

## 問題

Hub-socket 模式下（spoke 透過 reverse tunnel 連到 hub daemon），主機管理面板讀寫的是 spoke 本地 `config.toml` 的 `[[hosts]]`，而非 hub daemon（client 端）的 config。導致：

- spoke（mlab）看到的主機清單與 hub（mac-air）不一致
- spoke 上的 enable/disable/archive 操作只改本地檔案，不影響 hub 端
- 兩端主機設定互相衝突

## 目標

Hub-socket 模式下，所有 `[[hosts]]` 讀寫操作透過 gRPC RPC 走 hub daemon，統一讀取/更新 client 端（mac-air）的主機設定。

## 範圍

- **包含**：`[[hosts]]` 區段（enabled/disabled/archived、色彩、排序）
- **不包含**：全域設定（preview_lines、poll_interval、`[local]` 色彩）— 各機器獨立
- **不做降級**：hub daemon 必須支援新 RPC

## 模式判斷

| 條件 | 模式 | Host config 來源 |
|------|------|-----------------|
| `deps.HostMgr != nil` | tsm --host | 本地 config（現有行為不變）|
| `deps.HubMode && deps.HostMgr == nil` | hub-socket | RPC → hub daemon 的 config |
| 其他 | direct / daemon | 無 host 管理 |

判斷式：`deps.HubMode && deps.HostMgr == nil` = hub-socket 模式

## 架構設計

### 1. Proto 層

在 `api/proto/tsm/v1/tsm.proto` 新增：

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

message GetHostsConfigRequest {}
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

Service 新增：

```protobuf
rpc GetHostsConfig(GetHostsConfigRequest) returns (GetHostsConfigResponse);
rpc UpdateHostConfig(UpdateHostConfigRequest) returns (UpdateHostConfigResponse);
```

### 2. Hub Daemon 端（service.go）

**並發控制**：在 `Service` struct 新增 `cfgMu sync.Mutex`，所有 config 讀寫序列（`loadServerConfig` + `SaveConfig`）必須持鎖。

**GetHostsConfig**：讀取 daemon 所在機器的 `config.toml`，將 `cfg.Hosts` 轉為 `[]HostConfigEntry` 回傳。

**UpdateHostConfig**：讀取 config → 根據 action 修改對應 host → 寫回 config 檔 → 回傳更新後完整清單。

```go
func (s *Service) UpdateHostConfig(..., req) {
    s.cfgMu.Lock()
    defer s.cfgMu.Unlock()

    cfg := loadServerConfig()
    idx := findHostByName(cfg.Hosts, req.HostName)
    if idx < 0 {
        return nil, status.Errorf(codes.NotFound, "host %q not found", req.HostName)
    }
    switch req.Action {
    case UNSPECIFIED:
        return nil, status.Error(codes.InvalidArgument, "action unspecified")
    case ENABLE:         cfg.Hosts[idx].Enabled = true
    case DISABLE:        cfg.Hosts[idx].Enabled = false
    case ARCHIVE:        cfg.Hosts[idx].Archived = true; cfg.Hosts[idx].Enabled = false
    case UNARCHIVE:      cfg.Hosts[idx].Archived = false; cfg.Hosts[idx].Enabled = true
    case UPDATE_COLORS:  // 寫入 bar_bg/bar_fg/badge_bg/badge_fg/color
    case REORDER:        // 根據 ordered_hosts 重排 sort_order
    }
    SaveConfig(cfgPath, cfg)
    return &UpdateHostConfigResponse{Hosts: hostEntriesToProto(cfg.Hosts)}, nil
}
```

**錯誤處理**：
- `host_name` 不存在 → `codes.NotFound`
- `action = UNSPECIFIED` → `codes.InvalidArgument`
- config 寫入失敗 → `codes.Internal`

### 3. Client 層（client.go）

新增兩個方法：

```go
func (c *Client) GetHostsConfig(ctx context.Context) ([]config.HostEntry, error)
func (c *Client) UpdateHostConfig(ctx context.Context, req *tsmv1.UpdateHostConfigRequest) ([]config.HostEntry, error)
```

`UpdateHostConfig` 回傳更新後的完整 hosts 清單，供 UI 直接使用（省去額外 `GetHostsConfig` round trip）。

轉換函式 `hostEntryToProto` / `protoToHostEntry` 處理 `config.HostEntry` ↔ `HostConfigEntry` 雙向對應。

### 4. UI 讀取路徑

Hub-socket 模式下在 `Model` struct 新增 `hubHosts []config.HostEntry` 欄位，取代 `deps.Cfg.Hosts`。

**生命週期**：
1. **TUI 啟動時**：`Init()` 中 hub-socket 模式發起 `GetHostsConfig` 呼叫，結果存入 `m.hubHosts`
2. **每次 HubSnapshotMsg**：使用 `m.hubHosts`（非 `deps.Cfg.Hosts`）進行 `FilterActiveHosts`
3. **進入 host picker 面板時**：重新呼叫 `GetHostsConfig` 刷新 `m.hubHosts`，確保最新狀態

| 函式 | 原本讀 | 改為讀 |
|------|--------|--------|
| `FilterActiveHosts` | `deps.Cfg.Hosts` | `m.hubHosts` |
| `visibleHubHosts` | `deps.Cfg.Hosts` | `m.hubHosts` |
| `syncHubHostsToConfig` | `deps.Cfg.Hosts` | hub-socket 模式下跳過（新主機由 hub daemon 自動管理） |
| `ensureDraftFromEntry` | `deps.Cfg.Hosts` | `m.hubHosts` |
| `hubHostOriginalIndex` | `deps.Cfg.Hosts` | `m.hubHosts` |
| new session host tab（`[n]` handler） | `deps.Cfg.Hosts` | `m.hubHosts` |

### 5. UI 寫入路徑

Hub-socket 模式下所有 host config 修改改走 RPC，RPC 回傳更新後清單直接寫入 `m.hubHosts`：

| 操作 | 原本 | 改為 |
|------|------|------|
| Enable | 改 `deps.Cfg.Hosts` → `persistHubHosts()` | `UpdateHostConfig(ENABLE)` |
| Disable | 同上 | `UpdateHostConfig(DISABLE)` |
| Archive | 同上 | `UpdateHostConfig(ARCHIVE)` |
| Unarchive | 同上 | `UpdateHostConfig(UNARCHIVE)` |
| 色彩編輯 Ctrl+S | `applyHubHostDrafts()` → `persistHubHosts()` | `UpdateHostConfig(UPDATE_COLORS)` |
| 排序 J/K | 交換順序 → `persistHubHosts()` | `UpdateHostConfig(REORDER)` + `ordered_hosts` 清單 |

`persistHubHosts`：hub-socket 模式下不再呼叫。`tsm --host` 模式維持現有行為。

Draft 機制（`hostPanelDraft` / `applyHubHostDrafts`）在 hub-socket 模式下仍用於 UI 暫存，Ctrl+S 時改走 RPC。

## 不受影響的部分

- `tsm --host` 多主機模式（`deps.HostMgr != nil`）：完全不變
- direct / daemon 模式：完全不變
- 全域設定（GetConfig/SetConfig）：完全不變
- ProxyMutation（session 操作）：完全不變
