# シェバンインタプリタ解析結果のセッション内キャッシュ 実装計画書

## 1. 実装概要

### 1.1 目的

[01_requirements.md](01_requirements.md) の FR-3.1 〜 FR-3.4 を満たし、
`Validator` インスタンス内でシェバンチェーン上のインタプリタバイナリに対する
`analyzeRecordTarget` の戻り値を再利用する仕組みを実装する。
詳細設計は [02_architecture.md](02_architecture.md) を参照。

### 1.2 実装原則

- 既存の `libCacheKey` 構造体と、`validatorWithTempHashDir` /
      `libraryTestBinaryAnalyzer` のテストパターンを再利用し、二重実装を避ける
- プロダクションコードに `-tags test` 専用分岐やテスト用フラグを混入させない
- `*fileanalysis.Record` の最終生成内容はキャッシュ導入前後で同一
- コード中のコメント・識別子・テスト名はすべて英語（CLAUDE.md 既定）

---

## 2. 進捗状況

- [x] Step 1: `Validator` 構造体への `processedInterpreterAnalysis` フィールド追加
- [x] Step 2: `loadOrAnalyzeShebangTarget` ヘルパの実装
- [x] Step 3: `populateShebangData` のキャッシュ経由化
- [ ] Step 4: テスト追加（AC-1, AC-2, AC-3, AC-4 を網羅）
- [ ] Step 5: 既存テストの回帰確認と `make fmt` / `make test` / `make lint`
- [ ] Step 6: AC 対応の最終検証と巻き戻り点検

---

## 3. 各 Step の詳細

### Step 1: `Validator` 構造体への `processedInterpreterAnalysis` フィールド追加

対象ファイル:
- `internal/filevalidator/validator.go`

作業内容:
- [ ] `Validator` 構造体に `processedInterpreterAnalysis map[libCacheKey]*fileanalysis.Record` を追加
- [ ] フィールドコメントは英語で 1 行（用途と寿命）
- [ ] 初期化は lazy（既存の `processedLibAnalysis` と同じく nil チェック後に make）

成功条件:
- ビルドが通る（`go build ./...`）
- 既存テストが影響を受けない（フィールドを参照するコードを未追加のため）

