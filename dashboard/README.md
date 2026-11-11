# M4nhattan dashboard

Next.js dashboard for the M4nhattan services. Service and log data is mock data; prompt settings are live.

## Run

From the repository root:

```sh
make services-up
```

Open http://127.0.0.1:3005.

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

## Settings

Open http://127.0.0.1:3005/settings to edit the language model prompt. It is
stored in the `dashboard-data` volume and served at `/api/settings/prompt`.

The client reads it at startup through `VOICE_PROMPT_URL` (default
`http://127.0.0.1:3005/api/settings/prompt`). The editor shows the built-in
prompt when no custom prompt is saved; `Reset to built-in` removes the custom
prompt. `{{context}}` is replaced with the read-only Home Assistant context.

## Configuration

- `DASHBOARD_PORT`: published port (default `3005`).
- `src/lib/mock-data.ts`: mock services, logs and notifications.
