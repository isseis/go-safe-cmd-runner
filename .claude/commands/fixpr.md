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
> Project conventions:
> - Go 1.26 codebase (per go.mod), security-focused, interface-driven design
> - YAGNI/DRY: no premature abstractions
> - Modern Go idioms: slices/maps packages, errors.Is/AsType, `any` instead of
>   `interface{}`, etc.
> - Comments: English only; one-line max; explain WHY, not WHAT
> - Build checks: `make fmt && make test && make lint`
>
> For EACH thread: read the source file at `path:line` for context, then
> classify two independent dimensions.
>
> `verdict` — is the suggestion right for this codebase?
> - `"valid"` — fix clearly improves correctness, clarity, or convention alignment
> - `"invalid"` — suggestion is wrong or inapplicable in this context
> - `"unclear"` — genuinely uncertain
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
> - invalid: explain why the suggestion does not apply
> - unclear: leave as empty string
>
> Also find CLUSTERS: groups of 3+ valid threads sharing a root cause where a
> single structural change resolves all of them. Describe the structural change.
>
> Threads to triage: `<inline JSON array from Phase 1>`
>
> Reply with only this JSON: `{"threads": [{"threadId": "...", "verdict":
> "valid|invalid|unclear", "severity": "must-fix|worth-fixing|no-harm", "topic":
> "...", "replyBody": "..."}], "clusters": [{"threadIds": ["..."],
> "structuralChange": "..."}]}`

Split the returned `threads` into `valid`, `invalid`, `unclear` by `verdict`.
Report the counts and cluster count before continuing.

## Phase 3 — Fix (model: default)

Skip this phase entirely if `valid` is empty (go straight to Phase 4 with no
fixes applied).

Agent prompt (inline `clusters` from Phase 2 and the `valid` threads):

> Apply ALL of the following code fixes to the repository files.
>
> Steps:
> 1. Apply cluster (structural) fixes first — they may subsume per-thread fixes.
> 2. Then apply any remaining per-thread fixes not covered by a cluster fix.
> 3. Do NOT run build checks — that happens in the next phase.
> 4. For each thread, return `threadId`, `applied` (true/false), `replyBody`
>    (one English sentence describing exactly what was changed, for the PR
>    reply). If a cluster fix subsumed a thread, set `applied=true` and
>    reference the structural change.
>
> Clusters (structural changes): `<inline JSON>`
>
> Per-thread fixes (valid threads): `<inline JSON>`
>
> Reply with only this JSON: `{"fixes": [{"threadId": "...", "applied": bool,
> "replyBody": "..."}]}`

## Phase 4 — Build (model: haiku)

Skip this phase (treat as `{success: true, commitSha: ""}`) if `valid` was
empty in Phase 2.

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
  `verdict="valid"` AND `applied=true` in the Phase 3 fixes.
- For each, resolve `replyBody`: use the Phase 3 fix's `replyBody` if present
  (for applied valid threads), else the Phase 2 triage `replyBody`.
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

1. **Summary line**: PR, fixed / invalid / unclear counts, clusters, and
   commit SHA (or "no commit" / "build failed" / "no PR found" / "nothing to
   do" if a phase stopped early).

2. **Bot-comment assessment**: a table built from the Phase 2 triage results
   (`topic`, `severity`, `verdict`) plus the Phase 3 `applied` flag, grouped by
   `severity`, so the user sees what each comment raised and how much it
   mattered — not just that N were "fixed". Use these levels:
   - 🔴 **must-fix** — real bug / correctness or security defect
   - 🟡 **worth-fixing** — legitimate improvement, not a bug
   - 🟢 **no-harm** — cosmetic, or invalid/inapplicable: safe to ignore

   For each thread show its `topic`, whether a fix was `applied`, and (for
   unresolved/unclear ones) the `url`. Then give a one-line overall read: was
   this round substantive or noise, and is it worth running again or safe to
   merge.

3. **Skipped threads**: list every thread left out of Phase 5 (unclear
   verdict, valid-but-unapplied, missing `databaseId`, or empty `replyBody`)
   with its URL so the user can resolve it manually.
