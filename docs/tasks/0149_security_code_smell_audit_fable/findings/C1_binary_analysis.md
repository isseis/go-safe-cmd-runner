# C1: バイナリ解析パッケージ セキュリティ監査

- 監査日: 2026-07-19
- 対象:
  - `internal/security/binaryanalyzer/` (analyzer.go, network_symbols.go, implicit_system_libs.go, syscall_wrapper_libs.go, doc.go)
  - `internal/security/elfanalyzer/` (standard_analyzer.go, syscall_analyzer.go, x86_decoder.go, arm64_decoder.go, go_wrapper_resolver.go, x86/arm64_go_wrapper_resolver.go, pclntab_parser.go, plt_analyzer.go, syscall table 群, mprotect_risk.go, syscall_store.go ほか)
  - `internal/security/machoanalyzer/` (standard_analyzer.go, analyzer.go, svc_scanner.go, pass1_scanner.go, pass2_scanner.go, pclntab_macho.go, symbol_normalizer.go)
  - `internal/arm64util/` (arm64util.go)
- 方法: 静的コードレビュー（読み取り専用）。呼び出し契約（`BinaryAnalyzer` インターフェース）・fail-open/fail-closed の方向・バイナリパーサの境界条件・整数オーバーフローガードを中心に確認。

## サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 2 |
| 🟠 Low | 4 |
| 🔵 Info | 5 |

このパッケージ群は「ネットワーク能力検出」という検出器であり、直接特権を扱わない。中心的なセキュリティ観点は「検出漏れ（fail-open）を起こさないか」「不正・悪意あるバイナリ入力で panic / 過大メモリ確保 / 無限ループを起こさないか」の2点。全体としてはエラー時に `AnalysisError`（＝ネットワーク能力ありとして扱う）へ倒す fail-closed 設計が一貫しており、境界チェック・整数オーバーフローガードも丁寧。以下は残存する fail-open 余地と堅牢性・保守性の所見。

---

## 所見

### F-1 🟡Medium: syscall analysis store の想定外エラーが fail-open（NoNetworkSymbols）へ縮退する

- 該当箇所: `internal/security/elfanalyzer/standard_analyzer.go:297-332` (`lookupSyscallAnalysis`) と呼び出し元 `:182-195`
- 問題: `LoadSyscallAnalysis` が返すエラーのうち、`ErrHashMismatch` は `AnalysisError`（fail-closed）へ倒すのに対し、`default`（想定外の I/O エラー等）は `slog.Debug` でログした上で `StaticBinary` を返す。呼び出し元では、CGO バイナリ（`.dynsym` が `NoNetworkSymbols` だったケース）でこの `StaticBinary` を受け取ると「ストアにエントリなし」と同一視して `dynOutput`（＝`NoNetworkSymbols`）へフォールスルーする。
- 悪用/再現シナリオ: 静的リンク or CGO バイナリで、record 時にはネットワーク syscall が検出・保存されているが、verify 時にストア読み取りが一過性の I/O エラー（権限・ファイルロック・破損等）で失敗すると、`NetworkDetected` になるべき結果が黙って `NoNetworkSymbols` に縮退する。攻撃者がストア読み取りを失敗させられる状況（キャッシュファイルの配置に干渉できる等）では検出回避に使える余地がある。
- 緩和要因: 直接の網羅検証（`.dynsym` の名前一致）は別経路で機能する。ストアはあくまで「直接 syscall / CGO」の補完。ハッシュ不一致という改ざんの主シグナルは fail-closed に扱われている。
- 推奨対応: `default` エラーケースを `AnalysisError`（fail-closed）へ倒すか、少なくとも `slog.Warn` へ格上げしてサイレントにしない。「キャッシュ欠損（RecordNotFound / nil）」と「読み取り失敗」を意味的に区別する。

### F-2 🟡Medium: CGO オフセット検出（detectOffsetByCallTargets）を悪用した wrapper 範囲ずらしによる syscall 検出回避の余地

