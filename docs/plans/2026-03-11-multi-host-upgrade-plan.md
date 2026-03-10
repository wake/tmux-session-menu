# 多主機同步升級 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** ctrl+u 時同步升級所有主機（client + remote），TUI 內即時顯示進度。

**Architecture:** 擴充 gRPC proto 取得遠端版本 → 新增 `tsm upgrade --silent` 非互動模式 → TUI 新增 ModeUpgrade 面板驅動遠端 SSH 升級 → 本機最後升級。

**Tech Stack:** Go 1.24+, Protocol Buffers, gRPC, Bubble Tea TUI, SSH

**Design doc:** `docs/plans/2026-03-11-multi-host-upgrade-design.md`

---

### Task 1: gRPC proto — DaemonStatusResponse 加 version 欄位

**Files:**
- Modify: `api/proto/tsm/v1/tsm.proto:114-119`
- Regenerate: `api/tsm/v1/tsm.pb.go`（自動產生）
- Modify: `internal/daemon/service.go:184-189`
- Test: `internal/daemon/service_test.go`

**Step 1: 修改 proto 定義**

在 `api/proto/tsm/v1/tsm.proto` 的 `DaemonStatusResponse` 加入 `version` 欄位：

```protobuf
message DaemonStatusResponse {
  int32 pid = 1;
  google.protobuf.Timestamp started_at = 2;
  int32 watcher_count = 3;
  string version = 4;
}
```

**Step 2: 重新產生 Go 程式碼**

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       -I api/proto api/proto/tsm/v1/tsm.proto
```

若指令有差異，檢查 `api/tsm/v1/tsm.pb.go` 頂端的 `protoc-gen-go` 版本註解，對照產生。確認 `DaemonStatusResponse` 結構體多了 `Version string` 欄位。

**Step 3: 寫 daemon service 測試**

在 `internal/daemon/service_test.go` 修改既有的 `TestService_DaemonStatus`，加入 version 驗證：

```go
func TestService_DaemonStatus(t *testing.T) {
    // ... 既有的 setup ...
    resp, err := svc.DaemonStatus(context.Background(), &emptypb.Empty{})
    require.NoError(t, err)
    assert.NotZero(t, resp.Pid)
    assert.Equal(t, version.Version, resp.Version)
}
```

**Step 4: 執行測試確認失敗**

```bash
go test ./internal/daemon/... -v -race -run TestService_DaemonStatus
```

預期：FAIL（`resp.Version` 為空字串）

**Step 5: 修改 daemon service 實作**

在 `internal/daemon/service.go:184-189` 的 `DaemonStatus` 加入版本：

```go
func (s *Service) DaemonStatus(_ context.Context, _ *emptypb.Empty) (*tsmv1.DaemonStatusResponse, error) {
    return &tsmv1.DaemonStatusResponse{
        Pid:          int32(os.Getpid()),
        StartedAt:    timestamppb.New(s.startedAt),
        WatcherCount: int32(s.hub.Count()),
        Version:      version.Version,
    }, nil
}
```

需要在 `service.go` 頂部加入 import：`"github.com/wake/tmux-session-menu/internal/version"`

**Step 6: 執行測試確認通過**

```bash
go test ./internal/daemon/... -v -race -run TestService_DaemonStatus
```

預期：PASS

**Step 7: Commit**

```bash
git add api/ internal/daemon/service.go internal/daemon/service_test.go
git commit -m "feat: DaemonStatusResponse 加入 version 欄位"
```

---

### Task 2: `tsm upgrade --silent` CLI

**Files:**
- Modify: `cmd/tsm/main.go:129-132`（旗標解析）
- Create: `internal/upgrade/silent.go`
- Create: `internal/upgrade/silent_test.go`

**Step 1: 寫 RunSilent 測試**

建立 `internal/upgrade/silent_test.go`：

```go
package upgrade

import (
    "fmt"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestRunSilent_AlreadyLatest(t *testing.T) {
    ops := SilentOps{
        CurrentVersion: "0.28.0",
        CheckLatest: func() (Release, error) {
            return Release{Version: "0.28.0"}, nil
        },
    }
    result, err := RunSilent(ops)
    require.NoError(t, err)
    assert.Equal(t, "0.28.0", result.Version)
    assert.False(t, result.Upgraded)
}

func TestRunSilent_UpgradeSuccess(t *testing.T) {
    var installedFrom string
    var daemonStopped, daemonStarted bool
    ops := SilentOps{
        CurrentVersion: "0.27.0",
        CheckLatest: func() (Release, error) {
            return Release{
                Version: "0.28.0",
                Assets:  map[string]string{AssetName(): "https://example.com/dl"},
            }, nil
        },
        Download: func(url string) (string, error) {
            return "/tmp/tsm-upgrade-test", nil
        },
        InstallBinary: func(tmpPath string) (string, error) {
            installedFrom = tmpPath
            return "/usr/local/bin/tsm", nil
        },
        RemoveTmp: func(path string) {},
        StopDaemon: func() error {
            daemonStopped = true
            return nil
        },
        StartDaemon: func() error {
            daemonStarted = true
            return nil
        },
    }
    result, err := RunSilent(ops)
    require.NoError(t, err)
    assert.Equal(t, "0.28.0", result.Version)
    assert.True(t, result.Upgraded)
    assert.Equal(t, "/tmp/tsm-upgrade-test", installedFrom)
    assert.True(t, daemonStopped)
    assert.True(t, daemonStarted)
}

func TestRunSilent_CheckLatestError(t *testing.T) {
    ops := SilentOps{
        CurrentVersion: "0.27.0",
        CheckLatest: func() (Release, error) {
            return Release{}, fmt.Errorf("network error")
        },
    }
    _, err := RunSilent(ops)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "network error")
}

