package kql

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func requireTypedReviewCondition(t *testing.T, query string) Condition {
	t.Helper()
	parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 1 || len(semantic.Conditions) != 1 || parsed.Conditions[0].RHS == nil || !reflect.DeepEqual(parsed.Conditions[0].RHS, semantic.Conditions[0].RHS) {
		t.Fatalf("%q: parse=%#v semantic=%#v", query, parsed, semantic)
	}
	return parsed.Conditions[0]
}

func TestTypedReviewOpaqueReferenceMetadata(t *testing.T) {
	for _, fixture := range []struct {
		query, projected, computed, groupExpression, sourceField string
		join                                                     bool
	}{
		{query: `T | project bag['a'][0]`},
		{query: `T | extend d=bag['a'][0] | where d == 1`, computed: `bag['a'][0]`, sourceField: `bag['a'][0]`},
		{query: `T | project d=bag['a'][0] | where d == 1`, projected: `d`, computed: `bag['a'][0]`, sourceField: `bag['a'][0]`},
		{query: `T | extend d=bag['a'][0]+1 | where d == 1`, computed: `bag['a'][0]+1`},
		{query: `T | project d=bag['a'][0]+1 | where d == 1`, projected: `d`, computed: `bag['a'][0]+1`},
		{query: `T | extend d=a+bag['a'][0] | where d == 1`, computed: `a+bag['a'][0]`, sourceField: `a`},
		{query: `T | summarize count() by bag['a'][0]`, groupExpression: `bag['a'][0]`},
		{query: `T | join (U | project bag['a'][0]) on id`, join: true},
	} {
		parsed, semantic := ExtractConditions(fixture.query), AnalyzeQuery(fixture.query)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 {
			t.Fatalf("%q: parse=%#v semantic=%#v", fixture.query, parsed.Errors, semantic.Errors)
		}
		for _, result := range []any{parsed, semantic} {
			encoded, err := json.Marshal(result)
			if err != nil || strings.Contains(string(encoded), "__kql_typed_origin_") {
				t.Errorf("private metadata in %q: %s / %v", fixture.query, encoded, err)
			}
		}
		if fixture.projected == "" && (len(parsed.ProjectedFields) != 0 || len(semantic.ProjectedFields) != 0) || fixture.projected != "" && (!reflect.DeepEqual(parsed.ProjectedFields, []string{fixture.projected}) || !reflect.DeepEqual(semantic.ProjectedFields, []string{fixture.projected})) || len(parsed.GroupByFields) != 0 || len(semantic.GroupByFields) != 0 {
			t.Errorf("fabricated column in %q: parse=%#v semantic=%#v", fixture.query, parsed, semantic)
		}
		if fixture.computed != "" && (parsed.ComputedExpressions["d"] != fixture.computed || semantic.ComputedExpressions["d"] != fixture.computed) {
			t.Errorf("computed expression %q: parse=%#v semantic=%#v", fixture.query, parsed.ComputedExpressions, semantic.ComputedExpressions)
		}
		if fixture.computed != "" && (parsed.ComputedFields["d"] != fixture.sourceField || semantic.ComputedFields["d"] != fixture.sourceField) {
			t.Errorf("computed source %q: parse=%#v semantic=%#v", fixture.query, parsed.ComputedFields, semantic.ComputedFields)
		}
		if fixture.groupExpression != "" && (len(parsed.GroupByExpressions) != 1 || parsed.GroupByExpressions[0] != fixture.groupExpression) {
			t.Errorf("group expression %q: %#v", fixture.query, parsed.GroupByExpressions)
		}
		if fixture.join {
			if len(parsed.Joins) != 1 || parsed.Joins[0].Subsearch == nil || len(parsed.Joins[0].Subsearch.ProjectedFields) != 0 || !reflect.DeepEqual(parsed.Joins[0].ExposedFields, []string{"id"}) || len(semantic.Joins) != 1 || semantic.Joins[0].Subquery == nil || len(semantic.Joins[0].Subquery.ProjectedFields) != 0 || !reflect.DeepEqual(semantic.Joins[0].ExposedFields, []string{"id"}) {
				t.Errorf("join metadata %q: parse=%#v semantic=%#v", fixture.query, parsed.Joins, semantic.Joins)
			}
		}
	}
}

