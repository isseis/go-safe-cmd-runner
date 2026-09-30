# タスク: 依存検証の dynlib・shebang のエラー型を構造化する (Issue #1196)

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-30 |
| Review date | - |
| Reviewer | - |
| Comments | 2026-09-30: 決定 1（SOName を `Path`）、決定 2（hash を `Text`）、決定 3（インタプリタの識別子を `Path`）、決定 4（再解決のラップを `errmsg` 化）、決定 5（型は既存パッケージのまま）をすべて承認。文書全体の承認待ち。 |

## 位置づけ（フルセット文書を作らない理由）

本タスクは Task 0178（#1168）の後続であり、0178 が導入した
`internal/errmsg` の仕組みと部分ごとの redaction を、既存の 8 つのエラー型に
適用するだけである。新しい設計判断は、SOName を含む一部の値にどの役割を
割り当てるかという限定的なもので、要件・方針・検証方法は下記で完結する。
そのため `docs/dev/developer_guide/requirements_process.md` が定める
`01_requirements.md` / `02_architecture.md` / `03_implementation_plan.md` の
フルセットではなく、本ファイル 1 本に要件・方針・進捗を統合する
（`0152_redact_log_message_body/README.md` と同じ形式）。

## 概要

- Issue: [#1196](https://github.com/isseis/go-safe-cmd-runner/issues/1196)
- 親タスク: Task 0178（[#1168](https://github.com/isseis/go-safe-cmd-runner/issues/1168)、`docs/tasks/0178_structured_error_message_redaction/`）
- 関連: [#1197](https://github.com/isseis/go-safe-cmd-runner/issues/1197)、Task 0176 設計書 §5.2

0178 ではエラーの本文を、役割（`Constant`・`Identifier`・`Path`・`Text`）を型で
宣言した部分の列として運び、部分ごとに redaction する。構造を持たないエラーは
`Error()` 全体を 1 つの `Text` の部分として扱う（fail-closed）。dynlib 検証と
shebang 検証の内側のエラーは構造化していないため、内側のエラーが機密を示す語
（`key`・`secret`・`token` など）を含むと、その部分が値全体置換で `[REDACTED]` に
なる。

```
Command verification failed: command dependency verification failed for "/usr/bin/curl": [REDACTED]
```

消えるのは、`libkeyutils.so.1`（`libkrb5` 経由で多くのネットワーク系コマンドが
依存）のように語を含むライブラリ名やパスに限られる。通知からはどのコマンドの
検証が失敗したかは分かるが、どのライブラリで失敗したかは分からない。運用上の
次の手（改ざんの確認、`record` の再実行）はコマンド単位で決まるため、この情報が
欠けると判断が遅れる。全文は最終報告の stderr の `Details:` に残る（stderr は
redaction を通らない）。

## 対象の型

次の 8 型に `StructuredMessage() errmsg.Message` を追加し、`Error()` の文言は
変えない。型のパッケージは現状のまま（`errmsg` は標準ライブラリのみに依存する
末端パッケージなので、両パッケージから import でき、循環は生じない）。

| 型 | パッケージ | 値の欄 | 割り当てる役割 |
|---|---|---|---|
| `ErrLibraryHashMismatch` | `internal/dynlib` | `SOName` / `Path` | `Path` / `Path` |
| | | `ExpectedHash` / `ActualHash` | `Text` / `Text` |
| `ErrDynLibDepsRequired` | `internal/dynlib` | `BinaryPath` | `Path` |
| `ErrEmptyLibraryPath` | `internal/dynlib` | `SOName` | `Path` |
| `ErrRecursionDepthExceeded` | `internal/dynlib` | `SOName` | `Path` |
| | | `Depth` / `MaxDepth` | `Text`（`strconv.Itoa`） |
| `ErrDynLibDepsResolutionChanged` | `internal/verification` | `SOName` | `Path`（空なら `Path("<unknown>")`） |
| | | `RecordedPath` / `ResolvedPath` | `Path` / `Path` |
| `ErrInterpreterRecordNotFound` | `internal/verification` | `Path` | `Path` |
| `ErrInterpreterSymlinkRedirected` | `internal/verification` | `RawPath` / `RecordedPath` / `ActualPath` | `Path` |
| `ErrInterpreterPathMismatch` | `internal/verification` | `CommandName` / `RecordedPath` / `ActualPath` | `Path` |

`Path` は `RedactText` だけを適用し（key=value と値形式の検出）、値全体置換は
適用しない。これにより `libkeyutils.so.1` のような正当な名前が丸ごと消えるのを
防ぎつつ、値形式の機密は引き続き検出できる。

## 要件で決めること（承認待ち）

1. **SOName の役割。** SOName は検証対象バイナリの `DT_NEEDED` から読む値であり、
   設定で定義された名前ではないので `Identifier`（redaction 完全免除）にはしない。
   `Path` 相当（`RedactText` のみ、値全体置換なし）とすることを、保護の境界として
   承認するか。→ **決定: `Path` とする（2026-09-30 承認）**。
2. **hash 値の役割。** `ExpectedHash`・`ActualHash` は hex ダイジェストなので `Text`
   のままとし、fail-closed を維持する。値全体置換の判定語彙（`password|token|secret|key` など）は
   hex に現れず、値形式検出の各パターンにも当たらないため、現行フォーマットでは表示が
   劣化しない。→ **決定: `Text`（2026-09-30 承認）**。
3. **インタプリタの識別子の役割。** `ErrInterpreterPathMismatch.CommandName` は
   shebang 行の `entry.Ref`（例: `python3`）、`ErrInterpreterSymlinkRedirected.RawPath`
   は shebang 行のパス（例: `/bin/sh`）である。どちらも設定由来ではないが `Path`
   として扱い、`Ident` にはしない。→ **決定: どちらも `Path`（2026-09-30 承認）**。
4. **再解決のラップ。** `resolveDynLibDeps`（`manager.go:822`）の
   `failed to re-resolve ELF/Mach-O dynamic library dependencies for %s: %w` は
   `fmt.Errorf` であり、対象 8 型のうち `ErrRecursionDepthExceeded` だけがこの
   経路を通って構造を失う。この 2 か所を `errmsg.Text(fmt.Sprintf(...))` +
   `Cause` に変えて原因の構造を保つ。→ **決定: 変えて保つ（2026-09-30 承認）**。
   その他の I/O の失敗だけを包む `fmt.Errorf` は `Text` のまま変更しない。
5. **構造化メッセージの置き場所。** 8 型は既存パッケージに置いたまま `errmsg` を
   import する。0178 の設計（`errmsg` は末端）はこの制約を満たす。→ **決定:
   既存パッケージのまま（2026-09-30 承認）**。

## 対応方針

### 1. `StructuredMessage` の実装

各型の `Error()` を `return e.StructuredMessage().String()` の 1 文に変え、現在の
`fmt.Sprintf` の文言を `StructuredMessage` の部分列にそのまま移す。0178 の
§9.5（文言と構造の一致）ガードがこの形を要求するためである。例:

```go
func (e *ErrLibraryHashMismatch) Error() string {
	return e.StructuredMessage().String()
}

func (e *ErrLibraryHashMismatch) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("dynamic library hash mismatch: "),
		errmsg.Path(e.SOName),
		errmsg.Const("\n  path: "),
		errmsg.Path(e.Path),
		errmsg.Const("\n  expected hash: "),
		errmsg.Text(e.ExpectedHash),
		errmsg.Const("\n  actual hash: "),
		errmsg.Text(e.ActualHash),
		errmsg.Const("\n  please re-run 'record' command"),
	)
}
```

残り 7 型も同様に、`Const` の断片と値を運ぶ断片（`Path`・`Text`）を
`Error()` の文言の順に並べる。複数行の型（`ErrLibraryHashMismatch`・
`ErrEmptyLibraryPath`・`ErrDynLibDepsRequired`・`ErrDynLibDepsResolutionChanged`）は
改行と字下げを `Const` の文字列リテラルに含め、`Error()` とバイト単位で一致させる。

### 2. ガードの更新

- `internal/errmsg/errmsg_guard_test.go` の `exemptRolePositions` に、8 つの
  `StructuredMessage` を許可位置として追加する（`internal/dynlib/errors.go` に 4、
  `internal/verification/errors.go` に 4）。
- `productionGuardFiles` は `errmsg` への import 到達性で対象を決めるため、この
  2 パッケージが `errmsg` を import した時点で、`Const` の定数式検査（§9.3）・
  `Part` の流れ（§9.8）・文言と構造の一致（§9.5）の対象に自動的に入る。追加の
  一覧保守は不要。
- §9.1（`wrap_guard_test.go` が持つラップの対象範囲）には、この 2 ファイルを
  加えない。加えるとファイル全体の `Unwrap` 型がすべて `StructuredMessage` を
  要求され、本タスクの範囲を超える。§9.3 の `Const` 検査は §9.1 とは独立に
  import 到達性で広がるため、要件（`Constant` の定数式ガードが両パッケージに
  及ぶこと）は満たされる。この点は自己テストで確かめる。

### 3. redaction の期待

- 値全体置換だけが反応する入力（例: `libkeyutils.so.1` を `Path` とした断片）は
  そのまま残る。
- 値形式の検出だけが反応する入力（例: AWS キー形式）は `Path` の断片でも置換される。
- `Text` とした断片（hash・深さ）に語を含む値が入った場合だけ値全体置換が働く。
- 2 層を分けて確かめるため、対照の `Text` 断片（例: `errmsg.Text("libkeyutils.so.1")`）を
  同じメッセージに置き、そちらは置換され、`Path` の断片は置換されないことを
  1 つのテストで示す（CLAUDE.md の「層ごとの入力」）。

## テスト方針

- `internal/dynlib/errors_test.go`（新設または既存）と
  `internal/verification/errors_test.go` に、各型について次を追加する。
  - `StructuredMessage().Segments()` の役割とテキストが、表の割り当てと
    現在の `Error()` の文言に一致すること。
  - `Error()` の文字列が変更前と一致すること（§9.5 の一致はガードでも見る）。
  - 空の `SOName` が `<unknown>` になること（`ErrDynLibDepsResolutionChanged`）。
- `internal/redaction/message_test.go` に、dynlib・shebang の 8 型を原因とする
  本文を通し、`libkeyutils.so.1` が残り対照の `Text` 値が置換されることを確かめる
  統合ケースを追加する。
- ガードの自己テスト（§9.10）を追加し、実装を壊すと失敗することを確かめる。
  - `internal/dynlib/errors.go`・`internal/verification/errors.go` の
    `StructuredMessage` が許可位置として通ること。
  - 許可位置以外（例: 同ファイル内の別関数、別ファイル）で `Ident`・`Path` を
    呼ぶ合成入力が拒否されること。
  - 対象 8 型に `StructuredMessage` があること、および対象パッケージが
    `Const` の定数式検査の対象に入っていること。
- 完了時に `make fmt`・`make test`・`make lint` を通す。

## 受け入れ基準 (Acceptance Criteria)

- **AC-01**: 8 型すべてが `errmsg.Structured` を実装し、`Error()` の文字列が
  変更前と一致する。
- **AC-02**: 8 型の `StructuredMessage` の各部分の役割が上の表のとおりである
  （パス・SOName は `Path`、hash と深さは `Text`、固定の文言は `Constant`）。
- **AC-03**: 複数行の型の改行・字下げが `Error()` とバイト単位で一致する。
- **AC-04**: `ErrDynLibDepsResolutionChanged` で `SOName` が空のとき
  `<unknown>` となる。
- **AC-05**: redaction の統合テストで、`Path` の断片（`libkeyutils.so.1`）が
  値全体置換を受けずに残り、同じメッセージの対照の `Text` 断片は置換される。
  `Path` の断片でも値形式の検出は働く。
- **AC-06**: `internal/errmsg` のガードが `internal/dynlib`・`internal/verification`
  を対象に含み、両パッケージの `StructuredMessage` が許可位置として通り、許可
  位置以外の `Ident`・`Path` は拒否される。
- **AC-07**: `resolveDynLibDeps` のラップを `errmsg` の `Text` + `Cause` に
  変え、再解決経路の `ErrRecursionDepthExceeded` が内側の構造を保つことを
  テストで確かめる。
- **AC-08**: 上記を壊すと失敗するガード・テストの自己テストがある。

## 実装チェックリスト

- [ ] `internal/dynlib/errors.go` の 4 型に `StructuredMessage` を追加し、
      `Error()` を `StructuredMessage().String()` に変える (AC-01〜AC-03)
- [ ] `internal/verification/errors.go` の 4 型に同様に追加する (AC-01〜AC-04)
- [ ] `resolveDynLibDeps` の 2 か所のラップを `errmsg` に変える（決定 4。AC-07）
- [ ] `internal/errmsg/errmsg_guard_test.go` の `exemptRolePositions` を更新し、
      自己テストを追加する (AC-06, AC-08)
- [ ] `internal/dynlib`・`internal/verification`・`internal/redaction` のテストを
      追加する (AC-01〜AC-05)
- [ ] `make fmt` / `make test` / `make lint` を通す
- [ ] Issue #1196 をクローズ（PR とリンク）

## Acceptance Criteria Verification

| AC | Test | Implementation | Verification |
|---|---|---|---|
| AC-01 | `internal/dynlib/errors_test.go`・`internal/verification/errors_test.go` | 各 `Error()` / `StructuredMessage` | 文言不変と `Structured` 実装を確認 |
| AC-02 | 同上（`Segments()` の役割を表と突き合わせ） | 各 `StructuredMessage` | 部分ごとの役割を確認 |
| AC-03 | 同上（複数行の型） | 同上 | `Error()` とのバイト一致を確認 |
| AC-04 | `internal/verification/errors_test.go` | `ErrDynLibDepsResolutionChanged.StructuredMessage` | 空 SOName で `<unknown>` |
| AC-05 | `internal/redaction/message_test.go` | 8 型 | `Path` と対照 `Text` の層別の挙動 |
| AC-06 | `internal/errmsg/errmsg_guard_test.go::TestProductionExemptRoleCalls` ほか自己テスト | ガード | 対象パッケージと許可位置 |
| AC-07 | `internal/verification/manager_test.go`・`internal/redaction/message_test.go` | `resolveDynLibDeps` | 再解決経路で内側の構造が残る |
| AC-08 | 各ガードの自己テスト | ガード・テスト | 実装を壊すと失敗する |

## 関連

- [#1168](https://github.com/isseis/go-safe-cmd-runner/issues/1168)（Task 0178: 本文を役割付きの構造で運び、部分ごとに redaction する）
- [#1197](https://github.com/isseis/go-safe-cmd-runner/issues/1197)（設定の展開・検証のエラー型の構造化。本タスクの後に取り組む。役割の決定を引き継ぐ）
- `docs/tasks/0178_structured_error_message_redaction/03_detailed_specification.md` §9（ガードの契約。§9.1・§9.3・§9.5・§9.8・§9.10 を参照）
- Task 0176 設計書 §5.2（redaction の扱いと、本文が読めないときの運用）
