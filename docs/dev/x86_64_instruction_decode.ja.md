# x86_64 命令デコードの技術詳細

本ドキュメントは、ELF 機械語解析による syscall 静的解析機能における
x86_64 命令デコードの技術的な動作仕様と設計判断を記録する。

## 1. 解析の全体構造

`SyscallAnalyzer` は x86_64 と arm64 の両アーキテクチャを `MachineCodeDecoder`
インタフェースで抽象化している。x86_64 向け実装は `X86Decoder` 構造体が担う。

解析は 2 つのパスで構成される。

```
Pass 1: findSyscallInstructions
  → .text セクションを前向きスキャンして SYSCALL 命令 (0F 05) の位置を列挙
  → 各 SYSCALL 命令に対して backwardScanX86WithRegCopy で syscall 番号を解決

Pass 2: FindWrapperCalls (X86GoWrapperResolver)
  → .text セクションを前向きスキャンして Go syscall ラッパーへの CALL を検出
  → 直前の命令列を後向きスキャンして第 1 引数 (RAX/EAX) の値を解決
```

Pass 1 で検出された SYSCALL 命令が既知のラッパー関数・実装関数の内部に
位置する場合は、番号を静的に決定できないためスキップする。

## 2. デコード失敗時の動作

### 2.1 1バイトスキップによる再試行

命令デコードに失敗した場合、`InstructionAlignment()` が返す値（x86_64 では 1）
だけ位置を進めて再試行する。

```
失敗位置: pos
次の試行位置: pos + 1
```

これは x86_64 の可変長命令（1〜15バイト）に起因する制約である。
可変長命令では「次の正しい命令境界」を確実に見つける方法がないため、
1バイトずつ進めて次のデコード可能な位置を探す設計としている。

### 2.2 再同期メカニズム

`.text` セクションは通常ほぼ全てが有効な命令で構成されているため、
デコード失敗後も数バイト以内で正常な命令境界に再同期する。

```
例: 命令境界がずれた場合
実際の命令列: [5バイト命令][3バイト命令][2バイト命令]
ずれた開始:      ^ここから開始
              → 1-3バイト程度のデコード失敗後、正常な命令境界に到達
```

実用上、再同期は x86_64 コードにおいて通常 1〜3 バイト以内で完了する。
最悪ケース（15バイトの不正デコード）は稀であり、
後続の命令がデコードされれば正確性に影響しない。

### 2.3 誤検出リスク

デコード失敗後の再同期過程で命令境界がずれた場合、
偶然 `0F 05` パターンがデータ領域内に現れると、
誤って SYSCALL 命令として検出する可能性がある。

ただし、この場合も逆方向スキャンで syscall 番号の解析を試みるため、
不正な SYSCALL 命令の場合は以下のいずれかとなる。

- 妥当な syscall 番号が見つからない → `unknown:*` として High Risk 判定
- 偶然妥当な番号に見える → 誤検出（理論上のリスクだが、実用上は極めて稀）

## 3. ウィンドウベースのデコード

### 3.1 decodeWindow の役割

Pass 1 の後向きスキャンでは、SYSCALL 命令直前のバイト列全体を前向きデコード
して命令列を再構成する。セクション全体を毎回デコードする代わりに、
SYSCALL 命令の直前 `maxBackwardScan × maxInstructionLength` バイトのみを
ウィンドウとして切り出す (`decodeWindow`)。

```
defaultMaxBackwardScan  = 50 命令
maxInstructionLength    = 15 バイト (x86_64 の命令長上限)
ウィンドウサイズ (最大) = 50 × 15 = 750 バイト
```

### 3.2 ウィンドウ開始位置のずれ

ウィンドウの開始位置は固定バイト数を引いた位置であり、
命令境界と一致しているとは限らない。このため、ウィンドウ先頭付近では
デコード失敗が発生する可能性があるが、後向きスキャンは命令列の末尾
（SYSCALL 直前）から行うため、実用上の影響は小さい。

## 4. 逆方向スキャンによる syscall 番号解決

### 4.1 全体フロー（backwardScanX86WithRegCopy）

