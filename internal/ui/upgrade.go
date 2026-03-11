package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/wake/tmux-session-menu/internal/upgrade"
)

// UpgradeStatus 代表單一主機的升級進度。
type UpgradeStatus int

const (
	UpgradePending  UpgradeStatus = iota
	UpgradeRunning_               // 升級執行中（加底線避免與 bool 欄位衝突）
	UpgradeSuccess
	UpgradeFailed
)

// upgradeItem 代表升級面板中的一台主機。
type upgradeItem struct {
	HostID  string
	Name    string
	Address string
	IsLocal bool
	Version string
	Checked bool
	Status  UpgradeStatus
	NewVer  string
	Error   string
}

// UpgradeItems 回傳升級面板的主機清單（供外部 package 測試使用）。
func (m Model) UpgradeItems() []upgradeItem {
	return m.upgradeItems
}

// enterModeUpgrade 依 hostVersions 建立升級面板項目並切換到 ModeUpgrade。
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

// updateUpgrade 處理 ModeUpgrade 的按鍵事件。
func (m Model) updateUpgrade(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.upgradeRunning {
		if msg.String() == "esc" {
			m.upgradeCancelled = true
		}
		return m, nil
	}

	itemCount := len(m.upgradeItems)

	switch msg.String() {
	case "esc":
		m.mode = ModeNormal
		return m, nil
	case "j", "down":
		if m.upgradeCursor < itemCount { // items 0..N-1, 按鈕區 = N
			m.upgradeCursor++
		}
	case "k", "up":
		if m.upgradeCursor > 0 {
			m.upgradeCursor--
		}
	case "l", "right", "tab":
		if m.upgradeCursor >= itemCount && m.upgradeBtnFocus < 1 {
			m.upgradeBtnFocus = 1
		}
	case "h", "left", "shift+tab":
		if m.upgradeCursor >= itemCount && m.upgradeBtnFocus > 0 {
			m.upgradeBtnFocus = 0
		}
	case " ":
		if m.upgradeCursor < itemCount {
			m.upgradeItems[m.upgradeCursor].Checked = !m.upgradeItems[m.upgradeCursor].Checked
		}
	case "a":
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
		if m.upgradeCursor >= itemCount {
			if m.upgradeBtnFocus == 0 {
				return m.startUpgrade()
			}
			m.mode = ModeNormal
			return m, nil
		}
	}
	return m, nil
}

// startUpgrade 開始升級流程：標記已勾選的遠端項目為 UpgradeRunning_。
func (m Model) startUpgrade() (tea.Model, tea.Cmd) {
	hasChecked := false
	for _, item := range m.upgradeItems {
		if item.Checked {
			hasChecked = true
			break
		}
	}
	if !hasChecked {
		return m, nil
	}

	m.upgradeRunning = true
	m.upgradeCancelled = false

	for i := range m.upgradeItems {
		if m.upgradeItems[i].Checked && !m.upgradeItems[i].IsLocal {
			m.upgradeItems[i].Status = UpgradeRunning_
		}
	}

	// TODO: Task 5 會在這裡加入 remoteUpgradeCmd
	// TODO: Task 6 會在這裡加入 local 升級邏輯
	return m, nil
}

// UpgradeCursor 回傳升級面板的游標位置（供測試使用）。
func (m Model) UpgradeCursor() int { return m.upgradeCursor }

// UpgradeRunning 回傳升級是否正在執行（供測試使用）。
func (m Model) UpgradeRunning() bool { return m.upgradeRunning }

// UpgradeCancelled 回傳升級是否已被取消（供測試使用）。
func (m Model) UpgradeCancelled() bool { return m.upgradeCancelled }

// UpgradeBtnFocus 回傳按鈕區焦點位置（供測試使用）。
func (m Model) UpgradeBtnFocus() int { return m.upgradeBtnFocus }
