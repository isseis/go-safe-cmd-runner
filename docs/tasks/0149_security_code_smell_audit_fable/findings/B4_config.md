# セキュリティ監査レポート: `internal/runner/config/`

- 対象: `internal/runner/config/` パッケージ（`*_test.go` を除くソース中心）
- 監査種別: 静的解析（読み取り専用）
- 監査日: 2026-07-18
- 監査担当: Claude (Fable) 自動監査

対象ファイル（主なもの）:
`loader.go`, `path_resolver.go`, `expansion.go`, `validation.go`,
`template_expansion.go`, `template_inheritance.go`, `template_loader.go`,
`errors.go`

---

## サマリ

| 重大度 | 件数 |
|--------|------|
| 🔴 High | 0 |
| 🟡 Medium | 2 |
| 🟠 Low | 4 |
| 🔵 Info | 3 |
| **合計** | **9** |

全体として、変数展開・テンプレート展開・許可リスト処理は多層防御が丁寧に実装されており、
再帰深度制限・変数数上限・配列/文字列サイズ上限など DoS 対策も明示的に入っている。
致命的（High）な欠陥は検出されなかった。以下は改善余地のある所見。

---

## 🟡 Medium 所見

### M-1. env_import で取り込んだシステム環境変数の値が再帰的に再展開される（テンプレート構文インジェクション面）

- 該当: `internal/runner/config/expansion.go:100-155`（`resolveAndExpand` の resolver）、
  値の供給元は `expansion.go:368-370`（`value := systemEnv[systemVarName]`）
- 問題:
  `ProcessEnvImport` はシステム環境変数の値を**生のまま** `expandedVars` に格納する
  （`result[internalName] = value`）。その後 `ExpandString`→`resolveAndExpand` は、
  参照された変数の値に対して `resolveAndExpand(value, ...)` を**再帰的に呼び出し**、
  値の中の `%{...}` / エスケープ列 `\%`・`\\` を再解釈する。
  つまり許可リストで取り込まれたシステム環境変数の**値**が、後段の
  `env` / `verify_files` / `cmd` / `args` / `cmd_allowed` / `workdir` 等に埋め込まれる際、
  テンプレート構文として二次評価される。
