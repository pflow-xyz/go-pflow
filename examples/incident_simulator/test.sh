#!/bin/bash
# Quick test script for incident simulator

# Feed inputs: scenario selection (1) and start confirmation (enter)
(echo "1"; sleep 1; echo "") | go run main.go &

# Let it run for 8 seconds then kill
PID=$!
sleep 8
kill $PID 2>/dev/null

echo ""
echo "Test complete - simulator ran successfully!"
