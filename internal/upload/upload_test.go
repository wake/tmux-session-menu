package upload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyLocal(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	dstDir := t.TempDir()

	results, err := CopyLocal([]string{srcFile}, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].RemotePath != filepath.Join(dstDir, "test.txt") {
		t.Errorf("got remote_path %q", results[0].RemotePath)
	}
	if results[0].SizeBytes != 5 {
		t.Errorf("got size %d, want 5", results[0].SizeBytes)
	}

	data, err := os.ReadFile(filepath.Join(dstDir, "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want hello", string(data))
	}
}

func TestCopyLocal_MultipleFiles(t *testing.T) {
	srcDir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	dstDir := t.TempDir()
	results, err := CopyLocal([]string{
		filepath.Join(srcDir, "a.txt"),
		filepath.Join(srcDir, "b.txt"),
	}, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestCopyLocal_CreatesDstDir(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	dstDir := filepath.Join(t.TempDir(), "sub", "dir")
	_, err := CopyLocal([]string{srcFile}, dstDir)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFormatBracketedPaste(t *testing.T) {
	paths := []string{"/data/a.png", "/data/b.pdf"}
	got := FormatBracketedPaste(paths)
	want := "\x1b[200~/data/a.png /data/b.pdf\x1b[201~"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLocalPaths(t *testing.T) {
	paths := []string{"/Users/wake/a.png", "/Users/wake/my file.pdf"}
	got := FormatLocalPaths(paths)
	want := `/Users/wake/a.png /Users/wake/my\ file.pdf`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLocalPaths_Single(t *testing.T) {
	got := FormatLocalPaths([]string{"/tmp/test.png"})
	if got != "/tmp/test.png" {
		t.Errorf("got %q", got)
	}
}
