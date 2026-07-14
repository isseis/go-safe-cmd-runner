# Project context

- task root: `docs/tasks/`
- devdocs root: `docs/dev/`
- project root: `.`
- source root: `.`
- guide: `docs/dev/developer_guide/`
  - requirements_process: `docs/dev/developer_guide/requirements_process.md`
  - test_organization: `docs/dev/developer_guide/test_organization.md`
  - task_identification: `docs/dev/developer_guide/task_identification.md`
  - package_reference: `docs/dev/developer_guide/package_reference.md`
  - mermaid_reference: `docs/dev/developer_guide/mermaid_reference.md`
- implementation design: `03_implementation_plan.md`
- requirements: `01_requirements.md`
- architecture: `02_architecture.md`
- document_status keyword: `ドキュメントステータス`
- document_status values: `draft` → `approved`
- document language: Japanese
- language: Go
- source-language rule: Go comments, identifiers, and string literals must be English
- shared test helpers: `testutil/` (cross-package use) or `test_helpers.go` / `test_helpers_<category>.go` (package-internal, with the `//go:build test` tag)

- build:
  - format: `make fmt`
  - test: `make test`
  - lint: `make lint`
  - deadcode: `make deadcode`
  - green-gate (combined): `make fmt && make test && make lint && make deadcode`

- Domain-specific (go-safe-cmd-runner):
  - translation glossary: `docs/translation_glossary.md`
  - translation language pair: Japanese (primary) ⇄ English *(reference only — mktrans.md determines direction from file extension)*
  - Invariants for generated values: (none — this project is a consumer of upstream APIs, so ID/name generation rules belong to those upstream APIs)
  - Invariants for `--dry-run`: no external write/delete/unfollow side effects; every external API
  - Conditional security guide: `docs/dev/architecture_design/security-architecture.md`
  - Conditional-guide trigger: the feature touches privilege elevation/de-escalation, command execution, file path validation, file integrity verification, or sends notifications (e.g. Slack)
  - Target client environments: Slack (`make slack-notify-test`, `make slack-group-notification-test`)
