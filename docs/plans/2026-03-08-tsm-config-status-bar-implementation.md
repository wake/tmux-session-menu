# tsm config + tmux status bar 主題化 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 新增 `tsm config` TUI 設定指令與 tmux status bar 主題化，讓 local/remote 模式有不同視覺風格，並在 status bar 顯示 session 自訂名稱。

**Architecture:** 擴展 Config struct 加入 `[local]`/`[remote]` 顏色區段，新增 `tsm status-name` 輕量子指令供 tmux status-left 動態呼叫，在 `switchToSession` 和 `remote.Attach` 前後套用 status bar 樣式。Config TUI 採用獨立 Bubble Tea model（仿 setup TUI），遠端分頁透過 gRPC GetConfig/SetConfig 讀寫。

**Tech Stack:** Go 1.26, Bubble Tea, lipgloss, gRPC/protobuf, BurntSushi/toml, SQLite

---

### Task 1: 擴展 Config struct — 新增顏色欄位

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: 寫失敗測試**

在 `internal/config/config_test.go` 新增：

```go
func TestDefaultConfig_HasColorSections(t *testing.T) {
	cfg := config.Default()

	// local 預設
	assert.Equal(t, "", cfg.Local.BarBG)
	assert.Equal(t, "#5f8787", cfg.Local.BadgeBG)
	assert.Equal(t, "#c0caf5", cfg.Local.BadgeFG)

	// remote 預設
	assert.Equal(t, "#1a2b2b", cfg.Remote.BarBG)
	assert.Equal(t, "#73daca", cfg.Remote.BadgeBG)
	assert.Equal(t, "#1a1b26", cfg.Remote.BadgeFG)
}

func TestLoadFromTOML_WithColorSections(t *testing.T) {
	tomlData := `
preview_lines = 100

[local]
bar_bg = "#ff0000"
badge_bg = "#00ff00"
badge_fg = "#0000ff"

[remote]
bar_bg = "#112233"
badge_bg = "#445566"
badge_fg = "#778899"
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	assert.Equal(t, 100, cfg.PreviewLines)
	assert.Equal(t, "#ff0000", cfg.Local.BarBG)
	assert.Equal(t, "#00ff00", cfg.Local.BadgeBG)
	assert.Equal(t, "#0000ff", cfg.Local.BadgeFG)
	assert.Equal(t, "#112233", cfg.Remote.BarBG)
	assert.Equal(t, "#445566", cfg.Remote.BadgeBG)
	assert.Equal(t, "#778899", cfg.Remote.BadgeFG)
}

