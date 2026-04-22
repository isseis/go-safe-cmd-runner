# Mach-O arm64 syscall 番号解析 要件定義書

## 1. 概要

### 1.1 背景

タスク 0097 では Mach-O バイナリの `svc #0x80` スキャン結果を `SyscallAnalysis` にキャッシュし、`svc #0x80` の存在自体を高リスクとして扱う方針を実装した。

しかし、macOS arm64 向けに Go で記述されたバイナリ（本システムの `record` コマンドを含む）は、Go runtime が `libSystem.dylib` を経由せず直接 `svc #0x80` を発行するランタイムスタブを持つ。このため、タスク 0097 の方針では正規の Go バイナリが誤って高リスク判定される偽陽性問題が生じる。

実際に `record` バイナリ自身（`build/prod/record`、スキーマバージョン 15）を解析したところ、以下の `svc #0x80` が検出された：

```
0x1000ccbf0:  ldr x16, [sp, #0x18]
              ldr x0,  [sp, #0x20]
              ...
              svc #0x80               ← Go runtime syscall スタブ
```

このパターンは Go の `syscall.RawSyscall` 等が使用するランタイム内部のディスパッチスタブであり、syscall 番号は呼び出し元からスタック経由で渡される。タスク 0097 の方針ではこのバイナリを高リスクと判定し、自システムのコマンドが実行拒否される。

### 1.2 ELF バイナリとの比較

ELF バイナリの解析（タスク 0070/0072/0077）は `svc #0` の存在ではなく syscall 番号（X8 レジスタ）を解析し、ネットワーク関連 syscall の有無でリスク判定を行う。macOS arm64 も同様のアプローチを取るべきであり、`svc #0x80` の存在だけをシグナルとする現行方針は ELF との一貫性を欠く。

macOS arm64 と ELF arm64 の主な差異：

| 項目 | macOS arm64 | ELF arm64 (Linux) |
|------|------------|-------------------|
| syscall 命令 | `svc #0x80` (0xD4001001) | `svc #0` (0xD4000001) |
| syscall 番号レジスタ | X16 | X8 |
| syscall 番号の BSD prefix | 0x2000000 | なし |
| ネットワーク判定 | 本タスクで実装 | 実装済み |

### 1.3 目的

- Mach-O arm64 バイナリに対して X16 レジスタ後方スキャンにより syscall 番号を解析し、ネットワーク関連 syscall のみをネットワーク利用・高リスクとして分類する
- Go runtime スタブ関数（X16 をスタックから間接ロードする関数）を直接解析の対象から除外し、呼び出しサイト解析（Pass 2）によって実際の syscall 番号を判定する
- タスク 0097 の `SyscallAnalysis` キャッシュ基盤を継続利用しつつ、検出ロジックを精緻化する
- `record` コマンド自身を含む正規の Go バイナリが誤ってブロックされないようにする

### 1.4 スコープ

- **対象**: macOS (Darwin) arm64 Mach-O バイナリ（単一アーキテクチャおよび Fat バイナリ）
- **対象外**: ELF バイナリの解析フロー（変更しない）
- **対象外**: `mprotect(PROT_EXEC)` の引数評価（タスク 0099 で扱う）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `svc #0x80` | arm64 macOS における直接 syscall 命令（エンコード `0xD4001001`）|
| X16 | macOS arm64 における syscall 番号レジスタ |
| BSD prefix | macOS BSD syscall 番号のクラス識別子（0x2000000）。X16 の実際の値から差し引いて syscall 番号を得る |
| Pass 1 | `svc #0x80` を直接スキャンし X16 後方スキャンで syscall 番号を解析するパス |
| Pass 2 | Go ラッパー関数（`syscall.Syscall` 等）への `BL` 命令を検出し呼び出しサイトを後方スキャンして syscall 番号を解析するパス |
| Go runtime スタブ | X16 をスタックから間接ロード（`ldr x16, [sp, #N]`）する Go ランタイムの syscall ディスパッチ関数。syscall 番号は呼び出し元からスタック経由で渡される（旧スタック ABI）|
| 旧スタック ABI | Go アセンブリスタブが使う引数渡し規約。第1引数 `trap`（syscall 番号）は X0 ではなく呼び出し直前の `[SP, #8]` に格納される（`SP+0` は戻りアドレス用スロット）。`syscall.{,Raw}Syscall{,6,9}` はすべてこの ABI を使う |
| スタック経由渡し | 旧スタック ABI を使う Go スタブにおける syscall 番号の受け渡し方式。Pass 1 では解析不能（`unknown:indirect_setting`）となるため、Pass 2 の呼び出しサイト解析で補完する |
| `knownSyscallImpls` | Pass 1 から除外する Go runtime スタブ関数の集合（ELF 版と対応する概念）|
| `MacOSSyscallTable` | macOS BSD syscall 番号とネットワーク関連フラグのマッピングテーブル（`libccache` パッケージに既存実装）|

