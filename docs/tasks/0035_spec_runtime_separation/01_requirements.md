# 要件定義書: 構造体分離（Spec/Runtime分離）

## 1. 概要

### 1.1 プロジェクト名

Task 0035: 構造体分離（Spec/Runtime分離）

### 1.2 目的

現在の `runnertypes` パッケージの構造体（`Config`, `GlobalConfig`, `CommandGroup`, `Command`）は、TOML由来のフィールド（immutable）と実行時計算フィールド（mutable）が混在しており、以下の問題を引き起こしています:

- **不変性の保証がない**: シャローコピーに依存し、実装ミスのリスクがある
- **型による安全性の欠如**: 展開前/展開後を型で区別できない
- **テストの複雑さ**: 手動で `ExpandedXxx` フィールドを設定する必要がある
- **責務の曖昧さ**: 「設定」と「実行時状態」が混在

本プロジェクトでは、これらの問題を解決するため、全構造体を **Spec層**（immutable）と **Runtime層**（mutable）に分離します。

### 1.3 スコープ

#### 対象（In Scope）

- ✅ `Config`, `GlobalConfig`, `CommandGroup`, `Command` の分離
- ✅ Spec層の型定義（`ConfigSpec`, `GlobalSpec`, `GroupSpec`, `CommandSpec`）
- ✅ Runtime層の型定義（`RuntimeGlobal`, `RuntimeGroup`, `RuntimeCommand`）
- ✅ 展開関数の実装（`ExpandGlobal`, `ExpandGroup`, `ExpandCommand`）
- ✅ TOMLローダーの更新（Spec型を返すように変更）
- ✅ GroupExecutor、Executorの更新（Runtime型を使用）
- ✅ 全テストコードの更新
- ✅ ドキュメントの更新

#### 対象外（Out of Scope）

- ❌ 新機能の追加（構造変更のみに集中）
- ❌ TOMLファイルフォーマットの変更（後方互換性を維持）
- ❌ パフォーマンスの最適化（構造変更が主目的）

### 1.4 背景

本プロジェクトは、Task 0034「作業ディレクトリ仕様の再設計」の設計レビュー中に発見された構造的問題を解決するために立ち上げられました。

当初、Task 0034 では `expandCommand()` 関数内でシャローコピーを使用して元の `Command` を変更しない設計を採用していました。しかし、この設計には以下の問題がありました:

```go
// 問題のあるコード例
func expandCommand(cmd *Command, vars map[string]string) (*Command, error) {
    expanded := *cmd  // シャローコピー（脆弱）
    expanded.ExpandedCmd = expandString(cmd.Cmd, vars)
    // ...
    return &expanded, nil
}
```

この問題は `Command` に限らず、`Global`, `Group` も同様に抱えています。そのため、システム全体のアーキテクチャとして、**Spec/Runtime分離**を実施することになりました。

---

## 2. 機能要件

### 2.1 Spec層の要件

#### FR-001: ConfigSpec の定義

- **要件**: TOMLファイル全体の構造を表現する `ConfigSpec` 型を定義する
- **フィールド**:
  - `Version string`: 設定ファイルのバージョン
  - `Global GlobalSpec`: グローバル設定
  - `Groups []GroupSpec`: グループ設定のリスト
- **制約**: すべてのフィールドは読み取り専用として扱う

#### FR-002: GlobalSpec の定義

- **要件**: グローバル設定の仕様を表現する `GlobalSpec` 型を定義する
- **フィールド**:
  - 実行制御: `Timeout`, `LogLevel`, `SkipStandardPaths`, `MaxOutputSize`
  - セキュリティ: `VerifyFiles`, `EnvAllowlist`
  - 変数定義: `Env`, `FromEnv`, `Vars`（すべて `[]string` 型、生の値）
- **制約**: TOML由来のフィールドのみを含む（展開済みフィールドは含まない）

#### FR-003: GroupSpec の定義

- **要件**: グループ設定の仕様を表現する `GroupSpec` 型を定義する
- **フィールド**:
  - 基本情報: `Name`, `Description`, `Priority`
  - リソース管理: `WorkDir`
  - コマンド定義: `Commands []CommandSpec`
  - セキュリティ: `VerifyFiles`, `EnvAllowlist`
  - 変数定義: `Env`, `FromEnv`, `Vars`
- **制約**: TOML由来のフィールドのみを含む

#### FR-004: CommandSpec の定義

