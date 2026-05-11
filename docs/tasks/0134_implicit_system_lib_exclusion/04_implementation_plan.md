# 暗黙的システムライブラリの再帰的解析除外 実装計画書

## 0. 方針

要件定義書 AC-1〜AC-9 を満たすため、最小限のコード追加で「`Validator.analyzeLibraries`
における libselinux のスキップ」を実現する。アーキテクチャ設計書 §2 で確定した
設計判断（API 分離・スキーマ非変更・`DynLibDeps` 非干渉）を維持する。

- 主な変更:
  - `internal/security/binaryanalyzer/implicit_system_libs.go`（新規・公開 API 追加）
  - `internal/filevalidator/validator.go`（`analyzeLibraries` に分岐 1 行追加）
- 再利用優先:
  - 既存の `matchesKnownPrefix`（`syscall_wrapper_libs.go` の package-private 関数）を
    再利用してプレフィックス照合を共通化
  - 既存テストヘルパ（`libraryTestBinaryAnalyzer`, `requireWithSocketELF`）を流用
- 回帰防止:
  - 既存の `TestAnalyzeLibraries_excludesWrapperAndVDSO` には手を入れず、新規ケースは
    別テスト関数として追加

---

## 1. 進捗トラッキング

### Phase 1: 暗黙的システムライブラリ判定 API の実装

対象:

- `internal/security/binaryanalyzer/implicit_system_libs.go`（新規）
- `internal/security/binaryanalyzer/implicit_system_libs_test.go`（新規）

タスク:

- [ ] P1-1 `implicit_system_libs.go` を作成し、`implicitSystemLibPrefixes`
       スライスと `IsImplicitSystemLibrary(soname string) bool` を実装する
- [ ] P1-2 `implicitSystemLibPrefixes` 上部のドックコメントに選定基準（要件 FR-3）
       および除外しないライブラリの例を英語で記載する
- [ ] P1-3 各エントリ（初期は `libselinux` 1 件）に「なぜ除外するか」の inline
       コメントを英語で追加する
- [ ] P1-4 `implicit_system_libs_test.go` を作成し、以下の単体テストを実装する
       （既存 `syscall_wrapper_libs_test.go` のスタイルに準拠、`//go:build test` タグ）
  - match: `libselinux`, `libselinux.so.1`, `libselinux.so.2`
  - no match: `libssl.so.3`, `libcurl.so.4`, `libc.so.6`
  - prefix boundary: `libselinuxabc.so.1`, `libselinuxutil.so.1`

### Phase 2: Validator の除外判定統合

対象:

- `internal/filevalidator/validator.go`
- `internal/filevalidator/validator_library_analysis_test.go`

タスク:

- [ ] P2-1 `analyzeLibraries` のループ内、`IsSyscallWrapperLibrary` 分岐の直後に
       `if binaryanalyzer.IsImplicitSystemLibrary(soName) { continue }` を追加する
- [ ] P2-2 `validator_library_analysis_test.go` に
       `TestAnalyzeLibraries_excludesImplicitSystemLib` を追加し、
       `libselinux.so.1` を含む `DynLibDeps` で `bin.calls` がインクリメントされない
       ことを検証する（既存 `libraryTestBinaryAnalyzer` を再利用）
- [ ] P2-3 同テスト内で、`DynLibDeps` が呼び出し前後で変更されないこと
       （libselinux エントリが残ること）を assert する（AC-7）

### Phase 3: 回帰確認

タスク:

- [ ] P3-1 `TestAnalyzeLibraries_excludesWrapperAndVDSO` が手を加えずに通過する
       ことを確認する（既存の libc/VDSO/libssl 動作の維持）
- [ ] P3-2 ネットワーク API 検出経路の既存テスト（`network_analyzer_test.go`,
       `elfanalyzer`/`machoanalyzer` パッケージのテスト）が無変更で通過する
       ことを確認する（AC-3〜AC-6 の回帰確認）

### Phase 4: 品質ゲート

タスク:

- [ ] P4-1 `make fmt`
- [ ] P4-2 `go test -tags test -v ./internal/security/binaryanalyzer ./internal/filevalidator`
       （ピンポイント実行で早期フィードバック）
- [ ] P4-3 `make test`
- [ ] P4-4 `make lint`

### Phase 5: 実装計画書レビュー

- [ ] P5-1 AC カバレッジ確認: 要件定義書 AC-1〜AC-9 がすべて計画タスクに紐づくこと
- [ ] P5-2 テスト十分性確認: 非自明な分岐（プレフィックス境界、除外と非除外）に
       対するテストが揃うこと
- [ ] P5-3 テスト重複なし確認: 既存テストが既に保証する内容（libc/VDSO スキップ、
       libssl 解析継続）を再テストしていないこと
- [ ] P5-4 既存実装の再利用確認: `matchesKnownPrefix`、`libraryTestBinaryAnalyzer`、
       `requireWithSocketELF` を再利用していること
- [ ] P5-5 コード言語確認: 新規 Go ファイル（`.go`）のコメント・識別子・文字列
       リテラルに日本語が混入しないこと

