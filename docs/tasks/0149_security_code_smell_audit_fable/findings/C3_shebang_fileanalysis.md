# C3: internal/shebang / internal/fileanalysis セキュリティ監査

- 監査日: 2026-07-19
- 対象:
  - `internal/shebang/` (parser.go, env_resolver.go, errors.go)
  - `internal/fileanalysis/` (file_analysis_store.go, schema.go, syscall_store.go, errors.go)
- 方法: 静的コードレビュー（読み取り専用）。呼び出し元（`internal/filevalidator/validator.go`,
  `internal/verification/manager.go`, `internal/runner/base/security/indirect_execution.go`）も参照。

## サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 3 |
| 🟠 Low | 5 |
| 🔵 Info | 3 |

---

## 所見

### F1. 🟡 Medium: shebang トークン分割が Unicode 空白基準でカーネル挙動と乖離（解析対象と実行対象のインタプリタが異なりうる）

- 該当箇所: `internal/shebang/parser.go:120-126`（`strings.TrimLeft(..., " \t")` + `strings.Fields`）
- 問題:
  カーネル（Linux binfmt_script / macOS）はインタプリタパスの終端を **スペース・タブ・改行のみ** で判定する。
  一方 `strings.Fields` は `unicode.IsSpace` に基づき、`\v`, `\f`, U+0085, U+00A0 (NBSP) などでも分割する。
  このため、これらの文字を含む shebang 行では、本パーサが記録・検証するインタプリタと、実行時に
  カーネルが実際に起動するインタプリタが異なりうる。
- 悪用シナリオ:
  攻撃者が allowlist 登録（record）対象として提出するスクリプトに
  `#!/bin/sh\xc2\xa0evil`（NBSP 入り）のような行を仕込む。パーサは `/bin/sh` を
  インタプリタとして記録・ハッシュ検証するが、カーネルは NBSP を空白と見なさず
  `"/bin/sh\xc2\xa0evil"` というパスの実行を試みる。攻撃者が当該パスにバイナリを
  用意できれば、検証対象外のインタプリタで実行される。スクリプト本体は
  ハッシュ検証されるため行自体の改ざんは不可能だが、「検証したものと実行される
  ものが違う」という検証バイパスが成立する。
  なお `internal/runner/base/security/indirect_execution.go:593` 周辺のランタイム側リーダは
  カーネル準拠（残余を単一トークン保持）と明記されており、2 つのリーダの分割規則が
  意図的に異なることが本乖離を残している。
- 推奨対応:
  `strings.Fields` ではなくスペース (0x20) とタブ (0x09) のみをデリミタとするバイト単位の
  分割に変更する。加えて、インタプリタトークンに制御文字・非 ASCII 空白が含まれる場合は
  fail-closed で拒否する。

### F2. 🟡 Medium: 512 バイトまでの shebang 行を受理し、Linux カーネルの 256 バイト制限と挙動が乖離

- 該当箇所: `internal/shebang/parser.go:80-112`（`maxShebangBytes = common.MaxShebangLen = 512`）、
  `internal/common/filesystem.go:236`
- 問題:
  パーサは 512 バイト以内に改行があれば shebang 行として受理する。しかし Linux の
  `BINPRM_BUF_SIZE` は 256 バイトであり、257〜511 バイトの行はカーネル側では
  「256 バイト以内の最後の空白位置で切り詰め」（Linux 5.1+、空白がなければ ENOEXEC）となる。
  つまり Linux では、record/verify が解析したインタプリタと、実行時にカーネルが解決する
  インタプリタ（切り詰め後のトークン）が異なりうる。
  `indirect_execution.go` のコメントは「大きい方の境界を使うのは Linux では fail-closed-safe」と
  述べているが、それは残余トークンを保持するランタイム評価器についての話であり、
  トークン先頭を取り出す本パーサには当てはまらない。
- 悪用シナリオ:
  256 バイト境界をまたぐよう細工した shebang 行（例: 境界の直前に空白を置き、
  切り詰め後に別の実在パスがインタプリタになるよう構成）を持つスクリプトを record させる。
  record/verify は行全体から取り出した `/trusted/interp` を検証するが、Linux カーネルは
  切り詰め後の別パスを実行する。
