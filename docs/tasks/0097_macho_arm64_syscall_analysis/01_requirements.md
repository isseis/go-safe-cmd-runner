# Mach-O svc #0x80 キャッシュ統合・キャッシュ優先判定 要件定義書

## 1. 概要

### 1.1 背景

タスク 0073 では Mach-O バイナリのインポートシンボル解析と `svc #0x80` 命令の存在確認を
実装した。`svc #0x80` 検出時は `machoanalyzer` が `binaryanalyzer.AnalysisError` を返し、
`network_analyzer.go` の `handleAnalysisOutput` がこれを `(true, true)`（ネットワーク検出・高リスク確定）に変換する。

タスク 0076 でネットワークシンボル解析結果が `SymbolAnalysis` にキャッシュされたことで、
`runner` はシンボル判定に live 解析を必要としなくなった。しかし `svc #0x80` スキャンは
`SyscallAnalysis` に保存されておらず、以下の問題が残っている：

1. **svc #0x80 スキャン結果の未キャッシュ**: `runner` は Mach-O の `svc #0x80` 判定を
  `SymbolAnalysis` だけでは再利用できず、`SyscallAnalysis` に保存された結果を参照できない。
  そのため Mach-O の直接 syscall シグナルをキャッシュだけで完結して判定できず、
  `svc #0x80` スキャン結果の再利用性が不足している。

2. **SymbolAnalysis キャッシュヒット時の svc スキャン迂回（セキュリティ上の問題）**:
   `runner` の `isNetworkViaBinaryAnalysis` は `SymbolAnalysis` キャッシュが存在する場合に
   live 解析を行わずキャッシュ結果のみを返す。`SymbolAnalysis = NoNetworkSymbols` の
   Mach-O バイナリが `svc #0x80` を含む場合でも、キャッシュヒットによって svc スキャンが
   迂回され、バイナリが誤って実行許可される可能性がある。

本タスクは上記を解消し、タスク 0095 FR-4.4 および FR-4.5 を実装する。

なお、`svc #0x80` が存在する Mach-O バイナリは正規バイナリではないため（§1.2 参照）、
syscall 番号の解析によりネットワーク関連かを区別する必要はない。`svc #0x80` の存在自体を
高リスクとして扱う現行方針を維持する。

### 1.2 macOS arm64 における svc #0x80 の位置づけ

macOS 上の正規バイナリ（Go バイナリ・C バイナリを含む）は `libSystem.dylib` 経由で
syscall を発行するため、`svc #0x80` はバイナリ本体の `__TEXT,__text` セクションには
現れない。`svc #0x80` の存在は libSystem.dylib を迂回した直接 syscall を示し、
それ自体が異常なシグナルである。

このため本タスクでは syscall 番号（`x16` レジスタ）の解析は行わず、
`svc #0x80` の有無のみをシグナルとして扱う。

### 1.3 目的

- `record` 時に `svc #0x80` スキャン結果を `SyscallAnalysis` に保存し、`runner` のキャッシュ利用を可能にする
- `SymbolAnalysis` の結果にかかわらず全 Mach-O バイナリに `svc #0x80` スキャンを適用し、SymbolAnalysis キャッシュヒット時の検出迂回を防ぐ
- ELF 版タスク 0077 の Mach-O 対応として、`record` 時の svc スキャンを実装する

### 1.4 スコープ

- **対象**: macOS (Darwin) arm64 Mach-O バイナリ（単一アーキテクチャおよび Fat バイナリ）
- **対象外**: syscall 番号解析（`x16` レジスタ後方スキャン）
- **対象外**: `svc #0x80` による `mprotect(PROT_EXEC)` の引数評価（タスク 0099 で扱う）
- **対象外**: libSystem.dylib syscall ラッパーキャッシュ（タスク 0100 で扱う）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `svc #0x80` | arm64 macOS における直接 syscall 命令（エンコード `0xD4001001`）。正規バイナリは `libSystem.dylib` 経由で呼ぶため通常は現れない |
| `SyscallAnalysis` | `fileanalysis.Record` フィールド。syscall 解析結果をキャッシュする |
| svc スキャン | `__TEXT,__text` セクションを走査し `svc #0x80` 命令の有無を確認する処理。タスク 0073 の `containsSVCInstruction` が実装済み |
| `SymbolAnalysis` | `fileanalysis.Record` フィールド。インポートシンボル解析結果をキャッシュする |

## 3. 機能要件

### FR-3.1: `record` 時の svc スキャン結果の保存（FR-4.4 対応）

#### FR-3.1.1: SyscallAnalysis への記録

`record` コマンドが Mach-O バイナリの svc スキャンを実行した際、その結果を
`fileanalysis.Record.SyscallAnalysis` に保存すること。

