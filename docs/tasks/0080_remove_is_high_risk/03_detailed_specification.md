# 詳細仕様書: `IsHighRisk` 廃止・`HighRiskReasons` リネーム

## 0. 変更方針

本タスクは型定義の変更（フィールド削除・リネーム）を起点に、コンパイルエラーを順に解消する
単純な構造の変更である。リスク判定ロジック自体は変更しない。

- **フェーズ 1**: 型定義の変更とスキーマバージョン更新（コンパイルエラーの起点）
- **フェーズ 2**: 本体コードの変更（コンパイルエラーの解消）
- **フェーズ 3**: テストコードの変更（テストのコンパイルエラー解消と検証ロジック更新）
- **フェーズ 4**: 受け入れ条件検証

## 1. フェーズ 1: 型定義の変更

### 1.1 `SyscallSummary` から `IsHighRisk` を削除

**ファイル**: `internal/common/syscall_types.go` L67–82

変更前:
```go
type SyscallSummary struct {
	HasNetworkSyscalls  bool `json:"has_network_syscalls"`
	IsHighRisk          bool `json:"is_high_risk"`
	TotalDetectedEvents int  `json:"total_detected_events"`
	NetworkSyscallCount int  `json:"network_syscall_count"`
}
```

変更後:
```go
type SyscallSummary struct {
	HasNetworkSyscalls  bool `json:"has_network_syscalls"`
	TotalDetectedEvents int  `json:"total_detected_events"`
	NetworkSyscallCount int  `json:"network_syscall_count"`
}
```

### 1.2 `HighRiskReasons` → `AnalysisWarnings` リネーム

**ファイル**: `internal/common/syscall_types.go` L99–105

変更前:
```go
	// HighRiskReasons explains why the analysis resulted in high risk, if applicable.
	// Note: With omitempty, nil and empty slice ([]string{}) have different JSON output:
	//   - nil: field is omitted entirely
	//   - []string{}: field appears as "high_risk_reasons": []
	// When initializing, use nil (not empty slice) for no high risk
	// to ensure the field is omitted in JSON output.
	HighRiskReasons []string `json:"high_risk_reasons,omitempty"`
```

変更後:
```go
	// AnalysisWarnings contains observations and warnings generated during analysis.
	// Examples: "syscall number could not be determined", "mprotect PROT_EXEC confirmed".
	// Note: With omitempty, nil and empty slice ([]string{}) have different JSON output:
	//   - nil: field is omitted entirely
	//   - []string{}: field appears as "analysis_warnings": []
	// When initializing, use nil (not empty slice) for no warnings
	// to ensure the field is omitted in JSON output.
	AnalysisWarnings []string `json:"analysis_warnings,omitempty"`
```

### 1.3 スキーマバージョンの更新

**ファイル**: `internal/fileanalysis/schema.go` L19

変更前:
```go
	// Version 5 adds ArgEvalResults for syscall argument evaluation (mprotect PROT_EXEC detection).
	// Load returns SchemaVersionMismatchError for records with schema_version != 5.
	...
	CurrentSchemaVersion = 5
```

変更後:
```go
	// Version 5 adds ArgEvalResults for syscall argument evaluation (mprotect PROT_EXEC detection).
	// Version 6 removes is_high_risk from summary and renames high_risk_reasons to analysis_warnings.
	// Load returns SchemaVersionMismatchError for records with schema_version != 6.
	...
	CurrentSchemaVersion = 6
```

## 2. フェーズ 2: 本体コードの変更

### 2.1 `filevalidator` での `IsHighRisk` 代入削除

**ファイル**: `internal/filevalidator/validator.go` L770–785

変更前:
```go
	return &fileanalysis.SyscallAnalysisData{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture:       elfMachineToArchName(machine),
			DetectedSyscalls:   retained,
			HasUnknownSyscalls: hasUnknown,
			Summary: common.SyscallSummary{
				HasNetworkSyscalls:  hasNetwork,
				TotalDetectedEvents: len(retained),
				NetworkSyscallCount: networkCount,
				IsHighRisk:          hasUnknown,
			},
		},
		AnalyzedAt: time.Now().UTC(),
	}
```

