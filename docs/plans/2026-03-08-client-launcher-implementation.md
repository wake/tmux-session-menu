# Client Mode Launcher Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Client mode 啟動時顯示三選一 launcher，統一 tab 風格，修正 setup 符號語意

**Architecture:** 新建 `internal/launcher/` 套件作為 client mode 入口 TUI。修改 setup TUI 的 tab 渲染與符號邏輯。Config 層擴充 last_remote_host 持久化。main.go 在 client mode 時攔截，顯示 launcher 後分流。

**Tech Stack:** Go 1.24, Bubble Tea, lipgloss, TDD

---

### Task 1: Config — LastRemoteHost 持久化

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: 寫失敗測試**

在 `config_test.go` 的 InstallMode 測試區塊後加入：

```go
// --- LastRemoteHost 測試 ---

func TestSaveAndLoadLastRemoteHost(t *testing.T) {
	dir := t.TempDir()

	err := config.SaveLastRemoteHost(dir, "myserver.local")
	require.NoError(t, err)

	host := config.LoadLastRemoteHost(dir)
	assert.Equal(t, "myserver.local", host)
}

func TestLoadLastRemoteHost_NoFile_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	host := config.LoadLastRemoteHost(dir)
	assert.Equal(t, "", host)
}
```

**Step 2: 驗證 RED**

Run: `go test ./internal/config/... -v -race -run "TestSaveAndLoadLastRemoteHost|TestLoadLastRemoteHost_NoFile"`
Expected: 編譯失敗 `undefined: config.SaveLastRemoteHost`

**Step 3: 實作 GREEN**

在 `config.go` 的 `LoadInstallMode` 函式後加入：

```go
const lastRemoteHostFile = "last_remote_host"

// SaveLastRemoteHost 儲存最後連線的遠端主機名。
func SaveLastRemoteHost(dir, host string) error {
	return os.WriteFile(filepath.Join(dir, lastRemoteHostFile), []byte(host), 0644)
}

// LoadLastRemoteHost 讀取最後連線的遠端主機名，不存在時回傳空字串。
func LoadLastRemoteHost(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, lastRemoteHostFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
```

**Step 4: 驗證 GREEN**

Run: `go test ./internal/config/... -v -race`
Expected: 全部 PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add LastRemoteHost persistence"
```

---

### Task 2: Setup — Tab 風格渲染

**Files:**
- Modify: `internal/setup/setup.go:236-243`
- Modify: `internal/setup/setup_test.go`

**Step 1: 更新測試預期**

修改 `TestInstallMode_ViewShowsMode` 測試，改為驗證兩個 tab 同時可見：

```go
func TestInstallMode_ViewShowsMode(t *testing.T) {
	m := NewModel(fullClientComponents())
	view := m.View()
	// 兩個 tab 應同時可見
	assert.Contains(t, view, "完整模式")
	assert.Contains(t, view, "純客戶端")
	assert.Contains(t, view, "│")

	// 切到 client — 兩個 tab 仍可見
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	view = m.View()
	assert.Contains(t, view, "完整模式")
	assert.Contains(t, view, "純客戶端")
}
```

**Step 2: 驗證 RED**

Run: `go test ./internal/setup/... -v -race -run TestInstallMode_ViewShowsMode`
Expected: FAIL（目前只有 `◀ 完整模式 ▶`，不含 `│` 分隔）

**Step 3: 實作 GREEN**

修改 `setup.go:236-243`，將 `◀ X ▶` 替換為水平 tab：

```go
	case phaseSelect:
		// 模式選擇器 — 水平 tab
		tabs := []string{"完整模式", "純客戶端"}
		b.WriteString("  ")
		for i, tab := range tabs {
			if i > 0 {
				b.WriteString(dimStyle.Render(" │ "))
			}
			if i == int(m.installMode) {
				b.WriteString(selectedStyle.Render(tab))
			} else {
				b.WriteString(dimStyle.Render(tab))
			}
		}
		b.WriteString("    ")
		b.WriteString(dimStyle.Render("← → 切換"))
		b.WriteString("\n\n")