- 推奨対応:
  Linux では 256 バイト以内に改行がない行を `ErrShebangLineTooLong` で拒否する
  （プラットフォーム別の上限を用いる）か、より単純に「256 バイトを超える shebang 行は
  全プラットフォームで拒否」に統一する。record 側が厳しく拒否する分には fail-closed で安全。

### F3. 🟡 Medium: `SaveSyscallAnalysis` が ContentHash のみ更新し他フィールドを温存 — 新ハッシュ + 旧解析の不整合レコードを生成しうる（かつ本番未使用のデッドコード）

- 該当箇所: `internal/fileanalysis/syscall_store.go:53-71`
- 問題:
  `SaveSyscallAnalysis` は `Store.Update` 経由で `record.ContentHash = fileHash` と
  `record.SyscallAnalysis` のみを書き換え、`DynLibDeps` / `ShebangChain` /
  `SymbolAnalysis` / `AnalysisWarnings` は既存値を温存する。ファイル内容が変わった後に
  この API 単独で呼ばれると、「新しい ContentHash に旧内容由来の依存ライブラリ・
  shebang チェーン・シンボル解析が紐付いた」レコードが生成され、verify 側の
  ハッシュ一致チェック（`LoadSyscallAnalysis` の `record.ContentHash != expectedHash`）を
  通過してしまう。
  さらに、grep の結果 `SaveSyscallAnalysis` および `NewSyscallAnalysisStore` の本番呼び出しは
  存在せず（テストのみ）、コメントの「Used directly by cmd/record for saving/loading」は
  実態と乖離している。
- 悪用シナリオ:
  現状は本番経路がないため直接悪用は不可。ただし将来この API が再利用された場合、
  上記の不整合レコードにより「旧バイナリの解析結果で新バイナリのリスク評価を行う」
  検証バイパスが成立しうる（`validator.analyzeDynLibDeps` は明示的にフィールドを
  クリアしてから再解析しており、同じ配慮がここには無い）。
- 推奨対応:
  デッドコードであれば削除する（YAGNI）。維持する場合は、ContentHash が既存レコードと
  異なるときに他の解析フィールドをクリアする（stale data prevention を
  `analyzeDynLibDeps` と同様に実装する）。コメントの修正も行う。

### F4. 🟠 Low: `IsShebangScript` の単発 `Read` はショートリードで false を返す（再帰 shebang 検出が fail-open 方向）

- 該当箇所: `internal/shebang/parser.go:222-235`
- 問題:
  `f.Read(buf)` は `io.Reader` 契約上、エラーなしで 1 バイトだけ返すことが許される。
  その場合 `n < shebangPrefixLen` で `false, nil`（= shebang ではない）を返す。
  この関数は `filevalidator.checkNotShebang`（`validator.go:620-629`）で
  「インタプリタ自身がスクリプトであること」の拒否（`ErrRecursiveShebang`）に使われており、
  誤った false は再帰 shebang チェック の素通り（fail-open）方向に倒れる。
  通常の OS ファイルでは実害はほぼないが、`Parse` が `io.ReadFull` を使っているのと
  非対称であり、契約上の正しさを欠く。
- 推奨対応: `io.ReadFull(f, buf)` に変更し、`ErrUnexpectedEOF`/`EOF` のみ false 扱いとする。

### F5. 🟠 Low: `Store.Update` が破損レコードを警告なしに新規レコードで上書き（改ざん痕跡の消去）

- 該当箇所: `internal/fileanalysis/file_analysis_store.go:191-197`
- 問題:
  `RecordCorruptedError` を「not found と同じ」として黙って空レコードから作り直す。
  ログ出力（`slog.Warn` 等）が一切ないため、攻撃者や障害によりレコードが破壊されていた
  という事実が record の再実行で無警告に消える。record は運用者操作であり fail-open では
  ないが、監査証跡としては破損検出を可視化すべき。
- 悪用シナリオ:
  analysisDir に書き込める攻撃者（既に高い権限を持つが）がレコードを破壊した場合、
  次回 record 実行で痕跡が黙って消え、インシデント検知の機会が失われる。
