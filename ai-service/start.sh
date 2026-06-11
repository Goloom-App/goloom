#!/usr/bin/env bash
export PATH="/usr/bin:/home/sebastian/.local/bin:$PATH"
set -a
source "$(dirname "$0")/.env"
set +a
exec /usr/bin/uv run uvicorn app.main:app --host 0.0.0.0 --port 8090 --reload --log-level debug
