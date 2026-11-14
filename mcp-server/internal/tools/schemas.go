package tools

// Input structs for the tools. The `jsonschema` tag descriptions are surfaced
// to the model as the parameter documentation.

type EmptyIn struct{}

type PromQueryIn struct {
	Query string `json:"query" jsonschema:"the PromQL expression to evaluate"`
	Time  string `json:"time,omitempty" jsonschema:"optional evaluation timestamp (RFC3339 or unix seconds); defaults to now"`
}

type PromRangeIn struct {
	Query string `json:"query" jsonschema:"the PromQL expression to evaluate"`
	Start string `json:"start,omitempty" jsonschema:"range start (RFC3339 or unix seconds); defaults to now minus the default lookback"`
	End   string `json:"end,omitempty" jsonschema:"range end (RFC3339 or unix seconds); defaults to now"`
	Step  string `json:"step,omitempty" jsonschema:"resolution step, e.g. '30s' or '1m'; defaults to 30s"`
}

type MatchIn struct {
	Match string `json:"match,omitempty" jsonschema:"optional series selector to scope results, e.g. {service_name=\"provengo-pay\"}"`
}

type TargetsIn struct {
	State string `json:"state,omitempty" jsonschema:"optional target state filter: 'active' or 'dropped'"`
}

type LabelValuesIn struct {
	Label string `json:"label" jsonschema:"the label name to list values for, e.g. 'service_name'"`
	Match string `json:"match,omitempty" jsonschema:"optional series selector to scope results"`
}

type ServiceIn struct {
	Service string `json:"service" jsonschema:"the service name as reported to Jaeger"`
}

type TraceSearchIn struct {
	Service     string `json:"service" jsonschema:"service name to search traces for"`
	Operation   string `json:"operation,omitempty" jsonschema:"optional operation/span name filter"`
	Tags        string `json:"tags,omitempty" jsonschema:"optional tag filter as JSON, e.g. {\"error\":\"true\"}"`
	Lookback    string `json:"lookback,omitempty" jsonschema:"how far back to search, e.g. '1h'"`
	MinDuration string `json:"min_duration,omitempty" jsonschema:"only traces at least this long, e.g. '200ms'"`
	MaxDuration string `json:"max_duration,omitempty" jsonschema:"only traces at most this long, e.g. '5s'"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum number of traces to return (default 20)"`
}

type TraceIDIn struct {
	TraceID string `json:"trace_id" jsonschema:"the trace ID to fetch"`
}

type LokiQueryIn struct {
	Query     string `json:"query" jsonschema:"the LogQL query, e.g. {service_name=\"provengo-pay\"} |= \"error\""`
	Lookback  string `json:"lookback,omitempty" jsonschema:"how far back to query, e.g. '30m'"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum log lines to return (default 100)"`
	Direction string `json:"direction,omitempty" jsonschema:"'backward' (newest first, default) or 'forward'"`
}

type LookbackIn struct {
	Lookback string `json:"lookback,omitempty" jsonschema:"how far back to look, e.g. '1h'"`
}
