# UI 增強實作計畫

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 四項 UI 改進 — 移除箭頭提示、config 入口、host 持久化、new session 擴充

**Architecture:** 由小到大逐步實作。Task 1 是一行改動。Task 3 透過 post-TUI 機制啟動 config。Task 4 在 hostpicker 操作後寫回 config.toml。Task 2 是最大的變更，新增 ModeNewSession 多欄位表單、config agent 預設、路徑歷史。

**Tech Stack:** Go 1.24 / Bubble Tea / Lipgloss / TOML / SQLite / gRPC + protobuf

---

## Task 1：移除 cfgtui 分頁箭頭提示

**Files:**
- Modify: `internal/cfgtui/cfgtui.go:385`
- Test: `internal/cfgtui/cfgtui_test.go`（若存在）

**Step 1: 修改 cfgtui.go**

移除第 385 行的箭頭提示：

```go
// 修改前
b.WriteString("  ")
b.WriteString(dimStyle.Render("← →"))

// 修改後
// （刪除這兩行）
```

**Step 2: 編譯驗證**

Run: `make build`
Expected: 編譯成功

**Step 3: Commit**

```bash
git add internal/cfgtui/cfgtui.go
git commit -m "fix: 移除 cfgtui 分頁箭頭提示符號"
```

---

## Task 3：Config 從主選單呼叫

**Files:**
- Modify: `internal/ui/app.go` — 新增 `c` 鍵綁定和 `openConfig_` 旗標
- Modify: `internal/ui/mode.go` — （不需新 mode，用旗標即可）
- Modify: `cmd/tsm/main.go` — `handlePostTUI` 加入 config 迴圈
- Test: `internal/ui/app_test.go`

### Step 1: 寫 app.go 的失敗測試

```go
func TestModel_ConfigKey_SetsOpenConfig(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	m, _ = sendKey(m, "c")
	assert.True(t, m.(ui.Model).OpenConfig())
	assert.True(t, m.(ui.Model).Quitting())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/... -v -race -run TestModel_ConfigKey`
Expected: FAIL — `OpenConfig` 不存在

### Step 3: 實作 app.go

在 `Model` struct 新增 `openConfig_` 旗標：

```go
// Model struct 新增欄位
openConfig_ bool // c 鍵：開啟 config TUI
```

新增 public method：

```go
// OpenConfig 回傳使用者是否按了 c 要求開啟 config。
func (m Model) OpenConfig() bool {
	return m.openConfig_
}
```

在 `updateNormal` 的 switch 中，新增 `c` 鍵處理（`app.go` 約第 597 行附近，`"n"` case 之前）：

```go
case "c":
	m.openConfig_ = true
	m.quitting = true
	return m, tea.Quit
```

在 `renderToolbar` 的 line2Parts 中加入 config 提示：

```go
line2Parts = append(line2Parts, render("[c]", "設定"))
```

### Step 4: Run test to verify it passes

Run: `go test ./internal/ui/... -v -race -run TestModel_ConfigKey`
Expected: PASS

### Step 5: 實作 handlePostTUI 的 config 迴圈

修改 `cmd/tsm/main.go` 的 `handlePostTUI`：

```go
func handlePostTUI(m ui.Model) {
	os.Remove(postTUIPath())

	if selected := m.Selected(); selected != "" {
		switchToSession(selected, m.ReadOnly())
		return
	}
	if m.OpenConfig() {
		runConfig()
		return // config 結束後 tsm 也結束，使用者下次按 Ctrl+Q 重新進入
	}
	if m.ExitTmux() && os.Getenv("TMUX") != "" {
		_ = osexec.Command("tmux", "detach-client").Run()
	}
}
```

### Step 6: 編譯驗證

Run: `make build && make test`
Expected: 全部通過

### Step 7: Commit

```bash
git add internal/ui/app.go cmd/tsm/main.go internal/ui/app_test.go
git commit -m "feat: 主選單按 c 鍵開啟 config TUI"
```

---

## Task 4：Host 管理持久化

