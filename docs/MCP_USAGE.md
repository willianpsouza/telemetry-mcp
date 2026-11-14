# Using the telemetry MCP server with Claude

The MCP server exposes the telemetry stack as tools an MCP client (Claude Code,
Claude Desktop, …) can call. It speaks **stdio** (run locally) or **Streamable
HTTP** (run on the telemetry host).

## Tools

| Tool                       | What it does                                              |
|----------------------------|----------------------------------------------------------|
| `prometheus_query`         | Instant PromQL query                                     |
| `prometheus_query_range`   | Range PromQL query (time series)                         |
| `prometheus_list_metrics`  | List metric names (optionally scoped by selector)        |
| `prometheus_label_values`  | List values of a label (e.g. discover `service_name`)    |
| `service_health`           | Prometheus scrape target up/down                         |
| `jaeger_list_services`     | Services that reported traces                            |
| `jaeger_list_operations`   | Operations (span names) for a service                    |
| `jaeger_search_traces`     | Search traces by service/operation/tags/duration         |
| `jaeger_get_trace`         | Fetch one trace by ID                                    |
| `loki_query`               | LogQL query over a time window                           |
| `loki_labels`              | List log label names                                     |

## Run modes

### A) Local stdio (MCP client launches the binary)

Build once:

```bash
cd mcp-server && go build -o telemetry-mcp .
```

Claude Code — `~/.claude.json` (or project `.mcp.json`):

```json
{
  "mcpServers": {
    "provengo-telemetry": {
      "command": "/abs/path/to/mcp-server/telemetry-mcp",
      "env": {
        "MCP_PROMETHEUS_URL": "http://TELEMETRY_HOST:9090",
        "MCP_JAEGER_URL": "http://TELEMETRY_HOST:16686",
        "MCP_LOKI_URL": "http://TELEMETRY_HOST:3100"
      }
    }
  }
}
```

Or register via CLI:

```bash
claude mcp add provengo-telemetry /abs/path/to/telemetry-mcp \
  -e MCP_PROMETHEUS_URL=http://TELEMETRY_HOST:9090 \
  -e MCP_JAEGER_URL=http://TELEMETRY_HOST:16686 \
  -e MCP_LOKI_URL=http://TELEMETRY_HOST:3100
```

### B) Streamable HTTP (shared, runs in docker-compose)

The `mcp-server` service in `docker-compose.yml` runs with `MCP_TRANSPORT=http`
and listens on `:8080/mcp`. Point an HTTP-capable MCP client at it:

```json
{
  "mcpServers": {
    "provengo-telemetry": {
      "type": "http",
      "url": "http://TELEMETRY_HOST:8080/mcp"
    }
  }
}
```

## Example prompts

- "Which Provengo services are currently reporting metrics?"
  → `prometheus_label_values(label="service_name")`
- "Show me the p99 latency of provengo-pay over the last hour."
  → `prometheus_query_range(query="histogram_quantile(0.99, sum by (le)(rate(http_server_request_duration_seconds_bucket{service_name=\"provengo-pay\"}[5m])))")`
- "Any 5xx errors in provengo-manager recently? Find a slow trace."
  → `loki_query(query="{service_name=\"provengo-manager\"} |= \"error\"")` then
    `jaeger_search_traces(service="provengo-manager", min_duration="1s")`
- "Is anything down right now?" → `service_health(state="active")`

## Security

The MCP server is **read-only** — it only issues GET queries to the backends.
Still, exposing `:8080` grants access to all telemetry; keep it on a trusted
network or behind an auth proxy, same as Grafana.
