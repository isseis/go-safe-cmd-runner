# アーキテクチャ設計書: エラー隠蔽による fail-open パターンの横断修正（残件）

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-19 |
| Review date | 2026-07-20 |
| Reviewer | isseis |
| Comments | - |

## 関連ドキュメント

- 要件定義書: [01_requirements.md](./01_requirements.md)
- セキュリティアーキテクチャ: [security-architecture.md](../../dev/architecture_design/security-architecture.md)

---

## 1. 設計の全体像

### 1.1 設計原則

本設計の核心は、以下の1つの設計原則に集約される:

> **「解析不能・エラー」と「対象なし・問題なし」を型／制御フロー上で区別し、前者を一律 fail-closed として処理する。**

現状コードには、エラーをログ出力のみで無視し「依存なし」「ネットワーク機能なし」「検証成功」と偽った判定をする fail-open パターンが、6つの異なるパッケージに横断的に存在する。本設計では、これら6箇所すべてに対して一貫した修正を適用する。

具体的には:
- エラーハンドリングの `default` 節 / `slog.Debug` 無視 を `slog.Warn` へ格上げし、エラーを呼び出し元へ伝播させる。
- `(false, nil)` や `(nil, nil)` で「問題なし」と偽る箇所を、`(false, err)` またはエラー伝播に変更する。
- `switch` 文に `default` 節を追加し、未知の値が追加された場合に安全側のデフォルト動作に落ち込むことを保証する。

### 1.2 概念モデル

```mermaid
flowchart LR
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph Before["Before（現状）"]
        direction TB
        S1["解析・検証処理"] -->|"エラー発生"| L1["slog.Debug"]
        L1 --> R1["「対象なし」「問題なし」と偽った判定<br>（fail-open）"]
        S1 -->|"成功"| N1["正常結果"]
    end

    subgraph After["After（修正後）"]
        direction TB
        S2["解析・検証処理"] -->|"エラー発生"| L2["slog.Warn"]
        L2 --> R2["エラーとして伝播<br>（fail-closed）"]
        S2 -->|"成功"| N2["正常結果"]
    end

    class S1,S2 process
    class L1,R1 problem
    class L2,R2 enhanced
    class N1,N2 process

    After --> Caller["呼び出し元で<br>record/verify/risk評価が失敗"]
    class Caller enhanced
```

**矢印の意味**: 矢印 A → B は「A の後に B が実行される（制御フロー）」を表す。

| 解析結果 | 制御判断 |
|---|---|
| 解析成功 + 危険検出あり | fail-closed（危険として報告） |
| 解析成功 + 危険検出なし | 安全（そのまま通過） |
| 解析不能・エラー | fail-closed（危険とみなして拒否） |

---

## 2. システム構成

### 2.1 修正対象パッケージの関係

| 修正対象パッケージ | 修正対象関数 | 所見 ID | 呼び出し元 |
|---|---|---|---|
| `internal/security/elfanalyzer/` | `standard_analyzer.go` — `lookupSyscallAnalysis` | C1 F-1 | `AnalyzeNetworkSymbols` |
| `internal/elfmagic/`（新規） | `elfmagic.go` — `Is` | C2 F-3 | `elfdynlib.Analyze`, `elfanalyzer`（ELF マジック判定の共有実装） |
| `internal/dynlib/elfdynlib/` | `analyzer.go` — `Analyze`, `parseELFDeps` | C2 F-3 | record コマンド（dynlib 解析） |
| `internal/dynlib/machodylib/` | `analyzer.go` — `parseMachODeps`, `HasDynamicLibDeps` | C2 F-3, C2 F-5 | record コマンド / runner（dynlib 解析 / ErrDynLibDepsRequired 判定） |
| `internal/verification/` | `manager.go` — `collectVerificationFiles`, `hasDynamicLibraryDeps` | B3 M1, B3 L1 | runner（group_executor） |
| `internal/runner/base/risk/` | `evaluator.go` — `applyBinaryAnalysis` | A5 Low-3 | `EvaluateRisk` |

### 2.2 コンポーネント配置

| 修正対象ファイル | 修正対象関数 | 所見 ID | 修正内容概要 |
|---|---|---|---|
| `internal/security/elfanalyzer/standard_analyzer.go` | `lookupSyscallAnalysis` | C1 F-1 | `default` エラーを fail-closed 化 |
| `internal/elfmagic/elfmagic.go`（新規） | `Is` | C2 F-3 | ELF マジック判定を `elfanalyzer` と `elfdynlib` で共有する新規パッケージ |
| `internal/dynlib/elfdynlib/analyzer.go` | `Analyze`, `parseELFDeps` | C2 F-3 | 子依存パース失敗・トップレベルエラーを fail-closed 化。マジック判定は `elfmagic.Is` を使用 |
| `internal/dynlib/machodylib/analyzer.go` | `Analyze`, `HasDynamicLibDeps` | C2 F-3, C2 F-5 | `parseMachODeps` 失敗を fail-closed 化、`Seek`/`io.ReadFull` 失敗を fail-closed 化 |
| `internal/verification/manager.go` | `collectVerificationFiles`, `hasDynamicLibraryDeps` | B3 M1, B3 L1 | パス解決失敗・`DynString` エラーを fail-closed 化 |
| `internal/runner/base/risk/evaluator.go` | `applyBinaryAnalysis` | A5 Low-3 | `default` 節追加 |

---

## 3. コンポーネント設計

### 3.1 C1 F-1: `lookupSyscallAnalysis` の fail-closed 化

#### 3.1.1 現状

`standard_analyzer.go:297-332` の `lookupSyscallAnalysis` は、`LoadSyscallAnalysis` の戻り値エラーを以下の3ケースに分岐する:

1. `ErrRecordNotFound`: キャッシュ不在 → `StaticBinary` を返す（正常なフォールバック）。
2. `ErrHashMismatch`: ハッシュ不一致 → `AnalysisError` を返す（fail-closed、改ざん検知）。
3. `default`: 想定外の I/O エラー等 → `slog.Debug` でログ出力し、`StaticBinary` として素通りする（fail-open）。

`default` ケースで `StaticBinary` が返ると、呼び出し元の `AnalyzeNetworkSymbols` はこれを「ストアにエントリなし」と同一視し、`NoNetworkSymbols` へフォールスルーする。verify 時の一過性 I/O エラーでネットワーク syscall 検出が漏れる可能性がある。

#### 3.1.2 修正後

`default` 節の挙動を変更する:
- ログレベルを `slog.Debug` から `slog.Warn` に格上げする。
- 戻り値を `binaryanalyzer.AnalysisError`（fail-closed）に変更する。
- エラー文脈は新規 sentinel error `ErrSyscallStoreIOError` によるラップで上位に伝播させる。

