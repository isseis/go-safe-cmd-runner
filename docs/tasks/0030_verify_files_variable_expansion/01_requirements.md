# verify_files フィールドにおける環境変数展開機能 - 要件定義書

> **重要**: Task 0031 (Global/Group Level Environment Variables) の実装により、本ドキュメントで規定された仕様が拡張されました。
> - `global.env` および `groups[].env` フィールドが追加され、verify_files の展開で参照可能になりました
> - 詳細は [Task 0031 要件定義書](../0031_global_group_env/01_requirements.md) のセクション 2.5 (F008) を参照してください

## 1. 概要

### 1.1 背景

現在、runner は TOML 設定ファイル中の `cmd` および `args` フィールドに含まれる環境変数を展開する機能を提供している（タスク 0026 で実装）。しかし、`verify_files` フィールドに含まれる環境変数は展開されない。

```toml
[global]
verify_files = [
    "/usr/bin/python3",           # 絶対パス（現在動作）
    "${HOME}/bin/my_script.sh"    # 環境変数（現在展開されない）
]

[[groups]]
name = "example"
verify_files = [
    "${HOME}/verify.sh"      # システム環境変数（現在展開されない）
]
```

ユーザー環境やグループ固有の環境変数を使用してファイルパスを指定できるようにすることで、設定の柔軟性と再利用性が向上する。

### 1.2 目的

`verify_files` フィールドにおいても環境変数展開機能を提供し、`cmd` および `args` と同様の柔軟性を実現する。

### 1.3 スコープ

- **対象**: グローバルレベルおよびグループレベルの `verify_files` フィールド
- **実装範囲**: 環境変数展開、allowlist によるフィルタリング、エラーハンドリング
- **注**: コマンドレベルには `verify_files` フィールドが存在しないため、対象外

## 2. 機能要件

### 2.1 環境変数展開のタイミング

`verify_files` の環境変数展開は、`cmd` および `args` と同じタイミング（Phase 1: Configuration Loading）で行う。

- 設定ファイル読み込み後、検証実行前に展開
- 展開結果は構造体に保存し、検証時に再利用

### 2.2 環境変数のスコープ

