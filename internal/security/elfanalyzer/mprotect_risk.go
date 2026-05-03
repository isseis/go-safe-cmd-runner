package elfanalyzer

import "github.com/isseis/go-safe-cmd-runner/internal/common"

// FirstMprotectRisk returns the first ArgEvalResult in the mprotect family
// (mprotect or pkey_mprotect) that represents PROT_EXEC risk (exec_confirmed
// or exec_unknown), or nil if none found.
func FirstMprotectRisk(argEvalResults []common.SyscallArgEvalResult) *common.SyscallArgEvalResult {
	for i := range argEvalResults {
		r := &argEvalResults[i]
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
			return r
		}
	}
	return nil
}

// EvalMprotectRisk reports whether argEvalResults contain any mprotect-family
// PROT_EXEC risk. See FirstMprotectRisk for the matching rules.
func EvalMprotectRisk(argEvalResults []common.SyscallArgEvalResult) bool {
	return FirstMprotectRisk(argEvalResults) != nil
}