> **既存の `ErrSyscallAnalysisHighRisk` を再利用しない理由**: `ErrSyscallAnalysisHighRisk`（`standard_analyzer.go:41`）は既に `convertSyscallResult` 内で「未知 syscall の検出」「mprotect PROT_EXEC リスク検知」という、**解析が成功したうえで実際に高リスクと判定した**ケース専用の sentinel として使われている（同ファイル:352）。`default` ケースはストアの想定外 I/O エラーであり、解析自体が完了していない。両者を同一 sentinel に束ねると、`errors.Is(err, ErrSyscallAnalysisHighRisk)` だけでは「本物のセキュリティ検知」と「インフラ障害」を区別できなくなり、本設計の核心原則（§1.1: 「解析不能・エラー」と「対象なし・問題なし」の区別）と同種の区別を、今度はエラー型のレベルで壊してしまう。そのため `default` ケースには専用の新規 sentinel `ErrSyscallStoreIOError` を導入する。

運用上のトレーサビリティを確保するため、ログ出力には `"reason"` 構造化フィールドを追加し、値 `"store_io_error"` を設定する。これにより、オンコールエンジニアがログおよびエラー型の両方から「インフラ障害（`ErrSyscallStoreIOError`）」と「本物のセキュリティ検知（`ErrSyscallAnalysisHighRisk`）」を区別できる。

> **設計判断**: `default` ケースが使う新規 `ErrSyscallStoreIOError`、既存の `ErrHashMismatch` ケースが使う `ErrSyscallHashMismatch`、および `convertSyscallResult` が使う既存の `ErrSyscallAnalysisHighRisk` は、いずれも異なる sentinel error である。呼び出し元は `errors.Is(err, ErrSyscallHashMismatch)` でハッシュ不一致（改ざん検知）を、`errors.Is(err, ErrSyscallStoreIOError)` でストアの I/O 障害を、`errors.Is(err, ErrSyscallAnalysisHighRisk)` で解析成功後の高リスク検知を、それぞれ区別できる。三者とも `AnalysisError` を返す点で制御フロー上の fail-closed 動作は同一だが、エラー種別の区別は監視・アラート設計に必要である。

#### 3.1.3 責務表

| ファイル | 関数/構造体 | 責務 | 変更種別 |
|---|---|---|---|
| `internal/security/elfanalyzer/standard_analyzer.go` | `lookupSyscallAnalysis` | syscall analysis store の検索。想定外エラーを fail-closed（AnalysisError）で返す | 修正 |
| `internal/security/elfanalyzer/standard_analyzer.go` | `AnalyzeNetworkSymbols` | 呼び出し元。AnalysisError を受け取った場合の既存の挙動で正しく動作する | 変更不要 |
| `internal/security/elfanalyzer/errors.go`（または `standard_analyzer.go`） | `ErrSyscallStoreIOError`（新規 sentinel） | ストア I/O エラーを `ErrSyscallAnalysisHighRisk`（高リスク検知専用）と区別するための新規エラー変数 | 追加 |
| `internal/security/elfanalyzer/analyzer_test.go` | `TestLookupSyscallAnalysis_StoreIOError` | 新規追加 | 追加 |

#### 3.1.4 インタフェース変更

なし。`lookupSyscallAnalysis` は非公開関数であり、戻り値の型（`binaryanalyzer.AnalysisOutput`）も変更しない。

#### 3.1.5 呼び出し元への影響

`lookupSyscallAnalysis` の呼び出し元（`AnalyzeNetworkSymbols`、同ファイル:168-195）は、既に `AnalysisError` を正しく上位に伝播するコードパスを持つため、追加の変更は不要である。

---

### 3.2 C2 F-3: 子依存パース失敗の fail-closed 化

#### 3.2.1 現状（ELF側）

**トップレベル解析**（`elfdynlib/analyzer.go:115-127`）: `elf.NewFile` 失敗・`DynString(DT_NEEDED)` エラーをいずれも `nil, nil`（依存なし）と偽った判定にする（`//nolint:nilerr`）。

**BFS traversal 中の子依存パース失敗**（`elfdynlib/analyzer.go:207-218`）: `ErrDTRPATHNotSupported` のみ上位に伝播し、それ以外のパース失敗は `slog.Debug` + `continue` で子の traversal をスキップし無視する（fail-open）。

#### 3.2.2 現状（Mach-O側）

**BFS traversal 中の子依存パース失敗**（`machodylib/analyzer.go:215-221`）: `slog.Debug` + `continue` で子の traversal をスキップし無視する（fail-open）。

#### 3.2.3 修正後

**ELF トップレベル解析**: ELF マジックチェックを導入する。判定ロジックは `elfanalyzer.isELFMagic`（`standard_analyzer.go:288`、非公開）と実質同一であり、`elfdynlib` 側にも同じ判定が新たに必要になる。以下の理由から、両パッケージで重複実装するのではなく、新規パッケージ `internal/elfmagic` を切り出して共有する:

- **重複回避（DRY）**: ELF マジックバイト（`\x7fELF`）とその比較ロジックは、`elfanalyzer` と `elfdynlib` の双方が現時点で必要とする実装であり、将来に備えた予防的な抽象化ではない。
- **依存方向の妥当性**: `elfanalyzer.isELFMagic` を公開して `elfdynlib` から import する案は、依存解決を担う基盤寄りの `elfdynlib` が、syscall リスク解析を担う上位レイヤの `elfanalyzer` に依存するという逆行した結合を生む。`internal/elfmagic` はどちらのパッケージにも属さない leaf ユーティリティとして両者から依存される形にし、依存方向を自然に保つ。
- **コスト**: 新規ファイルは定数とごく短い比較関数のみ（10数行）。`elfanalyzer/standard_analyzer.go` は C1 F-1 の修正で既に本タスクの変更対象に含まれているため、そこでの置き換え（`isELFMagic` 呼び出しを `elfmagic.Is` に差し替え）は範囲外のコードへの侵食にはあたらない。

`internal/elfmagic` パッケージの内容:
- `Magic []byte`（`\x7fELF`）、`Len int`（4）を公開定数/変数として定義。
- `Is(b []byte) bool`: 先頭 `Len` バイトが `Magic` と一致するかを判定する（既存の `isELFMagic` と同一のロジック）。
- 依存先は `bytes` のみ。他パッケージへの依存を持たない leaf パッケージとする。

