# 要件定義書: groupmembership CGO版 getGroupMembers の fail-closed 化

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-07-19 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#858 [Security][H-1] groupmembership: getGroupMembers のエラー握りつぶしによる fail-open](https://github.com/isseis/go-safe-cmd-runner/issues/858)
- 詳細所見: [docs/tasks/0149_security_code_smell_audit_fable/findings/D1_groupmembership.md](../0149_security_code_smell_audit_fable/findings/D1_groupmembership.md) の H-1（および密接に関連する M-1 の一部）

## 背景

`internal/groupmembership` は `safefileio` 等が呼び出す、ハッシュファイル・設定ファイルの安全な読み書き判定に使われるセキュリティクリティカルなコンポーネントである。

CGO 版 `getGroupMembers`（`internal/groupmembership/membership_cgo.go`）は、C 関数 `get_group_members` が返す NULL を「グループが存在しない」「`getgrgid_r` のエラー（ERANGE・NSS 障害・EINTR 等）」「`malloc`/`strdup` 失敗」のいずれであっても区別せず、Go 側で一律 `([]string{}, nil)`（メンバー 0 人・エラーなし）として扱っている（`membership_cgo.go:122-127` 付近）。

この結果は `manager.go` の `isUserOnlyGroupMember` に渡り、「明示メンバー 0 人 → ユーザーが（プライマリグループの）唯一のメンバー」と解釈される（`manager.go:185-197` 付近）。これが `CanUserSafelyWriteFile` の group-writable 分岐で使われ、本来は安全でない group-writable ファイルへの書き込みを「安全」と誤判定する fail-open を引き起こす。

特に `getgrgid_r` は呼び出し時に渡したバッファがグループエントリを格納するのに不足していると `ERANGE` を返す仕様だが、現在の C 側実装は `sysconf(_SC_GETGR_R_SIZE_MAX)`（Linux では通常 1024 バイト程度）で確保した固定バッファを 1 回使うのみでリトライしない。メンバー数の多いグループ（攻撃者を含みうる）では ERANGE が現実的に発生し得る。

## 目的

CGO 版 `getGroupMembers` のエラーハンドリングを fail-closed に修正し、グループメンバー列挙に失敗した場合に「メンバー 0 人」への誤った縮退が発生しないようにする。これにより、group-writable ファイルの書き込み安全性判定が、列挙失敗時に安全側（拒否）に倒れることを保証する。

## スコープ

### 対象（本タスクで対応する）

- `internal/groupmembership/membership_cgo.go` の C 関数 `get_group_members` および Go 関数 `getGroupMembers`（CGO ビルド版のみ）。
- 「グループが見つからない」場合と「エラー（`getgrgid_r` 失敗・`malloc`/`strdup` 失敗）」の場合を区別し、後者は必ず non-nil error を返す（fail-closed）。
- `getgrgid_r` が `ERANGE` を返した場合のバッファ拡大リトライ。
- 呼び出し元（`manager.go` の `GetGroupMembers`, `isUserOnlyGroupMember` など）が「エラー時に安全側へ倒れる」ことを保証する（呼び出し元コードの変更が必要な場合のみ）。

### 対象外（別タスク・別 Issue とする）

- M-1（プライマリグループ共有環境での「メンバー 0 人 = 唯一のメンバー」仮定）: 「グループが実在し明示メンバー 0 人」という*正常系*の意味論の見直しであり、本タスクの「エラー握りつぶし」修正（*異常系*）とは異なる論点のため対象外。関連 Issue が必要であれば別途起票する。
- M-2（CGO 版・非 CGO 版の意味論差異の統一）: 本タスクは CGO 版のエラーハンドリングのみを対象とし、両実装の意味論統一は別タスクとする。
- M-3, M-4, L-1〜L-4, I-1, I-2: 本 Issue の対象外。

## 現状の問題点（再掲・詳細化）

1. **エラーと「見つからない」の混同**: C 側 `get_group_members` は以下すべてを NULL 返却で表現しており、呼び出し元 Go コードから区別できない。
   - グループ GID が `/etc/group`（または NSS）に存在しない（正常系: 空メンバー）
   - `getgrgid_r` がエラーコードを返した（`ERANGE`, `EINTR`, `EIO`, `EMFILE`, `ENOMEM` 等）（異常系: エラーとして扱うべき）
   - `malloc`（バッファ確保 or メンバー配列確保）や `strdup` の失敗（異常系: エラーとして扱うべき）
2. **ERANGE の未処理**: `getgrgid_r` は固定サイズバッファでは不十分な場合 `ERANGE` を返す。現行実装は 1 回のみ試行し、リトライしない。メンバー数の多いグループで発生しうる。
3. **fail-open への到達**: 上記の結果、`isUserOnlyGroupMember` は「メンバー 0 人」を「ユーザーが唯一のメンバー」と誤解釈し、group-writable ファイルへの書き込みを許可してしまう。

## 要件

### F-001: C 側で「グループが見つからない」と「エラー」を区別する

`get_group_members` は、呼び出し元が「グループ未検出（メンバー 0 人として扱ってよい正常系）」と「エラー発生（呼び出し元は fail-closed に倒すべき異常系）」を判別できる情報を返さなければならない。