変更後:
```go
	return &fileanalysis.SyscallAnalysisData{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture:       elfMachineToArchName(machine),
			DetectedSyscalls:   retained,
			HasUnknownSyscalls: hasUnknown,
			Summary: common.SyscallSummary{
				HasNetworkSyscalls:  hasNetwork,
				TotalDetectedEvents: len(retained),
				NetworkSyscallCount: networkCount,
			},
		},
		AnalyzedAt: time.Now().UTC(),
	}
```

`IsHighRisk: hasUnknown` の行を削除する。`filevalidator` は事実の記録（`HasUnknownSyscalls`）のみを行い、
リスク判断は runner 側（`elfanalyzer`）に委ねる。

### 2.2 `syscall_analyzer.go` での変更

**ファイル**: `internal/runner/security/elfanalyzer/syscall_analyzer.go`

#### 2.2.1 `HighRiskReasons` → `AnalysisWarnings` リネーム（3 箇所）

L313:
```go
// Before
result.HighRiskReasons = append(result.HighRiskReasons,
// After
result.AnalysisWarnings = append(result.AnalysisWarnings,
```

L339:
```go
// Before
result.HighRiskReasons = append(result.HighRiskReasons,
// After
result.AnalysisWarnings = append(result.AnalysisWarnings,
```

L373, L377:
```go
// Before
result.HighRiskReasons = append(result.HighRiskReasons,
// After
result.AnalysisWarnings = append(result.AnalysisWarnings,
```

#### 2.2.2 `IsHighRisk` 代入の削除（2 箇所）

L368:
```go
// Before
result.Summary.IsHighRisk = true
// After
(行を削除)
```

L386:
```go
// Before
result.Summary.IsHighRisk = result.Summary.IsHighRisk || result.HasUnknownSyscalls
// After
(行を削除)
```

#### 2.2.3 ビルドサマリーコメントの更新

L355 付近のコメントブロック:

変更前:
```go
	// Build summary with consistent field calculation rules:
	// - TotalDetectedEvents: total count of all detected syscall events (Pass 1 + Pass 2)
	// - HasNetworkSyscalls: true if NetworkSyscallCount > 0
	// - IsHighRisk: true if HasUnknownSyscalls or mprotect PROT_EXEC risk detected
	// - NetworkSyscallCount: incremented during Pass 1 and Pass 2
	// These rules ensure convertSyscallResult() in StandardELFAnalyzer correctly
	// interprets the analysis result for network capability detection.
```

変更後:
```go
	// Build summary with consistent field calculation rules:
	// - TotalDetectedEvents: total count of all detected syscall events (Pass 1 + Pass 2)
	// - HasNetworkSyscalls: true if NetworkSyscallCount > 0
	// - NetworkSyscallCount: incremented during Pass 1 and Pass 2
	// Risk derivation (HasUnknownSyscalls || EvalMprotectRisk) is performed
	// by convertSyscallResult() at read time, not stored in Summary.
```

### 2.3 `standard_analyzer.go` での変更

**ファイル**: `internal/runner/security/elfanalyzer/standard_analyzer.go`

#### 2.3.1 `convertSyscallResult` のリスク判定条件の置き換え

L347–365:

変更前:
```go
// convertSyscallResult converts SyscallAnalysisResult to AnalysisOutput.
// This method relies on Summary fields set by analyzeSyscallsInCode():
//   - HasNetworkSyscalls: true if any network-related syscall was detected
//   - IsHighRisk: true if any syscall number could not be determined
//     or mprotect PROT_EXEC risk was detected
//
// These fields are guaranteed to be set according to the rules in the detailed specification.
func (a *StandardELFAnalyzer) convertSyscallResult(result *SyscallAnalysisResult) binaryanalyzer.AnalysisOutput {
	// IsHighRisk takes precedence over NetworkDetected: when unknown syscalls are present,
	// the analysis is incomplete and unreliable, so we must treat the result as an error
	// even if network syscalls were also detected.
	if result.Summary.IsHighRisk {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("%w: %v", ErrSyscallAnalysisHighRisk, result.HighRiskReasons),
		}
	}
```

