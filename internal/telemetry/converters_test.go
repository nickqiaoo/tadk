package telemetry

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestConvertersRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want any
	}{
		{
			name: "nil",
			val:  nil,
			want: nil,
		},
		{
			name: "string",
			val:  "hello",
			want: "hello",
		},
		{
			name: "bool",
			val:  true,
			want: true,
		},
		{
			name: "float64",
			val:  123.456,
			want: 123.456,
		},
		{
			name: "int to int64",
			val:  int(123),
			want: int64(123),
		},
		{
			name: "slice of mixed types",
			val:  []any{1.0, true, "foo"},
			want: []any{1.0, true, "foo"},
		},
		{
			name: "map",
			val: map[string]any{
				"foo": "bar",
				"baz": 123.0,
			},
			want: map[string]any{
				"foo": "bar",
				"baz": 123.0,
			},
		},
		{
			name: "nested structure",
			val: map[string]any{
				"list": []any{
					map[string]any{"a": 1.0},
				},
			},
			want: map[string]any{
				"list": []any{
					map[string]any{"a": 1.0},
				},
			},
		},
		{
			name: "fallback for unsupported type",
			val:  struct{ A int }{A: 1},
			want: "{1}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Convert to log.Value
			val := toLogValue(tc.val)
			// Convert back to any
			got := FromLogValue(val)

			// Assert that result is the same as the expected want
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Round trip conversion mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
