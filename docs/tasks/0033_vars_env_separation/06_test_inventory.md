# テストインベントリ: vars-env分離 (Task 0033)

このドキュメントは、`${VAR}`構文削除に伴うテスト変更の完全なインベントリです。

## 概要統計

### 削除されたテストファイル (完全削除)
| ファイル | テスト | ベンチマーク | 総行数 |
|------|-------|-----------|-------------|
| `allowlist_violation_test.go` | 5 | 0 | 289 |
| `expansion_auto_env_test.go` | 3 | 0 | ~150 |
| `expansion_bench_test.go` | 0 | 7 | ~200 |
| `expansion_benchmark_test.go` | 0 | 2 | ~100 |
| `expansion_cmdenv_test.go` | 5 | 0 | ~250 |
| `expansion_self_ref_test.go` | 4 | 0 | ~200 |
| **合計** | **24** | **9** | **~1,189** |

### 修正されたテストファイル (部分削除/変更)
| ファイル | 変更前テスト数 | 変更後テスト数 | 削除テスト数 | 変更行数 |
|------|--------------|-------------|---------------|---------------|
| `expansion_test.go` | 109 | 62 | **47** | -3,099 |
| `runner_test.go` | 27 | 22 | **5** | -517 |
| `loader_test.go` | 8 | 8 | 0 | -65 (内容変更) |
| `loader_e2e_test.go` | 5 | 5 | 0 | -71 (内容変更) |
| `bootstrap/config_test.go` | 3 | 3 | 0 | -28 (内容変更) |
| **合計** | **152** | **100** | **52** | **-3,780** |

### 全体サマリー
- **完全削除されたファイル**: 6ファイル (24テスト + 9ベンチマーク)
- **修正されたファイル**: 5ファイル (52テスト削除、多数のテスト内容変更)
- **削除されたテスト総数**: 76テスト + 9ベンチマーク = **85テスト/ベンチマーク**
- **削除された総行数**: ~4,969行

---

## 削除されたテスト詳細

### 1. allowlist_violation_test.go (削除)

**ステータス**: ✅ 分析済み
- 総行数: 289
- テスト数: 5
- すべてのテストは**カテゴリB: 共通機能(セキュリティクリティカル)**

| テスト関数 | テストタイプ | 説明 | クリティカル? |
|---------------|-----------|-------------|-----------|
| `TestAllowlistViolation_Global` | Unit | globalのenv変数参照でのallowlist違反をテスト | ✅ はい (セキュリティ) |
| `TestAllowlistViolation_Group` | Unit | groupのenv変数参照でのallowlist違反をテスト | ✅ はい (セキュリティ) |
| `TestAllowlistViolation_Command` | Unit | commandのenv変数参照でのallowlist違反をテスト | ✅ はい (セキュリティ) |
| `TestAllowlistViolation_VerifyFiles` | Unit | verify_filesパス参照でのallowlist違反をテスト | ✅ はい (セキュリティ) |
| `TestAllowlistViolation_EmptyAllowlist` | Unit | allowlistが空の場合の動作をテスト | ✅ はい (セキュリティ) |

**テスト詳細**:
- すべてのテストは環境変数参照がallowlistを尊重することを検証
- global、group、commandレベルをカバー
- allowlist強制を伴うverify_files展開をテスト
- 空のallowlistはすべての参照をブロックすべき

**カテゴリ**: B (共通機能 - セキュリティ)
**正当化が必要**: ⚠️ **クリティカル - 同等の%{VAR}テストが必要**
- Allowlist強制はコアセキュリティ機能
- %{VAR}構文でも同一に動作する必要がある
- これらのテストは削除ではなく移行されるべきだった

---

### 2. expansion_auto_env_test.go (削除)

**ステータス**: ✅ 分析済み
- 総行数: ~150
- テスト数: 3
- すべてのテストは**カテゴリB: 共通機能(コア機能)**

| テスト関数 | テストタイプ | 説明 | クリティカル? |
|---------------|-----------|-------------|-----------|
| `TestExpandGlobalEnv_AutomaticEnvironmentVariables` | Unit | globalのenvで自動env変数(PID、DATETIMEなど)をテスト | ✅ はい |
| `TestExpandGroupEnv_AutomaticEnvironmentVariables` | Unit | groupのenvで自動env変数をテスト | ✅ はい |
| `TestConfigLoader_AutomaticEnvironmentVariables_Integration` | Integration | config読み込み全体での自動env変数の統合テスト | ✅ はい |

