package config

import (
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config 是 TSM 的全域設定。
type Config struct {
	DataDir         string `toml:"data_dir"`
	PreviewLines    int    `toml:"preview_lines"`
	PollIntervalSec int    `toml:"poll_interval_sec"`
	InTmux          bool   `toml:"-"` // 執行時偵測，不寫入設定檔
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
