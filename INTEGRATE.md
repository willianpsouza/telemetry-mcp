# Integrate your service — Provengo Observability Framework

> **For an AI agent (Claude):** You are integrating the current repository's
> service into the **Provengo Observability Framework**. This single file is
> self-contained — follow it end to end. Steps:
> 1. Detect the service language and its HTTP entrypoint (`main.go`, `main.py`, …).
> 2. Ask the user for the telemetry host (`TELEMETRY_HOST`) and the service
>    name to use (kebab-case, e.g. `provengo-pay`) if not obvious.
> 3. Apply the matching section below (Go / Python / Other).
> 4. Add the environment variables.
> 5. Report what you changed and how to verify.
> Do **not** make the service talk to Prometheus/Jaeger/Loki directly — it only
> emits **OTLP** to the collector, which fans out.

This framework gives you **one** telemetry endpoint. You instrument once with
OpenTelemetry; the collector routes the three signals:

```
your service ──OTLP(4317 gRPC / 4318 HTTP)──▶ OTel Collector ─┬─▶ Prometheus (metrics)
                                                               ├─▶ Jaeger     (traces)
                                                               └─▶ Loki       (logs)
```

- **Metrics** → Prometheus (PromQL, Grafana dashboards)
- **Traces** → Jaeger (span waterfalls, latency)
- **Logs** → Loki (LogQL), correlated to traces by `trace_id`
- The join key everywhere is **`service.name`** (OTEL_SERVICE_NAME).

---

## Configuration (all languages)

Set these env vars for the service. `TELEMETRY_HOST` is the framework host.

```bash
OTEL_SERVICE_NAME=<your-service-name>            # becomes service_name label
OTEL_RESOURCE_ATTRIBUTES=service.namespace=provengo,deployment.environment=production
OTEL_EXPORTER_OTLP_ENDPOINT=http://TELEMETRY_HOST:4317
OTEL_EXPORTER_OTLP_PROTOCOL=grpc                 # or http/protobuf on :4318
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=1.0                       # lower under heavy load
```

That is the **only** configuration. Endpoints, service name, and sampling are
all driven by these standard `OTEL_*` variables — no code changes to move hosts.

### Authenticated mode (per-app token)

If the framework runs the **registry** with enforcement, register the app in the
console (`http://TELEMETRY_HOST:8082`), copy the generated snippet, and send to
the **auth proxy** on `:4319` with the token instead of the raw collector:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://TELEMETRY_HOST:4319
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer pvgo_xxxxxxxx…
```

Everything else (code, SDK) is identical. See
<https://github.com/willianpsouza/telemetry-mcp/blob/main/docs/REGISTRY.md>.

---

## Go services

### 1. Dependencies

```bash
go get \
  go.opentelemetry.io/otel \
  go.opentelemetry.io/otel/sdk \
  go.opentelemetry.io/otel/sdk/metric \
  go.opentelemetry.io/otel/sdk/log \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc \
  go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc \
  go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc \
  go.opentelemetry.io/otel/log/global \
  go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp \
  go.opentelemetry.io/contrib/instrumentation/runtime \
  go.opentelemetry.io/contrib/bridges/otelslog
```

### 2. Add `internal/telemetry/telemetry.go`

```go
// Package telemetry bootstraps OpenTelemetry (traces, metrics, logs) over OTLP.
package telemetry

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Init configures global providers and returns a shutdown func. Call once at
// startup and defer the returned shutdown. Config comes from OTEL_* env vars.
func Init(ctx context.Context) (func(context.Context) error, error) {
	res, err := sdkresource.New(ctx,
		sdkresource.WithFromEnv(), sdkresource.WithHost(), sdkresource.WithProcess())
	if err != nil {
		return nil, err
	}

	traceExp, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	metricExp, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp,
			sdkmetric.WithInterval(15*time.Second))))
	otel.SetMeterProvider(mp)

	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		return nil, err
	}

	logExp, err := otlploggrpc.New(ctx)
	if err != nil {
		return nil, err
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)))
	global.SetLoggerProvider(lp)

	return func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx), lp.Shutdown(ctx))
	}, nil
}
```

### 3. Wire it in `main`

```go
shutdown, err := telemetry.Init(ctx)
if err != nil { log.Fatalf("telemetry init: %v", err) }
defer func() {
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = shutdown(sctx)
}()

