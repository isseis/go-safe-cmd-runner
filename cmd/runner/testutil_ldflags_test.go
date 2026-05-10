//go:build test

package main

import (
	"fmt"
	"os/exec"
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

// newGoRunCmd returns an *exec.Cmd that runs the current package via `go run`
// with a freshly created temporary directory embedded as the default hash
// directory via -ldflags. The directory is automatically cleaned up when the
// test ends. appArgs are passed to the compiled binary after ".".
func newGoRunCmd(t *testing.T, appArgs ...string) *exec.Cmd {
	t.Helper()
	hashDir := tu.SafeTempDir(t)
	ldflags := hashDirLDFlags(hashDir)
	args := append([]string{"run", "-ldflags", ldflags, "."}, appArgs...)
	cmd := exec.Command("go", args...)
	cmd.Dir = "."
	return cmd
}
