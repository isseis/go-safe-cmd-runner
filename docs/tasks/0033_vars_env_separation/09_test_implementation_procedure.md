# テストカバレッジギャップ実装手順書 (Task 0033)

**目的**: test_recommendations.mdで特定されたテストカバレッジギャップを段階的に実装する

**前提条件**:
- [test_recommendations.md](test_recommendations.md)を読んで各テストの目的を理解している
- ブランチ: `issei/vars-env-separation-16` で作業する
- TDD原則に従い、テストを先に書いて実装は後にする(今回は実装済みなので検証のみ)

---

## Phase 1: クリティカル優先度テスト (10-13時間)

### 1.1 Allowlist強制テスト (3-4時間)

#### ステップ1.1.1: テストファイル作成とテスト1実装 (45分)

**タスク**:
- [ ] 新規ファイル `internal/runner/config/allowlist_test.go` を作成
- [ ] `TestAllowlistViolation_Global` を実装
  - [ ] 5つのテストケースを実装:
    1. 許可された変数参照 - 成功すべき
    2. 禁止された変数参照 - 失敗すべき
    3. 空のallowlistはすべてをブロック
    4. 未定義のシステム変数(許可された名前)
    5. 複数の参照、1つが禁止

**検証**:
```bash
go test -v -run TestAllowlistViolation_Global ./internal/runner/config/
go test -race -run TestAllowlistViolation_Global ./internal/runner/config/
```

**コミット**:
```
test: add allowlist violation tests for global level

Add comprehensive test cases for global-level allowlist enforcement:
- Allowed variable references
- Blocked variable references
- Empty allowlist behavior
- Undefined system variables
- Multiple references with violations

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

#### ステップ1.1.2: テスト2実装 (1時間)

**タスク**:
- [ ] `TestAllowlistViolation_Group` を実装
  - [ ] 5つのテストケースを実装:
    1. globalのallowlistを継承 - 許可
    2. globalのallowlistを継承 - 禁止
    3. globalのallowlistを上書き - 今は許可
    4. globalのallowlistを上書き - 今は禁止
    5. 空のgroupのallowlistはすべてをブロック

**検証**:
```bash
go test -v -run TestAllowlistViolation_Group ./internal/runner/config/
go test -race -run TestAllowlistViolation_Group ./internal/runner/config/
```

**コミット**:
```
test: add allowlist violation tests for group level

Add test cases for group-level allowlist enforcement with inheritance:
- Inheritance from global allowlist
- Override of global allowlist
- Empty group allowlist blocking

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

#### ステップ1.1.3: テスト3実装 (1時間)

**タスク**:
- [ ] `TestAllowlistViolation_VerifyFiles` を実装
  - [ ] 5つのテストケースを実装:
    1. 許可された変数を持つglobalのverify_files
    2. 禁止された変数を持つglobalのverify_files
    3. 継承されたallowlistを持つgroupのverify_files
    4. 上書きされたallowlistを持つgroupのverify_files
    5. 複数のパス、1つに禁止された変数

**検証**:
```bash
go test -v -run TestAllowlistViolation_VerifyFiles ./internal/runner/config/
go test -race -run TestAllowlistViolation_VerifyFiles ./internal/runner/config/
```

**コミット**:
```
test: add allowlist violation tests for verify_files paths

Add test cases for allowlist enforcement in verify_files paths:
- Global and group level verify_files with allowlist
- Inheritance and override scenarios
- Multiple paths with violations

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

#### ステップ1.1.4: テスト4実装 (45分)

**タスク**:
- [ ] `TestAllowlistViolation_ProcessEnv` を実装
  - [ ] 4つのテストケースを実装:
    1. 許可された内部変数を参照するenv
    2. 許可されたシステムenvから来たvarsを参照するenv
    3. システムenvを直接参照しようとするenv
    4. allowlistを尊重する複雑なチェーン

**検証**:
```bash
go test -v -run TestAllowlistViolation_ProcessEnv ./internal/runner/config/
go test -race -run TestAllowlistViolation_ProcessEnv ./internal/runner/config/
```

**コミット**:
```
test: add allowlist tests for env value references

Add test cases for allowlist enforcement when env references variables:
- Internal variable references
- System env variable references
- Complex chaining with allowlist

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

