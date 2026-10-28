#!/usr/bin/env sh
# Sends a Wyoming "describe" event to each voice service and checks the "info"
# reply; checks the llama.cpp HTTP health endpoint.
set -eu

HOST="${VOICE_HOST:-127.0.0.1}"
OPENWAKEWORD_PORT="${OPENWAKEWORD_PORT:-10400}"
WHISPER_PORT="${WHISPER_PORT:-10300}"
PIPER_PORT="${PIPER_PORT:-10200}"
LLM_PORT="${LLM_PORT:-8080}"
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

reply="$(printf 'GET /health HTTP/1.0\r\nHost: %s\r\n\r\n' "$HOST" | nc -w 3 "$HOST" "$LLM_PORT" || true)"
if printf '%s' "$reply" | grep -q '^HTTP/.* 200'; then
  echo "ok   llm ($HOST:$LLM_PORT)"
else
  echo "FAIL llm ($HOST:$LLM_PORT): /health did not return 200"
  failures=$((failures + 1))
fi

exit "$failures"
