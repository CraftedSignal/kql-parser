package kql

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTypedOperandScalarPairs(t *testing.T) {
	for _, fixture := range []struct {
		literal string
		want    Operand
	}{
		{`42`, Operand{Kind: OperandNumber, Source: `42`, Value: `42`}},
		{`"42"`, Operand{Kind: OperandString, Source: `"42"`, Value: `42`}},
		{`true`, Operand{Kind: OperandBool, Source: `true`, Value: `true`}},
		{`"true"`, Operand{Kind: OperandString, Source: `"true"`, Value: `true`}},
		{`false`, Operand{Kind: OperandBool, Source: `false`, Value: `false`}},
		{`"false"`, Operand{Kind: OperandString, Source: `"false"`, Value: `false`}},
		{`null`, Operand{Kind: OperandNull, Source: `null`}},
		{`"null"`, Operand{Kind: OperandString, Source: `"null"`, Value: `null`}},
		{`""`, Operand{Kind: OperandString, Source: `""`}},
	} {
		query := `T | where x == ` + fixture.literal
		parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 1 || len(semantic.Conditions) != 1 {
			t.Fatalf("%s: parse %#v, semantic %#v", query, parsed, semantic)
		}
		if got := parsed.Conditions[0].RHS; got == nil || !reflect.DeepEqual(*got, fixture.want) {
			t.Errorf("parse %s: RHS = %#v, want %#v", query, got, fixture.want)
		}
		if got := semantic.Conditions[0].RHS; got == nil || !reflect.DeepEqual(*got, fixture.want) {
			t.Errorf("semantic %s: RHS = %#v, want %#v", query, got, fixture.want)
		}
		encoded, err := json.Marshal(semantic)
		if err != nil {
			t.Fatal(err)
		}
		var decoded SemanticQuery
		if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(semantic, &decoded) {
			t.Errorf("JSON %s: %v / %s", query, err, encoded)
		}
	}
}

func TestTypedOperandListAndRange(t *testing.T) {
	for _, fixture := range []struct {
		predicate string
		want      []Operand
	}{
		{`x in (42, "42", true, "true", null, "null")`, []Operand{
			{Kind: OperandNumber, Source: `42`, Value: `42`}, {Kind: OperandString, Source: `"42"`, Value: `42`},
			{Kind: OperandBool, Source: `true`, Value: `true`}, {Kind: OperandString, Source: `"true"`, Value: `true`},
			{Kind: OperandNull, Source: `null`}, {Kind: OperandString, Source: `"null"`, Value: `null`},
		}},
	} {
		r := ExtractConditions(`T | where ` + fixture.predicate)
		if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].RHS == nil || r.Conditions[0].RHS.Kind != OperandList || r.Conditions[0].RHS.Source != `(42, "42", true, "true", null, "null")` || !reflect.DeepEqual(r.Conditions[0].RHS.Items, fixture.want) {
			t.Errorf("%s: %#v", fixture.predicate, r)
		}
	}
	for _, predicate := range []string{`x between (1 .. "2")`, `x !between (1 .. "2")`} {
		r := ExtractConditions(`T | where ` + predicate)
		if len(r.Conditions) != 2 || r.Conditions[0].RHS == nil || r.Conditions[1].RHS == nil || r.Conditions[0].RHS.Kind != OperandNumber || r.Conditions[1].RHS.Kind != OperandString {
			t.Errorf("%s: %#v", predicate, r)
		}
	}
}

