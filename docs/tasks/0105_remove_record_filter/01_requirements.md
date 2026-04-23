# 要件定義書: record コマンドのシステムコールフィルタリング削除

## 1. 概要

### 1.1 背景

現行の `record` コマンドは、検出したシステムコールを JSON レコードに書き出す際に `FilterSyscallsForStorage` を呼び出し、以下の条件に合致するエントリのみを保存している。

- `IsNetwork == true`（ネットワーク関連システムコール）
- `Number == -1`（解決できなかった未知のシステムコール）

この設計はリスク判定に「現時点で必要な情報のみを保存する」という意図のもとに行われたが、以下の問題がある。

**問題: 関心の分離の逸脱**

`record` はバイナリを解析して結果を記録するコンポーネントであり、`runner` は記録された情報を参照してポリシーに照らし合わせてリスク判断するコンポーネントである。この役割分担において、「どの情報がリスク判定に必要か」はポリシー（`runner` 側）の関心事であり、`record` が判断すべきではない。

- ポリシーが変化した場合（例: ファイルシステム関連システムコールもリスク評価に含めるなど）、すべてのバイナリを再 `record` しなければならない
- `record` がフィルタリングを行うことで、`runner` は全体像を把握できず、将来の拡張性が損なわれる

### 1.2 目的

- `record` コマンドが検出したすべてのシステムコール・シンボルをフィルタリングせずに JSON レコードへ記録するよう変更する
- `runner` コマンドが、フィルタリングされていないレコードを前提としたリスク判定ロジックへ更新する

### 1.3 スコープ

#### 対象

- `internal/fileanalysis/syscall_store.go` の `FilterSyscallsForStorage` 関数の削除
- `internal/filevalidator/validator.go` の `buildSyscallData`（ELF）における `FilterSyscallsForStorage` 呼び出しの削除
- `internal/filevalidator/validator.go` の `buildMachoSyscallData`（Mach-O）における `FilterSyscallsForStorage` 呼び出しの削除、および `AnalysisWarnings` 生成ロジックの修正
- `internal/runner/security/network_analyzer.go` の `syscallAnalysisHasSVCSignal` および `syscallAnalysisHasNetworkSignal` のフィルタリングされていないレコードに対する動作確認・修正
- **macOS BSD syscall テーブルの自動生成化**（後述 FR-5）
- 上記に伴う既存テストの更新（少なくとも `internal/fileanalysis/syscall_store_test.go`、`internal/filevalidator/validator_test.go`、`internal/filevalidator/validator_macho_test.go`、`internal/runner/security/network_analyzer_test.go`、macOS syscall テーブル関連テスト）
- 関連仕様書との整合更新（少なくとも `docs/tasks/0104_macho_syscall_number_analysis/` 配下の `syscallAnalysisHasSVCSignal` 削除前提記述を本タスクの方針に合わせて修正、または superseded として明示）

#### 対象外

- `SymbolAnalysis`（シンボル解析）のフィルタリング。現行は `AnalyzeNetworkSymbols` によりネットワーク関連シンボルのみが記録されているが、本タスクのスコープではない（別タスクで対応）
- スキーマバージョンの変更。`FilterSyscallsForStorage` 削除はフィールド追加ではなく、既存フィールド `detected_syscalls` の内容が増えるだけであり JSON 構造の互換性を破壊しない

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `FilterSyscallsForStorage` | `internal/fileanalysis/syscall_store.go` に定義された、保存対象エントリを絞り込む関数。本タスクで削除する |
| svc エントリ | Mach-O arm64 バイナリで検出された直接 `svc #0x80` 命令のエントリ。`DeterminationMethod == "direct_svc_0x80"` |
| 解決済み svc | svc エントリのうち、Pass 1 / Pass 2 によりシステムコール番号（`Number`）が特定されたもの |
| 未解決 svc | svc エントリのうち `Number == -1` のまま残ったもの（backward scan で番号を特定できなかった） |
| libSystem エントリ | libSystem.dylib のインポートシンボルマッチングで検出されたエントリ。`Source == "libSystem"` |
| libc エントリ | ELF バイナリの libc インポートシンボルマッチングで検出されたエントリ。`Source == "libc_symbol_import"` |

## 3. 機能要件

### FR-1: `FilterSyscallsForStorage` の削除

`internal/fileanalysis/syscall_store.go` の `FilterSyscallsForStorage` 関数を削除する。

