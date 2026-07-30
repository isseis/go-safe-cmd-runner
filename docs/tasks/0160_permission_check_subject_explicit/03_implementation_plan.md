# 実装計画書: 権限チェックの基準UIDの決定方針を呼び出し元の明示指定へ変更

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-30 |
| Review date | 2026-07-30 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- 要件定義: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計: [02_architecture.md](02_architecture.md)
- 要件プロセス: [requirements_process.md](../../dev/developer_guide/requirements_process.md)
- テスト構成ガイド: [test_organization.md](../../dev/developer_guide/test_organization.md)
- セキュリティ設計: [security-architecture.ja.md](../../dev/architecture_design/security-architecture.ja.md)
- 用語集: [translation_glossary.md](../../translation_glossary.md)

本書で使う用語（基準UID、基準UID決定方針、実 UID、読み取り安全性チェック、プロセス既定方針、最終既定方針）は [02_architecture.md](02_architecture.md) 「用語」節の定義に従う。

---

## 1. 実装概要

### 1.1 目的

読み取り安全性チェックの基準UIDを、環境変数 `SUDO_UID` の有無からの推測ではなく、バイナリごとに宣言された基準UID決定方針に基づいて決めるようにする。これにより `runner` の読み取り判定経路から `SUDO_UID` の参照を取り除く。設計判断はすべて [02_architecture.md](02_architecture.md) にあるため、本書は「どのファイルをどう変えるか」と「どう検証するか」だけを扱う。

### 1.2 実装方針

- **`record` / `verify` の観測可能な挙動は変えない。** 挙動が変わるのは `sudo runner` の形態のみで、変化は厳しい側（設計書 §5.3）である。
- **Go ソースに書く識別子・コメント・文字列リテラルはすべて英語とする。** 本書の日本語は計画の記述にのみ用いる。設計書のコード例に付いている日本語コメントをそのまま貼り付けてはならず、内容を英語に訳して記す。
- **受入基準の識別子（`AC-NN` / `F-NNN`）を Go ソースに書かない。** テストの doc コメントには検証する挙動を平易な英語で記す。本書の AC 対応表がトレーサビリティを担うためである（[requirements_process.md](../../dev/developer_guide/requirements_process.md) §4）。
- **フェーズ2とフェーズ3は同一 PR にまとめる。** 設計書 §8 の指示に従う。フェーズ3が欠けると `record` / `verify` が sudo 実行時に機能退行するためである。
- 各フェーズの完了時に `make fmt` → `make test` → `make lint` を実行し、すべて成功した状態を保つ。

### 1.3 既存コード調査結果

実装前にリポジトリ全体を調査した。設計書に記載のない事項、および設計書の記載を補正すべき点を以下に挙げる。手当てが不要な箇所は省略した。設計書そのものの修正を要した4点は §1.4 に分けて記載する。

#### 対象コードの現状

| 対象 | 現状 | 対応 |
|---|---|---|
| `internal/groupmembership/manager.go` の `getPermissionCheckUID()`（:445-452） | パッケージ関数。`getProcessRealUID()` と `os.Getenv("SUDO_UID")` を呼び、`resolvePermissionCheckUID` へ渡す | `GroupMembership` のメソッドへ変更し、有効な方針と `os.Getenv` を `resolvePermissionCheckUID` へ渡す形にする |
| 同 `resolvePermissionCheckUID(realUID int, sudoUID string)`（:470-476） | 実 UID と `SUDO_UID` の文字列値を受け取る純粋関数 | 引数を `(policy, realUID, getenv)` に変え、`SudoUIDAware` 以外では環境変数取得関数を呼ばないようにする |
| 同 `parseSudoUID(sudoUID string)`（:487-496） | `SUDO_UID` 文字列の解析と範囲検査。`strconv` のエラーを `%w` で包み、範囲外は `ErrSudoUIDOutOfRange` を包む | 本体は変更しない。doc コメントの「separated from getPermissionCheckUID」の記述のみ更新する |
| 同 `getProcessRealUID()`（:514-522） | `os.Getuid()` を直接呼ぶ | 変更しない（設計書 §3.5） |
| 同 `New()`（:86-91） | 引数なし | `New(opts ...Option)` へ変更する |
| 同 `CanCurrentUserSafelyReadFile`（:304-349） | `getPermissionCheckUID()` をパッケージ関数として呼ぶ | `gm.getPermissionCheckUID()` へ変更する。`#nosec G115` のコメント（:312-314）は関数名が変わらないため修正不要 |

#### `groupmembership.New()` の呼び出し箇所（`rg -n 'groupmembership\.New\(' -g '*.go'` で全 13 箇所を確認）

設計書 §3.2 の記載どおり、本番コード 4 箇所（`internal/safefileio/safe_file.go:38`、`internal/safefileio/testutil/mock.go:51`、`internal/runner/runner.go:301`、`internal/security/dir_permissions_unix.go:35`）とテストコード 9 箇所（`internal/safefileio/safe_file_test.go:569`、`internal/safefileio/safe_file_cleanup_test.go:191`、`internal/runner/runner_test.go:93`、`internal/runner/base/security/validator_test.go:115,126,138`、`internal/runner/base/security/file_validation_test.go:241,456,1450`）である。加えて `internal/groupmembership/test_helpers.go:8` の `newWithEnumerator` がパッケージ内から `New()` を呼ぶ。`New` を可変長引数化するため、いずれも無修正でコンパイルが通り、最終既定方針 `RealUIDOnly` を継承する。**したがってこれら 14 箇所への編集タスクは設けない。**

#### 更新が必要な既存テスト

| テスト | 現状 | 対応 |
|---|---|---|
| `internal/groupmembership/manager_test.go::TestGetPermissionCheckUID`（:599-709） | `getPermissionCheckUID()` をパッケージ関数として呼ぶサブテスト 4 件（`normal user without sudo`、`simulated sudo environment for non-root user`、`with SUDO_UID empty returns os.Getuid`、`with SUDO_UID set returns appropriate UID`）を含む。最後の 1 件は `os.Getuid() == 0` の分岐で `SUDO_UID=9999` が採用されることを検証しており、実環境の環境変数が読まれることを確かめる唯一の箇所である。残る 3 件（`SUDO_UID with invalid value`、`malicious SUDO_UID values - out of bounds`、`valid SUDO_UID values`）は `parseSudoUID` を直接呼ぶため影響を受けない | メソッド呼び出しへ書き換えたうえで、既定方針下の重複サブテストを 1 件へ統合し、実環境の `SUDO_UID` を読むことの検証は `SudoUIDAware` を指定したサブテストへ移す（ステップ 2-1） |
| 同 `::TestResolvePermissionCheckUID`（:713-759） | 方針引数を取らない現行シグネチャに依存。実 UID 0/非 0 × `SUDO_UID` 空/有効値/不正値 を検証している | 削除する。検証していた不変条件は §4.5 のとおり新規テストが引き継ぐ |

#### 設計書に記載のない、または補正が必要な点

1. **`.golangci.yml` の `depguard` 設定変更が必要。** `depguard` の `main` ルール（`files: [$all, "!$test", "!**/internal/**", "!**/*test_helpers.go"]`）の `allow` リストに `internal/groupmembership` が含まれていない。`cmd/runner/main.go` / `cmd/record/main.go` / `cmd/verify/main.go` はこのルールの対象であるため、方針宣言のために `groupmembership` を import すると `make lint` が失敗する。ステップ 3-1 で `allow` リストへ追加する。`cmd/*/main_test.go` は `!$test` により対象外なので追加不要である。
2. **新規のテスト用ヘルパーファイルは lint の除外対象にならない。** `.golangci.yml` の `exclusions` はすべて `path: _test\.go` を条件としており、`make lint` は `--build-tags test`（`Makefile:24`）で実行される。したがって `internal/groupmembership/test_helpers_policy.go` は `gosec` / `err113` / `dupl` / `mnd` / `goconst` を含む全ルールの対象になる。`depguard` の `main` ルールについては、除外パターン `"!**/*test_helpers.go"` は `test_helpers_policy.go` に一致しないが、`"!**/internal/**"` によって除外されるため対象外である。`.golangci.yml` の変更は上記 1. の `allow` 追加だけで足りる。
3. **テスト専用ラッパーの名前と配置ステップを決めた。** 設計書 §3.9 と §7.2 が求める2つのラッパー（他パッケージのテストから有効な方針を取得する／基準UIDを解決する）と、プロセス既定方針の退避・復元ヘルパーの具体名を次のとおりとする。いずれも `internal/groupmembership/test_helpers_policy.go`（`//go:build test`）に置く。

   | 名前 | 用途 | 追加するステップ |
   |---|---|---|
   | `WithPermissionCheckUIDPolicy` | インスタンス方針の指定 | 1-1 |
   | `SwapProcessPermissionCheckUIDPolicy` | プロセス既定方針の退避・復元 | 1-1 |
   | `EffectivePermissionCheckUIDPolicy` | 有効な方針の取得（`internal/safefileio` のテストから使う） | 1-1 |
   | `ResolvePermissionCheckUID` | 基準UID解決（`cmd/record` / `cmd/verify` のテストから使う） | 2-1 |

   `ResolvePermissionCheckUID` だけをステップ 2-1 に置くのは、委譲先の `resolvePermissionCheckUID` のシグネチャがフェーズ2で変わるためである（設計書 §8）。
