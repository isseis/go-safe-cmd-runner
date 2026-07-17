//go:build test

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
)

const hashDirPackage = "github.com/isseis/go-safe-cmd-runner/internal/cmdcommon.DefaultHashDirectory"

// hashDirLDFlags returns a -ldflags string that embeds hashDir as the default
// hash directory. The key=value pair is single-quoted so that paths containing
// spaces (e.g. Windows user-profile temp dirs) are not split by Go's internal
// ldflags parser.
func hashDirLDFlags(hashDir string) string {
	return fmt.Sprintf("-X '%s=%s'", hashDirPackage, hashDir)
}

// newGoRunCmdWithHashDir returns an *exec.Cmd that runs a freshly built copy
// of the current package's binary with hashDir embedded as the default hash
// directory via -ldflags. appArgs are passed to the binary.
//
// The binary is built directly (rather than invoked via `go run`) because
// `go run` does not propagate the child process's real exit code: on a
// non-zero exit it always reports exit code 1 to the caller (printing the
// real code to stderr as "exit status N" instead). Tests that assert a
// specific dry-run exit code (e.g. DryRunExitVerificationUnavailable = 3)
// need the real code on cmd.ProcessState.ExitCode().
func newGoRunCmdWithHashDir(t *testing.T, hashDir string, appArgs ...string) *exec.Cmd {
	t.Helper()
	ldflags := hashDirLDFlags(hashDir)
	binaryPath := filepath.Join(tu.SafeTempDir(t), "runner")
	build := exec.Command("go", "build", "-tags", "test", "-ldflags", ldflags, "-o", binaryPath, ".")
	build.Dir = "."
	buildOutput, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build runner binary: %v\n%s", err, buildOutput)
	}
	return exec.Command(binaryPath, appArgs...)
}

// newGoRunCmd returns an *exec.Cmd that runs a freshly built copy of the
// current package's binary with a freshly created temporary directory
// embedded as the default hash directory via -ldflags. The directory is
// automatically cleaned up when the test ends. appArgs are passed to the
// binary.
func newGoRunCmd(t *testing.T, appArgs ...string) *exec.Cmd {
	t.Helper()
	return newGoRunCmdWithHashDir(t, tu.SafeTempDir(t), appArgs...)
}
