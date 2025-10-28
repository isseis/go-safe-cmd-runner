# Test Refactoring Checklist: t.Error*/t.Fatal* to testify

このドキュメントは、ブランチ内で変更されたテストファイルのうち、
`t.Error*`や`t.Fatal*`呼び出しをtestifyの`assert`/`require`で書き直す必要があるものをリストアップしたものです。

## ファイル一覧

- [x] internal/runner/bootstrap/logger_test.go (already completed)

- [x] internal/logging/message_formatter_test.go (17 calls)
  - [x] TestDefaultMessageFormatter_CustomLevel
    - [x] L349: `t.Error("Should contain the message for custom levels")`
    - [x] L352: `t.Error("Should contain the message for custom levels")`
  - [x] TestDefaultMessageFormatter_FormatLevel
    - [x] L256: `t.Errorf("formatLevel() = %q, expected %q", result, tt.expected)`
  - [x] TestDefaultMessageFormatter_FormatLogFileHint
    - [x] L227: `t.Errorf("FormatLogFileHint() = %q, expected %q", result, tt.expected)`
  - [x] TestDefaultMessageFormatter_FormatRecordInteractive
    - [x] L182: `t.Errorf("FormatRecordInteractive() = %q, expected one of %v", result, tt.expecteds)`
  - [x] TestDefaultMessageFormatter_FormatRecordWithAttributes
    - [x] L111: `t.Error("Result should contain timestamp")`
    - [x] L114: `t.Error("Result should contain level")`
    - [x] L117: `t.Error("Result should contain message")`
    - [x] L120: `t.Error("Result should contain first attribute")`
    - [x] L123: `t.Error("Result should contain second attribute")`
  - [x] TestDefaultMessageFormatter_FormatRecordWithColor
    - [x] L91: `t.Errorf("FormatRecordWithColor() = %q, expected one of %v", result, tt.expecteds)`
  - [x] TestDefaultMessageFormatter_FormatValue
    - [x] L310: `t.Errorf("formatValue() = %q, expected %q", result, tt.expected)`
  - [x] TestMessageFormatter_Interface
    - [x] L326: `t.Error("FormatRecordWithColor should return non-empty string")`
    - [x] L332: `t.Errorf("FormatLogFileHint() = %q, expected %q", hint, expected)`
  - [x] TestNewDefaultMessageFormatter
    - [x] L13: `t.Error("NewDefaultMessageFormatter should return a non-nil instance")`
  - [x] TestShouldSkipInteractiveAttr_False
    - [x] L404: `t.Errorf("shouldSkipInteractiveAttr(%q) = true, want false", attr)`
  - [x] TestShouldSkipInteractiveAttr_True
    - [x] L380: `t.Errorf("shouldSkipInteractiveAttr(%q) = false, want true", attr)`

- [x] internal/runner/bootstrap/environment_test.go (5 calls)
  - [x] TestSetupLogging_FilePermissionError
    - [x] L158: `t.Fatalf("Failed to create read-only directory: %v", err)`
    - [x] L167: `t.Error("SetupLogging() expected error for read-only directory, got nil")`
  - [x] TestSetupLogging_InvalidConfig
    - [x] L121: `t.Fatalf("Failed to create test directory: %v", err)`
    - [x] L142: `t.Errorf("SetupLogging() error = %v, wantErr %v", err, tt.wantErr)`
  - [x] TestSetupLogging_Success
    - [x] L83: `t.Errorf("SetupLogging() error = %v, wantErr %v", err, tt.wantErr)`

- [x] internal/runner/bootstrap/verification_test.go (9 calls)
  - [x] TestInitializeVerificationManager_InvalidHashDir
    - [x] L106: `t.Fatalf("Setup failed: %v", err)`
    - [x] L119: `t.Error("InitializeVerificationManager() should return nil manager on error")`
    - [x] L123: `t.Error("InitializeVerificationManager() should return a manager on success")`
  - [x] TestInitializeVerificationManager_PermissionError
    - [x] L134: `t.Fatalf("Failed to setup logging: %v", err)`
    - [x] L153: `t.Error("InitializeVerificationManager() should return nil manager on error")`
    - [x] L157: `t.Error("InitializeVerificationManager() should return a manager on success")`
  - [x] TestInitializeVerificationManager_Success
    - [x] L47: `t.Fatalf("Failed to setup logging: %v", err)`
    - [x] L54: `t.Error("InitializeVerificationManager() should return nil manager on error")`
    - [x] L58: `t.Error("InitializeVerificationManager() should return a manager on success")`

- [ ] internal/runner/cli/output_test.go (10 calls)
  - [ ] TestParseDryRunDetailLevel_InvalidLevel
    - [ ] L69: `t.Errorf("ParseDryRunDetailLevel(%q) error = nil, want error", tt.input)`
    - [ ] L72: `t.Errorf("ParseDryRunDetailLevel(%q) error = %v, want ErrInvalidDetailLevel", tt.input, err)`
    - [ ] L76: `t.Errorf("ParseDryRunDetailLevel(%q) = %v, want %v (default)", tt.input, got, resource.DetailLevelSummary)`
  - [ ] TestParseDryRunDetailLevel_ValidLevels
    - [ ] L37: `t.Errorf("ParseDryRunDetailLevel(%q) error = %v, want nil", tt.input, err)`
    - [ ] L40: `t.Errorf("ParseDryRunDetailLevel(%q) = %v, want %v", tt.input, got, tt.want)`
  - [ ] TestParseDryRunOutputFormat_InvalidFormat
    - [ ] L136: `t.Errorf("ParseDryRunOutputFormat(%q) error = nil, want error", tt.input)`
    - [ ] L139: `t.Errorf("ParseDryRunOutputFormat(%q) error = %v, want ErrInvalidOutputFormat", tt.input, err)`
    - [ ] L143: `t.Errorf("ParseDryRunOutputFormat(%q) = %v, want %v (default)", tt.input, got, resource.OutputFormatText)`
  - [ ] TestParseDryRunOutputFormat_ValidFormats
    - [ ] L104: `t.Errorf("ParseDryRunOutputFormat(%q) error = %v, want nil", tt.input, err)`
    - [ ] L107: `t.Errorf("ParseDryRunOutputFormat(%q) = %v, want %v", tt.input, got, tt.want)`

