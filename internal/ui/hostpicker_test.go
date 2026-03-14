package ui_test

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
	"github.com/wake/tmux-session-menu/internal/ui"
)

// hpApplyKey 是 hostpicker 測試用的 applyKey helper。
func hpApplyKey(m ui.Model, key string) (ui.Model, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	updated, cmd := m.Update(msg)
	return updated.(ui.Model), cmd
}

func TestHostPickerNavigation(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "dev", Address: "dev", Color: "#70AD47", Enabled: true})

	deps := ui.Deps{HostMgr: mgr}
	m := ui.NewModel(deps)

	// 按 h 進入 HostPicker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode(), "should enter ModeHostPicker")
	assert.Equal(t, 0, m.HostPickerCursor(), "cursor should start at 0")

	// 按 j 向下
	m, _ = hpApplyKey(m, "j")
	assert.Equal(t, 1, m.HostPickerCursor(), "cursor should move to 1")

	// 按 j 不超過邊界
	m, _ = hpApplyKey(m, "j")
	assert.Equal(t, 1, m.HostPickerCursor(), "cursor should stay at 1 (boundary)")

	// 按 k 向上
	m, _ = hpApplyKey(m, "k")
	assert.Equal(t, 0, m.HostPickerCursor(), "cursor should move back to 0")

	// 按 k 不超過邊界
	m, _ = hpApplyKey(m, "k")
	assert.Equal(t, 0, m.HostPickerCursor(), "cursor should stay at 0 (boundary)")

	// 按 esc 回到 Normal
	m, _ = hpApplyKey(m, "esc")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "should return to ModeNormal after esc")
}

func TestHostPickerCloseWithH(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 按 h 進入
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 再按 h 關閉
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "pressing h again should close HostPicker")
}

func TestHostPickerNoHostMgr(t *testing.T) {
	// 無 HostMgr 時按 h 不進入 HostPicker
	m := ui.NewModel(ui.Deps{})
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeNormal, m.Mode(), "should stay ModeNormal without HostMgr")
}

func TestHostPickerAddHost(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 按 n 進入新增主機輸入模式（原 a 已改為 n）
	m, _ = hpApplyKey(m, "n")
	assert.Equal(t, ui.ModeInput, m.Mode(), "pressing n should enter ModeInput")
	assert.Equal(t, ui.InputNewHost, m.InputTarget(), "input target should be InputNewHost")
}

func TestHostPickerDeleteLocalProtected(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker，游標在 local 主機上
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 按 d 不應進入確認模式（local 不可刪除）
	m, _ = hpApplyKey(m, "d")
	assert.Equal(t, ui.ModeHostPicker, m.Mode(), "should stay in ModeHostPicker when deleting local")
}

func TestHostPickerDeleteRemote(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "dev", Address: "dev.example.com", Color: "#70AD47", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker，移到第二個主機（遠端）
	m, _ = hpApplyKey(m, "h")
	m, _ = hpApplyKey(m, "j")
	assert.Equal(t, 1, m.HostPickerCursor())

	// 按 d 應直接封存（不再進入確認模式）
	m, _ = hpApplyKey(m, "d")
	assert.Equal(t, ui.ModeHostPicker, m.Mode(), "should stay in ModeHostPicker after archive")
	// 驗證已封存
	h := mgr.Host("dev")
	assert.True(t, h.Config().Archived, "dev should be archived")
}

func TestHostPickerReorder(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "aaa", Color: "#111111", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "bbb", Color: "#222222", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "ccc", Color: "#333333", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, 0, m.HostPickerCursor())

	// 按 J（shift+j）下移第一個主機
	m, _ = hpApplyKey(m, "J")
	assert.Equal(t, 1, m.HostPickerCursor(), "cursor should follow moved item to position 1")

	// 驗證順序：bbb, aaa, ccc
	hosts := mgr.Hosts()
	assert.Equal(t, "bbb", hosts[0].ID())
	assert.Equal(t, "aaa", hosts[1].ID())
	assert.Equal(t, "ccc", hosts[2].ID())

	// 按 K（shift+k）上移回去
	m, _ = hpApplyKey(m, "K")
	assert.Equal(t, 0, m.HostPickerCursor(), "cursor should follow moved item back to position 0")

	hosts = mgr.Hosts()
	assert.Equal(t, "aaa", hosts[0].ID())
	assert.Equal(t, "bbb", hosts[1].ID())
	assert.Equal(t, "ccc", hosts[2].ID())
}

