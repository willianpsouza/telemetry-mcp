package clients

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Prometheus wraps the Prometheus HTTP query API.
type Prometheus struct {
	base string
	hc   *http.Client
}

func NewPrometheus(base string, timeout time.Duration) *Prometheus {
	return &Prometheus{base: base, hc: newHTTPClient(timeout)}
}

// Query runs an instant PromQL query. ts is optional (RFC3339 or unix); empty = now.
func (p *Prometheus) Query(ctx context.Context, promql, ts string) (map[string]any, error) {
	q := url.Values{"query": {promql}}
	if ts != "" {
		q.Set("time", ts)
	}
	var out map[string]any
	err := doJSON(ctx, p.hc, p.base, "/api/v1/query", q, &out)
	return out, err
}

// QueryRange runs a range PromQL query. start/end are RFC3339 or unix seconds;
// step is a duration string like "30s" or "1m".
func (p *Prometheus) QueryRange(ctx context.Context, promql, start, end, step string) (map[string]any, error) {
	q := url.Values{
		"query": {promql},
		"start": {start},
		"end":   {end},
		"step":  {step},
	}
	var out map[string]any
	err := doJSON(ctx, p.hc, p.base, "/api/v1/query_range", q, &out)
	return out, err
}

// Metrics returns the list of metric names known to Prometheus.
func (p *Prometheus) Metrics(ctx context.Context, match string) (map[string]any, error) {
	q := url.Values{}
	if match != "" {
		q.Set("match[]", match)
	}
	var out map[string]any
	err := doJSON(ctx, p.hc, p.base, "/api/v1/label/__name__/values", q, &out)
	return out, err
}

// Targets returns scrape target health (active/dropped).
func (p *Prometheus) Targets(ctx context.Context, state string) (map[string]any, error) {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	var out map[string]any
	err := doJSON(ctx, p.hc, p.base, "/api/v1/targets", q, &out)
	return out, err
}

// LabelValues returns the values for a label, optionally filtered by a selector.
func (p *Prometheus) LabelValues(ctx context.Context, label, match string) (map[string]any, error) {
	q := url.Values{}
	if match != "" {
		q.Set("match[]", match)
	}
	var out map[string]any
	err := doJSON(ctx, p.hc, p.base, "/api/v1/label/"+url.PathEscape(label)+"/values", q, &out)
	return out, err
}

// Helper so callers can build relative timestamps.
func UnixNow() string { return strconv.FormatInt(time.Now().Unix(), 10) }
