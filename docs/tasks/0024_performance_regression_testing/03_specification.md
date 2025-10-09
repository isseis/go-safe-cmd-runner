# パフォーマンス回帰テストシステム 詳細仕様書

## 1. テストフレームワーク仕様

### 1.1 テスト実行モード

#### 1.1.1 コマンドライン引数
```go
// テストモード指定
-args -mode=baseline           // ベースライン測定モード
-args -mode=current           // 現在測定モード
-args -mode=compare           // 比較モード

// 出力ファイル指定
-args -output=filename.json   // 結果出力ファイル

// 設定ファイル指定
-args -config=config.yaml     // 設定ファイルパス
```

#### 1.1.2 測定モード実装
```go
func TestSubstitutionHashEscape_PerformanceRegression(t *testing.T) {
    mode := getTestMode()
    config := loadConfig()

    switch mode {
    case "baseline":
        result := runBaseline(t, config)
        saveResult(result, getOutputFile())
    case "current":
        result := runCurrent(t, config)
        saveResult(result, getOutputFile())
    case "compare":
        compareResults(t, config)
    default:
        // 従来の固定閾値モード（後方互換性）
        runLegacyMode(t)
    }
}
```

### 1.2 測定エンジン詳細仕様

#### 1.2.1 ベンチマーク定義構造体
```go
type BenchmarkDefinition struct {
    Name        string                 `yaml:"name"`
    Description string                 `yaml:"description"`
    Category    string                 `yaml:"category"`
    Operation   func() error          `yaml:"-"`
    Enabled     bool                  `yaml:"enabled"`
    Timeout     time.Duration         `yaml:"timeout"`
    Setup       func() error          `yaml:"-"`
    Cleanup     func() error          `yaml:"-"`
}

type MeasurementConfig struct {
    WarmupIterations int           `yaml:"warmup_iterations"`
    MeasureIterations int          `yaml:"measure_iterations"`
    TimeoutPerBench  time.Duration `yaml:"timeout_per_bench"`
    OutlierThreshold float64       `yaml:"outlier_threshold"`
    MemoryProfiling  bool          `yaml:"memory_profiling"`
}
```

#### 1.2.2 統計処理アルゴリズム
```go
type StatisticalAnalysis struct {
    RawMeasurements []int64 `json:"raw_measurements_ns"`
    Mean            int64   `json:"mean_ns"`
    Median          int64   `json:"median_ns"`
    StdDev          int64   `json:"std_dev_ns"`
    Min             int64   `json:"min_ns"`
    Max             int64   `json:"max_ns"`
    P95             int64   `json:"p95_ns"`
    P99             int64   `json:"p99_ns"`
    OutliersRemoved int     `json:"outliers_removed"`
}

func calculateStatistics(measurements []int64, config MeasurementConfig) StatisticalAnalysis {
    // 1. 外れ値除去
    filtered := removeOutliers(measurements, config.OutlierThreshold)

    // 2. 基本統計量計算
    sort.Slice(filtered, func(i, j int) bool { return filtered[i] < filtered[j] })

    return StatisticalAnalysis{
        RawMeasurements: measurements,
        Mean:            calculateMean(filtered),
        Median:          calculatePercentile(filtered, 50),
        StdDev:          calculateStdDev(filtered),
        Min:             filtered[0],
        Max:             filtered[len(filtered)-1],
        P95:             calculatePercentile(filtered, 95),
        P99:             calculatePercentile(filtered, 99),
        OutliersRemoved: len(measurements) - len(filtered),
    }
}
```

#### 1.2.3 メモリ測定仕様
```go
type MemoryMeasurement struct {
    AllocatedBytes     int64 `json:"allocated_bytes"`
    AllocationCount    int64 `json:"allocation_count"`
    HeapInUse          int64 `json:"heap_in_use"`
    HeapObjects        int64 `json:"heap_objects"`
    GCCycles           int   `json:"gc_cycles"`
    GCPauseTotal       int64 `json:"gc_pause_total_ns"`
}

func measureMemoryUsage(operation func() error, iterations int) MemoryMeasurement {
    var m1, m2 runtime.MemStats

    // GC実行で状態リセット
    runtime.GC()
    runtime.ReadMemStats(&m1)

    // 測定対象実行
    for i := 0; i < iterations; i++ {
        operation()
    }

    // 最終状態測定
    runtime.GC()
    runtime.ReadMemStats(&m2)

    return MemoryMeasurement{
        AllocatedBytes:  int64(m2.TotalAlloc - m1.TotalAlloc),
        AllocationCount: int64(m2.Mallocs - m1.Mallocs),
        HeapInUse:      int64(m2.HeapInuse),
        HeapObjects:    int64(m2.HeapObjects),
        GCCycles:       int(m2.NumGC - m1.NumGC),
        GCPauseTotal:   int64(totalPauseNs(m2) - totalPauseNs(m1)),
    }
}
```

