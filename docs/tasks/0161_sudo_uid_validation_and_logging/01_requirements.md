# 要件定義書: `SUDO_UID` の実在確認と採用事実の記録

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-30 |
| Review date | 2026-07-30 |
| Reviewer | isseis |
| Comments | - |

## 関連 Issue

- [#941 [Security] SUDO_UID の値を検証し、利用した事実を記録する（D1 M-3 残課題）](https://github.com/isseis/go-safe-cmd-runner/issues/941)
- 詳細所見: [docs/tasks/0149_security_code_smell_audit_fable/findings/D1_groupmembership.md](../0149_security_code_smell_audit_fable/findings/D1_groupmembership.md) の M-3
- 先行タスク: [#920（0160）](../0160_permission_check_subject_explicit/01_requirements.md)。残存リスクの整理は [0160 の 02_architecture.md](../0160_permission_check_subject_explicit/02_architecture.md) §5.2、拡張余地は同 §9 に記載されている。
- 関連（本タスクでは扱わない）: [#921 `runner` の native root 実行サポートの是非](https://github.com/isseis/go-safe-cmd-runner/issues/921)

## 背景

### D1 M-3 のうち未解決の部分

0149 監査の所見 D1 M-3 は、`internal/groupmembership` が環境変数 `SUDO_UID` を無検証で権限チェックの基準UIDとして採用している点を指摘している。指摘に対する推奨対応は3点あった。

| 推奨対応 | 0160 での扱い | 本タスクでの扱い |
|---|---|---|
| 基準UID決定方針を呼び出し元が明示する | 対応済み | 変更しない |
| `user.LookupId` による実在確認 | 対象外（本 issue へ送付） | **対象** |
| 採用した事実と値の記録 | 対象外（本 issue へ送付） | **対象** |
| 呼び出し元の実 UID との突き合わせ | 対象外（本 issue へ送付） | 対象外（後述「突き合わせを対象外とする根拠」） |

0160 は基準UID決定方針（`RealUIDOnly` / `SudoUIDAware`）を導入し、`runner` を `RealUIDOnly` に、`record` / `verify` を `SudoUIDAware` に固定した。その結果 D1 M-3 の攻撃対象領域は3バイナリから2バイナリへ縮小したが、`SUDO_UID` の値そのものは、依然として数値としての範囲チェック以外の検証を受けていない。

### 現状の解決処理

`internal/groupmembership/manager.go` の `resolvePermissionCheckUID` は、基準UID決定方針が `SudoUIDAware` かつ実 UID が 0 のときにのみ `SUDO_UID` を読み、`parseSudoUID` で数値としての妥当性（整数であること、`0` 以上 `math.MaxUint32` 以下であること）だけを検証してその値を基準UIDとして返す。この関数は `CanCurrentUserSafelyReadFile` から**読み取り安全性チェックのたびに**呼ばれる。

採用は成功パスであり、何も記録されない。そのため、root の cron に `SUDO_UID` が残留したまま `record` / `verify` が実行される事故シナリオ（D1 M-3 に記載）が起きても、運用者はそれを事後に検出できない。

### 調査結果1: 記録の出力先（2026-07-30 時点）

issue は「`defaultFS` 経由の読み取りはロガー設定より前に発生しうるため、出力先の扱いを併せて決める必要がある」と述べている。調査の結果、本タスクの対象である `record` / `verify` についてはこの懸念が成立しないことを確認した。

- 本番コードで `slog.SetDefault` を呼ぶのは `internal/runner/bootstrap/logger.go` の2箇所のみであり、その到達元は `runner` に限られる（ほかに `internal/verification/test_helpers.go` が呼ぶが、これは `//go:build test` のテスト補助であり本番バイナリに含まれない）。
- `cmd/record/main.go` および `cmd/verify/main.go` は `slog.SetDefault` を呼ばず、`slog.Default()`（Go 標準の既定ハンドラ、出力先は stderr）をそのまま使う。
- `runner` は `RealUIDOnly` を宣言しているため、本タスクが追加する記録処理には到達しない。

すなわち `record` / `verify` では、プロセスのどの時点であっても `slog.Default()` は同一の既定ハンドラであり、「ロガー設定前に記録が発生して失われる」事象は起きない。よって出力先は `log/slog` の既定ロガーとする。

なお `record` / `verify` は `internal/redaction` の redaction ハンドラを設定しないが、本タスクが記録する値は UID・実 UID・方針名・環境変数名のみであり、機微情報を含まない。

### 調査結果2: 環境変数許可リストポリシーとの整合（2026-07-30 時点）

issue 項目4 の確認結果を記す。`internal/runner/base/environment/denylist.go` の `IsForbiddenEnvVar` が定める denylist に `SUDO_UID` は含まれない。ただしこの denylist は**子プロセスへ渡す環境変数**を制限するものであり（動的リンカ制御やインタプリタ起動時のコード注入の防止が目的）、自プロセスが自身の判定のために環境変数を読むかどうかを規定しない。本タスクが扱うのは後者である。

したがって両者は対象とする層が異なり、`SUDO_UID` を denylist へ追加する必要はない。`SUDO_UID` は子プロセスにとってコード実行経路を持たないため、denylist の選定基準にも合致しない。この結論を文書として残すことをもって項目4の対応とする。

### 突き合わせを対象外とする根拠

issue 項目3 は「呼び出し元の実 UID との整合性チェック（可能なら）」を挙げている。調査の結果、信頼できる突き合わせ手段は存在しないと判断した。

- `sudo record ...` の形態では、当該プロセスの親プロセスは root として動作する `sudo` 自身である。`/proc/<ppid>/status` から得られる UID は 0 であり、呼び出し元ユーザーの UID ではない。
- 親プロセスは先に終了しうるため、`ppid` の指す先が呼び出し元であるという前提自体が成立しない。
- `SUDO_USER` との突き合わせは、`SUDO_UID` と `SUDO_USER` の双方を同じ攻撃者が設定できるため、攻撃者に対する防御力を持たない。

以上より、本タスクは実在確認と記録の2点に絞る。

## 目的

- `record` / `verify` が `SUDO_UID` を基準UIDとして採用する際に、その値が実在するユーザーを指すことを確認し、確認できない場合はフェイルクローズドに倒す。
- `SUDO_UID` の採用によって基準UIDが実 UID と異なる値になった事実を運用者が事後に検出できるようにする。
- 上記により、D1 M-3 のうち「無検証の採用」と「採用が観測できない」という2点を解消する。

### 本タスクで解消されないもの

`SUDO_UID` を任意の実在ユーザーの UID に設定し、そのユーザーの視点で読み取り安全性チェックを通過させるという操作自体は、引き続き可能である。実在確認は「実在しない UID の混入」と「残留による誤設定」を排除するに留まり、当該バイナリを root として起動できる者に対する防御にはならない。この位置づけは D1 M-3 の「直接の権限昇格ではなく、多層防御の欠落および誤設定への耐性の問題」という評価を引き継ぐ。

## スコープ

### 対象

1. 基準UID決定方針が `SudoUIDAware` で `SUDO_UID` を採用する場合の、`os/user` によるユーザー実在確認。
2. 実在確認に失敗した場合のフェイルクローズドなエラー返却と、判別可能なセンチネルエラーの定義。
3. `SUDO_UID` の採用により基準UIDが実 UID と異なる値になった場合の、`log/slog` への記録。
4. `SudoUIDAware` を宣言するバイナリ（`record` / `verify`）が起動時に基準UIDを1回解決すること（F-006）。
5. 上記に対応するテストの追加・更新。
6. 開発者向け文書（`docs/dev/architecture_design/security-architecture.ja.md` の基準UID解決規則の記述）および利用者向け文書（`record` / `verify` の文書）の更新。

### 対象外

- 呼び出し元の実 UID との突き合わせ（「突き合わせを対象外とする根拠」のとおり）。
- `SUDO_UID` を denylist へ追加すること（「調査結果2」のとおり）。
- 基準UID決定方針の型・伝播機構の変更。0160 で決定した構造をそのまま用いる。
- `RealUIDOnly` を宣言するバイナリ（`runner`）の挙動変更。
- `SUDO_USER` / `SUDO_GID` の参照。
- 書き込み安全性チェック（`CanUserSafelyWriteFile` 系）。従来どおり実 UID を直接受け取り、`SUDO_UID` を参照しない。
- `runner` の native root 実行サポートの是非（#921）。

## 検討事項（設計判断が必要な項目、02_architecture.md で決定する）

- **実在確認の実装手段と非 CGO ビルドでの挙動**: `os/user` の `LookupId` は CGO 有効時は NSS 経由、無効時は `/etc/passwd` の直接パースとなる。非 CGO ビルドでは LDAP/SSSD 管理のユーザーが「実在しない」と判定され、本タスクの方針によりエラーとなる。これは 0149 の L-2 が指摘する非 CGO ビルドの既知の制約と同じ性質のものであり、許容するか、非 CGO ビルドでの扱いを分けるかを決定する。
- **実在確認結果の再利用**: 解決処理は読み取り安全性チェックのたびに呼ばれるため、`record` で多数のファイルを処理すると `LookupId` が同じ値に対して繰り返し実行される。プロセス内での再利用（キャッシュ）の要否と、再利用する場合の有効期間・並行安全性を決定する。既存のグループメンバーシップキャッシュ（30 秒 TTL）との整合も併せて検討する。なおこの項が前提とする「`record` で読み取り安全性チェックが多数回実行される」という見積もりは、後の調査により誤りと判明した（F-006）。
- **記録を1回に制限する機構の置き場所**: 「プロセス毎に一度」をインスタンス単位で持つか、パッケージレベルの状態として持つかを決定する。`defaultFS` を含む複数の `GroupMembership` インスタンスが同一プロセス内に存在しうるため、インスタンス単位では複数回出力されうる。テストから初期化できる手段（`//go:build test` 付きファイル）の要否も併せて決定する。
- **`LookupId` の差し替え口**: 既存の `resolvePermissionCheckUID` は `getenv` を引数で受け取る純粋関数としてテスト可能性を確保している。実在確認とロガーを同じ方式で引数として渡すか、別の方式を採るかを決定する。0160 の付録A が「`GroupMembership` に環境変数取得関数のフィールドを持たせる案」を退けた経緯と整合させること。
- **記録するログの属性名と文言**: 運用者が事故シナリオ（root cron への `SUDO_UID` 残留）を認識できる文言とする。属性名は既存のログ出力の命名規則に揃える。
- **`LookupId` 自体の失敗（NSS 障害等）と「ユーザーが存在しない」の区別**: 前者は一時障害でありうるが、いずれもフェイルクローズドとする方針は同じである。エラーを区別して報告するか否かを決定する。

## Acceptance Criteria

#### F-001: `SUDO_UID` 採用時のユーザー実在確認

基準UID決定方針が `SudoUIDAware` であり、実 UID が 0 で `SUDO_UID` が数値として妥当な場合に、その UID が実在するユーザーを指すことを確認する。

**Acceptance Criteria**:
- **AC-01**: 基準UID決定方針が `SudoUIDAware`、実 UID が 0、`SUDO_UID` が数値として妥当かつ実在するユーザーを指す場合、その値が基準UIDとして返る（0160 からの挙動変化がない）。
- **AC-02**: 同条件で `SUDO_UID` が実在しないユーザーを指す場合、基準UIDを返さずエラーを返す。読み取り安全性チェックはこのエラーによって失敗し、判定を通さない（フェイルクローズド）。
- **AC-03**: AC-02 のエラーは、`errors.Is` で判別できる専用のセンチネルエラーを含む。数値として不正な場合のエラー（`ErrSudoUIDOutOfRange` および解析失敗）とは区別できる。
- **AC-04**: ユーザー実在確認の処理自体が失敗した場合（ユーザーが存在しないと確定できない障害、例: NSS の一時障害）も、基準UIDを返さずエラーを返す。エラーは元の失敗原因を `errors.Is` で辿れる形で保持する。
- **AC-05**: `SUDO_UID` が数値として不正な場合（数値でない、負数、`math.MaxUint32` 超過）、実在確認を行わずに現行と同じエラーを返す。
- **AC-06**: 実 UID が 0 以外の場合、`SUDO_UID` の値によらず実在確認は行われず、実 UID が基準UIDとして返る。

#### F-002: `SUDO_UID` 採用事実の記録

`SUDO_UID` の採用によって基準UIDが実 UID と異なる値になった場合に、その事実を `log/slog` へ記録する。

**Acceptance Criteria**:
- **AC-07**: `SudoUIDAware` かつ実 UID が 0 で、`SUDO_UID` の採用によって基準UIDが実 UID と異なる値になった場合、`log/slog` に警告レベル（`slog.LevelWarn`）で記録される。
- **AC-08**: 記録には、採用した基準UID、実 UID、採用の根拠となった環境変数名（`SUDO_UID`）、および基準UID決定方針の名称が含まれる。
- **AC-09**: 同一プロセス内で読み取り安全性チェックを複数回実行しても、AC-07 の記録は1回だけ出力される。
- **AC-10**: `SUDO_UID` が設定されておらず基準UIDが実 UID のままの場合、および `SUDO_UID` が実 UID と同じ値（`0`）で基準UIDが変化しない場合、AC-07 の記録は出力されない。
- **AC-11**: 記録の出力先は `log/slog` の既定ロガーであり、`record` / `verify` の実行のどの時点で発生しても失われない（「調査結果1」を参照）。

#### F-003: `RealUIDOnly` を宣言するバイナリへの非影響

`runner` が宣言する `RealUIDOnly` の挙動が本タスクによって変化しないことを保証する。

**Acceptance Criteria**:
- **AC-12**: 基準UID決定方針が `RealUIDOnly` の場合、実 UID が 0 かつ `SUDO_UID` が設定されていても、ユーザー実在確認は一度も実行されない。実在確認処理を差し替え可能にした上で、呼ばれないことをテストで検証する。
- **AC-13**: 基準UID決定方針が `RealUIDOnly` の場合、F-002 の記録は出力されない。
- **AC-14**: 0160 で追加された `RealUIDOnly` の既存の挙動（`SUDO_UID` の読み取り自体が行われず、常に実 UID が返る）が維持される。

#### F-004: 挙動の全組み合わせのテスト網羅

`SudoUIDAware` について、実 UID と `SUDO_UID` の値・実在性の組み合わせに対する期待挙動を表として固定する。

**Acceptance Criteria**:
- **AC-15**: 下表の全行について、返る基準UID・エラーの有無・記録の有無を検証するテストがある。

  | 実 UID | `SUDO_UID` | 実在確認の結果 | 期待する基準UID | 記録 |
  |---|---|---|---|---|
  | 0 | 未設定（空文字列） | 実施しない | 0 | なし |
  | 0 | `0` | 実在する | 0 | なし（AC-10） |
  | 0 | `0` | 実在しない | エラー | なし |
  | 0 | 有効値 `N`（`N` ≠ 0） | 実在する | `N` | あり |
  | 0 | 有効値 `N`（`N` ≠ 0） | 実在しない | エラー（AC-03 のセンチネル） | なし |
  | 0 | 有効値 `N` | 確認処理が失敗 | エラー（AC-04） | なし |
  | 0 | 不正値（数値でない、負数、`math.MaxUint32` 超過） | 実施しない | エラー（現行維持） | なし |
  | 非 0 | 未設定 / 有効値 / 不正値 | 実施しない | 実 UID | なし |

- **AC-16**: `SudoUIDAware` の解決処理が、テストから実在確認の実装とログ出力先を差し替えられる。root 権限なしに全分岐を検証できる。

#### F-005: 文書の更新

基準UIDの解決規則の変更を開発者向け・利用者向けの両文書に反映する。

**Acceptance Criteria**:
- **AC-17**: `docs/dev/architecture_design/security-architecture.ja.md` の基準UID解決規則の記述（`record` の `SudoUIDAware` について「実 UID が 0 かつ `SUDO_UID` が範囲内の数値 UID であればその値を採用する」旨の記載）が、実在確認を含む現行の規則へ更新されている。英語版は `/mktrans` で反映する。
- **AC-18**: `record` / `verify` の利用者向け文書に、`SUDO_UID` が実在しないユーザーを指す場合は対象ファイルを1件も処理せずに実行が失敗すること、および `SUDO_UID` による基準UIDの変更が警告として記録されることが記載されている。日本語版を先に更新し、英語版は `/mktrans` で反映する。
- **AC-19**: `docs/translation_glossary.md` に、本タスクで新たに導入した用語（実在確認など）が追加されている。

#### F-006: 起動時の基準UID解決

F-001 と F-002 は `internal/groupmembership` の解決処理に対する要件であり、この処理が呼ばれることを前提としている。しかしこの前提はバイナリ層で成立していない。`internal/safefileio` の `SafeOpenFile` はシンボリックリンク対策のみを行い、読み取り安全性チェックを含まない。読み取り安全性チェックを経て解決処理へ至るのは `SafeReadFile` 系だけである。`record` は対象ファイルを `SafeOpenFile` で直接開いて読むため、新規記録だけを行う実行では解決処理が一度も呼ばれず、実在確認によるフェイルクローズドも採用事実の記録も成立しない（`internal/filevalidator/validator.go` の `SaveRecord`、`calculateHash`、共有ライブラリ解析）。

この前提を、読み取り経路に依存しない形で満たす。`SudoUIDAware` を宣言するバイナリが、対象ファイルの処理を始める前に基準UIDを1回解決する。

**Acceptance Criteria**:
- **AC-20**: `record` / `verify` は、対象ファイルを1件も処理しないうちに基準UIDの解決を1回行う。
- **AC-21**: AC-20 の解決が失敗した場合、対象ファイルを1件も処理せず非ゼロで終了し、失敗の内容を標準エラー出力へ示す。
- **AC-22**: AC-07 の記録は、読み取り安全性チェックが実行されたか否かによらず、`SUDO_UID` の採用によって基準UIDが実 UID と異なる値になった実行では必ず出力される。出力回数は AC-09 のとおり1回のままである。
- **AC-23**: 起動時の解決は、ファイルのパーミッションに対する判定規則を変えない。`record` がこれまで記録できたファイルは、基準UIDが解決できる限り引き続き記録できる。

## Success Criteria（要件レベル）

- 上記すべての Acceptance Criteria が実装され、対応するテストが `make test` で成功する。
- `make lint` が警告なく通過する。
- `go test -race` が成功する（記録を1回に制限する機構および実在確認結果の再利用機構の競合検出のため）。
- `record` / `verify` について、`SUDO_UID` が実在するユーザーを指す通常の運用では、0160 完了時点からの外部から観測可能な挙動の変化が「警告ログが1回出力されること」のみである。
- `runner` の読み取り判定経路の挙動に変化がない。
