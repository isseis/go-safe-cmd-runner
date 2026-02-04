# ELF 動的シンボル解析 エッジケース対応

## 1. 実行専用権限バイナリの問題

### 1.1 問題の詳細

Linux では、バイナリに実行権限のみ（`--x--x--x`、octal `0111`）を設定した場合：

- **実行は可能**: カーネルがバイナリを実行できる
- **読み取り不可**: ELF 解析に必要な読み取りアクセスができない

```bash
# 実験で確認
$ chmod 111 /tmp/test_binary
$ /tmp/test_binary              # 実行可能
test
$ cat /tmp/test_binary          # 読み取り不可
cat: /tmp/test_binary: Permission denied
```

### 1.2 影響範囲

ELF `.dynsym` 解析には `os.O_RDONLY` でファイルを開く必要があるため、実行専用権限のバイナリに対しては以下の動作になる：

1. `SafeOpenFile(path, os.O_RDONLY, 0)` が `Permission denied` で失敗
2. `AnalysisResult` = `AnalysisError` を返す
3. Security by Default の原則により、ネットワーク操作の可能性ありとして扱われる（Middle Risk）

### 1.3 実世界での発生頻度

**非常にまれ**:

- 通常のバイナリは読み取り可能（少なくとも所有者に対して）
- 標準的なパーミッション: `0755` (rwxr-xr-x)、`0775` (rwxrwxr-x)
- セキュリティ強化された環境でも、実行専用権限は推奨されない
  - デバッグが困難
  - ツール（ldd、file、readelf など）が機能しない
  - システム管理が複雑になる

**発生する可能性のあるケース**:
- 特殊なセキュリティ要件を持つカスタム環境
- 機密性の高い商用バイナリの保護（極めてまれ）

## 2. 対処方針

### 2.1 採用しない対処法：特権昇格

**検討事項**: `CAP_DAC_READ_SEARCH` capability を使った読み取り権限の一時的取得

**採用しない理由**:

1. **セキュリティリスク**:
   - Runner は通常、非特権ユーザーで実行される設計
   - Capability の付与は攻撃対象領域を増やす
   - 権限昇格のバグが重大な脆弱性につながる

2. **複雑性の増加**:
   - `libcap` への依存が必要
   - プラットフォーム固有の実装が必要
   - テストの複雑性が大幅に増加

3. **実用上の必要性が低い**:
   - 実行専用バイナリは極めてまれ
   - Security by Default で適切にハンドリング可能

### 2.2 採用する対処法：現状維持 + 明示的な文書化

**方針**:

1. **現在の動作を維持**:
   - 読み取り不可のバイナリは `AnalysisError` として扱う
   - Middle Risk 判定により、ユーザーに確認を促す

2. **ログメッセージの明確化**:
   ```go
   slog.Warn("ELF analysis failed due to insufficient read permissions",
       "command", cmdName,
       "path", cmdPath,
       "error", "permission denied",
       "reason", "Binary is executable but not readable; treating as potential network operation for safety",
       "suggestion", "Consider adding read permissions for the executing user")
   ```

3. **ドキュメントでの説明**:
   - README およびセキュリティドキュメントに記載
   - 実行専用バイナリの制限事項を明示
   - 推奨されるパーミッション設定を提供

### 2.3 ユーザーガイダンス

**推奨パーミッション**:
- 標準バイナリ: `0755` (rwxr-xr-x)
- グループ実行可能: `0775` (rwxrwxr-x)
- セキュリティ強化: `0750` (rwxr-x---) - 読み取り権限を維持

**トラブルシューティング**:
```bash
# 問題: ELF analysis failed due to insufficient read permissions
# 解決方法:
chmod +r /path/to/binary  # 読み取り権限を追加
# または
chmod 755 /path/to/binary  # 標準的な権限に設定
```

## 3. 実装への影響

### 3.1 コード変更

**不要**: 既存の `SafeOpenFile` + `AnalysisError` ハンドリングで適切に対処される

### 3.2 ログメッセージの改善

`analyzer_impl.go` のエラーハンドリングで、Permission denied エラーを特定して明確なメッセージを提供：

