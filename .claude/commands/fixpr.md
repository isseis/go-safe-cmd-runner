Fix unresolved PR review threads for PR `$ARGUMENTS` (or, if no argument was
given, the PR for the current branch).

There is no `Workflow` tool in this environment, so run the six phases below
**yourself**, sequentially, using the `Agent` tool (`run_in_background: false`)
for each phase — do not try to do the work directly in this conversation, and
do not skip a phase. Each phase's agent has no memory of previous phases, so
its prompt must be self-contained: pass it the exact data it needs (thread
lists, triage results, etc.) inlined as JSON. Instruct every agent to reply
with **only** a single JSON object matching the shape given — no prose, no
markdown code fences — since there is no schema validation to fall back on;
if an agent's reply doesn't parse as JSON, re-invoke it once with that
constraint repeated more forcefully before giving up.

Model tiering (mirrors the original workflow's cost design): use
`model: "haiku"` for the purely mechanical phases (Fetch, Build, Reply, Wrap);
leave `model` unset (defaults to this session's model) for the phases that
require code reasoning (Triage, Fix).

Build checks for this project (see `_context.md` "Tech-stack convention" —
keep this in sync if that table changes): `make fmt && make test && make lint`

---

## Convergence policy

This loop has no natural end: a push triggers a fresh review, and a review
returns findings whenever it is asked. Stop by policy, not by exhausting
findings.

- **Round cap**: apply at most two fixpr rounds to the same PR. From the third
  round, act on `must-fix` only and leave the rest.
- **Severity floor**: once every remaining finding is `no-harm` (cosmetic or
  invalid), the review has converged — stop editing and merge.
- **Batch**: push once per round; a push per finding triggers a review per
  finding.
- **Accept, don't chase**: a finding that is a mirror case of a rule already
  fixed is closed by generalizing that rule (Phase 3), not by another
  per-instance patch. If it recurs after that, record it as accepted risk or a
  new issue and merge; do not reopen the same rule again.
- The Final report states whether the round was substantive or recurring churn
  and whether the floor is reached.

## Shared rules: document levels and the handoff contract

Phases 2 and 3 and the Final report inline this section verbatim into their
agent prompts. Edit these rules here only; do not restate them in the phase
bodies.

**R1 — Document levels (the off-level test).** Each process document states
only what is at its own level:

- `01_requirements.md`: observable behavior — what is accepted, which error or
  exit code a rejection maps to, what state changes. Valid exception:
  user-facing CLI and configuration options are part of the accepted contract,
  so a correction to them is `"valid"`, not `"off-level"`.
- `02_architecture.md`: components, responsibilities, interfaces (including
  their method signatures and field types), data flow, design decisions. Valid
  exception: public interface definitions are architecture content.
- `03_implementation_plan.md`: phases, concrete tasks, files to modify, and the
  verification approach at planning altitude.

A comment is `"off-level"` when it asks a document to carry detail below its
level — for `01_requirements.md`: which library API, a flag passed to an
implementation dependency (e.g. a `readelf` option), or pre-check to use, how
malformed input is detected, internal data structures or file layout,
temporary-file or retry mechanics, standard-library pitfalls, how a test is
constructed; for `02_architecture.md`: internal or helper function signatures,
specific call sequences, line-level code, test code structure; for
`03_implementation_plan.md`: exact assertions, full shell pipelines,
line-level code, test code structure — even when the comment is technically
correct. Use `"off-level"` also when the commented passage already
over-specifies such detail.

**R2 — Handoff routing.** Route by the phase the concern belongs to, not by
the commented document:

| Concern level | Destination | Consumer |
|---|---|---|
| design (raised on `01_requirements.md`) | `design_handoff.md` | `/mkarch` |
| architecture (raised on `01_requirements.md`) | `design_handoff.md` | `/mkarch` |
| implementation (raised on `01_requirements.md` or `02_architecture.md`) | `implementation_handoff.md` | `/mkplan` |
| plan-level (raised on `03_implementation_plan.md`) | none — retain the obligation in the plan at planning altitude | — |

**R3 — Handoff lifecycle.** Any change to a handoff item — add, extend,
correct, replace, or delete — is a decision change. Set the consumer that
already consumed it back to `draft`, and any later approved document after it,
even when no process-document text was edited:

- a `design_handoff.md` item reopens `02_architecture.md`, and then
  `03_implementation_plan.md`;
- an `implementation_handoff.md` item reopens `03_implementation_plan.md`.

Say so in the reply.

**R4 — Off-level reply.** The reply names where the concern was recorded: the
handoff item (`design_handoff.md` H-NN or `implementation_handoff.md` I-NN) or,
for a plan-level concern, the plan section or task where the obligation was
retained, plus the abstraction-level change made to the commented passage.

**R5 — Handoff item format.** Write in the project's document language. If the
file does not exist, create it with a short header stating its role: it
collects concerns raised in review that belong to a later phase, and that
phase's document (architecture document or implementation plan) records, for
each item, the approach taken or why it does not apply. Add the concern as the
next numbered item (`H-NN` in `design_handoff.md`, `I-NN` in
`implementation_handoff.md`) with: what the concern is, why it matters, a
candidate approach, and the related F-/AC- IDs. If an existing item already
covers it, extend that item instead of adding a duplicate. If the commented
document does not yet link to the handoff document, add one sentence that does.

## Phase 1 — Fetch (model: haiku)

Agent prompt:

> Run `gh pr view $ARGUMENTS --json number,url,headRefName` (omit `$ARGUMENTS`
> to use the current branch's PR if none was given).
> If no PR exists, return `{"found": false}`.
>
> Otherwise:
> 1. Extract owner and repo from the `url` field (format
>    `https://github.com/OWNER/REPO/pull/N`).
> 2. Fetch unresolved review threads:
>
> ```
> gh api graphql -F owner=OWNER -F repo=REPO -F number=NUMBER -f query='
>   query($owner:String!, $repo:String!, $number:Int!) {
>     repository(owner:$owner, name:$repo) {
>       pullRequest(number:$number) {
>         reviewThreads(first:100) {
>           pageInfo { hasNextPage }
>           nodes {
>             id
>             isResolved
>             comments(first:10) {
>               nodes { id databaseId body path line url author { login } }
>             }
>           }
>         }
>       }
>     }
>   }
> '
> ```
>
> 3. Filter to nodes where `isResolved=false`.
> 4. For each thread, use the FIRST comment's `databaseId`, `path`, `line`, `url`.
>    Concatenate the fetched comments (capped at `first:10` per thread) into a
>    `body` field, prefixing each with `@author: `. On busy threads there may be
>    more than 10 comments; `body` will only contain the first 10 fetched.
>    The `threadId` is the thread node's `id` (not the comment id).
> 5. Set `capHit=true` if `pageInfo.hasNextPage` is true (more threads exist
>    beyond the 100-node cap); otherwise `false`.
> 6. Reply with only this JSON: `{"found": true, "owner": "...", "repo": "...",
>    "number": N, "capHit": bool, "threads": [{"threadId": "...", "databaseId":
>    N, "body": "...", "path": "...", "line": N, "url": "..."}]}`

If `found=false`: report "No PR found" and stop — do not run further phases.
If `threads` is empty: report "No unresolved review threads — nothing to do"
and stop.
If `capHit=true`: note in your final report that the 100-thread fetch cap was
hit, so threads beyond it are invisible to this run; a re-run after resolving
this batch will pick up the remainder.

## Phase 2 — Triage (model: default)

Agent prompt (inline the fetched `threads` JSON from Phase 1):

> Triage these unresolved PR review threads for `OWNER/REPO#NUMBER`.
>
> Project conventions: read `CLAUDE.md` for the design principles, Go idioms, and
> testing rules this codebase holds itself to, and judge each comment against
> those. Beyond it: source comments are English only, one line where possible, and
> explain WHY rather than WHAT. Build checks are `make fmt && make test && make lint`.
>
> For EACH thread: read the source file at `path:line` for context, then
> classify two independent dimensions.
>
> `verdict` — is the suggestion right for this codebase?
> - `"valid"` — fix clearly improves correctness, clarity, or convention alignment
> - `"off-level"` — the thread is on a process document and asks it to carry
>   detail that belongs to a later phase (see "Abstraction-level check" below)
> - `"invalid"` — suggestion is wrong or inapplicable in this context
> - `"unclear"` — genuinely uncertain
>
> **Abstraction-level check — do this before choosing `valid`.** Process
> documents live under the task root (`docs/tasks/NNNN_<name>/`, see
> `.claude/commands/_context.md`). For a thread on one of them, apply the
> shared rules R1 and R2 (inlined below) to decide whether the comment asks
> the document to carry detail below its own level; if so, the verdict is
> `"off-level"`, **not** `"valid"`, even when the comment is technically
> correct — the fix is to raise the document's abstraction level (Phase 3).
> The underlying concern still counts for `severity`; an off-level comment
> can be `must-fix` if the concern is a real defect.
>
> <inline the full "Shared rules" section (R1–R5) verbatim here>
>
> For `"off-level"` threads, also set `behaviorGap`: `true` if the comment
> reveals an observable behavior the document does not yet require at its own
> level (e.g. "a timeout must still end the run when a descendant process
> keeps the pipe open" behind a comment asking for `exec.Cmd.WaitDelay`);
> `false` if it is purely about the means. Omit `behaviorGap` (or set
> `false`) for other verdicts.
>
> `severity` — how much does the raised issue actually matter? (judge the
> underlying concern on its merits, not just whether you will act on it)
> - `"must-fix"` — a real bug, or a correctness/security defect
> - `"worth-fixing"` — a legitimate improvement (clarity, convention, robustness) but not a bug
> - `"no-harm"` — cosmetic only, OR the comment is invalid/inapplicable: safe to ignore
>
> Calibration: rate by the WORST outcome when the affected code path executes,
> not by how often it executes. A defect that breaks shell/code syntax or makes
> a command fail (e.g. an indented heredoc terminator, a malformed quote) is
> `must-fix` even if its branch is rarely taken — rarity lowers likelihood, not
> the kind of defect. Do not downgrade a real bug to `worth-fixing` just
> because it sits in a guarded or seldom-run branch.
>
> `topic` — one concise English phrase naming what the comment raised (e.g.
> "heredoc terminator indentation", "stale Go version in prompt"). Used in the
> post-run summary so the user sees what each comment was about at a glance.
>
> Set `replyBody` to a concise English sentence:
> - valid: describe the fix (used as the PR reply after applying)
> - off-level: leave as empty string (Phase 3 writes it after applying)
> - invalid: explain why the suggestion does not apply
> - unclear: leave as empty string
>
> Also find CLUSTERS: groups of 3+ valid threads sharing a root cause where a
> single structural change resolves all of them. Describe the structural change.
>
> Threads to triage: `<inline JSON array from Phase 1>`
>
> Reply with only this JSON: `{"threads": [{"threadId": "...", "verdict":
> "valid|off-level|invalid|unclear", "severity":
> "must-fix|worth-fixing|no-harm", "topic": "...", "behaviorGap": bool,
> "replyBody": "..."}], "clusters": [{"threadIds": ["..."],
> "structuralChange": "..."}]}`

Split the returned `threads` into `valid`, `off-level`, `invalid`, `unclear`
by `verdict`. Report the counts and cluster count before continuing.

## Phase 3 — Fix (model: default)

Skip this phase entirely if both `valid` and `off-level` are empty (go
straight to Phase 4 with no fixes applied).

Agent prompt (inline `clusters` from Phase 2, the `valid` threads, and the
`off-level` threads with their `behaviorGap`, `path`, `line`, and comment
`body` from Phase 1):

> Apply ALL of the following fixes to the repository files.
>
> Off-level handling, handoff routing, handoff lifecycle, and the reply
> requirements follow the shared rules R1–R5, inlined here verbatim:
>
> <inline the full "Shared rules" section (R1–R5) verbatim here>
>
> Steps:
> 1. Apply cluster (structural) fixes first — they may subsume per-thread fixes.
> 2. Then apply any remaining per-thread fixes not covered by a cluster fix.
>    Generalize before patching: when a finding instances a rule already
>    stated elsewhere in the commented document, or already refined in a
>    previous round on this PR, change that rule once at its normative
>    location and make the other occurrences reference it rather than patching
>    each occurrence. A finding that is a mirror case of a rule just fixed is a
>    signal to generalize, not to add another special case.
> 3. Handle each off-level thread by raising the commented document's
>    abstraction level — never by adding the requested detail to it:
>    a. Do NOT add the later-phase detail the comment asks for to the
>       commented document.
>    b. If the commented passage itself states later-phase detail, rewrite it
>       at the document's own level: observable behavior for the requirements
>       document (`01_requirements.md`); components, responsibilities,
>       interfaces, and design decisions for the architecture document
>       (`02_architecture.md`); phases, concrete tasks, files to modify, and
>       the verification approach at planning altitude for the implementation
>       plan document (`03_implementation_plan.md`). Never renumber an
>       acceptance-criterion ID; reword a criterion that still carries an
>       observable obligation, and delete one that states only later-phase
>       mechanics, leaving its ID unused (as the requirements process guide
>       allows).
>    c. If `behaviorGap` is true, state the missing behavior at the document's
>       level (for the requirements document, an observable condition in the
>       relevant F-/AC- item, not a mechanism).
>    d. Record the concern in the handoff document named by R2, using the item
>       format and dedup rule in R5. A concern raised on the implementation
>       plan document has no later consumer: R2 sends it to no handoff, so keep
>       its obligation at planning altitude in the plan itself — state what
>       must be verified, not the exact assertion or shell pipeline — and do
>       NOT park it in `implementation_handoff.md`.
> 4. Apply R3 to the Document Status of every process document you edited and
>    every handoff item you added or changed, per "Editing an approved
>    document" in the requirements process guide
>    (`docs/dev/developer_guide/requirements_process.md`): a decision change
>    (including any handoff item change) resets the affected approved document,
>    and any later approved document after it, to `draft`; an editorial
>    correction keeps the status and records the change in `Comments`, stating
>    that no decision changed. Say in the reply which documents need
>    re-approval.
> 5. Do NOT run build checks — that happens in the next phase.
> 6. For each thread, return `threadId`, `applied` (true/false), `replyBody`
>    (one English sentence describing exactly what was changed, for the PR
>    reply). If a cluster fix subsumed a thread, set `applied=true` and
>    reference the structural change. For an off-level thread, the reply
>    follows R4 (which later phase the detail belongs to, and where the
>    concern was recorded or retained).
>
> Clusters (structural changes): `<inline JSON>`
>
> Per-thread fixes (valid threads): `<inline JSON>`
>
> Off-level threads (raise abstraction, record in handoff): `<inline JSON>`
>
> Reply with only this JSON: `{"fixes": [{"threadId": "...", "applied": bool,
> "replyBody": "..."}]}`

## Phase 4 — Build (model: haiku)

Skip this phase (treat as `{success: true, commitSha: ""}`) if both `valid`
and `off-level` were empty in Phase 2.

Agent prompt:

> Run the build checks, then commit if they pass.
>
> 1. `make fmt && make test && make lint`
> 2. If all pass:
>    ```
>    git add -A
>    git diff --cached --quiet && COMMITTED=0 || COMMITTED=1
>    if [ "$COMMITTED" = "1" ]; then git commit -m "fix: address PR #NUMBER review comments" || { COMMITTED=0; false; }; fi
>    ```
> 3. Only if a commit was actually created (`COMMITTED=1`), get the commit SHA
>    via `git log -1 --format=%H`. Otherwise use empty string.
> 4. Reply with only this JSON: `{"success": true, "commitSha": "...", "error":
>    ""}`. If any check fails, `{"success": false, "commitSha": "", "error":
>    "..."}` with the failing output in `error`. Do NOT commit on failure.

If `success=false`: report the build failure and its error output, and stop —
do not run Phase 5 or 6.

## Phase 5 — Reply + resolve (model: haiku)

Build the candidate list yourself (not via an agent) before invoking this phase:

- Take every thread where `verdict="invalid"`, plus every thread where
  `verdict` is `"valid"` or `"off-level"` AND `applied=true` in the Phase 3
  fixes.
- For each, resolve `replyBody`: use the Phase 3 fix's `replyBody` if present
  (for applied valid and off-level threads), else the Phase 2 triage
  `replyBody`.
- Look up each thread's `databaseId` and `url` from the Phase 1 fetch data.
- Threads with no `databaseId` cannot be replied to automatically — set them
  aside for the "skipped" list in your final report; do not send them to the
  agent.
- Threads with an empty `replyBody` — also set aside for "skipped"; do not
  send them to the agent.
- Only threads with both a `databaseId` and a non-empty `replyBody` are
  "actionable". If there are none, skip this phase entirely.

For actionable threads, build one shell block per thread:

```
# Thread <threadId> (comment <databaseId>)
gh api repos/<owner>/<repo>/pulls/<number>/comments/<databaseId>/replies -F body=@- <<'REPLYBODY_EOF' && gh api graphql -F threadId=<threadId> -f query='mutation($threadId:ID!){resolveReviewThread(input:{threadId:$threadId}){thread{id isResolved}}}'
<replyBody>
REPLYBODY_EOF
```

Agent prompt:

> Post replies and resolve threads for PR #NUMBER by running the following
> commands sequentially (not in parallel — avoids RPM/TPM rate limits). Each
> block posts a reply (`-F body=@-`, piped from a quoted heredoc,
> `<<'REPLYBODY_EOF'`) then, only if that succeeds (`&&`), resolves the
> thread. The heredoc must stay on the same logical command line as the
> `&&`-chained resolve command, with the body text and closing
> `REPLYBODY_EOF` delimiter following immediately after — do not restructure
> this into separate commands or reorder the lines. This piping avoids
> interpolating `replyBody` — LLM-generated text that may contain double
> quotes, backticks, or `$(...)` — into a shell string. Do not switch to
> `-f body="..."` inline quoting, `-f body=@-` (lowercase `-f` does NOT
> support the `@-` stdin-read syntax — only the uppercase `-F` typed-field
> flag does; using `-f` silently posts the literal string `@-` as the
> comment body), or hand-build a JSON literal.
>
> IMPORTANT: the reply endpoint requires the `/pulls/<number>/` segment
> (`POST /repos/{owner}/{repo}/pulls/{pull_number}/comments/{comment_id}/replies`).
> A path without it (`.../pulls/comments/...`) is a different resource and
> 404s for replies — do not drop it.
>
> `<inline the shell blocks built above>`

## Phase 6 — Wrap: PR description + push (model: haiku)

Agent prompt:

> Verify the PR description is still accurate and push.
>
> 1. `gh pr view NUMBER --json title,body`
> 2. `git log --oneline -10`
> 3. Only if the description is significantly stale (approach changed, scope
>    shifted) AND you have drafted a concrete final title and body from the
>    actual PR content, run:
>    ```
>    gh pr edit NUMBER --title "<real title>" --body-file - <<'EOF'
>    <real body>
>    EOF
>    ```
>    Never pass placeholder text — if you have not drafted concrete
>    replacement text, skip the edit entirely so the real PR description is
>    preserved.
> 4. `git push`

---

## Final report

After all phases complete (or a phase aborted early), report the result to
the user with **both** of the following — bare counts alone are not enough:

1. **Summary line**: PR, fixed / off-level (deferred to a handoff document) /
   invalid / unclear counts, clusters, and commit SHA (or "no commit" / "build
   failed" / "no PR found" / "nothing to do" if a phase stopped early).

2. **Bot-comment assessment**: a table built from the Phase 2 triage results
   (`topic`, `severity`, `verdict`) plus the Phase 3 `applied` flag, grouped by
   `severity`, so the user sees what each comment raised and how much it
   mattered — not just that N were "fixed". Use these levels:
   - 🔴 **must-fix** — real bug / correctness or security defect
   - 🟡 **worth-fixing** — legitimate improvement, not a bug
   - 🟢 **no-harm** — cosmetic, or invalid/inapplicable: safe to ignore

   For each thread show its `topic`, its `verdict`, whether a fix was
   `applied`, for off-level threads where the concern was recorded (the
   handoff item, or the plan section or task for a plan-level concern), and
   (for unresolved/unclear ones) the `url`. Then give a one-line overall read:
   was this round substantive or recurring churn, is the severity floor
   reached (Convergence policy), and is it safe to merge rather than run
   again. If most threads were off-level, say so: the document is converging
   and the remaining concerns now wait in the handoff document. Name every
   process document Phase 3 returned from `approved` to `draft` (edited
   documents and the later-phase documents reset with them), so the user knows
   it needs re-approval before the next phase proceeds.

3. **Skipped threads**: list every thread left out of Phase 5 (unclear
   verdict, valid- or off-level-but-unapplied, missing `databaseId`, or empty
   `replyBody`) with its URL so the user can resolve it manually.
