package kql

import (
	"strings"
	"testing"
)

func TestKQLNormalizeLookupSubqueryWithKindOption(t *testing.T) {
	input := `T | lookup kind=leftouter   (union WatchlistA, WatchlistB) on Id | where Account == "alice"`
	want := `T | lookup kind=leftouter   (LookupTable) on Id | where Account == "alice"`

	if got := normalizeLookupSubquery(input); got != want {
		t.Fatalf("normalizeLookupSubquery() = %q, want %q", got, want)
	}
}

func TestKQLNormalizeLookupSubqueryWithNestedUnionExpression(t *testing.T) {
	input := `T | lookup kind=inner (union (WatchlistA | project Id), WatchlistB) on Id`
	want := `T | lookup kind=inner (LookupTable) on Id`

	if got := normalizeLookupSubquery(input); got != want {
		t.Fatalf("normalizeLookupSubquery() = %q, want %q", got, want)
	}
}

func TestKQLNormalizeLookupSubqueryRequiresLookupOperator(t *testing.T) {
	input := `T | extend mylookup = "(union A, B)" | where mylookup contains "union"`

	if got := normalizeLookupSubquery(input); got != input {
		t.Fatalf("identifier/string containing lookup changed to %q", got)
	}
}

func TestKQLNormalizeLookupSubqueryLeavesMalformedKindLookupIntact(t *testing.T) {
	input := `T | lookup kind=leftouter (union WatchlistA, WatchlistB on Id | where Account == "alice"`

	if got := normalizeLookupSubquery(input); got != input {
		t.Fatalf("malformed lookup changed to %q", got)
	}
}

func TestKQLNormalizeLookupSubqueryLeavesLookupTablesIntact(t *testing.T) {
	input := `T | lookup kind=leftouter Watchlist on Id | where Account == "alice"`

	if got := normalizeLookupSubquery(input); got != input {
		t.Fatalf("lookup table reference changed to %q", got)
	}
}

func TestKQLNormalizeLookupSubqueryNormalizesThroughFullQuery(t *testing.T) {
	input := `T | lookup kind=inner ( union WatchlistA, WatchlistB ) on Id | where Account == "alice"`
	got := normalizeQuery(input)

	if !strings.Contains(got, `(LookupTable)`) || strings.Contains(got, `union WatchlistA`) {
		t.Fatalf("normalizeQuery() did not normalize lookup union: %q", got)
	}
}
