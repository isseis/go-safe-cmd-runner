# B3: internal/verification/ セキュリティ監査所見

- 監査日: 2026-07-18
- 対象: `internal/verification/` (manager.go, manager_production.go, path_resolver.go, result_collector.go, errors.go, types.go, interfaces.go, test_helpers.go)
- 方法: 静的コードレビュー（読み取り専用）。呼び出し元 (`internal/runner/group_executor.go`, `internal/runner/bootstrap/config.go`) も参照して攻撃面を評価。

## 所見サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 2 |
| 🟠 Low | 4 |
| 🔵 Info | 4 |

---

## 🟡 Medium

### M1: collectVerificationFiles のパス解決失敗が warn + continue（fail-open）で、検証セットからコマンドが静かに脱落する

- 該当箇所: `internal/verification/manager.go:264-277`（特に 269-273）
- 呼び出し元: `internal/runner/group_executor.go:404` 以降（verifyGroupFiles）

**問題**: グループ検証対象の収集時、`m.pathResolver.ResolvePath(command.ExpandedCmd)` が失敗すると `slog.Warn` を出して `continue` し、そのコマンドは検証対象ファイル集合に含まれない。`VerifyGroupFiles` 自体は成功として返る。

呼び出し元 (`group_executor.go`) は直後のループで再度 `ResolvePath` を呼び、失敗すれば fail-closed になるが、**収集時に失敗し、直後のループ実行時までにファイルが出現した場合**は解決に成功してしまう。このとき:

1. コマンドバイナリはハッシュ検証集合に含まれていない（`ContentHashes` にも無い）。
2. `VerifyCommandDynLibDeps` / `VerifyCommandShebangInterpreter` は `LoadRecord` が `ErrRecordNotFound` を返すため `return nil`（記録が無いバイナリはチェック対象外の設計）。
3. 結果として **一切のハッシュ検証なしでコマンドが実行され得る**。

なお、収集時と実行ループの両方で解決に成功するケースは PathResolver のキャッシュにより同一の解決済みパスが返るため、「別のパスにすり替わる」形の TOCTOU は防がれている。問題になるのは「失敗 → 成功」への遷移のみ。

**悪用シナリオ**: コマンドパス（またはその親ディレクトリ）に書き込み権限を持つ攻撃者が、対象ファイルを一時的に削除しておき、`collectVerificationFiles` 実行後〜per-command ループ実行前のごく短い窓で悪意あるバイナリを配置する。窓はミリ秒オーダーで、コマンドパスへの書き込み権限が前提のため実現性は低いが、成立すれば検証の完全バイパスとなる。

**推奨対応**: `collectVerificationFiles` での解決失敗を fail-closed（エラーとして `VerifyGroupFiles` を失敗させる）にする。あるいは呼び出し元ループで `result.ContentHashes[resolvedPath]` にエントリが存在すること（＝検証済みであること）を実行の前提条件にする。

### M2: isDeferredHashDirUnavailable による skip（fail-open）が dry-run に限定されていない

- 該当箇所: `internal/verification/manager.go:608-610`（定義）、`629-631`（verifyDynLibDeps）、`745-751`（VerifyCommandShebangInterpreter）

**問題**: コメントは「dry-run の read-only Validator が hash directory の欠如を deferred error として返すケース」と説明しているが、判定関数は

```go
return errors.Is(err, filevalidator.ErrHashDirNotExist) || errors.Is(err, os.ErrPermission)
```

であり、`m.isDryRun` でゲートされていない。本番モードでも `LoadRecord` がラップされた `os.ErrPermission` を返せば（例: 個別ハッシュレコードファイルが権限で読めない場合）、dynlib 検証・shebang 検証が **エラーなしでスキップ** される。`os.ErrPermission` へのマッチは広く、hash directory の不可視以外の権限エラーも吸収してしまう。

**緩和要因**: 本番フローでは `VerifyGroupFiles`（内部で同じレコードを読む `Verify`）が先に走り、同一の権限エラーでグループ検証自体が fail-closed になるため、現行の呼び出し順序では実害に至らない。ただしこれは呼び出し元の順序に依存した暗黙の前提であり、`VerifyCommandDynLibDeps` / `VerifyCommandShebangInterpreter` を単独で呼ぶ将来のコードパスでは fail-open になる。

**推奨対応**: `isDeferredHashDirUnavailable` による skip を `m.isDryRun` 条件でゲートする。または少なくとも `os.ErrPermission` を本番モードではエラーとして伝播させ、スキップ時には slog.Warn を残す。

---

## 🟠 Low

### L1: hasDynamicLibraryDeps が DynString のエラーを握りつぶす（fail-open）

