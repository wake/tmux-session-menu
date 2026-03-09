# In-TUI Upgrade 實作計畫

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在 TUI 選單內以 ctrl+u 觸發升級，下載新 binary 後重啟 daemon + exec 重啟 TUI。

**Architecture:** 兩階段分離 — Phase 1 在 UI 內處理互動與下載，Phase 2 在 main.go 的 p.Run() 返回後處理安裝、daemon 重啟與 syscall.Exec()。

**Tech Stack:** Go 1.24, Bubble Tea, gRPC, testify

---

## Task 1: UI 升級訊息類型與 Model 欄位

**Files:**
- Modify: `internal/ui/app.go:24-40` (Deps 結構) + `:90-116` (Model 結構)

**Step 1: 寫失敗測試 — ctrl+u 無 Upgrader 時不觸發**

在 `internal/ui/app_test.go` 末尾新增：

```go
func TestCtrlU_WithoutUpgrader_Noop(t *testing.T) {
	m := ui.NewModel(ui.Deps{}) // 無 Upgrader
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0}, Alt: false})
	// 模擬 ctrl+u
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.Nil(t, cmd)
}
```

**Step 2: 執行測試確認失敗**

```bash
go test ./internal/ui/... -v -race -run TestCtrlU_WithoutUpgrader_Noop
```

預期：FAIL（`tea.KeyCtrlU` 可能讓 Bubble Tea 處理為其他行為，或者 pass 因為目前 ctrl+u 沒有 handler）

**Step 3: 實作最小程式碼**

在 `internal/ui/app.go` 新增：

1. Deps 新增 Upgrader 欄位：
```go
type Deps struct {
	// ...existing fields...
	Upgrader *upgrade.Upgrader // nil 時隱藏 ctrl+u
}
```

2. import 新增 upgrade 套件：
```go
"github.com/wake/tmux-session-menu/internal/upgrade"
```

3. Model 新增升級相關欄位：
```go
// 升級狀態
upgradeReady   bool
upgradeTmpPath string
upgradeVersion string
upgradeAssetURL string // 下載 URL（確認後使用）
```

4. 新增訊息類型（在 Model 定義附近）：
```go
type checkUpgradeMsg struct {
	Release *upgrade.Release
	Err     error
}

type downloadUpgradeMsg struct {
	TmpPath string
	Err     error
}
```

5. 新增公開 accessor：
```go
func (m Model) UpgradeReady() bool                    { return m.upgradeReady }
func (m Model) UpgradeInfo() (tmpPath, version string) { return m.upgradeTmpPath, m.upgradeVersion }
```

6. updateNormal 中 ctrl+u 無 Upgrader 時 noop：
```go
case "ctrl+u":
	if m.deps.Upgrader == nil {
		return m, nil
	}
	// TODO: 觸發檢查
	return m, nil
```

**Step 4: 執行測試確認通過**

```bash
go test ./internal/ui/... -v -race -run TestCtrlU_WithoutUpgrader_Noop
```

預期：PASS

**Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): 新增升級訊息類型、Model 欄位與 Deps.Upgrader"
```

---

## Task 2: ctrl+u 觸發版本檢查

**Files:**
- Modify: `internal/ui/app.go` (updateNormal + 新增 checkUpgradeCmd)
- Test: `internal/ui/app_test.go`

**Step 1: 寫失敗測試**

```go
func TestCtrlU_WithUpgrader_TriggersCheck(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			return []byte(`{"tag_name": "v99.0.0", "assets": []}`), nil
		},
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	assert.NotNil(t, cmd, "ctrl+u 應觸發版本檢查命令")
}
```

**Step 2: 執行測試確認失敗**

```bash
go test ./internal/ui/... -v -race -run TestCtrlU_WithUpgrader_TriggersCheck
```

預期：FAIL（cmd 為 nil）

**Step 3: 實作**

在 `internal/ui/app.go` 中：

1. 新增 checkUpgradeCmd 函式：
```go
func checkUpgradeCmd(u *upgrade.Upgrader) tea.Cmd {
	return func() tea.Msg {
		rel, err := u.CheckLatest()
		if err != nil {
			return checkUpgradeMsg{Err: err}
		}
		return checkUpgradeMsg{Release: &rel}
	}
}
```

2. updateNormal 的 ctrl+u case 改為：
```go
case "ctrl+u":
	if m.deps.Upgrader == nil {
		return m, nil
	}
	return m, checkUpgradeCmd(m.deps.Upgrader)
