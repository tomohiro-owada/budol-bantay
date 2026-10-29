#!/usr/bin/env bash
# Keep budol-api running as the current (unprivileged) user. Safe to run every 5 minutes
# from cron and at @reboot.
#
#   - kill switch: if ~/budol/DISABLED exists, stop the service and exit
#   - sources secrets from ~/.config/budol/ai.env (mode 600) and
#     ~/budol/budol-api.env (live copy of deploy/budol-api.env.example
#     plus TURNSTILE_SECRET / BUDOL_STATS_TOKEN / BUDOL_IP_SALT)
#   - logs to ~/budol/logs/budol-api.log, rotated by size
set -u

APP_DIR="$HOME/budol"
BIN="$APP_DIR/bin/budol-api"
LOG_DIR="$APP_DIR/logs"
STATE_DIR="$APP_DIR/state"
DISABLED="$APP_DIR/DISABLED"
LOG_FILE="$LOG_DIR/budol-api.log"
PATTERN="budol/bin/budol-api"

# Kill switch: stop the service and stay stopped.
if [ -f "$DISABLED" ]; then
  pkill -u "$(id -un)" -f "$PATTERN" 2>/dev/null || true
  exit 0
fi

# Already running: nothing to do.
if pgrep -u "$(id -un)" -f "$PATTERN" >/dev/null 2>&1; then
  exit 0
fi

mkdir -p "$LOG_DIR" "$STATE_DIR" "$APP_DIR/bin"
if [ ! -x "$BIN" ]; then
  echo "ensure: $BIN missing or not executable" >>"$LOG_FILE"
  exit 1
fi

# Rotate logs by size: keep current + 5 x ~5 MB.
if [ -f "$LOG_FILE" ] && [ "$(stat -c%s "$LOG_FILE" 2>/dev/null || echo 0)" -gt 5242880 ]; then
  for i in 5 4 3 2 1; do
    [ -f "$LOG_FILE.$i" ] && mv -f "$LOG_FILE.$i" "$LOG_FILE.$((i + 1))"
  done
  mv -f "$LOG_FILE" "$LOG_FILE.1"
fi

# Load secrets and config. Values are exported to the daemon only;
# nothing is echoed or logged here.
set -a
# shellcheck disable=SC1090,SC1091
[ -f "$HOME/.config/budol/ai.env" ] && . "$HOME/.config/budol/ai.env"
# shellcheck disable=SC1090,SC1091
[ -f "$APP_DIR/budol-api.env" ] && . "$APP_DIR/budol-api.env"
set +a
export BUDOL_STATE_DIR="$STATE_DIR"
export BUDOL_ENV="${BUDOL_ENV:-prod}"

cd "$APP_DIR"
# shellcheck disable=SC2086
nohup "$BIN" >>"$LOG_FILE" 2>&1 &
echo "ensure: started budol-api pid $!" >>"$LOG_FILE"
