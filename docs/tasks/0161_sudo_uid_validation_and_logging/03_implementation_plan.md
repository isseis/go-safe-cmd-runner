# 実装計画書: `SUDO_UID` の実在確認と採用事実の記録

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-30 |
| Review date | 2026-07-30 |
| Reviewer | Issei Suzuki |
| Comments | - |

## 1. 実装概要

### 1.1 目的

[`02_architecture.md`](02_architecture.md)（status: `approved`）が定めた設計を実装し、[`01_requirements.md`](01_requirements.md) の AC-01〜AC-19 をすべて満たす。実装対象は次の2点である。

1. 基準UID決定方針が `SudoUIDAware` のときに `SUDO_UID` を採用する直前に実在確認を行い、確認できない場合は基準UIDを返さずエラーとする（フェイルクローズド）。
2. 採用によって基準UIDが実 UID と異なる値になった事実を、プロセスごとに1回だけ `log/slog` へ警告として記録する。

### 1.2 実装方針

- 変更するコードは `internal/groupmembership` 内に閉じる。`cmd/record` / `cmd/verify` / `cmd/runner` / `internal/safefileio` の本番コードは変更しない。
- 既存の公開 API のシグネチャは変更しない。例外は `//go:build test` のテスト用ラッパー `ResolvePermissionCheckUID` であり、これは本番バイナリに含まれない。
- 新たに公開するのはセンチネルエラー2つ（`ErrSudoUIDUserNotFound` / `ErrSudoUIDUserLookupFailed`）と、テスト用ラッパーが取る構造体 `PermissionCheckUIDDeps` のみである。変更する非公開の要素は `resolvePermissionCheckUID`、`getPermissionCheckUID`、`New`、`GroupMembership` 構造体、および `policy.go` の `SudoUIDAware` のコメントである。
- 設計の根拠・判断理由は 02_architecture.md に記載済みのため、本書では再掲せず節番号で参照する。
- Go のコメント・識別子・文字列リテラルはすべて英語で書く。

### 1.3 既存コード調査結果

実装前に対象パッケージと呼び出し元を調査した結果を示す。

#### 変更の起点となる既存コード

| 対象 | 現状 | 本タスクでの扱い |
|---|---|---|
| `internal/groupmembership/manager.go` の `resolvePermissionCheckUID`（485行目） | `getenv func(string) string` を第3引数に取り、`parseSudoUID` の結果をそのまま返す | 第3引数を `permissionCheckUIDDeps` へ置き換え、実在確認と記録を追加する（02 §3.1、§6.1） |
| 同 `getPermissionCheckUID`（457行目） | `resolvePermissionCheckUID(..., os.Getenv)` を呼ぶ | 本番依存3つを束ねて渡す形へ変更する（02 §3.1、§3.3、§3.4） |
| 同 `parseSudoUID`（505行目） | 数値妥当性のみを検査 | 変更しない。実在確認はこの関数の後段に置く |
| 同 `getProcessRealUID`（532行目） | `os.Getuid()` を直接呼ぶ | 変更しない（02 §3.1、§7.6） |
| 同 `New`（92行目） | `membershipCache` を `make` で初期化する | `sudoUIDExistence.confirmed` の `make` を1行追加する（02 §3.4） |
| 同 `GroupMembership` 構造体（66-82行目） | `membershipCache` / `cacheMutex` / `cleanupCounter` / `enumerateGroupMembers` / `policy` を持つ | `sudoUIDExistence sudoUIDExistenceMemo` フィールドを追加する |
| 同 `ErrSudoUIDOutOfRange`（49行目） | 定義済み | 変更しない。新エラー2つを同じファイルに隣接して追加する（02 §2.2） |
| `internal/groupmembership/policy.go` の `SudoUIDAware`（24-31行目） | 「`SUDO_UID` は数値としての妥当性しか検査していない」と述べるコメントを持つ | コメントを実在確認込みの内容へ書き換える（本書 2 章のステップ4-2） |
| `internal/groupmembership/membership_cgo.go` / `membership_nocgo.go` | ビルドタグ `cgo` / `!cgo` で切り替わる対のファイル。`membership_cgo.go` 側に `maxGroupMembers` 等のパッケージ定数がある | ユーザーデータベース種別の定数を両方に追加する（02 §3.7.5） |
| `internal/groupmembership/test_helpers_policy.go` の `ResolvePermissionCheckUID`（54行目） | `getenv` を位置引数に取る | 公開の依存構造体を取る形へ変更する（02 §7.3） |

#### 再利用する既存資産

- `user.LookupId` の呼び出し方法は同一パッケージ内の `IsUserInGroup`（`manager.go:148`）と `isUserOnlyGroupMember`（`manager.go:187`）に前例がある。ただし両者は戻り値の `Username` / `Gid` を使うため、差し替え口（テストから実装を差し替えられるようにするための引数）は共有できない（02 §3.2）。共通化は行わない。
- `errors.AsType` を値型のエラーに適用する用法は `internal/runner/resource/dryrun_manager.go:386` に前例がある（`user.UnknownUserError` に適用）。同じ形で `user.UnknownUserIdError` に適用する。
- プロセス全体の状態を `atomic` で持つ形は `policy.go:67` の `processPermissionCheckUIDPolicy` に前例がある。パッケージレベルのレポータ実体はこれに倣った命名とする。
- テストから `t.Cleanup` で復元するスワップ関数の形は `test_helpers_policy.go:36` の `SwapProcessPermissionCheckUIDPolicy` に前例がある。ただし本タスクではレポータ実体を書き換えないため、同種の関数は追加しない（02 §3.3）。
- `//go:build test` の非 `_test.go` ヘルパーは `test_helpers.go` と `test_helpers_policy.go` の2つが既にある。本タスクで新規のヘルパーファイルは追加しない（本書 7 章）。

#### `slog` 出力を捕捉するテスト用ハンドラ

パッケージ外に再利用できるものは存在しない。`internal/runner/runner_test.go:2407` の `logCaptureHandler` は `internal/runner` パッケージ内の `_test.go` にあり、`internal/groupmembership` からは参照できない。`internal/logging` にも公開の捕捉ハンドラはない。したがって `internal/groupmembership` のテスト内に捕捉ハンドラを新規に定義する。テスト専用かつパッケージ内のテストからのみ使うため、`_test.go` の中に置き、`test_helpers_*.go` は追加しない。

#### 挙動が変わる既存テスト（重要）

`internal/groupmembership/manager_test.go:610` の `TestGetPermissionCheckUID` のサブテスト `"reads SUDO_UID from the real environment under SudoUIDAware"` は、`t.Setenv("SUDO_UID", "9999")` を置いたうえで **実 UID が 0 のとき `9999` が返り、エラーにならないこと** を期待している。実在確認の追加後、UID `9999` に対応するユーザーが実在しない環境では `ErrSudoUIDUserNotFound` が返るため、root でこのテストを実行すると失敗する。本書 2 章のステップ3-7 でこのサブテストの書き換えを明示のタスクとして扱う。CI（`ubuntu-latest`、非 root）ではこの分岐に入らないため、CI だけでは検出できない。

#### 記述が偽になる既存文書

`rg -n "SUDO_UID"` および `rg -n "numeric validity|not verified to|数値としての妥当性"` をリポジトリ全体（`docs/tasks/` を除く）に対して実行し、次を確認した。

| 箇所 | 状態 | 扱い |
|---|---|---|
| `internal/groupmembership/policy.go:27` | 「`SUDO_UID` は数値としての妥当性しか検査していない」旨。本タスクで偽になる | 書き換える（本書 2 章のステップ4-2） |
| `docs/dev/architecture_design/security-architecture.ja.md:50` / `security-architecture.md:50` | 基準UID解決規則を「範囲内の数値 UID であればその値を採用」と述べる。本タスクで不正確になる | 書き換える（本書 2 章のステップ4-3、AC-17） |
| `cmd/record/main.go:29`、`cmd/verify/main.go:32`、`cmd/runner/main.go:84` の `init()` コメント | 方針の宣言理由のみを述べる | 変更不要 |
| `docs/user/runner_command.ja.md:1856` / `runner_command.md` の該当箇所 | `runner` が `SUDO_UID` を見ないことのみを述べる | 変更不要 |
| `cmd/record/main_test.go:417`、`cmd/verify/main_test.go:187` の「`matching the pre-refactor resolvePermissionCheckUID(0, "1000")` behavior」というコメント | 変更後のシグネチャと食い違う | 本書 2 章のステップ2-5 でコメントも書き換える |
| `CHANGELOG.md` の `[Unreleased]` | 0160 の変更を記載済み。本タスクの挙動変更は未記載 | 追記する（本書 2 章のステップ4-4） |
| `docs/translation_glossary.md` の `### B` 節（`base UID` として基準UID / 基準UID決定方針を収録。47-48行目） | 本タスクの新用語は未収録 | 追記する（本書 2 章のステップ4-1、AC-19） |

#### 外部前提の確認結果

| 前提 | 確認方法と結果 |
|---|---|
| `os/user` は CGO 版・非 CGO 版のいずれも「見つからない」を `user.UnknownUserIdError` で返す | `$(go env GOROOT)/src/os/user/cgo_lookup_unix.go:62` と `lookup_unix.go:188` の双方で `return nil, UnknownUserIdError(...)` を確認（Go 1.26.3） |
| CGO 版は照会失敗時に内側の `errno` を `%v` で平坦化する | `cgo_lookup_unix.go:64` の `fmt.Errorf("user: lookup userid %d: %v", uid, err)` を確認。02 §4.1 の記述どおり |
| `errors.AsType` が利用可能である | `go.mod` の宣言は `go 1.26.2`。`internal/runner/resource/dryrun_manager.go:386` に使用実績あり |
| `make test` が CGO 有効・無効の両方を実行する | `Makefile:454-460` の `unit-test` が `CGO_ENABLED=1`（`-race` 付き、455行目）と `CGO_ENABLED=0`（457行目）の2回 `go test -tags test ./...` を実行することを確認。したがって両ビルドタグ付きファイルの型エラーは `make test` で検出できる。ただし `CGO_ENABLED=0` の回は macOS では実行されない（456-460行目の `uname -s` 判定）ため、両ビルドの検証は Linux で行う |
| `//go:build test` の非 `_test.go` ファイルがテストビルドに含まれる | 同上（`-tags test` が付く） |
| CI がテストを非 root で実行する | `.github/workflows/ci.yml` は `runs-on: ubuntu-latest` でコンテナを使わない。`id -u` は 0 ではない |

## 2. 実装ステップ

フェーズ区分と順序は 02 §8 の実装優先順位に従う。

### フェーズ1: 独立した部品の追加

既存の挙動を変えない追加のみを行う。このフェーズ単独で `make test` が通る。

#### ステップ1-1: ユーザーデータベース種別の定数

**変更ファイル**: `internal/groupmembership/membership_cgo.go`、`internal/groupmembership/membership_nocgo.go`、`internal/groupmembership/membership_cgo_test.go`、`internal/groupmembership/membership_nocgo_test.go`

- [x] `membership_cgo.go` に `const userDatabaseSource = "nss"` を、参照先が NSS であることを述べる英語のコメント付きで追加する
- [x] `membership_nocgo.go` に `const userDatabaseSource = "passwd-file"` を、参照先が `/etc/passwd` のみであることを述べる英語のコメント付きで追加する
- [x] `membership_cgo_test.go`（`//go:build cgo`）に `TestUserDatabaseSource` を追加し、値が `"nss"` であることを検証する
- [x] `membership_nocgo_test.go`（`//go:build !cgo`）に `TestUserDatabaseSource` を追加し、値が `"passwd-file"` であることを検証する。このテストの目的は、定数の値を固定して不注意な書き換えを防ぐことにある。当該ビルドが実際にどのユーザーデータベースを参照するかを証明するものではない。この趣旨は、CGO 版のテストにも英語のコメントとして書く

