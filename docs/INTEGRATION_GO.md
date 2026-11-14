# Integrating a Go service with the Provengo telemetry stack

This guide shows how to wire a Go backend (ProvengoONE, ProvengoManager,
ProvengoPay, ProvengoAntiFake, CompaniesDatabase, …) to the central telemetry
server so its **metrics**, **traces**, and **logs** flow through one OTLP
endpoint.

```
your Go service ──OTLP(4317/4318)──▶ OTel Collector ─▶ Prometheus / Jaeger / Loki
```

You instrument **once** with OpenTelemetry; the collector fans the three
signals out to the right backend. You never talk to Prometheus/Jaeger/Loki
directly from the service.

---

## 0. Prerequisites

- Go 1.24+ (all Provengo backends are already on 1.25/1.26).
- Network access from the service host to the telemetry host on
  `4317` (gRPC) or `4318` (HTTP). See [DEPLOYMENT.md](DEPLOYMENT.md).
- One environment variable per service naming it (see step 4).

---

## 1. Add the dependencies

```bash
go get \
  go.opentelemetry.io/otel \
  go.opentelemetry.io/otel/sdk \
  go.opentelemetry.io/otel/sdk/metric \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc \
  go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc \
  go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc \
  go.opentelemetry.io/otel/log/global \
  go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp \
  go.opentelemetry.io/contrib/instrumentation/runtime
```

---

## 2. Drop in the shared `telemetry` package

Copy `telemetry/telemetry.go` (below) into your repo as
`internal/telemetry/telemetry.go`. It sets up traces, metrics, logs and Go
runtime metrics from a single `Init(ctx)` call, driven entirely by the
standard `OTEL_*` environment variables.

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

// Init configures global providers and returns a shutdown func. Call it once
// at startup and defer the returned shutdown. All endpoints/service name come
// from OTEL_* env vars (see step 4).
func Init(ctx context.Context) (func(context.Context) error, error) {
	// Resource (service.name etc.) is read from OTEL_RESOURCE_ATTRIBUTES and
	// OTEL_SERVICE_NAME automatically by the default resource detectors.
	res, err := sdkresource.New(ctx,
		sdkresource.WithFromEnv(),
		sdkresource.WithHost(),
		sdkresource.WithProcess(),
	)
	if err != nil {
		return nil, err
	}

	// --- Traces ---
	traceExp, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	// --- Metrics ---
	metricExp, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp,
			sdkmetric.WithInterval(15*time.Second))),
	)
	otel.SetMeterProvider(mp)

	// Go runtime metrics (heap, goroutines, GC).
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		return nil, err
	}

	// --- Logs ---
	logExp, err := otlploggrpc.New(ctx)
	if err != nil {
		return nil, err
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
	)
	global.SetLoggerProvider(lp)

	shutdown := func(ctx context.Context) error {
		return errors.Join(
			tp.Shutdown(ctx),
			mp.Shutdown(ctx),
			lp.Shutdown(ctx),
		)
	}
	return shutdown, nil
}
```

---

## 3. Call it from `main`

```go
func main() {
	ctx := context.Background()

	shutdown, err := telemetry.Init(ctx)
	if err != nil {
		log.Fatalf("telemetry init: %v", err)
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdown(sctx)
	}()

	// Wrap your HTTP handler so every request is traced + measured.
	// otelhttp emits http.server.request.duration (histogram) used by the
	// Grafana dashboard and MCP queries.
	handler := otelhttp.NewHandler(mux, "http.server")

	srv := &http.Server{Addr: ":8080", Handler: handler}
	log.Fatal(srv.ListenAndServe())
}
```

For an outbound HTTP client that propagates trace context:

```go
client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
```

---

## 4. Environment variables (the only config)

Set these per service — for example in each service's `docker-compose.yml` or
systemd unit. `TELEMETRY_HOST` is the dedicated telemetry server.

```bash
# Which service this is (becomes the service_name label everywhere)
OTEL_SERVICE_NAME=provengo-pay
OTEL_RESOURCE_ATTRIBUTES=service.namespace=provengo,deployment.environment=production

# Where to send everything (gRPC OTLP on the telemetry host)
OTEL_EXPORTER_OTLP_ENDPOINT=http://TELEMETRY_HOST:4317
OTEL_EXPORTER_OTLP_PROTOCOL=grpc

# Optional: sample a fraction of traces under heavy load (1.0 = all)
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=1.0
```

> Use `grpc` on `4317`, or switch to `http/protobuf` on `4318` if gRPC is
> blocked by a proxy. The shared package honours whatever you set.

---

## 5. Emit custom spans, metrics and logs

```go
// Span around a unit of work
tr := otel.Tracer("provengo-pay/payment")
ctx, span := tr.Start(ctx, "charge_card")
defer span.End()
// ... on failure: span.RecordError(err); span.SetStatus(codes.Error, "charge failed")

// A business counter
meter := otel.Meter("provengo-pay")
charges, _ := meter.Int64Counter("payments.charges",
	metric.WithDescription("number of card charges"))
charges.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "ok")))
```

For logs, use the OTel `slog` bridge so your existing `slog` calls ship to Loki
**with the trace_id attached** (which powers the Grafana logs↔traces jump):

```go
import "go.opentelemetry.io/contrib/bridges/otelslog"

logger := otelslog.NewLogger("provengo-pay")
logger.InfoContext(ctx, "charge accepted", "amount", 1999, "currency", "BRL")
```

---

## 6. Verify

1. Start the service pointing at the telemetry host.
2. Send it some traffic.
3. Open Grafana → **Provengo / Provengo Services Overview**; your
   `service_name` should appear in the dropdown.
4. Or ask Claude (via the MCP server): *"list instrumented services"* →
   `prometheus_label_values(label=service_name)`.

If nothing shows up, see [DEPLOYMENT.md](DEPLOYMENT.md) → Troubleshooting.

---

See also: [ARCHITECTURE.md](ARCHITECTURE.md) ·
[PROVENGO_SERVICES.md](PROVENGO_SERVICES.md) · [MCP_USAGE.md](MCP_USAGE.md)
