package elfanalyzer

import "github.com/isseis/go-safe-cmd-runner/internal/common"

// EvalMprotectRisk evaluates ArgEvalResults for mprotect-related risk.
// Returns true if IsHighRisk should be set based on mprotect detection.
//
// Mapping rules:
//   - exec_confirmed → true
//   - exec_unknown   → true
//   - exec_not_set   → false
//   - no mprotect entries → false
func EvalMprotectRisk(argEvalResults []common.SyscallArgEvalResult) bool {
	for _, r := range argEvalResults {
		if r.SyscallName != "mprotect" {
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
