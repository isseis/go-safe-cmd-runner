# A3: `internal/runner/base/environment/` セキュリティ監査所見

- 監査日: 2026-07-18
- 対象: `internal/runner/base/environment/filter.go`（102 行）およびテストファイル
- 方法: 静的読解（読み取り専用）。呼び出し元（`internal/runner/runner.go`、`internal/runner/config/expansion.go`、`cmd/runner/main.go`）も参照して実際のデータフローを確認。

## 概要

本パッケージは「allowlist ベースの環境変数フィルタ」を名乗るが、実際には **allowlist による排除を一切行わない**（検証はコマンド実行時・展開時に遅延される設計）。設計自体は下流の `config.ProcessEnvImport` が allowlist / 禁止変数チェックを行うため fail-open ではないが、パッケージ内には「名前と実装の乖離」「死んだフィールド・エラー定義」「存在しないメソッドへの参照コメント」が集積しており、将来の呼び出し元が「フィルタ済み」と誤信して allowlist 未適用のマップを子プロセス環境に流用する事故を誘発しやすい状態にある。

所見件数: 🔴High 0 / 🟡Medium 2 / 🟠Low 2 / 🔵Info 3

---

## 所見

### F-1 🟡Medium: `Filter` の allowlist は保持されるだけで一度も参照されない（フィルタ機能の不在と命名の乖離）

- 該当箇所: `internal/runner/base/environment/filter.go:25-38`（`globalAllowlist` フィールド）、`filter.go:72-102`（`FilterSystemEnvironment` / `FilterGlobalVariables`）
- 問題:
  - `NewFilter` は allowlist を `globalAllowlist map[string]struct{}` に格納するが、このフィールドを読み出すコードはリポジトリ全体に存在しない（テストがフィールドの存在を確認するのみ）。
  - `FilterSystemEnvironment` / `FilterGlobalVariables` という名前にもかかわらず、実際の処理は「空の変数名をスキップして全変数をそのままコピーする」だけで、allowlist との照合は行われない。テスト `TestFilterGlobalVariables_SourceSystem`（filter_test.go:64-80）自体が、allowlist `{PATH, HOME}` に対して `USER` も通過することを「期待動作」として固定している。
  - `filter.go:41` のコメントは「use IsVariableAccessAllowed for filtering」と案内するが、`IsVariableAccessAllowed` というメソッドはコードベースのどこにも存在しない（過去のリファクタリングの残骸）。
- 悪用/事故シナリオ: 直接の脆弱性ではないが、将来の開発者が「`FilterSystemEnvironment` の戻り値 = allowlist 適用済み」と誤解し、その戻り値を子プロセスの環境変数構築に直接使うと、`LD_PRELOAD` や機密変数を含む全システム環境が子プロセスへ漏れる。誤用への距離が非常に近い「装填済みの footgun」である。
- 現状の緩和: 実際の allowlist / 禁止変数（`isForbiddenEnvVar`）/ POSIX 名検証は `internal/runner/config/expansion.go:308-374` の `ProcessEnvImport` で行われており、子プロセスに渡る変数は `env_import` で明示的に取り込まれたものに限られる。
- 推奨対応（いずれか）:
  1. `globalAllowlist` フィールドと `NewFilter` の引数を削除し、型名を `SystemEnvReader` 等、実態（環境の列挙）に即した名前へ変更する。`Filter*` という名前を廃止する。
  2. あるいは allowlist を実際に適用する実装へ戻す（現行設計では二重適用になるため 1 を推奨）。
  - あわせて `filter.go:41` の `IsVariableAccessAllowed` 参照コメントを削除・修正する。

### F-2 🟡Medium: `Runner.LoadSystemEnvironment` は「filters」と称して無フィルタの全環境を保持し、しかもその結果は誰も読まない

- 該当箇所: `internal/runner/runner.go:388-397`（本パッケージの `FilterSystemEnvironment` の唯一の運用呼び出し元）、`cmd/runner/main.go:421`
- 問題:
  - `LoadSystemEnvironment` の doc コメントは「loads and filters system environment variables」と述べるが、F-1 の通りフィルタは行われず、`r.envVars` には全システム環境変数（機密値を含む）が格納される。
  - さらに `r.envVars` は代入（runner.go:395）以降どこからも読み出されない死にフィールドである（grep 上、読み取り箇所なし）。つまり `cmd/runner/main.go:421` の呼び出しは、全環境変数のコピーをプロセス寿命の間メモリに保持するだけの無意味な処理になっている。
- 悪用/事故シナリオ: 直接悪用は困難だが、(a) コアダンプ/デバッガ経由での機密露出面をわずかに広げる、(b) 将来 `r.envVars` を「フィルタ済み環境」として参照する変更が入った瞬間に allowlist バイパスが成立する。
- 推奨対応: `Runner.envVars` フィールド、`LoadSystemEnvironment` メソッド、および `cmd/runner/main.go:421` の呼び出しを削除する。これにより本パッケージの `FilterSystemEnvironment` も運用上の呼び出し元を失うため、F-1 の整理と併せてパッケージ全体の縮退（`ParseSystemEnvironment` 相当のみ残す）を検討する。

### F-3 🟠Low: 未使用のエラー変数群（うち 1 つは他パッケージと重複定義）

