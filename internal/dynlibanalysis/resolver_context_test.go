//go:build test

package dynlibanalysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRootContext(t *testing.T) {
	t.Run("with RPATH only", func(t *testing.T) {
		ctx := NewRootContext("/usr/bin/myapp", []string{"/opt/myapp/lib"}, nil, false)
		assert.Equal(t, "/usr/bin/myapp", ctx.ParentPath)
		assert.Equal(t, "/usr/bin", ctx.ParentDir)
		assert.Equal(t, []string{"/opt/myapp/lib"}, ctx.OwnRPATH)
		assert.Nil(t, ctx.OwnRUNPATH)
		assert.Nil(t, ctx.InheritedRPATH)
		assert.False(t, ctx.IncludeLDLibraryPath)
	})

	t.Run("with RUNPATH only", func(t *testing.T) {
		ctx := NewRootContext("/usr/bin/myapp", nil, []string{"/opt/myapp/lib"}, false)
		assert.Nil(t, ctx.OwnRPATH)
		assert.Equal(t, []string{"/opt/myapp/lib"}, ctx.OwnRUNPATH)
	})

	t.Run("RUNPATH overrides RPATH", func(t *testing.T) {
		ctx := NewRootContext("/usr/bin/myapp",
			[]string{"/rpath/lib"},
			[]string{"/runpath/lib"},
			false,
		)
		// When both are present, RUNPATH takes priority (OwnRPATH should be nil)
		assert.Nil(t, ctx.OwnRPATH)
		assert.Equal(t, []string{"/runpath/lib"}, ctx.OwnRUNPATH)
	})

	t.Run("with LD_LIBRARY_PATH enabled", func(t *testing.T) {
		ctx := NewRootContext("/usr/bin/myapp", nil, nil, true)
		assert.True(t, ctx.IncludeLDLibraryPath)
	})
}

func TestNewChildContext_RPATHInheritance(t *testing.T) {
	// Parent binary with RPATH
	parent := NewRootContext("/usr/bin/myapp", []string{"/opt/myapp/lib"}, nil, false)

	// Child library with RPATH (not RUNPATH), so it should inherit parent's RPATH
	child := parent.NewChildContext("/opt/myapp/lib/libfoo.so.1",
		[]string{"/opt/foo/lib"},
		nil,
	)

	assert.Equal(t, "/opt/myapp/lib/libfoo.so.1", child.ParentPath)
	assert.Equal(t, "/opt/myapp/lib", child.ParentDir)
	assert.Equal(t, []string{"/opt/foo/lib"}, child.OwnRPATH)
	assert.Nil(t, child.OwnRUNPATH)

	// Child should have parent's RPATH as inherited
	require.Len(t, child.InheritedRPATH, 1)
	assert.Equal(t, "/opt/myapp/lib", child.InheritedRPATH[0].Path)
	assert.Equal(t, "/usr/bin", child.InheritedRPATH[0].OriginDir)
}

func TestNewChildContext_RUNPATHTermination(t *testing.T) {
	// Parent binary with RPATH
	parent := NewRootContext("/usr/bin/myapp", []string{"/opt/myapp/lib"}, nil, false)

	// Child library with RUNPATH — should terminate the RPATH inheritance chain
	child := parent.NewChildContext("/usr/lib/x86_64-linux-gnu/libssl.so.3",
		nil,                              // no RPATH
		[]string{"/usr/lib/openssl/lib"}, // but has RUNPATH
	)

	// Child has RUNPATH so InheritedRPATH should be nil (inheritance terminated)
	assert.Nil(t, child.InheritedRPATH, "RUNPATH should terminate inherited RPATH chain")
	assert.Nil(t, child.OwnRPATH)
	assert.Equal(t, []string{"/usr/lib/openssl/lib"}, child.OwnRUNPATH)
}

func TestNewChildContext_DeepInheritance(t *testing.T) {
	// app -> libA (RPATH=/opt/a/lib) -> libB (no RPATH) -> libC's context
	root := NewRootContext("/usr/bin/myapp", []string{"/opt/app/lib"}, nil, false)
	libA := root.NewChildContext("/opt/a/lib/libA.so.1", []string{"/opt/a/lib"}, nil)
	libB := libA.NewChildContext("/opt/b/lib/libB.so.1", nil, nil)

	// libB should have:
	// - InheritedRPATH from app (/opt/app/lib) and libA (/opt/a/lib)
	require.Len(t, libB.InheritedRPATH, 2)
	// First: app's RPATH (inherited from root context)
	assert.Equal(t, "/opt/app/lib", libB.InheritedRPATH[0].Path)
	assert.Equal(t, "/usr/bin", libB.InheritedRPATH[0].OriginDir)
	// Second: libA's own RPATH (contributed by libA)
	assert.Equal(t, "/opt/a/lib", libB.InheritedRPATH[1].Path)
	assert.Equal(t, "/opt/a/lib", libB.InheritedRPATH[1].OriginDir)
}

func TestNewChildContext_ParentWithRUNPATH_NotInherited(t *testing.T) {
	// Parent has RUNPATH (not RPATH), so child should not inherit parent's RUNPATH
	parent := NewRootContext("/app", nil, []string{"/opt/app/lib"}, false)

	// Child with neither RPATH nor RUNPATH
	child := parent.NewChildContext("/opt/app/lib/libfoo.so.1", nil, nil)

	// Parent's RUNPATH should NOT be inherited (RUNPATH is not inherited by design)
	// InheritedRPATH should be empty (or nil) because parent had no RPATH to inherit
	assert.Empty(t, child.InheritedRPATH)
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := expandOrigin(tc.path, tc.originDir)
			assert.Equal(t, tc.expected, result)
		})
	}
}
