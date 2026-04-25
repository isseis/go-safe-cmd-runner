# 実装計画書: ディレクトリセキュリティ検証における信頼済みグループホワイトリスト

## 1. 実装概要

### 1.1 目的

要件定義書・アーキテクチャ設計書・詳細仕様書に基づき、グループ書き込み可能ディレクトリの安全判定を「root:root 固定」から「root 所有かつ信頼済みグループ」へ拡張する。

### 1.2 実装原則

1. 最小変更: `validateGroupWritePermissions` の判定置換を中心に、変更範囲を限定する
2. プラットフォーム分離: 信頼済み GID 判定は build tag ファイルに閉じ込める
3. 後方互換: GID 0 は全 OS で常に信頼済みとし既存挙動を維持する
4. セキュリティ優先: `others` 書き込み拒否と非信頼グループ拒否を維持する
5. 検証優先: AC 単位のテストを先に配置し、最後に `make test` / `make lint` で回帰確認する

### 1.3 参照ドキュメント

- 仕様整合の基準: `01_requirements.md`
- 設計整合の基準: `02_architecture.md`
- 実装詳細と AC 対応の基準: `03_detailed_specification.md`

## 2. 実装スコープ

### 2.1 新規作成ファイル

- `internal/runner/security/trusted_gids_darwin.go`
- `internal/runner/security/trusted_gids_linux.go`
- `internal/runner/security/trusted_gids_other.go`

### 2.2 変更対象ファイル

- `internal/runner/runnertypes/spec.go`
- `internal/runner/runnertypes/spec_test.go`
- `internal/runner/security/types.go`
- `internal/runner/security/file_validation.go`
- `internal/runner/security/file_validation_test.go`
- `internal/runner/security/trusted_gids_darwin_test.go`（新規想定）
- `internal/runner/security/trusted_gids_linux_test.go`（新規想定）
- `internal/runner/security/trusted_gids_other_test.go`（新規想定）
- `internal/runner/runner.go`

## 3. 実装フェーズ

### Phase 1: 設定スキーマ拡張

対象:
- `internal/runner/runnertypes/spec.go`
- `internal/runner/runnertypes/spec_test.go`

作業内容:
- [x] `SecuritySpec` を追加し `trusted_gids` を定義する
- [x] `ConfigSpec` に `Security SecuritySpec` を追加する
- [x] TOML パーステストを追加し、`[security].trusted_gids` の読込を検証する
- [x] `security` セクション省略時の後方互換をテストで確認する

成功条件:
- `trusted_gids` が `[]uint32` として正しくパースされる
- 既存設定でパース結果が変化しない

推定工数: 0.5日

実績: 完了

### Phase 2: セキュリティ設定モデル拡張

対象:
- `internal/runner/security/types.go`
- `internal/runner/runner.go`

作業内容:
- [x] `security.Config` に `TrustedGIDs []uint32` を追加する
- [x] `runner.go` で `configSpec.Security.TrustedGIDs` を `securityConfig.TrustedGIDs` へ転写する
- [x] コメントで macOS では値が無視されることを明記する

成功条件:
- `NewRunner` 経路で `TrustedGIDs` が validator 構築時に伝播される
- OS 分岐なしで転写が行われる

推定工数: 0.5日

実績: 完了

### Phase 3: プラットフォーム別信頼済みグループ判定実装

対象:
- `internal/runner/security/trusted_gids_darwin.go`
- `internal/runner/security/trusted_gids_linux.go`
- `internal/runner/security/trusted_gids_other.go`

作業内容:
- [x] darwin 向けに `defaultTrustedGIDs={0,80}` と `isTrustedGroup` を実装する
- [x] linux 向けに `defaultTrustedGIDs={0}` と `isTrustedGroup(default + config)` を実装する
- [x] other 向けに `defaultTrustedGIDs={0}` と `isTrustedGroup(default + config)` を実装する
- [x] `file_validation.go` から platform 依存判定を排除できるシグネチャを揃える

成功条件:
- macOS は config を無視し固定 whitelist のみで判定する
- Linux/other は default と config の和集合で判定する

推定工数: 0.5日

実績: 完了

### Phase 4: ディレクトリ権限判定の置換

対象:
- `internal/runner/security/file_validation.go`

作業内容:
- [x] `isRootOwned` 判定を `isTrustedOwnership` 判定に置換する
- [x] `uid==0 && isTrustedGroup(gid)` の場合に早期 return する
- [x] 信頼済み所有権で許可した場合の debug ログを追加する
- [x] 既存の `CanUserSafelyWriteFile` 経路とエラー型を維持する

成功条件:
- root:root は従来通り許可される
- root:admin(macOS) など信頼済みグループのみ許可される
- 非信頼グループは従来通り拒否される

