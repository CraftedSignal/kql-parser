package kql

import (
	"strings"
	"testing"
)

func TestKQLNormalizationScannerAdditionalEdges(t *testing.T) {
	normalized := normalizeUnicodeWhitespace("SecurityEvent\uFEFF|\u2060where Message == @\"a\u00a0b\" and Other == \"x\\\"|y\"")
	if strings.ContainsRune(normalized, '\u2060') {
		t.Fatalf("word joiner outside strings was not removed: %q", normalized)
	}
	if !strings.Contains(normalized, "@\"a\u00a0b\"") || !strings.Contains(normalized, "\"x\\\"|y\"") {
		t.Fatalf("unicode/escaped string content not preserved: %q", normalized)
	}

	var b strings.Builder
	handleUnicodeEscape("0000", &b)
	if got := b.String(); got != "\x00" {
		t.Fatalf("control unicode escape = %q", got)
	}

	if containsPipeOutsideStrings(`@"a|""b"`) || containsPipeOutsideStrings(`"a\"|b"`) {
		t.Fatalf("pipe inside strings should not be detected")
	}
	if !containsPipeOutsideStrings(`@"a|""b" | project Account`) {
		t.Fatalf("pipe outside verbatim string was not detected")
	}
}

func TestKQLDistinctPartitionAndSourceScannerEdges(t *testing.T) {
	distinct := normalizeDistinctColumnList(`tostring(Account), strcat(First, Last), 'literal,still', bin(TimeGenerated, 1h)`)
	wantDistinct := `Account, strcat(First, Last), 'literal,still', bin(TimeGenerated, 1h)`
	if distinct != wantDistinct {
		t.Fatalf("normalizeDistinctColumnList() = %q, want %q", distinct, wantDistinct)
	}

	for _, query := range []string{
		`T | partition hint.strategy=native by Computer`,
		`T | partition Computer (where EventID == 4688)`,
		`T | partition by Computer where EventID == 4688`,
	} {
		if got := normalizePartitionOperator(query); got != query {
			t.Fatalf("incomplete/unsupported partition changed from %q to %q", query, got)
		}
	}

	letNames := map[string]bool{"derived": true}
	sources := extractParenthesizedSources(`search in (SecurityEvent, Derived, cluster("c").database("d").SigninLogs) "failed"`, "search in", letNames)
	if len(sources) != 2 || sources[0] != "SecurityEvent" || sources[1] != "SigninLogs" {
		t.Fatalf("parenthesized sources not cleaned/deduped: %#v", sources)
	}
	for _, statement := range []string{
		`search in SecurityEvent "failed"`,
		`search in (SecurityEvent "failed"`,
	} {
		if got := extractParenthesizedSources(statement, "search in", nil); got != nil {
			t.Fatalf("malformed parenthesized sources %q produced %#v", statement, got)
		}
	}
}

func TestKQLPortableKeywordAndDedupAdditionalEdges(t *testing.T) {
	if got := extractSearchKeyword(`search in (SecurityEvent, SigninLogs) @"failed ""login"""`); got != `failed "login"` {
		t.Fatalf("search keyword = %q", got)
	}
	if got := extractPortableKeyword(`let External = externaldata(User:string)["https://example.invalid/users.csv"]; External`); got != "" {
		t.Fatalf("externaldata let should not produce keyword: %q", got)
	}
	if got := extractPortableKeyword(`print "https://example.invalid/ioc"`); strings.Contains(got, "https://") {
		t.Fatalf("URL literal should not become keyword: %q", got)
	}

	for expr, want := range map[string]string{
		`(Account == "a") and (Other == "b")`: `(Account == "a") and (Other == "b")`,
		`((Account == "a")`:                   `((Account == "a")`,
		`(")")`:                               `")"`,
	} {
		if got := stripOuterParens(expr); got != want {
			t.Fatalf("stripOuterParens(%q) = %q, want %q", expr, got, want)
		}
	}

	deduped := DeduplicateConditions([]Condition{
		{Field: "Account", Operator: "==", Value: "*", PipeStage: 3},
		{Field: "Account", Operator: "==", Value: "alice", PipeStage: 1},
		{Field: "account", Operator: "==", Value: "alice", PipeStage: 2},
		{Field: "Account", Operator: "!=", Value: "alice", PipeStage: 2},
		{Field: "Account", Operator: "!=", Value: "alice", PipeStage: 2},
	})
	if len(deduped) != 2 {
		t.Fatalf("deduped conditions = %#v", deduped)
	}
	if !containsCondition(deduped, Condition{Field: "account", Operator: "==", Value: "alice", PipeStage: 2}) ||
		!containsCondition(deduped, Condition{Field: "Account", Operator: "!=", Value: "alice", PipeStage: 2}) {
		t.Fatalf("latest distinct conditions not retained: %#v", deduped)
	}
}
