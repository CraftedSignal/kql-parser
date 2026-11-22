package kql

import (
	"context"
	"reflect"
	"testing"
)

func requireAggregateResult(t *testing.T, p *ParseResult, index int, names []string) {
	t.Helper()
	if p.Dataflow == nil || index >= len(p.Aggregations) {
		t.Fatalf("missing result model: %+v", p)
	}
	agg := p.Aggregations[index]
	result := agg.Result
	if result == nil || !result.Complete || len(result.Outputs) != len(names) || len(names) == 0 || agg.StageID == "" {
		t.Fatalf("incomplete result: %+v", agg)
	}
	if len(names) > 1 && (agg.Column != "" || agg.BindingID != "") {
		t.Fatal("multiple members fabricated scalar summary")
	}
	if len(names) == 1 && (agg.Column != names[0] || agg.BindingID != result.Outputs[0].BindingID) {
		t.Fatal("single member legacy mismatch")
	}
	seen := map[string]bool{}
	for i, member := range result.Outputs {
		if member.Ordinal != i || member.Column != names[i] || member.BindingID == "" || seen[member.BindingID] {
			t.Fatalf("member identity: %+v", result)
		}
		seen[member.BindingID] = true
		found := false
		for _, binding := range p.Dataflow.Bindings {
			if binding.ID == member.BindingID {
				found = true
				if binding.StageID != agg.StageID || binding.Kind != "aggregate" || binding.Field != member.Column || binding.Expression != result.Expression {
					t.Fatalf("member binding: %+v", binding)
				}
				for _, stage := range p.Dataflow.Stages {
					if stage.ID == agg.StageID && binding.InputScopeID != stage.Inputs[0].ScopeID {
						t.Fatal("member input scope")
					}
				}
			}
		}
		if !found {
			t.Fatal("missing result member binding")
		}
	}
}

func TestAggregateTuplePublic(t *testing.T) {
	for _, tc := range []struct {
		item, expression string
		names            []string
	}{
		{`(Low,High)=percentiles(Value,5,95)`, `percentiles(Value,5,95)`, []string{"Low", "High"}},
		{`percentiles(Value,005,(95))`, `percentiles(Value,005,(95))`, []string{"percentile_Value_5", "percentile_Value_95"}},
		{`(Low)=percentiles(Value,5,95)`, `percentiles(Value,5,95)`, []string{"Low", "percentile_Value_95"}},
		{`(Low,High)=percentilesw(Value,Weight,5.5,9e1)`, `percentilesw(Value,Weight,5.5,9e1)`, []string{"Low", "High"}},
		{`(Only)=count()`, `count()`, []string{"Only"}},
		{`(['a]b'],['Chosen' // c
 ' Name'])=percentiles(Value,5,95)`, `percentiles(Value,5,95)`, []string{"a]b", "Chosen Name"}},
	} {
		t.Run(tc.item, func(t *testing.T) {
			last := tc.names[len(tc.names)-1]
			query := `T | summarize ` + tc.item + ` | where ['` + last + `']>10`
			if errs := Validate(query); len(errs) != 0 {
				t.Fatalf("tuple syntax: %v", errs)
			}
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			if len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) || !reflect.DeepEqual(p.Aggregations, s.Aggregations) || len(p.Aggregations) != 1 {
				t.Fatalf("tuple model: %+v / %+v", p, s)
			}
			requireAggregateResult(t, p, 0, tc.names)
			result := p.Aggregations[0].Result
			if result.CallOrdinal != 0 || result.Expression != tc.expression || p.Dataflow.Stages[0].Source != "summarize "+tc.item || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.BindingID != result.Outputs[len(tc.names)-1].BindingID {
				t.Fatalf("call/source/downstream: %+v", p)
			}
		})
	}
}

