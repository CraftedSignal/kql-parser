package kql

import (
	"context"
	"reflect"
	"testing"
)

func TestArgMinMaxPublic(t *testing.T) {
	for _, tc := range []struct {
		item  string
		names []string
	}{
		{`(Peak,UserName)=arg_max(Time,User)`, []string{"Peak", "UserName"}},
		{`arg_max(Time)`, []string{"Time"}},
		{`arg_min(Time)`, []string{"Time"}},
		{`arg_max(1,User+1)`, []string{"max_", "max__arg1"}},
		{`arg_min(Chosen=Time,User+1)`, []string{"Chosen", "min_Chosen_arg1"}},
		{`(Peak)=arg_max(Time,User+1)`, []string{"Peak", "max_Time_arg1"}},
		{`(Peak,Out)=arg_max(State.Path,custom(User))`, []string{"Peak", "Out"}},
		{`arg_max(['Odd Name']=Time,User)`, []string{"Odd Name", "User"}},
		{`P=arg_max(State.Path)`, []string{"P"}},
		{`arg_min(State.Path) as P`, []string{"P"}},
		{`arg_max(Chosen=State.Path)`, []string{"Chosen"}},
		{`V=arg_max(Time, Id)`, []string{"V", "Id"}},
		{`arg_min(Time, Id)`, []string{"Time", "Id"}},
		{`arg_max(Time, Id)`, []string{"Time", "Id"}},
	} {
		t.Run(tc.item, func(t *testing.T) {
			q := `T | summarize ` + tc.item + ` | where ['` + tc.names[len(tc.names)-1] + `']=="x"`
			if errs := Validate(q); len(errs) != 0 {
				t.Fatalf("syntax: %v", errs)
			}
			p, s := ExtractConditions(q), AnalyzeQuery(q)
			if len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Aggregations, s.Aggregations) || !reflect.DeepEqual(p.Dataflow, s.Dataflow) {
				t.Fatalf("models: %+v / %+v", p, s)
			}
			requireAggregateResult(t, p, 0, tc.names)
			r := p.Aggregations[0].Result
			if p.Dataflow.Stages[0].Source != "summarize "+tc.item || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.BindingID != r.Outputs[len(tc.names)-1].BindingID {
				t.Fatalf("source/downstream: %+v", p)
			}
		})
	}
}

func TestArgMinMaxAllocation(t *testing.T) {
	p := ExtractConditions(`T | summarize arg_max(Time,Time,User) by Time,User`)
	requireAggregateResult(t, p, 0, []string{"Time1", "Time2", "User1"})
	for _, tc := range []struct {
		decl, name string
		visible    int
	}{{"Time", "Time", 1}, {"(Time)", "Time1", 2}, {"AS", "Time1", 2}} {
		item := tc.decl + `=arg_min(Other)`
		if tc.decl == "AS" {
			item = `arg_min(Other) as Time`
		}
		p := ExtractConditions(`T | summarize arg_max(Time),` + item + ` | where ` + tc.name + `>0`)
		requireAggregateResult(t, p, 1, []string{tc.name})
		if len(p.Dataflow.Scopes[1].BindingIDs) != tc.visible || len(p.Dataflow.Stages[0].BindingIDs) != 2 || p.Conditions[0].FieldBinding.BindingID != p.Aggregations[1].Result.Outputs[0].BindingID {
			t.Fatalf("allocation: %+v", p)
		}
	}
}

func TestArgMinMaxBoundaries(t *testing.T) {
	for _, item := range []string{`(A,B,C)=arg_max(Time,User)`, `arg_min(['bad\q']=Time)`} {
		q := `T | summarize ` + item
		if len(Validate(q)) == 0 || len(ExtractConditions(q).Errors) == 0 {
			t.Fatalf("invalid: %s", q)
		}
	}
	for _, item := range []string{`arg_max(State.Path)`, `(Peak)=arg_max(State.Path,User+1)`, `arg_min(Time,*)`, `arg_max(*,User)`, `arg_max(Time,User) as P`, `arg_min()`, `P=arg_max(['']=Time)`, `arg_max(Time),arg_min(State.Path)`} {
		q := `T | summarize ` + item
		if errs := Validate(q); len(errs) != 0 {
			t.Fatalf("opaque syntax %s: %v", q, errs)
		}
		p, s := ExtractConditions(q), AnalyzeQuery(q)
		for _, f := range []*QueryDataflow{p.Dataflow, s.Dataflow} {
			if f == nil || f.Complete || len(f.Bindings) != 0 {
				t.Fatalf("opaque %s: %+v", q, f)
			}
		}
		for _, a := range p.Aggregations {
			if a.Result == nil || a.Result.Complete || len(a.Result.Outputs) != 0 || a.StageID != "" || a.BindingID != "" {
				t.Fatalf("partial: %+v", a)
			}
		}
	}
}

func TestArgMinMaxNestedAtomic(t *testing.T) {
	q := `T | summarize count(),arg_max(Time,User)`
	p := ExtractConditions(q)
	requireAggregateResult(t, p, 1, []string{"Time", "User"})
	if len(p.Aggregations) != 2 || p.Aggregations[1].Result.CallOrdinal != 1 {
		t.Fatal("per-call inventory")
	}
	nested := ExtractConditions(`T | join (Other | summarize arg_min(Time,User)) on Key`)
	if len(nested.Joins) != 1 || nested.Joins[0].Subsearch == nil {
		t.Fatalf("nested: %+v", nested)
	}
	requireAggregateResult(t, nested.Joins[0].Subsearch, 0, []string{"Time", "User"})
	l := defaultParseLimits()
	l.maxOutputItems = 0
	if r := extractConditionsContext(context.Background(), q, l); len(r.Errors) != 1 || r.Dataflow != nil || len(r.Aggregations) != 0 {
		t.Fatal("partial quota result")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := AnalyzeQueryContext(ctx, q); len(r.Errors) != 1 || r.Dataflow != nil || len(r.Aggregations) != 0 {
		t.Fatal("partial canceled result")
	}
}