推定工数: 0.5日

実績: 完了

### Phase 5: テスト実装と受け入れ基準検証

対象:
- `internal/runner/security/file_validation_test.go`
- `internal/runner/runnertypes/spec_test.go`
- `internal/runner/security/trusted_gids_darwin_test.go`
- `internal/runner/security/trusted_gids_linux_test.go`
- `internal/runner/security/trusted_gids_other_test.go`
- 必要に応じて `internal/runner/runner_security_test.go` または `internal/runner/runner_test.go`

作業内容:
- [ ] AC-1: macOS で uid=0,gid=80,perm=0775 が許可されるテストを追加
- [ ] AC-3: 非信頼 GID の group-write が拒否されるテストを追加
- [ ] AC-4: uid=0,gid=0 の許可継続をテストで明示
- [ ] AC-5: Linux で `trusted_gids` 指定有無の差分をテスト追加
- [ ] `trusted_gids` TOML 読込テストで設定経路を確認
- [ ] `isTrustedGroup` のプラットフォーム別ユニットテストを追加する
- [ ] macOS で `Config.TrustedGIDs` が無視されることをテストで固定化する
- [ ] Linux/other で `default + config` の和集合判定をテストで固定化する
- [ ] 既存 AC-2 (`others` 書き込み拒否) に影響がないことを回帰で確認

成功条件:
- AC-1〜AC-6 を満たすテストが存在し、成功する
- 変更範囲外の既存テストが回帰しない

推定工数: 1.0日

実績: 未着手

### Phase 6: 回帰確認と文書整合チェック

対象:
- 変更済み `.go` / `_test.go` 一式
- `docs/tasks/0108_trusted_group_whitelist/03_detailed_specification.md`

作業内容:
- [ ] `make fmt` を実行する
- [ ] `make test` を実行する
- [ ] `make lint` を実行する
- [ ] 詳細仕様書の AC 対応テスト記述と実装が一致するか確認する
- [ ] 実装計画書のチェックボックス実績を更新する

成功条件:
- CI 相当コマンドがローカルで成功する
- 仕様書と実装の乖離がない

推定工数: 0.5日

実績: 未着手

## 4. 受け入れ基準トレーサビリティ

| AC | 検証方法 | 主対象テスト |
|---|---|---|
| AC-1 | macOS 固有ケースで `gid=80` 許可確認 | `file_validation_test.go` の macOS 条件付きテスト |
| AC-2 | `others` 書き込み拒否の既存テスト回帰確認 | 既存 `validateDirectoryComponentPermissions` 系テスト |
| AC-3 | 非信頼 GID で `ErrInvalidDirPermissions` を確認 | `file_validation_test.go` の非信頼 GID ケース |
| AC-4 | `uid=0,gid=0` の許可継続を確認 | `file_validation_test.go` の root:root ケース |
| AC-5 | Linux で `trusted_gids` 有無比較 | `file_validation_test.go` + `trusted_gids_linux_test.go` + `spec_test.go` |
| AC-6 | 全体回帰 | `make test` / `make lint` |

## 5. 設計整合チェックポイント

- `02_architecture.md` の方針どおり、platform 判定は `trusted_gids_*.go` へ隔離する
- `03_detailed_specification.md` のファイル一覧と一致する変更のみ実施する
- `runner.go` の転写は OS 非依存で行い、macOS では `isTrustedGroup` 側で無視する
- `validateGroupWritePermissions` の変更は root 判定ブロックの置換に限定する

## 6. リスクと緩和策

| リスク | 影響 | 緩和策 |
|---|---|---|
| Linux の追加 GID 判定漏れ | AC-5 不達 | `trusted_gids_linux.go` 専用テスト追加 |
| macOS で設定値を誤って適用 | セキュリティ低下 | darwin 実装で config 非参照を固定化 |
| 既存 root:root 許可を壊す | 後方互換性破壊 | AC-4 テストを先に追加してから実装 |
| 既存 others 拒否に影響 | セキュリティ退行 | AC-2 回帰テストを明示的に実行 |

## 7. 完了条件

- [ ] 変更対象ファイルの実装が完了している
- [ ] AC-1〜AC-6 の検証がテストで確認できる
- [ ] `make fmt` / `make test` / `make lint` が成功する
- [ ] 仕様書（要件・設計・詳細仕様）と計画書に矛盾がない

## 8. 次のステップ

1. Phase 1 から順に実装し、フェーズ完了ごとにテストを追加・実行する
2. Phase 6 で回帰確認後、チェックリストの実績欄を更新する
3. 必要に応じて `03_detailed_specification.md` の検証セクションを最新化する
