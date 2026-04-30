# arm64 命令デコードの技術詳細

本ドキュメントは、ELF 機械語解析による syscall 静的解析機能における
arm64 命令デコードの技術的な動作仕様と設計判断を記録する。

## 1. 解析の全体構造

`SyscallAnalyzer` は x86_64 と arm64 の両アーキテクチャを `MachineCodeDecoder`
インタフェースで抽象化している。arm64 向け実装は `ARM64Decoder` 構造体が担う。

解析は 2 つのパスで構成される。

```
Pass 1: findSyscallInstructions
  → .text セクションを前向きスキャンして SVC #0 命令 (0xD4000001) の位置を列挙
  → 各 SVC #0 命令に対して backwardScanForRegister で syscall 番号を解決

Pass 2: FindWrapperCalls (ARM64GoWrapperResolver)
  → .text セクションを前向きスキャンして Go syscall ラッパーへの BL を検出
  → 直前の命令列を後向きスキャンして第 1 引数 (X0/W0) の値を解決
```

Pass 1 で検出された SVC #0 命令が既知のラッパー関数・実装関数の内部に
位置する場合は、番号を静的に決定できないためスキップする。

## 2. デコード失敗時の動作

### 2.1 固定長命令による境界整合の保証

arm64 の全命令は 4 バイト固定長である。このため、`InstructionAlignment()` が
返す値は常に 4 であり、デコード失敗時も 4 バイト単位で位置を進めることで
命令境界の整合性が常に保たれる。

```
失敗位置: pos
次の試行位置: pos + 4
```

x86_64 の可変長命令（1〜15 バイト）と異なり、arm64 では命令境界のずれが
構造上発生しない。デコード失敗後に誤った命令境界に再同期するリスクは存在しない。

### 2.2 デコード失敗の要因

arm64 の `.text` セクションは全命令が 4 バイト境界に整列しているため、
デコード失敗は主に以下の原因で発生する。

- 未定義命令（undefined instruction）のエンコーディング
- arm64asm ライブラリが未対応の命令バリアント
- データ領域が `.text` セクションに混在しているケース

これらの場合も 4 バイト単位でスキップするため、後続の命令境界は
常に正しく維持される。

### 2.3 誤検出リスク

arm64 は固定長命令であるため、x86_64 で発生するような命令境界のずれによる
SVC #0 の誤検出リスクは存在しない。`0xD4000001` パターンが実際の SVC #0 命令
でない場合（データ領域の偶発的一致など）でも、逆方向スキャンで syscall 番号の
解析を試みるため、不正な SVC #0 命令の場合は以下のいずれかとなる。

- 妥当な syscall 番号が見つからない → `unknown:*` として High Risk 判定
- 偶然妥当な番号に見える → 誤検出（理論上のリスクだが、実用上は極めて稀）

## 3. 後向きスキャンによる syscall 番号解決

### 3.1 全体フロー（backwardScanForSyscallNumber）

arm64 では x86_64 向けの `backwardScanX86WithRegCopy` とは異なり、
汎用関数 `backwardScanForRegister` が使用される。コピーチェーン追跡や
CFG ベース分岐収束解析は行わない。

```
backwardScanForSyscallNumber
  → decoder が ARM64Decoder でない場合: backwardScanForRegister を呼ぶ
    ↓
    [1] ウィンドウ計算: windowStart = syscallOffset - (maxBackwardScan × 4)
    [2] decodeWindow でウィンドウ内命令列をデコード
    [3] 命令列を末尾から走査:
        - 制御フロー命令 → unknown:control_flow_boundary
        - W8/X8 への書き込みでない → スキップ
        - W8/X8 への即値ロード → 即値を取得して返却
        - W8/X8 への間接書き込み → unknown:indirect_setting
    [4] スキャン上限 (maxBackwardScan = 50) に達した → unknown:scan_limit_exceeded
    [5] ウィンドウ内命令を全消費 → unknown:window_exhausted
```

arm64 は固定長命令のため、x86_64 で必要であった代替ウィンドウ開始位置の
探索（デコード失敗時の境界ずれ対策）は不要である。

### 3.2 スキャン対象レジスタ

arm64 の syscall 番号は W8 または X8 レジスタに設定する規約である。
`WritesSyscallReg` は命令の第 1 オペランドが W8 または X8 への書き込みである
かを判定する。読み取り専用オペランドを持つ命令（STR, CMP, CMN, TST など）は
対象外とする。

### 3.3 有効な syscall 番号の検証

即値が得られた場合でも、`maxValidSyscallNumber = 500` を超える値または
負の値は `unknown:indirect_setting` として扱う。

## 4. 即値ロードのエンコーディング

arm64 では syscall 番号を W8/X8 レジスタに設定する命令として、2 種類の
エンコーディングを認識する。

### 4.1 MOV W8/X8, #imm（MOVZ 正規化）

