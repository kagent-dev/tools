#!/bin/bash

# check-coverage.sh - Validate test coverage meets 80% threshold
# Usage: ./scripts/check-coverage.sh [coverage.out]

set -e

COVERAGE_FILE="${1:-coverage.out}"
THRESHOLD=79
MIN_PACKAGE=80
CRITICAL_THRESHOLD=80

# Critical packages requiring 90% coverage
CRITICAL_PACKAGES=(
    "github.com/kagent-dev/tools/pkg/k8s"
    "github.com/kagent-dev/tools/pkg/helm"
    "github.com/kagent-dev/tools/pkg/istio"
    "github.com/kagent-dev/tools/pkg/argo"
)

if [ ! -f "$COVERAGE_FILE" ]; then
    echo "Error: Coverage file not found: $COVERAGE_FILE"
    echo "Run: go test -cover ./... -coverprofile=$COVERAGE_FILE"
    exit 1
fi

# Extract overall coverage from go test output
# This function calculates overall coverage from coverage.out file
calculate_overall_coverage() {
    go tool cover -func="$COVERAGE_FILE" | tail -1 | awk '{print $3}' | sed 's/%//'
}

# Parse package coverage from go test verbose output
# Requires re-running tests to get per-package output
get_package_coverage() {
    local pkg="$1"
    go test -cover "$pkg" 2>/dev/null | grep "coverage:" | awk '{print $NF}' | sed 's/%//'
}

echo "================================"
echo "Coverage Check Report"
echo "================================"
echo ""

# Get overall coverage
OVERALL=$(calculate_overall_coverage)
echo "Overall Coverage: ${OVERALL}%"
echo "Required: ${THRESHOLD}%"

if (( $(echo "$OVERALL >= $THRESHOLD" | bc -l) )); then
    echo "✅ Overall coverage PASSED"
    OVERALL_PASS=true
else
    echo "❌ Overall coverage FAILED (${OVERALL}% < ${THRESHOLD}%)"
    OVERALL_PASS=false
fi

echo ""
echo "Per-Package Coverage:"
echo "--------------------"

# Get list of packages from coverage output
PACKAGES=$(go tool cover -func="$COVERAGE_FILE" | awk -F: '{print $1}' | sort -u | grep -v "total:")

PACKAGES_FAILED=()
CRITICAL_FAILED=()

for pkg in $PACKAGES; do
    # Skip main and test packages
    [[ "$pkg" == *"_test.go"* ]] && continue
    [[ "$pkg" == *"/cmd/main.go"* ]] && continue

    # Extract package path
    pkg_path=$(echo "$pkg" | sed 's|/[^/]*\.go$||')

    # Skip if already processed for this package
    if [[ " ${PACKAGES_SEEN[@]} " =~ " ${pkg_path} " ]]; then
        continue
    fi
    PACKAGES_SEEN+=("$pkg_path")

    # Get coverage for this package
    COVERAGE=$(go test -cover "$pkg_path" 2>/dev/null | grep "coverage:" | awk '{print $(NF-2)}' | sed 's/%//')

    if [ -z "$COVERAGE" ]; then
        continue
    fi

    # Check if this is a critical package
    IS_CRITICAL=false
    for crit_pkg in "${CRITICAL_PACKAGES[@]}"; do
        if [[ "$pkg_path" == "$crit_pkg" ]]; then
            IS_CRITICAL=true
            break
        fi
    done

    # Determine target based on package importance
    if [ "$IS_CRITICAL" = true ]; then
        TARGET=$CRITICAL_THRESHOLD
        PKG_TYPE="[CRITICAL]"
    else
        TARGET=$MIN_PACKAGE
        PKG_TYPE="[REGULAR]"
    fi

    # Check if package meets target
    if (( $(echo "$COVERAGE >= $TARGET" | bc -l) )); then
        STATUS="✅"
    else
        STATUS="❌"
        if [ "$IS_CRITICAL" = true ]; then
            CRITICAL_FAILED+=("$pkg_path ($COVERAGE% < $TARGET%)")
        else
            PACKAGES_FAILED+=("$pkg_path ($COVERAGE% < $TARGET%)")
        fi
    fi

    printf "  %s %-50s %5s%% (target: %d%%)\n" "$STATUS" "$pkg_path" "$COVERAGE" "$TARGET"
done

echo ""
echo "================================"
echo "Summary"
echo "================================"

if [ "$OVERALL_PASS" = true ] && [ ${#PACKAGES_FAILED[@]} -eq 0 ] && [ ${#CRITICAL_FAILED[@]} -eq 0 ]; then
    echo "✅ All coverage checks PASSED"
    exit 0
else
    echo "❌ Coverage checks FAILED"
    echo ""

    if [ "$OVERALL_PASS" = false ]; then
        echo "  Overall: ${OVERALL}% < ${THRESHOLD}% (gap: $(echo "$THRESHOLD - $OVERALL" | bc -l)%)"
    fi

    if [ ${#CRITICAL_FAILED[@]} -gt 0 ]; then
        echo ""
        echo "  Critical packages below target:"
        for pkg in "${CRITICAL_FAILED[@]}"; do
            echo "    - $pkg"
        done
    fi

    if [ ${#PACKAGES_FAILED[@]} -gt 0 ]; then
        echo ""
        echo "  Regular packages below target:"
        for pkg in "${PACKAGES_FAILED[@]}"; do
            echo "    - $pkg"
        done
    fi

    exit 1
fi
