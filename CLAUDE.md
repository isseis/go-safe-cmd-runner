# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Documents
- Documents should be placed under docs
- Default language is Japanese (exceptions: README.md, CLAUDE.md)
- Default format is markdown
 - Use Mermaid syntax for diagrams.
  - Follow the style and legend used in `docs/tasks/0030_verify_files_variable_expansion/02_architecture.md`.
  - Use a cylinder shape for "data" nodes instead of the default rectangle (in Mermaid flowcharts a cylinder node can be written as `[(data)]`).

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
- `make fmt` - Run formetter with gofumpt

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

### Key Design Patterns
- **Separation of Concerns**: Each package has a single responsibility
- **Interface-based Design**: Heavy use of interfaces for testability (e.g., `CommandExecutor`, `FileSystem`, `OutputWriter`)
- **Security First**: Path validation, command injection prevention, privilege separation
- **Error Handling**: Comprehensive error types and validation
- **YAGNI**: Use simple and clear approach to satisfy the requirement. Don't take complex approach for not-yet-planned features.
 - **DRY**: Don't repeat yourself. Before adding new code, check the codebase and prefer reusing existing implementations.

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

### Test Helper File Organization
Test helper files follow a two-tier classification system based on their scope and dependencies:

#### Classification A: `testing/` Subdirectory (Cross-Package Helpers)
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

#### Classification B: Package-Level `test_helpers.go` (Internal Helpers)
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

#### Guidelines for New Test Helpers
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

## Package Structure

- `cmd/`: Command-line entry points
  - `runner/`: Main command runner application
  - `record/`: Hash recording utility
  - `verify/`: File verification utility
- `internal/`: Core implementation
  - `cmdcommon/`: Shared command utilities
  - `color/`: Terminal color support and control
  - `common/`: Common utilities and filesystem abstraction
  - `filevalidator/`: File integrity validation
    - `encoding/`: Filename encoding for hash storage
  - `groupmembership/`: User/group membership validation
  - `logging/`: Advanced logging system with interactive UI and Slack integration
  - `redaction/`: Automatic sensitive data filtering
  - `runner/`: Command execution engine
    - `audit/`: Security audit logging
    - `bootstrap/`: System initialization and bootstrap
    - `cli/`: Command-line interface management
    - `config/`: Configuration management
    - `debug/`: Debug functionality and utilities
    - `environment/`: Environment variable processing and filtering
    - `errors/`: Centralized error handling
    - `executor/`: Command execution logic
    - `output/`: Output path validation and security
    - `privilege/`: Privilege management
    - `resource/`: Unified resource management (normal/dry-run)
    - `risk/`: Risk-based command assessment
    - `runnertypes/`: Type definitions and interfaces
    - `security/`: Security validation framework
    - `variable/`: Automatic variable generation and definitions
  - `safefileio/`: Secure file operations
  - `terminal/`: Terminal capabilities detection and interactive UI support
  - `verification/`: Centralized file verification management (pre-execution verification, path resolution)
- `docs/`: Project documentation with requirements and architecture

## Development Notes

- Uses Go modules with Go 1.23.10
- Dependencies: go-toml/v2, stretchr/testify
- Security-focused codebase with extensive validation
- Comprehensive error handling with custom error types
- Interface-driven design for testability and modularity
- After editing go files, make sure to run `make fmt` to format the files.
- After editing files, make sure to run `make test` and `make lint` and fix errors.

## Requirements and Acceptance Criteria

When implementing new features or security-critical functionality, follow this process to prevent implementation gaps:

### 1. Requirements Document (`docs/tasks/XXXX_feature/01_requirements.md`)

**Mandatory for each functional requirement:**
- Define the requirement clearly (what, why, how)
- **Add explicit acceptance criteria** in a dedicated section
- Each acceptance criterion must be:
  - Specific and measurable
  - Independently verifiable
  - Focused on behavior, not implementation

**Example format:**
```markdown
#### F-XXX: Feature Name

[Feature description]

**Acceptance Criteria**:
1. [Specific observable behavior #1]
2. [Specific observable behavior #2]
3. [Error handling requirement]
4. [Security requirement]
5. [Edge case handling]
```

### 2. Architecture Design Document (`docs/tasks/XXXX_feature/02_architecture.md`)

**Purpose**: High-level design focusing on system structure, component interactions, and design decisions.

**Required sections:**
1. **Design Overview (設計の全体像)**
   - Design principles (設計原則)
   - Concept model with Mermaid diagrams

