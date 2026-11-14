# Deploying the telemetry server

The stack is designed to run on a **dedicated telemetry host**, separate from
the Provengo application servers. Applications only need network access to the
OTLP ports on this host.

## What runs here

| Component       | Image                                            | Port(s)        | Purpose                        |
|-----------------|--------------------------------------------------|----------------|--------------------------------|
| OTel Collector  | `otel/opentelemetry-collector-contrib`           | 4317, 4318, 8888 | OTLP ingestion + fan-out     |
| Prometheus      | `prom/prometheus`                                | 9090           | metrics storage + PromQL       |
| Jaeger          | `jaegertracing/all-in-one`                       | 16686          | trace storage + query UI       |
| Loki            | `grafana/loki`                                   | 3100           | log storage + LogQL            |
| Grafana         | `grafana/grafana`                                | 3000           | dashboards                     |
| MCP server      | built from `./mcp-server`                        | 8080           | telemetry tools for Claude     |
| Docs server     | built from `./docs-server`                       | 8081           | integration guides as HTML     |

## Bring it up

```bash
cp .env.example .env         # edit GRAFANA_ADMIN_PASSWORD at minimum
docker compose up -d
docker compose ps
```

- Grafana:  http://TELEMETRY_HOST:3000  (admin / your password)
- Jaeger:   http://TELEMETRY_HOST:16686
- Prometheus: http://TELEMETRY_HOST:9090
- Docs:     http://TELEMETRY_HOST:8081
- MCP:      http://TELEMETRY_HOST:8080/mcp  (Streamable HTTP)

## Firewall

Open **inbound** from the Provengo app subnet to the telemetry host:

| Port | Proto | From            | Why                        |
|------|-------|-----------------|----------------------------|
| 4317 | TCP   | app servers     | OTLP gRPC (telemetry in)   |
| 4318 | TCP   | app servers     | OTLP HTTP (telemetry in)   |
| 3000 | TCP   | operators/VPN   | Grafana                    |
| 8080 | TCP   | Claude/MCP host | MCP server                 |
| 8081 | TCP   | operators/VPN   | Docs                       |

Keep 9090/16686/3100 internal (operators via VPN only). The OTLP ports carry
your telemetry — put them on a private network or add TLS + auth (see below).

## Hardening (production)

- **TLS on OTLP**: terminate TLS at a reverse proxy (Caddy/Nginx) in front of
  the collector, or configure the collector `otlp` receiver with
  `tls.cert_file`/`tls.key_file`, and set `OTEL_EXPORTER_OTLP_ENDPOINT=https://…`
  on services.
- **Auth**: put Grafana, Prometheus, Jaeger behind the VPN or an auth proxy.
  Add a bearer token to the collector via the `headerssetter`/`bearertokenauth`
  extensions if OTLP is exposed beyond a trusted network.
- **Retention**: tune `PROM_RETENTION` (metrics) and `retention_period` in
  `config/loki-config.yaml` (logs). Jaeger all-in-one keeps traces in memory —
  for durable traces move to a Jaeger backend with Cassandra/Elasticsearch or
  Tempo.
- **Volumes**: `prometheus-data`, `loki-data`, `grafana-data` are named
  volumes; back them up.

## Troubleshooting

- **Service not visible**: check the service can reach `TELEMETRY_HOST:4317`
  (`nc -vz TELEMETRY_HOST 4317`). Check collector logs:
  `docker compose logs otel-collector`.
- **Metrics missing but traces present**: confirm `OTEL_SERVICE_NAME` is set;
  the dashboard groups by `service_name`.
- **Logs missing**: Loki needs `allow_structured_metadata: true` (already set)
  for OTLP; check `docker compose logs loki`.
- **Collector dropping data**: watch `otelcol_exporter_send_failed_*` and
  `otelcol_processor_dropped_*` on the `otel-collector-internal` Prometheus job.
