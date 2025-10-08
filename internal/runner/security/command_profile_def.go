package security

// CommandProfileDef associates a list of commands with their risk profile
type CommandProfileDef struct {
	commands []string
	profile  CommandRiskProfile
}

// Commands returns a copy of the list of commands for this profile
func (d CommandProfileDef) Commands() []string {
	if d.commands == nil {
		return nil
	}
	result := make([]string, len(d.commands))
	copy(result, d.commands)
	return result
}

// Profile returns the risk profile
func (d CommandProfileDef) Profile() CommandRiskProfile {
	return d.profile
}
