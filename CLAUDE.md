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
- Use Mermaid for diagrams, following the conventions in
  [mermaid_reference.md](docs/dev/developer_guide/mermaid_reference.md) (node-label
  quoting, `<br>` line breaks, cylinder `[(...)]` data nodes, the standard classDef
  palette, and Legend blocks). `docs/tasks/0030_verify_files_variable_expansion/02_architecture.md`
  is a worked example.

### Translation Guidelines (Japanese to English)

Create and commit the Japanese version first, then write the English version from
it. The translation principles (accuracy over fluency, faithful translation,
structural consistency) and the glossary workflow live in
[mktrans.md](.claude/commands/mktrans.md), which automates this; the glossary is
[docs/translation_glossary.md](docs/translation_glossary.md).

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

## Architecture Overview

A Go-based secure command runner: it executes batches of configured commands under
integrity, path, and privilege controls. Entry points are `internal/filevalidator`
(hash-based integrity verification), `internal/safefileio` (file operations with
symlink-attack prevention), `internal/runner/executor` (execution and output
handling), and `internal/runner/config` (TOML configuration loading).

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

### Testing Strategy

Interfaces (`CommandExecutor`, `FileSystem`, `OutputWriter`) exist so external
dependencies can be mocked and the file system abstracted; tests capture and verify
command output.

- **Error Testing**: Use `errors.Is` / `errors.AsType[T]` to validate error types,
  never string matching on error messages.
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

- Go 1.26.2 (see `go.mod`); dependencies: go-toml/v2, stretchr/testify, oklog/ulid.
- After editing go files, make sure to run `make fmt` to format the files.
- After editing files, make sure to run `make test` and `make lint` and fix errors.

## Go Idioms

Modern-idiom drift (`any` over `interface{}`, `for range n`, `min`/`max`, the
`slices`/`maps` packages, `strings.Cut`, `slices.SortFunc` over `sort.Slice`, …) is
enforced mechanically by the `modernize`, `intrange`, `copyloopvar`, and
`usestdlibvars` linters in `.golangci.yml`, most of them auto-fixable with
`golangci-lint run --fix`. Run `make lint` rather than working from a list here.

Only the conventions a linter does not check are worth stating:

- Prefer `errors.AsType[T]` over `errors.As` — it eliminates the `var target T`
  declaration. This is a Go 1.26 API, so it is not what habit produces:
  ```go
  // Before
  var pathErr *fs.PathError
  if errors.As(err, &pathErr) { ... }

  // After
  if pathErr, ok := errors.AsType[*fs.PathError](err); ok { ... }
  ```
- Use `map[T]struct{}` rather than `map[T]bool` for set semantics.
- In tests, prefer `t.Cleanup` over manual `defer` chains and `t.TempDir` over
  `os.MkdirTemp` + `defer os.RemoveAll`.

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