func TestLoadFromTOML_PartialColors_KeepsDefaults(t *testing.T) {
	tomlData := `
[local]
bar_bg = "#aabbcc"
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	assert.Equal(t, "#aabbcc", cfg.Local.BarBG)
	assert.Equal(t, "#5f8787", cfg.Local.BadgeBG)  // 預設值
	assert.Equal(t, "#1a2b2b", cfg.Remote.BarBG)    // 預設值
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/config/... -v -run "TestDefaultConfig_HasColorSections|TestLoadFromTOML_WithColorSections|TestLoadFromTOML_PartialColors"`
Expected: FAIL — `cfg.Local` 欄位不存在

**Step 3: 實作**

修改 `internal/config/config.go`：

```go
// ColorConfig 定義一組 status bar 顏色。
type ColorConfig struct {
	BarBG   string `toml:"bar_bg"`
	BadgeBG string `toml:"badge_bg"`
	BadgeFG string `toml:"badge_fg"`
}

// Config 是 TSM 的全域設定。
type Config struct {
	DataDir         string      `toml:"data_dir"`
	PreviewLines    int         `toml:"preview_lines"`
	PollIntervalSec int         `toml:"poll_interval_sec"`
	Local           ColorConfig `toml:"local"`
	Remote          ColorConfig `toml:"remote"`
	InTmux          bool        `toml:"-"`
	InPopup         bool        `toml:"-"`
}

// Default 回傳預設設定。
func Default() Config {
	return Config{
		DataDir:         "~/.config/tsm",
		PreviewLines:    150,
		PollIntervalSec: 2,
		Local: ColorConfig{
			BarBG:   "",
			BadgeBG: "#5f8787",
			BadgeFG: "#c0caf5",
		},
		Remote: ColorConfig{
			BarBG:   "#1a2b2b",
			BadgeBG: "#73daca",
			BadgeFG: "#1a1b26",
		},
	}
}
```

**Step 4: 執行測試確認通過**

Run: `go test ./internal/config/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): 新增 [local]/[remote] 顏色設定區段"
```

---

### Task 2: Config 儲存功能 — SaveConfig

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: 寫失敗測試**

```go
func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := config.Default()
	cfg.PreviewLines = 200
	cfg.Local.BarBG = "#aabbcc"

	err := config.SaveConfig(path, cfg)
	require.NoError(t, err)

	// 重新讀取驗證
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	loaded, err := config.LoadFromString(string(data))
	require.NoError(t, err)

	assert.Equal(t, 200, loaded.PreviewLines)
	assert.Equal(t, "#aabbcc", loaded.Local.BarBG)
	assert.Equal(t, "#5f8787", loaded.Local.BadgeBG)  // 保持預設
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/config/... -v -run TestSaveConfig`
Expected: FAIL — `SaveConfig` 不存在

**Step 3: 實作**

在 `internal/config/config.go` 新增：

```go
// SaveConfig 將設定寫入 TOML 檔案。
func SaveConfig(path string, cfg Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
```

需要在 import 中確認有 `"github.com/BurntSushi/toml"`。

**Step 4: 執行測試確認通過**

Run: `go test ./internal/config/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): 新增 SaveConfig 寫入 TOML 檔案"
```

---

### Task 3: Store — 取得單一 session 的 custom_name

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Step 1: 寫失敗測試**

在 `internal/store/store_test.go` 新增：

```go
func TestGetCustomName(t *testing.T) {
	st := openTestStore(t)

	// 無資料時回傳空字串
	name, err := st.GetCustomName("no-such-session")
	require.NoError(t, err)
	assert.Equal(t, "", name)

	// 設定後可查詢
	err = st.SetCustomName("my-session", "我的專案")
	require.NoError(t, err)

	name, err = st.GetCustomName("my-session")
	require.NoError(t, err)
	assert.Equal(t, "我的專案", name)
}
```

注意：確認 `openTestStore` helper 是否存在，若無需建立。

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/store/... -v -run TestGetCustomName`
Expected: FAIL — `GetCustomName` 不存在

**Step 3: 實作**

在 `internal/store/store.go` 新增：

```go
// GetCustomName 查詢指定 session 的自訂名稱。不存在時回傳空字串。
func (s *Store) GetCustomName(sessionName string) (string, error) {
	var name string
	err := s.db.QueryRow(
		"SELECT custom_name FROM session_meta WHERE session_name = ?",
		sessionName).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return name, err
}
```

**Step 4: 執行測試確認通過**

Run: `go test ./internal/store/... -v -run TestGetCustomName`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): 新增 GetCustomName 查詢單一 session 自訂名稱"
```

---

### Task 4: tmux status bar 樣式設定函式

**Files:**
- Create: `internal/tmux/statusbar.go`
- Create: `internal/tmux/statusbar_test.go`

**Step 1: 寫失敗測試**

建立 `internal/tmux/statusbar_test.go`：

```go
package tmux_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

func TestStatusBarArgs_LocalDefault(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "",
		BadgeBG: "#5f8787",
		BadgeFG: "#c0caf5",
	}
	args := tmux.StatusBarArgs(colors)

	// bar_bg 為空時不應包含 status-style 指令
	for _, a := range args {
		assert.NotContains(t, a, "status-style")
	}
	// 應包含 status-left 設定
	assert.NotEmpty(t, args)
}

func TestStatusBarArgs_RemoteWithBarBG(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "#1a2b2b",
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}
	args := tmux.StatusBarArgs(colors)

	// 應包含 status-style bg 設定
	found := false
	for i, a := range args {
		if a == "status-style" && i+1 < len(args) {
			assert.Contains(t, args[i+1], "bg=#1a2b2b")
			found = true
		}
	}
	assert.True(t, found, "should contain status-style with bg color")
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/tmux/... -v -run TestStatusBarArgs`
Expected: FAIL — `StatusBarArgs` 不存在

**Step 3: 實作**

建立 `internal/tmux/statusbar.go`：

```go
package tmux

import (
	"fmt"

	"github.com/wake/tmux-session-menu/internal/config"
)

// StatusBarArgs 根據顏色設定產生 tmux set-option 參數陣列。
// 回傳格式為多組 ["-g", "option-name", "value"]。
// 呼叫者須依序對每三個元素執行 tmux set-option。
func StatusBarArgs(colors config.ColorConfig) [][]string {
	var cmds [][]string

	// status bar 背景色
	if colors.BarBG != "" {
		cmds = append(cmds, []string{
			"set-option", "-g", "status-style",
			fmt.Sprintf("bg=%s", colors.BarBG),
		})
	}

	// status-left 含 badge（custom_name 由 #(tsm status-name) 動態取得）
	badgeStyle := fmt.Sprintf("#[bg=%s,fg=%s,bold]", colors.BadgeBG, colors.BadgeFG)
	resetStyle := "#[default]"
	statusLeft := fmt.Sprintf("%s #(tsm status-name) %s ", badgeStyle, resetStyle)

	cmds = append(cmds, []string{
		"set-option", "-g", "status-left", statusLeft,
	})
	cmds = append(cmds, []string{
		"set-option", "-g", "status-left-length", "30",
	})

	return cmds
}

// ApplyStatusBar 執行 tmux set-option 命令套用 status bar 樣式。
func ApplyStatusBar(exec Executor, colors config.ColorConfig) error {
	cmds := StatusBarArgs(colors)
	for _, cmd := range cmds {
		if _, err := exec.Execute(cmd...); err != nil {
			return fmt.Errorf("tmux %v: %w", cmd, err)
		}
	}
	return nil
}
```

**Step 4: 更新測試以匹配實際 API**

調整測試以配合回傳 `[][]string` 結構：

```go
func TestStatusBarArgs_LocalDefault(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "",
		BadgeBG: "#5f8787",
		BadgeFG: "#c0caf5",
	}
	cmds := tmux.StatusBarArgs(colors)

	// bar_bg 為空時不應包含 status-style 指令
	for _, cmd := range cmds {
		if len(cmd) > 2 {
			assert.NotEqual(t, "status-style", cmd[2])
		}
	}
	// 應包含 status-left 設定
	assert.NotEmpty(t, cmds)
}

func TestStatusBarArgs_RemoteWithBarBG(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "#1a2b2b",
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}
	cmds := tmux.StatusBarArgs(colors)

	found := false
	for _, cmd := range cmds {
		if len(cmd) >= 4 && cmd[2] == "status-style" {
			assert.Contains(t, cmd[3], "bg=#1a2b2b")
			found = true
		}
	}
	assert.True(t, found, "should contain status-style with bg color")
}
```

**Step 5: 執行測試確認通過**

Run: `go test ./internal/tmux/... -v -run TestStatusBarArgs`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/tmux/statusbar.go internal/tmux/statusbar_test.go
git commit -m "feat(tmux): 新增 StatusBarArgs 產生 tmux status bar 樣式指令"
```

---

### Task 5: `tsm status-name` 子指令

**Files:**
- Modify: `cmd/tsm/main.go`

**Step 1: 實作子指令**

在 `cmd/tsm/main.go` 的 `main()` 中，於 `if args[0] == "daemon"` 之前新增：

```go
if args[0] == "status-name" {
	runStatusName()
	return
}
```

新增 `runStatusName` 函式：

```go
// runStatusName 輸出當前 tmux session 的自訂名稱（供 tmux status-left 呼叫）。
func runStatusName() {
	// 取得當前 tmux session 名稱
	out, err := osexec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return // 非 tmux 環境，靜默退出
	}
	sessionName := strings.TrimSpace(string(out))
	if sessionName == "" {
		return
	}

	cfg := loadConfig()
	dataDir := config.ExpandPath(cfg.DataDir)
	dbPath := filepath.Join(dataDir, "state.db")

	st, err := store.Open(dbPath)
	if err != nil {
		return // 無法開啟 DB，靜默退出
	}
	defer st.Close()

	name, err := st.GetCustomName(sessionName)
	if err != nil || name == "" {
		return // 無自訂名稱，不輸出
	}

	fmt.Print(name)
}
```

更新 `printUsage()` 的 Commands 區塊新增：

```go
fmt.Fprintln(os.Stderr, "  status-name        輸出當前 session 自訂名稱（供 tmux status bar 使用）")
```

**Step 2: 手動驗證**

Run: `make build && bin/tsm status-name`
Expected: 若在 tmux 內且有自訂名稱則輸出，否則無輸出

**Step 3: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat: 新增 tsm status-name 子指令供 tmux status bar 動態顯示"
```

