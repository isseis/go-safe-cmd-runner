# 実装計画書: Command展開処理のアーキテクチャ統合

## 1. 実装概要

### 1.1 目的
展開処理の重複を解消し、責務を明確にするため、すべての変数展開（Global.Env、Group.Env、Command.Env、Cmd、Args）を`config.Loader`に統合する。

### 1.2 実装方針
- **段階的実装**: Phase 1からPhase 4まで順次実装
- **安全性優先**: 各Phaseで既存テストを実行し、リグレッションを防止
- **後方互換性**: 外部から見た動作は変更しない
- **テスト駆動**: 変更前に統合テストを追加

### 1.3 実装スコープ
本リファクタリングで変更する範囲:
- `internal/runner/config/loader.go`: Command展開処理を追加
- `internal/runner/bootstrap/config.go`: 展開処理を削除
- 関連テストファイル: テスト内容の更新

変更しない範囲:
- `internal/runner/config/expansion.go`: 展開ロジック本体
- `internal/runner/environment/`: 環境変数処理
- 公開API: 外部から見た動作

---

## 2. 実装フェーズ

### Phase 1: 準備と統合テスト追加
**目的**: 現在の動作を保証する統合テストを追加し、リグレッション防止の基盤を作る

#### 2.1.1 E2Eテストの作成
- [ ] サンプルTOMLファイル: `testdata/refactoring_e2e.toml`
  ```toml
  [global]
  env = ["BASE_DIR=/opt", "LOG_LEVEL=info"]
  env_allowlist = ["HOME", "USER"]
  verify_files = ["${BASE_DIR}/global-verify.sh"]

  [[groups]]
  name = "test_group"
  env = ["APP_DIR=${BASE_DIR}/app", "DATA_DIR=${BASE_DIR}/data"]
  verify_files = ["${APP_DIR}/group-verify.sh"]

  [[groups.commands]]
  name = "test_cmd"
  cmd = "${APP_DIR}/bin/server"
  args = ["--log", "${LOG_DIR}/app.log", "--data", "${DATA_DIR}"]
  env = ["LOG_DIR=${APP_DIR}/logs"]
  ```
- [ ] E2Eテスト: `internal/runner/config/loader_e2e_test.go`
  - [ ] `TestE2E_FullExpansionPipeline`: 全展開パイプラインの動作確認
    - Global.ExpandedEnv検証
    - Group.ExpandedEnv検証
    - Command.ExpandedEnv検証
    - Command.ExpandedCmd検証
    - Command.ExpandedArgs検証
    - Global.ExpandedVerifyFiles検証
    - Group.ExpandedVerifyFiles検証

#### 2.1.2 bootstrap統合テストの作成（必要に応じて）
- [ ] `internal/runner/bootstrap/`にテストファイルがあるか確認
- [ ] 存在する場合、現在の動作を保証するテストを追加
  - [ ] Global.Env展開の検証
  - [ ] Group.Env展開の検証
  - [ ] Command.Env/Cmd/Args展開の検証

#### 2.1.3 影響範囲の詳細調査
- [ ] `bootstrap.LoadAndPrepareConfig`を呼び出している箇所をすべて特定
  - [ ] `cmd/runner/main.go`
  - [ ] テストヘルパー関数
  - [ ] その他の呼び出し箇所
- [ ] 変更による影響を評価

#### 2.1.4 Phase 1の完了確認
- [ ] すべての既存テストがPASS
- [ ] 新規E2EテストがPASS
- [ ] `make lint`でエラーなし
- [ ] コミット: "Add E2E tests for expansion pipeline (preparation for refactoring)"

---

### Phase 2: config.Loader側にCommand展開を追加
**目的**: config.Loaderに完全な展開処理を実装する（この時点では重複展開が存在）

