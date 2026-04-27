# 0111 Syscall Indirect Resolution 実装計画

## 背景
`record` コマンドによる ELF システムコール解析で、`/usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2` に対して一部の `syscall` 命令が `number=-1`（`unknown:indirect_setting`）として記録される。

主因は、現行の後方スキャンが「syscall 番号レジスタ (RAX/EAX) への即値代入」のみを解決対象としており、`mov eax, edx` のようなレジスタ経由代入を追跡できないため。

## 目的
フェーズ1として、同一基本ブロック内の単純なレジスタコピー連鎖を解決し、`unknown:indirect_setting` を削減する。

## 対象範囲
- 対象アーキテクチャ: x86_64
- 対象解析経路: 直接 `syscall` 命令の番号抽出
- 対象パターン:
  - `mov eax, edx` のような番号レジスタへのコピー
  - コピー元レジスタに対する直近の即値代入（既存ロジックで扱う MOV imm / XOR same-reg）

## 非対象
- CFG 構築を伴う分岐横断の定数伝播
- メモリ参照や関数呼び出し戻り値を介した値解決
- arm64 側ロジック変更

## 実装方針
1. `backwardScanForRegister` に「追跡対象レジスタの動的切り替え」を導入する。
2. `x86` デコーダに「レジスタコピー判定」のヘルパを追加する。
3. 解析中に `targetReg <- srcReg` を検出したら、追跡対象を `srcReg` に切り替えて後方探索を継続する。
4. 制御フロー境界に達した場合は既存どおり不確定として終了する。
5. 判定メソッドは既存互換を維持し、即値を確定できた場合は `immediate` として扱う。

## 変更予定ファイル
- `internal/runner/security/elfanalyzer/syscall_analyzer.go`
- `internal/runner/security/elfanalyzer/x86_decoder.go`
- `internal/runner/security/elfanalyzer/syscall_analyzer_test.go`
- 必要に応じて `internal/runner/security/elfanalyzer/x86_decoder_test.go`

## テスト計画
- 新規ユニットテストを追加:
  - `mov eax, edx; mov edx, imm; syscall` が即値解決されること
  - `mov eax, edx; xor edx, edx; syscall` が `0` として解決されること
  - コピー元の値が不明な場合は `unknown:indirect_setting` を維持すること
- 既存テストを含め `make test` を実行
- フォーマット確認として `make fmt` を実行

## 実施ステップ
- [ ] Step 1: 解析ロジック拡張（レジスタコピー追跡）
- [ ] Step 2: x86 デコーダ拡張（コピー関係抽出）
- [ ] Step 3: ユニットテスト追加
- [ ] Step 4: `make fmt` 実行
- [ ] Step 5: `make test` 実行
- [ ] Step 6: 差分確認とコミット

## フェーズ2 実装計画（分岐横断の保守的定数伝播）

### 目的
同一基本ブロック内で解決できないケース（例: syscall 直前は `mov eax, r9d` だが、`r9d` の定義が predecessor ブロックにあるケース）を保守的に解決する。

### 方針
1. 後方スキャン範囲で最小限の基本ブロック境界を抽出する。
2. 追跡対象レジスタについて predecessor 側の定義を探索する。
3. 複数経路がすべて同一即値に収束する場合のみ番号を確定する。
4. 経路ごとに値が衝突する場合は不確定（`unknown:indirect_setting` など）を維持する。

### 変更候補
- `internal/runner/security/elfanalyzer/syscall_analyzer.go`
- 必要に応じて `internal/runner/security/elfanalyzer/x86_decoder.go`
- `internal/runner/security/elfanalyzer/syscall_analyzer_test.go`

### テスト項目
- predecessor が単一で一定値に到達するケースが解決されること
- predecessor が複数で値が一致するケースが解決されること
- predecessor が複数で値が不一致のケースは不確定のままであること
- 既存ケースの回帰がないこと

## フェーズ3 実装計画（診断性と安全性の強化）

### 目的
解決率向上後の運用観点として、判定根拠の可観測性を上げ、誤判定時に原因を追跡しやすくする。

### 方針
1. 判定メソッド詳細を拡張し、コピー連鎖・分岐収束のどちらで確定したかを区別可能にする。
2. 解析警告のメッセージを改善し、番号未解決の理由をより具体化する。
3. 解析統計（例: コピー連鎖で解決できた件数）を追加し、改善効果を測定可能にする。
4. 既存 JSON 互換性を崩さないよう、追加フィールドは後方互換で導入する。

### 変更候補
- `internal/runner/security/elfanalyzer/syscall_analyzer.go`
- `internal/common/syscall_types.go`
- `internal/common/syscall_types_test.go`
- 必要に応じて `internal/fileanalysis` 配下の保存・復元テスト

### テスト項目
- 既存 JSON との互換性が維持されること
- 新しい判定情報が正しくシリアライズ/デシリアライズされること
- 解析警告と統計が期待どおり生成されること

## 受け入れ条件
- 既存の即値解決ケースが回帰しない。
- レジスタコピー経由の代表ケースが `-1` ではなく有効なシステムコール番号になる。
- 解析不能ケースは従来どおり `unknown:indirect_setting` を返す。
- 追加テストを含めテストが成功する。
