//go:build test && darwin && cgo

package machodylib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDyldStructLayoutMatchesSDK verifies that the byte-offset and size constants
// hardcoded in dyld_extractor_darwin.go match the C struct layout in the committed
// snapshot of dyld_cache_format.h (testdata/dyld_headers/).
//
// The layout values are computed by the C compiler via cgo offsetof/sizeof in
// dyld_layout_darwin.go, so this test catches any mismatch between Go constants
// and the actual C struct — not just internal Go consistency.
//
// When Apple updates dyld_cache_format.h upstream:
//  1. CI detects the change via header diff (macos-dyld.yml).
//  2. Update testdata/dyld_headers/mach-o/dyld_cache_format.h (and fixup-chains.h if needed).
//  3. This test will fail if the Go constants in dyld_extractor_darwin.go need updating.
//  4. Update the Go constants and commit.
func TestDyldStructLayoutMatchesSDK(t *testing.T) {
	sdk := dyldSDKLayoutFromHeaders()

	t.Run("dyld_cache_header field offsets", func(t *testing.T) {
		assert.Equal(t, sdk.MappingOffset, int64(dyldHdrOffMappingOffset),
			"dyldHdrOffMappingOffset (mappingOffset)")
		assert.Equal(t, sdk.MappingCount, int64(dyldHdrOffMappingCount),
			"dyldHdrOffMappingCount (mappingCount)")
		assert.Equal(t, sdk.ImgTextOffset, int64(dyldHdrOffImgTextOffset),
			"dyldHdrOffImgTextOffset (imagesTextOffset)")
		assert.Equal(t, sdk.ImgTextCount, int64(dyldHdrOffImgTextCount),
			"dyldHdrOffImgTextCount (imagesTextCount)")
		assert.Equal(t, sdk.SubCacheOffset, int64(dyldHdrOffSubCacheOffset),
			"dyldHdrOffSubCacheOffset (subCacheArrayOffset)")
		assert.Equal(t, sdk.SubCacheCount, int64(dyldHdrOffSubCacheCount),
			"dyldHdrOffSubCacheCount (subCacheArrayCount)")
	})

	t.Run("struct sizes", func(t *testing.T) {
		assert.Equal(t, sdk.MappingInfoSize, int64(32),
			"dyld_cache_mapping_info size (mappingEntrySize in readSubCacheMapping)")
		assert.Equal(t, sdk.ImageTextSize, int64(32),
			"dyld_cache_image_text_info size (entrySize in findLibsystemKernelImage)")
		assert.Equal(t, sdk.SubcacheEntrySize, int64(56),
			"dyld_subcache_entry size (entrySize in buildSubCacheList)")
	})
}
