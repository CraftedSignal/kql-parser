package kql

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestWave4QuotedAggregateFieldIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, query, field, expression string
	}{
		{"plain", `Events | summarize Total=sum(Amount)`, "Amount", "sum(Amount)"},
		{"quoted ordinary", `Events | summarize Total=sum(['Amount'])`, "Amount", "sum(['Amount'])"},
		{"quoted space", `Events | summarize Total=sum(['Amount Value'])`, "Amount Value", "sum(['Amount Value'])"},
		{"digit leading", `Events | summarize Total=sum(3plogTime)`, "3plogTime", "sum(3plogTime)"},
		{"dotted spacing", `Events | summarize Total=sum(bag . Amount)`, "bag.Amount", "sum(bag . Amount)"},
		{"dotted comment", `Events | summarize Total=sum(bag /* owner */ . Amount)`, "bag.Amount", "sum(bag /* owner */ . Amount)"},
		{"dotted digit leading", `Events | summarize Total=sum(3plogTime . Amount)`, "3plogTime.Amount", "sum(3plogTime . Amount)"},
		{"dotted", `Events | summarize Total=sum(bag.Amount)`, "bag.Amount", "sum(bag.Amount)"},
		{"escaped", `Events | summarize Total=sum(['Am\u006funt'])`, "Amount", `sum(['Am\u006funt'])`},
		{"verbatim", `Events | summarize Total=sum([@'A\b'])`, `A\b`, `sum([@'A\b'])`},
		{"parenthesized", `Events | summarize Total=sum((Amount))`, "Amount", "sum((Amount))"},
		{"quoted number", `Events | summarize Total=sum(['1'])`, "1", "sum(['1'])"},
		{"string literal", `Events | summarize Total=sum("Amount")`, "", `sum("Amount")`},
		{"bool literal", `Events | summarize Total=sum(true)`, "", "sum(true)"},
		{"null literal", `Events | summarize Total=sum(null)`, "", "sum(null)"},
		{"typed literal", `Events | summarize Total=sum(long(1))`, "", "sum(long(1))"},
		{"nested call", `Events | summarize Total=sum(abs(Amount))`, "", "sum(abs(Amount))"},
		{"dynamic access", `Events | summarize Total=sum(bag['Amount'])`, "", "sum(bag['Amount'])"},
		{"index access", `Events | summarize Total=sum(bag[0])`, "", "sum(bag[0])"},
		{"numeric literal is not field", `Events | summarize Total=sum(1)`, "", "sum(1)"},
		{"compound expression is not field", `Events | summarize Total=sum(Amount + 1)`, "", "sum(Amount + 1)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, semantic := ExtractConditions(tc.query), AnalyzeQuery(tc.query)
			if parsed == nil || semantic == nil {
				t.Fatalf("public API returned nil: extract=%+v analyze=%+v", parsed, semantic)
			}
			for _, api := range []struct {
				name     string
				payload  any
				errors   []string
				warnings []string
				flow     *QueryDataflow
				aggs     []Aggregation
			}{
				{"ExtractConditions", parsed, parsed.Errors, parsed.Warnings, parsed.Dataflow, parsed.Aggregations},
				{"AnalyzeQuery", semantic, semantic.Errors, semantic.Warnings, semantic.Dataflow, semantic.Aggregations},
			} {
				t.Run(api.name, func(t *testing.T) {
					data, err := json.Marshal(api.payload)
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("query=%q result=%s", tc.query, data)
					if len(api.errors) != 0 || len(api.warnings) != 0 || api.flow == nil || !api.flow.Complete || len(api.flow.Diagnostics) != 0 {
						t.Fatalf("expected complete diagnostic-free graph before metadata assertions: errors=%q warnings=%q flow=%+v", api.errors, api.warnings, api.flow)
					}
					if len(api.aggs) != 1 || len(api.flow.Stages) != 1 || len(api.flow.Bindings) != 1 {
						t.Fatalf("expected one aggregate/stage/binding: aggs=%+v flow=%+v", api.aggs, api.flow)
					}
					agg, stage, binding := api.aggs[0], api.flow.Stages[0], api.flow.Bindings[0]
					if agg.StageID == "" || agg.BindingID == "" || agg.StageID != stage.ID || agg.BindingID != binding.ID || binding.StageID != stage.ID || !reflect.DeepEqual(stage.BindingIDs, []string{binding.ID}) || stage.Effect != "aggregate" || stage.Command != "summarize" || agg.PipeStage != stage.PipeStage || agg.Pipeline != stage.Pipeline {
						t.Fatalf("aggregate owner identity mismatch: aggregate=%+v stage=%+v binding=%+v", agg, stage, binding)
					}
					if agg.Function != "sum" || agg.Column != "Total" || binding.Kind != "aggregate" || binding.Field != "Total" || binding.Expression != tc.expression {
						t.Fatalf("function/alias/authored expression changed: aggregate=%+v binding=%+v, want expression %q", agg, binding, tc.expression)
					}
					if agg.Field != tc.field {
						t.Errorf("aggregated field=%q, want exact input field %q", agg.Field, tc.field)
					}
				})
			}
		})
	}
}

