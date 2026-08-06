# 実装計画書: entrypoints の run-id 検証・特権降格完全化・verify TOCTOU fail-closed化

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-08-05 |
| Review date | 2026-08-05 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- 要件定義書: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計書: [02_architecture.md](02_architecture.md)
- 要件プロセスガイド: [docs/dev/developer_guide/requirements_process.md](../../dev/developer_guide/requirements_process.md)
- テスト構成ガイド: [docs/dev/developer_guide/test_organization.md](../../dev/developer_guide/test_organization.md)

---

## 1. 実装の全体像

### 1.1 目的

3つの独立した欠陥を解消する。設計上の根拠はすべて `02_architecture.md` にあり、本書はその設計を実行可能な作業単位に分解したものである。

- **F-001 / F-002**: `--run-id` の形式検証（受理形式 `^[A-Za-z0-9_-]{1,64}$`、`02_architecture.md` §1.3 D-1）と、ログファイル名構築側の多層防御。
- **F-003**: `runner` の起動時特権降格を実効グループIDまで広げ、いかなる入力処理よりも前に完了させる。
- **F-004**: `verify` の TOCTOU 権限チェックのうち、ハッシュディレクトリとその祖先ディレクトリの違反を fail-closed（exit 3、`02_architecture.md` §1.3 D-2）とする。
- **F-005**: 上記の利用者から見える挙動変化を、利用者向け文書と CHANGELOG に反映する。

### 1.2 実装原則

- 設計判断は `02_architecture.md` を唯一の出典とし、本書では再掲しない。判断が必要な箇所では該当節を参照する。
- Go のコメント・識別子・文字列リテラルはすべて英語で書く。本書の日本語の説明文をそのままコードへ持ち込まない。
- 各 PR の作成前に `make fmt` → `make test` → `make lint` を実行し、すべて成功した状態で PR を作成する（§3.2 の PR 構成、および各 `### PR-N 作成ポイント` を参照）。

### 1.3 既存コード調査結果

実装前に調査した現行コードの状態を、変更が必要な領域ごとに示す。変更不要と確認した領域は末尾にまとめる。

#### `cmd/runner/main.go`