## 3. 機能要件

### FR-3.1: Mach-O arm64 syscall 番号解析エンジン（Pass 1）

#### FR-3.1.1: `svc #0x80` 検出と X16 後方スキャン

`__TEXT,__text` セクションを走査して `svc #0x80` 命令を検出した際、各命令アドレスから後方スキャンを実施して X16 レジスタへの即値設定パターンを探索すること。

検出対象パターン（`libccache/macho_analyzer.go` の `backwardScanX16` に既存実装あり）：

- `MOVZ X16, #imm`（16 bit 以下の値。BSD prefix を含まない小さいテスト値や prefix 除去後の値の確認用）
- `MOVZ X16, #hi, LSL #16` + `MOVK X16, #lo`（32 bit 値の組み合わせ）

これらのパターン以外（`ldr x16, [sp, #N]` 等のスタック経由ロード・間接ロード）は `DeterminationMethod = "unknown:indirect_setting"` として記録すること。

`ldr x16, [sp, #N]` パターンは Go の旧スタック ABI を使う syscall スタブ（`syscall.Syscall` 等）の内部でも発生する。これらの既知スタブは FR-3.1.2 の除外処理が先行するため Pass 1 の解析対象にはならず、代わりに Pass 2 の呼び出しサイト解析（FR-3.2）で syscall 番号を特定する。

制御フロー命令（`B`, `BL`, `BLR`, `BR`, `RET`, `CBZ`, `CBNZ`, `TBZ`, `TBNZ`）を後方スキャンの境界とすること。

スキャン最大命令数は `defaultMaxBackwardScan`（ELF 版の 50 命令に準拠）とすること。

#### FR-3.1.2: Go runtime スタブの除外

Mach-O バイナリに `.gopclntab` セクションが存在する場合、pclntab から Go 関数名テーブルを構築し、以下の既知 Go syscall スタブ実装関数のアドレス範囲内にある `svc #0x80` を Pass 1 の直接解析から除外すること：

- `syscall.Syscall`, `syscall.Syscall6`
- `syscall.RawSyscall`, `syscall.RawSyscall6`
- `internal/runtime/syscall.Syscall6` 等の Go バージョン依存スタブ名
- その他、ELF 版 `knownSyscallImpls` に対応する macOS arm64 のスタブ関数

この除外集合（`knownMachoSyscallImpls`）と FR-3.2.1 の Pass 2 解決対象集合（`knownGoWrappers`）は必ずしも同一ではない。`knownGoWrappers` には `runtime.syscall` 等の呼び出し元向け高レベルラッパーが追加的に含まれる場合がある（詳細は FR-3.2.1 を参照）。

`.gopclntab` が存在しない場合は除外処理なしで Pass 1 を継続すること（非 Go バイナリや stripped バイナリ向け）。

#### FR-3.1.3: BSD prefix の除去

後方スキャンで得た X16 の即値が BSD prefix（`0x2000000`）を含む場合、prefix を除去して syscall 番号を得ること。

例：`MOVZ X16, #0x0200, LSL #16` + `MOVK X16, #0x0062` → raw 値 `0x2000062` → BSD prefix 除去 → syscall 番号 `98`（`connect`）

#### FR-3.1.4: ネットワーク判定

取得した syscall 番号を `MacOSSyscallTable`（`libccache` に既存実装）で検索し、`IsNetwork` フラグを設定すること。

### FR-3.2: Go ラッパー呼び出しサイト解析（Pass 2）

#### FR-3.2.1: Go ラッパー関数の特定

`.gopclntab` セクションが存在する場合、以下の既知 Go syscall ラッパー関数のアドレスを解決すること（ELF 版 `knownGoWrappers` に対応する macOS arm64 版）。少なくとも調査で確認済みの公開ラッパーを対象とし、Go バージョン差分に応じて内部スタブ名を追加できること。

- `syscall.Syscall`, `syscall.Syscall6`
- `syscall.RawSyscall`, `syscall.RawSyscall6`
- `runtime.syscall`, `runtime.syscall6` や `internal/runtime/syscall.Syscall6` などの Go バージョン依存の内部スタブ

