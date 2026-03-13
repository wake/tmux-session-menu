# Host 管理功能重新設計

## 概述

擴充 `[h]` 主機管理功能：三態主機狀態（啟用/停用/封存）、右側設定面板（四色配置＋預覽）、移除 `[remote]` 配色區段改為 per-host 設定，所有設定存於 client 端 config。

## 三態主機狀態

| 狀態 | enabled | archived | session 列表 | 管理畫面 | config |
|------|---------|----------|-------------|---------|--------|
| 啟用 | true | false | 可見 | 可見 ● | 保留 |
| 停用 | false | false | 不可見 | 可見 ○ | 保留 |
| 封存 | — | true | 不可見 | 不可見 | 保留（軟刪除） |

- 封存 = 軟刪除，config 中保留 `[[hosts]]` 條目，使用者可手動刪除
- 新增同名主機時自動解封存並進入啟用狀態
- local 主機可停用、不可封存

### 管理畫面操作

| 按鍵 | 動作 |
|------|------|
| `[n]` | 新增主機（同名已封存 → 解封存＋啟用） |
| `[d]` | 封存主機（軟刪除） |
| `space` | 啟用/停用切換 |
| `q` / `Esc` | 離開管理畫面 |

### Enable/Disable 兩段式

1. **立即**：client 端過濾，TUI 馬上隱藏/顯示該主機的 sessions
2. **重啟後**：daemon 重讀 config，實際斷線/連線

非 hub 模式下 `HostMgr` 可同時做到過濾＋實際斷線。Hub 模式先以 client 過濾作為過渡，daemon 重啟後實際生效。

## Config 結構變更

### HostEntry 擴充

```toml
[[hosts]]
  name = "mlab1"
  address = "mlab1"
  color = "#e0af68"       # 主題色（UI 圓點、host title、badge_bg fallback）
  bar_bg = "#1a2b2b"
  bar_fg = ""             # 空值 = 由 tmux 自行決定
  badge_bg = "#e0af68"
  badge_fg = "#1a1b26"
  enabled = true
  archived = false
  sort_order = 0
```

### 移除 `[remote]`，保留 `[local]` 做為別名

```toml
[local]
  bar_bg = ""
  bar_fg = ""
  badge_bg = "#5f8787"
  badge_fg = "#c0caf5"
```

`[local]` 四色與 `[[hosts]]` 中 `address=""` 的 local 主機雙向同步：
- 讀取 config：`[local]` 值寫入 local host entry
- 儲存 config：local host entry 值回寫到 `[local]`

### ColorConfig 擴充

```go
type ColorConfig struct {
    BarBG   string `toml:"bar_bg"`
    BarFG   string `toml:"bar_fg"`   // 新增，空值=tmux 預設
    BadgeBG string `toml:"badge_bg"`
    BadgeFG string `toml:"badge_fg"`
}
```

### `color` 欄位保留用途

- 管理畫面 `●` 圓點顏色
- session 列表 host title 顏色
- 新增主機時自動 pick 的主題色
- `badge_bg` 為空時的 fallback

## 右側設定面板 UI

按 `Enter` 或 `→` 展開：

```
 主機管理                        │ mlab1 設定
                                 │
 ● local          ✓              │  [x] 啟用
 ● mlab1    ►     連線中...      │  bar_bg      #1a2b2b
 ○ mlab2          已停用         │  bar_fg               （留空由 tmux 自行決定）
                                 │  badge_bg    #e0af68
                                 │  badge_fg    #1a1b26
                                 │
                                 │  ┌──────────────────────┐
                                 │  │ ■ mlab1 ■  0:zsh*    │
                                 │  └──────────────────────┘
                                 │     ▲ 預覽
                                 │
                                 │  Ctrl+S 儲存並套用 | Esc 取消
```

### 操作方式

| 按鍵 | 面板外（左側） | 面板內（右側） |
|------|--------------|--------------|
| `↑/↓` | 切換主機 | 切換欄位 |
| `Enter` / `→` | 開啟面板 | 編輯當前欄位（啟用欄位則切換） |
| `←` / `Esc` | 離開管理畫面 | 關閉面板回左側 |
| `space` | 啟用/停用 | 切換 `[x] 啟用` |
| `Ctrl+S` | 儲存全部主機修改 | 儲存全部主機修改 |

