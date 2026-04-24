# 実装計画書: シンボル解析のライブラリフィルタ導入

## 1. 実装概要

### 1.1 目的

要件定義書と詳細仕様書に基づき、libc / libSystem 由来シンボルのみを記録するライブラリフィルタを導入し、`runner` のネットワーク判定をカテゴリベースへ移行する。

### 1.2 実装原則

1. **関心の分離**: `record` は libc / libSystem 由来シンボルを収集し、`runner` がネットワーク判定を行う
2. **最小変更**: 既存の `networkSymbols` マップと `binaryanalyzer` の型を最大限再利用する
3. **後方互換性**: 旧レコードの既存カテゴリを `IsNetworkCategory` で評価し続ける
4. **検証優先**: 各フェーズで対応するテストを追加し、最後に `make test` と `make lint` で回帰確認する

### 1.3 参照ドキュメント

- 設計判断は `02_architecture.md` の各コンポーネント設計とテスト戦略を参照する
- 実装詳細と AC トレーサビリティは `03_detailed_specification.md` の実装フェーズおよび受け入れ基準検証フェーズを参照する

## 2. 実装ステップ

### Phase 1: カテゴリ基盤の更新

**対象ファイル**:
- `internal/runner/security/binaryanalyzer/network_symbols.go`
- `internal/runner/security/binaryanalyzer/network_symbols_test.go`
- `internal/runner/security/binaryanalyzer/analyzer.go`

**作業内容**:
- [x] `CategorySyscallWrapper` を追加する
- [x] `IsNetworkCategory` を追加し、ネットワーク系カテゴリ集合を明文化する
- [x] `AnalysisOutput.DetectedSymbols` コメントを新セマンティクスに更新する
- [x] `network_symbols_test.go` に真偽境界のテストを追加する

**成功条件**:
- `syscall_wrapper` が新カテゴリとして文書化されている
- `IsNetworkCategory` が `socket` / `dns` / `tls` / `http` のみを `true` と判定する

**推定工数**: 0.5日

**実績**: 完了

### Phase 2: ELF 解析ロジックの更新

**対象ファイル**:
- `internal/runner/security/elfanalyzer/standard_analyzer.go`
- `internal/runner/security/elfanalyzer/analyzer_test.go`

**作業内容**:
- [x] `checkDynamicSymbols` の入力を `*elf.File` に変更する
- [x] VERNEED あり時は `sym.Library` ベースで libc 判定する
- [x] VERNEED なし時のみ DT_NEEDED フォールバックを適用する
- [x] libc 由来シンボルを全記録し、非対象ライブラリを除外する
- [x] AC-1 と AC-2 を満たすテストを追加または更新する

**成功条件**:
- `socket` と `read` がともに `DetectedSymbols` に記録される
- libc 以外のみの ELF バイナリでは `DetectedSymbols` が空になる
- `DetectedSymbols` のカテゴリにより `NetworkDetected` / `NoNetworkSymbols` が決まる

**推定工数**: 1日

**実績**: 完了

### Phase 3: Mach-O 解析ロジックの更新

**対象ファイル**:
- `internal/runner/security/machoanalyzer/standard_analyzer.go`
- `internal/runner/security/machoanalyzer/analyzer_test.go`

**作業内容**:
- [ ] library ordinal を使う libSystem 判定ヘルパーを追加する
- [ ] `NormalizeSymbolName` を通したカテゴリ付与を統合する
- [ ] Symtab なし時の `ImportedLibraries()` / `ImportedSymbols()` フォールバックを整理する
- [ ] libSystem 以外のシンボルが記録されないことを確認するテストを追加する
- [ ] AC-3 を満たすテストを追加または更新する

**成功条件**:
- libSystem 由来の `socket` と `read` が記録される
- 非 libSystem シンボルは `DetectedSymbols` に入らない
- Symtab の有無に応じて意図した経路に分岐する

**推定工数**: 1日

**実績**: 未着手

### Phase 4: `runner` 側判定の更新

**対象ファイル**:
- `internal/runner/security/network_analyzer.go`
- `internal/runner/security/network_analyzer_test.go`

**作業内容**:
- [ ] `len(data.DetectedSymbols) > 0` に依存した判定を除去する
- [ ] `IsNetworkCategory(sym.Category)` ベースの判定へ更新する
- [ ] 旧レコード互換ケースを含むテストを追加する
- [ ] `syscall_wrapper` のみでは `NoNetworkSymbols` となることを確認する

**成功条件**:
- ネットワーク系カテゴリを含む場合のみ `NetworkDetected` になる
- `syscall_wrapper` だけでは `NetworkDetected` にならない
- `KnownNetworkLibDeps` との OR 条件が維持される

**推定工数**: 0.5日

**実績**: 未着手

### Phase 5: 受け入れ基準検証と回帰確認

**対象ファイル**:
- 関連 `_test.go` 一式
- `docs/tasks/0106_symbol_library_filter/03_detailed_specification.md`

