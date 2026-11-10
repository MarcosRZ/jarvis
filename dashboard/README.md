# M4nhattan dashboard

Panel de control de los servicios de M4nhattan. Next.js (App Router), Tailwind CSS v4 y componentes de shadcn/ui. Tema "modern minimal" de tweakcn.

Estado actual: interfaz completa con datos simulados. Muestra estado de servicios, memoria, CPU, registros y notificaciones. La integracion real con Docker (stats, logs, reinicios, memoria) llega en la siguiente fase.

## Desarrollo

Desde la raiz del repo, arranca todos los servicios (incluido el dashboard):

```sh
make services-up
```

Abre http://127.0.0.1:3005.

Solo el dashboard:

```sh
make dashboard-up
```

Sin Docker, con Node 22:

```sh
npm install
npm run dev
```

## Produccion

```sh
docker compose up -d --build
```

El contenedor se llama `m4nhattan-dashboard` y escucha en el puerto `3005` (`DASHBOARD_PORT` para cambiarlo).

## Estructura

- `src/app`: layout raiz, tema global (tweakcn "modern minimal") y layout compartido del dashboard.
- `src/app/(dashboard)`: rutas `/` (General), `/servicios`, `/registros` y `/notificaciones`.
- `src/components/ui`: componentes generados por shadcn/ui.
- `src/components/dashboard`: sidebar, topbar, tarjetas de servicio, recursos, logs y notificaciones.
- `src/lib/mock-data.ts`: datos simulados de servicios, memoria, logs y alertas.

## Siguiente fase

- Leer contenedores y stats desde el socket de Docker.
- Streaming de logs por servicio.
- Reiniciar servicios y ajustar limites de memoria.
- Notificaciones reales y persistencia.
