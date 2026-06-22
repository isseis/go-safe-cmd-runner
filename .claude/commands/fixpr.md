> **Project context**: this command refers to "the build checks" (e.g. `make lint`,
> `make test`). Their exact values are defined in `.claude/commands/_context.md`
> under "Tech-stack convention". Read that file and use its values; the rest of
> this command is project-independent.

## Preparation

Check the PR for the current branch.

```
gh pr view --json number,url,headRefName
```

If no PR exists, stop. Note the owner, repo, and PR number for use in subsequent steps.

## Fetch Unresolved Comments

Use GraphQL to get a list of unresolved review threads.

```
gh api graphql -F owner=OWNER -F repo=REPO -F number=NUMBER -f query='
  query($owner:String!, $repo:String!, $number:Int!) {
    repository(owner:$owner, name:$repo) {
      pullRequest(number:$number) {
        reviewThreads(first:100) {
          nodes {
            id
            isResolved
            comments(first:10) {
              nodes {
                id
                databaseId
                body
                path
                line
                author { login }
                url
              }
            }
          }
        }
      }
    }
  }
'
```

Only process threads where `isResolved: false`. If there are none, stop.

This query caps at 100 threads (`reviewThreads(first:100)`) and 10 comments per thread (`comments(first:10)`). If a run hits either cap — 100 threads returned, or a thread already showing 10 comments — the query as written cannot reach the rest: add `pageInfo { hasNextPage endCursor }` to the capped connection, add an `$after: String` variable, and re-issue with `after: $after`, looping while `hasNextPage` is true, until the set is complete (the root-cause pass below assumes the full set). Alternatively narrow the query (e.g. by `path`). On a small PR these caps are not reached and no pagination is needed.

## Synthesize Across Threads (root-cause pass)

Before fixing threads one by one, look at them **together**. Per-comment spot-fixes miss the case where several comments are symptoms of one root cause or a missing general principle — fixing each in isolation produces churn and leaves the cause in place. This pass runs once, up front, and feeds the per-thread pass below.

1. **Read the full set first.** Load every unresolved thread (above) and do a quick validity triage of each (Valid / Invalid / Unclear, per the criteria in Step 1 under "Address Each Unresolved Thread") so you know which are real before acting.

2. **Cluster.** Group the Valid (and Unclear) threads by shared root cause — same file or surface, same kind of defect, or the same invariant being violated.

3. **Gate — only synthesize when the signal is real.** Act on a cluster only when one of these holds; otherwise skip straight to per-thread handling:
   - three or more related Valid threads, or
   - a cluster concentrated on one file / surface / subsystem, or
   - the same area recurring across review rounds you have already handled this session (from your run history), or sustained back-and-forth within a thread.

   A lone comment, or unrelated comments, get per-thread handling. Do **not** invent a root cause to justify a larger change. Base every gate signal only on data you actually have — the fetched threads and their comments (paginated to completeness above), plus your session memory of prior rounds — not on metadata the query does not return (timestamps, review IDs).

4. **Find the root cause / missing principle.** For each qualifying cluster, ask: would a single structural change — and/or recording a general principle (an invariant, a shared helper, a completeness rule, a data table that becomes the single source of truth) — resolve the whole cluster **and prevent recurrence**? The goal is to fix the cause, not each symptom.

5. **Apply once, if in scope.** If yes and the change is within the PR's scope:
   - Make the structural change a single, self-contained change. Run the build checks (defined in `.claude/commands/_context.md`, Tech-stack convention) to confirm no errors, then commit it **once** — not one commit per symptom.
   - Optionally record the principle/invariant in the artifact itself (a code comment, a design or plan document), **including how future same-class comments should be handled**, so the cause does not recur and the next review can be triaged against it.

6. **Guardrails (all required).**
   - **Subsumption check:** confirm each clustered comment is concretely addressed by the structural change. Drop nothing — a comment the structural change does not actually cover still gets per-thread handling.
   - **No manufactured causes:** the existence of several comments is not itself a root cause. Adopt a structural fix only when it genuinely subsumes the members; otherwise fix them individually.
   - **Do not dismiss real defects:** when you record a principle to handle a class of comments uniformly (e.g. "deferred to implementation"), it applies only to items the principle truly covers. A comment that reveals a *new* structural gap (a wrong invariant, a missing branch, a fail-open) is a defect, not noise — fix it, do not wave it through.
   - **Scope/size gate:** if the structural change is large, architecturally significant, or otherwise beyond a routine review fix, do **not** apply it unilaterally — present it to the user (problem, options with pros/cons, recommendation) and let them decide before applying.

