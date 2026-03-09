package upgrade

import "fmt"

// PostUpgradeOps 封裝 post-quit 升級操作的依賴，便於測試。
type PostUpgradeOps struct {
	InstallBinary func(tmpPath string) (installedPath string, err error)
	RemoveTmp     func(path string)
	StopDaemon    func() error
	StartDaemon   func() error
	Exec          func(path string, args []string, env []string) error
}

// RunPostUpgrade 執行 TUI 退出後的升級流程：安裝 → 清理 → 重啟 daemon → exec。
func RunPostUpgrade(ops PostUpgradeOps, tmpPath string, args []string, env []string) error {
	installedPath, err := ops.InstallBinary(tmpPath)
	if err != nil {
		ops.RemoveTmp(tmpPath)
		return fmt.Errorf("安裝失敗: %w", err)
	}
	ops.RemoveTmp(tmpPath)

	// daemon stop 失敗不阻斷流程（可能本來就沒跑）
	_ = ops.StopDaemon()

	if err := ops.StartDaemon(); err != nil {
		return fmt.Errorf("啟動 daemon 失敗: %w", err)
	}

	if err := ops.Exec(installedPath, args, env); err != nil {
		return fmt.Errorf("exec 失敗（新版本已安裝於 %s）: %w", installedPath, err)
	}
	return nil
}
