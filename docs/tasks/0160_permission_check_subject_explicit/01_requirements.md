# 要件定義書: 権限チェック主体を環境変数の推測から呼び出し元の明示指定へ変更

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-07-29 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#920 [Refactor][Security] 権限チェック主体の決定を環境変数の推測から呼び出し元の明示指定へ変える](https://github.com/isseis/go-safe-cmd-runner/issues/920)
- 詳細所見: [docs/tasks/0149_security_code_smell_audit_fable/findings/D1_groupmembership.md](../0149_security_code_smell_audit_fable/findings/D1_groupmembership.md) の M-3
- 関連（設計検討中に本 issue が判明した経緯）: [#864（0157）](../0157_dead_code_naming_cleanup/01_requirements.md)。0157 は命名・コメントの整合に留め、sudo 分岐そのものの是非を対象外としている。

## 背景

### 現状の実装

`internal/groupmembership/manager.go` の `getPermissionCheckUID`（445-452行目）は、実 UID が 0 のとき環境変数 `SUDO_UID` の値を無検証で権限チェックの主体として採用する（`resolvePermissionCheckUID` 470-476行目、`parseSudoUID` 487-496行目）。この関数は `CanCurrentUserSafelyReadFile`（304行目）からのみ呼ばれ、読み取り安全性チェック（`internal/safefileio/safe_file.go:445,492` から利用）の主体決定に使われる。

0149 監査の D1 M-3 は、この `SUDO_UID` が無検証であること自体を指摘している。root として起動できる者が `SUDO_UID=<任意 UID>` を設定すれば、読み取り安全性チェックを任意ユーザーの視点で通過させられる。root cron から `SUDO_UID` が残留した環境で実行される事故シナリオも挙げられている。

### 呼び出し元の運用差異

`internal/groupmembership` には `runner` / `record` / `verify` の3バイナリすべてが到達する（`go list -deps` で確認）が、想定される起動方法が異なる。

| バイナリ | 想定される起動方法 | 実 UID | sudo 分岐 |
|---|---|---|---|
| `runner` | root 所有 + setuid ビットのバイナリを一般ユーザーが起動（`docs/user/README.ja.md:590` の `install -m 4755`）。sudo 経由の実行は想定外 | 一般ユーザー | 発火しない |
| `record` | `sudo record -d ...`（`docs/user/README.ja.md:307`）。バイナリは非 setuid（`install -m 0755`） | 0 | 発火する |
| `verify` | `sudo verify -d ...`（`docs/user/verify_command.md:599`）。同上 | 0 | 発火する |

`record` / `verify` にとって sudo 分岐は意味がある。「呼び出し元ユーザーが本当にそのファイルを読めるのか」を検証しており、削除すると root 視点（＝何でも読める）に緩む。一方 `runner` にとっては、想定外の運用形態のための分岐が権限判定経路に居座っている状態であり、D1 M-3 の攻撃面をそのまま抱えている。

### 呼び出し経路の調査結果（2026-07-29 時点、`go list -deps` および `rg` で確認）

`groupmembership.New()` の生成箇所は本番コードに4箇所ある。

1. `internal/security/dir_permissions_unix.go:35`（`NewDirectoryPermChecker()` 内部）
2. `internal/runner/runner.go:301`
3. `internal/safefileio/safe_file.go:38`（`NewFileSystem()` 内部）
4. `internal/safefileio/testutil/mock.go:51`（テスト用モック、対象外）

このうち (1) `NewDirectoryPermChecker()` は `cmd/runner/main.go:343` / `cmd/record/main.go:104` / `cmd/verify/main.go:62` の3バイナリそれぞれの `main` から直接呼ばれており、引数を追加してバイナリごとに指定を分けることが容易である。

一方 (3) `safefileio.NewFileSystem(safefileio.FileSystemConfig{})` は、3バイナリ共通で到達する以下の7箇所から、バイナリの種類を区別する情報を持たないまま呼ばれている。

- `internal/dynamicanalysis/store.go:42`
- `internal/security/elfanalyzer/standard_analyzer.go:56`
- `internal/security/machoanalyzer/analyzer.go:26`
- `internal/runner/base/output/file.go:28`
- `internal/filevalidator/validator.go:241,344`
- `internal/verification/manager.go:475`
- `internal/logging/safeopen.go:34,75`

`go list -deps` で確認した通り、`dynamicanalysis` / `security/elfanalyzer` / `security/machoanalyzer` / `filevalidator` は `runner` / `record` / `verify` の3バイナリすべてから到達し、`logging` と `runner/base/output` は `runner` のみ、`verification` も `runner` のみから到達する。

したがって、issue #920 の提案が示す `groupmembership.New(WithPermissionCheckSubject(...))` という呼び出し変更を機械的に3バイナリの `main` にだけ適用しても、`safefileio.NewFileSystem` 経由で生成される `GroupMembership` インスタンスには反映されない。**主体指定をどの層まで明示的に伝播させるか（`FileSystemConfig` へのフィールド追加、各中間コンストラクタへの引数追加、あるいは別の注入機構）は、本要件定義書ではなく [02_architecture.md](02_architecture.md) で設計判断として決定する。**

## 目的

- 権限チェック主体の決定を、共有パッケージによる「呼び出し元の運用形態の環境変数からの推測」から、**呼び出し元による明示指定**へ変える。
- `runner` バイナリの権限判定経路から `os.Getenv("SUDO_UID")` の参照を排除し、D1 M-3 の攻撃面を `record` / `verify` のみに縮退させる。
- `record` / `verify` の sudo 分岐（呼び出し元ユーザーの視点での読み取り安全性チェック）は現状の挙動を維持する。
- 本タスクは **`SUDO_UID` の値自体を検証する対応（`user.LookupId` による実在確認や監査ログ記録など、D1 M-3 の別の推奨対応）を含まない**。値の解決主体をどのポリシーに基づいて決めるかを明示化するに留める。

## スコープ

### 対象

1. 権限チェック主体を表す型（`RealUIDOnly` / `SudoAware` の2値）を `internal/groupmembership` に定義する。
2. `GroupMembership` の生成 API に主体を明示指定する手段を追加する。
3. `getPermissionCheckUID` から `os.Getenv("SUDO_UID")` の直接参照を外し、指定された主体に従って解決する形にする。
4. `runner` バイナリから到達するすべての `GroupMembership` 生成箇所（`cmd/runner` 経由の直接・間接呼び出しを問わず）が `RealUIDOnly` を使用するようにする。「背景」節で列挙した、複数バイナリ共通の中間コンストラクタ（`safefileio.NewFileSystem` など）を経由する生成箇所も対象に含む。
5. `record` / `verify` バイナリから到達するすべての `GroupMembership` 生成箇所が `SudoAware` を使用するようにする。
6. オプション未指定時のデフォルト方針を決定し、テストで固定する。
7. 上記変更に対応するテストの追加・更新。

### 対象外

- `SUDO_UID` の値自体の検証（`user.LookupId` による実在確認、利用した事実と値の監査ログ記録）。D1 M-3 の残課題として別 issue で扱う。
- `runner` の native root 実行サポートの是非の検討（issue #920 に「本 issue の完了後に別途検討する」と明記されている後続課題）。
- `getProcessEUID` の命名・実装整合など、0157 で既に対応済みの D1 M-4 の内容。

## 検討事項（設計判断が必要な項目、02_architecture.md で決定する）

- **デフォルト方針**: オプション未指定時に `RealUIDOnly` と `SudoAware` のどちらを既定にするか。`RealUIDOnly` を既定にすると、オプション指定を忘れた `record` / `verify` の呼び出し元が「黙って緩む」方向ではなく「厳しくなる」方向に倒れるため安全側という考え方がある一方、`record` / `verify` が明示指定を怠った場合に本来必要な sudo 分岐が効かなくなり読み取りが失敗する退行にもなり得る。両立場のトレードオフを整理して決定する。
- **主体指定の伝播機構**: 「呼び出し経路の調査結果」で述べた、複数バイナリ共通の中間コンストラクタ（`safefileio.NewFileSystem` など計7箇所）へ主体指定をどう到達させるか。`FileSystemConfig` へのフィールド追加、各中間コンストラクタのシグネチャ変更、プロセス起動時に一度だけ設定される既定値機構など、複数の選択肢を比較検討する。

## Acceptance Criteria

#### F-001: 権限チェック主体を表す型とオプション指定 API の追加

`internal/groupmembership` パッケージに、権限チェック主体を表す型と、生成時にそれを明示指定する手段を追加する。

**Acceptance Criteria**:
- **AC-01**: `internal/groupmembership` パッケージに、権限チェック主体を表す型が定義されており、`RealUIDOnly` と `SudoAware` の少なくとも2つの値を持つ。
- **AC-02**: `GroupMembership` の生成 API から、権限チェック主体を明示的に指定できる。

#### F-002: `SUDO_UID` の参照を主体指定に基づく解決へ置き換え

`getPermissionCheckUID`（またはその後継関数）が環境変数 `SUDO_UID` を直接参照する構造をやめ、指定された主体に基づいて UID を解決する。

**Acceptance Criteria**:
- **AC-03**: 権限チェック主体の解決処理が `os.Getenv("SUDO_UID")` を直接呼び出さず、`GroupMembership` に指定された主体設定を経由して解決する。
- **AC-04**: 主体が `RealUIDOnly` の場合、実 UID が 0 かつ `SUDO_UID` が設定されていても、常に実 UID を権限チェックの主体として返す。
- **AC-05**: 主体が `SudoAware` の場合、実 UID が 0 かつ `SUDO_UID` が妥当な値であればその値を、それ以外は実 UID を権限チェックの主体として返す（現行の `resolvePermissionCheckUID` の挙動を維持する）。
- **AC-06**: 主体が `SudoAware` で `SUDO_UID` が不正な値（数値でない、範囲外など）の場合、現行と同様にエラーを返す。

#### F-003: `runner` バイナリの権限判定経路を `RealUIDOnly` に統一

`cmd/runner` から到達するすべての `GroupMembership` 生成箇所（直接・間接を問わず、複数バイナリ共通の中間コンストラクタ経由を含む）が `RealUIDOnly` を使用する。

**Acceptance Criteria**:
- **AC-07**: `cmd/runner` から到達する `GroupMembership` の生成箇所（本要件定義書「呼び出し経路の調査結果」に列挙した箇所を含む）がすべて `RealUIDOnly` を指定して生成される。
- **AC-08**: `runner` バイナリの実行経路に、`os.Getenv("SUDO_UID")` を読む分岐が到達しないことを静的に確認できる。

#### F-004: `record` / `verify` バイナリの権限判定経路を `SudoAware` に統一

`cmd/record` / `cmd/verify` から到達するすべての `GroupMembership` 生成箇所が `SudoAware` を使用し、現行の sudo 分岐の挙動を維持する。

**Acceptance Criteria**:
- **AC-09**: `cmd/record` / `cmd/verify` から到達する `GroupMembership` の生成箇所がすべて `SudoAware` を指定して生成される。
- **AC-10**: 実 UID=0 かつ有効な `SUDO_UID` が設定された環境で `record` / `verify` を実行した場合、変更前と同一の UID を権限チェックの主体として使用する（回帰がないことをテストで確認する）。

#### F-005: 両方針の組み合わせのテスト網羅

`RealUIDOnly` と `SudoAware` それぞれについて、実 UID が 0 / 0 以外、かつ `SUDO_UID` が未設定 / 有効値 / 不正値の組み合わせを検証する。

**Acceptance Criteria**:
- **AC-11**: `RealUIDOnly` を指定した場合の、実 UID が 0 / 0 以外 × `SUDO_UID` が未設定 / 設定済み（値の有効性を問わない）の全組み合わせについて、常に実 UID が返ることを検証するテストがある。
- **AC-12**: `SudoAware` を指定した場合の、実 UID が 0 / 0 以外 × `SUDO_UID` が未設定 / 有効値 / 不正値の全組み合わせについて、現行 `resolvePermissionCheckUID` と同一の挙動になることを検証するテストがある。

#### F-006: デフォルト方針の決定と固定

オプション未指定時のデフォルトの権限チェック主体を決定し、その挙動をテストで固定する。

**Acceptance Criteria**:
- **AC-13**: `02_architecture.md` の「検討事項」節で決定されたデフォルト方針（`RealUIDOnly` または `SudoAware`）が、オプション未指定時の `GroupMembership` の実際の挙動として実装され、テストで固定されている。

## Success Criteria（要件レベル）

- 上記すべての Acceptance Criteria が実装され、対応するテストが `make test` で成功する。
- `make lint` が警告なく通過する。
- `runner` バイナリの権限判定経路（`GroupMembership.CanCurrentUserSafelyReadFile` に到達する経路）に `os.Getenv("SUDO_UID")` の呼び出しが存在しないことを `rg` で確認できる。
- `record` / `verify` バイナリの sudo 対応読み取り安全性チェックについて、既存の外部から観測可能な挙動（実 UID=0 かつ有効な `SUDO_UID` 設定時の判定結果）に変化がない。
