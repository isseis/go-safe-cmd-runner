> **Project context (read first)**: Read `.claude/commands/_context.md`. It is the
> single source of truth for every project-specific value below — the task root,
> guide paths, document names (`01_requirements.md`, `02_architecture.md`, …),
> status values (`draft`/`approved`), source layout (`cmd/`/`internal/`), and
> document language. Where this command names such a path or value, treat the
> entry in `_context.md` as canonical. When porting, follow the porting steps in
> `_context.md`. Note that this command body also contains Go/project-specific
> references (`cmd/`/`internal/` layout inspection in step 6) that may need
> updating when porting to a different tech stack or project structure. The review
> step uses the shared procedure in
> `.claude/commands/_lib/review-subagent-pattern.md`.

Your goal is to create `02_architecture.md` for one task under `docs/tasks/`.

Work in the following order.

1. Identify the target task directory by following the rules in `docs/dev/developer_guide/task_identification.md`.

2. Read `01_requirements.md` in the target task directory.

3. Verify that architecture work is allowed.
- Check the document status in `01_requirements.md`.
- If the status is not `approved`, do not create `02_architecture.md`.
- In that case, stop and report that architecture work cannot begin until `01_requirements.md` is `approved`.

4. Read the remaining required input documents.
- `docs/dev/developer_guide/requirements_process.md`
- `docs/dev/developer_guide/mermaid_reference.md`

5. Read conditional guidance only when relevant.
- Read the conditional guide (path in `_context.md`, Domain-specific) if the conditional-guide trigger applies (trigger also in `_context.md`, Domain-specific).
- Read `docs/dev/developer_guide/package_reference.md` if that file exists and the task introduces new packages or modifies existing packages.

6. Inspect the current codebase before writing the design.
- Check the relevant packages under `cmd/` and `internal/`.
- Identify existing components that should be reused.
- Do not design new logic that duplicates responsibilities already handled elsewhere in the repository.
- For any diagram edge that depicts the *current* behavior of existing components (i.e., relationships that already exist in code today, not new relationships this feature is introducing), verify that it accurately reflects actual code behavior. Edges that show newly planned relationships introduced by this feature do not need to match existing code, but should be clearly distinguishable from current-behavior edges (e.g., by using the `enhanced` class or an explanatory label).
- If the design introduces any behavior that conflicts with or creates an exception to policies established in other architecture documents under `docs/tasks/`, identify and document all three of the following inline in the design (not only in an appendix): (1) the original policy and where it is documented, (2) why this design is an intentional exception, (3) which existing tests assert the old behavior and will therefore need updating.
- Identify existing tests (in `*_test.go` files) that assert behaviors this design changes. Note them in the component responsibilities table or the relevant design section so implementers know which tests require updating.
- **Before adopting any new feature of an external API** (e.g., a Slack Block Kit element, an IMAP extension, an RFC-defined capability), verify that it behaves correctly on **all target client environments** (listed in `_context.md`, Domain-specific). Record the verification result inline in the design document. If verification across all targets is not possible before the architecture is approved, document the risk explicitly and prefer a fallback that is known to work on all targets.
- **Before introducing a new approach to replace existing behavior**, state briefly why the existing simpler approach cannot satisfy the requirements. If it can, use the simpler approach (YAGNI). This is required even when the new approach is technically superior — the question is whether the requirement demands it.

7. Create `02_architecture.md` in the same task directory.
- Write in Japanese.
- Set the document status to `draft`.
- Include all required sections defined in `docs/dev/developer_guide/requirements_process.md`.
- Reflect all functional requirements and acceptance criteria from `01_requirements.md`.
- For any flag, mode, or option that changes which side effects occur (e.g. `--dry-run`, `--force`, read-only mode), define explicitly which external side effects (writes, deletes, network sends) it suppresses or permits. An under-specified side-effect contract leads to inconsistent implementations.
- Use Mermaid diagrams for the concept model, system structure, key processing flows, and a threat model when applicable.
- Restrict code examples to high-level interfaces, type definitions, and error type definitions only.
- Do not include implementation details, pseudocode, step-by-step algorithms, or low-level code.
- Write the body for an engineer meeting the current system for the first time: describe how it works now. Confine the rationale for removed or superseded designs, and cross-task decision history, to a bounded "decision history" appendix or a short blockquote pointing to git history — do not interleave it with current-state description. When editing a design document that earlier tasks have appended to, preserve this separation so the body does not become a changelog.

