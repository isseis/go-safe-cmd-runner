# 0089: セキュリティ検査所見の修正 - 実装計画書

## 1. 実装概要

### 1.1 目的

[01_requirements.md](./01_requirements.md) で定義されたセキュリティ検査所見 (M1, M2短期, M3, M4, L1, L4, I1)
を段階的に修正し、コードベースのセキュリティ態勢を改善する。

### 1.2 実装原則

1. **最小変更**: 各所見の修正スコープを要件定義の範囲に限定し、過剰な抽象化・リファクタリングを行わない
2. **テスト駆動**: 各修正に受け入れ条件対応のユニットテストを先行または同時に実装する
3. **回帰防止**: 既存の正常パスが変わらないことを回帰テストで担保する
4. **段階的リリース**: リスク・複雑度の低い所見から着手し、最終フェーズで M2 (新機能追加) を実施する

---

## 2. 実装ステップ

フェーズ順序:
**I1 → M1 → M3 → M4 → L4 → M2 (短期)**

> L1 (template include パス制約) は対応なし (Close)。ハッシュ検証が実質的な防御として機能しており、パス制約の追加にセキュリティ上の実質的な価値がないため。詳細は [01_requirements.md L1 セクション](./01_requirements.md) および [所見ドキュメント](../0088_security_audit_findings/L1_include_path_not_constrained.md) を参照。

---

### Phase 1: I1 — `verifyFileWithFallback` 系関数の命名修正

純粋なリネームで機能変更なし。最もリスクが低いため最初に実施することでテスト基盤を確認する。

**修正ファイル**:
- `internal/verification/manager.go`
- `internal/verification/manager_test.go`

**作業内容**:

- [ ] `verifyFileWithFallback` → `verifyFile` にリネーム (AC-I1-3)
  - 関数定義、すべての呼び出し箇所 (manager.go 内: 97行/179行)
  - 関連コメント (348行のコメント) を整合する名前に更新