- 該当箇所: `internal/security/elfanalyzer/pclntab_parser.go:129-324`、および Mach-O 版 `internal/security/machoanalyzer/pclntab_macho.go:162-288`
- 問題: CGO バイナリの pclntab アドレス補正を、`.text` 先頭 256KB 中の CALL/BL ターゲットと pclntab エントリ差分のヒストグラム投票で推定している。推定オフセットは全 pclntab 関数エントリ（`Entry`/`End`）に一律加算され、その結果が `wrapperRanges`（＝Pass1 で「wrapper 内部なので除外」する範囲）を決定する。オフセットが誤れば wrapper 範囲が実アドレスからずれ、ユーザコード中の本物の `svc`/`syscall` 命令が「wrapper 内部」と誤判定されて解析から除外され得る（＝検出漏れ方向）。
- 悪用/再現シナリオ: 攻撃者が `.text` 先頭領域に、特定の誤オフセットへ `minVotes`(=3) 以上の票が集まり唯一勝者となるような CALL/BL パターンを配置し、実際の syscall 命令を wrapper 範囲に潜り込ませる。生成された誤オフセットは `(0, textSize]` に収まる限り採用される。
- 緩和要因: `minVotes>=3` かつ唯一勝者要求、`(0, textSize]` 範囲制限、非 CGO では offset=0 でそもそも無効化。現実の正規 Go バイナリでは頑健。攻撃には自前バイナリを record させ得る前提が必要で、その場合そもそも脅威モデルが厳しい。未解決 syscall（Number==-1）は最終的に `AnalysisError`（fail-closed）へ倒れるため、「除外」でなく「未解決」に転ぶ限りは安全側。
- 推奨対応: オフセット検出の投票結果に対する追加サニティチェック（例: 補正後エントリが `.text` 範囲に収まる割合の検証）を検討。少なくとも「オフセット誤検出時は解析除外でなく解析対象に含める（誤検出は fail-closed 方向へ倒す）」という不変条件をコメントで明文化する。

### F-3 🟠Low: arm64util.BackwardScanX16 に上限境界チェックがなく、公開関数として OOB panic の潜在リスク

- 該当箇所: `internal/arm64util/arm64util.go:50-109` (`BackwardScanX16`)
- 問題: 同ファイルの `BackwardScanStackTrap`/`backwardScanRegImm` は各反復で `if off < 0 || off+instrLen > len(code)` と上下限をチェックするのに対し、`BackwardScanX16` は `if off < 0` の下限しかチェックせず、`binary.LittleEndian.Uint32(code[off:])` を無検査で呼ぶ。現状の唯一の呼び出し元 `machoanalyzer/pass1_scanner.go:92` は `svcOffset` が 4 バイト境界・`< len(code)` であることを保証しているため OOB には至らないが、これは呼び出し側の不変条件に依存した安全性である。
- 悪用/再現シナリオ: `BackwardScanX16` は `arm64util` パッケージの公開 API。将来別の呼び出し元が非境界整合・`len(code)` を超える `svcOffset` を渡すと `startIdx*instrLen` が範囲外となり `code[off:]` がスライス範囲外 panic を起こす（不正バイナリ経由の DoS）。
- 推奨対応: `BackwardScanStackTrap` と同様に `off+instrLen > len(code)` の上限チェックを追加し、公開関数として入力に対して防御的にする。

### F-4 🟠Low: Mach-O syscall 解析経路（ScanSyscallInfos）に maxFileSize ガードがない

- 該当箇所: `internal/security/machoanalyzer/svc_scanner.go:173-226` (`ScanSyscallInfos`) と `analyzeArm64Slice:99-155`
- 問題: `StandardMachOAnalyzer.AnalyzeNetworkSymbols`（standard_analyzer.go:225）は `fileInfo.Size() > maxFileSize`(1GB) をチェックするが、record 時に使われる `ScanSyscallInfos` はサイズチェックなしで `SafeOpenFile` → `macho.NewFile`/`NewFatFile` → `__text` 全体を `io.ReadAll` する。Fat バイナリでは全スライスを走査する。
- 悪用/再現シナリオ: 巨大な（あるいは巨大 `__text` を主張する）Mach-O を解析させることでメモリ・CPU を消費させる DoS。`section.Size` は Mach-O ヘッダ由来のフィールドで、`io.NewSectionReader` により実ファイル長でクランプされるため青天井ではないが、明示的な上限がない点は `AnalyzeNetworkSymbols` 経路と非対称。
- 推奨対応: `ScanSyscallInfos` にも `AnalyzeNetworkSymbols` と同じ regular-file 判定・`maxFileSize` 上限を適用して経路間の防御を揃える。