`elfanalyzer/standard_analyzer.go` の既存 `isELFMagic`／`elfMagic`／`elfMagicLen` は `elfmagic.Is`／`elfmagic.Magic`／`elfmagic.Len` の呼び出しに置き換え、重複定義を削除する（`AnalyzeNetworkSymbols` 等の既存呼び出し元の挙動に変更はない）。

`elfdynlib` 側は `elfmagic.Is` を用いて ELF トップレベル解析のマジックチェックを行う。処理の分岐は以下のとおり:

1. ファイル先頭から ELF マジック（4 バイト）を読み取る。読み取りに失敗するかマジックが不一致の場合は、非 ELF ファイルとして `nil, nil` を返す（正常系のスクリプト等に対応）。
2. マジック一致時は、Seek で先頭に戻したのち `elf.NewFile` を試行する。`elf.NewFile` 失敗時はエラーを返す（fail-closed）。
3. `elf.NewFile` 成功時は `DynString(DT_NEEDED)` を試行する。エラー時はエラーを返す（fail-closed）。`len(needed) == 0` の時のみ `nil, nil` を返す（依存なし・正常）。

> **なぜ既存の単純なアプローチでは要件を満たせないか**: 現在の ELF `Analyze` は `elf.NewFile` の失敗を一律 `nil, nil`（依存なし）に縮退させている。AC-05 は「ELF マジックを持つファイルのパース失敗をエラーとして区別すること」を要求しているため、単純にエラーを返すだけでは不十分であり、非 ELF ファイル（スクリプト等）の正常系パスを維持するためのマジックチェックが必須である。

**BFS traversal 中の子依存パース失敗（ELF・Mach-O 共通）**: `slog.Debug` を `slog.Warn` に格上げし、`continue` を `return nil, err`（ELF）/ `return nil, nil, err`（Mach-O）に変更する。ログ出力には `"reason": "child_parse_error"` 構造化フィールドを追加する。

**C2 F-3 の影響範囲（blast radius）**: BFS traversal 中に1つの子依存パースが失敗すると、当該バイナリの **全依存の記録が失敗** する（AC-04 が「解析全体の失敗」を要求しているため）。依存ツリーの一部に破壊された `.so` / `.dylib` が1つでも含まれると、当該バイナリの record 全体が失敗する。実運用への影響:
- `/usr/lib` の1つの共有ライブラリの破損が、それに依存する多数のバイナリの record を阻害しうる。
- 対応策として、エラーメッセージに失敗した子ライブラリのパスを明示し、どのファイルが原因かを運用者が特定できるようにする。

#### 3.2.4 責務表

| ファイル | 関数/構造体 | 責務 | 変更種別 |
|---|---|---|---|
| `internal/elfmagic/elfmagic.go` | `Magic`, `Len`, `Is` | ELF マジックバイトの定義と判定。`elfanalyzer`／`elfdynlib` から共有利用される leaf ユーティリティ | 追加（新規パッケージ） |
| `internal/elfmagic/elfmagic_test.go` | `TestIs` | 新規追加 | 追加 |
| `internal/security/elfanalyzer/standard_analyzer.go` | `isELFMagic`, `elfMagic`, `elfMagicLen` | 重複定義を削除し `elfmagic.Is`／`elfmagic.Magic`／`elfmagic.Len` の呼び出しに置き換え | 修正（削除+置換） |
| `internal/dynlib/elfdynlib/analyzer.go` | `Analyze` | ELF トップレベル解析。`elfmagic.Is` によるマジックチェック導入、エラー伝播 | 修正 |
| `internal/dynlib/elfdynlib/analyzer.go` | `Analyze`（BFSループ内） | 子依存パース失敗をエラー伝播 | 修正 |
| `internal/dynlib/machodylib/analyzer.go` | `Analyze`（BFSループ内） | 子依存パース失敗をエラー伝播 | 修正 |
| `internal/dynlib/elfdynlib/analyzer_test.go` | `TestAnalyze_TopLevelELFMagicMismatch` 他 | 新規追加 | 追加 |
| `internal/dynlib/elfdynlib/analyzer_test.go` | `TestAnalyze_ChildParseFailure` | 新規追加 | 追加 |
| `internal/dynlib/machodylib/analyzer_test.go` | `TestAnalyze_ChildParseFailure` | 新規追加 | 追加 |

#### 3.2.5 インタフェース変更

`Analyze` 自体の戻り値シグネチャ（ELF: `([]fileanalysis.LibEntry, error)`、Mach-O: `([]fileanalysis.LibEntry, []AnalysisWarning, error)`）は変更しない。エラー伝播は既存のエラーリターンパスを使用する。

新規に `internal/elfmagic` パッケージの公開 API（`Magic`, `Len`, `Is`）を追加する。`elfanalyzer` パッケージの `isELFMagic` 等は非公開関数であったため、削除・置換による外部インタフェースへの影響はない。

#### 3.2.6 呼び出し元への影響

- `Analyze` の呼び出し元（`cmd/record/main.go` 経由の `filevalidator`）は、既に `Analyze` のエラーを record 失敗として扱っているため、追加の変更は不要。
- ただし、これまでエラーを握りつぶして続行していたケースがエラーになるため、実運用環境で一過性の I/O エラー等により record が失敗する可能性が増加する。このリスクは `01_requirements.md` の「リスクと留意事項」で既に特定されており、ユーザー向けエラーメッセージの品質確保と、構造化ログによる原因特定容易性の向上が実装時の課題となる。
- 既存テストのうち、破壊された ELF ファイルや不正な Mach-O ファイルの解析が「エラーとして返る」ことを期待していないテスト（もし存在すれば）は、修正が必要。
  - `internal/dynlib/elfdynlib/analyzer_test.go`: AC-04/AC-05/AC-07 に対応する fail-closed テストを新規追加する（既存テストの修正は不要と見込まれる）。
  - `internal/dynlib/machodylib/analyzer_test.go`: AC-06/AC-07/AC-08/AC-09 に対応するテストを新規追加。

---

### 3.3 C2 F-5: `HasDynamicLibDeps` の fail-closed 化

#### 3.3.1 現状

`machodylib/analyzer.go:617-632` の単一アーキテクチャ Mach-O パスでは、`Seek` 失敗（2箇所）と `io.ReadFull` 失敗がいずれも `return false, nil`（fail-open）に縮退する。`HasDynamicLibDeps` は runner 側で `ErrDynLibDepsRequired` をトリガするゲートであり、I/O エラーを誘発できる状況ではこのゲートが黙ってスキップされ得る。

#### 3.3.2 修正後

