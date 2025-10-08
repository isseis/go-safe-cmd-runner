package security

// CommandProfileDef associates a list of commands with their risk profile
type CommandProfileDef struct {
	commands []string
	profile  CommandRiskProfileNew
}

// Commands returns the list of commands for this profile
func (d CommandProfileDef) Commands() []string {
	return d.commands
}

// Profile returns the risk profile
func (d CommandProfileDef) Profile() CommandRiskProfileNew {
	return d.profile
}