---

### Task 6: switchToSession 套用 status bar 樣式

**Files:**
- Modify: `cmd/tsm/main.go`

**Step 1: 實作**

修改 `switchToSession` 函式（行 479），在成功 switch-client 後套用 status bar：

在 `switchToSession` 函式開頭加入 `cfg` 參數，或直接在函式內讀取：

```go
func switchToSession(name string, readOnly bool) {
	// ... 現有程式碼 ...

	if os.Getenv("TMUX") != "" {
		// ... 現有 readOnly 處理 ...

		err = osexec.Command("tmux", "switch-client", "-t", name).Run()
		if err == nil {
			// 現有的 automatic-rename 和 key-table 清除
			_ = osexec.Command("tmux", "set-option", "-w", "-t", name, "automatic-rename", "on").Run()
			_ = osexec.Command("tmux", "set-option", "-t", name, "-u", "key-table").Run()

			// 套用 local status bar 樣式
			cfg := loadConfig()
			exec := tmux.NewRealExecutor()
			_ = tmux.ApplyStatusBar(exec, cfg.Local)
		}
	}
	// ...
}
```

**Step 2: 修改 handlePostTUI 傳遞 remote 資訊**

需要區分 local / remote。在 `handlePostTUI` 中已知是 local 模式。在 `runRemote` 中的 `remote.Attach` 前後套用 remote 樣式。

