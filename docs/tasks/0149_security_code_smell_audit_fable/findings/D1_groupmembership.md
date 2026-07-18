# セキュリティ監査所見: internal/groupmembership/

- 監査日: 2026-07-19
- 対象: `internal/groupmembership/`（manager.go, membership_cgo.go, membership_nocgo.go。テストは参照のみ）
- 方法: 静的コードレビュー（読み取り専用）。呼び出し元（`internal/safefileio`, `internal/security`, `internal/runner/base/security`）の利用文脈も確認。

本パッケージはハッシュファイル・設定ファイルの安全な読み書き判定（`safefileio` 経由）に使われる、セキュリティクリティカルなコンポーネントである。

## 所見サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 1 |
| 🟡 Medium | 4 |
| 🟠 Low | 4 |
| 🔵 Info | 2 |

---

## 🔴 High

### H-1: CGO 版 `getGroupMembers` がエラーを「メンバー 0 人」として握りつぶし、group-writable 書き込み判定が fail-open になる

- 該当箇所: `internal/groupmembership/membership_cgo.go:122-127`（`if members == nil { return []string{}, nil }`）、C 側 `membership_cgo.go:26-43`、および `manager.go:185-197`（`isUserOnlyGroupMember`）
- 問題:
  C 関数 `get_group_members` は「グループが存在しない」「`getgrgid_r` がエラー（ERANGE / NSS 障害 / EINTR 等）」「`malloc` 失敗」のすべてを NULL で返す。Go 側はこれを区別せず `([]string{}, nil)`（メンバーなし・エラーなし）として返す。
  この結果は `isUserOnlyGroupMember`（manager.go:189-191）で「プライマリグループかつ明示メンバー 0 人 → ユーザーが唯一のメンバー」と解釈され、`CanUserSafelyWriteFile` の group-writable 分岐（manager.go:240-247）で **書き込み許可（fail-open）** につながる。
  特に `getgrgid_r` はバッファ不足時に ERANGE を返すが、C 側は `sysconf(_SC_GETGR_R_SIZE_MAX)`（Linux では通常 1024）で確保した固定バッファ 1 回きりでリトライしない（membership_cgo.go:26-43）。メンバーの多い大きなグループでは ERANGE が現実的に発生する。
- 悪用/障害シナリオ:
  1. 攻撃者を含む多数のメンバーを持つグループ G（エントリが `_SC_GETGR_R_SIZE_MAX` を超える）がファイル F のグループで、F は group-writable、被害ユーザーが F の所有者かつ G がプライマリグループ。
  2. `getgrgid_r` が ERANGE で失敗 → メンバー 0 人と誤認 → 「ユーザーは唯一のメンバー」→ 書き込み安全と判定。
  3. 実際にはグループの他メンバー（攻撃者）も F を書き換えられるのに、`safefileio` はこのファイルへのハッシュ書き込み等を安全とみなす。
  また、一時的な NSS/LDAP 障害やメモリ枯渇でも同じ fail-open が起きる。
- 推奨対応:
  - C 側で「見つからない」（`s == 0 && result == NULL`）と「エラー」（`s != 0`）を区別して Go に伝え、エラー時は必ず `(nil, err)` を返す（fail-closed）。
  - ERANGE の場合はバッファを倍々で拡大してリトライする（getgrgid_r の定石パターン）。
  - `malloc`/`strdup` 失敗もエラーとして返す。

---

## 🟡 Medium

### M-1: `isUserOnlyGroupMember` の「明示メンバー 0 人 = 唯一のメンバー」仮定は、共有プライマリグループ環境で不成立

