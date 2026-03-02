package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// tsmEvents 是 tsm 管理的 6 個 Claude Code hook event
var tsmEvents = []string{
	"UserPromptSubmit",
	"Stop",
	"SessionStart",
	"SessionEnd",
	"PermissionRequest",
	"Notification",
}

// Result 表示 Install/Uninstall 操作的結果
type Result struct {
	Changed       bool
	EventsAdded   []string
	EventsRemoved []string
	NewSettings   string
}

// Manager 管理 Claude Code settings.json 中的 tsm hook 設定
type Manager struct {
	settingsPath string
	scriptPath   string
}

// NewManager 建立新的 Manager
func NewManager(settingsPath, scriptPath string) *Manager {
	return &Manager{
		settingsPath: settingsPath,
		scriptPath:   scriptPath,
	}
}

// readSettings 讀取並解析 settings.json
func (m *Manager) readSettings() (map[string]interface{}, error) {
	data, err := os.ReadFile(m.settingsPath)
	if err != nil {
		return nil, fmt.Errorf("讀取 settings.json 失敗: %w", err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("解析 settings.json 失敗: %w", err)
	}
	return settings, nil
}

// writeSettings 將 settings 寫回 settings.json（atomic write）
func (m *Manager) writeSettings(settings map[string]interface{}) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize settings: %w", err)
	}
	data = append(data, '\n') // trailing newline
	tmp, err := os.CreateTemp(filepath.Dir(m.settingsPath), ".settings-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, m.settingsPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename settings: %w", err)
	}
	return nil
}

// getOrCreateHooksMap 取得或建立 settings 中的 "hooks" map
func getOrCreateHooksMap(settings map[string]interface{}) map[string]interface{} {
	if raw, ok := settings["hooks"]; ok {
		if hooksMap, ok := raw.(map[string]interface{}); ok {
			return hooksMap
		}
	}
	hooksMap := make(map[string]interface{})
	settings["hooks"] = hooksMap
	return hooksMap
}

// hasTSMHook 檢查 event 的 matcher group 列表中是否已存在 tsm hook
func (m *Manager) hasTSMHook(entries []interface{}) bool {
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		innerHooks, ok := entryMap["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range innerHooks {
			hookMap, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if hookMap["command"] == m.scriptPath {
				return true
			}
		}
	}
	return false
}

// newMatcherGroup 建立一個新的 tsm matcher group
func (m *Manager) newMatcherGroup() map[string]interface{} {
	return map[string]interface{}{
		"matcher": "",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": m.scriptPath,
			},
		},
	}
}

// EnsureScript 確保 scriptPath 上存在最新版的 hook script
// 如果目錄不存在會自動建立，如果檔案內容相同則跳過寫入
func (m *Manager) EnsureScript() error {
	dir := filepath.Dir(m.scriptPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("建立目錄 %s 失敗: %w", dir, err)
	}

	existing, err := os.ReadFile(m.scriptPath)
	if err == nil && string(existing) == string(HookScript) {
		return nil // 內容相同，跳過
	}

	if err := os.WriteFile(m.scriptPath, HookScript, 0755); err != nil {
		return fmt.Errorf("寫入 hook script 失敗: %w", err)
	}
	return nil
}

// Install 將 tsm hook 加入 Claude Code settings.json 的 6 個 event
func (m *Manager) Install(dryRun bool) (Result, error) {
	settings, err := m.readSettings()
	if err != nil {
		return Result{}, err
	}

	hooksMap := getOrCreateHooksMap(settings)
	var eventsAdded []string

	for _, event := range tsmEvents {
		var entries []interface{}
		if raw, ok := hooksMap[event]; ok {
			if arr, ok := raw.([]interface{}); ok {
				entries = arr
			}
		}

		if m.hasTSMHook(entries) {
			continue
		}

		entries = append(entries, m.newMatcherGroup())
		hooksMap[event] = entries
		eventsAdded = append(eventsAdded, event)
	}

	changed := len(eventsAdded) > 0

	if !changed {
		return Result{Changed: false}, nil
	}

	// 產生 NewSettings JSON
	newSettingsBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("序列化 settings 失敗: %w", err)
	}

	result := Result{
		Changed:     true,
		EventsAdded: eventsAdded,
		NewSettings: string(newSettingsBytes) + "\n",
	}

	if !dryRun {
		if err := m.writeSettings(settings); err != nil {
			return Result{}, err
		}
	}

	return result, nil
}

// Uninstall 從 Claude Code settings.json 移除 tsm hook
func (m *Manager) Uninstall(dryRun bool) (Result, error) {
	settings, err := m.readSettings()
	if err != nil {
		return Result{}, err
	}

	hooksMap := getOrCreateHooksMap(settings)
	var eventsRemoved []string

	for _, event := range tsmEvents {
		raw, ok := hooksMap[event]
		if !ok {
			continue
		}
		entries, ok := raw.([]interface{})
		if !ok {
			continue
		}

		var remaining []interface{}
		removed := false

		for _, entry := range entries {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				remaining = append(remaining, entry)
				continue
			}
			innerHooks, ok := entryMap["hooks"].([]interface{})
			if !ok {
				remaining = append(remaining, entry)
				continue
			}

			isTSM := false
			for _, h := range innerHooks {
				hookMap, ok := h.(map[string]interface{})
				if !ok {
					continue
				}
				if hookMap["command"] == m.scriptPath {
					isTSM = true
					break
				}
			}

			if isTSM {
				removed = true
			} else {
				remaining = append(remaining, entry)
			}
		}

		if removed {
			eventsRemoved = append(eventsRemoved, event)
			if len(remaining) == 0 {
				delete(hooksMap, event)
			} else {
				hooksMap[event] = remaining
			}
		}
	}

	changed := len(eventsRemoved) > 0

	if !changed {
		return Result{Changed: false}, nil
	}

	newSettingsBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("序列化 settings 失敗: %w", err)
	}

	result := Result{
		Changed:       true,
		EventsRemoved: eventsRemoved,
		NewSettings:   string(newSettingsBytes) + "\n",
	}

	if !dryRun {
		if err := m.writeSettings(settings); err != nil {
			return Result{}, err
		}
	}

	return result, nil
}
