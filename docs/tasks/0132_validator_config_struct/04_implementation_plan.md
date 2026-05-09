# 実装計画書: Validator 設定構造体への移行

## 受け入れ条件とタスクの対応表

| AC | 内容 | 対応タスク |
|---|---|---|
| AC-1 | `ValidatorConfig{}` を渡した `New` が「セッターなし」構成と同一の挙動をする | 1.1, 1.2 — 移行後の既存テスト全通過で自動確認 |
| AC-2 | 削除セッター（`SetBinaryAnalyzer` 等）の呼び出しがコンパイルエラーになる | 1.3 — ビルド成功 ＝ 残存呼び出しが 0 を意味する |
| AC-3 | `SetDynamicLibAnalysisStore` が `dynamicanalysis.New(dir, fv)` 後に呼び出せる | 1.3 — validator_library_analysis_test.go の `validatorWithStore` が残存テスト |
| AC-4 | `cmd/record/main.go` が全設定を `ValidatorConfig` 経由で渡し旧セッター呼び出しが残っていない | 3.1, 3.2 — ビルド成功 ＋ Phase 3 完了で確認 |
| AC-5 | `make fmt` / `make test` / `make lint` が全成功する | 5.1 |

---

## フェーズ 1: `ValidatorConfig` 定義とコア変更

### タスク 1.1: `ValidatorConfig` 型の定義

対象: `internal/filevalidator/validator.go`

- [ ] `ValidatorConfig` 構造体をファイル先頭付近（`Validator` 構造体定義より前）に追加する
  ```go
  type ValidatorConfig struct {
      ELFDynLibAnalyzer   *elfdynlib.DynLibAnalyzer
      MachODynLibAnalyzer *machodylib.MachODynLibAnalyzer
      BinaryAnalyzer      binaryanalyzer.BinaryAnalyzer
      SyscallAnalyzer     SyscallAnalyzerInterface
      LibcCache           LibcCacheInterface
      LibSystemCache      LibSystemCacheInterface
      MachoSyscallTable   SyscallNumberTable
      DebugInfo           bool
  }
  ```
- [ ] コメントは英語で記載し日本語を含まないことを確認する

AC カバレッジ: AC-1 (ゼロ値 = 全解析無効)

### タスク 1.2: `New` / `newValidator` シグネチャ変更と設定フィールド反映

対象: `internal/filevalidator/validator.go`

- [ ] `New` の第 3 引数に `cfg ValidatorConfig` を追加する
  ```go
  func New(algorithm HashAlgorithm, hashDir string, cfg ValidatorConfig) (*Validator, error)
  ```
- [ ] `newValidator` の第 4 引数に `cfg ValidatorConfig` を追加し、返却する `&Validator{}` リテラルで各フィールドを設定する
  ```go
  func newValidator(algorithm HashAlgorithm, hashDir common.ResolvedPath, hashFilePathGetter common.HashFilePathGetter, cfg ValidatorConfig) (*Validator, error)
  ```
  設定対象フィールド: `elfDynlibAnalyzer`, `machoDynlibAnalyzer`, `binaryAnalyzer`,
  `syscallAnalyzer`, `libcCache`, `libSystemCache`, `machoSyscallTable`, `includeDebugInfo`
- [ ] `New` から `newValidator` へ `cfg` を転送する

AC カバレッジ: AC-1

### タスク 1.3: セッター削除

対象: `internal/filevalidator/validator.go`

削除対象メソッド（8 個）:
- [ ] `SetELFDynLibAnalyzer`
- [ ] `SetMachODynLibAnalyzer`
- [ ] `SetBinaryAnalyzer`
- [ ] `SetSyscallAnalyzer`
- [ ] `SetLibcCache`
- [ ] `SetLibSystemCache`
- [ ] `SetMachoSyscallTable`
- [ ] `SetIncludeDebugInfo`

存続させるメソッド（変更なし）:
- `SetDynamicLibAnalysisStore` — 循環依存のため除外（02_architecture.md 1.3 参照）

AC カバレッジ: AC-2, AC-3

---

## フェーズ 2: `internal/filevalidator` パッケージ内テスト移行

> フェーズ 1 完了後、パッケージ内テストがコンパイルエラーになるため優先して修正する。

### タスク 2.1: `validatorWithTempHashDir` ヘルパー拡張

対象: `internal/filevalidator/validator_library_analysis_test.go`

- [ ] `validatorWithTempHashDir(t *testing.T)` を
  `validatorWithTempHashDir(t *testing.T, cfg ValidatorConfig)` に変更する
