# Mach-O arm64 syscall 静的解析・キャッシュ統合・CGO フォールバック 要件定義書

## 1. 概要

### 1.1 背景

タスク 0073 では Mach-O バイナリのインポートシンボルを解析し、ネットワーク関連シンボルを
検出する機能を実装した。`svc #0x80` 命令（arm64 macOS のシステムコール命令）の存在確認も
行っているが、以下の不足がある：

1. **syscall 番号の未特定**: `svc #0x80` を検出した場合、`x16` レジスタへの即値設定を
   解析せずに即 `AnalysisError`（高リスク）を返す。ネットワーク関連 syscall であるか否かを
   区別できないため、socket/connect 等を明示的に確認した場合に `NetworkDetected` を
   返すことができない。

2. **解析結果のキャッシュ未保存**: `svc #0x80` 検出結果を `SyscallAnalysis` フィールドに
   保存していない。`runner` 実行時に毎回 live 解析が行われ、パフォーマンス上の無駄がある。
   さらに `AnalysisError` はそのままブロックとなるため、syscall 番号が判明した場合でも
   キャッシュから正確な結果を復元できない。

3. **CGO/動的バイナリへの syscall 解析フォールバック未実装**: ELF 版タスク 0077 では
   `.dynsym` 解析が `NoNetworkSymbols` を返した動的バイナリに対して `SyscallAnalysis` を
   フォールバック適用する。Mach-O では同等の仕組みが存在しない。macOS の正規バイナリは
   `libSystem.dylib` 経由で syscall を発行するため適用頻度は限られるが、
   libSystem.dylib を迂回するバイナリの検出に有用である。

本タスクは上記 3 点を解消し、タスク 0095 FR-4.2 / FR-4.4 / FR-4.5 を実装する。

### 1.2 macOS arm64 における syscall の仕組み

macOS arm64 における syscall の呼び出し規約は Linux arm64 と以下の点で異なる：

| 項目 | Linux arm64 | macOS arm64 |
|------|-------------|-------------|
| 命令 | `svc #0` (エンコード `0xD4000001`) | `svc #0x80` (エンコード `0xD4001001`) |
| syscall 番号レジスタ | `w8`/`x8` | `w16`/`x16` |
| BSD クラスプレフィックス | なし | `0x2000000`（`x16` に含まれる） |
| 引数レジスタ | `x0`–`x5` | `x0`–`x5`（同一） |

**BSD クラスプレフィックス**: Darwin arm64 では `x16` レジスタの上位ビットにクラス区分が
含まれる。一般的な POSIX/BSD syscall は `x16 = 0x2000000 | number` となる。
解析時は `x16 & 0x00FFFFFF` で syscall 番号（下位 24 ビット）を取り出してテーブルと照合する。

**正規バイナリでの希少性**: macOS 上の正規バイナリ（Go バイナリ・C バイナリを含む）は
`libSystem.dylib` を経由して syscall を発行するため、`svc #0x80` はバイナリ本体の
`__TEXT,__text` セクションには現れない。`svc #0x80` の存在は libSystem.dylib を迂回した
直接 syscall を示し、それ自体が異常なシグナルである。

### 1.3 目的

- `svc #0x80` から `x16` レジスタへの即値設定を後方スキャンし、BSD syscall 番号を特定する
- ネットワーク関連 syscall（`socket`=97, `connect`=98 等）を検出した場合に
  `NetworkDetected` を返し、現行の「即 AnalysisError」を改善する
- 解析結果を `SyscallAnalysis` フィールドに保存し、`runner` がキャッシュを利用できるようにする
- `SymbolAnalysis` が `NoNetworkSymbols` を返した Mach-O バイナリにも syscall 解析を
  フォールバック適用する（ELF 版タスク 0077 の Mach-O 対応）

### 1.4 スコープ