func TestTypedReviewSimpleReferenceMetadataControls(t *testing.T) {
	for _, fixture := range []struct {
		query, projected, computed, group string
	}{
		{query: `T | project x`, projected: `x`},
		{query: `T | project bag['key']`, projected: `bag.key`},
		{query: `T | project d=bag['key']`, projected: `d`, computed: `bag['key']`},
		{query: `T | summarize count() by bag['key']`, group: `bag.key`},
	} {
		parsed, semantic := ExtractConditions(fixture.query), AnalyzeQuery(fixture.query)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 {
			t.Fatalf("%q: parse=%#v semantic=%#v", fixture.query, parsed.Errors, semantic.Errors)
		}
		if fixture.projected != "" && (!reflect.DeepEqual(parsed.ProjectedFields, []string{fixture.projected}) || !reflect.DeepEqual(semantic.ProjectedFields, []string{fixture.projected})) {
			t.Errorf("projected %q: parse=%#v semantic=%#v", fixture.query, parsed.ProjectedFields, semantic.ProjectedFields)
		}
		if fixture.computed != "" && (parsed.ComputedExpressions["d"] != fixture.computed || semantic.ComputedExpressions["d"] != fixture.computed) {
			t.Errorf("computed %q: parse=%#v semantic=%#v", fixture.query, parsed.ComputedExpressions, semantic.ComputedExpressions)
		}
		if fixture.group != "" && (!reflect.DeepEqual(parsed.GroupByFields, []string{fixture.group}) || !reflect.DeepEqual(semantic.GroupByFields, []string{fixture.group})) {
			t.Errorf("grouped %q: parse=%#v semantic=%#v", fixture.query, parsed.GroupByFields, semantic.GroupByFields)
		}
	}
}

func TestTypedReviewAuthoredMarkerCollisions(t *testing.T) {
	for _, fixture := range []struct{ query, source, value string }{
		{`T | where x == "__kql_typed_" "origin_shield_0__" and y == dynamic([1])`, `"__kql_typed_" "origin_shield_0__"`, `__kql_typed_origin_shield_0__`},
		{`T | where x == "/*__kql_typed_" "origin_1__*/" and y == 1000.`, `"/*__kql_typed_" "origin_1__*/"`, `/*__kql_typed_origin_1__*/`},
	} {
		parsed, semantic := ExtractConditions(fixture.query), AnalyzeQuery(fixture.query)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 2 || len(semantic.Conditions) != 2 || parsed.Conditions[0].RHS == nil || semantic.Conditions[0].RHS == nil {
			t.Fatalf("%q: %#v / %#v", fixture.query, parsed, semantic)
		}
		for _, c := range []Condition{parsed.Conditions[0], semantic.Conditions[0]} {
			if c.RHS.Kind != OperandString || c.RHS.Source != fixture.source || c.RHS.Value != fixture.value || c.Value != fixture.value {
				t.Errorf("%q: %+v RHS=%+v", fixture.query, c, c.RHS)
			}
		}
	}
}

