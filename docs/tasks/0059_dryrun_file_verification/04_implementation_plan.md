# Dry-Run モードでのファイル検証機能 - 実装計画書

## 1. 概要

本ドキュメントは、dry-run モードでのファイル検証機能の実装計画を定義する。詳細仕様書（`03_specification.md`）に基づき、実装フェーズ、タスク分割、優先順位、リスク管理、レビュー基準を明確化する。

### 1.1 実装の基本方針

1. **段階的実装**: 機能を4つのフェーズに分割し、各フェーズで動作するコードを提供
2. **テスト駆動**: 各フェーズで対応するテストを先に実装
3. **副作用抑制の保証**: 各フェーズで dry-run モードの副作用抑制を確認
4. **回帰テスト**: 既存機能への影響を継続的に検証

## 2. 実装フェーズ

### 2.1 Phase 1: Result Collector Implementation

**目的**: 検証結果を収集・集約する基盤コンポーネントの実装

**期間**: 2日

**成果物**:
- `internal/verification/result_collector.go`
- `internal/verification/result_collector_test.go`
- データ構造体の定義

**完了条件**:
- [x] すべてのユニットテストが成功
- [x] コードカバレッジ > 90%
- [x] 並行アクセステストが成功
- [x] ベンチマークテストが性能要件を満たす（< 1000 ns/op for RecordSuccess）

### 2.2 Phase 2: Verification Manager Extension

**目的**: `verification.Manager` を拡張し、dry-run モードでの検証を有効化

**期間**: 3日

**成果物**:
- `internal/verification/manager.go` の修正
- `internal/verification/manager_test.go` の拡張
- 統合テスト

**完了条件**:
- [ ] `NewManagerForDryRun` が File Validator を有効化
- [ ] `verifyFileWithFallback` が warn-only モードで動作
- [ ] すべての検証メソッドが `context` パラメータを受け取る
- [ ] Hash Directory 不在時の処理が正しく動作
- [ ] 統合テストが成功

### 2.3 Phase 3: DryRunResult and Formatter Extension

**目的**: dry-run 結果の表示に検証サマリーを統合

**期間**: 2日

**成果物**:
- `internal/runner/resource/types.go` の修正
- `internal/runner/resource/formatter.go` の拡張
- フォーマッタテスト

**完了条件**:
- [ ] `DryRunResult` に `FileVerification` フィールドが追加
- [ ] TEXT フォーマッタが検証サマリーを表示
- [ ] JSON フォーマッタが検証サマリーを含む
- [ ] フォーマッタテストが成功

### 2.4 Phase 4: E2E Testing and Documentation

**目的**: E2E テスト、パフォーマンステスト、ドキュメント整備

**期間**: 2日

**成果物**:
- E2E テストスクリプト
- パフォーマンステスト結果
- ユーザーガイド更新
- CHANGELOG.md 更新

**完了条件**:
- [ ] すべての E2E テストが成功
- [ ] パフォーマンス要件を満たす（検証オーバーヘッド < 30%）
- [ ] 既存の dry-run テストの回帰確認
- [ ] ドキュメントのレビュー完了

## 3. 詳細タスク分解

### 3.1 Phase 1: Result Collector Implementation

#### Task 1.1: データ構造体の定義

**優先度**: 高
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `internal/verification/result_collector.go` を作成
2. 以下の構造体を定義:
   - `FileVerificationSummary`
   - `HashDirectoryStatus`
   - `FileVerificationFailure`
   - `VerificationFailureReason` (列挙型)

**検証方法**:
```bash
go build ./internal/verification
```

**成果物チェックリスト**:
- [x] `FileVerificationSummary` の全フィールドが定義済み
- [x] JSON タグが正しく設定されている
- [x] `VerificationFailureReason` の全定数が定義済み
- [x] ビルドエラーがない

---

#### Task 1.2: ResultCollector の基本実装

**優先度**: 高
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `ResultCollector` 構造体の定義
2. `NewResultCollector` コンストラクタの実装
3. `RecordSuccess` メソッドの実装
4. `RecordFailure` メソッドの実装
5. `RecordSkip` メソッドの実装
6. `SetHashDirStatus` メソッドの実装
7. `GetSummary` メソッドの実装

**検証方法**:
```bash
go build ./internal/verification
```

**成果物チェックリスト**:
- [x] `sync.Mutex` によるスレッドセーフティが実装されている
- [x] 不変条件が維持されている（`TotalFiles = VerifiedFiles + SkippedFiles + FailedFiles`）
- [x] ビルドエラーがない

