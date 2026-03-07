package remote

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoff_FirstAttempt(t *testing.T) {
	d := Backoff(0, 30*time.Second)
	assert.Equal(t, 1*time.Second, d)
}

func TestBackoff_ExponentialGrowth(t *testing.T) {
	assert.Equal(t, 1*time.Second, Backoff(0, 30*time.Second))
	assert.Equal(t, 2*time.Second, Backoff(1, 30*time.Second))
	assert.Equal(t, 4*time.Second, Backoff(2, 30*time.Second))
	assert.Equal(t, 8*time.Second, Backoff(3, 30*time.Second))
	assert.Equal(t, 16*time.Second, Backoff(4, 30*time.Second))
}

func TestBackoff_CapsAtMax(t *testing.T) {
	assert.Equal(t, 30*time.Second, Backoff(5, 30*time.Second))
	assert.Equal(t, 30*time.Second, Backoff(10, 30*time.Second))
	assert.Equal(t, 30*time.Second, Backoff(100, 30*time.Second))
}

func TestReconnState_String(t *testing.T) {
	assert.Equal(t, "disconnected", StateDisconnected.String())
	assert.Equal(t, "connecting", StateConnecting.String())
	assert.Equal(t, "connected", StateConnected.String())
}
