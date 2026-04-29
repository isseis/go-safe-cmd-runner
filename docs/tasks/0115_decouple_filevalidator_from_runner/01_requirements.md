# 要件定義: filevalidator から runner 依存を除去する

## 背景

task 0114 において `cmd/record` の `internal/runner/security/elfanalyzer` への依存は解消された。
しかし `cmd/record` / `cmd/verify` は `internal/filevalidator` 経由で引き続き
`internal/runner` 以下のパッケージに依存している。

```
cmd/record / cmd/verify
  └─ internal/filevalidator
       ├─ internal/runner/runnertypes   (PrivilegeManager, ElevationContext, ErrPrivilegedExecutionNotAvailable)
       └─ internal/runner/privilege     (ErrPrivilegedExecutionNotSupported)
```

`cmd/record` および `cmd/verify` は特権昇格を一切使用しない（`nil` を渡している）にもかかわらず、
パッケージをインポートするだけで runner 固有の依存ツリーを引き込んでいる。

## 問題

```
go list -deps ./cmd/record | grep internal/runner
go list -deps ./cmd/verify | grep internal/runner
```

上記コマンドが `internal/runner/runnertypes` および `internal/runner/privilege` を返す。

## 原因分析

`internal/filevalidator` には以下の runner 依存コードが存在する。

### privileged_file.go — PrivilegedFileValidator

```go
func (pfv *PrivilegedFileValidator) OpenFileWithPrivileges(
    filepath string,
    privManager runnertypes.PrivilegeManager,  // runner 依存
) (safefileio.File, error)
```

- `runnertypes.PrivilegeManager` / `ElevationContext` / `OperationFileValidation` を直接参照
- `privilege.ErrPrivilegedExecutionNotSupported` を参照
- **外部からの呼び出しはない**（`filevalidator.Validator` 内部でのみ使用）

### validator.go — FileValidator インターフェースと Validator 実装

```go
type FileValidator interface {
    VerifyWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) error
    VerifyAndReadWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) ([]byte, error)
    // ...
}
```

- `FileValidator` インターフェースの 2 メソッドが `runnertypes.PrivilegeManager` を引数に取る
- `Validator` が両メソッドを実装
- **本番コードからの呼び出しはない**（テストモックのみが実装する）

## 目標

`internal/filevalidator` から `internal/runner/runnertypes` と `internal/runner/privilege` への
依存を除去し、`cmd/record` / `cmd/verify` が `internal/runner` 以下に依存しない構成にする。

## 受け入れ基準

| # | 基準 |
|---|------|
| AC-1 | `go list -deps ./cmd/record \| grep internal/runner` が 0 件 |
| AC-2 | `go list -deps ./cmd/verify \| grep internal/runner` が 0 件 |
| AC-3 | `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功 |
| AC-4 | `make test` が全件パス |
| AC-5 | `cmd/runner` の特権昇格（execute-only バイナリの検証）が引き続き動作する |

## 設計方針

### 基本方針：呼び出し元のない特権 API の分離

本番コードから呼ばれていない特権系 API を `filevalidator` から除去し、
runner 固有の特権アクセスは runner 側のコードとして配置する。

#### Step 1: VerifyWithPrivileges / VerifyAndReadWithPrivileges の除去

`FileValidator` インターフェースおよび `Validator` から `VerifyWithPrivileges` /
`VerifyAndReadWithPrivileges` を削除する。

本番コードからの呼び出しがないため、YAGNI 原則に従って除去する。
runner がファイル検証に特権昇格を必要とするようになった際は、
`internal/runner/` 側に runner 固有の検証関数として実装する。

#### Step 2: PrivilegedFileValidator の分離

`privileged_file.go`（`PrivilegedFileValidator` 型）を `filevalidator` パッケージから分離する。

`PrivilegedFileValidator` を直接使用している `filevalidator.Validator` の
`privilegedFileValidator` フィールドも合わせて除去する。

`PrivilegedFileValidator` が runner 側で再び必要になった場合は
`internal/runner/security/` または `internal/runner/filevalidator/` に配置する。

#### Step 3: validator.go から runnertypes import を除去

Step 1・Step 2 完了後、`validator.go` からの `runnertypes` import が不要になるため削除する。

### 影響範囲

| コンポーネント | 変更内容 |
|---|---|
| `internal/filevalidator/privileged_file.go` | ファイルごと削除 |
| `internal/filevalidator/privileged_file_test.go` | ファイルごと削除 |
| `internal/filevalidator/validator.go` | `VerifyWithPrivileges` / `VerifyAndReadWithPrivileges` メソッド・インターフェース定義を削除、`privilegedFileValidator` フィールドを削除、`runnertypes` import を削除 |
| `internal/verification/testing/testify_mocks.go` | 削除したメソッドのモック実装を削除 |
| `internal/verification/manager_shebang_test.go` | 削除したメソッドのスタブ実装を削除 |

### スコープ外

- `internal/runner/` パッケージ自体の依存整理
- `internal/verification/` が `internal/runner/` に依存している問題（別タスク）
- `filevalidator` 以外のパッケージの runner 依存解消
