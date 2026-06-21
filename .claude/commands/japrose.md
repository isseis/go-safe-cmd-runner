> **Project context**: this command reviews and improves the **Japanese prose
> quality** of a document. It refers to "the document language" and "the
> translation glossary", whose values are defined in
> `.claude/commands/_context.md` (Process convention). Read that file and use its
> values. The review step follows the shared pattern in
> `.claude/commands/_lib/review-subagent-pattern.md`. The rest of this command is
> project-independent.

## Purpose

Improve the readability of a Japanese document by finding and fixing prose-quality
problems — **not** by changing its technical content, facts, numbers, structure, or
section order. This command targets the recurring problems that machine-assisted
Japanese writing tends to introduce:

1. **直訳調 (literal-translation tone)** — English word order, inanimate subjects,
   overuse of passive voice, and calqued idioms that read as a literal rendering of
   English rather than natural Japanese.
2. **不適切・不自然な語の用法** — a word that is technically wrong or unusual for the
   intended meaning (e.g. borrowing a kanji compound for a sense it does not carry,
   such as 「被覆」for "coverage"), or excessive katakana-English / raw English where a
   natural Japanese word exists.
3. **意味のくみ取りにくい一文** — overlong sentences, ambiguous modification (係り受け),
   missing subjects, or too many clauses packed into one sentence.
4. **未定義語の先行使用 (undefined term used before it is introduced)** — a term,
   abbreviation, or piece of jargon used before it is defined in reading order, so a
   reader going through the document top-to-bottom cannot understand it.
5. **用語の不統一** — the same concept written with more than one Japanese term or
   notation, or a deprecated term (旧称) that the glossary says not to use.
6. **冗長・回りくどい言い回し** — wording that can be made more direct without losing
   meaning.

## Preparation

Get the target file path from the argument. If no argument is provided and a task
document is open in the IDE, use that; otherwise ask the user which file to review.

**Scope guard — apply this command only to Japanese-primary documents:**

- This command operates on documents written in the document language (Japanese; see
  `_context.md`). Typical targets: task documents under `docs/tasks/`
  (`01_requirements.md`, `02_architecture.md`, `03_implementation_plan.md`) and
  Japanese guide files.
- Do **not** run it on an English `.md` that is the English half of a bilingual pair
  (its Japanese counterpart is `*.ja.md`); translation quality of those is handled by
  `mktrans`. If the target is such an English file, stop and say so.
- If the file is not in Japanese, stop and report that this command is for Japanese
  documents.

## Load the Glossary

Read the translation glossary (path in `_context.md`, Process convention). Use it as
the authority for canonical terminology: the document must use the glossary's terms
and must not use any term the glossary marks as deprecated (旧称). Do **not** invent
new terms; if a clearer term seems warranted but is not in the glossary, flag it as a
finding rather than silently introducing it.

## Review (via Subagent)

Run the critical-review subagent procedure in
`.claude/commands/_lib/review-subagent-pattern.md` in **single-reviewer mode** with
these inputs:

- **ARTIFACT**: the target document.
- **PERSONA**: a technical editor fluent in Japanese, whose job is to find prose that
  is unnatural, hard to follow, literally translated, or terminologically
  inconsistent — and to propose concrete rewrites that preserve the original meaning,
  structure, and section order. It must look for real problems and not rubber-stamp;
  if a category is clean it says so explicitly rather than inventing findings. It must
  prioritize the places where a reader actually stumbles over exhaustive nitpicking.
- **FILES**: the target document; the translation glossary (path in `_context.md`);
  and, when the target is a task document under `docs/tasks/<task>/`, the sibling task
  documents in the same directory that exist (`01_requirements.md`,
  `02_architecture.md`, `03_implementation_plan.md`) — all as resolved absolute-path
  strings. The sibling documents are **read-only reference** so the reviewer can check
  terminology consistency against them (e.g. a plan vs. its architecture); only the
  target document is edited.
