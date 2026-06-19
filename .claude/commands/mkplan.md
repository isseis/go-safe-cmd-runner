> **Project context (read first)**: Read `.claude/commands/_context.md`. It is the
> single source of truth for every project-specific value below — the task root,
> guide paths, document names, status values (`draft`/`approved`), build checks,
> source layout (`cmd/`/`internal/`, `testutil/`), and document language. Where
> this command names such a path or value, treat the entry in `_context.md` as
> canonical. The domain-specific examples in steps 5–6 (the symbol-verification
> `rg` example and the `FormatLogFileHint` before/after string-literal example)
> are illustrative for this project; see `_context.md` (Domain-specific) before
> reusing them elsewhere. When porting, follow the porting steps in `_context.md`:
> that includes editing this command body for domain-specific examples (steps 5–6)
> and for Go-specific test-helper rules (`testutil/`, `test_helpers.go`,
> `//go:build test`) when changing tech stacks. The review step uses the shared
> procedure in `.claude/commands/_lib/review-subagent-pattern.md`.

Your goal is to create `03_implementation_plan.md` for one task under `docs/tasks/`.

Work in the following order.

1. Identify the target task directory by following the rules in `docs/dev/developer_guide/task_identification.md`.

2. Read `02_architecture.md` in the target task directory.

3. Verify that implementation planning is allowed.
- Check the document status in `02_architecture.md`.
- If the status is not `approved`, do not create `03_implementation_plan.md`.
- In that case, stop and report that implementation planning cannot begin until `02_architecture.md` is `approved`.

4. Read the remaining required input documents.
- `01_requirements.md` in the target task directory
- `docs/dev/developer_guide/requirements_process.md`
- `docs/dev/developer_guide/test_organization.md`

5. Inspect the current codebase before writing the plan.
- Check the relevant packages, tests, and test helpers under `cmd/` and `internal/`.
- Identify existing functions, tests, and helper utilities that should be reused. Do not plan to re-implement logic or add duplicate tests when the repository already has suitable coverage or reusable helpers.
- For each relevant area, note: what already exists, what is missing, and what needs to change. Omit areas where the existing code requires no attention.
- **Verify symbols and enumerate all instances before writing.** Confirm every cited function, test, variable, or error name exists in the expected file; note which file when similar names appear in multiple locations. For any symbol or pattern the plan will change or delete, find ALL occurrences across code, comments, test names, error strings, docs, and translations. Cover all search variants: function/variable names, CLI flags, error messages, numeric values, old terminology, and comment phrases. Map each result to its enclosing function and planned action. Add explicit cleanup tasks for stale comments and documentation.
  - Example: `rg -n "func TestDefaultMessageFormatter_FormatLogFileHint" internal/logging/` to confirm the test file; `rg -n "\.FormatLogFileHint\(" -g '*.go' internal` to map every call site.
