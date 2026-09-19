# 統一 HostManager 啟動路徑 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 讓 `tsm` 所有啟動路徑統一走 HostManager，支援 `--host`/`--local` 旗標控制主機組合，取代原有的 `--remote`。

**Architecture:** 合併 `runTUI`/`runMultiHost`/`runTUIWithClient` 為統一入口，永遠建立 HostManager。CLI 旗標決定哪些 host enabled 並可選擇性寫回 config。UI 的 `FlattenMultiHost` 根據「只有 local enabled」來決定是否隱藏標題。

**Tech Stack:** Go 1.24+, Bubble Tea TUI, gRPC, testify

---

## Task 1: MergeHosts 加入 localFlag 參數

### Files:
- Modify: `internal/config/hosts.go`
- Modify: `internal/config/hosts_test.go`

### Step 1: 寫失敗測試 — MergeHosts 新簽名

在 `internal/config/hosts_test.go` 末尾加入新測試。注意：先改所有既有的 `MergeHosts` 呼叫，加上第三參數 `false`。

```go
// --- 更新所有既有呼叫 ---
// 所有 config.MergeHosts(hosts, xxx) → config.MergeHosts(hosts, xxx, false)

// --- 新增測試 ---

func TestMergeHostsLocalFlagOnly(t *testing.T) {
	// --local：enable local, disable 其餘
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: false, SortOrder: 0},
		{Name: "dev", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 1},
		{Name: "staging", Address: "10.0.0.2", Color: "#ff9e64", Enabled: true, SortOrder: 2},
	}

	result := config.MergeHosts(hosts, nil, true)

	require.Len(t, result, 3)
	assert.True(t, result[0].Enabled, "local 應啟用")
	assert.False(t, result[1].Enabled, "dev 應停用")
	assert.False(t, result[2].Enabled, "staging 應停用")
}

func TestMergeHostsLocalFlagWithHostFlags(t *testing.T) {
	// --local --host dev：enable local + dev, disable 其餘
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: false, SortOrder: 0},
		{Name: "dev", Address: "10.0.0.1", Color: "#73daca", Enabled: false, SortOrder: 1},
		{Name: "staging", Address: "10.0.0.2", Color: "#ff9e64", Enabled: true, SortOrder: 2},
	}

	result := config.MergeHosts(hosts, []string{"dev"}, true)

	require.Len(t, result, 3)
	assert.True(t, result[0].Enabled, "local 應啟用（--local）")
	assert.True(t, result[1].Enabled, "dev 應啟用（--host dev）")
	assert.False(t, result[2].Enabled, "staging 應停用")
}

func TestMergeHostsLocalFlagNoLocalInConfig(t *testing.T) {
	// --local 但 config 中沒有 local host → 自動補上
	hosts := []config.HostEntry{
		{Name: "dev", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 0},
	}

	result := config.MergeHosts(hosts, nil, true)

	require.True(t, len(result) >= 2)
	assert.True(t, result[0].IsLocal(), "應自動補上 local")
	assert.True(t, result[0].Enabled, "local 應啟用")
	assert.False(t, result[1].Enabled, "dev 應停用")
}

func TestMergeHostsNoFlags(t *testing.T) {
	// 無旗標（tsm 無參數、tsm --host 裸用）：不修改
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		{Name: "dev", Address: "10.0.0.1", Color: "#73daca", Enabled: false, SortOrder: 1},
	}

	result := config.MergeHosts(hosts, nil, false)

	require.Len(t, result, 2)
	assert.True(t, result[0].Enabled, "local 保持啟用")
	assert.False(t, result[1].Enabled, "dev 保持停用")
}
```

### Step 2: 跑測試確認失敗

Run: `go test ./internal/config/... -v -race -run TestMergeHosts`
Expected: 編譯失敗 — `MergeHosts` 簽名不匹配

### Step 3: 修改 MergeHosts 實作

修改 `internal/config/hosts.go`：

