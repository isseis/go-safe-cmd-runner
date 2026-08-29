package groupmembership

// incompletenessAdvice is what an operator is told about one cause: the fact
// behind the denial, and what to do about it on this build.
type incompletenessAdvice struct {
	fact        string
	remediation string
}

// implementationDefectAdvice is the advice for a cause that no environment can
// produce, so it is the same on every build.
func implementationDefectAdvice(what string) incompletenessAdvice {
	return incompletenessAdvice{
		fact:        what,
		remediation: "report this as a defect in the enumeration implementation",
	}
}
