package upgrade

import "fmt"

// SilentOps 封裝 silent 升級的可注入依賴。
type SilentOps struct {
	CurrentVersion string
	CheckLatest    func() (Release, error)
	Download       func(url string) (string, error)
	InstallBinary  func(tmpPath string) (installedPath string, err error)
	RemoveTmp      func(path string)
	StopDaemon     func() error
	StartDaemon    func() error
}

// SilentResult 封裝 silent 升級的結果。
type SilentResult struct {
	Version  string // 最終版本號
	Upgraded bool   // 是否有執行升級
}

// RunSilent 執行非互動升級。
// 已是最新 → 回傳當前版本 + Upgraded=false。
// 需升級 → 下載 → 安裝 → 重啟 daemon → 回傳新版本 + Upgraded=true。
func RunSilent(ops SilentOps) (SilentResult, error) {
	rel, err := ops.CheckLatest()
	if err != nil {
		return SilentResult{}, fmt.Errorf("檢查最新版本失敗: %w", err)
	}

	if !NeedsUpgrade(ops.CurrentVersion, rel.Version) {
		return SilentResult{Version: ops.CurrentVersion, Upgraded: false}, nil
	}

	asset := AssetName()
	url, ok := rel.Assets[asset]
	if !ok {
		return SilentResult{}, fmt.Errorf("找不到適用於此平台的檔案 (%s)", asset)
	}

	tmpPath, err := ops.Download(url)
	if err != nil {
		return SilentResult{}, fmt.Errorf("下載失敗: %w", err)
	}

	_, err = ops.InstallBinary(tmpPath)
	if err != nil {
		ops.RemoveTmp(tmpPath)
		return SilentResult{}, fmt.Errorf("安裝失敗: %w", err)
	}
	ops.RemoveTmp(tmpPath)

	_ = ops.StopDaemon()

	if err := ops.StartDaemon(); err != nil {
		return SilentResult{}, fmt.Errorf("啟動 daemon 失敗: %w", err)
	}

	return SilentResult{Version: rel.Version, Upgraded: true}, nil
}
