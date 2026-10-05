package kql

import "testing"

func TestAnalyzeQueryBuildsConditionBearingSemanticExpression(t *testing.T) {
	semantic := AnalyzeQuery(`SecurityEvent | where (CommandLine contains "whoami" or Image endswith "\\cmd.exe") and not(User == "SYSTEM")`)
	if semantic == nil {
		t.Fatal("expected semantic query")
	}
	if errors := unexpectedParserErrors(semantic.Errors); len(errors) > 0 {
		t.Fatalf("unexpected parser errors: %v", errors)
	}
	if len(semantic.Conditions) != 3 {
		t.Fatalf("conditions = %#v, want 3", semantic.Conditions)
	}
	if semantic.Expression == nil || semantic.Expression.Operator != BooleanAnd {
		t.Fatalf("expression = %#v, want AND root", semantic.Expression)
	}
	if len(semantic.Expression.Children) != 2 {
		t.Fatalf("AND children = %#v, want 2", semantic.Expression.Children)
	}
	left := semantic.Expression.Children[0]
	if left.Operator != BooleanOr || len(left.Children) != 2 {
		t.Fatalf("left expression = %#v, want OR with two children", left)
	}
	if got := left.Children[0].Condition.Field; got != "CommandLine" {
		t.Fatalf("left first field = %q, want CommandLine", got)
	}
	if got := left.Children[1].Condition.Field; got != "Image" {
		t.Fatalf("left second field = %q, want Image", got)
	}
	right := semantic.Expression.Children[1]
	if right.Operator != BooleanCondition || right.Condition == nil || right.Condition.Field != "User" || !right.Condition.Negated {
		t.Fatalf("right expression = %#v, want negated User condition", right)
	}
}

func TestSemanticInternalExtractionBuildsLogicalOpsAndClones(t *testing.T) {
	result := &ParseResult{
		Conditions: []Condition{
			{Field: "CommandLine", Operator: "contains", Value: "whoami", Alternatives: []string{"whoami", "hostname"}},
			{Field: "Image", Operator: "endswith", Value: "cmd.exe", LogicalOp: "OR"},
		},
		DataSources:    []string{"SecurityEvent"},
		ComputedFields: map[string]string{"Alias": "CommandLine"},
	}

	semantic := semanticFromExtraction(result)
	if semantic == nil {
		t.Fatal("expected semantic query")
	}
	if semantic.Expression == nil || semantic.Expression.Operator != BooleanOr || len(semantic.Expression.Children) != 2 {
		t.Fatalf("internal expression = %#v, want OR over two conditions", semantic.Expression)
	}
	if got := semantic.Expression.Children[0].Condition.Field; got != "CommandLine" {
		t.Fatalf("first internal condition field = %q", got)
	}

	result.Conditions[0].Field = "Changed"
	result.Conditions[0].Alternatives[0] = "changed"
	result.DataSources[0] = "Changed"
	result.ComputedFields["Alias"] = "Changed"
	if semantic.Conditions[0].Field != "CommandLine" ||
		semantic.Conditions[0].Alternatives[0] != "whoami" ||
		semantic.DataSources[0] != "SecurityEvent" ||
		semantic.ComputedFields["Alias"] != "CommandLine" {
		t.Fatalf("semantic query did not clone parse result: %#v", semantic)
	}
}

func TestSemanticInternalExtractionPreservesNestedJoinSubqueries(t *testing.T) {
	semantic := semanticFromExtraction(&ParseResult{
		Joins: []JoinInfo{{
			Type:       "inner",
			JoinFields: []string{"Account"},
			Subsearch: &ParseResult{
				DataSources: []string{"SigninLogs"},
				Conditions:  []Condition{{Field: "ResultType", Operator: "==", Value: "0"}},
			},
			ExposedFields: []string{"ResultType"},
		}},
	})
	if semantic == nil || len(semantic.Joins) != 1 {
		t.Fatalf("semantic joins = %#v, want one join", semantic)
	}
	subquery := semantic.Joins[0].Subquery
	if subquery == nil || subquery.Expression == nil || subquery.Expression.Condition == nil {
		t.Fatalf("join subquery semantic expression = %#v", subquery)
	}
	if got := subquery.Expression.Condition.Field; got != "ResultType" {
		t.Fatalf("subquery condition field = %q, want ResultType", got)
	}
	if got := subquery.DataSources[0]; got != "SigninLogs" {
		t.Fatalf("subquery datasource = %q, want SigninLogs", got)
	}
}

func TestSemanticExpressionConversionEdges(t *testing.T) {
	if got := semanticFromExtraction(nil); got != nil {
		t.Fatalf("nil parse result converted to %#v", got)
	}

	conditions := []Condition{
		{Field: "A", Operator: "==", Value: "1"},
		{Field: "B", Operator: "==", Value: "2"},
	}
	for name, expression := range map[string]*ConditionExpression{
		"bad_index": {
			Operator:       BooleanCondition,
			ConditionIndex: len(conditions),
		},
		"empty_boolean": {
			Operator: BooleanAnd,
		},
		"bad_child": {
			Operator: BooleanOr,
			Children: []*ConditionExpression{
				{Operator: BooleanCondition, ConditionIndex: 0},
				{Operator: BooleanCondition, ConditionIndex: len(conditions)},
			},
		},
		"unknown_operator": {
			Operator: "xor",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := semanticExpressionFromParseExpression(expression, conditions); got != nil {
				t.Fatalf("invalid expression converted to %#v", got)
			}
		})
	}

	leafA := &SemanticExpression{Operator: BooleanCondition, Condition: &conditions[0]}
	leafB := &SemanticExpression{Operator: BooleanCondition, Condition: &conditions[1]}
	if got := combineSemanticExpressions(BooleanAnd, nil, leafA); got != leafA {
		t.Fatalf("nil left combine = %#v, want leafA", got)
	}
	if got := combineSemanticExpressions(BooleanAnd, leafA, nil); got != leafA {
		t.Fatalf("nil right combine = %#v, want leafA", got)
	}
	combined := combineSemanticExpressions(
		BooleanAnd,
		&SemanticExpression{Operator: BooleanAnd, Children: []*SemanticExpression{leafA}},
		&SemanticExpression{Operator: BooleanAnd, Children: []*SemanticExpression{leafB}},
	)
	if combined.Operator != BooleanAnd || len(combined.Children) != 2 {
		t.Fatalf("flattened combine = %#v, want AND with two children", combined)
	}
}