- 該当箇所: `internal/groupmembership/manager.go:185-193`
- 問題: プライマリグループについて「明示メンバー（`gr_mem`）が 0 人なら、ユーザーが唯一のメンバー」とみなしている（コメント自身が "depends on implementation" と認めている）。これは Debian/Ubuntu 系の user-private-group（ユーザーごとに専用グループ）前提であり、複数ユーザーが同一プライマリグループ（例: 伝統的な `users` GID 100、あるいは LDAP で共通プライマリ GID を配る構成）を共有する環境では成立しない。`/etc/group` の `users` 行にはメンバーが列挙されないのが普通なので、CGO 版では明示メンバー 0 人 → 書き込み許可となる。
- 悪用シナリオ: プライマリグループを共有する環境で、被害ユーザー所有・group=users・group-writable のファイルが「安全に書ける」と判定される。同グループの他ユーザーが同ファイルを書き換え可能なのに、ハッシュ記録などの保護対象操作が許可される。
- 推奨対応: CGO 版でも `/etc/passwd`（NSS 経由なら `getpwent` 相当/`user` パッケージ）でプライマリ GID が一致するユーザーを列挙して合算する（non-CGO 版 `membership_nocgo.go:41-49` と同じ意味論に揃える）。少なくとも「メンバー 0 人 → true」の分岐は削除し fail-closed にする。

### M-2: CGO 版と非 CGO 版で `getGroupMembers` の意味論が異なる（ビルドフラグ依存でセキュリティ判定が変わる）

- 該当箇所: `internal/groupmembership/membership_cgo.go:122-148` vs `membership_nocgo.go:19-58`
- 問題: 非 CGO 版は「/etc/group の明示メンバー + /etc/passwd でプライマリ GID が一致するユーザー」を返すのに対し、CGO 版は `gr_mem`（明示メンバー）のみを返す。同一システム・同一グループでも、`CGO_ENABLED` の設定次第で `isUserOnlyGroupMember` の結果（＝書き込み可否）が変わる。セキュリティポリシーの結果がビルド構成に依存するのは危険な設計上の不一致であり、M-1 の悪用可否もビルドによって変わる。
- 推奨対応: 両実装の返す集合の定義を仕様として明文化し、一致させる（推奨は「明示メンバー + プライマリメンバー」の和集合）。`membership_common_test.go` に両実装共通の意味論テストを追加する。

### M-3: `SUDO_UID` 環境変数を無検証に信頼して権限チェック UID を差し替えている

- 該当箇所: `internal/groupmembership/manager.go:454-488`（`getPermissionCheckUID`, `parseSudoUID`）
- 問題: EUID（実際には実 UID、M-4 参照）が 0 のとき、環境変数 `SUDO_UID` の値をそのまま権限チェック対象 UID として採用する。数値範囲は検証するが、「その UID が本当に sudo の呼び出し元か」「実在ユーザーか」の検証はない。root として起動できる者が `SUDO_UID=<任意 UID>` を設定すれば、読み取り安全性チェック（`CanCurrentUserSafelyReadFile` → `IsUserInGroup`）を任意ユーザーの視点で通過させられる。root 前提なので直接の権限昇格ではないが、環境変数という攻撃面でセキュリティ判定の主体が差し替わるのは防御多層の観点で弱い。root cron から `SUDO_UID` が残留した環境で実行された場合など、意図しない UID で判定される事故も起こり得る。
- 悪用/障害シナリオ: `sudo -E` や環境保持設定で `SUDO_UID`/`SUDO_GID` が別プロセス由来のまま残った状態で実行 → 意図しないユーザーとしてグループ判定が行われ、本来拒否すべき group-writable ファイルの読み取りを許可（またはその逆で誤拒否）。
- 推奨対応: `SUDO_UID` 利用時は `user.LookupId` で実在確認する、利用した事実と値を監査ログに記録する、可能なら呼び出し元の実 UID（`getuid(2)`）と突き合わせる。プロジェクト全体の環境変数分離ポリシー（`internal/runner` の env allowlist）との整合も確認すること。

### M-4: `getProcessEUID` は実際には EUID ではなく実 UID（キャッシュ付き）を返す — 名前・コメントと実装の乖離

