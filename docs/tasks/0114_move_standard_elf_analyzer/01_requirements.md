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

詳細な設計は [アーキテクチャ設計書](02_architecture.md) を参照。

## スコープ外

- `StandardELFAnalyzer` 以外の `internal/runner/security/elfanalyzer` コンポーネントの移動
- `cmd/runner` の `internal/runner/security/elfanalyzer` 依存の解消（runner は PrivilegeManager を実際に使用するため許容）
- `filevalidator.PrivilegedFileValidator` の `runnertypes` 依存の解消