func TestHostPickerRenderContainsHosts(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "staging", Address: "staging.example.com", Color: "#70AD47", Enabled: false})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker 並渲染
	m, _ = hpApplyKey(m, "h")
	view := m.View()

	assert.Contains(t, view, "主機管理", "should show panel title")
	assert.Contains(t, view, "local", "should show local host name")
	assert.Contains(t, view, "staging", "should show staging host name")
	assert.Contains(t, view, "已停用", "should show disabled status for staging")
}

func TestHostPickerToolbarVisible(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})
	view := m.View()
	assert.Contains(t, view, "主機管理", "toolbar should show host management shortcut in multi-host mode")
}

func TestHostPickerToolbarHiddenWithoutHostMgr(t *testing.T) {
	m := ui.NewModel(ui.Deps{})
	view := m.View()
	// [h] 主機管理 should not appear without HostMgr
	assert.NotContains(t, view, "主機管理", "toolbar should not show host management shortcut without HostMgr")
}

func TestHostPicker_PersistHosts_AfterToggle(t *testing.T) {
	// 建立 temp config
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := config.Default()
	_ = config.SaveConfig(cfgPath, cfg)

	// 建立 HostManager 並加入主機
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Address: "", Color: "#5f8787", Enabled: true})

	deps := ui.Deps{HostMgr: mgr, Cfg: cfg, ConfigPath: cfgPath}
	m := ui.NewModel(deps)

	// 進入 host picker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 按 space 切換啟停用
	m, _ = hpApplyKey(m, " ")

	// 驗證 config 已寫入
	data, err := os.ReadFile(cfgPath)
	assert.NoError(t, err)
	loaded, err := config.LoadFromString(string(data))
	assert.NoError(t, err)
	assert.NotEmpty(t, loaded.Hosts)
}

func TestHostPicker_PersistHosts_AfterReorder(t *testing.T) {
	// 建立 temp config
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := config.Default()
	_ = config.SaveConfig(cfgPath, cfg)

	// 建立 HostManager 並加入多台主機
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "aaa", Color: "#111111", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "bbb", Color: "#222222", Enabled: true})

	deps := ui.Deps{HostMgr: mgr, Cfg: cfg, ConfigPath: cfgPath}
	m := ui.NewModel(deps)

	// 進入 host picker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 按 J 下移第一個主機
	m, _ = hpApplyKey(m, "J")

	// 驗證 config 已寫入且順序正確
	data, err := os.ReadFile(cfgPath)
	assert.NoError(t, err)
	loaded, err := config.LoadFromString(string(data))
	assert.NoError(t, err)
	assert.Len(t, loaded.Hosts, 2)
	assert.Equal(t, "bbb", loaded.Hosts[0].Name)
	assert.Equal(t, "aaa", loaded.Hosts[1].Name)
	assert.Equal(t, 0, loaded.Hosts[0].SortOrder)
	assert.Equal(t, 1, loaded.Hosts[1].SortOrder)
}

func TestHostPicker_PersistHosts_NoConfigPath(t *testing.T) {
	// 沒有設定 ConfigPath 時不應寫入
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5f8787", Enabled: true})

	deps := ui.Deps{HostMgr: mgr}
	m := ui.NewModel(deps)

	// 進入 host picker 並切換啟停用（不應 panic）
	m, _ = hpApplyKey(m, "h")
	m, _ = hpApplyKey(m, " ")
	// 測試通過即代表沒有 panic
}

// --- Task 7: 三態操作測試 ---

func TestHostPicker_NewKeyIsN(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 按 n 應進入新增主機輸入模式
	m, _ = hpApplyKey(m, "n")
	assert.Equal(t, ui.ModeInput, m.Mode(), "按 n 應進入 ModeInput")
	assert.Equal(t, ui.InputNewHost, m.InputTarget(), "input target 應為 InputNewHost")
}

func TestHostPicker_OldKeyANoLongerAdds(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 按 a 不應再進入新增模式
	m, _ = hpApplyKey(m, "a")
	assert.NotEqual(t, ui.ModeInput, m.Mode(), "按 a 不應再進入 ModeInput")
}

func TestHostPicker_ArchiveHost(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := config.Default()
	_ = config.SaveConfig(cfgPath, cfg)

	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "dev", Address: "dev.example.com", Color: "#70AD47", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: cfg, ConfigPath: cfgPath})

	// 進入 HostPicker，移到第二個主機（遠端）
	m, _ = hpApplyKey(m, "h")
	m, _ = hpApplyKey(m, "j")
	assert.Equal(t, 1, m.HostPickerCursor())

	// 按 d 應直接封存（不進入 ModeConfirm）
	m, _ = hpApplyKey(m, "d")
	assert.NotEqual(t, ui.ModeConfirm, m.Mode(), "按 d 不應進入 ModeConfirm，應直接封存")

	// 驗證主機已封存
	h := mgr.Host("dev")
	require.NotNil(t, h)
	devCfg := h.Config()
	assert.True(t, devCfg.Archived, "dev 應被封存")
	assert.False(t, devCfg.Enabled, "dev 封存後應停用")
}