**テスト詳細**:
- %{__runner_pid}、%{__runner_datetime}のような自動変数をテスト
- globalとgroupレベルで自動変数が動作することを検証
- 統合テストはエンドツーエンドの動作を検証

**カテゴリ**: B (共通機能 - コア)
**正当化が必要**: ⚠️ **同等の%{VAR}テストが必要**
- 自動変数はコア機能
- %{VAR}構文で動作する必要がある(実際、これらは%{VAR}変数!)
- これらのテストはenvironmentパッケージに移動された可能性がある?

---

### 3. expansion_bench_test.go (削除)

**ステータス**: ✅ 分析済み
- 総行数: ~200
- ベンチマーク数: 7
- すべてのベンチマークは**カテゴリD: パフォーマンステスト**

| ベンチマーク関数 | テストタイプ | 説明 |
|-------------------|-----------|-------------|
| `BenchmarkExpandGlobalEnv` | Benchmark | global env展開のパフォーマンスをベンチマーク |
| `BenchmarkExpandGroupEnv` | Benchmark | group env展開のパフォーマンスをベンチマーク |
| `BenchmarkExpandCommandEnv` | Benchmark | command env展開のパフォーマンスをベンチマーク |
| `BenchmarkLoadConfigWithEnvs` | Benchmark | env展開を伴うconfig読み込みをベンチマーク |
| `BenchmarkLoadConfigWithoutEnvs` | Benchmark | env展開なしのconfig読み込みをベンチマーク |
| `BenchmarkExpandGlobalEnv_LargeConfig` | Benchmark | 大規模configでの展開をベンチマーク |
| `BenchmarkExpandGroupEnv_ComplexReferences` | Benchmark | 複雑な変数参照での展開をベンチマーク |

**カテゴリ**: D (パフォーマンス/ベンチマーク)
**正当化**: ✅ **許容可能な削除**
- ベンチマークは統合可能
- 正確性にとって重要ではない
- パフォーマンス問題が発生したら後で追加可能
- **推奨事項**: 低優先度 - 将来的に%{VAR}ベンチマークの追加を検討

---

### 4. expansion_benchmark_test.go (削除)

**ステータス**: ✅ 分析済み
- 総行数: ~100
- ベンチマーク数: 2
- すべてのベンチマークは**カテゴリD: パフォーマンステスト**

| ベンチマーク関数 | テストタイプ | 説明 |
|-------------------|-----------|-------------|
| `BenchmarkAllowlistLookup` | Benchmark | allowlist検索のパフォーマンスをベンチマーク |
| `BenchmarkExpandEnvInternalAllowlistLookup` | Benchmark | 展開中の内部allowlist検索をベンチマーク |

**カテゴリ**: D (パフォーマンス/ベンチマーク)
**正当化**: ✅ **許容可能な削除**
- パフォーマンスベンチマーク、正確性テストではない
- **推奨事項**: 低優先度

---

### 5. expansion_cmdenv_test.go (削除)

**ステータス**: ✅ 分析済み
- 総行数: ~250
- テスト数: 5
- すべてのテストは**カテゴリB: 共通機能(コア機能)**

| テスト関数 | テストタイプ | 説明 | クリティカル? |
|---------------|-----------|-------------|-----------|
| `TestExpandCommandEnv_WithGlobalEnv` | Unit | global envが利用可能な状態でのcommand env展開をテスト | ✅ はい |
| `TestExpandCommandEnv_WithGroupEnv` | Unit | group envが利用可能な状態でのcommand env展開をテスト | ✅ はい |
| `TestExpandCommandEnv_WithBothGlobalAndGroupEnv` | Unit | globalとgroup env両方でのcommand env展開をテスト | ✅ はい |
| `TestExpandCommandEnv_VariableReferencePriority` | Unit | 変数参照解決の優先順位をテスト | ✅ はい |
| `TestExpandCommandEnv_AllowlistInheritance` | Unit | command env展開でのallowlist継承をテスト | ✅ はい (セキュリティ) |

**テスト詳細**:
- コマンドレベルの環境変数展開をテスト
- globalとgroupレベルからの適切な継承を検証
- 変数参照優先順位(command > group > global > system)をテスト
- allowlist継承を検証