```

**Step 4: 執行測試確認通過**

```bash
go test ./internal/ui/... -v -race -run TestCtrlU_With
```

預期：PASS

**Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): ctrl+u 觸發版本檢查"
```

---

## Task 3: 處理 checkUpgradeMsg — 有新版本

**Files:**
- Modify: `internal/ui/app.go` (Update 函式)
- Test: `internal/ui/app_test.go`

**Step 1: 寫失敗測試**

```go
func TestCheckUpgradeMsg_NewVersion(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			return nil, nil
		},
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	// 模擬收到有新版的 checkUpgradeMsg
	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{
			Version: "1.0.0",
			Assets:  map[string]string{upgrade.AssetName(): "https://example.com/dl"},
		},
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "1.0.0")
}
```

**Step 2: 執行測試確認失敗**

```bash
go test ./internal/ui/... -v -race -run TestCheckUpgradeMsg_NewVersion
```

預期：FAIL（checkUpgradeMsg 未公開、ConfirmPrompt 不存在）

**Step 3: 實作**

1. 將 `checkUpgradeMsg` 改為公開 `CheckUpgradeMsg`（因為測試在 `ui_test` 包中需要存取）：
```go
type CheckUpgradeMsg struct {
	Release *upgrade.Release
	Err     error
}
```

2. 同理 `downloadUpgradeMsg` → `DownloadUpgradeMsg`

3. 新增 ConfirmPrompt accessor：
```go
func (m Model) ConfirmPrompt() string { return m.confirmPrompt }
```

4. 在 Update() 函式中加入 CheckUpgradeMsg 處理（在 tea.KeyMsg case 之前）：
```go
case CheckUpgradeMsg:
	if msg.Err != nil {
		m.mode = ModeConfirm
		m.confirmPrompt = fmt.Sprintf("檢查失敗：%v", msg.Err)
		return m, nil
	}
	if !upgrade.NeedsUpgrade(version.Version, msg.Release.Version) {
		m.mode = ModeConfirm
		m.confirmPrompt = fmt.Sprintf("已是最新版本 (%s)", version.Version)
		return m, nil
	}
	asset := upgrade.AssetName()
	url, ok := msg.Release.Assets[asset]
	if !ok {
		m.mode = ModeConfirm
		m.confirmPrompt = fmt.Sprintf("找不到適用於此平台的檔案 (%s)", asset)
		return m, nil
	}
	m.upgradeVersion = msg.Release.Version
	m.upgradeAssetURL = url
	m.mode = ModeConfirm
	m.confirmPrompt = fmt.Sprintf("v%s → v%s 升級？", version.Version, msg.Release.Version)
	u := m.deps.Upgrader
	m.confirmAction = func() tea.Cmd {
		return downloadUpgradeCmd(u, url)
	}
	return m, nil
```

5. 新增 downloadUpgradeCmd：
```go
func downloadUpgradeCmd(u *upgrade.Upgrader, url string) tea.Cmd {
	return func() tea.Msg {
		path, err := u.Download(url)
		if err != nil {
			return DownloadUpgradeMsg{Err: err}
		}
		return DownloadUpgradeMsg{TmpPath: path}
	}
}
```

6. 更新 checkUpgradeCmd 以使用新的公開型別名稱。

**Step 4: 執行測試確認通過**

```bash
go test ./internal/ui/... -v -race -run TestCheckUpgradeMsg_NewVersion
```

預期：PASS

**Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): 處理版本檢查結果，有新版時顯示升級確認"
```

---

## Task 4: 處理 checkUpgradeMsg — 已是最新 + 錯誤

**Files:**
- Test: `internal/ui/app_test.go`
- Verify: `internal/ui/app.go` (已在 Task 3 實作)

**Step 1: 寫失敗測試**

```go
func TestCheckUpgradeMsg_AlreadyLatest(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{Version: version.Version},
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "已是最新版本")
}

