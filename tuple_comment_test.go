package kql

import (
	"strings"
	"testing"
)

func TestTupleUnpackingIgnoresParenthesesInBlockComments(t *testing.T) {
	baseline := ExtractConditions("T | extend (Low, High) = series_decompose_anomalies(Value)")
	if len(baseline.Errors) != 0 {
		t.Fatalf("uncommented tuple should parse: %v", baseline.Errors)
	}

	query := "T | extend (Low, /* ) */ High) = series_decompose_anomalies(Value)"
	got := normalizeQuery(query)
	result := ExtractConditions(query)
	if !strings.Contains(got, "extend _tuple_result =") {
		t.Fatalf("tuple assignment was not normalized: %q", got)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("tuple with parenthesis in a comment should parse: %v", result.Errors)
	}
}