- [ ] `readAndVerifyFileWithFallback` → `readAndVerifyFileWithReadFallback` にリネーム (AC-I1-2)
  - 関数定義、呼び出し箇所 (manager.go 63行)
  - コメントには以下の**2種類の `os.ReadFile` フォールバック**を明記する:
    1. `m.fileValidator == nil` (ファイル検証が無効化されている場合): 検証をスキップし、`os.ReadFile` で直接読み込む ([manager.go:411-415](../../../internal/verification/manager.go#L411-L415))
    2. dry-run モードかつ検証失敗時: 失敗を `resultCollector` に記録した上で、`os.ReadFile` でファイル読み込みを再試行する ([manager.go:421-435](../../../internal/verification/manager.go#L421-L435))
  - `WithReadFallback` という名前は上記の「ファイル読み込み処理にフォールバックする」動作全般を指すことを明記する
- [ ] `internal/verification/manager_test.go` のテスト関数名を更新 (AC-I1-4, AC-I1-5)
  - `TestVerifyFileWithFallback` → `TestVerifyFile`
  - `TestReadAndVerifyFileWithFallback` → `TestReadAndVerifyFileWithReadFallback`

**成功条件**:
- `make test` が全パス (AC-I1-5)
- `grep -r "verifyFileWithFallback\|readAndVerifyFileWithFallback" internal/` が 0 件

**推定工数**: 1時間

---

### Phase 2: M1 — 権限昇格失敗時の egid 復元バグ修正

`changeUserGroupInternal` の `Seteuid` 失敗時に `Getegid()` を返す no-op バグを修正する。

**修正ファイル**:
- `internal/runner/privilege/unix.go`
- `internal/runner/privilege/unix_privilege_test.go`

**作業内容**:

- [ ] `changeUserGroupInternal` のシグネチャに `originalEGID int` 引数を追加 (AC-M1-1)
  ```go
  // 変更前
  func (m *UnixPrivilegeManager) changeUserGroupInternal(
      userName, groupName string, dryRun bool) error

  // 変更後
  func (m *UnixPrivilegeManager) changeUserGroupInternal(
      userName, groupName string, dryRun bool, originalEGID int) error
  ```
- [ ] 関数内の `Seteuid` 失敗時リカバリを修正 (AC-M1-1, AC-M1-2)
  ```go
  // 変更前
  if restoreErr := syscall.Setegid(syscall.Getegid()); restoreErr != nil {
      m.logger.Error("Failed to restore GID after UID change failure", ...)
  }

  // 変更後
  if restoreErr := syscall.Setegid(originalEGID); restoreErr != nil {
      m.emergencyShutdown(restoreErr, "egid_rollback_failure_after_seteuid_failure")
  }
  ```
- [ ] 呼び出し側 `performElevation` で `execCtx.originalEGID` を渡すよう更新 (AC-M1-3)
  ```go
  // 変更前
  if err := m.changeUserGroupInternal(
      execCtx.elevationCtx.RunAsUser, execCtx.elevationCtx.RunAsGroup, isDryRun); err != nil {

  // 変更後
  if err := m.changeUserGroupInternal(
      execCtx.elevationCtx.RunAsUser, execCtx.elevationCtx.RunAsGroup, isDryRun,
      execCtx.originalEGID); err != nil {
  ```
- [ ] `UnixPrivilegeManager` に `syscallSeteuid func(int) error` と `syscallSetegid func(int) error` の injectable フィールドを追加し、`changeUserGroupInternal` が直接 `syscall.Seteuid` / `syscall.Setegid` を呼ぶ代わりにこれらのフィールドを経由するよう変更する
  - `osExit` の既存パターンに倣う
  - `newPlatformManager` のデフォルト値を `syscall.Seteuid` / `syscall.Setegid` に設定する
- [ ] `Seteuid` 失敗 → egid ロールバック成功 パスのユニットテスト追加 (AC-M1-4)
  - `syscallSeteuid` をモックして失敗させ、`syscallSetegid` が `originalEGID` で呼ばれることを確認する
- [ ] `Seteuid` 失敗 → egid ロールバック失敗 (`emergencyShutdown` 呼び出し) パスのユニットテスト追加 (AC-M1-5)
  - `syscallSeteuid` と `syscallSetegid` の両方をモックして失敗させ、`osExit` フィールド経由で `emergencyShutdown` が呼ばれたことを検証する

**成功条件**:
- `go test -tags test -v ./internal/runner/privilege/...` が全パス
- `Seteuid` 失敗時にロールバックが `originalEGID` に対して行われる

**推定工数**: 4時間 (syscall モック基盤追加分として +1時間)

---

### Phase 3: M3 — `LD_*` 環境変数フィルタの強化

**修正ファイル**:
- `internal/runner/executor/environment.go`
- `internal/runner/executor/environment_test.go` (既存テストファイルへ追記)
- `internal/runner/config/expansion.go`
- `internal/runner/config/expansion_test.go` (既存テストファイルへ追記)

**作業内容**:

- [ ] `internal/runner/executor/environment.go` の個別 delete を置き換え (AC-M3-1, AC-M3-2, AC-M3-3)

  変更前 (87-89行):
  ```go
  delete(result, "LD_LIBRARY_PATH")
  delete(result, "LD_PRELOAD")
  delete(result, "LD_AUDIT")
  ```

  変更後:
  ```go
  // Remove all LD_* dynamic linker control variables and other
  // dangerous loader-related variables from the child process environment.
  for key := range result {
      if strings.HasPrefix(key, "LD_") {
          delete(result, key)
      }
  }
  for _, key := range []string{
      "GCONV_PATH", "LOCPATH", "HOSTALIASES", "NLSPATH", "RES_OPTIONS",
  } {
      delete(result, key)
  }
  ```

- [ ] `internal/runner/config/expansion.go` の `forbiddenEnvVars` を更新 (AC-M3-6)

  変更前 (map 定義):
  ```go
  var forbiddenEnvVars = map[string]struct{}{
      "LD_LIBRARY_PATH": {},
      "LD_PRELOAD":      {},
      "LD_AUDIT":        {},
  }
  ```

  変更後:
  ```go
  // forbiddenEnvVarPrefixes lists prefixes of forbidden environment variables.
  var forbiddenEnvVarPrefixes = []string{"LD_"}

  // forbiddenEnvVarExact lists exact names of forbidden environment variables
  // that do not carry a forbidden prefix.
  var forbiddenEnvVarExact = map[string]struct{}{
      "GCONV_PATH":  {},
      "LOCPATH":     {},
      "HOSTALIASES": {},
      "NLSPATH":     {},
      "RES_OPTIONS": {},
  }
  ```

  あわせて `isForbiddenEnvVar` をプレフィックス + 完全一致でチェックするように更新する。

- [ ] `LD_FOOBAR` 等の任意の `LD_*` 変数、および5つの非 `LD_` 系変数が削除されることを確認するユニットテスト追加 (AC-M3-4, AC-M3-5)
- [ ] 回帰テスト: 既存の正当な環境変数 (`PATH`, `HOME`, `USER`, `LANG` 等) が削除されないことを確認するテスト追加

**成功条件**:
- `go test -tags test -v ./internal/runner/executor/... ./internal/runner/config/...` 全パス
- `LD_FOOBAR=x` を渡した場合に削除される

**推定工数**: 3時間

---

### Phase 4: M4 — 環境変数値の危険パターン検査の再設計

シェルメタ文字ベースの検査を廃止し、`\0`, `\n`, `\r` のみを禁止する。

**修正ファイル**:
- `internal/runner/security/validator.go`
- `internal/runner/security/environment_validation.go`
- `internal/runner/security/validator_test.go`
- `internal/runner/security/environment_validation_test.go` (既存または新規)

**作業内容**:

- [ ] `validator.go` の `dangerousPatterns` 定義とコンパイル処理を削除 (AC-M4-1)
- [ ] `Validator` 構造体から `dangerousEnvRegexps` フィールドを削除 (AC-M4-1)
- [ ] `environment_validation.go` の `ValidateEnvironmentValue` を再実装 (AC-M4-2, AC-M4-3)
  ```go
  func (v *Validator) ValidateEnvironmentValue(key, value string) error {
      if strings.ContainsRune(value, '\x00') {
          return fmt.Errorf("%w: environment variable %s contains null byte",
              ErrUnsafeEnvironmentVar, key)
      }
      if strings.ContainsAny(value, "\n\r") {
          return fmt.Errorf("%w: environment variable %s contains newline character",
              ErrUnsafeEnvironmentVar, key)
      }
      return nil
  }
  ```
- [ ] 下記を確認するユニットテスト追加 (AC-M4-5):
  - `\0` を含む値 → エラー
  - `\n` を含む値 → エラー
  - `\r` を含む値 → エラー
  - `;`, `|`, `$(`, `>`, `<` を含む値 → 通過
- [ ] 回帰テスト: JSON 値 (`{"key": "value"}`) を含む変数が通過することを確認 (AC-M4-4)
- [ ] `validator_test.go` の `dangerousEnvRegexps` を参照している箇所を更新

**成功条件**:
- `go test -tags test -v ./internal/runner/security/...` 全パス
- `; | $( > <` を含む値が `ValidateEnvironmentValue` を通過する

**推定工数**: 3時間

---

### Phase 5: L4 — CGO グループメンバ数の境界チェック

**修正ファイル**:
- `internal/groupmembership/membership_cgo.go`
- `internal/groupmembership/membership_cgo_test.go` (既存または新規)

**作業内容**:

- [ ] `count` が負値の場合はエラーを返す (AC-L4-1)
  ```go
  if count < 0 {
      return nil, fmt.Errorf("invalid group member count from C: %d", count)
  }
  ```
- [ ] `count` が 65536 超の場合はエラーを返す (AC-L4-2)
  ```go
  const maxGroupMembers = 65536
  if int(count) > maxGroupMembers {
      return nil, fmt.Errorf("group member count %d exceeds maximum %d", count, maxGroupMembers)
  }
  ```
- [ ] `(*[1 << 30]*C.char)(unsafe.Pointer(members))[:count:count]` を `unsafe.Slice` に置き換え (AC-L4-3)
  ```go
  cArray := unsafe.Slice(members, int(count))
  ```
  - `count` は `C.int` 型のため、境界チェック後に `int` へ明示変換して `unsafe.Slice` に渡す
- [ ] 境界チェックのユニットテスト追加 (AC-L4-4):
  - 境界チェックロジックを Go 内部ヘルパー関数 `validateGroupMemberCount(count C.int) error` として抽出し、テストはこのヘルパーを直接呼び出す形で実装する
    (CGO 関数 `C.get_group_members` はモックできないため、境界チェック部分のみをヘルパーに分離してテスト可能にする)
  - 負値 `count` でエラーが返ること
  - `count > 65536` でエラーが返ること

**注意**: CGO 依存のため `go test -tags test` 環境で実行可能か確認する。
メンバ 0 件のケースは既存の `nil` チェックでカバーされているため追加不要。

**成功条件**:
- `go test -tags test -v ./internal/groupmembership/...` 全パス
- 負値・上限超過でエラーが返る

**推定工数**: 2時間

---

### Phase 6: M2 短期 — TOCTOU ウィンドウの自動パーミッション検査

最も機能追加規模が大きいフェーズ。既存の `ValidateDirectoryPermissions` を
再利用するがパフォーマンス・エッジケースに注意する。

#### Step 6.1: 検査対象パスの列挙ロジック実装

**修正ファイル**:
- `internal/runner/security/toctou_check.go` (新規)
- `internal/runner/security/toctou_check_test.go` (新規)

**作業内容**:

- [ ] 以下の入力からチェック対象ディレクトリを列挙する関数を実装 (AC-M2S-2)
  - `verify_files` で参照される各ファイルの親ディレクトリ
  - 実行コマンド (`cmd` フィールド) の親ディレクトリ
  - `--hash-dir` が指すディレクトリ自身
  - ハッシュディレクトリの各親ディレクトリ (ルートまで)

  ```go
  // CollectTOCTOUCheckDirs collects directories to check for TOCTOU prevention.
  func CollectTOCTOUCheckDirs(
      verifyFilePaths []string,
      commandPaths []string,
      hashDir string,
  ) []string
  ```

- [ ] 重複パスを除去して返す (同一ディレクトリが複数の入力から現れる場合)
- [ ] 列挙ロジックのユニットテスト追加 (AC-M2S-6)

**パフォーマンス考慮事項**:
- ハッシュディレクトリからルートまでの親ディレクトリ列挙は O(depth) だが、
  通常の Linux/macOS では深さが 20 程度以下のため許容範囲。
  `filepath.Dir` のループで実装し、`parent := filepath.Dir(cur); if parent == cur { break }` をループ終了条件とする。Unix では `filepath.VolumeName` は常に空文字列を返すためルート判定には使用しないこと (Windows の drive letter / UNC パス対応が必要な場合のみ検討)。
- 重複除去に `map[string]struct{}` を使って O(n) で処理する。

**シンボリックリンク考慮事項**:
- 今フェーズでは列挙対象のパスを `filepath.EvalSymlinks` で解決せず渡す。
  `ValidateDirectoryPermissions` 内の `validateCompletePath` が `Lstat` を使って
  シンボリックリンクを検出するため、シンボリックリンクが含まれれば検査で失敗する。

**推定工数**: 3時間

#### Step 6.2: 検査実行・ログ出力ロジック実装

**修正ファイル**:
- `internal/runner/security/toctou_check.go` (Step 6.1 と同ファイル)
- `internal/runner/security/toctou_check_test.go`

**作業内容**:

- [ ] 列挙された各ディレクトリに対して `validator.ValidateDirectoryPermissions` を呼び出す関数を実装 (AC-M2S-3)

  ```go
  // RunTOCTOUPermissionCheck checks all collected directories and returns
  // a list of violations. Each violation contains the path and the error.
  type TOCTOUViolation struct {
      Path string
      Err  error
  }

  func RunTOCTOUPermissionCheck(
      v *Validator,
      dirs []string,
      logger *slog.Logger,
  ) []TOCTOUViolation
  ```

- [ ] 問題が検出された場合は `logger.Warn` で "path" と "violation" を含む警告ログを出力 (AC-M2S-4)
- [ ] 検査実行のユニットテスト追加 (AC-M2S-6)
  - 問題なし → 空のスライスが返ること
  - 1 件違反 → Violation が 1 件含まれること
  - 警告ログが出力されること (ログキャプチャで確認)

**推定工数**: 2時間

#### Step 6.3: runner / record / verify への組み込み

**修正ファイル**:
- `cmd/runner/main.go` または `internal/runner/runner.go`
- `cmd/record/main.go`
- `cmd/verify/main.go`
- 統合テスト

**作業内容**:

- [ ] `runner`: 設定ロード後、コマンド実行前に `CollectTOCTOUCheckDirs` + `RunTOCTOUPermissionCheck` を呼び出し、
  1 件以上の違反があればエラー終了 (AC-M2S-5)
- [ ] `record` / `verify`: 同様に検査を呼び出すが、違反があっても警告ログのみで継続 (AC-M2S-5)
- [ ] `runner` が検査失敗後に起動中断することを確認するテスト追加 (AC-M2S-7)
- [ ] `record` / `verify` が検査失敗後も継続することを確認するテスト追加 (AC-M2S-7)

**パフォーマンス考慮事項**:
- 検査は起動時 1 回のみ実行 (`sync.Once` 等は不要)。
- `ValidateDirectoryPermissions` は `Lstat` ベースで実装されており、
  ディレクトリ深さ分のシステムコールが発生する。通常は数十件以内のため許容範囲。
- パス数が多い設定ファイルの場合、検査時間が顕著になる可能性があるため、
  重複除去 (Step 6.1) を確実に行う。

**推定工数**: 4時間

---

### Phase 7: M2 短期 — 運用ドキュメント整備

**修正ファイル**:
- `docs/security/README.md`
- `CHANGELOG.md`

**作業内容**:

- [ ] 既存の `docs/security/README.md` を確認し、以下の内容を追記 (AC-M2S-1):
  - `verify_files` および `commands` で指定するバイナリの配置ディレクトリ要件
  - 対象ディレクトリおよびルートまでの全親ディレクトリのパーミッション要件
  - `--hash-dir` のパーミッション要件
  - 要件未充足時の TOCTOU リスクに関する説明
- [ ] L1 (include パス制約) は対応なし (Close) のため CHANGELOG.md への記載は不要

**推定工数**: 2時間

---

## 3. 実装順序とマイルストーン

| マイルストーン | フェーズ | 完了条件 |
|---|---|---|
| MS-1: コード品質改善完了 | Phase 1 (I1) | リネーム完了、全テストパス |
| MS-2: 権限管理バグ修正完了 | Phase 2 (M1) | ロールバックバグ修正、テスト追加 |
| MS-3: 環境変数フィルタ強化完了 | Phase 3-4 (M3, M4) | フィルタ更新、回帰テストパス |
| MS-4: 境界検査修正完了 | Phase 5 (L4) | CGO 境界検査、テストパス |
| MS-5: TOCTOU 検査実装完了 | Phase 6 (M2) | 新検査機能実装、統合テストパス |
| MS-6: ドキュメント更新完了 | Phase 7 (M2 docs) | セキュリティ文書更新 |

**全体推定工数**: 約 24 時間 (L1 Phase 削除により -1h、フェーズ番号繰り上げ後の合計)

---

## 4. テスト戦略

### 4.1 ユニットテスト

各フェーズで受け入れ条件対応のテストを追加する。テストタグは既存に合わせて `-tags test` を使用する。

| フェーズ | テストファイル | テストカテゴリ |
|---|---|---|
| I1 | `internal/verification/manager_test.go` | リネーム後の既存テスト |
| M1 | `internal/runner/privilege/unix_privilege_test.go` | エラーパス (Seteuid 失敗) のモック |
| M3 | `internal/runner/executor/environment_test.go` | LD_* フィルタ、非 LD_* フィルタ |
| M3 | `internal/runner/config/expansion_test.go` | `isForbiddenEnvVar` のプレフィックス検査 |
| M4 | `internal/runner/security/validator_test.go` | シェルメタ文字通過、制御文字拒否 |
| L4 | `internal/groupmembership/membership_cgo_test.go` | 負値・上限超過 |
| M2 | `internal/runner/security/toctou_check_test.go` | 列挙ロジック、違反検出 |

### 4.2 回帰テスト

M3・M4 の変更は既存の正当なユースケースに影響しうる。以下の回帰テストケースを明示的に追加する。

**M3 (env filter) 回帰**:
- `PATH`, `HOME`, `USER`, `LANG`, `TZ`, `TERM` 等の標準変数が削除されないこと
- `LD_` という名前の変数 (prefix のみで別単語となる場合) が削除されること

**M4 (env validation) 回帰**:
- JSON 値 (`{"a": 1, "b": [1,2]}`) を含む変数が通過すること
- URL 値 (`https://example.com/path?q=1&r=2`) が通過すること
- シェルスクリプトのコード断片 (`echo hello; ls -la`) が通過すること
  （シェル非経由実行のため、値としては許容する）

### 4.3 統合テスト

- Phase 6 Step 6.3 で runner / record / verify の統合テストを追加する
- 既存の `cmd/runner/integration_pre_execution_error_test.go` のパターンに倣い、
  バイナリをビルドして実行結果を検証する形式で行う

---

## 5. リスク管理

| リスク | 対象フェーズ | 影響 | 対策 |
|---|---|---|---|
| M3: 正当変数の誤削除 | Phase 3 | 実行中の機能破壊 | 回帰テストで網羅、既存 E2E テストで確認 |
| M4: 既存の正当パターン拒否 | Phase 4 | 設定ファイルが動作しない | 回帰テストでシェルメタ文字を含む値を通過確認 |
| M2: 深いパスのパフォーマンス劣化 | Phase 6 | 起動遅延 | 重複除去で stat 回数を最小化、起動時 1 回のみ実行 |
| M2: `record`/`verify` の意図しないエラー終了 | Phase 6 | 録画・検証が失敗 | `record`/`verify` は警告のみで継続することをテストで確認 |
| L4: CGO テストが CI で動作しない | Phase 5 | テスト漏れ | CGO が有効な環境でのみテストを実行する build tag 等で管理 |

---

## 6. 実装チェックリスト

### Phase 1 (I1)
- [ ] `verifyFileWithFallback` → `verifyFile` にリネーム
- [ ] `readAndVerifyFileWithFallback` → `readAndVerifyFileWithReadFallback` にリネーム
- [ ] コメント・ログ文言の更新
- [ ] テスト関数名の更新
- [ ] `make test` 全パス確認

### Phase 2 (M1)
- [ ] `UnixPrivilegeManager` に `syscallSeteuid` / `syscallSetegid` injectable フィールド追加
- [ ] `changeUserGroupInternal` シグネチャ変更
- [ ] `Seteuid` 失敗時ロールバックの修正
- [ ] `performElevation` 呼び出し側の更新
- [ ] ユニットテスト追加 (ロールバック成功パス)
- [ ] ユニットテスト追加 (ロールバック失敗 → emergencyShutdown パス)
- [ ] `make test` 全パス確認

### Phase 3 (M3)
- [ ] `environment.go` の個別 delete をプレフィックスループに置き換え
- [ ] 非 `LD_` 系危険変数の削除追加
- [ ] `expansion.go` の `forbiddenEnvVars` をプレフィックス + 完全一致に更新
- [ ] ユニットテスト追加 (全対象変数削除、回帰)
- [ ] `make test` 全パス確認

### Phase 4 (M4)
- [ ] `dangerousPatterns` および `dangerousEnvRegexps` を削除
- [ ] `ValidateEnvironmentValue` を `\0`, `\n`, `\r` のみチェックに再実装
- [ ] ユニットテスト追加 (拒否パス、通過パス、回帰)
- [ ] `make test` 全パス確認

### Phase 5 (L4)
- [ ] `count` 負値チェック追加
- [ ] `count` 上限チェック追加
- [ ] `unsafe.Slice` への置き換え
- [ ] ユニットテスト追加
- [ ] `make test` 全パス確認

### Phase 6 (M2 短期)
- [ ] `CollectTOCTOUCheckDirs` 実装
- [ ] `RunTOCTOUPermissionCheck` 実装 (違反ログ込み)
- [ ] `runner` への組み込み (違反時エラー終了)
- [ ] `record`/`verify` への組み込み (違反時警告継続)
- [ ] ユニットテスト追加
- [ ] 統合テスト追加
- [ ] `make test` 全パス確認

### Phase 7 (M2 ドキュメント)
- [ ] `docs/security/README.md` に運用要件セクション追記
- [ ] 内容が AC-M2S-1 の要件を満たしていることをレビュー
- [ ] L1 は対応なし (Close) のため CHANGELOG.md への記載は不要

### 全体
- [ ] `make lint` 全パス
- [ ] `make fmt` でフォーマット確認

---

## 7. 成功基準

### 機能完全性
- 全受け入れ条件 (AC-M1-1〜5, AC-M2S-1〜7, AC-M3-1〜6, AC-M4-1〜5,
  AC-L4-1〜4, AC-I1-2〜5) を満たすテストが存在し、パスすること
  (L1 は対応なし Close のため対象外)

### 品質基準
- `make test` (全テスト) がパスすること
- `make lint` がパスすること
- 既存の統合テスト群 (`cmd/runner/integration_*.go`) がすべてパスすること

### セキュリティ確認
- M1: 権限昇格失敗時に元の egid に確実にロールバックされること
- M3: `LD_DEBUG` 等の未明示変数が子プロセスに漏れないこと
- M4: シェル非経由実行で問題にならないシェルメタ文字が通過すること
- L1: 対応なし (Close) — セキュリティはハッシュ検証によって担保されている
- L4: C 側から悪意ある `count` 値を渡されても安全に失敗すること

### ドキュメント完全性
- `docs/security/README.md` に TOCTOU 運用要件が明記されていること

---

## 8. 次のステップ

実装完了後：
- 中長期 TOCTOU 対策 ([0090_toctou_fexecve](../0090_toctou_fexecve/00_analysis.md)) へのフィードバック
- Slack webhook allowlist ([0091_slack_webhook_allowlist](../0091_slack_webhook_allowlist/01_requirements.md)) の実装着手
