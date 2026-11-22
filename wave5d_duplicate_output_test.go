package kql

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// Literal names follow the frozen Microsoft projection builder. Inputs Bytes,
// num and num1 are long columns; these assertions do not evaluate aggregates.
func TestWave5dSummarizeOutputNames(t *testing.T) {
	for _, tc := range []struct {
		name, query                        string
		columns, functions, fields, groups []string
		target                             int
	}{
		{"repeated sum", `T | summarize sum(Bytes), sum(Bytes) | where sum_Bytes1 > 0`, []string{"sum_Bytes", "sum_Bytes1"}, []string{"sum", "sum"}, []string{"Bytes", "Bytes"}, nil, 1},
		{"first argument collision", `T | summarize binary_all_and(num), binary_all_or(num) | where num1 > 0`, []string{"num", "num1"}, []string{"binary_all_and", "binary_all_or"}, []string{"num", "num"}, nil, 1},
		{"distinct alias control", `T | summarize BitsAnd=binary_all_and(num), BitsOr=binary_all_or(num) | where BitsOr > 0`, []string{"BitsAnd", "BitsOr"}, []string{"binary_all_and", "binary_all_or"}, []string{"num", "num"}, nil, 1},
		{"groups reserve occupied suffix", `T | summarize binary_all_and(num), binary_all_or(num) by ['num'], num1 | where num3 > 0`, []string{"num2", "num3"}, []string{"binary_all_and", "binary_all_or"}, []string{"num", "num"}, []string{"num", "num1"}, 1},
		{"alias first", `T | summarize sum_Bytes=sum(Bytes), sum(Bytes) | where sum_Bytes1 > 0`, []string{"sum_Bytes", "sum_Bytes1"}, []string{"sum", "sum"}, []string{"Bytes", "Bytes"}, nil, 1},
		{"default then repeated requested alias", `T | summarize sum(Bytes), sum_Bytes=sum(Bytes), sum_Bytes=sum(Bytes) | where sum_Bytes2 > 0`, []string{"sum_Bytes", "sum_Bytes1", "sum_Bytes2"}, []string{"sum", "sum", "sum"}, []string{"Bytes", "Bytes", "Bytes"}, nil, 2},
		{"stage reset", `T | summarize binary_all_and(num), binary_all_or(num) | summarize binary_all_and(num), binary_all_or(num) | where num1 > 0`, []string{"num", "num1", "num", "num1"}, []string{"binary_all_and", "binary_all_or", "binary_all_and", "binary_all_or"}, []string{"num", "num", "num", "num"}, nil, 3},
		{"computed group alias", `T | summarize sum(Bytes), sum(Bytes) by G=tostring(num) | where sum_Bytes1 > 0`, []string{"sum_Bytes", "sum_Bytes1"}, []string{"sum", "sum"}, []string{"Bytes", "Bytes"}, []string{"G"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for api, m := range wave5bModels(tc.query) {
				if len(m.errors) > 0 || len(m.warnings) > 0 || m.flow == nil || !m.flow.Complete || len(m.aggs) != len(tc.columns) {
					t.Errorf("api%d incomplete: %+v", api, m)
					continue
				}
				bindings := map[string]FieldBinding{}
				seen := map[string]map[string]bool{}
				for _, binding := range m.flow.Bindings {
					bindings[binding.ID] = binding
				}
				for i, a := range m.aggs {
					b, ok := bindings[a.BindingID]
					if !ok || a.BindingID == "" || b.Kind != "aggregate" || b.Field != tc.columns[i] || a.Column != tc.columns[i] || a.Function != tc.functions[i] || a.Field != tc.fields[i] || a.StageID != b.StageID || b.Expression != tc.functions[i]+"("+tc.fields[i]+")" {
						t.Errorf("api%d occurrence%d: %+v / %+v", api, i, a, b)
					}
					if seen[a.StageID] == nil {
						seen[a.StageID] = map[string]bool{}
					}
					if seen[a.StageID][a.Column] {
						t.Errorf("stage retained duplicate %q", a.Column)
					}
					seen[a.StageID][a.Column] = true
					stage := m.flow.Stages[a.PipeStage]
					if a.Pipeline != 0 || stage.ID != a.StageID || stage.Effect != "aggregate" || stage.Source != strings.Split(tc.query, " | ")[a.PipeStage+1] {
						t.Errorf("source/stage owner: %+v / %+v", a, stage)
					}
					closed := false
					for _, scope := range m.flow.Scopes {
						if scope.ID == stage.OutputScopeID {
							closed = scope.Mode == "closed"
						}
					}
					if !closed {
						t.Error("summarize output was not closed")
					}
				}
				if len(m.conditions) != 1 || m.conditions[0].FieldBinding == nil || m.conditions[0].FieldBinding.Kind != "binding" || m.conditions[0].FieldBinding.BindingID != m.aggs[tc.target].BindingID {
					t.Errorf("api%d wrong occurrence target: %+v", api, m.conditions)
				}
				groupIndex := 0
				for _, b := range m.flow.Bindings {
					if b.Kind == "group" {
						if groupIndex >= len(tc.groups) || b.Field != tc.groups[groupIndex] {
							t.Errorf("unexpected group: %+v", b)
						}
						groupIndex++
					}
				}
				if groupIndex != len(tc.groups) {
					t.Errorf("lost groups: %+v", m.flow.Bindings)
				}
				if len(tc.groups) > 0 {
					for i := range tc.columns {
						if m.flow.Bindings[i].Kind != "aggregate" {
							t.Error("changed aggregate-first binding emission")
						}
					}
				}
			}
		})
	}
}

