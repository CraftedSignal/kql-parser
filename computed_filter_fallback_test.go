package kql

import (
	"fmt"
	"strings"
	"testing"
)

// renderFallbackExpression prints an expression as nested Boolean groups of
// [field operator values] leaves.
func renderFallbackExpression(result *ParseResult, expression *ConditionExpression) string {
	if expression == nil {
		return "<nil>"
	}
	switch expression.Operator {
	case BooleanCondition:
		condition := result.Conditions[expression.ConditionIndex]
		prefix := ""
		if condition.Negated {
			prefix = "NOT "
		}
		values := fmt.Sprintf("%q", condition.Value)
		if len(condition.Alternatives) > 0 {
			values = fmt.Sprintf("%q", condition.Alternatives)
		}
		return fmt.Sprintf("%s[%s %s %s]", prefix, condition.Field, condition.Operator, values)
	case BooleanScalar:
		return "scalar{" + expression.Scalar.Text + "}"
	}
	parts := make([]string, 0, len(expression.Children))
	for _, child := range expression.Children {
		parts = append(parts, renderFallbackExpression(result, child))
	}
	return "(" + strings.Join(parts, " "+strings.ToUpper(string(expression.Operator))+" ") + ")"
}

// Filters after summarize read computed columns: default aggregate names such
// as count_ and dcount_X, and group keys. The text fallback must not append a
// second copy of a filter the tree walker already captured on such a column.
func TestExtractConditions_ComputedColumnFilterIsNotDuplicated(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`SecurityEvent | summarize count() by Account | where count_ > 10 or count_ < 2`, `([count_ > "10"] OR [count_ < "2"])`},
		{`SecurityEvent | summarize dcount(Computer) by Account | where dcount_Computer !in (1, 2)`, `NOT [dcount_Computer in ["1" "2"]]`},
		{`SecurityEvent | summarize count() by Account | where count_ > 10 and Account startswith "adm"`, `([count_ > "10"] AND [Account startswith "adm"])`},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			if got := renderFallbackExpression(result, result.Expression); got != tt.want || len(result.Errors) != 0 {
				t.Fatalf("expression = %s (conditions=%d, errors=%q)\nwant         %s and no error", got, len(result.Conditions), result.Errors, tt.want)
			}
			for _, condition := range result.Conditions {
				if !condition.IsComputed {
					t.Fatalf("condition on %s is not marked computed: %+v", condition.Field, condition)
				}
			}
		})
	}
}
