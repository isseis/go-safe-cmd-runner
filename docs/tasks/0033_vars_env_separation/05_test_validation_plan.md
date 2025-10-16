# Task 0033: vars-env Separation - Test Validation Plan (実行計画書)

## 概要

本ドキュメントは、`${VAR}` 構文削除に伴うテスト変更の妥当性を検証するための**実行計画書**です。各作業項目にチェックボックスを設け、進捗を追跡します。

## 実行状況の記録方法

- `[ ]`: 未実施
- `[x]`: 完了
- `[-]`: スキップ(理由を記載)

---

## ✅ 実行完了サマリー

**実行日**: 2025-10-15
**ステータス**: ✅ 全Phase完了
**所要時間**: 約2-3時間

### 成果物
1. ✅ `test_inventory.md` - 完全なテストインベントリ(85テスト/ベンチマーク分析)
2. ✅ `test_validation_summary.md` - 包括的なサマリーレポート
3. ✅ `test_recommendations.md` - 詳細な推奨事項と実装ガイド

### 主要な発見
- 🔴 **3つのCRITICALなギャップ**を特定:
  1. Allowlist enforcement tests (5テスト)
  2. Security integration tests (3テスト)
  3. Environment priority E2E tests (4テスト)
- ⚠️ 追加で2つのHIGH priorityギャップを特定
- ✅ 新しいテストスイートは全体的に良く設計されている
- ⚠️ セキュリティ関連テストの追加が必要

### 推奨アクション
**PR承認前に必須**: Critical tests の実装(10-13時間の見積もり)

---

## Phase 1: テストインベントリの作成

### Step 1.1: 削除されたテストファイルの抽出

**目的**: 削除された全てのテスト関数をリストアップする

#### 対象ファイル
- [ ] `allowlist_violation_test.go` (削除) - allowlist検証のunit tests
- [ ] `expansion_auto_env_test.go` (削除) - 自動環境変数展開のunit tests
- [ ] `expansion_bench_test.go` (削除) - 展開パフォーマンスのbenchmark tests
- [ ] `expansion_benchmark_test.go` (削除) - 追加のbenchmark tests
- [ ] `expansion_cmdenv_test.go` (削除) - コマンド環境変数展開のunit tests
- [ ] `expansion_self_ref_test.go` (削除) - 自己参照シナリオのunit tests

#### 成果物
- [ ] `test_inventory.md` を作成
  - 各テスト関数の情報を以下の形式でリスト化:
    - ファイル名
    - テスト関数名
    - テストタイプ (Unit/Integration/E2E/Benchmark)
    - 変更タイプ (Deleted/Modified)
    - 元の行範囲
    - 簡単な説明

**出力フォーマット**:
```markdown
| File | Test Function | Test Type | Change Type | Original Lines | Description |
|------|---------------|-----------|-------------|----------------|-------------|
```

### Step 1.2: 修正されたテストファイルの抽出

**目的**: 修正または削除されたテスト関数をリストアップする

#### 対象ファイル（コミット済み）
- [ ] `expansion_test.go` (修正) - コア展開unit tests
- [ ] `loader_test.go` (修正) - config loader unit/integration tests
- [ ] `loader_e2e_test.go` (修正) - config loadingのE2E tests
- [ ] `config_test.go` (修正) - bootstrap config tests
- [ ] `runner_test.go` (修正) - runner integration/E2E tests
- [ ] `loader_compatibility_test.go` (修正)
- [ ] `autoenv_test.go` (修正) - environment package
- [ ] `integration_test.go` (修正) - environment package
- [ ] `manager_test.go` (修正) - environment package
- [ ] `processor_test.go` (修正) - environment package
- [ ] `environment_test.go` (修正) - executor package
- [ ] `manager_test.go` (修正) - verification package
- [ ] `path_resolver_test.go` (修正) - verification package
- [ ] `bootstrap/config_test.go` (修正)
- [ ] `config/expansion_test.go` (修正)
- [ ] `config/loader_test.go` (修正)
- [ ] `config/loader_e2e_test.go` (修正)
- [ ] `runner/runner_test.go` (修正)

#### 成果物
- [ ] `test_inventory.md` に修正情報を追加
  - 修正詳細カラムを追加
  - インパクトレベル (High/Medium/Low) を追加

