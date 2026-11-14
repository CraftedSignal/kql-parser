package kql

import (
	"context"
	"regexp"
	"strings"
)

// Validate reports the syntax errors Kusto would raise for a query. Unlike
// ExtractConditions, which reads what it can from messy text, it rejects text
// that is not a single query: a write-up around the query, a second query after
// the first, a query that starts with an operator, or another language. The
// bodies of let statements are not validated.
func Validate(query string) []string {
	return ValidateContext(context.Background(), query)
}

// ValidateContext validates within the same cooperative limits as extraction.
func ValidateContext(ctx context.Context, query string) []string {
	return validateContext(ctx, query, defaultParseLimits())
}

func validateContext(ctx context.Context, query string, limits parseLimits) (errs []string) {
	defer func() {
		if value := recover(); value != nil {
			errs = []string{parseFailure(value).Error()}
		}
	}()
	budget, cancel := newParseBudget(ctx, limits)
	defer cancel()
	budget.input(query)
	preflight(query, budget, false, nil)
	if strings.TrimSpace(query) == "" {
		return []string{"query is empty"}
	}
	normalized := normalizeBudgeted(query, true, budget)
	listener := &errorListener{budget: budget}
	stream := preflight(normalized, budget, true, listener)
	parser := NewKQLParser(&budgetTokenStream{CommonTokenStream: stream, budget: budget})
	parser.RemoveErrorListeners()
	parser.AddErrorListener(listener)
	parser.Query()
	budget.check()
	for i, msg := range listener.errors {
		budget.step()
		listener.errors[i] = shortenSyntaxError(msg)
	}
	return listener.errors
}

var expectedTokensRe = regexp.MustCompile(` expecting \{[^}]*\}$`)

// shortenSyntaxError drops a long list of expected tokens: where any operator
// or expression may follow, the parser names every token that can start one.
func shortenSyntaxError(msg string) string {
	if loc := expectedTokensRe.FindStringIndex(msg); loc != nil && strings.Count(msg[loc[0]:], ",") > 5 {
		return msg[:loc[0]]
	}
	return msg
}