func TestRunSilent_NoAssetForPlatform(t *testing.T) {
    ops := SilentOps{
        CurrentVersion: "0.27.0",
        CheckLatest: func() (Release, error) {
            return Release{Version: "0.28.0", Assets: map[string]string{}}, nil
        },
    }
    _, err := RunSilent(ops)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "找不到適用於此平台的檔案")
}

func TestRunSilent_DownloadError(t *testing.T) {
    ops := SilentOps{
        CurrentVersion: "0.27.0",
        CheckLatest: func() (Release, error) {
            return Release{
                Version: "0.28.0",
                Assets:  map[string]string{AssetName(): "https://example.com/dl"},
            }, nil
        },
        Download: func(url string) (string, error) {
            return "", fmt.Errorf("download failed")
        },
    }
    _, err := RunSilent(ops)
    assert.Error(t, err)
}

func TestRunSilent_StartDaemonError(t *testing.T) {
    ops := SilentOps{
        CurrentVersion: "0.27.0",
        CheckLatest: func() (Release, error) {
            return Release{
                Version: "0.28.0",
                Assets:  map[string]string{AssetName(): "https://example.com/dl"},
            }, nil
        },
        Download: func(url string) (string, error) {
            return "/tmp/test", nil
        },
        InstallBinary: func(tmpPath string) (string, error) {
            return "/usr/local/bin/tsm", nil
        },
        RemoveTmp:  func(path string) {},
        StopDaemon: func() error { return nil },
        StartDaemon: func() error {
            return fmt.Errorf("daemon start failed")
        },
    }
    _, err := RunSilent(ops)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "daemon start failed")
}
```

**Step 2: 執行測試確認失敗**

```bash
go test ./internal/upgrade/... -v -race -run TestRunSilent
```

預期：編譯失敗（`RunSilent` 未定義）

**Step 3: 實作 RunSilent**

建立 `internal/upgrade/silent.go`：

```go
package upgrade

import "fmt"

// SilentOps 封裝 silent 升級的可注入依賴。
type SilentOps struct {
    CurrentVersion string
    CheckLatest    func() (Release, error)
    Download       func(url string) (string, error)
    InstallBinary  func(tmpPath string) (installedPath string, err error)
    RemoveTmp      func(path string)
    StopDaemon     func() error
    StartDaemon    func() error
}

// SilentResult 封裝 silent 升級的結果。
type SilentResult struct {
    Version  string // 最終版本號
    Upgraded bool   // 是否有執行升級
}

