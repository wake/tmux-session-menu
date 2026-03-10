package upload

import (
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

// CopyLocal 複製檔案到本機目標目錄。
func CopyLocal(files []string, dstDir string) ([]*tsmv1.UploadedFile, error) {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dstDir, err)
	}

	var results []*tsmv1.UploadedFile
	for _, src := range files {
		dst := filepath.Join(dstDir, filepath.Base(src))
		size, err := copyFile(src, dst)
		if err != nil {
			return results, fmt.Errorf("copy %s: %w", src, err)
		}
		results = append(results, &tsmv1.UploadedFile{
			LocalPath:  src,
			RemotePath: dst,
			SizeBytes:  size,
		})
	}
	return results, nil
}

// ScpUpload 透過 scp 上傳檔案到遠端。
func ScpUpload(sshTarget string, sshPort int, dstDir string, files []string) ([]*tsmv1.UploadedFile, error) {
	mkdirArgs := []string{sshTarget, "mkdir", "-p", dstDir}
	if sshPort != 0 && sshPort != 22 {
		mkdirArgs = append([]string{"-p", fmt.Sprintf("%d", sshPort)}, mkdirArgs...)
	}
	if out, err := osexec.Command("ssh", mkdirArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ssh mkdir: %s: %w", string(out), err)
	}

	scpArgs := []string{}
	if sshPort != 0 && sshPort != 22 {
		scpArgs = append(scpArgs, "-P", fmt.Sprintf("%d", sshPort))
	}
	scpArgs = append(scpArgs, files...)
	scpArgs = append(scpArgs, fmt.Sprintf("%s:%s/", sshTarget, dstDir))

	if out, err := osexec.Command("scp", scpArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("scp: %s: %w", string(out), err)
	}

	var results []*tsmv1.UploadedFile
	for _, src := range files {
		info, _ := os.Stat(src)
		var size int64
		if info != nil {
			size = info.Size()
		}
		results = append(results, &tsmv1.UploadedFile{
			LocalPath:  src,
			RemotePath: filepath.Join(dstDir, filepath.Base(src)),
			SizeBytes:  size,
		})
	}
	return results, nil
}

// FormatBracketedPaste 格式化路徑為 bracketed paste 序列。
func FormatBracketedPaste(paths []string) string {
	return "\x1b[200~" + strings.Join(paths, " ") + "\x1b[201~"
}

// FormatLocalPaths 格式化本地路徑，空格需轉義。
func FormatLocalPaths(paths []string) string {
	escaped := make([]string, len(paths))
	for i, p := range paths {
		escaped[i] = strings.ReplaceAll(p, " ", "\\ ")
	}
	return strings.Join(escaped, " ")
}

func copyFile(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	n, err := io.Copy(out, in)
	return n, err
}
