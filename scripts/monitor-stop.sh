#!/usr/bin/env bash
# Stop the runright monitor.
# If the process is already dead, sets RUNRIGHT_UNEXPECTED_EXIT=1 in GITHUB_ENV
# so the recommend step knows to fall back to the heartbeat checkpoint.
set -euo pipefail

PID_FILE=/tmp/runright-monitor.pid

if [[ ! -f "$PID_FILE" ]]; then
  echo "runright: no PID file found, monitor may not have started"
  exit 0
fi

PID=$(cat "$PID_FILE")

if ! kill -0 "$PID" 2>/dev/null; then
  # Process already dead — likely OOM-killed or force-stopped by the runner.
  echo "::warning::RunRight: monitor exited unexpectedly before the job finished. " \
    "Your runner may have run out of memory or been force-stopped. " \
    "Check that your runner is online and consider upgrading to a larger instance type."
  echo "RUNRIGHT_UNEXPECTED_EXIT=1" >> "$GITHUB_ENV"
else
  kill -TERM "$PID" 2>/dev/null || true
  # `wait` only works for child PIDs of the current shell; poll instead.
  for i in $(seq 1 40); do
    kill -0 "$PID" 2>/dev/null || break
    sleep 0.25
  done
fi

rm -f "$PID_FILE"