```

**Step 4: 驗證 GREEN**

Run: `go test ./internal/setup/... -v -race`
Expected: 全部 PASS

**Step 5: Commit**

```bash
git add internal/setup/setup.go internal/setup/setup_test.go
git commit -m "feat(setup): horizontal tab rendering"
```

---

### Task 3: Setup — Installed 欄位 + 符號語意 + 移除行為

**Files:**
- Modify: `internal/setup/setup.go`
- Modify: `internal/setup/setup_test.go`

**Step 1: 寫失敗測試**

在 `setup_test.go` 加入新測試：

```go
func TestSymbol_InstalledFullOnlyInClient_ShowsDash(t *testing.T) {
	comps := []Component{
		{Label: "Binary", InstallFn: func() (string, error) { return "", nil }, Checked: true},
		{Label: "Hooks", InstallFn: func() (string, error) { return "", nil },
			UninstallFn: func() (string, error) { return "removed", nil },
			Checked: false, FullOnly: true, Installed: true},
	}
	m := NewModel(comps)
	m.SetInstallMode(ModeClient)

	view := m.View()
	assert.Contains(t, view, "[-]", "Installed+FullOnly+client 應顯示 [-]")
}

func TestSymbol_NotInstalledFullOnlyInClient_ShowsEmpty(t *testing.T) {
	comps := []Component{
		{Label: "Binary", InstallFn: func() (string, error) { return "", nil }, Checked: true},
		{Label: "Hooks", InstallFn: func() (string, error) { return "", nil },
			Checked: true, FullOnly: true, Installed: false},
	}
	m := NewModel(comps)
	m.SetInstallMode(ModeClient)

	view := m.View()
	assert.Contains(t, view, "[ ]", "NotInstalled+FullOnly+client 應顯示 [ ]")
	assert.NotContains(t, view, "[-]")
}

func TestRunInstall_ClientMode_UninstallsInstalledFullOnly(t *testing.T) {
	uninstalled := false
	comps := []Component{
		{Label: "Binary", InstallFn: func() (string, error) { return "ok", nil }, Checked: true},
		{Label: "Hooks",
			InstallFn:   func() (string, error) { return "", nil },
			UninstallFn: func() (string, error) { uninstalled = true; return "removed hooks", nil },
			Checked: false, FullOnly: true, Installed: true},
	}
	m := NewModel(comps)
	m.SetInstallMode(ModeClient)

	m = driveToPhaseDone(t, m)

	assert.True(t, uninstalled, "Installed+FullOnly+client 應執行 UninstallFn")
	results := m.Results()
	assert.Len(t, results, 2)
	assert.Equal(t, "removed hooks", results[1].message)
}
```

**Step 2: 驗證 RED**

Run: `go test ./internal/setup/... -v -race -run "TestSymbol|TestRunInstall_ClientMode"`
Expected: 編譯失敗 `unknown field Installed`

**Step 3: 實作 GREEN**

3a. 在 `Component` struct 加入 `Installed bool`：

```go
type Component struct {
	Label       string
	InstallFn   func() (string, error)
	UninstallFn func() (string, error)
	Checked     bool
	Disabled    bool
	Note        string
	FullOnly    bool
	Installed   bool   // 偵測到已安裝
}
```

3b. 修改 View 的元件渲染邏輯（取代現有 `disabled` 判斷）：

```go
		for i, comp := range m.components {
			cursor := "  "
			if i == m.cursor {
				cursor = selectedStyle.Render("► ")
			}

			if comp.Disabled {
				label := errorStyle.Render(comp.Label)
				b.WriteString(fmt.Sprintf("%s[-] %s\n", cursor, label))
				if comp.Note != "" {
					b.WriteString(fmt.Sprintf("       %s\n", errorStyle.Render(comp.Note)))
				}
				continue
			}

			isClientFullOnly := comp.FullOnly && m.installMode == ModeClient
			if isClientFullOnly {
				if comp.Installed {
					// [-] 已安裝，將移除
					label := warnStyle.Render(comp.Label)
					b.WriteString(fmt.Sprintf("%s[-] %s\n", cursor, label))
					if comp.Note != "" {
						b.WriteString(fmt.Sprintf("       %s\n", dimStyle.Render(comp.Note)))
					}
				} else {
					// [ ] 未安裝，不適用
					label := dimStyle.Render(comp.Label)
					b.WriteString(fmt.Sprintf("%s[ ] %s\n", cursor, label))
				}
				continue
			}

			check := "[ ]"
			if m.checked[i] {
				check = "[x]"
			}
			label := comp.Label
			if i == m.cursor {
				label = selectedStyle.Render(label)
			}
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, check, label))
			if comp.Note != "" {
				b.WriteString(fmt.Sprintf("       %s\n", dimStyle.Render(comp.Note)))
			}
		}
