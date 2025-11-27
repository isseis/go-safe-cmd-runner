//go:build test

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/require"
)

const (
	hashDirPerm    = 0o700
	configFilePerm = 0o600
)

// testEnvironment holds common test setup artifacts.
type testEnvironment struct {
	TestDir    string
	HashDir    string
	ConfigPath string
	RunID      string
}

// setupTestEnvironment creates the common test directory structure.
func setupTestEnvironment(t *testing.T, runID string) *testEnvironment {
	t.Helper()
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")

	err := os.MkdirAll(hashDir, hashDirPerm)
	require.NoError(t, err)

	return &testEnvironment{
		TestDir:    testDir,
		HashDir:    hashDir,
		ConfigPath: configPath,
		RunID:      runID,
	}
}

// writeConfig writes the configuration content to the config file.
func (env *testEnvironment) writeConfig(t *testing.T, configContent string) {
	t.Helper()
	err := os.WriteFile(env.ConfigPath, []byte(configContent), configFilePerm)
	require.NoError(t, err)
}

// createRunner creates and initializes a runner with the test configuration.
func (env *testEnvironment) createRunner(t *testing.T) *runner.Runner {
	t.Helper()

	verificationManager, err := verification.NewManagerForTest(env.HashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, env.ConfigPath, env.RunID)
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID(env.RunID),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	return r
}

// outputFilePath returns a path for the output.txt file in the test directory.
func (env *testEnvironment) outputFilePath() string {
	return filepath.Join(env.TestDir, "output.txt")
}
