# 詳細仕様書への申し送り（02 から移した詳細）

本書は設計書ではない。[02_architecture.md](02_architecture.md) から移した詳細（実際のコード、構築関数の一覧、箇所ごとの部分の並び、行番号、バイト単位の手順、ガードの判定の仕方、テストの細則）を集めたものであり、詳細仕様書（Task 0091 の命名に従い `03_detailed_specification.md`）と、関係する場合は実装計画を書くときの入力として使う。本書には承認の状態がない。本書の内容と 02 が食い違う場合は、02 に従う。

- 実装計画のファイル名: [design_carryover.md](design_carryover.md) と 01 は `03_implementation_plan.md` と呼んでいる。0091 の番号付けでは詳細仕様書が `03` になるので、実装計画は `04_implementation_plan.md` になる。どちらにするかは詳細仕様書を作るときに決め、参照をそろえる。→ [03_detailed_specification.md](03_detailed_specification.md) §0 で `04_implementation_plan.md` に確定した。
- 行番号は、特に断らない限りコミット `3bb634bd` のものである。詳細仕様書を書く時点で確かめ直す。
- 用語は 02 の §0 に従う（部分・断片・置換文字列・値全体置換など）。

## errmsg の型の内部表現

02 §1.2・§3.1.1 から移した内部表現の案である。02 が決めるのは、`Part` の欄が非公開で、パッケージの外で役割を選べないことだけである。

```go
// causeKind declares how a cause part is flattened. The zero value is a
// plain cause.
type causeKind int

// Part is one element of a Message. Its fields are unexported, so a part
// can only be built by the constructors in this package.
type Part struct {
    role      Role
    text      string
    cause     error
    causeKind causeKind
}

// Message is an error body as a sequence of parts.
type Message struct {
    parts []Part
}

// Segment is one flattened element: a role and its unredacted text.
type Segment struct {
    Role Role
    Text string
}

// Segments is the result of flattening a Message.
type Segments struct {
    List []Segment
}

// Summary is the summary line of a report: a constant or free text only.
type Summary struct {
    part Part
}

// Error is the general-purpose structured error used in place of
// fmt.Errorf on the paths in scope.
type Error struct {
    msg   Message
    cause error
}
```

- 02 の初版は `Part` に字下げの文字列の欄（`indent`）を持たせていた。字下げは errmsg が持つ固定の文字にしたので（「GroupError の整形の再現手順」）、この欄は置かない。字下げの有無は `causeKind` で表す。
- `Segments` は断片の列だけを持つ。深さの上限を置かないので、切り詰めの有無の欄は無い。`[]Segment` そのものにしてよい。
- `Segment` の欄は公開である。redaction の側が役割で分岐するために読む。範囲外の役割の値を直接与えるテストに使える（「テストの細則」）。

## errmsg の公開 API（構築関数・メソッド・シグネチャ）

02 §3.1.1 から移した一覧である。02 が決める契約（役割は errmsg の構築関数でしか決まらない、構造化メッセージを構造を保って組み合わせられる、`errmsg.Error` は nil でない原因をちょうど 1 つラップする、nil の原因は panic しない、`String()` は redaction 前の描画、`Const` は定数式だけ）を満たす形で、詳細仕様書で確定する。

```go
func Const(text string) Part                        // text must be a constant expression (AST guard)
func Ident(name string) Part
func Path(path string) Part
func Text(text string) Part
func Cause(err error) Part
func PathErrorCause(err error) Part
func IndentedCause(err error) Part                  // continuation-line indent is errmsg's fixed two spaces

func NewMessage(parts ...Part) Message
func (m Message) String() string
func (m Message) LogValue() slog.Value
func (m Message) Segments() Segments

func ConstSummary(text string) Summary              // text must be a constant expression (AST guard)
func TextSummary(text string) Summary
func (s Summary) String() string
func (s Summary) Part() Part

func NewError(parts ...Part) *Error                 // panics unless exactly one part is a cause part with a non-nil cause
func (e *Error) Error() string                      // returns e.StructuredMessage().String()
func (e *Error) Unwrap() error
func (e *Error) StructuredMessage() Message
```

- `IndentedCause` は字下げの引数を取らない。02 の初版は `IndentedCause(err, indent string)` だったが、呼び出し側が渡す文字列が免除の役割の断片に入りうるので、引数を除いた（02 §5.1 の「免除の役割の断片に入るバイトの出どころ」）。
- `Const` と `ConstSummary` の引数は定数式でなければならない（AC-24）。errmsg の中で作る `Constant` の部分（`PathErrorCause` の区切りの `" "`・`": "`、`IndentedCause` の字下げ）は、パッケージ自身の文字列リテラルから作るので、ガードの対象外である。
- `NewError` は、原因の部分がちょうど 1 つで、その原因が nil でないことを要求する。満たさなければ呼び出し側の誤りとして panic する。先頭に番兵のエラーを置く書式（`fmt.Errorf("%w: ...", ErrX, ...)`）は、`Cause(ErrX)` を先頭の部分にして表す。
- `Cause(nil)` などの nil の原因は panic しない。平らにすると、`fmt` の `%v` が nil のエラーに対して出す文字列 `<nil>` を 1 つの `Text` の断片にする。
- `String()` は平らにした断片の文字列をそのまま連結する。`LogValue()` は `String()` を `slog.StringValue` で返す。
- 名前: 構造化メッセージを返すものを `NewMessage`、エラーを返すものを `NewError` とし、`errors.New` と取り違えないようにする。
- 構造化メッセージの組み合わせ（未設計）: `GroupErrors` は各 `GroupError` の構造化メッセージを、平らにせずに改行の `Constant` でつないで 1 つの構造化メッセージにする（02 §3.1.1 の契約 2、§3.4.1）。02 の初版はこれを「各 `GroupError` の部分を直接並べる」と書いたが、`Part` の欄は非公開で、`Message` から部分の列を取り出す公開の API も無かった。組み合わせる API（`Message` を部分として受け付ける構築関数、`NewMessage` の可変長の引数に `Message` を渡す形など）を詳細仕様書で決める。組み合わせは `Message` の全体の単位だけで行い、`Message` から部分の列を返すメソッドは置かない（02 §3.1.1 の契約 8）。組み合わせた `Message` が役割を外から選べる経路にならないこと（部分の欄を公開しないこと）を保つ。

## 平らにする処理の細則

02 §3.1.3 から移した細則である。

- `*fs.PathError` の分け方: `PathErrorCause` で作った原因の部分は、原因が `*fs.PathError`（型アサーションで判定する）なら、`Op` を `Text`、区切りの空白を `Constant`、`Path` を `Path`、`": "` を `Constant`、`Err` を原因の部分として平らにする。この結果は `(*fs.PathError).Error()` と同じ文言になる。`*fs.PathError` でなければ、`Cause` と同じに扱う。
- 型アサーションで足りる理由: `os.MkdirTemp` と `os.Chmod` は、失敗したときに `*fs.PathError` を直接返す（Go 標準ライブラリの `os/tempfile.go` の `MkdirTemp` と `os/file_posix.go` の `Chmod`）。
- 深さ: 上限は置かない（02 §3.1.3）。`Structured` を実装する型の `Error()` は `StructuredMessage().String()` なので、原因の `Error()` を呼ぶと平らにする処理に再び入る。そのため、自身を原因に持つエラーではスタックがあふれる。変更前に `fmt` の `%v` で同じ連鎖を表示したときと同じである。

## GroupError の整形の再現手順