- **対象**: macOS (Darwin) arm64 Mach-O バイナリ（単一アーキテクチャおよび Fat バイナリ）
- **対象**: `__TEXT,__text` セクションに `svc #0x80` が存在するバイナリ
- **対象外**: x86_64 Mach-O バイナリ（macOS 固有の x86 syscall 命令は別途扱う）
- **対象外**: `svc #0x80` による `mprotect(PROT_EXEC)` の引数評価（タスク 0099 で扱う）
- **対象外**: libSystem.dylib syscall ラッパーキャッシュ（タスク 0100 で扱う）
- **対象外**: iOS/iPadOS/tvOS/watchOS 向けバイナリ
- **前提**: タスク 0095 FR-4.3（`LC_LOAD_DYLIB` 整合性検証）がタスク 0096 で完了していること

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `svc #0x80` | arm64 macOS における直接 syscall 命令（エンコード `0xD4001001`）。正規バイナリは `libSystem.dylib` 経由で呼ぶため通常は現れない |
| `x16`/`w16` | macOS arm64 における syscall 番号レジスタ。Linux arm64 の `x8`/`w8` に相当 |
| BSD クラスプレフィックス | `x16` レジスタ上位ビットのクラス区分。BSD/POSIX syscall は `0x2000000 | number` の形式 |
| BSD syscall 番号 | `x16 & 0x00FFFFFF` で取り出す下位 24 ビットの値。ネットワーク関連テーブルと照合する |
| `MachineCodeDecoder` | `elfanalyzer` パッケージが定義する命令デコードインターフェース |
| `ARM64DarwinDecoder` | 本タスクで実装する macOS 向け arm64 命令デコーダ。`x16`/`w16` を対象とする |
| `DarwinARM64SyscallTable` | 本タスクで実装する macOS BSD syscall 番号テーブル |
| `SyscallAnalysis` | `fileanalysis.Record` フィールド。syscall 解析結果をキャッシュする |
| フォールバック SyscallAnalysis | `SymbolAnalysis` が `NoNetworkSymbols` の Mach-O バイナリに対して適用する syscall 解析（FR-4.5） |

## 3. 機能要件

### FR-3.1: arm64 macOS 機械語 syscall 静的解析（FR-4.2 対応）

#### FR-3.1.1: `ARM64DarwinDecoder` の実装

`elfanalyzer.MachineCodeDecoder` インターフェースの macOS arm64 向け実装を提供すること。
タスク 0072 の `elfanalyzer.ARM64Decoder` のロジックを基盤として再利用し、
以下の差分を適用した `ARM64DarwinDecoder` を実装する：

| メソッド | ELF arm64 (ARM64Decoder) | macOS arm64 (ARM64DarwinDecoder) |
|----------|--------------------------|----------------------------------|
| `IsSyscallInstruction` | `svc #0`（即値=0）を検出 | `svc #0x80`（即値=0x80）を検出 |
| `ModifiesSyscallNumberRegister` | `w8`/`x8` の書き込みを検出 | `w16`/`x16` の書き込みを検出 |
| `IsImmediateToSyscallNumberRegister` | `w8`/`x8` への即値設定を検出 | `w16`/`x16` への即値設定を検出 |
| `IsControlFlowInstruction` | 同一（B, BL, BLR, BR, RET, CBZ, CBNZ, TBZ, TBNZ） | 同一 |
| `GetCallTarget` | BL 命令のターゲットアドレスを返す | 同一 |
| `IsImmediateToFirstArgRegister` | `x0`/`w0` への即値設定を検出 | 同一 |
| `InstructionAlignment` | 4 バイト | 4 バイト（同一） |

**実装場所**: `machoanalyzer` パッケージ内、または `elfanalyzer` と共用できる上位パッケージ。
設計上の判断は `02_architecture.md` で確定する。

**`elfanalyzer.ARM64Decoder` との共用範囲**: `IsSyscallInstruction`、
`ModifiesSyscallNumberRegister`、`IsImmediateToSyscallNumberRegister` の 3 メソッドのみが
差分を持つ。制御フロー・引数レジスタ関連のメソッドは実装を共用するか、同一ロジックを複製する
（設計フェーズで判断）。

**Darwin 固有の即値 materialization への対応**:
Darwin の BSD syscall 値 `0x2000000 | number` は、多くの場合 1 命令の `MOV` だけでは
表現されず、`MOVZ`/`MOVK` など複数命令で `x16` を構築する必要がある。
したがって `ARM64DarwinDecoder.IsImmediateToSyscallNumberRegister` 自体は
「単一命令が `x16`/`w16` に即値を書き込むか」を返す責務に留め、
複数命令をまたぐ Darwin 固有の即値復元は FR-3.1.3 の後方スキャン側で扱うこと。

#### FR-3.1.2: `DarwinARM64SyscallTable` の実装

`elfanalyzer.SyscallNumberTable` インターフェースの macOS BSD syscall 向け実装を提供すること。

