package kql

import (
	"context"
	"reflect"
	"testing"
)

func TestDataflowFinalCommentPrefixedCommands(t *testing.T) {
	for _, tc := range []struct {
		query, source, command, effect string
		bindings                       int
	}{
		{`T | /*c*/ extend y=x | where y=="a"`, `/*c*/ extend y=x`, "extend", "assign", 1},
		{`T | /*c*/ where x==1`, `/*c*/ where x==1`, "where", "filter", 0},
		{"T | //c\n /*d*/ extend y=x | where y==\"a\"", "//c\n /*d*/ extend y=x", "extend", "assign", 1},
	} {
		t.Run(tc.source, func(t *testing.T) {
			parsed, semantic := ExtractConditions(tc.query), AnalyzeQuery(tc.query)
			for _, result := range []struct {
				name       string
				errors     []string
				flow       *QueryDataflow
				conditions []Condition
			}{
				{"extract", parsed.Errors, parsed.Dataflow, parsed.Conditions}, {"analyze", semantic.Errors, semantic.Dataflow, semantic.Conditions},
			} {
				t.Run(result.name, func(t *testing.T) {
					if len(result.errors) != 0 || result.flow == nil || !result.flow.Complete || len(result.flow.Stages) == 0 || len(result.flow.Bindings) != tc.bindings || len(result.conditions) != 1 {
						t.Fatalf("supported stage unavailable: errors=%v flow=%+v conditions=%+v", result.errors, result.flow, result.conditions)
					}
					stage := result.flow.Stages[0]
					if stage.Command != tc.command || stage.Source != tc.source || stage.Effect != tc.effect {
						t.Fatalf("authored command/source: %+v", stage)
					}
					if tc.bindings == 1 {
						binding := result.flow.Bindings[0]
						condition := result.conditions[0]
						if binding.Field != "y" || binding.Expression != "x" || binding.StageID != stage.ID || condition.FieldBinding == nil || condition.FieldBinding.BindingID != binding.ID || condition.SourceField != "x" {
							t.Fatalf("assignment disconnected: binding=%+v condition=%+v", binding, condition)
						}
					}
				})
			}
		})
	}
}

func TestKQLFinalCommentPrefixedInventory(t *testing.T) {
	query := `T | /*c*/ extend y=x | /*d*/ where y==1`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	source := newKQLMappedSource(query, budget)
	var commands, stages, rhs []string
	for _, occurrence := range source.Occurrences {
		switch occurrence.Kind {
		case "stage":
			commands = append(commands, occurrence.Command)
			stages = append(stages, query[occurrence.Span.Start:occurrence.Span.End])
		case "rhs":
			rhs = append(rhs, query[occurrence.Span.Start:occurrence.Span.End])
		}
	}
	if !reflect.DeepEqual(commands, []string{"extend", "where"}) || !reflect.DeepEqual(stages, []string{"/*c*/ extend y=x", "/*d*/ where y==1"}) || !reflect.DeepEqual(rhs, []string{"x"}) {
		t.Fatalf("commands=%q stages=%q rhs=%q", commands, stages, rhs)
	}
}

