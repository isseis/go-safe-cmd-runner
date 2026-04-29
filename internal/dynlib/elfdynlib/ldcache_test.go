//go:build test

package elfdynlib

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestLDCache constructs a minimal valid new-format ld.so.cache binary
// with the given soname->path entries.
func buildTestLDCache(entries map[string]string) []byte {
	// Build string table first to determine offsets
	// Format: null-terminated strings packed sequentially
	type strEntry struct {
		key      string
		value    string
		keyOff   uint32
		valueOff uint32
	}

	var strTable bytes.Buffer
	var strEntries []strEntry

	for k, v := range entries {
		keyOff := uint32(strTable.Len())
		strTable.WriteString(k)
		strTable.WriteByte(0)

		valueOff := uint32(strTable.Len())
		strTable.WriteString(v)
		strTable.WriteByte(0)

		strEntries = append(strEntries, strEntry{
			key:      k,
			value:    v,
			keyOff:   keyOff,
			valueOff: valueOff,
		})
	}

	var buf bytes.Buffer

	// Write magic (19 bytes, no null terminator)
	buf.WriteString(cachemagicNew)

	// Align to 4-byte boundary after magic (19 bytes -> pad to 20)
	for buf.Len()%4 != 0 {
		buf.WriteByte(0)
	}

	// Write header
	header := newCacheHeader{
		NLibs:      uint32(len(strEntries)),
		LenStrings: uint32(strTable.Len()),
	}
	_ = binary.Write(&buf, binary.LittleEndian, header)

	// Write cache entries
	for _, se := range strEntries {
		entry := newCacheEntry{
			Flags:       0,
			KeyOffset:   se.keyOff,
			ValueOffset: se.valueOff,
			OSVersion:   0,
			HWCap:       0,
		}
		_ = binary.Write(&buf, binary.LittleEndian, entry)
	}

	// Write string table
	buf.Write(strTable.Bytes())

	return buf.Bytes()
}

func TestParseLDCache_NewFormat(t *testing.T) {
	testEntries := map[string]string{
		"libssl.so.3":  "/usr/lib/x86_64-linux-gnu/libssl.so.3.0.0",
		"libc.so.6":    "/lib/x86_64-linux-gnu/libc-2.31.so",
		"libpthread.0": "/lib/x86_64-linux-gnu/libpthread-2.31.so",
	}

	data := buildTestLDCache(testEntries)
	cache, err := parseLDCacheData(data)
	require.NoError(t, err)
	require.NotNil(t, cache)

	for soname, expectedPath := range testEntries {
		assert.Equal(t, expectedPath, cache.Lookup(soname),
			"lookup for %s should return %s", soname, expectedPath)
	}
}

func TestParseLDCache_UnsupportedFormat(t *testing.T) {
	// Old format magic
	data := []byte("ld.so-1.7.0\x00this is old format data padding here xxxx")
	_, err := parseLDCacheData(data)
	assert.ErrorIs(t, err, errUnsupportedFormat)
}

func TestParseLDCache_TooSmall(t *testing.T) {
	data := []byte("small")
	_, err := parseLDCacheData(data)
	assert.ErrorIs(t, err, errLDCacheTooSmall)
}

func TestParseLDCache_Truncated(t *testing.T) {
	// Build valid cache and then truncate it
	testEntries := map[string]string{
		"libssl.so.3": "/usr/lib/x86_64-linux-gnu/libssl.so.3.0.0",
	}
	data := buildTestLDCache(testEntries)
	// Truncate to less than full size (between header and string table)
	truncated := data[:len(data)/2]
	_, err := parseLDCacheData(truncated)
	// Should fail with either header truncated or data truncated depending on how far we cut
	assert.Error(t, err)
}

func TestParseLDCache_NotFound(t *testing.T) {
	// ParseLDCache with a non-existent path should return an error
	_, err := ParseLDCache("/nonexistent/ld.so.cache")
	assert.Error(t, err)
}

func TestLDCache_Lookup(t *testing.T) {
	testEntries := map[string]string{
		"libssl.so.3": "/usr/lib/x86_64-linux-gnu/libssl.so.3.0.0",
		"libc.so.6":   "/lib/x86_64-linux-gnu/libc-2.31.so",
	}

	data := buildTestLDCache(testEntries)
	cache, err := parseLDCacheData(data)
	require.NoError(t, err)

	// Found entries
	assert.Equal(t, "/usr/lib/x86_64-linux-gnu/libssl.so.3.0.0", cache.Lookup("libssl.so.3"))
	assert.Equal(t, "/lib/x86_64-linux-gnu/libc-2.31.so", cache.Lookup("libc.so.6"))

	// Not found
	assert.Equal(t, "", cache.Lookup("libnotexist.so.1"))
}

func TestParseLDCache_FirstEntryWins(t *testing.T) {
	// Build cache manually with duplicate keys to test first-entry-wins behavior
	// We need to manually construct the binary data with duplicate keys.
	// Since the builder uses a map (no duplicates), we construct manually.
	var strTable bytes.Buffer
	key1Off := uint32(strTable.Len())
	strTable.WriteString("libfoo.so.1")
	strTable.WriteByte(0)
	val1Off := uint32(strTable.Len())
	strTable.WriteString("/lib/first/libfoo.so.1")
	strTable.WriteByte(0)
	key2Off := uint32(strTable.Len())
	strTable.WriteString("libfoo.so.1")
	strTable.WriteByte(0)
	val2Off := uint32(strTable.Len())
	strTable.WriteString("/lib/second/libfoo.so.1")
	strTable.WriteByte(0)

	var buf bytes.Buffer
	buf.WriteString(cachemagicNew)
	for buf.Len()%4 != 0 {
		buf.WriteByte(0)
	}

	header := newCacheHeader{NLibs: 2, LenStrings: uint32(strTable.Len())}
	_ = binary.Write(&buf, binary.LittleEndian, header)

	entry1 := newCacheEntry{KeyOffset: key1Off, ValueOffset: val1Off}
	entry2 := newCacheEntry{KeyOffset: key2Off, ValueOffset: val2Off}
	_ = binary.Write(&buf, binary.LittleEndian, entry1)
	_ = binary.Write(&buf, binary.LittleEndian, entry2)
	buf.Write(strTable.Bytes())

	cache, err := parseLDCacheData(buf.Bytes())
	require.NoError(t, err)
	// First entry should win
	assert.Equal(t, "/lib/first/libfoo.so.1", cache.Lookup("libfoo.so.1"))
}
