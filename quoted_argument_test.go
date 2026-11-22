package kql

import (
	"context"
	"reflect"
	"testing"

	"github.com/antlr4-go/antlr/v4"
)

func TestQuotedArgumentPublic(t *testing.T) {
	for _, tc := range []struct{ name, reference, want, outer string }{
		{`['Chosen Name']`, `['Chosen Name']`, "Chosen Name", ""},
		{`['Chosen' // c
 ' Name']`, `['Chosen Name']`, "Chosen Name", ""},
		{`[@"""A"""]`, `['"A"']`, `"A"`, ""},
		{`['Chosen Name']`, `Result`, "Result", "Result="},
	} {
		t.Run(tc.name+tc.outer, func(t *testing.T) {
			call := `take_any(` + tc.name + `=State)`
			query := `T | summarize ` + tc.outer + call + ` | where ` + tc.reference + `=="x"`
			if errs := Validate(query); len(errs) != 0 {
				t.Fatalf("syntax: %v", errs)
			}
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			if len(p.Errors)+len(s.Errors)+len(p.Warnings)+len(s.Warnings) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) || len(p.Aggregations) != 1 || len(p.Conditions) != 1 || len(p.Dataflow.Bindings) != 1 {
				t.Fatalf("models: %+v / %+v", p, s)
			}
			agg, binding, stage := p.Aggregations[0], p.Dataflow.Bindings[0], p.Dataflow.Stages[0]
			if agg.Function != "take_any" || agg.Field != "State" || agg.Column != tc.want || agg.StageID != stage.ID || agg.BindingID != binding.ID || binding.Field != tc.want || binding.Kind != "aggregate" || binding.Expression != call || binding.StageID != stage.ID || binding.InputScopeID != stage.Inputs[0].ScopeID || stage.Source != "summarize "+tc.outer+call || stage.Effect != "aggregate" || !reflect.DeepEqual(stage.BindingIDs, []string{binding.ID}) {
				t.Fatalf("identity/source ownership: %+v / %+v / %+v", agg, binding, stage)
			}
			condition := p.Conditions[0]
			if condition.Field != tc.want || condition.FieldBinding == nil || condition.FieldBinding.BindingID != binding.ID || condition.InputScopeID != stage.OutputScopeID {
				t.Fatalf("downstream: %+v", condition)
			}
		})
	}
}

func TestQuotedArgumentCollision(t *testing.T) {
	query := `T | summarize take_any(['Chosen Name']=State), take_any(['Chosen Name']=Other) by ['Chosen Name'], ['Chosen Name1'] | where ['Chosen Name3']=="x"`
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	if len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) || len(p.Aggregations) != 2 {
		t.Fatalf("collision: %+v", p)
	}
	if p.Aggregations[0].Column != "Chosen Name2" || p.Aggregations[1].Column != "Chosen Name3" || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.BindingID != p.Aggregations[1].BindingID {
		t.Fatalf("group-first allocation: %+v", p)
	}
}

func TestQuotedArgumentBoundaries(t *testing.T) {
	for _, call := range []string{`take_any([abc]=State)`, `take_any(['bad\q']=State)`, `take_any(['X']=)`} {
		query := `T | summarize ` + call
		if len(Validate(query)) == 0 {
			t.Fatalf("invalid named argument syntax: %s", query)
		}
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		if len(p.Errors) == 0 || len(s.Errors) == 0 || p.Dataflow != nil || s.Dataflow != nil {
			t.Fatalf("malformed declaration promoted: %+v / %+v", p, s)
		}
	}
	for _, call := range []string{`take_any(['']=State)`, `Result=take_any(['']=State)`, `take_any(['X']=*)`} {
		query := `T | summarize N=count(), ` + call
		if errs := Validate(query); len(errs) != 0 {
			t.Fatalf("boundary syntax: %s: %v", query, errs)
		}
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		for _, flow := range []*QueryDataflow{p.Dataflow, s.Dataflow} {
			if flow == nil || flow.Complete || flow.Stages[0].Effect != "opaque" || len(flow.Bindings) != 0 || len(flow.Stages[0].BindingIDs) != 0 {
				t.Fatalf("partial finite boundary %s: %+v", call, flow)
			}
		}
		for _, agg := range p.Aggregations {
			if agg.StageID != "" || agg.BindingID != "" {
				t.Fatalf("partial metric owner: %+v", agg)
			}
		}
	}
}

func TestQuotedArgumentGetterAndScalarControl(t *testing.T) {
	for _, label := range []string{`Label`, `['Label']`} {
		call := kqlParserForTest(t, `tostring(`+label+`=State)`).FunctionCall()
		argument := call.ArgumentList().AllArgument()[0]
		if argument.ASSIGN() == nil || argument.Expression() == nil {
			t.Fatal("existing argument getters lost")
		}
		if label == "Label" {
			if argument.Identifier() == nil {
				t.Fatal("ordinary Identifier getter lost")
			}
		} else {
			quoted, ok := argument.(interface{ QUOTED_IDENTIFIER() antlr.TerminalNode })
			if !ok || quoted.QUOTED_IDENTIFIER() == nil || argument.Identifier() != nil {
				t.Fatal("quoted terminal getter unavailable")
			}
		}
		query := `T | where tostring(` + label + `=State)=="x"`
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		if len(Validate(query))+len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || len(p.Conditions)+len(s.Conditions) != 0 || p.Expression == nil || s.Expression == nil || len(p.Dataflow.Bindings) != 0 {
			t.Fatalf("label changed scalar semantics: %+v / %+v", p, s)
		}
	}
}

func TestQuotedArgumentAtomicResources(t *testing.T) {
	const query = `T | summarize take_any(['Chosen Name']=State)`
	for _, resource := range []string{"work", "output"} {
		limits := defaultParseLimits()
		if resource == "work" {
			limits.maxWork = 1
		} else {
			limits.maxOutputItems = 0
		}
		p, s := extractConditionsContext(context.Background(), query, limits), analyzeQueryContext(context.Background(), query, limits)
		if len(p.Errors) != 1 || len(s.Errors) != 1 || p.Dataflow != nil || s.Dataflow != nil || len(p.Aggregations)+len(s.Aggregations)+len(p.Conditions)+len(s.Conditions) != 0 {
			t.Fatalf("partial resource failure: %+v / %+v", p, s)
		}
	}
}