02 §3.1.4 から移した手順である。`GroupError.Error()` は、原因の文言の末尾の `"\r\n"` の並びを除き、残りの改行の後に 2 つの空白を入れる（`internal/runner/group_errors.go` の `(*GroupError).Error`）。`IndentedCause(err)` は、この整形を原因の断片の列に施す。

- 字下げの文字: errmsg が持つ固定の 2 つの空白である。`IndentedCause` は字下げの引数を取らない。呼び出し側が渡す文字列が、字下げとして `Identifier`・`Path`・`Constant` の断片に入らないようにするためである（02 §5.1）。
- 末尾の除去: 原因の断片の列全体の末尾に対して行う。末尾の断片が改行だけなら、その断片ごと除き、その前の断片の末尾の改行も続けて除く。途中の断片の末尾の改行は除かない。`strings.TrimRight(s, "\r\n")` と同じく、`\r` と `\n` の任意の並びを除く。
- 字下げの位置: 各断片の文字列の中の改行の後に入れる。役割は変えない。
- 範囲: 整形は原因の断片にだけ施し、`IndentedCause` の外の部分には施さない。
- 一致: 整形の後の断片の連結は、`GroupError.Error()` の文言と一致する。
- `Constant` の断片に字下げが入ると、その文字列は定数式だけから作ったものではなくなる。入るのは errmsg が持つ固定の空白と、元の断片にあった改行だけであり、呼び出し側のバイトは入らない。そのため AC-24 の制約（定数式）の例外として扱い、02 §3.8.2 の「呼び出し側のバイトが整形で断片に入らない」の不変条件で守る。
- テスト: 末尾の除去が断片をまたぐ場合（末尾の断片が改行だけ、`\r\n` の並び）、途中の断片の改行、`Identifier`・`Path` の断片の中の改行、原因が構造を持たない場合。どれも `GroupError.Error()` と比べる。字下げの引数が API に無いことは、シグネチャで固定される。

## 範囲を返す処理の細則

02 §3.2.1 から移した細則である。

- 既存の `RedactText` の段: PEM ブロックの置換（`maskPrivateKeyBlocks`）、key=value などの規則（`compiled`）、値形式の検出（`ValueDetector.Mask`）を、この順に適用する（`internal/redaction/redactor.go:272-307`、`value_detector.go:150-194`）。各段は正規表現の一致を探し、一致のうち残す範囲を除いた区間を置換文字列に置き換える。
- 案のシグネチャ:

  ```go
  // byteRange is a half-open byte range [start, end) of the original text.
  type byteRange struct {
      start, end int
  }

  // redactedRanges returns the ranges of text that RedactText replaces, in
  // the coordinates of text, sorted and non-overlapping.
  func (c *Config) redactedRanges(text string) []byteRange
  ```

- `RedactText` を使う箇所（差し替えない理由の根拠）: `internal/redaction/redactor.go` の `Handle`、`internal/runner/base/security/logging_security.go`、`internal/runner/base/audit/logger.go` など。
- 規則の共有: 正規表現は `Config` と `valueDetectorPatterns` にあるものを参照し、写しを作らない。値形式の検出の各段の残す範囲を `redactedRanges` から参照できる形にする変更は、`value_detector.go` のパッケージの中に留める。
- 残す範囲: 各段の置換のテンプレートが再び出力する範囲を「残す範囲」とする。前置きのグループ（`${1}` など。`Bearer `、key と区切り、`"private_key_id":"` など）と後ろのグループ（`gcpSAKey` の `${2}`、`jwt` の `${1}`）がこれに当たる。`urlCred` が一致の末尾の `@` を文字列リテラルとして出し直すのは、一致の最後の 1 バイトを残す範囲とみなす。
- 段の重なり: 後の段は、前の段が置換文字列を入れた後の文字列に対して一致を探す。後の段の置き換える範囲が、前の段の置換文字列と一部でも重なれば、その置換文字列に当たる元の範囲の全体を置き換える範囲に加える。残す範囲が前の段の置換文字列と重なっても、その置換文字列に当たる元の範囲は置き換えたままとする（置換文字列は分けない）。
- 表し方: 範囲は、一致の数に比例する大きさの区間の列で表す。文字列の長さに比例する表は作らない。
- 幅が 0 の範囲: 置き換える区間が空の一致（`password=""` の引用の間など）でも、`RedactText` は置換文字列を入れる（`internal/redaction/redactor_test.go:404` が `password=""` → `password="[REDACTED]"` を固定している）。そのため `redactedRanges` は `start == end` の範囲（挿入点）を返すことがある。挿入点は捨てずに返し、「sorted and non-overlapping」の定義に含める（同じ位置の挿入点と幅のある範囲の並べ方も決める）。描画での扱いは「RedactMessage の描画手順」の手順 4・6。
- `WithPlaceholder` の削除（02 §3.2.1 の前提条件）: 本番のコードで呼ぶ箇所は無く、使うのは `internal/redaction/redactor_test.go` の次の 3 か所だけである（コミット `d429b663` で確認）。
  - `:1728`（`placeholder reaches both redaction layers` の subtest）と `:3767`（`the placeholder option reaches the configured-host pattern` の subtest）: オプションを確かめるためだけのテストなので削除する。
  - `:3352`（`TestRedactText_ValueBasedDetection_BypassWhenNil`）: 置換文字列は付随的な使い方なので、`NewConfig()` の既定の置換文字列に替え、期待値を `password=[REDACTED] value AKIAIOSFODNN7EXAMPLE` に改める。
  - 削除の後、`go tool cover -func` の結果が関数ごとに変わらないことを確かめる（CLAUDE.md「Deleting a test is a claim that must be checked」）。`WithPlaceholder` 自体の行は消える。ほかの関数に差があれば（例: `internal/redaction` のテストでは `:1730` だけが呼ぶ `Placeholder()`）、その関数に届くテストを残すか加える。
- 差分のファジング: `RedactText` を基準にして、`redactedRanges` の範囲を置換文字列に置き換えた結果と比べる。既存の `RedactText` のテストの入力の全体を種にする。残す範囲の種類（前置き、後ろのグループ、`urlCred` の `@`）と段の重なりの入力を含める。`DefaultPlaceholder` がどの規則の一致にも関与しないことを固定するため、PEM ブロックの直後に `[0-9A-Z]{16}` の文字列を続けた入力と、PEM ブロックの直後にほかの規則の前置き（`Bearer ` など）を続けた入力も種に含める。空の引用の値（`password=""`・`"password":""`・`password=''`）も種に含める（幅が 0 の範囲）。
- `WithWebhookHost` の規則と置換文字列: 種に、設定した webhook のホストの URL を置換文字列の隣に置いた入力（例: `https://hooks.example.com/[REDACTED]`、`[REDACTED]https://hooks.example.com/x`）を含める。この規則は `\bhttps://` から一致を始め、パスの文字の集合は `[` を含まない（`internal/redaction/value_detector.go:125` の `compileWebhookHostPattern`）。そのため一致は `[REDACTED]` の中で始まることも終わることもない。
- `WithAdditionalKeyValuePatterns` の削除（02 §3.2.1 の前提条件）: 本番のコードで呼ぶ箇所は無い。使うのは `internal/redaction/redactor_test.go` の次の 5 か所だけである（コミット `89dbc8b1` で確認）。
  - `:645`（`TestKeyBoundaryGroup_Classification/user-added key is redacted with the loose boundary`）: 利用者が足すキーの境界を固定するテストで、足す経路が無くなるので削除する。
  - `:654`（同じ関数の `zero value of PatternKind is the key kind`）: `Kind` のゼロ値が key の規則になることは残す価値がある。`applyPattern`（`:48`）で `KeyValuePattern{Literal: "passphrase"}` を直接コンパイルする形に書き換える。
  - `:1574`（`TestPerformKeyValueRedaction/unknown kind never reaches the redaction path`）: `NewConfig` を通す後半を削除し、`compilePattern` での拒否（`:1571`）だけを残す。
  - `:1712`・`:1721`（`TestNewConfig_RejectsInvalidPatterns` の 2 つの subtest）: 足す経路の検証なので削除する。パターンの検証そのものは `TestKeyValuePattern_Validate` に残る。`WithPlaceholder` の subtest（`:1728`）も削除するので、関数ごと消える。
  - 削除の後、`go tool cover -func` の結果が関数ごとに変わらないことを確かめる。`NewConfig` の中の、パターンの検証とコンパイルが失敗したときに返す分岐は、既定の規則だけでは届かなくなる。これらの分岐を残すか（既定の規則が不正な形に編集されたときの拒否の境界として。`TestDefaultKeyValuePatterns_AreValid` と役割が重なる）、除くかを決め、`NewConfig` の網羅率の差をその判断として記録する。