// RunSilent 執行非互動升級。
// 已是最新 → 回傳當前版本 + Upgraded=false。
// 需升級 → 下載 → 安裝 → 重啟 daemon → 回傳新版本 + Upgraded=true。
func RunSilent(ops SilentOps) (SilentResult, error) {
    rel, err := ops.CheckLatest()
    if err != nil {
        return SilentResult{}, fmt.Errorf("檢查最新版本失敗: %w", err)
    }

    if !NeedsUpgrade(ops.CurrentVersion, rel.Version) {
        return SilentResult{Version: ops.CurrentVersion, Upgraded: false}, nil
    }

    asset := AssetName()
    url, ok := rel.Assets[asset]
    if !ok {
        return SilentResult{}, fmt.Errorf("找不到適用於此平台的檔案 (%s)", asset)
    }

    tmpPath, err := ops.Download(url)
    if err != nil {
        return SilentResult{}, fmt.Errorf("下載失敗: %w", err)
    }

    _, err = ops.InstallBinary(tmpPath)
    if err != nil {
        ops.RemoveTmp(tmpPath)
        return SilentResult{}, fmt.Errorf("安裝失敗: %w", err)
    }
    ops.RemoveTmp(tmpPath)

    // daemon stop 失敗不阻斷
    _ = ops.StopDaemon()

    if err := ops.StartDaemon(); err != nil {
        return SilentResult{}, fmt.Errorf("啟動 daemon 失敗: %w", err)
    }

    return SilentResult{Version: rel.Version, Upgraded: true}, nil
}
```

**Step 4: 執行測試確認通過**

```bash
go test ./internal/upgrade/... -v -race -run TestRunSilent
```

預期：全部 PASS

**Step 5: 接線 CLI 入口**

修改 `cmd/tsm/main.go:129-132`，解析 `--silent` 旗標：

```go
if args[0] == "upgrade" {
    force := containsFlag(args[1:], "--force") || containsFlag(args[1:], "-f")
    silent := containsFlag(args[1:], "--silent")
    if silent {
        runSilentUpgrade()
        return
    }
    runUpgrade(force)
    return
}
```

在 `cmd/tsm/main.go` 新增 `runSilentUpgrade` 函式：

```go
func runSilentUpgrade() {
    cfg := loadConfig()
    u := upgrade.DefaultUpgrader()

    ops := upgrade.SilentOps{
        CurrentVersion: version.Version,
        CheckLatest:    u.CheckLatest,
        Download:       u.Download,
        InstallBinary: func(tmpPath string) (string, error) {
            det := selfinstall.Detect()
            targetDir := filepath.Dir(det.Path)
            if targetDir == "" || targetDir == "." {
                home, err := os.UserHomeDir()
                if err != nil {
                    return "", fmt.Errorf("無法取得 home 目錄: %w", err)
                }
                targetDir = filepath.Join(home, ".local", "bin")
            }
            target := filepath.Join(targetDir, "tsm")
            if err := os.MkdirAll(targetDir, 0o755); err != nil {
                return "", fmt.Errorf("無法建立目錄: %w", err)
            }
            tmpTarget := target + ".tmp"
            data, err := os.ReadFile(tmpPath)
            if err != nil {
                return "", fmt.Errorf("讀取下載檔案失敗: %w", err)
            }
            if err := os.WriteFile(tmpTarget, data, 0o755); err != nil {
                return "", fmt.Errorf("寫入暫存檔失敗: %w", err)
            }
            if err := os.Rename(tmpTarget, target); err != nil {
                os.Remove(tmpTarget)
                return "", fmt.Errorf("替換執行檔失敗: %w", err)
            }
            return target, nil
        },
        RemoveTmp: func(path string) { os.Remove(path) },
        StopDaemon: func() error {
            if daemon.IsRunning(cfg) {
                return daemon.Stop(cfg)
            }
            return nil
        },
        StartDaemon: func() error {
            return daemon.Start(cfg)
        },
    }

    result, err := upgrade.RunSilent(ops)
    if err != nil {
        fmt.Fprintln(os.Stderr, err.Error())
        os.Exit(1)
    }
    fmt.Println(result.Version)
}
```

更新 `printUsage()` 的 upgrade 行：

```go
fmt.Fprintln(os.Stderr, "  upgrade [--force] [--silent]  檢查並升級到最新版本")
```

**Step 6: 執行全部測試**

```bash
make test
```

預期：全部 PASS

**Step 7: Commit**

```bash
git add internal/upgrade/silent.go internal/upgrade/silent_test.go cmd/tsm/main.go
git commit -m "feat: tsm upgrade --silent 非互動升級模式"
```

---

### Task 3: CheckUpgradeMsg 擴充遠端版本

**Files:**
- Modify: `internal/ui/app.go`（CheckUpgradeMsg 結構、checkUpgradeCmd、Update handler）

**Step 1: 寫測試**

在 `internal/ui/app_test.go` 新增：

```go
func TestModel_CheckUpgradeMsg_EntersModeUpgrade(t *testing.T) {
    // 建立含 HostMgr 的 model（有 local + 1 remote）
    mgr := hostmgr.New()
    mgr.AddHost(config.HostEntry{Name: "local", Address: "local", Enabled: true})
    mgr.AddHost(config.HostEntry{Name: "air-2019", Address: "air-2019", Enabled: true})

    deps := ui.Deps{
        HostMgr:  mgr,
        Upgrader: &upgrade.Upgrader{},
    }
    m := ui.NewModel(deps)

    // 模擬 CheckUpgradeMsg，含遠端版本
    msg := ui.CheckUpgradeMsg{
        Release: &upgrade.Release{Version: "0.28.0"},
        HostVersions: map[string]string{
            "local":    "0.27.0",
            "air-2019": "0.26.0",
        },
    }
    result, _ := m.Update(msg)
    model := result.(ui.Model)

    assert.Equal(t, ui.ModeUpgrade, model.Mode())
}
```

**Step 2: 執行測試確認失敗**

```bash
go test ./internal/ui/... -v -race -run TestModel_CheckUpgradeMsg_EntersModeUpgrade
```

預期：編譯失敗（`ModeUpgrade` 和 `HostVersions` 未定義）

**Step 3: 擴充 CheckUpgradeMsg 與 ModeUpgrade**

在 `internal/ui/mode.go` 加入：

```go
ModeUpgrade  // 升級面板
```

在 `internal/ui/app.go` 修改 `CheckUpgradeMsg`：

```go
type CheckUpgradeMsg struct {
    Release       *upgrade.Release
    Err           error
    InstalledVer  string
    InstalledPath string
    HostVersions  map[string]string // hostID → version（遠端 DaemonStatus 取得）
}
```

在 Model 結構加入升級面板所需的狀態欄位：

```go
// 升級面板狀態
upgradeItems      []upgradeItem  // 升級清單
upgradeCursor     int            // 游標位置（含按鈕）
upgradeRunning    bool           // 是否升級中
upgradeCancelled  bool           // 使用者按 Esc 中止
upgradeLatestVer  string         // 最新版本號
upgradeBtnFocus   int            // 0=升級按鈕, 1=取消按鈕（游標在按鈕區時）
```

定義 `upgradeItem` 結構：

```go
type upgradeItem struct {
    HostID    string
    Name      string
    Address   string // 遠端 SSH 地址（local 為空）
    IsLocal   bool
    Version   string // 當前版本
    Checked   bool
    Status    upgradeStatus // pending/running/success/failed
    NewVer    string        // 升級後版本
    Error     string        // 失敗原因
}

type upgradeStatus int

