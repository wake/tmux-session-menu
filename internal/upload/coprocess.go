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
	"github.com/wake/tmux-session-menu/internal/remote"
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

	// Local daemon 無 session 且非 upload mode → 嘗試 remote daemon。
	// 注意：fallback 後 c 仍指向 local daemon，ReportUploadResult 會送到 local daemon。
	// 這在 auto-upload 路徑影響有限（僅錯誤回報），upload mode 在此場景下不會觸發
	// （TUI 直接連 remote daemon 設定 upload mode）。
	if resp.SessionName == "" && !resp.UploadMode {
		if remoteResp := resolveRemoteTarget(ctx, cfg.Hosts, fileExists, dialAndGetTarget); remoteResp != nil {
			logger.Printf("fallback to remote: host=%s session=%s", remoteResp.SshTarget, remoteResp.SessionName)
			resp = remoteResp
		}
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

// fileExists 檢查檔案是否存在。
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// dialAndGetTarget 連到指定 socket 並取得上傳目標。
func dialAndGetTarget(ctx context.Context, sockPath string) (*tsmv1.GetUploadTargetResponse, error) {
	c, err := client.DialSocket(sockPath)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return c.GetUploadTarget(ctx)
}

// resolveRemoteTarget 在 local daemon 無 session 時，嘗試透過 tunnel socket 連到 remote daemon。
// 回傳 nil 代表沒有找到可用的 remote daemon。
func resolveRemoteTarget(
	ctx context.Context,
	hosts []config.HostEntry,
	socketExists func(string) bool,
	dialAndGet func(ctx context.Context, sockPath string) (*tsmv1.GetUploadTargetResponse, error),
) *tsmv1.GetUploadTargetResponse {
	for _, h := range hosts {
		if h.IsLocal() || !h.Enabled {
			continue
		}
		sockPath := remote.LocalSocketPath(h.Address)
		if !socketExists(sockPath) {
			continue
		}
		resp, err := dialAndGet(ctx, sockPath)
		if err != nil {
			continue
		}
		// 有 session 或 upload mode 才算有效
		if resp.SessionName == "" && !resp.UploadMode {
			continue
		}
		resp.IsRemote = true
		resp.SshTarget = h.Address
		return resp
	}
	return nil
}

// ReconstructFilenames 將被 shell word splitting 拆開的絕對路徑片段重組。
// iTerm2 fileDropCoprocess 的 \(filenames) 替換不會 shell-escape，
// 導致含空格的檔名被拆成多個 args。因為拖放路徑一定是絕對路徑（/ 開頭），
// 以 / 開頭的 arg 作為新檔案的起點，其餘片段用空格接回前一個檔案。
func ReconstructFilenames(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	var result []string
	var parts []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "/") && len(parts) > 0 {
			result = append(result, strings.Join(parts, " "))
			parts = nil
		}
		parts = append(parts, arg)
	}
	if len(parts) > 0 {
		result = append(result, strings.Join(parts, " "))
	}
	return result
}


// setupLogger 建立上傳日誌。
// 檔案不需要顯式關閉，因為 coprocess 是短命程序，OS 會在程序結束時回收。
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