**修正の判断基準**:
- テスト関数全体が削除された
- アサーションが大幅に変更された
- テストのセットアップ/ティアダウンロジックが変更された
- テストデータやフィクスチャが大幅に変更された
- 期待値が変更された（エラー期待の削除など）

**インパクトレベル**:
- **High**: コア機能がテストされなくなった
- **Medium**: テストカバレッジが減少したが部分的にカバーされている
- **Low**: 軽微な変更、カバレッジは本質的に維持されている

---

## Phase 2: 元のテスト目的の分析

### Step 2.1: 削除されたテストの目的分析

**目的**: 各削除されたテストが何をテストしていたかを分析する

#### 作業内容
- [ ] git履歴からテストコードを取得 (`git show ddf76a41f7c0e3700d97e9bcef8908cafa4e4ccf~:path/to/file`)
- [ ] 各テストについて以下を特定:
  - テスト対象の関数/メソッド
  - 入力条件とエッジケース
  - 期待される出力/挙動
  - 使用されていた構文 (`${VAR}` vs `%{VAR}`)
  - 単一ユニットの機能を検証しているか

#### 成果物
- [ ] `test_inventory.md` に以下のカラムを追加:
  - Test Purpose (テスト目的)
  - Tested Feature (テスト対象機能)
  - Test Level (Unit/Integration/E2E)
  - ${VAR} Specific? (Yes/No)
  - Critical Path? (Yes/No)

**Critical Path 判定基準**:
- セキュリティ関連（allowlist、権限昇格、redaction）
- コア機能（変数展開、config読み込み）
- 重要なシナリオのエラーハンドリング
- データ整合性/検証

### Step 2.2: テストのカテゴリ分類

**目的**: テストをカテゴリごとに分類する

#### カテゴリ定義

**カテゴリA: ${VAR}専用テスト**
- [ ] `${VAR}` 構文のパース検証
- [ ] `${VAR}` の非推奨警告チェック
- [ ] `${VAR}` 専用のエラーメッセージ
- [ ] `${VAR}` 参照専用のallowlist違反

**期待されるアクション**: `%{VAR}` に同等機能がない場合は削除が正当

**カテゴリB: 共通機能テスト（重要）**
- [ ] 変数展開ロジック（構文に依存しない）
- [ ] allowlist強制（変数参照全般）
- [ ] 循環参照検出
- [ ] セキュリティ/redaction挙動
- [ ] Config検証ロジック

**期待されるアクション**: `%{VAR}` の同等テストが**必須**

**カテゴリC: 統合/システムテスト**
- [ ] 環境変数の優先順位と継承
- [ ] 変数を含むConfigファイルの読み込み
- [ ] 変数置換を伴うコマンド実行
- [ ] コンポーネント間のデータフロー

**期待されるアクション**: `%{VAR}` を使用するように更新、またはカバレッジが維持されていることを確認

**カテゴリD: その他**
- [ ] ベンチマーク（統合される可能性あり）
- [ ] 重複テスト
- [ ] 非推奨エッジケースのテスト

**期待されるアクション**: ケースバイケースで正当化が必要

#### 成果物
- [ ] `test_inventory.md` にカテゴリ情報を追加
- [ ] 各カテゴリごとのテストレベル内訳（Unit/Integration/E2E）を記録

---

## Phase 3: 新システムのテストカバレッジ分析

### Step 3.1: テストレベル別の同等テスト検索

**目的**: カテゴリBとCの各テストについて、新システムでの同等テストを検索する

#### Unit Testカバレッジ分析
- [ ] 削除された各unit testについて:
  - [ ] テスト対象の関数/メソッドを特定
  - [ ] `%{VAR}` を使った同等のunit testを検索
  - [ ] すべての入力条件とエッジケースがカバーされているか確認
  - [ ] アサーションの完全性を検証

**Unit Test検索戦略**:
- テスト対象の関数名で検索 (e.g., `expandVariables`, `validateAllowlist`)
- 類似のテスト名パターンで検索 (e.g., `TestExpansion*`, `TestValidate*`)
- 同じテストファイル内を確認（ファイルが存在する場合）
- 複数のシナリオをカバーする統合テストを確認

**Unit Test同等性判定基準**:
- ✓ 同じ関数がテストされている
- ✓ 同じ入力バリエーションがカバーされている
- ✓ 同じエッジケースがテストされている
- ✓ 同じエラー条件が検証されている
- ✓ 同等のアサーション深度

