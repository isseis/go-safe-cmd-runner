# テンプレート定義でのグローバル変数参照 - 実装計画書

## 1. 概要

本文書は、テンプレート定義でグローバル変数を参照可能にする機能の実装計画を定義する。

### 1.1 関連文書

- [01_requirements.md](./01_requirements.md) - 要件定義書
- [02_architecture.md](./02_architecture.md) - アーキテクチャ設計書
- [03_detailed_specification.md](./03_detailed_specification.md) - 詳細仕様書

### 1.2 実装の全体像

実装は以下の3つのフェーズに分割される：

- **Phase 1**: 基礎実装（変数スコープとレジストリ）
- **Phase 2**: 統合実装（既存コードへの統合）
- **Phase 3**: 検証と最適化

各フェーズは独立してテストおよびレビュー可能な単位となっている。

## 2. Phase 1: 基礎実装（3-5日）

### 2.1 目標

変数スコープの型定義と変数レジストリの実装を完了する。既存コードに依存しない独立したモジュールとして実装し、完全にテストされた状態にする。

### 2.2 実装タスク

#### 2.2.1 変数スコープの型定義

**ファイル**: `internal/runner/variable/scope.go`

- [x] パッケージとファイルを作成
- [x] `VariableScope` 列挙型を定義（`ScopeError`, `ScopeGlobal`, `ScopeLocal`）
- [x] `VariableScope.String()` メソッドを実装
- [x] `DetermineScope(name string)` 関数を実装
  - [x] 空文字列チェック
  - [x] 予約済みプレフィックス（`__`）チェック
  - [x] 先頭文字によるスコープ判定（大文字→グローバル、小文字/`_`→ローカル）
  - [x] 無効な先頭文字のエラー処理
- [x] `ValidateVariableNameForScope(name, scope, location)` 関数を実装
  - [x] `DetermineScope()` を呼び出してスコープを判定
  - [x] スコープの一致を検証
  - [x] `security.ValidateVariableName()` を呼び出して文字種を検証
- [x] エラー型を定義
  - [x] `ErrReservedVariableName`
  - [x] `ErrInvalidVariableName`
  - [x] `ErrScopeMismatch`
  - [x] `ErrUndefinedGlobalVariable`
  - [x] `ErrUndefinedLocalVariable`

**完了基準**:
- 全ての関数とエラー型が実装されている
- コンパイルエラーがない
- godocコメントが完全

#### 2.2.2 変数スコープのテスト

**ファイル**: `internal/runner/variable/scope_test.go`

- [x] `TestDetermineScope` を実装
  - [x] グローバル変数の成功ケース（大文字始まり）
  - [x] ローカル変数の成功ケース（小文字始まり）
  - [x] ローカル変数の成功ケース（アンダースコア始まり）
  - [x] 予約済み変数のエラーケース（`__`始まり）
  - [x] 無効な変数名のエラーケース（数字始まり、特殊文字始まり）
  - [x] 空文字列のエラーケース
- [x] `TestValidateVariableNameForScope` を実装
  - [x] グローバルスコープでの有効な名前
  - [x] ローカルスコープでの有効な名前
  - [x] スコープミスマッチのエラー（グローバルで小文字）
  - [x] スコープミスマッチのエラー（ローカルで大文字）
  - [x] 予約済み変数のエラー
- [x] `TestErrorMessages` を実装
  - [x] 各エラー型のメッセージが明確で修正方法を含むことを検証
- [x] テストカバレッジを確認（目標: 90%以上）

**完了基準**:
- 全てのテストケースが実装されている
- 全てのテストが通過する
- テストカバレッジが90%以上
- エッジケースがカバーされている

#### 2.2.3 変数レジストリの実装

**ファイル**: `internal/runner/variable/registry.go`

- [x] `VariableRegistry` インターフェースを定義
  - [x] `RegisterGlobal(name, value string) error`
  - [x] `WithLocals(locals map[string]string) (VariableRegistry, error)`
  - [x] `Resolve(name string) (string, error)`
  - [x] `GlobalVars() []VariableEntry`
  - [x] `LocalVars() []VariableEntry`
- [x] `VariableEntry` 構造体を定義
- [x] `variableRegistry` 構造体を実装
  - [x] `globals` と `locals` のマップフィールド
  - [x] `sync.RWMutex` フィールド