func TestCheckUpgradeMsg_Error(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Err: fmt.Errorf("network timeout"),
	})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "檢查失敗")
	assert.Contains(t, model.ConfirmPrompt(), "network timeout")
}
```

**Step 2: 執行測試確認通過（已在 Task 3 實作）**

```bash
go test ./internal/ui/... -v -race -run "TestCheckUpgradeMsg_(AlreadyLatest|Error)"
```

預期：PASS（程式碼已在 Task 3 實作）

如果有任何失敗，修正 Update() 中的對應分支。

**Step 3: Commit**

```bash
git add internal/ui/app_test.go
git commit -m "test(ui): 版本檢查結果測試 — 已最新 + 錯誤"
```

---

## Task 5: 確認升級 — Y 觸發下載 / N 取消

**Files:**
- Test: `internal/ui/app_test.go`
- Verify: `internal/ui/app.go`

**Step 1: 寫失敗測試**

```go
func TestConfirmUpgrade_Yes_TriggersDownload(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			return []byte("binary-content"), nil
		},
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	// 模擬收到有新版的訊息
	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{
			Version: "99.0.0",
			Assets:  map[string]string{upgrade.AssetName(): "https://example.com/dl"},
		},
	})
	m = updated.(ui.Model)
	assert.Equal(t, ui.ModeConfirm, m.Mode())

	// 按 Y 確認
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.NotNil(t, cmd, "Y 應觸發下載命令")
}

func TestConfirmUpgrade_No_Cancels(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			return nil, nil
		},
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{
			Version: "99.0.0",
			Assets:  map[string]string{upgrade.AssetName(): "https://example.com/dl"},
		},
	})
	m = updated.(ui.Model)

	// 按 N 取消
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model := updated.(ui.Model)
	assert.Equal(t, ui.ModeNormal, model.Mode())
	assert.Nil(t, cmd)
}
```

**Step 2: 執行測試**

```bash
go test ./internal/ui/... -v -race -run "TestConfirmUpgrade_"
```

預期：PASS（使用既有 ModeConfirm + confirmAction 機制）

**Step 3: Commit**

```bash
git add internal/ui/app_test.go
git commit -m "test(ui): 升級確認 Y/N 測試"
```

---

## Task 6: 處理 DownloadUpgradeMsg — 成功

**Files:**
- Modify: `internal/ui/app.go` (Update 函式)
- Test: `internal/ui/app_test.go`

**Step 1: 寫失敗測試**

```go
func TestDownloadUpgradeMsg_Success(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	updated, cmd := m.Update(ui.DownloadUpgradeMsg{TmpPath: "/tmp/tsm-new"})
	model := updated.(ui.Model)
	assert.True(t, model.UpgradeReady())
	tmpPath, _ := model.UpgradeInfo()
	assert.Equal(t, "/tmp/tsm-new", tmpPath)
	// cmd 應為 tea.Quit
	assert.NotNil(t, cmd)
}
```

**Step 2: 執行測試確認失敗**

```bash
go test ./internal/ui/... -v -race -run TestDownloadUpgradeMsg_Success
```

預期：FAIL（DownloadUpgradeMsg 未在 Update 中處理）

**Step 3: 實作**

在 Update() 中加入 DownloadUpgradeMsg 處理：
```go
case DownloadUpgradeMsg:
	if msg.Err != nil {
		m.mode = ModeConfirm
		m.confirmPrompt = fmt.Sprintf("下載失敗：%v", msg.Err)
		return m, nil
	}
	m.upgradeTmpPath = msg.TmpPath
	m.upgradeReady = true
	m.quitting = true
	return m, tea.Quit
```

**Step 4: 執行測試確認通過**

```bash
go test ./internal/ui/... -v -race -run TestDownloadUpgradeMsg_Success
```

預期：PASS

**Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): 處理下載結果，成功時設定 upgradeReady 並退出"
```

---

## Task 7: 處理 DownloadUpgradeMsg — 失敗

**Files:**
- Test: `internal/ui/app_test.go`
- Verify: `internal/ui/app.go` (已在 Task 6 實作)

**Step 1: 寫測試**

```go
func TestDownloadUpgradeMsg_Error(t *testing.T) {
	m := ui.NewModel(ui.Deps{})

	updated, _ := m.Update(ui.DownloadUpgradeMsg{Err: fmt.Errorf("disk full")})
	model := updated.(ui.Model)
	assert.False(t, model.UpgradeReady())
	assert.Equal(t, ui.ModeConfirm, model.Mode())
	assert.Contains(t, model.ConfirmPrompt(), "下載失敗")
	assert.Contains(t, model.ConfirmPrompt(), "disk full")
}
```

