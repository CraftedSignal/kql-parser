package kql

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func requireIdentityModels(t *testing.T, query string) (*ParseResult, *SemanticQuery) {
	t.Helper()
	p, s := ExtractConditions(query), AnalyzeQuery(query)
	if len(p.Errors)+len(s.Errors) != 0 || p.Expression == nil || s.Expression == nil || p.Dataflow == nil || s.Dataflow == nil || !p.Dataflow.Complete || !s.Dataflow.Complete {
		t.Fatalf("expected complete public models for %q: parse=%+v semantic=%+v", query, p, s)
	}
	if !reflect.DeepEqual(p.Conditions, s.Conditions) || !reflect.DeepEqual(p.DataSources, s.DataSources) || !reflect.DeepEqual(p.Dataflow, s.Dataflow) {
		t.Fatal("public identity carriers differ")
	}
	return p, s
}

func TestIdentityLiteralPublicWitnesses(t *testing.T) {
	for _, literal := range []string{`"Ev\u0065nt"`, `"Ev\x65nt"`, `h"Ev\u0065nt"`, `"Event"`, `@"Event"`} {
		t.Run("table/"+literal, func(t *testing.T) {
			p, _ := requireIdentityModels(t, "table("+literal+`) | where Field == "x"`)
			if !reflect.DeepEqual(p.DataSources, []string{"Event"}) {
				t.Fatalf("decoded table identities = %q, want Event", p.DataSources)
			}
		})
	}
	for _, reference := range []string{`['F\u0069eld']`, `["F\x69eld"]`, `['Field']`} {
		t.Run("lhs/"+reference, func(t *testing.T) {
			p, _ := requireIdentityModels(t, "Event | where "+reference+` == "x"`)
			if len(p.Conditions) != 1 || p.Conditions[0].Field != "Field" || p.Conditions[0].FieldBinding == nil || p.Conditions[0].FieldBinding.Kind != "input" || p.Conditions[0].FieldBinding.ScopeID != p.Conditions[0].InputScopeID {
				t.Fatalf("decoded field/binding = %+v", p.Conditions)
			}
		})
	}
	t.Run("typed RHS", func(t *testing.T) {
		const reference = `['F\u0069eld']`
		p, _ := requireIdentityModels(t, "Event | where Other == "+reference)
		if len(p.Conditions) != 1 {
			t.Fatalf("conditions = %+v", p.Conditions)
		}
		c := p.Conditions[0]
		if c.RHS == nil || c.RHS.Kind != OperandField || c.RHS.Source != reference || c.RHS.Value != "Field" || c.ValueReference != "Field" || c.RHS.FieldBinding == nil || c.RHS.FieldBinding.Kind != "input" || c.RHS.FieldBinding.ScopeID != c.InputScopeID {
			t.Fatalf("decoded RHS/binding = %+v / %+v", c, c.RHS)
		}
	})
}

func TestIdentityLiteralRawSourceAndStringControls(t *testing.T) {
	const source = `cluster("clu\u0073ter").database("d\u0062").Event`
	p, _ := requireIdentityModels(t, source+` | where Field == "F\u0069eld"`)
	if len(p.Pipelines) != 1 || p.Pipelines[0].Source != source || p.Dataflow.Scopes[0].Source != source || !reflect.DeepEqual(p.DataSources, []string{"Event"}) {
		t.Fatalf("authored source changed: %+v / %+v", p.Pipelines, p.Dataflow)
	}
	if c := p.Conditions[0]; c.Value != `F\u0069eld` || c.RHS == nil || c.RHS.Source != `"F\u0069eld"` || c.RHS.Value != "Field" || c.RHS.Kind != OperandString {
		t.Fatalf("typed string control changed: %+v", c)
	}
	// The legacy reader serves portable scalar/list/search paths; identity
	// decoding must not globally change its existing values or tail contract.
	if value, tail, ok := parseStringLiteral(`"F\u0069eld" tail`); !ok || value != "Fu0069eld" || tail != " tail" {
		t.Fatalf("legacy literal changed: %q %q %v", value, tail, ok)
	}
	if values, ok := parseListValue(`("F\u0069eld", @"F\u0069eld")`, nil); !ok || !reflect.DeepEqual(values, []string{"Fu0069eld", `F\u0069eld`}) {
		t.Fatalf("legacy portable list changed: %q %v", values, ok)
	}
	if value := extractLeadingSearchKeyword(`search "F\u0069eld"`); value != "Fu0069eld" {
		t.Fatalf("legacy portable search changed: %q", value)
	}
}