## 2. データ形式仕様

### 2.1 測定結果JSON形式
```json
{
  "metadata": {
    "timestamp": "2025-01-17T10:30:00Z",
    "commit_hash": "abc123def456",
    "branch": "main",
    "go_version": "go1.23.10",
    "goos": "linux",
    "goarch": "amd64",
    "measurement_mode": "baseline",
    "config_hash": "sha256:...",
    "total_duration_ms": 180000
  },
  "environment": {
    "cpu_model": "Intel(R) Xeon(R) CPU @ 2.50GHz",
    "cpu_cores": 4,
    "memory_total_gb": 16,
    "ci_environment": true,
    "runner_type": "github-actions",
    "load_average": [0.8, 1.2, 1.1]
  },
  "benchmarks": {
    "encode_simple_path": {
      "execution": {
        "warmup_iterations": 10,
        "measure_iterations": 20,
        "timeout_reached": false,
        "errors_occurred": 0
      },
      "timing": {
        "raw_measurements_ns": [201, 198, 205, 201, 199, ...],
        "mean_ns": 201,
        "median_ns": 201,
        "std_dev_ns": 2.5,
        "min_ns": 195,
        "max_ns": 208,
        "p95_ns": 205,
        "p99_ns": 207,
        "outliers_removed": 1
      },
      "memory": {
        "allocated_bytes": 24,
        "allocation_count": 1,
        "heap_in_use": 8192,
        "heap_objects": 100,
        "gc_cycles": 0,
        "gc_pause_total_ns": 0
      }
    }
  }
}
```

### 2.2 比較結果JSON形式
```json
{
  "metadata": {
    "comparison_timestamp": "2025-01-17T10:35:00Z",
    "baseline_commit": "abc123def456",
    "current_commit": "def456ghi789",
    "pr_number": 123,
    "total_benchmarks": 4,
    "failed_benchmarks": 1
  },
  "overall": {
    "status": "WARN",
    "summary": "1 performance regression detected",
    "recommendation": "Review encode_simple_path performance"
  },
  "benchmarks": {
    "encode_simple_path": {
      "status": "FAIL",
      "baseline": {
        "median_ns": 201,
        "allocated_bytes": 24
      },
      "current": {
        "median_ns": 305,
        "allocated_bytes": 36
      },
      "analysis": {
        "time_ratio": 1.52,
        "memory_ratio": 1.50,
        "time_regression": true,
        "memory_regression": true,
        "statistical_significance": 0.001,
        "confidence_level": 99.9
      },
      "message": "Execution time increased by 52% (305ns vs 201ns)",
      "suggestions": [
        "Profile memory allocations in encode function",
        "Check for new string operations or copies"
      ]
    }
  },
  "thresholds": {
    "time_ratio_warn": 1.2,
    "time_ratio_fail": 1.5,
    "memory_ratio_warn": 1.2,
    "memory_ratio_fail": 1.5,
    "statistical_significance": 0.05
  }
}
```

## 3. 比較エンジン仕様

### 3.1 統計的有意性検定
```go
type SignificanceTest struct {
    TestType    string  `json:"test_type"`    // "mannwhitney", "ttest"
    PValue      float64 `json:"p_value"`
    Significant bool    `json:"significant"`
    Confidence  float64 `json:"confidence"`
    Effect      string  `json:"effect"`       // "small", "medium", "large"
}

func performSignificanceTest(baseline, current []int64) SignificanceTest {
    // Mann-Whitney U検定（ノンパラメトリック）
    pValue := mannWhitneyTest(baseline, current)
    significant := pValue < 0.05

    // 効果量計算（Cohen's d）
    effectSize := calculateCohenD(baseline, current)
    effect := categorizeEffectSize(effectSize)

    return SignificanceTest{
        TestType:    "mannwhitney",
        PValue:      pValue,
        Significant: significant,
        Confidence:  (1.0 - pValue) * 100,
        Effect:      effect,
    }
}
```

