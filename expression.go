package kql

import "github.com/antlr4-go/antlr/v4"

// BooleanOperator represents parsed boolean expression operators.
type BooleanOperator string

const (
	BooleanCondition BooleanOperator = "condition"
	BooleanScalar    BooleanOperator = "scalar"
	BooleanSearch    BooleanOperator = "search"
	BooleanAnd       BooleanOperator = "and"
	BooleanOr        BooleanOperator = "or"
)

// ConditionExpression preserves boolean grouping over extracted Conditions.
// ConditionIndex references ParseResult.Conditions.
type ConditionExpression struct {
	Operator       BooleanOperator        `json:"operator"`
	ConditionIndex int                    `json:"condition_index"`
	Scalar         *ScalarPredicate       `json:"scalar,omitempty"`
	Search         *SearchPredicate       `json:"search,omitempty"`
	Children       []*ConditionExpression `json:"children,omitempty"`
}

func buildConditionExpression(tree antlr.Tree, conditions []Condition, budget *parseBudget, extraction ...*conditionExtractor) *ConditionExpression {
	if tree == nil {
		return nil
	}
	builder := &kqlExpressionBuilder{conditions: conditions, budget: budget}
	if len(extraction) > 0 {
		builder.leaves = extraction[0].expressionLeaves
	}
	var expression *ConditionExpression
	for _, filter := range collectKQLFilterOperators(tree, budget) {
		budget.step()
		var leaf *ConditionExpression
		switch ctx := filter.(type) {
		case IWhereOperatorContext:
			leaf = builder.expression(ctx.Expression())
		case ISearchOperatorContext:
			if len(extraction) > 0 {
				leaf = extraction[0].searchLeaves[ctx]
			}
		}
		expression = combineConditionExpressions(BooleanAnd, expression, leaf, budget)
	}
	if expression != nil && builder.next == len(conditions) {
		return expression
	}
	return nil
}

func collectKQLFilterOperators(tree antlr.Tree, budget *parseBudget) []antlr.Tree {
	budget.step()
	var filters []antlr.Tree
	var visit func(antlr.Tree)
	visit = func(node antlr.Tree) {
		budget.step()
		switch node.(type) {
		case IJoinOperatorContext, ILookupOperatorContext:
			return
		case ISearchOperatorContext:
			budget.reserveOutput(1)
			filters = append(filters, node)
			return
		case IWhereOperatorContext:
			filters = append(filters, node)
			return
		}
		for i := 0; i < node.GetChildCount(); i++ {
			visit(node.GetChild(i))
		}
	}
	visit(tree)
	return filters
}

type kqlExpressionBuilder struct {
	budget     *parseBudget
	conditions []Condition
	next       int
	leaves     map[IComparisonExpressionContext]*ConditionExpression
}

func collectKQLWhereOperators(tree antlr.Tree, budget *parseBudget) []IWhereOperatorContext {
	var wheres []IWhereOperatorContext
	var visit func(antlr.Tree)
	visit = func(node antlr.Tree) {
		budget.step()
		if node == nil {
			return
		}
		if where, ok := node.(IWhereOperatorContext); ok {
			wheres = append(wheres, where)
		}
		for i := 0; i < node.GetChildCount(); i++ {
			budget.step()
			visit(node.GetChild(i))
		}
	}
	visit(tree)
	return wheres
}

func (b *kqlExpressionBuilder) expression(ctx IExpressionContext) *ConditionExpression {
	b.budget.step()
	if ctx == nil || ctx.OrExpression() == nil {
		return nil
	}
	return b.orExpression(ctx.OrExpression())
}

func (b *kqlExpressionBuilder) orExpression(ctx IOrExpressionContext) *ConditionExpression {
	b.budget.step()
	var expression *ConditionExpression
	for _, and := range ctx.AllAndExpression() {
		b.budget.step()
		expression = combineConditionExpressions(BooleanOr, expression, b.andExpression(and), b.budget)
	}
	return expression
}

