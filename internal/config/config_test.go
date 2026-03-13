package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wake/tmux-session-menu/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.Default()

	assert.Equal(t, "~/.config/tsm", cfg.DataDir)
	assert.Equal(t, 150, cfg.PreviewLines)
	assert.Equal(t, 2, cfg.PollIntervalSec)
}

func TestLoadFromTOML(t *testing.T) {
	tomlData := `
data_dir = "/tmp/tsm-test"
preview_lines = 50
poll_interval_sec = 5
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	assert.Equal(t, "/tmp/tsm-test", cfg.DataDir)
	assert.Equal(t, 50, cfg.PreviewLines)
	assert.Equal(t, 5, cfg.PollIntervalSec)
}

func TestLoadFromTOML_PartialOverride(t *testing.T) {
	tomlData := `preview_lines = 80`

	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	assert.Equal(t, "~/.config/tsm", cfg.DataDir)
	assert.Equal(t, 80, cfg.PreviewLines)
	assert.Equal(t, 2, cfg.PollIntervalSec)
}

func TestLoadFromTOML_InvalidInput(t *testing.T) {
	_, err := config.LoadFromString("invalid {{{")
	assert.Error(t, err)
}

// --- InstallMode 測試 ---

func TestInstallMode_Constants(t *testing.T) {
	assert.Equal(t, config.InstallMode("full"), config.ModeFull)
	assert.Equal(t, config.InstallMode("client"), config.ModeClient)
}

func TestSaveAndLoadInstallMode(t *testing.T) {
	dir := t.TempDir()

	err := config.SaveInstallMode(dir, config.ModeFull)
	require.NoError(t, err)

	mode := config.LoadInstallMode(dir)
	assert.Equal(t, config.ModeFull, mode)
}

func TestSaveAndLoadInstallMode_Client(t *testing.T) {
	dir := t.TempDir()

	err := config.SaveInstallMode(dir, config.ModeClient)
	require.NoError(t, err)

	mode := config.LoadInstallMode(dir)
	assert.Equal(t, config.ModeClient, mode)
}

func TestLoadInstallMode_NoFile_DefaultsFull(t *testing.T) {
	dir := t.TempDir()
	mode := config.LoadInstallMode(dir)
	assert.Equal(t, config.ModeFull, mode)
}

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

// --- ColorConfig 測試 ---

func TestDefaultConfig_HasColorSections(t *testing.T) {
	cfg := config.Default()

	// Local 預設色值
	assert.Equal(t, "", cfg.Local.BarBG, "local bar_bg 預設為空（保持 tmux 預設）")
	assert.Equal(t, "", cfg.Local.BarFG, "local bar_fg 預設為空（保持 tmux 預設）")
	assert.Equal(t, "#5f8787", cfg.Local.BadgeBG, "local badge_bg 預設值")
	assert.Equal(t, "#c0caf5", cfg.Local.BadgeFG, "local badge_fg 預設值")

	// Remote 不設預設值（全為空字串）
	assert.Equal(t, "", cfg.Remote.BarBG, "remote bar_bg 預設為空（不設預設）")
	assert.Equal(t, "", cfg.Remote.BadgeBG, "remote badge_bg 預設為空（不設預設）")
	assert.Equal(t, "", cfg.Remote.BadgeFG, "remote badge_fg 預設為空（不設預設）")
}

func TestLoadFromTOML_WithColorSections(t *testing.T) {
	tomlData := `
data_dir = "/tmp/tsm-test"

[local]
bar_bg = "#111111"
badge_bg = "#222222"
badge_fg = "#333333"

[remote]
bar_bg = "#444444"
badge_bg = "#555555"
badge_fg = "#666666"
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	assert.Equal(t, "/tmp/tsm-test", cfg.DataDir)

	assert.Equal(t, "#111111", cfg.Local.BarBG)
	assert.Equal(t, "#222222", cfg.Local.BadgeBG)
	assert.Equal(t, "#333333", cfg.Local.BadgeFG)

	// LoadFromString 會自動執行 MigrateRemoteToHosts，[remote] 段落會被清空
	assert.Equal(t, "", cfg.Remote.BarBG, "自動遷移後 [remote] 應清空")
	assert.Equal(t, "", cfg.Remote.BadgeBG, "自動遷移後 [remote] 應清空")
	assert.Equal(t, "", cfg.Remote.BadgeFG, "自動遷移後 [remote] 應清空")
}