修改 `runRemote` 函式，在 `remote.Attach` 前套用 remote 樣式：

```go
// 在 remote.Attach 前套用 remote status bar 樣式
exec := tmux.NewRealExecutor()
_ = tmux.ApplyStatusBar(exec, cfg.Remote)

result := remote.Attach(host, selected)

// attach 結束後恢復 local 樣式
_ = tmux.ApplyStatusBar(exec, cfg.Local)
```

**Step 3: 手動驗證**

Run: `make build` 並測試切換 session 觀察 status bar 變化
Expected: local session 使用 local 配色，remote session 使用 remote 配色

**Step 4: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat: switchToSession 和 remote.Attach 套用 status bar 主題色"
```

---

### Task 7: gRPC proto — 新增 GetConfig/SetConfig RPCs

**Files:**
- Modify: `api/proto/tsm/v1/tsm.proto`

**Step 1: 在 proto 中定義新的 message 和 RPC**

```protobuf
// ConfigValue 代表一個設定鍵值對。
message ConfigValue {
  string key = 1;    // 例如 "preview_lines", "local.bar_bg"
  string value = 2;
}

// GetConfigRequest 取得設定。
message GetConfigRequest {}

// GetConfigResponse 回傳設定值。
message GetConfigResponse {
  repeated ConfigValue values = 1;
}