4. **セキュリティ設計文書の記述が不正確になる。** `docs/dev/architecture_design/security-architecture.ja.md:50` と同 `.md:50` は、`record` 時点の読み取り安全性チェックの説明として「`getPermissionCheckUID`/`resolvePermissionCheckUID` が解決するUID。実UIDが0かつ`SUDO_UID`が有効な値であればその値を、それ以外は実UIDを採用」と、方針に依らない挙動として述べている。`record` は `SudoUIDAware` を宣言するため結論は変わらないが、この挙動が `record` の宣言によるものであることを明示する必要がある。加えて同 `.ja.md:831` と `.md:834` の `func New() *GroupMembership` が実際のシグネチャと乖離する。両方をステップ 4-3 で更新する。
5. **用語集に「基準UID決定方針」が未登録。** `docs/translation_glossary.md:47` に「基準UID / base UID」はあるが、方針の語がない。ステップ 4-4 で追加する。
6. **`make deadcode` は本番ビルドのみを解析する。** `Makefile:679-680` は `deadcode ./cmd/record ./cmd/runner ./cmd/verify` をビルドタグなしで実行するため、テストからのみ呼ばれる公開関数は到達不能として報告される。現状でも 7 件（`internal/runner/runner.go:140` の `WithRiskEvaluator` など）が報告されている。本タスクで追加する識別子のうち `String()`・`ProcessPermissionCheckUIDPolicy()`・`effectivePermissionCheckUIDPolicy()` は、次の 2 点によって本番経路から到達させる。(a) `effectivePermissionCheckUIDPolicy()` はプロセス既定方針を `ProcessPermissionCheckUIDPolicy()` 経由で読む（直接 atomic 変数を触らない）。これにより `CanCurrentUserSafelyReadFile` から両方へ到達する。(b) 2 つのエラー本文と各 `main` の panic メッセージを `%s` で組み立て、`String()` を参照させる。`test_helpers_policy.go` の 4 関数はタグなしビルドに存在しないため、そもそも解析対象外である。
7. **並行実行の前提を確認した。** `internal/safefileio` と `internal/groupmembership` のテストに `t.Parallel()` の呼び出しは存在しない（`rg -n 't\.Parallel\(\)' internal/safefileio/*_test.go internal/groupmembership/*_test.go` の一致は `safe_file_cleanup_test.go:23` のコメント1件のみ）。`internal/safefileio` は `safe_file_linux.go:120` の doc コメントで「Tests must not call t.Parallel() in this package」と明記しているが、`internal/groupmembership` には同様の記載がない。プロセス既定方針を書き換えるヘルパーの doc コメントにこの制約を書き、制約が利用側パッケージへ伝わるようにする（ステップ 1-1）。
8. **ビルドタグ付きテスト実行の影響はない。** `-tags integration` で走るのは `internal/security/elfanalyzer` と `internal/libccache`、`-tags performance` で走るのは `test/performance` のみ（`Makefile:487,493,599`）であり、いずれも `-tags test` 限定のヘルパーを参照しない。`make unit-test`（`Makefile:454-461`）は非 Darwin で `CGO_ENABLED=1 -race` と `CGO_ENABLED=0` の2構成を実行するため、`go test -race` の要求（要件書 Success Criteria）は `make test` で満たされる。
9. **既存の日英対応検証ツールは合否判定に使えない。** `make verify-docs`（`Makefile:683-684`）は `scripts/verification/compare_doc_structure.go` を実行して `docs/user` 配下の `.ja.md` / `.md` 対を比較するが、実行して確認したところ (a) 見出し文字列をそのまま比較するため翻訳済みの見出しがすべて「Missing in Japanese」として報告され、`docs/user` の全 10 対が現状で警告付きであり、(b) 警告があっても終了コードは 0 である。したがって完了条件の合否ゲートには使えない。代わりに、見出し数の増分を数える機械的な確認を用いる。現状 `rg -c '^#### ' docs/user/runner_command.ja.md docs/user/runner_command.md` はいずれも 28 であり、ステップ 4-1 / 4-2 で節を 1 つ追加したあとはいずれも 29 になる。日英で記述内容が対応していることは読み合わせで確認する。

### 1.4 反映済みの設計文書の修正

本計画の作成時に、`approved` 済みの [02_architecture.md](02_architecture.md) の記述と食い違う4点が判明した。いずれも 2026-07-30 にレビュアーの承認を得て設計文書へ反映済みであり、本計画は反映後の記述に従う。本節は追跡のための記録であり、実装時に参照すべき内容は設計文書の該当節にある。

| # | 修正した箇所 | 修正の内容 |
|---|---|---|
| 1 | §3.2、§3.9 | `WithPermissionCheckUIDPolicy` の配置を `test_helpers_policy.go`（`//go:build test`）に統一した。§3.9 の責務表では `policy.go` の行に含まれていたが、§5.5 の「本番バイナリにこの関数自体が存在しないことでコンパイル時に呼び出しを不可能にする」という論拠は `//go:build test` 側を前提とする |
| 2 | §4.1、§3.3、§4.2、§6.2 | `ErrInvalidPermissionCheckUIDPolicy` を追加した。§3.3 が求める `PolicyUnset` および範囲外の値に対するエラーを `errors.Is` で判別できるようにするためである。既存の `ErrPermissionCheckUIDPolicyConflict` は「設定済みの値と異なる値を設定しようとした」条件専用であり、入力値そのものが不正な条件とは意味が異なる |
| 3 | §7.2（4行目） | `safefileio` 経由の検証内容を、実施可能な形へ具体化した。`internal/safefileio` を変更しない方針（§2.2）のため `osFS` へインスタンス方針付きの `GroupMembership` を注入できず、また非 root 環境では基準UIDの値から方針を区別できない（§3.5）。検証対象を「インスタンス方針を持たずプロセス既定方針へ委譲すること」に改めた |
| 4 | §3.9、§8 | §3.9 の `test_helpers_policy.go` の責務に、他パッケージのテストから非公開処理を呼ぶための2つのラッパー（有効な方針の取得、基準UID解決）を加えた。また AC-14 / AC-15 をフェーズ2からフェーズ1へ移した。未指定時に `RealUIDOnly` が適用されることは、型とプロセス既定方針が存在した時点で検証できるためである |

---

## 2. 実装ステップ

### 2.1 ステップ 1-1 = フェーズ1: 型・オプション・プロセス既定方針の追加

設計: [02_architecture.md](02_architecture.md) §3.1、§3.2、§3.3、§3.4.1、§4.1、§6.2

**変更ファイル**

- 新規: `internal/groupmembership/policy.go`
- 新規: `internal/groupmembership/test_helpers_policy.go`（`//go:build test`）
- 変更: `internal/groupmembership/manager.go`

**作業内容**

- [x] `policy.go` に `PermissionCheckUIDPolicy`（基底型 `int32`）と定数 `PolicyUnset` / `RealUIDOnly` / `SudoUIDAware` を `iota` で定義する。各定数の doc コメントは、設計書 §3.1 の日本語コメントの内容を英語に訳して記す。とくに `SudoUIDAware` については「`SUDO_UID` は数値としての妥当性しか検査しておらず、この方針は当該バイナリを root として起動できる者が基準UIDを任意に指定できることを受け入れる」という前提を必ず含める
- [x] `policy.go` に非公開定数 `finalDefaultPermissionCheckUIDPolicy = RealUIDOnly` を定義する
- [x] `policy.go` に `String() string` を実装する。戻り値は `PolicyUnset` → `"unset"`、`RealUIDOnly` → `"real-uid-only"`、`SudoUIDAware` → `"sudo-uid-aware"`、それ以外 → `fmt.Sprintf("unknown(%d)", int32(p))`
- [x] `policy.go` に `ErrPermissionCheckUIDPolicyConflict = errors.New("process-wide permission check UID policy is already set to a different value")` を定義する
- [x] `policy.go` に `ErrInvalidPermissionCheckUIDPolicy = errors.New("invalid permission check UID policy")` を定義する（設計書 §4.1）
- [x] `policy.go` に `type Option func(*GroupMembership)` を定義する
- [x] `policy.go` にプロセス既定方針の保持変数を `atomic.Int32` として定義する。ゼロ値が `PolicyUnset` と一致することを doc コメントで明記する
- [x] `policy.go` に `SetProcessPermissionCheckUIDPolicy(p PermissionCheckUIDPolicy) error` を実装する。処理順は (1) `p` が `RealUIDOnly` / `SudoUIDAware` のいずれでもなければ `fmt.Errorf("%w: %s", ErrInvalidPermissionCheckUIDPolicy, p)` を返す（`PolicyUnset` もこの検査で弾かれる）、(2) 現在値が `p` と同じなら `nil` を返す、(3) 現在値が `PolicyUnset` 以外なら `fmt.Errorf("%w: current=%s, requested=%s", ErrPermissionCheckUIDPolicyConflict, current, p)` を返す、(4) `CompareAndSwap` で `PolicyUnset` から `p` へ設定する。(4) が失敗した場合は現在値の再読み取り、すなわち (2) から再試行する
- [x] `policy.go` に `ProcessPermissionCheckUIDPolicy() PermissionCheckUIDPolicy` を実装する（atomic ロードのみ）
- [x] `policy.go` に非公開メソッド `(gm *GroupMembership) effectivePermissionCheckUIDPolicy() PermissionCheckUIDPolicy` を実装する。設計書 §3.4.1 の順位表どおりに解決する。プロセス既定方針の読み取りは atomic 変数を直接触らず `ProcessPermissionCheckUIDPolicy()` を経由する（§1.3 の 6.(a)）。想定外の値に対する panic やデフォルトケースは設けない（設計書 §3.4.1 末尾）
- [x] `manager.go` の `GroupMembership` 構造体に `policy PermissionCheckUIDPolicy` フィールドを追加する
- [x] `manager.go` の `New()` を `New(opts ...Option) *GroupMembership` へ変更し、既存フィールドの初期化後に各オプションを適用する
- [x] `test_helpers_policy.go` に `WithPermissionCheckUIDPolicy(p PermissionCheckUIDPolicy) Option` を実装する。doc コメント（英語）には、インスタンス方針がプロセス既定方針より優先されること、およびテスト専用であること（本番の宣言は `SetProcessPermissionCheckUIDPolicy` を使う）を記す
- [x] `test_helpers_policy.go` に `SwapProcessPermissionCheckUIDPolicy(p PermissionCheckUIDPolicy) (restore func())` を実装する。検証を経ずにプロセス既定方針を `p` へ書き換え、呼び出し前の値へ戻す関数を返す。呼び出し側は `t.Cleanup(SwapProcessPermissionCheckUIDPolicy(...))` の形で使う（このヘルパー自身は `testing` を import しないため、`*testing.T` を受け取らない設計にする）。doc コメント（英語）に「この関数を使うテストは `t.Parallel()` を呼んではならない。プロセス全体で共有される状態を書き換えるためである」旨を明記する（§1.3 の 7.）
- [x] `test_helpers_policy.go` に `(gm *GroupMembership) EffectivePermissionCheckUIDPolicy() PermissionCheckUIDPolicy` を実装する（非公開メソッドへの委譲のみ）。他パッケージのテストから有効な方針を検査するための入口である