func TestHostPicker_ArchiveLocalProtected(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5B9BD5", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr})

	// 進入 HostPicker，游標在 local
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 按 d 不應封存 local
	m, _ = hpApplyKey(m, "d")
	h := mgr.Host("local")
	require.NotNil(t, h)
	assert.False(t, h.Config().Archived, "local 不應被封存")
}

// --- Hub 模式 host picker 測試 ---

func TestHubMode_HostPickerToolbarVisible(t *testing.T) {
	// Hub 模式（HubMode=true, HostMgr=nil）也應在 toolbar 顯示 [h] 主機管理
	m := ui.NewModel(ui.Deps{
		HubMode: true,
		Cfg: config.Config{Hosts: []config.HostEntry{
			{Name: "local", Enabled: true, Color: "#5f8787"},
			{Name: "mlab", Address: "mlab", Enabled: true, Color: "#70AD47"},
		}},
	})
	// 先餵一個 HubSnapshotMsg 使 items 非空（否則 View 可能沒渲染完整 toolbar）
	snap := &tsmv1.MultiHostSnapshot{
		Hosts: []*tsmv1.HostState{
			{HostId: "local", Name: "local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
			{HostId: "mlab", Name: "mlab", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
		},
	}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)

	view := m.View()
	assert.Contains(t, view, "主機管理", "hub 模式 toolbar 應顯示 [h] 主機管理")
}

func TestHubMode_HKeyEntersHostPicker(t *testing.T) {
	// Hub 模式下按 h 應進入 ModeHostPicker
	m := ui.NewModel(ui.Deps{
		HubMode: true,
		Cfg: config.Config{Hosts: []config.HostEntry{
			{Name: "local", Enabled: true, Color: "#5f8787"},
			{Name: "mlab", Address: "mlab", Enabled: true, Color: "#70AD47"},
		}},
	})
	snap := &tsmv1.MultiHostSnapshot{
		Hosts: []*tsmv1.HostState{
			{HostId: "local", Name: "local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
			{HostId: "mlab", Name: "mlab", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTING},
		},
	}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)

	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode(), "hub 模式按 h 應進入 ModeHostPicker")
}

func TestHubMode_HostPickerRendersHostStatus(t *testing.T) {
	// Hub 模式 host picker 應顯示從 HubSnapshotMsg 取得的主機狀態
	m := ui.NewModel(ui.Deps{
		HubMode: true,
		Cfg: config.Config{Hosts: []config.HostEntry{
			{Name: "local", Enabled: true, Color: "#5f8787"},
			{Name: "mlab", Address: "mlab", Enabled: true, Color: "#70AD47"},
		}},
	})
	snap := &tsmv1.MultiHostSnapshot{
		Hosts: []*tsmv1.HostState{
			{HostId: "local", Name: "local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
			{HostId: "mlab", Name: "mlab", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTING},
		},
	}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)

	m, _ = hpApplyKey(m, "h")
	view := m.View()
	assert.Contains(t, view, "主機管理", "應顯示面板標題")
	assert.Contains(t, view, "local", "應顯示 local 主機")
	assert.Contains(t, view, "mlab", "應顯示 mlab 主機")
	assert.Contains(t, view, "連線中", "應顯示 mlab 的 CONNECTING 狀態")
}

func TestHubMode_HostPickerNavigation(t *testing.T) {
	// Hub 模式 host picker 應支援 j/k 導航
	m := ui.NewModel(ui.Deps{
		HubMode: true,
		Cfg: config.Config{Hosts: []config.HostEntry{
			{Name: "local", Enabled: true, Color: "#5f8787"},
			{Name: "mlab", Address: "mlab", Enabled: true, Color: "#70AD47"},
		}},
	})
	snap := &tsmv1.MultiHostSnapshot{
		Hosts: []*tsmv1.HostState{
			{HostId: "local", Name: "local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
			{HostId: "mlab", Name: "mlab", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
		},
	}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)

	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, 0, m.HostPickerCursor())

	m, _ = hpApplyKey(m, "j")
	assert.Equal(t, 1, m.HostPickerCursor())

	m, _ = hpApplyKey(m, "k")
	assert.Equal(t, 0, m.HostPickerCursor())

	// esc 關閉
	m, _ = hpApplyKey(m, "esc")
	assert.Equal(t, ui.ModeNormal, m.Mode())
}

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

