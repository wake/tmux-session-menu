package ui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

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

// RemoteUpgradeMsg 單台遠端升級結果。
type RemoteUpgradeMsg struct {
	HostID  string
	Version string // 成功時的新版本
	Error   string // 失敗原因
}

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

// startLocalUpgradeCmd 回傳下載本機升級 binary 的 Cmd。
func (m Model) startLocalUpgradeCmd() tea.Cmd {
	if m.deps.Upgrader == nil {
		return nil
	}
	u := m.deps.Upgrader
	return func() tea.Msg {
		rel, err := u.CheckLatest()
		if err != nil {
			return DownloadUpgradeMsg{Err: err}
		}
		asset := upgrade.AssetName()
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

// startUpgrade 開始升級流程：標記已勾選的遠端項目為 UpgradeRunning_ 並並行執行 remoteUpgradeCmd。
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

	var cmds []tea.Cmd
	for i := range m.upgradeItems {
		if m.upgradeItems[i].Checked && !m.upgradeItems[i].IsLocal {
			m.upgradeItems[i].Status = UpgradeRunning_
			cmds = append(cmds, remoteUpgradeCmd(m.upgradeItems[i].Address, m.upgradeItems[i].HostID))
		}
	}

	if len(cmds) == 0 {
		// 只有 local 被勾選，直接開始本機升級
		for i, item := range m.upgradeItems {
			if item.IsLocal && item.Checked {
				m.upgradeItems[i].Status = UpgradeRunning_
				m.upgradeVersion = m.upgradeLatestVer
				return m, m.startLocalUpgradeCmd()
			}
		}
		// 不應到這裡（hasChecked=true 但沒有 local 也沒有 remote）
		m.upgradeRunning = false
		return m, nil
	}

	return m, tea.Batch(cmds...)
}

// handleRemoteUpgrade 處理單台遠端升級結果，並在所有遠端完成後決定後續流程。
func (m Model) handleRemoteUpgrade(msg RemoteUpgradeMsg) (Model, tea.Cmd) {
	for i := range m.upgradeItems {
		if m.upgradeItems[i].HostID == msg.HostID {
			if msg.Error != "" {
				m.upgradeItems[i].Status = UpgradeFailed
				m.upgradeItems[i].Error = msg.Error
				m.upgradeItems[i].Checked = true // 失敗項自動勾選方便重試
			} else {
				m.upgradeItems[i].Status = UpgradeSuccess
				m.upgradeItems[i].NewVer = msg.Version
				m.upgradeItems[i].Checked = false
			}
			break
		}
	}

	// 檢查是否所有遠端都完成
	allRemoteDone := true
	for _, item := range m.upgradeItems {
		if !item.IsLocal && item.Status == UpgradeRunning_ {
			allRemoteDone = false
			break
		}
	}

	if allRemoteDone {
		if m.upgradeCancelled {
			m.upgradeRunning = false
			return m, nil
		}
		// 檢查 local 是否需要升級
		for i, item := range m.upgradeItems {
			if item.IsLocal && item.Checked {
				m.upgradeItems[i].Status = UpgradeRunning_
				m.upgradeVersion = m.upgradeLatestVer // 設定版本供 runPostUpgrade 使用
				return m, m.startLocalUpgradeCmd()
			}
		}
		// local 未勾選 → 完成
		m.upgradeRunning = false
	}
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
