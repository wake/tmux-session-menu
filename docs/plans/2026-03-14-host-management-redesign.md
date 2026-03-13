# Host 管理功能重新設計 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 擴充主機管理功能：三態狀態（啟用/停用/封存）、per-host 四色設定（右側面板 UI＋預覽）、移除 `[remote]` 區段改為 host-level 顏色。

**Architecture:** 擴充 `HostEntry` 加入四色欄位 + `archived` 旗標，`ColorConfig` 加入 `bar_fg`。移除 `Config.Remote`，遷移邏輯在 `LoadFromString` 中完成。Host picker 新增右側面板 sub-mode，顏色套用改讀 host 自身四色。

**Tech Stack:** Go 1.26, Bubble Tea (TUI), TOML config, tmux set-option

**Spec:** `docs/specs/2026-03-14-host-management-redesign.md`

---

## Chunk 1: Config 層擴充與遷移

### Task 1: HostEntry 擴充 + ColorConfig 加 bar_fg

**Files:**
- Modify: `internal/config/config.go:11-16` (ColorConfig)
- Modify: `internal/config/config.go:18-25` (HostEntry)
- Modify: `internal/config/config.go:62-85` (Default)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 寫測試 — HostEntry 新增四色 + archived**

```go
// internal/config/config_test.go
func TestHostEntry_NewFields(t *testing.T) {
	tomlData := `
[[hosts]]
  name = "mlab1"
  address = "mlab1"
  color = "#e0af68"
  bar_bg = "#1a2b2b"
  bar_fg = "#c0caf5"
  badge_bg = "#e0af68"
  badge_fg = "#1a1b26"
  enabled = true
  archived = false
`
	cfg, err := LoadFromString(tomlData)
	require.NoError(t, err)
	require.Len(t, cfg.Hosts, 1)

	h := cfg.Hosts[0]
	assert.Equal(t, "#1a2b2b", h.BarBG)
	assert.Equal(t, "#c0caf5", h.BarFG)
	assert.Equal(t, "#e0af68", h.BadgeBG)
	assert.Equal(t, "#1a1b26", h.BadgeFG)
	assert.False(t, h.Archived)
}

func TestColorConfig_BarFG(t *testing.T) {
	tomlData := `
[local]
  bar_bg = "#cccccc"
  bar_fg = "#ffffff"
  badge_bg = "#5f8787"
  badge_fg = "#c0caf5"
`
	cfg, err := LoadFromString(tomlData)
	require.NoError(t, err)
	assert.Equal(t, "#ffffff", cfg.Local.BarFG)
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/config/... -v -race -run "TestHostEntry_NewFields|TestColorConfig_BarFG"`
Expected: FAIL — `BarFG`, `BadgeBG` 等欄位不存在

- [ ] **Step 3: 實作 — 擴充 struct**

`internal/config/config.go` — ColorConfig (line 11):
```go
type ColorConfig struct {
	BarBG   string `toml:"bar_bg"`   // 狀態列背景色（空字串=保持 tmux 預設）
	BarFG   string `toml:"bar_fg"`   // 狀態列前景色（空字串=tmux 預設）
	BadgeBG string `toml:"badge_bg"` // 徽章背景色
	BadgeFG string `toml:"badge_fg"` // 徽章前景色
}
```

`internal/config/config.go` — HostEntry (line 18):
```go
type HostEntry struct {
	Name      string `toml:"name"`
	Address   string `toml:"address"`  // 空字串 = 本機
	Color     string `toml:"color"`    // accent color（UI 圓點、host title）
	BarBG     string `toml:"bar_bg"`   // status bar 背景色
	BarFG     string `toml:"bar_fg"`   // status bar 前景色（空=tmux 預設）
	BadgeBG   string `toml:"badge_bg"` // badge 背景色（空=fallback to color）
	BadgeFG   string `toml:"badge_fg"` // badge 前景色
	Enabled   bool   `toml:"enabled"`
	Archived  bool   `toml:"archived"` // 軟刪除
	SortOrder int    `toml:"sort_order"`
}
```

`internal/config/config.go` — Default() (line 62) — 移除 Remote 預設值，Local 加 BarFG:
```go
func Default() Config {
	return Config{
		DataDir:         "~/.config/tsm",
		PreviewLines:    150,
		PollIntervalSec: 2,
		Local: ColorConfig{
			BarBG:   "",
			BarFG:   "",
			BadgeBG: "#5f8787",
			BadgeFG: "#c0caf5",
		},
		Hosts: []HostEntry{
			{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		},
		Upload: UploadConfig{
			RemotePath: "/tmp/iterm-upload",
		},
	}
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/config/... -v -race -run "TestHostEntry_NewFields|TestColorConfig_BarFG"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: HostEntry 擴充四色 + archived、ColorConfig 加 bar_fg"
```

---

### Task 2: 遷移邏輯 — [remote] → per-host 四色

**Files:**
- Modify: `internal/config/config.go:46-60` (Config struct — 保留 Remote 欄位供讀取)
- Create: `internal/config/migrate.go` (遷移函式)
- Test: `internal/config/migrate_test.go`

- [ ] **Step 1: 寫測試 — 舊格式遷移**

```go
// internal/config/migrate_test.go
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateConfig_RemoteToHostColors(t *testing.T) {
	tomlData := `
[local]
  bar_bg = ""
  badge_bg = "#5f8787"
  badge_fg = "#c0caf5"

[remote]
  bar_bg = "#1a2b2b"
  badge_bg = "#73daca"
  badge_fg = "#1a1b26"

[[hosts]]
  name = "local"
  address = ""
  color = "#5f8787"
  enabled = true

