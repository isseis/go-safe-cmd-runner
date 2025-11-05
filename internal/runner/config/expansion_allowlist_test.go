package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetermineEffectiveEnvAllowlist(t *testing.T) {
	tests := []struct {
		name            string
		groupAllowlist  []string
		globalAllowlist []string
		want            []string
	}{
		{
			name:            "nil group allowlist inherits from global",
			groupAllowlist:  nil,
			globalAllowlist: []string{"PATH", "HOME"},
			want:            []string{"PATH", "HOME"},
		},
		{
			name:            "empty group allowlist overrides (reject all)",
			groupAllowlist:  []string{},
			globalAllowlist: []string{"PATH", "HOME"},
			want:            []string{},
		},
		{
			name:            "defined group allowlist overrides global",
			groupAllowlist:  []string{"USER"},
			globalAllowlist: []string{"PATH", "HOME"},
			want:            []string{"USER"},
		},
		{
			name:            "nil group with empty global",
			groupAllowlist:  nil,
			globalAllowlist: []string{},
			want:            []string{},
		},
		{
			name:            "nil group with nil global",
			groupAllowlist:  nil,
			globalAllowlist: nil,
			want:            nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineEffectiveEnvAllowlist(tt.groupAllowlist, tt.globalAllowlist)

			// Handle nil vs empty slice comparison
			if tt.want == nil && got == nil {
				return
			}
			if (tt.want == nil) != (got == nil) {
				assert.Fail(t, "nil mismatch", "got %v, want %v", got, tt.want)
				return
			}

			assert.Equal(t, len(tt.want), len(got), "slice length mismatch")
			if len(got) != len(tt.want) {
				return
			}
			for i := range got {
				assert.Equal(t, tt.want[i], got[i], "element at index %d mismatch", i)
			}
		})
	}
}
