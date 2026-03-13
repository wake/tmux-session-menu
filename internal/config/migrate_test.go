package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMigrateConfig_RemoteToHostColors 驗證 [remote] 的顏色值能遷移到沒有設定顏色的遠端主機。
func TestMigrateConfig_RemoteToHostColors(t *testing.T) {
	cfg := &Config{
		Remote: &ColorConfig{
			BarBG:   "#444444",
			BarFG:   "#eeeeee",
			BadgeBG: "#555555",
			BadgeFG: "#666666",
		},
		Local: ColorConfig{
			BarBG:   "#111111",
			BarFG:   "#ffffff",
			BadgeBG: "#222222",
			BadgeFG: "#333333",
		},
		Hosts: []HostEntry{
			{Name: "local", Address: "", Color: "#5f8787", Enabled: true},
			{Name: "server-a", Address: "10.0.0.1", Color: "#73daca", Enabled: true},
		},
	}

	MigrateRemoteToHosts(cfg)

	// 遠端主機應繼承 [remote] 的顏色
	serverA := cfg.Hosts[1]
	assert.Equal(t, "#444444", serverA.BarBG, "遠端主機 bar_bg 應繼承自 [remote]")
	assert.Equal(t, "#eeeeee", serverA.BarFG, "遠端主機 bar_fg 應繼承自 [remote]")
	assert.Equal(t, "#555555", serverA.BadgeBG, "遠端主機 badge_bg 應繼承自 [remote]")
	assert.Equal(t, "#666666", serverA.BadgeFG, "遠端主機 badge_fg 應繼承自 [remote]")

	// [remote] 應清空（nil，SaveConfig 不再寫出）
	assert.Nil(t, cfg.Remote, "遷移後 [remote] 應為 nil")
}

// TestMigrateConfig_NoRemote_Noop 驗證已設定顏色的主機不會被 [remote] 覆蓋。
func TestMigrateConfig_NoRemote_Noop(t *testing.T) {
	cfg := &Config{
		Remote: &ColorConfig{
			BarBG:   "#444444",
			BarFG:   "#eeeeee",
			BadgeBG: "#555555",
			BadgeFG: "#666666",
		},
		Local: ColorConfig{
			BadgeBG: "#222222",
			BadgeFG: "#333333",
		},
		Hosts: []HostEntry{
			{Name: "local", Address: "", Color: "#5f8787", Enabled: true},
			{
				Name:    "server-a",
				Address: "10.0.0.1",
				Color:   "#73daca",
				Enabled: true,
				// 已有設定，不應被覆蓋
				BarBG:   "#1a2b2b",
				BarFG:   "#e0e0e0",
				BadgeBG: "#73daca",
				BadgeFG: "#1a1b26",
			},
		},
	}

	MigrateRemoteToHosts(cfg)

	// 已有顏色的主機不應被覆蓋
	serverA := cfg.Hosts[1]
	assert.Equal(t, "#1a2b2b", serverA.BarBG, "已有 bar_bg 不應被覆蓋")
	assert.Equal(t, "#e0e0e0", serverA.BarFG, "已有 bar_fg 不應被覆蓋")
	assert.Equal(t, "#73daca", serverA.BadgeBG, "已有 badge_bg 不應被覆蓋")
	assert.Equal(t, "#1a1b26", serverA.BadgeFG, "已有 badge_fg 不應被覆蓋")
}

// TestMigrateConfig_LocalSync 驗證 local 主機從 [local] 同步顏色。
func TestMigrateConfig_LocalSync(t *testing.T) {
	cfg := &Config{
		Local: ColorConfig{
			BarBG:   "#111111",
			BarFG:   "#ffffff",
			BadgeBG: "#222222",
			BadgeFG: "#333333",
		},
		Hosts: []HostEntry{
			{Name: "local", Address: "", Color: "#5f8787", Enabled: true},
			{Name: "server-a", Address: "10.0.0.1", Enabled: true},
		},
	}

	MigrateRemoteToHosts(cfg)

	// local 主機應從 [local] 同步顏色
	local := cfg.Hosts[0]
	assert.Equal(t, "#111111", local.BarBG, "local 主機 bar_bg 應同步自 [local]")
	assert.Equal(t, "#ffffff", local.BarFG, "local 主機 bar_fg 應同步自 [local]")
	assert.Equal(t, "#222222", local.BadgeBG, "local 主機 badge_bg 應同步自 [local]")
	assert.Equal(t, "#333333", local.BadgeFG, "local 主機 badge_fg 應同步自 [local]")
}

