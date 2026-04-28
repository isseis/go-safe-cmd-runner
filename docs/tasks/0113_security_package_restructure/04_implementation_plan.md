# 実装計画書: セキュリティパッケージの再構成

## 1. 実装概要

### 1.1 目的

`01_requirements.md` と `03_detailed_specification.md` で定義した再構成方針を、
段階的に実装し、依存関係の整理・テスト合格・セキュリティ挙動維持を達成する。

### 1.2 実装原則

- **段階的移行**: ビルドが壊れない順序で移行する
- **最小差分**: ロジック変更は行わず、配置と依存更新を中心に実施する
- **既存再利用**: 既存実装・既存テストを最大限再利用し、自前実装の重複を避ける
- **テスト先行確認**: 各フェーズで対象パッケージのテストを先に追加/移植し、その後変更を反映する
- **コメント品質**: ソースコード内コメントは英語で記載し、日本語を混在させない

## 2. 実装ステップ

### Phase 0: 事前準備とベースライン取得

**対象ファイル**:
- [x] `docs/tasks/0113_security_package_restructure/01_requirements.md`
- [x] `docs/tasks/0113_security_package_restructure/02_architecture.md`
- [x] `docs/tasks/0113_security_package_restructure/03_detailed_specification.md`

**作業内容**:
- [x] 現行の依存グラフを採取 (`go list -deps`)
- [x] ベースラインとして `make test` / `make lint` を実行
- [x] 変更対象パッケージの既存テスト一覧を整理

**成功条件**:
- [x] 依存グラフの差分比較用データが取得できている
- [x] ベースラインのテスト・lint結果が記録されている

**推定工数**: 0.5日
**実績**: [x] 完了

---

### Phase 1: `internal/security` の基盤導入

**対象ファイル（新規）**:
- [x] `internal/security/errors.go`
- [x] `internal/security/dir_permissions.go`
- [x] `internal/security/toctou.go`

**対象ファイル（更新）**:
- [x] `internal/runner/security/toctou_check.go`

**作業内容**:
- [x] `DirectoryPermChecker` インタフェースを追加
- [x] standalone 実装のディレクトリ権限チェッカーを実装
- [x] TOCTOU ユーティリティを `internal/security` へ移動
- [x] `internal/runner/security/toctou_check.go` は `NewValidatorForTOCTOU()` のみに整理

**成功条件**:
- [x] `go test ./internal/security/...` が成功
- [x] `RunTOCTOUPermissionCheck` が `DirectoryPermChecker` 引数で動作

**推定工数**: 1.0日
**実績**: [x] 完了

---

### Phase 2: 解析サブパッケージ移行（binary + mach-o + elf core）

**対象ファイル（新規ディレクトリ）**:
- [x] `internal/security/binaryanalyzer/*`
- [x] `internal/security/machoanalyzer/*`
- [x] `internal/security/elfanalyzer/*`（core のみ）

**対象ファイル（更新）**:
- [x] `internal/runner/security/elfanalyzer/standard_analyzer.go`
- [x] `internal/runner/security/syscall_store_adapter.go`

**作業内容**:
- [x] `binaryanalyzer` を移植（内部コード変更は最小）
- [x] `machoanalyzer` を移植し、`binaryanalyzer` 参照先を更新
- [x] `elfanalyzer` は core のみ移植し、`StandardELFAnalyzer` は runner 側に残留
- [x] `syscall_store_adapter` の参照先を `internal/security/elfanalyzer` へ更新

**成功条件**:
- [x] `internal/security/...` が `internal/runner/...` を import しない
- [x] `go test ./internal/security/elfanalyzer/...` が成功

**推定工数**: 1.5日
**実績**: [x] 完了

---

### Phase 3: 利用側の依存更新（record / verify / filevalidator / libccache）

**対象ファイル**:
- [ ] `cmd/verify/main.go`
- [ ] `cmd/record/main.go`
- [ ] `cmd/runner/main.go`
- [ ] `internal/filevalidator/validator.go`
- [ ] `internal/libccache/adapters.go`
- [ ] `internal/runner/security/binary_analyzer.go`

**作業内容**:
- [ ] `cmd/verify` の TOCTOU 呼び出しを `internal/security` へ移行
- [ ] `cmd/record` の TOCTOU 呼び出しを `internal/security` へ移行
- [ ] `cmd/record` の `NewSyscallAnalyzer()` 参照先を `internal/security/elfanalyzer` へ移行
- [ ] `cmd/record` の `internal/runner/security` 依存を `NewBinaryAnalyzer()` 用のみに限定
- [ ] `internal/filevalidator` / `internal/libccache` の import を `internal/security/...` へ変更
- [ ] `cmd/runner` の TOCTOU ユーティリティ参照を `internal/security` へ変更

