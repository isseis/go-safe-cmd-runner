# Requirements and Acceptance Criteria Process

When implementing new features or security-critical functionality, follow this process to prevent implementation gaps.

## 0. Review Workflow

### LLM Constraints (Critical)

- LLMs must **always create `01_requirements.md`, `02_architecture.md`, and `03_implementation_plan.md` in draft status (`draft`)**. They must never be created as approved (`approved`).
- Do not begin creating `02_architecture.md` until the status of `01_requirements.md` is `approved`.
- Do not begin creating `03_implementation_plan.md` until the status of `02_architecture.md` is `approved`.
- Do not begin writing any implementation code until the status of `03_implementation_plan.md` is `approved`.
- If a document with a non-`approved` status is found, do not proceed with subsequent work even if instructed to do so — wait until the status is `approved`.

### Review Flow

```
LLM creates 01_requirements.md (status: draft)
  → Human review and revision
  → Reviewer updates status to approved
  → LLM creates 02_architecture.md (status: draft)
    → Human review and revision
    → Reviewer updates status to approved
    → LLM creates 03_implementation_plan.md (status: draft)
      → Human review and revision
      → Reviewer updates status to approved
      → Proceed to writing implementation code
```

### Document Status Format

Include the following section at the top of `01_requirements.md`, `02_architecture.md`, and `03_implementation_plan.md`:

```markdown
## Document Status

| Item | Value |
|---|---|
| Status | `draft` or `approved` |
| Created | YYYY-MM-DD |
| Review date | YYYY-MM-DD (or `-` when draft) |
| Reviewer | Name (or `-` when draft) |
| Comments | Review notes and changes (or `-` if none) |
```

---

## 1. Requirements Document (`docs/tasks/XXXX_feature/01_requirements.md`)

**Mandatory for each functional requirement:**
- Define the requirement clearly (what, why, how)
- **Add explicit acceptance criteria** in a dedicated section
- Each acceptance criterion must be:
  - Specific and measurable
  - Independently verifiable
  - Focused on behavior, not implementation
- Assign a **document-wide unique identifier** (`AC-01`, `AC-02`, …) to each acceptance criterion. Rules for managing identifiers:
  - On initial creation, assign sequentially across all requirements (e.g., `F-001` gets `AC-01`–`AC-05`, `F-002` gets `AC-06`–`AC-08`)
  - **Deletion**: remove the criterion and leave the identifier unused — gaps are permitted
  - **Addition**: append the next available number, or use a suffix (`AC-01a`) when inserting between existing identifiers
  - Never renumber existing identifiers — doing so would break references in the implementation plan and test code

