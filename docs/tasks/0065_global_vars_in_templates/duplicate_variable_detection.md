# 重複変数定義の検出

## 概要

このドキュメントでは、設定ファイル内での重複変数定義の検出メカニズムについて説明します。

## 調査結果

TOMLパーサー（`github.com/pelletier/go-toml/v2`）は、TOML 1.0.0仕様に従い、同一スコープ内での重複キー定義を自動的に検出し拒否します。

### 検証されたケース

以下の全てのレベルで重複が自動的に検出されることを確認しました：

1. **グローバル変数** (`[global.vars]`)
2. **グループレベル変数** (`[groups.vars]`)
3. **コマンドレベル変数** (`[groups.commands.vars]`)
4. **インラインテーブル構文**

### エラーメッセージ

重複キーが検出された場合、TOMLパーサーは以下の形式でエラーを返します：

```
toml: key <variable_name> is already defined
```

例：
```
toml: key TestVar is already defined
toml: key test_var is already defined
```

## テストケース

詳細なテストケースは以下のファイルを参照してください：

- [internal/runner/config/duplicate_vars_test.go](../../../internal/runner/config/duplicate_vars_test.go)

### テストの例

```toml
# これはエラーになります
[global.vars]
TestVar = "first"
TestVar = "second"  # エラー: toml: key TestVar is already defined
```

```toml
# これはエラーになります
[[groups]]
name = "test"

[groups.vars]
test_var = "first"
test_var = "second"  # エラー: toml: key test_var is already defined
```

## 実装への影響

### アプリケーションレベルでの重複チェックは不要

TOMLパーサーが設定ファイルのパース段階で重複を保証するため、以下の実装は不要です：

- ✗ `ProcessVars()` での重複チェックロジック
- ✗ カスタムエラーハンドリング（重複変数用）
- ✗ 追加の検証ステップ

### VariableRegistryでの重複チェック

`VariableRegistry.RegisterGlobal()` メソッドには重複チェックは**含まれていません**（詳細仕様書 2.2.2節）。

理由:

1. **TOMLパーサーが既に保証**: 設定ファイルのパース段階で重複は確実に検出される
2. **到達不可能なコード**: TOMLパーサーを経由する限り、重複チェックには絶対に到達しない
3. **YAGNI原則**: 将来的に動的変数登録が必要になった時点で追加すべき

したがって、以下のコードや型は不要です：

- ✗ `RegisterGlobal()` 内の重複チェックロジック
- ✗ `ErrDuplicateVariableDefinition` エラー型
- ✗ 重複登録のテストケース

## TOML仕様の参照

この動作はTOML 1.0.0仕様に基づいています：

> Keys within a table must be unique. Defining a key more than once is invalid.

参照: [TOML Specification - Keys](https://toml.io/en/v1.0.0#keys)

## まとめ

- **重複変数定義は、TOMLパーサーが自動的に検出します**
- **全てのレベル**（global, group, command）で検出されます
- **アプリケーションコードでの追加の重複チェックは不要**です

## 関連ドキュメント

- [詳細仕様書](./03_detailed_specification.md)
- [テストコード](../../../internal/runner/config/duplicate_vars_test.go)
