# 動的リンクライブラリ経由 `mprotect(PROT_EXEC)` 検出 実装計画書

## 0. 方針

本計画は、要件定義書の AC-1〜AC-6 を最短で満たしつつ、既存実装の再利用を優先して
変更範囲を最小化する。

- 変更の主軸:
  - `record` 側: ライブラリ解析結果への `ArgEvalResults` 保存
  - `runner` 側: dynlib 由来 `ArgEvalResults` の高リスク判定反映
- 再利用優先:
  - 既存の `elfanalyzer.EvalMprotectRisk` を流用し、独自判定関数は新設しない
- 回帰防止:
  - 既存ネットワーク判定テストを維持し、追加テストは不足ケースのみに限定する

---

## 1. 進捗トラッキング

### Phase 1: 影響分析と設計確定

- [ ] P1-1 変更点を 2 箇所に限定する（`filevalidator` / `network_analyzer`）
- [ ] P1-2 `ArgEvalResults` の保存先が dynlib ストアスキーマで欠落しないことを確認する
- [ ] P1-3 高リスク判定ロジックは `EvalMprotectRisk` 再利用で実装する方針を確定する
- [ ] P1-4 既存 AC（要件定義書）との対応表を作成する

### Phase 2: `record` 側実装

対象:

- `internal/filevalidator/validator.go`

タスク:

- [ ] P2-1 `AnalyzeSyscallsFromELF` の戻り値で `ArgEvalResults` を受け取る
- [ ] P2-2 `detected_syscalls` 0 件でも `arg_eval_results` があれば `SyscallAnalysis` を保存する
- [ ] P2-3 既存挙動（非 ELF、unsupported arch、warning 伝播）に回帰がないことを確認する

### Phase 3: `runner` 側実装

対象:

- `internal/runner/base/security/network_analyzer.go`

タスク:

- [ ] P3-1 dynlib ループ内で `result.SyscallAnalysis.ArgEvalResults` を評価する
- [ ] P3-2 `mprotect`/`pkey_mprotect` の `exec_confirmed`/`exec_unknown` で `isHighRisk=true` を設定する
- [ ] P3-3 ネットワーク判定・動的ロード判定との OR 合成が維持されることを確認する
- [ ] P3-4 可観測性要件として `cmd_path` と `dep_path` を含むログ出力を追加/確認する

### Phase 4: テスト実装（不足分のみ）

対象:

- `internal/filevalidator/validator_library_analysis_test.go`
- `internal/runner/base/security/network_analyzer_test.go`

タスク:

- [ ] P4-1 AC-1 用テスト: ライブラリ解析結果に `ArgEvalResults` が保存されること
- [ ] P4-2 AC-2〜AC-4 用テスト: `exec_confirmed` / `exec_unknown` / `exec_not_set` を table-driven で検証する
- [ ] P4-5 重複防止: 既存の dynlib ネットワーク判定テストと重なるケースを追加しないこと

### Phase 5: 品質確認

- [ ] P5-1 `make fmt`
- [ ] P5-2 `go test -tags test -v ./internal/filevalidator ./internal/runner/base/security`
- [ ] P5-3 `make test`
- [ ] P5-4 `make lint`

### Phase 6: 実装計画書レビューと修正

- [ ] P6-1 要件定義書との整合をレビュー（AC が計画タスクに紐づくこと）
- [ ] P6-2 非機能要件レビュー（テストカバレッジ、パフォーマンス、可観測性）
- [ ] P6-3 テスト重複レビュー（無意味・重複ケースの排除）
- [ ] P6-4 再利用レビュー（既存関数を使わず自前実装していないこと）
- [ ] P6-5 日本語混入レビュー（Go ソース/テストコードに日本語文字列を追加していないこと）

---

## 2. AC トレーサビリティ

| AC | 対応タスク | 検証方法 |
|----|------------|----------|
| AC-1 | P2-1, P2-2, P4-1 | `validator_library_analysis_test` で `ArgEvalResults` 保存を検証 |
| AC-2 | P3-1, P3-2, P4-2 | `network_analyzer_test` table-driven で `exec_confirmed` 高リスク化を検証 |
| AC-3 | P3-1, P3-2, P4-2 | `network_analyzer_test` table-driven で `exec_unknown` 高リスク化を検証 |
| AC-4 | P3-1, P3-2, P4-2 | `network_analyzer_test` table-driven で `exec_not_set` 非高リスクを検証 |
| AC-5 | P4-5, P5-2, P5-3 | 既存 dynlib ネットワーク判定テストの回帰がないこと |
| AC-6 | P5-2 | 対象パッケージの `go test -tags test` 成功 |

---

## 3. 重複防止ポリシー

- 既存テストが既に保証する内容は新規追加しない
- 新規テストは「今回追加した分岐」のみを対象とする
- テスト名は意図が一意に伝わる粒度で命名し、同義の検証を別名で重複させない

---

## 4. レビュー結果（本計画書作成後）

### 4.1 要件整合

- FR/AC とのトレーサビリティは AC-1〜AC-6 で確保
- AC-2〜AC-4 は個別テスト乱立を避け、P4-2 の table-driven 1 系列で満たす方針に修正

### 4.2 非機能要件

- パフォーマンス: 判定は既存 `ArgEvalResults` 走査を再利用し、追加計算量は O(n)
- テストカバレッジ: 正常系（confirmed/unknown）と抑制系（not_set）を同一系列で網羅

### 4.3 テスト重複レビュー

- 既存の dynlib ネットワーク/動的ロード判定テストは維持し、今回の追加は PROT_EXEC 分岐のみを対象化

### 4.4 実装再利用レビュー

- `runner` 側判定は `elfanalyzer.EvalMprotectRisk` を利用する前提を維持
- mprotect 判定の自前ロジック新設は行わない

### 4.5 コード言語レビュー

- Go ソース/テストで日本語文字列を追加しないことをチェック項目として維持

---

## 5. 完了判定

以下をすべて満たした時点で完了とする。

- [ ] AC-1〜AC-6 のトレーサビリティ行がすべて完了済み
- [ ] `make fmt` / `make test` / `make lint` が成功
- [ ] 実装計画書レビュー（Phase 6）の全チェックが完了