---

#### Task 1.3: ヘルパー関数の実装

**優先度**: 中
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `determineFailureReason(error) VerificationFailureReason` の実装
2. `determineLogLevel(VerificationFailureReason) string` の実装
3. `getSecurityRisk(VerificationFailureReason) string` の実装

**検証方法**:
```bash
go build ./internal/verification
```

**成果物チェックリスト**:
- [x] `errors.Is` を使用したエラー分類が正しい
- [x] ログレベルのマッピングが仕様通り
- [x] セキュリティリスクの分類が適切

---

#### Task 1.4: ユニットテストの実装

**優先度**: 高
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `TestResultCollector_RecordSuccess`
2. `TestResultCollector_RecordFailure`
3. `TestResultCollector_RecordSkip`
4. `TestResultCollector_SetHashDirStatus`
5. `TestResultCollector_GetSummary`
6. `TestResultCollector_Concurrency`
7. `TestDetermineFailureReason`
8. `TestDetermineLogLevel`

**検証方法**:
```bash
go test -tags test -v ./internal/verification -run TestResultCollector
go test -tags test -cover ./internal/verification
```

**成果物チェックリスト**:
- [x] すべてのテストが成功
- [x] コードカバレッジ > 90% (100% achieved)
- [x] 並行アクセステスト（100 goroutines）が成功
- [x] Race detector でエラーがない（`go test -race`）

---

### 3.2 Phase 2: Verification Manager Extension

#### Task 2.1: Manager 構造体の拡張

**優先度**: 高
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `internal/verification/manager.go` に `resultCollector` フィールドを追加
2. `GetVerificationSummary()` メソッドを実装

**検証方法**:
```bash
go build ./internal/verification
```

**成果物チェックリスト**:
- [x] `resultCollector *ResultCollector` フィールドが追加されている
- [x] `GetVerificationSummary()` が nil チェックを実施
- [x] ビルドエラーがない

---

#### Task 2.2: NewManagerForDryRun の変更

**優先度**: 高
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `NewManagerForDryRun()` から `withFileValidatorDisabledInternal()` を削除
2. `newManagerInternal()` で `ResultCollector` を初期化
3. Hash Directory 不在時の処理を実装

**検証方法**:
```bash
go test -tags test -v ./internal/verification -run TestNewManagerForDryRun
```

**成果物チェックリスト**:
- [x] dry-run モードで File Validator が有効化されている
- [x] Hash Directory 不在時に `ResultCollector` が初期化される
- [x] Hash Directory 不在時に INFO ログが出力される
- [x] テストが成功

---

#### Task 2.3: verifyFileWithFallback の変更

**優先度**: 高
**担当**: 実装者
**期間**: 1日

**実装内容**:
1. `verifyFileWithFallback` に `context` パラメータを追加
2. dry-run モードでの warn-only 処理を実装
3. `RecordSuccess` / `RecordFailure` の呼び出しを追加
4. ログレベルに基づいたログ出力を実装

**検証方法**:
```bash
go test -tags test -v ./internal/verification -run TestManager_VerifyFileWithFallback
```

**成果物チェックリスト**:
- [x] `context` パラメータが追加されている
- [x] dry-run モードで常に `nil` を返す
- [x] 検証成功時に `RecordSuccess` が呼ばれる
- [x] 検証失敗時に `RecordFailure` が呼ばれ、適切なログが出力される
- [x] 通常モードの動作が変わっていない（回帰確認）

---

#### Task 2.4: 各検証メソッドの更新

**優先度**: 高
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `readAndVerifyFileWithFallback` に `context` パラメータを追加
2. `VerifyAndReadConfigFile` で `context: "config"` を渡す
3. `VerifyGlobalFiles` で `context: "global"` を渡す
4. `VerifyGroupFiles` で `context: "group:<name>"` を渡す
5. `VerifyEnvFile` で `context: "env"` を渡す
6. スキップ処理で `RecordSkip` を呼び出す

**検証方法**:
```bash
go test -tags test -v ./internal/verification
```

**成果物チェックリスト**:
- [x] すべての検証メソッドが `context` を渡している
- [x] スキップ処理で `RecordSkip` が呼ばれる
- [x] テストが成功
- [x] 既存テストの回帰がない

---

#### Task 2.5: 統合テストの実装

**優先度**: 中
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `TestVerifyGlobalFiles_DryRun_MixedResults`
2. `TestVerifyGroupFiles_DryRun_HashMismatch`
3. `TestVerifyConfigFile_DryRun_HashFileNotFound`

