# Agent instructions

## Language

- Write everything in the repository in English: code, comments, documentation, UI copy, commit messages, and any other content.

## Software factory

- Read `factory/PLAN.md` before starting factory work. It holds the checkpoints,
  current state and recovery instructions.
- The factory queue is local. Do not add GitHub issue or pull-request steps
  unless the plan explicitly says so.
- Auto-merge is disabled until the plan says otherwise. Jobs stop at
  `needs-human`.

## Factory runner

- Add tasks to `factory/queue.json`; `factory/queue.example.json` shows the
  schema.
- Run one job with `python3 factory/runner.py`; run every ready job until the
  queue is empty with `python3 factory/runner.py --all`. Inspect commands with
  `python3 factory/runner.py --dry-run --queue factory/queue.example.json`.
- Worktrees come from the base branch. Commit `.opencode` and `AGENTS.md` before
  a real run, or the runner stops the job at `needs-human`.
- `factory/checks.sh` is the gate. A failing check stops the job before review.
- Serve `factory/` with `python3 -m http.server` and open `/dashboard.html` to
  see the queue and job state.

## Validation

- Client: `cd client && go test ./... && go vet ./...`
- Shared action catalog: `cd shared/actions && go test ./... && go vet ./...`
- Collector: `cd collector && go test ./... && go vet ./...`
- News: `cd news && go test ./... && go vet ./...`
- Format: `make lint` (gofmt and go vet for the client, shared actions, collector and news).
- Dashboard: the host has no Node. Use Docker, for example
  `docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp -e npm_config_cache=/tmp/npm-cache -v "$PWD/dashboard:/app" -w /app node:22-alpine sh -c 'npm ci --no-audit --no-fund && npm run lint && npm run build'`.
- Full factory gate: `factory/checks.sh [checkout]`. It runs the Go checks and
  the dashboard lint/build. It does not need Home Assistant.

## Fresh checkouts and worktrees

- Factory worktrees live in `.factory/worktrees/<job-id>` and do not contain
  gitignored env files. Copy `client/.env` or `server/.env` from the main
  checkout only when a command needs them, and never commit them.
- Do not run `make services-up` from a worktree. It would recreate the shared
  containers instead of testing the branch in isolation.

## Home Assistant

- The local Home Assistant setup (`server/homeassistant`, `make ha-up`, `make services-up`) is **for tests only**.
- Real testing during development is done against the home Home Assistant, unless stated otherwise.
- The local Home Assistant container is expected to be stopped; do not assume it should be started, and do not start it without being asked.
- Do not treat closed ports 8123/8080 as an environment failure.
