# グループレベルコマンド許可リスト - 要件定義書

## 1. 概要

### 1.1 背景

現在、runner は `AllowedCommands` という正規表現パターンのリストで実行可能なコマンドを制限している。この制約はグローバルレベル（全グループ共通）でハードコーディングされており、特定のグループでのみ特別なコマンドを実行したい場合に柔軟性が不足している。

```toml
# 現在の設定（グローバルレベルでハードコーディング）
# "^/bin/.*", "^/usr/bin/.*" など

[[groups]]
name = "build"
# このグループでのみ /home/user/bin/special_tool を許可したいが、
# 現在の仕組みではソースコードを修正する必要があり、また修正すると
# 全グループで使えるようになってしまう
```

特定のユーザーが開発したツール（例：`/home/user/bin/custom_tool`）を特定のグループでのみ実行可能にしたいという要求がある。

### 1.2 目的

TOML 設定ファイルのグループレベルで、そのグループ専用の実行許可コマンドのリストを定義できるようにする。

### 1.3 スコープ

- **対象**: グループレベルの TOML 設定拡張、セキュリティバリデーション
- **実装範囲**:
  - GroupSpec への新フィールド追加
  - セキュリティバリデーションロジックの拡張
  - パス検証とセキュリティチェック
- **注**:
  - `AllowedCommands` の正規表現パターンマッチ機能は維持
  - `cmd_allowed` はパターンマッチをバイパスするのみで、他のセキュリティチェック（リスク評価、パス検証など）は引き続き実行される

## 2. 機能要件

### 2.1 TOML 設定フォーマット

#### F-001: グループレベルでの `cmd_allowed` フィールド追加

**概要**: `[[groups]]` セクションに `cmd_allowed` フィールドを追加し、絶対パスのリストを指定可能にする。

**TOML 例**:
```toml
[[groups]]
name = "build"
cmd_allowed = [
    "/home/user/bin/custom_build_tool",
    "/opt/myapp/bin/processor",
]

[[commands]]
name = "build"
cmd = "/home/user/bin/custom_build_tool"
args = ["--verbose"]
```

**制約**:
- `cmd_allowed` は文字列の配列
- 各要素は絶対パス（`/` で始まる）でなければならない
- 相対パスは拒否される
- 空配列 `[]` は許可される（何も追加しない）
- フィールド自体が省略された場合は `cmd_allowed` は適用されない

### 2.2 変数展開のサポート

#### F-002: `%{variable}` 形式の変数展開

**概要**: `cmd_allowed` のパス内で変数参照をサポートする。

**対応する変数**:
- `env_import` で取り込んだ環境変数（例：`%{home}` が `$HOME` から取り込まれた場合）
- `vars` で定義した変数（グローバル、グループ、コマンドの各レベルで定義可能）
- 自動生成される変数（例：`%{workdir}`, `%{config_dir}` など）

**TOML 例**:
```toml
[[groups]]
name = "build"
env_import = ["home=HOME"]
cmd_allowed = [
    "%{home}/bin/custom_tool",
    "%{workdir}/scripts/build.sh",
]
```

**変数展開のタイミング**: 設定ロード時（`config.Expander` によるグループ展開時）

### 2.3 セキュリティバリデーション

#### F-003: AllowedCommands との関係（OR条件）

**概要**: コマンドが実行可能と判定される条件は以下の **いずれか** を満たすこと:

1. グローバルレベルの `AllowedCommands` の正規表現パターンにマッチする
2. グループレベルの `cmd_allowed` リストに含まれる（完全一致）

**バリデーション順序**:
```
1. AllowedCommands のパターンマッチをチェック
   → マッチした場合: OK（処理終了）
   → マッチしなかった場合: 次へ

2. cmd_allowed のリストをチェック（該当グループに定義されている場合）
   → リストに含まれる場合: OK
   → リストに含まれない場合: エラー（ErrCommandNotAllowed）
```

**エラーメッセージ例**:
```
Error: command not allowed: /home/user/bin/foo
  - Not matched by allowed_commands patterns
  - Not in group-level cmd_allowed list
Available group-level commands: /home/user/bin/custom_tool, /opt/myapp/bin/processor
```

#### F-004: 絶対パスのバリデーション

**概要**: `cmd_allowed` に指定されたパスは絶対パスでなければならない。

**検証タイミング**: 設定ファイルロード時（変数展開後）

**エラーケース**:
- 相対パス: `bin/tool` → エラー
- 空文字列: `""` → エラー
- ホームディレクトリ省略記法: `~/bin/tool` → エラー

**エラーメッセージ例**:
```
Error: invalid cmd_allowed path in group 'build': 'bin/tool'
  cmd_allowed paths must be absolute (start with '/')
```

