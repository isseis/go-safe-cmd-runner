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
  translator and editor").
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
  subagent reports no Critical or Major issues.
- After three review passes, continue only if the remaining Critical or Major
  issues are concrete, scoped to ARTIFACT, and clearly fixable without expanding
  the scope. Otherwise, stop and report the remaining issues instead of
  continuing automatically.

The calling command may add an extra rule after this procedure (e.g. "commit
only after all review passes are complete"). Follow any such rule.
