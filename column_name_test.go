package kql

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestColumnNamePublic(t *testing.T) {
	for _, name := range []string{`['Alias Name']`, `['a]b']`, `['a\'b']`, `[@'a''b']`, `['Alias' // join
 ' Name']`, `[h'A' @'lias Name']`, `["""Alias Name"""]`} {
		t.Run(name, func(t *testing.T) {
			q := `T | extend ` + name + `=src | project ['Copy Name']=` + name + ` | project-rename ['Final Name']=['Copy Name'] | where ['Final Name']=="x"`
			if errs := Validate(q); len(errs) != 0 {
				t.Fatalf("Validate: %v", errs)
			}
			p, s := ExtractConditions(q), AnalyzeQuery(q)
			if len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) {
				t.Fatalf("models: %+v / %+v", p, s)
			}
			wantName := "Alias Name"
			if name == `['a]b']` {
				wantName = "a]b"
			}
			if name == `['a\'b']` || name == `[@'a''b']` {
				wantName = "a'b"
			}
			if len(p.Dataflow.Bindings) > 0 && p.Dataflow.Bindings[0].Field != wantName {
				t.Fatalf("decoded identity: %+v", p.Dataflow.Bindings[0])
			}
			if len(p.Dataflow.Bindings) != 3 {
				t.Fatalf("bindings: %+v", p.Dataflow.Bindings)
			}
			for i, want := range []string{"Copy Name", "Final Name"} {
				if p.Dataflow.Bindings[i+1].Field != want {
					t.Fatalf("binding: %+v", p.Dataflow.Bindings)
				}
			}
			if p.Dataflow.Bindings[0].Expression != "src" || p.Dataflow.Bindings[1].Expression != name || p.Dataflow.Bindings[2].Expression != `['Copy Name']` {
				t.Fatalf("source: %+v", p.Dataflow.Bindings)
			}
			if len(p.Conditions) != 1 || p.Conditions[0].Field != "Final Name" || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.BindingID != p.Dataflow.Bindings[2].ID {
				t.Fatalf("downstream: %+v", p.Conditions)
			}
			var extendSource string
			for _, stage := range p.Dataflow.Stages {
				if stage.Command == "extend" {
					extendSource = stage.Source
				}
			}
			if extendSource != "extend "+name+"=src" {
				t.Fatalf("stage source: %+v", p.Dataflow.Stages)
			}
		})
	}
}

func TestColumnNameSummarize(t *testing.T) {
	q := `T | summarize ['Metric Name']=count() by ['Group Name']=src | where ['Metric Name']>1`
	p, s := ExtractConditions(q), AnalyzeQuery(q)
	if errs := Validate(q); len(errs) != 0 {
		t.Fatal(errs)
	}
	if p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) || len(p.Aggregations) != 1 || p.Aggregations[0].Column != "Metric Name" || !reflect.DeepEqual(p.GroupByFields, []string{"Group Name"}) {
		t.Fatalf("summary: %+v", p)
	}
}

func TestColumnNameBoundaries(t *testing.T) {
	for _, q := range []string{`T | extend ['bad\q']=src`, `T | project [abc]=src`, `T | project-rename ['x']=[abc]`} {
		if len(Validate(q)) == 0 || len(ExtractConditions(q).Errors) == 0 || len(AnalyzeQuery(q).Errors) == 0 {
			t.Fatalf("invalid name accepted: %s", q)
		}
	}
	for _, q := range []string{`T | project ['']`, `T | summarize count() by ['']`, `T | where ['']=="x"`, `T | extend Good=src, ['']=other`, `T | project Good=src, ['']=other`, `T | project-rename Good=src, ['']=other`, `T | summarize Good=count(), ['']=count()`, `T | summarize count() by Good=src, ['']=other`} {
		if errs := Validate(q); len(errs) != 0 {
			t.Fatalf("empty syntax: %s: %v", q, errs)
		}
		p, s := ExtractConditions(q), AnalyzeQuery(q)
		for _, flow := range []*QueryDataflow{p.Dataflow, s.Dataflow} {
			if flow == nil || len(flow.Stages) != 1 || flow.Stages[0].Effect != "opaque" || len(flow.Stages[0].BindingIDs) != 0 || len(flow.Bindings) != 0 {
				t.Fatalf("partial empty stage %s: %+v", q, flow)
			}
		}
		if len(p.ComputedFields) != 0 || len(p.ProjectedFields) != 0 || len(p.GroupByFields) != 0 || len(p.Aggregations) != 0 {
			t.Fatalf("partial legacy metadata %s: %+v", q, p)
		}
	}
}