## RedactMessage の描画手順

02 §3.2.2 から移した手順である（処理の流れの図は 02 §6.1）。

- 案のシグネチャ（戻り値の形は「RedactingHandler・RedactLogAttribute の分岐の置き場所と戻り値の形」で確定した）:

  ```go
  // RedactMessage renders m with per-segment redaction and the cross-boundary
  // contract, using this Config's rules and replacement string. It returns an
  // error when flattening m panics or when the runtime range check fails.
  func (c *Config) RedactMessage(m errmsg.Message) (string, error)
  ```

1. `m.Segments()` を 1 回だけ呼び、断片の列を得る。redaction 前の描画結果 S は、その断片の文字列の連結とする。各断片の範囲も同じ列から求める。`Error()` を 2 回呼ぶと、結果が変わる原因のエラーで範囲がずれるためである。
2. 各断片に、役割ごとの redaction を適用する（01 決定事項「役割と適用する redaction」）。
   - `Identifier`・`Constant`: そのまま出す。
   - `Path`: 断片の文字列に `RedactText` を適用する。
   - `Text`（どの役割にも当たらない値を含む）: 断片の文字列に `RedactText` を適用し、変化がなければ `IsSensitiveValue` で判定して、当たれば断片全体を置換文字列にする。値全体置換は断片ごとに判定する（AC-36）。
3. S に `redactedRanges` を適用し、全体の検出範囲を得る。範囲を置換文字列に置き換えた結果が `RedactText(S)` と一致するかを確かめ、一致しなければエラーを返す（実行時の検査）。
4. `Identifier` 以外の断片のうち、全体の検出範囲に含まれるのに手順 2 では置き換えられないバイトを持つ断片を「境界の影響を受ける断片」とする。断片の外の文字列があって初めて検出される秘密は、この断片に現れる。
   - 幅が 0 の範囲（挿入点。「範囲を返す処理の細則」）は、その位置を含む断片を境界の影響を受ける断片にする。ただし手順 2 がその断片で同じ位置に置換文字列を入れていれば影響は無い。
   - 挿入点が断片の境界にあるときは、隣の `Identifier` でない断片に付ける。両隣がどちらも `Identifier` の断片なら、置換文字列はその 2 つの断片の間に出す。`Identifier` のバイトは隠さない。
5. 境界の影響を受けない断片は、手順 2 の結果をそのまま出す。
6. 境界の影響を受ける断片では、次のバイトを「隠すバイト」とする: 全体の検出範囲に含まれるバイト、手順 2 で置き換えられたバイト、手順 2 で値全体置換に当たった断片のすべてのバイト。`Identifier` の断片のバイトは隠さない（AC-37）。隠すバイトが連続する区間は、極大の区間ごとに 1 つの置換文字列にする。区間は、隣り合う境界の影響を受ける断片をまたいでよい。挿入点は、隣接する隠すバイトがあればそれと合わせて 1 つの極大の区間にし、無ければ幅が 0 の区間として 1 つの置換文字列を出す。
   - テスト: `Constant("password=\"")`・`Text("")`・`Constant("\"")` の 3 つの断片を与え、出力が `RedactText("password=\"\"")` と同じ `password="[REDACTED]"` になることを確かめる。挿入点は空の `Text` の断片の位置（2 つの `Constant` の境界）にあり、どの断片も単独では検出されない。

- 非公開の関数: 手順 2〜6 は、断片の列（`[]errmsg.Segment`）を受け取る非公開の関数で行う。`RedactMessage` は手順 1 の後にこの関数を呼ぶ。AC-38 のテストはこの関数に範囲外の役割を持つ断片を与える。
- 手順 5 の分け方を採る理由の詳細: 境界をまたぐ検出が無いときまで隠すバイトによる描画を使うと、置換文字列の数が断片単独の `RedactText` の結果と変わることがある（隣り合う 2 つの置き換えが 1 つの置換文字列にまとまるなど）。AC-04・AC-11 は、境界をまたぐ検出が無い断片では断片単独の結果と同じになることを求めている。
- `Config` が `NewConfig` を経ていない場合は、`RedactText` と同じく `RedactionFailurePlaceholder` を返す。
- 性能の確かめ方: 1 回の描画は、断片ごとの `RedactText` と `IsSensitiveValue`、全体の `redactedRanges` と `RedactText` からなり、S の長さのおよそ 3 倍の `RedactText` に当たる。02 の予算（100 group の失敗を連結した最終の実行エラー、数十 KiB、の描画 1 回につき 10 ms 以下）を、`BenchmarkRedactText`（`internal/redaction/redactor_test.go:3569`）に並べたベンチマークで確かめる。

## RedactingHandler・RedactLogAttribute の分岐の置き場所と戻り値の形

02 §3.2.3 から移した細則である。

- 分岐の置き場所: `RedactingHandler.redactLogAttributeWithContext`（`internal/redaction/redactor.go:799`）で、宣言済みの識別子の判定（`declaredIdentifier`、`:308-319`）と並べる。`Config.RedactLogAttribute`（`:330`）にも同じ分岐を置く。判定と描画は 1 つの補助関数にまとめる。
- `Config.RedactLogAttribute` は本番のコードから呼ばれていない（`:371` の自身の再帰だけ）。
- 失敗の記録: `RedactingHandler` は、`RedactMessage` が返したエラーを、既存の `ErrLogValuePanic` と同じく型付きのエラーとして `ErrorCollector` に記録する（`processLogValuer` と同じ形。`:854-872`）。そのため、終了時の報告（`ShutdownReporter`）に現れる。
- 戻り値の形: `RedactMessage` は `(string, error)` を返す。エラーは失敗（panic・範囲の不一致）だけを表す。深さの上限を置かないので、切り詰めの報告は無い。

## PreExecutionError・ExecutionError の変更の細則

02 §3.3.1・§3.3.2 から移した細則である。

```go
type PreExecutionError struct {
    Type                ErrorType
    Message             errmsg.Summary // was string
    Component           string
    RunID               string
    NotificationContext common.NotificationContext
    FailedFilePaths     []string
    Err                 error
}

// DetailMessage returns Message followed by the cause, as a structured message.
func (e *PreExecutionError) DetailMessage() errmsg.Message

// Detail returns DetailMessage().String().
func (e *PreExecutionError) Detail() string

type ExecutionError struct {
    Message     errmsg.Summary // was string
    Component   string
    RunID       string
    GroupName   string
    CommandName string
    Err         error
}

// ReportMessage returns the message HandleExecutionError reports:
// Message, the group/command context, and the cause.
func (e *ExecutionError) ReportMessage() errmsg.Message
```

