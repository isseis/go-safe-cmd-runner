# A7: `internal/runner/base/audit/` セキュリティ監査

- 監査日: 2026-07-18
- 対象: `internal/runner/base/audit/`（`logger.go` 342 行、`test_helpers.go`、参照として `logger_test.go`）
- 関連参照: `internal/redaction/redactor.go`（RedactingHandler の再帰挙動）、`internal/runner/base/executor/executor.go`（呼び出し側）

## サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 3 |
| 🟠 Low | 2 |
| 🔵 Info | 5 |

本パッケージは特権実行・リスク判定の監査ログ出力に特化した小さなパッケージであり、
`LogRiskProfile` 系は境界 redaction を含め丁寧に設計されている。一方で、
旧来からある `LogUserGroupExecution` / `LogSecurityEvent` / `LogPrivilegeEscalation` は
同等の境界 redaction を持たず、防御レベルが非対称になっている点が主要な所見である。

---

## 所見

### 🟡 M-1: `LogUserGroupExecution` が失敗時に stdout/stderr を境界 redaction なしで記録（Slack 通知付き）

- 該当箇所: `internal/runner/base/audit/logger.go:100-109`
- 問題:
  コマンド失敗時（`ExitCode != 0`）、`result.Stdout` / `result.Stderr` の全文を
  `slog.String` でそのままログ属性に載せ、さらに `slack_notify=true` を付与して
  外部（Slack webhook）へ送出される経路に乗せている。
  `LogRiskProfile` が持つ境界 redaction（`argRedactor.RedactText`）はここには適用されていない。
  - `slog.String` なので、production 構成では `RedactingHandler` のパターンベース
    redaction（`KindString` 分岐、`redactor.go:455-468`）が効くが、これは
    `password=...` / `token: ...` 等の既知パターンのみを対象とする。任意コマンドの
    出力に含まれる秘密（環境ダンプ、証明書、失敗時にツールが echo する API キー等）は
    パターンに一致しなければ平文で残る。
  - `NewAuditLogger()` は `slog.Default()` に依存するため（`logger.go:40-42`）、
    ハンドラ構成に依存した「ベストエフォート」の防御しかない。`LogRiskProfile` の
    doc コメント（`logger.go:27-30`）が明示的に警戒しているケース
    （RedactingHandler の無い logger への出力）が、本メソッドでは無防備。
- 悪用/事故シナリオ:
  特権昇格して実行したコマンドが失敗し、stderr に接続文字列や秘密鍵パスフレーズを
  出力 → 監査ログファイルおよび Slack チャネルに全文が転送され、閲覧権限の広い
  チャネルへ秘密が拡散する。
- 推奨対応:
  1. `LogRiskProfile` と同様に、この境界でも `argRedactor.RedactText` を stdout/stderr
     と `command_args` に適用する。
  2. stdout/stderr は先頭 N バイトへの truncation を検討する（ログ肥大・DoS 面でも有効）。

### 🟡 M-2: 境界 redaction の適用がメソッド間で非対称（`command_args` 等）

- 該当箇所: `internal/runner/base/audit/logger.go:71,73`（`LogUserGroupExecution`）、
  `logger.go:152-194`（`LogSecurityEvent`）、`logger.go:114-149`（`LogPrivilegeEscalation`）
- 問題:
  `LogRiskProfile` のみが `argRedactor` による境界 redaction を実装し
  （`logger.go:255-261, 290-306`）、他の 3 メソッドは `RedactingHandler` の存在を
  暗黙の前提にしている。特に `LogUserGroupExecution` の
  `command_args` / `expanded_command_args`（引数を空白 join した文字列）は
  秘密を含む典型フィールド（`--password=xxx` 等）である。
  パターン一致すればハンドラ側で救われるが、「audit パッケージ単体で masking を
  保証する」という `LogRiskProfile` が確立した不変条件がパッケージ内で一貫していない。
- 悪用/事故シナリオ:
  テスト・サブコマンド・将来のツールが素の `slog.Logger` を渡して
  （`NewAuditLoggerWithCustom`、または `slog.Default()` が未ラップの初期化順序で）
  audit logger を構築した場合、引数中の秘密が平文でログに残る。
- 推奨対応:
  4 メソッドすべてで、文字列系のユーザ由来フィールド（args、stdout/stderr）に
  `argRedactor.RedactText` を適用し、「audit パッケージを通る限り masking される」
  という単一の不変条件に揃える。

### 🟡 M-3: `LogSecurityEvent` の `details` map — redaction バイパスと属性キー衝突

