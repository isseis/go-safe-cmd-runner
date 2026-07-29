# 要件定義書: 権限チェックの基準UIDの決定方針を環境変数の推測から呼び出し元の明示指定へ変更

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-29 |
| Review date | 2026-07-29 |
| Reviewer | isseis |
| Comments | - |

## 関連 Issue

- [#920 [Refactor][Security] 権限チェック主体の決定を環境変数の推測から呼び出し元の明示指定へ変える](https://github.com/isseis/go-safe-cmd-runner/issues/920)
- 詳細所見: [docs/tasks/0149_security_code_smell_audit_fable/findings/D1_groupmembership.md](../0149_security_code_smell_audit_fable/findings/D1_groupmembership.md) の M-3
- 関連（設計検討中に本 issue が判明した経緯）: [#864（0157）](../0157_dead_code_naming_cleanup/01_requirements.md)。0157 は命名・コメントの整合に留め、sudo 分岐そのものの是非を対象外としている。

## 背景

### 現状の実装

`internal/groupmembership/manager.go` の `getPermissionCheckUID`（445-452行目）は、実 UID が 0 のとき環境変数 `SUDO_UID` の値を無検証で権限チェックの基準UIDとして採用する（`resolvePermissionCheckUID` 470-476行目、`parseSudoUID` 487-496行目）。この関数は `CanCurrentUserSafelyReadFile`（304行目）からのみ呼ばれ、読み取り安全性チェック（`internal/safefileio/safe_file.go:445,492` から利用）の基準UID決定に使われる。

0149 監査の D1 M-3 は、この `SUDO_UID` が無検証であること自体を指摘している。root として起動できる者が `SUDO_UID=<任意 UID>` を設定すれば、読み取り安全性チェックを任意ユーザーの視点で通過させられる。root cron から `SUDO_UID` が残留した環境で実行される事故シナリオも挙げられている。

### 呼び出し元の運用差異

`internal/groupmembership` には `runner` / `record` / `verify` の3バイナリすべてが到達する（`go list -deps` で確認）が、想定される起動方法が異なる。

| バイナリ | 想定される起動方法 | 実 UID | sudo 分岐 |
|---|---|---|---|
| `runner` | root 所有 + setuid ビットのバイナリを一般ユーザーが起動（`docs/user/runner_command.ja.md:1690` の `install -m 4755`）。sudo 経由の実行は想定外 | 一般ユーザー | 発火しない |
| `record` | `sudo record -d ...`（`docs/user/README.ja.md:307`）。バイナリは非 setuid（`install -m 0755`） | 0 | 発火する |
| `verify` | `sudo verify -d ...`（`docs/user/verify_command.md:599`）。同上 | 0 | 発火する |

`record` / `verify` にとって sudo 分岐は意味がある。「呼び出し元ユーザーが本当にそのファイルを読めるのか」を検証しており、削除すると root 視点（＝何でも読める）に緩む。一方 `runner` にとっては、想定外の運用形態のための分岐が権限判定経路に居座っている状態であり、D1 M-3 の攻撃対象領域をそのまま抱えている。

### 呼び出し経路の調査結果（2026-07-29 時点、`go list -deps` および `rg` で確認）

#### 基準UID決定方針が実際に効く生成箇所

`groupmembership.New()` の生成箇所は本番コードに4箇所あるが、**そのうち権限チェックの基準UIDが実際に使われるのは1系統だけ**である。基準UIDを参照するのは `CanCurrentUserSafelyReadFile` のみであり（`getPermissionCheckUID` を呼ぶ唯一の関数）、その呼び出し元は `internal/safefileio/safe_file.go:445,492` の2箇所に限られる。

| 生成箇所 | 基準UIDを参照するか | 根拠 |
|---|---|---|
| `internal/safefileio/safe_file.go:38`（`NewFileSystem()` 内部） | する | `canSafelyAccessFile` / `canSafelyReadFromFile` が `CanCurrentUserSafelyReadFile` を呼ぶ |
| `internal/safefileio/safe_file.go:49`（`defaultFS` パッケージ変数） | する | 同上（`NewFileSystem` 経由） |
| `internal/security/dir_permissions_unix.go:35`（`NewDirectoryPermChecker()` 内部） | しない | 使うのは `CanUserSafelyWriteFile` のみ。書き込み判定は実 UID を直接受け取る |
| `internal/runner/runner.go:301` | しない | `security.NewValidator(..., WithGroupMembership(gmProvider))` に渡されるが、`security` パッケージから `CanCurrentUserSafelyReadFile` への呼び出しは存在しない |
| `internal/safefileio/testutil/mock.go:51` | テスト用モック | 対象外 |

この結果は設計方針に直結する。`NewDirectoryPermChecker()` は3バイナリそれぞれの `main`（`cmd/runner/main.go:343` / `cmd/record/main.go:104` / `cmd/verify/main.go:62`）から直接呼ばれるため基準UID決定方針をバイナリごとに渡しやすいが、**そこで渡した指定は一度も読まれない**。すなわち、指定しやすい場所は効かず、効く場所（`safefileio`）が指定しにくいという構図になっている。指定が効かない箇所へ基準UID決定方針を渡すことは、後から読む者に「ここで基準UID決定方針が効いている」と誤解させる死んだ設定になるため、避ける。

#### 効く系統がバイナリを区別できない理由

`safefileio.NewFileSystem(safefileio.FileSystemConfig{})` は、以下の箇所からバイナリの種類を区別する情報を持たないまま呼ばれている。

- `internal/dynamicanalysis/store.go:42`
- `internal/security/elfanalyzer/standard_analyzer.go:56`
- `internal/security/machoanalyzer/analyzer.go:26`
- `internal/runner/base/output/file.go:28`
- `internal/filevalidator/validator.go:241,344`
- `internal/verification/manager.go:475`
- `internal/logging/safeopen.go:34,75`
- `cmd/record/main.go:52,55,154`

`go list -deps` で確認した通り、`dynamicanalysis` / `security/elfanalyzer` / `security/machoanalyzer` / `filevalidator` は `runner` / `record` / `verify` の3バイナリすべてから到達し、`logging` と `runner/base/output` と `verification` は `runner` のみから到達する。

さらに `internal/safefileio/safe_file.go:49` の `var defaultFS = NewFileSystem(FileSystemConfig{})` は**パッケージ初期化時、すなわち `main` が動き出す前に生成される**。この `defaultFS` はパッケージ関数 `safefileio.SafeReadFile` の実体であり、以下から利用されている。

- `internal/filevalidator/validator.go:1452`（3バイナリすべてから到達）
- `internal/runner/config/loader.go:127`
- `internal/fileanalysis/file_analysis_store.go:109,161`

したがって、issue #920 の提案が示す `groupmembership.New(WithPermissionCheckSubject(...))` という呼び出し変更を3バイナリの `main` に適用するだけでは足りないばかりか、`defaultFS` に対してはコンストラクタ引数を足す方式そのものが成立しない（`main` より前に生成が完了しているため）。**基準UID決定方針の指定をどの層までどう伝播させるかは、本要件定義書ではなく [02_architecture.md](02_architecture.md) で設計判断として決定する。ただし `defaultFS` を扱えることは設計案の必須条件とする。**

### `sudo runner` 実行時の挙動変化

`runner` を `RealUIDOnly` に切り替えると、`sudo runner` で起動された場合（実 UID=0 かつ `SUDO_UID` 設定済み）の権限チェックの基準UIDが、呼び出し元ユーザーから UID 0 へ変わる。

この変化は緩む方向ではなく、**厳しくなる方向**である。読み取り判定はグループ書き込み可能なファイルについて「基準UIDがそのファイルのグループに属するか」を見るが、root は通常そのグループの構成員ではないため、現在は通っているファイルが拒否されうる。したがって想定される影響は権限の抜け穴ではなく機能退行である。

なお root の cron から `runner` を直接起動する場合は `SUDO_UID` が設定されないため、現状すでに UID 0 が基準UIDであり、本変更による挙動変化はない。変化するのは `sudo runner` の形態のみである。

## 目的

- 権限チェックの基準UIDの決定を、共有パッケージによる「呼び出し元の運用形態の環境変数からの推測」から、**呼び出し元による明示指定**へ変える。
- `runner` バイナリの権限判定経路で `SUDO_UID` が参照されないようにし、D1 M-3 の攻撃対象領域を `record` / `verify` のみに縮退させる。
- `record` / `verify` の sudo 分岐（呼び出し元ユーザーの視点での読み取り安全性チェック）は現状の挙動を維持する。
- 本タスクは **`SUDO_UID` の値自体を検証する対応（`user.LookupId` による実在確認や監査ログ記録など、D1 M-3 の別の推奨対応）を含まない**。基準UIDをどのポリシーに基づいて解決するかを明示化するに留める。

### 残存リスク（本タスクで解消しないもの）

- `record` / `verify` では `SUDO_UID` が無検証で採用される状態が変わらない。本タスクは D1 M-3 を閉じるものではなく、影響範囲を3バイナリから2バイナリへ縮小するに留まる。
- そもそも `SUDO_UID` を任意の値に設定するには、当該バイナリを root として起動できる必要がある。したがって M-3 は権限昇格そのものではなく、多層防御の欠落および誤設定（root cron に `SUDO_UID` が残留するなど）への耐性の問題として扱う。この位置づけは、値の検証を別 issue へ送る判断の根拠でもある。

## スコープ

### 対象

1. 権限チェックの基準UID決定方針を表す型（`RealUIDOnly` / `SudoAware`、および未指定を表す値を設ける場合はそれ）を `internal/groupmembership` に定義する。
2. `GroupMembership` の生成 API に基準UID決定方針を明示指定する手段を追加する。
3. `getPermissionCheckUID` から `os.Getenv("SUDO_UID")` の直接参照を外し、指定された決定方針に従って解決する形にする。
4. `runner` バイナリの読み取り安全性チェック経路（`CanCurrentUserSafelyReadFile` に到達する経路）で `RealUIDOnly` が使われるようにする。「呼び出し経路の調査結果」で述べた `safefileio` 系統、およびパッケージ変数 `defaultFS` を対象に含む。
5. `record` / `verify` バイナリの同経路で `SudoAware` が使われるようにする。
6. 基準UID決定方針が明示指定されなかった場合の扱いを決定し、テストで固定する。
7. `sudo runner` 実行時の挙動変化（「`sudo runner` 実行時の挙動変化」節）を利用者向け文書に記載する。
8. 上記変更に対応するテストの追加・更新。

### 対象外

- `SUDO_UID` の値自体の検証（`user.LookupId` による実在確認、利用した事実と値の監査ログ記録）。D1 M-3 の残課題として別 issue で扱う。
- `runner` の native root 実行サポートの是非の検討（issue #920 に「本 issue の完了後に別途検討する」と明記されている後続課題）。
- `getProcessEUID` の命名・実装整合など、0157 で既に対応済みの D1 M-4 の内容。
- 基準UIDを参照しない生成箇所（`NewDirectoryPermChecker()`、`internal/runner/runner.go:301`）への基準UID決定方針指定の追加。効かない設定を足すことになるため行わない。
- `SUDO_USER` / `SUDO_GID` など `SUDO_UID` 以外の sudo 系環境変数の利用。現状も参照していない。

## 検討事項（設計判断が必要な項目、02_architecture.md で決定する）

- **未指定時の扱い**: 選択肢は「`RealUIDOnly` を既定にする」「`SudoAware` を既定にする」に加えて、**「型のゼロ値を不正値とし、未指定のまま使われたらエラーを返す」**の3つがある。前2者はいずれも、指定漏れが黙って別の判定ポリシーへ倒れる。`RealUIDOnly` 既定なら `record` / `verify` の指定漏れが読み取り失敗（機能退行）として現れ、`SudoAware` 既定なら `runner` の指定漏れが今回排除したはずの `SUDO_UID` 参照として残る。後者は指摘されるまで気付けない。第3案は指定漏れを実行時エラーとして即座に露見させる fail-closed 方針であり、本タスクの目的（推測をやめて明示指定にする）と最も整合する。一方で `defaultFS` のようにパッケージ初期化時に生成される実体があるため、第3案を採る場合は「生成はできるが利用前に設定が必要」というライフサイクルが成立するかを併せて検討する必要がある。
- **基準UID決定方針の伝播機構**: 「呼び出し経路の調査結果」で述べた `safefileio.NewFileSystem` 系統へ基準UID決定方針をどう到達させるか。`FileSystemConfig` へのフィールド追加、各中間コンストラクタのシグネチャ変更、プロセス起動時に一度だけ設定される既定値機構などを比較検討する。**`defaultFS` を扱えることが必須条件**であり、コンストラクタ引数のみの案は単独では成立しない。
- **プロセス全体の既定値機構を採る場合の安全性**: 設定が `main` の早期に一度だけ行われること、設定前の利用が検出できること、`go test -race` を含む並行実行で競合しないことをどう担保するか。
- **型の名称**: `SudoAware` は「sudo を認識する」と読めるが、実際には検証されていない環境変数を信頼するという意味である。名称またはドキュメントコメントで信頼前提を明示する。

## Acceptance Criteria

#### F-001: 権限チェックの基準UID決定方針を表す型とオプション指定 API の追加

`internal/groupmembership` パッケージに、権限チェックの基準UID決定方針を表す型と、生成時にそれを明示指定する手段を追加する。

**Acceptance Criteria**:
- **AC-01**: `internal/groupmembership` パッケージに、権限チェックの基準UID決定方針を表す型が定義されており、`RealUIDOnly` と `SudoAware` の少なくとも2つの値を持つ（未指定を表す第3の値を持つことは妨げない。F-006 を参照）。
- **AC-02**: `GroupMembership` の生成 API から、権限チェックの基準UID決定方針を明示的に指定できる。

#### F-002: `SUDO_UID` の参照を基準UID決定方針に基づく解決へ置き換え

`getPermissionCheckUID`（またはその後継関数）が環境変数 `SUDO_UID` を直接参照する構造をやめ、指定された決定方針に基づいて UID を解決する。

**Acceptance Criteria**:
- **AC-03**: 権限チェックの基準UIDの解決処理が `os.Getenv("SUDO_UID")` を直接呼び出さず、`GroupMembership` に指定された決定方針を経由して解決する。
- **AC-04**: 決定方針が `RealUIDOnly` の場合、実 UID が 0 かつ `SUDO_UID` が設定されていても、常に実 UID を権限チェックの基準UIDとして返す。
- **AC-05**: 決定方針が `SudoAware` の場合、実 UID が 0 かつ `SUDO_UID` が妥当な値であればその値を、それ以外は実 UID を権限チェックの基準UIDとして返す（現行の `resolvePermissionCheckUID` の挙動を維持する）。
- **AC-06**: 決定方針が `SudoAware` かつ実 UID が 0 で、`SUDO_UID` が不正な値（数値でない、負数、`math.MaxUint32` 超過）の場合、現行と同様にエラーを返す。実 UID が 0 以外の場合は `SUDO_UID` を解析しないため、不正な値でもエラーにならない（現行挙動の維持。AC-13 の表を参照）。

#### F-003: `runner` バイナリの読み取り判定経路を `RealUIDOnly` に統一

`cmd/runner` から到達する、権限チェックの基準UIDを参照する `GroupMembership`（`safefileio` 系統および `defaultFS`）が `RealUIDOnly` を使用する。

**Acceptance Criteria**:
- **AC-07**: `cmd/runner` から到達し、かつ基準UIDを参照する `GroupMembership` の生成箇所（`safefileio.NewFileSystem` 経由の各箇所およびパッケージ変数 `defaultFS`）が、すべて `RealUIDOnly` で動作する。
- **AC-08**: `runner` の依存グラフ上で使われる `GroupMembership` の基準UID決定方針が `RealUIDOnly` であることを、テストで実行時に検証できる。静的な `rg` 検索では検証しない（`SudoAware` の実装コードは共有パッケージ経由で `runner` バイナリにもリンクされるため、決定方針の違いは実行時設定であって静的な有無では表せない）。
- **AC-09**: 決定方針が `RealUIDOnly` のとき、`SUDO_UID` の読み取り自体が行われない。環境変数読み取りを差し替え可能にした上で、`RealUIDOnly` の判定中に一度も呼ばれないことをテストで検証する。

#### F-004: `record` / `verify` バイナリの読み取り判定経路を `SudoAware` に統一

`cmd/record` / `cmd/verify` から到達し基準UIDを参照する `GroupMembership` が `SudoAware` を使用し、現行の sudo 分岐の挙動を維持する。

**Acceptance Criteria**:
- **AC-10**: `cmd/record` / `cmd/verify` から到達し、かつ基準UIDを参照する `GroupMembership`（`defaultFS` を含む）が、すべて `SudoAware` で動作する。
- **AC-11**: 実 UID=0 かつ有効な `SUDO_UID` が設定された環境で `record` / `verify` を実行した場合、変更前と同一の UID を権限チェックの基準UIDとして使用する（回帰がないことをテストで確認する）。

#### F-005: 両方針の組み合わせのテスト網羅

`RealUIDOnly` と `SudoAware` それぞれについて、実 UID が 0 / 0 以外、かつ `SUDO_UID` が未設定 / 有効値 / 不正値の組み合わせを検証する。

**Acceptance Criteria**:
- **AC-12**: `RealUIDOnly` を指定した場合の、実 UID が 0 / 0 以外 × `SUDO_UID` が未設定 / 設定済み（値の有効性を問わない）の全組み合わせについて、常に実 UID が返ることを検証するテストがある。
- **AC-13**: `SudoAware` を指定した場合の、実 UID が 0 / 0 以外 × `SUDO_UID` が未設定 / 有効値 / 不正値の全組み合わせについて、下表の期待値を検証するテストがある。期待値は現行 `resolvePermissionCheckUID` の挙動に一致するが、リファクタ後に同関数が残らない可能性があるため、参照ではなく表として固定する。

  | 実 UID | `SUDO_UID` | 期待結果 |
  |---|---|---|
  | 0 | 未設定（空文字列） | 0 |
  | 0 | 有効値 `N` | `N` |
  | 0 | 不正値（数値でない、負数、`math.MaxUint32` 超過） | エラー |
  | 非 0 | 未設定 | 実 UID |
  | 非 0 | 有効値 | 実 UID |
  | 非 0 | 不正値 | 実 UID（エラーにしない） |

#### F-006: 基準UID決定方針未指定時の扱いの決定と固定

基準UID決定方針が明示指定されなかった場合の扱いを決定し、その挙動をテストで固定する。

**Acceptance Criteria**:
- **AC-14**: `02_architecture.md` の「検討事項」節で決定された未指定時の方針が、`GroupMembership` の実際の挙動として実装され、テストで固定されている。
- **AC-15**: 未指定を不正とする方針を採る場合、決定方針を設定せずに読み取り安全性チェックを実行すると、判定を通すことなくエラーを返す（fail-closed であることをテストで検証する）。既定値を持つ方針を採る場合は、その既定値が意図した側であることをテストで固定する。

#### F-007: 挙動変化の文書化

`sudo runner` 実行時の権限チェックの基準UIDの変化を利用者向け文書に記載する。

**Acceptance Criteria**:
- **AC-16**: `sudo runner` で起動した場合に権限チェックの基準UIDが呼び出し元ユーザーから UID 0 へ変わること、およびその結果グループ書き込み可能なファイルの読み取りが拒否されうることが、利用者向け文書に記載されている。日本語版を先に更新し、英語版は `/mktrans` で反映する。

## Success Criteria（要件レベル）

- 上記すべての Acceptance Criteria が実装され、対応するテストが `make test` で成功する。
- `make lint` が警告なく通過する。
- `go test -race` が成功する（プロセス全体の既定値機構を採る場合の競合検出のため）。
- `runner` の依存グラフ上で使われる `GroupMembership` の基準UID決定方針が `RealUIDOnly` であり、その判定経路で `SUDO_UID` が読まれないことがテストで確認できる。
- `record` / `verify` の sudo 対応読み取り安全性チェックについて、既存の外部から観測可能な挙動（実 UID=0 かつ有効な `SUDO_UID` 設定時の判定結果）に変化がない。
