# Test Data for dynlibanalysis Package

This directory contains test data for the `dynlibanalysis` package.

## Files

### `ldcache_new_format.bin`

This file is **not** stored in the repository. Instead, the tests in `ldcache_test.go`
generate minimal synthetic `ld.so.cache` binary data in-memory using Go code.

The `parseLDCacheData` function is unexported but accessible from within the same package
(since tests use `package dynlibanalysis`), allowing direct testing with synthetic data
without requiring a pre-built binary file.

## Generating Test Data

To create a minimal new-format ld.so.cache for manual testing:

```go
// Build the binary data in your test:
// 1. Write the magic string "glibc-ld.so.cache1.1" (19 bytes, no null terminator)
// 2. Align to 4-byte boundary
// 3. Write newCacheHeader{NLibs: N, LenStrings: M}
// 4. Write N newCacheEntry structs
// 5. Write the string table (null-terminated key/value pairs)
```

See `ldcache_test.go` for concrete examples.