変更後:
```go
// convertSyscallResult converts SyscallAnalysisResult to AnalysisOutput.
// Risk is derived at read time from primary facts:
//   - HasUnknownSyscalls: true if any syscall number could not be determined
//   - EvalMprotectRisk(ArgEvalResults): true if mprotect PROT_EXEC risk detected
//
// This replaces the former Summary.IsHighRisk field, which was a redundant cache
// of the same derivation. The formula is identical:
//   isHighRisk = HasUnknownSyscalls || EvalMprotectRisk(ArgEvalResults)
func (a *StandardELFAnalyzer) convertSyscallResult(result *SyscallAnalysisResult) binaryanalyzer.AnalysisOutput {
	// Risk takes precedence over NetworkDetected: when unknown syscalls are present
	// or mprotect PROT_EXEC risk is detected, the analysis is incomplete and unreliable,
	// so we must treat the result as an error even if network syscalls were also detected.
	isHighRisk := result.HasUnknownSyscalls || EvalMprotectRisk(result.ArgEvalResults)
	if isHighRisk {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("%w: %v", ErrSyscallAnalysisHighRisk, result.AnalysisWarnings),
		}
	}
```

**ポイント**:
- `result.Summary.IsHighRisk` の参照を `HasUnknownSyscalls || EvalMprotectRisk(ArgEvalResults)` に置き換え
- `result.HighRiskReasons` → `result.AnalysisWarnings` に更新
- ドキュメントコメントから `IsHighRisk` の説明を削除し、新しい導出条件を記述

### 2.4 `mprotect_risk.go` のコメント更新

**ファイル**: `internal/runner/security/elfanalyzer/mprotect_risk.go` L5–6

変更前:
```go
// EvalMprotectRisk evaluates ArgEvalResults for mprotect-related risk.
// Returns true if IsHighRisk should be set based on mprotect detection.
```

変更後:
```go
// EvalMprotectRisk evaluates ArgEvalResults for mprotect-related risk.
// Returns true if mprotect-derived risk exists (used for AnalysisWarnings
// entries and risk derivation in convertSyscallResult).
```

## 3. フェーズ 3: テストコードの変更

### 3.1 `syscall_types_test.go`

**ファイル**: `internal/common/syscall_types_test.go`

#### 3.1.1 `TestSyscallSummary_JSONRoundTrip`（L87–101）

`IsHighRisk` フィールドの設定を削除する。

変更前:
```go
	original := common.SyscallSummary{
		HasNetworkSyscalls:  true,
		IsHighRisk:          false,
		TotalDetectedEvents: 5,
		NetworkSyscallCount: 2,
	}
```

変更後:
```go
	original := common.SyscallSummary{
		HasNetworkSyscalls:  true,
		TotalDetectedEvents: 5,
		NetworkSyscallCount: 2,
	}
```

#### 3.1.2 `TestSyscallAnalysisResultCore_JSONRoundTrip`（L106–170）

- `HighRiskReasons` → `AnalysisWarnings` にリネーム
- `IsHighRisk` フィールドの設定を削除
- `"high_risk_reasons omitted when nil"` サブテストのキー名を `"analysis_warnings"` に変更

変更前（L117–128）:
```go
		original := common.SyscallAnalysisResultCore{
			...
			HighRiskReasons:    []string{"unknown:indirect_setting"},
			Summary: common.SyscallSummary{
				...
				IsHighRisk:          true,
				...
			},
		}
```

変更後:
```go
		original := common.SyscallAnalysisResultCore{
			...
			AnalysisWarnings:    []string{"unknown:indirect_setting"},
			Summary: common.SyscallSummary{
				...
			},
		}
```

変更前（L140–153）— `"high_risk_reasons omitted when nil"` サブテスト:
```go
	t.Run("high_risk_reasons omitted when nil", func(t *testing.T) {
		core := common.SyscallAnalysisResultCore{
			...
			HighRiskReasons:    nil,
			...
		}
		...
		_, hasHighRisk := m["high_risk_reasons"]
		assert.False(t, hasHighRisk, "high_risk_reasons should be omitted when nil")
	})
```

変更後:
```go
	t.Run("analysis_warnings omitted when nil", func(t *testing.T) {
		core := common.SyscallAnalysisResultCore{
			...
			AnalysisWarnings:    nil,
			...
		}
		...
		_, hasWarnings := m["analysis_warnings"]
		assert.False(t, hasWarnings, "analysis_warnings should be omitted when nil")
	})
```

### 3.2 `validator_test.go`

**ファイル**: `internal/filevalidator/validator_test.go`

#### 3.2.1 `TestSaveRecord_PreservesSyscallAnalysis`（L855–895 付近）

`HighRiskReasons` → `AnalysisWarnings` にリネーム。

