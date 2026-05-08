//go:build test

package main

import "fmt"

const hashDirPackage = "github.com/isseis/go-safe-cmd-runner/internal/cmdcommon.DefaultHashDirectory"

// hashDirLDFlags returns a -ldflags string that embeds hashDir as the default
// hash directory. The key=value pair is single-quoted so that paths containing
// spaces (e.g. Windows user-profile temp dirs) are not split by Go's internal
// ldflags parser.
func hashDirLDFlags(hashDir string) string {
	return fmt.Sprintf("-X '%s=%s'", hashDirPackage, hashDir)
}