- 要約文の渡し方: 定数式は `errmsg.ConstSummary(...)`、値を含めて作るものは `errmsg.TextSummary(fmt.Sprintf(...))`。
- `DetailMessage()`: `Err` が nil なら `Message.Part()` だけ、そうでなければ `Message.Part()`・`Const(": ")`・`Cause(Err)`。
- `PreExecutionError.Error()` の書式（`%s: %s: %v (component: %s, run_id: %s)`、`internal/logging/pre_execution_error.go:82-87`）は、`Message.String()` を使って変更前と同じ文言を返す。
- `Message` を設定する本番の箇所: 23 か所（`cmd/runner/main.go` 15、`internal/runner/bootstrap` 6、`internal/runner/runerrors` 1、`internal/runner/group_stage.go` 1 の、`PreExecutionError{` のリテラル。`grep` で数えた）。すべて機械的に書き換える。段階の定義表の要約文（`internal/runner/group_stage.go:67-98`）は `ConstSummary` にする。
- `HandleExecutionError`（`internal/logging/pre_execution_error.go:243-276`）が文字列で組み立てている本文（`Message (group: g, command: c): 原因`）を `ReportMessage()` に移す。
- `ExecutionError.Message` の唯一の設定箇所（`cmd/runner/main.go` の `executeRunner` の `error running commands`）は `ConstSummary` にする。
- context の部分の列は 1 つの非公開のメソッド（案: `contextParts`）で作り、`ReportMessage()` と `ContextString()` の両方がそれを使う。group 名・コマンド名は `Ident`。
- 変更するファイル: `internal/logging/pre_execution_error.go`・`execution_error.go`、`Message` のリテラルを書き換える `cmd/runner/main.go`・`internal/runner/bootstrap/config.go`・`environment.go`・`internal/runner/runerrors/pre_execution.go`・`internal/runner/group_stage.go`。

## 記録箇所の変更点

02 §3.3.3 から移した細則である。

- `errorRecordParams.errorMsg` の型を `errmsg.Message` に変える。`writeErrorLogRecord` は `error_message` を `slog.Any(key, msg)` で記録する（変更前は `slog.String`、`internal/logging/pre_execution_error.go:187`）。
- 凍結の操作（02 §3.3.3 の「1 回だけ平らにする」）: errmsg に、`Message` を 1 回平らにして、平らにした断片だけを持つ `Message` を返す操作を置く（案: `func (m Message) Freeze() Message`）。断片の役割は平らにした結果のものをそのまま移すので、役割を errmsg の外で選ぶ経路にはならない（02 §3.1.1 の契約 1）。凍結した `Message` は原因を持たないので、`String()`・`Segments()` は原因の `Error()` を呼ばない。
- 凍結する位置: `handleErrorCommon` が報告ごとに 1 回凍結し、stderr の `Details:` には凍結した `Message` の `String()` を使い、記録にも凍結した `Message` を渡す。そのため、`RedactMessage` の 1 回の `Segments()` の呼び出しは原因の `Error()` を評価しない。記録だけの `NotifyPreExecutionError`（`internal/logging/pre_execution_error.go:237-239`）も同じ扱いにそろえるなら、凍結を記録の組み立て（`preExecutionRecordParams`・`HandleExecutionError`）へ移す。どちらにするかは詳細仕様書で決める。
- 原因の `Error()` の panic は凍結の時点で起きる。変更前に `preExecutionRecordParams` の `Detail()`（`:215`）や `HandleExecutionError` の組み立てで起きていたのと同じく、報告は回復しない（02 §4.2）。
- テスト: 呼ぶたびに文言が変わる原因を持つ構造化メッセージで報告し、stderr の `Details:` と記録された `error_message` の redaction 前の文字列が同じ 1 回の結果から来ること（原因の `Error()` が 1 回だけ呼ばれること）を確かめる。
- `preExecutionRecordParams` は `DetailMessage()` を、`HandleExecutionError` は `ReportMessage()` を渡す。
- 通知ビルダー（`buildPreExecutionError`、`internal/logging/slack_handler.go:841`）は変えない。

## internal/runner のエラー型の部分の並び

02 §3.4.1 から移した並びである。

| 型 | 構造化メッセージ |
|---|---|
| `GroupStageError` | `Cause(err)` だけ。`err` が nil のゼロ値は `Const("group pre-execution failed")`（`internal/runner/group_stage.go:138-143` と同じ文言） |
| `GroupError` | `Const("failed to execute group ")`・`Ident(group)`・`Const(": ")`・`IndentedCause(err)` |
| `GroupErrors` | 各 `GroupError` の構造化メッセージを、間に `Const("\n")` を置いてつなぐ（組み合わせる API は「errmsg の公開 API」の組み合わせの項で決める） |
| `CommandExecutionError` | `Const("command ")`・`Ident(CommandName)`・`Const(" in group ")`・`Ident(GroupName)`・`Const(" failed: ")`・`Cause(Err)` |

- `CommandExecutionError` の欄は公開なので、`Err` が nil の値もありうる（テストのリテラルなど）。`Cause(nil)` は `<nil>` の `Text` になり、`Error()` は変更前の `%v` と同じ文言を返す。
- 各型のゼロ値の `Error()` が panic しないことをテストで確かめる。

## group_executor.go の箇所ごとの部分の並び（出発点の一覧）

02 §3.4.2 から移した一覧である。網羅は AC-41 のガードが担うので、この表は出発点であり、完全であることを保証しない。`fmt.Errorf` による `%w` のラップは 11 か所あった。

| 箇所（行） | 部分 |
|---|---|
| group の展開（`:169`） | `Const("failed to expand group[")`・`Ident(group)`・`Const("]: ")`・`Cause` |
| group の作業ディレクトリ（`:186`） | `Const("failed to resolve work directory: ")`・`Cause` |
| コマンドの事前展開（`:317`） | `Const(...)`・`Ident(group)`・`Const(...)`・`Ident(command)`・`Const("] (index ")`・`Text(index)`・`Const("): ")`・`Cause` |
| コマンドの作業ディレクトリ（`:334`） | `Const("failed to resolve workdir: ")`・`Cause` |
| 権限監査の到達しない分岐（`:374`） | `Cause(errUnhandledCheckSkipReason)`・`Const(": ")`・`Text(reason)`・`Const(" for path ")`・`Path(p)` |
| 権限監査の違反（`:388`） | `Cause(ErrDirPermViolation)`・`Const(" for group[")`・`Ident(group)`・`Const("]: ")`・`Text(件数)`・`Const(...)` |
| パス解決（`:435`） | `Const("command path resolution failed for ")`・`Path(strconv.Quote(path))`・`Const(": ")`・`Cause` |
| 依存検証（`:456`） | `Const("command dependency verification failed for ")`・`Path(strconv.Quote(path))`・`Const(": ")`・`Cause` |
| 環境変数の検証（`:511`）・出力パスの検証（`:521`） | `Const(...)`・`Cause` |
| 終了コード（`:671`） | `Cause(ErrCommandFailed)`・`Const(": command ")`・`Ident(command)`・`Const(" failed with exit code ")`・`Text(exit code)` |

