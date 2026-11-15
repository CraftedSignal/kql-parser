package kql

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func membershipScalar(t *testing.T, query, source string) *SemanticQuery {
	t.Helper()
	parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 {
		t.Fatalf("valid membership %q: parse errors %v, semantic errors %v", query, parsed.Errors, semantic.Errors)
	}
	if len(parsed.Conditions) != 0 || len(semantic.Conditions) != 0 ||
		parsed.Expression == nil || parsed.Expression.Operator != BooleanScalar || parsed.Expression.Scalar == nil || parsed.Expression.Scalar.Text != source ||
		semantic.Expression == nil || semantic.Expression.Operator != BooleanScalar || semantic.Expression.Scalar == nil || semantic.Expression.Scalar.Text != source {
		t.Fatalf("membership source was not a complete scalar %q: parsed %#v, semantic %#v", source, parsed, semantic)
	}
	if parsed.Expression.Scalar.Negated || semantic.Expression.Scalar.Negated {
		t.Fatalf("operator-internal negation applied twice: %#v / %#v", parsed.Expression.Scalar, semantic.Expression.Scalar)
	}
	return semantic
}

func semanticMembershipShape(expression *SemanticExpression) string {
	if expression == nil {
		return "nil"
	}
	switch expression.Operator {
	case BooleanCondition:
		if expression.Condition == nil {
			return "condition(nil)"
		}
		return "condition(" + expression.Condition.Field + ")"
	case BooleanScalar:
		if expression.Scalar == nil {
			return "scalar(nil)"
		}
		prefix := "scalar("
		if expression.Scalar.Negated {
			prefix = "not-scalar("
		}
		return prefix + expression.Scalar.Text + ")"
	default:
		shape := string(expression.Operator) + "("
		for i, child := range expression.Children {
			if i != 0 {
				shape += ","
			}
			shape += semanticMembershipShape(child)
		}
		return shape + ")"
	}
}

func TestMembershipSubquerySemanticIdentity(t *testing.T) {
	queries := []string{
		`T | where Account in ((SigninLogs | project Account))`,
		`T | where Account in ((AuditLogs | project Account))`,
		`T | where Account in ("SubqueryResult")`,
	}
	results := make([]*SemanticQuery, len(queries))
	for i, query := range queries {
		results[i] = AnalyzeQuery(query)
		if len(results[i].Errors) != 0 {
			t.Fatalf("valid membership %q returned errors: %v", query, results[i].Errors)
		}
	}
	for i := range results {
		for j := i + 1; j < len(results); j++ {
			if reflect.DeepEqual(results[i], results[j]) {
				t.Errorf("distinct membership sources produced identical semantic queries: %q and %q", queries[i], queries[j])
			}
		}
	}
}

func TestMembershipSubqueryOpaqueOperatorAndSource(t *testing.T) {
	for _, fixture := range []struct{ predicate string }{
		{`Account in (SigninLogs | project Account)`},
		{`Account in ((SigninLogs | project Account))`},
		{`Account !in ((SigninLogs | project Account))`},
		{`Account in~ ((SigninLogs | project Account))`},
		{`Account !in~ ((SigninLogs | project Account))`},
		{`tolower(Account) in ((SigninLogs | project Account))`},
		{`tolower(Account) !in ((SigninLogs | project Account))`},
		{`isnull(tolower(Account) in ((SigninLogs | project Account)))`},
		{`isnull(Account in ((SigninLogs | project Account))) == false`},
		{`toint(Account in ((SigninLogs | project Account))) + 1 > 0`},
		{`(Account in ((SigninLogs | project Account))) == true`},
	} {
		t.Run(fixture.predicate, func(t *testing.T) {
			membershipScalar(t, `T | where `+fixture.predicate, fixture.predicate)
		})
	}
}

