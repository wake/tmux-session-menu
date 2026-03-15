package ai

import (
	"regexp"
	"strings"
)

// claudeModelPattern 匹配 API model ID 格式（如 claude-opus-4-6）。
var claudeModelPattern = regexp.MustCompile(`claude-(?:sonnet|opus|haiku)-[\w.-]+`)

// claudeStatusPattern 匹配 Claude Code status bar 的顯示格式（如 [Opus 4.6]）。
var claudeStatusPattern = regexp.MustCompile(`\[(?:Opus|Sonnet|Haiku) \d+(?:\.\d+)*\]`)

func DetectModel(content string) string {
	return claudeModelPattern.FindString(content)
}

// HasStrongAiPresence 使用強信號檢查 AI agent 是否仍然存在。
// 用於 hook status TTL 過期時的降級驗證。
func HasStrongAiPresence(content string) bool {
	if DetectModel(content) != "" {
		return true
	}
	if claudeStatusPattern.MatchString(content) {
		return true
	}
	if strings.Contains(content, "ctrl+c to interrupt") ||
		strings.Contains(content, "esc to interrupt") {
		return true
	}
	// Claude Code 分隔線（────────）後接 ❯ prompt
	return hasSeparatorPrompt(content)
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
	// Claude Code status bar 顯示格式（如 [Opus 4.6]）
	if claudeStatusPattern.MatchString(content) {
		return "claude-code"
	}
	// Claude Code 分隔線（────────）後接 ❯ prompt
	if hasSeparatorPrompt(content) {
		return "claude-code"
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

// hasSeparatorPrompt 檢查是否有 Claude Code 的分隔線 + prompt 模式。
// 模式：一行全為 ─（U+2500）字元（至少 8 個），後接包含 ❯ 或 > 的行。
func hasSeparatorPrompt(content string) bool {
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines)-1; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if len(trimmed) >= 8 && isSeparatorLine(trimmed) {
			// 往下找 prompt（可能隔一行空行）
			for j := i + 1; j < len(lines) && j <= i+2; j++ {
				next := strings.TrimSpace(lines[j])
				if next == "❯" || next == ">" || next == "\u2771" {
					return true
				}
				if next != "" {
					break // 非空非 prompt → 不是目標模式
				}
			}
		}
	}
	return false
}

// isSeparatorLine 檢查字串是否全由 ─（U+2500）組成。
func isSeparatorLine(s string) bool {
	for _, r := range s {
		if r != '─' { // U+2500 Box Drawings Light Horizontal
			return false
		}
	}
	return true
}
