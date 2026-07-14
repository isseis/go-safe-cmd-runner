> **Project context (read first)**: Read `.cline/commands/_context.md`. It is the
> single source of truth for project-specific values — build checks (`make fmt`/`make test`/
> `make lint`/`make deadcode`), language (Go), source layout, and test-helper conventions.
> Where this command names a build check, treat the entry in `_context.md` as canonical.
> The review step follows the shared procedure in
> `.cline/commands/_lib/review-self-pattern.md` (no subagent tool is used — the review
> runs inline in the current conversation).

## Goal

Resolve all unresolved review threads on the PR associated with the current branch.

Work through the phases in order. After each phase, report what happened and continue.

---

## Phase 1: Fetch

Get the PR number and unresolved review threads.

1. Run `gh pr view --json number,url,headRefName` for the current branch.
   - If no PR exists, stop and report "No PR found for current branch."
   - Otherwise, extract `owner`, `repo`, and `number` from the URL (format: `https://github.com/OWNER/REPO/pull/N`).

2. Fetch unresolved review threads via the GitHub GraphQL API:

```bash
gh api graphql -F owner=OWNER -F repo=REPO -F number=NUMBER -f query='
  query($owner:String!, $repo:String!, $number:Int!) {
    repository(owner:$owner, name:$repo) {
      pullRequest(number:$number) {
        reviewThreads(first:100) {
          pageInfo { hasNextPage }
          nodes {
            id
            isResolved
            comments(first:10) {
              nodes { id databaseId body path line url author { login } }
            }
          }
        }
      }
    }
  }
'
```

3. Filter to nodes where `isResolved=false`.
4. For each unresolved thread:
   - Use the node `id` as the `threadId`.
   - Use the FIRST comment's `databaseId`, `path`, `line`, `url`.
   - Concatenate the fetched comments (up to 10 per thread) into a `body` field, prefixing each with `@author: `.
5. Set `capHit=true` if `pageInfo.hasNextPage` is true (more threads exist beyond the 100-node cap); otherwise `capHit=false`.
6. If there are no unresolved threads, stop and report "No unresolved review threads — nothing to do."
7. If `capHit` is true, warn: "WARNING: hit the 100-thread fetch cap — there may be more unresolved threads; re-run after this batch."

Report: PR number, owner/repo, and count of unresolved threads.

---

## Phase 2: Triage

For each unresolved thread, classify it along two independent dimensions.

### Verdict
- **valid** — the fix clearly improves correctness, clarity, or convention alignment for this codebase.
- **invalid** — the suggestion is wrong or inapplicable in this context.
- **unclear** — genuinely uncertain.

### Severity (judge the underlying concern on its merits, not just whether you will act on it)
- 🔴 **must-fix** — a real bug, or a correctness/security defect.
  - Calibration: rate by the WORST outcome when the affected code path executes, not by how often it executes. A defect that breaks shell/code syntax or makes a command fail is "must-fix" even if its branch is rarely taken.
- 🟡 **worth-fixing** — a legitimate improvement (clarity, convention, robustness) but not a bug.
- 🟢 **no-harm** — cosmetic only, OR the comment is invalid/inapplicable: safe to ignore.

### Topic
One concise English phrase naming what the comment raised (e.g. "heredoc terminator indentation", "stale Go version in prompt").

### Reply body
- **valid**: describe the fix (used as the PR reply after applying).
- **invalid**: explain why the suggestion does not apply.
- **unclear**: leave as empty string.

### Clusters
Find groups of 3+ valid threads sharing a root cause where a single structural change resolves all of them. Describe the structural change.

For each thread, read the source file at `path:line` for context before classifying.

Report: counts of valid / invalid / unclear threads, and number of clusters found.

---

## Phase 3: Fix

Apply all code fixes to the repository files.

1. Apply cluster (structural) fixes first — they may subsume per-thread fixes.
2. Then apply any remaining per-thread fixes not covered by a cluster fix.
3. Do NOT run build checks — that happens in the next phase.
4. For each thread, record: `threadId`, `applied` (true/false), and `replyBody` (one English sentence describing exactly what was changed, for the PR reply). If a cluster fix subsumed a thread, set `applied=true` and reference the structural change.

Report: how many fixes were applied, how many were skipped.

---

## Phase 4: Build + Commit

Run the build checks, then commit if they pass.

1. Run the build checks defined in `_context.md` (e.g. `make fmt && make test && make lint`).
2. If all pass:
   ```bash
   git add -A
   git diff --cached --quiet && COMMITTED=0 || COMMITTED=1
   if [ "$COMMITTED" = "1" ]; then
     git commit -m "fix: address PR #N review comments"
   fi
   ```
3. If a commit was created, get the SHA: `git log -1 --format=%H`.
4. If any check fails, stop and report the failure. Do NOT commit on failure.

Report: build success/failure, commit SHA (if any).

---

## Phase 5: Reply + Resolve

Post replies and resolve threads sequentially.

For each thread that is either:
- **valid** and a fix was applied, OR
- **invalid**

Post a reply and resolve the thread using the following commands sequentially (one block per thread):

```bash
# Thread <threadId> (comment <databaseId>)
gh api repos/<owner>/<repo>/pulls/<number>/comments/<databaseId>/replies \
  -f body="<replyBody>" \
  && gh api graphql -F threadId=<threadId> \
  -f query='mutation($threadId:ID!){resolveReviewThread(input:{threadId:$threadId}){thread{id isResolved}}}'
```

**Important**: The heredoc terminator `PAYLOAD` must be at column 0 (no leading whitespace) for the shell to recognise it.

For threads that are **unclear** or where a fix was not applied: do not reply automatically. Collect their URLs for the final report so the user can resolve them manually.

Report: how many threads were replied to and resolved, how many require manual attention.

---

## Phase 6: Wrap — PR description check + Push

1. Verify the PR description is still accurate: `gh pr view <number> --json title,body`
2. Check recent commits: `git log --oneline -10`
3. Only if the description is significantly stale (approach changed, scope shifted), AND you have drafted a concrete final title and body from the actual PR content: run `gh pr edit` with those real values. If you have not drafted concrete replacement text, SKIP the edit entirely.
4. Push: `git push`

---

## Report

After all phases complete, report the result with **both** of the following:

### 1. Summary line
Format: `PR <owner>/<repo>#<number>: <fixed> fixed, <invalid> invalid, <unclear> unclear, <clusters> clusters, commit <SHA>`

### 2. Bot-comment assessment table
Group every thread by `severity` using these levels:
- 🔴 **must-fix** — real bug / correctness or security defect
- 🟡 **worth-fixing** — legitimate improvement, not a bug
- 🟢 **no-harm** — cosmetic, or invalid/inapplicable: safe to ignore

For each thread show:
- `topic` — what the comment raised
- `applied` — whether a fix was applied (yes/no)
- `url` — link to the thread (for unresolved/unclear ones)

Then give a one-line overall read: was this round substantive or noise, and is it worth running again or safe to merge.

### 3. Skipped threads
List any "unclear" or unapplied threads with their URLs so the user can resolve them manually.
