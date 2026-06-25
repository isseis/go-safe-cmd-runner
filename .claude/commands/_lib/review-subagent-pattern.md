# Shared pattern: critical-review subagent

Several commands end by spawning a subagent to critically review the artifact
they just produced. The procedure is identical except for four inputs that the
calling command supplies. This file defines the procedure once; commands invoke
it by saying "follow `_lib/review-subagent-pattern.md`" and providing the four
inputs.

This is project-independent. It depends on nothing in `_context.md`.

## Inputs the calling command must supply

- **ARTIFACT** — what is being reviewed (e.g. "the created `02_architecture.md`",
  "this phase group's code changes", "the translation").
- **PERSONA** — the reviewer role and focus (e.g. "an experienced software
  architect and senior SRE", "a senior Go engineer and senior SRE", "a technical
  translator and editor"). This may be **a single persona** (single-reviewer mode,
  the default) or **a panel of two-or-more partitioned personas** (panel mode — see
  "Panel mode" below). A panel persona is specified as a role PLUS a non-overlapping
  **mandate** (what it owns) and an explicit **out-of-scope** list (what the other
  panel personas own — for a two-reviewer panel this is simply the counterpart).
- **FILES** — the list of files the subagent must read, given as resolved
  absolute path strings so the subagent does not rely on the caller's context.
- **CRITERIA** — the checklist(s) the caller defines, to be copied verbatim into
  the subagent prompt.

## Procedure

Spawn a review subagent using the Agent tool to critically evaluate ARTIFACT.
Construct a self-contained prompt that includes all of the following:

- **Persona**: act as PERSONA whose job is to find real problems — not to
  approve. Be thorough and unsparing. Surface gaps, ambiguities, and risks. Do
  not soften findings.
- **Files to read**: embed each path in FILES as a literal absolute-path string
  in the prompt so the subagent can read them without relying on your context.
- **Evaluation criteria**: every item from CRITERIA, copied verbatim.
- **Output format**: for each issue found, report Severity (Critical / Major /
  Minor), Location (section name, file and line, or checklist item), Problem
  (what is wrong or missing), and Suggestion (concrete fix). If a checklist
  category has no issues, state that explicitly.

After receiving findings:

- Fix all Critical and Major issues.
- Apply Minor fixes at your discretion.
- If any Critical or Major issue required a fix, spawn a second review subagent
  to verify the fixes. Repeat, subject to the three-pass limit below, until the
  subagent reports no Critical or Major issues. A **verification** pass confirms a
  bounded set of already-located fixes rather than searching the whole artifact for
  unknown problems, so it may run on a cheaper model than the initial review (see
  "Model tiering" below).
- After three review passes, continue only if the remaining Critical or Major
  issues are concrete, scoped to ARTIFACT, and clearly fixable without expanding
  the scope. Otherwise, stop and report the remaining issues instead of
  continuing automatically.

The calling command may add an extra rule after this procedure (e.g. "commit
only after all review passes are complete"). Follow any such rule.

## Model tiering (cost control)

The two kinds of pass have different difficulty, so they need not run on the same
model:

- **Initial (discovery) review** — searches the whole ARTIFACT for unknown
  problems across every CRITERIA item. This is the hard, high-recall task; run it
  on the session's default model. Do **not** downgrade it — a cheaper model here
  misses findings, which is the expensive failure mode.
- **Verification re-review** — confirms that a bounded, already-located set of
  Critical/Major fixes was applied correctly and introduced no regression. This is
  a narrower check and **may run on a cheaper model**. When spawning the
  verification subagent, pass the explicit list of fixes to confirm and request a
  cheaper model (the Agent tool's `model` parameter, e.g. a faster/cheaper tier);
  if the cheaper pass reports anything ambiguous or a new Critical/Major issue,
  re-run that pass once on the default model before trusting it.

This keeps recall where it matters (discovery) while cutting cost on the repeated
confirmation passes. It composes with panel mode below: the discovery panel runs
on the default model; a single combined verification pass may run cheaper.

## Panel mode (multiple partitioned reviewers)

Single-reviewer mode is the default and right for small or low-risk artifacts. For
**large, high-stakes artifacts** (e.g. a security- or operability-critical
architecture), a single combined reviewer tends to *satisfice* — it produces a
blended list and under-weights one lens. Panel mode runs **two-or-more reviewers,
each with a non-overlapping mandate**, so each goes deep in its domain.

A calling command opts into panel mode by supplying a PERSONA that is a panel
(two-or-more partitioned personas). When PERSONA is a panel:

1. **Partition the mandate, not just the label.** Two reviewers with the same scope
   but different titles mostly duplicate each other — you pay N× tokens for largely
   the same output. Each panel persona must get: (a) a **mandate** = the dimensions
   it owns and must go deep on; (b) an explicit **out-of-scope** list = the
   dimensions the other panel personas own (collectively, when the panel has more
   than two), which it must NOT report except as one-line "OUT-OF-LANE FLAGS" at the
   end. Route the CRITERIA items to the reviewer whose
   mandate covers them (an item may be shared only when both lenses genuinely apply).
2. **Spawn all panel reviewers in parallel** — in a single message with multiple
   Agent tool calls — so wall-clock does not increase. Each prompt is self-contained
   (embed FILES as absolute paths; copy that reviewer's CRITERIA verbatim;
   state mandate + out-of-scope; require the same Severity/Location/Problem/Suggestion
   output format). Two ways to give CRITERIA — the calling command picks one and is
   consistent: (a) **pre-routed** — give each reviewer only the checklist subset its
   mandate owns; or (b) **shared floor** — give every reviewer the full checklist and
   have each report only the items within its mandate, raising an out-of-mandate item
   ONLY as a one-line OUT-OF-LANE FLAG when it actually spots a potential issue there
   (it does NOT enumerate clean out-of-mandate checklist items — silence means no
   concern). Shared floor needs no item-by-item routing and avoids coverage gaps from
   mis-routing; pre-routing minimizes each reviewer's reading.
3. **Synthesize before fixing.** The caller (not a subagent) merges the panel
   outputs: **dedup** overlapping findings; **reconcile** conflicting severities
   (take the higher); and **reconstruct cross-cutting issues** that sit in the seam
   between mandates (a structural choice with an operational consequence) — this
   synthesis is panel mode's main risk to mitigate, since the parallel reviewers
   cannot see each other's output. For each "OUT-OF-LANE FLAG", the caller checks it
   against the mandate and findings of the panel persona(s) that own that dimension
   during this synthesis and promotes it to a real finding if it holds up.
4. **Then run the same fix / re-review loop** as the Procedure above on the merged
   findings. A verification pass may reuse panel mode or a single combined reviewer.

**Cost.** Panel mode reads the FILES once per reviewer (~N× input tokens). Use it
when the marginal findings justify the cost; prefer single-reviewer mode otherwise.