func TestTypedOperandORIdentityAndOwnership(t *testing.T) {
	query := `T | where x == 42 or x == "42" or x == 42`
	parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
	if len(parsed.Conditions) != 1 || parsed.Conditions[0].RHS == nil || parsed.Conditions[0].RHS.Kind != OperandList ||
		len(parsed.Conditions[0].RHS.Items) != 2 || parsed.Conditions[0].RHS.Items[0].Kind != OperandNumber || parsed.Conditions[0].RHS.Items[1].Kind != OperandString ||
		!reflect.DeepEqual(parsed.Conditions[0].Alternatives, []string{"42", "42"}) {
		t.Fatalf("typed OR group: %#v", parsed)
	}
	if len(semantic.Conditions) != 1 || semantic.Expression == nil || semantic.Expression.Condition == nil || semantic.Conditions[0].RHS == nil {
		t.Fatalf("semantic group: %#v", semantic)
	}
	semantic.Conditions[0].RHS.Items[0].Value = "changed"
	if parsed.Conditions[0].RHS.Items[0].Value != "42" {
		t.Fatal("semantic RHS aliases parse RHS")
	}
	if semantic.Expression.Condition.RHS.Items[0].Value != "42" {
		t.Fatal("semantic flat condition aliases expression leaf")
	}
	manual := []Condition{{Field: "x", Operator: "==", Value: "42"}, {Field: "x", Operator: "==", Value: "42", RHS: &Operand{Kind: OperandNumber, Source: "42", Value: "42"}}}
	if got := DeduplicateConditions(manual); len(got) != 2 || got[0].RHS != nil {
		t.Fatalf("nil RHS conflated with typed: %#v", got)
	}
}

func TestTypedOperandNumericAndConstructorSyntax(t *testing.T) {
	for _, literal := range []string{`-5`, `+5`, `(+5)`, `1e3`, `1e-3`, `0x10`, `9007199254740993`, `42L`, `1.25M`, `1e308`} {
		r := ExtractConditions(`T | where x > ` + literal)
		if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].RHS == nil || r.Conditions[0].RHS.Kind != OperandNumber || r.Conditions[0].RHS.Source != literal || r.Conditions[0].RHS.Value != literal {
			t.Errorf("numeric %s: %#v RHS=%+v", literal, r, r.Conditions[0].RHS)
		}
	}
	for _, fixture := range []struct {
		literal, typ string
		arg          Operand
	}{
		{`int(null)`, `int`, Operand{Kind: OperandNull, Source: `null`}},
		{`long(null)`, `long`, Operand{Kind: OperandNull, Source: `null`}},
		{`long("42")`, `long`, Operand{Kind: OperandString, Source: `"42"`, Value: `42`}},
		{`real(1e3)`, `real`, Operand{Kind: OperandNumber, Source: `1e3`, Value: `1e3`}},
	} {
		r := ExtractConditions(`T | where x == ` + fixture.literal)
		want := Operand{Kind: OperandTyped, Source: fixture.literal, Type: fixture.typ, Items: []Operand{fixture.arg}}
		if len(r.Conditions) != 1 || r.Conditions[0].RHS == nil || !reflect.DeepEqual(*r.Conditions[0].RHS, want) {
			t.Errorf("constructor %s: %#v", fixture.literal, r)
		}
	}
}

func TestTypedOperandNativeTypedAndComputedControl(t *testing.T) {
	for _, fixture := range []struct{ literal, typ string }{
		{`datetime(2026-01-01)`, "datetime"},
		{`time(01:02:03)`, "time"},
		{`guid(01234567-89ab-cdef-0123-456789abcdef)`, "guid"},
		{`dynamic([42, "42", true, null])`, "dynamic"},
		{`dynamic({"x":42})`, "dynamic"},
	} {
		r := ExtractConditions(`T | where x == ` + fixture.literal)
		if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].RHS == nil || r.Conditions[0].RHS.Kind != OperandTyped || r.Conditions[0].RHS.Type != fixture.typ || r.Conditions[0].RHS.Source != fixture.literal {
			t.Errorf("native typed %s: %#v", fixture.literal, r)
		}
	}
	r := ExtractConditions(`T | where x == long(1 + 2)`)
	if len(r.Conditions) != 0 || r.Expression == nil || r.Expression.Operator != BooleanScalar {
		t.Errorf("computed constructor became typed condition: %#v", r)
	}
}

