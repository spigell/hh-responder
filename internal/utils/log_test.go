package utils

import "testing"

func TestTruncateForLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		limit  int
		expect string
	}{
		{
			name:   "returns empty when limit non-positive",
			input:  "hello world",
			limit:  0,
			expect: "",
		},
		{
			name:   "shorter than limit",
			input:  "hello",
			limit:  10,
			expect: "hello",
		},
		{
			name:   "truncates and adds ellipsis",
			input:  "hello world",
			limit:  5,
			expect: "hello...",
		},
		{
			name:   "trims surrounding whitespace",
			input:  "  spaced  ",
			limit:  5,
			expect: "space...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := TruncateForLog(tt.input, tt.limit); got != tt.expect {
				t.Fatalf("expected %q, got %q", tt.expect, got)
			}
		})
	}
}