```

3c. 修改 `runInstall` 加入 client mode 移除邏輯：

```go
func (m Model) runInstall() (tea.Model, tea.Cmd) {
	return m, func() tea.Msg {
		var results []result
		for i, comp := range m.components {
			if comp.Disabled {
				continue
			}

			isClientFullOnly := comp.FullOnly && m.installMode == ModeClient
			if isClientFullOnly && comp.Installed && comp.UninstallFn != nil {
				// [-] 已安裝的 FullOnly 元件在 client mode 執行移除
				msg, err := comp.UninstallFn()
				results = append(results, result{label: comp.Label, message: msg, err: err})
				continue
			}

			if !m.checked[i] || isClientFullOnly {
				continue
			}
			msg, err := comp.InstallFn()
			results = append(results, result{label: comp.Label, message: msg, err: err})
		}
		return resultMsg{results: results}
	}
}
```

**Step 4: 驗證 GREEN**

Run: `go test ./internal/setup/... -v -race`
Expected: 全部 PASS

**Step 5: 更新 `TestInstallMode_ClientViewShowsDisabled` 測試**

這個測試需要更新，因為 `[-]` 現在只有 `Installed` 的 FullOnly 才會顯示：

```go
func TestInstallMode_ClientViewShowsDisabled(t *testing.T) {
	comps := fullClientComponents()
	comps[1].Installed = true // 標記為已安裝
	m := NewModel(comps)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)

	view := m.View()
	assert.Contains(t, view, "[-]")
}
```

**Step 6: 驗證 GREEN**

Run: `go test ./internal/setup/... -v -race`
Expected: 全部 PASS

**Step 7: Commit**

```bash
git add internal/setup/setup.go internal/setup/setup_test.go
git commit -m "feat(setup): Installed field, [-] symbol for removal, uninstall in client mode"
```

---

### Task 4: Setup — Client 模式不詢問 daemon

**Files:**
- Modify: `cmd/tsm/main.go`

**Step 1: 修改 `runSetup`**

在 `runSetup()` 中，將 daemon hint/restart 設定移到 model 完成後根據 mode 決定：

將現有的 daemon hint 設定改為只在 full mode 時啟用。最簡單的方式是在 `p.Run()` 完成前不改動，而是在設定 hint 時檢查 savedMode：

```go
	// 只在 full 模式設定 daemon 提示
	if savedMode == config.ModeFull {
		if isDaemonRunning() {
			m.SetDaemonHint("daemon 需要重新啟動以套用變更")
		} else {
			m.SetDaemonHint("daemon 需要啟動以套用變更")
		}
		m.SetRestartFn(func() error {
			origStderr := os.Stderr
			if devNull, err := os.Open(os.DevNull); err == nil {
				os.Stderr = devNull
				defer func() { os.Stderr = origStderr; devNull.Close() }()
			}
			cfg := loadConfig()
			if daemon.IsRunning(cfg) {
				if err := daemon.Stop(cfg); err != nil {
					return err
				}
			}
			return daemon.Start(cfg)
		})
	}
```

注意：如果使用者在 setup 中切換了 mode，最終 mode 可能與 savedMode 不同。但 daemon hint 在 TUI 啟動前已設定，所以用 savedMode 判斷即可。若切到 full 模式才需要 daemon，但 hint 已不在，使用者可手動 `tsm daemon start`。

**Step 2: 驗證**

Run: `go build ./... && go test ./... -race`
Expected: 全部 PASS

**Step 3: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat(setup): skip daemon prompt in client mode"
```

---

### Task 5: BuildComponent — 設定 Installed 旗標

**Files:**
- Modify: `internal/bind/bind.go:70-77`
- Modify: `internal/hooks/detect.go:143-150`
- Modify: `internal/selfinstall/selfinstall.go:178-210`

**Step 1: bind.BuildComponent**

在 `BindInstalled` case 加入 `Installed: true`：

```go
	case BindInstalled:
		return setup.Component{
			Label:       "Ctrl+Q 快捷鍵 (tmux keybinding)",
			Checked:     false,
			Installed:   true,
			Note:        fmt.Sprintf("已安裝於 %s", r.ConfPath),
			InstallFn:   installFn,
			UninstallFn: uninstallFn,
		}
```