func TestWave4AggregateSharedBudgets(t *testing.T) {
	for _, query := range []string{
		`Events | summarize sum(bag /* owner */ . Amount)`,
		`Events | summarize sum(['Am\u006funt']), Total=sum(['Amount Value'])`,
		`Events | join (Other | summarize Total=sum(['Amount Value']) by Key) on Key | summarize sum(['Am\u006funt'])`,
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

func TestWave4AggregateDefaultColumns(t *testing.T) {
	const query = `Events | summarize count(), sum(['Am\u006funt']), Total=sum(3plogTime)`
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	for _, aggs := range [][]Aggregation{p.Aggregations, s.Aggregations} {
		if len(aggs) != 3 || aggs[0].Column != "count_" || aggs[0].Field != "" || aggs[1].Column != "sum_Amount" || aggs[1].Field != "Amount" || aggs[2].Column != "Total" || aggs[2].Field != "3plogTime" {
			t.Fatalf("default columns/identities: %+v", aggs)
		}
	}
	if len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || s.Dataflow == nil || !p.Dataflow.Complete || !s.Dataflow.Complete || !reflect.DeepEqual(p.Aggregations, s.Aggregations) || len(p.Dataflow.Bindings) != 3 {
		t.Fatalf("multiple aggregate owners: %+v / %+v", p, s)
	}
	for i, a := range p.Aggregations {
		if a.BindingID != p.Dataflow.Bindings[i].ID || a.Column != p.Dataflow.Bindings[i].Field {
			t.Fatalf("owner %d: %+v", i, p)
		}
	}
	s.Aggregations[1].Field = "changed"
	if p.Aggregations[1].Field != "Amount" {
		t.Fatal("semantic summary aliases extracted slice")
	}
}

// The primary quoted node needs one traversal step, one decode step, then
// scan+decode reservations of three bytes each: 1+1+3+3 = 8. Two calls
// share sixteen units; neither temporary owns output/metadata yet.
func TestWave4AggregateIdentityExactWork(t *testing.T) {
	primary := kqlParserForTest(t, `['A']`).PrimaryExpression()
	for _, copies := range []int{1, 2} {
		for _, short := range []bool{false, true} {
			l := defaultParseLimits()
			l.maxWork = 8 * copies
			if short {
				l.maxWork--
			}
			b, cancel := newParseBudget(context.Background(), l)
			func() {
				defer func() {
					failure := recover()
					if short {
						if failure == nil || !strings.Contains(parseFailure(failure).Error(), "work budget") {
							t.Errorf("one-short %d copies: %v", copies, failure)
						}
					} else if failure != nil {
						t.Errorf("exact %d copies: %v", copies, failure)
					}
				}()
				e := conditionExtractor{budget: b}
				for i := 0; i < copies; i++ {
					if got := e.aggregateFieldIdentity(primary, b); got != "A" {
						t.Fatalf("decoded field=%q", got)
					}
				}
				if b.work != 8*copies || b.metadataBytes != 0 || b.output != 0 {
					t.Fatalf("temporary accounting: %+v", b)
				}
			}()
			cancel()
		}
	}
	// Dotted joining: traversal/validation 2, base recognition 5, first slot 1,
	// first copy 3, suffix visit/decode 2, suffix+dot copy 7, replacement slots2.
	dotted := kqlParserForTest(t, `bag . Amount`).PostfixExpression()
	for _, maxWork := range []int{22, 21} {
		l := defaultParseLimits()
		l.maxWork = maxWork
		b, cancel := newParseBudget(context.Background(), l)
		func() {
			defer func() {
				failure := recover()
				if maxWork == 21 {
					if failure == nil || !strings.Contains(parseFailure(failure).Error(), "work budget") {
						t.Errorf("dotted one-short: %v", failure)
					}
				} else if failure != nil {
					t.Errorf("dotted exact: %v", failure)
				}
			}()
			e := conditionExtractor{budget: b}
			if got := e.aggregateFieldIdentity(dotted, b); got != "bag.Amount" || b.work != 22 || b.metadataBytes != 0 || b.output != 0 {
				t.Fatalf("dotted temporary=%q budget=%+v", got, b)
			}
		}()
		cancel()
	}
}
