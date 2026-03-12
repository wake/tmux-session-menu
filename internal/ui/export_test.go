package ui

// ExportDetectSessionStatus 為測試匯出的包裝函式。
func ExportDetectSessionStatus(deps Deps, sessionName, paneTitle, paneContent string) DetectSessionStatusResult {
	return detectSessionStatus(deps, sessionName, paneTitle, paneContent)
}