- **要件**: コマンド設定の仕様を表現する `CommandSpec` 型を定義する
- **フィールド**:
  - 基本情報: `Name`, `Description`
  - コマンド定義: `Cmd`, `Args`
  - 実行設定: `WorkDir`, `Timeout`, `RunAsUser`, `RunAsGroup`, `MaxRiskLevel`, `Output`
  - 変数定義: `Env`, `FromEnv`, `Vars`
- **制約**: TOML由来のフィールドのみを含む

#### FR-005: Spec層の不変性保証

- **要件**: Spec層のすべての型は、生成後に変更されないことを保証する
- **実装方針**:
  - フィールドはエクスポートするが、ドキュメントで「読み取り専用」を明記
  - コンストラクタ、ビルダーパターンによる生成を推奨
  - 変更が必要な場合は新しいインスタンスを生成

### 2.2 Runtime層の要件

#### FR-006: RuntimeGlobal の定義

- **要件**: グローバル設定の実行時展開結果を表現する `RuntimeGlobal` 型を定義する
- **フィールド**:
  - `Spec *GlobalSpec`: 元の仕様への参照
  - `ExpandedVerifyFiles []string`: 展開済み検証ファイルパス
  - `ExpandedEnv map[string]string`: 展開済み環境変数
  - `ExpandedVars map[string]string`: 展開済み内部変数
- **制約**: `Spec` への参照を保持し、元の設定にアクセス可能にする

#### FR-007: RuntimeGroup の定義

- **要件**: グループ設定の実行時展開結果を表現する `RuntimeGroup` 型を定義する
- **フィールド**:
  - `Spec *GroupSpec`: 元の仕様への参照
  - `ExpandedVerifyFiles []string`: 展開済み検証ファイルパス
  - `ExpandedEnv map[string]string`: 展開済み環境変数
  - `ExpandedVars map[string]string`: 展開済み内部変数
  - `EffectiveWorkDir string`: 解決済み作業ディレクトリ
  - `Commands []*RuntimeCommand`: 展開済みコマンドリスト
- **制約**: グループ実行時に一度だけ生成される

#### FR-008: RuntimeCommand の定義

- **要件**: コマンドの実行時展開結果を表現する `RuntimeCommand` 型を定義する
- **フィールド**:
  - `Spec *CommandSpec`: 元の仕様への参照
  - `ExpandedCmd string`: 展開済みコマンドパス
  - `ExpandedArgs []string`: 展開済みコマンド引数
  - `ExpandedEnv map[string]string`: 展開済み環境変数
  - `ExpandedVars map[string]string`: 展開済み内部変数
  - `EffectiveWorkDir string`: 解決済み作業ディレクトリ
  - `EffectiveTimeout int`: 解決済みタイムアウト（Global/Group継承を考慮）
- **制約**: コマンド実行時に一度だけ生成される

#### FR-009: Runtime層の便利メソッド

- **要件**: Runtime層の型に、Spec層へのアクセスを簡略化する便利メソッドを提供する
- **メソッド例**:
  - `RuntimeCommand.Name() string`: `r.Spec.Name` のエイリアス
  - `RuntimeCommand.RunAsUser() string`: `r.Spec.RunAsUser` のエイリアス
  - `RuntimeCommand.GetMaxRiskLevel() (RiskLevel, error)`: `r.Spec.GetMaxRiskLevel()` の委譲
- **目的**: `cmd.Spec.Name` のような冗長なアクセスを `cmd.Name()` に簡略化

### 2.3 展開関数の要件

#### FR-010: ExpandGlobal の実装

- **要件**: `GlobalSpec` を受け取り、`RuntimeGlobal` を返す展開関数を実装する
- **処理内容**:
  1. `FromEnv` の処理（システム環境変数のインポート）
  2. `Vars` の処理（内部変数の定義）
  3. `Env` の展開（内部変数を使用した環境変数の展開）
  4. `VerifyFiles` の展開
- **エラーハンドリング**: 展開失敗時は詳細なエラーメッセージを返す

#### FR-011: ExpandGroup の実装

- **要件**: `GroupSpec` と `globalVars` を受け取り、`RuntimeGroup` を返す展開関数を実装する
- **処理内容**:
  1. グローバル変数の継承
  2. `FromEnv` の処理（グループレベル）
  3. `Vars` の処理（グループレベル）
  4. `Env` の展開
  5. `VerifyFiles` の展開
- **注意**: この時点ではコマンドは展開しない（GroupExecutor内で展開）

#### FR-012: ExpandCommand の実装