```
backwardScanX86WithRegCopy
  ↓
  [1] backwardScanX86WithWindow（初期ウィンドウ）
       → 結果が確定 or 完全な失敗 → 返却
       → x86_copy_chain_unresolved の場合:
           [2] resolveX86CopyChainTailConsensus（末尾コンセンサス探索）
                → 成功 → 返却
           [3] デコード失敗があった場合: 代替ウィンドウ開始位置を探索
                for candidateStart in [windowStart+1 .. windowStart+maxInstructionLength]:
                    backwardScanX86WithWindow(candidateStart)
                    → immediate → 返却
```

### 4.2 単一ウィンドウでのスキャン（backwardScanX86WithWindow）

`scanX86SyscallRegInBlock` が命令列を末尾から走査し、`x86ScanResult` に
結果を蓄積する。

| フィールド | 意味 |
|---|---|
| `foundImmediate` | RAX への即値ロード命令を発見 |
| `immediateValue` | 発見した即値 |
| `sawRegCopy` | レジスタコピー（MOV EAX, EDX など）を経由した |
| `hitControlBoundary` | 制御フロー命令（JMP / CALL / RET など）に到達 |
| `needPredResolution` | コピーチェーン追跡中に制御フロー境界に達した |
| `hasCopyInstAddr` / `copyInstAddr` | コピー命令のアドレス |
| `indirectSetting` | 間接書き込み（メモリ参照・レジスタ間接など） |
| `targetReg` | 追跡中のレジスタ（コピーで更新される） |

スキャン中にレジスタコピー（MOV EAX, EDX）を検出した場合は `targetReg` を
コピー元レジスタに切り替えて追跡を続ける。

### 4.3 CFGベース分岐収束解析（resolveX86RegAcrossPreds）

`x86_copy_chain_unresolved` と判定された場合（コピーチェーン追跡中に
制御フロー境界に達した場合）、以下の処理を行う。

1. `buildX86Successors` でウィンドウ内の命令列から後続グラフを構築する。
2. `resolveX86RegAcrossPreds` が前向きデータフロー解析（ワークリスト法）を実行する。
3. 各命令の入力状態をレジスタ値のマップとして伝播し、分岐点では値の交差
   （両辺の値が一致すれば既知、不一致なら未知）でマージする。
4. SYSCALL 命令の仮想ノード（`virtualEnd`）での入力状態が既知であれば解決成功。

複数の前駆ノードから同一の値が合流する場合は `DeterminationDetailX86BranchConverged`
を付与する。

### 4.4 末尾コンセンサス探索（resolveX86CopyChainTailConsensus）

SYSCALL 命令直前 128 バイトの範囲で候補開始位置を 1 バイトずつずらしながら
`backwardScanX86WithWindow` を実行し、`immediate` として解決できた全ケースが
同一の値を返す場合にその値を採用する。値が一致しない候補が存在する場合は
失敗とする（誤解析を避けるため）。

### 4.5 有効な syscall 番号の検証

即値が得られた場合でも、`maxValidSyscallNumber = 500` を超える値または
負の値は `unknown:indirect_setting` として扱う。

## 5. RIP 相対グローバル変数解決

### 5.1 対象パターン

Go の syscall パッケージでは、syscall 番号をパッケージレベル変数に格納する
ケースがある（例: `syscall.fcntl64Syscall` を参照する `forkAndExecInChild1`）。
このパターンでは以下の命令が生成される。

```asm
MOV RAX, [RIP + disp32]    ; RIP 相対メモリ参照で RAX にロード
SYSCALL
```

### 5.2 ResolveFirstArgGlobal の動作

`X86Decoder.ResolveFirstArgGlobal` は Pass 2（Go ラッパー解析）の
後向きスキャン中に呼ばれ、以下の手順で値を解決する。

1. 命令が `MOV RAX/EAX, [RIP + disp32]` 形式かを確認する。
2. `x86RIPRelAddr` で実効アドレス `nextPC + sign_extend(disp32)` を計算する
   （`x86asm` が disp32 を ゼロ拡張で保持するため `int32` に再解釈して符号拡張する）。
