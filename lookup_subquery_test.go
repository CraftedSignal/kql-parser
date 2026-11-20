package kql

import (
	"reflect"
	"testing"
)

// A lookup's right side is a dimension table (Microsoft Learn, lookup
// operator: "Extends the columns of a fact table with values looked-up in a
// dimension table"). A filter there selects dimension rows, so it is neither
// a condition of the main pipeline nor a reason to drop the main Expression.
// The lookup's table is a data source, as a join's right table is.
func TestExtractConditions_LookupRightSideIsNotAMainFilter(t *testing.T) {
	mainFilters := &ConditionExpression{Operator: BooleanAnd, Children: []*ConditionExpression{
		{Operator: BooleanCondition, ConditionIndex: 0},
		{Operator: BooleanCondition, ConditionIndex: 1},
	}}
	tests := []struct {
		name  string
		query string
	}{
		{"filtered subquery", `SecurityEvent | where EventID == 4624 | lookup (Watchlist | where Kind == "vip") on Account | where Tier == 0`},
		{"or inside the subquery", `SecurityEvent | where EventID == 4624 | lookup kind=inner (Watchlist | where Kind == "vip" or Kind == "admin") on Account | where Tier == 0`},
		{"table", `SecurityEvent | where EventID == 4624 | lookup kind=leftouter Watchlist on Account | where Tier == 0`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			if len(result.Errors) != 0 || len(result.Conditions) != 2 || result.Conditions[0].Field != "EventID" || result.Conditions[1].Field != "Tier" || result.Conditions[1].LogicalOp != "AND" {
				t.Fatalf("conditions = %+v errors=%q; want EventID and Tier joined by AND", result.Conditions, result.Errors)
			}
			if !reflect.DeepEqual(result.Expression, mainFilters) {
				t.Fatalf("expression = %+v; want the two main filters ANDed", result.Expression)
			}
			if !reflect.DeepEqual(result.DataSources, []string{"SecurityEvent", "Watchlist"}) {
				t.Fatalf("data_sources = %q; want [SecurityEvent Watchlist]", result.DataSources)
			}
		})
	}
}