const (
    upgradePending upgradeStatus = iota
    upgradeRunning_
    upgradeSuccess
    upgradeFailed
)
```

修改 `checkUpgradeCmd` 函式，並行取得遠端版本：

```go
func checkUpgradeCmd(u *upgrade.Upgrader, mgr *hostmgr.HostManager) tea.Cmd {
    return func() tea.Msg {
        rel, err := u.CheckLatest()

        det := selfinstall.Detect()
        var installedVer, installedPath string
        if det.Version != "" {
            installedVer = upgrade.ExtractSemver(det.Version)
            installedPath = det.Path
        }

        // 收集遠端版本
        hostVersions := make(map[string]string)
        if mgr != nil {
            hostVersions["local"] = version.Version
            var mu sync.Mutex
            var wg sync.WaitGroup
            for _, h := range mgr.Hosts() {
                if h.IsLocal() || h.Client() == nil {
                    continue
                }
                wg.Add(1)
                go func(host *hostmgr.Host) {
                    defer wg.Done()
                    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
                    defer cancel()
                    resp, rpcErr := host.Client().DaemonStatus(ctx)
                    mu.Lock()
                    defer mu.Unlock()
                    if rpcErr != nil || resp.Version == "" {
                        hostVersions[host.ID()] = ""
                    } else {
                        hostVersions[host.ID()] = resp.Version
                    }
                }(h)
            }
            wg.Wait()
        }

        if err != nil {
            return CheckUpgradeMsg{Err: err, InstalledVer: installedVer, InstalledPath: installedPath, HostVersions: hostVersions}
        }
        return CheckUpgradeMsg{Release: &rel, InstalledVer: installedVer, InstalledPath: installedPath, HostVersions: hostVersions}
    }
}
```

修改 `Update` 的 `CheckUpgradeMsg` 處理，有 HostMgr 時進入 ModeUpgrade：

```go
case CheckUpgradeMsg:
    m.upgradeChecking = false
    if msg.Err != nil {
        // 既有邏輯（ModeConfirm）保留用於無 HostMgr 的情境
        if m.deps.HostMgr == nil {
            // ... 既有的 installed version 偵測 + ModeConfirm ...
        }
        m.mode = ModeConfirm
        m.confirmPrompt = fmt.Sprintf("檢查失敗：%v", msg.Err)
        return m, nil
    }
    if m.deps.HostMgr != nil && msg.HostVersions != nil {
        return m.enterModeUpgrade(msg.Release.Version, msg.HostVersions), nil
    }
    // 既有邏輯用於無 HostMgr 的舊路徑
    // ...
```

`enterModeUpgrade` 方法建立 upgradeItems：

```go
func (m Model) enterModeUpgrade(latestVer string, hostVersions map[string]string) Model {
    m.mode = ModeUpgrade
    m.upgradeLatestVer = latestVer
    m.upgradeCursor = 0
    m.upgradeRunning = false
    m.upgradeCancelled = false
    m.upgradeBtnFocus = 0

    var items []upgradeItem
    // local 固定第一
    if v, ok := hostVersions["local"]; ok {
        needsUp := upgrade.NeedsUpgrade(v, latestVer)
        items = append(items, upgradeItem{
            HostID:  "local",
            Name:    "local",
            IsLocal: true,
            Version: v,
            Checked: needsUp || v == "",
        })
    }
    // 遠端
    if m.deps.HostMgr != nil {
        for _, h := range m.deps.HostMgr.Hosts() {
            if h.IsLocal() {
                continue
            }
            cfg := h.Config()
            v := hostVersions[h.ID()]
            display := v
            if display == "" {
                display = "未知"
            }
            needsUp := v == "" || upgrade.NeedsUpgrade(v, latestVer)
            items = append(items, upgradeItem{
                HostID:  h.ID(),
                Name:    cfg.Name,
                Address: cfg.Address,
                Version: display,
                Checked: needsUp,
            })
        }
    }
    m.upgradeItems = items
    return m
}
```

修改 ctrl+u 觸發處，傳入 HostMgr：

```go
case "ctrl+u":
    if m.deps.Upgrader == nil || m.upgradeChecking {
        return m, nil
    }
    m.upgradeChecking = true
    return m, checkUpgradeCmd(m.deps.Upgrader, m.deps.HostMgr)
```

**Step 4: 執行測試確認通過**

```bash
go test ./internal/ui/... -v -race -run TestModel_CheckUpgradeMsg_EntersModeUpgrade
```

預期：PASS

**Step 5: Commit**

```bash
git add internal/ui/mode.go internal/ui/app.go internal/ui/app_test.go
git commit -m "feat: CheckUpgradeMsg 擴充遠端版本，新增 ModeUpgrade"
```

---

### Task 4: ModeUpgrade 鍵盤操作 (updateUpgrade)

**Files:**
- Create: `internal/ui/upgrade.go`
- Create: `internal/ui/upgrade_test.go`

**Step 1: 寫測試**

建立 `internal/ui/upgrade_test.go`：

```go
package ui_test

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/stretchr/testify/assert"
    "github.com/wake/tmux-session-menu/internal/ui"
    "github.com/wake/tmux-session-menu/internal/upgrade"
    "github.com/wake/tmux-session-menu/internal/hostmgr"
    "github.com/wake/tmux-session-menu/internal/config"
)

func setupUpgradeModel() ui.Model {
    mgr := hostmgr.New()
    mgr.AddHost(config.HostEntry{Name: "local", Address: "local", Enabled: true})
    mgr.AddHost(config.HostEntry{Name: "air-2019", Address: "air-2019", Enabled: true})

    deps := ui.Deps{HostMgr: mgr, Upgrader: &upgrade.Upgrader{}}
    m := ui.NewModel(deps)

    // 模擬進入 ModeUpgrade
    msg := ui.CheckUpgradeMsg{
        Release:      &upgrade.Release{Version: "0.28.0"},
        HostVersions: map[string]string{"local": "0.27.0", "air-2019": "0.26.0"},
    }
    result, _ := m.Update(msg)
    return result.(ui.Model)
}

func TestUpgrade_SpaceTogglesCheck(t *testing.T) {
    m := setupUpgradeModel()
    assert.Equal(t, ui.ModeUpgrade, m.Mode())

    // 第一項（local）預設勾選，按 space 取消
    result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
    model := result.(ui.Model)
    items := model.UpgradeItems()
    assert.False(t, items[0].Checked)
}