**Files:**
- Modify: `internal/ui/app.go` — 新增 `persistHosts` 方法
- Modify: `internal/ui/hostpicker.go` — 操作後呼叫 persistHosts
- Test: `internal/ui/hostpicker_test.go`

### Step 1: 寫失敗測試

在 `hostpicker_test.go` 新增持久化測試。測試需要驗證 hostpicker 操作後 config 被寫入。

由於 `persistHosts` 需要寫入檔案，測試用 temp dir：

```go
func TestHostPicker_AddHost_PersistsConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := config.Default()

	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Address: "", Color: "#5f8787", Enabled: true})

	deps := ui.Deps{HostMgr: mgr, Cfg: cfg}
	m := ui.NewModel(deps)
	m = sendKeyModel(m, "h")       // 進入 host picker
	m = sendKeyModel(m, "a")       // 按 a 新增
	// 輸入主機位址
	m = typeAndEnter(m, "server1")

	// 驗證 HostMgr 已新增
	assert.Equal(t, 2, len(mgr.Hosts()))
}
```

注意：config 寫入的測試需要 `Model` 知道 config 路徑。在 `Deps` 新增 `ConfigPath string` 欄位。

### Step 2: 在 Deps 新增 ConfigPath

```go
// Deps struct 新增
ConfigPath string // config.toml 路徑，供 persistHosts 使用
```

### Step 3: 實作 persistHosts

在 `app.go` 新增：

```go
// persistHosts 將 HostManager 的主機清單寫回 config.toml。
func (m Model) persistHosts() {
	if m.deps.HostMgr == nil || m.deps.ConfigPath == "" {
		return
	}
	hosts := m.deps.HostMgr.Hosts()
	entries := make([]config.HostEntry, len(hosts))
	for i, h := range hosts {
		cfg := h.Config()
		cfg.SortOrder = i
		entries[i] = cfg
	}

	fileCfg := config.Default()
	cfgPath := m.deps.ConfigPath
	if data, err := os.ReadFile(cfgPath); err == nil {
		if loaded, err := config.LoadFromString(string(data)); err == nil {
			fileCfg = loaded
		}
	}
	fileCfg.Hosts = entries
	_ = config.SaveConfig(cfgPath, fileCfg)
}
```

### Step 4: 在 hostpicker.go 操作後呼叫 persistHosts

在 `updateHostPicker` 的 space（啟停用）、J/K（排序）、d（刪除的 confirmAction 內）操作後加上 `m.persistHosts()`。

新增主機（`InputNewHost` 的 Enter 處理，在 `app.go:818` 附近）也需要呼叫。

```go
// space 切換啟停用後
m.persistHosts()

// J/K 排序後
m.persistHosts()

// 刪除確認的 confirmAction 中
m.confirmAction = func() tea.Cmd {
	_ = m.deps.HostMgr.Disable(hostID)
	m.deps.HostMgr.Remove(hostID)
	m.persistHosts()
	return nil
}

// InputNewHost Enter 處理後
m.deps.HostMgr.AddHost(entry)
_ = m.deps.HostMgr.Enable(context.Background(), value)
m.persistHosts()
```

### Step 5: 在 main.go 傳入 ConfigPath

在 `runMultiHost` 和 `runTUI` 中，將 config 路徑傳入 Deps：

```go
deps := ui.Deps{
	HostMgr:    mgr,
	Cfg:        cfg,
	ConfigPath: config.ExpandPath("~/.config/tsm/config.toml"),
}
```

### Step 6: Run tests

Run: `make test`
Expected: 全部通過

### Step 7: Commit

```bash
git add internal/ui/app.go internal/ui/hostpicker.go internal/ui/hostpicker_test.go cmd/tsm/main.go
git commit -m "feat: host 管理操作持久化到 config.toml"
```

---

## Task 2：擴充 New Session

此為最大的 task，拆分為多個 sub-task。

### Sub-task 2a: Config AgentEntry 結構

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: 寫失敗測試**

```go
func TestLoadFromString_WithAgents(t *testing.T) {
	data := `