- `%q` のパスは、`Path` の部分に `strconv.Quote` した文字列を入れる。`%q` と同じ引用とエスケープになる。
- 番兵の例: `ErrCommandFailed` は `command failed`（`internal/runner/runner.go:35`）。どの番兵も機密を示す語を含まない。
- `ExpandWorkDir` の呼び出し（`:686`・`:723`）は、`fmt.Sprintf("group[%s]", ...)` の代わりに `config` の `Level` を渡す。

## cancelledRunError の細則

02 §3.4.3 から移した細則である。

- 置き換える箇所: `executeGroups` の `errors.Join(ctxErr, err)`（`internal/runner/runner.go:452`）。作る箇所はこの 1 か所だけで、`ctxErr` も `err` も nil でないことを確かめた後である（`:440-453`）。
- 構造化メッセージ: `Cause(ctxErr)`・`Const("\n")`・`Cause(err)`。
- 欄の案: `ctxErr error`（`ctx.Err()`）、`err error`（`ExecuteGroup` が返した失敗した group のエラー）。
- 0177 のガード: `TestProductionCodeDoesNotProbeMultiErrorShape`（`internal/runner/group_errors_guard_test.go:327-348`）は、具体的な型に `Unwrap() []error` を宣言することを許し、形による判定を禁じている。
- `cmd/runner/main.go` の `executionErrorContext` は `errors.AsType` で `*runner.CommandExecutionError` を探すので、判定の結果は変わらない。
- `GroupStageError.Error()` が原因の文言だけを返す根拠: `internal/runner/group_stage.go:138-143`。

## config の Level・Field の型と移行する関数

02 §3.5.1 から移した細則である。

```go
// Level is where a value was being expanded. The zero value is "no level"
// and renders as the empty string.
type Level struct {
    kind levelKind
    name string
}

// Field is the configuration field being expanded. The zero value is
// "no field" and renders as the empty string.
type Field struct {
    key      fieldKey
    name     string // variable name, for vars fields only
    index    int
    hasIndex bool
}

func (l Level) String() string       // "", "global", "group[<name>]", "command[<name>]", "template[<name>]"
func (l Level) parts() []errmsg.Part // Const("group["), Ident(name), Const("]") etc.
func (f Field) String() string       // "", "cmd", "args[0]", "vars.<name>", "vars.<name>[0]", ...
func (f Field) parts() []errmsg.Part
```

- ゼロ値: `HasVariableReference` は空の `level` と `field` で `processVarRefs` を呼ぶ（`internal/runner/config/expansion.go:92-104`）ので、このゼロ値で表す。index の有無は `hasIndex` で表し、値に番兵（`-1` など）を使わない。
- 種類: `levelKind` は global・group・command・template の 4 つと無し。template は `template_expansion.go` の `template[<name>]`（`processVarRefs` の呼び出し元、`internal/runner/config/template_expansion.go:686`・`:1114`）に使う。`fieldKey` は、`processVarRefs`・`ExpandString` のすべての呼び出し元が使うキー（`cmd`・`args`・`env`・`env_vars`・`env_import`・`workdir`・`verify_files`・`cmd_allowed`・`vars`）と無し。キーの一覧は呼び出し元をたどって確定し、単体テストで、変更前に `fmt.Sprintf` で作っていた文字列と `String()` が一致することを確かめる。
- 構築: `config` パッケージの中だけで使う関数は非公開にする（`globalLevel()`・`groupLevel(name)`・`varField(name)` など）。`group_executor.go` が `ExpandWorkDir` に渡す `Level` だけは、公開の `GroupLevel(name)`・`CommandLevel(name)` で作る。
- `parts()` は非公開である（02 §3.1.1 の契約 8）。呼び出し側は `config` パッケージの中（`ErrUndefinedVariableDetail.StructuredMessage` と `ExpandWorkDir`）だけである。
- `parts()` の文言: 種類ごとの `switch` で、各文言を `Const` の文字列リテラルから作る。キーの文言を変数から `Const` に渡すと、AST のガードが定数式でないとして拒否するためである。呼び出し側は、`parts()` の結果をほかの部分と `slices.Concat` でつないで `NewMessage`・`NewError` に渡す。
- 引数の型の変更: `level string` を受け取る関数（`ExpandString`・`ProcessVars`・`ProcessEnv`・`ProcessEnvImport`・`resolveAndExpand`・`processVarRefs`・`newVarExpander` などの 13 個）の引数を `Level` に、`field string` を受け取る関数の引数を `Field` に変える。
- 変数の定義側の名前を `Field` に入れる箇所（`expandVarsWithLazyResolution` の `vars.%s`・`vars.%s[%d]`、`:724`・`:740`）は、`varField`・`varElementField` で作る。
- `Level` の文字列を使うほかのエラー型（`ErrCircularReferenceDetail` など）は、`level.String()` を保持する。これらは #1197 の対象なので構造化しない。
- 変更するファイル: `internal/runner/config/expansion.go`・`errors.go`・`template_expansion.go`。

## ErrUndefinedVariableDetail の部分の並び

02 §3.5.2 から移した細則である。

```go
type ErrUndefinedVariableDetail struct {
    Level        Level  // was string
    Field        Field  // was string
    VariableName string
    Context      string
    Chain        []string
}

func (e *ErrUndefinedVariableDetail) Error() string // returns e.StructuredMessage().String()
func (e *ErrUndefinedVariableDetail) StructuredMessage() errmsg.Message
func (e *ErrUndefinedVariableDetail) Unwrap() error // ErrUndefinedVariable, unchanged
```

- 構造化メッセージ: `Const("undefined variable in ")`・`Level.parts()`・`Const(".")`・`Field.parts()`・`Const(": '")`・`Ident(VariableName)`・`Const("' (context: ")`・`Text(Context)`・`Const(")")`。`Chain` が空でなければ、`Const(" (expansion path: ")`・各変数名の `Ident` を `Const(" -> ")` でつないだもの・`Const(")")` を続ける。
- 変更前の文言の定義: `internal/runner/config/errors.go:262-268`。

## expansion.go のラップの分類（出発点の一覧）

02 §3.5.3 から移した一覧である。網羅は AC-41 のガードが担う。

- `ErrUndefinedVariableDetail` を作る箇所: `expansion.go:128`・`:425`。
- `ProcessEnv` の `env_vars` の拒否のエラー: `:784`。
- 名前と固定の文言を宣言するラップの例: `failed to process global vars: %w`（`:857`）は `Const("failed to process global vars: ")`・`Cause`。`failed to process group[%s] vars: %w`（`:1023`）は `Const("failed to process group[")`・`Ident(name)`・`Const("] vars: ")`・`Cause`。
- 前置きの全体を 1 つの `Text`（`errmsg.Text(fmt.Sprintf(...))`）にして `Cause(err)` を続けるラップ: `env_import` の処理の失敗（`:847`・`:1011`・`:1136`）、`cmd_allowed` の空のパスとパスの解決の失敗（`:919`・`:948`）、`Runtime*` の作成の失敗（`:832`・`:982`・`:1226`）。
- `expandCmdAllowed` の `:925` のラップ: `cmd_allowed` の展開の未定義変数は `ExpandString`（`:923`）からこのラップを通る。group 名は `Ident`、index は `Text`、展開前の値（`rawPath`）は生のテンプレートなので `Text`。
- `ExpandCommand` はテンプレートの解決のエラーをラップせず、`resolveAndPrepareCommandSpec` のエラーをそのまま返す（`:1219`）。
- `ExpandCommand` の `failed to create RuntimeCommand for command[%s]: %w` は、design_carryover.md ではコマンド名を `Identifier` とする候補だった。上の分類では `Runtime*` の作成の失敗として前置きの全体を `Text` にしている。この違いは、原因が `ErrUndefinedVariableDetail` を運びうるかをたどって詳細仕様書で確定する。