func TestAggregateTupleCollisionsAndReplacement(t *testing.T) {
	for _, compound := range []bool{false, true} {
		alias := "percentile_Value_5"
		if compound {
			alias = "(" + alias + ")"
		}
		p := ExtractConditions(`T | summarize percentiles(Value,5), ` + alias + `=percentiles(Value,50,95)`)
		requireAggregateResult(t, p, 0, []string{"percentile_Value_5"})
		first := "percentile_Value_5"
		if compound {
			first += "1"
		}
		requireAggregateResult(t, p, 1, []string{first, "percentile_Value_95"})
		if p.Aggregations[1].Result.CallOrdinal != 1 {
			t.Fatal("call ordinal")
		}
		scope := p.Dataflow.Scopes[len(p.Dataflow.Scopes)-1]
		visible := 2
		if compound {
			visible = 3
		}
		if len(p.Dataflow.Stages[0].BindingIDs) != 3 || len(scope.BindingIDs) != visible {
			t.Fatalf("historical/visible: %+v", p.Dataflow)
		}
		if !compound && scope.BindingIDs[0] != p.Aggregations[1].Result.Outputs[0].BindingID {
			t.Fatal("replaced binding still shadows visible output")
		}
	}
	p := ExtractConditions(`T | summarize count(), percentiles(Value,5,5) by percentile_Value_5, percentile_Value_51`)
	requireAggregateResult(t, p, 1, []string{"percentile_Value_52", "percentile_Value_53"})
	if p.Aggregations[1].Result.CallOrdinal != 1 || p.Aggregations[0].Result != nil {
		t.Fatal("ordinal among all calls")
	}
}

func TestAggregateTupleOpaqueAndInvalid(t *testing.T) {
	for _, item := range []string{`(A,B)=percentile(Value,95)`, `(A,B)=count()`, `(A,)=percentiles(Value,5,95)`, `(A,B)=`, `([abc],B)=percentiles(Value,5,95)`} {
		query := `T | summarize ` + item
		if len(Validate(query)) == 0 || len(ExtractConditions(query).Errors) == 0 || len(AnalyzeQuery(query).Errors) == 0 {
			t.Fatalf("invalid tuple admitted: %s", query)
		}
	}
	for _, item := range []string{`(Low)=percentilesw(Value,Weight,5,99.9)`, `percentiles(Value,9e1)`, `(A)=custom(Value)`, `(A,B)=percentiles(Value,dynamic([5,95]))`, `(A,B)=percentiles(Value,5,95), arg_max(Time,*)`, `([''],B)=percentiles(Value,5,95)`, `percentiles(Value,5,95) as P`} {
		query := `T | summarize ` + item
		if errs := Validate(query); len(errs) != 0 {
			t.Fatalf("unknown tuple syntax: %s: %v", query, errs)
		}
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		for _, flow := range []*QueryDataflow{p.Dataflow, s.Dataflow} {
			if flow == nil || flow.Complete || flow.Stages[0].Effect != "opaque" || len(flow.Bindings) != 0 {
				t.Fatalf("unknown tuple became finite: %+v", flow)
			}
		}
		if len(p.Aggregations) == 0 {
			t.Fatal("detached call descriptor lost")
		}
		for _, agg := range p.Aggregations {
			if agg.StageID != "" || agg.BindingID != "" || agg.Result != nil && (agg.Result.Complete || agg.Result.Outputs != nil) {
				t.Fatalf("provisional result escaped: %+v", agg)
			}
		}
	}
}