**完了条件**

- [x] `make fmt` / `make test` / `make lint` が成功する
- [x] `go vet -tags 'test integration performance' ./...` が成功する（`//go:build test` を含む新規ファイルの型・シグネチャ不整合をこの PR で検出するため。3 タグを並べるのは、§1.3 の 8. で確認した3種のテスト実行構成すべてを検査対象に含めるためである）

### 2.2 ステップ 1-2 = フェーズ1: 方針の型とプロセス既定方針の単体テスト

設計: [02_architecture.md](02_architecture.md) §7.1（型と生成 API、方針の解決順序、最終既定方針、設定の一度きり）、§7.3（2点目）、§7.4

**変更ファイル**

- 新規: `internal/groupmembership/policy_test.go`

**作業内容**

- [x] `TestPermissionCheckUIDPolicy_String` を追加する。`PolicyUnset` / `RealUIDOnly` / `SudoUIDAware` の `String()` がそれぞれ `"unset"` / `"real-uid-only"` / `"sudo-uid-aware"` を返し、3 値が相互に異なることを検証する。範囲外の値 `PermissionCheckUIDPolicy(99)` が `"unknown(99)"` を返すことも検証する（AC-01）
- [x] `TestSetProcessPermissionCheckUIDPolicy` を追加する。設計書 §6.2 の4行を、それぞれ独立に実行できるサブテストとして検証する。**各サブテストが自身の開始状態を自ら確立する**（`go test -run` で単独実行しても成立させるため）。(a) `t.Cleanup(SwapProcessPermissionCheckUIDPolicy(PolicyUnset))` の下で `RealUIDOnly` を設定 → `nil` かつ `ProcessPermissionCheckUIDPolicy()` が `RealUIDOnly`、(b) `t.Cleanup(SwapProcessPermissionCheckUIDPolicy(RealUIDOnly))` の下で `RealUIDOnly` を再設定 → `nil` かつ値が変わらない、(c) `t.Cleanup(SwapProcessPermissionCheckUIDPolicy(RealUIDOnly))` の下で `SudoUIDAware` を設定 → `errors.Is(err, ErrPermissionCheckUIDPolicyConflict)` かつ値が `RealUIDOnly` のまま、(d) `t.Cleanup(SwapProcessPermissionCheckUIDPolicy(PolicyUnset))` の下で `PolicyUnset` と `PermissionCheckUIDPolicy(99)` を設定 → `errors.Is(err, ErrInvalidPermissionCheckUIDPolicy)` かつ値が `PolicyUnset` のまま（AC-14）
- [x] `TestSetProcessPermissionCheckUIDPolicy_Concurrent` を追加する。`t.Cleanup(SwapProcessPermissionCheckUIDPolicy(PolicyUnset))` の下で、複数の goroutine が `RealUIDOnly` と `SudoUIDAware` を混在させて `SetProcessPermissionCheckUIDPolicy` を呼び、別の goroutine が `ProcessPermissionCheckUIDPolicy()` を読む構成にする。検証項目は (1) 返るエラーはすべて `errors.Is(err, ErrPermissionCheckUIDPolicyConflict)` である、(2) 最終値が要求された2値のいずれかである、(3) `PolicyUnset` 以外の値が一度観測されたあと、その値が変わらない。`SetProcessPermissionCheckUIDPolicy` の CAS 再試行経路と `-race` 下での競合の不在を、この1件で検証する（AC-14、要件書 Success Criteria の `go test -race`）
- [x] `TestEffectivePermissionCheckUIDPolicy_Precedence` を追加する。`policy_test.go` は `internal/groupmembership` パッケージ内（`manager_test.go` と同じ white-box）に置くため、`gm.effectivePermissionCheckUIDPolicy()` を直接呼ぶ（`test_helpers_policy.go` の公開ラッパー `EffectivePermissionCheckUIDPolicy` は他パッケージのテスト専用であり、本テストからは使わない）。`t.Cleanup(SwapProcessPermissionCheckUIDPolicy(SudoUIDAware))` の下で、(a) `New(WithPermissionCheckUIDPolicy(RealUIDOnly))` の有効な方針が `RealUIDOnly`（インスタンス方針がプロセス既定方針に優先する）、(b) `New()` の有効な方針が `SudoUIDAware`（プロセス既定方針に従う）ことを検証する（AC-02）
- [x] `TestEffectivePermissionCheckUIDPolicy_FinalDefault` を追加する。同じく `gm.effectivePermissionCheckUIDPolicy()` を直接呼ぶ。`t.Cleanup(SwapProcessPermissionCheckUIDPolicy(PolicyUnset))` の下で `New()` の有効な方針が `RealUIDOnly` であることを検証する。プロセス既定方針が未設定でも `SudoUIDAware` が選ばれないことの確認である（AC-14、AC-15、設計書 §7.3 の2点目）
- [x] 上記 5 テストのいずれにも `t.Parallel()` を書かない（設計書 §7.4）

**完了条件**

- [x] `make test` が成功し、上記 5 テストがすべて pass する
- [x] `go test -tags test -race -run 'TestSetProcessPermissionCheckUIDPolicy' ./internal/groupmembership/` が成功する
- [x] 各サブテストが `go test -tags test -run '<テスト名>/<サブテスト名>' ./internal/groupmembership/` で単独実行しても pass する
- [x] `make lint` が成功する

### PR-1 作成ポイント: internal groupmembership policy type & process-wide default

**対象ステップ**: 1-1 / 1-2

**推奨タイトル**: `feat(0160): add explicit permission-check UID policy type`

**レビュー観点**: プロセス既定方針の CAS 設定・再試行ロジックの正しさ / atomic 操作の `-race` 下での並行安全性 / doc コメントの英訳が設計書 §3.1 の趣旨（`SudoUIDAware` の信頼前提を含む）を保っているか / 既存 `New()` 呼び出し14箇所（本番4・テスト9・パッケージ内1）への影響がないこと

**実装モデル要件**: frontier-recommended

**判定理由**: ステップ 1-1 の `SetProcessPermissionCheckUIDPolicy` は CAS 再試行を伴う並行実装であり、ステップ 1-2 で `-race` 下の並行テスト（`TestSetProcessPermissionCheckUIDPolicy_Concurrent`）を追加する、孤立した並行処理ステップである

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#945](https://github.com/isseis/go-safe-cmd-runner/pull/945)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### 2.3 ステップ 2-1 = フェーズ2: 基準UID解決への方針分岐の導入

設計: [02_architecture.md](02_architecture.md) §3.4.2、§3.5、§6.1

**変更ファイル**

- 変更: `internal/groupmembership/manager.go`
- 変更: `internal/groupmembership/test_helpers_policy.go`
- 変更: `internal/groupmembership/manager_test.go`

**作業内容**

- [x] `manager.go` に非公開定数 `sudoUIDEnvVar = "SUDO_UID"` を `parseSudoUID` の近くに追加する
- [x] `getPermissionCheckUID()` をパッケージ関数から `(gm *GroupMembership) getPermissionCheckUID() (int, error)` へ変更する。本体は `getProcessRealUID()` で実 UID を得たのち `resolvePermissionCheckUID(gm.effectivePermissionCheckUIDPolicy(), realUID, os.Getenv)` を返す形にする
- [x] `getPermissionCheckUID` の doc コメントを書き換える。現行の「When running under sudo (the real UID is 0 and SUDO_UID is set), it returns the original user's UID taken from SUDO_UID.」という無条件の記述を改め、基準UIDは有効な方針に従って解決すること、および `SUDO_UID` を読むのは `SudoUIDAware` のときだけであることを記す
- [x] `resolvePermissionCheckUID` のシグネチャを `resolvePermissionCheckUID(policy PermissionCheckUIDPolicy, realUID int, getenv func(string) string) (int, error)` へ変更し、本体を設計書 §6.1 のフローどおりに実装する。環境変数の取得は `getenv(sudoUIDEnvVar)` で行う
- [x] `resolvePermissionCheckUID` の doc コメントを、新しい引数（方針・環境変数取得関数）と、`RealUIDOnly` では環境変数取得関数を呼ばないことを含む内容へ書き換える
- [x] `parseSudoUID` の doc コメント 1 行「This is separated from getPermissionCheckUID to allow independent testing.」を「This is separated from resolvePermissionCheckUID to allow independent testing.」へ変更する。関数本体とエラー文面は変更しない
- [x] `CanCurrentUserSafelyReadFile` 内の `getPermissionCheckUID()` の呼び出しを `gm.getPermissionCheckUID()` へ変更する
- [x] `test_helpers_policy.go` に `ResolvePermissionCheckUID(policy PermissionCheckUIDPolicy, realUID int, getenv func(string) string) (int, error)` を追加する（非公開の `resolvePermissionCheckUID` への委譲のみ）。`cmd/*` のテストから基準UID解決を検証するための入口である。**この追加はステップ 2-1 に置く。**`resolvePermissionCheckUID` の新シグネチャに依存するため、フェーズ1に置くとフェーズ1の時点でコンパイルできない
- [x] `manager_test.go::TestGetPermissionCheckUID` の重複するサブテスト 3 件（`normal user without sudo`、`with SUDO_UID empty returns os.Getuid`、`with SUDO_UID set returns appropriate UID`）を、`returns real UID under the final default policy` の 1 件へ統合する。内容は `t.Setenv("SUDO_UID", "9999")` の下で `New().getPermissionCheckUID()` が `os.Getuid()` を返すことの検証とする。最終既定方針が `RealUIDOnly` であるため、これら3件は統合後は同一の主張になる
- [x] `manager_test.go::TestGetPermissionCheckUID` のサブテスト `simulated sudo environment for non-root user` を削除する。最終既定方針の下では実 UID によらず結果が実 UID になるため、非 root 限定の分岐に意味がなくなる
- [x] `manager_test.go::TestGetPermissionCheckUID` に `reads SUDO_UID from the real environment under SudoUIDAware` を追加する。`gm := New(WithPermissionCheckUIDPolicy(SudoUIDAware))` と `t.Setenv("SUDO_UID", "9999")` の下で、`os.Getuid() == 0` なら `gm.getPermissionCheckUID()` が `9999`、それ以外なら `os.Getuid()` を返すことを検証する。これは `getPermissionCheckUID` が `os.Getenv` を実際に渡していること、および `sudoUIDEnvVar` の値が正しいことを実環境に対して確かめる唯一のテストであり、削除する既存サブテストの不変条件（§1.3 の「更新が必要な既存テスト」）を引き継ぐ
- [x] `manager_test.go::TestResolvePermissionCheckUID`（:713-759）を削除する。検証していた不変条件は `policy_test.go` の新規テストが引き継ぐ（§4.5）
- [x] `manager_test.go::TestGetPermissionCheckUID` の `parseSudoUID` を直接呼ぶサブテスト 3 件は変更しない

