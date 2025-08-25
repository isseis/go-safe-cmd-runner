# 要件定義書: セキュリティ検証メカニズムの統一

## 1. 背景・課題

### 1.1 現在の状況
- **PathResolver.ValidateCommand**: 特定ディレクトリ（`/bin/*`, `/usr/bin/*` 等）のみ実行許可
- **リスクベース評価**: `AnalyzeCommandSecurity` でコマンドの危険度を評価
- **二重検証**: パス制限とリスク評価が別々に動作

### 1.2 課題
- **柔軟性の欠如**: ディレクトリ制限により任意の場所のツールが使用不可
- **セキュリティ基準の分散**: パス許可とリスク評価で異なる判定基準
- **同名ファイル誤実行リスク**: PATH上の同名コマンドを間違って実行する可能性

## 2. 要件概要

### 2.1 基本要件
**R1**: パス制限を撤廃し、ハッシュ値ベース検証とリスクベース評価に統合する

**R2**: `AnalyzeCommandSecurity` でディレクトリベースのデフォルトリスク判定とハッシュ検証制御を実装

**R3**: 標準ディレクトリでのハッシュ検証スキップ機能を提供

### 2.2 機能要件

#### 2.2.1 ディレクトリベースリスク判定
**F1**: 標準ディレクトリのデフォルトリスクレベル設定：
- `/bin/*`, `/usr/bin/*`, `/usr/local/bin/*`: RiskLevelLow
- `/sbin/*`, `/usr/sbin/*`, `/usr/local/sbin/*`: RiskLevelMedium
- その他ディレクトリ: 個別パターン分析によるリスク判定

**F2**: 個別コマンドのリスクレベル上書き機能（コード中で設定）

#### 2.2.2 ハッシュ検証統合
**F3**: ハッシュ検証の条件分岐：
- `skip_standard_paths=true` + 標準ディレクトリ: ハッシュ検証スキップ
- `skip_standard_paths=false` または 非標準ディレクトリ: ハッシュ検証必須

**F4**: ハッシュ検証失敗時は RiskLevelCritical として扱う

#### 2.2.3 PathResolver最大限簡素化
**F5**: PathResolverを純粋なパス解決コンポーネントに最大限簡素化
- ValidateCommandメソッドを完全削除
- securityフィールドを完全削除
- 正規表現ベースのパス制限を完全撤廃

### 2.3 非機能要件

**NF1**: 既存APIインターフェースは変更しない（内部実装のみ変更）

**NF2**: TOML設定ファイルはリリース時に一括更新（互換性レイヤーなし）

**NF3**: ハッシュ検証スキップによる性能向上を実現

## 3. 実装概要

### 3.1 AnalyzeCommandSecurityの拡張
```go
func AnalyzeCommandSecurity(resolvedPath string, args []string) (riskLevel, pattern, reason, error) {
    // 1. ディレクトリベースのデフォルトリスク判定
    defaultRisk := getDefaultRiskByDirectory(resolvedPath)

    // 2. ハッシュ検証の要否判定と実行
    if shouldSkipHashValidation(resolvedPath) {
        // 標準ディレクトリ + skip_standard_paths=true の場合
    } else {
        if err := validateFileHash(resolvedPath); err != nil {
            return RiskLevelCritical, "hash_validation_failed", err.Error(), nil
        }
    }

    // 3. 既存のパターンベースリスク分析
    // 4. 個別オーバーライド適用
    return finalRiskLevel, pattern, reason, nil
}
```

### 3.2 設定項目
```toml
[security]
skip_standard_paths = false  # デフォルト: 全ファイルでハッシュ検証
```

## 4. 想定されるエラーメッセージ

### 4.1 ハッシュ検証失敗
```
Error: hash_validation_failed - File integrity check failed
Details:
  Command: /home/user/custom-tool
  Expected Hash: abc123...
  Actual Hash: def456...
  Suggestion: Verify file integrity or update hash manifest
```

### 4.2 リスクレベル超過
```
Error: command_risk_exceeded - Command risk level too high
Details:
  Command: /usr/bin/sudo
  Detected Risk Level: CRITICAL
  Max Allowed Risk Level: MEDIUM
  Suggestion: Adjust max_risk_level in command configuration
```

## 5. 実装対象

### 5.1 修正ファイル
- `internal/verification/path_resolver.go`: 大幅簡素化（securityフィールド削除、基本検証のみ）
- `internal/runner/security/command_analysis.go`: ディレクトリ判定とハッシュ検証統合
- `internal/runner/security/types.go`: skip_standard_paths設定追加
- `internal/verification/manager.go`: PathResolver呼び出し部分の調整
- 関連テストファイル

## 6. 受け入れ条件

- [ ] 任意ディレクトリからのコマンド実行が可能
- [ ] skip_standard_paths=trueで標準ディレクトリのハッシュ検証がスキップされる
- [ ] skip_standard_paths=falseで全ファイルのハッシュ検証が実行される
- [ ] ディレクトリベースのデフォルトリスクレベルが正しく適用される
- [ ] ハッシュ検証失敗時にコマンドがブロックされる
- [ ] 個別コマンドのリスクレベル上書きが機能する

## 7. 標準ディレクトリ定義

```go
// ハードコーディングされた標準ディレクトリ
var StandardDirectories = []string{
    "/bin",
    "/usr/bin",
    "/sbin",
    "/usr/sbin",
    "/usr/local/bin",
    "/usr/local/sbin",
}

// デフォルトリスクレベル
var DefaultRiskLevels = map[string]runnertypes.RiskLevel{
    "/bin":             runnertypes.RiskLevelLow,
    "/usr/bin":         runnertypes.RiskLevelLow,
    "/usr/local/bin":   runnertypes.RiskLevelLow,
    "/sbin":            runnertypes.RiskLevelMedium,
    "/usr/sbin":        runnertypes.RiskLevelMedium,
    "/usr/local/sbin":  runnertypes.RiskLevelMedium,
}
```

この仕様により、パス制限を撤廃しながらハッシュ検証とリスクベース評価で包括的なセキュリティを実現します。
