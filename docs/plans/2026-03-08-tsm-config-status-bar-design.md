# tsm config + tmux status bar 主題化設計

日期：2026-03-08

## 概述

新增 `tsm config` TUI 指令，以及 tmux status bar 主題化功能，讓 local 與 remote 模式的 tmux bar 顏色可區分，並在 status bar 顯示 session 自訂名稱。

## 設計

### 1. Config TOML 結構

在現有 `~/.config/tsm/config.toml` 新增顏色區段：

```toml
preview_lines = 150
poll_interval_sec = 2

[local]
bar_bg = ""           # 空字串 = 保持 tmux 預設（綠色）
badge_bg = "#5f8787"  # session 自訂名稱標籤背景
badge_fg = "#c0caf5"  # session 自訂名稱標籤前景

[remote]
bar_bg = "#1a2b2b"    # 青色調深色背景
badge_bg = "#73daca"  # 青色標籤背景
badge_fg = "#1a1b26"  # 深色標籤前景
```

### 2. Config TUI（`tsm config`）

兩個分頁：

- **客戶端**：local/remote 顏色設定（bar_bg、badge_bg、badge_fg）
- **伺服器 (hostname)**：preview_lines、poll_interval_sec、伺服器的顏色設定

分頁標題依連線狀態動態顯示 hostname，例如「伺服器 (air-2019)」。

操作方式：
- `←` `→` 切換分頁
- `↑` `↓` / `j` `k` 移動欄位
- `Enter` 進入編輯、確認
- `Del` 重置欄位為預設值
- `q` / `Esc` 離開

### 3. tmux Status Bar 佈局

```
[custom_name] [session-id] window-list              right-side-info
```

- `custom_name`：從 SQLite session_meta 讀取，由 `tsm status-name` 子指令提供
- 背景色依據 local/remote 設定使用不同顏色

### 4. `tsm status-name` 子指令

輕量級指令，tmux 透過 `#(tsm status-name)` 在 status-left 呼叫：
- 讀取當前 tmux session 的 custom_name（來自 SQLite）
- 有自訂名稱時輸出名稱，無則輸出空字串
- 必須快速回應（< 50ms），避免阻塞 tmux 渲染

### 5. Status Bar 套用時機

- **本地模式**：`switchToSession` 時套用 `[local]` 顏色
- **遠端模式**：`remote.Attach` 時套用 `[remote]` 顏色
- 透過 `tmux set-option -g status-style` 設定背景色
- 透過 `tmux set-option -g status-left` 設定 badge 格式與顏色

### 6. 遠端 Config 編輯

新增 gRPC RPCs：
- `GetConfig`：取得遠端伺服器的 config
- `SetConfig`：設定遠端伺服器的 config 欄位

當 config TUI 在「伺服器」分頁時，透過 gRPC 讀寫遠端設定。

## 配色方案

| 模式 | bar_bg | badge_bg | badge_fg |
|------|--------|----------|----------|
| 本地 | （tmux 預設） | `#5f8787` | `#c0caf5` |
| 遠端 | `#1a2b2b` | `#73daca` | `#1a1b26` |