func TestTypedOperandFieldAndStringDecoding(t *testing.T) {
	for _, fixture := range []struct{ literal, value string }{
		{`"line\nnext"`, "line\nnext"},
		{`'it\'s'`, "it's"},
		{`@"a""b"`, `a"b`},
		{`"\u00e9"`, "é"},
		{"```line\nnext```", "line\nnext"},
		{`"\b\f"`, "\b\f"},
	} {
		r := ExtractConditions(`T | where x == ` + fixture.literal)
		if len(r.Conditions) != 1 || r.Conditions[0].RHS == nil || r.Conditions[0].RHS.Kind != OperandString || r.Conditions[0].RHS.Source != fixture.literal || r.Conditions[0].RHS.Value != fixture.value {
			t.Errorf("string %s: %#v", fixture.literal, r)
		}
	}
	r := ExtractConditions(`T | where x == ['where']`)
	if len(r.Conditions) != 1 || r.Conditions[0].RHS == nil || r.Conditions[0].RHS.Kind != OperandField || r.Conditions[0].RHS.Source != `['where']` || r.Conditions[0].RHS.Value != "where" || r.Conditions[0].ValueReference != "where" {
		t.Errorf("field reference: %#v", r)
	}
}

func TestTypedOperandLetDefiningSource(t *testing.T) {
	for _, fixture := range []struct {
		query string
		want  Operand
	}{
		{`let n = 42; T | where x == n`, Operand{Kind: OperandNumber, Source: `42`, Value: `42`, Binding: `n`}},
		{`let n = "42"; T | where x == n`, Operand{Kind: OperandString, Source: `"42"`, Value: `42`, Binding: `n`}},
		{`let n = true; T | where x == n`, Operand{Kind: OperandBool, Source: `true`, Value: `true`, Binding: `n`}},
	} {
		r := ExtractConditions(fixture.query)
		if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].RHS == nil || !reflect.DeepEqual(*r.Conditions[0].RHS, fixture.want) {
			t.Errorf("let %s: %#v, RHS=%+v", fixture.query, r, func() *Operand {
				if len(r.Conditions) > 0 {
					return r.Conditions[0].RHS
				}
				return nil
			}())
		}
	}
}

func TestTypedOperandLetDynamicListSources(t *testing.T) {
	query := `let allowed = dynamic([42, "42", true]); T | where x in (allowed)`
	r := ExtractConditions(query)
	if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].RHS == nil {
		t.Fatalf("let list: %#v", r)
	}
	want := Operand{Kind: OperandList, Source: `dynamic([42, "42", true])`, Binding: "allowed", Items: []Operand{
		{Kind: OperandNumber, Source: `42`, Value: `42`, Binding: "allowed"},
		{Kind: OperandString, Source: `"42"`, Value: `42`, Binding: "allowed"},
		{Kind: OperandBool, Source: `true`, Value: `true`, Binding: "allowed"},
	}}
	if !reflect.DeepEqual(*r.Conditions[0].RHS, want) || !reflect.DeepEqual(r.Conditions[0].Alternatives, []string{"42", "42", "true"}) {
		t.Fatalf("let list = %+v / %v, want %+v", r.Conditions[0].RHS, r.Conditions[0].Alternatives, want)
	}
}

func TestTypedOperandNormalizerOriginalSource(t *testing.T) {
	for _, fixture := range []struct {
		literal string
		kind    OperandKind
	}{
		{`datetime(2026-02-04 10:00:00)`, OperandTyped},
		{`30min`, OperandTyped},
		{`1000.`, OperandNumber},
		{`"a" "b"`, OperandString},
		{`"datetime(2026-02-04 10:00:00)"`, OperandString},
		{`"30min"`, OperandString},
		{`"make_set("`, OperandString},
		{`"__kql_typed_origin_0__"`, OperandString},
		{`"a\0b"`, OperandString},
	} {
		r := ExtractConditions(`T | where x == ` + fixture.literal)
		if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].RHS == nil || r.Conditions[0].RHS.Source != fixture.literal || r.Conditions[0].RHS.Kind != fixture.kind {
			t.Errorf("normalizer %s: %#v RHS=%+v", fixture.literal, r, func() *Operand {
				if len(r.Conditions) > 0 {
					return r.Conditions[0].RHS
				}
				return nil
			}())
		}
	}
}