**Step 2: 執行測試**

```bash
go test ./internal/ui/... -v -race -run TestDownloadUpgradeMsg_Error
```

預期：PASS

**Step 3: Commit**

```bash
git add internal/ui/app_test.go
git commit -m "test(ui): 下載失敗測試"
```

---

## Task 8: PostUpgrade 操作結構與 happy path

**Files:**
- Create: `internal/upgrade/postupgrade.go`
- Create: `internal/upgrade/postupgrade_test.go`

**Step 1: 寫失敗測試**

```go
package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPostUpgrade_HappyPath(t *testing.T) {
	var calls []string

	ops := PostUpgradeOps{
		InstallBinary: func(tmpPath string) (string, error) {
			calls = append(calls, "install:"+tmpPath)
			return "/usr/local/bin/tsm", nil
		},
		RemoveTmp: func(path string) {
			calls = append(calls, "remove:"+path)
		},
		StopDaemon: func() error {
			calls = append(calls, "stop")
			return nil
		},
		StartDaemon: func() error {
			calls = append(calls, "start")
			return nil
		},
		Exec: func(path string, args []string, env []string) error {
			calls = append(calls, "exec:"+path)
			return nil
		},
	}

	err := RunPostUpgrade(ops, "/tmp/tsm-new", []string{"tsm", "--inline"}, []string{"HOME=/root"})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"install:/tmp/tsm-new",
		"remove:/tmp/tsm-new",
		"stop",
		"start",
		"exec:/usr/local/bin/tsm",
	}, calls)
}
```

**Step 2: 執行測試確認失敗**

```bash
go test ./internal/upgrade/... -v -race -run TestRunPostUpgrade_HappyPath
```

預期：FAIL（PostUpgradeOps 和 RunPostUpgrade 不存在）

**Step 3: 實作**

建立 `internal/upgrade/postupgrade.go`：
```go
package upgrade

import "fmt"

// PostUpgradeOps 封裝 post-quit 升級操作的依賴，便於測試。
type PostUpgradeOps struct {
	InstallBinary func(tmpPath string) (installedPath string, err error)
	RemoveTmp     func(path string)
	StopDaemon    func() error
	StartDaemon   func() error
	Exec          func(path string, args []string, env []string) error
}

// RunPostUpgrade 執行 TUI 退出後的升級流程：安裝 → 清理 → 重啟 daemon → exec。
func RunPostUpgrade(ops PostUpgradeOps, tmpPath string, args []string, env []string) error {
	installedPath, err := ops.InstallBinary(tmpPath)
	if err != nil {
		ops.RemoveTmp(tmpPath)
		return fmt.Errorf("安裝失敗: %w", err)
	}
	ops.RemoveTmp(tmpPath)

	// daemon stop 失敗不阻斷流程（可能本來就沒跑）
	_ = ops.StopDaemon()

	if err := ops.StartDaemon(); err != nil {
		return fmt.Errorf("啟動 daemon 失敗: %w", err)
	}

	return ops.Exec(installedPath, args, env)
}
```

**Step 4: 執行測試確認通過**

```bash
go test ./internal/upgrade/... -v -race -run TestRunPostUpgrade_HappyPath
```

預期：PASS

**Step 5: Commit**

```bash
git add internal/upgrade/postupgrade.go internal/upgrade/postupgrade_test.go
git commit -m "feat(upgrade): 新增 PostUpgradeOps 與 RunPostUpgrade"
```

---

## Task 9: PostUpgrade 失敗測試

**Files:**
- Test: `internal/upgrade/postupgrade_test.go`

**Step 1: 寫測試**

