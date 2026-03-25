# 実装計画書: `pkey_mprotect(PROT_EXEC)` 静的検出

## 進捗状況

- [ ] Phase 1: テスト先行実装
- [ ] Phase 2: 本体実装
- [ ] Phase 3: 動作確認・整合性検証

---

## Phase 1: テスト先行実装

### 1.1 `EvalProtExecRisk` 拡張テスト（`prot_exec_risk_test.go`）

- [ ] `pkey_mprotect exec_confirmed → true` テストケースを追加
- [ ] `pkey_mprotect exec_unknown → true` テストケースを追加
- [ ] `pkey_mprotect exec_not_set → false` テストケースを追加
- [ ] `mprotect exec_not_set + pkey_mprotect exec_unknown → true` テストケースを追加
- [ ] `both exec_not_set → false` テストケースを追加

この時点では EvalProtExecRisk は未変更のため、pkey_mprotect テストは失敗する（RED）。

### 1.2 x86_64 pkey_mprotect テスト（`syscall_analyzer_test.go`）

- [ ] `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs` 関数を追加
  - [ ] `PROT_EXEC confirmed (64bit rdx)` ケース（syscall 329 + `mov $0x7, %rdx`）
  - [ ] `PROT_EXEC confirmed (32bit edx)` ケース（syscall 329 + `mov $0x4, %edx`）
  - [ ] `PROT_EXEC not set` ケース（syscall 329 + `mov $0x3, %rdx`）
  - [ ] `indirect register setting` ケース（syscall 329 + `mov %rsi, %rdx`）
  - [ ] `pkey_mprotect syscall only` ケース（syscall 329 のみ）
  - [ ] `control flow boundary` ケース（`jmp` を挟む構成）
  - [ ] `non-pkey_mprotect syscall only` ケース（syscall 10 のみ → pkey_mprotect エントリなし）

### 1.3 arm64 pkey_mprotect テスト（`syscall_analyzer_test.go`）

- [ ] `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64` 関数を追加
  - [ ] `exec_confirmed (mov x2, #7)` ケース（syscall 288 + `mov x2, #0x7`）
  - [ ] `exec_not_set (mov x2, #3)` ケース（syscall 288 + `mov x2, #0x3`）
  - [ ] `exec_unknown (indirect register setting)` ケース（syscall 288 + `mov x2, x1`）
  - [ ] `exec_unknown (pkey_mprotect syscall only)` ケース（syscall 288 のみ）
  - [ ] `exec_unknown (control flow boundary)` ケース（`b` を挟む構成）

### 1.4 共存テスト（`syscall_analyzer_test.go`）

- [ ] `TestSyscallAnalyzer_MprotectAndPkeyMprotect` 関数を追加
  - [ ] `both detected: exec_confirmed + exec_confirmed` ケース
  - [ ] `both detected: exec_not_set + exec_unknown` ケース
  - [ ] `only mprotect detected` ケース
  - [ ] `only pkey_mprotect detected` ケース

---

## Phase 2: 本体実装

### 2.1 `schema.go` 更新

- [ ] `CurrentSchemaVersion` を 6 → 7 に変更
- [ ] コメントに `// Version 7 adds pkey_mprotect PROT_EXEC detection.` を追記
- [ ] `Load returns SchemaVersionMismatchError for records with schema_version != 7.` に更新

### 2.2 `syscall_analyzer.go` 更新

- [ ] `maxValidSyscallNumber` のコメントを更新（`0-288` → `up to 335 (as of Linux 6.x)`）
- [ ] `evalSingleMprotect` に `syscallName string` 引数を追加し、`SyscallName: "mprotect"` のハードコードを `SyscallName: syscallName` に置き換える
- [ ] `evaluateMprotectArgs` を `evaluateMprotectFamilyArgs` に改名
  - [ ] 戻り値を `(*SyscallArgEvalResult, uint64)` → `([]common.SyscallArgEvalResult, []uint64)` に変更
  - [ ] `mprotect` ファミリー（`"mprotect"`, `"pkey_mprotect"`）に対してループ処理を追加
  - [ ] 各 syscall 名ごとに集約ロジック（最大1件/名前）を適用
  - [ ] `evalSingleMprotect` 呼び出しに `syscallName` 引数を追加
