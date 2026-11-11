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
- `src/lib/mock-data.ts`: mock services, logs and notifications.