// TestMigrateConfig_BadgeBGFallbackToColor 驗證 badge_bg 為空時用 host.Color 作為 fallback。
func TestMigrateConfig_BadgeBGFallbackToColor(t *testing.T) {
	cfg := &Config{
		// Remote 只有 BadgeFG，沒有 BadgeBG
		Remote: &ColorConfig{
			BadgeFG: "#ffffff",
		},
		Hosts: []HostEntry{
			{
				Name:    "server-a",
				Address: "10.0.0.1",
				Color:   "#73daca",
				Enabled: true,
				// 四色均為空
			},
		},
	}

	MigrateRemoteToHosts(cfg)

	serverA := cfg.Hosts[0]
	// BadgeBG 應 fallback 到 host.Color
	assert.Equal(t, "#73daca", serverA.BadgeBG, "badge_bg 為空時應 fallback 到 host.Color")
}

// TestMigrateConfig_NoRemoteValues_NoChange 驗證 [remote] 完全為空時不做任何遷移。
func TestMigrateConfig_NoRemoteValues_NoChange(t *testing.T) {
	cfg := &Config{
		Remote: &ColorConfig{}, // 完全為空
		Local: ColorConfig{
			BadgeBG: "#222222",
			BadgeFG: "#333333",
		},
		Hosts: []HostEntry{
			{Name: "local", Address: "", Color: "#5f8787", Enabled: true},
			{Name: "server-a", Address: "10.0.0.1", Color: "#73daca", Enabled: true},
		},
	}

	MigrateRemoteToHosts(cfg)

	// [remote] 為空，遠端主機四色仍應為空（除了 fallback）
	serverA := cfg.Hosts[1]
	assert.Equal(t, "", serverA.BarBG, "[remote] 無值時不遷移 bar_bg")
	assert.Equal(t, "", serverA.BarFG, "[remote] 無值時不遷移 bar_fg")
	// badge_bg 仍會 fallback 到 Color
	assert.Equal(t, "#73daca", serverA.BadgeBG, "badge_bg 仍應 fallback 到 host.Color")
}

// TestSyncLocalHostToConfig 驗證 local 主機的顏色能寫回 Config.Local。
func TestSyncLocalHostToConfig(t *testing.T) {
	cfg := &Config{
		Local: ColorConfig{
			BarBG:   "#old-bar",
			BadgeBG: "#old-badge",
		},
		Hosts: []HostEntry{
			{
				Name:    "local",
				Address: "",
				BarBG:   "#new-bar",
				BarFG:   "#new-bar-fg",
				BadgeBG: "#new-badge",
				BadgeFG: "#new-badge-fg",
			},
			{Name: "server-a", Address: "10.0.0.1", BadgeBG: "#should-not-affect"},
		},
	}

	SyncLocalHostToConfig(cfg)

	// local 主機的顏色應寫回 Config.Local
	assert.Equal(t, "#new-bar", cfg.Local.BarBG, "Local.BarBG 應同步自 local 主機")
	assert.Equal(t, "#new-bar-fg", cfg.Local.BarFG, "Local.BarFG 應同步自 local 主機")
	assert.Equal(t, "#new-badge", cfg.Local.BadgeBG, "Local.BadgeBG 應同步自 local 主機")
	assert.Equal(t, "#new-badge-fg", cfg.Local.BadgeFG, "Local.BadgeFG 應同步自 local 主機")
}

// TestSyncLocalHostToConfig_NoLocalHost 驗證沒有 local 主機時不做任何變更。
func TestSyncLocalHostToConfig_NoLocalHost(t *testing.T) {
	cfg := &Config{
		Local: ColorConfig{
			BarBG:   "#original",
			BadgeBG: "#original-badge",
		},
		Hosts: []HostEntry{
			{Name: "server-a", Address: "10.0.0.1", BadgeBG: "#aabbcc"},
		},
	}

	SyncLocalHostToConfig(cfg)

	// 沒有 local 主機時 Config.Local 不應改變
	assert.Equal(t, "#original", cfg.Local.BarBG, "無 local 主機時 Config.Local 不應改變")
	assert.Equal(t, "#original-badge", cfg.Local.BadgeBG, "無 local 主機時 Config.Local 不應改變")
}