8. Run the critical-review subagent procedure in `.claude/commands/_lib/review-subagent-pattern.md` with these inputs:
   - **ARTIFACT**: the created architecture document (path in `_context.md`).
   - **PERSONA**: an experienced software architect and senior SRE. Direct it to surface gaps, ambiguities, and design risks.
   - **FILES**: the architecture document, the requirements document, the requirements process guide, and the Mermaid reference guide (paths in `_context.md`), as resolved absolute-path strings. If the conditional-guide trigger applies (`_context.md`, Domain-specific), also include the conditional guide.
   - **CRITERIA**: every item from the Technical correctness checklist and the Readability and consistency checklist below, copied verbatim.

   Extra rule: commit only after all review passes are complete and all Critical and Major issues are resolved.

**Technical correctness checklist (use verbatim as evaluation criteria in the subagent prompt above):**
- [ ] `01_requirements.md` is `approved`.
- [ ] `02_architecture.md` is written in Japanese and its status is `draft`.
- [ ] All required sections from the requirements process guide are present.
- [ ] All functional requirements and acceptance criteria in `01_requirements.md` are reflected in the design.
- [ ] For each acceptance criterion that applies to an existing code pattern (e.g., "log slog.Warn when X fails"), the design accounts for ALL instances of that pattern in the codebase, not only the most prominent ones. Verify by searching the codebase for the pattern.
- [ ] Class diagrams: each method signature and field type shown matches the actual Go source (verify by reading the corresponding `.go` file). Pay special attention to return types, including error returns, and fully-qualified package prefixes on types.
- [ ] If the design introduces an exception to a policy established in another architecture document under `docs/tasks/`, the exception is stated inline (not only in an appendix) with: the original policy and its location, the reason for the exception, and which existing tests assert the old behavior and will need updating.
- [ ] Mermaid diagrams follow the documented conventions consistently.
- [ ] Data nodes use cylinder shape `[("label")]`.
- [ ] Labels with special characters are double-quoted.
- [ ] Line breaks inside labels use `<br>`.
- [ ] Code examples contain only interfaces, type definitions, and error type definitions.
- [ ] No implementation details, pseudocode, or concrete algorithms are included.
- [ ] The security section is present and uses `N/A` when not applicable.
- [ ] The component responsibilities table lists all new and modified files.
- [ ] The design does not overlap with existing packages or re-implement existing responsibilities.
- [ ] Any new external-service feature the design relies on (Slack API, IMAP capability, etc.) is verified to behave correctly on all target client environments listed in `_context.md`; the verification result is stated inline, or the unverified risk is explicitly documented.
- [ ] When the design replaces existing behavior with a new approach, a "why not the existing approach?" justification is present and names the specific requirement the simpler approach cannot satisfy.

**Readability and consistency checklist (use verbatim as evaluation criteria in the subagent prompt above):**
- [ ] The arrow semantics used in each diagram are stated explicitly in a caption or note (e.g., "矢印 A → B は…を表す"), and are applied consistently within that diagram.
- [ ] Node labels read as component or type names, not as lists of values or behavioral descriptions.
- [ ] Every Mermaid diagram includes a Legend block that explains its node classes.
- [ ] Each Legend block shows only color-coded nodes; it does not contain arrows that could imply unintended relationships.
- [ ] Terminology is consistent throughout the document; the same concept always uses the same Japanese term.
- [ ] Ambiguous or overly terse expressions are rewritten in direct, plain Japanese. Readers should not need context from prior review discussions to understand the text.
- [ ] Architectural decisions that depend on constraints not obvious from the requirements are explained inline.
- [ ] The body describes the current system; rationale for removed or superseded designs and cross-task history is confined to a bounded appendix or blockquote, not interleaved with current-state description.

When finished, provide a concise summary of what you created and any assumptions you had to make.
