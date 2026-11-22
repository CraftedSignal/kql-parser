package kql

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestWave4kIntegerGroupBoundary(t *testing.T) {
	for _, tc := range []struct {
		metrics, groups string
		legacy          []string
		expressions     []string
		opaque          bool
	}{
		{"count()", "42", []string{"42"}, nil, true},
		{"count()", "Known, 42", []string{"Known", "42"}, nil, true},
		{"sum(Bytes)", "42", []string{"42"}, nil, true},
		{"count(), sum(Bytes)", "42, Known", []string{"42", "Known"}, nil, true},
		{"count()", "0", []string{"0"}, nil, true},
		{"count()", "9007199254740993", []string{"9007199254740993"}, nil, true},
		{"count()", "Known", []string{"Known"}, []string{"Known"}, false},
		{"count()", "3plogTime", []string{"3plogTime"}, []string{"3plogTime"}, false},
		{"count()", "G=42", []string{"G"}, []string{"42"}, false},
		{"count()", "42 as G", []string{"G"}, []string{"42"}, false},
		{"count()", "['42']", []string{"42"}, []string{"['42']"}, false},
		{"count()", "Known, ['42']", []string{"Known", "42"}, []string{"Known", "['42']"}, false},
		{"count()", "4.2", nil, nil, true},
		{"count()", `"x"`, nil, nil, true},
		{"count()", "'x'", nil, nil, true},
	} {
		t.Run(tc.metrics+" by "+tc.groups, func(t *testing.T) {
			source := "summarize " + tc.metrics + " by " + tc.groups
			p, s := ExtractConditions("T | "+source), AnalyzeQuery("T | "+source)
			for _, api := range []struct {
				flow             *QueryDataflow
				groups           []string
				aggregates       []Aggregation
				errors, warnings []string
			}{{p.Dataflow, p.GroupByFields, p.Aggregations, p.Errors, p.Warnings}, {s.Dataflow, s.GroupByFields, s.Aggregations, s.Errors, s.Warnings}} {
				if len(api.errors) != 0 || api.flow == nil || len(api.flow.Stages) != 1 {
					t.Fatalf("parse/graph: %+v", api)
				}
				stage := api.flow.Stages[0]
				if stage.Source != source || !reflect.DeepEqual(api.groups, tc.legacy) || len(api.aggregates) == 0 {
					t.Fatalf("source/legacy changed: %+v", api)
				}
				if api.aggregates[0].Function != strings.Split(tc.metrics, "(")[0] || api.aggregates[0].Column == "" {
					t.Errorf("legacy summary: %+v", api.aggregates)
				}
				scope := api.flow.Scopes[len(api.flow.Scopes)-1]
				if scope.ID != api.flow.OutputScopeID || scope.ID != stage.OutputScopeID {
					t.Fatalf("scope ownership: %+v", api.flow)
				}
				if tc.opaque {
					if api.flow.Complete || stage.Effect != "opaque" || scope.Mode != "opaque" || len(api.flow.Bindings) != 0 || len(stage.BindingIDs) != 0 || len(scope.BindingIDs) != 0 {
						t.Errorf("literal fabricated finite outputs: %+v", api.flow)
					}
					for _, a := range api.aggregates {
						if a.StageID != "" || a.BindingID != "" {
							t.Errorf("legacy binding leaked: %+v", a)
						}
					}
					if len(api.warnings) != 1 || api.warnings[0] != "dataflow:unsupported_stage_dataflow: unrepresented summarize output" {
						t.Errorf("diagnostic: %v", api.warnings)
					}
				} else {
					if !api.flow.Complete || stage.Effect != "aggregate" || scope.Mode != "closed" || len(api.warnings) != 0 || len(api.flow.Bindings) != len(tc.legacy)+len(api.aggregates) {
						t.Fatalf("supported group lost: %+v", api)
					}
					for i, field := range tc.legacy {
						b := api.flow.Bindings[len(api.aggregates)+i]
						if b.Field != field || b.Kind != "group" || b.Expression != tc.expressions[i] {
							t.Errorf("group identity: %+v", b)
						}
					}
					for _, a := range api.aggregates {
						if a.StageID != stage.ID || a.BindingID == "" {
							t.Errorf("aggregate owner: %+v", a)
						}
					}
				}
			}
		})
	}
}

