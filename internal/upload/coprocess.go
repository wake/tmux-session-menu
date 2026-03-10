package upload

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
)

// Action 代表 coprocess 的決策結果。
type Action int

const (
	ActionPassthrough Action = iota // 輸出本地路徑
	ActionAutoUpload                // 自動上傳 + bracketed paste
	ActionUploadMode                // 上傳模式 + 回報 TUI
)

// DecideAction 根據 daemon 回傳的上傳目標判斷動作。
func DecideAction(resp *tsmv1.GetUploadTargetResponse) Action {
	if resp.UploadMode {
		return ActionUploadMode
	}
	if resp.IsRemote && resp.IsClaudeActive {
		return ActionAutoUpload
	}
	return ActionPassthrough
}

// RunCoprocess 是 tsm iterm-coprocess 子命令的主邏輯。
func RunCoprocess(filenames []string) {
	logger := setupLogger()

	cfg := config.Default()
	cfgPath := config.ExpandPath("~/.config/tsm/config.toml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		if loaded, err := config.LoadFromString(string(data)); err == nil {
			cfg = loaded
		}
	}

	c, err := client.Dial(cfg)
	if err != nil {
		logger.Printf("daemon 連線失敗: %v", err)
		fmt.Print(FormatLocalPaths(filenames))
		return
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.GetUploadTarget(ctx)
	if err != nil {
		logger.Printf("GetUploadTarget 失敗: %v", err)
		fmt.Print(FormatLocalPaths(filenames))
		return
	}

	action := DecideAction(resp)
	logger.Printf("action=%d session=%s remote=%v claude=%v upload_mode=%v",
		action, resp.SessionName, resp.IsRemote, resp.IsClaudeActive, resp.UploadMode)

	switch action {
	case ActionUploadMode:
		handleUploadMode(ctx, c, resp, filenames, logger)
	case ActionAutoUpload:
		handleAutoUpload(ctx, c, resp, filenames, logger)
	default:
		fmt.Print(FormatLocalPaths(filenames))
	}
}

func handleUploadMode(ctx context.Context, c *client.Client, resp *tsmv1.GetUploadTargetResponse, files []string, logger *log.Logger) {
	var results []*tsmv1.UploadedFile
	var uploadErr string

	if resp.IsRemote && resp.SshTarget != "" {
		r, err := ScpUpload(resp.SshTarget, int(resp.SshPort), resp.UploadPath, files)
		if err != nil {
			uploadErr = err.Error()
		}
		results = r
	} else {
		r, err := CopyLocal(files, resp.UploadPath)
		if err != nil {
			uploadErr = err.Error()
		}
		results = r
	}

	if err := c.ReportUploadResult(ctx, resp.SessionName, results, uploadErr); err != nil {
		logger.Printf("ReportUploadResult 失敗: %v", err)
	}
}

func handleAutoUpload(ctx context.Context, c *client.Client, resp *tsmv1.GetUploadTargetResponse, files []string, logger *log.Logger) {
	results, err := ScpUpload(resp.SshTarget, int(resp.SshPort), resp.UploadPath, files)
	if err != nil {
		logger.Printf("自動上傳失敗: %v", err)
		_ = c.ReportUploadResult(ctx, resp.SessionName, nil, err.Error())
		fmt.Print(FormatLocalPaths(files))
		return
	}

	remotePaths := make([]string, len(results))
	for i, r := range results {
		remotePaths[i] = r.RemotePath
	}
	fmt.Print(FormatBracketedPaste(remotePaths))
}

// ParseFilenames 解析 iTerm2 傳入的 filenames 字串。
func ParseFilenames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var files []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case !inQuote && (ch == '\'' || ch == '"'):
			inQuote = true
			quoteChar = ch
		case inQuote && ch == quoteChar:
			inQuote = false
		case !inQuote && ch == '\\' && i+1 < len(raw):
			i++
			current.WriteByte(raw[i])
		case !inQuote && ch == ' ':
			if current.Len() > 0 {
				files = append(files, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		files = append(files, current.String())
	}
	return files
}

func setupLogger() *log.Logger {
	logDir := config.ExpandPath("~/.config/tsm/logs")
	_ = os.MkdirAll(logDir, 0755)
	f, err := os.OpenFile(
		logDir+"/upload.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		return log.New(os.Stderr, "[tsm-upload] ", log.LstdFlags)
	}
	return log.New(f, "", log.LstdFlags)
}
