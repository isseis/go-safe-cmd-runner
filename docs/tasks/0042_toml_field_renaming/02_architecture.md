# アーキテクチャ設計書: TOML設定フィールド名の改善

## 1. 概要

### 1.1 目的

TOML設定ファイルのフィールド名を改善し、以下を達成する：

- **直感性の向上**: positive sentenceによる分かりやすい命名
- **一貫性の確立**: 関連フィールドのプレフィックス統一
- **明確性の向上**: フィールドの役割が名前から明確に理解できる
- **セキュアデフォルト**: デフォルト値を安全側に変更

### 1.2 設計原則

1. **Positive Sentence First**: 否定形（`skip_`）より肯定形（`verify_`）を優先
2. **Prefix Consistency**: 関連フィールドは統一されたプレフィックスを使用
3. **Natural Word Order**: 自然な英語の語順を採用
4. **Explicit Suffixes**: `_limit`, `_file` など目的を明確にするサフィックス使用
5. **Secure by Default**: セキュリティ関連フィールドのデフォルトは安全側に

### 1.3 Breaking Change

この変更は **breaking change** として扱う。後方互換性は提供しない。

## 2. アーキテクチャ概要

### 2.1 影響範囲

```
┌─────────────────┐
│  TOML Files     │  ← ユーザーが記述（要更新）
│  (sample/*.toml)│
└────────┬────────┘
         │
         ↓ Parse
┌─────────────────────────────────────────┐
│  Spec Layer (internal/runner/runnertypes)│
│  ├─ ConfigSpec                           │
│  ├─ GlobalSpec    ← TOMLタグ変更        │
│  ├─ GroupSpec     ← TOMLタグ変更        │
│  └─ CommandSpec   ← TOMLタグ変更        │
└────────┬────────────────────────────────┘
         │
         ↓ Expand
┌─────────────────────────────────────────┐
│  Runtime Layer                           │
│  ├─ RuntimeGlobal                        │
│  ├─ RuntimeGroup                         │
│  └─ RuntimeCommand                       │
│  (構造体フィールド名は変更なし)          │
└─────────────────────────────────────────┘
         │
         ↓ Execute
┌─────────────────┐
│  Command        │
│  Execution      │
└─────────────────┘
```

### 2.2 レイヤー別影響

#### Spec Layer (変更あり)
- `GlobalSpec`, `GroupSpec`, `CommandSpec` の TOML タグを変更
- デフォルト値の変更: `verify_standard_paths` は `true` に
- 構造体フィールド名は内部的に変更（Goの命名規則に従う）

#### Runtime Layer (変更なし)
- `RuntimeGlobal`, `RuntimeGroup`, `RuntimeCommand` は変更なし
- Spec から Runtime への変換ロジックは、フィールド名の対応を更新するのみ

#### Parser/Loader (変更最小)
- TOML パーサーは自動的に新しいタグを認識
- 旧フィールド名のサポートは削除（breaking change）

## 3. コンポーネント設計

### 3.1 フィールド名マッピング

#### 3.1.1 Global レベル

| カテゴリ | 旧フィールド名 | 新フィールド名 | TOML タグ | デフォルト値変更 |
|---------|--------------|--------------|-----------|----------------|
| セキュリティ | `SkipStandardPaths` | `VerifyStandardPaths` | `verify_standard_paths` | `false` → `true` |
| 環境変数 | `Env` | `EnvVars` | `env_vars` | なし |
| 環境変数 | `EnvAllowlist` | `EnvAllowed` | `env_allowed` | なし |
| 環境変数 | `FromEnv` | `EnvImport` | `env_import` | なし |
| 出力 | `MaxOutputSize` | `OutputSizeLimit` | `output_size_limit` | なし |

#### 3.1.2 Group レベル

| カテゴリ | 旧フィールド名 | 新フィールド名 | TOML タグ |
|---------|--------------|--------------|-----------|
| 環境変数 | `Env` | `EnvVars` | `env_vars` |
| 環境変数 | `EnvAllowlist` | `EnvAllowed` | `env_allowed` |
| 環境変数 | `FromEnv` | `EnvImport` | `env_import` |

