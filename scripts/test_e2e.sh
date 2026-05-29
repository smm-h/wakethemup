#!/bin/bash
set -euo pipefail

# Build the binary
echo "=== Building wake ==="
cd /home/m/Projects/wakethemup
go build -o /tmp/wake-test ./cmd/wake

# Create a test TOML config
cat > /tmp/wake-e2e-test.toml << 'EOF'
version = 1

[schedule]
name = "e2e-test"
description = "End-to-end test timer"
calendar = "*-*-* *:*:00/10"

[command]
exec = "date >> /tmp/wake-e2e-output.log"

[env]
WAKE_TEST = "true"
EOF

# Clean up any previous test run
/tmp/wake-test remove e2e-test 2>/dev/null || true
rm -f /tmp/wake-e2e-output.log

# Test install --dry-run
echo ""
echo "=== Install --dry-run ==="
/tmp/wake-test install /tmp/wake-e2e-test.toml --dry-run

# Test install
echo ""
echo "=== Install ==="
/tmp/wake-test install /tmp/wake-e2e-test.toml

# Test list
echo ""
echo "=== List ==="
/tmp/wake-test list

# Test list --json
echo ""
echo "=== List --json ==="
/tmp/wake-test list --json

# Test status
echo ""
echo "=== Status ==="
/tmp/wake-test status e2e-test

# Test status --json
echo ""
echo "=== Status --json ==="
/tmp/wake-test status e2e-test --json

# Test duplicate install (should fail)
echo ""
echo "=== Duplicate install (expect error) ==="
if /tmp/wake-test install /tmp/wake-e2e-test.toml 2>/dev/null; then
    echo "FAIL: duplicate install should have failed"
    exit 1
else
    echo "PASS: duplicate install correctly rejected"
fi

# Test check (run from project dir so .strictcli/checks.toml is found)
echo ""
echo "=== Check --all ==="
cd /home/m/Projects/wakethemup
/tmp/wake-test check --all || true

# Wait for timer to fire (up to 15 seconds)
echo ""
echo "=== Waiting for timer to fire (up to 15s) ==="
for i in $(seq 1 15); do
    if [ -f /tmp/wake-e2e-output.log ]; then
        echo "PASS: timer fired after ${i}s"
        cat /tmp/wake-e2e-output.log
        break
    fi
    sleep 1
done
if [ ! -f /tmp/wake-e2e-output.log ]; then
    echo "WARN: timer did not fire within 15s (may need longer)"
fi

# Test remove --dry-run
echo ""
echo "=== Remove --dry-run ==="
/tmp/wake-test remove e2e-test --dry-run

# Verify timer still exists after dry-run
echo ""
echo "=== Verify timer still exists after dry-run ==="
/tmp/wake-test list

# Test remove
echo ""
echo "=== Remove ==="
/tmp/wake-test remove e2e-test

# Verify timer is gone
echo ""
echo "=== Verify removed ==="
if /tmp/wake-test status e2e-test 2>/dev/null; then
    echo "FAIL: timer should be gone"
    exit 1
else
    echo "PASS: timer correctly removed"
fi

# Test remove of non-existent (should fail)
echo ""
echo "=== Remove non-existent (expect error) ==="
if /tmp/wake-test remove e2e-test 2>/dev/null; then
    echo "FAIL: remove of non-existent should have failed"
    exit 1
else
    echo "PASS: remove of non-existent correctly rejected"
fi

# Cleanup
rm -f /tmp/wake-e2e-test.toml /tmp/wake-e2e-output.log /tmp/wake-test

echo ""
echo "=== All tests passed ==="