**Step 2: hooks.BuildComponent**

在 `HooksInstalled` 和 `HooksPartial` cases 加入 `Installed: true`：

```go
	case HooksInstalled:
		return setup.Component{
			Label:       "Claude Code hooks",
			Checked:     false,
			Installed:   true,
			Note:        fmt.Sprintf("已安裝（%d/%d events）", total, total),
			InstallFn:   installFn,
			UninstallFn: uninstallFn,
		}
	case HooksPartial:
		return setup.Component{
			Label:       "Claude Code hooks",
			Checked:     true,
			Installed:   true,
			Note:        fmt.Sprintf("部分安裝（%d/%d events）", len(r.InstalledEvents), total),
			InstallFn:   installFn,
			UninstallFn: uninstallFn,
		}
```

**Step 3: selfinstall.BuildComponent**

在 `StatusOutdated` 和 `StatusSameVersion` cases 加入 `Installed: true`：

```go
	case StatusOutdated:
		// ... existing fields ...
		return setup.Component{
			Label:     fmt.Sprintf("更新 tsm 到 %s", r.Path),
			Checked:   true,
			Installed: true,
			// ... rest unchanged ...
		}

	default: // StatusSameVersion
		// ... existing fields ...
		return setup.Component{
			Label:     fmt.Sprintf("覆蓋安裝 tsm 到 %s", r.Path),
			Checked:   false,
			Installed: true,
			// ... rest unchanged ...
		}
```

**Step 4: 驗證**

Run: `go build ./... && go test ./... -race`
Expected: 全部 PASS

**Step 5: Commit**

```bash
git add internal/bind/bind.go internal/hooks/detect.go internal/selfinstall/selfinstall.go
git commit -m "feat: set Installed flag in BuildComponent for bind/hooks/selfinstall"
```

---

### Task 6: Launcher TUI

**Files:**
- Create: `internal/launcher/launcher.go`
- Create: `internal/launcher/launcher_test.go`

**Step 1: 寫失敗測試**

```go
package launcher

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func applyKey(m Model, key string) (Model, tea.Cmd) {
	var msg tea.KeyMsg
	switch key {
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func TestNewModel_DefaultTab(t *testing.T) {
	m := NewModel("")
	assert.Equal(t, 0, m.ActiveTab())
	assert.Equal(t, ChoiceNone, m.Choice())
}

func TestNewModel_WithPlaceholder(t *testing.T) {
	m := NewModel("myserver")
	assert.Equal(t, "myserver", m.Placeholder())
}

func TestLeftRightSwitchTabs(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right")
	assert.Equal(t, 1, m.ActiveTab())

	m, _ = applyKey(m, "right")
	assert.Equal(t, 2, m.ActiveTab())

	// 到底不超出
	m, _ = applyKey(m, "right")
	assert.Equal(t, 2, m.ActiveTab())

	m, _ = applyKey(m, "left")
	assert.Equal(t, 1, m.ActiveTab())

	m, _ = applyKey(m, "left")
	assert.Equal(t, 0, m.ActiveTab())

	// 到頂不超出
	m, _ = applyKey(m, "left")
	assert.Equal(t, 0, m.ActiveTab())
}

func TestEnterOnRemoteTab_WithHost(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "s")
	m, _ = applyKey(m, "v")
	m, _ = applyKey(m, "r")
	m, cmd := applyKey(m, "enter")
	assert.Equal(t, ChoiceRemote, m.Choice())
	assert.Equal(t, "svr", m.Host())
	assert.NotNil(t, cmd)
}

func TestEnterOnRemoteTab_Empty_NoAction(t *testing.T) {
	m := NewModel("")
	m, cmd := applyKey(m, "enter")
	assert.Equal(t, ChoiceNone, m.Choice())
	assert.Nil(t, cmd)
}

func TestTabAutoFillsPlaceholder(t *testing.T) {
	m := NewModel("myserver")
	m, _ = applyKey(m, "tab")
	assert.Equal(t, "myserver", m.Host())
}

func TestTabNoPlaceholder_NoEffect(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "tab")
	assert.Equal(t, "", m.Host())
}

func TestBackspaceDeletesChar(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "a")
	m, _ = applyKey(m, "b")
	assert.Equal(t, "ab", m.Host())
	m, _ = applyKey(m, "backspace")
	assert.Equal(t, "a", m.Host())
}

func TestEnterOnLocalTab(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right") // tab 1
	m, cmd := applyKey(m, "enter")
	assert.Equal(t, ChoiceLocal, m.Choice())
	assert.NotNil(t, cmd)
}

func TestEnterOnFullSetupTab(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right") // tab 1
	m, _ = applyKey(m, "right") // tab 2
	m, cmd := applyKey(m, "enter")
	assert.Equal(t, ChoiceFullSetup, m.Choice())
	assert.NotNil(t, cmd)
}

func TestViewShowsAllTabs(t *testing.T) {
	m := NewModel("")
	view := m.View()
	assert.Contains(t, view, "連線遠端")
	assert.Contains(t, view, "簡易本地端")
	assert.Contains(t, view, "完整設定")
	assert.Contains(t, view, "│")
}

func TestViewRemoteTabShowsInput(t *testing.T) {
	m := NewModel("lasthost")
	view := m.View()
	assert.Contains(t, view, "SSH 主機")
	assert.Contains(t, view, "lasthost")
}

func TestViewLocalTabShowsDescription(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right")
	view := m.View()
	assert.Contains(t, view, "不啟用 daemon")
}

func TestViewFullSetupTabShowsDescription(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right")
	m, _ = applyKey(m, "right")
	view := m.View()
	assert.Contains(t, view, "hook")
}

func TestQuitReturnsNone(t *testing.T) {
	m := NewModel("")
	m, cmd := applyKey(m, "q")
	assert.Equal(t, ChoiceNone, m.Choice())
	assert.NotNil(t, cmd)
}

func TestTypingOnlyOnRemoteTab(t *testing.T) {
	m := NewModel("")
	m, _ = applyKey(m, "right") // tab 1 (local)
	m, _ = applyKey(m, "x")
	assert.Equal(t, "", m.Host(), "非遠端 tab 不應接受文字輸入")
}
```