// --- Task 9/10/11: 右側設定面板測試 ---

func TestHostPicker_OpenPanel(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true, BarBG: "#1a2b2b", BadgeBG: "#e0af68", BadgeFG: "#1a1b26"})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = hpApplyKey(m, "h")
	m, _ = hpApplyKey(m, "j") // 移到 mlab1

	// 按 Enter 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := updated.(ui.Model)
	assert.True(t, fm.HostPanelOpen(), "面板應開啟")
	assert.Equal(t, 0, fm.HostPanelCursor(), "面板游標應在第 0 個欄位")
}

func TestHostPicker_OpenPanelWithRight(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = hpApplyKey(m, "h")

	// 按 → 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	fm := updated.(ui.Model)
	assert.True(t, fm.HostPanelOpen(), "按 → 應開啟面板")
}

func TestHostPicker_ClosePanel(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true})
	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = hpApplyKey(m, "h")

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)
	assert.True(t, m.HostPanelOpen())

	// 按 Esc 關閉
	updated2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	fm := updated2.(ui.Model)
	assert.False(t, fm.HostPanelOpen(), "Esc 應關閉面板")
}

func TestHostPicker_ClosePanelWithLeft(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true})
	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = hpApplyKey(m, "h")

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	// 按 ← 關閉
	updated2, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	fm := updated2.(ui.Model)
	assert.False(t, fm.HostPanelOpen(), "← 應關閉面板")
}

func TestHostPicker_PanelShowsColors(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true, BarBG: "#1a2b2b", BadgeBG: "#e0af68", BadgeFG: "#1a1b26"})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = hpApplyKey(m, "h")

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := updated.(ui.Model)
	view := fm.View()
	assert.Contains(t, view, "bar_bg", "應顯示 bar_bg 標籤")
	assert.Contains(t, view, "#1a2b2b", "應顯示 bar_bg 值")
	assert.Contains(t, view, "badge_bg", "應顯示 badge_bg 標籤")
}

func TestHostPicker_PreviewRendered(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true, BarBG: "#1a2b2b", BadgeBG: "#e0af68", BadgeFG: "#1a1b26"})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = hpApplyKey(m, "h")

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := updated.(ui.Model)
	view := fm.View()
	// 預覽應顯示主機名與範例 window
	assert.Contains(t, view, "mlab1", "預覽應顯示主機名")
	assert.Contains(t, view, "0:zsh", "預覽應顯示範例 window")
}

func TestHostPicker_PanelNavigation(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = hpApplyKey(m, "h")

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	// 面板內 j 向下
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(ui.Model)
	assert.Equal(t, 1, m.HostPanelCursor(), "面板游標應到 1")

	// 面板內 k 向上
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(ui.Model)
	assert.Equal(t, 0, m.HostPanelCursor(), "面板游標應回到 0")
}

func TestHostPicker_PanelToggleEnabled(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = hpApplyKey(m, "h")

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	// 游標在第 0 個（啟用 toggle），按空白鍵切換
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	m = updated.(ui.Model)
	view := m.View()
	// 切換後應顯示 [ ] 而非 [x]
	assert.Contains(t, view, "[ ]", "切換後啟用欄應為 [ ]")
}

func TestHostPicker_PanelEditColor(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true, BarBG: "#111111"})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = hpApplyKey(m, "h")

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	// 移到 bar_bg（index 1）
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(ui.Model)
	assert.Equal(t, 1, m.HostPanelCursor())

	// 按 Enter 進入編輯
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)
	assert.True(t, m.HostPanelEditing(), "應進入編輯模式")

	// 按 Esc 取消編輯
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(ui.Model)
	assert.False(t, m.HostPanelEditing(), "Esc 應退出編輯模式")
}

func TestHostPicker_CtrlS_SaveDrafts(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := config.Default()
	_ = config.SaveConfig(cfgPath, cfg)

	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5f8787", Enabled: true, BadgeBG: "#5f8787", BadgeFG: "#c0caf5"})
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Color: "#70AD47", Enabled: true})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: cfg, ConfigPath: cfgPath})
	m, _ = hpApplyKey(m, "h")
	m, _ = hpApplyKey(m, "j") // 移到 mlab1

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	// 移到 bar_bg 並編輯
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(ui.Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // 進入編輯
	m = updated.(ui.Model)

	// 輸入色碼
	for _, r := range "#aabbcc" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(ui.Model)
	}
	// 確認
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	// Ctrl+S 儲存
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(ui.Model)

	// 面板應關閉
	assert.False(t, m.HostPanelOpen(), "Ctrl+S 後面板應關閉")

	// 驗證 HostMgr 已更新
	h := mgr.Host("mlab1")
	require.NotNil(t, h)
	assert.Equal(t, "#aabbcc", h.Config().BarBG, "mlab1 的 bar_bg 應更新")

	// 驗證 config 已寫入
	data, err := os.ReadFile(cfgPath)
	assert.NoError(t, err)
	loaded, err := config.LoadFromString(string(data))
	assert.NoError(t, err)
	// 找 mlab1
	var found bool
	for _, entry := range loaded.Hosts {
		if entry.Name == "mlab1" {
			assert.Equal(t, "#aabbcc", entry.BarBG)
			found = true
		}
	}
	assert.True(t, found, "config 應包含 mlab1")
}