func TestUpgrade_CursorMovement(t *testing.T) {
    m := setupUpgradeModel()

    // 按 j 往下
    result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
    model := result.(ui.Model)
    assert.Equal(t, 1, model.UpgradeCursor())

    // 按 k 往上
    result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
    model = result.(ui.Model)
    assert.Equal(t, 0, model.UpgradeCursor())
}

func TestUpgrade_EscReturnToNormal(t *testing.T) {
    m := setupUpgradeModel()
    result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
    model := result.(ui.Model)
    assert.Equal(t, ui.ModeNormal, model.Mode())
}

func TestUpgrade_AToggleAll(t *testing.T) {
    m := setupUpgradeModel()

    // a 全不選（因為預設全勾）
    result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
    model := result.(ui.Model)
    for _, item := range model.UpgradeItems() {
        assert.False(t, item.Checked)
    }

    // a 全選
    result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
    model = result.(ui.Model)
    for _, item := range model.UpgradeItems() {
        assert.True(t, item.Checked)
    }
}

func TestUpgrade_CursorMovesToButtons(t *testing.T) {
    m := setupUpgradeModel()
    // 有 2 個 host items，cursor 從 0 開始
    // 按 j 兩次到按鈕區
    result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
    result, _ = result.(ui.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
    model := result.(ui.Model)
    // cursor 應在按鈕區（index == len(items)）
    assert.Equal(t, 2, model.UpgradeCursor())
}
```

**Step 2: 執行測試確認失敗**

```bash
go test ./internal/ui/... -v -race -run TestUpgrade_
```

預期：編譯失敗（`UpgradeItems()`、`UpgradeCursor()` 未定義）

**Step 3: 實作 updateUpgrade**

建立 `internal/ui/upgrade.go`：

```go
package ui

import tea "github.com/charmbracelet/bubbletea"

// UpgradeItems 回傳升級清單（測試用）。
func (m Model) UpgradeItems() []upgradeItem { return m.upgradeItems }

// UpgradeCursor 回傳升級面板的游標位置（測試用）。
func (m Model) UpgradeCursor() int { return m.upgradeCursor }

// upgradeItemCount 回傳清單項目數（不含按鈕列）。
func (m Model) upgradeItemCount() int { return len(m.upgradeItems) }

// upgradeCursorOnButtons 回傳游標是否在按鈕區。
func (m Model) upgradeCursorOnButtons() bool {
    return m.upgradeCursor >= m.upgradeItemCount()
}

// updateUpgrade 處理 ModeUpgrade 的按鍵。
func (m Model) updateUpgrade(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    if m.upgradeRunning {
        // 升級進行中只允許 Esc
        if msg.String() == "esc" {
            m.upgradeCancelled = true
        }
        return m, nil
    }

    itemCount := m.upgradeItemCount()
    // 按鈕區有 2 個位置：0=升級, 1=取消
    maxCursor := itemCount + 1 // items + 按鈕列（視為一行，左右切換 focus）

    switch msg.String() {
    case "esc":
        m.mode = ModeNormal
        return m, nil
    case "j", "down":
        if m.upgradeCursor < itemCount { // 還在 items 區域可以往下
            m.upgradeCursor++
        }
    case "k", "up":
        if m.upgradeCursor > 0 {
            m.upgradeCursor--
        }
    case "l", "right", "tab":
        if m.upgradeCursorOnButtons() && m.upgradeBtnFocus < 1 {
            m.upgradeBtnFocus = 1
        }
    case "h", "left", "shift+tab":
        if m.upgradeCursorOnButtons() && m.upgradeBtnFocus > 0 {
            m.upgradeBtnFocus = 0
        }
    case " ":
        if !m.upgradeCursorOnButtons() && m.upgradeCursor < itemCount {
            m.upgradeItems[m.upgradeCursor].Checked = !m.upgradeItems[m.upgradeCursor].Checked
        }
    case "a":
        // 全選/全不選 toggle
        allChecked := true
        for _, item := range m.upgradeItems {
            if !item.Checked {
                allChecked = false
                break
            }
        }
        for i := range m.upgradeItems {
            m.upgradeItems[i].Checked = !allChecked
        }
    case "ctrl+u":
        return m.startUpgrade()
    case "enter":
        if m.upgradeCursorOnButtons() {
            if m.upgradeBtnFocus == 0 {
                return m.startUpgrade()
            }
            // 取消按鈕
            m.mode = ModeNormal
            return m, nil
        }
    }
    // 確保 maxCursor 範圍
    if m.upgradeCursor > maxCursor {
        m.upgradeCursor = maxCursor
    }
    return m, nil
}

// startUpgrade 開始升級流程。
func (m Model) startUpgrade() (tea.Model, tea.Cmd) {
    // 收集需要升級的遠端主機
    var remoteTargets []upgradeItem
    var localChecked bool
    for i := range m.upgradeItems {
        if m.upgradeItems[i].Checked {
            if m.upgradeItems[i].IsLocal {
                localChecked = true
                if len(remoteTargets) == 0 {
                    // 只有 local 被勾選，沒有遠端 → 直接開始 local 升級
                }
            } else {
                m.upgradeItems[i].Status = upgradeRunning_
                remoteTargets = append(remoteTargets, m.upgradeItems[i])
            }
        }
    }

    if len(remoteTargets) == 0 && !localChecked {
        return m, nil // 無勾選項
    }

    m.upgradeRunning = true
    m.upgradeCancelled = false

    if len(remoteTargets) == 0 && localChecked {
        // 只有 local → 直接走本機升級
        return m, m.startLocalUpgradeCmd()
    }

    // 有遠端 → 並行 SSH 升級
    var cmds []tea.Cmd
    for _, target := range remoteTargets {
        t := target
        cmds = append(cmds, remoteUpgradeCmd(t.Address, t.HostID))
    }
    return m, tea.Batch(cmds...)
}
```

`app.go` 的 `Update` switch 加入：

```go
case ModeUpgrade:
    return m.updateUpgrade(msg)
```

**Step 4: 加入測試用 accessor 方法**

這些已在 `upgrade.go` 中定義（`UpgradeItems()`、`UpgradeCursor()`）。

**Step 5: 執行測試確認通過**

```bash
go test ./internal/ui/... -v -race -run TestUpgrade_
```

預期：PASS

**Step 6: Commit**

```bash
git add internal/ui/upgrade.go internal/ui/upgrade_test.go internal/ui/app.go
git commit -m "feat: ModeUpgrade 鍵盤操作（移動、勾選、按鈕）"
```

---

### Task 5: 遠端 SSH 升級 + RemoteUpgradeMsg

**Files:**
- Modify: `internal/ui/upgrade.go`（remoteUpgradeCmd、Update handler）
- Modify: `internal/ui/upgrade_test.go`

**Step 1: 寫測試**

在 `internal/ui/upgrade_test.go` 新增：

```go
func TestUpgrade_RemoteUpgradeSuccess(t *testing.T) {
    m := setupUpgradeModel()

    // 模擬遠端升級成功
    msg := ui.RemoteUpgradeMsg{
        HostID:  "air-2019",
        Version: "0.28.0",
    }
    result, _ := m.Update(msg)
    model := result.(ui.Model)

    items := model.UpgradeItems()
    for _, item := range items {
        if item.HostID == "air-2019" {
            assert.Equal(t, "0.28.0", item.NewVer)
            assert.Empty(t, item.Error)
        }
    }
}

func TestUpgrade_RemoteUpgradeFailed(t *testing.T) {
    m := setupUpgradeModel()

    msg := ui.RemoteUpgradeMsg{
        HostID: "air-2019",
        Error:  "connection refused",
    }
    result, _ := m.Update(msg)
    model := result.(ui.Model)

    items := model.UpgradeItems()
    for _, item := range items {
        if item.HostID == "air-2019" {
            assert.Equal(t, "connection refused", item.Error)
        }
    }
}
```

**Step 2: 執行測試確認失敗**

```bash
go test ./internal/ui/... -v -race -run TestUpgrade_Remote
```

預期：編譯失敗（`RemoteUpgradeMsg` 未定義）

**Step 3: 實作**

在 `internal/ui/upgrade.go` 加入：

```go
// RemoteUpgradeMsg 單台遠端升級結果。
type RemoteUpgradeMsg struct {
    HostID  string
    Version string // 成功時的新版本
    Error   string // 失敗原因
}

// remoteUpgradeCmd 回傳一個 Cmd，SSH 執行遠端 tsm upgrade --silent。
func remoteUpgradeCmd(address, hostID string) tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
        defer cancel()

        cmd := exec.CommandContext(ctx, "ssh", address, "tsm", "upgrade", "--silent")
        var stdout, stderr bytes.Buffer
        cmd.Stdout = &stdout
        cmd.Stderr = &stderr

        err := cmd.Run()
        if err != nil {
            errMsg := strings.TrimSpace(stderr.String())
            if errMsg == "" {
                errMsg = err.Error()
            }
            return RemoteUpgradeMsg{HostID: hostID, Error: errMsg}
        }

        ver := strings.TrimSpace(stdout.String())
        return RemoteUpgradeMsg{HostID: hostID, Version: ver}
    }
}
```

在 `app.go` 的 `Update` 方法，`case tea.KeyMsg:` 之前加入 `RemoteUpgradeMsg` handler：

```go
case RemoteUpgradeMsg:
    return m.handleRemoteUpgrade(msg), nil
