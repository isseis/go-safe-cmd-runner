//go:build test

package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCollectPermissionCheckDirs verifies that directory collection covers both
// kinds of source, walks up to the root, and deduplicates. The assertion is an
// exact multiset comparison, so an extra or repeated directory fails too.
func TestCollectPermissionCheckDirs(t *testing.T) {
	tests := []struct {
		name      string
		filePaths []string
		dirs      []string
		wantDirs  []string
	}{
		{
			name:      "empty inputs returns empty",
			filePaths: []string{},
			dirs:      []string{},
			wantDirs:  []string{},
		},
		{
			name:      "file parent directories and all ancestors are included",
			filePaths: []string{"/usr/bin/python3", "/etc/hosts"},
			wantDirs:  []string{"/usr/bin", "/usr", "/", "/etc"},
		},
		{
			name:     "a directory itself is included with its ancestors",
			dirs:     []string{"/var/lib/hashes"},
			wantDirs: []string{"/var/lib/hashes", "/var/lib", "/var", "/"},
		},
		{
			name:      "empty strings are ignored",
			filePaths: []string{""},
			dirs:      []string{""},
			wantDirs:  []string{},
		},
		{
			name:      "duplicates are removed",
			filePaths: []string{"/usr/bin/python3", "/usr/bin/python2", "/usr/bin/echo"},
			wantDirs:  []string{"/usr/bin", "/usr", "/"},
		},
		{
			name:      "combined sources without duplicates",
			filePaths: []string{"/usr/bin/python3", "/usr/local/bin/tool"},
			dirs:      []string{"/var/hashes", "/usr/bin"},
			wantDirs:  []string{"/usr/bin", "/usr", "/", "/usr/local/bin", "/usr/local", "/var/hashes", "/var"},
		},
		{
			name:      "paths are cleaned before collection",
			filePaths: []string{"/usr//bin/./python3"},
			dirs:      []string{"/usr/bin/"},
			wantDirs:  []string{"/usr/bin", "/usr", "/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollectPermissionCheckDirs(tt.filePaths, tt.dirs)
			assert.ElementsMatch(t, tt.wantDirs, got)
		})
	}
}

// TestClassifyCheckTarget verifies the three skip reasons, the precedence of a
// declared unexpanded reference over relativeness, and that the declaration — not
// the text of the path — is what decides. The last two rows are the point: the same
// "%{"-bearing path is checked or skipped according to what the caller declared.
func TestClassifyCheckTarget(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		state PathExpansionState
		want  CheckSkipReason
	}{
		{name: "absolute path is checkable", path: "/usr/bin/echo", state: PathExpanded, want: CheckSkipNone},
		{name: "relative path", path: "bin/echo", state: PathExpanded, want: CheckSkipRelative},
		{name: "declared unexpanded reference", path: "/opt/%{APP}/bin", state: PathHasUnexpandedReference, want: CheckSkipVariableReference},
		{name: "declared unexpanded reference outranks relativeness", path: "%{APP}/bin", state: PathHasUnexpandedReference, want: CheckSkipVariableReference},
		{name: "expanded path may legitimately contain %{", path: "/opt/%{LITERAL}/bin", state: PathExpanded, want: CheckSkipNone},
		{name: "expanded relative path containing %{", path: "%{LITERAL}/bin", state: PathExpanded, want: CheckSkipRelative},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyCheckTarget(tt.path, tt.state))
		})
	}
}

// TestClassifyCheckTarget_UnknownStatePanics pins the fail-secure default: a state
// added without a case here must stop the run rather than fall into "checkable" or
// "skip" by accident.
func TestClassifyCheckTarget_UnknownStatePanics(t *testing.T) {
	assert.Panics(t, func() { ClassifyCheckTarget("/usr/bin/echo", PathExpansionState(99)) })
}
