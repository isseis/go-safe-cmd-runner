export const meta = {
  name: 'fixpr',
  description: 'Fix unresolved PR review threads — model-tiered agents to reduce cost',
  phases: [
    { title: 'Fetch',  detail: 'Get PR info and unresolved review threads' },
    { title: 'Triage', detail: 'Classify threads and identify root-cause clusters' },
    { title: 'Fix',    detail: 'Apply all code fixes in one pass' },
    { title: 'Build',  detail: 'Single build check + commit' },
    { title: 'Reply',  detail: 'Post replies and resolve threads in parallel' },
    { title: 'Wrap',   detail: 'PR description check and push' },
  ],
}

// Build checks for this Go project. Source of truth is the "Build checks" row in
// .claude/commands/_context.md (Tech-stack convention); a workflow script cannot
// read that markdown at runtime, so this mirrors it — keep the two in sync if the
// build commands change there.
const BUILD_CHECKS = 'make fmt && make test && make lint'

// ── Schemas ───────────────────────────────────────────────────────────────────

const PR_SCHEMA = {
  type: 'object',
  required: ['found', 'owner', 'repo', 'number', 'threads'],
  properties: {
    found:  { type: 'boolean' },
    owner:  { type: 'string' },
    repo:   { type: 'string' },
    number: { type: 'integer' },
    capHit: { type: 'boolean' },
    threads: {
      type: 'array',
      items: {
        type: 'object',
        required: ['threadId', 'databaseId', 'body', 'path'],
        properties: {
          threadId:   { type: 'string' },
          databaseId: { type: 'integer' },
          body:       { type: 'string' },
          path:       { type: 'string' },
          line:       { type: 'integer' },
          url:        { type: 'string' },
        },
      },
    },
  },
}

const TRIAGE_SCHEMA = {
  type: 'object',
  required: ['threads', 'clusters'],
  properties: {
    threads: {
      type: 'array',
      items: {
        type: 'object',
        required: ['threadId', 'verdict', 'replyBody'],
        properties: {
          threadId:  { type: 'string' },
          verdict:   { type: 'string', enum: ['valid', 'invalid', 'unclear'] },
          replyBody: { type: 'string' },
        },
      },
    },
    clusters: {
      type: 'array',
      items: {
        type: 'object',
        required: ['threadIds', 'structuralChange'],
        properties: {
          threadIds:       { type: 'array', items: { type: 'string' } },
          structuralChange: { type: 'string' },
        },
      },
    },
  },
}

const FIX_SCHEMA = {
  type: 'object',
  required: ['fixes'],
  properties: {
    fixes: {
      type: 'array',
      items: {
        type: 'object',
        required: ['threadId', 'applied', 'replyBody'],
        properties: {
          threadId:  { type: 'string' },
          applied:   { type: 'boolean' },
          replyBody: { type: 'string' },
        },
      },
    },
  },
}

const BUILD_SCHEMA = {
  type: 'object',
  required: ['success'],
  properties: {
    success:   { type: 'boolean' },
    commitSha: { type: 'string' },
    error:     { type: 'string' },
  },
}

// ── Phase 1: Fetch (haiku — pure data retrieval) ───────────────────────────

phase('Fetch')
const pr = await agent(
  `Run \`gh pr view --json number,url,headRefName\` for the current branch.
If no PR exists, return found=false, owner="", repo="", number=0, threads=[].

Otherwise:
1. Extract owner and repo from the "url" field (format: https://github.com/OWNER/REPO/pull/N).
2. Fetch unresolved review threads:

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

3. Filter to nodes where isResolved=false.
4. For each thread, use the FIRST comment's databaseId, path, line, url.
   Concatenate ALL comments into the body field, prefixing each with "@author: ".
   The threadId is the node "id" from reviewThreads (not the comment id).
5. Set capHit=true if pageInfo.hasNextPage is true (more threads exist beyond the
   100-node cap); otherwise capHit=false.
6. Return found=true with all threads.`,
  { schema: PR_SCHEMA, model: 'haiku', effort: 'low', phase: 'Fetch' }
)

