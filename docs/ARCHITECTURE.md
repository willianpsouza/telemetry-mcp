# Architecture

```
 ┌──────────────────────────── Provengo app servers ────────────────────────────┐
 │  ProvengoONE   ProvengoManager   ProvengoPay   ProvengoAntiFake   Companies…   │
 │     (Go)           (Go)             (Go)           (Go)             (Go)        │
 │       └──────────────┴───────── OTLP (gRPC 4317 / HTTP 4318) ──────────────┐   │
 └───────────────────────────────────────────────────────────────────────────┼──┘
                                                                               │
 ┌──────────────────────────── Telemetry server (dedicated) ──────────────────▼──┐
 │                                                                                │
 │   ┌───────────────────┐   metrics   ┌──────────────┐                           │
 │   │  OTel Collector    ├────────────▶│  Prometheus  │◀──┐                       │
 │   │  (contrib)         │   traces    ┌──────────────┐    │ PromQL / TraceQL /   │
 │   │  otlp receiver     ├────────────▶│   Jaeger     │    │ LogQL                │
 │   │  batch/memlimiter  │   logs      ┌──────────────┐    │                      │
 │   │                    ├────────────▶│    Loki      │    │                      │
 │   └───────────────────┘             └──────────────┘    │                      │
 │            ▲                                │  │  │       │                     │
 │            │ self-metrics (8888)            ▼  ▼  ▼       │                     │
 │            └──────────────────────────────┬─────────┐    │                     │
 │                                           │ Grafana │────┘  dashboards          │
 │                                           └─────────┘                          │
 │                                                                                │
 │   ┌───────────────┐  PromQL/TraceQL/LogQL over HTTP   ┌───────────────┐        │
 │   │  MCP server   │◀──────────────────────────────────│  docs-server  │        │
 │   │  (Go, :8080)  │                                    │  (Go, :8081)  │        │
 │   └──────┬────────┘                                    └───────────────┘        │
 └──────────┼─────────────────────────────────────────────────────────────────────┘
            │ MCP (stdio or Streamable HTTP)
            ▼
         Claude / any MCP client
```

## Design choices

- **One ingestion point.** Services emit OTLP only. Swapping a backend (e.g.
  Jaeger → Tempo) is a collector-config change, not a service change.
- **`service.name` is the join key.** `resource_to_telemetry_conversion` turns
  the OTLP resource attribute into the `service_name` label on metrics, the
  service in Jaeger, and an indexed label in Loki. Dashboards and MCP tools all
  pivot on it.
- **Trace ↔ log correlation.** The `otelslog` bridge stamps `trace_id` on log
  records; Grafana's derived field + datasource links let you jump from a log
  line to its trace and back.
- **MCP as the query surface for Claude.** Instead of teaching Claude three
  query languages against three APIs, the MCP server exposes typed tools
  (`prometheus_query`, `jaeger_search_traces`, `loki_query`, …) that return
  JSON. See [MCP_USAGE.md](MCP_USAGE.md).

## Signal paths

| Signal  | Service SDK                    | Collector exporter      | Backend    | Query language |
|---------|--------------------------------|-------------------------|------------|----------------|
| Metrics | otlpmetricgrpc + runtime       | `prometheus` (:8889)    | Prometheus | PromQL         |
| Traces  | otlptracegrpc + otelhttp       | `otlp/jaeger` (:4317)   | Jaeger     | Jaeger API     |
| Logs    | otlploggrpc + otelslog bridge  | `otlphttp/loki` (/otlp) | Loki       | LogQL          |

## Scaling notes

The default stack is a single-node footprint suited to a dedicated host.
For scale: Prometheus → remote-write to Mimir/Thanos; Jaeger all-in-one →
Jaeger with Cassandra/ES or Grafana Tempo; Loki single-binary → Loki
microservices on object storage. None of these change the service-side
instrumentation.