この Pass 2 解決対象集合（`knownGoWrappers`）は FR-3.1.2 の Pass 1 除外集合（`knownMachoSyscallImpls`）を包含する。Pass 1 除外集合は内部スタブ実装（直接 `svc #0x80` を発行する関数）のみを対象とするが、Pass 2 解決集合はその呼び出し側にあたる高レベルラッパー（`runtime.syscall` 等）も加える場合がある。

#### FR-3.2.2: 呼び出しサイトでの syscall 番号解析

`__TEXT,__text` セクション内で上記ラッパーへの `BL` 命令を検出し、各呼び出しサイトにおける第1引数（syscall 番号）を後方スキャンで解析すること。

`syscall.Syscall`/`Syscall6`/`RawSyscall`/`RawSyscall6` は Go の**旧スタック ABI**（stack-based calling convention）を使うアセンブリスタブである。第1引数 `trap`（syscall 番号）はレジスタ X0 ではなく `[SP, #8]` に格納される（`SP+0` は呼び出し規約上の戻りアドレス用スロット）。同等の呼び出し規約を使う Go バージョン依存スタブがある場合も、同じ解析手法を適用すること。

したがって、`BL` 命令直前を後方スキャンする際のターゲットは次の順序で探索すること：

1. `[SP, #8]` への書き込みストア命令（`STP xN, ..., [SP, #8]` または `STR xN, [SP, #8]`）を検出し、書き込みレジスタ xN を特定する
2. xN への即値設定命令（`MOV xN, #imm` / `MOVZ xN, #imm` / `MOVZ xN, #hi, LSL#16` + `MOVK xN, #lo`）を更に後方スキャンする

ストア命令またはそれ以前の即値設定が確定できない場合は `Number = -1`、`DeterminationMethod = "unknown:indirect_setting"` とする。

即値が確定した場合の `DeterminationMethod` は `"go_wrapper"` とすること。

#### FR-3.2.3: 結果のマージ

Pass 2 の呼び出しサイト解析結果を Pass 1 の結果とマージし、`SyscallAnalysis.DetectedSyscalls` に統合すること。各エントリの `Location` は呼び出しサイトのアドレス（`BL` 命令のアドレス）とすること。

### FR-3.3: 判定ロジックの変更

#### FR-3.3.1: ネットワーク syscall によるリスク判定

`runner` の `isNetworkViaBinaryAnalysis` における `SyscallAnalysis` キャッシュ参照の判定を以下のとおり変更すること：

| `SyscallAnalysis` の状態 | 変更前（タスク 0097）| 変更後（本タスク）|
|------------------------|---------------------|-----------------|
| `DetectedSyscalls` に `IsNetwork = true` のエントリあり | N/A（`direct_svc_0x80` シグナルで代替）| `true, true`（ネットワーク確定・高リスク）|
| `DetectedSyscalls` に `IsNetwork = true` なし（非ネットワーク syscall のみ、または空）| N/A | `SymbolAnalysis` 判定へ委譲 |
| `DetectedSyscalls` に `DeterminationMethod = "direct_svc_0x80"` のエントリあり | `true, true`（高リスク確定）| **廃止**（本判定方式を削除）|
| `SyscallAnalysis` が nil（`LoadSyscallAnalysis` が `nil, nil` を返す）| `false, false` | `false, false`（変更なし）|

#### FR-3.3.2: `SyscallAnalysis` への保存内容の変更

タスク 0097 では `svc #0x80` 検出時に `Number = -1`、`DeterminationMethod = "direct_svc_0x80"`、`Source = "direct_svc_0x80"` としていた。本タスクではこの形式を廃止し、以下に変更すること：

- `Number`: 解析で得た syscall 番号（不明の場合は `-1`）
- `IsNetwork`: `MacOSSyscallTable` による判定結果（不明の場合は `false`）
- `DeterminationMethod`: `"immediate"` / `"go_wrapper"` / `"unknown:*"` のいずれか（ELF 版と同一定数を使用）
- `Source`: `""` （空文字。ELF 直接 svc エントリと同形式）。`"direct_svc_0x80"` 値は廃止する。なお `Source` フィールド自体は ELF パス（`SourceLibcSymbolImport`）およびタスク 0100 で追加する `SourceLibsystemSymbolImport` との互換性のため `SyscallInfo` スキーマから削除しない

#### FR-3.3.3: スキーマバージョン

本タスクの変更はタスク 0097 が保存したスキーマ v15 レコードと互換性を持たない（`direct_svc_0x80` 判定方式が廃止されるため、v15 レコードを v16 の判定ロジックで安全に再利用できない）。**スキーマバージョンを v15 から v16 に変更し**、v15 以前のレコードを参照した場合は `SchemaVersionMismatchError` を返してユーザーに再 `record` を強制すること。