**検証方法**:
```bash
go test -tags test -v ./internal/verification -run TestVerify.*DryRun
```

**成果物チェックリスト**:
- [x] すべてのテストが成功
- [x] 混合結果（成功/失敗/スキップ）のシナリオが動作
- [x] Hash Mismatch が ERROR ログを出力

---

### 3.3 Phase 3: DryRunResult and Formatter Extension

#### Task 3.1: DryRunResult の拡張

**優先度**: 高
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `internal/runner/resource/types.go` に `FileVerification` フィールドを追加
2. `ExecutionStatus` から `StatusPartial` を削除（未使用のため）

**検証方法**:
```bash
go build ./internal/runner/resource
```

**成果物チェックリスト**:
- [x] `FileVerification *FileVerificationSummary` フィールドが追加されている
- [x] JSON タグが `"file_verification,omitempty"` に設定されている
- [x] ビルドエラーがない

---

#### Task 3.2: TextFormatter の拡張

**優先度**: 高
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `FormatResult` メソッドに File Verification セクションを追加
2. `writeFileVerification` メソッドを実装
3. `formatReason` ヘルパー関数を実装

**検証方法**:
```bash
go test -tags test -v ./internal/runner/resource -run TestTextFormatter
```

**成果物チェックリスト**:
- [x] File Verification セクションが Summary の後に表示される
- [x] Hash Directory Status が表示される
- [x] Failures が適切にフォーマットされる（INFO/WARN/ERROR マーカー）
- [x] DetailLevel に応じて詳細度が変わる

---

#### Task 3.3: JSONFormatter の確認

**優先度**: 低
**担当**: 実装者
**期間**: 0.25日

**実装内容**:
1. JSON 出力に `file_verification` フィールドが含まれることを確認
2. 自動シリアライゼーションが正しく動作することを確認

**検証方法**:
```bash
go test -tags test -v ./internal/runner/resource -run TestJSONFormatter
```

**成果物チェックリスト**:
- [x] JSON 出力に `file_verification` が含まれる
- [x] フィールドがすべて正しくシリアライズされる

---

#### Task 3.4: フォーマッタテストの実装

**優先度**: 中
**担当**: 実装者
**期間**: 0.25日

**実装内容**:
1. `TestTextFormatter_FormatResult_WithFileVerification`
2. `TestTextFormatter_WriteFileVerification_AllSuccess`
3. `TestTextFormatter_WriteFileVerification_WithFailures`
4. `TestJSONFormatter_FormatResult_WithFileVerification`

**検証方法**:
```bash
go test -tags test -v ./internal/runner/resource
```

**成果物チェックリスト**:
- [x] すべてのテストが成功
- [x] 出力フォーマットが仕様通り

---

### 3.4 Phase 4: E2E Testing and Documentation

#### Task 4.1: E2E テストスクリプトの実装

**優先度**: 高
**担当**: 実装者
**期間**: 1日

**実装内容**:
1. `cmd/runner/integration_dryrun_verification_test.go` を作成
2. 以下のテストケースを実装:
   - `TestDryRunE2E_HashDirectoryNotFound`
   - `TestDryRunE2E_HashFilesNotFound`
   - `TestDryRunE2E_HashMismatch`
   - `TestDryRunE2E_AllSuccess`
   - `TestDryRunE2E_MixedResults`

**検証方法**:
```bash
go test -tags test -v ./cmd/runner -run TestDryRunE2E
```

**成果物チェックリスト**:
- [ ] すべての E2E テストが成功
- [ ] exit code が常に 0（dry-run モード）
- [ ] JSON 出力が仕様通り
- [ ] TEXT 出力が仕様通り

---

#### Task 4.2: パフォーマンステストの実装と測定

**優先度**: 中
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `BenchmarkResultCollector_RecordSuccess`
2. `BenchmarkResultCollector_RecordFailure`
3. `BenchmarkVerifyFileWithFallback_DryRun`
4. 検証オーバーヘッドの測定スクリプト

**検証方法**:
```bash
go test -tags test -bench=. -benchmem ./internal/verification
```

**成果物チェックリスト**:
- [ ] `RecordSuccess` < 1000 ns/op
- [ ] `RecordFailure` < 2000 ns/op
- [ ] 検証オーバーヘッド < 30%
- [ ] メモリ使用量 < 1 MB

---

#### Task 4.3: 既存テストの回帰確認

**優先度**: 高
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. すべての既存テストを実行
2. 特に dry-run 関連のテストを重点確認
3. 失敗したテストの修正