- `Seek` 失敗（2箇所）: I/O エラーとしてエラーを返す。
- `io.ReadFull` 失敗: `io.EOF` / `io.ErrUnexpectedEOF` はファイルが Mach-O ヘッダ長（4バイト）に満たないことを示し、非 Mach-O ファイルの正常ケースであるため `(false, nil)` を返す。それ以外のエラーは I/O エラーとしてエラーを返す（fail-closed）。
- ログ出力には `"reason": "io_error"` 構造化フィールドを追加する。

> **Seek/ReadFull でのディスクリプタ再利用**: `HasDynamicLibDeps` の既存実装では、`Seek` の型アサーション `file.(io.Seeker)` に失敗すると Seek ブロック全体がスキップされる（fail-open）。これは `safefileio.SafeOpenFile` が返す File が常に `io.Seeker` を実装するため、通常は発生しない。ただし、将来 `safefileio` の File 型が `io.Seeker` を実装しなくなった場合、このコードパスで ELF/Mach-O マジックが読み取れず、分析不能になる。このリスクは現在の実装では顕在化しないが、`safefileio` の変更時に注意が必要である。

#### 3.3.3 責務表

| ファイル | 関数/構造体 | 責務 | 変更種別 |
|---|---|---|---|
| `internal/dynlib/machodylib/analyzer.go` | `HasDynamicLibDeps` | Mach-O バイナリの動的依存有無判定。I/O エラーをエラー伝播 | 修正 |
| `internal/dynlib/machodylib/analyzer_test.go` | `TestHasDynamicLibDeps_SeekError` | 新規追加 | 追加 |
| `internal/dynlib/machodylib/analyzer_test.go` | `TestHasDynamicLibDeps_ReadFullError` | 新規追加 | 追加 |

#### 3.3.4 インタフェース変更

なし。`HasDynamicLibDeps` の戻り値シグネチャ `(bool, error)` は変更しない。

#### 3.3.5 呼び出し元への影響

- `internal/verification/manager.go:725` の `hasMachODynamicLibraryDeps` は `HasDynamicLibDeps` のエラーを既に上位に伝播しているため、追加の変更は不要。
- 既存テストは normal FileSystem を使用し、エラー注入は行っていないため、修正不要。

---

### 3.4 B3 M1: `collectVerificationFiles` の fail-closed 化

#### 3.4.1 現状

`verification/manager.go:264-277` の `collectVerificationFiles` は、`pathResolver.ResolvePath` の失敗を `slog.Warn` + `continue` で無視し、当該コマンドを検証対象集合から静かに除外する。`collectVerificationFiles` の戻り値は `map[string]struct{}` であり、エラーを返せないシグネチャである。

#### 3.4.2 修正後

`collectVerificationFiles` のシグネチャを `(map[string]struct{}, error)` に変更する。パス解決失敗時は `slog.Warn` を出力した上でエラーを返す（fail-closed）。

呼び出し元 `VerifyGroupFiles`（同ファイル:196）は、`collectVerificationFiles` の戻り値エラーをチェックし、`OpError` にラップして上位に伝播する。`VerifyGroupFiles` の外部シグネチャ（`(*Result, error)`）は変更不要である。

ログ出力には `"reason": "path_resolution_failed"` 構造化フィールドを追加する。

#### 3.4.3 責務表

| ファイル | 関数/構造体 | 責務 | 変更種別 |
|---|---|---|---|
| `internal/verification/manager.go` | `collectVerificationFiles` | 検証対象ファイルの収集。戻り値の型に `error` を追加、パス解決失敗をエラー伝播 | 修正 |
| `internal/verification/manager.go` | `VerifyGroupFiles` | 呼び出し元。エラーハンドリングを追加 | 修正 |
| `internal/verification/manager_test.go` | `TestCollectVerificationFiles_PathResolutionFailure` | 新規追加 | 追加 |

#### 3.4.4 インタフェース変更

`collectVerificationFiles` は非公開関数であり、公開インタフェースへの影響はない。`VerifyGroupFiles`（公開メソッド）の外部シグネチャは変更なし。

#### 3.4.5 呼び出し元への影響

- `VerifyGroupFiles` の直接の呼び出し元（`internal/runner/group_executor.go`）は、既に `VerifyGroupFiles` のエラーを上位に伝播しているため、追加の変更は不要。
- `internal/verification/manager_test.go`: `TestVerifyGroupFiles_*` 系のテストで、パス解決不可能なコマンドを含むグループの検証がエラーを返すようになる。AC-11/AC-12/AC-13 に対応するテストケースを新規追加する。

---

### 3.5 B3 L1: `hasDynamicLibraryDeps` の fail-closed 化

#### 3.5.1 現状

`verification/manager.go:711-715` の `hasDynamicLibraryDeps` は、`elfFile.DynString(elf.DT_NEEDED)` のエラーを `(false, nil)`（依存なし）と偽った判定にする（fail-open）。動的セクションが破壊された ELF は `ErrDynLibDepsRequired` を回避し、dynlib 検証要求をバイパスし得る。

#### 3.5.2 修正後

`DynString` のエラーと `len(needed) == 0` を分離する:
- `DynString` エラー時: `(false, err)` を返す（fail-closed）。
- `len(needed) == 0` 時: `(false, nil)` を返す（依存なし・正常）。
- `len(needed) > 0` 時: `(true, nil)` を返す（依存あり）。

#### 3.5.3 責務表

| ファイル | 関数/構造体 | 責務 | 変更種別 |
|---|---|---|---|
| `internal/verification/manager.go` | `hasDynamicLibraryDeps` | ELF バイナリの動的依存有無判定。DynString エラーをエラー伝播 | 修正 |
| `internal/verification/manager_test.go` | `TestHasDynamicLibraryDeps_DynStringError` | 新規追加 | 追加 |

#### 3.5.4 インタフェース変更

なし。`hasDynamicLibraryDeps` は非公開関数であり、戻り値シグネチャ `(bool, error)` は変更しない。

#### 3.5.5 呼び出し元への影響

- `verifyDynLibDeps`（同ファイル:664）は `hasDynamicLibraryDeps` のエラーを `fmt.Errorf` でラップして上位に伝播しているため、追加の変更は不要。
- `verifyDynLibDeps` の呼び出し元 `VerifyCommandDynLibDeps`（同ファイル:592）は、このエラーを `group_executor.go` の `verifyGroupFiles` 内の一括検証ループ（442行目、`dlErr`）へ伝播する。当該箇所はエラーを受け取ると即座に `return dlErr` し、`ExecuteGroup` まで伝播してグループ実行を中断する。この経路には `ResultCollector` 記録や dry-run 用のバイパスは存在しない（`ResultCollector` は `manager.go` の `verifyFileWithHash`、すなわちファイルハッシュ検証専用であり、dynlib 依存チェックの経路には関与しない）。したがって、**B3 L1 の修正により、これまで `(false, nil)` で静かに通過していたケースが、dry-run を含めてグループ実行を中断させるようになる**。これは AC-10/AC-15 が要求する fail-closed 化そのものであり意図した挙動だが、dry-run のプレビュー継続性には影響する（詳細は §5.3）。

