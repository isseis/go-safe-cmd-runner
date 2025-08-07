# 権限昇格期間最小化 - アーキテクチャー設計書

## 1. アーキテクチャー概要

### 1.1 設計原則

本設計は以下の原則に基づいて構築される：

- **最小権限の原則**: 権限昇格は技術的に必要な最小期間のみ実行
- **API分離**: ファイルオープンとファイル検証処理を独立したAPIとして提供
- **例外安全性**: 例外発生時も適切に権限復元を実行
- **API互換性**: 既存のインターフェースを維持

### 1.2 システム構成概要

```
┌─────────────────────────────────────────────────────────────┐
│                    アプリケーション層                        │
│  Validator.Record()                                        │
│  Validator.Verify()                                        │
│                                                             │
│  新規: OpenFileWithPrivileges()                            │
│  新規: VerifyFromHandle()                                  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    FileValidator                            │
│    • 既存: ファイル検証ロジック                              │
│    • 新規: 権限付きファイルオープン                          │
│    • 新規: ファイルハンドルからの検証                        │
└─────────────────────────────────────────────────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          ▼                   ▼                   ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│  フェーズ1:      │ │  フェーズ2:      │ │  フェーズ3:      │
│  ファイル        │ │  ファイル        │ │  ハッシュ        │
│  オープン        │ │  読み取り        │ │  計算・検証      │
│                 │ │                 │ │                 │
│ [要権限昇格]     │ │ [一般権限]       │ │ [一般権限]       │
└─────────────────┘ └─────────────────┘ └─────────────────┘
```

## 2. API設計

### 2.1 新規API: OpenFileWithPrivileges

権限昇格を伴うファイルオープン専用のAPI。

#### 2.1.1 責務
- 一時的な権限昇格でのファイルオープン
- ファイルオープン完了後の即座の権限復元
- ファイルハンドルの安全な返却
- 権限操作のログ記録

#### 2.1.2 インターフェース設計

```go
// OpenFileWithPrivileges opens a file with elevated privileges and immediately restores them
func OpenFileWithPrivileges(filepath string) (*os.File, error)
```

#### 2.1.3 処理フロー

```
1. 現在のUID/GIDを保存
2. 権限昇格 (setuid to root)
3. ファイルオープン
4. 権限復元 (setuid to original)
5. ファイルハンドル返却
```

### 2.2 新規API: VerifyFromHandle

ファイルハンドルからのハッシュ検証専用のAPI。

#### 2.2.1 責務
- ファイルハンドルからの内容読み取り
- ハッシュ計算
- 記録されたハッシュとの比較
- 一般権限での実行

#### 2.2.2 インターフェース設計

```go
// VerifyFromHandle verifies a file's hash using an already opened file handle
func (v *Validator) VerifyFromHandle(file *os.File, targetPath string) error
```

### 2.3 既存API統合

#### 2.3.1 Validator.Verify() 改修

既存の `Verify()` メソッドを内部的に分離された処理に変更：

```go
func (v *Validator) Verify(filePath string) error {
    // 権限が必要な場合の処理例
    if needsPrivileges(filePath) {
        file, err := OpenFileWithPrivileges(filePath)
        if err != nil {
            return err
        }
        defer file.Close()

        return v.VerifyFromHandle(file, filePath)
    }

    // 通常の処理（既存ロジック）
    return v.verifyNormally(filePath)
}
```

## 3. データフロー設計

### 3.1 分離されたAPI使用パターン

```
[アプリケーション]
        │
        ▼ OpenFileWithPrivileges(filepath)
[OpenFileWithPrivileges]
        │
        ├─ [権限昇格] syscall.Seteuid(0)
        ├─ os.Open(filepath)
        ├─ [権限復元] syscall.Seteuid(originalUID)
        └─ return file handle
        │
        ▼ file handle
[アプリケーション]
        │
        ▼ VerifyFromHandle(file, targetPath)
[Validator.VerifyFromHandle]
        │
        ├─ [一般権限] io.ReadAll(file)
        ├─ [一般権限] algorithm.Sum(content)
        ├─ [一般権限] read expected hash
        ├─ [一般権限] compare hashes
        └─ return result
```

