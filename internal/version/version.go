package version

import "fmt"

// 透過 ldflags 注入的版本資訊。
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String 回傳格式化的版本字串。
// 若有正式版本號則回傳 "0.5.1(abc1234)"，否則回傳 "dev"。
func String() string {
	if Version == "dev" {
		return "dev"
	}
	return fmt.Sprintf("%s(%s)", Version, Commit)
}
