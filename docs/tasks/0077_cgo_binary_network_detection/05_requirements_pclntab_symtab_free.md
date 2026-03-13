# 要件定義書: pclntab オフセット検出の .symtab 非依存化

## 1. 背景

### 1.1 既存実装の問題

タスク 0077（Step 2）で実装した `detectPclntabOffset` 関数は、CGO バイナリの
`pclntab` アドレスずれを補正するために ELF `.symtab` セクション（静的シンボルテーブル）を参照している。

```go
// 現行実装（pclntab_parser.go）
func detectPclntabOffset(...) int64 {
    syms, err := elfFile.Symbols()  // .symtab を読む
    if err != nil { return 0 }      // .symtab がなければ補正なし
    ...
}
```

この設計には致命的な実用上の問題がある：**プロダクション環境にデプロイされるバイナリは
ほぼ例外なく `strip` されており、`.symtab` が存在しない**。`strip` コマンドは `.symtab`
セクションを削除してバイナリサイズを削減する標準的な最適化手順であり、デプロイ前に
自動適用されることが多い。

### 1.2 影響

`.symtab` が存在しない stripped バイナリに対して `detectPclntabOffset` は常に 0 を返す。
その結果、CGO バイナリでは以下の動作になる：

- pclntab の関数アドレスが実際の仮想アドレスとずれたまま `GoWrapperResolver` に登録される
- Pass 2（Go ラッパー呼び出し解析）が `syscall.RawSyscall` 等への CALL 命令を検出できない
- `HasNetworkSyscalls: false`、`IsHighRisk: true` → `AnalysisError`（高リスク）として誤処理される

現在のフェイルセーフ設計により実行は禁止方向に倒れるが、正確な検出
（`HasNetworkSyscalls: true` → `NetworkDetected`）が達成できないため、
タスク 0077 の本来目的を果たせない。

### 1.3 検証済みの事実

`ac1_verification_result_x86_64.md` の結果より：

| 項目 | 実測値 |
|------|--------|
| テスト環境 | Go 1.26.0 / x86_64 / CGO_ENABLED=1 |
| バイナリ種別 | 動的リンク ELF（not stripped） |
| 観測されたアドレスずれ | −0x100（−256 バイト） |
| ずれの原因 | `.text` セクション先頭の C スタートアップコード（約 256 バイト） |

`.symtab` が存在する（not stripped）状態での補正は正常動作することが確認済み。

---

## 2. 根本原因分析

### 2.1 CGO バイナリにおける .text レイアウト

通常の（非 CGO）Go バイナリでは `.text` セクション先頭から Go ランタイムコードが始まる。
CGO バイナリでは C リンカが C スタートアップコード（crt0、`_start`、`__libc_start_main` 等）を
`.text` 先頭に挿入する。

| アドレス範囲 | 内容 | 備注 |
|-----------|------|------|
| `.text.Addr` ～ `.text.Addr + C_startup_size` | C スタートアップコード（crt0 等） | 例：256 バイト（0x100） |
| `.text.Addr + C_startup_size` ～ `.text + .text.Size` | Go ランタイム & ユーザーコード | `runtime.text = .text.Addr + C_startup_size` |

**具体例（x86_64 CGO バイナリで観測）**:
- `.text.Addr` = 0x402300
- `C_startup_size` = 0x100 (256 bytes)
- `runtime.text` = 0x402400
- pclntab の関数アドレスは `runtime.text` を基準に記録されている

### 2.2 pclntab と textStart

Go ランタイムは `.gopclntab` セクションに関数情報を記録する。関数アドレスは
`runtime.text`（= Go コードの実際の開始アドレス）を基準としたオフセットとして格納される。

`gosym.NewLineTable(data, textStart)` は第2引数 `textStart` を使ってこれらのオフセットを
絶対仮想アドレスに変換する。

### 2.3 Go バージョン別の pclntab magic（Go 1.26.0 ソースで確認済み）

| Go バージョン | pclntab magic | `debug/gosym` の textStart 扱い |
|-------------|---------------|--------------------------------|
| 1.2–1.15    | `0xfffffffb`  | 渡した `addr` 引数をそのまま使用 |
| 1.16–1.17   | `0xfffffffa`  | 渡した `addr` 引数をそのまま使用 |
| 1.18–1.19   | `0xfffffff0`  | 渡した `addr` 引数をそのまま使用（`t.textStart = t.PC`）|
| 1.20–1.26   | `0xfffffff1`  | 渡した `addr` 引数をそのまま使用（`t.textStart = t.PC`）|

