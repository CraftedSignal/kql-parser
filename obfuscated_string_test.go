package kql

import (
	"reflect"
	"testing"
)

// An obfuscated string literal is an h or H before a standard or verbatim
// string literal (Microsoft Learn, "The string data type"). It has the value
// of the literal it marks; only query logs hide it.
func TestExtractConditions_ObfuscatedStringLiteral(t *testing.T) {
	for _, plain := range []string{`"secret"`, `'secret'`, `@"C:\secret"`, `@'it''s'`, `"a\"b"`} {
		for _, prefix := range []string{"h", "H"} {
			obfuscated := prefix + plain
			t.Run(obfuscated, func(t *testing.T) {
				got := ExtractConditions(`SecurityEvent | where Password == ` + obfuscated)
				want := ExtractConditions(`SecurityEvent | where Password == ` + plain)
				if len(got.Errors) != 0 || len(got.Conditions) != 1 || got.Conditions[0].ValueReference != "" {
					t.Fatalf("conditions = %+v errors=%q; want one literal comparison", got.Conditions, got.Errors)
				}
				condition, plainCondition := got.Conditions[0], want.Conditions[0]
				if condition.Field != "Password" || condition.Value != plainCondition.Value || condition.RHS == nil || condition.RHS.Kind != OperandString || condition.RHS.Value != plainCondition.RHS.Value || condition.RHS.Source != obfuscated {
					t.Fatalf("condition = %+v RHS=%+v; want the value of %s with source %s", condition, condition.RHS, plain, obfuscated)
				}
			})
		}
	}
	result := ExtractConditions(`SecurityEvent | where Account in (h"alice", H'bob') and CommandLine has h@"token"`)
	if len(result.Errors) != 0 || len(result.Conditions) != 2 || !reflect.DeepEqual(result.Conditions[0].Alternatives, []string{"alice", "bob"}) || result.Conditions[1].Value != "token" {
		t.Fatalf("conditions = %+v errors=%q; want in (alice, bob) and has token", result.Conditions, result.Errors)
	}
}
