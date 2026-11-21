// Package api serves the registry REST endpoints and session auth.
package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/willianpsouza/telemetry-mcp/registry/internal/store"
)

type API struct {
	st       *store.Store
	mu       sync.Mutex
	sessions map[string]session // cookie token -> session
	// Snippet hints shown to users on app creation.
	TelemetryHost string
	IngestPort    string
	IngestScheme  string
	IngestPath    string // path prefix for path-routed edges (e.g. "/ingest"); "" for host-routed
	// Collector OTLP/HTTP base for the "send test trace" action.
	CollectorOTLP string
	hc            *http.Client
}

type session struct {
	username string
	role     string
	expires  time.Time
}

func New(st *store.Store, telemetryHost, ingestPort, ingestScheme, ingestPath, collectorOTLP string) *API {
	if ingestScheme == "" {
		ingestScheme = "http"
	}
	// Normalize path: no trailing slash, leading slash if non-empty.
	if ingestPath != "" {
		ingestPath = "/" + strings.Trim(ingestPath, "/")
	}
	return &API{
		st: st, sessions: map[string]session{},
		TelemetryHost: telemetryHost, IngestPort: ingestPort, IngestScheme: ingestScheme,
		IngestPath:    ingestPath,
		CollectorOTLP: collectorOTLP,
		hc:            &http.Client{Timeout: 8 * time.Second},
	}
}

// Routes returns the API mux (mounted under /api by the caller).
func (a *API) Routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("POST /login", a.login)
	m.HandleFunc("POST /logout", a.logout)
	m.HandleFunc("GET /me", a.auth(a.me))
	m.HandleFunc("GET /applications", a.auth(a.listApps))
	m.HandleFunc("POST /applications", a.auth(a.createApp))
	m.HandleFunc("DELETE /applications/{id}", a.auth(a.deleteApp))
	m.HandleFunc("POST /applications/{id}/rotate", a.auth(a.rotateApp))
	m.HandleFunc("POST /applications/{id}/active", a.auth(a.setActive))
	m.HandleFunc("POST /applications/{id}/test", a.auth(a.testApp))
	m.HandleFunc("GET /users", a.auth(a.listUsers))
	m.HandleFunc("POST /users", a.auth(a.createUser))
	return m
}

// ---------------------------- auth ----------------------------

func (a *API) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("reg_session")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		a.mu.Lock()
		s, ok := a.sessions[c.Value]
		if ok && time.Now().After(s.expires) {
			delete(a.sessions, c.Value)
			ok = false
		}
		a.mu.Unlock()
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password string }
	if !decode(w, r, &in) {
		return
	}
	hash, role, ok := a.st.PasswordHash(in.Username)
	if !ok || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	tok := randToken()
	a.mu.Lock()
	a.sessions[tok] = session{username: in.Username, role: role, expires: time.Now().Add(12 * time.Hour)}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: "reg_session", Value: tok, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(12 * time.Hour),
	})
	writeJSON(w, map[string]any{"username": in.Username, "role": role})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("reg_session"); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "reg_session", Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("reg_session")
	a.mu.Lock()
	s := a.sessions[c.Value]
	a.mu.Unlock()
	writeJSON(w, map[string]any{"username": s.username, "role": s.role})
}

// ------------------------- applications ------------------------

func (a *API) listApps(w http.ResponseWriter, _ *http.Request) {
	apps, err := a.st.ListApps()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if apps == nil {
		apps = []store.Application{}
	}
	writeJSON(w, apps)
}

func (a *API) createApp(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name, ServiceName, Owner, HealthURL, Environment string
	}
	if !decode(w, r, &in) {
		return
	}
	app, token, err := a.st.CreateApp(in.Name, in.ServiceName, in.Owner, in.HealthURL, in.Environment)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Return the plaintext token ONCE, plus a ready-to-paste env snippet.
	writeJSON(w, map[string]any{
		"application": app,
		"token":       token,
		"snippet":     a.snippet(app.ServiceName, in.Environment, token),
	})
}

func (a *API) deleteApp(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := a.st.DeleteApp(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) rotateApp(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	token, err := a.st.RotateToken(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	app, _ := a.st.GetApp(id)
	writeJSON(w, map[string]any{"token": token, "snippet": a.snippet(app.ServiceName, app.Environment, token)})
}

func (a *API) setActive(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct{ Active bool }
	if !decode(w, r, &in) {
		return
	}
	if err := a.st.SetActive(id, in.Active); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testApp injects a minimal OTLP/HTTP trace for the app's service_name into the
// collector, so an operator can confirm the pipeline end-to-end with one click.
func (a *API) testApp(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	app, err := a.st.GetApp(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	payload := buildTestTrace(app.ServiceName)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		a.CollectorOTLP+"/v1/traces", bytes.NewReader(payload))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.hc.Do(req)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	okStatus := resp.StatusCode >= 200 && resp.StatusCode < 300
	writeJSON(w, map[string]any{
		"ok":      okStatus,
		"status":  resp.StatusCode,
		"message": fmt.Sprintf("test span sent for %q — check Jaeger (service %s)", app.ServiceName, app.ServiceName),
	})
}

func buildTestTrace(serviceName string) []byte {
	traceID := make([]byte, 16)
	spanID := make([]byte, 8)
	_, _ = rand.Read(traceID)
	_, _ = rand.Read(spanID)
	now := time.Now().UnixNano()
	start := now - int64(5*time.Millisecond)
	body := map[string]any{
		"resourceSpans": []any{map[string]any{
			"resource": map[string]any{"attributes": []any{
				attr("service.name", serviceName),
				attr("telemetry.source", "registry-test"),
			}},
			"scopeSpans": []any{map[string]any{
				"scope": map[string]any{"name": "provengo-registry"},
				"spans": []any{map[string]any{
					"traceId":           hex.EncodeToString(traceID),
					"spanId":            hex.EncodeToString(spanID),
					"name":              "registry.test",
					"kind":              1,
					"startTimeUnixNano": fmt.Sprintf("%d", start),
					"endTimeUnixNano":   fmt.Sprintf("%d", now),
				}},
			}},
		}},
	}
	b, _ := json.Marshal(body)
	return b
}

func attr(k, v string) map[string]any {
	return map[string]any{"key": k, "value": map[string]any{"stringValue": v}}
}

// ---------------------------- users ----------------------------

func (a *API) listUsers(w http.ResponseWriter, _ *http.Request) {
	us, err := a.st.ListUsers()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, us)
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password, Role string }
	if !decode(w, r, &in) {
		return
	}
	if in.Username == "" || len(in.Password) < 8 {
		http.Error(w, "username required and password min 8 chars", http.StatusBadRequest)
		return
	}
	if in.Role == "" {
		in.Role = "operator"
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	u, err := a.st.CreateUser(in.Username, string(hash), in.Role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, u)
}

// ------------------------- helpers ------------------------

func (a *API) snippet(serviceName, env, token string) string {
	if env == "" {
		env = "production"
	}
	host := a.TelemetryHost
	if !((a.IngestScheme == "http" && a.IngestPort == "80") ||
		(a.IngestScheme == "https" && a.IngestPort == "443") || a.IngestPort == "") {
		host += ":" + a.IngestPort
	}
	return "OTEL_SERVICE_NAME=" + serviceName + "\n" +
		"OTEL_RESOURCE_ATTRIBUTES=service.namespace=provengo,deployment.environment=" + env + "\n" +
		"OTEL_EXPORTER_OTLP_ENDPOINT=" + a.IngestScheme + "://" + host + a.IngestPath + "\n" +
		"OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf\n" +
		"OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer " + token
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func randToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