func TestColumnNameSharedBudgetBoundaries(t *testing.T) {
	for _, query := range []string{
		`T | extend ['Alias' @' Name']=src | where ['Alias Name']=="x"`,
	} {
		for _, semantic := range []bool{false, true} {
			var used parseBudget
			b, cancel := newParseBudget(context.Background(), defaultParseLimits())
			p := extractConditionsInternal(query, b)
			if semantic {
				_ = semanticFromExtraction(p, b)
			}
			used = *b
			cancel()
			if len(p.Errors) > 0 || p.Dataflow == nil || !p.Dataflow.Complete {
				t.Fatalf("invalid resource fixture: %+v", p)
			}
			// Legacy outputSize bounds each returned condition inventory in
			// addition to the cumulative owned-output reservation counter.
			var returnedCount func(*ParseResult) int
			returnedCount = func(r *ParseResult) int {
				n := len(r.Conditions)
				for _, join := range r.Joins {
					if join.Subsearch != nil {
						n = max(n, returnedCount(join.Subsearch))
					}
				}
				return n
			}
			for _, resource := range []struct {
				name string
				used int
				set  func(*parseLimits, int)
			}{
				{"work budget", used.work, func(l *parseLimits, n int) { l.maxWork = n }},

				{"metadata items", used.metadataItems, func(l *parseLimits, n int) { l.maxMetadataItems = n }},
				{"metadata bytes", used.metadataBytes, func(l *parseLimits, n int) { l.maxMetadataBytes = n }},
			} {
				run := func(limits parseLimits) (bool, []string) {
					if semantic {
						r := analyzeQueryContext(context.Background(), query, limits)
						return len(r.Conditions) > 0 || r.Expression != nil || r.Dataflow != nil || len(r.DataSources) > 0, r.Errors
					}
					r := extractConditionsContext(context.Background(), query, limits)
					return len(r.Conditions) > 0 || r.Expression != nil || r.Dataflow != nil || len(r.DataSources) > 0, r.Errors
				}
				limits := defaultParseLimits()
				resource.set(&limits, resource.used)
				if hasModel, errs := run(limits); !hasModel || len(errs) > 0 {
					t.Fatalf("exact %s=%d/semantic=%v rejected: %v; query=%s counters=%+v returned=%d", resource.name, resource.used, semantic, errs, query, used, returnedCount(p))
				}
				resource.set(&limits, resource.used-1)
				if hasModel, errs := run(limits); hasModel || !strings.Contains(strings.Join(errs, " "), resource.name) {
					t.Fatalf("one-short %s/semantic=%v not atomic: model=%v errors=%v", resource.name, semantic, hasModel, errs)
				}
			}
		}
	}
}

func TestColumnNameOutputAtomic(t *testing.T) {
	const query = `T | extend ['Alias' @' Name']=src | where ['Alias Name']=="x"`
	limits := defaultParseLimits()
	limits.maxOutputItems = 1
	p, s := extractConditionsContext(context.Background(), query, limits), analyzeQueryContext(context.Background(), query, limits)
	if len(p.Errors) != 1 || len(s.Errors) != 1 || !strings.Contains(p.Errors[0], "output items") || !strings.Contains(s.Errors[0], "output items") || p.Dataflow != nil || s.Dataflow != nil || p.Expression != nil || s.Expression != nil || len(p.Conditions)+len(s.Conditions)+len(p.ComputedFields)+len(s.ComputedFields) != 0 {
		t.Fatalf("partial output failure: %+v / %+v", p, s)
	}
}

