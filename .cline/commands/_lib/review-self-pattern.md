---
name: self-review
description: Run a critical review of implementation artifacts against architecture and requirements.
category: Test and Review
---

> **IMPORTANT — this is a self-review procedure without subagents.**
> The reviewer is the current conversation context (no subagent tool available).
> Follow the procedure below directly in this conversation.

## Procedure

Perform a code review of the implementation artifact provided in the top-level prompt.

1. Read **FILES** provided in the prompt in full. Then, read each source file added or modified in this phase in full.

2. Run `git diff <range>` and inspect the full set of changes.

3. Evaluate the implementation against the **CRITERIA** from the prompt, using a checklist to mark each item as pass/fail with specific findings.

4. For each issue found, classify its severity.
   - **Critical**: Behavioural defect, data-loss risk, security vulnerability, or handling of an error path whose fallout could be out of scope for other review and testing. (例：削除してはいけないポストを削除してしまう誤判定ロジック、AT Protocolの仕様に違反する実装、Slack通知の重大な誤報、資格情報のログ露出)
   - **Major**: Design-level concern or a maintainability hazard whose remediation would be hard later if deferred. (例：責务の誤配置 → テスタビリティ低下、他のタスクの前提を崩すインターフェース変更、コンフィギュレーション項目の一貫性欠如)
   - **Minor**: Convention, style, or documentation finding that improves clarity but does not block merging. (例：フォーマットの揺れ、英語表現の軽微な不備、コメントの過不足、Go idiomsの逸脱)
   - **Nit**: Subjective suggestion; purely stylistic.

5. Output your findings as follows:

  - Hit the **FULL** review checklist from the prompt, each item checked `[x]` pass / `[ ]` fail.
  - If Critical issues: list each, then stop — do not continue to Major/Minor/Nit.
  - Else, list Major issues.
  - Then, list Minor issues.
  - Finally, note Nits (no more than 5).
  - After findings, output the count of each severity level (Critical / Major / Minor / Nit).
  - End with: **Verdict: [PASS / BLOCKED]**, as in:
    - PASS: No Critical issues found.
    - BLOCKED: Critical issues present.

6. Keep the review output language (excluding quoted code and identifiers) in Japanese.

## Authoring and Maintenance

- This document is referenced from two places:
  - The main `/runplan` workflow (step 7), via inline self-review instructions.
  - The `/run-review` command (standalone review of a completed task group).
- This document is a subagent-free port of `.claude/commands/_lib/review-subagent-pattern.md`.
  It is designed for copilot tools that lack a subagent feature (Cline, Codex, etc.)
  and performs the same review inline in the current conversation.
