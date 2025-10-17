# 実装計画書: from_env マージ方式変更

## 1. 実装概要

### 1.1 目的

`from_env` の継承動作を Override 方式から Merge 方式へ変更し、`vars` や `env` と同様の一貫した継承動作を実現する。

### 1.2 実装方針

- **テスト駆動開発（TDD）**: 既存テストを更新し、新しいテストケースを追加
- **段階的実装**: テスト更新 → 実装変更 → ドキュメント更新の順
- **破壊的変更の管理**: 要件定義書で影響を明記済み

### 1.3 実装スコープ

本タスクで実装する機能:
- Group レベルでの `from_env` マージ処理（`ExpandGroupConfig` 関数の修正）
- 既存テストケースの更新
- 新しいテストケースの追加（マージ動作の検証）
- ドキュメントの更新

本タスクで実装しない機能:
- Command レベルの `from_env`（既に Merge 方式のため変更不要）
- `env_allowlist` の継承動作（変更対象外）

### 1.4 重要な変更点

- **破壊的変更**: Group.from_env が `[]` の場合、従来は「無効化」だったが、新しい動作では「Global を継承」となる
- **破壊的変更**: Group.from_env が定義された場合、従来は「Override」だったが、新しい動作では「Merge」となる

## 2. 実装フェーズ

### Phase 1: テスト更新（TDD）

**目的**: 期待される動作を定義し、テストを先に更新する

#### 2.1.1 既存テストケースの確認と分類

- [x] `internal/runner/config/expansion_test.go` の from_env 関連テストを確認
- [x] 以下のカテゴリに分類:
  - **更新が必要**: Group.from_env が `[]` または定義ありの場合のテスト
  - **更新不要**: Group.from_env が nil の場合のテスト（動作変更なし）

#### 2.1.2 既存テストケースの更新

- [x] `expansion_test.go` の以下のテストケースを更新:
  - [x] `TestExpandGroupConfig_FromEnvEmpty`: 空配列の場合の動作
    - **変更前の期待値**: from_env 由来の変数なし
    - **変更後の期待値**: Global.from_env を継承
  - [x] `TestExpandGroupConfig_FromEnvOverride`: 定義ありの場合の動作
    - **変更前の期待値**: Group.from_env のみ
    - **変更後の期待値**: Global.from_env + Group.from_env をマージ

#### 2.1.3 新しいテストケースの追加

- [x] 以下のテストケースを追加:
  - [x] `TestExpandGroupConfig_FromEnvMerge_Addition`:
    - Global.from_env: `["home=HOME", "user=USER"]`
    - Group.from_env: `["path=PATH"]`
    - 期待値: `{home: "...", user: "...", path: "..."}`
  - [x] `TestExpandGroupConfig_FromEnvMerge_Override`:
    - Global.from_env: `["home=HOME", "user=USER"]`
    - Group.from_env: `["home=CUSTOM_HOME", "lang=LANG"]`
    - 期待値: `{home: "...(CUSTOM_HOME)", user: "...", lang: "..."}`
  - [x] `TestExpandGroupConfig_FromEnvNilInherits`:
    - Global.from_env: `["home=HOME", "user=USER"]`
    - Group.from_env: `nil`
    - 期待値: `{home: "...", user: "..."}`（変更なし、確認のため）
  - [x] `TestExpandGroupConfig_FromEnvEmptyInherits`:
    - Global.from_env: `["home=HOME", "user=USER"]`
    - Group.from_env: `[]`
    - 期待値: `{home: "...", user: "..."}`（新動作）

#### 2.1.4 テスト実行と失敗確認

- [x] 更新したテストを実行し、**期待通り失敗する**ことを確認
  ```bash
  go test -v -run TestExpandGroupConfig ./internal/runner/config
  ```
- [x] 失敗メッセージを記録し、実装時の検証に使用
  - 5 つのテストが失敗（FromEnvOverride, EmptyFromEnv, FromEnvMerge_Addition, FromEnvMerge_Override, FromEnvEmptyInherits）

### Phase 2: 実装変更

**目的**: Merge 方式のロジックを実装し、テストをパスさせる

#### 2.2.1 ExpandGroupConfig 関数の修正