- [ ] 内部の `New(&SHA256{}, hashDir)` を `New(&SHA256{}, hashDir, cfg)` に変更する
- [ ] このヘルパーを呼ぶすべての箇所（同ファイル内）に `ValidatorConfig{}` または必要な設定を渡すよう更新する（下記 2.2 で詳細化）

AC カバレッジ: AC-1 (ゼロ値等価)

### タスク 2.2: `validator_library_analysis_test.go` 内セッター呼び出し移行（19 箇所）

対象: `internal/filevalidator/validator_library_analysis_test.go`

- [ ] `validatorWithTempHashDir(t)` 呼び出し後にセッターを呼ぶ各テストを、
  `validatorWithTempHashDir(t, ValidatorConfig{BinaryAnalyzer: ..., SyscallAnalyzer: ...})` 形式に変更する
- [ ] `validatorWithStore` は `validatorWithTempHashDir(t, ValidatorConfig{})` を呼ぶよう更新する
  （`SetDynamicLibAnalysisStore` の呼び出しはそのまま残す — AC-3 テスト）
- [ ] セッター呼び出し行を削除し、コンパイルエラーがなくなることを確認する

AC カバレッジ: AC-1, AC-2, AC-3

### タスク 2.3: `validator_test.go` 移行（27 `New` 呼び出し + 23 セッター呼び出し）

対象: `internal/filevalidator/validator_test.go`

- [ ] `New(&SHA256{}, hashDir)` を全 27 箇所で `New(&SHA256{}, hashDir, ValidatorConfig{})` に変更する
- [ ] `newValidatorWithStubs(t, libcCache)` ヘルパーを修正する
  - `v.SetLibcCache(libcCache)` を削除し、`New(&SHA256{}, hashDir, ValidatorConfig{LibcCache: libcCache})` に変更する
  - nil ガード（`if libcCache != nil`）は不要になるため削除する
- [ ] 各テスト内で `v.SetXxx(...)` を呼んでいる箇所は、直前の `New` 呼び出しの `ValidatorConfig` に移動する（23 箇所）
- [ ] `newCollisionValidator` ヘルパー内の `newValidator(&SHA256{}, resolvedHashDir, getter)` を
  `newValidator(&SHA256{}, resolvedHashDir, getter, ValidatorConfig{})` に変更する

AC カバレッジ: AC-1, AC-2

### タスク 2.4: `validator_shebang_cache_test.go` 移行（4 `New` + 4 セッター）

対象: `internal/filevalidator/validator_shebang_cache_test.go`

- [ ] `New(&SHA256{}, hashDir)` を 4 箇所で `ValidatorConfig{BinaryAnalyzer: spy}` 付きに変更する
- [ ] `v.SetBinaryAnalyzer(spy)` 呼び出し 4 箇所を削除する

AC カバレッジ: AC-1, AC-2

### タスク 2.5: `validator_macho_test.go` 移行（6 `New` + 9 セッター）

対象: `internal/filevalidator/validator_macho_test.go`

- [ ] `New(&SHA256{}, hashDir)` 6 箇所に対応する `ValidatorConfig` を設定する
- [ ] セッター呼び出し 9 箇所（`SetBinaryAnalyzer`, `SetIncludeDebugInfo`, `SetLibSystemCache`）を削除し、`ValidatorConfig` に移動する

AC カバレッジ: AC-1, AC-2

### タスク 2.6: `validator_macho_darwin_test.go` 移行（1 `New` + 1 セッター）

対象: `internal/filevalidator/validator_macho_darwin_test.go`

- [ ] `New(&SHA256{}, hashDir)` に `ValidatorConfig{MachODynLibAnalyzer: ...}` を追加する
- [ ] `v.SetMachODynLibAnalyzer(...)` を削除する

AC カバレッジ: AC-1, AC-2

### タスク 2.7: `validator_dynlib_sort_linux_test.go` 移行（1 `New` + 1 セッター）

対象: `internal/filevalidator/validator_dynlib_sort_linux_test.go`

- [ ] `New(&SHA256{}, hashDir)` に `ValidatorConfig{ELFDynLibAnalyzer: ...}` を追加する
- [ ] `v.SetELFDynLibAnalyzer(...)` を削除する

AC カバレッジ: AC-1, AC-2

### タスク 2.8: その他パッケージ内テストファイルの `New` 呼び出し更新

- [ ] `validator_error_test.go` — 3 箇所に `ValidatorConfig{}` を追加する
- [ ] `validator_shebang_test.go` — 7 箇所に `ValidatorConfig{}` を追加する
- [ ] `benchmark_test.go` — 1 箇所に `ValidatorConfig{}` を追加する