#### ステップ1.1.5: テスト5実装とセクション完了 (30分)

**タスク**:
- [ ] `TestAllowlistViolation_EdgeCases` を実装
  - [ ] エッジケースをテスト:
    - 大文字小文字区別
    - 特殊文字を含むallowlist
    - 長いallowlist
    - allowlistの変更
    - 同じシステム変数への複数参照

**検証**:
```bash
# 全allowlistテストを実行
go test -v ./internal/runner/config/ -run TestAllowlistViolation
go test -race ./internal/runner/config/ -run TestAllowlistViolation

# カバレッジ確認
go test -cover ./internal/runner/config/
```

**コミット**:
```
test: add edge case tests for allowlist violations

Add test cases for allowlist edge cases and complex scenarios:
- Case sensitivity in allowlist matching
- Special characters in allowlist entries
- Large allowlists
- Allowlist changes between levels
- Multiple references to same system variable

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### 1.2 セキュリティ統合テスト (4-5時間)

#### ステップ1.2.1: テストファイル作成とテスト1実装 (2時間)

**タスク**:
- [ ] 新規ファイル `internal/runner/config/security_integration_test.go` を作成
- [ ] `TestSecurityIntegration_E2E` を実装
  - [ ] 4つのテストケースを実装:
    1. Allowlist + Redaction統合
    2. 変数展開 + コマンド実行セキュリティ
    3. from_env + allowlist + vars + envチェーン
    4. 異なるallowlistを持つ複数のグループ

**検証**:
```bash
go test -v -run TestSecurityIntegration_E2E ./internal/runner/config/
go test -race -run TestSecurityIntegration_E2E ./internal/runner/config/
```

**コミット**:
```
test: add end-to-end security integration tests

Add comprehensive E2E tests for security feature integration:
- Allowlist + redaction integration
- Variable expansion + command execution security
- Full chain security (from_env → vars → env)
- Multi-group isolation with different allowlists

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

#### ステップ1.2.2: テスト2実装 (2-3時間)

**タスク**:
- [ ] `TestSecurityAttackPrevention` を実装
  - [ ] 6つの攻撃シナリオをテスト:
    1. 変数を介したコマンドインジェクション
    2. 変数を介したパストラバーサル
    3. Allowlistバイパス試行
    4. 環境変数インジェクション
    5. Redactionバイパス試行
    6. 予約プレフィックス違反

**検証**:
```bash
go test -v -run TestSecurityAttackPrevention ./internal/runner/config/
go test -race -run TestSecurityAttackPrevention ./internal/runner/config/
```

**コミット**:
```
test: add security attack prevention tests

Add tests to verify protection against common attack vectors:
- Command injection via variables
- Path traversal via variables
- Allowlist bypass attempts
- Environment variable injection
- Redaction bypass attempts
- Reserved prefix violations

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

#### ステップ1.2.3: テスト3実装とセクション完了 (1-2時間)

**タスク**:
- [ ] 新規ファイル `internal/runner/runner_security_test.go` を作成
- [ ] `TestRunner_SecurityIntegration` を実装
  - [ ] 3つのテストケースを実装:
    1. セキュリティ機能を持つ完全なconfig
    2. 異なるセキュリティコンテキストを持つ複数のコマンド
    3. ランタイムセキュリティチェック

**検証**:
```bash
# 全セキュリティテストを実行
go test -v ./internal/runner/config/ -run TestSecurity
go test -v ./internal/runner/ -run TestRunner_Security
go test -race ./internal/runner/config/ -run TestSecurity
go test -race ./internal/runner/ -run TestRunner_Security

# カバレッジ確認
go test -cover ./internal/runner/config/
go test -cover ./internal/runner/
```

**コミット**:
```
test: add runner-level security integration tests

Add full-stack security verification at runner level:
- Complete config with security features
- Multiple commands with different security contexts
- Runtime security checks and validation

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### 1.3 環境変数優先順位E2Eテスト (3-4時間)

#### ステップ1.3.1: テスト1実装 (1.5時間)

**タスク**:
- [ ] ファイル `cmd/runner/integration_test.go` に追加(存在しない場合は作成)
- [ ] `TestRunner_EnvironmentVariablePriority_Basic` を実装
  - [ ] 5つのテストケースを実装:
    1. システムenvのみ
    2. Globalがsystemを上書き
    3. Groupがglobalを上書き
    4. Commandがすべてを上書き
    5. 混合優先順位