---

### 3.6 A5 Low-3: `applyBinaryAnalysis` の `default` 節追加

#### 3.6.1 現状

`evaluator.go:461-477` の `applyBinaryAnalysis` は、`BinaryAnalysisClass` の switch で `Uncertain`/`HighRisk`/`Network`/`Clean` の4定数のみを列挙し、`default` 節がない。将来クラスが追加された場合、switch を素通りして無寄与（実質 Clean 扱い）になる。

> **既存テストがアサートする挙動**: `internal/runner/base/risktypes/types_test.go:16-28`（`TestBinaryAnalysisClass_ZeroValueIsUncertain`）はゼロ値が `BinaryAnalysisUncertain` であることを確認している。このテストは型の定義に対する静的アサーションであり、修正後も変更不要である。

#### 3.6.2 修正後

`default` 節を追加する。既存の `Uncertain` ケースと同様に `blockingAssessment("", "")` を用いて Blocking を返す。ReasonCodes は `result.ReasonCodes` を引き継ぐ。

新規の ReasonCode や ErrorClass の追加は不要であり、既存の空文字パターン（`blockingAssessment("", "")`）を踏襲する。これは既存の `Uncertain` ケースと完全に同一のシグネチャであり、両者の Blocking 挙動は一致する。

- 戻り値の型: `*risktypes.RiskAssessment`（nil でない Blocking アセスメント）
- 既存4クラスの挙動: 変更なし

#### 3.6.3 責務表

| ファイル | 関数/構造体 | 責務 | 変更種別 |
|---|---|---|---|
| `internal/runner/base/risk/evaluator.go` | `applyBinaryAnalysis` | バイナリ解析結果のリスク評価。`default` 節で未知クラスを Blocking に倒す | 修正 |
| `internal/runner/base/risk/evaluator_test.go` | `TestApplyBinaryAnalysis_UnknownClass` | 新規追加 | 追加 |

#### 3.6.4 インタフェース変更

なし。`applyBinaryAnalysis` は非公開関数であり、戻り値シグネチャは変更しない。

#### 3.6.5 呼び出し元への影響

- `evaluateDimensions`（同ファイル:337）は `applyBinaryAnalysis` の `blocked` 戻り値（非 nil）を既に正しく処理しているため、追加の変更は不要。
- 既存の4クラスに関するテストは影響を受けない（AC-18）。

---

## 4. エラーハンドリング設計

### 4.1 共通パターン

全6箇所の修正に共通するエラーハンドリングパターン:

1. `slog.Debug` による無視 → `slog.Warn` へ格上げ
2. エラーを無視して「問題なし」を返す → エラーを呼び出し元に伝播
3. `switch` の `default` なし → `default` 節で fail-closed

### 4.2 エラーメッセージと構造化ログ設計

各修正箇所では、`slog.Warn` の出力に以下の構造化フィールドを追加し、オンコールエンジニアがログのみから障害のカテゴリを特定できるようにする:

| 修正箇所 | `"reason"` フィールド値 | エラーメッセージの方向性 |
|---|---|---|
| C1 F-1 | `"store_io_error"` | syscall store の想定外 I/O エラーを区別 |
| C2 F-3 (ELF child) | `"child_parse_error"` | 子 ELF パース失敗を区別 |
| C2 F-3 (Mach-O child) | `"child_parse_error"` | 子 Mach-O パース失敗を区別 |
| C2 F-5 | `"io_error"` | Seek/ReadFull 失敗を区別 |
| B3 M1 | `"path_resolution_failed"` | コマンドパス解決失敗を区別 |
| B3 L1 | 構造化ログなし（エラー伝播のみ） | DynString エラーは fmt.Errorf ラップで上位に伝播 |

### 4.3 新規エラー型

C1 F-1 の `default` ケース用に新規 sentinel error `ErrSyscallStoreIOError` を1つ導入する（既存の `ErrSyscallAnalysisHighRisk` は解析成功後の高リスク検知専用のため転用しない。理由は §3.1.2 を参照）。それ以外は既存のエラー型を以下のとおり再利用する:

| 修正箇所 | 使用する型 / パターン |
|---|---|
| C1 F-1 | `binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.AnalysisError}`, 新規 `ErrSyscallStoreIOError` によるラップ |
| C2 F-3 | `fmt.Errorf("...: %w", err)` によるラップ |
| C2 F-5 | `fmt.Errorf("...: %w", err)` によるラップ |
| B3 M1 | `OpError` によるラップ |
| B3 L1 | `fmt.Errorf("...: %w", err)` によるラップ |
| A5 Low-3 | `blockingAssessment("", "")` — 既存の `Uncertain` ケースと同一パターン |

### 4.4 一過性 I/O エラーへの対応

本修正により、一過性の I/O エラー（NFS 切断、ディスク I/O エラー、tmpfs バックプレッシャー等）が発生した場合、各処理が fail-closed（エラー伝播）に倒れる。本設計では以下の理由からリトライループやサーキットブレーカーパターンを導入しない:

1. **セキュリティクリティカルパスにおける再試行の危険性**: 検証・解析が失敗した場合、再試行によって偶然成功しても、その間にファイルが改ざんされた可能性を排除できない。fail-closed で即座に停止することが安全側の挙動である。
2. **OS/ファイルシステムレベルの再試行**: I/O エラーの多くは OS カーネルまたは NFS クライアントが透過的に再試行する。Go の `os` パッケージに到達するエラーは、OS レベルで回復不能と判断されたものである。
3. **運用側の対応**: 一時的な環境障害（NFS マウントの再マウント、ディスク容量の回復等）は、運用者が環境を修復した後に record/verify を再実行することで対応する。これにより、意図しないタイミングでの再試行によるセキュリティ境界の曖昧化を回避する。

---

## 5. セキュリティ考慮事項

### 5.1 脅威モデル

