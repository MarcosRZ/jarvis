# M4nhattan dashboard

Next.js dashboard for the M4nhattan services. Service, resource, activity, log
and notification data is live; prompt settings are stored on the host.

## Run

From the repository root:

```sh
make services-up
```

Open http://127.0.0.1:3005.

## Live data

The dashboard does not mount the Docker socket. A read-only collector service
(`collector/`, added to `server/compose.yaml`) is the only component with the
socket. It exposes an internal JSON API on port 8090 with no published port.
The dashboard proxies it through server-side route handlers, so the browser
only calls the same origin:

| Route | Purpose |
|---|---|
| `GET /api/status` | Services, memory, CPU, activity and derived notifications |
| `GET /api/logs` | Container logs with a cursor and `service`/`level` filters |

The pages poll `api/status` every 5 seconds and `api/logs` every 2 seconds.
Polling pauses while the page is hidden and backs off after failures. When the
collector is unreachable the UI shows an honest error or stale state; it never
falls back to sample data.

The collector refreshes its snapshot in the background every 2 seconds and
serves that cached copy, so status responses stay fast and do not wait on the
Docker API. If Docker is temporarily unavailable, it serves the last good
snapshot with the Docker source marked unavailable.

Set `COLLECTOR_URL` to change the collector address (default
`http://collector:8090`). The collector reads `VOICE_HA_URL` and
`VOICE_HA_TOKEN` when they are set in `server/.env`; otherwise the Home
Assistant card is offline and host uptime is unavailable.

The collector runs as a non-root user and needs the Docker socket group id.
`make services-up` detects it automatically. For a direct compose call, set
`DOCKER_SOCKET_GID` (for example `stat -c '%g' /var/run/docker.sock`). Compose
also adds group 0 because Docker Desktop exposes the mounted socket as
`root:root` inside the container.

Notifications are derived from container transitions. Read state is stored in
the browser, so it is per browser profile and is not an audit log.

## Dashboard only

```sh
make dashboard-up
make dashboard-down
make dashboard-logs
```

Or from this directory:

```sh
docker compose up -d --build
```

The standalone `dashboard/compose.yaml` does not include the collector. The UI
still loads, but the live data routes report the collector as unavailable.

## Settings

Open http://127.0.0.1:3005/settings to edit the language model prompt. It is
stored in the `dashboard-data` volume and served at `/api/settings/prompt`.

The client reads it at startup through `VOICE_PROMPT_URL` (default
`http://127.0.0.1:3005/api/settings/prompt`). The editor shows the built-in
prompt when no custom prompt is saved; `Reset to built-in` removes the custom
prompt. `{{context}}` is replaced with the read-only Home Assistant context.

## Prompt settings API

`GET /api/settings/prompt` returns `{ "prompt": "..." }` with the saved prompt,
or an empty string when no custom prompt is saved. `POST /api/settings/prompt`
takes `{ "prompt": "..." }` and returns the saved text in the same shape.
Errors return `{ "error": "..." }`, and `prompt` is limited to 20000
characters.

The prompt is stored as `{ "prompt": "..." }` in `/data/prompt.json` inside the
`dashboard-data` volume. `Reset to built-in` saves an empty prompt, which removes
the custom prompt and falls back to the built-in prompt.

## Configuration

- `DASHBOARD_PORT`: published port (default `3005`).
- `COLLECTOR_URL`: collector address used by the server-side route handlers.
- `DOCKER_SOCKET_GID`: host Docker socket group id for the collector.
- `VOICE_HA_URL`, `VOICE_HA_TOKEN`: optional Home Assistant source.