- 悪用/再現シナリオ:
  - 許可リストに載っているシステム環境変数（例 `DEPLOY_TAG`）の値を攻撃者が制御でき、
    その値を `%{OTHER_SECRET}` のようにすると、参照時に別の内部変数の値へ展開されうる。
    別のフィールドへ機密値を「回り込ませる」経路になり得る。
  - 値に単独の `\`（末尾バックスラッシュ）や不正エスケープ `\x` が含まれると
    `ErrInvalidEscapeSequenceDetail` で **設定ロードが失敗**（想定外の値による fail-closed だが、
    正当な値に `%` や `\` を含むケースで実運用上の誤検知/DoS になりうる）。
- 影響評価: 参照先が未定義なら確実にエラー（fail-closed）、変数名は検証され、循環は検出される。
  よって任意コード実行には直結しないが、「インポート値＝データ」であるべきものが
  「テンプレート＝コード」として再解釈される設計は最小権限・データ/コード分離の観点で望ましくない。
- 推奨対応:
  env_import で取り込んだシステム値は**リテラル**として扱い、再スキャン対象から除外する
  （例: 取り込み値を `%`・`\` についてエスケープしてから格納する、あるいは
  「展開済み確定値」として resolver が再帰しない経路に載せる）。
  少なくとも、システム由来の値に対する二次展開の有無を仕様として明文化し、テストを追加する。

### M-2. `expandCmdAllowed` の `EvalSymlinks` による許可パス確定と実行時の TOCTOU

- 該当: `internal/runner/config/expansion.go:1004-1020`（`filepath.EvalSymlinks`）
- 問題:
  cmd_allowed は設定ロード時に `EvalSymlinks` で実体パスへ正規化し集合に格納する。
  一方、実際のコマンド実行はその後の別タイミングで行われる。ロード時点と実行時点の間に
  当該パス（またはその親ディレクトリ）のシンボリックリンク先が差し替えられると、
  許可判定の前提（ロード時の実体）と実行対象の実体がずれる TOCTOU が理論上成立する。
- 悪用/再現シナリオ:
  許可リスト対象ディレクトリに書き込み可能な攻撃者が、ロード後・実行前に
  リンク先を悪意あるバイナリへ張り替える。ハッシュ検証層があれば実行は阻止されうるが、
  許可リスト単体では判定が陳腐化する。
- 影響評価: 本リポジトリはファイル整合性検証（filevalidator/verification）を別層に持つため、
  実害は緩和される可能性が高い。とはいえ config 層単体では検証と実行の一貫性は保証していない。
- 推奨対応:
  TOCTOU がハッシュ検証層で確実に閉じられていることを設計上明記する。
  可能なら実行直前に実体パスの再確認（openat + O_NOFOLLOW 系）を行う方針をドキュメント化する。

---

## 🟠 Low 所見

### L-1. `env_vars` の KEY に対して禁止環境変数（`LD_*` 等）を config 層で拒否していない

- 該当: `internal/runner/config/expansion.go:806-852`（`ProcessEnv`）
- 問題:
  `ProcessEnvImport` は取り込み対象の**システム変数名**について `isForbiddenEnvVar` で
  `LD_*` / `GCONV_PATH` 等を拒否する（`expansion.go:352-356`）。しかし `env_vars` に
  ユーザーが直接書く `["LD_PRELOAD=/evil.so"]` は KEY を `security.ValidateVariableName` で
  形式検証するのみで、禁止名チェックを行わない。実際には実行層 `BuildProcessEnvironment`
  （`internal/runner/base/executor/environment.go:86-95`）が最終的に `LD_*` と exact 集合を削除するため
  子プロセスには渡らない（多層防御として良好）。
- 影響評価: 実害は実行層のスクラブで閉じているが、config 層では「設定した env が黙って剥がされる」
  ため、ユーザーの意図と挙動が乖離し、fail-silent な混乱を生む。
- 推奨対応:
  config 層でも `env_vars` KEY に対し `isForbiddenEnvVar` を適用し、明示的にエラーとする
  （ロード時点で誤設定を検出できる／実行層との二重防御になる）。

### L-2. 本番バイナリに検証スキップ経路（`verificationMgr == nil`）が残る

- 該当: `internal/runner/config/loader.go:107-135`（`loadTemplate` の nil 分岐）
- 問題:
  `NewLoader` は nil を panic で弾く（`loader.go:35-46`）が、`loadTemplate` 内の
  `if l.verificationMgr != nil { ... } else { /* 検証なし読み込み */ }` 分岐そのものは
  build tag に依存せず本番バイナリにもコンパイルされる。安全性が「コンストラクタでの
  非 nil 保証」という不変条件のみに依存している。
- 影響評価: 現状の生成経路では到達不可のはずだが、将来 `Loader` を別経路で構築した場合に
  無検証読み込みへフォールバックする fail-open リスクが潜在する。
- 推奨対応:
  無検証パスをテスト専用ファイル（`//go:build test`）へ隔離するか、
  本番経路では nil を即エラー化する防御的チェックを `loadTemplate` にも入れる。

### L-3. エラーメッセージへ変数値・コンテキスト文字列が埋め込まれる（ログ経由の機密露出面）

- 該当: `internal/runner/config/errors.go:232-238`（`ErrUndefinedVariableDetail`、`context:` に入力文字列全体）ほか
  `ErrInvalidEscapeSequenceDetail`（253行）、`ErrUnclosedVariableReferenceDetail`（267行）、
  `ErrInvalidEnvImportFormatDetail`（309行, `Mapping` を出力）等
- 問題:
  展開対象の元文字列（`Context`）や env_import マッピング（`Mapping`）をそのままエラー文へ含める。
  これらは機密値（トークン等を含む env 値やパス）を含みうる。エラーが監査ログ等へ出力されると
  redaction 層を通らない限り機密が平文で残る恐れ。
- 影響評価: 変数**名**中心の箇所が多く直接の値露出は限定的だが、`Context`/`Mapping` は
  値を含みうる。redaction 層の有無に依存。
- 推奨対応:
  値を含みうるフィールドはログ出力前に redaction を通す方針を徹底、
  もしくはエラー文では値を切り詰め/マスクする。

### L-4. `checkTemplateNameField` が TOML を二重パースし、失敗時に黙ってスキップ

- 該当: `internal/runner/config/loader.go:256-288`
- 問題:
  構造体デコード（`DisallowUnknownFields`）後に、同じ内容を `map[string]any` として再度
  `toml.Unmarshal` し、失敗したら `return nil`（チェック省略）する。構造体デコードが通れば
  マップ化も通る前提だが、パーサ差異があった場合に禁止フィールド `name` 検出を静かに素通りさせる。
