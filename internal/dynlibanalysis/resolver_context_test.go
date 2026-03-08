//go:build test

package dynlibanalysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewResolveContext(t *testing.T) {
	t.Run("with RUNPATH", func(t *testing.T) {
		ctx := NewResolveContext("/usr/bin/myapp", []string{"/opt/myapp/lib"})
		assert.Equal(t, "/usr/bin/myapp", ctx.ParentPath)
		assert.Equal(t, "/usr/bin", ctx.ParentDir())
		assert.Equal(t, []string{"/opt/myapp/lib"}, ctx.OwnRUNPATH)
	})

	t.Run("without RUNPATH", func(t *testing.T) {
		ctx := NewResolveContext("/usr/bin/myapp", nil)
		assert.Nil(t, ctx.OwnRUNPATH)
	})

	t.Run("child context fields", func(t *testing.T) {
		child := NewResolveContext("/opt/myapp/lib/libfoo.so.1", []string{"/opt/foo/lib"})
		assert.Equal(t, "/opt/myapp/lib/libfoo.so.1", child.ParentPath)
		assert.Equal(t, "/opt/myapp/lib", child.ParentDir())
		assert.Equal(t, []string{"/opt/foo/lib"}, child.OwnRUNPATH)
	})
}

func TestExpandOrigin(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		originDir string
		expected  string
	}{
		{
			name:      "no $ORIGIN",
			path:      "/opt/lib",
			originDir: "/usr/bin",
			expected:  "/opt/lib",
		},
		{
			name:      "$ORIGIN replaced",
			path:      "$ORIGIN/lib",
			originDir: "/usr/bin",
			expected:  "/usr/bin/lib",
		},
		{
			name:      "multiple $ORIGIN",
			path:      "$ORIGIN/../$ORIGIN/lib",
			originDir: "/usr/bin",
			expected:  "/usr/bin/..//usr/bin/lib",
		},
		{
			name:      "${ORIGIN} replaced",
			path:      "${ORIGIN}/lib",
			originDir: "/usr/bin",
			expected:  "/usr/bin/lib",
		},
		{
			name:      "mixed $ORIGIN and ${ORIGIN}",
			path:      "${ORIGIN}/a:$ORIGIN/b",
			originDir: "/app",
			expected:  "/app/a:/app/b",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := expandOrigin(tc.path, tc.originDir)
			assert.Equal(t, tc.expected, result)
		})
	}
}