**カテゴリ**: B (共通機能 - コア + セキュリティ)
**正当化が必要**: ⚠️ **同等の%{VAR}テストが必要**
- コマンドenv展開はコア機能
- 優先順位ルールは正しい動作にとって重要
- Allowlist継承はセキュリティ上重要

---

### 6. expansion_self_ref_test.go (削除)

**ステータス**: ✅ 分析済み
- 総行数: ~200
- テスト数: 4
- テストは**カテゴリB: 共通機能(コア機能)**

| テスト関数 | テストタイプ | 説明 | クリティカル? |
|---------------|-----------|-------------|-----------|
| `TestExpandGlobalEnv_SelfReferenceToSystemEnv` | Unit | globalのenvでのシステム環境変数への自己参照をテスト | ✅ はい |
| `TestExpandGroupEnv_SelfReferenceToSystemEnv` | Unit | groupのenvでのシステム環境変数への自己参照をテスト | ✅ はい |
| `TestSelfReferenceIntegration` | Integration | 自己参照シナリオの統合テスト | ✅ はい |
| `TestSelfReferenceWithAutomaticVariables` | Integration | 自動変数と組み合わせた自己参照をテスト | ✅ はい |

**テスト詳細**:
- 自己参照変数(例: PATH="${PATH}:/new/path")をテスト
- globalとgroupレベルで自己参照が動作することを検証
- 統合テストは他の機能との自己参照を検証

**カテゴリ**: B (共通機能 - コア)
**正当化が必要**: ⚠️ **同等の%{VAR}テストが必要**
- 自己参照は一般的で有用なパターン
- %{VAR}構文で正しく動作する必要がある
- 自動変数との統合テストが必要

---

## 修正されたテスト詳細

### 7. internal/runner/bootstrap/config_test.go (修正)

**ステータス**: ✅ 分析済み
- 変更前テスト数: 3
- 変更後テスト数: 3
- 削除テスト数: 0
- 修正テスト数: **内容変更のみ** (-28行)
- 影響レベル: **低** (テストデータが%{VAR}構文に更新)

**分析**: Bootstrap configテストは%{VAR}構文に更新されたが、テストカバレッジは維持された。

---

### 8. internal/runner/config/expansion_test.go (修正)

**ステータス**: ✅ 分析済み
- 変更前テスト数: 109
- 変更後テスト数: 62
- 削除テスト数: **47**
- 影響レベル: **高** (テストカバレッジが大幅に削減)

#### 削除されたテスト (合計47)

##### カテゴリA: ${VAR}構文専用 (3テスト)
| テスト関数 | テストタイプ | 説明 |
|---------------|-----------|-------------|
| `TestDetectDollarSyntax_Found` | Unit | ${VAR}構文の検出をテスト |
| `TestDetectDollarSyntax_NotFound` | Unit | ${VAR}構文の不在をテスト |
| `TestDetectDollarSyntax_Escaped` | Unit | エスケープされた${VAR}構文をテスト |

**正当化**: これらのテストは非推奨の${VAR}構文検出に特化

