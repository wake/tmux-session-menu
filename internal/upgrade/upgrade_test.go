package upgrade

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetName(t *testing.T) {
	name := AssetName()
	assert.Equal(t, "tsm-"+runtime.GOOS+"-"+runtime.GOARCH, name)
}

func TestParseRelease_Valid(t *testing.T) {
	body := []byte(`{
		"tag_name": "v0.15.0",
		"assets": [
			{"name": "tsm-darwin-arm64", "browser_download_url": "https://example.com/tsm-darwin-arm64"},
			{"name": "tsm-linux-amd64", "browser_download_url": "https://example.com/tsm-linux-amd64"}
		]
	}`)

	rel, err := ParseRelease(body)
	require.NoError(t, err)
	assert.Equal(t, "0.15.0", rel.Version)
	assert.Equal(t, "https://example.com/tsm-darwin-arm64", rel.Assets["tsm-darwin-arm64"])
	assert.Equal(t, "https://example.com/tsm-linux-amd64", rel.Assets["tsm-linux-amd64"])
}

func TestParseRelease_InvalidJSON(t *testing.T) {
	_, err := ParseRelease([]byte(`not json`))
	assert.Error(t, err)
}

func TestParseRelease_NoTag(t *testing.T) {
	_, err := ParseRelease([]byte(`{"assets": []}`))
	assert.Error(t, err)
}

func TestParseRelease_StripV(t *testing.T) {
	body := []byte(`{"tag_name": "v1.2.3", "assets": []}`)
	rel, err := ParseRelease(body)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", rel.Version)
}

func TestNeedsUpgrade_NewerAvailable(t *testing.T) {
	assert.True(t, NeedsUpgrade("0.14.0", "0.15.0"))
}

func TestNeedsUpgrade_SameVersion(t *testing.T) {
	assert.False(t, NeedsUpgrade("0.14.0", "0.14.0"))
}

func TestNeedsUpgrade_OlderAvailable(t *testing.T) {
	assert.False(t, NeedsUpgrade("0.15.0", "0.14.0"))
}

func TestNeedsUpgrade_MajorBump(t *testing.T) {
	assert.True(t, NeedsUpgrade("0.14.0", "1.0.0"))
}

func TestNeedsUpgrade_PatchBump(t *testing.T) {
	assert.True(t, NeedsUpgrade("0.14.0", "0.14.1"))
}

func TestNeedsUpgrade_DevVersion(t *testing.T) {
	// "dev" 版本（未注入版本號）一律不升級
	assert.False(t, NeedsUpgrade("dev", "0.15.0"))
}

func TestUpgrader_CheckLatest_Success(t *testing.T) {
	u := &Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			assert.Contains(t, url, "api.github.com")
			return []byte(`{"tag_name": "v1.0.0", "assets": [{"name": "tsm-linux-amd64", "browser_download_url": "https://example.com/dl"}]}`), nil
		},
	}
	rel, err := u.CheckLatest()
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", rel.Version)
}

func TestUpgrader_CheckLatest_NetworkError(t *testing.T) {
	u := &Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			return nil, fmt.Errorf("network error")
		},
	}
	_, err := u.CheckLatest()
	assert.Error(t, err)
}

func TestUpgrader_Download_Success(t *testing.T) {
	content := []byte("#!/bin/sh\necho hello")
	u := &Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			return content, nil
		},
	}
	path, err := u.Download("https://example.com/tsm-linux-amd64")
	require.NoError(t, err)
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, data)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.Mode()&0o111 != 0, "should be executable")
}

func TestUpgrader_Download_NetworkError(t *testing.T) {
	u := &Upgrader{
		HTTPGet: func(url string) ([]byte, error) {
			return nil, fmt.Errorf("download failed")
		},
	}
	_, err := u.Download("https://example.com/bad")
	assert.Error(t, err)
}