```go
func TestRunPostUpgrade_InstallFail(t *testing.T) {
	var calls []string

	ops := PostUpgradeOps{
		InstallBinary: func(tmpPath string) (string, error) {
			return "", fmt.Errorf("permission denied")
		},
		RemoveTmp: func(path string) {
			calls = append(calls, "remove")
		},
		StopDaemon: func() error {
			calls = append(calls, "stop")
			return nil
		},
		StartDaemon: func() error {
			calls = append(calls, "start")
			return nil
		},
		Exec: func(path string, args []string, env []string) error {
			calls = append(calls, "exec")
			return nil
		},
	}

	err := RunPostUpgrade(ops, "/tmp/tsm-new", nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "安裝失敗")
	// 安裝失敗時仍清理暫存檔，但不呼叫 stop/start/exec
	assert.Equal(t, []string{"remove"}, calls)
}

func TestRunPostUpgrade_DaemonStopFail_ContinuesAnyway(t *testing.T) {
	var calls []string

	ops := PostUpgradeOps{
		InstallBinary: func(tmpPath string) (string, error) {
			calls = append(calls, "install")
			return "/usr/local/bin/tsm", nil
		},
		RemoveTmp: func(path string) {
			calls = append(calls, "remove")
		},
		StopDaemon: func() error {
			calls = append(calls, "stop-fail")
			return fmt.Errorf("no daemon running")
		},
		StartDaemon: func() error {
			calls = append(calls, "start")
			return nil
		},
		Exec: func(path string, args []string, env []string) error {
			calls = append(calls, "exec")
			return nil
		},
	}

	err := RunPostUpgrade(ops, "/tmp/tsm-new", nil, nil)
	require.NoError(t, err)
	// stop 失敗不影響後續流程
	assert.Equal(t, []string{"install", "remove", "stop-fail", "start", "exec"}, calls)
}
```

**Step 2: 執行測試**

```bash
go test ./internal/upgrade/... -v -race -run "TestRunPostUpgrade_"
```

預期：PASS

**Step 3: Commit**

```bash
git add internal/upgrade/postupgrade_test.go
git commit -m "test(upgrade): PostUpgrade 安裝失敗 + daemon stop 失敗測試"
```

---

## Task 10: main.go 整合 — 注入 Upgrader + post-quit 處理

**Files:**
- Modify: `cmd/tsm/main.go:673-691` (runTUIWithClient)
- Modify: `cmd/tsm/main.go:694-733` (runTUILegacy)
- Modify: `cmd/tsm/main.go:244-246` (runMultiHost deps)

**Step 1: 修改 runTUIWithClient**

在 `runTUIWithClient` 中：

1. 建立 Deps 時注入 Upgrader：
```go
func runTUIWithClient(c *client.Client, cfg config.Config) {
	deps := ui.Deps{
		Client:   c,
		Cfg:      cfg,
		Upgrader: upgrade.DefaultUpgrader(),
	}
	// ...existing...
```

2. 在 handlePostTUI 之前加入升級檢查：
```go
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(ui.Model); ok {
		if m.UpgradeReady() {
			c.Close() // 先關閉 gRPC 連線
			runPostUpgrade(m, cfg)
			return // exec 成功不會到這裡
		}
		handlePostTUI(m)
	}
```

**Step 2: 實作 runPostUpgrade**

```go
func runPostUpgrade(m ui.Model, cfg config.Config) {
	tmpPath, ver := m.UpgradeInfo()

	ops := upgrade.PostUpgradeOps{
		InstallBinary: func(tmp string) (string, error) {
			det := selfinstall.Detect()
			targetDir := filepath.Dir(det.Path)
			if targetDir == "" || targetDir == "." {
				home, _ := os.UserHomeDir()
				targetDir = filepath.Join(home, ".local", "bin")
			}
			msg, err := selfinstall.Install(targetDir)
			if err != nil {
				return "", err
			}
			_ = msg
			return filepath.Join(targetDir, "tsm"), nil
		},
		RemoveTmp: func(path string) {
			os.Remove(path)
		},
		StopDaemon: func() error {
			if daemon.IsRunning(cfg) {
				return daemon.Stop(cfg)
			}
			return nil
		},
		StartDaemon: func() error {
			return daemon.Start(cfg)
		},
		Exec: func(path string, args []string, env []string) error {
			return syscall.Exec(path, args, env)
		},
	}

	fmt.Fprintf(os.Stderr, "升級至 v%s，重新啟動...\n", ver)
	if err := upgrade.RunPostUpgrade(ops, tmpPath, os.Args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "升級失敗: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 3: 同步更新 runTUILegacy 和 runMultiHost**

在 `runTUILegacy` 的 deps 中也注入 Upgrader：
```go
deps := ui.Deps{
	TmuxMgr:   mgr,
	Store:     st,
	Cfg:       cfg,
	StatusDir: statusDir,
	Upgrader:  upgrade.DefaultUpgrader(),
}
```

在 runTUILegacy 的 handlePostTUI 前加入升級檢查（同 runTUIWithClient 模式）。

在 runMultiHost 的 deps 中也注入 Upgrader：
```go
deps := ui.Deps{HostMgr: mgr, Cfg: cfg, ConfigPath: config.ExpandPath("~/.config/tsm/config.toml"), Upgrader: upgrade.DefaultUpgrader()}
```

在 runMultiHost 的 handlePostTUI 前也加入升級檢查。

**Step 4: 新增 import**

```go
import (
	// ...existing...
	"syscall"
)
```

**Step 5: 編譯驗證**

```bash
make build
```

預期：成功

**Step 6: 執行全部測試**

```bash
make test
```

預期：PASS

**Step 7: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat: 整合 in-TUI 升級到 main.go（注入 Upgrader + post-quit exec）"
```

