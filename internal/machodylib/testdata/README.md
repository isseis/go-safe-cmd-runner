# machodylib testdata

This directory holds test fixtures for `internal/machodylib`.

## Synthetic fixtures (generated in-process)

Most unit tests build minimal Mach-O binaries in-process using the
`buildMachOWithDeps` / `buildFatBinaryFromSlices` helpers defined in
`analyzer_test.go`.  These helpers produce 64-bit little-endian Mach-O
headers with the desired LC_LOAD_DYLIB / LC_LOAD_WEAK_DYLIB / LC_RPATH load
commands and write them to `t.TempDir()`.  No pre-built files in this
directory are required for these tests.

## Compiled fixtures (optional, macOS + clang required)

For higher-fidelity testing against real linker output the tests in
`internal/verification/manager_macho_test.go` compile a minimal shared
library and binary via `clang` at test time.  The helper
`findNonDyldCacheMachOBinary` produces:

```
<tmpdir>/libfoo.dylib   – install name @rpath/libfoo.dylib
<tmpdir>/testbin        – links libfoo; rpath = <tmpdir>
```

To reproduce the layout manually:

```sh
clang -shared -o libfoo.dylib libfoo.c -install_name @rpath/libfoo.dylib
clang -o testbin main.c -L. -lfoo -Xlinker -rpath -Xlinker $(pwd)
```

Where `libfoo.c` contains a minimal exported function and `main.c` calls it.

## Build tags

All tests in this package require both the `test` and `darwin` build tags:

```sh
go test -tags "test darwin" ./internal/machodylib/...
```

Tests that depend on `clang` skip automatically when the tool is not present.
