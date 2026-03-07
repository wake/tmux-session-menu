# Changelog

本檔案記錄 tsm (tmux session menu) 各版本的功能更替。格式基於 [Keep a Changelog](https://keepachangelog.com/)。

## [0.19.0] - 2026-03-08

### Changed
- Tab 選擇器改為背景色塊樣式（active 反色、inactive 極淡背景），取代 `│` 分隔線
- Setup/Upgrade 完成後 daemon 提示合併到結果區（無空行），Enter 預設 Y
- Daemon 提示訊息依偵測結果動態顯示「啟動」或「重新啟動」

### Fixed
- `[ ]` 項目在 client 模式下可用 Space 切換（僅 `[-]` 不可切換）

### Added
- `tsm upgrade --force` / `-f` 強制升級（即使版本相同）

## [0.18.0] - 2026-03-08

### Added
- Client mode launcher TUI：client 模式啟動時顯示三分頁選擇器（連線遠端 / 簡易本地端 / 完整設定）
- 水平 tab 選擇器統一風格，active 高亮、inactive dim
- Setup 元件符號語意修正：`[x]` 將安裝、`[ ]` 不安裝/跳過、`[-]` 已安裝將移除
- 記住最後連線的遠端主機名，Tab 鍵自動填入
- `config.SaveLastRemoteHost()` / `config.LoadLastRemoteHost()` 持久化

### Changed
- Client 模式 setup 不再詢問 daemon 啟動/重啟
- Setup 元件新增 `Installed` 欄位，各 `BuildComponent()` 自動偵測

## [0.17.0] - 2026-03-07

### Added
- Setup/Upgrade TUI 新增安裝模式選擇器（完整模式 / 純客戶端），左右切換
- 安裝模式持久化至 `~/.config/tsm/install_mode`

### Changed
- Popup 模式內容增加左右 padding
- tmux 模式下移除 `q` 鍵退出確認（直接退出）

## [0.16.1] - 2026-03-07

### Fixed
- TUI 選單模式下 Watch stream 斷線時正確觸發重連

## [0.16.0] - 2026-03-07

### Added
- SSH keepalive 心跳機制，防止連線閒置斷開
- 斷線倒數重連 UI（指數退避）
- 遠端模式支援 popup modal 顯示

### Fixed
- SSH attach 使用 login shell 解決遠端 tmux PATH 問題

## [0.15.0] - 2026-03-07

### Added
- `make publish` 一鍵發佈流程（build → tag → push → GitHub release）
- `install.sh` 安裝腳本，支援 curl 一鍵安裝
- `tsm upgrade` 自動升級指令（比對 GitHub release 版本）
- `make release` 交叉編譯（darwin/linux × arm64/amd64）

## [0.14.0] - 2026-03-07

### Added
- `tsm --remote <host>` 遠端模式
- SSH tunnel 轉發（本地 socket ↔ 遠端 tsm.sock）
- 遠端 session attach 與 exit code 分類
- 斷線自動重連 modal（指數退避 Bubble Tea model）
- `client.DialSocket()` 直連遠端 socket

## [0.11.4] - 2026-03-06

### Fixed
- 唯讀模式進入前偵測並清除殘留 `CLIENT_READONLY` 環境變數

## [0.11.3] - 2026-03-06

### Fixed
- 唯讀模式動態綁定 `C-q`，`bind install` 支援自動升級過時 block

## [0.11.2] - 2026-03-06

### Added
- 唯讀模式（`Shift+R`）使用 `select-pane -d` 限制輸入，僅允許 `Ctrl+Q` 退出

## [0.11.0] - 2026-03-06

### Added
- `Shift+Up/Down` 上下移動項目
- `Shift+R` 進入唯讀模式
- `Ctrl+E` 退出 tmux session
- tmux 內 `q` 鍵確認退出

### Fixed
- iTerm tab 名稱卡住問題

## [0.10.0] - 2026-03-06

### Added
- 雙行重命名輸入框
- 工具列改版

### Changed
- Header 清理簡化

## [0.9.5] - 2026-03-06

### Added
- Setup 完成後詢問是否重啟 daemon，取代手動提示

### Fixed
- 呼吸燈動畫更平滑，按需啟動 tick 減少無用渲染
- Cursor 背景色攔截 lipgloss `[49m]` 重置，空格填充到終端寬度

## [0.9.2] - 2026-03-06

### Fixed
- Running 狀態改用 sine 波呼吸動畫取代硬切閃爍
- Cursor 不截斷內容、工具列自動換行、header 縮短提示
- Cursor 背景色多次修正，確保延伸到行尾

## [0.9.1] - 2026-03-06

### Added
- TUI 配色與渲染全面改版 — cursor 背景色、群組行號、統一圖示、動畫閃動
- Claude Code dark mode 配色方案

### Fixed
- Cursor 背景色延伸到終端完整寬度

## [0.8.0] - 2026-03-06

### Added
- TUI 列表樣式改版 — 行號、樹狀符號、圖示前置

### Changed
- Setup TUI daemon 提示改為橘色 ⚠ 圖示

## [0.7.0] - 2026-03-06

### Added
- Daemon stop 倒數顯示
- `tsm daemon restart` 指令
- 左右箭頭改為 toggle 群組折疊

## [0.6.1] - 2026-03-06

### Added
- 群組操作增強（重命名、刪除群組）
- Session 清除自訂名稱功能

### Fixed
- `tsm daemon start` 自動 daemonize（不阻塞前景）
- Daemon stop 正確停止
- Cursor-follows-item：snapshot 更新時保持游標位置
- `StateManager.Scan()` 防抖，防止 J/K 排序閃爍

## [0.5.0] - 2026-03-06

### Added
- Version 管理（`VERSION` 檔案 + ldflags 注入）
- `tsm setup` 互動式安裝 TUI
- Binary self-install 偵測（自動安裝到 `~/.local/bin`）

### Fixed
- Setup 後自動重啟 daemon
- Daemon restart 等待舊程序結束

## [0.4.0] - 2026-03-05

### Added
- `Ctrl+Q` tmux 快捷鍵（`tsm bind install`）
- Ungrouped session 排序支援
- Sort order 正規化

## [0.3.0] - 2026-03-05

### Added
- gRPC daemon 架構，即時 session 更新
- Unix socket 通訊
- `StateManager` 循環掃描 tmux 狀態
- `WatcherHub` server streaming 推送快照

## [0.2.0] - 2026-03-05

### Added
- `PreviewLines` 設定支援
- AI model 偵測（Layer 2 pane title detection）
- UI mode 基礎架構（input + confirm 模式）
- `n` 鍵建立新 session
- `d` 鍵刪除 session（含確認）
- `r` 鍵重命名 session（SQLite custom_name）
- Popup 模式（自動偵測 + `--popup`/`--inline` flags）
- `g` 鍵建立群組
- `m` 鍵移動 session 到群組
- `/` 鍵即時搜尋過濾
- `J`/`K` 鍵重新排序群組與 session

## [0.1.0] - 2026-03-04

### Added
- Go TUI tmux session 選單（Phase 1 MVP）
- Claude Code hooks 整合（hook script + hooks manager）
- CLI `tsm hooks install/uninstall`
- 依賴注入架構（`Deps` struct）
- Session 列表載入與切換
- 三層狀態偵測（hook → title → terminal）
- 定期刷新 session 列表
- SQLite store（groups、session_meta）
- Tab 切換群組折疊
