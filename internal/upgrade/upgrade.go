package upgrade

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// Release 封裝 GitHub release 的版本與下載資訊。
type Release struct {
	Version string            // 版本號（不含 v 前綴）
	Assets  map[string]string // asset 名稱 → 下載 URL
}

// AssetName 回傳當前平台對應的 binary 檔名。
func AssetName() string {
	return fmt.Sprintf("tsm-%s-%s", runtime.GOOS, runtime.GOARCH)
}

// ParseRelease 從 GitHub releases API 的 JSON 回應中解析版本與 assets。
func ParseRelease(body []byte) (Release, error) {
	var raw struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Release{}, fmt.Errorf("parse release JSON: %w", err)
	}
	if raw.TagName == "" {
		return Release{}, fmt.Errorf("release missing tag_name")
	}

	ver := strings.TrimPrefix(raw.TagName, "v")
	assets := make(map[string]string, len(raw.Assets))
	for _, a := range raw.Assets {
		assets[a.Name] = a.URL
	}
	return Release{Version: ver, Assets: assets}, nil
}

// NeedsUpgrade 比較語意化版本，回傳是否需要升級。
// current 為 "dev" 時一律回傳 false。
func NeedsUpgrade(current, latest string) bool {
	if current == "dev" {
		return false
	}
	cur := parseVersion(current)
	lat := parseVersion(latest)
	if cur == nil || lat == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if lat[i] > cur[i] {
			return true
		}
		if lat[i] < cur[i] {
			return false
		}
	}
	return false
}

// parseVersion 將 "x.y.z" 解析為 [3]int，失敗回傳 nil。
func parseVersion(v string) []int {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	result := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		result[i] = n
	}
	return result
}