AC カバレッジ: AC-1

---

## フェーズ 3: `cmd/record/main.go` 更新

### タスク 3.1: `validatorFactory` 型変更

対象: `cmd/record/main.go`

- [ ] `deps.validatorFactory` フィールドの型を変更する
  ```go
  validatorFactory func(hashDir string, cfg filevalidator.ValidatorConfig) (*filevalidator.Validator, error)
  ```
- [ ] `defaultDeps()` 内の factory ラムダを更新する
  ```go
  validatorFactory: func(hashDir string, cfg filevalidator.ValidatorConfig) (*filevalidator.Validator, error) {
      return filevalidator.New(&filevalidator.SHA256{}, hashDir, cfg)
  },
  ```

AC カバレッジ: AC-4

### タスク 3.2: `run()` 内での `ValidatorConfig` 組み立てとセッター呼び出し削除

対象: `cmd/record/main.go`

- [ ] `elfDynlibAnalyzerFactory` / `machoDynlibAnalyzerFactory` を呼び出す前に
  `ValidatorConfig` を組み立てるコードブロックを追加する
  ```go
  vCfg := filevalidator.ValidatorConfig{
      BinaryAnalyzer:      security.NewBinaryAnalyzer(runtime.GOOS),
      SyscallAnalyzer:     libccache.NewSyscallAdapter(syscallAnalyzer),
      LibcCache:           libccache.NewCacheAdapter(cacheMgr, syscallAnalyzer),
      LibSystemCache:      libccache.NewMachoLibSystemAdapter(machoCacheMgr, safeFS),
      MachoSyscallTable:   libccache.MacOSSyscallTable{},
      DebugInfo:           cfg.debugInfo,
  }
  if d.elfDynlibAnalyzerFactory != nil {
      vCfg.ELFDynLibAnalyzer = d.elfDynlibAnalyzerFactory()
  }
  if d.machoDynlibAnalyzerFactory != nil {
      vCfg.MachODynLibAnalyzer = d.machoDynlibAnalyzerFactory()
  }
  ```
- [ ] `d.validatorFactory(cfg.hashDir)` を `d.validatorFactory(cfg.hashDir, vCfg)` に変更する
- [ ] 旧セッター呼び出し 8 行（`SetELFDynLibAnalyzer`, `SetMachODynLibAnalyzer`, `SetBinaryAnalyzer`,
  `SetSyscallAnalyzer`, `SetLibcCache`, `SetIncludeDebugInfo`, `SetLibSystemCache`,
  `SetMachoSyscallTable`）を削除する
- [ ] `SetDynamicLibAnalysisStore` の呼び出しはそのまま残す

> **実行順序の変更について**: 現状のコードでは `syscallAnalyzer`（line 143）・`cacheMgr`（line 157）・
> `machoCacheMgr`（line 167）の生成が `validatorFactory` 呼び出し（line 128）の**後**にある。
> `ValidatorConfig` にこれらを渡すには生成順序を以下に変更する必要がある。
>
> 1. `safeFS` を生成する（現 line 155 → 先頭へ移動）
> 2. `syscallAnalyzer` を生成する
> 3. `cacheMgr` を生成する（失敗時に早期 return — エラーハンドリングは維持する）
> 4. `machoCacheMgr` を生成する（同上）
> 5. `ValidatorConfig` を組み立て、`validatorFactory` を呼ぶ
> 6. `dynamicanalysis.New(dynlibStoreDir, validator)` を呼ぶ（`validator` が必要なため変更不可）
> 7. `SetDynamicLibAnalysisStore` を呼ぶ（変更なし）

AC カバレッジ: AC-4

### タスク 3.3: `cmd/record/main_test.go` factory 型更新

対象: `cmd/record/main_test.go`

- [ ] `testRunDeps` は `defaultDeps()` を呼ぶだけなので、factory 型変更は自動的に反映される
  — コンパイルが通ることで確認する
- [ ] `filevalidator.New` を直接呼ぶ 2 箇所（line 257, 278）に `filevalidator.ValidatorConfig{}` を追加する

AC カバレッジ: AC-4, AC-5

---

## フェーズ 4: 外部プロダクションコードの更新

### タスク 4.1: `internal/cmdcommon/common.go`

- [ ] `filevalidator.New(&filevalidator.SHA256{}, hashDir)` を
  `filevalidator.New(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})` に変更する

### タスク 4.2: `internal/runner/base/security/hash_validation.go`

