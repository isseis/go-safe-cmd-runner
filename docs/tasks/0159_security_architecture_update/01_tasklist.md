# Task 0159: security-architecture §5 ドキュメント更新

セキュリティ設計ドキュメント §5「特権管理」を実装に合わせて全面更新する。

**関連**: Issue #919（親 Issue: #864 / Task 0157 フォローアップ）

**依存**: 0157 Phase 2 のマージ完了（確認済み: 0157 は完了しクローズ済み）

---

## タスクチェックリスト

### 棚卸しと修正

- [x] Issue #919 のコメント（[issuecomment-5092735691](https://github.com/isseis/go-safe-cmd-runner/issues/919#issuecomment-5092735691)、0157 PR-7 実施時に記録）を起点として棚卸しする。0157 の設計書 §7.3 が挙げた「Phase 2 でフィールド削除により不正確さが増す」という当初想定は**誤りだったと訂正済み**（削除された `syscallSeteuid`/`syscallSetegid` は元々 §5 の構造体引用に含まれていなかった）なので、その訂正後の内容を出発点にすること
- [x] `security-architecture.ja.md` §5 内のすべてのコード引用（パス・行番号・フィールド構成・処理フロー）を棚卸しする。加えて「マルチチャンネルログ（構造化ログ、syslog、stderr）」の記述も棚卸しで発見した不正確さと判明（`internal/` 配下に syslog ハンドラは存在せず、実際は構造化ログと stderr の2経路のみ）。修正済み
- [x] `internal/runner/base/privilege/unix.go` の現行実装を確認し、引用との乖離を洗い出す
- [x] 古いパス `internal/runner/privilege/unix.go` を `internal/runner/base/privilege/unix.go` に修正（3箇所: 構造体引用・`WithPrivileges`引用・`isRootOwnedSetuidBinary`引用）
- [x] `UnixPrivilegeManager` のフィールド構成を現行版に更新（`logger`/`originalUID`/`privilegeSupported`/`metrics`/`mu` に加え `osExit`、`identityVerifier`、`readSavedIDs` を追記。`syscallSeteuid`/`syscallSetegid` は現行構造体に存在しないため追記しない）
- [x] `WithPrivileges` のコード引用・説明文を現行実装に合わせて全面更新する。現行は `prepareExecution` → `performElevation` → `handleCleanupAndMetrics` に分割されており、以下を引用に反映した
  - `OperationUserGroupExecution`/`OperationFileValidation`以外の operation では`prepareExecution`が`ErrUnsupportedOperationType`エラーを返し、`WithPrivileges`が`fn()`を呼び出さずにエラーを返す分岐
  - 復元後の EUID==UID / EGID==GID 防御的検証（`identityVerifier`）
  - 復元後の saved-set-uid/gid 不変条件チェック（`readSavedIDs`、非対応プラットフォームでは構造的にスキップ）
- [x] 上記の新規セキュリティ機構（防御的検証・saved-set 不変条件チェック）を「セキュリティ保証」節にも追記する

### 削除要素の確認

- [x] 0157 Phase 2 で削除される要素が §5 の記述に残っていないか確認（`rg` で 0 件を再確認済み: `syscallSeteuid`/`syscallSetegid`/`降格`/`WithUserGroup`/`IsUserGroupSupported` はいずれもドキュメント全体に未出現）
  - `syscallSeteuid` / `syscallSetegid`
  - 到達不能な降格パス
  - `WithUserGroup` / `IsUserGroupSupported`
- [x] 該当部分があれば削除または修正（該当なし）

### 設計整合性の確認

- [x] §13「ユーザーとグループ実行セキュリティ」との重複・矛盾がないか確認（§13 は `Command` の設定フィールドと `GroupMembershipChecker` を扱うのみで、`UnixPrivilegeManager` の内部実装には触れておらず矛盾なし）
- [x] 全体の記述が一貫性を持つことを確認

### 行番号維持方針の判定

- [x] 行番号付き引用を今後も維持するか、行番号を落として構造説明に寄せるかを判定 → **行番号を落とす**方針とした
- [x] 判定根拠を作業ログまたは PR 説明に記載 → 根拠: (1) 本ドキュメント内に既に行番号なし引用（`// 場所: <path>` のみ）が多数存在し、混在が既存の慣行である。(2) 本タスクの発端そのものが「行番号・パスの陳腐化」であり、行番号は関数の追加・削除のたびに崩れるのに対し、シンボル名（型名・関数名）は相対的に安定している。(3) §5 は今回のように今後も実装変更頻度が高い領域であり、行番号を維持すると同種の乖離を再発させやすい。よって §5 の全引用からは行番号を削除し、ファイルパスのみを記載する形にした

### 日本語の推敲

- [x] `security-architecture.ja.md` の修正をコミット（5d327c64）
- [x] `/japrose` コマンドで `security-architecture.ja.md` を推敲（Major 4件: §4「シンボリックリンク安全な」の直訳調、§5 地の文の生英語"operation"×2箇所、§5「多重防御」/「多重の防御的検証」の表記ゆれを修正。検証パスで反映確認済み）
- [x] `security-architecture.ja.md` の修正をコミット（07527fc2）

### バイリンガル文書の反映

- [x] `/mktrans` コマンドで `security-architecture.md` に反映（差分翻訳、レビューサブエージェントで Critical/Major 0件を確認、コミット c946fa68）
- [x] 英語版の節構成と段落数が日本語版と一致することを目視確認（見出し構成が全節で1:1一致。§5内の空行数も23/23で一致）

### 完了条件

- [x] `make test` / `make lint` が成功する
- [x] 日本語版と英語版のセクション構成が一致している
- [x] issue #919 との対応を PR 説明に記載（[PR #939](https://github.com/isseis/go-safe-cmd-runner/pull/939)）

---

## 参考資料

- **対象ファイル**
  - `docs/dev/architecture_design/security-architecture.ja.md` §5
  - `docs/dev/architecture_design/security-architecture.md` §5

- **実装ファイル**
  - `internal/runner/base/privilege/unix.go`

- **関連タスク**
  - 0157（#864）: [Dead code & naming cleanup in privilege management](../0157_dead_code_naming_cleanup/02_architecture.md) §7.3 / §9

---

## 注意事項

- 0157 Phase 2 は既にマージ済みのため、着手のブロッカーはない
- CLAUDE.md のバイリンガル文書編集順序に従うこと（日本語版を先にコミット → `/mktrans` で英語版に反映）