### FR-3.4: 既存機能への非影響

- `elfanalyzer` パッケージ（ELF バイナリの解析フロー）は変更しないこと
- `libccache` の `MacOSSyscallTable` および `backwardScanX16` は既存の動作を変更せずに再利用または参照すること

## 4. 非機能要件

### NFR-4.1: パフォーマンス

Pass 1 は `__TEXT,__text` セクション全体を 4 バイト単位でスキャンする（既存の `collectSVCAddresses` と同等）。Pass 2 は同セクションを再度走査して `BL` 命令を検出する。合計走査時間の増加は許容範囲とする。

### NFR-4.2: セキュリティ

ネットワーク syscall が確認された場合は常に `true, true`（ネットワーク検出・高リスク確定）を返す。解析エラー（`AnalysisError`）はフェイルセーフとして実行をブロックする。

X16 不明時にネットワークリスクなしと判定することで Go runtime スタブ由来の偽陽性を排除するが、実際のネットワーク syscall は Pass 2 によってカバーされるため、偽陰性のリスクは限定的である。

### NFR-4.3: 後方互換性

スキーマバージョンを v16 に変更する。v15（タスク 0097 で保存されたレコード）との後方互換性は持たない。

## 5. 受け入れ条件

### AC-1: Pass 1 - 直接 syscall 番号解析

- [ ] `MOVZ X16, #imm` + `svc #0x80` から 16 bit の syscall 番号が解析されること
- [ ] `MOVZ X16, #hi, LSL #16` + `MOVK X16, #lo` + `svc #0x80` から 32 bit syscall 番号が正しく解析されること
- [ ] BSD prefix（0x2000000）が除去されて正しい syscall 番号が得られること
- [ ] `ldr x16, [sp, #N]` のような間接ロードの場合、`DeterminationMethod = "unknown:indirect_setting"` となること
- [ ] 解析された syscall 番号が `MacOSSyscallTable` でネットワーク syscall と判定される場合、`IsNetwork = true` が設定されること
- [ ] 制御フロー命令を後方スキャンの境界として扱うこと

### AC-2: Go runtime スタブの除外

- [ ] `.gopclntab` が存在する Go バイナリに対して、既知 Go syscall スタブ実装関数のアドレス範囲内の `svc #0x80` が Pass 1 の解析から除外されること

### AC-3: Pass 2 - Go ラッパー呼び出しサイト解析

- [ ] `.gopclntab` が存在する Go バイナリに対して既知 Go ラッパー関数（少なくとも `syscall.Syscall`、`syscall.Syscall6`、`syscall.RawSyscall`、`syscall.RawSyscall6`）のアドレスが解決されること
- [ ] 既知 Go ラッパーへの `BL` 命令が検出されること
- [ ] 呼び出しサイトで `[SP, #8]` へのストア命令（旧スタック ABI における第1引数スロット）と xN への即値設定を後方スキャンすることにより syscall 番号が解析されること
- [ ] 解析された syscall 番号に対して `MacOSSyscallTable` でネットワーク判定が実施されること
- [ ] `.gopclntab` が存在しないバイナリでは Pass 2 がスキップされること

### AC-4: リスク判定変更

- [ ] ネットワーク syscall が検出されたバイナリに対して `runner` が `true, true` を返すこと
- [ ] 非ネットワーク syscall のみを含む正規の Go バイナリ（例：`record` コマンド自身）に対して `runner` が `true, true` を返さないこと（偽陽性の解消）
- [ ] X16 不明な `svc #0x80` のみを含み、ネットワーク syscall の呼び出しサイトも `SymbolAnalysis` のネットワーク根拠も存在しない場合に `false, false` を返すこと。この判定は `SyscallAnalysis` に `IsNetwork=true` エントリがないため `SymbolAnalysis` 判定に委譲され、`SymbolAnalysis` も NetworkDetected でなければ `false, false` となる（既存フローで達成）。
- [ ] `SymbolAnalysis = NetworkDetected` かつネットワーク syscall 検出の場合は `true, true`（高リスク）を返すこと。この判定は `SyscallAnalysis` の `IsNetwork=true` チェックが先行して `true, true` を返すことで達成される。
- [ ] `"direct_svc_0x80"` を判定条件に使用するコードが存在しないこと

### AC-5: スキーマ