func TestWave4kIntegerGroupLaterReferenceAndClone(t *testing.T) {
	const source = "summarize count() by Known, 42"
	const query = `T | summarize count() by Known, 42 | where Known == "x" and count_ > 1`
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	for _, api := range []struct {
		flow       *QueryDataflow
		conditions []Condition
		errors     []string
	}{{p.Dataflow, p.Conditions, p.Errors}, {s.Dataflow, s.Conditions, s.Errors}} {
		if len(api.errors) != 0 || api.flow == nil || api.flow.Complete || len(api.flow.Bindings) != 0 || len(api.conditions) != 2 {
			t.Fatalf("opaque later stage: %+v", api)
		}
		for i, c := range api.conditions {
			if c.Field != []string{"Known", "count_"}[i] || c.FieldBinding == nil || c.FieldBinding.Kind != "unknown" || c.FieldBinding.BindingID != "" {
				t.Errorf("invented downstream binding: %+v", c)
			}
		}
		if api.flow.Stages[0].Source != source || len(api.flow.Stages[0].BindingIDs) != 0 {
			t.Errorf("partial summarize leaked: %+v", api.flow)
		}
	}
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	clone := semanticFromExtraction(p, b)
	cancel()
	p.GroupByFields[1] = "mutated"
	p.Aggregations[0].Column = "mutated"
	p.Dataflow.Stages[0].Source = "mutated"
	p.Conditions[0].FieldBinding.ScopeID = "mutated"
	if clone.GroupByFields[1] != "42" || clone.Aggregations[0].Column != "count_" || clone.Dataflow.Stages[0].Source != source || clone.Conditions[0].FieldBinding.ScopeID == "mutated" {
		t.Fatal("semantic model aliases legacy/source/reference")
	}
}

func TestWave4kIntegerGroupSharedBudgets(t *testing.T) {
	for _, query := range []string{`T | summarize count() by Known, 42`, `T | summarize count() by G=42`, `T | join (U | summarize count() by Known, 42) on Key | project Known`} {
		for _, semantic := range []bool{false, true} {
			b, cancel := newParseBudget(context.Background(), defaultParseLimits())
			p := extractConditionsInternal(query, b)
			if semantic {
				_ = semanticFromExtraction(p, b)
			}
			used := *b
			cancel()
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
						return r.Dataflow != nil || len(r.GroupByFields) > 0 || len(r.Aggregations) > 0 || len(r.Conditions) > 0, r.Errors
					}
					r := extractConditionsContext(context.Background(), query, l)
					return r.Dataflow != nil || len(r.GroupByFields) > 0 || len(r.Aggregations) > 0 || len(r.Conditions) > 0, r.Errors
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
		p, s := extractConditionsContext(context.Background(), query, l), analyzeQueryContext(context.Background(), query, l)
		if p.Dataflow != nil || s.Dataflow != nil || len(p.GroupByFields) > 0 || len(p.Aggregations) > 0 || len(s.GroupByFields) > 0 || len(s.Aggregations) > 0 || !strings.Contains(strings.Join(p.Errors, " "), "output items") || !strings.Contains(strings.Join(s.Errors, " "), "output items") {
			t.Fatalf("output budget leaked model: %+v / %+v", p, s)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		p, s = ExtractConditionsContext(ctx, query), AnalyzeQueryContext(ctx, query)
		if p.Dataflow != nil || s.Dataflow != nil || len(p.GroupByFields) > 0 || len(p.Aggregations) > 0 || len(s.GroupByFields) > 0 || len(s.Aggregations) > 0 || !strings.Contains(strings.Join(p.Errors, " "), "context canceled") || !strings.Contains(strings.Join(s.Errors, " "), "context canceled") {
			t.Fatalf("cancellation leaked model: %+v / %+v", p, s)
		}
	}
}
