package kql

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestWave4jLiteralProjectionBoundary(t *testing.T) {
	for _, tc := range []struct {
		items       string
		fields      []string
		expressions []string
		kinds       []string
		opaque      bool
	}{
		{"42", []string{"42"}, nil, nil, true},
		{"Known, 42", []string{"Known", "42"}, nil, nil, true},
		{"X=42, 42", []string{"X", "42"}, nil, nil, true},
		{"0", []string{"0"}, nil, nil, true},
		{"9007199254740993", []string{"9007199254740993"}, nil, nil, true},
		{"Known", []string{"Known"}, []string{"Known"}, []string{"passthrough"}, false},
		{"3plogTime", []string{"3plogTime"}, []string{"3plogTime"}, []string{"passthrough"}, false},
		{"X=42", []string{"X"}, []string{"42"}, []string{"assignment"}, false},
		{"42 as X", []string{"X"}, []string{"42"}, []string{"assignment"}, false},
		{"['42']", []string{"42"}, []string{"['42']"}, []string{"passthrough"}, false},
		{"Known, ['42']", []string{"Known", "42"}, []string{"Known", "['42']"}, []string{"passthrough", "passthrough"}, false},
		{"4.2", nil, nil, nil, true},
		{`"x"`, nil, nil, nil, true},
		{"'x'", nil, nil, nil, true},
	} {
		t.Run(tc.items, func(t *testing.T) {
			query := "T | project " + tc.items
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			for _, api := range []struct {
				flow             *QueryDataflow
				projected        []string
				errors, warnings []string
			}{{p.Dataflow, p.ProjectedFields, p.Errors, p.Warnings}, {s.Dataflow, s.ProjectedFields, s.Errors, s.Warnings}} {
				if len(api.errors) != 0 || api.flow == nil || len(api.flow.Stages) != 1 {
					t.Fatalf("parse/graph: %+v", api)
				}
				stage := api.flow.Stages[0]
				if stage.Source != "project "+tc.items || !reflect.DeepEqual(api.projected, tc.fields) {
					t.Errorf("authored source/legacy projection changed: %+v", api)
				}
				scope := api.flow.Scopes[len(api.flow.Scopes)-1]
				if scope.ID != api.flow.OutputScopeID || scope.ID != stage.OutputScopeID {
					t.Fatalf("output scope ownership: %+v", api.flow)
				}
				if tc.opaque {
					if api.flow.Complete || stage.Effect != "opaque" || scope.Mode != "opaque" || len(api.flow.Bindings) != 0 || len(stage.BindingIDs) != 0 || len(scope.BindingIDs) != 0 {
						t.Errorf("unaliased scalar fabricated finite output: %+v", api.flow)
					}
					if len(api.warnings) != 1 || api.warnings[0] != "dataflow:unsupported_stage_dataflow: unrepresented project output" {
						t.Errorf("opaque diagnostic: %v", api.warnings)
					}
				} else {
					if !api.flow.Complete || stage.Effect != "project" || scope.Mode != "closed" || len(api.warnings) != 0 || len(api.flow.Bindings) != len(tc.fields) {
						t.Fatalf("supported reference/alias lost: %+v", api)
					}
					for i, binding := range api.flow.Bindings {
						if binding.Field != tc.fields[i] || binding.Kind != tc.kinds[i] || binding.Expression != tc.expressions[i] {
							t.Errorf("native reference/alias identity: %+v", binding)
						}
					}
				}
			}
		})
	}
}

func TestWave4jLiteralProjectionLaterReferenceAndClone(t *testing.T) {
	const source = "project Known, 42"
	const query = `T | project Known, 42 | where Known == "x" and ['42'] == 1`
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
			want := []string{"Known", "42"}[i]
			if c.Field != want || c.FieldBinding == nil || c.FieldBinding.Kind != "unknown" || c.FieldBinding.BindingID != "" {
				t.Errorf("invented downstream field binding: %+v", c)
			}
		}
		if api.flow.Stages[0].Source != source || len(api.flow.Stages[0].BindingIDs) != 0 {
			t.Errorf("partial projection leaked: %+v", api.flow)
		}
	}
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	clone := semanticFromExtraction(p, b)
	cancel()
	p.ProjectedFields[1] = "mutated"
	p.Dataflow.Stages[0].Source = "mutated"
	p.Conditions[0].FieldBinding.ScopeID = "mutated"
	if clone.ProjectedFields[1] != "42" || clone.Dataflow.Stages[0].Source != source || clone.Conditions[0].FieldBinding.ScopeID == "mutated" {
		t.Fatal("semantic model aliases legacy/source/reference")
	}
}

func TestWave4jLiteralProjectionSharedBudgets(t *testing.T) {
	for _, query := range []string{`T | project Known, 42`, `T | project X=42`, `T | join (U | project Known, 42) on Key | project Known`} {
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
						return r.Dataflow != nil || len(r.ProjectedFields) > 0 || len(r.Conditions) > 0, r.Errors
					}
					r := extractConditionsContext(context.Background(), query, l)
					return r.Dataflow != nil || len(r.ProjectedFields) > 0 || len(r.Conditions) > 0, r.Errors
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
		if p.Dataflow != nil || s.Dataflow != nil || len(p.ProjectedFields) > 0 || len(s.ProjectedFields) > 0 || !strings.Contains(strings.Join(p.Errors, " "), "output items") || !strings.Contains(strings.Join(s.Errors, " "), "output items") {
			t.Fatalf("output budget leaked model: %+v / %+v", p, s)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		p, s = ExtractConditionsContext(ctx, query), AnalyzeQueryContext(ctx, query)
		if p.Dataflow != nil || s.Dataflow != nil || len(p.ProjectedFields) > 0 || len(s.ProjectedFields) > 0 || !strings.Contains(strings.Join(p.Errors, " "), "context canceled") || !strings.Contains(strings.Join(s.Errors, " "), "context canceled") {
			t.Fatalf("cancellation leaked model: %+v / %+v", p, s)
		}
	}
}
