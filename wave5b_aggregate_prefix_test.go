package kql

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type wave5bModel struct {
	flow             *QueryDataflow
	aggs             []Aggregation
	conditions       []Condition
	errors, warnings []string
}

func wave5bModels(query string) []wave5bModel {
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	return []wave5bModel{{p.Dataflow, p.Aggregations, p.Conditions, p.Errors, p.Warnings}, {s.Dataflow, s.Aggregations, s.Conditions, s.Errors, s.Warnings}}
}

// Literal oracles follow Microsoft's frozen aggregate policy and binder, not
// the parser's default-name constructor. Dropping a prefix/FirstArgument policy
// must fail the legacy output, actual binding and downstream-reference checks.
func TestWave5bDocumentedAggregateOutputs(t *testing.T) {
	for _, tc := range []struct {
		id, query string
		want      []struct{ function, field, column, expression string }
	}{
		{"buildschema", "T | summarize buildschema(value)", []struct{ function, field, column, expression string }{{"buildschema", "value", "schema_value", "buildschema(value)"}}},
		{"make_list_if", "T | summarize make_list_if(name, include)", []struct{ function, field, column, expression string }{{"make_list_if", "name", "list_name", "make_list_if(name, include)"}}},
		{"make_set_if", "T | summarize make_set_if(name, include)", []struct{ function, field, column, expression string }{{"make_set_if", "name", "set_name", "make_set_if(name, include)"}}},
		{"make_bag_if", "T | summarize make_bag_if(value, include)", []struct{ function, field, column, expression string }{{"make_bag_if", "value", "bag_value", "make_bag_if(value, include)"}}},
		{"make_list_with_nulls", "T | summarize make_list_with_nulls(value)", []struct{ function, field, column, expression string }{{"make_list_with_nulls", "value", "list_value", "make_list_with_nulls(value)"}}},
		{"tdigest_merge", "T | summarize tdigest_merge(td)", []struct{ function, field, column, expression string }{{"tdigest_merge", "td", "merge_tdigests_td", "tdigest_merge(td)"}}},
		{"merge_tdigest", "T | summarize merge_tdigest(td)", []struct{ function, field, column, expression string }{{"merge_tdigest", "td", "merge_tdigests_td", "merge_tdigest(td)"}}},
		{"binary_all_and", "T | summarize binary_all_and(num)", []struct{ function, field, column, expression string }{{"binary_all_and", "num", "num", "binary_all_and(num)"}}},
		{"binary_all_or", "T | summarize binary_all_or(num)", []struct{ function, field, column, expression string }{{"binary_all_or", "num", "num", "binary_all_or(num)"}}},
		{"binary_all_xor", "T | summarize binary_all_xor(num)", []struct{ function, field, column, expression string }{{"binary_all_xor", "num", "num", "binary_all_xor(num)"}}},
		{"take_anyif", "T | summarize take_anyif(name, include)", []struct{ function, field, column, expression string }{{"take_anyif", "name", "name", "take_anyif(name, include)"}}},
		{"explicit_aliases", "T | summarize Schema=buildschema(value), NamesList=make_list_if(name, include), NamesSet=make_set_if(name, include), Bag=make_bag_if(value, include), NullableList=make_list_with_nulls(value), Digest=tdigest_merge(td), AliasDigest=merge_tdigest(td), BitsAnd=binary_all_and(num), BitsOr=binary_all_or(num), BitsXor=binary_all_xor(num), Chosen=take_anyif(name, include)", []struct{ function, field, column, expression string }{{"buildschema", "value", "Schema", "buildschema(value)"}, {"make_list_if", "name", "NamesList", "make_list_if(name, include)"}, {"make_set_if", "name", "NamesSet", "make_set_if(name, include)"}, {"make_bag_if", "value", "Bag", "make_bag_if(value, include)"}, {"make_list_with_nulls", "value", "NullableList", "make_list_with_nulls(value)"}, {"tdigest_merge", "td", "Digest", "tdigest_merge(td)"}, {"merge_tdigest", "td", "AliasDigest", "merge_tdigest(td)"}, {"binary_all_and", "num", "BitsAnd", "binary_all_and(num)"}, {"binary_all_or", "num", "BitsOr", "binary_all_or(num)"}, {"binary_all_xor", "num", "BitsXor", "binary_all_xor(num)"}, {"take_anyif", "name", "Chosen", "take_anyif(name, include)"}}},
		{"grouped_followup", "T | summarize buildschema(value), N=count() by Known | where isnotnull(schema_value)", []struct{ function, field, column, expression string }{{"buildschema", "value", "schema_value", "buildschema(value)"}, {"count", "", "N", "count()"}}},
		{"unrelated_conditionals", "T | summarize minif(num, include), maxif(num, include), avgif(num, include)", []struct{ function, field, column, expression string }{{"minif", "num", "minif_num", "minif(num, include)"}, {"maxif", "num", "maxif_num", "maxif(num, include)"}, {"avgif", "num", "avgif_num", "avgif(num, include)"}}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			for api, m := range wave5bModels(tc.query) {
				if len(m.errors) > 0 || len(m.warnings) > 0 || m.flow == nil || !m.flow.Complete || len(m.aggs) != len(tc.want) {
					t.Errorf("api%d complete model: %+v", api, m)
					continue
				}
				stage := m.flow.Stages[0]
				if stage.Effect != "aggregate" || stage.Source != strings.Split(strings.TrimPrefix(tc.query, "T | "), " | ")[0] {
					t.Errorf("source: %+v", stage)
				}
				for i, w := range tc.want {
					a, b := m.aggs[i], m.flow.Bindings[i]
					if a.Function != w.function || a.Field != w.field || a.Column != w.column || b.Field != w.column || b.Kind != "aggregate" || b.Expression != w.expression || a.StageID != stage.ID || a.BindingID != b.ID || b.StageID != stage.ID || stage.BindingIDs[i] != b.ID {
						t.Errorf("api%d metric%d got %+v binding %+v want %+v", api, i, a, b, w)
					}
				}
				if tc.id == "grouped_followup" {
					group := m.flow.Bindings[2]
					if group.Field != "Known" || group.Kind != "group" || len(m.conditions) != 1 || m.conditions[0].FieldBinding == nil || m.conditions[0].FieldBinding.Kind != "binding" || m.conditions[0].FieldBinding.BindingID != m.aggs[0].BindingID {
						t.Errorf("group/downstream reference: %+v %+v", group, m.conditions)
					}
				}
			}
		})
	}
}