- **CRITERIA**: every item from the Prose-quality checklist and the Hard constraints
  below, copied verbatim.

**Prose-quality checklist (use verbatim as evaluation criteria in the subagent prompt above):**
- [ ] 直訳調・英語語順の直訳（無生物主語の直訳、過剰な受動態、英語イディオムの逐語訳）が、意味を変えずに自然な日本語へ書き換えられている。
- [ ] 意図した意味に対して技術的に誤った/一般的でない語（例: "coverage" を「被覆」と当てる類）が、普通の日本語の語へ置き換えられている。
- [ ] 不要なカタカナ英語・生の英語が自然な日本語へ置き換えられている。ただし用語集に載る確立した専門用語、本文書で確立した略語、およびバッククォート内のコード識別子・コマンド名・型名は対象外（むやみに言い換えない）。
- [ ] 長すぎる一文・曖昧な係り受け・主語の欠落・詰め込みすぎの文が、意味を保ったまま分割・整理されて読みやすくなっている。
- [ ] 用語・略語・専門語が、読み進める順序で定義より前に使われていない（未定義の先行使用がない）。ある場合は、初出での簡潔な定義、または後続の定義箇所への参照（例:「後述の §X で定義」）で、頭から読んで理解できるようになっている。
- [ ] 同一概念には常に同一の日本語表記が使われ、用語集に一致している。用語集が「旧称」と記す語は使われていない。
- [ ] 冗長・回りくどい言い回しが、意味を変えずに簡潔にされている。
- [ ] 図（Mermaid）のラベル・キャプション中の日本語にも上記が適用されている（ノード識別子・コード識別子は除く）。

**Hard constraints (use verbatim; the reviewer must flag any fix that would violate one of these as out of scope):**
- [ ] 技術的な内容・事実・数値・固有名・構造・節の順序・見出し・文書ステータスを変更しない。表現だけを直す。
- [ ] コードブロック（```～```）の内部は変更しない——**ただし Mermaid 図のラベル/キャプションの表示テキストは対象とする**（Mermaid は ```mermaid のフェンス内にあるが、その表示テキストの日本語は本コマンドの対象）。Mermaid のノード/エッジ識別子、およびバッククォート内のコード識別子・コマンド名・関数名・型名は変更しない。編集対象は地の文と Mermaid 図のラベル/キャプションの表示テキストのみ。
- [ ] 用語集の語を保持し、新しい用語を勝手に導入しない。より良い語が必要なら、置換せずに指摘として挙げる。
- [ ] 既に文書内で一貫して使われている確立表現（例: fail-closed、max 合成、profile、Reject/Critical/High 等）を、「カタカナ/英語だから」という理由だけで言い換えない。

## Apply Fixes

From the review findings, apply the fixes to the document in place:

- Fix all Critical and Major findings. Apply Minor fixes at your discretion.
- For an **undefined-term** finding, prefer defining the term briefly at its first
  occurrence; use a pointer to where it is defined later (e.g.「後述の §X で定義」) when
  an inline definition would disrupt the flow. Do **not** reorder sections or move
  content to fix it — the structure-preservation Hard constraint forbids that; use a
  local inline definition or a forward pointer only.
- Never change meaning, numbers, code, or section order to satisfy a prose finding. If
  a finding cannot be fixed without violating a Hard constraint, leave the text and
  note why.
- Preserve Mermaid syntax and code fences exactly; re-check that fence counts stay
  balanced after editing diagram labels.

If any Critical or Major finding required a fix, run a second review pass to verify
the fixes (subject to the three-pass limit in the shared pattern), then stop.

## Commit

If this command was invoked **as a sub-step of another command** (e.g. `mkarch` /
`mkplan`), do **not** commit — return control to the calling command, which owns the
commit. Report the changes you made.

If this command was invoked **standalone**, commit the edited document after all
review passes are complete and all Critical and Major findings are resolved. Use a
`docs(<task-id>): improve Japanese prose` style message that lists the kinds of fixes
applied. (No need to wait for user confirmation before committing.)
