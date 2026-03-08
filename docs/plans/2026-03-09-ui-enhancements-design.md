# UI 增強設計文件

**日期：** 2026-03-09
**範圍：** 四項 UI 改進，按優先順序實作

---

## Task 1：移除 cfgtui 分頁箭頭提示符號

移除 `cfgtui.go` 分頁導航的 `← →` 提示符號。

**變更位置：** `internal/cfgtui/cfgtui.go:385`

---

## Task 3：Config 從主選單呼叫

主 TUI 新增 `c` 鍵綁定，按下後退出 TUI → 啟動 cfgtui → cfgtui 關閉後重啟主 TUI。

**實作方式：** 類似 `switchToSession` 的 post-TUI 處理模式。Model 新增 `openConfig` 旗標，`handlePostTUI` 檢測後啟動 config TUI 迴圈。

**變更位置：**
- `internal/ui/app.go` — 新增 `c` 鍵綁定、`OpenConfig()` method
- `internal/ui/mode.go` — 可能需要新 mode 常數
- `cmd/tsm/main.go` — `handlePostTUI` 加入 config 啟動邏輯

---

## Task 4：Host 管理持久化

### 4a. 操作持久化

`hostpicker.go` 的 AddHost/Remove/Reorder/Enable/Disable 操作後，將結果寫回 `config.toml`。

**做法：** 新增 `persistHosts` 函數，將 `HostManager.Hosts()` 轉回 `[]config.HostEntry`，透過 `config.SaveConfig` 寫入。

**變更位置：**
- `internal/ui/hostpicker.go` — 每個操作後呼叫 persistHosts
- `internal/ui/app.go` — persistHosts 實作（需存取 cfg）

### 4b. 單機模式也可管理 hosts

目前 `h` 鍵只在多主機模式（`deps.HostMgr != nil`）下可用。修改為即使單機模式也能開啟 host 管理面板，新增主機後自動切換為多主機模式。

---

## Task 2：擴充 New Session

### 表單 Layout

按 `n` 鍵後，在畫面底部展開內嵌多欄位表單：

```
Session 名稱: [____________]
啟動路徑:     [________________]
  ~/Workspace/wake/tmux-session-menu
  ~/Workspace/ark/csp-plugin
  ~/Workspace/coop/istdc

啟動 Agent
Coding   ○ Claude Code   ○ Claude (auto)
Other    ○ Codex         ○ Gemini
         ● Shell
```

### 欄位說明

1. **Session 名稱**：文字輸入，必填
2. **啟動路徑**：文字輸入，支援 tab 目錄補全，預設值為空（使用 tmux 預設）
3. **最近路徑**：列在啟動路徑下方，最近 3 筆，以選項方式呈現，space 選取後帶入輸入欄位
4. **啟動 Agent**：分群 radio button
   - 群組標題在左，選項往右橫排
   - 群組有選填標題（如 "Coding"、"Other"）
   - 預設選中 Shell
   - Shell 為內建選項，不可刪除

### 操作方式

- `↑↓`：在區塊間移動（名稱 → 路徑 → 最近路徑 → agent 群組）
- `←→`：在同區塊選項間移動
- `space`：選取路徑歷史 / agent 選項
- `tab`：路徑欄位的目錄補全
- `Enter`：建立 session
- `Esc`：取消

### 建立流程

1. 使用者填寫表單
2. 按 Enter
3. 建立 tmux session：`tmux new-session -d -s {name} -c {path}`
4. 若選擇了 agent（非 Shell），在新 session 的 pane 中執行 agent 啟動指令：`tmux send-keys -t {name} '{command}' Enter`
5. 重新載入 session 列表

### Config 結構

```toml
# config.toml 新增區段

[[agents]]
name = "Claude Code"
command = "claude"
group = "Coding"
enabled = true
sort_order = 0

[[agents]]
name = "Claude (auto)"
command = "claude --dangerously-skip-permissions"
group = "Coding"
enabled = true
sort_order = 1

[[agents]]
name = "Gemini"
command = "gemini"
group = "Other"
enabled = false
sort_order = 2
```

預設 config 只包含 Shell（內建）。使用者透過 config 自行新增 agent 預設。

### 路徑歷史

儲存位置：SQLite `path_history` 表，記錄最近使用的路徑（去重，最多保留 20 筆，顯示最近 3 筆）。

Schema：
```sql
CREATE TABLE IF NOT EXISTS path_history (
    path TEXT PRIMARY KEY,
    used_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### gRPC Protocol 擴充

```proto
message CreateSessionRequest {
  string name = 1;
  string path = 2;
  string command = 3;  // 新增：agent 啟動指令
}
```

### 變更位置

- `internal/config/config.go` — 新增 `AgentEntry` struct、`Config.Agents` 欄位
- `internal/store/store.go` — 新增 `path_history` 表的 CRUD
- `internal/ui/app.go` — 新 session 表單 UI、操作邏輯
- `internal/ui/mode.go` — 新增 ModeNewSession
- `internal/tmux/session.go` — send-keys 支援
- `api/proto/tsm/v1/tsm.proto` — CreateSessionRequest 加 command 欄位
- `internal/daemon/service.go` — CreateSession 處理 command
- `cmd/tsm/main.go` — 路徑補全邏輯