**完了条件**: `make test` が CGO 有効・無効の両方で通る（`Makefile:454-460` の `unit-test` が両方を実行する）。ただし macOS では CGO 無効の回が実行されない（`Makefile:456-460`）ため、両ビルドの検証は Linux で行う。

#### ステップ1-2: センチネルエラー2つ

**変更ファイル**: `internal/groupmembership/manager.go`

- [x] `ErrSudoUIDOutOfRange`（49行目）の直後に `ErrSudoUIDUserNotFound` を 02 §4.1 のメッセージ `"SUDO_UID does not refer to an existing user"` で追加する
- [x] 同じ位置に `ErrSudoUIDUserLookupFailed` を 02 §4.1 のメッセージ `"failed to verify that SUDO_UID refers to an existing user"` で追加する

**完了条件**: `go build ./...` と `make lint` が通る（この時点では未使用の公開変数だが、公開識別子なので未使用検出の対象外）。

#### ステップ1-3: 採用事実レポータ

**変更ファイル**: `internal/groupmembership/manager.go`

- [x] `sudoUIDAdoptionReporter` 構造体（`reported atomic.Bool`）を 02 §3.3 のドキュメントコメント付きで追加する
- [x] `report(logger *slog.Logger, policy PermissionCheckUIDPolicy, realUID, permissionCheckUID int)` メソッドを追加する。`reported.CompareAndSwap(false, true)` が成功したときのみ出力し、戻り値は持たない
- [x] 出力内容を 02 §4.3「採用事実の記録」の表どおりに実装する。レベルは `slog.LevelWarn` とし、メッセージは同表の1文をそのまま用いる。属性は次の5つとする。`permission_check_uid`、`real_uid`、`source_env_var`（値は定数 `sudoUIDEnvVar`）、`permission_check_uid_policy`（値は `policy.String()`）、`user_database_source`（値は定数 `userDatabaseSource`）
- [x] パッケージレベルの実体 `processSudoUIDAdoptionReporter sudoUIDAdoptionReporter` を追加し、プロセス全体で共有される唯一の実体であることをコメントで述べる
- [x] `import` に `log/slog` と `sync/atomic` を追加する

**完了条件**: ステップ1-5 のテストが通る。

#### ステップ1-4: 実在確認のメモ

**変更ファイル**: `internal/groupmembership/manager.go`

- [x] `sudoUIDExistenceMemo` 構造体（`mu sync.Mutex`、`confirmed map[int]struct{}`）を 02 §3.4 のドキュメントコメント付きで追加する
- [x] `verify(uid int, lookup func(uid int) error) error` メソッドを追加する。`confirmed` に `uid` があれば `nil` を返す。なければ `lookup(uid)` を呼び、`lookup` が `nil` を返したときだけ `uid` を `confirmed` へ登録する。失敗は登録しない
- [x] `GroupMembership` 構造体に `sudoUIDExistence sudoUIDExistenceMemo` フィールドを追加する
- [x] `New`（92行目）の構造体リテラルに `sudoUIDExistence: sudoUIDExistenceMemo{confirmed: make(map[int]struct{})}` を追加する

> **02 §8 との差異**: 02 §8 は `GroupMembership` へのフィールド追加と `New` の初期化をフェーズ2に置いている。本計画では上の2項目をフェーズ1へ前倒しした。ステップ1-5 の `TestNewInitializesSudoUIDExistenceMemo` が `New()` 経由の初期化を検証対象とするためである。フェーズの順序と各フェーズの目的は 02 §8 のままであり、前後関係は変わらない。

**完了条件**: ステップ1-5 のテストが通る。

#### ステップ1-5: フェーズ1の単体テスト

**変更ファイル**: `internal/groupmembership/membership_common_test.go`（捕捉ハンドラ）、`internal/groupmembership/manager_test.go`（テスト本体）

- [x] `slog.Handler` を実装する捕捉ハンドラを `membership_common_test.go` へ追加する。捕捉するのは、レベルとメッセージ、および属性名から値へのマップの3つとする。あわせて、`Handle` が固定のエラーを返すモードも持たせる（ステップ3-5 のテストで使う）。`manager_test.go` と `policy_test.go` の双方から使うため、パッケージ内でテストファイルをまたいで共有される既存のヘルパー置き場である `membership_common_test.go` に置く（ビルドタグなしの `_test.go` であり、`test_organization.md` が対象とする `//go:build test` のヘルパーファイルには当たらない）
- [x] 捕捉ハンドラは `Handle` が複数の goroutine から同時に呼ばれうるため、捕捉したレコードのスライスを `sync.Mutex` で保護し、ロックを取ってコピーを返す取得メソッドを設ける。並行テスト（下記2件）で `-race` の指摘が出ないようにする
- [x] `TestSudoUIDAdoptionReporter_Report` を追加する。`realUID` と `permissionCheckUID` に異なる値（0 と 1000）を与え、レベルが `slog.LevelWarn` であること、メッセージが 02 §4.3 の文言と完全一致すること、02 §4.3 の5属性がすべて存在し値が一致することを検証する
- [x] `TestSudoUIDAdoptionReporter_ReportsOnlyOnce` を追加する。同一実体に対して `report` を3回呼び、捕捉されたレコードが1件であることを検証する
- [x] `TestSudoUIDAdoptionReporter_ReportsOnlyOnceConcurrently` を追加する。50 goroutine から同時に `report` を呼び、捕捉されたレコードが1件であることを検証する
- [x] `TestSudoUIDExistenceMemo_ReusesConfirmation` を追加する。同じ UID を3回 `verify` し、`lookup` の呼び出し回数が1であることを検証する
- [x] `TestSudoUIDExistenceMemo_DoesNotRememberFailures` を追加する。`lookup` が1回目と2回目にエラーを返し、3回目に成功する場合を対象とする。`verify` を4回呼び、次の2点を検証する。1〜3回目の `verify` では毎回 `lookup` が呼ばれること（呼び出し回数 3）。3回目に成功したあとは、4回目の `verify` で `lookup` が呼ばれないこと（呼び出し回数が 3 のまま）
- [x] `TestSudoUIDExistenceMemo_Concurrent` を追加する。50 goroutine から同時に、成功する UID と失敗する UID を混ぜて `verify` する。`-race` で競合が検出されないこと、および goroutine の合流後に、成功した UID への `verify` が `lookup` を呼ばずに `nil` を返し、失敗した UID への `verify` が `lookup` を再度呼ぶことを検証する
- [x] `TestNewInitializesSudoUIDExistenceMemo` を追加する。`New()` で生成したインスタンスに対して `gm.sudoUIDExistence.verify(1000, func(int) error { return nil })` を2回呼び、パニックせず、2回目で `lookup` が呼ばれないことを検証する（`confirmed` マップの初期化漏れによる nil マップへの書き込みを検出する）
- [x] `TestProcessSudoUIDAdoptionReporterIsProcessWide` を追加する。パッケージレベルの実体 `processSudoUIDAdoptionReporter` が `sudoUIDAdoptionReporter` 型であることを検証し、その1回制限の契約（テストは報告に使わないこと、実体の一意性は PR 時の static チェックが担うこと）をドキュメントコメントで述べる。このテストは `make lint` の `unused` がパッケージレベルの実体を未使用と判定するのを防ぐために置く（実体はフェーズ2のステップ2-2 で初めて参照される）
- [x] `TestSudoUIDExistenceErrorMessages` を追加する。ステップ1-2 で定義した2つのセンチネルエラーの `.Error()` 文字列をリテラルで固定し（AC-18 が利用者向け文書でこの文言を逐語引用するため）、相互に `errors.Is` で一致しないことを検証する（AC-03 の型の定義部分をこのフェーズで検証する）
- [x] 上記9テストはプロセス全体の状態に触れないため、すべてに `t.Parallel()` を付ける

**完了条件**: `make test` が通る（`Makefile:454-460` の `unit-test` は CGO 有効の回で `-race` 付きで実行する）。Linux 以外では CGO 無効の回が実行されないため（`Makefile:456-460`）、CGO 無効側の検証は Linux で行う。

> **計画外の変更（`Makefile` の `lint` ターゲット）**: 本計画は各フェーズの完了条件で `make lint` が CGO 有効・無効の両方を通ることを求めているが、`lint` ターゲットは `--build-tags test` のみを渡し `CGO_ENABLED` は既定（=1）のままだったため、`//go:build !cgo` のファイル（`membership_nocgo.go` / `membership_nocgo_test.go`）は実際には一度も検査されていなかった。ステップ1-1 が両ファイルへコードを追加する以上、この完了条件が空手形になるため、`lint` を `unit-test` と同じ形（CGO 有効の回に続けて、Linux でのみ CGO 無効の回）に揃えた。これにより顕在化した既存の `revive` 指摘2件（`membership_nocgo_test.go` の未使用引数 `gid`）も併せて修正した。

### PR-1 作成ポイント: standalone building blocks

**対象ステップ**: 1-1 / 1-2 / 1-3 / 1-4 / 1-5

**推奨タイトル**: `feat(0161): add sentinel errors, adoption reporter and existence memo`

**レビュー観点**: `sudoUIDAdoptionReporter` の1回制限と `sudoUIDExistenceMemo` の排他制御が `-race` 付きで検証されているか / `New` が `confirmed` マップを初期化し、`TestNewInitializesSudoUIDExistenceMemo` が nil マップ書き込みを実際に踏む経路を通るか / `userDatabaseSource` が `membership_cgo.go` と `membership_nocgo.go` の双方に追加され、CGO 有効・無効の両ビルドで `make test` が通るか / 失敗した実在確認がメモに登録されないこと（`TestSudoUIDExistenceMemo_DoesNotRememberFailures`）

**実装モデル要件**: frontier-recommended

**判定理由**: ステップ1-3・1-4 が `atomic.Bool` による1回制限と `sync.Mutex` で保護した共有メモという並行処理を新規に導入する、独立した高リスク・複雑ステップに当たる（誤りは `-race` なしでは表面化せず、02 §3.4 の nil マップ初期化漏れも型では検出できない）。同居するステップ1-1・1-2 は定数2つとセンチネル2つの追加に留まるが、行数が小さく（合計10行未満）独立した PR に切り出す利得がレビュー1回分の手間を下回るため、この PR に含めたままとする。レビューの重心はステップ1-3〜1-5 に置く。

- [x] `rg -n "userDatabaseSource" --glob '*.go'` の結果が `internal/groupmembership/` 内に限られること（10 章。他パッケージの同名識別子との衝突確認）
- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### フェーズ2: 依存の束への移行（挙動は変えない）

外部依存を1つの構造体（以下「依存の束」。実体は `permissionCheckUIDDeps`）にまとめる変更と、それに追随する既存テストの移行を同一フェーズで行う。02 §8 のとおり、両者を別フェーズに分けると、途中の状態でパッケージがコンパイルできなくなるためである。

#### ステップ2-1: 依存の束と既定実装

**変更ファイル**: `internal/groupmembership/manager.go`

