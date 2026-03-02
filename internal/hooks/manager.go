package hooks

import (
	"encoding/json"
	"fmt"
	"os"
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

// writeSettings 將 settings 寫回 settings.json
func (m *Manager) writeSettings(settings map[string]interface{}) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 settings 失敗: %w", err)
	}
	if err := os.WriteFile(m.settingsPath, data, 0644); err != nil {
		return fmt.Errorf("寫入 settings.json 失敗: %w", err)
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

	// 產生 NewSettings JSON
	newSettingsBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("序列化 settings 失敗: %w", err)
	}

	result := Result{
		Changed:     changed,
		EventsAdded: eventsAdded,
		NewSettings: string(newSettingsBytes),
	}

	if !dryRun && changed {
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

	newSettingsBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("序列化 settings 失敗: %w", err)
	}

	result := Result{
		Changed:       changed,
		EventsRemoved: eventsRemoved,
		NewSettings:   string(newSettingsBytes),
	}

	if !dryRun && changed {
		if err := m.writeSettings(settings); err != nil {
			return Result{}, err
		}
	}

	return result, nil
}