3. `SetDataSections` で登録した ELF セクション（`.noptrdata`, `.rodata`, `.data`）
   から当該アドレスの値を読み取る（4バイトまたは 8バイトのリトルエンディアン整数）。

## 6. 判定メソッドと詳細コード

### 6.1 DeterminationMethod 定数

| 定数 | 意味 |
|---|---|
| `immediate` | 即値から直接決定 |
| `go_wrapper` | Go ラッパー呼び出しの引数から決定 |
| `unknown:decode_failed` | 命令デコード失敗により不明 |
| `unknown:control_flow_boundary` | 制御フロー境界に到達して不明 |
| `unknown:indirect_setting` | 間接的な設定（メモリ参照・レジスタ間接など）で不明 |
| `unknown:scan_limit_exceeded` | スキャンステップ上限（`defaultMaxBackwardScan = 50`）に到達 |
| `unknown:window_exhausted` | ウィンドウ内の全命令を消費したが見つからなかった |
| `unknown:invalid_offset` | SYSCALL 命令のオフセットが不正 |

### 6.2 DeterminationDetail 定数（x86_64 固有）

| 定数 | 意味 |
|---|---|
| `x86_copy_chain` | レジスタコピーチェーン経由で解決 |
| `x86_branch_converged` | 分岐収束点での CFG 解析により解決 |
| `x86_copy_chain_unresolved` | コピーチェーン追跡が未解決のまま終了 |
| `x86_indirect_write` | 間接書き込みを検出（detail 付き `unknown:indirect_setting`） |

## 7. 設計判断の根拠

### 7.1 デコード失敗を High Risk としない理由

1. **Pass 1 の解析対象は直接 SYSCALL 命令**であり、デコード失敗は
   SYSCALL 命令自体の検出には影響しにくい。SYSCALL 命令は `0F 05` の
   2バイト固定であり、デコード失敗がこの 2バイトパターンの検出精度に
   直接影響することは少ない。

2. **デコード失敗が多発するケースは稀**であり、過度に High Risk 判定を
   行うと実用性が低下する。`.text` セクションは通常有効な命令で構成されて
   おり、デコード失敗はデータ領域の混入やアライメント不一致など
   特殊なケースに限られる。

3. **Pass 2（Go ラッパー解析）でのデコード失敗**も、必ずしも
   syscall ラッパー呼び出しの見落としを意味しない。CALL 命令は
   正常にデコードされることが多く、周辺のデコード失敗が
   CALL ターゲットの解決に影響する可能性は低い。

### 7.2 安全側への設計原則との整合性

本設計では「検出できない syscall 番号」を High Risk とする（FR-3.1.4）。

- **デコード失敗**: 「命令自体を認識できない」ケース
- **syscall 番号不明**: 「SYSCALL 命令は認識できたが番号を特定できない」ケース

この 2つは異なる性質の問題であり、区別して扱う。

- SYSCALL 命令が正常にデコードされた場合に番号が不明 → **High Risk**
- デコード自体の失敗 → **ログ出力のみ**（リスク分類に影響しない）

`unknown:*` メソッドの結果（番号が不明な SYSCALL 命令）は
High Risk として扱われる（§8.5 / §9.1.2）。
デコード失敗カウントはリスク分類に影響しない。

### 7.3 デコード失敗の可視化

デコード失敗は `DecodeStatistics` 構造体で統計情報を収集し、
以下のログ出力で可視化する。

- **個別ログ**: Pass 1 (`findSyscallInstructions`) および
  Pass 2 (`FindWrapperCalls`) の両方で `slog.Debug` により出力。
  Pass 1・Pass 2 それぞれで出力件数を `MaxDecodeFailureLogs`（= 10）で制限。
- **サマリログ**: record コマンドで `slog.Debug` によりファイルパス、
  デコード失敗総数（Pass 1 + Pass 2 の合計）、解析バイト数を出力。

これにより、デコード失敗が多発するバイナリの調査が可能となり、
必要に応じて解析ロジックの改善や対象バイナリの手動検証を行える。
