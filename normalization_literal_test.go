package kql

import (
	"reflect"
	"strings"
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
		{"numeric identifier text in a backtick literal", "SecurityEvent | where QueryText == ```3plogTime```",
			[]condition{{"QueryText", "==", "```3plogTime```"}}},
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

func TestNormalizeQueryForDebugPreservesDatetimeLiteralAndCommentText(t *testing.T) {
	query := `T | where Message == "datetime(2026-02-04 10:00:00)" // datetime(2026-02-04 10:00:00)
| where TimeGenerated > datetime(2026-02-04 10:00:00)`
	got := NormalizeQueryForDebug(query)
	if !strings.Contains(got, `Message == "datetime(2026-02-04 10:00:00)"`) {
		t.Fatalf("date-time-looking string literal changed: %q", got)
	}
	if !strings.Contains(got, "// datetime(2026-02-04 10:00:00)") {
		t.Fatalf("date-time-looking comment changed: %q", got)
	}
	if !strings.Contains(got, "TimeGenerated > datetime(2026-02-04T10:00:00)") {
		t.Fatalf("date-time syntax outside literal was not normalized: %q", got)
	}
}

func TestFixNumericIdentifiersPreservesEscapedSingleQuotedLiteral(t *testing.T) {
	query := `SecurityEvent | where QueryText == 'can\'t 3plogTime'`
	if got := fixNumericIdentifiers(query); got != query {
		t.Fatalf("numeric identifier normalization changed a quoted value: got %q, want %q", got, query)
	}

	result := ExtractConditions(query)
	if len(result.Errors) != 0 || len(result.Conditions) != 1 {
		t.Fatalf("conditions=%+v errors=%q", result.Conditions, result.Errors)
	}
	condition := result.Conditions[0]
	if got := condition.Value; got != `can\'t 3plogTime` {
		t.Fatalf("condition value = %q, want raw literal contents %q", got, `can\'t 3plogTime`)
	}
	if condition.RHS == nil || condition.RHS.Value != "can't 3plogTime" {
		t.Fatalf("condition RHS = %+v, want decoded value %q", condition.RHS, "can't 3plogTime")
	}
}

func TestConvertBracketAccessPreservesEscapedSingleQuotedLiteral(t *testing.T) {
	query := `SecurityEvent | where QueryText == 'can\'t item[0]'`
	if got := convertBracketAccess(query); got != query {
		t.Fatalf("bracket normalization changed a quoted value: got %q, want %q", got, query)
	}
	result := ExtractConditions(query)
	if len(result.Errors) != 0 || len(result.Conditions) != 1 {
		t.Fatalf("conditions=%+v errors=%q", result.Conditions, result.Errors)
	}
	if result.Conditions[0].RHS == nil || result.Conditions[0].RHS.Value != "can't item[0]" {
		t.Fatalf("condition RHS = %+v, want decoded value %q", result.Conditions[0].RHS, "can't item[0]")
	}
}

func TestSkipLetStatementHandlesEscapedSingleQuotedLiteral(t *testing.T) {
	query := `let ignored = 'can\'t'; SecurityEvent | where EventID == 1`
	want := strings.IndexByte(query, ';') + 1
	if got := skipLetStatement(query, 0); got != want {
		t.Fatalf("skipLetStatement() = %d, want semicolon boundary %d", got, want)
	}
	result := ExtractConditions(query)
	if len(result.Errors) != 0 || len(result.Conditions) != 1 || result.Conditions[0].Field != "EventID" {
		t.Fatalf("conditions=%+v errors=%q", result.Conditions, result.Errors)
	}
}

func TestNormalizeUnicodeWhitespacePreservesEscapedSingleQuotedLiteral(t *testing.T) {
	query := "SecurityEvent | where QueryText == 'can\\'t\u00a0value'"
	if got := normalizeUnicodeWhitespace(query); got != query {
		t.Fatalf("Unicode whitespace normalization changed a quoted value: got %q, want %q", got, query)
	}
	result := ExtractConditions(query)
	if len(result.Errors) != 0 || len(result.Conditions) != 1 || result.Conditions[0].RHS == nil {
		t.Fatalf("conditions=%+v errors=%q", result.Conditions, result.Errors)
	}
	if got := result.Conditions[0].RHS.Value; got != "can't\u00a0value" {
		t.Fatalf("condition RHS value = %q, want escaped apostrophe and NBSP preserved", got)
	}
}

func TestNormalizeTrailingDecimalsPreservesEscapedSingleQuotedLiteral(t *testing.T) {
	query := `SecurityEvent | where QueryText == 'can\'t 1000. value'`
	if got := normalizeTrailingDecimals(query); got != query {
		t.Fatalf("decimal normalization changed a quoted value: got %q, want %q", got, query)
	}
	result := ExtractConditions(query)
	if len(result.Errors) != 0 || len(result.Conditions) != 1 || result.Conditions[0].RHS == nil {
		t.Fatalf("conditions=%+v errors=%q", result.Conditions, result.Errors)
	}
	if got := result.Conditions[0].RHS.Value; got != "can't 1000. value" {
		t.Fatalf("condition RHS value = %q, want %q", got, "can't 1000. value")
	}
}

func TestStripMvApplySubqueryHandlesEscapedSingleQuotedLiteral(t *testing.T) {
	query := `T | mv-apply x on (where QueryText == 'can\')') | where EventID == 1`
	got := stripMvApplySubquery(query)
	want := "T  | where EventID == 1"
	if got != want {
		t.Fatalf("stripMvApplySubquery() = %q, want %q", got, want)
	}
}

func TestStripMvApplySubqueryIgnoresParenthesesInComments(t *testing.T) {
	queries := map[string]string{
		"line":  "T | mv-apply x on (where QueryText == \"a\" // )\n) | where EventID == 1",
		"block": `T | mv-apply x on (where QueryText == "a" /* ) */) | where EventID == 1`,
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			got := stripMvApplySubquery(query)
			want := "T  | where EventID == 1"
			if got != want {
				t.Fatalf("stripMvApplySubquery() = %q, want %q", got, want)
			}

			result := ExtractConditions(query)
			if len(result.Errors) != 0 || len(result.Conditions) != 1 || result.Conditions[0].Field != "EventID" {
				t.Fatalf("conditions=%+v errors=%q", result.Conditions, result.Errors)
			}
		})
	}
}
