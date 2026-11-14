// Package tools registers the telemetry MCP tools on an MCP server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/willianpsouza/telemetry-mcp/mcp-server/internal/clients"
)

// Deps bundles the backend clients and defaults the tools need.
type Deps struct {
	Prom            *clients.Prometheus
	Jaeger          *clients.Jaeger
	Loki            *clients.Loki
	DefaultLookback time.Duration
}

type result = mcp.CallToolResultFor[any]

// Register wires every telemetry tool onto the server.
func Register(s *mcp.Server, d Deps) {
	// ---------------------------- Prometheus ----------------------------
	mcp.AddTool(s, &mcp.Tool{
		Name:        "prometheus_query",
		Description: "Run an instant PromQL query against Prometheus. Use for current values like 'up', request rate, or a gauge. Example: sum(rate(http_server_request_duration_seconds_count[5m])) by (service_name).",
	}, func(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[PromQueryIn]) (*result, error) {
		res, err := d.Prom.Query(ctx, p.Arguments.Query, p.Arguments.Time)
		return reply(res, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "prometheus_query_range",
		Description: "Run a PromQL range query returning a time series. start/end accept RFC3339 or unix seconds; step is a duration like '30s' or '1m'. If start/end omitted, the default lookback up to now is used.",
	}, func(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[PromRangeIn]) (*result, error) {
		in := p.Arguments
		start, end := in.Start, in.End
		if start == "" || end == "" {
			now := time.Now()
			start = fmt.Sprintf("%d", now.Add(-d.DefaultLookback).Unix())
			end = fmt.Sprintf("%d", now.Unix())
		}
		step := in.Step
		if step == "" {
			step = "30s"
		}
		res, err := d.Prom.QueryRange(ctx, in.Query, start, end, step)
		return reply(res, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "prometheus_list_metrics",
		Description: "List metric names known to Prometheus. Optional 'match' is a series selector, e.g. {service_name=\"provengo-pay\"}, to scope the list.",
	}, func(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[MatchIn]) (*result, error) {
		res, err := d.Prom.Metrics(ctx, p.Arguments.Match)
		return reply(res, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "service_health",
		Description: "Report Prometheus scrape target health (which collectors/services are up or down). Optional 'state' filters to 'active' or 'dropped'.",
	}, func(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[TargetsIn]) (*result, error) {
		res, err := d.Prom.Targets(ctx, p.Arguments.State)
		return reply(res, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "prometheus_label_values",
		Description: "List values for a Prometheus label (e.g. label 'service_name' to discover instrumented services). Optional 'match' scopes it with a selector.",
	}, func(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[LabelValuesIn]) (*result, error) {
		res, err := d.Prom.LabelValues(ctx, p.Arguments.Label, p.Arguments.Match)
		return reply(res, err)
	})

	// ------------------------------ Jaeger ------------------------------
	mcp.AddTool(s, &mcp.Tool{
		Name:        "jaeger_list_services",
		Description: "List services that have reported traces to Jaeger.",
	}, func(ctx context.Context, _ *mcp.ServerSession, _ *mcp.CallToolParamsFor[EmptyIn]) (*result, error) {
		res, err := d.Jaeger.Services(ctx)
		return reply(res, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "jaeger_list_operations",
		Description: "List operations (span names) reported by a given service in Jaeger.",
	}, func(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[ServiceIn]) (*result, error) {
		res, err := d.Jaeger.Operations(ctx, p.Arguments.Service)
		return reply(res, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "jaeger_search_traces",
		Description: "Search recent traces in Jaeger for a service. Filter by operation, tags (e.g. {\"error\":\"true\"}), and min/max duration (e.g. '200ms'). lookback is a duration like '1h'; defaults applied if omitted.",
	}, func(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[TraceSearchIn]) (*result, error) {
		in := p.Arguments
		lb := d.DefaultLookback
		if in.Lookback != "" {
			if v, err := time.ParseDuration(in.Lookback); err == nil {
				lb = v
			}
		}
		res, err := d.Jaeger.SearchTraces(ctx, clients.SearchParams{
			Service:     in.Service,
			Operation:   in.Operation,
			Tags:        in.Tags,
			Lookback:    lb,
			MinDuration: in.MinDuration,
			MaxDuration: in.MaxDuration,
			Limit:       in.Limit,
		})
		return reply(res, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "jaeger_get_trace",
		Description: "Fetch a single trace by its trace ID, including all spans. Use after jaeger_search_traces to drill into a specific trace.",
	}, func(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[TraceIDIn]) (*result, error) {
		res, err := d.Jaeger.Trace(ctx, p.Arguments.TraceID)
		return reply(res, err)
	})

	// ------------------------------- Loki -------------------------------
	mcp.AddTool(s, &mcp.Tool{
		Name:        "loki_query",
		Description: "Run a LogQL query against Loki over a time window. Example: {service_name=\"provengo-pay\"} |= \"error\". lookback is a duration like '30m'; limit caps returned lines; direction is 'backward' (newest first) or 'forward'.",
	}, func(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[LokiQueryIn]) (*result, error) {
		in := p.Arguments
		lb := d.DefaultLookback
		if in.Lookback != "" {
			if v, err := time.ParseDuration(in.Lookback); err == nil {
				lb = v
			}
		}
		res, err := d.Loki.QueryRange(ctx, in.Query, lb, in.Limit, in.Direction)
		return reply(res, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "loki_labels",
		Description: "List log label names available in Loki (useful to discover how to filter, e.g. service_name). lookback is a duration like '1h'.",
	}, func(ctx context.Context, _ *mcp.ServerSession, p *mcp.CallToolParamsFor[LookbackIn]) (*result, error) {
		lb := d.DefaultLookback
		if p.Arguments.Lookback != "" {
			if v, err := time.ParseDuration(p.Arguments.Lookback); err == nil {
				lb = v
			}
		}
		res, err := d.Loki.Labels(ctx, lb)
		return reply(res, err)
	})
}

// reply renders a backend result (or error) as a JSON text tool result.
func reply(v any, err error) (*result, error) {
	if err != nil {
		return &result{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "error: " + err.Error()}},
		}, nil
	}
	b, merr := json.MarshalIndent(v, "", "  ")
	if merr != nil {
		return &result{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "encode error: " + merr.Error()}},
		}, nil
	}
	return &result{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(b)}},
		StructuredContent: v,
	}, nil
}
