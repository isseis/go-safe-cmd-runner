# 実装計画書: verification から runner 依存を除去する

## 概要

`internal/verification` から `internal/runner/runnertypes` および
`internal/runner/security` への依存を除去し、`runner` から `verification` への
一方向依存に整理する。

本タスクでは、verification 側に runner 非依存の DTO とインターフェースを導入し、
境界で型変換を行う。あわせて、`PathResolver` のデッドコードを削除し、
本番用 `DirectoryValidator` の生成責務を `runner/bootstrap` 側へ移す。

## 進捗チェックリスト

### Phase 1: 前提確認

- [x] 現状の依存を確認: `go list -deps github.com/isseis/go-safe-cmd-runner/internal/verification | grep internal/runner`
- [x] task 0115 の継続条件を確認: `go list -deps ./cmd/record | grep internal/runner`
- [x] task 0115 の継続条件を確認: `go list -deps ./cmd/verify | grep internal/runner`
- [x] ベースラインビルドを確認: `go build ./cmd/record ./cmd/verify ./cmd/runner`
- [x] ベースラインテストを確認: `make test`

### Phase 2: verification の入力境界を runner 非依存化

対象: `internal/verification` に runner 非依存の入力 DTO と最小インターフェースを定義する。

- [ ] `internal/verification` に `GroupVerificationInput` を追加
- [ ] `internal/verification` に `GlobalVerificationInput` を追加
- [ ] `internal/verification` に `CommandEntry` を追加
- [ ] `internal/verification` に `DirectoryValidator` インターフェースを追加
- [ ] `verification.ManagerInterface` から `runnertypes.RuntimeGroup` / `runnertypes.RuntimeGlobal` 参照を除去
- [ ] `internal/verification` から `internal/runner/runnertypes` import を除去

### Phase 3: verification.Manager の本体を DTO ベースへ置換

対象: manager 本体のシグネチャと内部処理を DTO ベースへ切り替える。

- [ ] `VerifyGroupFiles` の引数を `*GroupVerificationInput` に変更
- [ ] `VerifyGlobalFiles` の引数を `*GlobalVerificationInput` に変更
- [ ] `collectVerificationFiles` の引数を `*GroupVerificationInput` に変更
- [ ] グループ名取得を `runnertypes.ExtractGroupName` 依存から `input.Name` 利用へ置換
- [ ] コマンド展開文字列の走査を `CommandEntry.ExpandedCmd` 利用へ置換
- [ ] `ValidateHashDirectory` の分岐順をスキップ判定優先へ整理

### Phase 4: security 依存を抽象化しデッドコードを削除

対象: verification から `internal/runner/security` への直接依存を除去する。

- [ ] `PathResolver.security` フィールドを削除
- [ ] `NewPathResolver` から `security` 引数を削除
- [ ] `internal/verification` に固定 PATH 定数を追加
- [ ] `manager.go` で `security.SecurePathEnv` 参照を新定数へ置換
- [ ] `Manager.security` フィールド型を `DirectoryValidator` へ変更
- [ ] `internal/verification` から `internal/runner/security` import を除去

### Phase 5: Manager 生成責務を production / dry-run で分離

対象: 本番用 validator 生成を `runner/bootstrap` に移し、verification は注入を受ける。

- [ ] `internal/verification/manager_production.go` から `NewManager()` を削除
- [ ] `internal/verification` に `NewManagerForProduction(DirectoryValidator)` を追加
- [ ] `NewManagerForDryRun()` を security 非依存のまま維持
- [ ] `internal/verification` に内部オプション `withDirectoryValidatorInternal` を追加
- [ ] `internal/runner/bootstrap` に `NewVerificationManager()` を追加
- [ ] `cmd/runner/main.go` の本番経路を `bootstrap.NewVerificationManager()` 呼び出しへ変更

### Phase 6: runner 側の呼び出し境界で DTO へ変換

対象: 上位レイヤーで `RuntimeGroup` / `RuntimeGlobal` から DTO へ変換する。

- [ ] `internal/runner/group_executor.go` で `GroupVerificationInput` 生成を追加
- [ ] `internal/runner/group_executor.go` でコマンド一覧を `[]verification.CommandEntry` に変換
- [ ] `cmd/runner/main.go` で `GlobalVerificationInput` 生成を追加
- [ ] 既存の呼び出しシーケンスとエラーハンドリングを維持

### Phase 7: テストダブルとヘルパーを DTO ベースへ更新

