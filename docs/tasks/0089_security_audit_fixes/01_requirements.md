# 0089: セキュリティ検査所見の修正

## 背景

[0088_security_audit_findings](../0088_security_audit_findings/00_overview.md) の静的解析で発見された所見のうち、短期対応可能なものをまとめて修正する。

対象所見: M1, M2(短期), M3, M4, L1, L2, L4, I1
対象外: L3 (監視のみ), M2 中長期 (→ [0090_toctou_fexecve](../0090_toctou_fexecve/00_analysis.md) に切り出し)

---

## M1: 権限昇格失敗時の egid 復元バグ修正

### 問題

[internal/runner/privilege/unix.go:478-485](../../../internal/runner/privilege/unix.go#L478-L485) の `changeUserGroupInternal` において、`Setegid(targetGID)` 成功後に `Seteuid(targetUID)` が失敗した場合のリカバリが不正。

リカバリコードが `syscall.Getegid()` (= 変更後の targetGID) を再度 `Setegid` に渡しており、実質 no-op のため元の egid に戻らない。

### 受け入れ条件

- AC-M1-1: `changeUserGroupInternal` の引数に `originalEGID int` を追加し、`Seteuid` 失敗時に `Setegid(originalEGID)` でロールバックすること
- AC-M1-2: `egid` のロールバックにも失敗した場合、既存の `emergencyShutdown` と同等の処理でプロセスを即時終了すること
- AC-M1-3: 呼び出し側 `performElevation` が `execCtx.originalEGID` を `changeUserGroupInternal` に渡すこと
- AC-M1-4: `Seteuid` 失敗 → egid ロールバック成功 のパスを網羅するユニットテストを追加すること
- AC-M1-5: `Seteuid` 失敗 → egid ロールバック失敗 のパスを網羅するユニットテストを追加すること

---

## M2 短期: TOCTOU ウィンドウの運用要件明文化と自動検査

### 問題

バイナリのハッシュ検証 (`VerifyGroupFiles`) と `exec.CommandContext` の間に TOCTOU ウィンドウが存在する。根本対策 (`fexecve`/`execveat`) は中長期課題 (→ 0090) とし、短期では運用要件の明文化と起動時の自動パーミッション検査を行う。

### 受け入れ条件

**運用ドキュメント整備:**

- AC-M2S-1: `docs/security/README.md` (なければ新規作成) に以下の必須運用要件を日本語で明記すること
  - `verify_files` および `commands` で指定するバイナリは、root 所有かつ group/other に書込権限なし (`o-w`, `g-w`) のディレクトリ配下に配置すること
  - そのディレクトリを含む親ディレクトリも同様のパーミッション要件を満たすこと
  - `--hash-dir` で指定するハッシュディレクトリ自体も同要件を満たすこと
  - これらの要件が満たされない場合、TOCTOU 攻撃によってハッシュ検証をバイパスされるリスクがあること

**自動パーミッション検査:**

- AC-M2S-2: `security.Validator` (または検証マネージャの起動時処理) に、`verify_files` / コマンドパス / ハッシュディレクトリの親ディレクトリを検査する関数を追加すること
- AC-M2S-3: 検査項目は「ディレクトリが root 所有であること」「group に書込権限がないこと」「other に書込権限がないこと」の3点とすること
- AC-M2S-4: 検査で問題が検出された場合は警告ログ (`logger.Warn`) を出力し、`runner` は起動を中断すること (`record`, `verify` は警告のみで継続可)
- AC-M2S-5: 自動検査のユニットテストを追加すること (モックファイルシステムで root 所有外・書込可ディレクトリの検出を確認)

---

## M3: `LD_*` 環境変数フィルタの強化

### 問題

[internal/runner/executor/environment.go:87-89](../../../internal/runner/executor/environment.go#L87-L89) で `LD_LIBRARY_PATH`, `LD_PRELOAD`, `LD_AUDIT` の3変数のみを明示削除しており、他の `LD_*` 変数 (`LD_DEBUG`, `GCONV_PATH` 等) が通り抜ける。

### 受け入れ条件

- AC-M3-1: `LD_` プレフィックスを持つ環境変数を **すべて** 削除すること (プレフィックスループで実装)
- AC-M3-2: 以下の非 `LD_` 系危険変数も削除すること: `GCONV_PATH`, `LOCPATH`, `HOSTALIASES`, `NLSPATH`, `RES_OPTIONS`
- AC-M3-3: 既存の個別削除コード (`delete(envMap, "LD_LIBRARY_PATH")` 等) を新実装に統合して重複を排除すること
- AC-M3-4: 上記の全変数が削除されることを確認するユニットテストを追加すること
- AC-M3-5: `LD_` プレフィックスを持つ任意の変数名 (例: `LD_FOOBAR`) も削除されることを確認するテストを含めること

---

## M4: 環境変数値の危険パターン検査の再設計

### 問題

[internal/runner/security/validator.go](../../../internal/runner/security/validator.go) の `dangerousValuePatterns` はシェルメタ文字 (`;`, `|`, `$(` 等) を文字列マッチで検出しているが、本プロジェクトはシェル非経由の `exec.CommandContext` を使用するため、この検査は実行モデルと噛み合っていない。false positive・false negative ともに問題がある。

### 採用方針: 案 C (再設計)

シェルメタ文字の検査を廃止し、execve に渡せない・多くのパーサを破壊する構文的に異常な文字のみを検査する。

### 受け入れ条件

- AC-M4-1: `dangerousValuePatterns` および関連するシェルメタ文字マッチのコードを削除すること
- AC-M4-2: 環境変数の値に `\0` (null byte) を含む場合はエラーを返すこと (`execve` に渡せないため)
- AC-M4-3: 環境変数の値に `\n` (LF) または `\r` (CR) を含む場合はエラーを返すこと (多くの子プロセスのパーサを破壊するため)
- AC-M4-4: `;`, `|`, `$(`, `>`, `<` 等のシェルメタ文字を含む値は拒否 **しない** こと (JSON 等の正当なユースケースを阻害しないため)
- AC-M4-5: `\0`, `\n`, `\r` を含む値が拒否され、それ以外のメタ文字を含む値が通過することを確認するユニットテストを追加すること

---

## L1: template include パスの basedir 制約

### 問題

[internal/runner/config/path_resolver.go](../../../internal/runner/config/path_resolver.go) の include パス解決において、`../` による basedir 脱出および絶対パス指定が無制限に許可されている。

### 受け入れ条件

- AC-L1-1: include パスが相対パスの場合、`filepath.Clean` 後のパスが basedir 配下であることを検証すること
- AC-L1-2: `filepath.Rel(basedir, resolved)` の結果が `..` で始まる場合はエラーを返すこと
- AC-L1-3: include パスに絶対パスを指定した場合はエラーを返すこと
- AC-L1-4: basedir 脱出・絶対パス指定のそれぞれについてエラーを返すことを確認するユニットテストを追加すること
- AC-L1-5: basedir 配下の正当な相対パス (例: `./sub/config.toml`, `sub/config.toml`) が正常に解決されることを確認するユニットテストを含めること

---

## L2: Slack webhook URL のホスト allowlist

### 問題

[internal/logging/slack_handler.go:132-152](../../../internal/logging/slack_handler.go#L132-L152) の `validateWebhookURL` は HTTPS スキームのみを検査し、ホスト名を制限しない。設定ファイルが改ざんされた場合に任意ホストへのログ送信 (情報漏洩・SSRF) が成立しうる。

### 受け入れ条件

- AC-L2-1: `validateWebhookURL` にホスト allowlist 検査を追加し、デフォルトで `hooks.slack.com` のみを許可すること
- AC-L2-2: allowlist に含まれないホストへの URL は検証エラーを返すこと
- AC-L2-3: 設定ファイルで追加ホストを allowlist に追加できること (拡張性のある設計)
- AC-L2-4: `hooks.slack.com` 以外のホスト (例: `evil.example.com`) がエラーになることを確認するユニットテストを追加すること
- AC-L2-5: `hooks.slack.com` の正当な URL が検証を通過することを確認するユニットテストを含めること

---

## L4: CGO グループメンバ数の境界チェック

### 問題

[internal/groupmembership/membership_cgo.go:106](../../../internal/groupmembership/membership_cgo.go#L106) で C 側から受け取った `count` 値に対して Go 側の境界チェックがなく、不正な値で unsafe なメモリアクセスが発生する可能性がある。

### 受け入れ条件

- AC-L4-1: C 側から受け取った `count` が負値の場合はエラーを返すこと
- AC-L4-2: `count` が合理的な上限 (65536) を超える場合はエラーを返すこと
- AC-L4-3: `(*[1 << 30]*C.char)(unsafe.Pointer(members))[:count:count]` パターンを `unsafe.Slice` を使った構築に置き換えること
- AC-L4-4: 境界チェックのユニットテストを追加すること (可能な範囲でモックまたはテスト用 C コードを用いる)

---

## I1: `verifyFileWithFallback` の命名修正

### 問題

`internal/verification/manager.go` 内の関数名 `verifyFileWithFallback` が実装内容と乖離している可能性があり、保守者が誤った前提でコードを変更するリスクがある。

### 受け入れ条件

- AC-I1-1: `internal/verification/manager.go` の `verifyFileWithFallback` の実装を調査し、「fallback」が何を指すか (または指さないか) を明確にすること
- AC-I1-2: 実装に fallback が存在する場合は、何にフォールバックするかを明示した名前にリネームすること (例: `verifyFileWithLegacyHashFallback`)
- AC-I1-3: 実装に fallback が存在しない場合は、実装を正確に表す名前にリネームすること (例: `verifyFileStrict` または `verifyFile`)
- AC-I1-4: リネーム後、全呼び出し箇所を新しい名前に更新すること
- AC-I1-5: 既存のテストがリネーム後もすべてパスすること

---

## 対象外

- **L3** (`ulid` の `math/rand` 使用): 将来バージョン追従時の監視事項であり、現時点でのアクションなし
- **M2 中長期** (`fexecve`/`execveat` による TOCTOU 根本対策): → [0090_toctou_fexecve](../0090_toctou_fexecve/00_analysis.md)
