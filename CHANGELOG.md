# Changelog

本檔案記錄 tsm (tmux session menu) 各版本的功能更替。格式基於 [Keep a Changelog](https://keepachangelog.com/)。

## [0.40.0] - 2026-03-14

### Added
- Hub 模式斷線重連 modal：遠端 session 斷線時顯示重連介面（倒數 60 秒、自動重試），取代直接回到選單
- 重連成功後自動 re-attach 同一 session，維持 iTerm tab 對應關係
- 支援 local daemon（含 auto-start）與 hub-socket 兩種重連路徑
- 提取可測試的 `reconnLoop` 重試核心邏輯

## [0.39.0] - 2026-03-14

### Fixed
- Hub 模式主機自動同步：hub 連線的遠端主機自動加入客戶端 config，不再只顯示 local
- Hub 模式啟用/停用/封存即時反映在 session 列表（不需等待下次 hub 推播）
- Hub-socket 模式路由修正：config 中有 host entry 時不再跳過 resolveHubHost 判斷

## [0.38.0] - 2026-03-14

### Fixed
- Hub 模式 `[h]` 主機管理面板從唯讀改為完整編輯：支援顏色設定、啟用/停用、封存/解封存、排序、新增主機
- Hub 模式主機管理操作全部在 client 端 config 完成，不依賴 HostManager

## [0.37.0] - 2026-03-14

### Added
- 主機管理三態模型：啟用/停用/封存（軟刪除），按 `[n]` 新增、`[d]` 封存、`space` 啟停切換
- 主機管理右側設定面板：`Enter`/`→` 展開，可編輯 bar_bg、bar_fg、badge_bg、badge_fg 四色
- Per-host 顏色設定：每台主機獨立儲存四色於 `[[hosts]]` 條目，取代舊版全域 `[remote]`
- 顏色預覽區塊：編輯顏色時即時模擬 status bar 外觀（badge + bar 色塊）
- `Ctrl+S` 一次儲存全部主機設定，當前 attach 主機立即套用 status bar
- `HostEntry.ToColorConfig()` 輔助方法：BadgeBG 為空時 fallback 到 Color
- `StatusBarArgs` 支援 bar_fg：`status-style` 同時設定 bg/fg
- Config TUI 新增 `local.bar_fg` 欄位
- Session 列表依 enabled/archived 過濾（含 hub 模式 client 端過濾）
- `MergeHosts` 支援 archived：封存主機不被 `--host` flag 意外啟用，新增同名自動解封存

### Changed
- `Config.Remote` 改為 `*ColorConfig`（已棄用），遷移後為 nil，`SaveConfig` 不再寫出 `[remote]` 區段
- `LoadFromString` 自動執行 `MigrateRemoteToHosts`：舊版 `[remote]` 顏色一次性遷移到各主機
- `applyHostBar` 改讀 per-host 四色，移除 `cfg.Remote` fallback
- Config TUI 移除所有 `remote.*` 欄位
- Daemon `GetConfig`/`SetConfig` 移除 `remote.*`、補齊 `local.bar_fg`

## [0.36.1] - 2026-03-14

### Fixed
- Status bar 顏色殘留：`bar_bg` 為空時改為發出 `set-option -gu status-style`（unset global），清除 remote attach 殘留的背景色
- Hub 模式 config TUI 返回後 session 消失：重建 `WatchMultiHost` stream，避免舊 recv goroutine 搶讀導致新 TUI 收不到 snapshot

## [0.36.0] - 2026-03-14

### Added
- Hub 模式 New Session 完整支援：spoke 端按 [n] 可看到主機 tab（從 WatchMultiHost 快照取得）、選擇目標主機建立 session
- `ProxyMutationRequest` proto 新增 `path` 與 `command` 欄位，CREATE_SESSION 代理操作可攜帶啟動路徑與 agent 指令
- Hub 模式建立 session 走 `ProxyMutation` 路由到正確的目標主機，不再誤建在 hub 本機

### Fixed
- Hub 模式按 [n] 時主機 tab 不顯示的問題（HostMgr 在 hub mode 為 nil，改從 hubHostSnap 提取）
- Hub 模式 `dispatchRemoteMutation` 與 `localMutationFn` 的 CREATE_SESSION 現在正確傳遞 path/command（原本固定為空字串）

## [0.35.1] - 2026-03-14

### Fixed
- Hook 狀態過期後的 AI 偵測增強：新增 `claudeStatusPattern` 匹配 Claude Code status bar 顯示格式（`[Opus 4.6]`、`[Sonnet 4.6]`、`[Haiku 4.5]`），即使 hook TTL 過期也能正確辨識 AI session
- Hook script 支援 Notification 子類型：從 stdin JSON 提取 `notification_type`，依子類型（permission_prompt → waiting、idle_prompt → idle、auth_success → running）分別對應狀態

### Changed
- `HookStatus` 新增 `NotificationType` 欄位，status JSON 輸出包含 `notification_type`

## [0.35.0] - 2026-03-13

### Added
- 新建 session 表單欄位與重命名對話框一致：名稱（CustomName）+ ID（tmux session name），ID 留空時自動使用名稱
- 新建 session 後自動掛載：建立完成後直接 switch-client 進入該 session
- 多主機模式下新建 session 時，表單頂部顯示主機 tab 選擇器（←→ / Tab 切換）

### Fixed
- 直接模式 AI 燈號偵測修正：補上與 daemon 一致的 content-based 降級偵測，無 hook 時仍能從 pane content 辨識 AI agent 並顯示實心 ●

## [0.34.0] - 2026-03-13

### Added
- Hub-socket 模式 session 切換：spoke 端透過 reverse tunnel 連線到 hub 後，可正確切換到 hub 上的遠端 session（SSH attach）或本機 session（tmux switch）
- `HostState` proto 新增 `address` 欄位，hub daemon 在 MultiHostSnapshot 中傳送各主機的 SSH 地址
- Hub daemon 建立 reverse tunnel 時自動設定 `@tsm_hub_host`（hub SSH 地址）和 `@tsm_hub_self`（spoke 的 host ID），spoke 端據此判斷 local vs remote session
- Bind block 自動升級：TUI 啟動時偵測已安裝的 bind block 版本，過舊則自動更新
- Bind block 改用 `if-shell` 取代 `run-shell`，修正 `display-popup` 無 client context 導致 "returned 9" 錯誤

### Fixed
- `main()` 路由修正：`--hub-socket` 作為首個參數時不再錯誤退出

## [0.32.1] - 2026-03-13

### Fixed
- 燈號 content-based 降級偵測：hook 無法提供 `ai_type` 時，從 pane content 偵測 Claude Code 存在 → 實心 ●
- 有效 hook 且 `ai_type` 為空時不觸發 content fallback（尊重 hook 的明確「非 AI」判斷）
- Hub mode reverse tunnel 接線：`makeHubDialFn` 使用 `WithReverse` 建立 reverse tunnel，並呼叫 `SetHubSocket`/`ClearHubSocket` 管理遠端 `@tsm_hub_socket`
- `SetHubSocket` 失敗時記錄 warn 日誌而非靜默忽略

## [0.32.0] - 2026-03-12

### Added
- AI presence 與 activity 分離：新增 `ai_type` 欄位區分「AI agent 是否存在」與「AI 正在做什麼」
- Hook status JSON 新增 `ai_type` 欄位，由 `SessionStart`/`SessionEnd` 生命週期控制，不受 2 分鐘 TTL 限制
- `HasStrongAiPresence()` 降級驗證：hook 過期時透過 pane content 二次確認 AI 是否仍在
- TUI 燈號改為 `ai_type` 驅動：有 AI → 實心 ●（顏色由 status 決定），無 AI → 空心 ○

### Changed
- `GetUploadTarget` 改用 `ai_type == "claude"` 判斷上傳觸發，取代原本的 status-based 判斷
- 解決 Claude Code idle 時 iTerm2 拖放上傳無法觸發的問題

## [0.31.0] - 2026-03-12

### Added
- `tsm daemon status` 顯示 daemon 版本號：`daemon running (pid=xxx, version=0.31.0, socket=...)`
- Daemon 啟動時寫入 `tsm.version` 檔案，結束時自動清理
- 向下相容舊版 daemon（無 version 檔案時正常顯示，不報錯）

## [0.30.1] - 2026-03-12

### Fixed
- Hub 模式下按 `h` 鍵可開啟主機狀態面板（唯讀），顯示各主機連線狀態
- Hub 模式下選取 session 不再無聲退出：正確執行 local switch 或 remote SSH attach
- Hub 模式 toolbar 現在顯示 `[h] 主機管理` 快捷鍵提示

## [0.30.0] - 2026-03-12

### Added
- Daemon Hub 模式遠端主機連線：HubManager 現在會透過 SSH tunnel + gRPC 主動連線到遠端 daemon，接收即時快照並廣播給 TUI 客戶端
- `HubRemoteClient` / `HubDialFn` 介面，支援依賴注入與測試

### Fixed
- `tsm --host mlab` 從遠端機器執行時不再顯示空白畫面：daemon hub 模式之前缺少遠端連線邏輯，所有遠端主機永遠停在 DISABLED 狀態

## [0.29.4] - 2026-03-12

### Fixed
- Hub 模式 TUI 在 daemon 重啟後不再陷入無限重連迴圈：偵測到 `not in hub mode` 永久性錯誤時自動降級到 HostManager 路徑

## [0.29.2] - 2026-03-12

### Fixed
- Popup 內切換 session 後選單不會自動關閉：bindBlock 的 `display-popup` 命令加入 `TSM_IN_POPUP=1` 環境變數
- 用戶執行 `tsm bind install` 或 `tsm setup` 後 keybinding 會自動升級

## [0.29.1] - 2026-03-12

### Fixed
- Hub 模式 `localMutationFn` 設定：`ProxyMutation` 現在能正確操作本機 session
- Hub 模式 TUI 的 kill/rename/move 操作改走 `ProxyMutation` 路由到正確的目標主機
- `dispatchRemoteMutation` 的 `GroupId` 解析錯誤改為回傳明確錯誤
- `WatchMultiHost` context done 改回傳 context error（與 `Watch` 一致）
- Hub 模式 stream 斷線加入 2 秒延遲重連（避免 TUI 凍結）
- Hub 模式 `Deps` 加入 `Upgrader`（恢復 Ctrl+U 升級功能）

## [0.29.0] - 2026-03-12

### Added
- Daemon Hub 模式：多主機聚合邏輯從 TUI HostManager 移入 daemon 層，所有主機按 Ctrl+Q 看到一致的多主機視圖
- `WatchMultiHost` gRPC streaming RPC，daemon 聚合所有主機快照後推送 `MultiHostSnapshot`
- `ProxyMutation` RPC，遠端 TUI 的操作（kill/create/rename/move session）路由到正確的目標 daemon
- SSH reverse tunnel (`-R`) 支援，讓遠端主機的 Ctrl+Q 能連回 hub daemon
- `--hub-socket` CLI flag，遠端 TUI 透過 reverse tunnel socket 連線到 hub
- Ctrl+Q 自動偵測 `@tsm_hub_socket` tmux user option，存在時走 hub 模式
- `HubManager` 聚合引擎 + `MultiHostHub` 廣播器
- 不支援 hub 的舊版 daemon 自動降級到 HostManager 路徑

## [0.28.3] - 2026-03-12

### Fixed
- Ctrl+Q 動態同步啟動模式：tsm 啟動時將 host 旗標寫入 tmux `@tsm_popup_args`，Ctrl+Q 讀取後重現相同模式
- 取代 v0.28.2 的靜態 `--host` 寫法，`tsm` 啟動為 local-only、`tsm --host` 啟動為 host mode，Ctrl+Q 皆能正確對應

## [0.28.2] - 2026-03-12

### Fixed
- Ctrl+Q 快捷鍵加入 `--host` 旗標，沿用 config 中已儲存的主機設定，不再強制 local-only

## [0.28.1] - 2026-03-11

### Fixed
- 新增 `config.EnsureLocal()` 防禦性確保主機清單中永遠包含 local，避免 config 缺漏導致本機消失
- 修正 `tsm --host` 裸用語意：直接讀取 config 中的 enabled 狀態，不再強制啟用所有主機

## [0.28.0] - 2026-03-11

### Added
- 多主機同步升級：ctrl+u 顯示升級面板，列出所有主機版本，可勾選批次升級
- `tsm upgrade --silent` 非互動升級模式，供遠端 SSH 呼叫
- `DaemonStatusResponse.version` gRPC 欄位，支援查詢遠端 daemon 版本
- ModeUpgrade TUI 面板：即時顯示遠端升級進度，支援重試失敗項目
- 底部 tab 按鈕列（activeTabStyle / inactiveTabStyle），可鍵盤導航

## [0.27.1] - 2026-03-11

### Fixed
- iTerm2 fileDropCoprocess 在升級/安裝時誤寫暫存路徑，改用 `exec.LookPath` 取得已安裝路徑
- 移除 iTerm2 coprocess 設定後「需重啟 iTerm2 生效」的多餘說明

## [0.27.0] - 2026-03-11

### Added
- 統一 HostManager 啟動路徑：所有模式（單機、多主機）皆走 HostManager，`[h]` 主機管理永遠可用
- `--host` 旗標：`tsm --host` 讀取設定啟動所有已啟用主機；`tsm --host <name>` 指定主機並覆寫設定
- `--local` 旗標：僅啟動本地端並覆寫設定，可搭配 `--host` 使用（如 `tsm --local --host air-2019`）
- `MergeHosts` 新增 `localFlag` 參數，支援 `--local` 語意
- `FlattenMultiHost` 智慧標題：只有 local 單獨啟用時隱藏主機標題，多主機時全部顯示
- `[U]` 上傳功能支援 HostManager 模式，`MultiHostSnapshotMsg` 聚合多主機上傳事件

### Changed
- `--remote` 旗標移除，改用 `--host`（**Breaking Change**）
- `tsm` 無參數啟動時強制 local-only（不讀 config 中其他 enabled hosts），`[h]` 可用
- `MergeHosts` 語意變更：有旗標時未指定的主機一律停用（舊版保持原設定）
- 合併 `runTUI` / `runMultiHost` / `runTUIWithClient` 為統一入口，移除 `runTUILegacy`

### Fixed
- `--inline --host dev` 混合旗標無法辨識的問題（改用 `hasHostMode()` 全域掃描）
- Popup 模式正確轉發 `--host` / `--local` 旗標到子程序

## [0.26.1] - 2026-03-10

### Fixed
- 重連後清空 stdin 緩衝區，避免 iTerm2 XTVERSION DCS 回應（`P>|iTerm2 x.y.z`）洩漏到遠端 shell 輸入行

## [0.26.0] - 2026-03-10

### Added
- iTerm2 拖曳上傳整合：`tsm iterm-coprocess` 子命令作為 fileDropCoprocess 處理器
- 上傳模式（Shift+U）：TUI 中開啟 Upload Modal，拖曳檔案到 iTerm2 即可上傳到 session 工作目錄
- 自動上傳模式：遠端 session + Claude Code 運行中 → scp 上傳後 bracketed paste 輸出遠端路徑
- `GetUploadTarget`、`SetUploadMode`、`ReportUploadResult` 三個新 gRPC RPC
- `UploadState` daemon 端上傳狀態追蹤，Watch stream 推送 `UploadEvent`
- `internal/upload/` 套件：`CopyLocal`、`ScpUpload`、`FormatBracketedPaste`、`ParseFilenames`
- `internal/setup/iterm.go`：iTerm2 偵測與 coprocess 自動安裝
- `PaneCurrentPath` tmux 方法取得當前 pane 工作目錄
- `UploadConfig` 設定結構（預設 remote_path: `/tmp/iterm-upload`）

### Fixed
- `DrainEvents` 改為僅在上傳模式啟用時才排出事件，避免事件在非上傳畫面被消耗而遺失
- SSH 命令注入修正：`ScpUpload` 對目標路徑使用 shell 單引號跳脫
- `copyFile` 明確關閉檔案並捕獲錯誤（取代 defer）
- Upload Modal 僅限 gRPC 模式（非多主機模式）可觸發

## [0.25.2] - 2026-03-09

### Fixed
- `ctrl+e` 在遠端模式下正確退出到筆電 shell

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