if (!pr || !pr.found) {
  log('No PR found for current branch — stopping.')
  return { status: 'no-pr' }
}
if (pr.threads.length === 0) {
  log('No unresolved review threads — nothing to do.')
  return { status: 'nothing-to-do' }
}

// The Fetch query caps reviewThreads at 100 (no pagination). On a PR that hits
// the cap, threads beyond 100 are silently invisible to this run — surface it so
// the gap is not mistaken for completeness. Re-running fixpr after resolving the
// first batch picks up the remainder.
if (pr.capHit) {
  log('WARNING: hit the 100-thread fetch cap — there may be more unresolved threads; re-run after this batch.')
}

log(`PR #${pr.number} (${pr.owner}/${pr.repo}) — ${pr.threads.length} unresolved threads`)

// ── Phase 2: Triage (sonnet — requires code reasoning) ────────────────────

phase('Triage')
const triage = await agent(
  `Triage these unresolved PR review threads for ${pr.owner}/${pr.repo}#${pr.number}.

Project conventions:
- Go 1.26 codebase (per go.mod), security-focused, interface-driven design
- YAGNI/DRY: no premature abstractions
- Modern Go idioms: slices/maps packages, errors.Is/As, any instead of interface{}, etc.
- Comments: English only; one-line max; explain WHY, not WHAT
- Build checks: ${BUILD_CHECKS}

For EACH thread: read the source file at path:line for context, then classify:
  "valid"   — fix clearly improves correctness, clarity, or convention alignment
  "invalid" — suggestion is wrong or inapplicable in this context
  "unclear" — genuinely uncertain

Set replyBody to a concise English sentence:
  valid:   describe the fix (used as the PR reply after applying)
  invalid: explain why the suggestion does not apply
  unclear: leave as empty string

Also find CLUSTERS: groups of 3+ valid threads sharing a root cause where a single
structural change resolves all of them. Describe the structural change.

Threads to triage:
${JSON.stringify(pr.threads, null, 2)}`,
  { schema: TRIAGE_SCHEMA, model: 'sonnet', phase: 'Triage' }
)

const valid   = triage.threads.filter(t => t.verdict === 'valid')
const invalid = triage.threads.filter(t => t.verdict === 'invalid')
const unclear = triage.threads.filter(t => t.verdict === 'unclear')

log(`Triage: valid=${valid.length} invalid=${invalid.length} unclear=${unclear.length} clusters=${triage.clusters.length}`)

// ── Phase 3: Fix (sonnet — code edits) ────────────────────────────────────

const fixes = []
if (valid.length > 0) {
  phase('Fix')
  const fixResult = await agent(
    `Apply ALL of the following code fixes to the repository files.

Steps:
1. Apply cluster (structural) fixes first — they may subsume per-thread fixes.
2. Then apply any remaining per-thread fixes not covered by a cluster fix.
3. Do NOT run build checks — that happens in the next step.
4. For each thread, return: threadId, applied=true/false, replyBody (one English
   sentence describing exactly what was changed, for the PR reply).
   If a cluster fix subsumed a thread, set applied=true and reference the structural change.

Clusters (structural changes):
${JSON.stringify(triage.clusters, null, 2)}

Per-thread fixes (valid threads):
${JSON.stringify(valid, null, 2)}`,
    { schema: FIX_SCHEMA, model: 'sonnet', phase: 'Fix' }
  )
  fixes.push(...fixResult.fixes)
}

// ── Phase 4: Build + commit (haiku — mechanical) ──────────────────────────

phase('Build')
const buildResult = valid.length > 0
  ? await agent(
      `Run the build checks, then commit if they pass.

1. ${BUILD_CHECKS}
2. If all pass:
   git add -A
   git diff --cached --quiet || git commit -m "fix: address PR #${pr.number} review comments"
3. Get the commit SHA: git log -1 --format=%H
4. Return success=true and commitSha.
If any check fails, return success=false and the error output in the error field. Do NOT commit on failure.`,
      { schema: BUILD_SCHEMA, model: 'haiku', effort: 'low', phase: 'Build' }
    )
  : { success: true, commitSha: '' }

