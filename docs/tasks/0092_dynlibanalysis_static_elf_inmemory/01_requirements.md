# 0092: TestAnalyze_StaticELF のインメモリ ELF 生成への移行

## 1. 概要

### 1.1 背景

`internal/dynlibanalysis/analyzer_test.go` の `TestAnalyze_StaticELF` は、ELF バイナリに動的依存ライブラリ（DT_NEEDED）が存在しない場合に `Analyze()` が nil を返すことを検証するテストである。

現在、このテストは `internal/runner/security/elfanalyzer/testdata/static.elf` を外部ファイルとして参照している。このファイルは以下の性質を持つ。

- `internal/runner/security/elfanalyzer/testdata/.gitignore` により Git 管理外（`*.elf` で除外）
- `make elfanalyzer-testdata` により `gcc -x c -static` で生成される
- `dynlibanalysis` パッケージとは別パッケージ（`elfanalyzer`）のテストデータディレクトリに存在する

この設計には以下の問題がある。

1. **クロスパッケージのテストデータ依存**：`dynlibanalysis` テストが `elfanalyzer` パッケージのテストデータ生成に依存している。パッケージ間の責務境界を侵害する。
2. **外部ツール依存**：GCC がインストールされていない環境や、`make elfanalyzer-testdata` を実行していない環境でテストが実行された場合、ファイルが存在せずテストがスキップされる（テストカバレッジが欠落する）。
3. **パスの脆弱性**：外部ファイルへの相対パス参照は、リポジトリ構造の変更やビルドアーティファクトの配置によって誤動作を起こしやすい（本タスクの起因となった障害はこれによって発生した）。

同じファイル内の他のテスト（`TestAnalyze_TransitiveDeps`、`TestAnalyze_CircularDeps` 等）は `buildTestELFWithDeps` ヘルパーを使ってインメモリで ELF を構築しており、外部ファイルに依存しない。`TestAnalyze_StaticELF` も同様の手法に統一する。

### 1.2 目的

`TestAnalyze_StaticELF` を、外部ファイルに依存しないインメモリ ELF 生成方式に書き換え、GCC や `make` ターゲットへの依存なしに任意の環境で安定してテストが実行できるようにする。

### 1.3 スコープ外

- `elfanalyzer/testdata/static.elf` 自体の削除・変更（`elfanalyzer` パッケージのテストは引き続き使用する）
- `buildTestELFWithDeps` の動作変更
- `dynlibanalysis` パッケージの他テストへの変更

---

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| 静的 ELF | PT_DYNAMIC セグメントを持たない、または DT_NEEDED エントリを持たない ELF バイナリ。`Analyze()` は nil を返す |
| `buildTestELFWithDeps` | `analyzer_test.go` に定義済みのインメモリ ELF 生成ヘルパー。動的依存ライブラリを指定して ELF を構築する |
| DT_NEEDED | ELF の DYNAMIC セクションに記録される動的リンク依存ライブラリのエントリ |

---

## 3. 機能要件

### FR-1: `buildTestELFWithDeps` を利用した静的 ELF の生成

`buildTestELFWithDeps` に `sonames=nil`、`runpath=""` を渡すと、DT_NEEDED エントリを持たない ELF が生成される。`elf.DynString(elf.DT_NEEDED)` は空スライスを返し、`Analyze()` は nil を返す。この挙動を利用して `TestAnalyze_StaticELF` を書き換える。

**変更対象**：`internal/dynlibanalysis/analyzer_test.go`

**変更内容**：

1. `TestAnalyze_StaticELF` 関数本体を以下の構造に書き換える：
   - `t.TempDir()` で一時ディレクトリを用意する
   - `buildTestELFWithDeps(t, tmpDir, "no_deps.elf", nil, "")` で DT_NEEDED なし ELF を生成する
   - `a.Analyze(path)` を呼び出す
   - 結果が nil であることを `assert.Nil` で検証する
2. 外部ファイルパス（`../runner/security/elfanalyzer/testdata/static.elf`）への参照を削除する
3. `os.Stat` によるファイル存在確認とスキップ処理を削除する

### FR-2: `os` パッケージのインポート整理

`os.Stat` の削除により `os` パッケージが不要になる場合は、インポートから除去する。ただし、他のテスト関数（例：`TestAnalyze_NonELF` の `os.WriteFile`）で使用されている場合は除去しない。

---

## 4. 非機能要件

### 4.1 テスト実行環境

変更後、`TestAnalyze_StaticELF` は GCC、`make`、外部バイナリファイルへの依存なしに実行できること。

### 4.2 テストのスキップ廃止

変更前は `os.Stat` 失敗時にテストをスキップしていた。変更後はスキップ処理を持たず、常にテストが実行されること。

---

## 5. 受け入れ基準

### AC-1: 外部ファイル参照の除去

- [ ] `TestAnalyze_StaticELF` 内に `elfanalyzer/testdata` へのパス参照が存在しないこと
- [ ] `os.Stat` によるファイル存在確認コードが除去されていること
- [ ] `t.Skip` / `t.Skipf` 呼び出しが除去されていること

### AC-2: インメモリ ELF による検証

- [ ] `buildTestELFWithDeps` を `sonames=nil`、`runpath=""` で呼び出して ELF を生成していること
- [ ] `a.Analyze(path)` の返り値が nil であることを `assert.Nil` で検証していること
- [ ] `require.NoError` でエラーがないことを検証していること

### AC-3: テストの安定実行

- [ ] `go test -tags test -v ./internal/dynlibanalysis/...` を `make elfanalyzer-testdata` なしで実行したとき `TestAnalyze_StaticELF` が PASS すること
- [ ] `make test` がすべてパスすること
- [ ] `make lint` がエラーなく完了すること

---

## 6. 実装ノート

`buildTestELFWithDeps(t, tmpDir, "no_deps.elf", nil, "")` が生成する ELF は、DT_STRTAB・DT_STRSZ・DT_NULL のみを含む `.dynamic` セクションを持つ。これは `gcc -static` で生成される「PT_DYNAMIC セグメントを持たない」ELF とは構造が異なるが、`Analyze()` の戻り値（DT_NEEDED が空のとき nil）の検証という観点では同等である。

テストの目的が「DT_NEEDED なし ELF に対して Analyze() が nil を返すこと」の確認であるため、本変更は機能的に等価な置き換えである。