```

在 `upgrade.go` 實作 handler：

```go
func (m Model) handleRemoteUpgrade(msg RemoteUpgradeMsg) Model {
    for i := range m.upgradeItems {
        if m.upgradeItems[i].HostID == msg.HostID {
            if msg.Error != "" {
                m.upgradeItems[i].Status = upgradeFailed
                m.upgradeItems[i].Error = msg.Error
                m.upgradeItems[i].Checked = true // 失敗項自動勾選方便重試
            } else {
                m.upgradeItems[i].Status = upgradeSuccess
                m.upgradeItems[i].NewVer = msg.Version
                m.upgradeItems[i].Checked = false
            }
            break
        }
    }

    // 檢查是否所有遠端都完成
    allRemoteDone := true
    for _, item := range m.upgradeItems {
        if !item.IsLocal && item.Status == upgradeRunning_ {
            allRemoteDone = false
            break
        }
    }

    if allRemoteDone {
        if m.upgradeCancelled {
            m.upgradeRunning = false
            return m
        }
        // 檢查 local 是否需要升級
        for _, item := range m.upgradeItems {
            if item.IsLocal && item.Checked {
                // 觸發本機升級（下載 → 安裝 → 退出）
                // 複用既有的 downloadUpgradeCmd
                // TODO: 在 startLocalUpgradeCmd 中實作
                return m
            }
        }
        // local 未勾選 → 完成
        m.upgradeRunning = false
    }
    return m
}
```

**Step 4: 執行測試確認通過**

```bash
go test ./internal/ui/... -v -race -run TestUpgrade_Remote
```

預期：PASS

**Step 5: Commit**

```bash
git add internal/ui/upgrade.go internal/ui/upgrade_test.go internal/ui/app.go
git commit -m "feat: RemoteUpgradeMsg 處理遠端升級結果"
```

---

### Task 6: 本機升級銜接

**Files:**
- Modify: `internal/ui/upgrade.go`（startLocalUpgradeCmd）
- Modify: `internal/ui/app.go`（DownloadUpgradeMsg 在 ModeUpgrade 的處理）

**Step 1: 寫測試**

在 `internal/ui/upgrade_test.go` 新增：

```go
func TestUpgrade_LocalUpgradeSetsReadyAndQuits(t *testing.T) {
    m := setupUpgradeModel()

    // 模擬已在 ModeUpgrade 且 upgradeRunning=true
    // 收到 DownloadUpgradeMsg
    msg := ui.DownloadUpgradeMsg{TmpPath: "/tmp/tsm-upgrade-test"}
    result, cmd := m.Update(msg)
    model := result.(ui.Model)

    assert.True(t, model.UpgradeReady())
    // cmd 應該是 tea.Quit
    assert.NotNil(t, cmd)
}
```

**Step 2: 實作 startLocalUpgradeCmd**

在 `upgrade.go`：

```go
func (m Model) startLocalUpgradeCmd() tea.Cmd {
    if m.deps.Upgrader == nil {
        return nil
    }
    asset := upgrade.AssetName()
    // 從 upgradeLatestVer 對應的 release 取 URL
    // 需要在 enterModeUpgrade 時保存 release
    u := m.deps.Upgrader
    return func() tea.Msg {
        rel, err := u.CheckLatest()
        if err != nil {
            return DownloadUpgradeMsg{Err: err}
        }
        url, ok := rel.Assets[asset]
        if !ok {
            return DownloadUpgradeMsg{Err: fmt.Errorf("找不到適用於此平台的檔案 (%s)", asset)}
        }
        path, err := u.Download(url)
        if err != nil {
            return DownloadUpgradeMsg{Err: err}
        }
        return DownloadUpgradeMsg{TmpPath: path}
    }
}
```

既有的 `DownloadUpgradeMsg` handler 已處理 `upgradeReady` + `tea.Quit`，在 ModeUpgrade 時也適用。在 `handleRemoteUpgrade` 的 `allRemoteDone && local.Checked` 分支觸發：

```go
if item.IsLocal && item.Checked {
    m.upgradeItems[i].Status = upgradeRunning_
    // 使用閉包回傳 cmd
    return m // caller 需要回傳 cmd
}
```

注意：`handleRemoteUpgrade` 目前回傳 `Model`，需要改為回傳 `(Model, tea.Cmd)` 以支援觸發 local upgrade cmd。修改：

```go
case RemoteUpgradeMsg:
    m, cmd := m.handleRemoteUpgrade(msg)
    return m, cmd
