#!/bin/bash
# Performance benchmark script for dry-run JSON output feature
#
# Requirements:
#   - GNU time (/usr/bin/time -v) for memory measurement (Linux)
#   - jq for JSON parsing
#   - bc for floating point calculations

set -euo pipefail

# Detect platform and GNU time availability
detect_time_command() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if [[ -x "/usr/bin/time" ]]; then
            # Verify it's GNU time by checking for -v option support
            if /usr/bin/time -v true 2>&1 | grep -q "Maximum resident set size"; then
                echo "gnu"
                return 0
            fi
        fi
    fi
    echo "unsupported"
    return 1
}

TIME_CMD_TYPE=$(detect_time_command)

if [[ "$TIME_CMD_TYPE" != "gnu" ]]; then
    echo "Error: This benchmark script requires GNU time for memory measurement."
    echo "       GNU time is typically available on Linux systems at /usr/bin/time"
    echo ""
    echo "Platform: $OSTYPE"
    echo ""
    echo "On Debian/Ubuntu: sudo apt-get install time"
    echo "On macOS: GNU time is not easily available; consider running in a Linux container"
    exit 1
fi

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
RESULTS_DIR="/home/issei/git/dryrun-debug-json-output-09/test/performance/results"
mkdir -p "$RESULTS_DIR"

# Configuration files
CONFIGS=(
    "small_scale.toml:Small Scale (10 groups, 20 commands)"
    "medium_scale.toml:Medium Scale (100 groups, 500 commands)"
    "large_scale.toml:Large Scale (500 groups, 5000 commands)"
)

# Test formats
FORMATS=("text" "json")

# Number of test runs per configuration
NUM_RUNS=10

echo "=== Performance Benchmark Started ==="
echo "Date: $(date)"
echo "Number of runs per test: $NUM_RUNS"
echo

# Function to run performance test
run_perf_test() {
    local config_file="$1"
    local config_name="$2"
    local format="$3"
    local results_file="$4"

    echo "Testing $config_name with $format format..."

    local total_time=0
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
        total_time=$(echo "$total_time + $elapsed" | bc -l)

        echo "${elapsed}s"
    done

    # Calculate statistics
    local avg_time=$(echo "scale=4; $total_time / $NUM_RUNS" | bc -l)

    # Find min and max
    local min_time=${times[0]}
    local max_time=${times[0]}
    for time in "${times[@]}"; do
        if (( $(echo "$time < $min_time" | bc -l) )); then
            min_time="$time"
        fi
        if (( $(echo "$time > $max_time" | bc -l) )); then
            max_time="$time"
        fi
    done

    # Calculate standard deviation
    local sum_sq_diff=0
    for time in "${times[@]}"; do
        local diff=$(echo "$time - $avg_time" | bc -l)
        local sq_diff=$(echo "$diff * $diff" | bc -l)
        sum_sq_diff=$(echo "$sum_sq_diff + $sq_diff" | bc -l)
    done
    local variance=$(echo "scale=6; $sum_sq_diff / $NUM_RUNS" | bc -l)
    local stddev=$(echo "scale=4; sqrt($variance)" | bc -l)

    # Write results
    echo "$config_name,$format,$avg_time,$min_time,$max_time,$stddev" >> "$results_file"

    echo "  Average: ${avg_time}s, Min: ${min_time}s, Max: ${max_time}s, StdDev: ${stddev}s"
    echo
}

# Function to measure memory usage
measure_memory() {
    local config_file="$1"
    local config_name="$2"
    local format="$3"
    local results_file="$4"

    echo "Measuring memory usage for $config_name with $format format..."

    local total_mem=0
    local mem_values=()

    for i in $(seq 1 5); do  # Fewer runs for memory measurement
        echo -n "  Run $i/5... "

        if [[ "$format" == "json" ]]; then
            local mem_usage=$(/usr/bin/time -v ./build/prod/runner --config "test/performance/$config_file" --dry-run --dry-run-format json --dry-run-detail full 2>&1 > /dev/null | grep "Maximum resident set size" | awk '{print $6}')
        else
            local mem_usage=$(/usr/bin/time -v ./build/prod/runner --config "test/performance/$config_file" --dry-run --dry-run-format text --dry-run-detail full 2>&1 > /dev/null | grep "Maximum resident set size" | awk '{print $6}')
        fi

        mem_values+=("$mem_usage")
        total_mem=$((total_mem + mem_usage))

        echo "${mem_usage} KB"
    done

    local avg_mem=$((total_mem / 5))

    echo "$config_name,$format,$avg_mem" >> "$results_file"
    echo "  Average memory: ${avg_mem} KB"
    echo
}