func TestTypedReviewStackedOrigins(t *testing.T) {
	for _, fixture := range []struct{ query, source, binding, value string }{
		{`let n = "a\0b"; T | where x == n`, `"a\0b"`, `n`, "a\x00b"},
		{`T | where x == "a\0" "b"`, `"a\0" "b"`, ``, "a\x00b"},
	} {
		parsed, semantic := ExtractConditions(fixture.query), AnalyzeQuery(fixture.query)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 1 || len(semantic.Conditions) != 1 || parsed.Conditions[0].RHS == nil || semantic.Conditions[0].RHS == nil {
			t.Fatalf("%q: %#v / %#v", fixture.query, parsed, semantic)
		}
		for _, c := range []Condition{parsed.Conditions[0], semantic.Conditions[0]} {
			if c.RHS.Source != fixture.source || c.RHS.Binding != fixture.binding || c.RHS.Value != fixture.value || c.Value != `a\x00b` {
				t.Errorf("%q: %+v RHS=%+v", fixture.query, c, c.RHS)
			}
		}
	}
	list := `let n = dynamic(["a\0b", 42]); T | where x in (n)`
	parsed, semantic := ExtractConditions(list), AnalyzeQuery(list)
	for _, result := range []struct {
		errors     []string
		conditions []Condition
	}{{parsed.Errors, parsed.Conditions}, {semantic.Errors, semantic.Conditions}} {
		if len(result.errors) != 0 || len(result.conditions) != 1 || result.conditions[0].RHS == nil || result.conditions[0].RHS.Kind != OperandList || len(result.conditions[0].RHS.Items) != 2 || result.conditions[0].RHS.Items[0].Source != `"a\0b"` || result.conditions[0].RHS.Items[0].Value != "a\x00b" || result.conditions[0].RHS.Items[0].Binding != "n" {
			t.Errorf("stacked list: %#v", result)
		}
	}
	if strings.Contains(fmt.Sprintf("%+v", parsed), "__kql_typed_origin_") || strings.Contains(fmt.Sprintf("%+v", semantic), "__kql_typed_origin_") {
		t.Fatal("private marker leaked")
	}
}

func TestTypedReviewUnaryComputationStaysScalar(t *testing.T) {
	for _, expression := range []string{`x == -"42"`, `x == -true`, `x == +false`} {
		for _, result := range []*ParseResult{ExtractConditions(`T | where ` + expression)} {
			if len(result.Conditions) != 0 || result.Expression == nil || result.Expression.Operator != BooleanScalar || !strings.Contains(result.Expression.Scalar.Text, expression) {
				t.Errorf("%s: %#v", expression, result)
			}
		}
		semantic := AnalyzeQuery(`T | where ` + expression)
		if len(semantic.Conditions) != 0 || semantic.Expression == nil || semantic.Expression.Operator != BooleanScalar {
			t.Errorf("semantic %s: %#v", expression, semantic)
		}
	}
}

func TestTypedReviewQuotedAliasUnchanged(t *testing.T) {
	for _, literal := range []string{`"| mvexpand "`, `"kind=leftantisemi"`, `"| filter "`, `" and $left.A"`} {
		c := requireTypedReviewCondition(t, `T | where x == `+literal)
		if c.RHS.Source != literal || c.RHS.Value != literal[1:len(literal)-1] || c.Value != literal[1:len(literal)-1] {
			t.Errorf("%s: %+v", literal, c)
		}
	}
}

func TestTypedReviewComposedSources(t *testing.T) {
	for _, fixture := range []struct {
		literal, source, child string
		kind                   OperandKind
	}{
		{`(1000.)`, `(1000.)`, ``, OperandNumber},
		{`-1000.`, `-1000.`, ``, OperandNumber},
		{`(30min)`, `(30min)`, ``, OperandTyped},
		{`-30min`, `-30min`, ``, OperandTyped},
		{`("a" "b")`, `("a" "b")`, ``, OperandString},
		{`int(1000.)`, `int(1000.)`, `1000.`, OperandTyped},
		{`string("a" "b")`, `string("a" "b")`, `"a" "b"`, OperandTyped},
	} {
		c := requireTypedReviewCondition(t, `T | where x == `+fixture.literal)
		if c.RHS.Source != fixture.source || c.RHS.Kind != fixture.kind || fixture.kind == OperandNumber && c.RHS.Value != fixture.source || fixture.child != "" && (len(c.RHS.Items) != 1 || c.RHS.Items[0].Source != fixture.child) {
			t.Errorf("%s: %+v", fixture.literal, c.RHS)
		}
	}
	c := requireTypedReviewCondition(t, `T | where x in ((1000.), string("a" "b"), "30min")`)
	if c.RHS.Kind != OperandList || c.RHS.Source != `((1000.), string("a" "b"), "30min")` || len(c.RHS.Items) != 3 || c.RHS.Items[0].Source != `(1000.)` || c.RHS.Items[1].Source != `string("a" "b")` {
		t.Errorf("list composition: %+v", c.RHS)
	}
}