```mermaid
flowchart TD
    classDef threat fill:#f5e6ff,stroke:#8b5cf6,stroke-width:2px,color:#4c1d95;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph 攻撃者["攻撃者（ファイルシステム書き込み権限あり）"]
        A1["一過性 I/O エラーを誘発する<br>（ディスク障害、NFS 切断、<br>リソース枯渇）"]
        A2["破壊された ELF/Mach-O を配置<br>（動的セクション破壊、<br>マジック偽装）"]
        A3["パスを解決できない環境を構築<br>（PATH 改ざん、シンボリック<br>リンク操作）"]
    end

    A1 --> V1["C1 F-1: syscall store 読み取り失敗<br>→ 静かに StaticBinary 扱い<br>→ ネットワーク syscall 検出漏れ"]
    A2 --> V2["C2 F-3: 子依存パース失敗<br>→ サブツリー全体が検証対象外<br>C2 F-5: Mach-O マジック読み取り失敗<br>→ DynLibDeps 要求をスキップ"]
    A3 --> V3["B3 M1: パス解決失敗<br>→ ハッシュ検証なしで実行<br>B3 L1: DT_NEEDED 読み取り失敗<br>→ 動的依存チェックをバイパス"]

    class A1,A2,A3 threat
    class V1,V2,V3 problem

    subgraph 対策["本設計の対策"]
        M1["全エラーパスを fail-closed 化<br>（エラー伝播、slog.Warn 格上げ）"]
        M2["ELF マジックチェックによる<br>非ELFとELF破壊の区別"]
        M3["switch default 節による<br>将来の未知クラスへの防御"]
    end

    V1 -.->|"修正"| M1
    V2 -.->|"修正"| M1
    V2 -.->|"修正"| M2
    V3 -.->|"修正"| M1

    class M1,M2,M3 enhanced
```

**矢印の意味**: 実線矢印 A → B は「攻撃シナリオ A が脆弱性 B を突く」、点線矢印 A -→ B は「脆弱性 A が対策 B によって修正される」ことを表す。

#### Legend
| Class | 意味 |
|---|---|
| `threat`（紫） | 攻撃者 / 攻撃手法 |
| `enhanced`（緑） | 修正対象コンポーネント / 対策 |
| `problem`（赤） | 問題のある既存コード / 脆弱性 |

### 5.2 セキュリティ設計パターン

本設計は、以下の既存セキュリティアーキテクチャ原則に完全に合致する:

- **Fail-Safe Design**（[security-architecture.md](../../dev/architecture_design/security-architecture.md) 第3節）: 「Default deny for all operations」を全エラーパスに適用する。
- **Defense-in-Depth**: 本修正は防御層の一貫性を高めるものであり、新たな防御層を追加するものではない。
- **fail-closed の一貫性**: 既に fail-closed として実装されている `ErrHashMismatch` ケースと、今回 fail-closed 化する `default` ケースの間にあった非対称性を解消する。

### 5.3 副作用と影響範囲

- **record/verify の失敗率上昇**: これまでエラーを握りつぶして続行していた環境（一過性の I/O エラー等）では、record/verify が失敗するようになる。ユーザーへの影響を最小化するため、エラーメッセージは具体的な原因（どのファイルの、どの操作が、どのような理由で失敗したか）を含める。構造化ログフィールド（§4.2）により、オンコールエンジニアは障害カテゴリをプログラム的に特定できる。
- **C2 F-3 BFS traversal の影響範囲**: 依存ツリー中の1つの破壊されたライブラリが、当該バイナリの全依存記録を失敗させる（§3.2.3 参照）。
- **dry-run モード**: 修正箇所によって、dry-run 時の扱いが異なる（一括りに「非致命的」ではない点に注意）:
  - **C2 F-3**: `record` コマンド（`cmd/record/main.go`）の解析経路でのみ発生する。`record` コマンドに `--dry-run` 相当のフラグは存在しない（`cmd/record/main.go` のフラグ定義を参照）ため、runner の dry-run モードとは無関係であり、修正後は record 処理自体がエラーで失敗する。
  - **C1 F-1**: `EvaluateRisk` を経由し、`plan.Assessment.Blocking` として risk evaluator の結果に現れる。runner の dry-run 実行は `internal/runner/resource/dryrun_manager.go` の専用パスを持ち、そこでは `Blocking` を（`normal_manager.go` のように `ErrCommandSecurityViolation` で即中断するのではなく）分析結果に説明を付与する形で扱う。したがって C1 F-1 の fail-closed 化は、dry-run のプレビュー継続性を壊さない可能性が高い（実装時に `dryrun_manager.go` 経由のケースで確認する）。
  - **C2 F-5 / B3 L1**: `VerifyCommandDynLibDeps` のエラーは `group_executor.go` の `verifyGroupFiles` 内の一括検証ループ（442行目）で即座に `return dlErr` され、`ExecuteGroup` まで伝播する。この経路に dry-run 用のバイパスは存在しない。したがって **dry-run でもグループ実行が中断されるようになる**（従来は `(false, nil)` で静かに通過していた）。
  - **B3 M1**: `collectVerificationFiles` のエラーは `VerifyGroupFiles` 内で即座に `OpError` として返る（§3.4.2）。これは `VerifyGroupFiles` 内の既存 dry-run バイパス（`FailedFiles` 後処理限定、`m.isDryRun` チェック）よりも前の段階で発生するため、同様に **dry-run でも検証が中断される**。
  - 上記のとおり、C2 F-5 / B3 L1 / B3 M1 は dry-run の非致命的挙動を変更する（これまで見えなかった fail-open 経路を dry-run でも顕在化させる）。これは fail-closed 化の意図と整合するが、運用者への影響（プレビューが以前より失敗しやすくなる）として明示し、リリースノート等で周知する必要がある。
- **後方互換性**: 正常系（エラーが発生しないケース）の挙動は変更されない。すべての AC が正常系のリグレッション防止を要求している。

### 5.4 ロールアウト安全性

本修正のデプロイにあたり、以下のリスクと推奨手順を考慮する:

- **record データの再取得**: C2 F-3 修正後、依存ツリーに破壊されたライブラリを含むバイナリの record が新たに失敗するようになる。`record` コマンドに dry-run 相当のフラグは存在しない（§5.3 参照）ため、デプロイ前に影響を受けるバイナリを特定するには、修正後のバイナリで実際に `record` を実行し、失敗するバイナリの一覧を確認する必要がある。
- **段階的ロールアウト**: strict/lenient モードのトグルは本設計では提供しない（YAGNI）。fail-closed への移行は一括で行い、事前テストで影響範囲を確認する。
- **runner dry-run プレビューの再確認**: §5.3 のとおり、C2 F-5 / B3 L1 / B3 M1 は runner の dry-run 実行を新たに中断させ得る。本番デプロイ前に、既存の運用構成（グループ定義）に対して修正後のバイナリで dry-run を実行し、想定外の中断が発生しないことを確認する。
- **アラート設計**: C1 F-1 の `"reason": "store_io_error"` ログに対するアラートを設定し、syscall store の I/O 障害を検知できるようにする。

