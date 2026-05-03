# 動的ライブラリ解析結果ストア導入 実装計画書

## 0. 実施順序付きの具体差分案

本章は、最終的に最も正確で分かりやすい用語体系へ到達するための
差分計画をファイル単位・見出し単位・シンボル単位で定義する。

### 0.1 フェーズ順序

1. 文書全面改定（要件・設計・詳細仕様・計画）
2. パッケージ名と公開シンボルの改名
3. record 側内部フィールド・メソッド改名
4. runner 側内部フィールド・コンストラクタ改名
5. テスト名・エラーメッセージ・ログ語彙改名
6. 互換 alias 削除と最終クリーンアップ

### 0.2 ファイル単位差分

| 順序 | 対象ファイル | 変更概要 |
|---|---|---|
| 1 | docs/tasks/0124_dynlib_library_analysis_cache/01_requirements.md | キャッシュ中心記述を解析結果中心へ変更 |
| 1 | docs/tasks/0124_dynlib_library_analysis_cache/02_architecture.md | runner フローを解析結果読取主語へ変更 |
| 1 | docs/tasks/0124_dynlib_library_analysis_cache/03_detailed_specification.md | API 名称・責務・用語規則を最終形へ定義 |
| 1 | docs/tasks/0124_dynlib_library_analysis_cache/04_implementation_plan.md | リネーム実装手順を本計画へ統合 |
| 2 | internal/dynlibcache/* | internal/dynlibanalysisstore/* へ移動・改名 |
| 3 | internal/filevalidator/validator.go | libAnalysisCacheManager 系シンボルを dynamicLibAnalysisStore 系へ改名 |
| 4 | internal/runner/base/security/network_analyzer.go | libCache 系シンボルを libAnalysisStore 系へ改名 |
| 5 | *_test.go（関連一式） | cache hit/miss 語彙を analysis reuse/not found に改名 |
| 6 | cmd/record/main.go, cmd/runner/main.go | 新パッケージと新コンストラクタへ完全移行 |

### 0.3 見出し単位差分（ドキュメント）

| 順序 | 文書 | 旧見出し | 新見出し |
|---|---|---|---|
| 1 | 01_requirements | ライブラリ解析結果の共通キャッシュ化 | 動的ライブラリ解析結果ストア導入 |
| 1 | 01_requirements | FR-3.2.2 runner による実行時キャッシュ参照 | FR-3.2.2 runner による実行時解析結果参照 |
| 1 | 02_architecture | キャッシュミス時の挙動 | 解析結果未取得時の挙動 |
| 1 | 03_detailed_specification | CacheManager の構造 | DynamicLibAnalysisStore の構造 |
| 1 | 03_detailed_specification | libCache の Get | libAnalysisStore の LoadAnalysis |

### 0.4 シンボル単位差分（コード）

| 順序 | 種別 | 旧 | 新 |
|---|---|---|---|
| 2 | package | internal/dynlibcache | internal/dynlibanalysisstore |
| 2 | type | CacheManager | DynamicLibAnalysisStoreImpl |
| 2 | interface | CacheManagerInterface | DynamicLibAnalysisStore |
| 2 | method | GetOrCreate | LoadOrAnalyzeAndStore |
| 2 | method | Get | LoadAnalysis |
| 2 | error | ErrCacheMiss | ErrAnalysisNotFound |
| 3 | field | libAnalysisCacheManager | dynamicLibAnalysisStore |
| 3 | setter | SetLibraryAnalysisCacheManager | SetDynamicLibAnalysisStore |
| 4 | field | libCache | libAnalysisStore |
| 4 | constructor | NewNetworkAnalyzerWithLibCache | NewNetworkAnalyzerWithLibAnalysisStore |

---

## 1. 進捗状況

- [x] Step 1: 文書全面改定（本計画の 0 章反映）
- [x] Step 2: 解析結果ストアパッケージ新設と旧パッケージ互換層作成
- [x] Step 3: record 側シンボル改名
- [x] Step 4: runner 側シンボル改名
- [x] Step 5: cmd 配線の完全移行
- [x] Step 6: テスト名・ログ語彙の改名
- [ ] Step 7: 互換 alias 削除
- [ ] Step 8: fmt/test/lint 実行と最終整合確認

---

## 2. 各 Step の詳細

### Step 1: 文書全面改定

対象:

- docs/tasks/0124_dynlib_library_analysis_cache/01_requirements.md
- docs/tasks/0124_dynlib_library_analysis_cache/02_architecture.md
- docs/tasks/0124_dynlib_library_analysis_cache/03_detailed_specification.md
- docs/tasks/0124_dynlib_library_analysis_cache/04_implementation_plan.md

作業内容:

- [ ] 用語規則を明文化（キャッシュ語は record の再解析回避に限定）
- [ ] runner 文脈のキャッシュ語を解析結果語へ置換
- [ ] 見出し・AC・表の名称を最終語彙へ更新

### Step 2: 解析結果ストアパッケージ新設

対象:

- internal/dynlibanalysisstore/schema.go
- internal/dynlibanalysisstore/store.go
- internal/dynlibanalysisstore/errors.go
- internal/dynlibanalysisstore/interfaces.go

作業内容:

- [x] internal/dynlibcache を internal/dynlibanalysisstore へ移設
- [x] DynamicLibAnalysisStore インタフェースを定義
- [x] LoadOrAnalyzeAndStore / LoadAnalysis の API を実装
- [x] ErrAnalysisNotFound を定義
- [-] 旧パッケージの互換 alias を一時実装

### Step 3: record 側シンボル改名

対象:

- internal/filevalidator/validator.go
- internal/filevalidator/validator_library_analysis_test.go
- cmd/record/main.go

作業内容:

- [x] libAnalysisCacheManager を dynamicLibAnalysisStore に改名
- [x] SetLibraryAnalysisCacheManager を SetDynamicLibAnalysisStore に改名
- [x] analyzeLibraries の呼び出しを LoadOrAnalyzeAndStore へ変更
- [x] record 側の warning/error 伝播テストを新名称へ更新

### Step 4: runner 側シンボル改名

対象:

- internal/runner/base/security/network_analyzer.go
- internal/runner/base/security/network_analyzer_test.go
- cmd/runner/main.go

作業内容:

- [x] libCache を libAnalysisStore に改名
- [x] NewNetworkAnalyzerWithLibCache を NewNetworkAnalyzerWithLibAnalysisStore に改名
- [x] Get 呼び出しを LoadAnalysis に差し替え
- [x] ErrCacheMiss 判定を ErrAnalysisNotFound 判定へ更新

### Step 5: cmd 配線の完全移行

対象:

- cmd/record/main.go
- cmd/runner/main.go

作業内容:

- [x] dynlibanalysisstore.NewDynamicLibAnalysisStore を利用
- [x] 旧 dynlibcache 参照を除去
- [x] 依存注入の型名を新シンボルへ統一

### Step 6: テスト名・ログ語彙改名

対象:

- 関連 test 一式
- ログ出力箇所一式

作業内容:

- [ ] cache hit/miss 表現を analysis reuse/not found へ置換
- [ ] テスト名を新 API 名称に合わせて改名
- [ ] 失敗時メッセージを解析結果未取得語彙へ統一

### Step 7: 互換 alias 削除

対象:

- internal/dynlibcache（暫定互換層）
- 旧シンボル参照箇所

作業内容:

- [ ] 旧 package/type/method の alias を削除
- [ ] 全参照が新シンボルに移行済みであることを確認
- [ ] ドキュメントの移行中注記を削除

### Step 8: 品質確認と最終整合レビュー

- [ ] make fmt
- [ ] go test -tags test -v ./...
- [ ] make lint
- [ ] 4 文書間で用語・見出し・AC・テスト名の整合を再レビュー
- [ ] 実施順序と依存関係が矛盾しないことを再確認

---

## 3. AC 対応トレーサビリティ（新用語）

| AC | 主担当 Step | 検証 |
|----|-------------|------|
| AC-1 | Step 2, 3 | 解析結果再利用テスト |
| AC-2 | Step 2 | hash 変更時再解析テスト |
| AC-3 | Step 3 | DynLibDeps 保持テスト |
| AC-4 | Step 4 | runner 解析結果判定統合テスト |
| AC-5 | Step 2 | 破損読込時再解析テスト |
| AC-6 | Step 3 | record JSON 縮小確認 |
| AC-7 | Step 3, 4 | wrapper/VDSO 除外テスト |
| AC-8 | Step 8 | fmt/test/lint |
| AC-9 | Step 4 | dynamic_load_symbols 高リスク判定テスト |
| AC-10 | Step 3 | サイズ超過 error 伝播テスト |
| AC-12 | Step 3 | 不在ファイル error 伝播テスト |
| AC-13 | Step 3 | VDSO 専用除外テスト |