コンパイラは 16 ビット即値を MOVZ 命令として生成し、arm64asm ライブラリは
これを `MOV Wn/Xn, #imm` として正規化して返す。`IsSyscallNumImm` は
`arm64asm.MOV` オペコードと W8/X8 宛先を確認して即値を取得する。

```asm
MOV W8, #198    ; MOVZ W8, #198 として生成; arm64asm が MOV に正規化
SVC #0
```

### 4.2 ORR W8/X8, WZR/XZR, #imm（ビットマスク即値形式）

MOVZ で表現できないが ARM64 ビットマスク即値形式に収まる定数は、
コンパイラが `ORR Wn/Xn, WZR/XZR, #imm` として生成する場合がある。
これは `MOV Wn, #imm` と機能的に等価である。`arm64OrrZeroRegImm` が
このパターンを検出する。

```asm
ORR X8, XZR, #imm   ; MOV X8, #imm と機能的に等価
SVC #0
```

## 5. ADRP+LDR によるグローバル変数解決

### 5.1 対象パターン

Go の syscall パッケージでは、syscall 番号をパッケージレベル変数に格納する
ケースがある。arm64 の場合、以下のような ADRP+LDR ペアが生成される。

```asm
ADRP Xn, <page>         ; Xn にページベースアドレスをロード
LDR  X0/W0, [Xn, #offset] ; ページ内オフセットから値をロード
BL   <wrapper>
```

### 5.2 ResolveFirstArgGlobal の動作

`ARM64Decoder.ResolveFirstArgGlobal` は Pass 2（Go ラッパー解析）の
後向きスキャン中に呼ばれ、以下の手順で値を解決する。

1. 命令が `LDR X0/W0, [Xn, #offset]` 形式（符号なしオフセット形式）かを確認する。
2. エンコーディングから `imm12` フィールドを抽出し、レジスタサイズに応じてシフトして
   バイトオフセットを算出する。
   - X0（64 ビット）: `offset = imm12 << 3`
   - W0（32 ビット）: `offset = imm12 << 2`
3. 直前の命令列を最大 `arm64ADRPBacktrackLimit = 4` 命令まで後向きスキャンして、
   同一ベースレジスタへの ADRP 命令を探す。
4. ADRP 命令の実効アドレス `(instOffset & ~0xFFF) + sign_extend(pcrel)` を計算する。
5. `SetDataSections` で登録した ELF セクション（`.noptrdata`, `.rodata`, `.data`）
   から当該アドレスの値を読み取る（W0 の場合は 4 バイト、X0 の場合は 8 バイトの
   リトルエンディアン整数）。

### 5.3 RIP 相対アドレッシングとの相違点

x86_64 では `MOV RAX, [RIP + disp32]` という RIP 相対形式が使われるのに対し、
arm64 では ADRP+LDR の 2 命令ペアを用いる。このため arm64 側の解決では
単一命令のデコードではなく、直前命令列への後向きスキャンが必要となる。

## 6. 透過的ラッパーの検出

### 6.1 透過的ラッパーとは

Go ランタイムでは、`syscall.Syscall` 等のラッパー関数を内部でさらにラップする
関数が生成されることがある。このような「透過的ラッパー」は、引数をスタック経由で
受け取り、内部ヘルパーを呼び出して戻り値をスタックに保存し、最終的に既知の
ラッパー関数を呼び出す構造を持つ。

### 6.2 discoverTransparentWrappers の動作

`ARM64GoWrapperResolver.discoverTransparentWrappers` は以下の構造パターンを
スライディングウィンドウで検索する。

```
[プロローグ] STR X30, [SP, #-n]!    ← 関数開始（リターンアドレス保存）
...
[スタック保存] STR X0, [SP, #offset] ← 引数をスタックに退避
...
[ヘルパー呼び出し] BL <helper>       ← 既知ラッパー以外への CALL
...
[スタック再ロード] LDR X0, [SP, #offset] ← スタックから引数を再ロード
...
[ラッパー呼び出し] BL <known wrapper> ← 既知ラッパーへの CALL
[末尾] RET                            ← 関数終了
```

ウィンドウサイズは以下の定数で制御される。

| 定数 | 値 | 意味 |
|---|---|---|
| `arm64ReloadSearchWindow` | 8 命令 | スタック再ロードの探索範囲 |
| `arm64HelperSearchWindow` | 15 命令 | ヘルパー呼び出しの探索範囲 |
| `arm64SaveSearchWindow` | 15 命令 | スタック保存の探索範囲 |
| `arm64PrologueSearchWindow` | 6 命令 | プロローグの探索範囲 |
| `arm64FunctionTailSearchSpan` | 24 命令 | RET 検索の先読み範囲 |

検出された透過的ラッパーは `wrapperAddrs` に追加され、その範囲は
`wrapperRanges` に登録される。Pass 1 での直接 SVC #0 のスキップ判定
（`IsInsideWrapper`）にも使用される。

