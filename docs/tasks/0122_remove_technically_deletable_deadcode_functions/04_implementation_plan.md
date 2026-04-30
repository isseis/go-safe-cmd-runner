# 技術的に削除可能な dead code 関数の削除計画

## 0. 目的

先行調査で抽出された `dead code 候補` および `要確認` のうち、
「残置予定コメントや将来計画を考慮せず、現時点で削除してもプロダクションコードの動作に影響しない関数」を削除する。

本計画は次を満たすことを目的とする。

1. プロダクション経路に未接続の関数を安全に削除する
2. 削除に伴うテスト修正を最小化する
3. 削除後の `go vet ./...`、`make lint`、`go test -tags test ./...` を成功させる

## 1. 対象スコープ

### 1.1 削除対象（dead code 候補）

- `internal/filevalidator/validator.go`: `buildSVCInfos`
- `internal/security/machoanalyzer/svc_scanner.go`: `containsSVCInstruction`

### 1.2 削除対象（要確認のうち技術的に削除可能）

- `internal/common/test_helpers.go`: `newResolvedPathForNew`
- `internal/runner/security/network_analyzer_test_helpers.go`: `newNetworkAnalyzerWithStores`
- `internal/runner/test_helpers_test.go`: `matchRuntimeGroupWithName`

注記:
- 上記はプロダクションコードの実行経路に接続されていないことを前提にする。
- 要確認群はテスト補助関数であり、削除時は呼び出し側テストを直接実装へ置換する。

### 1.3 非対象

- 構造体フィールド、インターフェース実装の未使用判定
- 既に削除済みのシンボル
- 削除により新たな設計変更を伴う API 再設計

## 2. 実施フェーズ

## 2.1 Phase A: 事前再確認

- [x] 対象 5 関数の参照箇所を `rg --glob '*.go'` で再確認
- [x] 参照を「本番」「テスト」「未参照」に分類
- [x] 依存するテスト修正箇所を一覧化

完了条件:
- [x] 削除対象 5 関数の最新参照マップが作成されている
- [x] プロダクション参照がないことを確認できる

## 2.2 Phase B: dead code 候補 2 件の削除

対象:
- `buildSVCInfos`
- `containsSVCInstruction`

作業:
- [x] 関数本体と関連コメントを削除
- [x] 呼び出しが残るテストは、代替ロジックへ置換またはテスト自体を統合
- [x] `make fmt` 実行
- [x] `go build ./...` 実行
- [x] `make lint` 実行
- [x] `go test -tags test ./...` 実行

完了条件:
- [x] 2 関数がコードベースから削除されている
- [x] 品質ゲート（build/lint/test）が成功

## 2.3 Phase C: 要確認関数 3 件の削除

対象:
- `newResolvedPathForNew`
- `newNetworkAnalyzerWithStores`
- `matchRuntimeGroupWithName`

作業:
- [x] 参照側テストを直接ロジック化（ヘルパー呼び出しをインライン化）
- [x] ヘルパー関数を削除
- [x] 付随する未使用定数/変数があれば同 PR で削除
- [x] `make fmt` 実行
- [x] `go build ./...` 実行
- [x] `make lint` 実行
- [x] `go test -tags test ./...` 実行

完了条件:
- [x] 3 関数がコードベースから削除されている
- [x] テスト可読性が低下していない（同等意図を維持）
- [x] 品質ゲート（build/lint/test）が成功

## 2.4 Phase D: 事後精査と結果更新

- [x] `go vet -tags=test ./...` 実行
- [x] `go run honnef.co/go/tools/cmd/staticcheck@latest -tags=test ./...` 実行
- [x] `golangci-lint run --build-tags=test --enable=unused,unparam,ineffassign` 実行
- [x] `go run golang.org/x/tools/cmd/deadcode@latest ./cmd/...` 実行
- [x] 結果を本タスク配下に記録（残件の dead code 候補を更新）

完了条件:
- [x] 対象 5 関数が再検出されない
- [x] 次の削除候補リストが更新されている

### 残件 dead code 候補（次フェーズ参考）

`go run golang.org/x/tools/cmd/deadcode@latest ./cmd/...` 出力より、以下が未使用として検出された（本タスクのスコープ外）:

| ファイル | シンボル | 備考 |
|---|---|---|
| internal/arm64util/arm64util.go | `BackwardScanX0` | 将来拡張向け |
| internal/runner/runner.go | `WithExecutor`, `WithResourceManager`, `WithGroupMembershipProvider` | テスト/外部向け API |
| internal/runner/audit/logger.go | `NewAuditLoggerWithCustom` | テスト向け |
| internal/runner/config/loader.go | `newLoaderInternal`, `NewLoaderForTest` | テスト向け |
| internal/runner/config/template_expansion.go | `ValidateParams` | 外部向け API |
| internal/runner/config/validation.go | `ValidateEnvImport` | 外部向け API |
| internal/runner/resource/normal_manager.go | `NewNormalResourceManagerWithOutput` | テスト向け |
| internal/runner/security/network_analyzer.go | `NewNetworkAnalyzer`, `NewNetworkAnalyzerWithStore` | 外部向け API |
| internal/runner/variable/registry.go | `NewRegistry` 他 | 外部向け API |
| internal/safefileio/safe_file.go | `SafeWriteFile`, `SafeAtomicMoveFile` 他 | 外部向け API |
| internal/security/machoanalyzer/svc_scanner.go | `ScanSVCAddrs` | 将来拡張向け |
| その他 | Error メソッド群、String メソッド群 | インターフェース実装 |

## 3. コミット戦略

- [x] Commit 1: Phase B（dead code 候補 2 件）
- [x] Commit 2: Phase C（要確認関数 3 件）
- [x] Commit 3: Phase D（結果更新ドキュメント）

ルール:
- 1 フェーズ完了ごとにコミットする
- 失敗で実施しなかった項目は `[-]` を付ける
- コミットメッセージは英語、1 行サマリ + 箇条書き本文

## 4. リスクと対策

- テストロジック劣化:
  - 対策: ヘルパー削除時にテスト意図コメントを残し、同等アサーションを維持
- 削除漏れによる lint 失敗:
  - 対策: `rg` と `unused` 出力を突き合わせ、関連シンボルを同時削除
- 非意図の本番影響:
  - 対策: `go build ./...` と `go vet ./...` を必須ゲート化

## 5. 進捗管理

### 5.1 ステータス

- [x] Phase A 実施中
- [x] Phase A 完了
- [x] Phase B 実施中
- [x] Phase B 完了
- [x] Phase C 実施中
- [x] Phase C 完了
- [x] Phase D 実施中
- [x] Phase D 完了

### 5.2 実行ログ

- 実施日:
- ブランチ:
- 実施者:
- 実行コマンド:
- 結果サマリ:
- 課題/ブロッカー:
- 次アクション:

- 実施日: 2026-04-30
- ブランチ: issei/deadcode-removal-02
- 実施者: GitHub Copilot
- 実行コマンド: `rg --glob '*.go'`（対象5関数の参照確認）
- 結果サマリ: 対象5関数はいずれも本番参照なし。テスト参照箇所を一覧化。
- 課題/ブロッカー: なし
- 次アクション: Phase B で `buildSVCInfos` と `containsSVCInstruction` を削除

- 実施日: 2026-04-30
- ブランチ: issei/deadcode-removal-02
- 実施者: GitHub Copilot
- 実行コマンド: `make fmt`, `go build ./...`, `make lint`, `go test -tags test ./...`
- 結果サマリ: `buildSVCInfos` と `containsSVCInstruction` を削除し、関連テストを代替ロジックへ置換。
- 課題/ブロッカー: なし
- 次アクション: Phase C で要確認3関数を削除

- 実施日: 2026-04-30
- ブランチ: issei/deadcode-removal-02
- 実施者: GitHub Copilot
- 実行コマンド: `make fmt`, `go build ./...`, `make lint`, `go test -tags test ./...`, `rg --glob '*.go'`
- 結果サマリ: `newResolvedPathForNew`、`newNetworkAnalyzerWithStores`、`matchRuntimeGroupWithName` を削除し、テストを直接ロジック化して置換。
- 課題/ブロッカー: なし
- 次アクション: Phase D の事後精査コマンドを実行

- 実施日: 2026-04-30
- ブランチ: issei/deadcode-removal-02
- 実施者: GitHub Copilot
- 実行コマンド: `go vet -tags=test ./...`, `staticcheck -tags=test ./...`, `golangci-lint run --build-tags=test --enable=unused,unparam,ineffassign`, `deadcode ./cmd/...`
- 結果サマリ: 対象5関数はいずれも再検出されず。残件候補（将来タスク用）を計画書に記録。
- 課題/ブロッカー: なし
- 次アクション: Phase D 完了。PR の更新・マージ作業へ移行。

## 6. レビュー観点

- 削除対象はプロダクション経路に未接続か
- テスト修正は最小差分で、重複ロジックを増やしていないか
- コメントは英語のみか
- dead code 再検出結果が計画と一致しているか