**検証**:
```bash
go test -v -run TestRunner_EnvironmentVariablePriority_Basic ./cmd/runner/
go test -race -run TestRunner_EnvironmentVariablePriority_Basic ./cmd/runner/
```

**コミット**:
```
test: add E2E tests for environment variable priority

Add integration tests for basic environment variable priority rules:
- System env only
- Global overrides system
- Group overrides global
- Command overrides all
- Mixed priority scenarios

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

#### ステップ1.3.2: テスト2実装 (1時間)

**タスク**:
- [ ] `TestRunner_EnvironmentVariablePriority_WithVars` を実装
  - [ ] 3つのテストケースを実装:
    1. 下位優先度envを参照するvars
    2. commandのvarsがgroupを上書き
    3. 優先順位を尊重する複雑なチェーン

**検証**:
```bash
go test -v -run TestRunner_EnvironmentVariablePriority_WithVars ./cmd/runner/
go test -race -run TestRunner_EnvironmentVariablePriority_WithVars ./cmd/runner/
```

**コミット**:
```
test: add E2E tests for variable priority with vars expansion

Add tests for environment variable priority with vars references:
- Vars referencing lower-priority env
- Command vars overriding group vars
- Complex chains respecting priority

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

#### ステップ1.3.3: テスト3とテスト4実装、セクション完了 (1-1.5時間)

**タスク**:
- [ ] `TestRunner_EnvironmentVariablePriority_EdgeCases` を実装
  - [ ] 6つのエッジケースをテスト:
    1. 異なるレベルでの空値
    2. より高い優先度で未設定
    3. 数値と特殊値
    4. 非常に長い値
    5. 多くの変数
    6. レベル間の循環参照試行
- [ ] `TestRunner_ResolveEnvironmentVars_Integration` を実装

**検証**:
```bash
# 全優先順位テストを実行
go test -v ./cmd/runner/ -run TestRunner_EnvironmentVariablePriority
go test -v ./cmd/runner/ -run TestRunner_ResolveEnvironmentVars_Integration
go test -race ./cmd/runner/ -run TestRunner_Environment

# カバレッジ確認
go test -cover ./cmd/runner/
```

**コミット**:
```
test: add edge case and integration tests for env priority

Add edge case tests for environment variable priority:
- Empty values at different levels
- Unset at higher priority
- Numeric and special values
- Very long values
- Many variables
- Circular reference attempts

Add integration tests for environment variable resolution.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### 1.4 Phase 1完了検証 (30分)

**タスク**:
- [x] 全テストスイートを実行
  ```bash
  go test ./...
  ```
- [x] レース検出器で実行
  ```bash
  go test -race ./...
  ```
- [x] カバレッジレポート生成
  ```bash
  go test -cover ./... | tee coverage_phase1.txt
  ```
- [x] リンター実行
  ```bash
  make lint
  ```
- [x] すべてのテストがパスしていることを確認
- [x] カバレッジが向上していることを確認

**コミット**:
```
test: verify Phase 1 critical tests completion

Verify all Phase 1 critical priority tests:
- Allowlist enforcement tests (5 tests)
- Security integration tests (3 tests)
- Environment variable priority E2E tests (4 tests)

All tests pass with race detector.
Coverage increased for critical security paths.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

## Phase 2: 高優先度テスト (4-6時間) - オプションだが推奨

### 2.1 コマンドenv展開テスト (2-3時間)

#### ステップ2.1.1: テスト実装 (2-3時間)

**タスク**:
- [ ] `internal/runner/config/command_env_expansion_test.go` を作成
- [ ] コマンドレベルでのenv展開テストを実装
  - [ ] 基本的なコマンドenv展開
  - [ ] コマンドenvでのvars参照
  - [ ] コマンドenvでのglobal/group vars参照
  - [ ] コマンドenv展開エラーハンドリング
  - [ ] コマンドenv優先順位

**検証**:
```bash
go test -v ./internal/runner/config/ -run CommandEnv
go test -race ./internal/runner/config/ -run CommandEnv
```

