package clients

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Loki wraps the Loki query HTTP API.
type Loki struct {
	base string
	hc   *http.Client
}

func NewLoki(base string, timeout time.Duration) *Loki {
	return &Loki{base: base, hc: newHTTPClient(timeout)}
}

// QueryRange runs a LogQL query over a time window.
// direction is "backward" (newest first) or "forward".
func (l *Loki) QueryRange(ctx context.Context, logql string, lookback time.Duration, limit int, direction string) (map[string]any, error) {
	now := time.Now()
	start := now.Add(-lookback)
	if limit <= 0 {
		limit = 100
	}
	if direction == "" {
		direction = "backward"
	}
	q := url.Values{
		"query":     {logql},
		"start":     {strconv.FormatInt(start.UnixNano(), 10)},
		"end":       {strconv.FormatInt(now.UnixNano(), 10)},
		"limit":     {strconv.Itoa(limit)},
		"direction": {direction},
	}
	var out map[string]any
	err := doJSON(ctx, l.hc, l.base, "/loki/api/v1/query_range", q, &out)
	return out, err
}

// Labels lists label names available in Loki.
func (l *Loki) Labels(ctx context.Context, lookback time.Duration) (map[string]any, error) {
	now := time.Now()
	q := url.Values{
		"start": {strconv.FormatInt(now.Add(-lookback).UnixNano(), 10)},
		"end":   {strconv.FormatInt(now.UnixNano(), 10)},
	}
	var out map[string]any
	err := doJSON(ctx, l.hc, l.base, "/loki/api/v1/labels", q, &out)
	return out, err
}

// LabelValues lists values for a given label.
func (l *Loki) LabelValues(ctx context.Context, label string, lookback time.Duration) (map[string]any, error) {
	now := time.Now()
	q := url.Values{
		"start": {strconv.FormatInt(now.Add(-lookback).UnixNano(), 10)},
		"end":   {strconv.FormatInt(now.UnixNano(), 10)},
	}
	var out map[string]any
	err := doJSON(ctx, l.hc, l.base, "/loki/api/v1/label/"+url.PathEscape(label)+"/values", q, &out)
	return out, err
}