func TestWave5bTypedResultNames(t *testing.T) {
	for _, tc := range []struct{ query, column, field string }{
		{`T | summarize buildschema(['value'])`, "schema_value", "value"},
		{`T | summarize make_set_if(['name.with.dot'],include)`, "set_name.with.dot", "name.with.dot"},
		{`T | summarize take_anyif(['name.with.dot'],include)`, "name.with.dot", "name.with.dot"},
		{`T | summarize binary_all_and((num))`, "num", "num"},
		{`T | summarize make_list_with_nulls(3plogTime)`, "list_3plogTime", "3plogTime"},
		{`let n=7; T | summarize binary_all_and(n)`, "n", "n"},
		{`T | summarize make_list_with_nulls(num+1)`, "list_", ""},
		{`T | summarize make_list_if("x",include)`, "list_", ""},
		{`T | summarize buildschema(dynamic({"x":1}))`, "schema_", ""},
		{`T | summarize buildschema(bag_pack("x",num))`, "schema_", ""},
		{`T | summarize make_list_if(tostring(num),include)`, "list_num", ""},
		{`T | summarize make_list_if(tolower(name),include)`, "list_", ""},
		{`T | summarize make_list_if(name,include,100)`, "list_name", "name"},
		{`T | summarize make_set_if(name,include,100)`, "set_name", "name"},
		{`T | summarize make_bag_if(value,include,100)`, "bag_value", "value"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			for api, m := range wave5bModels(tc.query) {
				raw, _ := json.Marshal(map[string]any{"flow": m.flow, "aggregations": m.aggs, "errors": m.errors, "warnings": m.warnings})
				t.Logf("api%d first-role-characterization %s", api, raw)
				if len(m.errors) > 0 || len(m.warnings) > 0 || m.flow == nil || !m.flow.Complete || len(m.aggs) != 1 || len(m.flow.Bindings) != 1 {
					t.Errorf("complete typed name api%d: %+v", api, m)
					continue
				}
				a, b := m.aggs[0], m.flow.Bindings[0]
				if a.Field != tc.field || a.Column != tc.column || b.Field != tc.column || b.Kind != "aggregate" || a.BindingID != b.ID || a.StageID != b.StageID || b.Expression != strings.Split(tc.query, "summarize ")[1] {
					t.Errorf("typed output api%d got %+v %+v want %q", api, a, b, tc.column)
				}
			}
		})
	}
}
func TestWave5bUnrepresentedNamesAndAliases(t *testing.T) {
	for _, call := range []string{`binary_all_and(num+1)`, `take_anyif("x",include)`, `buildschema(value.key)`, `buildschema(custom(value))`, `make_list_if(name)`, `buildschema(expr=value)`} {
		for _, alias := range []bool{false, true} {
			name := call
			if alias {
				name = "alias:" + call
			}
			t.Run(name, func(t *testing.T) {
				prefix := ""
				if alias {
					prefix = "A="
				}
				query := "T | summarize N=count(), " + prefix + call + " by Known | where isnotnull(After)"
				// The later After reference is intentionally unavailable in either path;
				// compare its kind to distinguish closed output from an opaque summarize.
				for api, m := range wave5bModels(query) {
					if len(m.errors) > 0 || m.flow == nil || len(m.aggs) != 2 || len(m.conditions) != 1 {
						t.Fatalf("carrier api%d: %+v", api, m)
					}
					stage := m.flow.Stages[0]
					if alias {
						if stage.Effect != "aggregate" || len(stage.BindingIDs) != 3 || len(m.flow.Bindings) != 3 || m.aggs[1].Column != "A" || m.aggs[1].BindingID == "" || m.conditions[0].FieldBinding.Kind != "absent" {
							t.Errorf("alias must retain finite aggregate: %+v", m)
						}
					} else {
						if stage.Effect != "opaque" || len(stage.BindingIDs) != 0 || len(m.flow.Bindings) != 0 || m.flow.Complete || m.aggs[0].BindingID != "" || m.aggs[1].StageID != "" || m.conditions[0].FieldBinding.Kind != "unknown" {
							t.Errorf("whole summarize leaked unavailable name api%d: %+v", api, m)
						}
					}
				}
			})
		}
	}
}

