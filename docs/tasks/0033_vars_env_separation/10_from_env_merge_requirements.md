# from_env マージ方式変更 - 要件定義書

## 1. 概要

### 1.1 目的

現在 `from_env` は Override 方式（nil 時のみ継承、定義時は完全上書き）を採用しているが、これを **Merge 方式**に変更する。これにより、`vars` や `env` と同様の継承動作を実現し、利便性を向上させる。

### 1.2 背景

#### 現在の問題点

1. **利便性の欠如**
   - Global で共通の環境変数を取り込んでも、Group で追加したい場合に全て再定義が必要
   - 例: Global で `HOME`, `USER` を取り込み、deploy グループで `PATH` も追加したい場合、`["home=HOME", "user=USER", "path=PATH"]` と全て書く必要がある

2. **他の設定項目との不整合**
   - `vars`: Merge 方式
   - `env`: Merge 方式
   - `from_env`: Override 方式 ← **不整合**

3. **ドキュメントの複雑性**
   - nil/空配列/定義ありの3パターンを説明する必要がある

#### セキュリティ面での検討

- `from_env` は `env_allowlist` で既にフィルタ済みの環境変数のみを扱う
- 内部変数への変換処理であり、コマンド実行環境への直接影響はない
- セキュリティリスクは **env_allowlist より低い**
- したがって、利便性を優先して Merge 方式を採用することが妥当

### 1.3 スコープ

#### 対象

- `from_env` の継承動作を Override 方式から Merge 方式へ変更
- Group レベルおよび Command レベルの両方

#### 対象外

- `env_allowlist` の継承動作（inherit/explicit/reject モードは維持）
- その他の設定項目の継承動作

## 2. 要件

### 2.1 機能要件

#### FR-1: Group レベルでの from_env マージ

**変更前（Override 方式）:**

| Group.from_env の状態 | 動作 |
|---------------------|------|
| 未定義(nil) | Global.from_env を継承 |
| 空配列 `[]` | どのシステム環境変数も取り込まない |
| 定義あり | Global.from_env を無視し、Group.from_env のみ使用 |

**変更後（Merge 方式）:**

| Group.from_env の状態 | 動作 |
|---------------------|------|
| 未定義(nil) または 空配列 `[]` | Global.from_env を継承 |
| 定義あり | Global.from_env + Group.from_env をマージ（Group が優先） |

**マージルール:**

1. Global.from_env で定義された変数を全て継承
2. Group.from_env で追加の変数を取り込む
3. 同じ内部変数名が定義された場合、Group の定義が優先（上書き）

**設定例:**

```toml
[global]
env_allowlist = ["HOME", "USER", "PATH", "LANG"]
from_env = ["home=HOME", "user=USER"]

[[group]]
name = "deploy"
from_env = ["path=PATH"]
# 結果: home=HOME, user=USER, path=PATH

[[group]]
name = "build"
from_env = ["home=CUSTOM_HOME", "lang=LANG"]
# 結果: home=CUSTOM_HOME (上書き), user=USER, lang=LANG
```

#### FR-2: Command レベルでの from_env マージ

**現在の動作:**

Command レベルでは `from_env` は **Merge 方式**（Group の ExpandedVars に追加マージ）

**変更:**

- 動作は変更しない（既に Merge 方式）
- ただし、Group レベルが Merge になることで、Global → Group → Command の一貫したマージチェーンが実現される

### 2.2 非機能要件

#### NFR-1: 後方互換性

**影響を受ける既存の設定:**

1. **Group.from_env が未定義(nil)の場合**
   - 変更前: Global.from_env を継承
   - 変更後: Global.from_env を継承
   - → **互換性あり**

2. **Group.from_env が空配列 `[]` の場合**
   - 変更前: 何も取り込まない
   - 変更後: Global.from_env を継承
   - → **破壊的変更**（意図的に無効化していた設定が有効になる）

3. **Group.from_env が定義されている場合**
   - 変更前: Group.from_env のみ使用
   - 変更後: Global.from_env + Group.from_env をマージ
   - → **破壊的変更**（Global の変数も含まれるようになる）

**対応方針:**

- この変更は **破壊的変更** である
- ただし、以下の理由により許容可能:
  1. プロジェクトは開発段階であり、外部ユーザーはまだ少ない
  2. 変更により利便性が大幅に向上する
  3. Merge 方式の方が直感的で、ユーザーの期待に沿う
  4. 必要に応じて、Group レベルで明示的に変数を上書きすることで制御可能

#### NFR-2: パフォーマンス

- マップのマージ処理による性能への影響は negligible（微小）
- 既存の vars, env でも同様のマージを実施済み

#### NFR-3: テスト容易性

- 既存のテストケースを更新
- マージ動作を検証する新しいテストケースを追加

## 3. 実装への影響

### 3.1 影響を受けるコンポーネント

1. **internal/runner/config/expansion.go**
   - `ExpandGroupConfig` 関数の from_env 処理ロジック

2. **テストファイル**
   - `expansion_test.go`: Group レベルの from_env テスト
   - その他の統合テスト

3. **ドキュメント**
   - `docs/user/toml_config/05_group_level.ja.md`: from_env の説明
   - `docs/tasks/0033_vars_env_separation/01_requirements.md`: 継承ルールの記述

### 3.2 実装方針

- Override ロジックを削除し、Merge ロジックに置き換え
- nil と空配列 `[]` を同じ扱いにする（どちらも継承のみ）
- テストケースを TDD 方式で先に更新

## 4. 受け入れ基準

### AC-1: Group レベルでのマージ動作

- [ ] Global.from_env で定義された変数が Group で継承される
- [ ] Group.from_env で追加の変数を定義できる
- [ ] 同名の変数は Group の定義で上書きされる
- [ ] Group.from_env が nil または `[]` の場合、Global.from_env のみ継承される

### AC-2: Command レベルでの一貫性

- [ ] Global → Group → Command の一貫したマージチェーンが動作する

### AC-3: テスト

- [ ] 全ての既存テストがパスする（更新後）
- [ ] マージ動作を検証する新しいテストケースがパスする

### AC-4: ドキュメント

- [ ] ユーザー向けドキュメントが更新されている
- [ ] 継承ルールの説明が Merge 方式に対応している

## 5. リスクと軽減策

### リスク 1: 破壊的変更による既存設定への影響

**影響度:** 中
**発生確率:** 高

**軽減策:**

- リリースノートに破壊的変更として明記
- サンプル設定ファイルを更新
- 必要に応じて、マイグレーションガイドを作成

### リスク 2: 意図しない変数の混入

**影響度:** 低
**発生確率:** 低

**軽減策:**

- env_allowlist で既にフィルタ済み
- 必要に応じて Group レベルで明示的に上書き可能

## 6. 用語集

- **Override 方式**: 下位レベルで定義すると上位レベルの設定を完全に無視する方式
- **Merge 方式**: 上位レベルと下位レベルの設定を統合する方式
- **内部変数**: `%{VAR}` 構文で参照される変数
- **システム環境変数**: OS レベルの環境変数（`$ENV_VAR` 構文）

## 7. 関連ドキュメント

- [0033 vars/env 分離プロジェクト要件定義書](01_requirements.md)
- [ユーザー向け TOML 設定ドキュメント - Group レベル](../../user/toml_config/05_group_level.ja.md)