```

```go
func (m Model) handleRemoteUpgrade(msg RemoteUpgradeMsg) (Model, tea.Cmd) {
    // ... 同前 ...
    if allRemoteDone && !m.upgradeCancelled {
        for i, item := range m.upgradeItems {
            if item.IsLocal && item.Checked {
                m.upgradeItems[i].Status = upgradeRunning_
                m.upgradeVersion = m.upgradeLatestVer
                return m, m.startLocalUpgradeCmd()
            }
        }
        m.upgradeRunning = false
    }
    return m, nil
}
```

**Step 3: 執行測試確認通過**

```bash
go test ./internal/ui/... -v -race -run TestUpgrade_
```

預期：PASS

**Step 4: Commit**

```bash
git add internal/ui/upgrade.go internal/ui/upgrade_test.go internal/ui/app.go
git commit -m "feat: ModeUpgrade 本機升級銜接（download → ready → quit）"
```

---

### Task 7: renderUpgrade 渲染

**Files:**
- Modify: `internal/ui/upgrade.go`（renderUpgrade）
- Modify: `internal/ui/styles.go`（tab 樣式提取）
- Modify: `internal/ui/app.go`（View 加入 ModeUpgrade 分支）

**Step 1: 提取共用 tab 樣式**

在 `internal/ui/styles.go` 加入 tab 按鈕樣式（與 cfgtui 一致）：

```go
activeTabStyle = lipgloss.NewStyle().
    Bold(true).
    Foreground(lipgloss.Color("#1a1b26")).
    Background(lipgloss.Color("#7aa2f7")).
    Padding(0, 1)
inactiveTabStyle = lipgloss.NewStyle().
    Foreground(lipgloss.Color("#787fa0")).
    Background(lipgloss.Color("#24283b")).
    Padding(0, 1)
