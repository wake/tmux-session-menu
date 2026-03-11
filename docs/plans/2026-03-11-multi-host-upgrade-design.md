# 多主機同步升級設計

## 目標

ctrl+u 時同步升級所有主機（client + remote），升級過程 TUI 不退出，即時顯示進度。

## 架構

TUI 內全程驅動：ModeUpgrade 顯示主機清單 → 遠端以 `ssh <host> tsm upgrade --silent` 並行升級 → 本機最後升級。HostManager 重連機制自動銜接遠端 daemon 重啟。

## gRPC proto 擴充

`DaemonStatusResponse` 新增 `string version = 4`：

```protobuf
message DaemonStatusResponse {
  int32 pid = 1;
  google.protobuf.Timestamp started_at = 2;
  int32 watcher_count = 3;
  string version = 4;
}
```

- daemon 從 `version.Version` 填入
- 新 client + 舊 daemon → 空字串，TUI 顯示「未知」
- 本機版本直接用 `version.Version`，不需 RPC

## `tsm upgrade --silent` CLI

非互動升級模式：

1. 檢查 GitHub 最新版本
2. 已是最新 → stdout 印當前版號 → exit 0
3. 需要升級 → 下載 → 安裝 → 停舊 daemon → 啟新 daemon → stdout 印新版號 → exit 0
4. 失敗 → stderr 印錯誤 → exit 1

不做 `syscall.Exec`（SSH 場景不適用）。stdout 只印一行版本號供 TUI 解析。

## 版本檢查流程

ctrl+u 按下後並行取得：

1. 本機版本：`version.Version`（零成本）
2. 遠端版本：對每台已連線遠端呼叫 `DaemonStatus` RPC 取 `version`
3. 最新版本：`Upgrader.CheckLatest()` → GitHub API

全部完成後進入 ModeUpgrade。GitHub 檢查失敗 → ModeConfirm 顯示錯誤。遠端 DaemonStatus 失敗 → 該主機版本「未知」，預設勾選。

## ModeUpgrade UI

### 升級前

```
升級至 v0.28.0

  [x] local          v0.27.1
  [x] air-2019       v0.27.0
  [ ] dev-server     v0.28.0  (已是最新)
  [x] staging        未知

   ctrl+u 升級    Esc 取消
```

- 需要升級 → 預設勾選
- 已是最新 → 預設不勾、灰色「(已是最新)」
- 版本未知 → 預設勾選
- local 固定最上面
- Space 切換勾選、a 全選/全不選、上下移動游標
- 游標可移到底部按鈕，Enter 觸發 focused 按鈕
- 按鈕用 cfgtui 的 activeTabStyle / inactiveTabStyle 樣式

### 升級中

```
升級至 v0.28.0

  [ ] local          v0.27.1  等待遠端完成...
  [-] air-2019       v0.27.0  更新中...
  [ ] dev-server     v0.28.0  (已是最新)
  [-] staging        未知     更新中...

  升級進行中...   Esc 中止
```

- 遠端項目 checkbox 變 `[-]`（disabled）
- 鍵盤鎖定（僅允許 Esc 中止）
- Esc 不中斷進行中的 SSH，但取消後續排隊，local 不升級

### 升級完成

```
升級至 v0.28.0

  [x] local          v0.27.1
  [✓] air-2019       v0.28.0  已更新
  [ ] dev-server     v0.28.0  (已是最新)
  [!] staging        未知     升級失敗: connection refused

   ctrl+u 升級    Esc 取消
```

- 成功：`[✓]` + 新版號 + 「已更新」
- 失敗：`[!]` + 錯誤訊息
- 回到可操作狀態，按鈕恢復為升級前那組
- 失敗項目自動勾選，ctrl+u 重試
- local 升級成功 → TUI 退出 + exec（binary 已替換，無法避免）

## 錯誤處理

| 情境 | 行為 |
|------|------|
| 全部成功（含 local） | exec 到新版 |
| 全部成功但 local 未勾選 | 停留 ModeUpgrade，Esc 返回 |
| 部分遠端失敗，local 成功 | exec 到新版，重進後 ctrl+u 可重試 |
| 部分遠端失敗，local 未開始 | 停留 ModeUpgrade，失敗項自動勾選 |
| 升級中按 Esc | 不中斷進行中 SSH，取消後續，等完成回到可操作 |
| SSH 超時 | 60 秒超時，視為失敗 |
| 遠端無法連 GitHub | 顯示失敗，不支援本機代下載 |

## 資料流

```
ctrl+u
  ↓
CheckUpgradeMsg (並行: GitHub + DaemonStatus × N)
  ↓
ModeUpgrade 顯示清單
  ↓
ctrl+u / Enter 確認
  ↓
並行 ssh <host> tsm upgrade --silent (60s timeout)
  ↓
RemoteUpgradeMsg{HostID, NewVer/Err} 逐台更新
  ↓
遠端全部完成
  ↓
local 已勾選 → 下載 → 安裝 → 停 daemon → 啟 daemon → exec
local 未勾選 → 回到可操作狀態
```

## 新增/修改項目

| 類型 | 項目 |
|------|------|
| proto | `DaemonStatusResponse.version` |
| daemon | 填入 `version.Version` |
| CLI | `tsm upgrade --silent` |
| TUI mode | `ModeUpgrade` |
| TUI update | `updateUpgrade()` |
| TUI render | `renderUpgrade()` |
| TUI msg | `CheckUpgradeMsg` 擴充、`RemoteUpgradeMsg` |
| styles | 共用 activeTabStyle / inactiveTabStyle |

## 不需改動

- HostManager 連線/重連機制
- `Upgrader.CheckLatest()` / `Download()`
- `runPostUpgrade` 本機升級流程