func TestHostPicker_CtrlS_SyncsLocalToConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := config.Default()
	_ = config.SaveConfig(cfgPath, cfg)

	mgr := hostmgr.New()
	// badge_bg 起始為空，方便測試輸入
	mgr.AddHost(config.HostEntry{Name: "local", Color: "#5f8787", Enabled: true, BadgeFG: "#c0caf5"})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: cfg, ConfigPath: cfgPath})
	m, _ = hpApplyKey(m, "h")

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	// 移到 badge_bg（index 3）並編輯
	for i := 0; i < 3; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(ui.Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)
	for _, r := range "#ffaa00" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(ui.Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	// Ctrl+S 儲存
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(ui.Model)

	// 驗證 Config.Local 已同步
	data, err := os.ReadFile(cfgPath)
	assert.NoError(t, err)
	loaded, err := config.LoadFromString(string(data))
	assert.NoError(t, err)
	assert.Equal(t, "#ffaa00", loaded.Local.BadgeBG, "Config.Local.BadgeBG 應同步")
}

func TestHostPicker_HintForEmptyBarFG(t *testing.T) {
	mgr := hostmgr.New()
	mgr.AddHost(config.HostEntry{Name: "mlab1", Address: "mlab1", Enabled: true, BarBG: "#1a2b2b"})

	m := ui.NewModel(ui.Deps{HostMgr: mgr, Cfg: config.Default()})
	m, _ = hpApplyKey(m, "h")

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := updated.(ui.Model)
	view := fm.View()
	assert.Contains(t, view, "留空由 tmux 自行決定", "bar_fg 空值時應顯示提示")
}

func TestHubMode_SelectedItem_HasHostID(t *testing.T) {
	// Hub 模式下選取 session 應回傳包含 HostID 的 ListItem
	m := ui.NewModel(ui.Deps{HubMode: true})
	snap := &tsmv1.MultiHostSnapshot{
		Hosts: []*tsmv1.HostState{
			{
				HostId: "mlab", Name: "mlab",
				Status:   tsmv1.HostStatus_HOST_STATUS_CONNECTED,
				Snapshot: &tsmv1.StateSnapshot{Sessions: []*tsmv1.Session{{Name: "dev", Id: "$1"}}},
			},
		},
	}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)

	// 模擬 Enter 選取第一個 session
	items := m.Items()
	// 找到第一個 session item
	for i, item := range items {
		if item.Type == ui.ItemSession {
			// 設定 cursor 到該位置
			for j := 0; j < i; j++ {
				m, _ = hpApplyKey(m, "j")
			}
			break
		}
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	assert.Equal(t, "dev", m.Selected(), "應選取 dev session")
	item := m.SelectedItem()
	assert.Equal(t, "mlab", item.HostID, "SelectedItem 的 HostID 應為 mlab")
}

// --- Hub 模式 host picker 編輯功能測試 ---