#### 3.1.3 Command レベル

| カテゴリ | 旧フィールド名 | 新フィールド名 | TOML タグ | デフォルト値 |
|---------|--------------|--------------|-----------|-------------|
| 環境変数 | `Env` | `EnvVars` | `env_vars` | なし |
| 環境変数 | `FromEnv` | `EnvImport` | `env_import` | なし |
| リスク管理 | `MaxRiskLevel` | `RiskLevel` | `risk_level` | `"low"` (変更なし) |
| 出力 | `Output` | `OutputFile` | `output_file` | なし |

### 3.2 命名規則の確立

#### 3.2.1 環境変数関連フィールド

**プレフィックス**: `env_`

```
env_vars      ← 環境変数の設定（KEY=VALUE形式の配列）
env_allowed   ← 許可する環境変数リスト
env_import    ← システム環境変数からのインポート
```

**設計根拠**:
- 関連するフィールドをグループ化
- 検索・補完時の利便性向上
- 将来の拡張性（例: `env_export`, `env_filter` など）

#### 3.2.2 検証関連フィールド

**プレフィックス**: `verify_`

```
verify_files          ← ファイル検証リスト
verify_standard_paths ← 標準パスの検証実施フラグ
```

**設計根拠**:
- 肯定形で動作が明確
- セキュリティ関連機能のグループ化

#### 3.2.3 出力関連フィールド

**プレフィックス**: `output_`

```
output_file       ← 出力先ファイルパス
output_size_limit ← 出力サイズ制限
```

**設計根拠**:
- 自然な語順（"output" + 属性）
- `_limit` サフィックスで制約であることを明示

#### 3.2.4 リスク管理フィールド

**命名**: `risk_level`

**設計根拠**:
- ユーザー視点では「このコマンドのリスクレベル」を指定
- `max_` プレフィックスは内部実装の詳細
- より直感的でシンプル

### 3.3 デフォルト値の変更

#### `verify_standard_paths`

**変更内容**:
- 旧: `skip_standard_paths = false` （デフォルトで検証しない）
- 新: `verify_standard_paths = true` （デフォルトで検証する）

**設計根拠**:
- **Secure by Default**: セキュリティ機能はデフォルトで有効
- **Fail-Safe**: 検証を明示的に無効化しない限り実行される
- **Defence in Depth**: 多層防御の一環として標準パス検証を推奨

**影響**:
- 既存の設定ファイルで `skip_standard_paths` を省略していた場合、動作が変わる
- 検証をスキップしたいユーザーは明示的に `verify_standard_paths = false` を記述する必要がある

**移行パス**:
```toml
# 旧設定（検証をスキップしていた）
[global]
skip_standard_paths = true

# 新設定（等価）
[global]
verify_standard_paths = false
```

## 4. データフロー

### 4.1 パース時のデータフロー

```
TOML File
  ↓
[toml.Unmarshal]
  ↓
Spec structs (新フィールド名)
  ├─ GlobalSpec.VerifyStandardPaths (bool)
  ├─ GlobalSpec.EnvVars ([]string)
  ├─ GlobalSpec.EnvAllowed ([]string)
  ├─ GlobalSpec.EnvImport ([]string)
  ├─ GlobalSpec.OutputSizeLimit (int64)
  ├─ GroupSpec.EnvVars ([]string)
  ├─ GroupSpec.EnvAllowed ([]string)
  ├─ GroupSpec.EnvImport ([]string)
  ├─ CommandSpec.EnvVars ([]string)
  ├─ CommandSpec.EnvImport ([]string)
  ├─ CommandSpec.RiskLevel (string)
  └─ CommandSpec.OutputFile (string)
```

### 4.2 展開時のデータフロー

```
Spec structs (immutable)
  ↓
[config.ExpandGlobal/ExpandGroup/ExpandCommand]
  ↓
Runtime structs (mutable)
  ├─ RuntimeGlobal.ExpandedEnv (map[string]string)
  ├─ RuntimeGroup.ExpandedEnv (map[string]string)
  └─ RuntimeCommand.ExpandedEnv (map[string]string)
```