---

## 2. AC トレーサビリティ

| AC | 内容（要約） | 対応タスク | 検証方法 |
|----|-------------|-----------|----------|
| AC-1 | libselinux 由来の `DetectedSymbols` が記録されない | P1-1, P2-1, P2-2 | `TestAnalyzeLibraries_excludesImplicitSystemLib` で `bin.calls==0` を検証（libselinux のみの DynLibDeps） |
| AC-2 | `SyscallAnalysis.occurrences[].source_path` に libselinux なし | P1-1, P2-1, P2-2 | 同テスト内で `record.SyscallAnalysis` が libselinux 由来のエントリを含まないことを確認（解析自体がスキップされるため波及的に検証される） |
| AC-3 | libc ネットワークシンボル検出の維持 | P3-2 | 既存 ELF アナライザテスト（無変更で通過） |
| AC-4 | 機械語 syscall 検出の維持 | P3-2 | 既存 syscall アナライザテスト（無変更で通過） |
| AC-5 | dlopen/dlsym 検出の維持 | P3-2 | 既存 `DynamicLoadSymbols` 検出テスト（無変更で通過） |
| AC-6 | mprotect+PROT_EXEC 検出の維持 | P3-2 | 既存 mprotect 検出テスト（無変更で通過） |
| AC-7 | libselinux が `DynLibDeps` に残存 | P2-3 | 同テスト内で `record.DynLibDeps` に libselinux エントリが残ることを assert |
| AC-8 | `libselinuxabc.so.1` は除外されない | P1-4 | `implicit_system_libs_test.go` の prefix-boundary ケース |
| AC-9 | `make test` / `make lint` 通過 | P4-3, P4-4 | コマンド成功を確認 |

---

## 3. 重複防止ポリシー

- 既存 `TestAnalyzeLibraries_excludesWrapperAndVDSO` は libc/VDSO の排他を保証して
  いるため、本タスクでは触らない
- libselinux 用テストは「`bin.calls` がインクリメントされないこと」のみを確認し、
  ネットワーク検出経路の動作確認（AC-3〜AC-6）は既存テストへの委譲とする
- `IsImplicitSystemLibrary` のテストは `IsSyscallWrapperLibrary` のテストと同パターン
  だが、対象リストが異なるため独立した検証となる

---

## 4. 既存実装の再利用ポイント

| 既存資産 | 再利用箇所 |
|---------|-----------|
| `matchesKnownPrefix`（`syscall_wrapper_libs.go`） | `IsImplicitSystemLibrary` のプレフィックス照合 |
| `libraryTestBinaryAnalyzer`（`validator_library_analysis_test.go`） | libselinux スキップ検証テストの mock |
| `requireWithSocketELF` | テスト用 ELF パス取得 |
| `validatorWithTempHashDir` | Validator インスタンス生成 |
| `analyzeLibraries` の early-continue パターン | libselinux 分岐の配置パターン |

---

## 5. レビュー結果（本計画書作成後）

### 5.1 AC カバレッジ

- AC-1〜AC-9 のすべてが Phase 1〜5 のいずれかのタスクに紐づくことを確認した
- AC-3〜AC-6 は既存テストへの委譲となるが、Phase 3（P3-2）で明示的に「無変更で通過する
  ことを確認」という検証タスクとして残している

### 5.2 テスト十分性

- 単体テスト: `IsImplicitSystemLibrary` の正例・反例・境界値（prefix boundary）を P1-4 で網羅
- 統合テスト: validator レベルで「libselinux を含む `DynLibDeps` に対して `bin.calls` が
  ゼロのまま」を検証
- false positive 解消の本質（AC-1/AC-2）は「解析が呼ばれない＝シンボル/syscall が
  集約されない」という因果関係に基づく検証で十分

### 5.3 テスト重複

- 既存テスト（libc/VDSO 排他、ネットワーク検出経路）はそのまま維持し、libselinux
  ケースのみを新規テストで補う
- `IsImplicitSystemLibrary` と `IsSyscallWrapperLibrary` は意味論的に独立した API のため、
  テスト分離は重複ではなく機能境界の明確化に該当する

### 5.4 既存実装の再利用

- §4 のとおり、ヘルパー関数・テストフィクスチャ・mock 型をすべて流用し、新規実装は
  リスト定義と関数 1 つに留まる
- アーキテクチャ §5.3 の API スケルトンも `IsSyscallWrapperLibrary` と同型

### 5.5 コード言語

- 新規 Go ファイルのコメント・識別子・文字列リテラルは英語のみとすることを P5-5 で
  チェックする
- ドキュメント（本計画書、要件、アーキテクチャ）は日本語で記述（CLAUDE.md 規則に従う）

---

## 6. 完了判定

以下をすべて満たした時点で完了とする。

- [ ] Phase 1〜5 の全タスクが完了済み
- [ ] AC トレーサビリティ表（§2）の全 AC が「対応タスク完了」かつ「検証方法実施済み」
- [ ] `make fmt` / `make test` / `make lint` がエラーなしで成功
