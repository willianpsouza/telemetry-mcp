# Provengo services — onboarding checklist

Each Provengo Go backend follows the same three steps from
[INTEGRATION_GO.md](INTEGRATION_GO.md). Pick the `OTEL_SERVICE_NAME` below so
dashboards and MCP queries stay consistent.

| Service                   | Repo path (backend)                    | `OTEL_SERVICE_NAME`     | Notes                                        |
|---------------------------|----------------------------------------|-------------------------|----------------------------------------------|
| ProvengoONE               | `ProvengoONE/backend`                  | `provengo-one`          | HTTP API — wrap mux with `otelhttp`.         |
| CompaniesDatabase         | `ProvengoCompaniesDatabase/api`        | `provengo-companies`    | API in `api/`; also instrument `geocoder`.   |
| ProvengoManager           | `ProvengoManager/backend`              | `provengo-manager`      | Has probes/ws/metrics modules — name each.   |
| ProvengoAntiFake          | `ProvengoAntiFake/backend-go`          | `provengo-antifake`     | Also `worker-go` → `provengo-antifake-worker`. |
| ProvengoPay               | `ProvengoPay/backend`                  | `provengo-pay`          | Add business spans around charge/webhook.    |

## Per-service steps (same every time)

1. Copy the shared `telemetry` package into `internal/telemetry/`.
2. Call `telemetry.Init(ctx)` at the top of `main`, defer its shutdown.
3. Wrap the HTTP handler: `otelhttp.NewHandler(mux, "http.server")`.
4. Set env in that service's compose/systemd:
   ```bash
   OTEL_SERVICE_NAME=<from table>
   OTEL_RESOURCE_ATTRIBUTES=service.namespace=provengo,deployment.environment=production
   OTEL_EXPORTER_OTLP_ENDPOINT=http://TELEMETRY_HOST:4317
   OTEL_EXPORTER_OTLP_PROTOCOL=grpc
   ```
5. Deploy, send traffic, confirm the name appears in Grafana / via MCP.

## Suggested per-service custom metrics & spans

- **provengo-pay**: `payments.charges` counter (attr `result`), spans
  `charge_card`, `handle_webhook`; alert on 5xx ratio + charge failure rate.
- **provengo-antifake**: `antifake.checks` counter (attr `verdict`), histogram
  for model inference latency; the Python worker can ship OTLP too
  (`opentelemetry-instrumentation`).
- **provengo-manager**: instrument probe runs and websocket sessions as spans;
  gauge for connected clients.
- **provengo-one**: trace auth + downstream calls; propagate context with
  `otelhttp.NewTransport`.
- **provengo-companies**: trace import/export and geocoding; counter for
  records processed.

## Non-Go components

- **Python worker (AntiFake)**: `pip install opentelemetry-distro
  opentelemetry-exporter-otlp`, run `opentelemetry-instrument python worker.py`
  with the same `OTEL_*` env. Signals land in the same collector.
- **Frontends / Caddy**: optional — Caddy can emit access logs to Loki via the
  collector's `filelog` receiver, or push OTLP from a browser SDK through the
  collector's `4318` HTTP endpoint.
