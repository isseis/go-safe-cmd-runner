# 実装計画: TestAnalyze_StaticELF のインメモリ ELF 生成への移行

- [ ] 1. `TestAnalyze_StaticELF` をインメモリ ELF 生成方式に書き換え (AC-1, AC-2, AC-3)
  - [ ] `internal/dynlibanalysis/analyzer_test.go` の `TestAnalyze_StaticELF` 関数を変更する
    - `staticELF := "../runner/security/elfanalyzer/testdata/static.elf"` を削除 (AC-1)
    - `os.Stat(staticELF)` によるファイル存在確認を削除 (AC-1)
    - `t.Skipf(...)` によるスキップ処理を削除 (AC-1)
    - `tmpDir := t.TempDir()` を追加 (AC-2)
    - `staticELF := buildTestELFWithDeps(t, tmpDir, "static.elf", nil, "")` を追加 (AC-2)
  - `os` パッケージは `TestAnalyze_NonELF` で引き続き使用されるためインポート変更は不要 (AC-3)

- [ ] 2. テストの実行確認 (AC-3)
  - [ ] `make test` を実行してすべてのテストがパスすることを確認する
  - [ ] `make lint` を実行してエラーがないことを確認する
