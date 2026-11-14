package kql

import "testing"

func assertSearchFailureOnly(t *testing.T, query string) {
	t.Helper()
	r := ExtractConditions(query)
	if len(r.Errors) == 0 || len(r.Conditions) != 0 || r.Expression != nil || len(r.Joins) != 0 || len(r.Pipelines) != 0 {
		t.Errorf("extraction kept a partial search: %#v", r)
	}
	s := AnalyzeQuery(query)
	if len(s.Errors) == 0 || len(s.Conditions) != 0 || s.Expression != nil || len(s.Joins) != 0 {
		t.Errorf("analysis kept a partial search: %#v", s)
	}
	if errs := Validate(query); len(errs) == 0 {
		t.Error("validation accepted unsupported search")
	}
}

func TestSelectedSearchDroppedByNormalizationFailsClosed(t *testing.T) {
	for _, query := range []string{
		`T | mv-apply x on (U | search kind=bad "needle") | where y == 1`,
		`T | mv-apply x on (U | search "needle") | where y == 1`,
		`T | partition by x (U | search kind=bad "needle") | where y == 1`,
		`T | partition by x (U | search "needle") | where y == 1`,
	} {
		t.Run(query, func(t *testing.T) { assertSearchFailureOnly(t, query) })
	}
}

func TestLookupChildSearchFailsClosed(t *testing.T) {
	assertSearchFailureOnly(t, `T | lookup (U | search "needle") on Id | where y == 1`)
}

func TestNestedSearchValidationIsRecursive(t *testing.T) {
	for _, query := range []string{
		`T | search "a" or (Field in ((U | search kind=bad "b")))`,
		`T | search "a" or (Field in ((U | search in (V) "b")))`,
		`T | search "a" or (Field in ((search in (unsupported("x").T) "b")))`,
	} {
		t.Run(query, func(t *testing.T) { assertSearchFailureOnly(t, query) })
	}
	query := `T | search "a" or (Field in ((U | search "b")))`
	if r := ExtractConditions(query); len(r.Errors) != 0 || len(searchLeaves(r.Expression)) != 1 {
		t.Fatalf("valid nested search source lost: %#v", r)
	}
	if errs := Validate(query); len(errs) != 0 {
		t.Fatalf("valid nested search rejected: %v", errs)
	}
}

func TestMaterializeSearchSourceIsPreserved(t *testing.T) {
	for _, predicate := range []string{`"make_set("`, `"https://example.invalid/a:b"`} {
		query := `materialize(search ` + predicate + `) | where x == 1`
		r := ExtractConditions(query)
		leaves := searchLeaves(r.Expression)
		if len(r.Errors) != 0 || len(leaves) != 1 || leaves[0].Text != predicate || len(r.Conditions) != 1 {
			t.Errorf("materialize search source lost: %#v", r)
		}
		if errs := Validate(query); len(errs) != 0 {
			t.Errorf("materialize validation rejected source: %v", errs)
		}
	}
}