- 推奨対応: 破損レコードを上書きする際に警告ログを出す（旧スキーマ上書き時も同様）。

### F6. 🟠 Low: record 時にハッシュ計算と shebang 解析が別 open（同一内容の保証がない TOCTOU）

- 該当箇所: `internal/shebang/parser.go:74`（`SafeOpenFile` による再 open）、
  呼び出し元 `internal/filevalidator/validator.go:596-616`
- 問題:
  ContentHash の計算と `shebang.Parse` は同一ファイルパスをそれぞれ独立に open して読む。
  record 実行中にファイルが差し替えられると、記録される ContentHash と
  ShebangChain / インタプリタ情報が異なる内容に由来する可能性がある。
  攻撃には record 実行中に対象ファイルへ書き込める権限が必要であり、その時点で
  攻撃者は多くの前提を握っているため実害は限定的。verify 時にはハッシュ不一致で
  検出される（fail-closed）ため、成立しても「誤った解析メタデータの記録」に留まる。
- 推奨対応: 可能なら 1 回の open で得た fd からハッシュ計算と先頭バイト解析の両方を行う
  （`io.TeeReader` あるいは先頭バッファの共有）。少なくとも設計上の前提をコメント化する。

### F7. 🟠 Low: record 時の env コマンド解決がプロセスの ambient PATH（`os.Getenv("PATH")`）に依存

- 該当箇所: `internal/shebang/parser.go:192`
- 問題:
  `parseEnvForm` は record を実行したプロセスの環境変数 PATH で `#!/usr/bin/env cmd` を
  解決する。verify 側（`verification/manager.go:925`）は設定由来の `envVars["PATH"]` を使う。
  両者が異なると verify は不一致 → fail-closed であり直接の危険はないが、
  (a) record 結果が実行環境ではなく record 環境の PATH に依存して非決定的になる、
  (b) record 実行者の PATH に攻撃者ディレクトリが含まれていれば、そのパスが
  「正」として記録される、という注入面がある。相対 PATH エントリのスキップ
  （`LookPathInEnv`）で一部緩和されているが、依存自体は残る。
- 推奨対応: `Parse` に PATH を明示引数で渡せるようにし、record 側でも実行時と同じ
  （設定で定義された）PATH を使って解決する。

### F8. 🟠 Low: `Store.Update` の read-modify-write に排他制御がない（並行 record での lost update）

- 該当箇所: `internal/fileanalysis/file_analysis_store.go:179-207`
- 問題:
  Load → updateFn → Save の間にファイルロックや CAS がなく、複数の record プロセス
  （または将来の並行化）が同一レコードを更新すると後勝ちで更新が失われる。
  現状 record は単一プロセスの運用者操作であり実害は低いが、部分更新 API
  （`SaveSyscallAnalysis`）と組み合わさると F3 の不整合を並行実行が助長する。
- 推奨対応: 少なくとも godoc に「並行実行非対応」と明記する。必要なら `flock` 等の
  アドバイザリロックを導入する。

### F9. 🔵 Info: 「256-byte limit」のエラーメッセージ・コメントが定数 512 と不一致

- 該当箇所: `internal/shebang/errors.go:10`（"exceeds 256-byte limit"）、
  `internal/shebang/errors.go:8-9`、`internal/shebang/parser.go:61`（"no newline within 256 bytes"）
- 問題: `maxShebangBytes` は `common.MaxShebangLen = 512` に変更済みだが、エラー文言と
  godoc が 256 のまま。運用者が誤ったしきい値を前提に判断する恐れがある。
  また `internal/fileanalysis/schema.go:27,43` の版履歴コメントにも
  「Load returns SchemaVersionMismatchError for records with schema_version != 16 / != 20」
  という現行版 (23) と矛盾する記述が残っている。
- 推奨対応: 文言を定数参照ベース（`fmt` で埋め込むか「MaxShebangLen bytes」）に修正。
  schema.go の版履歴は「その版で導入された変更」のみを述べる形に整理。

