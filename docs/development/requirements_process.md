# Requirements and Acceptance Criteria Process

When implementing new features or security-critical functionality, follow this process to prevent implementation gaps.

## 1. Requirements Document (`docs/tasks/XXXX_feature/01_requirements.md`)

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

## 2. Architecture Design Document (`docs/tasks/XXXX_feature/02_architecture.md`)

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

## 3. Detailed Specification (`docs/tasks/XXXX_feature/03_detailed_specification.md`)

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

## 4. Implementation Plan (`docs/tasks/XXXX_feature/04_implementation_plan.md`)

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

## 5. Acceptance Tests

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

## 6. Pre-Commit Checklist

Before considering a feature complete:
- [ ] All acceptance criteria defined in requirements document
- [ ] Architecture design document created with high-level design
- [ ] Detailed specification created with implementation details
- [ ] Implementation plan created and updated during development
- [ ] Acceptance verification phase added to detailed specification
- [ ] At least one test per acceptance criterion
- [ ] All acceptance tests pass
- [ ] Security requirements explicitly tested

## 7. Historical Context

This process was established after discovering a critical security gap in the template include feature (task 0066). The included template files were not being hash-verified, despite the requirement stating "included files should also be subject to checksum verification to detect tampering". The gap occurred because:

1. Requirements lacked explicit acceptance criteria
2. No verification phase in the detailed specification
3. No tests specifically validating the security requirement

The security implementation was later added (`VerifiedTemplateFileLoader`), and this process ensures such gaps don't recur.
