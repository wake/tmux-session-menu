# tsm — tmux session menu

## 語意化版本規範

本專案採用 [Semantic Versioning 2.0.0](https://semver.org/)：

- **MAJOR**：不相容的 API 或行為變更
- **MINOR**：向下相容的新功能
- **PATCH**：向下相容的錯誤修正

### 版本號位置

- **VERSION 檔案**（專案根目錄）：單一真相來源，格式為 `x.y.z`
- 編譯時透過 Go ldflags 注入 `internal/version/version.go`：

```
-X github.com/wake/tmux-session-menu/internal/version.Version=$(VERSION)
-X github.com/wake/tmux-session-menu/internal/version.Commit=$(COMMIT)
-X github.com/wake/tmux-session-menu/internal/version.Date=$(DATE)
```

- 顯示格式：`x.y.z(commit_hash)`，未注入時為 `dev`

### 發佈流程

1. 更新 `VERSION` 檔案中的版本號
2. 通過所有測試（`make test`）
3. 建立 git tag：`git tag v<MAJOR>.<MINOR>.<PATCH>`
4. 推送 tag：`git push origin v<MAJOR>.<MINOR>.<PATCH>`
5. 編譯：`make build`（自動從 `VERSION` 讀取版本號）

### 版本推進規則

每次開發或更新時需推進版本號：
- 新增功能 → 推進 MINOR
- 修正錯誤 → 推進 PATCH
- 破壞性變更 → 推進 MAJOR

## 專案架構

```
cmd/tsm/main.go          CLI 入口、子命令路由
internal/
  version/               版本資訊（ldflags 注入）
  setup/                 互動式安裝 TUI（獨立 Bubble Tea model）
  ui/                    主 TUI 畫面與樣式
  tmux/                  tmux 偵測、解析與執行
  config/                設定讀取與驗證
  store/                 資料儲存層（SQLite）
  bind/                  tmux 快捷鍵安裝
  hooks/                 Claude Code hooks 整合
  daemon/                gRPC daemon 服務
  client/                gRPC 客戶端
  ai/                    AI 狀態偵測
api/                     Protocol Buffers 定義與生成碼
docs/                    設計文件與計畫
```

## 建置指令

- `make build` — 編譯並輸出到 `bin/tsm`
- `make test` — 執行所有測試（含 race 檢查）
- `make install` — 編譯並安裝到 `$GOPATH/bin`
- `make uninstall` — 移除已安裝的 binary
- `make lint` — 靜態分析
- `make clean` — 清理編譯產物
