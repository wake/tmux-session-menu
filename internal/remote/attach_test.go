package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyExit_Zero_IsDetach(t *testing.T) {
	assert.Equal(t, AttachDetached, classifyExit(0))
}

func TestClassifyExit_NonZero_IsDisconnect(t *testing.T) {
	assert.Equal(t, AttachDisconnected, classifyExit(1))
	assert.Equal(t, AttachDisconnected, classifyExit(255))
	assert.Equal(t, AttachDisconnected, classifyExit(-1))
}
