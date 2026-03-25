#!/bin/bash

echo "=========================================="
echo "Aegis Malicious Command Detection Test"
echo "=========================================="
echo ""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASSED=0

run_test() {
    local test_name="$1"
    local test_cmd="$2"
    local expected_detection="$3"
    
    echo -n "Testing: $test_name... "
    
    eval "$test_cmd" 2>/dev/null || true
    
    echo -e "${GREEN}EXECUTED${NC}"
    echo "  Command: $test_cmd"
    echo "  Expected detection: $expected_detection"
    echo ""
    ((PASSED++))
}

echo "=== Reverse Shell Tests (T1059.004) ==="
echo ""

run_test "Bash Reverse Shell" \
    "echo 'test' | /bin/bash -c 'echo test'" \
    "aegis-reverse-shell-001"

run_test "Netcat Pattern" \
    "echo 'nc -e test' | grep nc" \
    "aegis-reverse-shell-001"

run_test "Bash TCP Pattern" \
    "echo 'test /dev/tcp/127.0.0.1/4444' | grep /dev/tcp" \
    "aegis-reverse-shell-001"

echo ""
echo "=== Privilege Escalation Tests (T1548) ==="
echo ""

run_test "SUID Binary Search" \
    "find /tmp -perm -4000 2>/dev/null | head -1 || true" \
    "aegis-privesc-001"

run_test "Sudo List Check" \
    "sudo -l 2>/dev/null || echo 'sudo not available'" \
    "aegis-privesc-001"

echo ""
echo "=== Credential Access Tests (T1003) ==="
echo ""

run_test "Shadow File Access" \
    "echo 'cat /etc/shadow' | grep shadow" \
    "aegis-credential-001"

run_test "Passwd File Access" \
    "echo 'cat /etc/passwd' | grep passwd" \
    "aegis-credential-001"

echo ""
echo "=== Persistence Tests (T1053) ==="
echo ""

run_test "Cron Modification" \
    "echo 'crontab -l' | grep crontab" \
    "aegis-persistence-001"

run_test "Cron Directory Access" \
    "ls /etc/cron.d 2>/dev/null || echo 'cron.d check'" \
    "aegis-persistence-001"

echo ""
echo "=== Defense Evasion Tests (T1070) ==="
echo ""

run_test "History Clear" \
    "history -c 2>/dev/null || echo 'history test'" \
    "aegis-evasion-001"

run_test "HISTFILE Unset" \
    "echo 'unset HISTFILE' | grep HISTFILE" \
    "aegis-evasion-001"

echo ""
echo "=========================================="
echo "Test Execution Summary"
echo "=========================================="
echo ""
echo "Total tests executed: $PASSED"
echo ""
echo "Note: These commands are designed to trigger detection rules."
echo "Check the Aegis dashboard for alerts."
echo ""
echo "To verify detections:"
echo "1. Open Aegis web interface"
echo "2. Navigate to Detection > Alerts"
echo "3. Look for alerts with matching MITRE ATT&CK IDs"
echo ""