func TestLoadFromTOML_PartialColors_KeepsDefaults(t *testing.T) {
	tomlData := `
[local]
badge_bg = "#aabbcc"

[remote]
bar_bg = "#ddeeff"
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	// Local: 只覆蓋 badge_bg，其餘保持預設
	assert.Equal(t, "", cfg.Local.BarBG, "未設定的 local bar_bg 保持預設空值")
	assert.Equal(t, "#aabbcc", cfg.Local.BadgeBG, "已設定的 local badge_bg 為覆蓋值")
	assert.Equal(t, "#c0caf5", cfg.Local.BadgeFG, "未設定的 local badge_fg 保持預設")

	// LoadFromString 自動執行 MigrateRemoteToHosts，[remote] 段落遷移後清空
	assert.Equal(t, "", cfg.Remote.BarBG, "自動遷移後 [remote] bar_bg 應清空")
	assert.Equal(t, "", cfg.Remote.BadgeBG, "自動遷移後 [remote] badge_bg 應清空")
	assert.Equal(t, "", cfg.Remote.BadgeFG, "自動遷移後 [remote] badge_fg 應清空")
}

// --- SaveConfig 測試 ---

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
	assert.Equal(t, "#5f8787", loaded.Local.BadgeBG)
}

func TestSaveConfig_RuntimeFieldsNotSaved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := config.Default()
	cfg.InTmux = true
	cfg.InPopup = true

	err := config.SaveConfig(path, cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.NotContains(t, content, "in_tmux")
	assert.NotContains(t, content, "in_popup")
}

// --- HostEntry 測試 ---

func TestDefaultConfig_HasLocalHost(t *testing.T) {
	cfg := config.Default()

	require.Len(t, cfg.Hosts, 1, "預設應包含一筆 local host")

	h := cfg.Hosts[0]
	assert.Equal(t, "local", h.Name)
	assert.Equal(t, "", h.Address, "local host address 應為空字串")
	assert.Equal(t, "#5f8787", h.Color)
	assert.True(t, h.Enabled)
	assert.Equal(t, 0, h.SortOrder)
}

func TestHostEntry_IsLocal(t *testing.T) {
	local := config.HostEntry{Name: "local", Address: ""}
	assert.True(t, local.IsLocal(), "address 為空時 IsLocal() 應為 true")

	remote := config.HostEntry{Name: "server", Address: "192.168.1.100"}
	assert.False(t, remote.IsLocal(), "address 非空時 IsLocal() 應為 false")
}

func TestLoadFromTOML_MultipleHosts(t *testing.T) {
	tomlData := `
data_dir = "/tmp/tsm-test"

[[hosts]]
name = "local"
address = ""
color = "#5f8787"
enabled = true
sort_order = 0

[[hosts]]
name = "server-a"
address = "10.0.0.1"
color = "#73daca"
enabled = true
sort_order = 1

[[hosts]]
name = "server-b"
address = "10.0.0.2"
color = "#ff9e64"
enabled = false
sort_order = 2
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	require.Len(t, cfg.Hosts, 3)

	assert.Equal(t, "local", cfg.Hosts[0].Name)
	assert.True(t, cfg.Hosts[0].IsLocal())

	assert.Equal(t, "server-a", cfg.Hosts[1].Name)
	assert.Equal(t, "10.0.0.1", cfg.Hosts[1].Address)
	assert.Equal(t, "#73daca", cfg.Hosts[1].Color)
	assert.True(t, cfg.Hosts[1].Enabled)
	assert.Equal(t, 1, cfg.Hosts[1].SortOrder)
	assert.False(t, cfg.Hosts[1].IsLocal())

	assert.Equal(t, "server-b", cfg.Hosts[2].Name)
	assert.False(t, cfg.Hosts[2].Enabled)
}

func TestHostEntry_TOML_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		{Name: "remote-1", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 1},
	}

	err := config.SaveConfig(path, cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	loaded, err := config.LoadFromString(string(data))
	require.NoError(t, err)

	require.Len(t, loaded.Hosts, 2)
	assert.Equal(t, cfg.Hosts[0].Name, loaded.Hosts[0].Name)
	assert.Equal(t, cfg.Hosts[0].Address, loaded.Hosts[0].Address)
	assert.Equal(t, cfg.Hosts[0].Color, loaded.Hosts[0].Color)
	assert.Equal(t, cfg.Hosts[0].Enabled, loaded.Hosts[0].Enabled)
	assert.Equal(t, cfg.Hosts[0].SortOrder, loaded.Hosts[0].SortOrder)

	assert.Equal(t, cfg.Hosts[1].Name, loaded.Hosts[1].Name)
	assert.Equal(t, cfg.Hosts[1].Address, loaded.Hosts[1].Address)
	assert.Equal(t, cfg.Hosts[1].Color, loaded.Hosts[1].Color)
	assert.Equal(t, cfg.Hosts[1].Enabled, loaded.Hosts[1].Enabled)
	assert.Equal(t, cfg.Hosts[1].SortOrder, loaded.Hosts[1].SortOrder)
}