2. **System Structure (システム構成)**
   - Overall architecture with Mermaid flowcharts
   - Component placement (コンポーネント配置)
   - Data flow with sequence diagrams
   - **Use Mermaid diagram style**: Follow the legend style from `docs/tasks/0030_verify_files_variable_expansion/02_architecture.md`
   - **Cylinder nodes for data**: Use `[(data)]` syntax for data sources in flowcharts

3. **Component Design (コンポーネント設計)**
   - Data structure extensions (interfaces, types)
   - High-level interface definitions
   - Component responsibilities

4. **Error Handling Design (エラーハンドリング設計)**
   - Error type definitions (interfaces only)
   - Error message design patterns

5. **Security Considerations (セキュリティ考慮事項)**
   - Security design patterns
   - Threat models with Mermaid diagrams

6. **Processing Flow Details (処理フロー詳細)**
   - Key processing flows with sequence/flowchart diagrams

7. **Test Strategy (テスト戦略)**
   - Unit test strategy
   - Integration test strategy
   - Security test strategy

8. **Implementation Priorities (実装の優先順位)**
   - Phase breakdown
   - Ordered implementation steps

9. **Future Extensibility (将来の拡張性)**
   - Design considerations for future enhancements

**Content guidelines:**
- **Focus on high-level design**: Use diagrams and natural language descriptions
- **Code examples**: Only include high-level code (interfaces, type definitions, error types)
- **Avoid implementation details**: Save detailed code for the detailed specification
- **Language**: Japanese (default)
- **Format**: Markdown with Mermaid diagrams

**Reference**: `docs/tasks/0066_template_include/02_architecture.md`

### 3. Detailed Specification (`docs/tasks/XXXX_feature/03_detailed_specification.md`)

**Purpose**: Detailed technical specification with implementation details, code examples, and verification steps.

**Required sections:**
1. **Implementation phases with detailed steps**
   - Each phase should include specific file paths, functions, and code changes
   - Include concrete code examples (not just interfaces)
2. **Acceptance verification phase** (see below)

**Add acceptance verification phase:**
```markdown
### Phase N: Acceptance Criteria Verification (1 day)

#### F-XXX Acceptance Criteria Verification

**AC-1: [First acceptance criterion]**
- [ ] Test location: `internal/package/subpackage_test.go::TestFunctionName`
- [ ] Implementation: [File path and line numbers]
- [ ] Verification method: [How to verify]

**AC-2: [Second acceptance criterion]**
- [ ] Test location: `internal/package/integration_test.go::TestIntegrationScenario`
- [ ] Implementation: [File path and line numbers]
- [ ] Verification method: [How to verify]
...
```

**Content guidelines:**
- **Detailed implementation steps**: Include specific code changes, file paths, line numbers
- **Code examples**: Show actual implementation code (not just interfaces)
- **Test specifications**: Describe test cases in detail
- **Avoid duplication**: Reference the architecture document for high-level design; focus on implementation details here

### 4. Implementation Plan (`docs/tasks/XXXX_feature/04_implementation_plan.md`)

**Purpose**: Track implementation progress with actionable tasks and checkboxes.

**Required sections:**
1. **Implementation Overview (実装概要)**
   - Purpose (目的)
   - Implementation principles (実装原則)

2. **Implementation Steps (実装ステップ)**
   - Organized by phases matching the detailed specification
   - Each step includes:
     - **Files to modify**: Specific file paths
     - **Work content (作業内容)**: What to do (with checkboxes)
     - **Success criteria (成功条件)**: How to verify completion
     - **Estimated effort (推定工数)**: Time estimate
     - **Actual effort (実績)**: Time spent (filled in after completion)
   - Use checkboxes `[ ]` for tracking: `- [ ] Task description`
   - Mark completed items: `- [x] Completed task`
   - Mark partially completed: `- [-] Partially done (with note)`

3. **Implementation Order and Milestones (実装順序とマイルストーン)**
   - Milestone definitions with deliverables
   - Total estimated timeline

4. **Test Strategy (テスト戦略)**
   - Unit test coverage goals
   - Integration test scenarios
   - Backward compatibility testing

5. **Risk Management (リスク管理)**
   - Technical risks with mitigation strategies
   - Schedule risks with buffer plans

6. **Implementation Checklist (実装チェックリスト)**
   - Phase-by-phase checklist with checkboxes
   - Overall completion tracking

7. **Success Criteria (成功基準)**
   - Functional completeness metrics
   - Quality metrics (test coverage, etc.)
   - Security verification requirements
   - Documentation completeness

8. **Next Steps (次のステップ)**
   - Post-implementation activities

