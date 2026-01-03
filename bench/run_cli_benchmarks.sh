#!/bin/bash

# PDF Comparison CLI Benchmark Script
# This script runs the pdfcompare tool against various scenarios to measure real-world performance.

BINARY="./pdfcompare"
FIXTURES="bench/fixtures"

# Ensure binary exists
if [ ! -f "$BINARY" ]; then
    echo "Error: Binary not found. Run 'make build' first."
    exit 1
fi

echo "================================================================================"
echo "PDFCompare CLI Benchmarks"
echo "================================================================================"
echo

# Helper function to run benchmark
run_bench() {
    local name=$1
    local file1=$2
    local file2=$3
    local extra_args=$4

    echo "--- Scenario: $name ---"
    echo "Command: $BINARY compare $file1 $file2 $extra_args"
    
    # Run and measure time
    start_time=$(python3 -c 'import time; print(time.time())')
    output=$($BINARY compare "$file1" "$file2" $extra_args 2>&1)
    status=$?
    end_time=$(python3 -c 'import time; print(time.time())')
    
    duration=$(python3 -c "print(f'{($end_time - $start_time) * 1000:.2f}')")
    
    echo "$output" | grep -E "Similarity|IDENTICAL|DIFFERENT|Layers|Duration" || echo "$output"
    echo "Total CLI execution time: ${duration}ms"
    echo
}

# 1. Byte-identical comparison (Layer 1 exit)
run_bench "Byte Identical (Fast Cache Hit)" "$FIXTURES/small_a.pdf" "$FIXTURES/small_a.pdf"

# 2. Similar but different content (Strategic sampling)
run_bench "Similar Content (Medium size, Strategic)" "$FIXTURES/medium_a.pdf" "$FIXTURES/medium_modified.pdf"

# 3. Different page count (Metadata exit)
run_bench "Page Count Mismatch (Immediate exit)" "$FIXTURES/single_a.pdf" "$FIXTURES/medium_a.pdf"

# 4. Large document sampling vs full
run_bench "Large Document (Strategic Sampling)" "$FIXTURES/large_a.pdf" "$FIXTURES/large_b.pdf" "--sampling strategic"
run_bench "Large Document (Full Comparison)" "$FIXTURES/large_a.pdf" "$FIXTURES/large_b.pdf" "--sampling all"

# 5. Visual mode only (forcing pipeline)
run_bench "Visual Mode (Override byte check)" "$FIXTURES/small_a.pdf" "$FIXTURES/small_a_copy.pdf" "--mode visual"

echo "================================================================================"
echo "Benchmarks Complete"
echo "================================================================================"