**Step 2: 驗證 RED**

Run: `go test ./internal/launcher/... -v -race`
Expected: 編譯失敗（套件不存在）

**Step 3: 實作 GREEN**

建立 `internal/launcher/launcher.go`：

```go
package launcher

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wake/tmux-session-menu/internal/version"
)

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c0caf5"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#787fa0"))
)

// Choice 代表使用者的選擇。
type Choice int

const (
	ChoiceNone      Choice = iota
	ChoiceRemote
	ChoiceLocal
	ChoiceFullSetup
)

var tabLabels = []string{"連線遠端", "簡易本地端", "完整設定"}

// Model 是 launcher TUI 的 Bubble Tea model。
type Model struct {
	activeTab   int
	hostInput   string
	placeholder string
	choice      Choice
	quitting    bool
}

// NewModel 建立 launcher model，placeholder 為上次使用的遠端主機。
func NewModel(placeholder string) Model {
	return Model{placeholder: placeholder}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "left":
		if m.activeTab > 0 {
			m.activeTab--
		}
	case "right":
		if m.activeTab < len(tabLabels)-1 {
			m.activeTab++
		}
	case "enter":
		return m.handleEnter()
	case "tab":
		if m.activeTab == 0 && m.hostInput == "" && m.placeholder != "" {
			m.hostInput = m.placeholder
		}
	case "backspace":
		if m.activeTab == 0 && len(m.hostInput) > 0 {
			m.hostInput = m.hostInput[:len(m.hostInput)-1]
		}
	default:
		if m.activeTab == 0 && msg.Type == tea.KeyRunes {
			m.hostInput += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.activeTab {
	case 0: // 連線遠端
		if m.hostInput == "" {
			return m, nil // 沒有輸入主機
		}
		m.choice = ChoiceRemote
	case 1: // 簡易本地端
		m.choice = ChoiceLocal
	case 2: // 完整設定
		m.choice = ChoiceFullSetup
	}
	return m, tea.Quit
}

func (m Model) View() string {
	if m.quitting && m.choice == ChoiceNone {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render("tsm"))
	b.WriteString(dimStyle.Render("  " + version.String()))
	b.WriteString("\n\n")

	// Tab bar
	b.WriteString("  ")
	for i, tab := range tabLabels {
		if i > 0 {
			b.WriteString(dimStyle.Render(" │ "))
		}
		if i == m.activeTab {
			b.WriteString(selectedStyle.Render(tab))
		} else {
			b.WriteString(dimStyle.Render(tab))
		}
	}
	b.WriteString("\n\n")

	// Tab content
	switch m.activeTab {
	case 0:
		b.WriteString("  SSH 主機: ")
		if m.hostInput == "" && m.placeholder != "" {
			b.WriteString(dimStyle.Render(m.placeholder))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("            Tab 自動填入, Enter 連線"))
		} else if m.hostInput == "" {
			b.WriteString(dimStyle.Render("輸入主機名稱..."))
		} else {
			b.WriteString(fmt.Sprintf("%s█", m.hostInput))
		}
	case 1:
		b.WriteString(dimStyle.Render("  直接啟動本地 tmux 管理介面（不啟用 daemon）"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  Enter 啟動"))
	case 2:
		b.WriteString(dimStyle.Render("  安裝完整元件後啟動（hook + bind key + daemon）"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  Enter 開始設定"))
	}
	b.WriteString("\n")

	return b.String()
}

// ActiveTab 回傳目前選取的 tab 索引。
func (m Model) ActiveTab() int { return m.activeTab }

// Choice 回傳使用者的選擇。
func (m Model) Choice() Choice { return m.choice }

// Host 回傳使用者輸入的遠端主機名。
func (m Model) Host() string { return m.hostInput }

// Placeholder 回傳 placeholder 主機名。
func (m Model) Placeholder() string { return m.placeholder }
```

