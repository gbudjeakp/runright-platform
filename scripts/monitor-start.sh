#!/usr/bin/env bash
# Start the runright monitor in the background and record its PID.
# Env vars:
#   RUNRIGHT_INTERVAL      - sampling interval (e.g. "5s")
#   RUNRIGHT_EXPORT        - export backends (e.g. "file")
#   RUNRIGHT_OUTPUT_DIR    - directory to write metrics files
#   RUNRIGHT_JOB_ID        - unique job identifier
#   RUNRIGHT_HTTP_URL      - backend URL for http export (may be empty)
#   RUNRIGHT_SLACK_WEBHOOK - Slack incoming webhook URL (may be empty)
#   RUNRIGHT_TEAMS_WEBHOOK - Microsoft Teams incoming webhook URL (may be empty)
set -euo pipefail

mkdir -p "$RUNRIGHT_OUTPUT_DIR"
runright monitor \
  --interval  "$RUNRIGHT_INTERVAL" \
  --export    "$RUNRIGHT_EXPORT" \
  --output-dir "$RUNRIGHT_OUTPUT_DIR" \
  --job-id    "$RUNRIGHT_JOB_ID" \
  --http-url  "${RUNRIGHT_HTTP_URL:-}" \
  --slack-webhook "${RUNRIGHT_SLACK_WEBHOOK:-}" \
  --teams-webhook "${RUNRIGHT_TEAMS_WEBHOOK:-}" &
echo $! > /tmp/runright-monitor.pid
echo "runright: monitor started (PID $(cat /tmp/runright-monitor.pid))"
