package kql

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestWave4oMakeBagDirectOutput(t *testing.T) {
	for _, tc := range []struct {
		call, function, field, column string
	}{
		{"make_bag(p)", "make_bag", "p", "bag_p"},
		{"make_bag(p,100)", "make_bag", "p", "bag_p"},
		{"make_bag(P)", "make_bag", "P", "bag_P"},
		{"make_bag(_p)", "make_bag", "_p", "bag__p"},
		{"B=make_bag(p)", "make_bag", "p", "B"},
		// Existing alias controls are source-characterized through the shared
		// newAggregation alias path and preflight before this repair. Their
		// first public observation will be retained in baseline test output.
		// No dynamic-path/computed backend output name is inferred from them.
		{"B=make_bag(p.q)", "make_bag", "p.q", "B"},
		{"B=make_bag(['p.q'])", "make_bag", "p.q", "B"},
		{"B=make_bag(p+1)", "make_bag", "", "B"},
		{"make_set(p)", "make_set", "p", "set_p"},
		{"count()", "count", "", "count_"},
	} {
		for _, suffix := range []string{"", " by Known"} {
			t.Run(tc.call+suffix, func(t *testing.T) {
				query := "T | summarize " + tc.call + suffix
				p, s := ExtractConditions(query), AnalyzeQuery(query)
				for _, api := range []struct {
					flow             *QueryDataflow
					aggs             []Aggregation
					errors, warnings []string
				}{
					{p.Dataflow, p.Aggregations, p.Errors, p.Warnings},
					{s.Dataflow, s.Aggregations, s.Errors, s.Warnings},
				} {
					if len(api.errors) != 0 || len(api.warnings) != 0 || api.flow == nil || !api.flow.Complete || len(api.aggs) != 1 || len(api.flow.Stages) != 1 {
						t.Fatalf("complete control: %+v", api)
					}
					a, stage := api.aggs[0], api.flow.Stages[0]
					wantBindings := 1
					if suffix != "" {
						wantBindings++
					}
					if len(api.flow.Bindings) != wantBindings || len(stage.BindingIDs) != wantBindings || stage.Effect != "aggregate" || stage.Source != "summarize "+tc.call+suffix {
						t.Fatalf("binding/source shape: %+v", api.flow)
					}
					b := api.flow.Bindings[0]
					if a.Function != tc.function || a.Field != tc.field || a.Column != tc.column || b.Field != tc.column || b.Kind != "aggregate" || b.Expression != strings.TrimPrefix(tc.call, "B=") || a.StageID != stage.ID || a.BindingID != b.ID || b.StageID != stage.ID || stage.BindingIDs[0] != b.ID {
						t.Errorf("exact aggregate output identity: aggregate=%+v binding=%+v want column %q", a, b, tc.column)
					}
					if suffix != "" {
						group := api.flow.Bindings[1]
						if group.Field != "Known" || group.Kind != "group" || group.Expression != "Known" {
							t.Errorf("group identity: %+v", group)
						}
					}
					for _, scope := range api.flow.Scopes {
						if scope.ID == stage.OutputScopeID && scope.Mode != "closed" {
							t.Errorf("output must be closed: %+v", scope)
						}
					}
				}
			})
		}
	}
}

func TestWave4oMakeBagUnprovedOutputsOpaque(t *testing.T) {
	// These are parser-carrier controls, not claims that every authored arity or
	// expression has a valid backend signature or an independently proven name.
	for _, tc := range []struct{ call, field, column string }{
		{`make_bag(p.q)`, "p.q", "make_bag_p.q"},
		{`make_bag(['p.q'])`, "p.q", "make_bag_p.q"},
		{`make_bag(['p'])`, "p", "make_bag_p"},
		{`make_bag(p+1)`, "", "make_bag_"},
		{`make_bag(bag_pack("x",p))`, "", "make_bag_"},
		{`make_bag("p")`, "", "make_bag_"},
		{`make_bag(*)`, "", "make_bag_"},
		{`make_bag(expr=p)`, "", "make_bag_"},
		{`make_bag(3plogTime)`, "3plogTime", "make_bag_3plogTime"},
		{`make_bag()`, "", "make_bag_"},
		{`make_bag(p,100,200)`, "p", "make_bag_p"},
	} {
		for _, prefix := range []string{"", "count(), "} {
			t.Run(prefix+tc.call, func(t *testing.T) {
				source := "summarize " + prefix + tc.call + " by Known"
				p, s := ExtractConditions("T | "+source+" | where After > 1"), AnalyzeQuery("T | "+source+" | where After > 1")
				for _, api := range []struct {
					flow             *QueryDataflow
					aggs             []Aggregation
					conditions       []Condition
					errors, warnings []string
				}{
					{p.Dataflow, p.Aggregations, p.Conditions, p.Errors, p.Warnings},
					{s.Dataflow, s.Aggregations, s.Conditions, s.Errors, s.Warnings},
				} {
					raw, err := json.Marshal(map[string]any{
						"dataflow": api.flow, "aggregations": api.aggs,
						"conditions": api.conditions, "errors": api.errors, "warnings": api.warnings,
					})
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("call=%s flow=%+v aggregates=%+v characterization=%s", tc.call, api.flow, api.aggs, raw)
					wantAggs := 1
					if prefix != "" {
						wantAggs++
					}
					if len(api.errors) != 0 || api.flow == nil || api.flow.Complete || len(api.flow.Bindings) != 0 || len(api.aggs) != wantAggs || len(api.conditions) != 1 || len(api.flow.Stages) != 2 {
						t.Fatalf("unproved summarize leaked a finite graph: %+v", api)
					}
					stage := api.flow.Stages[0]
					if stage.Source != source || stage.Effect != "opaque" || len(stage.BindingIDs) != 0 {
						t.Errorf("opaque source/rollback: %+v", stage)
					}
					for _, a := range api.aggs {
						if a.StageID != "" || a.BindingID != "" {
							t.Errorf("partial aggregate IDs: %+v", a)
						}
					}
					a := api.aggs[len(api.aggs)-1]
					if a.Function != "make_bag" || a.Field != tc.field || a.Column != tc.column {
						t.Errorf("legacy summary changed: %+v", a)
					}
					if prefix != "" && api.aggs[0].Column != "count_" {
						t.Errorf("legacy count changed: %+v", api.aggs[0])
					}
					c := api.conditions[0]
					if c.Field != "After" || c.FieldBinding == nil || c.FieldBinding.Kind != "unknown" || c.FieldBinding.BindingID != "" {
						t.Errorf("opaque downstream reference: %+v", c)
					}
					if len(api.warnings) == 0 || api.warnings[0] != "dataflow:unsupported_stage_dataflow: unrepresented summarize output" {
						t.Errorf("missing existing opaque diagnostic: %v", api.warnings)
					}
				}
			})
		}
	}
}