[[hosts]]
  name = "mlab1"
  address = "mlab1"
  color = "#e0af68"
  enabled = true
`
	cfg, err := LoadFromString(tomlData)
	require.NoError(t, err)
	MigrateRemoteToHosts(&cfg)

	// mlab1 應從 [remote] 繼承四色
	mlab := cfg.Hosts[1]
	assert.Equal(t, "#1a2b2b", mlab.BarBG)
	assert.Equal(t, "#e0af68", mlab.BadgeBG) // color fallback
	assert.Equal(t, "#1a1b26", mlab.BadgeFG)

	// local 應從 [local] 同步
	local := cfg.Hosts[0]
	assert.Equal(t, "", local.BarBG)
	assert.Equal(t, "#5f8787", local.BadgeBG)
	assert.Equal(t, "#c0caf5", local.BadgeFG)
}

func TestMigrateConfig_NoRemote_Noop(t *testing.T) {
	tomlData := `
[local]
  badge_bg = "#5f8787"
  badge_fg = "#c0caf5"

[[hosts]]
  name = "mlab1"
  address = "mlab1"
  bar_bg = "#custom"
  badge_bg = "#custom2"
  badge_fg = "#custom3"
  enabled = true
`
	cfg, err := LoadFromString(tomlData)
	require.NoError(t, err)
	MigrateRemoteToHosts(&cfg)

	// 已有四色的 host 不應被覆蓋
	assert.Equal(t, "#custom", cfg.Hosts[0].BarBG)
}

func TestMigrateConfig_LocalSync(t *testing.T) {
	tomlData := `
[local]
  bar_bg = "#aabbcc"
  bar_fg = "#112233"
  badge_bg = "#5f8787"
  badge_fg = "#c0caf5"

[[hosts]]
  name = "local"
  address = ""
  color = "#5f8787"
  enabled = true
`
	cfg, err := LoadFromString(tomlData)
	require.NoError(t, err)
	MigrateRemoteToHosts(&cfg)

	local := cfg.Hosts[0]
	assert.Equal(t, "#aabbcc", local.BarBG)
	assert.Equal(t, "#112233", local.BarFG)
	assert.Equal(t, "#5f8787", local.BadgeBG)
	assert.Equal(t, "#c0caf5", local.BadgeFG)
}

func TestSyncLocalHostToConfig(t *testing.T) {
	cfg := Config{
		Hosts: []HostEntry{
			{Name: "local", Address: "", BarBG: "#aaa", BarFG: "#bbb", BadgeBG: "#ccc", BadgeFG: "#ddd"},
		},
	}
	SyncLocalHostToConfig(&cfg)
	assert.Equal(t, "#aaa", cfg.Local.BarBG)
	assert.Equal(t, "#bbb", cfg.Local.BarFG)
	assert.Equal(t, "#ccc", cfg.Local.BadgeBG)
	assert.Equal(t, "#ddd", cfg.Local.BadgeFG)
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/config/... -v -race -run "TestMigrateConfig|TestSyncLocal"`
Expected: FAIL — `MigrateRemoteToHosts` 不存在

- [ ] **Step 3: 實作遷移函式**

```go
// internal/config/migrate.go
package config

// MigrateRemoteToHosts 將舊 [remote] 區段的顏色遷移到 per-host 設定。
// 1. 若 Remote 有值且遠端 host 四色皆空 → 從 Remote 填入
// 2. badge_bg 為空但 color 有值 → fallback 到 color
// 3. [local] 值同步到 local host entry
// 4. 清除 Config.Remote
func MigrateRemoteToHosts(cfg *Config) {
	hasRemote := cfg.Remote.BarBG != "" || cfg.Remote.BadgeBG != "" || cfg.Remote.BadgeFG != ""

	for i := range cfg.Hosts {
		h := &cfg.Hosts[i]

		if h.IsLocal() {
			// local host 從 [local] 同步
			h.BarBG = cfg.Local.BarBG
			h.BarFG = cfg.Local.BarFG
			h.BadgeBG = cfg.Local.BadgeBG
			h.BadgeFG = cfg.Local.BadgeFG
			continue
		}

		// 遠端 host：若四色皆空且有 [remote]，從 remote 繼承
		if hasRemote && h.BarBG == "" && h.BadgeBG == "" && h.BadgeFG == "" {
			h.BarBG = cfg.Remote.BarBG
			h.BarFG = cfg.Remote.BarFG
			h.BadgeFG = cfg.Remote.BadgeFG
		}

		// badge_bg fallback 到 color
		if h.BadgeBG == "" && h.Color != "" {
			h.BadgeBG = h.Color
		}
	}

	// 清除 Remote
	cfg.Remote = ColorConfig{}
}

// SyncLocalHostToConfig 將 local host entry 的四色回寫到 Config.Local。
// 用於 host picker 儲存後同步。
func SyncLocalHostToConfig(cfg *Config) {
	for _, h := range cfg.Hosts {
		if h.IsLocal() {
			cfg.Local.BarBG = h.BarBG
			cfg.Local.BarFG = h.BarFG
			cfg.Local.BadgeBG = h.BadgeBG
			cfg.Local.BadgeFG = h.BadgeFG
			return
		}
	}
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/config/... -v -race -run "TestMigrateConfig|TestSyncLocal"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/migrate.go internal/config/migrate_test.go
git commit -m "feat: [remote] → per-host 四色遷移邏輯"
```

---

### Task 3: MergeHosts 支援 archived

**Files:**
- Modify: `internal/config/hosts.go:28-45`
- Test: `internal/config/hosts_test.go`

- [ ] **Step 1: 寫測試 — archived 主機不被 --host 啟用、新增同名解封存**

