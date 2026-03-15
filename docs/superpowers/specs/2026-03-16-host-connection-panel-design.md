# [h] 主機管理三欄佈局：連線面板設計

## 背景

[h] 主機管理面板目前是二欄結構（主機列表 + 設定面板）。連線相關的資訊和操作散落在 [i] 面板和手動 tmux 指令中，沒有統一的入口來查看和管理 SSH tunnel、reverse tunnel、hub-socket 的狀態。

本設計在 [h] 面板加入「連線」中欄，提供完整的主機連線狀態顯示和操作（檢測、重建 tunnel、設定 hub-socket）。

## 三欄佈局

```
┌─ 主機列表 ─┐  ┌─ {host} 連線 ──────┐  ┌─ {host} 設定 ──────┐
│             │  │                     │  │                     │
│  ● local    │  │  SSH:   —           │  │  名稱: mlab         │
│  ● mlab ◄  │  │  tmux:  ✓           │  │  位址: mlab         │
│  ● mlab2    │  │  tunnel: ✓ 已連線   │  │  顏色: #5B9BD5      │
│             │  │  hub-sock: ✓        │  │  bar_bg: ...        │
│             │  │  最後連線: 剛剛      │  │  bar_fg: ...        │
│             │  │                     │  │  badge_bg: ...      │
│             │  │  [r] 檢測           │  │  badge_fg: ...      │
│             │  │  [t] 重建 tunnel    │  │                     │
│             │  │  [s] 設定 hub-sock  │  │  [Ctrl+S] 儲存      │
└─────────────┘  └─────────────────────┘  └─────────────────────┘
     焦點 0              焦點 1                   焦點 2
```

### 焦點模型

新增 `hostFocusCol int`（0=左, 1=中, 2=右）取代現有 `hostPanelOpen bool`。

| 焦點 | ↑↓ | ←→ | 特殊按鍵 |
|------|----|----|----------|
| 0（左欄） | 選主機 | → 展開並移到中欄 | Enter 同 →、Space 切換啟用、n/d/J/K |
| 1（中欄） | 連線項目間移動 | → 移到右欄、← 收合回左欄 | r/t/s 操作 |
| 2（右欄） | 設定欄位間移動 | ← 移到中欄 | Enter 編輯色彩、Ctrl+S 儲存 |

- Esc：任何位置都收合回左欄
- 非焦點欄 dimmed 顯示

### 展開/收合

- Enter/→ 從左欄展開：中欄 + 右欄同時出現，焦點到中欄
- ← 從中欄回左欄：兩欄同時收合
- 切換主機（收合後 ↑↓ 換主機再 Enter）：展開新主機的面板
- 設定欄的 draft 不因收合而丟失（per-host `hostPanelDraft` 保留）

## 連線欄內容

### Remote 主機

```
── mlab 連線 ──
  SSH:       ✓ 可連線
  tmux:      ✓ 已安裝
  tunnel:    ✓ 已連線
  hub-sock:  ✓ 已設定
             ~/.config/tsm/tsm-hub-dd05...sock
  最後連線:  剛剛
  錯誤:      （無）

  [r] 重新檢測  [t] 重建 tunnel  [s] 設定 hub-socket
```

### Local 主機

```
── local 連線 ──
  tmux:      ✓ 已安裝

  [r] 重新檢測
```

Local 沒有 SSH/tunnel/hub-socket，也沒有 [t] [s] 按鍵。

### 狀態資料來源

| 欄位 | 來源 |
|------|------|
| SSH / tmux | `hostcheck.Checker`（即時檢測，[r] 觸發） |
| tunnel | `HostState.status`（CONNECTING/CONNECTED/DISCONNECTED） |
| hub-socket | `HostState` 或 SSH 到遠端查詢 `@tsm_hub_socket` |
| 最後連線 | `HostState.last_connected`（新增欄位） |
| 錯誤 | `HostState.last_error`（新增欄位） |

### 操作