```go
// AnalyzeNetworkSymbols implements ELFAnalyzer interface.
func (a *StandardELFAnalyzer) AnalyzeNetworkSymbols(path string) AnalysisOutput {
    file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        // Check if it's a permission error
        if errors.Is(err, os.ErrPermission) {
            return AnalysisOutput{
                Result: AnalysisError,
                Error:  fmt.Errorf("insufficient read permissions (executable-only binary?): %w", err),
            }
        }
        return AnalysisOutput{
            Result: AnalysisError,
            Error:  fmt.Errorf("failed to open file: %w", err),
        }
    }
    defer file.Close()
    // ... 以降の処理
}
```

### 3.3 統合部分（command_analysis.go）のログ改善

```go
case elfanalyzer.AnalysisError:
    // Check if it's a permission error
    errMsg := output.Error.Error()
    if strings.Contains(errMsg, "insufficient read permissions") {
        slog.Warn("ELF analysis failed: binary is executable but not readable",
            "command", cmdName,
            "path", cmdPath,
            "error", output.Error,
            "action", "treating as potential network operation for safety",
            "suggestion", "add read permissions to the binary for accurate analysis")
    } else {
        slog.Warn("ELF analysis failed, treating as potential network operation",
            "command", cmdName,
            "path", cmdPath,
            "error", output.Error,
            "reason", "Unable to determine network capability, assuming middle risk for safety")
    }
    return true, true
```

## 4. テストケース

### 4.1 ユニットテスト追加

`analyzer_test.go` にテストケースを追加：

```go
func TestStandardELFAnalyzer_ExecuteOnlyBinary(t *testing.T) {
    testdataDir := "testdata"
    if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
        t.Skip("testdata directory not found")
    }

    // Create a binary with execute-only permissions
    execOnlyPath := filepath.Join(testdataDir, "exec_only.elf")

    // Copy from a working binary
    sourcePath := filepath.Join(testdataDir, "with_socket.elf")
    if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
        t.Skip("source binary not found")
    }

    input, err := os.ReadFile(sourcePath)
    require.NoError(t, err)

    err = os.WriteFile(execOnlyPath, input, 0111) // --x--x--x
    require.NoError(t, err)
    defer os.Remove(execOnlyPath)

    analyzer := NewStandardELFAnalyzer(nil)
    output := analyzer.AnalyzeNetworkSymbols(execOnlyPath)

    // Should return AnalysisError due to permission denied
    assert.Equal(t, AnalysisError, output.Result)
    assert.NotNil(t, output.Error)
    assert.Contains(t, output.Error.Error(), "permission")
}
```

## 5. ドキュメント更新

### 5.1 README への追記

セキュリティセクションに以下を追加：

```markdown
### Binary Permission Requirements

For accurate network operation detection via ELF analysis:

- **Minimum required**: Execute permission (`--x------`)
- **Recommended**: Read + Execute permission (`r-x------` or better)
- **Standard**: `755` (rwxr-xr-x) for system binaries

**Note**: Binaries with execute-only permissions (e.g., `111`) can run but
cannot be analyzed. Such binaries will be treated as potentially performing
network operations for safety.
```

### 5.2 セキュリティドキュメントへの追記

`docs/dev/security-architecture.md` の ELF 解析セクションに制限事項を明記。

## 6. まとめ

### 6.1 決定事項

- ✅ **現状の動作を維持**: 特権昇格は実装しない
- ✅ **エラーメッセージを改善**: Permission denied を明確に識別
- ✅ **ドキュメントを充実**: 制限事項と推奨設定を明記
- ✅ **テストケースを追加**: 実行専用バイナリのケースをカバー

### 6.2 Security by Default の原則の維持

実行専用バイナリは以下の理由で安全側に倒す：

1. **極めてまれな状況**: 通常の運用では発生しない
2. **Middle Risk 判定**: ユーザーに明示的な確認を求める
3. **明確なガイダンス**: 問題解決方法を提供
4. **セキュリティ優先**: 不確実な場合は安全側に倒す

この方針により、セキュリティを損なわず、実用上の問題も最小化される。
