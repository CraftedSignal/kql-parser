package kql

import (
	"reflect"
	"testing"
)

// in and !in are case-sensitive, in~ and !in~ are not. The operator comes from
// the authored token, never from list values that contain "in~".
func TestInOperatorCaseSensitivityFollowsToken(t *testing.T) {
	for _, tc := range []struct {
		query, operator string
		negated         bool
		values          []string
	}{
		{`SecurityEvent | where FolderPath in (@"C:\Users\ADMIN~1\AppData")`, "in", false, []string{`C:\Users\ADMIN~1\AppData`}},
		{`SecurityEvent | where FolderPath !in (@"C:\Users\ADMIN~1\AppData")`, "in", true, []string{`C:\Users\ADMIN~1\AppData`}},
		{`SecurityEvent | where Account in ("a", "join in~ b")`, "in", false, []string{"a", "join in~ b"}},
		{`SecurityEvent | where Account in~ ("a", "b")`, "in~", false, []string{"a", "b"}},
		{`SecurityEvent | where Account !in~ ("a")`, "in~", true, []string{"a"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			result := ExtractConditions(tc.query)
			if len(result.Errors) != 0 || len(result.Conditions) != 1 {
				t.Fatalf("conditions=%+v errors=%q", result.Conditions, result.Errors)
			}
			condition := result.Conditions[0]
			if condition.Operator != tc.operator || condition.Negated != tc.negated || !reflect.DeepEqual(condition.Alternatives, tc.values) {
				t.Fatalf("got operator %q negated=%v values=%q, want %q negated=%v values=%q", condition.Operator, condition.Negated, condition.Alternatives, tc.operator, tc.negated, tc.values)
			}
		})
	}
}
