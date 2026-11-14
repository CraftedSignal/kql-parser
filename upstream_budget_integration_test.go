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

func TestRecoveryScansUseSharedWorkBudget(t *testing.T) {
	structured := kqlParserForTest(t, `T | project a`).Query()
	search := kqlParserForTest(t, `T | search "needle"`).Query()
	for _, fixture := range []struct {
		name string
		work int
		scan func(*parseBudget)
	}{
		{"structured entry", 0, func(budget *parseBudget) { hasStructuredKQLQuery(structured, budget) }},
		{"search entry", 0, func(budget *parseBudget) { extractParsedKQLSearchKeyword(search, budget) }},
		{"search operator", 1, func(budget *parseBudget) { extractParsedKQLSearchKeyword(search, budget) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			limits := defaultParseLimits()
			limits.maxWork = fixture.work
			budget, cancel := newParseBudget(context.Background(), limits)
			defer cancel()
			defer requireBudgetAbort(t, "work budget")
			fixture.scan(budget)
		})
	}
}