[[agents]]
name = "Claude Code"
command = "claude"
group = "Coding"
enabled = true
sort_order = 0

[[agents]]
name = "Gemini"
command = "gemini"
group = "Other"
enabled = false
sort_order = 1
`
	cfg, err := config.LoadFromString(data)
	assert.NoError(t, err)
	assert.Len(t, cfg.Agents, 2)
	assert.Equal(t, "Claude Code", cfg.Agents[0].Name)
	assert.Equal(t, "claude", cfg.Agents[0].Command)
	assert.Equal(t, "Coding", cfg.Agents[0].Group)
	assert.True(t, cfg.Agents[0].Enabled)
	assert.False(t, cfg.Agents[1].Enabled)
}
```

**Step 2: Run test, verify FAIL**

Run: `go test ./internal/config/... -v -race -run TestLoadFromString_WithAgents`
Expected: FAIL — `cfg.Agents` 不存在

**Step 3: 實作 AgentEntry + Config.Agents**

在 `config.go` 新增：

```go
// AgentEntry 代表一個 agent 預設。
type AgentEntry struct {
	Name      string `toml:"name"`
	Command   string `toml:"command"`
	Group     string `toml:"group"`     // 群組標題（選填）
	Enabled   bool   `toml:"enabled"`
	SortOrder int    `toml:"sort_order"`
}
```

在 `Config` struct 新增：

```go
Agents []AgentEntry `toml:"agents"` // Agent 預設清單
```

**Step 4: Run test, verify PASS**

Run: `go test ./internal/config/... -v -race -run TestLoadFromString_WithAgents`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): 新增 AgentEntry 結構支援 agent 預設清單"
```

---

### Sub-task 2b: Store path_history 表

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Step 1: 寫失敗測試**

```go
func TestPathHistory_AddAndList(t *testing.T) {
	s := openTestDB(t)
	defer s.Close()

	assert.NoError(t, s.AddPathHistory("/home/user/project-a"))
	assert.NoError(t, s.AddPathHistory("/home/user/project-b"))
	assert.NoError(t, s.AddPathHistory("/home/user/project-c"))
	assert.NoError(t, s.AddPathHistory("/home/user/project-d"))

	paths, err := s.RecentPaths(3)
	assert.NoError(t, err)
	assert.Len(t, paths, 3)
	// 最近的在前
	assert.Equal(t, "/home/user/project-d", paths[0])
	assert.Equal(t, "/home/user/project-c", paths[1])
	assert.Equal(t, "/home/user/project-b", paths[2])
}

