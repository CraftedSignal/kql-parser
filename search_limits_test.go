package kql

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"
)

func TestSearchHelpersUseSharedWorkAndOutputBudgets(t *testing.T) {
	query := `search in (T,U*) "needle"`
	lexer := NewKQLLexer(antlr.NewInputStream(query))
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	ctx := NewKQLParser(stream).Query().TabularExpression().TabularSource().SearchOperator()
	for _, phase := range []string{"capture", "header", "restore", "expression", "clone"} {
		t.Run(phase, func(t *testing.T) {
			limits := defaultParseLimits()
			limits.maxWork = 0
			budget, cancel := newParseBudget(context.Background(), limits)
			defer cancel()
			defer requireBudgetAbort(t, "work budget")
			switch phase {
			case "capture":
				protectSearchClauses(query, budget)
			case "header":
				searchPredicateContext(ctx, budget)
			case "restore":
				(&searchProtection{}).restore(query, budget)
			case "expression":
				collectKQLFilterOperators(ctx, budget)
			case "clone":
				cloneSearchPredicate(&SearchPredicate{TableSources: []string{"T"}}, budget)
			}
		})
	}
	for _, phase := range []string{"leaf", "clone", "semantic"} {
		t.Run("output "+phase, func(t *testing.T) {
			limits := defaultParseLimits()
			limits.maxOutputItems = 0
			budget, cancel := newParseBudget(context.Background(), limits)
			defer cancel()
			defer requireBudgetAbort(t, "output items")
			switch phase {
			case "leaf":
				searchPredicateContext(ctx, budget)
			case "clone":
				cloneSearchPredicate(&SearchPredicate{TableSources: []string{"T"}}, budget)
			case "semantic":
				semanticExpressionFromParseExpression(&ConditionExpression{Operator: BooleanSearch, Search: &SearchPredicate{Text: `"x"`, Scope: "database"}}, nil, budget)
			}
		})
	}
}

func TestSearchRestorationChecksCumulativeBytesBeforeGrowth(t *testing.T) {
	p := &searchProtection{clauses: []protectedSearchClause{{marker: "a", source: `search "` + strings.Repeat("x", 35) + `" `}, {marker: "b", source: `search "` + strings.Repeat("y", 35) + `"`}}}
	// Each source fits by itself; the two restored clauses together do not.
	limits := defaultParseLimits()
	limits.maxInputBytes = 70
	budget, cancel := newParseBudget(context.Background(), limits)
	defer cancel()
	defer requireBudgetAbort(t, "input bytes")
	p.restore(`search "a" | search "b"`, budget)
}

func TestSearchPublicLimitsReturnFailureOnly(t *testing.T) {
	query := `T | where x == 1 | search "needle" | join (U | search "other") on Id`
	for _, resource := range []string{"work", "output", "tokens", "nesting", "bytes"} {
		limits := defaultParseLimits()
		switch resource {
		case "work":
			limits.maxWork = 0
		case "output":
			limits.maxOutputItems = 3
		case "tokens":
			limits.maxTokens = 5
		case "nesting":
			limits.maxNestingDepth = 0
		case "bytes":
			limits.maxInputBytes = 15
		}
		r := extractConditionsContext(context.Background(), query, limits)
		if len(r.Errors) != 1 || len(r.Conditions) != 0 || r.Expression != nil || len(r.Joins) != 0 {
			t.Errorf("%s returned partial extraction: %#v", resource, r)
		}
		s := analyzeQueryContext(context.Background(), query, limits)
		if len(s.Errors) != 1 || len(s.Conditions) != 0 || s.Expression != nil || len(s.Joins) != 0 {
			t.Errorf("%s returned partial semantics: %#v", resource, s)
		}
		if errs := validateContext(context.Background(), query, limits); len(errs) == 0 {
			t.Errorf("%s validation ignored shared budget", resource)
		}
	}
	limits := defaultParseLimits()
	limits.maxNestingDepth = 1
	r := extractConditionsContext(context.Background(), `T | search "(([[{{|}}]]))" // (((
`, limits)
	if len(r.Errors) > 0 || len(searchLeaves(r.Expression)) != 1 {
		t.Fatalf("literal/comment delimiters counted as nesting: %#v", r)
	}
	ctx := panicDeadlineContext{context.Background()}
	if r := ExtractConditionsContext(ctx, `search "x"`); len(r.Errors) != 1 || r.Expression != nil || len(r.Conditions) != 0 {
		t.Fatalf("panic returned usable search: %#v", r)
	}
	if s := AnalyzeQueryContext(ctx, `search "x"`); len(s.Errors) != 1 || s.Expression != nil || len(s.Conditions) != 0 || len(s.Joins) != 0 {
		t.Fatalf("panic returned usable semantic search: %#v", s)
	}
	if errs := ValidateContext(ctx, `search "x"`); len(errs) != 1 || !strings.Contains(errs[0], "parser panic") {
		t.Fatalf("panic escaped search validation: %v", errs)
	}
}

func TestSearchConcurrentRequestsAndCancellationStayIsolated(t *testing.T) {
	for worker := 0; worker < 8; worker++ {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			for iteration := 0; iteration < 4; iteration++ {
				term := fmt.Sprintf(`"worker-%d-%d"`, worker, iteration)
				query := `search in (T,U*) ` + term
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if r := ExtractConditionsContext(ctx, query); len(r.Errors) != 1 || r.Expression != nil || len(r.Conditions) != 0 {
					t.Fatalf("canceled extract returned search: %#v", r)
				}
				if r := AnalyzeQueryContext(ctx, query); len(r.Errors) != 1 || r.Expression != nil || len(r.Conditions) != 0 {
					t.Fatalf("canceled semantic returned search: %#v", r)
				}
				if errs := ValidateContext(ctx, query); len(errs) != 1 || !strings.Contains(errs[0], "context canceled") {
					t.Fatalf("canceled validation: %v", errs)
				}
				r := ExtractConditions(query)
				leaves := searchLeaves(r.Expression)
				if len(r.Errors) > 0 || len(leaves) != 1 || leaves[0].Text != term {
					t.Fatalf("request source mixed: %#v / %#v", r, leaves)
				}
				s := AnalyzeQuery(query)
				if len(s.Errors) > 0 || s.Expression == nil || s.Expression.Search == nil || s.Expression.Search.Text != term {
					t.Fatalf("request semantics mixed: %#v", s)
				}
				if errs := Validate(query); len(errs) > 0 {
					t.Fatalf("independent validation: %v", errs)
				}
			}
		})
	}
}
