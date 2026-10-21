#!/usr/bin/env sh
# Sends a Wyoming "describe" event to each voice service and checks the "info" reply.
set -eu

HOST="${VOICE_HOST:-127.0.0.1}"
OPENWAKEWORD_PORT="${OPENWAKEWORD_PORT:-10400}"
WHISPER_PORT="${WHISPER_PORT:-10300}"
PIPER_PORT="${PIPER_PORT:-10200}"
WAKEWORD_MODEL="${WAKEWORD_MODEL:-alexa}"
PIPER_VOICE="${PIPER_VOICE:-es_ES-sharvard-medium}"
SPANISH_LANGUAGE="\"es\""

failures=0

check() {
  name="$1"
  port="$2"
  expected="$3"
  reply="$(printf '{"type":"describe"}\n' | nc -w 3 "$HOST" "$port" || true)"
  if printf '%s' "$reply" | grep -q "$expected"; then
    echo "ok   $name ($HOST:$port)"
  else
    echo "FAIL $name ($HOST:$port): reply has no $expected"
    failures=$((failures + 1))
  fi
}

check openwakeword "$OPENWAKEWORD_PORT" "\"name\": \"$WAKEWORD_MODEL\""
check whisper "$WHISPER_PORT" "$SPANISH_LANGUAGE"
check piper "$PIPER_PORT" "\"name\": \"$PIPER_VOICE\""

exit "$failures"