- [x] `permissionCheckUIDDeps` 構造体（`getenv` / `verifyUserExists` / `reportAdoption` の3フィールド）を 02 §3.1 のドキュメントコメント付きで追加する
- [x] `lookupUserByUID(uid int) error` を追加する。`user.LookupId(strconv.Itoa(uid))` を呼び、エラーを加工せずそのまま返す。エラー分類を行わない理由を英語のコメントで述べる（02 §3.2）
- [x] `resolvePermissionCheckUID` の第3引数を `getenv func(string) string` から `deps permissionCheckUIDDeps` へ変更し、内部の `getenv(sudoUIDEnvVar)` を `deps.getenv(sudoUIDEnvVar)` へ置き換える。この時点では `verifyUserExists` と `reportAdoption` は呼ばない
- [x] `resolvePermissionCheckUID` のドキュメントコメント（465-484行目）の `Parameters` 節を `deps` の説明へ差し替える。返しうるエラーの記述はフェーズ3で更新する

**完了条件**: `go build ./...` が通る（テストの移行前なのでテストはまだコンパイルできない）。

#### ステップ2-2: 本番依存の組み立て

**変更ファイル**: `internal/groupmembership/manager.go`

- [x] `getPermissionCheckUID`（457行目）で `permissionCheckUIDDeps` を組み立てて渡す形へ変更する。`getenv` は `os.Getenv`、`verifyUserExists` は `gm.sudoUIDExistence.verify(uid, lookupUserByUID)` を呼ぶクロージャ、`reportAdoption` は `processSudoUIDAdoptionReporter.report(slog.Default(), ...)` を呼ぶクロージャとする
- [x] `slog.Default()` をクロージャの内側で呼ぶ（束ねる時点ではなく記録の時点で解決する。02 §3.3、AC-11）
- [x] ステップ3-6 のテストがそのまま複製できるよう、`reportAdoption` は途中に中間変数を挟まず、ひとつづきの式として書く

**完了条件**: `go build ./...` が通る。

#### ステップ2-3: テスト用ラッパーの更新

**変更ファイル**: `internal/groupmembership/test_helpers_policy.go`

- [x] 公開構造体 `PermissionCheckUIDDeps`（`Getenv` / `VerifyUserExists` / `ReportAdoption` の3フィールド）を追加する
- [x] `ResolvePermissionCheckUID` のシグネチャを `(policy PermissionCheckUIDPolicy, realUID int, deps PermissionCheckUIDDeps) (int, error)` へ変更する
- [x] `Getenv` または `VerifyUserExists` が `nil` の場合は `panic` する。文言は既存の `WithPermissionCheckUIDPolicy`（20-22行目）の `panic` に倣い `groupmembership: ` 接頭辞を付ける
- [x] `ReportAdoption` が `nil` の場合は、呼び出しごとに新しい `sudoUIDAdoptionReporter` を生成して `slog.Default()` へ出力する実装を、既定値として補う
- [x] ドキュメントコメントに、呼び出しごとに新しいレポータ実体を使うため1回制限の検証には使えないこと（02 §7.3）を英語で明記する

**完了条件**: `go vet -tags test ./internal/groupmembership/` が通る。

#### ステップ2-4: パッケージ内の既存テストの移行

**変更ファイル**: `internal/groupmembership/policy_test.go`

- [x] `TestResolvePermissionCheckUID_RealUIDOnly`（152行目）を `permissionCheckUIDDeps` を渡す形へ移行する。既存の `sudoUIDValues` 8種 × `realUID` 2種の網羅は維持する。02 §7.5 が求める「実在確認の呼び出し回数の検証」もこのテストへ追加し、全組み合わせで `verifyUserExists` と `reportAdoption` の呼び出し回数がいずれも 0 であることを検証する
- [x] `TestResolvePermissionCheckUID_SudoUIDAware`（174行目）を同様に移行する。行の追加はフェーズ3で行う
- [x] `TestResolvePermissionCheckUID_EnvAccess`（222行目）を同様に移行する
- [x] 3テストで使う `permissionCheckUIDDeps` を組み立てるテストヘルパー関数を `policy_test.go` 内に1つ用意し、`verifyUserExists` の既定を「常に `nil`（実在する）」、`reportAdoption` の既定を「呼び出しを数えるだけの関数」とする。各テストは必要なフィールドのみ上書きする。ヘルパー名・引数名・コメントはすべて英語で書く
- [x] `TestResolvePermissionCheckUID_SudoUIDAware` のドキュメントコメント（`policy_test.go:169-173`）と `TestResolvePermissionCheckUID_EnvAccess` のドキュメントコメント（`policy_test.go:218-221`）が参照している節番号は 0160 のもの（それぞれ `§3.4.2`、`§3.5`）である。本タスクの決定表と対比要件を指すよう、`0161 §3.5`（決定表）と `0161 §7.1`（対比要件）へ書き換える

**完了条件**: `go test -tags test ./internal/groupmembership/` が通り、上記3テストの検証内容が移行前より弱まっていないこと（表の行数と網羅の組み合わせ数が減っていないこと）。

> **計画外の変更（ステップ2-4 で追加した3点）**: PR-2 のレビュー指摘への対応として、上の項目に挙げていない次の3点を同じステップで行った。
>
> 1. `TestResolvePermissionCheckUID_PanicsOnNilDeps` を新規追加した。ステップ2-3 で `ResolvePermissionCheckUID` に `Getenv` / `VerifyUserExists` の nil パニックを入れたが、その契約を固定するテストがどのステップにも無かったためである（8 章の AC-16 に行を追加した）。
> 2. `TestResolvePermissionCheckUID_EnvAccess` の `SudoUIDAware` 側の読み取り回数の検証を `assert.GreaterOrEqual(t, calls, 1)` から `assert.Equal(t, 1, calls)` へ強めた。移行前の緩い表明では、フェーズ3 で `getenv` の呼び出しが増えても検出できないためである。完了条件が禁じているのは検証内容が弱まることであり、強める方向のため完了条件には抵触しない。
> 3. テストヘルパー `newPermissionCheckUIDDeps` の `getenv` にも既定値（常に空文字列＝`SUDO_UID` 未設定）を与えた。当初は「呼び出し側が必ず上書きする」前提で nil のままにしていたが、上書きを忘れた場合に `RealUIDOnly` では素通りし `SudoUIDAware` でだけ nil 関数のパニックになるため、既定値を置いて未設定として解決されるようにした。

#### ステップ2-5: `cmd/*` の既存テストの移行

**変更ファイル**: `cmd/record/main_test.go`、`cmd/verify/main_test.go`、`cmd/runner/main_test.go`

- [x] `TestRecordDeclaresSudoUIDAwarePolicy`（`cmd/record/main_test.go:420`）の `ResolvePermissionCheckUID` 呼び出しを `groupmembership.PermissionCheckUIDDeps{Getenv: func(string) string { return "1000" }, VerifyUserExists: func(int) error { return nil }}` を渡す形へ変更する
- [x] `TestVerifyDeclaresSudoUIDAwarePolicy`（`cmd/verify/main_test.go:190`）を同様に変更する
- [x] `TestRunnerDeclaresRealUIDOnlyPolicy`（`cmd/runner/main_test.go:349`）を同様に変更する。`RealUIDOnly` では `VerifyUserExists` が呼ばれないことを、呼ばれたら `t.Error` を出すクロージャで検証する（AC-12）
- [x] `cmd/record/main_test.go:417` と `cmd/verify/main_test.go:187` の `matching the pre-refactor resolvePermissionCheckUID(0, "1000") behavior` というコメント文を、新シグネチャに合わせた表現へ書き換える

**完了条件**: `go test -tags test ./cmd/...` が通り、3テストの検証内容（宣言された方針の確認と採用の可否）が移行前と同じであること。

**フェーズ2の完了条件**:
- `make fmt` → `make test` → `make lint` がすべて通る。`make test` は `//go:build test` 付きの `test_helpers_policy.go` を含めて CGO 有効・無効の両方でコンパイルする
- 挙動は 0160 完了時点と同一である（実在確認も記録もまだ呼ばれない）

### PR-2 作成ポイント: dependency bundle refactoring

**対象ステップ**: 2-1 / 2-2 / 2-3 / 2-4 / 2-5

**推奨タイトル**: `refactor(0161): pass permission check UID dependencies as a struct`

**レビュー観点**: 挙動が 0160 完了時点と同一で、`verifyUserExists` と `reportAdoption` がまだ一度も呼ばれないか / 移行した既存3テスト（`policy_test.go`）と `cmd/*` の3テストの検証内容が移行前より弱まっていないか（表の行数と網羅の組み合わせ数） / `getPermissionCheckUID` が `slog.Default()` をクロージャの内側で解決しているか（束ねる時点ではない） / `//go:build test` の `test_helpers_policy.go` を含めて CGO 有効・無効の両方でコンパイルされるか

**実装モデル要件**: standard

**判定理由**: 差し替え口の導入に伴う機械的なシグネチャ移行で挙動を変えず、`既存コード調査結果` にも競合する実装案は挙がっていない。該当する Conditional check はビルドタグ下のコンパイル1件のみで、パネルモードの契機にも当たらない。5.1 節が挙げる「ステップ2-1〜2-5 を同一コミット群に置かないとコンパイルできない」という制約は、フェーズの順序が 02 §8 と食い違っているという意味ではなく、PR 内部の作業順の話であるため、Conditional check の「フェーズ名・順序が承認済みアーキテクチャの実装優先順位と一致するか」には当たらない。またこの制約に反した場合の帰結はコンパイルエラーであり、グリーンゲートが確実に検出する。

- [x] `rg -n "getenv func\(string\) string" internal/groupmembership/` の結果が空であること（10 章。旧シグネチャの残存確認）
- [x] `rg -n "pre-refactor resolvePermissionCheckUID" cmd/` の結果が空であること（10 章。ステップ2-5 のコメント書き換え漏れの確認）
- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### フェーズ3: 実在確認と記録の組み込み

#### ステップ3-1: 実在確認とエラー分類

**変更ファイル**: `internal/groupmembership/manager.go`

- [x] `resolvePermissionCheckUID` の `parseSudoUID` 成功後に `deps.verifyUserExists(parsedUID)` を呼ぶ処理を追加する（02 §6.1 の順序: 数値妥当性の検査 → 実在確認 → 記録）
- [x] `errors.AsType[user.UnknownUserIdError](err)` が真のとき、次の書式でエラーを返す。既存の `parseSudoUID`（`manager.go:511`）が採る「本文のあとにセンチネルを `%w` で置く」書式に揃え、センチネルは本文の後ろ、元のエラーの直前に置く。
  `fmt.Errorf("SUDO_UID %s does not exist in the user database (user_database_source=%s); check whether SUDO_UID is a stale value inherited from the environment, then re-run from an interactive sudo session: %w: %w", sudoUID, userDatabaseSource, ErrSudoUIDUserNotFound, err)`
- [x] それ以外のエラーのとき、次の書式でエラーを返す。
  `fmt.Errorf("could not verify SUDO_UID %s against the user database (user_database_source=%s); check the state of the user database, then re-run: %w: %w", sudoUID, userDatabaseSource, ErrSudoUIDUserLookupFailed, err)`
- [x] `sudoUID`（環境変数の生の文字列）だけを出力し、`parsedUID` は出力しない。両者は同じ値を表すため重複になる
- [x] いずれのエラーでも基準UIDは返さず `0` と非 nil エラーを返す

**完了条件**: `go build ./...` が通り、`make lint` が新たな指摘を出さないこと。挙動の検証はステップ3-4 で行う。

#### ステップ3-2: 採用事実の記録の組み込み

**変更ファイル**: `internal/groupmembership/manager.go`

