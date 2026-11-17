# Edge: Caddy + Cloudflare Tunnel

The whole stack sits behind **one Caddy reverse proxy**. Backends are not
published to the host — Caddy is the only entrypoint, routing by subdomain.
Cloudflare Tunnel exposes Caddy publicly without opening any inbound port.

```
                       ┌───────────── Cloudflare edge (TLS) ─────────────┐
 apps / operators ────▶│  obs.  ingest.  docs.  grafana.  mcp.  <domain> │
 Claude           ────▶└───────────────────────┬─────────────────────────┘
                                  Cloudflare Tunnel (outbound only)
                                                │
                                        cloudflared ──▶ caddy:80
                                                            │ routes by Host
          ┌───────────────┬───────────────┬────────────────┼───────────────┐
      registry:8082   registry:4319   docs-server:8081   grafana:3000   mcp-server:8080
      (console)       (OTLP auth)     (guides)           (dashboards)   (Bearer)
```

## Subdomain map

| Hostname            | Upstream           | Who          | Auth                         |
|---------------------|--------------------|--------------|------------------------------|
| `obs.<domain>`      | registry :8082     | operators    | registry login (cookie)      |
| `ingest.<domain>`   | registry :4319     | applications | per-app Bearer token         |
| `docs.<domain>`     | docs-server :8081  | anyone       | public read                 |
| `grafana.<domain>`  | grafana :3000      | operators    | Grafana login                |
| `mcp.<domain>`      | mcp-server :8080   | Claude       | `MCP_AUTH_TOKEN` Bearer       |

Prometheus, Jaeger, Loki and the collector have **no hostname** — reachable only
inside the Docker network. Expose them only via Grafana (or a VPN), never
publicly.

## Local (no DNS, no Cloudflare)

`*.localhost` resolves to 127.0.0.1, so after `docker compose up -d`:

- http://obs.localhost · http://docs.localhost · http://grafana.localhost
- Ingest: `http://ingest.localhost` (OTLP/HTTP + Bearer)

## Production with Cloudflare Tunnel

1. In Cloudflare Zero Trust → **Networks → Tunnels**, create a tunnel, copy its
   token into `.env` as `CLOUDFLARE_TUNNEL_TOKEN`.
2. Add public hostnames on the tunnel, each pointing at the same service
   `http://caddy:80`:
   - `obs.<domain>` → `http://caddy:80`
   - `ingest.<domain>` → `http://caddy:80`
   - `docs.<domain>`, `grafana.<domain>`, `mcp.<domain>` → `http://caddy:80`
   (Caddy distinguishes them by the `Host` header.)
3. Set in `.env`:
   ```bash
   BASE_DOMAIN=<domain>
   GRAFANA_ROOT_URL=https://grafana.<domain>
   REGISTRY_TELEMETRY_HOST=ingest.<domain>
   REGISTRY_INGEST_PORT=443
   REGISTRY_INGEST_SCHEME=https
   MCP_AUTH_TOKEN=$(openssl rand -hex 32)
   ```
4. Start everything including the tunnel:
   ```bash
   docker compose --profile edge up -d
   ```

TLS is handled by Cloudflare; Caddy stays on plain HTTP behind the tunnel.

## Hardening

- **Cloudflare Access (Zero Trust)** in front of `obs.` and `grafana.` adds SSO /
  device checks with no code. Recommended.
- `ingest.` must stay reachable by your apps but every request still needs a
  valid per-app token (the registry proxy enforces it). Revoke by disabling or
  deleting the app in the console.
- `mcp.` requires the `MCP_AUTH_TOKEN` Bearer; rotate it by changing the env and
  restarting `mcp-server`. You can also put `mcp.` behind Cloudflare Access.
- Keep raw OTLP (4317/4318) off the public edge entirely — it has no auth.
  The authenticated path is `ingest.<domain>` only.

## Note on gRPC

OTLP over gRPC through Cloudflare is finicky. The registry ingest proxy and the
generated snippets use **OTLP/HTTP (`http/protobuf`)**, which passes cleanly
over HTTPS. Use gRPC only on a trusted LAN straight to the collector.