- 該当箇所: `internal/verification/manager.go:711-715`

**問題**: `elfFile.DynString(elf.DT_NEEDED)` がエラーを返した場合、`(false, nil)` として「動的依存なし」扱いになる。動的セクションが壊れている（あるいは意図的に細工された）ELF は `ErrDynLibDepsRequired` を回避して dynlib 検証要求をバイパスする。バイナリ本体はハッシュ検証済みなので record 時と同一ファイルであることは保証されるが、record 側が同様にパースに失敗して DynLibDeps 未記録のまま通過した場合、実際には動的リンクされたバイナリがライブラリ検証なしで実行され得る。

**推奨対応**: `err != nil` と `len(needed) == 0` を区別し、パースエラーは呼び出し元に伝播させる（`elf.NewFile` の「ELF でない」判定と、「ELF だが壊れている」判定は意味が異なる）。

### L2: verifiedDepHashes キャッシュの鮮度が呼び出し順序に依存し、排他制御もない

- 該当箇所: `internal/verification/manager.go:36-40, 592-598, 654-659, 842`

**問題**: キャッシュのリセットは `VerifyCommandDynLibDeps` の冒頭でのみ行われる。`VerifyCommandShebangInterpreter` が dynlib 検証を経ずに（あるいは別コマンドの dynlib 検証の後に）呼ばれると、前コマンド実行時に検証された古いハッシュでインタプリタの再ハッシュがスキップされ得る。現行の `group_executor.go` は必ず「dynlib → shebang」の順でコマンドごとに呼ぶため成立しないが、`ManagerInterface` はこの順序契約を表現しておらず、契約はコメントのみに存在する。また `verifiedDepHashes` は mutex なしで読み書きされるため、Manager が並行利用された場合 data race になる（現状の runner はグループを直列実行するため顕在化しない）。

**推奨対応**: `VerifyCommandShebangInterpreter` 側でもコマンド単位のリセットを保証する API 形状（例: 1 コマンド分の検証をまとめた単一メソッド）にするか、順序契約を interface のドキュメントに明記する。並行利用の想定が生まれる場合は mutex を追加。

### L3: PathResolver の Stat → EvalSymlinks 間の TOCTOU と、実行可否チェックの時点性

- 該当箇所: `internal/verification/path_resolver.go:31-53`

**問題**: `validateAndCacheCommand` は `os.Stat`（symlink 追従）で存在・regular・実行ビットを確認した後、別システムコールの `filepath.EvalSymlinks` で正規化する。2 呼び出しの間にリンク先を差し替えられると、実行可否チェックとキャッシュされる解決済みパスが別ファイルを指し得る。差し替え後のパスは後段のハッシュ検証・fd-bound 実行の対象になるため改ざん自体は検出されるが、チェックの原子性という点で `safefileio` 系の open-and-fstat パターン（O_NOFOLLOW ベース）と比べ弱い。また、キャッシュヒット時は存在・実行可否を再確認しない（stale cache は fail-safe 方向だが挙動として把握しておくべき）。

**推奨対応**: EvalSymlinks で得た解決済みパスに対して Stat/実行可否チェックを行う（順序の入れ替え）か、open した fd に対する fstat で判定する。

### L4: shebang シンボリックリンク検査と実際の exec の間に残る TOCTOU 窓

- 該当箇所: `internal/verification/manager.go:907-920`（verifyInterpreterSymlinkTarget）

**問題**: `/bin/sh` 等の raw インタプリタパスを `filepath.EvalSymlinks` で検査し record 時の解決先と比較するが、実際のスクリプト実行時には**カーネルが exec 時点で shebang パスを再解決**する。検査合格後〜exec の間にシンボリックリンクを差し替えられると、検証済みでないインタプリタが起動する。インタプリタバイナリ自体のハッシュ検証（`verifyInterpreterHash`）は record 済みパスに対して行われるため、この窓を塞ぐことはできない。シンボリックリンク差し替えには通常 root 相当の権限が必要であり、残余リスクは小さい。

**推奨対応**: 完全な排除は困難（exec は kernel 側の解決）。ドキュメントに残余リスクとして明記し、可能ならインタプリタを直接 fd-bound で起動する方式（`execveat` 等）の検討余地を記録しておく。

---

## 🔵 Info

### I1: fileValidator 無効時に verifyFile が nil を返し「検証成功」として計数される

- 該当箇所: `internal/verification/manager.go:324-328, 353-356`、計数側 `156-157, 220-221`