func TestAggregateTupleNestedCloneAndAtomic(t *testing.T) {
	const query = `T | summarize count(), (L,H)=percentiles(Value,5,95)`
	p := ExtractConditions(query)
	requireAggregateResult(t, p, 1, []string{"L", "H"})
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	clone := semanticFromExtraction(p, b)
	cancel()
	clone.Aggregations[1].Result.Outputs[0].Column = "changed"
	clone.Aggregations[1].Result.Expression = "changed"
	if p.Aggregations[1].Result.Outputs[0].Column != "L" || p.Aggregations[1].Result.Expression != "percentiles(Value,5,95)" {
		t.Fatal("result clone aliases source")
	}
	nested := ExtractConditions(`T | join (Other | summarize (['L'],H)=percentiles(Value,5,95)) on Key`)
	if len(nested.Joins) != 1 || nested.Joins[0].Subsearch == nil {
		t.Fatalf("nested result: %+v", nested)
	}
	requireAggregateResult(t, nested.Joins[0].Subsearch, 0, []string{"L", "H"})
	if nested.Joins[0].Subsearch.Aggregations[0].Result.CallOrdinal != 0 || nested.Joins[0].Subsearch.Dataflow.Stages[0].Source != `summarize (['L'],H)=percentiles(Value,5,95)` {
		t.Fatal("nested source/coordinate")
	}
	limits := defaultParseLimits()
	limits.maxOutputItems = 0
	limited := extractConditionsContext(context.Background(), query, limits)
	if len(limited.Errors) != 1 || limited.Dataflow != nil || len(limited.Aggregations) != 0 {
		t.Fatal("partial limited tuple")
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if r := AnalyzeQueryContext(ctx, query); len(r.Errors) != 1 || r.Dataflow != nil || len(r.Aggregations) != 0 {
		t.Fatal("partial canceled tuple")
	}
}

// Explicit oracle table used only to migrate the approved old percentile rows.
func tupleFixtureColumns(call string) []string {
	switch call {
	case "percentiles(Bytes,5,50,95)", "percentilesw(Bytes,Weight,5,50,95)":
		return []string{"percentile_Bytes_5", "percentile_Bytes_50", "percentile_Bytes_95"}
	case "P=percentiles(Bytes,5,50,95)", "P=percentilesw(Bytes,Weight,5,50,95)":
		return []string{"P", "percentile_Bytes_50", "percentile_Bytes_95"}
	case "percentiles(Bytes,95)", "percentilesw(Bytes,Weight,95)", "percentile(Bytes,95)", "percentilew(Bytes,Weight,95)":
		return []string{"percentile_Bytes_95"}
	case "P=percentiles(Bytes,95)", "P=percentilesw(Bytes,Weight,95)", "P=percentile(Bytes,95)", "P=percentilew(Bytes,Weight,95)", "percentile(Bytes,95) as P", "percentiles(Bytes,95) as P", "percentilew(Bytes,Weight,95) as P":
		return []string{"P"}
	}
	return nil
}

func TestAggregateTupleR1ASCollision(t *testing.T) {
	query := `T | summarize sum(Value), percentile(Value,95) as sum_Value | where sum_Value1>0`
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	if len(Validate(query))+len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) {
		t.Fatalf("AS collision: %+v / %+v", p, s)
	}
	requireAggregateResult(t, p, 1, []string{"sum_Value1"})
	stage := p.Dataflow.Stages[0]
	scope := p.Dataflow.Scopes[1]
	if len(stage.BindingIDs) != 2 || !reflect.DeepEqual(scope.BindingIDs, stage.BindingIDs) || p.Dataflow.Bindings[0].Field != "sum_Value" || len(p.Conditions) != 1 || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.BindingID != p.Aggregations[1].BindingID {
		t.Fatalf("AS replaced visible automatic output: %+v", p)
	}
}

func TestAggregateTupleR1NamedRankCompatibility(t *testing.T) {
	for _, query := range []string{
		`T | summarize P=percentile(Value,Rank) | where P>10`,
		`let Rank=95; T | summarize percentilesw(Value,Weight,Rank) as P | where P>10`,
	} {
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		if len(Validate(query))+len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) {
			t.Fatalf("named rank compatibility: %s: %+v / %+v", query, p, s)
		}
		requireAggregateResult(t, p, 0, []string{"P"})
		if p.Aggregations[0].Field != "Value" || len(p.Conditions) != 1 || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.BindingID != p.Aggregations[0].BindingID {
			t.Fatalf("named rank ownership: %+v", p)
		}
	}
}
