# 要件定義: TOCTOU チェックから runner/security.Validator 依存を除去する

## 背景

`internal/security` パッケージには TOCTOU（Time-Of-Check-Time-Of-Use）ディレクトリ権限チェック専用の軽量な実装が存在する。

```
internal/security.DirectoryPermChecker  （インターフェース）
internal/security.NewDirectoryPermChecker()  （スタンドアロン実装）
```

`cmd/record` と `cmd/verify` はすでにこの実装を正しく使用している。

しかし `cmd/runner` のみが `internal/runner/security.Validator`（全セキュリティ検証機能を持つ重厚な構造体）を TOCTOU チェックに使用している。この `Validator` は AllowedCommands パターン・SensitiveEnvVars・TrustedGIDs など TOCTOU チェックとは無関係な多数のフィールドを持つ。

## 問題

### 不整合

| コマンド | TOCTOU チェックの実装 |
|---|---|
| `cmd/record` | `isec.NewDirectoryPermChecker()` ✓ |
| `cmd/verify` | `isec.NewDirectoryPermChecker()` ✓ |
| `cmd/runner` | `runner/security.NewValidatorForTOCTOU()` ← 重厚な `Validator` を使用 |

### 原因

`runner/runner.go`、`group_executor.go`、`group_executor_options.go` の `toctouValidator` フィールドが
`*security.Validator` のコンクリート型で宣言されており、`cmd/runner/main.go` まで型が伝播している。

```go
// runner/runner.go:88（現状）
toctouValidator *security.Validator

// group_executor.go:70（現状）
toctouValidator *security.Validator

// group_executor_options.go:19（現状）
toctouValidator *security.Validator
```

### 不要なコード

`internal/runner/security/toctou_check.go` に定義された `NewValidatorForTOCTOU()` は
`NewValidator(nil, WithGroupMembership(groupmembership.New()))` を呼ぶだけの
3行のラッパーであり、`cmd/runner/main.go` 以外から使用されていない。

`RunTOCTOUPermissionCheck` が受け取る型はすでに `isec.DirectoryPermChecker` インターフェースであり、
`*security.Validator` 全体は不要である。

## 目標

TOCTOU チェックの実装を `cmd/runner` においても `isec.DirectoryPermChecker` インターフェースに統一し、
`runner/security.Validator` への不要な依存と重複実装を除去する。

## 受け入れ基準

| # | 基準 |
|---|------|
| AC-1 | `cmd/runner/main.go` が TOCTOU チェックに `isec.NewDirectoryPermChecker()` を使用する |
| AC-2 | `runner/runner.go`、`group_executor.go`、`group_executor_options.go` の `toctouValidator` フィールドの型が `isec.DirectoryPermChecker` インターフェースになっている |
| AC-3 | `internal/runner/security/toctou_check.go` および `NewValidatorForTOCTOU()` が削除されている |
| AC-4 | `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功する |
| AC-5 | `make test` が全件パスする |
| AC-6 | `make lint` がエラーなしで完了する |

## 設計方針

### フィールド型をインターフェースに変更する

`toctouValidator` フィールドの型を `*security.Validator` から `isec.DirectoryPermChecker` に変更する。

```go
// 変更前
toctouValidator *security.Validator

// 変更後
toctouValidator isec.DirectoryPermChecker
```

対象ファイル：
- `internal/runner/runner.go`
- `internal/runner/group_executor.go`
- `internal/runner/group_executor_options.go`

### `cmd/runner/main.go` の呼び出しを変更する

```go
// 変更前
secValidator, secErr := security.NewValidatorForTOCTOU()
// ...
runnerOptions = append(runnerOptions, runner.WithTOCTOUValidator(secValidator))

// 変更後
secValidator, secErr := isec.NewDirectoryPermChecker()
// ...
runnerOptions = append(runnerOptions, runner.WithTOCTOUValidator(secValidator))
```

`runner/security` の import が TOCTOU チェック目的のみで使用されていた場合、その import も削除する。

### `toctou_check.go` を削除する

`internal/runner/security/toctou_check.go` を削除し、`NewValidatorForTOCTOU()` を除去する。

### スコープ外

- `runner/security.Validator.ValidateDirectoryPermissions` メソッド自体の削除または移動
  （テストで mock FS を注入する用途などで引き続き使われる可能性がある）
- `internal/security.dirPermChecker` と `runner/security.Validator` の実装重複の解消
- `toctouValidator` フィールドの nil チェック挙動の変更

## 影響範囲

| ファイル | 変更内容 |
|---|---|
| `internal/runner/security/toctou_check.go` | 削除 |
| `internal/runner/runner.go` | `toctouValidator` フィールド型変更、`WithTOCTOUValidator` 引数型変更 |
| `internal/runner/group_executor.go` | `toctouValidator` フィールド型変更 |
| `internal/runner/group_executor_options.go` | `toctouValidator` フィールド型変更 |
| `cmd/runner/main.go` | `isec.NewDirectoryPermChecker()` に変更、不要 import 削除 |
