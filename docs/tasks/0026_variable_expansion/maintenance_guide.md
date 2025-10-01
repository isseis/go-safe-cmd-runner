# メンテナンスガイド: コマンド・引数内環境変数展開機能

## 1. はじめに

### 1.1 目的

本ドキュメントは、go-safe-cmd-runnerの環境変数展開機能の保守・拡張を行う開発者向けに、コードの構造、変更手順、テスト方法、トラブルシューティング手法を提供します。

### 1.2 対象読者

- 機能の保守を担当する開発者
- 機能の拡張やカスタマイズを行う開発者
- バグ修正を行う開発者

### 1.3 前提知識

- Go言語の基本知識（特にエラーハンドリング、インターフェース）
- go-safe-cmd-runnerの基本アーキテクチャ
- TDD（テスト駆動開発）の基本概念

## 2. コードベース概要

### 2.1 ディレクトリ構造

```
internal/runner/
├── environment/
│   ├── processor.go           # VariableExpander実装（約500行）
│   ├── processor_test.go      # 単体テスト（約900行）
│   ├── filter.go              # 環境変数フィルタリング
│   └── errors.go              # エラー定義
├── config/
│   ├── expansion.go           # Config統合（約40行）
│   ├── expansion_test.go      # 統合テスト（約600行）
│   ├── expansion_benchmark_test.go  # ベンチマーク（約200行）
│   └── command.go             # Command構造体
└── security/
    └── validator.go           # セキュリティ検証（キャッシュ化）
```

### 2.2 主要コンポーネント

#### 2.2.1 VariableExpander（processor.go）

**責務**:
- 環境変数の展開処理
- エスケープシーケンスの処理
- 循環参照検出
- セキュリティ検証との統合

**主要メソッド**:
- `ExpandCommandEnv()`: Command.Envの展開
- `ExpandString()`: 単一文字列の展開
- `ExpandStrings()`: 複数文字列の一括展開
- `handleEscapeSequence()`: エスケープシーケンス処理
- `handleVariableExpansion()`: 変数展開処理

**設計上の重要ポイント**:
- 1文字スキャンアルゴリズムによる正確なエスケープ処理
- visited mapによるシンプルな循環参照検出
- SecurityValidatorのキャッシュ化による性能最適化

#### 2.2.2 Config Expansion（expansion.go）

**責務**:
- コマンド全体（Cmd, Args, Env）の展開
- VariableExpanderとConfig Parserの統合

**主要関数**:
- `ExpandCommand()`: コマンド全体の展開

**設計上の重要ポイント**:
- シンプルで明確なインターフェース
- エラーハンドリングの一貫性
- 既存コードへの影響を最小化

#### 2.2.3 Security Validator（validator.go）

**責務**:
- allowlist検証
- 環境変数値のセキュリティ検証
- 検証結果のキャッシュ化

**最適化ポイント**:
- 正規表現の事前コンパイルとキャッシュ
- 繰り返し検証の高速化

## 3. 一般的なメンテナンス作業

### 3.1 新しいエスケープシーケンスの追加

**手順**:

1. **要件の確認**
   - どのエスケープシーケンスが必要か
   - セキュリティ上の問題はないか
   - 既存の`\$`や`\\`との整合性

2. **コードの修正**（processor.go）
   ```go
   func (p *VariableExpander) handleEscapeSequence(inputChars []rune, i int, result *strings.Builder) (int, error) {
       if i+1 >= len(inputChars) {
           return 0, fmt.Errorf("%w at position %d (trailing backslash)", ErrInvalidEscapeSequence, i)
       }
       nextChar := inputChars[i+1]
       switch nextChar {
       case '$', '\\':
           // 既存のエスケープシーケンス
           result.WriteRune(nextChar)
           return i + 2, nil
       case 'n':  // 新しいエスケープシーケンスの例
           result.WriteRune('\n')
           return i + 2, nil
       default:
           return 0, fmt.Errorf("%w at position %d: \\%c", ErrInvalidEscapeSequence, i, nextChar)
       }
   }
   ```

3. **テストの追加**（processor_test.go）
   ```go
   func TestExpandString_NewEscapeSequence(t *testing.T) {
       // テストケースの追加
   }
   ```

4. **ドキュメントの更新**
   - user_guide.md
   - api_specification.md

### 3.2 エラーメッセージの改善

**手順**:

1. **現状のエラーメッセージを確認**
   ```bash
   grep -r "fmt.Errorf" internal/runner/environment/processor.go
   ```

2. **コンテキスト情報の追加**
   ```go
   // 改善前
   return "", fmt.Errorf("variable not found: %s", varName)

   // 改善後
   return "", fmt.Errorf("variable not found: %s (group: %s, command: %s, position: %d)",
       varName, groupName, cmdName, position)
   ```

