#!/bin/bash
# Test script for graceful shutdown
# This script tests that the agent terminates cleanly without timeout

set -e

echo "🧪 Testing graceful shutdown of WOL Agent"
echo "=========================================="

# Start agent in background
echo "➤ Starting agent..."
./bin/agent --node-name=test-node --operator-address=localhost:9090 --ports=9 &
AGENT_PID=$!

echo "✓ Agent started with PID: $AGENT_PID"
echo ""

# Wait a bit to let it initialize
sleep 3
echo "➤ Agent is running, checking if it's alive..."
if kill -0 $AGENT_PID 2>/dev/null; then
    echo "✓ Agent process is alive"
else
    echo "✗ Agent process died unexpectedly"
    exit 1
fi

# Send SIGTERM (like Kubernetes does)
echo ""
echo "➤ Sending SIGTERM signal (graceful shutdown)..."
START_TIME=$(date +%s)
kill -TERM $AGENT_PID

# Wait for process to terminate with timeout
echo "➤ Waiting for agent to terminate..."
TIMEOUT=10
ELAPSED=0

while kill -0 $AGENT_PID 2>/dev/null; do
    sleep 0.5
    ELAPSED=$(( $(date +%s) - START_TIME ))
    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "✗ FAILED: Agent did not terminate within ${TIMEOUT}s"
        echo "   This indicates a graceful shutdown problem!"
        kill -9 $AGENT_PID 2>/dev/null || true
        exit 1
    fi
done

END_TIME=$(date +%s)
SHUTDOWN_TIME=$(( END_TIME - START_TIME ))

echo ""
echo "=========================================="
echo "✅ SUCCESS!"
echo "   Agent terminated gracefully in ${SHUTDOWN_TIME}s"
if [ $SHUTDOWN_TIME -le 2 ]; then
    echo "   ⭐ Excellent! Shutdown was very fast"
elif [ $SHUTDOWN_TIME -le 5 ]; then
    echo "   👍 Good shutdown time"
else
    echo "   ⚠️  Shutdown was slow (but within timeout)"
fi
echo "=========================================="