**コミット**:
```
test: add command-level env expansion tests

Add comprehensive tests for command-level env expansion:
- Basic command env expansion
- Command env referencing vars
- Command env referencing global/group vars
- Error handling in command env expansion
- Command env priority verification

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### 2.2 Verify Files展開テスト (2-3時間)

#### ステップ2.2.1: テスト実装 (2-3時間)

**タスク**:
- [ ] `internal/runner/config/verify_files_expansion_test.go` を作成
- [ ] verify_filesパス展開の包括的テストを実装
  - [ ] Global verify_filesでのvars展開
  - [ ] Group verify_filesでのvars展開
  - [ ] 複数の変数参照を含むパス
  - [ ] ネストされた変数参照
  - [ ] パス展開エラーハンドリング
  - [ ] 相対パスと絶対パス
  - [ ] 特殊文字を含むパス
  - [ ] エッジケース(空のパス、非常に長いパスなど)

**検証**:
```bash
go test -v ./internal/runner/config/ -run VerifyFiles
go test -race ./internal/runner/config/ -run VerifyFiles
```

**コミット**:
```
test: add comprehensive verify_files expansion tests

Add extensive tests for verify_files path expansion:
- Global and group level verify_files
- Multiple variable references in paths
- Nested variable references
- Error handling for path expansion
- Relative and absolute paths
- Special characters in paths
- Edge cases (empty paths, very long paths)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### 2.3 Phase 2完了検証 (30分)

**タスク**:
- [ ] 全テストスイートを実行
  ```bash
  go test ./...
  ```
- [ ] レース検出器で実行
  ```bash
  go test -race ./...
  ```
- [ ] カバレッジレポート生成
  ```bash
  go test -cover ./... | tee coverage_phase2.txt
  ```
- [ ] リンター実行
  ```bash
  make lint
  ```

**コミット**:
```
test: verify Phase 2 high-priority tests completion

Verify all Phase 2 high-priority tests:
- Command env expansion tests
- Verify files expansion tests

All tests pass with race detector.
Coverage further increased for critical paths.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

## Phase 3: 中優先度テスト (2-3時間) - オプション

### 3.1 自己参照テスト (2-3時間)

#### ステップ3.1.1: テスト実装 (2-3時間)

**タスク**:
- [ ] `internal/runner/config/self_reference_test.go` を作成
- [ ] 自己参照と循環参照のテストを実装
  - [ ] 直接的な自己参照検出
  - [ ] 循環参照検出
  - [ ] レベル間の循環参照
  - [ ] 複雑な循環パターン

**検証**:
```bash
go test -v ./internal/runner/config/ -run SelfReference
go test -race ./internal/runner/config/ -run SelfReference
```

**コミット**:
```
test: add self-reference and circular reference tests

Add tests for detecting self-references and circular dependencies:
- Direct self-reference detection
- Circular reference detection
- Cross-level circular references
- Complex circular patterns

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

## 最終検証とドキュメント更新

### 最終検証 (1時間)

**タスク**:
- [ ] 完全なテストスイート実行
  ```bash
  go test -v ./...
  go test -race ./...
  go test -cover ./... > final_coverage.txt
  ```
- [ ] リンター実行とすべての警告修正
  ```bash
  make lint
  make fmt
  ```
- [ ] すべてのチェックボックスが完了していることを確認
- [ ] test_recommendations.mdのチェックリストを更新

