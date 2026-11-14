package kql

import "context"

// SemanticQuery is the parser-owned semantic view of a KQL query.
// It resolves ANTLR/listener output into stable condition-bearing nodes so
// callers do not need to understand parse-tree details or condition indexes.
type SemanticQuery struct {
	Conditions          []Condition         `json:"conditions,omitempty"`
	Expression          *SemanticExpression `json:"expression,omitempty"`
	DataSources         []string            `json:"data_sources,omitempty"`
	LetStatements       []LetStatement      `json:"let_statements,omitempty"`
	ComputedFields      map[string]string   `json:"computed_fields,omitempty"`
	ComputedExpressions map[string]string   `json:"computed_expressions,omitempty"`
	GroupByFields       []string            `json:"group_by_fields,omitempty"`
	Commands            []string            `json:"commands,omitempty"`
	ProjectedFields     []string            `json:"projected_fields,omitempty"`
	Joins               []SemanticJoin      `json:"joins,omitempty"`
	Errors              []string            `json:"errors,omitempty"`
}

// SemanticExpression preserves boolean grouping while pointing directly at
// semantic conditions instead of parser-internal condition indexes.
type SemanticExpression struct {
	Operator  BooleanOperator       `json:"operator"`
	Scalar    *ScalarPredicate      `json:"scalar,omitempty"`
	Condition *Condition            `json:"condition,omitempty"`
	Children  []*SemanticExpression `json:"children,omitempty"`
}

// SemanticJoin is the parser-owned semantic view of a KQL join.
type SemanticJoin struct {
	Type          string         `json:"type"`
	JoinFields    []string       `json:"join_fields,omitempty"`
	LeftFields    []string       `json:"left_fields,omitempty"`
	RightFields   []string       `json:"right_fields,omitempty"`
	RightTable    string         `json:"right_table,omitempty"`
	Subquery      *SemanticQuery `json:"subquery,omitempty"`
	PipeStage     int            `json:"pipe_stage"`
	ExposedFields []string       `json:"exposed_fields,omitempty"`
}

// AnalyzeQuery parses a KQL query and returns the parser-owned semantic model.
func AnalyzeQuery(query string) *SemanticQuery {
	return AnalyzeQueryContext(context.Background(), query)
}

// AnalyzeQueryContext shares caller cancellation and parser limits through
// extraction and semantic conversion, including recursive subqueries.
func AnalyzeQueryContext(ctx context.Context, query string) *SemanticQuery {
	return analyzeQueryContext(ctx, query, defaultParseLimits())
}

func analyzeQueryContext(ctx context.Context, query string, limits parseLimits) (result *SemanticQuery) {
	defer func() {
		if value := recover(); value != nil {
			result = &SemanticQuery{Errors: []string{parseFailure(value).Error()}}
		}
	}()
	budget, cancel := newParseBudget(ctx, limits)
	defer cancel()
	result = semanticFromExtraction(extractConditionsInternal(query, budget), budget)
	budget.check()
	return result
}

// semanticFromExtraction normalizes parser extraction output into the
// parser-owned semantic model.
func semanticFromExtraction(result *ParseResult, budget *parseBudget) *SemanticQuery {
	budget.step()
	if result == nil {
		return nil
	}

	budget.reserveOutput(1 + len(result.LetStatements))
	semantic := &SemanticQuery{
		Conditions:          cloneSemanticConditions(result.Conditions, budget),
		DataSources:         cloneSemanticStrings(result.DataSources, budget),
		LetStatements:       append([]LetStatement(nil), result.LetStatements...),
		ComputedFields:      cloneSemanticStringMap(result.ComputedFields, budget),
		ComputedExpressions: cloneSemanticStringMap(result.ComputedExpressions, budget),
		GroupByFields:       cloneSemanticStrings(result.GroupByFields, budget),
		Commands:            cloneSemanticStrings(result.Commands, budget),
		ProjectedFields:     cloneSemanticStrings(result.ProjectedFields, budget),
		Errors:              cloneSemanticStrings(result.Errors, budget),
	}
	semantic.Expression = semanticExpressionFromParseExpression(result.Expression, semantic.Conditions, budget)
	if semantic.Expression == nil {
		semantic.Expression = semanticExpressionFromConditions(semantic.Conditions, budget)
	}
	semantic.Joins = semanticJoinsFromParseJoins(result.Joins, budget)
	return semantic
}