## ExpandWorkDir の変更の細則

02 §3.5.4 から移した細則である。

```go
func ExpandWorkDir(workdir string, expandedVars map[string]string, level Level) (string, error)
```

- 変数の展開の失敗（`expansion.go:58`）: `Const("failed to expand workdir: ")`・`Cause(err)`。
- 相対パスの拒否（`:63-64`）: `Level.parts()`・`Const(": ")`・`Cause(ErrInvalidWorkDir)`・`Const(": ")`・`Path(strconv.Quote(expanded))`・`Const(" (relative paths are not allowed for security reasons)")`。
- 呼び出し側（`internal/runner/group_executor.go:686`・`:723`）は `GroupLevel`・`CommandLevel` を渡す。

## 一時ディレクトリの 2 つのラップ

02 §3.6.1 から移した細則である。`DefaultTempDirManager.Create` の 2 つのラップは、それぞれ既存の文言を保つ。

| 箇所 | 既存の前置き | 原因 |
|---|---|---|
| `internal/runner/base/executor/tempdir_manager.go:77`（`os.MkdirTemp` の失敗） | `failed to create temporary directory: ` | `PathErrorCause(err)` |
| `internal/runner/base/executor/tempdir_manager.go:90`（`os.Chmod` の失敗） | `failed to set permissions on temporary directory: ` | `PathErrorCause(err)` |

- 02 の初版は 2 つとも `failed to create temporary directory: ` の形にすると書いていた。`:90` の前置きは `failed to set permissions on temporary directory: ` であり、これは誤りだった。02 がコードの持つ文言を書き写していたことが原因なので、02 からは文言を除き、「各ラップは既存の文言を保つ」という契約だけを残した。
- テスト: ラップごとに、`Error()` の文言が変更前の `fmt.Errorf` の結果と同じであることを確かめる。`os.Chmod` の失敗は、`os.MkdirTemp` の失敗とは別に起こして確かめる。
- 一時ディレクトリのパスは group 名を含む（`scr-<group>-`、`:74`）。
- `Cleanup` のラップ（`:116`）は、ログに出すだけで 2 つのレコードの原因にならないので対象にしない。

## コマンドの実行の経路の箇所ごとの部分の並び

02 §3.6.2 から移した一覧である。網羅は AC-41 のガードが担う。

| パッケージ | 関数 | 挿入する値（例） |
|---|---|---|
| `internal/runner/resource` | `(*NormalResourceManager).ExecuteCommand`・`executeCommandWithOutput` | コマンドパス（`normal_manager.go:147`・`:150` の `cmd.ExpandedCmd`）。`:266` は `Cause(err)`・`Const("; and also failed to close output capture: ")`・`Text(closeErr.Error())`、ほかのラップ（`:253`・`:264`・`:290`）は `Const`・`Cause` |
| `internal/runner/resource` | `(*DryRunResourceManager).ExecuteCommand`・`evaluateCommandRisk` | コマンドパス（`evaluateCommandRisk` の `dryrun_manager.go:416`・`:427`・`:441`） |
| `internal/runner/base/executor` | `(*DefaultExecutor).Validate`・`validatePrivilegedCommand`・`executeNormal`・`executeWithUserGroup` | コマンドパス・作業ディレクトリ（`executor.go` の `Validate` など） |
| `internal/runner/base/executor` | `runCommand`・`superviseCommand`・`killChild`・`killOutcome`（`command_lifecycle.go`） | 挿入する名前は無い。`privilege.Error` を運ぶので範囲に入る（下の経路 A・B） |
| `internal/runner/base/privilege` | `(*Error).StructuredMessage`・`(*UnixPrivilegeManager).performElevation` | コマンド名（`Error.CommandName`） |

- `:266` は 2 つのエラー（コマンドの失敗と出力の取り込みの後始末の失敗）を持つ。変更前も後始末の失敗は `%v` で入れていてラップしていないので、`Text(closeErr.Error())` にしても到達性は変わらない。
- 02 §3.8.1 の範囲に入れる規則による分類（コミット `24b0c2f7` で確認。`internal/runner/base` はコミット `3bb634bd` から変わっておらず、下の行番号はどちらでも同じ）:
  - 範囲外: `internal/runner/base/executor/command_lifecycle.go` の `prepareCommand`・`rankedError`、`output_pump.go`・`fdexec_linux.go` のラップ、`executor.go` の `stageFromFD`。原因は OS・標準ライブラリのエラーか executor の番兵であり、`privilege.Error` を運ばない。挿入するのは固定の文言と pid（`stageFromFD` は gid）だけである。
  - `reportStartFailure`（`command_lifecycle.go:618-626`）は範囲に入れた（02 §3.8.1 の表に反映済み）。`runCommand` の `!started` の分岐（`:591`）が渡す `errors.Join(elevErr, closeErr, fdErr)` は、`runCommand` を範囲に入れると宣言した複合の型になり、この型は `Structured` を実装するので、規則 (i) に当たる。この分岐では `privilege.Error` は入らない（窓が開いているので `WithPrivileges` は `fn` の結果を返し、権限の復元の失敗は `emergencyShutdown` に進む）が、規則は字面どおりに当てはめる。
    - `:619` の `errors.Join(startErr, pc.release())` は、同じ宣言した複合の型にする。
    - `:625` の `fmt.Errorf("command execution failed: %w", combinedErr)` は `Const("command execution failed: ")`・`Cause(combinedErr)` にする。
    - 文言は変わらない（AC-18）。`:620-624` のログの `error` 属性の値も、同じ文言の複合の型になる。
    - テスト: 2 つの呼び出し元の経路のそれぞれで、`Error()` の文言が変更前の `fmt.Errorf("command execution failed: %w", errors.Join(startErr, releaseErr))` と同じであること、`errors.Is` が変更前と同じ対象に届くことを確かめる。1 つは `pc.spent` の経路（`:557`、`ErrPreparedCommandSpent`。既存の `TestRunCommand_ChildStateTransitions/spent_command_stays_not_started`（`executor_supervise_test.go:433`）が `errors.Is` だけを確かめている）、もう 1 つは開始の失敗の経路（`:591`。既存の `TestExecute_FdBoundStartFailureNoLeak`（`executor_fdexec_test.go:111`）と `TestStartPrepared_StartFailureRemovesStagedCopyInsideWindow`（`executor_lifecycle_test.go:815`）が通る）。
  - #1196・#1197 でこの経路のどれかの型が `Structured` を実装したら、規則 (i) に当たる箇所が変わるので、この分類を確かめ直す。
- `privilege.Error` は範囲に入れる（ユーザーの決定。規則どおりで、例外を設けない）。`Structured` を実装し、`CommandName` を `Ident` にする。
  - 部分の並び（変更前の書式は `internal/runner/base/privilege/errors.go:34-37` の `privilege operation '%s' failed for command '%s' (uid %d->%d): %v`）: `Const("privilege operation '")`・`Text(string(Operation))`・`Const("' failed for command '")`・`Ident(CommandName)`・`Const("' (uid ")`・`Text(strconv.Itoa(OriginalUID))`・`Const("->")`・`Text(strconv.Itoa(TargetUID))`・`Const("): ")`・`Cause(SyscallErr)`。`Timestamp` は描画しない。`SyscallErr` は変更前も `%v` で入れているが、`Unwrap()` が `SyscallErr` を返すので、`Cause` にしても到達性は変わらない。nil の `SyscallErr` は `<nil>` になり、`%v` と同じである。
  - `WithPrivileges`（`internal/runner/base/privilege/unix.go:105-143`）は `performElevation` のエラーをそのまま返す（`:132-134`）。ラップしないので範囲に入れない。`escalatePrivileges` は `&Error{...}` を作って返すだけで、ラップしない（`ErrPrivilegedExecutionNotAvailable` のラップ（`:302`）は構造を持つ原因を運ばず、名前も挿入しない）。
