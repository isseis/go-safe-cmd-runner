//go:build darwin && cgo

package machodylib

/*
#cgo CFLAGS: -I./testdata/dyld_headers
#include <mach-o/dyld_cache_format.h>
#include <stddef.h>

static const size_t sdk_off_mappingOffset  = offsetof(struct dyld_cache_header, mappingOffset);
static const size_t sdk_off_mappingCount   = offsetof(struct dyld_cache_header, mappingCount);
static const size_t sdk_off_imgsTextOffset = offsetof(struct dyld_cache_header, imagesTextOffset);
static const size_t sdk_off_imgsTextCount  = offsetof(struct dyld_cache_header, imagesTextCount);
static const size_t sdk_off_subCacheOffset = offsetof(struct dyld_cache_header, subCacheArrayOffset);
static const size_t sdk_off_subCacheCount  = offsetof(struct dyld_cache_header, subCacheArrayCount);

static const size_t sdk_sz_mappingInfo    = sizeof(struct dyld_cache_mapping_info);
static const size_t sdk_sz_imageTextInfo  = sizeof(struct dyld_cache_image_text_info);
static const size_t sdk_sz_subcacheEntry  = sizeof(struct dyld_subcache_entry);
*/
import "C"

// dyldSDKLayout holds struct layout values derived from the committed snapshot
// of dyld_cache_format.h via cgo offsetof/sizeof.
type dyldSDKLayout struct {
	MappingOffset     int64
	MappingCount      int64
	ImgTextOffset     int64
	ImgTextCount      int64
	SubCacheOffset    int64
	SubCacheCount     int64
	MappingInfoSize   int64
	ImageTextSize     int64
	SubcacheEntrySize int64
}

// dyldSDKLayoutFromHeaders returns layout values computed by the C compiler from
// the committed snapshot of dyld_cache_format.h in testdata/dyld_headers/.
// Called only from TestDyldStructLayoutMatchesSDK; the linker eliminates it in
// non-test builds.
func dyldSDKLayoutFromHeaders() dyldSDKLayout {
	return dyldSDKLayout{
		MappingOffset:     int64(C.sdk_off_mappingOffset),
		MappingCount:      int64(C.sdk_off_mappingCount),
		ImgTextOffset:     int64(C.sdk_off_imgsTextOffset),
		ImgTextCount:      int64(C.sdk_off_imgsTextCount),
		SubCacheOffset:    int64(C.sdk_off_subCacheOffset),
		SubCacheCount:     int64(C.sdk_off_subCacheCount),
		MappingInfoSize:   int64(C.sdk_sz_mappingInfo),
		ImageTextSize:     int64(C.sdk_sz_imageTextInfo),
		SubcacheEntrySize: int64(C.sdk_sz_subcacheEntry),
	}
}