func TestTypedOperandMembershipVariantsAndHas(t *testing.T) {
	for _, operator := range []string{"in", "in~", "!in", "!in~"} {
		r := ExtractConditions(`T | where x ` + operator + ` (42, "42", true)`)
		if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].RHS == nil || r.Conditions[0].RHS.Kind != OperandList || len(r.Conditions[0].RHS.Items) != 3 || r.Conditions[0].RHS.Items[0].Kind != OperandNumber || r.Conditions[0].RHS.Items[1].Kind != OperandString || r.Conditions[0].RHS.Items[2].Kind != OperandBool {
			t.Errorf("%s: %#v", operator, r)
		}
	}
	for _, operator := range []string{"has_any", "has_all"} {
		r := ExtractConditions(`T | where x ` + operator + ` (42, "42", true)`)
		want := 3
		if operator == "has_any" {
			want = 1
		}
		if len(r.Errors) != 0 || len(r.Conditions) != want {
			t.Errorf("%s: %#v", operator, r)
			continue
		}
		for i, kind := range []OperandKind{OperandNumber, OperandString, OperandBool} {
			var item *Operand
			if operator == "has_any" {
				item = &r.Conditions[0].RHS.Items[i]
			} else {
				item = r.Conditions[i].RHS
			}
			if item == nil || item.Kind != kind {
				t.Errorf("%s item %d: %+v", operator, i, item)
			}
		}
	}
}

func TestTypedOperandBooleanTopology(t *testing.T) {
	query := `T | where x == 42 or (x == "42" and gate == "yes")`
	r := ExtractConditions(query)
	if len(r.Conditions) != 3 || r.Expression == nil || r.Expression.Operator != BooleanOr || len(r.Expression.Children) != 2 || r.Conditions[0].RHS.Kind != OperandNumber || r.Conditions[1].RHS.Kind != OperandString || len(r.Conditions[0].Alternatives) != 0 {
		t.Errorf("Boolean topology: %#v", r)
	}
}

func TestTypedOperandContextFailureIsAtomic(t *testing.T) {
	query := `T | where x in (42, "42", true)`
	limits := defaultParseLimits()
	limits.maxOutputItems = 3
	parsed := extractConditionsContext(context.Background(), query, limits)
	semantic := analyzeQueryContext(context.Background(), query, limits)
	if len(parsed.Errors) == 0 || len(parsed.Conditions) != 0 || len(semantic.Errors) == 0 || len(semantic.Conditions) != 0 || semantic.Expression != nil {
		t.Fatalf("limited results leaked data: %#v / %#v", parsed, semantic)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	parsed = ExtractConditionsContext(ctx, query)
	semantic = AnalyzeQueryContext(ctx, query)
	if len(parsed.Errors) == 0 || len(parsed.Conditions) != 0 || len(semantic.Errors) == 0 || len(semantic.Conditions) != 0 {
		t.Fatalf("canceled results leaked data: %#v / %#v", parsed, semantic)
	}
}

func TestTypedOperandPortableRawSyntax(t *testing.T) {
	for _, fixture := range []struct {
		predicate string
		want      Operand
	}{
		{`x == "42"`, Operand{Kind: OperandString, Source: `"42"`, Value: "42"}},
		{`x == 42`, Operand{Kind: OperandNumber, Source: `42`, Value: "42"}},
		{`x == true`, Operand{Kind: OperandBool, Source: `true`, Value: "true"}},
	} {
		condition, ok := parsePortablePredicate(fixture.predicate, nil)
		if !ok || condition.RHS == nil || !reflect.DeepEqual(*condition.RHS, fixture.want) {
			t.Errorf("portable %s: %#v ok=%v", fixture.predicate, condition, ok)
		}
	}
	if condition, ok := parsePortablePredicate(`x in (Known)`, map[string][]string{"known": {"a", "b"}}); !ok || condition.RHS == nil || condition.RHS.Kind != OperandField || condition.RHS.Source != `(Known)` || condition.RHS.Value != `Known` {
		t.Errorf("portable reference = %#v, ok=%v", condition, ok)
	}
	if condition, ok := parsePortablePredicate(`x in (42)`, nil); !ok || condition.RHS == nil || condition.RHS.Kind != OperandList || condition.RHS.Source != `(42)` || len(condition.RHS.Items) != 1 || condition.RHS.Items[0].Kind != OperandNumber || condition.RHS.Items[0].Source != `42` {
		t.Errorf("portable singleton list = %#v, ok=%v", condition, ok)
	}
	if condition, ok := parsePortablePredicate(`x in ("a", Unknown)`, map[string][]string{"unknown": {"b"}}); ok {
		t.Errorf("portable mixed source was guessed: %#v", condition)
	}
}

func TestTypedOperandComparisonOrientationAndBounds(t *testing.T) {
	inverse := map[string]string{"==": "==", "!=": "!=", "=~": "=~", "!~": "!~", "<": ">", "<=": ">=", ">": "<", ">=": "<="}
	for op, swapped := range inverse {
		for _, literal := range []string{`42`, `"42"`, `true`, `"true"`, `null`, `"null"`, `1e-9999`} {
			for _, reverse := range []bool{false, true} {
				predicate, wantOp := `x `+op+` `+literal, op
				if reverse {
					predicate, wantOp = literal+` `+op+` x`, swapped
				}
				r := ExtractConditions(`T | where ` + predicate)
				if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].RHS == nil || r.Conditions[0].RHS.Source != literal || r.Conditions[0].Operator != wantOp || r.Conditions[0].Field != "x" {
					t.Errorf("%s: %#v", predicate, r)
				}
			}
		}
	}
	for _, predicate := range []string{`x between (int(null) .. y)`, `x !between (int(null) .. y)`} {
		r := ExtractConditions(`T | where ` + predicate)
		if len(r.Conditions) != 2 || r.Conditions[0].RHS == nil || r.Conditions[1].RHS == nil || r.Conditions[0].RHS.Kind != OperandTyped || r.Conditions[0].RHS.Type != "int" || r.Conditions[1].RHS.Kind != OperandField || r.Conditions[1].RHS.Source != "y" || r.Conditions[1].ValueReference != "y" {
			t.Errorf("range %s: %#v", predicate, r)
		}
	}
}

