//go:build darwin

package machodylib

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Sentinel errors for dyld cache parsing.
var (
	errInvalidDyldMagic    = errors.New("unexpected dyld cache magic")
	errImplausibleCount    = errors.New("implausible sub-cache count")
	errInvalidMachOMagic   = errors.New("unexpected Mach-O magic")
	errImplausibleSizeCmds = errors.New("implausible sizeofcmds")
	errImplausibleReadSize = errors.New("implausible read size")
	errUnsupportedType     = errors.New("unsupported readAt type")
	errSymbolTableExceeded = errors.New("symbol table exceeds maximum allowed entries")
)

// Load command types and layout constants for Mach-O parsing.
const (
	lcSegment64         = 0x19 // LC_SEGMENT_64
	lcSymtab            = 0x2  // LC_SYMTAB
	minLoadCmdSize      = 8    // minimum valid cmdsize
	seg64SectionsOffset = 72   // byte offset of first section_64 within LC_SEGMENT_64
	section64Size       = 80   // sizeof(section_64)
	section64FileOffset = 48   // offset of fileoff field within section_64
	machoHeaderSize     = 32   // sizeof(mach_header_64)
)

// dyld shared cache header field offsets.
// imagesTextOffset/Count have been stable since macOS 10.15.
// subCacheArrayOffset/Count were added in macOS 13 (Ventura).
// All values verified against macOS 15/16 (Darwin 25.x) dyld cache.
const (
	dyldHdrOffMappingOffset  = 16  // uint32: offset to dyld_cache_mapping_info array
	dyldHdrOffMappingCount   = 20  // uint32: number of mapping entries
	dyldHdrOffImgTextOffset  = 136 // uint64: offset to dyld_cache_image_text_info array
	dyldHdrOffImgTextCount   = 144 // uint64: count of image text entries
	dyldHdrOffSubCacheOffset = 392 // uint32: offset to dyld_subcache_entry array (macOS 13+)
	dyldHdrOffSubCacheCount  = 396 // uint32: number of sub-cache entries
)

// dyldCacheMagic is the expected magic prefix for dyld shared cache files.
const dyldCacheMagic = "dyld_v1"

// dyldSharedCachePaths is the ordered list of dyld shared cache paths to try.
// macOS 13 (Ventura) and later store the cache under the Cryptexes volume;
// older releases used /System/Library/dyld directly.
var dyldSharedCachePaths = []string{
	"/System/Volumes/Preboot/Cryptexes/OS/System/Library/dyld/dyld_shared_cache_arm64e",
	"/System/Library/dyld/dyld_shared_cache_arm64e",
	"/System/Library/dyld/dyld_shared_cache_arm64",
}

// dyldMappingInfo mirrors dyld_cache_mapping_info (on-disk: addr+size+fileOffset+maxProt+initProt = 28 bytes).
type dyldMappingInfo struct {
	Address    uint64
	Size       uint64
	FileOffset uint64
}

// dyldImgTextInfo mirrors dyld_cache_image_text_info (32 bytes on disk).
type dyldImgTextInfo struct {
	UUID            [16]byte
	LoadAddress     uint64
	TextSegmentSize uint32
	PathFileOffset  uint32
}

// subCacheFile describes a sub-cache file path and its VM base address.
type subCacheFile struct {
	path      string
	vmBase    uint64 // base VM address = main cache base + CacheVMOffset
	cacheBase uint64 // VM address of the main cache's first mapping
}