- 該当箇所: `internal/groupmembership/manager.go:490-515`、参照: Go 標準 `os/user/cgo_lookup_unix.go`（`current()` は `syscall.Getuid()` を使用）
- 問題: `user.Current()` は **実 UID**（`getuid(2)`）でユーザーを引き、かつ結果をプロセス生存中キャッシュする。したがって:
  1. 関数名 `getProcessEUID` とコメント（"actual EUID of the running process"）が事実と異なる。setuid バイナリ（ruid=一般ユーザー, euid=0）では 0 ではなく一般ユーザー UID が返り、`CanCurrentUserSafelyWriteFile` の「実際に書けるか」の判定意図（manager.go:285-288 のコメント）とずれる。
  2. `getPermissionCheckUID` の sudo 判定 `currentUID == 0`（manager.go:461）も実 UID 判定であり、「EUID must be 0」というコメントと不一致（sudo は既定で ruid も 0 にするため偶然動くが、`seteuid` ベースの特権昇格中には検出されない）。
  3. `user.Current()` のキャッシュにより、本プロジェクトの特権昇格/降格（`internal/runner/privilege`）の前後で UID が変わってもチェックは初回の値で固定される。
  現状の主な影響は fail-closed 方向（setuid 構成で root 権限の書き込みが不当に拒否される）だが、セキュリティ判定の主体 UID が意図と異なるのは重大な smell。
- 推奨対応: EUID が必要なら `os.Geteuid()` を直接使う（キャッシュもなく意図が明確）。実 UID が正しい仕様ならば関数名とコメントを修正する。特権昇格の前後どちらの文脈で呼ばれるかを呼び出し元と合わせて仕様化する。

---

## 🟠 Low

### L-1: `GetGroupMembers` がキャッシュ内部のスライスをそのまま返す（キャッシュ汚染の可能性）

- 該当箇所: `internal/groupmembership/manager.go:88-90, 99-100, 117-122`
- 問題: キャッシュヒット時に `cached.members` を返却しており、呼び出し元がスライス要素を書き換えると以後最大 30 秒間、全呼び出し元のセキュリティ判定（`isUserOnlyGroupMember`, `IsUserInGroup`）が汚染された内容で行われる。現在の呼び出し元は読み取りのみだが、共有可変状態の露出は堅牢でない。
- 推奨対応: 返却時に `slices.Clone(cached.members)` を返す。

### L-2: 非 CGO 版は `/etc/group`・`/etc/passwd` の直接パースのため NSS（LDAP/SSSD 等）のメンバーが見えない

- 該当箇所: `internal/groupmembership/membership_nocgo.go:68-97, 121-151`
- 問題: ディレクトリサービス管理のユーザー/グループメンバーはローカルファイルに現れないため、実際にはグループに他メンバーがいるのに「唯一のメンバー」と判定され、group-writable ファイルへの書き込みが許可され得る（fail-open）。CGO 版（NSS 経由）との差異は M-2 とも関連。
- 推奨対応: 非 CGO ビルドでの利用は NSS 未使用環境に限る旨をドキュメント化する、または非 CGO 版では group-writable の許可判定を常に拒否（fail-closed）に倒すことを検討する。

### L-3: 非 CGO 版パーサが不正行を黙って読み飛ばす

- 該当箇所: `internal/groupmembership/membership_nocgo.go:82-85, 136-139`
- 問題: `parseGroupLine`/`parsePasswdLine` の失敗行を `continue` で無視する。対象グループの行が（破損・手編集ミスで）不正だった場合「グループなし＝メンバー 0 人」となり、H-1/M-1 と同じ経路で fail-open に合流する。ログも出ないため運用上検知できない。
- 推奨対応: 少なくとも `slog.Warn` で不正行を記録する。対象 GID の行がパース不能だった場合はエラーを返すことを検討する。

### L-4: 判定 API が「(false, nil)」と「(false, err)」を混在して返す

- 該当箇所: `internal/groupmembership/manager.go:219-259`（`CanUserSafelyWriteFile`）ほか
- 問題: 拒否理由をエラーで返す分岐（world-writable, not-owner 等）と、bool=false かつ err=nil で返す分岐（`isUserOnlyGroupMember` が false の場合、manager.go:246）が混在する。呼び出し元が `err != nil` のみで判定すると group-writable のケースをすり抜ける。現行の呼び出し元（safefileio）は bool も見ているため実害はないが、誤用を誘発しやすい API 形状。
- 推奨対応: 「拒否は常に sentinel エラー」または「拒否は常に (false, nil)」のどちらかに統一する。