#### F-005: パス正規化とシンボリックリンク解決

**概要**: `cmd_allowed` で指定されたパスと、実行時のコマンドパスの両方を正規化して比較する。

**正規化処理**:
1. シンボリックリンクを解決（`filepath.EvalSymlinks` 相当）
2. パスの正規化（`filepath.Clean` 相当）
3. 正規化後のパスで完全一致比較

**例**:
```toml
cmd_allowed = ["/usr/local/bin/node"]
```

実行時のコマンド: `/usr/local/bin/node` → `/usr/bin/node` （シンボリックリンク）

- `cmd_allowed` の `/usr/local/bin/node` も `/usr/bin/node` に解決される
- 両方とも `/usr/bin/node` になるため、マッチする

#### F-006: 他のセキュリティチェックは継続

**概要**: `cmd_allowed` でパターンマッチをバイパスしても、以下のセキュリティチェックは引き続き実行される:

- リスク評価（`internal/runner/risk`）
- パス検証（シンボリックリンク攻撃防止など）
- ファイルパーミッション検証
- ハッシュ検証（`verify_files` に含まれる場合）
- 特権実行時の追加チェック

**動作**:
- `cmd_allowed` によって許可されたコマンドでも、他のセキュリティチェックで問題があれば実行は拒否される
- 例：`cmd_allowed` に含まれていても、ファイルが world-writable であれば拒否される

### 2.4 エラーハンドリング

#### F-007: 設定ファイルロード時のバリデーション

**検証項目**:
1. `cmd_allowed` の各パスが絶対パスであること（変数展開後）
2. パスに不正な文字が含まれていないこと
3. パスが最大長を超えていないこと（`MaxPathLength` 制約）

**エラーケース**:
```toml
[[groups]]
name = "test"
cmd_allowed = [
    "relative/path",  # 相対パス → エラー
    "",               # 空文字列 → エラー
]
```

**エラーメッセージ**:
```
Error loading config: group 'test' has invalid cmd_allowed paths:
  - 'relative/path': must be an absolute path
  - '': path cannot be empty
```

#### F-008: 実行時のコマンド許可チェック

**条件**: 実行するコマンドが以下のいずれの条件も満たさない場合:
- `AllowedCommands` パターンにマッチしない
- `cmd_allowed` リストに含まれない

**動作**: `ErrCommandNotAllowed` エラーを返す

**エラーメッセージ**:
```
Error: command not allowed: /home/user/bin/unknown_tool
  - Not matched by global allowed_commands patterns
  - Not in group 'build' cmd_allowed list: [/home/user/bin/custom_tool, /opt/myapp/bin/processor]
```

### 2.5 既存機能との互換性

#### F-009: 既存の設定ファイルとの互換性

**動作**: `cmd_allowed` フィールドが定義されていない既存の設定ファイルは、変更なしで動作する。

**デフォルト値**: `cmd_allowed` が省略された場合は、空リスト（何も追加しない）として扱われる。

#### F-010: AllowedCommands の動作維持

**動作**: `AllowedCommands` の正規表現パターンマッチは、従来通り機能する。

**例**:
```toml
allowed_commands = ["^/usr/bin/.*"]

[[groups]]
name = "test"
# cmd_allowed なし

[[commands]]
name = "run"
cmd = "/usr/bin/echo"  # AllowedCommands でマッチ → OK
```

### 2.6 ワイルドカードとグロブパターン

#### F-011: ワイルドカードのサポート無し

**動作**: `cmd_allowed` はワイルドカード（`*`, `?`）やグロブパターンをサポートしない。

**理由**: セキュリティリスクの増大を避けるため。

**例（サポートされない）**:
```toml
cmd_allowed = [
    "/usr/local/bin/*",      # サポートされない
    "/home/user/bin/tool?",  # サポートされない
]
```

**代替案**: 必要なコマンドを個別に列挙する。

## 3. 非機能要件

### 3.1 セキュリティ

#### NFR-1: パス検証の厳格性

**要件**: `cmd_allowed` に指定されたパスは、以下のセキュリティチェックを通過しなければならない:
- 絶対パスであること
- パストラバーサル攻撃（`../` など）を含まないこと
- 不正な文字を含まないこと

#### NFR-2: 最小権限の原則

**要件**: `cmd_allowed` でコマンドを許可しても、他のセキュリティレイヤー（リスク評価、パーミッション検証など）は維持される。

#### NFR-3: シンボリックリンク攻撃への対策

**要件**: パス比較時にシンボリックリンクを解決し、実際のファイルパスで比較する。