### 3.2 統合されたAPI使用パターン

```
[アプリケーション]
        │
        ▼ Verify(filepath)
[Validator.Verify]
        │
        ├─ needsPrivileges(filepath) ?
        │   │
        │   ├─ Yes: OpenFileWithPrivileges(filepath)
        │   │     │ [権限昇格 + ファイルオープン + 権限復元]
        │   │     ▼ file handle
        │   │   VerifyFromHandle(file, filepath)
        │   │     │ [一般権限でハッシュ検証]
        │   │     ▼ result
        │   │
        │   └─ No: 既存ロジック
        │         │ [一般権限で全処理]
        │         ▼ result
        │
        └─ return result
```

### 3.3 エラー処理フロー

```
[エラー発生時]
        │
        ▼ error detected
[OpenFileWithPrivileges]
        │
        ├─ 権限昇格エラー:
        │   └─ return error (権限復元不要)
        │
        ├─ ファイルオープンエラー:
        │   ├─ 権限復元実行
        │   └─ return error
        │
        └─ 権限復元エラー:
            ├─ ログ記録
            ├─ 緊急処理
            └─ return error

[VerifyFromHandle]
        │
        ├─ ファイル読み取りエラー:
        │   └─ return error
        │
        └─ ハッシュ不一致:
            └─ return ErrMismatch
```

## 4. セキュリティ設計

### 4.1 権限分離

#### 4.1.1 権限昇格フェーズ（最小化）
- **対象**: `OpenFileWithPrivileges()` 内のファイルオープンのみ
- **権限**: root（uid=0）
- **期間**: ファイルオープン完了まで（数マイクロ秒）

#### 4.1.2 一般権限フェーズ（拡張）
- **対象**: `VerifyFromHandle()` 内のファイル読み取り・ハッシュ計算
- **権限**: 元のユーザー権限
- **期間**: 処理の大部分（数ミリ秒～数秒）

### 4.2 攻撃面削減効果

```
現在: [権限昇格]→[ファイル処理全体]→[権限復元]
改善: [権限昇格]→[ファイルオープン]→[権限復元] + [一般権限でファイル処理]

権限昇格期間: 100% → 5-10%
```

### 4.3 安全性機構

- **defer による権限復元保証**
- **パニック時の権限復元**
- **権限復元失敗時のログ記録**

## 5. パフォーマンス設計

### 5.1 オーバーヘッド

- **権限昇格・復元**: 追加 ~2μs
- **API分離**: 追加 ~0.1μs
- **総合影響**: <1% 増加

### 5.2 メモリ影響

- **追加メモリ**: 制御情報のみ（~50 bytes）
- **既存処理**: 変更なし

## 6. 拡張性

### 6.1 API拡張可能性

```go
// 将来の拡張例
func OpenFileWithPrivilegesContext(ctx context.Context, filepath string) (*os.File, error)
func (v *Validator) VerifyFromHandleWithOptions(file *os.File, targetPath string, opts *VerifyOptions) error
```

### 6.2 監査機能拡張

- 権限操作の詳細ログ
- セキュリティイベント記録
- メトリクス収集対応

## 7. 実装計画

### 7.1 Phase 1: 基本API実装
1. `OpenFileWithPrivileges()` 関数の実装
2. `Validator.VerifyFromHandle()` メソッドの実装
3. 基本的なエラーハンドリング
4. 単体テスト

### 7.2 Phase 2: 統合と安全性
1. 既存 `Verify()` メソッドの統合
2. 権限復元の安全性強化
3. 統合テスト
4. セキュリティテスト

### 7.3 Phase 3: 最適化
1. パフォーマンス最適化
2. ログ機能強化
3. 監査機能追加

このシンプルなAPI分離により、権限昇格期間を最小化し、セキュリティリスクを大幅に削減できます。
