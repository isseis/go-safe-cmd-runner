# A5: internal/runner/base/risk/ セキュリティ監査所見

- 監査日: 2026-07-18
- 対象: `internal/runner/base/risk/`（実体は `evaluator.go` 565 行 + テスト群。`test_helpers.go` は `//go:build test` タグ付き）
- 方法: 静的読解（依存先 `internal/runner/base/security/`、`internal/runner/base/risktypes/`、`internal/runner/resource/normal_manager.go`・`dryrun_manager.go`、`internal/runner/group_executor.go`、`internal/runner/base/executor/fdexec_linux.go` を突き合わせて確認）

## サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 1 |
| 🟠 Low | 4 |
| 🔵 Info | 3 |

---

## 所見

### 🟡 Medium-1: openVerifiedIdentity がハッシュ検証と open の間の TOCTOU を fd 内容の再検証で塞いでいない

- 該当箇所: `internal/runner/base/risk/evaluator.go:555-565`（`openVerifiedIdentity`）、関連: `internal/runner/group_executor.go:404-439`
- 問題:
  実行バイナリのハッシュ検証（`VerifyGroupFiles`）は**グループ開始時**に一括で行われ、`cmd.ExpandedCmdContentHash` に伝播される。一方、fd バインド実行用のディスクリプタは各コマンドのリスク評価時（`allowedPlan` → `openVerifiedIdentity`）に **パス指定で再 open** される。open 後の窓は `/proc/self/fd` 経由の fd バインド実行（`fdexec_linux.go`）で閉じられているが、**「ハッシュ検証時点」と「open 時点」の間の窓**が残る。グループ内に長時間走るコマンドがあれば、この窓は数分〜数時間になり得る。open した fd の内容を `ExpandedCmdContentHash` と照合していないため、この窓で差し替えられたバイナリを「検証済みハッシュ」を audit ログに残したまま実行し得る。
  付随して `syscall.Open(path, O_RDONLY|O_CLOEXEC, 0)` は:
  - `O_NOFOLLOW` なし（`ResolveCommandNames` の解決結果と open 時のシンボリックリンク追跡が別時点）
  - `O_NONBLOCK` なし（パスが FIFO に差し替えられると open が無期限ブロック → DoS）
  - open 後の `fstat` による通常ファイル確認なし
- 悪用シナリオ: 検証済みパス（またはその中間ディレクトリ/シンボリックリンク）に書き込み権を持つ攻撃者が、グループ先頭の検証完了後〜当該コマンドの評価前にバイナリを差し替える。fd は差し替え後の inode に束縛され、audit には旧（検証済み）ハッシュが記録される。前提として検証済みパスへの書き込みが必要であり、それ自体は filevalidator/safefileio 層のパーミッション検査で抑止される設計のため、単独では成立しにくい（深層防御の欠落と評価）。
- 推奨対応: `openVerifiedIdentity` で open した **fd から** 内容ハッシュを再計算し `ExpandedCmdContentHash` と照合する（不一致は fail-closed で Blocking）。あわせて `O_NONBLOCK` を付けて open → `fstat` で通常ファイル確認 → `O_NONBLOCK` 解除、の定石を採用する。これにより検証とバインドが同一 inode に対して原子的になる。

### 🟠 Low-1: zoningParams.dedicatedTempDir が本番経路で一度も設定されない（死んだ設定項目）

- 該当箇所: `internal/runner/base/risk/evaluator.go:41`（フィールド定義）、`evaluator.go:86-93`（`NewStandardEvaluator` は未設定）、`evaluator.go:420`（`zoningInput` で参照）
- 問題: `dedicatedTempDir` は axis-2 ゾーニングの safe-zone 起点（`security.ZoningInput.DedicatedTempDir`、`destination_zoning.go:242`）として配管されているが、`NewStandardEvaluator` は設定せず、`security.Config` にも対応フィールドがない。本番では常に空文字列で、専用一時ディレクトリ（`executor/tempdir_manager.go` が作る dir）への書き込みは safe-zone にならず ordinary（Medium）に分類される。方向としては fail-closed であり脆弱性ではないが、実装と意図（コメント・仕様）が乖離した死にコードであり、将来の配線ミス時に意図しない信頼アンカーになるリスクがある。
- 推奨対応: tempdir_manager の専用一時ディレクトリを実際に配線するか、当面使わないならフィールドと `ZoningInput.DedicatedTempDir` の参照を削除して仕様との整合を取る（YAGNI）。なお `classifyZone` 側は空文字列・相対パスの起点をスキップする防御があることは確認済み。