func TestColumnNameCloneAndCancel(t *testing.T) {
	const query = `T | extend ['Alias Name']=src | where ['Alias Name']=="x"`
	p := ExtractConditions(query)
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	if p.Dataflow == nil {
		t.Fatalf("clone premise: %+v", p)
	}
	s := semanticFromExtraction(p, b)
	s.Dataflow.Bindings[0].Field = "changed"
	s.Dataflow.Stages[0].Source = "changed"
	if p.Dataflow.Bindings[0].Field != "Alias Name" || p.Dataflow.Stages[0].Source == "changed" {
		t.Fatal("shared clone")
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if len(ValidateContext(ctx, query)) == 0 || len(ExtractConditionsContext(ctx, query).Errors) != 1 || len(AnalyzeQueryContext(ctx, query).Errors) != 1 {
		t.Fatal("cancellation ignored")
	}
}

func TestColumnNameAsAndCollision(t *testing.T) {
	for _, query := range []string{
		`T | extend src as ['Alias Name'] | project ['Alias Name'] as ['Copy Name'] | where ['Copy Name']=="x"`,
		`T | summarize count() as ['Metric Name'] by src as ['Group Name'] | where ['Metric Name']>1`,
		`T | summarize ['count_']=count(), count() | where ['count_1']>1`,
	} {
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		if len(Validate(query))+len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) {
			t.Fatalf("AS/collision: %s: %+v", query, p)
		}
		if len(p.Conditions) != 1 || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.Kind != "binding" {
			t.Fatalf("AS/collision binding: %+v", p.Conditions)
		}
	}
}

func TestColumnNameAbsenceProof(t *testing.T) {
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	const query = `T | where x==1`
	raw, normalized := preflight(query, b, false, nil), preflight(query, b, true, nil)
	if !columnNamesAbsent(raw, normalized, query, query, b) {
		t.Fatal("complete ordinary streams did not prove absence")
	}
	normalized.GetTokenSource().(*budgetLexer).complete = false
	if columnNamesAbsent(raw, normalized, query, query, b) || columnNamesAbsent(nil, raw, query, query, b) {
		t.Fatal("incomplete inventory proved absence")
	}
	other, stop := newParseBudget(context.Background(), defaultParseLimits())
	defer stop()
	if columnNamesAbsent(raw, raw, query, query, other) || columnNamesAbsent(raw, raw, "foreign", query, b) {
		t.Fatal("foreign inventory proved absence")
	}
	quoted := preflight(`T | extend ['Name']=x`, b, false, nil)
	if columnNamesAbsent(quoted, raw, `T | extend ['Name']=x`, query, b) {
		t.Fatal("raw quoted name disappeared from absence proof")
	}
}

func TestColumnNameInputAtomic(t *testing.T) {
	const query = `T | extend ['Alias' @' Name']=src | where ['Alias Name']=="x"`
	limits := defaultParseLimits()
	limits.maxInputBytes = len(query) - 1
	p, s := extractConditionsContext(context.Background(), query, limits), analyzeQueryContext(context.Background(), query, limits)
	if len(p.Errors) != 1 || len(s.Errors) != 1 || !strings.Contains(p.Errors[0], "input bytes") || !strings.Contains(s.Errors[0], "input bytes") || p.Dataflow != nil || s.Dataflow != nil || p.Expression != nil || s.Expression != nil || len(p.Conditions)+len(s.Conditions)+len(p.ComputedFields)+len(s.ComputedFields) != 0 {
		t.Fatalf("partial input failure: %+v / %+v", p, s)
	}
}

func TestColumnNameCommentNewlines(t *testing.T) {
	for _, tc := range []struct{ name, newline string }{{"CR", "\r"}, {"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			name := `['A' // c` + tc.newline + `' B']`
			query := `T | extend ` + name + `=src | where ['A B']=="x"`
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			if errs := Validate(query); len(errs) != 0 {
				t.Fatalf("syntax: %v", errs)
			}
			if len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) {
				t.Fatalf("models: %+v / %+v", p, s)
			}
			if len(p.Dataflow.Bindings) != 1 || p.Dataflow.Bindings[0].Field != "A B" || p.Dataflow.Bindings[0].Expression != "src" || p.Dataflow.Stages[0].Source != "extend "+name+"=src" {
				t.Fatalf("source ownership: %+v", p.Dataflow)
			}
			if len(p.Conditions) != 1 || p.Conditions[0].Field != "A B" || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.BindingID != p.Dataflow.Bindings[0].ID || p.Conditions[0].InputScopeID != p.Dataflow.Stages[0].OutputScopeID {
				t.Fatalf("downstream binding: %+v", p.Conditions)
			}
		})
	}
}
