# 要件定義: StandardELFAnalyzer の internal/security/elfanalyzer への移動

## 背景

パッケージ再構成タスク（0113）において、`internal/runner/security/elfanalyzer` パッケージのうち
runner 非依存の低レベル解析エンジン（syscall アナライザ、アーキテクチャ固有デコーダ等）は
`internal/security/elfanalyzer` に移動済みである。

しかし ELF バイナリのネットワーク検出を担う高レベルコンポーネント `StandardELFAnalyzer` は
`internal/runner/security/elfanalyzer` に残っており、`runnertypes.PrivilegeManager` に
依存しているため、以下の推移的依存が発生している。

```
cmd/record
  └─ internal/runner/security          (NewBinaryAnalyzer)
       └─ internal/runner/security/elfanalyzer   (NewStandardELFAnalyzer)
            └─ internal/runner/runnertypes        (PrivilegeManager)
                 └─ internal/runner/privilege
```

`cmd/record` は `PrivilegeManager` を利用しない（`nil` を渡している）にもかかわらず、
パッケージをインポートするだけで runner 固有の依存ツリーを引き込んでいる。

## 問題

```
go list -deps ./cmd/record | grep internal/runner/security/elfanalyzer
```

上記コマンドが `internal/runner/security/elfanalyzer` を返す。
これは `cmd/record` の依存境界が期待より広いことを意味する。

## 目標

`StandardELFAnalyzer` を `internal/security/elfanalyzer` に移動することで、
`cmd/record` の `internal/runner/security/elfanalyzer` への依存を解消する。

## 受け入れ基準

| # | 基準 |
|---|------|
| AC-1 | `go list -deps ./cmd/record | grep internal/runner/security/elfanalyzer` が 0 件 |
| AC-2 | `go list -deps ./cmd/verify | grep internal/runner/security` が 0 件（現状維持） |
| AC-3 | `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功 |
| AC-4 | `make test` が全件パス |
| AC-5 | `cmd/runner` において execute-only バイナリ（`os.ErrPermission`）への特権アクセスが引き続き動作する |

## 設計方針

### 根本原因

`StandardELFAnalyzer` が `runnertypes.PrivilegeManager` を直接フィールドに持つため、
このパッケージを利用するだけで runner の依存ツリーが引き込まれる。
特権昇格が必要な呼び出し箇所は1箇所のみ（`standard_analyzer.go` 内の `os.ErrPermission` ハンドラ）。

### 解決策: PrivilegedFileOpener インターフェースによる抽象化

`runnertypes.PrivilegeManager` への直接依存を除去し、`internal/security/elfanalyzer` 内に
ファイル特権オープンを抽象化するインターフェースを定義する。

```
internal/security/elfanalyzer/
  privileged_opener.go     // PrivilegedFileOpener インターフェース定義
  standard_analyzer.go     // StandardELFAnalyzer（PrivilegedFileOpener を使用）

internal/runner/security/elfanalyzer/
  privileged_opener_impl.go  // PrivilegeManager を使った PrivilegedFileOpener 実装
  // （NewStandardELFAnalyzerWithSyscallStore など runner 固有コンストラクタ）
```

#### PrivilegedFileOpener インターフェース（案）

```go
// internal/security/elfanalyzer/privileged_opener.go

// PrivilegedFileOpener はアクセス権不足時に特権昇格でファイルを開く手段を提供する。
// nil の場合は特権昇格を行わない（os.ErrPermission をそのまま返す）。
type PrivilegedFileOpener interface {
    OpenWithPrivileges(path string) (safefileio.File, error)
}
```

#### runner 側の実装（案）

```go
// internal/runner/security/elfanalyzer/privileged_opener_impl.go

type privilegedFileOpenerImpl struct {
    pfv         *filevalidator.PrivilegedFileValidator
    privManager runnertypes.PrivilegeManager
}

func NewPrivilegedFileOpener(
    fs safefileio.FileSystem,
    privManager runnertypes.PrivilegeManager,
) secelfanalyzer.PrivilegedFileOpener {
    return &privilegedFileOpenerImpl{
        pfv:         filevalidator.NewPrivilegedFileValidator(fs),
        privManager: privManager,
    }
}
```

### 依存関係の変化

移動後:

```
cmd/record
  └─ internal/runner/security          (NewBinaryAnalyzer)
       └─ internal/security/elfanalyzer   (NewStandardELFAnalyzer)
            ※ internal/runner/runnertypes への依存なし

cmd/runner
  └─ internal/runner/security/elfanalyzer  (NewPrivilegedFileOpener + コンストラクタ)
       └─ internal/security/elfanalyzer
       └─ internal/runner/runnertypes
```

### internal/runner/security/binary_analyzer.go の変更

```go
// 変更前
import "internal/runner/security/elfanalyzer"
return elfanalyzer.NewStandardELFAnalyzer(nil, nil)

// 変更後
import secelfanalyzer "internal/security/elfanalyzer"
return secelfanalyzer.NewStandardELFAnalyzer(nil, nil)
```

## スコープ外

- `StandardELFAnalyzer` 以外の `internal/runner/security/elfanalyzer` コンポーネントの移動
- `cmd/runner` の `internal/runner/security/elfanalyzer` 依存の解消（runner は PrivilegeManager を実際に使用するため許容）
- `filevalidator.PrivilegedFileValidator` の `runnertypes` 依存の解消
