package kql

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
)

// A function predicate is one boolean filter leaf, including any comparison
// with a boolean literal. Keep the original operator: empty and null checks
// differ for empty strings.
type kqlFunctionPredicate struct {
	field, operator string
	negated         bool
}

// kqlFilterFunctionPredicate is shared by extraction and expression building,
// so both consume exactly one condition for the same filter expression.
func kqlFilterFunctionPredicate(ctx IComparisonExpressionContext) (kqlFunctionPredicate, bool) {
	if !isKQLFilterContext(ctx) || hasKQLFunctionPredicateAncestor(ctx) {
		return kqlFunctionPredicate{}, false
	}
	return recognizeKQLFunctionPredicate(ctx)
}

func isKQLFilterContext(tree antlr.Tree) bool {
	for parent := tree.GetParent(); parent != nil; parent = parent.GetParent() {
		switch parent.(type) {
		case IWhereOperatorContext:
			return true
		case IFunctionCallContext, IArgumentContext, IIffExpressionContext, ICaseExpressionContext, IToScalarExpressionContext, ITabularExpressionContext:
			return false
		}
	}
	return false
}

// Parentheses and boolean comparisons can enclose another comparison node.
// Only their outermost recognized predicate emits a condition.
func hasKQLFunctionPredicateAncestor(tree antlr.Tree) bool {
	for parent := tree.GetParent(); parent != nil; parent = parent.GetParent() {
		if _, ok := parent.(IWhereOperatorContext); ok {
			break
		}
		if comparison, ok := parent.(IComparisonExpressionContext); ok {
			if _, recognized := recognizeKQLFunctionPredicate(comparison); recognized {
				return true
			}
		}
	}
	return false
}

func recognizeKQLFunctionPredicate(tree antlr.Tree) (kqlFunctionPredicate, bool) {
	switch ctx := tree.(type) {
	case IFunctionCallContext:
		if ctx.Identifier() == nil || ctx.ArgumentList() == nil {
			return kqlFunctionPredicate{}, false
		}
		operator := strings.ToLower(ctx.Identifier().GetText())
		switch operator {
		case "isnull", "isnotnull", "isempty", "isnotempty":
			arguments := ctx.ArgumentList().AllArgument()
			if len(arguments) == 1 && arguments[0].Expression() != nil {
				return kqlFunctionPredicate{field: arguments[0].GetText(), operator: operator}, true
			}
		}
		return kqlFunctionPredicate{}, false
	case INotExpressionContext:
		if ctx.NOT() != nil {
			predicate, ok := recognizeKQLFunctionPredicate(ctx.NotExpression())
			predicate.negated = !predicate.negated
			return predicate, ok
		}
	case IComparisonExpressionContext:
		if operator := ctx.ComparisonOperator(); operator != nil {
			op := operator.GetText()
			if op != "==" && op != "!=" && op != "<>" {
				return kqlFunctionPredicate{}, false
			}
			values := ctx.AllAdditiveExpression()
			if len(values) != 2 {
				return kqlFunctionPredicate{}, false
			}
			for side := 0; side < 2; side++ {
				predicate, ok := recognizeKQLFunctionPredicate(values[side])
				boolean, isBoolean := kqlBooleanLiteral(values[1-side])
				if ok && isBoolean {
					invert := !boolean
					if op != "==" {
						invert = !invert
					}
					predicate.negated = predicate.negated != invert
					return predicate, true
				}
			}
			return kqlFunctionPredicate{}, false
		}
	}
	child := kqlTransparentScalarChild(tree)
	if child == nil {
		return kqlFunctionPredicate{}, false
	}
	return recognizeKQLFunctionPredicate(child)
}

func kqlBooleanLiteral(tree antlr.Tree) (bool, bool) {
	if text, ok := tree.(interface{ GetText() string }); ok {
		switch strings.ToLower(text.GetText()) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	if child := kqlTransparentScalarChild(tree); child != nil {
		return kqlBooleanLiteral(child)
	}
	return false, false
}

// Only descend through scalar grammar wrappers and parentheses. In particular,
// do not turn predicates inside arbitrary functions or arithmetic into filters.
func kqlTransparentScalarChild(tree antlr.Tree) antlr.Tree {
	switch tree.(type) {
	case IExpressionContext, IOrExpressionContext, IAndExpressionContext,
		INotExpressionContext, IComparisonExpressionContext, IAdditiveExpressionContext,
		IMultiplicativeExpressionContext, IUnaryExpressionContext,
		IPostfixExpressionContext, IPrimaryExpressionContext:
	default:
		return nil
	}
	var child antlr.Tree
	for i := 0; i < tree.GetChildCount(); i++ {
		node := tree.GetChild(i)
		if terminal, ok := node.(antlr.TerminalNode); ok {
			if terminal.GetText() != "(" && terminal.GetText() != ")" {
				return nil
			}
			continue
		}
		if child != nil {
			return nil
		}
		child = node
	}
	return child
}