変更前:
```go
			HighRiskReasons:    []string{"test reason"},
```
```go
	require.Len(t, record.SyscallAnalysis.HighRiskReasons, 1, "HighRiskReasons should be preserved")
	assert.Equal(t, "test reason", record.SyscallAnalysis.HighRiskReasons[0], "HighRiskReason content should be preserved")
```

変更後:
```go
			AnalysisWarnings:    []string{"test reason"},
```
```go
	require.Len(t, record.SyscallAnalysis.AnalysisWarnings, 1, "AnalysisWarnings should be preserved")
	assert.Equal(t, "test reason", record.SyscallAnalysis.AnalysisWarnings[0], "AnalysisWarning content should be preserved")
```

#### 3.2.2 `TestBuildSyscallAnalysisData`（L1303–1330 付近）

`IsHighRisk` アサーションを削除する。

変更前（L1314）:
```go
		assert.True(t, data.Summary.IsHighRisk, "IsHighRisk must mirror HasUnknownSyscalls")
```

変更後: 行を削除。

変更前（L1326）:
```go
		assert.False(t, data.Summary.IsHighRisk, "IsHighRisk must mirror HasUnknownSyscalls")
```

変更後: 行を削除。

### 3.3 `syscall_store_test.go`

**ファイル**: `internal/fileanalysis/syscall_store_test.go`

#### 3.3.1 基本ラウンドトリップテスト（L25–64）

`IsHighRisk` フィールドの設定を削除する。

変更前（L47–52）:
```go
			Summary: SyscallSummary{
				HasNetworkSyscalls:  true,
				IsHighRisk:          false,
				TotalDetectedEvents: 1,
				NetworkSyscallCount: 1,
			},
```

変更後:
```go
			Summary: SyscallSummary{
				HasNetworkSyscalls:  true,
				TotalDetectedEvents: 1,
				NetworkSyscallCount: 1,
			},
```

#### 3.3.2 `TestSyscallAnalysisStore_HighRiskReasons`（L146–193）

テスト関数名とフィールド参照を更新する。

- 関数名: `TestSyscallAnalysisStore_HighRiskReasons` → `TestSyscallAnalysisStore_AnalysisWarnings`
- `HighRiskReasons` → `AnalysisWarnings` にリネーム
- `IsHighRisk` フィールドの設定を削除
- アサーション

変更前:
```go
func TestSyscallAnalysisStore_HighRiskReasons(t *testing.T) {
	...
			HighRiskReasons: []string{
				"syscall at 0x402000: number could not be determined (unknown:indirect_setting)",
			},
			Summary: SyscallSummary{
				IsHighRisk:          true,
				TotalDetectedEvents: 1,
			},
	...
	assert.True(t, loadedResult.Summary.IsHighRisk)
	require.Len(t, loadedResult.HighRiskReasons, 1)
	assert.Contains(t, loadedResult.HighRiskReasons[0], "indirect_setting")
```

変更後:
```go
func TestSyscallAnalysisStore_AnalysisWarnings(t *testing.T) {
	...
			AnalysisWarnings: []string{
				"syscall at 0x402000: number could not be determined (unknown:indirect_setting)",
			},
			Summary: SyscallSummary{
				TotalDetectedEvents: 1,
			},
	...
	require.Len(t, loadedResult.AnalysisWarnings, 1)
	assert.Contains(t, loadedResult.AnalysisWarnings[0], "indirect_setting")
```

#### 3.3.3 ArgEvalResults ラウンドトリップテスト（L320–395）

`IsHighRisk` フィールドの設定を削除する。

変更前（L342–345）:
```go
				Summary: SyscallSummary{
					IsHighRisk:          true,
					TotalDetectedEvents: 1,
				},
```

変更後:
```go
				Summary: SyscallSummary{
					TotalDetectedEvents: 1,
				},
```

変更前（L361）:
```go
		assert.True(t, loaded.Summary.IsHighRisk)
```

変更後: 行を削除。

変更前（L378–381）:
```go
				Summary: SyscallSummary{
					IsHighRisk:          false,
					TotalDetectedEvents: 0,
				},
```

変更後:
```go
				Summary: SyscallSummary{
					TotalDetectedEvents: 0,
				},
```

### 3.4 `file_analysis_store_test.go`

**ファイル**: `internal/fileanalysis/file_analysis_store_test.go` L143

変更前:
```go
				HighRiskReasons:    []string{"reason1"},
```

変更後:
```go
				AnalysisWarnings:    []string{"reason1"},
```