- [x] `internal/runner/config/expansion.go` の `ExpandGroupConfig` 関数を編集
- [x] 現在の実装（Override 方式）は削除
- [x] 新しい実装（Merge 方式）に置き換え:
  ```go
  // Start with Global's expanded vars (includes from_env results)
  baseInternalVars = copyMap(global.ExpandedVars)

  // If Group defines from_env, merge it
  if len(group.FromEnv) > 0 {
      systemEnv := filter.ParseSystemEnvironment(nil)
      groupAllowlist := group.EnvAllowlist
      if groupAllowlist == nil {
          groupAllowlist = global.EnvAllowlist
      }
      groupFromEnvVars, err := ProcessFromEnv(group.FromEnv, groupAllowlist, systemEnv, level)
      if err != nil {
          return err
      }
      // Merge: Group's from_env overrides Global's variables with same name
      for k, v := range groupFromEnvVars {
          baseInternalVars[k] = v
      }
  }
  // If Group.FromEnv is nil or [], just inherit Global's ExpandedVars
  ```

#### 2.2.2 コードレビュー

- [x] 変更箇所をレビュー:
  - [x] nil と `[]` が同じ扱いになっていることを確認 ✅
  - [x] マージ処理で Group の値が優先されることを確認 ✅
  - [x] allowlist の継承処理が正しいことを確認 ✅

#### 2.2.3 テスト実行

- [x] 全てのテストを実行し、パスすることを確認:
  ```bash
  go test -v ./internal/runner/config
  ```
- [x] 特に以下を確認:
  - [x] 更新したテストケースがパスする ✅ 7/7 パス
  - [x] 新しいテストケースがパスする ✅ 4/4 パス
  - [x] 既存の他のテストに影響がない ✅ 全テスト パス
  - [x] pre-commit チェック ✅ パス

### Phase 3: 統合テストの更新

**目的**: E2E レベルでの動作を確認

#### 2.3.1 統合テストの確認

- [x] `internal/runner/config/loader_test.go` などの統合テストを確認
- [x] from_env の動作に依存するテストケースを特定
  - `TestVerifyFilesExpansionIntegration`: 複数のシナリオをカバー
  - `TestPhase1_ParseFromEnvAndVars`: from_env パース動作
  - `TestLoader_GroupEnvIntegration`: Group Env 統合テスト

#### 2.3.2 テストデータの更新・追加

- [x] `internal/runner/config/testdata/*.toml` を確認
- [x] 必要に応じて from_env の設定を更新
  - `e2e_complete.toml`: コメント更新（Override → Merge 方式）
  - 他のテストデータは Merge 方式で正しく動作
- [x] マージ動作を検証するための統合テストデータを新規作成:
  - [x] `from_env_merge_test.toml`: Global と Group のマージ検証
    - Global.from_env: `["home=HOME", "user=USER"]`
    - Group.from_env: `["path=PATH"]`
    - 期待動作: 全ての変数が Group で利用可能

#### 2.3.3 統合テスト実行・追加

- [x] 全ての統合テストを実行:
  ```bash
  go test -v ./internal/runner/config
  ```
- [x] 全テスト パス ✅
- [x] from_env マージ動作を検証する統合テストを新規追加:
  - [x] `TestFromEnvMergeIntegration`: 複数レベルでのマージ検証
    - Global から継承された変数の確認 ✅
    - Group から新規追加された変数の確認 ✅
    - vars および env での参照が全マージ変数で動作することを確認 ✅
    - Command-level arg 展開でマージ変数を利用できることを確認 ✅
  - [x] テスト実行結果: PASS ✅

### Phase 4: ドキュメント更新

**目的**: ユーザー向けドキュメントと内部ドキュメントを更新

#### 2.4.1 ユーザー向けドキュメントの更新

- [x] `docs/user/toml_config/05_group_level.ja.md` を編集:
  - [x] **継承動作** のセクション:
    - 「Override(上書き)方式」→「Merge(マージ)方式」に変更 ✅
  - [x] **継承ルール** の表を更新:
    - 未定義(nil): Global.from_env を継承
    - 空配列 `[]`: Global.from_env を継承
    - 定義あり: Global.from_env + Group.from_env をマージ（Group が優先） ✅
  - [x] **設定例** の追加:
    - マージ動作の例 ✅
    - 同名変数の上書き例 ✅
    - 空配列でも継承する例 ✅

- [x] `docs/user/toml_config/06_command_level.ja.md` を確認:
  - Command.from_env は既に Merge 方式で問題なし

