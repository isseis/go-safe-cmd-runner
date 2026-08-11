# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Quick Links

**Development Guides:**
- Requirements and Acceptance Criteria Process: [requirements_process.md](docs/dev/developer_guide/requirements_process.md) - Process for implementing new features
- Test Organization Guide: [test_organization.md](docs/dev/developer_guide/test_organization.md) - Test helper file organization
- [Package Reference](docs/dev/developer_guide/package_reference.md) - Detailed package structure

## Documents

- Documents should be placed under docs
- Default language is Japanese (exceptions: README.md, CLAUDE.md)
- Default format is markdown
 - Use Mermaid syntax for diagrams.
  - Follow the style and legend used in `docs/tasks/0030_verify_files_variable_expansion/02_architecture.md`.
  - Use a cylinder shape for "data" nodes instead of the default rectangle (in Mermaid flowcharts a cylinder node can be written as `[(data)]`).
  - **Node label quoting**: Always wrap node labels in double quotes if they contain special characters (parentheses, brackets, colons, slashes, etc.). Example: `A["label (with parens)"]`
  - **Line breaks in labels**: Use `<br>` for line breaks inside node labels, not `\n`. Example: `A["line1<br>line2"]`

### Translation Guidelines (Japanese to English)

When translating Japanese documentation to English:

1. **Translation Workflow**:
   - First create and commit the Japanese version
   - Then create the English version based on the Japanese original

2. **Translation Principles**:
   - **Accuracy over fluency**: Prioritize precise translation over natural-sounding English
   - **Faithful translation**: Do not delete content from the Japanese version or add content not present in the original
   - **Structural consistency**: Match chapter headings and sentence structure between Japanese and English versions

3. **Terminology Management**:
   - Create and maintain a glossary file under `docs/` directory
   - Use consistent terminology from the glossary
   - Add new terms to the glossary as needed
   - Glossary location: `docs/translation_glossary.md`

## Commands

### Build Commands
- `make build` - Build all binaries (record and verify executables)
- `make clean` - Clean build artifacts
- `make all` - Default build target

### Test Commands
- `make test` - Run all tests with verbose output
- `go test -tags test -v ./...` - Run all tests directly
- `go test -tags test -v ./internal/specific/package` - Run tests for specific package

### Code Quality
- `make lint` - Run linter with golangci-lint
- `golangci-lint run` - Run linter directly
- `make fmt` - Run formatter with gofumpt

### Individual Binary Builds
- Build record binary: `go build -o build/record -v cmd/record/main.go`
- Build verify binary: `go build -o build/verify -v cmd/verify/main.go`

## Architecture Overview

This is a Go-based secure command runner with the following core components:

### Core Architecture
- **Command Runner**: Safe execution wrapper for batch processing with security controls
- **File Validator**: Integrity verification using hash validation (`internal/filevalidator`)
- **Safe File I/O**: Secure file operations with symlink protection (`internal/safefileio`)
- **Command Executor**: Core execution engine with output handling (`internal/runner/executor`)
- **Config Management**: TOML-based configuration loading (`internal/runner/config`)

See [Package Reference](docs/dev/developer_guide/package_reference.md) for detailed package structure.

### Key Design Patterns
- **Separation of Concerns**: Each package has a single responsibility
- **Interface-based Design**: Heavy use of interfaces for testability (e.g., `CommandExecutor`, `FileSystem`, `OutputWriter`)
- **Security First**: Path validation, command injection prevention, privilege separation
- **Error Handling**: Comprehensive error types and validation
- **YAGNI**: Use simple and clear approach to satisfy the requirement. Don't take complex approach for not-yet-planned features.
 - **DRY**: Don't repeat yourself. Before adding new code, check the codebase and prefer reusing existing implementations.
- **Declare, don't infer**: Do not choose behavior by inspecting the content of a
  string supplied by a caller (`strings.Contains`/`HasPrefix` over caller data to
  pick a code path). Put the choice in the type: an enum field whose zero value is
  the interpretation that assumes least about its input, dispatched by a `switch`
  whose `default` fails secure. Changing a struct's shape is cheap here — the
  project has no consumers outside the repository, so "it would change the exported
  API" is not on its own a reason to keep inferring.
