package groupmembership

import "fmt"

// enumerationCompleteness states whether an enumeration result covers all
// members of the group.
type enumerationCompleteness int

const (
	// completenessUnstated is the zero value: the enumeration did not state
	// its completeness. It is a programming error, not an environment
	// condition, and is never treated as complete.
	completenessUnstated enumerationCompleteness = iota
	// completenessComplete states that the result covers all members.
	completenessComplete
	// completenessIncomplete states that the result may omit members.
	completenessIncomplete
)

// String returns the completeness name, used in error messages.
func (c enumerationCompleteness) String() string {
	switch c {
	case completenessUnstated:
		return "unstated"
	case completenessComplete:
		return "complete"
	case completenessIncomplete:
		return "incomplete"
	default:
		return fmt.Sprintf("unknown(%d)", int(c))
	}
}

// incompletenessCause names why an enumeration was judged incomplete. It
// selects the remediation text of the denial message.
type incompletenessCause int

const (
	// causeUnspecified is the zero value and carries no cause. It is only
	// valid on a verdict whose completeness is not completenessIncomplete.
	causeUnspecified incompletenessCause = iota
	// causeUnsupportedPlatform means the build's target platform cannot be
	// classified (anything other than linux).
	causeUnsupportedPlatform
	// causeNSSSources means /etc/nsswitch.conf configures a source whose
	// enumeration cannot be confirmed exhaustive, or could not be read.
	causeNSSSources
	// causeMalformedLine means a scan of /etc/group or /etc/passwd skipped
	// at least one line it could not parse.
	causeMalformedLine
)

// allIncompletenessCauses lists every defined cause. Advice for a cause is
// build-specific and chosen by a switch in each build, so the tests range over
// this rather than over a list of their own: a cause added above and left
// unclassified would otherwise fall silently into a switch's default and tell
// the operator to report their own host configuration as a defect.
var allIncompletenessCauses = []incompletenessCause{
	causeUnspecified,
	causeUnsupportedPlatform,
	causeNSSSources,
	causeMalformedLine,
}

// String returns the cause name, used in error messages and skip reasons.
func (c incompletenessCause) String() string {
	switch c {
	case causeUnspecified:
		return "unspecified"
	case causeUnsupportedPlatform:
		return "unsupported-platform"
	case causeNSSSources:
		return "nss-sources"
	case causeMalformedLine:
		return "malformed-line"
	default:
		return fmt.Sprintf("unknown(%d)", int(c))
	}
}

// completenessVerdict is the completeness of one enumeration together with
// the reason behind it. Construct it with completeVerdict or
// incompleteVerdict so that an incomplete verdict always carries a cause.
type completenessVerdict struct {
	completeness enumerationCompleteness
	cause        incompletenessCause
	detail       string
}

// completeVerdict states that an enumeration covers all members.
func completeVerdict() completenessVerdict {
	return completenessVerdict{completeness: completenessComplete}
}

// incompleteVerdict states that an enumeration may omit members, and why.
func incompleteVerdict(cause incompletenessCause, detail string) completenessVerdict {
	return completenessVerdict{completeness: completenessIncomplete, cause: cause, detail: detail}
}

// combine returns v unless v is complete, in which case it returns other.
// An enumeration is complete only when every source of doubt says so, and
// the cause reported is the one evaluated first.
func (v completenessVerdict) combine(other completenessVerdict) completenessVerdict {
	if v.completeness == completenessComplete {
		return other
	}
	return v
}

// groupEnumeration is the result of one group member enumeration.
type groupEnumeration struct {
	members []string
	verdict completenessVerdict
}