// --- ColorConfig bar_fg 測試 ---

func TestColorConfig_ParseBarFG(t *testing.T) {
	tomlData := `
[local]
bar_bg = "#111111"
bar_fg = "#ffffff"
badge_bg = "#222222"
badge_fg = "#333333"
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	assert.Equal(t, "#111111", cfg.Local.BarBG, "local bar_bg 應正確解析")
	assert.Equal(t, "#ffffff", cfg.Local.BarFG, "local bar_fg 應正確解析")
	assert.Equal(t, "#222222", cfg.Local.BadgeBG)
	assert.Equal(t, "#333333", cfg.Local.BadgeFG)
}

func TestDefaultConfig_LocalBarFGEmpty(t *testing.T) {
	cfg := config.Default()

	assert.Equal(t, "", cfg.Local.BarFG, "local bar_fg 預設應為空字串")
}

// --- HostEntry 新欄位測試 ---

func TestHostEntry_ParseNewColorFields(t *testing.T) {
	tomlData := `
[[hosts]]
name = "server-a"
address = "10.0.0.1"
color = "#73daca"
enabled = true
sort_order = 1
bar_bg = "#1a2b2b"
bar_fg = "#e0e0e0"
badge_bg = "#73daca"
badge_fg = "#1a1b26"
archived = false

[[hosts]]
name = "server-b"
address = "10.0.0.2"
color = "#ff9e64"
enabled = false
sort_order = 2
archived = true
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	require.Len(t, cfg.Hosts, 2)

	h := cfg.Hosts[0]
	assert.Equal(t, "server-a", h.Name)
	assert.Equal(t, "#1a2b2b", h.BarBG, "host bar_bg 應正確解析")
	assert.Equal(t, "#e0e0e0", h.BarFG, "host bar_fg 應正確解析")
	assert.Equal(t, "#73daca", h.BadgeBG, "host badge_bg 應正確解析")
	assert.Equal(t, "#1a1b26", h.BadgeFG, "host badge_fg 應正確解析")
	assert.False(t, h.Archived, "archived=false 應正確解析")

	h2 := cfg.Hosts[1]
	assert.Equal(t, "server-b", h2.Name)
	assert.True(t, h2.Archived, "archived=true 應正確解析")
	assert.Equal(t, "", h2.BarBG, "未設定的 bar_bg 應為空字串")
	assert.Equal(t, "", h2.BarFG, "未設定的 bar_fg 應為空字串")
	// LoadFromString 自動遷移：badge_bg 為空但 Color 有值時 fallback 到 host.Color
	assert.Equal(t, "#ff9e64", h2.BadgeBG, "badge_bg 為空時自動 fallback 到 host.Color")
	assert.Equal(t, "", h2.BadgeFG, "未設定的 badge_fg 應為空字串")
}

func TestDefaultConfig_NoRemotePreset(t *testing.T) {
	cfg := config.Default()

	// Remote 不應有預設值（全為零值）
	assert.Equal(t, "", cfg.Remote.BarBG, "Default() 不應預設 remote bar_bg")
	assert.Equal(t, "", cfg.Remote.BarFG, "Default() 不應預設 remote bar_fg")
	assert.Equal(t, "", cfg.Remote.BadgeBG, "Default() 不應預設 remote badge_bg")
	assert.Equal(t, "", cfg.Remote.BadgeFG, "Default() 不應預設 remote badge_fg")
}

func TestHostEntry_ArchivedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := config.Default()
	cfg.Hosts = []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		{
			Name: "server", Address: "10.0.0.1", Color: "#73daca", Enabled: false, SortOrder: 1,
			BarBG: "#1a2b2b", BarFG: "#e0e0e0", BadgeBG: "#73daca", BadgeFG: "#1a1b26",
			Archived: true,
		},
	}

	err := config.SaveConfig(path, cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	loaded, err := config.LoadFromString(string(data))
	require.NoError(t, err)

	require.Len(t, loaded.Hosts, 2)
	s := loaded.Hosts[1]
	assert.Equal(t, "#1a2b2b", s.BarBG, "bar_bg 應 round-trip 正確")
	assert.Equal(t, "#e0e0e0", s.BarFG, "bar_fg 應 round-trip 正確")
	assert.Equal(t, "#73daca", s.BadgeBG, "badge_bg 應 round-trip 正確")
	assert.Equal(t, "#1a1b26", s.BadgeFG, "badge_fg 應 round-trip 正確")
	assert.True(t, s.Archived, "archived 應 round-trip 正確")
}