```go
// MergeHosts 整合 config 中的主機清單與 CLI 旗標。
//
// 當 hostFlags 和 localFlag 都為空/false：
//   - 不修改，直接回傳原清單副本
//
// 當 localFlag=true 且 hostFlags 為空：
//   - local host 啟用（不存在則自動補上）
//   - 其他主機全部停用
//
// 當 hostFlags 有值且 localFlag=false：
//   - 旗標中的主機啟用，不在旗標中的停用
//   - local host 停用
//   - 不在清單的主機自動新增
//
// 當 hostFlags 有值且 localFlag=true：
//   - local host 啟用 + 旗標中的主機啟用
//   - 其餘主機停用
//   - 不在清單的主機自動新增
func MergeHosts(hosts []HostEntry, hostFlags []string, localFlag bool) []HostEntry {
	if len(hostFlags) == 0 && !localFlag {
		// 無旗標：回傳副本不修改
		result := make([]HostEntry, len(hosts))
		copy(result, hosts)
		return result
	}
	if len(hostFlags) == 0 && localFlag {
		return mergeLocalOnly(hosts)
	}
	return mergeWithFlags(hosts, hostFlags, localFlag)
}
```

新增 `mergeLocalOnly`：

```go
// mergeLocalOnly 處理 --local（無 --host）的情況。
func mergeLocalOnly(hosts []HostEntry) []HostEntry {
	result := make([]HostEntry, len(hosts))
	copy(result, hosts)

	hasLocal := false
	for i := range result {
		if result[i].IsLocal() {
			result[i].Enabled = true
			hasLocal = true
		} else {
			result[i].Enabled = false
		}
	}

	if !hasLocal {
		local := HostEntry{
			Name:      "local",
			Address:   "",
			Color:     "#5f8787",
			Enabled:   true,
			SortOrder: 0,
		}
		result = append([]HostEntry{local}, result...)
	}

	return result
}
```

重新命名 `mergeWithRemote` → `mergeWithFlags`，加入 `localFlag` 參數：

```go
// mergeWithFlags 處理有 --host 旗標的情況。
func mergeWithFlags(hosts []HostEntry, hostFlags []string, localFlag bool) []HostEntry {
	flagSet := make(map[string]bool, len(hostFlags))
	for _, f := range hostFlags {
		flagSet[f] = true
	}

	usedColors := make(map[string]bool)
	for _, h := range hosts {
		if h.Color != "" {
			usedColors[h.Color] = true
		}
	}

	result := make([]HostEntry, len(hosts))
	copy(result, hosts)

	matched := make(map[string]bool, len(hostFlags))

	for i := range result {
		if result[i].IsLocal() {
			result[i].Enabled = localFlag
			continue
		}

		if flagSet[result[i].Name] {
			result[i].Enabled = true
			matched[result[i].Name] = true
		} else if flagSet[result[i].Address] {
			result[i].Enabled = true
			matched[result[i].Address] = true
		} else {
			result[i].Enabled = false
		}
	}

	for _, f := range hostFlags {
		if matched[f] {
			continue
		}
		newHost := HostEntry{
			Name:      f,
			Address:   f,
			Color:     pickColor(usedColors),
			Enabled:   true,
			SortOrder: len(result),
		}
		usedColors[newHost.Color] = true
		result = append(result, newHost)
	}

	return result
}
```

刪除舊的 `mergeNoRemote` 和 `mergeWithRemote`。

### Step 4: 跑測試確認通過

Run: `go test ./internal/config/... -v -race`
Expected: 全部 PASS

### Step 5: 更新既有 MergeHosts 呼叫者

`cmd/tsm/main.go` line 228 的呼叫也需要加第三參數（暫時 `false`，後續 Task 會正式改）：

```go
// 舊：merged := config.MergeHosts(cfg.Hosts, remoteFlags)
// 新：merged := config.MergeHosts(cfg.Hosts, remoteFlags, false)
```

### Step 6: 全域測試確認

Run: `make test`
Expected: 全部 PASS

### Step 7: Commit

```bash
git add internal/config/hosts.go internal/config/hosts_test.go cmd/tsm/main.go
git commit -m "feat: MergeHosts 加入 localFlag 參數，支援 --local 語意"
```

---

