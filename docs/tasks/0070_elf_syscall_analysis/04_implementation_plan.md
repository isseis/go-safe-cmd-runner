# 実装計画: ELF 機械語解析による syscall 静的解析

## 目的

- syscall 静的解析機能の実装作業を段階的に整理し、受け入れ条件とテスト対応を明確化する。

## 進捗管理

- [ ] 1. pclntab 解析の範囲確定と仕様整合
  - Go 1.16+ pclntab 形式に限定して関数名・アドレスの抽出を実装する。
  - Go 1.2-1.15 はベストエフォートとし、解析不能時は ErrInvalidPclntab を返す。
  - 詳細仕様書の記述と整合していることを確認する。

- [ ] 2. `PclntabParser.parseFuncTable` の実装
  - pcHeader と functab に基づく解析を実装する。
  - `textStart`, `funcnameOffset`, `ftabOffset`, `nfunc` を読み取り、
    関数名とエントリーポイントを復元する。
  - 解析失敗時のエラーハンドリング（境界検証、null 終端）を実装する。

- [ ] 3. Go ラッパー解決の動作確認
  - `.gopclntab` の関数抽出結果が `GoWrapperResolver` に反映されることを確認する。
  - `TestGoWrapperResolver_Resolve` の前提（関数抽出）が満たされることを確認する。

- [ ] 4. 仕様・テストの整合チェック
  - 受け入れ条件とテストマッピングを再確認する。
  - 実装差分と仕様の不一致がないことを確認する。

- [ ] 5. ドキュメント更新
  - 実装後タスク（docs/development/ 更新）に反映する。
  - pclntab 解析の制約や前提を明記する。