func TestWave5bOwnershipAndSharedLimits(t *testing.T) {
	const query = `T | summarize tdigest_merge(td) by Known | where isnotnull(merge_tdigests_td)`
	p := ExtractConditions(query)
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	clone := semanticFromExtraction(p, b)
	cancel()
	if len(clone.Aggregations) != 1 || clone.Aggregations[0].Column != "merge_tdigests_td" {
		t.Fatalf("name control: %+v", clone)
	}
	p.Aggregations[0].Column = "mutated"
	p.Dataflow.Bindings[0].Field = "mutated"
	p.Conditions[0].FieldBinding.BindingID = "mutated"
	if clone.Aggregations[0].Column != "merge_tdigests_td" || clone.Dataflow.Bindings[0].Field != "merge_tdigests_td" || clone.Conditions[0].FieldBinding.BindingID == "mutated" {
		t.Fatal("semantic owners alias extraction")
	}
	for _, semantic := range []bool{false, true} {
		b, cancel := newParseBudget(context.Background(), defaultParseLimits())
		r := extractConditionsInternal(query, b)
		if semantic {
			_ = semanticFromExtraction(r, b)
		}
		used := *b
		cancel()
		for _, resource := range []struct {
			name string
			used int
			set  func(*parseLimits, int)
		}{
			{"work", used.work, func(l *parseLimits, n int) { l.maxWork = n }},
			{"metadata bytes", used.metadataBytes, func(l *parseLimits, n int) { l.maxMetadataBytes = n }},
			{"metadata items", used.metadataItems, func(l *parseLimits, n int) { l.maxMetadataItems = n }},
		} {
			run := func(l parseLimits) (bool, []string) {
				if semantic {
					r := analyzeQueryContext(context.Background(), query, l)
					return r.Dataflow != nil || len(r.Aggregations) > 0, r.Errors
				}
				r := extractConditionsContext(context.Background(), query, l)
				return r.Dataflow != nil || len(r.Aggregations) > 0, r.Errors
			}
			l := defaultParseLimits()
			resource.set(&l, resource.used)
			if model, errs := run(l); !model || len(errs) > 0 {
				t.Errorf("exact %s semantic%v: %v", resource.name, semantic, errs)
			}
			resource.set(&l, resource.used-1)
			if model, errs := run(l); model || len(errs) == 0 {
				t.Errorf("one-short %s leaked result semantic%v: %v", resource.name, semantic, errs)
			}
		}
	}
	// Output limits also bound temporary lists; their size is not accumulated
	// in budget.output. Check atomic failure at zero instead of guessing a cap.
	l := defaultParseLimits()
	l.maxOutputItems = 0
	p0, s0 := extractConditionsContext(context.Background(), query, l), analyzeQueryContext(context.Background(), query, l)
	if p0.Dataflow != nil || s0.Dataflow != nil || len(p0.Aggregations) > 0 || len(s0.Aggregations) > 0 || !strings.Contains(strings.Join(p0.Errors, " "), "output items") || !strings.Contains(strings.Join(s0.Errors, " "), "output items") {
		t.Fatal("zero output cap leaked model")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := ExtractConditionsContext(ctx, query); len(r.Errors) == 0 || r.Dataflow != nil || len(r.Aggregations) > 0 {
		t.Fatalf("canceled extraction leaked: %+v", r)
	}
	if r := AnalyzeQueryContext(ctx, query); len(r.Errors) == 0 || r.Dataflow != nil || len(r.Aggregations) > 0 {
		t.Fatalf("canceled semantics leaked: %+v", r)
	}
}