func TestTypedOperandSemanticPairsDiffer(t *testing.T) {
	for _, pair := range [][2]string{
		{`x == 42`, `x == "42"`}, {`x == true`, `x == "true"`}, {`x == null`, `x == "null"`},
		{`x in (42, "42")`, `x in ("42", "42")`}, {`x between (1 .. 2)`, `x between ("1" .. "2")`},
	} {
		first, second := AnalyzeQuery(`T | where `+pair[0]), AnalyzeQuery(`T | where `+pair[1])
		if len(first.Errors) != 0 || len(second.Errors) != 0 || reflect.DeepEqual(first, second) || len(first.Conditions) == 0 || len(second.Conditions) == 0 || first.Conditions[0].RHS == nil || second.Conditions[0].RHS == nil {
			t.Errorf("semantic pair %s / %s: %#v / %#v", pair[0], pair[1], first, second)
		}
	}
}

func TestTypedOperandLetScopeControls(t *testing.T) {
	for _, query := range []string{
		`let n = 42; let n = "42"; T | where x == n`,
		`let n = 42; T | extend n = "42" | where x == n`,
	} {
		r := ExtractConditions(query)
		if r.Expression == nil || len(scalarLeaves(r.Expression)) == 0 {
			t.Errorf("ambiguous let was typed as known: %#v", r)
		}
	}
	forward := `let early = T | where x == n; let n = 42; T | where y == n`
	if got := inlineLetConstants(forward, extractLetStatements(forward), nil); !strings.Contains(got, `early = T | where x == n`) {
		t.Errorf("forward let body was substituted: %q", got)
	}
	r := ExtractConditions(`let n = 42; T | where x == "n" and y == n`)
	if len(r.Conditions) != 2 || r.Conditions[0].RHS == nil || r.Conditions[1].RHS == nil || r.Conditions[0].RHS.Binding != "" || r.Conditions[0].RHS.Kind != OperandString || r.Conditions[1].RHS.Binding != "n" {
		t.Errorf("quoted let lookalike: %#v", r)
	}
}