**Step 4: 驗證 GREEN**

Run: `go test ./internal/launcher/... -v -race`
Expected: 全部 PASS

**Step 5: Commit**

```bash
git add internal/launcher/
git commit -m "feat(launcher): client mode launcher TUI with 3-tab selector"
```

---

### Task 7: Main.go — 接線

**Files:**
- Modify: `cmd/tsm/main.go`

**Step 1: 在 `runWithMode` 加入 client mode 攔截**

在 `runWithMode()` 的 popup/inline 判斷前，加入 client mode 檢查：

```go
func runWithMode(mode runMode) {
	// Client mode：顯示 launcher 選單
	dataDir := config.ExpandPath(config.Default().DataDir)
	if config.LoadInstallMode(dataDir) == config.ModeClient {
		runClientLauncher(dataDir)
		return
	}

	// ... existing popup/inline logic ...
}
```

**Step 2: 實作 `runClientLauncher`**

```go
func runClientLauncher(dataDir string) {
	placeholder := config.LoadLastRemoteHost(dataDir)
	m := launcher.NewModel(placeholder)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fm, ok := finalModel.(launcher.Model)
	if !ok || fm.Choice() == launcher.ChoiceNone {
		return
	}

	switch fm.Choice() {
	case launcher.ChoiceRemote:
		host := fm.Host()
		_ = config.SaveLastRemoteHost(dataDir, host)
		runRemote(host)

	case launcher.ChoiceLocal:
		cfg := loadConfig()
		cfg.InTmux = os.Getenv("TMUX") != ""
		cfg.InPopup = os.Getenv("TSM_IN_POPUP") == "1"
		runTUILegacy(cfg)

	case launcher.ChoiceFullSetup:
		// 切換到 full mode 並執行 setup
		_ = config.SaveInstallMode(dataDir, config.ModeFull)
		runSetup(nil)
		runTUI()
	}
}
```

**Step 3: 加入 import**

在 import 區塊加入 `"github.com/wake/tmux-session-menu/internal/launcher"`。

**Step 4: 驗證**

Run: `go build ./... && go test ./... -race`
Expected: 全部 PASS，`bin/tsm` 可正常編譯

**Step 5: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat: wire client mode launcher into main flow"
```

---

### Task 8: 版本推進 + 發佈

**Step 1: 更新 VERSION**

```bash
echo "0.18.0" > VERSION
```

**Step 2: 全部測試**

Run: `make test`
Expected: 全部 PASS

**Step 3: Build + tag + push + publish**

```bash
make build
git add -A
git commit -m "feat: v0.18.0 — client mode launcher, tab UI, setup symbol semantics"
git tag v0.18.0
git push origin main v0.18.0
make release
gh release create v0.18.0 bin/tsm-darwin-arm64 bin/tsm-darwin-amd64 bin/tsm-linux-amd64 bin/tsm-linux-arm64 -R wake/tmux-session-menu --title "v0.18.0" --generate-notes
```
