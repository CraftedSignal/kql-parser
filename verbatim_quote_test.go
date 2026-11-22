package kql

import (
	"reflect"
	"testing"
)

func TestVerbatimQuoteIdentityPublic(t *testing.T) {
	for _, tc := range []struct{ name, reference, want string }{
		{`[@"""A"""]`, `['"A"']`, `"A"`},
		{`[@'''A''']`, `["'A'"]`, `'A'`},
		{`[h@"""A"""]`, `['"A"']`, `"A"`},
		{`[@"A"]`, `['A']`, `A`},
		{`["""A"""]`, `['A']`, `A`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := `T | extend ` + tc.name + `=src | where ` + tc.reference + `=="x"`
			if errs := Validate(query); len(errs) != 0 {
				t.Fatalf("syntax: %v", errs)
			}
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			if len(p.Errors)+len(s.Errors)+len(p.Warnings)+len(s.Warnings) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) || !reflect.DeepEqual(p.Conditions, s.Conditions) {
				t.Fatalf("public identity models: %+v / %+v", p, s)
			}
			if len(p.Dataflow.Bindings) != 1 || len(p.Conditions) != 1 || len(p.Aggregations)+len(s.Aggregations) != 0 {
				t.Fatalf("assignment inventory: %+v", p)
			}
			binding, stage, condition := p.Dataflow.Bindings[0], p.Dataflow.Stages[0], p.Conditions[0]
			if binding.Field != tc.want || binding.Expression != "src" || binding.Kind != "assignment" || binding.StageID != stage.ID || binding.InputScopeID != stage.Inputs[0].ScopeID || stage.Source != "extend "+tc.name+"=src" || stage.Effect != "assign" || len(stage.BindingIDs) != 1 || stage.BindingIDs[0] != binding.ID {
				t.Fatalf("declared identity/source: %+v / %+v", binding, stage)
			}
			if condition.Field != tc.want || condition.FieldBinding == nil || condition.FieldBinding.BindingID != binding.ID || condition.InputScopeID != stage.OutputScopeID {
				t.Fatalf("equivalent reference lost dependency: %+v", condition)
			}
		})
	}
}

func TestVerbatimQuoteLiteralPublic(t *testing.T) {
	for _, tc := range []struct{ literal, want string }{
		{`@"""A"""`, `"A"`}, {`@'''A'''`, `'A'`}, {`h@"""A"""`, `"A"`}, {`@"A"`, `A`}, {`"""A"""`, `A`},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			query := `T | where Value==` + tc.literal
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			if len(Validate(query))+len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Conditions, s.Conditions) || len(p.Conditions) != 1 || p.Conditions[0].RHS == nil || p.Conditions[0].RHS.Kind != OperandString || p.Conditions[0].RHS.Source != tc.literal || p.Conditions[0].RHS.Value != tc.want {
				t.Fatalf("literal decode: %+v / %+v", p, s)
			}
		})
	}
}