- [x] `NewVariableRegistry()` コンストラクタを実装
- [x] `RegisterGlobal()` メソッドを実装
  - [x] `ValidateVariableNameForScope()` で検証
  - [x] グローバルマップに追加
- [x] `WithLocals()` メソッドを実装
  - [x] 全てのローカル変数名を検証
  - [x] 新しいレジストリを作成（グローバルをコピー）
  - [x] ローカル変数を設定
- [x] `Resolve()` メソッドを実装
  - [x] `DetermineScope()` でスコープを判定
  - [x] 適切なマップから値を検索
  - [x] 未定義エラーを返す
- [x] `GlobalVars()` メソッドを実装
  - [x] スライスに変換
  - [x] 名前順にソート
- [x] `LocalVars()` メソッドを実装
  - [x] スライスに変換
  - [x] 名前順にソート

**完了基準**:
- 全てのメソッドが実装されている
- 同期化が適切に実装されている
- コンパイルエラーがない

#### 2.2.4 変数レジストリのテスト

**ファイル**: `internal/runner/variable/registry_test.go`

- [x] `TestVariableRegistry_RegisterGlobal` を実装
  - [x] 有効なグローバル変数の登録成功
  - [x] 小文字名の拒否
  - [x] 予約済み名の拒否
- [x] `TestVariableRegistry_WithLocals` を実装
  - [x] 有効なローカル変数の追加成功
  - [x] 大文字名の拒否
  - [x] 予約済み名の拒否
  - [x] 元のレジストリが変更されないことの検証
- [x] `TestVariableRegistry_Resolve` を実装
  - [x] 親レジストリからのグローバル変数解決
  - [x] 子レジストリからのグローバル変数解決
  - [x] 子レジストリからのローカル変数解決
  - [x] 親レジストリでローカル変数が未定義
  - [x] 未定義のグローバル変数エラー
- [x] `TestVariableRegistry_NamespaceIsolation` を実装
  - [x] 同名の異なるスコープ変数の分離
- [x] `TestVariableRegistry_GlobalVars` を実装
  - [x] ソート順の検証
  - [x] 全ての変数が含まれることの検証
- [x] `TestVariableRegistry_LocalVars` を実装
  - [x] ソート順の検証
  - [x] 全ての変数が含まれることの検証
- [-] 並行アクセスのテスト（オプション）

**完了基準**:
- 全てのテストケースが実装されている
- 全てのテストが通過する
- テストカバレッジが90%以上
- 不変性が検証されている

### 2.3 Phase 1 完了チェックリスト

- [x] 全てのファイルが作成されている
  - [x] `internal/runner/variable/scope.go`
  - [x] `internal/runner/variable/scope_test.go`
  - [x] `internal/runner/variable/registry.go`
  - [x] `internal/runner/variable/registry_test.go`
- [x] 全てのテストが通過する（`make test`）
- [x] テストカバレッジが90%以上（`go test -cover`） - 97.8%達成
- [x] godocコメントが完全
- [-] コードレビュー完了
- [x] リンターチェック通過（`make lint`）

### 2.4 Phase 1 の成果物

- 完全にテストされた変数スコープモジュール
- 独立して動作する変数レジストリ
- 明確なエラーメッセージを持つエラー型

---

## 3. Phase 2: 統合実装（3-4日）

### 3.1 目標

Phase 1で実装した変数スコープとレジストリを、既存の設定読み込みと変数展開処理に統合する。既存のテストが全て通過することを保証しながら、新機能を追加する。

### 3.2 実装タスク

#### 3.2.1 エラー型の追加

**ファイル**: `internal/runner/config/errors.go`

- [x] テンプレート検証用のエラー型を追加
  - [x] `ErrLocalVariableInTemplate`
  - [x] `ErrUndefinedGlobalVariableInTemplate`
  - [x] `ErrInvalidVariableScopeDetail`
- [x] 各エラー型の `Error()` メソッドを実装
- [x] エラーメッセージが明確で修正方法を含むことを確認

**完了基準**:
- [x] エラー型が定義されている
- [x] エラーメッセージが分かりやすい
- [x] コンパイルエラーがない

#### 3.2.2 テンプレート検証の実装

