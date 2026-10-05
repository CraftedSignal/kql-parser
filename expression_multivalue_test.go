package kql

import "testing"

func TestExpressionPreservesHasAllAsAnd(t *testing.T) {
	result := ExtractConditions(`events | where ProcessCommandLine has_all ("whoami", "/priv")`)
	if result.Expression == nil {
		t.Fatalf("expected expression, got nil conditions %#v errors %v", result.Conditions, result.Errors)
	}
	if result.Expression.Operator != BooleanAnd {
		t.Fatalf("expected has_all to become AND, got %#v", result.Expression)
	}
	if len(result.Expression.Children) != 2 {
		t.Fatalf("expected two has_all children, got %#v", result.Expression)
	}
}

func TestExpressionPreservesNegatedHasAllAsOr(t *testing.T) {
	result := ExtractConditions(`events | where not(ProcessCommandLine has_all ("whoami", "/priv"))`)
	if result.Expression == nil {
		t.Fatalf("expected expression, got nil conditions %#v errors %v", result.Conditions, result.Errors)
	}
	if result.Expression.Operator != BooleanOr {
		t.Fatalf("expected negated has_all to become OR, got %#v", result.Expression)
	}
	if len(result.Expression.Children) != 2 {
		t.Fatalf("expected two negated has_all children, got %#v", result.Expression)
	}
	for _, condition := range result.Conditions {
		if !condition.Negated {
			t.Fatalf("expected negated leaves, got %#v", result.Conditions)
		}
	}
}

func TestExpressionPreservesNegatedSameFieldOr(t *testing.T) {
	result := ExtractConditions(`events | where not(ProcessCommandLine contains "whoami") or not(ProcessCommandLine contains "/priv")`)
	if result.Expression == nil {
		t.Fatalf("expected expression, got nil conditions %#v errors %v", result.Conditions, result.Errors)
	}
	if result.Expression.Operator != BooleanOr || len(result.Expression.Children) != 2 {
		t.Fatalf("expected OR over two negated leaves, got %#v", result.Expression)
	}
	if len(result.Conditions) != 2 {
		t.Fatalf("expected two separate negated conditions, got %#v", result.Conditions)
	}
	for _, condition := range result.Conditions {
		if !condition.Negated || len(condition.Alternatives) != 0 {
			t.Fatalf("negated OR leaves must not merge into alternatives: %#v", result.Conditions)
		}
	}
}

func TestExpressionPreservesHasAnyAlternativesInGroupedLogic(t *testing.T) {
	result := ExtractConditions(`events | where (ProcessCommandLine has_any ("whoami", "hostname") and Image == "cmd.exe") or ParentImage == "winword.exe"`)
	if result.Expression == nil {
		t.Fatalf("expected expression, got nil conditions %#v errors %v", result.Conditions, result.Errors)
	}
	if len(result.Conditions) != 3 {
		t.Fatalf("expected compacted alternatives plus two scalar conditions, got %#v", result.Conditions)
	}
	if len(result.Conditions[0].Alternatives) != 2 {
		t.Fatalf("expected has_any alternatives to stay compacted, got %#v", result.Conditions[0])
	}
	if result.Expression.Operator != BooleanOr {
		t.Fatalf("expected grouped expression root OR, got %#v", result.Expression)
	}
	if len(result.Expression.Children) != 2 {
		t.Fatalf("expected two OR children, got %#v", result.Expression)
	}
	left := result.Expression.Children[0]
	if left.Operator != BooleanAnd || len(left.Children) != 2 {
		t.Fatalf("expected left OR branch to be AND(has_any, image), got %#v", left)
	}
	requireConditionIndex(t, left.Children[0], 0)
	requireConditionIndex(t, left.Children[1], 1)
	requireConditionIndex(t, result.Expression.Children[1], 2)
}

func TestExpressionPreservesGroupedSameFieldOrAlternatives(t *testing.T) {
	result := ExtractConditions(`events | where CommandLine contains "whoami" and (Image == "cmd.exe" or Image == "powershell.exe") and not(AccountName == "SYSTEM")`)
	if result.Expression == nil {
		t.Fatalf("expected expression, got nil conditions %#v errors %v", result.Conditions, result.Errors)
	}
	if len(result.Conditions) != 3 {
		t.Fatalf("expected grouped same-field OR plus surrounding predicates, got %#v", result.Conditions)
	}
	if len(result.Conditions[1].Alternatives) != 2 {
		t.Fatalf("expected Image alternatives to stay compacted, got %#v", result.Conditions[1])
	}
	if result.Expression.Operator != BooleanAnd || len(result.Expression.Children) != 3 {
		t.Fatalf("expected AND over command, compacted image alternatives, and negated account, got %#v", result.Expression)
	}
	for index, child := range result.Expression.Children {
		requireConditionIndex(t, child, index)
	}
}

func TestExpressionPreservesNotBetweenAsOr(t *testing.T) {
	result := ExtractConditions(`events | where Count !between (10 .. 100)`)
	if result.Expression == nil {
		t.Fatalf("expected expression, got nil conditions %#v errors %v", result.Conditions, result.Errors)
	}
	if result.Expression.Operator != BooleanOr {
		t.Fatalf("expected !between to become OR over negated bounds, got %#v", result.Expression)
	}
	if len(result.Expression.Children) != 2 {
		t.Fatalf("expected two !between children, got %#v", result.Expression)
	}
	for _, condition := range result.Conditions {
		if !condition.Negated {
			t.Fatalf("expected !between leaves to be negated, got %#v", result.Conditions)
		}
	}
}

func requireConditionIndex(t *testing.T, expression *ConditionExpression, index int) {
	t.Helper()
	if expression == nil || expression.Operator != BooleanCondition || expression.ConditionIndex != index {
		t.Fatalf("expected condition index %d, got %#v", index, expression)
	}
}