func TestTypedReviewReferenceSourceAndIdentity(t *testing.T) {
	for _, fixture := range []struct{ reference, source, value string }{
		{`bag['key']`, `bag['key']`, `bag.key`},
		{`bag.["key"]`, `bag.["key"]`, `bag.key`},
		{`3plogTime`, `3plogTime`, `3plogTime`},
	} {
		c := requireTypedReviewCondition(t, `T | where x == `+fixture.reference)
		if c.RHS.Kind != OperandField || c.RHS.Source != fixture.source || c.RHS.Value != fixture.value || c.ValueReference != fixture.value {
			t.Errorf("%s: %+v", fixture.reference, c)
		}
	}
}

func TestTypedReviewOpaqueBracketReferenceStaysScalar(t *testing.T) {
	for _, reference := range []string{`bag['two words']`, `bag.["two-words"]`, `bag[0]`} {
		query := `T | where x == ` + reference
		for _, result := range []*ParseResult{ExtractConditions(query)} {
			if len(result.Errors) != 0 || len(result.Conditions) != 0 || result.Expression == nil || result.Expression.Operator != BooleanScalar || result.Expression.Scalar == nil || result.Expression.Scalar.Text != `x == `+reference {
				t.Errorf("opaque reference %q: %#v", reference, result)
			}
		}
		semantic := AnalyzeQuery(query)
		if len(semantic.Errors) != 0 || len(semantic.Conditions) != 0 || semantic.Expression == nil || semantic.Expression.Operator != BooleanScalar || semantic.Expression.Scalar == nil || semantic.Expression.Scalar.Text != `x == `+reference {
			t.Errorf("opaque semantic reference %q: %#v", reference, semantic)
		}
	}
	c := requireTypedReviewCondition(t, `T | where x == bag['key']`)
	if c.RHS.Kind != OperandField || c.RHS.Source != `bag['key']` || c.RHS.Value != `bag.key` || c.ValueReference != `bag.key` {
		t.Errorf("simple key: %+v", c)
	}
}

func TestTypedReviewUnicodeSurrogatePair(t *testing.T) {
	literal := `"\uD83D\uDE00"`
	query := `T | where x == ` + literal
	parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 1 || len(semantic.Conditions) != 1 {
		t.Fatalf("parse=%#v semantic=%#v", parsed, semantic)
	}
	for _, condition := range []Condition{parsed.Conditions[0], semantic.Conditions[0]} {
		if condition.RHS == nil || condition.RHS.Kind != OperandString || condition.RHS.Source != literal || condition.RHS.Value != "😀" {
			t.Errorf("surrogate pair = %+v", condition.RHS)
		}
	}
}

func TestTypedReviewPortableMixedListDeclinesLossyProjection(t *testing.T) {
	if condition, ok := parsePortablePredicate(`x in (42,"42",true,"true")`, nil); ok {
		t.Errorf("lossy portable list was accepted: %+v", condition)
	}
}

