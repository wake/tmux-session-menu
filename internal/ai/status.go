package ai

import (
	"regexp"
	"strings"
)

var claudeModelPattern = regexp.MustCompile(`claude-(?:sonnet|opus|haiku)-[\w.-]+`)

func DetectModel(content string) string {
	return claudeModelPattern.FindString(content)
}

// HasStrongAiPresence 使用強信號檢查 AI agent 是否仍然存在。
// 不使用 > 提示符（會被一般 shell 誤觸）。
// 用於 hook status TTL 過期時的降級驗證。
func HasStrongAiPresence(content string) bool {
	if DetectModel(content) != "" {
		return true
	}
	return strings.Contains(content, "ctrl+c to interrupt") ||
		strings.Contains(content, "esc to interrupt")
}

func DetectTool(content string) string {
	claudeIndicators := []string{
		"ctrl+c to interrupt",
		"esc to interrupt",
	}
	for _, indicator := range claudeIndicators {
		if strings.Contains(content, indicator) {
			return "claude-code"
		}
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) > 0 {
		last := strings.TrimSpace(lines[len(lines)-1])
		if last == ">" || last == "\u2771" {
			return "claude-code"
		}
	}
	return ""
}
