// Command telemetry-mcp is an MCP server that exposes the Provengo telemetry
// stack (Prometheus metrics, Jaeger traces, Loki logs) to MCP clients such as
// Claude. It speaks stdio by default, or Streamable HTTP when MCP_TRANSPORT=http.
package main

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/willianpsouza/telemetry-mcp/mcp-server/internal/clients"
	"github.com/willianpsouza/telemetry-mcp/mcp-server/internal/config"
	"github.com/willianpsouza/telemetry-mcp/mcp-server/internal/tools"
)

const version = "0.1.0"

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	deps := tools.Deps{
		Prom:            clients.NewPrometheus(cfg.PrometheusURL, cfg.HTTPTimeout),
		Jaeger:          clients.NewJaeger(cfg.JaegerURL, cfg.HTTPTimeout),
		Loki:            clients.NewLoki(cfg.LokiURL, cfg.HTTPTimeout),
		DefaultLookback: cfg.DefaultLookback,
	}

	newServer := func() *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{
			Name:    "provengo-telemetry",
			Version: version,
		}, nil)
		tools.Register(s, deps)
		return s
	}

	switch cfg.Transport {
	case "http":
		var handler http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return newServer()
		}, nil)
		if cfg.AuthToken != "" {
			handler = bearerAuth(cfg.AuthToken, handler)
			log.Printf("telemetry-mcp: Bearer auth enabled on /mcp")
		} else {
			log.Printf("telemetry-mcp: WARNING /mcp has no auth (set MCP_AUTH_TOKEN)")
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		mux.Handle("/mcp", handler)
		srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}
		go func() {
			<-ctx.Done()
			_ = srv.Close()
		}()
		log.Printf("telemetry-mcp %s listening (http) on %s/mcp", version, cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	default:
		log.Printf("telemetry-mcp %s starting (stdio)", version)
		if err := newServer().Run(ctx, &mcp.StdioTransport{}); err != nil {
			log.Fatalf("stdio server: %v", err)
		}
	}
}

// bearerAuth guards next with a constant-time Bearer token check.
func bearerAuth(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if len(got) != len(want) || subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
