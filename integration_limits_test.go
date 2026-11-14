package kql

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestScalarLeavesUseSharedOutputBudget(t *testing.T) {
	limits := defaultParseLimits()
	limits.maxOutputItems = 0
	result := extractConditionsContext(context.Background(), `T | where true`, limits)
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "output items") || len(result.Conditions) != 0 || result.Expression != nil {
		t.Fatalf("scalar output escaped limit or returned partial result: %#v", result)
	}
}

func TestScalarSemanticConversionUsesSharedOutputBudget(t *testing.T) {
	limits := defaultParseLimits()
	limits.maxOutputItems = 0
	budget, cancel := newParseBudget(context.Background(), limits)
	defer cancel()
	defer requireBudgetAbort(t, "output items")
	semanticExpressionFromParseExpression(&ConditionExpression{Operator: BooleanScalar, Scalar: &ScalarPredicate{Text: "true"}}, nil, budget)
}

func requireBudgetAbort(t *testing.T, reason string) {
	t.Helper()
	value := recover()
	abort, ok := value.(budgetAbort)
	if !ok || !strings.Contains(abort.err.Error(), reason) {
		t.Errorf("expected %s budget abort, got %#v", reason, value)
	}
}

func TestScalarScansUseSharedWorkBudget(t *testing.T) {
	comparison := kqlParserForTest(t, `tolower(Name) == "alice"`).ComparisonExpression()
	for _, name := range []string{"operand", "function", "let reference", "scalar scan", "marker generation", "marker restoration", "let shadowing"} {
		t.Run(name, func(t *testing.T) {
			limits := defaultParseLimits()
			limits.maxWork = 0
			budget, cancel := newParseBudget(context.Background(), limits)
			defer cancel()
			defer requireBudgetAbort(t, "work budget")
			switch name {
			case "operand":
				recognizeKQLOperand(comparison, budget)
			case "function":
				kqlFilterFunctionPredicate(comparison, budget)
			case "let reference":
				kqlContainsLetReference(comparison, map[string]bool{"name": true}, budget)
			case "scalar scan":
				containsScalarExpression(&ConditionExpression{Operator: BooleanScalar}, budget)
			case "marker generation":
				newKQLNormalizationMetadata("T | where true", budget)
			case "marker restoration":
				newKQLNormalizationMetadata("").restoreSubqueryMarkers(`T | where true`, budget)
			case "let shadowing":
				inlineLetConstants(`let limit=1; T | extend limit=2 | where x == limit`, []LetStatement{{Name: "limit", Expression: "1"}}, budget)
			}
		})
	}
}

func TestConcurrentScalarRequestsKeepBudgetsIndependent(t *testing.T) {
	for worker := 0; worker < 8; worker++ {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			for iteration := 0; iteration < 4; iteration++ {
				value := fmt.Sprintf("worker-%d-%d", worker, iteration)
				query := fmt.Sprintf(`T | where tolower(Name) == %q or User == %q`, value, value)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				failed := AnalyzeQueryContext(ctx, query)
				if len(failed.Errors) != 1 || len(failed.Conditions) != 0 || failed.Expression != nil {
					t.Fatalf("cancellation returned partial scalar result: %#v", failed)
				}
				parsed := ExtractConditionsContext(context.Background(), query)
				if len(parsed.Errors) != 0 || len(parsed.Conditions) != 1 || parsed.Conditions[0].Value != value || parsed.Expression == nil || parsed.Expression.Operator != BooleanOr {
					t.Fatalf("independent extraction lost its own result: %#v", parsed)
				}
				semantic := AnalyzeQueryContext(context.Background(), query)
				if len(semantic.Errors) != 0 || len(semantic.Conditions) != 1 || semantic.Conditions[0].Value != value || semantic.Expression == nil || semantic.Expression.Operator != BooleanOr || len(semantic.Expression.Children) != 2 || semantic.Expression.Children[0].Scalar == nil || semantic.Expression.Children[0].Scalar.Text != fmt.Sprintf(`tolower(Name) == %q`, value) {
					t.Fatalf("independent semantic request mixed scalar state: %#v", semantic)
				}
				if errs := ValidateContext(context.Background(), query); len(errs) != 0 {
					t.Fatalf("independent validation failed: %v", errs)
				}
			}
		})
	}
}
