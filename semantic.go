package kql

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
	return semanticFromExtraction(ExtractConditions(query))
}

// semanticFromExtraction normalizes parser extraction output into the
// parser-owned semantic model.
func semanticFromExtraction(result *ParseResult) *SemanticQuery {
	if result == nil {
		return nil
	}

	semantic := &SemanticQuery{
		Conditions:          cloneSemanticConditions(result.Conditions),
		DataSources:         cloneSemanticStrings(result.DataSources),
		LetStatements:       append([]LetStatement(nil), result.LetStatements...),
		ComputedFields:      cloneSemanticStringMap(result.ComputedFields),
		ComputedExpressions: cloneSemanticStringMap(result.ComputedExpressions),
		GroupByFields:       cloneSemanticStrings(result.GroupByFields),
		Commands:            cloneSemanticStrings(result.Commands),
		ProjectedFields:     cloneSemanticStrings(result.ProjectedFields),
		Errors:              cloneSemanticStrings(result.Errors),
	}
	semantic.Expression = semanticExpressionFromParseExpression(result.Expression, semantic.Conditions)
	if semantic.Expression == nil {
		semantic.Expression = semanticExpressionFromConditions(semantic.Conditions)
	}
	semantic.Joins = semanticJoinsFromParseJoins(result.Joins)
	return semantic
}

func semanticExpressionFromParseExpression(expression *ConditionExpression, conditions []Condition) *SemanticExpression {
	if expression == nil {
		return nil
	}

	switch expression.Operator {
	case BooleanCondition:
		if expression.ConditionIndex < 0 || expression.ConditionIndex >= len(conditions) {
			return nil
		}
		return &SemanticExpression{
			Operator:  BooleanCondition,
			Condition: &conditions[expression.ConditionIndex],
		}
	case BooleanAnd, BooleanOr:
		converted := &SemanticExpression{Operator: expression.Operator}
		for _, child := range expression.Children {
			childExpression := semanticExpressionFromParseExpression(child, conditions)
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

func semanticExpressionFromConditions(conditions []Condition) *SemanticExpression {
	if len(conditions) == 0 {
		return nil
	}
	var expression *SemanticExpression
	for index := range conditions {
		leaf := &SemanticExpression{Operator: BooleanCondition, Condition: &conditions[index]}
		if expression == nil {
			expression = leaf
			continue
		}
		operator := BooleanAnd
		if conditions[index].LogicalOp == "OR" {
			operator = BooleanOr
		}
		expression = combineSemanticExpressions(operator, expression, leaf)
	}
	return expression
}

func combineSemanticExpressions(operator BooleanOperator, left, right *SemanticExpression) *SemanticExpression {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	combined := &SemanticExpression{Operator: operator}
	if left.Operator == operator && left.Condition == nil {
		combined.Children = append(combined.Children, left.Children...)
	} else {
		combined.Children = append(combined.Children, left)
	}
	if right.Operator == operator && right.Condition == nil {
		combined.Children = append(combined.Children, right.Children...)
	} else {
		combined.Children = append(combined.Children, right)
	}
	return combined
}

func semanticJoinsFromParseJoins(joins []JoinInfo) []SemanticJoin {
	if len(joins) == 0 {
		return nil
	}
	out := make([]SemanticJoin, 0, len(joins))
	for _, join := range joins {
		out = append(out, SemanticJoin{
			Type:          join.Type,
			JoinFields:    cloneSemanticStrings(join.JoinFields),
			LeftFields:    cloneSemanticStrings(join.LeftFields),
			RightFields:   cloneSemanticStrings(join.RightFields),
			RightTable:    join.RightTable,
			Subquery:      semanticFromExtraction(join.Subsearch),
			PipeStage:     join.PipeStage,
			ExposedFields: cloneSemanticStrings(join.ExposedFields),
		})
	}
	return out
}

func cloneSemanticConditions(conditions []Condition) []Condition {
	if len(conditions) == 0 {
		return nil
	}
	out := make([]Condition, len(conditions))
	for i, condition := range conditions {
		out[i] = condition
		out[i].Alternatives = cloneSemanticStrings(condition.Alternatives)
	}
	return out
}

func cloneSemanticStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneSemanticStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
