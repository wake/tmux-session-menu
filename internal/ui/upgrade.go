package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/wake/tmux-session-menu/internal/upgrade"
)

// upgradeStatus 代表單一主機的升級進度。
type upgradeStatus int

const (
	upgradePending  upgradeStatus = iota
	upgradeRunning_               // 升級執行中（加底線避免與 bool 欄位衝突）
	upgradeSuccess
	upgradeFailed
)

// upgradeItem 代表升級面板中的一台主機。
type upgradeItem struct {
	HostID  string
	Name    string
	Address string
	IsLocal bool
	Version string
	Checked bool
	Status  upgradeStatus
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
	if msg.String() == "esc" {
		m.mode = ModeNormal
		return m, nil
	}
	return m, nil
}