```go
// 在 internal/config/hosts_test.go 新增

func TestMergeHosts_ArchivedNotActivatedByFlag(t *testing.T) {
	hosts := []HostEntry{
		{Name: "local", Address: "", Enabled: true},
		{Name: "mlab1", Address: "mlab1", Enabled: false, Archived: true},
	}
	result := MergeHosts(hosts, []string{"mlab1"}, false)
	// archived 主機不應被 --host 旗標啟用
	for _, h := range result {
		if h.Name == "mlab1" {
			assert.True(t, h.Archived, "archived host should stay archived")
			assert.False(t, h.Enabled)
		}
	}
}

func TestUnarchiveHost(t *testing.T) {
	hosts := []HostEntry{
		{Name: "local", Address: "", Enabled: true},
		{Name: "mlab1", Address: "mlab1", Enabled: false, Archived: true, BarBG: "#old"},
	}
	found, idx := FindArchivedHost(hosts, "mlab1")
	assert.True(t, found)
	assert.Equal(t, 1, idx)

	hosts[idx].Archived = false
	hosts[idx].Enabled = true
	assert.True(t, hosts[idx].Enabled)
	assert.False(t, hosts[idx].Archived)
	assert.Equal(t, "#old", hosts[idx].BarBG) // 保留舊設定
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/config/... -v -race -run "TestMergeHosts_Archived|TestUnarchiveHost"`
Expected: FAIL

- [ ] **Step 3: 實作**

在 `internal/config/hosts.go` 中加入：

```go
// FindArchivedHost 搜尋已封存的同名主機，回傳是否找到及索引。
func FindArchivedHost(hosts []HostEntry, name string) (bool, int) {
	for i, h := range hosts {
		if h.Name == name && h.Archived {
			return true, i
		}
	}
	return false, -1
}
```

修改 `mergeWithFlags` 函式（在 hosts.go 中），在啟用主機的邏輯中跳過 archived 主機：

在判斷 `enabled` 前加入：
```go
if h.Archived {
	continue
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/config/... -v -race`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/hosts.go internal/config/hosts_test.go
git commit -m "feat: MergeHosts 支援 archived、FindArchivedHost 解封存查詢"
```

---

## Chunk 2: StatusBar + Config TUI

### Task 4: StatusBarArgs 支援 bar_fg

**Files:**
- Modify: `internal/tmux/statusbar.go:9-40`
- Test: `internal/tmux/statusbar_test.go`

- [ ] **Step 1: 寫測試 — bar_fg 有值/無值**

```go
// 在 internal/tmux/statusbar_test.go 新增

func TestStatusBarArgs_WithBarFG(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "#1a2b2b",
		BarFG:   "#c0caf5",
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}
	cmds := tmux.StatusBarArgs(colors)

	found := false
	for _, cmd := range cmds {
		if len(cmd) >= 4 && cmd[2] == "status-style" {
			assert.Contains(t, cmd[3], "bg=#1a2b2b")
			assert.Contains(t, cmd[3], "fg=#c0caf5")
			found = true
		}
	}
	assert.True(t, found)
}

