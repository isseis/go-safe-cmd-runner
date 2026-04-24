# 実装計画書: Mach-O arm64 syscall 番号解析

## 1. 実装方針

詳細仕様書（`03_detailed_specification.md`）に従い、以下の順序で実装する。

依存関係の方向に沿って下位レイヤから実装し、各ステップで対応するユニットテストを合わせて作成する。

```
libccache（BackwardScanX16 公開）
  ↓
fileanalysis/schema.go（v16）
  ↓
machoanalyzer/pclntab_macho.go（新規）
  ↓
machoanalyzer/pass1_scanner.go（新規）
  ↓
machoanalyzer/pass2_scanner.go（新規）
  ↓
machoanalyzer/syscall_number_analyzer.go（新規）
  ↓
filevalidator/validator.go（analyzeMachoSyscalls 拡張）
  ↓
runner/security/network_analyzer.go（判定ロジック変更）
  ↓
統合テスト
```

## 2. 実装ステップ

### ステップ 1: `internal/libccache/macho_analyzer.go` — BackwardScanX16 公開

**変更内容:**
- `backwardScanX16` → `BackwardScanX16` に改名（シグネチャ・実装は変更なし）
- `isControlFlowInstruction` → `IsControlFlowInstruction` に改名（pass2 から参照するため公開）
- 同パッケージ内の呼び出し箇所（`analyzeWrapperFunction`）を新名称に更新

**テスト:**
- [ ] 既存の `libccache` テストがすべてパスすること
- [ ] `BackwardScanX16` が公開されていること（コンパイル確認）

**対応 AC:** AC-6（既存テストへの非影響）

---

### ステップ 2: `internal/fileanalysis/schema.go` — スキーマバージョン v16

**変更内容:**
```go
const CurrentSchemaVersion = 16
```

**テスト（既存テストの更新）:**
- [ ] `CurrentSchemaVersion == 16` のアサーションが通ること
- [ ] v15 レコード Load で `SchemaVersionMismatchError` が返ること

**対応 AC:** AC-5

---

### ステップ 3: `internal/runner/security/machoanalyzer/pclntab_macho.go`（新規）

**実装内容:**

1. エラー定数
   ```go
   var (
       ErrNoPclntab                 = errors.New("no __gopclntab section found")
       ErrUnsupportedPclntabVersion = errors.New("unsupported pclntab version")
       ErrInvalidPclntab            = errors.New("invalid pclntab data")
   )
   ```

2. 型定義
   ```go
   type MachoPclntabFunc struct { Name string; Entry, End uint64 }
   type funcRange struct { start, end uint64 }
   func isInsideRange(addr uint64, ranges []funcRange) bool  // バイナリサーチ
   ```

3. `ParseMachoPclntab(f *macho.File) (map[string]MachoPclntabFunc, error)`
   - `__gopclntab` セクション取得 → `ErrNoPclntab`
   - pclntab マジック確認（`0xfffffff1` = Go 1.20+）→ `ErrUnsupportedPclntabVersion`
   - `gosym.NewLineTable` + `gosym.NewTable` で関数テーブル構築
   - `detectMachoPclntabOffset` で CGO オフセット補正

4. `detectMachoPclntabOffset(f *macho.File, funcs map[string]MachoPclntabFunc) uint64`
   - ELF 版 `detectOffsetByCallTargets`（`elfanalyzer/pclntab_parser.go`）と同一アルゴリズム
   - `__TEXT,__text` の BL 命令とのクロスリファレンスで補正量を検出

**テスト（`pclntab_macho_test.go`）:**
- [ ] `__gopclntab` セクションなし → `ErrNoPclntab`（AC-2, AC-3）
- [ ] 不正マジック → `ErrUnsupportedPclntabVersion`
- [ ] 正常な pclntab バイト列 → `syscall.Syscall` 等のエントリが含まれること（AC-2, AC-3）
- [ ] `isInsideRange` のユニットテスト（ソート済みリスト、境界値）
- [ ] `Analyze` が `ErrNoPclntab` をエラーとして伝播させないこと（AC-3 前提）

