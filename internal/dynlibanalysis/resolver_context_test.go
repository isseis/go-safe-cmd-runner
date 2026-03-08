//go:build test

package dynlibanalysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRootContext(t *testing.T) {
	t.Run("with RUNPATH", func(t *testing.T) {
		ctx := NewRootContext("/usr/bin/myapp", []string{"/opt/myapp/lib"}, false)
		assert.Equal(t, "/usr/bin/myapp", ctx.ParentPath)
		assert.Equal(t, "/usr/bin", ctx.ParentDir)
		assert.Equal(t, []string{"/opt/myapp/lib"}, ctx.OwnRUNPATH)
		assert.False(t, ctx.IncludeLDLibraryPath)
	})

	t.Run("without RUNPATH", func(t *testing.T) {
		ctx := NewRootContext("/usr/bin/myapp", nil, false)
		assert.Nil(t, ctx.OwnRUNPATH)
	})

	t.Run("with LD_LIBRARY_PATH enabled", func(t *testing.T) {
		ctx := NewRootContext("/usr/bin/myapp", nil, true)
		assert.True(t, ctx.IncludeLDLibraryPath)
	})
}

func TestNewChildContext(t *testing.T) {
	t.Run("child inherits IncludeLDLibraryPath", func(t *testing.T) {
		parent := NewRootContext("/usr/bin/myapp", []string{"/opt/myapp/lib"}, true)
		child := parent.NewChildContext("/opt/myapp/lib/libfoo.so.1", []string{"/opt/foo/lib"})

		assert.Equal(t, "/opt/myapp/lib/libfoo.so.1", child.ParentPath)
		assert.Equal(t, "/opt/myapp/lib", child.ParentDir)
		assert.Equal(t, []string{"/opt/foo/lib"}, child.OwnRUNPATH)
		assert.True(t, child.IncludeLDLibraryPath)
	})

	t.Run("child with no RUNPATH", func(t *testing.T) {
		parent := NewRootContext("/usr/bin/myapp", nil, false)
		child := parent.NewChildContext("/usr/lib/libssl.so.3", nil)

		assert.Nil(t, child.OwnRUNPATH)
		assert.False(t, child.IncludeLDLibraryPath)
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
