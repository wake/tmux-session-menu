package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wake/tmux-session-menu/internal/config"
)

// --- MergeHosts 測試 ---

func TestMergeHostsNoRemoteFlag(t *testing.T) {
	// 沒有 --remote 旗標時：local 強制啟用，其他保持原狀
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: false, SortOrder: 0},
		{Name: "server-a", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 1},
		{Name: "server-b", Address: "10.0.0.2", Color: "#ff9e64", Enabled: false, SortOrder: 2},
	}

	result := config.MergeHosts(hosts, nil)

	require.Len(t, result, 3)

	// local 應強制啟用（即使原本 Enabled=false）
	assert.Equal(t, "local", result[0].Name)
	assert.True(t, result[0].Enabled, "local 應強制啟用")

	// 其他主機保持原設定
	assert.Equal(t, "server-a", result[1].Name)
	assert.True(t, result[1].Enabled, "server-a 保持啟用")

	assert.Equal(t, "server-b", result[2].Name)
	assert.False(t, result[2].Enabled, "server-b 保持停用")
}

func TestMergeHostsNoRemoteFlag_NoLocalHost(t *testing.T) {
	// 使用者 config 中沒有 local host 時，應自動補上
	hosts := []config.HostEntry{
		{Name: "server-a", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 1},
	}

	result := config.MergeHosts(hosts, nil)

	// 應自動補上 local host 在最前面
	require.True(t, len(result) >= 2, "應至少有 local + server-a")

	// 第一筆應為 local
	assert.True(t, result[0].IsLocal(), "第一筆應為 local host")
	assert.True(t, result[0].Enabled, "local 應為啟用")
	assert.Equal(t, "local", result[0].Name)

	// server-a 保持原狀
	assert.Equal(t, "server-a", result[1].Name)
	assert.True(t, result[1].Enabled)
}

func TestMergeHostsWithRemoteFlags(t *testing.T) {
	// --remote dev：local 停用，dev 啟用
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		{Name: "dev", Address: "10.0.0.1", Color: "#73daca", Enabled: false, SortOrder: 1},
		{Name: "staging", Address: "10.0.0.2", Color: "#ff9e64", Enabled: true, SortOrder: 2},
	}

	result := config.MergeHosts(hosts, []string{"dev"})

	require.Len(t, result, 3)

	// local 應被停用
	assert.Equal(t, "local", result[0].Name)
	assert.False(t, result[0].Enabled, "有 --remote 時 local 應停用")

	// dev 在旗標中 → 應啟用
	assert.Equal(t, "dev", result[1].Name)
	assert.True(t, result[1].Enabled, "dev 在 --remote 旗標中應啟用")

	// staging 不在旗標中 → 保持原設定
	assert.Equal(t, "staging", result[2].Name)
	assert.True(t, result[2].Enabled, "staging 不在旗標中，保持原設定")
}

func TestMergeHostsNewRemoteHost(t *testing.T) {
	// --remote newhost：不在清單中的主機應自動新增
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
	}

	result := config.MergeHosts(hosts, []string{"newhost"})

	require.Len(t, result, 2, "應有 local + newhost")

	// local 停用
	assert.Equal(t, "local", result[0].Name)
	assert.False(t, result[0].Enabled, "有 --remote 時 local 應停用")

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

	result := config.MergeHosts(hosts, []string{"host-a", "host-b"})

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
	result := config.MergeHosts(hosts, []string{"dev.example.com"})

	require.Len(t, result, 2, "不應新增，因為 Address 已匹配")

	// local 停用
	assert.False(t, result[0].Enabled, "有 --remote 時 local 應停用")

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

	result := config.MergeHosts(hosts, []string{"dev"})

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

	result := config.MergeHosts(hosts, []string{"beta"})

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

	result := config.MergeHosts(hosts, []string{"new-host"})

	require.Len(t, result, 3)
	assert.Equal(t, "local", result[0].Name)
	assert.Equal(t, "alpha", result[1].Name)
	assert.Equal(t, "new-host", result[2].Name, "新主機應附加在最後")
}

func TestMergeHostsEmptyRemoteFlags(t *testing.T) {
	// 空的 remoteFlags slice（非 nil）與有值的行為應一致：
	// 長度為 0 → 視同沒有 --remote，與 nil 相同行為
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Color: "#5f8787", Enabled: false, SortOrder: 0},
		{Name: "dev", Address: "10.0.0.1", Color: "#73daca", Enabled: true, SortOrder: 1},
	}

	result := config.MergeHosts(hosts, []string{})

	// 空 slice 視同 nil：local 強制啟用
	assert.True(t, result[0].Enabled, "空 remoteFlags 時 local 應強制啟用")
	assert.True(t, result[1].Enabled, "dev 保持原設定")
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
