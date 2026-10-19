# 要件定義書: プロセス生成機能を持つ一般ツールのリスク分類

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-10-19 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#1228](https://github.com/isseis/go-safe-cmd-runner/issues/1228) [Security][SA-01] risk: プロセス生成機能を持つ一般ツールが Low リスクで実行される
- 出典: [security-audit-2026-10.ja.md](../../dev/security/security-audit-2026-10.ja.md) §4.1 SA-01（重大度 High、攻撃者 B）
- 関連する別 issue（本タスクの対象外）: [#1241](https://github.com/isseis/go-safe-cmd-runner/issues/1241)（SA-13、`GIT_CONFIG_*` などコード実行に使える環境変数の denylist）、[#1232](https://github.com/isseis/go-safe-cmd-runner/issues/1232)（SA-05、子プロセスへ渡す `PATH`）、[#1254](https://github.com/isseis/go-safe-cmd-runner/issues/1254)（SA-25、`risk_level` の配備側の上限）

## 背景

### 現在の流れ

runner はコマンドごとに算出リスクを求め、設定の `risk_level`（省略時は `low`）を超えるコマンドを実行しない。算出リスクは、次の次元の最大値である（`internal/runner/base/risk/evaluator.go` の `EvaluateRisk`）。

| 次元 | 根拠 | 主な定義 |
|---|---|---|
| 間接実行 | ラッパー・インラインコード・ヘルパー起動オプション | `indirect_execution.go` の `analyzeIndirect` |
| 特権昇格 | コマンド名 | `command_analysis.go` の特権プロファイル（`sudo`・`su` など） |
| システム変更 | コマンド名 | `highSystemModificationNames` |
| 任意コード実行 | コマンド名 | `arbitrary_code.go` の `arbitraryCodeExecutionRunners` |
| バイナリ解析 | record 時の静的解析の結果 | `network_analyzer.go` の `classifyRecordSignals` |

名前による分類は、設定のパスから symlink を辿って得た名前の集合（設定のパスの basename と各 symlink の先の basename）に対して行う。

間接実行の次元は、形態に応じて次の 3 通りの結果を返す。

| 結果 | 意味 | 例 |
|---|---|---|
| Critical | 特権昇格のトークンを含む。常に拒否 | `env sudo id` |
| Reject（Blocking） | `risk_level` によらず拒否 | `tar --to-command=…`、`find -exec`、`xargs` |
| High 下限 | `risk_level = "high"` なら許可 | `bash -c …`、`timeout 5 <cmd>`、`ssh -o ProxyCommand=…`、`rsync -e …` |

`ssh -o ProxyCommand=…` と `rsync -e …` は、オプションの値からヘルパーのコマンドを取り出して評価する（以下「ヘルパーコマンドの評価」）。値に特権昇格のトークンがあれば Critical、安全に分割できない値は Reject、それ以外は High 下限である。

### 問題

名前で High 以上になるコマンドは閉じたリストで定義されている。`sed`、`awk` 系、`busybox`/`toybox`、`vi`/`vim` 系、`sg`/`newgrp`、`git` は、High 以上にするどのリストにも含まれない（`git` はネットワークの条件付きプロファイル（Medium）だけを持つ）。これらは引数で与えた文字列を `sh -c` などで実行できる。

バイナリ解析は、`dlopen`/`dlsym` のインポート、libc のインポートから照合した `execve` などの syscall、`svc #0x80`、`PROT_EXEC` の `mprotect` を High の信号とする。`system`・`popen`・`execl`・`fork` のように、syscall を直接発行しない libc 関数のインポートは信号にならない。このため、同じツールでも、ビルドがどの関数をインポートするかによって High になったりならなかったりする。

`ssh` 系と `tar` のヘルパー起動オプションも、一部しか認識していない。`scp -S`、`sftp -S`/`-D`、`ssh -o KnownHostsCommand`、`tar -I`（`--use-compress-program`）などは認識していない。`tar --to-command` も、正式な綴りしか認識しない。GNU の `getopt_long` は一意な略記（`--to-comm=…`）を受理し、GNU tar は旧形式（先頭に `-` のない最初の引数）のオプションも受理する。

脅威モデルの攻撃者 B（設定ファイルの作成者）に対して、runner は「宣言した `risk_level` を超える操作（シェル取得など）を紛れ込ませられない」ことを保証する（監査文書 §2.1）。上の分類漏れにより、`risk_level` を省略した（`low` の）コマンドや `medium` のコマンドから、任意のコマンドを runner の実行主体（setuid なら root）で実行できる。

### 現状の実測

監査が挙げた形態と、調査中に見つけた同種の形態を、現行のコード（`c551855`）で実測した。環境は Ubuntu 24.04（GNU sed 4.9、mawk 1.3.4、VIM 9.1、GNU tar 1.35、git 2.43.0）である。各バイナリと設定を `record` し、`risk_level` を省略した設定を `runner --dry-run` で評価した。

| 形態 | 実行される内容 | 算出リスク | 主な根拠 |
|---|---|---|---|
| `sed '1e /usr/bin/id > /tmp/x' /etc/hostname` | `e` コマンドが `sh -c` で実行 | `low`（許可） | なし |
| `mawk 'BEGIN{system("id")}'` | `system()` が `sh -c` で実行 | `low`（許可） | なし |
| `vim.basic -es -c '!id' -c q` | `!` がシェルで実行 | `high` | バイナリ解析の `dlopen`（ビルド依存） |
| `sg root -c id`（`newgrp` への symlink） | 指定グループでシェルを実行 | `medium` | バイナリ解析のネットワークシンボル |
| `git -c core.fsmonitor=/tmp/evil status` | フックとして実行 | `high` | バイナリ解析の `execve`（ビルド依存） |
| `tar -I /tmp/evil -tf <archive>` | 圧縮プログラムとして実行 | `medium` | バイナリ解析のネットワークシンボル |
| `tar --to-comm=id -xf <archive>` | `--to-command` の略記 | `high`（Reject にならない） | フラグ解析の fail-closed |
| `tar cIf /tmp/evil <archive> <file>` | 旧形式の `-I` | `high`（Reject にならない） | フラグ解析の fail-closed |
| `sed --sandbox -n 1p /etc/hostname` | 実行なし（`e`/`r`/`w` を拒否） | `low` | なし |

`tar --use-compr=…`（`--use-compress-program` の略記）、`tar cIf …`、`tar --to-comm=…` が実際にコマンドを実行することは、GNU tar 1.35 で確かめた。`sed --sandbox` が `e` を含むスクリプトを `e/r/w commands disabled in sandbox mode` で拒否することは、GNU sed 4.9 で確かめた。

`vim` と `git` が High になるのは、このビルドが `dlopen` や `execve` をインポートしているためである。同じ名前のツールでも、別のビルドで High になる保証はない。

## 目的

- 監査が挙げたプロセス生成機能を持つ一般ツールを、ビルドに依存せず、コマンドの名前と引数から High 以上に分類する。
- `ssh` 系と `tar` のヘルパー起動オプションを、既存の `-o ProxyCommand` と `--to-command` と同じ扱いにする。オプションの綴りの違い（略記・束ね・旧形式）で扱いが変わらないようにする。
- 正当な用途の逃げ道を残す。`sed` は `--sandbox` を付ければ本規則で引き上げない。

## 用語

- **名前集合:** 設定のパスから symlink を辿って得た名前の集合。設定のパスの basename と、各 symlink の先の basename からなる。既存の名前による分類と同じものを使う。
- **設定の basename:** 設定のパス（symlink を辿る前）の basename。
- **最初のオペランド:** コマンドの引数を先頭から読んだとき、オプションでもオプションの値でもない最初のトークン。`--` より後のトークンはすべてオペランドである。
- **ヘルパーコマンドの評価:** 既存の `ssh -o ProxyCommand`・`rsync -e` と同じ評価。オプションの値からコマンドを取り出し、特権昇格のトークンがあれば Critical、安全に分割できなければ Reject、それ以外は High 下限とする。
- **ラッパーのインナーの評価:** 既存の `timeout`・`nice` などのラッパーと同じ評価。インナーのコマンドが特権昇格のトークンなら Critical、インナーが Reject の形態なら Reject、それ以外は内容によらず High 下限とする（`risk_assessment.ja.md` §8.3）。

## スコープ

### 対象

1. **名前による High:** `awk` 系、`vi`/`vim` 系、`git` を、引数とバイナリ解析の結果によらず High にする（F-001・F-002・F-006）。
2. **`sed`:** `--sandbox` がない呼び出しを High にする（F-003）。
3. **`busybox`/`toybox`:** マルチコールの入口として呼んだときの applet を、ラッパーのインナーとして評価する。`busybox` のシェル applet `hush` を、シェルとして扱う（F-004）。
4. **`sg`/`newgrp`:** 特権昇格として Critical にする（F-005）。
5. **`ssh` 系のヘルパー起動オプション:** `scp -S`、`sftp -S`、`sftp -D`、`ssh`/`scp`/`sftp` の `-o KnownHostsCommand` を、ヘルパーコマンドの評価の対象に加える（F-007）。
6. **`tar` のヘルパー起動オプション:** `-I`/`--use-compress-program`、`-F`/`--info-script`/`--new-volume-script`、`--rsh-command`、`--rmt-command` を、既存の `--to-command`/`--checkpoint-action` と同じく Reject にする。略記・束ね・旧形式の綴りを、既存の 2 つを含めて認識する（F-008）。
7. **経路の一貫性:** 上の分類が、ラッパーのインナーと shebang のインタプリタとして現れた場合にも適用される（F-009）。
8. **文書:** `docs/user/risk_assessment.ja.md` の算出ルールと移行ノートを更新する。英語版は `/mktrans` で反映する（F-010）。

### 対象外

- **プロセス生成のインポートによる引き上げ:** 監査が中長期の対策として挙げた「プロファイルのないバイナリが `execve`/`posix_spawn`/`system` をインポートしていれば少なくとも Medium」とする規則。record はインポートしたシンボルを保持しているので実装はできるが、`tar`・`newgrp`・`git` など多くのツールの分類が変わる。影響を実際の設定で測ってから、別タスクで判断する（決定事項 8）。
- **`git -c` の設定キーの解析:** `git` は名前だけで High にする。実行ファイルを指す設定キーを解析して Reject にする方式は採らない（決定事項 5）。
- **`GIT_CONFIG_*` などの環境変数:** SA-13（[#1241](https://github.com/isseis/go-safe-cmd-runner/issues/1241)）で扱う。
- **子プロセスの `PATH`:** `tar -z` が `gzip` を `PATH` から探すような経路は、SA-05（[#1232](https://github.com/isseis/go-safe-cmd-runner/issues/1232)）で扱う。
- **`gawk --sandbox`・`mawk -W sandbox` の例外:** `awk` 系は例外なく High とする（決定事項 2）。
- **`busybox` の applet の安全リスト:** `coreutils` のような applet ごとの Low の分類は設けない（決定事項 3）。
- **同種の候補:** 調査中に見つけた、同じく閉じたリストから漏れているツールは、付録 B に挙げ、本タスクでは扱わない。

## 変更の効果

本節は規範ではない例示であり、要件は決定事項と受け入れ基準が定める。「変更後」は、決定事項の提案どおりに承認された場合に期待される結果であり、実装で確かめた結果ではない。

| 形態 | 変更前（実測） | 変更後 |
|---|---|---|
| `sed '1e id' f` | `low` | `high` |
| `sed --sandbox -n 1p f` | `low` | `low`（本規則では引き上げない） |
| `mawk 'BEGIN{system("id")}'`、`awk …`（実体が `mawk`） | `low` | `high` |
| `vim.tiny -es -c '!id' -c q` | ビルド依存 | `high` |
| `busybox sh -c id` | ビルド依存 | `high` |
| `busybox su` | ビルド依存 | Critical |
| `/bin/ls`（`busybox` への symlink） | ビルド依存 | 変わらない |
| `sg root -c id` | `medium` | Critical |
| `git status` | ビルド依存（実測環境では `high`） | `high` |
| `scp -S /tmp/prog host:f .` | `medium` | `high` 下限（ヘルパーコマンドの評価） |
| `tar -I /tmp/evil -tf a.tar` | `medium` | Reject |
| `tar --to-comm=id -xf a.tar` | `high` | Reject |
| `tar --checkpoint=10 -cf a.tar d` | 変わらない | 変わらない（`--checkpoint-action` の略記ではない） |

既存の設定への影響:

- `sed`・`awk`・`git` を `risk_level` 省略または `medium` で使っている設定は拒否される。`risk_level = "high"` を設定するか、`sed` には `--sandbox` を付ける必要がある。実測環境の `git` は既に `high` なので、その環境では `git` の設定に影響はない。
- `tar -I zstd` のように外部の圧縮プログラムを指定する設定は、`risk_level` によらず拒否される。`--zstd`・`-z`・`-j`・`-J` などの組み込みの圧縮オプションに置き換える必要がある。
- リポジトリの `sample/` と `docs/user/` の設定例に、`sed`・`awk`・`git`・`tar -I` を使う例はない（`tar` の例はあるが、`-I` などのヘルパー起動オプションを使わない）。

## 決定事項

各項目の「提案」は本書のレビューで承認を受ける。承認後に「決定」へ改める。

### 1. 分類の水準

**提案:** 名前による High を基本とする。Reject は、既存の Reject と同じ性質の形態（ツール自身の子プロセスがオプションの値のプログラムを起動し、runner がそのコマンドを取り出して評価しない形態）にだけ使う。特権昇格は Critical とする。

**理由:** 攻撃者 B に対する保証は、宣言した `risk_level` を超える操作をさせないことである。`high` は、シェルやインタプリタの実行を許すレベルとして既に定義されている（`risk_assessment.ja.md` §2）。プロセス生成の機能を持つツールを High にすれば、`low`/`medium` を宣言したコマンドからシェルを得る経路はなくなる。パッケージマネージャや `systemctl` も、引数を解析せず名前だけで High にしている（`risk_assessment.ja.md` §8.4）。

### 2. `awk` 系と `vi`/`vim` 系

**提案:** 付録 A の名前を、引数とバイナリ解析の結果によらず High とする。`gawk --sandbox`・`mawk -W sandbox`・`rvim` などの制限モードも例外としない。

**理由:** どちらもスクリプト言語のインタプリタであり、既存の `python`・`perl` と同じ扱いになる。制限モードの有無は実装ごとに異なり、`awk` という名前の実体が `gawk` か `mawk` かも環境で異なる。`rvim` もシェルコマンドは禁止するが、任意のファイルへの書き込みはできる。

### 3. `busybox`/`toybox`

**提案:** 設定の basename が `busybox` または `toybox` で始まるとき、マルチコールの入口として呼んだものとみなす。このとき、最初の引数を applet とし、applet とそれ以降の引数をラッパーのインナーとして評価する。applet がない場合、または最初の引数が `-` で始まる場合（`--list`・`--install` など）は High 下限とする。applet の symlink 経由で呼んだ場合（設定の basename が applet の名前）は、本規則の対象とせず、既存の名前による分類に任せる。`busybox`・`toybox` の名前そのものは、名前による分類のどのリストにも加えない。

**理由:** `busybox` は argv[0] の basename が `busybox` で始まるとき、最初の引数を applet として実行する。`toybox` も同様である。Alpine などでは `/bin/ls` や `/bin/sh` が `busybox` への symlink であり、名前集合に `busybox` が含まれる。`busybox` をリストに加えると、これらのコマンドがすべて High になる。symlink 経由の呼び出しは、名前集合の applet の名前（`sh`・`awk`・`vi` など）で既に分類できる。`busybox` のシェル applet のうち `ash`・`sh` は既存のシェルのリストにあるが、`hush` はないので加える。

applet ごとの安全リスト（`coreutils` の方式）は設けない。入口として `busybox <applet>` と書く設定は少なく、applet の数だけ保守が増えるためである。

### 4. `sed`

**提案:** 名前集合に `sed` または `gsed` を含むコマンドは、`--sandbox` がない限り High とする。`--sandbox` があるとみなすのは、次をすべて満たす場合に限る。

- 正確に `--sandbox` という綴りのトークンである。略記（`--sand` など）は認めない。
- `sed` 自身のオプションとして、最初のオペランドより前にある。オプションの値（`-e --sandbox` の `--sandbox` はスクリプトである）や、`--` より後のトークンは数えない。
- それより前に、引数の個数が分からない未知のオプションがない。未知のオプションがあれば High とする。

スクリプトの内容（`e` コマンドや `s///e` の有無）は解析しない。

**理由:** `sed` は設定で多用されるので、一律に High にすると多くの設定に `risk_level = "high"` が必要になる。GNU sed の `--sandbox` は、`e`/`r`/`w` コマンドと `s` の `e`/`w` フラグを含むスクリプトを、実行の前に拒否する。スクリプトの解析は、`-f` で与えたファイルの内容に依存し、`sed` の文法の解釈を誤ると取りこぼす。

`--sandbox` を最初のオペランドより前に限るのは、環境変数 `POSIXLY_CORRECT` があると GNU の `getopt` が最初のオペランドでオプションの解析をやめるためである。最初のオペランドより前にあれば、どちらのモードでもオプションとして解析される。

**既知の制約:** BSD sed（macOS）は `--sandbox` を持たず、`e` コマンドも持たない。macOS の `sed` は `--sandbox` を付けても起動に失敗するので、`risk_level = "high"` が必要になる。名前からは GNU sed と BSD sed を区別できないため、安全側に倒す。

**代替案:** (a) `sed` を一律に High とする。最も単純だが、上の理由で設定への影響が大きい。(b) スクリプトを解析して `e` を検出する。上の理由で採らない。

### 5. `git`

**提案:** 名前集合に `git`、または `git-` で始まる名前を含むコマンドを、引数とバイナリ解析の結果によらず High とする。`-c`・`--config-env`・`--exec-path` などの引数は解析しない。

**理由:** `git` は、`-c` の設定キー（`core.fsmonitor`・`core.sshCommand`・`alias.*` など）に加えて、リポジトリの設定ファイル、フック、`--exec-path`、別のコマンドで書き込んだ設定（`git config core.fsmonitor …` の後の `git status`）からも外部のプログラムを実行する。引数だけを解析しても、これらの経路は閉じられない。名前で High にすれば、`low`/`medium` の宣言から外部のプログラムを実行する経路はすべて閉じる。実測環境では `git` はバイナリ解析で既に High であり、その環境では挙動が変わらない。

`git-` で始まる名前を含めるのは、`git-upload-pack`・`git-receive-pack`（`git` への symlink）や、`/usr/lib/git-core/git-*`（`git` のハードリンク。名前集合に `git` を含まない）、`git-shell`（別のバイナリ）があるためである。

**代替案:** 監査は、実行ファイルを指す設定キーを `git -c` で与えた場合を Reject とすることも挙げている。`risk_level = "high"` を宣言した `git` からも外部のプログラムを実行させないことになり、保証は強くなる。ただし、キーの一覧は閉じたリストになり（`alias.*`・`include.path`・`filter.*`・`*.command`・`safe.directory` など）、`git -c core.sshCommand="ssh -i <key>"` のような正当な用途も拒否される。攻撃者 B に対する保証には名前による High で足りるので、本タスクでは採らない。なお、環境変数 `GIT_SSH_COMMAND` は既に denylist で拒否されており、`-c core.sshCommand` との扱いの差は残る。

### 6. `sg`/`newgrp`

**提案:** `sg`・`newgrp` を特権昇格のコマンドとし、既存の `sudo`・`runuser`・`setpriv` などと同じく Critical とする。ラッパーのインナーとして現れた場合も Critical とする。

**理由:** どちらも setuid root のヘルパーであり、プロセスのグループ資格を変えてシェルやコマンドを実行する。runner はグループの切り替えを `run_as_group` として持っており、設定から `sg`/`newgrp` を呼ぶ正当な理由はない。

### 7. ヘルパー起動オプション

**提案:**

- `ssh` 系: `scp -S <program>`、`sftp -S <program>`、`sftp -D <command>`、`ssh`/`scp`/`sftp` の `-o KnownHostsCommand=<command>` を、ヘルパーコマンドの評価の対象とする。既存の `-o ProxyCommand`/`LocalCommand` と同じく、該当するオプションがすべて評価され、最も厳しい結果を採る。
- `tar`: `-I`/`--use-compress-program`、`-F`/`--info-script`/`--new-volume-script`、`--rsh-command`、`--rmt-command` を、既存の `--to-command`/`--checkpoint-action` と同じく Reject とする。認識する綴りは次のとおり。既存の 2 つにも適用する。
  - 正式な綴り（`--use-compress-program=X`・`--use-compress-program X`）
  - 短いオプションの分離・連結・束ね（`-I X`・`-IX`・`-cI X`）
  - 長いオプションの略記（オプション名の先頭部分。ただし、`tar` の他の正式なオプション名と完全に一致するもの（例: `--checkpoint`）は除く）
  - 旧形式（最初の引数が `-` で始まらないとき、その中の `I`・`F` の文字）

**理由:** `ssh` 系の追加は、既存の `-o ProxyCommand` と同じくコマンドを取り出せる形態である。`tar` の追加は、既存の `--to-command` と同じく `tar` の子プロセスが `sh -c` でコマンドを実行する形態であり、`tar` の中で扱いを揃える。`tar -I` の正当な用途には、組み込みの圧縮オプション（`--zstd` など）という代わりがある。

略記を広く認識するのは安全側である。`tar` が略記として受理するならヘルパー起動オプションであり、曖昧な略記は `tar` 自身がエラーにする。他の正式なオプション名と一致する場合は、`getopt_long` が完全一致を優先するので除く。

### 8. プロセス生成のインポートによる引き上げ

**提案:** 本タスクでは扱わず、別の issue とする。

**理由:** 名前によらない最後の砦として有効だが、`tar`・`newgrp`・`git`・`vim` など、`fork`/`exec*`/`system`/`popen` をインポートする多くのツールの分類が変わる。どのシンボルを信号とするか（`fork` 単独を含めるか、`posix_spawn` を含めるか）と、既存の設定への影響を測ってから決める必要がある。本タスクの名前による分類とは独立に追加できる。

## 受け入れ基準（Acceptance Criteria）

#### F-001: `awk` 系

**Acceptance Criteria**:
- **AC-01**: 名前集合に付録 A.1 の名前のいずれかを含むコマンドは、引数とバイナリ解析の結果によらず High 以上と評価され、評価の理由コードに `arbitrary_code_execution` が含まれる。

#### F-002: `vi`/`vim` 系

**Acceptance Criteria**:
- **AC-02**: 名前集合に付録 A.2 の名前のいずれかを含むコマンドは、引数とバイナリ解析の結果によらず High 以上と評価され、評価の理由コードに `arbitrary_code_execution` が含まれる。

#### F-003: `sed`

**Acceptance Criteria**:
- **AC-03**: 名前集合に `sed` または `gsed` を含み、決定事項 4 の `--sandbox` の条件を満たさないコマンドは、High 以上と評価され、評価の理由コードに `arbitrary_code_execution` が含まれる。少なくとも次の形態が High になる: `sed '1e id' f`、`sed -n 1p f`、`sed 's/a/b/' --sandbox f`、`sed -e --sandbox f`、`sed -- --sandbox f`、`sed --sand -n 1p f`。
- **AC-04**: `--sandbox` より前に、引数の個数が分からない未知のオプションがあるコマンドは、High 以上と評価される。
- **AC-05**: 決定事項 4 の `--sandbox` の条件を満たすコマンドは、本規則では引き上げられない。少なくとも次の形態で成り立つ: `sed --sandbox -n 1p f`、`sed -n --sandbox -e p f`、`sed -e p --sandbox f`、`sed -i.bak --sandbox 's/a/b/' f`。他の次元（バイナリ解析など）による評価は変わらない。

#### F-004: `busybox`/`toybox`

**Acceptance Criteria**:
- **AC-06**: 設定の basename が `busybox` または `toybox` で始まり、最初の引数が `-` で始まらないコマンドは、その引数を applet とし、applet と以降の引数をラッパーのインナーとして評価される。少なくとも次が成り立つ: `busybox sh -c id` は High 以上、`busybox su` は Critical、`busybox xargs id` は Reject、`busybox ls` は High 以上。
- **AC-07**: 設定の basename が `busybox` または `toybox` で始まり、引数がない、または最初の引数が `-` で始まるコマンドは、High 以上と評価される。
- **AC-08**: 設定の basename が `busybox`・`toybox` で始まらず、symlink の先が `busybox` であるコマンドは、AC-06・AC-07 の規則の対象にならない。`busybox` への symlink である `ls` は、本タスクの規則では引き上げられない。
- **AC-09**: 名前集合に `hush` を含むコマンドは、既存のシェル（`ash` など）と同じく、名前で High 以上と評価され、`-c` のインラインコードとしても扱われる。

#### F-005: `sg`/`newgrp`

**Acceptance Criteria**:
- **AC-10**: 名前集合に `sg` または `newgrp` を含むコマンドは、`risk_level` によらず Critical として拒否され、評価の理由コードに `privilege_escalation` が含まれる。
- **AC-11**: `sg` または `newgrp` がラッパーのインナーとして現れるコマンド（例: `timeout 5 sg g -c id`、`env newgrp g`）は、Critical として拒否される。

#### F-006: `git`

**Acceptance Criteria**:
- **AC-12**: 名前集合に `git` を含むコマンド、または `git-` で始まる名前を含むコマンドは、引数とバイナリ解析の結果によらず High 以上と評価され、評価の理由コードに `arbitrary_code_execution` が含まれる。少なくとも次の形態で成り立つ: `git status`、`git -c core.fsmonitor=/tmp/evil status`、`git --config-env=core.pager=X log`、`git-upload-pack .`（名前集合に `git` を含まない場合を含む）。

#### F-007: `ssh` 系のヘルパー起動オプション

**Acceptance Criteria**:
- **AC-13**: `scp -S <value>`、`sftp -S <value>`、`sftp -D <value>`、および `ssh`/`scp`/`sftp` の `-o KnownHostsCommand=<value>`（`-o KnownHostsCommand <value>`・`-oKnownHostsCommand=<value>`・大文字と小文字の違いを含む）は、ヘルパーコマンドの評価を受ける。値が特権昇格のトークンなら Critical、安全に分割できなければ Reject、それ以外は High 以上と評価される。
- **AC-14**: 1 つのコマンドに AC-13 のオプションや既存の `-o ProxyCommand`/`LocalCommand` が複数あるとき、すべてが評価され、最も厳しい結果が採られる。無害な値の後に特権昇格のトークンを含む値を置いても Critical になる。
- **AC-15**: AC-13 のオプションも既存の `-o ProxyCommand`/`LocalCommand` もない `scp`・`sftp` の評価は、変更前と同じである。

#### F-008: `tar` のヘルパー起動オプション

**Acceptance Criteria**:
- **AC-16**: `tar` の `-I`/`--use-compress-program`、`-F`/`--info-script`/`--new-volume-script`、`--rsh-command`、`--rmt-command`、`--to-command`、`--checkpoint-action` は、`risk_level` によらず Reject と評価される。動作モード（`-c`・`-x`・`-t` など）によらない。
- **AC-17**: AC-16 の判定は綴りによらない。少なくとも次の形態が Reject になる: `-I X`、`-IX`、`-cI X`、`--use-compress-program=X`、`--use-compress-program X`、`--use-compr=X`、`--to-comm=X`、`--checkpoint-act=exec=X`、`--info-scr=X`、旧形式の `cIf X a.tar f`・`xFf X a.tar`。
- **AC-18**: 次の形態は AC-16 の対象とならず、変更前と同じ評価になる: `--checkpoint`・`--checkpoint=10`（他の正式なオプション名と完全一致）、`--` より後のトークン、既知の値を取るオプションの値（例: `-C --to-command` の `--to-command` はディレクトリ名である）。

#### F-009: 経路の一貫性

**Acceptance Criteria**:
- **AC-19**: F-001〜F-003、F-006 の規則は、shebang のインタプリタとして現れたコマンドにも適用される。少なくとも次が High 以上になる: `#!/usr/bin/awk -f` のスクリプト、`#!/bin/sed -f` のスクリプト（`--sandbox` がない）。
- **AC-20**: 名前による規則（F-001〜F-003、F-005、F-006、AC-09）は、設定のパスが symlink（例: `/usr/bin/awk` → `/etc/alternatives/awk` → `/usr/bin/mawk`、`/usr/bin/sg` → `newgrp`）の場合も、symlink の先の実体のパスの場合も、同じ結果になる。

#### F-010: 文書

**Acceptance Criteria**:
- **AC-21**: `docs/user/risk_assessment.ja.md` の §3.1 に、F-001〜F-008 の分類（対象の名前・オプション、算出リスク、`sed --sandbox` の条件、`busybox`/`toybox` の入口の判定）が記載されている。
- **AC-22**: `docs/user/risk_assessment.ja.md` の §8（移行ノート）に、変更前に許可されていて変更後に拒否される形態と、その対処（`risk_level = "high"`、`sed --sandbox`、`tar` の組み込みの圧縮オプション）が記載されている。決定事項 4 の既知の制約（BSD sed）と、付録 B の候補が本タスクの対象外であることが記載されている。
- **AC-23**: `docs/user/risk_assessment.md`（英語版）は、AC-21・AC-22 について日本語版と同じ内容である。

#### F-011: 全体の健全性

**Acceptance Criteria**:
- **AC-24**: `make test`・`make lint` が通る。

### テストの入力についての制約

- 名前による規則を確かめるテストは、バイナリ解析だけでは High にならない record（`dlopen`・`execve` などの信号を持たない record）を使う。そのうえで、同じ record と引数で、名前だけを対象外の名前に変えたコマンドが High にならないことを先に確かめる（CLAUDE.md「A layered path needs inputs only one layer can handle」）。実測環境の `vim.basic` や `git` のように、バイナリ解析で既に High になるバイナリの実物を使ったテストは、本タスクの規則の効果を示さない。
- `sed --sandbox` の例外を確かめるテストは、`--sandbox` の位置だけが異なる 2 つの入力の組を使い、一方だけが引き上げられることを示す。
- `tar` の綴りのテストは、他の次元（フラグ解析の fail-closed による High）では Reject にならないことを示すため、評価の結果が Reject であること（High ではないこと）を確かめる。

## Success Criteria（要件レベル）

- 監査が挙げた形態（`sed` の `e`、`awk` の `system()`、`busybox sh -c`、`vim -c '!cmd'`、`sg -c`、`git -c`、`scp -S`）と `tar -I` は、`risk_level` を省略した設定でも `medium` の設定でも実行されない。
- 分類はコマンドの名前と引数で決まり、バイナリがどの libc 関数をインポートするかに依存しない。
- `sed --sandbox` の呼び出しと、`busybox` の applet の symlink 経由の呼び出しは、本タスクの規則では引き上げられない。
- ヘルパー起動オプションの扱いは、オプションの綴りによらない。

## 付録 A: 名前による High の対象

### A.1 `awk` 系

`awk`、`gawk`、`mawk`、`nawk`、`original-awk`、`goawk`

### A.2 `vi`/`vim` 系

`vi`、`vim`、`view`、`ex`、`rvim`、`rview`、`vimdiff`、`gvim`、`gview`、`gvimdiff`、`evim`、`eview`、`nvim`、`nvi`、`nex`、`nview`、`vim.basic`、`vim.tiny`、`vim.nox`、`vim.gtk`、`vim.gtk3`、`vim.athena`、`vim.motif`

`vim.*` は Debian/Ubuntu の alternatives の実体の名前である。設定が `/usr/bin/vi` などの symlink を指す場合は、名前集合に `vi` が含まれるので、実体の名前が一覧になくても High になる。実体の名前は、実体のパスを直接設定した場合のために挙げる。

## 付録 B: 同種の候補（本タスクの対象外）

調査中に見つけた、引数からコマンドを実行できるが、名前による分類のどのリストにも含まれないツールである。名前は確認したが、各環境での現在の算出リスクは実測していない。必要に応じて別の issue とする。

| 分類 | ツール | 実行の経路 |
|---|---|---|
| シェル | `mksh`、`oksh`、`loksh`、`yash`、`posh`、`toysh` | シェルそのもの |
| エディタ・インタプリタ | `emacs`（`--batch --eval`）、`ed`、`m4`（`syscmd`）、`dc`（`!`）、`gdb`（`-ex 'shell …'`）、`expect` | 組み込みのコマンド実行 |
| ページャ | `less`、`more`、`man` | `!` コマンド、`LESSOPEN` |
| データベースクライアント | `sqlite3`（`.shell`）、`psql`（`\!`）、`mysql`（`system`） | 組み込みのコマンド実行 |
| 内部コマンドを実行するツール | `script -c`、`strace`、`ltrace`、`valgrind`、`perf record`、`parallel`、`run-parts`、`zip -TT` | 引数のコマンドを子プロセスで実行 |
| `ssh` 系の追加の経路 | `-F <config>`（設定ファイルの `ProxyCommand`）、`-I`/`-o PKCS11Provider`/`-o SecurityKeyProvider`（共有ライブラリの読み込み） | 未検証の設定ファイル、`dlopen` |
| オプションの略記 | `rsync` の長いオプションの略記（`--rs=…` など） | `-e`/`--rsh` の判定の綴り |