---

## 🔵 Info

### I-1: グループメンバーシップキャッシュ（30 秒 TTL）による判定の遅延反映

- 該当箇所: `internal/groupmembership/manager.go:19, 85-123`
- 説明: グループからユーザーを除名しても最大 30 秒間は旧メンバーシップで判定される。長寿命デーモンではなくバッチランナーである本ツールの用途では許容範囲だが、仕様として認識しておくべき。ロック処理自体（RWMutex + double-check、manager.go:87-101）は正しく実装されている。

### I-2: check-to-use（TOCTOU）は呼び出し元の責務

- 該当箇所: `internal/groupmembership/manager.go:219`（`CanUserSafelyWriteFile` は uid/gid/perm を引数で受ける純粋判定）
- 説明: 本パッケージ自身は stat を取らないため TOCTOU は発生しないが、判定結果の有効性は呼び出し元が「開いた fd に対する fstat の値」を渡すことに依存する。確認した範囲では `safefileio` は `canSafelyAccessFile` で開いた file の情報を使っており適切。この契約（fd ベースの stat を渡すこと）を godoc に明記するとよい。

---

## 観察された良好な防御層

- **fail-closed な権限ビット検査**: world-writable の一律拒否（manager.go:235-237, 329-331）、読み書き別の許可上限ビットマスク方式（`MaxAllowedReadPerms`/`MaxAllowedWritePerms`、`&^` による禁止ビット検出）は allowlist 型で堅牢（manager.go:350-355, 382-389）。
- **setuid/setgid/sticky を含む検査**: `ValidateRequestedPermissions` は `Perm()` ではなく `AllPermissionBits (0o7777)` でマスクし、特殊ビットの混入を検出している（manager.go:383-385）。
- **UID 境界検査**: `userUID`/`SUDO_UID`/現在 UID の負値・uint32 超過を変換前に検証し、整数アンダーフロー/オーバーフローを防いでいる（manager.go:222-224, 484-487, 510-512）。
- **CGO 境界の防御**: `getgrgid_r`（スレッドセーフ版）の使用、C から受け取った count の負値・上限（65536）検証を `free`/`unsafe.Slice` の**前**に実施、不正 count 時は外側配列のみ解放して未知個数のポインタ走査を回避（membership_cgo.go:109-141）。エラーパスでの C メモリ解放（strdup 失敗時の巻き戻し含む）も丁寧。
- **`unsafe.Slice` の使用**: 旧来の `(*[1 << 30]*C.char)` キャストではなく境界付きの `unsafe.Slice` を使用（membership_cgo.go:141）。
- **並行安全なキャッシュ**: RWMutex + 書き込みロック下での double-check、期限付きエントリと定期クリーンアップ（manager.go:85-123, 431-439）。
- **機密情報のログ出力なし**: 本パッケージはログ出力を行わず、パスワードフィールド（/etc/group の第 2 フィールド等）も保持・出力しない。
- **テスト整備**: 判定ロジック・キャッシュ・境界値（`parseSudoUID` の範囲外等）に対するユニットテストが存在する（manager_test.go, validate_permissions_test.go, membership_*_test.go）。

---

## 総評

権限ビットの検査は allowlist 型で fail-closed に設計されており良好。一方、**グループメンバー列挙の失敗系が一貫して「メンバー 0 人」に縮退し、それが `isUserOnlyGroupMember` 経由で書き込み許可（fail-open）に到達する**のが本パッケージ最大の構造的リスクである（H-1, M-1, L-2, L-3 は同一シンクに合流する）。「メンバー列挙の失敗・空結果を許可根拠にしない」よう判定側を fail-closed に倒すことが、最も費用対効果の高い是正である。