### 3.2 回帰判定ロジック
```go
type RegressionDecision struct {
    Status         string   `json:"status"`         // PASS, WARN, FAIL
    Reasons        []string `json:"reasons"`
    Severity       string   `json:"severity"`       // LOW, MEDIUM, HIGH
    Action         string   `json:"action"`         // IGNORE, INVESTIGATE, BLOCK
    Confidence     float64  `json:"confidence"`
}

func determineRegression(comparison BenchComparison, thresholds Thresholds) RegressionDecision {
    var reasons []string
    var status string
    var severity string

    // 1. 統計的有意性チェック
    if !comparison.StatTest.Significant {
        return RegressionDecision{
            Status:     "PASS",
            Reasons:    []string{"No statistically significant change"},
            Severity:   "LOW",
            Action:     "IGNORE",
            Confidence: comparison.StatTest.Confidence,
        }
    }

    // 2. 実行時間の回帰判定
    if comparison.TimeRatio >= thresholds.TimeRatioFail {
        reasons = append(reasons, fmt.Sprintf("Execution time increased by %.1f%%", (comparison.TimeRatio-1)*100))
        status = "FAIL"
        severity = "HIGH"
    } else if comparison.TimeRatio >= thresholds.TimeRatioWarn {
        reasons = append(reasons, fmt.Sprintf("Execution time increased by %.1f%%", (comparison.TimeRatio-1)*100))
        status = "WARN"
        severity = "MEDIUM"
    }

    // 3. メモリの回帰判定
    if comparison.MemoryRatio >= thresholds.MemoryRatioFail {
        reasons = append(reasons, fmt.Sprintf("Memory usage increased by %.1f%%", (comparison.MemoryRatio-1)*100))
        if status != "FAIL" {
            status = "FAIL"
            severity = "HIGH"
        }
    } else if comparison.MemoryRatio >= thresholds.MemoryRatioWarn {
        reasons = append(reasons, fmt.Sprintf("Memory usage increased by %.1f%%", (comparison.MemoryRatio-1)*100))
        if status == "" {
            status = "WARN"
            severity = "MEDIUM"
        }
    }

    // 4. 最終判定
    if status == "" {
        status = "PASS"
        severity = "LOW"
    }

    action := determineAction(status, severity, comparison.StatTest.Effect)

    return RegressionDecision{
        Status:     status,
        Reasons:    reasons,
        Severity:   severity,
        Action:     action,
        Confidence: comparison.StatTest.Confidence,
    }
}
```

## 4. CI統合仕様

### 4.1 GitHub Actions ワークフロー
```yaml
name: Performance Regression Check

on:
  pull_request:
    paths:
      - 'internal/filevalidator/encoding/**'
      - 'docs/tasks/0001_performance_regression_testing/**'

env:
  PERFORMANCE_CONFIG: 'docs/tasks/0001_performance_regression_testing/config.yaml'

jobs:
  performance-regression:
    runs-on: ubuntu-latest
    timeout-minutes: 15

    steps:
      - name: Checkout PR branch
        uses: actions/checkout@v4
        with:
          fetch-depth: 0  # 全履歴取得

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.23.10'

      - name: Measure baseline performance
        run: |
          git checkout origin/main
          make performance-test-baseline

      - name: Measure current performance
        run: |
          git checkout ${{ github.head_ref }}
          make performance-test-current

      - name: Compare performance
        run: make performance-compare

      - name: Upload results
        uses: actions/upload-artifact@v4
        with:
          name: performance-results
          path: |
            performance-baseline.json
            performance-current.json
            performance-report.md

      - name: Comment PR
        if: always()
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const reportPath = 'performance-report.md';
            if (fs.existsSync(reportPath)) {
              const report = fs.readFileSync(reportPath, 'utf8');
              github.rest.issues.createComment({
                issue_number: context.issue.number,
                owner: context.repo.owner,
                repo: context.repo.repo,
                body: report
              });
            }

      - name: Fail on regression
        run: |
          if grep -q "FAIL" performance-report.md; then
            echo "Performance regression detected!"
            exit 1
          fi
```