**テーブル参照の仕組み**:
1. 後方スキャン側で `x16` レジスタ最終値が `0x2000000` 以上かつ `0x2FFFFFF` 以下であれば
   BSD クラスと判定する
2. 後方スキャン側で BSD syscall 番号 = `value & 0x00FFFFFF` を取り出す
3. `DarwinARM64SyscallTable` は正規化済み BSD syscall 番号を受け取り、syscall 名と
   ネットワーク関連フラグを返す

**ネットワーク関連 BSD syscall テーブル**（最小限、本タスクでのスコープ）:

| syscall | 番号 | ネットワーク関連 |
|---------|------|----------------|
| `recvmsg` | 27 | ✅ |
| `sendmsg` | 28 | ✅ |
| `recvfrom` | 29 | ✅ |
| `accept` | 30 | ✅ |
| `socket` | 97 | ✅ |
| `connect` | 98 | ✅ |
| `bind` | 104 | ✅ |
| `listen` | 106 | ✅ |
| `sendto` | 133 | ✅ |
| `socketpair` | 135 | ✅ |
| `getsockname` | 32 | ✅ |
| `getpeername` | 31 | ✅ |
| `getsockopt` | 118 | ✅ |
| `setsockopt` | 105 | ✅ |
| `accept4` | 541 | ✅ |
| `recvmmsg` | 548 | ✅ |
| `sendmmsg` | 547 | ✅ |

**非 BSD クラスプレフィックスの扱い**:
後方スキャンで復元した `x16` 値に `0x2000000` プレフィックスが付かない場合（Mach トラップ等）は、
「syscall 番号不明」として高リスク扱いとする（FR-3.1.3 の「番号不明」ケースを参照）。

**テーブルの完全性**: 上記以外の BSD syscall 番号（ファイル I/O 等の非ネットワーク syscall）が
`x16` に設定されている場合は `IsNetwork: false` として扱う。ネットワーク関連 syscall 番号を
テーブルが正しくカバーしていることが最低要件であり、全 BSD syscall の網羅は本タスクでは不要。

#### FR-3.1.3: `__TEXT,__text` セクションの syscall 解析

`svc #0x80` 命令を起点に後方スキャンし、BSD syscall 番号を特定すること。
スキャンロジックは ELF 版（`elfanalyzer.SyscallAnalyzer`）と同一の後方スキャンアルゴリズムを
`ARM64DarwinDecoder` と `DarwinARM64SyscallTable` を使って適用する。

**スキャン結果の分類**:

| 状態 | 判定 | 意味 |
|------|------|------|
| BSD ネットワーク関連 syscall が 1 件以上特定された | `NetworkDetected` | socket, connect 等を直接呼び出している |
| `svc #0x80` は存在するが全て番号不明（間接設定等） | `AnalysisError`（高リスク） | 現行動作を維持 |
| `svc #0x80` は存在するが全て非ネットワーク BSD syscall のみ特定された | `AnalysisError`（高リスク） | `svc #0x80` 自体が異常なシグナルのため高リスク扱いを維持する |
| `svc #0x80` が存在しない | `NoNetworkSymbols` | 現行動作を維持 |

**根拠（非ネットワーク BSD syscall が特定された場合も高リスクとする理由）**:
正規の macOS バイナリは `svc #0x80` を直接含まない。たとえ非ネットワーク syscall
（例: `write`=4）であっても、`svc #0x80` の存在自体が libSystem.dylib を迂回した
異常な挙動を示すため、高リスク扱いとする。

**Fat バイナリ**:
全スライスを解析し最も深刻な結果を採用する。タスク 0073 の方針に準拠する。

#### FR-3.1.4: `DetectedSyscalls` への記録形式

`SyscallInfo` の各フィールドを以下のように設定すること：

| フィールド | 設定値 |
|-----------|--------|
| `Number` | BSD syscall 番号（`x16 & 0x00FFFFFF`）。不明な場合は -1 |
| `Name` | BSD syscall 名（例: `"socket"`）。テーブルに存在しない場合は空文字列 |
| `IsNetwork` | BSD ネットワーク関連テーブルに含まれる場合 `true` |
| `Location` | `svc #0x80` 命令の仮想アドレス |
| `DeterminationMethod` | 番号特定時は既存 ELF 実装と整合する `"immediate"`、不明時は `"unknown:indirect_setting"` 等 |