**保存内容**: `svc #0x80` が 1 件以上検出された場合：
- `SyscallAnalysis.Architecture = "arm64"`
- `SyscallAnalysis.AnalysisWarnings` に「`svc #0x80` を検出: libSystem.dylib を迂回した
  直接 syscall が存在する」旨のメッセージを追加する
- `SyscallAnalysis.DetectedSyscalls` に各 `svc #0x80` の情報を記録する。
  syscall 番号は解析しないため `Number = -1` とし、検出したアドレスは `Location` に保存する。
  また、検出理由は既存スキーマに合わせて `Source = "direct_svc_0x80"` で表現する

**保存しない場合**: `svc #0x80` が 1 件も検出されなかった場合は `SyscallAnalysis` を `nil` のままにする。

**既存フィールドの変更禁止**: `ContentHash`・`SymbolAnalysis`・`DynLibDeps` フィールドは変更しない。

#### FR-3.1.2: SymbolAnalysis との保存順序

SyscallAnalysis の保存は Mach-O バイナリであれば `SymbolAnalysis` の結果にかかわらず実行すること。
`record` の責務はバイナリの状態を忠実に記録することであり、`SyscallAnalysis` を参照するかどうかの判断は `runner` が行う。
`runner` は `SymbolAnalysis = NetworkDetected` の場合は `SyscallAnalysis` を参照しない（FR-3.3.2 参照）。

#### FR-3.1.3: スキーマバージョン

本タスクで**スキーマバージョンを v14 から v15 に変更する**。
v15 レコードはスキャンが実施済みであることを論理的に保証するため、`SyscallAnalysis` フィールドが
`nil` であっても「スキャン実施済み・未検出」を意味し、`runner` は `false, false` を返す。
v14 以前のレコードとの後方互換性は持たず、`SchemaVersionMismatchError` を返してユーザーに再 `record` を強制する。

#### FR-3.1.4: `--force` フラグとの整合性

`record --force` 実行時は `SyscallAnalysis` も新しい値で上書きする。

### FR-3.2: Mach-O バイナリへの svc スキャン適用と runner における参照制御（FR-4.5 対応）

#### FR-3.2.1: svc スキャンの適用条件

以下を満たす場合に `record` 時に svc スキャンを実行し結果を保存すること：

1. Mach-O バイナリである

`SymbolAnalysis` の結果（`NetworkDetected` / `NoNetworkSymbols` 等）にかかわらず svc スキャンを実行すること。
`record` の責務はバイナリの状態を忠実に記録することであり、svc スキャン結果の利用方法は `runner` が決定する。

#### FR-3.2.2: runner における SyscallAnalysis の参照条件

`runner` は `SymbolAnalysis = NoNetworkSymbols` の場合にのみ `SyscallAnalysis` キャッシュを参照すること。
`SymbolAnalysis = NetworkDetected` の場合、`runner` は既存の判定（実行ブロック）を優先し、
`SyscallAnalysis` は参照しない。
`record` 側でのスキャン有無ではなく、`runner` 側の参照ロジックで制御することで責任境界を明確にする。
AC-1 はこの動作を受け入れ条件として規定する。

### FR-3.3: `runner` 実行時の SyscallAnalysis キャッシュ利用（FR-4.4 / FR-4.5 対応）

#### FR-3.3.1: SymbolAnalysis = NoNetworkSymbols 時の SyscallAnalysis 参照

`runner` の `isNetworkViaBinaryAnalysis` において、`SymbolAnalysis` キャッシュが
`NoNetworkSymbols` を返した場合、`SyscallAnalysis` キャッシュを追加参照すること。

これにより SymbolAnalysis キャッシュヒット時に svc スキャンが迂回される問題（§1.1）を解消する。

ELF 版タスク 0077 FR-3.3.1 の Mach-O 対応として、同一の参照フローを適用する。

#### FR-3.3.2: SyscallAnalysis からの判定ロジック

`SymbolAnalysis = NetworkDetected` の場合、`runner` は `SyscallAnalysis` を参照せず既存の判定を採用する。
`SymbolAnalysis = NoNetworkSymbols` の場合のみ `SyscallAnalysis` を参照する（FR-3.2.2 参照）。

| SyscallAnalysis の状態 | 判定 | 動作 |
|----------------------|------|------|
| `SyscallAnalysis` が nil（スキャン実施済み・未検出） | `false, false` を返す | v15 スキーマ保証により live 解析フォールバック不要 |
| `svc #0x80` 検出記録あり（`DetectedSyscalls` に `direct_svc_0x80` あり） | `true, true`（ネットワーク検出・高リスク確定） | 実行をブロック |