## Task 2: CLI 解析 — `--host`/`--local` 取代 `--remote`

### Files:
- Modify: `cmd/tsm/parse_test.go`
- Modify: `cmd/tsm/main.go`

### Step 1: 寫失敗測試

改寫 `cmd/tsm/parse_test.go`：

```go
package main

import (
	"testing"
)

func TestParseHostFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "無 host 參數",
			args: []string{"--inline"},
			want: nil,
		},
		{
			name: "單一 host",
			args: []string{"--host", "hostA"},
			want: []string{"hostA"},
		},
		{
			name: "多個 host",
			args: []string{"--host", "hostA", "--host", "hostB"},
			want: []string{"hostA", "hostB"},
		},
		{
			name: "host 混合其他 flag",
			args: []string{"--inline", "--host", "hostA", "--local", "--host", "hostB"},
			want: []string{"hostA", "hostB"},
		},
		{
			name: "尾端 host 缺少值",
			args: []string{"--host", "hostA", "--host"},
			want: []string{"hostA"},
		},
		{
			name: "空 args",
			args: []string{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHostFlags(tt.args)
			if len(got) != len(tt.want) {
				t.Errorf("parseHostFlags(%v) = %v, want %v", tt.args, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseHostFlags(%v)[%d] = %q, want %q", tt.args, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseLocalFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "無 local", args: []string{"--host", "a"}, want: false},
		{name: "有 local", args: []string{"--local"}, want: true},
		{name: "local 與 host 混合", args: []string{"--local", "--host", "a"}, want: true},
		{name: "空 args", args: []string{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLocalFlag(tt.args)
			if got != tt.want {
				t.Errorf("parseLocalFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestHasHostMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "裸 --host", args: []string{"--host"}, want: true},
		{name: "有值 --host", args: []string{"--host", "a"}, want: true},
		{name: "無 --host", args: []string{"--inline"}, want: false},
		{name: "--local 也算 host 模式", args: []string{"--local"}, want: true},
		{name: "空 args", args: []string{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasHostMode(tt.args)
			if got != tt.want {
				t.Errorf("hasHostMode(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
```

### Step 2: 跑測試確認失敗

Run: `go test ./cmd/tsm/... -v -race -run "TestParseHostFlags|TestParseLocalFlag|TestHasHostMode"`
Expected: 編譯失敗 — 函式不存在

### Step 3: 實作解析函式

在 `cmd/tsm/main.go` 中：

```go
// parseHostFlags 從命令列參數中收集所有 --host <value> 的主機名稱。
func parseHostFlags(args []string) []string {
	var hosts []string
	for i, a := range args {
		if a == "--host" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			hosts = append(hosts, args[i+1])
		}
	}
	return hosts
}

// parseLocalFlag 回傳命令列中是否包含 --local 旗標。
func parseLocalFlag(args []string) bool {
	return containsFlag(args, "--local")
}

// hasHostMode 回傳命令列中是否指定了多主機模式（--host 或 --local）。
func hasHostMode(args []string) bool {
	return containsFlag(args, "--host") || containsFlag(args, "--local")
}
```

同時刪除 `parseRemoteHosts`。

### Step 4: 跑測試確認通過

Run: `go test ./cmd/tsm/... -v -race`
Expected: 全部 PASS（舊的 TestParseRemoteHosts 已被取代）

### Step 5: Commit

```bash
git add cmd/tsm/main.go cmd/tsm/parse_test.go
git commit -m "feat: CLI 解析 --host/--local 取代 --remote"
```

---

## Task 3: FlattenMultiHost 標題顯示規則

### Files:
- Modify: `internal/ui/items.go`
- Modify: `internal/ui/items_test.go`

### Step 1: 寫失敗測試

在 `internal/ui/items_test.go` 末尾加入：

