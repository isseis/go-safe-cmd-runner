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

### 2.3 Go バージョン別の textStart 管理

| Go バージョン | pclntab magic | textStart の扱い |
|-------------|---------------|-----------------|
| 1.2–1.15    | `0xfffffffb`  | header に含まれない / 別途管理 |
| 1.16–1.17   | `0xfffffffa`  | header に含まれない |
| 1.18–1.19   | `0xfffffff0`  | **header に埋め込み**（オフセット: `8 + ptrSize`）|
| 1.20–1.25   | `0xfffffff1`  | **header に埋め込み**（オフセット: `8 + 2*ptrSize`）|
| 1.26+       | TBD           | **header から削除**（textStart は外部から供給）|

> **注記**: Go 1.26 の pclntab magic 値は実装前に Go ソースで確認する。
> `$GOROOT/src/debug/gosym/pclntab.go` を参照。

### 2.4 現在の問題箇所（Go 1.26+）

現行コードは `gosym.NewLineTable` に `.text` セクションのアドレス（`.text.Addr`）を渡す：

```go
// internal/runner/security/elfanalyzer/pclntab_parser.go
textStart = textSection.Addr  // .text セクション先頭アドレス
lineTable := gosym.NewLineTable(pclntabData, textStart)
```

Go 1.26+ の pclntab では header に textStart がないため、`gosym` は渡された `textStart`
（= `.text.Addr`）をそのまま関数アドレス計算の基準として使用する。CGO バイナリでは
`.text.Addr ≠ runtime.text` であるため、関数アドレスが `C_startup_size` 分低い値になる。

```
gosym が返す fn.Entry = (pclntab 内オフセット) + .text.Addr
                      = fn_offset + .text.Addr
                      = actual_fn_addr - C_startup_size  ← C_startup_size バイト低い
```

**Go 1.18–1.25 については**: `gosym` が header から textStart を自動的に読み取り使用するため、
渡した `.text.Addr` にかかわらず正しいアドレスが返る可能性がある。
この挙動は実装後のテストで確認する。

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

### 3.2 非機能要件

- サポート対象 Go バージョン: 1.18 以上（現在の対応範囲を変更しない）
- 対象アーキテクチャ: x86_64、arm64（現行対応と同じ）

---

## 4. 受け入れ基準

| ID | 条件 | 期待結果 |
|----|------|---------|
| AC-1 | Not-stripped CGO バイナリ（既存の .symtab ありケース） | offset = C_startup_size（例: 0x100）を検出、pclntab アドレスが正しく補正される |
| AC-2 | Stripped CGO バイナリ（.symtab なし） | offset = C_startup_size を検出（.symtab を参照せずに）、pclntab アドレスが正しく補正される |
| AC-3 | 非 CGO Go バイナリ（static/dynamic） | offset = 0 |
| AC-4 | pclntab 解析失敗時（不正なデータ等） | offset = 0（フェイルセーフ） |
| AC-5 | Go 1.18–1.25 CGO バイナリ | `detectPclntabOffset` が返す offset を適用した後、`fn.Entry + offset` が ELF シンボルテーブル（.symtab）上の関数 VA と一致すること。gosym が既に textStart を適用済みなら offset = 0 かつ fn.Entry が正しい VA、未適用なら offset = C_startup_size かつ補正後に正しい VA — いずれの場合も補正後アドレスが正しいことを検証する |
| AC-6 | Go 1.26+ CGO バイナリ | offset = C_startup_size を検出できること |
