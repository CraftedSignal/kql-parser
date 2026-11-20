package kql

import "testing"

// A condition that excludes an event ID, or admits more than that ID, does not
// make the query a rule for that event type.
func TestGetEventTypeIgnoresExcludingConditions(t *testing.T) {
	for _, tc := range []struct {
		query, want string
	}{
		{`SecurityEvent | where EventID != 4688`, ""},
		{`SecurityEvent | where EventID <> 4688`, ""},
		{`SecurityEvent | where not(EventID == 4688)`, ""},
		{`SecurityEvent | where EventID !in (4688)`, ""},
		{`SecurityEvent | where EventID !in~ ("4688")`, ""},
		{`SecurityEvent | where EventID > 4688`, ""},
		{`SecurityEvent | where EventID == 4688 and EventID != 4624`, "windows_4688"},
		{`SecurityEvent | where EventID == 4624 | where not(EventID == 4625)`, "windows_4624"},
		{`SecurityEvent | where EventID == 4688`, "windows_4688"},
		{`SecurityEvent | where EventID =~ "4688"`, "windows_4688"},
		{`SecurityEvent | where EventID in (4688)`, "windows_4688"},
		{`SecurityEvent | where EventID in~ ("4688")`, "windows_4688"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			result := ExtractConditions(tc.query)
			if len(result.Errors) != 0 {
				t.Fatalf("unexpected errors: %q", result.Errors)
			}
			if got := GetEventTypeFromConditions(result); got != tc.want {
				t.Fatalf("event type = %q, want %q", got, tc.want)
			}
		})
	}
}
