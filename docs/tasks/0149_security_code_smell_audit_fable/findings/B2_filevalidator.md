# B2: filevalidator / pathencoding パッケージ セキュリティ監査

- 対象: `internal/filevalidator/`（`*_test.go` を除く）, `internal/filevalidator/pathencoding/`
- 監査日: 2026-07-18
- 監査方法: ソースコードの静的読解（読み取り専用、コード修正なし）

対象ファイル:

- `internal/filevalidator/validator.go` (1973 行)
- `internal/filevalidator/errors.go`
- `internal/filevalidator/hash_algo.go`
- `internal/filevalidator/hybrid_hash_path_getter.go`
- `internal/filevalidator/sha256_path_hash_getter.go`
- `internal/filevalidator/test_helpers.go`（test ビルドタグ付き）
- `internal/filevalidator/pathencoding/substitution_hash_escape.go`
- `internal/filevalidator/pathencoding/errors.go`

## 所見サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 2 |
| 🟠 Low | 6 |
| 🔵 Info | 5 |

---

## 🟡 Medium

### B2-1: SaveRecord 時にハッシュ計算と各解析が別々のファイル読み取りで行われ、記録時レースで「ハッシュと解析結果の不整合」が生じうる

- 該当箇所:
  - `internal/filevalidator/validator.go:388-411`（`saveRecordCore`: `calculateHash` → `updateAnalysisRecord`）
  - `internal/filevalidator/validator.go:365-382`（`SaveRecord`: `resolveShebangInfo` がさらに先行して別読み取り）
  - `internal/filevalidator/validator.go:532-562`（`analyzeRecordTarget`: dynlib / シンボル / syscall 解析がそれぞれ path 指定で再オープン）
- 説明: `SaveRecord` のフローでは、(1) shebang 解析、(2) `calculateHash` によるハッシュ計算、(3) `analyzeDynLibDeps` / `binaryAnalyzer.AnalyzeNetworkSymbols` / `analyzeELFSyscalls` / `analyzeMachoSyscalls` の各解析が、いずれも **同一パスを独立に open して読む**。単一の fd やメモリ上の内容を共有していないため、読み取りの合間にファイルが差し替えられると、ContentHash が指す内容と、SyscallAnalysis / SymbolAnalysis / DynLibDeps が記述する内容が食い違ったレコードが永続化される。
- 悪用シナリオ: 対象パスへの書き込み権限を持つ攻撃者が、管理者による `record` 実行のタイミングを狙い、解析フェーズには無害なバイナリを見せ、ハッシュ計算の瞬間だけ悪性バイナリに差し替える（またはその逆順）。結果として「悪性バイナリのハッシュ + 無害バイナリの解析結果」を持つレコードが作られ、後続の verify はハッシュ一致で成功し、リスク評価（ネットワークシンボル・syscall 検出）は実体を過小評価する。前提として記録時に攻撃者が対象ファイルを書き換えられる必要があるため（通常 record は信頼できる環境で行う運用前提）、影響は限定的と判断し Medium。
- 推奨対応: ハッシュ計算と解析を単一の open（`SafeOpenFile` で得た fd / 読み込んだバイト列）から行うか、少なくとも全解析完了後にハッシュを再計算して一致を確認し、不一致なら記録を失敗させる（fail-closed）。

### B2-2: 解析器が無効な再記録時に、旧コンテンツ由来の解析結果が新ハッシュのレコードへ引き継がれる

- 該当箇所: `internal/filevalidator/validator.go:469-477`（`populateAnalysisRecord` の既存フィールド温存ロジック）
- 説明: `SymbolAnalysis` / `SyscallAnalysis` / `AnalysisWarnings` は、対応する解析器が nil（無効）のとき既存レコードの値をそのまま温存する。ファイル内容が変わって `ContentHash` が更新されても、旧内容に対する解析結果が新レコードに残るため、「ハッシュは新ファイル、解析メタデータは旧ファイル」というレコードが正規のフローで生成される。docstring 上は意図的な仕様（"preserves existing fields"）だが、解析結果がリスク評価に使われる以上、内容とメタデータの整合性が壊れるのはセキュリティ上望ましくない。
- 悪用シナリオ: 解析器フル構成で無害な旧バージョンを record → その後、解析器を構成しない呼び出し経路（`ValidatorConfig` のゼロ値）で悪性の新バージョンを force 再 record。新レコードは無害版のシンボル/syscall 解析を保持したまま verify を通過し、下流のリスク評価が誤誘導される。record 操作自体が特権的である点で緩和されるが、運用ミス一つで成立する。
- 推奨対応: 温存する場合は「解析結果が対応する ContentHash」をレコードに併記し、不一致時は温存しない（または警告を残す）。あるいは解析器なしでの ContentHash 更新時は既存解析フィールドを破棄する。

