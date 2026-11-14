package kql

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSearchPredicatesPreserveCompleteSource(t *testing.T) {
	for _, predicate := range []string{
		`not("needle")`, `not "needle"`, // Bare NOT is a local compatibility form.
		`"alpha" and "beta"`, `"alpha" or "beta"`, `"alpha" or "beta" and "gamma"`,
		`("alpha" or "beta") and not("gamma")`, `"needle"`, `'needle'`, `@"nee""dle"`,
		`"nee" "dle"`, "\"nee\" /* separator */ \"dle\"", `""`,
		`"needle*"`, `"*needle"`, `"*needle*"`, `"ne*le"`, `"*"`, `*`,
		`"https://example.invalid/ioc"`, `"make_set("`, `"materialize("`,
		`Field:"needle"`, `['Field Name']:"needle"`, `Field == "needle"`,
		`Field matches regex @"a.*b"`, `"needle" or (Amount > 2 and Amount < 9)`,
		`@"a|b"`, `"é" or not("ß")`, "\"\"\"alpha\nbeta\"\"\"",
		`Field in ((U | project Field))`,
	} {
		t.Run(predicate, func(t *testing.T) {
			query := `T | search ` + predicate
			r := ExtractConditions(query)
			if len(r.Errors) != 0 || len(r.Conditions) != 0 || r.Expression == nil || r.Expression.Operator != "search" {
				t.Fatalf("search missing or lowered: %#v", r)
			}
			var envelope struct {
				Search struct {
					Text, Kind, Scope   string
					PipeStage, Pipeline int
					TableSources        []string `json:"table_sources"`
				}
			}
			encoded, err := json.Marshal(r.Expression)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Search.Text != predicate || envelope.Search.Scope != "input" || envelope.Search.Kind != "" {
				t.Fatalf("search source/context changed: %s", encoded)
			}
			semantic := AnalyzeQuery(query)
			if len(semantic.Errors) != 0 || semantic.Expression == nil || semantic.Expression.Operator != "search" {
				t.Fatalf("semantic search missing: %#v", semantic)
			}
			encoded, err = json.Marshal(semantic)
			if err != nil {
				t.Fatal(err)
			}
			var decoded SemanticQuery
			if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(semantic, &decoded) {
				t.Fatalf("roundtrip changed search: %s / %v", encoded, err)
			}
			if errs := Validate(query); len(errs) != 0 {
				t.Fatalf("validation rejected preserved search: %v", errs)
			}
			if got := NormalizeQueryForDebug(query); got != query {
				t.Fatalf("normalization changed protected search: %q", got)
			}
		})
	}
}

func TestMalformedSearchIsFailureOnly(t *testing.T) {
	for _, query := range []string{
		`T | search in (U) "needle"`, `search kind=unknown "needle"`, `search`,
		`search in () "needle"`, `search "needle" or`, `search Field:`, `search in (T "needle"`,
		`search in (toscalar(T | count)) "needle"`, `search in (cluster(x).T) "needle"`,
		`search in (unsupported("x").T) "needle"`,
	} {
		t.Run(query, func(t *testing.T) {
			r := ExtractConditions(query)
			if len(r.Errors) == 0 || len(r.Conditions) != 0 || r.Expression != nil {
				t.Fatalf("partial malformed search: %#v", r)
			}
			s := AnalyzeQuery(query)
			if len(s.Errors) == 0 || len(s.Conditions) != 0 || s.Expression != nil {
				t.Fatalf("partial malformed semantic search: %#v", s)
			}
			if errs := Validate(query); len(errs) == 0 {
				t.Fatal("validation accepted malformed search")
			}
		})
	}
	if errs := Validate(`T | where Field:"needle"`); len(errs) == 0 {
		t.Fatal("search syntax escaped into where")
	}
}

func searchLeaves(expression *ConditionExpression) []*SearchPredicate {
	if expression == nil {
		return nil
	}
	if expression.Operator == BooleanSearch {
		return []*SearchPredicate{expression.Search}
	}
	var result []*SearchPredicate
	for _, child := range expression.Children {
		result = append(result, searchLeaves(child)...)
	}
	return result
}

