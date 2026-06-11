> **Project context (read first)**: Read `.claude/commands/_context.md`. It is the
> single source of truth for every project-specific value below — the task root,
> document names, status values, the green gate (see `_context.md`), source
> layout (`internal/`/`cmd/`), and the PR marker label/format conventions. Where
> this command names such a value, treat the entry in `_context.md` as canonical.
> When porting, follow the porting steps in `_context.md`. Note that this command
> body also contains Go-specific design principles (`internal/`→`cmd/` ordering
> rule, build-independence gate) that must be updated when porting to a non-Go or
> non-`cmd/`/`internal/` layout. The review step uses the shared procedure in
> `.claude/commands/_lib/review-subagent-pattern.md`.

Your goal is to design PR boundaries for an existing `03_implementation_plan.md` and embed them directly into the document.

Work in the following order.

1. Identify the target task directory by following the rules in `docs/dev/developer_guide/task_identification.md`.

2. Read `03_implementation_plan.md` in the target task directory.
   - If the document status is not `approved`, stop and report. PR boundary design requires an approved plan.
   - If `### PR-` sections already exist, stop and report that PR boundaries are already defined. Do not overwrite them unless the user explicitly asks.

3. Read `02_architecture.md` and `01_requirements.md` in the target task directory to understand the component structure and acceptance criteria.

4. Analyse the implementation steps and design PR groupings.

   Apply the following principles:
   - **Reviewability**: Each PR should have a single, coherent concern that a reviewer can evaluate independently (e.g. "all internal API changes", "cmd-layer wiring", "one high-risk subcommand").
   - **Buildability**: Every PR must pass the green gate (defined in `_context.md`) independently. Never split a tightly coupled unit (interface + implementation + test) across PRs.
   - **Risk isolation**: Place high-risk or complex steps (e.g. recovery flows, concurrency) in their own PRs so they can be reviewed in detail without unrelated noise.
   - **Internal-before-cmd**: Changes to `internal/` packages should land before the `cmd/` layer that depends on them.
   - **Small over large**: Prefer more, smaller PRs over fewer large ones; 3–6 PRs is a reasonable target for a medium-sized feature.

   Decide whether any steps need to be reordered within their phase to align with the PR groupings. Only reorder steps when necessary — do not change step order unless it is required to make a PR group coherent.

5. If reordering is needed, update the step sections:
   - Physically move the step sections into the new order.
   - Renumber steps consistently (e.g. `ステップ 1-1`, `ステップ 1-2`, …) using sequential numbers within each phase.
   - Update all cross-references to renumbered steps throughout the document (§3, §4, §5, §6, and any other section that names step numbers).

6. Insert `### PR-N 作成ポイント` sections after each PR group's last step, using the following format exactly:

   ```
   ### PR-N 作成ポイント: <scope label in English>

   **対象ステップ**: X-Y / X-Z / …

   **推奨タイトル**: `feat(<task-id>): <concise English title>`

   **レビュー観点**: <key1> / <key2> / <key3>

   - [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
   - [ ] PR を作成した
   - [ ] PR がマージされた
   - [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）
   ```

   Where:
   - `N` is a sequential integer starting from 1.
   - `<scope label>` is a short English description of the PR's concern (e.g. `internal API extensions`, `cmd boot layer`, `gc and reprocess subcommands`).
   - `**対象ステップ**` lists the step IDs covered by this PR, separated by ` / `.
   - `**推奨タイトル**` is a conventional commit title in English.
   - `**レビュー観点**` lists 2–4 key review points in Japanese.

7. Add or update a `### 3.2 PR 構成` subsection under §3 (or whichever section covers the implementation overview/test strategy) with a summary table:

   ```markdown
   ### 3.2 PR 構成

   | PR | 対象ステップ | 主な変更内容 |
   |---|---|---|
   | PR-1 | 1-1 / 1-2 / 1-3 | … |
   | PR-2 | 1-4 / 1-5 / 1-6 | … |
   …
   ```

8. Update §6 (checklist or completion criteria section) to be organised by PR rather than by phase. Replace any phase-based checklist entries with PR-based ones:

   ```markdown
   - [ ] PR-1 マージ済み（対象ステップ: X-Y / X-Z）
   - [ ] PR-2 マージ済み（対象ステップ: X-Y / X-Z）
   …
   ```

9. Run the critical-review subagent procedure in `.claude/commands/_lib/review-subagent-pattern.md` with these inputs:
    - **ARTIFACT**: the PR boundary design.
    - **PERSONA**: an experienced senior engineer and senior SRE. Direct it to surface PRs that are too large to review, PRs that cannot be built independently, missing risk isolation, and cross-references that were not updated after renumbering.
    - **FILES**: the implementation plan document, the architecture document, and the requirements document (paths in `_context.md`), as resolved absolute-path strings.
    - **CRITERIA**: every item from the PR boundary review checklist below, copied verbatim.

10. Commit the implementation plan document with a message that explains the PR grouping rationale, only after all review passes are complete and all Critical and Major issues are resolved.

**PR boundary review checklist (use verbatim as evaluation criteria in the subagent prompt above):**
- [ ] Every `### PR-N 作成ポイント` section appears after all steps it covers.
- [ ] Every step belongs to exactly one PR.
- [ ] No step that modifies an `internal/` interface lands in a later PR than the `cmd/` step that depends on it.
- [ ] Each PR group can pass the green gate (defined in `_context.md`) on its own without stubs from future steps.
- [ ] High-risk or complex steps (recovery, concurrency, state machines) are isolated in their own PR or placed last in a PR so they do not block review of simpler changes.
- [ ] The `**対象ステップ**` field in each PR marker lists exactly the steps in that group and no others.
- [ ] The `### 3.2 PR 構成` table is present and consistent with the PR marker sections.
- [ ] §6 checklist entries reference PRs (not phases) and are consistent with the PR markers.
- [ ] All step-number cross-references in the document (§3, §4, §5, §6, step descriptions) reflect the final numbering after any reordering.
- [ ] `推奨タイトル` values are valid conventional commit titles in English.
- [ ] `レビュー観点` items are specific and meaningful — not generic placeholders.

When finished, provide a concise summary of the PR grouping decisions and any assumptions made.