- **要件**: `CommandSpec` と `groupVars` を受け取り、`RuntimeCommand` を返す展開関数を実装する
- **処理内容**:
  1. グループ変数の継承
  2. `FromEnv` の処理（コマンドレベル）
  3. `Vars` の処理（コマンドレベル）
  4. `Cmd` の展開
  5. `Args` の展開
  6. `Env` の展開
- **注意**: `EffectiveWorkDir` や `EffectiveTimeout` は GroupExecutor 内で設定される

### 2.4 統合要件

#### FR-013: TOMLローダーの更新

- **要件**: `config.Loader` を更新し、`ConfigSpec` を返すようにする
- **変更内容**:
  - `Load(path string) (*ConfigSpec, error)` に変更
  - パース処理は既存のまま（TOML構造は変わらない）
- **後方互換性**: TOMLファイルフォーマットは変更しない

#### FR-014: GroupExecutor の更新

- **要件**: `GroupExecutor` を更新し、`RuntimeGroup` を使用するようにする
- **変更内容**:
  - `ExecuteGroup(ctx context.Context, groupSpec *GroupSpec) error` に変更
  - 内部で `ExpandGroup()` を呼び出し、`RuntimeGroup` を生成
  - 各コマンドに対して `ExpandCommand()` を呼び出し、`RuntimeCommand` を生成
  - Executor に `RuntimeCommand` を渡す

#### FR-015: Executor の更新

- **要件**: `CommandExecutor` を更新し、`RuntimeCommand` を受け取るようにする
- **変更内容**:
  - `Execute(ctx context.Context, cmd *RuntimeCommand) error` に変更
  - `cmd.ExpandedCmd`, `cmd.ExpandedArgs` などを使用
  - `cmd.Spec.Name`, `cmd.Spec.RunAsUser` などの生の値も参照可能

---

## 3. 非機能要件

### 3.1 パフォーマンス要件

#### NFR-001: 展開処理のパフォーマンス

- **要件**: 既存の実装と比較して、展開処理のパフォーマンスが劣化しないこと
- **許容範囲**: ±10% 以内
- **測定方法**: ベンチマークテスト（既存コードとの比較）

#### NFR-002: メモリ使用量

- **要件**: Spec と Runtime の両方を保持するため、メモリ使用量が増加するが、許容範囲内であること
- **許容範囲**: 従来比 +30% 以内
- **根拠**: Runtime は Spec への参照のみなので、実質的な増加は展開済みフィールドのみ

### 3.2 保守性要件

#### NFR-003: コードの可読性

- **要件**: 分離後のコードは、分離前よりも可読性が向上すること
- **評価基準**:
  - 型名が明確（`Spec` vs `Runtime` の区別が一目で分かる）
  - 責務が明確（設定 vs 実行時状態の分離）
  - コメントが充実

#### NFR-004: テストの容易性

- **要件**: 分離後のコードは、テストが容易であること
- **評価基準**:
  - Spec層のテスト: TOMLパースのみをテスト
  - Runtime層のテスト: 展開ロジックのみをテスト
  - 単体テストのカバレッジ > 80%

### 3.3 互換性要件

#### NFR-005: TOMLフォーマットの互換性

- **要件**: 既存のTOMLファイルがそのまま動作すること
- **制約**: TOMLファイルのフォーマット変更は許可しない
- **検証方法**: 既存のサンプルファイルがすべてロードできることを確認

#### NFR-006: 段階的な移行

- **要件**: 既存のコードから新しいコードへの移行が段階的に行えること
- **対策**: すべての変更を1つのPRにまとめず、レビュー可能な単位に分割

---

## 4. セキュリティ要件

### SR-001: 不変性の保証

- **要件**: Spec層のインスタンスが外部から変更されないことを保証する
- **対策**:
  - ドキュメントで「読み取り専用」を明記
  - テストで不変性を検証
  - レビューで変更がないことを確認

### SR-002: Runtime層のライフサイクル管理

- **要件**: Runtime層のインスタンスが適切に生成・破棄されることを保証する
- **対策**:
  - GroupExecutor 内でのみ生成
  - グループ実行終了後は破棄
  - 並行実行時は独立したインスタンスを使用

---

## 5. テスト要件

### 5.1 単体テスト

#### TR-001: Spec層のテスト

- **要件**: すべてのSpec型（`ConfigSpec`, `GlobalSpec`, `GroupSpec`, `CommandSpec`）のTOMLパーステストを実装する
- **テストケース**:
  - 正常系: 有効なTOMLファイルのパース
  - 異常系: 不正なTOMLファイルのパース失敗
  - エッジケース: 空のフィールド、デフォルト値

