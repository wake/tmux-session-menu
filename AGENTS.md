# Repository Guidelines

## 專案結構與模組劃分
- `cmd/tsm/main.go`：CLI 入口，子命令路由（含 `setup`、`bind`、`hooks`、`daemon`）。
- `internal/`：核心程式碼（僅供本專案使用）。
- `internal/version/`：版本資訊，透過 ldflags 編譯時注入。
- `internal/setup/`：互動式安裝 TUI（獨立 Bubble Tea model，不依賴 `ui` 套件）。
- `internal/tmux/`：tmux 偵測、解析與執行邏輯。
- `internal/ui/`：主 Bubble Tea TUI 畫面與樣式。
- `internal/config/`：設定讀取與驗證。
- `internal/store/`：資料儲存層（SQLite）。
- `internal/bind/`：tmux Ctrl+Q 快捷鍵安裝與卸載。
- `internal/hooks/`：Claude Code hooks 整合。
- `internal/daemon/`：gRPC daemon 服務。
- `internal/client/`：gRPC 客戶端。
- `internal/ai/`：AI 狀態偵測輔助邏輯。
- `api/`：Protocol Buffers 定義與生成碼。
- `docs/plans/`：設計與實作計畫；`docs/archived/`：封存文件。

## 建置、測試與開發指令
- `make build`：編譯 `./cmd/tsm` 並輸出 `bin/tsm`（含版本 ldflags）。
- `make install`：編譯並安裝到 `$GOPATH/bin/tsm`。
- `make uninstall`：移除 `$GOPATH/bin/tsm`。
- `make run`：先建置再本機執行。
- `make test`：執行 `go test ./... -v -race`。
- `make test-cover`：產生 `coverage.out` 並開啟 coverage HTML。
- `make lint`：執行 `go vet ./...`。
- `make clean`：移除 `bin/tsm` 與 `coverage.out`。

## 程式風格與命名
- 使用 Go 1.24.x；提交前請執行 `gofmt`（或 `go fmt ./...`）。
- 縮排使用 tab（遵循 Go 慣例），避免手動對齊空白。
- 套件名稱簡短小寫（如 `tmux`、`ui`）；檔名以職責命名（如 `resolver.go`）。
- 測試檔命名採 `*_test.go`，測試函式使用 `TestXxx`。

## 測試規範
- 新功能與修正需附對應單元測試，優先放在同套件的 `*_test.go`。
- 至少執行 `make test`，確保 race 檢查通過。
- 變更核心流程（`internal/tmux`、`internal/ui`、`internal/store`）時，建議再跑 `make test-cover` 檢查覆蓋率變化。

## Commit 與 Pull Request 規範
- Commit 訊息建議採 Conventional Commits：`feat: ...`、`fix: ...`、`docs: ...`。
- 一個 commit 聚焦一件事，訊息主詞清楚描述行為與範圍。
- PR 請包含：變更摘要、測試結果（貼上執行指令）、關聯 issue（例如 `#1`）。
- 若有 UI 變更，附上終端截圖或操作說明。

## 設定與安全
- 本工具依賴 tmux 執行環境；請先確認本機可用 `tmux`。
- 不要提交本機機密或環境特定設定；必要時以文件說明設定方式，不直接寫入程式碼。