---

## Task 11: UI 渲染 — 升級中提示

**Files:**
- Modify: `internal/ui/app.go` (View 函式)
- Test: `internal/ui/app_test.go`

**Step 1: 寫測試**

```go
func TestView_UpgradeConfirm_ShowsVersionDiff(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) { return nil, nil },
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})

	// 模擬收到升級訊息
	updated, _ := m.Update(ui.CheckUpgradeMsg{
		Release: &upgrade.Release{
			Version: "99.0.0",
			Assets:  map[string]string{upgrade.AssetName(): "https://example.com/dl"},
		},
	})
	model := updated.(ui.Model)
	view := model.View()
	assert.Contains(t, view, "99.0.0")
	assert.Contains(t, view, "升級")
}
```

**Step 2: 執行測試**

```bash
go test ./internal/ui/... -v -race -run TestView_UpgradeConfirm
```

預期：PASS（使用既有 ModeConfirm 渲染邏輯，confirmPrompt 已包含版本資訊）

**Step 3: Commit**

```bash
git add internal/ui/app_test.go
git commit -m "test(ui): 升級確認畫面渲染測試"
```

---

## Task 12: Toolbar 顯示 ctrl+u 提示

**Files:**
- Modify: `internal/ui/app.go` (renderToolbar)
- Test: `internal/ui/app_test.go`

**Step 1: 寫測試**

```go
func TestToolbar_ShowsUpgradeHint_WhenUpgraderPresent(t *testing.T) {
	u := &upgrade.Upgrader{
		HTTPGet: func(url string) ([]byte, error) { return nil, nil },
	}
	m := ui.NewModel(ui.Deps{Upgrader: u})
	view := m.View()
	assert.Contains(t, view, "ctrl+u")
}

func TestToolbar_HidesUpgradeHint_WhenNoUpgrader(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	view := m.View()
	assert.NotContains(t, view, "ctrl+u")
}
```

**Step 2: 執行測試確認失敗**

```bash
go test ./internal/ui/... -v -race -run "TestToolbar_.*Upgrade"
```

預期：第一個 FAIL（toolbar 尚未顯示 ctrl+u）

**Step 3: 實作**

找到 `renderToolbar()` 函式，在既有的按鍵提示中，若 `m.deps.Upgrader != nil` 則加入 `ctrl+u 升級`。

**Step 4: 執行測試確認通過**

```bash
go test ./internal/ui/... -v -race -run "TestToolbar_.*Upgrade"
```

預期：PASS

**Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): toolbar 顯示 ctrl+u 升級提示"
```

---

## Task 13: 全量測試 + 版本推進

**Files:**
- Modify: `VERSION`
- Modify: `CHANGELOG.md` (如果存在)

**Step 1: 執行全部測試**

```bash
make test
```

預期：全部 PASS

**Step 2: 靜態分析**

```bash
make lint
```

預期：無問題

**Step 3: 編譯驗證**

```bash
make build
```

預期：成功

**Step 4: 更新版本號**

讀取 `VERSION` 檔案，MINOR +1：
```bash
# 假設目前為 0.23.1 → 推進到 0.24.0
echo "0.24.0" > VERSION
```

**Step 5: Commit**

```bash
git add VERSION
git commit -m "chore: v0.24.0 — in-TUI upgrade"
```