#### Integration Testカバレッジ分析
- [ ] 削除された各integration testについて:
  - [ ] テスト対象のコンポーネント間相互作用を特定
  - [ ] `%{VAR}` を使った同等のintegration testを検索
  - [ ] コンポーネント間のデータフローが検証されているか確認

**Integration Test検索戦略**:
- 関連するコンポーネント名で検索 (e.g., `loader`, `expander`, `validator`)
- 統合を示すテスト名で検索 (e.g., `TestLoaderExpansion*`)
- 複数パッケージのintegration testファイルを確認
- 同じ統合をカバーする可能性のあるE2Eテストを確認

**Integration Test同等性判定基準**:
- ✓ 同じコンポーネントが関与している
- ✓ 同じデータフローがテストされている
- ✓ 同じ統合ポイントが検証されている
- ✓ 同じコンポーネント間エラーハンドリング

#### E2E Testカバレッジ分析
- [ ] 削除された各E2E testについて:
  - [ ] テスト対象のend-to-endシナリオを特定
  - [ ] `%{VAR}` 構文を使った同等のE2E testを検索
  - [ ] 完全なワークフローが検証されているか確認

**E2E Test検索戦略**:
- シナリオ/ワークフロー名で検索
- `*_e2e_test.go` ファイルを確認
- `integration_test.go` ファイルを確認
- テストカバレッジのためのサンプルTOMLファイルを確認

**E2E Test同等性判定基準**:
- ✓ 同じユーザー向けシナリオ
- ✓ 同じワークフローステップ
- ✓ 同じ成功/失敗条件
- ✓ 同じ環境相互作用

#### 検索場所
- **Unit Tests**: 削除されたテストと同じパッケージの `*_test.go` ファイル
- **Integration Tests**: `integration_test.go`, `loader_test.go`, `config_test.go`
- **E2E Tests**: `*_e2e_test.go`, `main_test.go` (cmdパッケージ内)
- **Test Data**: `testdata/*.toml`, `sample/*.toml`

#### 成果物
- [ ] `test_inventory.md` に以下のカラムを追加:
  - Test Level (Unit/Integration/E2E)
  - Equivalent Test Exists? (Yes/No)
  - New Test Name/Location
  - Coverage Comparison (Full/Partial/None/Superseded)

**Coverage Comparison値**:
- `Full`: 元のテストのすべての側面がカバーされている
- `Partial`: 一部の側面はカバーされているが、エッジケースや条件が不足している
- `None`: 同等のテストが見つからない
- `Superseded`: 新システムのより広範なテストでカバーされている

### Step 3.2: テストレベル別のカバレッジギャップ特定

**目的**: 同等テストが存在しないテストを分析する

#### Unit Test カバレッジギャップ
- [ ] `Coverage Comparison = None` または `Partial` の全unit testsをリスト化
- [ ] 各ギャップについて:
  - [ ] テストされなくなった関数/メソッドを特定
  - [ ] 関数がコードベースに存在するか確認
  - [ ] インパクトを評価: `Critical`, `High`, `Medium`, `Low`
  - [ ] 不足している具体的なテストシナリオを提供

**Unit Test ギャップインパクト基準**:
- **Critical**: セキュリティ関連関数、データ検証、権限処理
- **High**: コアビジネスロジック、重要パスのエラーハンドリング
- **Medium**: 非重要ビジネスロジック、複雑なロジックを持つヘルパー関数
- **Low**: シンプルなユーティリティ関数、フォーマット関数

#### Integration Test カバレッジギャップ
- [ ] `Coverage Comparison = None` または `Partial` の全integration testsをリスト化
- [ ] 各ギャップについて:
  - [ ] テストされなくなったコンポーネント間相互作用を特定
  - [ ] 相互作用がまだ存在するか確認
  - [ ] データフローの重要度に基づいてインパクトを評価
  - [ ] 不足している具体的な統合シナリオを提供

#### E2E Test カバレッジギャップ
- [ ] `Coverage Comparison = None` または `Partial` の全E2E testsをリスト化
- [ ] 各ギャップについて:
  - [ ] テストされなくなったend-to-endワークフローを特定
  - [ ] ワークフローがまだサポートされているか確認
  - [ ] ユーザーへのインパクトを評価
  - [ ] 不足している具体的なE2Eシナリオを提供