---

## 🟠 Low

### B2-3: analyzeOneLibrary のサイズ検査と実解析の間に TOCTOU（stat 後にパスで再オープン）

- 該当箇所: `internal/filevalidator/validator.go:713-776`（`analyzeOneLibrary`）
- 説明: ライブラリファイルを `SafeOpenFile` で open → `Stat` でサイズ検査 → **close** した後、`binaryAnalyzer.AnalyzeNetworkSymbols(lib.Path, ...)` や `openELFFile(v.fileSystem, lib.Path)` がパス指定で再オープンする。検査時と解析時でファイルが同一である保証がなく、`maxFileSize`（1GB）のサイズ上限もすり抜け可能（検査後に巨大ファイルへ差し替え → 解析側でのメモリ/時間消費）。また、この関数は `lib.Hash` を受け取るが解析前に現内容がそのハッシュに一致するかを検証しないため、ハッシュをキーに保存される解析結果（`dynamicanalysis.Store` / `processedLibAnalysis`）が実際には別内容の解析である可能性がある。
- 悪用シナリオ: record 時にライブラリパスへ書き込める攻撃者が、サイズ検査と解析の間に差し替えを行い、解析スキップ（DoS 側）またはハッシュキーと不一致な解析結果の混入（B2-1 と同系統の不整合）を起こす。
- 推奨対応: open した fd を使い回して Stat と解析を行う。ハッシュキー付き解析では、解析対象バイト列から実測ハッシュを計算しキーと照合する。

### B2-4: pathencoding.ErrInvalidPath の Unwrap がポインタレシーバのため errors.Is が機能しない

- 該当箇所:
  - `internal/filevalidator/pathencoding/errors.go:74-81`（`Error()` は値レシーバ、`Unwrap()` は `*ErrInvalidPath` レシーバ）
  - `internal/filevalidator/pathencoding/substitution_hash_escape.go:67`（値型 `ErrInvalidPath{...}` で return）
- 説明: `Encode` は `ErrInvalidPath` を**値**として返すが、`Unwrap` はポインタレシーバで定義されているため、値型のメソッドセットに含まれず、`errors.Is(err, ErrEmptyPath)` が **false** になる。errors.go の docstring 自身が推奨している `errors.Is(err, encoding.ErrEmptyPath)` パターンが実際には動作しない。現状 sentinel 判定に依存する呼び出し元は確認できなかったが、将来「空パスなら無視」等の分岐を書いた際に静かに素通りする（エラーハンドリングの罠）。CLAUDE.md の方針（`errors.Is` によるエラー判定）とも不整合。
- 再現: `_, err := encoder.Encode(""); errors.Is(err, pathencoding.ErrEmptyPath)` → false。
- 推奨対応: `Unwrap` を値レシーバにする、またはエラーを `&ErrInvalidPath{...}` で返す（`Error()` とレシーバを統一）。

### B2-5: os.IsNotExist によるラップ済みエラーの誤判定リスク（Verify / VerifyWithHash）

- 該当箇所: `internal/filevalidator/validator.go:1215-1221`, `1241-1247`
- 説明: `calculateHash` の失敗を `os.IsNotExist(err)` で判定して生エラーを返す分岐があるが、`os.IsNotExist` は `errors.Is` と異なり `Unwrap` チェーンを辿らない。`SafeOpenFile` 実装が `%w` でラップしたエラーを返すと not-exist 判定が false になり、呼び出し元（dry-run の失敗理由分類など）が「ファイル不存在」を汎用エラーとして誤分類しうる。fail-open にはならない（検証は失敗する）ため Low。
- 推奨対応: `errors.Is(err, fs.ErrNotExist)` へ置き換える（リポジトリ方針とも一致）。

### B2-6: Mach-O の ImportedSymbols 失敗を警告なしで空扱いし、解析カバレッジが静かに低下する

