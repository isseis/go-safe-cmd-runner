# Task 0159: security-architecture §5 ドキュメント更新

セキュリティ設計ドキュメント §5「特権管理」を実装に合わせて全面更新する。

**関連**: Issue #919（親 Issue: #864 / Task 0157 フォローアップ）

**依存**: 0157 Phase 2 のマージ完了

---

## タスクチェックリスト

### 棚卸しと修正

- [ ] `security-architecture.ja.md` §5 内のすべてのコード引用（パス・行番号・フィールド構成）を棚卸しする
- [ ] `internal/runner/base/privilege/unix.go` の現行実装を確認し、引用との乖離を洗い出す
- [ ] 古いパス `internal/runner/privilege/unix.go` を `internal/runner/base/privilege/unix.go` に修正
- [ ] `UnixPrivilegeManager` のフィールド構成を現行版に更新（`osExit`、`syscallSeteuid`、`syscallSetegid`、`identityVerifier`、`readSavedIDs` など）

### 削除要素の確認

- [ ] 0157 Phase 2 で削除される要素が記述に残っていないか確認
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

### バイリンガル文書の反映

- [ ] `security-architecture.ja.md` の修正をコミット
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

- 0157 Phase 2 のマージ後に着手すること（先行すると同じ箇所を二度書き直す）
- CLAUDE.md のバイリンガル文書編集順序に従うこと（日本語版を先にコミット → `/mktrans` で英語版に反映）