**注**: Runtime層の構造体フィールド名は変更しない。
Spec から Runtime への変換ロジックのみを更新。

## 5. セキュリティ設計

### 5.1 セキュアデフォルト

#### `verify_standard_paths` のデフォルト変更

**目的**: 標準パスの検証をデフォルトで有効化

**効果**:
- `/bin`, `/usr/bin` などの標準パスのファイル改ざん検出
- 攻撃者による重要コマンドの置き換えを防止
- システム整合性の自動検証

**トレードオフ**:
- 初回実行時のハッシュ生成に時間がかかる可能性
- 誤検知のリスク（正当なシステムアップデート時）

**推奨設定**:
```toml
[global]
# 本番環境: 検証を有効化（デフォルト）
verify_standard_paths = true

# 開発環境: 検証をスキップ（パフォーマンス優先）
# verify_standard_paths = false
```

### 5.2 環境変数の一貫した管理

新しい命名規則により、環境変数関連のセキュリティ設定が統一される：

```toml
[global]
env_allowed = ["PATH", "HOME", "USER"]  # 許可リスト
env_vars = ["LANG=en_US.UTF-8"]         # 明示的設定
env_import = ["user=USER"]              # システムからインポート
```

**効果**:
- 環境変数の流れが明確
- 設定の見落としが減少
- セキュリティレビューが容易

## 6. 互換性とマイグレーション

### 6.1 Breaking Change の扱い

**方針**: 後方互換性を提供しない

**理由**:
- 中間状態（新旧両方をサポート）のメンテナンスコストが高い
- 設定ファイルの数が限定的（主にサンプルとテスト）
- 明確な移行パスを提供することで対応可能

### 6.2 移行支援

#### ドキュメント提供
- `CHANGELOG.md` に Breaking Change セクションを追加
- フィールド名対応表を提供
- 移行例を記載

#### エラーメッセージ
旧フィールド名を検出した場合のエラーメッセージ案：

```
Error: Unknown field 'skip_standard_paths' in [global] section.
Did you mean 'verify_standard_paths'?

Note: Field names have changed in version X.X.X.
See CHANGELOG.md for the migration guide.
```

**実装**: TOML パーサーの標準エラーに依存（カスタムエラーハンドリングは不要）

### 6.3 移行チェックリスト

ユーザー向けの移行チェックリスト：

- [ ] `CHANGELOG.md` でフィールド名対応表を確認
- [ ] 既存の TOML ファイルをすべて更新
- [ ] `verify_standard_paths` のデフォルト値変更を理解
- [ ] テスト実行で動作確認
- [ ] ドキュメントを確認（新しいフィールド名に対応）

## 7. テスト戦略

### 7.1 ユニットテスト

#### Spec層のテスト
- 新フィールド名でのパーステスト
- デフォルト値のテスト（`verify_standard_paths = true`）
- 無効な旧フィールド名の検出テスト

#### Runtime層のテスト
- Spec → Runtime 変換のテスト
- フィールドマッピングの正確性テスト

### 7.2 インテグレーションテスト

- 新フィールド名を使用した E2E テスト
- サンプル設定ファイルでの動作確認
- デフォルト値変更の影響確認

### 7.3 ドキュメントテスト

- すべてのドキュメント内のコード例が新フィールド名を使用
- サンプルファイルが新フィールド名を使用
- リンク切れがないことを確認

## 8. ドキュメント戦略

### 8.1 更新対象ドキュメント

#### ユーザーガイド トップレベル
- `docs/user/README.md` / `docs/user/README.ja.md`

#### ユーザーガイド詳細（`docs/user/toml_config/`）
- `04_global_level.md` / `04_global_level.ja.md`
- `05_group_level.md` / `05_group_level.ja.md`
- `06_command_level.md` / `06_command_level.ja.md`
- `07_variable_expansion.md` / `07_variable_expansion.ja.md`
- `08_practical_examples.md` / `08_practical_examples.ja.md`
- `09_best_practices.md` / `09_best_practices.ja.md`
- `10_troubleshooting.md` / `10_troubleshooting.ja.md`

