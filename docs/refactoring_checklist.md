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

- [x] internal/runner/cli/output_test.go (10 calls)
  - [x] TestParseDryRunDetailLevel_InvalidLevel
    - [x] L69: `t.Errorf("ParseDryRunDetailLevel(%q) error = nil, want error", tt.input)`
    - [x] L72: `t.Errorf("ParseDryRunDetailLevel(%q) error = %v, want ErrInvalidDetailLevel", tt.input, err)`
    - [x] L76: `t.Errorf("ParseDryRunDetailLevel(%q) = %v, want %v (default)", tt.input, got, resource.DetailLevelSummary)`
  - [x] TestParseDryRunDetailLevel_ValidLevels
    - [x] L37: `t.Errorf("ParseDryRunDetailLevel(%q) error = %v, want nil", tt.input, err)`
    - [x] L40: `t.Errorf("ParseDryRunDetailLevel(%q) = %v, want %v", tt.input, got, tt.want)`
  - [x] TestParseDryRunOutputFormat_InvalidFormat
    - [x] L136: `t.Errorf("ParseDryRunOutputFormat(%q) error = nil, want error", tt.input)`
    - [x] L139: `t.Errorf("ParseDryRunOutputFormat(%q) error = %v, want ErrInvalidOutputFormat", tt.input, err)`
    - [x] L143: `t.Errorf("ParseDryRunOutputFormat(%q) = %v, want %v (default)", tt.input, got, resource.OutputFormatText)`
  - [x] TestParseDryRunOutputFormat_ValidFormats
    - [x] L104: `t.Errorf("ParseDryRunOutputFormat(%q) error = %v, want nil", tt.input, err)`
    - [x] L107: `t.Errorf("ParseDryRunOutputFormat(%q) = %v, want %v", tt.input, got, tt.want)`

- [x] internal/runner/cli/validation_test.go (2 calls)
  - [x] TestValidateConfigCommand_Invalid
    - [x] L93: `t.Error("ValidateConfigCommand() with invalid config error = nil, want error")`
  - [x] TestValidateConfigCommand_Valid
    - [x] L34: `t.Errorf("ValidateConfigCommand() with valid config error = %v, want nil", err)`

- [x] internal/runner/config/errors_test.go (28 calls)
  - [x] TestErrCircularReferenceDetail_Error
    - [x] L135: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrCircularReferenceDetail_Unwrap
    - [x] L149: `t.Errorf("Unwrap() should return ErrCircularReference")`
  - [x] TestErrDuplicateVariableDefinitionDetail_Error
    - [x] L387: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrDuplicateVariableDefinitionDetail_Unwrap
    - [x] L400: `t.Errorf("Unwrap() should return ErrDuplicateVariableDefinition")`
  - [x] TestErrInvalidEnvFormatDetail_Error
    - [x] L331: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrInvalidEnvFormatDetail_Unwrap
    - [x] L344: `t.Errorf("Unwrap() should return ErrInvalidEnvFormat")`
  - [x] TestErrInvalidEnvKeyDetail_Error
    - [x] L359: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrInvalidEnvKeyDetail_Unwrap
    - [x] L373: `t.Errorf("Unwrap() should return ErrInvalidEnvKey")`
  - [x] TestErrInvalidEscapeSequenceDetail_Error
    - [x] L193: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrInvalidEscapeSequenceDetail_Unwrap
    - [x] L207: `t.Errorf("Unwrap() should return ErrInvalidEscapeSequence")`
  - [x] TestErrInvalidFromEnvFormatDetail_Error
    - [x] L277: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrInvalidFromEnvFormatDetail_Unwrap
    - [x] L290: `t.Errorf("Unwrap() should return ErrInvalidFromEnvFormat")`
  - [x] TestErrInvalidSystemVariableNameDetail_Error
    - [x] L48: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrInvalidSystemVariableNameDetail_Unwrap
    - [x] L62: `t.Errorf("Unwrap() should return ErrInvalidSystemVariableName")`
  - [x] TestErrInvalidVariableNameDetail_Error
    - [x] L19: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrInvalidVariableNameDetail_Unwrap
    - [x] L33: `t.Errorf("Unwrap() should return ErrInvalidVariableName")`
  - [x] TestErrInvalidVarsFormatDetail_Error
    - [x] L304: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrInvalidVarsFormatDetail_Unwrap
    - [x] L317: `t.Errorf("Unwrap() should return ErrInvalidVarsFormat")`
  - [x] TestErrMaxRecursionDepthExceededDetail_Error
    - [x] L249: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrMaxRecursionDepthExceededDetail_Unwrap
    - [x] L263: `t.Errorf("Unwrap() should return ErrMaxRecursionDepthExceeded")`
  - [x] TestErrReservedVariablePrefixDetail_Error
    - [x] L77: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrReservedVariablePrefixDetail_Unwrap
    - [x] L91: `t.Errorf("Unwrap() should return ErrReservedVariablePrefix")`
  - [x] TestErrUnclosedVariableReferenceDetail_Error
    - [x] L221: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrUnclosedVariableReferenceDetail_Unwrap
    - [x] L234: `t.Errorf("Unwrap() should return ErrUnclosedVariableReference")`
  - [x] TestErrUndefinedVariableDetail_Error
    - [x] L164: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrUndefinedVariableDetail_Unwrap
    - [x] L178: `t.Errorf("Unwrap() should return ErrUndefinedVariable")`
  - [x] TestErrVariableNotInAllowlistDetail_Error
    - [x] L106: `t.Errorf("Error() = %q, want %q", err.Error(), expected)`
  - [x] TestErrVariableNotInAllowlistDetail_Unwrap
    - [x] L120: `t.Errorf("Unwrap() should return ErrVariableNotInAllowlist")`

- [x] internal/runner/runnertypes/errors_test.go (1 calls)
  - [x] TestSecurityViolationError_Is
    - [x] L93: `t.Errorf("Is() should return true for SecurityViolationError instances")`


## 凡例

- `[ ]` - 未対応
- `[x]` - 対応完了
- `[-]` - 対応不要と判断

## 注意事項

- testifyの`assert.Error`/`assert.NoError`を使う場合、エラーの有無をチェックします
- testifyの`assert.ErrorIs`を使う場合、特定のエラー型をチェックします
- testifyの`require.*`を使う場合、テスト継続不可能な致命的エラーをチェックします
- `t.Errorf`で値の比較をしている場合は`assert.Equal`を使います