func TestStatusBarArgs_EmptyBarFG(t *testing.T) {
	colors := config.ColorConfig{
		BarBG:   "#1a2b2b",
		BarFG:   "",
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}
	cmds := tmux.StatusBarArgs(colors)

	for _, cmd := range cmds {
		if len(cmd) >= 4 && cmd[2] == "status-style" {
			assert.Contains(t, cmd[3], "bg=#1a2b2b")
			assert.NotContains(t, cmd[3], "fg=")
		}
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/tmux/... -v -race -run "TestStatusBarArgs_WithBarFG|TestStatusBarArgs_EmptyBarFG"`
Expected: FAIL — `WithBarFG` 測試失敗（目前不會輸出 `fg=`）

- [ ] **Step 3: 實作 — 修改 StatusBarArgs**

`internal/tmux/statusbar.go` — StatusBarArgs 函式，替換 bar_bg 區段 (lines 15-25):

```go
if colors.BarBG != "" {
	style := fmt.Sprintf("bg=%s", colors.BarBG)
	if colors.BarFG != "" {
		style += fmt.Sprintf(",fg=%s", colors.BarFG)
	}
	cmds = append(cmds, []string{
		"set-option", "-g", "status-style", style,
	})
} else if colors.BarFG != "" {
	// 只有 fg，沒有 bg
	cmds = append(cmds, []string{
		"set-option", "-g", "status-style",
		fmt.Sprintf("fg=%s", colors.BarFG),
	})
} else {
	cmds = append(cmds, []string{
		"set-option", "-gu", "status-style",
	})
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/tmux/... -v -race`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tmux/statusbar.go internal/tmux/statusbar_test.go
git commit -m "feat: StatusBarArgs 支援 bar_fg"
```

---

### Task 5: HostEntry 的 ColorConfig 轉換輔助

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 寫測試**

```go
func TestHostEntry_ToColorConfig(t *testing.T) {
	h := HostEntry{
		Color:   "#e0af68",
		BarBG:   "#1a2b2b",
		BarFG:   "#c0caf5",
		BadgeBG: "#e0af68",
		BadgeFG: "#1a1b26",
	}
	cc := h.ToColorConfig()
	assert.Equal(t, "#1a2b2b", cc.BarBG)
	assert.Equal(t, "#c0caf5", cc.BarFG)
	assert.Equal(t, "#e0af68", cc.BadgeBG)
	assert.Equal(t, "#1a1b26", cc.BadgeFG)
}

func TestHostEntry_ToColorConfig_BadgeFallback(t *testing.T) {
	h := HostEntry{
		Color:   "#e0af68",
		BarBG:   "#1a2b2b",
		BadgeBG: "", // 空 → fallback to color
		BadgeFG: "#1a1b26",
	}
	cc := h.ToColorConfig()
	assert.Equal(t, "#e0af68", cc.BadgeBG) // fallback
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/config/... -v -race -run TestHostEntry_ToColorConfig`
Expected: FAIL

- [ ] **Step 3: 實作**

`internal/config/config.go` — 在 HostEntry 方法區加入：

```go
// ToColorConfig 將 HostEntry 的四色轉為 ColorConfig。
// BadgeBG 為空時 fallback 到 Color。
func (h HostEntry) ToColorConfig() ColorConfig {
	badgeBG := h.BadgeBG
	if badgeBG == "" && h.Color != "" {
		badgeBG = h.Color
	}
	return ColorConfig{
		BarBG:   h.BarBG,
		BarFG:   h.BarFG,
		BadgeBG: badgeBG,
		BadgeFG: h.BadgeFG,
	}
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/config/... -v -race -run TestHostEntry_ToColorConfig`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: HostEntry.ToColorConfig 輔助方法"
```

---

### Task 6: Config TUI — 移除 remote、加 bar_fg

**Files:**
- Modify: `internal/cfgtui/cfgtui.go:50-99`
- Test: `internal/cfgtui/cfgtui_test.go`

- [ ] **Step 1: 寫測試**

```go
// 在 internal/cfgtui/cfgtui_test.go 新增

func TestClientFields_NoRemote(t *testing.T) {
	cfg := config.Default()
	m := NewModel(cfg)
	fields := m.currentFields() // 預設為 TabClient

	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = f.key
	}

	assert.Contains(t, keys, "local.bar_bg")
	assert.Contains(t, keys, "local.bar_fg")
	assert.Contains(t, keys, "local.badge_bg")
	assert.Contains(t, keys, "local.badge_fg")
	assert.NotContains(t, keys, "remote.bar_bg")
	assert.NotContains(t, keys, "remote.badge_bg")
	assert.NotContains(t, keys, "remote.badge_fg")
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/cfgtui/... -v -race -run TestClientFields_NoRemote`
Expected: FAIL — 仍包含 remote 欄位、缺少 bar_fg

- [ ] **Step 3: 實作 — 更新欄位定義**

`internal/cfgtui/cfgtui.go` — clientFields():
```go
func clientFields(cfg config.Config) []field {
	return []field{
		{key: "local.bar_bg", label: "本地 bar 背景色", value: cfg.Local.BarBG},
		{key: "local.bar_fg", label: "本地 bar 前景色", value: cfg.Local.BarFG},
		{key: "local.badge_bg", label: "本地 badge 背景色", value: cfg.Local.BadgeBG},
		{key: "local.badge_fg", label: "本地 badge 前景色", value: cfg.Local.BadgeFG},
	}
}
```

serverFields():
```go
func serverFields(cfg config.Config) []field {
	return []field{
		{key: "preview_lines", label: "預覽行數", value: strconv.Itoa(cfg.PreviewLines)},
		{key: "poll_interval_sec", label: "輪詢間隔", value: strconv.Itoa(cfg.PollIntervalSec)},
		{key: "local.bar_bg", label: "本地 bar 背景色", value: cfg.Local.BarBG},
		{key: "local.bar_fg", label: "本地 bar 前景色", value: cfg.Local.BarFG},
		{key: "local.badge_bg", label: "本地 badge 背景色", value: cfg.Local.BadgeBG},
		{key: "local.badge_fg", label: "本地 badge 前景色", value: cfg.Local.BadgeFG},
	}
}
```

defaultFieldValue() — 移除 remote 分支，加入 bar_fg：
```go
func defaultFieldValue(key string) string {
	def := Default()
	switch key {
	case "preview_lines":
		return strconv.Itoa(def.PreviewLines)
	case "poll_interval_sec":
		return strconv.Itoa(def.PollIntervalSec)
	case "local.bar_bg":
		return def.Local.BarBG
	case "local.bar_fg":
		return def.Local.BarFG
	case "local.badge_bg":
		return def.Local.BadgeBG
	case "local.badge_fg":
		return def.Local.BadgeFG
	default:
		return ""
	}
}
```

ResultConfig() — 移除 remote 欄位回寫，加入 bar_fg：
```go
if v, ok := values["local.bar_fg"]; ok {
	cfg.Local.BarFG = v
}
```
並移除所有 `remote.*` 的回寫區段。

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/cfgtui/... -v -race`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cfgtui/cfgtui.go internal/cfgtui/cfgtui_test.go
git commit -m "feat: config TUI 移除 remote 欄位、加 local.bar_fg"
```

---

## Chunk 3: Host Picker UI 改造

### Task 7: 三態操作 — [n] 新增、[d] 封存、space 啟停

**Files:**
- Modify: `internal/ui/hostpicker.go:14-106`
- Modify: `internal/ui/app.go:1253-1280` (InputNewHost)
- Test: `internal/ui/hostpicker_test.go`

- [ ] **Step 1: 寫測試 — 封存操作**

```go
// 在 internal/ui/hostpicker_test.go 或 app_test.go 新增

func TestHostPicker_ArchiveHost(t *testing.T) {
	cfg := config.Default()
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: cfg})
	// 進入 host picker
	m, _ = applyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 移到 mlab1（index 1）
	m, _ = applyKey(m, "down")
	// 按 d 封存
	m, _ = applyKey(m, "d")

	// 確認 mlab1 被封存
	hosts := mgr.Hosts()
	for _, h := range hosts {
		if h.Config().Name == "mlab1" {
			assert.True(t, h.Config().Archived)
			assert.False(t, h.Config().Enabled)
		}
	}
}

func TestHostPicker_CannotArchiveLocal(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = applyKey(m, "h")
	// cursor 在 local（index 0），按 d
	m, _ = applyKey(m, "d")

	// local 不應被封存
	hosts := mgr.Hosts()
	for _, h := range hosts {
		if h.Config().Name == "local" {
			assert.False(t, h.Config().Archived)
		}
	}
}

func TestHostPicker_NewKeyIsN(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = applyKey(m, "h")
	m, _ = applyKey(m, "n")
	assert.Equal(t, ui.ModeInput, m.Mode())
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/ui/... -v -race -run "TestHostPicker_Archive|TestHostPicker_NewKeyIsN"`
Expected: FAIL

- [ ] **Step 3: 實作 — 修改 hostpicker.go 按鍵處理**

`internal/ui/hostpicker.go` — updateHostPicker:

將 `case "a"` (line 78) 改為 `case "n"`。

將 `case "d"` (line 86-103) 改為封存邏輯（不再 Remove，改為設定 archived + disabled）：

```go
case "d":
	// 封存主機（軟刪除）— local 不可封存
	if m.hostPickerCursor < len(hosts) {
		h := hosts[m.hostPickerCursor]
		if !h.IsLocal() {
			_ = m.deps.HostMgr.Disable(h.ID())
			m.deps.HostMgr.SetArchived(h.ID(), true)
			m.persistHosts()
		}
	}
```

注意：需在 `hostmgr.HostManager` 加入 `SetArchived(id string, archived bool)` 方法。

`internal/ui/app.go` — InputNewHost handler (line 1253):

在新增前先查是否有同名封存主機：
```go
case InputNewHost:
	if m.deps.HostMgr != nil {
		// 檢查是否有同名封存主機
		if found, host := m.deps.HostMgr.FindArchived(value); found {
			host.SetArchived(false)
			_ = m.deps.HostMgr.Enable(context.Background(), host.ID())
			m.persistHosts()
			m.mode = ModeHostPicker
			return m, nil
		}

		// 既有新增邏輯...
		hosts := m.deps.HostMgr.Hosts()
		// ... (保持不變)
	}
```

- [ ] **Step 4: 實作 — hostmgr 加入 SetArchived 和 FindArchived**

`internal/hostmgr/manager.go`:

```go
// SetArchived 設定主機的封存狀態。
func (m *HostManager) SetArchived(hostID string, archived bool) {
	h := m.Host(hostID)
	if h == nil {
		return
	}
	h.mu.Lock()
	h.cfg.Archived = archived
	if archived {
		h.cfg.Enabled = false
	}
	h.mu.Unlock()
}

// FindArchived 搜尋已封存的同名主機。
func (m *HostManager) FindArchived(name string) (bool, *Host) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.hosts {
		cfg := h.Config()
		if cfg.Name == name && cfg.Archived {
			return true, h
		}
	}
	return false, nil
}
```

`internal/hostmgr/host.go` 加入：

```go
func (h *Host) SetArchived(archived bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg.Archived = archived
	if archived {
		h.cfg.Enabled = false
	}
}
```

- [ ] **Step 5: 執行測試確認通過**

Run: `go test ./internal/ui/... -v -race -run "TestHostPicker_Archive|TestHostPicker_NewKeyIsN"`
Expected: PASS

- [ ] **Step 6: 全測試通過**

Run: `go test ./... -race`
Expected: ALL PASS（可能需要修正其他測試中對 `case "a"` 的引用）

- [ ] **Step 7: Commit**

```bash
git add internal/ui/hostpicker.go internal/ui/app.go internal/hostmgr/manager.go internal/hostmgr/host.go internal/ui/hostpicker_test.go
git commit -m "feat: host picker 三態操作 — [n] 新增、[d] 封存、解封存"
```

---

### Task 8: 管理畫面 — 封存主機過濾 + 渲染更新

**Files:**
- Modify: `internal/ui/hostpicker.go:132-192` (renderHostPicker)
- Test: `internal/ui/hostpicker_test.go`

- [ ] **Step 1: 寫測試 — 封存主機不顯示**

```go
func TestHostPicker_ArchivedHostHidden(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: false, Archived: true})
	mgr.AddHost(config.HostEntry{Name: "mlab2", Address: "mlab2", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = applyKey(m, "h")
	view := m.View()

	assert.Contains(t, view, "local")
	assert.NotContains(t, view, "mlab1")
	assert.Contains(t, view, "mlab2")
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/ui/... -v -race -run TestHostPicker_ArchivedHostHidden`
Expected: FAIL — mlab1 仍顯示

- [ ] **Step 3: 實作 — renderHostPicker 過濾 archived**

在 `renderHostPicker` 的主機迴圈中加入過濾：

```go
// 過濾：跳過 archived 主機
if cfg.Archived {
    continue
}
```

同時更新 `updateHostPicker` 中取得 hosts 的邏輯，使 cursor 計算排除 archived 主機。建立輔助函式：

```go
// visibleHosts 回傳非封存的主機列表。
func (m Model) visibleHosts() []*hostmgr.Host {
	if m.deps.HostMgr == nil {
		return nil
	}
	var result []*hostmgr.Host
	for _, h := range m.deps.HostMgr.Hosts() {
		if !h.Config().Archived {
			result = append(result, h)
		}
	}
	return result
}
```

替換 `updateHostPicker` 和 `renderHostPicker` 中所有 `m.deps.HostMgr.Hosts()` 為 `m.visibleHosts()`。

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/ui/... -v -race -run TestHostPicker_ArchivedHostHidden`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/hostpicker.go internal/ui/hostpicker_test.go
git commit -m "feat: host picker 過濾封存主機"
```

---

### Task 9: 右側設定面板 — 基本結構

**Files:**
- Modify: `internal/ui/hostpicker.go`
- Modify: `internal/ui/mode.go` (若需新 sub-mode 常數)
- Test: `internal/ui/hostpicker_test.go`

- [ ] **Step 1: 寫測試 — 面板開關**

```go
func TestHostPicker_OpenPanel(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = applyKey(m, "h")
	// 按 Enter 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := updated.(ui.Model)
	assert.True(t, fm.HostPanelOpen())

	// 按 Esc 關閉面板
	updated2, _ := fm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	fm2 := updated2.(ui.Model)
	assert.False(t, fm2.HostPanelOpen())
}

func TestHostPicker_PanelShowsColors(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{
		Name: "mlab1", Address: "mlab1", Enabled: true,
		BarBG: "#1a2b2b", BadgeBG: "#e0af68", BadgeFG: "#1a1b26",
	})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = applyKey(m, "h")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := updated.(ui.Model)
	view := fm.View()

	assert.Contains(t, view, "bar_bg")
	assert.Contains(t, view, "#1a2b2b")
	assert.Contains(t, view, "badge_bg")
	assert.Contains(t, view, "#e0af68")
	assert.Contains(t, view, "留空由 tmux 自行決定")
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/ui/... -v -race -run "TestHostPicker_OpenPanel|TestHostPicker_PanelShowsColors"`
Expected: FAIL

- [ ] **Step 3: 實作 — 面板 state 與 render**

在 `internal/ui/app.go` 的 Model struct 加入面板狀態欄位：

```go
// host picker 右側面板
hostPanelOpen    bool
hostPanelCursor  int    // 0=[x]啟用, 1=bar_bg, 2=bar_fg, 3=badge_bg, 4=badge_fg
hostPanelEditing bool
hostPanelDraft   map[string]config.HostEntry // hostID → 編輯中的 entry（尚未儲存）
```

加入公開方法：
```go
func (m Model) HostPanelOpen() bool { return m.hostPanelOpen }
```

在 `updateHostPicker` 加入：
```go
case "enter", "right":
	if !m.hostPanelOpen {
		m.hostPanelOpen = true
		m.hostPanelCursor = 0
		m.ensureHostDraft() // 建立當前主機的 draft copy
	} else {
		// 面板內：啟用欄位 toggle 或進入編輯
		return m.handlePanelEnter()
	}
```

面板內的按鍵處理：
```go
if m.hostPanelOpen {
	return m.updateHostPanel(msg)
}
```

`updateHostPanel` 處理 ↑/↓ 切換欄位、Enter 編輯/toggle、space toggle 啟用、Esc/← 關閉。

`renderHostPicker` 改為左右分割佈局：
- 左側：既有主機列表
- 右側（面板開啟時）：啟用 toggle + 四色欄位 + 預覽

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/ui/... -v -race -run "TestHostPicker_OpenPanel|TestHostPicker_PanelShowsColors"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/hostpicker.go internal/ui/app.go internal/ui/hostpicker_test.go
git commit -m "feat: host picker 右側設定面板 — 開關、四色顯示"
```

---

### Task 10: 右側面板 — 顏色編輯 + 預覽

**Files:**
- Modify: `internal/ui/hostpicker.go`
- Test: `internal/ui/hostpicker_test.go`

- [ ] **Step 1: 寫測試 — 編輯色碼、預覽渲染**

```go
func TestHostPicker_EditColor(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{
		Name: "mlab1", Address: "mlab1", Enabled: true,
		BarBG: "#1a2b2b",
	})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = applyKey(m, "h")
	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)
	// 移到 bar_bg（cursor=1，0 是啟用 toggle）
	m, _ = applyKey(m, "down")
	// Enter 進入編輯
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)
	assert.True(t, m.HostPanelEditing())
}

func TestHostPicker_PreviewRendered(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{
		Name: "mlab1", Address: "mlab1", Enabled: true,
		BarBG: "#1a2b2b", BadgeBG: "#e0af68", BadgeFG: "#1a1b26",
	})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = applyKey(m, "h")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)
	view := m.View()

	// 預覽區塊應包含主機名稱
	assert.Contains(t, view, "mlab1")
	// 預覽應有某種視覺分隔（框線或區塊）
	assert.Contains(t, view, "預覽")
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/ui/... -v -race -run "TestHostPicker_EditColor|TestHostPicker_PreviewRendered"`
Expected: FAIL

- [ ] **Step 3: 實作 — 編輯與預覽**

面板內編輯模式：使用 `textInput` 元件（已在 Model 中存在）。

Enter 進入編輯 → textInput.SetValue(當前值) → Focus。
Enter/Esc 結束編輯 → 更新 draft entry。

預覽渲染函式：

```go
func renderColorPreview(entry config.HostEntry) string {
	barBG := lipgloss.Color(entry.BarBG)
	barFG := lipgloss.Color(entry.BarFG)
	badgeBG := lipgloss.Color(entry.BadgeBG)
	badgeFG := lipgloss.Color(entry.BadgeFG)

	if entry.BadgeBG == "" && entry.Color != "" {
		badgeBG = lipgloss.Color(entry.Color)
	}

	badgeStyle := lipgloss.NewStyle().
		Background(badgeBG).
		Foreground(badgeFG).
		Bold(true).
		Padding(0, 1)

	barStyle := lipgloss.NewStyle()
	if entry.BarBG != "" {
		barStyle = barStyle.Background(barBG)
	}
	if entry.BarFG != "" {
		barStyle = barStyle.Foreground(barFG)
	}

	badge := badgeStyle.Render(entry.Name)
	bar := barStyle.Render(" 0:zsh*  1:vim ")

	return badge + bar
}
```

色碼驗證（用於預覽更新判斷）：

```go
var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func isValidHexColor(s string) bool {
	return s == "" || hexColorRe.MatchString(s)
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/ui/... -v -race -run "TestHostPicker_EditColor|TestHostPicker_PreviewRendered"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/hostpicker.go internal/ui/hostpicker_test.go
git commit -m "feat: host picker 面板顏色編輯 + status bar 預覽"
```

---

### Task 11: Ctrl+S 儲存全部主機 + 立即套用

**Files:**
- Modify: `internal/ui/hostpicker.go`
- Modify: `internal/ui/app.go` (persistHosts 加同步)
- Test: `internal/ui/hostpicker_test.go`

- [ ] **Step 1: 寫測試**

```go
func TestHostPicker_CtrlSSavesAllHosts(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{
		Name: "local", Enabled: true, BadgeBG: "#old",
	})
	mgr.AddHost(config.HostEntry{
		Name: "mlab1", Address: "mlab1", Enabled: true, BadgeBG: "#old2",
	})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = applyKey(m, "h")

	// 開啟 mlab1 面板，修改 badge_bg
	m, _ = applyKey(m, "down")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)
	// ... (模擬編輯 draft)

	// Ctrl+S
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(ui.Model)

	// 面板應關閉，顯示已儲存提示
	assert.False(t, m.HostPanelOpen())
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/ui/... -v -race -run TestHostPicker_CtrlSSavesAllHosts`
Expected: FAIL

- [ ] **Step 3: 實作**

`updateHostPicker` 加入 Ctrl+S 處理（面板內外都可觸發）：

```go
case "ctrl+s":
	m.applyHostDrafts() // 將所有 draft 寫回 HostMgr
	m.persistHostsWithSync() // persistHosts + SyncLocalHostToConfig
	m.hostPanelOpen = false
	m.hostSavedMsg = "已儲存"
	// 若本機有改色，立即套用
	return m, m.applyCurrentStatusBar()
```

`persistHostsWithSync` 是 `persistHosts` 的擴充版，在寫入前呼叫 `config.SyncLocalHostToConfig`。

`applyCurrentStatusBar` 回傳一個 tea.Cmd：
```go
func (m Model) applyCurrentStatusBar() tea.Cmd {
	return func() tea.Msg {
		// 讀取 local host 的 ColorConfig 並套用
		if m.deps.HostMgr != nil {
			for _, h := range m.deps.HostMgr.Hosts() {
				if h.IsLocal() {
					exec := tmux.NewRealExecutor()
					_ = tmux.ApplyStatusBar(exec, h.Config().ToColorConfig())
					return nil
				}
			}
		}
		return nil
	}
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/ui/... -v -race -run TestHostPicker_CtrlSSavesAllHosts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/hostpicker.go internal/ui/app.go internal/ui/hostpicker_test.go
git commit -m "feat: Ctrl+S 儲存全部主機設定 + 立即套用 status bar"
```

---

## Chunk 4: 整合 — main.go、session 過濾、CLAUDE.md

### Task 12: applyHostBar 改讀 per-host 四色

**Files:**
- Modify: `cmd/tsm/main.go:433-445` (multiHostTUI applyHostBar)
- Modify: `cmd/tsm/main.go:777-787` (ensureLocalStatusBar)
- Modify: `cmd/tsm/main.go:972-984` (runHubTUI applyHostBar)
- Test: 既有測試應繼續通過

- [ ] **Step 1: 修改 multiHostTUI 的 applyHostBar (line 433)**

```go
applyHostBar := func() {
	_ = tmux.ApplyStatusBar(exec, hostCfg.ToColorConfig())
}
```

`hostCfg` 是 `config.HostEntry`（已有 `ToColorConfig` 方法）。

移除 `cfg.Remote` 引用。

- [ ] **Step 2: 修改 runHubTUI 的 applyHostBar (line 972)**

```go
applyHostBar := func() {
	_ = tmux.ApplyStatusBar(exec, hostEntry.ToColorConfig())
}
```

同樣移除 `cfg.Remote` 引用。hub 模式的 hostEntry 需要從 config 中讀取四色。查找邏輯：

```go
// 從 config 中查找 host 的四色設定
func findHostColors(cfg config.Config, hostID string) config.HostEntry {
	for _, h := range cfg.Hosts {
		if h.Name == hostID {
			return h
		}
	}
	return config.HostEntry{}
}
```

- [ ] **Step 3: 修改 ensureLocalStatusBar (line 777)**

```go
func ensureLocalStatusBar(cfg config.Config) {
	if cfg.InTmux {
		exec := tmux.NewRealExecutor()
		_ = tmux.ApplyStatusBar(exec, cfg.Local)
	}
}
```

這裡不需改 — `cfg.Local` 已經與 local host entry 同步。

- [ ] **Step 4: 移除 remote mode 的 cfg.Remote 引用 (lines 526, 554)**

將 remote mode（單主機遠端）的 `cfg.Remote` 改為從 config 查找對應主機：

```go
// 套用 remote status bar 樣式
hostColors := findHostColors(cfg, host)
_ = tmux.ApplyStatusBar(exec, hostColors.ToColorConfig())
```

- [ ] **Step 5: 確認編譯通過**

Run: `go build ./...`
Expected: 成功

- [ ] **Step 6: 執行全測試**

Run: `go test ./... -race`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat: applyHostBar 改讀 per-host 四色、移除 cfg.Remote"
```

---

### Task 13: Session 列表 — enable/archived 過濾

**Files:**
- Modify: `internal/ui/app.go` — session 列表過濾
- Test: `internal/ui/app_test.go`

- [ ] **Step 1: 寫測試 — 停用/封存主機不顯示 sessions**

```go
func TestSessionList_DisabledHostHidden(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: false})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	// 模擬 snapshot 包含 mlab1 的 session
	// 確認渲染時 mlab1 不出現
}
```

- [ ] **Step 2: 實作過濾**

在 `FlattenMultiHost` 或呼叫端，過濾 `enabled=false` 或 `archived=true` 的主機。

Hub 模式：在 `hubHostSnap` 轉換為 UI items 時，交叉比對 client config 的 `enabled`/`archived` 進行過濾。

- [ ] **Step 3: 執行全測試**

Run: `go test ./... -race`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/ui/app.go internal/ui/items.go internal/ui/app_test.go
git commit -m "feat: session 列表過濾停用/封存主機"
```

