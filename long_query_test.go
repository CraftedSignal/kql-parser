package kql

import (
	"strings"
	"testing"
)

// The documented input limit is 128 KiB. A query longer than 4,096 characters
// with few conditions is within every limit and must extract.
func TestLongQueryWithinInputLimitExtracts(t *testing.T) {
	for _, tc := range []struct {
		name, query string
	}{
		{"commented", "SecurityEvent\n" + strings.Repeat("// triage note kept with the rule\n", 200) + "| where EventID == 4624"},
		{"comments between stages", "SecurityEvent\n" + strings.Repeat("| extend Note = 1 // a long explanation of this triage step\n", 80) + "| where EventID == 4624"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractConditions(tc.query)
			if len(tc.query) <= 4096 || len(result.Errors) != 0 || len(result.Conditions) != 1 || result.Conditions[0].Field != "EventID" {
				t.Fatalf("query of %d bytes: conditions=%+v errors=%q", len(tc.query), result.Conditions, result.Errors)
			}
		})
	}
}