// SetConfigRequest 設定值（可一次設定多個）。
message SetConfigRequest {
  repeated ConfigValue values = 1;
}
```

在 `service SessionManager` 中新增：

```protobuf
rpc GetConfig(GetConfigRequest) returns (GetConfigResponse);
rpc SetConfig(SetConfigRequest) returns (google.protobuf.Empty);
```

**Step 2: 重新生成 proto 程式碼**

Run: `make proto`（或手動 protoc 命令）

確認生成的檔案：`api/tsm/v1/tsm.pb.go` 和 `api/tsm/v1/tsm_grpc.pb.go`

**Step 3: Commit**

```bash
git add api/
git commit -m "feat(proto): 新增 GetConfig/SetConfig RPCs"
```

---

### Task 8: Daemon service — 實作 GetConfig/SetConfig

**Files:**
- Modify: `internal/daemon/service.go`

**Step 1: 實作 GetConfig**

在 `internal/daemon/service.go` 新增：

```go
// GetConfig 回傳 daemon 所在主機的設定。
func (s *Service) GetConfig(_ context.Context, _ *tsmv1.GetConfigRequest) (*tsmv1.GetConfigResponse, error) {
	cfg := s.loadConfig()
	values := []*tsmv1.ConfigValue{
		{Key: "preview_lines", Value: fmt.Sprintf("%d", cfg.PreviewLines)},
		{Key: "poll_interval_sec", Value: fmt.Sprintf("%d", cfg.PollIntervalSec)},
		{Key: "local.bar_bg", Value: cfg.Local.BarBG},
		{Key: "local.badge_bg", Value: cfg.Local.BadgeBG},
		{Key: "local.badge_fg", Value: cfg.Local.BadgeFG},
		{Key: "remote.bar_bg", Value: cfg.Remote.BarBG},
		{Key: "remote.badge_bg", Value: cfg.Remote.BadgeBG},
		{Key: "remote.badge_fg", Value: cfg.Remote.BadgeFG},
	}
	return &tsmv1.GetConfigResponse{Values: values}, nil
}
```

Service 需要一個 `loadConfig` 方法或接受 config 路徑。可以在 `NewService` 新增 `cfgPath string` 參數或利用既有的 config 載入。

**Step 2: 實作 SetConfig**

```go
// SetConfig 更新 daemon 所在主機的設定。
func (s *Service) SetConfig(_ context.Context, req *tsmv1.SetConfigRequest) (*emptypb.Empty, error) {
	cfg := s.loadConfig()
	for _, kv := range req.Values {
		switch kv.Key {
		case "preview_lines":
			if v, err := strconv.Atoi(kv.Value); err == nil {
				cfg.PreviewLines = v
			}
		case "poll_interval_sec":
			if v, err := strconv.Atoi(kv.Value); err == nil {
				cfg.PollIntervalSec = v
			}
		case "local.bar_bg":
			cfg.Local.BarBG = kv.Value
		case "local.badge_bg":
			cfg.Local.BadgeBG = kv.Value
		case "local.badge_fg":
			cfg.Local.BadgeFG = kv.Value
		case "remote.bar_bg":
			cfg.Remote.BarBG = kv.Value
		case "remote.badge_bg":
			cfg.Remote.BadgeBG = kv.Value
		case "remote.badge_fg":
			cfg.Remote.BadgeFG = kv.Value
		}
	}
	cfgPath := config.ExpandPath("~/.config/tsm/config.toml")
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
```

**Step 3: 執行測試確認編譯通過**

Run: `go build ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/daemon/service.go
git commit -m "feat(daemon): 實作 GetConfig/SetConfig gRPC RPCs"
```

---

### Task 9: Client — 新增 GetConfig/SetConfig 方法

**Files:**
- Modify: `internal/client/client.go`

**Step 1: 實作**

在 `internal/client/client.go` 新增：

```go
// GetConfig 取得遠端設定。
func (c *Client) GetConfig(ctx context.Context) (*tsmv1.GetConfigResponse, error) {
	return c.rpc.GetConfig(ctx, &tsmv1.GetConfigRequest{})
}

// SetConfig 設定遠端設定值。
func (c *Client) SetConfig(ctx context.Context, values []*tsmv1.ConfigValue) error {
	_, err := c.rpc.SetConfig(ctx, &tsmv1.SetConfigRequest{Values: values})
	return err
}
```

**Step 2: 確認編譯通過**

Run: `go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/client/client.go
git commit -m "feat(client): 新增 GetConfig/SetConfig 方法"
```

---

### Task 10: Config TUI — 獨立 Bubble Tea model

**Files:**
- Create: `internal/cfgtui/cfgtui.go`
- Create: `internal/cfgtui/cfgtui_test.go`

**Step 1: 寫失敗測試**

建立 `internal/cfgtui/cfgtui_test.go`：

```go
package cfgtui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/cfgtui"
	"github.com/wake/tmux-session-menu/internal/config"
)