**注**: Go 1.26 は `CurrentPCLnTabMagic = Go120PCLnTabMagic = 0xfffffff1`（新 magic なし）。
`debug/gosym` はすべてのバージョンで `t.textStart = t.PC`（`NewLineTable` の `addr` 引数）を使用する。
ヘッダの `8+2*ptrSize` フィールドは Go 1.20+ では `0` が書き込まれており読み取り不可。

### 2.4 現在の問題箇所（Go 1.26+）

現行コードは `gosym.NewLineTable` に `.text` セクションのアドレス（`.text.Addr`）を渡す：

```go
// internal/runner/security/elfanalyzer/pclntab_parser.go
textStart = textSection.Addr  // .text セクション先頭アドレス
lineTable := gosym.NewLineTable(pclntabData, textStart)
```

`gosym` は渡された `textStart`（= `.text.Addr`）をそのまま関数アドレス計算の基準として使用する。
CGO バイナリでは `.text.Addr ≠ runtime.text` であるため、関数アドレスが `C_startup_size` 分低い値になる。

```
gosym が返す fn.Entry = (pclntab 内オフセット) + .text.Addr
                      = fn_offset + .text.Addr
                      = actual_fn_addr - C_startup_size  ← C_startup_size バイト低い
```

---

## 3. 要件

### 3.1 機能要件

#### FR-1: .symtab 非依存のオフセット検出

`detectPclntabOffset` は `.symtab` セクション（`elfFile.Symbols()`）を参照せずに
pclntab アドレスのずれを検出・返却できること。

#### FR-2: Stripped バイナリ対応

ELF バイナリが `strip` されていて `.symtab` が存在しない場合でも、正しいオフセットを返すこと。

#### FR-3: 非 CGO バイナリでの誤検出なし

非 CGO バイナリ（C スタートアップコードなし）に対しては offset = 0 を返すこと。

#### FR-4: フェイルセーフ

オフセット検出に失敗した場合は offset = 0 を返し、既存の IsHighRisk パスに委ねること。

#### FR-5: サポート外バージョンの明示的エラー

`ParsePclntab` は pclntab の magic 値を `detectPclntabOffset` の呼び出し前に確認し、
サポート対象外（magic ≠ `0xfffffff1`、すなわち Go 1.18–1.19 以前）の場合は
offset = 0 を返すのではなく、`ErrUnsupportedPclntabVersion` エラーを返すこと。
誤動作（誤った offset 適用）を避けるためにフェイルオープンではなくフェイルクローズとする。

### 3.2 非機能要件

- **サポート対象: magic = `0xfffffff1` のバイナリのみ**（Go 1.20–1.26 の共通 magic）
  - magic ≠ `0xfffffff1`（Go 1.18–1.19: `0xfffffff0`、それ以前: `0xfffffffa`/`0xfffffffb`）のバイナリは `ErrUnsupportedPclntabVersion` エラーとして扱う
  - 公称サポートバージョンは Go 1.26（実環境テストが Go 1.26 のみのため）
  - Go 1.20–1.25 は magic が同一であり技術的には通過するが、動作検証が Go 1.26 のみのため保証外
  - サポート対象外バージョンに対して、誤検出による誤動作を避けるためサイレントな `offset = 0` フォールバックは行わない（FR-4 の `offset = 0` フォールバックはサポート対象バージョンでの検出失敗時のみ適用）
- 対象アーキテクチャ: x86_64、arm64（現行対応と同じ）

---

## 4. 受け入れ基準

| ID | 条件 | 期待結果 |
|----|------|---------|
| AC-1 | Not-stripped Go 1.26+ CGO バイナリ（.symtab あり）| offset = C_startup_size（例: 0x100）を検出、pclntab アドレスが正しく補正される |
| AC-2 | Stripped Go 1.26+ CGO バイナリ（.symtab なし） | offset = C_startup_size を検出（.symtab を参照せずに）、pclntab アドレスが正しく補正される |
| AC-3 | Go 1.26+ 非 CGO Go バイナリ（static/dynamic） | offset = 0 |
| AC-4 | pclntab 解析失敗時（不正なデータ等） | `ErrInvalidPclntab` または `ErrUnsupportedPclntab` を返す |
| AC-5 | magic ≠ `0xfffffff1` のバイナリ（Go 1.18–1.19: magic = `0xfffffff0`） | `ErrUnsupportedPclntabVersion` を返す（誤動作防止のため明示的エラー）|
| AC-6 | Go 1.26+ CGO バイナリ（CALL ターゲット相互参照） | offset = C_startup_size を検出できること |