- 影響評価: 実害は小さい（`name` 混入は仕様逸脱の検出漏れ程度）だが、
  「パース失敗＝チェック無効化」は fail-open 的パターン。二重パースは DRY/効率面の code smell でもある。
- 推奨対応:
  マップパース失敗時はエラーを返す（構造体デコードが成功している以上、失敗は異常）。
  可能なら二重パースを避け、デコーダのメタ情報で検出する。

---

## 🔵 Info 所見

### I-1. 変数展開 resolver の重複実装

- 該当: `expansion.go:113-152`（`resolveAndExpand` 内 resolver）と
  `expansion.go:439-529`（`varExpander.resolveVariable`）
- 内容:
  「変数を引く→再帰展開→visited をマーク/アンマーク」というロジックが 2 箇所に重複気味。
  循環検出の visited 管理が経路ごとに微妙に異なり、将来の改修で片方だけ直す事故が起きやすい。
  機能的欠陥ではないが保守性の code smell。統一を検討。

### I-2. `path_resolver.go` のコメントが実装より強い保証を主張

- 該当: `path_resolver.go:22-25, 65-67`
- 内容:
  インターフェースコメントは「path traversal 対策」「symlink safety を safefileio でチェック」と
  述べるが、`ResolvePath` 自体は `filepath.Clean` + 存在確認のみで、symlink 検査は
  「実ファイル読み込み時に FS 抽象が行う」とコメントで委譲している（実際の防御は
  `safefileio.SafeReadFile` / verification 層）。命名・コメントと実装の責務境界が曖昧。
  実際の防御は下流に存在するため実害はないが、コメントを実態に合わせて明確化すると良い。

### I-3. 観察された良好な防御層（記録）

- **DoS 上限の明示**: `MaxRecursionDepth=100`, `MaxVarsPerLevel=1000`,
  `MaxArrayElements=1000`, `MaxStringValueLen=10KB`（`expansion.go:18-34`）で
  再帰・変数数・配列長・文字列長を厳格に制限。
- **循環参照検出**: `processVarRefs` の `visited` マップと `expansionChain` による
  詳細な循環検出（`expansion.go:250-258`）。
- **禁止ローダ変数の多層拒否**: config 層の `isForbiddenEnvVar`（`LD_*` prefix + exact 集合、
  `expansion.go:279-304`）に加え、実行層 `BuildProcessEnvironment` でも `LD_*` と exact 集合を
  最終スクラブ（`executor/environment.go:81-95`）。DYLD_ を含む扱いは security 層でも重複防御。
- **予約プレフィックス保護**: 内部変数 `__runner_` プレフィックス（`validation.go:14, 94-101`）と
  テンプレート名 `__` プレフィックス（`template_expansion.go:596-601`）を予約し衝突を防止。
- **変数スコープ検証**: global（大文字始まり）/ local（小文字始まり）のスコープを
  レベルに応じて強制（`validation.go:103-119`）。テンプレートでは local 変数参照を禁止し、
  未定義グローバル変数参照も検出（`template_expansion.go:1076-1160`）。
- **env_vars KEY へのプレースホルダ注入禁止**: テンプレート env_vars の KEY 部分に
  `${...}` を許さない（`template_expansion.go:485-523`、セキュリティ制約として明記）。
- **cmd_allowed の厳格正規化**: 生文字列重複検出→展開→空文字/相対パス/長さ検証→
  `EvalSymlinks` 正規化→実体パス重複検出、の 7 段パイプライン（`expansion.go:953-1024`）。
- **allowlist 継承モデル**: `nil=継承 / 空配列=全拒否` を `determineEffectiveEnvAllowlist` で
  明確化（`expansion.go:854-862`）。fail-closed 寄りで妥当。
- **厳格 TOML デコード**: 本体・テンプレートとも `DisallowUnknownFields()` で未知フィールドを拒否
  （`loader.go:218`, `template_loader.go:25`）。

---

## 補足（監査範囲・限界）

- 本監査は config パッケージのソース静的読解に基づく。TOCTOU（M-2）や env 再展開（M-1）の
  実害度は、下流の filevalidator / verification / executor スクラブ層の挙動に依存するため、
  それら層と組み合わせた動的検証は本監査の範囲外。
- コード修正は行っていない（読み取り専用監査）。