**参照:** `elfanalyzer/pclntab_parser.go`

---

### ステップ 4: `internal/runner/security/machoanalyzer/pass1_scanner.go`（新規）

**実装内容:**

1. `knownMachoSyscallImpls` マップ
   ```go
   var knownMachoSyscallImpls = map[string]struct{}{
       "syscall.Syscall": {}, "syscall.Syscall6": {},
       "syscall.RawSyscall": {}, "syscall.RawSyscall6": {},
       "internal/runtime/syscall.Syscall6": {},
   }
   ```

2. `buildStubRanges(funcs map[string]MachoPclntabFunc) []funcRange`
   - `knownMachoSyscallImpls` に含まれる関数の `funcRange` を収集してソート

3. `scanSVCWithX16(svcAddrs []uint64, code []byte, textBase uint64, stubRanges []funcRange, table libccache.MacOSSyscallTable) []common.SyscallInfo`

**テスト（`pass1_scanner_test.go`）:**
- [ ] `MOVZ X16, #98` + `svc #0x80` → `Number=98, IsNetwork=true, Method="immediate"`（AC-1）
- [ ] `MOVZ X16, #3` + `svc #0x80` → `Number=3, IsNetwork=false, Method="immediate"`（AC-1）
- [ ] BSD prefix 付き 32bit → `Number=98`（AC-1）
- [ ] `ldr x16, [sp, #N]` + `svc #0x80` → `Number=-1, Method="unknown:indirect_setting"`（AC-1）
- [ ] 制御フロー命令（BL）を挟んだ `svc` → スキャン停止 → `Number=-1`（AC-1）
- [ ] `svc` がスタブ範囲内 → 結果スライスに含まれない（AC-2）
- [ ] `svc` がスタブ範囲外 → 結果スライスに含まれる（AC-2）

---

### ステップ 5: `internal/runner/security/machoanalyzer/pass2_scanner.go`（新規）

**実装内容:**

1. 型定義
   ```go
   type MachoWrapperCall struct {
       CallSiteAddress uint64; TargetFunction string
       SyscallNumber int; DeterminationMethod string
   }
   ```

2. `knownMachoGoWrappers` マップ
   ```go
   var knownMachoGoWrappers = map[string]struct{}{
       "syscall.Syscall": {}, "syscall.Syscall6": {},
       "syscall.RawSyscall": {}, "syscall.RawSyscall6": {},
       "runtime.syscall": {}, "runtime.syscall6": {},
   }
   ```

3. `MachoWrapperResolver` 構造体と以下のメソッド
   - `NewMachoWrapperResolver(funcs map[string]MachoPclntabFunc) *MachoWrapperResolver`
   - `HasWrappers() bool`
   - `FindWrapperCalls(code []byte, textBase uint64, table libccache.MacOSSyscallTable) []MachoWrapperCall`

4. ヘルパー関数
   - `decodeBLTarget(word uint32, instrAddr uint64) (uint64, bool)`
   - `resolveStackABIArg(code []byte, blOffset int) (int, string)` — `[SP, #8]` 後方スキャン（Phase A → Phase B）

**テスト（`pass2_scanner_test.go`）:**
- [ ] `MOVZ xN, #98` + `STP xN, ..., [SP, #8]` + `BL syscall.Syscall` → `Number=98, IsNetwork=true, Method="go_wrapper"`（AC-3）
- [ ] `MOVZ xN, #3` + `STR xN, [SP, #8]` + `BL syscall.Syscall6` → `Number=3, IsNetwork=false, Method="go_wrapper"`（AC-3）
- [ ] `MOVZ xN, #49` + `STP xN, ..., [SP, #8]` + `BL syscall.RawSyscall` → `Number=49, IsNetwork=false, Method="go_wrapper"`（AC-3）
- [ ] 間接ロード + `BL syscall.RawSyscall6` → `Number=-1, Method="unknown:indirect_setting"`（AC-3）
- [ ] `.gopclntab` なし（`wrapperAddrs` 空）→ Pass 2 結果なし（AC-3）
- [ ] ラッパー内からの BL（`IsInsideWrapper`）→ 結果に含まれない
- [ ] 制御フロー境界 → スキャン停止 → `Number=-1`（AC-3）
- [ ] `decodeBLTarget` ユニットテスト（有効 BL / 非 BL 命令）