### 3.5 `syscall_analyzer_test.go`

**ファイル**: `internal/runner/security/elfanalyzer/syscall_analyzer_test.go`

このファイルでは `result.Summary.IsHighRisk` への参照を削除し、
代わりに `HasUnknownSyscalls` や `EvalMprotectRisk` を使った等価な確認に置き換える。
`HighRiskReasons` → `AnalysisWarnings` にリネームする。

#### 3.5.1 未知syscall検出テスト（L155–170 付近）

変更前:
```go
			assert.True(t, result.Summary.IsHighRisk)
			assert.NotEmpty(t, result.HighRiskReasons)
```

変更後:
```go
			assert.NotEmpty(t, result.AnalysisWarnings)
```

`result.HasUnknownSyscalls` が直前行で確認済みのため、`IsHighRisk` の確認は不要。

#### 3.5.2 複数syscall検出テスト（L240–270 付近）

変更前:
```go
	assert.False(t, result.Summary.IsHighRisk)
```

変更後: 行を削除。`HasUnknownSyscalls = false` の確認が直後にあるため十分。

#### 3.5.3 syscall未検出テスト（L254–270 付近）

変更前:
```go
	assert.False(t, result.Summary.IsHighRisk)
```

変更後: 行を削除。

#### 3.5.4 ネットワーク+未知syscall混在テスト（L320–330 付近）

変更前:
```go
	assert.True(t, result.Summary.IsHighRisk)
```

変更後: 行を削除。`HasUnknownSyscalls = true` の確認が直前にあるため十分。

#### 3.5.5 スキャンリミット超過テスト（L500–535 付近）

変更前:
```go
	assert.True(t, result.Summary.IsHighRisk)
```

変更後: 行を削除。`DeterminationMethodUnknownScanLimitExceeded` の確認で `HasUnknownSyscalls` が暗黙的に保証される。

#### 3.5.6 ウィンドウ枯渇テスト（L525–535 付近）

変更前:
```go
	assert.True(t, result.Summary.IsHighRisk)
```

変更後: 行を削除。

#### 3.5.7 mprotect テスト群（L830–895 付近）

`result.Summary.IsHighRisk` の確認を、テストの文脈に応じて適切な代替に置き換える。

`exec_confirmed` テスト（L838）:
```go
// Before
assert.True(t, result.Summary.IsHighRisk)
// After
assert.True(t, EvalMprotectRisk(result.ArgEvalResults))
```

`exec_unknown + exec_not_set` テスト（L855）:
```go
// Before
assert.True(t, result.Summary.IsHighRisk)
// After
assert.True(t, EvalMprotectRisk(result.ArgEvalResults))
```

`exec_not_set only` テスト（L865）:
```go
// Before
assert.False(t, result.Summary.IsHighRisk)
// After
assert.False(t, EvalMprotectRisk(result.ArgEvalResults))
```

`exec_not_set does not overwrite pre-existing IsHighRisk=true` テスト（L871–890）:

テスト名を更新し、検証を `HasUnknownSyscalls` のみに変更する。

変更前:
```go
	t.Run("exec_not_set does not overwrite pre-existing IsHighRisk=true", func(t *testing.T) {
		...
		// IsHighRisk must remain true (set by HasUnknownSyscalls), not be overwritten by exec_not_set.
		assert.True(t, result.HasUnknownSyscalls)
		assert.True(t, result.Summary.IsHighRisk)
	})
```

変更後:
```go
	t.Run("exec_not_set with HasUnknownSyscalls remains high risk", func(t *testing.T) {
		...
		// HasUnknownSyscalls must remain true regardless of exec_not_set.
		// Risk derivation (HasUnknownSyscalls || EvalMprotectRisk) is done at read time.
		assert.True(t, result.HasUnknownSyscalls)
	})
```

#### 3.5.8 ARM64 mprotect テスト（L905–1010 付近）

テーブル駆動テストの `wantIsHighRisk` フィールドを削除し、
代わりに `EvalMprotectRisk` + `HasUnknownSyscalls` で検証する。

変更前:
```go
	tests := []struct {
		name           string
		code           []byte
		wantStatus     common.SyscallArgEvalStatus
		wantHasResult  bool
		wantIsHighRisk bool
	}{
		{
			name:           "exec_confirmed (mov x2, #7)",
			...
			wantIsHighRisk: true,
		},
		...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			...
			assert.Equal(t, tt.wantIsHighRisk, result.Summary.IsHighRisk)
		})
	}
```