> **注**: Task 0031の実装により、以下の仕様が拡張されています。最新の仕様は [Task 0031 要件定義書のセクション 2.5 (F008)](../0031_global_group_env/01_requirements.md#25-verifyfiles-での環境変数参照) を参照してください。

#### 2.2.1 グローバルレベルの verify_files（Task 0030時点の仕様）

- **使用可能な環境変数**: システム環境変数のみ
- **Task 0031での拡張**: `global.env` で定義された環境変数も参照可能
- **allowlist**: `global.env_allowlist` を適用（システム環境変数参照時のみ）
- **例**:
  ```toml
  [global]
  env_allowlist = ["HOME", "USER"]
  verify_files = [
      "${HOME}/bin/script.sh",    # OK: HOME は allowlist に含まれる
      "${PATH}/bin/tool"          # エラー: PATH は allowlist に含まれない
  ]
  ```

#### 2.2.2 グループレベルの verify_files（Task 0030時点の仕様）

- **使用可能な環境変数**: システム環境変数のみ
- **Task 0031での拡張**: `global.env` および `groups[].env` で定義された環境変数も参照可能
- **allowlist**: グループの `env_allowlist` を適用（継承モードに従う、システム環境変数参照時のみ）
- **注（Task 0030時点）**: グループレベルには `env` フィールドが存在しないため、グループ固有の環境変数は定義できない
- **注（Task 0031以降）**: `groups[].env` フィールドが追加され、グループ固有の環境変数が定義可能になった
- **例**:
  ```toml
  [[groups]]
  name = "tools"
  env_allowlist = ["HOME", "USER"]
  verify_files = [
      "${HOME}/.config/app.conf",     # OK: システム環境変数
      "${USER}/data/verify.sh"        # OK: システム環境変数
  ]
  ```


### 2.3 既存機能との互換性

#### 2.3.1 絶対パスの継続サポート

従来の絶対パス指定は引き続き動作する。

```toml
[global]
verify_files = [
    "/usr/bin/python3",           # OK: 絶対パス
    "/bin/sh"                     # OK: 絶対パス
]
```

#### 2.3.2 ドル記号を含む既存設定

ファイル名に `$` を含む既存設定については、互換性を考慮しない。環境変数展開エラー（${VAR}形式の場合）もしくは解析エラー（$VARの場合）となる。

```toml
[global]
verify_files = [
    "/path/to/file$name.txt"      # エラー: $name は解析エラーとして扱われる
]
```

**移行ガイド**: ドル記号をエスケープする必要がある場合は、`\$` を使用する（タスク 0026 の仕様に準拠）。これにより '\' を含む既存設定に影響が出るが、次のセクションの方針で対応する。

#### 2.3.2 バックスラッシュ記号を含む既存設定

ファイル名に `\` を含む既存設定については、互換性を考慮しない。解析エラーとなる。

```toml
[global]
verify_files = [
    "/path/to/file\name.txt"      # エラー: \ は解析エラーとして扱われる
]
```

**移行ガイド**: バックスラッシュ記号をエスケープする必要がある場合は、`\\` を使用する（タスク 0026 の仕様に準拠）。


### 2.4 エラーハンドリング

#### 2.4.1 未定義変数

allowlist に含まれない変数、または定義されていない変数を参照した場合はエラーで停止する。

```toml
[global]
env_allowlist = ["HOME"]
verify_files = [
    "${UNDEFINED_VAR}/script.sh"    # エラー: 未定義変数
]
```

**エラーメッセージ例**:
```
Error: failed to expand verify_files at global level: undefined variable: UNDEFINED_VAR
```

#### 2.4.2 循環参照

システム環境変数の循環参照が検出された場合はエラーで停止する（タスク 0026 と同じ動作）。

**注**: `verify_files` で使用できるのはシステム環境変数のみであるため、TOML設定ファイル内で循環参照を作成することはできない。この検証はシステム環境変数自体に循環参照がある場合のみ適用される。

#### 2.4.3 allowlist フィルタリング

allowlist に含まれない変数を使用した場合はエラーで停止する。

```toml
[global]
env_allowlist = ["HOME"]
verify_files = [
    "${PATH}/bin/tool"              # エラー: PATH は allowlist に含まれない
]
```

**エラーメッセージ例**:
```
Error: failed to expand verify_files at global level: variable not in allowlist: PATH
```

### 2.5 展開後のパス検証

環境変数展開後のパスに対して、既存のセキュリティ検証を適用する。

- シンボリックリンク攻撃の防止
- パスの正規化
- アクセス権限の確認

## 3. 非機能要件

### 3.1 セキュリティ

- **環境変数のフィルタリング**: allowlist による厳格な制御
- **インジェクション攻撃の防止**: 展開後のパスに対する検証
- **権限昇格の防止**: 展開後のパスが意図しない特権ファイルを指さないことを確認

### 3.2 パフォーマンス

- **展開の最適化**: Phase 1 で一度だけ展開し、結果を再利用
- **キャッシュの活用**: 既存の環境変数展開ロジックを再利用

### 3.3 保守性

- **既存コードの再利用**: タスク 0026 で実装された `environment.CommandEnvProcessor` を活用
- **テストカバレッジ**: 全ての展開パターンをカバーするテストを作成
- **ドキュメント**: ユーザーガイドの更新

## 4. 制約事項

### 4.1 技術的制約

- Go 1.23.10 以上
- 既存の環境変数展開ロジック（`internal/runner/environment`）に依存
- 既存の検証ロジック（`internal/verification`）との統合

### 4.2 互換性制約

- ドル記号 `$` ならびにバックスラッシュ `\` を含む既存のファイルパス設定は動作しなくなる可能性がある
- 移行パスとして `\$` と `\\` によるエスケープを提供

## 5. 成功基準

### 5.1 機能の完全性

- [ ] グローバルレベルの verify_files で環境変数展開が動作する
- [ ] グループレベルの verify_files で環境変数展開が動作する
- [ ] システム環境変数が使用できる
- [ ] allowlist によるフィルタリングが正しく機能する
- [ ] 未定義変数、循環参照、allowlist 違反で適切にエラーが発生する

### 5.2 テストカバレッジ

- [ ] ユニットテストで全ての展開パターンをカバー
- [ ] エラーケースのテストを作成
- [ ] 統合テストで実際の TOML ファイルを使用した検証

### 5.3 ドキュメント

- [ ] ユーザーガイドの更新
- [ ] サンプル TOML ファイルの追加
- [ ] 移行ガイドの作成

## 6. 想定される利用シナリオ

### 6.1 ユーザー固有のツールパス

```toml
[global]
env_allowlist = ["HOME"]
verify_files = ["${HOME}/bin/custom_tool.sh"]
```

### 6.2 グループレベルでのシステム環境変数使用

```toml
[[groups]]
name = "development"
env_allowlist = ["HOME", "USER"]
verify_files = [
    "${HOME}/.config/linter.conf",
    "${HOME}/.config/formatter.conf"
]
```

### 6.3 複数の環境変数を使用したパス構成

```toml
[global]
env_allowlist = ["HOME", "USER"]
verify_files = ["${HOME}/${USER}/tools/deploy.sh"]
```

## 7. 今後の拡張可能性

### 7.1 将来的な機能追加

- コマンドレベルでの `verify_files` サポート（構造体拡張が必要）
- より高度なパス解決機能（glob パターンなど）

### 7.2 パフォーマンス最適化

- 展開結果のキャッシュ最適化
- 並列処理による展開の高速化

## 8. 参照

- タスク 0026: Variable Expansion Implementation（環境変数展開の基盤実装）
- タスク 0007: verify_hash_all（ファイル検証機能）
- タスク 0008: env_allowlist（環境変数 allowlist 機能）
