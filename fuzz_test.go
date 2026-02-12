package kql

import (
	"strings"
	"testing"
)

// FuzzKQLParser is a crash/panic fuzz test for the KQL parser.
// It verifies that ExtractConditions never panics on arbitrary input.
func FuzzKQLParser(f *testing.F) {
	// Simple where clauses
	f.Add("SecurityEvent | where EventID == 4624")
	f.Add("SecurityEvent | where Status != \"Success\"")
	f.Add("SecurityEvent | where Count > 100")
	f.Add("SecurityEvent | where Level < 3")
	f.Add("SecurityEvent | where Score >= 50")
	f.Add("SecurityEvent | where Rank <= 10")

	// Multiple piped where clauses
	f.Add("SecurityEvent | where EventID == 4624 | where Status == \"Success\" | where Account != \"SYSTEM\"")
	f.Add("SecurityEvent | where EventID == 4625 | where LogonType == 10 | where IpAddress != \"127.0.0.1\"")

	// Boolean logic (and, or, not)
	f.Add("SecurityEvent | where EventID == 4624 and Status == \"Success\"")
	f.Add("SecurityEvent | where EventID == 4624 or EventID == 4625")
	f.Add("SecurityEvent | where not(Status == \"Failed\")")
	f.Add("SecurityEvent | where (EventID == 4624 or EventID == 4625) and Status == \"Success\"")
	f.Add("SecurityEvent | where EventID == 4624 and (Status == \"Success\" or Status == \"0\")")

	// Comparison operators (==, !=, >, <, >=, <=)
	f.Add("T | where A == 1 and B != 2 and C > 3 and D < 4 and E >= 5 and F <= 6")

	// String operators (contains, has, startswith, endswith, matches regex)
	f.Add("SecurityEvent | where CommandLine contains \"powershell\"")
	f.Add("SecurityEvent | where Message has \"error\"")
	f.Add("SecurityEvent | where FilePath startswith \"C:\\\\Windows\"")
	f.Add("SecurityEvent | where FileName endswith \".exe\"")
	f.Add("SecurityEvent | where Account matches regex @\"admin[0-9]+\"")

	// Case-insensitive variants (~)
	f.Add("SecurityEvent | where UserName =~ \"admin\"")
	f.Add("SecurityEvent | where Account !contains_cs \"SYSTEM\"")
	f.Add("SecurityEvent | where Message contains_cs \"ERROR\"")
	f.Add("SecurityEvent | where FilePath startswith_cs \"C:\\\\\"")
	f.Add("SecurityEvent | where FileName endswith_cs \".EXE\"")
	f.Add("SecurityEvent | where AccountType =~ \"User\"")

	// in/!in operators
	f.Add("SecurityEvent | where EventID in (4624, 4625, 4626)")
	f.Add("SecurityEvent | where Status !in (\"Failed\", \"Error\")")
	f.Add("SecurityEvent | where LogonType in (2, 10, 11)")
	f.Add("SecurityEvent | where FileName in~ (\"cmd.exe\", \"powershell.exe\", \"pwsh.exe\")")

	// between operator
	f.Add("SecurityEvent | where TimeGenerated between (ago(1d) .. ago(1h))")
	f.Add("SecurityEvent | where Count between (10 .. 100)")
	f.Add("SecurityEvent | where EventID between (4624 .. 4626)")

	// Numeric field names (Sysmon-style)
	f.Add("SysmonData | where 1 == \"CreateProcess\" and 3 == \"cmd.exe\"")
	f.Add("SysmonData | where 1 == \"CreateProcess\" and 7 == \"x86\" and 10 == \"C:\\\\Windows\\\\System32\"")
	f.Add("SysmonData | where 22 contains \"malicious.com\" and 3 == \"dns.exe\"")
	f.Add("SysmonData | where 1 == \"CreateProcess\" | where 3 startswith \"C:\\\\\"")
	f.Add("SysmonData | where 1 in (\"CreateProcess\", \"ProcessTerminate\")")

	// let statements
	f.Add("let threshold = 10; SecurityEvent | where EventID == 4625 | where Count > threshold")
	f.Add("let timeframe = 1d; let limit = 50; SecurityEvent | where TimeGenerated > ago(timeframe)")
	f.Add("let items = dynamic([\"a\", \"b\"]); SecurityEvent | where Account has_any (items)")

	// summarize/extend/project
	f.Add("SecurityEvent | where EventID == 4624 | summarize count() by Account, Computer")
	f.Add("SecurityEvent | where EventID == 4625 | extend FullName = strcat(Domain, \"\\\\\", Account)")
	f.Add("SecurityEvent | where EventID == 4624 | project TimeGenerated, Account, Computer, EventID")
	f.Add("SecurityEvent | where EventID == 4624 | extend IsAdmin = iff(Account contains \"admin\", true, false) | where IsAdmin == true")
	f.Add("SecurityEvent | extend Risk = case(EventID == 4625, \"High\", EventID == 4624, \"Low\", \"Medium\") | where Risk == \"High\"")

	// join/union
	f.Add("SecurityEvent | where EventID == 4624 | join kind=inner (AuditLogs | where OperationName has \"password\") on Account")
	f.Add("union SecurityEvent, WindowsEvent | where EventID == 4625")
	f.Add("SecurityEvent | join kind=leftouter (SigninLogs | where ResultType != \"0\") on $left.Account == $right.UserPrincipalName")

	// Real security detection queries (Sysmon)
	f.Add(`DeviceProcessEvents
| where ActionType == "ProcessCreated"
| where FileName in ("cmd.exe", "powershell.exe", "pwsh.exe")
| where InitiatingProcessFileName !in ("explorer.exe", "services.exe")
| project Timestamp, DeviceName, FileName, ProcessCommandLine`)

	// Real security detection queries (Windows Security)
	f.Add(`SecurityEvent
| where EventID == 4625
| where LogonType == 10
| summarize FailedAttempts = count(), UniqueAccounts = dcount(TargetAccount), Accounts = make_set(TargetAccount)
by IpAddress, Computer, bin(TimeGenerated, 1h)
| where FailedAttempts > 20`)

	// Real security detection queries (Azure AD)
	f.Add(`SigninLogs
| where ResultType != "0"
| where RiskLevelDuringSignIn != "none"
| extend DeviceOS = tostring(DeviceDetail.operatingSystem)
| summarize RiskySignins = count(), UniqueApps = dcount(AppDisplayName), Locations = make_set(Location)
by UserPrincipalName, RiskLevelDuringSignIn, DeviceOS`)

	// Kerberoasting detection
	f.Add(`SecurityEvent
| where EventID == 4769
| where TicketEncryptionType == "0x17"
| where TicketOptions == "0x40810000"
| where Status == "0x0"
| where ServiceName !contains "$" and ServiceName !contains "krbtgt"`)

	// Network anomaly detection
	f.Add(`DeviceNetworkEvents
| where ActionType == "ConnectionSuccess"
| where RemotePort in (22, 23, 3389, 5900)
| where RemoteIP !startswith "10." and RemoteIP !startswith "192.168."
| where LocalIP startswith "10."`)

	// Edge cases: empty/malformed
	f.Add("")
	f.Add("|")
	f.Add("| where")
	f.Add("| where |")
	f.Add("SecurityEvent |")
	f.Add("SecurityEvent | where")
	f.Add("SecurityEvent | where ==")
	f.Add("SecurityEvent | where Field ==")
	f.Add("| | | | |")
	f.Add("where where where")

	// Edge cases: deeply nested
	f.Add("T | where ((((A == 1))))")
	f.Add("T | where (A == 1 and (B == 2 or (C == 3 and (D == 4 or E == 5))))")

	// Edge cases: special characters
	f.Add("T | where Field == \"value with spaces and 'quotes'\"")
	f.Add("T | where Field == @\"C:\\path\\to\\file\"")
	f.Add("T | where Field contains \"\\n\\t\\r\"")
	f.Add("T | where Field matches regex @\"^[a-zA-Z0-9]+$\"")

	// Edge cases: very long field names
	f.Add("T | where VeryLongFieldNameThatExceedsNormalLength == \"value\"")

	// Edge cases: unicode
	f.Add("T | where Message contains \"utilisateur\" or Message contains \"Benutzer\"")

	// Dotted field names
	f.Add("AuditLogs | where TargetResources.displayName == \"admin\"")
	f.Add("SigninLogs | where DeviceDetail.operatingSystem == \"Windows\"")

	// has_any / has_all
	f.Add("T | where ProcessCommandLine has_any (\"cmd\", \"powershell\", \"bash\")")
	f.Add("T | where Message has_all (\"error\", \"critical\")")

	// Conditions after summarize
	f.Add("SecurityEvent | summarize cnt = count() by Account | where cnt > 10")
	f.Add("SecurityEvent | summarize TotalEvents = count() by Computer | where TotalEvents >= 100")

	f.Fuzz(func(t *testing.T, query string) {
		// The parser must never panic, regardless of input
		result := ExtractConditions(query)
		if result == nil {
			t.Error("ExtractConditions returned nil")
		}
	})
}