- [x] 実在確認が成功し、かつ `parsedUID != realUID` のときにのみ `deps.reportAdoption(policy, realUID, parsedUID)` を呼ぶ処理を追加する
- [x] `reportAdoption` は戻り値を持たない。呼び出し結果を受け取る変数も分岐も書かず、記録の成否が基準UIDやエラーに影響しないことをコードの形で示す（02 §1.2）

**完了条件**: `go build ./...` が通ること。挙動の検証はステップ3-4 で行う。

#### ステップ3-3: ドキュメントコメントの更新

**変更ファイル**: `internal/groupmembership/manager.go`

- [x] `resolvePermissionCheckUID` のドキュメントコメントに、`SudoUIDAware` では採用前に実在確認を行い、失敗時は `ErrSudoUIDUserNotFound` または `ErrSudoUIDUserLookupFailed` を返すことを英語で追記する
- [x] `getPermissionCheckUID` のドキュメントコメント（446-456行目）の `Returns` 節に、同じ2つのエラーを返しうることを英語で追記する

> **02 §8 との差異**: 02 §8 は `manager.go` の2つの関数のドキュメントコメント更新をフェーズ4に置いている。本計画ではこれをフェーズ3のステップ3-3 へ前倒しした。両コメントはステップ3-1 が追加するエラーそのものを述べるものであり、フェーズ4へ残すと PR-3 と PR-4 の間、コメントが実装と食い違う期間が生じるためである。`policy.go` の `SudoUIDAware` コメント（ステップ4-2）は方針の説明であり、他の文書更新とまとめてレビューする利得の方が大きいためフェーズ4に残す。

**完了条件**: 両関数のコメントが、ステップ3-1 で追加したエラー2つを列挙していること。

#### ステップ3-4: 決定表と実在確認のテスト

**変更ファイル**: `internal/groupmembership/policy_test.go`

- [x] `TestResolvePermissionCheckUID_SudoUIDAware` の既存の表に、実在確認の入力を表す列（実在する／実在しない／確認処理が失敗）と「記録の有無」の期待列を追加する。既存7行（`unset` / `zero` / `valid` / `max uint32` / `negative` / `exceeds uint32` / `non-numeric`。`policy_test.go:183-189`）はそのまま残し、次の4行を新設する。
  - `SUDO_UID` が `"0"` で実在しない → エラー（`ErrSudoUIDUserNotFound`）、記録なし
  - `SUDO_UID` が `"1000"` で実在しない → エラー（`ErrSudoUIDUserNotFound`）、記録なし
  - `SUDO_UID` が `"1000"` で確認処理が失敗 → エラー（`ErrSudoUIDUserLookupFailed`）、記録なし
  - `SUDO_UID` が `"0"` で確認処理が失敗 → エラー（`ErrSudoUIDUserLookupFailed`）、記録なし
- [x] 既存の `max uint32`（`"4294967295"`）の行と、`TestResolvePermissionCheckUID_RealUIDOnly` が持つ 300 桁の値（`strings.Repeat("9", 300)`）の境界値は削らない
- [x] 02 §3.5 の決定表の8行と、テストの各行との対応を確認する。1〜7行目（実 UID が 0 の全ケース）は `realUID 0` サブテストの表が担い、8行目（実 UID 非 0）は既存の別サブテスト `"realUID non-zero always returns realUID without error"`（`policy_test.go:206`）が担う。8行目のサブテストには `verifyUserExists` と `reportAdoption` の呼び出し回数がいずれも 0 であることの検証を追加する
- [x] 追加・変更するサブテスト名および表の `name` フィールドはすべて英語で書く（例: `zero and user exists`、`valid and user missing`、`valid and lookup failed`）
- [x] `TestResolvePermissionCheckUID_UserNotFound` を追加する。`verifyUserExists` が `user.UnknownUserIdError(1000)` を返す場合に、基準UIDが返らず、返るエラーが `ErrSudoUIDUserNotFound` と `user.UnknownUserIdError(1000)` の両方に `errors.Is` で一致することを検証する（後者は元の失敗原因を `%w` で保持していることの確認）
- [x] `TestResolvePermissionCheckUID_UserLookupFailed` を追加する。`verifyUserExists` が独自のエラー値を返す場合に、`ErrSudoUIDUserLookupFailed` と、渡したエラー値そのものの両方に `errors.Is` で一致することを検証する
- [x] `TestResolvePermissionCheckUID_ErrorMessageContent` を追加する。実在しない場合と確認処理が失敗した場合の両方について、`err.Error()` が (a) `SUDO_UID` の生の文字列、(b) `user_database_source=` と定数 `userDatabaseSource` の値、(c) 対処を示す語句（それぞれ `re-run from an interactive sudo session` と `check the state of the user database`）の3つを含むことを検証する。02 §4.3 が求めるエラーメッセージの構成要素を固定する
- [x] `TestResolvePermissionCheckUID_SentinelErrorsAreDistinct` を追加する。`ErrSudoUIDUserNotFound` を含むエラーが `ErrSudoUIDOutOfRange` / `strconv.ErrSyntax` / `ErrSudoUIDUserLookupFailed` のいずれにも `errors.Is` で一致しないこと、および逆方向（`ErrSudoUIDUserLookupFailed` を含むエラーが `ErrSudoUIDUserNotFound` に一致しないこと）を検証する
- [x] `TestResolvePermissionCheckUID_ExistenceCheckNotInvoked` を追加する。次の2条件で `verifyUserExists` の呼び出し回数が 0 であることを検証する。(a) `SUDO_UID` が数値として不正（`"abc"` / `"-1"` / `"4294967296"`）、(b) 実 UID が 0 以外。いずれも 0160 と同じ結果（(a) は同じエラー、(b) は実 UID）が返ることを併せて検証する
- [x] `TestResolvePermissionCheckUID_ExistenceCheckSkippedUnderRealUIDOnly` を追加する。実 UID 0 かつ `SUDO_UID` が有効値のとき、`RealUIDOnly` では `verifyUserExists` と `reportAdoption` の呼び出し回数がいずれも 0 であり、同条件の `SudoUIDAware` では `verifyUserExists` が 1 回、`reportAdoption` が 1 回呼ばれることを対比して検証する（02 §7.1 の対比要件）
- [x] `TestResolvePermissionCheckUID_AdoptionRecordConditions` を追加する。記録が出るのは「実 UID 0、`SUDO_UID` 有効値かつ実 UID と異なる、実在確認成功」の場合のみであり、`SUDO_UID` 未設定・`SUDO_UID` が `0`・実在確認失敗・`RealUIDOnly` のいずれでも出ないことを検証する
- [x] `TestResolvePermissionCheckUID_ReportsAdoptionOnlyOncePerReporter` を追加する。1つの `sudoUIDAdoptionReporter` 実体に束ねた `reportAdoption` を用いて `resolvePermissionCheckUID` を3回実行し、3回すべてで基準UIDが正しく返ること、かつ捕捉されたレコードが1件であることを検証する。02 §7.1 が AC-09 について求める「同一のレポータ実体で解決を複数回実行する」形である
- [x] `policy_test.go` の `import` に `os/user` を追加する（`user.UnknownUserIdError` を使うため）

**完了条件**: `go test -tags test -run TestResolvePermissionCheckUID ./internal/groupmembership/` が非 root で通る。

#### ステップ3-5: セキュリティテスト

**変更ファイル**: `internal/groupmembership/policy_test.go`

- [x] `TestResolvePermissionCheckUID_FailsClosedOnExistenceFailure` を追加する。実在しない場合と確認処理が失敗する場合の両方について、返る UID が **`0` そのものである**こと（`SUDO_UID` の値でも実 UID でもない）とエラーが非 nil であることを検証する。決定表の行が検証するのは「エラーであること」であり、本テストは「基準UIDとして使える値が返らないこと」を別途固定する。この重複は意図的である
- [x] `TestResolvePermissionCheckUID_VerifiesEvenWhenSudoUIDIsZero` を追加する。`SUDO_UID` が `"0"` のときにも `verifyUserExists` が **呼ばれること（呼び出し回数 1）** を検証する。決定表の対応行が検証するのは結果（エラーになること）であり、本テストは「実 UID と同じ値でも確認を省かない」という処理の実行そのものを固定する。この重複は意図的である
- [x] `TestResolvePermissionCheckUID_RecordFailureDoesNotChangeVerdict` を追加する。`Handle` が常にエラーを返す捕捉ハンドラを使うロガーへ記録する場合でも、基準UIDが正しく返りエラーが nil であることを検証する
- [x] 02 §7.2 の4項目のうち「実在確認が失敗した結果がメモに登録されないこと」は、ステップ1-5 の `TestSudoUIDExistenceMemo_DoesNotRememberFailures` が既に検証している。重複するテストは追加しない

**完了条件**: 上記3テストが非 root で通る。

#### ステップ3-6: 既定ロガーへの出力の検証（AC-11）

**変更ファイル**: `internal/groupmembership/manager_test.go`

- [x] `TestSudoUIDAdoptionRecordReachesDefaultLogger` を追加する。手順は次のとおり。
  1. `previous := slog.Default()` を保存し、`t.Cleanup(func() { slog.SetDefault(previous) })` を登録する
  2. 捕捉ハンドラを持つロガーを `slog.SetDefault` で既定ロガーとして設定する
  3. `t.Setenv(sudoUIDEnvVar, "1000")` を置く
  4. `permissionCheckUIDDeps` を組み立てる。`getenv` は `os.Getenv` とする。`reportAdoption` は **ステップ2-2 で `getPermissionCheckUID` に書いた式をそのまま複製する**。ただし、パッケージレベルの `processSudoUIDAdoptionReporter` ではなく、このテスト内で生成した新しい `sudoUIDAdoptionReporter` の実体を使う（理由は次の項目に述べる）。`verifyUserExists` は常に `nil` を返す関数へ差し替える
  5. `resolvePermissionCheckUID(SudoUIDAware, 0, deps)` を実行し、基準UIDが `1000` であること、および捕捉ハンドラが警告1件を受け取ったことを検証する
- [x] レポータ実体をテスト内で新規に生成する理由を、テストのドキュメントコメントに英語で書く。パッケージレベルの実体は1回制限のフラグをプロセス単位で持つため、それを使うと `go test -count=2` や他テストの実行順によって結果が変わる。本テストが確認するのは「`slog.Default()` を記録の出力先として解決する組み立てが機能すること」であり、パッケージレベル実体を渡していること自体は 02 §7.6 のとおりコードレビューで確認する
- [x] このテストはプロセス全体の状態（`slog` の既定ロガー、環境変数）を変更するため `t.Parallel()` を付けない。02 §7.4 のとおり

**完了条件**: `go test -tags test -count=2 -run TestSudoUIDAdoptionRecordReachesDefaultLogger ./internal/groupmembership/` が通る（再実行でも結果が変わらないこと）。

#### ステップ3-7: `TestGetPermissionCheckUID` の期待値の更新

02 §7.5 が挙げる更新対象のテストのうち、シグネチャ移行はステップ2-4 / 2-5 に、決定表の行追加はステップ3-4 に含めた。残る1件をここで扱う。

**変更ファイル**: `internal/groupmembership/manager_test.go`

