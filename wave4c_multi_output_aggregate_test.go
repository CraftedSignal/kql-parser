package kql

import (
	"context"
	"strings"
	"testing"
)

func TestWave4cMultiOutputAggregatesOpaque(t *testing.T) {
	for _, tc := range []struct {
		call, function, field, column string
		count                         int
	}{
		{"arg_max(Time, *)", "arg_max", "Time", "arg_max_Time", 1},
		{"V=arg_max(Time, Id)", "arg_max", "Time", "V", 1},
		{"argmax(Time, Id)", "argmax", "Time", "argmax_Time", 1},
		{"arg_min(Time, Id)", "arg_min", "Time", "arg_min_Time", 1},
		{"argmin(Time, Id)", "argmin", "Time", "argmin_Time", 1},
		{"V=take_any(*)", "take_any", "", "V", 1},
		{"any(*)", "any", "", "any_", 1},
		{"any(Name, Value)", "any", "Name", "any_Name", 1},
		{"arg_max(Time, Id)", "arg_max", "Time", "arg_max_Time", 1},
		{"take_any(*)", "take_any", "", "take_any_", 1},
		{"take_any(Name, Value) as V", "take_any", "Name", "V", 1},
		{"count(), arg_max(Time, Id)", "arg_max", "Time", "arg_max_Time", 2},
	} {
		t.Run(tc.call, func(t *testing.T) {
			query := "Events | summarize " + tc.call + " by User"
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			for _, api := range []struct {
				flow                     *QueryDataflow
				aggs                     []Aggregation
				errors, warnings, groups []string
			}{
				{p.Dataflow, p.Aggregations, p.Errors, p.Warnings, p.GroupByFields}, {s.Dataflow, s.Aggregations, s.Errors, s.Warnings, s.GroupByFields},
			} {
				if len(api.errors) != 0 || len(api.warnings) != 1 || api.warnings[0] != "dataflow:unsupported_stage_dataflow: unrepresented summarize output" || api.flow == nil || len(api.flow.Stages) != 1 || len(api.aggs) != tc.count {
					t.Fatalf("valid native parse/source: %+v", api)
				}
				stage := api.flow.Stages[0]
				if api.flow.Complete || stage.Effect != "opaque" || len(stage.BindingIDs) != 0 || len(api.flow.Bindings) != 0 {
					t.Errorf("schema/multi-output stage claims fabricated finite outputs: %+v", api.flow)
				}
				if stage.Source != "summarize "+tc.call+" by User" || len(api.groups) != 1 || api.groups[0] != "User" {
					t.Errorf("authored source/legacy group changed: %+v", api)
				}
				if len(api.flow.Scopes) != 2 || api.flow.Scopes[0].Source != "Events" || api.flow.Scopes[1].Mode != "opaque" || api.flow.Scopes[1].ID != stage.OutputScopeID {
					t.Errorf("source/output scope ownership: %+v", api.flow)
				}
				found := false
				for _, d := range api.flow.Diagnostics {
					if d.Code == "unsupported_stage_dataflow" && d.StageID == stage.ID {
						found = true
					}
				}
				if !found {
					t.Errorf("missing opaque-stage diagnostic: %+v", api.flow)
				}
				for _, a := range api.aggs {
					if a.StageID != "" || a.BindingID != "" {
						t.Errorf("legacy summary claims finite ownership: %+v", a)
					}
				}
				last := api.aggs[len(api.aggs)-1]
				if last.Function != tc.function || last.Field != tc.field || last.Column != tc.column || last.PipeStage != 0 || last.Pipeline != 0 {
					t.Errorf("legacy metadata changed: %+v", last)
				}
				if tc.count == 2 && (api.aggs[0].Function != "count" || api.aggs[0].Column != "count_") {
					t.Errorf("mixed scalar sibling summary changed: %+v", api.aggs)
				}
			}
		})
	}
}
func TestWave4cSingleOutputAggregateControls(t *testing.T) {
	for _, tc := range []struct{ call, column, field string }{
		{"take_any(Name)", "Name", "Name"},
		{"count()", "count_", ""},
		{"sum(Bytes)", "sum_Bytes", "Bytes"},
		{"V=take_any(Name)", "V", "Name"},
		{"take_any(Name) as V", "V", "Name"},
		{"any(Name)", "any_Name", "Name"},
		{"V=any(Name)", "V", "Name"},
		{"V=take_any(Bytes * Factor)", "V", ""},
		{`V=take_any("*")`, "V", ""},
		{"V=take_anyif(Name, IsActive)", "V", "Name"},
	} {
		t.Run(tc.call, func(t *testing.T) {
			query := "Events | summarize " + tc.call + " by User"
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			for _, api := range []struct {
				flow             *QueryDataflow
				aggs             []Aggregation
				errors, warnings []string
			}{
				{p.Dataflow, p.Aggregations, p.Errors, p.Warnings}, {s.Dataflow, s.Aggregations, s.Errors, s.Warnings},
			} {
				if len(api.errors)+len(api.warnings) != 0 || api.flow == nil || !api.flow.Complete || len(api.flow.Bindings) != 2 || len(api.aggs) != 1 {
					t.Fatalf("single-output control %s: %+v", query, api)
				}
				a := api.aggs[0]
				if a.Column != tc.column || a.Field != tc.field || a.StageID == "" || a.BindingID == "" {
					t.Errorf("legacy/owned control identity: %+v", a)
				}
			}
		})
	}
}

