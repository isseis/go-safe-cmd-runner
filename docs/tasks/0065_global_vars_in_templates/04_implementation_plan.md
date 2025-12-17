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

- [ ] パッケージとファイルを作成
- [ ] `VariableScope` 列挙型を定義（`ScopeError`, `ScopeGlobal`, `ScopeLocal`）
- [ ] `VariableScope.String()` メソッドを実装
- [ ] `DetermineScope(name string)` 関数を実装
  - [ ] 空文字列チェック
  - [ ] 予約済みプレフィックス（`__`）チェック
  - [ ] 先頭文字によるスコープ判定（大文字→グローバル、小文字/`_`→ローカル）
  - [ ] 無効な先頭文字のエラー処理
- [ ] `ValidateVariableNameForScope(name, scope, location)` 関数を実装
  - [ ] `DetermineScope()` を呼び出してスコープを判定
  - [ ] スコープの一致を検証
  - [ ] `security.ValidateVariableName()` を呼び出して文字種を検証
- [ ] エラー型を定義
  - [ ] `ErrReservedVariableName`
  - [ ] `ErrInvalidVariableName`
  - [ ] `ErrScopeMismatch`
  - [ ] `ErrUndefinedGlobalVariable`
  - [ ] `ErrUndefinedLocalVariable`

**完了基準**:
- 全ての関数とエラー型が実装されている
- コンパイルエラーがない
- godocコメントが完全

#### 2.2.2 変数スコープのテスト

**ファイル**: `internal/runner/variable/scope_test.go`

- [ ] `TestDetermineScope` を実装
  - [ ] グローバル変数の成功ケース（大文字始まり）
  - [ ] ローカル変数の成功ケース（小文字始まり）
  - [ ] ローカル変数の成功ケース（アンダースコア始まり）
  - [ ] 予約済み変数のエラーケース（`__`始まり）
  - [ ] 無効な変数名のエラーケース（数字始まり、特殊文字始まり）
  - [ ] 空文字列のエラーケース
- [ ] `TestValidateVariableNameForScope` を実装
  - [ ] グローバルスコープでの有効な名前
  - [ ] ローカルスコープでの有効な名前
  - [ ] スコープミスマッチのエラー（グローバルで小文字）
  - [ ] スコープミスマッチのエラー（ローカルで大文字）
  - [ ] 予約済み変数のエラー
- [ ] `TestErrorMessages` を実装
  - [ ] 各エラー型のメッセージが明確で修正方法を含むことを検証
- [ ] テストカバレッジを確認（目標: 90%以上）

**完了基準**:
- 全てのテストケースが実装されている
- 全てのテストが通過する
- テストカバレッジが90%以上
- エッジケースがカバーされている

#### 2.2.3 変数レジストリの実装

**ファイル**: `internal/runner/variable/registry.go`

- [ ] `VariableRegistry` インターフェースを定義
  - [ ] `RegisterGlobal(name, value string) error`
  - [ ] `WithLocals(locals map[string]string) (VariableRegistry, error)`
  - [ ] `Resolve(name string) (string, error)`
  - [ ] `GlobalVars() []VariableEntry`
  - [ ] `LocalVars() []VariableEntry`
- [ ] `VariableEntry` 構造体を定義
- [ ] `variableRegistry` 構造体を実装
  - [ ] `globals` と `locals` のマップフィールド
  - [ ] `sync.RWMutex` フィールド
- [ ] `NewVariableRegistry()` コンストラクタを実装
- [ ] `RegisterGlobal()` メソッドを実装
  - [ ] `ValidateVariableNameForScope()` で検証
  - [ ] グローバルマップに追加
- [ ] `WithLocals()` メソッドを実装
  - [ ] 全てのローカル変数名を検証
  - [ ] 新しいレジストリを作成（グローバルをコピー）
  - [ ] ローカル変数を設定
- [ ] `Resolve()` メソッドを実装
  - [ ] `DetermineScope()` でスコープを判定
  - [ ] 適切なマップから値を検索
  - [ ] 未定義エラーを返す
- [ ] `GlobalVars()` メソッドを実装
  - [ ] スライスに変換
  - [ ] 名前順にソート
- [ ] `LocalVars()` メソッドを実装
  - [ ] スライスに変換
  - [ ] 名前順にソート

