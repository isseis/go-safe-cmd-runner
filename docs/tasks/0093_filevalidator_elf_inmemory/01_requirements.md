# 0093: filevalidator テストのインメモリ ELF 生成への移行

## 1. 概要

### 1.1 背景

`internal/filevalidator/validator_test.go` には、実システムの ELF バイナリ（`/usr/bin/ls`）を参照する 3 つのテストが存在する。

これらのテストは、テスト実行時に `/usr/bin/ls` が利用可能であることを条件としており、ファイルが存在しない場合は `t.Skipf` によってスキップされる。

```go
const elfPath = "/usr/bin/ls"
if _, err := os.Stat(elfPath); err != nil {
    t.Skipf("skipping: %s not available: %v", elfPath, err)
}
```

この設計には以下の問題がある。

1. **環境依存**: Alpine Linux コンテナや GNU coreutils を持たない Docker イメージなど、`/usr/bin/ls` が存在しない環境でテストがスキップされる。
2. **テストカバレッジの欠落**: `analyzeSyscalls` のエラー伝播テスト・`SyscallAnalysis` の nil 遷移テストが実行されない場合がある。
3. **外部ファイルへの依存**: テストの正確性が OS 環境のファイル配置に依存している。
4. **テストデータの不確定性**: システムの `/usr/bin/ls` の ELF フォーマットはディストリビューションによって異なる可能性がある。

同パッケージの関連テスト（`internal/runner/security/elfanalyzer/testing/helpers.go`）には、テスト用のインメモリ ELF を生成する `CreateDynamicELFFile(t, path)` ヘルパーがすでに整備されている。これを活用することで、外部ファイルへの依存を解消できる。

### 1.2 目的

`internal/filevalidator/validator_test.go` 内の `/usr/bin/ls` に依存する 3 つのテストを、`elfanalyzertesting.CreateDynamicELFFile` を用いたインメモリ ELF 生成方式に書き換え、任意の環境で安定してテストが実行できるようにする。

### 1.3 対象テスト

| テスト名 | 説明 |
|---|---|
| `TestRecord_LibcCache_Error_CausesRecordFailure` | libc キャッシュエラーが `analyzeSyscalls` を失敗させることを検証 |
| `TestRecord_Force_ELFToNonELF_ClearsSyscallAnalysis` | ELF → 非 ELF の force 再記録で `SyscallAnalysis` が nil になることを検証 |
| `TestRecord_Force_SyscallsToNone_ClearsSyscallAnalysis` | ELF の force 再記録で syscall 数が 0 になると `SyscallAnalysis` が nil になることを検証 |

### 1.4 スコープ外

- 上記 3 テスト以外の変更
- `elfanalyzertesting.CreateDynamicELFFile` 自体の変更
- `internal/runner/security/elfanalyzer/testdata/*.elf` ファイルの変更

---

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| インメモリ ELF | Go の `debug/elf` パッケージで解析可能な最小限の ELF バイナリをテスト実行時にバイト列として構築したもの。外部ファイルや GCC に依存しない |
| `CreateDynamicELFFile` | `internal/runner/security/elfanalyzer/testing/helpers.go` で定義されるテストヘルパー。`.dynsym` セクションを持つ動的 ELF64 LE ファイルをディスクに生成する |
| `SyscallAnalysis` | `fileanalysis.Record` のフィールド。ELF バイナリから検出されたシステムコール情報を格納する。非 ELF または検出ゼロの場合は `nil` |

---

## 3. 機能要件

### FR-1: `TestRecord_LibcCache_Error_CausesRecordFailure` の外部ファイル依存の解消

`TestRecord_LibcCache_Error_CausesRecordFailure` は、外部 ELF ファイル依存なしに動作すること。

- `/usr/bin/ls` の存在確認および `t.Skipf` を除去する
- `elfanalyzertesting.CreateDynamicELFFile(t, elfPath)` でインメモリ ELF ファイルを生成し、それを `analyzeSyscalls` に渡す
- 生成した ELF は `debug/elf.Open` で解析可能であること（`CreateDynamicELFFile` が保証）
- libc stub はエラーを返し、`analyzeSyscalls` がそのエラーを伝播することを検証する動作を維持する