`Architecture` は `SyscallInfo` ではなく `SyscallAnalysisData` 側のフィールドとして `"arm64"`
を設定すること。

**`AnalysisWarnings` への出力**:
番号不明の `svc #0x80` については `AnalysisWarnings` に記録する（ELF 版と同様）。

### FR-3.2: `record` 時の SyscallAnalysis 保存（FR-4.4 対応）

#### FR-3.2.0: 解析インターフェースの汎用化

現行の `filevalidator.SyscallAnalyzerInterface` および関連アダプタは
`AnalyzeSyscallsFromELF(*elf.File)` を前提としており、Mach-O の syscall 解析結果をそのまま
流し込めない。したがって本タスクでは、以下のいずれかを満たす設計変更を要件に含めること。

1. `filevalidator` / `runner` 間の syscall 解析受け渡しをフォーマット非依存の抽象に一般化する
2. 既存 ELF 経路を維持したまま、Mach-O 専用の解析・保存経路を追加する

いずれの方式でも、ELF の既存フローとテストを壊さず、`fileanalysis.Record.SyscallAnalysis` へ
同じスキーマで保存できることを必須とする。

#### FR-3.2.1: Mach-O バイナリへの SyscallAnalysis 記録

`record` コマンドが Mach-O バイナリに対して FR-3.1 の解析を実行し、
結果を `fileanalysis.Record.SyscallAnalysis` に保存すること。

**保存対象**: 以下のいずれかの条件を満たす場合に `SyscallAnalysis` を記録する：
- `svc #0x80` が 1 件以上検出された（ネットワーク・非ネットワーク・番号不明を問わず）
- FR-4.5 フォールバック（FR-3.3 参照）として syscall 解析が実行された場合

**保存しない場合**: `svc #0x80` が全く検出されず、FR-4.5 フォールバックも未適用の場合は
`SyscallAnalysis` を `nil` のままにする（`omitempty` によりシリアライズ時に省略）。

#### FR-3.2.2: 既存フィールドの使用

タスク 0095 §9 スキーマ方針に準拠し、`fileanalysis.Record.SyscallAnalysis`
（`*SyscallAnalysisData`）に保存する。新規フィールドは追加しない。

`SyscallAnalysisData` の `Architecture` フィールドに `"arm64"` を設定する。

#### FR-3.2.3: スキーマバージョン

本タスクでは **スキーマバージョンを変更しない**。`SyscallAnalysis` は既存の任意フィールドであり、
Mach-O バイナリが `SyscallAnalysis` を持たない場合でも `runner` はエラーとしない。
これは ELF 版タスク 0070 が静的バイナリに対して `SyscallAnalysis` を必須とするのと異なり、
Mach-O では `svc #0x80` の存在が稀であることによる。

#### FR-3.2.4: `--force` フラグとの整合性

`record --force` 実行時は `SyscallAnalysis` も新しい値で上書きする。

#### FR-3.2.5: SymbolAnalysis との保存順序

`SyscallAnalysis` は `SymbolAnalysis` と同一の `record` フローで記録する。
`SymbolAnalysis` が `NetworkDetected` を返した場合（インポートシンボルでネットワークを検出）、
`SyscallAnalysis` は実行しなくてよい（高コストを避けるため）。

**注記**: `SymbolAnalysis` が `NetworkDetected` の場合、`runner` は `SymbolAnalysis` キャッシュを
参照して判定できるため、`SyscallAnalysis` は不要。

### FR-3.3: CGO/動的 Mach-O バイナリへの syscall 解析フォールバック（FR-4.5 対応）

#### FR-3.3.1: フォールバック適用条件

以下を全て満たす場合に、`record` 時に FR-3.1 の syscall 解析をフォールバック適用すること：

1. Mach-O バイナリである
2. `SymbolAnalysis`（インポートシンボル解析）の結果が `NoNetworkSymbols` である
3. `SyscallAnalysis` が未実行（`svc #0x80` が存在しなかった場合を含む）

**適用意図**: macOS の正規 Go バイナリは `libSystem.dylib` 経由で syscall を発行するため、
`libSystem.dylib` のシンボル（`socket` 等）が `.dynsym` に現れ `NetworkDetected` となる。
本フォールバックが主に検出するのは「難読化・マルウェア的バイナリ」であり、正規バイナリへの
副作用は小さい。

ELF 版タスク 0077 の Mach-O 対応版として位置づける。