// newHubTestModel 建立 hub 模式測試用 Model，含預設主機與快照。
func newHubTestModel(t *testing.T) ui.Model {
	t.Helper()
	deps := ui.Deps{
		HubMode:    true,
		ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
		Cfg: config.Config{
			Hosts: []config.HostEntry{
				{Name: "local", Enabled: true, Color: "#5f8787"},
				{Name: "mlab", Address: "mlab", Enabled: true, Color: "#70AD47"},
			},
		},
	}
	// 寫入初始設定檔供 persistHubHosts 讀取
	_ = config.SaveConfig(deps.ConfigPath, deps.Cfg)
	m := ui.NewModel(deps)
	// 餵入 hub snapshot
	snap := &tsmv1.MultiHostSnapshot{Hosts: []*tsmv1.HostState{
		{HostId: "local", Name: "local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
		{HostId: "mlab", Name: "mlab", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
	}}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)
	// 進入 host picker
	m, _ = hpApplyKey(m, "h")
	return m
}

func TestHubMode_HostPicker_SpaceToggle(t *testing.T) {
	m := newHubTestModel(t)
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 游標在 local（index 0），按 space 切換停用
	m, _ = hpApplyKey(m, " ")

	// 驗證 deps.Cfg.Hosts 中 local 已停用
	cfg := m.DepsCfg()
	var localEntry config.HostEntry
	for _, h := range cfg.Hosts {
		if h.Name == "local" {
			localEntry = h
			break
		}
	}
	assert.False(t, localEntry.Enabled, "local 按 space 後應停用")

	// 再按一次 space 重新啟用
	m, _ = hpApplyKey(m, " ")
	cfg = m.DepsCfg()
	for _, h := range cfg.Hosts {
		if h.Name == "local" {
			localEntry = h
			break
		}
	}
	assert.True(t, localEntry.Enabled, "local 再按 space 後應啟用")
}

func TestHubMode_HostPicker_Archive(t *testing.T) {
	m := newHubTestModel(t)

	// 移到 mlab（index 1）
	m, _ = hpApplyKey(m, "j")
	assert.Equal(t, 1, m.HostPickerCursor())

	// 按 d 封存
	m, _ = hpApplyKey(m, "d")
	assert.Equal(t, ui.ModeHostPicker, m.Mode(), "封存後應留在 ModeHostPicker")

	// 驗證 mlab 已封存
	cfg := m.DepsCfg()
	for _, h := range cfg.Hosts {
		if h.Name == "mlab" {
			assert.True(t, h.Archived, "mlab 應被封存")
			assert.False(t, h.Enabled, "mlab 封存後應停用")
		}
	}

}

func TestHubMode_HostPicker_ArchiveLocalProtected(t *testing.T) {
	m := newHubTestModel(t)

	// 游標在 local（index 0），按 d 不應封存
	m, _ = hpApplyKey(m, "d")
	cfg := m.DepsCfg()
	for _, h := range cfg.Hosts {
		if h.Name == "local" {
			assert.False(t, h.Archived, "local 不應被封存")
		}
	}
}

func TestHubMode_HostPicker_OpenPanel(t *testing.T) {
	m := newHubTestModel(t)

	// 按 Enter 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)
	assert.True(t, m.HostPanelOpen(), "Enter 應開啟面板")
	assert.Equal(t, 0, m.HostPanelCursor(), "面板游標應在 0")

	// 面板中顯示設定欄位
	view := m.View()
	assert.Contains(t, view, "bar_bg", "面板應顯示 bar_bg 標籤")
	assert.Contains(t, view, "badge_bg", "面板應顯示 badge_bg 標籤")
}

func TestHubMode_HostPicker_PanelEditColor(t *testing.T) {
	m := newHubTestModel(t)

	// 開啟面板
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	// 移到 bar_bg（index 1）
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(ui.Model)
	assert.Equal(t, 1, m.HostPanelCursor())

	// 按 Enter 進入編輯
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)
	assert.True(t, m.HostPanelEditing(), "應進入編輯模式")

	// 輸入色碼
	for _, r := range "#aabbcc" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(ui.Model)
	}

	// 確認
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)
	assert.False(t, m.HostPanelEditing(), "Enter 應退出編輯模式")
}