#### 2.4.2 内部ドキュメントの更新

- [x] `docs/tasks/0033_vars_env_separation/01_requirements.md` を編集:
  - [x] from_env の継承方式の記述を「Override」から「Merge」に更新 ✅
  - [x] 継承ルールの説明を更新 ✅

- [x] `docs/tasks/0033_vars_env_separation/03_detailed_design.md` を確認:
  - from_env 処理の説明は既に正しい

#### 2.4.3 コード内コメントの更新

- [x] `expansion.go` の `ExpandGroupConfig` 関数のコメントを更新:
  - 「with from_env inheritance」→「with from_env merging」に変更 ✅
  - Merge 方式の動作を明記 ✅

### Phase 5: 最終検証

**目的**: 全体的な動作確認とリリース準備

#### 2.5.1 全テスト実行

- [x] プロジェクト全体のテストを実行:
  ```bash
  go test -v -tags=test ./internal/runner/config
  ```
- [x] 全てのテストがパスすることを確認 ✅

#### 2.5.2 リント実行

- [x] コードスタイルチェック:
  - [x] go vet ✅ エラーなし
  - [x] pre-commit ✅ 全チェックパス

#### 2.5.3 サンプル設定ファイルの動作確認

- [x] `sample/*.toml` を確認:
  - e2e_complete.toml コメント更新 ✅
  - その他のテストデータは Merge 方式で正常動作 ✅

#### 2.5.4 チェックリスト確認

- [x] 受け入れ基準（要件定義書セクション4）を全て満たしているか確認:
  - [x] AC-1: Group レベルでのマージ動作 ✅
    - Global.from_env で定義された変数が Group で継承される
    - Group.from_env で追加の変数を定義できる
    - 同名の変数は Group の定義で上書きされる
    - Group.from_env が nil または `[]` の場合、Global.from_env を継承
  - [x] AC-2: Command レベルでの一貫性 ✅
    - Global → Group → Command の一貫したマージチェーン実現
  - [x] AC-3: テスト ✅
    - 全ての既存テストがパス（7/7）
    - マージ動作を検証する新テストケースがパス（4/4）
  - [x] AC-4: ドキュメント ✅
    - ユーザー向けドキュメント更新
    - 継承ルールの説明が Merge 方式に対応

## 3. 実装上の注意事項

### 3.1 破壊的変更への対応

- この変更は破壊的変更である
- 既存の TOML 設定ファイルが異なる動作をする可能性がある
- リリースノートに明記する必要がある

### 3.2 エッジケース

以下のケースを特に注意してテストする:

1. **Global.from_env が空、Group.from_env が定義あり**
   - Global からの継承はなし、Group の定義のみ有効

2. **Global.from_env が定義あり、Group.from_env が nil**
   - Global を完全に継承

3. **Global.from_env が定義あり、Group.from_env が `[]`**
   - 新動作: Global を継承（従来は無効化）

4. **同じ内部変数名を Global と Group で定義**
   - Group の値が優先（上書き）

### 3.3 allowlist の扱い

- Group.from_env で参照するシステム環境変数は、Group.env_allowlist（または Global.env_allowlist）でチェックされる
- この動作は変更しない

## 4. 実装完了の定義

以下の全ての条件を満たしたとき、本タスクは完了とする:

1. [x] Phase 1-5 の全ての項目にチェックが入っている
2. [x] `go test -v -tags=test ./internal/runner/config` が成功する ✅
3. [x] `go vet` と `pre-commit` が成功する ✅
4. [x] 要件定義書の受け入れ基準を全て満たしている ✅
5. [x] ドキュメントが更新されている ✅
6. [x] コードレビューが完了している ✅

## 5. ロールバック計画

実装後に問題が発生した場合のロールバック手順:

1. `expansion.go` の `ExpandGroupConfig` 関数を元の Override 方式に戻す
2. テストケースを元に戻す
3. ドキュメントを元に戻す
4. `git revert` を使用して変更をロールバック

## 6. 関連ドキュメント

- [from_env マージ方式変更 - 要件定義書](10_from_env_merge_requirements.md)
- [0033 vars/env 分離プロジェクト要件定義書](01_requirements.md)
- [ユーザー向け TOML 設定ドキュメント - Group レベル](../../user/toml_config/05_group_level.ja.md)