func semanticExpressionFromParseExpression(expression *ConditionExpression, conditions []Condition, budget *parseBudget) *SemanticExpression {
	budget.step()
	if expression == nil {
		return nil
	}

	switch expression.Operator {
	case BooleanScalar:
		budget.reserveOutput(1)
		return &SemanticExpression{Operator: BooleanScalar, Scalar: cloneScalarPredicate(expression.Scalar, budget)}
	case BooleanCondition:
		if expression.ConditionIndex < 0 || expression.ConditionIndex >= len(conditions) {
			return nil
		}
		budget.reserveOutput(1)
		return &SemanticExpression{
			Operator:  BooleanCondition,
			Condition: &conditions[expression.ConditionIndex],
		}
	case BooleanAnd, BooleanOr:
		budget.reserveOutput(1 + len(expression.Children))
		converted := &SemanticExpression{Operator: expression.Operator}
		for _, child := range expression.Children {
			budget.step()
			childExpression := semanticExpressionFromParseExpression(child, conditions, budget)
			if childExpression == nil {
				return nil
			}
			converted.Children = append(converted.Children, childExpression)
		}
		if len(converted.Children) == 0 {
			return nil
		}
		return converted
	default:
		return nil
	}
}

func semanticExpressionFromConditions(conditions []Condition, budget *parseBudget) *SemanticExpression {
	budget.step()
	if len(conditions) == 0 {
		return nil
	}
	var expression *SemanticExpression
	for index := range conditions {
		budget.step()
		budget.reserveOutput(1)
		leaf := &SemanticExpression{Operator: BooleanCondition, Condition: &conditions[index]}
		if expression == nil {
			expression = leaf
			continue
		}
		operator := BooleanAnd
		if conditions[index].LogicalOp == "OR" {
			operator = BooleanOr
		}
		expression = combineSemanticExpressions(operator, expression, leaf, budget)
	}
	return expression
}

func combineSemanticExpressions(operator BooleanOperator, left, right *SemanticExpression, budget *parseBudget) *SemanticExpression {
	budget.step()
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	budget.reserveOutput(1)
	combined := &SemanticExpression{Operator: operator}
	if left.Operator == operator && left.Condition == nil {
		budget.reserveOutput(len(left.Children))
		combined.Children = append(combined.Children, left.Children...)
	} else {
		budget.reserveOutput(1)
		combined.Children = append(combined.Children, left)
	}
	if right.Operator == operator && right.Condition == nil {
		budget.reserveOutput(len(right.Children))
		combined.Children = append(combined.Children, right.Children...)
	} else {
		budget.reserveOutput(1)
		combined.Children = append(combined.Children, right)
	}
	return combined
}

func semanticJoinsFromParseJoins(joins []JoinInfo, budget *parseBudget) []SemanticJoin {
	budget.step()
	if len(joins) == 0 {
		return nil
	}
	budget.reserveOutput(len(joins))
	out := make([]SemanticJoin, 0, len(joins))
	for _, join := range joins {
		budget.step()
		out = append(out, SemanticJoin{
			Type:          join.Type,
			JoinFields:    cloneSemanticStrings(join.JoinFields, budget),
			LeftFields:    cloneSemanticStrings(join.LeftFields, budget),
			RightFields:   cloneSemanticStrings(join.RightFields, budget),
			RightTable:    join.RightTable,
			Subquery:      semanticFromExtraction(join.Subsearch, budget),
			PipeStage:     join.PipeStage,
			ExposedFields: cloneSemanticStrings(join.ExposedFields, budget),
		})
	}
	return out
}

func cloneSemanticConditions(conditions []Condition, budget *parseBudget) []Condition {
	budget.step()
	if len(conditions) == 0 {
		return nil
	}
	budget.reserveOutput(len(conditions))
	out := make([]Condition, len(conditions))
	for i, condition := range conditions {
		budget.step()
		out[i] = condition
		out[i].Alternatives = cloneSemanticStrings(condition.Alternatives, budget)
	}
	return out
}

func cloneSemanticStrings(values []string, budget *parseBudget) []string {
	budget.step()
	if len(values) == 0 {
		return nil
	}
	budget.reserveOutput(len(values))
	return append([]string(nil), values...)
}

func cloneSemanticStringMap(values map[string]string, budget *parseBudget) map[string]string {
	budget.step()
	if len(values) == 0 {
		return nil
	}
	budget.reserveOutput(len(values))
	out := make(map[string]string, len(values))
	for key, value := range values {
		budget.step()
		out[key] = value
	}
	return out
}
