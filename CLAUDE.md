# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 語意化版本規範

本專案採用 [Semantic Versioning 2.0.0](https://semver.org/)：

- **MAJOR**：不相容的 API 或行為變更
- **MINOR**：向下相容的新功能
- **PATCH**：向下相容的錯誤修正

### 版本號位置

- **VERSION 檔案**（專案根目錄）：單一真相來源，格式為 `x.y.z`
- 編譯時透過 Go ldflags 注入 `internal/version/version.go`
- 顯示格式：`x.y.z(commit_hash)`，未注入時為 `dev`

### 版本推進規則

每次開發或更新時需推進版本號：
- 新增功能 → 推進 MINOR
- 修正錯誤 → 推進 PATCH
- 破壞性變更 → 推進 MAJOR

### 發佈流程

1. 更新 `VERSION` 檔案中的版本號
2. 通過所有測試（`make test`）
3. 建立 git tag：`git tag v<MAJOR>.<MINOR>.<PATCH>`
4. 推送 tag：`git push origin v<MAJOR>.<MINOR>.<PATCH>`
5. 編譯：`make build`（自動從 `VERSION` 讀取版本號）

## 建置指令

- `make build` — 編譯並輸出到 `bin/tsm`（含版本 ldflags）
- `make test` — 執行所有測試（`go test ./... -v -race`）
- `make test-cover` — 產生 coverage 報告並開啟 HTML
- `make lint` — 靜態分析（`go vet ./...`）
- `make install` — 編譯並安裝到 `$GOPATH/bin`
- `make run` — 先建置再本機執行
- `make release` — 交叉編譯到 `dist/`（darwin/linux, arm64/amd64）
- `make clean` — 清理編譯產物

### 執行單一測試

```bash
go test ./internal/store/... -v -race                    # 特定套件
go test ./internal/store/... -v -race -run TestCreateGroup  # 特定函式
```

## 架構概覽

tsm 是 tmux session 管理工具，以 Bubble Tea TUI 為前端，gRPC daemon 為後端。

### 分層架構

```
cmd/tsm/main.go (CLI 入口、子命令路由)
    │
    ├─ [gRPC 模式 — 優先]
    │   client.Dial() ──→ daemon (Unix socket) ──→ tmux + SQLite
    │       │                    │
    │       │            StateManager: 循環掃描 tmux 狀態
    │       │            WatcherHub: server streaming 推送快照
    │       │
    │       └──→ ui.Model (Bubble Tea TUI)
    │             接收 SnapshotMsg → 更新畫面
    │             操作 → unary RPC → daemon 執行 → Watch 推播
    │
    ├─ [直接模式 — 降級]
    │   daemon 連線失敗時降級
    │   ui.Model 直接使用 tmux.Manager + store.Store
    │   TickMsg 定時輪詢
    │
    └─ [遠端模式]
        SSH tunnel (本地 socket ↔ 遠端 tsm.sock)
        → client.DialSocket() 直接連線
        → SSH attach 到遠端 session
        → 斷線自動重連 modal（指數退避）
```

### 關鍵設計

- **ui.Deps 依賴注入**：TUI 透過 `Deps` 結構注入 `Client`（gRPC 模式）或 `TmuxMgr`+`Store`（直接模式），兩種模式共用同一 UI
- **Watch stream**：gRPC server streaming，daemon 偵測 tmux 狀態變化後透過 WatcherHub 廣播 `StateSnapshot` 到所有客戶端
- **Setup TUI**：`internal/setup/` 是獨立的 Bubble Tea model，不依賴 `internal/ui/`
- **Config**：`~/.config/tsm/config.toml`（TOML），`config.Default()` 提供預設值
- **Store**：SQLite WAL 模式，管理 groups、session_meta、path_history 表

### Protocol Buffers

- 定義在 `api/proto/tsm/v1/tsm.proto`
- 生成碼在 `api/tsm/v1/`

## 程式風格

- Go 1.24+，提交前執行 `go fmt ./...`
- Commit 訊息採 Conventional Commits：`feat: ...`、`fix: ...`、`docs: ...`
- 中文註解（zh-tw）

## 專案架構

```
cmd/tsm/main.go          CLI 入口、子命令路由
internal/
  ai/                    AI 狀態偵測
  bind/                  tmux 快捷鍵安裝
  cfgtui/                設定 TUI（tsm config）
  client/                gRPC 客戶端
  config/                設定讀取與驗證
  daemon/                gRPC daemon 服務
  hooks/                 Claude Code hooks 整合
  hostmgr/               多主機連線管理
  launcher/              Client mode 啟動選擇器
  remote/                SSH tunnel、attach、重連
  selfinstall/           自動安裝到 PATH
  setup/                 互動式安裝 TUI（獨立 Bubble Tea model）
  store/                 資料儲存層（SQLite）
  tmux/                  tmux 偵測、解析與執行
  ui/                    主 TUI 畫面與樣式
  upgrade/               自動升級（GitHub release）
  version/               版本資訊（ldflags 注入）
api/                     Protocol Buffers 定義與生成碼
scripts/                 輔助測試腳本
```