func TestDataflowFinalAuthoredRHSFieldIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		fields      []string
		sources     []string
		ids         []string
	}{
		{"reserved", `T | extend pattern=src | where x == pattern | take 1`, []string{"pattern"}, []string{"pattern"}, []string{"b0"}},
		{"parenthesized", `T | extend pattern=src | where x == (pattern) | take 1`, []string{"pattern"}, []string{"(pattern)"}, []string{"b0"}},
		{"authored collision", `T | extend pattern=src, _pattern_=other | where x == pattern or x == _pattern_ | take 1`, []string{"pattern", "_pattern_"}, []string{"pattern", "_pattern_"}, []string{"b0", "b1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, semantic := ExtractConditions(tc.query), AnalyzeQuery(tc.query)
			check := func(t *testing.T, errors []string, flow *QueryDataflow, conditions []Condition) {
				t.Helper()
				if len(errors) != 0 || flow == nil || !flow.Complete || len(conditions) != len(tc.fields) || len(flow.Bindings) != len(tc.fields) {
					t.Fatalf("field fixture unavailable: errors=%v flow=%+v conditions=%+v", errors, flow, conditions)
				}
				for i, condition := range conditions {
					rhs := condition.RHS
					binding := flow.Bindings[i]
					if binding.ID != tc.ids[i] || binding.Field != tc.fields[i] || condition.Field != "x" || condition.ValueReference != tc.fields[i] || rhs == nil || rhs.Kind != OperandField || rhs.Value != tc.fields[i] || rhs.Source != tc.sources[i] || rhs.Binding != "" || rhs.FieldBinding == nil || rhs.FieldBinding.Kind != "binding" || rhs.FieldBinding.BindingID != tc.ids[i] || rhs.FieldBinding.ScopeID != condition.InputScopeID {
						t.Errorf("field occurrence %d: binding=%+v condition=%+v rhs=%+v", i, binding, condition, rhs)
					}
				}
			}
			t.Run("extract", func(t *testing.T) { check(t, parsed.Errors, parsed.Dataflow, parsed.Conditions) })
			t.Run("analyze", func(t *testing.T) {
				check(t, semantic.Errors, semantic.Dataflow, semantic.Conditions)
				var leaves []Condition
				var visit func(*SemanticExpression)
				visit = func(e *SemanticExpression) {
					if e == nil {
						return
					}
					if e.Condition != nil {
						leaves = append(leaves, *e.Condition)
					}
					for _, child := range e.Children {
						visit(child)
					}
				}
				visit(semantic.Expression)
				check(t, semantic.Errors, semantic.Dataflow, leaves)
				if len(leaves) == len(semantic.Conditions) && len(leaves) > 0 && leaves[0].RHS != nil && leaves[0].RHS.FieldBinding != nil && semantic.Conditions[0].RHS != nil {
					leaves[0].RHS.Value = "mutated"
					leaves[0].RHS.FieldBinding.BindingID = "mutated"
					if semantic.Conditions[0].RHS.Value == "mutated" || semantic.Conditions[0].RHS.FieldBinding.BindingID == "mutated" {
						t.Fatal("expression field operand aliases semantic condition")
					}
				}
			})
		})
	}
}

func TestDataflowFinalAggregateAuthoredIdentityAndOccurrence(t *testing.T) {
	for _, tc := range []struct {
		name, query                  string
		columns, expressions, stages []string
	}{
		{"clean", `T | summarize pattern=count() | take 1`, []string{"pattern"}, []string{"count()"}, []string{"t0"}},
		{"collision and repeated stage", `T | summarize pattern=count(), _pattern_=sum(x) | summarize pattern=count() | take 1`, []string{"pattern", "_pattern_", "pattern"}, []string{"count()", "sum(x)", "count()"}, []string{"t0", "t0", "t1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, semantic := ExtractConditions(tc.query), AnalyzeQuery(tc.query)
			for _, result := range []struct {
				name     string
				errors   []string
				flow     *QueryDataflow
				aggs     []Aggregation
				computed map[string]string
			}{
				{"extract", parsed.Errors, parsed.Dataflow, parsed.Aggregations, parsed.ComputedFields}, {"analyze", semantic.Errors, semantic.Dataflow, semantic.Aggregations, semantic.ComputedFields},
			} {
				t.Run(result.name, func(t *testing.T) {
					if len(result.errors) != 0 || result.flow == nil || !result.flow.Complete || len(result.aggs) != len(tc.columns) || len(result.flow.Bindings) != len(tc.columns) {
						t.Fatalf("aggregate unavailable: errors=%v flow=%+v aggs=%+v", result.errors, result.flow, result.aggs)
					}
					for i, agg := range result.aggs {
						binding := result.flow.Bindings[i]
						if agg.Column != tc.columns[i] || agg.StageID != tc.stages[i] || agg.BindingID == "" || agg.BindingID != binding.ID || binding.StageID != tc.stages[i] || binding.Field != tc.columns[i] || binding.Kind != "aggregate" || binding.Expression != tc.expressions[i] {
							t.Errorf("aggregate occurrence %d: agg=%+v binding=%+v", i, agg, binding)
						}
					}
					want := map[string]string{"pattern": "count()"}
					if len(tc.columns) > 1 {
						want["_pattern_"] = "sum(x)"
					}
					if !reflect.DeepEqual(result.computed, want) {
						t.Errorf("computed identity: got=%v want=%v", result.computed, want)
					}
				})
			}
		})
	}
}

