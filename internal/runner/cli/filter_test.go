package cli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/require"
)

func TestParseGroupNames(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "single group",
			input:    "build",
			expected: []string{"build"},
		},
		{
			name:     "multiple groups with spaces",
			input:    "build, test",
			expected: []string{"build", "test"},
		},
		{
			name:     "extra commas",
			input:    "build,,test",
			expected: []string{"build", "test"},
		},
		{
			name:     "commas only",
			input:    ",,",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGroupNames(tt.input)
			if tt.expected == nil {
				require.Nil(t, got)
				return
			}
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestFilterGroups(t *testing.T) {
	cfg := newTestConfig("common", "build", "test")

	t.Run("nil names returns all", func(t *testing.T) {
		got, err := FilterGroups(nil, cfg)
		require.NoError(t, err)
		expected := map[string]struct{}{
			"common": {},
			"build":  {},
			"test":   {},
		}
		require.Equal(t, expected, got)
	})

	t.Run("empty names returns all", func(t *testing.T) {
		got, err := FilterGroups([]string{}, cfg)
		require.NoError(t, err)
		expected := map[string]struct{}{
			"common": {},
			"build":  {},
			"test":   {},
		}
		require.Equal(t, expected, got)
	})

	t.Run("subset returns map", func(t *testing.T) {
		input := []string{"build"}
		got, err := FilterGroups(input, cfg)
		require.NoError(t, err)
		expected := map[string]struct{}{
			"build": {},
		}
		require.Equal(t, expected, got)
	})

	t.Run("multiple groups returns map", func(t *testing.T) {
		input := []string{"build", "test"}
		got, err := FilterGroups(input, cfg)
		require.NoError(t, err)
		expected := map[string]struct{}{
			"build": {},
			"test":  {},
		}
		require.Equal(t, expected, got)
	})

	t.Run("invalid name (hyphenated)", func(t *testing.T) {
		// Invalid group names (like "bad-name") won't exist in config since
		// config loading validates group names
		_, err := FilterGroups([]string{"bad-name"}, cfg)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrGroupNotFound))
	})

	t.Run("missing group", func(t *testing.T) {
		_, err := FilterGroups([]string{"deploy"}, cfg)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrGroupNotFound))
	})

	t.Run("nil config", func(t *testing.T) {
		_, err := FilterGroups([]string{"build"}, nil)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrNilConfig))
		require.False(t, errors.Is(err, ErrGroupNotFound))
	})
}

func newTestConfig(names ...string) *runnertypes.ConfigSpec {
	groups := make([]runnertypes.GroupSpec, len(names))
	for i, name := range names {
		groups[i] = runnertypes.GroupSpec{Name: name}
	}
	return &runnertypes.ConfigSpec{Groups: groups}
}

// listIn renders the "..." of "prefix[...]": the whitespace-separated elements
// between the first "[" after prefix and the next "]".
func listIn(t *testing.T, message, prefix string) []string {
	t.Helper()
	start := strings.Index(message, prefix)
	require.GreaterOrEqual(t, start, 0, "message is missing %q: %q", prefix, message)
	rest := message[start+len(prefix):]
	end := strings.Index(rest, "]")
	require.GreaterOrEqual(t, end, 0, "message has no closing bracket: %q", message)
	body := rest[:end]
	if body == "" {
		return nil
	}
	return strings.Split(body, " ")
}

// TestFilterGroups_GroupNotFoundStructuredMessage pins that the group-not-found
// error is a structured message: the requested names and every available group
// name are identifiers, so a name that looks like a secret still reaches the
// report, the missing/available lists render like fmt's %v, and errors.Is still
// reaches ErrGroupNotFound.
func TestFilterGroups_GroupNotFoundStructuredMessage(t *testing.T) {
	tests := []struct {
		name          string
		config        *runnertypes.ConfigSpec
		requested     []string
		wantMissing   []string
		wantAvailable []string
	}{
		{
			name:          "single missing name",
			config:        newTestConfig("common", "token_rotate", "build"),
			requested:     []string{"token_rotat"},
			wantMissing:   []string{"token_rotat"},
			wantAvailable: []string{"common", "token_rotate", "build"},
		},
		{
			name:          "two missing names keep request order",
			config:        newTestConfig("a", "b"),
			requested:     []string{"y", "x"},
			wantMissing:   []string{"y", "x"},
			wantAvailable: []string{"a", "b"},
		},
		{
			name:          "duplicate missing names are deduplicated",
			config:        newTestConfig("a"),
			requested:     []string{"x", "x"},
			wantMissing:   []string{"x"},
			wantAvailable: []string{"a"},
		},
		{
			name:          "no available groups",
			config:        &runnertypes.ConfigSpec{},
			requested:     []string{"x"},
			wantMissing:   []string{"x"},
			wantAvailable: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FilterGroups(tt.requested, tt.config)
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrGroupNotFound))

			structured, ok := err.(errmsg.Structured)
			require.True(t, ok, "%T must implement errmsg.Structured", err)

			wantIdentifiers := append(slices.Clone(tt.wantMissing), tt.wantAvailable...)
			var identifiers []string
			for _, segment := range structured.StructuredMessage().Segments() {
				if segment.Role == errmsg.RoleIdentifier {
					identifiers = append(identifiers, segment.Text)
				}
			}
			require.ElementsMatch(t, wantIdentifiers, identifiers,
				"the requested and available group names must all be identifiers")

			// The fixed wording and the %v-style lists are unchanged; only the
			// available-list order varies with map iteration.
			message := err.Error()
			require.Contains(t, message,
				"specified in --groups do not exist in configuration\nAvailable groups: [")
			require.Equal(t, tt.wantMissing, listIn(t, message, "group(s) ["))
			gotAvailable := listIn(t, message, "Available groups: [")
			slices.Sort(gotAvailable)
			wantAvailable := slices.Clone(tt.wantAvailable)
			slices.Sort(wantAvailable)
			require.Equal(t, wantAvailable, gotAvailable)
		})
	}
}