func TestWave5dUnknownAndDiagnosedPlansAreAtomic(t *testing.T) {
	for _, tc := range []struct {
		query   string
		columns []string
	}{
		{`T | summarize N=sum(Bytes), N=sum(Bytes)`, []string{"N", "N"}},
		{`T | summarize n=count() by n`, []string{"n"}},
		{`T | summarize sum(Bytes), sum(Bytes), make_bag(value.key)`, []string{"sum_Bytes", "sum_Bytes", "make_bag_value.key"}},
		{`T | summarize sum(Bytes), sum(Bytes) by value.key`, []string{"sum_Bytes", "sum_Bytes"}},
	} {
		for api, m := range wave5bModels(tc.query) {
			if len(m.errors) != 0 || m.flow == nil || m.flow.Complete || len(m.flow.Bindings) != 0 || m.flow.Stages[0].Effect != "opaque" {
				t.Errorf("api%d invalid/unknown plan leaked finite model %q: %+v", api, tc.query, m)
				continue
			}
			for i, a := range m.aggs {
				if i >= len(tc.columns) || a.Column != tc.columns[i] || a.StageID != "" || a.BindingID != "" {
					t.Errorf("partial name publication: %+v", a)
				}
			}
		}
	}
	// An explicit alias proves the otherwise schema-dependent path output name.
	p := ExtractConditions(`T | summarize sum(Bytes), sum(Bytes) by G=value.key | where sum_Bytes1 > 0`)
	if len(p.Errors) > 0 || p.Dataflow == nil || !p.Dataflow.Complete || p.Aggregations[1].Column != "sum_Bytes1" {
		t.Errorf("explicit group alias was rejected: %+v", p)
	}
}

func TestWave5dCloneAndSharedBudgets(t *testing.T) {
	const query = `T | summarize binary_all_and(num), binary_all_or(num) by ['num'], num1 | where num3 > 0`
	p := ExtractConditions(query)
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	clone := semanticFromExtraction(p, b)
	cancel()
	if p.Dataflow == nil || !p.Dataflow.Complete || clone.Dataflow == nil || !clone.Dataflow.Complete {
		t.Fatal("missing finite collision model")
	}
	p.Aggregations[1].Column = "mutated"
	p.Dataflow.Bindings[1].Field = "mutated"
	p.Conditions[0].FieldBinding.BindingID = "mutated"
	if clone.Aggregations[1].Column != "num3" || clone.Dataflow.Bindings[1].Field != "num3" || clone.Conditions[0].FieldBinding.BindingID == "mutated" {
		t.Fatal("semantic clone aliases collision owners")
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
			{"metadata items", used.metadataItems, func(l *parseLimits, n int) { l.maxMetadataItems = n }},
			{"metadata bytes", used.metadataBytes, func(l *parseLimits, n int) { l.maxMetadataBytes = n }},
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
				t.Errorf("one-short %s semantic%v leaked: %v", resource.name, semantic, errs)
			}
		}
	}
	control := ExtractConditions(`T | summarize BitsAnd=binary_all_and(num), BitsOr=binary_all_or(num)`)
	if !reflect.DeepEqual(control.ComputedFields, map[string]string{"BitsAnd": "binary_all_and(num)", "BitsOr": "binary_all_or(num)"}) {
		t.Errorf("authored computed aliases changed: %+v", control.ComputedFields)
	}
}