この関数は `buildSyscallData`（ELF）と `buildMachoSyscallData`（Mach-O）の 2 箇所から呼ばれている。各呼び出し箇所を以下の通り修正する。

### FR-2: ELF システムコール記録のフィルタリング削除

`buildSyscallData`（`internal/filevalidator/validator.go`）を修正し、`FilterSyscallsForStorage` を呼び出さずにすべてのシステムコールエントリを `DetectedSyscalls` に格納する。

修正前:
```go
retained := fileanalysis.FilterSyscallsForStorage(all)
```

修正後:
```go
// all: すべて格納（フィルタリングなし）
```

### FR-3: Mach-O システムコール記録のフィルタリング削除

`buildMachoSyscallData`（`internal/filevalidator/validator.go`）を修正し、`FilterSyscallsForStorage` を呼び出さずにすべての svc / libSystem / wrapper エントリを `DetectedSyscalls` に格納する。

#### FR-3.1: `AnalysisWarnings` ロジックの修正

現行の `AnalysisWarnings` 生成ロジックは、フィルタリング後のエントリに `DeterminationMethod == "direct_svc_0x80"` が存在する場合に警告を出力する。フィルタリング削除後は、**未解決 svc エントリ（`Number == -1` かつ `DeterminationMethod == "direct_svc_0x80"`）** が存在する場合のみ警告を出力するよう修正する。

理由: フィルタリングを削除すると解決済み svc エントリも格納されるため、`DeterminationMethod` のみによる判定では解決済み・非ネットワーク svc（例: `read()`）でも誤って警告が発生する。

修正前:
```go
retained := fileanalysis.FilterSyscallsForStorage(merged)
var warnings []string
for _, s := range retained {
    if s.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 {
        warnings = []string{"svc #0x80 detected: syscall number unresolved, ..."}
        break
    }
}
```

修正後:
```go
// フィルタリングなし: すべてのエントリを格納
var warnings []string
for _, s := range merged {
    if s.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 && s.Number == -1 {
        warnings = []string{"svc #0x80 detected: syscall number unresolved, direct kernel call bypassing libSystem.dylib"}
        break
    }
}
```

### FR-4: `runner` リスク判定ロジックの更新

`internal/runner/security/network_analyzer.go` の 2 つの判定関数を、フィルタリングされていないレコードに対して正しく動作するよう更新する。

#### FR-4.1: `syscallAnalysisHasSVCSignal` の修正

**高リスク（`isHighRisk = true`）シグナルとして扱う条件:** 未解決 svc エントリ、すなわち `Number == -1` かつ `DeterminationMethod == "direct_svc_0x80"` のエントリが存在する場合。

理由: 未解決の直接 `svc #0x80` 呼び出しは libSystem.dylib を迂回した未知のカーネル呼び出しであり、ビルド後のバイナリインジェクションや異常実行の可能性がある。

解決済み svc エントリ（`Number != -1`）のリスク評価は以下の通り FR-4.2 に委ねる。

修正前:
```go
for _, s := range result.DetectedSyscalls {
    if s.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 {
        return true
    }
}
```

修正後:
```go
for _, s := range result.DetectedSyscalls {
    if s.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 && s.Number == -1 {
        return true
    }
}
```

#### FR-4.2: `syscallAnalysisHasNetworkSignal` の修正

**ネットワーク中リスク（`isNetwork = true`）シグナルとして扱う条件:** `IsNetwork == true` のエントリが存在する場合。`DeterminationMethod` による除外は行わない。

理由: フィルタリング削除後、解決済み svc エントリのうちネットワーク関連のもの（例: `socket()` が svc #0x80 経由で呼び出されたケース）が `DetectedSyscalls` に含まれる。現行の `DeterminationMethod != "direct_svc_0x80"` 除外はこれらを見逃すため、除外条件を削除する。

修正前:
```go
for _, s := range result.DetectedSyscalls {
    if s.IsNetwork && s.DeterminationMethod != common.DeterminationMethodDirectSVC0x80 {
        return true
    }
}
```

修正後:
```go
for _, s := range result.DetectedSyscalls {
    if s.IsNetwork {
        return true
    }
}
```

### FR-5: macOS BSD syscall テーブルの自動生成化

#### 背景と問題