func (b *kqlExpressionBuilder) andExpression(ctx IAndExpressionContext) *ConditionExpression {
	b.budget.step()
	var expression *ConditionExpression
	for _, not := range ctx.AllNotExpression() {
		b.budget.step()
		expression = combineConditionExpressions(BooleanAnd, expression, b.notExpression(not), b.budget)
	}
	return expression
}

func (b *kqlExpressionBuilder) notExpression(ctx INotExpressionContext) *ConditionExpression {
	b.budget.step()
	if ctx == nil {
		return nil
	}
	if ctx.NOT() != nil {
		return demorganConditionExpression(b.notExpression(ctx.NotExpression()), b.budget)
	}
	return b.comparisonExpression(ctx.ComparisonExpression())
}

func (b *kqlExpressionBuilder) comparisonExpression(ctx IComparisonExpressionContext) *ConditionExpression {
	b.budget.step()
	if ctx == nil {
		return nil
	}
	if b.leaves != nil {
		if leaf, ok := b.leaves[ctx]; ok {
			b.next += countConditionLeaves(leaf, b.budget)
			return leaf
		}
		if nested := groupedKQLExpression(ctx, b.budget); nested != nil {
			return b.expression(nested)
		}
		return nil
	}
	if _, ok := kqlFilterFunctionPredicate(ctx, b.budget); ok {
		return b.condition()
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
	if isKQLPredicateComparison(ctx, b.budget) {
		if ctx.ComparisonOperator() != nil {
			if _, _, _, ok := kqlComparisonOperands(ctx, b.budget); !ok {
				b.budget.reserveOutput(1)
				return &ConditionExpression{Operator: BooleanScalar, Scalar: &ScalarPredicate{Text: kqlSourceText(ctx, b.budget)}}
			}
		}
		return b.condition()
	}
	if nested := firstNestedKQLExpression(ctx, b.budget); nested != nil {
		return b.expression(nested)
	}
	return nil
}

func isKQLPredicateComparison(ctx IComparisonExpressionContext, budgets ...*parseBudget) bool {
	budget := suppliedBudget(budgets)
	budget.step()

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

func firstNestedKQLExpression(tree antlr.Tree, budgets ...*parseBudget) IExpressionContext {
	budget := suppliedBudget(budgets)
	budget.step()

	if tree == nil {
		return nil
	}
	for i := 0; i < tree.GetChildCount(); i++ {
		budget.step()
		child := tree.GetChild(i)
		if expression, ok := child.(IExpressionContext); ok {
			return expression
		}
		if expression := firstNestedKQLExpression(child, budget); expression != nil {
			return expression
		}
	}
	return nil
}

func (b *kqlExpressionBuilder) condition() *ConditionExpression {
	b.budget.step()
	if b.next >= len(b.conditions) {
		return nil
	}
	index := b.next
	b.next++
	return &ConditionExpression{Operator: BooleanCondition, ConditionIndex: index}
}

func (b *kqlExpressionBuilder) multiValueCondition(exprList IExpressionListContext, operator BooleanOperator) *ConditionExpression {
	b.budget.step()
	count := 1
	if exprList != nil {
		if values := extractExpressionListValues(exprList, b.budget); len(values) > 0 {
			count = len(values)
		}
	}
	return b.conditionGroup(count, operator)
}

func (b *kqlExpressionBuilder) conditionGroup(count int, operator BooleanOperator) *ConditionExpression {
	b.budget.step()
	if count <= 1 {
		return b.condition()
	}
	if b.next+count > len(b.conditions) {
		return nil
	}
	var expression *ConditionExpression
	for i := 0; i < count; i++ {
		b.budget.step()
		expression = combineConditionExpressions(operator, expression, b.condition(), b.budget)
	}
	return expression
}

func remapConditionExpression(expression *ConditionExpression, indexMap []int, budgets ...*parseBudget) *ConditionExpression {
	budget := suppliedBudget(budgets)
	budget.step()

	if expression == nil || len(indexMap) == 0 {
		return expression
	}
	switch expression.Operator {
	case BooleanSearch:
		return &ConditionExpression{Operator: BooleanSearch, Search: cloneSearchPredicate(expression.Search, budget)}
	case BooleanScalar:
		return &ConditionExpression{Operator: BooleanScalar, Scalar: cloneScalarPredicate(expression.Scalar, budget)}
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
			budget.step()
			remapped := remapConditionExpression(child, indexMap, budget)
			if remapped == nil {
				continue
			}
			if remapped.Operator == expression.Operator {
				for _, grandchild := range remapped.Children {
					budget.step()
					converted.Children = appendUniqueConditionExpression(converted.Children, grandchild, budget)
				}
				continue
			}
			converted.Children = appendUniqueConditionExpression(converted.Children, remapped, budget)
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

func appendConditionRangeExpression(expression *ConditionExpression, start, end int, budgets ...*parseBudget) *ConditionExpression {
	budget := suppliedBudget(budgets)
	budget.step()

	for index := start; index < end; index++ {
		budget.step()
		leaf := &ConditionExpression{Operator: BooleanCondition, ConditionIndex: index}
		expression = combineConditionExpressions(BooleanAnd, expression, leaf, budget)
	}
	return expression
}

func appendUniqueConditionExpression(children []*ConditionExpression, child *ConditionExpression, budgets ...*parseBudget) []*ConditionExpression {
	budget := suppliedBudget(budgets)
	budget.step()

	// Unevaluated scalar functions can be nondeterministic. Preserve each occurrence.
	if containsScalarExpression(child, budget) {
		return append(children, child)
	}
	for _, existing := range children {
		budget.step()
		if equalConditionExpression(existing, child, budget) {
			return children
		}
	}
	return append(children, child)
}

func equalConditionExpression(a, b *ConditionExpression, budgets ...*parseBudget) bool {
	budget := suppliedBudget(budgets)
	budget.step()

	if a == nil || b == nil {
		return a == b
	}
	if (a.Scalar == nil) != (b.Scalar == nil) || (a.Scalar != nil && *a.Scalar != *b.Scalar) {
		return false
	}
	if !equalSearchPredicate(a.Search, b.Search, budget) {
		return false
	}
	if a.Operator != b.Operator || a.ConditionIndex != b.ConditionIndex || len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		budget.step()
		if !equalConditionExpression(a.Children[i], b.Children[i], budget) {
			return false
		}
	}
	return true
}

func combineConditionExpressions(operator BooleanOperator, left, right *ConditionExpression, budgets ...*parseBudget) *ConditionExpression {
	budget := suppliedBudget(budgets)
	budget.step()

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

func demorganConditionExpression(expression *ConditionExpression, budgets ...*parseBudget) *ConditionExpression {
	budget := suppliedBudget(budgets)
	budget.step()

	if expression == nil {
		return nil
	}
	switch expression.Operator {
	case BooleanScalar:
		scalar := cloneScalarPredicate(expression.Scalar, budget)
		if scalar != nil {
			scalar.Negated = !scalar.Negated
		}
		return &ConditionExpression{Operator: BooleanScalar, Scalar: scalar}
	case BooleanAnd:
		converted := &ConditionExpression{Operator: BooleanOr}
		for _, child := range expression.Children {
			budget.step()
			converted.Children = append(converted.Children, demorganConditionExpression(child, budget))
		}
		return converted
	case BooleanOr:
		converted := &ConditionExpression{Operator: BooleanAnd}
		for _, child := range expression.Children {
			budget.step()
			converted.Children = append(converted.Children, demorganConditionExpression(child, budget))
		}
		return converted
	default:
		return expression
	}
}

func countConditionLeaves(expression *ConditionExpression, budgets ...*parseBudget) int {
	budget := suppliedBudget(budgets)
	budget.step()

	if expression == nil {
		return 0
	}
	if expression.Operator == BooleanCondition {
		return 1
	}
	count := 0
	for _, child := range expression.Children {
		budget.step()
		count += countConditionLeaves(child, budget)
	}
	return count
}
