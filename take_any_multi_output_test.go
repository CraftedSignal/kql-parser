package kql

import (
	"context"
	"reflect"
	"testing"
)

func TestTakeAnyMultiPublic(t *testing.T) {
	for _, tc := range []struct {
		item, call string
		index      int
		names      []string
	}{
		{`(A,B)=take_any(State,User)`, `take_any(State,User)`, 0, []string{"A", "B"}},
		{`take_any(State,User+1)`, `take_any(State,User+1)`, 0, []string{"State", "any_arg1"}},
		{`take_any(Chosen=State+1,['Odd Name']=User+1)`, `take_any(Chosen=State+1,['Odd Name']=User+1)`, 0, []string{"Chosen", "Odd Name"}},
		{`(Outer)=take_any(Chosen=State,User)`, `take_any(Chosen=State,User)`, 0, []string{"Outer", "User"}},
		{`(A,B)=take_any(State.Path,custom(User))`, `take_any(State.Path,custom(User))`, 0, []string{"A", "B"}},
		{`take_any(State,Chosen=State.Path)`, `take_any(State,Chosen=State.Path)`, 0, []string{"State", "Chosen"}},
		{`(Only)=take_any(State.Path)`, `take_any(State.Path)`, 0, []string{"Only"}},
		{`take_any(Name, Value)`, `take_any(Name, Value)`, 0, []string{"Name", "Value"}},
		{`count(), take_any(Name, Value)`, `take_any(Name, Value)`, 1, []string{"Name", "Value"}},
		{`take_any(Chosen=State, Other)`, `take_any(Chosen=State, Other)`, 0, []string{"Chosen", "Other"}},
		{`Result=take_any(Chosen=State, Other)`, `take_any(Chosen=State, Other)`, 0, []string{"Result", "Other"}},
		{`take_any(['X']=State, Other)`, `take_any(['X']=State, Other)`, 0, []string{"X", "Other"}},
		{`Result=take_any(['X']=State, Other)`, `take_any(['X']=State, Other)`, 0, []string{"Result", "Other"}},
	} {
		t.Run(tc.item, func(t *testing.T) {
			last := tc.names[len(tc.names)-1]
			query := `T | summarize ` + tc.item + ` | where ['` + last + `']=="x"`
			if errs := Validate(query); len(errs) != 0 {
				t.Fatalf("syntax: %v", errs)
			}
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			if len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) || !reflect.DeepEqual(p.Aggregations, s.Aggregations) || len(p.Aggregations) != tc.index+1 {
				t.Fatalf("multi model: %+v / %+v", p, s)
			}
			requireAggregateResult(t, p, tc.index, tc.names)
			result := p.Aggregations[tc.index].Result
			if result.CallOrdinal != tc.index || result.Expression != tc.call || p.Dataflow.Stages[0].Source != "summarize "+tc.item || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.BindingID != result.Outputs[len(tc.names)-1].BindingID {
				t.Fatalf("call/source/downstream: %+v", p)
			}
		})
	}
}

func TestTakeAnyMultiNamingAndCompatibility(t *testing.T) {
	p := ExtractConditions(`T | summarize count(), take_any(State,Chosen=State+1,1,State) by State,State1`)
	requireAggregateResult(t, p, 1, []string{"State2", "Chosen", "any_arg2", "State3"})
	if p.Aggregations[1].Result.CallOrdinal != 1 {
		t.Fatal("call ordinal")
	}
	for _, item := range []string{`P=take_any(State.Path)`, `take_any(State.Path) as P`, `take_any(P=State.Path)`} {
		query := `T | summarize ` + item + ` | where P=="x"`
		r := ExtractConditions(query)
		if len(r.Errors) != 0 || r.Dataflow == nil || !r.Dataflow.Complete || len(r.Aggregations) != 1 || r.Aggregations[0].Result != nil || r.Aggregations[0].Column != "P" || r.Conditions[0].FieldBinding == nil || r.Conditions[0].FieldBinding.BindingID != r.Aggregations[0].BindingID {
			t.Fatalf("old single named Unknown changed: %+v", r)
		}
	}
	old := ExtractConditions(`T | summarize take_any(State),take_any(Other) as State | where State1=="x"`)
	if old.Dataflow == nil || !old.Dataflow.Complete || old.Aggregations[0].Result != nil || old.Aggregations[1].Result != nil || old.Aggregations[1].Column != "State1" || len(old.Dataflow.Scopes[1].BindingIDs) != 2 {
		t.Fatalf("old single AS replaced output: %+v", old)
	}
}