対象: verification / runner テストヘルパーを新シグネチャへ追従させる。

- [ ] `internal/verification/testing/testify_mocks.go` の `MockManager.VerifyGroupFiles` を DTO シグネチャへ変更
- [ ] `internal/verification/testing/helpers.go` のマッチャを `GroupVerificationInput.Name` ベースへ変更
- [ ] `internal/runner/test_helpers.go` のマッチャを DTO ベースへ変更
- [ ] `internal/verification/manager_test.go` の入力ヘルパーを DTO 型へ置換
- [ ] `internal/verification/testing/testify_mocks_test.go` のテスト入力を DTO 型へ置換
- [ ] テストコードから不要になった `runnertypes` import を削除

### Phase 8: 回帰テストの妥当性確認と最小補強

対象: 受け入れ条件を満たすうえで必要十分なテストだけを維持し、重複を避ける。

- [ ] `cmd/runner` のグループファイル検証をカバーする既存テストを特定
- [ ] `cmd/runner` のグローバルファイル検証経路変更をカバーする既存テストを確認
- [ ] 既存テストで AC-5 を満たせるなら重複テストを追加しない
- [ ] グローバル検証経路が既存テストで確認できるなら専用の重複テストを追加しない
- [ ] 既存テストだけでは AC-5 を満たせない場合のみ最小の回帰テストを追加
- [ ] グローバル検証経路が未カバーの場合のみ最小の回帰テストを追加
- [ ] DTO 変換自体を過剰に個別テストせず、既存の振る舞いテストで検証する方針を確認
- [ ] 新規または更新テストに日本語文字列を追加しない

### Phase 9: 整形・ビルド・受け入れ基準の検証

- [ ] `make fmt` を実行
- [ ] `go build ./cmd/record ./cmd/verify ./cmd/runner` を実行
- [ ] `make test` を実行
- [ ] `make lint` を実行
- [ ] AC-1 を確認: `go list -deps github.com/isseis/go-safe-cmd-runner/internal/verification | grep internal/runner` が 0 件
- [ ] AC-6 を確認: `go list -deps ./cmd/record | grep internal/runner` が 0 件
- [ ] AC-6 を確認: `go list -deps ./cmd/verify | grep internal/runner` が 0 件

### Phase 10: 実装レビュー

対象: ドキュメントと実装の整合性、および非機能面の抜け漏れを確認する。

- [ ] 受け入れ基準 AC-1 から AC-6 がすべて計画内の作業で満たせることを確認
- [ ] 要件定義書と実装計画書の変更対象に矛盾がないことを確認
- [ ] テストが十分で、同じ振る舞いを重複検証していないことを確認
- [ ] `VerifyGroupFiles` と `VerifyGlobalFiles` の両方で変更境界が既存または最小追加テストで確認できることを確認
- [ ] 既存の再利用可能な関数や型変換を使わずに重複実装していないことを確認
- [ ] 本番コードとテストコードに不要な日本語文字列や日本語コメントを追加していないことを確認

## 受け入れ基準との対応

| AC | 基準 | 対応フェーズ |
|----|------|-------------|
| AC-1 | `go list -deps github.com/isseis/go-safe-cmd-runner/internal/verification \| grep internal/runner` が 0 件 | Phase 2-5, 9 |
| AC-2 | `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功 | Phase 2-7, 9 |
| AC-3 | `make test` が全件パス | Phase 7-8, 9 |
| AC-4 | `verification.ManagerInterface` のメソッドシグネチャが `runnertypes.RuntimeGroup` / `runnertypes.RuntimeGlobal` を参照しない | Phase 2-3 |
| AC-5 | `cmd/runner` のグループファイル検証がテストで継続確認される | Phase 6-8, 9 |
| AC-6 | task 0115 の AC-1・AC-2 が継続して成立する | Phase 1, 9 |

## 注意事項

- Phase 2 から Phase 7 はコンパイルエラーを長く残さないよう、インターフェース変更と呼び出し側変更を近接して進める
- `DirectoryValidator` は必要最小限の 1 メソッドだけを持たせ、security 実装の詳細を露出させない
- `PathResolver` の `security` フィールドはデッドコードとして削除し、代替の抽象化は導入しない
- `NewManagerForDryRun()` は引き続き verification パッケージに残し、dry-run で不要な production 初期化を持ち込まない
- ドキュメントは日本語で記述するが、本番コードとテストコードに新規の日本語コメントや日本語文字列は追加しない
