package config

import (
	"testing"
)

func TestParseHashMetrics(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
		checks  func(t *testing.T, defs []HashMetricDef)
	}{
		{
			name:  "single definition with all fields",
			input: "redis_key=myapp:stats,metric=active_count,help=Active items per key,label=item",
			want:  1,
			checks: func(t *testing.T, defs []HashMetricDef) {
				t.Helper()
				d := defs[0]
				assertEqual(t, "RedisKey", d.RedisKey, "myapp:stats")
				assertEqual(t, "MetricName", d.MetricName, "active_count")
				assertEqual(t, "Help", d.Help, "Active items per key")
				assertEqual(t, "FieldLabel", d.FieldLabel, "item")
			},
		},
		{
			name:  "multiple definitions separated by semicolon",
			input: "redis_key=app:counters,metric=request_count,help=Requests,label=endpoint;redis_key=app:sessions,metric=session_count,help=Sessions,label=user",
			want:  2,
			checks: func(t *testing.T, defs []HashMetricDef) {
				t.Helper()
				assertEqual(t, "defs[0].RedisKey", defs[0].RedisKey, "app:counters")
				assertEqual(t, "defs[0].FieldLabel", defs[0].FieldLabel, "endpoint")
				assertEqual(t, "defs[1].RedisKey", defs[1].RedisKey, "app:sessions")
				assertEqual(t, "defs[1].FieldLabel", defs[1].FieldLabel, "user")
			},
		},
		{
			name:  "missing help gets default",
			input: "redis_key=app:counters,metric=request_count,label=endpoint",
			want:  1,
			checks: func(t *testing.T, defs []HashMetricDef) {
				t.Helper()
				if defs[0].Help == "" {
					t.Error("expected non-empty default help")
				}
			},
		},
		{
			name:    "missing redis_key returns error",
			input:   "metric=request_count,help=test,label=endpoint",
			wantErr: true,
		},
		{
			name:    "missing metric returns error",
			input:   "redis_key=app:counters,help=test,label=endpoint",
			wantErr: true,
		},
		{
			name:    "missing label returns error",
			input:   "redis_key=app:counters,metric=request_count,help=test",
			wantErr: true,
		},
		{
			name:  "empty string returns nil",
			input: "",
			want:  0,
		},
		{
			name:  "whitespace only returns nil",
			input: "   ",
			want:  0,
		},
		{
			name:  "trailing semicolons are ignored",
			input: "redis_key=app:counters,metric=request_count,label=endpoint;;;",
			want:  1,
		},
		{
			name:  "extra whitespace is trimmed",
			input: "  redis_key = app:counters , metric = request_count , help = Req count , label = endpoint  ",
			want:  1,
			checks: func(t *testing.T, defs []HashMetricDef) {
				t.Helper()
				assertEqual(t, "RedisKey", defs[0].RedisKey, "app:counters")
				assertEqual(t, "MetricName", defs[0].MetricName, "request_count")
				assertEqual(t, "FieldLabel", defs[0].FieldLabel, "endpoint")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defs, err := ParseHashMetrics(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(defs) != tt.want {
				t.Fatalf("expected %d defs, got %d", tt.want, len(defs))
			}
			if tt.checks != nil {
				tt.checks(t, defs)
			}
		})
	}
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: want %q, got %q", field, want, got)
	}
}