func TestTypedReviewJoinedChildAndLeftReference(t *testing.T) {
	joined := `T | join (U | where x == (1000.)) on Key`
	parsed, semantic := ExtractConditions(joined), AnalyzeQuery(joined)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Joins) != 1 || parsed.Joins[0].Subsearch == nil || len(semantic.Joins) != 1 || semantic.Joins[0].Subquery == nil {
		t.Fatalf("joined result: %#v / %#v", parsed, semantic)
	}
	parseChildren, semanticChildren := parsed.Joins[0].Subsearch.Conditions, semantic.Joins[0].Subquery.Conditions
	foundParse, foundSemantic := false, false
	for _, c := range parseChildren {
		if c.Field == "x" && c.RHS != nil && c.RHS.Source == `(1000.)` && c.RHS.Value == `(1000.)` {
			foundParse = true
		}
	}
	for _, c := range semanticChildren {
		if c.Field == "x" && c.RHS != nil && c.RHS.Source == `(1000.)` && c.RHS.Value == `(1000.)` {
			foundSemantic = true
		}
	}
	if !foundParse || !foundSemantic {
		t.Errorf("joined operand lost source: %#v / %#v", parseChildren, semanticChildren)
	}
	for _, query := range []string{`T | where 3plogTime == "a"`, `T | where x == (3plogTime)`} {
		parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 1 || len(semantic.Conditions) != 1 {
			t.Fatalf("reference %q: %#v / %#v", query, parsed, semantic)
		}
		if strings.Contains(query, `where 3plogTime`) {
			if parsed.Conditions[0].Field != `3plogTime` || semantic.Conditions[0].Field != `3plogTime` {
				t.Errorf("left reference identity %q: %#v / %#v", query, parsed.Conditions[0], semantic.Conditions[0])
			}
		} else if parsed.Conditions[0].RHS == nil || parsed.Conditions[0].RHS.Value != `3plogTime` || parsed.Conditions[0].ValueReference != `3plogTime` || semantic.Conditions[0].RHS == nil || semantic.Conditions[0].RHS.Value != `3plogTime` {
			t.Errorf("nested reference identity %q: %#v / %#v", query, parsed.Conditions[0], semantic.Conditions[0])
		}
	}
}

func TestTypedReviewJoinedChildLetBinding(t *testing.T) {
	for _, fixture := range []struct {
		query, source, binding, value string
		kind                          OperandKind
	}{
		{`let n = "a\0b"; T | join (U | where x == n) on Key`, `"a\0b"`, `n`, "a\x00b", OperandString},
		{`let n = dynamic([42,"42"]); T | join (U | where x in (n)) on Key`, `(n)`, ``, ``, OperandList},
	} {
		parsed, semantic := ExtractConditions(fixture.query), AnalyzeQuery(fixture.query)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Joins) != 1 || parsed.Joins[0].Subsearch == nil || len(semantic.Joins) != 1 || semantic.Joins[0].Subquery == nil {
			t.Fatalf("joined let %q: %#v / %#v", fixture.query, parsed, semantic)
		}
		for _, conditions := range [][]Condition{parsed.Joins[0].Subsearch.Conditions, semantic.Joins[0].Subquery.Conditions} {
			if len(conditions) != 1 || conditions[0].RHS == nil {
				t.Errorf("joined let %q: %#v", fixture.query, conditions)
				continue
			}
			rhs := conditions[0].RHS
			if rhs.Kind != fixture.kind {
				t.Errorf("joined let kind %q: %+v", fixture.query, rhs)
			}
			if fixture.kind == OperandString && (rhs.Source != fixture.source || rhs.Binding != fixture.binding || rhs.Value != fixture.value) {
				t.Errorf("joined let string %q: %+v", fixture.query, rhs)
			}
			if fixture.kind == OperandList && (len(rhs.Items) != 2 || rhs.Items[0].Kind != OperandNumber || rhs.Items[0].Source != `42` || rhs.Items[0].Binding != `n` || rhs.Items[1].Kind != OperandString || rhs.Items[1].Source != `"42"` || rhs.Items[1].Binding != `n`) {
				t.Errorf("joined let list %q: %+v", fixture.query, rhs)
			}
		}
	}
}

