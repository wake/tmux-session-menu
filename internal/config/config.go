package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config 是 TSM 的全域設定。
type Config struct {
	DataDir         string `toml:"data_dir"`
	PreviewLines    int    `toml:"preview_lines"`
	PollIntervalSec int    `toml:"poll_interval_sec"`
	InTmux          bool   `toml:"-"` // 執行時偵測，不寫入設定檔
	InPopup         bool   `toml:"-"` // popup 模式，不寫入設定檔
}

// Default 回傳預設設定。
func Default() Config {
	return Config{
		DataDir:         "~/.config/tsm",
		PreviewLines:    150,
		PollIntervalSec: 2,
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