3. **エラー処理のテスト追加**
   ```go
   func TestExpandString_ImprovedErrorMessage(t *testing.T) {
       // エラーメッセージの内容を検証
   }
   ```

### 3.3 性能最適化

**プロファイリング手順**:

1. **ベンチマークの実行**
   ```bash
   cd internal/runner/config
   go test -bench=BenchmarkVariableExpansion -benchmem -cpuprofile=cpu.prof
   ```

2. **プロファイル結果の分析**
   ```bash
   go tool pprof cpu.prof
   # (pprof) top10
   # (pprof) list ExpandString
   ```

3. **ボトルネックの特定と改善**
   - 文字列連結 → strings.Builderの使用
   - 繰り返し検証 → キャッシュの導入
   - 不要なアロケーション → 構造体の再利用

4. **改善後のベンチマーク**
   ```bash
   go test -bench=BenchmarkVariableExpansion -benchmem
   ```

5. **性能要件の確認**
   - 処理時間: 1ms/要素以下
   - メモリ使用量: 展開前の2倍以下
   - アロケーション数: 最小化

## 4. 変更時の注意点

### 4.1 後方互換性の維持

**チェックリスト**:
- [ ] 既存のTOML設定ファイルが変更なしで動作するか
- [ ] APIシグネチャに破壊的変更がないか
- [ ] エラー型が既存のエラーハンドリングと互換性があるか
- [ ] 性能が劣化していないか

**互換性テスト**:
```bash
# 既存のサンプルファイルでテスト
./build/runner sample/variable_expansion_test.toml
./build/runner sample/output_capture_advanced.toml
```

### 4.2 セキュリティの確認

**セキュリティチェックリスト**:
- [ ] allowlist検証が正しく機能するか
- [ ] コマンドインジェクション攻撃への対策は十分か
- [ ] 情報漏洩のリスクはないか
- [ ] DoS攻撃（循環参照等）への対策は機能しているか

**セキュリティテスト**:
```bash
# セキュリティテストの実行
go test -v -run TestSecurity internal/runner/config
```

### 4.3 エラーハンドリングの一貫性

**ガイドライン**:
1. センチネルエラーを使用（`errors.Is()`で判定可能）
2. 詳細なコンテキスト情報を含める
3. ユーザーフレンドリーなエラーメッセージ
4. ログレベルを適切に設定（Debug, Error等）

**例**:
```go
// 良い例
if varName == "" {
    p.logger.Error("Empty variable name detected", "position", i, "group", groupName)
    return "", fmt.Errorf("%w: empty variable name at position %d", ErrInvalidVariableName, i)
}

// 悪い例
if varName == "" {
    return "", errors.New("error")  // コンテキスト不足、センチネルエラー未使用
}
```

## 5. テスト戦略

### 5.1 テスト実行

**全テストの実行**:
```bash
# 全パッケージのテスト
make test

# 特定パッケージのテスト
go test -v ./internal/runner/environment/
go test -v ./internal/runner/config/
```

**カバレッジの確認**:
```bash
go test -cover ./internal/runner/environment/
go test -cover ./internal/runner/config/

# カバレッジレポートの生成
go test -coverprofile=coverage.out ./internal/runner/environment/
go tool cover -html=coverage.out
```

### 5.2 テストケースの追加

**TDDアプローチ**:
1. **失敗するテストを書く**
   ```go
   func TestExpandString_NewFeature(t *testing.T) {
       expander := createTestExpander(t)
       result, err := expander.ExpandString("${NEW_FEATURE}", env, allowlist, "test", visited)
       require.NoError(t, err)
       assert.Equal(t, "expected_value", result)
   }
   ```

2. **テストを実行して失敗を確認**
   ```bash
   go test -v -run TestExpandString_NewFeature
   ```

3. **実装を追加**
   ```go
   // processor.goに新機能を実装
   ```

4. **テストを再実行して成功を確認**

5. **リファクタリング（必要に応じて）**

### 5.3 ベンチマークの追加

**手順**:
```go
func BenchmarkNewFeature(b *testing.B) {
    expander := createBenchmarkExpander()
    env := map[string]string{"VAR": "value"}
    allowlist := []string{"VAR"}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = expander.ExpandString("${VAR}", env, allowlist, "test", make(map[string]bool))
    }
}
```

**実行**:
```bash
go test -bench=BenchmarkNewFeature -benchmem
```

## 6. トラブルシューティング

### 6.1 よくある問題と解決方法

#### 問題1: テストが失敗する

**症状**:
```
FAIL: TestExpandString_Basic
Expected: "/home/user", Got: "${HOME}"
```

**診断手順**:
1. ログレベルをdebugに設定
2. テスト内でprintfデバッグ
3. 環境変数マップの内容を確認

