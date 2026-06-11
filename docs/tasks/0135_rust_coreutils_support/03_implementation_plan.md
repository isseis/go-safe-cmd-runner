# 実装計画書：Ubuntu 26.04 Rust coreutils 対応

## ドキュメントステータス

| 項目 | 内容 |
|---|---|
| ステータス | `approved` |
| 作成日 | 2026-06-11 |
| レビュー日 | 2026-06-11 |
| レビュアー | isseis |
| コメント | - |

---

## 1. 実装概要 (Implementation Overview)

### 1.1. 目的

`02_architecture.md` の設計に従い、`/usr/lib/cargo/bin/coreutils` 配下に解決された
coreutils コマンドを、単一バイナリのバイナリ解析結果に依存せずに分類する仕組みを実装する。
実行時経路（`EvaluateRisk`）と dry-run 経路（`AnalyzeCommandSecurity`）の双方が同一の
分類関数 `CoreutilsCommandRisk` を共有し、要件 F-001〜F-005 と非機能 4.1 を満たす。

設計の詳細（判定順序・不変条件・脅威モデル・ケース分析）は `02_architecture.md` を参照する。
本書は実装タスクと検証手順のみを記述し、設計内容は再掲しない。

### 1.2. 実装原則

- **設計への準拠:** 公開シグネチャ・判定ステップの位置は `02_architecture.md` §3.2 / §6.1 に従う。
- **既存実装の再利用:** 後述の既存関数（`findFirstSubcommand` / `hasSetuidOrSetgidBit`）を再利用し、重複実装しない（DRY）。
- **適用範囲の限定:** coreutils ディレクトリ直下のみに作用し、他ディレクトリの挙動を変えない（要件 F-002 / 非機能 4.1）。
- **Go ソースの記述言語:** コメント・識別子・文字列リテラルはすべて英語で記述する。

### 1.3. 既存コード調査結果

実装着手前に対象パッケージを調査した結果を記す。

**再利用する既存関数（いずれも `internal/runner/base/security` パッケージ内・非公開。新規 `coreutils.go` は同一パッケージのため直接呼び出せる）:**

