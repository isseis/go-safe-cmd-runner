# A4: `internal/runner/base/security/` セキュリティ監査

対象パッケージ: `internal/runner/base/security/`
監査方法: 非テストソースの静的解析（`*_test.go` は必要に応じ参照）。コード修正なし（読み取り専用監査）。
監査日: 2026-07-18

---

## サマリ

| 重大度 | 件数 |
|--------|------|
| 🔴 High | 0 |
| 🟡 Medium | 2 |
| 🟠 Low | 4 |
| 🔵 Info | 3 |

総じて、当パッケージは fail-closed（不明・解決不能な入力を High/Reject に倒す）を徹底し、
symlink 解決・getopt 解析・間接実行(indirect execution)の各所に丁寧な設計コメントと防御が施されている。
致命的な fail-open は発見されなかった。以下は主に「防御の非対称性」「ライブ ID／ライブ FS 依存」
「過度に緩い部分一致」といった、深層防御・堅牢性の観点での指摘である。

---

## 🟡 Medium 所見

### M-1. インタプリタ用コード注入 env 変数が loader-control 拒否リストに未登録（防御の非対称性）

- **該当箇所**: `indirect_execution.go:1917` `isLoaderControlVar()`（呼び出し元 `checkEnvAssignment` `indirect_execution.go:767`）
- **問題の説明**:
  `env NAME=VALUE cmd ...` 形式の間接実行では、argv 内に直接書かれた `NAME=VALUE` 割り当てが
  `[env]` 設定の環境変数許可リスト（env allowlist）を迂回してプロセス環境に注入される。
  この危険性ゆえに、当コードは `LD_*` / `DYLD_*`（動的ローダ制御変数）を prefix 一致で
  検知し **Reject（ブロック拒否）** している。
  しかし、検証済みインタプリタに対して同等のコード注入を行える他系統の変数
  （`NODE_OPTIONS=--require=/tmp/evil.js`, `PERL5OPT=-M...`, `PERL5LIB`, `PYTHONPATH`,
  `PYTHONSTARTUP`, `BASH_ENV`, `ENV`, `RUBYOPT`, `GIT_SSH`/`GIT_EXTERNAL_DIFF` 等）は
  拒否リストに含まれておらず、`LD_PRELOAD` と異なり Reject されない。
- **再現/悪用シナリオ**:
  `env NODE_OPTIONS=--require=/tmp/evil.js node app.js` や
  `env BASH_ENV=/tmp/x.sh bash script.sh` は、`env` の inner command として解析され、
  インタプリタ本体（node/bash 等）は High と評価されるものの、割り当て自体は
  「無害な NAME=VALUE」として通過し、検証済みバイナリに未検証コードを読み込ませられる。
  `LD_PRELOAD` が同構文で完全拒否されるのに対し、これらは `risk_level = "high"` を
  許容している運用では実行され得る（深層防御の非対称性）。
- **緩和要因**: inner がインタプリタ／シェルの場合は常に High なので、`risk_level` を
  high 未満に設定していれば実行はブロックされる。影響はあくまで「High 許容運用下での
  コード注入経路」に限られる。
- **推奨対応**:
  `isLoaderControlVar`（あるいは近傍に新設する `isInterpreterCodeInjectionVar`）に、
  インタプリタ／ランタイム固有のコード・ライブラリ注入変数を拡充し、
  `LD_*`/`DYLD_*` と同様に Reject する。少なくとも `NODE_OPTIONS`, `PERL5OPT`,
  `PERL5LIB`, `PYTHONPATH`, `PYTHONSTARTUP`, `BASH_ENV`, `ENV`, `RUBYOPT`, `GIT_SSH`
  等の既知ベクタを対象に含めることを推奨。網羅困難であれば、`env` argv 内の
  割り当てを許可リスト方式（既知の無害変数のみ許可）に切り替える設計も検討。

### M-2. `SanitizeEnvironmentVariables` は「値」を検査せず「キー」パターンのみで redaction

- **該当箇所**: `environment_validation.go:9` `SanitizeEnvironmentVariables()` / `:29` `isSensitiveEnvVar()`
- **問題の説明**:
  redaction 判定は変数名のパターン一致のみに依存する（`.*PASSWORD.*`, `.*TOKEN.*`,
  `.*KEY.*`, `.*API.*` 等）。値そのものが秘匿情報（例: 秘密鍵 PEM、Bearer トークン、
  AWS アクセスキー ID/シークレット）であっても、キー名がパターンに合致しなければ素通しする。
  例えば `CONFIG_BLOB=-----BEGIN PRIVATE KEY----- ...` や `AUTH=eyJ...`（JWT）は
  キー名がパターン外なのでログ／Slack 送出時に秘匿されない。
  さらに `sensitiveEnvRegexps` は `strings.ToUpper(name)` に対して適用されるため
  （`environment_validation.go:36`）、利用者が小文字のカスタムパターンを
  `Config.SensitiveEnvVars` に設定すると **恒常的に一致しない**（設定のフットガン）。