---

### ステップ 6: `internal/runner/security/machoanalyzer/syscall_number_analyzer.go`（新規）

**実装内容:**

1. 定数
   ```go
   const (
       determinationMethodImmediate       = "immediate"
       determinationMethodGoWrapper       = "go_wrapper"
       determinationMethodUnknownIndirect = "unknown:indirect_setting"
   )
   ```

2. `MachoSyscallAnalyzerInterface` インターフェース（定義は `filevalidator` パッケージ — 仕様書 §8.2）
   ```go
   // filevalidator/validator.go または filevalidator/macho_analyzer_interface.go に定義
   type MachoSyscallAnalyzerInterface interface {
       AnalyzeFile(filePath string, fs safefileio.FileSystem) (*fileanalysis.SyscallAnalysisResult, error)
   }
   ```
   備考: インターフェースを利用側（`filevalidator`）に定義する Go 慣習に従う。`syscall_number_analyzer.go` には定義しない。

3. `MachoSyscallNumberAnalyzer` 構造体と以下のメソッド
   - `NewMachoSyscallNumberAnalyzer() *MachoSyscallNumberAnalyzer`
   - `Analyze(f *macho.File) (*fileanalysis.SyscallAnalysisResult, error)` — Pass 1 + Pass 2 実行
   - `AnalyzeFile(filePath string, fs safefileio.FileSystem) (*fileanalysis.SyscallAnalysisResult, error)` — Fat バイナリ対応ラッパー

4. `wrapperCallsToSyscallInfos(calls []MachoWrapperCall, table libccache.MacOSSyscallTable) []common.SyscallInfo`
   - `MachoWrapperCall.SyscallNumber` から `table.GetSyscallName` / `table.IsNetworkSyscall` で `Name`・`IsNetwork` を設定
   - `MachoWrapperCall` 自体は `IsNetwork` を持たないため、この関数内でテーブル参照が必要

**テスト（`syscall_number_analyzer_test.go`）:**
- [ ] arm64 以外の Mach-O → `nil, nil`
- [ ] `__text` セクションなし → `nil, nil`
- [ ] Pass 1 + Pass 2 の統合（モック pclntab）
- [ ] `Analyze` が `nil, nil` を返す際 `AnalyzeFile` が `nil, nil` を透過すること
- [ ] Pass 1 の `Source` フィールドが `""` であること（AC-5）
- [ ] Pass 2 の `Source` フィールドが `""` であること（AC-5）

---

### ステップ 7: `internal/filevalidator/validator.go` — `analyzeMachoSyscalls` 拡張

**変更内容:**

1. `Validator` 構造体に `machoSyscallAnalyzer MachoSyscallAnalyzerInterface` フィールドを追加
2. コンストラクタ（`NewValidator`）で `MachoSyscallNumberAnalyzer` を注入
3. `analyzeMachoSyscalls` を以下に差し替え:
   - `v.machoSyscallAnalyzer.AnalyzeFile(filePath, v.fileSystem)` で Pass 1/Pass 2 結果取得
   - `v.analyzeLibSystem(record, filePath)` で libSystem 解析（変更なし）
   - `mergeMachoSyscallResults(result, libsysEntries, libsysArch)` でマージして保存
4. `buildSVCInfos` 関数と `DeterminationMethodDirectSVC0x80` を使う箇所を削除

**テスト（`validator_test.go` または新規テストファイル）:**
- [ ] `MachoSyscallAnalyzerInterface` のモック注入テスト
- [ ] Pass 1/Pass 2 結果と libSystem 結果のマージ検証
- [ ] `machoSyscallAnalyzer` がエラーを返す場合に `analyzeMachoSyscalls` がエラーを伝播すること
- [ ] `buildSVCInfos` が削除されていること（コンパイル確認）
- [ ] libSystem import 解析（`analyzeLibSystem`）エントリの `Source` が `"libc_symbol_import"` のまま変更されていないこと（AC-5, AC-6）

