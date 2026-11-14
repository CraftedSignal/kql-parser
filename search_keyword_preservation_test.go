package kql

import "testing"

func TestKQLSearchKeywordPreservedWithWherePredicate(t *testing.T) {
	result := ExtractConditions(`search "needle" | where EventID == 4688`)

	requireKQLCondition(t, result.Conditions, "EventID", "==", "4688", false)
	if leaves := searchLeaves(result.Expression); len(result.Errors) != 0 || len(leaves) != 1 || leaves[0].Text != `"needle"` || leaves[0].Scope != "database" {
		t.Fatalf("complete search missing: %#v", result)
	}
}

func TestKQLSearchInKeywordPreservedWithWherePredicate(t *testing.T) {
	result := ExtractConditions(`search kind=case_sensitive in (OfficeActivity, SigninLogs) "needle" | where Operation == "FileDownloaded"`)

	requireKQLCondition(t, result.Conditions, "Operation", "==", "FileDownloaded", false)
	if leaves := searchLeaves(result.Expression); len(result.Errors) != 0 || len(leaves) != 1 || leaves[0].Text != `"needle"` || leaves[0].Kind != "case_sensitive" || leaves[0].Scope != "tables" || len(leaves[0].TableSources) != 2 {
		t.Fatalf("complete scoped search missing: %#v", result)
	}
}

func TestKQLWhereStringLiteralDoesNotBecomeKeywordWhenPredicateExists(t *testing.T) {
	result := ExtractConditions(`SecurityEvent | where Account == "alice"`)

	requireKQLCondition(t, result.Conditions, "Account", "==", "alice", false)
	for _, condition := range result.Conditions {
		if condition.Field == "_keyword_" {
			t.Fatalf("unexpected keyword condition from where literal: %#v", result.Conditions)
		}
	}
}

func TestKQLLeadingSearchKeywordExtractorEdges(t *testing.T) {
	cases := map[string]string{
		`search "needle" | where EventID == 4688`:                      "needle",
		`search kind=default "needle"`:                                 "needle",
		`search in (OfficeActivity, cluster("c").database("d").T) "n"`: "n",
		`searching "needle"`:                                           "",
		`T | search "needle"`:                                          "",
		`search "https://example.invalid/ioc"`:                         "",
	}
	for query, want := range cases {
		if got := extractLeadingSearchKeyword(query); got != want {
			t.Fatalf("extractLeadingSearchKeyword(%q) = %q, want %q", query, got, want)
		}
	}
	for _, query := range []string{
		`search`,
		`search kind=default`,
		`search in OfficeActivity "needle"`,
		`search in (OfficeActivity "needle"`,
		`search in (OfficeActivity) | where EventID == 4688`,
	} {
		if got := extractLeadingSearchKeyword(query); got != "" {
			t.Fatalf("extractLeadingSearchKeyword(%q) = %q, want empty", query, got)
		}
	}
}
