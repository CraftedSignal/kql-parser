package kql

import "testing"

func TestKQLStringOperatorAliasScannerCompletionEdges(t *testing.T) {
	input := `T | where Message !containscs "blocked" and Details containscs @"literal containscs" and fieldcontainscs == "unchanged"`
	want := `T | where Message !contains_cs "blocked" and Details contains_cs @"literal containscs" and fieldcontainscs == "unchanged"`
	if got := normalizeStringOperatorAliases(input); got != want {
		t.Fatalf("normalizeStringOperatorAliases() = %q, want %q", got, want)
	}
	escaped := `T | where Message == "escaped \" containscs" and Details containscs "blocked"`
	escapedWant := `T | where Message == "escaped \" containscs" and Details contains_cs "blocked"`
	if got := normalizeStringOperatorAliases(escaped); got != escapedWant {
		t.Fatalf("escaped string alias normalization = %q, want %q", got, escapedWant)
	}
	if got := normalizeStringOperatorAliases(`T | where Message contains "blocked"`); got != `T | where Message contains "blocked"` {
		t.Fatalf("query without aliases changed to %q", got)
	}
	if hasOperatorTokenAt(`fieldcontainscs`, 5, "containscs") {
		t.Fatalf("operator alias matched inside identifier")
	}
	if hasOperatorTokenAt(`containscsSuffix`, 0, "containscs") {
		t.Fatalf("operator alias matched before identifier suffix")
	}
	if hasOperatorTokenAt(`contains`, 0, "containscs") {
		t.Fatalf("short token matched as containscs")
	}
}

func TestKQLIndexAndKeywordScannerCompletionEdges(t *testing.T) {
	if got := indexKeyword(`mywhere where`, "where", 0); got != len(`mywhere `) {
		t.Fatalf("indexKeyword skipped-token result = %d", got)
	}
	if got := indexKeywordOutsideStrings(`mywhere where`, "where", 0); got != len(`mywhere `) {
		t.Fatalf("indexKeywordOutsideStrings skipped-token result = %d", got)
	}
	if got := indexKeywordOutsideStrings(`@"where""still string" where`, "where", 0); got != len(`@"where""still string" `) {
		t.Fatalf("indexKeywordOutsideStrings verbatim result = %d", got)
	}
	if got := indexOutsideStrings(`abc`, "", 2, false); got != 2 {
		t.Fatalf("empty needle index = %d", got)
	}
	if got := indexOutsideStrings(`abc`, "b", -10, false); got != 1 {
		t.Fatalf("negative start index = %d", got)
	}
	if got := indexOutsideStrings(`@"needle""still string" needle`, "needle", 0, false); got != len(`@"needle""still string" `) {
		t.Fatalf("indexOutsideStrings verbatim result = %d", got)
	}
}

func TestKQLLiteralNormalizationCompletionEdges(t *testing.T) {
	decimal := normalizeTrailingDecimals(`T | where A == @"1.""2." and B == 5.`)
	if want := `T | where A == @"1.""2." and B == 5.0`; decimal != want {
		t.Fatalf("normalizeTrailingDecimals() = %q, want %q", decimal, want)
	}

	nulls := normalizeNullEscapes(`T | where A == @"a""\0" and B == @'b''\0' and C == "\0"`)
	if want := `T | where A == @"a""\0" and B == @'b''\0' and C == "\x00"`; nulls != want {
		t.Fatalf("normalizeNullEscapes() = %q, want %q", nulls, want)
	}
}

func TestKQLDistinctColumnNormalizationCompletionEdges(t *testing.T) {
	input := `Alias = tostring(Device.Name), Label = strcat("a\",b", Account), Broken = , tostring((Nested.Field)), "literal\"column"`
	want := `Name, strcat("a\",b", Account), _distinct_col_, Field, "literal\"column"`
	if got := normalizeDistinctColumnList(input); got != want {
		t.Fatalf("normalizeDistinctColumnList() = %q, want %q", got, want)
	}

	if looksLikeTupleUnpacking("A, , B") {
		t.Fatalf("tuple unpacking with empty element was accepted")
	}
	if isValidPropertyKey("") {
		t.Fatalf("empty property key should be invalid")
	}
	if got := sanitizeIdentifier("DataVolume(Bytes)"); got != "_DataVolume_Bytes__" {
		t.Fatalf("sanitizeIdentifier() = %q", got)
	}
}

func TestKQLArgFunctionBoundaryCompletionEdges(t *testing.T) {
	input := `myarg("sub").Resources | union arg("sub").Resources | union arg("sub") | union arg(format("tenant-{0}", "a\"b")).Resources | union arg("unterminated"`
	want := `myarg("sub").Resources | union Resources | union ArgResult | union Resources | union arg("unterminated"`
	if got := replaceArgFunction(input); got != want {
		t.Fatalf("replaceArgFunction() = %q, want %q", got, want)
	}
}