**ファイル**: `internal/runner/config/template_expansion.go`

- [x] `validateStringFieldVariableReferences()` 関数を実装
  - [x] `parseAndSubstitute()` を使用して変数参照を抽出
  - [x] 各変数参照のスコープを判定
  - [x] ローカル変数参照をエラーとして検出
  - [x] 未定義のグローバル変数をエラーとして検出
- [x] `ValidateTemplateVariableReferences()` 関数を実装
  - [x] `cmd` フィールドの検証
  - [x] `args` 配列の検証
  - [x] `env` 配列の検証（KEY=value形式）
  - [x] `workdir` フィールドの検証
- [x] `ValidateAllTemplates()` 関数を実装
  - [x] 全てのテンプレートをループで検証
  - [x] 最初のエラーで停止

**完了基準**:
- [x] 全ての関数が実装されている
- [x] `parseAndSubstitute()` を再利用している
- [x] エラー処理が適切
- [x] コンパイルエラーがない

#### 3.2.3 テンプレート検証のテスト

**ファイル**: `internal/runner/config/template_expansion_validation_test.go`

- [x] `TestValidateTemplateVariableReferences` を実装（17テストケース）
  - [x] グローバル変数のみを参照するテンプレート
  - [x] ローカル変数を参照するテンプレート（エラー）
  - [x] 未定義のグローバル変数を参照するテンプレート（エラー）
  - [x] env配列の値の検証
  - [x] workdirフィールドの検証
  - [x] エスケープされた%の処理
  - [x] 複数フィールドの検証
- [x] `TestValidateStringFieldVariableReferences` を実装（7テストケース）
  - [x] 無効文字を含む変数参照の検出
- [x] `TestValidateAllTemplates` を実装（4テストケース）
  - [x] 複数テンプレートの検証
- [x] `TestErrorMessages` を実装（2テストケース）
  - [x] エラーメッセージの内容検証

**完了基準**:
- [x] 全てのテストケースが実装されている
- [x] 全てのテストが通過する
- [x] エラーメッセージが検証されている
- [x] テストカバレッジ: 88.2%達成

#### 3.2.4 スコープ検証の統合（validation.go）

**ファイル**: `internal/runner/config/validation.go`

- [x] `validateVariableName` 関数にスコープ検証を追加
  - [x] level が "global" の場合は ScopeGlobal を期待
  - [x] それ以外の場合は ScopeLocal を期待
  - [x] `variable.ValidateVariableNameForScope()` を呼び出し
  - [x] 検証エラーを ErrInvalidVariableScopeDetail としてラップ
- [x] 既存の POSIX検証と予約済みプレフィックスチェックを保持

**完了基準**:
- [x] 命名規則検証が追加されている
- [x] 既存のロジックが保持されている
- [x] エラーハンドリングが適切

#### 3.2.5 main.go への統合

**ファイル**: `cmd/runner/main.go`

- [x] グローバル展開後にテンプレート検証を追加
  - [x] `ValidateAllTemplates()` を呼び出し（line ~225）
  - [x] `runtimeGlobal.ExpandedVars` を渡す
  - [x] エラーを PreExecutionError としてラップして返す
- [x] コメントで検証の目的を説明
- [x] 処理フローの順序を維持（ExpandGlobal → ValidateAllTemplates → ExpandGroup）

**完了基準**:
- [x] テンプレート検証が適切な位置に追加されている
- [x] エラーハンドリングが適切
- [x] コメントが明確

#### 3.2.6 既存テストの更新

**ファイル**: `internal/runner/config/expansion_unit_test.go`

- [x] TestProcessVars系のテストを更新
  - [x] TestProcessVars_ComplexReferenceChain - 変数名を小文字に変更（a, b, c, d, base, new_var）
  - [x] TestProcessVars_UndefinedReference - 変数名を小文字に変更（var, undefined）
  - [x] TestProcessVars_EnvImportVarsConflict - 変数名を小文字に変更（my_var, var1, var2）
- [x] TestProcessVars_InvalidVariableScope を追加（6テストケース）
  - [x] グローバルレベルでの小文字変数（エラー）
  - [x] ローカルレベルでの大文字変数（エラー）
  - [x] 正常なケースの検証