### F10. 🔵 Info: `isExecutableFile` の `os.Stat` と実行時点の間の TOCTOU（多層防御で緩和済み）

- 該当箇所: `internal/shebang/env_resolver.go:82-88`
- 問題:
  PATH 探索の実行可否判定（stat）と、その後の `EvalSymlinks`・ハッシュ検証・実際の exec の
  間には本質的に時間差がある。stat はシンボリックリンクを追う。また `#nosec G703` の根拠
  コメント「trusted PATH env value」は、record 時の ambient PATH（F7）には厳密には
  当てはまらない。実行直前のハッシュ検証（verification 層）が最終防衛線として機能する
  ため、残余リスクは低い。
- 推奨対応: 現状維持で可。`#nosec` の根拠コメントを「最終的な整合性は verify 時の
  ハッシュ検証で担保される」に改める。

### F11. 🔵 Info: 細かな code smell

- `internal/fileanalysis/file_analysis_store.go:104,111`: `os.IsNotExist(err)` は
  `errors.Is(err, fs.ErrNotExist)` が現代的（wrap されたエラーへの耐性）。
- `internal/fileanalysis/file_analysis_store.go:124,135`: `json.Unmarshal` が未知フィールドを
  黙って受理する（`DisallowUnknownFields` なし）。レコードは信頼ディレクトリ内なので
  実害は低いが、破損検出の感度は下がる。
- `internal/shebang/parser.go:103-108`: 手書きの改行探索ループは `bytes.IndexByte(line, '\n')`
  で置き換え可能。
- `internal/shebang/parser.go:69-98` と `211-236`: `Parse` と `IsShebangScript` で
  open + 先頭読み取りのロジックが重複（DRY）。

---

## 観察された良好な防御層

1. **safefileio 経由のファイルアクセス**: `Parse`/`IsShebangScript` は `SafeOpenFile`
   （openat2 `RESOLVE_NO_SYMLINKS` 相当）でスクリプトを開き、symlink 攻撃を遮断。
   `Store.Load/Save` は `NewResolvedPathParentOnly` を使い、leaf symlink を事前解決して
   検出を無効化しない理由が明確にコメントされている（`file_analysis_store.go:98-101`）。
2. **env 判定の allowlist**: `/usr/bin/env`, `/bin/env` の raw トークンのみを env(1) と
   認定し、任意パスの「env という名前のバイナリ」による解析バイパスを防止
   （`parser.go:133-171`）。symlink 解決前に判定する理由（busybox 等）も文書化。
3. **fail-closed な拒否**: env フラグ（`-S` 等）・環境変数代入・CR 混入・相対インタプリタ
   パス・改行なし長大行をすべてエラーで拒否。空/相対 PATH エントリはスキップ
   （cwd 依存の非決定性を排除）。
4. **record/verify の解決アルゴリズム共有**: `ResolveEnvCommand` を record
   （`parseEnvForm`）と verify（`verifyEnvPathResolution`）の両方が使い、実装乖離による
   誤判定を構造的に防止。
5. **symlink リダイレクト検出**: `RawInterpreterPath` を記録し、verify 時に再解決して
   `InterpreterPath` と比較（`verifyInterpreterSymlinkTarget`）。
6. **再帰 shebang の拒否**: record 時にインタプリタ／resolved command が自身 shebang
   スクリプトである場合を `ErrRecursiveShebang` で拒否（`validator.go:596-629`）。
7. **スキーマ前方互換保護**: `Store.Update` は新しいスキーマ（Actual > Expected）の
   レコード上書きを拒否し、旧バイナリによる新レコードの破壊を防止。
   Load はバージョンを先に単独パースし、スキーマ不一致と破損を正しく区別。
8. **ハッシュ束縛**: `LoadSyscallAnalysis` はキャッシュ利用前に ContentHash と期待ハッシュの
   一致を要求し、不一致は `ErrHashMismatch` で fail-closed。
9. **制限的パーミッション**: レコードファイル 0o600、ディレクトリ 0o750。`NewStore` の
   Lstat→MkdirAll TOCTOU は受容理由付きで文書化され、symlink のディレクトリは
   `Lstat` により拒否される。
