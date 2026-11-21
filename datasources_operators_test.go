package kql

import "testing"

// TestDataSourcesIgnoreOperatorsCommentsAndParameters: a word after a pipe is a
// tabular operator, comment text is not part of the query, and join and union
// parameters may be written with spaces around "=". None of them is a table.
func TestDataSourcesIgnoreOperatorsCommentsAndParameters(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"limit", `SecurityEvent | where EventID == 4624 | limit 100 | project Account`, []string{"SecurityEvent"}},
		{"serialize", `SecurityEvent | where EventID == 4624 | serialize | extend rn = row_number()`, []string{"SecurityEvent"}},
		{"sample", `SecurityEvent | sample 10 | project Account`, []string{"SecurityEvent"}},
		{"getschema", `SecurityEvent | getschema | project ColumnName`, []string{"SecurityEvent"}},
		{"as", `SecurityEvent | as Events | project Account`, []string{"SecurityEvent"}},
		{"invoke", `SecurityEvent | invoke MyFunc() | project Account`, []string{"SecurityEvent"}},
		{"consume", `SecurityEvent | consume | project Account`, []string{"SecurityEvent"}},
		{"facet", `SecurityEvent | facet by Computer | project Account`, []string{"SecurityEvent"}},
		{"reduce", `SecurityEvent | reduce by Computer | project Account`, []string{"SecurityEvent"}},
		{"fork", `SecurityEvent | fork (where EventID == 1) | project Account`, []string{"SecurityEvent"}},
		{"scan", `SecurityEvent | scan with (step s: true;) | project Account`, []string{"SecurityEvent"}},
		{"filter", `SecurityEvent | filter EventID == 4624 | project Account`, []string{"SecurityEvent"}},
		{"mvexpand", `SecurityEvent | mvexpand Accounts | project Account`, []string{"SecurityEvent"}},
		{"pipe in line comment", "SecurityEvent\n| where EventID == 4624 // old | Bogus\n| project Account", []string{"SecurityEvent"}},
		{"semicolon in line comment", "SecurityEvent\n| where EventID == 4624 // old; Bogus\n| project Account", []string{"SecurityEvent"}},
		{"leading line comment", "// header | Bogus\nSecurityEvent\n| where EventID == 4624\n| project Account", []string{"SecurityEvent"}},
		{"slashes in a string", `SecurityEvent | where Url has "https://x" | join (Heartbeat) on Computer`, []string{"SecurityEvent", "Heartbeat"}},
		{"join kind spaced", `SecurityEvent | join kind = leftouter (Heartbeat | where OSType == "Linux") on Computer`, []string{"SecurityEvent", "Heartbeat"}},
		{"join kind spaced unparenthesized", `SecurityEvent | join kind = inner Heartbeat on Computer`, []string{"SecurityEvent", "Heartbeat"}},
		{"union kind spaced", `union kind = outer SecurityEvent, Heartbeat | where EventID == 4624`, []string{"SecurityEvent", "Heartbeat"}},
		{"union withsource spaced", `union withsource = SourceTable SecurityEvent, Heartbeat`, []string{"SecurityEvent", "Heartbeat"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSameSources(t, tt.query, ExtractConditions(tt.query).DataSources, tt.want)
		})
	}

	// A partition query is not extracted yet, so check the source scan itself.
	query := `SecurityEvent | partition by Computer (top 1 by TimeGenerated) | project Account`
	assertSameSources(t, query, extractDataSourcesFromQuery(query, nil), []string{"SecurityEvent"})
}