```go
func TestFlattenMultiHostLocalOnlyHidesTitle(t *testing.T) {
	// 只有 local enabled → 不顯示 host title
	snaps := []ui.HostSnapshotInput{
		{
			HostID:   "local",
			Name:     "local",
			Color:    "#5f8787",
			Status:   2, // connected
			Sessions: []tmux.Session{{Name: "dev", SortOrder: 0}, {Name: "web", SortOrder: 1}},
		},
	}

	items := ui.FlattenMultiHost(snaps)

	// 應只有 2 個 session item，無 host title
	require.Len(t, items, 2)
	assert.Equal(t, ui.ItemSession, items[0].Type)
	assert.Equal(t, "dev", items[0].Session.Name)
	assert.Equal(t, ui.ItemSession, items[1].Type)
	assert.Equal(t, "web", items[1].Session.Name)
}

func TestFlattenMultiHostLocalOnlyWithDisabledOthers(t *testing.T) {
	// local enabled + 其他 disabled → 只有 local，隱藏標題
	snaps := []ui.HostSnapshotInput{
		{
			HostID:   "local",
			Name:     "local",
			Color:    "#5f8787",
			Status:   2,
			Sessions: []tmux.Session{{Name: "dev", SortOrder: 0}},
		},
		{
			HostID: "remote-a",
			Name:   "remote-a",
			Color:  "#ff0000",
			Status: 0, // disabled
		},
	}

	items := ui.FlattenMultiHost(snaps)

	require.Len(t, items, 1)
	assert.Equal(t, ui.ItemSession, items[0].Type)
	assert.Equal(t, "dev", items[0].Session.Name)
}

func TestFlattenMultiHostSingleRemoteShowsTitle(t *testing.T) {
	// 只有一台 remote enabled（local disabled）→ 顯示 title
	snaps := []ui.HostSnapshotInput{
		{
			HostID: "local",
			Name:   "local",
			Status: 0, // disabled
		},
		{
			HostID:   "remote-a",
			Name:     "remote-a",
			Color:    "#ff0000",
			Status:   2,
			Sessions: []tmux.Session{{Name: "web", SortOrder: 0}},
		},
	}

	items := ui.FlattenMultiHost(snaps)

	require.Len(t, items, 2)
	assert.Equal(t, ui.ItemHostTitle, items[0].Type)
	assert.Equal(t, "remote-a", items[0].HostID)
	assert.Equal(t, ui.ItemSession, items[1].Type)
}
```

### Step 2: 跑測試確認失敗

Run: `go test ./internal/ui/... -v -race -run "TestFlattenMultiHostLocalOnly"`
Expected: FAIL — 目前 FlattenMultiHost 始終產生 host title

### Step 3: 修改 FlattenMultiHost

修改 `internal/ui/items.go` 的 `FlattenMultiHost`：

```go
// FlattenMultiHost 將多台主機的快照扁平化為一維列表。
// 每台主機先放一個 ItemHostTitle，若已連線（Status==2）再展開其 sessions/groups。
// 例外：若 enabled 的主機只有 local 一台，則隱藏 host title。
func FlattenMultiHost(snaps []HostSnapshotInput) []ListItem {
	// 判斷是否要隱藏標題：只有 local 一台 enabled
	hideTitle := isLocalOnly(snaps)

	var items []ListItem

	for _, snap := range snaps {
		if snap.Status == HostStateDisabled {
			continue
		}

		if !hideTitle {
			items = append(items, ListItem{
				Type:      ItemHostTitle,
				HostID:    snap.HostID,
				HostColor: snap.Color,
				HostState: snap.Status,
				HostError: snap.Error,
			})
		}

		if snap.Status == HostStateConnected && len(snap.Sessions) > 0 {
			sub := FlattenItems(snap.Groups, snap.Sessions)
			for i := range sub {
				sub[i].HostID = snap.HostID
				sub[i].HostColor = snap.Color
			}
			items = append(items, sub...)
		}
	}

	return items
}

// isLocalOnly 判斷 enabled 的主機是否只有 local 一台。
func isLocalOnly(snaps []HostSnapshotInput) bool {
	enabledCount := 0
	hasLocalEnabled := false
	for _, s := range snaps {
		if s.Status != HostStateDisabled {
			enabledCount++
			if s.HostID == "local" {
				hasLocalEnabled = true
			}
		}
	}
	return enabledCount == 1 && hasLocalEnabled
}
```