---

## 6. 処理フロー詳細

### 6.1 C1 F-1: `lookupSyscallAnalysis` のエラーハンドリングフロー

```mermaid
flowchart TD
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    Start(["lookupSyscallAnalysis"]) --> Load["LoadSyscallAnalysis<br>(path, contentHash)"]
    Load --> CheckError{"err != nil ?"}
    CheckError -->|"RecordNotFound"| ReturnStatic["StaticBinary を返す<br>（キャッシュ不在）"]
    CheckError -->|"ErrHashMismatch"| ReturnAnalysisError["AnalysisError を返す<br>（改ざん検知）"]
    CheckError -->|"その他のエラー"| LogWarn["slog.Warn 出力<br>reason: store_io_error"]
    LogWarn --> ReturnAnalysisError2["AnalysisError を返す<br>（fail-closed）"]
    CheckError -->|"err == nil"| CheckNil{"result == nil ?"}
    CheckNil -->|"nil"| ReturnStatic2["StaticBinary を返す<br>（解析結果空）"]
    CheckNil -->|"非 nil"| Convert["convertSyscallResult"]
    Convert --> ReturnConverted["変換結果を返す"]

    class ReturnStatic,ReturnStatic2,ReturnConverted process
    class ReturnAnalysisError,ReturnAnalysisError2,LogWarn enhanced
```

**矢印の意味**: 矢印 A → B は「A の判定結果により B の処理が実行される」ことを表す。`enhanced` クラス（緑）のノードが修正後の追加/変更部分。

#### Legend
| Class | 意味 |
|---|---|
| `process`（橙） | 既存の挙動（変更なし） |
| `enhanced`（緑） | 修正対象の挙動 |

### 6.2 C2 F-3: ELF `Analyze` のトップレベルフロー

```mermaid
flowchart TD
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    Start(["Analyze(binaryPath)"]) --> SafeOpen["SafeOpenFile"]
    SafeOpen --> ReadMagic["io.ReadFull で<br>ELF マジック読み取り"]
    ReadMagic --> CheckMagic{"ELFマジック検証"}
    CheckMagic -->|"不一致"| ReturnNil["nil, nil を返す<br>（非ELF・正常）"]
    CheckMagic -->|"一致"| SeekBack["Seek で先頭に戻す"]
    SeekBack --> NewFile["elf.NewFile"]
    NewFile --> CheckNewFile{"パース成功 ?"}
    CheckNewFile -->|"失敗"| ReturnError["エラーを返す<br>（fail-closed）"]
    CheckNewFile -->|"成功"| DynString["DynString(DT_NEEDED)"]
    DynString --> CheckDynStr{"取得成功 ?"}
    CheckDynStr -->|"失敗"| ReturnError2["エラーを返す<br>（fail-closed）"]
    CheckDynStr -->|"len == 0"| ReturnNil2["nil, nil を返す<br>（依存なし・正常）"]
    CheckDynStr -->|"len > 0"| BFS["BFS traversal 開始"]

    class ReturnNil,ReturnNil2,BFS process
    class ReadMagic,SeekBack,CheckMagic,ReturnError,ReturnError2 enhanced
```

**矢印の意味**: 矢印 A → B は「A の次に B が実行される」ことを表す。`enhanced` クラス（緑）のノードが修正後の追加/変更部分。

#### Legend
| Class | 意味 |
|---|---|
| `process`（橙） | 既存の挙動（変更なし） |
| `enhanced`（緑） | 修正対象の挙動 |

### 6.3 B3 M1: `collectVerificationFiles` のフロー

```mermaid
flowchart TD
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    Start(["collectVerificationFiles"]) --> AddExplicit["明示的ファイルを<br>fileSet に追加"]
    AddExplicit --> Loop["各コマンドについて"]
    Loop --> ResolvePath["pathResolver.ResolvePath"]
    ResolvePath --> CheckErr{"解決成功 ?"}
    CheckErr -->|"失敗"| ReturnError["slog.Warn 出力 →<br>エラーを返す<br>（fail-closed）"]
    CheckErr -->|"成功"| AddToSet["fileSet に解決済み<br>パスを追加"]
    AddToSet --> Loop
    Loop -->|"全コマンド処理完了"| ReturnSet["fileSet を返す<br>（正常）"]

    class ReturnSet,AddExplicit,AddToSet process
    class ReturnError enhanced
```

**矢印の意味**: 矢印 A → B は「A の次に B が実行される」ことを表す。`enhanced` クラス（緑）のノードが修正後の変更部分。

#### Legend
| Class | 意味 |
|---|---|
| `process`（橙） | 既存の挙動（変更なし） |
| `enhanced`（緑） | 修正対象の挙動 |

---

## 7. テスト戦略

### 7.1 単体テスト戦略

各修正箇所に対して、以下のカテゴリのテストを追加する:

| AC | テスト対象 | テスト種別 | テスト内容 |
|---|---|---|---|
| AC-01 | `lookupSyscallAnalysis` | 単体 | 想定外エラー時に `AnalysisError` が返ること |
| AC-02 | `lookupSyscallAnalysis` | 単体 | `RecordNotFound` 時に `StaticBinary` が返ること（既存挙動維持） |
| AC-03 | `lookupSyscallAnalysis` | 単体 | 想定外エラー時に `slog.Warn` レベル以上のログが出力されること |
| AC-04 | `elfdynlib.Analyze`（BFS traversal） | 単体 | 子 ELF パース失敗がエラーとして伝播すること |
| AC-05 | `elfdynlib.Analyze`（トップレベル） | 単体 | ELF マジックあり + パース失敗がエラーとして伝播すること、非ELF は nil, nil が返ること |
| AC-06 | `machodylib.Analyze`（BFS traversal） | 単体（darwin ビルドタグ） | 子 Mach-O パース失敗がエラーとして伝播すること |
| AC-07 | `elfdynlib.Analyze`, `machodylib.Analyze` | 単体 + 統合 | 正常系（依存あり/なし、多階層依存）が従来どおり成功すること |
| AC-08 | `HasDynamicLibDeps` | 単体（darwin ビルドタグ） | `Seek` 失敗時に `(false, err)` が返ること |
| AC-09 | `HasDynamicLibDeps` | 単体（darwin ビルドタグ） | `io.ReadFull` 失敗（非EOF）時に `(false, err)` が返ること |
| AC-10 | `hasMachODynamicLibraryDeps` | 単体 | AC-08/AC-09 のエラーが上位に伝播すること |
| AC-11 | `collectVerificationFiles` | 単体 | パス解決失敗時にエラーが返ること |
| AC-12 | `VerifyGroupFiles` | 単体 | パス解決失敗により検証なし実行経路が存在しないこと |
| AC-13 | `VerifyGroupFiles` | 単体 | 正常系のパス解決が従来どおり成功すること |
| AC-14 | `hasDynamicLibraryDeps` | 単体 | `DynString` エラー時に `(false, err)` が返ること |
| AC-15 | `verifyDynLibDeps` | 単体 | `hasDynamicLibraryDeps` のエラーが検証失敗として伝播すること |
| AC-16 | `hasDynamicLibraryDeps` | 単体 | DT_NEEDED なし（正常系）が `(false, nil)` を返すこと |
| AC-17 | `applyBinaryAnalysis` | 単体 | 未知クラスが Blocking を返すこと |
| AC-18 | `applyBinaryAnalysis` | 単体 | 既存4クラスの挙動が変更されないこと |

