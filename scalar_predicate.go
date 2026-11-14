package kql

import (
	"regexp"
	"strings"

	"github.com/antlr4-go/antlr/v4"
)

// ScalarPredicate is a complete, unevaluated row-filter predicate. Text is
// the exact normalized input slice; Negated applies an enclosing boolean NOT.
// Consumers must preserve it or explicitly reject unsupported scalar filters.
type ScalarPredicate struct {
	Text      string `json:"text"`
	Negated   bool   `json:"negated,omitempty"`
	PipeStage int    `json:"pipe_stage"`
	Pipeline  int    `json:"pipeline"`
}

type kqlOperand struct {
	text               string
	reference, literal bool
}

// Classify syntax, never a float approximation or a field-name guess. In
// particular INT_NUMBER is a literal even though identifier accepts it too.
func recognizeKQLOperand(tree antlr.Tree, budgets ...*parseBudget) kqlOperand {
	budget := suppliedBudget(budgets)
	budget.step()

	switch ctx := tree.(type) {
	case ILiteralContext:
		return kqlOperand{text: ctx.GetText(), literal: true}
	case IIdentifierContext:
		text := ctx.GetText()
		if numericToken.MatchString(text) || text == "true" || text == "false" {
			return kqlOperand{text: text, literal: true}
		}
		return kqlOperand{text: text, reference: true}
	case IPrimaryExpressionContext:
		if token := ctx.QUOTED_IDENTIFIER(); token != nil {
			return kqlOperand{text: decodeKQLIdentifier(token.GetText(), budget), reference: true}
		}
	case IUnaryExpressionContext:
		if ctx.MINUS() != nil || ctx.PLUS() != nil {
			child := recognizeKQLOperand(ctx.UnaryExpression(), budget)
			if child.literal {
				child.text = kqlSourceText(ctx, budget)
				return child
			}
			return kqlOperand{}
		}
	case IPostfixExpressionContext:
		if len(ctx.AllPostfixOperator()) > 0 {
			if base := recognizeKQLOperand(ctx.PrimaryExpression(), budget); base.reference && isValidFieldName(ctx.GetText()) {
				return kqlOperand{text: ctx.GetText(), reference: true}
			}
			return kqlOperand{}
		}
	case IFunctionCallContext:
		text := kqlSourceText(ctx, budget)
		name := ""
		if ctx.Identifier() != nil {
			name = strings.ToLower(ctx.Identifier().GetText())
		}
		switch name {
		case "int", "long", "real", "double", "decimal", "bool", "boolean", "string", "datetime", "date", "timespan", "time", "guid", "dynamic":
			if args := ctx.ArgumentList(); args != nil && len(args.AllArgument()) == 1 {
				argument := args.AllArgument()[0]
				if recognizeKQLOperand(argument.Expression(), budget).literal {
					return kqlOperand{text: text, literal: true}
				}
			}
		}
		return kqlOperand{}
	}
	if child := kqlTransparentScalarChild(tree, budget); child != nil {
		return recognizeKQLOperand(child, budget)
	}
	return kqlOperand{}
}

var numericToken = regexp.MustCompile(`(?i)^(?:0x[0-9a-f]+|(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)(?:e[+-]?[0-9]+)?[lm]?)$`)
var numericPrefix = regexp.MustCompile(`(?i)^(?:0x[0-9a-f]+|(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)(?:e[+-]?[0-9]+)?[lm]?)`)

func decodeKQLIdentifier(text string, budgets ...*parseBudget) string {
	budget := suppliedBudget(budgets)
	budget.step()

	if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		text = text[1 : len(text)-1]
		if value, _, ok := parseStringLiteral(text); ok {
			return value
		}
	}
	return text
}

// Shared by extraction and expression building: only a reference compared
// with a reference or literal becomes a Condition. Everything else stays scalar.
func kqlComparisonOperands(ctx IComparisonExpressionContext, budgets ...*parseBudget) (kqlOperand, string, kqlOperand, bool) {
	budget := suppliedBudget(budgets)
	budget.step()

	values := ctx.AllAdditiveExpression()
	if len(values) != 2 {
		return kqlOperand{}, "", kqlOperand{}, false
	}
	op := ""
	if ctx.ComparisonOperator() != nil {
		op = ctx.ComparisonOperator().GetText()
	} else if ctx.StringOperator() != nil {
		op = ctx.StringOperator().GetText()
	} else {
		return kqlOperand{}, "", kqlOperand{}, false
	}
	left, right := recognizeKQLOperand(values[0], budget), recognizeKQLOperand(values[1], budget)
	if !left.reference && left.literal && right.reference && ctx.ComparisonOperator() != nil {
		left, right = right, left
		switch op {
		case "<":
			op = ">"
		case "<=":
			op = ">="
		case ">":
			op = "<"
		case ">=":
			op = "<="
		}
	}
	return left, op, right, left.reference && (right.reference || right.literal)
}

func kqlSourceText(ctx antlr.ParserRuleContext, budgets ...*parseBudget) string {
	budget := suppliedBudget(budgets)
	budget.step()

	start, stop := ctx.GetStart(), ctx.GetStop()
	if start == nil || stop == nil || start.GetInputStream() == nil {
		return ctx.GetText()
	}
	return start.GetInputStream().GetText(start.GetStart(), stop.GetStop())
}

func cloneScalarPredicate(scalar *ScalarPredicate, budgets ...*parseBudget) *ScalarPredicate {
	budget := suppliedBudget(budgets)
	budget.step()

	if scalar == nil {
		return nil
	}
	budget.reserveOutput(1)
	copy := *scalar
	return &copy
}

func kqlContainsLetReference(tree antlr.Tree, names map[string]bool, budgets ...*parseBudget) bool {
	budget := suppliedBudget(budgets)
	budget.step()

	if primary, ok := tree.(IPrimaryExpressionContext); ok && primary.QUOTED_IDENTIFIER() != nil && names[decodeKQLIdentifier(primary.QUOTED_IDENTIFIER().GetText(), budget)] {
		return true
	}
	if identifier, ok := tree.(IIdentifierContext); ok && names[identifier.GetText()] {
		return true
	}
	for i := 0; i < tree.GetChildCount(); i++ {
		budget.step()
		if kqlContainsLetReference(tree.GetChild(i), names, budget) {
			return true
		}
	}
	return false
}

// A parenthesized boolean group is transparent; function arguments and
// arithmetic containing expressions are not boolean groups.
func groupedKQLExpression(tree antlr.Tree, budgets ...*parseBudget) IExpressionContext {
	budget := suppliedBudget(budgets)
	budget.step()

	for tree != nil {
		budget.step()
		tree = kqlTransparentScalarChild(tree, budget)
		if expression, ok := tree.(IExpressionContext); ok {
			return expression
		}
	}
	return nil
}

func containsScalarExpression(expression *ConditionExpression, budgets ...*parseBudget) bool {
	budget := suppliedBudget(budgets)
	budget.step()

	if expression == nil {
		return false
	}
	if expression.Operator == BooleanScalar || expression.Operator == BooleanSearch {
		return true
	}
	for _, child := range expression.Children {
		budget.step()
		if containsScalarExpression(child, budget) {
			return true
		}
	}
	return false
}