- `findFirstSubcommand(args []string) string`（[command_analysis.go:404](../../../internal/runner/base/security/command_analysis.go#L404)）
  — 先頭の非オプション引数を返す。マルチコール入口 `coreutils <util> ...` の実効サブコマンド解決に再利用する（`02_architecture.md` §3.2 ステップ 2）。新規実装は不要。
- `hasSetuidOrSetgidBit(cmdPath string) (bool, error)`（[command_analysis.go:807](../../../internal/runner/base/security/command_analysis.go#L807)）
  — 解決済み絶対パスの setuid/setgid ビットを検査する。`CoreutilsCommandRisk` の setuid 検査（`02_architecture.md` §3.2 ステップ 1）に再利用する。新規実装は不要。

**変更対象の既存コード:**

- [internal/common/secure_path.go:9](../../../internal/common/secure_path.go#L9)
  — 現在 `SecurePathEnv` に coreutils ディレクトリがハードコードで連結された暫定状態（作業ツリー上の未コミット編集）。`CoreutilsDir` 定数を導入し、`SecurePathEnv` をこの定数から構築する。
- [internal/runner/base/security/directory_risk.go:18](../../../internal/runner/base/security/directory_risk.go#L18)
  — `DefaultRiskLevels` に暫定追加された一律 `"/usr/lib/cargo/bin/coreutils": RiskLevelMedium` エントリを削除する。`getDefaultRiskByDirectory` のロジック自体は不変。
- [internal/runner/base/risk/evaluator.go:28](../../../internal/runner/base/risk/evaluator.go#L28)
  — `EvaluateRisk` に coreutils 判定ステップを追加（破壊的操作判定の後、ネットワーク判定の前）。
- [internal/runner/base/security/command_analysis.go:602](../../../internal/runner/base/security/command_analysis.go#L602)
  — `AnalyzeCommandSecurity` のステップ 6（setuid 検査）の後、ステップ 7（Medium パターン照合）の前に coreutils 判定ステップを挿入。関数 docコメントのステップ一覧（[command_analysis.go:575-586](../../../internal/runner/base/security/command_analysis.go#L575-L586)）も更新する。

**コードベース横断調査の結果:**

- Go ソース中の文字列 `/usr/lib/cargo/bin/coreutils` は上記 2 箇所（`secure_path.go`・`directory_risk.go`）のみ（`rg -n "/usr/lib/cargo/bin/coreutils" --type go` で確認済み）。本タスクで両方とも `common.CoreutilsDir` 由来に置き換わる。
- `internal` 配下の `*_test.go` に coreutils ディレクトリを参照するテストケースは現状ゼロ（`02_architecture.md` §3.1 の確認と一致）。よって暫定エントリ削除で失敗する既存テストは存在しない。
- `security` パッケージは既に `internal/common` を import 済み（[validator.go:47](../../../internal/runner/base/security/validator.go#L47) ほか）。`coreutils.go` から `common.CoreutilsDir` を参照しても循環依存は発生しない。
- `common` パッケージにはテストファイルは存在するが `secure_path_test.go` は無い（新規作成する）。

**テスト容易性に関する設計判断（公開シグネチャは不変）:**

`CoreutilsCommandRisk` は setuid 検査のため `os.Stat(resolvedPath)` を実行する。分類ロジックを CI 上で検証するには、対象ファイルが実在する必要があるが、テスト環境に `/usr/lib/cargo/bin/coreutils/<cmd>` は存在しない。そこで `coreutils.go` 内にパッケージ非公開変数 `var coreutilsDir = common.CoreutilsDir` を置き、`CoreutilsCommandRisk` はこの変数を参照する。テストは後述の build タグ付きヘルパー `SetCoreutilsDirForTest` で一時ディレクトリへ差し替える。公開関数のシグネチャ（`02_architecture.md` §3.2）は変更しない。`common.CoreutilsDir` 定数自体は SSOT として不変。

> **重要な制約（全 coreutils テストに共通して適用）:** `coreutilsDir` はパッケージグローバル変数であり、`SetCoreutilsDirForTest` がこれを書き換えるため、本変数を差し替えるテスト（`coreutils_test.go` の各テスト・`evaluator_test.go` の coreutils テスト・`coreutils_consistency_test.go`）は `t.Parallel()` を**使用しない**。差し替えを伴う全テスト関数にこの制約を一律適用する。`security` パッケージと `risk` パッケージは別々のテストバイナリ（別プロセス）として実行されるため両パッケージ間でのグローバル競合は生じないが、`coreutilsDir` の唯一の変更手段を `SetCoreutilsDirForTest`（`t.Cleanup` で必ず復元）に限定し、テスト外からの直接代入は行わない。

---

## 2. 実装ステップ (Implementation Steps)

### フェーズ 0：前提確認（`02_architecture.md` §8 フェーズ 0）

- [x] **ステップ 0-1:** 対象環境で coreutils コマンドのパス解決後 basename が保持されることを確認（Ubuntu 26.04 実機で確認済み、2026-06-11。`02_architecture.md` §1.3）。
- [x] **ステップ 0-2:** Ubuntu 26.04 参照ホストで `ls /usr/lib/cargo/bin/coreutils` を実行し、実在するサブコマンド名一覧を取得する。
  - High 一覧（`rm`/`rmdir`/`unlink`/`shred`/`dd`/`truncate`）・Low 一覧の各エントリが実在するかを照合し、実装する一覧と整合させる。実在しないが防御的に残す名（uutils に未実装のもの）はコード内の英語コメントでその旨を明記する。
  - 注：分類関数は実在しないサブコマンド名に対しても安全側（Low 一覧外は Medium 以上）に動作するため、防御的エントリが残っても安全性は損なわれない。本確認は「Low 一覧に取りこぼした実在コマンドが無いか」を主眼とする。

### フェーズ 1：基盤（定数化・分類関数・ユニットテスト）

**対象ファイル:** `internal/common/secure_path.go`, `internal/runner/base/security/coreutils.go`（新規）, `internal/runner/base/security/test_helpers.go`（既存・追記）

- [x] **ステップ 1-1:** `secure_path.go` に定数 `CoreutilsDir` を追加する。
  - 値：`"/usr/lib/cargo/bin/coreutils"`。docコメントは英語（`02_architecture.md` §3.2 の文面を使用）。
- [x] **ステップ 1-2:** `secure_path.go` の `SecurePathEnv` を `CoreutilsDir` 定数から構築するよう書き換える。
  - 変更後（**到達すべき確定値**）：`SecurePathEnv = "/sbin:/usr/sbin:/bin:/usr/bin:" + CoreutilsDir`。
  - **必達条件:** 連結結果の文字列値は必ず `"/sbin:/usr/sbin:/bin:/usr/bin:/usr/lib/cargo/bin/coreutils"` となり、末尾に coreutils ディレクトリを含むこと。これは F-001（coreutils ディレクトリの安全ディレクトリ判定）の前提であり、coreutils ディレクトリを PATH から外してはならない。本ステップの目的は「coreutils ディレクトリを PATH に含めること」と「そのパス文字列を `CoreutilsDir` 定数 1 箇所に集約しハードコード重複を解消すること」の両方。
  - **着手時のベースラインに関する注意:** コミット済みのブランチ状態では `SecurePathEnv` は coreutils 無しの `"/sbin:/usr/sbin:/bin:/usr/bin"`、作業ツリーの暫定未コミット編集（`02_architecture.md` 付録）では coreutils を直書きで連結済み、という 2 種類の出発点があり得る。いずれから着手しても上記「必達条件」の確定値に到達させる（暫定編集を取り込む場合は直書きを `CoreutilsDir` 連結へ置換、コミット済み状態から着手する場合は coreutils を新たに追加）。
  - 検証：フェーズ 4 の `TestSecurePathEnv_IncludesCoreutilsDir`（`strings.Contains(SecurePathEnv, CoreutilsDir)`）がこの必達条件を担保する。
- [x] **ステップ 1-3:** `coreutils.go`（新規）にパッケージ非公開変数 `var coreutilsDir = common.CoreutilsDir` を定義する。
- [x] **ステップ 1-4:** `coreutils.go` に安全コマンド一覧（Low）と破壊的コマンド一覧（High）を `map[string]struct{}` で定義する（`02_architecture.md` §3.4 の区分方針に従う。集合用途のため `map[string]struct{}` を用いる）。
  - Low 一覧：`mkdir`, `ls`, `cat`, `echo` ほか §3.4 の方針に沿う読み取り・新規作成中心のコマンド。
  - High 一覧：`rm`, `rmdir`, `unlink`, `shred`, `dd`, `truncate`。
- [x] **ステップ 1-5:** `coreutils.go` に `CoreutilsCommandRisk(resolvedPath string, args []string) (runnertypes.RiskLevel, bool, error)` を実装する（`02_architecture.md` §3.2 のシグネチャ・処理順）。
  - ディレクトリ判定：`filepath.Dir(resolvedPath) != coreutilsDir` のとき `(RiskLevelUnknown, false, nil)` を返す（exact match）。
  - setuid 検査：`hasSetuidOrSetgidBit(resolvedPath)` を呼ぶ。エラー時 `(RiskLevelUnknown, false, err)`、ビットありなら `(RiskLevelHigh, true, nil)`。
  - 実効サブコマンド解決：basename が `"coreutils"` のとき `findFirstSubcommand(args)` を実効サブコマンド名とする。それ以外は basename を用いる。
    - **`findFirstSubcommand` の流用に関する前提:** 同関数は git 向けに `-c`/`-C` 等の「値を取るオプション」とその値をスキップする実装だが、uutils のマルチコール構文は `coreutils <util> <util-args>` であり、util 名の前にグローバルオプションを取らない（util 名が必ず先頭の非オプション位置に来る）。したがって先頭非オプションを返す挙動で実効 util 名を正しく得られる。util 名が得られない場合（例：`coreutils --help` のようにオプションのみ）は実効サブコマンドが空文字となり、High/Low いずれの一覧にも該当せず Medium（フェイルセーフ）に落ちる。この前提と挙動はフェーズ 1 のテストで担保する（下記）。
  - 分類：High 一覧該当 → `(RiskLevelHigh, true, nil)`、Low 一覧該当 → `(RiskLevelLow, true, nil)`、それ以外 → `(RiskLevelMedium, true, nil)`。
- [x] **ステップ 1-6:** **既存**の `test_helpers.go`（`//go:build test`・`package security`。既に `IsVariableValueSafe` を持つ）に `SetCoreutilsDirForTest(t *testing.T, dir string)` を**追記**する（新規作成・上書きはしない。既存ヘルパーを破壊しないこと）。
  - 現在の `coreutilsDir` を保存し `dir` を代入、`t.Cleanup` で元の値へ復元する。非公開変数 `coreutilsDir` を書き換えるため Classification B（`test_helpers.go`）に置く（`testutil/` は公開 API のみ扱うため不可）。`risk` パッケージのテストからも呼べるよう公開関数とする。本ヘルパーは `-tags test` 下でのみ可視であり、`risk` パッケージのテストは `make test`（`go test -tags test`）で実行されるため参照可能。
- [x] **ステップ 1-7:** `coreutils_test.go`（新規・`//go:build test`）にユニットテストを追加する（`02_architecture.md` §7.1）。各テストは冒頭で `SetCoreutilsDirForTest(t, tmp)` を呼び `t.TempDir()` 配下に対象ファイルを作成する。`t.Parallel()` は使用しない。
  - [x] `TestCoreutilsCommandRisk_SafeCommands`：`mkdir`/`ls`/`cat`/`echo` → `(Low, true, nil)`（F-003）。
  - [x] `TestCoreutilsCommandRisk_MediumCommands`：`chmod`/`chown`/`env`/`nohup`/`cp`/`mv`、および明らかに coreutils でないセンチネル名 `definitely-not-a-coreutil`（Low/High いずれの一覧にも属さないことを保証し、Medium 既定分岐を確実に通す）→ `(Medium, true, nil)`（F-003 / 非機能 4.1）。
  - [x] `TestCoreutilsCommandRisk_DestructiveCommands`：`rm`/`dd`/`shred`/`truncate` → `(High, true, nil)`（F-004）。
  - [x] `TestCoreutilsCommandRisk_MulticallEntrypoint`：basename `coreutils` で実効サブコマンド解決を検証する（F-004）。
    - `args=["rm","-rf","/tmp/x"]` → `(High, true, nil)`
    - `args=["mkdir","d"]` → `(Low, true, nil)`
    - `args=["--help"]`（オプションのみ、実効 util 名が空）→ `(Medium, true, nil)`（フェイルセーフ既定）
    - `args=[]`（引数なし、実効 util 名が空）→ `(Medium, true, nil)`（フェイルセーフ既定）
  - [x] `TestCoreutilsCommandRisk_Setuid`：`mkdir`（安全名）に setuid ビットを付与 → `(High, true, nil)`。setgid が OS に無視される場合は既存テスト（[command_analysis_test.go:771-779](../../../internal/runner/base/security/command_analysis_test.go#L771-L779)）と同様に `t.Skip`。
  - [x] `TestCoreutilsCommandRisk_NonCoreutilsPath`：`/usr/bin/mkdir`・`/bin/ls`（coreutils 直下でない）→ `(RiskLevelUnknown, false, nil)`（適用範囲限定）。
- [x] **ステップ 1-8:** `make fmt && make test && make lint` を実行し緑であることを確認する。
  - 注: `make test` は緑。`make lint` は security パッケージの既存 goconst 違反（本タスク前から存在）が残存するが、本タスクが新たに導入した goconst 違反はない。

**成功基準:** `coreutils.go` と上記ユニットテストが緑。`go test -tags test ./internal/runner/base/security/...` がパス。

### PR-1 作成ポイント: internal foundation — constant and classification function

**対象ステップ**: 0-2 / 1-1 / 1-2 / 1-3 / 1-4 / 1-5 / 1-6 / 1-7 / 1-8

**推奨タイトル**: `feat(0135): add CoreutilsDir constant and CoreutilsCommandRisk`

**レビュー観点**: `CoreutilsDir` 定数が SSOT として正しく定義されているか / `CoreutilsCommandRisk` の判定順序（setuid → 実効サブコマンド → 分類）が `02_architecture.md` §3.2 と一致しているか / Low/High 一覧の区分方針が §3.4 に従い、フェイルセーフとして Medium 既定になっているか / `SetCoreutilsDirForTest` が既存ヘルパーを壊さず `t.Cleanup` で必ず復元しているか

- [x] グリーンゲート（`make test && make lint`）がパスしていることを確認した
  - 注: `make test` は緑。`make lint` は security パッケージの既存 goconst 違反が残るが、本 PR が導入した違反ではない。
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### フェーズ 2：実行時経路（`EvaluateRisk`）

**対象ファイル:** `internal/runner/base/risk/evaluator.go`, `internal/runner/base/risk/evaluator_test.go`

- [x] **ステップ 2-1:** `evaluator.go` の `EvaluateRisk` に coreutils 判定ステップを追加する（`02_architecture.md` §6.1）。
  - 位置：`IsDestructiveFileOperation` 判定（[evaluator.go:39-41](../../../internal/runner/base/risk/evaluator.go#L39-L41)）の後、`IsNetworkOperation` 呼び出し（[evaluator.go:46](../../../internal/runner/base/risk/evaluator.go#L46)）の前。
  - 処理：`risk, handled, err := security.CoreutilsCommandRisk(cmd.ExpandedCmd, cmd.ExpandedArgs)`。`err != nil` なら `(RiskLevelUnknown, err)` を返す（フェイルクローズ）。`handled` が真なら `(risk, nil)` を返す（バイナリ解析を迂回）。偽なら従来経路を継続する。
- [x] **ステップ 2-2:** `evaluator_test.go`（既存・`//go:build test`）に `TestStandardEvaluator_EvaluateRisk_Coreutils` を追加する（`02_architecture.md` §7.1）。`SetCoreutilsDirForTest(t, tmp)` で一時ディレクトリへ差し替え、`t.TempDir()` 配下に `mkdir`/`rm`/`coreutils` 等のファイルを作成。`t.Parallel()` は使用しない。
  - [x] `ExpandedCmd = <tmp>/mkdir`（setuid なし）→ `Low`（F-002：バイナリ解析を通らない）。
  - [x] `ExpandedCmd = <tmp>/rm` → `High`（F-004）。
  - [x] `ExpandedCmd = <tmp>/coreutils` ＋ `ExpandedArgs=["rm","-rf","/tmp/x"]` → `High`（F-004）。
- [x] **ステップ 2-3:** `make fmt && make test && make lint` を実行し緑であることを確認する（`make lint` は既存 goconst 違反のみ残存。本フェーズが導入した違反はない）。

**成功基準:** coreutils パスが実行時経路でバイナリ解析を迂回し、設計どおりの分類になる。

### フェーズ 3：dry-run 経路と整合

**対象ファイル:** `internal/runner/base/security/directory_risk.go`, `internal/runner/base/security/directory_risk_test.go`, `internal/runner/base/security/command_analysis.go`, `internal/runner/base/security/coreutils_test.go`

- [ ] **ステップ 3-1:** `directory_risk.go` の `DefaultRiskLevels` から暫定エントリ行を削除する。
  - 削除対象：`"/usr/lib/cargo/bin/coreutils": runnertypes.RiskLevelMedium,`（[directory_risk.go:18](../../../internal/runner/base/security/directory_risk.go#L18)）。`getDefaultRiskByDirectory` のロジック・`/bin`・`/usr/bin`・`/sbin` 等の既存エントリは不変。委譲は追加しない（`02_architecture.md` §5.2）。
  - **⚠️ コミット順の制約:** ステップ 3-1（削除）は**必ずステップ 3-3（`AnalyzeCommandSecurity` へのcoreutils 判定ステップ挿入）と同一コミットに含めるか、またはステップ 3-3 を先にコミットする**こと。削除のみを先にコミットすると、coreutils パスの dry-run 分類が `RiskLevelUnknown` になり `make test` が失敗する。
- [ ] **ステップ 3-2:** `directory_risk_test.go` の `TestGetDefaultRiskByDirectory` に coreutils ケースを追加する。
  - `cmdPath = "/usr/lib/cargo/bin/coreutils/mkdir"` → `RiskLevelUnknown`（暫定エントリ削除後、同関数は coreutils を分類しない）。既存ケースは不変。
- [ ] **ステップ 3-3:** `command_analysis.go` の `AnalyzeCommandSecurity` に coreutils 判定ステップを挿入する（`02_architecture.md` §3.1 / §6）。
  - 位置：ステップ 6（setuid 検査、[command_analysis.go:633-642](../../../internal/runner/base/security/command_analysis.go#L633-L642)）の後、ステップ 7（Medium パターン照合、[command_analysis.go:644-647](../../../internal/runner/base/security/command_analysis.go#L644-L647)）の前。
  - 処理：`risk, handled, err := CoreutilsCommandRisk(resolvedPath, args)`。
    - `err != nil` の場合：**dry-run 経路では既存ステップ 6 と同一のエラー扱いとする**。すなわち `(RiskLevelHigh, resolvedPath, fmt.Sprintf("Unable to check setuid/setgid status: %v", err), nil)` を返し、エラーは伝播せず安全側（High）に倒して dry-run の表示を継続する。これにより、stat 失敗時の dry-run 挙動が非 coreutils コマンド（ステップ 6 で High 表示）と coreutils コマンドで一致し、dry-run が異常終了しない。
    - `handled` が真の場合：`(risk, resolvedPath, "Coreutils command risk classification", nil)` を返す。
    - `handled` が偽の場合：従来経路を継続する。
  - 注 1：dry-run 経路はステップ 6（setuid 検査）が本ステップより先に実行されるため、`CoreutilsCommandRisk` 内の stat 失敗はステップ 6 で既に捕捉済みのことが多く、本ステップの `err != nil` 分岐はほぼ到達しない。それでも上記のとおりステップ 6 と同じ扱いにして一貫性を保つ。
  - 注 2：実行時経路（`EvaluateRisk`）の `err != nil` は `02_architecture.md` §4 のとおりフェイルクローズ（`(RiskLevelUnknown, err)` を伝播しコマンドをブロック）であり、dry-run 経路の上記扱いとは意図的に異なる（実行時は不確実なら実行しない、dry-run は表示を止めない）。両者とも「不確実なら最も厳しい結果」である点で F-005 と整合する（High 表示 ⇔ 実行ブロック）。
  - 注 3：setuid バイナリは本ステップ到達前にステップ 6 で既に High 確定するため、本ステップ内の setuid 検査は実質冗長だが、実行時経路（`CoreutilsCommandRisk` 内検査）との一貫性のため関数仕様は変更しない（`02_architecture.md` §5.4）。
- [ ] **ステップ 3-4:** `AnalyzeCommandSecurity` の docコメントのステップ一覧（[command_analysis.go:575-586](../../../internal/runner/base/security/command_analysis.go#L575-L586)）に coreutils 判定ステップを追記する（英語）。
  - 現行の「6. setuid / setgid bit detection」と「7. Medium-risk dangerous command pattern matching」の間に新ステップを挿入し、以降の番号を繰り上げる（挿入後: 旧 7 → 8、旧 8 → 9、旧 9 → 10）。文面は英語で記述する。
  - **注意:** 本計画書中のステップ番号記述（「ステップ 7」「ステップ 9」等）は挿入前の既存コードの番号を指す。挿入後の docコメントでは上記の繰り上げ後の番号を使用すること。
- [ ] **ステップ 3-5:** `coreutils_test.go` に dry-run 経路のテスト `TestAnalyzeCommandSecurity_Coreutils` を追加する。`SetCoreutilsDirForTest(t, tmp)` で差し替え、`hashDir=""`（ハッシュ検証スキップ）で呼ぶ。`t.Parallel()` は使用しない。
  - [ ] `<tmp>/mkdir` → `Low` / `<tmp>/chmod`（`args=["+x","file"]`）→ `Medium` / `<tmp>/cp`（`args=["a","b"]`）→ `Medium` / `<tmp>/rm`（`args=["-rf","/tmp/x"]`）→ `High`。
  - [ ] `<tmp>/coreutils` ＋ `args=["rm","-rf","/tmp/x"]` → `High`。
- [ ] **ステップ 3-6:** `make fmt && make test && make lint` を実行し緑であることを確認する。

**成功基準:** dry-run 経路が coreutils 判定ステップ（挿入後はステップ 7、ディレクトリ既定は旧ステップ 9 → 繰り上げ後ステップ 10）で分類を確定し、ディレクトリ既定フォールバックへ到達しない。

### PR-2 作成ポイント: integration — wire classification into both evaluation paths

> **ブランチの前提:** このブランチは PR-1 マージ後の `main` から作成するか、またはマージ前であれば PR-1 ブランチをベースとして作成する。PR-1 がマージされていない状態で `main` に対してレビューすると `security.CoreutilsCommandRisk` と `security.SetCoreutilsDirForTest` が未定義となりコンパイルエラーになる。

**対象ステップ**: 2-1 / 2-2 / 2-3 / 3-1 / 3-2 / 3-3 / 3-4 / 3-5 / 3-6

**推奨タイトル**: `feat(0135): wire CoreutilsCommandRisk into both evaluation paths`

**レビュー観点**: `EvaluateRisk` への挿入位置（`IsDestructiveFileOperation` の後、`IsNetworkOperation` の前）が §6.1 と一致しているか / `AnalyzeCommandSecurity` への挿入位置（setuid 検査の後、Medium パターン照合の前）が §6 と一致しているか / dry-run 経路の `err != nil` 扱いが既存ステップ 6 と同一の安全側（High 表示・エラー非伝播）になっているか / 暫定エントリ削除後に `getDefaultRiskByDirectory` が coreutils を `RiskLevelUnknown` として返すことを確認するテストがあるか / docコメントのステップ番号が実処理順と一致しているか / **ステップ 3-1（削除）とステップ 3-3（挿入）が同一コミットまたは 3-3 先行コミットで実装されているか**（削除のみ先行コミットすると dry-run の coreutils 分類が壊れ `make test` が失敗する）

- [ ] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### フェーズ 4：検証（F-001 / F-005 の統合・整合確認）

**対象ファイル:** `internal/common/secure_path_test.go`（新規）, `internal/runner/base/risk/coreutils_consistency_test.go`（新規）

- [ ] **ステップ 4-1:** `secure_path_test.go`（新規）に F-001 の検証テスト `TestSecurePathEnv_IncludesCoreutilsDir` を追加する。
  - `strings.Contains(SecurePathEnv, CoreutilsDir)` が真であることを検証（安全ディレクトリ判定が coreutils を含む）。
- [ ] **ステップ 4-2:** `internal/runner/base/risk/coreutils_consistency_test.go`（新規・`//go:build test`）に F-005 の整合テスト `TestCoreutilsRiskConsistency_RuntimeVsDryRun` を追加する。`SetCoreutilsDirForTest(t, tmp)` で差し替え、同一の coreutils コマンド集合に対し実行時経路（`risk.StandardEvaluator.EvaluateRisk`）と dry-run 経路（`security.AnalyzeCommandSecurity`、`hashDir=""`）の最終リスクが一致することを検証する（`02_architecture.md` §5.4 / §7.2）。`t.Parallel()` は使用しない。
  - **配置の根拠:** 本テストは `risk` パッケージを import するため、依存方向（`risk → security` の一方向。`security` は `risk` を import できない）から `risk` パッケージ側に置く。`security` パッケージの `coreutils_test.go` には置けない。
  - 検証対象（各コマンドで両経路の最終リスクが一致することを assert する）：
    - `mkdir`（引数なし）→ 両経路 Low
    - `chmod`（`args=["+x","file"]`）→ 両経路 Medium
    - `cp`（`args=["a","b"]`）→ 両経路 Medium
    - `rm`（`args=["-rf","/tmp/x"]`）→ 両経路 High
    - **`rm`（引数なし）→ 両経路 High**（前段の破壊的判定／高リスクパターンは coreutils フルパス・`-rf` 無しでは発火せず、coreutils 判定ステップの High 一覧のみが High を保証することを検証。F-004 の要となるケース。`02_architecture.md` §5.4）
    - **`shred`（`args=["file"]`）→ 両経路 High**（同上：High 一覧が保証）
    - **`truncate`（`args=["-s","0","file"]`）→ 両経路 High**（同上）
    - **`dd`（引数なし、`if=` 無し）→ 両経路 High**（dry-run の `{"dd","if="}` パターンは `if=` 無しでは不発火、coreutils 判定ステップの High 一覧が保証することを検証）
    - **`unlink`（`args=["x"]`）→ 両経路 High**（同上）
    - マルチコール入口 `coreutils` ＋ `args=["rm","-rf","/tmp/x"]` → 両経路 High
    - setuid 付与 `mkdir` → 両経路 High
  - **このケース集合の狙い:** 前段ステップ（`IsDestructiveFileOperation` はフルパスを basename 化せず照合するため coreutils 解決済みパスでは不発火、dry-run の高リスクパターンは特定の引数形にのみ反応）に依存せず、`CoreutilsCommandRisk` の High 一覧が F-004 を両経路で保証していることを機械的に検証する。
- [ ] **ステップ 4-3:** セキュリティ回帰テストを追加する（`02_architecture.md` §7.3）。`risk` パッケージ側（`evaluator_test.go`）に配置。
  - [ ] 非 coreutils ディレクトリのコマンドが従来どおりの最終リスクを返すこと（適用範囲限定。`<tmp>` 差し替え下で、`<tmp>` 以外のパスに対してリスク結果が coreutils 前後で変わらないことを最終リスク値で確認）。
  - [ ] `sudo` 等の特権昇格が coreutils 判定ステップの前段で Critical のままであること（既存 `TestStandardEvaluator_EvaluateRisk_PrivilegeEscalation` が担保。回帰なきことを確認）。
- [ ] **ステップ 4-4:** `make fmt && make test && make lint` を実行し全体が緑であることを確認する。
- [ ] **ステップ 4-5:** `make deadcode` を実行し本番コード経路に未使用コードがないことを確認する（回帰チェック）。なお `make deadcode` は `cmd/...` を `-tags test` 無しで走査するため、test タグ付きヘルパー（`SetCoreutilsDirForTest`）や test 経路は対象外である。test タグ付きコードの未使用検出はスコープ外とし、`make test` のコンパイル失敗で参照不整合を検出する。

**成功基準:** F-001〜F-005・非機能 4.1 のすべての検証タスクが緑。

### PR-3 作成ポイント: verification — F-001/F-005 integration and security regression tests

> **ブランチの前提:** このブランチは PR-2 マージ後の `main` から作成するか、またはマージ前であれば PR-2 ブランチをベースとして作成する。PR-2 がマージされていない状態では `security.AnalyzeCommandSecurity` の coreutils 判定ステップが存在せず、整合テストが期待どおりに通らない。

**対象ステップ**: 4-1 / 4-2 / 4-3 / 4-4 / 4-5

**推奨タイトル**: `feat(0135): add F-001/F-005 integration and security regression tests`

**レビュー観点**: 整合テスト（`TestCoreutilsRiskConsistency_RuntimeVsDryRun`）が `rm`/`shred`/`dd`/`truncate`/`unlink` の引数なし・最小引数ケースを含み、前段ステップ不発火の経路でも High を保証しているか / `t.Parallel()` が差し替えを伴うすべてのテストで使用されていないか / 非 coreutils パスが従来経路（`handled=false`）にフォールバックすることを確認するテストがあるか / `TestSecurePathEnv_IncludesCoreutilsDir` が F-001 必達条件（`SecurePathEnv` が `CoreutilsDir` を含む）を正しく検証しているか / 整合テストの `AnalyzeCommandSecurity` 呼び出しで `hashDir=""` を渡していること（誤って実パスを渡すとハッシュ検証ステップで意図しない結果になる）/ dry-run 経路の setuid ケースで既存ステップ 6 が coreutils 判定ステップより先に High を返すという不変条件が成立しているか（両経路とも High になることの検証に加え、この順序依存が将来壊れないことを確認）

- [ ] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン (Implementation Order and Milestones)

`02_architecture.md` §8 のフェーズ順に準拠する。

### 3.1 マイルストーン

| マイルストーン | 内容 | 完了条件 |
|---|---|---|
| M0 | 前提確認 | basename 保持の確認済み（実機検証済み） |
| M1 | 基盤（`CoreutilsDir` 定数・`CoreutilsCommandRisk`・ユニットテスト） | フェーズ 1 のチェック全完了 |
| M2 | 実行時経路（`EvaluateRisk`） | フェーズ 2 のチェック全完了 |
| M3 | dry-run 経路と整合（暫定エントリ削除・`AnalyzeCommandSecurity` 挿入） | フェーズ 3 のチェック全完了 |
| M4 | 検証（F-001 / F-005 整合・セキュリティ回帰） | フェーズ 4 のチェック全完了、緑ゲート通過 |

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 |
|---|---|---|
| PR-1 | 0-2 / 1-1 〜 1-8 | `CoreutilsDir` 定数を `internal/common/secure_path.go` に追加、`CoreutilsCommandRisk` を `security/coreutils.go` に新規実装、`SetCoreutilsDirForTest` ヘルパー追記、ユニットテスト追加 |
| PR-2 | 2-1 〜 2-3 / 3-1 〜 3-6 | `EvaluateRisk` に coreutils 判定ステップ追加、`DefaultRiskLevels` 暫定エントリ削除、`AnalyzeCommandSecurity` に coreutils 判定ステップ挿入・docコメント更新、各経路テスト追加 |
| PR-3 | 4-1 〜 4-5 | F-001 検証テスト（`secure_path_test.go`）、F-005 整合テスト（`coreutils_consistency_test.go`）、セキュリティ回帰テスト追加、`make deadcode` 確認 |

---

## 4. テスト戦略 (Test Strategy)

詳細は `02_architecture.md` §7 を参照。本タスクでのテスト方針は以下。

- **ユニットテスト:** `CoreutilsCommandRisk` の全分類（Low/Medium/High）・setuid・マルチコール入口・非 coreutils パスを `coreutils_test.go` で網羅する（フェーズ 1 / PR-1）。
- **経路別テスト:** 実行時（`EvaluateRisk`）と dry-run（`AnalyzeCommandSecurity`）で coreutils コマンドが設計どおり分類されることを各経路で検証する（フェーズ 2 / 3 / PR-2）。
- **整合テスト:** 同一コマンド集合に対し両経路の最終リスクが一致することを検証する（F-005、フェーズ 4 / PR-3）。
- **セキュリティ回帰:** 適用範囲限定（非 coreutils の従来挙動不変）・特権昇格不変を検証する（非機能 4.1、フェーズ 4 / PR-3）。
- **後方互換性:** coreutils 非導入環境（`<tmp>` に該当ファイルを置かない）でも従来経路が機能することは、適用範囲限定テスト（`handled=false` フォールバック）で担保する。
- **既存テストの再利用:** setuid 検査・パターン照合の低レベル挙動は既存テスト（`TestHasSetuidOrSetgidBit_Detailed` 等）が担保済みのため再テストしない（DRY）。

テストヘルパー：**既存**の `internal/runner/base/security/test_helpers.go`（`//go:build test`・`package security`）に `SetCoreutilsDirForTest` を**追記**する（既存の `IsVariableValueSafe` はそのまま）。`testutil/` には追加しない（非公開変数を扱うため。`test_organization.md` Classification B）。

---

## 5. リスク管理 (Risk Management)

| リスク | 影響 | 緩和策 |
|---|---|---|
| グローバル変数差し替えによるテスト間干渉 | テストが非決定的に失敗 | 差し替えを伴う全テストで `t.Parallel()` を禁止し、`t.Cleanup` で必ず復元する（§1.3 の制約）。 |
| 将来 `/usr/bin/<cmd>` がマルチコール本体へのシンボリックリンク化（basename 喪失） | 本設計が機能しなくなる | `02_architecture.md` §1.3 の残存リスクとして記録済み。発生時は解決前コマンド名で分類する代替設計へ切替（別タスク）。 |
| 安全コマンド一覧の取りこぼし | 安全コマンドが Medium に落ちる | 既定 Medium はフェイルセーフ側であり安全性は損なわない。一覧は §4.3 のとおり追加容易な `map` で保持。 |
| dry-run と実行時の前後ステップ差異による不一致 | F-005 違反 | フェーズ 4 の整合テストで具体コマンド集合の一致を機械的に検証する。 |

---

## 6. 実装チェックリスト (Implementation Checklist)

- [ ] PR-1 マージ済み（対象ステップ: 0-2 / 1-1 〜 1-8）
- [ ] PR-2 マージ済み（対象ステップ: 2-1 〜 2-3 / 3-1 〜 3-6）
- [ ] PR-3 マージ済み（対象ステップ: 4-1 〜 4-5）

### クロスサーチ確認（`make lint` / `make test` で検出できない項目のみ）

- [ ] `rg -n "/usr/lib/cargo/bin/coreutils" --type go` の結果が、`secure_path.go` の `CoreutilsDir` 定数定義 1 箇所のみであること（`directory_risk.go` の暫定エントリが削除され、ハードコード重複が解消されたこと）。
- [ ] `AnalyzeCommandSecurity` の docコメントのステップ番号が本文の実処理順と一致していること（手動確認）。

---

## 7. 受け入れ基準の検証 (Acceptance Criteria Verification)

`01_requirements.md` は機能要件を `F-001`〜`F-005` および非機能 4.1 として定義している（個別の `AC-NN` 識別子は未付与）。各機能要件を受け入れ基準とみなし、検証手段を以下に対応づける。検証種別は `test`（実行可能）/ `static`（rg/grep）/ `manual`（PR 観察）。

| 要件 | 内容 | 検証種別 | 検証手段 |
|---|---|---|---|
| F-001 | coreutils ディレクトリの安全ディレクトリ判定 | test | `internal/common/secure_path_test.go::TestSecurePathEnv_IncludesCoreutilsDir`（`SecurePathEnv` が `CoreutilsDir` を含む）。既存 `internal/runner/base/security/types_test.go::TestDefaultConfig_AllowedCommandsConsistency`（`GenerateAllowedCommandsFromPath(common.SecurePathEnv)` 由来の許可パターン）が回帰なきこと。 |
| F-002 | バイナリ解析に依存しないリスク判定 | test | `internal/runner/base/risk/evaluator_test.go::TestStandardEvaluator_EvaluateRisk_Coreutils`（`<tmp>/mkdir` がバイナリ解析を通らず `Low`） |
| F-003 | コマンド別リスク分類（Low / Medium） | test | `internal/runner/base/security/coreutils_test.go::TestCoreutilsCommandRisk_SafeCommands`（Low）, `::TestCoreutilsCommandRisk_MediumCommands`（Medium） |
| F-004 | 破壊的コマンドのリスク維持（High） | test | `internal/runner/base/security/coreutils_test.go::TestCoreutilsCommandRisk_DestructiveCommands`, `::TestCoreutilsCommandRisk_MulticallEntrypoint`; `internal/runner/base/risk/evaluator_test.go::TestStandardEvaluator_EvaluateRisk_Coreutils`（`<tmp>/rm` → High） |
| F-005 | 実行時と dry-run の一貫性 | test | `internal/runner/base/risk/coreutils_consistency_test.go::TestCoreutilsRiskConsistency_RuntimeVsDryRun`（同一コマンド集合で両経路一致。前段ステップに依存せず High 一覧が F-004 を保証するケース（引数なし `rm`/`shred`/`truncate`/`dd`/`unlink`）を含む） |
| 非機能 4.1（適用範囲限定） | 非 coreutils ディレクトリの検出能力が低下しない | test | `internal/runner/base/risk/evaluator_test.go`（非 coreutils コマンドが従来経路で判定されること） |
| 非機能 4.1（特権昇格不変） | `sudo` 等が Critical のまま | test | `internal/runner/base/risk/evaluator_test.go::TestStandardEvaluator_EvaluateRisk_PrivilegeEscalation`（既存・回帰確認） |
| 非機能 4.1（未知コマンドの安全側） | 一覧外コマンドは Medium 以上 | test | `internal/runner/base/security/coreutils_test.go::TestCoreutilsCommandRisk_MediumCommands`（未知名 → Medium） |
| F-003 / 非機能（setuid） | setuid バイナリは安全名でも High | test | `internal/runner/base/security/coreutils_test.go::TestCoreutilsCommandRisk_Setuid` |
| 暫定エントリ削除の整合 | `getDefaultRiskByDirectory` は coreutils を分類しない | test | `internal/runner/base/security/directory_risk_test.go::TestGetDefaultRiskByDirectory`（coreutils → `RiskLevelUnknown`） |
| ハードコード重複の解消 | coreutils パスの定義が定数 1 箇所 | static | `rg -n "/usr/lib/cargo/bin/coreutils" --type go` の出力が `internal/common/secure_path.go` の `CoreutilsDir` 定義 1 行のみ |

---

## 8. 成功基準 (Success Criteria)

- **機能完全性:** §7 の全要件行の検証タスクが緑。
- **品質:** `make test && make lint` が緑。`make deadcode` で未使用コードなし。
- **セキュリティ:** 適用範囲限定・特権昇格不変・setuid High・破壊的コマンド High の各テストが緑（非機能 4.1 / F-004）。
- **ドキュメント整合:** `AnalyzeCommandSecurity` の docコメントのステップ一覧が実処理順と一致。

---

## 9. 次のステップ (Next Steps)

- 本実装計画書を `approved` にしたのち、フェーズ 1 から実装を開始する。
- 実装中はチェックボックスを随時更新する。
- 完了後、`02_architecture.md` §9 に記録した将来拡張（既存処理の basename 正規化・他の単一バイナリツール対応）は別タスクで扱う。