- [ ] `analyzeSyscallsInCode` 内の `evaluateMprotectArgs` 呼び出しを `evaluateMprotectFamilyArgs` に更新
  - [ ] 戻り値をスライスとして受け取り、ループで処理
  - [ ] `fmt.Sprintf` のフォーマット文字列を `evalResult.SyscallName` を使う形に統一（`"mprotect at ..."` → `"%s at ..."` + `evalResult.SyscallName`）
  - [ ] `EvalProtExecRisk` の呼び出しを1エントリずつに変更

### 2.3 `mprotect_risk.go` → `prot_exec_risk.go` 改名・更新

- [ ] `mprotect_risk.go` を `prot_exec_risk.go` にリネームする
- [ ] `mprotect_risk_test.go` を `prot_exec_risk_test.go` にリネームする
- [ ] `EvalProtExecRisk` のフィルター条件を拡張
  - [ ] `r.SyscallName != "mprotect"` → `r.SyscallName != "mprotect" && r.SyscallName != "pkey_mprotect"`
- [ ] 関数コメントを更新（`pkey_mprotect` も評価対象であることを明記）

---

## Phase 3: 動作確認・整合性検証

### 3.1 テスト実行

- [ ] `go test -tags test -v ./internal/runner/security/elfanalyzer/...` を実行し、Phase 1 で追加した全テストが GREEN になること
- [ ] `go test -tags test -v ./internal/fileanalysis/...` を実行し、スキーマバージョンテストが GREEN になること
- [ ] `make test` を実行し、リポジトリ全体のテストが全て GREEN になること

### 3.2 `convertSyscallResult` 経路の確認

`standard_analyzer.go:354` の `convertSyscallResult` は `EvalProtExecRisk(result.ArgEvalResults)` を
直接呼ぶ。`pkey_mprotect` エントリが `ArgEvalResults` に追加されれば（Phase 2.2 の実装完了後）、
`EvalProtExecRisk` の拡張（Phase 2.3）と組み合わせて自動的に `IsHighRisk` へ反映される。

この経路が正しく動作することを以下のテストで確認すること：

- [ ] `TestSyscallAnalyzer_MprotectAndPkeyMprotect` の各ケースにおいて、`result.Summary.IsHighRisk` が
  `EvalProtExecRisk(result.ArgEvalResults) || result.HasUnknownSyscalls` と一致することを検証する

### 3.2 コード品質

- [ ] `make lint` を実行し、lint エラーがないこと
- [ ] `make fmt` を実行し、フォーマット差分がないこと

---

## 実装上の注意事項

### バイト列の組み立て

`pkey_mprotect` の syscall 番号はタスク 0078 の `mprotect`（番号 10）と比べて大きいため、
`mov eax, imm32` 形式（5バイト：`0xb8 0x49 0x01 0x00 0x00`）が必要になる。
タスク 0078 のテストが使用する `mov eax, imm8` 形式とは異なる点に注意する。

arm64 の場合も同様に、`pkey_mprotect` の syscall 番号 288 は
`MOVZ X8, #288`（`0x08 0x24 0x84 0xd2`）で表現する。

### `EvalProtExecRisk` の呼び出し方

`analyzeSyscallsInCode` からの呼び出しでは、エントリを1件ずつ渡す
（`EvalProtExecRisk([]common.SyscallArgEvalResult{evalResult})`）。
これにより `mprotect` のエントリが `pkey_mprotect` の警告生成に影響することを防ぐ。

### スキーマバージョン変更の波及

`CurrentSchemaVersion` を 7 に変更すると、バージョン 6 の記録は
次回の `record` コマンド実行時に自動的に再解析・上書きされる（`--force` 不要）。
開発環境のキャッシュレコードは無効化されるが、これは意図した動作である。