// --- HostEntry.ToColorConfig 測試 ---

func TestHostEntry_ToColorConfig(t *testing.T) {
	h := config.HostEntry{
		Color:   "#accent",
		BarBG:   "#1a2b2b",
		BarFG:   "#e0e0e0",
		BadgeBG: "#73daca",
		BadgeFG: "#1a1b26",
	}
	cc := h.ToColorConfig()
	assert.Equal(t, "#1a2b2b", cc.BarBG, "BarBG 應與 host bar_bg 一致")
	assert.Equal(t, "#e0e0e0", cc.BarFG, "BarFG 應與 host bar_fg 一致")
	assert.Equal(t, "#73daca", cc.BadgeBG, "BadgeBG 有值時不應 fallback")
	assert.Equal(t, "#1a1b26", cc.BadgeFG, "BadgeFG 應與 host badge_fg 一致")
}

func TestHostEntry_ToColorConfig_BadgeFallback(t *testing.T) {
	h := config.HostEntry{
		Color:   "#5f8787",
		BarBG:   "#111111",
		BarFG:   "#ffffff",
		BadgeBG: "", // 空值 → fallback 到 Color
		BadgeFG: "#cccccc",
	}
	cc := h.ToColorConfig()
	assert.Equal(t, "#5f8787", cc.BadgeBG, "BadgeBG 為空時應 fallback 到 Color")
	assert.Equal(t, "#111111", cc.BarBG, "BarBG 應保持不變")
	assert.Equal(t, "#ffffff", cc.BarFG, "BarFG 應保持不變")
	assert.Equal(t, "#cccccc", cc.BadgeFG, "BadgeFG 應保持不變")
}

func TestHostEntry_ToColorConfig_AllEmpty(t *testing.T) {
	h := config.HostEntry{}
	cc := h.ToColorConfig()
	assert.Equal(t, "", cc.BarBG, "全空時 BarBG 應為空")
	assert.Equal(t, "", cc.BarFG, "全空時 BarFG 應為空")
	assert.Equal(t, "", cc.BadgeBG, "全空時 BadgeBG 應為空（Color 亦空，不 fallback）")
	assert.Equal(t, "", cc.BadgeFG, "全空時 BadgeFG 應為空")
}

// --- LoadFromString 自動遷移整合測試 ---

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
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	// mlab1 應繼承 [remote] 的顏色（四色全空 + [remote] 有值 → 繼承）
	require.Len(t, cfg.Hosts, 2)
	assert.Equal(t, "#1a2b2b", cfg.Hosts[1].BarBG)
	assert.Equal(t, "#73daca", cfg.Hosts[1].BadgeBG) // [remote].badge_bg 繼承（非 color fallback）
	assert.Equal(t, "#1a1b26", cfg.Hosts[1].BadgeFG)

	// [remote] 應清空
	assert.Empty(t, cfg.Remote.BarBG)
	assert.Empty(t, cfg.Remote.BadgeBG)
}

// --- AgentEntry 測試 ---

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

func TestSaveConfig_WithAgents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := config.Default()
	cfg.Agents = []config.AgentEntry{
		{Name: "Claude", Command: "claude", Group: "Coding", Enabled: true, SortOrder: 0},
	}

	err := config.SaveConfig(path, cfg)
	assert.NoError(t, err)

	data, _ := os.ReadFile(path)
	loaded, err := config.LoadFromString(string(data))
	assert.NoError(t, err)
	assert.Len(t, loaded.Agents, 1)
	assert.Equal(t, "Claude", loaded.Agents[0].Name)
}

// --- UploadConfig 測試 ---

func TestDefaultUploadConfig(t *testing.T) {
	cfg := config.Default()
	if cfg.Upload.RemotePath != "/tmp/iterm-upload" {
		t.Errorf("got %q, want /tmp/iterm-upload", cfg.Upload.RemotePath)
	}
}

func TestLoadUploadConfig(t *testing.T) {
	data := `
[upload]
remote_path = "/data/uploads"
`
	cfg, err := config.LoadFromString(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Upload.RemotePath != "/data/uploads" {
		t.Errorf("got %q, want /data/uploads", cfg.Upload.RemotePath)
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/.config/tsm", home + "/.config/tsm"},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := config.ExpandPath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