フィルタリング削除後、Mach-O バイナリで解決済みの非ネットワーク svc エントリ（例: `read()`、`write()`）も `detected_syscalls` に格納される。しかし現行の `MacOSSyscallTable`（`internal/libccache/macos_syscall_table.go`）は 17 エントリのみ手動管理であり、これらのエントリは `Name == ""`（空文字）になる。JSON の可読性とデバッグ容易性のため、全 BSD syscall を網羅したテーブルへ拡張する。

ELF テーブルと同様に macOS SDK ヘッダーから自動生成することで、テーブルの保守コストをなくす。

#### FR-5.1: 生成スクリプトの拡張

`scripts/generate_syscall_table.py` を拡張し、macOS BSD syscall テーブルの生成も行えるようにする。

- **入力ソース**: macOS SDK の `sys/syscall.h`（デフォルト: `$(xcrun --show-sdk-path)/usr/include/sys/syscall.h`）
- **define 形式**: `SYS_<name>` 形式（Linux の `__NR_<name>` と異なる）
  ```c
  #define SYS_read           3
  #define SYS_write          4
  #define SYS_socket         97
  ```
- **出力ファイル**: `internal/libccache/macos_syscall_numbers.go`（新規、コミット対象）
- **生成される内容**: 全 BSD syscall を収録した `macOSSyscallEntries` マップ変数

`generate_syscall_table.py` への追加は `--macos-header` オプションで行い、既存の `--x86-header` / `--arm64-header` オプションとの後方互換を維持する。

#### FR-5.2: ネットワーク syscall 分類

macOS のネットワーク関連 syscall 名は ELF 版の `NETWORK_SYSCALL_NAMES` と共通のものが多いが、macOS 固有の差分がある（`accept4`・`recvmmsg`・`sendmmsg` は Linux 固有で macOS には存在しない）。スクリプト内で macOS 用の `MACOS_NETWORK_SYSCALL_NAMES` セットを定義し、ヘッダーに存在する名前のみに絞って `isNetwork` フラグを付与する。

#### FR-5.3: 生成ファイルとの役割分担

| ファイル | 変更 | 内容 |
|---------|------|------|
| `internal/libccache/macos_syscall_numbers.go` | 新規（自動生成） | `macOSSyscallEntries` マップ変数（全 BSD syscall） |
| `internal/libccache/macos_syscall_table.go` | 修正 | 手動管理の `macOSSyscallEntries` 定義を削除。`MacOSSyscallTable` 構造体・メソッド・`networkSyscallWrapperNames` は残す |

`MacOSSyscallTable` の `GetSyscallName` / `IsNetworkSyscall` メソッドは引き続き `macOSSyscallEntries` を参照するため、呼び出し側（Pass 1 / Pass 2 / matcher）の変更は不要。

#### FR-5.4: Makefile の更新

`generate-syscall-tables` ターゲットを更新し、macOS SDK ヘッダーが存在する場合に `macos_syscall_numbers.go` も生成するようにする。

```makefile
MACOS_SYSCALL_HEADER ?= $(shell xcrun --show-sdk-path 2>/dev/null)/usr/include/sys/syscall.h
SYSCALL_TABLE_OUTPUTS += internal/libccache/macos_syscall_numbers.go
```

macOS SDK ヘッダーが存在しない環境（Linux CI 等）では macOS テーブルの生成をスキップし、コミット済みの `macos_syscall_numbers.go` をそのまま使用する。

## 4. 非機能要件

### NFR-1: 既存レコードとの互換性

フィルタリング削除は `detected_syscalls` フィールドの内容を増やす変更であり、JSON 構造自体は変わらない。既存の（フィルタリングされた）レコードは引き続き正常にロードされ、`runner` は旧レコードに対しても正しく動作する。

旧レコードを持つバイナリに対して再 `record` は不要（ただし旧レコードには非ネットワーク・解決済みシステムコールが含まれていないため、将来ポリシーが拡張された場合はメリットを享受できない）。

### NFR-2: `record` の出力サイズの増加

フィルタリング削除により、多くのシステムコールを持つバイナリでは `detected_syscalls` の件数が増加する。これはストレージ使用量・`record` の実行時間に軽微な影響を与えうる。セキュリティ上の正確性を優先するため許容する。

### NFR-3: スキーマバージョンの非変更

`CurrentSchemaVersion` は変更しない。JSON 構造の互換性（フィールドの追加・削除・型変更）は変わらないためである。

## 5. 受け入れ基準

### AC-1: ELF バイナリの全システムコール記録