**完了基準**:
- [x] 変更されたテストケースが新スコープルールに適合
- [x] 更新されたテストが通過する

#### 3.2.7 既存テスト修正（残作業）

**多数のテストファイル**

- [ ] 既存の統合テストを全て実行し、失敗を修正
  - [ ] `cmd/runner/*_test.go` - スコープ検証エラー多数
  - [ ] `internal/runner/config/*_test.go` - 命名規則違反多数
  - [ ] `internal/runner/bootstrap/*_test.go` - 同上
- [ ] 各テストの変数名をスコープルールに適合させる
  - [ ] グローバルレベル: 大文字始まりに変更
  - [ ] グループ/コマンドレベル: 小文字始まりに変更

**完了基準**:
- 全ての統合テストが通過する
- スコープルールが全体で適用されている

### 3.3 Phase 2 完了チェックリスト

- [x] コア機能ファイルが実装されている
  - [x] `internal/runner/config/errors.go` - 3エラー型追加
  - [x] `internal/runner/config/template_expansion.go` - 検証関数追加
  - [x] `internal/runner/config/template_expansion_validation_test.go` - 新規テスト
  - [x] `internal/runner/config/validation.go` - スコープ検証統合
  - [x] `cmd/runner/main.go` - テンプレート検証呼び出し
  - [x] `internal/runner/config/expansion_unit_test.go` - 一部更新
- [x] 新機能のテストが全て通過する（36テストケース）
- [ ] 既存のテストが全て通過する（残作業 - 多数のテスト失敗）
- [x] テストカバレッジ 88.2% (config package)
- [-] コードレビュー完了
- [x] リンターチェック通過
- [x] Phase 2 コミット完了 (d448db30)

### 3.4 Phase 2 の成果物

- [x] テンプレート検証機能の完全な実装
- [x] スコープ検証の統合
- [x] 新機能の包括的なテスト
- [ ] 既存テストの修正（次フェーズで対応予定）

---

## 4. Phase 3: 検証と最適化（2-3日）

### 4.1 目標

エンドツーエンドテストを追加し、エラーメッセージを改善し、パフォーマンスを検証する。ドキュメントを完成させる。

### 4.2 実装タスク

#### 4.2.1 エンドツーエンドテスト

**ファイル**: `internal/runner/config/end_to_end_expansion_test.go`

- [ ] `TestEndToEndExpansion_GlobalVariablesInTemplates` を実装
  - [ ] グローバル変数定義
  - [ ] テンプレート定義（グローバル変数参照）
  - [ ] コマンド定義（params使用）
  - [ ] 完全な展開フローの検証
- [ ] `TestEndToEndExpansion_LocalVariablesInParams` を実装
  - [ ] グローバル変数定義
  - [ ] グループレベルのローカル変数定義
  - [ ] テンプレート定義（グローバル変数参照）
  - [ ] コマンド定義（paramsでローカル変数参照）
  - [ ] 完全な展開フローの検証
- [ ] `TestEndToEndExpansion_ScopeMismatchErrors` を実装
  - [ ] グローバルスコープでの小文字変数定義
  - [ ] ローカルスコープでの大文字変数定義
  - [ ] エラーメッセージの検証
- [ ] `TestEndToEndExpansion_TemplateValidationErrors` を実装
  - [ ] テンプレートでのローカル変数参照
  - [ ] テンプレートでの未定義グローバル変数参照
  - [ ] エラーメッセージの検証

**完了基準**:
- 全てのエンドツーエンドテストが実装されている
- 全てのテストが通過する
- 実際の使用シナリオがカバーされている

#### 4.2.2 TOMLファイルを使った統合テスト

**ファイル**: `cmd/runner/integration_global_vars_in_templates_test.go`

- [ ] テスト用TOMLファイルを作成
  - [ ] `sample/global_vars_in_templates_success.toml`
  - [ ] `sample/global_vars_in_templates_local_in_template_error.toml`
  - [ ] `sample/global_vars_in_templates_undefined_error.toml`
- [ ] 成功ケースのテスト
  - [ ] dry-run出力の検証
  - [ ] 変数が正しく展開されていることの確認
- [ ] エラーケースのテスト
  - [ ] 適切なエラーメッセージが表示されることの確認

**完了基準**:
- TOMLファイルが作成されている
- 統合テストが実装されている
- 全てのテストが通過する