func TestPathHistory_DuplicateUpdatesTimestamp(t *testing.T) {
	s := openTestDB(t)
	defer s.Close()

	assert.NoError(t, s.AddPathHistory("/a"))
	assert.NoError(t, s.AddPathHistory("/b"))
	assert.NoError(t, s.AddPathHistory("/a")) // 重複

	paths, err := s.RecentPaths(3)
	assert.NoError(t, err)
	assert.Equal(t, "/a", paths[0]) // /a 最近使用
	assert.Equal(t, "/b", paths[1])
}
```

**Step 2: Run test, verify FAIL**

Run: `go test ./internal/store/... -v -race -run TestPathHistory`
Expected: FAIL — `AddPathHistory` 不存在

**Step 3: 實作**

在 `store.go` 的 `migrate()` 新增 path_history 表：

```go
CREATE TABLE IF NOT EXISTS path_history (
	path TEXT PRIMARY KEY,
	used_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

新增方法：

```go
// AddPathHistory 記錄路徑使用歷史（重複路徑更新時間戳）。
func (s *Store) AddPathHistory(path string) error {
	_, err := s.db.Exec(`
		INSERT INTO path_history (path, used_at) VALUES (?, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET used_at = CURRENT_TIMESTAMP`, path)
	return err
}

// RecentPaths 回傳最近使用的路徑（最新在前，最多 limit 筆）。
func (s *Store) RecentPaths(limit int) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT path FROM path_history ORDER BY used_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}
```

**Step 4: Run test, verify PASS**

Run: `go test ./internal/store/... -v -race -run TestPathHistory`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): 新增 path_history 表記錄路徑使用歷史"
```

---

### Sub-task 2c: gRPC CreateSessionRequest 擴充

**Files:**
- Modify: `api/proto/tsm/v1/tsm.proto`
- Regenerate: `api/tsm/v1/` (protoc)
- Modify: `internal/daemon/service.go`
- Modify: `internal/tmux/session.go`

**Step 1: 擴充 proto**

```proto
message CreateSessionRequest {
  string name = 1;
  string path = 2;
  string command = 3; // agent 啟動指令（空字串=純 shell）
}
```

**Step 2: 重新產生 protobuf**

Run: `make proto` 或手動 `protoc`（視 Makefile 而定）
若無 make proto，手動：

```bash
protoc --go_out=. --go-grpc_out=. api/proto/tsm/v1/tsm.proto
```

**Step 3: tmux send-keys 支援**

在 `internal/tmux/session.go` 新增：

```go
// SendKeys 在指定 session 中送出按鍵（用於啟動 agent 指令）。
func (m *Manager) SendKeys(sessionName, keys string) error {
	_, err := m.exec.Execute("send-keys", "-t", sessionName, keys, "Enter")
	return err
}
```

**Step 4: service.go 處理 command**

修改 `CreateSession`：

```go
func (s *Service) CreateSession(_ context.Context, req *tsmv1.CreateSessionRequest) (*emptypb.Empty, error) {
	if err := s.tmuxMgr.NewSession(req.Name, req.Path); err != nil {
		return nil, err
	}
	if req.Command != "" {
		_ = s.tmuxMgr.SendKeys(req.Name, req.Command)
	}
	s.state.Scan()
	return &emptypb.Empty{}, nil
}
```

**Step 5: 編譯驗證**

Run: `make build`
Expected: 編譯成功

**Step 6: Commit**

```bash
git add api/ internal/daemon/service.go internal/tmux/session.go
git commit -m "feat: CreateSession 支援 command 參數，啟動 agent 指令"
```

---

### Sub-task 2d: ModeNewSession 表單 UI

這是核心 UI 部分。

**Files:**
- Modify: `internal/ui/mode.go` — 新增 ModeNewSession
- Create: `internal/ui/newsession.go` — 表單邏輯與渲染
- Modify: `internal/ui/app.go` — 整合 ModeNewSession
- Test: `internal/ui/newsession_test.go`

**Step 1: mode.go 新增 ModeNewSession**

```go
const (
	ModeNormal     Mode = iota
	ModeInput
	ModeConfirm
	ModePicker
	ModeSearch
	ModeHostPicker
	ModeNewSession // 新建 session 多欄位表單
)
```

**Step 2: Model struct 新增表單狀態**

在 `app.go` 的 `Model` struct 新增：

```go
// NewSession 表單
newSession newSessionForm
```

新增 `newSessionForm` struct（放在 `newsession.go`）：

```go
package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/wake/tmux-session-menu/internal/config"
)

// newSessionField 代表表單中的焦點區塊。
type newSessionField int

const (
	fieldName   newSessionField = iota // Session 名稱
	fieldPath                          // 啟動路徑
	fieldRecent                        // 最近路徑選擇
	fieldAgent                         // Agent 選擇
)

// newSessionForm 管理 new session 表單的狀態。
type newSessionForm struct {
	nameInput textinput.Model
	pathInput textinput.Model

	field       newSessionField // 目前焦點區塊
	recentPaths []string        // 最近 3 筆路徑
	recentIdx   int             // 最近路徑的游標位置

	agents       []config.AgentEntry // 已啟用的 agent 清單
	agentGroups  []agentGroup        // 分群後的 agent
	agentCursor  int                 // agent 清單中的全域游標
	selectedAgent int               // 已選取的 agent index（-1 = Shell）
}

type agentGroup struct {
	title   string
	entries []config.AgentEntry
	indices []int // 對應 agents 切片的 index
}