**検証方法**:
```bash
make test
go test -tags test -v ./... -run DryRun
```

**成果物チェックリスト**:
- [ ] すべてのテストが成功
- [ ] 新規実装によるテスト失敗がない
- [ ] dry-run モードの副作用抑制が維持されている

---

#### Task 4.4: ドキュメント更新

**優先度**: 中
**担当**: 実装者
**期間**: 0.5日

**実装内容**:
1. `CHANGELOG.md` の更新
2. ユーザーガイドの更新（該当する場合）
3. コード内コメントの追加/修正

**検証方法**:
- ドキュメントのレビュー

**成果物チェックリスト**:
- [ ] `CHANGELOG.md` に新機能が記載されている
- [ ] ユーザーガイドが最新の動作を反映
- [ ] コード内コメントが英語で記述されている

---

## 4. リスク管理

### 4.1 技術リスク

| リスク | 影響度 | 発生確率 | 対策 |
|-------|-------|---------|------|
| File Validator の並行アクセスに関するバグ | 高 | 中 | `sync.Mutex` の使用、race detector でのテスト |
| 既存の dry-run テストの失敗 | 高 | 低 | Phase 4 で回帰テストを実施 |
| パフォーマンス要件未達 | 中 | 低 | ベンチマークテストで早期検出、最適化 |
| Hash Directory 不在時の処理漏れ | 中 | 中 | E2E テストで全パターンをカバー |

### 4.2 スケジュールリスク

| リスク | 影響度 | 発生確率 | 対策 |
|-------|-------|---------|------|
| Phase 2 の実装が予定より遅延 | 中 | 中 | Phase 1 を早期完了し、バッファを確保 |
| E2E テストの環境構築に時間がかかる | 低 | 低 | 既存の統合テスト環境を活用 |

## 5. レビュー基準

### 5.1 コードレビューチェックリスト

各 Phase 完了時に以下を確認:

**機能性**:
- [ ] 仕様書通りに実装されている
- [ ] すべてのエッジケースが処理されている
- [ ] エラーハンドリングが適切

**品質**:
- [ ] コードカバレッジ > 90%
- [ ] すべてのテストが成功
- [ ] Race detector でエラーがない
- [ ] Lint エラーがない

**副作用抑制**:
- [ ] dry-run モードでファイル書き込みが発生しない
- [ ] dry-run モードでネットワーク通信が発生しない
- [ ] 検証失敗時も exit code が 0

**パフォーマンス**:
- [ ] ベンチマークテストが性能要件を満たす
- [ ] メモリリークがない

**ドキュメント**:
- [ ] コード内コメントが英語で記述されている
- [ ] 複雑なロジックに説明コメントがある
- [ ] 公開 API に godoc コメントがある

### 5.2 フェーズゲートクライテリア

各フェーズを次フェーズに進める前に確認:

**Phase 1 → Phase 2**:
- [ ] `ResultCollector` のすべてのテストが成功
- [ ] 並行アクセステストが成功
- [ ] ベンチマークテストが性能要件を満たす

**Phase 2 → Phase 3**:
- [ ] `NewManagerForDryRun` が File Validator を有効化
- [ ] warn-only モードが正しく動作
- [ ] 統合テストが成功
- [ ] 既存の通常モードテストが成功（回帰確認）

**Phase 3 → Phase 4**:
- [ ] TEXT/JSON フォーマッタが検証サマリーを表示
- [ ] フォーマッタテストが成功
- [ ] `DryRunResult` の構造が正しい

**Phase 4 完了基準**:
- [ ] すべての E2E テストが成功
- [ ] パフォーマンス要件を満たす
- [ ] すべての既存テストが成功（回帰なし）
- [ ] ドキュメントが更新されている

## 6. テスト戦略

### 6.1 テストピラミッド

```
        E2E Tests (5-10 tests)
       /                      \
      /  Integration Tests     \
     /    (10-20 tests)         \
    /                            \
   /  Unit Tests (50-100 tests)  \
  /________________________________\
```

### 6.2 テスト分類

| テスト種別 | 目的 | カバレッジ目標 |
|----------|------|--------------|
| ユニットテスト | 個別コンポーネントの動作確認 | > 90% |
| 統合テスト | コンポーネント間の連携確認 | 主要フロー 100% |
| E2E テスト | 実運用シナリオの動作確認 | 全シナリオ |
| パフォーマンステスト | 性能要件の確認 | 全主要パス |
| 回帰テスト | 既存機能への影響確認 | 既存テスト 100% |