func TestNewModel_DefaultTab(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg, nil)
	assert.Equal(t, cfgtui.TabClient, m.Tab())
}

func TestNewModel_FieldCount(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg, nil)
	// 客戶端分頁: local.bar_bg, local.badge_bg, local.badge_fg,
	//             remote.bar_bg, remote.badge_bg, remote.badge_fg = 6 欄位
	assert.Equal(t, 6, m.FieldCount(cfgtui.TabClient))
}

func TestModel_SwitchTab(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg, nil)

	// 按右切換到伺服器分頁
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := newM.(cfgtui.Model)
	assert.Equal(t, cfgtui.TabServer, model.Tab())
}

func TestModel_EditField(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg, nil)

	// 按 Enter 進入編輯模式
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := newM.(cfgtui.Model)
	assert.True(t, model.Editing())
}

func TestModel_ResultConfig(t *testing.T) {
	cfg := config.Default()
	m := cfgtui.NewModel(cfg, nil)

	result := m.ResultConfig()
	assert.Equal(t, cfg.Local.BadgeBG, result.Local.BadgeBG)
	assert.Equal(t, cfg.Remote.BarBG, result.Remote.BarBG)
}
```

**Step 2: 執行測試確認失敗**

Run: `go test ./internal/cfgtui/... -v`
Expected: FAIL — package 不存在

**Step 3: 實作 Config TUI model**

建立 `internal/cfgtui/cfgtui.go`：

```go
package cfgtui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/version"
)

// Tab 代表分頁。
type Tab int

const (
	TabClient Tab = iota
	TabServer
)

// Field 代表一個設定欄位。
type Field struct {
	Key          string // config key (e.g. "local.bar_bg")
	Label        string // 顯示名稱
	DefaultValue string
}

// 樣式定義
var (
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c0caf5"))
	dimStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#787fa0"))
	selectedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	activeTabStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1a1b26")).Background(lipgloss.Color("#7aa2f7")).Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#787fa0")).Background(lipgloss.Color("#24283b")).Padding(0, 1)
	labelStyle       = lipgloss.NewStyle().Width(20)
)

// clientFields 客戶端分頁的欄位。
var clientFields = []Field{
	{Key: "local.bar_bg", Label: "本地 bar 背景色"},
	{Key: "local.badge_bg", Label: "本地 badge 背景色"},
	{Key: "local.badge_fg", Label: "本地 badge 前景色"},
	{Key: "remote.bar_bg", Label: "遠端 bar 背景色"},
	{Key: "remote.badge_bg", Label: "遠端 badge 背景色"},
	{Key: "remote.badge_fg", Label: "遠端 badge 前景色"},
}

// serverFields 伺服器分頁的欄位。
var serverFields = []Field{
	{Key: "preview_lines", Label: "預覽行數"},
	{Key: "poll_interval_sec", Label: "輪詢間隔 (秒)"},
	{Key: "local.bar_bg", Label: "本地 bar 背景色"},
	{Key: "local.badge_bg", Label: "本地 badge 背景色"},
	{Key: "local.badge_fg", Label: "本地 badge 前景色"},
	{Key: "remote.bar_bg", Label: "遠端 bar 背景色"},
	{Key: "remote.badge_bg", Label: "遠端 badge 背景色"},
	{Key: "remote.badge_fg", Label: "遠端 badge 前景色"},
}

// Model 是 config TUI 的 Bubble Tea model。
type Model struct {
	tab        Tab
	cursor     int
	editing    bool
	textInput  textinput.Model
	values     map[string]string // key → value
	defaults   map[string]string
	saved      bool
	quitting   bool
	hostname   string // 遠端主機名稱（空 = 本地）
	clientConn interface{} // *client.Client（避免直接依賴）
}

