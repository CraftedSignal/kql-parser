package kql

import (
	"context"
	"strings"
	"testing"
)

func TestWave9aCompoundStringSources(t *testing.T) {
	for _, literal := range []string{`"a" 'b'`, `"a" "b"`, `"ab"`, "\"a\" // authored gap\n 'b'"} {
		query := "T | extend note=" + literal + " | where note==" + literal
		if errs := Validate(query); len(errs) != 0 {
			t.Errorf("%s: validation: %v", literal, errs)
		}
		result := ExtractConditions(query)
		if len(result.Errors) != 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Bindings) != 1 || len(result.Dataflow.Stages) != 2 || len(result.Conditions) != 1 {
			t.Errorf("%s: incomplete extraction: %+v", literal, result)
			continue
		}
		flow, c := result.Dataflow, result.Conditions[0]
		if flow.Stages[0].Source != "extend note="+literal || flow.Stages[1].Source != "where note=="+literal || flow.Bindings[0].Expression != literal || c.RHS == nil || c.RHS.Source != literal || c.RHS.Value != "ab" || c.Value != "ab" || c.FieldBinding == nil || c.FieldBinding.BindingID != flow.Bindings[0].ID {
			t.Errorf("%s: authored ownership/value lost: stages=%+v binding=%+v condition=%+v rhs=%+v", literal, flow.Stages, flow.Bindings, c, c.RHS)
		}
	}
}

func TestWave9aCompoundStringDecoderReservation(t *testing.T) {
	value := strings.Repeat("a", 3000)
	source := `"` + value + `"`
	limits := defaultParseLimits()
	limits.maxWork = 2 * len(value) // One complete scan and one fixed backing buffer.
	budget, cancel := newParseBudget(context.Background(), limits)
	defer cancel()
	if got := decodeKQLStringBudgeted(source, budget); got != value || budget.work != limits.maxWork {
		t.Fatalf("planned decoding did not fit its exact reservation: length=%d work=%d", len(got), budget.work)
	}
	limits.maxWork--
	short, stop := newParseBudget(context.Background(), limits)
	defer stop()
	var got string
	defer func() {
		failure := recover()
		if failure == nil || !strings.Contains(parseFailure(failure).Error(), "work budget") || got != "" || short.work != len(value) {
			t.Errorf("one-short decoder must reject before backing allocation/publication: got=%q work=%d failure=%v", got, short.work, failure)
		}
	}()
	got = decodeKQLStringBudgeted(source, short)
}

func TestWave9aCompoundStringFidelity(t *testing.T) {
	for _, tc := range []struct{ literal, value string }{
		{`"\n" @'\n''x'`, "\n\\n'x"},
		{`h'a' @"b"`, "ab"},
		{"```a``` 'b'", "ab"},
		{`"\0" @'b'`, "\x00b"},
		{`"\U0001F600" @"tail"`, "😀tail"},
	} {
		query := "T | where x == " + tc.literal
		if errs := Validate(query); len(errs) != 0 {
			t.Errorf("%s: validation: %v", tc.literal, errs)
		}
		result := ExtractConditions(query)
		if len(result.Errors) != 0 || len(result.Conditions) != 1 || result.Conditions[0].RHS == nil || result.Conditions[0].RHS.Value != tc.value || result.Conditions[0].RHS.Source != tc.literal {
			t.Errorf("%s: decoded/source mismatch: %+v", tc.literal, result)
		}
		if strings.HasPrefix(tc.literal, "h") && !strings.HasPrefix(joinAdjacentStringLiterals(tc.literal), "h") {
			t.Errorf("obfuscation lost: %q", joinAdjacentStringLiterals(tc.literal))
		}
	}
	if got := joinAdjacentStringLiterals(`"\x61" "\u0062"`); got != `"\x61\u0062"` {
		t.Errorf("homogeneous spelling changed: %q", got)
	}
	for _, literal := range []string{`"\q" @'b' 'c'`, `"\u12xz" @'b' 'c'`, `"\U00110000" @'b' 'c'`, `"\U0000D800" @'b' 'c'`, `"""a""" 'b'`, `"a" /* note */ 'b'`} {
		if got := joinAdjacentStringLiterals(literal); got != literal {
			t.Errorf("invalid token or whole-token boundary changed: %q -> %q", literal, got)
		}
	}
	if errs := Validate(`T | where x == "\q" @'b' 'c'`); len(errs) == 0 {
		t.Error("malformed mixed escape was repaired")
	}
}

func TestWave9aCompoundStringResourceFailure(t *testing.T) {
	query := `T | extend note="a" 'b' | where note=="a" 'b'`
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := ExtractConditionsContext(ctx, query); len(result.Errors) == 0 || result.Dataflow != nil || len(result.Conditions) != 0 {
		t.Fatalf("cancellation exposed partial result: %+v", result)
	}
	for _, limit := range []string{"work", "output"} {
		limits := defaultParseLimits()
		if limit == "work" {
			limits.maxWork = len(query)
		} else {
			limits.maxOutputItems = 0
		}
		result := extractConditionsContext(context.Background(), query, limits)
		if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], limit) || result.Dataflow != nil || len(result.Conditions) != 0 {
			t.Errorf("%s failure exposed partial result: %+v", limit, result)
		}
	}
}

func TestWave10InvalidUnicodeCompoundIsAtomic(t *testing.T) {
	for _, literal := range []string{`"\U00110000" @'b' 'c'`, `"\U0000D800" @'b' 'c'`} {
		query := "T | where x == " + literal
		if errs := Validate(query); len(errs) == 0 {
			t.Errorf("invalid U8 scalar accepted: %s", literal)
		}
		p, s := ExtractConditions(query), AnalyzeQuery(query)
		requireRejectedUnicodeModels(t, p, s)
	}
}
