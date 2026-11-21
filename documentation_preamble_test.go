package kql

import (
	"reflect"
	"testing"
)

// A table followed by a pipe starts the query: the documentation-preamble
// heuristic must not skip that line for its word count, its operators or the
// casing of the table name, and so drop its filters.
func TestQueryFirstLineIsNotDocumentation(t *testing.T) {
	type condition struct{ field, operator, value string }
	for _, tc := range []struct {
		name, query string
		want        []condition
	}{
		{"relational operators", "SecurityEvent | where EventID > 4000 and Level < 3\n| where Computer has \"dc\"",
			[]condition{{"EventID", ">", "4000"}, {"Level", "<", "3"}, {"Computer", "has", "dc"}}},
		{"has and =~", "DeviceProcessEvents | where FileName =~ \"powershell.exe\" and ProcessCommandLine has \"-enc\"\n| where InitiatingProcessFileName =~ \"winword.exe\"",
			[]condition{{"FileName", "=~", "powershell.exe"}, {"ProcessCommandLine", "has", "-enc"}, {"InitiatingProcessFileName", "=~", "winword.exe"}}},
		{"lower-case table", "requests | where success == false\n| where resultCode == 500",
			[]condition{{"success", "==", "false"}, {"resultCode", "==", "500"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractConditions(tc.query)
			var got []condition
			for _, c := range result.Conditions {
				got = append(got, condition{c.Field, c.Operator, c.Value})
			}
			if len(result.Errors) != 0 || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("conditions = %v errors=%q, want %v", got, result.Errors, tc.want)
			}
			if e := result.Expression; e == nil || e.Operator != BooleanAnd || len(e.Children) != len(tc.want) {
				t.Fatalf("expression = %+v, want an AND of all %d conditions", e, len(tc.want))
			}
		})
	}

	// A lower-case table alone on its line, before the first pipe, is kept too.
	if got, want := NormalizeQueryForDebug("requests\n| where success == false"), "requests\n| where success == false"; got != want {
		t.Fatalf("NormalizeQueryForDebug() = %q, want %q", got, want)
	}
}
