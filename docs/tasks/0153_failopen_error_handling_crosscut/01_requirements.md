# 要件定義書: エラー隠蔽による fail-open パターンの横断修正（残件）

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-19 |
| Review date | 2026-07-20 |
| Reviewer | isseis |
| Comments | - |

## 関連 Issue

- [#860 [Security][P1] エラー隠蔽による fail-open パターンの横断修正](https://github.com/isseis/go-safe-cmd-runner/issues/860)
- 関連（解消済み）: [#858 [Security][H-1] groupmembership: getGroupMembers のエラー握りつぶしによる fail-open](https://github.com/isseis/go-safe-cmd-runner/issues/858) — D1 (groupmembership) H-1/M-1/M-2 は [docs/tasks/0150_groupmembership_getgrgid_failclosed](../0150_groupmembership_getgrgid_failclosed/) / [docs/tasks/0151_groupmembership_failclosed](../0151_groupmembership_failclosed/) で対応済み。#860 に挙げられている D1 の L-2/L-3 は本タスクでも対象外（未着手）。
- 詳細所見:
  - [findings/C1_binary_analysis.md](../0149_security_code_smell_audit_fable/findings/C1_binary_analysis.md) F-1
  - [findings/C2_dynlib.md](../0149_security_code_smell_audit_fable/findings/C2_dynlib.md) F-3, F-5
  - [findings/B3_verification.md](../0149_security_code_smell_audit_fable/findings/B3_verification.md) M1, L1
  - [findings/A5_risk.md](../0149_security_code_smell_audit_fable/findings/A5_risk.md) Low-3

## 背景

Issue #860 は「解析・検証に失敗した場合、安全側ではなく『対象なし』『問題なし』と偽った判定に落ち込む」という同型の fail-open 欠陥がセキュリティクリティカル部に横断的に分布していると指摘している。このうち D1 (groupmembership) は既に #858 で個別対応済み（[0150](../0150_groupmembership_getgrgid_failclosed/)/[0151](../0151_groupmembership_failclosed/)）。本タスクは #860 に挙げられている残りの該当箇所（C1, C2, B3, A5 の各所見）をまとめて是正する。

現状コードを確認したところ、以下はいずれも未修正であることを確認済み（2026-07-19 時点）:

- `internal/security/elfanalyzer/standard_analyzer.go` の `lookupSyscallAnalysis`（C1 F-1）
- `internal/dynlib/elfdynlib/analyzer.go` の子依存パース失敗ハンドリング（C2 F-3）
- `internal/dynlib/machodylib/analyzer.go` の `HasDynamicLibDeps`（C2 F-5）
- `internal/verification/manager.go` の `collectVerificationFiles`（B3 M1）と `hasDynamicLibraryDeps`（B3 L1）
- `internal/runner/base/risk/evaluator.go` の `applyBinaryAnalysis`（A5 Low-3）

## 目的

- 「解析不能・エラー」と「対象なし・問題なし」を型／制御フロー上で区別し、前者を一律 fail-closed（Blocking / AnalysisError 相当）として処理する設計原則を、#860 が指摘した残りの箇所に適用する。
- 各修正が本来の解析対象（正常系）に副作用を与えないことをテストで保証する。

## スコープ

### 対象（本タスクで対応する）

1. **C1 F-1**: `lookupSyscallAnalysis`（`internal/security/elfanalyzer/standard_analyzer.go`）の syscall analysis store 読み取りにおける想定外エラー（`ErrHashMismatch` 以外の `default` ケース）。
2. **C2 F-3**: 子 ELF 依存パース失敗（`internal/dynlib/elfdynlib/analyzer.go`）、トップレベル `elf.NewFile`/`DynString` 失敗を「依存なし」と偽った判定にする箇所、および Mach-O 側 `parseMachODeps` 失敗（`internal/dynlib/machodylib/analyzer.go`）。
3. **C2 F-5**: `HasDynamicLibDeps`（`internal/dynlib/machodylib/analyzer.go`）の `Seek`/`io.ReadFull` 失敗を `(false, nil)` と偽った判定にする箇所。
4. **B3 M1**: `collectVerificationFiles`（`internal/verification/manager.go`）のコマンドパス解決失敗が warn + continue で検証対象集合から静かに脱落する箇所。
5. **B3 L1**: `hasDynamicLibraryDeps`（`internal/verification/manager.go`）の `DynString(elf.DT_NEEDED)` エラーを `(false, nil)` と偽った判定にする箇所。
6. **A5 Low-3**: `applyBinaryAnalysis`（`internal/runner/base/risk/evaluator.go`）の `BinaryAnalysisClass` switch に `default` 節がなく、将来の未知クラス追加時に無寄与（fail-open）へ倒れる構造。

### 対象外（別 Issue・別タスクとする）

- D1 (groupmembership) L-2, L-3: #860 記載のまま未着手。本タスクでは扱わない。
- C1 F-2, F-3, F-4, F-5, F-6, F-7, F-8（syscall 検出以外の頑健性・保守性所見。fail-open の実害が限定的、または既に fail-closed 方向）。
- C2 F-1, F-2, F-4, F-6〜F-11（探索順シャドーイング再解決、ld.so.cache フラグ照合、`ResolveRealPath` エラー種別区別、libccache キャッシュ検証強化など。#860 の「該当箇所」リストに含まれず、影響範囲・設計変更が大きいため別途検討）。
- B3 M2, L2, L3, L4, I1〜I4（dry-run 限定でない `isDeferredHashDirUnavailable`、キャッシュ排他制御、TOCTOU 系。#860 の「該当箇所」リストに含まれない）。
- A5 Low-4, Info-1（#860 の「該当箇所」リストに含まれない）。

## 現状の問題点（詳細）

### 1. C1 F-1: syscall analysis store の想定外エラーが fail-open へ落ち込む

`internal/security/elfanalyzer/standard_analyzer.go:297-332` の `lookupSyscallAnalysis` は、`LoadSyscallAnalysis` のエラーのうち `ErrHashMismatch` は `AnalysisError`（fail-closed）に倒すが、`default`（想定外の I/O エラー等）は `slog.Debug` でログした上で `StaticBinary` を返す。呼び出し元（同ファイル:168-195）はこれを「ストアにエントリなし」と同一視し `NoNetworkSymbols` へフォールスルーする。CGO/静的リンクバイナリで record 時に検出されたネットワーク syscall が、verify 時の一過性 I/O エラーで検出漏れになり得る。

### 2. C2 F-3: 子依存パース失敗が fail-soft で遷移的依存の記録漏れを許す

- `internal/dynlib/elfdynlib/analyzer.go:207-218`: 子 ELF パース失敗を `slog.Debug` で無視し traversal をスキップ。
- `internal/dynlib/elfdynlib/analyzer.go:115-127`: トップレベルの `elf.NewFile` 失敗・`DynString` エラーを `nil, nil`（依存なし）と偽った判定にする（`//nolint:nilerr`）。
- `internal/dynlib/machodylib/analyzer.go:215-221`: `parseMachODeps` 失敗を `slog.Debug` で無視する。

解決済みライブラリの子依存パースに失敗すると、そのサブツリー全体が記録・検証対象から漏れる。Mach-O 側の `HasDynamicLibDeps` は ELF マジック相当の判定（`looksLikeMachO`）でトップレベル失敗をエラー化しており非対称。

### 3. C2 F-5: HasDynamicLibDeps のシーク／読み取り失敗が「依存なし」に縮退する

`internal/dynlib/machodylib/analyzer.go:617-632` は `Seek` 失敗・`io.ReadFull` 失敗のいずれも `return false, nil` とする。この関数は「DynLibDeps が記録されているべきなのに欠けている Mach-O」を runner 側で検出するゲートであり、I/O エラーを誘発できる状況では `ErrDynLibDepsRequired` 相当の強制検出が黙ってスキップされ得る。

### 4. B3 M1: collectVerificationFiles のパス解決失敗が warn + continue で検証対象から脱落する

`internal/verification/manager.go:264-277` は `m.pathResolver.ResolvePath(command.ExpandedCmd)` の失敗を `slog.Warn` して `continue` し、`VerifyGroupFiles` 自体は成功として返る。呼び出し元 (`internal/runner/group_executor.go`) は直後のループで再度 `ResolvePath` を呼ぶため、収集時に失敗し実行ループ実行前にファイルが出現した場合、ハッシュ検証集合に含まれないままコマンドが実行され得る（fail-open の窓）。

### 5. B3 L1: hasDynamicLibraryDeps が DynString のエラーを無視する

`internal/verification/manager.go:711-715` は `elfFile.DynString(elf.DT_NEEDED)` のエラーを `(false, nil)`（動的依存なし）として扱う。動的セクションが壊れている（または細工された）ELF は `ErrDynLibDepsRequired` を回避し、dynlib 検証要求をバイパスし得る。

### 6. A5 Low-3: applyBinaryAnalysis の switch に default がなく未知クラスが無寄与になる

`internal/runner/base/risk/evaluator.go:461-477` の `BinaryAnalysisClass` switch は `Uncertain`/`HighRisk`/`Network`/`Clean` のみを列挙し `default` 節がない。将来クラスが追加された場合、当該クラスは無寄与（実質 Clean 扱い）になる。ゼロ値が `BinaryAnalysisUncertain` である点が現状の緩和要因だが、型システム上の安全側デフォルト保証がない。

## 受け入れ基準（Acceptance Criteria）

#### F-001: syscall analysis store の想定外エラーを fail-closed 化する（C1 F-1）

- **AC-01**: `lookupSyscallAnalysis` は、`LoadSyscallAnalysis` が `ErrHashMismatch` 以外の想定外エラー（キャッシュ欠損 `RecordNotFound` を除く）を返した場合、`binaryanalyzer.AnalysisError`（fail-closed）を返す。
- **AC-02**: レコードが存在しない場合（`RecordNotFound` 相当）は、従来どおり `StaticBinary` を返す（キャッシュ欠損と読み取り失敗を意味的に区別する）。
- **AC-03**: AC-01 のケースで `slog.Warn` レベル以上のログが出力される。

#### F-002: 子依存パース失敗を fail-closed 化する（C2 F-3）

- **AC-04**: `internal/dynlib/elfdynlib/analyzer.go` の子 ELF 依存パース失敗（traversal 中）は、`slog.Warn` へ格上げしたうえで、当該依存の記録漏れを解析全体の失敗として呼び出し元に伝播させる（fail-closed）。
- **AC-05**: `internal/dynlib/elfdynlib/analyzer.go` のトップレベル解析において、ELF マジックを持つファイルの `elf.NewFile` 失敗・`DynString(DT_NEEDED)` エラーは「依存なし」ではなくエラーとして呼び出し元に伝播する（Mach-O 側 `HasDynamicLibDeps` の `looksLikeMachO` 判定方式と同等の区別を導入する）。
- **AC-06**: `internal/dynlib/machodylib/analyzer.go` の `parseMachODeps` 失敗は、`slog.Warn` へ格上げしたうえで解析全体の失敗として呼び出し元に伝播する（fail-closed）。
- **AC-07**: AC-04〜AC-06 の変更後も、依存を持たない・正しくパース可能な既存バイナリの record/verify が従来どおり成功する（正常系のリグレッションがない）。

#### F-003: HasDynamicLibDeps の I/O エラーを fail-closed 化する（C2 F-5）

- **AC-08**: `internal/dynlib/machodylib/analyzer.go` の `HasDynamicLibDeps` は、`Seek` 失敗時に `(false, err)`（non-nil error）を返す。
- **AC-09**: 同関数は `io.ReadFull` 失敗時（`io.EOF`/`io.ErrUnexpectedEOF` によりファイルが Mach-O ヘッダ長に満たないことが判明する場合を除く）に `(false, err)` を返す。
- **AC-10**: 呼び出し元は AC-08/AC-09 のエラーを「DynLibDeps 要求判定不能」として fail-closed に扱う（要求ありとみなす、またはエラーとして record/verify を中断する）。

#### F-004: collectVerificationFiles のパス解決失敗を fail-closed 化する（B3 M1）

- **AC-11**: `collectVerificationFiles` はコマンドパス解決に失敗した場合、当該コマンドを検証対象集合から静かに除外するのではなく、`VerifyGroupFiles` 全体をエラーとして失敗させる。
- **AC-12**: AC-11 により、パス解決に一時的に失敗したコマンドがハッシュ検証なしで実行される経路が存在しないことをテストで確認できる。
- **AC-13**: 正常にパス解決できるコマンドのみで構成されるグループの検証は、従来どおり成功する（正常系のリグレッションがない）。

#### F-005: hasDynamicLibraryDeps の DynString エラーを fail-closed 化する（B3 L1）

- **AC-14**: `internal/verification/manager.go` の `hasDynamicLibraryDeps` は `elfFile.DynString(elf.DT_NEEDED)` がエラーを返した場合、`(false, nil)` ではなく `(false, err)`（non-nil error）を返す。
- **AC-15**: 呼び出し元は AC-14 のエラーを検証失敗として扱う（`ErrDynLibDepsRequired` のバイパスを許さない）。
- **AC-16**: `DT_NEEDED` が存在しない（動的依存なし）正常系との区別が維持される（`err == nil && len(needed) == 0` は従来どおり `(false, nil)`）。

#### F-006: applyBinaryAnalysis の未知クラスを fail-closed 化する（A5 Low-3）

- **AC-17**: `internal/runner/base/risk/evaluator.go` の `applyBinaryAnalysis` の `BinaryAnalysisClass` switch に `default` 節を追加し、`Uncertain`/`HighRisk`/`Network`/`Clean` のいずれにも一致しない値は `Uncertain` と同じ Blocking 相当の結果を返す。
- **AC-18**: 既存の 4 クラス（`Uncertain`/`HighRisk`/`Network`/`Clean`）の挙動は変更しない（正常系のリグレッションがない）。

## 非機能要件

- 既存の `make test` / `make lint` が変更後も成功すること。
- 変更対象パッケージの既存テストスイートに、各 AC に対応するテストケースを追加する（[Test Organization Guide](../../dev/developer_guide/test_organization.md) に従う）。
- ログレベルの変更（`slog.Debug` → `slog.Warn` 等）はログ出力量の大幅な増加を招かないよう、record/verify 1 回あたりの発生頻度が実運用上妥当であることを確認する。

## リスクと留意事項

- F-002/F-003/F-004/F-005 は、これまで「エラーだが無視して続行」していた経路を fail-closed 化するため、環境要因（一過性の I/O エラー、権限問題等）で record/verify が失敗しやすくなる可能性がある。ユーザー向けエラーメッセージが原因を特定しやすい内容になっていることを実装時に確認する。
- F-002 (AC-04) は「子依存パース失敗を解析全体の失敗にする」変更であり、既存の正当な（パース可能な）依存ツリーを持つ環境で誤って失敗させないよう、実装時に十分なテストケース（多階層依存、循環依存、既存の record 済みデータ）で確認する。