func TestSearchScopeKindStagesAndOwnership(t *testing.T) {
	for _, fixture := range []struct {
		query, scope, kind string
		selectors          []string
	}{
		{`search "needle"`, "database", "", nil},
		{`search in (T,U*) "needle"`, "tables", "", []string{"T", "U*"}},
		{`search kind=case_sensitive in (T,U*) "needle"`, "tables", "case_sensitive", []string{"T", "U*"}},
		{`search in (['Table Name'],cluster("c").database("d").T) "needle"`, "tables", "", []string{`['Table Name']`, `cluster("c").database("d").T`}},
		{`T | search kind=default "needle"`, "input", "default", nil},
		{`T | search kind=case_insensitive "needle"`, "input", "case_insensitive", nil},
		{`T | search kind=case_sensitive "needle" or Field has "X"`, "input", "case_sensitive", nil},
	} {
		r := ExtractConditions(fixture.query)
		leaves := searchLeaves(r.Expression)
		if len(r.Errors) > 0 || len(r.Conditions) != 0 || len(leaves) != 1 {
			t.Fatalf("lost scope fixture %s: %#v", fixture.query, r)
		}
		leaf := leaves[0]
		if leaf.Scope != fixture.scope || leaf.Kind != fixture.kind || !reflect.DeepEqual(leaf.TableSources, fixture.selectors) || leaf.PipeStage != 0 || leaf.Pipeline != 0 {
			t.Errorf("scope changed: %s: %#v", fixture.query, leaf)
		}
		if errs := Validate(fixture.query); len(errs) > 0 {
			t.Errorf("scope validate: %s: %v", fixture.query, errs)
		}
		if len(r.Pipelines) != 1 || r.Pipelines[0].Source == "DummyTable" || r.Pipelines[0].Source == "search" {
			t.Errorf("invented search source: %#v", r)
		}
	}
	for _, query := range []string{`T | search "alpha" | search "beta"`, `T | search rand() > 0.5 | search rand() > 0.5`} {
		r := ExtractConditions(query)
		leaves := searchLeaves(r.Expression)
		if len(r.Errors) > 0 || len(leaves) != 2 || leaves[0].PipeStage != 0 || leaves[1].PipeStage != 1 || r.Expression.Operator != BooleanAnd || len(r.Pipelines) != 1 || r.Pipelines[0].Operators != 2 {
			t.Errorf("repeated searches collapsed/staged incorrectly: %#v / %#v", r, leaves)
		}
	}
	for _, fixture := range []struct {
		query                   string
		searchStage, whereStage int
	}{
		{`T | where x == 1 | search not("needle")`, 1, 0},
		{`search "alpha" or "beta" | where x == 1`, 0, 1},
		{`T | extend Label=Name | search Label:"needle" | where x == 1`, 1, 2},
	} {
		r := ExtractConditions(fixture.query)
		leaves := searchLeaves(r.Expression)
		if len(r.Errors) > 0 || len(leaves) != 1 || len(r.Conditions) != 1 || leaves[0].PipeStage != fixture.searchStage || r.Conditions[0].PipeStage != fixture.whereStage || r.Expression.Operator != BooleanAnd {
			t.Errorf("mixed filter topology: %#v / %#v", r, leaves)
		}
	}
	r := ExtractConditions(`T | join (U | search "needle") on Id`)
	if len(r.Errors) > 0 || len(searchLeaves(r.Expression)) != 0 || len(r.Joins) != 1 || r.Joins[0].Subsearch == nil || len(searchLeaves(r.Joins[0].Subsearch.Expression)) != 1 {
		t.Errorf("join search escaped child: %#v", r)
	}
	r = ExtractConditions(`T | where Name == "é" | join (U | search "ß") on Id`)
	if len(r.Errors) > 0 || len(r.Joins) != 1 || r.Joins[0].Subsearch == nil || len(r.Joins[0].Subsearch.Errors) > 0 || len(searchLeaves(r.Joins[0].Subsearch.Expression)) != 1 || searchLeaves(r.Joins[0].Subsearch.Expression)[0].Text != `"ß"` {
		t.Errorf("Unicode source offsets corrupted join search: %#v", r)
	}
	r = ExtractConditions(`T | union (U | search "needle")`)
	if leaves := searchLeaves(r.Expression); len(r.Errors) > 0 || len(leaves) != 1 || leaves[0].Pipeline != 1 || r.Pipelines[1].Operator != "union" {
		t.Errorf("union search lost ownership: %#v / %#v", r, leaves)
	}
	for _, query := range []string{`T | extend msg="| search not 'needle'"`, "T // | search not 'needle'\n| where x == 1"} {
		if r := ExtractConditions(query); len(searchLeaves(r.Expression)) != 0 {
			t.Errorf("text became operator: %#v", r)
		}
	}
	r = ExtractConditions(`let threshold=1; T | search x > threshold`)
	if leaves := searchLeaves(r.Expression); len(r.Errors) > 0 || len(leaves) != 1 || leaves[0].Text != `x > threshold` || len(r.LetStatements) != 1 {
		t.Errorf("search let binding rewritten: %#v / %#v", r, leaves)
	}
}