**対応 AC:** AC-5（`Source` フィールド確認）、AC-6（既存機能への非影響）

---

### ステップ 8: `internal/runner/security/network_analyzer.go` — 判定ロジック変更

注記: このステップの初版には `syscallAnalysisHasSVCSignal` 削除案が含まれていたが、後続タスク 0105 で superseded となった。実装・レビュー時は「削除」ではなく「未解決 svc (`Number == -1`) のみを高リスク判定する条件へ修正」を正とする。

**変更内容:**

1. `syscallAnalysisHasSVCSignal` 関数を削除せず、未解決 svc (`Number == -1`) のみを高リスク判定する条件へ修正する
   - superseded: 0105 以降は削除しない。`Number == -1` 条件を追加して保持する
2. `isNetworkViaBinaryAnalysis` 内の `SyscallAnalysis` 参照ブロックを以下に変更:
   ```go
   // 変更前: syscallAnalysisHasSVCSignal → true, true
   // 変更後: 削除

   // 変更前: syscallAnalysisHasNetworkSignal → true, false
   // 変更後: syscallAnalysisHasNetworkSignal → true, true（isHighRisk=true に変更）

   // 変更前: SyscallAnalysis==nil → return false, false（早期リターン）
   // 変更後: SyscallAnalysis==nil → SymbolAnalysis 判定へ委譲（フォールスルー）
   ```
3. `syscallAnalysisHasNetworkSignal` の実装を `IsNetwork==true` のみ判定するように確認（`DeterminationMethod` 非参照）
4. ログメッセージ更新

**テスト（`network_analyzer_test.go` 既存テスト更新）:**
- [ ] `IsNetwork=true` エントリあり → `true, true`（AC-4）
- [ ] `IsNetwork=false` のみ → `SymbolAnalysis` 判定に委譲（AC-4）
- [ ] `SyscallAnalysis==nil`（`nil, nil`）→ `SymbolAnalysis` 判定に委譲（AC-4）
- [ ] v15 レコード（`SchemaVersionMismatchError`）→ `true, true`（AC-4）
- [ ] 未解決 svc (`Number == -1` かつ `"direct_svc_0x80"`) のみを高リスク判定すること（0105 で更新）

---

### ステップ 9: 統合テスト

**テスト（新規ファイル or 既存統合テストに追記）:**
- [ ] `record` コマンド自身（`build/prod/record`）→ `runner` が `true, true` を返さないこと（偽陽性なし）（AC-4）
- [ ] `record` バイナリの `SyscallAnalysis` に `IsNetwork=true` エントリが含まれないこと
- [ ] ELF バイナリの既存テストがすべてパスすること（AC-6）
- [ ] ELF パス（`SourceLibcSymbolImport`）の `Source` フィールドが変更されていないこと（AC-5, AC-6）

---

## 3. 進捗チェックリスト

### ステップ 1: libccache 公開化
- [x] `BackwardScanX16` 実装
- [x] `IsControlFlowInstruction` 実装（必要に応じて公開）
- [x] 既存テスト通過確認

### ステップ 2: スキーマバージョン v16
- [x] `CurrentSchemaVersion = 16` 変更
- [x] スキーマテスト更新

### ステップ 3: pclntab_macho.go
- [x] エラー定数定義
- [x] `MachoPclntabFunc`、`funcRange`、`isInsideRange` 実装
- [x] `ParseMachoPclntab` 実装
- [x] `detectMachoPclntabOffset` 実装
- [x] `pclntab_macho_test.go` 作成

### ステップ 4: pass1_scanner.go
- [x] `knownMachoSyscallImpls` 定義
- [x] `buildStubRanges` 実装
- [x] `scanSVCWithX16` 実装
- [x] `pass1_scanner_test.go` 作成

