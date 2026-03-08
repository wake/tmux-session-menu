package config

// DefaultColors 是自動分配給新主機的色池。
var DefaultColors = []string{
	"#5B9BD5", "#70AD47", "#FFC000", "#FF6B6B",
	"#C678DD", "#56B6C2", "#E5C07B", "#98C379",
}

// MergeHosts 整合 config 中的主機清單與 --remote 參數。
//
// 當 remoteFlags 為 nil 或空：
//   - local host（IsLocal()==true）強制啟用
//   - 若清單中不存在 local host，自動在最前面補上
//   - 其他主機保持原設定
//
// 當 remoteFlags 有值：
//   - 旗標中的主機（以 Name 或 Address 比對）→ 啟用
//   - 旗標中但不在清單的主機 → 自動新增（enabled=true，自動分配顏色）
//   - 不在旗標中的主機 → 保持原設定
//   - local host → 停用
func MergeHosts(hosts []HostEntry, remoteFlags []string) []HostEntry {
	if len(remoteFlags) == 0 {
		return mergeNoRemote(hosts)
	}
	return mergeWithRemote(hosts, remoteFlags)
}

// mergeNoRemote 處理沒有 --remote 旗標的情況。
func mergeNoRemote(hosts []HostEntry) []HostEntry {
	result := make([]HostEntry, len(hosts))
	copy(result, hosts)

	hasLocal := false
	for i := range result {
		if result[i].IsLocal() {
			result[i].Enabled = true
			hasLocal = true
		}
	}

	// 使用者 config 中沒有 local host 時，自動補上在最前面
	if !hasLocal {
		local := HostEntry{
			Name:      "local",
			Address:   "",
			Color:     "#5f8787",
			Enabled:   true,
			SortOrder: 0,
		}
		result = append([]HostEntry{local}, result...)
	}

	return result
}

// mergeWithRemote 處理有 --remote 旗標的情況。
func mergeWithRemote(hosts []HostEntry, remoteFlags []string) []HostEntry {
	// 建立旗標查詢集合
	flagSet := make(map[string]bool, len(remoteFlags))
	for _, f := range remoteFlags {
		flagSet[f] = true
	}

	// 收集已使用的顏色，用於自動分配時避免重複
	usedColors := make(map[string]bool)
	for _, h := range hosts {
		if h.Color != "" {
			usedColors[h.Color] = true
		}
	}

	result := make([]HostEntry, len(hosts))
	copy(result, hosts)

	// 記錄哪些旗標值已被既有主機匹配
	matched := make(map[string]bool, len(remoteFlags))

	for i := range result {
		if result[i].IsLocal() {
			// local host 停用
			result[i].Enabled = false
			continue
		}

		// 比對 Name 或 Address
		if flagSet[result[i].Name] {
			result[i].Enabled = true
			matched[result[i].Name] = true
		}
		if flagSet[result[i].Address] {
			result[i].Enabled = true
			matched[result[i].Address] = true
		}
	}

	// 處理旗標中未匹配到的 → 新增主機
	for _, f := range remoteFlags {
		if matched[f] {
			continue
		}

		newHost := HostEntry{
			Name:      f,
			Address:   f,
			Color:     pickColor(usedColors),
			Enabled:   true,
			SortOrder: len(result),
		}
		usedColors[newHost.Color] = true
		result = append(result, newHost)
	}

	return result
}

// pickColor 從 DefaultColors 色池中選取第一個未使用的顏色。
// 若色池耗盡，回傳第一個顏色（循環使用）。
func pickColor(used map[string]bool) string {
	for _, c := range DefaultColors {
		if !used[c] {
			return c
		}
	}
	// 色池耗盡，從頭開始
	if len(DefaultColors) > 0 {
		return DefaultColors[0]
	}
	return "#888888"
}
