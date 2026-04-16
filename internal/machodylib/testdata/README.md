# machodylib testdata

This directory contains test fixtures for the `machodylib` package tests.

## Test Binary Generation (macOS only)

Test binaries for Mach-O analysis tests must be generated on macOS using
the provided build scripts or commands below. Generated test binaries are
committed to the repository for reproducibility.

### Simple dynamic Mach-O binary

```sh
# libfoo.dylib - simple shared library with no external dependencies
cat > /tmp/libfoo.c <<'EOF'
int foo(void) { return 42; }
EOF
clang -shared -o testdata/libfoo.dylib /tmp/libfoo.c \
  -install_name @rpath/libfoo.dylib

# libbar.dylib - depends on libfoo.dylib
cat > /tmp/libbar.c <<'EOF'
extern int foo(void);
int bar(void) { return foo() + 1; }
EOF
clang -shared -o testdata/libbar.dylib /tmp/libbar.c \
  -install_name @rpath/libbar.dylib \
  -L testdata -lfoo \
  -Xlinker -rpath -Xlinker @loader_path

# dynamic_binary - a binary that links libbar.dylib
cat > /tmp/main.c <<'EOF'
extern int bar(void);
int main(void) { return bar(); }
EOF
clang -o testdata/dynamic_binary /tmp/main.c \
  -L testdata -lbar \
  -Xlinker -rpath -Xlinker @executable_path/testdata
```

### Fat binary (Universal)

```sh
# Requires access to both x86_64 and arm64 toolchains (e.g., on Apple Silicon Mac).
# Cross-compile for x86_64
clang -target x86_64-apple-macos11 -o /tmp/thin_x86_64 /tmp/main.c
# Native arm64
clang -target arm64-apple-macos11 -o /tmp/thin_arm64 /tmp/main.c
# Combine
lipo -create /tmp/thin_x86_64 /tmp/thin_arm64 -output testdata/fat_binary
```

## Test Data Inventory

| File | Description |
|------|-------------|
| `libfoo.dylib` | Simple dynamic library (no external deps, @rpath install name) |
| `libbar.dylib` | Dynamic library depending on libfoo.dylib via @loader_path rpath |
| `dynamic_binary` | Mach-O binary with transitive dependency on libfoo via libbar |
| `fat_binary` | Fat (universal) binary containing arm64 and x86_64 slices |

