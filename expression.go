package kql

import "github.com/antlr4-go/antlr/v4"

// BooleanOperator represents parsed boolean expression operators.
type BooleanOperator string

const (
	BooleanCondition BooleanOperator = "condition"
	BooleanAnd       BooleanOperator = "and"
	BooleanOr        BooleanOperator = "or"
)

// ConditionExpression preserves boolean grouping over extracted Conditions.
// ConditionIndex references ParseResult.Conditions.
type ConditionExpression struct {
	Operator       BooleanOperator        `json:"operator"`
	ConditionIndex int                    `json:"condition_index"`
	Children       []*ConditionExpression `json:"children,omitempty"`
}

func buildConditionExpression(tree antlr.Tree, conditions []Condition) *ConditionExpression {
	if tree == nil || len(conditions) < 2 {
		return nil
	}
	builder := &kqlExpressionBuilder{conditions: conditions}
	var expression *ConditionExpression
	for _, where := range collectKQLWhereOperators(tree) {
		expression = combineConditionExpressions(BooleanAnd, expression, builder.expression(where.Expression()))
	}
	if expression != nil && builder.next == len(conditions) {
		return expression
	}
	return nil
}

type kqlExpressionBuilder struct {
	conditions []Condition
	next       int
}

func collectKQLWhereOperators(tree antlr.Tree) []IWhereOperatorContext {
	var wheres []IWhereOperatorContext
	var visit func(antlr.Tree)
	visit = func(node antlr.Tree) {
		if node == nil {
			return
		}
		if where, ok := node.(IWhereOperatorContext); ok {
			wheres = append(wheres, where)
		}
		for i := 0; i < node.GetChildCount(); i++ {
			visit(node.GetChild(i))
		}
	}
	visit(tree)
	return wheres
}

func (b *kqlExpressionBuilder) expression(ctx IExpressionContext) *ConditionExpression {
	if ctx == nil || ctx.OrExpression() == nil {
		return nil
	}
	return b.orExpression(ctx.OrExpression())
}

func (b *kqlExpressionBuilder) orExpression(ctx IOrExpressionContext) *ConditionExpression {
	var expression *ConditionExpression
	for _, and := range ctx.AllAndExpression() {
		expression = combineConditionExpressions(BooleanOr, expression, b.andExpression(and))
	}
	return expression
}

func (b *kqlExpressionBuilder) andExpression(ctx IAndExpressionContext) *ConditionExpression {
	var expression *ConditionExpression
	for _, not := range ctx.AllNotExpression() {
		expression = combineConditionExpressions(BooleanAnd, expression, b.notExpression(not))
	}
	return expression
}

func (b *kqlExpressionBuilder) notExpression(ctx INotExpressionContext) *ConditionExpression {
	if ctx == nil {
		return nil
	}
	if ctx.NOT() != nil {
		return demorganConditionExpression(b.notExpression(ctx.NotExpression()))
	}
	return b.comparisonExpression(ctx.ComparisonExpression())
}

func (b *kqlExpressionBuilder) comparisonExpression(ctx IComparisonExpressionContext) *ConditionExpression {
	if ctx == nil {
		return nil
	}
	if ctx.HAS_ANY() != nil || ctx.HAS_ALL() != nil {
		operator := BooleanOr
		if ctx.HAS_ALL() != nil {
			operator = BooleanAnd
		}
		return b.multiValueCondition(ctx.ExpressionList(), operator)
	}
	if ctx.BETWEEN() != nil || ctx.NOT_BETWEEN() != nil {
		operator := BooleanAnd
		if ctx.NOT_BETWEEN() != nil {
			operator = BooleanOr
		}
		return b.conditionGroup(2, operator)
	}
	if isKQLPredicateComparison(ctx) {
		return b.condition()
	}
	if nested := firstNestedKQLExpression(ctx); nested != nil {
		return b.expression(nested)
	}
	return nil
}

func isKQLPredicateComparison(ctx IComparisonExpressionContext) bool {
	return ctx.ComparisonOperator() != nil ||
		ctx.BETWEEN() != nil ||
		ctx.NOT_BETWEEN() != nil ||
		ctx.IN() != nil ||
		ctx.NOT_IN() != nil ||
		ctx.IN_CS() != nil ||
		ctx.NOT_IN_CS() != nil ||
		ctx.HAS_ANY() != nil ||
		ctx.HAS_ALL() != nil ||
		ctx.StringOperator() != nil
}