#### 4.2.3 エラーメッセージの改善

- [ ] 全てのエラーメッセージをレビュー
- [ ] ユーザーフレンドリーな表現に改善
- [ ] 修正方法を含めることを確認
- [ ] 一貫性を確認（用語、フォーマット）
- [ ] 実際にエラーを発生させて、メッセージを確認

**完了基準**:
- 全てのエラーメッセージが明確
- 修正方法が含まれている
- 一貫性がある

#### 4.2.4 パフォーマンステスト

**ファイル**: `internal/runner/variable/registry_bench_test.go`

- [ ] `BenchmarkDetermineScope` を実装
- [ ] `BenchmarkRegisterGlobal` を実装
- [ ] `BenchmarkResolve` を実装
- [ ] 大規模な設定ファイルでのベンチマーク
- [ ] 既存実装と比較（劣化が5%以内であることを確認）

**完了基準**:
- ベンチマークが実装されている
- パフォーマンス劣化が5%以内
- ボトルネックが特定されている（もしあれば）

#### 4.2.5 ドキュメントの更新

- [ ] `README.md` の更新
  - [ ] 新機能の説明を追加
  - [ ] グローバル変数とローカル変数の説明
  - [ ] 命名規則の説明
  - [ ] サンプルコードの追加
- [ ] `docs/user/variables.md` の更新（または新規作成）
  - [ ] 変数の種類とスコープの詳細説明
  - [ ] 命名規則の詳細
  - [ ] 使用例
  - [ ] よくある間違いと対処法
- [ ] `CHANGELOG.md` の更新
  - [ ] 新機能の追加
  - [ ] 破壊的変更の説明（もしあれば）
- [ ] サンプルTOMLファイルの更新
  - [ ] 新しい命名規則に従うように修正
  - [ ] グローバル変数を使用した例を追加

**完了基準**:
- 全てのドキュメントが更新されている
- ドキュメントが一貫性を持っている
- サンプルコードが動作する

#### 4.2.6 セキュリティレビュー

- [ ] セキュリティチェックリストの作成
  - [ ] 命名規則の悪用シナリオ
  - [ ] スコープの混乱シナリオ
  - [ ] 未定義変数の注入シナリオ
  - [ ] 予約済み変数の悪用シナリオ
- [ ] 各シナリオに対する対策の確認
- [ ] テストケースでカバーされていることの確認
- [ ] コードレビューの実施

**完了基準**:
- セキュリティチェックリストが完了している
- 全ての攻撃シナリオに対策がある
- テストでカバーされている

### 4.3 Phase 3 完了チェックリスト

- [ ] 全てのエンドツーエンドテストが実装され、通過している
- [ ] TOMLファイルを使った統合テストが実装され、通過している
- [ ] エラーメッセージが改善されている
- [ ] パフォーマンステストが実施され、劣化が5%以内
- [ ] 全てのドキュメントが更新されている
- [ ] セキュリティレビューが完了している
- [ ] 全てのテストが通過する（`make test`）
- [ ] リンターチェック通過（`make lint`）
- [ ] コードレビュー完了

### 4.4 Phase 3 の成果物

- 完全にテストされた機能
- 改善されたエラーメッセージ
- 更新されたドキュメント
- セキュリティレビュー完了

---

## 5. 最終チェックリスト

### 5.1 機能要件

- [ ] グローバル変数の命名規則違反を検出できる
- [ ] ローカル変数の命名規則違反を検出できる
- [ ] テンプレート内のローカル変数参照を検出できる
- [ ] テンプレート内の未定義グローバル変数参照を検出できる
- [ ] paramsでグローバル変数とローカル変数の両方を参照できる
- [ ] 予約済み変数（`__`始まり）を拒否できる

### 5.2 品質要件

- [ ] 全てのエラーメッセージが明確で修正方法を含む
- [ ] 展開後のコマンドが既存のセキュリティ検証を通過する
- [ ] 既存の全テストが通過する
- [ ] 新機能のテストカバレッジが90%以上
- [ ] パフォーマンスの劣化が5%以内

### 5.3 ドキュメント

- [ ] README.mdが更新されている
- [ ] ユーザー向けドキュメントが完全
- [ ] CHANGELOG.mdが更新されている
- [ ] サンプルファイルが更新されている
- [ ] godocコメントが完全