### 6.3 スライディングウィンドウの効率

セクション全体を配列としてメモリに保持せず、一定サイズ（最大 69 命令）の
ウィンドウをスライドさせることで、O(1) のメモリ使用量でセクション全体を
解析できる。

## 7. 判定メソッドと詳細コード

### 7.1 DeterminationMethod 定数

x86_64 と共通の定数を使用する（arm64 固有の詳細コードは存在しない）。

| 定数 | 意味 |
|---|---|
| `immediate` | 即値から直接決定 |
| `go_wrapper` | Go ラッパー呼び出しの引数から決定 |
| `unknown:decode_failed` | 命令デコード失敗により不明 |
| `unknown:control_flow_boundary` | 制御フロー境界に到達して不明 |
| `unknown:indirect_setting` | 間接的な設定（メモリ参照・レジスタ間接など）で不明 |
| `unknown:scan_limit_exceeded` | スキャンステップ上限（`defaultMaxBackwardScan = 50`）に到達 |
| `unknown:window_exhausted` | ウィンドウ内の全命令を消費したが見つからなかった |
| `unknown:invalid_offset` | SVC #0 命令のオフセットが不正 |

### 7.2 arm64 に固有の詳細コード

arm64 ではレジスタコピーチェーンや CFG 分岐収束解析を行わないため、
x86_64 固有の詳細コード（`x86_copy_chain`, `x86_branch_converged` など）は
使用しない。`DeterminationDetail` フィールドは `invalid_offset` の場合のみ
設定される。

## 8. 設計判断の根拠

### 8.1 固定長命令による設計の単純化

arm64 の固定長命令（4 バイト）は x86_64 の可変長命令（1〜15 バイト）と比較して、
命令デコードの実装を大幅に単純化する。

1. **デコード失敗時の再同期が不要**: 4 バイト単位で進めれば常に正しい命令境界に
   到達するため、x86_64 で必要であった「代替ウィンドウ開始位置の探索」は不要。
2. **命令境界ずれによる誤検出リスクがない**: x86_64 ではデコード失敗後の
   再同期中に偶然 `0F 05` が現れる誤検出リスクがあったが、arm64 ではこのような
   リスクが構造上存在しない。

### 8.2 コピーチェーン・CFG 解析を省略した理由

arm64 コンパイラ（特に Go）は通常、syscall 番号を W8/X8 に直接即値でロードする
パターンを生成する。x86_64 で見られるようなレジスタコピーチェーン（MOV EAX, EDX
等）が連鎖するパターンは arm64 ではほとんど観測されないため、これらの複雑な解析
機構は省略している。

### 8.3 デコード失敗を High Risk としない理由

x86_64 と同様の理由による。

1. **Pass 1 の解析対象は直接 SVC #0 命令**であり、デコード失敗は
   SVC #0 命令自体の検出には影響しにくい。SVC #0 は `0xD4000001` の
   4 バイト固定であり、デコード失敗がこのパターンの検出精度に
   直接影響することは少ない。

2. **固定長命令のためデコード失敗が多発するケースは x86_64 より稀**であり、
   `.text` セクションは通常有効な命令で構成されている。

3. **Pass 2（Go ラッパー解析）でのデコード失敗**も、必ずしも
   syscall ラッパー呼び出しの見落としを意味しない。BL 命令は
   正常にデコードされることが多く、周辺のデコード失敗が
   BL ターゲットの解決に影響する可能性は低い。

### 8.4 安全側への設計原則との整合性

本設計では「検出できない syscall 番号」を High Risk とする（FR-3.1.4）。

- **デコード失敗**: 「命令自体を認識できない」ケース
- **syscall 番号不明**: 「SVC #0 命令は認識できたが番号を特定できない」ケース

この 2 つは異なる性質の問題であり、区別して扱う。

- SVC #0 命令が正常にデコードされた場合に番号が不明 → **High Risk**
- デコード自体の失敗 → **ログ出力のみ**（リスク分類に影響しない）

`unknown:*` メソッドの結果（番号が不明な SVC #0 命令）は
High Risk として扱われる。
デコード失敗カウントはリスク分類に影響しない。

### 8.5 デコード失敗の可視化

デコード失敗は `DecodeStatistics` 構造体で統計情報を収集し、
以下のログ出力で可視化する。

- **個別ログ**: Pass 1 (`findSyscallInstructions`) および
  Pass 2 (`FindWrapperCalls`) の両方で `slog.Debug` により出力。
  Pass 1・Pass 2 それぞれで出力件数を `MaxDecodeFailureLogs`（= 10）で制限。
- **サマリログ**: record コマンドで `slog.Debug` によりファイルパス、
  デコード失敗総数（Pass 1 + Pass 2 の合計）、解析バイト数を出力。

これにより、デコード失敗が多発するバイナリの調査が可能となり、
必要に応じて解析ロジックの改善や対象バイナリの手動検証を行える。
