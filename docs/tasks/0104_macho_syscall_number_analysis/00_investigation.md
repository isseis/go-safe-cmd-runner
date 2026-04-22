# 調査記録：`svc #0x80` 番号不明エントリの詳細解析

## 目的

`record` コマンド自身を `record` で解析したところ、以下の `svc #0x80` が `number: -1`（syscall 番号不明）として記録された。
本ドキュメントはその原因と実際の内容を逆アセンブルにより調査した記録である。

## 対象エントリ

```json
{ "number": -1, "location": 4295805936, "determination_method": "direct_svc_0x80" }
{ "number": -1, "location": 4295806060, "determination_method": "direct_svc_0x80" }
```

16 進数への変換：

| location (dec) | location (hex) |
|---|---|
| 4295805936 | `0x1000CCBF0` |
| 4295806060 | `0x1000CCC6C` |

## 逆アセンブル結果

### `0x1000CCBF0` 付近（前後含む）

```asm
0x1000ccbd0  MOVD.W R30, -16(RSP)         ; ← 関数先頭 (str x30, [sp, #-0x10]!)
0x1000ccbd4  MOVD   R29, -8(RSP)
0x1000ccbd8  SUB    $8, RSP, R29
0x1000ccbdc  CALL   runtime·entersyscall  ; bl 0x10007e620
0x1000ccbe0  MOVD   24(RSP), R16          ; ldr x16, [sp, #0x18]  ← x16 をスタックから読む
0x1000ccbe4  MOVD   32(RSP), R0
0x1000ccbe8  MOVD   40(RSP), R1
0x1000ccbec  MOVD   48(RSP), R2
0x1000ccbf0  SVC    $0x80                 ← 検出された svc #0x80
0x1000ccbf4  BCC    ok
...
```

### `0x1000CCC6C` 付近（前後含む）

```asm
0x1000ccc40  MOVD.W R30, -16(RSP)         ; ← 関数先頭
0x1000ccc44  MOVD   R29, -8(RSP)
0x1000ccc48  SUB    $8, RSP, R29
0x1000ccc4c  CALL   runtime·entersyscall  ; bl 0x10007e620
0x1000ccc50  MOVD   24(RSP), R16          ; ldr x16, [sp, #0x18]  ← x16 をスタックから読む
0x1000ccc54  MOVD   32(RSP), R0
0x1000ccc58  MOVD   40(RSP), R1
0x1000ccc5c  MOVD   48(RSP), R2
0x1000ccc60  MOVD   56(RSP), R3
0x1000ccc64  MOVD   64(RSP), R4
0x1000ccc68  MOVD   72(RSP), R5
0x1000ccc6c  SVC    $0x80                 ← 検出された svc #0x80
0x1000ccc70  BCC    ok
...
```

### なぜ `backwardScanX16` が失敗するか

`backwardScanX16` は `svc #0x80` の直前を後方スキャンし、`MOVZ X16, #imm` や `MOVK X16, #imm` による**即値ロード**パターンを探す。
両関数では x16 が `MOVD 24(RSP), R16`（= `ldr x16, [sp, #0x18]`）でスタックからロードされるため、即値パターンが存在せずスキャンは失敗し `number = -1` となる。

## 関数の特定

`go tool objdump` のソース参照から：

| アドレス | 関数名 | ソースファイル |
|---|---|---|
| `0x1000CCBD0` | `syscall.Syscall` | `syscall/asm_darwin_arm64.s:12` |
| `0x1000CCC40` | `syscall.Syscall6` | `syscall/asm_darwin_arm64.s:53` |

Go ツールチェーン（`go1.26.2`）の当該ソース（`syscall/asm_darwin_arm64.s`）：

```asm
// func Syscall(trap uintptr, a1, a2, a3 uintptr) (r1, r2, err uintptr)
TEXT ·Syscall(SB),NOSPLIT,$0-56
    BL   runtime·entersyscall<ABIInternal>(SB)
    MOVD trap+0(FP), R16   // syscall 番号を第1引数（FP+0）から R16 へ
    MOVD a1+8(FP), R0
    MOVD a2+16(FP), R1
    MOVD a3+24(FP), R2
    SVC  $0x80
    ...

// func Syscall6(trap uintptr, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2, err uintptr)
TEXT ·Syscall6(SB),NOSPLIT,$0-80
    BL   runtime·entersyscall<ABIInternal>(SB)
    MOVD trap+0(FP), R16
    MOVD a1+8(FP), R0
    ...（以下 R5 まで）
    SVC  $0x80
```

これらは Go の**旧スタック ABI**（stack-based calling convention）を使う assembly スタブである。
引数はレジスタではなくスタック上のフレームオフセット（`FP+N`）で渡される。