**変更対象**: `internal/filevalidator/validator_test.go`

### FR-2: `TestRecord_Force_ELFToNonELF_ClearsSyscallAnalysis` の外部ファイル依存の解消

`TestRecord_Force_ELFToNonELF_ClearsSyscallAnalysis` は、外部 ELF ファイル依存なしに動作すること。

- `/usr/bin/ls` の存在確認および `t.Skipf` を除去する
- `os.ReadFile(elfPath)` + `os.WriteFile(targetFile, elfBytes, ...)` のパターンを `elfanalyzertesting.CreateDynamicELFFile(t, targetFile)` の直接呼び出しに置き換える
- 以降の「非 ELF への上書き → force 再記録 → `SyscallAnalysis` が nil」という検証ロジックは変更しない

**変更対象**: `internal/filevalidator/validator_test.go`

### FR-3: `TestRecord_Force_SyscallsToNone_ClearsSyscallAnalysis` の外部ファイル依存の解消

`TestRecord_Force_SyscallsToNone_ClearsSyscallAnalysis` は、外部 ELF ファイル依存なしに動作すること。

- `/usr/bin/ls` の存在確認および `t.Skipf` を除去する
- FR-2 と同様に `os.ReadFile(elfPath)` + `os.WriteFile(targetFile, elfBytes, ...)` を `elfanalyzertesting.CreateDynamicELFFile(t, targetFile)` に置き換える
- 「force 再記録 → syscall 数 0 → `SyscallAnalysis` が nil」という検証ロジックは変更しない

**変更対象**: `internal/filevalidator/validator_test.go`

---

## 4. 非機能要件

### 4.1 テスト実行環境

変更後、3 つのテストはすべて `/usr/bin/ls` の存在に依存せず実行できること。

### 4.2 テストのスキップ廃止

変更前は `/usr/bin/ls` 不在時にテストをスキップしていた。変更後はスキップ処理を含まず、常にテストが実行されること。

### 4.3 既存の検証ロジックの維持

各テストの「何を検証しているか」（`analyzeSyscalls` のエラー伝播、`SyscallAnalysis` の nil 遷移）はそのまま維持すること。ELF ファイルの調達方法のみを置き換える。

---

## 5. 受け入れ基準

### AC-1: `/usr/bin/ls` 参照の除去

- [ ] `TestRecord_LibcCache_Error_CausesRecordFailure` 内に `elfPath = "/usr/bin/ls"` の参照が存在しないこと
- [ ] `TestRecord_Force_ELFToNonELF_ClearsSyscallAnalysis` 内に `elfPath = "/usr/bin/ls"` の参照が存在しないこと
- [ ] `TestRecord_Force_SyscallsToNone_ClearsSyscallAnalysis` 内に `elfPath = "/usr/bin/ls"` の参照が存在しないこと
- [ ] ファイル存在確認に基づくスキップ処理（`t.Skip` / `t.Skipf`）がこれら 3 テストから除去されていること

### AC-2: インメモリ ELF による検証

- [ ] 3 つのテストがそれぞれ `elfanalyzertesting.CreateDynamicELFFile` を呼び出してインメモリ ELF ファイルを生成すること
- [ ] `TestRecord_LibcCache_Error_CausesRecordFailure` が libc キャッシュエラーの伝播を検証していること
- [ ] `TestRecord_Force_ELFToNonELF_ClearsSyscallAnalysis` が ELF → 非 ELF 遷移後に `SyscallAnalysis` が nil になることを検証していること
- [ ] `TestRecord_Force_SyscallsToNone_ClearsSyscallAnalysis` が syscall 数 0 の再記録後に `SyscallAnalysis` が nil になることを検証していること

### AC-3: テストの安定実行

- [ ] `go test -tags test -v ./internal/filevalidator/...` を `/usr/bin/ls` なしの環境でも実行したとき 3 テストが PASS すること
- [ ] `make test` がすべてパスすること
- [ ] `make lint` がエラーなく完了すること
