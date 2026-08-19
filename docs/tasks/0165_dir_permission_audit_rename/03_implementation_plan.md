# 実装計画書: ディレクトリ権限監査の TOCTOU 改名

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-08-19 |
| Review date | 2026-08-19 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- issue: [#1033](https://github.com/isseis/go-safe-cmd-runner/issues/1033)
- 出発点: [0164 実装計画書 §11](../0164_entrypoint_residual_low_info/03_implementation_plan.md#11-後続タスク-ディレクトリ権限監査の改名)（目的・対象・完了条件はここで確定済み。本書はその作業手順のチェックボックス展開のみを担う）

---

## 1. 実装の全体像

### 1.1 目的

`RunTOCTOUPermissionCheck` とその呼び出し側が行っているのは、収集したディレクトリを1回ずつ `lstat` して権限・所有者・経路要素を判定する静的なディレクトリ権限監査であり、check 時と use 時の観測を突き合わせる TOCTOU チェックではない。本来の TOCTOU 対策は `internal/safefileio` の `O_NOFOLLOW` 経由の open と、`internal/runner/base/executor` のファイル記述子経由の実行にある。

`TOCTOU` という語を、静的なディレクトリ権限監査を指す識別子・ログ文言・テスト名から取り除き、本来の TOCTOU 対策と語のうえで区別する。**挙動は変えない**。判定規則・ログレベル・終了コード・エラーの型と包み方・関数の引数と戻り値の意味は、いずれも現状のままとする。

### 1.2 実装方針

0164 §11.5 の方針に従い、`TOCTOU` という語がリポジトリ内の30以上のファイルに現れることを踏まえ、一括置換はしない。0164 §11.2〜§11.4 に列挙された対象だけを、1つずつ確認しながら変更する。判断に迷った箇所は変更せず、レビューで諮る。

0164 §11.6 に従い、コミットを3つに分ける。

1. 識別子の改名のみ（機械的でレビューしやすい単位）
2. ログとエラーの文言、およびそれに依存するテストの更新
3. 文書と CHANGELOG

各コミットで `make test` が緑であること。

### 1.3 事前調査で確認した現状（本書作成時点）

- `resolvePathForCheck`（`internal/security/path_resolution.go:93`）の doc コメントは既に中立な文言（"the directory permission check"）になっており、0164 §11.2 が指摘した `// resolvePathForCheck resolves a path for the TOCTOU permission check.` という文言は現存しない。0164 の本文執筆後にこの箇所自体が改稿されたため、**このステップは対応不要（既に完了）**。
- `cmd/runner/startup_order_guard_test.go` は `runTOCTOUCheck` / `TOCTOU` を一切参照していない（`grep` で確認済み）。0164 §11.4 が「追随の要否を確認する」としていた項目の結論は「不要」。
- `internal/runner` で `toctouValidator` フィールドを持つのは `internal/runner/runner.go`・`internal/runner/group_executor.go`・`internal/runner/group_executor_options.go` の3ファイル（0164 §11.2 の記載どおり）。

---

## 2. 実装ステップ

### コミット1: 識別子の改名

**対象ファイル**:
- `internal/security/toctou.go` → `internal/security/dir_permissions_audit.go`
- `internal/security/toctou_test.go` → `internal/security/dir_permissions_audit_test.go`
- `internal/security/dir_permissions_unix.go`
- `internal/security/dir_permissions.go`（`DirectoryPermChecker` の doc コメント。実装時に追加）
- `cmd/runner/main.go`
- `cmd/runner/main_test.go`（`runTOCTOUCheck` の呼び出し箇所。実装時に追加）
- `cmd/verify/main.go`
- `cmd/record/main.go`
- `internal/runner/runner.go`
- `internal/runner/group_executor_options.go`
- `internal/runner/group_executor.go`
- `internal/runner/group_executor_test.go`（フィールド名・メソッド名・`ErrDirPermViolation` の呼び出し箇所。テスト関数名の改名はコミット2。実装時に追加）

**作業内容**:
- [x] `internal/security`: `RunTOCTOUPermissionCheck` → `AuditDirectoryPermissions`
- [x] `internal/security`: `TOCTOUViolation` → `DirPermViolation`
- [x] `internal/security`: `TOCTOUCheckResult` → `DirPermAuditResult`
- [x] `internal/security`: `toctou.go` を `dir_permissions_audit.go` に、`toctou_test.go` を `dir_permissions_audit_test.go` に `git mv` する
- [x] `internal/security`: `ErrTOCTOUViolation` → `ErrDirPermViolation`
- [x] `internal/security`: `AuditDirectoryPermissions` の doc コメントを書き直し、これが事前の静的な権限監査であって time-of-check/time-of-use のチェックではないこと、本来の TOCTOU 対策（`internal/safefileio` の `O_NOFOLLOW` 経由 open、`internal/runner/base/executor` のファイル記述子経由の実行）は別の場所にあることを英語で明記する。`DirPermAuditResult` の `Checked`・`Skipped` に付いている計数規則の説明（`internal/security/toctou.go:16-51` 付近）は内容を保ったまま移す
- [x] `cmd/runner/main.go`: `runTOCTOUCheck` → `auditConfiguredDirPermissions`
- [x] `internal/runner/runner.go`: `WithTOCTOUValidator` → `WithDirPermAuditor`
- [x] `internal/runner/group_executor_options.go`: `WithGroupTOCTOUValidator` → `WithGroupDirPermAuditor`
- [x] `internal/runner/runner.go`・`internal/runner/group_executor.go`・`internal/runner/group_executor_options.go`: フィールド `toctouValidator` → `dirPermAuditor`
- [x] `internal/runner/group_executor.go`: `runGroupTOCTOUCheck` → `auditGroupDirPermissions`
- [x] 上記改名に追随して `cmd/verify/main.go`・`cmd/record/main.go` の呼び出し箇所を更新する
- [x] `make fmt && make test && make lint` が緑であることを確認する

**成功基準**: 識別子の改名のみで、ログ文言・エラー文言・テスト名は未変更のままビルドとテストが通る。

### コミット2: 文言とテストの更新

**対象ファイル**:
- `internal/security/dir_permissions_audit.go`（WARN ログ）
- `cmd/runner/main.go`（ERROR メッセージ）
- `internal/runner`（グループ側エラー文言）
- `cmd/record/main_test.go`
- `cmd/verify/main_test.go`
- `internal/runner/group_executor_test.go`
- `internal/security/dir_permissions_audit_test.go`（テスト関数名。実装時に追加）
- `cmd/runner/integration_toctou_test.go`（ファイル名含む）

**作業内容**:
- [x] WARN ログ `"TOCTOU permission check violation"` → `"insecure directory permissions"`。`path`・`violation` の属性名と値は変えない
- [x] `cmd/runner` の ERROR メッセージ `"TOCTOU permission check failed: ..."` → `"directory permission audit failed: ..."`
- [x] `internal/runner` のグループ側エラー文言も同じ語に揃える（`ErrDirPermViolation` の文言を `"directory permission audit failed"` に）
- [x] `cmd/record/main_test.go`: `TestRunTOCTOU_*` を `TestCheckDirPermissions_*` に改名する（被テスト関数 `checkDirPermissions` に合わせた）
- [x] `cmd/verify/main_test.go`: `TestRunTOCTOU_ContinuesWhenOnlyTargetDirViolates` を `TestDirPermAudit_ContinuesWhenOnlyTargetDirViolates` に改名する
- [x] `internal/runner/group_executor_test.go`: `TestRunGroupTOCTOUCheck_*` を `TestAuditGroupDirPermissions_*` に改名する
- [x] `internal/security/dir_permissions_audit_test.go`: `TestRunTOCTOUPermissionCheck_*` を `TestAuditDirectoryPermissions_*` に改名する（コミット1 の識別子改名に伴い必要になったため実装時に追加）
- [x] `cmd/runner/integration_toctou_test.go` を `integration_dir_permissions_test.go` に改名し、`TestE2E_TOCTOU_RunnerFailsOnWorldWritableVerifyFilesDir` を `TestE2E_DirPermAudit_RunnerFailsOnWorldWritableVerifyFilesDir` に改名する
- [x] `cmd/verify/main_test.go` の WARN 文言を照合している2箇所（`FindRecords(slog.LevelWarn, ...)` と `assert.Equal(..., warnRecords[0].Message)`。計画時に想定した `assert.Contains` ではなく、実際にはこの2箇所だった）を新しい文言に追随させる
- [x] `cmd/runner/integration_toctou_test.go:74` の `strings.Contains(combined, "TOCTOU") || strings.Contains(combined, "permission") || strings.Contains(combined, "file_access_error")` を置き換える。**このテストは他の項が独立に一致するため、文字列を変え忘れてもテストが通ってしまう**。計画では一時コメントアウトによる確認を予定していたが、**選言そのものを廃し** `assert.Contains(combined, "directory permission audit failed")` の単一条件にした（他の項が独立に一致する構造自体を取り除いたため、以後この確認手順は不要になる）。期待文字列を誤った値に一時変更して当該テストが実際に失敗することを確認済み（`make fmt && make test && make lint` 緑）
- [x] `make fmt && make test && make lint` が緑であることを確認する

**成功基準**: 新しい文言・テスト名でテストが通り、上記の「変え忘れても通ってしまう」テストについては、選言の除去によりその構造自体が解消され、期待文字列を壊すと実際に失敗することを確認済みである。

### コミット3: 文書と CHANGELOG

**作業内容**:
- [x] `CHANGELOG.ja.md` に、ログ文言 `"TOCTOU permission check violation"` → `"insecure directory permissions"` の変更を記載する（運用者が検索の手がかりにしている可能性があるため必須）。判定規則と終了コードは変わらないことを明記する
- [x] `/mktrans` で `CHANGELOG.md` に反映する
- [x] `make fmt && make test && make lint` が緑であることを確認する

**成功基準**: 0164 §11.6 の完了条件をすべて満たす。

---

## 3. 完了条件（0164 §11.6 準拠）

- [x] `make fmt` → `make test` → `make lint` がすべて緑
- [x] `rg -n 'TOCTOU' --type go` の残存が本来の TOCTOU 対策（`internal/safefileio`・`internal/runner/base/executor`・`internal/filevalidator`・`internal/dynlib`・`internal/shebang`・`internal/dynamicanalysis`・`internal/security/elfanalyzer`・`internal/security/binaryanalyzer`・`internal/fileanalysis`・`internal/verification` など）を指すものだけになっており、残した各ファイルについて理由を1行で説明できる → §3.1 に一覧
- [x] 改名の前後で `go tool cover -func` を比較し、関数単位で低下がないこと（挙動を変えていないので一致するはず。関数名が変わるため対応をとって比較する）→ 改名前（`HEAD~2` の worktree）と改名後の `go tool cover -func` を、行番号を除去し改名対応表（`RunTOCTOUPermissionCheck`→`AuditDirectoryPermissions` 等）を適用したうえで比較。**関数数 1803 で完全一致**（差分ゼロ）

### 3.1 残存する `TOCTOU` の理由

いずれも check 時と use 時の観測がずれうる本来の race を指しており、本タスクの対象外。

| ファイル群 | 理由 |
|---|---|
| `internal/safefileio/{errors,safe_file,safe_file_linux,safe_file_nonlinux}.go` | `O_NOFOLLOW` 付き open による symlink 差し替え race の防止そのもの |
| `internal/runner/base/executor/{executor,fdexec_linux}.go` | 検証済みファイル記述子経由の実行によるパス再解決 race の防止そのもの |
| `internal/runner/group_executor.go`（397行付近）・`internal/runner/group_executor_test.go`（1304・1370行付近）・`internal/runner/bootstrap/config.go` | 上記 fd 束縛実行の説明。実行直前の再解決を廃して race を閉じた経緯を記述 |
| `internal/filevalidator/validator.go` とそのテスト・`internal/verification/{manager,path_resolver}.go`・`internal/runner/config/loader_verification_test.go`・`internal/runner/e2e_dynlib_verification_test.go` | ハッシュ再検証による同一性保証（検証したファイルと使うファイルが同一であることの担保） |
| `internal/dynlib/*`・`internal/dynamicanalysis/store.go` とそのテスト・`internal/fileanalysis/file_analysis_store.go`・`internal/shebang/parser.go`・`internal/security/{elfanalyzer,binaryanalyzer}/*` | 解析対象ファイルを safefileio 経由で開く理由の説明（同一 open での読み取りによる race 回避） |
| `internal/runner/base/security/{indirect_execution,validator}.go` | 間接実行の残存リスクとして TOCTOU を明示する脅威モデルの記述 |
| `test/security/{hash_bypass,output_security}_test.go` | 上記 race に対する防御を対象としたセキュリティテスト |

## 4. Acceptance Criteria Verification

本タスクは挙動を変えない改名のみであり、新規の外部挙動に対する AC は設定しない。検証は「識別子・文言が新しい名前に揃っていること」自体を対象とする静的検証で行う。

| 検証項目 | 検証方法 |
|---|---|
| `TOCTOU` を含む識別子が `internal/security`・`internal/runner`・`cmd/runner`・`cmd/verify`・`cmd/record` から消えている | `rg -n 'TOCTOU' --type go` の結果を目視確認（**済**。残存は §3.1 のとおり本来の TOCTOU 対策のみ） |
| WARN ログ文言が新文言に変わっている | `cmd/verify/main_test.go` の `FindRecords(slog.LevelWarn, "insecure directory permissions")` と `assert.Equal(..., warnRecords[0].Message)` が新文言で通る（**済**。計画時に想定した `assert.Contains` ではなく、実際にはこの2箇所だった） |
| `cmd/runner/integration_dir_permissions_test.go` が新文言を正しく検出できている | 選言を廃して `assert.Contains(combined, "directory permission audit failed")` の単一条件にしたうえで、期待文字列を誤った値へ一時変更し、当該テストが実際に失敗することを確認（**済**。PR に記載する） |
| 挙動（判定規則・ログレベル・終了コード・エラー型）が変わっていない | 改名前後で `go test -tags test ./...` と `go tool cover -func` の関数単位カバレッジを比較（**済**。関数数 1803 で完全一致。手順は §3 完了条件を参照） |

## 5. Next Steps

- 完了後、0164 §10 で登録した issue [#1033](https://github.com/isseis/go-safe-cmd-runner/issues/1033) をクローズする。