func TestMembershipSubqueryBooleanScope(t *testing.T) {
	const member = `Account in ((SigninLogs | project Account))`
	for _, fixture := range []struct {
		predicate, shape string
		conditions       int
	}{
		{`a == 1 and ` + member, `and(condition(a),scalar(` + member + `))`, 1},
		{member + ` and a == 1`, `and(scalar(` + member + `),condition(a))`, 1},
		{`a == 1 or ` + member, `or(condition(a),scalar(` + member + `))`, 1},
		{`a == 1 or (b == 2 and ` + member + `)`, `or(condition(a),and(condition(b),scalar(` + member + `)))`, 2},
		{`not(` + member + `)`, `not-scalar(` + member + `)`, 0},
		{`not(not(` + member + `))`, `scalar(` + member + `)`, 0},
	} {
		t.Run(fixture.predicate, func(t *testing.T) {
			parsed, semantic := ExtractConditions(`T | where `+fixture.predicate), AnalyzeQuery(`T | where `+fixture.predicate)
			if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != fixture.conditions || len(semantic.Conditions) != fixture.conditions || semanticMembershipShape(semantic.Expression) != fixture.shape {
				t.Fatalf("Boolean membership shape = %s, want %s; parse %#v, semantic %#v", semanticMembershipShape(semantic.Expression), fixture.shape, parsed, semantic)
			}
		})
	}
}

func TestMembershipSubqueryOccurrenceAndNestedOwnership(t *testing.T) {
	const first = `Account in ((SigninLogs | project Account))`
	const second = `User in ((AuditLogs | project User))`
	for _, fixture := range []struct{ predicate, shape string }{
		{first + ` and ` + second, `and(scalar(` + first + `),scalar(` + second + `))`},
		{first + ` and ` + first, `and(scalar(` + first + `),scalar(` + first + `))`},
	} {
		parsed, semantic := ExtractConditions(`T | where `+fixture.predicate), AnalyzeQuery(`T | where `+fixture.predicate)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 0 || len(semantic.Conditions) != 0 || semanticMembershipShape(semantic.Expression) != fixture.shape {
			t.Errorf("occurrence ownership = %s, want %s: %#v / %#v", semanticMembershipShape(semantic.Expression), fixture.shape, parsed, semantic)
		}
	}
	nested := `Account in ((SigninLogs | where User in ((AuditLogs | project User)) | project Account))`
	membershipScalar(t, `T | where `+nested, nested)
}

func TestMembershipSubqueryOriginalSourceAndAuthoredMarkers(t *testing.T) {
	for _, predicate := range []string{
		"Account in ((SigninLogs | where Message == @\"x\"\"y\" // note\n | project Account))",
		`Account in ((SigninLogs | where Message == "é" | project Account))`,
		`Account in ((SigninLogs | where Message == """é""" | project Account))`,
		`Account in ((SigninLogs | where Message containscs "ERR" | project Account))`,
		`Account in ((SigninLogs | search "needle" | project Account))`,
		`Account in ((materialize(SigninLogs | where Message == "make_set(") | project Account))`,
		`Account in ((SigninLogs | where Message == "SubqueryResult" | project Account))`,
		`Account in ((SigninLogs | where Message == "__kql_in_subquery_0__" | project Account))`,
	} {
		t.Run(predicate, func(t *testing.T) { membershipScalar(t, `T | where `+predicate, predicate) })
	}
	for _, literal := range []string{`"SubqueryResult"`, `"__kql_in_subquery_0__"`, `"__kql_in_" "subquery_0__"`, `"a|b"`} {
		query := `T | where Account in (` + literal + `)`
		parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 1 || len(semantic.Conditions) != 1 || parsed.Expression == nil || parsed.Expression.Operator != BooleanCondition || semantic.Expression == nil || semantic.Expression.Operator != BooleanCondition {
			t.Errorf("authored literal %q changed: %#v / %#v", literal, parsed, semantic)
		}
	}
	query := `T | where Account in ("SubqueryResult") and User in ((AuditLogs | project User))`
	parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 1 || len(semantic.Conditions) != 1 || semanticMembershipShape(semantic.Expression) != `and(condition(Account),scalar(User in ((AuditLogs | project User))))` {
		t.Errorf("authored/generated marker collision: %#v / %#v", parsed, semantic)
	}
}