- 経路 A（開始の窓）: run-as の実行で `seteuid(0)` が拒否された場合。
  1. `escalatePrivileges`（`unix.go:316-325`）が `syscall.Seteuid(0)` の失敗で `&Error{...}` を返す。
  2. `performElevation`（`unix.go:217`）の `fmt.Errorf("privilege escalation failed: %w", err)` を `Const("privilege escalation failed: ")`・`Cause(err)` にする。
  3. `WithPrivileges` がそのまま返す。`executeWithUserGroup` の窓の関数（`executor.go:258-274`）は、`fn` が走らないので `opened` が偽のまま、それを `elevErr` として返す。
  4. `runCommand` の `!opened` の分岐（`command_lifecycle.go:583-589`）の `errors.Join(elevErr, closeErr, fdErr, pc.release())` を宣言した複合の型にする。ガードは関数の単位で働くので、同じ関数の `:591`・`:593` の `errors.Join` も同じ型にする。`reportStartFailure` を通らないことに注意する（02 の初版の申し送りは `reportStartFailure` を通ると書いていたが誤りだった）。
  5. `executeWithUserGroup` の `fmt.Errorf("user/group privilege execution failed: %w", err)`（`executor.go:305`）は、すでに範囲の中にある。
  6. `NormalResourceManager` の `executeCommandInternal` はそのまま返し、出力を取り込む場合は `executeCommandWithOutput` の `:266` を経て、`group_executor.go` の `CommandExecutionError`（`:645`・`:668`）の原因になる。
- 経路 B（中断の後の停止の窓）: run-as の子プロセスを中断の後に止めるとき。
  1. `killChild` の `killReelevated` の分岐（`command_lifecycle.go:919-922`）が `WithPrivileges` を呼び、昇格が拒否されると `privilege.Error` を運ぶエラーが返る。
  2. `killOutcome`（`:942`）の `fmt.Errorf("%w: pid=%d: %w", ErrKillAfterCancel, pid, err)` を `Cause(ErrKillAfterCancel)`・`Const(": pid=")`・`Text(strconv.Itoa(pid))`・`Const(": ")`・`Cause(err)` にする。変更前は 2 つの `%w` の両方に `errors.Is` が届くので、両方を保つ。02 §3.1.1 の契約 3 は `errmsg.Error` が原因をちょうど 1 つラップすると定めている。原因を 2 つ持つ `errmsg.Error` の形を加えるのではなく、宣言した複合の型を使う（詳細仕様書で決める。契約 3 を変えるなら 02 を先に直す）。
  3. `killChild` の `ErrNoPrivilegeManager` の分岐（`:906`）も同じ形である。`ErrKillStrategyUnset` の分岐（`:931`）は `Cause(ErrKillStrategyUnset)`・`Const(": pid=")`・`Text(strconv.Itoa(pid))`。
  4. `superviseCommand` の `errors.Join(rankedError(outcome), outcome.killErr, notReapedErr, startupErr)`（`:782`）を宣言した複合の型にする。`fmt.Errorf("command execution failed: %w", cmdErr)`（`:789`）は `Const("command execution failed: ")`・`Cause(cmdErr)`。同じ関数の `notReapedErr`（`:736`）は `Cause(ErrChildNotReaped)`・`Const(": pid=")`・`Text(strconv.Itoa(pid))`。
  5. その後は経路 A の 5・6 と同じく `executor.go:305` を経て届く。
  - 一時コピーの後始末の窓（`removeStagedCopy`、`:806-852`）の失敗は `pc.stagingWindowErr` に記録してログに出すだけで、返すエラーに入らない（`superviseCommand` は `:756` で戻り値を捨てる）。そのため範囲に入れない。
- 複合の型: 子を `Unwrap() []error` で返し、`errors.Join` と同じく子の文言を改行でつなぐ（nil の子は除く。すべて nil なら nil を返す点も `errors.Join` と同じにする）。構造を持たない子は `Text` になるので、文言は変わらない（AC-18）。`cancelledRunError` と共通の型にできるかを詳細仕様書で決める。
- テスト:
  - 2 つの経路のそれぞれで、`Error()` の文言が変更前と同じであること、`errors.Is`・`errors.AsType` が変更前と同じ対象（`*privilege.Error`、`ErrKillAfterCancel` など）に届くこと。
  - AC-41: 検出される語を含むコマンド名が、`error_message` で置き換えられずに出ること。入力は `Identifier` の免除だけが効くものにする。先に、コマンド名を `Text` として redaction すると置き換えられることを確かめる（値全体置換だけが反応する語を使い、key=value・値形式の検出は反応しない形にする）。

## cmd/runner の 4 か所

02 §3.7 から移した細則である。形は `Message: errmsg.ConstSummary("<固定の文言>")` と `Err: err`。

| 箇所（関数） | `Message` |
|---|---|
| global の展開（`run`） | `Failed to expand global configuration` |
| テンプレート検証（`run`） | `Template validation failed` |
| ディレクトリ権限チェッカーの初期化（`auditConfiguredDirPermissions`、`cmd/runner/main.go:512`） | `directory permission checker initialisation failed` |
| `--groups` の指定誤り（`executeRunner`、`cmd/runner/main.go:642`） | `Invalid groups specified` |

- `Detail()` は `Message: Err.Error()` になり、変更前の `fmt.Sprintf("…: %v", err)` と同じ文言である。
- `mainWithExitCode` の分岐の順は `dryRunPreviewExit`・`SilentExitError`・`PreExecutionError`・`ExecutionError` である（`cmd/runner/main.go:219-252`）。付け替えた原因（`config.ExpandGlobal`・`config.ValidateAllTemplates`・`newPermChecker`・`cli.FilterGroups` のエラー）は `dryRunPreviewExit`・`SilentExitError` を含まないので、4 つとも `PreExecutionError` として報告され、終了コードは 1 のまま変わらない。

## ガードの判定の細則

02 §3.8.2 から移した判定の仕方である。02 が決めるのは、各ガードが守る不変条件と、各ガードに検出の自己テストを付けることである。

