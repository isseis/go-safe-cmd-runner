# 要件定義: TOCTOU チェックの実装重複を解消する

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

本タスクは 2 フェーズで構成する。

- **フェーズ 1**: `cmd/runner` の TOCTOU チェックを `isec.DirectoryPermChecker` に統一する
- **フェーズ 2**: フェーズ 1 完了後、`isec.dirPermChecker` と `runner/security.Validator` の実装重複を解消する

## 受け入れ基準

### フェーズ 1

| # | 基準 |
|---|------|
| AC-1 | `cmd/runner/main.go` が TOCTOU チェックに `isec.NewDirectoryPermChecker()` を使用する |
| AC-2 | `runner/runner.go`、`group_executor.go`、`group_executor_options.go` の `toctouValidator` フィールドの型が `isec.DirectoryPermChecker` インターフェースになっている |
| AC-3 | `internal/runner/security/toctou_check.go` および `NewValidatorForTOCTOU()` が削除されている |
| AC-4 | `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功する |
| AC-5 | `make test` が全件パスする |
| AC-6 | `make lint` がエラーなしで完了する |

### フェーズ 2

| # | 基準 |
|---|------|
| AC-7 | ディレクトリ権限検証のコアロジックが 1 か所に集約され、`isec.dirPermChecker` と `runner/security.Validator` の双方がそれを使用する（または一方が不要となり削除される） |
| AC-8 | `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功する |
| AC-9 | `make test` が全件パスする |
| AC-10 | `make lint` がエラーなしで完了する |

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

### フェーズ 2: 実装重複の解消

フェーズ 1 完了後に着手する。

**現状の差異**

| 観点 | `isec.dirPermChecker` | `runner/security.Validator` |
|---|---|---|
| FS アクセス | `os.Lstat()` 直接 | `v.fs.Lstat()`（インタフェース経由・モック可） |
| パス長上限 | `DefaultMaxPathLength` 定数 | `v.config.MaxPathLength`（設定値、デフォルト同値） |
| 信頼 GID | root + darwin-admin ハードコード | `v.trustedGIDs`（`DefaultConfig()` では空のため実質同値） |
| `testPermissiveMode` | なし | あり（テスト用フラグ） |

**解消方針の選択肢**

フェーズ 1 の結果を踏まえて以下のいずれかを選択する。

- **方針 A（削除）**: `bootstrap.NewVerificationManager()` も `isec.NewDirectoryPermChecker()` に切り替え、`Validator.ValidateDirectoryPermissions` をプロダクションから削除する。`isec.dirPermChecker` に mock FS サポートを追加してテストを移行する。
- **方針 B（共通化）**: コアロジックをパッケージレベル関数として `internal/security` に抽出し、`isec.dirPermChecker` と `Validator.ValidateDirectoryPermissions` の双方がそれを呼び出す形にする。

フェーズ 1 完了時点で `Validator.ValidateDirectoryPermissions` のプロダクション利用箇所（`bootstrap.NewVerificationManager()` 経由）を再確認し、方針を確定する。

### スコープ外

- `toctouValidator` フィールドの nil チェック挙動の変更

## 影響範囲（フェーズ 1）

| ファイル | 変更内容 |
|---|---|
| `internal/runner/security/toctou_check.go` | 削除 |
| `internal/runner/runner.go` | `toctouValidator` フィールド型変更、`WithTOCTOUValidator` 引数型変更 |
| `internal/runner/group_executor.go` | `toctouValidator` フィールド型変更 |
| `internal/runner/group_executor_options.go` | `toctouValidator` フィールド型変更 |
| `cmd/runner/main.go` | `isec.NewDirectoryPermChecker()` に変更、不要 import 削除 |