func TestWave4cMultiOutputSharedBudgets(t *testing.T) {
	for _, query := range []string{
		`Events | summarize count(), arg_max(Time, Id) by User`,
		`Events | summarize V=take_any(Name) by User`,
		`Events | join (Other | summarize take_any(*) by User) on User | summarize count()`,
	} {
		for _, semantic := range []bool{false, true} {
			b, cancel := newParseBudget(context.Background(), defaultParseLimits())
			p := extractConditionsInternal(query, b)
			if semantic {
				_ = semanticFromExtraction(p, b)
			}
			used := *b
			cancel()
			// Join output completeness remains its existing unsupported boundary; this
			// control tests only the shared recursive budget and atomic failures.
			if len(p.Errors) != 0 || p.Dataflow == nil {
				t.Fatalf("resource control %q: %+v", query, p)
			}
			for _, resource := range []struct {
				name string
				used int
				set  func(*parseLimits, int)
			}{
				{"work budget", used.work, func(l *parseLimits, n int) { l.maxWork = n }},
				{"metadata bytes", used.metadataBytes, func(l *parseLimits, n int) { l.maxMetadataBytes = n }},
			} {
				run := func(l parseLimits) (bool, []string) {
					if semantic {
						r := analyzeQueryContext(context.Background(), query, l)
						return r.Dataflow != nil || len(r.Aggregations) > 0 || len(r.Conditions) > 0, r.Errors
					}
					r := extractConditionsContext(context.Background(), query, l)
					return r.Dataflow != nil || len(r.Aggregations) > 0 || len(r.Conditions) > 0, r.Errors
				}
				l := defaultParseLimits()
				resource.set(&l, resource.used)
				if model, errs := run(l); !model || len(errs) > 0 {
					t.Fatalf("exact %s=%d semantic=%v: %v", resource.name, resource.used, semantic, errs)
				}
				resource.set(&l, resource.used-1)
				if model, errs := run(l); model || !strings.Contains(strings.Join(errs, " "), resource.name) {
					t.Fatalf("one-short %s semantic=%v: model=%v errors=%v", resource.name, semantic, model, errs)
				}
			}
		}
		l := defaultParseLimits()
		l.maxOutputItems = 0
		p0, s0 := extractConditionsContext(context.Background(), query, l), analyzeQueryContext(context.Background(), query, l)
		if p0.Dataflow != nil || s0.Dataflow != nil || len(p0.Aggregations) > 0 || len(s0.Aggregations) > 0 || !strings.Contains(strings.Join(p0.Errors, " "), "output items") || !strings.Contains(strings.Join(s0.Errors, " "), "output items") {
			t.Fatalf("output limit leaked model: %+v / %+v", p0, s0)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		p, s := ExtractConditionsContext(ctx, query), AnalyzeQueryContext(ctx, query)
		if p.Dataflow != nil || s.Dataflow != nil || len(p.Aggregations) > 0 || len(s.Aggregations) > 0 || !strings.Contains(strings.Join(p.Errors, " "), "context canceled") || !strings.Contains(strings.Join(s.Errors, " "), "context canceled") {
			t.Fatalf("cancellation leaked model: %+v / %+v", p, s)
		}
	}
}

func TestWave4cMultiOutputLaterReferenceAndClone(t *testing.T) {
	const query = "Events | summarize count(), arg_max(Time, Id) by User | where Id == 1"
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	for _, api := range []struct {
		flow       *QueryDataflow
		conditions []Condition
		errors     []string
	}{
		{p.Dataflow, p.Conditions, p.Errors}, {s.Dataflow, s.Conditions, s.Errors},
	} {
		if len(api.errors) != 0 || api.flow == nil || api.flow.Complete || len(api.flow.Bindings) != 0 || len(api.conditions) != 1 {
			t.Fatalf("opaque later reference: %+v", api)
		}
		c := api.conditions[0]
		if c.Field != "Id" || c.Operator != "==" || c.Value != "1" || c.FieldBinding == nil || c.FieldBinding.Kind != "unknown" || c.FieldBinding.BindingID != "" {
			t.Errorf("invented finite output reference: %+v", c)
		}
	}
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	clone := semanticFromExtraction(p, b)
	cancel()
	p.Aggregations[1].Column = "mutated"
	p.Dataflow.Stages[0].Source = "mutated"
	p.Conditions[0].FieldBinding.ScopeID = "mutated"
	if clone.Aggregations[1].Column != "arg_max_Time" || clone.Dataflow.Stages[0].Source != "summarize count(), arg_max(Time, Id) by User" || clone.Conditions[0].FieldBinding.ScopeID == "mutated" {
		t.Fatal("semantic model aliases extracted metadata/source/reference")
	}
}
