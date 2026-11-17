// Command registry is the Provengo Observability Framework onboarding console:
// a self-contained Go service (embedded SPA + SQLite) to register applications,
// issue ingest tokens, probe their /health, and authenticate OTLP ingestion.
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/willianpsouza/telemetry-mcp/registry/internal/api"
	"github.com/willianpsouza/telemetry-mcp/registry/internal/health"
	"github.com/willianpsouza/telemetry-mcp/registry/internal/proxy"
	"github.com/willianpsouza/telemetry-mcp/registry/internal/store"
)

//go:embed web
var webFS embed.FS

func main() {
	logger := log.New(os.Stdout, "registry ", log.LstdFlags)

	dbPath := env("REGISTRY_DB_PATH", "/data/registry.db")
	addr := env("REGISTRY_ADDR", ":8082")
	proxyAddr := env("REGISTRY_OTLP_PROXY_ADDR", ":4319")
	collector := env("REGISTRY_COLLECTOR_OTLP", "http://otel-collector:4318")
	telemetryHost := env("REGISTRY_TELEMETRY_HOST", "localhost")
	ingestPort := env("REGISTRY_INGEST_PORT", "4319")
	ingestScheme := env("REGISTRY_INGEST_SCHEME", "http")
	healthEvery := envDur("REGISTRY_HEALTH_INTERVAL", 30*time.Second)

	st, err := store.Open(dbPath)
	if err != nil {
		logger.Fatalf("open store: %v", err)
	}
	defer st.Close()

	seedAdmin(st, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Health poller.
	go health.New(st, healthEvery).Run(ctx)

	// OTLP auth proxy (separate listener).
	px, err := proxy.New(st, collector, logger)
	if err != nil {
		logger.Fatalf("proxy: %v", err)
	}
	proxySrv := &http.Server{Addr: proxyAddr, Handler: px.Handler()}
	go func() {
		logger.Printf("OTLP auth proxy listening on %s -> %s", proxyAddr, collector)
		if err := proxySrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("proxy server: %v", err)
		}
	}()

	// Console + API.
	a := api.New(st, telemetryHost, ingestPort, ingestScheme, collector)
	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if st.Ping() != nil {
			http.Error(w, "db down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/api/", http.StripPrefix("/api", a.Routes()))
	mux.Handle("/", http.FileServer(http.FS(sub)))

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sc)
		_ = proxySrv.Shutdown(sc)
	}()

	logger.Printf("console listening on %s (db %s)", addr, dbPath)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("console server: %v", err)
	}
}

func seedAdmin(st *store.Store, logger *log.Logger) {
	user := env("REGISTRY_ADMIN_USER", "admin")
	pass := os.Getenv("REGISTRY_ADMIN_PASSWORD")
	if pass == "" {
		pass = "admin"
		logger.Printf("WARNING: REGISTRY_ADMIN_PASSWORD unset; seeding admin/admin — change it")
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	created, err := st.EnsureUser(user, string(hash), "admin")
	if err != nil {
		logger.Fatalf("seed admin: %v", err)
	}
	if created {
		logger.Printf("seeded admin user %q", user)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envDur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
