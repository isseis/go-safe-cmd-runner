package elfanalyzer

import "github.com/isseis/go-safe-cmd-runner/internal/common"

// EvalMprotectRisk evaluates ArgEvalResults for mprotect-family risk.
// Covers both mprotect and pkey_mprotect syscalls.
// Returns true if PROT_EXEC risk exists (used for AnalysisWarnings
// entries and risk derivation in convertSyscallResult).
//
// Mapping rules:
//   - exec_confirmed → true
//   - exec_unknown   → true
//   - exec_not_set   → false
//   - no mprotect/pkey_mprotect entries → false
func EvalMprotectRisk(argEvalResults []common.SyscallArgEvalResult) bool {
	for _, r := range argEvalResults {
		isMember := false
		for _, familyName := range MprotectFamilyNames {
			if r.SyscallName == familyName {
				isMember = true
				break
			}
		}
		if !isMember {
			continue
		}
		switch r.Status {
		case common.SyscallArgEvalExecConfirmed,
			common.SyscallArgEvalExecUnknown:
			return true
		}
	}
	return false
}
