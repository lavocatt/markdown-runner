#!/bin/bash

# A simple integration test suite for the markdown-runner.

VERBOSE=false
if [ "$1" == "--verbose" ]; then
    VERBOSE=true
fi

# Function to print a colored message
print_msg() {
    COLOR=$1
    MSG=$2
    printf "\e[${COLOR}m%s\e[0m\n" "${MSG}"
}


# --- Test Setup ---
print_msg "33" "--- Building markdown-runner binary ---"
cd "$(dirname "$0")" # Run from the script's directory
(cd .. && go build -o test/markdown-runner)

# --- Test Cases ---
TEST_COUNTER=0
FAILED_TESTS=0

# Cleanup function to be called on exit
cleanup() {
    print_msg "33" "--- Cleaning up ---"
    rm -f markdown-runner
    rm -rf test_ws
    print_msg "32" "Cleanup complete."
}

trap cleanup EXIT

# A helper function to run a test
run_test() {
    ((TEST_COUNTER++))
    DESC=$1
    CMD=$2
    printf "\e[36mRunning test #${TEST_COUNTER}: ${DESC}...\e[0m"

    local runner_cmd="./markdown-runner"
    if [ "$VERBOSE" = "true" ]; then
        runner_cmd="./markdown-runner -v"
        printf "\n"
    fi

    local final_cmd="${CMD/.\/markdown-runner/$runner_cmd}"

    local cmd_to_run_quietly="${final_cmd}"
    if [ "$VERBOSE" = "false" ]; then
        if echo "${final_cmd}" | grep -q "grep"; then
            cmd_to_run_quietly="${final_cmd} > /dev/null"
        else
            cmd_to_run_quietly="${final_cmd} >/dev/null 2>&1"
        fi
    fi

    if eval "${cmd_to_run_quietly}"; then
        printf "\e[32m ✅\e[0m\n"
        if [ "$VERBOSE" = "true" ]; then
            eval "${final_cmd}"
        fi
        return 0
    else
        printf "\e[31m ❌\e[0m\n"
        echo -e "\e[31m--- Failing command output ---\e[0m"
        eval "${final_cmd}"
        echo -e "\e[31m------------------------------\e[0m"
        return 1
    fi
}