### Step 4: 跑測試確認通過

Run: `go test ./internal/ui/... -v -race`
Expected: 全部 PASS（新舊測試皆通過）

### Step 5: Commit

```bash
git add internal/ui/items.go internal/ui/items_test.go
git commit -m "feat: FlattenMultiHost 只有 local enabled 時隱藏標題"
```

---

## Task 4: 統一啟動路徑 — 合併 runTUI/runMultiHost/runTUIWithClient

### Files:
- Modify: `cmd/tsm/main.go`

這是最大的改動。合併三個函式為統一流程。

### Step 1: 跑既有測試確認基線

Run: `make test`
Expected: 全部 PASS

### Step 2: 改寫 main() 的路由邏輯

修改 `cmd/tsm/main.go` 的 `main()` 函式：

```go
func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		runWithMode(modeAuto)
		return
	}

	if args[0] == "--version" || args[0] == "-v" {
		fmt.Printf("tsm %s\n", version.String())
		return
	}

	if args[0] == "--help" || args[0] == "-h" {
		printUsage()
		return
	}

	// --host / --local / --inline / --popup 都可能混合出現
	if hasHostMode(args) || args[0] == "--inline" || args[0] == "--popup" {
		runWithMode(parseRunMode(args))
		return
	}

	// 子命令保持不變
	// ... bind, hooks, status-name, daemon, config, iterm-coprocess, setup, upgrade ...
}
```

### Step 3: 改寫 runWithMode

```go
func runWithMode(mode runMode) {
	dataDir := config.ExpandPath(config.Default().DataDir)
	if config.LoadInstallMode(dataDir) == config.ModeClient {
		runClientLauncher(dataDir)
		return
	}

	if mode == modeAuto && os.Getenv("TMUX") != "" {
		mode = modePopup
	}

	if mode == modePopup {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		// 將 CLI 旗標轉發給 popup 子程序（除了 --popup 本身）
		popupArgs := []string{exe, "--inline"}
		for _, a := range os.Args[1:] {
			if a != "--popup" {
				popupArgs = append(popupArgs, a)
			}
		}
		cmd := osexec.Command("tmux", append([]string{"display-popup", "-E", "-w", "80%", "-h", "80%"}, popupArgs...)...)
		cmd.Env = append(os.Environ(), "TSM_IN_POPUP=1")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(0)
		}
		return
	}

	runTUI()
}
```

### Step 4: 改寫 runTUI — 統一 HostManager 入口

