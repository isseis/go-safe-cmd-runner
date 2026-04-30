# Dead Code Removal Follow-up Implementation Plan

## 0. 目的

この手順書は、dead code 調査結果を安全に反映するための実行計画である。
以下の 3 段階で進める。

1. dead code 5件を最小差分で削除し、ビルドとテスト影響を確認する
2. dead code 候補のうち test helper API 群を別 PR で削除する
3. go vet 失敗要因（test helper 欠落と build tag 問題）を修正し、2回目の精査を実施する

## 1. 前提条件

- 作業ブランチを分ける（例: `issei/deadcode-removal-01`, `issei/deadcode-removal-02`, `issei/deadcode-audit-02`）
- 各 PR は目的を 1 つに限定する
- 変更後は必ず `make fmt`、`go build ./...`、`make test` を実行する
- `go vet ./...` は現状失敗するため、Phase 3 までは参考値として扱う

## 2. Phase 1: dead code 5件を先に削除（最小差分 PR）

### 2.1 対象

削除対象（前回調査で `dead code` 判定）:

- `internal/security/elfanalyzer/x86_go_wrapper_resolver.go`: `(*X86GoWrapperResolver).resolveSyscallArgument`
- `internal/verification/test_helpers.go`: `WithFileValidatorEnabled`
- `internal/verification/test_helpers.go`: `WithTestingSecurityLevel`
- `internal/runner/config/errors.go`: `ErrConfigFileInvalidFormat`（型 + メソッド）
- `internal/runner/config/template_errors.go`: `ErrMultipleValuesInStringContext`（型 + メソッド）

### 2.2 手順

- [ ] `rg` で各シンボルの最終参照確認（`--glob '*.go'`）
- [ ] 対象 5 件のみ削除（関連コメント含む）
- [ ] `make fmt` 実行
- [ ] `go build ./...` 実行
- [ ] `make test` 実行
- [ ] 失敗時は失敗ログを整理し、削除の妥当性と別因子を切り分け

### 2.3 完了条件

- [ ] 対象 5 件以外に不要な差分がない
- [ ] `go build ./...` が成功
- [ ] `make test` が成功、または既知失敗のみであることを説明できる
- [ ] PR タイトルと説明に「Phase 1 / dead code 5件削除」と明記

### 2.4 PR チェックリスト

- [ ] 変更理由（なぜ削除可能か）
- [ ] 参照調査結果（テスト外参照なし）
- [ ] 実行コマンドと結果要約
- [ ] リスクとロールバック方法

## 3. Phase 2: test helper API 群を別 PR で削除

### 3.1 方針

`dead code 候補` のうち「テストからのみ使われる API（または build tag 付き test helper からのみ参照）」をまとめて削除する。
Phase 1 と混ぜず、小さい PR とする。

### 3.2 候補抽出

- [ ] 前回一覧から `dead code 候補` を再確認
- [ ] `//go:build test` / `//go:build test || performance` ファイル経由参照を識別
- [ ] 削除対象を「test helper API 群」に限定して確定

### 3.3 実施手順

- [ ] 参照されるテストコードを先に削除または置換
- [ ] helper API 本体を削除
- [ ] `make fmt` 実行
- [ ] `go build ./...` 実行
- [ ] `make test` 実行

### 3.4 完了条件

- [ ] 本番パスに影響する API 変更が混入していない
- [ ] PR 単体でレビュー可能なサイズ（目安: 300 行前後）
- [ ] 削除対象の根拠（参照経路）が PR 内で説明されている

## 4. Phase 3: go vet 問題修正 + 2回目精査

### 4.1 go vet 失敗の先行修正

現時点の代表的失敗要因:

- test helper 欠落（例: `Int32Ptr`, `MockFileSystem`）
- build constraints により test パッケージが空になる問題

### 4.2 修正手順

- [ ] `go vet ./...` 実行し、失敗一覧を最新版に更新
- [ ] 原因を「欠落シンボル」「build tag 不整合」「その他」に分類
- [ ] 1 件ずつ修正し、都度 `go test` で局所確認
- [ ] `go vet ./...` が通るまで繰り返す

### 4.3 2回目 dead code 精査

`go vet` クリーン後に再精査する。

- [ ] `staticcheck ./...`（U1000 記録）
- [ ] `golangci-lint run --enable=unused,unparam,ineffassign`
- [ ] `go run golang.org/x/tools/cmd/deadcode@latest ./cmd/...`
- [ ] grep ベースで「テストのみ参照」を再判定
- [ ] struct field / interface 実装の未使用も対象に追加

### 4.4 完了条件

- [ ] `go vet ./...` が成功
- [ ] 2回目精査の結果表を更新
- [ ] 判定カテゴリ（dead code / dead code 候補 / 要確認）を再確定
- [ ] 次の削除 PR のスコープを確定

## 5. 進捗管理テンプレート

### 5.1 ステータス

- [ ] Phase 1 実施中
- [ ] Phase 1 完了
- [ ] Phase 2 実施中
- [ ] Phase 2 完了
- [ ] Phase 3 実施中
- [ ] Phase 3 完了

### 5.2 実行ログ（追記用）

- 実施日:
- ブランチ:
- 実施者:
- 実行コマンド:
- 結果サマリ:
- 課題/ブロッカー:
- 次アクション:

## 6. レビュー観点

- 削除対象は本当に本番非使用か
- build tag 付きファイルを本番参照と誤判定していないか
- PR の責務が単一か（Phase 混在がないか）
- 調査再現性（コマンド・判定根拠）が残っているか