- 共通: 既存のガードと同じく `go/parser` で解析し、`internal/testutil/identitymutationguard` の補助関数を使う。`errmsg` の関数は、名前ではなく import のパスで解決する（`ResolveLocalImports`）。別名での import やドットでの import も解決する。
- ラップの検査（AC-41）: 02 §3.8.1 の範囲の中に、`fmt.Errorf`（`%w` の有無によらない）、`errors.Join`、定数式でない引数の `errors.New` の呼び出しが無いこと。範囲の中のファイルで `Unwrap` を宣言する型は、`StructuredMessage` も宣言すること。ラップせずに `err` をそのまま返すことは許す。ガードは範囲の定義を持つ（案: `internal/runner/wrap_guard_test.go`）。関数の単位で指定した名前と除く関数の名前が実際のコードに見つかることも確かめる。
- `Const` の検査（AC-24）: 本番のコードの `errmsg.Const`・`errmsg.ConstSummary` の呼び出しの引数が、文字列リテラル、定数の名前、またはそれらを `+` でつないだ式であること。定数の名前は、同じパッケージの `const` 宣言に解決できるものに限り、解決できない名前は拒否する（fail-closed）。関数の呼び出し以外の使い方（`f := errmsg.Const` など）も拒否する。
- 免除の役割の検査: `errmsg.Ident`・`errmsg.Path` の呼び出しが 02 §3.8.1 の範囲の中にあること。`errmsg.PathErrorCause` の呼び出しが `(*DefaultTempDirManager).Create` の中にあること。判定は呼び出しの位置（ファイルと関数）で行い、関数やメソッドの名前の一致では行わない。呼び出し以外の使い方は拒否する。
- 文言と構造の一致: 本番のコードで `StructuredMessage` を宣言する型の `Error()` の本体が、`return <受け手>.StructuredMessage().String()` の 1 文だけであること。`errmsg.Error` もこの形で書く。`*errmsg.Error` を埋め込んだ型が `Error()` を宣言することも拒否する。`StructuredMessage` という名前のメソッドを持つ型と埋め込みをたどって調べるので、型の一覧は保守しない。
- 役割を選ぶ処理の禁止（AC-25）: `internal/redaction` の本番のコードが、`errmsg.Role` の値を作らないこと（`errmsg.RoleText` などの定数の参照、`errmsg.Role(...)` の変換、`Segment` の `Role` 欄への代入が無いこと。`switch` の `case` での参照は読むだけなので許す）。
- 形による判定の禁止（AC-33）: 既存の `TestProductionCodeDoesNotProbeMultiErrorShape` が、`internal/errmsg` と `internal/redaction` を含む本番のコード全体を調べている。
- 欄の非公開の検査: `errmsg.Part` の欄が非公開であり、`errmsg` の外で `Part` の複合リテラルを作っていないこと。
- 部分を返す関数の検査（02 §3.1.1 の契約 8）: `errmsg` の外に、結果の型が `errmsg.Part` を含む（`[]errmsg.Part` や、`Part` を欄に持つ型などの複合も含む）公開の関数・メソッドが無いこと。結果に `errmsg.Part` を含む非公開の関数・メソッドは 02 §3.8.1 の範囲の中にあること。自己テストには、公開の `Parts()` メソッドを持つ型を与えて検出されることを確かめる。
- 整形のバイトの検査（02 §3.8.2 で加えた不変条件）: 呼び出し側が渡すバイトが、整形として `Identifier`・`Path`・`Constant` の断片に入らないこと。判定の仕方（公開の構築関数の引数の型を調べる、`IndentedCause` の引数が原因だけであることをシグネチャで固定するなど）を決める。
- 自己テスト: 各ガードに、検出すべき形を与えて検出されることを確かめるテストを付ける（既存の `TestMultiErrorShapeProbeCheckRecognizesForms` と同じ形）。
- 案のファイル: `internal/errmsg/errmsg_guard_test.go`（`Const`・免除の役割・欄の非公開・文言と構造の一致）、`internal/redaction/redaction_guard_test.go`（AC-25）、`internal/runner/wrap_guard_test.go`（AC-41）。

## 既存のテストへの影響

02 §3.9・§5.4 から移した細則である。

- 変わる既存のテスト（挙動や型が変わるので更新が要る）:
  - `PreExecutionError`・`ExecutionError` のリテラルで `Message` に文字列を渡すテスト（`internal/logging/pre_execution_error_test.go` 16、`internal/runner/runerrors/pre_execution_guard_test.go` 7、`internal/logging/notification_contract_guard_test.go` 19、`cmd/runner` 2 のリテラルなど）。書き換えは機械的で、確かめる内容は変わらない。`notification_contract_guard_test.go` は `PreExecutionError` のリテラルを AST で調べるので、`Message` の新しい型に合わせる。
  - `cmd/runner/main_test.go:730`: `preExec.Message` が原因の文言を含むことを確かめている。原因が `Err` に移るので、`Detail()` の文言か `errors.Is(err, errCheckerUnavailable)` で確かめる形に変える。
  - `ErrUndefinedVariableDetail` の `Level`・`Field` を文字列として比べる `internal/runner/config` のテスト。`.String()` で比べる。
- 確認が要る既存のテスト: 記録された `error_message` を `attr.Value.String()` で読むテスト（`internal/logging/pre_execution_error_test.go:594`・`:658`・`:998`、`internal/runner/multi_group_error_integration_test.go:48`、`internal/runner/group_stage_test.go:183` など）。値は `slog.KindLogValuer` になるが、`slog.Value.String()` は `fmt` を通して `Message.String()` を呼ぶので、読み出す文字列は変わらない見込みである。実装のときに実行して確かめる。
- 変わらない既存のテスト: `error` 属性の全文が値全体置換を受けることを固定するテスト（`internal/redaction/redactor_test.go:3947` の `failed to execute group monkey`）。対象外の属性（01 対象外「`error_message` 以外の属性」）についてのものである。
- 0176 の `error_message` の本文が `[REDACTED]` になることを固定するテストは無い。`internal/logging`・`internal/runner`・`cmd/runner` のテストのうち `REDACTED` と `error_message` の両方を含むものは 3 つ（`internal/logging/slack_handler_test.go`・`internal/runner/runner_test.go`・`internal/runner/base/security/logging_security_test.go`）で、いずれも宣言された識別子が消えないこと、または key=value・Webhook の値が置き換えられることを確かめている。
- security-architecture の書き換える段落: `docs/dev/architecture_design/security-architecture.ja.md` の `:645-651`（「識別子の型宣言による免除」）。
- 案の新しいテストファイル: `internal/errmsg/errmsg_test.go`（平らにする規則、`String()` と `Error()` の一致、`PathErrorCause`、`IndentedCause`、nil の原因、深い連鎖の描画）、`internal/redaction/message_test.go`・`ranges_test.go`（AC-01〜04・07・36〜38、範囲と `RedactText` の差分のファジング、実行時の検査）。

## テストの細則

02 §3.1.2・§7.1 から移した細則である。

- AC-38: `RedactMessage` の中の断片の列を受け取る非公開の関数（「RedactMessage の描画手順」）に、`Segment{Role: errmsg.Role(99), ...}` を直接与える。`Segment` の欄は公開なので、範囲外の値を与えられる。値全体置換だけが反応する入力を使い、その断片が置換文字列になることを確かめる。
- 深い連鎖: 16 段より深い構造化エラーの連鎖の `String()` が、同じ形の `fmt.Errorf` の `%w` の連鎖の `Error()` と一致すること。深さの上限を仮に戻すとこのテストが失敗することを確かめる（CLAUDE.md「Every test must be able to fail for its stated reason」）。
- AC-21: `RedactingHandler` を通らない標準のハンドラ（`slog.NewTextHandler` など）に `errmsg.Message` の属性を渡し、出力が `String()` と同じ文字列になること。
- AC-37: 検出の種類（key=value、`Bearer `・`Basic ` の次の語、`Authorization` のヘッダ値、値形式の検出）ごとに、各断片だけに redaction を適用しても秘密が見えたまま残ることを先に確かめる（design_carryover.md「テストの入力の細則」）。
- 一時ディレクトリ: 2 つのラップのそれぞれで文言を確かめる（「一時ディレクトリの 2 つのラップ」）。
- `cancelledRunError`: 文言が `errors.Join(ctxErr, err)` と同じであること、到達性（AC-32）。
- `config` の `Level`・`Field`: `String()` が変更前の `fmt.Sprintf` の結果と同じであること。