**Example format:**
```markdown
#### F-XXX: Feature Name

[Feature description]

**Acceptance Criteria**:
- **AC-01**: [Specific observable behavior #1]
- **AC-02**: [Specific observable behavior #2]
- **AC-03**: [Error handling requirement]
- **AC-04**: [Security requirement]
- **AC-05**: [Edge case handling]
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
   - **Use Mermaid diagram style**: Follow the conventions in [mermaid_reference.md](mermaid_reference.md)
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

6. **Processing Flow Details**
   - Key processing flows with sequence/flowchart diagrams

7. **Test Strategy**
   - Unit test strategy
   - Integration test strategy
   - Security test strategy

8. **Implementation Priorities**
   - Phase breakdown
   - Ordered implementation steps

9. **Future Extensibility**
   - Design considerations for future enhancements

**Content guidelines:**
- **Focus on high-level design**: Use diagrams and natural language descriptions
- **Code examples**: Only include high-level code (interfaces, type definitions, error types)
- **Avoid implementation details**: Concrete code belongs in the implementation itself, not the design doc
- **Language**: Japanese (default)
- **Format**: Markdown with Mermaid diagrams

**Reference**: `docs/tasks/0084_resolved_path_type_safety/02_architecture.md`

## 3. Implementation Plan (`docs/tasks/XXXX_feature/03_implementation_plan.md`)

**Purpose**: Track implementation progress with actionable tasks and checkboxes.

**Required sections:**
1. **Implementation Overview**
   - Purpose (目的)
   - Implementation principles

2. **Implementation Steps**
   - Organized by phases derived from the architecture document
   - Each step includes:
     - **Files to modify**: Specific file paths
     - **Work content**: What to do (with checkboxes)
     - **Success criteria**: How to verify completion
   - Use checkboxes `[ ]` for tracking: `- [ ] Task description`
   - Mark completed items: `- [x] Completed task`
   - Mark partially completed: `- [-] Partially done (with note)`

3. **Implementation Order and Milestones**
   - Milestone definitions with deliverables

4. **Test Strategy**
   - Unit test coverage goals
   - Integration test scenarios
   - Backward compatibility testing

5. **Risk Management**
   - Technical risks with mitigation strategies
   - Schedule risks with buffer plans

6. **Implementation Checklist**
   - Phase-by-phase checklist with checkboxes
   - Overall completion tracking

7. **Success Criteria**
   - Functional completeness metrics
   - Quality metrics (test coverage, etc.)
   - Security verification requirements
   - Documentation completeness

8. **Next Steps**
   - Post-implementation activities

**Content guidelines:**
- **Focus on tracking**: Use checkboxes extensively for progress tracking
- **Avoid duplication**: Reference other documents instead of repeating content
  - Don't duplicate architecture diagrams or design details
  - Reference sections like "See 02_architecture.md Section 3.2 for design details"
- **Actionable tasks**: Each checkbox should represent a concrete, completable action
- **Update during implementation**: Mark tasks as complete in real-time
- **AC traceability**: Include an explicit "Acceptance Criteria Verification" section
  mapping each AC to the test that proves it (see § 4)
- **Language**: Japanese (default)

**Reference**: `docs/tasks/0084_resolved_path_type_safety/03_implementation_plan.md`

## 4. Acceptance Tests

**Create appropriate test coverage:**
- Place tests in standard test files (`*_test.go`)
- Follow normal test naming conventions based on what is being tested
- Tests can be unit tests, integration tests, or any appropriate type
- Each acceptance criterion must have at least one `test` or `static` verification (see "Acceptance Criteria Verification" in `03_implementation_plan.md`); a `static` check alone is sufficient only for criteria that are purely about textual/documentation presence
- Tests must verify the actual behavior, not just the happy path
- Link tests to acceptance criteria in the implementation plan

**Traceability in implementation plan:**
Document which tests verify each acceptance criterion in `03_implementation_plan.md`:

```markdown
**AC-01: [First acceptance criterion]**
- Test location: `internal/package/subpackage_test.go::TestFunctionName`
- Implementation: `internal/package/subpackage.go:123-145`
- Verification method: [How to verify]

**AC-02: [Second acceptance criterion]**
- Test location: `internal/package/integration_test.go::TestIntegrationScenario`
- Implementation: `internal/package/another.go:67-89`
- Verification method: [How to verify]
```

**Example test:**
```go
// TestFoo verifies that [behavior].
func TestFoo(t *testing.T) {
    // Test implementation that verifies the specific criterion
}
```

Traceability between this test and its acceptance criteria is recorded in
`03_implementation_plan.md` (see "Traceability in implementation plan" above),
not as `AC-NN`/`F-NNN` references in source comments — `runplan.md`'s
pre-commit checks reject such references in Go source.

## 5. Pre-Commit Checklist

Before considering a feature complete:
- [ ] All acceptance criteria defined in requirements document
- [ ] Architecture design document created with high-level design
- [ ] Implementation plan created and updated during development
- [ ] Acceptance criteria verification section present in implementation plan
- [ ] At least one test per acceptance criterion
- [ ] All acceptance tests pass
- [ ] Security requirements explicitly tested

## 6. Background

This process was established after a critical security gap was discovered in a feature: a requirement explicitly stated that certain files should be subject to checksum verification to detect tampering, yet the implementation omitted this check entirely. The gap occurred because:

1. Requirements lacked explicit acceptance criteria
2. No verification phase traced each criterion to a test
3. No tests specifically validated the security requirement

This process ensures such gaps do not recur by requiring explicit acceptance criteria for every functional requirement, and traceability between each criterion and its test.