##### カテゴリB: 共通機能 - 展開ロジック (25テスト)
| テスト関数 | テストタイプ | 説明 | クリティカル? |
|---------------|-----------|-------------|-----------|
| `TestExpandGlobalEnv_Basic` | Unit | 基本的なglobal env展開 | ✅ はい |
| `TestExpandGlobalEnv_Empty` | Unit | 空のglobal env処理 | ✅ はい |
| `TestExpandGlobalEnv_EmptyValue` | Unit | 展開での空値 | ✅ はい |
| `TestExpandGlobalEnv_VariableReference` | Unit | 変数参照解決 | ✅ はい |
| `TestExpandGlobalEnv_SystemEnvReference` | Unit | システムenv参照 | ✅ はい |
| `TestExpandGlobalEnv_SelfReference` | Unit | 自己参照検出 | ✅ はい |
| `TestExpandGlobalEnv_CircularReference` | Unit | 循環参照検出 | ✅ はい |
| `TestExpandGlobalEnv_DuplicateKey` | Unit | 重複キー処理 | ✅ はい |
| `TestExpandGlobalEnv_InvalidFormat` | Unit | 無効フォーマットエラー処理 | ✅ はい |
| `TestExpandGlobalEnv_SpecialCharacters` | Unit | 特殊文字処理 | ✅ はい |
| `TestExpandGlobalEnv_AllowlistViolation` | Unit | Allowlist違反検出 | ✅ はい (セキュリティ) |
| `TestExpandGroupEnv_Basic` | Unit | 基本的なgroup env展開 | ✅ はい |
| `TestExpandGroupEnv_Empty` | Unit | 空のgroup env処理 | ✅ はい |
| `TestExpandGroupEnv_ReferenceGlobal` | Unit | global envへの参照 | ✅ はい |
| `TestExpandGroupEnv_ReferenceSystemEnv` | Unit | システムenvへの参照 | ✅ はい |
| `TestExpandGroupEnv_CircularReference` | Unit | groupでの循環参照 | ✅ はい |
| `TestExpandGroupEnv_DuplicateKey` | Unit | groupでの重複キー | ✅ はい |
| `TestExpandGroupEnv_AllowlistInherit` | Unit | Allowlist継承 | ✅ はい (セキュリティ) |
| `TestExpandGroupEnv_AllowlistOverride` | Unit | Allowlist上書き | ✅ はい (セキュリティ) |
| `TestExpandGroupEnv_AllowlistReject` | Unit | Allowlist拒否 | ✅ はい (セキュリティ) |
| `TestExpandCommandEnv` | Unit | Command env展開 | ✅ はい |
| `TestExpandCommandStrings` | Unit | コマンド文字列展開 | ✅ はい |
| `TestExpandCommandStrings_SingleCommand` | Unit | 単一コマンド展開 | ✅ はい |
| `TestExpandCommandStrings_AutoEnv` | Unit | コマンド文字列での自動env | ✅ はい |
| `TestCircularReferenceDetection` | Unit | 汎用循環参照検出 | ✅ はい |

**正当化が必要**: これらは%{VAR}構文で存在すべきコア展開ロジックをテスト

##### カテゴリB: 共通機能 - Verify Files (8テスト)
| テスト関数 | テストタイプ | 説明 | クリティカル? |
|---------------|-----------|-------------|-----------|
| `TestExpandGlobalVerifyFiles` | Unit | Globalのverify_files展開 | ✅ はい |
| `TestExpandGlobalVerifyFiles_WithGlobalEnv` | Unit | global envを伴うverify files | ✅ はい |
| `TestExpandGlobalVerifyFiles_SystemEnv` | Unit | verify filesでのシステムenv | ✅ はい |
| `TestExpandGlobalVerifyFiles_Priority` | Unit | verify filesでの変数優先順位 | ✅ はい |
| `TestExpandGroupVerifyFiles` | Unit | Groupのverify_files展開 | ✅ はい |
| `TestExpandGroupVerifyFiles_WithGlobalEnv` | Unit | global envを伴うverify files | ✅ はい |
| `TestExpandGroupVerifyFiles_WithGroupEnv` | Unit | group envを伴うverify files | ✅ はい |
| `TestExpandGroupVerifyFiles_Priority` | Unit | groupのverify filesでの優先順位 | ✅ はい |

**正当化が必要**: Verify files展開は重要なセキュリティ機能

##### カテゴリB: 共通機能 - from_env解決 (2テスト)
| テスト関数 | テストタイプ | 説明 | クリティカル? |
|---------------|-----------|-------------|-----------|
| `TestResolveGroupFromEnv` | Unit | from_env解決 | ✅ はい |
| `TestResolveGroupFromEnv_AllowlistInheritance` | Unit | from_envでのAllowlist | ✅ はい (セキュリティ) |

**正当化が必要**: from_envはコア機能

##### カテゴリC: 統合テスト (7テスト)
| テスト関数 | テストタイプ | 説明 | クリティカル? |
|---------------|-----------|-------------|-----------|
| `TestConfigLoader_GlobalEnvIntegration` | Integration | global envを伴うconfig loader | ✅ はい |
| `TestConfigLoader_GlobalEnvError` | Integration | loaderでのエラー処理 | ✅ はい |
| `TestConfigLoader_BackwardCompatibility` | Integration | 後方互換性チェック | ⚠️ おそらく |
| `TestExpandCommand_CommandEnvExpansionError` | Integration | コマンド展開エラー | ✅ はい |
| `TestExpandCommand_AutoEnvInCommandEnv` | Integration | コマンドでの自動env | ✅ はい |
| `TestSecurityIntegration` | Integration | セキュリティ統合テスト | ✅ はい (セキュリティ) |
| `TestSecurityAttackPrevention` | Integration | 攻撃防止テスト | ✅ はい (セキュリティ) |

