package kql

import (
	"reflect"
	"testing"
)

func TestKQLOrGroupingPreservesCaseAndReferenceOperands(t *testing.T) {
	for _, fixture := range []struct {
		query  string
		fields []string
		refs   []string
	}{
		{`T | where User == "a" or user == "b"`, []string{"User", "user"}, []string{"", ""}},
		{`T | where x == y or x == z`, []string{"x", "x"}, []string{"y", "z"}},
	} {
		t.Run(fixture.query, func(t *testing.T) {
			r := ExtractConditions(fixture.query)
			if len(r.Errors) != 0 || len(r.Conditions) != 2 || r.Expression == nil || r.Expression.Operator != BooleanOr || len(r.Expression.Children) != 2 {
				t.Fatalf("OR predicate collapsed: %#v", r)
			}
			for i, condition := range r.Conditions {
				if condition.Field != fixture.fields[i] || condition.ValueReference != fixture.refs[i] || len(condition.Alternatives) != 0 || r.Expression.Children[i].ConditionIndex != i {
					t.Errorf("OR leaf %d changed: %#v / %#v", i, condition, r.Expression.Children[i])
				}
			}
			s := AnalyzeQuery(fixture.query)
			if len(s.Errors) != 0 || len(s.Conditions) != 2 || s.Expression == nil || s.Expression.Operator != BooleanOr || len(s.Expression.Children) != 2 {
				t.Fatalf("semantic OR predicate collapsed: %#v", s)
			}
			for i, child := range s.Expression.Children {
				if child.Condition == nil || child.Condition.Field != fixture.fields[i] || child.Condition.ValueReference != fixture.refs[i] {
					t.Errorf("semantic OR leaf %d changed: %#v", i, child)
				}
			}
		})
	}
}

func TestKQLDedupIdentityPreservesCaseReferencesAndLiteralStar(t *testing.T) {
	for _, query := range []string{
		`T | where User == "a" and user == "a"`,
		`T | where x == y and x == z`,
	} {
		r := ExtractConditions(query)
		if len(r.Errors) != 0 || len(r.Conditions) != 2 {
			t.Fatalf("fixture did not extract two predicates: %#v", r)
		}
		if got := DeduplicateConditions(r.Conditions); !reflect.DeepEqual(got, r.Conditions) {
			t.Errorf("dedup collapsed distinct predicates: %#v", got)
		}
	}
	r := ExtractConditions(`T | where x == "*"`)
	if len(r.Errors) != 0 || len(r.Conditions) != 1 {
		t.Fatalf("literal star fixture did not extract: %#v", r)
	}
	if got := DeduplicateConditions(r.Conditions); !reflect.DeepEqual(got, r.Conditions) {
		t.Errorf("literal star dropped: %#v", got)
	}
}

func TestKQLDedupIdentityIsInjectiveAndStable(t *testing.T) {
	base := Condition{Field: "A", Operator: "==", Value: "v", PipeStage: 1, Pipeline: 2}
	for name, change := range map[string]func(*Condition){
		"field case":   func(c *Condition) { c.Field = "a" },
		"operator":     func(c *Condition) { c.Operator = "!=" },
		"value":        func(c *Condition) { c.Value = "V" },
		"reference":    func(c *Condition) { c.ValueReference = "v" },
		"negation":     func(c *Condition) { c.Negated = true },
		"stage":        func(c *Condition) { c.PipeStage = 2 },
		"connector":    func(c *Condition) { c.LogicalOp = "OR" },
		"computed":     func(c *Condition) { c.IsComputed = true },
		"source case":  func(c *Condition) { c.SourceField = "a" },
		"pipeline":     func(c *Condition) { c.Pipeline = 3 },
		"alternatives": func(c *Condition) { c.Alternatives = []string{"v"} },
	} {
		t.Run(name, func(t *testing.T) {
			other := base
			change(&other)
			if conditionDedupKey(base) == conditionDedupKey(other) {
				t.Fatalf("identity collision: %#v and %#v", base, other)
			}
		})
	}
	for _, alternatives := range [][2][]string{
		{nil, {""}},
		{{"a\x00b"}, {"a", "b"}},
		{{"a\x00\xff"}, {"a", "\xff"}},
	} {
		a, b := base, base
		a.Alternatives, b.Alternatives = alternatives[0], alternatives[1]
		if conditionDedupKey(a) == conditionDedupKey(b) {
			t.Errorf("alternative boundary collision: %#v and %#v", a.Alternatives, b.Alternatives)
		}
	}
	input := []Condition{
		{Field: "B", Operator: "==", Value: "old", PipeStage: 0, Pipeline: 0},
		{Field: "A", Operator: "==", Value: "first", PipeStage: 1, Pipeline: 0},
		{Field: "b", Operator: "==", Value: "lower", PipeStage: 0, Pipeline: 0},
		{Field: "B", Operator: "==", Value: "new", PipeStage: 2, Pipeline: 0},
		{Field: "B", Operator: "==", Value: "other pipeline", PipeStage: 0, Pipeline: 1},
		{Field: "A", Operator: "==", Value: "first", PipeStage: 1, Pipeline: 0},
	}
	want := []Condition{input[1], input[2], input[3], input[4]}
	for i := 0; i < 10; i++ {
		if got := DeduplicateConditions(input); !reflect.DeepEqual(got, want) {
			t.Fatalf("latest-stage source order changed: %#v", got)
		}
	}
}