func TestIdentityLiteralNestedBooleanAndDistinctSlash(t *testing.T) {
	const query = `Event | where (['F\u0069eld'] == "a" or Other == "b") and [@'F\u0069eld'] == Other`
	p, s := requireIdentityModels(t, query)
	if len(p.Conditions) != 3 || p.Conditions[0].Field != "Field" || p.Conditions[1].Field != "Other" || p.Conditions[2].Field != `F\u0069eld` || p.Conditions[2].RHS == nil || p.Conditions[2].RHS.Kind != OperandField || p.Conditions[2].RHS.Value != "Other" {
		t.Fatalf("same decoded/distinct literal identities lost: %+v", p.Conditions)
	}
	if s.Expression.Operator != BooleanAnd || len(s.Expression.Children) != 2 || s.Expression.Children[0].Operator != BooleanOr || len(s.Expression.Children[0].Children) != 2 || s.Expression.Children[1].Condition == nil || s.Expression.Children[1].Condition.Field != `F\u0069eld` {
		t.Fatalf("nested grouping changed: %+v", s.Expression)
	}
	p, _ = requireIdentityModels(t, `Event | where ['F\u0069eld'] == "a" or Field == "b"`)
	if len(p.Conditions) != 1 || p.Conditions[0].Field != "Field" || !reflect.DeepEqual(p.Conditions[0].Alternatives, []string{"a", "b"}) {
		t.Fatalf("equivalent field spellings did not share the existing OR representation: %+v", p.Conditions)
	}
}

func TestIdentityLiteralMalformedPublic(t *testing.T) {
	for _, query := range []string{
		`table() | where Field == "x"`,
		`table("Event" junk) | where Field == "x"`,
		`table("Event) | where Field == "x"`,
		`Event | where ['Field' junk] == "x"`,
		`Event | where ['F\qield'] == "x"`,
	} {
		t.Run(query, func(t *testing.T) {
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			if len(p.Errors) == 0 || len(s.Errors) == 0 {
				t.Fatalf("malformed identity accepted: %+v / %+v", p, s)
			}
			if len(p.Conditions) != 0 || p.Expression != nil || p.Dataflow != nil || len(s.Conditions) != 0 || s.Expression != nil || s.Dataflow != nil {
				t.Fatalf("malformed identity exposed partial model: %+v / %+v", p, s)
			}
			for _, sources := range [][]string{p.DataSources, s.DataSources} {
				for _, source := range sources {
					if source == "table" {
						t.Errorf("fabricated table identity from %q", query)
					}
				}
			}
		})
	}
}

func TestIdentityLiteralExplicitTableNames(t *testing.T) {
	for _, name := range []string{"T", "Event.Log", "Event Log", "Event-Log", "event"} {
		p, _ := requireIdentityModels(t, `table("`+name+`") | where Field == "x"`)
		if !reflect.DeepEqual(p.DataSources, []string{name}) {
			t.Fatalf("explicit table %q became %q", name, p.DataSources)
		}
		// A function source is an expression in the existing pipeline model;
		// do not replace that authored-source contract with guessed text.
		if p.Pipelines[0].Source != "" || p.Dataflow.Scopes[0].Source != "" {
			t.Fatalf("explicit table changed source representation: %+v", p.Pipelines)
		}
	}
	p, _ := requireIdentityModels(t, `let T=Event; table("T") | where Field == "x"`)
	if !reflect.DeepEqual(p.DataSources, []string{"Event", "T"}) {
		t.Fatalf("explicit physical table confused with let binding: %q", p.DataSources)
	}
}

