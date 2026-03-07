package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	osexec "os/exec"
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

// releaseURL 為 GitHub releases API 的 URL。
const releaseURL = "https://api.github.com/repos/wake/tmux-session-menu/releases/latest"

// Upgrader 封裝升級操作的依賴（HTTP 下載 + 執行程式）。
type Upgrader struct {
	HTTPGet  func(url string) ([]byte, error)            // 下載檔案內容
	ExecFunc func(binary string, args ...string) error    // 執行外部程式
}

// DefaultUpgrader 使用標準 HTTP 和 os/exec。
func DefaultUpgrader() *Upgrader {
	return &Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			resp, err := http.Get(url)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
			}
			return io.ReadAll(resp.Body)
		},
		ExecFunc: func(binary string, args ...string) error {
			cmd := osexec.Command(binary, args...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		},
	}
}

// CheckLatest 從 GitHub 取得最新 release 資訊。
func (u *Upgrader) CheckLatest() (Release, error) {
	body, err := u.HTTPGet(releaseURL)
	if err != nil {
		return Release{}, fmt.Errorf("fetch latest release: %w", err)
	}
	return ParseRelease(body)
}

// Download 下載指定 URL 到暫存目錄並設為可執行。回傳暫存檔路徑。
func (u *Upgrader) Download(url string) (string, error) {
	data, err := u.HTTPGet(url)
	if err != nil {
		return "", fmt.Errorf("download binary: %w", err)
	}

	f, err := os.CreateTemp("", "tsm-upgrade-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	path := f.Name()

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("write binary: %w", err)
	}
	f.Close()

	if err := os.Chmod(path, 0o755); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("chmod: %w", err)
	}
	return path, nil
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
