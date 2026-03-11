package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wake/tmux-session-menu/internal/config"
)

// --- MergeHosts 測試 ---

func TestMergeHostsNoRemoteFlag(t *testing.T) {
	// 無旗標（localFlag=false, hostFlags=nil）：不修改，直接回傳副本
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: false, SortOrder: 0},
		{Name: "server-a", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 1},
		{Name: "server-b", Address: "10.0.0.2", Color: "#ff9e64", Enabled: false, SortOrder: 2},
	}

	result := config.MergeHosts(hosts, nil, false)

	require.Len(t, result, 3)

	// 無旗標時不修改任何狀態，直接回傳副本
	assert.Equal(t, "local", result[0].Name)
	assert.False(t, result[0].Enabled, "local 保持原狀（false）")

	assert.Equal(t, "server-a", result[1].Name)
	assert.True(t, result[1].Enabled, "server-a 保持啟用")

	assert.Equal(t, "server-b", result[2].Name)
	assert.False(t, result[2].Enabled, "server-b 保持停用")
}

func TestMergeHostsNoRemoteFlag_NoLocalHost(t *testing.T) {
	// 無旗標（localFlag=false, hostFlags=nil）且 config 中沒有 local host：不修改，不自動補上
	hosts := []config.HostEntry{
		{Name: "server-a", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 1},
	}

	result := config.MergeHosts(hosts, nil, false)

	// 無旗標時不自動補上 local host
	require.Len(t, result, 1)
	assert.Equal(t, "server-a", result[0].Name)
	assert.True(t, result[0].Enabled)
}

func TestMergeHostsWithRemoteFlags(t *testing.T) {
	// --host dev：local 停用，dev 啟用，staging 不在旗標中應停用
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		{Name: "dev", Address: "10.0.0.1", Color: "#73daca", Enabled: false, SortOrder: 1},
		{Name: "staging", Address: "10.0.0.2", Color: "#ff9e64", Enabled: true, SortOrder: 2},
	}

	result := config.MergeHosts(hosts, []string{"dev"}, false)

	require.Len(t, result, 3)

	// local 應被停用
	assert.Equal(t, "local", result[0].Name)
	assert.False(t, result[0].Enabled, "有 --host 時 local 應停用")

	// dev 在旗標中 → 應啟用
	assert.Equal(t, "dev", result[1].Name)
	assert.True(t, result[1].Enabled, "dev 在 --host 旗標中應啟用")

	// staging 不在旗標中應停用
	assert.Equal(t, "staging", result[2].Name)
	assert.False(t, result[2].Enabled, "staging 不在旗標中應停用")
}

func TestMergeHostsNewRemoteHost(t *testing.T) {
	// --host newhost：不在清單中的主機應自動新增
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
	}

	result := config.MergeHosts(hosts, []string{"newhost"}, false)

	require.Len(t, result, 2, "應有 local + newhost")

	// local 停用
	assert.Equal(t, "local", result[0].Name)
	assert.False(t, result[0].Enabled, "有 --host 時 local 應停用")

	// 新增的 newhost
	assert.Equal(t, "newhost", result[1].Name)
	assert.Equal(t, "newhost", result[1].Address, "新主機的 Address 應等於旗標值")
	assert.True(t, result[1].Enabled, "新主機應啟用")
	assert.NotEmpty(t, result[1].Color, "新主機應自動分配顏色")
}

func TestMergeHostsNewRemoteHost_AutoColor(t *testing.T) {
	// 自動分配顏色應從 DefaultColors 色池中選取
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
	}

	result := config.MergeHosts(hosts, []string{"host-a", "host-b"}, false)

	require.Len(t, result, 3)

	// 兩台新主機應分配到不同顏色
	colorA := result[1].Color
	colorB := result[2].Color
	assert.NotEmpty(t, colorA)
	assert.NotEmpty(t, colorB)
	assert.NotEqual(t, colorA, colorB, "不同主機應分配到不同顏色")

	// 顏色應在 DefaultColors 色池中
	assert.Contains(t, config.DefaultColors, colorA, "顏色應來自 DefaultColors")
	assert.Contains(t, config.DefaultColors, colorB, "顏色應來自 DefaultColors")
}

func TestMergeHostsMatchByAddress(t *testing.T) {
	// 透過 Address 比對，而非只靠 Name
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		{Name: "my-dev-server", Address: "dev.example.com", Color: "#73daca", Enabled: false, SortOrder: 1},
	}

	// 用 Address 比對
	result := config.MergeHosts(hosts, []string{"dev.example.com"}, false)

	require.Len(t, result, 2, "不應新增，因為 Address 已匹配")

	// local 停用
	assert.False(t, result[0].Enabled, "有 --host 時 local 應停用")

	// my-dev-server 的 Address 匹配旗標 → 應啟用
	assert.Equal(t, "my-dev-server", result[1].Name)
	assert.True(t, result[1].Enabled, "Address 匹配旗標時應啟用")
}