### F-5 🟠Low: フラット名前空間 Mach-O で libSystem 非依存だとネットワークシンボルが未検出になる

- 該当箇所: `internal/security/machoanalyzer/standard_analyzer.go:81-104, 326-340`
- 問題: 検出は「libSystem 由来のシンボル」に限定される。two-level namespace では ordinal からライブラリを解決するが、フラット名前空間（全シンボル ordinal=0）の場合は `hasLibSystem = flatNamespace && libs に libSystem を含む` が真のときのみ全シンボルを libSystem 由来として分類する。フラット名前空間かつ imported libraries に libSystem が現れないバイナリでは、`isLibSystemSymbol` も `flatNamespace && hasLibSystem` も偽となり、`socket` 等のネットワークシンボルがあっても記録されず `NoNetworkSymbols` になる（検出漏れ方向）。
- 悪用/再現シナリオ: フラット名前空間でリンクし、ネットワーク関数を libSystem 以外（あるいは libSystem を imported libraries に明示しない形）から解決するよう細工した Mach-O。現代 macOS ではフラット名前空間は稀で、通常のツールチェーンでは発生しにくい。
- 緩和要因: 実運用の一般的ビルドは two-level namespace + libSystem。arm64 では直接 `svc #0x80` 経路が別途 Pass1/Pass2 で検査される。
- 推奨対応: フラット名前空間時のフォールバック方針（libSystem 非依存でも undefined シンボルを名前ベースで最低限分類する等）を検討、または既知の検出限界としてドキュメント化。

### F-6 🔵Info: elf と macho で knownSyscallImpls（除外スタブ集合）が非対称

- 該当箇所: `internal/security/elfanalyzer/go_wrapper_resolver.go:61-67` vs `internal/security/machoanalyzer/pass1_scanner.go:14-20`
- 問題: ELF 側の `knownSyscallImpls` は `syscall.rawVforkSyscall` / `syscall.rawSyscallNoError` / 各 runtime 変種を含むが、Mach-O 側 `knownMachoSyscallImpls` はこれらを含まない。Mach-O Go バイナリにこれらのスタブが存在すると、その内部 `svc #0x80` がユーザコードの直接 syscall として計上され得る（過検出＝fail-closed 方向なのでセキュリティ実害はないが、誤検出増と挙動非対称）。
- 推奨対応: 両パッケージの除外集合を共通ソース化するか、少なくとも差分理由をコメントで明記して意図的か否かを判別可能にする。

### F-7 🔵Info: musl（VERNEED 非依存）バイナリでは非ネットワーク libc シンボルが syscall_wrapper に分類されない

- 該当箇所: `internal/security/elfanalyzer/standard_analyzer.go:198-285` (`checkDynamicSymbols`, `isLibcLibrary`)
- 問題: `sym.Library` が空になる musl 系バイナリでは Step2（libc シンボルの `syscall_wrapper` 分類）が発火しない。ネットワークシンボルの名前ベース検出（Step1）は VERNEED 有無に依存しないため検出漏れは生じないが、`DetectedSymbols` の内容が glibc と非対称になり、`syscall_wrapper` カテゴリに依存する下流消費者があると挙動差が出る。コード中コメントでも既知の制約として明記済み。
- 推奨対応: 検出（ネットワーク判定）に影響しないため対応不要。下流が `DetectedSymbols` のカテゴリ完全性に依存しないことを確認しておく。

### F-8 🔵Info: maxValidSyscallNumber=500 のハードコード上限

- 該当箇所: `internal/security/elfanalyzer/syscall_analyzer.go:64-69`
- 問題: syscall 番号即値の妥当性上限を 500 に固定。将来 500 超の syscall が追加され、それを即値でセットする直接 syscall があると「範囲外＝indirect 扱い」となり Number=-1 に落ちる。ただし Number=-1 は最終的に `AnalysisError`（fail-closed）へ倒れるため安全側。過検出方向でありセキュリティ上の穴ではないが、将来のカーネル追随で保守が必要な定数。
- 推奨対応: syscall テーブル生成スクリプトから上限も自動導出する等で乖離を防ぐ。現状は fail-closed なので緊急性なし。

### F-9 🔵Info: analyzeSlice が defer Close 前に個々のスライスをクローズしないなど、Fat 解析のリソース保持（軽微）