#### 機能の検証
- [ ] 各ギャップについて以下を判定:
  1. 新システムでも機能が存在するか?
     - **YES**: テストカバレッジが必要（リスクとしてフラグ）
     - **NO**: 機能削除が意図的であることを確認（要件で確認）
     - **MODIFIED**: 新実装が異なるテストを必要とするか確認

  2. リスク評価:
     - **High Risk**: 重要な機能でテストカバレッジなし
     - **Medium Risk**: 重要な機能だが部分的なカバレッジあり
     - **Low Risk**: 軽微な機能、または他のテストで冗長
     - **No Risk**: 非推奨機能が正しく削除された

#### 成果物
- [ ] サマリーテーブルを作成:
```markdown
| Test Level | Gap Count | Critical Gaps | High Priority Gaps | Medium/Low Priority |
|-----------|-----------|---------------|-------------------|---------------------|
| Unit      |           |               |                   |                     |
| Integration|          |               |                   |                     |
| E2E       |           |               |                   |                     |
```

- [ ] `test_inventory.md` をカバレッジ分析とギャップ評価で更新

---

## Phase 4: 削除の正当性評価

### Step 4.1: テストレベル別の削除正当性評価

**目的**: 同等テストがない各テストについて、削除の正当性を評価する

#### Unit Test削除の正当性
- [ ] 同等テストがない各unit testについて:
  - **Justified - Deprecated Feature**: `${VAR}` のパース/構文のみを検証
  - **Justified - Redundant**: `%{VAR}` を使った他のunit testで同じ関数がテストされている
  - **Justified - Feature Removed**: 関数が存在しない（意図的に削除）
  - **Justified - Consolidated**: 複数のunit testが包括的なテストにマージされた
  - **Needs Review - Missing Coverage**: 関数は存在するがunit testカバレッジがない
  - **Needs Review - Partial Coverage**: 関数はテストされているがエッジケースが不足
  - **Needs Review - Unclear**: 関数の目的またはテストの必要性が不明確

**Unit Test正当化基準**:
- 関数が `%{VAR}` のみを使用し、同等のunit testがある → Justified
- 関数がコードベースから削除された → Justified（意図的か確認）
- 関数は存在するがunit testがない → **Needs Review** (HIGH PRIORITY)
- 重要な関数（セキュリティ、検証）→ **Needs Review** (CRITICAL)

#### Integration Test削除の正当性
- [ ] 同等テストがない各integration testについて:
  - **Justified - ${VAR} Integration**: `${VAR}` 専用のコンポーネント間相互作用を検証
  - **Justified - Covered by E2E**: より広範なE2E testで統合がテストされている
  - **Justified - Components Removed**: 相互作用が存在しない
  - **Needs Review - Missing Integration Test**: 相互作用は存在するがテストされていない
  - **Needs Review - Data Flow Untested**: 重要なデータフローが検証されていない

**Integration Test正当化基準**:
- 相互作用が `${VAR}` システムのみに関与 → Justified
- 相互作用が `%{VAR}` を使ったE2E testでカバーされている → Justified（E2E testの存在を確認）
- 相互作用は存在するがテストがない → **Needs Review** (MEDIUM-HIGH PRIORITY)
- セキュリティ重要な相互作用 → **Needs Review** (CRITICAL)

#### E2E Test削除の正当性
- [ ] 同等テストがない各E2E testについて:
  - **Justified - ${VAR} Scenario**: `${VAR}` 専用のend-to-endシナリオ
  - **Justified - Feature Removed**: ワークフローがサポートされなくなった（意図的）
  - **Needs Review - Missing E2E Coverage**: ユーザーワークフローがテストされていない
  - **Needs Review - Critical Path Untested**: 重要なユーザーシナリオが検証されていない

**E2E Test正当化基準**:
- シナリオが `${VAR}` ワークフローのみをテストしていた → Justified
- ワークフローが製品から削除された → Justified（要件で確認）
- ユーザー向けワークフローは存在するがE2E testがない → **Needs Review** (HIGH PRIORITY)
- セキュリティまたはデータ整合性ワークフロー → **Needs Review** (CRITICAL)

#### 理由付けテンプレート
- [ ] 各「Needs Review」ケースについて以下を提供:
  - **What was tested**: 元のテストの簡単な説明
  - **Why missing**: 同等テストが存在しない理由
  - **Risk**: 機能が壊れた場合の影響
  - **Recommendation**: テストを追加すべきか? それとも本当に不要か?

