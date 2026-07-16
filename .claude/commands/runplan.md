> **Project context (read first)**: Read `.claude/commands/_context.md`. It is the
> single source of truth for every project-specific value below — the task root,
> guide paths, document/status conventions, build checks (`make fmt`/`make test`/
> `make lint`/`make deadcode`), the green gate, source layout, and test-helper
> placement (`testutil/`, `test_helpers.go`, `//go:build test`). Where this command
> names such a path or command, treat the entry in `_context.md` as canonical. The
> domain-specific invariant examples in step 5 (ULID test IDs, `--dry-run`
> side-effects, privilege-restoration teardown) are illustrative for this project; see
> `_context.md` (Domain-specific) before reusing them elsewhere. When porting,
> follow the porting steps in `_context.md`: that includes editing this command body
> for domain-specific examples (step 5) and for Go-specific rules (`testutil/`,
> `test_helpers.go`, `//go:build test`) when changing tech stacks. The review step
> uses the shared procedure in `.claude/commands/_lib/review-subagent-pattern.md`.
> Before starting an implementation session, check the `**実装モデル要件**` field of
> the `### PR-N 作成ポイント` marker covering the work you are about to pick up (see
> `_context.md`, "PR marker conventions"): `frontier-required` means run this session
> on a frontier model; `frontier-recommended` prefers one; `standard` may run on a
> cheaper model, with `weakreview.md` as the follow-up check. This does not change
> any step below.

Your goal is to implement one task under `docs/tasks/` by following its `03_implementation_plan.md`.

Work in order.

1. Identify the target task per `docs/dev/developer_guide/task_identification.md`.

2. Read `03_implementation_plan.md`. If the document status is not `approved`, stop and report.

2.5. Check whether PR boundary design is needed.
   - Count `### フェーズ` headers in the plan. If there are 2 or more and no `### PR-` sections exist, invoke the `/mkplan2` skill to design PR boundaries before proceeding.
   - After `/mkplan2` completes, re-read the implementation plan document so the updated content (with PR markers) is in context.
   - If PR markers already exist, skip this step and continue.

3. Read `01_requirements.md`, `02_architecture.md` (both in the target task directory), and `docs/dev/developer_guide/test_organization.md`.

4. Select the next unit of work from `03_implementation_plan.md` checkboxes (`[ ]` not started, `[x]` done, `[-]` skipped).
- If all phases and all PR markers are complete, skip to step 8 and follow the "If all phases are complete" bullet.
- Scan forward from the last completed item. If the next unchecked items are inside a `### PR-N 作成ポイント` section (PR checkpoint checkboxes), treat this as a PR checkpoint — go to step 5a instead of step 5.
- Otherwise, select the next implementation group: use one phase by default unless it cannot pass `make test` alone (e.g. stub-only or tightly coupled); then extend the group until it can pass. Stop the group at the next `### PR-N 作成ポイント` boundary — do not include the PR checkpoint checkboxes in the implementation group. Briefly note the reason for any grouping before starting work.