**完了条件**

- [x] `make fmt` / `make test` / `make lint` が成功する
- [x] `rg -n 'os\.Getenv\("SUDO_UID"\)' -g '*.go'` の一致が 0 件である
- [x] `make deadcode` の出力に `internal/groupmembership/policy.go` を含む行が現れない（§1.3 の 6.）

### 2.4 ステップ 2-2 = フェーズ2: 基準UID解決の単体テスト

設計: [02_architecture.md](02_architecture.md) §7.1（基準UID解決の純粋関数、環境変数取得関数の呼び出し回数）、§7.3（1点目）

**変更ファイル**

- 変更: `internal/groupmembership/policy_test.go`

**作業内容**

- [x] `TestResolvePermissionCheckUID_RealUIDOnly` を追加する。表駆動で実 UID `0` / `1000` × `SUDO_UID` 値 `""` / `"0"` / `"1000"`（有効値）/ `"4294967295"`（`math.MaxUint32`）/ `"abc"`（数値でない）/ `"-1"`（負数）/ `"4294967296"`（`math.MaxUint32` 超過）/ 数字 300 桁の文字列 の全 16 組み合わせについて、常にエラーなく実 UID が返ることを検証する（AC-04、AC-12、設計書 §7.3 の1点目）
- [x] `TestResolvePermissionCheckUID_SudoUIDAware` を追加する。表駆動で設計書 §3.4.2 / AC-13 の表の全行を検証する。期待値は行ごとに指定する。実 UID `0`: `""` → `0`、`"0"` → `0`、`"1000"` → `1000`、`"4294967295"` → `4294967295`、`"-1"` → `errors.Is(err, ErrSudoUIDOutOfRange)`、`"4294967296"` → `errors.Is(err, ErrSudoUIDOutOfRange)`、`"abc"` → `errors.Is(err, strconv.ErrSyntax)`。実 UID `1000`: `""` / `"2000"` / `"abc"` / `"-1"` のいずれでもエラーなく `1000`。エラーは文字列一致ではなく `errors.Is` で判定する（AC-05、AC-06、AC-13）
- [x] `TestResolvePermissionCheckUID_EnvAccess` を追加する。呼び出された環境変数名を記録し呼び出し回数を数える環境変数取得関数を渡す。実 UID `0` かつ有効値を返す条件で、(a) `RealUIDOnly` では呼び出し回数が 0、(b) `SudoUIDAware` では 1 以上でありかつ記録された環境変数名が `"SUDO_UID"` であることを検証する。両者を同一テストで対比させ、実 UID が 0 でないために読まれなかった状態と区別する（AC-03、AC-09、設計書 §3.5）
- [x] 上記 3 テストはプロセス既定方針を参照しないため、`SwapProcessPermissionCheckUIDPolicy` を使わない

**完了条件**

- [x] `make test` が成功し、上記 3 テストが pass する
- [x] `resolvePermissionCheckUID` の設計書 §6.1 のフロー上の全分岐（方針が `SudoUIDAware` でない／実 UID が 0 でない／`SUDO_UID` が空／解析成功／解析失敗）が、上記 3 テストのいずれかで通過している
- [x] `make lint` が成功する

### 2.5 ステップ 3-1 = フェーズ3: 3バイナリでの方針宣言

設計: [02_architecture.md](02_architecture.md) §3.7、§4.2

**変更ファイル**

- 変更: `cmd/runner/main.go`
- 変更: `cmd/record/main.go`
- 変更: `cmd/verify/main.go`
- 変更: `.golangci.yml`

**作業内容**

- [x] `.golangci.yml` の `depguard` の `main` ルールの `allow` リストへ `github.com/isseis/go-safe-cmd-runner/internal/groupmembership` を追加する（§1.3 の 1.）。追加位置は同リスト内の `internal/filevalidator` の直後とし、既存の並びに合わせる。この変更は同ステップの `cmd/*` の import 追加と同一コミットに含める
- [x] `cmd/runner/main.go` の既存の `init()`（:55）の末尾に、`SetProcessPermissionCheckUIDPolicy(groupmembership.RealUIDOnly)` の呼び出しを追加する。エラーが返った場合は panic する（設計書 §4.2）。panic メッセージは `fmt.Sprintf` で組み立て、宣言しようとした方針と `ProcessPermissionCheckUIDPolicy()` の現在値を `%s` で含める（§1.3 の 6.(b)）。この宣言は最終既定方針と同じ値であり、挙動を変えるためではなく意図を明示するために置く。その趣旨を英文コメント 1 行で添える
- [x] `cmd/record/main.go` に `init()` を新設し、`SetProcessPermissionCheckUIDPolicy(groupmembership.SudoUIDAware)` を呼ぶ。エラー時は `runner` と同じ形式の panic とする
- [x] `cmd/verify/main.go` に `init()` を新設し、`SetProcessPermissionCheckUIDPolicy(groupmembership.SudoUIDAware)` を呼ぶ。エラー時は `runner` と同じ形式の panic とする
- [x] 3 ファイルの `init()` に、宣言の根拠（`runner` は setuid バイナリを一般ユーザーが起動する運用のため sudo 経由を想定しない／`record` と `verify` は `sudo` 実行が想定運用のため呼び出し元ユーザー視点の判定を維持する）を英文コメントで記す

**完了条件**

- [x] `make build` が成功する
- [x] `make lint` が成功する（`depguard` の追加が効いていることの確認になる）
- [x] `make fmt` / `make test` が成功する

### 2.6 ステップ 3-2 = フェーズ3: バイナリごとの宣言の実行時検証テスト

設計: [02_architecture.md](02_architecture.md) §7.2、§3.6

**変更ファイル**

- 変更: `cmd/runner/main_test.go`
- 変更: `cmd/record/main_test.go`
- 変更: `cmd/verify/main_test.go`

**作業内容**

- [x] `cmd/runner/main_test.go` に `TestRunnerDeclaresRealUIDOnlyPolicy` を追加する。(a) `groupmembership.ProcessPermissionCheckUIDPolicy()` が `groupmembership.RealUIDOnly` を返すこと、(b) その方針の下で `groupmembership.ResolvePermissionCheckUID(groupmembership.ProcessPermissionCheckUIDPolicy(), 0, func(string) string { return "1000" })` が `0` をエラーなく返すことを検証する。(b) は、テストバイナリの `init()` が実際に宣言した方針の下で、実 UID 0 の状況でも `SUDO_UID` が採用されないことを確認するものである（AC-07、AC-08、AC-09）
- [x] `cmd/record/main_test.go` に `TestRecordDeclaresSudoUIDAwarePolicy` を追加する。(a) `groupmembership.ProcessPermissionCheckUIDPolicy()` が `groupmembership.SudoUIDAware` を返すこと、(b) `groupmembership.ResolvePermissionCheckUID(groupmembership.ProcessPermissionCheckUIDPolicy(), 0, func(string) string { return "1000" })` が `1000` をエラーなく返すことを検証する。(b) の期待値は変更前の `resolvePermissionCheckUID(0, "1000")` の結果と同一であり、この検証が AC-11 の回帰確認にあたる（AC-10、AC-11）
- [x] `cmd/verify/main_test.go` に `TestVerifyDeclaresSudoUIDAwarePolicy` を追加する。内容は `cmd/record` と同じ（AC-10、AC-11）
- [x] 3 テストはいずれもプロセス既定方針を読み取るだけで書き換えない（設計書 §7.4 の3点目）。`t.Parallel()` は書かない

**完了条件**

- [x] `make test` が成功し、上記 3 テストが pass する
- [x] `make lint` が成功する

### 2.7 ステップ 3-3 = フェーズ3: `safefileio` 経由の方針委譲の検証

設計: [02_architecture.md](02_architecture.md) §7.2（4行目とそれに続く説明）、§2.2

**変更ファイル**

- 変更: `internal/safefileio/safe_file_test.go`

**作業内容**

- [x] `TestGroupMembershipFollowsProcessPermissionCheckUIDPolicy` を追加する。`NewFileSystem(FileSystemConfig{}).GetGroupMembership()` とパッケージ変数 `defaultFS.GetGroupMembership()` の両方について、2 つのサブテストで検証する。(a) `t.Cleanup(groupmembership.SwapProcessPermissionCheckUIDPolicy(groupmembership.PolicyUnset))` で未設定状態を明示的に確立した下で `EffectivePermissionCheckUIDPolicy()` が `groupmembership.RealUIDOnly` を返すこと、(b) `t.Cleanup(groupmembership.SwapProcessPermissionCheckUIDPolicy(groupmembership.SudoUIDAware))` の下で `groupmembership.SudoUIDAware` を返すこと。各サブテストが自身の開始状態を自ら確立するため、単独実行でも成立する。`safefileio` が生成する `GroupMembership` がインスタンス方針を持たずプロセス既定方針へ委譲することの確認であり、`cmd/*` 側の宣言検証（ステップ 3-2）と組み合わせて AC-07 / AC-10 を成立させる
- [x] 本テストに `t.Parallel()` は書かない（プロセス既定方針を書き換えるため。`internal/safefileio` はパッケージ全体で `t.Parallel()` を禁じている。`safe_file_linux.go:120`）

**完了条件**

- [x] `make test` が成功する
- [x] `go test -tags test -race ./internal/safefileio/` が成功する
- [x] `make lint` が成功する

### PR-2 作成ポイント: policy-based UID resolution and per-binary declaration

**対象ステップ**: 2-1 / 2-2 / 3-1 / 3-2 / 3-3

**推奨タイトル**: `feat(0160): resolve permission-check UID via declared per-binary policy`