- **再現/悪用シナリオ**:
  秘密値を持つが名前が中立な環境変数がログ出力・エラーメッセージ・Slack 通知へ
  平文で漏れる。特に `SanitizeErrorForLogging`/`SanitizeOutputForLogging` 経由の
  redaction は `redaction.Config.RedactText`（値ベース）に委ねられるが、
  `SanitizeEnvironmentVariables` 自体はキー名依存であり、両者の適用範囲差が漏洩窓になり得る。
- **推奨対応**:
  (1) 値に対する redaction（既存の `redactionConfig.RedactText` / `sensitivePatterns`）を
  `SanitizeEnvironmentVariables` にも適用し、キー・値の両面で判定する。
  (2) `sensitiveEnvRegexps` を `upperName` に適用している非対称性をドキュメント化し、
  設定パターンの正規化（例: パターン側も大文字化、または `(?i)` を強制）で
  小文字パターンの silent-miss を排除する。

---

## 🟠 Low 所見

### L-1. `EvaluateOutputSecurityRisk` のホームディレクトリ Low 判定が run-as ID でなくライブ ID を参照

- **該当箇所**: `file_validation.go:412` `user.Current()` を用いた home-dir Low 分類
- **問題の説明**:
  出力先リスク評価で「現在ユーザのホーム配下 → Low」を判定する際に `user.Current()`
  （プロセスの実効ユーザ）を用いており、当該実行の run-as 識別子ではない。
  特権昇格／降格が絡む文脈では、評価時点の実効ユーザと実際にファイルを書く ID が
  食い違い、意図しない Low 降格を招く可能性がある。
- **緩和要因**: `/root` 等は `OutputCriticalPathPatterns`（Critical、先に評価）に含まれるため
  home-dir Low へ到達する前に Critical に倒れる。実害は限定的。
- **推奨対応**: 他の判定（destination_zoning は `RunAsIdent` を注入）と整合させ、
  home-dir 判定にも run-as 識別子を用いるか、少なくともライブ ID 依存であることを明記する。

### L-2. `ValidateCommandAllowed` は入力パスの絶対性・正規化を呼び出し側保証に依存

- **該当箇所**: `validator.go:213` `ValidateCommandAllowed()`
- **問題の説明**:
  `AllowedCommands` パターンは `^/dir/.*`（`GenerateAllowedCommandsFromPath`,
  `types.go:359`）で、cmdPath は「呼び出し側が symlink 解決・clean 済み」である前提。
  空文字チェックのみ行い、絶対性・`filepath.Clean` 済みかの防御的検証がない。
  仮に未正規化パス（`/usr/bin/../../etc/evil`）が渡ると `^/usr/bin/.*` に一致し得る。
- **緩和要因**: 実際の呼び出し経路は `verification.PathResolver.ResolvePath()`
  （EvalSymlinks 相当）を通しており、clean+絶対が保証される。現状は理論上の懸念。
- **推奨対応**: 深層防御として `filepath.IsAbs(cmdPath)` および
  `filepath.Clean(cmdPath) == cmdPath` の軽量チェックを冒頭に追加。

### L-3. `DangerousRootArgPatterns` / `dangerousCommandPatterns` の過度に緩い部分一致

- **該当箇所**: `command_analysis.go:292` `HasDangerousRootArgs()`（`strings.Contains(argLower, pattern)`),
  `command_analysis.go:227` `{"chown","root"}` 等の静的パターン
- **問題の説明**:
  `DangerousRootArgPatterns = {"rf","force","recursive","all"}` を部分一致で判定するため、
  `perf`, `scarf`, パス中の `...rf...` 等に過剰一致する。また `chmod 777` パターンは
  リテラル `777` のみ一致し `0777`/`o+w` は取りこぼす。
- **緩和要因**: これらは主に dry-run 表示・補助的シグナルであり、実効リスク評価は
  `CheckDangerousArgPatterns` / `chmodGrantsHigh`（octal/symbolic を数値解釈）や
  axis-2 destination zoning が別途担保する。過剰一致は安全側、取りこぼしは他層が補完。
- **推奨対応**: code smell として整理。advisory 用途であることをコメント明記し、
  実効判定と役割を明確に分離（現状ほぼ分離済み）。

### L-4. name-based 分類がライブ FS（`os.Lstat`/`os.Readlink`）を直接使用（注入 FS を迂回）

- **該当箇所**: `command_analysis.go:669,691` `walkSymlinkChain()`,
  `command_analysis.go:821` `hasSetuidOrSetgidBit()`（`os.Stat`）,
  `operand_path_resolver.go`（production は `os.Lstat`/`os.Readlink`）
- **問題の説明**:
  `Validator` は `common.FileSystem`（`v.fs`）を注入可能だが、symlink チェーン解決・
  setuid 判定・operand 解決はグローバルな `os.*` を直接呼ぶ。評価時点と実行時点の間には
  本質的な TOCTOU 窓が存在する（評価後にリンク先が差し替わる等）。
- **緩和要因**: 実行対象バイナリのハッシュ検証（別パッケージ）が identity を担保しており、
  symlink 差し替えは検証段でも検知され得る。resolver は hop-counter とメモ化で
  無限 I/O を防ぎ、`analyzeShebang`/`readShebang` は `O_NONBLOCK`+`fstat` で
  FIFO ブロッキング・open/stat TOCTOU を閉じるなど、個別の TOCTOU 対策は良好。
