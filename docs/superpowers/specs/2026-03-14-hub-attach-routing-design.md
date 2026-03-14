# Hub-Socket Attach Routing Design

## 問題

Hub-socket 模式下，spoke 端 Ctrl+Q popup 選擇跨主機 session 時，嘗試從 spoke 直接 SSH 到目標主機。這有兩個根本問題：

1. **Spoke 缺乏 SSH config**：只有 hub（mac-air）有完整的 SSH config 可連線所有主機，spoke 不一定能解析或連線其他主機
2. **Hub session 是 local**：hub 的 session 對 hub 而言是 local，不需要任何網路連線即可 attach

## 目標

所有跨主機 session attach 統一由 hub 端發起。spoke 只負責「選」，hub 負責「切換」。

## 路由規則

| 使用者位置 | 選擇目標 | 行為 |
|-----------|---------|------|
| B popup | B session | popup 退出 → B 本地 switch session |
| B popup | A session（hub） | RequestAttach → detach B → A 本地 switchToSession |
| B popup | C session（其他 spoke） | RequestAttach → detach B → A 執行 remote.Attach(C) |

## 架構設計

### 1. Proto 層

新增兩個 RPC：

```protobuf
message RequestAttachRequest {
  string host_id = 1;       // 目標主機 ID
  string session_name = 2;  // 目標 session 名稱
}

message RequestAttachResponse {
  bool success = 1;
}

message TakePendingAttachRequest {}

message TakePendingAttachResponse {
  string host_id = 1;       // 空字串 = 無 pending
  string session_name = 2;
}

// Service 新增：
rpc RequestAttach(RequestAttachRequest) returns (RequestAttachResponse);
rpc TakePendingAttach(TakePendingAttachRequest) returns (TakePendingAttachResponse);
```

### 2. Daemon 層（HubManager）

`HubManager` 新增 pending attach 暫存：

```go
type pendingAttach struct {
    HostID      string
    SessionName string
}
```

- `SetPendingAttach(hostID, session)`：寫入（後寫覆蓋）
- `TakePendingAttach() *pendingAttach`：讀取並清除（read-and-clear，確保只消費一次）
- 兩者皆由 `mu` 保護

`Service.RequestAttach`：驗證 hostID 存在於 `HubManager.hosts` → 存入 pending → 回傳 success。hostID 不存在 → `codes.NotFound`。

`Service.TakePendingAttach`：呼叫 `HubManager.TakePendingAttach()`，回傳結果。

### 3. Client 層

新增：

```go
func (c *Client) RequestAttach(ctx context.Context, hostID, sessionName string) error
func (c *Client) TakePendingAttach(ctx context.Context) (hostID, sessionName string, err error)
```

### 4. Spoke 端行為（runHubTUI with hctx）

選擇 session 後的路由：

- `item.HostID == hctx.HubSelf` → 本機 session → `switchToSession`（現有行為不變）
- 其他 → 跨主機：
  1. `client.RequestAttach(hostID, sessionName)`
  2. `tmux detach-client`（觸發 SSH 斷線，回到 hub）
  3. `return hubResultExit`

spoke 不再呼叫 `remote.Attach`，不再呼叫 `resolveHubHost`。

**detach 失敗處理**：`tmux detach-client` 失敗（極罕見）時，spoke 忽略錯誤並仍 return `hubResultExit`。pending 已存入 daemon，hub 會在下次 attach 返回時消費。

### 5. Hub 端消費（runHubTUI without hctx）

`remote.Attach` 返回 `AttachDetached` 後，新增 pending 檢查。**`hubReconnectLoop` 中的 re-attach 返回後也需相同檢查**：

```go
if result == remote.AttachDetached {
    if remote.CheckAndClearExitRequested(hostEntry.Address) {
        return hubResultExit  // ctrl+e 退出（現有行為）
    }
    // 檢查 spoke 是否請求了跨主機切換
    if pendingHostID, pendingSession, _ := c.TakePendingAttach(ctx); pendingHostID != "" {
        pendingHost := findHostEntry(cfg.Hosts, pendingHostID)
        if pendingHost == nil || pendingHost.IsLocal() {
            // hub local session
            switchToSession(pendingSession, false)
            if cfg.InPopup { return hubResultExit }
        } else {
            // 遠端 spoke session — 由 hub SSH 過去
            // 更新 hostEntry 讓外層迴圈用新目標繼續 attach + reconnect
            hostEntry = pendingHost
            selected = pendingSession
            goto attachRemote  // 或重構為函式呼叫
        }
    }
    continue  // 無 pending → 回到 TUI（現有行為）
}
```

**「hub local」判斷**：使用 `findHostEntry(cfg.Hosts, hostID)` 取得 entry，再以 `entry.IsLocal()`（即 `Address == ""`）判斷。

**ctrl+e 退出路徑不受影響**：spoke 的 ctrl+e 只寫 exit marker + detach，不呼叫 RequestAttach，hub 在步驟 1 就會 return exit。

### 6. 清理

**移除**：
- `detectLocalAddr()`、`resolveSSHHostname()`（v0.41.1 新增）
- `resolveHubHost()`
- `@tsm_hub_host` tmux 選項相關：`SetHubHost`/`ClearHubHost`、`readHubContext` 中的 hubHost
- `hubContext.HubHost` 欄位
- `readHubContext()` 中讀取 `@tsm_hub_host` 的邏輯（簡化為只讀 `@tsm_hub_self`）
- `net` import

**保留**：
- `@tsm_hub_self`（spoke 判斷哪些 session 是自己的）
- `@tsm_hub_socket`（spoke 連回 hub daemon）

## 邊界處理

| 情況 | 處理 |
|------|------|
| spoke 發 RequestAttach 後 detach 前斷線 | pending 已存，hub 下次 attach 返回時消費 |
| spoke 發 RequestAttach 後 hub daemon 重啟 | pending 丟失（記憶體），hub 回到 TUI，使用者重選 |
| 連續快速切換 | 後寫覆蓋，只執行最後一次 |
| pending hostID 對應的 spoke 已斷線 | remote.Attach 失敗 → 進入重連 modal（現有行為） |
| detach-client 失敗 | spoke 忽略錯誤並退出 popup，pending 已存，hub 下次消費 |
| ctrl+e 退出（不選 session） | 只寫 exit marker + detach，不觸發 RequestAttach，hub 走現有 exit 路徑 |

## 已知限制

- **單 slot pending**：多 spoke 同時發 RequestAttach 時 last-write-wins，僅保留最後一筆。單一使用者場景下不成問題。

## 不受影響

- `tsm --host` 多主機模式（HostMgr 直接路由）
- direct / daemon 單機模式
- ProxyMutation（session 建立/刪除/改名等操作）
- hub-socket config unification（GetHostsConfig / UpdateHostConfig）
- hub 端直接選擇 local / remote session 的現有路徑（`hctx == nil` 時不經過 RequestAttach）