7. **Hand off to the per-thread pass.** A thread resolved by a structural change is closed below by replying with a link to that change/commit and resolving it — not by a duplicate spot-fix. Every remaining thread is handled individually.

## Address Each Unresolved Thread

Process each thread in order as follows.

### Step 1: Assess validity of the comment

Before making any change, evaluate whether the suggested fix is actually correct and beneficial for this codebase. Consider:

- Does the fix align with the project's design principles, conventions, and goals?
- Could the suggestion be based on a misunderstanding of the context (e.g., applying general style rules to a domain-specific file like an AI prompt)?
- Does it improve correctness, clarity, or maintainability — or is it a stylistic preference that doesn't apply here?

Based on this assessment, classify the thread as one of:
- **Valid**: The fix is clearly correct and beneficial → follow [When the comment is valid and the fix is clear].
- **Invalid**: The fix is incorrect or inappropriate for this context → follow [When the comment is invalid].
- **Unclear**: You are uncertain whether the fix is appropriate → follow [When the fix is unclear].

### When the comment is valid and the fix is clear

If the root-cause pass already addressed this thread with a structural change, reply pointing to that change (the commit or the recorded principle/section) and resolve the thread — skip the per-comment fix below to avoid duplicating it. Otherwise:

1. Fix the code as indicated by the comment.
2. Run the build checks (defined in `.claude/commands/_context.md`, Tech-stack convention) to confirm no errors.
3. Commit.
4. Reply to the PR comment thread with a description of the fix (in English).

   ```
   gh api repos/OWNER/REPO/pulls/NUMBER/comments/DATABASE_ID/replies \
     -X POST -f body="Description of the fix in English"
   ```

5. Resolve the thread.

   ```
   gh api graphql -F threadId=THREAD_ID -f query='
     mutation($threadId:ID!) {
       resolveReviewThread(input:{threadId:$threadId}) {
         thread { id isResolved }
       }
     }
   '
   ```

### When the comment is invalid

1. Reply to the PR comment thread explaining why the suggestion does not apply (in English).

   ```
   gh api repos/OWNER/REPO/pulls/NUMBER/comments/DATABASE_ID/replies \
     -X POST -f body="Explanation of why the suggestion is not applicable"
   ```

2. Resolve the thread.

   ```
   gh api graphql -F threadId=THREAD_ID -f query='
     mutation($threadId:ID!) {
       resolveReviewThread(input:{threadId:$threadId}) {
         thread { id isResolved }
       }
     }
   '
   ```

### When the fix is unclear

Skip and move to the next thread (revisit in a later step).

## Check PR Description Accuracy

Before pushing, verify that the PR title and body still accurately describe the current state of the changes. A PR description becomes stale when the approach changes significantly during review (e.g., a TLS strategy is revised, a scope item is added or removed, a file list changes). Stale descriptions cause reviewers to flag inconsistencies that are not real bugs.

If the description is stale, update it:

```
gh pr edit NUMBER --body "$(cat <<'EOF'
...updated body...
EOF
)"
```

## Push

Once the PR description is accurate and all clear comments have been addressed, run `git push`.

## Revisit Skipped Threads

For each skipped thread, present the following:

- **Problem summary**: Briefly describe the issue raised by the comment.
- **Proposed approaches**: List multiple possible options with pros and cons for each.
- **Recommendation**: If possible, recommend one option and explain why.

## Summarize Root Causes

If the root-cause pass produced any structural change, recorded principle, or cluster, report it to the user so the systemic fix is visible (not buried among per-thread replies):

- **Clusters found**: the groups of comments and the shared root cause / missing principle behind each.
- **Structural changes applied**: what was changed once instead of N spot-fixes, and which threads each subsumes.
- **Principles recorded**: any invariant / rule written into the artifact, and how future same-class comments will be triaged against it.
- **Surfaced for decision**: any structural change held back by the scope/size gate and awaiting the user's call.

If the root-cause pass found nothing (all comments were independent), state that briefly and skip this section.
