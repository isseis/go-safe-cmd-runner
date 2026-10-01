package cli

import (
	"errors"
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
	})
}

func newTestConfig(names ...string) *runnertypes.ConfigSpec {
	groups := make([]runnertypes.GroupSpec, len(names))
	for i, name := range names {
		groups[i] = runnertypes.GroupSpec{Name: name}
	}
	return &runnertypes.ConfigSpec{Groups: groups}
}

// TestFilterGroups_GroupNotFoundStructuredMessage pins that the group-not-found
// error is a structured message: the requested name and every available group
// name are identifiers, so a name that looks like a secret still reaches the
// report, and errors.Is still reaches ErrGroupNotFound.
func TestFilterGroups_GroupNotFoundStructuredMessage(t *testing.T) {
	cfg := newTestConfig("common", "token_rotate", "build")

	_, err := FilterGroups([]string{"token_rotat"}, cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrGroupNotFound))

	structured, ok := err.(errmsg.Structured)
	require.True(t, ok, "%T must implement errmsg.Structured", err)

	var identifiers []string
	for _, segment := range structured.StructuredMessage().Segments() {
		if segment.Role == errmsg.RoleIdentifier {
			identifiers = append(identifiers, segment.Text)
		}
	}
	require.ElementsMatch(t,
		[]string{"token_rotat", "common", "token_rotate", "build"},
		identifiers,
		"the requested and available group names must all be identifiers")

	// The fixed wording is unchanged; only the available-list order varies.
	message := err.Error()
	require.True(t,
		strings.HasPrefix(message, "group not found: group(s) [token_rotat] specified in --groups do not exist in configuration\nAvailable groups: ["),
		"unexpected message: %q", message)
	require.True(t, strings.HasSuffix(message, "]"), "unexpected message: %q", message)
}

// TestFilterGroups_NilConfigUnchanged pins that a nil config still returns
// ErrNilConfig before any group check runs, the behavior the removed
// reachable-only branch used to hide.
func TestFilterGroups_NilConfigUnchanged(t *testing.T) {
	_, err := FilterGroups([]string{"build"}, nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNilConfig))
	require.False(t, errors.Is(err, ErrGroupNotFound))
}
