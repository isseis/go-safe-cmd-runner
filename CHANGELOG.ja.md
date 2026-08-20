# 変更履歴

このプロジェクトの主な変更は、このファイルに記載されます。

形式は [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) に準拠し、
このプロジェクトは [Semantic Versioning](https://semver.org/spec/v2.0.0.html) に従います。

## [未リリース]

### 破壊的変更

#### `runner`: `--run-id` の受理形式を `^[A-Za-z0-9_-]{1,64}$` に限定

`--run-id` に指定できる値が、英大文字（`A-Z`）・英小文字（`a-z`）・数字（`0-9`）・アンダースコア（`_`）・ハイフン（`-`）のみで構成された1〜64文字の文字列に限定されました。この形式に合致しない値を指定した場合、実行は開始されずにエラー終了します。

**影響範囲:** 自動生成以外の値を `--run-id` に渡しているCI・運用スクリプトが影響を受けます。渡している値が上記の形式に収まるかを確認してください。自動生成されるULID（26文字のCrockford Base32）や、利用者向け文書が推奨する例（`my-custom-run-001`、`gh-<GitHub ActionsのRun ID>`、`jenkins-<ビルド番号>`、`backup-<タイムスタンプ>`）はいずれも受理形式に含まれます。

#### `verify`: ハッシュディレクトリの権限違反をfail-closed化

ハッシュディレクトリまたはその祖先ディレクトリに権限違反が検出された場合、`verify` は対象ファイルを1件も検証せずに終了コード 3 で終了するようになりました。対象ファイルの祖先ディレクトリのみに違反が検出された場合は、従来どおり警告を記録したうえで検証を継続します（終了コードは変わりません）。バイパス手段は用意されていません。

**アップグレード前に影響有無を判定する手順:**

現行版の `verify` を対象ファイルに対して実行し、標準エラー出力の `TOCTOU permission check violation` 警告を確認してください。

```bash
# ハッシュディレクトリを明示して実行し、TOCTOU警告の有無を確認
verify -d <ハッシュディレクトリ> <検証対象ファイル> 2>&1 | grep "TOCTOU permission check violation"
```

（ハッシュディレクトリを明示していない場合は、既定値 `/usr/local/etc/go-safe-cmd-runner/hashes` を指定してください。）

この出力が空であれば、アップグレード後も影響はありません。出力がある場合、警告の `path` がハッシュディレクトリまたはその祖先を指しているかを確認してください。ハッシュディレクトリ側の違反がある場合は、アップグレード前にハッシュディレクトリの権限を修正するか、権限の適切なパスへハッシュディレクトリを移動してください。対象ファイル側のみの違反であれば、アップグレード後も検証は継続されます。

#### `verify`: ハッシュディレクトリを作成しなくなりました

`verify` は、指定されたハッシュディレクトリが存在しない場合でも、それを作成しなくなりました。存在しない場合は対象ファイルを1件も検証せずに終了コード 3 で終了します。ハッシュ記録は `record` コマンドで作成してください。

検証を1件も実施せずに終了した場合、標準エラー出力のメッセージに `verify-error=<トークン>` の形で原因を表す識別トークンが含まれます。`hash_dir_not_found`（不在）・`hash_dir_unreadable`（読み取り不能）・`hash_dir_permission_violation`（権限違反）・`path_resolution_failed`（パス解決の失敗）・`permission_checker_init_failed`（権限チェッカの初期化失敗）を区別できます。トークンの一覧は [verify コマンド利用ガイド](docs/user/verify_command.ja.md) を参照してください。

**影響範囲:** 「終了コード 3 = 改ざんの可能性」として警報を上げる監視ルールを持つホストでは、ハッシュディレクトリが未作成であるだけの状態でも発報するようになります。監視ルールを識別トークンで分けるか、`record` でハッシュ記録を作成しておいてください。

#### `record`: world-writable な場所のハッシュディレクトリを拒否します

`record` は、ハッシュディレクトリが誰からでも書き込める（world-writable）場合にエラー終了するようになりました。sticky ビットが設定されていても拒否します。対象は次の2つです。

- **既に存在するハッシュディレクトリ自身が world-writable な場合。** 是正するには `chmod go-w <ハッシュディレクトリ>` を実行するか、自分だけが書き込める場所へハッシュディレクトリを移動してください。
- **ハッシュディレクトリが存在せず、その作成先（指定パスのうち実在する最深の祖先）が world-writable な場合。** この場合はディレクトリを作成しません。作成後は上記の world-writable 拒否ではなく通常の祖先権限チェックが働き、こちらは sticky ビットを安全とみなします。そのため作成先に sticky ビットが設定されていれば（`/tmp` などが該当します）、利用者自身が先に `mkdir -m 700 -p <ハッシュディレクトリ>` でディレクトリを作成することで、これまでどおり `record` を実行できます。設定されていない場合は、ディレクトリを作成しても祖先ディレクトリの権限チェックで拒否されるため、`chmod go-w` でその祖先を是正するか、自分だけが書き込める場所をハッシュディレクトリに選んでください。

いずれも、他者が名前を先取りできる状態では、`record` が処理していないファイルのハッシュ記録を先に置かれうるためです。

**影響範囲:** 本番環境の既定のハッシュディレクトリ（`/usr/local/etc/go-safe-cmd-runner/hashes`）は該当しません。`/tmp` 配下などを指定していた場合が該当します。

#### パス解決の変更により新たに権限違反が検出されることがあります

ディレクトリ権限のチェックは、指定されたパスをシンボリックリンクの実体まで解決したうえで行うようになりました。これにより、リンクを経由して指定していたハッシュディレクトリや検証対象ファイルについて、これまで検査されていなかった実体側の祖先ディレクトリが検査されます。

**アップグレード前に影響有無を判定する手順:**

ハッシュディレクトリと検証対象ファイルに `readlink -m` を適用し、その実体の祖先ディレクトリの権限を確認してください。

```bash
# ハッシュディレクトリと検証対象ファイルの実体パスを求める（未作成でも解決できる -m を使う）
readlink -m "$HASH_DIR"
readlink -m "$TARGET_FILE"

# 実体パスの祖先を根まで辿り、他者・グループからの書き込み許可を確認する
# readlink -f はパスの途中が実在しないと失敗して空を返すため、-m を使う
p=$(readlink -m "$HASH_DIR")
while [ -n "$p" ]; do
    ls -ld "$p" 2>/dev/null || echo "(not created yet) $p"
    [ "$p" = / ] && break
    p=$(dirname "$p")
done
```

実体パスが指定したパスと同じであれば、影響はありません。異なる場合は、列挙された祖先ディレクトリに他者からの書き込み許可（`o+w`）や、所有者以外のメンバーがいるグループへの書き込み許可（`g+w`）が付いていないかを確認し、付いていれば `chmod go-w` で是正してください。

### 変更

#### ログファイル名のタイムスタンプが UTC になりました

`runner` が `-log-dir` 配下に作成するログファイル名（`<hostname>_<timestamp>_<run-id>.json`）のタイムスタンプの基準時刻が、ホストのローカル時刻から UTC に変わりました。書式は従来どおり `YYYYMMDDThhmmssZ` で、名前の形からは新旧を区別できません。

**影響範囲:** UTC より進んだタイムゾーンのホストでは、移行直後に、ローカル時刻で作られた既存のファイル名と UTC で作られる新しいファイル名が混在し、ファイル名の辞書順と実際の時系列が一致しない期間が生じます（時差の分だけ続きます）。ログを名前順で並べて処理しているスクリプトがある場合は、この期間だけ順序が入れ替わりうる点に注意してください。

#### 新規作成するハッシュディレクトリのパーミッションが 0700 になりました

新規に作成されるハッシュディレクトリのパーミッションが、作成経路によらず `0700` に揃いました。`record` はこれまでも `0700` で作成していましたが、`verify` は `0750` で作成し（本リリースで `verify` は作成そのものを行わなくなりました）、解析ストア（`internal/fileanalysis`）も `0750` でディレクトリを作成していました。

**影響範囲:** 既存のハッシュディレクトリのパーミッションは変更されません。`0750` で作られた既存のディレクトリはそのまま残るため、必要であれば `chmod 0700 <ハッシュディレクトリ>` で手動で是正してください。ただし `record` の実行者と `runner` の実行者が異なる分離運用では、`0700` に狭めると `runner` がハッシュを読めなくなります。この構成での設定手順は [record コマンド利用ガイド](docs/user/record_command.ja.md) を参照してください。

#### ディレクトリ権限違反のログ文言が変わりました

ディレクトリの権限・所有者・経路要素を検査した結果を報告する文言から、`TOCTOU` という語を取り除きました。この検査は、実行に先立って各ディレクトリを1回ずつ調べる静的な監査であり、検査した時点と使用する時点の観測を突き合わせる TOCTOU 対策ではありません。TOCTOU 対策そのものは、シンボリックリンクを追わないファイル open と、パスを解決し直さずファイル記述子経由で実行する仕組みが担っており、そちらは変わっていません。

- `runner`・`verify`・`record` が違反ごとに出力する WARN: `TOCTOU permission check violation` → `insecure directory permissions`。`path`・`violation` の属性名と値は変わりません。
- `runner` が実行を中止するときの ERROR: `TOCTOU permission check failed: ...` → `directory permission audit failed: ...`。

違反と判定する規則、ログレベル、終了コードは変わりません。検査の対象も結果も従来どおりで、変わったのは文言だけです。

**影響範囲:** 上記の文言でログを検索・照合している監視ルールやスクリプトが影響を受けます。新しい文言に更新してください。なお、本リリースの「`verify`: ハッシュディレクトリの権限違反をfail-closed化」に記載したアップグレード前の判定手順は、アップグレード**前**のバージョンで実行するものなので、そこに書かれた古い文言のままで正しく動作します。

### セキュリティ

#### `groupmembership`: `/etc/group`・`/etc/passwd` の不正形式行をログに記録

非 CGO フォールバック実装（`internal/groupmembership`）は、これまで `/etc/group`・`/etc/passwd` の不正形式行を黙って読み飛ばしていました。現在はファイル名と行番号を添えて `slog.Warn` を出力するため、グループメンバーを隠してしまう破損・手編集ミスのエントリがログで検知できるようになりました（以前は「メンバー 0 人」判定に静かに縮退していました）。

#### 既知の制限: 公式バイナリ（`CGO_ENABLED=0`）はグループメンバーシップで NSS を参照しない

公式リリースバイナリは `CGO_ENABLED=0` でビルドされているため（`.github/workflows/release.yml` 参照）、`internal/groupmembership` はグループメンバーとプライマリグループのユーザーを `/etc/group`・`/etc/passwd` の直接パースのみで列挙します。LDAP や SSSD などの NSS（Name Service Switch）ディレクトリサービスは参照しません。グループメンバーシップを NSS のみで管理している環境では、ローカルファイルに現れないメンバーがカウントされず、group-writable なファイルの書き込み安全性判定（`CanUserSafelyWriteFile`）が実際より緩く評価されることがあります。NSS 管理のグループメンバーシップに依存する環境では、`CGO_ENABLED=1` でのセルフビルドを検討してください。詳細は [security-risk-assessment.ja.md](docs/user/security-risk-assessment.ja.md) を参照してください。

## [1.1.1] - 2026-08-03

### 破壊的変更

#### `record` / `verify`: SUDO_UID は存在するユーザーを参照する必要があります

`record` または `verify` がルートとして実行され、ファイル読み取り権限チェックの基準UID として `SUDO_UID` の値を使用する場合、その UID は使用前にユーザーデータベース上に存在することが確認されます。存在しないまたは解決不可能な `SUDO_UID` 値を持つ呼び出しは、未検証の値を無言で採用する代わりに直ちに失敗します。

**影響を受けるシナリオ：**
- LDAP/SSSD で管理されるユーザーを使用する非 CGO ビルド
- ユーザーデータベースの一時的な障害
- ルートの cron/systemd 環境から残された古い `SUDO_UID` 値
- `/etc/passwd` を持たないコンテナイメージ

**回避方法：** 環境から `SUDO_UID` を削除します（`sudo env -u SUDO_UID record ...`）ただし、これはグループ書き込み可能ファイルの権限チェック動作を変更することに注意してください。

### 変更

#### `sudo runner`: ファイル読み取り権限チェックの基準UID が `SUDO_UID` を読み取らなくなりました

`runner` は、ファイル読み取り権限チェックの基準UID を決定する際に `SUDO_UID` 環境変数を読み取らなくなりました。`sudo runner` の下では、基準UID は呼び出し元ユーザーから `0`（root）に変更されます。これにより、root がファイルのグループに属さない場合、グループ書き込み可能ファイルの読み取りが拒否される可能性があります。

意図された動作（`install -m 4755` でインストールされたセットUID `runner` を起動する通常ユーザー）は影響を受けません。ルートの cron からの直接実行も影響を受けません。

`record` および `verify` の動作は変わりません。これらは引き続き `sudo` 経由で実行する場合、既存の読み取り安全性チェックセマンティクスを保持する、呼び出し元ユーザーの UID を `SUDO_UID` から使用します。

#### 権限チェックは、プロセス自身の UID に対して passwd エントリを必要としなくなりました

以前は、プロセスの実 UID に解決可能な passwd エントリがない場合（CGO 有効時の NSS 失敗、または CGO 無効時の `/etc/passwd` エントリの欠落/不在）、権限チェックは UID の決定に失敗し、ファイルアクセスを拒否して実行を停止していました（フェイルクローズド）。権限チェックに使用される UID は passwd データベースを通じてではなく、カーネルから直接読み込まれるようになりました（`os.Getuid()`）。そのため、この障害モード は発生しなくなり、この特定の障害に対して実行は継続されます（フェイルオープン）。

判断に使用される UID、GID、および権限ビット、ならびに判定ルール自体は変わりません。

**グループ書き込み可能ファイル** の判定は依然として passwd エントリを必要とします。これはファイルのグループに属する他のユーザーを特定するためにグループメンバーシップ（`user.LookupId`）を検索するためです。これらのファイルについては、passwd エントリがないと引き続きアクセスが拒否されます（フェイルクローズド）。

#### `record` / `verify`: 起動時の UID 検証とロギング

両方のコマンドは、起動時に権限チェック基準UID を一度だけ解決します。`SUDO_UID` が採用され、実 UID と異なる場合、構造化ログ（`log/slog`、つまり標準エラー）に対して一度だけプロセスごとに警告が発行されます。採用 UID、実 UID、および参照されるユーザーデータベースが記載されます。`sudo` の下では、これは通常の場合であるため、`sudo record` または `sudo verify` の実行ごとに1つのそのような警告を期待してください。これは、権限チェックが使用した UID を記録し、障害ではありません。

**トラブルシューティング：** `SUDO_UID` 実在確認要件の影響を受ける環境の詳細な移行ガイドについては、破壊的変更のセクションを参照してください。

## [1.0.0] - 2026-06-27

### 破壊的変更

#### `slack_allowed_host`: Slack Webhook 通知に必須

`GSCR_SLACK_WEBHOOK_URL_SUCCESS` または `GSCR_SLACK_WEBHOOK_URL_ERROR` 環境変数が設定されている場合、`slack_allowed_host` フィールドを `[global]` セクションで構成する必要があります。Slack Webhook 環境変数が存在するが `slack_allowed_host` が設定されていない場合、起動は設定エラーで失敗します。

**移行:**
設定の `[global]` セクションに `slack_allowed_host` を追加します：
```toml
[global]
slack_allowed_host = "hooks.slack.com"
```

#### ファイルレコードスキーマバージョン: v16 → v17

ファイルレコードの `detected_syscalls` フィールドが再構成されました。前のバージョンで作成されたレコードは、現在の `verify` および `runner` コマンドと互換性がありません。

**移行：** `record --force` を使用してすべてのコマンドを再記録します。

#### リスク評価の動作変更

- **`risk_level = "unknown"` が拒否される**：以前は暗黙的に 0 として処理されていましたが、現在は設定読み込み時に設定エラーで失敗します。
- **不確実なバイナリが拒否される**：バイナリ分析レコードが欠落しているか読み込めないコマンドは、設定される `risk_level` に関わらず、常に実行時に拒否されます。以前は警告とともに許可されていました。
- **`systemctl status`/`show` が Medium に再分類**：これらの読み取り専用サブコマンドは、もはや High として評価されません。これらのコマンドに `risk_level = "medium"` を設定する設定は正しく機能するようになりました。

#### ファイルパス信頼区分リスク昇格（判断軸2）

信頼重要パス（`/etc`, `/usr`, `/lib`, `/boot`, `/var`, `/sbin`、デバイスノードなど）を対象とする書き込み操作は、操作自体（例：`cp`, `install`）が以前に Medium と評価されていても、現在は **High** リスクと評価されます。

**移行：** システムパスに書き込むコマンドを確認します。必要に応じて `risk_level = "high"` を追加するか、最初にコマンドを安全な作業ディレクトリに書き込むように再構成します。

#### ファイル操作コマンドの厳密なフラグ検証

ファイル操作コマンド（例：`ln`, `mkdir`, `chmod`）は現在、実際のフラグ仕様に対して検証されます。存在しないフラグを参照するコマンドは **High** リスクと評価されます（フェイルクローズド）。

**移行：** コマンド引数から無効なフラグを削除するか、リスクが受け入れ可能である場合は `risk_level = "high"` を追加します。

### 追加

#### Slack Webhook URL ホスト許可リスト（`slack_allowed_host`）

新しい `[global]` フィールド `slack_allowed_host` は、環境変数が侵害された場合の SSRF 攻撃を防止し、Slack Webhook 通知を特定のホスト名に制限します。

**設定:**
```toml
[global]
slack_allowed_host = "hooks.slack.com"
```

- 起動は、ホストが `slack_allowed_host` と一致しない Webhook URL を拒否します
- ログ初期化は現在は 2 段階です。コンソール/ファイルロギングが最初に開始され、TOML 検証が成功した後で Slack ロギングが追加されます

#### リスクプロファイル監査ログ

コマンド実行は、完全なリスク評価結果を含む構造化監査ログエントリを発行するようになりました：
- `RiskAuditEntry`、相関フィールド（ULID、コマンドパス、リスクレベル、理由コード）付き
- 理由コードは、許可/拒否判定を決定した特定のルールを特定します
- ファイル操作コマンドのオペランドゾーン情報（safe-zone、ordinary、trust-critical）
- 通常モードと dry-run モードの両方で利用可能

#### Dry-Run 許可/拒否プレビュー

`--dry-run` モードは、通常モードと同じ `StandardEvaluator` を使用してリスクを評価し、実行時に各コマンドが許可されるか拒否されるかを正確に予測するようになりました。以前の分岐実装は不正確な予測を生成する可能性がありました。

#### ファイルパス信頼区分（判断軸2）

ファイル操作コマンドは、コマンドの内在的リスク（判断軸1）に加えて、宛先パスの信頼区分に基づいて評価されるようになりました：

- **Safe-zone**（`/tmp`, 自動生成作業ディレクトリ）：破壊的操作（例：`rm`）は **Low** と評価
- **通常パス**：**Medium** と評価
- **信頼重要パス**（`/etc`, `/usr`, `/lib`, `/boot`, `/var`, `/sbin`、デバイスノード）：書き込み操作は **High** と評価

これにより、一時ファイルをクリーンアップする正規のメンテナンススクリプトが `risk_level = "low"` を使用できるようになり、システムパスへの書き込みは厳密に制御されたままになります。

#### 監査出力のリスク理由コードとオペランドゾーン

リスク監査ログエントリは現在以下を含むようになりました：
- `reason_code`：特定の評価ルール（例：`trust_boundary_write`, `privilege_escalation`, `unknown_binary`）を特定する機械可読コード
- `reason_family`：関連する理由コードのグループ化
- `operand_zones`：ファイル操作のオペランド毎のパス解決と信頼区分分類

### 変更

#### `detected_syscalls` JSON 構造（スキーマ v17）

ファイルレコードの syscall 発生は syscall 番号でグループ化されるようになりました。各 syscall 番号は、検出の詳細を含む `occurrences` 配列で一度だけ表示され、同じ syscall を繰り返しトリガーするバイナリのレコードサイズが削減されます。

**旧形式:**
```json
"detected_syscalls": [
  {"name": "mprotect", "address": "0x1000", ...},
  {"name": "mprotect", "address": "0x2000", ...}
]
```

**新形式:**
```json
"detected_syscalls": [
  {"name": "mprotect", "occurrences": [{"address": "0x1000"}, {"address": "0x2000"}]}
]
```

#### リスク評価の完全性と精度

- コマンドプロファイルのすべてのリスク要因（例：完全なコマンドパス `^/usr/bin/rm$` パターン）は実行時に評価されるようになりました。ベース名マッチングのみではなくなりました。
- リスク評価でのシンボリックリンク解決の失敗は、チェックを静かに迂回するのではなく、安全に実行を拒否するようになりました。
- Dry-run モードは通常モードと同じリスク評価器を使用するようになりました（単一の真実のソース）。

#### フラグ仕様が実 CLI に合致

ファイル操作コマンドのフラグ認識は、実際の CLI ドキュメント（GNU coreutils と uutils の和集合）から派生するようになりました。どちらのリファレンスにも存在しないフラグは認識されなくなり、安全と誤って評価されることを防止します。

#### `verify_standard_paths` 機能の削除

`verify_standard_paths` 設定フィールドと関連するすべてのコードが完全に削除されました。ハッシュ検証は、ディレクトリに関係なく、すべてのコマンドに対して常に実行されるようになりました。

**削除された項目:**
- `GlobalSpec.VerifyStandardPaths` フィールド（TOML: `verify_standard_paths`）
- `DefaultVerifyStandardPaths` 定数
- `DetermineVerifyStandardPaths()` 関数
- `RuntimeGlobal.SkipStandardPaths()` メソッド
- `RuntimeCommand.SkipBinaryAnalysis` フィールド
- `AnalysisOptions.VerifyStandardPaths` フィールド
- `DryRunOptions.VerifyStandardPaths` フィールド
- `PathResolver.ShouldSkipVerification()` メソッド
- `shouldPerformHashValidation()` 関数
- `isStandardDirectory()` 関数と `StandardDirectories` 変数
- `IsNetworkOperation()` の `skipBinaryAnalysis` パラメータ
- `FileVerificationSummary.SkippedFiles` フィールド
- Dry-run JSON 出力の `skipped_files` フィールド

**移行:**
- すべての TOML 設定ファイルから `verify_standard_paths = ...` を削除します。
  このフィールドはもう認識されません。含むと「不明なフィールド」エラーで読み込みに失敗します。
- 上記の削除された型または関数を参照するコードを更新します。

#### Shebang インタプリタ追跡

Shebang インタプリタ追跡を record/verify パイプラインに追加し、スクリプト自体に加えてスクリプトインタプリタの整合性検証を有効にしました。

**機能:**
- 新しい `shebang` パッケージは、スクリプトファイルから shebang 行（`#!`）を解析します
- Shebang インタプリタパスは `record` フェーズ中にコマンドとともに解決され、記録されます
- `VerifyCommandShebangInterpreter` は `verify` 時に記録されたインタプリタを検証します
- Shebang インタプリタへのシンボリックリンクリダイレクト攻撃を検出します（スキーマ v12）
- env 形式 shebangs（`#!/usr/bin/env python3`）を `PATH` 経由で解決することでサポートします
- `PATH` の相対エントリを拒否し、作業ディレクトリ依存の解決を防止します
- グループ実行者検証パイプラインに統合されます

**セキュリティに関する考慮事項:**
- 解析中のシンボリックリンク安全なファイルアクセスに `safefileio.FileSystem` を使用します
- インタプリタパスのシンボリックリンクリダイレクトは検証エラーとして扱われます
- 非再帰的 shebang 検証は無限インタプリタチェーンを防止します

**新しいパッケージ:**
- `internal/shebang`: Shebang 行解析とインタプリタ解決

#### ネットワーク検出のための ELF 動的シンボル分析

未知のコマンドのネットワーク操作検出を改善するため、ELF バイナリ解析機能を追加しました。

**機能:**
- ELF バイナリの `.dynsym` セクションを解析し、ネットワーク関連シンボルを検出します
- Socket API を検出します（socket、connect、bind、listen など）
- DNS 解決関数を検出します（getaddrinfo、gethostbyname など）
- HTTP ライブラリを検出します（libcurl 関数）
- TLS/SSL ライブラリを検出します（OpenSSL、GnuTLS）
- 非 ELF ファイル、静的バイナリ、解析エラーを適切に処理します

**セキュリティに関する考慮事項:**
- シンボリックリンク攻撃防止に safefileio を使用します
- カーネルレベルのパス検証（openat2）があるところで TOCTOU 保護を使用します
- フェイルセーフ動作：解析エラーは潜在的なネットワーク操作として扱われます
- ファイルサイズ制限（1GB）で資源枯渇を防止します

**統合:**
- `IsNetworkOperation()` は未知のコマンドに対して ELF 解析を実行するようになりました
- プロファイルベースの検出は ELF 解析より優先されます
- 静的バイナリ（Go バイナリなど）は特定され、個別に処理されます

**パフォーマンス:**
- 平均解析時間：バイナリあたり 15 マイクロ秒未満
- 無視できるメモリオーバーヘッド

**新しいパッケージ:**
- `internal/runner/security/elfanalyzer`: スタンドアロン ELF 解析パッケージ

#### テンプレート継承の拡張

追加フィールドの継承とマージをサポートするようにコマンドテンプレート機能を拡張しました。

**新しい継承可能フィールド:**
- **WorkDir**: 作業ディレクトリパス
  - 継承モデル：上書き（指定された場合、コマンドレベルの値が優先）
  - `nil`: フィールド未指定、テンプレートから継承可能
  - 空文字列：現在のディレクトリに明示的に設定
  - 非空：指定された絶対パスを使用
- **OutputFile**: コマンド出力キャプチャの出力ファイルパス
  - 継承モデル：上書き（指定された場合、コマンドレベルの値が優先）
  - `nil`: フィールド未指定、テンプレートから継承可能
  - 非空：指定されたファイルパスを使用
- **EnvImport**: 内部変数としてインポートする環境変数のリスト
  - 継承モデル：ユニオンマージ（テンプレートとコマンドレベルのリストを結合）
  - 重複は自動的に削除されます
  - すべての変数は `env_allowed` リストに含まれる必要があります
- **Vars**: 内部変数定義
  - 継承モデル：マップマージ（コマンドレベルの変数は同じキーのテンプレート変数をオーバーライド）
  - テンプレート変数は最初に継承されます
  - コマンドレベルの変数は競合するキーをオーバーライドします

**利点:**
- テンプレートで一般的なフィールドを定義して設定の重複を削減
- 異なるフィールド型に対する柔軟な継承モデル
- 既存設定との下位互換性を維持

**例:**
```toml
[command_templates.build_template]
cmd = "make"
workdir = "/workspace"
env_import = ["cc=CC", "cxx=CXX"]

[command_templates.build_template.vars]
optimization = "O2"

[[groups.commands]]
name = "build-debug"
template = "build_template"
args = ["debug"]
# 継承: workdir="/workspace", env_import=["cc=CC", "cxx=CXX"], vars={optimization: "O2"}

[[groups.commands]]
name = "build-release"
template = "build_template"
args = ["release"]
workdir = "/opt/build"  # テンプレートの workdir をオーバーライド
env_import = ["ldflags=LDFLAGS"]  # テンプレートと結合: ["cc=CC", "cxx=CXX", "ldflags=LDFLAGS"]

[groups.commands.vars]
optimization = "O3"  # テンプレート変数をオーバーライド
```

#### 変数スコープと命名規則

ユーザー定義変数にスコープ分離を適用し、設定エラーを防止するための厳密な命名規則を追加しました。

**機能:**
- **グローバル変数**: 大文字（A-Z）で始まる必要があります
  - `[global.vars]` セクションで定義
  - すべてのグループとコマンド間で利用可能
  - 例：`BackupDir`, `MaxRetries`, `Environment`
- **ローカル変数**: 小文字（a-z）またはアンダースコア（_）で始まる必要があります
  - `[groups.vars]` または `[groups.commands.vars]` セクションで定義
  - スコープ内でのみ利用可能
  - 例：`backup_date`, `_temp_file`, `retry_count`
- **予約プレフィックス**: ダブルアンダースコア（`__`）で始まる変数名はシステム使用に予約されています
- **検証**: スコープ違反は設定読み込み時に明確なエラーメッセージで検出されます

**利点:**
- 変数スコープは変数名から即座に認識可能
- 意図しないスコープ使用を防止
- 設定保守性を向上
- 将来の最適化を可能に（テンプレート変数参照はグローバル変数のみ使用可能）

**例:**
```toml
# グローバル変数（大文字）
[global.vars]
BackupDir = "/data/backups"
MaxRetries = "3"

[[groups]]
name = "backup"

# ローカル変数（小文字またはアンダースコア）
[groups.vars]
backup_date = "20250101"
_temp_file = "/tmp/backup.tmp"

[[groups.commands]]
name = "database_backup"
cmd = "/usr/bin/mysqldump"
args = ["--all-databases", "--result-file=%{BackupDir}/db-%{backup_date}.sql"]
```

**移行ガイド:**
- 設定ファイルのすべての変数定義を確認
- グローバル変数を大文字で始まるように名前変更
- ローカル変数を小文字またはアンダースコアで始まるように名前変更
- 命名違反が検出されると明確なエラーメッセージが報告されます

**ドキュメント:**
- 更新：`docs/user/toml_config/08_variable_expansion.ja.md`
- サンプルファイルは新しい命名規則に従うように更新されました

#### コマンドテンプレート

設定の重複を削減し、保守性を向上させるために、パラメータ置換機能を持つ再利用可能なコマンドテンプレートを追加しました。

**機能:**
- `[command_templates.template_name]` セクションでコマンドテンプレートを定義
- コマンド定義で `template` フィールドを使用してテンプレートを参照
- 3 つのパラメータ型：
  - `${param}`：必須パラメータ（欠落時にエラー）
  - `${?param}`：オプションパラメータ（空の場合は省略）
  - `${@param}`：配列パラメータ（複数の引数に展開）
- エスケープシーケンス：リテラルドルサインに `\$`
- 変数展開（`%{var}`）は `params` 値で許可
- セキュリティ制約：テンプレート定義では `%{var}` 構文は禁止

**例:**
```toml
# テンプレート定義
[command_templates.restic_backup]
cmd = "restic"
args = ["${@flags}", "backup", "${path}"]
env = ["RESTIC_REPOSITORY=${repo}"]

# テンプレート使用
[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
flags = ["-v", "--exclude-caches"]
path = "/data/volumes"
repo = "/backup/repo"
```

**型定義:**
- `runnertypes/spec.go` に `CommandTemplate` struct を追加
- `ConfigSpec` に `CommandTemplates map[string]CommandTemplate` フィールドを追加
- `CommandSpec` に `Template string` と `Params map[string]interface{}` フィールドを追加

**ドキュメント:**
- ユーザーガイド：`docs/user/command_templates.md`
- サンプル設定：`sample/command_template_example.toml`

#### ResolvedPath 型安全性と safefileio API 移行

`common.ResolvedPath` 型をシンボリックリンク解決セマンティクスを持つように強化し、`safefileio` 公開 API を `ResolvedPath` 引数を必須にするように移行しました。

**ResolvedPath struct 変換:**
- `ResolvedPath` は struct（型別名ではない）で、`resolveMode` フィールドを持つようになりました
- 2 つのコンストラクタは構築時に明確なセマンティクスを適用します：
  - `NewResolvedPath(path)`：完全シンボリックリンク解決 — 最終要素を含むすべてのコンポーネントが解決されます
  - `NewResolvedPathParentOnly(path)`：親のみ解決 — 親ディレクトリのみ解決されます；最終コンポーネントはまだ存在しない可能性があります（以前は `NewResolvedPathForNew`）
- `IsParentOnly()` メソッドにより、呼び出し側はどのモードが使用されたかを確認できます
- 書き込み境界関数（`SafeWriteFile`, `SafeWriteFileOverwrite`, `SafeAtomicMoveFile`）は呼び出し時に `IsParentOnly()` を適用し、完全解決パスが提供された場合はエラーを返します

**このワークに含まれるセキュリティ修正:**
- `atomicMoveFileCore` の TOCTOU レースを修正：`fchmod` はパスベースの `chmod` ではなく、開いたファイルハンドル経由で呼び出されるようになりました。レース窓を削除します
- `NewResolvedPathAbsOnly` を削除し、`ResolvedPath` 型保証を保持
- `osFS.AtomicMoveFile` FileSystem ブリッジのセキュリティ回帰を修正

**safefileio 公開 API 移行:**
- `safefileio` のすべての公開関数は、プレーン `string` ではなく `common.ResolvedPath` を受け入れるようになりました
- `fileanalysis`, `filevalidator`, および設定ローダーの呼び出し側が更新されました
- `pathencoding.Encode` は冗長な `IsAbs`/`filepath.Clean` チェックが削除されました（`ResolvedPath` で保証）

**名前変更:**
- `NewResolvedPathForNew` → `NewResolvedPathParentOnly`（意図がより明確）

#### Go ツールチェーン アップグレード

- Go を **1.26.2** にアップグレード（1.23.10 から）
- `golangci-lint` を **v2.11.4** にアップグレード
- CI は `go.mod` で宣言された Go バージョンに `go-version-file` 経由で固定

#### WorkDir および OutputFile 型変更

**変更内容:**
`CommandTemplate` および `CommandSpec` の `workdir` フィールドは、適切な継承セマンティクスをサポートするために、`string` から `*string`（ポインタ型）に変更されました。

**動作:**
- `nil`: フィールド未指定、テンプレートから継承可能
- 空文字列ポインタ（`""`）：現在のディレクトリを使用するように明示的に設定
- 非 nil で値を持つ：指定された絶対パスを使用

**影響:**
- 既存設定は修正なしで引き続き機能します
- TOML パーサーは自動的に文字列値をポインタに変換します
- `WorkDir` を参照するコードは nil ケースを処理する必要があります：`if cmdSpec.WorkDir == nil || *cmdSpec.WorkDir == "" { ... }`

**利点:**
- 「未指定」（nil）と「明示的に空」（""）の区別を可能に
- `Timeout`, `OutputSizeLimit`, `RiskLevel` などの他のポインタ型フィールドと一貫
- 適切なテンプレート継承サポートに必須

#### 破壊的変更：Vars 設定形式

**変更内容:**
`vars` フィールド設定形式は、文字列の配列から TOML テーブル形式に変更されました。

**旧形式（廃止予定）:**
```toml
[global]
vars = [
    "app_dir=/opt/myapp",
    "config_file=%{app_dir}/config.yml"
]

[[groups]]
name = "example"
vars = ["backup_dir=/var/backups"]

[[groups.commands]]
name = "backup"
vars = ["timestamp=20250114"]
```

**新形式（必須）:**
```toml
[global.vars]
app_dir = "/opt/myapp"
config_file = "%{app_dir}/config.yml"

[[groups]]
name = "example"

[groups.vars]
backup_dir = "/var/backups"

[[groups.commands]]
name = "backup"

[groups.commands.vars]
timestamp = "20250114"
```

**移行ガイド:**
1. グローバルレベル：`[global] vars = [...]` を `[global.vars]` テーブルに変更
2. グループレベル：`[[groups]] vars = [...]` を `[groups.vars]` テーブルに変更
3. コマンドレベル：`[[groups.commands]] vars = [...]` を `[groups.commands.vars]` テーブルに変更
4. 各 `"key=value"` 配列エントリを `key = "value"` テーブルエントリに変換

**この変更の理由:**
- 改善された TOML コンプライアンスと可読性
- 複雑な値型（文字列、配列、ネストされたテーブル）への改善されたサポート
- 標準 TOML 慣行との一貫性
- より簡単な検証とエラー報告

#### グループレベルコマンド許可リスト（`cmd_allowed`）

ハードコードされたグローバルパターンでカバーされていないグループ固有の許可コマンドを定義する機能を追加しました。この機能はより細粒度のセキュリティ制御を可能にします。

**ハードコードされたグローバルパターン**（TOML から設定不可）:
```
^/bin/.*
^/usr/bin/.*
^/usr/sbin/.*
^/usr/local/bin/.*
```

**機能:**
- グループごとのコマンド許可リスト用の `[[groups]]` セクションの `cmd_allowed` フィールド
- 柔軟なパス設定のための変数展開サポート（`%{variable}`）
- OR 条件評価：ハードコードされたグローバルパターン OR グループレベルリストのいずれかと一致する場合、コマンドは合格
- シンボリックリンク解決とパス正規化によるセキュリティ
- パストラバーサル攻撃を防止する絶対パス要件
- その他すべてのセキュリティチェック（権限、リスク評価）は有効なまま
- グローバルパターンはセキュリティのためハードコードされています（TOML から設定不可）

**設定例:**
```toml
[global]
env_import = ["home=HOME"]

[[groups]]
name = "custom_build"
cmd_allowed = [
    "%{home}/bin/custom_tool",
    "/opt/myapp/bin/processor"
]

[[groups.commands]]
name = "run_custom"
cmd = "%{home}/bin/custom_tool"
args = ["--verbose"]
```

**サンプルファイル:** 完全な例については `sample/group_cmd_allowed.toml` を参照してください。

#### Dry-Run モードでのファイル検証

Dry-run モードはファイル検証チェックを実行するようになりました。実行を中断することなく、設定ファイル、グローバルファイル、グループファイル、および実行可能ファイルの整合性ステータスの可視性を提供します。

**機能:**
- Dry-run モードでの警告のみの動作によるファイル検証の有効化
- Dry-run 出力（TEXT および JSON 形式）に含まれる検証結果
- 検証失敗によってドライランが終了しません（終了コードは常に 0）
- 詳細な検証サマリーを表示：
  - 検証されたファイルの総数
  - ハッシュディレクトリステータス
  - 重大度レベル（INFO/WARN/ERROR）を持つ検証失敗
  - 各ファイルのコンテキスト情報（config、global、group、env）
  - 失敗のセキュリティリスク評価

**検証失敗理由:**
- ハッシュディレクトリが見つかりません（INFO レベル）
- ハッシュファイルが見つかりません（WARN レベル）
- ハッシュ不一致（ERROR レベル - 潜在的な改ざん）
- ファイル読み取りエラー（ERROR レベル）
- 権限拒否（ERROR レベル）

**出力例（TEXT）:**
```
=== ファイル検証 ===
ハッシュディレクトリ: /usr/local/etc/go-safe-cmd-runner/hashes
  存在: true
  検証: true
ファイル数: 2
  検証済み: 0
  スキップ: 0
  失敗: 2
期間: 3.469ms

失敗:
1. [WARN] /tmp/test-config.toml
   理由: ハッシュファイルが見つかりません
   コンテキスト: config
   メッセージ: hash file not found
2. [WARN] /bin/echo
   理由: ハッシュファイルが見つかりません
   コンテキスト: group:test_group
   メッセージ: hash file not found
```

**出力例（JSON）:**
```json
{
  "file_verification": {
    "total_files": 2,
    "verified_files": 0,
    "skipped_files": 0,
    "failed_files": 2,
    "duration": 3469483,
    "hash_dir_status": {
      "path": "/usr/local/etc/go-safe-cmd-runner/hashes",
      "exists": true,
      "validated": true
    },
    "failures": [
      {
        "path": "/tmp/test-config.toml",
        "context": "config",
        "reason": "hash_file_not_found",
        "message": "hash file not found",
        "level": "warn"
      },
      {
        "path": "/bin/echo",
        "context": "group:test_group",
        "reason": "hash_file_not_found",
        "message": "hash file not found",
        "level": "warn"
      }
    ]
  }
}
```

**副作用の保証:**
- Dry-run モードは副作用がないままです
- 読み取り専用操作のみ実行（ファイルとハッシュ読み取り）
- ファイルは書き込みまたは変更されません
- ネットワーク通信なし
- 検証失敗に関わらず終了コードは常に 0

**ドキュメント:**
- 実装計画で文書化されている検証動作

#### Dry-Run モード向け JSON 形式出力

Dry-run モードは、マシン処理と実行計画の自動分析を可能にする包括的なデバッグ情報を備えた JSON 形式出力をサポートするようになりました。

**機能:**
- JSON 出力用の新しい `--dry-run-format=json` フラグ（デフォルト：text）
- 詳細レベルに基づいて JSON 出力に含まれるデバッグ情報：
  - `summary`: デバッグ情報なし
  - `detailed`: 基本的なデバッグ情報（環境継承、最終環境）
  - `full`: 完全なデバッグ情報と差分分析
- 環境変数継承分析を表示：
  - グローバルおよびグループレベルの設定
  - 継承モード（inherit/explicit/reject）
  - 継承変数リスト
  - 削除されたホワイトリスト変数
  - 利用不可な env_import 変数
- 最終環境変数とソース追跡
- JSON モードではログは stdout をパイプ処理用にクリーンに保つために stderr に出力

**JSON スキーマ:**
- `debug_info` フィールド付きの `ResourceAnalysis` オブジェクト
- 環境変数継承の詳細のための `InheritanceAnalysis`
- 変数ごとのソース追跡を備えた `FinalEnvironment`
- `InheritanceMode` JSON シリアライゼーション（inherit/explicit/reject）

**使用例:**
```bash
# フルデバッグ情報を備えた JSON 出力
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full

# 分析のため jq にパイプ
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | jq '.'

# デバッグ情報を抽出
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info != null) | .debug_info'
```

**ドキュメント:**
- 完全な JSON スキーマリファレンスについては `docs/user/dry_run_json_schema.md` を参照してください
- 使用例については `docs/user/runner_command.md` を参照してください

#### Dry-Run モードでの最終環境変数表示

`--dry-run-detail=full` を使用するとき、各コマンドの最終環境変数は元の情報とともに表示されるようになりました。

**機能:**
- Dry-run モードでコマンド実行前に最終環境変数を表示
- 各変数の元を表示（System、Global、Group、Command）
- 可読性のため長い値は 60 文字に切り詰められます
- 機密情報（パスワード、トークン、シークレット）はデフォルトで `[REDACTED]` としてマスクされます

**新しいフラグ:**
- `--show-sensitive`: 機密環境変数値をマスクせずに表示する（注意して使用）
  - デフォルト：機密値はマスク
  - セキュリティ警告：本番環境または CI/CD 環境では使用しないでください

**出力例:**
```
===== 最終プロセス環境 =====

環境変数（5）:
  PATH=/usr/local/bin:/usr/bin:/bin
    (Global より)
  HOME=/home/testuser
    (System より (許可リストでフィルタ))
  APP_DIR=/opt/myapp
    (Group[build] より)
  DB_PASSWORD=[REDACTED]
    (Global より)
  LOG_FILE=/opt/myapp/logs/app.log
    (Command[run_tests] より)
```

**パフォーマンス:**
- Dry-run モードでの最終環境表示のオーバーヘッドはほぼ無視できます（テストで 10% 未満）。最小限のパフォーマンス影響を保証します。

#### タイムアウト動作変更

**破壊的**: `timeout = 0` は無制限実行を意味するようになりました（以前はデフォルト 60 秒）

- **以前**: `timeout = 0` は無効として扱われました（受け入れられない）
- **現在**: `timeout = 0` は明示的に無制限実行時間を意味します（タイムアウトなし）

**必要な移行**: 既存設定ファイルのすべての `timeout = 0` 設定を確認してください。

#### TOML フィールド名変更

すべての TOML 設定フィールド名が、明確さと一貫性を向上させるために更新されました。

**必要な移行**: 既存の設定ファイルは手動で更新する必要があります。

##### フィールド名マッピング

| レベル | 旧フィールド名 | 新フィールド名 | デフォルト値変更 |
|--------|--------------|--------------|-----|
| グローバル | `skip_standard_paths` | `verify_standard_paths` | `false`（検証）→ `true`（検証）|
| グローバル | `env` | `env_vars` | - |
| グローバル | `env_allowlist` | `env_allowed` | - |
| グローバル | `from_env` | `env_import` | - |
| グローバル | `max_output_size` | `output_size_limit` | - |
| グループ | `env` | `env_vars` | - |
| グループ | `env_allowlist` | `env_allowed` | - |
| グループ | `from_env` | `env_import` | - |
| コマンド | `env` | `env_vars` | - |
| コマンド | `from_env` | `env_import` | - |
| コマンド | `max_risk_level` | `risk_level` | - |
| コマンド | `output` | `output_file` | - |

##### 主な変更

1. **肯定的な命名**: `skip_standard_paths` → `verify_standard_paths`
   - 旧：`skip_standard_paths = false`（デフォルト：標準パス検証）
   - 新：`verify_standard_paths = true`（デフォルト：標準パス検証）
   - **デフォルト動作は変わりません（検証は継続）。フィールド名がより明確になります**

2. **環境変数プレフィックスの統一**: すべての環境関連フィールドが `env_` プレフィックスを使用
   - `env` → `env_vars`
   - `env_allowlist` → `env_allowed`
   - `from_env` → `env_import`

3. **自然な語順**: `max_output_size` → `output_size_limit`

4. **明確さ**: `output` → `output_file`, `max_risk_level` → `risk_level`

#### 作業ディレクトリ指定の再設計

作業ディレクトリ設定を簡素化し、自動一時ディレクトリサポートを追加しました。
- **削除**: `Global.WorkDir` フィールド：グローバルレベルの作業ディレクトリ設定はサポートされなくなりました
- **削除**: `Group.TempDir` フィールド：`workdir` を指定しない場合の自動一時ディレクトリ生成に置き換えられました
- **名前変更**: `Command.Dir` → `Command.WorkDir`：コマンドレベルディレクトリ指定が `workdir` フィールドを使用するようになりました
- **デフォルト動作変更**: `workdir` を指定しないグループは、現在のディレクトリを使用する代わりに自動的に一時ディレクトリを生成するようになりました
- **自動クリーンアップ**: 一時ディレクトリはグループ実行後に自動的に削除されます（`--keep-temp-dirs` を指定しない限り）

- `timeout = 0` での無制限コマンド実行のサポート
- 強化されたタイムアウト階層解決（command → global → system default）
- 無制限実行コマンドのセキュリティ監視
- 長時間実行プロセスの検出とロギング
- `sample/timeout_examples.toml` の包括的なタイムアウト例
- タイムアウト変更の移行ガイド
- **`__runner_workdir` 予約変数**: コマンド実行のランタイム作業ディレクトリを参照する新しい自動変数
- **`--keep-temp-dirs` フラグ**: 実行後の一時ディレクトリを保持してデバッグするための新しいコマンドラインフラグ
- **自動一時ディレクトリ生成**: 指定された `workdir` のないグループは自動的に一時ディレクトリを生成するようになりました
- **Dry-run モード一時ディレクトリサポート**: Dry-run モードは一時ディレクトリに仮想パスを使用するようになりました
- **verify_files 変数展開**: グローバルおよびグループ設定の `verify_files` フィールドの環境変数展開サポート
  - グローバルレベルの `verify_files` は環境変数を使用できるようになりました（例：`${HOME}/config.toml`）
  - グループレベルの `verify_files` は許可リスト継承で環境変数を使用できるようになりました
  - 単一パス内での複数変数のサポート（例：`${BASE}/${ENV}/config.toml`）
  - 詳細なエラーメッセージの包括的なエラー処理
  - `env_allowlist` 検証によるセキュリティ制御
  - 環境変数の循環参照検出
  - サンプル設定：`sample/verify_files_expansion.toml`
  - ドキュメント：変数展開ユーザーガイドにセクション 7.11 を追加

- タイムアウト設定はより柔軟な制御のためにヌル可能整数を使用するようになりました
- 明確な継承階層を備えた改善されたタイムアウト解決ロジック
- タイムアウト設定エラーの強化されたエラーメッセージ
- 破壊的変更通知と例を使用した更新されたドキュメント
- 設定読み込みは `verify_files` フィールドの環境変数を自動的に展開するようになりました
- 検証マネージャーは展開されたファイルパスをすべての検証操作に使用するようになりました

### セキュリティ

- 無制限タイムアウト実行のためのセキュリティロギングを追加
- 長時間実行プロセスの監視を実装
- 無制限実行コマンドのリソース使用追跡を強化

### 技術詳細

- 新しいフィールド：`GlobalConfig.ExpandedVerifyFiles` および `CommandGroup.ExpandedVerifyFiles`
- 新しい関数：設定パッケージの `ExpandGlobalVerifyFiles()` および `ExpandGroupVerifyFiles()`
- 新しいエラー型：より良いエラー処理のための `VerifyFilesExpansionError` とセンチネルエラー
- 環境パッケージの `ResolveAllowlistConfiguration()` メソッドをエクスポート
- タスク 0026 の既存 `Filter` および `VariableExpander` インフラストラクチャとの統合

### 移行ガイド

#### タイムアウト設定

詳細な移行手順については、タイムアウト設定ドキュメントを参照してください。

#### TOML フィールド名変更

詳細な手順については [移行ガイド](docs/migration/toml_field_renaming.ja.md) を参照してください。

#### 作業ディレクトリ設定

既存の TOML 設定ファイルは次のように更新する必要があります：

1. **`[global]` セクションの `workdir` を削除**:
   ```toml
   # 以前（エラーになります）
   [global]
   workdir = "/tmp"

   # 後（削除）
   [global]
   # workdir フィールドが削除されました
   ```

2. **`[[groups]]` セクションの `temp_dir` を削除**:
   ```toml
   # 以前（エラーになります）
   [[groups]]
   name = "backup"
   temp_dir = true

   # 後（自動一時ディレクトリ）
   [[groups]]
   name = "backup"
   # temp_dir フィールドが削除されました - 自動一時ディレクトリが作成されます
   ```

3. **`[[groups.commands]]` の `dir` を `workdir` に変更**:
   ```toml
   # 以前（エラーになります）
   [[groups.commands]]
   name = "backup"
   cmd = "pg_dump"
   dir = "/var/backups"

   # 後
   [[groups.commands]]
   name = "backup"
   cmd = "pg_dump"
   workdir = "/var/backups"
   ```

4. **動的パス参照に `%{__runner_workdir}` 変数を使用**:
   ```toml
   [[groups]]
   name = "backup"
   # workdir 未指定 - 自動一時ディレクトリ

   [[groups.commands]]
   name = "dump"
   cmd = "pg_dump"
   args = ["mydb", "-f", "%{__runner_workdir}/dump.sql"]
   ```

## [前のリリース]

（前のリリースノートは利用可能になったら追加されます）
