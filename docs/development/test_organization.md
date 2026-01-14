# Test Helper File Organization

Test helper files follow a two-tier classification system based on their scope and dependencies:

## Classification A: `testing/` Subdirectory (Cross-Package Helpers)

**Use for**: Test helpers and mocks used across multiple packages or that only use public APIs

```
<package>/
├── <implementation>.go
├── <implementation>_test.go
└── testing/
    ├── mocks.go              # Lightweight mocks (no external dependencies)
    ├── testify_mocks.go      # testify-based mocks (for complex scenarios)
    ├── mocks_test.go         # Tests for mock implementations
    └── helpers.go            # Test utility functions
```

**File Naming Rules:**
- **`testing/mocks.go`**: Simple mock implementations without external library dependencies
- **`testing/testify_mocks.go`**: Advanced mocks using stretchr/testify framework
- **`testing/mocks_test.go`**: Unit tests for mock implementations
- **`testing/helpers.go`**: Common test utility functions and setup helpers

**Package Naming:**
- All testing utilities use `package testing` within the `testing/` subdirectory
- Import as: `<module>/internal/<package>/testing`

## Classification B: Package-Level `test_helpers.go` (Internal Helpers)

**Use for**: Test helpers that must remain in the same package due to:
- Adding methods to package-internal types
- Using non-exported (private) package APIs
- Avoiding circular dependencies

```
<package>/
├── <implementation>.go
├── <implementation>_test.go
└── test_helpers.go           # Package-internal test helpers
```

**File Naming Rules:**
- **`test_helpers.go`**: Single file for package-internal test helpers
- If multiple helper categories needed: `test_helpers_<category>.go` (e.g., `test_helpers_group.go`)

**Package Naming:**
- Use the same package name as the production code
- Always include `//go:build test` build tag

## Guidelines for New Test Helpers

When adding new test helper code, follow this decision tree:

1. **Does the helper use only public APIs?**
   - Yes → Continue to step 2 (Classification A)
   - No → Continue to step 4 (likely Classification B)

2. **What type of test helper are you creating?** (Classification A - `testing/` subdirectory)
   - **Mock implementation** → Choose based on complexity:
     - Simple mock (no external dependencies) → `testing/mocks.go`
     - Complex mock (using testify/mock) → `testing/testify_mocks.go`
   - **Helper function** (setup, utilities, fixtures) → `testing/helpers.go`
   - **Mock tests** → `testing/mocks_test.go`

3. **Is the helper used by tests in other packages?**
   - Yes → Ensure it uses only public APIs, then place in appropriate `testing/` file (step 2)
   - No → Continue to step 4

4. **Package-internal considerations** (Classification B - `test_helpers.go`)
   Place in `test_helpers.go` if the helper:
   - Adds methods to package-internal types
   - Uses non-exported (private) package APIs
   - Would create circular dependencies if placed in `testing/` subdirectory
   - If multiple helper categories exist: use `test_helpers_<category>.go` (e.g., `test_helpers_group.go`)

**Build Tags:**
- All test helper files must include `//go:build test` at the top
- This ensures they are only compiled during test builds, not in production binaries

**Examples:**
- Mock interface implementation → `testing/mocks.go` or `testing/testify_mocks.go`
- Test setup helper function → `testing/helpers.go`
- Method on internal type → `test_helpers.go`
- Factory function using private constructor → `test_helpers.go`