参照: [02_architecture.md § 4.2](02_architecture.md#42-validator-構造体への追加)

### Step 2: `loadOrAnalyzeShebangTarget` ヘルパの実装

対象ファイル:
- `internal/filevalidator/validator.go`

作業内容:
- [ ] 関数シグネチャ: `func (v *Validator) loadOrAnalyzeShebangTarget(filePath, contentHash string) (*fileanalysis.Record, error)`
- [ ] 既存の `loadOrAnalyzeLibrary` とそろえた構造（lazy-init → cache lookup → miss 時に `analyzeRecordTarget` 実行 → cache 格納）
- [ ] 失敗結果はキャッシュに格納しない（fail-closed）
- [ ] 関数コメント（英語、1〜2 行）

成功条件:
- ビルドが通る
- 単体では呼び出し元がないため、Step 3 と組み合わせて検証

参照: [02_architecture.md § 4.4](02_architecture.md#44-影響範囲)

### Step 3: `populateShebangData` のキャッシュ経由化

対象ファイル:
- `internal/filevalidator/validator.go`

作業内容:
- [ ] `populateShebangData` 内のシェバンチェーンエントリ処理ループ内、
      `analyzeRecordTarget(entry.Path, entryHash)` の呼び出しを
      `loadOrAnalyzeShebangTarget(entry.Path, entryHash)` に置き換え
- [ ] `aggregate.addRecord` および `depCollector.addEntries` への登録は従来どおり、
      キャッシュヒット時にも同じ `*fileanalysis.Record` を渡して呼ぶ
- [ ] `depCollector.addEntry`（インタプリタ自身の `LibEntry` 登録）は順序を変えない

成功条件:
- 既存の shebang 関連テスト（`validator_shebang_test.go`、`e2e_shebang_test.go`）がすべて pass

参照: [02_architecture.md § 3.2 § 8.2](02_architecture.md#32-変更後フロー)

### Step 4: テスト追加

対象ファイル:
- `internal/filevalidator/validator_shebang_cache_test.go`（新規、`//go:build test` タグ付き）

#### Step 4.1: 共有インタプリタの再解析抑制（AC-1）

- [ ] テスト名: `TestSaveRecord_ShebangInterpreterCacheReuse`
- [ ] 共有インタプリタは `/bin/sh`（既存テストと同じく Linux 標準、CI 互換）
- [ ] パス別呼び出し回数を計上するスパイ型を新規追加（`libraryTestBinaryAnalyzer` の
      `calls int` パターンを参考に、`callsByPath map[string]int` を持つ新規型を定義。
      既存型は変更しない）
- [ ] 同一 `Validator` インスタンスで 2 つの異なるスクリプト（同じ shebang を持つ）を `SaveRecord`
- [ ] 検証: インタプリタ解決済みパス（例: `/bin/sh` の symlink 先）に対する
      `AnalyzeNetworkSymbols` 呼び出し回数が 1

#### Step 4.2: 出力同一性 + `depCollector` 連携（AC-2 + AC-5 を統合）

- [ ] テスト名: `TestSaveRecord_ShebangInterpreterCacheOutputEquivalence`
- [ ] 同じインタプリタ（`/bin/sh`）を使う 2 つの異なるスクリプト（内容違い）を
      同一 `Validator` インスタンスで `SaveRecord`。1 つ目が cache miss、
      2 つ目が cache hit となる
- [ ] 検証 1（AC-2）: 両スクリプトの `LoadRecord` 結果について、
      `ShebangChain` / `DynLibDeps` / `SymbolAnalysis` / `SyscallAnalysis` /
      `AnalysisWarnings` をそれぞれ完全一致で比較する
- [ ] 検証 2（AC-2）: `ShebangInterpreter` も完全一致で比較し、シェバン解決情報が
      cache hit 経路で欠落しないことを確認する
- [ ] 検証 3（AC-5）: cache hit 経路（2 つ目のスクリプト）の `record.DynLibDeps` に
      インタプリタ自身（`ShebangChain[i].Path`）が `LibEntry` として含まれていること

#### Step 4.3: 同一パス・ハッシュ変化時の独立解析（AC-3）

- [ ] テスト名: `TestSaveRecord_ShebangInterpreterCacheHashChangeReanalyzes`
- [ ] `validator_shebang_cache_test.go` 内に、Linux 上で小さなテスト用 ELF
      インタプリタを生成する package-local helper を追加する
- [ ] 同一パスのインタプリタ実体を差し替えてハッシュを変え、同じパスを参照する
      2 本のスクリプトに対して `SaveRecord` を順に実行する
- [ ] 検証 1（AC-3）: `Validator.processedInterpreterAnalysis` に同一パスで
      `{path, hashA}` と `{path, hashB}` の 2 キーが格納されること
- [ ] 検証 2（AC-3）: パス別スパイまたは content-hash 別観測により、
      ハッシュ変化後のインタプリタに対して `analyzeRecordTarget` 経路が再実行されること

#### Step 4.4: env 形式チェーンのキャッシュ動作（AC-4）

- [ ] テスト名: `TestSaveRecord_ShebangInterpreterCacheEnvForm`
- [ ] `#!/usr/bin/env sh` を持つ 2 スクリプトに対して連続 `SaveRecord`
- [ ] 検証: env バイナリと解決済みコマンド双方に対する `AnalyzeNetworkSymbols`
      呼び出し回数がそれぞれ 1（= 2 要素どちらもキャッシュヒットしている）

成功条件:
- 上記 4 テストが pass
- 既存の `validator_shebang_test.go` 全テストも pass

### Step 5: 既存テストの回帰確認と整形

作業内容:
- [ ] `make fmt`
- [ ] `go test -tags test -v ./...` がすべて pass
- [ ] `make lint` がすべて pass
- [ ] `internal/runner/e2e_shebang_test.go` の統合テスト群が pass

成功条件:
- 上記 3 コマンドがエラーなく完了

### Step 6: AC 対応の最終検証

作業内容:
- [ ] § 4 の AC トレーサビリティ表に従い、各 AC の対応テストが実在することを確認
- [ ] FR-3.4.1（観測手段）がプロダクションコードに分岐を残していないことを確認
- [ ] [02_architecture.md](02_architecture.md) の前提どおり、verify 側経路に変更がないことを
      手で diff して確認

---

## 4. AC 対応トレーサビリティ

| AC | 主担当 Step | 検証テスト |
|----|-------------|------------|
| AC-1 | Step 4.1 | `TestSaveRecord_ShebangInterpreterCacheReuse` でカウンタが 1 になること |
| AC-2 | Step 4.2 | `TestSaveRecord_ShebangInterpreterCacheOutputEquivalence` で `ShebangChain` / `DynLibDeps` / `SymbolAnalysis` / `SyscallAnalysis` / `AnalysisWarnings` / `ShebangInterpreter` が完全一致すること |
| AC-3 | Step 4.3 | `TestSaveRecord_ShebangInterpreterCacheHashChangeReanalyzes` でハッシュ変化後も独立に解析されること |
| AC-4 | Step 4.4 | `TestSaveRecord_ShebangInterpreterCacheEnvForm` で env / sh 両方が 1 回ずつであること |
| AC-5 | Step 4.2 | 同テスト内で `record.DynLibDeps` にインタプリタが含まれていることを検証 |
| AC-6 | Step 5 | `make fmt` / `go test -tags test -v ./...` / `make lint` の全合格 |
| AC-7 | Step 5 | `validator_library_analysis_test.go` 全テストが回帰なく pass |

---

## 5. テスト戦略

### 5.1 ユニットテスト

新規テストは Step 4 の 4 件のみ。AC-2 と AC-5 は単一テストで両立できるためテスト数を最小化する。

カバレッジ観点:
- キャッシュヒット経路: AC-1, AC-2, AC-4
- キャッシュミス経路: AC-1（1 回目）, AC-3
- env 形式の 2 段チェーン: AC-4
- depCollector 連携の維持: AC-5（AC-2 と統合）

### 5.2 重複回避

- 既存の `TestSaveRecord_ShebangDirect` / `TestSaveRecord_ShebangEnv`
  （`validator_shebang_test.go`）は単発記録のシェバンチェーン構造を検証している。
  本タスクで新設する 4 テストは「複数回記録時の挙動」に焦点を絞り、単発記録の
  正常系は既存テストに任せる
- ライブラリ単位キャッシュの正しさは `validator_library_analysis_test.go` の
  既存テストで担保済みなので、本タスクでは再検証しない（AC-7 はそれら既存テストの
  回帰なし pass によって満たす）

### 5.3 テスト用ヘルパの再利用

| 用途 | 既存ヘルパ |
|---|---|
| 一時ディレクトリ生成 | `safeTempDir` / `commontesting.SafeTempDir` |
| 実行可能ファイル書き込み | `commontesting.WriteExecutableFile` |
| バイナリ解析カウント | `libraryTestBinaryAnalyzer.calls` のパス別拡張 |
| ELF 形式テスト fixture | `TestSaveRecord_ShebangELF` で使われている ELF ヘッダパターン |
| `Validator` 生成 | `validatorWithTempHashDir` |

---

## 6. リスク管理

| リスク | 影響 | 緩和策 |
|---|---|---|
| キャッシュヒット時に `aggregate` への登録漏れが発生し出力が変わる | AC-2 違反 | Step 4.2 で record 同一性を厳格に比較 |
| `loadOrAnalyzeShebangTarget` 内のエラーキャッシュにより以後の record が一貫してエラーになる | 本番障害 | Step 2 で「失敗結果はキャッシュに格納しない」を厳守し、Step 4 で間接的に確認 |
| カウントスパイの実装誤りにより AC-1 のテストが偽陽性 | キャッシュ未動作の見逃し | パスごとの呼び出し回数を `map` で記録し、複数アサートで挙動を担保 |
| 既存ライブラリキャッシュの挙動を意図せず変える | AC-7 違反 | Step 5 で既存ライブラリ解析テストを完全 pass させる |

---

## 7. 実装チェックリスト

- [x] Step 1: フィールド追加完了
- [x] Step 2: ヘルパ実装完了
- [x] Step 3: `populateShebangData` 置換完了
- [ ] Step 4.1: AC-1 テスト追加完了
- [ ] Step 4.2: AC-2 + AC-5 テスト追加完了
- [ ] Step 4.3: AC-3 テスト追加完了
- [ ] Step 4.4: AC-4 テスト追加完了
- [ ] Step 5: `make fmt` / `go test -tags test ./...` / `make lint` 全合格
- [ ] Step 6: AC 対応最終検証完了

---

## 8. 成功基準

- 機能面: 同一インタプリタを共有する複数スクリプトの連続 `SaveRecord` で、
  インタプリタ解析（dynlib / シンボル / syscall）が 1 回に抑えられる
- 品質面: `make fmt` / `go test -tags test -v ./...` / `make lint` 全合格
- 互換性: 既存の `record` コマンド出力（`fileanalysis.Record` 内容）が同一
- セキュリティ: キャッシュキーの (パス + ハッシュ) 一致判定により、
  異なるバイナリ間で結果が混在しない
- ドキュメント: AC トレーサビリティ表（§ 4）が完成している

---

## 9. 次のステップ

実装完了後の発展タスク（本タスクのスコープ外）:

- セッション横断永続キャッシュの設計（[02_architecture.md § 11.1](02_architecture.md#111-セッション横断永続キャッシュへの発展余地)）
- ライブラリ単位キャッシュとの統合検討（同 § 11.2）
