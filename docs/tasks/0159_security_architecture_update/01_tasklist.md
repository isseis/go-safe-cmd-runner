# Task 0159: security-architecture §5 ドキュメント更新

セキュリティ設計ドキュメント §5「特権管理」を実装に合わせて全面更新する。

**関連**: Issue #919（親 Issue: #864 / Task 0157 フォローアップ）

**依存**: 0157 Phase 2 のマージ完了（確認済み: 0157 は完了しクローズ済み）

---

## タスクチェックリスト

### 棚卸しと修正

- [ ] Issue #919 のコメント（[issuecomment-5092735691](https://github.com/isseis/go-safe-cmd-runner/issues/919#issuecomment-5092735691)、0157 PR-7 実施時に記録）を起点として棚卸しする。0157 の設計書 §7.3 が挙げた「Phase 2 でフィールド削除により不正確さが増す」という当初想定は**誤りだったと訂正済み**（削除された `syscallSeteuid`/`syscallSetegid` は元々 §5 の構造体引用に含まれていなかった）なので、その訂正後の内容を出発点にすること
- [ ] `security-architecture.ja.md` §5 内のすべてのコード引用（パス・行番号・フィールド構成・処理フロー）を棚卸しする
- [ ] `internal/runner/base/privilege/unix.go` の現行実装を確認し、引用との乖離を洗い出す
- [ ] 古いパス `internal/runner/privilege/unix.go` を `internal/runner/base/privilege/unix.go` に修正（3箇所: 構造体引用・`WithPrivileges`引用・`isRootOwnedSetuidBinary`引用）
- [ ] `UnixPrivilegeManager` のフィールド構成を現行版に更新（`logger`/`originalUID`/`privilegeSupported`/`metrics`/`mu` に加え `osExit`、`identityVerifier`、`readSavedIDs` を追記。`syscallSeteuid`/`syscallSetegid` は現行構造体に存在しないため追記しない）
- [ ] `WithPrivileges` のコード引用・説明文を現行実装に合わせて全面更新する。現行は `prepareExecution` → `performElevation` → `handleCleanupAndMetrics` に分割されており、以下が引用に反映されていない
  - dry-run 等 `needsPrivilegeEscalation` が偽の operation では昇格自体をスキップする分岐
  - 復元後の EUID==UID / EGID==GID 防御的検証（`identityVerifier`）
  - 復元後の saved-set-uid/gid 不変条件チェック（`readSavedIDs`、非対応プラットフォームでは構造的にスキップ）
- [ ] 上記の新規セキュリティ機構（防御的検証・saved-set 不変条件チェック）を「セキュリティ保証」節にも追記する

### 削除要素の確認

- [ ] 0157 Phase 2 で削除される要素が §5 の記述に残っていないか確認（`rg` で 0 件を確認済み: `syscallSeteuid`/`syscallSetegid`/`降格`/`WithUserGroup`/`IsUserGroupSupported` はいずれも §5 に未出現。念のため作業時に再確認する）
  - `syscallSeteuid` / `syscallSetegid`
  - 到達不能な降格パス
  - `WithUserGroup` / `IsUserGroupSupported`
- [ ] 該当部分があれば削除または修正

### 設計整合性の確認

- [ ] §13「ユーザーとグループ実行セキュリティ」との重複・矛盾がないか確認
- [ ] 全体の記述が一貫性を持つことを確認

### 行番号維持方針の判定

- [ ] 行番号付き引用を今後も維持するか、行番号を落として構造説明に寄せるかを判定
- [ ] 判定根拠を作業ログまたは PR 説明に記載

### 日本語の推敲

- [ ] `security-architecture.ja.md` の修正をコミット
- [ ] `/japrose` コマンドで `security-architecture.ja.md` を推敲
- [ ] `security-architecture.ja.md` の修正をコミット

### バイリンガル文書の反映

- [ ] `/mktrans` コマンドで `security-architecture.md` に反映
- [ ] 英語版の節構成と段落数が日本語版と一致することを目視確認

### 完了条件

- [ ] `make test` / `make lint` が成功する
- [ ] 日本語版と英語版のセクション構成が一致している
- [ ] issue #919 との対応を PR 説明に記載

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