func TestHubMode_HostPicker_CtrlS_Save(t *testing.T) {
	deps := ui.Deps{
		HubMode:    true,
		ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
		Cfg: config.Config{
			Hosts: []config.HostEntry{
				{Name: "local", Enabled: true, Color: "#5f8787"},
				{Name: "mlab", Address: "mlab", Enabled: true, Color: "#70AD47"},
			},
		},
	}
	_ = config.SaveConfig(deps.ConfigPath, deps.Cfg)
	m := ui.NewModel(deps)
	snap := &tsmv1.MultiHostSnapshot{Hosts: []*tsmv1.HostState{
		{HostId: "local", Name: "local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
		{HostId: "mlab", Name: "mlab", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
	}}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)
	m, _ = hpApplyKey(m, "h")

	// 移到 mlab 並開啟面板
	m, _ = hpApplyKey(m, "j")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(ui.Model)

	// 移到 bar_bg 並編輯
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(ui.Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // 進入編輯
	m = updated.(ui.Model)

	// 輸入色碼
	for _, r := range "#aabbcc" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(ui.Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // 確認
	m = updated.(ui.Model)

	// Ctrl+S 儲存
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(ui.Model)

	// 面板應關閉
	assert.False(t, m.HostPanelOpen(), "Ctrl+S 後面板應關閉")
	assert.Equal(t, "已儲存", m.HostSavedMsg(), "應顯示已儲存")

	// 驗證 deps.Cfg.Hosts 已更新
	cfg := m.DepsCfg()
	for _, h := range cfg.Hosts {
		if h.Name == "mlab" {
			assert.Equal(t, "#aabbcc", h.BarBG, "mlab 的 bar_bg 應更新為 #aabbcc")
		}
	}

	// 驗證 config 已寫入檔案
	data, err := os.ReadFile(deps.ConfigPath)
	assert.NoError(t, err)
	loaded, err := config.LoadFromString(string(data))
	assert.NoError(t, err)
	var found bool
	for _, entry := range loaded.Hosts {
		if entry.Name == "mlab" {
			assert.Equal(t, "#aabbcc", entry.BarBG)
			found = true
		}
	}
	assert.True(t, found, "config 應包含 mlab")
}

// --- Hub 模式主機自動同步與即時重建測試 ---

func TestHubMode_SyncHubHostsToConfig(t *testing.T) {
	// 設定只有 local 的 config（模擬新客戶端）
	deps := ui.Deps{
		HubMode:    true,
		ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
		Cfg: config.Config{
			Hosts: []config.HostEntry{
				{Name: "local", Enabled: true, Color: "#5f8787"},
			},
		},
	}
	_ = config.SaveConfig(deps.ConfigPath, deps.Cfg)
	m := ui.NewModel(deps)

	// Hub 快照包含 local + mlab + air（config 只有 local）
	snap := &tsmv1.MultiHostSnapshot{Hosts: []*tsmv1.HostState{
		{HostId: "local", Name: "local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
		{HostId: "mlab", Name: "mlab", Address: "mlab.local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED, Color: "#70AD47"},
		{HostId: "air", Name: "air", Address: "air.local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
	}}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)

	// 驗證 config 已自動補入 mlab 和 air
	cfg := m.DepsCfg()
	assert.Len(t, cfg.Hosts, 3, "應有 3 台主機（local + mlab + air）")

	var mlabEntry, airEntry config.HostEntry
	for _, h := range cfg.Hosts {
		switch h.Name {
		case "mlab":
			mlabEntry = h
		case "air":
			airEntry = h
		}
	}
	assert.Equal(t, "mlab.local", mlabEntry.Address, "mlab 的 Address 應從 hub 同步")
	assert.Equal(t, "#70AD47", mlabEntry.Color, "mlab 的 Color 應從 hub 同步")
	assert.True(t, mlabEntry.Enabled, "新同步的主機應預設啟用")

	assert.Equal(t, "air.local", airEntry.Address, "air 的 Address 應從 hub 同步")
	assert.True(t, airEntry.Enabled, "新同步的主機應預設啟用")
	assert.NotEmpty(t, airEntry.Color, "新同步的主機應有自動分配的顏色")

	// 驗證持久化到檔案
	data, err := os.ReadFile(deps.ConfigPath)
	require.NoError(t, err)
	loaded, err := config.LoadFromString(string(data))
	require.NoError(t, err)
	assert.Len(t, loaded.Hosts, 3, "config 檔案應有 3 台主機")
}

func TestHubMode_SyncSkipsExistingAndArchived(t *testing.T) {
	deps := ui.Deps{
		HubMode:    true,
		ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
		Cfg: config.Config{
			Hosts: []config.HostEntry{
				{Name: "local", Enabled: true, Color: "#5f8787"},
				{Name: "mlab", Address: "mlab", Enabled: true, Color: "#custom"},
				{Name: "old", Address: "old", Enabled: false, Archived: true},
			},
		},
	}
	_ = config.SaveConfig(deps.ConfigPath, deps.Cfg)
	m := ui.NewModel(deps)

	snap := &tsmv1.MultiHostSnapshot{Hosts: []*tsmv1.HostState{
		{HostId: "local", Name: "local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
		{HostId: "mlab", Name: "mlab", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED, Color: "#override"},
		{HostId: "old", Name: "old", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
	}}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)

	cfg := m.DepsCfg()
	// 不應增加（mlab 已存在、old 已封存但仍在 config）
	assert.Len(t, cfg.Hosts, 3, "不應新增主機")

	for _, h := range cfg.Hosts {
		if h.Name == "mlab" {
			assert.Equal(t, "#custom", h.Color, "已存在的主機顏色不應被覆蓋")
		}
		if h.Name == "old" {
			assert.True(t, h.Archived, "已封存的主機不應被解封存")
		}
	}
}

func TestHubMode_SpaceToggle_RebuildsItems(t *testing.T) {
	deps := ui.Deps{
		HubMode:    true,
		ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
		Cfg: config.Config{
			Hosts: []config.HostEntry{
				{Name: "local", Enabled: true, Color: "#5f8787"},
				{Name: "mlab", Address: "mlab", Enabled: true, Color: "#70AD47"},
			},
		},
	}
	_ = config.SaveConfig(deps.ConfigPath, deps.Cfg)
	m := ui.NewModel(deps)

	// 餵入 hub 快照（各主機有 session）
	snap := &tsmv1.MultiHostSnapshot{Hosts: []*tsmv1.HostState{
		{HostId: "local", Name: "local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED,
			Snapshot: &tsmv1.StateSnapshot{Sessions: []*tsmv1.Session{{Name: "main"}}}},
		{HostId: "mlab", Name: "mlab", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED,
			Snapshot: &tsmv1.StateSnapshot{Sessions: []*tsmv1.Session{{Name: "dev"}}}},
	}}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)

	// 確認兩台主機的 session 都在列表中
	items := m.Items()
	hasLocal := false
	hasMlab := false
	for _, it := range items {
		if it.HostID == "local" {
			hasLocal = true
		}
		if it.HostID == "mlab" {
			hasMlab = true
		}
	}
	assert.True(t, hasLocal, "初始應有 local 的 session")
	assert.True(t, hasMlab, "初始應有 mlab 的 session")

	// 進入 host picker，移到 mlab（index 1），按 space 停用
	m, _ = hpApplyKey(m, "h")
	m, _ = hpApplyKey(m, "j")
	m, _ = hpApplyKey(m, " ")

	// 驗證 session 列表已即時更新：mlab 的 session 應被過濾
	items = m.Items()
	hasMlab = false
	for _, it := range items {
		if it.HostID == "mlab" {
			hasMlab = true
		}
	}
	assert.False(t, hasMlab, "停用 mlab 後其 session 應從列表消失")

	// 再按 space 重新啟用
	m, _ = hpApplyKey(m, " ")
	items = m.Items()
	hasMlab = false
	for _, it := range items {
		if it.HostID == "mlab" {
			hasMlab = true
		}
	}
	assert.True(t, hasMlab, "重新啟用 mlab 後其 session 應回到列表")
}

func TestHubMode_HostPicker_ShowsHubHosts(t *testing.T) {
	// 模擬新客戶端（config 只有 local），hub 有多台主機
	deps := ui.Deps{
		HubMode:    true,
		ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
		Cfg: config.Config{
			Hosts: []config.HostEntry{
				{Name: "local", Enabled: true, Color: "#5f8787"},
			},
		},
	}
	_ = config.SaveConfig(deps.ConfigPath, deps.Cfg)
	m := ui.NewModel(deps)

	snap := &tsmv1.MultiHostSnapshot{Hosts: []*tsmv1.HostState{
		{HostId: "local", Name: "local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
		{HostId: "mlab", Name: "mlab", Address: "mlab.local", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED, Color: "#70AD47"},
	}}
	updated, _ := m.Update(ui.HubSnapshotMsg{Snapshot: snap})
	m = updated.(ui.Model)

	// 進入 host picker
	m, _ = hpApplyKey(m, "h")
	assert.Equal(t, ui.ModeHostPicker, m.Mode())

	// 渲染應包含 mlab（從 hub 同步過來的主機）
	view := m.View()
	assert.Contains(t, view, "mlab", "host picker 應顯示從 hub 同步的 mlab")
}

// TestApplyCurrentStatusBarCmd_HubMode 驗證 Hub 模式（HostMgr==nil）也能套用 local host 色彩。
func TestApplyCurrentStatusBarCmd_HubMode(t *testing.T) {
	deps := ui.Deps{
		HubMode: true,
		Cfg: config.Config{
			Hosts: []config.HostEntry{
				{Name: "local", BarBG: "#111111", BarFG: "#222222", BadgeBG: "#333333", BadgeFG: "#444444"},
				{Name: "remote", Address: "remote.local", BarBG: "#aaaaaa"},
			},
		},
	}
	m := ui.NewModel(deps)
	cmd := m.ApplyCurrentStatusBarCmd()
	assert.NotNil(t, cmd, "Hub mode 有 local host 時應回傳非 nil cmd")
}

// TestApplyCurrentStatusBarCmd_NoLocal 驗證無 local host 時回傳 nil。
func TestApplyCurrentStatusBarCmd_NoLocal(t *testing.T) {
	deps := ui.Deps{
		HubMode: true,
		Cfg: config.Config{
			Hosts: []config.HostEntry{
				{Name: "remote", Address: "remote.local"},
			},
		},
	}
	m := ui.NewModel(deps)
	cmd := m.ApplyCurrentStatusBarCmd()
	assert.Nil(t, cmd, "無 local host 時應回傳 nil")
}