- 該当箇所: `internal/filevalidator/validator.go:1581-1588`（`extractMachoSliceInfo`）
- 説明: `mf.ImportedSymbols()` のエラー（Symtab 欠落など）を握りつぶして空スライスにフォールバックする。stripped バイナリでは正当だが、それ以外のパースエラーも同一経路で「インポートなし」となり、libSystem 経由の syscall 検出が丸ごと skip される。`AnalysisWarnings` にも記録されないため、レコードを見ても解析が縮退したことが分からない（解析面の fail-open）。
- 悪用シナリオ: Symtab を意図的に破損させた Mach-O バイナリを用意すると、シンボルベースの syscall/ネットワーク検出を回避しつつ record/verify を通過できる（直接 svc スキャン等の他レイヤは残るため部分的）。
- 推奨対応: `elf.ErrNoSymbols` 相当（Symtab なし）のケースのみ静かに許容し、それ以外のエラーは `AnalysisWarnings` に記録するか失敗させる。

### B2-7: 破損したハッシュレコードが「新規レコード」として警告なく上書きされ、改ざんの痕跡が消える

- 該当箇所: `internal/filevalidator/validator.go:420-439`（`updateAnalysisRecord` の Update コールバック。コメント "An empty FilePath means the record is new (not found or was corrupted)"）
- 説明: `Store.Update` は破損レコードを空レコードと同様に扱うため、`SaveRecord` は `force=false` でも破損レコードを黙って上書きする（`ErrHashFileExists` にならない）。verify 側は破損レコードで fail-closed になるので検証バイパスはないが、record 側では「ハッシュディレクトリ内のレコードが壊されていた」というセキュリティ上意味のあるシグナルが無警告で消える。
- 推奨対応: 破損レコード検出時は少なくとも警告ログを出す（可能なら force なしでは上書きしない）。

### B2-8: Validator 内部キャッシュ（map）が同期なしで、並行 SaveRecord でデータレース

- 該当箇所: `internal/filevalidator/validator.go:168-170, 824-848, 852-869`（`processedLibAnalysis`, `processedInterpreterAnalysis` の遅延初期化と読み書き）
- 説明: これらの map はロックなしで読み書きされる。現状の呼び出し元（`cmd/record`）は逐次実行だが、Validator 自体に「並行使用不可」の明示がなく、将来 goroutine 化した場合にレース（クラッシュ、キャッシュ破損による解析結果混線）となる。
- 推奨対応: doc comment で単一 goroutine 前提を明示するか、`sync.Mutex` / `sync.Map` で保護する。

---

## 🔵 Info

### B2-9: Verify はハッシュ検証と後続実行の間の TOCTOU を構造的に持つ（既知・アーキテクチャ由来）

- 該当箇所: `internal/filevalidator/validator.go:1203-1254`（`Verify` / `VerifyWithHash`）
- 説明: パス指定の verify → 後で同パスを実行、という利用形態では検証と実行の間の差し替えを本パッケージ単体では防げない（fdexec 等の上位対策の領分）。コンテンツを返す用途には `VerifyAndRead` が「読んだバイト列そのもの」をハッシュ検証しており TOCTOU-free。責務分担として妥当だが、`Verify` の docstring に「検証時点の保証であること」を明記するとよい。

### B2-10: SHA256 フォールバックのファイル名が 72bit（base64 12 文字）に切り詰められている

- 該当箇所: `internal/filevalidator/sha256_path_hash_getter.go:57-65`
- 説明: 72bit への切り詰めは誕生日衝突が約 2^36 計算で可能であり、攻撃者制御のロングパス同士を衝突させうる。ただし (1) フォールバックはエンコード名が 250 文字を超えるパスのみ、(2) 衝突時は `record.FilePath` 照合（`validator.go:425-431`, `1266-1270`）で `ErrHashFilePathCollision` として fail-closed になるため、実害は DoS（該当パスの record/verify 不能）に限られる。将来フォーマット変更の機会があれば切り詰め長の拡大を推奨。

### B2-11: pathencoding.Decode は本番コードから未使用で、非正規入力に対して寛容（非単射）

- 該当箇所: `internal/filevalidator/pathencoding/substitution_hash_escape.go:147-225`
- 説明: `Decode` はテスト以外に呼び出し元がない。また末尾の孤立 `#` や `#x` などエンコーダが生成しない列をエラーにせず素通しするため、`~a#` と `~a#1` が同じ `/a#` に復号されるなど非単射（例: 復号結果をキーにした処理を将来書くと曖昧性が生じる）。現状は Encode 側が単射でありセキュリティ影響なし。使うなら非正規列をエラーにする、使わないなら削除を検討。