- **Enforce invariants with the type, not with convention**: if a value must pass
  validation before use, make the compiler the thing that guarantees it —
  unexported fields plus a mandatory constructor — rather than relying on "every
  production path happens to call `DefaultConfig`". Before preserving an exported
  field as an extension point, count its real uses; an extension point nobody uses
  costs the guarantee and buys nothing.
- **Reject, don't normalize**: when code can either quietly repair a caller's
  malformed input or reject it, reject it. Silent repair makes a wrong definition
  indistinguishable from a right one, so the mistake never surfaces and propagates
  into whatever someone copies next. Prefer a `validate` method returning a
  sentinel error, rejection at the construction boundary, and a test over the real
  defaults that fails the build when they are edited into an invalid shape.

### Performance

- **A benchmark regression is not by itself a defect.** Justify an optimization
  against an *absolute* budget — wall time of a real run, allocations per log line
  — never against a relative delta versus the previous commit. "1.9x slower" means
  nothing until it is converted to an absolute number and compared with what the
  runner already spends (a `fork`/`exec` is tens of microseconds; a 1,000-line run's
  whole logging budget is milliseconds). If the honest conclusion is "the regression
  does not show up in wall time", record that and close it — do not add a mechanism.
- **Profile before optimizing** to confirm the cost is where you assume. If the
  stage you are about to optimize is a small fraction of its path's total cost,
  stop; also check that the work you are caching is actually all of the work (a
  cache in front of one step that leaves the expensive setup running per call buys
  nothing).
- **An optimization that adds a correctness obligation** — case folding, encoding
  assumptions, cache invalidation, a fast path that must agree with the slow path —
  must clear a much higher bar. State the obligation in the commit message and pin
  it with a test that fails if the optimization is later changed unsafely.
- **Optimizations go in their own commit**, separable by revert, and never in the
  same commit as the behavior change that motivated them.
- **Do not close a review finding by adding a mechanism until the finding's premise
  is verified.** Measure whether the reported cost is real harm in absolute terms
  first; a self-inflicted regression is not an obligation to build something.

### Security Features
- Command path validation and sanitization
- Environment variable isolation
- Working directory validation
- File integrity verification with hash validation
- Safe file operations with symlink attack prevention

### Configuration
- Uses TOML format for configuration files
- Supports environment variable management
- Template-based command definitions
- Group-based command execution with dependency management

### Testing Strategy
- Unit tests for all core components
- Mock implementations for external dependencies
- File system abstraction for testing
- Output capture and verification
- **Error Testing**: Use `errors.Is()` to validate error types, not string matching on error messages
- **Every test must be able to fail for its stated reason.** Before committing a
  test, disable the thing it claims to cover — nil the collaborator, revert the
  branch, break the default — and confirm it fails. Say in the commit message that
  you did.
- **A layered path needs inputs only one layer can handle.** Where two mechanisms
  can produce the same output (e.g. key-name redaction and value-format detection),
  an input both layers match proves nothing about either: assertions like "output
  differs from input" or "output contains the placeholder" cannot say which layer
  ran. Construct the input so only the layer under test can act, and assert first
  that the other layer alone leaves it untouched — that makes the test
  self-policing.
- **Do not assert that a constant equals its own literal**, or that a struct field
  holds what was just assigned to it. These pass unconditionally and reach nothing.
- **Check for duplication before adding a table test for a helper.** If the helper
  differs from its caller only by a loop, and the caller's table already covers the
  same rows with the same literals, one table is enough. Keep the helper-level test
  only for cases the caller's table cannot reach.
- **Deleting a test is a claim that must be checked**: confirm `go tool cover -func`
  is unchanged function by function afterwards, and say so.

See [Test Organization Guide](docs/dev/developer_guide/test_organization.md) for test helper file structure.

## Development Notes