**成功条件**:
- [ ] `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功
- [ ] `go list -deps ./cmd/verify | grep internal/runner/security` が 0 件
- [ ] `go list -deps ./cmd/record | grep internal/runner/security/elfanalyzer` が 0 件

**推定工数**: 1.0日
**実績**: [ ] 未着手

---

### Phase 4: 旧配置の削除と整理

**対象ファイル/ディレクトリ**:
- [ ] `internal/runner/security/binaryanalyzer/`（削除）
- [ ] `internal/runner/security/machoanalyzer/`（削除）
- [ ] `internal/runner/security/elfanalyzer/`（core 部分削除）

**作業内容**:
- [ ] 移行済みコードを削除
- [ ] 残留対象（`StandardELFAnalyzer` 系）を明示
- [ ] dangling import / 参照切れを修正

**成功条件**:
- [ ] `go test ./internal/runner/security/...` が成功
- [ ] 削除対象に未参照の残骸がない

**推定工数**: 0.5日
**実績**: [ ] 未着手

---

### Phase 5: テスト整備・回帰確認

**対象ファイル（追加/更新）**:
- [ ] `internal/security/*_test.go`
- [ ] `internal/filevalidator/*_test.go`
- [ ] `internal/libccache/*_test.go`
- [ ] `cmd/record/*_test.go`
- [ ] `cmd/verify/*_test.go`

**作業内容**:
- [ ] 移植先パッケージで必要な単体テストを追加
- [ ] 既存テストの import path を更新
- [ ] 重複テストを除去（同一ロジックの二重検証を回避）
- [ ] 既存ヘルパーを再利用し、自前ユーティリティの重複実装を避ける

**成功条件**:
- [ ] `make test` 成功
- [ ] 追加テストが AC と 1:1 または 1:多で対応付けられている
- [ ] 既存テストと重複する無意味なテストがない

**推定工数**: 1.0日
**実績**: [ ] 未着手

---

### Phase 6: 品質ゲート・受け入れ判定

**作業内容**:
- [ ] `make lint` を実行
- [ ] `make build` を実行
- [ ] AC 充足レビュー（FR-1〜FR-6）を実施
- [ ] ソースコード中の日本語コメント/文字列混入を確認（仕様で要求される例外を除く）
- [ ] 変更差分の最終レビュー（再利用性、重複実装の有無）

**成功条件**:
- [ ] `make lint` / `make test` / `make build` がすべて成功
- [ ] AC チェックリストがすべて完了

**推定工数**: 0.5日
**実績**: [ ] 未着手

## 3. 実装順序とマイルストーン

### M1: 基盤分離完了
- [ ] Phase 1 完了
- [ ] `internal/security` の TOCTOU / dir-permission API が利用可能

### M2: 解析パッケージ移行完了
- [ ] Phase 2 完了
- [ ] `internal/security/{binaryanalyzer,machoanalyzer,elfanalyzer(core)}` が利用可能

### M3: 利用側移行完了
- [ ] Phase 3, 4 完了
- [ ] `cmd/verify` とライブラリ群の依存再編が完了

### M4: 品質ゲート通過
- [ ] Phase 5, 6 完了
- [ ] 受け入れ条件を全て満たす

## 4. 受け入れ条件トレーサビリティ

### FR-1
- [ ] Phase 1 で `internal/security` 基盤導入
- [ ] Phase 6 で `go build ./internal/security/...` 確認

### FR-2
- [ ] Phase 2 でサブパッケージ移行
- [ ] Phase 5/6 で関連テスト実施

### FR-3
- [ ] Phase 3 で `cmd/verify` 依存解消
- [ ] Phase 3 で `cmd/record` の `elfanalyzer` 依存解消
- [ ] Phase 6 で依存グラフ検証

### FR-4
- [ ] Phase 3 で `internal/filevalidator` / `internal/libccache` 依存更新
- [ ] Phase 5/6 でテスト検証

### FR-5
- [ ] Phase 1/3/4 で `internal/runner/security` の責務再編
- [ ] Phase 6 で `cmd/runner` / `internal/verification` 回帰確認

### FR-6
- [ ] Phase 6 で `make test` / `make build` / `make lint` 完了

## 5. リスク管理

### 技術リスク

- [ ] **ELF 層分割時の型循環**: `StandardELFAnalyzer` と core 型の依存方向を固定
- [ ] **TOCTOU API 変更による呼び出し漏れ**: `grep` ベースで呼び出し箇所を網羅確認
- [ ] **テスト破壊の見落とし**: フェーズ単位で小さく実行し、失敗箇所を即時修正

### スケジュールリスク

- [ ] 依存切替と削除を同一コミットに混在させない（段階コミット）
- [ ] マイルストーン単位でレビューを入れる

## 6. 実装チェックリスト

### 実装前
- [ ] ベースライン取得
- [ ] 影響範囲の明確化

### 実装中
- [ ] 各 Phase の成功条件を満たしてから次へ進む
- [ ] 既存関数の再利用を優先し、重複実装しない
- [ ] コードコメントは英語で記述する

### 実装後
- [ ] AC トレース完了
- [ ] 重複・不要テストがないことを確認
- [ ] ドキュメント（01-04）が相互整合している

## 7. 成功基準

- [ ] 機能要件 FR-1〜FR-6 をすべて満たす
- [ ] 非機能要件（後方互換性・テスト戦略・セキュリティ）を満たす
- [ ] 依存グラフが設計方針に一致する
- [ ] 実装計画書のチェックボックスで進捗追跡が可能

## 8. 次のステップ

- [ ] Phase 0 着手（ベースライン取得）
- [ ] M1 完了時に中間レビュー
- [ ] M4 完了後に最終レビューとマージ判断
