package dynlibanalysis

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

const (
	// cachemagicNew is the magic string for the new format of ld.so.cache.
	// Only this format is supported; the old "ld.so-1.7.0" format is not.
	cachemagicNew    = "glibc-ld.so.cache1.1"
	cachemagicNewLen = 19 // length of cachemagicNew without null terminator

	// newEntrySize is the size of a single cache entry in the new format.
	newEntrySize = 24 // flags(4) + key_offset(4) + value_offset(4) + osversion(4) + hwcap(8)

	// headerPadding is the number of unused uint32 fields in the header.
	headerPadding = 5

	// alignment is the byte alignment boundary for the cache header.
	alignment = 4

	// magicPreviewLen is the number of bytes to show in unsupported format error messages.
	magicPreviewLen = 20
)

// Sentinel errors for ld.so.cache parsing failures.
var (
	errLDCacheTooSmall   = errors.New("ld.so.cache too small")
	errUnsupportedFormat = errors.New("unsupported ld.so.cache format")
	errHeaderTruncated   = errors.New("ld.so.cache header truncated")
	errDataTruncated     = errors.New("ld.so.cache data truncated")
)

// LDCache represents a parsed /etc/ld.so.cache file.
type LDCache struct {
	entries map[string]string // soname -> resolved path
}

// newCacheHeader is the header structure of the new ld.so.cache format.
type newCacheHeader struct {
	// NLibs is the number of library entries.
	NLibs uint32
	// LenStrings is the total size of the string table in bytes.
	LenStrings uint32
	// Unused contains reserved fields.
	Unused [headerPadding]uint32
}

// newCacheEntry is a single entry in the new ld.so.cache format.
type newCacheEntry struct {
	Flags       int32
	KeyOffset   uint32
	ValueOffset uint32
	OSVersion   uint32
	HWCap       uint64
}

// ParseLDCache parses the /etc/ld.so.cache binary file at the given path.
// Only the new format ("glibc-ld.so.cache1.1") is supported.
// Returns (nil, error) if the cache is unavailable or uses an unsupported format.
// The caller should treat a nil cache as "cache unavailable" and proceed with
// default path fallback.
func ParseLDCache(path string) (*LDCache, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is controlled by the caller (system cache)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("ld.so.cache not found, falling back to default paths",
				"path", path)
			return nil, fmt.Errorf("ld.so.cache not found: %w", err)
		}
		slog.Warn("failed to read ld.so.cache, falling back to default paths",
			"path", path, "error", err)
		return nil, fmt.Errorf("failed to read ld.so.cache: %w", err)
	}

	return parseLDCacheData(data)
}

// parseLDCacheData parses the raw bytes of an ld.so.cache file.
// Unexported; tests within the same package access it directly with synthetic cache data.
func parseLDCacheData(data []byte) (*LDCache, error) {
	// Check minimum size for magic
	if len(data) < cachemagicNewLen {
		return nil, fmt.Errorf("%w: %d bytes", errLDCacheTooSmall, len(data))
	}

	// The new format may appear either at the beginning of the file or
	// after the old format header. Search for the magic string.
	newStart := bytes.Index(data, []byte(cachemagicNew))
	if newStart < 0 {
		slog.Warn("unsupported ld.so.cache format, falling back to default paths",
			"format", fmt.Sprintf("%q", data[:min(magicPreviewLen, len(data))]))
		return nil, errUnsupportedFormat
	}

	// Skip the magic string (padded to align with the header)
	headerStart := newStart + cachemagicNewLen
	// Align to next uint32 boundary
	if headerStart%alignment != 0 {
		headerStart += alignment - (headerStart % alignment)
	}

	// Parse header
	headerSize := binary.Size(newCacheHeader{})
	if len(data) < headerStart+headerSize {
		return nil, errHeaderTruncated
	}

	var header newCacheHeader
	reader := bytes.NewReader(data[headerStart:])
	if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
		return nil, fmt.Errorf("failed to read ld.so.cache header: %w", err)
	}

	// Calculate offsets using uint64 to prevent integer overflow on 32-bit platforms.
	// header.NLibs, header.LenStrings, entry.KeyOffset, and entry.ValueOffset are
	// all uint32, so multiplying or adding them as plain int can overflow on 32-bit.
	// We compute in uint64 and bounds-check before converting back to int.
	// headerStart and headerSize are non-negative ints bounded by len(data).
	// Casting to uint64 is safe and intentional: subsequent arithmetic uses
	// uint32 fields (NLibs, LenStrings) that would overflow 32-bit int.
	entryStartU := uint64(headerStart) + uint64(headerSize) //nolint:gosec // G115: safe, see above
	stringTableStartU := entryStartU + uint64(header.NLibs)*newEntrySize
	dataEndU := stringTableStartU + uint64(header.LenStrings)

	dataLen := uint64(len(data))
	if dataEndU > dataLen {
		return nil, fmt.Errorf("%w: need %d bytes, have %d",
			errDataTruncated, dataEndU, dataLen)
	}

	// entryStartU <= dataEndU <= dataLen = len(data), so it fits in int.
	entryStart := int(entryStartU) //nolint:gosec // G115: bounded by len(data) above

	// Parse entries
	cache := &LDCache{
		entries: make(map[string]string, header.NLibs),
	}

	for i := uint32(0); i < header.NLibs; i++ {
		offset := entryStart + int(i)*newEntrySize
		entryReader := bytes.NewReader(data[offset:])

		var entry newCacheEntry
		if err := binary.Read(entryReader, binary.LittleEndian, &entry); err != nil {
			return nil, fmt.Errorf("failed to read cache entry %d: %w", i, err)
		}

		// Extract strings from string table; bounds are checked by extractCString.
		keyStartU := stringTableStartU + uint64(entry.KeyOffset)
		valueStartU := stringTableStartU + uint64(entry.ValueOffset)

		// Convert to int only after verifying the values fit within the data slice.
		// extractCString handles out-of-range offsets gracefully (returns ""),
		// but we cap at len(data) to avoid int overflow on 32-bit platforms.
		keyStart := int(min(keyStartU, dataLen))     //nolint:gosec // G115: capped by min(., dataLen) which fits in int
		valueStart := int(min(valueStartU, dataLen)) //nolint:gosec // G115: capped by min(., dataLen) which fits in int

		key := extractCString(data, keyStart)
		value := extractCString(data, valueStart)

		if key != "" && value != "" {
			// First entry wins (consistent with ld.so behavior)
			if _, exists := cache.entries[key]; !exists {
				cache.entries[key] = value
			}
		}
	}

	return cache, nil
}

// Lookup returns the resolved path for the given soname.
// Returns empty string if not found.
func (c *LDCache) Lookup(soname string) string {
	return c.entries[soname]
}

// extractCString extracts a null-terminated C string from data starting at offset.
func extractCString(data []byte, offset int) string {
	if offset < 0 || offset >= len(data) {
		return ""
	}
	end := bytes.IndexByte(data[offset:], 0)
	if end < 0 {
		return ""
	}
	return string(data[offset : offset+end])
}
