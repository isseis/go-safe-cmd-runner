//go:build test

package debug_test

import (
	"bytes"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/debug"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestPrintFromEnvInheritance_Inherit_WithGlobalAllowlist(t *testing.T) {
	t.Parallel()

	global := &runnertypes.GlobalSpec{
		EnvAllowed: []string{"VAR1", "VAR2"},
	}
	group := &runnertypes.GroupSpec{
		Name: "test-group",
	}
	runtimeGroup := &runnertypes.RuntimeGroup{
		Spec:                        group,
		EnvAllowlistInheritanceMode: runnertypes.InheritanceModeInherit,
	}

	var buf bytes.Buffer
	debug.PrintFromEnvInheritance(&buf, global, runtimeGroup)

	output := buf.String()
	assert.Contains(t, output, "Inheriting Global env_allowlist")
	assert.Contains(t, output, "Allowlist (2): VAR1, VAR2")
}

func TestPrintFromEnvInheritance_Inherit_EmptyGlobalAllowlist(t *testing.T) {
	t.Parallel()

	global := &runnertypes.GlobalSpec{}
	group := &runnertypes.GroupSpec{
		Name: "test-group",
	}
	runtimeGroup := &runnertypes.RuntimeGroup{
		Spec:                        group,
		EnvAllowlistInheritanceMode: runnertypes.InheritanceModeInherit,
	}

	var buf bytes.Buffer
	debug.PrintFromEnvInheritance(&buf, global, runtimeGroup)

	output := buf.String()
	assert.Contains(t, output, "Inheriting Global env_allowlist")
	assert.Contains(t, output, "(Global has no env_allowlist defined, so all variables allowed)")
}

func TestPrintFromEnvInheritance_Explicit(t *testing.T) {
	t.Parallel()

	global := &runnertypes.GlobalSpec{
		EnvAllowed: []string{"VAR1", "VAR2", "VAR3"},
	}
	group := &runnertypes.GroupSpec{
		Name:       "test-group",
		EnvAllowed: []string{"VAR1"},
	}
	runtimeGroup := &runnertypes.RuntimeGroup{
		Spec:                        group,
		EnvAllowlistInheritanceMode: runnertypes.InheritanceModeExplicit,
	}

	var buf bytes.Buffer
	debug.PrintFromEnvInheritance(&buf, global, runtimeGroup)

	output := buf.String()
	assert.Contains(t, output, "Using group-specific env_allowlist")
	assert.Contains(t, output, "Group allowlist (1): VAR1")
	assert.Contains(t, output, "Removed from Global allowlist: VAR2, VAR3")
}

func TestPrintFromEnvInheritance_Reject(t *testing.T) {
	t.Parallel()

	global := &runnertypes.GlobalSpec{
		EnvAllowed: []string{"VAR1", "VAR2"},
	}
	group := &runnertypes.GroupSpec{
		Name:       "test-group",
		EnvAllowed: []string{},
	}
	runtimeGroup := &runnertypes.RuntimeGroup{
		Spec:                        group,
		EnvAllowlistInheritanceMode: runnertypes.InheritanceModeReject,
	}

	var buf bytes.Buffer
	debug.PrintFromEnvInheritance(&buf, global, runtimeGroup)

	output := buf.String()
	assert.Contains(t, output, "Rejecting all environment variables")
	assert.Contains(t, output, "(Group has empty env_allowlist defined, blocking all env inheritance)")
}
