package config

// DefaultColors 是自動分配給新主機的色池。
var DefaultColors = []string{
	"#5B9BD5", "#70AD47", "#FFC000", "#FF6B6B",
	"#C678DD", "#56B6C2", "#E5C07B", "#98C379",
}

// EnsureLocal 確保主機清單中包含 local 主機 entry。
// 若不存在則自動補上在最前面（Enabled=true）。
// 注意：若 local 已存在但 Enabled=false，本函式不會修改其 Enabled 狀態。
func EnsureLocal(hosts []HostEntry) []HostEntry {
	for _, h := range hosts {
		if h.IsLocal() {
			return hosts
		}
	}
	local := HostEntry{
		Name:      "local",
		Address:   "",
		Color:     "#5f8787",
		Enabled:   true,
		SortOrder: 0,
	}
	return append([]HostEntry{local}, hosts...)
}

// MergeHosts 整合 config 中的主機清單與 --host / --local 參數。
//
// hostFlags=[], localFlag=false → 不修改，直接回傳副本
// hostFlags=[], localFlag=true  → 啟用 local（不存在則自動補上），停用其餘
// hostFlags 有值, localFlag=false → 啟用匹配的主機，停用 local 及未匹配的主機
// hostFlags 有值, localFlag=true  → 啟用 local + 匹配的主機，停用未匹配的主機
func MergeHosts(hosts []HostEntry, hostFlags []string, localFlag bool) []HostEntry {
	if len(hostFlags) == 0 && !localFlag {
		// 無旗標：直接回傳副本，不修改
		result := make([]HostEntry, len(hosts))
		copy(result, hosts)
		return result
	}
	if len(hostFlags) == 0 && localFlag {
		return mergeLocalOnly(hosts)
	}
	return mergeWithFlags(hosts, hostFlags, localFlag)
}

// mergeLocalOnly 處理僅有 --local 旗標的情況。
// 啟用 local host（不存在則自動補上），停用所有其他主機。
func mergeLocalOnly(hosts []HostEntry) []HostEntry {
	result := make([]HostEntry, len(hosts))
	copy(result, hosts)

	hasLocal := false
	for i := range result {
		if result[i].IsLocal() {
			result[i].Enabled = true
			hasLocal = true
		} else {
			result[i].Enabled = false
		}
	}

	// config 中沒有 local host 時，自動補上在最前面
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

// mergeWithFlags 處理有 hostFlags 的情況（localFlag 可為 true 或 false）。
// localFlag=false：停用 local + 未匹配的主機，啟用匹配的主機
// localFlag=true：啟用 local + 匹配的主機，停用未匹配的主機
func mergeWithFlags(hosts []HostEntry, hostFlags []string, localFlag bool) []HostEntry {
	// 建立旗標查詢集合
	flagSet := make(map[string]bool, len(hostFlags))
	for _, f := range hostFlags {
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
	matched := make(map[string]bool, len(hostFlags))

	for i := range result {
		if result[i].IsLocal() {
			// local host 依 localFlag 決定啟停
			result[i].Enabled = localFlag
			continue
		}

		// 封存主機不受旗標影響，維持停用狀態
		if result[i].Archived {
			result[i].Enabled = false
			// 仍標記為已匹配，避免被當作未知主機重複新增
			if flagSet[result[i].Name] {
				matched[result[i].Name] = true
			}
			if flagSet[result[i].Address] {
				matched[result[i].Address] = true
			}
			continue
		}

		// 比對 Name 或 Address
		nameMatch := flagSet[result[i].Name]
		addrMatch := flagSet[result[i].Address]

		if nameMatch || addrMatch {
			result[i].Enabled = true
			if nameMatch {
				matched[result[i].Name] = true
			}
			if addrMatch {
				matched[result[i].Address] = true
			}
		} else {
			// 不在旗標中的主機 → 停用
			result[i].Enabled = false
		}
	}

	// 處理旗標中未匹配到的 → 新增主機
	for _, f := range hostFlags {
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

// FindArchivedHost 在主機清單中尋找指定名稱且已封存的主機。
// 找到時回傳 (true, 索引)；未找到或主機未封存時回傳 (false, -1)。
func FindArchivedHost(hosts []HostEntry, name string) (bool, int) {
	for i, h := range hosts {
		if h.Name == name && h.Archived {
			return true, i
		}
	}
	return false, -1
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