- [ ] internal/runner/cli/validation_test.go (2 calls)
  - [ ] TestValidateConfigCommand_Invalid
    - [ ] L93: `t.Error("ValidateConfigCommand() with invalid config error = nil, want error")`
  - [ ] TestValidateConfigCommand_Valid
    - [ ] L34: `t.Errorf("ValidateConfigCommand() with valid config error = %v, want nil", err)`

- [ ] internal/runner/config/errors_test.go (28 calls)
  - [ ] TestErrCircularReferenceDetail_Error
    - [ ] L135: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrCircularReferenceDetail_Unwrap
    - [ ] L149: `t.Errorf("Unwrap() should return ErrCircularReference")`
  - [ ] TestErrDuplicateVariableDefinitionDetail_Error
    - [ ] L387: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrDuplicateVariableDefinitionDetail_Unwrap
    - [ ] L400: `t.Errorf("Unwrap() should return ErrDuplicateVariableDefinition")`
  - [ ] TestErrInvalidEnvFormatDetail_Error
    - [ ] L331: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrInvalidEnvFormatDetail_Unwrap
    - [ ] L344: `t.Errorf("Unwrap() should return ErrInvalidEnvFormat")`
  - [ ] TestErrInvalidEnvKeyDetail_Error
    - [ ] L359: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrInvalidEnvKeyDetail_Unwrap
    - [ ] L373: `t.Errorf("Unwrap() should return ErrInvalidEnvKey")`
  - [ ] TestErrInvalidEscapeSequenceDetail_Error
    - [ ] L193: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrInvalidEscapeSequenceDetail_Unwrap
    - [ ] L207: `t.Errorf("Unwrap() should return ErrInvalidEscapeSequence")`
  - [ ] TestErrInvalidFromEnvFormatDetail_Error
    - [ ] L277: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrInvalidFromEnvFormatDetail_Unwrap
    - [ ] L290: `t.Errorf("Unwrap() should return ErrInvalidFromEnvFormat")`
  - [ ] TestErrInvalidSystemVariableNameDetail_Error
    - [ ] L48: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrInvalidSystemVariableNameDetail_Unwrap
    - [ ] L62: `t.Errorf("Unwrap() should return ErrInvalidSystemVariableName")`
  - [ ] TestErrInvalidVariableNameDetail_Error
    - [ ] L19: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrInvalidVariableNameDetail_Unwrap
    - [ ] L33: `t.Errorf("Unwrap() should return ErrInvalidVariableName")`
  - [ ] TestErrInvalidVarsFormatDetail_Error
    - [ ] L304: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrInvalidVarsFormatDetail_Unwrap
    - [ ] L317: `t.Errorf("Unwrap() should return ErrInvalidVarsFormat")`
  - [ ] TestErrMaxRecursionDepthExceededDetail_Error
    - [ ] L249: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrMaxRecursionDepthExceededDetail_Unwrap
    - [ ] L263: `t.Errorf("Unwrap() should return ErrMaxRecursionDepthExceeded")`
  - [ ] TestErrReservedVariablePrefixDetail_Error
    - [ ] L77: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrReservedVariablePrefixDetail_Unwrap
    - [ ] L91: `t.Errorf("Unwrap() should return ErrReservedVariablePrefix")`
  - [ ] TestErrUnclosedVariableReferenceDetail_Error
    - [ ] L221: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrUnclosedVariableReferenceDetail_Unwrap
    - [ ] L234: `t.Errorf("Unwrap() should return ErrUnclosedVariableReference")`
  - [ ] TestErrUndefinedVariableDetail_Error
    - [ ] L164: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrUndefinedVariableDetail_Unwrap
    - [ ] L178: `t.Errorf("Unwrap() should return ErrUndefinedVariable")`
  - [ ] TestErrVariableNotInAllowlistDetail_Error
    - [ ] L106: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [ ] TestErrVariableNotInAllowlistDetail_Unwrap
    - [ ] L120: `t.Errorf("Unwrap() should return ErrVariableNotInAllowlist")`

- [ ] internal/runner/runnertypes/errors_test.go (1 calls)
  - [ ] TestSecurityViolationError_Is
    - [ ] L93: `t.Errorf("Is() should return true for SecurityViolationError instances")`


## 凡例

- `[ ]` - 未対応
- `[x]` - 対応完了
- `[-]` - 対応不要と判断

## 注意事項

- testifyの`assert.Error`/`assert.NoError`を使う場合、エラーの有無をチェックします
- testifyの`assert.ErrorIs`を使う場合、特定のエラー型をチェックします
- testifyの`require.*`を使う場合、テスト継続不可能な致命的エラーをチェックします
- `t.Errorf`で値の比較をしている場合は`assert.Equal`を使います