5. Implement the selected phase group.
- Follow the design in `02_architecture.md`.
- Before writing new files in a package, read at least one existing file of the same kind (e.g. a `_test.go` in the target package) to confirm assertion library, import style, and helper conventions in use. Mismatches with the established style will be caught in review — reading first avoids the rework.
- **State invariants before coding.** For any generated value (IDs/names), flag/mode, or side-effecting operation, write its contract in one line as a code comment above the implementation and implement to that — not to the first approach that compiles. Examples that would have prevented real bugs: a test ID → *unique per call, within the protocol length limit* (a bare ULID, no test-name prefix); `--dry-run` → *no external side effects* (skip every write and network send, not just notifications); a privilege-elevation block → *always restores original privileges before returning, even on panic or error* (`WithPrivileges` defers `restorePrivileges`, not just on the success path).
- Place test helpers per `docs/dev/developer_guide/test_organization.md`: cross-package helpers under `testutil/`; package-internal helpers in `test_helpers.go` (or `test_helpers_<category>.go`) with `//go:build test`.
- After each file change (Go or otherwise), run `make test && make lint`; for Go file changes also run `make fmt` first. Fix errors before continuing. Exception: errors caused by the phase group's incomplete state (e.g. build or test failures from missing implementations that stubs depend on) need not be fixed until the group is complete; fix only errors unrelated to the in-progress group.
- When removing multiple scattered code sites (e.g. several test functions), delete them one at a time using the exact text read from the file. Do not script bulk deletion (e.g. a brace-counting loop); nested literals make such heuristics over-consume adjacent code. After each removal, check IDE diagnostics for unintended breakage.
- For any newly authored or substantially rewritten artifact in this group (runbook, command example, table, prose, translation, design-doc section), confirm it is correct before committing — do not rely on absence-search of removed terms. Run any documented command and check its exit code and output; cite the source implementation that prose describes; diff a translation against its source. For a design document such as an ADR, also keep the body on the current system and confine removed-design rationale to a bounded history note rather than interleaving it.
- **Before committing each group, self-check** (catches common defects before the step 7 review):
  - **If the implementation approach diverges from `02_architecture.md` or the plan step descriptions** (e.g., a simpler alternative is adopted, an API turns out to have compatibility issues): update (1) the plan step *description* text (not only the checkbox) and (2) any architecture document section that no longer matches in the same commit, and (3) the PR description if one exists for this group. A divergence that is only marked `[x]` without updating the description text will appear as a plan/code inconsistency to reviewers and generate stale review threads.
  - No planning-doc references (`AC-01`, `F-001`) in source comments or strings — put the *why* in plain English.
  - Validators/parsers/env-checks have a happy-path test (all-valid → no error), not only failure cases.
  - A helper guarding a precondition calls its own guard internally; callers shouldn't have to.
  - Sub-test names match what they test (`smtp_host_missing` tests `IMAP_TEST_SMTP_HOST`, not `IMAP_TEST_HOST`).
  - Reuse existing utilities before writing new ones (e.g. `ulid.Make()`); consolidate duplicate regexes/constants.
  - Test-only behavior stays in test packages; don't branch production code for tests.
  - A Go test file importing a `testutil` that imports the package under test uses `package foo_test` to avoid an import cycle.
  - Any new or modified file with build tags beyond `//go:build test` alone (e.g. `//go:build test && foo`): confirm it is reached by at least one `make lint` invocation, or explicitly document the gap. Ask: "does `make lint` compile this file?"

- When complete, update checkboxes (`[x]` done, `[-]` skipped with a note) and commit.

5a. **PR checkpoint** (reached when step 4 directed you here instead of step 5).
- Verify the green gate (defined in `_context.md`) passes. Fix any failures before continuing.
- Mark the first PR checkpoint checkbox (the green gate confirmation line) as `[x]` and commit.
- Push the current branch with an upstream tracking reference (e.g. `git push -u origin HEAD`) so `gh pr create` does not prompt interactively for where to push.
- Run `gh pr create --title "<推奨タイトル>" --body "<レビュー観点を含む本文>"`, using the `推奨タイトル` value from the `### PR-N 作成ポイント` section as `--title` and including the `レビュー観点` items in `--body`. Use explicit flags to avoid interactive prompts.
- Output the PR URL, mark the second checkbox (`PR を作成した`) as `[x]`, commit, and `git push` so this update is included in the PR before it is merged.
- Pause and ask the user: "PR-N を作成しました: <URL>。マージされたらお知らせください。"
- Wait for the user to confirm the PR is merged. Then:
  - Update the local base branch so it includes the merge (e.g. `git checkout main && git pull`), then create a new branch for the next group of work from it (e.g. `git checkout -b <feature-branch>-<N+1>`). Without this, especially after a squash or rebase merge, the new branch would still be based on the pre-merge commits.
  - Mark the remaining PR checkpoint checkboxes (`PR がマージされた` and `次のブランチへ切り替えた`) as `[x]` and commit.