| 按鍵 | 條件 | 行為 |
|------|------|------|
| `r` | 所有主機 | 非同步執行 `hostcheck`，結果更新到畫面 |
| `t` | remote 且焦點在中欄 | `ReconnectHost(host_id)` RPC → daemon 重啟該主機的 `runRemote` goroutine |
| `s` | remote 且焦點在中欄 | 文字輸入 hub-socket 路徑（見下方流程） |

連線欄**不使用 draft**。[r] [t] 是立即執行，[s] 的文字輸入完成後立即生效。

### [s] 設定 hub-socket 流程

1. 按 [s] → 進入文字輸入模式，預填 `detectHubSocket()` 偵測到的路徑
2. 使用者輸入或修改路徑（必須含 `tsm-hub-` pattern，否則顯示錯誤不執行）
3. Enter 確認 → SSH 到遠端執行 `tmux set-option -g @tsm_hub_socket <path>`（復用 `remote.SetHubSocket`）
4. 同時設定 `@tsm_hub_self`（復用 `remote.SetHubSelf`）
5. 成功 → 更新連線欄 hub-sock 狀態為 ✓
6. 失敗 → 顯示錯誤訊息（如「SSH 執行失敗」）
7. Esc → 取消輸入，不執行

注意：輸入的是**遠端主機上的**路徑。無法在本機做 `os.Stat` 驗證。由 SSH 執行結果判斷成功與否。

## 新增 RPC

### ReconnectHost

```protobuf
message ReconnectHostRequest {
  string host_id = 1;
}

message ReconnectHostResponse {
  bool success = 1;
  string error = 2;
}

rpc ReconnectHost(ReconnectHostRequest) returns (ReconnectHostResponse);
```

daemon 端 `HubManager.ReconnectHost(hostID)`：
1. Cancel 該主機的 `runRemote` goroutine context
2. 等待 goroutine 結束（cleanup 執行：`ClearHubSocket` + `ClearHubSelf` + `tun.Close()`）
3. 重新啟動新的 `runRemote` goroutine（完整流程）
4. 對 local 主機回傳 error

**實作前提**：目前 `hubHost` struct 沒有 per-host cancel func。所有 remote goroutine 共用 `remoteCancel` context。需要將 `hubHost` 擴充為：

```go
type hubHost struct {
    config       config.HostEntry
    status       tsmv1.HostStatus
    snapshot     *tsmv1.StateSnapshot
    lastErr      string
    connectedAt  time.Time           // 新增
    cancel       context.CancelFunc  // 新增：per-host cancel
    done         chan struct{}        // 新增：goroutine 結束信號
    // ... 現有欄位
}
```

`StartRemoteHosts` 為每台主機建立獨立的 child context（從共用 parent 派生），存入 `hubHost.cancel`。`ReconnectHost` 呼叫 `cancel()` + `<-done` + 重啟。

### Spoke 端的 [t] 行為

hub-socket 模式（spoke）下，[t] 的 RPC 透過 reverse tunnel 送到 hub daemon，由 hub 執行重連。spoke 本身不執行任何連線操作。任何連到 hub daemon 的客戶端都可觸發 `ReconnectHost`，不做額外權限限制（hub daemon 是受信任的內部服務）。

### HostState 擴充

現有 `HostState` 已使用 field 1-7（host_id, name, color, status, error, snapshot, address）。新增：

```protobuf
message HostState {
  // ... 現有 field 1-7 ...
  google.protobuf.Timestamp last_connected = 8;
}
```

注意：現有 `error`（field 5）已可表達最後錯誤，不需另加 `last_error`。連線欄的「錯誤」直接讀取 `HostState.error`。

`last_connected` 資料來源：`HubManager` 的 `hubHost` struct 新增 `connectedAt time.Time`，在 `watchRemote` 成功連線時記錄。

## [i] 面板變化

### 搬到連線欄

- Hub socket 路徑輸入 → 連線欄 [s]
- 當前 socket 路徑顯示 → 連線欄 hub-sock 欄位

### [i] 保留（唯讀）

- 模式（Hub/Daemon/Direct）
- 版本
- Daemon socket 路徑

### [i] 移除

- Hub 連線輸入框 + Enter 連線
- `hubReconnectSock` 機制（被 `ReconnectHost` RPC 取代）

