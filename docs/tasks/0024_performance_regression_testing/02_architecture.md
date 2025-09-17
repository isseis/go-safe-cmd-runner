# パフォーマンス回帰テストシステム アーキテクチャ設計書

## 1. システム概要

### 1.1 アーキテクチャ原則
- **相対比較**: 同一環境でのPR前後比較により環境依存性を排除
- **統計的信頼性**: 複数回測定による統計処理で測定精度を向上
- **自動化**: CI/CDパイプラインとの完全統合
- **拡張性**: 新しいベンチマーク追加が容易な設計

### 1.2 全体構成図

```
┌─────────────────────────────────────────────────────────────────┐
│                     GitHub Actions CI                          │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐│
│  │  Baseline測定   │───▶│  Current測定    │───▶│  比較・レポート ││
│  │  (main branch)  │    │  (PR branch)    │    │  (regression)   ││
│  └─────────────────┘    └─────────────────┘    └─────────────────┘│
│           │                       │                       │       │
│           ▼                       ▼                       ▼       │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐│
│  │baseline.json    │    │current.json     │    │report.md        ││
│  └─────────────────┘    └─────────────────┘    └─────────────────┘│
└─────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
                          ┌─────────────────┐
                          │   PR Comment    │
                          └─────────────────┘
```

## 2. コンポーネント設計

### 2.1 測定エンジン (Measurement Engine)

#### 2.1.1 役割
- パフォーマンステストの実行
- 測定データの収集・統計処理
- 結果のJSON出力

#### 2.1.2 内部構造
```go
type MeasurementEngine struct {
    benchmarks []Benchmark
    iterations int
    warmup     int
}

type Benchmark struct {
    Name      string
    Operation func() error
    Category  string
}

type MeasurementResult struct {
    Metadata    ResultMetadata       `json:"metadata"`
    Benchmarks  map[string]BenchData `json:"benchmarks"`
}

type BenchData struct {
    Measurements    []int64           `json:"measurements_ns"`
    MedianNs        int64             `json:"median_ns"`
    P95Ns           int64             `json:"p95_ns"`
    AllocatedBytes  int64             `json:"allocated_bytes"`
    Environment     EnvironmentInfo   `json:"environment"`
}
```

#### 2.1.3 統計処理
- **ウォームアップ**: 10回実行で初期化処理の影響排除
- **本測定**: 20回実行で統計的に安定したデータ収集
- **外れ値除去**: 上下10%の値を除外
- **代表値算出**: 中央値、95パーセンタイル値を算出

### 2.2 比較エンジン (Comparison Engine)

#### 2.2.1 役割
- ベースラインと現在の測定結果比較
- 回帰判定ロジックの実行
- 比較結果レポートの生成

#### 2.2.2 内部構造
```go
type ComparisonEngine struct {
    thresholds map[string]float64
    config     ComparisonConfig
}

type ComparisonResult struct {
    Overall    OverallResult            `json:"overall"`
    Benchmarks map[string]BenchComparison `json:"benchmarks"`
    Summary    ComparisonSummary        `json:"summary"`
}

type BenchComparison struct {
    Baseline       BenchData    `json:"baseline"`
    Current        BenchData    `json:"current"`
    TimeRatio      float64      `json:"time_ratio"`
    MemoryRatio    float64      `json:"memory_ratio"`
    Status         string       `json:"status"` // PASS, WARN, FAIL
    Message        string       `json:"message"`
}
```

#### 2.2.3 判定ロジック
- **PASS**: 実行時間比率 < 1.2倍 かつ メモリ比率 < 1.2倍
- **WARN**: 実行時間比率 < 1.5倍 かつ メモリ比率 < 1.5倍
- **FAIL**: 実行時間比率 ≥ 1.5倍 または メモリ比率 ≥ 1.5倍

### 2.3 レポートエンジン (Report Engine)

#### 2.3.1 役割
- Markdown形式のレポート生成
- PRコメント用フォーマット
- 開発者向けの分かりやすい表示

#### 2.3.2 レポート構造
```markdown
# 🚀 Performance Regression Report

## Summary
- **Overall Status**: PASS/WARN/FAIL
- **Total Benchmarks**: N
- **Regressions Detected**: N

## Benchmark Results
| Benchmark | Baseline | Current | Ratio | Status |
|-----------|----------|---------|-------|--------|
| encode_simple_path | 201ns | 245ns | 1.22x | ⚠️ WARN |

## Details
[詳細な分析結果]
```

### 2.4 CI統合レイヤー (CI Integration Layer)

#### 2.4.1 役割
- GitHub Actionsとの統合
- ブランチ切り替え制御
- エラーハンドリング

#### 2.4.2 ワークフロー
```yaml
name: Performance Regression Check
on:
  pull_request:
    paths: ['internal/filevalidator/encoding/**']

jobs:
  performance-check:
    steps:
      - name: Checkout
      - name: Setup Go
      - name: Measure Baseline (main)
      - name: Measure Current (PR)
      - name: Compare Results
      - name: Post PR Comment
```

## 3. データフロー設計

### 3.1 測定フェーズ
```
入力: テスト対象コード
  ↓
[ウォームアップ実行] × 10回
  ↓
[本測定] × 20回
  ↓
[統計処理] → 中央値、P95算出
  ↓
[メモリ測定] → 割り当て量計測
  ↓
出力: JSON測定結果
```

### 3.2 比較フェーズ
```
入力: baseline.json + current.json
  ↓
[データ読み込み・検証]
  ↓
[ベンチマーク毎比較]
  ↓
[閾値判定]
  ↓
[総合判定]
  ↓
出力: 比較結果 + Markdownレポート
```

## 4. 設定管理

### 4.1 設定ファイル構造
```yaml
# performance-config.yaml
measurement:
  iterations: 20
  warmup: 10
  timeout: "5m"

thresholds:
  time_ratio:
    warn: 1.2
    fail: 1.5
  memory_ratio:
    warn: 1.2
    fail: 1.5

benchmarks:
  - name: "encode_simple_path"
    category: "encoding"
    enabled: true
  - name: "encode_with_fallback_normal"
    category: "encoding"
    enabled: true
```

### 4.2 環境別設定
- **ローカル環境**: 詳細ログ、長時間測定
- **CI環境**: 簡潔ログ、時間制限
- **本番環境**: エラー時即座停止

## 5. エラーハンドリング

### 5.1 測定エラー
- **症状**: ベンチマーク実行失敗
- **対策**: リトライ機能、フォールバック測定

### 5.2 比較エラー
- **症状**: ベースラインファイル不正
- **対策**: データ検証、エラーレポート

### 5.3 CI環境エラー
- **症状**: ブランチ切り替え失敗
- **対策**: 権限確認、詳細ログ出力

## 6. セキュリティ考慮事項

### 6.1 データ保護
- 測定データに機密情報が含まれないことを保証
- 一時ファイルの適切な削除

### 6.2 権限管理
- mainブランチへの読み取り専用アクセス
- PRコメント投稿権限の制限

## 7. パフォーマンス考慮事項

### 7.1 測定精度 vs 実行時間
- 測定回数: 精度とCI時間のバランス
- タイムアウト: 異常な長時間実行の防止

### 7.2 リソース使用量
- メモリ使用量: 大量測定データの効率的処理
- CPU使用量: CI環境での他ジョブへの影響最小化

## 8. 将来拡張性

### 8.1 他パッケージ対応
- プラグイン機構による新パッケージ追加
- 共通測定インフラの再利用

### 8.2 高度な分析機能
- 性能トレンド分析
- 回帰原因の自動分析
- 最適化提案機能
