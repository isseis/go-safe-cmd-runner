//go:build test

// Package runner contains integration tests for the F-001 (identity hash
// re-verification) and F-004 (dependency resolution re-execution) TOCTOU
// fixes, exercised end-to-end through a real GroupExecutor/Runner rather than
// the mocked VerificationManager used by group_executor_test.go.
//
// Run with: go test -tags test -v ./internal/runner -run TestGroupExecutor_F0

package runner

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGroupExecutor_F001_HashMismatchBlocksExecution reproduces the exact
// TOCTOU window F-001 closes. ExecuteGroup's step 7 (verifyGroupFiles)
// computes and caches every command's ExpandedCmdContentHash before step 8
// (executeAllCommands) runs any command for real. If an earlier command
// replaces a later command's binary as a side effect of its own real
// execution, the later command's fd-bound identity re-hash
// (openVerifiedIdentity) must detect the content mismatch and block
// execution rather than trusting the now-stale group-level hash. Because both
// commands run sequentially and deterministically, this reproduces the race
// described in 01_requirements.md without relying on real concurrency.
func TestGroupExecutor_F001_HashMismatchBlocksExecution(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	scriptDir := tu.SafeTempDir(t)

	targetPath := tu.WriteExecutableFile(t, scriptDir, "target.sh", []byte("#!/bin/sh\necho original\n"))
	tamperScript := "#!/bin/sh\nprintf '#!/bin/sh\\necho tampered\\n' > " + targetPath + "\nchmod +x " + targetPath + "\n"
	tamperPath := tu.WriteExecutableFile(t, scriptDir, "tamper.sh", []byte(tamperScript))

	// Record both scripts' true (original) content before any tampering happens.
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(tamperPath, false)
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(targetPath, false)
	require.NoError(t, err)

	verificationManager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	configSpec := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: tu.Int32Ptr(30),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:       "toctou-hash-mismatch-group",
				CmdAllowed: []string{tamperPath, targetPath},
				Commands: []runnertypes.CommandSpec{
					{
						Name:      "tamper-earlier-command",
						Cmd:       tamperPath,
						RiskLevel: runnertypes.RiskLevelHighPtr,
					},
					{
						Name:      "target-later-command",
						Cmd:       targetPath,
						RiskLevel: runnertypes.RiskLevelHighPtr,
					},
				},
			},
		},
	}

	r, err := NewRunner(
		configSpec,
		WithRunID("test-run-f001-hash-mismatch"),
		WithVerificationManager(verificationManager),
	)
	require.NoError(t, err)
	require.NoError(t, r.LoadSystemEnvironment())

	err = r.Execute(context.Background(), nil)
	require.Error(t, err, "execution must fail: the later command's binary was replaced after group verification but before its own execution")
	assert.Contains(t, err.Error(), string(risktypes.ReasonIdentityHashMismatch),
		"error should identify the identity hash mismatch reason")
}

// TestGroupExecutor_F004_LibraryShadowingBlocksExecution reproduces
// search-order shadowing: a library placed at a search location the resolver
// newly consults after record time. Dropping one entry from the recorded
// DynLibDeps snapshot (rather than manipulating RUNPATH/ld.so.cache directly)
// produces the same observable mismatch a real shadowing attack would: "the
// live resolver finds a dependency record time did not see", deterministically
// and without depending on system-specific library layouts.
func TestGroupExecutor_F004_LibraryShadowingBlocksExecution(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF test requires Linux")
	}

	hashDir := tu.SafeTempDir(t)
	cmdPath, err := filepath.EvalSymlinks("/bin/ls")
	require.NoError(t, err)

	validator := filevalidator.NewTestDynLibValidator(t, hashDir)
	_, _, err = validator.SaveRecord(cmdPath, false)
	require.NoError(t, err)

	getter := filevalidator.NewHybridHashFilePathGetter()
	store, err := fileanalysis.NewStore(hashDir, getter)
	require.NoError(t, err)
	resolvedPath, err := common.NewResolvedPath(cmdPath)
	require.NoError(t, err)
	err = store.Update(resolvedPath, func(record *fileanalysis.Record) error {
		if len(record.DynLibDeps) == 0 {
			return errors.New("test binary must have at least one recorded dependency")
		}
		record.DynLibDeps = record.DynLibDeps[1:]
		return nil
	})
	require.NoError(t, err)

	verificationManager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	configSpec := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: tu.Int32Ptr(30),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:       "dynlib-shadow-group",
				CmdAllowed: []string{cmdPath},
				Commands: []runnertypes.CommandSpec{
					{
						Name: "list-command",
						Cmd:  cmdPath,
						Args: []string{"/"},
					},
				},
			},
		},
	}

	r, err := NewRunner(
		configSpec,
		WithRunID("test-run-f004-library-shadowing"),
		WithVerificationManager(verificationManager),
	)
	require.NoError(t, err)
	require.NoError(t, r.LoadSystemEnvironment())

	err = r.Execute(context.Background(), nil)
	require.Error(t, err, "execution must fail: live dependency resolution no longer matches the recorded snapshot")
	assert.Contains(t, err.Error(), "dynamic library dependency resolution changed since record")
}