if (!buildResult.success) {
  log(`Build failed — aborting. Error: ${buildResult.error || '(see above)'}`)
  return { status: 'build-failed', error: buildResult.error }
}

if (buildResult.commitSha) {
  log(`Build passed — commit ${buildResult.commitSha}`)
}

// ── Phase 5: Reply + resolve (haiku, parallel) ────────────────────────────

phase('Reply')

// Build a map of fix-phase reply bodies (override triage reply for valid threads)
const replyOverrides = {}
for (const f of fixes) {
  if (f.replyBody) replyOverrides[f.threadId] = f.replyBody
}

// Set of threadIds where fixes were actually applied
const appliedThreadIds = new Set(fixes.filter(f => f.applied).map(f => f.threadId))

// Look up original thread data (databaseId) by threadId
const threadIndex = {}
for (const t of pr.threads) threadIndex[t.threadId] = t

const actionable = triage.threads
  .filter(t => t.verdict === 'invalid' || (t.verdict === 'valid' && appliedThreadIds.has(t.threadId)))
  .map(t => ({
    threadId:   t.threadId,
    databaseId: (threadIndex[t.threadId] || {}).databaseId,
    replyBody:  replyOverrides[t.threadId] || t.replyBody || '',
    owner:      pr.owner,
    repo:       pr.repo,
    number:     pr.number,
  }))
  .filter(t => t.databaseId)
  .filter(t => t.replyBody)

if (actionable.length > 0) {
  // Build the list of reply+resolve commands to run sequentially in one agent
  // call, avoiding the per-thread parallel fan-out that risks RPM/TPM rate limits.
  const replyCommands = actionable.map(item => {
    const replyBodyPayload = JSON.stringify({ body: item.replyBody })
    return `# Thread ${item.threadId} (comment ${item.databaseId})
gh api repos/${item.owner}/${item.repo}/pulls/${item.number}/comments/${item.databaseId}/replies \\
  -X POST --input - <<'PAYLOAD'
${replyBodyPayload}
PAYLOAD
gh api graphql -F threadId=${item.threadId} \\
  -f query='mutation($threadId:ID!){resolveReviewThread(input:{threadId:$threadId}){thread{id isResolved}}}'`
  }).join('\n\n')

  await agent(
    `Post replies and resolve threads for PR #${pr.number} by running the following
commands sequentially. Each block posts a reply then resolves the thread.
The quoted 'PAYLOAD' heredoc passes the body verbatim — no shell expansion.
The heredoc terminator must be at column 0 for the shell to recognise it.

${replyCommands}`,
    { model: 'haiku', effort: 'low', phase: 'Reply' }
  )
}

// ── Phase 6: Wrap — PR description check + push (haiku) ───────────────────

phase('Wrap')
await agent(
  `Verify the PR description is still accurate and push.

1. gh pr view ${pr.number} --json title,body
2. git log --oneline -10
3. Only if the description is significantly stale (approach changed, scope shifted),
   AND you have drafted a concrete final title and body from the actual PR content:
   run gh pr edit with those real values. The strings "NEW TITLE" and
   "...updated body..." below are PLACEHOLDERS — never pass them literally; if you
   have not drafted concrete replacement text, SKIP the edit entirely so the real
   PR description is preserved.
   gh pr edit ${pr.number} --title "NEW TITLE" --body-file - <<'EOF'
   ...updated body...
   EOF
4. git push`,
  { model: 'haiku', effort: 'low', phase: 'Wrap' }
)

// ── Return summary ────────────────────────────────────────────────────────

const unclearUrls = unclear.map(t => (threadIndex[t.threadId] || {}).url || t.threadId)
const unappliedValidUrls = valid
  .filter(t => !appliedThreadIds.has(t.threadId))
  .map(t => (threadIndex[t.threadId] || {}).url || t.threadId)

return {
  status:   'done',
  pr:       `${pr.owner}/${pr.repo}#${pr.number}`,
  fixed:    fixes.filter(f => f.applied).length,
  invalid:  invalid.length,
  unclear:  [...unclearUrls, ...unappliedValidUrls],
  clusters: triage.clusters.length,
  commit:   buildResult.commitSha || '(none)',
}