#### FR-3.3.2: フォールバック実行と結果の保存

フォールバックとして FR-3.1 の syscall 解析を実行し、結果を `SyscallAnalysis` に保存する。
結果の分類（NetworkDetected / AnalysisError / NoNetworkSymbols）は FR-3.1.3 と同一のルールに従う。

**`SyscallAnalysis` が nil のまま保存する場合**:
`svc #0x80` が存在せず、フォールバック解析でも `svc #0x80` が検出されなかった場合は、
`SyscallAnalysis` を `nil` のままとする（不要なエントリを生成しない）。

### FR-3.4: `runner` 実行時の SyscallAnalysis キャッシュ利用（FR-4.4 対応）

#### FR-3.4.1: SyscallAnalysis の参照

`runner` がコマンドを実行する前に行うネットワーク判定において、
Mach-O バイナリかつ `SyscallAnalysis` が記録済みの場合、
記録済みの結果を参照してネットワーク判定を行うこと。

現行の `isNetworkViaBinaryAnalysis` では、`SymbolAnalysis` キャッシュが存在する場合は
live 解析を行わない。本タスクでは、`SymbolAnalysis` が `NoNetworkSymbols` かつ
`SyscallAnalysis` が記録済みの場合に `SyscallAnalysis` を追加参照するフローを導入する。

ELF 版タスク 0077 `FR-3.3.1` の Mach-O 対応として、同一のフォールバックロジックを Mach-O に
適用する。

#### FR-3.4.2: SyscallAnalysis からの判定ロジック

| SyscallAnalysis の状態 | 判定 | 動作 |
|----------------------|------|------|
| `SyscallAnalysis` が nil（記録なし） | そのまま通過 | SymbolAnalysis の結果（NoNetworkSymbols）を採用 |
| ネットワーク関連 syscall が検出されている（`HasNetworkSyscalls` 相当） | `NetworkDetected` | 実行前に確認が必要 |
| 高リスク信号あり（`IsHighRisk` 相当: `Number = -1` の syscall、または同等の未知判定） | `AnalysisError` | 実行をブロック |
| `SyscallAnalysis` 記録済みだがネットワーク・高リスクいずれもなし | そのまま通過 | `NoNetworkSymbols` 相当 |

**`HasNetworkSyscalls` / `IsHighRisk` の判定**: `DetectedSyscalls` に `IsNetwork: true` が
1 件以上あれば `HasNetworkSyscalls`。`DetectedSyscalls` に `Number = -1` のエントリが 1 件以上
あれば `IsHighRisk` とする。`AnalysisWarnings` および `DeterminationMethod = "unknown:..."` は
監査・説明用の補助情報として扱う。
詳細は `03_detailed_specification.md` で定義する。

#### FR-3.4.3: SyscallAnalysis の読み込みエラー処理

ELF 版タスク 0077 と同様のエラーハンドリングを適用する：

| エラー種別 | 処理 |
|-----------|------|
| `ErrRecordNotFound` / `ErrNoSyscallAnalysis` | `NoNetworkSymbols` のまま通過（フォールバックなし） |
| `ErrHashMismatch` | `AnalysisError` を返す（安全側フェイルセーフ） |
| `SchemaVersionMismatchError` | ログ警告を出し、live 解析にフォールバック |
| その他エラー | `NoNetworkSymbols` のまま通過 |

## 4. 非機能要件

### NFR-4.1: パフォーマンス

#### NFR-4.1.1: `record` 時の解析時間

`svc #0x80` を含まない通常の Mach-O バイナリへの追加オーバーヘッドは最小化すること。
`svc #0x80` が存在しない場合はセクション走査のみで終了するため、既存比較での大きな劣化はない。

タスク 0095 NFR-5.1 に従い、既存比で実行時間が **2 倍を超えない**こと。

#### NFR-4.1.2: `runner` 実行時のオーバーヘッド

`SyscallAnalysis` キャッシュのロードは JSON 読み込みのみ。
キャッシュヒット時の追加オーバーヘッドは 10ms 未満を目標とする。

### NFR-4.2: セキュリティ

#### NFR-4.2.1: 安全側フォールバック

`svc #0x80` は検出されるがシスコール番号が不明の場合は、既存の `AnalysisError`（高リスク）を
維持し、実行をブロックする。番号が不明な直接 syscall を安全と見なさない。

#### NFR-4.2.2: Fat バイナリのクロスアーキテクチャ対策