**レビュー観点**: `sudo runner` の挙動が厳しい側へ変わること（設計書 §5.3）の妥当性 / `cmd/runner` `cmd/record` `cmd/verify` 3バイナリで宣言漏れがないこと（`.golangci.yml` の `depguard` 変更と import 追加が同一コミットであることを含む） / `manager_test.go::TestGetPermissionCheckUID` のサブテスト統合・削除がカバレッジを落としていないか（§4.5 の引き継ぎ表） / `safefileio` 経由の委譲検証が非 root 環境でも AC-07/AC-10 を成立させる設計になっているか（§7.2） / レビュー時はステップ 2-1/2-2（internal パッケージの解決ロジック変更）とステップ 3-1/3-2/3-3（3バイナリの宣言・実行時検証）をコミット単位で分けて順に読むこと（単一 PR だがレビュー負荷を下げるため）

**実装モデル要件**: frontier-recommended

**判定理由**: ステップ 2-1（読み取り安全性判定の中核である `resolvePermissionCheckUID` への方針分岐導入）とステップ 3-1（3本の本番バイナリの `init()` への方針宣言追加、`.golangci.yml` の同時変更を伴う）は、それぞれが孤立した高リスク・複雑ステップであり（frontier-recommended の「孤立した高リスク/複雑ステップ」基準に該当）、かつ設計書 §8 の指示によりこの2フェーズを別 PR に分割できないため、両方を含む本 PR 全体を frontier-recommended とする。なお §3.1 のマイルストーン表が M2 の全体リスクを「中」と評価しているのは sudo 実行時の機能退行という影響範囲の評価であり、上記のステップ単位の実装複雑度評価とは別軸である

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（[#946](https://github.com/isseis/go-safe-cmd-runner/pull/946)）
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### 2.8 ステップ 4-1 = フェーズ4: 利用者向け文書（日本語版）の更新

設計: [02_architecture.md](02_architecture.md) §5.3

**変更ファイル**

- 変更: `docs/user/runner_command.ja.md`

**作業内容**

- [ ] `### 6.2 実行時エラー` の `#### 権限エラー`（見出しは :1833、本文は次の `####` 見出しの直前まで）の**節末**、すなわち次の `#### ファイル検証エラー` 見出しの直前に、`#### sudo 経由で起動した場合のファイル読み取り拒否` を新設する。記載内容は次の 3 点とする。(1) `sudo runner` で起動した場合、ファイル読み取り可否の判定に用いる UID が呼び出し元ユーザーから 0（root）へ変わる、(2) その結果、グループ書き込み可能なファイルについて、root がそのグループの構成員でなければ読み取りが拒否されうる、(3) 想定される運用（`install -m 4755` した `runner` を一般ユーザーが起動する形態）および root の cron から直接起動する形態では、この変化の影響を受けない
- [ ] 対処法として、`sudo runner` ではなく setuid ビット付き `runner` を一般ユーザーとして起動する形態を用いる旨を記す
- [ ] `## 目次` は `##` 見出しのみを列挙しているため更新しない

**完了条件**

- [ ] `rg -n 'sudo runner' docs/user/runner_command.ja.md` が新設節に一致する
- [ ] 記載内容が設計書 §5.3 の記述と矛盾しない（§7 の AC-16 の検証手順で確認する）

### 2.9 ステップ 4-2 = フェーズ4: 利用者向け文書（英語版）と `CHANGELOG.md`

**変更ファイル**

- 変更: `docs/user/runner_command.md`
- 変更: `CHANGELOG.md`

**作業内容**

- [ ] ステップ 4-1 のコミット後に `/mktrans` を `docs/user/runner_command.md` に対して実行し、新設節を英語版へ反映する。日本語版を直接英訳して両方を同時に編集することはしない
- [ ] `CHANGELOG.md` の `## [Unreleased]` の `### Changed` に、`sudo runner` 利用者向けの破壊的挙動変更として英語で記載する。内容は (1) `runner` の読み取り判定が `SUDO_UID` を参照しなくなったこと、(2) `sudo runner` では基準UIDが呼び出し元ユーザーから 0 へ変わり、グループ書き込み可能なファイルの読み取りが拒否されうること、(3) `install -m 4755` した `runner` を一般ユーザーが起動する想定運用は影響を受けないこと、(4) `record` / `verify` の挙動は変わらないこと
- [ ] 記載した 4 点を、`cmd/runner/main.go` の `init()` の宣言値、`cmd/record/main.go` / `cmd/verify/main.go` の `init()` の宣言値、`internal/groupmembership/manager.go` の `resolvePermissionCheckUID` の実装と読み合わせ、いずれも実装の挙動と一致することを確認する

**完了条件**

- [ ] `rg -c '^#### ' docs/user/runner_command.ja.md docs/user/runner_command.md` が両ファイルで 29 である（変更前はいずれも 28。§1.3 の 9.）
- [ ] 日本語版と英語版の新設節を並べて読み、記載する 3 点と対処法が対応していることを確認済みである
- [ ] `CHANGELOG.md` の 4 点が上記の読み合わせで実装と一致していることを確認済みである
- [ ] `make fmt` / `make test` / `make lint` が成功する

### 2.10 ステップ 4-3 = フェーズ4: セキュリティ設計文書の追随

**変更ファイル**

- 変更: `docs/dev/architecture_design/security-architecture.ja.md`
- 変更: `docs/dev/architecture_design/security-architecture.md`

**作業内容**

- [ ] `.ja.md:50` の括弧内の記述を、`record` が `SudoUIDAware` を宣言していることに基づく挙動として書き換える。現行の「権限チェックの基準UID（`getPermissionCheckUID`/`resolvePermissionCheckUID` が解決するUID。実UIDが0かつ`SUDO_UID`が有効な値であればその値を、それ以外は実UIDを採用）」を、「権限チェックの基準UID（`record` は基準UID決定方針として `SudoUIDAware` を宣言しているため、実UIDが0かつ`SUDO_UID`が有効な値であればその値を、それ以外は実UIDを採用する）」へ改める。関数名の列挙は、シグネチャが変わるため記述から外す
- [ ] `.ja.md:831` の `func New() *GroupMembership` を `func New(opts ...Option) *GroupMembership` へ変更する
- [ ] 上記 2 点を `/mktrans` で `security-architecture.md`（対応行は :50 と :834）へ反映する
- [ ] 書き換えた :50 の記述を、`cmd/record/main.go` の `init()` が宣言する方針（`SudoUIDAware`）と `internal/groupmembership/manager.go` の `resolvePermissionCheckUID` の実装と読み合わせ、記述が実装の挙動と一致することを確認する

**完了条件**

- [ ] `rg -n 'func New\(\) \*GroupMembership' docs/` の一致が 0 件である
- [ ] `rg -n 'getPermissionCheckUID' docs/dev/` の一致が 0 件である
- [ ] `rg -n 'func New\(opts \.\.\.Option\) \*GroupMembership' docs/dev/architecture_design/` が 2 件（日英各 1 件）一致する
- [ ] 書き換えた記述が上記の読み合わせで実装と一致していることを確認済みである

### 2.11 ステップ 4-4 = フェーズ4: 用語集の更新

**変更ファイル**

- 変更: `docs/translation_glossary.md`

**作業内容**

- [ ] 「基準UID | base UID」の行（:47）の直後に「基準UID決定方針 | base UID policy | 基準UIDの決定規則。`RealUIDOnly` と `SudoUIDAware` の2種（Task 0160）」の行を追加する
- [ ] 文書末尾の更新履歴表に「2026-07-30 | 基準UID決定方針の明示指定（Task 0160）関連の用語を追加 (base UID policy)」の行を追加する
- [ ] 追加した行の英語欄が、ステップ 4-2 / 4-3 で英語版文書に実際に書いた表現と一致していることを確認する

**完了条件**

- [ ] `rg -n '基準UID決定方針' docs/translation_glossary.md` が 1 件以上一致する
- [ ] 英語欄の表現が英語版文書の表現と一致していることを確認済みである

### PR-3 作成ポイント: user and security documentation updates

**対象ステップ**: 4-1 / 4-2 / 4-3 / 4-4

**推奨タイトル**: `docs(0160): document sudo runner base-UID policy change`

**レビュー観点**: 日本語版と英語版の新設節・修正箇所の記述内容が対応しているか / `CHANGELOG.md` の4点が `cmd/*/main.go` の `init()` 宣言および `resolvePermissionCheckUID` の実装と一致しているか / `security-architecture.ja.md`/`.md` :50 の書き換えが `record` の `SudoUIDAware` 宣言に基づく記述として正確か / 用語集の英語欄が英語版文書中の実際の表現と一致しているか

**実装モデル要件**: standard

**判定理由**: コード変更を伴わない文書更新のみであり、frontier-required・frontier-recommended のいずれのトリガー（未知の設計判断、パネルモード対象、競合する実装アプローチ、孤立した高リスク/並行性ステップ）にも該当しない

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

フェーズの名前と順序、および各フェーズの対応 AC は設計書 §8 の実装優先順位表に一致させている。

### 3.1 マイルストーン

| マイルストーン | PR | 対象ステップ | 成果物 | リスク |
|---|---|---|---|---|
| M1 | PR-1 | 1-1、1-2 | `policy.go`、`test_helpers_policy.go`、`policy_test.go`。方針の型・プロセス既定方針・最終既定方針とその単体テスト | 小。既存の判定経路に触れないため挙動は変わらない |
| M2 | PR-2 | 2-1、2-2、3-1、3-2、3-3 | 方針分岐を入れた `resolvePermissionCheckUID` とその単体テスト、3バイナリの宣言、`depguard` 設定、実行時検証テスト | 中。`sudo runner` の挙動が変わる唯一のマイルストーン |
| M3 | PR-3 | 4-1、4-2、4-3、4-4 | 利用者向け文書（日英）、`CHANGELOG.md`、セキュリティ設計文書（日英）、用語集 | 小。コードを変更しない |

M1（PR-1）は単独でマージできる。M1 の成果物は既存の判定経路から呼ばれないが、既定方針の解決だけは `CanCurrentUserSafelyReadFile` から到達するため、`make deadcode` の報告は増えない（§1.3 の 6.）。M2（PR-2）は設計書 §8 の指示によりフェーズ2とフェーズ3を分割せず 1 つの PR にまとめる（フェーズ2のみをマージすると、最終既定方針が適用されて `record` / `verify` が sudo 実行時に機能退行する）。M3（PR-3）は M2 の後に実施する。文書に書く挙動が M2 のマージによって初めて事実になるためである。

マイルストーン M{n} と PR-{n} は常に1対1で対応するよう定義している。今後 PR 構成を変更する場合は、本節と §3.2 の両方を同時に更新し、両者を乖離させないこと。

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1-1 / 1-2 | `internal/groupmembership` に方針の型・プロセス既定方針・最終既定方針とその単体テストを追加 | frontier-recommended |
| PR-2 | 2-1 / 2-2 / 3-1 / 3-2 / 3-3 | 基準UID解決へ方針分岐を導入し、3バイナリで方針を宣言、`depguard` 設定・実行時検証テスト・`safefileio` 経由の委譲検証を追加 | frontier-recommended |
| PR-3 | 4-1 / 4-2 / 4-3 / 4-4 | 利用者向け文書（日英）、`CHANGELOG.md`、セキュリティ設計文書（日英）、用語集を更新 | standard |

### 3.3 前提関係

- ステップ 1-2 はステップ 1-1 の型・プロセス既定方針・`SwapProcessPermissionCheckUIDPolicy` を前提とする。
- ステップ 2-1 はステップ 1-1 の `effectivePermissionCheckUIDPolicy` を前提とする。
- ステップ 2-2 はステップ 2-1 の `resolvePermissionCheckUID` の新シグネチャを前提とする。
- ステップ 3-2 はステップ 2-1 の `ResolvePermissionCheckUID`（テスト専用の公開ラッパー）とステップ 3-1 の宣言を前提とする。テスト専用ラッパーをフェーズ1ではなくフェーズ2に置くのは、委譲先の非公開関数のシグネチャがフェーズ2で変わるためである。フェーズ1に置くと、その時点ではコンパイルできない。
- ステップ 3-3 はステップ 1-1 の `EffectivePermissionCheckUIDPolicy` / `SwapProcessPermissionCheckUIDPolicy` を前提とする。
- ステップ 3-1 の `.golangci.yml` 変更は、同ステップの `cmd/*` の import 追加と同一コミットに含める（片方だけでは `make lint` が失敗する）。
- ステップ 4-2 はステップ 4-1 のコミットを前提とする（`/mktrans` の運用）。ステップ 4-3 の英語版反映も同様である。ステップ 4-4 はステップ 4-2 / 4-3 で確定した英語表現を前提とする。

---

## 4. テスト戦略

設計書 §7 の方針に従う。本節は「どのテストをどう書くか」に絞る。

### 4.1 カバレッジ目標

数値目標は設けず、次の分岐網羅を条件とする。

- `policy.go` に追加する全公開関数・メソッド（`String`、`SetProcessPermissionCheckUIDPolicy`、`ProcessPermissionCheckUIDPolicy`）と非公開メソッド `effectivePermissionCheckUIDPolicy` の全分岐。`SetProcessPermissionCheckUIDPolicy` については設計書 §6.2 の 4 行に加えて CAS 再試行経路も含む。
- `resolvePermissionCheckUID` の設計書 §6.1 のフロー上の全分岐（方針が `SudoUIDAware` でない／実 UID が 0 でない／`SUDO_UID` が空／解析成功／解析失敗）。
- 3 バイナリそれぞれの `init()` が宣言した方針（`cmd/*/main_test.go` の 3 テスト）。

### 4.2 新規テスト

| テスト | 場所 | 目的 |
|---|---|---|
| `TestPermissionCheckUIDPolicy_String` | `internal/groupmembership/policy_test.go` | 3 定数の定義と相互の区別、および範囲外値の表示を固定する（AC-01） |
| `TestSetProcessPermissionCheckUIDPolicy` | 同 | 設計書 §6.2 の 4 行（未設定からの設定、同値の再設定、異値の競合、不正値）を、単独実行可能なサブテストとして固定する（AC-14） |
| `TestSetProcessPermissionCheckUIDPolicy_Concurrent` | 同 | CAS 再試行経路と、`-race` 下でのプロセス既定方針の競合の不在を固定する（AC-14） |
| `TestEffectivePermissionCheckUIDPolicy_Precedence` | 同 | インスタンス方針 > プロセス既定方針の順位を固定する（AC-02） |
| `TestEffectivePermissionCheckUIDPolicy_FinalDefault` | 同 | 無指定時に `RealUIDOnly` が適用されることを固定する（AC-14、AC-15） |
| `TestResolvePermissionCheckUID_RealUIDOnly` | 同 | 実 UID 2 種 × `SUDO_UID` 8 種の全組み合わせで実 UID が返ることを固定する（AC-04、AC-12、設計書 §7.3） |
| `TestResolvePermissionCheckUID_SudoUIDAware` | 同 | AC-13 の表の全行を、行ごとの期待 sentinel 付きで固定する。境界値（`math.MaxUint32` とその超過、負数、数値でない値）とエラー経路を含む（AC-05、AC-06、AC-13） |
| `TestResolvePermissionCheckUID_EnvAccess` | 同 | `RealUIDOnly` では環境変数取得関数が呼ばれず、`SudoUIDAware` では `"SUDO_UID"` を引数に呼ばれることを対比で固定する（AC-03、AC-09） |
| `TestRunnerDeclaresRealUIDOnlyPolicy` | `cmd/runner/main_test.go` | `runner` のテストバイナリで実行時にプロセス既定方針が `RealUIDOnly` であり、実 UID 0 でも `SUDO_UID` が採用されないことを固定する（AC-07、AC-08、AC-09） |
| `TestRecordDeclaresSudoUIDAwarePolicy` | `cmd/record/main_test.go` | `record` の宣言と、実 UID 0 + 有効な `SUDO_UID` の解決結果が変更前と一致することを固定する（AC-10、AC-11） |
| `TestVerifyDeclaresSudoUIDAwarePolicy` | `cmd/verify/main_test.go` | 同上（AC-10、AC-11） |
| `TestGroupMembershipFollowsProcessPermissionCheckUIDPolicy` | `internal/safefileio/safe_file_test.go` | `NewFileSystem` 経由の `GroupMembership` と `defaultFS` の `GroupMembership` がプロセス既定方針へ委譲することを固定する（AC-07、AC-10） |

### 4.3 既存テストの更新

`internal/groupmembership/manager_test.go::TestGetPermissionCheckUID` を次のように整理する（ステップ 2-1）。

- 最終既定方針の下で同一の主張になるサブテスト 3 件を 1 件（`returns real UID under the final default policy`）へ統合する。
- 非 root 限定の分岐が無意味になるサブテスト `simulated sudo environment for non-root user` を削除する。
- `reads SUDO_UID from the real environment under SudoUIDAware` を追加する。純粋関数のテストがすべて環境変数取得関数を注入するため、実環境の `os.Getenv` が実際に渡されていることと `sudoUIDEnvVar` の値の正しさを確かめるのはこのサブテストだけである。
- `parseSudoUID` を直接呼ぶサブテスト 3 件は変更しない。

### 4.4 テストヘルパーの追加

新規ヘルパーファイルは `internal/groupmembership/test_helpers_policy.go` の 1 つのみである。テスト構成ガイドのクラスB（パッケージ内部ヘルパー、`test_helpers_<category>.go`、`//go:build test`）に従う。クラスB を選ぶ理由は、`WithPermissionCheckUIDPolicy` が非公開フィールド `policy` を設定し、`SwapProcessPermissionCheckUIDPolicy` が非公開のプロセス既定方針変数を書き換え、`ResolvePermissionCheckUID` と `EffectivePermissionCheckUIDPolicy` が非公開関数・メソッドへ委譲するためで、いずれも `testutil/` サブディレクトリからは実装できない。既存の `internal/groupmembership/test_helpers.go`（`newWithEnumerator`）は変更しない。

このファイルは `_test.go` ではないため `.golangci.yml` の `exclusions` が適用されず、全 lint ルールの対象になる（§1.3 の 2.）。

### 4.5 削除するテストのカバレッジ引き継ぎ

`manager_test.go::TestResolvePermissionCheckUID`（:713-759）と、`TestGetPermissionCheckUID` のサブテスト 4 件を整理する。検証していた不変条件と引き継ぎ先の対応は次のとおりであり、失われるカバレッジはない。

| 削除・統合するサブテスト | 検証内容 | 引き継ぎ先 |
|---|---|---|
| `TestResolvePermissionCheckUID/sudoUID empty returns realUID` | 実 UID 0 / 1000 × `SUDO_UID` 空 → 実 UID | `policy_test.go::TestResolvePermissionCheckUID_SudoUIDAware` の該当 2 行 |
| `TestResolvePermissionCheckUID/realUID 0 and valid SUDO_UID returns SUDO_UID value` | 実 UID 0 × `"1000"` → 1000 | 同テストの該当行、および `cmd/record`・`cmd/verify` の AC-11 検証 |
| `TestResolvePermissionCheckUID/realUID non-zero ignores SUDO_UID` | 実 UID 1000 × `"2000"` → 1000 | 同テストの該当行 |
| `TestResolvePermissionCheckUID/realUID 0 and invalid SUDO_UID returns error` | 実 UID 0 × 負数 / uint32 超過 / 数値でない値 → エラー | 同テストの不正値 3 行（行ごとの期待 sentinel 付き） |
| `TestGetPermissionCheckUID/with SUDO_UID set returns appropriate UID` | 実 UID が 0 のとき、実環境の `SUDO_UID=9999` が採用される | `TestGetPermissionCheckUID/reads SUDO_UID from the real environment under SudoUIDAware`（方針を `SudoUIDAware` に指定して同じ主張を保つ） |
| `TestGetPermissionCheckUID/normal user without sudo`、`/with SUDO_UID empty returns os.Getuid`、`/simulated sudo environment for non-root user` | 既定の方針で `os.Getuid()` が返る | `TestGetPermissionCheckUID/returns real UID under the final default policy` |

### 4.6 回帰確認

- 設計書 §3.2 のとおり `New()` の呼び出し 14 箇所は無修正でコンパイルが通り、最終既定方針 `RealUIDOnly` を継承する。これらを含む既存テストが無修正で pass することを回帰の裏づけとする。とくに `internal/safefileio/safe_file_test.go::TestCanSafelyReadFromFile`、`::TestSafeReadFileWithRelaxedPermissions`、`internal/groupmembership/manager_test.go::TestCanCurrentUserSafelyReadFile`（グループ書き込み可能ファイルの判定経路）を無修正での成功として確認する。
- `make unit-test` が `CGO_ENABLED=1 -race` と `CGO_ENABLED=0` の2構成で実行される（`Makefile:454-461`）ため、`go test -race` の要求は `make test` で満たされる。マージ前に CI の 2 レグ（`make test-ci-cgo1` / `make test-ci-cgo0`）の成功も確認する。
- `//go:build test` を含むファイルを追加するため、`go vet -tags 'test integration performance' ./...` を、ファイルを新設・変更するステップ（ステップ 1-1）の完了条件と、各マイルストーンのチェックリスト（§6.1、§6.2）に置く。3 タグを並べるのは、§1.3 の 8. で確認した3種のテスト実行構成すべてを検査対象に含めるためである。

### 4.7 テストで担保できない範囲

- **非 root 環境では、基準UIDの値そのものからは方針を区別できない。** `getProcessRealUID()` が `os.Getuid()` を直接呼び、本タスクではこれを変更しない（設計書 §3.5）。方針で結果が変わるのは実 UID 0 の行だけであるため、AC-04〜AC-06、AC-09、AC-11〜AC-13 は純粋関数 `resolvePermissionCheckUID` に対して検証する。`TestGetPermissionCheckUID/reads SUDO_UID from the real environment under SudoUIDAware` の実 UID 0 側の主張も、非 root の CI では実行されない。
- **インスタンス方針の悪用は AC-08 のテストで検出できない**（設計書 §5.5）。`WithPermissionCheckUIDPolicy` を `//go:build test` タグ付きファイルへ置き、本番コードからの呼び出しをコンパイル不可にすることで担保する。この担保は `make build`（本番タグ）の成功によって確認される。
- **本番タグでビルドしたバイナリを CI は実行しない**（設計書 §7.5）。`make integration-test` は `test-ci` に含まれず sudo と Slack webhook を要する。最終既定方針があるため宣言漏れでも起動不能にはならず、宣言漏れ自体はステップ 3-2 のテストで検出する。

---

## 5. リスク管理

| リスク | 影響 | 緩和 |
|---|---|---|
| `depguard` の設定変更を忘れる | `cmd/*` の import 追加で `make lint` が失敗し、原因が分かりにくい | ステップ 3-1 の最初の作業項目に置き、`.golangci.yml` の変更と import 追加を同一コミットに含める（§3.3） |
| フェーズ2のみをマージする | `record` / `verify` が sudo 実行時に最終既定方針へ倒れ、グループ書き込み可能なファイルの読み取りが失敗する | 設計書 §8 の指示どおりフェーズ2とフェーズ3を同一 PR（PR-2 / M2）にまとめる |
| 本番コードが `os.Getenv` の代わりに空文字列を返す関数を渡す実装ミス | 純粋関数のテストはすべて偽の環境変数取得関数を注入するため全件 pass し、`sudo record` / `sudo verify` が基準UID 0 へ静かに退行する | `TestGetPermissionCheckUID/reads SUDO_UID from the real environment under SudoUIDAware` で実環境の値を経由した結果を検証し、`TestResolvePermissionCheckUID_EnvAccess` で要求された環境変数名が `"SUDO_UID"` であることを検証する（§4.3、§4.5） |
| プロセス既定方針の退避・復元がテスト間で漏れる | 後続テストが意図しない方針で走り、失敗が別のテストに現れる | `SwapProcessPermissionCheckUIDPolicy` は復元関数を返す形にし、呼び出し側は必ず `t.Cleanup` へ渡す。各サブテストが自身の開始状態を自ら確立する。該当テストに `t.Parallel()` を書かず、その制約をヘルパーの doc コメントにも書く（§1.3 の 7.） |
| `sudo runner` 利用者への周知が漏れる | 読み取り拒否が原因不明の機能退行として現れる | ステップ 4-1（利用者向け文書）とステップ 4-2（`CHANGELOG.md`）を PR-3（M3）の必須成果物とする |
| テスト専用の公開関数が `make deadcode` に現れる | 到達不能コードとして誤って削除される | `effectivePermissionCheckUIDPolicy` が `ProcessPermissionCheckUIDPolicy()` を経由し、エラー本文と panic メッセージが `String()` を参照する構成にする（§1.3 の 6.）。ステップ 2-1 の完了条件で `policy.go` 由来の報告が 0 件であることを確認する |
| セキュリティ設計文書の記述が実装と乖離したまま残る | `record` の読み取り安全性チェックの説明が、方針に依らない挙動として読まれ続ける | ステップ 4-3 を PR-3（M3）の成果物に含め、完了条件に `rg` による不在確認と実装との読み合わせを置く |
| 実装中に設計書との新たな食い違いが判明する | 承認済み設計と矛盾した実装が、指摘されるまで気付かれない | §1.4 と同じ扱いにする。本計画の側で読み替えて済ませず、設計書の改訂をレビュアーに諮り、承認後に設計書と本計画の双方へ反映する |

---

## 6. 実装チェックリスト

### 6.1 PR-1（M1、ステップ 1-1、1-2: 型とプロセス既定方針）

- [ ] `policy.go` を新規作成した（型・3 定数・`String()`・最終既定方針・2 エラー・`Option`・プロセス既定方針の設定/取得・`effectivePermissionCheckUIDPolicy`）
- [ ] `test_helpers_policy.go` を新規作成した（`WithPermissionCheckUIDPolicy`・`SwapProcessPermissionCheckUIDPolicy`・`EffectivePermissionCheckUIDPolicy`）
- [ ] `manager.go` の `New` を可変長引数化し `policy` フィールドを追加した
- [ ] `policy_test.go` の 5 テストを追加した
- [ ] `make fmt` / `make test` / `make lint` / `go vet -tags 'test integration performance' ./...` が成功する
- [ ] 各サブテストが単独実行で pass する
- [ ] PR-1 マージ済み（対象ステップ: 1-1 / 1-2）

### 6.2 PR-2（M2、ステップ 2-1、2-2、3-1、3-2、3-3: 方針分岐と宣言）

- [ ] `manager.go` の `getPermissionCheckUID` をメソッド化し、`resolvePermissionCheckUID` に方針と環境変数取得関数を導入した
- [ ] `manager.go` の 3 関数（`getPermissionCheckUID`・`resolvePermissionCheckUID`・`parseSudoUID`）の doc コメントを更新した
- [ ] `test_helpers_policy.go` に `ResolvePermissionCheckUID` を追加した
- [ ] `manager_test.go::TestGetPermissionCheckUID` を §4.3 のとおり整理した（3 件統合・1 件削除・1 件追加）
- [ ] `manager_test.go::TestResolvePermissionCheckUID` を削除した
- [ ] `policy_test.go` に解決テスト 3 件を追加した
- [ ] `.golangci.yml` の `depguard` の `main` ルールに `internal/groupmembership` を追加した
- [ ] `cmd/runner/main.go` の `init()` で `RealUIDOnly` を宣言した
- [ ] `cmd/record/main.go` に `init()` を新設し `SudoUIDAware` を宣言した
- [ ] `cmd/verify/main.go` に `init()` を新設し `SudoUIDAware` を宣言した
- [ ] `cmd/runner/main_test.go::TestRunnerDeclaresRealUIDOnlyPolicy` を追加した
- [ ] `cmd/record/main_test.go::TestRecordDeclaresSudoUIDAwarePolicy` を追加した
- [ ] `cmd/verify/main_test.go::TestVerifyDeclaresSudoUIDAwarePolicy` を追加した
- [ ] `internal/safefileio/safe_file_test.go::TestGroupMembershipFollowsProcessPermissionCheckUIDPolicy` を追加した
- [ ] `make build` / `make fmt` / `make test` / `make lint` / `go vet -tags 'test integration performance' ./...` が成功する
- [ ] `make deadcode` の出力に `internal/groupmembership/policy.go` を含む行が現れない
- [ ] PR-2 マージ済み（対象ステップ: 2-1 / 2-2 / 3-1 / 3-2 / 3-3）

### 6.3 PR-3（M3、ステップ 4-1〜4-4: 文書）

- [ ] `docs/user/runner_command.ja.md` の `#### 権限エラー` 節末に `#### sudo 経由で起動した場合のファイル読み取り拒否` を新設した
- [ ] `/mktrans` で `docs/user/runner_command.md` に反映した
- [ ] `CHANGELOG.md` の `[Unreleased]` / `### Changed` に記載し、4 点を実装と読み合わせた
- [ ] `security-architecture.ja.md` の :50 と :831 を更新し、:50 の記述を実装と読み合わせた
- [ ] `/mktrans` で `security-architecture.md` に反映した
- [ ] `docs/translation_glossary.md` に「基準UID決定方針」と更新履歴の行を追加し、英語欄を英語版文書と照合した
- [ ] `rg -c '^#### ' docs/user/runner_command.ja.md docs/user/runner_command.md` が両ファイルで 29 である
- [ ] PR-3 マージ済み（対象ステップ: 4-1 / 4-2 / 4-3 / 4-4）

---

## 7. 受入基準の検証

種別は `test`（実行可能テスト）、`static`（`rg` / コンパイル）、`manual`（目視確認）を表す。

| AC | 種別 | 検証手段 | 期待結果 |
|---|---|---|---|
| AC-01 | test | `internal/groupmembership/policy_test.go::TestPermissionCheckUIDPolicy_String` | pass。`RealUIDOnly` / `SudoUIDAware` / `PolicyUnset` の 3 定数が定義され相互に異なる |
| AC-02 | test | `internal/groupmembership/policy_test.go::TestEffectivePermissionCheckUIDPolicy_Precedence` | pass。`New(WithPermissionCheckUIDPolicy(RealUIDOnly))` の有効な方針がプロセス既定方針 `SudoUIDAware` を上書きする |
| AC-03 | test | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_EnvAccess` | pass。環境変数取得関数が引数として渡され、方針によって呼ばれる／呼ばれないが分かれ、要求される環境変数名が `"SUDO_UID"` である |
| AC-03 | static | `rg -n 'os\.Getenv\("SUDO_UID"\)' -g '*.go'` | 一致 0 件（変更前は `internal/groupmembership/manager.go:450` の 1 件が一致する） |
| AC-04 | test | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_RealUIDOnly` | pass。実 UID 0 × `SUDO_UID` 有効値の行を含め、全 16 組み合わせで実 UID が返る |
| AC-05 | test | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_SudoUIDAware` | pass。実 UID 0 × 有効値 → その値、それ以外 → 実 UID |
| AC-05 | test | `internal/groupmembership/manager_test.go::TestGetPermissionCheckUID`（サブテスト `reads SUDO_UID from the real environment under SudoUIDAware`） | pass。実環境の `SUDO_UID` を経由した結果が同じ規則に従う（root 実行時のみ実 UID 0 側を検証する） |
| AC-06 | test | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_SudoUIDAware` | pass。実 UID 0 × 不正値 3 種 → 行ごとに指定した sentinel（`ErrSudoUIDOutOfRange` / `strconv.ErrSyntax`）でエラー、実 UID 非 0 × 不正値 → エラーなく実 UID |
| AC-07 | test | `cmd/runner/main_test.go::TestRunnerDeclaresRealUIDOnlyPolicy` | pass。`runner` のテストバイナリでプロセス既定方針が `RealUIDOnly` |
| AC-07 | test | `internal/safefileio/safe_file_test.go::TestGroupMembershipFollowsProcessPermissionCheckUIDPolicy` | pass。`NewFileSystem` 経由と `defaultFS` の `GroupMembership` がプロセス既定方針へ委譲する |
| AC-08 | test | `cmd/runner/main_test.go::TestRunnerDeclaresRealUIDOnlyPolicy` | pass。実行時に `ProcessPermissionCheckUIDPolicy()` を検査し、静的検索は用いない |
| AC-09 | test | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_EnvAccess` | pass。実 UID 0 の条件で `RealUIDOnly` の呼び出し回数 0、`SudoUIDAware` は 1 以上 |
| AC-09 | test | `cmd/runner/main_test.go::TestRunnerDeclaresRealUIDOnlyPolicy` | pass。`runner` の宣言方針では実 UID 0 かつ `SUDO_UID=1000` でも 0 が返る |
| AC-10 | test | `cmd/record/main_test.go::TestRecordDeclaresSudoUIDAwarePolicy`、`cmd/verify/main_test.go::TestVerifyDeclaresSudoUIDAwarePolicy` | pass。両バイナリのテストバイナリでプロセス既定方針が `SudoUIDAware` |
| AC-10 | test | `internal/safefileio/safe_file_test.go::TestGroupMembershipFollowsProcessPermissionCheckUIDPolicy` | pass。`defaultFS` を含む生成箇所がプロセス既定方針へ委譲する |
| AC-11 | test | `cmd/record/main_test.go::TestRecordDeclaresSudoUIDAwarePolicy`、`cmd/verify/main_test.go::TestVerifyDeclaresSudoUIDAwarePolicy` | pass。宣言された方針の下で実 UID 0 かつ `SUDO_UID=1000` の解決結果が 1000（変更前の `resolvePermissionCheckUID(0, "1000")` と同一） |
| AC-12 | test | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_RealUIDOnly` | pass。実 UID 2 種 × `SUDO_UID` 8 種の全組み合わせ |
| AC-13 | test | `internal/groupmembership/policy_test.go::TestResolvePermissionCheckUID_SudoUIDAware` | pass。AC-13 の表の 6 行すべてに対応する行が存在し、境界値 `"4294967295"` / `"0"` の行も含む |
| AC-14 | test | `internal/groupmembership/policy_test.go::TestSetProcessPermissionCheckUIDPolicy`、`::TestSetProcessPermissionCheckUIDPolicy_Concurrent`、`::TestEffectivePermissionCheckUIDPolicy_FinalDefault` | pass。無指定時に `RealUIDOnly` が適用され、設定は一度きりであり、並行呼び出しでも値が入れ替わらない |
| AC-15 | test | `internal/groupmembership/policy_test.go::TestEffectivePermissionCheckUIDPolicy_FinalDefault` | pass。既定値案を採るため AC-15 後半（既定値が意図した側であることの固定）が適用される。`SudoUIDAware` は宣言なしには選ばれない |
| AC-16 | static | `rg -n 'sudo runner' docs/user/runner_command.ja.md docs/user/runner_command.md` | 両ファイルで新設節に一致する（変更前は `.ja.md` / `.md` ともに一致 0 件） |
| AC-16 | static | `rg -n -e 'グループ書き込み可能' -e 'group-writable' docs/user/runner_command.ja.md docs/user/runner_command.md` | 日本語版で「グループ書き込み可能」、英語版で "group-writable" が新設節に一致する |
| AC-16 | static | `rg -c '^#### ' docs/user/runner_command.ja.md docs/user/runner_command.md` | 両ファイルで 29（変更前はいずれも 28）。日英の双方に節が追加されたことを機械的に確認する |
| AC-16 | manual | 新設節の記述を設計書 §5.3 および実装（`cmd/runner/main.go` の `init()` の宣言値と `internal/groupmembership/manager.go` の `resolvePermissionCheckUID`）と読み合わせる | 記載の 3 点（基準UIDが 0 へ変わる／グループ書き込み可能なファイルの読み取りが拒否されうる／setuid 運用と root cron は影響を受けない）が実装の挙動と一致する |

`static` のみで足りる AC はない。AC-16 は文書の記述内容が対象であるため `static` と `manual` の組み合わせとし、記述の正しさは `manual` の読み合わせで担保する。AC に紐づかない文書変更（`CHANGELOG.md`、`security-architecture` 対、用語集）の内容確認は、ステップ 4-2 / 4-3 / 4-4 の完了条件に置いた読み合わせで行う。

---

## 8. 横断検索チェックリスト

`make lint` と `make test` では検出できない項目のみを挙げる。§7 に記載した `rg` は重複させない。

- [ ] `rg -n 'getPermissionCheckUID|resolvePermissionCheckUID' docs/ --glob '!docs/tasks/**'` — 一致 0 件であること。変更前は `docs/dev/architecture_design/security-architecture.ja.md:50` と同 `.md:50` の 2 件が一致し、いずれもシグネチャが変わる関数を挙動の説明に用いている（ステップ 4-3 で削除する）。`docs/tasks/**` は過去タスクの記録であり変更しない
- [ ] `rg -n 'func New\(\) \*GroupMembership' docs/` — 一致 0 件であること。変更前は `security-architecture.ja.md:831` と `security-architecture.md:834` の 2 件が一致する
- [ ] `rg -n '基準UID決定方針' docs/translation_glossary.md` — 1 件以上一致すること（用語集への登録漏れの検出）
- [ ] `rg -n 'RealUIDOnly|SudoUIDAware|PermissionCheckUIDPolicy' internal/ cmd/ -g '*.go' -g '!*_test.go' -g '!*test_helpers*.go'` — 一致するのは `internal/groupmembership/policy.go`、`internal/groupmembership/manager.go`、`cmd/runner/main.go`、`cmd/record/main.go`、`cmd/verify/main.go` の 5 ファイルのみであること。他の本番ファイルに方針の指定が漏れ出していないこと（とくに設計書 §3.10 が方針を渡さないと決めた `internal/security/dir_permissions_unix.go` と `internal/runner/runner.go`）を確認する
- [ ] `rg -n 'WithPermissionCheckUIDPolicy' -g '*.go' --files-with-matches` — 一致するファイルが `internal/groupmembership/test_helpers_policy.go` と `_test.go` のみであること。本番ファイルからの呼び出しはコンパイルできないが、`//go:build test` 付きの他ファイルからの呼び出しはコンパイルエラーとして現れないため、明示的に確認する
- [ ] `rg -n '\bAC-[0-9]+[a-z]?\b|\bF-[0-9]+[a-z]?\b' -g '*.go'` — 一致 0 件であること。受入基準の識別子を Go ソースへ持ち込まない（§1.2）

---

## 9. 成功基準

- [ ] AC-01〜AC-16 のすべてに対し、§7 の検証手段が実行され期待結果を満たしている
- [ ] 各 AC に少なくとも 1 つの `test` または `static` の検証が対応している
- [ ] `make fmt` / `make test` / `make lint` がグリーンである
- [ ] `make build` が成功する（本番タグで `WithPermissionCheckUIDPolicy` が存在しないことの確認を含む）
- [ ] `go vet -tags 'test integration performance' ./...` が成功する
- [ ] `make test-ci-cgo1` / `make test-ci-cgo0` が成功する（`go test -race` の要求を含む）
- [ ] `make deadcode` の出力に `internal/groupmembership/policy.go` を含む行が現れない
- [ ] `docs/user/runner_command` 対、`security-architecture` 対のそれぞれについて、日本語版と英語版の該当箇所を読み合わせ、記述が対応していることを確認済みである
- [ ] §4.1 のカバレッジ目標（3 項目の分岐網羅）を満たしている
- [ ] §4.6 に挙げた既存テストが無修正で pass している
- [ ] §8 の横断検索チェックリストの全項目が期待結果を満たしている
- [ ] `CHANGELOG.md` の `[Unreleased]` に `sudo runner` の挙動変化が記載され、記載内容が実装と読み合わせ済みである

---

## 10. 残作業

- [ ] §2 の全ステップの実装と、§3.1 / §3.2 の PR-1〜PR-3 の PR 作成・レビュー・マージ
- [ ] PR-2（M2）のマージ後に、`SUDO_UID` の値の検証（`user.LookupId` による実在確認）と利用の監査ログ記録（[#941](https://github.com/isseis/go-safe-cmd-runner/issues/941)）へ進む。設計書 §9 のとおり `SudoUIDAware` の解決処理の内側に閉じて追加でき、方針の型や伝播機構には影響しない
- [ ] [#941](https://github.com/isseis/go-safe-cmd-runner/issues/941) の完了後に、`runner` の native root 実行サポートの是非（[#921](https://github.com/isseis/go-safe-cmd-runner/issues/921)）を検討する。着手する場合、`cmd/runner/main.go` の宣言を変えるか起動形態に応じて切り替えることになる（設計書 §9）
