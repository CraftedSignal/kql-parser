package kql

import (
	"fmt"
	"strings"
	"testing"
)

func iocTestHashes(n int) string {
	hashes := make([]string, n)
	for i := range hashes {
		hashes[i] = fmt.Sprintf("%q", fmt.Sprintf("%064x", i+1))
	}
	return strings.Join(hashes, ",")
}

// An ordinary IOC list is one condition with one alternative per value. Its
// text counts against the metadata text allowance, not against output items.
func TestIOCListsExtractWithinBudget(t *testing.T) {
	short := make([]string, 30)
	for i := range short {
		short[i] = fmt.Sprintf("%q", fmt.Sprintf("v%d", i))
	}
	for _, tc := range []struct {
		name, query string
		want        int
	}{
		{"20 sha256 inline", "DeviceFileEvents | where SHA256 in (" + iocTestHashes(20) + ")", 20},
		{"20 sha256 let", "let h = dynamic([" + iocTestHashes(20) + "]);\nDeviceFileEvents | where SHA256 in (h)", 20},
		{"30 short let", "let l = dynamic([" + strings.Join(short, ",") + "]);\nSecurityEvent | where Account in (l)", 30},
		{"30 short let has_any", "let l = dynamic([" + strings.Join(short, ",") + "]);\nSecurityEvent | where Account has_any (l)", 30},
		{"500 sha256 inline", "DeviceFileEvents | where SHA256 in (" + iocTestHashes(500) + ")", 500},
		{"450 sha256 let", "let h = dynamic([" + iocTestHashes(450) + "]);\nDeviceFileEvents | where SHA256 in (h)", 450},
		{"1500 char literal", `SecurityEvent | where CommandLine == "` + strings.Repeat("x", 1500) + `"`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := ExtractConditions(tc.query)
			if len(r.Errors) != 0 || len(r.Conditions) != 1 || len(r.Conditions[0].Alternatives) != tc.want {
				t.Fatalf("query of %d bytes: conditions=%d errors=%q", len(tc.query), len(r.Conditions), r.Errors)
			}
		})
	}

	// A list past the structural allowance still fails without partial output.
	values := make([]string, 3000)
	for i := range values {
		values[i] = fmt.Sprint(i)
	}
	r := ExtractConditions("T | where x in (" + strings.Join(values, ",") + ")")
	if len(r.Errors) != 1 || !strings.Contains(r.Errors[0], "parser limit exceeded") || len(r.Conditions) != 0 {
		t.Fatalf("oversized list: conditions=%d errors=%q", len(r.Conditions), r.Errors)
	}
}