---

### Task 14: 遷移整合 — LoadFromString 呼叫 MigrateRemoteToHosts

**Files:**
- Modify: `internal/config/config.go:87-94` (LoadFromString)
- Modify: `internal/ui/app.go:50-72` (persistHosts 呼叫 SyncLocalHostToConfig)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 寫整合測試**

```go
func TestLoadFromString_AutoMigrate(t *testing.T) {
	tomlData := `
[local]
  badge_bg = "#5f8787"
  badge_fg = "#c0caf5"

[remote]
  bar_bg = "#1a2b2b"
  badge_bg = "#73daca"
  badge_fg = "#1a1b26"

[[hosts]]
  name = "local"
  address = ""
  color = "#5f8787"
  enabled = true

[[hosts]]
  name = "mlab1"
  address = "mlab1"
  color = "#e0af68"
  enabled = true
`
	cfg, err := LoadFromString(tomlData)
	require.NoError(t, err)

	// 自動遷移後，mlab1 應有四色
	assert.Equal(t, "#1a2b2b", cfg.Hosts[1].BarBG)
	assert.Equal(t, "#e0af68", cfg.Hosts[1].BadgeBG) // color fallback
	// Remote 應被清空
	assert.Empty(t, cfg.Remote.BarBG)
}
```

- [ ] **Step 2: 實作**