// ExtractLibSystemKernelFromDyldCache extracts libsystem_kernel.dylib from the
// dyld shared cache.
//
// On failure (cache not found, image not found, or extraction failure),
// returns nil, nil so the caller can fall back to symbol-name matching.
// Logs at slog.Info level for all non-error fallback conditions.
func ExtractLibSystemKernelFromDyldCache() (*LibSystemKernelBytes, error) {
	var cachePath string
	for _, p := range dyldSharedCachePaths {
		if _, err := os.Stat(p); err == nil {
			cachePath = p
			break
		}
	}
	if cachePath == "" {
		slog.Info("dyld shared cache not found; applying fallback", "tried", dyldSharedCachePaths)
		return nil, nil
	}

	machoBytes, err := extractLibsystemKernel(cachePath)
	if err != nil {
		slog.Info("Failed to extract libsystem_kernel from dyld cache; applying fallback",
			"path", cachePath, "error", err)
		return nil, nil
	}
	if machoBytes == nil {
		slog.Info("libsystem_kernel.dylib not found in dyld shared cache; applying fallback",
			"path", cachePath)
		return nil, nil
	}

	h := sha256.Sum256(machoBytes)
	return &LibSystemKernelBytes{
		Data: machoBytes,
		Hash: fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:])),
	}, nil
}

