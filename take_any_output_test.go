package kql

import (
	"reflect"
	"testing"
)

func TestTakeAnyOutputPublic(t *testing.T) {
	for _, tc := range []struct{ call, field, column, reference string }{
		{`take_any(State)`, "State", "State", "State"},
		{`take_any(['State Name'])`, "State Name", "State Name", `['State Name']`},
		{`take_any((State))`, "State", "State", "State"},
		{`take_any(Bytes+1)`, "", "any_arg0", "any_arg0"},
		{`take_any(tostring(State))`, "", "State", "State"},
		{`take_any(tolower(State))`, "", "any_arg0", "any_arg0"},
		{`take_any(Chosen=State)`, "State", "Chosen", "Chosen"},
		{`Result=take_any(Chosen=State)`, "State", "Result", "Result"},
	} {
		t.Run(tc.call, func(t *testing.T) {
			query := `T | summarize ` + tc.call + ` | where ` + tc.reference + `=="x"`
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			if len(p.Errors)+len(s.Errors)+len(p.Warnings)+len(s.Warnings) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) || !reflect.DeepEqual(p.Aggregations, s.Aggregations) {
				t.Fatalf("models: %+v / %+v", p, s)
			}
			if len(p.Aggregations) != 1 || len(p.Dataflow.Bindings) != 1 || len(p.Conditions) != 1 {
				t.Fatalf("output inventory: %+v", p)
			}
			agg, binding, stage := p.Aggregations[0], p.Dataflow.Bindings[0], p.Dataflow.Stages[0]
			expression := tc.call
			if tc.call == `Result=take_any(Chosen=State)` {
				expression = `take_any(Chosen=State)`
			}
			if agg.Function != "take_any" || agg.Field != tc.field || agg.Column != tc.column || agg.StageID != stage.ID || agg.BindingID != binding.ID || agg.PipeStage != stage.PipeStage || agg.Pipeline != stage.Pipeline || binding.Field != tc.column || binding.Kind != "aggregate" || binding.Expression != expression || binding.InputScopeID != stage.Inputs[0].ScopeID || binding.StageID != stage.ID || stage.Source != "summarize "+tc.call || stage.Effect != "aggregate" || !reflect.DeepEqual(stage.BindingIDs, []string{binding.ID}) {
				t.Fatalf("aggregate/source ownership: %+v / %+v / %+v", agg, binding, stage)
			}
			condition := p.Conditions[0]
			if condition.Field != tc.column || condition.FieldBinding == nil || condition.FieldBinding.BindingID != binding.ID || condition.InputScopeID != stage.OutputScopeID {
				t.Fatalf("downstream result: %+v", condition)
			}
			s.Aggregations[0].Column = "changed"
			s.Dataflow.Bindings[0].Expression = "changed"
			if p.Aggregations[0].Column != tc.column || p.Dataflow.Bindings[0].Expression != expression {
				t.Fatal("public models share aggregate storage")
			}
		})
	}
}

func TestTakeAnyOutputCollision(t *testing.T) {
	query := `T | summarize take_any(State), take_any(State) by State, State1 | where State3=="x"`
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	if len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) || len(p.Aggregations) != 2 || len(p.Dataflow.Bindings) != 4 {
		t.Fatalf("collision model: %+v", p)
	}
	for i, want := range []string{"State2", "State3"} {
		agg := p.Aggregations[i]
		if agg.Column != want || agg.Field != "State" || agg.BindingID == "" || agg.StageID != p.Dataflow.Stages[0].ID {
			t.Fatalf("group-first metric: %+v", agg)
		}
		found := false
		for _, binding := range p.Dataflow.Bindings {
			if binding.ID == agg.BindingID {
				found = true
				if binding.Kind != "aggregate" || binding.Field != want {
					t.Fatalf("metric binding: %+v", binding)
				}
			}
		}
		if !found {
			t.Fatal("missing metric binding")
		}
	}
	if len(p.Conditions) != 1 || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.BindingID != p.Aggregations[1].BindingID {
		t.Fatalf("collision downstream: %+v", p.Conditions)
	}
}

func TestTakeAnyUnknownRollback(t *testing.T) {
	query := `T | summarize N=count(), take_any(State.Path) by Known | where N>0`
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	for _, flow := range []*QueryDataflow{p.Dataflow, s.Dataflow} {
		if flow == nil || flow.Complete || flow.Stages[0].Effect != "opaque" || len(flow.Bindings) != 0 || len(flow.Stages[0].BindingIDs) != 0 {
			t.Fatalf("unknown name leaked finite stage: %+v", flow)
		}
	}
	for _, agg := range p.Aggregations {
		if agg.BindingID != "" || agg.StageID != "" {
			t.Fatalf("partial aggregate ownership: %+v", agg)
		}
	}
	if len(p.Conditions) != 1 || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.Kind != "unknown" {
		t.Fatalf("unknown output resolved: %+v", p.Conditions)
	}
}

func TestTakeAnyNamedMultiOutputOpaque(t *testing.T) {
	for _, call := range []string{`take_any(Chosen=State, Other)`, `Result=take_any(Chosen=State, Other)`, `take_any()`} {
		query := `T | summarize N=count(), ` + call
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		for _, flow := range []*QueryDataflow{p.Dataflow, s.Dataflow} {
			if flow == nil || flow.Complete || len(flow.Bindings) != 0 || flow.Stages[0].Effect != "opaque" {
				t.Fatalf("non-single argument became finite %s: %+v", call, flow)
			}
		}
	}
}