- [x] サブテスト `"reads SUDO_UID from the real environment under SudoUIDAware"`（610-621行目）を書き換える。実在確認の追加により、UID `9999` が実在しない環境では `ErrSudoUIDUserNotFound` が返るため、現在の実装のままでは root で失敗する
- [x] **失敗するアサーションは分岐の内側ではなく、`gm.getPermissionCheckUID()` の直後にある無条件の `assert.NoError(t, err)`（615行目）である。** この無条件のアサーションを削除し、エラーと UID の検証を `os.Getuid() == 0` の各分岐へ移す
- [x] `gm.getPermissionCheckUID()` を呼ぶ **前** に、`if os.Getuid() == 0 { if _, err := user.LookupId("9999"); err == nil { t.Skip("UID 9999 exists in this environment; the not-found path cannot be exercised") } }` を置く。呼び出しの後にスキップを置くと、スキップより先にアサーションが評価されてしまうため、順序が重要である
- [x] `os.Getuid() == 0` の分岐に `require.Error(t, err)` と `assert.ErrorIs(t, err, ErrSudoUIDUserNotFound)` を置く
- [x] `else` 分岐に `require.NoError(t, err)` と `assert.Equal(t, os.Getuid(), uid)` を置く
- [x] サブテスト名を `"consults SUDO_UID from the real environment under SudoUIDAware and requires the user to exist"` へ変更する
- [x] `TestGetPermissionCheckUID` の期待値の根拠を述べるコメントを、実在確認込みの規則へ更新する（02 §7.5）
- [x] `os/user` は `manager_test.go:6` で既に import 済みであることを確認する（追加は不要）

**完了条件**: 非 root（`id -u` が 0 以外）で `make test` が通る。root 環境での挙動は本書 5 章のリスクとして扱う。

**フェーズ3の完了条件**:
- [x] `make fmt` → `make test` → `make lint` がすべて通る
- [x] 本書 8 章の受け入れ基準検証表のうち AC-01〜AC-16 の `test` 項目がすべて成功する

### PR-3 作成ポイント: existence check and adoption record

**対象ステップ**: 3-1 / 3-2 / 3-3 / 3-4 / 3-5 / 3-6 / 3-7

**推奨タイトル**: `feat(0161)!: verify SUDO_UID exists before adopting it`（本文に `BREAKING CHANGE:` フッタを置き、02 §5.3 の4環境を挙げる）

**レビュー観点**: 実在確認の失敗時に基準UIDを返さず `0` と非 nil エラーを返すフェイルクローズドが両経路（実在しない／確認処理が失敗）で固定されているか / 2つのセンチネルが `errors.Is` で相互に区別でき、元のエラーを `%w` で保持しているか / 記録が「実 UID 0・`SUDO_UID` が実 UID と異なる有効値・実在確認成功」の場合に限られ、記録の失敗が読み取り判定を変えないか / ステップ3-7 のスキップ判定が `getPermissionCheckUID()` の呼び出し **前** に置かれ、root 環境で無条件アサーションが残っていないか / ステップ3-6 が複製した `reportAdoption` の式が、マージ済みの `getPermissionCheckUID`（PR-2）の式と字句どおり一致しているか（レポータ実体だけが異なる）

**レビュー順序**: この PR は本番コードの差分が `manager.go` の約30行である一方、テストの追加が2ファイルにまたがり12本に及ぶ。セキュリティ上の差分を見失わないよう、(1) ステップ3-1・3-2 の `manager.go` の変更、(2) ステップ3-7 の既存テストの書き換え、(3) 残りのテスト追加、の順に読む。ステップ3-5 と 3-6 のテストだけを後続 PR へ切り出す案は採らない。ステップ3-5 はフェイルクローズドの保証そのものを固定するテストであり、遅らせると PR-3 のマージからその PR までの間、保証が検証されない期間が生じるためである。

**実装モデル要件**: frontier-required

**判定理由**: これまで成功していた `sudo record` / `sudo verify` を失敗させるセキュリティゲートの変更であり（02 §5.3 の4環境）、`mkplan.md` ステップ8 のパネルモード契機「セキュリティゲート／移行（挙動の引き上げと引き下げが同時に起きる、テスト更新が多い）」に該当する。決定表の4行追加と11本のテスト追加に加え、ステップ3-7 は既存テストが root で失敗する破壊的更新を含む。

- [x] root で `go test -tags test -run TestGetPermissionCheckUID ./internal/groupmembership/` を1回実行し、ステップ3-7 の `t.Skip` または `ErrSudoUIDUserNotFound` の分岐が通ることを確認した。グリーンゲート（非 root）ではこの分岐に入らないため、この確認だけが 5.1 節の最上位リスクの検証手段である。root 実行ができない場合は、その旨と未検証であることを PR 本文に記す
- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### フェーズ4: 文書の更新

フェーズ4のステップは、用語集への追加を先頭に置いた順序で並べている。`/mktrans` による英語版の生成が用語集の訳語を参照するため、用語の確定を先に済ませる必要があるからである（本書 10 章）。

#### ステップ4-1: 用語集（AC-19）

**変更ファイル**: `docs/translation_glossary.md`

- [ ] 「A-Z (アルファベット順)」の各節へ次の5語を追加する。この用語集は英語見出し語のアルファベット順で節が分かれているため、追加先の節は英訳の頭文字で決まる。訳語は 02 の「用語」節に対応させる。
  - `### A` 節: 採用（`SUDO_UID` を基準UIDとして用いること）→ adoption
  - `### A` 節: 採用事実の記録 → adoption record
  - `### E` 節: 実在確認 → existence check
  - `### S` 節: センチネルエラー → sentinel error
  - `### U` 節: ユーザーデータベース種別 → user database source
- [ ] 既存の `基準UID` / `基準UID決定方針`（47-48行目、`### B` 節）と同じ3列構成（`| 日本語 | English | 備考 |`）に揃え、`備考` セルの末尾に `（Task 0161）` を付す。表のヘッダ行（`| 日本語 | English | 備考 |`）は変更しない
- [ ] 「更新履歴」表の末尾に `| YYYY-MM-DD | SUDO_UID 実在確認関連の用語を追加 (existence check, adoption, adoption record, sentinel error, user database source) |` を追加する（`YYYY-MM-DD` は実際に追加した日付に置き換える）

**完了条件**: 8 章の AC-19 の `static` チェックが期待どおりの結果になること。ステップ4-3 と 4-5 の `/mktrans` 実行より前に完了させる。

#### ステップ4-2: 既存ドキュメントコメントの更新

**変更ファイル**: `internal/groupmembership/policy.go`

- [ ] `SudoUIDAware` 定数のコメント（24-31行目）のうち、次の2文を書き換える。
  - 変更前: `// SUDO_UID is only checked for numeric validity; it is not verified to`／`// correspond to a real user. This policy therefore accepts that anyone`／`// able to start the binary as root can specify the base UID at will.`
  - 変更後: `// SUDO_UID is checked for numeric validity and verified to correspond to`／`// a user that exists in the user database; the resolution fails closed when`／`// it does not. This policy therefore accepts that anyone able to start the`／`// binary as root can specify any existing user's UID as the base UID.`
- [ ] 直後の `// It is only selected when explicitly declared.` は変更しない

**完了条件**: `rg -n "it is not verified to" internal/ cmd/` の結果が空であること。

#### ステップ4-3: 開発者向け文書（AC-17）

**変更ファイル**: `docs/dev/architecture_design/security-architecture.ja.md`、`docs/dev/architecture_design/security-architecture.md`

- [ ] `security-architecture.ja.md:50` の括弧内の基準UID解決規則の記述を、実在確認込みの規則へ書き換える。
  - 変更前: `record` は基準UID決定方針として `SudoUIDAware` を宣言しているため、実UIDが0かつ`SUDO_UID`が0..MaxUint32の範囲の数値UIDであればその値を、それ以外は実UIDを採用）
  - 変更後: `record` は基準UID決定方針として `SudoUIDAware` を宣言しているため、実UIDが0かつ`SUDO_UID`が0..MaxUint32の範囲の数値UIDであり、かつその UID がユーザーデータベース上に実在する場合にその値を採用し、実在を確認できない場合は読み取り安全性チェックを失敗させる。それ以外は実UIDを採用）
- [ ] ステップ4-1 の用語集への追加（`実在確認` → `existence check` ほか）が完了していることを確認する。`/mktrans` は用語集を参照するため、英訳の語が確定していないと、8 章の AC-17 の英語版チェックが期待どおりに一致しない
- [ ] 日本語版をコミットしたのち、`/mktrans` を実行して `security-architecture.md:50` の対応箇所へ反映する（CLAUDE.md「Translation Workflow」の順序に従う）
- [ ] 書き換えた記述が実装（ステップ3-1、3-2）と一致していることを、`resolvePermissionCheckUID` のコードと対照して確認する

**完了条件**: 8 章の AC-17 の2つの `static` チェックが期待どおりの結果になること。

#### ステップ4-4: CHANGELOG

**変更ファイル**: `CHANGELOG.md`

- [ ] `## [Unreleased]` の `### Changed` に `#### record / verify: SUDO_UID must refer to an existing user` を追加する。内容は 02 §5.3 の影響環境の表と対処、および採用時に警告が1回記録されるようになる点とする
- [ ] 既存の2項目（`sudo runner`: base UID …、`Permission checks no longer require a passwd entry …`）は変更しない

**完了条件**: `rg -n "SUDO_UID must refer to an existing user" CHANGELOG.md` が1件一致すること。

**PR-4 の完了条件**: 本書 8 章の AC-17 と AC-19 の検証コマンドが期待どおりの結果になる。AC-18 はステップ4-5（PR-5）が担うため、この時点では未達でよい。

### PR-4 作成ポイント: glossary, code comment, developer doc and changelog

**対象ステップ**: 4-1 / 4-2 / 4-3 / 4-4

**推奨タイトル**: `docs(0161): update SUDO_UID docs and SudoUIDAware comment`

**レビュー観点**: 用語集の5語が 02 の用語節と一致し英訳の頭文字の節へ入っているか / `policy.go` の書き換え後コメントが実装（ステップ3-1、3-2）の挙動と一致するか / `security-architecture` の日英が同じ規則を述べ、英語版が用語集の訳語を使っているか / CHANGELOG が 02 §5.3 の影響環境と対処を落とさず記載しているか

**実装モデル要件**: standard

**判定理由**: 用語集・コメント・開発者向け文書・CHANGELOG の記述更新のみで、未確定の設計判断もパネルモードの契機（重い統合テスト／CI／外部資源、セキュリティゲート、移行）も該当しない。ステップ4-2 は `policy.go` を変更するが、変更対象はコメント3行であり挙動を持たない。

- [ ] `rg -n "it is not verified to" internal/ cmd/` の結果が空であること（10 章。ステップ4-2 の書き換え漏れの確認）
- [ ] ステップ4-1 の用語集への追加が、ステップ4-3 の `/mktrans` 実行より前に完了していること（10 章）
- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### ステップ4-5: 利用者向け文書（AC-18）

**変更ファイル**: `docs/user/record_command.ja.md`、`docs/user/verify_command.ja.md`、`docs/user/record_command.md`、`docs/user/verify_command.md`

