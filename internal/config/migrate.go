package config

// MigrateRemoteToHosts 將舊版 [remote] 段落的顏色遷移到每台主機的獨立設定。
//
// 遷移規則：
//  1. 若 [remote] 有任何顏色值（BarBG、BadgeBG 或 BadgeFG 非空），視為需要遷移。
//  2. 對每台主機：
//     - Local 主機（Address == ""）：從 cfg.Local 同步四色。
//     - 遠端主機且四色全空且 [remote] 有值：繼承 [remote] 的顏色。
//     - 若 BadgeBG 仍為空但 Color 有值：fallback 到 host.Color。
//  3. 遷移完成後清空 cfg.Remote。
func MigrateRemoteToHosts(cfg *Config) {
	// 判斷 [remote] 是否有需要遷移的值
	var hasRemote bool
	if cfg.Remote != nil {
		hasRemote = cfg.Remote.BarBG != "" || cfg.Remote.BadgeBG != "" || cfg.Remote.BadgeFG != ""
	}

	for i := range cfg.Hosts {
		h := &cfg.Hosts[i]

		if h.IsLocal() {
			// local 主機：從 cfg.Local 同步四色
			h.BarBG = cfg.Local.BarBG
			h.BarFG = cfg.Local.BarFG
			h.BadgeBG = cfg.Local.BadgeBG
			h.BadgeFG = cfg.Local.BadgeFG
			continue
		}

		// 遠端主機：四色全空且 [remote] 有值時，繼承 [remote]
		allEmpty := h.BarBG == "" && h.BarFG == "" && h.BadgeBG == "" && h.BadgeFG == ""
		if allEmpty && hasRemote {
			h.BarBG = cfg.Remote.BarBG
			h.BarFG = cfg.Remote.BarFG
			h.BadgeBG = cfg.Remote.BadgeBG
			h.BadgeFG = cfg.Remote.BadgeFG
		}

		// BadgeBG 仍為空但 Color 有值時，fallback 到 host.Color
		if h.BadgeBG == "" && h.Color != "" {
			h.BadgeBG = h.Color
		}
	}

	// 清空 [remote]，遷移完成；nil 使 SaveConfig 不再寫出 [remote] 區段
	cfg.Remote = nil
}

// SyncLocalHostToConfig 將 local 主機的四色設定寫回 Config.Local。
//
// 找到第一個 Address == "" 的主機，將其 BarBG/BarFG/BadgeBG/BadgeFG
// 同步回 cfg.Local，確保兩者保持一致。
// 若主機清單中沒有 local 主機，則不做任何變更。
func SyncLocalHostToConfig(cfg *Config) {
	for _, h := range cfg.Hosts {
		if h.IsLocal() {
			cfg.Local.BarBG = h.BarBG
			cfg.Local.BarFG = h.BarFG
			cfg.Local.BadgeBG = h.BadgeBG
			cfg.Local.BadgeFG = h.BadgeFG
			return
		}
	}
}
