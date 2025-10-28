package config

import (
	"testing"
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
				t.Errorf("nil mismatch: got %v, want %v", got, tt.want)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("len(got) = %d, len(want) = %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %s, want[%d] = %s", i, got[i], i, tt.want[i])
				}
			}
		})
	}
}