### 4.2 Makefile統合
```makefile
# パフォーマンステスト用ターゲット追加
PERFORMANCE_CONFIG ?= docs/tasks/0001_performance_regression_testing/config.yaml
BASELINE_FILE ?= performance-baseline.json
CURRENT_FILE ?= performance-current.json
REPORT_FILE ?= performance-report.md

performance-test-baseline:
	$(ENVSET) $(GOTEST) -tags performance -v ./internal/filevalidator/encoding/ \
		-args -mode=baseline -output=$(BASELINE_FILE) -config=$(PERFORMANCE_CONFIG)

performance-test-current:
	$(ENVSET) $(GOTEST) -tags performance -v ./internal/filevalidator/encoding/ \
		-args -mode=current -output=$(CURRENT_FILE) -config=$(PERFORMANCE_CONFIG)

performance-compare:
	$(ENVSET) $(GOTEST) -tags performance -v ./internal/filevalidator/encoding/ \
		-args -mode=compare -baseline=$(BASELINE_FILE) -current=$(CURRENT_FILE) -output=$(REPORT_FILE) -config=$(PERFORMANCE_CONFIG)

performance-regression-check: performance-test-baseline performance-test-current performance-compare
	@echo "Performance regression check completed. See $(REPORT_FILE) for results."
```

## 5. エラーハンドリング仕様

### 5.1 エラー分類
```go
type PerformanceTestError struct {
    Type        string    `json:"type"`
    Message     string    `json:"message"`
    Benchmark   string    `json:"benchmark,omitempty"`
    Timestamp   time.Time `json:"timestamp"`
    Recoverable bool      `json:"recoverable"`
    Context     map[string]interface{} `json:"context"`
}

const (
    ErrorTypeTimeout     = "timeout"
    ErrorTypePanic       = "panic"
    ErrorTypeComparison  = "comparison"
    ErrorTypeIO          = "io"
    ErrorTypeConfig      = "config"
    ErrorTypeEnvironment = "environment"
)
```

### 5.2 リカバリー戦略
```go
func runBenchmarkWithRecovery(benchmark BenchmarkDefinition, config MeasurementConfig) (result BenchmarkResult, err error) {
    defer func() {
        if r := recover(); r != nil {
            err = &PerformanceTestError{
                Type:        ErrorTypePanic,
                Message:     fmt.Sprintf("Benchmark panicked: %v", r),
                Benchmark:   benchmark.Name,
                Timestamp:   time.Now(),
                Recoverable: false,
                Context:     map[string]interface{}{"stack": string(debug.Stack())},
            }
        }
    }()

    // タイムアウト制御
    ctx, cancel := context.WithTimeout(context.Background(), config.TimeoutPerBench)
    defer cancel()

    // 実際の測定実行
    return runBenchmarkMeasurement(ctx, benchmark, config)
}
```

## 6. 設定ファイル仕様

### 6.1 完全な設定ファイル例
```yaml
# docs/tasks/0001_performance_regression_testing/config.yaml
measurement:
  warmup_iterations: 10
  measure_iterations: 20
  timeout_per_bench: "2m"
  outlier_threshold: 0.1
  memory_profiling: true

thresholds:
  time_ratio:
    warn: 1.2
    fail: 1.5
  memory_ratio:
    warn: 1.2
    fail: 1.5
  statistical_significance: 0.05

benchmarks:
  - name: "encode_simple_path"
    description: "Simple path encoding performance"
    category: "encoding"
    enabled: true
    timeout: "30s"

  - name: "encode_with_fallback_normal"
    description: "Fallback encoding performance"
    category: "encoding"
    enabled: true
    timeout: "30s"

  - name: "encode_special_chars"
    description: "Special character encoding performance"
    category: "encoding"
    enabled: true
    timeout: "30s"

  - name: "decode_normal"
    description: "Normal decoding performance"
    category: "decoding"
    enabled: true
    timeout: "30s"

reporting:
  include_raw_data: false
  include_environment: true
  include_suggestions: true
  markdown_template: "default"

ci:
  fail_on_regression: true
  post_pr_comment: true
  upload_artifacts: true
  artifact_retention_days: 30
```

この詳細仕様書に基づいて、実装フェーズに進むことができます。
