//go:build test

package fileanalysis

import (
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynLibDepsStore_LoadDynLibDeps_Normal(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, &mockPathGetter{})
	require.NoError(t, err)

	depsStore := NewDynLibDepsStore(store)

	// Create a temp file to record.
	f, err := os.CreateTemp(dir, "cmd")
	require.NoError(t, err)
	_ = f.Close()

	rp, err := common.NewResolvedPath(f.Name())
	require.NoError(t, err)

	deps := []LibEntry{
		{SOName: "libssl.so.3", Path: "/usr/lib/libssl.so.3", Hash: "sha256:aaaa"},
	}
	record := &Record{
		ContentHash: "sha256:testhash",
		DynLibDeps:  deps,
	}
	require.NoError(t, store.Save(rp, record))

	got, err := depsStore.LoadDynLibDeps(f.Name(), "sha256:testhash")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "/usr/lib/libssl.so.3", got[0].Path)
	assert.Equal(t, "sha256:aaaa", got[0].Hash)
}

func TestDynLibDepsStore_LoadDynLibDeps_HashMismatch(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, &mockPathGetter{})
	require.NoError(t, err)

	depsStore := NewDynLibDepsStore(store)

	f, err := os.CreateTemp(dir, "cmd")
	require.NoError(t, err)
	_ = f.Close()

	rp, err := common.NewResolvedPath(f.Name())
	require.NoError(t, err)

	require.NoError(t, store.Save(rp, &Record{ContentHash: "sha256:correct"}))

	_, err = depsStore.LoadDynLibDeps(f.Name(), "sha256:wrong")
	assert.ErrorIs(t, err, ErrHashMismatch)
}

func TestDynLibDepsStore_LoadDynLibDeps_NoDeps(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, &mockPathGetter{})
	require.NoError(t, err)

	depsStore := NewDynLibDepsStore(store)

	f, err := os.CreateTemp(dir, "cmd")
	require.NoError(t, err)
	_ = f.Close()

	rp, err := common.NewResolvedPath(f.Name())
	require.NoError(t, err)

	require.NoError(t, store.Save(rp, &Record{ContentHash: "sha256:hash"}))

	got, err := depsStore.LoadDynLibDeps(f.Name(), "sha256:hash")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDynLibDepsStore_LoadDynLibDeps_RecordNotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, &mockPathGetter{})
	require.NoError(t, err)

	depsStore := NewDynLibDepsStore(store)
	// Use an existing path but without saving a record for it.
	f, err := os.CreateTemp(dir, "norecord")
	require.NoError(t, err)
	_ = f.Close()

	_, err = depsStore.LoadDynLibDeps(f.Name(), "sha256:hash")
	assert.ErrorIs(t, err, ErrRecordNotFound)
}