Fat バイナリの全スライスを解析し、最も深刻な結果を採用する。
タスク 0073 の方針に準拠し、1 つのスライスが高リスクであれば全体が高リスクとなる。

### NFR-4.3: 保守性

#### NFR-4.3.1: ELF 版との一貫性

`ARM64DarwinDecoder` と `DarwinARM64SyscallTable` の実装は、
ELF 版（`ARM64Decoder`・`ARM64LinuxSyscallTable`）と一貫した構造を維持すること。
差分は syscall 命令の即値・syscall 番号レジスタ・syscall テーブルのみとする。

#### NFR-4.3.2: 外部依存の追加なし

ELF 版と同様に `golang.org/x/arch/arm64/arm64asm` のみを使用し、外部コマンドへの依存を持たない。

### NFR-4.4: テスタビリティ

Mach-O バイナリの解析は macOS 環境でのみ動作する。テストは macOS 上で実行できる必要がある。

**ユニットテスト**: `svc #0x80` を含む最小限の arm64 バイト列をデコーダに渡して検証する。
ELF 版タスク 0072 のユニットテスト（`arm64_decoder_test.go`）と同様のアプローチ。

**統合テスト**: 最小 Mach-O フィクスチャは、可能な限り既存 Mach-O テストと同様に in-process
生成を優先する。実バイナリ fixture を `testdata/` に置くのは、リンカ出力との差異が重要で
in-process 生成では十分に再現できないケースに限る。いずれの場合も生成方法を明記し、再現性を
確保する。

## 5. 受け入れ条件

### AC-1: arm64 macOS 命令デコード

- [ ] `ARM64DarwinDecoder` が `svc #0x80`（即値=0x80）を `IsSyscallInstruction` で検出できること
- [ ] `ARM64DarwinDecoder` が `svc #0`（即値=0）を `IsSyscallInstruction` で検出しないこと
- [ ] `ARM64DarwinDecoder` が `w16`/`x16` への書き込みを `ModifiesSyscallNumberRegister` で検出できること
- [ ] `ARM64DarwinDecoder` が `w8`/`x8` への書き込みを `ModifiesSyscallNumberRegister` で検出しないこと
- [ ] `mov w16, #97` のような単一命令の即値設定を `IsImmediateToSyscallNumberRegister` で検出できること
- [ ] `MOVZ`/`MOVK` 等の複数命令で materialize された `x16 = 0x2000061` を、FR-3.1.3 の後方スキャンが認識できること

### AC-2: BSD syscall テーブル

- [ ] BSD syscall 番号 97 を渡した `DarwinARM64SyscallTable` が `socket` を返すこと
- [ ] BSD syscall 番号 98 を渡した `DarwinARM64SyscallTable` が `connect` を返すこと
- [ ] 後方スキャン側が非 BSD クラス（`0x2000000` プレフィックスなし）の `x16` 値を「不明」として扱うこと
- [ ] `DarwinARM64SyscallTable` が BSD クラスだが非ネットワーク番号（例: `write`=4）に対して `IsNetwork: false` を返すこと

### AC-3: syscall 解析結果の分類

- [ ] `svc #0x80` + `MOVZ`/`MOVK` 等で materialize された `x16 = 0x2000061`（socket）を含む Mach-O に対して `NetworkDetected` が返されること
- [ ] `svc #0x80` + `MOVZ`/`MOVK` 等で materialize された `x16 = 0x2000004`（write）のみを含む Mach-O に対して `AnalysisError` が返されること
- [ ] `svc #0x80` + 番号不明（スタックロード等から `x16` 設定）を含む Mach-O に対して `AnalysisError` が返されること
- [ ] `svc #0x80` を含まない Mach-O に対して既存の動作（`NoNetworkSymbols` または `NetworkDetected`）が維持されること
- [ ] Fat バイナリで 1 スライスが `NetworkDetected`、別スライスが `NoNetworkSymbols` の場合、全体として `NetworkDetected` が返されること

### AC-4: `record` 拡張