- [ ] `filevalidator.New(&filevalidator.SHA256{}, hashDir)` に `filevalidator.ValidatorConfig{}` を追加する

### タスク 4.3: `internal/verification/manager.go`

- [ ] `filevalidator.New(&filevalidator.SHA256{}, hashDir)` に `filevalidator.ValidatorConfig{}` を追加する

AC カバレッジ: AC-5 (コンパイル成功)

---

## フェーズ 5: 外部テストの更新

### タスク 5.1: `internal/libccache/integration_test.go`（1 `New` + 4 セッター）

- [ ] `filevalidator.New(&filevalidator.SHA256{}, hashDir)` に対応する `ValidatorConfig`
  （`ELFDynLibAnalyzer`, `SyscallAnalyzer`, `LibcCache`, `DebugInfo: true`）を渡すよう変更する
- [ ] セッター呼び出し 4 行を削除する

### タスク 5.2: `internal/libccache/integration_darwin_test.go`（2 `New` + 2 セッター）

- [ ] `filevalidator.New` の 2 箇所のうちセッターを呼ぶ箇所（line 42 付近）に
  `ValidatorConfig{MachODynLibAnalyzer: ..., LibSystemCache: ...}` を渡すよう変更する
- [ ] もう 1 箇所（line 240 付近）は `ValidatorConfig{}` のみ追加する
- [ ] セッター呼び出し 2 行を削除する

### タスク 5.3: `internal/verification/manager_macho_test.go`（2 `New` + 2 セッター）

- [ ] 各 `filevalidator.New` 呼び出し後の `SetMachODynLibAnalyzer` を
  `ValidatorConfig{MachODynLibAnalyzer: ...}` に移動する

### タスク 5.4: その他外部テストの `New` 呼び出し更新（`ValidatorConfig{}` 追加のみ）

- [ ] `internal/runner/e2e_shebang_test.go` — 4 箇所
- [ ] `internal/runner/bootstrap/config_test.go` — 1 箇所
- [ ] `internal/runner/config/loader_verification_test.go` — 1 箇所
- [ ] `cmd/runner/integration_security_test.go` — 1 箇所
- [ ] `test/security/hash_bypass_test.go` — 4 箇所

AC カバレッジ: AC-5 (コンパイル成功)

---

## フェーズ 6: 品質確認

### タスク 6.1: フォーマット・テスト・リント

- [ ] `make fmt` を実行してフォーマットエラーがないことを確認する
- [ ] `go test -tags test -v ./...` を実行して全テストが通ることを確認する
- [ ] `make lint` を実行して linter エラーがないことを確認する

AC カバレッジ: AC-5

---

## 実施上の注意事項

1. フェーズ 1 完了後、コンパイルエラーが多数発生する。フェーズ 2→5 を順に修正する。
2. `ValidatorConfig` のフィールド名は Go の公開命名規則（`DebugInfo` 等）を使用し、
   既存の private フィールド名（`includeDebugInfo`）とは意図的に異なる。
3. コード（コメント含む）に日本語を使用しない。
4. `newValidator` の `cfg ValidatorConfig` 引数は、`hashFilePathGetter` の後に追加する
   （既存の引数順を乱さないため）。

---

## 受け入れ条件の検証方法

**AC-1**: `ValidatorConfig{}` ゼロ値等価性
- 検証: フェーズ 2 完了後に既存テスト（旧来は「セッターなし」で動作していたもの）が
  `ValidatorConfig{}` を渡すだけで全通過することで自動確認される。
- テスト: `validator_test.go` の `TestRecord_*` 等（既存テスト群を移行したもの）

**AC-2**: 削除セッターがコンパイルエラーを引き起こす
- 検証: ビルド成功 ＝ 全呼び出し箇所が除去済みであることの証明。
  `go build ./...` が通れば旧セッター呼び出しは存在しない。

**AC-3**: `SetDynamicLibAnalysisStore` の継続利用
- テスト: `internal/filevalidator/validator_library_analysis_test.go` — `validatorWithStore`
  が `New(cfg) → dynamicanalysis.New(dir, v) → SetDynamicLibAnalysisStore(store)` の順で呼び、
  `TestAnalyzeLibraries_*` が動作することで確認。

**AC-4**: `cmd/record` での旧セッター完全排除
- 検証: タスク 3.2 でセッター行 8 本を削除 ＋ ビルド成功で確認。

**AC-5**: `make fmt` / `make test` / `make lint` 全成功
- 検証: タスク 6.1 の実行結果で確認。