- [ ] `record_command.ja.md` の「5. トラブルシューティング」に `### 5.6 SUDO_UID の実在確認に失敗する` を追加する（既存の最後の項は `### 5.5 ディレクトリを指定した場合`）。内容は次を含める。
  - `sudo record` の実行時に `SUDO_UID` が指す UID が実在するユーザーでなければ、対象ファイルごとに読み取りが失敗すること
  - エラーメッセージの見分け方。2種のセンチネル文言 `SUDO_UID does not refer to an existing user` と `failed to verify that SUDO_UID refers to an existing user` を、実装（ステップ1-2）の文字列リテラルからそのまま引用して載せる。加えて、メッセージ中の `user_database_source=nss` / `user_database_source=passwd-file` で参照先ユーザーデータベースを判別できることを説明する（`user_database_source` は採用事実の記録の属性名でもあり、エラーメッセージ内でも同じ綴りで現れる）
  - 02 §5.3 の影響環境の表に対応する4つの対処（CGO 有効での再ビルド、ユーザーデータベース復旧後の再実行、環境から `SUDO_UID` を除く、`/etc/passwd` を用意する）
  - `sudo env -u SUDO_UID record ...` が回避策になるが、基準UIDが 0 になるため読み取り判定が厳しくなり、記録そのものが失敗しうること（02 §5.3「対処手段」）
  - `SUDO_UID` の採用によって基準UIDが実 UID と異なる値になった場合、警告が標準エラー出力へ1回記録されること。この警告は `sudo record` の通常運用でも毎回出ること（02 §3.7.4）
  - cron や systemd unit から実行する場合は、この警告を捉えるため標準エラー出力を保存する必要があること（02 §5.5「記録の欠落」）
- [ ] `verify_command.ja.md` の「5. トラブルシューティング」に `### 5.7 SUDO_UID の実在確認に失敗する` を追加する（既存の最後の項は `### 5.6 スクリプトでのエラーハンドリング`）。内容は上と同じ観点を `verify` に合わせて記載する
- [ ] 両文書の「目次」は `## ` 見出しのみを列挙しているため、追加した `### ` 見出しに伴う更新は不要であることを確認する
- [ ] 記載した4つの対処のうち、実行して確認できるものは実際に確認する。(a) `sudo env -u SUDO_UID record <file>` を実行し、`SUDO_UID` が渡らないこと（および基準UIDが 0 になる結果として記録が失敗しうること）を確かめる。(b) `CGO_ENABLED=0 make build` した `record` のエラーメッセージに `user_database_source=passwd-file` が出ること、CGO 有効のビルドでは `nss` が出ることを、実在しない `SUDO_UID` を与えて確かめる。(c) ユーザーデータベースの一時障害と `/etc/passwd` の欠落は再現に環境構築を要するため、02 §5.3 の表を典拠として引用し、実行による確認は行わないことを明記する
- [ ] 日本語版をコミットしたのち、`/mktrans` を実行して `record_command.md` と `verify_command.md` へ反映する

**完了条件**: 8 章の AC-18 の `static` チェックが期待どおりの結果になり、上記 (a) と (b) の実行結果が文書の記述と一致すること。

**PR-5 の完了条件**: 本書 8 章の AC-18 の検証コマンドが期待どおりの結果になる。PR-4 の完了条件と併せて、これでフェーズ4の AC-17〜AC-19 がすべて満たされる。

### PR-5 作成ポイント: user-facing troubleshooting documentation

**対象ステップ**: 4-5

**推奨タイトル**: `docs(0161): add SUDO_UID troubleshooting to user guides`

**レビュー観点**: 掲載したセンチネル文言が実装（ステップ1-2）の文字列リテラルと一字一句一致するか / 02 §5.3 の4つの対処が漏れなく記載され、実行確認できない2つが典拠付きで区別されているか / 通常運用でも警告が毎回出ることと標準エラー出力の保存の必要性が書かれているか / `/mktrans` 生成の英語版が用語集（ステップ4-1）の訳語を使っているか

**実装モデル要件**: standard

**判定理由**: 未確定の設計判断はなく、記述内容は PR-3 で確定済みの挙動をなぞるものである。ステップ4-5 の確認手順 (a)(b) は、CGO 有効・無効の2種のビルドを作り、実在しない `SUDO_UID` を与えて `sudo` 経由で実行するという特権実行を伴うが、これは固定の2コマンドを1回ずつ実行してエラーメッセージの語を目視する確認であり、パネルモードの契機が指す「重い統合テスト／CI／外部資源のサーフェス」（継続的に維持される自動テスト基盤）には当たらない。失敗した場合の影響も文書の記述の誤りに限られ、製品の挙動には及ばない。

- [ ] `/mktrans` で生成した英語版が、ステップ4-1 で用語集に登録した訳語をそのまま使っていること（10 章）
- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含むフェーズ | 成果物 | 検証 |
|---|---|---|---|
| M1: 部品が揃う | フェーズ1（PR-1） | ユーザーデータベース種別の定数、センチネルエラー2つ、レポータ型と1回制限、メモ型、およびそれらの単体テスト | Linux で `make test`（CGO 有効の回は `-race` 付き、CGO 無効の回も実行される）。既存の挙動に変化がないこと |
| M2: 差し替え口が整う | フェーズ2（PR-2） | `permissionCheckUIDDeps`、`lookupUserByUID`、新シグネチャの `resolvePermissionCheckUID`、公開ラッパー、移行済みの既存テスト | `make fmt` → `make test` → `make lint`。挙動が 0160 完了時点と同一であること |
| M3: 機能が完成する | フェーズ3（PR-3） | 実在確認とエラー分類、採用事実の記録、決定表・セキュリティ・既定ロガーのテスト | 8 章の AC-01〜AC-16 の `test` 項目がすべて成功 |
| M4: 文書が揃う | フェーズ4（PR-4 + PR-5） | ドキュメントコメント、開発者向け・利用者向け文書、用語集、CHANGELOG | 8 章の AC-17〜AC-19 の検証コマンドが期待どおり |

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1-1 / 1-2 / 1-3 / 1-4 / 1-5 | ユーザーデータベース種別の定数、センチネルエラー2つ、採用事実レポータ（1回制限）、実在確認のメモ、およびそれらの単体テスト | frontier-recommended |
| PR-2 | 2-1 / 2-2 / 2-3 / 2-4 / 2-5 | `permissionCheckUIDDeps` と `lookupUserByUID` の追加、`resolvePermissionCheckUID` のシグネチャ変更、公開ラッパーの更新、`internal/groupmembership` と `cmd/*` の既存テストの移行 | standard |
| PR-3 | 3-1 / 3-2 / 3-3 / 3-4 / 3-5 / 3-6 / 3-7 | 実在確認とエラー分類、採用事実の記録の組み込み、`manager.go` のドキュメントコメント更新、決定表の拡張・セキュリティ・既定ロガーのテスト、`TestGetPermissionCheckUID` の期待値更新。挙動を変える唯一の PR | frontier-required |
| PR-4 | 4-1 / 4-2 / 4-3 / 4-4 | 用語集、`policy.go` の `SudoUIDAware` コメント、`security-architecture`（日英）、`CHANGELOG.md` | standard |
| PR-5 | 4-5 | `record` / `verify` の利用者向けトラブルシューティング項目（日英） | standard |

**PR-1〜PR-3 の分割と 02 §8 との差異**: 02 §8 はフェーズ1〜3 を1つの PR とする構成を想定している。本計画ではこれをフェーズ単位の3つの PR（PR-1〜PR-3）に分けた。フェーズ1は既存の挙動を変えない追加のみ、フェーズ2は挙動を変えないシグネチャ移行であり、いずれも単独でグリーンゲートを通せるうえ、フェーズ3のセキュリティ挙動変更を機械的な変更から切り離して詳細にレビューできるためである。フェーズの順序と各フェーズの目的は 02 §8 のままである。

**マージ順序**: PR-1 → PR-2 → PR-3 → PR-4 → PR-5 の順にマージする。この順序は入れ替えられない。PR-2 のステップ2-2 は PR-1 が導入する `sudoUIDExistenceMemo.verify` と `processSudoUIDAdoptionReporter` を参照するためコンパイルできず、PR-3 のテストは PR-2 の差し替え口を前提とし、PR-5 の `/mktrans` は PR-4 の用語集を前提とする。各 PR が単独でグリーンゲートを通せるというのは、直前までの PR がマージ済みであることを前提とした話であり、依存関係がないという意味ではない。

02 §8 が述べる「M2 単独では出荷しない」という制約は維持する。これはマージ単位ではなくリリース単位の制約であり、PR-2 のマージ時点では実在確認が行われないため、PR-3 のマージより前にリリースを切ってはならない。

**PR-4 と PR-5 の分割**: フェーズ4のうち利用者向け文書（ステップ4-5）は新規のトラブルシューティング項目2件を4ファイルへ追加し、実バイナリでの実行確認を伴うため、他の文書更新とは分けて独立にレビューする。ステップ4-1 の用語集は `/mktrans` の前提であるため、PR-4 が PR-5 より先にマージされる必要がある。

## 4. テスト戦略

方針は 02 §7 に従う。ここでは実装計画としての要点のみを述べる。

### 4.1 単体テストの範囲

- 全テストを `internal/groupmembership` パッケージ内に置く。実 UID・環境変数・実在確認・記録の出力をすべて引数で与えるため、root 権限も実際のユーザーデータベースも不要である（AC-16）。
- 02 §3.5 の決定表の全8行を `TestResolvePermissionCheckUID_SudoUIDAware` の表で固定する。各行で基準UID・エラー・記録の有無の3つを検証する（AC-15）。
- 境界値は既存テストが持つ `""` / `"0"` / `"1000"` / `"4294967295"` / `"4294967296"` / `"-1"` / `"abc"` / `strings.Repeat("9", 300)` を維持し、実在確認の観点を追加する。
- エラー経路は「実在しない（確定的）」「確認処理が失敗（不確定）」の2種を別テストで検証し、`errors.Is` による相互の区別も検証する（AC-03、AC-04）。

### 4.2 並行性

- `sudoUIDAdoptionReporter` の1回制限と `sudoUIDExistenceMemo` の並行安全性を、いずれも 50 goroutine から同時に呼ぶテストで検証する。
- 競合検出は `make test` の CGO 有効の回が `-race` 付きで実行する（`Makefile:455`）。ローカルで対象パッケージだけを速く確認する場合は `go test -tags test -race ./internal/groupmembership/` を用いる。両者は同じ検査であり、独立した追加要件ではない。
- プロセス全体の状態に触れないテストには `t.Parallel()` を付ける。触れるテスト（`slog` の既定ロガーを差し替える AC-11 のテスト）には付けない（02 §7.4）。

### 4.3 後方互換性

- `RealUIDOnly` の既存挙動は `TestResolvePermissionCheckUID_RealUIDOnly` の既存の網羅（`realUID` 2種 × `SUDO_UID` 8種）を維持することで担保する（AC-14）。
- `cmd/record` / `cmd/verify` / `cmd/runner` の方針宣言テストは、シグネチャ移行のみを行い検証内容を弱めない。
- 公開 API のシグネチャは変更しないため、`internal/safefileio` を含む呼び出し元のコンパイルは影響を受けない。

### 4.4 テストで到達できない範囲

02 §7.6 の3点（パッケージレベルのレポータ実体を渡していること、`slog.Default()` を渡していること、`lookupUserByUID` の既定実装が実際のユーザーデータベースを引くこと）はテストで検証できない。本書 8 章では該当 AC（順に AC-09、AC-11、AC-01）に `manual` としてコードレビューによる確認を併記し、同じ AC に `test` の検証も必ず1つ以上置く。AC-09 についてはさらに、実体の宣言が1つだけであることを `static` な `rg` チェックで補強する。

## 5. リスク管理

### 5.1 技術リスク

