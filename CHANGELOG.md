# Changelog

本檔案記錄 tsm (tmux session menu) 各版本的功能更替。格式基於 [Keep a Changelog](https://keepachangelog.com/)。

## [0.25.1] - 2026-03-09

### Fixed
- 多主機模式啟動時 UI 空白：`Init()` 改為立即送出當前快照，TUI 第一幀即顯示連線中主機名稱與「連線中...」狀態

## [0.25.0] - 2026-03-09

### Added
- `Ctrl+U` TUI 內升級：版本檢查 → 確認 → 下載 → daemon 重啟 → 無縫 exec 重啟
- TUI 版本偵測重啟：若磁碟上 binary 已更新但 TUI 仍為舊版，提供直接重啟（免重複下載）
- `selfinstall.Detect()` 偵測已安裝 binary 版本
- `upgrade.ExtractSemver()` 從含 commit hash 的版本字串中提取 semver
- Toolbar 顯示 `Ctrl+U` 升級提示

### Fixed
- 主機停用時錯誤顯示為「斷線」：watchLoop 加入世代檢查，`stop()` 設 `HostDisabled` 後不被覆蓋為 `HostDisconnected`
- `Ctrl+E` 退出嵌套 tsm menu 時不再 detach 整個 tmux，僅退出當前 TUI 層
- Shell（非 tmux）模式按 `c` 開啟 config 後正確返回主選單（三種入口均加入 for 迴圈）
- 子選單（主機管理、新建 session）按 `q`/`Esc` 返回主選單，不再直接退出 tsm
- `ModeConfirm` 新增 `confirmReturnMode`，從子選單進入確認對話框後正確返回父模式

## [0.24.0] - 2026-03-09

### Added
- In-TUI 升級功能（`Ctrl+U`）
  - `CheckUpgradeMsg`：從 GitHub releases API 檢查最新版本
  - `ConfirmUpgradeMsg` / `DownloadUpgradeMsg`：確認後下載、替換 binary
  - `Deps.Upgrader` 依賴注入，支援測試 mock
  - `PostUpgradeOps`：TUI 退出後執行 daemon 重啟 + `syscall.Exec()` 無縫切換
- Toolbar 動態顯示升級狀態（檢查中/下載中/可升級提示）

### Changed
- 移除非必要檔案、停止追蹤編譯產物
- 移除死碼、去重、path_history 防膨脹

## [0.23.1] - 2026-03-09

### Fixed
- 多主機模式 attach 斷線改為重連 popup + 自動 re-attach（取代直接回選單）
- 斷線狀態顯示改為友善訊息「連線中斷，重連中...」（取代原始 gRPC 錯誤）
- ReconnectModel 在 StateAskContinue 時忽略 StateConnecting 訊息，防止 60 秒超時提示被覆蓋

## [0.23.0] - 2026-03-09

### Added
- `ModeNewSession` 多欄位表單（名稱、啟動路徑、最近路徑、agent 選擇）
- `CreateSession` 支援 command 參數，可直接啟動 agent 指令
- `path_history` 表記錄路徑使用歷史
- `AgentEntry` 結構支援 config.toml 中定義 agent 預設清單
- 主選單按 `c` 鍵開啟 config TUI
- host 管理操作（啟用/停用/排序/新增/刪除）持久化到 config.toml

### Fixed
- status bar 自訂名稱改用 per-session tmux user option，修正跨 session 名稱污染
- TUI 啟動時主動套用 status-left 格式
- 多主機模式 disabled 主機不顯示、ctrl+e 退出修正、popup 選單迴圈
- 移除 cfgtui 分頁箭頭提示符號

## [0.22.0] - 2026-03-08

### Added
- 多主機混合模式（`--remote` 支援多個主機同時連線）
- `HostManager` 快照聚合與多主機連線管理
- Host 連線生命週期與 Watch goroutine（指數退避自動重連）
- `h` 鍵開啟主機管理面板（啟用/停用/排序/新增/刪除）
- `ListItem` 擴展 `ItemHostTitle` 與 `FlattenMultiHost` 多主機列表渲染
- `MergeHosts` 整合 config.toml 與 `--remote` 參數
- `HostEntry` 結構與 `Config.Hosts` 主機清單

### Fixed
- 同名 session 識別、Remove 清理、goroutine 世代保護
- collectGroups 多主機過濾同一主機群組 + 通道關閉保護

## [0.21.2] - 2026-03-08

### Fixed
- 遠端模式斷線重連 race condition：goroutine 先 Send 再寫 channel，TUI 可能在寫入前退出導致 client 遺失
- 重連後 re-attach 失敗時直接回選單，改為再次進入重連流程

## [0.21.1] - 2026-03-08

### Fixed
- `tsm upgrade` 下載新版後 setup 未實際更新 binary（StatusOutdated 誤設 `Installed: true` 導致預設 ActionKeep 跳過安裝）

## [0.21.0] - 2026-03-08

### Added
- `tsm config` TUI 設定指令，雙分頁（客戶端/伺服器）編輯顏色與參數
- `tsm status-name` 子指令，供 tmux status bar 動態顯示 session 自訂名稱
- Config TOML 新增 `[local]`/`[remote]` 顏色設定區段（bar_bg、badge_bg、badge_fg）
- tmux status bar 主題化：local/remote 模式不同背景色與 badge 色
- gRPC `GetConfig`/`SetConfig` RPCs 支援遠端設定編輯
- `config.SaveConfig()` 將設定寫入 TOML 檔案
- `store.GetCustomName()` 查詢單一 session 自訂名稱

### Changed
- `switchToSession` 切換時自動套用 local status bar 樣式
- `remote.Attach` 前後切換 remote/local status bar 樣式

## [0.20.1] - 2026-03-08

### Fixed
- ActionKeep 狀態 `[x]` 僅 checkbox 淡灰，label 維持正常亮度以增加對比

## [0.20.0] - 2026-03-08

### Changed
- 已安裝元件符號與操作改為三態循環：
  - `[x]`（淡灰）— 已安裝，不重複安裝（預設）
  - `[x]`（一般白）— 覆蓋安裝
  - `[-]` — 移除安裝
- Space 鍵在已安裝元件上循環切換三個狀態
- 內部資料模型從 `checked []bool` 重構為 `actions []ComponentAction`

## [0.19.1] - 2026-03-08

### Fixed
- Claude Code hooks 元件缺少寫入路徑說明（新增 Note 顯示 settings.json 路徑）
- 主 TUI 與 setup TUI 左側增加 1 格內距，避免內容貼齊邊框

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