func TestIdentityLiteralValidUnsupportedTableForms(t *testing.T) {
	// These are valid vendor syntax whose baseline metadata was unfaithful;
	// no constant evaluator or data-scope representation is introduced here.
	for _, query := range []string{
		`table("Event", "hotcache") | where Field == "x"`,
		`table("Event", "all") | where Field == "x"`,
		`table("Event", "default") | where Field == "x"`,
		`let n="Event"; table(n) | where Field == "x"`,
		`table("Ev" "ent") | where Field == "x"`,
	} {
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		if !strings.Contains(strings.Join(p.Errors, " "), "source unavailable: unsupported constant table identity literal") || !strings.Contains(strings.Join(s.Errors, " "), "source unavailable: unsupported constant table identity literal") {
			t.Fatalf("unsupported table form was not explicit unavailable: %+v / %+v", p, s)
		}
		if len(p.Conditions) != 0 || p.Expression != nil || p.Dataflow != nil || len(p.DataSources) != 0 || len(s.Conditions) != 0 || s.Expression != nil || s.Dataflow != nil || len(s.DataSources) != 0 {
			t.Fatalf("unsupported table form exposed partial identity: %+v / %+v", p, s)
		}
	}
}

func TestIdentityLiteralCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, query := range []string{`Event | where ['F\u0069eld'] == "x"`, `table("Ev\u0065nt") | where Field == "x"`} {
		p, s := ExtractConditionsContext(ctx, query), AnalyzeQueryContext(ctx, query)
		if len(p.Conditions) != 0 || p.Expression != nil || p.Dataflow != nil || !strings.Contains(strings.Join(p.Errors, " "), "context canceled") {
			t.Fatalf("canceled parse exposed partial model: %+v", p)
		}
		if len(s.Conditions) != 0 || s.Expression != nil || s.Dataflow != nil || !strings.Contains(strings.Join(s.Errors, " "), "context canceled") {
			t.Fatalf("canceled analysis exposed partial model: %+v", s)
		}
	}
}

