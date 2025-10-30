package environment

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/require"
)

func TestNewFilter(t *testing.T) {
	config := &runnertypes.ConfigSpec{}
	filter := NewFilter(config.Global.EnvAllowed)

	require.NotNil(t, filter, "NewFilter returned nil")
}