#### 2.2.1 processConfig()にCommand展開を追加
- [ ] `internal/runner/config/loader.go`を編集
  - [ ] `processConfig`関数内のGroup処理(Phase 3)の後に、各コマンドに対してCommand.Env/Cmd/Args展開を実行
    ```go
    // Phase 4: Command processing (Command.Env, Cmd, Args expansion)
    for i := range cfg.Groups {
        group := &cfg.Groups[i]
        for j := range group.Commands {
            cmd := &group.Commands[j]

            // Expand Command.Cmd, Args, and Env
            expandedCmd, expandedArgs, expandedEnv, err := ExpandCommand(&ExpansionContext{
                Command:            cmd,
                Expander:           expander,
                AutoEnv:            autoEnv,
                GlobalEnv:          cfg.Global.ExpandedEnv,
                GroupEnv:           group.ExpandedEnv,
                GlobalEnvAllowlist: cfg.Global.EnvAllowlist,
                GroupName:          group.Name,
                GroupEnvAllowlist:  group.EnvAllowlist,
            })
            if err != nil {
                return fmt.Errorf("failed to expand command %q in group %q: %w",
                    cmd.Name, group.Name, err)
            }

            cmd.ExpandedCmd = expandedCmd
            cmd.ExpandedArgs = expandedArgs
            cmd.ExpandedEnv = expandedEnv
        }
    }
    ```

#### 2.2.2 loader_test.goのテスト更新
- [ ] `internal/runner/config/loader_test.go`を編集
  - [ ] 既存のPhase 4テスト（`TestLoader_Phase4_CommandEnvIntegration`）を更新
    - [ ] Command.ExpandedEnvがnilでないことを確認
    - [ ] Command.ExpandedCmdが展開されていることを確認
    - [ ] Command.ExpandedArgsが展開されていることを確認
  - [ ] 期待値の追加:
    ```go
    // Command.ExpandedEnv should be populated (Phase 2 change)
    expectedCommandEnv := map[string]string{
        "LOG_DIR": "/opt/myapp/logs",
    }
    assert.Equal(t, expectedCommandEnv, cmd.ExpandedEnv)

    // Command.ExpandedCmd should be expanded
    assert.Equal(t, "/opt/myapp/bin/server", cmd.ExpandedCmd)

    // Command.ExpandedArgs should be expanded
    expectedArgs := []string{"--log", "/opt/myapp/logs/app.log"}
    assert.Equal(t, expectedArgs, cmd.ExpandedArgs)
    ```

#### 2.2.3 重複展開の動作確認
- [ ] この時点では、展開が2回実行される（意図的）
  - 1回目: config.Loader.processConfig()（新規追加）
  - 2回目: bootstrap.LoadAndPrepareConfig()（既存）
- [ ] 両方で同じ結果が得られることを確認
- [ ] すべてのテストがPASS

#### 2.2.4 Phase 2の完了確認
- [ ] すべての既存テストがPASS
- [ ] Phase 2の新規テストがすべてPASS
- [ ] `make lint`でエラーなし
- [ ] 重複展開が意図的に存在することをコメントに記載
- [ ] コミット: "Add command expansion to config.Loader (duplicate expansion exists intentionally)"

---

### Phase 3: bootstrap側の展開処理を削除
**目的**: bootstrap側の展開処理を削除し、重複を解消する

#### 2.3.1 bootstrap/config.goの編集
- [ ] `internal/runner/bootstrap/config.go`を編集
  - [ ] `LoadAndPrepareConfig()`関数の実装を確認
  - [ ] 以下の処理を削除:
    - [ ] autoEnv生成
    - [ ] ExpandGlobalEnv()呼び出し
    - [ ] ExpandGroupEnv()呼び出し
    - [ ] ExpandCommand()呼び出し（全ループ）
  - [ ] 残す処理:
    - [ ] verificationManager.VerifyAndReadConfigFile()
    - [ ] config.Loader.LoadConfig()
  - [ ] 最終的な実装:
    ```go
    func LoadAndPrepareConfig(verificationManager *verification.Manager, configPath, runID string) (*runnertypes.Config, error) {
        if configPath == "" {
            return nil, &logging.PreExecutionError{
                Type:      logging.ErrorTypeRequiredArgumentMissing,
                Message:   "Config file path is required",
                Component: "config",
                RunID:     runID,
            }
        }

        // Perform atomic verification and reading to prevent TOCTOU attacks
        content, err := verificationManager.VerifyAndReadConfigFile(configPath)
        if err != nil {
            return nil, &logging.PreExecutionError{
                Type:      logging.ErrorTypeFileAccess,
                Message:   fmt.Sprintf("Config verification and reading failed: %v", err),
                Component: "verification",
                RunID:     runID,
            }
        }

        // Load config from the verified content
        // All expansion (Global.Env, Group.Env, Command.Env, Cmd, Args) is now
        // performed inside config.Loader.LoadConfig()
        cfgLoader := config.NewLoader()
        cfg, err := cfgLoader.LoadConfig(content)
        if err != nil {
            return nil, &logging.PreExecutionError{
                Type:      logging.ErrorTypeConfigParsing,
                Message:   fmt.Sprintf("Failed to load config: %v", err),
                Component: "config",
                RunID:     runID,
            }
        }

        return cfg, nil
    }
    ```