func TestIdentityLiteralExtentAndGrammarBoundary(t *testing.T) {
	for _, tc := range []struct{ literal, value string }{
		{`'F\u0069eld'`, "Field"}, {`"F\x69eld"`, "Field"},
		{`h"F\u0069eld"`, "Field"}, {`H'F\u0069eld'`, "Field"},
		{`@'F\u0069eld'`, `F\u0069eld`}, {`h@'F\u0069eld'`, `F\u0069eld`}, {`H@"F\u0069eld"`, `F\u0069eld`},
		{`'A\'B'`, "A'B"}, {`"A\"B"`, `A"B`}, {`"A\\B"`, `A\B`},
		{`@"A""B"`, `A"B`}, {`@'A''B'`, "A'B"},
		{`"\u00E9"`, "é"}, {`"\uD83D\uDE00"`, "😀"}, {`"é"`, "é"},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			value, tail, ok := identityLiteral(" \t"+tc.literal+" trailing ", nil)
			if !ok || value != tc.value || tail != " trailing " {
				t.Fatalf("identity literal = %q, %q, %v; want %q with exact tail", value, tail, ok, tc.value)
			}
		})
	}
	for _, literal := range []string{`"unterminated`, `'A\`, `'\q'`, `'\x1'`, `'\u123'`, `'\u12xz'`, `"\U0001F600"`, "\"a\nb\""} {
		if value, _, ok := identityLiteral(literal, nil); ok {
			t.Errorf("unsupported/incomplete literal %q invented value %q", literal, value)
		}
	}
	// Uppercase-U is not admitted by the current lexer; reuse of the broader
	// existing decoder must not silently extend the identity grammar.
	for _, query := range []string{`Event | where ['F\U00000069eld'] == "x"`, `table("Ev\U00000065nt") | where Field == "x"`} {
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		if len(p.Errors) == 0 || len(s.Errors) == 0 || len(p.Conditions) != 0 || len(s.Conditions) != 0 || p.Dataflow != nil || s.Dataflow != nil {
			t.Fatalf("unsupported identity escape exposed a model: %+v / %+v", p, s)
		}
	}
}

func TestIdentityLiteralAdmittedPublicForms(t *testing.T) {
	for _, tc := range []struct{ literal, value string }{
		{`h"F\u0069eld"`, "Field"}, {`H'F\u0069eld'`, "Field"},
		{`h@'F\u0069eld'`, `F\u0069eld`}, {`@"A""B"`, `A"B`},
		{`'A\'B'`, "A'B"}, {`"A\\B"`, `A\B`}, {`'étail'`, "étail"}, {`'\u00E9tail'`, "étail"},
		{`'\uD83D\uDE00tail'`, "😀tail"},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			reference := "[" + tc.literal + "]"
			p, _ := requireIdentityModels(t, "Event | where "+reference+` == Other`)
			if len(p.Conditions) != 1 || p.Conditions[0].Field != tc.value || p.Conditions[0].RHS == nil || p.Conditions[0].RHS.Kind != OperandField || p.Conditions[0].RHS.Value != "Other" {
				t.Fatalf("admitted field identity = %+v; want %q", p.Conditions, tc.value)
			}
		})
	}
	for _, literal := range []string{`'Event'`, `H'Ev\u0065nt'`, `h@"Event"`, `H@'Event'`} {
		p, _ := requireIdentityModels(t, "table("+literal+`) | where Field == "x"`)
		if !reflect.DeepEqual(p.DataSources, []string{"Event"}) {
			t.Fatalf("%s: table identity = %q", literal, p.DataSources)
		}
	}
}

func TestIdentityLiteralSharedBudgetBoundaries(t *testing.T) {
	for _, query := range []string{
		`Event | where ['F\u0069eld'] == "x" or ['F\u0069eld'] == Other`,
		`table("Ev\u0065nt") | where ['F\u0069eld'] == "x" or ['F\u0069eld'] == Other`,
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
				{"output items", max(used.output, returnedCount(p)), func(l *parseLimits, n int) { l.maxOutputItems = n }},
				{"metadata items", used.metadataItems, func(l *parseLimits, n int) { l.maxMetadataItems = n }},
				{"metadata bytes", used.metadataBytes, func(l *parseLimits, n int) { l.maxMetadataBytes = n }},
				{"input bytes", len(query), func(l *parseLimits, n int) { l.maxInputBytes = n }},
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

func TestIdentityLiteralRecursiveSourceBudget(t *testing.T) {
	const query = `table("Ev\u0065nt") | join (table("Oth\u0065r") | where Field == "x") on Field`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	if got := extractDataSourcesFromQuery(query, nil, budget); !reflect.DeepEqual(got, []string{"Event", "Other"}) {
		t.Fatalf("recursive source identities = %q", got)
	}
	used := budget.work
	for _, limit := range []int{used, used - 1} {
		limits := defaultParseLimits()
		limits.maxWork = limit
		b, cancel := newParseBudget(context.Background(), limits)
		func() {
			defer func() {
				got := recover()
				if limit == used && got != nil {
					t.Errorf("exact recursive source budget rejected: %v", got)
				}
				if limit < used && (got == nil || !strings.Contains(parseFailure(got).Error(), "work budget")) {
					t.Errorf("one-short recursive source budget was not shared: %v", got)
				}
			}()
			_ = extractDataSourcesFromQuery(query, nil, b)
		}()
		cancel()
	}
}
