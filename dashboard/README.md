# M4nhattan dashboard

Next.js dashboard for the M4nhattan services. It currently uses mock data.

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

## Configuration

- `DASHBOARD_PORT`: published port (default `3005`).
- `src/lib/mock-data.ts`: mock services, logs and notifications.
