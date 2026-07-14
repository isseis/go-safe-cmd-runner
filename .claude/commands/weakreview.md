> **Project context (read first)**: Read `.claude/commands/_context.md`. It is the
> single source of truth for the task root, guide paths, document/status
> conventions, build checks (`make fmt`/`make test`/`make lint`/`make deadcode`),
> the green gate, source layout, and test-helper placement. Where this command
> names such a path or command, treat the entry in `_context.md` as canonical.
> The review step uses the shared procedure in
> `.claude/commands/_lib/review-subagent-pattern.md`.

## Purpose

`runplan.md` is written assuming the executing model reliably applies every
rule it states. In practice, a lower-capability model following the same
`runplan.md` steps tends to satisfy the *literal* instruction (checkbox
ticked, code compiles, tests pass) while missing the *judgment* the
instruction actually depends on: noticing an implicit invariant, keeping
several documents in sync, resisting a shortcut, or applying an exception
correctly. `code-review` and `simplify` already cover general correctness and
cleanup; this command is a **targeted second pass** for the specific failure
patterns a weaker model produces when executing `runplan.md` — it assumes the
generic review already ran (or runs alongside it) and does not duplicate its
checklist.

Use this after a phase group (or a full task) was implemented by a
lower-capability model, before merging.

## Scope

Argument `$ARGUMENTS`: a commit range (e.g. `HEAD~N..HEAD`), a PR number, or
empty (defaults to `origin/main...HEAD` on the current branch). Resolve it to
a concrete commit range yourself before continuing — the checklist below
needs `git diff <range>` and `git log <range>` to work from.

## Procedure

1. Identify the target task directory per
   `docs/dev/developer_guide/task_identification.md`, and resolve the commit
   range from `$ARGUMENTS` as above.

2. Read the task's `01_requirements.md`, `02_architecture.md`,
   `03_implementation_plan.md`, `docs/dev/developer_guide/test_organization.md`,
   `docs/dev/developer_guide/requirements_process.md`, and
   `docs/design/security.md`. These are the documents whose rules the weak
   model was supposed to have followed while producing the diff.

3. Run the deterministic pre-checks below over the changed files in the
   resolved range. These are mechanical and cheaper than AI review — catch
   them here, not in the subagent pass.

   ```bash
   RANGE="<resolved range>"
   CHANGED_GO=$(git diff $RANGE --diff-filter=d --name-only | grep '\.go$' || true)
   CHANGED_ALL=$(git diff $RANGE --diff-filter=d --name-only || true)

   # Check 1: planning-doc references leaked into source (should live only in the plan doc)
   if [ -n "$CHANGED_GO" ]; then
     echo "$CHANGED_GO" | xargs rg -n '\bAC-[0-9]+[a-z]?\b|\bF-[0-9]+[a-z]?\b' 2>/dev/null \
       && echo "FLAG: planning-doc references in Go source" || echo "OK: no planning-doc references"
   fi

   # Check 2: draft-status documents with commits already built on top of them
   NON_APPROVED=""
   for f in 01_requirements.md 02_architecture.md 03_implementation_plan.md; do
     path="docs/tasks/<task-dir>/$f"
     rg -n '^\| Status \|' "$path"
     if [ -f "$path" ] && ! rg -q '^\| Status \|.*approved' "$path"; then
       NON_APPROVED="$NON_APPROVED $f"
     fi
   done
   # Any non-`approved` status for a document whose successor exists (e.g. a
   # 03_implementation_plan.md exists while 02_architecture.md is still draft)
   # means implementation started before its gate — FLAG regardless of what
   # the commit history says about being told to proceed.
   if [ -n "$NON_APPROVED" ] && [ -f "docs/tasks/<task-dir>/03_implementation_plan.md" ]; then
     echo "FLAG: non-approved doc(s) with a successor doc present:$NON_APPROVED"
   else
     echo "OK: no non-approved doc precedes a successor doc"
   fi

   # Check 3: Dockerfile base images pinned by tag instead of digest
   CHANGED_DOCKERFILES=$(git diff $RANGE --diff-filter=d --name-only | grep -i dockerfile || true)
   if [ -n "$CHANGED_DOCKERFILES" ]; then
     echo "$CHANGED_DOCKERFILES" | xargs rg -n '^FROM .*:[a-zA-Z0-9._-]+\s*$|^FROM .*:[a-zA-Z0-9._-]+\s+[Aa][Ss]\s+\S+\s*$' 2>/dev/null \
       && echo "FLAG: tag-pinned FROM (expect @sha256: digest)" || echo "OK: no tag-only FROM lines"
   else
     echo "OK: no Dockerfile changes"
   fi

   # Check 4: build-tagged files actually reachable by `make lint`
   [ -n "$CHANGED_GO" ] && echo "$CHANGED_GO" | xargs rg -l '^//go:build' 2>/dev/null
   # For each hit with a tag other than a bare `test`, confirm the Makefile's
   # lint target actually passes that tag (grep the Makefile / CI config for
   # -tags). A file only ever compiled under a tag no invocation passes is a
   # silent dead corner.

   # Check 5: non-ASCII in Go source (same as runplan.md step 6.5 — re-verify,
   # since a weak model's self-check in runplan step 5 may have been superficial)
   [ -n "$CHANGED_GO" ] && echo "$CHANGED_GO" | xargs rg -Pn '[^\x00-\x7F]' 2>/dev/null
   ```

   Report every FLAG before continuing to the subagent pass — do not let the
   subagent re-discover what a grep already found.