### ステップ 5: pass2_scanner.go
- [ ] `MachoWrapperCall` 型定義
- [ ] `knownMachoGoWrappers` 定義
- [ ] `MachoWrapperResolver` 実装（`NewMachoWrapperResolver`、`HasWrappers`、`FindWrapperCalls`）
- [ ] `decodeBLTarget` 実装
- [ ] `resolveStackABIArg` 実装（Phase A: `[SP, #8]` ストア検出 → Phase B: 即値解析）
- [ ] `pass2_scanner_test.go` 作成

### ステップ 6: syscall_number_analyzer.go
- [ ] `MachoSyscallAnalyzerInterface` 定義
- [ ] `MachoSyscallNumberAnalyzer.Analyze` 実装
- [ ] `MachoSyscallNumberAnalyzer.AnalyzeFile` 実装（Fat バイナリ対応）
- [ ] `wrapperCallsToSyscallInfos` 実装
- [ ] `syscall_number_analyzer_test.go` 作成

### ステップ 7: validator.go 拡張
- [ ] `machoSyscallAnalyzer` フィールド追加
- [ ] コンストラクタ更新
- [ ] `analyzeMachoSyscalls` 差し替え
- [ ] `mergeMachoSyscallResults` 実装
- [ ] `buildSVCInfos` 削除
- [ ] バリデータテスト更新

### ステップ 8: network_analyzer.go 変更
- [ ] `syscallAnalysisHasSVCSignal` 削除
- [ ] `isNetworkViaBinaryAnalysis` 判定ロジック変更
- [ ] `nil` SyscallAnalysis の委譲動作確認
- [ ] ログメッセージ更新
- [ ] `network_analyzer_test.go` 更新

### ステップ 9: 統合テスト
- [ ] `build/prod/record` を使った偽陽性テスト
- [ ] ELF バイナリ既存テスト通過確認
- [ ] `make test` + `make lint` 全通過

## 4. 受け入れ条件対応表

| AC | 対応ステップ | テスト場所 |
|---|---|---|
| AC-1: Pass 1 直接 syscall 番号解析 | ステップ 4 | `pass1_scanner_test.go` |
| AC-2: Go runtime スタブの除外 | ステップ 3, 4 | `pass1_scanner_test.go`, `pclntab_macho_test.go` |
| AC-3: Pass 2 呼び出しサイト解析 | ステップ 3, 5, 6 | `pass2_scanner_test.go`, `pclntab_macho_test.go` |
| AC-4: リスク判定変更 | ステップ 8, 9 | `network_analyzer_test.go`, 統合テスト |
| AC-5: スキーマ | ステップ 2, 6, 7 | `schema_test.go`, `syscall_number_analyzer_test.go` |
| AC-6: 既存機能への非影響 | 全ステップ | 既存テスト一式, 統合テスト |

## 5. 注意事項・実装上のポイント

### `isControlFlowInstruction` の公開について
`libccache/macho_analyzer.go` の `isControlFlowInstruction` は pass2 の後方スキャンでも必要になる。`IsControlFlowInstruction` として公開するか、`machoanalyzer` パッケージに複製するかを選択すること。**DRY 原則を優先して公開する**（詳細仕様書 §6.5 参照）。

### `collectSVCAddresses` の参照
`svc_scanner.go` の `collectSVCAddresses` はパッケージ内プライベート。`syscall_number_analyzer.go` は同パッケージ（`machoanalyzer`）内に配置されるため直接参照可能。

### `MachoSyscallAnalyzerInterface` の配置
詳細仕様書（§8.2）ではインターフェースを `filevalidator` パッケージに定義している。ただし、インターフェースを利用側パッケージに定義する Go 慣習と一致するため、このまま採用する。

### スキーマバージョン v16 変更の影響
`CurrentSchemaVersion` 変更はすべてのスキーマ依存テストに影響する。ステップ 2 実施後に `make test` を実行して影響範囲を確認し、テストの期待値を更新すること。

### `build/prod/record` の存在前提
統合テスト（ステップ 9）は `build/prod/record` が存在することを前提とする。CI 環境では事前に `make build` が必要。テストファイルに `//go:build integration` タグを付与してデフォルト実行から除外することを検討する。