#### TR-002: Runtime層のテスト

- **要件**: すべてのRuntime型（`RuntimeGlobal`, `RuntimeGroup`, `RuntimeCommand`）の展開テストを実装する
- **テストケース**:
  - 変数展開の正常系
  - 変数継承の確認
  - 未定義変数のエラーハンドリング

#### TR-003: 展開関数のテスト

- **要件**: すべての展開関数（`ExpandGlobal`, `ExpandGroup`, `ExpandCommand`）のテストを実装する
- **テストケース**:
  - 各展開フェーズの検証
  - エラーケースの検証
  - 複雑な変数展開パターン

### 5.2 統合テスト

#### TR-004: エンドツーエンドテスト

- **要件**: TOMLファイルのロードから、コマンド実行までの一連の流れをテストする
- **テストケース**:
  - 既存のサンプルファイルがすべて動作すること
  - 新しい構造体で既存機能が動作すること

### 5.3 リグレッションテスト

#### TR-005: 既存テストの成功

- **要件**: 既存のすべてのテストが新しい構造体で成功すること
- **対策**: テストコードを段階的に更新し、すべてのテストが通過するまで継続

---

## 6. ドキュメント要件

### DR-001: API ドキュメント

- **要件**: すべての新しい型、関数にGoDocコメントを記述する
- **内容**:
  - 型の目的、用途
  - フィールドの説明
  - 使用例

### DR-002: アーキテクチャドキュメント

- **要件**: Spec/Runtime分離のアーキテクチャを説明するドキュメントを作成する
- **内容**:
  - 分離の背景、理由
  - 各層の責務
  - データフロー図

### DR-003: マイグレーションガイド

- **要件**: 既存コードから新しいコードへの移行方法を説明するガイドを作成する
- **内容**:
  - 変更点の要約
  - 移行手順
  - Before/After のコード例

---

## 7. 成功基準

### 7.1 機能面

- [ ] すべてのSpec型が定義されている
- [ ] すべてのRuntime型が定義されている
- [ ] すべての展開関数が実装されている
- [ ] TOMLローダーが `ConfigSpec` を返す
- [ ] GroupExecutor が `RuntimeGroup` を使用する
- [ ] Executor が `RuntimeCommand` を使用する

### 7.2 品質面

- [ ] すべてのテストが成功している
- [ ] コードカバレッジ > 80%
- [ ] リグレッションテストがゼロ件
- [ ] パフォーマンステストが許容範囲内

### 7.3 ドキュメント面

- [ ] すべての型にGoDocコメントがある
- [ ] アーキテクチャドキュメントが完成している
- [ ] マイグレーションガイドが完成している

---

## 8. リスクと制約

### 8.1 リスク

| リスクID | リスク内容 | 影響度 | 発生確率 | 対策 |
|---------|-----------|-------|---------|------|
| R-001 | 大規模リファクタリングによるデグレーション | 高 | 中 | 段階的な移行、徹底的なテスト |
| R-002 | レビューコストの増大 | 中 | 高 | PR の分割、詳細なコメント |
| R-003 | パフォーマンス劣化 | 中 | 低 | ベンチマークテストの実施 |
| R-004 | メモリ使用量の増加 | 低 | 中 | メモリプロファイリング |

### 8.2 制約

- **時間的制約**: Task 0034 がブロックされているため、優先度が高い
- **技術的制約**: 既存のTOMLフォーマットを変更できない
- **リソース的制約**: 一人で実装するため、作業量に限界がある

---

## 9. 承認

### 9.1 要件の承認

本要件定義書は、以下の観点で承認されることを想定しています:

- [ ] 機能要件が明確に定義されている
- [ ] 非機能要件が適切に設定されている
- [ ] セキュリティ要件が考慮されている
- [ ] テスト要件が十分である
- [ ] リスクが適切に評価されている

### 9.2 次のステップ

本要件定義書の承認後、以下のドキュメントを作成します:

1. アーキテクチャ設計書（`02_architecture.md`）
2. 詳細仕様書（`03_specification.md`）
3. 実装計画書（`04_implementation_plan.md`）

---

## まとめ

本プロジェクトは、`runnertypes` パッケージの構造体を **Spec層**（immutable）と **Runtime層**（mutable）に分離することで、型安全性の向上、不変性の保証、テストの容易化を実現します。

これは Task 0034 の前提条件であり、優先度の高いプロジェクトです。段階的に実装し、徹底的にテストすることで、デグレーションのリスクを最小化します。