func TestMembershipSubqueryScopeAndNonfilterControls(t *testing.T) {
	for _, query := range []string{
		`T | project x=Account in ((SigninLogs | project Account))`,
		`T | extend x=Account in ((SigninLogs | project Account))`,
		`T | summarize n=countif(Account in ((SigninLogs | project Account)))`,
	} {
		parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 0 || parsed.Expression != nil || len(semantic.Conditions) != 0 || semantic.Expression != nil {
			t.Errorf("nonfilter membership became row filter: %s: %#v / %#v", query, parsed, semantic)
		}
	}
	query := `let unused = T | where Account in ((SigninLogs | project Account)); T | where a == 1`
	parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 1 || len(semantic.Conditions) != 1 || parsed.Conditions[0].Field != "a" || semantic.Conditions[0].Field != "a" {
		t.Errorf("unused let changed selected filter: %#v / %#v", parsed, semantic)
	}
	query = `let allowed = SigninLogs | project Account; T | where Account in ((allowed | project Account))`
	semantic = membershipScalar(t, query, `Account in ((allowed | project Account))`)
	if len(semantic.LetStatements) != 1 || semantic.LetStatements[0].Name != "allowed" {
		t.Errorf("let binding lost: %#v", semantic.LetStatements)
	}
	query = `let allowed = SigninLogs | project Account; T | where Account in (allowed)`
	semantic = membershipScalar(t, query, `Account in (allowed)`)
	if len(semantic.LetStatements) != 1 {
		t.Errorf("let reference lost its binding: %#v", semantic)
	}
	query = `let allowed = SigninLogs | project Account; T | where Account in ("allowed")`
	parsed, semantic = ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 1 || len(semantic.Conditions) != 1 || parsed.Conditions[0].Value != "allowed" || semantic.Expression == nil || semantic.Expression.Operator != BooleanCondition {
		t.Errorf("quoted let name ceased to be a literal: %#v / %#v", parsed, semantic)
	}
	query = `T | where a == 1 | where Account in ((SigninLogs | project Account))`
	parsed, semantic = ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(semantic.Conditions) != 1 || semanticMembershipShape(semantic.Expression) != `and(condition(a),scalar(Account in ((SigninLogs | project Account))))` || semantic.Expression.Children[1].Scalar.PipeStage != 1 || semantic.Expression.Children[1].Scalar.Pipeline != 0 {
		t.Errorf("pipeline stage ownership: %#v / %#v", parsed, semantic)
	}
	query = `let allowed = AuditLogs | project Account; let allowed = SigninLogs | project Account; T | where Account in ((allowed | project Account))`
	semantic = membershipScalar(t, query, `Account in ((allowed | project Account))`)
	if len(semantic.LetStatements) != 2 {
		t.Errorf("shadowed bindings lost: %#v", semantic.LetStatements)
	}
	query = `T | join (U | where Account in ((SigninLogs | project Account))) on Account`
	parsed, semantic = ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(semantic.Joins) != 1 || semantic.Joins[0].Subquery == nil || semanticMembershipShape(semantic.Joins[0].Subquery.Expression) != `scalar(Account in ((SigninLogs | project Account)))` {
		t.Errorf("join child lost original membership: %#v / %#v", parsed, semantic)
	}
	query = `T | union (U | where Account in ((SigninLogs | project Account)))`
	parsed, semantic = ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(semantic.Conditions) != 0 || semanticMembershipShape(semantic.Expression) != `scalar(Account in ((SigninLogs | project Account)))` || semantic.Expression.Scalar.Pipeline == 0 {
		t.Errorf("union child lost owned membership: %#v / %#v", parsed, semantic)
	}
	query = `T | search "Account in ((SigninLogs | project Account))"`
	parsed, semantic = ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(semantic.Conditions) != 0 || semantic.Expression == nil || semantic.Expression.Operator != BooleanSearch {
		t.Errorf("opaque search duplicated membership: %#v / %#v", parsed, semantic)
	}
}

