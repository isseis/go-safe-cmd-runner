# 実装計画: filevalidator テストのインメモリ ELF 生成への移行

- [x] 1. `internal/filevalidator/validator_test.go` の修正 (AC-1, AC-2)
  - [x] `elfanalyzertesting` パッケージのインポートを追加する
  - [x] `TestRecord_LibcCache_Error_CausesRecordFailure` を修正する (FR-1)
    - `const elfPath = "/usr/bin/ls"` および `os.Stat` + `t.Skipf` ブロックを削除
    - `safeTempDir(t)` で一時ディレクトリを作成し、必要に応じて `filepath.EvalSymlinks` で解決したパス配下の `test.elf` を `elfanalyzertesting.CreateDynamicELFFile(t, ...)` で生成する形に置き換える
    - `analyzeSyscalls(record, elfPath)` の `elfPath` を生成した ELF のパスに変更する
  - [x] `TestRecord_Force_ELFToNonELF_ClearsSyscallAnalysis` を修正する (FR-2)
    - `const elfPath = "/usr/bin/ls"` および `os.Stat` + `t.Skipf` ブロックを削除
    - `elfBytes, err := os.ReadFile(elfPath)` + `os.WriteFile(targetFile, elfBytes, ...)` を `elfanalyzertesting.CreateDynamicELFFile(t, targetFile)` に置き換える
  - [x] `TestRecord_Force_SyscallsToNone_ClearsSyscallAnalysis` を修正する (FR-3)
    - `const elfPath = "/usr/bin/ls"` および `os.Stat` + `t.Skipf` ブロックを削除
    - FR-2 と同様に `elfBytes, err := os.ReadFile(elfPath)` + `os.WriteFile(targetFile, elfBytes, ...)` を `elfanalyzertesting.CreateDynamicELFFile(t, targetFile)` に置き換える

- [x] 2. テストの実行確認 (AC-3)
  - [x] `make test` を実行してすべてのテストがパスすることを確認する
  - [x] `make lint` を実行してエラーがないことを確認する