#### 必要なアクション評価
- **CRITICAL**: 直ちにテストを追加（セキュリティ、データ整合性）
- **HIGH**: このPRでテストを追加（コア機能）
- **MEDIUM**: 近いうちにテストを追加（重要だが非クリティカル）
- **LOW**: 追加を検討（あるといい程度）
- **NONE**: アクション不要（正当な削除）

#### 成果物
- [ ] `test_inventory.md` に以下のカラムを追加:
  - Justification (Justified/Needs Review)
  - Reason
  - Risk Level (Critical/High/Medium/Low/None)
  - Action Required (CRITICAL/HIGH/MEDIUM/LOW/NONE)

---

## Phase 5: レポート生成

### Step 5.1: テストレベル別サマリーレポート生成

**目的**: テストレベル統計を含む包括的なサマリーレポートを作成する

#### レポート構造

**1. エグゼクティブサマリー**
- [ ] 分析したテストの合計（削除 + 修正）
- [ ] 全体的な正当性評価
- [ ] 直ちにアクションが必要な重要な発見
- [ ] ハイレベルな推奨事項

**2. テストレベル別統計**
- [ ] 統計テーブルを作成:
```markdown
| Test Level    | Total Analyzed | Deleted | Modified | Justified | Needs Review |
|---------------|----------------|---------|----------|-----------|--------------|
| Unit          |                |         |          |           |              |
| Integration   |                |         |          |           |              |
| E2E           |                |         |          |           |              |
| Benchmark     |                |         |          |           |              |
| **TOTAL**     |                |         |          |           |              |
```

**3. カテゴリとレベル別の内訳**
- [ ] カテゴリ別テーブルを作成:
```markdown
| Category | Unit Tests | Integration Tests | E2E Tests | Total | Justified | Needs Review |
|----------|-----------|------------------|-----------|-------|-----------|--------------|
| A (${VAR} specific)    |     |      |     |       |           |              |
| B (Common features)     |     |      |     |       |           |              |
| C (Integration/System)  |     |      |     |       |           |              |
| D (Other)               |     |      |     |       |           |              |
```

**4. レベル別カバレッジギャップ分析**
- [ ] **Unit Test Gaps**:
  - unit testカバレッジがない関数: カウントとリスト
  - テストがない重要な関数: リスク評価付きリスト
  - 部分的なカバレッジケース: 不足しているシナリオ付きリスト

- [ ] **Integration Test Gaps**:
  - テストされていないコンポーネント間相互作用: カウントとリスト
  - テストがない重要なデータフロー: リスク評価付きリスト
  - 部分的な統合カバレッジ: 不足しているシナリオ付きリスト

- [ ] **E2E Test Gaps**:
  - テストされていないユーザーワークフロー: カウントとリスト
  - カバレッジがない重要なシナリオ: リスク評価付きリスト
  - 部分的なE2Eカバレッジ: 不足しているシナリオ付きリスト

**5. リスク評価サマリー**
- [ ] リスクサマリーテーブルを作成:
```markdown
| Risk Level | Count | Test Levels Affected | Primary Concerns |
|-----------|-------|---------------------|------------------|
| Critical  |       | Unit/Integration/E2E |                  |
| High      |       | Unit/Integration/E2E |                  |
| Medium    |       | Unit/Integration/E2E |                  |
| Low       |       | Unit/Integration/E2E |                  |
```

**6. 優先度別アクションアイテム**
- [ ] **CRITICAL**: 直ちに追加すべきテスト（テストレベル表示）
- [ ] **HIGH**: このPRで追加すべきテスト（テストレベル表示）
- [ ] **MEDIUM**: 近いうちに追加すべきテスト（テストレベル表示）
- [ ] **LOW**: 検討すべきテスト（テストレベル表示）

#### 成果物
- [ ] `test_validation_summary.md` を作成（チャートとテーブル付きのMarkdownドキュメント）

### Step 5.2: テストレベル別詳細推奨事項の生成

**目的**: ギャップが見つかった場合、詳細な推奨事項を作成する

#### 推奨ドキュメント構造