**最終コミット**:
```
test: complete test coverage gap implementation

Implement all critical and high-priority test coverage gaps:

Phase 1 (Critical):
- Allowlist enforcement tests (5 test functions)
- Security integration tests (3 test functions)
- Environment variable priority E2E tests (4 test functions)

Phase 2 (High - if implemented):
- Command env expansion tests
- Verify files expansion tests

Phase 3 (Medium - if implemented):
- Self-reference tests

All tests pass with race detector.
Test coverage significantly improved for security-critical paths.

Refs: #33

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

## 作業のヒントとベストプラクティス

### テスト作成時の注意点

1. **%{VAR}構文を使用**: 新しい構文を使用する(${VAR}は廃止予定)
2. **t.Setenv()を使用**: システム環境変数のセットアップに使用
3. **errors.Is()を使用**: エラータイプの検証には文字列マッチングではなくerrors.Is()を使用
4. **テーブル駆動テスト**: すべてのテストでテーブル駆動パターンを使用
5. **明確なテスト名**: テストケースの名前は日本語で明確に記述
6. **並行実行**: t.Parallel()を適切に使用(ただし環境変数を使う場合は注意)

### エラーが出た場合

1. **テスト失敗**:
   - テストロジックを見直す
   - 期待値が正しいか確認
   - 実装コードの動作を確認

2. **コンパイルエラー**:
   - インポート文を確認
   - 型定義を確認
   - 関数シグネチャを確認

3. **レースコンディション**:
   - 共有状態を確認
   - 並行実行を見直す
   - t.Parallel()の使用を再考

### 進捗追跡

- 各ステップ完了後にチェックボックスをマークする
- コミットメッセージは明確で一貫性を保つ
- 定期的に`go test ./...`を実行して既存テストが壊れていないか確認

---

## タイムライン概要

| Phase | 内容 | 見積時間 | 優先度 |
|-------|------|----------|--------|
| Phase 1.1 | Allowlist強制テスト | 3-4時間 | 🔴 クリティカル |
| Phase 1.2 | セキュリティ統合テスト | 4-5時間 | 🔴 クリティカル |
| Phase 1.3 | 環境変数優先順位E2Eテスト | 3-4時間 | 🔴 クリティカル |
| Phase 1.4 | Phase 1完了検証 | 0.5時間 | 🔴 クリティカル |
| **Phase 1合計** | **クリティカルテスト** | **10.5-13.5時間** | **🔴 必須** |
| Phase 2.1 | コマンドenv展開テスト | 2-3時間 | 🟡 高 |
| Phase 2.2 | Verify Files展開テスト | 2-3時間 | 🟡 高 |
| Phase 2.3 | Phase 2完了検証 | 0.5時間 | 🟡 高 |
| **Phase 2合計** | **高優先度テスト** | **4.5-6.5時間** | **🟡 推奨** |
| Phase 3.1 | 自己参照テスト | 2-3時間 | 🟢 中 |
| **Phase 3合計** | **中優先度テスト** | **2-3時間** | **🟢 オプション** |
| 最終検証 | 完全検証とドキュメント更新 | 1時間 | 🔴 必須 |
| **総計** | | **18-24時間** | |

---

## チェックリスト: Phase 1 (クリティカル - 必須)

### 1.1 Allowlist強制テスト
- [x] ステップ1.1.1: TestAllowlistViolation_Global (45分)
- [x] ステップ1.1.2: TestAllowlistViolation_Group (1時間)
- [x] ステップ1.1.3: TestAllowlistViolation_VerifyFiles (1時間)
- [x] ステップ1.1.4: TestAllowlistViolation_ProcessEnv (45分)
- [x] ステップ1.1.5: TestAllowlistViolation_EdgeCases (30分)

### 1.2 セキュリティ統合テスト
- [x] ステップ1.2.1: TestSecurityIntegration_E2E (2時間)
- [x] ステップ1.2.2: TestSecurityAttackPrevention (2-3時間)
- [x] ステップ1.2.3: TestRunner_SecurityIntegration (1-2時間)

### 1.3 環境変数優先順位E2Eテスト
- [x] ステップ1.3.1: TestRunner_EnvironmentVariablePriority_Basic (1.5時間)
- [x] ステップ1.3.2: TestRunner_EnvironmentVariablePriority_WithVars (1時間)
- [x] ステップ1.3.3: TestRunner_EnvironmentVariablePriority_EdgeCases + Integration (1-1.5時間)

### 1.4 Phase 1完了検証
- [x] ステップ1.4: 全テスト実行とカバレッジ検証 (30分)

---

## チェックリスト: Phase 2 (高優先度 - 推奨)

- [ ] ステップ2.1.1: コマンドenv展開テスト (2-3時間)
- [ ] ステップ2.2.1: Verify Files展開テスト (2-3時間)
- [ ] ステップ2.3: Phase 2完了検証 (30分)

---

## チェックリスト: Phase 3 (中優先度 - オプション)

- [ ] ステップ3.1.1: 自己参照テスト (2-3時間)

---

## チェックリスト: 最終検証

- [ ] 最終検証: 全テスト実行とドキュメント更新 (1時間)

---

**ドキュメントバージョン**: 1.0
**作成日**: 2025-10-16
**ステータス**: 実装準備完了
**関連ドキュメント**: [test_recommendations.md](test_recommendations.md)
