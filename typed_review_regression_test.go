package kql

import (
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
