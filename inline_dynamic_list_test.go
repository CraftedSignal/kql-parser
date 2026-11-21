package kql

import (
	"reflect"
	"testing"
)

// Kusto flattens a dynamic array operand of in, in~, !in, has_any and
// has_all into the value list (Microsoft Learn, in~ operator: "x in
// (dynamic([1,[2,3]])) becomes x in (1,2,3)"). An inline array of literals
// must give the same conditions as the same array bound with let.
func TestExtractConditions_InlineDynamicListExpands(t *testing.T) {
	tests := []struct {
		name     string
		inline   string
		bound    string
		wantRHS  []Operand
		wantList string
	}{
		{
			name:     "in~",
			inline:   `SecurityEvent | where Account in~ (dynamic(["x","y"]))`,
			bound:    "let l = dynamic([\"x\",\"y\"]);\nSecurityEvent | where Account in~ (l)",
			wantRHS:  []Operand{{Kind: OperandString, Source: `"x"`, Value: "x"}, {Kind: OperandString, Source: `"y"`, Value: "y"}},
			wantList: `(dynamic(["x","y"]))`,
		},
		{
			name:     "!in",
			inline:   `SecurityEvent | where EventID !in (dynamic([4624, 4625]))`,
			bound:    "let l = dynamic([4624, 4625]);\nSecurityEvent | where EventID !in (l)",
			wantRHS:  []Operand{{Kind: OperandNumber, Source: `4624`, Value: "4624"}, {Kind: OperandNumber, Source: `4625`, Value: "4625"}},
			wantList: `(dynamic([4624, 4625]))`,
		},
		{
			name:   "has_any",
			inline: `SecurityEvent | where CommandLine has_any (dynamic(["x","y"]))`,
			bound:  "let l = dynamic([\"x\",\"y\"]);\nSecurityEvent | where CommandLine has_any (l)",
		},
		{
			name:   "has_all",
			inline: `SecurityEvent | where CommandLine has_all (dynamic(["x","y"]))`,
			bound:  "let l = dynamic([\"x\",\"y\"]);\nSecurityEvent | where CommandLine has_all (l)",
		},
	}
	type shape struct {
		Field, Operator, Value string
		Alternatives           []string
		Negated                bool
	}
	shapes := func(result *ParseResult) []shape {
		var out []shape
		for _, condition := range result.Conditions {
			out = append(out, shape{condition.Field, condition.Operator, condition.Value, condition.Alternatives, condition.Negated})
		}
		return out
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inline, bound := ExtractConditions(tt.inline), ExtractConditions(tt.bound)
			if len(inline.Errors) != 0 || len(bound.Errors) != 0 || !reflect.DeepEqual(shapes(inline), shapes(bound)) || !reflect.DeepEqual(inline.Expression, bound.Expression) {
				t.Fatalf("inline conditions = %+v errors=%q\nlet-bound          %+v errors=%q", shapes(inline), inline.Errors, shapes(bound), bound.Errors)
			}
			if tt.wantRHS == nil {
				return
			}
			rhs := inline.Conditions[0].RHS
			if rhs == nil || rhs.Kind != OperandList || rhs.Source != tt.wantList || !reflect.DeepEqual(rhs.Items, tt.wantRHS) {
				t.Fatalf("RHS = %+v, want list %s with items %+v", rhs, tt.wantList, tt.wantRHS)
			}
		})
	}
}

// An array Kusto would flatten further, or an operand that is not an array of
// literals, cannot be lowered to values and stays one unevaluated predicate.
func TestExtractConditions_InlineDynamicNonLiteralListStaysScalar(t *testing.T) {
	for _, query := range []string{
		`SecurityEvent | where EventID in (dynamic([1,[2,3]]))`,
		`SecurityEvent | where Account in (dynamic({"a":1}))`,
	} {
		result := ExtractConditions(query)
		if len(result.Conditions) != 0 || result.Expression == nil || result.Expression.Operator != BooleanScalar {
			t.Errorf("%s: conditions=%+v expression=%+v, want one scalar predicate", query, result.Conditions, result.Expression)
		}
	}
}
