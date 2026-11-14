package clients

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Jaeger wraps the Jaeger query HTTP API (served by jaeger-query / all-in-one).
type Jaeger struct {
	base string
	hc   *http.Client
}

func NewJaeger(base string, timeout time.Duration) *Jaeger {
	return &Jaeger{base: base, hc: newHTTPClient(timeout)}
}

// Services lists service names that have reported traces.
func (j *Jaeger) Services(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := doJSON(ctx, j.hc, j.base, "/api/services", nil, &out)
	return out, err
}

// Operations lists operations (span names) for a service.
func (j *Jaeger) Operations(ctx context.Context, service string) (map[string]any, error) {
	q := url.Values{"service": {service}}
	var out map[string]any
	err := doJSON(ctx, j.hc, j.base, "/api/operations", q, &out)
	return out, err
}

// SearchParams controls trace search.
type SearchParams struct {
	Service     string
	Operation   string
	Tags        string // Jaeger logfmt/JSON tag filter, e.g. {"error":"true"}
	Lookback    time.Duration
	MinDuration string // e.g. "100ms"
	MaxDuration string
	Limit       int
}

// SearchTraces finds traces matching the given parameters.
func (j *Jaeger) SearchTraces(ctx context.Context, p SearchParams) (map[string]any, error) {
	now := time.Now()
	start := now.Add(-p.Lookback)
	q := url.Values{
		"service": {p.Service},
		"start":   {strconv.FormatInt(start.UnixMicro(), 10)},
		"end":     {strconv.FormatInt(now.UnixMicro(), 10)},
	}
	if p.Operation != "" {
		q.Set("operation", p.Operation)
	}
	if p.Tags != "" {
		q.Set("tags", p.Tags)
	}
	if p.MinDuration != "" {
		q.Set("minDuration", p.MinDuration)
	}
	if p.MaxDuration != "" {
		q.Set("maxDuration", p.MaxDuration)
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}
	q.Set("limit", strconv.Itoa(limit))

	var out map[string]any
	err := doJSON(ctx, j.hc, j.base, "/api/traces", q, &out)
	return out, err
}

// Trace fetches a single trace by ID.
func (j *Jaeger) Trace(ctx context.Context, traceID string) (map[string]any, error) {
	var out map[string]any
	err := doJSON(ctx, j.hc, j.base, "/api/traces/"+url.PathEscape(traceID), nil, &out)
	return out, err
}
