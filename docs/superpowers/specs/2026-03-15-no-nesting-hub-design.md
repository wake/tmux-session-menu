# 消除 tmux 嵌套：統一跨主機切換設計

## 背景

當 mac-air（hub）透過 `tsm --host` 連線到 mlab 等遠端主機時，目前的 popup 模式在 mac-air 的 tmux window 內開啟 SSH，導致 mlab 的 tmux 嵌套在 mac-air 的 tmux 裡，產生雙層 status bar。

本設計消除所有 tmux 嵌套，讓跨主機切換統一走「detach → 外層 tsm 接手 → 直接連線」的路徑。

## 核心原則

- **零嵌套**：任何時刻終端機裡只有一層 tmux
- **外層 tsm 為 hub 協調者**：`tsm --host` 在裸 shell 運行，是所有跨主機連線的唯一入口
- **popup 只做選擇，不做連線**：popup 選到跨主機 session 時，透過 daemon RPC 委託外層 tsm 處理

## 兩種運行模式

### Hub Mode（裸 shell `tsm --host`）

```
mac-air 裸 shell
  └─ tsm --host mlab1 mlab2 → runHubTUI for loop (hub menu)
       ├─ 選 local → tmux attach-session → detach → 回到 for loop
       ├─ 選 mlab1 → ssh -t mlab1 "tmux attach" → detach/斷線 → 回到 for loop
       └─ 選 mlab2 → ssh -t mlab2 "tmux attach" → detach/斷線 → 回到 for loop
```

跨主機切換由 popup 發起，外層 for loop 接手：

```
mlab1 tmux → Ctrl+Q → hub menu → 選 local
  → RequestAttach("local", session) + tmux detach-client
  → SSH 結束 → 外層 tsm TakePendingAttach → tmux attach-session
```

### Daemon Mode（tmux 內 Ctrl+Q，無 hub-socket）

只看到本機 sessions，`switch-client` 切換。無跨主機能力。

### Spoke Mode（tmux 內 Ctrl+Q，有 hub-socket）

透過 reverse tunnel 連到 hub daemon，看到所有主機。同主機 `switch-client`，跨主機 `RequestAttach + detach`。

## 模式判斷邏輯

```
tsm 啟動
  ├─ --hub-socket <path> → Spoke Mode
  ├─ --host / --local
  │    ├─ $TMUX == "" → Hub Mode（外層 for loop）
  │    └─ $TMUX != "" → Popup Hub Mode
  │         ├─ 同主機 → switch-client
  │         └─ 跨主機 → RequestAttach + detach-client
  └─ 無參數 → Daemon Mode（純本機）
```

## 跨主機切換流程

### Popup 內的跨主機選擇（Spoke Mode 和 Popup Hub Mode 統一行為）

```
popup TUI: 使用者選了不同主機的 session
  ├─ c.RequestAttach(hostID, sessionName)
  │   ├─ 成功 → 設定 pendingRequested = true → 繼續
  │   └─ 失敗 → 顯示錯誤，不 detach
  ├─ tmux detach-client → popup 銷毀
  └─ 外層 tsm for loop 接手：
      TakePendingAttach → 連到目標
```

### Popup 異常關閉的清理

popup TUI 用 `pendingRequested bool` 旗標追蹤。退出時（非 detach 路徑），如果旗標為 true，呼叫 `CancelPendingAttach` 清除。

### 外層 for loop 的 pending 檢查點

`TakePendingAttach` 檢查在兩個位置：

1. **remote.Attach 返回後**（已存在，`main.go:1165`）
2. **switchToSession 返回後**（新增）— local tmux detach 回來時檢查

```
switchToSession 返回
  → TakePendingAttach
  ├─ 有 pending → 直接連到目標（不回 TUI）
  │   ├─ pending.IsLocal → tmux attach-session
  │   └─ pending.IsRemote → remote.Attach (ssh)
  └─ 無 pending → continue（回到 TUI）
```

### 程式碼改動點

`runHubTUI` 中 `cfg.InPopup && hostEntry 為 remote` 的分支（`main.go:1145-1153`）：

- **舊**：`tmux new-window "ssh -t host 'tmux attach'"`（嵌套）
- **新**：`RequestAttach + detach-client`（委託外層）

同時，mac-air 本機 popup（`$TMUX != ""` + `--host`）的跨主機路徑也走相同邏輯。

## 新增 RPC

```protobuf
rpc CancelPendingAttach(CancelPendingAttachRequest) returns (CancelPendingAttachResponse);
```

daemon 端：`HubManager.CancelPendingAttach()` 將 `pending` 設為 nil。

## 邊界情況處理