**作業内容**:
- [ ] AC-1 から AC-4 までのテスト実装を完了する
- [ ] `03_detailed_specification.md` の受け入れ基準検証フェーズを更新する
- [ ] `make test` を実行して回帰を確認する
- [ ] `make lint` を実行して lint を確認する

**成功条件**:
- AC-1 から AC-5 までの検証経路が実装と一致する
- リポジトリ全体のテストと lint が成功する

**推定工数**: 0.5日

**実績**: 未着手

## 3. 実装順序とマイルストーン

### Milestone 1: カテゴリ基盤の完成

- Deliverable: `CategorySyscallWrapper` と `IsNetworkCategory` が追加され、関連単体テストが存在する
- 対応フェーズ: Phase 1

### Milestone 2: バイナリ解析ロジックの完成

- Deliverable: ELF / Mach-O の両アナライザが libc / libSystem 由来シンボルのみを記録する
- 対応フェーズ: Phase 2, Phase 3

### Milestone 3: `runner` 判定切り替えの完成

- Deliverable: `network_analyzer.go` がカテゴリベース判定へ移行し、後方互換テストが通る
- 対応フェーズ: Phase 4

### Milestone 4: 受け入れ完了

- Deliverable: AC-1 から AC-5 の検証と `make test` / `make lint` の成功
- 対応フェーズ: Phase 5

**総推定工数**: 3.5日

## 4. テスト戦略

### 4.1 ユニットテスト

- `binaryanalyzer/network_symbols_test.go` で `IsNetworkCategory` の境界値を確認する
- `elfanalyzer/analyzer_test.go` で VERNEED 分岐、DT_NEEDED フォールバック、非 libc 除外を確認する
- `machoanalyzer/analyzer_test.go` で library ordinal 解決、Symtab なしフォールバック、非 libSystem 除外を確認する
- `network_analyzer_test.go` でカテゴリベース判定と旧レコード互換性を確認する

### 4.2 統合テスト

- ELF と Mach-O の fixture を用意し、`DetectedSymbols` のカテゴリが期待どおりに出力されることを確認する
- `KnownNetworkLibDeps` とシンボル判定の両方が併存するケースを確認する

### 4.3 後方互換性テスト

- 旧レコード相当の `DetectedSymbols` データを使い、既存カテゴリだけで `NetworkDetected` が成立することを確認する
- `syscall_wrapper` のみを含む新レコード相当データで `NoNetworkSymbols` になることを確認する

## 5. リスク管理

| リスク | 影響 | 緩和策 |
|--------|------|--------|
| ELF の VERNEED 判定と DT_NEEDED フォールバックを混在させる | 非 libc シンボルを誤記録する | VERNEED の有無を先に一意に判定し、混在を禁止する |
| Mach-O の library ordinal 解決ミス | libSystem 以外のシンボルを誤記録する | ordinal 範囲外と特殊値を明示的に除外する |
| `runner` 側の判定条件を更新し忘れる | `syscall_wrapper` のみで誤検知する | `network_analyzer_test.go` に否定ケースを追加する |
| `DetectedSymbols` の件数増加で既存期待値が壊れる | 既存テストが広く失敗する | AC 対応テストを先に用意し、既存期待値の見直しを限定的に行う |
| Mach-O / ELF の fixture 調整に想定以上の時間がかかる | 実装後半の検証が遅延する | Phase 2 完了時点でテスト資材の不足を棚卸しし、0.5日分のバッファを Phase 5 に確保する |

## 6. 実装チェックリスト

### Phase 1

- [x] カテゴリ定数を追加した
- [x] `IsNetworkCategory` を追加した
- [x] カテゴリ判定テストを追加した

### Phase 2

- [x] ELF の libc 判定を実装した
- [x] VERNEED なし時のフォールバックを実装した
- [x] AC-1 / AC-2 のテストを追加した

### Phase 3

- [x] Mach-O の libSystem 判定を実装した
- [x] Mach-O フォールバックを実装した
- [x] AC-3 のテストを追加した

### Phase 4

- [ ] `runner` のカテゴリベース判定を実装した
- [ ] AC-4 のテストを追加した
- [ ] 旧レコード互換ケースを追加した

### Phase 5

- [ ] AC-5 の検証を完了した
- [ ] `make test` を通した
- [ ] `make lint` を通した

## 7. 成功基準

- AC-1 から AC-5 に対応するテストがすべて存在する
- `DetectedSymbols` のセマンティクス変更が要件、設計、詳細仕様、実装計画で一貫している
- `runner` がカテゴリベースでのみシンボル由来の network 判定を行う
- 既存レコード互換性が維持される
- 実装完了後に `make test` と `make lint` が成功する
- 実装完了時に `03_detailed_specification.md` と本計画書の進捗が実装内容と一致している

## 8. 次のステップ

1. Phase 1 のカテゴリ基盤更新から着手する
2. ELF と Mach-O の解析更新を個別に実装し、各フェーズでテストを先に追加する
3. `runner` 側を切り替えた後、AC 検証と回帰確認を実施する
