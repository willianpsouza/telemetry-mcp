// Package proxy is an authenticating OTLP/HTTP reverse proxy. It validates the
// Bearer token against the registry, stamps last_seen, and forwards accepted
// traffic to the real OTel Collector OTLP/HTTP endpoint.
//
// Enforcement model: in authenticated mode, apps send OTLP/HTTP to THIS proxy
// (not directly to the collector). The collector's raw 4317/4318 must be kept
// off the app network (firewall) so every ingest passes a token here.
package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/willianpsouza/telemetry-mcp/registry/internal/store"
)

type Proxy struct {
	st  *store.Store
	rp  *httputil.ReverseProxy
	log *log.Logger
}

// New builds a proxy forwarding to collectorBase (e.g. http://otel-collector:4318).
func New(st *store.Store, collectorBase string, logger *log.Logger) (*Proxy, error) {
	target, err := url.Parse(collectorBase)
	if err != nil {
		return nil, err
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	return &Proxy{st: st, rp: rp, log: logger}, nil
}

func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		token := bearer(r.Header.Get("Authorization"))
		if token == "" {
			unauthorized(w, "missing bearer token")
			return
		}
		app, ok := p.st.AppByToken(token)
		if !ok {
			unauthorized(w, "invalid or revoked token")
			return
		}
		p.st.TouchLastSeen(app.ID)
		// Strip the inbound auth header before forwarding to the collector.
		r.Header.Del("Authorization")
		p.rp.ServeHTTP(w, r)
	})
}

func bearer(h string) string {
	const pfx = "Bearer "
	if strings.HasPrefix(h, pfx) {
		return strings.TrimSpace(h[len(pfx):])
	}
	return ""
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, msg, http.StatusUnauthorized)
}