### 6.3 副作用抑制の検証戦略

各 Phase で以下を確認:

1. **ファイルシステム監視**:
   ```bash
   # Before test
   ls -lR /usr/local/etc/go-safe-cmd-runner/ > before.txt

   # Run dry-run test
   go test -tags test -v ./cmd/runner -run TestDryRun

   # After test
   ls -lR /usr/local/etc/go-safe-cmd-runner/ > after.txt

   # Compare
   diff before.txt after.txt  # Should be empty
   ```

2. **ネットワーク監視**:
   - テスト実行中にネットワーク通信が発生していないことを確認
   - モックを使用した検証

3. **Exit Code 確認**:
   ```bash
   go test -tags test -v ./cmd/runner -run TestDryRun
   echo $?  # Should be 0
   ```

## 7. 実装スケジュール

### 7.1 ガントチャート

```
Phase 1: Result Collector Implementation [Day 1-2]
├── Task 1.1: Data Structures          [Day 1: 0-4h]
├── Task 1.2: ResultCollector          [Day 1: 4-8h]
├── Task 1.3: Helper Functions         [Day 2: 0-4h]
└── Task 1.4: Unit Tests               [Day 2: 4-8h]

Phase 2: Verification Manager Extension [Day 3-5]
├── Task 2.1: Manager Extension        [Day 3: 0-4h]
├── Task 2.2: NewManagerForDryRun      [Day 3: 4-8h]
├── Task 2.3: verifyFileWithFallback   [Day 4: 0-8h]
├── Task 2.4: Update Verify Methods    [Day 5: 0-4h]
└── Task 2.5: Integration Tests        [Day 5: 4-8h]

Phase 3: DryRunResult and Formatter [Day 6-7]
├── Task 3.1: DryRunResult Extension   [Day 6: 0-4h]
├── Task 3.2: TextFormatter            [Day 6: 4-8h]
├── Task 3.3: JSONFormatter            [Day 7: 0-2h]
└── Task 3.4: Formatter Tests          [Day 7: 2-8h]

Phase 4: E2E Testing and Documentation [Day 8-9]
├── Task 4.1: E2E Tests                [Day 8: 0-8h]
├── Task 4.2: Performance Tests        [Day 9: 0-4h]
├── Task 4.3: Regression Tests         [Day 9: 4-6h]
└── Task 4.4: Documentation            [Day 9: 6-8h]
```

### 7.2 マイルストーン

| マイルストーン | 完了予定 | 完了条件 |
|--------------|---------|---------|
| M1: Result Collector 完成 | Day 2 | Phase 1 完了基準を満たす |
| M2: Verification Manager 完成 | Day 5 | Phase 2 完了基準を満たす |
| M3: Formatter 統合完成 | Day 7 | Phase 3 完了基準を満たす |
| M4: リリース準備完了 | Day 9 | Phase 4 完了基準を満たす |

## 8. 開発環境セットアップ

### 8.1 必要なツール

```bash
# Go version
go version  # Should be >= 1.21

# Linter
golangci-lint --version

# Test coverage tools
go install github.com/axw/gocov/gocov@latest
go install github.com/AlekSi/gocov-xml@latest

# Race detector (built-in)
# Benchmark tools (built-in)
```

### 8.2 開発コマンド

```bash
# Build
make build

# Run all tests
make test

# Run tests with race detector
go test -tags test -race -v ./...

# Run specific phase tests
go test -tags test -v ./internal/verification
go test -tags test -v ./internal/runner/resource
go test -tags test -v ./cmd/runner -run TestDryRun

# Run benchmarks
go test -tags test -bench=. -benchmem ./internal/verification

# Check coverage
go test -tags test -cover ./...

# Lint
make lint

# Format
make fmt
```

## 9. まとめ

本実装計画書は、dry-run モードでのファイル検証機能を9日間で完成させるためのロードマップを提供する。

**成功の鍵**:

1. **段階的実装**: 各フェーズで動作するコードを提供し、継続的に統合
2. **テスト駆動**: テストファーストで品質を担保
3. **副作用抑制の保証**: 各フェーズで dry-run の原則を確認
4. **レビュー基準の遵守**: フェーズゲートで品質を担保

**期待される成果**:

- 開発環境と本番環境の両方で動作する堅牢な実装
- 90% 以上のコードカバレッジ
- パフォーマンス要件を満たす実装
- 既存機能への影響なし（回帰テスト 100% 成功）

本計画に従って実装することで、要件定義書と詳細仕様書で定義された機能を確実に実現できる。