// Wrap the HTTP handler — emits http.server.request.duration (the histogram
// behind the dashboards). "http.server" is the span name root.
handler := otelhttp.NewHandler(mux, "http.server")
srv := &http.Server{Addr: ":8080", Handler: handler}
```

Outbound HTTP that propagates trace context:
```go
client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
```

Logs that ship to Loki **with `trace_id`** (enables log↔trace jumping):
```go
import "go.opentelemetry.io/contrib/bridges/otelslog"
logger := otelslog.NewLogger(os.Getenv("OTEL_SERVICE_NAME"))
logger.InfoContext(ctx, "charge accepted", "amount", 1999, "currency", "BRL")
```

Custom span + metric:
```go
ctx, span := otel.Tracer("svc").Start(ctx, "charge_card")
defer span.End()
c, _ := otel.Meter("svc").Int64Counter("payments.charges")
c.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "ok")))
```

A full runnable reference lives at
<https://github.com/willianpsouza/telemetry-mcp/tree/main/examples/instrumented-go-service>.

---

## Python services

```bash
pip install opentelemetry-distro opentelemetry-exporter-otlp
opentelemetry-bootstrap -a install     # auto-adds instrumentations (Flask, FastAPI, requests, …)
```

Run with the same env vars, wrapped by the agent:
```bash
OTEL_SERVICE_NAME=provengo-antifake-worker \
OTEL_EXPORTER_OTLP_ENDPOINT=http://TELEMETRY_HOST:4317 \
OTEL_EXPORTER_OTLP_PROTOCOL=grpc \
OTEL_LOGS_EXPORTER=otlp OTEL_METRICS_EXPORTER=otlp OTEL_TRACES_EXPORTER=otlp \
opentelemetry-instrument python worker.py
```

Manual spans/metrics use the `opentelemetry-api` exactly like Go.

---

## Other languages (Node, Java, Rust, …)

Any OpenTelemetry SDK works — the framework only speaks OTLP. Install that
language's OTel SDK + OTLP exporter, set the **same `OTEL_*` env vars** above,
and send to `TELEMETRY_HOST:4317`. Nothing else is framework-specific.

---

## Per-tool reference (what lands where, how to read it)

### Prometheus (metrics)
- HTTP server metrics from `otelhttp`: `http_server_request_duration_seconds`
  (`_bucket`/`_count`/`_sum`), labelled `service_name`, `http_route`,
  `http_response_status_code`.
- Go runtime: `process_runtime_go_*` (heap, goroutines, GC).
- Your custom counters/histograms, prefix with the service domain.
- Example — p95 latency:
  `histogram_quantile(0.95, sum by (le)(rate(http_server_request_duration_seconds_bucket{service_name="<svc>"}[5m])))`
- UI: `http://TELEMETRY_HOST:9090` · dashboards in Grafana `:3000`.

### Jaeger (traces)
- Every wrapped request is a trace; `service.name` is the Jaeger service.
- Add spans around meaningful work; `span.RecordError(err)` +
  `span.SetStatus(codes.Error, …)` on failure.
- Propagate context across services with `otelhttp.NewTransport` (W3C
  tracecontext is the default propagator here).
- UI: `http://TELEMETRY_HOST:16686`.

### Loki (logs)
- Use the `otelslog` bridge (Go) or the language's OTel log appender so logs
  carry `service_name` and `trace_id`.
- Query: `{service_name="<svc>"} |= "error"` at `http://TELEMETRY_HOST:3100`
  or via Grafana Explore.
- From a log line in Grafana you can jump to its trace, and back.

---

## Ask Claude to do it for you

Open your service repo and tell Claude:

> Read https://raw.githubusercontent.com/willianpsouza/telemetry-mcp/main/INTEGRATE.md
> and integrate this service into the Provengo Observability Framework.
> Telemetry host is `telemetry.internal`; name the service `provengo-xyz`.

Claude fetches this file, detects your stack, applies the right section, adds
the env vars, and tells you how to verify.

---

## Verify

1. Start the service pointing at `TELEMETRY_HOST:4317`, send traffic.
2. Grafana → *Provengo / Services Overview* → your `service_name` appears.
3. Jaeger → your service lists operations and recent traces.
4. Grafana Explore (Loki) → `{service_name="<svc>"}` shows logs.
5. Nothing showing? `nc -vz TELEMETRY_HOST 4317`, then
   `docker compose logs otel-collector` on the telemetry host.
   See <https://github.com/willianpsouza/telemetry-mcp/blob/main/docs/DEPLOYMENT.md>.

## Checklist

- [ ] OTel SDK + OTLP exporter added
- [ ] `telemetry.Init` (or `opentelemetry-instrument`) wired at startup
- [ ] HTTP handler wrapped (traces + the duration histogram)
- [ ] Logs go through the OTel bridge (carry `trace_id`)
- [ ] `OTEL_SERVICE_NAME` + `OTEL_EXPORTER_OTLP_ENDPOINT` set
- [ ] Service visible in Grafana, Jaeger and Loki
