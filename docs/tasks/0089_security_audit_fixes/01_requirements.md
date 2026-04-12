# 0089: セキュリティ検査所見の修正

## 背景

[0088_security_audit_findings](../0088_security_audit_findings/00_overview.md) の静的解析で発見された所見のうち、短期対応可能なものをまとめて修正する。

対象所見: M1, M2(短期), M3, M4, L1, L4, I1
対象外: L3 (監視のみ), M2 中長期 (→ [0090_toctou_fexecve](../0090_toctou_fexecve/00_analysis.md) に切り出し), L2 (→ [0091_slack_webhook_allowlist](../0091_slack_webhook_allowlist/01_requirements.md) に切り出し)

---

## M1: 権限昇格失敗時の egid 復元バグ修正

### 問題

[internal/runner/privilege/unix.go:478-485](../../../internal/runner/privilege/unix.go#L478-L485) の `changeUserGroupInternal` において、`Setegid(targetGID)` 成功後に `Seteuid(targetUID)` が失敗した場合のリカバリが不正。

リカバリコードが `syscall.Getegid()` (= 変更後の targetGID) を再度 `Setegid` に渡しており、実質 no-op のため元の egid に戻らない。

### 受け入れ条件

- AC-M1-1: `changeUserGroupInternal` の引数に `originalEGID int` を追加し、`Seteuid` 失敗時に `Setegid(originalEGID)` でロールバックすること
- AC-M1-2: `egid` のロールバックにも失敗した場合、既存の `emergencyShutdown` を呼び出してプロセスを即時終了すること
- AC-M1-3: 呼び出し側 `performElevation` が `execCtx.originalEGID` を `changeUserGroupInternal` に渡すこと
- AC-M1-4: `Seteuid` 失敗 → egid ロールバック成功 のパスを網羅するユニットテストを追加すること
- AC-M1-5: `Seteuid` 失敗 → egid ロールバック失敗 のパスを網羅するユニットテストを追加すること

---

## M2 短期: TOCTOU ウィンドウの運用要件明文化と自動検査

### 問題

バイナリのハッシュ検証 (`VerifyGroupFiles`) と `exec.CommandContext` の間に TOCTOU ウィンドウが存在する。根本対策 (`fexecve`/`execveat`) は中長期課題 (→ 0090) とし、短期では運用要件の明文化と、起動前に実行対象パスおよびハッシュディレクトリの配置ディレクトリを検査する自動パーミッション検査を行う。

### 受け入れ条件

**運用ドキュメント整備:**

- AC-M2S-1: `docs/security/README.md` に以下の必須運用要件を日本語で明記すること
  - `verify_files` および `commands` で指定するバイナリは、既存のディレクトリパーミッション検査 (`ValidateDirectoryPermissions` / `validateCompletePath`) が通過するディレクトリ配下に配置すること
  - 具体的には、対象ディレクトリおよびルートまでの全親ディレクトリが、other 書込不可 (sticky bit ディレクトリを除く)、group 書込は root 所有または実行ユーザが唯一のグループメンバである場合のみ許可、owner 書込は root または実行ユーザ所有の場合のみ許可、パスの途中にシンボリックリンクを含まないこと、という条件を満たすこと
  - `--hash-dir` で指定するハッシュディレクトリ自体と、その親ディレクトリ群も同要件を満たすこと
  - これらの要件が満たされない場合、TOCTOU 攻撃によってハッシュ検証をバイパスされるリスクがあること

**自動パーミッション検査:**

- AC-M2S-2: 起動前検査の責務を持つコンポーネントに、`verify_files` で参照される各ファイルの親ディレクトリ、実行コマンドの親ディレクトリ、ハッシュディレクトリ自身とその**ルートまでの全親ディレクトリ**を列挙して検査する機能を追加すること
- AC-M2S-3: 各対象ディレクトリについて、既存の `ValidateDirectoryPermissions` / `validateCompletePath` と同等のポリシーで検査すること (other 書込不可、group 書込制約、owner 書込制約、シンボリックリンク不可)
- AC-M2S-4: 検査で問題が検出された場合は、問題のあったパスと違反内容を含む警告ログ (`logger.Warn`) を出力すること
- AC-M2S-5: 検査で問題が検出された場合、`runner` は実行開始前にエラー終了し、`record` と `verify` は警告のみで継続できること
- AC-M2S-6: 自動検査のユニットテストを追加すること (既存の `ValidateDirectoryPermissions` テストと整合する形で、検出対象パスの列挙と検査実行の振る舞いを確認)
- AC-M2S-7: `runner` が検査失敗後に起動中断すること、および `record` / `verify` が警告のみで継続することを確認するテストを追加すること

---

## M3: `LD_*` 環境変数フィルタの強化

### 問題

[internal/runner/executor/environment.go:87-89](../../../internal/runner/executor/environment.go#L87-L89) で `LD_LIBRARY_PATH`, `LD_PRELOAD`, `LD_AUDIT` の3変数のみを明示削除しており、他の `LD_*` 変数や glibc ローダ関連の危険変数 (`LD_DEBUG`, `GCONV_PATH` 等) が通り抜ける。

### 受け入れ条件

- AC-M3-1: `LD_` プレフィックスを持つ環境変数を **すべて** 削除すること (プレフィックスループで実装)
- AC-M3-2: 以下の非 `LD_` 系危険変数も削除すること: `GCONV_PATH`, `LOCPATH`, `HOSTALIASES`, `NLSPATH`, `RES_OPTIONS`
- AC-M3-3: 既存の個別削除コード (`delete(envMap, "LD_LIBRARY_PATH")` 等) を新実装に統合して重複を排除すること
- AC-M3-4: 上記の全変数が削除されることを確認するユニットテストを追加すること
- AC-M3-5: `LD_` プレフィックスを持つ任意の変数名 (例: `LD_FOOBAR`) も削除されることを確認するテストを含めること
- AC-M3-6: `internal/runner/config/expansion.go` の `forbiddenEnvVars` (`env_import` 検査) についても、`LD_` プレフィックス全体の拒否および AC-M3-2 の非 `LD_` 系危険変数の拒否を反映すること

---

## M4: 環境変数値の危険パターン検査の再設計

### 問題

[internal/runner/security/environment_validation.go](../../../internal/runner/security/environment_validation.go) の環境変数値検査は、`Validator` が保持するシェルメタ文字ベースの危険パターンに依存しているが、本プロジェクトはシェル非経由の `exec.CommandContext` を使用するため、この検査は実行モデルと噛み合っていない。false positive・false negative ともに問題がある。

### 採用方針: 案 C (再設計)

シェルメタ文字の検査を廃止し、execve に渡せない・多くのパーサを破壊する構文的に異常な文字のみを検査する。

### 受け入れ条件

- AC-M4-1: 環境変数値検査から、シェルメタ文字ベースの危険パターン定義および関連するマッチ処理を削除すること
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
- AC-L1-2: `filepath.Rel(basedir, resolved)` の結果が `..` と等しい、または `..` + パス区切りで始まる場合はエラーを返すこと
- AC-L1-3: include パスに絶対パスを指定した場合はエラーを返すこと
- AC-L1-4: basedir 脱出・絶対パス指定のそれぞれについてエラーを返すことを確認するユニットテストを追加すること
- AC-L1-5: basedir 配下の正当な相対パス (例: `./sub/config.toml`, `sub/config.toml`) が正常に解決されることを確認するユニットテストを含めること

---

## L2: Slack webhook URL のホスト allowlist

→ [0091_slack_webhook_allowlist](../0091_slack_webhook_allowlist/01_requirements.md) に切り出し

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

## I1: `verifyFileWithFallback` 系関数の命名修正

### 問題

`internal/verification/manager.go` 内の関数名 `verifyFileWithFallback` および `readAndVerifyFileWithFallback` が実装内容と乖離している可能性があり、保守者が誤った前提でコードを変更するリスクがある。少なくとも現状の `verifyFileWithFallback` には名称が示す fallback 処理が存在しない。

### 受け入れ条件

- AC-I1-1: `internal/verification/manager.go` の `verifyFileWithFallback` と `readAndVerifyFileWithFallback` の実装を調査し、「fallback」が何を指すか (または指さないか) を明確にすること

> **調査結果 (2026-04-12):**
> - `verifyFileWithFallback`: コメントに "falls back to privileged verification" とあるがフォールバック実装は存在しない。→ AC-I1-3 を適用
> - `readAndVerifyFileWithFallback`: dry-run モードで検証失敗時に `os.ReadFile` でファイル読み込みにフォールバックする。→ AC-I1-2 を適用
- AC-I1-2: 実装に fallback が存在する場合は、何にフォールバックするかを明示した名前にリネームすること (例: `verifyFileWithLegacyHashFallback`)
- AC-I1-3: 実装に fallback が存在しない場合は、実装を正確に表す名前にリネームすること (例: `verifyFile`, `readAndVerifyFile`)
- AC-I1-4: リネーム後、関連するコメント・ログ文言・全呼び出し箇所を新しい名前に更新すること
- AC-I1-5: 既存のテストがリネーム後もすべてパスすること

---

## 対象外

- **L3** (`ulid` の `math/rand` 使用): 将来バージョン追従時の監視事項であり、現時点でのアクションなし
- **M2 中長期** (`fexecve`/`execveat` による TOCTOU 根本対策): → [0090_toctou_fexecve](../0090_toctou_fexecve/00_analysis.md)