```

**Step 2: 寫 renderUpgrade**

在 `internal/ui/upgrade.go` 加入渲染方法：

```go
func (m Model) renderUpgrade() string {
    var b strings.Builder
    b.WriteString("\n")
    b.WriteString(fmt.Sprintf("  %s\n\n", selectedStyle.Render(
        fmt.Sprintf("升級至 v%s", m.upgradeLatestVer))))

    for i, item := range m.upgradeItems {
        isCursor := i == m.upgradeCursor && !m.upgradeCursorOnButtons()

        // Checkbox
        var checkbox string
        switch item.Status {
        case upgradeSuccess:
            checkbox = successStyle.Render("[✓]")
        case upgradeFailed:
            checkbox = errorStyle.Render("[!]")
        case upgradeRunning_:
            checkbox = dimStyle.Render("[-]")
        default:
            if item.Checked {
                checkbox = "[x]"
            } else {
                checkbox = "[ ]"
            }
        }

        // 名稱
        name := item.Name

        // 版本
        ver := item.Version
        if ver == "" {
            ver = "未知"
        }

        // 狀態文字
        var statusText string
        switch item.Status {
        case upgradeRunning_:
            statusText = dimStyle.Render("更新中...")
        case upgradeSuccess:
            statusText = successStyle.Render(fmt.Sprintf("已更新 v%s", item.NewVer))
        case upgradeFailed:
            statusText = errorStyle.Render(fmt.Sprintf("升級失敗: %s", item.Error))
        default:
            if item.IsLocal && m.upgradeRunning && !m.upgradeCancelled {
                statusText = dimStyle.Render("等待遠端完成...")
            } else if !upgrade.NeedsUpgrade(ver, m.upgradeLatestVer) && ver != "未知" {
                statusText = dimStyle.Render("(已是最新)")
            }
        }

        line := fmt.Sprintf("  %s %-16s %s  %s", checkbox, name, dimStyle.Render(ver), statusText)
        if isCursor {
            line = m.cursorLine(line)
        }
        b.WriteString(line + "\n")
    }

    // 按鈕列
    b.WriteString("\n  ")
    onButtons := m.upgradeCursorOnButtons()
    if m.upgradeRunning {
        b.WriteString(dimStyle.Render("升級進行中...") + "  ")
        if onButtons {
            b.WriteString(activeTabStyle.Render("Esc 中止"))
        } else {
            b.WriteString(inactiveTabStyle.Render("Esc 中止"))
        }
    } else {
        // 升級按鈕
        if onButtons && m.upgradeBtnFocus == 0 {
            b.WriteString(activeTabStyle.Render("ctrl+u 升級"))
        } else {
            b.WriteString(inactiveTabStyle.Render("ctrl+u 升級"))
        }
        b.WriteString("  ")
        // 取消按鈕
        if onButtons && m.upgradeBtnFocus == 1 {
            b.WriteString(activeTabStyle.Render("Esc 取消"))
        } else {
            b.WriteString(inactiveTabStyle.Render("Esc 取消"))
        }
    }
    b.WriteString("\n")

    return b.String()
}
```

**Step 3: 在 View 加入 ModeUpgrade 分支**

在 `app.go` 的 `View()` 方法，找到模式渲染邏輯（通常在 `renderToolbar` 之前），加入：

```go
case ModeUpgrade:
    return m.renderUpgrade()
```

具體位置：找到 `m.mode` 的 switch/if 渲染邏輯，加入 ModeUpgrade 的渲染。

**Step 4: 執行全部測試**

```bash
make test
```

預期：全部 PASS

**Step 5: Commit**

```bash
git add internal/ui/upgrade.go internal/ui/styles.go internal/ui/app.go
git commit -m "feat: ModeUpgrade 渲染（主機清單 + tab 按鈕列）"
```

---

### Task 8: 整合測試與收尾

**Files:**
- Modify: `internal/ui/upgrade_test.go`（完整流程測試）
- Modify: `cmd/tsm/main.go`（確認 UpgradeReady 在 ModeUpgrade 後的處理）
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Step 1: 完整流程測試**

在 `internal/ui/upgrade_test.go` 加入端對端流程測試：

```go
func TestUpgrade_FullFlow_RemoteThenLocal(t *testing.T) {
    m := setupUpgradeModel()
    assert.Equal(t, ui.ModeUpgrade, m.Mode())

    // 觸發升級
    result, cmds := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x15}}) // ctrl+u
    model := result.(ui.Model)
    assert.True(t, model.UpgradeRunning())
    assert.NotNil(t, cmds) // 應該有 remoteUpgradeCmd

    // 模擬遠端成功
    result, _ = model.Update(ui.RemoteUpgradeMsg{HostID: "air-2019", Version: "0.28.0"})
    model = result.(ui.Model)

    // 遠端全完成，local 應開始升級
    for _, item := range model.UpgradeItems() {
        if item.HostID == "air-2019" {
            assert.Equal(t, "0.28.0", item.NewVer)
        }
    }
}

func TestUpgrade_EscDuringUpgradeCancels(t *testing.T) {
    m := setupUpgradeModel()

    // 觸發升級
    result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x15}}) // ctrl+u
    model := result.(ui.Model)

    // 按 Esc
    result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEscape})
    model = result.(ui.Model)
    assert.True(t, model.UpgradeCancelled())
}
```

新增 accessor 方法供測試用：
```go
func (m Model) UpgradeRunning() bool { return m.upgradeRunning }
func (m Model) UpgradeCancelled() bool { return m.upgradeCancelled }
```

**Step 2: 執行全部測試**

```bash
make test
make lint
```

預期：全部 PASS

**Step 3: 更新版本與 CHANGELOG**

`VERSION`：`0.28.0`（新功能 → MINOR bump）

`CHANGELOG.md` 頂部加入：

```markdown
## [0.28.0] - 2026-03-11

### Added
- 多主機同步升級：ctrl+u 顯示升級面板，列出所有主機版本，可勾選批次升級
- `tsm upgrade --silent` 非互動升級模式，供遠端 SSH 呼叫
- `DaemonStatusResponse.version` gRPC 欄位，支援查詢遠端 daemon 版本
- ModeUpgrade TUI 面板：即時顯示遠端升級進度，支援重試失敗項目
- 底部 tab 按鈕列（activeTabStyle / inactiveTabStyle），可鍵盤導航
```

**Step 4: Commit**

```bash
git add VERSION CHANGELOG.md internal/ui/upgrade.go internal/ui/upgrade_test.go
git commit -m "feat: 多主機同步升級 v0.28.0"
```

**Step 5: 最終驗證**

```bash
make build
make test
make lint
```

預期：全部 PASS