### 3.2 パフォーマンス

#### NFR-4: コマンド許可チェックのオーバーヘッド

**要件**: `cmd_allowed` のチェック追加によるオーバーヘッドは、1コマンドあたり1ms未満とする。

**実装ガイドライン**:
- `cmd_allowed` リストをマップに変換してO(1)ルックアップ
- シンボリックリンク解決結果をキャッシュ（同一コマンドの繰り返し実行時）

### 3.3 保守性

#### NFR-5: コードの明瞭性

**要件**:
- `cmd_allowed` のロジックは、既存の `AllowedCommands` チェックから明確に分離する
- バリデーション処理は `security.Validator` にカプセル化する

#### NFR-6: テストカバレッジ

**要件**: 以下のテストを実装する:
- ユニットテスト: `cmd_allowed` のパース、変数展開、バリデーション
- ユニットテスト: コマンド許可チェックの各分岐
- 統合テスト: 実際の TOML ファイルを使用したエンドツーエンドテスト
- セキュリティテスト: パストラバーサル、シンボリックリンク攻撃の防止

### 3.4 ユーザビリティ

#### NFR-7: エラーメッセージの明瞭性

**要件**: エラーが発生した場合、以下の情報を含むメッセージを提供する:
- 実行しようとしたコマンドのパス
- 拒否された理由（AllowedCommands 不一致、cmd_allowed 不一致、など）
- 該当グループの `cmd_allowed` リスト

#### NFR-8: ドキュメント

**要件**:
- 設定ファイルの例を含むユーザーガイド
- 変数展開の例
- セキュリティ上の注意事項

## 4. 制約事項

### 4.1 技術的制約

- Go 1.23.10 以上
- 既存の `security.Validator` インターフェースを拡張
- 既存の `config.Expander` による変数展開を利用

### 4.2 設計制約

- 絶対パスのみサポート（相対パス不可）
- ワイルドカード・グロブパターン不可
- `cmd_allowed` は OR 条件（AllowedCommands と併用）

### 4.3 互換性制約

- 既存の TOML 設定ファイルは変更なしで動作
- `AllowedCommands` の動作は変更しない

## 5. データ構造

### 5.1 GroupSpec への追加

```go
type GroupSpec struct {
    // ... 既存フィールド ...

    // CmdAllowed はこのグループで実行を許可する追加コマンドのリスト
    // 各要素は絶対パス（変数展開前）
    // 空の場合、グループレベルの追加許可は行わない
    CmdAllowed []string `toml:"cmd_allowed"`
}
```

### 5.2 RuntimeGroup への追加

```go
type RuntimeGroup struct {
    // ... 既存フィールド ...

    // ExpandedCmdAllowed は変数展開後の許可コマンドリスト
    // シンボリックリンク解決済みの正規化されたパス
    ExpandedCmdAllowed []string
}
```

### 5.3 Validator インターフェース拡張

```go
type ValidatorInterface interface {
    // ... 既存メソッド ...

    // ValidateCommandAllowed は、コマンドが実行許可されているかを検証する
    // AllowedCommands パターンまたは groupCmdAllowed リストをチェックする
    //
    // Parameters:
    //   - cmdPath: 実行するコマンドのパス（変数展開済み）
    //   - groupCmdAllowed: グループレベルの許可コマンドリスト（空の場合は nil）
    //
    // Returns:
    //   - error: 許可されていない場合は ErrCommandNotAllowed
    ValidateCommandAllowed(cmdPath string, groupCmdAllowed []string) error
}
```

## 6. 成功基準

### 6.1 機能の完全性

- [ ] `cmd_allowed` フィールドを TOML から読み込める
- [ ] 変数展開（`%{variable}` 形式）が正しく動作する
- [ ] 絶対パスのバリデーションが動作する
- [ ] AllowedCommands と cmd_allowed の OR 条件が正しく動作する
- [ ] シンボリックリンク解決が正しく動作する
- [ ] 他のセキュリティチェックがバイパスされない

### 6.2 エラーハンドリング

- [ ] 相対パスを指定した場合にエラーになる
- [ ] 許可されていないコマンドを実行しようとした場合にエラーになる
- [ ] エラーメッセージに十分な情報が含まれる

### 6.3 互換性

- [ ] 既存の設定ファイル（`cmd_allowed` なし）が動作する
- [ ] AllowedCommands の動作が変わらない

### 6.4 テストカバレッジ

