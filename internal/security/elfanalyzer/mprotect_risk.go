package elfanalyzer

import (
	"slices"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// FirstMprotectRisk returns a copy of the first ArgEvalResult in the mprotect
// family (mprotect or pkey_mprotect) that represents PROT_EXEC risk
// (exec_confirmed or exec_unknown), plus true. Returns the zero value and
// false when no such entry is found.
func FirstMprotectRisk(argEvalResults []common.SyscallArgEvalResult) (common.SyscallArgEvalResult, bool) {
	for _, r := range argEvalResults {
		if slices.Contains(MprotectFamilyNames, r.SyscallName) {
			switch r.Status {
			case common.SyscallArgEvalExecConfirmed,
				common.SyscallArgEvalExecUnknown:
				return r, true
			}
		}
	}
	return common.SyscallArgEvalResult{}, false
}

// EvalMprotectRisk reports whether argEvalResults contain any mprotect-family
// PROT_EXEC risk. See FirstMprotectRisk for the matching rules.
func EvalMprotectRisk(argEvalResults []common.SyscallArgEvalResult) bool {
	_, ok := FirstMprotectRisk(argEvalResults)
	return ok
}
