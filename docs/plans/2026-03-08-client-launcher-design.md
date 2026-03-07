# Client Mode Launcher 設計

## 概述

當 tsm 以 client mode 安裝時，啟動 `tsm`（無 `--remote`）應顯示 launcher 選單，讓使用者選擇三種操作模式。同時統一 tab 風格、修正 setup 符號語意、client mode 不詢問 daemon 重啟。

## 變更範圍

### 1. Launcher TUI (`internal/launcher/`)

新建 Bubble Tea model，顯示三個水平 tab：

```
  連線遠端 │ 簡易本地端 │ 完整設定
```

- Tab 1「連線遠端」：文字輸入 SSH 主機名，placeholder 顯示上次使用的主機，Tab 鍵自動填入
- Tab 2「簡易本地端」：說明文字「直接啟動本地 tmux 管理介面（不啟用 daemon）」，Enter 確認
- Tab 3「完整設定」：說明文字「安裝完整元件後啟動（hook + bind key + daemon）」，Enter 確認

Model 回傳 `Choice` enum + `Host()` string。

### 2. Tab 風格統一

所有 tab 選擇器改為水平排列，active 高亮（selectedStyle）、inactive dim（dimStyle），以 `│` 分隔。

適用於：
- Launcher（3 tabs）
- Setup 模式選擇器（2 tabs：完整模式、純客戶端）

### 3. Setup 符號語意

| 符號 | 意義 | 條件 |
|------|------|------|
| `[x]` | 將安裝 | Checked |
| `[ ]` | 不安裝/已安裝/跳過 | 未勾選 |
| `[-]` | 已安裝，將移除 | `Installed && FullOnly && client mode` |

- `Component` 新增 `Installed bool` 欄位
- 各 `BuildComponent()` 根據既有偵測邏輯設定 `Installed`

### 4. Client 模式 setup 不問 daemon

`runSetup()` 中，若最終選擇的 mode 是 client，不設定 `daemonHint` / `restartFn`。

### 5. Config 持久化擴充

```
~/.config/tsm/
  install_mode         # 既有 (full/client)
  last_remote_host     # 新增
```

新增 `config.SaveLastRemoteHost()` / `config.LoadLastRemoteHost()`。

### 6. main.go 接線

```
runWithMode()
  ├─ install_mode == full → 現有流程
  └─ install_mode == client → launcher TUI
       ├─ ChoiceRemote → runRemote(host) + 儲存 host
       ├─ ChoiceLocal  → runTUILegacy(cfg)
       └─ ChoiceFullSetup → runSetup() → runTUI()
```
