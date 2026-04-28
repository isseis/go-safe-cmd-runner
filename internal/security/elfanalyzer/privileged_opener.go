package elfanalyzer

import "github.com/isseis/go-safe-cmd-runner/internal/safefileio"

// PrivilegedFileOpener provides privileged open for execute-only binaries.
// If nil, StandardELFAnalyzer returns os.ErrPermission as AnalysisError.
type PrivilegedFileOpener interface {
	OpenWithPrivileges(path string) (safefileio.File, error)
}