// initForm 初始化表單欄位。
func (f *newSessionForm) initForm(agents []config.AgentEntry, recentPaths []string) {
	f.nameInput = textinput.New()
	f.nameInput.Placeholder = "session-name"
	f.nameInput.Focus()

	f.pathInput = textinput.New()
	f.pathInput.Placeholder = "~/Workspace/..."

	f.field = fieldName
	f.recentPaths = recentPaths
	f.recentIdx = 0
	f.selectedAgent = -1 // Shell

	// 過濾已啟用的 agent
	var enabled []config.AgentEntry
	for _, a := range agents {
		if a.Enabled {
			enabled = append(enabled, a)
		}
	}
	f.agents = enabled

	// 依群組分組
	groupOrder := []string{}
	groupMap := make(map[string][]int)
	for i, a := range enabled {
		g := a.Group
		if _, ok := groupMap[g]; !ok {
			groupOrder = append(groupOrder, g)
		}
		groupMap[g] = append(groupMap[g], i)
	}
	f.agentGroups = nil
	for _, g := range groupOrder {
		indices := groupMap[g]
		entries := make([]config.AgentEntry, len(indices))
		for j, idx := range indices {
			entries[j] = enabled[idx]
		}
		f.agentGroups = append(f.agentGroups, agentGroup{
			title:   g,
			entries: entries,
			indices: indices,
		})
	}
	f.agentCursor = 0
}
```

**Step 3: 表單更新邏輯**

在 `newsession.go` 新增更新函數：

```go
// updateNewSession 處理 ModeNewSession 的按鍵。
func (m Model) updateNewSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.newSession

	switch msg.String() {
	case "esc":
		m.mode = ModeNormal
		return m, nil

	case "enter":
		name := f.nameInput.Value()
		if name == "" {
			return m, nil
		}
		path := f.pathInput.Value()
		command := ""
		if f.selectedAgent >= 0 && f.selectedAgent < len(f.agents) {
			command = f.agents[f.selectedAgent].Command
		}

		// 建立 session
		if c := m.clientForCursor(); c != nil {
			if err := c.CreateSession(context.Background(), name, path); err != nil {
				m.err = err
				m.mode = ModeNormal
				return m, nil
			}
			// command 的處理由 daemon service 端完成（CreateSessionRequest.Command）
			// 但目前 client.CreateSession 需要擴充
		}
		if m.deps.TmuxMgr != nil {
			if err := m.deps.TmuxMgr.NewSession(name, path); err != nil {
				m.err = err
				m.mode = ModeNormal
				return m, nil
			}
			if command != "" {
				_ = m.deps.TmuxMgr.SendKeys(name, command)
			}
		}

		// 記錄路徑歷史
		if path != "" && m.deps.Store != nil {
			_ = m.deps.Store.AddPathHistory(path)
		}

		m.mode = ModeNormal
		return m, loadSessionsCmd(m.deps)

	case "tab":
		// 路徑補全（由 OS 目錄列表驅動，未來實作）
		// 目前先跳過，保留介面
		if f.field == fieldPath {
			// TODO: 目錄補全
		}
		return m, nil

	case "up":
		switch f.field {
		case fieldPath:
			f.field = fieldName
			f.nameInput.Focus()
			f.pathInput.Blur()
		case fieldRecent:
			if f.recentIdx > 0 {
				f.recentIdx--
			} else {
				f.field = fieldPath
				f.pathInput.Focus()
			}
		case fieldAgent:
			if f.agentCursor > 0 {
				f.agentCursor--
			} else if len(f.recentPaths) > 0 {
				f.field = fieldRecent
				f.recentIdx = len(f.recentPaths) - 1
			} else {
				f.field = fieldPath
				f.pathInput.Focus()
			}
		}
		return m, nil

	case "down":
		switch f.field {
		case fieldName:
			f.field = fieldPath
			f.nameInput.Blur()
			f.pathInput.Focus()
		case fieldPath:
			f.pathInput.Blur()
			if len(f.recentPaths) > 0 {
				f.field = fieldRecent
				f.recentIdx = 0
			} else if len(f.agents) > 0 {
				f.field = fieldAgent
				f.agentCursor = 0
			}
		case fieldRecent:
			if f.recentIdx < len(f.recentPaths)-1 {
				f.recentIdx++
			} else if len(f.agents) > 0 {
				f.field = fieldAgent
				f.agentCursor = 0
			}
		case fieldAgent:
			totalAgents := len(f.agents) + 1 // +1 for Shell
			if f.agentCursor < totalAgents-1 {
				f.agentCursor++
			}
		}
		return m, nil

	case "left":
		if f.field == fieldAgent {
			if f.agentCursor > 0 {
				f.agentCursor--
			}
		}
		return m, nil

	case "right":
		if f.field == fieldAgent {
			totalAgents := len(f.agents) + 1
			if f.agentCursor < totalAgents-1 {
				f.agentCursor++
			}
		}
		return m, nil

	case " ":
		if f.field == fieldRecent && len(f.recentPaths) > 0 {
			f.pathInput.SetValue(f.recentPaths[f.recentIdx])
			return m, nil
		}
		if f.field == fieldAgent {
			// agentCursor 0..len(agents)-1 = agent, len(agents) = Shell
			if f.agentCursor < len(f.agents) {
				f.selectedAgent = f.agentCursor
			} else {
				f.selectedAgent = -1 // Shell
			}
			return m, nil
		}

	default:
		// 文字輸入轉發
		if f.field == fieldName {
			var cmd tea.Cmd
			f.nameInput, cmd = f.nameInput.Update(msg)
			return m, cmd
		}
		if f.field == fieldPath {
			var cmd tea.Cmd
			f.pathInput, cmd = f.pathInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}
```

**Step 4: 表單渲染**

在 `newsession.go` 新增：

```go
// renderNewSession 渲染 new session 表單。
func (m Model) renderNewSession() string {
	f := &m.newSession
	var b strings.Builder

	// Session 名稱
	nameMarker := "  "
	if f.field == fieldName {
		nameMarker = selectedStyle.Render("► ")
	}
	b.WriteString(fmt.Sprintf("\n  %s%s: %s\n",
		nameMarker, selectedStyle.Render("Session 名稱"), f.nameInput.View()))

	// 啟動路徑
	pathMarker := "  "
	if f.field == fieldPath {
		pathMarker = selectedStyle.Render("► ")
	}
	b.WriteString(fmt.Sprintf("  %s%s: %s\n",
		pathMarker, selectedStyle.Render("啟動路徑"), f.pathInput.View()))

	// 最近路徑
	for i, p := range f.recentPaths {
		marker := "    "
		if f.field == fieldRecent && i == f.recentIdx {
			marker = "  " + selectedStyle.Render("► ")
		}
		style := dimStyle
		if f.field == fieldRecent && i == f.recentIdx {
			style = lipgloss.NewStyle()
		}
		b.WriteString(fmt.Sprintf("%s%s\n", marker, style.Render(p)))
	}

	// 啟動 Agent 標題
	b.WriteString(fmt.Sprintf("\n  %s\n", selectedStyle.Render("啟動 Agent")))

	// 分群渲染
	agentIdx := 0
	for _, g := range f.agentGroups {
		// 群組標題（左側）+ 選項（右側橫排）
		label := "         "
		if g.title != "" {
			label = fmt.Sprintf("  %-7s", g.title)
		}

		var opts []string
		for _, entry := range g.entries {
			radio := "○"
			if agentIdx == f.selectedAgent {
				radio = "●"
			}
			name := entry.Name
			if f.field == fieldAgent && agentIdx == f.agentCursor {
				opts = append(opts, selectedStyle.Render(fmt.Sprintf("%s %s", radio, name)))
			} else {
				opts = append(opts, fmt.Sprintf("%s %s", radio, name))
			}
			agentIdx++
		}
		b.WriteString(fmt.Sprintf("%s%s\n", label, strings.Join(opts, "   ")))
	}

	// Shell 選項（始終在最後）
	shellRadio := "○"
	if f.selectedAgent == -1 {
		shellRadio = "●"
	}
	shellLabel := fmt.Sprintf("%s Shell", shellRadio)
	if f.field == fieldAgent && f.agentCursor == len(f.agents) {
		shellLabel = selectedStyle.Render(shellLabel)
	}
	b.WriteString(fmt.Sprintf("  %s%s\n", "       ", shellLabel))

	// 操作提示
	b.WriteString(fmt.Sprintf("\n  %s\n",
		dimStyle.Render("[↑↓] 移動  [←→] 切換  [space] 選取  [Enter] 建立  [Esc] 取消")))

	return b.String()
}
```

**Step 5: 整合到 app.go**

修改 `n` 鍵處理（`app.go` 約第 597 行）：

```go
case "n":
	var recentPaths []string
	if m.deps.Store != nil {
		recentPaths, _ = m.deps.Store.RecentPaths(3)
	}
	m.newSession.initForm(m.deps.Cfg.Agents, recentPaths)
	m.mode = ModeNewSession
	return m, nil
```

在 `Update` 方法的 mode switch 中新增：

```go
case ModeNewSession:
	return m.updateNewSession(msg.(tea.KeyMsg))
```

在 `View` 方法的 mode 相關底部列 switch 中新增：

```go
case ModeNewSession:
	b.WriteString(m.renderNewSession())
```

**Step 6: 寫測試**

```go
func TestNewSession_Form_BasicFlow(t *testing.T) {
	cfg := config.Default()
	cfg.Agents = []config.AgentEntry{
		{Name: "Claude", Command: "claude", Group: "Coding", Enabled: true},
	}
	m := ui.NewModel(ui.Deps{Cfg: cfg})

	// 按 n 進入表單
	m, _ = sendKey(m, "n")
	assert.Equal(t, ui.ModeNewSession, m.(ui.Model).Mode())

	// Esc 取消
	m, _ = sendKey(m, "esc")
	assert.Equal(t, ui.ModeNormal, m.(ui.Model).Mode())
}
```

**Step 7: Run tests**

Run: `make test`
Expected: 全部通過

**Step 8: Commit**

```bash
git add internal/ui/mode.go internal/ui/newsession.go internal/ui/app.go internal/ui/newsession_test.go
git commit -m "feat: ModeNewSession 多欄位表單 — 名稱、路徑、最近路徑、agent 選擇"
```

---

### Sub-task 2e: client.CreateSession 擴充 command 參數

**Files:**
- Modify: `internal/client/client.go`

**Step 1: 修改 client**

找到 `CreateSession` method，加上 command 參數傳遞：

```go
// 原本
func (c *Client) CreateSession(ctx context.Context, name, path string) error {

// 改為
func (c *Client) CreateSession(ctx context.Context, name, path, command string) error {
	_, err := c.client.CreateSession(ctx, &tsmv1.CreateSessionRequest{
		Name: name, Path: path, Command: command,
	})
	return err
}
```

**Step 2: 更新所有呼叫點**

搜尋 `CreateSession(context.Background()` 確保所有呼叫點都傳入 command 參數。

**Step 3: 編譯驗證**

Run: `make build && make test`
Expected: 全部通過

**Step 4: Commit**

```bash
git add internal/client/client.go internal/ui/app.go internal/ui/newsession.go
git commit -m "feat(client): CreateSession 支援 command 參數"
```

---

### Sub-task 2f: 版本推進 + 最終驗證

**Step 1: 更新 VERSION**

```
0.23.0
```

MINOR bump：new session 擴充是新功能。

**Step 2: 最終驗證**

Run: `make test && make lint && make build`
Expected: 全部通過

**Step 3: 安裝測試**

Run: `make install && cp bin/tsm ~/.local/bin/tsm`

**Step 4: Commit + Tag**

```bash
git add VERSION
git commit -m "chore: v0.23.0"
git tag v0.23.0
```