- [ ] ユニットテスト: TOML パース
- [ ] ユニットテスト: 変数展開
- [ ] ユニットテスト: パスバリデーション
- [ ] ユニットテスト: コマンド許可チェック（AllowedCommands ヒット）
- [ ] ユニットテスト: コマンド許可チェック（cmd_allowed ヒット）
- [ ] ユニットテスト: コマンド許可チェック（両方ミス → エラー）
- [ ] 統合テスト: 実際の TOML ファイルを使用
- [ ] セキュリティテスト: パストラバーサル防止
- [ ] セキュリティテスト: シンボリックリンク攻撃防止

### 6.5 ドキュメント

- [ ] ユーザーガイドに `cmd_allowed` の説明を追加
- [ ] 設定例を追加
- [ ] セキュリティ上の注意事項を記載

## 7. 想定される利用シナリオ

### 7.1 ユーザーのホームディレクトリ内のツール

開発者が `$HOME/bin` に配置したカスタムツールを、特定のグループでのみ使用する。

```toml
[[groups]]
name = "build"
env_import = ["home=HOME"]
cmd_allowed = [
    "%{home}/bin/custom_build_tool",
]

[[commands]]
name = "build"
cmd = "%{home}/bin/custom_build_tool"
args = ["--config", "build.yaml"]
```

### 7.2 プロジェクト固有のスクリプト

プロジェクトディレクトリ内のスクリプトを実行する。

```toml
[[groups]]
name = "deploy"
workdir = "/opt/myapp"
cmd_allowed = [
    "/opt/myapp/scripts/deploy.sh",
    "/opt/myapp/scripts/healthcheck.sh",
]

[[commands]]
name = "deploy"
cmd = "/opt/myapp/scripts/deploy.sh"
args = ["--env", "production"]
```

### 7.3 複数グループで異なるツールセット

グループごとに異なるツールセットを許可する。

```toml
[[groups]]
name = "build_backend"
cmd_allowed = [
    "/opt/go/bin/go",
    "/opt/protoc/bin/protoc",
]

[[groups]]
name = "build_frontend"
cmd_allowed = [
    "/opt/node/bin/node",
    "/opt/node/bin/npm",
]
```

## 8. 実装の考慮事項

### 8.1 実装フェーズ

**Phase 1: データ構造とパース**
- `GroupSpec` に `CmdAllowed` フィールド追加
- `RuntimeGroup` に `ExpandedCmdAllowed` フィールド追加
- TOML パースの実装

**Phase 2: 変数展開**
- `config.Expander` での `cmd_allowed` 変数展開
- 絶対パスバリデーション

**Phase 3: セキュリティバリデーション**
- `security.Validator` の拡張
- AllowedCommands と cmd_allowed の OR 条件実装
- シンボリックリンク解決

**Phase 4: エラーハンドリング**
- エラーメッセージの改善
- デバッグ情報の追加

**Phase 5: テストとドキュメント**
- ユニットテスト作成
- 統合テスト作成
- ユーザーガイド更新

### 8.2 既存コードとの統合ポイント

- **TOML パース**: `internal/runner/config/loader.go`
- **変数展開**: `internal/runner/config/expansion.go` (`Expander.ExpandGroup`)
- **セキュリティ検証**: `internal/runner/security/validator.go`
- **コマンド実行**: `internal/runner/executor/executor.go`

### 8.3 パフォーマンス最適化

- `cmd_allowed` リストをマップに変換（O(1) ルックアップ）
- シンボリックリンク解決結果のキャッシュ
- 正規化されたパスの事前計算

## 9. 今後の拡張可能性

### 9.1 将来的な機能追加

以下の機能は現時点では対象外だが、将来的に検討可能:

- **グロブパターンサポート**: `/opt/myapp/bin/*` のようなパターン（セキュリティリスクとのトレードオフを慎重に評価）
- **コマンドレベルの `cmd_allowed`**: グループだけでなく、個別のコマンドでも指定可能に
- **継承制御**: グローバル設定の `allowed_commands` を完全に上書きするモード

### 9.2 セキュリティ監査ログ

`cmd_allowed` によって許可されたコマンドの実行をセキュリティ監査ログに記録:

```json
{
  "event": "command_allowed_via_group_cmd_allowed",
  "group": "build",
  "command": "/home/user/bin/custom_tool",
  "matched_entry": "/home/user/bin/custom_tool"
}
```

## 10. 参照

- セキュリティ設定: `internal/runner/security/types.go` (`Config.AllowedCommands`)
- グループ定義: `internal/runner/runnertypes/spec.go` (`GroupSpec`)
- 変数展開: `internal/runner/config/expansion.go` (`Expander`)
- パス検証: `internal/safefileio/path_validation.go`

---

**文書バージョン**: 1.0
**作成日**: 2025-11-25
**承認日**: 2025-11-25
**次回レビュー予定**: [実装完了後]