### 預覽區塊

- 一行模擬 status bar：badge 用 `badge_bg/fg`，其餘用 `bar_bg/fg`
- 即時反映編輯中的色碼（每次按鍵更新）
- 色碼格式不合法時保持上一個合法狀態

### 儲存行為

`Ctrl+S`：
1. 全部主機的四色 + enabled/archived 寫入 config（不只當前面板）
2. 若當前 attach 的主機有改色 → 立即 `ApplyStatusBar`
3. local host 四色同步到 `[local]`
4. 顯示「已儲存」提示

## 顏色套用邏輯

### applyHostBar 改讀 host 四色

```
attach 遠端時：
  bar_bg   = host.bar_bg    （空值 → unset）
  bar_fg   = host.bar_fg    （空值 → 省略，由 tmux 決定）
  badge_bg = host.badge_bg   （空值 → fallback 到 host.color）
  badge_fg = host.badge_fg   （空值 → 不設）

detach 返回：
  讀取 local host 四色套用
```

不再有 `cfg.Remote` fallback。每台主機的顏色自己負責自己的。

### StatusBarArgs 支援 bar_fg

```go
// bar_bg + bar_fg 都有值
"set-option", "-g", "status-style", "bg=#1a2b2b,fg=#c0caf5"

// bar_fg 為空
"set-option", "-g", "status-style", "bg=#1a2b2b"

// bar_bg 也為空
"set-option", "-gu", "status-style"  // unset 整個
```

每次 `ApplyStatusBar` 都是替換整個 `status-style`，不會殘留。

## 向後相容與遷移

### 讀取 config 時

1. 若 `[remote]` 區段存在 → 作為預設色填入尚未設定四色的遠端 host
2. 若遠端 host 的 `badge_bg` 為空但 `color` 有值 → `badge_bg` fallback 到 `color`
3. `[local]` 值覆寫 local host entry 四色

### 儲存 config 時

1. 不再寫出 `[remote]` 區段
2. local host entry 四色回寫到 `[local]`
3. 每個 `[[hosts]]` 寫出四色 + `archived`

## Config TUI 簡化

客戶端分頁移除所有 `remote.*` 欄位，加入 `local.bar_fg`：

```
 local.bar_bg        #cccccc
 local.bar_fg                    （留空由 tmux 自行決定）
 local.badge_bg      #5f8787
 local.badge_fg      #c0caf5
```

## 影響範圍

### 需修改

| 檔案 | 變更 |
|------|------|
| `internal/config/config.go` | `HostEntry` 加四色 + `archived`、`ColorConfig` 加 `bar_fg`、移除 `[remote]`、local↔host 同步、遷移邏輯 |
| `internal/ui/hostpicker.go` | 三態操作（n/d/space/enter）、右側面板、顏色編輯、預覽、Ctrl+S 儲存、封存過濾 |
| `internal/ui/app.go` | session 列表依 enabled/archived 過濾（含 hub 模式）、移除 `cfg.Remote` 引用 |
| `internal/tmux/statusbar.go` | `StatusBarArgs` 支援 `bar_fg` |
| `cmd/tsm/main.go` | `applyHostBar` 改讀 host 四色、移除 `cfg.Remote` fallback |
| `internal/cfgtui/cfgtui.go` | 移除 `remote.*` 欄位、加 `local.bar_fg` |
| `CLAUDE.md` | 補充軟刪除設計說明 |

### 不需改

- Proto / gRPC — 不影響
- `internal/daemon/` — hub daemon 不需感知 archived / 顏色
- `internal/hostmgr/` — enable/disable 邏輯不變（非 hub 模式仍可實際斷線）

## 測試重點

- 三態切換：啟用↔停用↔封存、解封存
- Config 遷移：舊格式（有 `[remote]`）→ 新格式
- local 同步：改 `[local]` 反映到 host entry，反之亦然
- 顏色套用：per-host 四色、bar_fg 留空時的行為
- Hub 模式：client 端過濾 enabled/archived
- 預覽渲染：合法/不合法色碼
- 右側面板：開關、編輯、儲存全部主機