**解決例**:
```go
t.Logf("env map: %+v", env)
t.Logf("allowlist: %+v", allowlist)
```

#### 問題2: 循環参照が誤検出される

**症状**:
```
Error: circular variable reference detected
But: A=${B}, B=value (no circular reference)
```

**原因**:
visited mapが不適切に共有されている

**解決方法**:
```go
// 各展開で新しいvisited mapを使用
visited := make(map[string]bool)
```

#### 問題3: 性能が劣化している

**診断手順**:
```bash
# ベンチマーク比較
go test -bench=. -benchmem > new.txt
git checkout main
go test -bench=. -benchmem > old.txt
benchcmp old.txt new.txt
```

**よくある原因**:
- 不要な文字列アロケーション
- キャッシュの非効率化
- 繰り返し検証の増加

### 6.2 デバッグ手法

#### ログの活用

```go
// デバッグログの追加
p.logger.Debug("Variable expansion step",
    "input", value,
    "position", i,
    "current_char", string(inputChars[i]),
    "visited", visited,
)
```

#### テストデータの作成

```go
// 最小再現ケースの作成
func TestMinimalReproduction(t *testing.T) {
    expander := createTestExpander(t)

    // 問題を再現する最小ケース
    env := map[string]string{"VAR": "${VAR}"}  // 循環参照
    _, err := expander.ExpandString("${VAR}", env, nil, "test", make(map[string]bool))

    require.Error(t, err)
    assert.ErrorIs(t, err, environment.ErrCircularReference)
}
```

## 7. リリース手順

### 7.1 リリース前チェックリスト

- [ ] 全テストがパス（`make test`）
- [ ] リントエラーなし（`make lint`）
- [ ] ベンチマークで性能要件を満たしている
- [ ] ドキュメントが更新されている
- [ ] CHANGELOGが更新されている
- [ ] 破壊的変更がドキュメント化されている

### 7.2 リリースコマンド

```bash
# 1. テストとリント
make test
make lint

# 2. ビルド確認
make build

# 3. サンプルファイルでの動作確認
./build/runner sample/variable_expansion_test.toml

# 4. バージョンタグの作成（mainブランチで）
git tag -a v1.2.0 -m "Release v1.2.0: Variable expansion feature"
git push origin v1.2.0
```

### 7.3 リリース後の確認

- [ ] CIが成功している
- [ ] ドキュメントサイトが更新されている
- [ ] リリースノートが公開されている

## 8. 参考情報

### 8.1 関連ドキュメント

- [ユーザーガイド](user_guide.md) - エンドユーザー向け
- [API仕様書](api_specification.md) - APIリファレンス
- [アーキテクチャ設計書](02_architecture.md) - システム設計
- [要件定義書](01_requirements.md) - 機能要件

### 8.2 有用なコマンド

```bash
# コードフォーマット
make fmt

# 静的解析
make lint

# 全テスト実行
make test

# カバレッジ確認
go test -cover ./...

# ベンチマーク実行
go test -bench=. -benchmem ./internal/runner/config/

# プロファイリング
go test -cpuprofile=cpu.prof -bench=.
go tool pprof cpu.prof
```

### 8.3 開発環境

**必須ツール**:
- Go 1.23.10以上
- golangci-lint
- gofumpt

**推奨ツール**:
- benchcmp - ベンチマーク比較
- pprof - プロファイリング
- delve - デバッガ

## 9. よくある質問（FAQ）

### Q1: 新しい変数形式（例: `$VAR`）をサポートしたい

**A**: 現在の実装は`${VAR}`形式のみをサポートしており、これには明確な理由があります：
- 変数の境界が明確（`${VAR}_suffix`のような場合に有用）
- エスケープ処理がシンプル
- セキュリティリスクが低い

新形式の追加は慎重に検討し、セキュリティレビューを実施してください。

### Q2: 展開深度の制限を変更したい

**A**: 現在はvisited mapによる循環参照検出により、展開深度に制限はありません。深度制限を追加する場合：
1. 要件を明確化（なぜ制限が必要か）
2. 設定可能なパラメータとして実装
3. 適切なエラーメッセージを提供

### Q3: 性能要件を満たせない場合は？

**A**: 以下の手順で対処：
1. プロファイリングでボトルネックを特定
2. 不要なアロケーションを削減
3. キャッシュの効率化
4. 必要に応じてアルゴリズムを見直し

それでも改善しない場合は、要件の見直しを検討してください。

## 10. 変更履歴

| バージョン | 日付 | 変更内容 | 担当者 |
|----------|------|---------|--------|
| 1.0.0 | 2025-10-01 | 初版作成 | - |

## 11. 連絡先・サポート

問題や質問がある場合：
1. GitHubのissueを作成
2. プロジェクトのドキュメントを確認
3. コードコメントを参照
