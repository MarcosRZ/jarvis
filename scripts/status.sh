#!/usr/bin/env sh
# Shows the state of each voice service, the reason of each failed health
# check, and the result of scripts/verify.sh.
set -eu

SCRIPTS_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPTS_DIR/../server"

docker compose ps --all --format 'table {{.Service}}\t{{.Status}}\t{{.Ports}}'

for id in $(docker compose ps --all --quiet); do
  service="$(docker inspect --format '{{index .Config.Labels "com.docker.compose.service"}}' "$id")"
  health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$id")"
  state="$(docker inspect --format '{{.State.Status}}' "$id")"
  if [ "$state" != "running" ]; then
    echo
    echo "$service is $state. Last log lines:"
    docker compose logs --no-log-prefix --tail 5 "$service"
  elif [ "$health" = "unhealthy" ]; then
    echo
    echo "$service: last health check output:"
    docker inspect --format '{{range .State.Health.Log}}{{.Output}}{{end}}' "$id" | grep -v '^$' | tail -n 1
  fi
done

echo
"$SCRIPTS_DIR/verify.sh"
