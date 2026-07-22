package verification

// ManagerInterface defines the interface for verification management
// This interface is introduced for testing purposes
type ManagerInterface interface {
	ResolvePath(path string) (string, error)
	VerifyGroupFiles(input *GroupVerificationInput) (*Result, error)
	VerifyCommandDynLibDeps(cmdPath string) error
	VerifyCommandShebangInterpreter(cmdPath string, envVars map[string]string) error
}

// Ensure Manager implements ManagerInterface
var _ ManagerInterface = (*Manager)(nil)