- [ ] スキーマバージョンが v16 であること
- [ ] v15 レコードに対して `SchemaVersionMismatchError` が返されること
- [ ] `DetectedSyscalls` の各エントリに正しい `Number`、`IsNetwork`、`DeterminationMethod` が設定されること
- [ ] Mach-O 解析エントリの `Source` が `""` であること（`"direct_svc_0x80"` が使用されていないこと）
- [ ] ELF パス（`SourceLibcSymbolImport`）の `Source` フィールドが変更されていないこと

### AC-6: 既存機能への非影響

- [ ] ELF バイナリの解析フローが変更されないこと
- [ ] 既存のテストがすべてパスすること

## 6. テスト方針

### 6.1 Pass 1 テスト（ユニットテスト）

| テストケース | 検証内容 |
|-------------|---------|
| `MOVZ X16, #98` + `svc #0x80` | `Number=98`, `IsNetwork=true`, `DeterminationMethod="immediate"` |
| `MOVZ X16, #3` + `svc #0x80` | `Number=3`, `IsNetwork=false`, `DeterminationMethod="immediate"` |
| `MOVZ X16, #0x0200, LSL #16` + `MOVK X16, #0x0062` + `svc #0x80` | BSD prefix が除去され `Number=98` になること |
| `ldr x16, [sp, #N]` + `svc #0x80` | `Number=-1`, `IsNetwork=false`, `DeterminationMethod="unknown:indirect_setting"` |
| 制御フロー命令を挟んだ `svc #0x80` | 後方スキャンが制御フロー命令で停止すること |

### 6.2 Pass 2 テスト（ユニットテスト）

旧スタック ABI（`[SP, #8]` = 第1引数スロット）の解析を検証する。

| テストケース | 検証内容 |
|-------------|---------|
| `MOV xN, #98` + `STP xN, ..., [SP, #8]` + `BL syscall.Syscall` | `Number=98`, `IsNetwork=true`, `DeterminationMethod="go_wrapper"` |
| `MOV xN, #3` + `STR xN, [SP, #8]` + `BL syscall.Syscall6` | `Number=3`, `IsNetwork=false`, `DeterminationMethod="go_wrapper"` |
| `MOV xN, #49` + `STP xN, ..., [SP, #8]` + `BL syscall.RawSyscall` | `Number=49`, `IsNetwork=false`, `DeterminationMethod="go_wrapper"` |
| `[SP, #8]` への書き込みが間接ロード由来の `BL syscall.RawSyscall6` | `Number=-1`, `DeterminationMethod="unknown:indirect_setting"` |
| 制御フロー命令（`BL other`）を挟んだ `BL syscall.Syscall` | 後方スキャンが制御フロー命令で停止し `Number=-1` となること |
| `.gopclntab` なしバイナリ | Pass 2 がスキップされること |

### 6.3 リスク判定テスト

| テストケース | 検証内容 |
|-------------|---------|
| `SyscallAnalysis` に `IsNetwork=true` エントリあり | `runner` が `true, true` を返すこと |
| `SyscallAnalysis` に `IsNetwork=true` エントリなし | `runner` が `SymbolAnalysis` 判定に委譲すること |
| `SyscallAnalysis` が nil（`LoadSyscallAnalysis` が `nil, nil` を返す）| `runner` が `false, false` を返すこと（変更なし）|
| v15 スキーマレコード（`SchemaVersionMismatchError`）| `runner` が `true, true` を返すこと（既存のフェイルセーフ動作）|

### 6.4 統合テスト

| テストケース | 検証内容 |
|-------------|---------|
| `record` コマンド自身（Go バイナリ）| `record` → `runner` で実行が拒否されないこと（偽陽性なし）|
| ネットワーク syscall を持つテスト Mach-O | `record` → `runner` で `true, true` が返されること |

### 6.5 テストフィクスチャ

- ネットワーク syscall を含む arm64 Mach-O バイナリのフィクスチャが不足する場合は追加生成する
- `record` コマンド自身（`build/prod/record`）をリグレッションテストの対象とすることを検討する

## 7. 先行タスクとの関係

| 先行タスク | 関連 | 備考 |
|----------|------|------|
| 0073 (Mach-O ネットワーク検出) | svc スキャン基盤 | `collectSVCAddresses` を Pass 1 の基盤として継続利用 |
| 0097 (svc キャッシュ統合) | `SyscallAnalysis` スキーマ v15 | 本タスクは v16 に変更し `direct_svc_0x80` 判定方式を廃止 |
| 0100 (libSystem.dylib キャッシュ) | `MacOSSyscallTable`・`backwardScanX16` | 本タスクの Pass 1 で利用する既存実装が含まれる |
| 0099 (mprotect PROT_EXEC) | mprotect 引数評価 | 本タスクで実装する X16 後方スキャンを再利用予定 |
