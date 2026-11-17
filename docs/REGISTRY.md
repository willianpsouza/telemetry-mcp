# Registry & onboarding console

A self-contained Go service (`registry/`) with an embedded SPA and SQLite. It is
the control plane of the framework: register applications, issue per-app
ingestion tokens, probe each app's `/health`, and **authenticate OTLP
ingestion**.

- **Console + API**: `http://TELEMETRY_HOST:8082`
- **Authenticated OTLP/HTTP ingest proxy**: `http://TELEMETRY_HOST:4319`
- Single binary, no build step (SPA embedded via `go:embed`, SQLite is pure-Go).

## What it stores

| Entity        | Fields                                                                 |
|---------------|------------------------------------------------------------------------|
| Operator      | username, bcrypt password, role                                        |
| Application   | name, `service_name`, owner, token (hashed), `health_url`, environment, active, health status/latency, `last_seen`, created |

Tokens are stored **hashed** (SHA-256). The plaintext token is shown **once** on
creation or rotation — copy it then.

## Quick-check via /health

Each application can register a `health_url`. The registry polls it every
`REGISTRY_HEALTH_INTERVAL` (default 30s) and the console shows a green/red badge
with the status code and latency — one place to eyeball the whole environment.
`last_seen` additionally shows the last time the app actually shipped telemetry
through the auth proxy, so "configured but silent" is visible at a glance.

## Enforcement model (authenticated ingestion)

```
app ──OTLP/HTTP + Bearer <token>──▶ registry proxy (:4319) ──▶ OTel Collector (:4318)
                                      │  validate token
                                      │  revoked/unknown → 401
                                      └─ stamp last_seen
```

To make tokens meaningful, **keep the collector's raw OTLP ports (4317/4318) off
the application network** (firewall) so every ingest must pass a valid token at
`:4319`. Revoking = disable or delete the app; its token stops working
immediately.

> Trusted single-team network? You can skip enforcement and let apps hit the
> collector's `:4317/:4318` directly (see [INTEGRATE.md](../INTEGRATE.md),
> unauthenticated mode). The registry still gives you the app inventory and
> health view.

## Onboard an application

1. Open the console `:8082`, sign in (seeded `REGISTRY_ADMIN_*`).
2. **Register application** → name, `service_name`, owner, environment,
   `health_url`.
3. Copy the generated snippet (shown once), e.g.:
   ```bash
   OTEL_SERVICE_NAME=provengo-pay
   OTEL_RESOURCE_ATTRIBUTES=service.namespace=provengo,deployment.environment=production
   OTEL_EXPORTER_OTLP_ENDPOINT=http://TELEMETRY_HOST:4319
   OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
   OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer pvgo_xxxxxxxx…
   ```
4. Paste into the service's secrets/env, deploy. The row turns green when the
   `/health` probe and first ingest succeed.

## Configuration

| Env var                      | Default                        | Purpose                                    |
|------------------------------|--------------------------------|--------------------------------------------|
| `REGISTRY_ADDR`              | `:8082`                        | console + API listen                       |
| `REGISTRY_OTLP_PROXY_ADDR`   | `:4319`                        | authenticated OTLP/HTTP listen             |
| `REGISTRY_COLLECTOR_OTLP`    | `http://otel-collector:4318`   | upstream collector OTLP/HTTP               |
| `REGISTRY_DB_PATH`           | `/data/registry.db`            | SQLite file (named volume `registry-data`) |
| `REGISTRY_ADMIN_USER/PASSWORD` | `admin` / `admin`            | seeded operator (change it)                |
| `REGISTRY_TELEMETRY_HOST`    | `localhost`                    | host shown in the generated snippet        |
| `REGISTRY_INGEST_PORT`       | `4319`                         | port shown in the snippet                  |
| `REGISTRY_HEALTH_INTERVAL`   | `30s`                          | `/health` poll cadence                     |

## Security notes

- Change `REGISTRY_ADMIN_PASSWORD`; the service warns and seeds `admin/admin`
  otherwise.
- Put the console behind the VPN/auth proxy like Grafana.
- Sessions are cookie-based (HttpOnly, 12h). Tokens are revocable per app.
- The proxy strips the inbound `Authorization` header before forwarding to the
  collector.