4. Spawn the review subagent via `.claude/commands/_lib/review-subagent-pattern.md`
   with these inputs:
   - **ARTIFACT**: the code changes in the resolved commit range.
   - **PERSONA**: a skeptical senior Go engineer and senior SRE who has
     reviewed many PRs produced by a lower-capability LLM executing a
     detailed implementation plan. Their default assumption is that any
     instruction requiring cross-document synchronization, an implicit
     invariant, or resisting a shortcut was *not* actually carried out just
     because the surface artifact (checkbox, test, comment) suggests it was
     — each such claim must be checked against the actual diff, not taken on
     faith.
   - **FILES**: `01_requirements.md`, `02_architecture.md`,
     `03_implementation_plan.md`, `docs/design/security.md`, and every file
     touched in the resolved range (all as resolved absolute paths). Include
     the exact commit range and instruct the subagent to run
     `git diff <range>` and `git log <range>` itself.
   - **CRITERIA**: every item in the "Weak-model failure checklist" below,
     copied verbatim, plus the pre-check FLAGS from step 3 (so the subagent
     verifies them in context rather than re-deriving them).

5. After the subagent returns, run the fix / re-review loop as defined in
   `_lib/review-subagent-pattern.md`. This command does not auto-fix by
   default — report findings grouped by severity, and apply fixes only if
   the invocation explicitly asked for `--fix` (mirrors `code-review`'s
   convention).

## Weak-model failure checklist

(Use verbatim as evaluation criteria in the subagent prompt. Each item names
the concrete thing to check, not just the underlying rule, since these are
exactly the checks a weaker executing model would have applied only
superficially.)

**Invariant discipline**
- [ ] For every generated identifier (IDs, names, per-call tokens), the code
      enforces uniqueness/length constraints the plan or architecture implies
      — not just "looks unique in the happy path". Check the actual generation
      site, not just its call sites.
- [ ] Every flag or mode documented as suppressing side effects (`--dry-run`
      and equivalents) is checked at *every* side-effecting call site it
      should gate (deletes, unfollows, network sends), not only the ones
      exercised by the tests actually added in this range.
- [ ] Every session-scoped or resource-scoped block (`WithSession`-style
      helpers, opened clients, acquired locks) releases its resource on
      **every exit path**, including panics and early returns/errors — not
      only the success path. Read the actual `defer`/`recover` placement,
      don't infer it from a comment saying it does.

**Cross-document synchronization**
- [ ] If this diff's implementation diverges from `02_architecture.md` or
      the corresponding step description in `03_implementation_plan.md`
      (a simpler approach was substituted, an assumed API turned out
      different), the plan's *description text* (not only its checkbox) and
      the architecture section were updated in the same commit range. A
      `[x]` with stale prose describing the old approach is a finding.
- [ ] `03_implementation_plan.md`'s Acceptance Criteria Verification section
      still names real test locations (`path::TestName`) or real static
      commands for every AC this range claims to close — not a placeholder
      or a description of a test that does not exist under that name.