変更後:
```go
	tests := []struct {
		name          string
		code          []byte
		wantStatus    common.SyscallArgEvalStatus
		wantHasResult bool
		wantHighRisk  bool
	}{
		{
			name:         "exec_confirmed (mov x2, #7)",
			...
			wantHighRisk: true,
		},
		...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			...
			gotHighRisk := result.HasUnknownSyscalls || EvalMprotectRisk(result.ArgEvalResults)
			assert.Equal(t, tt.wantHighRisk, gotHighRisk)
		})
	}
```

### 3.6 `analyzer_test.go`

**ファイル**: `internal/runner/security/elfanalyzer/analyzer_test.go`

モックストアが返す `SyscallAnalysisResult` から `IsHighRisk` フィールドの設定を削除し、
`HighRiskReasons` → `AnalysisWarnings` にリネームする。
`convertSyscallResult` は `HasUnknownSyscalls` と `ArgEvalResults` から
リスクを導出するため、モックデータに `IsHighRisk` は不要。

#### 3.6.1 `TestStandardELFAnalyzer_SyscallLookup_HighRisk`（L355–389）

変更前:
```go
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				...
				HasUnknownSyscalls: true,
				HighRiskReasons: []string{
					"syscall at 0x401000: ...",
				},
				Summary: SyscallSummary{
					HasNetworkSyscalls:  false,
					IsHighRisk:          true,
					TotalDetectedEvents: 1,
				},
			},
		},
	}
```

変更後:
```go
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				...
				HasUnknownSyscalls: true,
				AnalysisWarnings: []string{
					"syscall at 0x401000: ...",
				},
				Summary: SyscallSummary{
					HasNetworkSyscalls:  false,
					TotalDetectedEvents: 1,
				},
			},
		},
	}
```

テストの検証対象は `AnalysisError` が返ることであり、`HasUnknownSyscalls = true` により
`convertSyscallResult` がリスクを導出する。

#### 3.6.2 `TestStandardELFAnalyzer_SyscallLookup_HighRiskTakesPrecedenceOverNetwork`（L391–436）

同様に `IsHighRisk` フィールド設定を削除、`HighRiskReasons` → `AnalysisWarnings` にリネーム。

変更前:
```go
				HasUnknownSyscalls: true,
				HighRiskReasons: []string{
					"syscall at 0x401010: ...",
				},
				Summary: SyscallSummary{
					HasNetworkSyscalls:  true,
					NetworkSyscallCount: 1,
					IsHighRisk:          true,
					TotalDetectedEvents: 2,
				},
```

変更後:
```go
				HasUnknownSyscalls: true,
				AnalysisWarnings: []string{
					"syscall at 0x401010: ...",
				},
				Summary: SyscallSummary{
					HasNetworkSyscalls:  true,
					NetworkSyscallCount: 1,
					TotalDetectedEvents: 2,
				},
```

#### 3.6.3 `TestAC3_DynamicELF_SyscallFallback_HighRisk`（L580–610 付近）

変更前:
```go
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				HasUnknownSyscalls: true,
				HighRiskReasons:    []string{"syscall at 0x401000: ..."},
				Summary: SyscallSummary{
					IsHighRisk:          true,
					TotalDetectedEvents: 1,
				},
			},
		},
	}
```

変更後:
```go
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				HasUnknownSyscalls: true,
				AnalysisWarnings:    []string{"syscall at 0x401000: ..."},
				Summary: SyscallSummary{
					TotalDetectedEvents: 1,
				},
			},
		},
	}
```

#### 3.6.4 その他のモックデータ

ファイル内で `IsHighRisk` を設定している他のモックデータがあれば同様に削除する。
`HighRiskReasons` → `AnalysisWarnings` のリネームも全箇所で実施する。

### 3.7 `syscall_analyzer_integration_test.go`

**ファイル**: `internal/runner/security/elfanalyzer/syscall_analyzer_integration_test.go` L398

変更前:
```go
		t.Logf("IsHighRisk: %v", result.Summary.IsHighRisk)
```

変更後: 行を削除。
`HasUnknownSyscalls` は直後の行（L400）で出力されており、
`IsHighRisk` が存在しないためログ出力も不要。

## 4. フェーズ 4: 受け入れ条件検証

### AC-1: `IsHighRisk` フィールド廃止（型定義）

