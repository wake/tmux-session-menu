# In-TUI Upgrade 設計文件

## 概述

在 TUI 選單內觸發升級，下載新 binary 後以 `syscall.Exec()` 重啟 TUI，並在 exec 前重啟 daemon。

## 需求

| 項目 | 決定 |
|------|------|
| 觸發方式 | `ctrl+u` 快捷鍵（ModeNormal） |
| UI 呈現 | ModeConfirm 對話框 |
| 無新版本 | ModeConfirm「已是最新版本 (x.y.z)」 |
| 錯誤處理 | ModeConfirm 顯示錯誤詳情 |
| Daemon 重啟 | exec 前：停舊 daemon → 啟新 daemon |
| TUI 重啟 | `syscall.Exec()` 替換自身為新 binary |
| UI 狀態 | 不保留，全新啟動 |

## 方案選擇：兩階段分離

- **Phase 1（UI 內）**：互動 + 下載
- **Phase 2（main.go，p.Run() 返回後）**：安裝 + daemon 重啟 + exec

理由：符合現有 post-quit action 模式，職責分離乾淨。

## 資料流

```
ctrl+u (ModeNormal)
  │
  ├─ tea.Cmd: upgrader.CheckLatest()
  │    ↓
  │  checkUpgradeMsg { Release, Err }
  │    ↓
  ├─ Err ≠ nil → ModeConfirm「檢查失敗：{err}」
  ├─ !NeedsUpgrade → ModeConfirm「已是最新版本 (x.y.z)」
  └─ NeedsUpgrade → ModeConfirm「v{current} → v{new}，是否升級？」
                        │
                      [Y] 確認
                        │
                    tea.Cmd: upgrader.Download(url)
                        ↓
                    downloadUpgradeMsg { TmpPath, Err }
                        ↓
                    ├─ Err → ModeConfirm「下載失敗：{err}」
                    └─ OK → upgradeReady = true → tea.Quit
                              ↓
                        ─── p.Run() 返回 ───
                              ↓
                        main.go 偵測 UpgradeReady()
                          ├─ selfinstall.Install(tmpPath)
                          ├─ os.Remove(tmpPath)
                          ├─ daemon.Stop()
                          ├─ daemon.Start()
                          └─ syscall.Exec(installedPath, os.Args, os.Environ())
```

## UI 層變更

### 新增訊息類型

```go
type checkUpgradeMsg struct {
    Release *upgrade.Release
    Err     error
}

type downloadUpgradeMsg struct {
    TmpPath string
    Err     error
}
```

### Model 新增欄位

```go
upgradeReady   bool
upgradeTmpPath string
upgradeVersion string
```

### Deps 新增

```go
type Deps struct {
    // ...existing...
    Upgrader *upgrade.Upgrader // nil 時隱藏 ctrl+u
}
```

### ModeConfirm 升級場景

| 場景 | 對話框內容 | 按鍵 |
|------|-----------|------|
| 有新版 | `v0.23.1 → v0.24.0 升級？` | Y/N |
| 已最新 | `已是最新版本 (0.23.1)` | Esc 關閉 |
| 錯誤 | `檢查失敗：{err}` 或 `下載失敗：{err}` | Esc 關閉 |

### 公開方法

```go
func (m Model) UpgradeReady() bool
func (m Model) UpgradeInfo() (tmpPath, version string)
```

## Post-Quit 處理（main.go）

```go
type PostUpgradeOps struct {
    Install     func(tmpPath string) (string, error)
    RemoveTmp   func(path string) error
    StopDaemon  func() error
    StartDaemon func() error
    Exec        func(path string, args []string, env []string) error
}
```

使用函式注入，便於測試。

## 測試策略（TDD）

### Layer 1：upgrade 包（已有測試，不需新增）

### Layer 2：UI 訊息流

| 測試 | 驗證 |
|------|------|
| TestCtrlU_TriggersCheck | ctrl+u → cmd 不為 nil |
| TestCheckResult_NewVersion | 有新版 msg → ModeConfirm + 版本差異 |
| TestCheckResult_AlreadyLatest | 無需升級 → ModeConfirm「已是最新」 |
| TestCheckResult_Error | err msg → ModeConfirm 錯誤 |
| TestConfirmUpgrade_Yes | Y → download cmd |
| TestConfirmUpgrade_No | N → ModeNormal |
| TestDownloadResult_Success | 成功 → upgradeReady + tea.Quit |
| TestDownloadResult_Error | 失敗 → ModeConfirm 錯誤 |
| TestUpgradeReady_Accessors | 公開方法回傳正確值 |

### Layer 3：post-quit 整合

| 測試 | 驗證 |
|------|------|
| TestPostUpgrade_HappyPath | 全部成功 → 依序呼叫 |
| TestPostUpgrade_InstallFail | 安裝失敗 → 不 exec |
| TestPostUpgrade_DaemonStopFail | Stop 失敗 → 仍繼續 |

## 安全考量

- Binary 安裝使用 `.tmp` + `os.Rename()` 確保原子性
- 多 tab 情境：其他 TUI 的 Watch stream 不受影響（WatcherHub 隔離）
- Proto 向下相容：新增欄位用 optional
- exec 失敗時程序自然退出，使用者手動重開即可