**Acceptance Criteria**:
- **AC-01**: `getgrgid_r` が `s == 0 && result == NULL` を返した場合（グループが未検出）、Go 側の `getGroupMembers` は `([]string{}, nil)` を返す。
- **AC-02**: `getgrgid_r` が `s != 0` を返した場合（エラー）、Go 側の `getGroupMembers` は `(nil, err)` を返す。`err` は errno 相当の情報（少なくともエラーであることが `errors.Is` 等で判別可能なセンチネルエラー、可能なら errno 値を含むメッセージ）を含む。
- **AC-03**: `malloc`（グループエントリ用バッファ、メンバー配列）または `strdup`（メンバー名文字列）が失敗した場合、Go 側の `getGroupMembers` は `(nil, err)` を返す。

### F-002: ERANGE 時にバッファを拡大してリトライする

`getgrgid_r` が `ERANGE` を返した場合、C 側はバッファサイズを拡大して再試行しなければならない（`getgrgid_r` の定石パターン）。

**Acceptance Criteria**:
- **AC-04**: 初期バッファサイズで `getgrgid_r` が `ERANGE` を返した場合、バッファサイズを拡大（例: 倍化）して再試行する。
- **AC-05**: リトライには上限（最大バッファサイズ、または最大リトライ回数）を設け、無限ループを防止する。上限に達しても `ERANGE` が解消しない場合はエラーとして扱う（AC-02 に従う）。
- **AC-06**: バッファ拡大後の `getgrgid_r` 呼び出しが成功した場合、期待通りのグループメンバー一覧を返す（ERANGE が発生しない小規模グループと同じ結果になる）。

### F-003: エラー時に呼び出し元が fail-closed になることを保証する

`getGroupMembers` がエラーを返した場合、その呼び出し元（`manager.go` の `GetGroupMembers`, `isUserOnlyGroupMember`, `CanUserSafelyWriteFile` 等）は、メンバー列挙失敗を「書き込み安全」の根拠として使わない。

**Acceptance Criteria**:
- **AC-07**: `getGroupMembers` がエラーを返した場合、`GroupMembership.GetGroupMembers` はキャッシュに書き込まずエラーをそのまま呼び出し元に伝播する。
- **AC-08**: `getGroupMembers` がエラーを返した場合、`isUserOnlyGroupMember` はエラーを呼び出し元に伝播し、`false`（安全でない）と解釈可能な状態でエラーを返す。`(true, nil)` を返すことは決してない。
- **AC-09**: `getGroupMembers` がエラーを返した場合、`CanUserSafelyWriteFile` の group-writable 分岐は書き込みを許可しない（`(false, err)` を返す）。

### F-004: 既存の正常系動作の非退行

**Acceptance Criteria**:
- **AC-10**: グループが実在し、メンバー数が既存のバッファサイズで収まる既存のテストケース（`membership_cgo_test.go` 等）はすべて現状どおりパスする。
- **AC-11**: `free_string_array` によるメモリ解放は、エラーパス（ERANGE リトライ失敗、`malloc`/`strdup` 失敗）を含むすべての経路でリークを起こさない。

## 非機能要件

- **NF-01**: 変更は CGO ビルド (`membership_cgo.go`, `//go:build cgo`) のみに閉じ、非 CGO 版 (`membership_nocgo.go`) の挙動・インターフェースは変更しない。
- **NF-02**: `getGroupMembers(gid uint32) ([]string, error)` のシグネチャは変更しない（CGO 版・非 CGO 版で共通のインターフェース契約のため）。
- **NF-03**: 既存の `maxGroupMembers` によるメンバー数上限検証、count 検証ロジックは維持する。
- **NF-04**: Go 1.23.10 / 既存のビルドタグ構成を維持する。

## セキュリティ要件

- **SEC-01**: グループメンバー列挙に失敗した場合、いかなる呼び出し経路でも「書き込み許可」または「唯一のメンバーである」という判定に到達してはならない（fail-closed の原則）。
- **SEC-02**: エラーメッセージに機密情報（パスワードフィールド等）を含めない（既存方針の踏襲）。

## テスト方針（概要）

- ERANGE を意図的に発生させるための手段（例: 多数メンバーを持つ一時グループを `/etc/group` 相当のテストフィクスチャで用意する、または C 側関数を単体でテストする、もしくはバッファ初期サイズを一時的に小さくするテストビルドフックを検討）を要件定義後の設計フェーズで具体化する。
- `getgrgid_r` のエラー（`s != 0`）を注入する手段が CGO テストでは限定的であるため、モック可能な設計(関数変数化など)を検討する。
- 既存の `membership_cgo_test.go`, `manager_test.go` のテストケースを非退行の基準とする。

## 未解決事項 / レビューで確認したい点

- ERANGE リトライの上限（バッファサイズ上限、リトライ回数上限）の具体的な値。
- エラー注入によるテスト手法（C 側関数の直接テスト vs Go 側のフォールトインジェクション用フック導入）の是非。
- M-1（プライマリグループ共有環境の意味論）を本タスクと同時に扱うか、完全に別 Issue に切り出すか。
