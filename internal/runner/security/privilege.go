package security

// PrivilegeEscalationType represents different types of privilege escalation
type PrivilegeEscalationType string

const (
	// PrivilegeEscalationTypeSudo represents sudo-like privilege escalation commands
	PrivilegeEscalationTypeSudo PrivilegeEscalationType = "sudo"

	// PrivilegeEscalationTypeSu represents su-like privilege escalation commands
	PrivilegeEscalationTypeSu PrivilegeEscalationType = "su"

	// PrivilegeEscalationTypeSystemd represents systemd service control commands
	PrivilegeEscalationTypeSystemd PrivilegeEscalationType = "systemd"

	// PrivilegeEscalationTypeService represents legacy service control commands
	PrivilegeEscalationTypeService PrivilegeEscalationType = "service"

	// PrivilegeEscalationTypeOther represents other types of privilege escalation
	PrivilegeEscalationTypeOther PrivilegeEscalationType = "other"
)

// PrivilegeEscalationResult contains the result of privilege escalation analysis
type PrivilegeEscalationResult struct {
	IsPrivilegeEscalation bool
	EscalationType        PrivilegeEscalationType
	RiskLevel             RiskLevel
	RequiredPrivileges    []string
	CommandPath           string
	DetectedPattern       string
	Reason                string
}