- 該当箇所: `internal/runner/base/audit/logger.go:171-174`
- 問題（2 点）:
  1. **redaction バイパス**: `details` の値は `slog.Any` で出力される。
     `RedactingHandler.processKindAny`（`redactor.go:522-543`）は `LogValuer` と
     slice のみ処理し、**map や struct は素通し**、slice でも非 LogValuer 要素は
     そのまま保持される（`redactor.go:727-730`）。つまり `details` に
     `map[string]string{"api_key": "..."}` やネストした構造体を渡すと、
     RedactingHandler が有効な production 構成でも秘密が一切マスクされずに出力される。
     キー名ベースの `IsSensitiveKey` 検査もトップレベルのキーにしか効かない。
  2. **キー衝突（log forging に類する属性偽装）**: `details` のキーは無検証で
     属性に追加されるため、`severity` / `audit_type` / `slack_notify` /
     `decision` 等のスキーマ用キーと重複し得る。slog は重複キーを両方出力し、
     JSON パーサの多くは後勝ちで解釈するため、下流の監査解析・アラート抑制の
     判断を汚染できる（例: `details["severity"]="info"` で SIEM 側の重大度が上書き）。
- 悪用/事故シナリオ:
  攻撃者が直接呼べる API ではないが、呼び出し側が「検出した悪性入力そのもの」を
  `details` に入れる用途のメソッドであり、悪性入力側が秘密パターン回避や
  キー衝突を誘発する値を含み得る。また善意の呼び出しでも map 値の秘密が漏れる。
- 推奨対応:
  - `details` の値を文字列化して `RedactText` を通す（または
    `RedactLogAttribute` 相当の再帰処理を map に拡張する）。
  - スキーマ予約キー（`audit_type`, `severity`, `slack_notify`,
    `message_type` 等）と衝突するキーは prefix（例: `detail.`）を付けて隔離する。

### 🟠 L-1: `LogSecurityEvent` の severity 判定が二重実装で、未知の severity に fail-open

- 該当箇所: `internal/runner/base/audit/logger.go:177`（定数比較）と
  `logger.go:186-193`（文字列リテラル switch）
- 問題:
  Slack 通知判定は `common.SeverityCritical` / `common.SeverityHigh` 定数を使うが、
  ログレベルの switch はリテラル `"critical"`, `"high"`, `"medium"` を使っており
  DRY 違反（定数変更時に片方だけ壊れる）。さらに `severity` は自由文字列であり、
  タイプミスや大文字（`"Critical"`）を渡すと `default` 分岐で **Info レベル・
  Slack 通知なし**に落ちる。セキュリティイベントのログとして fail-open
  （不明なものほど目立たなくなる）な設計になっている。
- 悪用/事故シナリオ:
  呼び出し側の新規コードが `"CRITICAL"` を渡す → 重大イベントが Info に沈み、
  Warn/Error ベースの監視・Slack アラートから漏れる。
- 推奨対応:
  severity を型付き enum にする、switch を `common.Severity*` 定数に統一する、
  そして未知の severity は保守的に Warn 以上 + Slack 通知にフォールバックする
  （fail-closed 方向のデフォルト）。

### 🟠 L-2: `chain[].path` は redaction 非適用（意図的なリスク受容だが非対称）

- 該当箇所: `internal/runner/base/audit/logger.go:265-277`（chain 出力）、
  同 `logger.go:283-289` のコメント
- 問題:
  `operand_zones` の `raw`/`resolved`/`unresolved_err` は境界 redaction を通すのに対し、
  `chain` の `path` はマスクなしで `map[string]string` として出力される。
  コメント自身が「RedactingHandler は slog.Any 経由の slice/map 要素に再帰しない」
  と認めており、`chain.Path` はどの層でもマスクされない。パスは通常低リスクだが、
  間接実行チェーンの解析対象が URL 風文字列やトークン埋め込みパス
  （`/tmp/token=xxx.sh` 等）を返した場合、平文で漏れる。
- 推奨対応:
  一貫性のため `chain[i].path` にも `argRedactor.RedactText` を適用する
  （固定パスなら redaction は no-op であり、コストは僅少）。

### 🔵 I-1: `NewAuditLogger` が構築時点の `slog.Default()` をキャプチャ

- 該当箇所: `internal/runner/base/audit/logger.go:40-42`
- 構築後に `slog.SetDefault` でハンドラ（RedactingHandler / Slack handler）が
  差し替わっても audit logger には反映されない。bootstrap の初期化順序
  （logger 構成 → runner 構築）への暗黙依存がある。現状の唯一の生成箇所
  `internal/runner/runner.go:179` は順序上問題ないが、順序保証はコード上に表現されていない。