- [ ] `svc #0x80` を含む Mach-O に対して `SyscallAnalysis` が `record` 時に保存されること
- [ ] `SyscallAnalysis.Architecture` が `"arm64"` であること
- [ ] `svc #0x80` が存在しない Mach-O に対して、FR-4.5 フォールバックを経て `SyscallAnalysis` が nil のままであること（`svc #0x80` が検出されない場合）
- [ ] `SymbolAnalysis` が `NetworkDetected` の Mach-O に対して `SyscallAnalysis` が実行されないこと
- [ ] `record --force` で `SyscallAnalysis` が更新されること
- [ ] 既存の `ContentHash`・`SymbolAnalysis`・`DynLibDeps` フィールドが変更されないこと

### AC-5: CGO フォールバック（FR-4.5）

- [ ] `SymbolAnalysis` が `NoNetworkSymbols` の Mach-O に対して syscall 解析がフォールバック実行されること
- [ ] フォールバック解析が `NetworkDetected` を返した場合に `record` の `SyscallAnalysis` に保存されること

### AC-6: `runner` キャッシュ利用

- [ ] `SymbolAnalysis` が `NoNetworkSymbols` かつ `SyscallAnalysis` にネットワーク syscall が記録されている場合、`NetworkDetected` が返されること
- [ ] `SymbolAnalysis` が `NoNetworkSymbols` かつ `SyscallAnalysis` が高リスク（番号不明 `svc #0x80`）の場合、`AnalysisError` が返されること
- [ ] `SyscallAnalysis` が nil の場合はそのまま通過し（`NoNetworkSymbols`）live 解析フォールバックが呼ばれないこと
- [ ] `ErrHashMismatch` 時に `AnalysisError` が返されること

### AC-7: 既存機能への非影響

- [ ] ELF バイナリの解析フロー（`SyscallAnalysis` 含む）が変更されないこと
- [ ] Mach-O インポートシンボル解析（`SymbolAnalysis`）が変更されないこと
- [ ] 既存のテストがすべてパスすること

## 6. テスト方針

### 6.1 `ARM64DarwinDecoder` ユニットテスト

テスト用 arm64 バイト列（機械語）を直接デコーダに渡してテストする。

| テストケース | 検証内容 |
|-------------|---------|
| `svc #0x80` のエンコード | `IsSyscallInstruction` が true を返すこと |
| `svc #0` のエンコード | `IsSyscallInstruction` が false を返すこと（Linux 版と区別） |
| `mov w16, #97` | `IsImmediateToSyscallNumberRegister` が true と値を返すこと |
| `mov w8, #198` | `ModifiesSyscallNumberRegister` が false を返すこと（w8 は対象外） |
| `ldr x16, [sp, #8]` | `ModifiesSyscallNumberRegister` が true、`IsImmediateToSyscallNumberRegister` が false を返すこと |
| `bl <addr>` | `IsControlFlowInstruction` が true を返すこと |

### 6.2 `DarwinARM64SyscallTable` ユニットテスト

| テストケース | 検証内容 |
|-------------|---------|
| `97` (socket) | `IsNetwork: true`, `Name: "socket"` を返すこと |
| `98` (connect) | `IsNetwork: true`, `Name: "connect"` を返すこと |
| `4` (write) | `IsNetwork: false` を返すこと |
| 後方スキャンで `0x4000000 | 97`（非 BSD クラス） | 「不明」扱いになること |
| 後方スキャンで `97`（プレフィックスなし） | 「不明」扱いになること |

### 6.3 syscall 解析ユニットテスト（バイト列ベース）

| テストケース | 検証内容 |
|-------------|---------|
| `MOVZ`/`MOVK` 等で `x16 = 0x2000061` を materialize + `svc #0x80` | socket(97) を検出し `NetworkDetected` になること |
| `MOVZ`/`MOVK` 等で `x16 = 0x2000062` を materialize + `svc #0x80` | connect(98) を検出し `NetworkDetected` になること |
| `MOVZ`/`MOVK` 等で `x16 = 0x2000004` を materialize + `svc #0x80` | write(4) を検出し `AnalysisError`（高リスク）になること |
| `ldr x16, [sp, #8]` + `svc #0x80` | 番号不明で `AnalysisError` になること |
| `svc #0x80` のみ（スキャン範囲内に即値設定なし） | 番号不明で `AnalysisError` になること |
| `svc #0x80` なし | `NoNetworkSymbols`（または `NetworkDetected`）が返されること（既存動作が維持される） |
| 制御フロー命令（`bl`）を越えて `svc #0x80` | 高リスク判定（スキャン境界） |

### 6.4 `record` / `runner` 統合テスト