### 7.2 テストファイル構成

- `internal/security/elfanalyzer/analyzer_test.go`: AC-01, AC-02, AC-03 のテストを追加
- `internal/dynlib/elfdynlib/analyzer_test.go`: AC-04, AC-05, AC-07 のテストを追加
- `internal/dynlib/machodylib/analyzer_test.go`: AC-06, AC-07, AC-08, AC-09 のテストを追加
- `internal/verification/manager_test.go`: AC-10, AC-11, AC-12, AC-13, AC-14, AC-15, AC-16 のテストを追加
- `internal/runner/base/risk/evaluator_test.go`: AC-17, AC-18 のテストを追加

### 7.3 統合テスト戦略

- 多階層依存・循環依存を持つ実バイナリに対する record/verify の正常系テスト（AC-07）
- 正常にパス解決できるコマンドのみで構成されるグループの検証テスト（AC-13）

### 7.4 セキュリティテスト戦略

- 破壊された ELF（DT_NEEDED セクション破壊）に対する `hasDynamicLibraryDeps` のエラー伝播確認
- syscall analysis store に対する I/O エラー注入テスト
- `applyBinaryAnalysis` への不正な `BinaryAnalysisClass` 値の注入テスト

---

## 8. 実装優先順位

### 8.1 フェーズ分割

| Phase | 修正箇所 | 依存関係 | リスク |
|---|---|---|---|
| Phase 1 | A5 Low-3（`applyBinaryAnalysis`） | なし | 低（`default` 節の追加のみ） |
| Phase 2 | B3 L1（`hasDynamicLibraryDeps`） | なし | 低（条件分岐の分離のみ） |
| Phase 3 | B3 M1（`collectVerificationFiles`） | なし | 中（シグネチャ変更あり） |
| Phase 4 | C1 F-1（`lookupSyscallAnalysis`） | なし | 低（`default` 節の変更のみ） |
| Phase 5 | C2 F-5（`HasDynamicLibDeps`） | なし | 低（エラー伝播追加のみ） |
| Phase 6 | C2 F-3（ELF/Mach-O 子依存パース、`internal/elfmagic` 新設） | ファイル競合注意（下記） | 中（マジックチェック導入あり） |

機能的な依存関係はなく全フェーズ並行実装可能だが、Phase 6 は `internal/elfmagic` への切り出しに伴い `internal/security/elfanalyzer/standard_analyzer.go` を変更する。同ファイルは Phase 4（C1 F-1）でも変更対象であるため、両フェーズを別ブランチ/別PRで並行実装する場合はマージ時のファイル競合に注意する。

### 8.2 実装順序の根拠

フェーズを変更量とリスクで昇順に並べている。Phase 1（A5 Low-3: `default` 節追加のみ）から着手し、Phase 6（C2 F-3: マジックチェック導入、`internal/elfmagic` 新設）を最後にすることで、各修正の影響を個別に確認しながら進められる。Phase 6 を最後に置くことで、`standard_analyzer.go` に対する Phase 4 の変更が先に確定し、Phase 6 での `isELFMagic` 置き換えがその上に素直に積み重なる。

---

## 9. 将来の拡張性

### 9.1 設計上の考慮点

- **エラー型の統一**: 現状、各パッケージが独自のエラー型を使用している。将来的に「エラー握りつぶしによる fail-open」パターンを静的に検出する lint ルールを導入できるよう、エラーハンドリングのパターンを統一することが考えられる。ただし、本タスクのスコープ外であり、YAGNI の原則から本設計では扱わない。
- **slog.Warn の頻度**: ログレベルを `Debug` から `Warn` に格上げする箇所（AC-03, AC-04, AC-06）は、実運用上のログ出力量に影響を与えうる。`01_requirements.md` の非機能要件に従い、実装後にログ出力量の検証を行う必要がある。
- **パフォーマンス**: ELF `Analyze` のマジックチェックは、解析対象バイナリ1件あたり `io.ReadFull(4)` + `Seek(0, SeekStart)` の2回の追加 syscall を発生させる。1回の record 処理で数万のバイナリを解析するユースケースでは線形のオーバーヘッドとなるが、追加コストは1バイナリあたり O(1) であり、ハッシュ計算（ファイル全体の読み取り）に比べて無視できる。`safefileio.SafeOpenFile` が返す File は Linux/macOS ともに `io.Seeker` を実装しており、Seek の型アサーション失敗による分析不能化は発生しない。
- **Mach-O 側のマジック判定重複**: 本設計では ELF 側のマジック判定を `internal/elfmagic` に切り出したが、Mach-O 側には `internal/security/machoanalyzer/standard_analyzer.go` の `isMachOMagic`、同 `svc_scanner.go` の `isMachOMagicAll`、`internal/dynlib/machodylib/analyzer.go` の `looksLikeMachO` という3つの独立実装が既に存在する（判定対象マジック値の集合が異なる可能性があり要棚卸し）。本タスクのスコープ外のため対応せず、フォローアップとして [issue #876](https://github.com/isseis/go-safe-cmd-runner/issues/876) に切り出した。
- **未着手の類似パターン**:
  - `internal/filevalidator/validator.go` の `//nolint:nilerr` パターン（`validator.go:1587`: Symtab 不在を空スライスに縮退、`validator.go:1652`: Mach-O パース失敗を `nil, nil` に縮退）は、本タスクと同型の fail-open パターンである。これらは record パス（`cmd/record/main.go`）のみに影響し、verify 時の fail-open には該当しない。`01_requirements.md` のスコープ定義に従い本タスクでは対象外とするが、将来のタスクで対応可能である。
  - D1 (groupmembership) L-2/L-3: `01_requirements.md` スコープ外。未着手。

### 9.2 決定履歴

> 本設計は新規作成であり、削除・置換された過去の設計は存在しない。git 履歴を参照すること。