# Initialize result files
TIME_RESULTS="$RESULTS_DIR/execution_time_$(date +%Y%m%d_%H%M%S).csv"
MEMORY_RESULTS="$RESULTS_DIR/memory_usage_$(date +%Y%m%d_%H%M%S).csv"

echo "Configuration,Format,AvgTime(s),MinTime(s),MaxTime(s),StdDev(s)" > "$TIME_RESULTS"
echo "Configuration,Format,AvgMemory(KB)" > "$MEMORY_RESULTS"

# Run execution time tests
echo "=== Execution Time Benchmarks ==="
for config_entry in "${CONFIGS[@]}"; do
    IFS=':' read -r config_file config_name <<< "$config_entry"

    for format in "${FORMATS[@]}"; do
        run_perf_test "$config_file" "$config_name" "$format" "$TIME_RESULTS"
    done
done

# Run memory usage tests
echo "=== Memory Usage Benchmarks ==="
for config_entry in "${CONFIGS[@]}"; do
    IFS=':' read -r config_file config_name <<< "$config_entry"

    for format in "${FORMATS[@]}"; do
        measure_memory "$config_file" "$config_name" "$format" "$MEMORY_RESULTS"
    done
done

# Generate summary report
SUMMARY_FILE="$RESULTS_DIR/benchmark_summary_$(date +%Y%m%d_%H%M%S).txt"

echo "=== Performance Benchmark Summary ===" > "$SUMMARY_FILE"
echo "Date: $(date)" >> "$SUMMARY_FILE"
echo "Number of runs per test: $NUM_RUNS" >> "$SUMMARY_FILE"
echo >> "$SUMMARY_FILE"

echo "=== Execution Time Results ===" >> "$SUMMARY_FILE"
column -t -s ',' "$TIME_RESULTS" >> "$SUMMARY_FILE"
echo >> "$SUMMARY_FILE"

echo "=== Memory Usage Results ===" >> "$SUMMARY_FILE"
column -t -s ',' "$MEMORY_RESULTS" >> "$SUMMARY_FILE"
echo >> "$SUMMARY_FILE"

# Calculate overhead percentages
echo "=== JSON Format Overhead Analysis ===" >> "$SUMMARY_FILE"
echo >> "$SUMMARY_FILE"

# Process time results for overhead calculation
while IFS=',' read -r config format avg_time min_time max_time stddev; do
    if [[ "$format" == "text" && "$config" != "Configuration" ]]; then
        text_time="$avg_time"

        # Find corresponding JSON time
        json_line=$(grep "^$config,json," "$TIME_RESULTS")
        if [[ -n "$json_line" ]]; then
            json_time=$(echo "$json_line" | cut -d',' -f3)

            if [[ "$text_time" != "0" ]]; then
                overhead=$(echo "scale=2; (($json_time - $text_time) / $text_time) * 100" | bc -l)
                echo "$config: JSON overhead = ${overhead}% (Text: ${text_time}s, JSON: ${json_time}s)" >> "$SUMMARY_FILE"
            fi
        fi
    fi
done < "$TIME_RESULTS"

echo >> "$SUMMARY_FILE"

# Process memory results for overhead calculation
echo "=== Memory Overhead Analysis ===" >> "$SUMMARY_FILE"
while IFS=',' read -r config format avg_mem; do
    if [[ "$format" == "text" && "$config" != "Configuration" ]]; then
        text_mem="$avg_mem"

        # Find corresponding JSON memory
        json_line=$(grep "^$config,json," "$MEMORY_RESULTS")
        if [[ -n "$json_line" ]]; then
            json_mem=$(echo "$json_line" | cut -d',' -f3)

            if [[ "$text_mem" != "0" ]]; then
                mem_overhead=$(echo "scale=2; (($json_mem - $text_mem) / $text_mem) * 100" | bc -l)
                echo "$config: JSON memory overhead = ${mem_overhead}% (Text: ${text_mem}KB, JSON: ${json_mem}KB)" >> "$SUMMARY_FILE"
            fi
        fi
    fi
done < "$MEMORY_RESULTS"

echo
echo "=== Benchmark Completed ==="
echo "Results saved to:"
echo "  Time results: $TIME_RESULTS"
echo "  Memory results: $MEMORY_RESULTS"
echo "  Summary report: $SUMMARY_FILE"
echo
echo "Summary:"
cat "$SUMMARY_FILE"