```go
func runTUI() {
	cfg := loadConfig()
	cfg.InTmux = os.Getenv("TMUX") != ""
	cfg.InPopup = os.Getenv("TSM_IN_POPUP") == "1"

	ensureLocalStatusBar(cfg)

	args := os.Args[1:]
	hostFlags := parseHostFlags(args)
	localFlag := parseLocalFlag(args)
	hostMode := hasHostMode(args)

	// 決定 host 清單
	if hostMode && (len(hostFlags) > 0 || localFlag) {
		// 有明確旗標 → merge + 存 config
		merged := config.MergeHosts(cfg.Hosts, hostFlags, localFlag)
		cfg.Hosts = merged
		cfgPath := config.ExpandPath("~/.config/tsm/config.toml")
		_ = config.SaveConfig(cfgPath, cfg)
	} else if !hostMode {
		// tsm 無參數 → 強制只啟用 local，不存 config
		for i := range cfg.Hosts {
			if cfg.Hosts[i].IsLocal() {
				cfg.Hosts[i].Enabled = true
			} else {
				cfg.Hosts[i].Enabled = false
			}
		}
	}
	// else: tsm --host 裸用 → 直接使用 config 中的 enabled 狀態

	// 建立 HostManager
	mgr := hostmgr.New()
	for _, h := range cfg.Hosts {
		mgr.AddHost(h)
	}
	for _, h := range mgr.Hosts() {
		if h.IsLocal() {
			h.SetGlobalConfig(cfg)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartAll(ctx)
	defer mgr.Close()

	cfgPath := config.ExpandPath("~/.config/tsm/config.toml")
	exec := tmux.NewRealExecutor()
	upgrader := upgrade.DefaultUpgrader()

	for {
		deps := ui.Deps{
			HostMgr:    mgr,
			Cfg:        cfg,
			ConfigPath: cfgPath,
			Upgrader:   upgrader,
		}
		m := ui.NewModel(deps)
		p := tea.NewProgram(m, tea.WithAltScreen())

		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fm, ok := finalModel.(ui.Model)
		if !ok {
			return
		}

		if fm.UpgradeReady() {
			cancel()
			runPostUpgrade(fm, cfg)
			return
		}
		if fm.OpenConfig() {
			runConfig()
			cfg = loadConfig()
			cfg.InTmux = os.Getenv("TMUX") != ""
			cfg.InPopup = os.Getenv("TSM_IN_POPUP") == "1"
			continue
		}

		selected := fm.Selected()
		if selected == "" {
			// 無選取 → 可能是 q/esc 退出
			if fm.ExitTmux() && os.Getenv("TMUX") != "" {
				_ = remote.WriteExitMarker()
				_ = osexec.Command("tmux", "detach-client").Run()
			}
			return
		}

		// 判斷選取的 session 屬於哪台主機
		item := fm.SelectedItem()
		host := mgr.Host(item.HostID)

		if host == nil || host.IsLocal() {
			// 本機 session
			switchToSession(selected, fm.ReadOnly())
			if cfg.InPopup {
				return
			}
			continue
		}

		// 遠端 session — attach 邏輯（與原 runMultiHost 相同）
		hostCfg := host.Config()
		applyHostBar := func() {
			if hostCfg.Color != "" {
				barCfg := config.ColorConfig{
					BarBG:   hostCfg.Color,
					BadgeBG: hostCfg.Color,
					BadgeFG: cfg.Remote.BadgeFG,
				}
				_ = tmux.ApplyStatusBar(exec, barCfg)
			} else {
				_ = tmux.ApplyStatusBar(exec, cfg.Remote)
			}
		}

		applyHostBar()
		result := remote.Attach(hostCfg.Address, selected)
		_ = tmux.ApplyStatusBar(exec, cfg.Local)

		if result == remote.AttachDetached {
			if remote.CheckAndClearExitRequested(hostCfg.Address) {
				return
			}
			continue
		}

		// 斷線重連
		for {
			if !doMultiHostReconnect(mgr, item.HostID, selected) {
				break
			}
			applyHostBar()
			result = remote.Attach(hostCfg.Address, selected)
			_ = tmux.ApplyStatusBar(exec, cfg.Local)
			if result == remote.AttachDetached {
				if remote.CheckAndClearExitRequested(hostCfg.Address) {
					return
				}
				break
			}
		}
	}
}
```

### Step 5: 刪除廢棄函式

刪除：
- `runMultiHost()`
- `runTUIWithClient()`
- `runTUILegacy()`
- `parseRemoteHosts()`

### Step 6: 更新 printUsage

```go
func printUsage() {
	fmt.Fprintf(os.Stderr, "tsm %s\n", version.String())
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: tsm [command] [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  (no args)          啟動 TUI 選單（單機模式，自動管理 daemon）")
	fmt.Fprintln(os.Stderr, "  config             設定 tsm 參數與主題色")
	fmt.Fprintln(os.Stderr, "  setup              互動式安裝所有元件")
	fmt.Fprintln(os.Stderr, "  bind install       安裝 Ctrl+Q 快捷鍵到 ~/.tmux.conf")
	fmt.Fprintln(os.Stderr, "  bind uninstall     移除 Ctrl+Q 快捷鍵")
	fmt.Fprintln(os.Stderr, "  daemon start       啟動 daemon（背景執行）")
	fmt.Fprintln(os.Stderr, "  daemon stop        停止 daemon")
	fmt.Fprintln(os.Stderr, "  daemon restart     重新啟動 daemon")
	fmt.Fprintln(os.Stderr, "  daemon status      顯示 daemon 狀態")
	fmt.Fprintln(os.Stderr, "  hooks install      安裝 tsm hooks 到 Claude Code settings")
	fmt.Fprintln(os.Stderr, "  hooks uninstall    移除 tsm hooks")
	fmt.Fprintln(os.Stderr, "  status-name        輸出當前 session 自訂名稱（供 tmux status bar 使用）")
	fmt.Fprintln(os.Stderr, "  upgrade [--force]  檢查並升級到最新版本")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --version, -v      顯示版本號")
	fmt.Fprintln(os.Stderr, "  --inline           強制使用內嵌全螢幕模式")
	fmt.Fprintln(os.Stderr, "  --popup            強制使用 tmux popup 模式")
	fmt.Fprintln(os.Stderr, "  --host             啟動多主機模式（讀取設定中已啟用的主機）")
	fmt.Fprintln(os.Stderr, "  --host <name>      啟動指定主機（可多次使用，覆寫設定）")
	fmt.Fprintln(os.Stderr, "  --local            僅啟動本地端（可搭配 --host 使用，覆寫設定）")
}
```

