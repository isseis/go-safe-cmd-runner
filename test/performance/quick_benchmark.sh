#!/bin/bash
# Quick performance benchmark test for small scale only

set -euo pipefail

# Check if jq is available
if ! command -v jq &> /dev/null; then
    echo "Error: jq is required for JSON parsing. Please install jq."
    exit 1
fi

# Ensure the runner binary exists
if [[ ! -f "./build/prod/runner" ]]; then
    echo "Building runner binary..."
    make build
fi

# Create results directory
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
RESULTS_DIR="$SCRIPT_DIR/results"
mkdir -p "$RESULTS_DIR"

# Configuration files - only small scale for quick test
CONFIGS=(
    "small_scale.toml:Small Scale (10 groups, 20 commands)"
)

# Test formats
FORMATS=("text" "json")

# Number of test runs per configuration (reduced for quick test)
NUM_RUNS=3

echo "=== Quick Performance Test Started ==="
echo "Date: $(date)"
echo "Number of runs per test: $NUM_RUNS"
echo

# Function to run performance test
run_perf_test() {
    local config_file="$1"
    local config_name="$2"
    local format="$3"

    echo "Testing $config_name with $format format..."

    local times=()

    for i in $(seq 1 $NUM_RUNS); do
        echo -n "  Run $i/$NUM_RUNS... "

        # Measure execution time
        local start_time=$(date +%s.%N)

        if [[ "$format" == "json" ]]; then
            ./build/prod/runner --config "test/performance/$config_file" --dry-run --dry-run-format json --dry-run-detail full > /dev/null 2>&1
        else
            ./build/prod/runner --config "test/performance/$config_file" --dry-run --dry-run-format text --dry-run-detail full > /dev/null 2>&1
        fi

        local end_time=$(date +%s.%N)
        local elapsed=$(echo "$end_time - $start_time" | bc -l)

        times+=("$elapsed")

        echo "${elapsed}s"
    done

    # Calculate average
    local total_time=0
    for time in "${times[@]}"; do
        total_time=$(echo "$total_time + $time" | bc -l)
    done
    local avg_time=$(echo "scale=4; $total_time / $NUM_RUNS" | bc -l)

    echo "  Average: ${avg_time}s"
    echo "$config_name,$format,$avg_time" >> "/tmp/quick_perf_results.csv"
    echo
}

# Initialize result file
echo "Configuration,Format,AvgTime(s)" > "/tmp/quick_perf_results.csv"

# Run execution time tests
echo "=== Quick Execution Time Test ==="
for config_entry in "${CONFIGS[@]}"; do
    IFS=':' read -r config_file config_name <<< "$config_entry"

    for format in "${FORMATS[@]}"; do
        run_perf_test "$config_file" "$config_name" "$format"
    done
done

# Calculate overhead
echo "=== Quick Overhead Analysis ==="
text_time=$(grep "Small Scale,text," /tmp/quick_perf_results.csv | cut -d',' -f3)
json_time=$(grep "Small Scale,json," /tmp/quick_perf_results.csv | cut -d',' -f3)

if [[ -n "$text_time" && -n "$json_time" && "$text_time" != "0" ]]; then
    overhead=$(echo "scale=2; (($json_time - $text_time) / $text_time) * 100" | bc -l)
    echo "JSON overhead: ${overhead}% (Text: ${text_time}s, JSON: ${json_time}s)"

    # Check if overhead is within acceptable limits
    overhead_int=$(echo "scale=0; $overhead/1" | bc -l)
    if [[ "$overhead_int" -le 10 ]]; then
        echo "✓ Overhead is within target (≤10%)"
    elif [[ "$overhead_int" -le 15 ]]; then
        echo "⚠ Overhead is within acceptable range (≤15%)"
    else
        echo "✗ Overhead exceeds acceptable limit (>15%)"
    fi
fi

echo
echo "Quick test results:"
cat "/tmp/quick_perf_results.csv"

# Clean up
rm -f "/tmp/quick_perf_results.csv"