**高リスク判定の具体的条件**: `DetectedSyscalls` に `DeterminationMethod = "direct_svc_0x80"` の
エントリが存在する場合に `true, true`（`isNetwork=true, isHighRisk=true`）を返す。
これは解析成功・ネットワーク確定のケースであり、解析失敗によるフェイルセーフとは意味合いが異なる。
`AnalysisWarnings` は ELF 側の syscall 解析による警告を含み得るため判定条件に使用しない。

#### FR-3.3.3: キャッシュ読み込みエラー処理

`isNetworkViaBinaryAnalysis` の cache-backed path ではすべてのエラーケースを直接 return とし、
`svc #0x80` 判定のための live 解析フォールバックを持たない。

本要件は `store` と `contentHash` が提供される通常実行パスを対象とする。
`NewNetworkAnalyzer()` や `NewNetworkAnalyzerWithStore(nil)` の legacy live 解析経路は
本タスク完了後に削除する（実装計画書 Section 8 参照）。

| エラー種別 | 処理 |
|-----------|------|
**SymbolAnalysis 読み込みエラー**

| エラー種別 | 処理 |
|-----------|------|
| `ErrRecordNotFound` / `ErrNoNetworkSymbolAnalysis` / `ErrHashMismatch` / `SchemaVersionMismatchError` / その他 | `AnalysisError` を返す（production では record 済みが前提） |

**SyscallAnalysis 読み込みエラー**

| エラー種別 | 処理 |
|-----------|------|
| `ErrNoSyscallAnalysis` | `false, false` を返す（v15 スキーマ保証：スキャン実施済み・未検出） |
| `ErrHashMismatch` | `AnalysisError` を返す（安全側フェイルセーフ） |
| `SchemaVersionMismatchError` | `AnalysisError` を返す（v14 → 再 `record` を要求） |
| `ErrRecordNotFound` / その他エラー | `AnalysisError` を返す（SymbolAnalysis ロード成功後は record が必ず存在するため整合性エラー） |

cache-backed path における `svc #0x80` 用の live 解析コード
（`a.binaryAnalyzer.AnalyzeNetworkSymbols()` による追加再判定）は削除する。
production の通常実行パスでは store と hash が提供される前提のため、
SVC 判定はキャッシュ結果だけで完結させ、フォールバックパスを持たない。

## 4. 非機能要件

### NFR-4.1: パフォーマンス

svc スキャン自体はタスク 0073 で実装済みであり、追加コストは `SyscallAnalysis` の保存と
読み込みのみ。キャッシュヒット時の追加オーバーヘッドは 10ms 未満を目標とする。

### NFR-4.2: セキュリティ

`svc #0x80` 検出時は番号解析の有無によらず常に `true, true`（ネットワーク検出・高リスク確定）を返す。
エラーによる解析失敗（`AnalysisError`）とは意味合いが異なるが、どちらも実行をブロックする。
SymbolAnalysis キャッシュヒット時も SyscallAnalysis を参照することで、
キャッシュ経由の検出迂回を防ぐ。

### NFR-4.3: 後方互換性

スキーマバージョンを v15 に変更し、v14 以前のレコードとの後方互換性は持たない。
`runner` が v14 レコードを参照した場合は `SchemaVersionMismatchError` を返し、ユーザーに再 `record` を強制する。
v15 スキーマ保証により `ErrNoSyscallAnalysis` は「スキャン実施済み・未検出」を意味するため、cache-backed path では live 解析フォールバックは不要となる。

### NFR-4.4: 既存実装の活用

svc スキャン（`containsSVCInstruction`）は変更不要または最小変更に留める。
各 `svc #0x80` のアドレスを収集するための拡張のみ行う。

## 5. 受け入れ条件

### AC-1: `record` 拡張

- [ ] `svc #0x80` を含む Mach-O バイナリに対して `SyscallAnalysis` が `record` 時に保存されること
- [ ] `SyscallAnalysis.Architecture` が `"arm64"` であること
- [ ] `SyscallAnalysis.AnalysisWarnings` に `svc #0x80` 検出を示すメッセージが含まれること
- [ ] `SyscallAnalysis.DetectedSyscalls` に各 `svc #0x80` の仮想アドレスが `Number = -1`、`DeterminationMethod = "direct_svc_0x80"` で記録されること
- [ ] `svc #0x80` が存在しない Mach-O に対して `SyscallAnalysis` が `nil` のままであること
- [ ] `SymbolAnalysis = NetworkDetected` の場合でも `svc #0x80` が検出されれば `SyscallAnalysis` が保存されること
- [ ] `record --force` で `SyscallAnalysis` が更新されること
- [ ] 既存の `ContentHash`・`SymbolAnalysis`・`DynLibDeps` フィールドが変更されないこと

### AC-2: Mach-O バイナリへの svc スキャン適用（FR-3.2）