// NewModel 建立 config TUI model。client 為 nil 表示本地模式。
func NewModel(cfg config.Config, client interface{}) Model {
	defaults := configToMap(config.Default())
	values := configToMap(cfg)

	ti := textinput.New()
	ti.CharLimit = 30

	return Model{
		tab:        TabClient,
		values:     values,
		defaults:   defaults,
		textInput:  ti,
		clientConn: client,
	}
}

// Tab 回傳當前分頁。
func (m Model) Tab() Tab { return m.tab }

// Editing 回傳是否在編輯模式。
func (m Model) Editing() bool { return m.editing }

// FieldCount 回傳指定分頁的欄位數。
func (m Model) FieldCount(tab Tab) int {
	if tab == TabClient {
		return len(clientFields)
	}
	return len(serverFields)
}

// ResultConfig 回傳編輯後的 Config。
func (m Model) ResultConfig() config.Config {
	return mapToConfig(m.values)
}

// Saved 回傳是否已儲存。
func (m Model) Saved() bool { return m.saved }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editing {
		return m.handleEditKey(msg)
	}

	fields := m.currentFields()

	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(fields)-1 {
			m.cursor++
		}
	case "left", "right":
		if m.tab == TabClient {
			m.tab = TabServer
		} else {
			m.tab = TabClient
		}
		m.cursor = 0
	case "enter":
		m.editing = true
		field := fields[m.cursor]
		m.textInput.SetValue(m.values[field.Key])
		m.textInput.Focus()
		return m, m.textInput.Cursor.BlinkCmd()
	case "delete", "backspace":
		if !m.editing {
			field := fields[m.cursor]
			if def, ok := m.defaults[field.Key]; ok {
				m.values[field.Key] = def
			}
		}
	case "ctrl+s":
		m.saved = true
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		fields := m.currentFields()
		field := fields[m.cursor]
		m.values[field.Key] = m.textInput.Value()
		m.editing = false
		m.textInput.Blur()
		return m, nil
	case "esc":
		m.editing = false
		m.textInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) currentFields() []Field {
	if m.tab == TabClient {
		return clientFields
	}
	return serverFields
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render("tsm config"))
	b.WriteString(dimStyle.Render("  " + version.String()))
	b.WriteString("\n\n")

	// Tabs
	clientTab := "客戶端"
	serverTab := "伺服器"
	if m.hostname != "" {
		serverTab = fmt.Sprintf("伺服器 (%s)", m.hostname)
	}

	b.WriteString("  ")
	if m.tab == TabClient {
		b.WriteString(activeTabStyle.Render(clientTab))
		b.WriteString(" ")
		b.WriteString(inactiveTabStyle.Render(serverTab))
	} else {
		b.WriteString(inactiveTabStyle.Render(clientTab))
		b.WriteString(" ")
		b.WriteString(activeTabStyle.Render(serverTab))
	}
	b.WriteString("  ")
	b.WriteString(dimStyle.Render("← →"))
	b.WriteString("\n\n")

	// Fields
	fields := m.currentFields()
	for i, field := range fields {
		cursor := "  "
		if i == m.cursor {
			cursor = selectedStyle.Render("► ")
		}

		label := labelStyle.Render(field.Label)
		if i == m.cursor {
			label = selectedStyle.Render(labelStyle.Render(field.Label))
		}

		value := m.values[field.Key]
		if value == "" {
			value = dimStyle.Render("（空）")
		}

		if m.editing && i == m.cursor {
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, label, m.textInput.View()))
		} else {
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, label, value))
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Enter 編輯 | Del 重置 | Ctrl+S 儲存 | q 取消"))
	b.WriteString("\n")

	// 左側 padding
	output := b.String()
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = " " + line
		}
	}
	return strings.Join(lines, "\n")
}

// SetHostname 設定遠端主機名稱（顯示在伺服器分頁標題）。
func (m *Model) SetHostname(hostname string) {
	m.hostname = hostname
}