**1. Unit Test推奨事項**
- [ ] 不足している各unit testについて:
  - **Function**: 名前とパッケージ
  - **Original Test**: 削除されたもの
  - **Gap**: 現在カバーされていないもの
  - **Recommendation**: 追加すべき具体的なテスト
  - **Priority**: CRITICAL/HIGH/MEDIUM/LOW
  - **Estimated Effort**: 人時
  - **Sample Test Outline**: 擬似コードまたは構造

**2. Integration Test推奨事項**
- [ ] 不足している各integration testについて:
  - **Components**: 関与するコンポーネント
  - **Original Test**: 削除されたもの
  - **Gap**: テストされていない相互作用
  - **Recommendation**: 追加すべき具体的な統合テスト
  - **Priority**: CRITICAL/HIGH/MEDIUM/LOW
  - **Estimated Effort**: 人時
  - **Sample Test Outline**: 擬似コードまたは構造

**3. E2E Test推奨事項**
- [ ] 不足している各E2E testについて:
  - **Scenario**: ユーザー向けワークフロー
  - **Original Test**: 削除されたもの
  - **Gap**: テストされていないend-to-endフロー
  - **Recommendation**: 追加すべき具体的なE2E test
  - **Priority**: CRITICAL/HIGH/MEDIUM/LOW
  - **Estimated Effort**: 人時
  - **Sample Test Outline**: 擬似コードまたは構造

**4. 実装優先度マトリクス**
- [ ] 優先度マトリクスを作成:
```markdown
| Priority | Test Level | Count | Est. Total Effort | Should Block PR? |
|----------|-----------|-------|------------------|-----------------|
| Critical | Unit      |       |                  | YES             |
| Critical | Integration|      |                  | YES             |
| Critical | E2E       |       |                  | YES             |
| High     | Unit      |       |                  | MAYBE           |
| High     | Integration|      |                  | MAYBE           |
| High     | E2E       |       |                  | MAYBE           |
| Medium/Low| All      |       |                  | NO              |
```

**5. テスト実装ガイド**
- [ ] 新しい `%{VAR}` テストのベストプラクティス
- [ ] 新システムでの一般的なテストパターン
- [ ] テストデータセットアップの推奨事項
- [ ] アサーション戦略

#### 成果物
- [ ] `test_recommendations.md` を作成（必要な場合）（実行可能なアイテム付きのMarkdownドキュメント）

---

## 検証基準

テストの削除/修正は、そのテストレベルに適した基準を満たす場合に**有効**と見なされます。

### Unit Test検証基準

**有効な削除**:
1. **カテゴリA (${VAR} specific)**: unit testが `${VAR}` のみのパース/構文を検証
   - 例: `TestParseDollarSyntax`, `TestDollarSignDetection`
   - 基準: 同等の `%{VAR}` 機能が存在しない

2. **カテゴリB (Common features)**: `%{VAR}` システムの同等unit testが存在
   - 同じ関数/メソッドをテストする必要がある
   - 同じ入力条件とエッジケースをカバーする必要がある
   - 同じエラーハンドリングを検証する必要がある
   - テストデータは異なっても良いが、同じアサーションが必要

3. **カテゴリC (Integration)**: unit testが統合テストに統合された
   - 機能がより高いレベルでテストされている
   - 統合テストがユニット動作をカバーしていることを確認する必要がある

4. **カテゴリD (Other)**: unit testが重複または冗長だった
   - 他のunit testが同じ関数をカバーしていることを確認する必要がある

**無効な削除**（要レビュー）:
- 関数はまだ存在するがunit testカバレッジがない
- 重要な関数（セキュリティ、検証、データ整合性）でテストがない
- エッジケースまたはエラー条件がテストされなくなった
- テストが削除されたが関数の動作が拡張された（より少ないテストではなく、より多くのテストが必要）

### Integration Test検証基準

**有効な削除**:
1. **カテゴリA (${VAR} specific)**: integration testが `${VAR}` 専用のコンポーネント間相互作用を検証
   - 例: `TestLoaderExpanderDollarSyntaxIntegration`
   - 基準: 非推奨機能にのみ関連する相互作用

2. **カテゴリB (Common features)**: `%{VAR}` システムの同等integration testが存在
   - 同じコンポーネント間相互作用をテストする必要がある
   - 同じデータフローを検証する必要がある
   - 同じコンポーネント間シナリオをカバーする必要がある

3. **カテゴリC (Integration/System)**: integration testがE2E testでカバーされている
   - E2E testが存在し、相互作用をカバーしている必要がある
   - E2E testに十分なアサーションがあることを確認する必要がある

