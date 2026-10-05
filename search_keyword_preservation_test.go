package kql

import "testing"

func TestKQLSearchKeywordPreservedWithWherePredicate(t *testing.T) {
	result := ExtractConditions(`search "needle" | where EventID == 4688`)

	requireKQLCondition(t, result.Conditions, "_keyword_", "contains", "needle", false)
	requireKQLCondition(t, result.Conditions, "EventID", "==", "4688", false)
	if !containsTestString(result.Errors, portableKeywordExtractionNote) {
		t.Fatalf("expected keyword extraction note, got %v", result.Errors)
	}
}

func TestKQLSearchInKeywordPreservedWithWherePredicate(t *testing.T) {
	result := ExtractConditions(`search kind=case_sensitive in (OfficeActivity, SigninLogs) "needle" | where Operation == "FileDownloaded"`)

	requireKQLCondition(t, result.Conditions, "_keyword_", "contains", "needle", false)
	requireKQLCondition(t, result.Conditions, "Operation", "==", "FileDownloaded", false)
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

func containsTestString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
