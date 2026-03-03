package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringDefault(t *testing.T) {
	// 預設值（未透過 ldflags 注入）應為 "dev"
	Version = "dev"
	Commit = "unknown"
	assert.Equal(t, "dev", String())
}

func TestStringWithVersion(t *testing.T) {
	Version = "0.5.1"
	Commit = "abc1234"
	defer func() {
		Version = "dev"
		Commit = "unknown"
	}()

	assert.Equal(t, "0.5.1(abc1234)", String())
}