- **推奨対応**: 情報として記録。評価→実行の一貫性はハッシュ検証層に依存する旨を
  設計ドキュメントで明示。テスト容易性のため注入 FS へ寄せる余地はあるが必須ではない。

---

## 🔵 Info 所見（code smell / 観察）

### I-1. 巨大な単一パッケージと複雑な getopt 状態機械

- `indirect_execution.go`（約2000行）と getopt 系（`getopt.go`, `flag_spec.go`,
  `skipLeadingOptions`/`scanShortCluster`）は極めて複雑。各所に fail-closed 方針の
  詳細コメントがあり保守性は担保されているが、認知負荷が高い。ラッパー個別ハンドラ
  （env/taskset/ip/flock/watch/nsenter…）はロジック重複が散見される。将来的な
  テーブル駆動への更なる集約余地あり（DRY）。破綻や脆弱性ではない。

### I-2. `DefaultConfig` のパターン群がハードコードで OS/ディストロ差異に非追従

- `types.go:191-260` の危険コマンド・クリティカルパス群は Linux 前提のハードコード。
  非標準配置（`/opt`, `/snap`, `/nix/store`, busybox 単一バイナリ環境等）では
  名前一致・パス一致が外れる可能性。ただし coreutils 単一バイナリ・symlink chain・
  axis-2 が補完しており、全体としては fail-closed 側に倒れる設計。

### I-3. `firstOperand`/`skipLeadingOptions` の unreliable→"" 変換に依存した fail-closed

- `firstOperand`（`indirect_execution.go:288`）は unreliable スキャン時に `""` を返し、
  呼び出し側は `""` を「サブコマンドなし＝安全側」または「不明＝High」として扱う。
  意味づけが呼び出し側ごとに異なる（coreutils multicall は High、git は lenient）ため、
  各呼び出し点の `unknownOptionPolicy` 選択ミスが将来的な fail-open を生みやすい構造。
  現状の選択は各所コメントで妥当性が説明されているが、レビュー時の注意点として記録。

---

## 観察された良好な防御層（所見とは別記録）

- **一貫した fail-closed 方針**: symlink 解決の depth/cycle 超過、operand 解決失敗、
  getopt スキャンの arity 不明（`reliable=false`）、coreutils 不明サブコマンド、
  env `-S` の解釈不能ペイロード等を、いずれも Reject / High へ倒している。
- **coreutils 単一バイナリ対応**: `CoreutilsCommandRisk` が multicall
  (`coreutils rm -rf`) を含め basename→サブコマンド分類し、setuid ビット付き
  coreutils を High、不明サブコマンドを（Medium でなく）High に倒す。
- **間接実行の網羅的検知**: wrapper（timeout/nice/env/taskset/…）、名前空間
  （chroot/unshare/nsenter, ip netns exec）、find/xargs 子プロセス exec、
  動的ローダ直接呼び出し（ld-linux*）、tar 出力フィルタ、rsync `-e`/ssh
  `ProxyCommand` 等のローカルヘルパ実行を分類し、identity 束縛不能な形式を Reject。
- **shebang 読み取りの TOCTOU/DoS 対策**: `O_RDONLY|O_NONBLOCK` で FIFO/デバイスの
  ブロッキングを回避し、open 済み fd を `fstat` して regular file を確認（open/stat 間の
  スワップを封じる）。カーネルの shebang 解釈（余剰部を単一トークンとして保持）に忠実で
  `env -S` ペイロードの取りこぼしを防ぐ。
- **rsync `-e` / ssh `-o ProxyCommand` の最悪ケース合成**: 複数出現を「最も制限的な
  結果へマージ」し、良性オプションで危険オプションを隠す攻撃を封じる（fail-closed）。
- **destination zoning（axis-2）の対称的 fail-closed**: 解決不能 operand は
  write=High / read=Medium、run-as identity 不明時は全 operand を ZoneUnresolved に倒す。
  `isTrustedOperand` は run-as が root の場合に degenerate（Low を得られない）とし、
  sticky world-writable(/tmp) を repoint リスクから正しく除外。
- **operand 解決の TOCTOU 意識**: 最終要素も `lstat`/`readlink` のみで解決し、
  `os.Stat`（leaf symlink 追従）を意図的に排除。symlink 先の所有者で誤分類されるのを防ぐ。
- **loader 制御変数の prefix 拒否**: `LD_*`/`DYLD_*` を許可リストでなく prefix 一致で
  拒否し、`LD_AUDIT`/`LD_DEBUG` 等の亜種も一括ブロック（DYLD は全 OS で拒否）。
- **path traversal 防止**: `isSimpleUnitName`（service init script）で `/`・`..` を排除し
  `/etc/init.d/<name>` のディレクトリ脱出を防止。
- **ロギング redaction の secure default**: `DefaultLoggingOptions` は
  `IncludeErrorDetails=false`, `RedactSensitiveInfo=true`, stdout truncate=true。
