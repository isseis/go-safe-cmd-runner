# 実装計画書: entrypoints の残 Low/Info 所見（パス解決・起動処理の整理）

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-08-14 |
| Review date | 2026-08-14 |
| Reviewer | isseis |
| Comments | AC-31 の解釈は §1.5 で決定済み。残る二重出力は [#1020](https://github.com/isseis/go-safe-cmd-runner/issues/1020) へ分離した。レビュー指摘により、AC-31 の選別条件（`level=ERROR` → `error_message=`）、AC-27 の検索式（`isec` 別名）、ステップ 4-3 の除外範囲、`deps.resolvePathForCheck` の宣言、AC-01・AC-15 の現状件数を修正済み。さらに PR レビュー指摘により、権限チェッカの注入口を引数無しのファクトリ `newPermChecker func() (security.DirectoryPermChecker, error)` に一本化し、`toctouChecker` を廃止した。承認後に PR 境界（§3.2、全7件）を追加し、フェーズ1とフェーズ4のステップ順を PR 単位に整理した |

## 関連文書

- 要件定義書: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計書: [02_architecture.md](02_architecture.md)
- 先行タスク: [0162](../0162_entrypoint_runid_privilege_toctou_hardening/03_implementation_plan.md)
- テストヘルパー配置規約: [test_organization.md](../../dev/developer_guide/test_organization.md)

---

## 1. 実装の全体像

### 1.1 目的

`runner`・`record`・`verify` の起動処理に残る 11 件の監査所見（L-1〜L-5・L-7・I-1〜I-5）を解消する。設計の根拠と決定内容は 02_architecture.md に記述済みであり、本書は作業手順と検証手段のみを扱う。

### 1.2 実装方針

- 共有処理（`internal/security`・`internal/cmdcommon`・`internal/fileanalysis`・`internal/filevalidator`）を先に追加し、利用側の3コマンドを後から移す（02_architecture.md §8 のフェーズ分割に従う）。
- Go のコメント・識別子・文字列リテラルはすべて英語で書く。テストコードも同じ規則に従う。
- 各フェーズの完了時に `make fmt` → `make test` → `make lint` を実行し、緑であることを確認してから次のフェーズへ進む。
- 日本語版の文書を先に更新し、英語版は `/mktrans` で反映する。

### 1.3 既存コード調査結果

実装前に対象箇所を全件確認した。以下では領域ごとに、今どうなっているか、足りないものは何か、どう変えるかを示す。

#### 1.3.1 パス解決（F-001・F-004）

| 対象 | 現状 | 変更内容 |
|---|---|---|
| `internal/security/path_resolution.go:15` `ResolveAbsPathForCheck` | 絶対パスでなければ `("", false)`、`EvalSymlinks` 失敗時は入力パスをそのまま返す | 削除し、`ResolvePathForCheck`・`ResolveAllForCheck`・`ClassifyCheckTarget` に置き換える |
| `cmd/runner/main.go:414` `resolveStaticAbsPath` | `%{` を含むパスを弾き、`ResolveAbsPathForCheck` に委譲 | 削除し、`ClassifyCheckTarget` + `ResolvePathForCheck` に置き換える |
| `cmd/runner/main.go:457` | `ResolveAbsPathForCheck(cmdcommon.DefaultHashDirectory)` | `ResolvePathForCheck` に置き換える |
| `internal/runner/group_executor.go:357,364` | `ResolveAbsPathForCheck` を直接呼ぶ | `ClassifyCheckTarget` + `ResolvePathForCheck` に置き換える |
| `cmd/record/main.go:113-132` | 対象ファイル群とハッシュディレクトリを個別に `filepath.Abs` + `EvalSymlinks`（約20行） | `ResolveAllForCheck`（ファイル群）と `ResolvePathForCheck`（ハッシュディレクトリ）に置き換える |
| `cmd/verify/main.go:91-98,126-137` | 同上の重複 | 同上 |

本番コードで `filepath.EvalSymlinks` を呼ぶのは上記4ブロック（`cmd/record/main.go:119,127`・`cmd/verify/main.go:93,132`）のみである。`cmd/runner` の一致はテストファイル6件（`integration_workdir_test.go:333,339`・`integration_cmd_allowed_security_test.go:68,135`・`integration_workdir_unix_test.go:21,24`）とコメント1件（`cmd/runner/main.go:456`）で、テスト側は本タスクの対象外である。

`ResolveAbsPathForCheck` の単体テストは `internal/security/path_resolution_test.go` に存在しない（`rg -n -e ResolveAbsPathForCheck internal/security/path_resolution_test.go` が無出力）。02_architecture.md §3.6 の責務表は同テストの更新を挙げているが、実体は無いため、削除ではなく新規追加のみを行う。

#### 1.3.2 権限チェッカ生成（F-006）

`security.NewDirectoryPermChecker()` の失敗時に同一内容の panic を持つ箇所は3件（`cmd/runner/main.go:446-451`・`cmd/record/main.go:105-112`・`cmd/verify/main.go:81-90`）。`internal/security/dir_permissions_unix.go:34` の現行実装は常に `nil` エラーを返すため、この panic は到達不能である。したがって AC-24・AC-26 を検証するには、チェッカ生成そのものを差し替えられる注入口が本番コード側に要る（ステップ 2-2・3-2・4-2 でこれを追加する）。

#### 1.3.3 `verify` の書き込み副作用と依存注入（F-003・F-010）

- `cmd/verify/main.go:221` の `mkdirAll(dir, hashDirPermissions)` に加え、`cmdcommon.CreateValidator`（`internal/cmdcommon/common.go:14`）が `filevalidator.New` 経由でディレクトリを作成する。両方を断たないと AC-14 は成立しない。
- `cmdcommon.CreateValidator` の本番呼び出し元は `cmd/verify/main.go:40` のみ。テストは `internal/cmdcommon/common_test.go` に5件（`TestCreateValidator_ValidHashDirectory`・`_DefaultHashDirectory`・`_NonExistentDirectory`・`_RelativePath`・`_EmptyPath`）。
- `filevalidator.NewReadOnly`（`internal/filevalidator/validator.go:285`）は既に用途に合致する。不在時は `ErrHashDirNotExist` を、権限不足時は生の `os.ErrPermission` 系エラーを `deferredErr` に載せて構築に成功する。
- `Validator.HashDirAvailable()`（同 361 行）は `v.deferredErr == nil` を返すだけなので、`HashDirError()` を追加して `HashDirAvailable()` をその上に再定義できる。`HashDirAvailable` の呼び出し元は `internal/verification/manager.go` の1件と、`internal/filevalidator/validator_test.go`・`validator_error_test.go` の計4件で、いずれも挙動が変わらないため修正不要。
- `cmd/verify` のパッケージレベル可変変数は `validatorFactory`・`mkdirAll`・`ensurePermissionCheckUID`・`toctouChecker` の4つ。テストはこれらを差し替えるヘルパー（`overrideValidatorFactory`・`overrideTOCTOUChecker`）と直接代入（`cmd/verify/main_test.go:186-190,372-374,469-471`）に依存しており、`cmd/verify/main_test.go` の全 15 テストが書き換え対象になる。
- `cmd/verify/main.go:80-152` の `checkDirPermissions` は、02_architecture.md §6.2 の段階 3（ハッシュディレクトリ側）と段階 5（対象ファイル側）を1つの関数で行っている。§6.2 の順序を実現するには両者を分ける必要がある（ステップ 3-4）。

#### 1.3.4 ハッシュディレクトリのパーミッション（F-003）

| 場所 | 現在の値 |
|---|---|
| `cmd/record/main.go:37` `hashDirPermissions` | `0o700` |
| `cmd/verify/main.go:19` `hashDirPermissions` | `0o750` |
| `internal/fileanalysis/file_analysis_store.go:19` `dirPermission` | `0o750`（コメントは 18 行目と 32 行目） |

`record` が作るサブディレクトリは `internal/libccache/cache.go:38`・`internal/libccache/macho_cache.go:28`・`internal/dynamicanalysis/store.go:35` の3件で、いずれも `cacheDirPerm`／`storeDirPerm`（`0o755`）。これらは変更しない。

`record` はハッシュディレクトリを暗黙にも作る。`d.validatorFactory` → `filevalidator.New`（`internal/filevalidator/validator.go:196`）→ `fileanalysis.NewStore` が `os.MkdirAll` を呼ぶためである。AC-08（違反時に作られない）と AC-11 が成立するのは、権限チェックが `run` の中で `validatorFactory` より前に置かれているからであり、この順序は実装上の前提条件である（ステップ 2-3）。

#### 1.3.5 `record` の重複計算（F-010 / AC-40）

`cmd/record/main.go:192` の `cacheDir` と `cmd/record/main.go:201` の `machoCacheDir` はいずれも `filepath.Join(cfg.hashDir, libcCacheSubDir)` であり、値が同一である。

#### 1.3.6 `runner`・`bootstrap`（F-005・F-007・F-008・F-009・F-011）

| 対象 | 現状 |
|---|---|
| `internal/runner/bootstrap/logger.go:181` | `time.Now().Format("20060102T150405Z")`。`Z` はリテラルでローカル時刻 |
| `internal/runner/bootstrap/logger.go:242` | `fmt.Sprintf("%s_%s_%s.json", hostname, timestamp, config.RunID)` |
| `internal/runner/bootstrap/config.go:42` | IPv6 分岐の `bareHost = u.Hostname()`（小文字化なし） |
| `cmd/runner/main.go:284` | `fmt.Fprintln(os.Stderr, err.Error())` と `PreExecutionError` 返却で二重出力 |
| `cmd/runner/main.go:572-578` | `resource.Formatter` を選ぶ `switch` に `default` が無い |
| `cmd/runner/main.go:166` | `dropStartupPrivileges(syscall.Getuid(), syscall.Getgid())` の呼び出し（AC-42 のコメント追記先） |

`ValidateSlackWebhookEnv`（`internal/runner/bootstrap/environment.go:49`）がエラーを返すのは `GSCR_SLACK_WEBHOOK_URL_SUCCESS` のみが設定されている場合だけで、返るのは複数行の `SuccessWithoutErrorError`（同 28-40 行）である。既存の E2E テスト `TestE2E_PreExecutionError_MissingSlackAllowedHost` は `GSCR_SLACK_WEBHOOK_URL_ERROR` のみを設定しており、この検証を通過してから設定側で失敗する別経路なので、AC-31 の根拠にはならない。この経路の出力回数については §1.5 を参照。

#### 1.3.7 文書

| 文書 | 該当箇所 | 現状と対応 |
|---|---|---|
| `docs/user/verify_command.ja.md` | 97-104 行 | 終了コード表。`3` は TOCTOU 権限違反のみ。改訂する |
| 同上 | 208 行 | 「ハッシュディレクトリが存在しない場合はエラーになります」（実装が未追随）。終了コードとトークンを補う |
| 同上 | 641-680 行付近 | `robust-verification.sh` 例。終了コード 3 を権限違反と決め打ち。改訂する |
| `docs/user/runner_command.ja.md` | 849・854 行 | 命名規則と例。例は `myhost_20260805140000_...` で、実際の `T` 区切り・`Z` 付き書式と一致していない。修正する |
| `docs/user/record_command.ja.md` | 199-206 行 | 「指定したディレクトリが存在しない場合、自動的に作成されます（権限: 0700）」。作成時期がステップ 2-3 で変わり、ステップ 2-5 で拒否条件が増えるため、条件付きの記述に改める |
| 同上 | 211 行・531 行 | `0700` 記載と 0146 由来の `0o750` からの是正手順。いずれも本タスクでは変更しない（531 行は §8 の `0o750` 検索で意図的な残存として扱う） |
| `docs/user/*.md`（英語版） | 上記の対応箇所 | `/mktrans` で反映する |
| `CHANGELOG.ja.md` / `CHANGELOG.md` | `## [未リリース]` 節 | 0162 由来の記述あり。本タスク分を追記する |

### 1.4 本計画で確定させた実装詳細

02_architecture.md が「実装計画で定める」とした項目、および設計から一意に決まらない項目をここで確定する。

#### 1.4.1 `verify` の識別トークン（02_architecture.md §4.2）

検証を1件も行わずに終わった理由を機械的に判別できるよう、標準エラー出力の各メッセージに `verify-error=<token>` の形で固定トークンを含める。前半5件は終了コード `3` の原因を分けるためのもので、後半4件は終了コード `1` に付く（表の右列と、表の下の実装時の追加を参照）。トークンは以下の9種類とする。

| トークン | 原因 | 終了コード |
|---|---|---|
| `hash_dir_permission_violation` | ハッシュディレクトリ側の TOCTOU 権限違反 | 3 |
| `path_resolution_failed` | ハッシュディレクトリのパス解決に失敗 | 3 |
| `hash_dir_not_found` | ハッシュディレクトリが存在しない | 3 |
| `hash_dir_unreadable` | 存在するが開けない（権限不足など） | 3 |
| `permission_checker_init_failed` | 権限チェッカの初期化に失敗 | 3 |
| `hash_dir_not_a_directory` | ハッシュディレクトリのパスがディレクトリでない | 1 |
| `invalid_arguments` | 検証対象のファイルが指定されていない、または引数の解析に失敗 | 1 |
| `permission_check_uid_unresolved` | 権限チェックの判定主体となる UID を確定できない | 1 |
| `validator_init_failed` | バリデータを構築できない（解析ストアを開けないなど） | 1 |

**実装時の追加（レビュー指摘、PR-5）**: 終了コード `1` で終わる残り3件（`invalid_arguments`・`permission_check_uid_unresolved`・`validator_init_failed`）を追加した。いずれも検証を1件も行わずに終わる経路であり、トークンが無ければ「検証を1件も行わずに終わった理由には必ずトークンが付く」という上記の位置づけが成り立たず、呼び出し元は環境の不備を改ざんの検出と読み違える。

**実装時の追加（レビュー指摘、ステップ 3-4）**: `hash_dir_not_a_directory` を6番目として追加した。終了コード `1` は通常の検証失敗（＝改ざんの検出）と同じ値であり、トークンが無ければ呼び出し元は設定ミスとの区別を地の文の照合でしか行えない。トークンを地の文の照合の代わりに置くのが本 PR の目的であるから、終了コードが `1` であることを理由にトークンを省く合理性は無い。したがってトークンは「終了コード 3 の原因を分ける印」ではなく「検証を1件も行わずに終わった理由の印」と位置づける。

**実装時の追加（ステップ 3-4）**: `path_resolution_failed` を5番目として追加した。ステップ 3-4 が「返るエラーに固有の識別を与える」としていた対象であり、AC-05 の2本のテスト（実権限による経路と注入による経路）が同じ出力を根拠にできるのは、パス解決の失敗それ自体が fail-closed の原因として独立している場合に限られるためである。理由は 02_architecture.md §3.1 の失敗表の最終行と同じで、未実在部分に `..` を含むパスでは解決結果が入力とは別の木の健全な祖先になり得るため、それを検査して通過させると fail-open になる。

トークンは `cmd/verify/main.go` に名前付き定数として定義し、テストと利用者向け文書がその定数と同じ文字列を参照する。テスト側でリテラルを二重定義しない。**実装時の変更**: 定数名は `token...` ではなく `cause...`（`causeHashDirNotFound` など）とした。`gosec` の G101 が識別子名に `token` を含む文字列定数を「ハードコードされた資格情報の疑い」として報告し、`make lint` が通らないためである。

#### 1.4.2 `RunTOCTOUPermissionCheck` の戻り値（02_architecture.md §6.3）

§6.3 は起動時チェックのログに「実際に検査した数」と「存在せず読み飛ばした数」を含めることを求めるが、現行の `RunTOCTOUPermissionCheck` は違反のみを返すため、呼び出し側からはこの2つが分からない。

**決定**: 戻り値を集約した構造体に変える。

```go
// Package security

// TOCTOUCheckResult reports the outcome of a directory permission check run.
type TOCTOUCheckResult struct {
    Violations []TOCTOUViolation
    Checked    int // directories that existed and were inspected
    Skipped    int // directories that did not exist and were skipped
}

func RunTOCTOUPermissionCheck(checker DirectoryPermChecker, dirs []string, logger *slog.Logger) TOCTOUCheckResult
```

計数規則を次のとおり定める。現行の `internal/security/toctou.go:32-63` は正常経路に分岐を持たないため、規則を明示しないと実装が割れる。

- `ValidateDirectoryPermissions` が `nil` を返した場合: `Checked` を 1 増やす。
- `fs.ErrNotExist` を返した場合: `Skipped` を 1 増やす（`Checked` は増やさない）。
- それ以外のエラー（違反）を返した場合: `Checked` を 1 増やし、`Violations` に追加する。検査は行われているためである。

呼び出し側で件数を数え直す案（収集したディレクトリを個別に `os.Stat` する）は採らない。チェック本体と別に存在判定を行うことになり、両者が食い違い得るためである。これは戻り値の形の変更であって判定規則の変更ではないので、要件定義書がスコープ外とする「TOCTOU 権限チェック処理そのものの挙動変更」には当たらない。

呼び出し元は `cmd/record/main.go:144`・`cmd/verify/main.go:105,153`・`cmd/runner/main.go:459`・`internal/runner/group_executor.go:371` の5箇所、テストは `internal/security/toctou_test.go:28,48,72,82` の4箇所。

#### 1.4.3 起動時チェックのログ属性名（02_architecture.md §6.3）

`runTOCTOUCheck` が `INFO` で記録する属性名を次のとおり固定する。テストはこの名前で検証する。

| 属性名 | 数える単位 | 意味 |
|---|---|---|
| `collected_dirs` | ディレクトリ | `CollectPermissionCheckDirs` が返した数 |
| `checked_dirs` | ディレクトリ | 実際に検査された数 |
| `skipped_missing_dirs` | ディレクトリ | 存在せず読み飛ばされた数 |
| `skipped_variable_reference_paths` | 設定パス | `CheckSkipVariableReference` による除外件数 |
| `skipped_relative_paths` | 設定パス | `CheckSkipRelative` による除外件数 |

前3つと後2つは数える単位が異なる。`CollectPermissionCheckDirs` は各パスをその祖先すべてに展開して重複を除く（`internal/security/check_targets.go:77-122`）ため、`collected_dirs` は設定に書かれたパスの数ではない。一方、除外は展開より前の設定パスに対して起きる。`collected_dirs = checked_dirs + skipped_missing_dirs` は成り立つが、除外件数はこの等式に加わらない。属性名の `_paths` 接尾辞はこの違いを示すためのものであり、利用者向けの説明を書く場合も同じ区別を明記する。

メッセージは `"startup TOCTOU permission check completed"` とする。

#### 1.4.4 権限チェッカ生成とパス解決の注入口

`security.NewDirectoryPermChecker()` が常に成功する以上（§1.3.2）、AC-24〜AC-26 は生成関数そのものを差し替えなければ検証できない。02_architecture.md §7.1 はこれを求めており、同 §3.4 の `deps` 構造体にも次のフィールドを置く。

```go
// cmd/record and cmd/verify: added to deps
// newPermChecker builds the directory permission checker.
newPermChecker func() (security.DirectoryPermChecker, error)
```

**実装時の決定（レビュー指摘による）**: `verify` ではこのフィールドと `resolvePathForCheck` を必須とし、`nil` の場合の既定へ倒す分岐は置かない。`defaultDeps()` が常に値を入れ、テストもそこから組み立てるため、その分岐はどのテストからも到達できない死んだコードになる（同じ構造体の `validatorFactory` にも既定へ倒す分岐は無い）。`record` の同名フィールドは既存テストが `deps{}` を渡すため既定へ倒す分岐が生きており、そちらは変更しない。

このフィールドは、`verify` のパッケージレベル可変変数 `toctouChecker`（および `deps` に同名のフィールドを置く案）を置き換えるものであり、両者は併存させない。チェッカの実体を差し替える口と生成関数を差し替える口は同じ目的を果たすうえ、実体だけを差し替えると本番の生成経路を迂回してしまうためである。テストは `d.newPermChecker` に偽のチェッカを返す実装を与えることで差し替える。

`runner` は `deps` を持たないため、02_architecture.md §3.3 のとおり `runTOCTOUCheck` の引数として同じシグネチャの関数を受け取る。呼び出し元（`cmd/runner/main.go:401`）は `security.NewDirectoryPermChecker` をそのまま渡し、`runTOCTOUCheck` は引数無しでそれを呼ぶ（nil を渡す必要はもう無い）。3コマンドとも既定値が同一の生成関数を指す状態を保ち、AC-27 を弱めないためである。

同じ理由で、`verify` の `deps` にはパス解決関数も持たせる。AC-05（fail-closed 終了時にパス解決の失敗を出力する）の根拠のうち、実際に権限を落とす `TestRunFailsClosedReportsPathResolutionFailure` は root 実行時にスキップされるため、権限に依存しない根拠として `ErrPathResolution` を確実に返す実装を注入する必要がある（ステップ 3-5 の `TestRunFailsClosedReportsInjectedPathResolutionFailure`）。差し替え口が無ければ、この経路は root で1本も検証されない。

```go
// cmd/verify: added to deps
// resolvePathForCheck resolves a path for the TOCTOU permission check.
resolvePathForCheck func(path string) (string, error)
```

`checkHashDirPermissions` と `checkTargetFilePermissions` は `security.ResolvePathForCheck` を直接呼ばず、このフィールド経由で呼ぶ（既定値は `defaultDeps()` が与える）。`record` と `runner` には対応する AC が無いので追加しない（YAGNI）。

これらの追加は 02_architecture.md §3.4 の構造体定義を2フィールド拡張する。設計の意図（§7.1）とは整合しており、§3.4 の記載は追補済みである。

#### 1.4.5 `os.Getwd` の差し替え口

02_architecture.md §7.2 が挙げるパス解決の4経路目（絶対パス化の失敗）は `os.Getwd` が失敗しないと通らず、実運用では作業ディレクトリが削除された場合などに限られる。差し替え口が無ければこの分岐は1本も検証できない。

**決定: 差し替え口は `//go:build test` を付けたファイルにのみ置き、本番ビルドには差し替えられる値を残さない**。パッケージレベルに `var getwd = os.Getwd` を1つ置く形は採らない。その形では本番ビルドにも書き換え可能な変数が残り、パス解決という権限チェックの前提を、テスト以外の場所から差し替えられる状態になるためである。代わりに、`internal/security` に同名の `getwd` をビルドタグで排他的に定義する2ファイルを置く。

```go
// getwd.go — //go:build !test
// 本番ビルド。差し替え口は無い。
func getwd() (string, error) { return os.Getwd() }
```

```go
// test_helpers_getwd.go — //go:build test
// テストビルドのみ。ResolvePathForCheck の絶対パス化失敗の分岐へ到達するための差し替え口。
var getwdHook = os.Getwd

func getwd() (string, error) { return getwdHook() }
```

`ResolvePathForCheck` はどちらのビルドでも `getwd()` を呼ぶだけであり、本番コードには関数値そのものが存在しない。テストは `getwdHook` に失敗する実装を代入し、`t.Cleanup` で元へ戻す。テスト用ファイルの名前と `//go:build test` の付与は、テストヘルパー配置規約（`test_helpers_<category>.go`）に従う。

この形は「テストのときだけ差し替えられる」ことをビルドタグで保証する点で、`deps.resolvePathForCheck`（§1.4.4）が構造体フィールドで保証しているのと同じ考え方である。両者を使い分ける理由は次のとおり。`verify` の `deps` は既に存在する差し替え口の集合であり、フィールドを1つ足すだけで済む。一方 `ResolvePathForCheck` は3コマンドが共有するパッケージ関数で、引数や構造体を持たないため、同じ手はそのままでは使えない。

**本番ビルドの確認**: `make lint` は `--build-tags test` で走るため、`//go:build !test` 側のファイルは lint されない。`make build`（本番バイナリのビルド）がこのファイルを唯一コンパイルする経路なので、フェーズ1の完了ゲートに `make build` を入れる。

### 1.5 AC-31 の「1回だけ」の範囲

**決定: 人間向けブロックに1回とする**。02_architecture.md §6.5 は、`cmd/runner/main.go:284` の直接出力を消せば出力が1回になるとしている。しかし実測すると、そうはならない。

`logging.handleErrorCommon`（`internal/logging/pre_execution_error.go:99-122`）は、同じ `errorMsg` を2回標準エラー出力へ書く。

1. `fmt.Fprintf(&stderrBuilder, "  Details: %s\n", params.errorMsg)` → 標準エラー出力。
2. `slog.Error(..., slog.String(...ErrorMessage, params.errorMsg), ...)` → 既定ロガー。

`ValidateSlackWebhookEnv` は `bootstrap.SetupLogging`（`cmd/runner/main.go:293`）より前の `cmd/runner/main.go:282` で呼ばれるため、2 の時点で既定ロガーは Go 標準のハンドラであり、出力先は標準エラー出力である。`SuccessWithoutErrorError` は複数行だが、`GSCR_SLACK_WEBHOOK_URL_SUCCESS is set but GSCR_SLACK_WEBHOOK_URL_ERROR is not.` という一文は改行を含まないため、構造化ログ行の中にも連続した部分文字列として現れる。

したがって現状の出現回数は3回、ステップ 4-7 を行うと2回になる。

**本タスクは AC-31 の「標準エラー出力に1回」を「人間向けブロックに1回」と解釈する**。ステップ 4-7 は 02_architecture.md §6.5 のとおり直接出力の削除にとどめ、`internal/logging` には手を入れない。同パッケージは起動前エラーと実行時エラーの両方が通る共有経路であり、そこへの変更は本タスクのスコープ（エントリポイント3バイナリの起動処理）から外れるためである。

構造化ログ行に本文がもう一度現れる点は、[#1020](https://github.com/isseis/go-safe-cmd-runner/issues/1020) として分離した。同 issue が解消すれば、構造化ログ行を含めても1回になる。

この解釈のもとで AC-31 が検証可能であるためには、人間向けブロックと構造化ログ行を機械的に分けられる必要がある。ここで分離の根拠に使えるのは、**この時点の既定ロガーが `slog` の組み込み既定ハンドラである**という事実である。`bootstrap.SetupLogging` より前なので `TextHandler` ではなく、出力は `2026/08/14 05:43:44 ERROR pre-execution error occurred error_type=... error_message="..." ...` の形になる。`level=ERROR` という表記は現れない（レベルは `ERROR` という語だけが日時の後に置かれる）ため、`level=ERROR` による選別は1行も選ばず、除外が何も起きない。

代わりに属性キーで分ける。組み込み既定ハンドラは1レコードを1行にまとめ、属性値を引用符で囲んで改行を `\n` へエスケープするため、複数行の案内文全体が `error_message="…"` を含む1行の中に収まる。一方 `Details:` 側は案内文を改行そのままで出すため、対象の一文は `error_message=` を含まない行に単独で現れる。したがって「`error_message=` を含む行を除いたうえで対象の一文を数える」という手順で、人間向けブロックの出現回数だけを取り出せる（§7 の AC-31 行）。`error_message` は `common.PreExecErrorAttrs.ErrorMessage` の値であり、テストではリテラルを書かずこの定数を参照する。

---

## 2. 実装ステップ

### フェーズ1: 共有処理の追加

このフェーズは既存の呼び出し側を変えず、新しい API を追加するだけに留める。ただし `RunTOCTOUPermissionCheck` の戻り値変更（§1.4.2）だけは全呼び出し元の機械的な追随を伴う。

#### ステップ 1-1: `internal/security` にパス解決と除外判定を追加

**変更ファイル**: `internal/security/toctou.go`、`internal/security/errors.go`、`internal/security/getwd.go`（新規）、`internal/security/test_helpers_getwd.go`（新規）

- [x] `ErrPathResolution` 番兵エラーを追加する（02_architecture.md §4.1 の宣言どおり）。**実装時の変更**: 置き場所は `toctou.go` ではなく `internal/security/errors.go` とした。同パッケージの番兵エラー（`ErrInvalidDirPermissions`・`ErrInsecurePathComponent`・`ErrInvalidPath`）はすべてこのファイルにまとまっており、1つだけ別ファイルに置く理由が無いため。
- [x] `DeepestExistingAncestor(path string) (string, error)` を追加する。与えられた絶対パスから上へ辿り、実在する最深の祖先を返す。`ResolvePathForCheck` はこれを内部で使い、`cmd/record` のステップ 2-5 の判定もこれを使う（同じ探索を2箇所に書かないため）。存在判定には `os.Lstat` を使う（リンク切れのシンボリックリンクを「実在する」側に数え、リンクが置かれている木を黙って検査してしまうより、解決失敗として表面化させるため）。
- [x] §1.4.5 の `getwd` を、ビルドタグで排他的な2ファイルとして追加する。`getwd.go`（`//go:build !test`）は `os.Getwd` を呼ぶだけの関数で、差し替え口を持たない。`test_helpers_getwd.go`（`//go:build test`）は `var getwdHook = os.Getwd` と、それを呼ぶ同名の関数を持つ。どちらのファイルにも、なぜ2つに分かれているのか（本番ビルドに差し替え可能な値を残さないため）を英語のコメントで書く。
- [x] `ResolvePathForCheck(path string) (string, error)` を追加する。相対パスは `getwd` 基準で絶対パス化し、`DeepestExistingAncestor` まで `filepath.EvalSymlinks` で解決したうえで残りを字句的に連結する。祖先を辿る途中の `ENOENT` は失敗として扱わない。**レビュー指摘による修正**: 空パスは `ErrInvalidPath` で拒否する（作業ディレクトリに解決すると、呼び出し元が「未設定」の意味で渡す `""` が無関係な木の検査に化けるため）。未実在部分に `..` が含まれる場合も拒否する（`..` は未実在部分を抜けて未解決のシンボリックリンクに戻りうるため、字句的な連結では扱えない）。失敗時に返すパスは `filepath.Clean` しない（`..` の字句的な畳み込みは、生のまま辿る設計が避けている当のものであるため）。
- [x] `ResolveAllForCheck(paths []string, logger *slog.Logger) (resolved []string, failures int)` を追加する。失敗ごとに `WARN` を1件記録し、失敗した要素にも検査可能なパス（字句的に正規化した絶対パス）を入れる。
- [x] `CheckSkipReason` 列挙型（`CheckSkipNone`・`CheckSkipVariableReference`・`CheckSkipRelative`）と `ClassifyCheckTarget(path string, state PathExpansionState) CheckSkipReason` を追加する。**レビュー指摘による修正**: 未展開参照の有無はパス文字列から推論せず、`PathExpansionState`（`PathExpanded`・`PathHasUnexpandedReference`）として呼び出し元が宣言する。理由と、判定を展開前のテンプレートに対して行わなければならない理由は 02_architecture.md §3.2 を参照。あわせて `config.HasVariableReference(input string) bool` を `internal/runner/config` に追加する（展開の構文を知っている層に判定を置き、エスケープ規則を二重管理にしないため）。

`ResolveAbsPathForCheck` の削除はステップ 4-4 で行う（本フェーズの時点ではまだ呼び出し元が残っているため）。

**完了条件**: `go test -tags test ./internal/security/...` が通る。

#### ステップ 1-2: `RunTOCTOUPermissionCheck` の戻り値を構造体に変える

**変更ファイル**: `internal/security/toctou.go`、および §1.4.2 に挙げた呼び出し元5箇所とテスト4箇所

- [x] `TOCTOUCheckResult` 型を追加し、`RunTOCTOUPermissionCheck` の戻り値を `[]TOCTOUViolation` から `TOCTOUCheckResult` に変える。計数規則は §1.4.2 のとおり。
- [x] `cmd/record/main.go:138` を `result := ...; result.Violations` の形に追随させる。
- [x] `cmd/verify/main.go:105` を追随させる。
- [x] `cmd/verify/main.go:150`（対象ファイル側、戻り値を捨てている）を追随させる。**実装時の確認**: この呼び出しは戻り値を使わないため、コンパイル上も意味上も変更は不要だった。捨てているのが意図的であることを示すコメントのみ添えた。
- [x] `cmd/runner/main.go:460` を追随させる。
- [x] `internal/runner/group_executor.go:372` を追随させる。
- [x] `internal/security/toctou_test.go` の4箇所（`TestRunTOCTOUPermissionCheck_NoViolations`・`_ViolationDetected`・`_MultipleViolations`・`_EmptyDirs`）を追随させる。

**完了条件**: `make test` が通る。既存4テストは元の観点（違反の有無と件数）を引き続き検証している。

#### ステップ 1-3: `internal/security` の新規 API に単体テストを追加

**変更ファイル**: `internal/security/toctou_test.go`

以下のうち権限不足を再現するテストは、§4.6 のとおり root スキップと `t.Cleanup` での権限復帰を必ず入れる。

- [x] `TestResolvePathForCheck_FullyExistingPathResolvesSymlinks`: パス全体が実在し、途中にシンボリックリンクがある場合に実体パスが返る。
- [x] `TestResolvePathForCheck_PartiallyExistingPath`: 末尾2階層が未作成のパスで、実在する最深の祖先まで解決され、残りが字句的に連結される（AC-02）。
- [x] `TestResolvePathForCheck_SymlinkedAncestorOfMissingPath`: 祖先がシンボリックリンクである未作成パスで、返るパスがリンク先の実体側になる（AC-03）。
- [x] `TestResolvePathForCheck_UnreadableAncestorReturnsErrPathResolution`: 読み取り権限のない祖先を経由するパスで、字句的な絶対パスが返り、`errors.Is(err, ErrPathResolution)` が真になる。root スキップと権限復帰を入れる。
- [x] `TestResolvePathForCheck_RelativePathUsesWorkingDirectory`: 相対パスがプロセスの作業ディレクトリ基準で絶対パス化される（AC-06）。作業ディレクトリの変更には `t.Chdir` を使う（自動で復元され、`t.Parallel` との併用を Go 側が拒否する）。
- [x] `TestResolvePathForCheck_AbsConversionFailure`: `os.Getwd` の失敗経路（02_architecture.md §7.2 が挙げる4経路目）。§1.4.5 の `getwdHook` を必ず失敗する実装に差し替え（`t.Cleanup` で復元）、`ErrPathResolution` が返ることと、返るパスが入力そのままであることを検証する。
- [x] `TestResolveAllForCheck_WarnsOncePerFailure`: 失敗パス2件・成功パス1件を渡し、`failures == 2`、`WARN` 記録が2件、返る要素数が3であること（AC-04）。**あわせて、失敗した2要素が期待どおりの字句的絶対パスであることを表明する**（02_architecture.md §3.1 の「失敗した要素にも検査可能なパスが入る」が fail-closed の根拠であり、件数だけでは検証できないため）。root スキップと権限復帰を入れる。
- [x] `TestResolveAllForCheck_NoWarnOnSuccessfulResolution`: 全件成功時に `failures == 0` かつ `WARN` 記録が0件であること（AC-04 の後半）。
- [x] `TestClassifyCheckTarget`: 表形式で `CheckSkipNone`（絶対パス）・`CheckSkipVariableReference`（`PathHasUnexpandedReference` を宣言）・`CheckSkipRelative`（相対パス）の3値を検証する。未展開参照の宣言が相対パスより優先されることも1行として含める。**あわせて、`%{` を含むパスに `PathExpanded` を宣言した場合は検査対象になる行を含める**（判定しているのが文字列ではなく宣言であることは、この行だけが表明できる）。
- [x] `TestClassifyCheckTarget_UnknownStatePanics`: 未知の `PathExpansionState` で panic すること（`switch` の `default` が安全側に倒れることの固定）。
- [x] `TestHasVariableReference`・`TestHasVariableReference_MustBeAskedOfTheTemplate`（`internal/runner/config`）: エスケープされた `\%{` が参照として数えられないこと、および展開後の値に同じ問いを立てると逆の答えになること（＝事実は展開時に記録して持ち回るほかないこと）。
- [x] `TestRunTOCTOUPermissionCheck_CountsCheckedAndSkipped`: 実在ディレクトリ1件・不在ディレクトリ1件・違反ディレクトリ1件を渡し、`Checked == 2`・`Skipped == 1`・`len(Violations) == 1` になること（§1.4.2 の計数規則を固定する）。
- [x] `TestDeepestExistingAncestor`: 全体が実在する場合はパス自身、途中まで実在する場合はその最深の祖先が返ること。相対パスを渡した場合に `ErrInvalidPath` が返ること。
- [x] `TestResolvePathForCheck_DotDotAfterSymlinkFollowsTheLink`（レビュー指摘により追加）: シンボリックリンクの後ろに `..` が続くパスで、リンク先側が検査対象になること。遡る前に字句的な正規化を行う実装では、リンクが置かれている側の木が返って失敗する。
- [x] `TestResolvePathForCheck_DanglingSymlinkAncestorFails`（レビュー指摘により追加）: 実在判定に `os.Stat` ではなく `os.Lstat` を使う選択を固定する。リンク切れのシンボリックリンクを祖先に持つパスで `ErrPathResolution` が返ること。
- [x] `TestResolvePathForCheck_DotDotAfterSymlinkInRelativePath`（レビュー指摘により追加）: 相対パスの絶対パス化でも `..` が字句的に畳まれないこと。前2件の絶対パス版と対になる。

**根拠テストの自己検証**: 各テストを追加したあと、検証対象の分岐（最深祖先への遡り、`WARN` の記録、`Skipped` の加算）を一時的に無効化して失敗することを確認し、その旨をコミットメッセージに記す。

**完了条件**: 上記テストが通り、無効化時に失敗することを確認済み。

### PR-1 作成ポイント: internal/security path resolution and check-result shape

**対象ステップ**: 1-1 / 1-2 / 1-3

**推奨タイトル**: `feat(0164)!: add path resolution helpers and return check counts from RunTOCTOUPermissionCheck`

**レビュー観点**: `ResolvePathForCheck` が実在する最深の祖先まで解決し残りを字句的に連結する規則 / 解決失敗時に検査可能な字句的絶対パスを返す fail-closed 挙動 / `TOCTOUCheckResult` への戻り値変更が5つの呼び出し元とテスト4箇所へ機械的に追随できているか / 相対パスの絶対パス化が `getwd`（§1.4.5）経由になっており、差し替え口が `//go:build test` 側にしか無いこと（本番ビルドに書き換え可能な関数値が残っていないこと）

**実装モデル要件**: frontier-recommended

**判定理由**: `ResolvePathForCheck` の解決規則が本タスクの移行リスクの中心にあるため。02_architecture.md §9 のとおり、実在する最深の祖先まで解決する変更によってアップグレード後に新たな権限違反が検出され得る。あわせてステップ 1-2 が5つの呼び出し元へ波及するシグネチャ変更（PR タイトルの `!`）を含む。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#1023](https://github.com/isseis/go-safe-cmd-runner/pull/1023)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### ステップ 1-4: `filevalidator.HashDirError` を追加

`internal/cmdcommon` の新しいテスト（ステップ 1-5）がこのアクセサを使うため、`cmdcommon` より先に置く。

**変更ファイル**: `internal/filevalidator/validator.go`、`internal/filevalidator/validator_test.go`

- [x] `func (v *Validator) HashDirError() error { return v.deferredErr }` を追加し、doc コメントに「`New` で構築した `Validator` では常に nil」を含める。
- [x] `HashDirAvailable()` の実装を `return v.HashDirError() == nil` に書き換える（doc コメントは維持する）。
- [x] `TestHashDirError_MissingDirectoryReturnsErrHashDirNotExist`: `NewReadOnly` に不在ディレクトリを渡し、`errors.Is(err, ErrHashDirNotExist)` が真。
- [x] `TestHashDirError_UnreadableDirectoryReturnsPermissionError`: 読み取れないディレクトリで `errors.Is(err, os.ErrPermission)` が真。**root では `chmod 0o000` が `EACCES` を生まないため、`syscall.Geteuid() == 0` なら `t.Skip` する**（`internal/logging/safeopen_test.go`・`internal/verification/manager_permission_test.go`・`internal/safefileio/safe_file_linux_test.go` に同じ形の先例がある）。**権限を落とした直後に `t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })` を登録する**（`t.TempDir` の自動削除が失敗しないようにするため）。
- [x] `TestHashDirError_UsableDirectoryReturnsNil`: 実在する読み取り可能なディレクトリで nil。

**完了条件**: `go test -tags test ./internal/filevalidator/...` が通り、`internal/verification` のテストも通る。

#### ステップ 1-5: `internal/cmdcommon` に読み取り専用バリデータ生成を追加

**変更ファイル**: `internal/cmdcommon/common.go`、`internal/cmdcommon/common_test.go`

- [x] `NewDirectoryPermChecker` のラッパーは置かない（02_architecture.md §3.3）。委譲先とシグネチャが同一のため、注入口の既定値には `security.NewDirectoryPermChecker` をそのまま代入でき、ラッパーは何も束ねない。重複しているのは生成の呼び出しではなく panic ブロックであり、その解消はステップ 2-2・3-2・4-2 が行う。
- [x] `CreateReadOnlyValidator(hashDir string) (*filevalidator.Validator, error)` を追加する。`filevalidator.NewReadOnly(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})` に委譲する。
- [x] `TestCreateReadOnlyValidator_DoesNotCreateHashDirectory`: 存在しないハッシュディレクトリを渡しても、構築は成功し、親ディレクトリを `filepath.WalkDir` で走査した結果が実行前後で一致すること。走査ヘルパーは `internal/testutil` の `tu.WalkEntries` とする（ステップ 3-4 の `TestRunCreatesNoFilesystemEntries` が同じ比較を要するため、パッケージ内に置くと複製になる）。この比較は部分木の形（パスとモード）のみを見るもので、既存ファイルへの書き込みは検出しない。
- [x] `TestCreateReadOnlyValidator_ExistingHashDirectoryHasNoDeferredError`: 実在するハッシュディレクトリでは `HashDirError()` が nil を返すこと（ステップ 1-4 で追加済み）。

**注**: `CreateValidator` の削除はステップ 3-1（`verify` の移行完了後）に行う。

**完了条件**: `go test -tags test ./internal/cmdcommon/...` が通る。

#### ステップ 1-6: `internal/fileanalysis` のパーミッション定数を公開し `0o700` に変える

**変更ファイル**: `internal/fileanalysis/file_analysis_store.go`、`internal/fileanalysis/file_analysis_store_test.go`

- [x] `dirPermission = 0o750` を `HashDirPerm os.FileMode = 0o700` として公開する（18-19 行）。付随するコメントを、02_architecture.md §3.5 の理由（所有者以外に内容を見せる必要がない）に沿った英文へ書き換える。
- [x] 32 行目の doc コメント `If analysisDir does not exist, it will be created with mode 0o750.` を `... with mode HashDirPerm (0o700).` に変える。
- [x] 48 行目の `os.MkdirAll(analysisDir, dirPermission)` を `HashDirPerm` に変える。
- [x] 既存の `TestNewStore_CreatesDirectory` に、作られたディレクトリの `os.Stat` の `Mode().Perm()` が `0o700` であるという表明を足す。中間ディレクトリ（`MkdirAll` が途中で作るもの）についても同じ表明を置く。比較対象は `HashDirPerm` ではなく `0o700` リテラルとする（定数が自分自身と等しいことを確かめても何も検証したことにならないため）。所有者ビットは umask に削られないため、この表明は通常の umask 設定（022・027・077）で成立する。専用のテスト関数を別に立てない: 同じ一時ディレクトリ・同じパスリテラル・同じ事前確認を繰り返し、最後の表明だけが異なる複製になるため。

**完了条件**: `go test -tags test ./internal/fileanalysis/...` が通る。

#### フェーズ1 完了ゲート

- [x] `make fmt` → `make test` → `make lint` が緑。
- [x] `make build` が通る。`make lint` は `--build-tags test` で走るため、`getwd.go`（`//go:build !test`）をコンパイルするのはこの経路だけである（§1.4.5）。
- [x] `make deadcode` を実行する。`ResolvePathForCheck`・`ResolveAllForCheck`・`ClassifyCheckTarget`・`DeepestExistingAncestor`・`security.getwd`（`ResolvePathForCheck` からのみ呼ばれる）・`HasVariableReference`・`trimLastPathComponent`・`hasDotDotComponent`（後2者は上記関数の非公開ヘルパー）・`CreateReadOnlyValidator` は、利用側の移行が済むフェーズ2〜4まで呼び出し元を持たないため、未到達として報告される。これは想定内であり、他に新しい未到達シンボルが出ていないことだけを確認する。

### PR-2 作成ポイント: read-only validator helpers and hash-directory permission

**対象ステップ**: 1-4 / 1-5 / 1-6

**推奨タイトル**: `feat(0164): add read-only validator helpers and tighten hash directory permission`

**レビュー観点**: `HashDirError()` が `NewReadOnly` の3状態（不在・読み取り不能・正常）を区別できること / `CreateReadOnlyValidator` がハッシュディレクトリを作らないことを部分木の走査で示せているか / `HashDirPerm` を `0o700` に変えても `fileanalysis` の既存利用が壊れないこと / パーミッションの表明が定数ではなく `0o700` リテラルと比較されているか

**実装モデル要件**: standard

**判定理由**: 委譲だけのヘルパー追加と定数1件の変更であり、未決の実装方式・パネルモードのトリガ・孤立した高リスク段階のいずれにも該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#1024](https://github.com/isseis/go-safe-cmd-runner/pull/1024)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### フェーズ2: `record` の移行

#### ステップ 2-1: `record` のパス解決を共有処理に置き換える

**変更ファイル**: `cmd/record/main.go`

- [x] `checkDirPermissions` 内の対象ファイル群の絶対パス化・リンク解決ループ（113-124 行）を `security.ResolveAllForCheck(cfg.files, logger)` に置き換える。
- [x] ハッシュディレクトリの解決（125-132 行）を `security.ResolvePathForCheck(cfg.hashDir)` に置き換える。解決エラーは無視せず記録する。**レビュー指摘による修正**: `WARN` を出して続行する形では fail-open になるため、`ERROR` を記録したうえで標準エラー出力に理由を書き、`false` を返す（非ゼロ終了）形にした。理由は 02_architecture.md §3.1 の失敗表の最終行にあり、未実在部分に `..` を含むパスでは解決が入力より短い健全な祖先を返すため、それを検査して通過したうえで別の木にハッシュディレクトリを作ってしまう（`-d <sticky world-writable>/x/../hashes` が現に通っていた）。対象ファイル群の解決失敗は従来どおり `WARN` のみ。

#### ステップ 2-2: `record` の権限チェッカ生成から panic を無くす

**変更ファイル**: `cmd/record/main.go`

- [x] `deps` の `toctouChecker` フィールド（52 行）を §1.4.4 の `newPermChecker` フィールドで置き換え（併存させない）、`defaultDeps()` で `security.NewDirectoryPermChecker` を設定する。
- [x] `checkDirPermissions` の 105-112 行を `d.newPermChecker()` の呼び出しに置き換え、エラー時は `panic` せず `fmt.Fprintf(stderr, ...)` でエラーを出して `false` を返す。
- [x] `toctouChecker` に偽のチェッカを代入していた既存テストを、`d.newPermChecker` が偽のチェッカを返す形へ書き換える（テスト側のヘルパー `fixedPermChecker`）。

#### ステップ 2-3: ハッシュディレクトリの作成を権限チェック通過後へ移す

**変更ファイル**: `cmd/record/main.go`

- [x] `parseArgs` から `d.mkdirAll(dir, hashDirPermissions)`（274-276 行）を削除する。**実装時の変更**: これにより `parseArgs` が `deps` を一切使わなくなったため、引数 `d` も落として `parseArgs(args []string, stderr io.Writer)` とした（`verify` 側のステップ 3-2 が同じ形にすると述べているのと揃う）。呼び出し元は `run` と2件のテストのみ。
- [x] `run` の `checkDirPermissions` 成功直後（186-189 行の後）に、`d.mkdirAll(cfg.hashDir, fileanalysis.HashDirPerm)` を置く。失敗時は `errEnsureHashDir` で包んだエラーを標準エラー出力に書き、`1` を返す。**レビュー指摘による修正**: `checkDirPermissions` が解決済みのハッシュディレクトリを返すようにし、`run` は以降その値を `cfg.hashDir` に入れて使う。検査した対象と作成・利用する対象が別々に計算される状態を残さないため。
- [x] `hashDirPermissions` 定数（37 行）を削除し、`internal/fileanalysis` を import して `fileanalysis.HashDirPerm` を使う。
- [x] この作成が `libccache.NewLibcCacheManager`（192-198 行）・`NewMachoLibSystemCacheManager`（201-206 行）・`dynamicanalysis.New`（229-234 行）・`d.validatorFactory`（223 行）より前にあることを、コードの並び順とコメントで示す。とくに `validatorFactory` は `filevalidator.New` 経由でハッシュディレクトリを暗黙に作る（§1.3.4）ため、権限チェックがこれより前にある順序が AC-08 の成立条件である。この依存関係を英語のコメントで明記する。

#### ステップ 2-4: `record` の重複計算を解消

**変更ファイル**: `cmd/record/main.go`

- [x] 201 行の `machoCacheDir := filepath.Join(cfg.hashDir, libcCacheSubDir)` を削除し、192 行の `cacheDir` を `NewMachoLibSystemCacheManager` に渡す。

#### ステップ 2-5: 作成が必要な場合に sticky ビットの例外を適用しない

**変更ファイル**: `cmd/record/main.go`

- [x] `checkDirPermissions` に、ハッシュディレクトリが実行時点で存在しない場合のみ働く追加判定を入れる（02_architecture.md §5.2）。作成先は `security.DeepestExistingAncestor`（ステップ 1-1）で求める。同じ探索を `cmd/record` に書き直さない。**実装時の変更**: 判定本体は `cmd/record/main.go` の `checkHashDirCreationSite` として切り出し、`checkDirPermissions` が違反0件のときに呼ぶ形にした（判定条件と失敗時の案内が既存の違反報告と混ざらないようにするため）。
- [x] 求めた作成先を `os.Stat` し、`Mode().Perm()&0o002 != 0` であれば、sticky ビットの有無にかかわらず違反として扱い、`false` を返す。**実装時の追加**: 作成先が求まらない場合（絶対パスでない・祖先を stat できない）も `false` を返す（書き込み直前に安全性を確認できない状態であるため）。**レビュー指摘による修正**: 作成先の検査は `os.Stat` ではなく `os.Lstat` で行い、ディレクトリでなければ拒否する（作成先がシンボリックリンクだった場合、見るべきモードはリンク先のものになり、その祖先は検査されていないため）。
- [x] **レビュー指摘による追加**: 同じ判定をハッシュディレクトリ自身にも適用する（02_architecture.md §5.2 の表2行目）。sticky ビットが「まだ存在しない名前」を守らないという §5.2 の根拠は、ハッシュディレクトリの名前だけでなく、その中のハッシュ記録ファイルの名前にも同じように当てはまる。sticky 付き world-writable なハッシュディレクトリ（`/tmp/hashes` を `1777` で運用する等）は共有チェックを通過しており、`record` がまだ処理していないファイルのハッシュ記録を第三者が先回りで置ける状態だった。実装では入口を `checkHashDirWriteSafety` とし、存在すれば `checkExistingHashDirMode`、不在なら `checkHashDirCreationSite` へ分岐させる。**この項目は破壊的変更である**: sticky 付き world-writable なハッシュディレクトリを使っていた環境は、ディレクトリのモードを絞るまで `record` が通らなくなる。
- [x] 判定は `cmd/record` 側に置き、`security.ValidateDirectoryPermissions` の規則は変更しない。
- [x] 拒否時の標準エラー出力に、先に利用者自身がディレクトリを作れば通ることを含める。

#### ステップ 2-6: `record` のテストを追加・更新

**変更ファイル**: `cmd/record/main_test.go`

- [x] `TestHashDirPermissions_0o700` を、`parseArgs` ではなく `run` を対象に書き換える（作成位置が移るため）。不在のハッシュディレクトリを指定して `run` を実行し、作成後のパーミッションが `0o700` であることを検証する。
- [x] `TestRunTOCTOU_HashDirNotCreatedOnViolation` を追加する（AC-08）。実行前に不在であることを確認し、違反を返すチェッカを注入して `run` を呼び、終了コードが `1`、かつ実行後もディレクトリが不在であることを検証する。
- [x] `TestRun_CreatesHashDirAfterPermissionCheckPasses` を追加する（AC-09）。違反なしのチェッカで `run` を呼び、ディレクトリが作られ、ハッシュ記録が生成されることを検証する。
- [x] `TestRunTOCTOU_ChecksAncestorsWhenHashDirMissing` を追加する（AC-10）。不在のハッシュディレクトリを指定し、注入したチェッカが祖先ディレクトリのパスで呼ばれた記録を持つことを検証する。
- [x] `TestRunTOCTOU_ReportsViolationBehindSymlinkedAncestor` を追加する（AC-03・AC-07、02_architecture.md §7.4 の1点目）。見かけ上の祖先は健全、シンボリックリンク先の祖先だけが違反という配置を作り、違反が報告されることを検証する。リンクを辿らない実装では見かけ上の健全な祖先しか検査されず違反が出ないため、このテストは解決処理が働いていることの根拠になる。
- [x] `TestRun_ReportsHashDirCreationFailure` を追加する（AC-11）。常に失敗する `d.mkdirAll` を注入し、終了コードが `1` かつ標準エラー出力に理由が出ることを検証する。
- [x] `TestRun_RejectsHashDirCreationUnderStickyWorldWritableParent` を追加する（02_architecture.md §5.2）。**`t.TempDir()` の直下に中間ディレクトリを自分で作り、`chmod 1777` を掛けたうえで、その下の未作成パスをハッシュディレクトリに指定する**（`t.TempDir()` 自体は `0o700` なので、これを作らないと作成先が world-writable にならず、テストが誤った理由で通る）。非ゼロ終了かつディレクトリが作られないことを検証する。非 root ユーザーでも自分の所有ディレクトリに sticky ビットは立てられるため、CI で root スキップは不要である。
- [x] `TestRun_AllowsExistingHashDirUnderStickyWorldWritableParent` を追加する。同じ配置で、ハッシュディレクトリが既に存在する場合は通ること。判定が「作成が必要な場合」に限られることを示す対の検証である。
- [x] `TestRun_CreatesHashDirBeforeSubdirectories` を追加する（02_architecture.md §3.5）。不在状態から `run` を実行し、実行後のハッシュディレクトリ自身のパーミッションが `0o700`（`libccache` の `0o755` ではない）であることを検証する。**レビュー指摘による修正**: モードの比較だけでは umask 077 の環境で `libccache` の `0o755` も `0o700` に落ちて表明が効かなくなるため、`d.mkdirAll` を差し替えて、ハッシュディレクトリを作る時点で `lib-cache` がまだ存在しないことを順序として直接表明する。
- [x] `TestRun_RefusesUnresolvableHashDirPath` を追加する（レビュー指摘により追加）。未実在部分に `..` を含むハッシュディレクトリパスを、world-writable な親（sticky 有無の両方）の下に指定し、非ゼロ終了かつ何も作られないことを検証する。解決失敗を `WARN` で流す実装では、健全な祖先が検査されて通過し、別の木にハッシュディレクトリが作られる。
- [x] `TestCheckHashDirWriteSafety_RefusesWhenSiteIsUnusable` を追加する（レビュー指摘により追加）。作成先が求まらない・使えない3経路（相対パス、リンク切れの祖先、読み取り不能な祖先）を `checkHashDirWriteSafety` に直接与えて拒否を確認する。本番でこれらに入るのは解決直後の競合のみであり、`run` 経由では到達させられない。リンク切れの経路は `os.Lstat` と `IsDir` の選択を固定する。読み取り不能の経路は root スキップと権限復帰を入れる。
- [x] `TestRun_RejectsExistingStickyWorldWritableHashDir` を追加する（レビュー指摘により追加、02_architecture.md §7.4 の3点目）。既存のハッシュディレクトリを sticky 付き world-writable にして `run` を呼び、非ゼロ終了かつハッシュ記録が書かれないことを検証する。**sticky を付けるのが要点**で、付けなければ共有チェックが単独で弾き、`record` 側の判定が無くてもテストが通ってしまう。したがってテストはまず `security.NewDirectoryPermChecker()` がこのディレクトリを受理することを `require` で表明し、そのうえで `record` 側の判定に固有の文言（`is world-writable` と `pre-plant`）で拒否を確認し、共有チェックの文言（`permission violation in hash directory`）が出ていないことも表明する。
- [x] `TestRun_ExitsWithoutPanicWhenCheckerInitFails` を追加する（AC-24・AC-26）。`d.newPermChecker`（§1.4.4）に失敗を返す実装を注入し、終了コードが `1`、標準エラー出力にエラーが出ることを検証する。`run` を同一プロセス内で呼ぶため panic はテストバイナリごと落ちる。したがって「panic しない」の実質的な根拠は「`run` が値を返して来ること」であり、出力に `goroutine ` が無いことの表明は補助にとどまる旨を、テストの doc コメントに英語で記す。
- [x] 既存の `TestRunTOCTOU_FailsClosedOnWorldWritableDir`・`TestRunTOCTOU_NoViolation_Continues`・`TestRunTOCTOU_ViolationLogsErrorAndExits`・`TestRunTOCTOU_ForceFlagDoesNotBypassViolation`・`TestRunTOCTOU_ViolationLogsRemediationWithActualPath`・`TestRunUsesDefaultHashDirectoryWhenNotSpecified` を、作成位置の移動と `deps` の変更に追随させる。各テストが元の観点を引き続き検証していることを §7 のトレーサビリティ表で示す。**実装時の変更**: ハッシュディレクトリを作らせないためだけに存在したヘルパー `testRunDeps(hashDir)` は、作成位置が `run` に移って不要になったため削除し、呼び出しを `defaultDeps()` に置き換えた。

#### フェーズ2 完了ゲート

- [x] `make fmt` → `make test` → `make lint` が緑。

### PR-3 作成ポイント: record write-ordering and creation guard

**対象ステップ**: 2-1 / 2-2 / 2-3 / 2-4 / 2-5 / 2-6

**推奨タイトル**: `feat(0164)!: create the record hash directory only after the permission check passes`

**レビュー観点**: ハッシュディレクトリの作成が `validatorFactory`・`libccache`・`dynamicanalysis` のどれよりも前かつ権限チェック通過後に置かれているか（AC-08 の成立条件） / sticky world-writable な作成先を拒否する判定が「作成が必要な場合」だけに効き、既存ディレクトリの通過を妨げないか / 拒否時の標準エラー出力が回避手順を示しているか / 偽チェッカの注入が `d.newPermChecker` 経由に統一され、`toctouChecker` が残っていないか

**実装モデル要件**: frontier-required

**判定理由**: `mkplan.md` step 8 のパネルモード・トリガのうち「セキュリティゲート/移行」に該当する。書き込みを権限チェックの後ろへ移す変更に、world-writable な作成先を拒否する新しい規則（ステップ 2-5）とハッシュディレクトリのパーミッション変更が重なり、§5 のリスク表が「正常な運用を拒否して `record` が使えなくなる環境が出る」可能性を挙げている。PR-5（`verify` の終了コード契約）と同じ種類の変更であり、同じ段階に置く。ステップ 2-5 はこの PR の実装ステップの最後（テストの直前）に置き、独立した修正（ステップ 2-4）がその後ろに残らないようにした。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#1026](https://github.com/isseis/go-safe-cmd-runner/pull/1026)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### フェーズ3: `verify` の移行

#### ステップ 3-1: `verify` の書き込み副作用を断つ

**変更ファイル**: `cmd/verify/main.go`、`internal/cmdcommon/common.go`、`internal/cmdcommon/common_test.go`、`internal/security/dir_permissions_unix.go`、`internal/security/toctou.go`、`internal/security/toctou_test.go`（後3者はレビュー指摘による追加分。下の最終項目を参照）

- [x] 削除の前に `go test -tags test -coverprofile=/tmp/cmdcommon-before.out ./internal/cmdcommon/... && go tool cover -func=/tmp/cmdcommon-before.out` を実行し、結果を保存する。
- [x] `parseArgs` から `mkdirAll(dir, hashDirPermissions)`（221-223 行）を削除する。
- [x] `hashDirPermissions` 定数（19 行）を削除する。
- [x] `errEnsureHashDir`（38 行）を削除する。参照している `cmd/verify/main_test.go:176`（`TestParseArgsInvalidHashDir`）も、作成しなくなったことで存在意義が無くなるため削除する。ハッシュディレクトリが使えない場合の検証はステップ 3-5 の新規テストが引き継ぐ。
- [x] バリデータ生成を `cmdcommon.CreateReadOnlyValidator` に切り替える。
- [x] `internal/cmdcommon/common.go` から `CreateValidator` を削除する。
- [x] `internal/cmdcommon/common_test.go` の `TestCreateValidator_ValidHashDirectory` を削除する。
- [x] 同 `TestCreateValidator_DefaultHashDirectory` を削除する。
- [x] 同 `TestCreateValidator_NonExistentDirectory` を削除する（「自動作成される」ことを主張するテストであり、廃止する挙動そのものを固定している）。
- [x] 同 `TestCreateValidator_RelativePath` を削除する。
- [x] 同 `TestCreateValidator_EmptyPath` を削除する。
- [x] `TestCreateReadOnlyValidator_RelativePath` を追加して相対パスの観点を引き継ぐ。作業ディレクトリの移動には `t.Chdir`、ハッシュディレクトリには `t.TempDir` を使う（削除元のテストはパッケージのソースディレクトリ内に `./test_hashes` を作り `defer os.RemoveAll` していた。この形は panic 時に残骸を残すため引き継がない）。
- [x] `TestCreateReadOnlyValidator_EmptyPath` を追加して空パスの観点を引き継ぐ。**実装時の確認**: 削除元の `TestCreateValidator_EmptyPath` は構築エラーを表明していたが、`NewReadOnly` では `os.Lstat("")` が `ENOENT` を返すため構築は成功し、`HashDirError()` が `ErrHashDirNotExist` を返す。空パスが使えないハッシュディレクトリとして報告されるという観点は同じなので、新テストはこの形で表明する。
- [x] 削除後に同じ手順でカバレッジを取得（`/tmp/cmdcommon-after.out`）し、関数単位で低下していないことを確認する。低下した関数があれば補うテストを追加する。**確認結果**: 削除前後とも 100.0%（残る `CreateReadOnlyValidator` も 100.0%）。
- [x] `make deadcode` を実行し、`CreateReadOnlyValidator` への切り替えによって `internal/cmdcommon` に新たな未到達の公開シンボルが生じていないことを確認する。
- [x] （レビュー指摘により追加）`internal/security/dir_permissions_unix.go` の `Lstat` 失敗ログを、`fs.ErrNotExist` の場合のみ `DEBUG` に下げる。作成をやめた結果、ハッシュディレクトリ不在は `verify` の通常経路になったが、`ValidateDirectoryPermissions` はこれを `ERROR` で記録していた。`RunTOCTOUPermissionCheck` は同じ状態を「読み飛ばし（`Skipped`）」に数えるため、記録と判定が食い違い、未整備のホストでの通常実行がログ監視に警報として見える。エラー自体は従来どおり返るので情報は失われない。それ以外の stat 失敗（検査できない＝下流で回復不能）は `ERROR` のまま。判定規則は変えていないため、要件定義書がスコープ外とする「TOCTOU 権限チェック処理そのものの挙動変更」には当たらない。根拠テストは `internal/security/toctou_test.go` の `TestRunTOCTOUPermissionCheck_MissingDirIsNotLoggedAsAnError`（不在は `ERROR` を出さず、stat できない祖先では出ることを対で表明）。
  - **レビュー指摘による補足**: `ValidateDirectoryPermissions` は共有関数であり、呼び出し元は `RunTOCTOUPermissionCheck` のほかに `internal/verification/manager.go`（ハッシュディレクトリ検証）と `internal/runner/base/security/file_validation.go`（出力先検証）がある。後2者は不在を含む全エラーを失敗として返すため、記録は各呼び出し元のエラー経路に残り、この変更で消えるのは共有関数側の重複した1行だけである。「不在かどうかをどう扱うか」は呼び出し元の判断であるという理由をコメントに書き直した。記録が完全に消えるわけではない点も根拠テストで固定した。下げた先の `DEBUG` 行が当該パスを含むことを表明しており、レベル変更が「記録の削除」に化けた場合は失敗する。読み飛ばしの件数は `TOCTOUCheckResult.Skipped` が持ち、起動時チェックではフェーズ4で `INFO` に出る。

#### ステップ 3-2: `verify` を `deps` 様式へ移行

**変更ファイル**: `cmd/verify/main.go`

- [x] 02_architecture.md §3.4 の `deps` 構造体（`validatorFactory`・`newPermChecker`・`resolvePathForCheck`・`ensurePermissionCheckUID`）を定義し、`defaultDeps()` で既定値（`security.NewDirectoryPermChecker`・`security.ResolvePathForCheck`）を与える。
- [x] パッケージレベル変数 `validatorFactory`・`mkdirAll`・`ensurePermissionCheckUID`・`toctouChecker`（39-48 行）をすべて削除する。`toctouChecker` は `deps` のフィールドとしても復活させず、§1.4.4 の `newPermChecker` が唯一の差し替え口になる。`errNoFilesProvided` は不変の番兵エラーなので残す。
- [x] `run` と `checkDirPermissions` のシグネチャに `deps` を渡す。ステップ 3-4 で `checkDirPermissions` を2つに分けるときは、両方へ引き継ぐ。
- [x] `main()` を `os.Exit(run(os.Args[1:], defaultDeps(), os.Stdout, os.Stderr))` に変える。
- [x] `parseArgs` は `deps` を必要としなくなるため、`d` を渡さない（`record` の `parseArgs` は `mkdirAll` のために受け取っていたが、`verify` にはその必要が無い）。
- [x] （レビュー指摘により追加、ステップ 3-4 から前倒し）ハッシュディレクトリのパス解決を `filepath.Abs` + `filepath.EvalSymlinks` から `d.resolvePathForCheck` へ移す。この形は `parseArgs` が先にディレクトリを作っていたから成立していたもので、作成をやめた本 PR では不在ディレクトリで `EvalSymlinks` が失敗し、残る未解決パスの祖先にシンボリックリンクがあると `Lstat` 判定が「ディレクトリではない」＝違反と見なして fail-closed（終了コード `3`）になる。これは PR-5 が担当する終了コードの契約変更が PR-4 に漏れる形であり、PR-5 だけを差し戻せるようにした §3.2 の分離も崩れる。エラー時の戻り値（存在する最深祖先）をそのまま使うため、終了コードの挙動自体はこの PR では変えない（`ErrPathResolution` に固有の終了コードを与えるのはステップ 3-4）。根拠テストは `cmd/verify/main_test.go` の `TestRunResolvesMissingHashDirUnderSymlinkedAncestor`（`tu.SafeTempDir` はシンボリックリンクを解決するため、既存の `TestRunCreatesNoFilesystemEntries` ではこの経路に到達できない）。

#### ステップ 3-3: `verify` のテストを `deps` 経由へ移す

**変更ファイル**: `cmd/verify/main_test.go`

- [x] ファイル冒頭の「パッケージレベル変数を差し替えるので `t.Parallel()` を呼んではならない」旨のコメントを、`slog` の既定ロガーのみが対象である旨に書き換える。
- [x] `overrideValidatorFactory` と `overrideTOCTOUChecker` を削除し、`deps` を組み立てるヘルパー `testDeps(...)` に置き換える（`cmd/record/main_test.go` の `testRunDeps` と同じ形）。
- [x] 既存 15 テスト（`TestRunRequiresAtLeastOneFile`・`TestRunProcessesMultipleFiles`・`TestRunReportsFailuresAndContinues`・`TestRunWarnsWhenDeprecatedFlagUsed`・`TestRunUsesDefaultHashDirectoryWhenNotSpecified`・`TestRunTOCTOU_ContinuesWhenOnlyTargetDirViolates`・`TestRunFailsClosedOnHashDirViolation_ExplicitHashDir`・`TestRunProceedsWithRealCheckerOnCleanDirs`・`TestRunFailsClosedOnHashDirViolation_AncestorViolation`・`TestRunFailsClosedOnHashDirViolation_DefaultHashDir`・`TestRunFailsClosedOnHashDirViolation_LogsErrorLevel`・`TestRunSkipsTargetSetCheckWhenHashDirViolates`・`TestRunFailsClosedWhenPermissionCheckUIDUnresolvable`・`TestVerifyDeclaresSudoUIDAwarePolicy`）を `deps` 経由に書き換える。`TestParseArgsInvalidHashDir` はステップ 3-1 で削除する。
- [x] `TestRunCreatesNoFilesystemEntries` を追加する（AC-14）。ハッシュディレクトリの**親を起点に `filepath.WalkDir` でパスとモードの集合を実行前後で取得し、完全に一致すること**を検証する。親の直下だけを見る方式では、既存のハッシュディレクトリの内部に作られた残骸を見落とすため使わない。存在するハッシュディレクトリを指定した場合と不在の場合の両方を対象にする。

**完了条件**: 書き換え前後で `go test -tags test -coverprofile=... ./cmd/verify/... && go tool cover -func=...` を比較し、関数単位で低下がないこと（§5 のリスク表）。この比較は挙動が変わらないこの段階でのみ意味を持つ。

**確認結果**（PR-4 の最終状態で再測定）: 未到達ブロックは書き換え前 11 件から 9 件へ減り、増えた関数は無い。`parseArgs` の百分率だけが 90.5% → 89.5% と動くが、これは削除した `mkdirAll` 呼び出しの2文（いずれも到達済み）が母数から抜けたためで、未到達ブロックは2件のまま変わらない。`checkDirPermissions` は `EvalSymlinks` 失敗分岐と `validatorFactory` 既定値が `TestRunCreatesNoFilesystemEntries` により到達し、91.7% → 93.9% に上がった。`internal/cmdcommon` は前後とも 100.0%。

### PR-4 作成ポイント: verify read-only migration and deps wiring

**対象ステップ**: 3-1 / 3-2 / 3-3

**推奨タイトル**: `refactor(0164)!: stop verify from writing and move it to deps-style injection`

**レビュー観点**: `verify` がいかなる引数でもファイルシステムに書き込まないこと（部分木の走査による確認） / `CreateValidator` の削除で `internal/cmdcommon` のカバレッジが関数単位で低下していないこと / パッケージレベル可変変数が4つとも消え、`toctouChecker` が `deps` にも復活していないこと / 既存 15 テストの書き換えで元の観点が落ちていないこと（この段階では挙動が変わらないため、カバレッジ比較が根拠として成立する）

**実装モデル要件**: frontier-recommended

**判定理由**: 書き込み副作用の停止（AC-14）は利用者から見える挙動変更でありながら、同時に `cmd/verify/main_test.go` の全 15 テストの差し替え口の移行を伴うため、退行が埋もれやすい。終了コードの契約変更は次の PR-5 に分離した。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#1028](https://github.com/isseis/go-safe-cmd-runner/pull/1028)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）


#### ステップ 3-4: `verify` の起動判定を組み替える

**変更ファイル**: `cmd/verify/main.go`

現行の `checkDirPermissions`（80-152 行）は 02_architecture.md §6.2 の段階 3 と段階 5 を1つの関数で行っている。§6.2 の順序（3 → 4 → 5）を実現するため、次のように分割する。

- [x] `checkHashDirPermissions(cfg *verifyConfig, d deps, stderr io.Writer) (checker security.DirectoryPermChecker, hashDirs []string, exitCode int)` を新設し、現行 81-122 行（チェッカ生成、ハッシュディレクトリの解決、段階 3 のチェックと fail-closed 判定）を移す。構築したチェッカと `hashDirs` を戻り値で返す。パス解決の `d.resolvePathForCheck`（§1.4.4）への移行はステップ 3-2 で済んでいるので、ここでは移設のみを行い、返るエラーに固有の識別トークン（`path_resolution_failed`、§1.4.1）を与えて fail-closed で終了する。**実装時の変更**: 戻り値は `(hashDirCheck, int)` とした。`hashDirCheck` はチェッカ・検査したディレクトリ列・解決済みハッシュディレクトリを持つ小さな構造体で、返り値が4つに増えるのを避けるためのものである。3番目の戻り値を `ok bool` ではなく終了コードにしたのは、下の「ディレクトリでない場合」の項目により、この関数は `3` と `1` の2つの終了コードを返し分ける必要があるためである。`exitOK` が「続行してよい」を表す。
- [x] `checkTargetFilePermissions(cfg *verifyConfig, checker security.DirectoryPermChecker, hashDirs []string, logger *slog.Logger)` を新設し、現行 124-151 行（対象ファイルの解決と段階 5 のチェック）を移す。**現行 140-149 行の重複除去（`checked map[string]struct{}` により、ハッシュディレクトリ側で既に検査したディレクトリを二重に警告しない）を、分割後も維持する**。これが `hashDirs` を戻り値として渡す理由である。**実装時の変更**: (1) 対象ファイルの `filepath.Abs` + `EvalSymlinks` ループを `security.ResolveAllForCheck` に置き換えた（§1.3.1 の表がこの置き換えを挙げており、AC-01 の「同等の処理が各コマンドに重複して残らない」もこれを求める）。(2) その結果この関数は `deps` を使わなくなったため、引数 `d` を落とした。
- [x] （レビュー指摘による追加）読み取り専用バリデータを、コマンドラインのパスではなく解決済みのパス（`checkHashDirPermissions` が返す `resolved`）で構築する。理由は2つある。(1) 検査した対象と読み取る対象が別々に計算される状態を残さない（`record` のステップ 2-3 と同じ理由）。(2) `filevalidator.NewReadOnly` は `os.Lstat` で判定するため、ハッシュディレクトリ自身がシンボリックリンクの場合に `ErrHashPathNotDir`（「ディレクトリではない」）で構築に失敗する。PR-4 で `New` から `NewReadOnly` へ切り替えた結果、シンボリックリンクのハッシュディレクトリを使っていた構成が動かなくなっていた。解決済みパスを渡すとこの退行も同時に解消する。根拠テストは `TestRunVerifiesThroughSymlinkedHashDir`。
- [x] （レビュー指摘による追加）段階 4 に、ハッシュディレクトリの**検索権限**の確認を加える。`HashDirError()` はこれを見ない。`NewReadOnly` はディレクトリを `Lstat` するだけで、これは親ディレクトリの権限しか要さないため、`0o000` のハッシュディレクトリでも遅延エラーが載らず、実行はファイル単位の `FAILED`（終了コード `1`）に落ちる。これは §3.4 と AC-13 が避けようとしている提示そのものである（環境の問題が「そのファイルが改ざんされた」ように見える）。確認は `os.Stat(dir + "/.")` で行う。ハッシュ記録の読み取りは `<dir>/<name>` を開く操作であり、必要なのは検索権限（`x`）だけで一覧権限（`r`）は要らないため、ディレクトリ自体を `os.Open` する確認では検索専用ディレクトリ（`0o100` など）を誤って拒否する。根拠テストは `TestRunUnsearchableHashDirExitsUntrustedEnvironment` と、その境界を固定する `TestRunAcceptsSearchOnlyHashDir`。
- [x] （実装時の追加）ハッシュディレクトリのパスが実在してディレクトリでない場合を、段階 3 の権限チェックより前に `os.Lstat` で判定し、`exitVerificationFailed`（`1`）と識別トークン `hash_dir_not_a_directory` を返す。02_architecture.md §4.3 はこの場合を「バリデータ生成失敗（`ErrHashPathNotDir`）として終了コード `1`」としているが、実際には段階 3 の `ValidateDirectoryPermissions` が先に到達し、`ErrInvalidDirPermissions`（`... is not a directory`）として違反を報告するため、バリデータ生成まで届かない。放置すると設定ミス（`-d` の指定先がファイル）が終了コード `3`（改ざんの疑い）として警報になり、しかも案内は「平文ファイルのパーミッションを直せ」という無意味なものになる。判定は誤りの余地が無い（`Lstat` してモードを見るだけで、エラー文字列の照合を伴わない）。`Lstat` の失敗は無視する。不在は段階 4 が、stat できない場合は段階 3 の権限チェックが、それぞれ担当するためである。
- [x] `run` を §6.2 の6段階の順に並べ替える。段階 4（読み取り専用バリデータの構築と `HashDirError()` の判定）を、`checkHashDirPermissions` と `checkTargetFilePermissions` の間に置く。
- [x] `hashValidator` インターフェースに `HashDirError() error` を追加する。
- [x] 段階 4 で `HashDirError()` を判定し、`ErrHashDirNotExist` なら `hash_dir_not_found`、それ以外なら `hash_dir_unreadable` のトークンを含むメッセージを標準エラー出力へ出して `exitUntrustedEnvironment` を返す。検索権限の確認（上記）も同じ `hash_dir_unreadable` を使う。**実装時の変更**: 後者の条件を `os.ErrPermission` の判定から「不在以外のすべて」に広げた。`NewReadOnly` の遅延エラーには `ENOTDIR`・`ELOOP` など生の `Lstat` エラーも載るため、`os.ErrPermission` だけを分けると残りが無言で素通りする。いずれも「存在するが開けなかった」であり、利用者への案内は同じである。
- [x] `ErrHashPathNotDir` は `NewReadOnly` が構築エラーとして返すため、既存のバリデータ生成失敗経路で `exitVerificationFailed`（`1`）になる。この対応を doc コメントに明記する。**実装時の確認**: この経路は段階 3 が先に到達するため実際には使われない（上の「実装時の追加」を参照）。doc コメントは、同じ事象を段階 3 側で判定していることを述べる形にした。
- [x] §1.4.1 のトークンを名前付き定数として定義し、既存の権限違反メッセージ（120 行）にも `hash_dir_permission_violation` を追加する。
- [x] チェッカ生成を `d.newPermChecker()`（§1.4.4）に置き換え、失敗時は `permission_checker_init_failed` トークン付きのエラーを標準エラー出力へ出す。この場合の終了コードは `exitUntrustedEnvironment`（`3`）とする。
- [x] ハッシュディレクトリの解決に失敗した場合、fail-closed 終了時の標準エラー出力にその旨と対象パスを1行追記する（AC-05）。この判定点で解決済みなのはハッシュディレクトリ1件のみ（対象ファイル群は段階 5 で解決する）なので、件数ではなく「ハッシュディレクトリのパス解決に失敗した」という事実と当該パスを示す形にする。
- [x] 21-25 行の終了コード説明コメントを更新する。変更前は `(the policy declaration in init and the checker initialisation in checkDirPermissions)`、変更後は `(the policy declaration in init)` とし、残る panic が1点のみであることを示す（AC-28）。

#### ステップ 3-5: 終了コードと識別トークンのテストを追加

**変更ファイル**: `cmd/verify/main_test.go`

- [x] `fakeValidator` に `HashDirError() error` を実装する。
- [x] `TestRunSkipsTargetSetCheckWhenHashDirViolates` が、ステップ 3-4 の関数分割後も「ハッシュディレクトリ側で止まったとき対象ファイル側のチェックが走らない」ことを検証していることを確認する。**確認結果**: 無変更で通る。同テストは対象ファイル側のディレクトリがログに現れないことを表明しており、分割後も `checkTargetFilePermissions` が呼ばれないことの根拠になっている。
- [x] `TestRunMissingHashDirExitsUntrustedEnvironment` を追加する（AC-12）。不在のハッシュディレクトリを指定し、終了コードが `3` であること、実行後もディレクトリが不在であることを検証する。
- [x] `TestRunMissingHashDirMessageIdentifiesCause` を追加する（AC-13）。標準エラー出力が `hash_dir_not_found` トークンとハッシュディレクトリのパスを含み、かつハッシュ照合の失敗を示す文言を含まないことを検証する。**実装時の変更**: 照合失敗の文言はリテラルではなく `filevalidator.ErrMismatch.Error()` を参照する（同じ文字列を2箇所に持たないため）。
- [x] `TestRunFailsClosedReportsPathResolutionFailure` を追加する（AC-05）。読み取り権限のない祖先を持つハッシュディレクトリを指定し、fail-closed 終了時の標準エラー出力に解決失敗の旨と当該パスが出ることを検証する。root スキップと `t.Cleanup` での権限復帰を入れる。
- [x] `TestRunFailsClosedReportsInjectedPathResolutionFailure` を追加する（AC-05 の第二経路）。§1.4.4 の `deps.resolvePathForCheck` に、確実に `ErrPathResolution` を返す実装を注入して同じ出力を検証する。前項は root 実行時にスキップされるため、権限に依存しない根拠をもう1本用意する。**実装時の追加**: 注入する実装はエラーと併せて健全な既存ディレクトリを返す。これが「返り値を検査して続行する」実装では通ってしまう配置であり、失敗それ自体を拒否していることの根拠になる。
- [x] `TestRunExitsWithoutPanicWhenCheckerInitFails` を追加する（AC-24・AC-26）。`d.newPermChecker` に失敗を返す実装を注入し、終了コードが `3`、標準エラー出力に `permission_checker_init_failed` が出ることを検証する。ステップ 2-6 と同じ理由で、`goroutine ` の非包含は補助的な表明である旨をテストの doc コメントに英語で記す。
- [x] `TestRunUnreadableHashDirExitsUntrustedEnvironment` を追加する。読み取れないハッシュディレクトリで終了コード `3` と `hash_dir_unreadable` トークンが出ること。**実装時の変更**: 実ディレクトリの `chmod 0o000` では到達できないため（`Lstat` は成功し `NewReadOnly` も遅延エラーを持たない。読み取り失敗はファイル単位で表面化する）、また祖先を読めなくする配置は先にパス解決が拒否するため、`HashDirError()` が権限エラーを返す `fakeValidator` を注入する形にした。本番でこの遅延エラーが載るのは、パス解決とバリデータ構築の間にアクセスが失われた場合である。同じ理由づけで `cmd/record` の `TestCheckHashDirWriteSafety_RefusesWhenSiteIsUnusable` も直接呼び出しで検証している。root スキップは不要になった。
- [x] `TestRunHashDirIsNotADirectoryExitsVerificationFailed` を追加する。ハッシュディレクトリのパスが通常ファイルの場合、終了コードが `1` になること（02_architecture.md §4.3）。**実装時の変更**: §1.4.1 のとおりトークンは終了コード `3` 専用ではないため、`hash_dir_not_a_directory` が出ることを表明する（当初はトークンが出ないことを表明する予定だった）。あわせて、バリデータ構築前にこの事象が名指しされること（`Error creating validator` が出ないこと）も表明する。

- [x] （レビュー指摘による追加）`TestRunVerifiesThroughSymlinkedHashDir`: ハッシュディレクトリ自身がシンボリックリンクの場合に、拒否されずファイル単位の検証まで進むこと。解決済みパスでバリデータを構築する選択を固定する（コマンドラインのパスを渡す実装では `ErrHashPathNotDir` で終了コード `1` になる）。
- [x] （レビュー指摘による追加）`TestRunUnsearchableHashDirExitsUntrustedEnvironment`: `0o000` のハッシュディレクトリで終了コード `3` と `hash_dir_unreadable` が出ること。root スキップと権限復帰を入れる。
- [x] （レビュー指摘による追加）`TestRunAcceptsSearchOnlyHashDir`: `0o100`（検索可・一覧不可）のハッシュディレクトリが拒否されず、ファイル単位の検証まで進むこと。検索権限の確認が一覧権限を要求していないことを固定する対のテストである。root スキップと権限復帰を入れる。
- [x] （レビュー指摘による追加、PR-5）§1.4.1 で追加した終了コード `1` の3トークンの根拠テスト。`TestRunRequiresAtLeastOneFile` に `invalid_arguments` の表明を足し、`TestRunUnresolvableCheckUIDIdentifiesCause`（`deps.ensurePermissionCheckUID` に失敗を注入）と `TestRunValidatorInitFailureIdentifiesCause`（`deps.validatorFactory` に失敗を注入）を追加する。いずれも検証が1件も始まらないこと（標準出力が空）も表明する。
- [x] （実装時の追加）ステップ 3-4 の終了コード契約の変更に、既存の2テストを追随させる。`TestRunCreatesNoFilesystemEntries` の不在ケースは `1` から `3` になり、ファイル単位の検証まで届かなくなるため、「バリデータ構築まで到達した」ことの根拠をケースごとに分ける（不在ケースは `hash_dir_not_found` の出力、既存ケースは従来どおり `[1/1] <file>` の出力）。`TestRunResolvesMissingHashDirUnderSymlinkedAncestor` も `3` になるため、表明を「終了コードが `1`」から「原因トークンが `hash_dir_not_found` であって `hash_dir_permission_violation` ではない」へ移す。同テストが本来固定しているのはシンボリックリンクの祖先が違反として報告されないことであり、その観点は保たれる。

**根拠テストの自己検証**: 上記の新規テストについて、対応する実装分岐を一時的に無効化して失敗することを確認し、その旨をコミットメッセージに記す。

#### フェーズ3 完了ゲート

- [x] `make fmt` → `make test` → `make lint` が緑。

### PR-5 作成ポイント: verify exit-code contract and cause tokens

**対象ステップ**: 3-4 / 3-5

**推奨タイトル**: `feat(0164)!: report the cause of verify fail-closed exits with identification tokens`

**レビュー観点**: §6.2 の6段階の順序と、ハッシュディレクトリ側で止まったとき対象ファイル側のチェックが走らないこと / 4つの識別トークンと終了コード（`3` と `1`）の対応が文書・定数・テストで一致しているか / 不在・読み取り不能・ディレクトリでない場合の3経路が別々のトークンと終了コードに落ちること / パス解決失敗の第二経路（注入）が root 実行でも動くこと

**実装モデル要件**: frontier-required

**判定理由**: `mkplan.md` step 8 のパネルモード・トリガのうち「セキュリティゲート/移行」に該当する。不在のハッシュディレクトリが終了コード `3` になる契約変更は「終了コード 3 = 改ざんの可能性」として警報を上げている監視ルールに影響し、CHANGELOG の移行注記を伴うため。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#1029](https://github.com/isseis/go-safe-cmd-runner/pull/1029)）
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### フェーズ4: `runner`・`internal/runner`・`bootstrap` と文書

#### ステップ 4-1: 起動時 TOCTOU チェックの除外判定と件数記録

**変更ファイル**: `cmd/runner/main.go`

- [ ] `resolveStaticAbsPath`（410-419 行）を削除する。
- [ ] `runTOCTOUCheck` のパス収集ループ（427-445 行）で、`security.ClassifyCheckTarget` の戻り値を理由ごとに数え、`CheckSkipNone` のパスだけを `security.ResolvePathForCheck` に渡す。
- [ ] **`PathExpansionState` を出所ごとに宣言する**（02_architecture.md §3.2 の表）。`runtimeGlobal.ExpandedVerifyFiles` は構成上必ず展開済みなので `security.PathExpanded` を渡す。`g.VerifyFiles`・`cmd.Cmd` は未展開のテンプレートなので `config.HasVariableReference` の結果で `PathHasUnexpandedReference` と `PathExpanded` を選ぶ。判定は展開前のテンプレートに対してのみ行う（展開は冪等ではないため、展開後の値からは復元できない）。
- [ ] 429 行のコメント `Variables are already expanded so no %{ filter is needed here` を、この出所ごとの宣言が何を根拠にしているかを述べる文へ書き換える。
- [ ] ハッシュディレクトリの解決（457 行）を `security.ResolvePathForCheck` に置き換える。
- [ ] 452-456 行のコメント（`ResolveAbsPathForCheck preserves the original path when EvalSymlinks fails`）を、`ResolvePathForCheck` の実際の挙動（実在する最深の祖先まで解決し、残りを字句的に連結する）を述べる文へ書き換える。
- [ ] チェック実行後に、§1.4.3 の5属性を持つ `INFO` を1件記録する。除外が0件の実行でも同じ記録を出す。
- [ ] 除外は違反として扱わず、合否判定（460-468 行）は変更しない。

#### ステップ 4-2: `runner` の権限チェッカ生成から panic を無くす

**変更ファイル**: `cmd/runner/main.go`

- [ ] `runTOCTOUCheck` に `newPermChecker func() (isec.DirectoryPermChecker, error)` を引数として追加する（§1.4.4）。型を `security.NewDirectoryPermChecker` と同一にすることで、呼び出し元がアダプタを挟まずに渡せる。
- [ ] `runTOCTOUCheck` の内部では `newPermChecker()` を呼ぶ。
- [ ] 446-451 行の panic を削除し、生成失敗時は `logging.PreExecutionError`（`Type: logging.ErrorTypeFileAccess`、`Component: string(resource.ComponentVerification)`、`RunID: runID`）を返す。
- [ ] 呼び出し元（401 行）が `isec.NewDirectoryPermChecker` をそのまま渡すようにする。

#### ステップ 4-3: グループ実行時チェックを共有処理へ差し替え

**変更ファイル**: `internal/runner/group_executor.go`、`internal/runner/group_executor_test.go`

- [ ] 357 行と 364 行の `isec.ResolveAbsPathForCheck` 呼び出しを、`isec.ClassifyCheckTarget` による分類と `isec.ResolvePathForCheck` による解決に置き換える。件数の記録は入れない（02_architecture.md §6.3）。
- [ ] **`isec.PathExpanded` を宣言する**。グループ実行時のパスは `preExpandCommands` による展開後であり、展開済みであることは呼び出し元が構成上知っている事実である。したがって `CheckSkipVariableReference` はこの経路では返らず、除外の範囲は現行（相対パスのみ読み飛ばす）と同一に保たれる。分類の戻り値は次のように扱う。
  - `CheckSkipRelative` — 読み飛ばす（現行と同じ）。
  - `CheckSkipNone` — `ResolvePathForCheck` に渡して検査する。
- [ ] 起動時チェック（ステップ 4-1）と宣言が分かれる理由を、英語のコメントで当該箇所に残す。展開後の値に `%{` が残っていても、それはエスケープ由来の文字である可能性があり、除外の根拠にならない。除外すれば fail-closed のチェックが黙って狭まり、しかも §6.3 によりグループ側には件数記録が無いため、判定にも記録にも現れない。
- [ ] `TestRunGroupTOCTOUCheck_AbsolutePathContainingBraceIsStillChecked` を追加する。`%{` を含む絶対パスを1件渡し、フェイクチェッカが当該パスの祖先について呼ばれること（＝読み飛ばされていないこと）を表明する。除外範囲が将来広げられたときに失敗する唯一の表明である。
- [ ] `TestRunGroupTOCTOUCheck_RelativePathsSkipped`（`group_executor_test.go:3691-3703`）を書き換える。現行は実チェッカを使って `require.NoError` するだけであり、相対パスが「除外された」場合も「解決されて健全なディレクトリが検査された」場合も通るため、除外を検証できていない。**呼ばれたパスを記録するフェイクチェッカに差し替え、チェッカが1件も呼ばれないことを表明する**。これが除外と検査済みを区別できる唯一の表明である。
- [ ] 同テストのコメント `Relative paths only → CollectPermissionCheckDirs collects nothing → no violations.` を、除外の担い手が `ClassifyCheckTarget` に移ったことを述べる文へ書き換える。

#### ステップ 4-4: 特権降格のコメント追記と `ResolveAbsPathForCheck` の削除

**変更ファイル**: `cmd/runner/main.go`、`internal/security/path_resolution.go`

- [ ] `main()` の `dropStartupPrivileges` 呼び出し（163-168 行）のコメントに、saved-set-uid が変更されないこと、それにより privilege manager が必要時に再昇格できること、したがってこれが恒久降格ではないことを英語で追記する（AC-42）。
- [ ] `internal/security/path_resolution.go` から `ResolveAbsPathForCheck` を削除する（この時点で呼び出し元が無くなる）。

#### ステップ 4-5: TOCTOU チェック周辺のテストを追加・更新

**変更ファイル**: `cmd/runner/main_test.go`、`cmd/runner/startup_order_guard_test.go`

- [ ] `cmd/runner/main_test.go` に `captureLogs` ヘルパーを追加する（`cmd/verify/main_test.go:85` と同じ形）。`runTOCTOUCheck` は `slog.Default()` に記録するため、以下のログ検証テストにはこれが要る。既定ロガーはプロセス全体の状態なので、これらのテストは `t.Parallel()` を呼ばない。その制約をファイル冒頭のコメントに英語で記す。
- [ ] `TestRunTOCTOUCheck_LogsZeroSkipCounts` を追加する（AC-18）。除外が発生しない設定でチェックを実行し、`INFO` 記録が `skipped_variable_reference_paths=0` と `skipped_relative_paths=0` を含むことを検証する。
- [ ] `TestRunTOCTOUCheck_LogsSkipBreakdownByReason` を追加する（AC-17）。`%{VAR}` を含むパス1件と相対パス1件を含む設定で、`skipped_variable_reference_paths=1` と `skipped_relative_paths=1` が記録されることを検証する。
- [ ] `TestRunTOCTOUCheck_SkipDoesNotAffectVerdict` を追加する（AC-19）。除外対象のみを含む設定で違反が返らないこと、および除外対象と違反ディレクトリを併せ持つ設定で違反件数が違反ディレクトリの数と一致することを検証する。
- [ ] `TestRunTOCTOUCheck_CheckerInitFailureReturnsPreExecutionError` を追加する（AC-24・AC-25）。失敗する生成関数を渡し、panic せず `*logging.PreExecutionError` が返ることを `errors.AsType` で検証する。
- [ ] `cmd/runner/startup_order_guard_test.go` を再実行する。同テストは `cmd/runner/main.go` のソースを解析して `main` 内の呼び出し順を表明しており、ステップ 4-2（`runTOCTOUCheck` のシグネチャ変更）とステップ 4-4（`dropStartupPrivileges` 周辺のコメント追記）が同じ領域に触れる。追随が必要かを確認し、必要なら書き換える。不要だった場合もその旨を確認済みとして記録する。

### PR-6 作成ポイント: runner TOCTOU check migration

**対象ステップ**: 4-1 / 4-2 / 4-3 / 4-4 / 4-5

**推奨タイトル**: `feat(0164): move the runner TOCTOU checks onto the shared path resolution`

**レビュー観点**: 起動時（除外あり・件数記録あり）とグループ実行時（除外は相対パスのみ・件数記録なし）で除外規則が意図どおり分かれ、その理由がコメントに残っているか / 除外が違反判定に影響しないこと / 生成失敗が panic ではなく `*logging.PreExecutionError` になること / `ResolveAbsPathForCheck` の削除時点で呼び出し元が残っていないこと

**実装モデル要件**: frontier-recommended

**判定理由**: ステップ 4-3 が fail-closed な権限チェックの除外範囲を扱う高リスク段階であり、起動時とグループ実行時で規則が異なるうえ、グループ側には件数記録が無く誤りが判定にも記録にも現れないため。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### ステップ 4-6: dry-run 出力形式の分岐に `default` を入れる

**変更ファイル**: `cmd/runner/main.go`

- [ ] `newDryRunFormatter(format resource.OutputFormat) (resource.Formatter, error)` を追加し、`default` で未知の形式に対するエラーを返す。
- [ ] 572-578 行の `switch` を `newDryRunFormatter` の呼び出しに置き換え、エラー時はそれを返す。

#### ステップ 4-7: Slack 環境変数エラーの二重出力を解消

§1.5 の決定に従い、直接出力の削除のみを行う。`internal/logging` には手を入れない。

**変更ファイル**: `cmd/runner/main.go`

- [ ] `cmd/runner/main.go:284` の `fmt.Fprintln(os.Stderr, err.Error())` を削除する。`PreExecutionError` の返却はそのまま残す。
- [ ] 同箇所に、構造化ログ行にも本文が現れるため人間向けブロックとしては1回になる旨と、残る重複を [#1020](https://github.com/isseis/go-safe-cmd-runner/issues/1020) で扱う旨を、英語のコメントで残す。次に読む者が「まだ2回出ている」と見て直接出力を戻すことを防ぐためである。

#### ステップ 4-8: ログファイル名タイムスタンプを UTC に

**変更ファイル**: `internal/runner/bootstrap/logger.go`

- [ ] 181 行を `timestamp := time.Now().UTC().Format("20060102T150405Z")` に変える。書式文字列は変更しない（AC-23）。

#### ステップ 4-9: 許可 Slack ホストの IPv6 正規化を揃える

**変更ファイル**: `internal/runner/bootstrap/config.go`

- [ ] 42 行を `bareHost = strings.ToLower(u.Hostname())` に変える。
- [ ] 22-29 行の doc コメントに、IPv6 リテラルも小文字化することを追記する。

#### ステップ 4-10: dry-run・Slack・`bootstrap` のテストを追加・更新

**変更ファイル**: `cmd/runner/main_test.go`、`cmd/runner/integration_pre_execution_error_test.go`、`internal/runner/bootstrap/logger_test.go`、`internal/runner/bootstrap/config_test.go`

- [ ] `TestNewDryRunFormatter_KnownFormats` を追加する（AC-30）。`resource.OutputFormatText` と `resource.OutputFormatJSON` に対して期待する型のフォーマッタが返り、エラーが無いこと。
- [ ] `TestNewDryRunFormatter_UnknownFormatReturnsError` を追加する（AC-29）。`cli.ParseDryRunOutputFormat` を経由せず不正な `resource.OutputFormat` 値を直接渡し、nil でないエラーが返ること。
- [ ] `TestE2E_SlackWebhookEnvErrorPrintedOnce` を `cmd/runner/integration_pre_execution_error_test.go` に追加する（AC-31〜AC-33）。**子プロセスの環境を明示的に組み立てる**: `os.Environ()` から `GSCR_SLACK_` で始まる変数をすべて取り除いたうえで `GSCR_SLACK_WEBHOOK_URL_SUCCESS` のみを設定する。既存の兄弟テストのように `append(os.Environ(), ...)` とすると、開発者や CI が `GSCR_SLACK_WEBHOOK_URL_ERROR` を export している環境で `ValidateSlackWebhookEnv` が成功してしまい、3つの表明がいずれも別の失敗を見ることになる。実行は `go run . -config <valid.toml> -dry-run`（webhook 未設定のまま起動検証を通すため、既存の E2E テストと同様に `-dry-run` を付ける）。まず終了コードが `1` であることを表明する（AC-33）。環境が漏れて別経路に入った場合に静かに通らないよう、これを最初に見る。次に、§1.5 のとおり**標準エラー出力から `common.PreExecErrorAttrs.ErrorMessage` の属性キー（`error_message=`）を含む行を除いたうえで**、`GSCR_SLACK_WEBHOOK_URL_SUCCESS is set but GSCR_SLACK_WEBHOOK_URL_ERROR is not.` の出現回数がちょうど1であることを検証する（AC-31）。**選別条件を `level=ERROR` にしてはならない**: この出力は `SetupLogging` 前の組み込み既定ハンドラによるもので `level=` 表記を持たないため、1行も除外されず、実装が正しくてもこの表明は失敗する（§1.5）。除外の効きめを自己点検するため、除外前の行数と除外後の行数が異なることも併せて表明する。除外した構造化ログ行に同じ本文が残ることは既知であり、[#1020](https://github.com/isseis/go-safe-cmd-runner/issues/1020) で扱う。この表明は、直接出力が戻された場合に2になって失敗する。最後に、除外前の標準エラー出力全体が `export GSCR_SLACK_WEBHOOK_URL_ERROR=` を含むことを検証する（AC-32）。除外の意図と #1020 との関係を、テストの doc コメントに英語で記す。
- [ ] `TestSetupLoggerWithConfig_LogFileNameTimestampIsUTC` を `internal/runner/bootstrap/logger_test.go` に追加する（AC-21〜AC-23）。`time.Local` を UTC+9 相当のロケーションに差し替えたうえで（`t.Cleanup` で復元）、ログディレクトリに作られたファイル名を読み、`<hostname>_<timestamp>_<runID>.json` の3要素構成であること、タイムスタンプ部が `20060102T150405Z` の書式に合致し 16 文字であること、その値を UTC としてパースした結果が実行時刻と 1 分以内で一致することを検証する。`time.Local` はプロセス全体の状態なので `t.Parallel()` を呼ばない。
- [ ] `internal/runner/bootstrap/config_test.go: TestNormalizeSlackAllowedHost` の表に、`[2001:DB8::1]` → `2001:db8::1` と `[2001:db8::1]` → `2001:db8::1` の2行を追加する（AC-34）。
- [ ] `TestNormalizeSlackAllowedHost_UppercaseIPv6PassesWebhookURLValidation` を追加する（AC-35）。大文字 IPv6 を正規化した値を許可ホストとして `internal/logging` の webhook URL 検証に渡し、同じアドレスを含む URL が許可されること。
- [ ] `TestNormalizeSlackAllowedHost_UppercaseIPv6IsRedacted` を追加する（AC-36）。同じ正規化結果から秘匿パターンを組み立て、当該 host を含む webhook URL が秘匿されること。
- [ ] AC-35・AC-36 の2テストは、下流が大文字小文字を区別しないため変更前でも通る（02_architecture.md §6.7）。退行防止の位置づけであることを、各テストの doc コメントに英語で明記する。

#### フェーズ4 完了ゲート

- [ ] `make fmt` → `make test` → `make lint` が緑。
- [ ] `make deadcode` が新たな未使用シンボルを報告しない（フェーズ1で想定内としたシンボルも、この時点ではすべて呼び出し元を持つ）。

### PR-7 作成ポイント: dry-run, Slack output and bootstrap fixes

**対象ステップ**: 4-6 / 4-7 / 4-8 / 4-9 / 4-10

**推奨タイトル**: `fix(0164): harden dry-run format selection and startup output`

**レビュー観点**: `newDryRunFormatter` の `default` が未知の形式でエラーを返し、既知の2形式の出力が変わらないこと / Slack 案内文が人間向けブロックに1回だけ出ること（選別条件が `error_message=` であること） / ログファイル名の書式文字列と3要素構成が変わらないこと / IPv6 リテラルの小文字化が下流の検証と秘匿の双方で効くこと

**実装モデル要件**: frontier-recommended

**判定理由**: 本番コードの修正はいずれも数行だが、ステップ 4-10 が本タスク唯一の新規 E2E テスト（子プロセスの環境を組み立て、標準エラー出力を行単位で選別する）と、`time.Local` をプロセス全体で差し替えるテストを含み、統合テストの面が大きいため。§1.5 が記録しているとおり、この選別条件は一度取り違えている。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### ステップ 4-11: 利用者向け文書の更新

**変更ファイル**: `docs/user/verify_command.ja.md`、`docs/user/runner_command.ja.md`、`docs/user/record_command.ja.md`

- [ ] `verify_command.ja.md` の終了コード表（97-104 行）で、`3` の意味を「(a) ハッシュディレクトリ側の TOCTOU 権限違反、(b) ハッシュディレクトリの不在または読み取り不能、(c) 権限チェッカの初期化失敗。いずれも検証を1件も実施していない」に改める。`1` の意味に「ハッシュディレクトリのパスがディレクトリでない場合」を追記する。
- [ ] 同表の直後に、§1.4.1 の識別トークン一覧（トークン・原因・対処）を表として追加する。
- [ ] 104 行の段落に、**「`verify` はハッシュディレクトリを作成しません。」という文をそのまま**追記する（§7 の AC-43 の検索がこの文言を根拠にするため、表記を先に固定する）。
- [ ] 208 行付近の「注意事項」に、ハッシュディレクトリが存在しない場合の終了コードが `3` であること、`record` で記録を作る必要があることを追記する。
- [ ] 641-680 行の `robust-verification.sh` 例で、`EXIT_CODE -eq 3` の分岐を識別トークンによる原因分けに書き換える。
- [ ] **書き換えた `robust-verification.sh` を実際に走らせて確認する**。少なくとも「ハッシュディレクトリ不在」と「ハッシュディレクトリ側の権限違反」の2条件を一時ディレクトリで作り、スクリプトが意図した分岐に入ることを確かめる。分岐が増えた手順書を、実行せずに載せない。
- [ ] `runner_command.ja.md` の 854 行の例を、実際の書式（`T` 区切り・末尾 `Z`・UTC）に合わせて `myhost_20260805T140000Z_01K2YK812JA735M4TWZ6BK0JH9.json` に修正する。
- [ ] 849 行の命名規則の直後に、タイムスタンプが UTC であることを1文で追記する。
- [ ] `record_command.ja.md` の 199-206 行「指定したディレクトリが存在しない場合、自動的に作成されます（権限: 0700）」を、本タスク後の挙動に合わせて改める。作成が権限チェックの通過後に行われること、作成先（実在する最深の祖先）が world-writable な場合は作成せずエラー終了すること、その場合は利用者が先にディレクトリを作れば通ることを記す。211 行は変更しない。
- [x] `record_command.ja.md` の「権限について」節に、分離運用（`record` 実行者と `runner` 実行者が異なる構成）の設定手順を追加する（AC-47）。`runner` が実行者の権限でハッシュを読むこと、`chgrp` + `0750` が必要であること、それが起動時チェックを通ること（拒否されるのは書き込み許可のみ）、グループへの書き込みと所有者の付け替えは行わないことを記す。
- [x] 同ファイルの移行案内（`0o750` からのアップグレード）に、分離運用では狭めない旨を併記する（AC-47・AC-45）。
- [ ] **地の確認**: 一時ディレクトリに対して `record`・`verify` を実行し、終了コードと標準エラー出力のトークンが文書の記述と一致することを確認する。ログファイル名については、`internal/runner/bootstrap` のテストが生成した実ファイル名が文書の例と同じ形であることを確認する。
- [ ] 英語版 `docs/user/verify_command.md`・`docs/user/runner_command.md`・`docs/user/record_command.md` を `/mktrans` で反映する。

#### ステップ 4-12: CHANGELOG と用語集

**変更ファイル**: `CHANGELOG.ja.md`、`docs/translation_glossary.md`

`## [未リリース]` 節に、以下の見出しをこの文言で立てる（§7 の AC-45 の検索がこの見出しを根拠にする）。

- [ ] 見出し「`verify`: ハッシュディレクトリを作成しなくなりました」 — 不在時は終了コード `3` で終了すること、識別トークンで原因を分けられること、「終了コード 3 = 改ざんの可能性」として警報を上げている監視ルールが未整備のホストで発報するようになることを記す。
- [ ] 見出し「ログファイル名のタイムスタンプが UTC になりました」 — 02_architecture.md §6.4 の移行時の注意（UTC より進んだタイムゾーンのホストで、移行直後に辞書順と時系列が一致しない期間が生じる）を含める。
- [ ] 見出し「パス解決の変更により新たに権限違反が検出されることがあります」 — アップグレード前の判定手順として、ハッシュディレクトリと検証対象のパスに `readlink -f` を適用し、その実体の祖先の権限を確認する手順を示す（02_architecture.md §9）。
- [ ] 見出し「新規作成するハッシュディレクトリのパーミッションが 0700 になりました」 — 既存ディレクトリは変わらないことを添える。
- [ ] 見出し「`record`: world-writable な場所への新規ハッシュディレクトリ作成を拒否します」 — 本番既定のハッシュディレクトリは該当しないこと、先に利用者自身がディレクトリを作れば通ることを添える。
- [ ] **地の確認**: CHANGELOG に記した `readlink -f` を用いた判定手順を実際に実行し、記載どおりの出力になることを確認する。
- [ ] `docs/translation_glossary.md` に「識別トークン / identification token」を追加する。§1.4.1 で導入した概念であり、`verify` の利用者向け文書と CHANGELOG の両方に現れるため、訳語を固定する必要がある。既存項目に同義のものが無いことを確認したうえで追加する。
- [ ] 「読み取り専用バリデータ / read-only validator」「除外理由 / skip reason」についても、既存項目で足りるかを判断する。追加した場合はそれを、追加不要と判断した場合はその理由を、Document Status の Comments に記録する。
- [ ] 英語版 `CHANGELOG.md` を `/mktrans` で反映する。
- [ ] `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` の §2・§3 から、本タスクで解消した 11 件（L-1〜L-5・L-7・I-1〜I-5）を消し込む。追跡文書の更新も本タスクの成果物であり、どの PR にも属さないまま残さない。

### PR-8 作成ポイント: user documentation and CHANGELOG

**対象ステップ**: 4-11 / 4-12

**推奨タイトル**: `docs(0164): document the verify exit codes, UTC log timestamps and hash directory changes`

**レビュー観点**: 終了コード表と識別トークン一覧が実装と一致していること（実行による地の確認が済んでいるか） / `robust-verification.sh` を実際に走らせて分岐を確認したか / CHANGELOG の5見出しが §7 の検索文言どおりか / 日本語版と英語版が同じ節に反映されているか

**実装モデル要件**: standard

**判定理由**: 実装済みの挙動を文書へ反映する作業であり、未決の方式・パネルモードのトリガ・孤立した高リスク段階のいずれにも該当しない。

**マージ後の作業**: issue [#986](https://github.com/isseis/go-safe-cmd-runner/issues/986) をクローズする（§10）。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 内容 | 成果物 | 対応フェーズ |
|---|---|---|---|
| M1 | 共有処理が揃い、既存の呼び出し元が新しい戻り値に追随している | `ResolvePathForCheck`・`ResolveAllForCheck`・`ClassifyCheckTarget`・`DeepestExistingAncestor`・`TOCTOUCheckResult`・`CreateReadOnlyValidator`・`fileanalysis.HashDirPerm`・`Validator.HashDirError` | フェーズ1 |
| M2 | `record` が共有処理を使い、書き込みが権限チェック通過後に限られる | `cmd/record/main.go` と対応テスト | フェーズ2 |
| M3 | `verify` が読み取り専用になり、`deps` 様式へ移行している | `cmd/verify/main.go` と書き換え済みテスト、`CreateValidator` の削除 | フェーズ3 |
| M4 | `runner`・`bootstrap` の残項目と文書が揃っている | `cmd/runner/main.go`・`internal/runner/group_executor.go`・`bootstrap` と文書一式 | フェーズ4 |

フェーズ2とフェーズ3は互いに独立しており、順序を入れ替えてもよい（02_architecture.md §8）。`internal/runner/group_executor.go` の移行（ステップ 4-3）は起動時チェックの移行（ステップ 4-1）と同じリリースに含める。

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1-1 / 1-2 / 1-3 | `internal/security` にパス解決・除外分類・`TOCTOUCheckResult` を追加し、既存の呼び出し元5箇所を追随させる | frontier-recommended |
| PR-2 | 1-4 / 1-5 / 1-6 | `filevalidator.HashDirError`、`cmdcommon` の共有ヘルパー、`fileanalysis.HashDirPerm`（`0o700`） | standard |
| PR-3 | 2-1 / 2-2 / 2-3 / 2-4 / 2-5 / 2-6 | `record` を共有処理へ移し、ハッシュディレクトリの作成を権限チェック通過後に限り、sticky world-writable な作成先を拒否する | frontier-required |
| PR-4 | 3-1 / 3-2 / 3-3 | `verify` の書き込み副作用を断ち、`deps` 様式へ移行する（挙動は非作成化のみ） | frontier-recommended |
| PR-5 | 3-4 / 3-5 | `verify` の起動判定を6段階へ組み替え、終了コードと識別トークンで fail-closed の原因を分ける | frontier-required |
| PR-6 | 4-1 / 4-2 / 4-3 / 4-4 / 4-5 | `runner` の起動時チェックとグループ実行時チェックを共有処理へ移し、panic を排し、`ResolveAbsPathForCheck` を削除する | frontier-recommended |
| PR-7 | 4-6 / 4-7 / 4-8 / 4-9 / 4-10 | dry-run 形式の `default`、Slack 二重出力の解消、ログファイル名の UTC 化、IPv6 正規化 | frontier-recommended |
| PR-8 | 4-11 / 4-12 | 利用者向け文書・CHANGELOG・用語集の更新と、0149 の残件一覧の消し込み | standard |

PR-1 と PR-2 はフェーズ1（`internal/`）で、これを利用する PR-3〜PR-7（`cmd/` と `internal/runner`）より先にマージする。PR-3（`record`）と PR-4・PR-5（`verify`）は互いに独立しており、順序を入れ替えてよい。PR-4 は挙動を変えない移行、PR-5 は終了コードの契約変更であり、後者だけを差し戻せるように分けている。PR-8 は挙動を確定させる PR-3〜PR-7 のあとに置く。
---

## 4. テスト戦略

### 4.1 単体テスト

02_architecture.md §7.2 の方針に従い、以下を対象とする。

- `internal/security`: `ResolvePathForCheck` の4経路（全体が実在・途中まで実在・`ENOENT` 以外の失敗・`os.Getwd` の失敗）と相対パスの絶対化、`ResolveAllForCheck` の失敗件数と `WARN` 記録と返却パス、`ClassifyCheckTarget` の3値、`TOCTOUCheckResult` の件数、`DeepestExistingAncestor`。
- `internal/cmdcommon`: `CreateReadOnlyValidator` の非作成性。権限チェッカ生成にラッパーを置かないため、`cmdcommon` 側にその単体テストは無い（失敗経路は各コマンドの注入口で検証する）。
- `internal/fileanalysis`: 新規作成ディレクトリのパーミッション。
- `internal/filevalidator`: `HashDirError()` の3状態。
- `internal/runner/bootstrap`: ログファイル名の UTC 化と構成の不変、IPv6 正規化。
- `cmd/runner`: `newDryRunFormatter` の既知・未知の形式。

### 4.2 統合テスト

- `cmd/record`: 権限違反時の非作成、違反なし時の作成と記録生成、不在時のチェック実施、作成失敗時の報告、sticky world-writable な親の下での拒否と既存ディレクトリでの通過、ハッシュディレクトリ自身が sticky world-writable な場合の拒否、解決できないパスの拒否、サブディレクトリ構築より前の作成、シンボリックリンク先の祖先の違反検出。
- `cmd/verify`: 不在時の終了コードとメッセージ、いかなる引数でも新規作成が起きないこと、パス解決失敗の標準エラー出力への提示、読み取り不能時とディレクトリでない場合の終了コード。
- `cmd/runner`: 除外件数の記録（0件・1件以上）、除外が合否に影響しないこと、Slack 環境変数エラーが1回だけ出ること。

### 4.3 セキュリティテスト

02_architecture.md §7.4 の5点をフェーズ2のテストに含める。

| §7.4 の項目 | 対応するテスト |
|---|---|
| リンク経由の祖先が検査されること | `cmd/record/main_test.go::TestRunTOCTOU_ReportsViolationBehindSymlinkedAncestor` |
| 作成先が守られていない場合に拒否すること | 同`::TestRun_RejectsHashDirCreationUnderStickyWorldWritableParent` と `::TestRun_AllowsExistingHashDirUnderStickyWorldWritableParent` |
| ハッシュディレクトリ自身が world-writable な場合に拒否すること | 同`::TestRun_RejectsExistingStickyWorldWritableHashDir` |
| 解決できないハッシュディレクトリパスを拒否すること | 同`::TestRun_RefusesUnresolvableHashDirPath` |
| サブディレクトリ構築より前にハッシュディレクトリが作られること | 同`::TestRun_CreatesHashDirBeforeSubdirectories` |

判定できない場合の拒否（02_architecture.md §5.2 の「判定できない場合も違反として扱う」表の上3行）は、本番では解決直後の競合でしか到達しないため、`cmd/record/main_test.go::TestCheckHashDirWriteSafety_RefusesWhenSiteIsUnusable` が `checkHashDirWriteSafety` を直接呼んで確認する。3経路（相対パス、リンク切れの祖先、読み取り不能な祖先）を持ち、リンク切れの経路が `os.Lstat` と `IsDir` の選択を固定する。読み取り不能の経路は §4.6 の root スキップ規約に従う。

### 4.4 退行防止

表に載らないテストのうち、`internal/cmdcommon/common_test.go::TestCreateReadOnlyValidator_ExistingHashDirectoryHasNoDeferredError`、および `internal/filevalidator/validator_test.go::TestHashDirError_*` の3本は、特定の AC に直接対応しない下支え API のテストである。これらが確かめるのはフェーズ1で足した API 自身の契約であり、その API を使う側の挙動が AC-12・AC-13・AC-14・AC-24 の各行で検証される。

要件定義書が「既存テストが通過する」と定める AC-07・AC-16・AC-20・AC-30・AC-37・AC-41 は、既存テストの通過をもって確認する。ただし §1.3 で挙げた書き換え対象テストについては、書き換え後も元の観点を検証していることを §7 の表で明示する。AC-20 については、既存の `TestRunGroupTOCTOUCheck_RelativePathsSkipped` が除外を検証できていないため、ステップ 4-3 で表明を入れ替える。

### 4.5 テストの自己検証

新規テストは、対応する実装分岐を一時的に無効化して失敗することを確認してからコミットする。確認した旨をコミットメッセージに記す。

### 4.6 権限に依存するテストの扱い

読み取り不能ディレクトリを使うテストは、root で実行すると `chmod 0o000` が `EACCES` を生まないため、そのままでは失敗する。該当するテストはすべて `syscall.Geteuid() == 0` で `t.Skip` し、権限を落とした直後に `t.Cleanup` で権限を戻す（`t.TempDir` の自動削除が失敗しないようにするため）。対象は `TestHashDirError_UnreadableDirectoryReturnsPermissionError`・`TestResolvePathForCheck_UnreadableAncestorReturnsErrPathResolution`・`TestResolveAllForCheck_WarnsOncePerFailure`・`TestRunFailsClosedReportsPathResolutionFailure`・`TestRunUnsearchableHashDirExitsUntrustedEnvironment`・`TestRunAcceptsSearchOnlyHashDir` の6件。**実装時の変更**: `TestRunUnreadableHashDirExitsUntrustedEnvironment` は当初この一覧に含めていたが、実ディレクトリの権限では当該分岐に到達できないことが分かり、遅延エラーを注入する形にした（ステップ 3-5）。権限に依存しないため、この一覧から外した。

スキップによって根拠が消える受け入れ基準は AC-05 のみである。これに対しては、権限に依存しない第二の根拠（解決関数を注入する `TestRunFailsClosedReportsInjectedPathResolutionFailure`）を用意する（ステップ 3-5）。

### 4.7 テストヘルパー

クロスパッケージのテストヘルパーやモックは必要としない。パッケージ内には `internal/security/test_helpers_getwd.go` を1つだけ追加する（§1.4.5 の `getwdHook`）。テストヘルパー配置規約の分類 B に当たり、ファイル名と `//go:build test` の付与はその規約に従う。`cmd/record`・`cmd/verify` の `fakeDirPermChecker` は互いに別の `main` パッケージの非公開型であり、`testutil/` へ移せない（既存コメントが述べているとおり）。ステップ 2-6・3-5 で各パッケージに追加するチェッカ生成失敗のスタブも同じ制約を受けるため、`fakeDirPermChecker` と同様に「意図的な重複である」旨のコメントを英語で添える。上記以外に `test_helpers.go` の追加は行わない。

---

## 5. リスク管理

| リスク | 影響 | 対処 |
|---|---|---|
| 構造化ログ行に本文が残ることを、後から見た者が退行と誤認して直接出力を戻す | 二重出力が復活する | ステップ 4-7 で削除箇所に英語のコメントを残し、[#1020](https://github.com/isseis/go-safe-cmd-runner/issues/1020) を参照させる。AC-31 のテストは直接出力が戻ると失敗する |
| `RunTOCTOUPermissionCheck` の戻り値変更が5つの呼び出し元とテスト4箇所に波及する | コンパイルエラーの取りこぼし | ステップ 1-2 を単独のコミットにし、`make test` が緑になるまで次へ進まない |
| `cmd/verify/main_test.go` の全面書き換えで、既存の観点が落ちる | 退行の見落とし | §7 のトレーサビリティ表で書き換え前後の観点を対応付ける。書き換え前後で `go test -tags test -coverprofile=... ./cmd/verify/... && go tool cover -func=...` を比較する |
| sticky world-writable の判定（ステップ 2-5）が正常な運用を拒否する | `record` が使えなくなる環境が出る。判定は作成先だけでなくハッシュディレクトリ自身にも及ぶため、`/tmp/hashes` を `1777` で運用していた環境は既存ディレクトリでも通らなくなる | 本番既定のハッシュディレクトリ（`/usr/local/etc/go-safe-cmd-runner/hashes`）は該当しない。対の2テスト（拒否と通過）で判定範囲を固定する。回避手順（`chmod go-w`、または他者が書けない場所へ移す）は標準エラー出力に出したうえで、CHANGELOG と `record_command.ja.md` に記す |
| ログファイル名の UTC 化が収集スクリプトを壊す | 移行直後に最新ファイルの誤認 | 書式文字列と桁数を変えない。CHANGELOG に移行時の注意を記す |
| 識別トークンの文字列が文書とコードで食い違う | 呼び出し元スクリプトが原因を分けられない | トークンを Go の名前付き定数として定義し、テストがその定数を参照する。文書との一致は §8 の横断検索で確認する |
| root 実行の CI で権限依存テストが一斉にスキップされる | 気づかないまま根拠が失われる | §4.6 のとおり、スキップで根拠が消える AC-05 にのみ権限非依存の第二経路を用意する |

---

## 6. 実装チェックリスト

PR 単位で進捗を追う。各 PR の対象ステップと作業内容は §3.2 の表と、各ステップ群の末尾にある「PR-N 作成ポイント」節を参照する。

- [ ] PR-1 マージ済み（対象ステップ: 1-1 / 1-2 / 1-3）
- [ ] PR-2 マージ済み（対象ステップ: 1-4 / 1-5 / 1-6、フェーズ1 完了ゲートを含む）
- [ ] PR-3 マージ済み（対象ステップ: 2-1 / 2-2 / 2-3 / 2-4 / 2-5 / 2-6、フェーズ2 完了ゲートを含む）
- [ ] PR-4 マージ済み（対象ステップ: 3-1 / 3-2 / 3-3）
- [ ] PR-5 マージ済み（対象ステップ: 3-4 / 3-5、フェーズ3 完了ゲートを含む）
- [ ] PR-6 マージ済み（対象ステップ: 4-1 / 4-2 / 4-3 / 4-4 / 4-5）
- [ ] PR-7 マージ済み（対象ステップ: 4-6 / 4-7 / 4-8 / 4-9 / 4-10、フェーズ4 完了ゲートを含む）
- [ ] PR-8 マージ済み（対象ステップ: 4-11 / 4-12）
---

## 7. 受け入れ基準の検証

`種別` 列は `test`（実行可能で、挙動が誤っていれば失敗する）、`static`（`rg` またはコンパイルによる静的確認）、`manual`（実行して目視確認）を表す。

検索は `rg` の `-e` を繰り返す形で書く。`rg` の既定の正規表現では `\|` は選択肢の区切りではなく**リテラルのパイプ文字**として扱われるため、`"A\|B"` と書くと常に一致0件になり、「一致0件」を期待する検証がすべて素通りする。`-e` を並べる形にすればこの取り違えが起きない。

| AC | 種別 | 検証手段 | 期待結果 |
|---|---|---|---|
| AC-01 | static | `rg -n -e ResolveAbsPathForCheck -e resolveStaticAbsPath --glob '*.go' .` | 一致0件 |
| AC-01 | static | `rg -n -e EvalSymlinks --glob '!*_test.go' cmd/record cmd/verify cmd/runner` | 一致0件（本番コードでの解決は共有処理のみが行う。現状は5件＝`cmd/record/main.go` 2件・`cmd/verify/main.go` 2件と、`cmd/runner/main.go:455` のコメント1件。コメントはステップ 4-1 で書き換える） |
| AC-01 | static | `rg -c -e ResolvePathForCheck -e ResolveAllForCheck cmd/record/main.go cmd/verify/main.go cmd/runner/main.go` | 3ファイルすべてが1件以上で報告される |
| AC-02 | test | `internal/security/path_resolution_test.go::TestResolvePathForCheck_PartiallyExistingPath` | — |
| AC-03 | test | `internal/security/path_resolution_test.go::TestResolvePathForCheck_SymlinkedAncestorOfMissingPath` | — |
| AC-03 | test | `cmd/record/main_test.go::TestRunTOCTOU_ReportsViolationBehindSymlinkedAncestor` | — |
| AC-04 | test | `internal/security/path_resolution_test.go::TestResolveAllForCheck_WarnsOncePerFailure` | — |
| AC-04 | test | `internal/security/path_resolution_test.go::TestResolveAllForCheck_NoWarnOnSuccessfulResolution` | — |
| AC-05 | test | `cmd/verify/main_test.go::TestRunFailsClosedReportsInjectedPathResolutionFailure`（権限に依存しない主たる根拠） | — |
| AC-05 | test | `cmd/verify/main_test.go::TestRunFailsClosedReportsPathResolutionFailure`（実際の権限不足による確認。root ではスキップ） | — |
| AC-06 | test | `internal/security/path_resolution_test.go::TestResolvePathForCheck_RelativePathUsesWorkingDirectory` | — |
| AC-06 | static | AC-01 の3行目と同じ（3コマンドが同一関数を使う） | 3ファイルすべてが1件以上 |
| AC-07 | test | `cmd/record/main_test.go::TestRunTOCTOU_ReportsViolationBehindSymlinkedAncestor`（リンク経由でも同じ形で報告される） | — |
| AC-07 | test | `cmd/record/main_test.go::TestRunTOCTOU_FailsClosedOnWorldWritableDir`、`cmd/verify/main_test.go::TestRunFailsClosedOnHashDirViolation_AncestorViolation`（リンクを含まない場合。書き換え後も通過） | — |
| AC-08 | test | `cmd/record/main_test.go::TestRunTOCTOU_HashDirNotCreatedOnViolation` | — |
| AC-09 | test | `cmd/record/main_test.go::TestRun_CreatesHashDirAfterPermissionCheckPasses` | — |
| AC-10 | test | `cmd/record/main_test.go::TestRunTOCTOU_ChecksAncestorsWhenHashDirMissing` | — |
| AC-11 | test | `cmd/record/main_test.go::TestRun_ReportsHashDirCreationFailure` | — |
| AC-12 | test | `cmd/verify/main_test.go::TestRunMissingHashDirExitsUntrustedEnvironment` | — |
| AC-13 | test | `cmd/verify/main_test.go::TestRunMissingHashDirMessageIdentifiesCause` | — |
| AC-14 | test | `cmd/verify/main_test.go::TestRunCreatesNoFilesystemEntries`（`filepath.WalkDir` による部分木全体の比較） | — |
| AC-14 | test | `internal/cmdcommon/common_test.go::TestCreateReadOnlyValidator_DoesNotCreateHashDirectory` | — |
| AC-15 | test | `internal/fileanalysis/file_analysis_store_test.go::TestNewStore_CreatesDirectory`（葉と中間ディレクトリの双方が `0o700` であることを表明する。専用テストを別に立てず既存テストへ表明を足したのは、同一のリテラルと手順を繰り返すだけの複製になるため） | — |
| AC-15 | static | `rg -n -e hashDirPermissions -e dirPermission cmd internal` | 一致0件（現状は `cmd/record/main.go` 2件・`cmd/verify/main.go` 2件・`internal/fileanalysis/file_analysis_store.go` 3件〈17 行の doc コメント・19 行の定義・48 行の使用〉の計7件） |
| AC-15 | static | `rg -n -e 'HashDirPerm\s+os\.FileMode\s*=' --glob '!*_test.go' internal` | 一致1件（`internal/fileanalysis/file_analysis_store.go`） |
| AC-15 | static | `rg -n -e 'MkdirAll\(' --glob '!*_test.go' cmd internal` を目視し、ハッシュディレクトリ自身を作る呼び出しがステップ 2-3 の1箇所と `fileanalysis.NewStore` のみであることを確認する。`libccache`・`dynamicanalysis` の3件は配下のサブディレクトリで対象外 | 別名の重複定数が無いこと |
| AC-16 | test | `cmd/verify/main_test.go::TestRunProcessesMultipleFiles`、`::TestRunReportsFailuresAndContinues` | 書き換え後も通過 |
| AC-17 | test | `cmd/runner/main_test.go::TestRunTOCTOUCheck_LogsSkipBreakdownByReason` | — |
| AC-18 | test | `cmd/runner/main_test.go::TestRunTOCTOUCheck_LogsZeroSkipCounts` | — |
| AC-19 | test | `cmd/runner/main_test.go::TestRunTOCTOUCheck_SkipDoesNotAffectVerdict` | — |
| AC-20 | test | `internal/runner/group_executor_test.go::TestRunGroupTOCTOUCheck_RelativePathsSkipped`（ステップ 4-3 で、チェッカが1件も呼ばれないことを表明する形に入れ替えたもの） | — |
| AC-20 | test | `internal/runner/group_executor_test.go::TestRunGroupTOCTOUCheck_AbsolutePathWithVariableReferenceIsStillChecked`（除外範囲が現行より広がっていないこと。ステップ 4-3） | — |
| AC-20 | test | `cmd/runner/integration_toctou_test.go::TestE2E_TOCTOU_RunnerFailsOnWorldWritableVerifyFilesDir`、`internal/runner/group_executor_test.go::TestRunGroupTOCTOUCheck_ViolationReturnsError` | 通過 |
| AC-21 | test | `internal/runner/bootstrap/logger_test.go::TestSetupLoggerWithConfig_LogFileNameTimestampIsUTC` | — |
| AC-22 | test | 同上（`time.Local` を UTC+9 に差し替えた状態で検証する） | — |
| AC-23 | test | 同上（3要素構成・区切り文字 `_`・タイムスタンプ 16 文字を検証する） | — |
| AC-24 | test | `cmd/record/main_test.go::TestRun_ExitsWithoutPanicWhenCheckerInitFails` | — |
| AC-24 | test | `cmd/verify/main_test.go::TestRunExitsWithoutPanicWhenCheckerInitFails` | — |
| AC-24 | test | `cmd/runner/main_test.go::TestRunTOCTOUCheck_CheckerInitFailureReturnsPreExecutionError` | — |
| AC-25 | test | `cmd/runner/main_test.go::TestRunTOCTOUCheck_CheckerInitFailureReturnsPreExecutionError`（`*logging.PreExecutionError` が返ることを `errors.AsType` で検証） | — |
| AC-26 | test | `cmd/record/main_test.go::TestRun_ExitsWithoutPanicWhenCheckerInitFails`、`cmd/verify/main_test.go::TestRunExitsWithoutPanicWhenCheckerInitFails`（標準エラー出力にエラーが出ること） | — |
| AC-27 | static | `rg -n -e 'security validator initialisation failed' cmd internal` | 一致0件（現状は3件） |
| AC-27 | static | `rg -n -A6 -e 'NewDirectoryPermChecker\(\)' cmd \| rg -n 'panic\('` | 一致0件（重複していたのは生成の呼び出しではなく panic ブロックである。生成関数は `internal/security` に1つしかなく、3コマンドはその同一の関数を注入口の既定値として指す） |
| AC-27 | static | `rg -n -e 'newPermChecker' cmd` | `cmd/record`・`cmd/verify` の `defaultDeps()` と `cmd/runner` の `runTOCTOUCheck` 呼び出しが、いずれも既定値として `security.NewDirectoryPermChecker`（`cmd/runner` は `isec.` 別名）を渡していること。`cmd/runner` は `internal/security` を `isec` として取り込むため、`security\.` だけの検索では 446 行を捕らえられない |
| AC-28 | static | `rg -n -e 'the policy declaration in init' cmd/verify/main.go` | 一致1件 |
| AC-28 | static | `rg -n -e 'checker initialisation in checkDirPermissions' cmd/verify/main.go` | 一致0件 |
| AC-29 | test | `cmd/runner/main_test.go::TestNewDryRunFormatter_UnknownFormatReturnsError` | — |
| AC-30 | test | `cmd/runner/main_test.go::TestNewDryRunFormatter_KnownFormats` | — |
| AC-30 | test | `cmd/runner/dry_run_integration_test.go::TestDryRunTextOutput_Unchanged`、`::TestDryRunJSONOutput_DetailLevels` | 通過 |
| AC-31 | test | `cmd/runner/integration_pre_execution_error_test.go::TestE2E_SlackWebhookEnvErrorPrintedOnce`（`error_message=` を含む行を除いた標準エラー出力で、案内文の出現回数がちょうど1。§1.5 の解釈による） | — |
| AC-32 | test | 同上（除外前の標準エラー出力全体が `export GSCR_SLACK_WEBHOOK_URL_ERROR=` を含むことを検証） | — |
| AC-33 | test | 同上（終了コード `1`） | — |
| AC-34 | test | `internal/runner/bootstrap/config_test.go::TestNormalizeSlackAllowedHost`（大文字・小文字の IPv6 が同一結果になる2行） | — |
| AC-35 | test | `internal/runner/bootstrap/config_test.go::TestNormalizeSlackAllowedHost_UppercaseIPv6PassesWebhookURLValidation` | — |
| AC-36 | test | `internal/runner/bootstrap/config_test.go::TestNormalizeSlackAllowedHost_UppercaseIPv6IsRedacted` | — |
| AC-37 | test | `internal/runner/bootstrap/config_test.go::TestNormalizeSlackAllowedHost`（既存のホスト名・不正値の行） | 通過 |
| AC-38 | static | `rg -n -e validatorFactory -e mkdirAll -e ensurePermissionCheckUID -e toctouChecker cmd/verify/main.go` | `toctouChecker` と `mkdirAll` は一致0件（`deps` のフィールドとしても残さない）。`validatorFactory` と `ensurePermissionCheckUID` は `deps` のフィールド名としての一致のみで、パッケージレベル変数としての宣言は0件（目視で切り分ける） |
| AC-38 | test | `cmd/verify/main_test.go::TestRunProcessesMultipleFiles`（`deps` 経由で差し替える形になっている） | — |
| AC-39 | static | `rg -n -e overrideValidatorFactory -e overrideTOCTOUChecker cmd/verify/main_test.go` | 一致0件 |
| AC-40 | static | `rg -c -e libcCacheSubDir cmd/record/main.go` | 一致2件（定数定義と `filepath.Join` 1箇所のみ。現状は3件） |
| AC-41 | test | `cmd/verify/main_test.go::TestRunWarnsWhenDeprecatedFlagUsed`、`cmd/record/main_test.go::TestRunWarnsWhenDeprecatedFlagUsed`、`::TestRun_DebugInfoFlag_ControlsDebugFieldOmitEmpty` | 通過。本フェーズ群は不在ハッシュディレクトリの終了コードを `1` から `3` へ、ディレクトリでないパスの提示を意図的に変えている（02_architecture.md §4.3）。AC-41 が対象とするのは、存在するハッシュディレクトリを指定する通常の検証経路である |
| AC-42 | static | `rg -n -e 'saved-set-uid' cmd/runner/main.go` | 一致1件以上 |
| AC-42 | manual | 上記の一致が `main()` 内の `dropStartupPrivileges` 呼び出しに隣接していることを PR 差分で確認する | 隣接している |
| AC-43 | static | `rg -n -e ハッシュディレクトリを作成しません docs/user/verify_command.ja.md`（ステップ 4-11 が固定した文言） | 一致1件 |
| AC-43 | static | `rg -c -e hash_dir_not_found -e hash_dir_unreadable -e hash_dir_permission_violation -e permission_checker_init_failed docs/user/verify_command.ja.md docs/user/verify_command.md` | 両ファイルとも4件以上 |
| AC-43 | manual | 一時ディレクトリに不在ハッシュディレクトリを指定して `verify` を実行し、終了コードと標準エラー出力のトークンが文書の記述と一致することを確認する | 終了コード `3`、`hash_dir_not_found` を含む |
| AC-43 | manual | 書き換えた `robust-verification.sh` を、不在と権限違反の2条件で実行し、意図した分岐に入ることを確認する | 両条件とも意図どおり |
| AC-44 | static | `rg -n -e 'myhost_20260805140000_' docs/user/runner_command.ja.md docs/user/runner_command.md` | 一致0件（旧例が残っていない） |
| AC-44 | static | `rg -c -e 'myhost_20260805T140000Z_' docs/user/runner_command.ja.md docs/user/runner_command.md` | 両ファイルで1件 |
| AC-45 | static | `rg -n -e ハッシュディレクトリを作成しなくなりました CHANGELOG.ja.md` | 一致1件 |
| AC-45 | static | `rg -n -e 'ログファイル名のタイムスタンプが UTC になりました' CHANGELOG.ja.md` | 一致1件 |
| AC-45 | static | `rg -n -e パス解決の変更により新たに権限違反が検出されることがあります CHANGELOG.ja.md` | 一致1件 |
| AC-45 | static | `rg -n -e '新規作成するハッシュディレクトリのパーミッションが 0700 になりました' CHANGELOG.ja.md` | 一致1件 |
| AC-45 | static | `rg -n -e 'world-writable な場所への新規ハッシュディレクトリ作成を拒否します' CHANGELOG.ja.md` | 一致1件 |
| AC-45 | manual | CHANGELOG に記した `readlink -f` の判定手順を実行し、記載どおりの出力になることを確認する | 記載と一致 |
| AC-47 | static | `rg -n -e 'chgrp' docs/user/record_command.ja.md docs/user/record_command.md` | 両ファイルとも1件以上（分離運用の手順） |
| AC-47 | static | `rg -n -e '0o750' -A3 docs/user/record_command.ja.md \| rg -e 分離運用` | 一致1件（移行案内が例外に言及している） |
| AC-47 | manual | 分離運用の構成（`chgrp` + `0750`、グループ書き込み無し）を一時ディレクトリで作り、権限チェックが通ることを確認する | 違反として拒否されない |
| AC-46 | static | `rg -n -e 識別トークン docs/translation_glossary.md` | 一致1件（訳語 `identification token` の行） |
| AC-46 | manual | 残る候補語（読み取り専用バリデータ・除外理由）について、追加したもの、または追加不要と判断した理由を Document Status の Comments に記録する | 判断が記録されている |

---

## 8. 横断検索チェックリスト

`make lint` と `make test` では検出できない残存参照・整合性のみを挙げる（§7 の AC 検証表と重複する検索はここに再掲しない）。

- [ ] `rg -n -e ResolveAbsPathForCheck -e resolveStaticAbsPath --glob '*.md' docs/user CHANGELOG.ja.md CHANGELOG.md` — 利用者向け文書に旧関数名が残っていないこと（`docs/tasks/0149_*` の監査所見と残件一覧は当時の記録なので対象外）。
- [ ] `rg -n -e CreateValidator --glob '*.go' --glob '*.md' .` — 削除した関数への参照が残っていないこと。`CreateReadOnlyValidator` が部分一致するので、一致行を目視で切り分ける。
- [ ] `rg -n -e 0o750 internal/fileanalysis cmd docs/user` — 旧パーミッション値への言及が残っていないこと。ただし `docs/user/record_command.ja.md:531` と `docs/user/record_command.md:531` は 0146 由来の是正手順で、残すのが正しい。この2件のみが一致する状態を期待値とする。
- [ ] `rg -n -e 'CollectPermissionCheckDirs collects nothing' internal/runner` — ステップ 4-3 で書き換えるコメントが残っていないこと。
- [ ] `rg -n -e hash_dir_not_found -e hash_dir_unreadable -e hash_dir_permission_violation -e permission_checker_init_failed --glob '*.go' .` — トークン文字列が `cmd/verify/main.go` に定数として1箇所ずつ定義され、テストがリテラルを二重定義していないこと。
- [ ] `rg -n -e CheckSkipReason -e CheckSkipNone -e CheckSkipVariableReference -e CheckSkipRelative --glob '*.go' .` — `internal/security` 以外に同名の識別子が生まれていないこと。
- [ ] `docs/user/verify_command.ja.md` と `docs/user/verify_command.md`、`docs/user/runner_command.ja.md` と `docs/user/runner_command.md`、`docs/user/record_command.ja.md` と `docs/user/record_command.md`、`CHANGELOG.ja.md` と `CHANGELOG.md` の各対で、本タスクの変更が同じ節に反映されていること。

---

## 9. 成功基準

- [ ] §7 のすべての AC 検証が期待結果どおりである。
- [ ] `make test` と `make lint` が緑である。
- [ ] `make deadcode` が本タスクに由来する未使用シンボルを報告しない。
- [ ] 権限違反もパス解決失敗も起きない通常運用において、`runner`・`record` の外部から観測できる挙動が 0162 完了時点から変わらない。`verify` については、存在するハッシュディレクトリを指定する通常運用で挙動が変わらない。
- [ ] `verify` が fail-closed で終了した場合、原因が権限違反・ディレクトリ不在・読み取り不能・チェッカ初期化失敗のいずれであるかを、標準エラー出力の識別トークンのみから判別できる。
- [ ] 対象 11 件（L-1〜L-5・L-7・I-1〜I-5）それぞれについて、所見が指摘した挙動が現行コードに残っていないことを、§7 の `test` または `static` の項目で示せる。

---

## 10. 次のステップ

- [ ] 本書のレビューと `approved` への更新（レビュアー作業）。02_architecture.md §3.4 への `deps` の `newPermChecker`・`resolvePathForCheck` の追補（§1.4.4）は反映済み。
- [ ] 承認後、§3.2 の PR-1 から順に実装する。PR の区切りは各ステップ群の末尾にある「PR-N 作成ポイント」に従い、1つの PR をマージしてから次のブランチへ移る。
- [ ] `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` の消し込みはステップ 4-12（PR-8）で行う。
- [ ] issue [#986](https://github.com/isseis/go-safe-cmd-runner/issues/986) をクローズする。対象外とした L-6（[#1018](https://github.com/isseis/go-safe-cmd-runner/issues/1018)）と I-6（[#1019](https://github.com/isseis/go-safe-cmd-runner/issues/1019)）が別 issue として残ることを、クローズコメントに記す。