- 該当箇所: `internal/security/machoanalyzer/standard_analyzer.go:141-183` (`analyzeAllFatSlices`)、`svc_scanner.go:204-212`
- 問題: Fat バイナリの各スライス `fat.Arches[i].File` は個別に Close されず、親 `fat.Close()` の defer に委ねられる。スライス数は Fat ヘッダ由来で通常少数のため実害はないが、`macho.NewFatFile` が返す各 `*macho.File` の内部リソース保持がループ完了までまとめて残る。
- 推奨対応: 現状問題なし。将来 Fat スライス数上限の明示や個別クローズを検討する程度。

### F-10 🔵Info: network_symbols レジストリは POSIX socket/DNS のみ（設計上の意図）

- 該当箇所: `internal/security/binaryanalyzer/network_symbols.go:26-82`
- 問題（情報）: OpenSSL/libcurl 等の高レベルプロトコルシンボルは意図的に非登録で、それらは内部で socket/connect/getaddrinfo を呼ぶため dynlib 依存解析で推移的に検出する設計。この前提（＝依存ライブラリ側の解析が確実に走る）が崩れると検出漏れになるため、dynlib 解析側（C2 監査対象）との結合が検出網羅性のキーになる。単体では所見でなく、パッケージ間の前提として記録。

---

## 観察された良好な防御層

- **fail-closed の徹底**: ELF/Mach-O とも、ファイルオープン失敗・stat 失敗・パース失敗・シーク失敗は一律 `AnalysisError` を返し、`IsNetworkCapable()` が真になる（＝ネットワーク能力ありとして扱う）。`standard_analyzer.go:90-196`, `machoanalyzer/standard_analyzer.go:196-285`。
- **未解決 syscall / mprotect の安全側倒し**: `convertSyscallResult`（standard_analyzer.go:337-374）は不明 syscall 番号（-1）や mprotect PROT_EXEC の否定不能（`SyscallArgEvalExecUnknown`）を検出すると、たとえネットワーク syscall が同時検出されていても `AnalysisError` を優先する。`mprotect_risk.go` も exec_unknown をリスク扱い。
- **ハッシュ不一致の改ざんシグナル保持**: syscall ストアの `ErrHashMismatch` は「record 後にバイナリが差し替えられた」として `AnalysisError` に倒す（standard_analyzer.go:307-315）。
- **TOCTOU/symlink 対策**: 全経路で `safefileio.SafeOpenFile` を用い、開いた `io.ReaderAt`（fd）をそのまま `elf.NewFile`/`macho.NewFile` に渡して再オープンを避け、TOCTOU 窓を排除（doc コメントにも明記）。
- **入力サイズ・種別の事前検証**: regular file 判定（デバイス/FIFO/ソケット/ディレクトリ拒否）と maxFileSize(1GB) 上限（syscall スキャン経路を除く、F-4 参照）、パース前のマジックナンバー検査。
- **Fat バイナリの全スライス解析**: `analyzeAllFatSlices` は全アーキスライスを走査し最悪結果を採用。良性スライスの裏に悪性スライスを隠す攻撃を防止（standard_analyzer.go:134-183）。
- **整数オーバーフローの網羅的ガード**: アドレス計算（RIP/ADRP 相対、PLT stub アドレス、pclntab オフセット加算、分岐ターゲット）で `math.MaxInt64`/`MaxUint64` 境界を検査し、負値・ラップを弾く。x86/arm64 デコーダ、`plt_analyzer.go`、`pclntab_parser.go` 全般。
- **デコーダ不変条件の明示 panic**: 成功デコードで `inst.Len <= 0` は「デコーダ実装バグ」として即 panic（不正データを黙って進めない）。
- **バックワードスキャンの有界化**: `maxBackwardScan`/ウィンドウサイズ/`MaxDecodeFailureLogs` により、巨大・難読化バイナリでも走査量とログ量が有界。arm64 の `discoverTransparentWrappers` はスライディングウィンドウで O(1) メモリ。
- **レジストリの防御的コピー**: `GetNetworkSymbols()` は内部マップのコピーを返し外部改変を防止。
- **prefix マッチの厳密化**: `matchesKnownPrefix` はバージョン区切り（`.`/`-`/数字）を要求し、`libpythonista` が `libpython` に誤マッチするのを防ぐ。
