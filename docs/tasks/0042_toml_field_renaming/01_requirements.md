# タスク 0042: TOML設定フィールド名の改善

## 1. 背景

### 1.1 現状の問題点

TOML設定ファイルのフィールド名に以下の問題が存在する：

1. **否定形の使用**: `skip_standard_paths` が否定形で直感的でない
2. **命名の一貫性欠如**: 環境変数関連フィールドが `env_` プレフィックスで統一されていない
   - `env`: プレフィックスなし
   - `env_allowlist`: `env_` プレフィックスあり
   - `from_env`: プレフィックスなし
3. **仕様変更による名称の不適合**: 初期設計時からの仕様変更により、フィールド名が現在の動作を正確に表現していない
4. **冗長または不明瞭な命名**: 一部のフィールド名が冗長または意味が不明瞭

### 1.2 影響範囲

- ユーザーが記述するTOML設定ファイル
- 内部データ構造（`GlobalSpec`, `GroupSpec`, `CommandSpec`）
- ドキュメント（ユーザーガイド、サンプル設定）

**注**: 後方互換性は提供しない。既存の設定ファイルは手動で更新する必要がある。

### 1.3 改善の目的

- **直感性の向上**: positive sentenceによる分かりやすい命名
- **一貫性の確立**: 関連フィールドのプレフィックス統一
- **明確性の向上**: フィールドの役割が名前から明確に理解できる
- **メンテナンス性向上**: 将来の拡張時に命名規則に従いやすい

## 2. 要件

### 2.1 フィールド名変更一覧

| カテゴリ | 現在の名前 | 新しい名前 | レベル | 優先度 |
|---------|-----------|-----------|--------|--------|
| セキュリティ | `skip_standard_paths` | `verify_standard_paths` | Global | 高 |
| 環境変数 | `env` | `env_vars` | Global, Group, Command | 高 |
| 環境変数 | `env_allowlist` | `env_allowed` | Global, Group | 高 |
| 環境変数 | `from_env` | `env_import` | Global, Group, Command | 高 |
| リスク管理 | `max_risk_level` | `risk_level` | Command | 中 |
| 出力 | `output` | `output_file` | Command | 低 |
| 出力 | `max_output_size` | `output_size_limit` | Global | 低 |

### 2.2 各フィールド変更の詳細と議論

#### 2.2.1 `skip_standard_paths` → `verify_standard_paths`

**変更理由**:
- 否定形（skip）から肯定形（verify）への変更
- デフォルト値を `true`（検証する）にすることで、安全側に倒す

**議論**:
- 現状: `skip_standard_paths = true` で標準パスの検証をスキップ（安全性低）
- 変更後: `verify_standard_paths = true` で標準パスを検証（安全性高）
- デフォルト動作の変更を伴うため、移行時の注意が必要

**デフォルト値**: `true`（検証を実施、セキュアデフォルト）

#### 2.2.2 `env` → `env_vars`

**変更理由**:
- 環境変数関連フィールドの `env_` プレフィックス統一
- `env` が "environment" なのか "environment variables" なのか曖昧
- 複数形 `vars` で配列であることを示す

**議論**:
- 他の候補: `env_variables`（冗長）
- 選択理由: 簡潔さと明確性のバランス

**影響範囲**: Global, Group, Command の3レベル

#### 2.2.3 `env_allowlist` → `env_allowed`

**変更理由**:
- 環境変数関連フィールドの `env_` プレフィックス維持
- より簡潔な表現

**議論**:
- 他の候補:
  - `allowed_env_vars`: 配列であることがより明確だが冗長
  - `env_var_allowlist`: 現状に近いが冗長
- 選択理由: `env_allowed` は簡潔で、型定義で配列であることは明確

**影響範囲**: Global, Group

#### 2.2.4 `from_env` → `env_import`

**変更理由**:
- 環境変数関連フィールドの `env_` プレフィックス統一
- `from_env` の「from」は方向性を示すが、`import` で動作が明確

**議論**:
- 他の候補:
  - `env_imported`: 過去形で「既にimportされた」と誤解される可能性
  - `env_import_mapping`: mapping であることが明確だが冗長
  - `import_from_env`: 動詞+前置詞で意図が明確だがプレフィックス不統一
- 選択理由: プレフィックス統一と簡潔性のバランス

**形式**: `"internal_var_name=SYSTEM_ENV_VAR_NAME"` 形式（変更なし）

**影響範囲**: Global, Group, Command

#### 2.2.5 `max_risk_level` → `risk_level`

**変更理由**:
- ユーザー視点では「このコマンドのリスクレベル」を記述している
- 内部的には「許容最大値」だが、結果的に同じ意味として機能

**議論**:
- 他の候補:
  - `allowed_risk_level`: 「許容される」が明示的
  - `max_allowed_risk_level`: 非常に明確だが冗長
  - `risk_limit`: 簡潔だが「制限」というニュアンスが強い
- 選択理由: ユーザーの認知モデルに合致し、最も自然

**実装詳細**:
- runner が自動的にコマンドのリスクレベルを評価
- ユーザーが `risk_level` に指定した値と比較して実行可否を判定
- `risk_level` 未指定時のデフォルト値: `"low"`
  - つまり、`risk_level` を省略したコマンドは低リスクコマンドのみ実行可能

**デフォルト動作の例**:
```toml
[[groups.commands]]
name = "example"
cmd = "/bin/ls"
# risk_level 未指定 → デフォルト "low" が適用される
# runner がこのコマンドを "medium" 以上と評価した場合は実行拒否
```