- Uses Go modules with Go 1.23.10
- Dependencies: go-toml/v2, stretchr/testify
- Security-focused codebase with extensive validation
- Comprehensive error handling with custom error types
- Interface-driven design for testability and modularity
- After editing go files, make sure to run `make fmt` to format the files.
- After editing files, make sure to run `make test` and `make lint` and fix errors.

## Modern Go Idioms (Go 1.21+)

When writing or modifying Go code in this repository, prefer the following modern idioms over older equivalents. These improve readability, reduce boilerplate, and leverage standard library improvements.

### Language Features
- Use `any` instead of `interface{}`.
- Use `for range n` (Go 1.22+) instead of `for i := 0; i < n; i++` when the index is unused or only counts iterations.
- Rely on per-iteration loop variable scope (Go 1.22+); do not write `i := i` shadowing inside loop bodies.
- Use range-over-function iterators (Go 1.23+) for custom traversal where appropriate.

### Built-in Functions
- Use `min(a, b)` / `max(a, b)` instead of hand-written comparisons or `math.Max`/`math.Min`.
- Use `clear(m)` to clear maps and slices instead of manual `for k := range m { delete(m, k) }`.

### Standard Library
- Use the `slices` package: `slices.Contains`, `slices.Index`, `slices.Sort`, `slices.SortFunc`, `slices.Equal`, `slices.Clone`, `slices.Concat`, `slices.Delete`, `slices.Insert`, etc., instead of explicit loops.
- Use the `maps` package: `maps.Keys`, `maps.Values`, `maps.Clone`, `maps.Equal`, `maps.Copy`.
- Use `cmp.Or(a, b, c)` to return the first non-zero value instead of chained `if x == zero { x = y }`.
- Use `cmp.Compare` for three-way comparisons, especially in `slices.SortFunc`.
- Use `errors.Join(err1, err2)` for combining multiple errors.
- Use `fmt.Errorf("...: %w", err)` for error wrapping.
- Use `strings.Cut` / `bytes.Cut` instead of `SplitN(s, sep, 2)`.
- Use `strings.CutPrefix` / `strings.CutSuffix` instead of `HasPrefix` + `TrimPrefix` combinations.
- Use `sync.OnceFunc` / `sync.OnceValue` / `sync.OnceValues` instead of `sync.Once` + closure boilerplate.
- Use `log/slog` for structured logging.
- Use `context.WithoutCancel` to detach cancellation propagation.
- Use `reflect.TypeFor[T]()` instead of `reflect.TypeOf((*T)(nil)).Elem()`.

### Generics
- Use type parameters (Go 1.18+) to consolidate duplicated `int`/`int64`/`float64` helpers.
- Prefer `slices.SortFunc` over `sort.Slice` for type-safe, faster sorting without reflection.

### Other Patterns
- Use `map[T]struct{}` instead of `map[T]bool` for set semantics (saves memory).
- Use `errors.Is` / `errors.AsType[T]` instead of string matching on error messages. Prefer `errors.AsType[T]` over `errors.As` — it eliminates the `var target T` declaration:
  ```go
  // Before
  var pathErr *fs.PathError
  if errors.As(err, &pathErr) { ... }

  // After
  if pathErr, ok := errors.AsType[*fs.PathError](err); ok { ... }
  ```
- In tests, use `t.Cleanup` instead of manual `defer` chains, and `t.TempDir` instead of `os.MkdirTemp` + `defer os.RemoveAll`.

## Requirements and Acceptance Criteria

When implementing new features or security-critical functionality, follow the process documented in [Requirements Process Guide](docs/dev/developer_guide/requirements_process.md).

**Quick summary:**
1. Create `01_requirements.md` with explicit acceptance criteria
2. Create `02_architecture.md` with high-level design (Mermaid diagrams)
3. Create `03_implementation_plan.md` with progress tracking (checkboxes) and AC traceability
4. Write tests for each acceptance criterion
5. Link tests to acceptance criteria in the implementation plan

## Tool Execution Safety

**CRITICAL**
- Don't run following commands without user's explicit approval
  - commands interacting with network, e.g. git pull
  - merging pull requests on GitHub
- `git commit` and `git push` may be executed without explicit approval