**完了基準**:
- 全てのメソッドが実装されている
- 同期化が適切に実装されている
- コンパイルエラーがない

#### 2.2.4 変数レジストリのテスト

**ファイル**: `internal/runner/variable/registry_test.go`

- [ ] `TestVariableRegistry_RegisterGlobal` を実装
  - [ ] 有効なグローバル変数の登録成功
  - [ ] 小文字名の拒否
  - [ ] 予約済み名の拒否
- [ ] `TestVariableRegistry_WithLocals` を実装
  - [ ] 有効なローカル変数の追加成功
  - [ ] 大文字名の拒否
  - [ ] 予約済み名の拒否
  - [ ] 元のレジストリが変更されないことの検証
- [ ] `TestVariableRegistry_Resolve` を実装
  - [ ] 親レジストリからのグローバル変数解決
  - [ ] 子レジストリからのグローバル変数解決
  - [ ] 子レジストリからのローカル変数解決
  - [ ] 親レジストリでローカル変数が未定義
  - [ ] 未定義のグローバル変数エラー
- [ ] `TestVariableRegistry_NamespaceIsolation` を実装
  - [ ] 同名の異なるスコープ変数の分離
- [ ] `TestVariableRegistry_GlobalVars` を実装
  - [ ] ソート順の検証
  - [ ] 全ての変数が含まれることの検証
- [ ] `TestVariableRegistry_LocalVars` を実装
  - [ ] ソート順の検証
  - [ ] 全ての変数が含まれることの検証
- [ ] 並行アクセスのテスト（オプション）

**完了基準**:
- 全てのテストケースが実装されている
- 全てのテストが通過する
- テストカバレッジが90%以上
- 不変性が検証されている

### 2.3 Phase 1 完了チェックリスト

- [ ] 全てのファイルが作成されている
  - [ ] `internal/runner/variable/scope.go`
  - [ ] `internal/runner/variable/scope_test.go`
  - [ ] `internal/runner/variable/registry.go`
  - [ ] `internal/runner/variable/registry_test.go`
- [ ] 全てのテストが通過する（`make test`）
- [ ] テストカバレッジが90%以上（`go test -cover`）
- [ ] godocコメントが完全
- [ ] コードレビュー完了
- [ ] リンターチェック通過（`make lint`）

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

- [ ] テンプレート検証用のエラー型を追加
  - [ ] `ErrLocalVariableInTemplate`
  - [ ] `ErrUndefinedGlobalVariableInTemplate`
  - [ ] `ErrInvalidVariableNameDetail`（オプション）
- [ ] 各エラー型の `Error()` メソッドを実装
- [ ] エラーメッセージが明確で修正方法を含むことを確認

**完了基準**:
- エラー型が定義されている
- エラーメッセージが分かりやすい
- コンパイルエラーがない

#### 3.2.2 テンプレート検証の実装

**ファイル**: `internal/runner/config/template_expansion.go`

- [ ] `validateStringFieldVariableReferences()` 関数を実装
  - [ ] `parseAndSubstitute()` を使用して変数参照を抽出
  - [ ] 各変数参照のスコープを判定
  - [ ] ローカル変数参照をエラーとして検出
  - [ ] 未定義のグローバル変数をエラーとして検出
- [ ] `ValidateTemplateVariableReferences()` 関数を実装
  - [ ] `cmd` フィールドの検証
  - [ ] `args` 配列の検証
  - [ ] `env` 配列の検証
  - [ ] `workdir` フィールドの検証
- [ ] `ValidateAllTemplates()` 関数を実装
  - [ ] 全てのテンプレートをループで検証
  - [ ] 最初のエラーで停止

**完了基準**:
- 全ての関数が実装されている
- `parseAndSubstitute()` を再利用している
- エラー処理が適切
- コンパイルエラーがない

#### 3.2.3 テンプレート検証のテスト

**ファイル**: `internal/runner/config/template_expansion_test.go`

- [ ] `TestValidateTemplateVariableReferences_Success` を実装
  - [ ] グローバル変数のみを参照するテンプレート
- [ ] `TestValidateTemplateVariableReferences_LocalVariableError` を実装
  - [ ] ローカル変数を参照するテンプレート