`internal/config/config.go` — LoadFromString:
```go
func LoadFromString(data string) (Config, error) {
	cfg := Default()
	if _, err := toml.Decode(data, &cfg); err != nil {
		return Config{}, err
	}
	MigrateRemoteToHosts(&cfg)
	return cfg, nil
}
```

`internal/ui/app.go` — persistHosts 末尾加入：
```go
config.SyncLocalHostToConfig(&fileCfg)
_ = config.SaveConfig(cfgPath, fileCfg)
```

- [ ] **Step 3: 執行全測試**

Run: `go test ./... -race`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go internal/ui/app.go internal/config/config_test.go
git commit -m "feat: LoadFromString 自動遷移 [remote] → per-host 四色"
```

---

### Task 15: Config.Remote 清理 + CLAUDE.md 更新

**Files:**
- Modify: `internal/config/config.go:46-60` — Config struct 保留 Remote 但標記 deprecated
- Modify: `CLAUDE.md`
- Test: 全測試通過

- [ ] **Step 1: Config struct — 標記 Remote 為遷移用途**

```go
type Config struct {
	DataDir         string `toml:"data_dir"`
	PreviewLines    int    `toml:"preview_lines"`
	PollIntervalSec int    `toml:"poll_interval_sec"`
	InTmux          bool   `toml:"-"`
	InPopup         bool   `toml:"-"`

	Local  ColorConfig `toml:"local"`
	Remote ColorConfig `toml:"remote"` // deprecated: 僅供遷移讀取，不再寫出

	Hosts  []HostEntry  `toml:"hosts"`
	Agents []AgentEntry `toml:"agents"`
	Upload UploadConfig `toml:"upload"`
}
```

確保 `SaveConfig` 不會寫出空的 `[remote]`（TOML encoder 預設不寫零值 struct，但需驗證）。若會寫出，需在 SaveConfig 中手動清空。

- [ ] **Step 2: 更新 CLAUDE.md — 補充軟刪除說明**

在 `## 架構概覽` 後加入：