- Return to step 4.

6. Run `make deadcode`. Remove functions made unreachable by this phase group; keep intentional scaffolding for future phases or tasks. If changes were made, run `make fmt && make test && make lint` and commit.

6.5. Run programmatic pre-checks on the changed Go files before spawning the review agent. These checks are deterministic and cheaper than AI review — catch them here rather than in the review loop.

   ```bash
   # Files changed in this phase group — exclude deleted files so rg never
   # receives a path that no longer exists (which would exit 2, masking matches)
   CHANGED=$(git diff origin/main...HEAD --diff-filter=d --name-only | grep '\.go$' || true)

   if [ -n "$CHANGED" ]; then
     # Check 1: no planning-doc identifiers in source
     if echo "$CHANGED" | xargs rg -l '\bAC-[0-9]+[a-z]?\b|\bF-[0-9]+[a-z]?\b' 2>/dev/null; then
       echo "FAIL: planning-doc references found — fix before continuing"
     else
       echo "OK: no planning-doc references"
     fi

     # Check 2: no non-ASCII characters in Go source
     if echo "$CHANGED" | xargs rg -Pn '[^\x00-\x7F]' 2>/dev/null; then
       echo "REVIEW: non-ASCII found — verify each is intentional"
     else
       echo "OK: all ASCII"
     fi
   else
     echo "OK: no Go files changed"
   fi
   ```

   - If Check 1 has any matches: fix them, run `make fmt && make test && make lint`, commit, then continue (these are never intentional in source).
   - If Check 2 has matches: inspect each — test-data literals and error strings may legitimately contain non-ASCII, but identifiers and non-test comments must not. Fix any unintentional occurrences, run `make fmt && make test && make lint`, and commit before continuing.

7. Run the critical-review subagent procedure in `.claude/commands/_lib/review-subagent-pattern.md` with these inputs:
   - **ARTIFACT**: this phase group's code changes.
   - **PERSONA**: an experienced senior Go engineer and senior SRE. Direct it to surface bugs, missing test coverage, architecture drift, and unclear code.
   - **FILES**: the architecture document and the implementation plan document (paths in `_context.md`), as resolved absolute-path strings; instruct the subagent to read both in full before evaluating the code. Also list the source files added or modified in this phase group as resolved absolute-path strings (read in full). Provide the specific commit range for this phase group (e.g., `HEAD~N..HEAD`) and instruct the subagent to run `git diff <range>` to see exactly what changed.
   - **CRITERIA**: every item from the phase-group review checklist below, copied verbatim.

   Extra rule: when fixing Critical and Major issues, run the build checks (defined in `_context.md`) and commit before spawning the verification pass.

Phase-group review checklist (use verbatim as evaluation criteria in the subagent prompt above):
- [ ] Implementation is consistent with `02_architecture.md`.
- [ ] Every AC assigned to this phase group by the implementation plan has at least one `test` or `static` verification, as labeled in the implementation plan's Acceptance Criteria Verification section.
- [ ] Test coverage is sufficient: non-trivial logic, error paths, and boundary values are covered.
- [ ] No tests duplicate existing coverage without good reason.
- [ ] No tests are so trivial that they add no verification value.
- [ ] No logic is reimplemented when an existing function in the codebase can be used.
- [ ] Newly authored artifacts (runbooks, command examples, tables, prose, translations) are verified against ground truth, not only by absence-search.
- [ ] All source comments and identifiers are in English. (Non-ASCII flagged by step 6.5 Check 2 has been reviewed and is intentional.)

8. Decide whether to continue or finish.
- If implementation ran this iteration: summarize implementation, verified ACs, assumptions, and deferred items.
- If phases remain: ask "Shall I continue with the next phase group?" — return to step 4 if agreed, otherwise report status and stop.
- If all phases are complete: verify every AC in `01_requirements.md` is satisfied by the implementation and has at least one test. Report the final status and any gaps.
