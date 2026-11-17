// Package health periodically probes each application's health_url.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/willianpsouza/telemetry-mcp/registry/internal/store"
)

type Poller struct {
	st       *store.Store
	interval time.Duration
	client   *http.Client
}

func New(st *store.Store, interval time.Duration) *Poller {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Poller{st: st, interval: interval, client: &http.Client{Timeout: 5 * time.Second}}
}

// Run blocks, probing on each tick until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	p.probeAll(ctx) // once at startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.probeAll(ctx)
		}
	}
}

func (p *Poller) probeAll(ctx context.Context) {
	apps, err := p.st.ListApps()
	if err != nil {
		return
	}
	for _, app := range apps {
		if app.HealthURL == "" || !app.Active {
			continue
		}
		p.probe(ctx, app.ID, app.HealthURL)
	}
}

func (p *Poller) probe(ctx context.Context, id int64, url string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		p.st.UpdateHealth(id, "down", 0, 0)
		return
	}
	start := time.Now()
	resp, err := p.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		p.st.UpdateHealth(id, "down", 0, latency)
		return
	}
	defer resp.Body.Close()
	status := "down"
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		status = "up"
	}
	p.st.UpdateHealth(id, status, resp.StatusCode, latency)
}