#### サンプル設定（`sample/`）
- すべての `.toml` ファイル

#### 変更履歴
- `CHANGELOG.md`: Breaking Change セクション

### 8.2 ドキュメント更新の原則

1. **一貫性**: すべてのコード例で新フィールド名を使用
2. **完全性**: 対応表を含め、移行を容易にする
3. **説明**: 変更理由と影響を明記
4. **例示**: Before/After の例を提供

## 9. リスク管理

### 9.1 識別されたリスク

| リスク | 影響度 | 発生確率 | 対策 |
|--------|--------|---------|------|
| 既存設定ファイルの破損 | 高 | 高 | CHANGELOG.mdで明確に通知 |
| デフォルト値変更による動作変更 | 中 | 中 | ドキュメントで明示、セキュリティ向上を強調 |
| ドキュメントの更新漏れ | 中 | 低 | チェックリストによる確認 |
| ユーザーの混乱 | 中 | 中 | 移行ガイドと対応表を提供 |

### 9.2 リスク軽減策

#### 既存設定ファイルの破損
- **予防**: CHANGELOG.md で目立つように Breaking Change を記載
- **検出**: テスト実行で即座に検出可能
- **対処**: フィールド名対応表を提供

#### デフォルト値変更
- **予防**: デフォルト値変更を CHANGELOG.md で明示
- **検出**: 既存の動作と異なる場合はログで確認可能
- **対処**: 必要に応じて `verify_standard_paths = false` を明示

## 10. 今後の拡張性

### 10.1 命名規則の適用

確立された命名規則により、将来の拡張が容易になる：

#### 環境変数関連
- `env_export`: 環境変数のエクスポート設定
- `env_filter`: 環境変数のフィルタリングルール
- `env_template`: 環境変数のテンプレート

#### 検証関連
- `verify_checksums`: チェックサム検証の有効化
- `verify_signatures`: デジタル署名検証の有効化
- `verify_permissions`: パーミッション検証の有効化

#### 出力関連
- `output_format`: 出力フォーマット指定
- `output_encoding`: 出力エンコーディング指定
- `output_redirect`: 出力リダイレクト設定

### 10.2 設計原則の継続

今後の機能追加時も以下の原則を維持：

1. **Positive Sentence First**: 肯定形を優先
2. **Prefix Consistency**: 関連フィールドのプレフィックス統一
3. **Natural Word Order**: 自然な語順
4. **Explicit Suffixes**: 明示的なサフィックス使用
5. **Secure by Default**: セキュアなデフォルト値

## 11. 実装の優先順位

### 11.1 Phase 1: コア変更（高優先度）

1. Spec層の構造体フィールド名とTOMLタグ変更
2. デフォルト値の変更実装
3. 基本的なユニットテスト

### 11.2 Phase 2: テストとドキュメント（高優先度）

1. インテグレーションテストの更新
2. サンプルファイルの更新
3. CHANGELOG.md の作成

### 11.3 Phase 3: ドキュメント完全対応（中優先度）

1. ユーザーガイドの更新（日英両方）
2. 移行ガイドの作成
3. ドキュメントの整合性確認

## 12. 成功基準

- [ ] すべてのSpec構造体フィールドが新命名規則に従っている
- [ ] `verify_standard_paths` のデフォルトが `true` になっている
- [ ] すべてのユニットテストがパスする
- [ ] すべてのインテグレーションテストがパスする
- [ ] サンプル設定ファイルが新フィールド名を使用している
- [ ] CHANGELOG.md に Breaking Change が記載されている
- [ ] フィールド名対応表が作成されている
- [ ] ユーザーガイドが更新されている（日英両方）
- [ ] コード例がすべて新フィールド名を使用している

## 13. 参考資料

- Task 0035: Spec/Runtime Separation（構造体分離の前提）
- Task 0031: Global・Groupレベル環境変数設定機能
- TOML v1.0.0 Specification
- Go struct tag conventions