func TestWave5dAllocatorSpellingAndReservations(t *testing.T) {
	a := newSummarizeNameAllocator(6, nil)
	for _, tc := range []struct{ base, want string }{{"num1", "num1"}, {"num1", "num11"}, {"num", "num"}, {"Num", "Num"}, {"num", "num2"}} {
		if got := a.add(tc.base); got != tc.want {
			t.Errorf("Add(%q)=%q want %q", tc.base, got, tc.want)
		}
	}
	// A declaration after an undeclared base records the allocated suffix, not
	// the requested spelling; repeated requests may therefore remain valid.
	a = newSummarizeNameAllocator(4, nil)
	a.add("N")
	for _, want := range []string{"N1", "N2"} {
		if got, valid := a.declare("N"); !valid || got != want {
			t.Errorf("allocated declaration: %q / %v", got, valid)
		}
	}
	if _, valid := a.declare("N1"); valid {
		t.Error("accepted an already declared allocated name")
	}

	limits := defaultParseLimits()
	limits.maxWork = 9
	b := &parseBudget{ctx: context.Background(), limits: limits}
	a = &summarizeNameAllocator{used: map[string]int{"num": 0, "num1": 0}, declared: map[string]bool{}, budget: b}
	// Lookup=4, loop=1, decimal=1, base-copy=3: nine units precede the
	// candidate's final byte. The tenth must be reserved before constructing it.
	func() {
		defer func() {
			v := recover()
			if _, ok := v.(budgetAbort); !ok {
				t.Fatalf("candidate construction was not precharged: %v", v)
			}
		}()
		a.add("num")
	}()
	if b.work != 9 || !reflect.DeepEqual(a.used, map[string]int{"num": 0, "num1": 0}) || len(a.declared) != 0 {
		t.Fatalf("failed construction mutated allocation state: work=%d used=%v", b.work, a.used)
	}
	ctx := &wave5dCheckpointContext{Context: context.Background(), remaining: 8}
	b = &parseBudget{ctx: ctx, limits: defaultParseLimits()}
	a.budget = b
	func() {
		defer func() {
			v := recover()
			aborted, ok := v.(budgetAbort)
			if !ok || aborted.err != context.Canceled {
				t.Fatalf("collision ignored cancellation: %v", v)
			}
		}()
		a.add("num")
	}()
	if ctx.remaining != 0 || !reflect.DeepEqual(a.used, map[string]int{"num": 0, "num1": 0}) {
		t.Fatalf("canceled collision continued/mutated: %+v / %v", ctx, a.used)
	}
}

type wave5dCheckpointContext struct {
	context.Context
	remaining int
}

func (c *wave5dCheckpointContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}

// Unnamed scalar projections need Kusto's contextual ColumnN allocation,
// which this finite-name graph deliberately does not implement.
func TestWave5dR1MissingBaseNamesStayOpaque(t *testing.T) {
	for _, tc := range []struct {
		query   string
		columns []string
	}{
		{`T | summarize 1, 1`, []string{"", ""}},
		{`T | summarize sum(Bytes), sum(Bytes), 1`, []string{"sum_Bytes", "sum_Bytes", ""}},
	} {
		for api, m := range wave5bModels(tc.query) {
			if len(m.errors) != 0 || len(m.aggs) != len(tc.columns) {
				t.Fatalf("api%d lost original scalar summaries %q: %+v", api, tc.query, m)
			}
			if m.flow == nil || m.flow.Complete || len(m.flow.Stages) != 1 || m.flow.Stages[0].Effect != "opaque" || len(m.flow.Bindings) != 0 || len(m.flow.Stages[0].BindingIDs) != 0 {
				t.Errorf("api%d missing base invented finite names %q: %+v", api, tc.query, m)
			}
			for i, a := range m.aggs {
				if a.Column != tc.columns[i] || a.StageID != "" || a.BindingID != "" {
					t.Errorf("api%d item%d partial scalar-name publication: %+v want column%q", api, i, a, tc.columns[i])
				}
			}
		}
	}
}

func TestWave5dR1ExplicitScalarAliasesStayFinite(t *testing.T) {
	const query = `T | summarize A=1, B=1 | where B > 0`
	for api, m := range wave5bModels(query) {
		if len(m.errors) != 0 || len(m.warnings) != 0 || len(m.aggs) != 2 || m.flow == nil || !m.flow.Complete || len(m.flow.Bindings) != 2 {
			t.Fatalf("api%d explicit scalar aliases unavailable: %+v", api, m)
		}
		for i, want := range []string{"A", "B"} {
			a, b := m.aggs[i], m.flow.Bindings[i]
			if a.Column != want || b.Field != want || b.Kind != "aggregate" || b.Expression != "1" || a.StageID != b.StageID || a.BindingID != b.ID || a.BindingID == "" {
				t.Errorf("api%d scalar owner%d: %+v / %+v", api, i, a, b)
			}
		}
		if m.aggs[0].BindingID == m.aggs[1].BindingID || len(m.conditions) != 1 || m.conditions[0].Field != "B" || m.conditions[0].FieldBinding == nil || m.conditions[0].FieldBinding.Kind != "binding" || m.conditions[0].FieldBinding.BindingID != m.aggs[1].BindingID {
			t.Errorf("api%d B did not bind second scalar occurrence: %+v", api, m.conditions)
		}
	}
}
