package groupmembership

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnumerationCompleteness_String(t *testing.T) {
	tests := []struct {
		name string
		c    enumerationCompleteness
		want string
	}{
		{name: "zero value is unstated", c: completenessUnstated, want: "unstated"},
		{name: "complete", c: completenessComplete, want: "complete"},
		{name: "incomplete", c: completenessIncomplete, want: "incomplete"},
		{name: "unrecognized value", c: enumerationCompleteness(99), want: "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.c.String())
		})
	}
}

func TestIncompletenessCause_String(t *testing.T) {
	tests := []struct {
		name string
		c    incompletenessCause
		want string
	}{
		{name: "zero value is unspecified", c: causeUnspecified, want: "unspecified"},
		{name: "unsupported platform", c: causeUnsupportedPlatform, want: "unsupported-platform"},
		{name: "nss sources", c: causeNSSSources, want: "nss-sources"},
		{name: "malformed line", c: causeMalformedLine, want: "malformed-line"},
		{name: "unrecognized value", c: incompletenessCause(99), want: "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.c.String())
		})
	}
}

func TestCompleteVerdict(t *testing.T) {
	v := completeVerdict()
	assert.Equal(t, completenessComplete, v.completeness)
	assert.Equal(t, causeUnspecified, v.cause)
	assert.Empty(t, v.detail)
}

func TestIncompleteVerdict(t *testing.T) {
	v := incompleteVerdict(causeNSSSources, "group: ldap")
	assert.Equal(t, completenessIncomplete, v.completeness)
	assert.Equal(t, causeNSSSources, v.cause)
	assert.Equal(t, "group: ldap", v.detail)
}

func TestCompletenessVerdict_Combine(t *testing.T) {
	complete := completeVerdict()
	incompleteA := incompleteVerdict(causeNSSSources, "detail-a")
	incompleteB := incompleteVerdict(causeMalformedLine, "detail-b")
	unstated := completenessVerdict{}

	tests := []struct {
		name string
		v    completenessVerdict
		with completenessVerdict
		want completenessVerdict
	}{
		{
			name: "complete combined with complete stays complete",
			v:    complete,
			with: complete,
			want: complete,
		},
		{
			name: "complete combined with incomplete becomes incomplete",
			v:    complete,
			with: incompleteA,
			want: incompleteA,
		},
		{
			name: "incomplete combined with complete keeps its own cause",
			v:    incompleteA,
			with: complete,
			want: incompleteA,
		},
		{
			name: "incomplete combined with incomplete keeps the first-evaluated cause",
			v:    incompleteA,
			with: incompleteB,
			want: incompleteA,
		},
		{
			name: "unstated combined with complete never becomes complete",
			v:    unstated,
			with: complete,
			want: unstated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.v.combine(tt.with))
		})
	}
}
