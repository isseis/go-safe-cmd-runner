# M3: `LD_*` 変数のブロック不完全

- **重大度**: 🟡 Medium
- **領域**: 環境変数フィルタ (`internal/runner/executor`)
- **影響コマンド**: `runner`

## 問題

[internal/runner/executor/environment.go:87-89](../../../internal/runner/executor/environment.go#L87-L89) において、glibc/動的リンカ関連で悪用可能な環境変数のうち、`LD_LIBRARY_PATH`, `LD_PRELOAD`, `LD_AUDIT` の 3 つだけが強制削除対象になっている。

```go
delete(envMap, "LD_LIBRARY_PATH")
delete(envMap, "LD_PRELOAD")
delete(envMap, "LD_AUDIT")
```

しかし glibc の動的リンカ (`ld.so`) は他にも実行時挙動を変更できる環境変数を多数受け付ける。以下は代表例 (`man ld.so` 参照):

| 変数 | 影響 |
|---|---|
| `LD_DEBUG` | デバッグ出力 (情報漏洩、`LD_DEBUG_OUTPUT` と組み合わせて任意ファイル書込) |
| `LD_DEBUG_OUTPUT` | `LD_DEBUG` の出力先、任意パスへの書込 |
| `LD_PROFILE` | プロファイル対象指定、`LD_PROFILE_OUTPUT` で任意パス書込 |
| `LD_PROFILE_OUTPUT` | プロファイル出力先 |
| `LD_SHOW_AUXV` | auxiliary vector を stderr に出力 (情報漏洩) |
| `LD_TRACE_LOADED_OBJECTS` | ロード対象の表示のみで実行せず (実質 `ldd` 相当) |
| `LD_DYNAMIC_WEAK` | 弱シンボル解決の挙動変更 |
| `LD_POINTER_GUARD` | pointer mangling 無効化 (攻撃緩和の低下) |
| `LD_HWCAP_MASK` | ハードウェア機能ビットマスク操作 |
| `LD_ORIGIN_PATH` | `$ORIGIN` 展開の制御 |
| `LD_USE_LOAD_BIAS` | PIE load bias 制御 |
| `LD_WARN` | 警告レベル変更 |
| `LD_BIND_NOW` / `LD_BIND_NOT` | lazy binding 制御 |
| `GCONV_PATH` | iconv モジュール検索パス (任意 `.so` ロード) |
| `LOCPATH` | locale データ検索パス (任意バイナリデータ解釈) |
| `RES_OPTIONS` | resolver 挙動変更 |
| `HOSTALIASES` | hosts 解決の上書き |
| `NLSPATH` | メッセージカタログ検索パス |
| `TMPDIR` | 一時ディレクトリ (一部のライブラリが参照) |

## 影響

- 直接の任意コード実行: `GCONV_PATH` は iconv モジュール (`.so`) 検索パスとして働き、suid バイナリでなくても任意 `.so` ロードが可能。
- 情報漏洩: `LD_DEBUG` + `LD_DEBUG_OUTPUT` により `/proc/self/...` や読み取り可能な任意ファイルの内容がダンプされうる。
- 任意ファイル書込: `LD_DEBUG_OUTPUT`, `LD_PROFILE_OUTPUT` によりプロセス権限で到達可能なパスへの書込み。
- pointer guard 無効化: `LD_POINTER_GUARD=0` で ROP 耐性を低下させ、他脆弱性の悪用難度を下げる。

### 緩和要因

- `runner` プロセス自身は setuid から **即時 drop** しているため、`ld.so` の secure-execution モードは発動しない (=上記変数がそのまま効く)。
- 実行される子プロセスが suid/sgid でない限り、攻撃者と同じ権限で動作するため昇格経路としての危険性は低い。
- ただし `runner` が root 相当で実行されている運用では、`runner` 自身の挙動が上記変数の影響を受ける。

しかし本プロジェクトの防御方針は **allowlist ベースで環境変数を制限** することであり、denylist 漏れは層状防御の弱体化に直結する。

## 修正方針

### 案 A (推奨): プレフィックスベースの除外

`LD_` で始まる変数および上表の非 `LD_` 系 (`GCONV_PATH`, `LOCPATH`, `HOSTALIASES`, `NLSPATH`, `RES_OPTIONS`) を一律削除。

```go
for k := range envMap {
    if strings.HasPrefix(k, "LD_") {
        delete(envMap, k)
    }
}
for _, k := range []string{"GCONV_PATH", "LOCPATH", "HOSTALIASES", "NLSPATH", "RES_OPTIONS"} {
    delete(envMap, k)
}
```

### 案 B: allowlist の厳格化

そもそも環境変数は allowlist でフィルタされているため、allowlist に上記のような危険変数を **絶対に含めない** ことをドキュメント化し、`security.Validator` で allowlist ロード時にブラックリストと突き合わせて警告。

- 案 A はゼロコストかつ絶対安全。案 B は allowlist 設計者への教育になるが運用事故を防ぎきれない。両方適用が望ましい。

## 参考箇所

- [internal/runner/executor/environment.go:87-89](../../../internal/runner/executor/environment.go#L87-L89) — 現在の削除リスト
- `man 8 ld.so` — glibc 動的リンカのドキュメント