- `main()`（[main.go:93-122](../../../cmd/runner/main.go#L93-L122)）の現行順序は `flag.Parse()` → run ID の既定値設定 → ハッシュディレクトリの絶対パス検査 → `syscall.Seteuid(syscall.Getuid())` である。`syscall.Setegid` の呼び出しは存在しない。`02_architecture.md` §3.2.2 の順序へ並べ替える。
- `runID` の既定値設定は `if runID == "" { runID = logging.GenerateRunID() }`（[main.go:98-100](../../../cmd/runner/main.go#L98-L100)）のインライン記述であり、検証は一切ない。`resolveRunID` へ切り出す。
- `init()`（[main.go:56-91](../../../cmd/runner/main.go#L56-L91)）は全フラグの登録と `groupmembership.SetProcessPermissionCheckUIDPolicy` を行う。`cmd/runner` の製品コードにある `init()` はこの1個だけである。
- `cmd/runner` の製品コードで `syscall` の識別子変更系関数を呼ぶのは [main.go:109](../../../cmd/runner/main.go#L109) の `syscall.Seteuid` のみである（`internal/testutil/identitymutationguard` の追跡対象関数名で全数確認）。

#### `internal/logging`

- `GenerateRunID` は [safeopen.go:56-60](../../../internal/logging/safeopen.go#L56-L60) にある。呼び出し元は製品コードでは [cmd/runner/main.go:99](../../../cmd/runner/main.go#L99) の1箇所のみ。テストからの参照は `internal/logging/safeopen_test.go` の `TestGenerateRunID_Uniqueness`（:122）と `TestGenerateRunID_Format`（:139）の2件。
- run ID の形式を定義する定数・検証関数・エラー値はいずれも存在しない。新規に `runid.go` を作る。
- `ErrorType` 定数は [pre_execution_error.go:26-42](../../../internal/logging/pre_execution_error.go#L26-L42) に9個ある。`ErrorTypeInvalidRunID` は存在しないため追加する。
- `PreExecutionError.Is` は型のみを見て `Type` を見ない（[pre_execution_error.go:63-66](../../../internal/logging/pre_execution_error.go#L63-L66)）。`02_architecture.md` §4.1 のとおり、種別の判別はプロセス外部の出力トークンで行う。
- `RUN_SUMMARY` 行を出力するのは `handleErrorCommon`（[pre_execution_error.go:124](../../../internal/logging/pre_execution_error.go#L124)）の1箇所だけであり、`exit_code=1` が固定で埋め込まれる。**正常終了の経路には `RUN_SUMMARY` を出す実装が存在しない**（`rg -n "RUN_SUMMARY" --type go` で製品コードは同1箇所のみ）。したがって「正常終了時に採用された run ID」をプロセス外部から観測する手段は `RUN_SUMMARY` ではなく、`-log-dir` に生成されるログファイル名（`{hostname}_{timestamp}_{runID}.json`）である。

#### `internal/runner/bootstrap`

- `SetupLoggerWithConfig`（[logger.go:75](../../../internal/runner/bootstrap/logger.go#L75)）は `config.RunID` を検証せずログファイル名 `fmt.Sprintf("%s_%s_%s.json", hostname, timestamp, config.RunID)` へ埋め込む（[logger.go:138](../../../internal/runner/bootstrap/logger.go#L138)）。関数先頭に `logging.ValidateRunID` を追加する。
- `bootstrap` は既に `internal/logging` を import しているため、依存の追加は発生しない。
- `SetupLoggerWithConfig` の呼び出し元は製品コードでは [environment.go:93](../../../internal/runner/bootstrap/environment.go#L93)（`SetupLogging` 内）の1箇所。テストからの呼び出しは `internal/runner/bootstrap/logger_test.go` に7箇所、`internal/runner/bootstrap/environment_test.go` に2箇所、`cmd/runner/integration_logger_test.go` に3箇所。
- 上記テストが渡す `RunID` 値を全数確認した。`logger_test.go` は 15 個の `LoggerConfig` すべてが `RunID` を設定しており（`test-min-001` 〜 `test-slack-exclusion-001`）、値を省略した（空文字列になる）ものはない。`environment_test.go` は `test-propagation-001`・`test-missing-host-001`・`test-run-001` 〜 `test-run-005`・`test-run-error-001`・`test-run-error-002`・`test-run-perm`、`integration_logger_test.go` は `test-run-001` 〜 `test-run-004`・`integration-test-run`・`test-error-003`。いずれも `^[A-Za-z0-9_-]{1,64}$` を満たすため、値の修正は不要である。

#### `cmd/verify/main.go`

- TOCTOU 権限チェックは `run()` 内にインラインで書かれており（[main.go:78-109](../../../cmd/verify/main.go#L78-L109)）、`security.RunTOCTOUPermissionCheck` の戻り値を破棄している。これが M-3 の fail-open である。
- 差し替え用のパッケージ変数は `validatorFactory`・`mkdirAll`・`ensurePermissionCheckUID` の3つ（[main.go:24-29](../../../cmd/verify/main.go#L24-L29)）。`toctouChecker` は存在しないため追加する。
- 終了コードは `run()` と `processFiles` に散らばる裸の `return 0` / `return 1` である。定数は存在しない。
- `cmd/record/main.go` の `checkDirPermissions`（[main.go:93-156](../../../cmd/record/main.go#L93-L156)）が fail-closed 側の先例であり、違反ごとの ERROR ログ・是正方法の組み立て・標準エラー出力メッセージの形をここから写す。

#### `cmd/verify/main_test.go`

- `run()` を呼び、かつ `checkDirPermissions` に到達するテストは5件ある。`TestRunProcessesMultipleFiles`（:57）、`TestRunReportsFailuresAndContinues`（:77）、`TestRunWarnsWhenDeprecatedFlagUsed`（:97）、`TestRunUsesDefaultHashDirectoryWhenNotSpecified`（:129）、`TestRunTOCTOU_ContinuesOnWorldWritableDir`（:155）。
- `TestRunRequiresAtLeastOneFile`（:47）は `parseArgs` で、`TestRunFailsClosedWhenPermissionCheckUIDUnresolvable`（:188）は `ensurePermissionCheckUID` で、それぞれ `checkDirPermissions` より前に return するため対象外。`TestParseArgsInvalidHashDir`（:114）と `TestVerifyDeclaresSudoUIDAwarePolicy`（:216）は `run()` を呼ばない。
- 差し替え用の共通ヘルパーは `overrideValidatorFactory`（:35）のみで、`DirectoryPermChecker` のスタブ型は `cmd/verify` に存在しない（`cmd/record/main_test.go` の `fakeDirPermChecker` は別パッケージのため再利用できない）。
- `TestRunTOCTOU_ContinuesOnWorldWritableDir` の関数コメント「verify does NOT abort on TOCTOU violations — it only logs a warning」と `AC-M2S-7` への参照は、変更後は誤りになる（`02_architecture.md` §3.6）。

#### `internal/testutil/identitymutationguard`

- `CallSite` は `{FuncName, SyscallName, CallExpr string}` で位置情報を持たない（[helpers.go:68-72](../../../internal/testutil/identitymutationguard/helpers.go#L68-L72)）。呼び出し順序の判定に位置情報が要る。
- `isTrackedImportPath`（[helpers.go:63](../../../internal/testutil/identitymutationguard/helpers.go#L63)）は import パスを `syscall` / `unix` に限定しており、`flag.Parse` を追跡対象にできない。
- 既存の利用者は `internal/runner/resource/identity_mutation_guard_test.go` と `internal/runner/base/risktypes/identity_mutation_guard_test.go` の2件。いずれも `FindRefs` / `RefsInSource` / `CallSitesInSource` を呼び、`CallSite` の `FuncName`・`SyscallName`・`CallExpr` のみを読む。構造体へのフィールド追加と新規関数の追加は、この2件の主張に影響しない。
- 製品ファイルの列挙ロジック（`_test.go` の除外と `//go:build test` 制約の判定）は `FindRefs` の内部に閉じており、外部から再利用できない。`init()` 個数の検査に必要なため、公開の列挙関数を追加する。

#### `security.CollectTOCTOUCheckDirs` / `RunTOCTOUPermissionCheck`

- `CollectTOCTOUCheckDirs(verifyFilePaths, commandPaths []string, hashDir string) []string`（[toctou.go:33](../../../internal/security/toctou.go#L33)）は、`hashDir` については祖先ディレクトリも含めて収集する。引数を変えて2回呼ぶだけでハッシュディレクトリ集合と対象ファイル集合を分けられるため、この関数自体の変更は不要である（`02_architecture.md` §3.4）。
- `RunTOCTOUPermissionCheck`（[toctou.go:82](../../../internal/security/toctou.go#L82)）は存在しないディレクトリを読み飛ばし、違反を WARN で記録して `[]TOCTOUViolation` を返す。この挙動は変更しない。
- `validateDirectoryComponentPermissions`（[dir_permissions_unix.go:141](../../../internal/security/dir_permissions_unix.go#L141)）は、sticky ビットの立った world-writable ディレクトリを違反と見なさない。したがって Linux の `/tmp` 配下や macOS の `/var/folders` 配下に作られる `t.TempDir()` をハッシュディレクトリとして渡す既存テストは、fail-closed 化後も違反にならない。ただしこれはホストのファイルシステム構成に依存する結論であるため、`02_architecture.md` §7.1 の方針に従い `toctouChecker` のスタブ注入で依存を断つ。

#### 利用者向け文書

- `docs/user/runner_command.ja.md` の `-run-id` 節（894〜956行目）は受理形式に触れていない。`<id>` の説明は「実行を識別する一意な文字列（推奨：ULID形式）」のみである。
- 同文書 849 行目のログファイル命名規則は `<log-dir>/runner-<run-id>.json` と書かれているが、実装は `{hostname}_{timestamp}_{runID}.json` である（[logger.go:138](../../../internal/runner/bootstrap/logger.go#L138)）。文書が誤っている。誤った形式は同文書内に3箇所ある（849行目の規則、854行目付近の例、1660行目付近の `cat` コマンド例）。英語版 `docs/user/runner_command.md` にも対応する3箇所（854・859・1695行目付近）がある。
- `docs/user/verify_command.ja.md` には終了コードを扱う節が2つ既にある。93行目の「2.4 終了コードによる判定」と 630行目の「終了コードを使用した適切なエラーハンドリング」であり、後者は出力の `grep` でエラー種別を分岐している。exit 3 の導入はこの2節にも波及する。
- 同文書 955 行目の「同じRun IDを複数回使用すると、ログファイルが上書きされる可能性があります」は `02_architecture.md` §5.3 の残存リスクに対応する注意書きであり、維持する。
- `docs/user/verify_command.ja.md` には終了コードの一覧表が存在しない。判定例は 97 行目の `if verify ...; then` と 641〜642 行目の `EXIT_CODE=${PIPESTATUS[0]}` のみである。
- `CHANGELOG.ja.md` / `CHANGELOG.md` の `## [未リリース]` 節は現在「なし」（`None`）である。

#### 変更不要と確認した領域

- `internal/security` の TOCTOU 関連コード（要件のスコープ外）。
- `cmd/record` の権限チェック（既に fail-closed）。
- `security.RunTOCTOUPermissionCheck` の他の呼び出し元3件（`02_architecture.md` §3.5 の表 #1〜#3）。
- `internal/runner/bootstrap/environment_test.go`・`cmd/runner/integration_logger_test.go`・`internal/runner/bootstrap/logger_test.go` の既存 `RunID` 値（全数が新形式を満たす）。

---

## 2. 実装ステップ

### Phase 1: run ID の形式定義（`internal/logging`）

**対象ファイル**: `internal/logging/runid.go`（新規）、`internal/logging/runid_test.go`（新規）、`internal/logging/safeopen.go`、`internal/logging/safeopen_test.go`、`internal/logging/pre_execution_error.go`

**作業内容**

- [x] `internal/logging/runid.go` を新規作成し、`MaxRunIDLength`・`RunIDFormatDescription`・`ErrInvalidRunID`・`ValidateRunID` を定義する。シグネチャとドキュメントコメントの要件は `02_architecture.md` §3.1 に従う。
- [x] `RunIDFormatDescription` を `MaxRunIDLength` から導出する（`fmt.Sprintf("1-%d characters, each of A-Z a-z 0-9 '_' '-'", MaxRunIDLength)` を非公開の `var runIDFormatDescription` で保持し、公開 API は値を返す関数 `RunIDFormatDescription() string` とする。可変なパッケージ変数を公開しないための実装上の判断であり、PR-1 で確定した）。定数どうしに数値を二重に書かないことで、上限値を変えたときに説明文だけが古いまま残るのを防ぐ。
- [x] `ValidateRunID` を許可リスト方式で実装する。長さ 0 と `MaxRunIDLength` 超過を拒否し、`A-Z` `a-z` `0-9` `_` `-` 以外のバイトを1つでも含む値を拒否する。
- [x] `ValidateRunID` が返すエラーに、最初に違反したバイトの位置（0 始まりのインデックス）とそのバイトの `%q` 表現のみを含め、入力値全体は含めない。エラーは `ErrInvalidRunID` をラップする。
- [x] `GenerateRunID` を `internal/logging/safeopen.go`（56〜60行目）から `internal/logging/runid.go` へ移設する。実装は変更せず、ドキュメントコメントに「出力は常に `ValidateRunID` を満たす」旨を追記する。
- [x] `internal/logging/pre_execution_error.go` の `ErrorType` 定数群に `ErrorTypeInvalidRunID ErrorType = "invalid_run_id"` を追加する（`02_architecture.md` §4.1）。加えて `TestErrorTypeInvalidRunID_Token` でトークン文字列を固定する。

**テスト**

- [x] `internal/logging/runid_test.go` に `TestValidateRunID_AcceptsAllowedCharacters` を追加し、大文字・小文字・数字・`_`・`-` のみからなる値（`my-custom-run-001`、`gh-12345678`、`A_b-9` など）が受理されることを検証する。ULID の受理は `TestGenerateRunID_SatisfiesValidateRunID` が担当するため、ここには含めない。
- [x] `TestValidateRunID_LengthBoundaries` を追加し、長さ 1（受理）・`MaxRunIDLength`（受理）・`MaxRunIDLength+1`（拒否）・0（拒否）を検証する。境界値はリテラルではなく `MaxRunIDLength` から算出し、上限値の変更にテストが追随するようにする。
- [x] `TestValidateRunID_RejectsNonAllowlistedValues` を追加し、`../../etc/cron.d/evil`、`/tmp/evil`、`..`、`a.b`、`a b`、実際の改行を含む値、NUL（`"a\x00b"`）、ESC（`"a\x1bb"`）、マルチバイト文字（`"ラン"`）、`%` を含む値をすべて拒否し、返るエラーが `errors.Is(err, ErrInvalidRunID)` を満たすことを検証する。
- [x] `TestValidateRunID_ErrorOmitsRejectedValue` を追加し、上記の各拒否ケースについて、返るエラーの文字列が入力値全体を部分文字列として含まないことを検証する。入力値は 2 文字以上とし、違反バイト1個の `%q` 表現がそのまま入力値全体と一致しないようにする。
- [x] `TestValidateRunID_ErrorIdentifiesFirstViolatingByte` を追加し、エラー文字列が最初に違反したバイトの位置（`index N`）とそのバイトの `%q` 表現（例: `"/"`・`"\x00"`）を含むことを検証する。これは §3.1 の診断契約（違反バイトの位置と `%q` 表現のみ、入力値全体は含めない）のうち「含む」側を直接検証する唯一のテストである。
- [x] `TestRunIDFormatDescription_ReflectsMaxRunIDLength` を追加し、`strings.Contains(RunIDFormatDescription, strconv.Itoa(MaxRunIDLength))` が真であることを検証する。`RunIDFormatDescription` は `MaxRunIDLength` から導出されるため、上限値だけを変えて説明文が古いまま残る状態をこのテストが検出する。
- [x] `TestGenerateRunID_Uniqueness`（`internal/logging/safeopen_test.go:122`）を `internal/logging/runid_test.go` へ移設する。アサーションは変更しない。
- [x] `TestGenerateRunID_Format`（`internal/logging/safeopen_test.go:139`）を `internal/logging/runid_test.go` へ移設する。アサーションは変更しない。
- [x] `TestGenerateRunID_SatisfiesValidateRunID` を追加し、`GenerateRunID()` の出力が `ValidateRunID` を通過することを検証する。

**完了条件**

- [x] `go test -tags test ./internal/logging/...` が成功する。
- [x] `rg -n "func GenerateRunID" internal/logging/safeopen.go` が 0 件、`rg -n "func GenerateRunID" internal/logging/runid.go` が 1 件である。

### PR-1 作成ポイント: run ID format primitives

**対象ステップ**: Phase 1

**推奨タイトル**: `feat(0162): add run ID format validation to internal/logging`

**レビュー観点**: `ValidateRunID` が許可リスト方式であり、返すエラーに違反バイトの位置と `%q` 表現だけを含めて入力値全体を漏らさないか / `RunIDFormatDescription` が `MaxRunIDLength` から導出され、`TestRunIDFormatDescription_ReflectsMaxRunIDLength` が両者の乖離を検出できるか / `GenerateRunID` の移設で実装が変わっておらず、`safeopen.go` と `safeopen_test.go` に残存参照がないか / 長さ境界テストがリテラルではなく `MaxRunIDLength` から算出されているか

**実装モデル要件**: standard

**判定理由**: 該当するトリガーなし。定数・センチネルエラー・検証関数の追加と既存関数の同一パッケージ内移設に留まり、§1.3 に競合する実装案の記載はなく、統合テスト・CI・外部資源の面もセキュリティゲートの引き上げも含まない（この時点では検証はどの経路からも呼ばれない）。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 2: `identitymutationguard` の拡張

**対象ファイル**: `internal/testutil/identitymutationguard/helpers.go`

`02_architecture.md` §7.2 が要求する2つの拡張（位置情報、追跡対象の呼び出し側指定）に、Phase 3-A のガードテストが必要とする3点（呼び出し元ファイルの識別、同一パッケージの非修飾呼び出しの追跡、製品ファイルの列挙）を加えた計5点を実装する。追加の3点は `02_architecture.md` §7.2 には明記されていないが、同節の主張1〜4を成立させるために不可欠であるため、本計画の判断として Phase 2 の作業に含める。既存の2つのガードテスト（`internal/runner/resource/identity_mutation_guard_test.go`、`internal/runner/base/risktypes/identity_mutation_guard_test.go`）は無変更で通過させる。

**作業内容**

- [x] `CallSite` に呼び出し位置を表すフィールド `Pos token.Pos` を追加し、`scanner.visit` の `*ast.CallExpr` 分岐で呼び出し式の位置を格納する。既存の3フィールドは変更しない。
- [x] `CallSite` に呼び出し元ファイルを表すフィールド `File string` を追加し、解析中のファイル名を格納する。`RefsInSource` は1ファイルごとに `token.NewFileSet()` を作る実装であるため（[helpers.go:198](../../../internal/testutil/identitymutationguard/helpers.go#L198)）、`FindRefs` が複数ファイルを走査した結果の `Pos` はファイルをまたぐと基準が異なり、比較しても意味を持たない。`File` を持たせることで、利用者側が「同一ファイル内の比較であること」を確認してから `Pos` を比較できるようにする。この制約を `CallSite.Pos` のドキュメントコメントに英語で明記する。
- [x] 追跡対象を呼び出し側から追加指定するための型 `ExtraTrackedFunc{ImportPath, FuncName string}` と、`Options{Extra []ExtraTrackedFunc}` を追加する。
- [x] `RefsInSourceWithOptions(t *testing.T, filename, src string, opts Options) ([]CallSite, []ValueRef)` と `FindRefsWithOptions(t *testing.T, dir string, opts Options) ([]CallSite, []ValueRef)` を追加する。`RefsInSource` と `FindRefs` は空の `Options` を渡す薄いラッパーとして残す。
- [x] `scanner.trackedSelector` を、既存の `isTrackedImportPath` × `FuncNames` の判定に加えて `Options.Extra` の (import パス, 関数名) の完全一致でも真になるよう拡張する。追加指定した関数は `CallSite.SyscallName` にその関数名を入れる。
- [x] `ExtraTrackedFunc.ImportPath` が空文字列の場合は、`*ast.SelectorExpr` ではなく**非修飾の `*ast.Ident`** の呼び出し（同一パッケージの関数呼び出し）に一致させる分岐を `scanner.visit` に追加する。Phase 3-A の主張2 が追跡する `dropStartupPrivileges` は `cmd/runner` 内の関数であり、現行の `trackedSelector` は `*ast.SelectorExpr` でない式に対して即座に false を返すため（[helpers.go:258-261](../../../internal/testutil/identitymutationguard/helpers.go#L258-L261)）、この分岐がないと主張2 が成立しない。非修飾識別子については `ValueRef` の検出は行わない（同一パッケージ内では値参照の検出が過剰検知になるため）。
- [x] 製品 `.go` ファイルのパスを列挙する `ProductionGoFiles(t *testing.T, dir string) []string` を追加する。除外条件は `FindRefs` と同一（ディレクトリ、非 `.go`、`_test.go`、`//go:build` が `test` タグを積極的に要求するファイル）とし、`FindRefs` 側もこの関数を使うよう書き換えて判定ロジックを1箇所に保つ。

**テスト**

- [x] `internal/runner/resource/identity_mutation_guard_test.go` と `internal/runner/base/risktypes/identity_mutation_guard_test.go` が無変更で通過することを確認する（後方互換の確認。新規テストは追加しない）。
- [x] 拡張そのものの検証は Phase 3-A のガードテストが利用者として行う。`identitymutationguard` 自身には専用のテストファイルを追加しない（既存パッケージにもテストファイルはなく、利用者側のコントロールケースで検証する方式を踏襲する）。

**完了条件**

- [x] `go test -tags test ./internal/testutil/... ./internal/runner/resource/... ./internal/runner/base/risktypes/...` が成功する。
- [x] `internal/runner/resource/identity_mutation_guard_test.go` と `internal/runner/base/risktypes/identity_mutation_guard_test.go` の差分が空である（`git diff --stat` で確認）。

---

### Phase 3: `cmd/runner` の起動順序変更と `--run-id` 検証

**前提**: Phase 1（`logging.ValidateRunID`）と Phase 2（ガード拡張）が完了していること。

**対象ファイル**: `cmd/runner/main.go`、`cmd/runner/main_test.go`、`cmd/runner/startup_privilege_test.go`（新規）、`cmd/runner/startup_order_guard_test.go`（新規）、`cmd/runner/integration_pre_execution_error_test.go`、`cmd/runner/integration_logger_test.go`

本フェーズは要件上の関心事が2つ（F-003 の特権降格と F-001 の run ID 検証）あり、両者は `main()` の同一の書き換え箇所を触る以外に共有部分を持たない。そこで Phase 3-A（特権降格）と Phase 3-B（run ID 検証）に分け、別々の PR としてレビューする。Phase 3-A は Phase 2 と同じ PR-2 に、Phase 3-B は PR-3 に入る（§3.2）。3-A だけを適用した状態でもグリーンゲートを通せるよう、3-A では `main()` の run ID 既定値設定を現行のまま残し、3-B でこれを `resolveRunID` へ置き換える。

#### Phase 3-A: 起動時特権降格の完全化と実行順序の是正

**作業内容（製品コード）**

- [x] `cmd/runner/main.go` に `startupPrivilegeStage` 型と定数 `stageSetegid` / `stageSeteuid`、`startupPrivilegeError` 型を追加する（`02_architecture.md` §3.2.1）。
- [x] `dropStartupPrivileges(targetUID, targetGID int) error` を追加する。`syscall.Setegid(targetGID)` を先に、成功した場合のみ `syscall.Seteuid(targetUID)` を呼ぶ。いずれかが失敗したら該当 `Stage` を持つ `*startupPrivilegeError` を返し、後続の処理は行わない。
- [x] `reportStartupPrivilegeFailure(err error) int` を追加する。`logging.GenerateRunID()` で run ID を生成し、`logging.HandlePreExecutionError(logging.ErrorTypePrivilegeDrop, <失敗段階を含むメッセージ>, "main", <生成した run ID>)` を呼び、終了コード 1 を返す。
- [x] `main()` の先頭を `02_architecture.md` §3.2.2 の順序へ書き換える。(1) `dropStartupPrivileges(syscall.Getuid(), syscall.Getgid())`、失敗時は `os.Exit(reportStartupPrivilegeFailure(err))`。(2) `flag.Parse()`。(3) run ID の既定値設定（本フェーズでは現行の `if runID == "" { runID = logging.GenerateRunID() }` のまま残す。Phase 3-B で置き換える）。(4) ハッシュディレクトリの絶対パス検査。
- [x] `main()` から `syscall.Seteuid` の直接呼び出し（109〜112行目）を削除する。ハッシュディレクトリ検査と `mainWithExitCode` 以降は現行のまま残す。

**テスト**

- [x] `cmd/runner/startup_privilege_test.go`（新規）に `TestDropStartupPrivileges_FailsClosedOnSetegidFailure` を追加する。`syscall.Geteuid() == 0` のとき `t.Skip` する。`dropStartupPrivileges(os.Getuid(), 0)` を呼び、返るエラーが `*startupPrivilegeError` で `Stage == stageSetegid` であること、呼び出し前後で `syscall.Geteuid()` と `syscall.Getegid()` の**両方**が変化していないこと（`Setegid` が失敗し、`Seteuid` へ進んでいないこと）を検証する。
- [x] 同ファイルに `TestDropStartupPrivileges_FailsClosedOnSeteuidFailure` を追加する。`syscall.Geteuid() == 0` のとき `t.Skip` する。`dropStartupPrivileges(0, os.Getgid())` を呼び、`Setegid` は成功したうえで `Seteuid(0)` が `EPERM` で失敗し、`Stage == stageSeteuid` のエラーが返ること、`syscall.Geteuid()` が変化していないことを検証する。
- [x] 同ファイルに `TestDropStartupPrivileges_SucceedsForCurrentIdentity` を追加し、`dropStartupPrivileges(syscall.Getuid(), syscall.Getgid())` が `nil` を返し、**かつ** 呼び出し後の `syscall.Getegid()` が `syscall.Getgid()` と、`syscall.Geteuid()` が `syscall.Getuid()` と一致することを検証する。
  - 実装時の訂正: 本テストは「常に `nil` を返す空実装」を落とせない。テストバイナリは setuid/setgid で起動されないため、呼び出し前から実効IDと実ID は一致しており、実効グループIDが実際に動いたことを非特権プロセスから観測する手段がない（非特権プロセスは補助グループへ `setegid` できないため、観測可能な降格先も存在しない）。空実装を落とすのは同ファイルの失敗経路テスト2件であり、`return nil` の実装は両方で失敗する。本テストは製品コードの呼び出し形（戻り値 `nil` と呼び出し後の識別子の整合）を固定する位置づけとし、この限界をテストのコメントに英語で明記する。
- [x] 同ファイルに `TestReportStartupPrivilegeFailure_UsesValidRunID` を追加する。合成した `*startupPrivilegeError` を渡し、戻り値が非0であること、標準出力に出る `RUN_SUMMARY` 行の `run_id` フィールドの値が空でなく `logging.ValidateRunID` を通過すること、標準エラー出力に `privilege_drop_failed` と失敗段階（`setegid`）が現れることを検証する。標準出力・標準エラー出力は `os.Pipe` で差し替え、`t.Cleanup` で元の `*os.File` へ戻したうえでパイプの読み書き両端を `Close` する。
- [x] `cmd/runner/startup_privilege_test.go` の全テストに `t.Parallel()` を付けない。プロセスの実効ID・`os.Stdout`・`os.Stderr` というプロセス全体の状態を書き換えるため、同パッケージのフラグ操作系テスト（`setupTestFlags` を使うもの）と並行実行してはならない。この理由をファイル冒頭のコメントに英語で記す。
- [x] `TestDropStartupPrivileges_FailsClosedOn*` の2件は実効ユーザーIDが 0 のとき `t.Skip` するため、root で `make test` を実行すると AC-15・AC-16 の検証が消える。GitHub Actions の `ubuntu-latest` ランナーは非 root であり CI では実行されることを、同ファイルのコメントに英語で記録する（CI 構成が変わったときに検証が黙って失われないようにするため）。
- [x] `cmd/runner/startup_order_guard_test.go`（新規、`//go:build test` を付ける。`identitymutationguard` 自身が `//go:build test` であり、既存の2つのガードテストも同じタグを持つため）に `TestStartupPrivilegeDropOrder` を追加し、`02_architecture.md` §7.2 の主張1〜4を検証する。
  - 主張1: `identitymutationguard.FindRefsWithOptions` の結果から `FuncName == "dropStartupPrivileges"` の `CallSite` を抽出し、`SyscallName == "Setegid"` の `Pos` が `SyscallName == "Seteuid"` の `Pos` より小さいことを検証する。両方が1件ずつ存在することを `require` で確認し、走査対象の取り違えによる空振り成功を防ぐ。`Pos` を比較する前に、両者の `File` が一致することを `require` で確認する（Phase 2 の `File` フィールドの制約）。
  - 主張2: `Options.Extra` に `{ImportPath: "flag", FuncName: "Parse"}` と `{ImportPath: "", FuncName: "dropStartupPrivileges"}` を指定して再走査し、`FuncName == "main"` の `CallSite` のうち `dropStartupPrivileges` の呼び出し位置が `flag.Parse` の呼び出し位置より小さいことを検証する。両方の呼び出しが1件ずつ存在すること、および両者の `File` が一致することを `require` で確認してから `Pos` を比較する。
  - 主張2b（実装時に追加）: `main.go` を `go/parser` で解析し、`main` の本体の最初の文が `dropStartupPrivileges` の呼び出しを含むことを検証する。主張2 だけでは、降格より上に入力を読む文が差し込まれても検出できず、`02_architecture.md` §3.2.2 が保証すると述べる内容に届かない。
  - 主張3: `identitymutationguard.FindRefs(t, ".")` の結果に含まれる `CallSite` が `dropStartupPrivileges` 内の `Setegid` と `Seteuid` の2件だけであり、`ValueRef` が0件であることを検証する。
  - 主張4: `identitymutationguard.ProductionGoFiles(t, ".")` の各ファイルを `go/parser` で解析し、`init` という名前の `*ast.FuncDecl` の総数が 1 であることを検証する。
  - コントロールケース: 主張1・2の走査が関数本体を対象にしていることを、`RefsInSourceWithOptions` に合成ソース（`main` 本体に `flag.Parse()` と `dropStartupPrivileges(...)` を意図した順序と逆順で並べたもの）を渡して、順序判定が実際に失敗側へ倒れることで確認する。
  - 主張1〜4 とコントロールケースは、いずれも Phase 3-A を適用した時点で成立する（主張2 が見る `flag.Parse` との前後関係は 3-A の `main()` で確定し、3-B の変更では動かない）。したがって AC-14 の検証は PR-2 の中で完結させ、後続の PR へ持ち越さない。

**完了条件（Phase 3-A）**

- [x] `go test -tags test ./cmd/runner/...` が成功する。
- [x] 既存の `TestShortFlags`・`TestShortFlagsEquivalence`（`cmd/runner/main_test.go`）がアサーション無変更で通過する。

### PR-2 作成ポイント: startup privilege drop ordering

**対象ステップ**: Phase 2 / Phase 3-A

**推奨タイトル**: `feat(0162): drop startup privileges before flag parsing`

**レビュー観点**: `dropStartupPrivileges` が `Setegid` 成功時にのみ `Seteuid` へ進み、失敗時に実効ユーザーID・実効グループIDのいずれも変化しないことがテストで観測されているか / ガードテストの主張1・2が `File` の一致を `require` してから `Pos` を比較しており、コントロールケースが実際に失敗側へ倒れて空振り成功でないこと（AC-14 の検証を後続 PR へ持ち越していないこと） / `identitymutationguard` の追加が既存2つのガードテストの差分を空に保ち、追加した API が本 PR のガードテストからすべて使われていること / `main()` の run ID 既定値設定が現行のまま残され、本 PR が特権降格以外の挙動を変えていないこと

**実装モデル要件**: frontier-required

**判定理由**: Phase 3-A は起動時特権降格というセキュリティゲート段階の変更（`mkplan.md` step 8 のパネル発動条件）に当たり、順序を誤っても正常系には痕跡が残らないため実行時テストで担保できず、AST 静的検証に依存する（`02_architecture.md` §7.2）。加えて Phase 2 の「`ImportPath` が空のとき非修飾 `*ast.Ident` に一致させる」拡張は前例のない設計判断であり、S-3 が AST 走査の試行錯誤リスクを挙げている。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#993](https://github.com/isseis/go-safe-cmd-runner/pull/993)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### Phase 3-B: `--run-id` の入口検証

**作業内容（製品コード）**

- [x] `resolveRunID(flagValue, bootstrapID string) (string, error)` を追加する。`flagValue` が空文字列なら `bootstrapID` を返し、それ以外は `logging.ValidateRunID(flagValue)` に合格した場合のみ `flagValue` を返す。不合格時は `logging.ErrInvalidRunID` をラップしたエラーを返す（`02_architecture.md` §3.2.3）。
- [x] `main()` を `02_architecture.md` §3.2.2 の最終形へ書き換える。Phase 3-A で確定した (1) `dropStartupPrivileges` と (2) `flag.Parse()` の間に `bootstrapID := logging.GenerateRunID()` を挿入し、`flag.Parse()` の直後に `resolveRunID(runID, bootstrapID)` を置く。ハッシュディレクトリの絶対パス検査はその後に残す。
- [x] `resolveRunID` が返した run ID を、パッケージ変数 `runID`（`cmd/runner/main.go:49`）へ代入する。`mainWithExitCode(runID)` 以降はこの変数を読むため、ローカル変数に留めるとフラグの生値が下流へ流れる。
  - 実装時の強化: `-run-id` の登録先を `runID` から新設のパッケージ変数 `runIDFlag` へ変更し、`runID` には `resolveRunID` の戻り値だけを代入する。元の設計では `flag.Parse()` から `runID = resolvedRunID` までの区間と拒否経路で `runID` が未検証の生値を保持し続けるため、拒否ブロックへ `HandlePreExecutionError(..., runID)`（同ファイルの他の呼び出し箇所と同じ形）を書き足すだけで漏洩が再発しうる。登録先を分けることで `runID` は構成上つねに検証済みとなる。`cmd/runner/main_test.go` の `setupTestFlags` も同じ変数へ揃える。
- [x] `resolveRunID` が失敗した場合、`logging.HandlePreExecutionError` に渡す run ID を **`bootstrapID`** とし、メッセージには `err` の文字列と `logging.RunIDFormatDescription()` を含め、フラグに渡された値そのものは含めない（`02_architecture.md` §1.1 P-2、AC-09・AC-10）。その後 `os.Exit(1)` する。
- [x] `main()` から既存の `if runID == "" { runID = logging.GenerateRunID() }`（98〜100行目。Phase 3-A では残したもの）を削除する。
- [x] `-run-id` フラグの説明文字列を、受理形式が読み取れる内容へ更新する。`cmd/runner/main.go` の `init()` 内の登録を、`"unique identifier for this execution run (auto-generates ULID if not provided)"` から `"unique identifier for this execution run (" + logging.RunIDFormatDescription() + "; auto-generates ULID if not provided)"` へ変更する。
- [x] `cmd/runner/main_test.go` の `setupTestFlags`（22行目〜）にある `-run-id` の登録文字列を、上記と同一の値へ揃える（同関数は `init()` のフラグ定義を写しているため）。

**テスト**

- [x] `cmd/runner/main_test.go` に `TestResolveRunID` を追加する。テーブルで次の4分岐を検証する。(a) `flagValue` 未指定（空文字列）→ `bootstrapID` が返る、(b) `--run-id=""` 相当の明示的な空文字列 → `bootstrapID` が返る、(c) 受理形式の値 `my-custom-run-001` → その値が返る、(d) 拒否形式の値 `../evil` → エラーが返り `errors.Is(err, logging.ErrInvalidRunID)` が真、返る run ID は空文字列。
  - 実装時の訂正: (b) をテーブルに入れると (a) と入力・期待値が完全に同一の重複ケースになり、検証価値がない。代わりに (b) は独立したサブテスト `explicit empty string` とし、`setupTestFlags` + `os.Args = []string{"runner", "-run-id="}` + `flag.Parse()` を経由して `runIDFlag` の値を得てから `resolveRunID` へ渡す。これにより「明示的な空文字列に対して `flag` パッケージが何を返すか」を実際に検証する。
- [x] `cmd/runner/integration_pre_execution_error_test.go` に `TestE2E_PreExecutionError_InvalidRunIDPathTraversal` を追加する。`go run . -config <有効な設定> -dry-run -log-dir <一時ディレクトリ> -run-id ../../etc/cron.d/evil` を実行し、次を検証する。(a) 終了コードが 1、(b) 標準エラー出力に `invalid_run_id` が含まれる、(c) 標準エラー出力に `logging.RunIDFormatDescription()` の文字列が含まれる、(d) 標準出力・標準エラー出力のいずれにも `../../etc/cron.d/evil` が部分文字列として現れない、(e) 標準出力の `RUN_SUMMARY` 行の `run_id` フィールドの値が `logging.ValidateRunID` を通過する、(f) `-log-dir` に指定した一時ディレクトリの中身が 0 件、(g) 一時ディレクトリの直下に `etc` という名前のエントリが作られていない。
  - 実装時の訂正: 起動には `go run .` ではなく `newGoRunCmd`（`cmd/runner/testutil_ldflags_test.go`）を用いる。`go run` は子プロセスの終了コードを非0であれば常に 1 に潰すため、(a) の「終了コードが 1」が反証不能なアサーションになる（run ID が受理された場合の実際の終了コードは dry-run の deny 経路で 3）。本 PR の拒否系3件すべてに同じ訂正を適用する。
  - 実装時の訂正: (f) の「一時ディレクトリの中身が 0 件」を `assert.Empty(t, os.ReadDir(logDir))` で検査するため、(g) の「直下に `etc` エントリがない」は (f) に包含される。個別のアサーションは追加していない。
  - (g) の対象を一時ディレクトリ**直下**とする根拠: 入口検証がなければ構築されるパスは `filepath.Join(logDir, "<hostname>_<timestamp>_../../etc/cron.d/evil.json")` であり、`filepath.Join` の正規化で `<hostname>_<timestamp>_..` が1つの通常要素として直後の `..` に打ち消されるため、脱出先は `<logDir>/etc/cron.d/evil.json`、すなわちログディレクトリの内側になる。一時ディレクトリの親ディレクトリを検査対象にしても、実際に書き込みが発生する位置（一時ディレクトリの直下）を検査したことにはならない。
- [x] 同ファイルに `TestE2E_PreExecutionError_InvalidRunIDNewlineInjection` を追加する。`-run-id` に Go の文字列リテラル `"x\nRUN_SUMMARY run_id=fake exit_code=0"`（実際の改行を含む値）を argv 要素として与え、(a) 終了コードが 1、(b) 標準出力に `RUN_SUMMARY` を含む行がちょうど1行しか現れない、(c) 標準出力に `run_id=fake` が現れないことを検証する（`02_architecture.md` §7.4）。
- [x] 同ファイルに `TestE2E_PreExecutionError_InvalidRunIDTooLong` を追加する。`-run-id` に `strings.Repeat("a", logging.MaxRunIDLength+1)` を与え、終了コードが 1 で標準エラー出力に `invalid_run_id` が現れることを検証する。
- [x] `cmd/runner/integration_logger_test.go` に `TestE2E_ValidRunIDIsAdopted` を追加する（成功経路のテストであり、`integration_pre_execution_error_test.go` の担当範囲ではない）。`go run . -config <有効な設定> -dry-run -log-dir <一時ディレクトリ> -run-id backup-20260805-143000` を実行し、(a) 終了コードが 0、(b) 一時ディレクトリに `*_backup-20260805-143000.json` に一致するファイルがちょうど1件生成されていることを検証する。`RUN_SUMMARY` は誤り経路でしか出力されないため（§1.3）、採用された run ID の観測にはログファイル名（`{hostname}_{timestamp}_{runID}.json`、[logger.go:138](../../../internal/runner/bootstrap/logger.go#L138)）を用いる。
  - 実装時の訂正: `go run .` ではなく `newGoRunCmdWithHashDir`（`cmd/runner/testutil_ldflags_test.go`）で `-ldflags` にハッシュディレクトリを埋め込んだバイナリを起動し、設定ファイルと `/bin/echo` のハッシュを事前記録する（`recordDryRunJSONHashes` を再利用）。ハッシュ未記録のまま `-dry-run` を実行すると dry-run プレビューが「検証不能」と判定して終了コード 3 を返すため、(a) の「終了コードが 0」を満たせない。また `go run` は子プロセスの終了コードを常に 1 に潰すため、終了コードを直接主張できない。これに伴い `cmd/runner/integration_logger_test.go` へ `//go:build test` を追加する（`newGoRunCmdWithHashDir` が同タグ配下にあるため）。本リポジトリのテスト・lint はすべて `-tags test` で実行されるため、検証範囲は失われない。

**完了条件（Phase 3-B）**

- [x] `go test -tags test ./cmd/runner/...` が成功する。
- [x] 既存の `TestShortFlags`・`TestShortFlagsEquivalence`（`cmd/runner/main_test.go`）がアサーション無変更で通過する。

### PR-3 作成ポイント: --run-id entry validation

**対象ステップ**: Phase 3-B

**推奨タイトル**: `feat(0162): validate --run-id before it reaches any output path`

**レビュー観点**: 拒否経路が `bootstrapID` を使い、フラグの生値が標準出力・標準エラー出力・ログファイル名のいずれにも現れないこと（`resolveRunID` の失敗時にパッケージ変数 `runID` へ生値を代入していないこと） / `resolveRunID` の結果がローカル変数ではなくパッケージ変数 `runID` へ代入され、`mainWithExitCode` 以降が検証済みの値だけを読むこと / パストラバーサルの E2E が `-log-dir` の**直下**に `etc` エントリが作られていないことを検査しており、検査位置の根拠（`filepath.Join` の正規化）が実装と合っていること / `-run-id` の登録文字列が `main.go` と `main_test.go` でトークン単位に一致していること

**実装モデル要件**: frontier-required

**判定理由**: `go run .` で実プロセスを起動する E2E 統合テスト4件（拒否系3件・受理系1件）という重い統合テスト面を持ち、`mkplan.md` step 8 のパネル発動条件に該当する。加えて `--run-id` の受理形式を絞る破壊的変更であり、拒否経路自体が M-1 の被害（ログ行注入）を再現しうるという、経路の向きを取り違えやすい判断を含む。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 4: `bootstrap` の多層防御

**前提**: Phase 1（`logging.ValidateRunID`）が完了していること。

**対象ファイル**: `internal/runner/bootstrap/logger.go`、`internal/runner/bootstrap/logger_test.go`

**作業内容**

- [ ] `SetupLoggerWithConfig`（[logger.go:75](../../../internal/runner/bootstrap/logger.go#L75)）の先頭、`hostname := common.GetHostname()` より前に `if err := logging.ValidateRunID(config.RunID); err != nil { return fmt.Errorf("invalid run ID: %w", err) }` を追加する。`config.LogDir` が空かどうかで分岐させない（`02_architecture.md` §3.3）。
- [ ] シグネチャは変更しない。ハンドラ生成・ファイルオープン・グローバル変数への代入は、この検証を通過した後にのみ実行されることを確認する。

**テスト**

- [ ] `internal/runner/bootstrap/logger_test.go` に `TestSetupLoggerWithConfig_RejectsInvalidRunID` を追加する。`LogDir` に `tu.SafeTempDir(t)` を指定し、`RunID` に `../evil`・`/tmp/evil`・`""`・実際の改行を含む値・`logging.MaxRunIDLength+1` 文字の値をそれぞれ与えたテーブルで、(a) エラーが返ること、(b) `errors.Is(err, logging.ErrInvalidRunID)` が真であること、(c) `os.ReadDir(LogDir)` の結果が 0 件であること（ログファイルが作られていないこと）を検証する。
- [ ] 同テーブルに `LogDir: ""` かつ `RunID: "../evil"` のケースを追加し、エラーが返り `errors.Is(err, logging.ErrInvalidRunID)` が真であることを検証する。この1件がないと、検証を `if config.LogDir != ""` で囲った誤実装がテストを全件通過してしまい、`02_architecture.md` §3.3 が求める「`LogDir` の有無で防御の有無を変えない」という設計が守られたことを確認できない。
- [ ] 同テストは `bootstrap.SetupLogging` を経由せず `SetupLoggerWithConfig` を直接呼ぶ構成とし、入口検証を経ない呼び出しでも防御が働くことを示す（AC-12）。
- [ ] 既存テストが使う `saveAndRestoreGlobals(t)` 相当のグローバル退避・復元を、新規テストでも同じ形で呼ぶ（`SetupLoggerWithConfig` は `slog.SetDefault` とパッケージグローバルを書き換えるため）。

**完了条件**

- [ ] `go test -tags test ./internal/runner/bootstrap/... ./cmd/runner/...` が成功する。
- [ ] `internal/runner/bootstrap/logger_test.go`・`internal/runner/bootstrap/environment_test.go`・`cmd/runner/integration_logger_test.go` の既存テストが `RunID` 値の修正なしで通過する。

### PR-4 作成ポイント: bootstrap defense in depth

**対象ステップ**: Phase 4

**推奨タイトル**: `feat(0162): validate run ID in SetupLoggerWithConfig as defense in depth`

**レビュー観点**: 検証が `config.LogDir` の有無で分岐していないこと（`LogDir: ""` かつ不正 `RunID` のケースがテーブルにあり、それが `if config.LogDir != ""` で囲った誤実装を落とすこと） / 検証が `common.GetHostname()`・ハンドラ生成・ファイルオープン・グローバル変数への代入のいずれよりも前に置かれていること / テストが `SetupLogging` を経由せず `SetupLoggerWithConfig` を直接呼び、拒否時に `LogDir` が 0 件のままであることを確認していること / `SetupLoggerWithConfig` のシグネチャが変わっておらず、既存テストの `RunID` 値に差分がないこと

**実装モデル要件**: standard

**判定理由**: 該当するトリガーなし。追加は関数先頭の 1 分岐とテーブル駆動テスト1件で、実装方針は `02_architecture.md` §3.3 で確定しており §1.3 に競合案の記載はない。統合テスト・CI・外部資源の面もなく、`cmd/runner` 側は既に PR-3 で入口検証を持つため利用者から見える挙動の引き上げも伴わない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 5: `cmd/verify` の fail-closed 化

Phase 1〜4 とは依存関係がないため、Phase 1 と並行して着手できる（`02_architecture.md` §8.1）。

**対象ファイル**: `cmd/verify/main.go`、`cmd/verify/main_test.go`

**作業内容（製品コード）**

- [ ] `cmd/verify/main.go` に終了コード定数 `exitOK = 0`・`exitVerificationFailed = 1`・`exitUntrustedEnvironment = 3` を追加する。`2` を欠番にした理由（Go ランタイムが未捕捉 panic に使用する）をコメントで書く（`02_architecture.md` §3.4）。
- [ ] `run()` と `processFiles` にある裸の `return 0` / `return 1` をすべて上記定数へ置き換える（`rg -n "return [01]$" cmd/verify/main.go` で全数を洗い出す）。
- [ ] パッケージ変数 `toctouChecker security.DirectoryPermChecker` を既存の差し替え口（[main.go:24-29](../../../cmd/verify/main.go#L24-L29)）と同じ `var` ブロックへ追加する。`nil` は「`security.NewDirectoryPermChecker` で生成する」を意味する旨をコメントに書く。
- [ ] `checkDirPermissions(cfg *verifyConfig, stderr io.Writer) bool` を追加し、`run()` にインライン展開されている TOCTOU 処理（[main.go:78-109](../../../cmd/verify/main.go#L78-L109)）をここへ移す。絶対パス化とシンボリックリンク解決の前処理は現行の実装をそのまま移設する（共通化しない。`02_architecture.md` §3.4 の「既知の重複」）。
- [ ] `security.NewDirectoryPermChecker` 失敗時の `panic`（[main.go:82-87](../../../cmd/verify/main.go#L82-L87)）を、削除せず `checkDirPermissions` 内へ移設する。`toctouChecker` が `nil` のときのみチェッカーを生成し、失敗したら panic する形とし、`cmd/record/main.go` の同処理（[main.go:105-111](../../../cmd/record/main.go#L105-L111)）に揃える。この panic は `02_architecture.md` §1.3 D-2 が exit 2 を予約した根拠となる2箇所の panic の一方であるため、経路として存続させる必要がある。
- [ ] `checkDirPermissions` の中で `security.CollectTOCTOUCheckDirs(nil, nil, absHashDir)` を呼んでハッシュディレクトリ集合を得て、先に `security.RunTOCTOUPermissionCheck` にかける。違反が1件以上あれば、各違反について `cmd/record/main.go` の `checkDirPermissions`（[main.go:142-153](../../../cmd/record/main.go#L142-L153)）と同じ形で ERROR ログ（`path`・`violation`・`remediation` 属性）を出し、標準エラー出力へ理由と是正方法を示す1行を書いて `false` を返す。この時点で対象ファイル集合のチェックは行わない。
- [ ] 標準エラー出力へ書く1行を、`record` の文言（`"Error: permission violation in hash directory or its ancestor directories — refusing to generate hash records. Fix directory permissions and re-run."`、[main.go:154](../../../cmd/record/main.go#L154)）に倣った英語の文字列リテラルとし、`verify` の文脈へ合わせて `"Error: permission violation in hash directory or its ancestor directories — verification results cannot be trusted; no file was verified. Fix directory permissions and re-run."` とする。`remediation` の組み立ても `record` に倣い、`security.ErrInvalidDirPermissions` の場合は `chmod go-w <path>` を含む文言、それ以外は一般的な文言とする。
- [ ] 新設する `fmt.Fprintln` / `fmt.Fprintf` の標準エラー出力への書き込みに `//nolint:errcheck` を付ける。`cmd/verify/main.go` と `cmd/record/main.go` の既存の同種の書き込みはすべてこの抑制を持っており、付けないと Phase 6 の `make lint` が失敗する。抑制は個々の文に限定し、ファイル単位・パッケージ単位では行わない。
- [ ] ハッシュディレクトリ集合に違反がない場合のみ、`security.CollectTOCTOUCheckDirs(absFiles, nil, "")` の結果からハッシュディレクトリ集合を差し引いた差分に対して `security.RunTOCTOUPermissionCheck` を呼び、戻り値は破棄して（WARN のみ）`true` を返す。
- [ ] `run()` から `checkDirPermissions(cfg, stderr)` を呼び、`false` なら `validatorFactory` を呼ばずに `exitUntrustedEnvironment` を返す。呼び出し位置は `ensurePermissionCheckUID` の直後、`validatorFactory` より前とする（`02_architecture.md` §6.2）。
- [ ] `run()` の TOCTOU 処理に付いている現行コメント「Violations are logged as warnings only — verify continues even if the check fails.」（[main.go:80-81](../../../cmd/verify/main.go#L80-L81)）を削除する。

**テスト**

- [ ] `cmd/verify/main_test.go` にスタブ型 `fakeDirPermChecker`（`ValidateDirectoryPermissions(path string) error` を `validateDirFn func(path string) error` へ委譲する）と、注入ヘルパー `overrideTOCTOUChecker(t *testing.T, checker security.DirectoryPermChecker)` を追加する。ヘルパーは元の値を `t.Cleanup` で復元する。`cmd/verify` 内でのみ使うため、専用のヘルパーファイルは作らず `main_test.go` に置く。`cmd/record/main_test.go` にも同名・同形のスタブがあるが、別パッケージの非公開型であり import できないため、この重複は意図的なものである旨をスタブ定義の直上に英語コメントで記す。
- [ ] `TestRunFailsClosedOnHashDirViolation_ExplicitHashDir` を追加する。`-hash-dir` に `tu.SafeTempDir(t)` で作った一時ディレクトリを渡し、そのディレクトリを `os.Chmod(dir, 0o777)` で world-writable にする（`t.Cleanup` で 0o755 へ戻す）。実チェッカーのまま `run()` を呼び、(a) 終了コードが `exitUntrustedEnvironment`、(b) `validator.calls` が 0 件、(c) 標準出力が空（`Verifying N files...` が出ていない）、(d) 標準エラー出力が `"verification results cannot be trusted"` と `"Fix directory permissions"` を含むことを検証する（AC-19・AC-21・AC-23 の明示指定側）。
- [ ] `TestRunFailsClosedOnHashDirViolation_DefaultHashDir` を追加する。`mkdirAll` をスタブし、`toctouChecker` に「常に `security.ErrInvalidDirPermissions` をラップしたエラーを返す」スタブを注入したうえで `-hash-dir` を指定せずに `run()` を呼び、終了コードが `exitUntrustedEnvironment` かつ `validator.calls` が 0 件であることを検証する（AC-23 の既定ディレクトリ側）。
- [ ] `TestRunFailsClosedOnHashDirViolation_LogsErrorLevel` を追加する。`slog.SetDefault` を捕捉用ハンドラへ差し替え（`t.Cleanup` で復元）、`toctouChecker` に違反を返すスタブを注入して `run()` を呼び、捕捉したログに `level=ERROR` の行が違反ごとに1件以上出ており、`path` と `remediation` の属性を含むことを検証する。共有チェックが出す WARN 行も残っていること（ERROR が WARN の置き換えではなく追加であること）を併せて検証する（AC-20）。
- [ ] `TestRunSkipsTargetSetCheckWhenHashDirViolates` を追加する。`toctouChecker` に「ハッシュディレクトリのパスと対象ファイルの祖先パスの**両方**に違反を返す」経路依存スタブを注入して `run()` を呼び、終了コードが `exitUntrustedEnvironment` であること、および捕捉したログに対象ファイル側のパスに対する WARN 行が1件も出ていないことを検証する。これは `02_architecture.md` §4.3 の副作用契約「対象ファイル集合の権限チェックは発生しない」を検証する唯一のテストである。
- [ ] `cmd/verify/main_test.go` の `toctouChecker`・`slog` の既定ロガー・その他パッケージ変数を差し替えるテストに `t.Parallel()` を付けない。`validatorFactory`・`mkdirAll`・`ensurePermissionCheckUID`・`toctouChecker` はいずれもパッケージ変数であり、既存テストも逐次実行を前提としている。この前提をファイル冒頭のコメントに英語で記す。
- [ ] `TestRunTOCTOU_ContinuesOnWorldWritableDir`（`cmd/verify/main_test.go:155`）を `TestRunTOCTOU_ContinuesWhenOnlyTargetDirViolates` へ改名する。既存のアサーション（exit 0 と `validator.calls` が1件）は変更しない。
- [ ] 同テストに経路依存の `toctouChecker` スタブを注入する。world-writable にした対象ファイルの親ディレクトリに対しては `security.ErrInvalidDirPermissions` をラップしたエラーを返し、それ以外のパス（ハッシュディレクトリとその祖先を含む）には `nil` を返す。現行はハッシュディレクトリ側も実ファイルシステムの権限に依存しており、fail-closed 化後はホスト構成次第で誤った exit 3 になりうる。`02_architecture.md` §7.1 の方針（実ファイルシステムに依存してよいのは自分で権限を操作したディレクトリのみ）を、このテストにも他の4件と同じく適用する。
- [ ] 同テストの関数コメントを書き換える。現行の「TestRunTOCTOU_ContinuesOnWorldWritableDir verifies that the verify command continues processing even when the file's parent directory is world-writable. This validates AC-M2S-7: verify warns but does not abort on TOCTOU violations.」を、「a violation confined to a target file's ancestor directories is not fail-closed: only the hash directory is the root of trust, so verification continues and the violation stays a warning」の趣旨を述べる英語コメントへ置き換える。`AC-M2S-7` への参照は削除し、代わりの `AC-NN` 形式の識別子も**書かない**（Go ソースへの `AC-NN` / `F-NNN` 参照は要件プロセスガイド §4 が禁じており、`runplan` のコミット前チェックが拒否する）。挙動そのものを説明する文にとどめる。
- [ ] 同テストに、終了コードが `exitUntrustedEnvironment` ではないことの明示的なアサーションを追加する（AC-28 が「fail-closed にならない」ことを主張していると読めるようにする）。
- [ ] `TestRunUsesDefaultHashDirectoryWhenNotSpecified`（:129）に `overrideTOCTOUChecker` で「違反を返さないスタブ」を注入し、ホストの `/usr/local` の権限に依存しないようにする（`02_architecture.md` §7.1）。
- [ ] `TestRunProcessesMultipleFiles`（:57）に同じ「違反を返さないスタブ」を注入する。
- [ ] `TestRunReportsFailuresAndContinues`（:77）に同じ「違反を返さないスタブ」を注入する。
- [ ] `TestRunWarnsWhenDeprecatedFlagUsed`（:97）に同じ「違反を返さないスタブ」を注入する。

**完了条件**

- [ ] `go test -tags test ./cmd/verify/...` が成功する。
- [ ] `rg -n "return 0|return 1|return 3" cmd/verify/main.go` の結果が 0 件である（すべて定数経由になっている）。

### PR-5 作成ポイント: verify fail-closed on hash directory violations

**対象ステップ**: Phase 5

**推奨タイトル**: `feat(0162): fail closed on hash directory violations in verify`

**レビュー観点**: `CollectTOCTOUCheckDirs` の2回呼び出しによるハッシュディレクトリ集合と対象ファイル集合の分割が正しく、ハッシュ側に違反があるときに対象ファイル側のチェックへ進まないこと（`TestRunSkipsTargetSetCheckWhenHashDirViolates` が唯一の検証である） / `exitUntrustedEnvironment` が `validatorFactory` を呼ぶ前に返り、`Verify` が1件も実行されず標準出力が空であること / 終了コード 2 を欠番とした理由がコメントに残り、`NewDirectoryPermChecker` 失敗時の panic 経路が `checkDirPermissions` 内へ移設されて存続していること / 既存4テストへのスタブ注入がアサーションを変えておらず、`TestRunTOCTOU_ContinuesWhenOnlyTargetDirViolates` の経路依存スタブがホストのファイルシステム構成から切り離されていること

**実装モデル要件**: frontier-required

**判定理由**: `verify` の信頼判断そのものを変えるセキュリティゲート段階の変更（fail-open → fail-closed、`mkplan.md` step 8 のパネル発動条件）に該当する。加えて、引き上げ範囲をハッシュディレクトリ側に限定し対象ファイル側を警告のまま据え置くという判定範囲の切り分けを含み、2集合の分割を誤ると exit 0 の信頼性が静かに失われたままテストが通り続ける。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 6: 文書と CHANGELOG の更新

**前提**: Phase 3〜5 の実装が完了し、利用者から見える挙動が確定していること。

**対象ファイル**: `docs/user/runner_command.ja.md`、`docs/user/verify_command.ja.md`、`CHANGELOG.ja.md`、`docs/translation_glossary.md`、および各英語版

**作業内容**

- [ ] `docs/user/runner_command.ja.md` の `-run-id` 節（894行目〜）の「**パラメータ**」に受理形式を明記する。現行の `- `<id>`: 実行を識別する一意な文字列（推奨：ULID形式）` を、受理形式が「英大文字・英小文字・数字・アンダースコア（`_`）・ハイフン（`-`）のみ、1〜64文字」であること、およびこれに合致しない値を指定した場合は実行を開始せずエラー終了することを述べる記述へ書き換える。
- [ ] 同節に、拒否される値の代表例（パス区切り文字 `/` を含む値、`..`、空白や改行を含む値、65文字以上の値）を挙げる。
- [ ] 同節の「**注意事項**」にある「同じRun IDを複数回使用すると、ログファイルが上書きされる可能性があります」（955行目）を維持する（`02_architecture.md` §5.3 の残存リスクに対応する注意書き）。
- [ ] `docs/user/runner_command.ja.md` にある誤ったログファイル命名規則を全件修正する。`rg -n -e "runner-<run-id>" -e "runner-01K" docs/user/runner_command.ja.md` のヒット全件（849行目の規則、854行目付近の例、1660行目付近の `cat` コマンド例）を、実装（[logger.go:138](../../../internal/runner/bootstrap/logger.go#L138)）に合わせて `<hostname>_<timestamp>_<run-id>.json` の形式へ書き換える。
- [ ] `docs/user/verify_command.ja.md` の「2.4 終了コードによる判定」（93行目）に終了コードの一覧表を新設する。0（全ファイルの検証成功）、1（引数エラー・バリデータ生成失敗・1件以上の検証失敗）、2（Go ランタイムが未捕捉 panic に使用する予約値。`verify` が明示的に返すことはない）、3（ハッシュディレクトリまたはその祖先ディレクトリの権限違反により検証を1件も行わずに終了）を記載する（`02_architecture.md` §4.2）。
- [ ] 同文書の「終了コードを使用した適切なエラーハンドリング」（630行目付近）のスクリプト例に exit 3 の分岐を追加する。同節は出力の `grep` でエラー種別を分けているため、exit 3 を「検証結果が信頼できない」ケースとして明示的に扱う分岐を加える。
- [ ] 同文書に、exit 3 となる条件（ハッシュディレクトリ側の違反に限ること）、対象ファイル側のみの違反では警告のうえ検証が継続すること、バイパス手段は用意しないこと、是正方法はハッシュディレクトリの権限修正または権限の適切なパスへの移動であることを記載する（`02_architecture.md` §8.2）。
- [ ] `CHANGELOG.ja.md` の `## [未リリース]` 節に `### 破壊的変更` を新設し、「なし」を置き換える。項目1は `runner`: `--run-id` の受理形式を `^[A-Za-z0-9_-]{1,64}$` に限定し、合致しない値は起動前に拒否すること。影響を受けるのは自動生成以外の値を渡している CI・運用スクリプトであり、渡している値がこの形式に収まるかを確認すれば影響の有無を判定できる旨を書く。
- [ ] `CHANGELOG.ja.md` の同節に項目2として、`verify`: ハッシュディレクトリまたはその祖先ディレクトリの TOCTOU 権限違反を検出した場合、対象ファイルを1件も検証せず exit 3 で終了するようになったことを書く。対象ファイル側のみの違反は従来どおり警告で継続すること、バイパス手段がないことを併記する。
- [ ] 同項目に、アップグレード前に影響有無を判定する手順を書く（`02_architecture.md` §8.2）。現行版の `verify` を対象ファイルに対して実行し、標準エラー出力の `TOCTOU permission check violation` 警告のうち、`path` がハッシュディレクトリまたはその祖先を指すものがあるかを確認する、という手順を具体的なコマンド例つきで示す。
- [ ] `docs/translation_glossary.md` を確認し、本タスクで導入した用語のうち未収録のもの（「受理形式」「多層防御」「信頼の起点」「起動時特権降格」など、実際に文書へ書いた語）を追加する。既に収録されている語は追加しない。
- [ ] `docs/user/runner_command.md` と `docs/user/verify_command.md`（英語版）を `/mktrans` で日本語版から反映する（AC-24・AC-25 が指定する手順）。英語版の `runner_command.md` にも誤ったログファイル命名規則が3箇所（854・859・1695行目付近）あるため、日本語版と同じ修正が反映されていることを確認する。
- [ ] `CHANGELOG.md`（英語版）に `CHANGELOG.ja.md` と同じ2項目を反映する。

**検証**

- [ ] `docs/user/verify_command.ja.md` に新設した終了コード表の内容が実装と一致することを、`rg -n "exitUntrustedEnvironment|exitVerificationFailed|exitOK" cmd/verify/main.go` の定義値と突き合わせて確認する。
- [ ] CHANGELOG に書いた影響判定手順のコマンドを実際に実行し、記載どおりの出力（`TOCTOU permission check violation` を含む WARN 行、または違反なしの場合は該当行が出ないこと）になることを確認する。確認には Phase 5 適用前の `verify` を用いる。`git worktree add <一時ディレクトリ> <Phase 5 着手前のコミット SHA>` で作業ツリーを作り、そこで `go build -o <一時ディレクトリ>/verify ./cmd/verify` してから実行する（`git stash` は未コミットの変更しか退避しないため、PR ごとにコミット・マージする本計画の進め方では旧版のビルドを得られない）。
- [ ] `docs/user/runner_command.ja.md` に書いた受理形式の説明が `logging.RunIDFormatDescription()` および `logging.MaxRunIDLength` と一致することを、両者を並べて確認する。
- [ ] 修正したログファイル命名規則の記述が実装と一致することを、`-log-dir` を指定して `runner` を1回実行し、生成されたファイル名と突き合わせて確認する。

**完了条件**

- [ ] `make test` と `make lint` が成功する。
- [ ] 文書の日本語版と英語版の章立てが一致している。

### PR-6 作成ポイント: user documentation and changelog

**対象ステップ**: Phase 6

**推奨タイトル**: `docs(0162): document run ID format and verify exit code 3`

**レビュー観点**: 新設した終了コード表の値が `cmd/verify/main.go` の `exitOK` / `exitVerificationFailed` / `exitUntrustedEnvironment` と一致し、2 を欠番として説明していること / 受理形式の記述が `logging.RunIDFormatDescription` と `logging.MaxRunIDLength` と一致していること / 誤ったログファイル命名規則が日本語版3箇所・英語版3箇所のすべてで `<hostname>_<timestamp>_<run-id>.json` へ修正され、`rg` 検査が 0 件になること / CHANGELOG の影響判定手順が Phase 5 適用前のビルドで実際に再現され、記載どおりの出力になること

**実装モデル要件**: standard

**判定理由**: 該当するトリガーなし。既に確定した挙動を文書へ反映する作業のみで、製品コードの変更を含まない。`git worktree` による旧版ビルドは記述の裏取り手順であって、CI・外部資源への依存を新たに持ち込むものではない。破壊的変更の記述そのものは PR 本文にも要約する。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

マイルストーンは進捗把握のための内部的な区切りであり、マージの単位は §3.2 の PR である。両者の対応を「対応 PR」列に示す。

| マイルストーン | 含むフェーズ | 対応 PR | 成果物 | 完了判定 |
|---|---|---|---|---|
| M1: run ID 形式の基盤 | Phase 1 | PR-1 | `internal/logging/runid.go`、`runid_test.go`、`ErrorTypeInvalidRunID` | `go test -tags test ./internal/logging/...` が成功 |
| M2: ガード共通部品の拡張 | Phase 2 | PR-2（M3 と同一 PR） | `identitymutationguard` の位置情報・追跡対象指定・製品ファイル列挙 | 既存2つのガードテストが無変更で通過 |
| M3: 起動時特権降格の完全化 | Phase 3-A | PR-2 | 起動順序の是正、`dropStartupPrivileges`、振る舞いテスト、起動順序ガードテスト | `go test -tags test ./cmd/runner/...` が成功 |
| M4: `--run-id` の入口検証 | Phase 3-B | PR-3 | `resolveRunID`、フラグ説明文字列、E2E テスト4件 | `go test -tags test ./cmd/runner/...` が成功 |
| M5: 多層防御 | Phase 4 | PR-4 | `SetupLoggerWithConfig` の再検証 | `go test -tags test ./internal/runner/bootstrap/... ./cmd/runner/...` が成功 |
| M6: `verify` の fail-closed 化 | Phase 5 | PR-5 | `checkDirPermissions`、終了コード定数、`toctouChecker` | `go test -tags test ./cmd/verify/...` が成功 |
| M7: 文書反映 | Phase 6 | PR-6 | 利用者向け文書、CHANGELOG、用語集 | `make test` と `make lint` が成功 |

M2 は単独ではマージの単位にならない（`identitymutationguard` の拡張はその唯一の利用者である M3 のガードテストと同じ PR で入る）。

Phase 5（M6）は他フェーズに依存しないため、Phase 1 と並行して着手できる。Phase 3-A は Phase 2 を前提とし、Phase 3-B は Phase 1 と Phase 3-A の両方を前提とする。Phase 4 は Phase 1 を前提とする。Phase 6 は Phase 3〜5 の完了を前提とする。

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1 | `internal/logging/runid.go` の新設（`MaxRunIDLength`・`RunIDFormatDescription`・`ErrInvalidRunID`・`ValidateRunID`）、`GenerateRunID` の移設、`ErrorTypeInvalidRunID` の追加、`runid_test.go`（新規6テスト + 移設2テスト） | standard |
| PR-2 | Phase 2 / Phase 3-A | `identitymutationguard` の位置情報・呼び出し側指定・製品ファイル列挙の追加と、`cmd/runner` の起動時特権降格の完全化（`dropStartupPrivileges` / `reportStartupPrivilegeFailure` / `main()` 先頭の順序是正）、`startup_privilege_test.go` の4テスト、`startup_order_guard_test.go` の主張1〜4とコントロールケース | frontier-required |
| PR-3 | Phase 3-B | `resolveRunID` とパッケージ変数 `runID` への代入、拒否時の報告経路、`-run-id` フラグ説明文字列の更新（`main.go` / `main_test.go`）、`TestResolveRunID`、E2E 統合テスト4件 | frontier-required |
| PR-4 | Phase 4 | `SetupLoggerWithConfig` 先頭での `ValidateRunID` 呼び出しと `TestSetupLoggerWithConfig_RejectsInvalidRunID` | standard |
| PR-5 | Phase 5 | `cmd/verify` の終了コード定数、`toctouChecker` パッケージ変数、`checkDirPermissions` の切り出しと2集合の分割判定、fail-closed テスト4件、既存5テストのスタブ注入・改名 | frontier-required |
| PR-6 | Phase 6 | 利用者向け文書（日英）、`CHANGELOG.ja.md` / `CHANGELOG.md` の破壊的変更2項目、用語集 | standard |

**PR 分割の根拠**

- **PR-1 を独立させる理由**: Phase 1 は既存の挙動を一切変えない追加（と同一パッケージ内の関数移設）であり、単独でグリーンゲートを通せる。受理形式という本タスクの中核判断（`02_architecture.md` §1.3 D-1）を、それを利用する2つの経路（PR-3 の入口検証、PR-4 の多層防御）から切り離して集中的にレビューできる。
- **Phase 2 と Phase 3-A を1つの PR にまとめる理由**: Phase 2 は `identitymutationguard` の API を追加するのみで、自前のテストを持たない（Phase 2 の「テスト」節のとおり、拡張の検証は Phase 3 のガードテストが利用者として行う）。両者を分けると、利用者もテストも無い API だけが先にマージされる PR ができ、「インターフェース + 実装 + テスト」という密結合の単位を PR 境界で割ることになる。Phase 3-A のガードテストは追加した API をすべて使うため、この組み合わせで密結合の単位が閉じる。
- **Phase 3 を 3-A と 3-B に分ける理由**: Phase 3 は F-003（特権降格、AST 静的検証で担保）と F-001（run ID 検証、E2E 統合テストで担保）という関心事もリスクの担保手段も異なる2つの塊を含み、まとめると `cmd/runner` の起動処理全体が1つの差分になる。両者が共有するのは `main()` の同一箇所の書き換えのみで、3-A が run ID の既定値設定を現行のまま残すことで、3-A 単独でもグリーンゲートを通せる。分割により AST 走査のレビューと E2E のレビューが互いのノイズにならない。
- **PR-4 が PR-3 より後になる理由**: Phase 4 は `internal/runner/bootstrap` の変更だが、`SetupLoggerWithConfig` のシグネチャを変えず、`cmd/runner` 側はこの変更に依存しない（PR-3 は入口検証を自前で持つ）。したがって「`internal/` を `cmd/` より先に」の原則には抵触しない。文書順どおりに PR 番号を振り、多層防御という独立した関心事を単独でレビューできるようにした。
- **PR-5 を独立させる理由**: Phase 5 は他のどのフェーズにも依存せず、`cmd/verify` のみを触る。`runner` 側の変更とは共有するコードがないため、fail-closed の判定範囲というリスクの高い判断を無関係な差分なしでレビューできる。
- **PR-6 を分割しない理由**: Phase 6 の英語版反映は `/mktrans` による日本語版からの機械的な反映であり、日本語版と同じ PR に入れることで日英の乖離（R-7）をレビュー時点で検出できる。日本語版の確定が滞って直列化した場合に限り、S-2 のバッファ計画に従って分割する（下記）。

**条件付き PR-7（S-2 が発動した場合のみ）**: 日本語版の確定待ちで直列化した場合、PR-6 を日本語版（`docs/user/*.ja.md`・`CHANGELOG.ja.md`・用語集）のみで切り、英語版3ファイルの反映を PR-7 として追加する。その場合の `推奨タイトル` は `docs(0162): sync English user documentation and changelog`、`レビュー観点` は「日本語版との章立ての一致 / 誤ったログファイル命名規則3箇所の修正の反映 / 終了コード表の値の一致」、`実装モデル要件` は `standard`（該当するトリガーなし。`/mktrans` による反映のみ）とする。分割の判断基準は「日本語版が確定してから2営業日以内に英語版を反映できないこと」とする。

**マージ順序**: PR-1 → PR-2 → PR-3 → PR-4 → PR-5 → PR-6 の順にマージする。PR-3 は PR-1 が導入する `logging.ValidateRunID`・`RunIDFormatDescription`・`ErrInvalidRunID` を参照するためコンパイルできず、PR-4 も同じ理由で PR-1 に依存する。PR-3 は PR-2 が確定させた `main()` の順序の上に `bootstrapID` と `resolveRunID` を挿入するため、PR-2 の後でなければならない。PR-6 は PR-2〜PR-5 が確定させた利用者から見える挙動を記述する。並行して進めてよいのは次の2つである。PR-5 は他のどの PR にも依存しないため PR-1 と並行して着手・レビューでき、この順序の中で前倒ししてもよい（`02_architecture.md` §8.1）。PR-4 も PR-1 のマージ後であれば PR-2・PR-3 と並行して着手・レビューできる。各 PR が単独でグリーンゲートを通せるというのは、先行する PR がマージ済みであることを前提とした話である。

---

## 4. テスト戦略

テストの配置と観点は `02_architecture.md` §7 に従う。本節はそれを実行計画へ落とし込んだものである。

### 4.1 単体テスト

| 対象 | ファイル | 主な観点 |
|---|---|---|
| `logging.ValidateRunID` | `internal/logging/runid_test.go` | 長さ境界（0・1・`MaxRunIDLength`・`MaxRunIDLength+1`）、許可文字の全種別、拒否ケース（パス区切り・`..`・空白・改行・NUL・ESC・マルチバイト・`%`）、エラーが入力値全体を含まないこと |
| `logging.GenerateRunID` | `internal/logging/runid_test.go` | 一意性・形式（既存テストを移設）、出力が `ValidateRunID` を通過すること |
| `resolveRunID` | `cmd/runner/main_test.go` | 未指定・明示的な空文字列・受理・拒否の4分岐 |
| `dropStartupPrivileges` | `cmd/runner/startup_privilege_test.go` | `Setegid` 失敗・`Seteuid` 失敗・正常系。失敗時に実効ユーザーIDと実効グループIDが変化しないこと、正常系で両者が降格先と一致すること |
| `reportStartupPrivilegeFailure` | `cmd/runner/startup_privilege_test.go` | 報告に含まれる run ID が空でなく `ValidateRunID` を通過すること、失敗段階が判別できること |
| `SetupLoggerWithConfig` | `internal/runner/bootstrap/logger_test.go` | 不正な `RunID` でエラーを返し、ログファイルを1件も作らないこと |
| `checkDirPermissions` | `cmd/verify/main_test.go` | ハッシュディレクトリ側違反で exit 3・検証0件、対象ファイル側のみの違反で継続、ERROR ログの出力 |

### 4.2 静的検証

- `cmd/runner/startup_order_guard_test.go` が、降格順序（`Setegid` → `Seteuid`）と `main` 本体での降格と `flag.Parse` の前後関係、識別子変更系 syscall の許可リスト、`init()` の個数を AST 走査で検証する。
- 静的検証を AC-14 の主たる手段とする理由（順序どおりに実行されると観測できる痕跡が残らないため実行時テストが成立しない）は `02_architecture.md` §7.2 に記載済み。AC-15・AC-16 は振る舞いテストで担保するため、F-003 全体としては静的検証のみに依存しない。

### 4.3 統合テスト

実際のフラグ解析経路を通した `--run-id` の受理・拒否を、統合テスト4件で検証する。拒否系3件は `cmd/runner/integration_pre_execution_error_test.go` へ、受理系1件（`TestE2E_ValidRunIDIsAdopted`）は `cmd/runner/integration_logger_test.go` へ置く。いずれも既存テストと同様に `go run .` でプロセスを起動し、標準出力・標準エラー出力の全文を捕捉する。

- 統合テストはいずれも `-dry-run` を付けて起動する。`-dry-run` を付けないと `bootstrap.NewVerificationManager` が既定ハッシュディレクトリを要求して別の理由で失敗し、検証したい経路へ到達する前に終了するためである。この制約は追加する4件すべてに等しく適用する。
- 拒否ケースの3件は `-log-dir` に `tu.SafeTempDir(t)` を渡し、拒否後にそのディレクトリが空であることを検証する。

### 4.4 セキュリティテスト

`02_architecture.md` §7.4 の3観点を、次のテストで担保する。

- パストラバーサル: `TestE2E_PreExecutionError_InvalidRunIDPathTraversal`（ログディレクトリ外にファイルが作られないことをファイルシステム上で確認）。
- ログ行注入: `TestE2E_PreExecutionError_InvalidRunIDNewlineInjection`（実際の改行文字を argv 要素として与え、`RUN_SUMMARY` 行が1行しか出ないことを確認）。パーセントエンコード表記は `%` が許可リスト外で別の理由で拒否されるため使わない。
- 入口検証を経ない経路: `TestSetupLoggerWithConfig_RejectsInvalidRunID`（`bootstrap.SetupLoggerWithConfig` を直接呼ぶ）。

### 4.5 後方互換性テスト（回帰）

次の既存テストが、アサーションを変更せずに通過することを確認する。

- `cmd/runner/main_test.go` の `TestShortFlags`・`TestShortFlagsEquivalence`（AC-17）
- `internal/runner/bootstrap/logger_test.go` の全テスト（`SetupLoggerWithConfig` を7箇所で呼ぶ）（AC-13）
- `internal/runner/bootstrap/environment_test.go` の全テスト（`SetupLoggerWithConfig` 直接2箇所、`SetupLogging` 経由3箇所）（AC-13）
- `cmd/runner/integration_logger_test.go` の全テスト（`SetupLoggerWithConfig` 直接3箇所）（AC-13）
- `cmd/verify/main_test.go` の TOCTOU 以外のテスト（AC-22）。ただし `checkDirPermissions` に到達する4件（`TestRunProcessesMultipleFiles`・`TestRunReportsFailuresAndContinues`・`TestRunWarnsWhenDeprecatedFlagUsed`・`TestRunUsesDefaultHashDirectoryWhenNotSpecified`）にはホスト非依存化のための `toctouChecker` 注入を追加するため、「無変更」ではなく「アサーション無変更」で通過することを確認する。
- `internal/runner/resource/identity_mutation_guard_test.go`・`internal/runner/base/risktypes/identity_mutation_guard_test.go`（Phase 2 の後方互換）

---

## 5. リスク管理

| # | リスク | 影響 | 対策 |
|---|---|---|---|
| R-1 | Phase 2 の `CallSite` 拡張が既存2つのガードテストを壊す | セキュリティガードの一時的な失効 | フィールドの追加と新規関数の追加のみとし、既存関数のシグネチャと戻り値の意味を変えない。Phase 2 の完了条件に「既存ガードテストの差分が空であること」を含める |
| R-2 | 起動順序ガードテストの走査対象を取り違え、常に成功する（空振り） | AC-14 の検証が無意味になる | 主張1・2で対象の呼び出しが1件ずつ存在することを `require` で確認し、順序を逆にした合成ソースで判定が失敗側へ倒れることをコントロールケースとして検証する |
| R-3 | `TestRunFailsClosedOnHashDirViolation_ExplicitHashDir` と `TestRunTOCTOU_ContinuesWhenOnlyTargetDirViolates` が実ファイルシステムの権限に依存する | ホスト構成の違いでテストが不安定になる | 一時ディレクトリは `tu.SafeTempDir(t)` で作り、権限操作は自分で作ったディレクトリに限る。他のテストは `toctouChecker` のスタブを注入して実ファイルシステムから切り離す（`02_architecture.md` §7.1 の方針） |
| R-4 | `verify` の絶対パス化・シンボリックリンク解決の失敗時フォールバック（監査所見 L-2）が、fail-closed 化後に誤った exit 3 を生む | 正常な環境で検証が止まる | 本タスクでは受容する（`02_architecture.md` §3.4）。L-2（[#986](https://github.com/isseis/go-safe-cmd-runner/issues/986)）の優先度を上げる根拠として記録する |
| R-5 | `--run-id` の形式厳格化により、既存の CI・運用スクリプトが起動できなくなる | 利用者の運用停止 | CHANGELOG に受理形式と影響判定手順を明記する（Phase 6）。受理形式は利用者向け文書が推奨する4例をすべて包含する（`02_architecture.md` §1.3 D-1） |
| R-6 | `verify` の exit 3 導入により、既存の監視スクリプトが未知の終了コードを受け取る | 誤検知・見逃し | 利用者向け文書が示す判定パターンはいずれも「0 か否か」で分岐するため失敗側へ倒れる（`02_architecture.md` §1.3 D-2）。CHANGELOG にアップグレード前の影響判定手順を記載する |
| R-7 | Phase 6 の英語版反映が日本語版から乖離する | 文書間の不整合 | `/mktrans` を使い、日本語版を先に確定させてから反映する。日本語版と同じ `rg` 検査を英語版にも掛ける（§7 の AC-24・AC-25 の行） |

**スケジュールリスク**

| # | リスク | 影響 | バッファ計画 |
|---|---|---|---|
| S-1 | Phase 2 の `identitymutationguard` 拡張が、既存2つのガードテストの後方互換を壊し、想定より広い修正を要する | PR-2 の完了が遅れる | Phase 2 はフィールドと関数の追加のみで既存シグネチャを変えない設計であるため影響範囲は小さい。仮に後方互換を保てないと判明した場合は、既存2つのガードテストの修正を Phase 2 のスコープ（＝PR-2 の差分）へ追加し、PR-2 の完了を1営業日遅らせる |
| S-2 | Phase 6 の英語版反映（`/mktrans`）と用語集更新が、日本語版の確定待ちで直列化する | 全体の完了が遅れる | §3.2 の「条件付き PR-7」に従い、PR-6 を「日本語版の更新」と「英語版の反映」の2つの PR に分割する。日本語版が確定した時点で先行してレビューへ回す |
| S-3 | Phase 3-A の起動順序ガードテストが、AST 走査の細部で想定以上に試行錯誤を要する | PR-2 の作成が遅れる | 主張1・2（順序判定）と主張3・4（許可リスト・`init()` 個数）は独立しているため、先に主張3・4 を完成させて実装を進め、順序判定は後追いで足す。ただし主張1〜4 とコントロールケースがすべて揃うまで PR-2 を作成しない。主張1・2 は AC-14 の唯一の検証手段であり（§4.2）、これを欠いたまま特権降格の順序変更をマージすると、順序が守られていることを誰も確認できない状態で PR-3 以降が積み上がる。難航した場合は PR-2 の作成を1営業日遅らせ、フェーズを部分的に閉じることはしない |

---

## 6. 実装チェックリスト

### PR-1: run ID の形式定義（Phase 1）

- [x] `internal/logging/runid.go` の新規作成（`MaxRunIDLength`・`MaxRunIDLength` から導出する `RunIDFormatDescription`・`ErrInvalidRunID`・`ValidateRunID`）
- [x] `GenerateRunID` の `safeopen.go` からの移設
- [x] `ErrorTypeInvalidRunID` の追加
- [x] `internal/logging/runid_test.go` の新規作成（既存2テストの移設を含む10テスト）
- [x] `go test -tags test ./internal/logging/...` の成功
- [x] PR-1 マージ済み（対象ステップ: Phase 1）

### PR-2: ガード拡張と起動時特権降格（Phase 2 / Phase 3-A）

- [x] `CallSite.Pos` の追加
- [x] `CallSite.File` の追加と、`Pos` のファイル間比較不可をドキュメントコメントへ明記
- [x] `Options` / `ExtraTrackedFunc` と `*WithOptions` 関数の追加
- [x] `ExtraTrackedFunc.ImportPath` が空のときに非修飾 `*ast.Ident` 呼び出しへ一致させる分岐の追加
- [x] `ProductionGoFiles` の追加と `FindRefs` からの利用
- [x] 既存2つのガードテストの無変更通過
- [x] `dropStartupPrivileges` / `reportStartupPrivilegeFailure` と `startupPrivilegeStage` / `startupPrivilegeError` の追加
- [x] `main()` 先頭の順序変更（降格 → `flag.Parse`）と `syscall.Seteuid` 直接呼び出しの削除。run ID の既定値設定は現行のまま残す
- [x] `cmd/runner/startup_privilege_test.go` の新規作成（4テストと、逐次実行・root スキップに関するコメント）
- [x] `cmd/runner/startup_order_guard_test.go` の新規作成（`//go:build test`、主張1〜4とコントロールケース。主張1・2 を後続 PR へ持ち越さない）
- [x] `go test -tags test ./cmd/runner/...` の成功
- [x] PR-2 マージ済み（対象ステップ: Phase 2 / Phase 3-A）

### PR-3: `--run-id` の入口検証（Phase 3-B）

- [x] `resolveRunID` の追加
- [x] `main()` への `bootstrapID` と `resolveRunID` の挿入、パッケージ変数 `runID` への代入、旧 run ID 既定値設定の削除
- [x] 拒否時の `logging.HandlePreExecutionError` 呼び出し（run ID は `bootstrapID`、メッセージに生値を含めない）
- [x] `-run-id` フラグ説明文字列の更新（`main.go` と `main_test.go` の両方）
- [x] `TestResolveRunID` の追加
- [x] `cmd/runner/integration_pre_execution_error_test.go` への拒否系統合テスト3件の追加
- [x] `cmd/runner/integration_logger_test.go` への `TestE2E_ValidRunIDIsAdopted` の追加
- [x] `make deadcode` の確認（PR-1 が導入し本 PR で到達可能になる `internal/logging` の新規シンボルを含め、未到達の報告がないこと）
- [x] `go test -tags test ./cmd/runner/...` の成功
- [ ] PR-3 マージ済み（対象ステップ: Phase 3-B）

### PR-4: `bootstrap` の多層防御（Phase 4）

- [ ] `SetupLoggerWithConfig` 先頭での `ValidateRunID` 呼び出し
- [ ] `TestSetupLoggerWithConfig_RejectsInvalidRunID` の追加
- [ ] 既存の `bootstrap` テストと `cmd/runner/integration_logger_test.go` の通過
- [ ] PR-4 マージ済み（対象ステップ: Phase 4）

### PR-5: `cmd/verify` の fail-closed 化（Phase 5）

- [ ] 終了コード定数の導入と既存 `return` の置き換え
- [ ] `toctouChecker` パッケージ変数の追加
- [ ] `checkDirPermissions` の切り出しと2集合の分割判定
- [ ] `NewDirectoryPermChecker` 失敗時の `panic` の `checkDirPermissions` への移設
- [ ] 標準エラー出力メッセージ（英語リテラル）と `//nolint:errcheck` の付与
- [ ] `run()` からの呼び出しと fail-closed 分岐
- [ ] `fakeDirPermChecker` / `overrideTOCTOUChecker` の追加
- [ ] fail-closed テスト4件の追加（明示ハッシュディレクトリ・既定ディレクトリ・ERROR ログ・対象ファイル集合の打ち切り）
- [ ] `TestRunTOCTOU_ContinuesOnWorldWritableDir` の改名・コメント更新・経路依存スタブ注入・アサーション追加
- [ ] 既存4テストへの `toctouChecker` 注入
- [ ] `go test -tags test ./cmd/verify/...` の成功
- [ ] PR-5 マージ済み（対象ステップ: Phase 5）

### PR-6: 文書と CHANGELOG（Phase 6）

- [ ] `docs/user/runner_command.ja.md` の `-run-id` 節更新
- [ ] `docs/user/runner_command.ja.md` の誤ったログファイル命名規則3箇所の修正
- [ ] `docs/user/verify_command.ja.md` の終了コード表の新設、既存の終了コード関連2節（§2.4・エラーハンドリング節）への exit 3 の反映、fail-closed 挙動の記載
- [ ] `CHANGELOG.ja.md` の破壊的変更2項目と影響判定手順の追加
- [ ] `docs/translation_glossary.md` への新規用語の追加
- [ ] 英語版3ファイルの反映
- [ ] 文書の記載内容と実装の突き合わせ検証（Phase 6 の「検証」項目）
- [ ] PR-6 マージ済み（対象ステップ: Phase 6）

### 全体（各 PR の作成前に実施）

- [ ] `make fmt` の実行
- [ ] `make test` の成功
- [ ] `make lint` の成功

`make deadcode` は `cmd/record` / `cmd/runner` / `cmd/verify` からの到達可能性を見るため、PR-1 の時点では `internal/logging` の新規シンボルが未到達として報告される（グリーンゲートは `make test && make lint` であり、これは PR-1 のマージを妨げない）。全シンボルが到達可能になるのは PR-3 のマージ後であるため、確認は PR-3 のチェックリストに置いてある。

---

## 7. Acceptance Criteria 検証

各 AC の検証手段を `test`（実行可能。挙動が誤っていれば失敗する）／`static`（`rg`・コンパイル）／`manual`（PR 上での確認）で分類する。

なお `internal/logging/runid_test.go::TestGenerateRunID_Uniqueness`・`::TestGenerateRunID_Format`・`::TestGenerateRunID_SatisfiesValidateRunID` は、特定の AC ではなく `GenerateRunID` の不変条件（一意性・ULID 形式・出力が常に `ValidateRunID` を満たすこと）を固定するテストであり、下表には行を持たない。

| AC | 種別 | 検証場所またはコマンド | 期待結果 |
|---|---|---|---|
| AC-01 | test | `cmd/runner/main_test.go::TestResolveRunID`（サブテスト「flag unset」） | `flagValue == ""` のとき `bootstrapID` が返り、エラーが `nil` |
| AC-02 | test | `cmd/runner/main_test.go::TestResolveRunID`（サブテスト「explicit empty string」） | 明示的な空文字列でも `bootstrapID` が返り、エラーが `nil` |
| AC-03 | test | `cmd/runner/main_test.go::TestResolveRunID`（サブテスト「accepted value」）、`cmd/runner/integration_logger_test.go::TestE2E_ValidRunIDIsAdopted` | 指定値がそのまま返る。E2E では終了コードが 0 で、`-log-dir` に `*_backup-20260805-143000.json` に一致するファイルがちょうど1件生成される |
| AC-04 | test | `internal/logging/runid_test.go::TestValidateRunID_AcceptsAllowedCharacters`、`::TestValidateRunID_RejectsNonAllowlistedValues` | 許可文字のみの値が受理され、それ以外を含む全ケースが `ErrInvalidRunID` で拒否される |
| AC-05 | test | `internal/logging/runid_test.go::TestValidateRunID_RejectsNonAllowlistedValues`（`../../etc/cron.d/evil`・`/tmp/evil`・`..`）、`cmd/runner/integration_pre_execution_error_test.go::TestE2E_PreExecutionError_InvalidRunIDPathTraversal` | 拒否される。E2E では終了コードが 1 |
| AC-06 | test | `internal/logging/runid_test.go::TestValidateRunID_RejectsNonAllowlistedValues`（空白・改行・NUL・ESC）、`cmd/runner/integration_pre_execution_error_test.go::TestE2E_PreExecutionError_InvalidRunIDNewlineInjection` | 拒否される。E2E では終了コードが 1 かつ `RUN_SUMMARY` を含む行が1行だけ |
| AC-07 | test | `internal/logging/runid_test.go::TestValidateRunID_LengthBoundaries`、`cmd/runner/integration_pre_execution_error_test.go::TestE2E_PreExecutionError_InvalidRunIDTooLong` | 長さ `MaxRunIDLength` は受理、`MaxRunIDLength+1` は拒否。E2E では終了コードが 1 |
| AC-08 | test | `internal/logging/runid_test.go::TestErrorTypeInvalidRunID_Token`、`cmd/runner/integration_pre_execution_error_test.go::TestE2E_PreExecutionError_InvalidRunIDPathTraversal` | `ErrorTypeInvalidRunID` のトークン文字列が `invalid_run_id` に固定されている。標準エラー出力に `invalid_run_id` が含まれる。`-log-dir` に渡した一時ディレクトリの `os.ReadDir` が 0 件、かつその直下に `etc` エントリが存在しない |
| AC-09 | test | `internal/logging/runid_test.go::TestValidateRunID_ErrorOmitsRejectedValue`、`::TestValidateRunID_ErrorIdentifiesFirstViolatingByte`、`cmd/runner/integration_pre_execution_error_test.go::TestE2E_PreExecutionError_InvalidRunIDPathTraversal` | エラー文字列と標準出力・標準エラー出力の全文のいずれにも入力値 `../../etc/cron.d/evil` が部分文字列として現れない（`ErrorOmitsRejectedValue` は §3.1 の診断契約の「含まない」側、`ErrorIdentifiesFirstViolatingByte` は「違反バイトの位置と `%q` 表現を含む」側を検証する）。`RUN_SUMMARY` 行の `run_id` が `logging.ValidateRunID` を通過する |
| AC-10 | test | `internal/logging/runid_test.go::TestRunIDFormatDescription_ReflectsMaxRunIDLength`、`cmd/runner/integration_pre_execution_error_test.go::TestE2E_PreExecutionError_InvalidRunIDPathTraversal` | `RunIDFormatDescription` が `MaxRunIDLength` を反映している。標準エラー出力が `logging.RunIDFormatDescription()` の文字列を含む |
| AC-11 | test | `internal/runner/bootstrap/logger_test.go::TestSetupLoggerWithConfig_RejectsInvalidRunID` | `RunID` が `../evil`・`/tmp/evil` のときエラーが返り、`LogDir` の `os.ReadDir` が 0 件 |
| AC-12 | test | `internal/runner/bootstrap/logger_test.go::TestSetupLoggerWithConfig_RejectsInvalidRunID` | 同テストは `SetupLoggerWithConfig` を直接呼ぶため、入口検証を経ない呼び出しでも防御が働くことを示す |
| AC-13 | test | 既存テストの通過: `internal/runner/bootstrap/logger_test.go` 全体、`internal/runner/bootstrap/environment_test.go` 全体、`cmd/runner/integration_logger_test.go` 全体 | `go test -tags test ./internal/runner/bootstrap/... ./cmd/runner/...` が成功し、これらのファイルの `RunID` 値に差分がない |
| AC-14 | static | `cmd/runner/startup_order_guard_test.go::TestStartupPrivilegeDropOrder` | 主張1（`dropStartupPrivileges` 内で `Setegid` の位置 < `Seteuid` の位置）、主張2（`main` 内で `dropStartupPrivileges` の位置 < `flag.Parse` の位置）、主張3（識別子変更系の呼び出しが上記2件のみ、値参照が0件）、主張4（`init` が1個）がすべて成立。逆順の合成ソースでは判定が失敗することをコントロールケースで確認 |
| AC-15 | test | `cmd/runner/startup_privilege_test.go::TestDropStartupPrivileges_FailsClosedOnSetegidFailure` | `Stage == stageSetegid` のエラーが返り、`syscall.Geteuid()` が呼び出し前から変化しない（`Seteuid` へ進んでいない）。非特権ユーザーでのみ実行され、実効ユーザーIDが 0 のときは `t.Skip` |
| AC-16 | test | `cmd/runner/startup_privilege_test.go::TestDropStartupPrivileges_FailsClosedOnSeteuidFailure` | `Stage == stageSeteuid` のエラーが返り、`syscall.Geteuid()` が変化しない |
| AC-17 | test | 既存 `cmd/runner/main_test.go::TestShortFlags`、`::TestShortFlagsEquivalence` | アサーション無変更で通過する |
| AC-18 | test | `cmd/runner/startup_privilege_test.go::TestReportStartupPrivilegeFailure_UsesValidRunID` | 標準出力の `RUN_SUMMARY` 行の `run_id` が空でなく `logging.ValidateRunID` を通過する。戻り値が非0 |
| AC-19 | test | `cmd/verify/main_test.go::TestRunFailsClosedOnHashDirViolation_ExplicitHashDir`、`::TestRunSkipsTargetSetCheckWhenHashDirViolates` | 終了コードが `exitUntrustedEnvironment`、`validator.calls` が 0 件、標準出力が空。対象ファイル集合の権限チェックも行われない（対象ファイル側パスの WARN 行が 0 件） |
| AC-20 | test | `cmd/verify/main_test.go::TestRunFailsClosedOnHashDirViolation_LogsErrorLevel` | 捕捉ログに違反ごとの ERROR 行（`path`・`remediation` 属性つき）と、共有チェックの WARN 行の両方が現れる |
| AC-21 | test | `cmd/verify/main_test.go::TestRunFailsClosedOnHashDirViolation_ExplicitHashDir` | 標準エラー出力が `"verification results cannot be trusted"` と `"Fix directory permissions"` を含む |
| AC-22 | test | 既存 `cmd/verify/main_test.go::TestRunProcessesMultipleFiles`、`::TestRunReportsFailuresAndContinues`、`::TestRunWarnsWhenDeprecatedFlagUsed`、`::TestRunUsesDefaultHashDirectoryWhenNotSpecified` | アサーション無変更で通過する（追加するのは `toctouChecker` のスタブ注入のみ） |
| AC-23 | test | `cmd/verify/main_test.go::TestRunFailsClosedOnHashDirViolation_ExplicitHashDir`（`-hash-dir` 明示）、`::TestRunFailsClosedOnHashDirViolation_DefaultHashDir`（既定ディレクトリ） | 両ケースとも終了コードが `exitUntrustedEnvironment`、`validator.calls` が 0 件 |
| AC-28 | test | `cmd/verify/main_test.go::TestRunTOCTOU_ContinuesWhenOnlyTargetDirViolates` | 対象ファイルの祖先のみに違反がある構成で、終了コードが 0（`exitUntrustedEnvironment` ではない）、`validator.calls` が 1 件 |
| AC-24 | static + manual | static: `rg -n "1〜64文字" docs/user/runner_command.ja.md`、`rg -n -e "起動前に拒否" -e "実行を開始せず" docs/user/runner_command.ja.md`、`rg -n "1-64 characters" docs/user/runner_command.md`。manual: Phase 6 の「検証」項目で `logging.RunIDFormatDescription()` および `logging.MaxRunIDLength` と記述内容を突き合わせる | static の3コマンドがいずれも1件以上ヒットする（日本語版は `-run-id` 節、英語版は対応する節）。manual では受理形式の記述が定数と一致している |
| AC-25 | static | `rg -n -e "exit 3" -e "終了コード 3" docs/user/verify_command.ja.md`、`rg -n "対象ファイル" docs/user/verify_command.ja.md`、`rg -n "exit 3" docs/user/verify_command.md` | 日本語版・英語版とも終了コード表に 3 の行が存在し、日本語版に対象ファイル側のみの違反では警告のうえ検証が継続する旨の記述が1件以上ヒットする |
| AC-26 | static | `rg -n -A2 "^### 破壊的変更" CHANGELOG.ja.md \| head -40` と `rg -n "run-id" CHANGELOG.ja.md`、`rg -n "verify" CHANGELOG.ja.md` | `## [未リリース]` 節に `### 破壊的変更` が存在し、`--run-id` の形式厳格化と `verify` の fail-closed 化の2項目、および影響判定手順が記載されている。`CHANGELOG.md` にも同じ2項目が存在する |
| AC-27 | static | `git diff --stat docs/translation_glossary.md` | 本タスクで新規に導入した用語がある場合は差分が存在する。新規用語がない場合は、その判断を PR 本文に明記したうえで差分なしを許容する |

---

## 8. 成功基準

### 8.1 機能の完全性

- [ ] `01_requirements.md` の AC-01〜AC-28（欠番なし）がすべて §7 の表のとおり検証されている。
- [ ] `02_architecture.md` §3.7 のコンポーネント責務表に挙げられたファイルがすべて、記載された区分（新規・変更・回帰確認のみ）どおりに扱われている。

### 8.2 品質

- [ ] `make test` が成功する。
- [ ] `make lint` が警告なく通過する。
- [ ] `make deadcode` が成功する。
- [ ] 新規追加した関数のうち、エラー経路を持つものはすべてエラー経路のテストを持つ。

### 8.3 セキュリティ

- [ ] `cmd/runner` の製品コードに、識別子変更系 syscall への値参照（`ValueRef`）が0件である（`cmd/runner/startup_order_guard_test.go` の主張3）。
- [ ] `--run-id` に与えた不正値が、標準出力・標準エラー出力・ログのいずれにも原文のまま現れない。
- [ ] `verify` がハッシュディレクトリ側の違反を検出した場合、`Verify` を1件も呼ばない。

### 8.4 後方互換性

- [ ] 受理形式に合致する `--run-id` を指定する運用、および `--run-id` を指定しない運用で、外部から観測可能な挙動が変わらない。
- [ ] ハッシュディレクトリ側に違反が検出されない `verify` の運用で、外部から観測可能な挙動が変わらない（`sudo verify` で利用者のホームディレクトリ配下のファイルを検証する運用を含む）。

### 8.5 文書

- [ ] AC-24〜AC-27 の検証がすべて通過する。
- [ ] 日本語版と英語版の章立てが一致している。

---

## 9. 横断検索チェックリスト

`make lint` と `make test` では検出できない残存参照と整合性を、実装完了後に確認する。

- [ ] `rg -n "GenerateRunID" --type go` の結果が、`internal/logging/runid.go` の定義、`internal/logging/runid_test.go` のテスト、`cmd/runner/main.go` の呼び出しのみであり、`internal/logging/safeopen.go` と `internal/logging/safeopen_test.go` に残っていない。
- [ ] `rg -n "AC-M2S-7" --type go` の結果が 0 件である（`cmd/verify/main_test.go` のコメントから除去済み）。
- [ ] `rg -n "TestRunTOCTOU_ContinuesOnWorldWritableDir" -g '!docs/**'` の結果が 0 件である（改名の取りこぼしがない）。
- [ ] `rg -n "does NOT abort on TOCTOU|only logs a warning" cmd/verify/` の結果が 0 件である（fail-open を前提とした古いコメントが残っていない）。
- [ ] `rg -n -e "runner-<run-id>" -e "runner-01K" docs/` の結果が 0 件である（誤ったログファイル命名規則の記述が日本語版・英語版のいずれにも残っていない）。
- [ ] `rg -n "auto-generates ULID if not provided" --type go` の結果が `cmd/runner/main.go` と `cmd/runner/main_test.go` の2箇所であり、両ファイルの `-run-id` 登録式（`logging.RunIDFormatDescription()` を含む連結式）がトークン単位で同一である。
- [ ] `rg -n -e "AC-[0-9]" -e "F-[0-9]" --type go` の結果に、本タスクで追加・変更した Go ファイルが含まれていない（要件プロセスガイド §4 が禁じる `AC-NN` / `F-NNN` 参照を持ち込んでいない）。
- [ ] `docs/translation_glossary.md` に追加した用語が、`docs/user/runner_command.ja.md`・`docs/user/verify_command.ja.md`・`CHANGELOG.ja.md` の日本語表記と一致している。

---

## 10. 次のステップ

1. 本実装計画書のレビューを受け、Status を `approved` に更新する。
2. `approved` 後、§3.2 の PR 構成に従い PR-1（Phase 1）から着手する。PR-5（Phase 5）は他のどの PR にも依存しないため PR-1 と並行して着手してよく、PR-4（Phase 4）も PR-1 のマージ後であれば PR-2・PR-3 と並行して着手してよい。
3. 各 PR の完了時に §6 の該当 PR のチェックボックスを更新し、`make fmt` → `make test` → `make lint` を実行する。各 `### PR-N 作成ポイント` でグリーンゲートを確認してから PR を作成し、マージ後に次のブランチへ切り替える。
4. Phase 6 の完了後、`02_architecture.md` §3.4 に記録した L-2 のリスク（誤検知が exit 3 を生む）を [#986](https://github.com/isseis/go-safe-cmd-runner/issues/986) へ追記し、優先度の見直しを依頼する。