**正当化が必要**: 統合テストはコンポーネント間の相互作用を検証

##### カテゴリD: エッジケースとその他 (2テスト)
| テスト関数 | テストタイプ | 説明 | クリティカル? |
|---------------|-----------|-------------|-----------|
| `TestExpandCommandConfig_NilCommand` | Unit | Nilコマンド処理 | ⚠️ おそらく |
| `TestProcessEnv_ReservedVariableName` | Unit | 予約変数名チェック | ✅ はい |

**expansion_test.goのサマリー**:
- カテゴリA (${VAR}専用): 3テスト → ✅ 削除は正当
- カテゴリB (共通機能): 35テスト → ⚠️ **同等の%{VAR}テストが必要**
- カテゴリC (統合): 7テスト → ⚠️ **同等の%{VAR}テストが必要**
- カテゴリD (その他): 2テスト → ⚠️ 要レビュー

---

### 9. internal/runner/config/loader_e2e_test.go (修正)

**ステータス**: ✅ 分析済み
- 変更前テスト数: 5
- 変更後テスト数: 5
- 削除テスト数: 0
- 修正テスト数: **内容変更のみ** (-71行)
- 影響レベル: **低** (テストアサーションが%{VAR}構文に更新)

**分析**: テストは${VAR}の代わりに%{VAR}構文を使用するように更新されたが、テスト構造とカバレッジは維持された。

---

### 10. internal/runner/config/loader_test.go (修正)

**ステータス**: ✅ 分析済み
- 変更前テスト数: 8
- 変更後テスト数: 8
- 削除テスト数: 0
- 修正テスト数: **内容変更のみ** (-65行)
- 影響レベル: **低** (テストデータが%{VAR}構文に更新)

**分析**: テストデータとアサーションが%{VAR}構文に更新されたが、テストカバレッジは維持された。

---

### 11. internal/runner/runner_test.go (修正)

**ステータス**: ✅ 分析済み
- 変更前テスト数: 27
- 変更後テスト数: 22
- 削除テスト数: **5**
- 影響レベル: **中** (環境変数優先順位テストが削除)

#### 削除されたテスト (合計5)

##### カテゴリC: 統合/E2Eテスト (5テスト)
| テスト関数 | テストタイプ | 説明 | クリティカル? |
|---------------|-----------|-------------|-----------|
| `TestRunner_EnvironmentVariablePriority` | E2E | runnerでのenv変数優先順位をテスト | ✅ はい |
| `TestRunner_EnvironmentVariablePriority_CurrentImplementation` | E2E | 現在の実装の優先順位をテスト | ✅ はい |
| `TestRunner_EnvironmentVariablePriority_EdgeCases` | E2E | 優先順位のエッジケースをテスト | ✅ はい |
| `TestRunner_resolveEnvironmentVars` | Integration | env変数解決をテスト | ✅ はい |
| `TestRunner_SecurityIntegration` | Integration | runnerでのセキュリティ統合 | ✅ はい (セキュリティ) |

**runner_test.goのサマリー**:
- 5テストすべてがカテゴリC (統合/E2E)
- すべてのテストがクリティカルで重要なランタイム動作をテスト
- ⚠️ **同等の%{VAR}テストが必要** - これらはクリティカルなenv優先順位とセキュリティをテスト

---

## 次のステップ

### Phase 2: 元のテスト目的の分析
- [x] 各削除されたテストの詳細なコード分析
- [x] テスト対象の関数/メソッドを特定
- [x] 入力条件とエッジケースを特定
- [x] ${VAR}専用か共通機能かを判定
- [x] カテゴリ分類 (A/B/C/D)

### Phase 3: 新システムのテストカバレッジ分析
- [x] 各テストについて同等テストを検索
- [x] カバレッジギャップを特定
- [x] リスク評価

### Phase 4: 削除の正当性評価
- [x] 各テストの削除理由を評価
- [x] リスクレベルを判定
- [x] 必要なアクションを特定

### Phase 5: レポート生成
- [x] サマリーレポート作成
- [x] 推奨事項ドキュメント作成