- **Trace behavioral impact and coverage gaps.** When a function's return values or error conditions change, enumerate its callers and assess whether their observable behavior also changes. Before deleting a test, confirm each non-trivial invariant it verifies is covered by a surviving test, or add an explicit replacement test to the plan.
- **Verify external-tool, platform, and environment assumptions against ground truth.** Every command, config snippet, build/test flag, container image, or CI service definition the plan prescribes must be checked against an authoritative source before being written: the image's actual Dockerfile/contents for installed binaries (`bash`/`nc` may be absent), the CI platform's schema for supported keys (GitHub Actions `services.<id>` supports `options:`, not Compose's `healthcheck:`), the tool's own `go help`/`--help` for flag semantics (`go test -c -o` writes one file and fails for multi-package patterns), and the relevant RFC/spec or server behavior for protocol constraints. Prefer runtimes already guaranteed by the project's dev/CI environment (Go) over introducing new dependencies (Ruby, bashisms). Note the verification source for each non-trivial external assumption.
- **Enumerate analogous tasks, not just symbols.** The "all instances" rule above also applies to setup, teardown, suppressions, and constraints shared across analogous tasks: when sibling steps follow the same pattern (parallel tests, duplicated helpers, repeated CLI invocations), confirm each includes the same mandatory setup (e.g. a shared `loadXxxConfig(t)` first step), cleanup, and constraints, and apply any single-instance correctness fix to every instance (e.g. the same fix in both a Phase 2 and a Phase 4 helper).

6. Create `03_implementation_plan.md` in the same task directory.

   **[Always required] structural and traceability rules — apply to every plan:**
   - Write in Japanese; set the document status to `draft`.
   - Include all required sections from `docs/dev/developer_guide/requirements_process.md`, plus explicit top-level sections for: Implementation Order and Milestones, Test Strategy, Implementation Checklist, Acceptance Criteria Verification. Do not let phase task lists implicitly substitute for these.
   - Add a `既存コード調査結果` subsection under the implementation overview (§1) with the findings from step 5. If none, state explicitly that no existing code changes are required.
   - Organize work into small, phase-based steps with checkboxes. When a step deletes multiple distinct, separately-named entities (e.g. several test functions), give each its own checkbox rather than a batch instruction (`Test*_全件削除`). (Uniform edits across many call sites may still be described by one pattern — see "change sites" below.)
   - Map each acceptance criterion to the tasks and tests that verify it. Every AC row in the Acceptance Criteria Verification section must name either an exact test location in `path::TestName` format, or an explicit static verification command with its expected result. Do not use vague labels ("compile passes", "document review", "grep check", "none"); spell out the exact `rg` command and what counts as success.
   - Label each AC verification as `test` (executable, fails on wrong behavior), `static` (rg/grep/compile), or `manual` (PR observation, deploy check). Every AC must have at least one `test` or `static` entry; `manual` supplements but never replaces them. A static `rg` check is valid only when the AC is purely about textual presence/absence; behavior encoded in scripts, env handling, or command routing needs an executable test (for workflow YAML declarative conditions such as `if:` expressions, manual verification on a test branch is acceptable).
   - Keep tasks actionable, observable, and small enough to complete and verify. Include specific file paths where confidently known.
   - Add a cross-search checklist **only** for items that `make lint` and `make test` cannot detect: deleted/renamed symbols with potential residual references, cross-package naming conflicts for generic identifiers, and doc/glossary consistency. Do not duplicate items already covered by the AC verification table — if the same `rg` command appears in both the AC table and a cross-search section, keep it in the AC table only. For purely additive tasks (new symbols with a unique prefix, single-package scope), a cross-search checklist is likely unnecessary.
   - Any planned Go source comment, identifier, or string literal must be written in English (the plan prose itself may be Japanese).
   - Reconcile templated/boilerplate sections with the document's actual state: the "next steps"/remaining-work section must not list steps already completed for this document (e.g. do not list reviewing or approving this plan when its status is past `draft`).

   **[When applicable] apply only when the trigger matches:**
   - Documentation-only ACs: create concrete verification tasks; include every touched doc (glossaries, translations) in the AC table or cross-search checklist. Example: `rg -n -e "old term" docs/file.md` expected: no matches except allowed historical notes.
   - New authored content (prose, tables, command examples, runbooks, translations): verify against ground truth, not only absence-search — run the documented command and confirm exit code/output, cite the source implementation the text describes, or diff a translation against its source. When a table groups several cases under one entry, confirm their behavior/remediation is truly identical, else split the entry.
   - Text edits to a string literal, error message, or source comment: state the complete before/after string, not just the substring to change. This prevents dropped prefixes, dangling `%w`, or stray trailing spaces.
     - Bad:  "Remove ` for more details` from `FormatLogFileHint`'s no-color message."
     - Good: "Change `FormatLogFileHint(100, false)`'s return value from `"HINT: Check log file around line 100 for more details"` to `"HINT: Check log file around line 100"`."
   - Change sites spread across many locations: describe them by grep pattern (e.g. "all `_ = notifyXxx(...)` call sites"), not line numbers, which go stale. Use line numbers only when the pattern alone cannot locate the spot.

7. Apply test helper planning rules from `docs/dev/developer_guide/test_organization.md`.
- If new cross-package helpers or mocks are needed, plan them under `testutil/` with the correct file naming and package naming rules.
- If package-internal helpers are needed, plan them as `test_helpers.go` or `test_helpers_<category>.go` with `//go:build test`.
- Do not add helper files in the plan unless they are actually needed.

8. Run the critical-review procedure in `.claude/commands/_lib/review-subagent-pattern.md` with these inputs:
   - **ARTIFACT**: the created implementation plan document (path in `_context.md`).
   - **PERSONA**: default to a **single reviewer** — an experienced senior engineer and senior SRE. A single reviewer is sufficient for routine plans, because the plan checklists below are already exhaustive and leave little room for a combined reviewer to under-cover one lens. Use **panel mode** (per `.claude/commands/_lib/review-subagent-pattern.md`, two partitioned reviewers run in parallel) only for **high-risk or large plans** — e.g. a heavy integration-test / CI / external-resource surface, or a security-gate / migration plan (staged rollout, simultaneous behavior raises and lowers, many test updates). When panel mode is warranted, partition by **what to build vs. how to verify it** (the natural seam of a plan), NOT engineer-vs-SRE (those overlap on a plan):
     - **Reviewer A — implementation (SWE).** Mandate (go deep): every AC is addressed by ≥1 implementation task (AC→impl coverage); reuse / DRY vs re-implementation; file paths and package-boundary fit; **phase names/order match the approved architecture's priorities (this is the Conditional-checks phase-order item — it is A's, not B's)**; external-assumption verification (tool-flag semantics, real API/lifecycle-method existence, image/CI schema); English-only (no Japanese) in **production** Go comments, identifiers, and string literals (per step 6); the plan references architecture instead of duplicating it. Out-of-scope (defer to Reviewer B — one-line OUT-OF-LANE FLAGS): test coverage/quality, test-infra isolation, helper-file organization, the Conditional checks **other than the phase-order item**.
     - **Reviewer B — test / test-infra (SDET).** Mandate (go deep): every AC has at least one concrete test/static verification task and an explicit AC-verification section (AC→test coverage); tests cover non-trivial logic, error paths, boundary values; creation tasks have ground-truth verification; no unjustified duplicate tests; test-helper files follow the test-organization guide; **the same English-only rule (no Japanese in comments, identifiers, or string literals) applied to Go test source** — `*_test.go`, `testutil/` helpers, AND package-internal test helpers `test_helpers.go` / `test_helpers_<category>.go` (`//go:build test`), which are Go source too and fall in B's lane not A's; AND **every Conditional check below except the phase-order item** — i.e. the entire Conditional section (rerun isolation, normalization helpers, linter-suppression scoping, CI scripted-logic extraction, env-skip purity, resource cleanup/leak at acquisition, repeated-CLI startup validation, real cleanup/close/logout API existence **for test-code lifecycle calls**, build-tag compilation, …; this list is illustrative, B owns all of them). Out-of-scope (defer to Reviewer A — one-line OUT-OF-LANE FLAGS): production-code task decomposition, file paths, phase ordering, reuse of production code, and real-API/lifecycle existence for **production** code (that is A's).
     - The **seam both reviewers cover** is AC traceability — A checks AC→implementation-task, B checks AC→test-task — so the merged AC→{task,test} matrix catches any dropped AC from both ends; reconcile it in synthesis.
   - **FILES**: the implementation plan document, the architecture document, the requirements document, the requirements process guide, and the test organization guide (paths in `_context.md`), as resolved absolute-path strings. In panel mode, pass the same FILES to both reviewers.
   - **CRITERIA**: every item from the Technical correctness checklist, the Conditional checks section, and the Readability and consistency checklist below, copied verbatim. For Conditional checks, evaluate each item that applies to the plan; mark inapplicable items N/A (N/A is not a finding). In panel mode, give both reviewers all of them as the shared floor (option (b) in `.claude/commands/_lib/review-subagent-pattern.md`), but each reports only items within its mandate, raising an out-of-mandate item ONLY as a one-line OUT-OF-LANE FLAG when it actually spots a potential issue there (it does not enumerate clean out-of-mandate items); the Conditional checks are Reviewer B's, except the phase-names/order item which is Reviewer A's (it is an architecture-alignment concern in A's mandate).
   - **Synthesis** (panel mode only): after the parallel panel returns, YOU (not a subagent) merge the two outputs per the Panel-mode "Synthesize" step in `.claude/commands/_lib/review-subagent-pattern.md` — dedup overlapping findings, reconcile conflicting severities to the higher, reconstruct the full AC→{task,test} matrix from both reviewers' AC checks (so no AC is dropped from either the build or the verify side), and promote any confirmed OUT-OF-LANE FLAG. Then run the fix / re-review loop on the merged findings.

   Extra rule: commit the implementation plan document only after all review passes are complete (in panel mode, after the Synthesis step above) and all Critical and Major issues are resolved.

**Technical correctness checklist (use verbatim as evaluation criteria in the subagent prompt above):**
- [ ] `02_architecture.md` is `approved`.
- [ ] `03_implementation_plan.md` is written in Japanese and its status is `draft`.
- [ ] All required sections from the requirements process guide are present.
- [ ] Every acceptance criterion in `01_requirements.md` is addressed by at least one implementation task.
- [ ] For each acceptance criterion that covers an existing code pattern, the plan addresses ALL instances of that pattern in the codebase. Verify by searching the codebase for the pattern before assessing completeness.
- [ ] The plan includes an explicit acceptance criteria verification section.
- [ ] Every acceptance criterion has at least one concrete `test` or `static` verification task (per the labeling in step 6); `static`-only is valid only for criteria that are purely about textual/documentation presence.
- [ ] Test tasks cover non-trivial logic, error paths, and boundary values where applicable.
- [ ] Creation tasks (new prose, tables, command examples, runbooks, translations) each have a verification task that checks the content against ground truth, not only an absence-search.
- [ ] The plan does not duplicate tests or re-test behavior that existing tests already cover without reason.
- [ ] The plan reuses existing implementations, tests, and helper utilities where appropriate.
- [ ] The plan references architecture sections instead of duplicating design details.
- [ ] The plan does not imply Japanese text in Go source comments, identifiers, or string literals.
- [ ] Any planned test helper files follow `docs/dev/developer_guide/test_organization.md`.
- [ ] Planned file paths are specific where known and do not conflict with existing package responsibilities.
- [ ] Every non-trivial external assumption (container image contents, CI platform schema, tool flag semantics, protocol/server behavior) is verified against an authoritative source (image Dockerfile, platform docs, `go help`/`--help`, RFC), and the plan uses runtimes already available in the project's dev/CI environment rather than introducing new dependencies.
- [ ] When a correctness rule, setup step, constraint, or suppression applies to one instance of a duplicated pattern (sibling tests, parallel helpers, repeated CLI invocations), it is applied to ALL analogous instances — no instance is left out.
- [ ] Templated sections (e.g. "next steps") reflect the document's actual state; actions already completed for this document are not listed as remaining work.

**Conditional checks (apply each only when its trigger matches; mark N/A otherwise — N/A is not a finding):**
- [ ] Integration tests touch external/long-lived state (containers, DBs, mailboxes, queues, files outside `t.TempDir`) → rerun isolation is planned: identifiers unique per run (not just per test name), and tests locate their own artifacts by a stable marker instead of assuming result length/ordering. When a unique identifier is length-limited (e.g. a 32-char mailbox name), the stable prefix is truncated first and the uniqueness suffix appended afterward, so truncation never drops the suffix.
- [ ] Assertions on values returned by external systems (message IDs, headers, URLs) → a normalization helper compares normalized expected vs normalized actual, not exact string equality against the injected value.
- [ ] A security-linter-flagged construct is introduced (`InsecureSkipVerify`, `sql.Open` with interpolation, etc.) → a narrow `//nolint:<linter>` task scoped to the minimum block, with a rationale comment, is planned — never file- or package-wide. The suppression is mirrored on every analogous instance, including non-`_test.go` helpers (e.g. files under `testutil/` built with `//go:build test`), where `_test.go`-only linter exemptions do not apply. Where the plan states the construct's purpose, the wording scopes it to the specific test-only context — it must not read as a general or production capability.
- [ ] CI/workflow or change-detection **scripted logic** (e.g., complex shell steps) has two or more conditions → it is extracted to a standalone script with a test feeding representative inputs and asserting outputs. `actionlint` covers syntax only, not behavior. (Declarative `if:` conditions in GHA control job/step execution at the orchestrator level and cannot be extracted to scripts; manual verification is acceptable for those.) Change-detection predicates additionally cover the scripts and workflows that drive or validate the job itself (e.g. `.github/scripts/`), with a representative test case, so a change to the classifier cannot disable the very job it controls.
- [ ] Environment-variable skip logic exists → detection is a pure `missing...Env(getenv func(string) string) []string` helper with a thin `t.Skip` wrapper, unit-tested for missing required vars, invalid values (e.g. non-integer port), and successful propagation.
- [ ] Phase names/order match the approved architecture's implementation priorities. A prerequisite for a later phase is stated within that phase, not by renumbering; if the order must change, the architecture is revised first.
- [ ] Every cleanup/close/logout call in the plan (`store.Close()`, `conn.Logout()`, `t.Cleanup`, …) names a real API confirmed to exist, or an explicit task to add it — no invented lifecycle methods.
- [ ] Real external resources are acquired in tests (IMAP/DB/connection sessions, created mailboxes) → each acquisition registers an explicit `t.Cleanup`/`defer` close at the acquisition point (not only at test end, and even when later assertions fail), so sessions do not leak and exhaust a long-lived server. Any server-side ordering constraint is stated explicitly (e.g. close/deselect a mailbox before another session deletes it).
- [ ] A CLI/binary is invoked more than once in a test or runbook and each invocation re-runs the same startup validation (config/`Bootstrap`) → environment constraints required for that validation to pass (e.g. `-dry-run` when no webhook is configured) are applied to ALL invocations, not just the first; otherwise an AC can fail before reaching the behavior under test.
- [ ] New non-`_test.go` source is compiled only under build tags (`//go:build test`/`integration`) → the introducing phase's completion gate compiles those files under the SAME tags they will ultimately use (e.g. `go test -run '^$' -tags test,integration ./...`, not a `-c -o /dev/null` form that breaks for multi-package patterns), so type/signature errors surface in that PR rather than a later one.

**Readability and consistency checklist (use verbatim as evaluation criteria in the subagent prompt above):**
- [ ] Terminology is consistent with `02_architecture.md`; the same concept always uses the same Japanese term.
- [ ] Task descriptions are phrased as clear, actionable instructions. Vague verbs (e.g., Japanese equivalents of "handle", "implement", "address") are replaced with specific actions where possible.
- [ ] Redundant or repetitive content is removed; design details already in the architecture document are referenced rather than restated.
- [ ] Ambiguous or overly terse expressions are rewritten in direct, plain Japanese so readers do not need prior context to understand what is expected.
- [ ] When a planning decision changes an artifact's name, extension, implementation language, or invocation, every reference to it (filename, section header, prose, completion/verification command) is updated consistently — no `.sh` filename paired with a `go run …go` description.

When finished, provide a concise summary of what you created and any assumptions you had to make.
