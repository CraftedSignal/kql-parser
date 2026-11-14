package kql

import (
	"fmt"
	"strings"
	"testing"
)

func predicateSemanticShape(expression *SemanticExpression) string {
	if expression == nil {
		return "nil"
	}
	if expression.Condition != nil {
		condition := expression.Condition
		return fmt.Sprintf("%s:%s:%t", condition.Field, condition.Operator, condition.Negated)
	}
	children := make([]string, len(expression.Children))
	for i, child := range expression.Children {
		children[i] = predicateSemanticShape(child)
	}
	return string(expression.Operator) + "(" + strings.Join(children, ",") + ")"
}

func TestFunctionPredicatesPreserveBooleanGrouping(t *testing.T) {
	for _, function := range []string{"isnull", "isnotnull", "isempty", "isnotempty"} {
		for _, tc := range []struct{ expression, shape string }{
			{"a == 1 or %s(b)", "or(a:==:false,b:%s:false)"},
			{"%s(b) or a == 1", "or(b:%s:false,a:==:false)"},
			{"a == 1 and (%s(b) or c == 2)", "and(a:==:false,or(b:%s:false,c:==:false))"},
			{"(%s(b) or a == 1) and c == 2", "and(or(b:%s:false,a:==:false),c:==:false)"},
			{"not(a == 1 or %s(b))", "and(a:==:true,b:%s:true)"},
			{"not(%s(b) and a == 1)", "or(b:%s:true,a:==:true)"},
			{"a == 1 or not(%s(b))", "or(a:==:false,b:%s:true)"},
		} {
			query := "T | where " + fmt.Sprintf(tc.expression, function)
			t.Run(query, func(t *testing.T) {
				result := ExtractConditions(query)
				if len(result.Errors) != 0 {
					t.Fatalf("parse errors: %v", result.Errors)
				}
				if result.Expression == nil {
					t.Fatal("missing condition expression")
				}
				if got, want := predicateSemanticShape(semanticFromExtraction(result).Expression), fmt.Sprintf(tc.shape, function); got != want {
					t.Fatalf("semantic shape = %s, want %s", got, want)
				}
			})
		}
	}
}

func TestFunctionPredicatesBooleanComparisons(t *testing.T) {
	for _, function := range []string{"isnull", "isnotnull", "isempty", "isnotempty"} {
		for _, tc := range []struct {
			expression string
			negated    bool
		}{
			{"%s(b)", false}, {"not(%s(b))", true}, {"not(not(%s(b)))", false},
			{"%s(b) == true", false}, {"%s(b) == false", true},
			{"%s(b) != true", true}, {"%s(b) != false", false},
			{"true == %s(b)", false}, {"false == %s(b)", true},
			{"true != %s(b)", true}, {"false != %s(b)", false},
			{"not(%s(b) == false)", false}, {"(%s(b)) == false", true},
			{"(not %s(b)) == false", false}, {"false == (not %s(b))", false},
			{"%s(b) == (false)", true},
		} {
			query := "T | where " + fmt.Sprintf(tc.expression, function)
			t.Run(query, func(t *testing.T) {
				result := ExtractConditions(query)
				if len(result.Errors) != 0 || len(result.Conditions) != 1 {
					t.Fatalf("conditions = %#v, errors = %v", result.Conditions, result.Errors)
				}
				condition := result.Conditions[0]
				if condition.Field != "b" || condition.Operator != function || condition.Negated != tc.negated {
					t.Fatalf("condition = %#v, want b %s negated=%t", condition, function, tc.negated)
				}
			})
		}
	}
}