| リスク | 影響 | 緩和策 |
|---|---|---|
| root で `make test` を実行すると `TestGetPermissionCheckUID` が失敗する | CI（非 root）では検出できず、root で開発する環境でのみ失敗する | ステップ3-7 で当該サブテストを実在確認込みの期待値へ書き換え、UID `9999` が実在する環境では `t.Skip` する |
| `resolvePermissionCheckUID` のシグネチャ変更が `//go:build test` のファイル経由で `cmd/*` のテストへ波及する | フェーズ2を分割するとコンパイルできない | ステップ2-1〜2-5 を同一フェーズ・同一コミット群に置く。完了条件を `make test` の通過とする |
| `userDatabaseSource` の定数を片方のビルドタグ付きファイルにしか追加しない | 一方のビルドでコンパイルエラー | ステップ1-1 で両ファイルへ追加し、`make test` が CGO 有効・無効の両方を実行することで検出する（`Makefile:454-460`）。macOS では CGO 無効の回が走らないため、Linux での実行を必須とする |
| `sudoUIDExistenceMemo.confirmed` の初期化漏れ | `nil` マップへの書き込みでパニック | ステップ1-4 で `New` の構造体リテラルへ `make` を含める。`TestSudoUIDExistenceMemo_*` はメモを直接生成するため `New()` 経由の初期化を検証せず、既存の `CanCurrentUserSafelyReadFile` 系テストも既定方針が `RealUIDOnly` で非 root 実行のためメモに到達しない。いずれもこの初期化漏れを検出できないため、ステップ1-5 に `TestNewInitializesSudoUIDExistenceMemo` を専用に置き、`New()` 経由のインスタンスでメモを実際に使う経路を検証する |
| `reportAdoption` の `realUID` と `permissionCheckUID` の引数を入れ替える | 記録が誤った UID を示す。型では検出できない | `TestSudoUIDAdoptionReporter_Report` で両者に異なる値を与え、属性ごとに検証する（02 §3.1） |
| 実在確認の追加でユーザーデータベースへの照会が増える | ディレクトリ NSS 環境で `record` が大幅に遅くなる | メモによりインスタンスあたり1回に抑える（02 §3.4）。`TestSudoUIDExistenceMemo_ReusesConfirmation` で呼び出し回数を固定する |

### 5.2 運用リスク

| リスク | 影響 | 緩和策 |
|---|---|---|
| これまで成功していた `sudo record` / `sudo verify` が失敗するようになる | 02 §5.3 の4環境で実行が止まる | エラーメッセージに `SUDO_UID` の値・UID・ユーザーデータベース種別・対処を含める（ステップ3-1）。`CHANGELOG.md` と利用者向け文書に対処を記載する（ステップ4-4、4-5） |
| 通常運用でも警告が毎回出るため、運用者が事故と混同する | 記録の意味を誤解する | 利用者向け文書に、「この警告は `sudo` 経由の通常運用でも毎回出る」ことと、事故かどうかは実行文脈と照らし合わせて判断することを明記する（ステップ4-5、02 §3.7.4） |

### 5.3 スケジュールリスク

フェーズ4の `/mktrans` による英語版の反映は、日本語版のコミット後に別ステップとして実行する。翻訳が滞った場合でも PR-1〜PR-3 は独立して完了できるため、機能実装がブロックされることはない。

## 6. 実装チェックリスト

### PR-1: 独立した部品の追加
- [x] ステップ1-1: ユーザーデータベース種別の定数（`membership_cgo.go` / `membership_nocgo.go` + 各ビルドタグのテスト）
- [x] ステップ1-2: `ErrSudoUIDUserNotFound` / `ErrSudoUIDUserLookupFailed`
- [x] ステップ1-3: `sudoUIDAdoptionReporter` 型・`report`・パッケージレベル実体
- [x] ステップ1-4: `sudoUIDExistenceMemo` 型・`verify`・`GroupMembership` フィールド・`New` の初期化
- [x] ステップ1-5: フェーズ1の単体テスト（捕捉ハンドラ + 9テスト）
- [x] `make fmt` → `make test` → `make lint`（Linux で実行し、CGO 有効・無効の両方を通す）
- [ ] PR-1 マージ済み（対象ステップ: 1-1 / 1-2 / 1-3 / 1-4 / 1-5）

### PR-2: 依存の束への移行
- [x] ステップ2-1: `permissionCheckUIDDeps`・`lookupUserByUID`・シグネチャ変更
- [x] ステップ2-2: `getPermissionCheckUID` での本番依存の組み立て
- [x] ステップ2-3: `test_helpers_policy.go` の公開ラッパー更新
- [x] ステップ2-4: `policy_test.go` の既存3テストの移行
- [x] ステップ2-5: `cmd/record` / `cmd/verify` / `cmd/runner` の既存テストの移行とコメント修正
- [x] `make fmt` → `make test` → `make lint`（Linux で実行し、CGO 有効・無効の両方でコンパイルされることを確認）
- [ ] PR-2 マージ済み（対象ステップ: 2-1 / 2-2 / 2-3 / 2-4 / 2-5）

### PR-3: 実在確認と記録の組み込み
- [x] ステップ3-1: 実在確認の呼び出しとエラー分類・エラーメッセージ
- [x] ステップ3-2: 採用事実の記録の組み込み
- [x] ステップ3-3: `resolvePermissionCheckUID` / `getPermissionCheckUID` のドキュメントコメント更新
- [x] ステップ3-4: 決定表の拡張と実在確認・エラーメッセージ・記録のテスト（表の拡張 + 8テスト）
- [x] ステップ3-5: セキュリティテスト（3テスト）
- [x] ステップ3-6: 既定ロガーへの出力の検証（AC-11）
- [x] ステップ3-7: `TestGetPermissionCheckUID` の期待値とサブテスト名の更新
- [x] `make fmt` → `make test` → `make lint`（Linux で実行）
- [ ] PR-3 マージ済み（対象ステップ: 3-1 / 3-2 / 3-3 / 3-4 / 3-5 / 3-6 / 3-7）

### PR-4: 用語集・コメント・開発者向け文書・CHANGELOG
- [ ] ステップ4-1: `translation_glossary.md`（用語5語 + 更新履歴）— ステップ4-3 / 4-5 の `/mktrans` 実行より先に行う
- [ ] ステップ4-2: `policy.go` の `SudoUIDAware` コメント
- [ ] ステップ4-3: `security-architecture.ja.md` → `/mktrans` で `security-architecture.md`
- [ ] ステップ4-4: `CHANGELOG.md`
- [ ] 8 章の AC-17 と AC-19 の検証コマンドを実行
- [ ] PR-4 マージ済み（対象ステップ: 4-1 / 4-2 / 4-3 / 4-4）

### PR-5: 利用者向け文書
- [ ] ステップ4-5: `record_command.ja.md` / `verify_command.ja.md` → `/mktrans` で英語版
- [ ] 8 章の AC-18 の検証コマンドを実行
- [ ] PR-5 マージ済み（対象ステップ: 4-5）

## 7. テストヘルパーの配置

[test_organization.md](../../dev/developer_guide/test_organization.md) の分類に従い、本タスクで追加・変更するテスト補助は次のとおりとする。

| 対象 | 配置 | 根拠 |
|---|---|---|
| `slog` 出力の捕捉ハンドラ | 既存の `internal/groupmembership/membership_common_test.go` 内 | `manager_test.go` と `policy_test.go` の双方から使うため、パッケージ内でテストファイルをまたいで共有されるヘルパーの既存の置き場に置く。他パッケージからは参照しないため `testutil/` は不要。ビルドタグなしの `_test.go` であり、`test_organization.md` が規定する `//go:build test` のヘルパーファイル（`test_helpers_*.go`）には当たらない |
| `permissionCheckUIDDeps` を組み立てるヘルパー | `internal/groupmembership/policy_test.go` 内 | 非公開型を扱うためパッケージ内に限る。`_test.go` 内で足りる |
| `PermissionCheckUIDDeps` と `ResolvePermissionCheckUID` | 既存の `internal/groupmembership/test_helpers_policy.go`（`//go:build test`） | 非公開の `resolvePermissionCheckUID` を他パッケージのテストへ露出する。既存の同種ラッパーと同じファイルへ置く。新規ファイルは作らない |

`internal/groupmembership/testutil/` は作成しない。新規のヘルパーファイルも作成しない。

## 8. 受け入れ基準の検証

各 AC の検証手段を `test`（実行可能・誤った挙動で失敗する）／`static`（`rg` またはコンパイル）／`manual`（レビュー・観察）で分類する。すべての AC に `test` または `static` を1つ以上置く。