```markdown
### 軟刪除設計

主機管理採用三態設計，不使用硬刪除：

- **啟用** (`enabled=true, archived=false`)：正常連線、session 列表可見
- **停用** (`enabled=false, archived=false`)：不連線、管理畫面可見
- **封存** (`enabled=false, archived=true`)：軟刪除，管理畫面不可見，config 中保留

封存主機可透過新增同名主機自動解封存。local 主機可停用但不可封存。

### Per-Host 顏色設定

每台主機在 `[[hosts]]` 中有獨立的四色設定：`bar_bg`、`bar_fg`、`badge_bg`、`badge_fg`。
`[local]` 區段為 local host 的別名，雙向同步。`[remote]` 已廢棄，首次儲存時自動遷移。
```

- [ ] **Step 3: 執行全測試**

Run: `go test ./... -race`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go CLAUDE.md
git commit -m "docs: CLAUDE.md 補充軟刪除與 per-host 顏色設計"
```

---

### Task 16: 最終驗證 + 版本推進

- [ ] **Step 1: 全測試 + lint**

```bash
make test
make lint
make build
```
Expected: ALL PASS

- [ ] **Step 2: 版本推進**

VERSION: `0.36.1` → `0.37.0`（MINOR — 新功能）

更新 `CHANGELOG.md` 加入 0.37.0 條目。

- [ ] **Step 3: Commit + Tag**

```bash
git add VERSION CHANGELOG.md
git commit -m "chore: bump version to 0.37.0"
git tag v0.37.0
```