func TestWave4oMakeBagLaterReferenceAndClone(t *testing.T) {
	// Parser lineage only: this does not assert that a dynamic bag/string
	// comparison is a valid or equivalent backend evaluation.
	const query = `T | summarize make_bag(p) | project bag_p | where bag_p == "x"`
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	for _, api := range []struct {
		flow       *QueryDataflow
		conditions []Condition
		errors     []string
	}{
		{p.Dataflow, p.Conditions, p.Errors}, {s.Dataflow, s.Conditions, s.Errors},
	} {
		if len(api.errors) != 0 || api.flow == nil || !api.flow.Complete || len(api.flow.Bindings) != 2 || len(api.conditions) != 1 {
			t.Fatalf("later reference: %+v", api)
		}
		metric, projection := api.flow.Bindings[0], api.flow.Bindings[1]
		if metric.Field != "bag_p" || metric.Kind != "aggregate" || projection.Field != "bag_p" || projection.Kind != "passthrough" || projection.Expression != "bag_p" {
			t.Errorf("output/project identity: %+v", api.flow.Bindings)
		}
		c := api.conditions[0]
		if c.FieldBinding == nil || c.FieldBinding.Kind != "binding" || c.FieldBinding.BindingID != projection.ID {
			t.Errorf("projected name resolved as raw/unknown: %+v", c)
		}
	}
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	clone := semanticFromExtraction(p, b)
	cancel()
	p.Aggregations[0].Column = "mutated"
	p.Dataflow.Bindings[0].Field = "mutated"
	p.Dataflow.Stages[0].Source = "mutated"
	p.Dataflow.Scopes[1].BindingIDs[0] = "mutated"
	p.Conditions[0].FieldBinding.BindingID = "mutated"
	if clone.Aggregations[0].Column != "bag_p" || clone.Dataflow.Bindings[0].Field != "bag_p" || clone.Dataflow.Stages[0].Source != "summarize make_bag(p)" || clone.Dataflow.Scopes[1].BindingIDs[0] == "mutated" || clone.Conditions[0].FieldBinding.BindingID == "mutated" {
		t.Fatal("semantic clone aliases extracted owners")
	}
}

func TestWave4oMakeBagSharedBudgets(t *testing.T) {
	for _, query := range []string{
		`T | summarize make_bag(p,100) by Known`,
		`T | summarize count(), make_bag(p.q) by Known`,
		`T | join (Other | summarize make_bag(p) by Known) on Known | summarize B=make_bag(p)`,
	} {
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
				{"metadata items", used.metadataItems, func(l *parseLimits, n int) { l.maxMetadataItems = n }},
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
		// The public output cap also bounds temporary list sizes that are not
		// accumulated in budget.output. Its used counter is not an exact cap.
		l := defaultParseLimits()
		l.maxOutputItems = 0
		p0, s0 := extractConditionsContext(context.Background(), query, l), analyzeQueryContext(context.Background(), query, l)
		if p0.Dataflow != nil || s0.Dataflow != nil || len(p0.Aggregations) > 0 || len(s0.Aggregations) > 0 || !strings.Contains(strings.Join(p0.Errors, " "), "output items") || !strings.Contains(strings.Join(s0.Errors, " "), "output items") {
			t.Fatalf("zero output cap leaked model: %+v / %+v", p0, s0)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		p, s := ExtractConditionsContext(ctx, query), AnalyzeQueryContext(ctx, query)
		if p.Dataflow != nil || s.Dataflow != nil || len(p.Aggregations) > 0 || len(s.Aggregations) > 0 || !strings.Contains(strings.Join(p.Errors, " "), "context canceled") || !strings.Contains(strings.Join(s.Errors, " "), "context canceled") {
			t.Fatalf("cancellation leaked model: %+v / %+v", p, s)
		}
	}
}
