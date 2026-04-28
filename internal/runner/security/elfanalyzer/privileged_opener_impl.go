package elfanalyzer

import (
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	secelfanalyzer "github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
)

type privilegedFileOpenerImpl struct {
	pfv         *filevalidator.PrivilegedFileValidator
	privManager runnertypes.PrivilegeManager
}

// NewPrivilegedFileOpener creates a runner-specific privileged opener implementation.
func NewPrivilegedFileOpener(
	fs safefileio.FileSystem,
	privManager runnertypes.PrivilegeManager,
) secelfanalyzer.PrivilegedFileOpener {
	return &privilegedFileOpenerImpl{
		pfv:         filevalidator.NewPrivilegedFileValidator(fs),
		privManager: privManager,
	}
}

// OpenWithPrivileges implements secelfanalyzer.PrivilegedFileOpener.
func (o *privilegedFileOpenerImpl) OpenWithPrivileges(path string) (safefileio.File, error) {
	return o.pfv.OpenFileWithPrivileges(path, o.privManager)
}
