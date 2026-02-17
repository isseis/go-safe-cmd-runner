# pclntab (Program Counter Line Table) メンテナンスガイド

## 概要

本ドキュメントでは、Go バイナリの `.gopclntab` セクション（pclntab）のパース処理について、Go バージョンアップ時の対応手順と背景情報を説明する。

pclntab は Go ランタイムがスタックトレース生成とガベージコレクションに使用する内部構造であり、strip されたバイナリでも関数名とアドレスを復元するために利用する。

## pclntab バージョン履歴

| Go バージョン | pclntab バージョン | マジックナンバー | 主な変更点 |
|--------------|-------------------|-----------------|-----------|
| Go 1.2-1.15 | ver12 | `0xFFFFFFFB` | 初期フォーマット。全データが単一配列に格納 |
| Go 1.16-1.17 | ver116 | `0xFFFFFFFA` | テーブル分離。絶対ポインタ使用 |
| Go 1.18-1.19 | ver118 | `0xFFFFFFF0` | エントリ PC が 32 ビットオフセットに変更 |
| Go 1.20+ | ver120 | `0xFFFFFFF1` | 現行フォーマット |

**重要**: pclntab バージョンは Go ランタイムバージョンと常に一致するわけではない。例えば Go 1.19 は ver118 (Go 1.18 形式) を使用する。

## ヘッダー構造

全バージョン共通の最小 8 バイトヘッダー:

```
オフセット  サイズ  内容
0-3        4      マジックナンバー（リトルエンディアン）
4-5        2      パディング（0x00, 0x00）
6          1      PC quantum（1, 2, or 4）
7          1      ポインタサイズ（4 or 8）
8+         可変   バージョン固有データ
```

**注記**:
- Go 1.18+ は追加フィールドにより 16 バイト以上のヘッダーが必要

### Go 1.18+ (ver118, ver120) の追加フィールド

```go
// pcHeader 構造（Go 1.18+）
// 参照: https://go.dev/src/runtime/symtab.go
type pcHeader struct {
    magic          uint32  // offset 0x00: マジックナンバー
    pad1, pad2     uint8   // offset 0x04-0x05: パディング
    minLC          uint8   // offset 0x06: 最小命令サイズ (PC quantum)
    ptrSize        uint8   // offset 0x07: ポインタサイズ（4 or 8）
    nfunc          int     // offset 0x08: 関数数
    nfiles         uint    // offset 0x10: ファイルテーブルエントリ数
    textStart      uintptr // offset 0x18: 関数エントリ PC のベースアドレス
    funcnameOffset uintptr // offset 0x20: 関数名テーブルへのオフセット
    cuOffset       uintptr // offset 0x28: コンパイル単位テーブルへのオフセット
    filetabOffset  uintptr // offset 0x30: ファイルテーブルへのオフセット
    pctabOffset    uintptr // offset 0x38: PC テーブルへのオフセット
    pclnOffset     uintptr // offset 0x40: pclntab データへのオフセット
    ftabOffset     uintptr // offset 0x48: 関数テーブル（functab）へのオフセット
}
```

**注記**:
- `nfunc` と `nfiles` のサイズは `ptrSize` に依存（32-bit: 4 bytes, 64-bit: 8 bytes）
- `ftabOffset` は関数エントリを取得するために必須。詳細仕様書 §2.4 `parseFuncTable` を参照
- 総ヘッダーサイズ: 64-bit の場合は 80 バイト（0x50）、32-bit の場合は 52 バイト

## 新バージョン対応時の作業手順

### 1. 変更検出（目安: 1-2 時間）

新しい Go メジャー/マイナーバージョンがリリースされた際:

1. **マジックナンバーの確認**
   - Go ソースコード [`src/debug/gosym/pclntab.go`](https://go.dev/src/debug/gosym/pclntab.go) を確認
   - 新しいマジックナンバー定数が追加されているか確認

2. **ランタイム構造の確認**
   - [`src/runtime/symtab.go`](https://go.dev/src/runtime/symtab.go) の `pcHeader` 構造体を確認
   - フィールドの追加・変更・削除を確認

3. **変更なしの場合**
   - pclntab バージョンがランタイムバージョンより遅れることが多い
   - 例: Go 1.21, 1.22, 1.23 は ver120 (Go 1.20 形式) を継続使用
   - この場合、対応作業は不要

### 2. パーサー修正（目安: 2-4 時間）

変更が検出された場合:

1. **マジックナンバー定数の追加**
   ```go
   const (
       pclntabMagicGo12  = 0xFFFFFFFB
       pclntabMagicGo116 = 0xFFFFFFFA
       pclntabMagicGo118 = 0xFFFFFFF0
       pclntabMagicGo120 = 0xFFFFFFF1
       pclntabMagicGoXXX = 0xXXXXXXXX  // 新規追加
   )
   ```

2. **パース関数の追加**
   ```go
   func (p *pclntabParser) parseGoXXX(data []byte) error {
       // 新バージョン固有のパースロジック
   }
   ```

3. **switch 文の更新**
   ```go
   switch magic {
   case pclntabMagicGoXXX:
       return p.parseGoXXX(data)
   // ... 既存ケース
   }
   ```

### 3. テスト追加（目安: 2-3 時間）

1. **テスト用バイナリの作成**
   - 新バージョンの Go でコンパイルしたテスト用バイナリを用意
   - strip されたバージョンも用意

2. **ユニットテストの追加**
   - 新バージョンの pclntab パースが成功することを確認
   - 関数名・アドレスが正しく抽出されることを確認

### 4. ドキュメント更新（目安: 30 分）

1. 本ドキュメントのバージョン履歴テーブルを更新
2. 要件定義書（`docs/tasks/0070_elf_syscall_analysis/01_requirements.md`）のマジックナンバーリストを更新

## 対応コストの見積もり

| シナリオ | 作業時間 | 発生頻度 |
|---------|---------|---------|
| 変更なし（確認のみ） | 1-2 時間 | Go メジャーリリースの約 70% |
| 軽微な変更（オフセット調整等） | 4-6 時間 | Go メジャーリリースの約 20% |
| 大幅な構造変更 | 1-2 日 | Go メジャーリリースの約 10% |

**注記**:
- Go のメジャーリリースは約 6 ヶ月ごと（2 月と 8 月）
- pclntab 構造の大幅変更は 2-3 年に 1 回程度（Go 1.16, 1.18, 1.20 で発生）
- 軽微な変更であれば、Go 公式の `debug/gosym` パッケージの変更差分を参考にできる

## 参考リソース

### Go 公式ソースコード

- [debug/gosym/pclntab.go](https://go.dev/src/debug/gosym/pclntab.go) - pclntab パーサーの公式実装
- [runtime/symtab.go](https://go.dev/src/runtime/symtab.go) - ランタイムでの pclntab 構造定義
- [cmd/link/internal/ld/pcln.go](https://go.dev/src/cmd/link/internal/ld/pcln.go) - リンカーでの pclntab 生成

### 外部ツール・ドキュメント

- [GoReSym](https://github.com/mandiant/GoReSym) - Mandiant 製の Go シンボル復元ツール。複数バージョン対応の実装例として参考になる
- [Go 1.2 Runtime Symbol Information](https://docs.google.com/document/d/1lyPIbmsYbXnpNj57a261hgOYVpNRcgydurVQIyZOz_o/pub) - pclntab 設計の原典（Go 1.2 時点）
- [Golang Internals: Symbol Recovery](https://cloud.google.com/blog/topics/threat-intelligence/golang-internals-symbol-recovery) - Google Cloud Blog の詳細解説

## 代替アプローチの検討

pclntab のメンテナンスコストが問題になる場合、以下の代替アプローチを検討できる:

### 1. debug/gosym パッケージの利用

Go 標準ライブラリの `debug/gosym` パッケージを直接利用する方法。

**メリット**:
- Go 本体のアップデートで自動的に新バージョンに対応
- メンテナンスコストがほぼゼロ

**デメリット**:
- strip されたバイナリでは `.gosymtab` セクションが必要（Go 1.3 以降は空）
- 単独では strip されたバイナリに対応できない

### 2. GoReSym のライブラリ利用

[GoReSym](https://github.com/mandiant/GoReSym) のコードを参考にするか、ライブラリとして利用する方法。

**メリット**:
- 複数バージョン対応済み
- strip されたバイナリ、難読化されたバイナリにも一部対応
- 活発にメンテナンスされている

**デメリット**:
- 外部依存が増加
- ライセンス確認が必要（Apache 2.0）

### 3. サポートバージョンの限定

現行バージョン（Go 1.18+）のみをサポートし、古いバージョンは High Risk として扱う方法。

**メリット**:
- 実装・メンテナンスが単純化
- 実務上、古いバージョンの Go でビルドされたバイナリは少数

**デメリット**:
- 古いバイナリが false positive になる

## 関連ファイル

- `internal/runner/security/elfanalyzer/pclntab_parser.go` - pclntab パーサー実装
- `internal/runner/security/elfanalyzer/go_wrapper_resolver.go` - pclntab を利用する Go ラッパー解析
- `docs/tasks/0070_elf_syscall_analysis/03_detailed_specification.md` - 詳細設計