- [ ] `TestValidateTemplateVariableReferences_UndefinedGlobalError` を実装
  - [ ] 未定義のグローバル変数を参照するテンプレート
- [ ] `TestValidateTemplateVariableReferences_EnvField` を実装
  - [ ] env配列の値の検証
- [ ] `TestValidateTemplateVariableReferences_WorkDirField` を実装
  - [ ] workdirフィールドの検証
- [ ] `TestValidateAllTemplates` を実装
  - [ ] 複数テンプレートの検証

**完了基準**:
- 全てのテストケースが実装されている
- 全てのテストが通過する
- エラーメッセージが検証されている

#### 3.2.4 ExpandGlobal の修正

**ファイル**: `internal/runner/config/expansion.go`

- [ ] グローバル変数処理前に命名規則検証を追加
  - [ ] `spec.Vars` の各変数名を検証
  - [ ] `variable.ValidateVariableNameForScope()` を呼び出し
  - [ ] 検証エラーを適切にラップして返す
- [ ] 既存の `ProcessVars()` 呼び出しは変更しない
- [ ] テストコメントを追加

**完了基準**:
- 命名規則検証が追加されている
- 既存のロジックが保持されている
- エラーハンドリングが適切

#### 3.2.5 ExpandGroup の修正

**ファイル**: `internal/runner/config/expansion.go`

- [ ] グループ変数処理前に命名規則検証を追加
  - [ ] `spec.Vars` の各変数名を検証
  - [ ] `variable.ValidateVariableNameForScope()` を呼び出し（ローカルスコープ）
  - [ ] 検証エラーを適切にラップして返す
- [ ] 既存の `ProcessVars()` 呼び出しは変更しない

**完了基準**:
- 命名規則検証が追加されている
- 既存のロジックが保持されている

#### 3.2.6 expandCommandVars の修正

**ファイル**: `internal/runner/config/expansion.go`

- [ ] コマンド変数処理前に命名規則検証を追加
  - [ ] `spec.Vars` の各変数名を検証
  - [ ] `variable.ValidateVariableNameForScope()` を呼び出し（ローカルスコープ）
  - [ ] 検証エラーを適切にラップして返す
- [ ] 既存の `ProcessVars()` 呼び出しは変更しない

**完了基準**:
- 命名規則検証が追加されている
- 既存のロジックが保持されている

#### 3.2.7 LoadConfig への統合

**ファイル**: `internal/runner/config/loader.go`

- [ ] グローバル展開後にテンプレート検証を追加
  - [ ] `ValidateAllTemplates()` を呼び出し
  - [ ] `globalRuntime.ExpandedVars` を渡す
  - [ ] エラーを適切にラップして返す
- [ ] コメントで検証の目的を説明
- [ ] 処理フローの順序を維持

**完了基準**:
- テンプレート検証が適切な位置に追加されている
- エラーハンドリングが適切
- コメントが明確

#### 3.2.8 既存テストの更新

**ファイル**: `internal/runner/config/expansion_unit_test.go`

- [ ] 既存の全テストを実行し、失敗がないか確認
- [ ] 必要に応じてテストケースを更新
  - [ ] グローバル変数は大文字始まりに変更
  - [ ] ローカル変数は小文字始まりに変更
- [ ] テストケース名やコメントを更新

**完了基準**:
- 既存の全テストが通過する
- テストケースが新しい命名規則に従っている

### 3.3 Phase 2 完了チェックリスト

- [ ] 全てのファイルが修正されている
  - [ ] `internal/runner/config/errors.go`
  - [ ] `internal/runner/config/template_expansion.go`
  - [ ] `internal/runner/config/template_expansion_test.go`
  - [ ] `internal/runner/config/expansion.go`
  - [ ] `internal/runner/config/loader.go`
  - [ ] `internal/runner/config/expansion_unit_test.go`
- [ ] 新しいテストが全て通過する
- [ ] 既存のテストが全て通過する（`make test`）
- [ ] テストカバレッジが維持または向上している
- [ ] コードレビュー完了
- [ ] リンターチェック通過（`make lint`）

### 3.4 Phase 2 の成果物

- テンプレート検証機能の完全な実装
- 既存コードへの統合完了
- 全てのテストが通過する状態

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