func TestSearchSourceProtectionProvenanceAndCloning(t *testing.T) {
	query := `T | where a == "__kql_search_" "0__" | search "__kql_search_0__" or "make_set(" | search "materialize("`
	r := ExtractConditions(query)
	leaves := searchLeaves(r.Expression)
	if len(r.Errors) > 0 || len(leaves) != 2 || len(r.Conditions) != 1 || r.Conditions[0].Value != "__kql_search_0__" || leaves[0].Text != `"__kql_search_0__" or "make_set("` || leaves[1].Text != `"materialize("` {
		t.Fatalf("marker provenance collision: %#v / %#v", r, leaves)
	}
	r = ExtractConditions(`search in (T,U*) "needle"`)
	s := semanticFromExtraction(r, nil)
	if s.Expression == nil || s.Expression.Search == nil {
		t.Fatal("semantic search missing")
	}
	s.Expression.Search.TableSources[0] = "changed"
	if r.Expression.Search.TableSources[0] != "T" {
		t.Fatal("semantic table selectors alias parser data")
	}
}

func TestSearchSourceProtectionAtNestedSourceBoundaries(t *testing.T) {
	for _, query := range []string{
		`T | join (search "https://example.invalid/make_set(") on Id`,
		`T | union (search "https://example.invalid/make_set(")`,
		`let Q = search "https://example.invalid/make_set("; Q | where x == 1`,
		`let Q = (search "https://example.invalid/make_set("); Q | where x == 1`,
		`T | where Field in ((search "https://example.invalid/make_set("))`,
	} {
		t.Run(query, func(t *testing.T) {
			protected, provenance := protectSearchClauses(query, nil)
			if len(provenance.clauses) != 1 {
				t.Fatalf("expected one protected search, got %d: %q", len(provenance.clauses), protected)
			}
			if got := provenance.restore(protected, nil); got != query {
				t.Fatalf("nested search source changed: %q", got)
			}
		})
	}
}

func TestSearchSelectionKeepsIgnoredQueriesAndDeclarationsOut(t *testing.T) {
	query := "search \"needle\"\n\nU | where x == 1"
	r := ExtractConditions(query)
	if leaves := searchLeaves(r.Expression); len(r.Errors) > 0 || len(r.Conditions) != 0 || len(leaves) != 1 || leaves[0].Text != `"needle"` {
		t.Fatalf("secondary query changed search: %#v / %#v", r, leaves)
	}
	if errs := Validate(query); len(errs) == 0 {
		t.Fatal("strict validation accepted second query")
	}
	r = ExtractConditions(`let f=(){T | search "ignored"}; U | where x == 1`)
	if len(r.Errors) > 0 || len(searchLeaves(r.Expression)) != 0 || len(r.Conditions) != 1 {
		t.Fatalf("function declaration search extracted: %#v", r)
	}
}

func TestSearchUnsupportedSelectorsHaveExplicitDiagnostics(t *testing.T) {
	for _, fixture := range []struct{ query, diagnostic string }{
		{`search in (unsupported("x").T) "needle"`, "unsupported search table selector"},
		{`search in (cluster("c").cluster("d").T) "needle"`, "unsupported search table selector"},
		{`search in (T U) "needle"`, "malformed or unsupported search syntax"},
		{`search kind=unknown "needle"`, "unsupported search kind"},
		{`T | search in (U) "needle"`, "search table scope cannot be combined with input"},
	} {
		r := ExtractConditions(fixture.query)
		if len(r.Errors) != 1 || r.Errors[0] != fixture.diagnostic || r.Expression != nil || len(r.Conditions) != 0 {
			t.Errorf("diagnostic changed: %s: %#v", fixture.query, r)
		}
	}
}
