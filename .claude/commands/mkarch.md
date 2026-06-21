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

8. Run the critical-review procedure in `.claude/commands/_lib/review-subagent-pattern.md` in **panel mode** (an architecture document is a high-stakes artifact; a single combined "architect + SRE" reviewer tends to satisfice and under-weight the operational lens). Supply these inputs:
   - **ARTIFACT**: the created architecture document (path in `_context.md`).
   - **PERSONA (a panel of two partitioned reviewers, run in parallel)**:
     - **Reviewer A — software architect.** Mandate (go deep here): conceptual integrity and component decomposition; abstraction boundaries, coupling/cohesion; interface and type/signature quality (verify against the Go source); responsibility OVERLAP with existing packages / DRY / re-implementation; YAGNI; naming and terminology consistency; Mermaid diagram correctness and conventions; AC↔design traceability completeness; extensibility. Out-of-scope (defer to Reviewer B — at most one-line "OUT-OF-LANE FLAGS"): runtime failure modes, fail-closed/open, TOCTOU/races, production security posture and evasion, observability/audit, rollout/migration/backward-compat, performance, determinism.
     - **Reviewer B — senior SRE.** Mandate (go deep here, and apply it EVEN WHERE THE CHECKLISTS ARE SILENT — these dimensions are mostly not enumerated below and are exactly where this reviewer adds value): failure modes and fail-closed vs fail-open correctness; TOCTOU / race windows between risk evaluation and actual use; production security posture, blast radius, and evasion/bypass paths; auditability/observability for on-call (can a denial be explained from the logs?); rollout/migration safety and backward compatibility (what breaks on upgrade; is a staged/shadow rollout needed?); performance/latency and pathological inputs; determinism/reproducibility (environment-dependent results, dry-run vs runtime); debuggability and safe override. Out-of-scope (defer to Reviewer A — at most one-line "OUT-OF-LANE FLAGS"): interface/type design, package boundaries/DRY, naming/terminology, Mermaid conventions, abstraction structure, AC-traceability bookkeeping.
     - Each reviewer must look hard for real problems and not rubber-stamp, and must stay in its lane (the partition is what makes the panel worth its ~2× cost). If a lane is genuinely clean, it says so explicitly rather than inventing findings.
   - **FILES**: the architecture document, the requirements document, the requirements process guide, and the Mermaid reference guide (paths in `_context.md`), as resolved absolute-path strings. If the conditional-guide trigger applies (`_context.md`, Domain-specific), also include the conditional guide. Pass the same FILES to both reviewers.
   - **CRITERIA**: give BOTH reviewers every item from the Technical correctness checklist and the Readability and consistency checklist below, copied verbatim, as the shared floor — but each reviewer reports only items within its mandate, raising an out-of-mandate item ONLY as a one-line OUT-OF-LANE FLAG when it actually spots a potential issue there (it does not enumerate clean out-of-mandate items). Reviewer B additionally applies its operational mandate above beyond the checklists.
   - **Synthesis**: after the parallel panel returns, YOU (not a subagent) merge the two outputs per the Panel-mode "Synthesize" step in `.claude/commands/_lib/review-subagent-pattern.md` — dedup overlapping findings, reconcile conflicting severities to the higher, and reconstruct any cross-cutting issue sitting in the seam between the two mandates (a structural choice with an operational consequence). Then run the fix / re-review loop on the merged findings.

   Extra rule: do not commit yet. After this engineering review, a Japanese
   prose-quality pass runs (step 9 below); commit only after BOTH passes are complete
   and all Critical and Major issues from both are resolved.

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

9. Run the Japanese prose-quality pass by invoking the `japrose` command
   (`.claude/commands/japrose.md`) on the created architecture document (path in
   `_context.md`). It runs a technical-editor review focused on natural Japanese —
   literal-translation tone, unusual/incorrect word usage, hard-to-follow sentences,
   terms used before they are defined (so the document reads top-to-bottom), and
   terminology consistency with the glossary — and applies the fixes. The engineering
   review (step 8) checks design correctness; this pass checks that the document reads
   as clear, natural Japanese. The two are complementary.
   - Run this **after** step 8's Critical and Major issues are resolved, so prose is
     not polished on text that step 8 then rewrites.
   - `japrose`, when invoked as this sub-step, does not commit. After both step 8 and
     step 9 are complete and all Critical and Major issues from both passes are
     resolved, commit the created architecture document.

When finished, provide a concise summary of what you created and any assumptions you had to make.
