package kql

import "testing"

func TestExpressionPreservesGroupedNotAnd(t *testing.T) {
	result := ExtractConditions(`events | where not((CommandLine contains "whoami" and Image endswith "cmd.exe" and ParentImage endswith "winword.exe"))`)
	if result.Expression == nil {
		t.Fatalf("expected expression, got nil with conditions %#v errors %v", result.Conditions, result.Errors)
	}
	if result.Expression.Operator != BooleanOr {
		t.Fatalf("expected grouped NOT AND to become OR of negated leaves, got %#v", result.Expression)
	}
	if len(result.Expression.Children) != 3 {
		t.Fatalf("expected three detection children, got %#v", result.Expression)
	}
	for _, condition := range result.Conditions {
		if !condition.Negated {
			t.Fatalf("expected condition to remain negated in flat output: %#v", result.Conditions)
		}
	}
}