**影響範囲**: Command のみ

#### 2.2.6 `output` → `output_file`

**変更理由**:
- `output` が曖昧（出力先なのか、出力内容なのか）
- `output_file` で「ファイルパス」であることが明確

**議論**:
- 他の候補:
  - `output_capture_file`: 明確だが冗長
  - `stdout_file`: 技術的に正確だが専門的
- 選択理由: 簡潔さと明確性のバランス

**影響範囲**: Command のみ

#### 2.2.7 `max_output_size` → `output_size_limit`

**変更理由**:
- より自然な語順（"output size" の "limit"）
- `max_` プレフィックスの統一性がないため、`_limit` サフィックスに変更

**議論**:
- 他の候補: `output_max_size`（現状に近い）
- 選択理由: "size limit" の方が自然な英語表現

**影響範囲**: Global のみ

### 2.3 命名規則の確立

変更後の命名規則：

#### 環境変数関連フィールド
- **プレフィックス**: `env_`
- **例**: `env_vars`, `env_allowed`, `env_import`

#### 検証関連フィールド
- **プレフィックス**: `verify_`
- **例**: `verify_files`, `verify_standard_paths`

#### 出力関連フィールド
- **プレフィックス**: `output_`
- **例**: `output_file`, `output_size_limit`

#### その他
- **肯定形を優先**: `verify_` (not `skip_`)
- **自然な語順**: `output_size_limit` (not `max_output_size`)
- **サフィックス**: `_limit`, `_file` など目的を明確に

### 2.4 Breaking Change の扱い

**方針**: 後方互換性は提供せず、breaking change として扱う。

#### ドキュメント要件
- CHANGELOG.md に breaking change として明記
- 旧フィールド名から新フィールド名への対応表を提供
- 各フィールドの変更理由を簡潔に説明
- サンプル設定ファイルを新フィールド名に更新

### 2.5 デフォルト値の変更

| フィールド | 旧デフォルト | 新デフォルト | 理由 |
|-----------|------------|------------|------|
| `skip_standard_paths` → `verify_standard_paths` | `false` (検証する) | `true` (検証する) | フィールド名の反転に伴う値の変更。動作は変わらず検証を継続 |

**重要**: `skip_standard_paths` のデフォルト `false` は「スキップしない = 検証する」を意味していました。新しい `verify_standard_paths` のデフォルト `true` は「検証する」を意味します。**デフォルトの動作は変わらず、標準パスの検証は引き続き実行されます**。変更はフィールド名の分かりやすさの向上のみです。

その他のフィールドはデフォルト値変更なし。

### 2.6 実装要件

#### データ構造
- `GlobalSpec`, `GroupSpec`, `CommandSpec` の TOML タグを新フィールド名に更新
- 旧フィールド名のサポートは削除

#### テスト
- 新フィールド名でのユニットテスト
- 新フィールド名でのインテグレーションテスト
- デフォルト値変更のテスト（`verify_standard_paths`）

#### ドキュメント更新

**ユーザーガイド トップレベル**:
- `docs/user/README.md`, `docs/user/README.ja.md`: ユーザーガイドのトップページ、サンプルコードを更新

**ユーザーガイド詳細**: `docs/user/toml_config/` 以下の全ドキュメント（日英両方）

**サンプル設定ファイル**: `sample/` ディレクトリ内のすべての TOML ファイル

**変更履歴**:
- `CHANGELOG.md`: Breaking change セクションに記載
- フィールド名対応表を含める

## 3. 非機能要件

### 3.1 ドキュメント品質
- CHANGELOG.md で breaking change を明確に記載
- フィールド名対応表を提供
- 変更理由を簡潔に説明
- ユーザー向けドキュメント（特に `docs/user/toml_config/` 以下）を新仕様に完全対応
- すべてのサンプルコードとコード例が新フィールド名を使用

### 3.2 保守性
- 新しい命名規則を文書化
- 将来の拡張時に規則に従いやすい

### 3.3 テスト品質
- すべての新フィールド名でテストを実施
- デフォルト値変更のテストを含める

## 4. 制約事項

### 4.1 技術的制約
- TOML パーサーの制限内で実装
- Go の struct tag による制約

### 4.2 Breaking Change 制約
- 後方互換性は提供しない
- 既存ユーザーは手動で設定ファイルを更新する必要がある

## 5. 想定されるリスクと対策

### 5.1 リスク: 既存ユーザーの設定ファイル破損
- **影響**: 旧フィールド名を使用している設定ファイルが動作しなくなる
- **対策**: CHANGELOG.md で明確に breaking change として通知
- **対策**: フィールド名対応表を提供して手動更新を支援

### 5.2 リスク: デフォルト値変更による動作変更
- **影響**: `verify_standard_paths` のデフォルトが `false` → `true` に変更
- **対策**: CHANGELOG.md で明示
- **対策**: セキュリティ向上のため、変更は正当化される

## 6. 成功基準

- [ ] すべての新フィールド名が実装されている
- [ ] すべてのドキュメントが更新されている
- [ ] ユニットテストとインテグレーションテストがパスする
- [ ] サンプル設定ファイルが新フィールド名を使用している
- [ ] CHANGELOG.md に breaking change が記載されている
- [ ] フィールド名対応表が作成されている

## 7. 次のステップ

1. アーキテクチャ設計書の作成（02_architecture.md）
2. 詳細仕様書の作成（03_specification.md）
3. 実装計画書の作成（04_implementation_plan.md）
4. 実装とテスト
5. ドキュメント更新
6. リリース準備