// extractLibsystemKernel performs the actual extraction.
func extractLibsystemKernel(cachePath string) ([]byte, error) {
	f, err := os.Open(cachePath) //nolint:gosec // cachePath comes from the trusted dyldSharedCachePaths list
	if err != nil {
		return nil, fmt.Errorf("open dyld cache: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Verify magic.
	var magic [16]byte
	if _, err := f.ReadAt(magic[:], 0); err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}
	if !strings.HasPrefix(string(magic[:]), dyldCacheMagic) {
		return nil, fmt.Errorf("got %q: %w", string(magic[:7]), errInvalidDyldMagic)
	}

	// Read the main mapping to get the cache base VM address.
	mainMapping, err := readMainMapping(f)
	if err != nil {
		return nil, err
	}
	cacheBase := mainMapping.Address

	// Find libsystem_kernel.dylib in the image text info array.
	libImg, err := findLibsystemKernelImage(f)
	if err != nil {
		return nil, err
	}
	if libImg == nil {
		return nil, nil // not found
	}

	// Build the sub-cache file list from the sub-cache entry array.
	subCaches, err := buildSubCacheList(f, cachePath, cacheBase)
	if err != nil {
		return nil, err
	}

	// Locate the sub-cache file that contains the library's __TEXT segment.
	textFile, textMapping, err := findSubCacheForAddr(libImg.LoadAddress, cachePath, mainMapping, subCaches)
	if err != nil {
		return nil, err
	}
	if textFile == "" {
		return nil, nil
	}

	// Compute the library's file offset within the text sub-cache file.
	libFileOff := textMapping.FileOffset + (libImg.LoadAddress - textMapping.Address)

	// Read the Mach-O header and load commands.
	hdr, lcData, err := readMachOHeader(textFile, libFileOff)
	if err != nil {
		return nil, err
	}

	// Parse load commands to find __TEXT, __LINKEDIT, and LC_SYMTAB.
	textSeg, linkeditSeg, symtab := parseSegmentsAndSymtab(lcData, hdr.byteOrder)
	if textSeg == nil || linkeditSeg == nil || symtab == nil {
		return nil, nil
	}

	// Find the sub-cache file that contains __LINKEDIT.
	// The __LINKEDIT vmaddr may fall exactly at a mapping boundary (the .dyldlinkedit
	// sub-cache maps only a small "header" region), so use vmBase-range lookup rather
	// than mapping coverage. The fileOff from LC_SEGMENT_64 is already relative to
	// the sub-cache file and gives the correct offset directly.
	linkeditFile := findSubCacheFileForAddr(linkeditSeg.vmaddr, cachePath, mainMapping, subCaches)
	if linkeditFile == "" {
		return nil, nil
	}
	linkeditFileOffInSub := linkeditSeg.fileOff

	// Read the __TEXT segment data from the text sub-cache.
	textData, err := readFileRange(textFile, textSeg.fileOff, textSeg.fileSize)
	if err != nil {
		return nil, fmt.Errorf("read __TEXT: %w", err)
	}

	// Read and compact the symbol table from the LINKEDIT sub-cache.
	compactSyms, compactStrtab, err := buildCompactSymtab(
		linkeditFile,
		linkeditFileOffInSub+uint64(symtab.symoff),
		symtab.nsyms,
		linkeditFileOffInSub+uint64(symtab.stroff),
	)
	if err != nil {
		return nil, fmt.Errorf("build compact symtab: %w", err)
	}

	// Reconstruct a standalone Mach-O.
	return reconstructMachO(hdr, lcData, textSeg, textData, compactSyms, compactStrtab), nil
}

// readMainMapping reads the first (or only) mapping info from the main cache file.
func readMainMapping(f *os.File) (dyldMappingInfo, error) {
	var mapOff uint32
	if err := readAt(f, dyldHdrOffMappingOffset, &mapOff); err != nil {
		return dyldMappingInfo{}, fmt.Errorf("read mappingOffset: %w", err)
	}
	var m dyldMappingInfo
	var raw [28]byte
	if _, err := f.ReadAt(raw[:], int64(mapOff)); err != nil {
		return dyldMappingInfo{}, fmt.Errorf("read main mapping: %w", err)
	}
	m.Address = binary.LittleEndian.Uint64(raw[0:])
	m.Size = binary.LittleEndian.Uint64(raw[8:])
	m.FileOffset = binary.LittleEndian.Uint64(raw[16:])
	return m, nil
}

// findLibsystemKernelImage scans the image text info array for libsystem_kernel.dylib.
func findLibsystemKernelImage(f *os.File) (*dyldImgTextInfo, error) {
	var imgTextOff uint64
	var imgTextCount uint64
	if err := readAt(f, dyldHdrOffImgTextOffset, &imgTextOff); err != nil {
		return nil, fmt.Errorf("read imagesTextOffset: %w", err)
	}
	if err := readAt(f, dyldHdrOffImgTextCount, &imgTextCount); err != nil {
		return nil, fmt.Errorf("read imagesTextCount: %w", err)
	}
	if imgTextCount == 0 || imgTextOff == 0 {
		return nil, nil
	}

	// Sanity check: reject implausible image counts before looping.
	const maxImageTextEntries = 8192
	if imgTextCount > maxImageTextEntries {
		return nil, fmt.Errorf("%d image text entries exceeds limit %d: %w", imgTextCount, maxImageTextEntries, errImplausibleCount)
	}

	const entrySize = 32 // sizeof(dyld_cache_image_text_info)
	for i := uint64(0); i < imgTextCount; i++ {
		off := int64(imgTextOff) + int64(i)*entrySize
		var raw [32]byte
		if _, err := f.ReadAt(raw[:], off); err != nil {
			return nil, fmt.Errorf("read image text info[%d]: %w", i, err)
		}
		img := dyldImgTextInfo{
			LoadAddress:     binary.LittleEndian.Uint64(raw[16:]),
			TextSegmentSize: binary.LittleEndian.Uint32(raw[24:]),
			PathFileOffset:  binary.LittleEndian.Uint32(raw[28:]),
		}
		copy(img.UUID[:], raw[:16])

		// Read the path string.
		var pathBuf [128]byte
		n, err := f.ReadAt(pathBuf[:], int64(img.PathFileOffset))
		if n == 0 {
			if err != nil && err.Error() != "EOF" {
				slog.Warn("failed to read image path", "offset", img.PathFileOffset, "error", err)
			}
			continue
		}
		nullIdx := bytes.IndexByte(pathBuf[:n], 0)
		if nullIdx < 0 {
			nullIdx = n
		}
		if string(pathBuf[:nullIdx]) == libsystemKernelInstallName {
			return &img, nil
		}
	}
	return nil, nil
}

// buildSubCacheList reads sub-cache entries and constructs file paths.
// Returns an empty slice when there are no sub-caches (pre-Ventura single-file cache).
func buildSubCacheList(f *os.File, mainPath string, cacheBase uint64) ([]subCacheFile, error) {
	var subCacheOff, subCacheCount uint32
	if err := readAt(f, dyldHdrOffSubCacheOffset, &subCacheOff); err != nil {
		return nil, fmt.Errorf("read subCacheArrayOffset: %w", err)
	}
	if err := readAt(f, dyldHdrOffSubCacheCount, &subCacheCount); err != nil {
		return nil, fmt.Errorf("read subCacheArrayCount: %w", err)
	}
	if subCacheCount == 0 {
		return nil, nil
	}

	// Sanity check: reject implausible counts before allocating.
	const maxSubCaches = 256
	if subCacheCount > maxSubCaches {
		return nil, fmt.Errorf("%d sub-caches: %w", subCacheCount, errImplausibleCount)
	}

	const entrySize = 56 // sizeof(dyld_subcache_entry)
	result := make([]subCacheFile, 0, subCacheCount)
	for i := uint32(0); i < subCacheCount; i++ {
		off := int64(subCacheOff) + int64(i)*entrySize
		var raw [56]byte
		if _, err := f.ReadAt(raw[:], off); err != nil {
			return nil, fmt.Errorf("read subCacheEntry[%d]: %w", i, err)
		}
		vmOff := binary.LittleEndian.Uint64(raw[16:])
		suffix := strings.TrimRight(string(raw[24:56]), "\x00")

		subPath := mainPath + suffix
		result = append(result, subCacheFile{
			path:      subPath,
			vmBase:    cacheBase + vmOff,
			cacheBase: cacheBase,
		})
	}
	return result, nil
}

// findSubCacheForAddr finds the sub-cache file and its relevant mapping that
// covers the given VM address.  Falls back to the main cache mapping when
// no sub-caches are defined (pre-Ventura) or when the address is in the main file.
func findSubCacheForAddr(vmAddr uint64, mainPath string, mainMapping dyldMappingInfo, subCaches []subCacheFile) (string, dyldMappingInfo, error) {
	// Check if the address is in the main cache.
	if vmAddr >= mainMapping.Address && vmAddr < mainMapping.Address+mainMapping.Size {
		return mainPath, mainMapping, nil
	}

	for i, sc := range subCaches {
		// Determine the VM range of this sub-cache: from sc.vmBase to the next one (or ∞).
		var vmEnd uint64
		if i+1 < len(subCaches) {
			vmEnd = subCaches[i+1].vmBase
		} else {
			vmEnd = ^uint64(0)
		}
		if vmAddr < sc.vmBase || vmAddr >= vmEnd {
			continue
		}

		// Found the sub-cache. Read its mapping to get the file offset.
		mapping, err := readSubCacheMapping(sc.path, vmAddr)
		if err != nil {
			return "", dyldMappingInfo{}, fmt.Errorf("read sub-cache mapping %s: %w", sc.path, err)
		}
		if mapping == nil {
			continue // address not covered by this sub-cache's mappings
		}
		return sc.path, *mapping, nil
	}
	return "", dyldMappingInfo{}, nil
}

// findSubCacheFileForAddr returns the path of the sub-cache whose vmBase range contains
// vmAddr, without requiring that vmAddr fall within a mapped region.
// This is used for segments (like __LINKEDIT in .dyldlinkedit sub-caches) whose vmaddr
// may lie just beyond the sub-cache's small header mapping.
// Returns empty string when vmAddr is in the main file or no sub-cache covers it.
func findSubCacheFileForAddr(vmAddr uint64, mainPath string, mainMapping dyldMappingInfo, subCaches []subCacheFile) string {
	if vmAddr >= mainMapping.Address && vmAddr < mainMapping.Address+mainMapping.Size {
		return mainPath
	}
	for i, sc := range subCaches {
		var vmEnd uint64
		if i+1 < len(subCaches) {
			vmEnd = subCaches[i+1].vmBase
		} else {
			vmEnd = ^uint64(0)
		}
		if vmAddr >= sc.vmBase && vmAddr < vmEnd {
			return sc.path
		}
	}
	return ""
}

// readSubCacheMapping opens a sub-cache file and finds the mapping that contains vmAddr.
func readSubCacheMapping(path string, vmAddr uint64) (*dyldMappingInfo, error) {
	f, err := os.Open(path) //nolint:gosec // path is constructed from trusted dyldSharedCachePaths + validated suffix
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var mapOff, mapCount uint32
	if err := readAt(f, dyldHdrOffMappingOffset, &mapOff); err != nil {
		return nil, err
	}
	if err := readAt(f, dyldHdrOffMappingCount, &mapCount); err != nil {
		return nil, err
	}

	const mappingEntrySize = 28
	for i := uint32(0); i < mapCount; i++ {
		off := int64(mapOff) + int64(i)*mappingEntrySize
		var raw [28]byte
		if _, err := f.ReadAt(raw[:], off); err != nil {
			return nil, err
		}
		m := dyldMappingInfo{
			Address:    binary.LittleEndian.Uint64(raw[0:]),
			Size:       binary.LittleEndian.Uint64(raw[8:]),
			FileOffset: binary.LittleEndian.Uint64(raw[16:]),
		}
		if vmAddr >= m.Address && vmAddr < m.Address+m.Size {
			return &m, nil
		}
	}
	return nil, nil
}

// machoHeader holds the fields from a Mach-O header that we need.
type machoHeader struct {
	magic      uint32
	cputype    uint32
	cpusubtype uint32
	filetype   uint32
	ncmds      uint32
	sizeofcmds uint32
	flags      uint32
	reserved   uint32 // arm64 has 8-word header
	byteOrder  binary.ByteOrder
}

// segInfo holds parsed LC_SEGMENT_64 fields we need.
type segInfo struct {
	name     string
	vmaddr   uint64
	vmsize   uint64
	fileOff  uint64
	fileSize uint64
	nsects   uint32
	lcOffset int // offset of this LC within lcData
}

// symtabInfo holds parsed LC_SYMTAB fields.
type symtabInfo struct {
	symoff   uint32
	nsyms    uint32
	stroff   uint32
	strsize  uint32
	lcOffset int
}

// readMachOHeader reads and returns the Mach-O header and load command bytes.
func readMachOHeader(path string, fileOff uint64) (*machoHeader, []byte, error) {
	f, err := os.Open(path) //nolint:gosec // path is from trusted sub-cache list
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	var raw [32]byte
	if _, err := f.ReadAt(raw[:], int64(fileOff)); err != nil { //nolint:gosec // G115: fileOff is a cache file offset (< tens of GB, well within int64)
		return nil, nil, fmt.Errorf("read mach-o header: %w", err)
	}

	magic := binary.LittleEndian.Uint32(raw[0:])
	const macho64Magic = 0xFEEDFACF // little-endian arm64/x86-64 only; dyld caches never contain BE slices
	if magic != macho64Magic {
		return nil, nil, fmt.Errorf("0x%08x at 0x%x: %w", magic, fileOff, errInvalidMachOMagic)
	}

	hdr := &machoHeader{
		magic:      magic,
		cputype:    binary.LittleEndian.Uint32(raw[4:]),
		cpusubtype: binary.LittleEndian.Uint32(raw[8:]),
		filetype:   binary.LittleEndian.Uint32(raw[12:]),
		ncmds:      binary.LittleEndian.Uint32(raw[16:]),
		sizeofcmds: binary.LittleEndian.Uint32(raw[20:]),
		flags:      binary.LittleEndian.Uint32(raw[24:]),
		reserved:   binary.LittleEndian.Uint32(raw[28:]),
		byteOrder:  binary.LittleEndian,
	}

	if hdr.sizeofcmds == 0 || hdr.sizeofcmds > 1<<20 {
		return nil, nil, fmt.Errorf("%d: %w", hdr.sizeofcmds, errImplausibleSizeCmds)
	}

	lcData := make([]byte, hdr.sizeofcmds)
	if _, err := f.ReadAt(lcData, int64(fileOff)+machoHeaderSize); err != nil { //nolint:gosec // G115: fileOff is a cache file offset (< tens of GB)
		return nil, nil, fmt.Errorf("read load commands: %w", err)
	}
	return hdr, lcData, nil
}

// parseSegmentsAndSymtab extracts __TEXT, __LINKEDIT, and LC_SYMTAB from load command bytes.
func parseSegmentsAndSymtab(lcData []byte, bo binary.ByteOrder) (*segInfo, *segInfo, *symtabInfo) {
	var textSeg, linkeditSeg *segInfo
	var symtab *symtabInfo

	offset := 0
	for offset+8 <= len(lcData) {
		cmd := bo.Uint32(lcData[offset:])
		cmdsize := bo.Uint32(lcData[offset+4:])
		if cmdsize < 8 || offset+int(cmdsize) > len(lcData) {
			break
		}

		switch cmd {
		case lcSegment64:
			if offset+seg64SectionsOffset > len(lcData) {
				break
			}
			name := strings.TrimRight(string(lcData[offset+8:offset+24]), "\x00")
			si := &segInfo{
				name:     name,
				vmaddr:   bo.Uint64(lcData[offset+24:]),
				vmsize:   bo.Uint64(lcData[offset+32:]),
				fileOff:  bo.Uint64(lcData[offset+40:]),
				fileSize: bo.Uint64(lcData[offset+48:]),
				nsects:   bo.Uint32(lcData[offset+64:]),
				lcOffset: offset,
			}
			switch name {
			case "__TEXT":
				textSeg = si
			case "__LINKEDIT":
				linkeditSeg = si
			}

		case lcSymtab:
			if offset+24 > len(lcData) {
				break
			}
			symtab = &symtabInfo{
				symoff:   bo.Uint32(lcData[offset+8:]),
				nsyms:    bo.Uint32(lcData[offset+12:]),
				stroff:   bo.Uint32(lcData[offset+16:]),
				strsize:  bo.Uint32(lcData[offset+20:]),
				lcOffset: offset,
			}
		}
		offset += int(cmdsize)
	}
	return textSeg, linkeditSeg, symtab
}

// readFileRange reads size bytes from path starting at offset.
func readFileRange(path string, offset, size uint64) ([]byte, error) {
	if size == 0 || size > 64<<20 { // sanity: max 64 MB
		return nil, fmt.Errorf("%d bytes from %s: %w", size, path, errImplausibleReadSize)
	}
	f, err := os.Open(path) //nolint:gosec // path is from trusted sub-cache list
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	data := make([]byte, size)
	if _, err := f.ReadAt(data, int64(offset)); err != nil { //nolint:gosec // G115: offset is a file offset within a dyld cache (< tens of GB)
		return nil, err
	}
	return data, nil
}

// nlist64Size is the size of a nlist_64 entry on disk.
const nlist64Size = 16

// buildCompactSymtab reads the symbol table entries for this library and builds
// a compact symbol table + string table containing only the referenced symbols.
// symFileOff and strFileOff are absolute file offsets within the LINKEDIT sub-cache.
func buildCompactSymtab(linkeditPath string, symFileOff uint64, nsyms uint32, strFileOff uint64) ([]byte, []byte, error) {
	const maxSymbols = 1 << 17 // 131072 symbols
	if nsyms == 0 {
		// No symbols: return minimal valid symbol table.
		return []byte{}, []byte{0}, nil
	}
	if nsyms > maxSymbols {
		slog.Warn("symbol table truncated: too many symbols", "nsyms", nsyms, "limit", maxSymbols) //nolint:gosec // Static hardcoded message, no injection risk
		return nil, nil, fmt.Errorf("%d entries: %w", nsyms, errSymbolTableExceeded)
	}

	f, err := os.Open(linkeditPath) //nolint:gosec // path is from trusted sub-cache list
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	// Read all nlist_64 entries.
	symData := make([]byte, uint64(nsyms)*nlist64Size)
	if _, err := f.ReadAt(symData, int64(symFileOff)); err != nil { //nolint:gosec // G115: symFileOff is a file offset in dyld cache (< tens of GB)
		return nil, nil, fmt.Errorf("read symtab entries: %w", err)
	}

	// Build compact string table: start with a null byte (index 0 = empty string).
	var strtabBuf bytes.Buffer
	strtabBuf.WriteByte(0)

	// Map from original string offset to new string offset.
	strIndexMap := make(map[uint32]uint32)

	// Patch n_strx in each symbol entry.
	compactSyms := make([]byte, len(symData))
	copy(compactSyms, symData)

	var nameBuf [512]byte
	for i := uint32(0); i < nsyms; i++ {
		entOff := i * nlist64Size
		origStrx := binary.LittleEndian.Uint32(compactSyms[entOff:])

		newStrx, ok := strIndexMap[origStrx]
		if !ok {
			// Read the null-terminated symbol name from the shared strtab.
			n, _ := f.ReadAt(nameBuf[:], int64(strFileOff)+int64(origStrx)) //nolint:gosec // G115: strFileOff is a file offset (< tens of GB); origStrx is a strtab offset (< 32-bit)
			if n == 0 {
				newStrx = 0 // fall back to empty string
			} else {
				end := bytes.IndexByte(nameBuf[:n], 0)
				if end < 0 {
					end = n
				}
				newStrx = uint32(strtabBuf.Len()) //nolint:gosec // G115: string table for ~1700 symbols fits in uint32
				strtabBuf.Write(nameBuf[:end])
				strtabBuf.WriteByte(0)
			}
			strIndexMap[origStrx] = newStrx
		}
		binary.LittleEndian.PutUint32(compactSyms[entOff:], newStrx)
	}

	return compactSyms, strtabBuf.Bytes(), nil
}

// reconstructMachO builds a standalone Mach-O byte slice that debug/macho.NewFile can parse.
// It contains only the __TEXT and a compact __LINKEDIT with the library's symbols.
func reconstructMachO(
	hdr *machoHeader,
	lcData []byte,
	textSeg *segInfo,
	textData []byte,
	compactSyms []byte,
	compactStrtab []byte,
) []byte {
	const pageSize = 0x4000

	// Layout:
	//   [0..machoHeaderSize)    mach_header_64
	//   [machoHeaderSize..lcEnd)     load commands (patched)
	//   [textStart..textEnd)    __TEXT segment data (page-aligned)
	//   [linkeditStart..)       compact __LINKEDIT (symtab + strtab)

	lcEnd := machoHeaderSize + len(lcData)
	textStart := align(lcEnd, pageSize)
	textEnd := textStart + len(textData)
	linkeditStart := align(textEnd, pageSize)

	nsyms := len(compactSyms) / nlist64Size
	newSymoff := uint32(linkeditStart)                            //nolint:gosec // G115: linkeditStart is < a few MB (reconstructed Mach-O)
	newStroff := uint32(linkeditStart) + uint32(len(compactSyms)) //nolint:gosec // G115: same; len(compactSyms) is bounded by nsyms*16
	newLinkeditSize := uint64(len(compactSyms) + len(compactStrtab))

	// Patch a copy of the load commands.
	patchedLC := make([]byte, len(lcData))
	copy(patchedLC, lcData)
	patchLoadCommands(
		patchedLC,
		uint64(textStart), textSeg.fileOff, //nolint:gosec // G115: textStart is a small positive int offset
		uint64(linkeditStart), newLinkeditSize, //nolint:gosec // G115: linkeditStart is a small positive int offset
		newSymoff, uint32(nsyms), newStroff, uint32(len(compactStrtab)), //nolint:gosec // G115: nsyms and len are bounded
	)

	// Build the output buffer.
	totalSize := linkeditStart + int(newLinkeditSize) //nolint:gosec // G115: newLinkeditSize is len of two small slices
	out := make([]byte, totalSize)

	// Write mach_header_64.
	bo := hdr.byteOrder
	bo.PutUint32(out[0:], hdr.magic)
	bo.PutUint32(out[4:], hdr.cputype)
	bo.PutUint32(out[8:], hdr.cpusubtype)
	bo.PutUint32(out[12:], hdr.filetype)
	bo.PutUint32(out[16:], hdr.ncmds)
	bo.PutUint32(out[20:], uint32(len(patchedLC))) //nolint:gosec // G115: patchedLC is a copy of load commands (< 1 MB)
	bo.PutUint32(out[24:], hdr.flags)
	bo.PutUint32(out[28:], hdr.reserved)

	// Write patched load commands.
	copy(out[machoHeaderSize:], patchedLC)

	// Write __TEXT segment data.
	copy(out[textStart:], textData)

	// Write compact LINKEDIT.
	copy(out[linkeditStart:], compactSyms)
	copy(out[int(newStroff):], compactStrtab)

	return out
}

// patchLoadCommands modifies load command bytes in-place to reflect the new file layout.
// Note: dyld caches are always little-endian; the bo parameter is present for reading but
// patching always uses little-endian writes (per the invariant in readMachOHeader).
func patchLoadCommands(
	lcData []byte,
	newTextFileOff, oldTextFileOff uint64,
	newLinkeditFileOff, newLinkeditFileSize uint64,
	newSymoff, newNsyms, newStroff, newStrsize uint32,
) {
	offset := 0
	for offset+8 <= len(lcData) {
		cmd := binary.LittleEndian.Uint32(lcData[offset:])
		cmdsize := binary.LittleEndian.Uint32(lcData[offset+4:])
		if cmdsize < minLoadCmdSize {
			break
		}

		switch cmd {
		case lcSegment64:
			if offset+seg64SectionsOffset > len(lcData) {
				break
			}
			name := strings.TrimRight(string(lcData[offset+8:offset+24]), "\x00")

			switch name {
			case "__TEXT":
				// Update segment fileoff.
				binary.LittleEndian.PutUint64(lcData[offset+40:], newTextFileOff)
				// Update section offsets (each section_64 is section64Size bytes, starting at offset+seg64SectionsOffset).
				nsects := binary.LittleEndian.Uint32(lcData[offset+64:])
				for s := uint32(0); s < nsects; s++ {
					sBase := offset + seg64SectionsOffset + int(s)*section64Size //nolint:gosec // G115: s is bounded by nsects < 256
					if sBase+section64FileOffset+4 > len(lcData) {
						break
					}
					oldSectOff := binary.LittleEndian.Uint32(lcData[sBase+section64FileOffset:])
					if oldSectOff == 0 {
						continue // section with no file data (e.g. zerofill)
					}
					newSectOff := uint32(newTextFileOff) + (oldSectOff - uint32(oldTextFileOff)) //nolint:gosec // G115: newTextFileOff fits in uint32 (reconstructed Mach-O is < 4GB)
					binary.LittleEndian.PutUint32(lcData[sBase+section64FileOffset:], newSectOff)
				}

			case "__LINKEDIT":
				binary.LittleEndian.PutUint64(lcData[offset+40:], newLinkeditFileOff)
				binary.LittleEndian.PutUint64(lcData[offset+48:], newLinkeditFileSize)
			}

		case lcSymtab:
			if offset+24 > len(lcData) {
				break
			}
			binary.LittleEndian.PutUint32(lcData[offset+8:], newSymoff)
			binary.LittleEndian.PutUint32(lcData[offset+12:], newNsyms)
			binary.LittleEndian.PutUint32(lcData[offset+16:], newStroff)
			binary.LittleEndian.PutUint32(lcData[offset+20:], newStrsize)
		}

		offset += int(cmdsize)
	}
}

// readAt reads a little-endian value from the file at the given offset.
// dst must be a pointer to uint32 or uint64.
func readAt(f *os.File, offset int64, dst any) error {
	switch v := dst.(type) {
	case *uint32:
		var buf [4]byte
		if _, err := f.ReadAt(buf[:], offset); err != nil {
			return err
		}
		*v = binary.LittleEndian.Uint32(buf[:])
	case *uint64:
		var buf [8]byte
		if _, err := f.ReadAt(buf[:], offset); err != nil {
			return err
		}
		*v = binary.LittleEndian.Uint64(buf[:])
	default:
		return fmt.Errorf("%T: %w", dst, errUnsupportedType)
	}
	return nil
}

// align rounds n up to the nearest multiple of alignment.
func align(n, alignment int) int {
	return (n + alignment - 1) &^ (alignment - 1)
}
