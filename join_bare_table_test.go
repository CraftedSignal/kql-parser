package kql

import (
	"reflect"
	"testing"
)

// A join's parenthesized right side is a tabular expression, so a lone name
// there is a table. It must not become a bare-term _keyword_ search.
func TestJoinBareTableRightSideHasNoKeywordCondition(t *testing.T) {
	for _, query := range []string{
		`SecurityEvent | join kind=inner (IdentityInfo) on AccountName`,
		`SecurityEvent | join kind=inner (['Identity Info']) on AccountName`,
	} {
		t.Run(query, func(t *testing.T) {
			result := ExtractConditions(query)
			if len(result.Joins) != 1 || result.Joins[0].Subsearch == nil {
				t.Fatalf("joins = %+v, want one join with a subsearch", result.Joins)
			}
			join := result.Joins[0]
			sub := join.Subsearch
			if len(sub.Conditions) != 0 || sub.Expression != nil || len(sub.Errors) != 0 {
				t.Fatalf("subsearch conditions=%+v expression=%+v errors=%q, want none for a table", sub.Conditions, sub.Expression, sub.Errors)
			}
			if !reflect.DeepEqual(join.ExposedFields, []string{"AccountName"}) {
				t.Fatalf("exposed fields = %q, want only the join key", join.ExposedFields)
			}
		})
	}
}