4. **カテゴリD (Other)**: integration testが重複だった

**無効な削除**（要レビュー）:
- コンポーネント間相互作用はまだ存在するがテストされていない
- 重要なデータフロー（セキュリティ、権限、環境）が検証されていない
- 統合ポイントの障害モードがテストされていない
- テストが削除されたがコンポーネントがより複雑になった

### E2E Test検証基準

**有効な削除**:
1. **カテゴリA (${VAR} specific)**: E2E testが `${VAR}` のみのユーザーワークフローを検証
   - 例: `TestE2E_DollarSyntaxInConfigFile`
   - 基準: 非推奨機能専用のワークフロー

2. **カテゴリB (Common features)**: `%{VAR}` システムの同等E2E testが存在
   - 同じユーザー向けシナリオをテストする必要がある
   - 同じワークフローステップを検証する必要がある
   - 同じ成功/失敗条件をカバーする必要がある

3. **カテゴリC (Integration/System)**: E2Eシナリオがサポートされなくなった（意図的）
   - 要件/設計ドキュメントで確認する必要がある
   - リグレッションではないことを確認する必要がある

4. **カテゴリD (Other)**: E2E testが重複または過度に狭かった

**無効な削除**（要レビュー）:
- ユーザーワークフローはまだサポートされているがend-to-endでテストされていない
- クリティカルパス（セキュリティ、データ整合性）でE2Eカバレッジがない
- システムレベルのエラーハンドリングが検証されていない
- テストが削除されたが機能の複雑性が増加した

### レベル横断の検証基準

**セキュリティテスト**（全レベル）:
- 機能がまだ存在する場合、同等のテストが**必須**
- 含まれるもの: allowlist検証、権限処理、redaction、インジェクション防止
- セキュリティテストの場合、「正当な削除」の例外はなし

**コア機能テスト**（全レベル）:
- 機能がまだ存在する場合、同等のテストが**必須**
- 含まれるもの: 変数展開、config読み込み、コマンド実行、検証
- カバレッジが維持されている場合、レベルを統合可能（例: unit → integration）

**エラーハンドリングテスト**（全レベル）:
- 重要なエラーパスについては同等のテストが**推奨**
- いずれかのレベルで包括的なエラーテストが存在する場合、統合可能

**エッジケーステスト**（主にUNITレベル）:
- エッジケースがまだ可能な場合、同等のテストが推奨
- 機能が簡素化され、エッジケースが存在しなくなった場合は省略可能

---

## 成果物

1. **test_inventory.md**: 分析付きの完全なテストインベントリ
   - 場所: `docs/tasks/0033_vars_env_separation/test_inventory.md`

2. **test_validation_summary.md**: サマリーレポート
   - 場所: `docs/tasks/0033_vars_env_separation/test_validation_summary.md`

3. **test_recommendations.md**: 推奨事項（ギャップが見つかった場合）
   - 場所: `docs/tasks/0033_vars_env_separation/test_recommendations.md`

---

## スケジュール見積もり

- Phase 1: 2-3 時間（インベントリ作成）
- Phase 2: 3-4 時間（目的分析）
- Phase 3: 4-5 時間（カバレッジ分析）
- Phase 4: 2-3 時間（正当性評価）
- Phase 5: 1-2 時間（レポート生成）

**合計**: 12-17 時間

---

## 注意事項

- 速度よりも完全性を優先する
- 不明点があればすべて文書化してレビュー用に残す
- セキュリティ関連のテストを分析で優先する
- パターンが現れた場合（例: ファイル内のすべてのテストが正当化される）、パターンを文書化する

---

## 開始前に解決すべき質問

1. ✅ テストファイルのみを含めるべきか、テストデータファイル(`.toml`)も含めるべきか?
   - **回答**: テストの挙動に影響する可能性があるため、テストデータファイルも分析に含める

2. ✅ 「大幅に修正された」テストのしきい値は?
   - **回答**: アサーションまたはテストロジックが本質的に変更されたテスト

3. 残りのテストが実際にパスすることを検証すべきか?
   - **提案**: 必要に応じてPhase 6として実施可能

4. 変更前後のコードカバレッジメトリクスを確認すべきか?
   - **提案**: 分析を補足する可能性があるが、範囲外の可能性あり
