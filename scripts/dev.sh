#!/bin/sh
set -eu

cd "$(dirname "$0")/.."

docker compose up -d postgres >/dev/null

RUNRIGHT_DISABLE_AUTH=true \
DATABASE_URL='postgres://runright:runright@localhost:5435/runright?sslmode=disable' \
go run ./cmd/runright serve --port 8080 &
backend_pid=$!

cleanup() {
  kill "$backend_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

pnpm --dir web dev --host 0.0.0.0 --port 5173