func firstNestedKQLExpression(tree antlr.Tree) IExpressionContext {
	if tree == nil {
		return nil
	}
	for i := 0; i < tree.GetChildCount(); i++ {
		child := tree.GetChild(i)
		if expression, ok := child.(IExpressionContext); ok {
			return expression
		}
		if expression := firstNestedKQLExpression(child); expression != nil {
			return expression
		}
	}
	return nil
}

func (b *kqlExpressionBuilder) condition() *ConditionExpression {
	if b.next >= len(b.conditions) {
		return nil
	}
	index := b.next
	b.next++
	return &ConditionExpression{Operator: BooleanCondition, ConditionIndex: index}
}

func (b *kqlExpressionBuilder) multiValueCondition(exprList IExpressionListContext, operator BooleanOperator) *ConditionExpression {
	count := 1
	if exprList != nil {
		if values := extractExpressionListValues(exprList); len(values) > 0 {
			count = len(values)
		}
	}
	return b.conditionGroup(count, operator)
}

func (b *kqlExpressionBuilder) conditionGroup(count int, operator BooleanOperator) *ConditionExpression {
	if count <= 1 {
		return b.condition()
	}
	if b.next+count > len(b.conditions) {
		return nil
	}
	var expression *ConditionExpression
	for i := 0; i < count; i++ {
		expression = combineConditionExpressions(operator, expression, b.condition())
	}
	return expression
}

func remapConditionExpression(expression *ConditionExpression, indexMap []int) *ConditionExpression {
	if expression == nil || len(indexMap) == 0 {
		return expression
	}
	switch expression.Operator {
	case BooleanCondition:
		if expression.ConditionIndex < 0 || expression.ConditionIndex >= len(indexMap) {
			return nil
		}
		mapped := indexMap[expression.ConditionIndex]
		if mapped < 0 {
			return nil
		}
		return &ConditionExpression{Operator: BooleanCondition, ConditionIndex: mapped}
	case BooleanAnd, BooleanOr:
		converted := &ConditionExpression{Operator: expression.Operator}
		for _, child := range expression.Children {
			remapped := remapConditionExpression(child, indexMap)
			if remapped == nil {
				continue
			}
			if remapped.Operator == expression.Operator {
				for _, grandchild := range remapped.Children {
					converted.Children = appendUniqueConditionExpression(converted.Children, grandchild)
				}
				continue
			}
			converted.Children = appendUniqueConditionExpression(converted.Children, remapped)
		}
		if len(converted.Children) == 0 {
			return nil
		}
		if len(converted.Children) == 1 {
			return converted.Children[0]
		}
		return converted
	default:
		return nil
	}
}

func appendConditionRangeExpression(expression *ConditionExpression, start, end int) *ConditionExpression {
	for index := start; index < end; index++ {
		leaf := &ConditionExpression{Operator: BooleanCondition, ConditionIndex: index}
		expression = combineConditionExpressions(BooleanAnd, expression, leaf)
	}
	return expression
}

func appendUniqueConditionExpression(children []*ConditionExpression, child *ConditionExpression) []*ConditionExpression {
	for _, existing := range children {
		if equalConditionExpression(existing, child) {
			return children
		}
	}
	return append(children, child)
}

func equalConditionExpression(a, b *ConditionExpression) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Operator != b.Operator || a.ConditionIndex != b.ConditionIndex || len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		if !equalConditionExpression(a.Children[i], b.Children[i]) {
			return false
		}
	}
	return true
}

func combineConditionExpressions(operator BooleanOperator, left, right *ConditionExpression) *ConditionExpression {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	combined := &ConditionExpression{Operator: operator}
	if left.Operator == operator {
		combined.Children = append(combined.Children, left.Children...)
	} else {
		combined.Children = append(combined.Children, left)
	}
	if right.Operator == operator {
		combined.Children = append(combined.Children, right.Children...)
	} else {
		combined.Children = append(combined.Children, right)
	}
	return combined
}

func demorganConditionExpression(expression *ConditionExpression) *ConditionExpression {
	if expression == nil {
		return nil
	}
	switch expression.Operator {
	case BooleanAnd:
		converted := &ConditionExpression{Operator: BooleanOr}
		for _, child := range expression.Children {
			converted.Children = append(converted.Children, demorganConditionExpression(child))
		}
		return converted
	case BooleanOr:
		converted := &ConditionExpression{Operator: BooleanAnd}
		for _, child := range expression.Children {
			converted.Children = append(converted.Children, demorganConditionExpression(child))
		}
		return converted
	default:
		return expression
	}
}