### 🟠 Low-2: deny 経路間で audit 情報（OperandZones・エラー詳細）の欠落に非一貫性がある

- 該当箇所: `internal/runner/base/risk/evaluator.go:284-288`（coreutils 分類失敗の deny）と `evaluator.go:337-347`（binary-analysis uncertain の deny）の対比
- 問題: binary-analysis の fail-closed deny は `blocked.OperandZones = zone.Operands` でオペランド毎のゾーニング audit を保持するのに対し、coreutils 分類失敗（`CoreutilsCommandRisk` のエラー）の deny は zone 計算後にもかかわらず `OperandZones` を落とす。さらにこのとき元の `err` は破棄され（`return blockingAssessment(...), nil`）、reason code のみでエラー内容がログにも audit にも残らない。fail-closed であり安全性は保たれるが、インシデント時の監査可能性が deny 経路によってばらつく。
- 推奨対応: coreutils 失敗の deny にも `zone.Operands` を載せ、破棄している `err` を `slog.Warn` 等で記録する。

### 🟠 Low-3: applyBinaryAnalysis の switch に default がなく、未知クラスが「寄与なし」に落ちる

- 該当箇所: `internal/runner/base/risk/evaluator.go:461-477`
- 問題: `BinaryAnalysisClass` の switch は Uncertain/HighRisk/Network/Clean を列挙するのみで default がない。将来クラスが追加された場合、当該クラスは無寄与（実質 Clean 扱い）となり fail-open。現状はゼロ値が `BinaryAnalysisUncertain`（`risktypes/types.go:146-149`）である点が緩和になっているが、列挙追加時の安全側デフォルトが型システム上保証されていない。
- 推奨対応: default 節で Uncertain と同じ Blocking を返す（未知クラス = fail-closed）。exhaustive lint の適用も検討。

### 🟠 Low-4: プロファイル持ちコマンドはネットワーク引数ディメンションの対象外

- 該当箇所: `internal/runner/base/risk/evaluator.go:331-333`
- 問題: `HasNetworkArguments`（URL/SSH 形式の引数で Medium に引き上げ）は `!profileFound` のときだけ評価される。コメントは「プロファイル持ちは上で NetworkRisk を寄与済み」とするが、`ProfileFactorRisk`（`command_risk_profile.go:196`）は **プロファイルが NetworkRisk を宣言している場合のみ** 引数を見る。つまり「プロファイルはあるがネットワーク非宣言」のコマンドに URL/SSH 引数を渡した場合、どちらの経路でも Medium に上がらず Low のままになり得る。実害はプロファイル定義の網羅性に依存し限定的（非ネットワークツールに URL を渡しても通信は起きにくい）だが、コメントと実装の主張が一致していない。
- 推奨対応: 条件を `!profileFound || profile.NetworkRisk.Level <= Low` に緩めるか、コメントを実挙動（プロファイル定義に依存して免除される）に合わせて修正する。

### 🔵 Info-1: evaluateDimensions 内の filepath.IsAbs 再チェックは到達不能

- 該当箇所: `internal/runner/base/risk/evaluator.go:336`
- 問題: 非絶対パスは `EvaluateRisk` 冒頭（`evaluator.go:115-121`）で既に Blocking deny されるため、`!coreutilsHandled && filepath.IsAbs(cmdPath)` の後半は常に真。防御的再チェックとしては無害（false 側は fail-closed で binary analysis スキップだが、そもそも到達しない）だが、読み手に「非絶対パスがここまで来る」と誤解させる。
- 推奨対応: 条件を `!coreutilsHandled` に簡約するか、到達不能な防御であることをコメントで明示。

### 🔵 Info-2: zoningInput の Spec==nil ガードは fail-open 方向のフォールバック