### 🔵 I-2: `timestamp` が Unix 秒精度

- 該当箇所: `logger.go:68,126,162,209`
- slog Record 自体の時刻（ナノ秒精度）と重複し、かつ秒精度では高速連続イベントの
  順序復元に不十分。フォレンジック用途ならミリ秒以上（`UnixMilli`）を推奨。
  重複を許容するなら少なくとも精度は揃えるべき。

### 🔵 I-3: 呼び出し側での監査スキップ経路（参考: executor 側、A2 スコープ）

- 該当箇所: `internal/runner/base/executor/executor.go:245-248, 251`
- `WithPrivileges` がエラーを返した場合は早期 return で `LogUserGroupExecution` が
  呼ばれず、また `e.AuditLogger != nil` ガードにより nil 注入時は監査が静かに
  スキップされる。特権実行の失敗こそ監査価値が最も高いイベントであり、
  「deny でも必ず書く」`LogRiskProfile` の設計思想（`logger.go:200-202`）と対照的。
  audit パッケージ自体の欠陥ではないため Info とする（A2 監査と重複確認を推奨）。

### 🔵 I-4: `LogUserGroupExecution` の nil 引数未検査

- 該当箇所: `logger.go:58-64`
- `cmd` / `result` が nil の場合は panic する。監査の黙殺（fail-open）ではなく
  実行停止（fail-closed 方向）になるため実害は小さいが、監査ロガーとしては
  panic より「フィールド欠落を明示した監査エントリを書く」方が望ましい。

### 🔵 I-5: 特権昇格失敗のログレベルが Warn

- 該当箇所: `logger.go:147`
- `LogPrivilegeEscalation` の失敗は Warn で記録される。`LogUserGroupExecution` の
  コマンド失敗（Error）や `LogSecurityEvent` の high/critical（Error）と比べて
  一段低い。特権昇格の失敗は攻撃兆候（権限設定の改変・バイナリ差し替えの試行）
  でもあるため、Error への引き上げを検討する価値がある（Slack 通知フラグは
  既に付与されており、通知面の実害はない）。

---

## 観察された良好な防御層

1. **`LogRiskProfile` の境界 redaction（defense-in-depth）**: `command_args` と
   `operand_zones` の `raw`/`resolved`/`unresolved_err` に `argRedactor.RedactText`
   を適用（`logger.go:255-261, 290-306`）。RedactingHandler が slice/map 要素に
   再帰しないという下流の制約を正しく認識し、コメントで根拠まで明文化している。
   `TestLogRiskProfile_ArgMasking` / `TestLogRiskProfile_OperandZoneMasking` で検証済み。
2. **deny の severity floor**: `riskLogLevel`（`logger.go:326-342`）で deny は最低
   Warn に引き上げられ、低リスク判定の deny が Info/Debug に沈まない。
   `TestLogRiskProfile_DenySeverityFloor` で検証済み。
3. **相関フィールドの nil/`"n/a"` 境界マーカー方式**（`logger.go:18-24, 314-319`）:
   DTO に sentinel 文字列を持ち込まず、出力境界のみでマーカーを描画。キーが常に
   存在するためインシデント検索が一様に行える。
4. **成功時は stdout/stderr を記録しない**（`logger.go:96-97`）: 漏洩面の最小化。
5. **「deny を含め監査エントリは必ず書かれる」設計**（`logger.go:200-202` の
   doc コメント）: エラー early-return による監査スキップを設計レベルで排除。
6. **失敗系イベントへの `slack_notify` / `message_type` の一貫した付与**と、
   `slices.Clone` / 明示的 copy による attrs スライスの aliasing 回避
   （`logger.go:106-108, 141`）。
7. **テスト網羅**: 15 のテスト関数がログレベル、マスキング、相関フィールドの
   欠落表現、chain/operand_zones の直列化、severity floor をカバーしており、
   セキュリティ関連分岐のテスト密度は高い。
8. **テスト専用コンストラクタの build tag 隔離**（`test_helpers.go` の
   `//go:build test`）: 任意 logger 注入経路が production バイナリに含まれない。

## 総評

パッケージ全体として High 相当の脆弱性は見つからなかった。中心的なリスクは
「新設計（`LogRiskProfile`）が確立した境界 redaction の不変条件が、旧来の
3 メソッドに遡及適用されていない」という非対称性であり、M-1〜M-3 はいずれも
同じ根本原因（RedactingHandler 依存の暗黙前提と、slog.Any 経由の map/slice が
redaction されないという下流制約）に帰着する。境界 redaction の統一適用で
まとめて解消できる。
