package kql

import (
	"context"
	"strings"
	"testing"
)

func TestWave4cPercentileOutputBoundary(t *testing.T) {
	for _, tc := range []struct {
		call, function, column string
		opaque                 bool
	}{
		{"percentiles(Bytes,5,50,95)", "percentiles", "percentiles_Bytes", true},
		{"P=percentiles(Bytes,5,50,95)", "percentiles", "P", true},
		{"percentiles(Bytes,95)", "percentiles", "percentiles_Bytes", true},
		{"P=percentiles(Bytes,95)", "percentiles", "P", false},
		{"percentilesw(Bytes,Weight,95)", "percentilesw", "percentilesw_Bytes", true},
		{"P=percentilesw(Bytes,Weight,95)", "percentilesw", "P", false},
		{"P=percentilesw(Bytes,Weight,5,50,95)", "percentilesw", "P", true},
		{"percentilesw(Bytes,Weight,5,50,95)", "percentilesw", "percentilesw_Bytes", true},
		{"percentile(Bytes,95)", "percentile", "percentile_Bytes", true},
		{"P=percentile(Bytes,95)", "percentile", "P", false},
		{"percentile(Bytes,95) as P", "percentile", "P", false},
		{"percentiles(Bytes,95) as P", "percentiles", "P", false},
		{"percentiles_array(Bytes,5,50,95)", "percentiles_array", "percentiles_array_Bytes", true},
		{"A=percentiles_array(Bytes,5,50,95)", "percentiles_array", "A", false},
		{"A=percentiles_array(Bytes,dynamic([5,50,95]))", "percentiles_array", "A", false},
		{"percentiles_array(Bytes,dynamic([5,50,95]))", "percentiles_array", "percentiles_array_Bytes", true},
		{"count()", "count", "count_", false},
		{"sum(Bytes)", "sum", "sum_Bytes", false},
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
				if len(api.errors) != 0 || api.flow == nil || len(api.flow.Stages) != 1 || len(api.aggs) != 1 {
					t.Fatalf("native parse/source: %+v", api)
				}
				a, stage := api.aggs[0], api.flow.Stages[0]
				if a.Function != tc.function || a.Column != tc.column || (tc.function != "count" && a.Field != "Bytes") || stage.Source != "summarize "+tc.call+" by User" {
					t.Errorf("legacy summary/source changed: %+v / %+v", a, stage)
				}
				if tc.opaque {
					if api.flow.Complete || stage.Effect != "opaque" || len(api.flow.Bindings) != 0 || len(stage.BindingIDs) != 0 || a.StageID != "" || a.BindingID != "" {
						t.Errorf("unproved output shape/name claims finite graph: %+v / %+v", api.flow, a)
					}
					if len(api.warnings) != 1 || api.warnings[0] != "dataflow:unsupported_stage_dataflow: unrepresented summarize output" {
						t.Errorf("expected exact opaque diagnostic: %+v", api.warnings)
					}
				} else if !api.flow.Complete || len(api.flow.Bindings) != 2 || len(api.warnings) != 0 || a.StageID == "" || a.BindingID == "" {
					t.Errorf("explicit scalar/array/count/sum control lost finite identity: %+v", api)
				}
			}
		})
	}
}

func TestWave4cPercentileSharedBudgets(t *testing.T) {
	for _, query := range []string{
		`Events | summarize count(), percentiles(Bytes,5,50,95) by User`,
		`Events | summarize A=percentiles_array(Bytes,dynamic([5,50,95])) by User`,
		`Events | join (Other | summarize percentile(Bytes,95) by User) on User | summarize P=percentiles(Bytes,95)`,
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

func TestWave4cPercentileMixedStageAndClone(t *testing.T) {
	const query = "Events | summarize count(), percentiles(Bytes,5,50,95) by User | where percentile_Bytes_95 > 1"
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	for _, api := range []struct {
		flow       *QueryDataflow
		aggs       []Aggregation
		conditions []Condition
		errors     []string
	}{
		{p.Dataflow, p.Aggregations, p.Conditions, p.Errors}, {s.Dataflow, s.Aggregations, s.Conditions, s.Errors},
	} {
		if len(api.errors) != 0 || api.flow == nil || api.flow.Complete || len(api.flow.Bindings) != 0 || len(api.aggs) != 2 || len(api.conditions) != 1 {
			t.Fatalf("opaque mixed stage/metadata: %+v", api)
		}
		for _, a := range api.aggs {
			if a.StageID != "" || a.BindingID != "" {
				t.Errorf("partial mixed aggregate ownership: %+v", a)
			}
		}
		if api.aggs[0].Column != "count_" || api.aggs[1].Column != "percentiles_Bytes" || api.aggs[1].Field != "Bytes" || api.aggs[1].Function != "percentiles" {
			t.Errorf("legacy sibling/percentile summary changed: %+v", api.aggs)
		}
		c := api.conditions[0]
		if c.Field != "percentile_Bytes_95" || c.Operator != ">" || c.Value != "1" || c.FieldBinding == nil || c.FieldBinding.Kind != "unknown" || c.FieldBinding.BindingID != "" {
			t.Errorf("invented finite output reference: %+v", c)
		}
	}
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	clone := semanticFromExtraction(p, b)
	cancel()
	p.Aggregations[1].Column = "mutated"
	p.Dataflow.Stages[0].Source = "mutated"
	p.Conditions[0].FieldBinding.ScopeID = "mutated"
	if clone.Aggregations[1].Column != "percentiles_Bytes" || clone.Dataflow.Stages[0].Source != "summarize count(), percentiles(Bytes,5,50,95) by User" || clone.Conditions[0].FieldBinding.ScopeID == "mutated" {
		t.Fatal("semantic model aliases extracted summary/source/reference")
	}
}