#### 2.3.2 bootstrap側のコメント更新
- [ ] 関数のdocコメントを更新
  - [ ] 展開処理がconfig.Loaderで行われることを明記
  - [ ] この関数の役割が「検証とロード」であることを明記
    ```go
    // LoadAndPrepareConfig loads and verifies a configuration file.
    //
    // This function performs the following steps:
    //  1. Verifies the configuration file's hash (TOCTOU protection)
    //  2. Loads the configuration using config.Loader
    //
    // Note: All variable expansion (Global.Env, Group.Env, Command.Env, Cmd, Args)
    // is now performed inside config.Loader.LoadConfig(). This function only handles
    // verification and loading.
    //
    // The returned Config is ready for execution with all variables expanded.
    ```

#### 2.3.3 bootstrap側のテスト更新（存在する場合）
- [ ] `internal/runner/bootstrap/`のテストファイルを確認
- [ ] 展開処理のテストを削除または更新
  - [ ] 展開結果の検証は残す（config.Loaderで展開済み）
  - [ ] 展開処理自体のテストは削除
- [ ] テストが正常にPASS

#### 2.3.4 統合テストの実行
- [ ] すべての既存テストがPASS
- [ ] E2Eテスト（Phase 1で追加）がPASS
- [ ] 実際のサンプルTOMLファイルで動作確認
  - [ ] `sample/`ディレクトリ内のすべてのファイル
  - [ ] Phase 1-4のテストファイル

#### 2.3.5 Phase 3の完了確認
- [ ] すべての既存テストがPASS
- [ ] Phase 3の変更がすべて完了
- [ ] `make lint`でエラーなし
- [ ] 重複展開が解消されたことを確認
- [ ] コミット: "Remove expansion logic from bootstrap (consolidate to config.Loader)"

---

### Phase 4: クリーンアップと最適化
**目的**: コードのクリーンアップ、ドキュメント更新、最終検証

#### 2.4.1 関数名の検討
- [ ] `bootstrap.LoadAndPrepareConfig`の名前を検討
  - Option A: 現在の名前を維持（「Prepare」は検証を含むと解釈）
  - Option B: `LoadVerifiedConfig`に変更（より正確）
  - Option C: `LoadConfig`に変更（シンプル）
- [ ] 決定した名前に変更（必要に応じて）
- [ ] すべての呼び出し箇所を更新

#### 2.4.2 コメントの更新
- [ ] `internal/runner/config/loader.go`
  - [ ] processConfig()のコメントを更新
  - [ ] Phase 4のコメントを追加（Command展開）
  - [ ] 全体的なアーキテクチャの説明を更新
- [ ] `internal/runner/bootstrap/config.go`
  - [ ] LoadAndPrepareConfig()のコメントを更新
  - [ ] パッケージのdocコメントを更新

#### 2.4.3 不要なコードの削除
- [ ] Phase 2で追加した重複展開のコメントを削除
- [ ] 使用されていない変数や関数を削除（あれば）
- [ ] importの整理

#### 2.4.4 ドキュメントの更新
- [ ] `docs/tasks/0031_global_group_env/02_architecture.md`を更新
  - [ ] アーキテクチャ図を更新
  - [ ] 展開処理のフローを更新
- [ ] `docs/tasks/0031_global_group_env/03_specification.md`を更新
  - [ ] 展開処理の説明を更新
- [ ] 他の関連ドキュメントを確認・更新

#### 2.4.5 パフォーマンス確認
- [ ] ベンチマークテストを実行（あれば）
- [ ] 展開処理が1回のみ実行されることを確認
- [ ] 設定ロード時間を測定（改善を確認）

#### 2.4.6 最終テスト
- [ ] すべてのテストスイートを実行
  - [ ] `make test`で全テストPASS
  - [ ] カバレッジレポートを確認
- [ ] すべてのサンプルファイルで動作確認
- [ ] リグレッションテスト
  - [ ] 既存機能に影響がないことを確認
  - [ ] パフォーマンスが改善していることを確認

