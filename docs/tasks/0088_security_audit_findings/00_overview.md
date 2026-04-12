# セキュリティ検査所見一覧

## 検査概要

- **検査日**: 2026-04-11
- **検査対象**: `cmd/record`, `cmd/verify`, `cmd/runner` 3コマンド、および `internal/` 以下の主要セキュリティ関連パッケージ
- **検査手法**: 静的解析 (ソースコード読解)
- **検査対象外**: 動的テスト、ファジング、外部ライブラリ (`oklog/ulid/v2`, `pelletier/go-toml/v2` 等) の内部実装

## 検査した主要パッケージ

| パッケージ | 主責務 |
|---|---|
| `internal/runner/privilege` | setuid バイナリの権限昇格・降格管理 |
| `internal/runner/executor` | コマンド実行、環境変数構築、パス検証 |
| `internal/safefileio` | symlink-safe ファイル I/O (openat2 + fallback) |
| `internal/filevalidator` | SHA-256 ベースのハッシュ検証 |
| `internal/verification` | config/binary の統合検証マネージャ |
| `internal/runner/config` | TOML 設定ロード・変数展開 |
| `internal/runner/security` | コマンド/環境変数の allowlist 検証 |
| `internal/logging` | 構造化ログ、Slack webhook 通知 |
| `internal/groupmembership` | CGO による getgrgid_r 呼び出し |

## 所見サマリ

| ID | 重大度 | 領域 | 概要 | ファイル |
|---|---|---|---|---|
| M1 | 🔴 Medium-High | privilege | 権限昇格失敗時に egid が元に戻らない | [M1_privilege_egid_not_restored.md](M1_privilege_egid_not_restored.md) |
| M2 | 🟡 Medium | verification ↔ exec | バイナリ検証と exec の間の TOCTOU ウィンドウ | [M2_toctou_verify_exec.md](M2_toctou_verify_exec.md) |
| M3 | 🟡 Medium | env filter | `LD_*` 変数のブロック不完全 | [M3_ld_env_filter_incomplete.md](M3_ld_env_filter_incomplete.md) |
| M4 | 🟡 Medium | env validation | dangerous env value パターンがバイパス可能 | [M4_dangerous_env_bypass.md](M4_dangerous_env_bypass.md) |
| L1 | 🟠 Low | config loader | template include のパス正規化不足 | [L1_include_path_not_constrained.md](L1_include_path_not_constrained.md) |
| L2 | 🟠 Low | slack webhook | webhook URL の宛先 allowlist なし | [L2_slack_webhook_no_allowlist.md](L2_slack_webhook_no_allowlist.md) |
| L3 | 🔵 Info | ulid lib | 将来の ulid バージョン追従時の監視事項 | [L3_ulid_math_rand_monitoring.md](L3_ulid_math_rand_monitoring.md) |
| L4 | 🟠 Low | cgo | C 側 count 値の Go 側境界チェック欠如 | [L4_cgo_bounds_check.md](L4_cgo_bounds_check.md) |
| I1 | 🔵 Info | naming | `verifyFileWithFallback` の命名と実装の乖離 | [I1_verify_fallback_naming.md](I1_verify_fallback_naming.md) |

## 観察された良好な防御層

本検査で確認した、すでに実装されている堅牢な防御を記録しておく (所見の背景コンテキストとして重要)。

1. **多層防御**: config hash 検証 → パス canonical 解決 → allowlist 検証 → 実行時のコマンド検証 → 権限境界
2. **setuid 起動時の即時降格**: [cmd/runner/main.go:97](../../../cmd/runner/main.go#L97) で `syscall.Seteuid(syscall.Getuid())` 実行
3. **atomic symlink-safe I/O**: [safefileio/safe_file_linux.go](../../../internal/safefileio/safe_file_linux.go) の `openat2(RESOLVE_NO_SYMLINKS)`
4. **config/template の atomic 検証読込**: [verification/manager.go:39](../../../internal/verification/manager.go#L39) `VerifyAndReadConfigFile`
5. **シェル非経由の exec**: [executor.go:196](../../../internal/runner/executor/executor.go#L196) で `exec.CommandContext` を直接利用、stdin は `/dev/null` 固定
6. **固定 SecurePathEnv**: `/sbin:/usr/sbin:/bin:/usr/bin` を環境変数非依存で使用 ([security/types.go:93](../../../internal/runner/security/types.go#L93))
7. **`LD_LIBRARY_PATH`/`LD_PRELOAD`/`LD_AUDIT` の強制削除**: [executor/environment.go:87-89](../../../internal/runner/executor/environment.go#L87-L89)
8. **allowlist ベースの環境変数フィルタ** + 機密値 redaction
9. **TOML 厳格パース**: `DisallowUnknownFields` で未知フィールド拒否
10. **dynamic library / shebang interpreter / ELF syscall 解析** による深い静的検証

## 対応優先度 (推奨)

1. **M1** — 具体的なバグ、修正コスト小。**最優先**。
2. **M3** — denylist 拡張、多層防御強化、修正コスト小。
3. **M4** — 運用での false positive 影響を評価したうえで再設計検討。
4. **M2** — `fexecve` 対応は Go の制約があり中長期課題。短期は運用ドキュメントで対象ディレクトリ要件明記。
5. **L1, L2, L4, I1** — 余力で対応。
6. **L3** — 監視のみ (アクション不要)。

## 本レポートの限界

- 静的解析による観点のみ。動的テスト・ファジング・並行実行レースの実証は未実施。
- `elfanalyzer`, `machoanalyzer`, `dynlibanalysis` の深部 (命令デコード、ELF dynsym 解析) は大規模につき未検査。
- `filevalidator/pathencoding` モジュールのハッシュファイル命名衝突耐性は未検査。
- 並行実行時のレース条件は `privilege.Manager` の `sync.Mutex` 保護を確認したのみ。