- 該当箇所: `internal/runner/base/environment/filter.go:14-22`
- 問題: `ErrGroupNotFound` / `ErrVariableNameEmpty` / `ErrInvalidVariableName` / `ErrDangerousVariableValue` / `ErrVariableNotFound` / `ErrVariableNotAllowed` の 6 個はいずれも本パッケージ内でもリポジトリ内でも一度も返却・比較されない死んだ定義である。特に `ErrGroupNotFound` は `internal/runner/runner.go:36` に同メッセージの別インスタンスが存在し、`errors.Is` で互いにマッチしない同名エラーが二重定義されている。
- 事故シナリオ: 呼び出し側が誤って本パッケージ側の sentinel と `errors.Is` 比較を書くと常に false になり、エラーハンドリング分岐が静かに素通りする（fail-open 型のバグの温床）。
- 推奨対応: 6 個の未使用エラーを削除する。`ErrMalformedEnvVariable` に関する注記コメント（filter.go:21）も、実体が config パッケージにあるなら本パッケージから除去してよい。

### F-4 🟠Low: `FilterGlobalVariables` / `FilterSystemEnvironment` の error 戻り値は常に nil（検証が行われている錯覚を与えるシグネチャ）

- 該当箇所: `internal/runner/base/environment/filter.go:72-102`
- 問題: 両関数は `error` を返すシグネチャだが、失敗パスが存在せず常に `nil` を返す。呼び出し側（runner.go:391-394）は丁寧にエラーをラップして返しており、「ここで危険値検証が行われ、失敗し得る」という誤った印象をコードレビュー時に与える。また空名変数の検出は `slog.Warn` を出して黙って continue するため、異常な環境（本来 `ParseKeyValue` が空キーを弾くので `SourceEnvFile` 経路でしか起き得ない）でもエラーとして伝播しない。
- 推奨対応: error 戻り値を削除するか、空名を実際にエラーとして返す。前者の場合は関数名の変更（F-1）と同時に行うのが望ましい。
- 補足: 空名変数は `os.Environ()` 経由では `common.ParseKeyValue`（`internal/common/string.go:59-65`、空キーを ok=false で拒否）により到達不能であり、この分岐は `SourceEnvFile` 側の防御としてのみ意味を持つ。到達不能分岐と実効分岐が同居している点も可読性を下げている。

### F-5 🔵Info: `filter_benchmark_test.go` は package 宣言 1 行のみの空ファイル

- 該当箇所: `internal/runner/base/environment/filter_benchmark_test.go:1`
- 問題: 中身が `package environment` のみでベンチマークが 1 つも存在しない。過去のベンチマーク削除の残骸とみられる。実害はないが、ファイル名が提供しない機能を約束している。
- 推奨対応: ファイルを削除する。

### F-6 🔵Info: `ParseSystemEnvironment` が不正形式のエントリを無音でスキップする

- 該当箇所: `internal/runner/base/environment/filter.go:45-49`
- 問題: `=` を含まない、または空キーのエントリは `continue` で黙って捨てられる。`os.Environ()` 由来では通常発生しないため実害は低いが、攻撃者が細工した環境（例: `execve` で渡された `KEY` 形式のエントリ）が痕跡なく消えるため、監査ログの観点では debug レベルの記録があってもよい。
- 推奨対応: 任意。スキップ時に `slog.Debug` を追加する程度で十分。

### F-7 🔵Info: allowlist 判定ロジックの分散（DRY 観点）

- 該当箇所: `internal/runner/base/environment/filter.go:36` と `internal/runner/config/expansion.go:317`
- 問題: `common.SliceToSet(allowlist)` による set 構築が本パッケージ（未使用・F-1）と `ProcessEnvImport`（実効）の双方に存在する。実効的な allowlist 判定は config パッケージに一元化されているため現状の整合性は保たれているが、本パッケージ側の死んだ複製が「どちらが正か」の混乱源になる。
- 推奨対応: F-1 の整理で自然に解消される。

---

## 観察された良好な防御層

1. **allowlist 強制の一元化**: 実効的な環境変数アクセス制御は `config.ProcessEnvImport`（`internal/runner/config/expansion.go:308-374`）に集約されており、(a) 内部変数名の POSIX 検証、(b) システム変数名の `security.ValidateVariableName` 検証、(c) `isForbiddenEnvVar` による禁止変数（危険変数）の拒否、(d) allowlist 照合、(e) 重複定義の拒否、が実行時ではなく設定展開時に fail-closed で行われる。
2. **`common.ParseKeyValue` の堅実なエッジケース処理**（`internal/common/string.go:59-65`）: 空キー・`=` 欠落を明示的に拒否し、doc コメントに 4 つのエッジケースが文書化されている。`strings.Cut` を用いた最初の `=` での分割は、値に `=` を含む環境変数（例: `LS_COLORS`）を正しく扱う。
3. **遅延検証設計の明文化**: 「検証は実際に使用される変数に対して実行時に行う」という設計判断が filter.go:70-71, 79-80 のコメントで明示されており、全変数の値を事前検証して誤検知で起動不能になる事態を避けつつ、使用変数には検証が必ず届く構造になっている（`group_executor.go:522` の `ValidateAllEnvironmentVars` が最終防衛線）。
4. **値のログ非出力**: 本パッケージの `slog.Warn` / `slog.Debug` は変数名・件数・ソースのみを記録し、環境変数の値（機密の可能性がある）を一切ログに出さない。
5. **O(1) set 表現の採用**: `map[string]struct{}` による set 表現（CLAUDE.md の指針に準拠）。

## 結論

子プロセスへ渡る環境変数は `env_import` 明示宣言 + allowlist + 禁止変数チェックで統制されており、本パッケージ起因の直接的な脆弱性（権限昇格・インジェクション・情報漏えい）は確認されなかった。一方で、パッケージ全体が「フィルタすると称してフィルタしない」歴史的残骸となっており、誤用一歩手前の API 形状（F-1, F-2, F-4）を放置することは中期的なセキュリティ負債である。パッケージの縮退・改名によるリファクタリングを推奨する。
