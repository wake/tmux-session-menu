# iTerm2 拖曳上傳整合設計

## 目標

將 iterm-auto-upload 的功能整合進 tsm，解決原工具在 tsm 架構下失效的問題（foreground process 不是 ssh、OSC 1337 被 tmux 攔截、TTY 映射斷裂）。所有偵測邏輯改為查詢 tsm daemon，coprocess 只負責上傳與輸出。

## 兩種觸發機制

### 自動模式

iTerm2 `fileDropCoprocess` 觸發 → 查詢 daemon → 遠端 session + Claude Code 運行中 → scp 上傳到設定目錄 → bracketed paste 輸出遠端路徑。

其他情況（本機 session、非 Claude Code、daemon 不在）→ 輸出本地路徑（原始行為）。

### 上傳模式

TUI 中按 Shift+U → 進入 Upload Modal → 使用者拖曳檔案到 iTerm2 → 無論遠端/本機都上傳（scp/cp）到該 session 當前工作目錄 → TUI 即時顯示結果累積列表 → 支援多次拖曳 → Esc 關閉。

## 資料流

```
┌─ iTerm2 ─────────────────────────────────────┐
│  fileDropCoprocess = "tsm iterm-coprocess"    │
│  使用者拖曳檔案 → 觸發 coprocess              │
└──────────────┬────────────────────────────────┘
               │ exec
               ▼
┌─ tsm iterm-coprocess ────────────────────────┐
│  1. Dial daemon (unix socket)                 │
│  2. GetUploadTarget()                         │
│  3. 判斷回傳結果：                             │
│     ├─ upload_mode=true                       │
│     │   → scp/cp 到 session 工作目錄          │
│     │   → ReportUploadResult() 回報 TUI       │
│     │   → stdout: 無輸出                      │
│     ├─ remote + claude_active                 │
│     │   → scp 到設定目錄                       │
│     │   → stdout: 遠端路徑 (bracketed paste)   │
│     └─ 其他 / daemon 不在                      │
│         → stdout: 本地路徑                     │
│  4. 失敗處理：                                 │
│     ├─ 自動模式：fallback 本地路徑 + 通知 daemon│
│     └─ 上傳模式：ReportUploadResult(error)     │
└──────────────────────────────────────────────┘

┌─ daemon ─────────────────────────────────────┐
│  新增 RPC：                                    │
│  • GetUploadTarget()                          │
│  • SetUploadMode(on/off, session)             │
│  • ReportUploadResult(files, error)           │
│                                               │
│  已有資訊：                                    │
│  • hostmgr → SSH target                       │
│  • hook status / detector → Claude Code 狀態   │
│  • tmux pane_current_path → 工作目錄           │
│  • config → upload_path 設定                   │
└──────────────────────────────────────────────┘

┌─ TUI Upload Modal ───────────────────────────┐
│  Shift+U → SetUploadMode(on) → 等待拖曳       │
│  Watch stream 接收 UploadEvent → 更新列表      │
│  Esc → SetUploadMode(off) → 回到主畫面         │
└──────────────────────────────────────────────┘
```

## gRPC Proto 新增

```protobuf
message GetUploadTargetRequest {}

message GetUploadTargetResponse {
  bool upload_mode = 1;
  bool is_remote = 2;
  bool is_claude_active = 3;
  string ssh_target = 4;
  int32 ssh_port = 5;
  string upload_path = 6;
  string session_name = 7;
}

message SetUploadModeRequest {
  bool enabled = 1;
  string session_name = 2;
}
message SetUploadModeResponse {}

message ReportUploadResultRequest {
  string session_name = 1;
  repeated UploadedFile files = 2;
  string error = 3;
}
message ReportUploadResultResponse {}

message UploadedFile {
  string local_path = 1;
  string remote_path = 2;
  int64 size_bytes = 3;
}

message UploadEvent {
  repeated UploadedFile files = 1;
  string error = 2;
  int64 timestamp = 3;
}
```

在 `StateSnapshot` 新增 `repeated UploadEvent upload_events` 欄位。

在 `SessionManager` service 新增三個 RPC：
- `GetUploadTarget(GetUploadTargetRequest) returns (GetUploadTargetResponse)`
- `SetUploadMode(SetUploadModeRequest) returns (SetUploadModeResponse)`
- `ReportUploadResult(ReportUploadResultRequest) returns (ReportUploadResultResponse)`

## `tsm iterm-coprocess` 子命令

新增 `internal/upload/coprocess.go`，作為 `tsm iterm-coprocess` 子命令的實作。

流程：
1. 連線 daemon，失敗 → 輸出本地路徑
2. `GetUploadTarget()` 取得判斷結果
3. `upload_mode=true` → scp/cp 到工作目錄 → `ReportUploadResult()` → 不輸出
4. `is_remote && is_claude_active` → scp 到設定目錄 → 輸出遠端路徑（bracketed paste）
5. 其他 → 輸出本地路徑

上傳實作：
- 遠端：`ssh mkdir -p <path>` + `scp` 上傳
- 本機：`cp` 複製

iTerm2 fileDropCoprocess 設定值：`tsm iterm-coprocess \(filenames)`

## TUI Upload Modal

`internal/ui/upload_modal.go`，是 `ui.Model` 的子元件。

畫面：
```
┌─ 上傳模式 ─────────────────────────────────┐
│                                             │
│  目標: air-2019 ~/projects/myapp            │
│  (遠端 session: myapp)                      │
│                                             │
│  拖曳檔案到 iTerm2 視窗即可上傳              │
│                                             │
│  ✓ screenshot.png        1.2 MB             │
│  ✓ design.pdf            3.4 MB             │
│  ✗ huge-file.zip         上傳失敗: timeout   │
│                                             │
│                          [Esc] 關閉          │
└─────────────────────────────────────────────┘
```

- Shift+U 進入 → `SetUploadMode(on)`
- Watch stream 的 `UploadEvent` 轉為 `UploadResultMsg`
- 支援多次拖曳，累積顯示
- Esc 退出 → `SetUploadMode(off)`

## Setup 整合

`internal/setup/iterm.go`

1. 偵測 iTerm2（`defaults read com.googlecode.iterm2` 或 app bundle）
2. 有 → 顯示安裝選項
3. 同意 → `defaults write com.googlecode.iterm2 fileDropCoprocess -string 'tsm iterm-coprocess \(filenames)'`
4. 顯示手動設定指引
5. 提示重啟 iTerm2

## Config

`~/.config/tsm/config.toml` 新增：

```toml
[upload]
remote_path = "/tmp/iterm-upload"
```

`internal/config/config.go` 新增 `UploadConfig` 結構，`Default()` 預設 `/tmp/iterm-upload`。

上傳模式不使用此設定，直接用 session 的 `pane_current_path`。

## 錯誤處理

### 自動模式

上傳失敗 → `ReportUploadResult(error)` 通知 daemon → fallback 輸出本地路徑 → daemon 推送 status bar 訊息（短暫顯示後自動消失）。

### 上傳模式

上傳失敗 → `ReportUploadResult(error)` → TUI Modal 顯示失敗狀態 → 不輸出路徑 → 使用者可重新拖曳。

### Log

所有上傳操作記錄到 `~/.config/tsm/logs/upload.log`。
