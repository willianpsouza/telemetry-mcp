# Provengo Observability Framework

A drop-in **observability framework**: stand up one stack, then connect any
application to it. Services emit OpenTelemetry (OTLP) to a single collector,
which fans out to **Prometheus** (metrics), **Jaeger** (traces) and **Loki**
(logs). **Grafana** draws the dashboards; an **MCP server** exposes the whole
stack to Claude as typed tools; a small **docs server** publishes the
integration guides as HTML; and a **registry** console registers applications,
issues ingest tokens, and probes their `/health`.

**Self-service integration:** a developer opens their repo and points Claude at
one public URL — [`INTEGRATE.md`](INTEGRATE.md) — and the service is
instrumented. See [One-shot integration](#one-shot-integration).

```
Go services ──OTLP──▶ OTel Collector ─┬─▶ Prometheus (metrics)
                                       ├─▶ Jaeger     (traces)
                                       └─▶ Loki       (logs)
                                             ▲
                        Grafana ─────────────┤  dashboards
                        MCP server ──────────┘  tools for Claude
```

## Why

Instrument each service **once** with OpenTelemetry instead of bolting a
different agent onto each backend. Swapping or scaling a backend is a collector
config change, never a service change. And Claude can answer "is provengo-pay
healthy? show me a slow trace" by calling the MCP tools directly.

## One-shot integration

The whole point of a framework: a developer doesn't read five docs. They open
their service repo and tell Claude —

> Read https://raw.githubusercontent.com/willianpsouza/telemetry-mcp/main/INTEGRATE.md
> and integrate this service into the Provengo Observability Framework.
> Telemetry host is `telemetry.internal`; name the service `provengo-xyz`.

[`INTEGRATE.md`](INTEGRATE.md) is a single, self-contained file: Claude fetches
it, detects the stack (Go / Python / other), applies the right recipe, adds the
`OTEL_*` env vars, and reports how to verify. No framework-specific SDK, no
per-service agent — just OTLP to one endpoint.

## Quickstart (stand up the stack)

```bash
git clone https://github.com/willianpsouza/telemetry-mcp.git
cd telemetry-mcp
cp .env.example .env            # set GRAFANA_ADMIN_PASSWORD
docker compose up -d
```

- Grafana   → http://localhost:3000  (dashboard: *Provengo / Services Overview*)
- Jaeger    → http://localhost:16686
- Prometheus→ http://localhost:9090
- Docs      → http://localhost:8081
- Registry  → http://localhost:8082  (onboarding console; OTLP auth proxy on :4319)
- MCP (HTTP)→ http://localhost:8080/mcp

Then instrument a service — see **[docs/INTEGRATION_GO.md](docs/INTEGRATION_GO.md)**
— and point it at `http://TELEMETRY_HOST:4317`.

Try it immediately with the bundled example:

```bash
cd examples/instrumented-go-service
OTEL_SERVICE_NAME=example-service \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
OTEL_EXPORTER_OTLP_PROTOCOL=grpc go run .
# in another shell: curl localhost:8090/work  (repeat a few times)
```

## Repository layout

```
.
├── docker-compose.yml           # the whole stack
├── .env.example                 # ports, passwords, retention, MCP endpoints
├── config/
│   ├── otel-collector.yaml       # OTLP in → Prometheus/Jaeger/Loki out
│   ├── prometheus.yml
│   ├── loki-config.yaml
│   └── grafana/                  # provisioned datasources + dashboard
├── mcp-server/                  # Go MCP server (stdio + Streamable HTTP)
├── registry/                    # onboarding console + app registry + OTLP auth proxy (Go + SQLite + embedded SPA)
├── docs-server/                 # Go (chi) server that renders docs/*.md
├── docs/                        # the guides below
└── examples/instrumented-go-service/  # runnable reference service
```

## Documentation

| Guide | What it covers |
|-------|----------------|
| [INTEGRATE.md](INTEGRATE.md)                  | **Start here** — one self-contained, agent-readable integration file (Go/Python/other) |
| [INTEGRATION_GO.md](docs/INTEGRATION_GO.md)   | Go deep-dive, step by step (copy-paste code) |
| [PROVENGO_SERVICES.md](docs/PROVENGO_SERVICES.md) | Per-service onboarding checklist & names |
| [DEPLOYMENT.md](docs/DEPLOYMENT.md)           | Run the telemetry host, ports, firewall, hardening |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md)       | How the pieces fit; signal paths; scaling |
| [MCP_USAGE.md](docs/MCP_USAGE.md)             | Connect Claude; the tools; example prompts |
| [REGISTRY.md](docs/REGISTRY.md)               | Onboarding console, app registry, token auth, `/health` checks |

## MCP tools (read-only)

`prometheus_query` · `prometheus_query_range` · `prometheus_list_metrics` ·
`prometheus_label_values` · `service_health` · `jaeger_list_services` ·
`jaeger_list_operations` · `jaeger_search_traces` · `jaeger_get_trace` ·
`loki_query` · `loki_labels`

## License

MIT — see [LICENSE](LICENSE).
