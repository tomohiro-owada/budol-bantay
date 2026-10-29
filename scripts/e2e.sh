#!/usr/bin/env bash
# E2E (mock AI, local only): build the dev frontend, start the API
# (BUDOL_ENV=dev AI_MODE=mock on 127.0.0.1:18790) and a static server on
# 127.0.0.1:8788, screenshot the merged rules+AI result, and assert via
# --dump-dom that the AI section rendered. Kills ONLY its own PIDs.
set -euo pipefail
cd "$(dirname "$0")/.."

go run ./cmd/build -env dev
mkdir -p /tmp/budol-e2e/state poc
go build -o /tmp/budol-e2e/budol-api ./cmd/budol-api

API_PID=""
WEB_PID=""
cleanup() {
  if [ -n "$API_PID" ]; then kill "$API_PID" 2>/dev/null || true; fi
  if [ -n "$WEB_PID" ]; then kill "$WEB_PID" 2>/dev/null || true; fi
}
trap cleanup EXIT

# E2E_REAL=1 uses the real configured provider (key read from the env var named
# by ai.key_env, e.g. AI_API_KEY, inherited by the API process only; never
# printed). Default is the mock provider. Output PNG: poc/preview-ai.png (real)
# or poc/preview-ai-mock.png (mock).
if [ "${E2E_REAL:-0}" = "1" ]; then AI_MODE_VAL=""; SHOT=poc/preview-ai.png; else AI_MODE_VAL=mock; SHOT=poc/preview-ai-mock.png; fi
BUDOL_ENV=dev AI_MODE=$AI_MODE_VAL BUDOL_LISTEN=127.0.0.1:18790 \
  ALLOWED_ORIGINS=http://localhost:8788 BUDOL_STATE_DIR=/tmp/budol-e2e/state \
  setsid /tmp/budol-e2e/budol-api >/tmp/budol-e2e/api.log 2>&1 &
API_PID=$!
echo "$API_PID" > /tmp/budol-e2e/api.pid

python3 -m http.server 8788 --bind 127.0.0.1 -d dist >/tmp/budol-e2e/web.log 2>&1 &
WEB_PID=$!
echo "$WEB_PID" > /tmp/budol-e2e/web.pid

# Wait for the API (up to ~10 s).
ok=0
for _ in $(seq 1 50); do
  if curl -s -o /dev/null http://127.0.0.1:18790/v1/health; then ok=1; break; fi
  sleep 0.2
done
if [ "$ok" != 1 ]; then echo "E2E FAIL: API did not start"; exit 1; fi
# Wait for the static server.
for _ in $(seq 1 50); do
  if curl -s -o /dev/null http://127.0.0.1:8788/budol-bantay/; then break; fi
  sleep 0.2
done

URL='http://localhost:8788/budol-bantay/#demo=1&auto'
google-chrome --headless=new --no-sandbox --disable-gpu \
  --virtual-time-budget=40000 --window-size=412,2400 \
  --force-device-scale-factor=2 --screenshot="$SHOT" "$URL"

DUMP="$(google-chrome --headless=new --no-sandbox --disable-gpu \
  --virtual-time-budget=40000 --dump-dom "$URL")"
echo "$DUMP" | grep -q 'id="ai-section"' \
  || { echo "E2E FAIL: no #ai-section in dump-dom"; exit 1; }
echo "$DUMP" | grep -q 'data-ai-status="ok"' \
  || { echo "E2E FAIL: ai-status is not ok in dump-dom"; exit 1; }
echo "E2E OK: AI section rendered with ai_status=ok (${AI_MODE_VAL:-real provider}); screenshot $SHOT"