func TestFunctionPredicatesOnlyFilterRows(t *testing.T) {
	for _, function := range []string{"isnull", "isnotnull", "isempty", "isnotempty"} {
		for _, query := range []string{
			"T | extend present=%s(b) | where a == 1",
			"T | project present=%s(b) | where a == 1",
			"T | summarize n=countif(%s(b)) | where a == 1",
			"T | where a == 1 | summarize n=countif(%s(b))",
			"let present=%s(b); T | where a == 1",
			"T | extend present=false == %s(b) | where a == 1",
			"T | where coalesce(%s(b), false) or a == 1",
			"T | where iff(%s(b), true, false) or a == 1",
			"T | where a == (%s(b))",
			"T | where a == (1 + (%s(b)))",
			"T | where a == bag[(%s(b))]",
		} {
			query = fmt.Sprintf(query, function)
			t.Run(query, func(t *testing.T) {
				result := ExtractConditions(query)
				if len(result.Errors) != 0 || len(result.Conditions) != 1 || result.Conditions[0].Field != "a" {
					t.Fatalf("conditions = %#v, errors = %v; want only a filter", result.Conditions, result.Errors)
				}
			})
		}
		t.Run(function+" balances depth", func(t *testing.T) {
			result := ExtractConditions(fmt.Sprintf("T | where %s(b) | summarize n=countif(a == 1) | where n > 0", function))
			if len(result.Errors) != 0 || len(result.Conditions) != 2 || result.Conditions[0].Field != "b" || result.Conditions[1].Field != "n" {
				t.Fatalf("conditions = %#v, errors = %v; countif must not filter input rows", result.Conditions, result.Errors)
			}
		})
	}
}

func TestFunctionPredicatesBelongToNestedPipelines(t *testing.T) {
	result := ExtractConditions(`T | where isnull(a) | union (U | where isempty(b) or c == 1) | where isnotnull(d)`)
	if len(result.Errors) != 0 || len(result.Conditions) != 4 {
		t.Fatalf("conditions = %#v, errors = %v", result.Conditions, result.Errors)
	}
	a, b, c, d := result.Conditions[0], result.Conditions[1], result.Conditions[2], result.Conditions[3]
	if a.Pipeline != d.Pipeline || a.Pipeline == b.Pipeline || b.Pipeline != c.Pipeline {
		t.Fatalf("predicate pipeline ownership = %#v", result.Conditions)
	}
	if result.Pipelines[b.Pipeline].Operator != "union" || result.Pipelines[b.Pipeline].Parent != a.Pipeline || b.PipeStage != c.PipeStage || d.PipeStage <= b.PipeStage {
		t.Fatalf("predicate pipeline metadata = %#v, pipelines = %#v", result.Conditions, result.Pipelines)
	}
}

func TestFunctionPredicatesCarryFilterMetadata(t *testing.T) {
	result := ExtractConditions(`T | extend Copied=b | where a == 1 or isnotempty(Copied) | where isnull(c)`)
	if len(result.Errors) != 0 || len(result.Conditions) != 3 {
		t.Fatalf("conditions = %#v, errors = %v", result.Conditions, result.Errors)
	}
	first, predicate, last := result.Conditions[0], result.Conditions[1], result.Conditions[2]
	if predicate.LogicalOp != "OR" || predicate.PipeStage != first.PipeStage || predicate.Pipeline != first.Pipeline || !predicate.IsComputed || predicate.SourceField != "b" {
		t.Fatalf("predicate metadata = %#v, preceding comparison = %#v", predicate, first)
	}
	if last.LogicalOp != "AND" || last.PipeStage <= predicate.PipeStage || last.Pipeline != predicate.Pipeline {
		t.Fatalf("following predicate metadata = %#v, previous = %#v", last, predicate)
	}
	if got := predicateSemanticShape(semanticFromExtraction(result).Expression); got != "and(or(a:==:false,Copied:isnotempty:false),c:isnull:false)" {
		t.Fatalf("pipeline semantic shape = %s", got)
	}
}

func TestFunctionPredicateArgumentDoesNotChangeFilterLogicalOp(t *testing.T) {
	result := ExtractConditions(`T | where isnull(b == 1 or c == 2) and a == 1`)
	if len(result.Errors) != 0 || len(result.Conditions) != 2 {
		t.Fatalf("conditions = %#v, errors = %v", result.Conditions, result.Errors)
	}
	if result.Conditions[1].Field != "a" || result.Conditions[1].LogicalOp != "AND" {
		t.Fatalf("following comparison = %#v; argument OR must not link row filters", result.Conditions[1])
	}
}