- [ ] 実装箇所: `internal/common/syscall_types.go` L67–82（`SyscallSummary` 構造体）
- [ ] テスト: `internal/common/syscall_types_test.go::TestSyscallSummary_JSONRoundTrip`
- [ ] テスト: `internal/common/syscall_types_test.go::TestSyscallAnalysisResultCore_JSONRoundTrip`
- [ ] 検証方法: `make build` がエラーなく完了すること

### AC-2: `HighRiskReasons` → `AnalysisWarnings` リネーム

- [ ] 実装箇所: `internal/common/syscall_types.go` L99–105（`SyscallAnalysisResultCore` 構造体）
- [ ] テスト: `internal/common/syscall_types_test.go::TestSyscallAnalysisResultCore_JSONRoundTrip` の `"analysis_warnings omitted when nil"` サブテスト
- [ ] 検証方法: `grep -r HighRiskReasons --include='*.go' .` でヒットなし
- [ ] 検証方法: `grep -r HighRiskReasons docs/development docs/user` で該当なし（現時点で該当箇所なし）

### AC-3: `filevalidator` の責務限定

- [ ] 実装箇所: `internal/filevalidator/validator.go` L770–785（`buildSyscallAnalysisData` 関数）
- [ ] テスト: `internal/filevalidator/validator_test.go::TestBuildSyscallAnalysisData`
- [ ] 検証方法: `grep IsHighRisk internal/filevalidator/validator.go` でヒットなし

### AC-4: リスク判定の同値性（リアルタイム解析経路）

- [ ] 実装箇所: `internal/runner/security/elfanalyzer/syscall_analyzer.go` L355–390（`analyzeSyscallsInCode` 末尾）
- [ ] テスト: `internal/runner/security/elfanalyzer/syscall_analyzer_test.go`
  - 未知syscall検出: `TestSyscallAnalyzer_UnknownSyscall_*`
  - mprotect `exec_confirmed`/`exec_unknown`: `TestSyscallAnalyzer_MultipleMprotect`
  - `exec_not_set` のみ: 同テストの `exec_not_set only` サブテスト
  - `exec_not_set` + `HasUnknownSyscalls`: `exec_not_set with HasUnknownSyscalls remains high risk` サブテスト
- [ ] 検証方法: `go test -tags test -v ./internal/runner/security/elfanalyzer/ -run TestSyscallAnalyzer`

### AC-5: リスク判定の同値性（キャッシュ読み取り経路）

- [ ] 実装箇所: `internal/runner/security/elfanalyzer/standard_analyzer.go` L347–365（`convertSyscallResult` 関数）
- [ ] テスト: `internal/runner/security/elfanalyzer/analyzer_test.go`
  - `HasUnknownSyscalls = true` + `ArgEvalResults` 空: `TestStandardELFAnalyzer_SyscallLookup_HighRisk`
  - `HasUnknownSyscalls = true` + ネットワークあり: `TestStandardELFAnalyzer_SyscallLookup_HighRiskTakesPrecedenceOverNetwork`
  - `ArgEvalResults` に `exec_confirmed`: mprotect 高リスクモック使用テスト
  - `HasUnknownSyscalls = false` + `ArgEvalResults` 空: 既存の `NoNetworkSymbols`/`NetworkDetected` テスト
- [ ] 検証方法: `go test -tags test -v ./internal/runner/security/elfanalyzer/ -run TestStandardELFAnalyzer`

### AC-6: JSON スキーマ更新

- [ ] 実装箇所: `internal/fileanalysis/schema.go` L19（`CurrentSchemaVersion = 6`）
- [ ] テスト: `internal/fileanalysis/syscall_store_test.go`
  - 基本ラウンドトリップ: `TestSyscallAnalysisStore_SaveAndLoad`
  - `AnalysisWarnings` ラウンドトリップ: `TestSyscallAnalysisStore_AnalysisWarnings`
  - ArgEvalResults ラウンドトリップ: `TestSyscallAnalysisStore_SchemaV5_ArgEvalResults`
- [ ] テスト: `TestStore_SchemaVersionMismatch`（既存テスト — スキーマ不一致の動作確認）
- [ ] 検証方法: `go test -tags test -v ./internal/fileanalysis/ -run TestSyscall`

### AC-7: 全テスト通過

- [ ] 検証方法: `make test` が全パス
- [ ] 検証方法: `make lint` がエラーなし
