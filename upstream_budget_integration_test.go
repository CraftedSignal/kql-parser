package kql

import (
	"context"
	"strings"
	"testing"
)

func TestFunctionPredicateTraversalUsesSharedWorkBudget(t *testing.T) {
	tree := kqlExpressionForTest(t, `isnull(b)`)
	limits := defaultParseLimits()
	limits.maxWork = 2
	budget, cancel := newParseBudget(context.Background(), limits)
	defer cancel()
	defer func() {
		abort, ok := recover().(budgetAbort)
		if !ok || !strings.Contains(abort.err.Error(), "work budget") {
			t.Fatalf("predicate traversal bypassed shared work budget: %+v", abort)
		}
	}()
	_, _ = recognizeKQLFunctionPredicate(tree, budget)
}

func TestFunctionPredicateReservesOutputBeforeAppending(t *testing.T) {
	query := kqlParserForTest(t, `T | where isempty(b)`).Query()
	wheres := collectKQLWhereOperators(query, nil)
	comparison := wheres[0].Expression().OrExpression().AllAndExpression()[0].AllNotExpression()[0].ComparisonExpression()
	limits := defaultParseLimits()
	limits.maxOutputItems = 0
	budget, cancel := newParseBudget(context.Background(), limits)
	defer cancel()
	extractor := &conditionExtractor{budget: budget}
	defer func() {
		abort, ok := recover().(budgetAbort)
		if !ok || !strings.Contains(abort.err.Error(), "output items") || len(extractor.conditions) != 0 {
			t.Fatalf("predicate appended before output reservation: abort=%+v conditions=%+v", abort, extractor.conditions)
		}
	}()
	extractor.EnterComparisonExpression(comparison.(*ComparisonExpressionContext))
}