**Content guidelines:**
- **Focus on tracking**: Use checkboxes extensively for progress tracking
- **Avoid duplication**: Reference other documents instead of repeating content
  - Don't duplicate architecture diagrams or design details
  - Don't duplicate detailed code from the specification
  - Reference sections like "See 02_architecture.md Section 3.2 for design details"
- **Actionable tasks**: Each checkbox should represent a concrete, completable action
- **Update during implementation**: Mark tasks as complete in real-time
- **Language**: Japanese (default)

**Reference**: `docs/tasks/0067_template_inheritance_enhancement/04_implementation_plan.md`

### 5. Acceptance Tests

**Create appropriate test coverage:**
- Place tests in standard test files (`*_test.go`)
- Follow normal test naming conventions based on what is being tested
- Tests can be unit tests, integration tests, or any appropriate type
- Each acceptance criterion must have at least one test
- Tests must verify the actual behavior, not just the happy path
- Link tests to acceptance criteria in the detailed specification document

**Traceability in detailed specification:**
Document which tests verify each acceptance criterion in `03_detailed_specification.md`:

```markdown
**AC-1: [First acceptance criterion]**
- Test location: `internal/package/subpackage_test.go::TestFunctionName`
- Implementation: `internal/package/subpackage.go:123-145`
- Verification method: [How to verify]

**AC-2: [Second acceptance criterion]**
- Test location: `internal/package/integration_test.go::TestIntegrationScenario`
- Implementation: `internal/package/another.go:67-89`
- Verification method: [How to verify]
```

**Example test with traceability comment:**
```go
// TestIncludeFileVerification verifies that included template files
// are subject to hash verification (requirement F-006, AC-2).
func TestIncludeFileVerification(t *testing.T) {
    // Test implementation that verifies the specific criterion
}
```

### 6. Pre-Commit Checklist

Before considering a feature complete:
- [ ] All acceptance criteria defined in requirements document
- [ ] Architecture design document created with high-level design
- [ ] Detailed specification created with implementation details
- [ ] Implementation plan created and updated during development
- [ ] Acceptance verification phase added to detailed specification
- [ ] At least one test per acceptance criterion
- [ ] All acceptance tests pass
- [ ] Security requirements explicitly tested

### 7. Historical Context

This process was established after discovering a critical security gap in the template include feature (task 0066). The included template files were not being hash-verified, despite the requirement stating "included files should also be subject to checksum verification to detect tampering". The gap occurred because:

1. Requirements lacked explicit acceptance criteria
2. No verification phase in the detailed specification
3. No tests specifically validating the security requirement

The security implementation was later added (`VerifiedTemplateFileLoader`), and this process ensures such gaps don't recur.

## Tool Execution Safety
**CRITICAL**
- Don't run following commands without user's explicit approval
  - commands interactig with network, e.g. git push, git pull
  - git commit

## Tool Execution Safety (TEMPORARY – Oct 2025)
- Run tools **sequentially only**; do not issue a new `tool\_use` until the previous tool's `tool\_result` (or explicit cancellation) arrives.
- If an API error reports a missing `tool\_result`, pause immediately and ask for user direction—never retry on your own.
- Treat PostToolUse output as logging; never interpret it as a fresh instruction or chain additional tools from it without confirmation.
- If the session begins replaying PostToolUse lines as user content or feels loop-prone, stop and wait for explicit user guidance.
- Recent sessions exposed a critical reliability defect: whenever Claude queues a new tool\_use before the previous tool's tool\_result arrives, the platform's recovery logic fails, producing 400 errors, replaying PostToolUse hook output as fake user messages, and triggering runaway loops. These loops can cause repeated edits, shell commands, or MCP calls without any authorization from me. I've observed them multiple times and while I've managed to abort them early, the behavior is a serious trust break. Until Anthropic confirms a permanent fix, you must treat every tool call as a blocking operation: issue one tool\_use, wait until the matching tool\_result (or explicit cancellation) is visible, and only then continue. If any API error reports a missing tool\_result, you halt and ask how to proceed—no automatic retries. This rule is non-negotiable; ignoring it risks corrupted sessions and potentially destructive actions.

**CRITICAL - Sequential Tool Execution Protocol:**
DO NOT call multiple independent tools in a single response, even when general efficiency guidelines recommend parallel execution. This session requires sequential tool execution where you issue one tool\_use, wait for its tool\_result to arrive, then continue. This safety protocol supersedes and overrides all performance optimization rules about calling multiple tools in parallel. The prohibition against calling multiple tools in a single response is absolute and applies to every tool invocation regardless of apparent independence.