func TestMembershipSubqueryMalformedAndContextFailures(t *testing.T) {
	for _, query := range []string{
		`T | where Account in ((SigninLogs | project ))`,
		`T | where a == 1 and Account in ((SigninLogs | where ResultType == | project Account))`,
		`T | where Account in ((SigninLogs | project Account)`,
		`T | where Account in ((SigninLogs | where Message == "unterminated | project Account))`,
		`T | where Account in ((SigninLogs | search kind=bad "needle" | project Account))`,
		`T | mv-apply x on (U | where Account in ((SigninLogs | project Account))) | where a == 1`,
	} {
		parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
		if len(parsed.Errors) == 0 || len(semantic.Errors) == 0 || len(parsed.Conditions) != 0 || parsed.Expression != nil || len(semantic.Conditions) != 0 || semantic.Expression != nil {
			t.Errorf("malformed membership produced partial success: %s: %#v / %#v", query, parsed, semantic)
		}
		if len(Validate(query)) == 0 {
			t.Errorf("validation accepted malformed or removed membership: %s", query)
		}
	}
	query := `T | where Account in ((SigninLogs | project Account))`
	parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
	if errs := Validate(query); len(errs) != 0 {
		t.Errorf("validation rejected valid membership: %v", errs)
	}
	parseJSON, _ := json.Marshal(parsed)
	semanticJSON, _ := json.Marshal(semantic)
	var parsedCopy ParseResult
	var semanticCopy SemanticQuery
	if json.Unmarshal(parseJSON, &parsedCopy) != nil || json.Unmarshal(semanticJSON, &semanticCopy) != nil {
		t.Errorf("membership JSON round trip changed model: %s / %s", parseJSON, semanticJSON)
	} else {
		parseCopyJSON, _ := json.Marshal(&parsedCopy)
		semanticCopyJSON, _ := json.Marshal(&semanticCopy)
		if string(parseJSON) != string(parseCopyJSON) || string(semanticJSON) != string(semanticCopyJSON) {
			t.Errorf("membership JSON round trip changed model: %s / %s", parseCopyJSON, semanticCopyJSON)
		}
	}
	contextParsed, contextSemantic := ExtractConditionsContext(context.Background(), query), AnalyzeQueryContext(context.Background(), query)
	if !reflect.DeepEqual(parsed, contextParsed) || !reflect.DeepEqual(semantic, contextSemantic) {
		t.Errorf("context API differs: %#v / %#v", contextParsed, contextSemantic)
	}
	semantic.Expression.Scalar.Text = "mutated"
	if parsed.Expression.Scalar.Text != `Account in ((SigninLogs | project Account))` {
		t.Errorf("semantic conversion aliased parse source: %#v", parsed.Expression)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	failed := AnalyzeQueryContext(ctx, query)
	if len(failed.Errors) == 0 || len(failed.Conditions) != 0 || failed.Expression != nil {
		t.Errorf("canceled membership returned partial model: %#v", failed)
	}
	for _, change := range []func(*parseLimits){
		func(limits *parseLimits) { limits.maxWork = 40 },
		func(limits *parseLimits) { limits.maxOutputItems = 1 },
	} {
		limits := defaultParseLimits()
		change(&limits)
		limitedParse := extractConditionsContext(context.Background(), query, limits)
		limitedSemantic := analyzeQueryContext(context.Background(), query, limits)
		if len(limitedParse.Errors) == 0 || len(limitedSemantic.Errors) == 0 || len(limitedParse.Conditions) != 0 || limitedParse.Expression != nil || len(limitedSemantic.Conditions) != 0 || limitedSemantic.Expression != nil {
			t.Errorf("budget failure returned partial membership: %#v / %#v", limitedParse, limitedSemantic)
		}
	}
}

func TestMembershipSourceAssociationRejectsMissingOrDuplicateIdentity(t *testing.T) {
	query := `T | where Account in ((SigninLogs | project Account))`
	for _, mutate := range []func(string, string) string{
		func(normalized, marker string) string { return strings.Replace(normalized, marker, "true", 1) },
		func(normalized, marker string) string {
			return strings.Replace(normalized, marker, marker+" and "+marker, 1)
		},
	} {
		func() {
			budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
			defer cancel()
			protected, ownership := protectMembershipSources(query, budget)
			changed := mutate(protected, ownership.entries[0].marker)
			parser := NewKQLParser(preflight(changed, budget, true, nil))
			parser.RemoveErrorListeners()
			tree := parser.Query()
			defer func() {
				if failure := recover(); failure == nil {
					t.Errorf("missing or duplicated membership identity was accepted: %q", changed)
				} else if _, ok := failure.(budgetAbort); !ok {
					t.Errorf("unexpected association failure: %v", failure)
				}
			}()
			ownership.bind(tree, changed, budget)
		}()
	}
}