| テストケース | 検証内容 |
|-------------|---------|
| `svc #0x80` + socket → `record` | `SyscallAnalysis` が保存されること |
| `svc #0x80` + socket → `runner` | `NetworkDetected` を返すこと（キャッシュ利用） |
| `svc #0x80` + 番号不明 → `runner` | `AnalysisError`（キャッシュ利用） |
| `SyscallAnalysis` なし | `NoNetworkSymbols`（キャッシュ miss 時はそのまま通過） |
| ErrHashMismatch | `AnalysisError` |
| CGO フォールバック: `SymbolAnalysis = NoNetworkSymbols` かつ `svc #0x80` + socket | `record` 後に `SyscallAnalysis` が保存され、`runner` で `NetworkDetected` |

### 6.5 テストフィクスチャ

`svc #0x80` + ネットワーク syscall（socket, connect 等）を含む最小 Mach-O バイナリを
原則として in-process 生成し、必要な場合のみ `testdata/` に実バイナリ fixture を配置する。
いずれの場合も生成スクリプトまたは生成手順を明記し、再現性を確保する。

**フィクスチャの種類**:
1. `svc #0x80` + `socket`（BSD 番号 97）を含む最小 arm64 Mach-O
2. `svc #0x80` + 番号不明（スタックロードから `x16` 設定）の最小 arm64 Mach-O
3. `svc #0x80` を含まない通常のシンボル解析テスト用 Mach-O（既存フィクスチャを活用）

## 7. 設計上の制約と限界

1. **正規 macOS バイナリへの適用頻度の低さ**: macOS の正規バイナリ（Go・C を含む）は
   `libSystem.dylib` 経由で syscall を発行するため `svc #0x80` を含まない。本機能が主に
   検出するのは異常なバイナリである。

2. **非ネットワーク BSD syscall への高リスク扱い**: `write`(4) 等の非ネットワーク BSD syscall が
   特定されても `svc #0x80` 自体が異常なためを `AnalysisError` とする。
   これは誤検知の可能性を含まないが、正規利用ケースもほぼ存在しないため許容する。

3. **Go ラッパー解析（Pass 2）の非適用**: ELF 版タスク 0077 では `GoWrapperResolver` による
   Pass 2 解析（`BL syscall.RawSyscall` の追跡）を実装した。macOS では正規バイナリが
   `libSystem.dylib` 経由で syscall を発行するため、Go ランタイムが `svc #0x80` を直接
   発行するケースは極めて稀である。Pass 2 は本タスクのスコープ外とし、Pass 1（直接 `svc #0x80`
   スキャン）のみを実装する。

4. **x86_64 Mach-O バイナリ**: `containsSVCInstruction` は既に `CpuArm64` のみを対象としており、
   x86_64 スライスは現行通りシンボル解析のみで判定される。本タスクは arm64 のみに対応する。

5. **`mprotect(PROT_EXEC)` の引数評価**: タスク 0099 のスコープ。本タスクでは `mprotect`(74) が
   検出された場合でも `PROT_EXEC` フラグの評価は行わない。

## 8. 先行タスクとの関係

| 先行タスク | 関連する本タスクの機能 | 備考 |
|----------|----------------------|------|
| 0072 (ELF arm64 syscall) | FR-3.1（ARM64DarwinDecoder） | arm64 デコーダロジックを再利用・拡張。差分は syscall 命令即値とレジスタ番号のみ |
| 0073 (Mach-O ネットワーク検出) | 既存の `svc #0x80` 検出基盤 | 本タスクで `svc #0x80` 検出を拡張し、番号解析・キャッシュを追加 |
| 0076 (ネットワークシンボルキャッシュ) | FR-3.4（runner キャッシュ利用） | `SymbolAnalysis` キャッシュは既存実装。本タスクは `SyscallAnalysis` のキャッシュを追加 |
| 0077 (CGO バイナリフォールバック) | FR-3.3（FR-4.5 Mach-O フォールバック） | ELF 版フォールバックの Mach-O 対応 |
| 0095 (Mach-O 機能パリティ) | FR-4.2 / FR-4.4 / FR-4.5 | 本タスクが担うサブタスク |
| 0096 (LC_LOAD_DYLIB 整合性) | 前提タスク | スキーマ v14・machodylib・パス解決ロジックを前提 |
| 0099 (mprotect PROT_EXEC 検出) | FR-4.6 | タスク 0097 の ARM64DarwinDecoder を再利用する後続タスク |
