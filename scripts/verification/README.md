# Documentation Verification Scripts

This directory contains automation scripts for verifying consistency between Japanese documentation and implementation code.

## Overview

These scripts implement the automation proposed in the "8. Automation Considerations" section of [docs/tasks/0064_ja_docs_implementation_verification/execution_plan.md](../../docs/tasks/0064_ja_docs_implementation_verification/execution_plan.md).

## Scripts

### 1. verify_toml_keys.go

Extracts TOML configuration keys from Go source code and compares them with keys documented in documentation.

**Features:**
- Extract `toml:"key"` struct tags from Go code
- Extract TOML keys from documentation
- Compare both to detect missing or extra keys

**Usage:**
```bash
go run verify_toml_keys.go \
  --source=../../internal \
  --docs=../../docs/user \
  --verbose \
  --output=toml_report.json
```

**Options:**
- `--source`: Root directory of Go source code (default: `.`)
- `--docs`: Root directory of documentation (default: `docs/user`)
- `--verbose`: Verbose output
- `--output`: JSON format report output destination (optional)

### 2. verify_cli_args.go

Extracts command-line arguments from Go flag package calls and compares them with documentation.

**Features:**
- Parse calls to `flag.String()`, `flag.Bool()`, etc.
- Extract command name, argument name, type, default value, and description
- Compare with argument descriptions in documentation

**Usage:**
```bash
go run verify_cli_args.go \
  --source=../../cmd \
  --docs=../../docs/user \
  --verbose \
  --output=cli_report.json
```

**Options:**
- `--source`: Command source code directory (default: `cmd`)
- `--docs`: Root directory of documentation (default: `docs/user`)
- `--verbose`: Verbose output
- `--output`: JSON format report output destination (optional)

### 3. compare_doc_structure.go

Compares structure between Japanese documentation (.ja.md) and English documentation (.md).

**Features:**
- Compare heading levels and count
- Compare count of code blocks, tables, and lists
- Detect differences in section structure

**Usage:**
```bash
go run compare_doc_structure.go \
  --docs=../../docs/user \
  --verbose \
  --output=structure_report.json
```

**Options:**
- `--docs`: Root directory of documentation (default: `docs/user`)
- `--verbose`: Verbose output
- `--output`: JSON format report output destination (optional)

### 4. verify_links.go

Verifies links (internal links and external links) in documentation.

**Features:**
- Extract Markdown format links
- Verify existence of internal links (file paths)
- Verify access to external links (HTTP/HTTPS) (optional)

**Usage:**
```bash
go run verify_links.go \
  --docs=../../docs \
  --verbose \
  --external \
  --output=links_report.json
```

**Options:**
- `--docs`: Root directory of documentation (default: `docs`)
- `--external`: Also verify external links (may take time)
- `--verbose`: Verbose output
- `--timeout`: Timeout in seconds for external link verification (default: 10)
- `--output`: JSON format report output destination (optional)

### 5. run_all.sh

Orchestration script to execute all verification scripts in batch.

**Features:**
- Build all verification tools
- Execute each verification sequentially
- Output reports to unified location
- Display summary

**Usage:**
```bash
./run_all.sh [OPTIONS]
```

**Options:**
- `-v, --verbose`: Verbose output
- `-e, --external`: Also check external links (takes time)
- `-n, --no-json`: Do not generate JSON reports
- `-o, --output DIR`: Specify output directory (default: `build/verification-reports`)
- `-h, --help`: Display help message

**Examples:**
```bash
# Execute with default settings
./run_all.sh

# Verbose output and external link checking
./run_all.sh -v -e

# Custom output directory
./run_all.sh -o /tmp/my-reports
```

## Output Format

Each script outputs reports in the following formats:

### Text Report

Displays reports in human-readable format to standard output.

Example:
```
=== TOML Configuration Key Verification Report ===

Total keys in code: 45
Total keys in docs: 42
Keys in both: 40

⚠️  Keys in CODE but NOT in DOCS (3):
  - new_setting (struct: Config, file: config.go:123)
  ...

⚠️  Keys in DOCS but NOT in CODE (2):
  - deprecated_option
  ...
```

### JSON Report

JSON format reports can be generated with the `--output` option. This is suitable for subsequent automated processing and CI pipeline usage.

Example:
```json
{
  "in_code_only": [
    {
      "key": "new_setting",
      "type": "string",
      "struct_tag": "toml:\"new_setting\"",
      "source_file": "internal/config/config.go",
      "line_number": 123,
      "parent_struct": "Config"
    }
  ],
  "in_docs_only": ["deprecated_option"],
  "in_both": [...],
  "code_key_count": 45,
  "doc_key_count": 42
}
```

## Makefile Integration

Verification can be executed easily by adding the following targets to the project Makefile:

```makefile
# Documentation verification
.PHONY: verify-docs
verify-docs:
	@./scripts/verification/run_all.sh

# Detailed verification (including external links)
.PHONY: verify-docs-full
verify-docs-full:
	@./scripts/verification/run_all.sh -v -e
```

Usage:
```bash
make verify-docs      # Basic verification
make verify-docs-full # Complete verification (including external links)
```

## CI/CD Integration

These scripts can be integrated into CI/CD pipelines:

```yaml
# GitHub Actions example
- name: Verify Documentation
  run: |
    cd scripts/verification
    ./run_all.sh -e

- name: Upload Reports
  if: failure()
  uses: actions/upload-artifact@v3
  with:
    name: verification-reports
    path: build/verification-reports/
```

## Recommended Regular Execution

To maintain consistency between documentation and implementation, execution is recommended at the following timings:

1. **During Pull Requests**: Verify that new code changes are consistent with documentation
2. **Weekly**: Early detection of issues with regular checks
3. **Before Releases**: Final verification before release

## Troubleshooting

### When There Are Many False Positives

- Adjust exclusion lists in `isValidTOMLKey()` or `isValidArgName()` functions in `verify_toml_keys.go` or `verify_cli_args.go`
- Improve accuracy by adjusting regex patterns

### Build Errors

```bash
# Verify dependencies
go mod tidy

# Clean build
rm -rf build/verification-reports
./run_all.sh
```

### External Link Checking Is Slow

- Shorten timeout with `--timeout` option
- Execute external link checking only when necessary (without `-e` option)

## Future Improvement Ideas

- [ ] Acceleration through multi-threading support
- [ ] More precise structure analysis (such as comparing section order)
- [ ] Diff-based verification (only changed files)
- [ ] HTML report generation
- [ ] Automatic correction feature (within feasible range)
- [ ] Automatic consistency checking of translation glossary

## Reference Materials

- [execution_plan.md](../../docs/tasks/0064_ja_docs_implementation_verification/execution_plan.md) - Overall plan for verification project
- [CLAUDE.md](../../CLAUDE.md) - Project-wide guidelines