### B2-12: findLibcEntry は "libc.so." プレフィックスのみで、musl 等の libc を検出しない

- 該当箇所: `internal/filevalidator/validator.go:1807-1816`
- 説明: musl（`libc.musl-x86_64.so.1`, `ld-musl-*.so.1`）などは検出されず、libc インポート由来の syscall 検出が黙って skip される（直接 syscall スキャンは残る）。Alpine 系バイナリを扱う場合は解析カバレッジ低下として認識しておくこと。

### B2-13: deferredErr 付き Validator の一部アクセサはガードされていない / 大文字小文字非区別 FS での挙動

- 該当箇所: `internal/filevalidator/validator.go:126-134`（`HashFilePath`, `Store`）
- 説明: `NewReadOnly` 経由で deferredErr を持つ Validator では `Store()` が nil を返し、呼び出し側で nil デリファレンスの余地がある（`HashFilePath` は空 hashDir → `ErrEmptyHashDir` で安全）。また macOS 既定の大文字小文字非区別 FS ではエンコード名がケース違いで同一ファイルに衝突しうるが、`record.FilePath` 照合により fail-closed（`ErrHashFilePathCollision`）となることを確認した。いずれも現状実害なし。

---

## 観察された良好な防御層

1. **symlink-safe なファイルオープン**: ハッシュ計算・ELF/Mach-O 解析とも `safefileio.SafeOpenFile` 経由で、パス解決も `common.NewResolvedPath`（`EvalSymlinks`）で正規化してから行う（`validator.go:1282-1311`）。
2. **regular file 限定**: `validatePath` が `Lstat` + `Mode().IsRegular()` でデバイスファイル・FIFO 等を拒否し、読み取りブロッキングや特殊ファイル経由の攻撃面を閉じている（`validator.go:1289-1295`）。
3. **TOCTOU-free な VerifyAndRead**: 返却するバイト列そのものをハッシュ検証しており、検証と利用の間の差し替えが不可能（`validator.go:1315-1354`）。
4. **ハッシュファイルパス衝突の fail-closed 検出**: レコード内に元パス（`FilePath`）を保持し、record 時・verify 時の双方で照合。フォールバックエンコーディングの衝突が検証バイパスではなくエラーになる（`validator.go:425-431`, `1266-1270`）。
5. **エンコーディング名前空間の分離**: 通常エンコードは必ず `~` 始まり（絶対パス前提）、SHA256 フォールバックは base64url + `.json` で `~` を含まず、両者のファイル名空間が交差しない（`hybrid_hash_path_getter.go`, `sha256_path_hash_getter.go`）。
6. **依存ライブラリのハッシュ不一致は即エラー**: `depCollector.addEntry` が同一パスに異なるハッシュが来た時点で `errDependencyHashMismatch` で fail-fast（`validator.go:894-918`）。
7. **fat Mach-O のスライス選択が fail-closed**: 未知の GOARCH では `ErrUnsupportedGOARCH` で解析中止し、誤スライスの黙認を防ぐ（`validator.go:1547-1560`）。
8. **ELF 判定はマジックバイト前置チェック**: Go バージョン依存のエラー分類に依存せず magic を直接検査（`validator.go:1744-1778`）。
9. **再帰 shebang の拒否**: インタプリタ自身が shebang スクリプトの場合は `ErrRecursiveShebang` で拒否し、解釈チェーンの無限延伸を防ぐ（`validator.go:597-630`）。
10. **解析サイズ上限**: 1GB 上限（`maxFileSize`）で解析リソースを制限（B2-3 の TOCTOU はあるが上限自体は妥当）。
11. **NewReadOnly の deferred-error 設計**: ハッシュディレクトリ不在/アクセス不能を握りつぶさず、各 Verify 呼び出しで確実にエラーを返す fail-closed 設計。ガード漏れ時の nil パニックを避けるためのフィールド初期化も配慮されている（`validator.go:279-349`）。
12. **決定的な出力順序**: 依存・警告・シンボル・syscall のソートにより、レコードが再現可能で diff ベースの改ざん検知や監査に向く。
