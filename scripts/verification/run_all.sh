#!/bin/bash
# run_all.sh - Run all documentation verification scripts
#
# This script orchestrates all verification tools to check documentation
# consistency and generate reports.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUTPUT_DIR="$PROJECT_ROOT/build/verification-reports"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Command-line options
VERBOSE=0
CHECK_EXTERNAL=0
OUTPUT_JSON=1

usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Run all documentation verification scripts.

OPTIONS:
    -v, --verbose         Verbose output
    -e, --external        Check external links (may be slow)
    -n, --no-json         Don't generate JSON reports
    -o, --output DIR      Output directory (default: build/verification-reports)
    -h, --help            Show this help message

SECURITY WARNING:
    The -e/--external flag makes HTTP requests to all URLs found in documentation.
    DO NOT use this flag on untrusted branches or pull requests, as it can lead to
    Server-Side Request Forgery (SSRF) attacks. Only use for trusted content.

    See docs/security/SSRF-001-external-link-verification.md for details.

EXAMPLES:
    $0                    # Run all checks with default settings
    $0 -v                 # Run with verbose output
    $0 -e                 # Include external link checking (TRUSTED CONTENT ONLY)
    $0 -v -e -o /tmp/reports  # Verbose, external links, custom output dir

EOF
    exit 0
}

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose)
            VERBOSE=1
            shift
            ;;
        -e|--external)
            CHECK_EXTERNAL=1
            shift
            ;;
        -n|--no-json)
            OUTPUT_JSON=0
            shift
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "Unknown option: $1"
            usage
            ;;
    esac
done

# Create output directory
mkdir -p "$OUTPUT_DIR"

echo -e "${BLUE}=== Documentation Verification Suite ===${NC}"
echo -e "Project root: $PROJECT_ROOT"
echo -e "Output directory: $OUTPUT_DIR"
echo ""

# Build verification tools
echo -e "${YELLOW}[1/5] Building verification tools...${NC}"
cd "$SCRIPT_DIR"

go build -o "$OUTPUT_DIR/verify_toml_keys" verify_toml_keys.go
go build -o "$OUTPUT_DIR/verify_cli_args" verify_cli_args.go
go build -o "$OUTPUT_DIR/compare_doc_structure" compare_doc_structure.go
go build -o "$OUTPUT_DIR/verify_links" verify_links.go

echo -e "${GREEN}✓ Build complete${NC}"
echo ""

# Run TOML key verification
echo -e "${YELLOW}[2/5] Verifying TOML configuration keys...${NC}"
TOML_OPTS="--source=$PROJECT_ROOT/internal --docs=$PROJECT_ROOT/docs/user"
if [ $VERBOSE -eq 1 ]; then
    TOML_OPTS="$TOML_OPTS --verbose"
fi
if [ $OUTPUT_JSON -eq 1 ]; then
    TOML_OPTS="$TOML_OPTS --output=$OUTPUT_DIR/toml_keys_report.json"
fi

if "$OUTPUT_DIR/verify_toml_keys" $TOML_OPTS > "$OUTPUT_DIR/toml_keys_report.txt"; then
    echo -e "${GREEN}✓ TOML keys verification passed${NC}"
else
    echo -e "${RED}✗ TOML keys verification found issues${NC}"
fi
echo ""

# Run CLI argument verification
echo -e "${YELLOW}[3/5] Verifying command-line arguments...${NC}"
CLI_OPTS="--source=$PROJECT_ROOT/cmd --docs=$PROJECT_ROOT/docs/user"
if [ $VERBOSE -eq 1 ]; then
    CLI_OPTS="$CLI_OPTS --verbose"
fi
if [ $OUTPUT_JSON -eq 1 ]; then
    CLI_OPTS="$CLI_OPTS --output=$OUTPUT_DIR/cli_args_report.json"
fi

if "$OUTPUT_DIR/verify_cli_args" $CLI_OPTS > "$OUTPUT_DIR/cli_args_report.txt"; then
    echo -e "${GREEN}✓ CLI arguments verification passed${NC}"
else
    echo -e "${RED}✗ CLI arguments verification found issues${NC}"
fi
echo ""

# Run document structure comparison
echo -e "${YELLOW}[4/5] Comparing document structure (Japanese vs English)...${NC}"
STRUCT_OPTS="--docs=$PROJECT_ROOT/docs/user"
if [ $VERBOSE -eq 1 ]; then
    STRUCT_OPTS="$STRUCT_OPTS --verbose"
fi
if [ $OUTPUT_JSON -eq 1 ]; then
    STRUCT_OPTS="$STRUCT_OPTS --output=$OUTPUT_DIR/structure_comparison_report.json"
fi

if "$OUTPUT_DIR/compare_doc_structure" $STRUCT_OPTS > "$OUTPUT_DIR/structure_comparison_report.txt"; then
    echo -e "${GREEN}✓ Document structure comparison passed${NC}"
else
    echo -e "${RED}✗ Document structure comparison found issues${NC}"
fi
echo ""

# Run link verification
echo -e "${YELLOW}[5/5] Verifying links...${NC}"
LINK_OPTS="--docs=$PROJECT_ROOT/docs"
if [ $VERBOSE -eq 1 ]; then
    LINK_OPTS="$LINK_OPTS --verbose"
fi
if [ $CHECK_EXTERNAL -eq 1 ]; then
    LINK_OPTS="$LINK_OPTS --external"
fi
if [ $OUTPUT_JSON -eq 1 ]; then
    LINK_OPTS="$LINK_OPTS --output=$OUTPUT_DIR/links_report.json"
fi

if "$OUTPUT_DIR/verify_links" $LINK_OPTS > "$OUTPUT_DIR/links_report.txt"; then
    echo -e "${GREEN}✓ Link verification passed${NC}"
else
    echo -e "${RED}✗ Link verification found issues${NC}"
fi
echo ""

# Summary
echo -e "${BLUE}=== Verification Complete ===${NC}"
echo ""
echo "Reports generated in: $OUTPUT_DIR"
echo ""
echo "Text reports:"
echo "  - toml_keys_report.txt"
echo "  - cli_args_report.txt"
echo "  - structure_comparison_report.txt"
echo "  - links_report.txt"
echo ""

if [ $OUTPUT_JSON -eq 1 ]; then
    echo "JSON reports:"
    echo "  - toml_keys_report.json"
    echo "  - cli_args_report.json"
    echo "  - structure_comparison_report.json"
    echo "  - links_report.json"
    echo ""
fi

echo -e "${GREEN}✓ All verification tasks completed${NC}"
echo ""
echo "To view detailed reports:"
echo "  cat $OUTPUT_DIR/toml_keys_report.txt"
echo "  cat $OUTPUT_DIR/cli_args_report.txt"
echo "  cat $OUTPUT_DIR/structure_comparison_report.txt"
echo "  cat $OUTPUT_DIR/links_report.txt"