# --- Automatically Find and Run Tests ---
# Find all markdown files in the cases directory and run them as tests.
for test_file in cases/*.md; do
    test_name=$(basename "${test_file}" .md)
    # We expect schema_error.md to fail, so we invert the result with !
    if [ "${test_name}" == "schema_error" ]; then
        run_test "Schema error test (${test_name}) should fail as expected" \
            "! ./markdown-runner cases -f '.*'${test_file}" || ((FAILED_TESTS++))
    # The teardown test has a specific success condition
    elif [ "${test_name}" == "teardown" ]; then
        run_test "Teardown test (${test_name}) should execute teardown" \
            "./markdown-runner cases -f '.*'${test_file} 2>&1 | grep 'SUCCESS.*echo teardown should execute'" || ((FAILED_TESTS++))
    # The dry-run test has a specific success condition
    elif [ "${test_name}" == "happy" ]; then
        run_test "Dry run test for (${test_name}) should show DRY-RUN" \
            "./markdown-runner -d cases -f '.*'${test_file} 2>&1 | grep -E 'DRY-RUN.*|.*echo happy path'" || ((FAILED_TESTS++))
        run_test "Happy path test (${test_name}) should succeed" \
            "./markdown-runner cases -f '.*'${test_file}" || ((FAILED_TESTS++))
    # The verbose_set_x test verifies set -x functionality
    elif [ "${test_name}" == "verbose_set_x" ]; then
        run_test "Verbose set -x test (${test_name}) should show command tracing with -v" \
            "./markdown-runner -v cases -f '.*'${test_file} 2>&1 | grep -E '\\+ echo.*Testing set -x functionality'" || ((FAILED_TESTS++))
        run_test "Non-verbose test (${test_name}) should not show command tracing without -v" \
            "! ./markdown-runner cases -f '.*'${test_file} 2>&1 | grep -E '\\+ echo.*Testing set -x functionality'" || ((FAILED_TESTS++))
    # The start_from test verifies that -s accepts the file@stage/chunk format
    elif [ "${test_name}" == "start_from" ]; then
        run_test "Test case ${test_name}" \
            "./markdown-runner cases -f '.*'${test_file}" || ((FAILED_TESTS++))
        run_test "Start from chunk (${test_name}) should run the requested chunk and the following ones" \
            "./markdown-runner cases -f '.*'${test_file} -s start_from@main/second 2>&1 | grep -c -E 'SUCCESS.*start-from marker (second|end)' | grep -q '^2$'" || ((FAILED_TESTS++))
        run_test "Start from chunk (${test_name}) should not run the preceding chunks" \
            "./markdown-runner cases -f '.*'${test_file} -s start_from@main/second 2>&1 | grep -c -E 'SUCCESS.*start-from marker (init|first)' | grep -q '^0$'" || ((FAILED_TESTS++))
        run_test "Start from chunk index (${test_name}) should not run the preceding chunks" \
            "./markdown-runner cases -f '.*'${test_file} -s main/1 2>&1 | grep -c -E 'SUCCESS.*start-from marker (init|first)' | grep -q '^0$'" || ((FAILED_TESTS++))
        run_test "Start from unknown chunk (${test_name}) should fail" \
            "! ./markdown-runner cases -f '.*'${test_file} -s main/missing" || ((FAILED_TESTS++))
        run_test "Start from with an empty stage (${test_name}) should fail" \
            "! ./markdown-runner cases -f '.*'${test_file} -s start_from@" || ((FAILED_TESTS++))
    else
        run_test "Test case ${test_name}" \
            "./markdown-runner cases -f '.*'${test_file}" || ((FAILED_TESTS++))
    fi
done

run_test "Recursive test" \
    "./markdown-runner -r cases/recursive 2>&1 | grep -c -E 'echo.*nested' | grep -q 2" || ((FAILED_TESTS++))

run_test "Recursive test with file filter" \
    "./markdown-runner -r cases/recursive -f '.*other.md' 2>&1 | grep -c 'nested script' | grep -q 1" || ((FAILED_TESTS++))

run_test "Several breakpoints can be set" \
    "./markdown-runner --view ci -B test -B test/0 cases -f '.*'cases/happy.md" || ((FAILED_TESTS++))

run_test "Every breakpoint is validated, not only the last one" \
    "! ./markdown-runner --view ci -B test/1 -B test/0 cases -f '.*'cases/happy.md" || ((FAILED_TESTS++))

run_test "Unreachable breakpoint reports its location and reason" \
    "./markdown-runner --view ci -s main -B init cases/start_from.md 2>&1 | grep -F 'unreachable breakpoint \"init\": before the start stage'" || ((FAILED_TESTS++))

run_test "A filtered-out file cannot match a breakpoint" \
    "./markdown-runner --quiet -B start_from@main cases -f 'happy[.]md$' 2>&1 >/dev/null | grep -F 'unreachable breakpoint \"start_from@main\": no such file'" || ((FAILED_TESTS++))

run_test "A breakpoint can target a later selected file" \
    "./markdown-runner --view ci -r -B other@test cases/recursive" || ((FAILED_TESTS++))

check_invalid_start_and_breakpoints() {
    local diagnostics status=0
    diagnostics=$(./markdown-runner --view ci -s main/missing -B mian -B absent@main cases/start_from.md 2>&1 >/dev/null) || status=$?
    [[ "$status" -eq 1 &&
        "$diagnostics" == *"chunk with ID 'missing' not found in stage 'main'"* &&
        "$diagnostics" == *'"mian": unknown stage'* &&
        "$diagnostics" == *'"absent@main": no such file'* ]]
}

run_test "An invalid start chunk and unreachable breakpoints are reported on stderr" \
    "check_invalid_start_and_breakpoints" || ((FAILED_TESTS++))

prepare_breakpoint_files() {
    rm -rf test_ws &&
        mkdir -p test_ws &&
        cp cases/dynamic_breakpoint/a.md test_ws/a.md &&
        cp cases/dynamic_breakpoint/b.md.in test_ws/b.md
}

check_dynamic_breakpoint() {
    local output
    prepare_breakpoint_files &&
        output=$(./markdown-runner --view ci -B b@deploy test_ws 2>&1) &&
        [[ "$output" == *'warning: unreachable breakpoint "b@deploy": unknown stage'*"test_ws/a.md:"* &&
           -f test_ws/executed ]]
}

run_test "An earlier file can create a breakpoint target after a pre-run warning" \
    "check_dynamic_breakpoint" || ((FAILED_TESTS++))

check_breakpoints_across_files() {
    local diagnostics status=0
    prepare_breakpoint_files || return 1
    diagnostics=$(./markdown-runner --view ci -B a@typo -B b@oops test_ws 2>&1 >/dev/null) || status=$?
    [[ "$status" -eq 1 &&
        "$diagnostics" == *'"a@typo": unknown stage'* &&
        "$diagnostics" == *'"b@oops": unknown stage'* &&
        ! -e test_ws/started ]]
}

run_test "Unreachable breakpoints in separate files are reported together" \
    "check_breakpoints_across_files" || ((FAILED_TESTS++))

check_persistent_breakpoint_typo() {
    local output status=0
    prepare_breakpoint_files || return 1
    output=$(./markdown-runner --view ci -B b@typo test_ws 2>&1) || status=$?
    [[ "$status" -eq 1 &&
        "$output" == *'warning: unreachable breakpoint "b@typo": unknown stage'*"test_ws/a.md:"*'unreachable breakpoint "b@typo": unknown stage'* &&
        -f test_ws/started && ! -e test_ws/executed ]]
}

run_test "A later-file typo is warned about before commands and rejected before its file runs" \
    "check_persistent_breakpoint_typo" || ((FAILED_TESTS++))

# --- Test Summary ---
print_msg "33" "\n--- Test Summary ---"
if [ "${FAILED_TESTS}" -eq 0 ]; then
    print_msg "32" "All ${TEST_COUNTER} tests passed!"
    exit 0
else
    print_msg "31" "${FAILED_TESTS} of ${TEST_COUNTER} tests failed."
    exit 1
fi