// configToMap 將 Config 轉換為 key-value map。
func configToMap(cfg config.Config) map[string]string {
	return map[string]string{
		"preview_lines":    fmt.Sprintf("%d", cfg.PreviewLines),
		"poll_interval_sec": fmt.Sprintf("%d", cfg.PollIntervalSec),
		"local.bar_bg":     cfg.Local.BarBG,
		"local.badge_bg":   cfg.Local.BadgeBG,
		"local.badge_fg":   cfg.Local.BadgeFG,
		"remote.bar_bg":    cfg.Remote.BarBG,
		"remote.badge_bg":  cfg.Remote.BadgeBG,
		"remote.badge_fg":  cfg.Remote.BadgeFG,
	}
}

// mapToConfig 將 key-value map 轉回 Config。
func mapToConfig(values map[string]string) config.Config {
	cfg := config.Default()
	if v, ok := values["preview_lines"]; ok {
		fmt.Sscanf(v, "%d", &cfg.PreviewLines)
	}
	if v, ok := values["poll_interval_sec"]; ok {
		fmt.Sscanf(v, "%d", &cfg.PollIntervalSec)
	}
	cfg.Local.BarBG = values["local.bar_bg"]
	cfg.Local.BadgeBG = values["local.badge_bg"]
	cfg.Local.BadgeFG = values["local.badge_fg"]
	cfg.Remote.BarBG = values["remote.bar_bg"]
	cfg.Remote.BadgeBG = values["remote.badge_bg"]
	cfg.Remote.BadgeFG = values["remote.badge_fg"]
	return cfg
}
```

**Step 4: 執行測試確認通過**

Run: `go test ./internal/cfgtui/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cfgtui/
git commit -m "feat(cfgtui): 新增 Config TUI 雙分頁設定介面"
```

---

### Task 11: CLI — 新增 `tsm config` 子指令

**Files:**
- Modify: `cmd/tsm/main.go`

**Step 1: 實作**

在 `main()` 中新增：

```go
if args[0] == "config" {
	runConfig()
	return
}
```

新增 `runConfig` 函式：

```go
func runConfig() {
	cfg := loadConfig()

	m := cfgtui.NewModel(cfg, nil)

	// 嘗試連線 daemon 取得 hostname（可選）
	if c, err := client.Dial(cfg); err == nil {
		defer c.Close()
		// 可在此取得 remote config
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fm, ok := finalModel.(cfgtui.Model)
	if !ok || !fm.Saved() {
		return
	}

	// 儲存設定
	cfgPath := config.ExpandPath("~/.config/tsm/config.toml")
	result := fm.ResultConfig()
	// 保留 runtime 欄位
	result.DataDir = cfg.DataDir
	if err := config.SaveConfig(cfgPath, result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: 儲存設定失敗: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("設定已儲存")
}
```

更新 `printUsage()`：

```go
fmt.Fprintln(os.Stderr, "  config             設定 tsm 參數與主題色")
```

新增 import：`"github.com/wake/tmux-session-menu/internal/cfgtui"`

**Step 2: 確認編譯通過**

Run: `make build`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat: 新增 tsm config 子指令"
```

---

### Task 12: 版本推進與整合測試

**Files:**
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Step 1: 推進版本號**

```bash
echo "0.21.0" > VERSION
```

**Step 2: 更新 CHANGELOG**

在 `CHANGELOG.md` 頂部新增 v0.21.0 區段：

```markdown
## [0.21.0] - 2026-03-08

### Added
- `tsm config` TUI 設定指令，雙分頁（客戶端/伺服器）
- `tsm status-name` 子指令，供 tmux status bar 動態顯示 session 自訂名稱
- Config TOML 新增 `[local]`/`[remote]` 顏色設定區段
- tmux status bar 主題化：local/remote 模式不同背景色與 badge 色
- gRPC GetConfig/SetConfig RPCs 支援遠端設定編輯

### Changed
- `switchToSession` 切換時自動套用 status bar 樣式
- `remote.Attach` 前後切換 remote/local status bar 樣式
```

**Step 3: 全面測試**

Run: `make test`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add VERSION CHANGELOG.md
git commit -m "feat: v0.21.0 — tsm config TUI、status bar 主題化"
```

**Step 5: 建置與發佈**

```bash
make build
```