func TestDataflowFinalOpaqueAggregateKeepsBlankGraphIDs(t *testing.T) {
	query := `T | summarize pattern=count() by bin(x,1) | take 1`
	parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
	for _, result := range []struct {
		errors []string
		flow   *QueryDataflow
		aggs   []Aggregation
	}{{parsed.Errors, parsed.Dataflow, parsed.Aggregations}, {semantic.Errors, semantic.Dataflow, semantic.Aggregations}} {
		if len(result.errors) != 0 || result.flow == nil || result.flow.Complete || len(result.aggs) != 1 || len(result.flow.Bindings) != 0 || result.flow.Stages[0].Effect != "opaque" || result.aggs[0].Column != "pattern" || result.aggs[0].StageID != "" || result.aggs[0].BindingID != "" {
			t.Fatalf("opaque record conflated with represented aggregate: errors=%v flow=%+v aggs=%+v", result.errors, result.flow, result.aggs)
		}
	}
}

func TestKQLFinalizationRejectsUnownedRepresentedAggregate(t *testing.T) {
	parsed := ExtractConditions(`T | summarize n=count()`)
	if len(parsed.Errors) != 0 || parsed.Dataflow == nil || !parsed.Dataflow.Complete || len(parsed.Aggregations) != 1 {
		t.Fatalf("aggregate fixture unavailable: %+v", parsed)
	}
	parsed.Aggregations[0].StageID = ""
	parsed.Aggregations[0].BindingID = ""
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	defer func() {
		if recover() == nil {
			t.Fatal("supported represented aggregate published without graph IDs")
		}
	}()
	finalizeDataflow(parsed, budget)
}

// Removing a synthetic aggregate must retain the remaining occurrence owners.
func TestDataflowFinalRewrittenAggregateDoesNotShiftSupportedOwner(t *testing.T) {
	query := `T | summarize by x | summarize pattern=count() | take 1`
	parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
	for _, result := range []struct {
		errors []string
		flow   *QueryDataflow
		aggs   []Aggregation
	}{{parsed.Errors, parsed.Dataflow, parsed.Aggregations}, {semantic.Errors, semantic.Dataflow, semantic.Aggregations}} {
		if len(result.errors) != 0 || result.flow == nil || result.flow.Complete || len(result.flow.Stages) != 3 || result.flow.Stages[0].Effect != "opaque" || len(result.aggs) != 1 || len(result.flow.Bindings) != 1 {
			t.Fatalf("rewritten/supported stages unavailable: errors=%v flow=%+v aggs=%+v", result.errors, result.flow, result.aggs)
		}
		agg, binding := result.aggs[0], result.flow.Bindings[0]
		if agg.Column != "pattern" || agg.Function != "count" || agg.StageID != "t1" || agg.BindingID != "b0" || binding.ID != agg.BindingID || binding.StageID != agg.StageID || binding.Field != agg.Column || binding.Expression != "count()" {
			t.Fatalf("supported aggregate occurrence shifted: agg=%+v binding=%+v", agg, binding)
		}
	}
}