- ネットワーク非関連の解決済みシステムコール（例: `write()`、`read()`）を持つ ELF バイナリに対して `record` を実行すると、これらのエントリが `detected_syscalls` に記録されること
- `FilterSyscallsForStorage` が削除されていること（またはいずれの箇所からも呼び出されていないこと）

### AC-2: Mach-O バイナリの全システムコール記録

- 解決済みの非ネットワーク svc エントリを持つ Mach-O バイナリに対して `record` を実行すると、これらのエントリが `detected_syscalls` に記録されること
- libSystem エントリと svc エントリがすべて `detected_syscalls` に記録されること

### AC-3: `AnalysisWarnings` の正確な発出

- 未解決 svc エントリ（`Number == -1`）のみが存在する Mach-O バイナリでは `analysis_warnings` に警告が記録されること
- すべての svc エントリが解決済み（`Number != -1`）の Mach-O バイナリでは `analysis_warnings` に svc 関連の警告が記録されないこと

### AC-4: `runner` の未解決 svc 高リスク判定

- 未解決 svc エントリ（`Number == -1`、`DeterminationMethod == "direct_svc_0x80"`）を含むレコードで `runner` を実行すると、高リスク（`isHighRisk = true`）と判定されること
- 解決済み非ネットワーク svc エントリのみ（`Number != -1`、`IsNetwork == false`、`DeterminationMethod == "direct_svc_0x80"`）を含むレコードで `runner` を実行すると、高リスクと判定されないこと

### AC-5: `runner` の解決済みネットワーク svc 判定

- 解決済みネットワーク svc エントリ（`Number != -1`、`IsNetwork == true`、`DeterminationMethod == "direct_svc_0x80"`）を含むレコードで `runner` を実行すると、ネットワーク操作あり（`isNetwork = true`）と判定されること

### AC-6: macOS syscall テーブルの拡張

- `internal/libccache/macos_syscall_numbers.go` が自動生成されており、`SYS_read`（3）・`SYS_write`（4）などネットワーク非関連の syscall を含む全 BSD syscall が収録されていること
- `MacOSSyscallTable.GetSyscallName(3)` が `"read"` を返すこと
- `MacOSSyscallTable.IsNetworkSyscall(97)` が `true`（socket）を返すこと
- `MacOSSyscallTable.IsNetworkSyscall(3)` が `false`（read）を返すこと
- `make generate-syscall-tables` が macOS 環境で `macos_syscall_numbers.go` を再生成できること

### AC-7: 既存テストの通過

- `make test` / `make lint` がエラーなしで通過すること
- `internal/filevalidator/validator_test.go` の `buildSyscallData` テストが、非ネットワーク・解決済み syscall も保持される前提へ更新されること
- `internal/filevalidator/validator_macho_test.go` の `buildMachoSyscallData` テストが、解決済み非ネットワーク svc を保持しつつ、未解決 svc のみ `analysis_warnings` を発火させる前提へ更新されること
- `internal/runner/security/network_analyzer_test.go` の SVC / network signal テストが、未解決 svc のみ high risk、`IsNetwork == true` は `DeterminationMethod` に依存せず network signal と判定する前提へ更新されること
- `internal/fileanalysis/syscall_store_test.go` の `FilterSyscallsForStorage` 前提テストが削除または置換されること

### AC-8: 関連ドキュメントの整合

- `docs/tasks/0104_macho_syscall_number_analysis/` 配下に残っている「`syscallAnalysisHasSVCSignal` を削除する」前提の記述が、本タスク後の実装方針と矛盾しない状態へ更新されること

## 6. 先行タスクとの関係

| タスク | 関係 |
|-------|------|
| 0097 Mach-O arm64 svc スキャン | `buildMachoSyscallData` と `FilterSyscallsForStorage` を導入したタスク。本タスクでフィルタリングを削除する |
| 0100 Mach-O libSystem キャッシュ | Mach-O libSystem エントリと svc エントリのマージロジックを実装したタスク。マージ後のエントリが本タスクで全件格納されるようになる |
| 0104 Mach-O システムコール番号解析 | Pass 1 / Pass 2 により svc エントリのシステムコール番号を解決するタスク。解決済みエントリが本タスクで全件格納されるようになる |
| 0102 AnalysisFeatures | `AnalysisFeatures` フラグで解析実施を追跡するタスク。本タスクとは独立しており、干渉しない |
| ELF syscall テーブル自動生成 | `scripts/generate_syscall_table.py` と `make generate-syscall-tables` ターゲット。本タスクで macOS 対応を追加する |
