package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ColorConfig 是本地或遠端模式的顏色設定。
type ColorConfig struct {
	BarBG   string `toml:"bar_bg"`   // 狀態列背景色（空字串=保持 tmux 預設）
	BadgeBG string `toml:"badge_bg"` // 徽章背景色
	BadgeFG string `toml:"badge_fg"` // 徽章前景色
}

// HostEntry 代表一台主機的設定。
type HostEntry struct {
	Name      string `toml:"name"`
	Address   string `toml:"address"` // 空字串 = 本機
	Color     string `toml:"color"`   // accent color
	Enabled   bool   `toml:"enabled"`
	SortOrder int    `toml:"sort_order"`
}

// IsLocal 回傳此主機是否為本機（Address 為空）。
func (h HostEntry) IsLocal() bool {
	return h.Address == ""
}

// AgentEntry 代表一個 agent 預設。
type AgentEntry struct {
	Name      string `toml:"name"`
	Command   string `toml:"command"`
	Group     string `toml:"group"` // 群組標題（選填）
	Enabled   bool   `toml:"enabled"`
	SortOrder int    `toml:"sort_order"`
}

// UploadConfig 是拖曳上傳的設定。
type UploadConfig struct {
	RemotePath string `toml:"remote_path"` // 自動模式遠端上傳目錄
}

// Config 是 TSM 的全域設定。
type Config struct {
	DataDir         string `toml:"data_dir"`
	PreviewLines    int    `toml:"preview_lines"`
	PollIntervalSec int    `toml:"poll_interval_sec"`
	InTmux          bool   `toml:"-"` // 執行時偵測，不寫入設定檔
	InPopup         bool   `toml:"-"` // popup 模式，不寫入設定檔

	Local  ColorConfig `toml:"local"`  // 本地模式顏色
	Remote ColorConfig `toml:"remote"` // 遠端模式顏色

	Hosts  []HostEntry  `toml:"hosts"`  // 主機清單
	Agents []AgentEntry `toml:"agents"` // Agent 預設清單
	Upload UploadConfig `toml:"upload"` // 拖曳上傳設定
}

// Default 回傳預設設定。
func Default() Config {
	return Config{
		DataDir:         "~/.config/tsm",
		PreviewLines:    150,
		PollIntervalSec: 2,
		Local: ColorConfig{
			BarBG:   "", // 空值=保持 tmux 預設
			BadgeBG: "#5f8787",
			BadgeFG: "#c0caf5",
		},
		Remote: ColorConfig{
			BarBG:   "#1a2b2b",
			BadgeBG: "#73daca",
			BadgeFG: "#1a1b26",
		},
		Hosts: []HostEntry{
			{Name: "local", Address: "", Color: "#5f8787", Enabled: true, SortOrder: 0},
		},
		Upload: UploadConfig{
			RemotePath: "/tmp/iterm-upload",
		},
	}
}

// LoadFromString 從 TOML 字串載入設定，未指定欄位使用預設值。
func LoadFromString(data string) (Config, error) {
	cfg := Default()
	if _, err := toml.Decode(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// InstallMode 代表安裝模式。
type InstallMode string

const (
	ModeFull   InstallMode = "full"
	ModeClient InstallMode = "client"
)

const installModeFile = "install_mode"

// SaveInstallMode 將安裝模式寫入指定目錄。
func SaveInstallMode(dir string, mode InstallMode) error {
	return os.WriteFile(filepath.Join(dir, installModeFile), []byte(string(mode)), 0644)
}

// LoadInstallMode 從指定目錄讀取安裝模式，檔案不存在時預設為 full。
func LoadInstallMode(dir string) InstallMode {
	data, err := os.ReadFile(filepath.Join(dir, installModeFile))
	if err != nil {
		return ModeFull
	}
	mode := InstallMode(strings.TrimSpace(string(data)))
	if mode == ModeClient {
		return ModeClient
	}
	return ModeFull
}

const lastRemoteHostFile = "last_remote_host"

// SaveLastRemoteHost 儲存最後連線的遠端主機名。
func SaveLastRemoteHost(dir, host string) error {
	return os.WriteFile(filepath.Join(dir, lastRemoteHostFile), []byte(host), 0644)
}

// LoadLastRemoteHost 讀取最後連線的遠端主機名，不存在時回傳空字串。
func LoadLastRemoteHost(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, lastRemoteHostFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SaveConfig 將 Config 寫入指定路徑的 TOML 檔案。
// 標記 toml:"-" 的執行時欄位（InTmux、InPopup）不會被寫入。
func SaveConfig(path string, cfg Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

// ExpandPath 將 ~ 展開為使用者家目錄。
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home + path[1:]
	}
	return path
}