## 呼び出し規約の詳細

### 旧スタック ABI でのフレームレイアウト

`syscall.Syscall` の場合（`$0-56`：ローカル frame 0 bytes、args 56 bytes）：

```
caller_SP+0   : （呼び出し前の戻りアドレス格納用スロット）
caller_SP+8   : trap   ← syscall 番号（第1引数）
caller_SP+16  : a1
caller_SP+24  : a2
caller_SP+32  : a3
caller_SP+40  : r1（戻り値）
caller_SP+48  : r2（戻り値）
caller_SP+56  : err（戻り値）
```

`Syscall` 内部のフレーム確保（`MOVD.W R30, -16(RSP)` → sp -= 16）後：

```
新 sp+0   : 保存 R30
新 sp+8   : （パディング）
新 sp+16  : caller_SP+0 相当
新 sp+24  = caller_SP+8 = trap  → MOVD 24(RSP), R16
新 sp+32  = caller_SP+16 = a1   → MOVD 32(RSP), R0
...
```

### 呼び出しサイトの実際の命令列

`syscall.Syscall` の呼び出し元（例：`0x10043158c`）：

```asm
0x100431580  MOV  x5, #0x49            ; syscall 番号（73 = munmap）を x5 に
0x100431584  STP  x5, x4, [sp, #0x8]  ; [sp+8]=trap=x5=0x49, [sp+16]=a1=x4
0x100431588  STP  x3, xzr, [sp, #0x18]; [sp+24]=a2, [sp+32]=a3
0x10043158c  BL   0x1000ccbd0          ; syscall.Syscall
```

`syscall.Syscall6` の呼び出し元（例：`0x1004351ec`）：

```asm
0x1004351cc  MOV  x0, #0xc5            ; syscall 番号（197 = mmap）を x0 に
0x1004351d0  STR  x0, [sp, #0x8]      ; [sp+8]=trap=0xc5
0x1004351d4  STP  x1, x2, [sp, #0x10]
...
0x1004351ec  BL   0x1001ea590          ; stub → b 0x1000ccc40 (syscall.Syscall6)
```

## 実際の syscall 番号

| 関数 | syscall 番号 | 名前 |
|---|---|---|
| `syscall.Syscall` | 73 (`0x49`) | `munmap` |
| `syscall.Syscall6` | 197 (`0xc5`) | `mmap` |

`munmap` と `mmap` はいずれも非ネットワーク syscall であり、`record` コマンドが Mach-O バイナリをメモリマップして解析する際に呼び出されるものと考えられる。

## pclntab の状況

バイナリに `.gopclntab` セクションが存在する（magic: `0xfffffff1`、Go 1.26）。
ただし `syscall.Syscall` および `syscall.Syscall6` は `go tool objdump` の TEXT ラベルとして出力されるが、
`nm` によるシンボルテーブルには現れない（アセンブリスタブのため）。

Pass 2 で関数アドレスを特定するには、pclntab から `syscall.Syscall`・`syscall.Syscall6` の関数エントリを読み出す必要がある。

## Pass 2 設計への影響

### 問題点

要件書 FR-3.2.2 は「Go レジスタ ABI では第1引数は X0 で渡される」として `MOV X0, #imm` を後方スキャン対象としているが、`syscall.Syscall`/`syscall.Syscall6` は**旧スタック ABI** を使うため、`trap` 引数は X0 ではなく `[SP, #8]` に格納される。

### 正しいスキャン方法

呼び出しサイトで syscall 番号を取得するには：

1. `BL syscall.Syscall[6]` 命令を検出する
2. その直前を後方スキャンし、`[SP, #8]`（trap 引数スロット）に書き込むストア命令を探す
   - `STP xN, ..., [SP, #8]`（xN が trap）
   - `STR xN, [SP, #8]`
3. xN に即値を設定している命令を更に後方スキャンする
   - `MOV xN, #imm`（`MOVZ xN, #imm`）
   - `MOVZ xN, #hi, LSL#16` + `MOVK xN, #lo`

`[SP, #8]` が固定オフセットである根拠：
Go 旧スタック ABI では呼び出し直前の `SP+8` が `trap+0(FP)` に対応する（`SP+0` は戻りアドレス用スロット）。

### `RawSyscall`/`RawSyscall6` との違い

`RawSyscall`/`RawSyscall6` は `entersyscall` を呼ばない点のみが異なり、引数の渡し方は同じ旧スタック ABI である。
Pass 2 の呼び出しサイトスキャンロジックはすべての `syscall.{,Raw}Syscall{,6,9}` に共通して適用できる。