[i] 變成純粹的唯讀資訊面板。

## Hub 模式與非 Hub 模式

`hostFocusCol` 和三欄佈局同時適用兩種模式。差異：

| 面向 | Hub 模式 | 非 Hub 模式（HostMgr） |
|------|----------|----------------------|
| 左欄資料 | `visibleHubHosts()` → `[]config.HostEntry` | `deps.HostMgr.Hosts()` → `[]*hostmgr.Host` |
| tunnel 狀態 | `hubHostSnap.Hosts` 的 `HostStatus` | `hostmgr.Host.Status()` |
| 錯誤訊息 | `HostState.error` | `hostmgr.Host.LastError()` |
| hub-sock | 從 snapshot 或 SSH 查詢 | 從 snapshot 或 SSH 查詢（相同） |
| 儲存方式 | `UpdateHostConfig` RPC | `persistHostsWithSync()` 本地 config |
| [t] 重建 | `ReconnectHost` RPC | `ReconnectHost` RPC（相同 daemon） |

中欄和右欄的渲染邏輯統一——接收 host name/address/enabled 等共通資訊，不直接操作 `hostmgr.Host` 或 `config.HostEntry`。由 `hostpicker.go` 負責從不同資料來源組裝共通結構，傳給中欄/右欄。

## 連線欄的 loading 狀態

[r] 檢測和 [t] 重建是非同步操作。進行中的 UI：

```
── mlab 連線 ──
  SSH:       ⠋ 檢測中...        ← [r] 觸發後顯示 spinner
  tmux:      ⠋ 檢測中...
  tunnel:    ⠋ 重建中...        ← [t] 觸發後顯示 spinner
```

使用 `hostcheck.StatusChecking` 狀態控制。結果回來後自動更新。

## 時間格式

「最後連線」使用相對時間：
- < 1 分鐘 → 「剛剛」
- < 1 小時 → 「N 分鐘前」
- < 24 小時 → 「N 小時前」
- 其他 → 「N 天前」

復用 `tmux.Session.RelativeTime()` 的現有格式函式。

## 檔案拆分

### 現有

```
hostpicker.go (1255 行) — 全部混在一起
```

### 拆分後

```
hostpicker.go      — 三欄編排、焦點管理、左欄列表
hostconnection.go  — 中欄：連線狀態 state、update、render
hostsettings.go    — 右欄：設定面板 state、update、render
```

### 呼叫關係

```
hostpicker.go
  ├─ updateHostPicker(msg)
  │   switch hostFocusCol:
  │     case 0: updateHostPickerLeft(msg)
  │     case 1: updateHostConnection(msg)     ← hostconnection.go
  │     case 2: updateHostSettings(msg)       ← hostsettings.go
  │
  └─ renderHostPicker()
      left  := renderHostPickerLeft()
      mid   := renderHostConnection(host)     ← hostconnection.go
      right := renderHostSettings(host)       ← hostsettings.go
      lipgloss.JoinHorizontal(left, mid, right)
```

## TDD 策略

### 新增測試

**hostconnection_test.go：**
- 連線欄渲染：local 只顯示 tmux
- 連線欄渲染：remote 顯示 SSH/tmux/tunnel/hub-sock
- [r] 觸發 hostcheck
- [t] 對 local 無效
- [s] 只接受 tsm-hub-* pattern

**hostsettings_test.go：**
- 從 hostpicker_test.go 搬來的設定面板測試

**hostpicker_test.go：**
- 三欄焦點切換：左→中→右→←中→←左（收合）
- Esc 從任何位置收合
- 展開時 dimmed 非焦點欄

**internal/daemon/hub_test.go：**
- ReconnectHost remote → 成功
- ReconnectHost local → error
- ReconnectHost unknown → error

**internal/client/client_test.go：**
- ReconnectHost RPC 呼叫

**internal/daemon/state_test.go：**
- HostState 包含 last_connected

**internal/ui/hostconnection_test.go：**
- hub-socket 模式下 [t] 透過 RPC 送到 hub daemon

### 不測

- 實際 SSH 連線（mock ExecFn）
- tmux display-popup
- lipgloss 渲染的精確字串