// TestConditionCompleteness verifies that ExtractConditions extracts all expected
// fields from a variety of KQL query patterns. This is a deterministic test that
// catches regressions in field extraction.
func TestConditionCompleteness(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedFields []string
	}{
		{
			name:           "simple equality",
			query:          "SecurityEvent | where EventID == 4624",
			expectedFields: []string{"EventID"},
		},
		{
			name:           "two piped where clauses",
			query:          "SecurityEvent | where EventID == 4624 | where Status == \"Success\"",
			expectedFields: []string{"EventID", "Status"},
		},
		{
			name:           "three piped where clauses",
			query:          "SecurityEvent | where EventID == 4624 | where AccountType == \"User\" | where LogonType == 10",
			expectedFields: []string{"EventID", "AccountType", "LogonType"},
		},
		{
			name:           "AND conditions in single where",
			query:          "SecurityEvent | where EventID == 4625 and Status == \"Failed\" and IpAddress != \"127.0.0.1\"",
			expectedFields: []string{"EventID", "Status", "IpAddress"},
		},
		{
			name:           "OR conditions in single where",
			query:          "SecurityEvent | where EventID == 4624 or EventID == 4625",
			expectedFields: []string{"EventID"},
		},
		{
			name:           "mixed AND/OR with parentheses",
			query:          "SecurityEvent | where (EventID == 4624 or EventID == 4625) and AccountType == \"User\"",
			expectedFields: []string{"EventID", "AccountType"},
		},
		{
			name:           "contains operator",
			query:          "DeviceProcessEvents | where ProcessCommandLine contains \"powershell\"",
			expectedFields: []string{"ProcessCommandLine"},
		},
		{
			name:           "has operator",
			query:          "SecurityEvent | where Message has \"error\"",
			expectedFields: []string{"Message"},
		},
		{
			name:           "startswith operator",
			query:          "SecurityEvent | where FilePath startswith \"C:\\\\Windows\"",
			expectedFields: []string{"FilePath"},
		},
		{
			name:           "multiple string operators",
			query:          "DeviceProcessEvents | where FileName endswith \".exe\" and ProcessCommandLine contains \"admin\" and FolderPath startswith \"C:\\\\\"",
			expectedFields: []string{"FileName", "ProcessCommandLine", "FolderPath"},
		},
		{
			name:           "in operator with list",
			query:          "SecurityEvent | where EventID in (4624, 4625, 4626)",
			expectedFields: []string{"EventID"},
		},
		{
			name:           "not-in operator with list",
			query:          "SecurityEvent | where Status !in (\"Failed\", \"Error\", \"Timeout\")",
			expectedFields: []string{"Status"},
		},
		{
			name:           "between operator",
			query:          "SecurityEvent | where Count between (10 .. 100)",
			expectedFields: []string{"Count"},
		},
		{
			name:           "negated contains",
			query:          "SecurityEvent | where ServiceName !contains \"$\" and ServiceName !contains \"krbtgt\"",
			expectedFields: []string{"ServiceName"},
		},
		{
			name:           "case insensitive equality",
			query:          "SecurityEvent | where AccountType =~ \"User\"",
			expectedFields: []string{"AccountType"},
		},
		{
			name:           "numeric field names with surrounding conditions",
			query:          "SysmonData | where 1 == \"CreateProcess\" and 3 == \"cmd.exe\"",
			expectedFields: []string{"1", "3"},
		},
		{
			name:           "multiple numeric field names",
			query:          "SysmonData | where 1 == \"CreateProcess\" and 7 == \"x86\" and 10 == \"C:\\\\Windows\\\\System32\"",
			expectedFields: []string{"1", "7", "10"},
		},
		{
			name:           "numeric field name with string operator",
			query:          "SysmonData | where 22 contains \"malicious.com\" and 3 == \"dns.exe\"",
			expectedFields: []string{"22", "3"},
		},
		{
			name:           "numeric field name in piped where",
			query:          "SysmonData | where 1 == \"CreateProcess\" | where 3 startswith \"C:\\\\\"",
			expectedFields: []string{"1", "3"},
		},
		{
			name:           "dotted field names",
			query:          "AuditLogs | where TargetResources.displayName == \"admin\"",
			expectedFields: []string{"TargetResources.displayName"},
		},
		{
			name:           "real-world Sentinel: brute force RDP",
			query:          "SecurityEvent | where EventID == 4625 | where LogonType == 10",
			expectedFields: []string{"EventID", "LogonType"},
		},
		{
			name:           "real-world Sentinel: process creation monitoring",
			query:          "DeviceProcessEvents | where ActionType == \"ProcessCreated\" | where FileName in (\"cmd.exe\", \"powershell.exe\") | where InitiatingProcessFileName !in (\"explorer.exe\", \"services.exe\")",
			expectedFields: []string{"ActionType", "FileName", "InitiatingProcessFileName"},
		},
		{
			name:           "real-world Sentinel: Azure AD sign-in risk",
			query:          "SigninLogs | where RiskLevelDuringSignIn != \"none\" | where ResultType == \"0\"",
			expectedFields: []string{"RiskLevelDuringSignIn", "ResultType"},
		},
		{
			name:           "conditions after extend",
			query:          "SecurityEvent | extend FullName = strcat(TargetDomainName, \"\\\\\", TargetUserName) | where FullName !has \"SYSTEM\"",
			expectedFields: []string{"FullName"},
		},
		{
			name:           "conditions after summarize",
			query:          "SecurityEvent | where EventID == 4625 | summarize FailedAttempts = count() by Account | where FailedAttempts > 20",
			expectedFields: []string{"EventID", "FailedAttempts"},
		},
		{
			name:           "multiple comparison operators",
			query:          "T | where A > 1 and B < 100 and C >= 5 and D <= 50",
			expectedFields: []string{"A", "B", "C", "D"},
		},
		{
			name:           "real-world Sentinel: Kerberoasting",
			query:          "SecurityEvent | where EventID == 4769 | where TicketEncryptionType == \"0x17\" | where Status == \"0x0\" | where ServiceName !contains \"$\"",
			expectedFields: []string{"EventID", "TicketEncryptionType", "Status", "ServiceName"},
		},
		{
			name:           "real-world Sentinel: network connections",
			query:          "DeviceNetworkEvents | where ActionType == \"ConnectionSuccess\" | where RemotePort in (22, 23, 3389, 5900) | where RemoteIP !startswith \"10.\"",
			expectedFields: []string{"ActionType", "RemotePort", "RemoteIP"},
		},
		{
			name:           "NOT expression",
			query:          "SecurityEvent | where not(Status == \"Success\")",
			expectedFields: []string{"Status"},
		},
		{
			name:           "in operator with strings",
			query:          "DeviceProcessEvents | where FileName in~ (\"cmd.exe\", \"powershell.exe\", \"pwsh.exe\")",
			expectedFields: []string{"FileName"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			extractedFields := make(map[string]bool)
			for _, c := range result.Conditions {
				extractedFields[strings.ToLower(c.Field)] = true
			}
			for _, expected := range tt.expectedFields {
				if !extractedFields[strings.ToLower(expected)] {
					t.Errorf("field %q not extracted\n  query: %s\n  extracted: %v\n  errors: %v",
						expected, tt.query, extractedFields, result.Errors)
				}
			}
		})
	}
}