func TestMergeHostsMatchByName(t *testing.T) {
	// 也可以透過 Name 比對
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		{Name: "dev", Address: "10.0.0.1", Color: "#73daca", Enabled: false, SortOrder: 1},
	}

	result := config.MergeHosts(hosts, []string{"dev"}, false)

	require.Len(t, result, 2)
	assert.True(t, result[1].Enabled, "Name 匹配旗標時應啟用")
}

func TestMergeHostsPreserveOrder(t *testing.T) {
	// 既有主機應保持原始順序
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		{Name: "alpha", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 1},
		{Name: "beta", Address: "10.0.0.2", Color: "#ff9e64", Enabled: true, SortOrder: 2},
		{Name: "gamma", Address: "10.0.0.3", Color: "#c678dd", Enabled: true, SortOrder: 3},
	}

	result := config.MergeHosts(hosts, []string{"beta"}, false)

	require.Len(t, result, 4)

	// 順序應保持：local, alpha, beta, gamma
	assert.Equal(t, "local", result[0].Name)
	assert.Equal(t, "alpha", result[1].Name)
	assert.Equal(t, "beta", result[2].Name)
	assert.Equal(t, "gamma", result[3].Name)
}

func TestMergeHostsPreserveOrder_NewHostsAppendedAtEnd(t *testing.T) {
	// 新增主機應附加在最後面
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		{Name: "alpha", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 1},
	}

	result := config.MergeHosts(hosts, []string{"new-host"}, false)

	require.Len(t, result, 3)
	assert.Equal(t, "local", result[0].Name)
	assert.Equal(t, "alpha", result[1].Name)
	assert.Equal(t, "new-host", result[2].Name, "新主機應附加在最後")
}

func TestMergeHostsEmptyRemoteFlags(t *testing.T) {
	// 空的 hostFlags slice（非 nil）+ localFlag=false：不修改，直接回傳副本
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: false, SortOrder: 0},
		{Name: "dev", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 1},
	}

	result := config.MergeHosts(hosts, []string{}, false)

	// 無旗標時不修改任何狀態
	assert.False(t, result[0].Enabled, "無旗標時 local 保持原狀（false）")
	assert.True(t, result[1].Enabled, "dev 保持原設定")
}

// --- localFlag 測試 ---

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
	// 無旗標（tsm 無參數）：不修改，直接回傳副本
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		{Name: "dev", Address: "10.0.0.1", Color: "#73daca", Enabled: false, SortOrder: 1},
	}
	result := config.MergeHosts(hosts, nil, false)
	require.Len(t, result, 2)
	assert.True(t, result[0].Enabled, "local 保持啟用")
	assert.False(t, result[1].Enabled, "dev 保持停用")
}

// --- EnsureLocal 測試 ---

func TestEnsureLocal_AlreadyExists(t *testing.T) {
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true},
		{Name: "mlab", Address: "mlab", Color: "#5B9BD5", Enabled: true},
	}
	result := config.EnsureLocal(hosts)
	require.Len(t, result, 2, "已有 local 時不應新增")
	assert.Equal(t, "local", result[0].Name)
}

func TestEnsureLocal_Missing(t *testing.T) {
	hosts := []config.HostEntry{
		{Name: "mlab", Address: "mlab", Color: "#5B9BD5", Enabled: true},
	}
	result := config.EnsureLocal(hosts)
	require.Len(t, result, 2, "應自動補上 local")
	assert.True(t, result[0].IsLocal(), "local 應在最前面")
	assert.True(t, result[0].Enabled, "自動補上的 local 應啟用")
	assert.Equal(t, "mlab", result[1].Name, "既有主機保持在後面")
}

func TestEnsureLocal_EmptyList(t *testing.T) {
	result := config.EnsureLocal(nil)
	require.Len(t, result, 1)
	assert.True(t, result[0].IsLocal())
	assert.True(t, result[0].Enabled)
}

func TestEnsureLocal_ExistsButDisabled(t *testing.T) {
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: false},
	}
	result := config.EnsureLocal(hosts)
	require.Len(t, result, 1, "不應新增")
	assert.False(t, result[0].Enabled, "EnsureLocal 不修改已存在 local 的 Enabled 狀態")
}

// --- DefaultColors 測試 ---

func TestDefaultColors(t *testing.T) {
	assert.Len(t, config.DefaultColors, 8, "DefaultColors 應有 8 個顏色")

	// 所有顏色應為有效的 hex 色碼
	for _, c := range config.DefaultColors {
		assert.Regexp(t, `^#[0-9A-Fa-f]{6}$`, c, "顏色應為 #RRGGBB 格式")
	}

	// 不應有重複
	seen := make(map[string]bool)
	for _, c := range config.DefaultColors {
		assert.False(t, seen[c], "顏色不應重複：%s", c)
		seen[c] = true
	}
}