- [ ] `SymbolAnalysis = NoNetworkSymbols` かつ `svc #0x80` を含む Mach-O に対して `record` 後に `SyscallAnalysis` が保存されること
- [ ] `SymbolAnalysis = NoNetworkSymbols` かつ `svc #0x80` を含まない Mach-O に対して `SyscallAnalysis` が `nil` であること

### AC-3: `runner` キャッシュ利用

- [ ] `SymbolAnalysis = NoNetworkSymbols` かつ `SyscallAnalysis` に `svc #0x80` 記録がある場合、`runner` が `true, true`（ネットワーク検出・高リスク確定）を返すこと
- [ ] `SymbolAnalysis = NoNetworkSymbols` かつ `SyscallAnalysis` が `nil`（`ErrNoSyscallAnalysis`）の場合、`false, false` を返すこと（live 解析フォールバックを行わないこと）
- [ ] `SyscallAnalysis` ロード時の `ErrHashMismatch` で `AnalysisError` が返されること
- [ ] `SyscallAnalysis` ロード時の `SchemaVersionMismatchError` で `AnalysisError` が返されること
- [ ] `SyscallAnalysis` ロード時の `ErrRecordNotFound` / その他エラーで `AnalysisError` が返されること
- [ ] `SymbolAnalysis` ロード時のキャッシュミスで `AnalysisError` が返されること
- [ ] `store` と `contentHash` が提供される cache-backed path では `isNetworkViaBinaryAnalysis` 内で live 解析（`AnalyzeNetworkSymbols()`）が呼ばれないこと

### AC-4: 既存機能への非影響

- [ ] `SymbolAnalysis = NetworkDetected` の Mach-O バイナリの判定が変わらないこと
- [ ] ELF バイナリの解析フローが変更されないこと
- [ ] 既存のテストがすべてパスすること

## 6. テスト方針

### 6.1 `record` 拡張のテスト

| テストケース | 検証内容 |
|-------------|---------|
| `svc #0x80` あり・シンボルなし | `SyscallAnalysis` が保存され、AnalysisWarnings に検出メッセージがあること |
| `svc #0x80` なし・シンボルなし | `SyscallAnalysis` が `nil` であること |
| `svc #0x80` あり・ネットワークシンボルあり | `SyscallAnalysis` が保存されること（`record` は SymbolAnalysis 結果にかかわらず記録する） |
| `svc #0x80` なし・ネットワークシンボルあり | `SyscallAnalysis` が `nil` であること |
| `svc #0x80` 複数 | `DetectedSyscalls` に複数エントリが記録されること |

### 6.2 `runner` キャッシュ利用のテスト

| テストケース | 検証内容 |
|-------------|---------|
| SymbolAnalysis=NoNetworkSymbols + SyscallAnalysis に svc #0x80 あり | `true, true`（isNetwork=true, isHighRisk=true）が返されること |
| SymbolAnalysis=NoNetworkSymbols + SyscallAnalysis が nil（ErrNoSyscallAnalysis） | `false, false` を返すこと（live 解析フォールバックを行わないこと） |
| SyscallAnalysis ロード時 ErrHashMismatch | `AnalysisError` が返されること |
| SyscallAnalysis ロード時 SchemaVersionMismatchError | `AnalysisError` が返されること |
| SyscallAnalysis ロード時 ErrRecordNotFound / その他エラー | `AnalysisError` が返されること（live 解析フォールバックなし） |

### 6.3 統合テスト

| テストケース | 検証内容 |
|-------------|---------|
| `svc #0x80` あり → `record` → `runner` | `runner` がキャッシュを利用して `true, true`（isNetwork=true, isHighRisk=true）を返すこと（live 解析なし） |
| ネットワークシンボルなし・`svc #0x80` なし → `record` → `runner` | 通過すること |

### 6.4 テストフィクスチャ

タスク 0073 の既存フィクスチャを活用する。
`svc #0x80` を含む arm64 Mach-O フィクスチャが不足する場合は追加生成する。

## 7. 先行タスクとの関係

| 先行タスク | 関連 | 備考 |
|----------|------|------|
| 0073 (Mach-O ネットワーク検出) | svc スキャン基盤 | `containsSVCInstruction` を拡張・再利用 |
| 0076 (ネットワークシンボルキャッシュ) | `SymbolAnalysis` キャッシュ基盤 | runner の SymbolAnalysis 参照フローを前提 |
| 0077 (CGO バイナリフォールバック) | `SyscallAnalysis` 参照パターン | ELF 版 FR-3.3.1 の Mach-O 対応 |
| 0095 (Mach-O 機能パリティ) | FR-4.4 / FR-4.5 | 本タスクが担う |
| 0096 (LC_LOAD_DYLIB 整合性) | スキーマ前提 | スキーマ v14 上に構築 |
| 0099 (mprotect PROT_EXEC 検出) | 後続タスク | 本タスクの svc スキャン拡張を再利用する可能性あり |