**Approval-gate integrity**
- [ ] No implementation commit in this range is dated/ordered before its
      governing document (`01_requirements.md`/`02_architecture.md`/
      `03_implementation_plan.md`) reached `approved` status. If any
      commit message or plan history suggests the model proceeded because
      a user asked it to despite a draft-status gate, flag it regardless of
      the user request — the gate has no override clause.

**Traceability leakage**
- [ ] No `AC-NN` / `F-NNN` planning identifiers appear in Go source comments,
      identifiers, or string literals (confirm the step-3 grep result was
      accurate, i.e. no escaped/split variant like `AC` + `-01` across a
      line break defeats it).
- [ ] Conversely, every AC this range claims to satisfy has a corresponding
      entry in the plan's Acceptance Criteria Verification table — a test
      added in the diff with no plan-side traceability entry is a finding
      (the model implemented the behavior but skipped the paperwork the
      process depends on for future audits).

**Shortcut resistance**
- [ ] Any removal of 3+ scattered same-shaped code sites (test functions,
      duplicated blocks) in this range was done as discrete edits, not a
      generated brace-counting / regex-based bulk deletion — check for
      accidental over-deletion of adjacent code (a truncated function, a
      dangling brace, an orphaned comment) as the tell.
- [ ] Any newly authored non-code artifact in this range (runbook step,
      command example, table, translated text) was checked against ground
      truth (the command was actually run, the cited implementation exists
      at the cited location, the translation matches its source) — not
      verified only by confirming an old term is absent.

**PR/phase boundary judgment**
- [ ] The set of changes in this range corresponds to a coherent phase
      group per `03_implementation_plan.md` — it doesn't stop mid-phase in
      a state that cannot pass `make test` on its own, and it doesn't bundle
      unrelated later-phase work that the plan intended to separate.

**Security-adjacent omissions** (judgment-heavy; a weak model satisfies the
ticket but misses the neighboring risk — check `docs/design/security.md`'s
attack-vector list against this diff even where no AC mentions it explicitly)
- [ ] Any new or modified network response handling (HTTP client reads,
      pagination loops) bounds response size and iteration count — an
      unbounded `io.Copy`/`Decode` or a pagination loop whose only stop
      condition is a value an untrusted server controls is a finding, per
      the `doXRPC`-style gap example in `docs/design/security.md`.
- [ ] Any new error path that wraps an HTTP client error, request, or
      response object does not carry `Authorization` headers or session
      tokens through to a log line or notification payload — check what the
      wrapped struct actually serializes to, not whether the code "looks
      like" it redacts.
- [ ] Any new external-endpoint resolution (DID/PDS-style host resolution,
      redirect following, webhook target validation) fails closed on
      mismatch/ambiguity rather than falling back to a default or a
      best-effort continue.
- [ ] Any new retry/backoff loop has a bounded ceiling (attempts or elapsed
      time), not an open-ended loop relying on the remote side to eventually
      succeed or fail.
- [ ] Any new interruption/partial-failure path (timeout mid-operation,
      crash-and-resume) is treated as "outcome unknown, safe to retry" and
      not silently promoted to either "succeeded" or "failed" — check this
      against the operation's actual idempotency guarantee, not the
      surrounding code's comment.

**Language and convention discipline**
- [ ] Every Go comment, identifier, and string literal added in this range
      is English; any non-ASCII found by the step-3 grep is in a test-data
      literal or an intentionally localized error string, not an
      identifier or a non-test comment (a weak model tends to apply this
      rule uniformly instead of case-by-case).
- [ ] Any new test file's assertion style (testify `assert`/`require` vs.
      hand-rolled `if`/`t.Errorf`), package naming (`testutil` vs.
      package-internal `test_helpers.go`), and import style match the
      convention of an existing file in the same package — evidence the
      model actually read a sibling file rather than generating from a
      generic template.
- [ ] No logic is reimplemented where an existing function in the codebase
      already does it — check for a near-duplicate helper, not just an
      identical one (weak models often rename rather than reuse).

## Output format

Report, grouped by severity (Critical / Major / Minor) as in
`_lib/review-subagent-pattern.md`, plus one line per checklist category
above stating whether it was clean, flagged, or not applicable to this
range. End with a one-line overall read: is this range's failure profile
consistent with a lower-capability model cutting corners on judgment-heavy
rules (expected — fix and move on), or does it suggest the plan/architecture
itself was unclear (escalate: revise the plan before continuing further
phase groups with the same model).
