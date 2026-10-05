package kql

import (
	"testing"

	"github.com/antlr4-go/antlr/v4"
)

func kqlParserForTest(t *testing.T, query string) *KQLParser {
	t.Helper()

	input := antlr.NewInputStream(query)
	lexer := NewKQLLexer(input)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	return NewKQLParser(stream)
}

func kqlExpressionForTest(t *testing.T, expression string) IExpressionContext {
	t.Helper()

	return kqlParserForTest(t, expression).Expression()
}

func TestKQLExpressionBuilderHelperEdges(t *testing.T) {
	conditions := []Condition{
		{Field: "a", Operator: "==", Value: "1"},
		{Field: "b", Operator: "==", Value: "2"},
	}
	builder := &kqlExpressionBuilder{conditions: conditions}

	if got := buildConditionExpression(nil, conditions); got != nil {
		t.Fatalf("nil tree returned expression %#v", got)
	}
	if got := buildConditionExpression(kqlParserForTest(t, `SecurityEvent | where a == 1`).Query(), conditions[:1]); got != nil {
		t.Fatalf("single condition returned expression %#v", got)
	}
	if got := collectKQLWhereOperators(nil); got != nil {
		t.Fatalf("nil tree where collection returned %#v", got)
	}
	if got := builder.expression(nil); got != nil {
		t.Fatalf("nil expression returned %#v", got)
	}
	if got := builder.expression(NewEmptyExpressionContext()); got != nil {
		t.Fatalf("empty expression context returned %#v", got)
	}
	if got := builder.notExpression(nil); got != nil {
		t.Fatalf("nil NOT expression returned %#v", got)
	}
	if got := builder.comparisonExpression(nil); got != nil {
		t.Fatalf("nil comparison expression returned %#v", got)
	}
	if got := firstNestedKQLExpression(nil); got != nil {
		t.Fatalf("nil nested expression returned %#v", got)
	}

	exhausted := &kqlExpressionBuilder{conditions: conditions, next: len(conditions)}
	if got := exhausted.condition(); got != nil {
		t.Fatalf("exhausted condition builder returned %#v", got)
	}
}

func TestKQLExpressionBuilderDirectParsingEdges(t *testing.T) {
	builder := &kqlExpressionBuilder{
		conditions: []Condition{
			{Field: "a", Operator: "==", Value: "1"},
			{Field: "b", Operator: "==", Value: "2"},
			{Field: "c", Operator: "==", Value: "3"},
		},
	}
	expression := builder.expression(kqlExpressionForTest(t, `(a == 1 or b == 2) and not(c == 3)`))
	if expression == nil || expression.Operator != BooleanAnd || len(expression.Children) != 2 {
		t.Fatalf("expression did not preserve AND over grouped OR and NOT, got %#v", expression)
	}
	if expression.Children[0].Operator != BooleanOr || len(expression.Children[0].Children) != 2 {
		t.Fatalf("left branch should preserve OR grouping, got %#v", expression.Children[0])
	}
	if expression.Children[1].Operator != BooleanCondition || expression.Children[1].ConditionIndex != 2 {
		t.Fatalf("right branch should remain the third condition leaf, got %#v", expression.Children[1])
	}
}

func TestKQLExpressionRemapAndEqualityEdges(t *testing.T) {
	leaf0 := &ConditionExpression{Operator: BooleanCondition, ConditionIndex: 0}
	leaf1 := &ConditionExpression{Operator: BooleanCondition, ConditionIndex: 1}
	nested := &ConditionExpression{
		Operator: BooleanAnd,
		Children: []*ConditionExpression{
			leaf0,
			{
				Operator: BooleanAnd,
				Children: []*ConditionExpression{
					leaf1,
					{Operator: BooleanCondition, ConditionIndex: 2},
				},
			},
			{Operator: BooleanCondition, ConditionIndex: 3},
		},
	}

	if got := remapConditionExpression(nil, []int{0}); got != nil {
		t.Fatalf("nil expression remapped to %#v", got)
	}
	if got := remapConditionExpression(leaf0, nil); got != leaf0 {
		t.Fatalf("empty index map should return original expression, got %#v", got)
	}
	if got := remapConditionExpression(&ConditionExpression{Operator: BooleanCondition, ConditionIndex: -1}, []int{0}); got != nil {
		t.Fatalf("negative condition index remapped to %#v", got)
	}
	if got := remapConditionExpression(&ConditionExpression{Operator: BooleanCondition, ConditionIndex: 2}, []int{0}); got != nil {
		t.Fatalf("out-of-range condition index remapped to %#v", got)
	}
	if got := remapConditionExpression(&ConditionExpression{Operator: BooleanCondition, ConditionIndex: 0}, []int{-1}); got != nil {
		t.Fatalf("dropped condition index remapped to %#v", got)
	}
	if got := remapConditionExpression(&ConditionExpression{Operator: "xor"}, []int{0}); got != nil {
		t.Fatalf("unknown expression operator remapped to %#v", got)
	}

	remapped := remapConditionExpression(nested, []int{1, 0, 2, -1})
	if remapped == nil || remapped.Operator != BooleanAnd || len(remapped.Children) != 3 {
		t.Fatalf("remap should flatten same-operator children and prune dropped indexes, got %#v", remapped)
	}
	if remapped.Children[0].ConditionIndex != 1 || remapped.Children[1].ConditionIndex != 0 || remapped.Children[2].ConditionIndex != 2 {
		t.Fatalf("remap did not apply expected indexes, got %#v", remapped)
	}
	if got := remapConditionExpression(&ConditionExpression{
		Operator: BooleanOr,
		Children: []*ConditionExpression{
			{Operator: BooleanCondition, ConditionIndex: 0},
			{Operator: BooleanCondition, ConditionIndex: 1},
		},
	}, []int{-1, -1}); got != nil {
		t.Fatalf("fully pruned expression remapped to %#v", got)
	}
	collapsed := remapConditionExpression(&ConditionExpression{
		Operator: BooleanOr,
		Children: []*ConditionExpression{
			{Operator: BooleanCondition, ConditionIndex: 0},
			{Operator: BooleanCondition, ConditionIndex: 1},
		},
	}, []int{-1, 0})
	if collapsed == nil || collapsed.Operator != BooleanCondition || collapsed.ConditionIndex != 0 {
		t.Fatalf("single remaining child did not collapse, got %#v", collapsed)
	}

	if !equalConditionExpression(nil, nil) {
		t.Fatalf("nil expressions should compare equal")
	}
	if equalConditionExpression(leaf0, nil) {
		t.Fatalf("non-nil expression compared equal to nil")
	}
	if equalConditionExpression(leaf0, leaf1) {
		t.Fatalf("different condition indexes compared equal")
	}
	if equalConditionExpression(
		&ConditionExpression{Operator: BooleanAnd, Children: []*ConditionExpression{leaf0}},
		&ConditionExpression{Operator: BooleanAnd, Children: []*ConditionExpression{leaf1}},
	) {
		t.Fatalf("different children compared equal")
	}
}
