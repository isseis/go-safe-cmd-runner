package security

import (
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// coreutilsDir is the coreutils directory used for path matching.
// Tests can redirect it to a temporary directory via SetCoreutilsDirForTest.
var coreutilsDir = common.CoreutilsDir

// safeCoreutilsCommands is the set of coreutils subcommands classified as Low risk:
// read-only, informational, text-processing, or new-creation commands that do not
// implicitly overwrite or delete existing data.
var safeCoreutilsCommands = map[string]struct{}{
	"arch":        {},
	"b2sum":       {},
	"base32":      {},
	"base64":      {},
	"basename":    {},
	"basenc":      {},
	"cat":         {},
	"cksum":       {},
	"comm":        {},
	"cut":         {},
	"date":        {},
	"df":          {},
	"dir":         {},
	"dircolors":   {},
	"dirname":     {},
	"du":          {},
	"echo":        {},
	"expand":      {},
	"expr":        {},
	"factor":      {},
	"false":       {},
	"fmt":         {},
	"fold":        {},
	"groups":      {},
	"hashsum":     {},
	"head":        {},
	"hostid":      {},
	"id":          {},
	"join":        {},
	"logname":     {},
	"ls":          {},
	"md5sum":      {},
	"mkdir":       {},
	"mktemp":      {},
	"nl":          {},
	"nproc":       {},
	"numfmt":      {},
	"od":          {},
	"paste":       {},
	"pathchk":     {},
	"pinky":       {},
	"pr":          {},
	"printenv":    {},
	"printf":      {},
	"ptx":         {},
	"pwd":         {},
	"readlink":    {},
	"realpath":    {},
	"relpath":     {},
	"seq":         {},
	"sha1sum":     {},
	"sha224sum":   {},
	"sha256sum":   {},
	"sha384sum":   {},
	"sha3sum":     {},
	"sha3-224sum": {},
	"sha3-256sum": {},
	"sha3-384sum": {},
	"sha3-512sum": {},
	"sha512sum":   {},
	"shake128sum": {},
	"shake256sum": {},
	"shuf":        {},
	"sleep":       {},
	"sort":        {},
	"stat":        {},
	"sum":         {},
	"tac":         {},
	"tail":        {},
	"test":        {},
	"tr":          {},
	"true":        {},
	"tsort":       {},
	"tty":         {},
	"uname":       {},
	"unexpand":    {},
	"uniq":        {},
	"users":       {},
	"vdir":        {},
	"wc":          {},
	"who":         {},
	"whoami":      {},
	"yes":         {},
}

// destructiveCoreutilsCommands is the set of coreutils subcommands classified as High risk:
// commands that can cause data loss or disk corruption.
var destructiveCoreutilsCommands = map[string]struct{}{
	"dd":       {},
	"rm":       {},
	"rmdir":    {},
	"shred":    {},
	"truncate": {},
	"unlink":   {},
}

// CoreutilsCommandRisk reports the risk level of a command whose resolved path
// is a direct child of the coreutils single-binary directory (CoreutilsDir).
//
// The directory test is an exact match: filepath.Dir(resolvedPath) == CoreutilsDir.
// When the path is not a direct child, it returns (RiskLevelUnknown, false, nil),
// and callers fall back to the normal risk evaluation path.
//
// For a coreutils path it determines the risk in this order:
//  1. setuid/setgid check: if the binary carries a setuid/setgid bit it returns
//     RiskLevelHigh (a coreutils hardlink is never expected to be setuid; a set
//     bit indicates packaging error or compromise). This stat may fail; on error
//     it returns (RiskLevelUnknown, false, err) so callers fail closed.
//  2. effective subcommand selection: normally the subcommand is the path
//     basename. For the multicall entrypoint (basename == "coreutils"), the
//     first non-option element of args is used as the effective subcommand, so
//     "coreutils rm -rf ..." is classified as rm.
//  3. classification by the effective subcommand:
//     - destructive commands (rm, dd, ...) -> RiskLevelHigh
//     - known safe commands (mkdir, ls, ...) -> RiskLevelLow
//     - everything else, including an unknown or unidentifiable subcommand ->
//     RiskLevelHigh (fail-safe default). Only subcommands explicitly listed in
//     safeCoreutilsCommands are treated as Low; an unparseable multicall
//     invocation may hide a destructive subcommand, so it must not pass at
//     Medium.
func CoreutilsCommandRisk(resolvedPath string, args []string) (runnertypes.RiskLevel, bool, error) {
	if filepath.Dir(resolvedPath) != coreutilsDir {
		return runnertypes.RiskLevelUnknown, false, nil
	}

	hasSetuid, err := hasSetuidOrSetgidBit(resolvedPath)
	if err != nil {
		return runnertypes.RiskLevelUnknown, false, err
	}
	if hasSetuid {
		return runnertypes.RiskLevelHigh, true, nil
	}

	subcmd := filepath.Base(resolvedPath)
	if subcmd == "coreutils" {
		subcmd = findFirstSubcommand(args)
	}

	if _, ok := destructiveCoreutilsCommands[subcmd]; ok {
		return runnertypes.RiskLevelHigh, true, nil
	}
	if _, ok := safeCoreutilsCommands[subcmd]; ok {
		return runnertypes.RiskLevelLow, true, nil
	}
	// Unknown or unidentifiable subcommand: fail safe to High rather than Medium,
	// so an unparseable "coreutils <subcmd>" cannot run under risk_level = "medium".
	return runnertypes.RiskLevelHigh, true, nil
}
