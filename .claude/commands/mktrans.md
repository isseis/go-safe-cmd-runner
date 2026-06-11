> **Project context**: this command refers to "the translation glossary". Its path
> is defined in `.claude/commands/_context.md` (Process convention: "Translation
> glossary"). Read that file and use its value. The translation direction is
> determined by the source file extension (see Preparation below), not by a
> runtime value from `_context.md` — `_context.md` describes the project's
> language-pair convention for reference only. The review step follows the shared
> pattern in `.claude/commands/_lib/review-subagent-pattern.md`.
> The rest of this command is project-independent.

## Preparation

Get the source file path from the argument.
If no argument is provided, ask the user.

This command is for translating bilingual documentation pairs (e.g. files under
`docs/dev/developer_guide/`) where a `.md` (English) and `.ja.md` (Japanese)
version exist or are being created together. Task documents under `docs/tasks/`
(`01_requirements.md`, `02_architecture.md`, `03_implementation_plan.md`) are
Japanese-primary working documents with no translation counterpart — do not run
this command on them.

Determine the translation direction from the file extension:

- Source ends in `.ja.md` → **Japanese → English**
  - Source: `foo.ja.md`
  - Output: `foo.md`
- Source ends in `.md` (but not `.ja.md`) → **English → Japanese**
  - Source: `foo.md`
  - Output: `foo.ja.md`

If the source file does not match either pattern, stop and ask the user to clarify.

## Mode Selection: Full Translation or Differential Translation

**If the output file does not exist**: proceed with full translation (see "Full Translation" section).

**If the output file already exists**: proceed with differential translation (see "Differential Translation" section).

---

## Full Translation

Translate the entire source file from scratch.

### Load Glossary

Read the translation glossary (path in `.claude/commands/_context.md`, Process convention).
Use the terms listed there consistently throughout the translation.

### Translate

Translate strictly following these principles:

- **Accuracy over fluency**: Prioritize precise translation over natural-sounding output.
- **Faithful translation**: Do not delete any content from the source. Do not add any content not present in the source.
- **Structural consistency**: Match chapter headings and sentence structure to the source.

Write the translated output to the output path.

---

## Differential Translation

Translate only the sections that changed in the source file since the output file was last updated.

### Find the Sync Point

Run the following command to find the last commit that modified the output file:

```bash
git log -1 --format=%H -- <output-file>
```

Record the commit hash (call it `SYNC_HASH`). If no hash is returned (e.g., the output file is not yet committed), stop and inform the user that differential translation requires the output file to be committed.

### Get the Diff

Run:

```bash
git diff SYNC_HASH -- <source-file>
```

If the diff is empty, the output file is already up to date. Stop and report this to the user.

### Load Glossary

Read the translation glossary (path in `.claude/commands/_context.md`, Process convention).
Use the terms listed there consistently throughout the translation.

### Translate Only Changed Sections

From the diff, identify which sections (by heading) were added, modified, or removed.

For each changed section:
- **Added lines** (`+`): translate the new source content into the target language.
- **Removed lines** (`-`): identify the corresponding content in the output file and remove it.
- **Modified lines**: translate the new source content and replace the old output content.

Apply these changes to the output file. Do not touch sections that are not in the diff.

---

## Update Glossary

If any terms were used during translation that are not in the glossary, add them to the translation glossary (path in `.claude/commands/_context.md`, Process convention).
Skip this step if no new terms were introduced.

## Review the Translation (via Subagent)

Run the critical-review subagent procedure in
`.claude/commands/_lib/review-subagent-pattern.md` with these inputs:

- **ARTIFACT**: the translation.
- **PERSONA**: an experienced technical translator and editor. Direct it to
  surface omissions, additions not in the source, mistranslations, and
  inconsistent terminology.
- **FILES**: the source file, the translated output file, and the translation
  glossary (path in `_context.md`) — all as resolved absolute-path strings.
- **CRITERIA**: every item from the Accuracy checklist and the Readability
  checklist below, copied verbatim.

**Accuracy checklist (use verbatim as evaluation criteria in the subagent prompt above):**
- [ ] No content from the source is missing in the translation.
- [ ] No content was added that is not present in the source.
- [ ] All technical terms match the glossary.

**Readability checklist (use verbatim as evaluation criteria in the subagent prompt above):**

The translation principles (Accuracy over fluency, Structural consistency) take precedence. The following checks apply only within those constraints: do not restructure sentences, reorder clauses, or rephrase in ways that would diverge from the source structure.

- [ ] Word choices that are technically correct but unnecessarily obscure are replaced with clearer equivalents that carry the same meaning and preserve the source structure.
- [ ] Terminology is used consistently throughout the translation; the same concept always uses the same term in the target language.
- [ ] Sentence structure follows target-language conventions where the source structure permits it; literal carry-overs from source syntax that produce unnatural output are corrected as long as doing so does not alter meaning or structure.

## Commit

Commit only after all review passes are complete and all Critical and Major issues are resolved.

Commit in the following order:
1. Commit the translated file only (do not include glossary changes).
2. If the glossary was updated, commit that as a separate commit.

(No need to wait for user confirmation before committing.)