#### 2.4.7 Phase 4の完了確認
- [ ] すべてのテストがPASS
- [ ] すべてのドキュメントが更新されている
- [ ] `make lint`でエラーなし
- [ ] コミット: "Refactoring: consolidate all expansion to config.Loader (complete)"

---

## 3. 完了基準

### 3.1 機能的完了基準
- [ ] すべての変数展開がconfig.Loaderで実行される
- [ ] bootstrap側に展開処理が残っていない
- [ ] Global.Env/Group.Env/Command.Env/Cmd/Argsが正しく展開される
- [ ] 既存のすべてのテストがPASS
- [ ] 既存機能に影響がない（リグレッションなし）

### 3.2 品質完了基準
- [ ] すべての既存テストがPASS
- [ ] 新規E2EテストがPASS
- [ ] テストカバレッジが低下していない
- [ ] `make lint`でエラーなし
- [ ] コードレビューが完了

### 3.3 ドキュメント完了基準
- [ ] アーキテクチャドキュメント更新完了
- [ ] 詳細仕様書更新完了
- [ ] コメントが正確で理解しやすい
- [ ] ADR（本文書）が最新状態

### 3.4 パフォーマンス完了基準
- [ ] 展開処理が1回のみ実行される
- [ ] 設定ロード時間が改善されている（または同等）
- [ ] メモリ使用量が増加していない

---

## 4. リスク管理

### 4.1 技術リスク

**リスク**: bootstrap側の削除により予期しない動作変更
- **影響度**: 高
- **対策**:
  - [ ] Phase 1でE2Eテストを追加し、動作を保証
  - [ ] Phase 2で重複展開を意図的に作り、両方の結果を比較
  - [ ] Phase 3で慎重に削除し、すぐにテスト実行

**リスク**: テストの更新漏れ
- **影響度**: 中
- **対策**:
  - [ ] すべてのテストファイルを洗い出し
  - [ ] 影響範囲を明確化
  - [ ] チェックリストで管理

**リスク**: 関数名変更による広範囲な影響
- **影響度**: 中
- **対策**:
  - [ ] Phase 4で実施（機能変更後）
  - [ ] grepで呼び出し箇所をすべて特定
  - [ ] 変更後すぐにテスト実行

### 4.2 スケジュールリスク

**リスク**: 影響範囲が予想より大きい
- **影響度**: 中
- **対策**:
  - [ ] Phase 1で影響範囲を詳細調査
  - [ ] 問題があれば早期に発見
  - [ ] 必要に応じて計画を調整

**リスク**: テストが失敗し続ける
- **影響度**: 高
- **対策**:
  - [ ] 各Phaseで既存テストを必ず実行
  - [ ] 失敗したら即座にロールバック
  - [ ] 原因を特定してから再実施

---

## 5. 実装チェックリスト

### 5.1 実装前の準備
- [ ] Task 0031（Global・Groupレベル環境変数設定機能）が完了している
- [ ] Phase 1-4がすべて正常に完了している
- [ ] ADR（06_refactoring_adr.md）をレビューし、方針を理解
- [ ] 本実装計画書をレビューし、手順を理解

### 5.2 各Phaseの実装
- [ ] Phase 1: 準備と統合テスト追加（完了）
- [ ] Phase 2: config.Loader側にCommand展開を追加（完了）
- [ ] Phase 3: bootstrap側の展開処理を削除（完了）
- [ ] Phase 4: クリーンアップと最適化（完了）

### 5.3 最終確認
- [ ] すべての完了基準を満たしている
- [ ] すべてのリスクが軽減されている
- [ ] ドキュメントが完全である
- [ ] コードレビューが完了している

---

## 6. まとめ

本リファクタリングは、展開処理の重複を解消し、アーキテクチャを明確化するための重要な改善である。

**重要なポイント**:
1. **段階的実装**: Phase 1-4で安全に移行
2. **テスト優先**: Phase 1でE2Eテスト追加、各Phaseでテスト実行
3. **重複の意図的利用**: Phase 2で重複展開を作り、両方の結果を比較
4. **慎重な削除**: Phase 3で重複を解消、すぐにテスト実行
5. **最終検証**: Phase 4でクリーンアップと最終確認

各Phaseの完了時には必ずチェックボックスをマークし、進捗を可視化すること。
