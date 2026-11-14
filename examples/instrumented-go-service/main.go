// A minimal, fully instrumented Go HTTP service. Run it against the telemetry
// stack to see metrics, traces and logs flow end-to-end.
//
//	OTEL_SERVICE_NAME=example-service \
//	OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
//	OTEL_EXPORTER_OTLP_PROTOCOL=grpc \
//	go run .
package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func main() {
	ctx := context.Background()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init: %v", err)
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdown(sctx)
	}()

	logger := otelslog.NewLogger("example-service")
	tracer := otel.Tracer("example-service")
	meter := otel.Meter("example-service")
	hits, _ := meter.Int64Counter("example.requests",
		metric.WithDescription("handled requests"))

	mux := http.NewServeMux()
	mux.HandleFunc("/work", func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "do_work")
		defer span.End()

		time.Sleep(time.Duration(rand.Intn(120)) * time.Millisecond)
		hits.Add(ctx, 1, metric.WithAttributes(attribute.String("route", "/work")))
		logger.InfoContext(ctx, "did some work", "route", "/work")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("done\n"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})

	handler := otelhttp.NewHandler(mux, "http.server")
	log.Println("example-service listening on :8090 (try GET /work)")
	if err := http.ListenAndServe(":8090", handler); err != nil {
		log.Fatal(err)
	}
}