func TestTypedReviewReferenceRestorationAcrossOperators(t *testing.T) {
	for _, fixture := range []struct{ query, field, source string }{
		{`T | extend 3plogTime=Source | where 3plogTime in (42, "42")`, "3plogTime", "Source"},
		{`T | project 3plogTime=Source | where 3plogTime in (42, "42")`, "3plogTime", "Source"},
		{`T | extend Derived=3plogTime | where Derived in (42, "42")`, "Derived", "3plogTime"},
	} {
		computedParse, computedSemantic := ExtractConditions(fixture.query), AnalyzeQuery(fixture.query)
		for _, result := range []struct {
			name       string
			conditions []Condition
			errors     []string
		}{{"parse", computedParse.Conditions, computedParse.Errors}, {"semantic", computedSemantic.Conditions, computedSemantic.Errors}} {
			if len(result.errors) != 0 || len(result.conditions) != 1 || result.conditions[0].Field != fixture.field || !result.conditions[0].IsComputed || result.conditions[0].SourceField != fixture.source {
				t.Errorf("%s computed reference %q: %#v / %#v", result.name, fixture.query, result.conditions, result.errors)
			}
		}
	}
	boundsParsed, boundsSemantic := ExtractConditions(`T | where x between (3lo .. 3hi)`), AnalyzeQuery(`T | where x between (3lo .. 3hi)`)
	for _, result := range []struct {
		name       string
		conditions []Condition
		errors     []string
	}{
		{"parse", boundsParsed.Conditions, boundsParsed.Errors},
		{"semantic", boundsSemantic.Conditions, boundsSemantic.Errors},
	} {
		if len(result.errors) != 0 || len(result.conditions) != 2 {
			t.Fatalf("%s bounds: %#v / %#v", result.name, result.conditions, result.errors)
		}
		for i, expected := range []struct{ operator, name string }{{">=", "3lo"}, {"<=", "3hi"}} {
			c := result.conditions[i]
			if c.Field != "x" || c.Operator != expected.operator || c.ValueReference != expected.name || c.Value != "" || c.RHS == nil || c.RHS.Kind != OperandField || c.RHS.Source != expected.name || c.RHS.Value != expected.name {
				t.Errorf("%s bound %d: %+v", result.name, i, c)
			}
		}
	}
	for _, fixture := range []struct {
		query string
		count int
	}{
		{`T | where 3plogTime in (42, "42")`, 1},
		{`T | where 3plogTime between (1 .. 2)`, 2},
		{`T | where 3plogTime has_all ("a", "b")`, 2},
		{`T | where 3plogTime == "a"`, 1},
	} {
		parsed, semantic := ExtractConditions(fixture.query), AnalyzeQuery(fixture.query)
		for _, result := range []struct {
			name       string
			conditions []Condition
			errors     []string
		}{{"parse", parsed.Conditions, parsed.Errors}, {"semantic", semantic.Conditions, semantic.Errors}} {
			if len(result.errors) != 0 || len(result.conditions) != fixture.count {
				t.Fatalf("%s %q: %#v / %#v", result.name, fixture.query, result.conditions, result.errors)
			}
			for _, c := range result.conditions {
				if c.Field != "3plogTime" {
					t.Errorf("%s %q: field=%q", result.name, fixture.query, c.Field)
				}
			}
		}
	}
}

func TestTypedReviewCompleteBracketReferenceBoundary(t *testing.T) {
	for _, reference := range []string{`bag['a'][0]`, `bag['a']['b']`, `3bag['key'][0]`} {
		query := `T | where x == ` + reference
		parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
		if len(parsed.Errors) != 0 || len(parsed.Conditions) != 0 || parsed.Expression == nil || parsed.Expression.Operator != BooleanScalar || parsed.Expression.Scalar == nil || parsed.Expression.Scalar.Text != `x == `+reference {
			t.Errorf("parse %q: %#v", query, parsed)
		}
		if len(semantic.Errors) != 0 || len(semantic.Conditions) != 0 || semantic.Expression == nil || semantic.Expression.Operator != BooleanScalar || semantic.Expression.Scalar == nil || semantic.Expression.Scalar.Text != `x == `+reference {
			t.Errorf("semantic %q: %#v", query, semantic)
		}
	}
	for _, fixture := range []struct{ reference, identity string }{
		{`bag['key']`, `bag.key`},
		{`3bag['key']`, `3bag.key`},
	} {
		c := requireTypedReviewCondition(t, `T | where x == `+fixture.reference)
		if c.RHS.Kind != OperandField || c.RHS.Source != fixture.reference || c.RHS.Value != fixture.identity || c.ValueReference != fixture.identity {
			t.Errorf("simple reference %q: %+v", fixture.reference, c)
		}
	}
}