| AC | 種別 | 検証内容 |
|---|---|---|
| AC-01 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_SudoUIDAware`（`SUDO_UID` 有効値かつ実在確認成功の行で基準UIDが `SUDO_UID` の値になる） |
| AC-01 | `test` | `cmd/record/main_test.go::TestRecordDeclaresSudoUIDAwarePolicy`、`cmd/verify/main_test.go::TestVerifyDeclaresSudoUIDAwarePolicy`（宣言下での採用が維持される） |
| AC-01 | `manual` | `lookupUserByUID` の既定実装が `user.LookupId` を呼ぶ1行であることをコードレビューで確認（02 §7.6） |
| AC-02 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_UserNotFound`、`internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_FailsClosedOnExistenceFailure` |
| AC-03 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_SentinelErrorsAreDistinct` |
| AC-03 | `test` | `internal/groupmembership/manager_test.go::TestSudoUIDExistenceErrorMessages`（センチネル2種の定義部分。`.Error()` 文言をリテラルで固定し、相互に `errors.Is` で一致しないことを検証する） |
| AC-04 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_UserLookupFailed`（`ErrSudoUIDUserLookupFailed` と渡したエラー値の双方に `errors.Is` で一致する） |
| AC-05 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_ExistenceCheckNotInvoked`（数値として不正な3値で `verifyUserExists` の呼び出し回数が 0、かつ 0160 と同じエラー） |
| AC-06 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_ExistenceCheckNotInvoked`（実 UID が 0 以外で呼び出し回数 0、実 UID が返る） |
| AC-07 | `test` | `internal/groupmembership/manager_test.go::TestSudoUIDAdoptionReporter_Report`（レベルが `slog.LevelWarn`）、`internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_AdoptionRecordConditions`（採用時に記録が出る） |
| AC-08 | `test` | `internal/groupmembership/manager_test.go::TestSudoUIDAdoptionReporter_Report`（`permission_check_uid` / `real_uid` / `source_env_var` / `permission_check_uid_policy` / `user_database_source` の5属性を検証し、`real_uid` と `permission_check_uid` に異なる値を与える） |
| AC-09 | `test` | `internal/groupmembership/manager_test.go::TestSudoUIDAdoptionReporter_ReportsOnlyOnce`、`internal/groupmembership/manager_test.go::TestSudoUIDAdoptionReporter_ReportsOnlyOnceConcurrently` |
| AC-09 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_ReportsAdoptionOnlyOncePerReporter`（同一レポータ実体で解決を3回実行して記録は1件） |
| AC-09 | `static` | `rg -n "^var processSudoUIDAdoptionReporter\s+sudoUIDAdoptionReporter$" internal/groupmembership/manager.go` が1件一致すること（パッケージレベルの実体が1つだけ宣言されていること） |
| AC-09 | `static` | `rg -n "sudoUIDAdoptionReporter" internal/groupmembership/manager.go internal/groupmembership/test_helpers_policy.go` の結果に、上記の `var` 宣言・型宣言・`report` のレシーバ・`test_helpers_policy.go` のラッパー内の生成以外の実体の生成が現れないこと（本番コードが呼び出しごとに新しい実体を作っていないこと） |
| AC-09 | `test` | `internal/groupmembership/manager_test.go::TestProcessSudoUIDAdoptionReporterIsProcessWide`（パッケージレベルの実体が未報告のまま保たれていること。テストがこの実体を経由して報告しないことの担保でもある） |
| AC-09 | `manual` | 上記2つの `static` チェックを踏まえ、`getPermissionCheckUID` がパッケージレベルの実体を渡していることをコードレビューで確認（02 §7.6） |
| AC-10 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_AdoptionRecordConditions`（`SUDO_UID` 未設定と `SUDO_UID` が `0` の場合に記録が出ない） |
| AC-11 | `test` | `internal/groupmembership/manager_test.go::TestSudoUIDAdoptionRecordReachesDefaultLogger`（`slog.SetDefault` で差し替えた既定ロガーへ記録が届くこと。`-count=2` でも結果が変わらないこと） |
| AC-11 | `manual` | `getPermissionCheckUID` が `slog.Default()` を記録の時点で解決してレポータへ渡すことをコードレビューで確認（02 §7.6） |
| AC-12 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_ExistenceCheckSkippedUnderRealUIDOnly`（`RealUIDOnly` で呼び出し回数 0、同条件の `SudoUIDAware` で 1 という対比を取る） |
| AC-12 | `test` | `cmd/runner/main_test.go::TestRunnerDeclaresRealUIDOnlyPolicy`（`VerifyUserExists` が呼ばれたら `t.Error`） |
| AC-12 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_RealUIDOnly`（`realUID` 2種 × `SUDO_UID` 8種の全組み合わせで `verifyUserExists` の呼び出し回数 0。同条件の `SudoUIDAware` との対比は上記 `TestResolvePermissionCheckUID_ExistenceCheckSkippedUnderRealUIDOnly` が担う） |
| AC-13 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_ExistenceCheckSkippedUnderRealUIDOnly`（`RealUIDOnly` で `reportAdoption` の呼び出し回数 0）、`internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_AdoptionRecordConditions` |
| AC-13 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_RealUIDOnly`（`realUID` 2種 × `SUDO_UID` 8種の全組み合わせで `reportAdoption` の呼び出し回数 0） |
| AC-14 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_RealUIDOnly`（`realUID` 2種 × `SUDO_UID` 8種で常に実 UID）、`internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_EnvAccess`（`RealUIDOnly` では `getenv` が呼ばれない） |
| AC-15 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_SudoUIDAware`（02 §3.5 の決定表の1〜7行目を `realUID 0` サブテストの表が、8行目を `realUID non-zero` サブテストが担う。各行で基準UID・エラー・記録の有無を検証。行と 02 §3.5 の対応はステップ3-4 に明記） |
| AC-16 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_SudoUIDAware`（実在確認の差し替え口を使って全行を到達する） |
| AC-16 | `test` | `internal/groupmembership/manager_test.go::TestSudoUIDAdoptionRecordReachesDefaultLogger`（記録の出力先の差し替え口を使う） |
| AC-16 | `test` | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_PanicsOnNilDeps`（パッケージ外向けの差し替え口 `ResolvePermissionCheckUID` が、必須の2口が nil のときに解決へ進まずパニックすること） |
| AC-16 | `static` | 非 root（`id -u` が 0 以外）で `go test -tags test -run 'TestResolvePermissionCheckUID|TestSudoUIDAdoptionReporter|TestSudoUIDAdoptionRecordReachesDefaultLogger|TestSudoUIDExistenceMemo' ./internal/groupmembership/` が終了コード 0 で終わること。実在確認とログ出力先の両方の差し替え口を使うテストが、root 権限なしに実行できることの確認 |
| AC-16 | `static` | `rg -n "func ResolvePermissionCheckUID\(.*PermissionCheckUIDDeps" internal/groupmembership/test_helpers_policy.go` が1件一致すること（パッケージ外のテストからも差し替えられること） |
| AC-17 | `static` | `rg -n "実在" docs/dev/architecture_design/security-architecture.ja.md` が50行目に一致すること、かつ `rg -n "範囲の数値UIDであればその値を" docs/dev/architecture_design/security-architecture.ja.md` が一致しないこと |
| AC-17 | `static` | `rg -n "existence check|exists in the user database" docs/dev/architecture_design/security-architecture.md` が50行目に一致すること、かつ `rg -n "that value is used; otherwise the real UID is used" docs/dev/architecture_design/security-architecture.md` が一致しないこと。前者の語は `docs/translation_glossary.md` に登録した `実在確認` → `existence check` に対応する（ステップ4-3 は 4-1 の完了後に実行する） |
| AC-17 | `manual` | 書き換えた記述が実装（ステップ3-1、3-2）と一致することをコードと対照して確認する（新規記述の典拠確認） |
| AC-18 | `test` | `internal/groupmembership/manager_test.go::TestSudoUIDExistenceErrorMessages`（文書が逐語引用する2種のセンチネル文言を実装側で固定する。文書側との一致は下の `static` チェックが担う） |
| AC-18 | `static` | `rg -n "SUDO_UID" docs/user/record_command.ja.md docs/user/verify_command.ja.md docs/user/record_command.md docs/user/verify_command.md` が4ファイルすべてで1件以上一致すること |
| AC-18 | `static` | 2種のセンチネル文言のそれぞれについて、実装と文書で同一のリテラルが使われていることを確認する。`rg -n "SUDO_UID does not refer to an existing user" internal/groupmembership/manager.go docs/user/record_command.ja.md docs/user/verify_command.ja.md docs/user/record_command.md docs/user/verify_command.md` と `rg -n "failed to verify that SUDO_UID refers to an existing user" internal/groupmembership/manager.go docs/user/record_command.ja.md docs/user/verify_command.ja.md docs/user/record_command.md docs/user/verify_command.md` が、いずれも5ファイルすべてで1件以上一致すること |
| AC-18 | `static` | `rg -n "user_database_source" docs/user/record_command.ja.md docs/user/verify_command.ja.md` が両ファイルで一致すること（ユーザーデータベース種別による判別方法が記載されていること） |
| AC-18 | `manual` | 記載した `sudo env -u SUDO_UID record <file>` を実行し、`SUDO_UID` が渡らないことと文書の説明どおりの挙動になることを確認する（新規コマンド例の典拠確認） |
| AC-18 | `manual` | CGO 有効ビルドと `CGO_ENABLED=0` ビルドの `record` に、実在しない `SUDO_UID` を与えて実行し、エラーメッセージの `user_database_source` がそれぞれ `nss` / `passwd-file` になることを確認する（新規記述の典拠確認）。残る2つの対処（ユーザーデータベースの一時障害、`/etc/passwd` の欠落）は再現に環境構築を要するため、02 §5.3 の表を典拠として引用するに留める |
| AC-19 | `static` | 追加した5語のそれぞれについて行が存在すること。`rg -n "^\|\s*実在確認\s*\|\s*existence check\s*\|" docs/translation_glossary.md`、`rg -n "^\|\s*採用\s*\|\s*adoption\s*\|" docs/translation_glossary.md`、`rg -n "^\|\s*採用事実の記録\s*\|\s*adoption record\s*\|" docs/translation_glossary.md`、`rg -n "^\|\s*センチネルエラー\s*\|\s*sentinel error\s*\|" docs/translation_glossary.md`、`rg -n "^\|\s*ユーザーデータベース種別\s*\|\s*user database source\s*\|" docs/translation_glossary.md` がそれぞれ1件一致すること（計5件） |

### 8.1 全体の成功条件

- [ ] 上表のすべての `test` 項目が `make test` で成功する
- [ ] 上表のすべての `static` 項目が期待どおりの結果になる
- [ ] `make lint` が警告なく通る
- [ ] `make test` を Linux で実行し、CGO 有効の回（`-race` 付き）と CGO 無効の回の両方が成功する
- [ ] `make deadcode` が本タスクで追加した識別子について新たな指摘を出さない
- [ ] `make fmt` の実行後に差分が出ない

## 9. 成功基準

### 9.1 機能の完全性

- AC-01〜AC-19 のすべてが 8 章の検証で満たされている。
- 02 §3.5 の決定表の8行が、いずれもテストで検証されている。1〜7行目は `TestResolvePermissionCheckUID_SudoUIDAware` の `realUID 0` サブテストの表、8行目は同テストの `realUID non-zero` サブテストが担う（対応はステップ3-4 に明記）。

### 9.2 品質

- `make test` を Linux で実行し、CGO 有効の回（`-race` 付き）と CGO 無効の回の両方が通る。
- `make lint` が警告なく通る。`//nolint` の追加は行わない（本タスクはセキュリティリンタが指摘する構文を導入しない）。

### 9.3 セキュリティの確認

- 実在確認に失敗した場合に基準UIDが返らないことが、実在しない場合と確認処理が失敗した場合の両方について検証されている（ステップ3-5）。
- `SUDO_UID` が `0` の場合にも実在確認が行われることが検証されている（ステップ3-5）。
- 記録の失敗が読み取り判定を変えないことが検証されている（ステップ3-5）。
- `RealUIDOnly` の経路で実在確認も記録も実行されないことが、同条件の `SudoUIDAware` との対比つきで検証されている（ステップ3-4）。

### 9.4 文書の完全性

- `internal/groupmembership/policy.go` の `SudoUIDAware` コメントが現行の規則を述べている。
- `docs/dev/architecture_design/security-architecture.ja.md` とその英語版が実在確認込みの規則を述べている。
- `record` / `verify` の利用者向け文書（日本語版・英語版）に失敗条件・対処・警告の記録が記載されている。
- `docs/translation_glossary.md` に本タスクの新用語が収録されている。
- `CHANGELOG.md` に挙動変更と対処が記載されている。

## 10. 残存確認事項（`make lint` / `make test` で検出できないもの）

8 章の検証表に含めていない確認のみを挙げる。いずれも該当する PR の作成ポイントのチェックリストに再掲してあり、その PR のレビュー時点で実行する。本節はその索引である。

| 確認 | 対象ステップ | 実行する PR |
|---|---|---|
| `rg -n "userDatabaseSource" --glob '*.go'` の結果が `internal/groupmembership/` 内に限られること（他パッケージの同名識別子との衝突の確認） | 1-1 | PR-1 |
| `rg -n "getenv func\(string\) string" internal/groupmembership/` の結果が空であること（旧シグネチャの残存の確認） | 2-1 | PR-2 |
| `rg -n "pre-refactor resolvePermissionCheckUID" cmd/` の結果が空であること（コメント書き換え漏れの確認） | 2-5 | PR-2 |
| `rg -n "it is not verified to" internal/ cmd/` の結果が空であること（書き換え漏れの確認。`policy.go:27` が唯一の該当箇所であることを実装前に確認済み。書き換え後の文にも `numeric validity` は残るため、消える語句だけを対象とする） | 4-2 | PR-4 |
| `docs/translation_glossary.md` への追加を `/mktrans` より先に完了させること | 4-1 → 4-3 | PR-4 |
| `/mktrans` が生成した英語版文書が用語集の訳語をそのまま使っていること | 4-5 | PR-5 |

## 11. 次のステップ

- [ ] 本書のレビューと `approved` への更新
- [ ] 3.2 節の PR 構成に従い、PR-1 から PR-5 までを順に実装・マージする（各 PR の表題と観点は本書 2 章の `### PR-N 作成ポイント` に記載）
- [ ] PR-3 のマージ前にリリースを切らない（3.2 節「M2 単独では出荷しない」の維持）
- [ ] 実装完了後、[#941](https://github.com/isseis/go-safe-cmd-runner/issues/941) の対応状況を更新し、D1 M-3 の残課題のうち解消した2点を記録する
- [ ] 02 §9 の将来の課題（拒否の構造化ログへの記録、`user.LookupId` 呼び出しの共通化、1回制限の前提の見直し）を必要に応じて別 issue として登録する