### 5.4 セキュリティ

- [ ] セキュリティチェックリストが完了している
- [ ] 全ての攻撃シナリオに対策がある
- [ ] テストでカバーされている
- [ ] コードレビューが完了している

### 5.5 リリース準備

- [ ] 全てのテストが通過する
- [ ] リンターチェックが通過する
- [ ] ドキュメントが完全
- [ ] CHANGELOGが更新されている
- [ ] マイグレーションガイド（必要な場合）が作成されている

---

## 6. タイムライン

| フェーズ | 期間 | 開始日 | 終了日 |
|---------|------|--------|--------|
| Phase 1: 基礎実装 | 3-5日 | - | - |
| Phase 2: 統合実装 | 3-4日 | - | - |
| Phase 3: 検証と最適化 | 2-3日 | - | - |
| **合計** | **8-12日** | - | - |

*注: 開始日と終了日は実際の実装スケジュールに応じて記入してください。*

---

## 7. リスク管理

### 7.1 技術的リスク

| リスク | 影響 | 確率 | 軽減策 | 担当フェーズ |
|--------|------|------|--------|------------|
| 既存機能の破壊 | 高 | 中 | 全既存テストの通過を各フェーズで確認 | 全フェーズ |
| パフォーマンス劣化 | 中 | 低 | Phase 3でベンチマークテスト実施 | Phase 3 |
| 複雑性の増加 | 中 | 中 | コードレビューとペアプログラミング | 全フェーズ |
| テンプレート検証の誤検知 | 中 | 中 | エンドツーエンドテストで実際の使用例を検証 | Phase 3 |

### 7.2 運用リスク

| リスク | 影響 | 確率 | 軽減策 | 担当フェーズ |
|--------|------|------|--------|------------|
| 後方互換性の問題 | 高 | 中 | 明確なエラーメッセージと移行ガイド | Phase 3 |
| 学習曲線 | 中 | 中 | 充実したドキュメントとサンプル | Phase 3 |
| デバッグの困難さ | 中 | 低 | dry-runモードでの詳細情報表示 | Phase 2 |

---

## 8. レビューポイント

各フェーズの完了時に、以下の点をレビューする：

### 8.1 Phase 1 レビュー

- [ ] 変数スコープの実装が仕様通りか
- [ ] エラー型が適切か
- [ ] テストカバレッジが十分か
- [ ] godocコメントが明確か
- [ ] 独立して動作するか

### 8.2 Phase 2 レビュー

- [ ] 既存コードとの統合が適切か
- [ ] 既存のテストが全て通過するか
- [ ] テンプレート検証が正しく動作するか
- [ ] エラーハンドリングが適切か
- [ ] コードの重複がないか

### 8.3 Phase 3 レビュー

- [ ] エンドツーエンドテストが十分か
- [ ] エラーメッセージが分かりやすいか
- [ ] パフォーマンスが許容範囲内か
- [ ] ドキュメントが完全か
- [ ] セキュリティ対策が十分か

---

## 9. 用語集

| 用語 | 定義 |
|------|------|
| **グローバル変数** | `[global.vars]`で定義される、大文字始まりの変数 |
| **ローカル変数** | `[groups.vars]`または`[groups.commands.vars]`で定義される、小文字始まりの変数 |
| **スコープ** | 変数が定義され、参照可能な範囲 |
| **命名規則** | 変数名の先頭文字に基づくスコープの判定規則 |
| **テンプレート検証** | テンプレート内の変数参照がグローバル変数のみであることを確認する処理 |
| **予約済み変数** | `__`で始まる変数名。将来の拡張用に予約 |

---

## 10. 参考資料

- [01_requirements.md](./01_requirements.md) - 要件定義書
- [02_architecture.md](./02_architecture.md) - アーキテクチャ設計書
- [03_detailed_specification.md](./03_detailed_specification.md) - 詳細仕様書
- Go言語仕様: https://go.dev/ref/spec
- testifyドキュメント: https://github.com/stretchr/testify

---

## 11. 変更履歴

| 日付 | バージョン | 変更内容 | 著者 |
|------|-----------|---------|------|
| 2025-12-17 | 1.0 | 初版作成 | - |
