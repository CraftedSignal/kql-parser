package kql

import (
	"testing"
)

func assertSameSources(t *testing.T, query string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("query %q\n DataSources = %v\n        want = %v", query, got, want)
	}
	seen := make(map[string]bool, len(got))
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Fatalf("query %q\n DataSources = %v\n     missing = %q", query, got, w)
		}
	}
}

// TestDataSourcesSingleSegmentAndJoin covers two symptoms of one defect:
// allowLeadingSource required more than one pipe segment, so any single-segment
// query yielded no data sources. That discarded bare table queries, and — since a
// parenthesised join operand recurses as its own single-segment query — it also
// discarded the right-hand table of every parenthesised join.
func TestDataSourcesSingleSegmentAndJoin(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"bare table", `SecurityEvent`, []string{"SecurityEvent"}},
		{"piped table", `SecurityEvent | where EventID == 4688`, []string{"SecurityEvent"}},
		{"union", `union SecurityEvent, DeviceProcessEvents`, []string{"SecurityEvent", "DeviceProcessEvents"}},
		{"join unparenthesized", `SecurityEvent | join kind=leftouter SigninLogs on Account`, []string{"SecurityEvent", "SigninLogs"}},
		{"join parenthesized", `SecurityEvent | join kind=inner (SigninLogs) on Account`, []string{"SecurityEvent", "SigninLogs"}},
		{"join parenthesized subquery", `SecurityEvent | join (DeviceProcessEvents | where FileName == "x") on DeviceId`, []string{"SecurityEvent", "DeviceProcessEvents"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSameSources(t, tt.query, ExtractConditions(tt.query).DataSources, tt.want)
		})
	}
}

// TestDataSourcesJoinShapes exercises join forms beyond the minimal repro: the
// operand may be filtered, aggregated, chained, or itself a union, and a query
// may join more than once.
func TestDataSourcesJoinShapes(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			"two sequential joins",
			`SecurityEvent | join (SigninLogs) on Account | join (DeviceProcessEvents) on DeviceId`,
			[]string{"SecurityEvent", "SigninLogs", "DeviceProcessEvents"},
		},
		{
			"operand with multiple inner pipes",
			`SecurityEvent | join (SigninLogs | where ResultType == 0 | project Account) on Account`,
			[]string{"SecurityEvent", "SigninLogs"},
		},
		{
			"operand is a union",
			`SecurityEvent | join (union SigninLogs, AuditLogs) on Account`,
			[]string{"SecurityEvent", "SigninLogs", "AuditLogs"},
		},
		{
			"join then where",
			`SecurityEvent | join (SigninLogs) on Account | where EventID == 4688`,
			[]string{"SecurityEvent", "SigninLogs"},
		},
		{
			"duplicate tables deduplicated",
			`SecurityEvent | join (SecurityEvent | where EventID == 1) on Account`,
			[]string{"SecurityEvent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSameSources(t, tt.query, ExtractConditions(tt.query).DataSources, tt.want)
		})
	}
}

// TestSplitTopLevelPipeSegments pins the splitter's contract directly rather than
// only through its observable effect on data sources. Data-source extraction needs
// nesting respected; predicate extraction deliberately does not, and the two must
// not be collapsed back into one function.
func TestSplitTopLevelPipeSegments(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"no pipes", `SecurityEvent`, 1},
		{"one top-level pipe", `SecurityEvent | where x == 1`, 2},
		{"pipe inside parens does not split", `T | join (U | where x == 1) on k`, 2},
		{"nested parens", `T | join (U | join (V | where x == 1) on j) on k`, 2},
		{"pipe inside double quotes", `T | where Msg == "a | b"`, 2},
		{"pipe inside single quotes", `T | where Msg == 'a | b'`, 2},
		{"pipe inside verbatim string", `T | where Msg == @"a | b"`, 2},
		{"unbalanced close paren does not underflow", `T | where x == 1) | project y`, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(splitTopLevelPipeSegments(tt.query)); got != tt.want {
				t.Fatalf("splitTopLevelPipeSegments(%q) produced %d segments, want %d\nsegments: %q",
					tt.query, got, tt.want, splitTopLevelPipeSegments(tt.query))
			}
		})
	}
}

// TestSplitPipeSegmentsStillSplitsInsideParens guards the regression that the
// first attempt at this fix caused: making the shared splitter paren-aware broke
// predicate extraction for parenthesised let-subqueries, silently dropping their
// where-clause conditions.
func TestSplitPipeSegmentsStillSplitsInsideParens(t *testing.T) {
	query := `let H = (T | where MD5 in (IOC) | project MD5);`
	if got := len(splitPipeSegments(query)); got < 3 {
		t.Fatalf("splitPipeSegments must split inside parens for predicate extraction; got %d segments: %q",
			got, splitPipeSegments(query))
	}
}