`fileValidator == nil` のとき `verifyFile` / `verifyFileWithHash` は無条件 nil を返し、呼び出し元は `result.VerifiedFiles++` する。検証していないものを "verified" と数えるのは意味論の乖離。無効化オプション (`WithFileValidatorDisabled`) は `//go:build test` タグ配下にのみ存在し本番から到達不能なため実害はないが、"skipped" として別計上する方が正確。

### I2: newManagerInternal の冗長な条件分岐

- 該当箇所: `internal/verification/manager.go:449-454`

`if hashDir == "" { return ... }` の直後の `if hashDir != ""` は常に真であり不要。デッドコンディション（code smell）。

### I3: verifyInterpreterSymlinkTarget / verifyEnvPathResolution の doc コメントが交錯している

- 該当箇所: `internal/verification/manager.go:903-905, 922-923`

`verifyEnvPathResolution` の doc コメントの 1 行目が `verifyInterpreterSymlinkTarget` の直前に置かれ、続きが 922 行目に孤立している。godoc が壊れており、リファクタ時のコメント移動漏れとみられる。セキュリティ関数のドキュメント正確性の問題として修正推奨。

### I4: dry-run モードは hash directory のパーミッション検証を全面スキップする

- 該当箇所: `internal/verification/manager_production.go:33-48`、`internal/verification/manager.go:92-100`

`NewManagerForDryRun` は `skipHashDirectoryValidation` を立てるため、hash directory が誰でも書き込める状態でも dry-run の検証結果（verified 表示）はそのまま出る。dry-run の結果を本番安全性の根拠として読むユーザーには誤解を与え得る。設計上意図された挙動（read-only 構築・UNVERIFIED 追跡あり）だが、dry-run サマリに「hash directory permission unchecked」の注記を出すとより安全。

---

## 観察された良好な防御層

- **hash directory の固定**: 本番 API (`NewManagerForProduction` / `NewManagerForDryRun`) はコンパイル時定数 `cmdcommon.DefaultHashDirectory` のみを許可し、カスタムディレクトリは `HashDirectorySecurityError` で拒否（`manager_production.go:109-117`）。環境変数やフラグ経由の hash directory 差し替え攻撃面が存在しない。
- **TOCTOU 対策の多層化**: 設定ファイルは `VerifyAndRead` による read-once 検証（読んだバイト列そのものを検証・使用）。コマンドパスは一度だけ解決して `cmd.ExpandedCmd` にピン留めし、下流で fd-bound 実行（`group_executor.go:429-434` のコメント参照）。PathResolver のキャッシュも収集時と実行時の解決結果を一致させる方向に働く。
- **PATH 非継承**: PathResolver は環境から PATH を継承せず固定の `common.SecurePathEnv` を使用（`manager.go:497-504`）。一方 shebang の `env` 解決検査は実際にコマンドへ渡す finalEnv の PATH で再解決して record と照合しており、環境変数経由のインタプリタすり替えを検出する。
- **symlink-safe I/O**: ELF 検査・インタプリタハッシュ計算は `safefileio.SafeOpenFile` を使用（`manager.go:697, 887`）。
- **shebang チェーン検証の fail-closed 設計**: 空 path/ref のレコードは corrupt として拒否、未対応ハッシュアルゴリズムは `ErrUnsupportedHashAlgorithm` で拒否、record 未登録インタプリタは `ErrInterpreterRecordNotFound` として「改ざん」と区別しつつ拒否。symlink リダイレクトと PATH 再解決の両方を検査。
- **スキーマバージョンの非対称処理**: 新しいスキーマのレコードは拒否（未知フォーマットを信用しない）、古いスキーマは dynlib 検証のみ warn 付きスキップと、方向によって扱いを変えている（`manager.go:635-641`）。
- **per-command キャッシュリセット**: `VerifyCommandDynLibDeps` 冒頭で `verifiedDepHashes` をリセットし、前コマンドの検証結果の持ち越しによるステイルハッシュ受理を防ぐ（`manager.go:592-598`、意図がコメントで明示）。
- **テスト専用の弱化オプションの隔離**: `WithFileValidatorDisabled` 等は `//go:build test` タグの `test_helpers.go` にのみ存在し、本番バイナリから到達不能。
- **dry-run の未検証コンテンツ追跡**: 検証なしで採用されたコンテンツは `UnverifiedFileUsage` として明示的に記録され、tampering signal（hash_mismatch）と環境要因を区別して下流に伝える。
- **ResultCollector の並行安全性**: 全操作を mutex で保護し、`GetSummary` はポインタフィールドまで deep copy して data race を防止。
- **監査ログ**: Manager 生成時に呼び出し元ファイル・行番号付きで audit ログを出力（`manager_production.go:70-106`）。