### Step 7: 跑全域測試

Run: `make test`
Expected: 全部 PASS

### Step 8: Commit

```bash
git add cmd/tsm/main.go
git commit -m "feat: 統一啟動路徑，所有模式走 HostManager，--host 取代 --remote"
```

---

## Task 5: 移除 UI 中的單機模式路徑（SnapshotMsg/watchCmd）

### Files:
- Modify: `internal/ui/app.go`

### Step 1: 確認基線

Run: `make test`
Expected: 全部 PASS

### Step 2: 清理 Init() 中的 gRPC 單機分支

由於 `Deps.Client` 不再單獨使用（改由 HostManager 管理），移除 `Init()` 中的 `if m.deps.Client != nil` 分支。

保留 `SnapshotMsg`、`watchCmd` 等型別/函式以備 legacy 測試使用，但 `Init()` 入口精簡為：

```go
func (m Model) Init() tea.Cmd {
	if m.deps.HostMgr != nil {
		mgr := m.deps.HostMgr
		return tea.Batch(
			func() tea.Msg { return buildMultiHostMsg(mgr) },
			recvMultiHostCmd(mgr),
		)
	}
	// 舊模式：直接輪詢（保留給測試用）
	return tea.Batch(
		loadSessionsCmd(m.deps),
		tickCmd(m.pollInterval()),
	)
}
```

### Step 3: 確認 [U] 上傳按鈕在多主機模式可用

目前 `[U]` 只在 `m.deps.Client != nil` 時顯示（`app.go` line 1857-1859）。多主機模式下，需要改為透過 `clientForCursor()` 取得 client。

toolbar 的 `[U]` 條件改為：

```go
if m.deps.HostMgr != nil || m.deps.Client != nil {
	line2Parts = append(line2Parts, render("[U]", "上傳"))
}
```

### Step 4: 跑全域測試

Run: `make test`
Expected: 全部 PASS

### Step 5: Commit

```bash
git add internal/ui/app.go
git commit -m "refactor: UI Init() 移除 gRPC 單機分支，統一走 HostManager"
```

---

## Task 6: 版本號推進 + 最終驗證

### Files:
- Modify: `VERSION`

### Step 1: 推進版本號

這是新功能（MINOR），從 `0.26.2` → `0.27.0`：

```
0.27.0
```

### Step 2: 全域測試

Run: `make test`
Expected: 全部 PASS

### Step 3: 編譯測試

Run: `make build && ./bin/tsm --version`
Expected: `tsm 0.27.0(...)`

### Step 4: 手動驗證矩陣

| 指令 | 預期 |
|---|---|
| `tsm` | 單機模式，無 host title，有 `[h]` |
| `tsm --host` | 讀 config，顯示所有 enabled host |
| `tsm --host <name>` | 啟用指定 host，存 config |
| `tsm --local` | 僅 local，存 config |
| `tsm --local --host <name>` | local + 指定 host，存 config |

### Step 5: Commit + tag

```bash
git add VERSION
git commit -m "chore: bump version to 0.27.0"
```
