package kql

import (
	"reflect"
	"testing"
)

// Normalization rewrites query syntax the grammar lacks. Text inside a string
// literal or a comment is not syntax: a literal keeps the value the rule
// compares against, and a comment cannot swallow the next line.
func TestNormalizationKeepsLiteralAndCommentText(t *testing.T) {
	for _, literal := range []string{
		"1<>1",
		"summarize by x",
		"imDns",
		"join hint.remote=left x",
		"union isfuzzy=true A, B",
		"union kind=outer A",
		"union *, A",
		"Table* | where x",
		"datatable(a:int)[1]",
		"declare query_parameters(x:int);",
		"T | parse x with * 'a' b | where c == 1",
		"(a, b) = f()",
		"top-nested 3 of x",
		"make-series x on y",
		"distinct a, tostring(b)",
		"evaluate f(x) : (a:int)",
		"arg('x').Resources",
		"make-graph T --> x",
		"lookup (union A, B) on x",
		"join (union A, B) on x",
		"T | graph-match (a)-[e]->(b) project a",
		"| partition by x (top 1 by y)",
		"| mv-apply x on (where y == 1)",
		"| render timechart kind=stacked",
	} {
		t.Run(literal, func(t *testing.T) {
			result := ExtractConditions(`SecurityEvent | where QueryText contains "` + literal + `"`)
			if len(result.Errors) != 0 || len(result.Conditions) != 1 {
				t.Fatalf("conditions=%+v errors=%q", result.Conditions, result.Errors)
			}
			if c := result.Conditions[0]; c.Value != literal || c.RHS == nil || c.RHS.Value != literal {
				t.Fatalf("value = %q, RHS = %+v, want %q", c.Value, c.RHS, literal)
			}
		})
	}

	type condition struct{ field, operator, value string }
	for _, tc := range []struct {
		name, query string
		want        []condition
	}{
		{"top-nested in a comment", "SecurityEvent | where x == 1 // top-nested 3 of x\n| where y == 2",
			[]condition{{"x", "==", "1"}, {"y", "==", "2"}}},
		{"make-series in a comment", "SecurityEvent | where x == 1 // make-series x on y\n| where y == 2",
			[]condition{{"x", "==", "1"}, {"y", "==", "2"}}},
		{"syntax beside a literal", `SecurityEvent | where QueryText contains "1<>1" and Count <> 2`,
			[]condition{{"QueryText", "contains", "1<>1"}, {"Count", "!=", "2"}}},
		{"summarize by beside a literal", `SecurityEvent | where QueryText contains "summarize by x" | summarize by Account`,
			[]condition{{"QueryText", "contains", "summarize by x"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractConditions(tc.query)
			var got []condition
			for _, c := range result.Conditions {
				got = append(got, condition{c.Field, c.Operator, c.Value})
			}
			if len(result.Errors) != 0 || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("conditions = %v errors=%q, want %v", got, result.Errors, tc.want)
			}
		})
	}
}
