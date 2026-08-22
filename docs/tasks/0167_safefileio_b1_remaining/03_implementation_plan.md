# 実装計画書: safefileio の残所見（資源リーク・失敗時契約・書き込みのアトミック化）

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-08-20 |
| Review date | 2026-08-21 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- 要件定義: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計: [02_architecture.md](02_architecture.md)
- 対象 Issue: [#978](https://github.com/isseis/go-safe-cmd-runner/issues/978)

---

## 1. 実装の概要

### 目的

[01_requirements.md](01_requirements.md) の F-001〜F-008（AC-01〜AC-38）を実装する。`internal/safefileio` が
エラーを返したときに fd・作成済みファイル・一時ファイルを残さない状態にし、原理的に取り消せない副作用は
公開 API の契約として明記する。併せて解析レコードの書き込みをアトミックにする。設計は
[02_architecture.md](02_architecture.md) が正であり、本書は設計を繰り返さず、作業と検証の手順のみを書く。

### 実装方針

- **設計文書を参照し、複製しない。** 各ステップは `02_architecture.md` の該当節を指す。判断の根拠を本書で
  述べ直さない。
- **段階の区切りで挙動を確かめる。** [02_architecture.md](02_architecture.md) § 8 の Phase 1〜5 を用いる
  （Phase 4 だけは、レビューの関心が分かれるため本書で 4a・4b の 2 節に分けた。§ 3.2）。各 Phase の
  終わりで `make fmt` → `make test` → `make lint` を通す。
- **テスト用の差し替え点を本番ビルドへ持ち込まない。** 新設する 3 つの差し替え点は、本番ビルドには差し替え可能な値を
  残さないよう、`//go:build !test` と `//go:build test` の 2 ファイルで同名の関数を排他的に定義する形に
  する（§ 1「本タスクで新規に必要になる差し替え点」）。`internal/security` の `getwd` が既に採っている形であり、
  新しい種類の注入機構は作らない。
- **モックではなく実ファイルシステムで検証する。** 書き込み・移動の経路はディレクトリ fd を起点にするため、
  `FileSystem` のモックからは届かなくなる（[02_architecture.md](02_architecture.md) § 7.1）。後始末と
  アトミック性の検証は `t.TempDir` 系の実ディレクトリ上で行う。
- **Go のソース（本番・テスト・テストヘルパのいずれも）に日本語を書かない。** コメント・識別子・
  文字列リテラルはすべて英語にする。
- **doc コメントは追記だけでなく、古くなった記述の書き換えも行う。** 経路が変わる関数の doc コメントは、
  仕組みの説明が実装と食い違ったまま残らないようにする（§ 2 Phase 5）。
- **テストは理由どおりに失敗できることを確かめる。** 後始末・検査順序・`EINTR` 再試行の 3 つについて、
  対象処理を外すとテストが落ちることを確認し、外した方法をコミットメッセージに記す（AC-05・07・28）。

### 既存コード調査結果

行番号は 2026-08-20 時点（`47f0ef77`）のものである。

#### `internal/safefileio/safe_file.go`（499 行・変更）

- `FileSystem` インターフェース（:52-61）に `Remove(name string) error`（:57-58）があり、実装は
  `osFS.Remove`（:95-98）の `os.Remove` 素通し。**本番の呼び出し元は 0 件**であることを再確認した
  （`rg` の結果はインターフェース定義・`osFS` の実装・テスト用モック 3 つのみ）。
- `File` インターフェース（:65-74）に `Sync` は無い。`Truncate` はある（:73）。
- `atomicMoveFileCore`（:140-201）は `srcFile.Chmod`（:162）が `canSafelyAccessFile`（:167）より前にあり、
  移動後に `fs.SafeOpenFile(absDst, …)`（:185）で宛先を開き直して `FileOpWrite` 検査（:196）を行っている。
- `safeWriteFileCommon`（:204-247）は `O_WRONLY|O_CREATE` で開き（:220）、検査後に `Truncate(0)`（:238）→
  `Write`（:242）。`defer` の `Close`（:225-230）は「他に失敗が無いときだけ」`Close` の失敗をエラーにする。
- `safeOpenFileFallback`（:475-499）は 2 回目の `ensureParentDirsNoSymlinks`（:494）の失敗時に
  `file` を Close せず返す。2 回目の呼び出しには差し替え点が無い。
- `ensureParentDirsNoSymlinks`（:257-307）は解決済みパスを内部で捨てている（:296 で `currentPath` に代入し、
  戻り値は `error` のみ）。
- `canSafelyReadFromFile`（:451-469）の doc コメント（:448-450）に UID を見ない理由の記載は無い。
- package コメント（:1-6）はフォールバックの存在に触れるが、保証の差には触れていない。
- **`File.Truncate` の本番の呼び出し元は `safe_file.go:238` の 1 か所だけ**である
  （`rg -n "\.Truncate\(" --glob '*.go' internal/ cmd/` から `_test.go` を除いた結果が 1 件）。この 1 行は
  Phase 4-3 で消えるため、`Truncate` は `Remove` と同じ「本番の呼び出し元を持たないインターフェース
  メソッド」になる。同じ基準で削除する（Phase 4-4。2026-08-21 決定）。

#### `internal/safefileio/safe_file_linux.go`（298 行・変更）

- `openat2`（:74-96）は `syscall.Syscall6` の直呼びで、`errno != 0` をそのまま返す。再試行は無い。
  システムコール発行の差し替え点が無いため新設が要る。
- `safeOpenFileInternal`（:268-298）は `mode: uint64(perm)`（:278）をそのまま渡す。errno の対応付けは
  :285-294（`ELOOP`→`ErrIsSymlink`、`EEXIST`→`ErrFileExists`、`ENOENT`→`os.ErrNotExist`）。
- `isOpenat2Available`（:44-71）は**それ自身が `openat2` を呼ぶ**（:64）。したがって `openat2` に差し替え点を
  入れると、`NewFileSystem` の中でその差し替え点が 1 回消費される。テストの手順はこれを踏まえる必要がある
  （Phase 1）。
- `moveFileAnchored`（:154-185）は `(srcFile File, absSrc, absDst string)`。内部で `os.Rename`（:172）・
  `verifySameFile`（:176）・`os.Remove(absSrc)`（:180）をパス名で行う。冒頭（:155-158）で `srcFile` を
  `*os.File` へ型アサートしている。
- `linkFileToTempName`（:226-251）は `(srcFile *os.File, dstDir string) (string, error)` で、ディレクトリを
  **パス名**で受け取り**フルパス**を返す。`moveFileAnchored` の後始末（:164-170）もそのフルパスに対する
  `os.Remove` である。Phase 3 のディレクトリ fd 化はこの関数も対象になる。
- `randomTempName`（:256-262）・`maxLinkatAttempts`（:102）・`tmpNameRandBytes`（:106）・
  `generateTempLinkName`（:112）・`linkatFunc`（:122）はいずれもこのファイルにある。`randomTempName` は
  接頭辞 `.safefileio-move-` を直書きしている（:261）。
- :120-121 のコメントが「このパッケージのテストは `t.Parallel()` を使わない」ことを明記している。

#### `internal/safefileio/safe_file_nonlinux.go`（32 行・変更）

`moveFileAnchored`（:29-31）は `os.Rename(absSrc, absDst)` の 1 行で、`srcFile` を使っていない。
`golang.org/x/sys/unix` を import していない。`unix.Openat`・`unix.Renameat`・`unix.Fstatat`・`unix.Unlinkat`
は darwin・netbsd・freebsd のいずれにも存在することを `GOOS=<os> go doc golang.org/x/sys/unix <名前>` で
確認した。`syscall.Stat_t` と `unix.Stat_t` の `Dev`・`Ino` のフィールド型は linux（ともに
`uint64`/`uint64`）・darwin（ともに `int32`/`uint64`）で一致しており、fd 側（`File.Stat()` 由来の
`*syscall.Stat_t`）と `fstatat` 側（`unix.Stat_t`）を型変換なしで比較できる。

#### `internal/safefileio/errors.go`（49 行・変更）

`ErrTempLinkNameExhausted`（:31-33）がある。`ErrUnsupportedFileMode`・`ErrDestinationCommitted` は無い。

#### `File`・`FileSystem` の実装型の全数

インターフェースの形を変えるため、手書きでメソッドを列挙している型をすべて洗い出した。

| 型 | 場所 | 必要な追従 |
|---|---|---|
| `osFS` | `internal/safefileio/safe_file.go:28` | `Remove` を削除 |
| `*os.File` | 標準ライブラリ | `Sync` を既に持つ。作業なし |
| `MockFileSystem` | `internal/safefileio/testutil/mock.go:19` | `Remove`・`RemoveFunc`・`RemoveCalls` を削除 |
| `mockFileSystem` | `internal/safefileio/safe_file_cleanup_test.go:190` | `Remove`・`removeFunc`・`removeCallCount`・`getRemoveCallCount` を削除 |
| `largeFakeFS` | `internal/security/machoanalyzer/analyzer_test.go:234` | `Remove`（:241）を削除 |
| `mockFile` | `internal/safefileio/safe_file_cleanup_test.go:25` | `Sync` を追加 |
| `largeFakeFile` | `internal/security/machoanalyzer/analyzer_test.go:222` | `Sync` を追加 |

`oversizeFileSystem`（`internal/filevalidator/validator_library_analysis_test.go:537`）・`countingFileSystem`
（`internal/filevalidator/validator_test.go:1793`）・`oversizeStatFile`（同 :552）・`seekErrorFile`／
`readErrorFile`（`internal/dynlib/machodylib/analyzer_test.go:642,650`）はいずれもインターフェースを埋め込む
形なので追従は要らない。`MockFileSystem` の利用者 5 ファイル（`internal/verification/manager_test.go`・
`internal/dynlib/machodylib/analyzer_test.go`・`internal/dynlib/elfdynlib/analyzer_test.go`・
`internal/runner/base/output/file_test.go`・`internal/runner/base/output/manager_test.go`）を確認したが、
`RemoveFunc`・`RemoveCalls` を参照している箇所は 1 件も無い。

#### 挙動が変わるため書き換える既存テスト

| テスト | 現在の主張 | 本タスク後の扱い |
|---|---|---|
| `safe_file_cleanup_test.go::TestSafeWriteFileOverwrite_NoCleanupOnError`（3 サブテスト、:240-350） | `mockFileSystem.Remove` の呼び出し回数が 0 | 検証対象が消滅する。削除し、4-5（PR-5）の実ファイルシステム上のテストへ置き換える |
| `safe_file_cleanup_test.go::TestFileCleanup_Integration`（:357-387） | 上書き失敗時に既存ファイルが消えない | 同じ実ディレクトリ上の検証を、宛先の内容保持と一時ファイル非残置の両方を見る新テストへ統合する |
| `safe_file_test.go::TestSafeWriteFileOverwrite_FileCloseError`（:354-374） | `Close` の失敗がエラーとして返る／`Write` の失敗が優先される | 差し替え後の `Close` の失敗は警告になる（§ 4.2）。加えて `failingCloseFS`・`failingWriteFS` は `FileSystem.SafeOpenFile` を差し替える形であり、書き込み経路が `openFileAt` に変わると届かなくなる。テストごと削除し、`linkatFunc` を使う差し替え前失敗のテストへ置き換える |
| `safe_file_linux_test.go::TestLinkFileToTempName_ExhaustsAttempts`（:247-270） | `require.Error(t, err)` のみで sentinel を名指ししていない | `ErrTempNameExhausted` への改名を検証できるよう `require.ErrorIs` へ強める。改名前は sentinel を主張するテストが 1 つも無い |

削除に伴って参照が無くなるテストヘルパも併せて消す。`safe_file_test.go` の :162-215 の一続きのブロック
（`failingFile`・`errSimulatedClose`・`failingCloseFS`・`failingWriteCloseFS`・`errSimulatedWrite`・
`failingWriteFS`）が丸ごと該当する。`failingFile` は `failingCloseFS` からのみ、`failingWriteCloseFS` は
`failingWriteFS` からのみ参照されており、両 FS 型を消すと未参照になる。`.golangci.yml` の `_test.go` 向け
除外リスト（gocyclo・errcheck・err113・dupl・gosec・goconst）に `unused` は含まれないため、消し残すと
`make lint` が落ちる。`safe_file_cleanup_test.go` からは `errDiskFull`・`errTruncateFailed`（:81-82）と、
`mockFile` の `writeErr`・`statErr`・`closeErr` フィールドを消す。`truncateErr` と `TestMockFileTruncate`
（:389-460）も、`File.Truncate` の削除（Phase 4-4）に伴って消える。

`safe_file_test.go` の `TestValidateFilePermissions`・`TestCanSafelyWriteToFile`・
`TestValidateFileOperationDifferences`・`TestResolvedPathModeEnforcement`・`TestEnsureParentDirsNoSymlinks` は
無変更で通る想定である（[02_architecture.md](02_architecture.md) § 3.9 末尾）。

#### 再利用できる既存の資産

- **AC-25（レコードの往復）**: `internal/fileanalysis/file_analysis_store_test.go` の
  `TestStore_SaveAndLoad`・`TestStore_PreservesExistingFields`・`TestStore_ArgEvalResultsRoundtrip`・
  `TestStore_Load_V9DynLibDepsObjectFormat` が保存・読み戻しと既存レコードの読み取りを既に押さえている。
  新しいテストは追加せず、これらが無変更で通ることを確認する。
- **AC-12（`internal/common` の `Remove`）**: `internal/runner/base/output/file_test.go::TestSafeFileManager_RemoveTemp`
  と `::TestSafeFileManager_RemoveTemp_WithMock` が `commonFS.Remove`（`file.go:137`）の唯一の本番の
  呼び出し元を押さえている。無変更で通ることを確認する。
- **AC-07b（`output` の移動）**: 同ファイルの `TestSafeFileManager_MoveToFinal` と
  `::TestSafeFileManager_MoveToFinal_WithMock` を無変更で通す。
- **AC-22**: `safe_file_test.go::TestResolvedPathModeEnforcement` を無変更で通す。
- **警告の検証**: `internal/testutil` の `NewRecordingLogger`・`LogRecorder.RequireRecord`・
  `RecordSnapshot.AssertHasAttrs`（`internal/testutil/handlers.go:158,187,214`）を使う。safefileio は
  パッケージ関数の `slog.Warn` を呼ぶため、テストは `slog.SetDefault` で記録用ロガーへ差し替え、
  `t.Cleanup` で元へ戻す。
- **umask を避ける権限設定**: `safe_file_test.go:325-330` が「作成後に `os.Chmod` で明示的に設定する」
  手順を確立している。新しい権限フィクスチャもこれに倣う。
- 差し替え点の書き方は `safe_file_linux_test.go::TestLinkFileToTempName_RetriesOnNameCollision`
  （:207-245、差し替えは :228-234）が手本になる（`t.Cleanup` で元の値へ戻す）。
- 一時ディレクトリは `internal/testutil` の `SafeTempDir(t)` を使う（既存テストと同じ）。

#### 本タスクで新規に必要になる差し替え点

設計（[02_architecture.md](02_architecture.md) § 3.2・§ 7.1）が求める検証を実ファイルシステム上で決定的に
行うには、既存の差し替え点だけでは足りない。次の 3 つを追加する。他の差し替え点は増やさない。

| 差し替え点 | 差し替える対象 | シグネチャ | 必要な理由 |
|---|---|---|---|
| `openat2Syscall` | `openat2` が発行する生のシステムコール | `func(dirfd int, pathname string, how *openHow) (int, error)` | `EINTR` を返す状況を実環境で再現できない（AC-26・28）。`*openHow` を受け取る位置で切ることで、カーネルへ渡る `mode` をテストから読める（AC-15） |
| `ensureParentDirsAfterOpen` | `safeOpenFileFallback` の 2 回目の親ディレクトリ確認 | `func(absPath string) error` | 1 回目と 2 回目のあいだに介入する手段が他に無く、2 回目だけを失敗させられない（AC-01〜05） |
| `verifyMovedFile` | `moveOpenFileCore` の `rename` 後の同一性確認 | `func(file File, dirFd int, name string) error` | `rename` 成功**後**の失敗を作る手段が書き込み経路に無い。移動元と移動先が同じディレクトリのため、既存のテストが使う「親ディレクトリの権限を落とす」手法では `rename` 自体が失敗してしまう（AC-18・21 の `ErrDestinationCommitted` 側） |

**3 つとも、本番ビルドには差し替え可能な値を一切残さない形で実装する（2026-08-21 承認）。**
`linkatFunc`・`generateTempLinkName` のような素のパッケージ変数にはしない。本番ビルドに可変の
パッケージ変数を置くと、セキュリティ上重要な経路に本番でも書き換えうる値が増えるためである。
実装は `internal/security` が `getwd` で既に採っている形をそのまま使う。

- **本番側**（`//go:build !test`）: 差し替えの余地がない普通の関数として定義する。
  例: `internal/security/getwd.go` の `getwd()`。
- **テスト側**（`//go:build test`）: 同名の関数を、パッケージ変数を経由して呼ぶ形で定義する。
  例: `internal/security/test_helpers_getwd.go` の `getwdOverride` と `getwd()`。

同名の識別子をビルドタグで排他的に定義するため、呼び出し側のコードはどちらのビルドでも変わらない。
配置するファイルは次のとおりとする。テスト側は
[test_organization.md](../../dev/developer_guide/test_organization.md) の Classification B
（パッケージ内部のヘルパは `test_helpers_<category>.go` に置き `//go:build test` を付ける）に従う。

| 差し替え点 | 本番側（`!test`） | テスト側（`test`） |
|---|---|---|
| `ensureParentDirsAfterOpen`・`verifyMovedFile` | `internal/safefileio/overrides.go`（`//go:build !test`） | `internal/safefileio/test_helpers_overrides.go`（`//go:build test`） |
| `openat2Syscall` | `internal/safefileio/overrides_linux.go`（`//go:build !test`） | `internal/safefileio/test_helpers_overrides_linux.go`（`//go:build test`） |

**4 つ目の差し替え点 `isAllowedOSManagedSymlink` を Phase 3 で追加した（実装時の決定）。** 上の表を書いた時点では
差し替え点は 3 つに限る予定だったが、3-4 が求める「`ensureDirNoSymlinks` が解決済みパスを返すことを Linux でも
検証する」は、`common.IsAllowedOSManagedSymlink` が Linux で常に false を返すため、allowlist 判定を差し替え
られなければ成立しない（前掲の 3 つはどれもこの分岐に届かない）。`internal/common` は変更せず、
safefileio 側に他の 3 つと同じ 2 ファイル方式（`overrides.go`／`test_helpers_overrides.go`）で置いた。
本番ビルドに可変のパッケージ変数を残さない形は変わらない。

**本番側（`//go:build !test`）のファイルは `make lint` の対象外である。** `make lint` は `golangci-lint run --build-tags test` を実行するため、`overrides.go`・`overrides_linux.go` は コンパイル対象にならない。コンパイル自体は `make build`／`go build`（タグ無し）が確認するため、各 Phase の完了条件に `go build ./internal/safefileio/` を入れてある。既存の `internal/security/getwd.go` も同じ状態であり、リンタ設定の見直しは本タスクの対象外とする。

`_linux.go` の接尾辞が GOOS を表すため、Linux 用の 2 ファイルのビルドタグに `linux` を書く必要はない。ただし同じ
パッケージの `safe_file_linux.go` が冗長に `//go:build linux` を書いており、実装ではそれに合わせて
`//go:build linux && !test`・`//go:build linux && test` と書いた（Phase 1 の step 1-2 の記述どおり）。

doc コメントは両側に置き、本番側には「本番ビルドには差し替えられる値を置かない」理由を、テスト側には
差し替え点の用途と、このパッケージのテストが `t.Parallel()` を使えない理由を、いずれも英語で書く
（`getwd.go` と `test_helpers_getwd.go` が同じ書き分けをしている）。既存の `linkatFunc`・
`generateTempLinkName` は本タスクの対象外であり、素のパッケージ変数のまま残す。

#### 文書側の調査結果

- `docs/user/security-risk-assessment.ja.md:158-172` が `safeOpenFileInternal` を引用しており、`mode:
  uint64(perm)` の行が Phase 1 で変わる。同 :204 が `safeOpenFileFallback` の二段階チェックを説明しており、
  Phase 2 の作成プローブで手順が増える。英語版 `docs/user/security-risk-assessment.md:161,164,210` に同じ
  箇所がある。
- `docs/dev/architecture_design/security-architecture.md` は**日本語版
  `security-architecture.ja.md` を持つ**（両方に同じ 2 つの引用がある。英語版 :205-213・:216-233、
  日本語版 :206-215・:217-233）。したがって日本語版を先に更新し、英語版は `/mktrans` で反映する。
  `openat2()` の引用は Phase 1 の `EINTR` 再試行で変わる。`ensureParentDirsNoSymlinks()` の引用も、走査の
  中身は変わらないが Phase 3 で本体が `ensureDirNoSymlinks` へ移るため、引用の見出し（どの関数の本体か）が
  合わなくなる。両方とも更新が要る。
- `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md:53-61` が B1 の残件一覧。B1 の
  記述は §2（🟠Low）だけにあり、§1・§3 には現れない（`rg -n "B1"` の結果が :53 と :60 の 2 件）。
  残件の箇条書きは `  - F-2:`・`  - F-3:`・`  - F-4:`・`  - F-5:`・`  - F-6〜F-9:` の 5 行（:55-59）で、
  F-2 と F-6〜F-9 は他と形が違うため、削除確認の検索式はこの 5 行すべてを捕まえる形にする必要がある。
  引用ブロックの書式は同文書 :15・:17・:51 の `> **… について**: …` に倣う。
- `docs/tasks/0149_security_code_smell_audit_fable/findings/B1_safefileio.md` の F-2〜F-9 は :28-81。
  対応結果の追記は、同ディレクトリの `A1_privilege.md:48,57,64,71` が採っている「所見の最後に箇条書きを
  1 つ足す」形に倣う。ただし箇条書きのラベルは B1 の地の文に合わせて `- 対応状況:`（太字なし）とする。
  `A1_privilege.md` は `- **該当箇所**:` と太字を使うが、`B1_safefileio.md` は `- 該当箇所:` と太字を
  使わないためである。F-1 には 0155 による追記が無いが、本タスクの対象外なので触れない。

#### ビルド検査の前提

`GOOS=darwin go vet ./internal/safefileio/` は**変更前の時点で既に失敗する**（exit 1）。テスト側が
`//go:build test` の `internal/testutil` を import しているためである。`-tags test` を付けると通る
（`GOOS=darwin go vet -tags test ./internal/safefileio/` が exit 0）。本書の完了条件はすべて `-tags test`
付きの形で書く。

---

## 2. 実装ステップ

### Phase 1: mode の検証・正規化と `openat2` の `EINTR` 再試行（F-004・F-006 / AC-14〜17・AC-26〜28）

対応する設計: [02_architecture.md](02_architecture.md) § 3.1・§ 3.2。

**変更するファイル**: `internal/safefileio/errors.go`・`safe_file.go`・`safe_file_linux.go`・
`safe_file_test.go`・`safe_file_linux_test.go`

#### 1-1. mode の検証と正規化

- [x] `errors.go` に `ErrUnsupportedFileMode` を追加する（宣言と doc コメントは
      [02_architecture.md](02_architecture.md) § 3.1 のコードブロックのとおり）。`ErrDestinationCommitted` は
      Phase 4a で追加するため、ここでは入れない。
- [x] `safe_file.go` に `validateOpenPerm(perm os.FileMode) error` を追加する。`perm &^ os.ModePerm` が
      0 でなければ `ErrUnsupportedFileMode` を返す。doc コメントに、`groupmembership.MaxAllowedReadPerms`
      が setuid・setgid を許すこととは対象が違う（あちらはディスク上のファイルの POSIX 権限、こちらは
      `open(2)` へ渡す `os.FileMode`）ことを 1 文で書く。
- [x] `osFS.SafeOpenFile`（`safe_file.go:86-93`）の `filepath.Abs` の直後、`safeOpenFileInternal` を呼ぶ前に
      `validateOpenPerm` の呼び出しを足す。Phase 4a・4b で書き込み・移動の経路が `SafeOpenFile` を通らなくなる
      ため、それらの入口にも同じ検査を足す（Phase 4-3 の該当ステップ）。
- [x] `safe_file_linux.go` に `openat2Mode(flag int, perm os.FileMode) uint64` を追加する。ファイルを作りうる
      呼び出し（`O_CREATE` または `O_TMPFILE`）でのみ `uint64(perm.Perm())` を返し、それ以外では 0 を返す。
      判定は補助関数 `mayCreateFile(flag int) bool` に置く。`O_TMPFILE` を含めるのは実装時の訂正であり、
      根拠は [02_architecture.md](02_architecture.md) § 3.1 に記録した（`O_TMPFILE` で mode 0 は `EINVAL`
      にならず、権限 `0000` のファイルが黙って作られる）。
- [x] `safeOpenFileInternal`（`safe_file_linux.go:275-280`）の `mode: uint64(perm)` を
      `mode: openat2Mode(flag, perm)` に置き換える。
#### 1-2. `openat2` の `EINTR` 再試行と差し替え点の新設

- [x] `safe_file_linux.go` の `openat2` を、`EINTR` のあいだ再試行するラッパにする。生のシステムコール発行を
      `rawOpenat2`（シグネチャは § 1 の差し替え点表のとおり `*openHow` を受け取る形）へ切り出し、
      `unsafe.Pointer` の取り回しはその中に閉じる。再試行に上限は設けない
      （[02_architecture.md](02_architecture.md) § 3.2）。`EINTR` 以外の errno は現在と同じ形でそのまま
      返し、`safeOpenFileInternal` の errno 対応付け（:285-294）は変更しない。
- [x] `openat2Syscall` を § 1 の 2 ファイル方式で用意する。`overrides_linux.go`（`//go:build linux && !test`）に
      `func openat2Syscall(dirfd int, pathname string, how *openHow) (int, error) { return rawOpenat2(…) }` を、
      `test_helpers_overrides_linux.go`（`//go:build linux && test`）に `var openat2SyscallOverride = rawOpenat2` と、
      それを呼ぶ同名の `openat2Syscall` を置く。本番ビルドに差し替え可能な値を残さない
      （`internal/security/getwd.go` と `test_helpers_getwd.go` と同じ形）。
#### 1-3. mode と `EINTR` のテスト

- [x] `safe_file_test.go` に `TestSafeOpenFile_RejectsNonPermissionModeBits` を追加する。`os.ModeSetuid`・
      `os.ModeSetgid`・`os.ModeSticky`・`os.ModeDir`・`os.ModeAppend` を含む `perm` について、
      `FileSystemConfig{}` と `FileSystemConfig{DisableOpenat2: true}` の両方で `ErrUnsupportedFileMode` が
      返ることを表で確認する。
- [x] `safe_file_test.go` に `TestSafeOpenFile_ReadOpenPermIgnoredOnBothPaths` を追加する。`O_CREATE` を
      伴わない `O_RDONLY` の open に非ゼロの `perm`（例: `0o644`）を渡し、両経路とも同じく成功することを
      確認する。本タスクの前は Linux 経路だけが `EINVAL` で失敗していた分岐である。
- [x] `safe_file_test.go` に `TestSafeOpenFile_CreatePermUnchanged` を追加する。テスト内で `syscall.Umask`
      を固定し（`t.Cleanup` で必ず元へ戻す。`Umask` はプロセス全体に効き、このパッケージは `t.Parallel()` を
      使わないため、戻し忘れると後続のテストが静かに壊れる）、`O_CREATE|O_WRONLY|O_EXCL` と `perm=0o640` で
      作ったファイルの権限が両経路とも `0o640 &^ umask` と一致することを確認する。両経路の一致だけを見る
      形にはしない（同じ壊れ方をすると通ってしまうため）。
- [x] **上の 3 つのテストで `FileSystemConfig{}` を使う行に、`fs.(*osFS).IsOpenat2Available()` が true で
      あることの `require` を入れる**（false なら理由を明記して `t.Skip`）。openat2 が使えない環境
      （Linux 5.5 以下、古い既定 seccomp プロファイルのコンテナ）では `NewFileSystem(FileSystemConfig{})` が
      静かにフォールバック経路になり、「両経路を通した」はずのテストが同じ経路を 2 回通るだけになる。
- [x] `safe_file_linux_test.go` に `TestOpenat2_RetriesOnEINTR` を追加する。`openat2Syscall` を差し替えて
      1 回目に `syscall.EINTR`、2 回目に本物へ委譲させ、`SafeOpenFile` が成功し呼び出し回数が 2 であることを
      確認する。**`FileSystem` は差し替えの前に構築する**（`isOpenat2Available` が `openat2` を呼ぶため、
      構築が差し替え点を 1 回消費する）。さらにスタブは対象のパス名に一致する呼び出しだけを数える。`t.Cleanup`
      で元へ戻す。
- [x] `safe_file_linux_test.go` に `TestOpenat2_NonEINTRErrnoMapping` を追加する。`openat2Syscall` を
      `ELOOP`・`EEXIST`・`ENOENT` を返すよう差し替え、`ErrIsSymlink`・`ErrFileExists`・`os.ErrNotExist` が
      `errors.Is` で判定できることを表で確認する。ここでも `FileSystem` を差し替えの前に構築する。構築後に
      差し替えないと、可用性判定が false になってフォールバック経路の errno 対応付けを検証してしまう。
- [x] `safe_file_linux_test.go` に `TestOpenat2_ReadOpenPassesZeroMode` を追加する。`openat2Syscall` を
      差し替えて `how.mode` を記録し、`O_CREATE` を伴わない open では 0、`O_CREATE` を伴う open では
      `uint64(perm.Perm())` であることを確認する（AC-15 のカーネル側の主張）。
- [x] 再試行のループを外すと `TestOpenat2_RetriesOnEINTR` が落ちることを確認し、外し方をコミット
      メッセージに記す。

**完了条件**: `make fmt` → `make test` → `make lint` が通る。
`GOOS=darwin go vet -tags test ./internal/safefileio/` が通る。この Phase は `overrides_linux.go` を追加するため、
差し替え点が両ビルドで成立していることも確かめる。`go build ./internal/safefileio/`（タグ無し＝本番側の
`//go:build !test` を通す）と `make test`（`-tags test`＝テスト側の `//go:build test` を通す）の両方が
通り、かつ `rg -n "^var " internal/safefileio/overrides.go internal/safefileio/overrides_linux.go` が
0 件である（本番側にパッケージ変数が無い）ことを確認する。

### PR-1 作成ポイント: open-mode validation and openat2 EINTR retry

**対象ステップ**: 1-1 / 1-2 / 1-3

**推奨タイトル**: `feat(0167): validate open mode and retry openat2 on EINTR`

**レビュー観点**: `validateOpenPerm` と既存の `ValidateRequestedPermissions` の役割分担が doc コメントで区別できているか / `openat2Mode` の「`O_CREATE` が無ければ mode は 0」という規則が両経路で一致しているか / `openat2Syscall` の 2 ファイル方式が本番ビルドに可変のパッケージ変数を残していないか / openat2 可用性の `require` があり、フォールバック経路を 2 回通すだけのテストになっていないか / `EINTR` 再試行に上限を設けないという判断（§ 2 Phase 1）が、セキュリティ上重要な open 経路で受け入れられるか

**実装モデル要件**: frontier-required

**判定理由**: 1-1 が `open` というセキュリティゲート上で拒否の追加と削除を同時に行う（`validateOpenPerm` が `os.ModePerm` 外のビットを新たに拒否する一方、`openat2Mode` により `O_CREATE` を伴わない非ゼロ `perm` の open が Linux 経路で `EINVAL` にならなくなる）。これは `mkplan.md` step 8 のパネルモード・トリガ「simultaneous behavior raises and lowers」そのものである。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#1050](https://github.com/isseis/go-safe-cmd-runner/pull/1050)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: 共通ヘルパの整理とフォールバック経路の後始末（F-001 / AC-01〜05）

対応する設計: [02_architecture.md](02_architecture.md) § 3.3・§ 6.2。

**変更するファイル**: `internal/safefileio/errors.go`・`safe_file.go`・`safe_file_linux.go`・
`safe_file_cleanup_test.go`・`safe_file_linux_test.go`

#### 2-1. 共通ヘルパの移動と改名

- [x] `verifySameFile` を `safe_file_linux.go:200-220` から `safe_file.go` へ移す。第 1 引数の型を
      `*os.File` から `File` インターフェースへ広げ、fd 側の `syscall.Stat_t` は `getFileStatInfo` と同じく
      `Stat().Sys()` から取り出す。
- [x] `verifySameFile` の doc コメント（現 :186-199）を書き換える。現在の文面は「パス名による確認とパス名に
      よる unlink のあいだの隙」「`rename` と unlink のあいだに `absSrc` を差し替えられる」という
      `moveFileAnchored` 専用の説明になっている。Phase 3 で Linux 経路が `fstatat`／`unlinkat` に変わり、
      さらにフォールバック経路の後始末という第 2 の呼び出し元が加わるため、そのまま持っていくと実装と
      食い違う。新しい文面は、（a）確認と操作が別のシステムコールである以上どちらの呼び出し元でも隙は
      狭まるだけで閉じないこと、（b）ディレクトリ fd 相対の呼び出し元では差し替えの対象が名前だけに
      限られること、の 2 点を英語で書く（[02_architecture.md](02_architecture.md) § 5.3 の R3）。
- [x] `randomTempName` を `safe_file_linux.go:256-262` から `safe_file.go` へ移し、接頭辞を引数に取る
      `randomTempName(prefix string) (string, error)` にする。`tmpNameRandBytes` も併せて移す。
- [x] `generateTempLinkName`（`safe_file_linux.go:112`）を、接頭辞を受け取る形の差し替え点として維持する
      （`var generateTempLinkName = randomTempName`）。`linkFileToTempName`（:230）の呼び出しを
      `generateTempLinkName(".safefileio-move-")` に直す。
- [x] `maxLinkatAttempts`（`safe_file_linux.go:102`）を `maxTempNameAttempts` へ改名して `safe_file.go` へ
      移す。doc コメントを、ハードリンク名と一時ファイル名の双方に使う旨へ書き換える。
- [x] `errors.go` の `ErrTempLinkNameExhausted` を `ErrTempNameExhausted` へ改名する。エラー文字列を
      `"failed to allocate a unique temporary link name"` から
      `"failed to allocate a unique temporary name"` へ変える。doc コメントも「ハードリンク名・一時ファイル名の
      いずれか」を指す表現へ書き換える。
- [x] `safe_file_linux_test.go::TestLinkFileToTempName_ExhaustsAttempts`（:247-270）の
      `require.Error(t, err)` を `require.ErrorIs(t, err, ErrTempNameExhausted)` へ強める。現状この sentinel を
      名指しするテストは 1 つも無く、改名が無検証のまま入ってしまうため。CLAUDE.md の
      「`errors.Is` で判定し、文字列一致に頼らない」にも沿う。
#### 2-2. フォールバック経路の作成プローブと後始末

- [x] `safe_file.go` に `removeVerifiedFileByPath(file File, path string) error` を追加する。
      `verifySameFile` で同一性を確認し、一致した場合だけ `os.Remove` する。一致しない、
      または確認自体が失敗した場合は削除せず、`slog.Warn` に対象パスと理由を記録する
      （[02_architecture.md](02_architecture.md) § 5.4）。`Close` の失敗も同じ警告に含める。
      **`Close` は同一性の一致・不一致にかかわらずこの関数が行う**（実装時の確定。
      [02_architecture.md](02_architecture.md) § 6.2 の判断フローが D5・D6 の両分岐で Close を求めており、
      呼び出し元に任せると不一致の分岐で fd が漏れるため）。順序は `verifySameFile` → `Close` →
      `os.Remove` で、§ 3.3 が求める「fd を握ったままの削除をしない」形を保つ。
- [x] 2 回目の親ディレクトリ確認のための差し替え点 `ensureParentDirsAfterOpen` を § 1 の 2 ファイル方式で
      用意する。`overrides.go`（`//go:build !test`）に `ensureParentDirsNoSymlinks` を直接呼ぶだけの関数を、
      `test_helpers_overrides.go`（`//go:build test`）に `var ensureParentDirsAfterOpenOverride =
      ensureParentDirsNoSymlinks` と、それを呼ぶ同名の関数を置く。`safeOpenFileFallback:494` の呼び出しを
      `ensureParentDirsAfterOpen` に差し替える。
- [x] `safeOpenFileFallback`（`safe_file.go:475-499`）に作成プローブを実装する。分岐は
      [02_architecture.md](02_architecture.md) § 6.2 の判断フロー図が正であり、そのとおりに実装する。
      再試行の上限には `maxTempNameAttempts` を使う。
- [x] 内部由来の `EEXIST` を `ErrFileExists` へ変換しないようにする。`ErrFileExists` を返すのは、呼び出し元
      自身が `O_EXCL` を指定していた場合だけである。
- [x] **開き直しの `ENOENT` が上限に達したときの戻り値を、非 nil であることが構造上保証される形にする
      （レビュー指摘による実装時の追加）。** 最後の `ENOENT` を `fmt.Errorf` で包んで返し、併せて
      `slog.Warn` に対象パスと試行回数を記録する。上限到達そのものを踏むテストは置かない。踏むには
      「プローブと開き直しのあいだで対象を消す」差し替え点が要り、§ 1 が「差し替え点は 3 つに限る」と
      定めているためである。代わりに、(a) 包むことで「nil エラー＋nil ファイル」を返す形自体を作れなく
      し、(b) `ENOENT` **以外**の開き直し失敗が再試行されずそのまま返ることを
      `TestSafeOpenFileFallback_CreationProbe/reopen_failure_other_than_enoent_is_not_retried` で
      押さえる。
- [x] 2 回目の親ディレクトリ確認が失敗した場合の後始末を実装する。作成していない場合は `Close` して元の
      エラーを返す。作成していた場合は `removeVerifiedFileByPath` を呼ぶ。いずれの場合も 2 回目の確認の失敗
      そのものを `slog.Warn` に記録する（[02_architecture.md](02_architecture.md) § 5.4）。**呼び出し元へ
      返るのは常に 2 回目の確認が返したエラーであり、後始末の失敗（同一性の不一致を含む）を返さない**
      （§ 4.2）。
#### 2-3. 後始末のテスト

- [x] `safe_file_linux_test.go` に `TestSafeOpenFileFallback_ClosesFDWhenPostCheckFails` を追加する
      （fd の観察に `/proc/self/fd` を使うため Linux 専用ファイルに置く。対象の
      `safeOpenFileFallback` 自体は `DisableOpenat2: true` で Linux からも通る）。
      `ensureParentDirsAfterOpen` を差し替えて 2 回目だけ失敗させ、エラーが返ること・戻り値の `File` が
      nil であることを確認したうえで、**`/proc/self/fd/*` のリンク先のうちテストの一時ディレクトリ配下を
      指すものの集合**が呼び出しの前後で変わらないことを確認する。エントリ数の差分では、Go ランタイムが
      開閉する fd や `os.ReadDir` 自身の fd が混ざって不安定になるため使わない。負の対照（後始末を外すと
      落ちること）を確認するときは `GOGC=off` で実行し、`os.File` のファイナライザが漏れた fd を閉じて
      しまってテストが誤って通ることを防ぐ。この点をコミットメッセージにも記す。
- [x] `safe_file_cleanup_test.go` に `TestSafeOpenFileFallback_RemovesCreatedFileWhenPostCheckFails` を
      追加する。サブテストを 3 つ置く。
      - `created`: `O_CREATE` で新規作成した場合にファイルが残らないこと。
      - `pre_existing`: 既存ファイルを開いただけの場合は削除されず内容が保たれること。
      - `identity_mismatch`: 作成後・2 回目の確認の失敗の前に対象を別ファイルへ差し替え、差し替えた
        ファイルが削除されないこと、返るエラーが 2 回目の確認のエラーであり
        `ErrSourceIdentityMismatch` **ではない**こと、および `slog.Warn` に対象パスを含む記録が
        残ることを確認する（AC-03 の「削除せず、警告し、元のエラーを返す」の 3 点すべて）。
- [x] `safe_file_cleanup_test.go` に `TestRemoveVerifiedFileByPath_SkipsRemovalOnInodeMismatch` を追加する。
      ヘルパ単体の検証として、実在するファイルのパスと `mockFile`（`Stat()` が返す `syscall.Stat_t` の
      `Dev`・`Ino` が 0 で実在のパスとは決して一致しない）を渡し、`ErrSourceIdentityMismatch` が返ること、
      対象ファイルが残っていることを確認する。
- [x] `safe_file_linux_test.go` の `generateTempLinkName` 差し替え（:228-234・:262-264）を、接頭辞つきの
      新しいシグネチャへ追従させる。
- [x] 後始末（`Close` と `removeVerifiedFileByPath` の呼び出し）を外すと上記のテストが落ちることを
      確認し、外し方をコミットメッセージに記す（AC-05）。
- [x] **`safe_file_test.go` に `TestSafeOpenFileFallback_CreationProbe` を追加する（実装時の追加）。**
      2-2 の作成プローブは open そのものを 2 回のシステムコールへ分けるため、後始末の 3 テストとは別に、
      呼び出し元から見た open の結果が変わらないことを押さえる必要がある。サブテストは 4 つとする。
      - `creates_when_absent`: 対象が無い場合に作成して開けること。
      - `opens_existing_without_reporting_it_exists`: 既存ファイルを `O_EXCL` なしの `O_CREATE` で開くと、
        内部の `EEXIST` が `ErrFileExists` として外へ出ず、開き直しが既存の内容に当たること。
      - `reports_exists_when_caller_asked_for_o_excl`: 呼び出し元自身が `O_EXCL` を指定した場合は
        従来どおり `ErrFileExists` が返ること。
      - `rejects_leaf_symlink`: リーフがシンボリックリンクの場合に `ErrIsSymlink` が返り、リンク先の
        内容が変わらないこと（開き直しが `O_NOFOLLOW` を保つことの検証。開き直しから `O_NOFOLLOW` を
        外すとこのサブテストが落ちることを確認済み）。

**完了条件**: `make fmt` → `make test` → `make lint` が通る。
`GOOS=darwin go vet -tags test ./internal/safefileio/` が通る。この Phase は `overrides.go` を追加するため、
差し替え点が両ビルドで成立していることも確かめる。`go build ./internal/safefileio/`（タグ無し＝本番側の
`//go:build !test` を通す）と `make test`（`-tags test`＝テスト側の `//go:build test` を通す）の両方が
通り、かつ `rg -n "^var " internal/safefileio/overrides.go internal/safefileio/overrides_linux.go` が
0 件である（本番側にパッケージ変数が無い）ことを確認する。

### PR-2 作成ポイント: fallback-path creation probe and failure cleanup

**対象ステップ**: 2-1 / 2-2 / 2-3

**推奨タイトル**: `feat(0167): clean up fd and file when the fallback post-check fails`

**レビュー観点**: 2 回目の親ディレクトリ確認が失敗した 4 分岐（Close のみ／作成済みファイルの削除／既存ファイルの保持／同一性不一致で削除せず警告）が漏れなく実装され、元のエラーが握り潰されていないか / 内部由来の `EEXIST` が `ErrFileExists` として外へ出ていないか / `removeVerifiedFileByPath` が同一性の確認に失敗したときに削除しないか / fd リークの検証が `GOGC=off` 前提で書かれているか

**実装モデル要件**: frontier-recommended

**判定理由**: 2-2 が失敗時の復旧処理（fd の Close・作成済みファイルの削除・同一性不一致時の保持）そのものであり、`mkplan2.md` step 4 の「isolated high-risk/complex step（recovery flows）」に該当する。作成プローブが `internal/runner/bootstrap/logger.go` へ `ENOENT` を新たに返しうる点（§ 5 の R-1〜R-7 のうち R-5）は「拒否の追加と削除の同時実施」に見えるが、この変化はフォールバック経路にのみ生じ、本番ターゲット（Linux 5.6+）では `openat2` 経路が使われてプローブが動かないため、`frontier-required` のトリガとしては数えない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#1051](https://github.com/isseis/go-safe-cmd-runner/pull/1051)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: ディレクトリ fd プリミティブと `moveFileAnchored` の書き換え（F-002・F-005 の前提）

対応する設計: [02_architecture.md](02_architecture.md) § 3.4.1・§ 3.4.5。

この Phase は **Linux の外部挙動を変えない**。ただし実装時のレビューで、変わってしまう箇所が 2 つ見つかった
（いずれも修正済み・記録済み）。

1. ディレクトリ fd を `O_RDONLY` で開くと、パス名で操作していたときには要らなかった読み取り権限を親
   ディレクトリに要求してしまう（投函用ディレクトリ `0o733` などが拒否される）。Linux では `O_PATH` で
   開くことで元の権限要件に戻し、`TestAtomicMoveFile_MovesIntoUnreadableDirectory` で両経路を押さえた
   （[02_architecture.md](02_architecture.md) § 3.4.1 に追記）。非 Linux には `O_PATH` が無いため読み取り
   権限を要する制限が残る。
2. 宛先の親ディレクトリの確認が fchmod より前に移るため、宛先の親が不正な場合にソースの権限が変わらなく
   なる。これは AC-06 が Phase 4a で意図している変化が 1 段階早く現れたもので、方向は同じ（副作用が減る）
   である。

非 Linux では、`moveFileAnchored` に移動直前の同一性確認が
加わるため `ErrSourceIdentityMismatch` が新たに返りうる（現在は `os.Rename` 一発で確認が無い）。これは
[02_architecture.md](02_architecture.md) § 5.3 の R4 が意図した変化で、隙が狭まる方向である。
`01_requirements.md` Success Criteria が挙げる挙動の変化には含まれていなかったため、2026-08-21 に
7 点目として同文書へ追記した。挙動の組み替えは Phase 4a・4b で行う。

**変更するファイル**: `internal/safefileio/safe_file.go`・`safe_file_linux.go`・`safe_file_nonlinux.go`・
`safe_file_linux_test.go`

#### 3-1. ディレクトリ fd プリミティブの実装

- [x] `ensureParentDirsNoSymlinks`（`safe_file.go:257-307`）から
      `ensureDirNoSymlinks(dir string) (string, error)` を切り出す。走査は現行のまま（allowlist に載る
      OS 管理シンボリックリンクは `EvalSymlinks` で解決して続行）で、**解決済みのディレクトリパスを返す**点だけが
      異なる。`ensureParentDirsNoSymlinks` は `ensureDirNoSymlinks(filepath.Dir(absPath))` を呼んで解決済み
      パスを捨てるラッパとして残す。allowlist 判定の呼び出しは、3-4 のテストのために差し替え点
      `isAllowedOSManagedSymlink` 経由へ変える（§ 1 の 4 つ目の差し替え点）。
- [x] `safe_file_linux.go` に `openDirNoSymlinks` の openat2 版を実装する。
      `openat2(AtFdcwd, dir, &openHow{flags: O_DIRECTORY|O_RDONLY, resolve: ResolveNoSymlinks})` の 1 回の
      呼び出しで開く。`openat2` が使えない場合はフォールバック版へ委譲する。**openat2 の可用性は `osFS` の
      フィールドにしか無いため、`openDirNoSymlinks`・`openFileAt` は自由関数ではなく `*osFS` のメソッドと
      した**（フォールバック版は自由関数 `openDirNoSymlinksFallback`・`openFileAtFallback`）。併せて
      `atomicMoveFileCore` の第 4 引数を `FileSystem` から `*osFS` へ変える。本番・テストとも呼び出し元は
      `(*osFS).AtomicMoveFile` の 1 か所だけであり、モックは渡らない。
- [x] `safe_file.go`（両経路共通）にフォールバック版の `openDirNoSymlinks` を実装する。`ensureDirNoSymlinks`
      が返した**解決済みパス**を `O_DIRECTORY|O_NOFOLLOW` で開く。元のパスを開くと、開こうとしている
      ディレクトリ自身が allowlist の OS 管理シンボリックリンクである場合に失敗する
      （[02_architecture.md](02_architecture.md) § 3.4.1。errno は Linux では `ENOTDIR`、`O_DIRECTORY` を
      伴わない場合の `ELOOP` ではない。3-4 のテストはこの差を踏まえて errno を主張しない）。
- [x] `safe_file_linux.go` と `safe_file_nonlinux.go` に `openFileAt` を実装する。Linux（openat2 が使える
      場合）は `openat2(dirfd, name, …)`、それ以外は `unix.Openat(dirfd, name, flag|O_NOFOLLOW, mode)`。
      **戻り値の型は `*os.File` とする**（`os.NewFile` で包む）。`linkFileToTempName` が `/proc/self/fd/<n>`
      のために `Fd()` を必要とし、`moveFileAnchored` も現在 `*os.File` への型アサート（`safe_file_linux.go:155`）に
      依存しているため、インターフェース値のままでは書き込み経路の一時ファイル fd をハードリンクに使えない。
      両者とも `ELOOP`（フォールバックでは `isNoFollowError` の判定）を `ErrIsSymlink` に、`EEXIST` を
      `ErrFileExists` に、`ENOENT` を `os.ErrNotExist` に対応付ける。mode は Phase 1 の `openat2Mode` と
      同じ規則（`O_CREATE` が無ければ 0）で決める。**実装時の確定が 3 点ある。**
      （a）errno の対応付けは新設の `mapOpenErrno` 1 か所に集約し、`safeOpenFileInternal` の既存の
      switch もこれに置き換えた。netbsd の `EFTYPE` を扱うため判定には `isNoFollowErrno`（`isNoFollowError`
      から切り出した errno 版）を使う。副作用として openat2 経路でも `EMLINK` が `ErrIsSymlink` になるが、
      Linux のフォールバック経路は元からそう扱っており、経路間の一致は強まる方向である。
      （b）mode の規則は `openPermBits` として `safe_file.go` へ移し、`openat2Mode` はその薄いラッパにした。
      `mayCreateFile` は `O_TMPFILE` が Linux にしか無いためプラットフォーム別の定義に分けた。
      （c）自分で開く fd には `O_CLOEXEC` を付ける。フォールバック経路が使っていた `os.OpenFile` は
      常に `O_CLOEXEC` を付けるため、`unix.Openat` へ移ることで子プロセスへの fd 漏れが生じないようにする。
      ディレクトリ fd も同じ扱いにする。ディレクトリ fd のアクセスモードは Linux では `O_PATH`、
      非 Linux では `O_RDONLY` とする（本節冒頭の 1 番。定数 `dirAccessFlag` をプラットフォーム別に置く）。
      （d）`perm` の検査（`validateOpenPerm`）は `openFileAt` の両経路の入口でも行う。この 2 つは
      `SafeOpenFile` を通らない呼び出し元（`atomicMoveFileCore`、Phase 4b の `safeWriteFileCommon`）から
      直接呼ばれるため、`openPermBits` が `perm.Perm()` で黙って落とす形になってはならない。
      （e）フォールバック版のディレクトリ open は、`O_DIRECTORY` によりシンボリックリンクが Linux では
      `ENOTDIR` になる。`mapDirOpenErrno` が対象を `Lstat` し直して `ErrIsSymlink` へ寄せ、openat2 経路と
      同じ sentinel を返す。
- [x] `verifySameFile` を、fd とパス名で比較する形と、fd とディレクトリ fd＋名前で比較する形の両方から
      使えるようにする（比較そのものは 1 か所に置く）。実装は `verifySameFile`（`lstat`）と
      `verifySameFileAt`（`fstatat` + `AT_SYMLINK_NOFOLLOW`）の 2 つが、`fdStatOf` と `compareInode` を
      共有する形とした。パス側の型は両者を揃えるため `syscall.Stat_t` から `unix.Stat_t` へ変えた。

#### 3-2. `moveFileAnchored`・`atomicMoveFileCore` のディレクトリ fd 対応

- [x] `linkFileToTempName`（`safe_file_linux.go:226-251`）を、ディレクトリのパス名ではなく
      **宛先ディレクトリ fd** を受け取り、フルパスではなく**作った名前**を返す形へ変える。`linkat` の
      呼び出しを `linkatFunc(unix.AT_FDCWD, procPath, dstDirFd, name, unix.AT_SYMLINK_FOLLOW)` にする。
      名前が単一の構成要素であることの確認は、この関数が持っていた個別の判定をやめ、新設の
      `validateOpenAtName` に寄せる（`openFileAt`・`verifySameFileAt`・`moveFileAnchored` と同じ判定）。
- [x] `moveFileAnchored` の後始末（`safe_file_linux.go:164-170`）の `os.Remove(tmpPath)` を
      `unix.Unlinkat(dstDirFd, tmpName, 0)` に変える。上の変更で一時ハードリンクの参照がフルパスから
      「ディレクトリ fd と名前」に変わるため、後始末も同じ指し方に揃える。
- [x] `moveFileAnchored` のシグネチャを
      `moveFileAnchored(srcFile File, srcDirFd int, srcName string, dstDirFd int, dstName string) error`
      へ変える。Linux 版は `rename` を宛先ディレクトリ fd 相対（`unix.Renameat`）に、移動元の削除を
      `unix.Unlinkat(srcDirFd, srcName, 0)` にする。`verifySameFile` の呼び出しは
      `fstatat(srcDirFd, srcName, AT_SYMLINK_NOFOLLOW)` を使う形にする。
- [x] **`rename` の errno を `mapRenameErrno` で 1 か所補正する（実装時の追加）。** 宛先が既存の
      ディレクトリである場合、カーネルは `EISDIR` を返すが、`os.Rename` は宛先を `lstat` してから
      `EEXIST` に差し替えていた（Go の `os.rename`）。`internal/runner/base/output` の
      `TestSafeFileManager_MoveToFinal` が `fs.ErrExist` を主張しており、素の `unix.Renameat` に
      置き換えるとこの契約が黙って変わる。`EISDIR` の場合だけ `EEXIST` と元の errno の両方を包んで返し、
      呼び出し元から見た挙動を変えない。Linux・非 Linux の両方に適用する。
- [x] `moveFileAnchored` の doc コメント（現 :124-153）を書き換える。現在の文面は `absSrc`／`absDst` という
      消えるパラメータを十数か所で参照し、不変条件を「`absSrc` をパス名で解決し直さない」と述べている。
      新しい文面では、（a）0155 が確立した「宛先へ現れるのは必ず `srcFile` が指す inode であり、それを
      示せないときは何も動かさず失敗する」という不変条件、（b）`may_linkat` によって差し替えられたソースが
      宛先へ到達できない理由、（c）`rename` 成功後の失敗は巻き戻さないこと、の 3 点を保ちつつ、
      ディレクトリ fd と名前を受け取る新しい形に合わせて書き直す（英語）。
- [x] 非 Linux 版（`safe_file_nonlinux.go:29-31`）を、`fstatat(srcDirFd, srcName, AT_SYMLINK_NOFOLLOW)` に
      よる同一性確認のうえで `unix.Renameat(srcDirFd, srcName, dstDirFd, dstName)` を実行する形にする。
      doc コメントを、ディレクトリは固定されるが inode への固定はできない旨
      （[02_architecture.md](02_architecture.md) § 5.3 の R4）に更新する。
- [x] `atomicMoveFileCore`（`safe_file.go:140-201`）を、移動元・移動先のディレクトリ fd を
      `openDirNoSymlinks` で取得し、ソースを `openFileAt` で開き、新シグネチャの `moveFileAnchored` を呼ぶ
      形に組み替える。この Phase では検査の順序（chmod と検証）と移動後の検証はまだ変えない。取得した
      ディレクトリ fd は取得の直後に `defer` で閉じる登録を行い、以降の分岐がどこで失敗しても漏れないように
      する。
- [x] `atomicMoveFileCore` の doc コメント（現 :137-139）から、`SafeOpenFile`／`ensureParentDirsNoSymlinks`
      で検査すると書いている部分を、`openDirNoSymlinks`／`openFileAt` による検査へ書き換える。
#### 3-3. 既存テストの新シグネチャへの追従

- [x] `safe_file_linux_test.go` の `moveFileAnchored` を直接呼ぶ 5 つのテスト
      （`TestMoveFileAnchored_RegressionSuccessfulMove`・`_SourceReplacementFailsClosed`・
      `_RenameFailureCleansUpTemporaryLink`・`_UnlinkSourceFailureReturnsErrorAfterSuccessfulRename`・
      `TestAtomicMoveFileCore_EndToEndUsesFDAnchoredMove`）を新しいシグネチャへ追従させる。検証内容は
      変えない。
- [x] `safe_file_linux_test.go` の `linkFileToTempName` を直接呼ぶ 3 つのテスト
      （`TestLinkFileToTempName_RetriesOnNameCollision`・`_ExhaustsAttempts`・
      `_NonEEXISTErrorIsNotRetried`）を、ディレクトリ fd を渡し名前を受け取る新しいシグネチャへ
      追従させる。検証内容は変えない（`_ExhaustsAttempts` の主張の強化は Phase 2 で済んでいる）。

#### 3-4. ディレクトリ fd プリミティブのテスト

- [x] `safe_file_test.go` に `TestEnsureDirNoSymlinks_ReturnsResolvedPath` を追加する。`ensureDirNoSymlinks`
      が **allowlist に載る OS 管理シンボリックリンクを解決した後のパスを返す**ことを、戻り値そのものに
      対して確認する。`common.IsAllowedOSManagedSymlink` は Linux では常に false を返すため
      （`internal/common/osmanaged_symlink_other.go:8-10`）、`/tmp` を使う形の検証は Linux では必ず skip に
      なり、§ 3.4.1 が新たに課した「解決済みパスを開く」という義務が本番環境で一度も検証されない。
      allowlist に依存しない形（allowlist 判定を差し替え可能にし、`t.TempDir` 配下に作った
      シンボリックリンクを許可する）で Linux からも踏めるようにする。**`internal/common` の変更は要らな
      かった。** safefileio 側に差し替え点 `isAllowedOSManagedSymlink` を § 1 の 2 ファイル方式で置き、
      テストヘルパ `allowOSManagedSymlink(t, path)` から使う。テストは 2 つのサブテストを持ち、1 つ目
      （allowlist を差し替えない）でシンボリックリンクが拒否されることを先に押さえる。これが無いと、
      2 つ目の主張が差し替えの結果なのか元から通るのかを言えない。
- [x] **`openDirNoSymlinks` のフォールバック版が解決済みパスを開いていることを直接押さえる
      `TestOpenDirNoSymlinksFallback_OpensResolvedPath` を追加する（実装時の追加）。** 上のテストは
      `ensureDirNoSymlinks` の戻り値までしか見ないため、それを使う側が元のパスを開いていても落ちない。
      同じ差し替えを使い、（a）解決前のパスを `O_NOFOLLOW` で開くと実際に失敗すること（前提の確認）、
      （b）`openDirNoSymlinksFallback` が成功すること、（c）得られた fd の inode がリンク先の
      ディレクトリと一致することの 3 点を確認する。
- [x] 上のテストに加えて、`common.IsAllowedOSManagedSymlink("/tmp")` が true の環境でだけ実行する
      テストを置く（false なら `t.Skip`）。判定に
      `runtime.GOOS` を使わない。`/tmp` は `t.TempDir` の外なので、ファイル名は
      `.safefileio-test-<ランダム>` の形で実行ごとに一意にし、作成の直後に `t.Cleanup` で削除を登録する。
      本 Phase では書き込み経路がまだ `openDirNoSymlinks` を通らないため、名前は
      `TestAtomicMoveFile_WritesUnderOSManagedSymlink` とし、`/tmp` 直下への `AtomicMoveFile` で踏む。
- [x] 上の 2 つのテストが理由どおりに落ちることを確認した（`ensureDirNoSymlinks` の戻り値を解決前の
      `dir` に戻すと 1 つ目が、`openDirNoSymlinksFallback` が開く対象を解決前のパスに戻すと 2 つ目が
      落ちる）。なお `/tmp` を使うテストは macOS 専用であり、Linux の CI では常に skip になる。
      § 3.4.1 の義務を実際に押さえているのは `TestOpenDirNoSymlinksFallback_OpensResolvedPath` である。
- [x] **プリミティブ単体のテストを追加した（レビュー指摘による実装時の追加）。**
      - `TestValidateOpenAtName`: 単一構成要素の判定（`""`・`.`・`..`・`/`・`//`・絶対パス・`a/b`・
        `sub/` を拒否）。`filepath.Base("/")` が `"/"` を返すため、`Base` との比較だけでは `/` を
        取りこぼす。絶対パスはディレクトリ fd を無視させるため、この漏れは塞ぐ必要がある。
      - `TestVerifySameFileAt`: 一致・別 inode への差し替え・**同じ inode を指すシンボリックリンクへの
        差し替え**・不正な名前・対象消滅の 5 分岐。3 つ目は `AT_SYMLINK_NOFOLLOW` を外すと落ちることを
        確認した。この関数は非 Linux の `moveFileAnchored` が `ErrSourceIdentityMismatch` を返す唯一の
        根拠だが、Linux の `moveFileAnchored` では `may_linkat` により先に失敗するため、この直接の
        テストが無いと不一致の分岐がどちらのプラットフォームでも一度も踏まれない。
      - `TestAtomicMoveFileCore_EndToEndUsesFDAnchoredMove` を `openRoutes` の両経路で実行するように
        した。従来は openat2 経路しか通らず、`openFileAtFallback`・`openDirNoSymlinksFallback` が
        Linux の CI で一度も実行されていなかった。検証内容は変えていない。
      - 非 Linux 版の `moveFileAnchored` 自体は Linux の CI では実行できない（リスク管理表 R-1）。
        `GOOS=darwin`／`GOOS=netbsd` の `go vet -tags test` でビルドを確認するにとどまる。

**完了条件**: `make fmt` → `make test` → `make lint` が通り、既存テストが検証内容を変えずに通る。
`GOOS=darwin go vet -tags test ./internal/safefileio/` と
`GOOS=netbsd go vet -tags test ./internal/safefileio/` が通る（非 Linux 版のビルドを両方の系統で確かめる）。
3-4 の 2 つのテストが、`ensureDirNoSymlinks` が解決済みパスを返すという § 3.4.1 の義務を Linux でも
踏んでいる（allowlist 判定の差し替えが `internal/common` の変更を要するかどうかは、この PR の中で決着
させる。PR-5 へ持ち越さない）。

### PR-3 作成ポイント: directory-fd primitives and fd-anchored move

**対象ステップ**: 3-1 / 3-2 / 3-3 / 3-4

**推奨タイトル**: `refactor(0167): anchor the move path on directory file descriptors`

**レビュー観点**: `ensureDirNoSymlinks` が返す解決済みパスをフォールバック版の `openDirNoSymlinks` が確かに開いているか（元のパスを開くと allowlist 上の OS 管理シンボリックリンクで `ELOOP` になる） / 取得したディレクトリ fd が全分岐で漏れずに閉じられるか / 非 Linux 版に `ErrSourceIdentityMismatch` が新たに生じる変化が `01_requirements.md` Success Criteria の 7 点目と一致しているか / Linux の外部挙動が変わっていないこと（既存テスト 8 件が検証内容を変えずに通ること） / 3-4 の 2 つのテストが、フォールバック版が解決済みパスを開くという義務を Linux 上でも踏めているか（`common.IsAllowedOSManagedSymlink` が Linux で常に false のため、素朴に書くと必ず skip になる）

**実装モデル要件**: frontier-recommended

**判定理由**: 3-1・3-2 が Linux／非 Linux 双方のプリミティブを同時に書き換える複雑なステップで、リスク管理表 R-1（非 Linux のビルド破壊に CI で気づけない）が本計画で最も影響の大きいリスクとして挙がっているため、「isolated high-risk/complex step」に該当する。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#1052](https://github.com/isseis/go-safe-cmd-runner/pull/1052)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4a: 移動経路の分割と検査順序の変更（F-002・F-005 / AC-06・07・07a・07b）

[02_architecture.md](02_architecture.md) § 8 の Phase 4 を、レビューの関心ごとに 4a（移動経路）と
4b（書き込み経路と `Remove` の削除）の 2 つに分けて記す。段階の内容・順序・先行条件は同節の Phase 4 の
ままであり、4a → 4b の順に続けて実施する。

対応する設計: [02_architecture.md](02_architecture.md) § 3.4.2〜§ 3.4.4・§ 6.1。

**変更するファイル**: `internal/safefileio/errors.go`・`safe_file.go`・`safe_file_linux.go`・
`safe_file_test.go`・`safe_file_linux_test.go`

#### 4-1. 移動経路の分割と順序の変更

- [x] `errors.go` に `ErrDestinationCommitted` を追加する（宣言と doc コメントは
      [02_architecture.md](02_architecture.md) § 3.1 のコードブロックのとおり）。
- [x] `atomicMoveFileCore` から `moveOpenFileCore` を切り出す。責務の分担は
      [02_architecture.md](02_architecture.md) § 3.4.1 の表のとおりとする。
- [x] `moveOpenFileCore` の中で、fchmod を `canSafelyAccessFile(FileOpRead)` によるソース検証の**後**に置く
      （AC-06）。ソース検証は `atomicMoveFileCore` 側に残るため、`moveOpenFileCore` は「最初の副作用から
      後」だけを担う形になった。
- [x] fchmod の直後・`rename` の前に、同じ fd に対して `canSafelyAccessFile(FileOpWrite)` を実行する
      （[02_architecture.md](02_architecture.md) § 3.4.3）。移動後の宛先を開き直す検査（現行
      `safe_file.go:185-198`）はここへ移す。
- [x] 移動後に残す検証を、宛先ディレクトリ fd と宛先名に対する `verifySameFile` による同一性確認へ変える。
      呼び出しは差し替え点 `verifyMovedFile` を経由させ、宛先をパス名で開き直さない。差し替え点は § 1 の 2 ファイル
      方式で用意する。`overrides.go`（`//go:build !test`）に `verifySameFile` を fstatat 経由で呼ぶだけの関数を、
      `test_helpers_overrides.go`（`//go:build test`）に `var verifyMovedFileOverride = …` と、それを呼ぶ同名の
      関数を置く。
- [x] `rename` に到達した後のすべての失敗を `ErrDestinationCommitted` で包む。包む場所は `moveOpenFileCore`
      とする。**`rename` に到達したかどうかは `moveFileAnchored` が内部 sentinel `errRenameCommitted` で
      申告する（実装時の決定。[02_architecture.md](02_architecture.md) § 3.4.4 に追記した）。**
      `moveFileAnchored` は `rename` の後に移動元の削除を行うため、戻り値のエラーだけでは前後を区別できず、
      それを「未到達」とみなすと § 5.2 が `ErrDestinationCommitted` の対象として挙げる「移動元の削除失敗」が
      漏れる。宛先の同一性を問い合わせて判定する案はレビューで棄却した（`rename` 直後に宛先を奪われると
      「何も起きなかった」と報告してしまう。§ 3.4.4）。公開 sentinel を付ける位置は `moveOpenFileCore` の
      ままであり、`moveFileAnchored` の戻り値の形は変わらないため、同関数を直接呼ぶ既存テストの前提も
      変わらない。
- [x] `slog.Warn` に、宛先が置き換わったうえで失敗した事象を記録する。宛先のパスと失敗した検証の内容を
      含める（[02_architecture.md](02_architecture.md) § 5.4 の最重要項目）。
- [x] 権限検査による拒否の記録に、対象の `mode`・`uid`・`gid` と、判定を下した規則（world-writable・
      グループ非所属・上限超過のいずれか）を含める（同 § 5.4）。規則の名前は `groupmembership` の
      sentinel から決める（`rejectionRule`）。記録は `canSafelyAccessFile` の 1 か所に置き、
      `canSafelyReadFromFile` は同関数の読み取り検査を呼ぶ形へ寄せたため（重複していた読み取り方針の
      実装が 1 つになる）、`SafeReadFile` の拒否も同じ記録を残す。**`rule` は `groupmembership` の
      sentinel から決めるが、共有グループによる書き込みの拒否だけは `CanUserSafelyWriteFile` が
      エラー無しの false を返すため、書き込み検査でエラーが無い場合を
      `group-writable-not-sole-member` と名付けている（レビュー指摘による追加。両方針のうち
      エラー無しで false を返す分岐はこれだけである）。** 検査対象がパスの示すファイルなのか、これから
      そのパスへ移す inode なのかは `subject` 属性で区別する（`accessSubject`）。

#### 4-2. 移動経路のテストの追加

- [x] `safe_file_test.go` に `TestAtomicMoveFile_ValidatesSourceBeforeChmod` を追加する。ソース検証を
      失敗させ、`AtomicMoveFile` がエラーを返すこと、およびソースの権限が呼び出し前のままであることを
      確認する（AC-06・07）。**検証を失敗させる手段を `0o1644` から `0o666`（world-writable）へ変えた
      （実装時の訂正）。** 本書の当初の記述は「sticky が `MaxAllowedReadPerms=0o6775` を超える」という
      前提だったが、実ファイルではこの分岐に到達しない（根拠は
      [02_architecture.md](02_architecture.md) § 3.4.2 に追記した）。`0o666` は `requiredPerm=0o600` に
      狭めれば通ってしまうため、順序の入れ替えを元に戻すとこのテストが落ちるという性質は変わらない。
- [x] `safe_file_test.go` に `TestAtomicMoveFile_RejectsUnsafeSourcePermissions` を追加する（AC-07a）。
      権限は**いずれも作成後に `os.Chmod` で明示的に設定する**（`os.WriteFile` の perm は umask に削られ、
      `0o666` が `0o644` になって拒否条件が成立しなくなる。既存の `TestValidateFilePermissions`
      〈`safe_file_test.go:325-330`〉と同じ手順）。**サブテストは 2 つとした（実装時の訂正）。**
      - `world_writable`: `0o666` のソース。`requiredPerm=0o600` でも拒否されること。
      - `group_writable_non_member`: `os.Getgroups()` に含まれない GID へ `os.Chown(path, -1, gid)` した
        `0o660` のソース。`chown` が `EPERM` で失敗する環境では理由を明記して `t.Skip` する。
      - `perms_exceed_maximum` は**置かない**。実ファイルの mode ではこの規則に到達できないためである
        （[02_architecture.md](02_architecture.md) § 3.4.2）。`world_writable` は権限を要さず常に実行
        されるため、拒否経路そのものは無条件に踏まれる。
- [x] `safe_file_test.go` に `TestAtomicMoveFile_SafeSourceStillMoves` を追加する。`0600` のソースを
      `requiredPerm=0o644` で移動し、成功すること・宛先の権限が `0o644` になること・内容が保たれることを
      確認する（AC-07b）。実行者が属するグループから書き込み可能なソース（`0o660`、グループは
      `os.Getgid()`）が従来どおり受け入れられるサブテストも併せて置く。
- [x] `safe_file_test.go` に `TestMoveOpenFileCore_RejectsRequiredPermBeforeRename` を追加する
      （[02_architecture.md](02_architecture.md) § 3.4.3）。`moveOpenFileCore` は可搬なので Linux 専用
      ファイルには置かない。エラーが `ErrInvalidFilePermissions` を含み `ErrDestinationCommitted` を
      含まないこと、宛先が変化していないこと、および `requiredPerm` が入口の
      `ValidateRequestedPermissions` を通ること（＝拒否したのが後段の検査であること）を確認する。
      サブテストは 2 つとした。
      - `required_perm_not_writable_by_owner`: `requiredPerm=0o444`。`MaxAllowedWritePerms=0o664` 以下
        なので入口は通るが、`CanUserSafelyWriteFile` は所有者書き込みビットの無い mode を拒否する。
        **権限の準備を要さないため常に実行される（実装時の追加）。** これが無いと、この段の拒否を踏む
        テストが構成に依存して 1 つも走らない環境が生じる。
      - `required_perm_group_writable_in_shared_group`: `requiredPerm=0o660` とし、ソースの GID を、
        実行者が属していて**かつ他にも構成員がいる**グループへ `os.Chown(path, -1, gid)` で設定する
        （`internal/groupmembership/manager.go:243-251`）。該当するグループが無い場合は理由を明記して
        `t.Skip` する。
- [x] **`ErrDestinationCommitted` を返す 2 つの分岐のテストを追加する（実装時の追加）。** 4-1 で新設した
      契約であり、書き込み経路のテスト（Phase 4b）まで無検証で置くと、包む境界が誤っていても気づけない。
      - `safe_file_test.go::TestMoveOpenFileCore_PostMoveIdentityFailureIsDestinationCommitted`:
        `verifyMovedFileOverride` を差し替えて移動後の同一性確認を失敗させ、`ErrDestinationCommitted` が
        返ること・宛先が移動後の内容であること・宛先のパスを含む警告が記録されることを確認する。
      - `safe_file_linux_test.go::TestMoveOpenFileCore_MoveFailureAfterRenameIsDestinationCommitted`:
        移動元の親ディレクトリから書き込み権限を落として `rename` 後の unlink を失敗させ、
        `moveFileAnchored` 由来の失敗も同じく包まれることを確認する（Linux 専用。移動元の削除が
        `rename` と別の段になるのが Linux の経路だけであるため）。
- [x] **拒否の記録のテストを追加する（レビュー指摘による追加）。**
      `safe_file_test.go::TestCanSafelyAccessFile_RecordsRejection` が実ファイルに対する拒否で
      `path`・`subject`・`operation`・`mode`・`uid`・`gid`・`rule` が残ることを確認し、
      `::TestRejectionRule` が、実ファイルでは作れない規則（グループ非所属・上限超過・非所有者）と
      エラー無しの false の扱いを表で確認する。`TestMoveOpenFileCore_RejectsRequiredPermBeforeRename` は
      `subject` が `pending-destination` であることを併せて確認する。
- [x] `internal/runner/base/output` の既存テスト（`TestSafeFileManager_MoveToFinal`・
      `TestSafeFileManager_MoveToFinal_WithMock`）が無変更で通ることを確認する（AC-07b）。
- [x] 検査順序の入れ替えを元に戻すと `TestAtomicMoveFile_ValidatesSourceBeforeChmod` が落ちることを
      確認し、戻し方をコミットメッセージに記す（AC-07）。

**Phase 4a の完了条件**: `make fmt` → `make test` → `make lint` が通る。
`GOOS=darwin go vet -tags test ./...` と `GOOS=netbsd go vet -tags test ./internal/safefileio/` が通る。
4-1 で `overrides.go` に `verifyMovedFile` を足すため、差し替え点が本番ビルドとテストビルドの両方で
成立していることも確かめる。`go build ./internal/safefileio/` と `make test` の両方が通り、かつ
`rg -n "^var " internal/safefileio/overrides.go internal/safefileio/overrides_linux.go` が 0 件である
ことを確認する。書き込み経路はこの時点では未変更のため、`make deadcode` の比較と性能の実測は 4-6 で行う。
この段階で既存テストが落ちないのは、§ 1「挙動が変わるため書き換える既存テスト」の 3 件がいずれも書き込み
経路のテストであり `safeWriteFileCommon` は 4-3 まで手を付けないこと、および `moveFileAnchored` を直接
呼ぶ既存テストが `ErrDestinationCommitted` を包む位置（`moveOpenFileCore`）の外側にあることによる。
包む位置を `moveFileAnchored` へ下げるとこの前提が崩れるため、4-1 では下げない。

### PR-4 作成ポイント: move-path split and check reordering

**対象ステップ**: 4-1 / 4-2

**推奨タイトル**: `feat(0167): split the move path and reorder its safety checks`

**レビュー観点**: fchmod がソース検証の後に移り、失敗時にソースの権限が変わらないか / `rename` 前の `FileOpWrite` 検査が § 3.4.3 のとおり前倒しされているか / `ErrDestinationCommitted` を包む境界が `rename` 到達後に限られ、到達前の失敗に混ざっていないか / 拒否の記録に `mode`・`uid`・`gid` と適用した規則が含まれるか

**実装モデル要件**: frontier-required

**判定理由**: 4-1 はセキュリティゲートの並び替えを行うステップ（`mkplan.md` step 8 のパネルモード・トリガ「security-gate step」）であり、同時に不可逆な `rename` を境に前後で契約が変わる分岐を新設する。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#1053](https://github.com/isseis/go-safe-cmd-runner/pull/1053)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4b: 書き込みのアトミック化と `Remove` の削除（F-002・F-003 / AC-10〜13・AC-18〜25）

対応する設計: [02_architecture.md](02_architecture.md) § 3.5・§ 3.6・§ 6.1。

**変更するファイル**: `internal/safefileio/safe_file.go`・`safe_file_linux.go`・`safe_file_nonlinux.go`・
`testutil/mock.go`・`safe_file_test.go`・`safe_file_cleanup_test.go`・`safe_file_linux_test.go`・
`internal/security/machoanalyzer/analyzer_test.go`

#### 4-3. 書き込みのアトミック化

- [ ] `File` インターフェース（`safe_file.go:65-74`）に `Sync() error` を追加する。実装型
      （`mockFile`・`largeFakeFile`）の追従は 4-4 で行うため、4-3 だけではビルドが通らない。PR-5 の中で
      4-3 → 4-4 を続けて行う（`File.Truncate` の削除だけが 4-3 の書き換え完了を待つ、という向きの制約で
      ある）。
- [ ] `safe_file.go` に `createTempFileInDir` を追加する。`randomTempName(".safefileio-write-")` で名前を
      作り、`openFileAt(dirFd, name, O_WRONLY|O_CREATE|O_EXCL, 0o600)` で作る。`ErrFileExists` の場合は
      `maxTempNameAttempts` まで名前を変えて再試行し、超えたら `ErrTempNameExhausted` を返す。権限は
      `perm` ではなく固定の `0o600` とする。
- [ ] `safe_file.go` に `removeVerifiedFileAt(file File, dirFd int, name string) error` を追加する。同一性の
      確認は `fstatat`、削除は `unlinkat` で、どちらもディレクトリ fd 相対に行う。比較そのものは
      `verifySameFile` に委ねる。
- [ ] `safeWriteFileCommon`（`safe_file.go:204-247`）を、[02_architecture.md](02_architecture.md) § 3.6.2 の
      7 手順と § 6.1 の判断フローのとおりに書き換える。§ 2.3 の概略図と § 6.1 が食い違う場合は § 6.1 に従う。
      `moveOpenFileCore` へ渡す `requiredPerm` は呼び出し元の `perm` である（§ 2.3 のシーケンス図。
      AC-19 の「宛先の権限が `perm` と一致する」はこの受け渡しで成立する）。
- [ ] `safeWriteFileCommon` と `atomicMoveFileCore` の入口に `validateOpenPerm`（Phase 1）の呼び出しを足す。
      両者は書き換え後 `SafeOpenFile` を通らなくなるため、Phase 1 で入れた特殊ビットの拒否がこの 2 経路から
      抜け落ちる。既存の `ValidateRequestedPermissions` は `perm & 0o7777` で判定するため
      `os.ModeSetuid`（`1<<23`）を捕まえられず、代わりにならない
      （[02_architecture.md](02_architecture.md) § 3.1）。
- [ ] 宛先プローブで `ErrIsSymlink` を受け取った場合、一時ファイルを作る前に拒否する（AC-20）。
- [ ] 一時ファイルの段階で失敗した場合に呼び出し元へ返るエラーを、一時ファイル名ではなく宛先のパスを
      名指しする形で包む（[02_architecture.md](02_architecture.md) § 3.6.2 末尾）。
- [ ] 差し替え後の `Close` の失敗は `slog.Warn` に記録し、エラーとしては返さない（§ 4.2）。
- [ ] `ErrDestinationCommitted` を含む失敗では一時ファイルの削除を試みず、一時ファイルのパスを含む警告を
      記録する（[02_architecture.md](02_architecture.md) § 5.4）。

#### 4-4. `FileSystem.Remove` の削除とインターフェース追従

- [ ] `safe_file.go` の `FileSystem` インターフェースから `Remove`（:57-58）を削除する。
- [ ] `safe_file.go` の `osFS.Remove`（:95-98）を削除する。
- [ ] `testutil/mock.go` の `MockFileSystem.Remove`（:54-61）・`RemoveFunc`（:24-25）・`RemoveCalls`（:35）を
      削除する。
- [ ] `safe_file_cleanup_test.go` の `mockFileSystem.Remove`（:212-220）・`removeFunc`（:191）・
      `removeCallCount`（:195）・`getRemoveCallCount`（:233-237）を削除する。
- [ ] `internal/security/machoanalyzer/analyzer_test.go` の `largeFakeFS.Remove`（:241）を削除し、
      `largeFakeFile` に `Sync`（`func (largeFakeFile) Sync() error { return nil }`）を追加する。
- [ ] `safe_file_cleanup_test.go` の `mockFile` に `Sync` を追加する。
- [ ] **`File.Truncate` を削除する（2026-08-21 決定）。** Phase 4-3 で `safe_file.go:238` の
      `file.Truncate(0)` が消えると、`Truncate` は本番の呼び出し元を 1 つも持たなくなる。これは
      `01_requirements.md` の F-5 が `Remove` の削除を正当化した基準（0157・0166 と同じ）に当てはまる。
      Phase 4-3 の書き換えが済んだ**後**に、次の 5 か所をまとめて消す。`make deadcode` はインターフェースの
      メソッドを報告しないため、この削除は自動検査では代替できない。
      - `safe_file.go:73` の `Truncate(size int64) error`（`File` インターフェース）
      - `safe_file_cleanup_test.go:141-165` の `mockFile.Truncate`
      - `safe_file_cleanup_test.go` の `mockFile.truncateErr` フィールド
      - `safe_file_cleanup_test.go:389-460` の `TestMockFileTruncate`
      - `internal/security/machoanalyzer/analyzer_test.go:231` の `largeFakeFile.Truncate`
- [ ] `TestMockFileTruncate` の削除で本番コードのカバレッジが下がらないことを確認する。同テストは
      `mockFile` 自身の `Truncate` の挙動（負のサイズの拒否、伸長時のゼロ埋め）だけを検証しており、
      `internal/safefileio` の本番関数を 1 つも通らない。`go tool cover -func` の比較（この節の後段）で
      関数単位の差が無いことを確かめ、その旨をコミットメッセージに記す。
- [ ] `make deadcode` を実行し、本タスクの削除に起因する新たな未使用シンボルが出ないことを確認する
      （AC-13）。実行前の出力を控えておき、差分で判断する。
- [ ] `go tool cover -func` を `Remove` 関連テストの整理の前後で取得し、関数単位で失われるカバレッジが
      無いことを確認する。確認した旨をコミットメッセージに記す（AC-11）。

#### 4-5. 書き込み経路のテストの入れ替えと追加

- [ ] `safe_file_cleanup_test.go::TestSafeWriteFileOverwrite_NoCleanupOnError` を削除する（3 サブテストとも
      検証対象の `Remove` が消滅するため）。
- [ ] `safe_file_cleanup_test.go::TestFileCleanup_Integration` を削除する（次に足す新テストへ統合する）。
- [ ] `safe_file_test.go::TestSafeWriteFileOverwrite_FileCloseError` を削除する（差し替え後の `Close` は
      エラーを返さなくなり、`FileSystem` 差し替えによる注入も届かなくなるため）。
- [ ] 参照が無くなるヘルパを削除する: `safe_file_test.go` の :162-215 の一続きのブロック
      （`failingFile`・`errSimulatedClose`・`failingCloseFS`・`failingWriteCloseFS`・`errSimulatedWrite`・
      `failingWriteFS`）と、`safe_file_cleanup_test.go` の `errDiskFull`・`errTruncateFailed`（:81-82）、
      `mockFile` の `writeErr`・`statErr`・`closeErr` フィールド。
- [ ] `safe_file_test.go` に `TestSafeWriteFileOverwrite_SucceedsWithPermApplied` を追加する。書き込みが
      成功すること、内容が一致すること、宛先の権限が `perm` と一致すること（新規作成・既存の上書きの
      両方）を確認する（AC-19）。`syscall.Umask` をテスト内で固定し、`t.Cleanup` で必ず元へ戻す。
- [ ] `safe_file_test.go` に `TestSafeWriteFileOverwrite_RejectsSymlinkDestination` を追加する（AC-20・24）。
      宛先がシンボリックリンクのとき、`ErrIsSymlink` が返ること、**リンク先のファイルの内容が変わって
      いないこと**、および**宛先のパスが依然としてシンボリックリンクであること**（`os.Lstat` の
      `Mode()&os.ModeSymlink != 0` と `os.Readlink` の戻り値）の 3 つを確認する。
- [ ] `safe_file_test.go` に `TestSafeWriteFileOverwrite_ExistingDestinationRejectedLeavesItIntact` を
      追加する（AC-18・21・23 の可搬な経路）。宛先を用意したうえで `os.Chmod` で `0o666` を明示的に設定し
      （`FileOpWrite` の検査に落ちる権限。umask に削られると検査を通ってしまう）、書き込みが失敗すること、
      返るエラーが `ErrDestinationCommitted` を**含まない**こと、宛先の内容が元のままであること、宛先
      ディレクトリに `.safefileio-` で始まるエントリが 1 つも残っていないことを確認する。
- [ ] `safe_file_linux_test.go` に `TestSafeWriteFileOverwrite_PreCommitFailureLeavesDestinationIntact` を
      追加する（AC-18・21・23 の、一時ファイルへ書き終えた後に失敗する経路）。既存の `linkatFunc` を
      差し替えて `rename` の手前で失敗させ、返るエラーが `ErrDestinationCommitted` を**含まない**こと、
      宛先の内容が元のままであること、宛先ディレクトリに `.safefileio-` で始まるエントリが残らないことを
      確認する。`linkatFunc` は Linux 専用の差し替え点なので、この経路の検証は Linux に限られる。可搬な経路の
      検証は上の `ExistingDestinationRejected…` が担う。
- [ ] `safe_file_test.go` に `TestSafeWriteFileOverwrite_PostCommitFailureIsDestinationCommitted` を
      追加する（AC-18・21 の差し替え**後**の側。現在この分岐を主張するテストが 1 つも無い）。
      差し替え点 `verifyMovedFile` を差し替えて `rename` 成功後に失敗させ、次の 4 点を確認する。
      - 返るエラーが `errors.Is(err, ErrDestinationCommitted)` を満たす。
      - 宛先の内容が**新しい内容**になっている。
      - 一時ファイルの削除を試みていない（一時ファイル名の inode が消されていない）。
      - `slog.Warn` に宛先のパスと一時ファイルのパスを含む記録が残る
        （[02_architecture.md](02_architecture.md) § 5.4）。
- [ ] `internal/fileanalysis` の既存テスト（`TestStore_SaveAndLoad`・`TestStore_PreservesExistingFields`・
      `TestStore_ArgEvalResultsRoundtrip`・`TestStore_Load_V9DynLibDepsObjectFormat`）が無変更で通ることを
      確認する（AC-25）。
- [ ] `internal/runner/base/output` の既存テスト（`TestSafeFileManager_RemoveTemp`・
      `TestSafeFileManager_RemoveTemp_WithMock`）が無変更で通ることを確認する（AC-12）。

#### 4-6. 性能の実測

[02_architecture.md](02_architecture.md) § 3.6.4 と `01_requirements.md` の F-7 が、`record` 実行の wall time を
変更の前後で実測して絶対値を本書に記録することを求めている。

- [ ] 変更前後の 2 つのバイナリを用意する。作業ツリーを退避せずに済むよう、変更前は
      `git worktree add ../0167-base $BASE` で別ツリーを作り、そこで `make build` する
      （`build/prod/record` が生成物）。計測後に `git worktree remove` する。基点は § 7 冒頭で固定した
      `$BASE` であり、計測時点の `origin/main` ではない。`origin/main` は PR-1〜PR-4 のマージで既に
      本タスクの変更を含んでおり、`01_requirements.md` の F-7 と § 9 が求める「本タスクの前後」に
      ならない。
- [ ] 計測用のハッシュディレクトリを `dir=$(mktemp -d)` で作り、`chmod 700 "$dir"` する。固定パスは
      同時に走る別のセッションと衝突するため使わない。
- [ ] 対象ファイルの一覧をファイルに固定し（`/usr/bin` 配下の先頭 200 件など）、両リビジョンで同じ一覧を
      使う。
- [ ] 各リビジョンについて、まず計測しない**ウォームアップ実行**を 1 回行う（対象 200 件をページ
      キャッシュへ載せ、初回だけ遅くなる分を計測から外す）。その後
      `time <record> -force -d "$dir" $(cat <一覧>)` を 3 回実行し、中央値を採る。各回の前に
      `rm -rf "$dir"/*` でレコードを消して条件を揃える。
- [ ] 次の表を実測値で埋める。相対比ではなく絶対値（秒）と 1 件あたりの増分（ミリ秒）を記す。

基点（`$BASE`）のコミットハッシュを表の下に記録する。

| 対象件数 | 変更前（`$BASE`、秒） | 変更後（秒） | 差（秒） | 1 件あたりの増分（ms） |
|---|---|---|---|---|
| 200 | 実装時に記入 | 実装時に記入 | 実装時に記入 | 実装時に記入 |

**Phase 4b の完了条件**: `make fmt` → `make test` → `make lint` が通る。`make deadcode` に新規項目が
出ない。`GOOS=darwin go vet -tags test ./...` と `GOOS=netbsd go vet -tags test ./internal/safefileio/` が
通る。`go build ./internal/safefileio/` と `make test` の両方が通り、かつ
`rg -n "^var " internal/safefileio/overrides.go internal/safefileio/overrides_linux.go` が 0 件である
ことを確認する。上の性能表が埋まっている。

### PR-5 作成ポイント: atomic write and removal of the unused FileSystem.Remove / File.Truncate

**対象ステップ**: 4-3 / 4-4 / 4-5 / 4-6

**推奨タイトル**: `feat(0167): make SafeWriteFileOverwrite atomic and drop dead File API`

**レビュー観点**: `safeWriteFileCommon` が § 3.6.2 の 7 手順と § 6.1 の判断フローに従っているか（食い違う場合は § 6.1 が正） / 差し替え前の失敗で宛先の内容が保たれ `.safefileio-` の一時ファイルが残らないか / `Remove` と `Truncate` の削除が本番の呼び出し元 0 件という同じ基準で行われ、`go tool cover -func` と `make deadcode` の前後比較で裏づけられているか / `fsync` の追加分が絶対値（秒・1 件あたり ms）で記録され、相対比で判断されていないか

**実装モデル要件**: frontier-required

**判定理由**: 4-3 は宛先の差し替え（`rename`）という不可逆な境界を新設するセキュリティゲートのステップであり（`mkplan.md` step 8 のパネルモード・トリガ）、4-4 の `Remove`／`Truncate` 削除が同一 PR に同居しないと「後始末の手段が無い」または「使われない `Remove` が残る」中間状態が生じる（`02_architecture.md` § 8）。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: 契約の明記と監査文書への反映（F-007・F-008 / AC-29〜38）

対応する設計: [02_architecture.md](02_architecture.md) § 3.7・§ 3.9・§ 7.3。

**変更するファイル**: `internal/safefileio/safe_file.go`・`safe_file_nonlinux.go`・
`docs/user/security-risk-assessment.ja.md`・`docs/user/security-risk-assessment.md`・
`docs/dev/architecture_design/security-architecture.ja.md`・
`docs/dev/architecture_design/security-architecture.md`・
`docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md`・
`docs/tasks/0149_security_code_smell_audit_fable/findings/B1_safefileio.md`

#### 5-1. package コメントと公開 API の doc コメント

- [ ] `safe_file.go` の package コメント（:1-6）に、`openat2` が使える環境とフォールバック経路とで保証の
      強さが異なること、後者は競合の隙を狭めるが排除はしない best-effort であること、本番ターゲットは
      Linux 5.6+ であり非 Linux は開発・限定用途に限ることを英語で追記する（AC-29・31・33）。
- [ ] `safe_file_nonlinux.go` の package コメント（:3-7）の表現を、上の package コメントと矛盾しない
      共通の言い回しに揃える。
- [ ] `SafeOpenFile`・`SafeReadFile`・`SafeWriteFileOverwrite`・`AtomicMoveFile` の各 doc コメントに、
      package コメントの限界の記述への参照（`See the package documentation …`）を英語で足す（AC-30・33）。
- [ ] **古くなった仕組みの説明を書き換える。** `AtomicMoveFile` の doc コメント（`safe_file.go:100-104`）は
      「`SafeOpenFile` via openat2 RESOLVE_NO_SYMLINKS for the source、`ensureParentDirsNoSymlinks` for the
      destination parent」と、Phase 3 で置き換わる仕組みを名指ししている。`SafeWriteFileOverwrite` の
      doc コメント（:117-127）も「falls back to path verification before opening the file」と、一時ファイル
      方式になる前の手順を説明している。両方を新しい仕組み（`openDirNoSymlinks`／`openFileAt`、および
      一時ファイルへ書いて差し替える手順）に合わせて書き直す。
- [ ] `AtomicMoveFile` の doc コメントに [02_architecture.md](02_architecture.md) § 3.4.4 が挙げる 4 点を
      英語で追記する（AC-08・09）。
- [ ] `SafeWriteFileOverwrite` の doc コメントに、差し替えに到達する前の失敗では宛先が書き込み前の内容の
      ままであること、到達後の失敗は `ErrDestinationCommitted` を含み `errors.Is` で判別できることを英語で
      追記する（[02_architecture.md](02_architecture.md) § 5.2）。
- [ ] `canSafelyReadFromFile` の doc コメント（`safe_file.go:448-450`）に、読み取り検査が所有者 UID を見ず
      `(gid, mode)` だけで判定すること、それが意図的であること、および AC-32 が挙げる 2 つの理由
      （ディレクトリ権限監査との役割分担、分離運用の成立条件）を英語で書く。あわせて、この非対称性が
      「ソースをパス名で開き直してはならない」理由でもあることを 1 文で書く
      （[02_architecture.md](02_architecture.md) § 3.7 末尾）。
#### 5-2. 利用者向け文書・設計文書の更新

- [ ] `docs/user/security-risk-assessment.ja.md:158-172` の `safeOpenFileInternal` の引用を、Phase 1 の
      変更後の実装（`mode: openat2Mode(flag, perm)` と `EINTR` 再試行を含む形）に合わせる（AC-38）。
- [ ] 同 :204 付近の `safeOpenFileFallback` の説明を、Phase 2 の作成プローブを含む手順に合わせる（AC-38）。
- [ ] 同文書「前提と限界」節（:180-211）の本番ターゲット（Linux 5.6+）と非 Linux の位置づけの記述が、
      package コメントの追記と同じ内容であることを読み合わせて確認する（AC-31）。食い違いがあれば
      package コメント側を文書に合わせる。
- [ ] `docs/dev/architecture_design/security-architecture.ja.md:206-215` の `openat2()` の引用を、`EINTR`
      再試行後の形に更新する。
- [ ] 同 :217-233 の `ensureParentDirsNoSymlinks()` の引用の見出しと関数名を `ensureDirNoSymlinks()` へ
      変える。走査のループそのものは変わらないが、Phase 3 で本体がこの関数へ移るため、引用が「どの関数の
      本体か」を誤って示すことになる。
- [ ] 日本語版（`security-risk-assessment.ja.md`・`security-architecture.ja.md`）をコミットしたうえで、
      `/mktrans` で英語版（`security-risk-assessment.md`・`security-architecture.md`）へ反映する。
      日英を直接両方編集しない。
#### 5-3. 0149 監査記録への反映

- [ ] `98_remaining_issues.md` §2 の「B1（safefileio）」（:53-61）から F-2〜F-9 の箇条書き 5 行（:55-59）を
      取り除き、同文書が :15・:17・:51 で用いている `> **B1 F-2〜F-9 について**: …` の引用ブロック形式で、
      本タスクと #978 への参照を含む解消済みの記述に置き換える（AC-34）。
- [ ] 同じ引用ブロックに、F-2・F-4-2・F-8 を所見の主推奨とは異なる形で close したことと、その根拠
      （本番ターゲットの限定、0155 の既存の設計決定、読み取り側のポリシーの所在）を書く（AC-35）。
- [ ] `findings/B1_safefileio.md` の F-2〜F-9 の各節に `- 対応状況: …` の箇条書きを 1 つずつ足す
      （計 8 箇所）。所見の原文（該当箇所・問題・悪用シナリオ・推奨対応）は書き換えない（AC-36）。
- [ ] `98_remaining_issues.md` の B1 以外の節に変更行が出ていないことを差分で確認する（AC-37）。

**完了条件**: `make test` と `make lint` が通る。§7 の AC 検証をすべて実施済みである。

### PR-6 作成ポイント: failure contracts and audit-document updates

**対象ステップ**: 5-1 / 5-2 / 5-3

**推奨タイトル**: `docs(0167): state the failure contracts and update the audit records`

**レビュー観点**: doc コメントが Phase 3・4 で置き換わった仕組み（`openDirNoSymlinks`／`openFileAt`、一時ファイル方式）を正しく名指ししており、古い説明が残っていないか / `canSafelyReadFromFile` の非対称性の説明が AC-32 の 2 つの理由を含むか / 日本語版を先にコミットし英語版を `/mktrans` で反映する手順が守られているか / `98_remaining_issues.md` の変更が B1 節の範囲に収まっているか（AC-37）

**実装モデル要件**: standard

**判定理由**: 文書と doc コメントの更新のみで、`既存コード調査結果` に競合する実装案は無く、Conditional checks にも該当せず、復旧処理・並行処理・状態機械のような高リスクステップも含まないため、frontier のトリガに該当しない。5-3 は F-2・F-4-2・F-8 を所見の主推奨とは異なる形で close した理由を書く判断を含むが、根拠（本番ターゲットの限定、0155 の設計決定、読み取り側のポリシーの所在）は本書と `02_architecture.md` に既に書かれており、`weakreview.md` のパスで裏を取れる範囲に収まる。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | Phase | 成果物 | 完了の判定 |
|---|---|---|---|
| M1 | Phase 1: mode の検証・正規化と `openat2` の `EINTR` 再試行 | `ErrUnsupportedFileMode`・`validateOpenPerm`・`openat2Mode`・`EINTR` 再試行 | AC-14〜17・AC-26〜28 が検証済み |
| M2 | Phase 2: 共通ヘルパの整理とフォールバック経路の後始末 | `verifySameFile` の共通化、`removeVerifiedFileByPath`、作成プローブ | AC-01〜05 が検証済み |
| M3 | Phase 3: ディレクトリ fd プリミティブと `moveFileAnchored` の書き換え | `ensureDirNoSymlinks`・`openDirNoSymlinks`・`openFileAt`・新シグネチャの `moveFileAnchored`／`linkFileToTempName` | 既存テストが検証内容を変えずに通り、Linux の外部挙動が変わっていない |
| M4 | Phase 4a・4b: 移動経路の分割／書き込みのアトミック化と `Remove` の削除 | `ErrDestinationCommitted`・`moveOpenFileCore`・一時ファイル方式の `safeWriteFileCommon`・`Remove` の削除 | AC-06〜13・AC-18〜25 が検証済み。性能表が埋まっている |
| M5 | Phase 5: 契約の明記と監査文書への反映 | doc コメント、利用者向け文書、0149 の監査記録 | AC-29〜38 が検証済み |

Phase の名前・順序・先行条件は [02_architecture.md](02_architecture.md) § 8 の表に従う。§ 8 の Phase 4 は、
レビューの関心が移動経路と書き込み経路に分かれるため、本書では 4a と 4b の 2 つの節に分けて記す。段階の
内容・順序・先行条件は変えていない。Phase 3 を独立させる
理由と、Phase 4 で `Remove` の削除と一時ファイルの後始末を同じ段階に置く理由も同節にある。

### 3.2 PR 構成

Phase の区切りをそのまま PR の区切りにする。§ 8 の Phase 4 だけは移動経路と書き込み経路でレビューの
関心が分かれるため、本書で 4a（4-1・4-2）と 4b（4-3〜4-6）に分け、それぞれを 1 つの PR にする。

4b の 4 ステップを 1 つの PR にまとめる理由は、ステップごとに異なる。

- **4-3 と 4-4**: `FileSystem.Remove` の削除と一時ファイルによる後始末は互いの代替手段であり、分けると
  「後始末の手段が無い」または「使われない `Remove` が残る」中間状態が生じる
  （[02_architecture.md](02_architecture.md) § 8）。
- **4-5**: 4-3 の書き換えで既存テスト 3 件が落ちるようになり、それを削除・置換するのが 4-5 である。
  分けるとどちらの PR もグリーンゲートを通らない。
- **4-6**: `fsync` の追加を受け入れる根拠が絶対値の実測であり（`01_requirements.md` の F-7、§ 5 の R-4）、
  別 PR にすると根拠の無いまま 4-3 がマージされる。

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1-1 / 1-2 / 1-3 | `ErrUnsupportedFileMode` と `validateOpenPerm` による mode の検証、`openat2Mode` による正規化、`openat2` の `EINTR` 再試行と `openat2Syscall` 差し替え点 | frontier-required |
| PR-2 | 2-1 / 2-2 / 2-3 | `verifySameFile`・`randomTempName` の共通化と `ErrTempNameExhausted` への改名、`safeOpenFileFallback` の作成プローブと失敗時の後始末 | frontier-recommended |
| PR-3 | 3-1 / 3-2 / 3-3 / 3-4 | `ensureDirNoSymlinks`・`openDirNoSymlinks`・`openFileAt` の新設と、`moveFileAnchored`／`atomicMoveFileCore` のディレクトリ fd 起点への組み替え（Linux の外部挙動は不変）、プリミティブのテスト | frontier-recommended |
| PR-4 | 4-1 / 4-2 | `ErrDestinationCommitted` の追加、`moveOpenFileCore` の切り出しと検査順序の変更、`rename` 前後の契約の分離と移動経路のテスト | frontier-required |
| PR-5 | 4-3 / 4-4 / 4-5 / 4-6 | `safeWriteFileCommon` の一時ファイル方式への書き換え、`File.Sync` の追加、`FileSystem.Remove`／`File.Truncate` の削除、書き込み経路のテストの入れ替え、性能の実測 | frontier-required |
| PR-6 | 5-1 / 5-2 / 5-3 | package コメントと公開 API の doc コメントへの失敗時契約の明記、利用者向け・設計文書の更新と英語版反映、0149 監査記録への反映 | standard |

---

## 4. テスト戦略

### 4.1 単体テスト

- 対象は `internal/safefileio` の新規・変更関数。方針は [02_architecture.md](02_architecture.md) § 7.1 に従う。
- 権限に関するテストは、**観測する側**（作成されたファイルの権限を見る）では `syscall.Umask` を固定し、
  **フィクスチャを作る側**（拒否されるべき権限のファイルを用意する）では作成後に `os.Chmod` で明示的に
  設定する。`os.WriteFile` の perm 引数は umask に削られるため、`0o666` のつもりが `0o644` になって
  拒否条件が成立しない。`Umask` を触った場合は `t.Cleanup` で必ず元へ戻す（プロセス全体に効くため）。
- グループの条件は「実行者が属さないグループ」「実行者が属していて他にも構成員がいるグループ」を
  明示的に用意して作る。用意できない環境では理由を明記して `t.Skip` する。実行者の主グループに依存した
  期待値を置かない。
- 経路の切り替えは `FileSystemConfig{DisableOpenat2: true}` で行い、mode の検証（AC-14〜17）は両経路で
  同じ表を流す。openat2 経路を通すつもりの行では `IsOpenat2Available()` が true であることを `require` し、
  false の環境では skip する。openat2 が使えない環境では `FileSystemConfig{}` が静かにフォールバック
  経路になり、両経路を通したつもりのテストが同じ経路を 2 回通ってしまうためである。
- このパッケージのテストは `t.Parallel()` を使わない（`safe_file_linux.go:120-121` の既存の制約。本タスクで
  差し替え点が 3 つ増えるため、この制約は新しいテストにも等しく当てはまる）。
- 差し替えたパッケージ変数は必ず `t.Cleanup` で元へ戻す（既存の
  `TestLinkFileToTempName_RetriesOnNameCollision` と同じ形）。`openat2Syscall` を差し替えるテストは、
  `NewFileSystem` が `isOpenat2Available` 経由でこの差し替え点を 1 回消費することを踏まえ、`FileSystem` を
  差し替えの前に構築する。

### 4.2 統合テスト

- 後始末・アトミック性・シンボリックリンクの拒否は、モックではなく実ディレクトリ（`SafeTempDir`）上で
  検証する（[02_architecture.md](02_architecture.md) § 7.2）。モックの `File` は `Dev`・`Ino` を持たないため
  同一性が一致する分岐を作れず、`removeVerifiedFileByPath`／`removeVerifiedFileAt` の**一致しない**分岐の
  検証にのみ使う。
- 「一時ファイルが残らない」の検証は、宛先ディレクトリを列挙して `.safefileio-` で始まるエントリが 0 件で
  あることで行う。特定の一時ファイル名を仮定しない。
- `t.TempDir` の外へ書くテスト（`/tmp` 直下の OS 管理シンボリックリンクの検証）は、名前を実行ごとに一意に
  し、作成の直後に `t.Cleanup` で削除を登録する。
- 警告（`slog.Warn`）の検証は、`internal/testutil` の `NewRecordingLogger` で作った記録用ロガーを
  `slog.SetDefault` で既定に据え、`t.Cleanup` で元へ戻したうえで `LogRecorder.RequireRecord` を使う。
- `internal/fileanalysis` と `internal/runner/base/output` の既存テストを、書き換えずに回帰として使う。

### 4.3 後方互換性の確認

- `SafeWriteFileOverwrite` が成功したときに書き込まれる内容と、`AtomicMoveFile` が成功したときの宛先が
  本タスクの前後で変わらないことを、上記の既存テストで確認する。
- 既存の解析レコードファイルが変更なしに読めることは、`TestStore_Load_V9DynLibDepsObjectFormat` などの
  固定入力を読むテストが担う。
- 意図した挙動の変化は `01_requirements.md` Success Criteria の 7 点に限る（7 点目が Phase 3 の非 Linux
  の `ErrSourceIdentityMismatch` である）。これを超える差分が見つかった場合は実装ではなく設計の問題として
  扱い、`02_architecture.md` の改訂から行う。

### 4.4 テストが理由どおりに失敗できることの確認

後始末（Phase 2）・検査順序の入れ替え（Phase 4-1。確認は Phase 4-2 の最終ステップ）・`EINTR` 再試行
（Phase 1）のそれぞれについて、対象の処理を取り除いた状態でテストが失敗することを確認し、取り除いた方法と
結果をコミットメッセージに記す（AC-05・07・28）。fd リークの確認は `GOGC=off` で行い、ファイナライザが漏れた fd を閉じてテストが誤って
通ることを防ぐ。

### 4.5 外部サービスへの依存

本タスクは Slack その他の外部サービスを新たに使用しない。対象クライアント環境での検証は該当しない（N/A）。

---

## 5. リスク管理

| # | リスク | 影響 | 対応 |
|---|---|---|---|
| R-1 | ディレクトリ fd 起点への変更（Phase 3）が広範で、非 Linux 版のコンパイルエラーに CI で気づけない | 非 Linux のビルドが壊れたまま進む | 各 Phase の完了条件に `GOOS=darwin go vet -tags test` と `GOOS=netbsd go vet -tags test` を含める。`-tags test` が必要なのは、テスト側が `//go:build test` の `internal/testutil` を import しているためで、この形が変更前のツリーで通ることは確認済み（§ 1「ビルド検査の前提」）。`unix.Openat`・`unix.Renameat`・`unix.Fstatat`・`unix.Unlinkat` の存在は各 GOOS で `go doc` により確認済み |
| R-2 | `FileSystem` のモックが書き込み・移動の経路に届かなくなり、既存テストの差し替え点が消える | 既存の失敗経路の検証が失われる | § 1 の表で失われる主張を 1 件ずつ洗い出し、実ファイルシステム上のテストまたは `linkatFunc`／`verifyMovedFile` の差し替え点へ置き換える。`go tool cover -func` の前後比較で漏れを確認する |
| R-3 | AC-07a の「実行者が属さないグループから書き込み可能なソース」と、差し替え前拒否テストの「他にも構成員がいるグループ」を、非特権環境で用意できない | 3 つの拒否条件のうち 1 つ、および § 3.4.3 の前倒し検査が skip になる | どちらも `t.Skip` の条件と理由をテスト内に明記する。AC-07a の他の 2 条件（world-writable・上限超過）は権限を要さず常に実行され、拒否経路そのものは無条件に踏まれる |
| R-4 | `fsync` の追加により `record` の実行時間が延びる | 対象が数百件のとき 0.1〜数秒の増加 | Phase 4-6 で絶対値を実測して記録する。相対比では判断しない（CLAUDE.md の性能方針）。受け入れる根拠は [02_architecture.md](02_architecture.md) § 3.6.4 にある |
| R-5 | 作成プローブにより、`O_CREATE` を `O_EXCL` なしで使う呼び出しが `ENOENT` で失敗しうる | `internal/runner/bootstrap/logger.go:246` のログファイル open が、フォールバック経路でのみ失敗しうる | 本番ターゲット（Linux 5.6+）では `openat2` 経路が使われプローブは動かない。上限つき再試行で通常の競合は吸収する。`01_requirements.md` Success Criteria に既に記載済みの変化である |
| R-6 | `98_remaining_issues.md` の書き換えが B1 以外の節に及ぶ | 監査記録の他の残件が失われる | AC-37 の差分確認を Phase 5 の完了条件に含める |
| R-7 | テスト用の差し替え点を、セキュリティ上重要な経路へ本番でも書き換えられる形で増やしてしまう | 攻撃面が広がる | ビルドタグで本番側とテスト側を排他的に定義し、本番ビルドには差し替え可能な値を一切残さない（§ 1。2026-08-21 承認）。この形が成立していることは、タグ無しのビルド（`go build ./internal/safefileio/`）が通り、かつ本番側のファイルにパッケージ変数が現れないことで確認する（§7 の AC 検証とは別に、Phase 1・2・4 の完了条件に含める） |

---

## 6. 実装チェックリスト

Phase ごとの作業内容は § 2 に、PR の区切りは § 3.2 にある。本節は PR 単位のマージ進捗だけを追う。

- [x] PR-1 マージ済み（対象ステップ: 1-1 / 1-2 / 1-3。[#1050](https://github.com/isseis/go-safe-cmd-runner/pull/1050)）
- [ ] PR-2 マージ済み（対象ステップ: 2-1 / 2-2 / 2-3）
- [ ] PR-3 マージ済み（対象ステップ: 3-1 / 3-2 / 3-3 / 3-4）
- [ ] PR-4 マージ済み（対象ステップ: 4-1 / 4-2）
- [ ] PR-5 マージ済み（対象ステップ: 4-3 / 4-4 / 4-5 / 4-6）
- [ ] PR-6 マージ済み（対象ステップ: 5-1 / 5-2 / 5-3）

### 全体

- [ ] すべての AC が §7 の方法で検証済み
- [ ] `make test` と `make lint` が警告なく通る
- [ ] `make deadcode` に本タスク由来の新規項目が無い

---

## 7. Acceptance Criteria 検証

`git diff` を用いる検証は、コミット後でも意味を持つようマージベースからの差分を対象とする。本タスクは
§ 3.2 の 6 つの PR に分けて出すため、比較の基点に `origin/main` を使うと PR がマージされるたびに基点が
進み、AC-12・AC-36・AC-37 と § 8 の追加行検査が「直前の PR の差分しか見ていない」状態で通ってしまう。
そこで**タスク全体で固定した基点** `$BASE` を使う。`$BASE` は PR-1 のブランチを切った時点の
`origin/main` のコミットとし、最初の PR の作業開始時に確定させて以降動かさない。**実際の `$BASE` は
`a01a45e8`（PR #1049 のマージコミット）である**（2026-08-21 確定。本書の作成時点の見込みは `5bcc2be8`
だったが、その後 PR #1049 がマージされたため実際の分岐点は先に進んだ）。以下の表の `$BASE...HEAD` はすべてこの意味である。`go tool cover -func`（AC-11）と
`make deadcode`（AC-13）の「変更前」も同じ基点の作業ツリーを指す。「種別」の意味は `test`（実行して挙動が違えば落ちる）、`static`（`rg`／コンパイル／`make` の
出力で判定する）、`manual`（人の読み合わせ）である。

`rg` の検索式は Rust の正規表現構文であり、選択は `|` と書く（`\|` はリテラルのパイプ文字にマッチする
別物で、意図した検索にならない）。「0 件であること」を主張する検索式は、実装前に一度、意図した対象が
現に見つかることを確かめてから使う。

| AC | 種別 | 検証方法 |
|---|---|---|
| AC-01 | test | `internal/safefileio/safe_file_linux_test.go::TestSafeOpenFileFallback_ClosesFDWhenPostCheckFails` の `not_created`／`created` サブテスト（一時ディレクトリ配下を指す `/proc/self/fd` エントリの集合が呼び出しの前後で不変で あること）。2 つに分けるのは、fd を解放する経路が分岐ごとに違うためである（作成していない場合はその場の `Close`、作成していた場合は `removeVerifiedFileByPath` 経由） |
| AC-02 | test | `internal/safefileio/safe_file_cleanup_test.go::TestSafeOpenFileFallback_RemovesCreatedFileWhenPostCheckFails` の `created`／`pre_existing` サブテスト。両者が分かれる前提である「作成したか既にあったか」の判定そのものは `internal/safefileio/safe_file_test.go::TestSafeOpenFileFallback_CreationProbe`（5 サブテスト）が押さえる。うち `rejects_leaf_symlink` は、作成プローブの開き直しが `O_NOFOLLOW` を保ち、リーフ symlink を拒否するという ADR の前提を崩していないことの検証である |
| AC-03 | test | 同テストの `identity_mismatch` サブテスト（差し替えたファイルが削除されないこと、返るエラーが 2 回目の確認のエラーで `ErrSourceIdentityMismatch` ではないこと、警告が記録されること）。ヘルパ単体は `internal/safefileio/safe_file_cleanup_test.go::TestRemoveVerifiedFileByPath_SkipsRemovalOnInodeMismatch` |
| AC-04 | test | AC-01〜AC-03 の各テストが、それぞれ「2 回目の確認の失敗 → Close のみ」「同 → 作成済みファイルの削除」「同 → 既存ファイルの保持」「同 → 同一性不一致で削除せず警告」の 4 分岐を 1 つずつ踏む。`go test -tags test -run 'TestSafeOpenFileFallback_|TestRemoveVerifiedFileByPath_' -v ./internal/safefileio/` の出力で各分岐名の PASS を確認する |
| AC-05 | test + manual | AC-01〜03 の 3 テストが `test` の主体である。加えて Phase 2 の最終ステップで後始末（`Close` と `removeVerifiedFileByPath` の呼び出し）を外し、`GOGC=off` の下でそれらが落ちることを確認してコミットメッセージに記す |
| AC-06 | test | `internal/safefileio/safe_file_test.go::TestAtomicMoveFile_ValidatesSourceBeforeChmod` |
| AC-07 | test + manual | 同上のテストがソースの権限の不変を検証する。加えて Phase 4-2 の最終ステップで順序を元に戻すと落ちることを確認し、コミットメッセージに記す |
| AC-07a | test | `internal/safefileio/safe_file_test.go::TestAtomicMoveFile_RejectsUnsafeSourcePermissions`（`world_writable`・`group_writable_non_member` の 2 サブテスト。2 番目は `os.Chown` が `EPERM` の環境で `t.Skip`。`perms_exceed_maximum` は実ファイルの mode では到達できないため置かない。§ 2 Phase 4-2） |
| AC-07b | test | `internal/safefileio/safe_file_test.go::TestAtomicMoveFile_SafeSourceStillMoves`、および `internal/runner/base/output/file_test.go::TestSafeFileManager_MoveToFinal` が無変更で通る |
| AC-08 | static | `rg -n -B 30 "func \(fs \*osFS\) AtomicMoveFile" internal/safefileio/safe_file.go`（doc コメントはシグネチャの手前にあるため後方向に見る）の出力に `ErrDestinationCommitted`、宛先にファイルが残る旨、移動前の内容が復元されない旨の 3 点が現れる |
| AC-09 | static | 同じ出力を `rg -i "rollback"` に通して 1 件以上ヒットし、その文がロールバックしない理由（上書き時に元の内容を復元できないこと）を述べている |
| AC-10 | static | `rg -n "^\s*Remove\(name string\) error" internal/safefileio/safe_file.go` が 0 件、`rg -n "func \(fs \*osFS\) Remove" internal/safefileio/safe_file.go` が 0 件、`rg -n "func \(m \*MockFileSystem\) Remove" internal/safefileio/testutil/mock.go` が 0 件。`rg -n "Remove"` 全体を 0 件にする形では検査できない（`removeVerifiedFileByPath` が `os.Remove` を呼ぶため） |
| AC-11 | static | `rg -n "getRemoveCallCount|removeCallCount|removeFunc|RemoveCalls|RemoveFunc" internal/ cmd/` が 0 件（変更前は 5 件以上ヒットすることを実施前に確認する）。加えて `go tool cover -func` の出力を変更の前後で比較し、`internal/safefileio` の関数単位のカバレッジが低下していないことを確認してコミットメッセージに記す |
| AC-12 | test | `internal/runner/base/output/file_test.go::TestSafeFileManager_RemoveTemp` と `::TestSafeFileManager_RemoveTemp_WithMock` が無変更で通る。加えて `git diff --exit-code $BASE...HEAD -- internal/common/filesystem.go internal/runner/base/output/file.go` が差分なし |
| AC-13 | static | `make deadcode` の出力を変更の前後で比較し、`internal/safefileio` 由来の新規項目が 0 件であること |
| AC-14 | test | `internal/safefileio/safe_file_test.go::TestSafeOpenFile_RejectsNonPermissionModeBits`（setuid・setgid・sticky・`os.ModeDir`・`os.ModeAppend` × 両経路の表。すべて `errors.Is(err, ErrUnsupportedFileMode)`） |
| AC-15 | test | `internal/safefileio/safe_file_test.go::TestSafeOpenFile_ReadOpenPermIgnoredOnBothPaths`（`O_CREATE` なし・非ゼロ `perm` で両経路とも成功）と `internal/safefileio/safe_file_linux_test.go::TestOpenat2_ReadOpenPassesZeroMode`（`openHow.mode` が 0） |
| AC-16 | test | `internal/safefileio/safe_file_test.go::TestSafeOpenFile_CreatePermUnchanged`（umask を固定し、作成されたファイルの権限が両経路とも `0o640 &^ umask` と一致すること） |
| AC-17 | test | AC-14〜16 の 3 テストがいずれも `FileSystemConfig{}` と `FileSystemConfig{DisableOpenat2: true}` の両方を表の行として持ち、前者の行が `IsOpenat2Available()` の true を `require` する |
| AC-18 | test | 差し替え前: `internal/safefileio/safe_file_test.go::TestSafeWriteFileOverwrite_ExistingDestinationRejectedLeavesItIntact` と `internal/safefileio/safe_file_linux_test.go::TestSafeWriteFileOverwrite_PreCommitFailureLeavesDestinationIntact`（いずれもエラーが `ErrDestinationCommitted` を含まないことを併せて検証）。差し替え後: `internal/safefileio/safe_file_test.go::TestSafeWriteFileOverwrite_PostCommitFailureIsDestinationCommitted`（`errors.Is` で `ErrDestinationCommitted` を満たし、宛先が新しい内容であること） |
| AC-19 | test | `internal/safefileio/safe_file_test.go::TestSafeWriteFileOverwrite_SucceedsWithPermApplied`（内容の一致と、新規作成・既存上書きの両方で宛先の権限が `perm` と一致すること） |
| AC-20 | test | `internal/safefileio/safe_file_test.go::TestSafeWriteFileOverwrite_RejectsSymlinkDestination` |
| AC-21 | test | 差し替え前: AC-18 の 2 テストが、宛先ディレクトリに `.safefileio-` で始まるエントリが 0 件であることを併せて検証する。差し替え後: `::TestSafeWriteFileOverwrite_PostCommitFailureIsDestinationCommitted` が、一時ファイルの削除を試みていないことと警告が記録されることを検証する |
| AC-22 | test | `internal/safefileio/safe_file_test.go::TestResolvedPathModeEnforcement` が無変更で通る |
| AC-23 | test | AC-18 の差し替え前の 2 テストが、失敗後の宛先の内容と宛先ディレクトリの残存エントリの両方を検証する |
| AC-24 | test | AC-20 のテストが、リンク先の内容が不変であることと、宛先が依然としてシンボリックリンクであること（`os.Lstat` の `ModeSymlink` と `os.Readlink` の戻り値）の両方を検証する |
| AC-25 | test | `internal/fileanalysis/file_analysis_store_test.go::TestStore_SaveAndLoad`・`::TestStore_PreservesExistingFields`・`::TestStore_ArgEvalResultsRoundtrip`・`::TestStore_Load_V9DynLibDepsObjectFormat` が無変更で通る |
| AC-26 | test | `internal/safefileio/safe_file_linux_test.go::TestOpenat2_RetriesOnEINTR` |
| AC-27 | test | `internal/safefileio/safe_file_linux_test.go::TestOpenat2_NonEINTRErrnoMapping`（`ELOOP`→`ErrIsSymlink`、`EEXIST`→`ErrFileExists`、`ENOENT`→`os.ErrNotExist`） |
| AC-28 | test + manual | AC-26 のテストが呼び出し回数 2 を検証する。加えて Phase 1 の最終ステップで再試行ループを外すと落ちることを確認し、コミットメッセージに記す |
| AC-29 | static | `rg -n -m 1 -A 25 "^// Package safefileio" internal/safefileio/safe_file.go` の出力に `openat2`・`best-effort`・`5.6` の 3 語がすべて現れる |
| AC-30 | static | `rg -n -B 20 -e "^func \(fs \*osFS\) SafeOpenFile\(" -e "^func SafeReadFile\(" -e "^func SafeWriteFileOverwrite\(" -e "^func \(fs \*osFS\) AtomicMoveFile\(" internal/safefileio/safe_file.go` の出力に、4 つのシグネチャそれぞれの直前 doc コメントで package コメントを指す語（`See the package documentation`）が現れる。末尾の `\(` を省くと内部ラッパの `SafeReadFileWithFS` も拾って 5 件になるため、括弧まで含めて指定する |
| AC-31 | static + manual | `rg -n "5\.6" internal/safefileio/safe_file.go docs/user/security-risk-assessment.ja.md` が両ファイルでヒットする。そのうえで package コメントの追記と同文書「前提と限界」節（:180-211）を読み合わせ、本番ターゲットが Linux 5.6+ であること、非 Linux は開発・限定用途に限ることの 2 点が同じ内容であることを確認する |
| AC-32 | static | `rg -n -B 20 "^func canSafelyReadFromFile" internal/safefileio/safe_file.go` の出力に、`(gid, mode)` のみで判定する旨、意図的である旨、ディレクトリ権限監査への言及、および所有者と読み手が異なる運用への言及がすべて現れる |
| AC-33 | static | `rg -n "[\x{3000}-\x{303F}\x{3040}-\x{30FF}\x{4E00}-\x{9FFF}\x{FF00}-\x{FFEF}]" internal/safefileio/ internal/security/machoanalyzer/analyzer_test.go` が 0 件（`_test.go` と `testutil/` を含む。ひらがな・カタカナ・漢字に加えて長音符・全角記号も捕まえる。現行ツリーで 0 件であることを確認済み） |
| AC-34 | static | `rg -n "F-2〜F-9" docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` が引用ブロック内の 1 件のみヒットし、その行を含むブロックに `0167`・`#978`・`解消` の語が含まれる。加えて `rg -n "^\s+- F-[0-9]" docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` が 0 件（残件としての 5 行がすべて消えていること。変更前は 5 件ヒットする） |
| AC-35 | static | 同じ引用ブロックに `F-2`・`F-4-2`・`F-8` の 3 つと、`0155`・`5.6`・`ディレクトリ権限監査` に相当する根拠の語が現れる |
| AC-36 | static | `rg -c "^- 対応状況:" docs/tasks/0149_security_code_smell_audit_fable/findings/B1_safefileio.md` が 8。加えて `git diff --numstat $BASE...HEAD -- docs/tasks/0149_security_code_smell_audit_fable/findings/B1_safefileio.md` の削除行数が 0（所見の原文を書き換えていないこと） |
| AC-37 | static | `git diff $BASE...HEAD -- docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` の変更行が `### B1（safefileio）` 節の範囲内に収まっている（他節への変更行が 0 件） |
| AC-38 | static | `docs/user/security-risk-assessment.ja.md` の 2 つのコード片を実装と 1 行ずつ突き合わせる。裏づけとして `rg -n "uint64\(perm\)" docs/user/security-risk-assessment.ja.md docs/user/security-risk-assessment.md` が 0 件（Phase 1 で消える式が文書に残っていないこと。変更前は 2 件ヒットする）、かつ `rg -n "openat2Mode" docs/user/security-risk-assessment.ja.md docs/user/security-risk-assessment.md` が両ファイルで 1 件以上ヒットすること |

---

## 8. 横断検索チェックリスト

`make lint` と `make test` では気づけない残存参照だけを挙げる。§7 の AC 検証と重複する項目はここに置かない。
`rg` の選択は `|` で書く（§7 冒頭の注記と同じ）。

- [ ] `rg -n "ErrTempLinkNameExhausted" .` が 0 件（改名の取りこぼし。コード側はコンパイルが捕まえるが、
      コメントと文書は捕まえない）
- [ ] `rg -n "randomTempName\(\)" .` が 0 件（接頭辞を取らない旧シグネチャへの言及がコメントに残っていない）
- [ ] `rg -n "maxLinkatAttempts" .` が 0 件（`maxTempNameAttempts` への改名の取りこぼし）
- [ ] `rg -n "ensureParentDirsNoSymlinks" internal/safefileio/ docs/` の各ヒットが、ラッパとして残る関数を
      正しく指している（`openDirNoSymlinks`／`ensureDirNoSymlinks` が担うようになった役割を、この関数の
      ものとして説明している箇所が残っていない）
- [ ] `rg -n "safefileio.*Remove|FileSystem.*Remove" docs/` の結果に、削除した `safefileio.FileSystem.Remove`
      を現存するものとして説明している箇所が無い（`internal/common` の `Remove` への言及は残ってよい）
- [ ] `rg -n "Truncate\(0\)" docs/ internal/safefileio/` の結果に、`safeWriteFileCommon` が宛先を切り詰める
      という説明が残っていない
- [ ] `git diff $BASE...HEAD -- '*.go' | rg -n "^\+.*(AC-[0-9]|F-00[0-9])"` が 0 件（本タスクが Go
      ソースへ AC 番号を持ち込んでいないこと。`runplan` のコミット前検査が拒否する。ツリー全体を対象に
      すると本タスクと無関係な既存のヒットが出るため、追加行だけを見る）

---

## 9. Success Criteria

- §7 のすべての AC が、そこに書いた方法で検証済みである。
- `make test` と `make lint` が警告なく通る。`GOOS=darwin` と `GOOS=netbsd` の `go vet -tags test` も通る。
- `make deadcode` に本タスク由来の新規項目が無い。
- 本タスクの前後で、`safefileio` の公開 API が成功したときに書き込まれる内容と移動先のファイルが
  変わらない。挙動の変化は `01_requirements.md` Success Criteria が挙げる 7 点に限られる。
- リーフのシンボリックリンクを検知して拒否するという ADR の設計前提が、すべての公開 API について
  維持されている。
- Phase 4-6 の性能表が実測値で埋まっている。
- #978 が挙げる 8 件それぞれについて、解消したのか所見の推奨とは異なる形で close したのかが、コードと
  監査文書の双方から追える。

---

## 10. 次のステップ

- 本書は `approved` である。本書の作成時に未決だった 3 点は 2026-08-21 に決着した。差し替え点は本番
  ビルドに可変の値を残さない 2 ファイル方式とする（§ 1）、Phase 3 が非 Linux にもたらす挙動の変化は
  承認され `01_requirements.md` Success Criteria の 7 点目として追記済み、`File.Truncate` は書き込み
  経路の書き換えのあとに削除する（§ 2 Phase 4-4）。
- § 3.2 の PR-1 から順に実装する。1 つの PR につき 1 つのブランチを使い、マージしてから次の PR の
  ブランチを切る。Phase 3（PR-3）は外部挙動をほぼ変えないため、移動経路を組み替える PR-4 とは分けて
  レビューを受ける。
- PR-6 のマージ後に #978 を close する。
- [02_architecture.md](02_architecture.md) § 9 が挙げる将来の候補（非 Linux の dirfd ウォーク、
  `internal/dynamicanalysis` と `internal/libccache` の `writeFileAtomic` の統合、ディレクトリの `fsync`）は
  本タスクでは扱わない。統合の候補については、4 つ目の同一実装を増やさないよう、新しい書き込み処理を
  足す際に本書と § 3.6.1 を参照する。