func TestTakeAnyMultiReplacement(t *testing.T) {
	for _, compound := range []bool{false, true} {
		declaration := "State"
		if compound {
			declaration = "(State)"
		}
		p := ExtractConditions(`T | summarize take_any(State,User), ` + declaration + `=take_any(Other,Third)`)
		requireAggregateResult(t, p, 0, []string{"State", "User"})
		first := "State"
		if compound {
			first = "State1"
		}
		requireAggregateResult(t, p, 1, []string{first, "Third"})
		visible := p.Dataflow.Scopes[1].BindingIDs
		want := 3
		if compound {
			want = 4
		}
		if len(p.Dataflow.Stages[0].BindingIDs) != 4 || len(visible) != want || !compound && visible[0] != p.Aggregations[1].Result.Outputs[0].BindingID {
			t.Fatalf("historical/visible outputs: %+v", p.Dataflow)
		}
	}
}

func TestTakeAnyMultiOpaqueAndInvalid(t *testing.T) {
	for _, item := range []string{`(A,B,C)=take_any(State,User)`, `take_any(['bad\q']=State,User)`, `take_any(State,)`} {
		query := `T | summarize ` + item
		if len(Validate(query)) == 0 || len(ExtractConditions(query).Errors) == 0 {
			t.Fatalf("invalid admitted: %s", query)
		}
	}
	for _, item := range []string{`take_any(State,State.Path)`, `(A)=take_any(State,custom(User))`, `take_any(State,*)`, `take_any(*,State)`, `take_any(State,User) as P`, `take_any(['']=State,User)`, `P=take_any(['']=State,User)`, `take_any(State,User),arg_max(Time,*)`, `take_any()`} {
		query := `T | summarize ` + item
		if errs := Validate(query); len(errs) != 0 {
			t.Fatalf("opaque syntax: %s: %v", query, errs)
		}
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		for _, flow := range []*QueryDataflow{p.Dataflow, s.Dataflow} {
			if flow == nil || flow.Complete || flow.Stages[0].Effect != "opaque" || len(flow.Bindings) != 0 {
				t.Fatalf("unknown became finite %s: %+v", item, flow)
			}
		}
		for _, agg := range p.Aggregations {
			if agg.StageID != "" || agg.BindingID != "" || agg.Result != nil && (agg.Result.Complete || agg.Result.Outputs != nil) {
				t.Fatalf("provisional outputs escaped: %+v", agg)
			}
		}
		if item == `take_any()` && p.Aggregations[0].Result != nil {
			t.Fatal("zero argument legacy carrier changed")
		}
	}
}

func TestTakeAnyMultiNestedCloneAtomic(t *testing.T) {
	const query = `T | summarize take_any(State,User), take_any(State,User)`
	p := ExtractConditions(query)
	requireAggregateResult(t, p, 0, []string{"State", "User"})
	requireAggregateResult(t, p, 1, []string{"State1", "User1"})
	if p.Aggregations[1].Result.CallOrdinal != 1 {
		t.Fatal("repeated call ordinal")
	}
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	clone := semanticFromExtraction(p, b)
	cancel()
	clone.Aggregations[0].Result.Expression = "changed"
	clone.Aggregations[0].Result.Outputs[0].BindingID = "changed"
	if p.Aggregations[0].Result.Expression != "take_any(State,User)" || p.Aggregations[0].Result.Outputs[0].BindingID == "changed" {
		t.Fatal("member clone aliases source")
	}
	nested := ExtractConditions(`T | join (Other | summarize take_any(State,User)) on Key`)
	if len(nested.Joins) != 1 || nested.Joins[0].Subsearch == nil {
		t.Fatalf("nested owner: %+v", nested)
	}
	requireAggregateResult(t, nested.Joins[0].Subsearch, 0, []string{"State", "User"})
	limits := defaultParseLimits()
	limits.maxOutputItems = 0
	limited := extractConditionsContext(context.Background(), query, limits)
	if len(limited.Errors) != 1 || limited.Dataflow != nil || len(limited.Aggregations) != 0 {
		t.Fatal("partial limited result")
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if r := AnalyzeQueryContext(ctx, query); len(r.Errors) != 1 || r.Dataflow != nil || len(r.Aggregations) != 0 {
		t.Fatal("partial canceled result")
	}
}