| 情境 | 處理 |
|------|------|
| popup Esc/q 關閉，已發 RequestAttach | `CancelPendingAttach` 清除 |
| RequestAttach 成功但 detach 失敗 | popup cleanup 統一處理 |
| 外層 tsm 不存在（daemon mode） | daemon mode 不顯示 hub menu，不會觸發跨主機 |
| `@tsm_hub_socket` 存在但外層 tsm 已退出 | 外層 tsm 退出時 `ClearHubSocket`（已有此邏輯） |
| 目標主機不可達 | 走現有 `hubReconnectLoop` 重連機制 |
| 快速連續切換 | last-write-wins，每次 Attach 返回後檢查最新 pending |
| daemon 重啟，pending 丟失 | for loop 無 pending → 回到 TUI 讓使用者重新選擇 |
| SSH 在 popup 期間斷線 | 外層 remote.Attach 返回 Disconnected → reconnect 流程 |
| pending 指向已停用/封存主機 | `findHostEntry` 找不到或已停用 → 忽略 pending，回到 menu |

## Host Preflight Check 系統

### 新模組 `internal/hostcheck/`

```go
type Status int
const (
    StatusUnknown  Status = iota
    StatusChecking
    StatusReady
    StatusFailed
)

type Result struct {
    Status    Status
    SSH       *bool      // nil=local 不適用, true=通過, false=失敗
    Tmux      *bool      // true=通過, false=失敗
    Message   string     // 失敗時的錯誤訊息
    CheckedAt time.Time
}
```

### 檢測項目

| 主機類型 | 檢測內容 | 方法 |
|----------|----------|------|
| local | tmux 已安裝 | `exec.LookPath("tmux")` |
| remote | SSH 可連線 | `ssh -o ConnectTimeout=5 host "echo ok"` |
| remote | tmux 已安裝 | `ssh host "which tmux"` |

### 檢測時機

1. `tsm --host` 啟動時 — 對所有 enabled 主機並行檢測
2. [h] 面板新增主機後 — 立即檢測
3. [h] 面板手動 [r] 觸發 — 重新檢測

### 啟用限制

未通過檢測的主機可以新增、設定，但不能啟用。嘗試啟用時顯示錯誤訊息。

### 檢測結果不持久化

不存入 config.toml。每次 tsm 啟動時重新檢測。原因：網路狀態隨時變化，上次結果不可信。

### [h] 面板顯示

```
── 主機管理 ──
  ● local          [tmux ✓]              已啟用
  ● mlab1          [ssh ✓] [tmux ✓]     已啟用
  ● mlab2          [ssh ✓] [tmux ✗]     已停用 (tmux 未安裝)
  ● lab-server     [ssh ✗]              已停用 (連線逾時)
  ○ old-server                           已封存

  [a] 新增  [r] 檢測  [Enter] 編輯  [e] 啟用/停用  [d] 封存
```

## [i] 面板持久化（Bug 2 修復）

`infoConnect()` 成功連線後，額外寫入 `@tsm_hub_socket`：

```go
if m.deps.Cfg.InTmux {
    exec := tmux.NewRealExecutor()
    exec.Execute("set-option", "-g", "@tsm_hub_socket", sockPath)
}
```

下次 Ctrl+Q 時 bind block 偵測到有效 socket，自動走 hub-socket 模式。

## Ctrl+Q Bind Block

bind block 本身不變。行為改動全在 tsm 內部的 `runHubTUI` 和模式判斷中。

## TDD 策略

### 測試分層

**Unit Tests（hostcheck 模組）**：
- local tmux 檢測（LookPath mock）
- remote SSH 檢測（sshRunFn mock）
- remote tmux 檢測（sshRunFn mock）
- 逾時處理
- 並行檢測多台主機
- 檢測結果模型

**Unit Tests（CancelPendingAttach）**：
- SetPending → Cancel → TakePending 應為 nil
- Cancel 在無 pending 時不 panic
- client RPC 呼叫正確性

**Unit Tests（popup 跨主機切換）**：
- `pendingRequested` 旗標追蹤
- 退出時 cleanup 呼叫 CancelPendingAttach
- 跨主機選擇不再走 new-window

**Unit Tests（外層 for loop pending 檢查）**：
- local attach 返回後有 pending → 連到目標
- local attach 返回後無 pending → 回到 menu
- pending 指向無效主機 → 忽略

**Unit Tests（[i] 面板）**：
- 成功連線後 tmux option 被寫入（mock executor）
- 不在 tmux 內時不寫入

### 不測

- tmux `display-popup`（外部 CLI）
- 實際 SSH 連線（mock `sshRunFn`）
- bind block（純 tmux config）