- 該当箇所: `internal/runner/base/risk/evaluator.go:405-417`
- 問題: `cmd.Spec == nil` の場合、run_as 指定を読めないままデフォルト実行 identity にフォールバックする（panic 回避が目的）。本番では `NewRuntimeCommand` が Spec を必ず設定する不変条件があるが、仮に不変条件が破れて run_as 指定付きコマンドが Spec なしで到達すると、より弱い（=デフォルト）identity で Trusted 判定される方向。run_as 解決失敗時は `IdentityUnresolved` で fail-closed にしている設計と非対称。
- 推奨対応: `Spec == nil` も `identUnresolved = true`（fail-closed）に倒す。

### 🔵 Info-3: applyProfileFactors の Reasons 代入が上書きセマンティクス

- 該当箇所: `internal/runner/base/risk/evaluator.go:448`
- 問題: `a.Reasons = profile.GetRiskReasons()` は append ではなく代入。現在の呼び出し順（rank 5 の時点で `a.Reasons` は常に空）では実害がないが、ディメンションの追加・並べ替えで先行 Reasons が黙って消える壊れ方をする、順序依存の潜在バグ。
- 推奨対応: `append` + `common.DedupeStable`（rank 2 floor の fold と同じイディオム、`evaluator.go:210`）に揃える。

---

## 観察された良好な防御層

1. **全面的な fail-closed 設計**: 非絶対パス、シンボリックリンク解決失敗、未検証ハッシュ、解析無効、解析レコード欠落/スキーマ不一致、run_as 解決失敗、ゾーニングのオペランド数超過（`maxZoningOperands=64`）、識別子 open 失敗（`ReasonIdentityUnbound`）のいずれも Blocking deny に倒れる。曖昧さを Clean に潰す経路が見当たらない。
2. **identity gate の最優先実行**（`evaluator.go:142-144`）: ハッシュ未検証・解析無効のバイナリは、coreutils/プロファイル/ゾーニング等どのディメンションでも許可判定に到達できない。
3. **fd バインド実行による open 後 TOCTOU の遮断**: 許可プランは open 済み fd（`O_RDONLY|O_CLOEXEC`）を保持し、executor が `/proc/self/fd/<n>` を exec する（`fdexec_linux.go`）。open 以後の rename/symlink 差し替えは無効化される。プランの fd は `normal_manager.go`/`dryrun_manager.go` の双方で `defer plan.Close()` されリークしない。
4. **risk_level=critical の設定禁止**: `ParseRiskLevel`（`runnertypes/config.go:93-110`）が `critical`/`unknown` を拒否するため、特権昇格（sudo 等）の Critical 判定を設定で許可上限に入れて回避することはできない。
5. **シンボリックリンク別名対策**: 特権コマンド検出はパス名でなく `ResolveCommandNames` の解決済み名集合＋プロファイルで行い、symlink 経由の別名で sudo 検出を回避できない。解決失敗は fail-closed。
6. **ライブ identity 非依存のゾーニング判定**: run-as identity は構築時に一度解決して注入され（`OriginalExecutionIdentity()`）、判定時に euid を読まない。テスト（`live_identity_guard_test.go`）がこの性質を明示的に検証している。
7. **抑制（suppressLegacy）の慎重な粒度制御**: axis-2 が完全認識した場合のみレガシー破壊ディメンションを抑制し、部分認識では併用（ダウングレード回避）。coreutils 抑制時も setuid/setgid シグナルは保持し、stat 失敗すら High に倒す（`applyCoreutilsRisk`）。
8. **監査可能性の担保**: deny 経路でも identity gate 通過後は検証済み identity をプランに残し、オペランド毎の `OperandZones` を audit に載せる。reason code は `DedupeStable` で安定的に重複排除。
9. **注入可能フィールドの閉じ込め**: `openIdentity`/`resolveRunAs`/`zoning` は非公開フィールドで、パッケージ外（本番コード）からの差し替え不可。テスト専用ヘルパは `//go:build test` タグで本番バイナリから除外。
10. **並行安全性**: `StandardEvaluator` のフィールドは構築後読み取り専用で、評価は入力コマンド毎に独立。共有可変状態はない。
