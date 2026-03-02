package ui

// Mode 代表 TUI 的互動模式。
type Mode int

const (
	ModeNormal  Mode = iota // 一般瀏覽
	ModeInput               // 文字輸入
	ModeConfirm             // 確認對話框
	ModePicker              // 選擇器（例如選擇群組）
	ModeSearch              // 搜尋模式
)

// InputTarget 標識文字輸入的目的。
type InputTarget int

const (
	InputNewSession    InputTarget = iota // 新建 session
	InputRenameSession                    // 重命名 session (custom_name)
	InputNewGroup                         // 新建群組
)
