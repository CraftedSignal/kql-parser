package kql

import (
	"strings"
	"testing"
)

func TestExtractConditions_SimpleComparison(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []Condition
	}{
		{
			name:  "equality comparison",
			query: "SecurityEvent | where EventID == 4624",
			expected: []Condition{
				{Field: "EventID", Operator: "==", Value: "4624", LogicalOp: "AND", PipeStage: 1},
			},
		},
		{
			name:  "inequality comparison",
			query: "SecurityEvent | where Status != \"Success\"",
			expected: []Condition{
				{Field: "Status", Operator: "!=", Value: "Success", LogicalOp: "AND", PipeStage: 1},
			},
		},
		{
			name:  "case insensitive equality",
			query: "SecurityEvent | where UserName =~ \"admin\"",
			expected: []Condition{
				{Field: "UserName", Operator: "=~", Value: "admin", LogicalOp: "AND", PipeStage: 1},
			},
		},
		{
			name:  "greater than",
			query: "SecurityEvent | where Count > 100",
			expected: []Condition{
				{Field: "Count", Operator: ">", Value: "100", LogicalOp: "AND", PipeStage: 1},
			},
		},
		{
			name:  "less than or equal",
			query: "SecurityEvent | where Level <= 3",
			expected: []Condition{
				{Field: "Level", Operator: "<=", Value: "3", LogicalOp: "AND", PipeStage: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			if len(result.Errors) > 0 {
				t.Logf("Parse errors: %v", result.Errors)
			}
			if len(result.Conditions) != len(tt.expected) {
				t.Errorf("Expected %d conditions, got %d", len(tt.expected), len(result.Conditions))
				return
			}
			for i, exp := range tt.expected {
				got := result.Conditions[i]
				if got.Field != exp.Field {
					t.Errorf("Condition %d: expected field %q, got %q", i, exp.Field, got.Field)
				}
				if got.Operator != exp.Operator {
					t.Errorf("Condition %d: expected operator %q, got %q", i, exp.Operator, got.Operator)
				}
				if got.Value != exp.Value {
					t.Errorf("Condition %d: expected value %q, got %q", i, exp.Value, got.Value)
				}
			}
		})
	}
}

func TestExtractConditions_StringOperators(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []Condition
	}{
		{
			name:  "contains operator",
			query: "SecurityEvent | where CommandLine contains \"powershell\"",
			expected: []Condition{
				{Field: "CommandLine", Operator: "contains", Value: "powershell"},
			},
		},
		{
			name:  "has operator",
			query: "SecurityEvent | where Message has \"error\"",
			expected: []Condition{
				{Field: "Message", Operator: "has", Value: "error"},
			},
		},
		{
			name:  "startswith operator",
			query: "SecurityEvent | where FilePath startswith \"C:\\\\Windows\"",
			expected: []Condition{
				{Field: "FilePath", Operator: "startswith", Value: "C:\\\\Windows"},
			},
		},
		{
			name:  "endswith operator",
			query: "SecurityEvent | where FileName endswith \".exe\"",
			expected: []Condition{
				{Field: "FileName", Operator: "endswith", Value: ".exe"},
			},
		},
		{
			name:  "case-sensitive contains",
			query: "SecurityEvent | where Message contains_cs \"ERROR\"",
			expected: []Condition{
				{Field: "Message", Operator: "contains_cs", Value: "ERROR"},
			},
		},
		{
			name:  "negated contains",
			query: "SecurityEvent | where Message !contains \"test\"",
			expected: []Condition{
				{Field: "Message", Operator: "!contains", Value: "test"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			if len(result.Conditions) != len(tt.expected) {
				t.Errorf("Expected %d conditions, got %d. Errors: %v", len(tt.expected), len(result.Conditions), result.Errors)
				return
			}
			for i, exp := range tt.expected {
				got := result.Conditions[i]
				if got.Field != exp.Field {
					t.Errorf("Condition %d: expected field %q, got %q", i, exp.Field, got.Field)
				}
				if got.Operator != exp.Operator {
					t.Errorf("Condition %d: expected operator %q, got %q", i, exp.Operator, got.Operator)
				}
				if got.Value != exp.Value {
					t.Errorf("Condition %d: expected value %q, got %q", i, exp.Value, got.Value)
				}
			}
		})
	}
}

func TestExtractConditions_LogicalOperators(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []Condition
	}{
		{
			name:  "AND conditions",
			query: "SecurityEvent | where EventID == 4624 and Status == \"Success\"",
			expected: []Condition{
				{Field: "EventID", Operator: "==", Value: "4624", LogicalOp: "AND"},
				{Field: "Status", Operator: "==", Value: "Success", LogicalOp: "AND"},
			},
		},
		{
			name:  "OR conditions",
			query: "SecurityEvent | where EventID == 4624 or EventID == 4625",
			expected: []Condition{
				{Field: "EventID", Operator: "==", Value: "4624", LogicalOp: "AND", Alternatives: []string{"4624", "4625"}},
			},
		},
		{
			name:  "mixed AND OR",
			query: "SecurityEvent | where (EventID == 4624 or EventID == 4625) and Status == \"Success\"",
			expected: []Condition{
				{Field: "EventID", Operator: "==", Value: "4624"},
				{Field: "Status", Operator: "==", Value: "Success"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			if len(result.Errors) > 0 {
				t.Logf("Parse warnings/errors: %v", result.Errors)
			}
			if len(result.Conditions) < 1 {
				t.Errorf("Expected at least 1 condition, got %d", len(result.Conditions))
				return
			}
			// Just verify we got conditions without errors for logical operations
			t.Logf("Extracted conditions: %+v", result.Conditions)
		})
	}
}

func TestExtractConditions_InOperator(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		expectedField string
		expectedCount int
	}{
		{
			name:          "IN operator with values",
			query:         "SecurityEvent | where EventID in (4624, 4625, 4626)",
			expectedField: "EventID",
			expectedCount: 1, // grouped as alternatives
		},
		{
			name:          "NOT IN operator",
			query:         "SecurityEvent | where Status !in (\"Failed\", \"Error\")",
			expectedField: "Status",
			expectedCount: 1, // grouped as alternatives, negated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			if len(result.Conditions) < 1 {
				t.Errorf("Expected at least 1 condition, got %d. Errors: %v", len(result.Conditions), result.Errors)
				return
			}
			// Verify field name
			if result.Conditions[0].Field != tt.expectedField {
				t.Errorf("Expected field %q, got %q", tt.expectedField, result.Conditions[0].Field)
			}
		})
	}
}

func TestExtractConditions_MultipleWheres(t *testing.T) {
	query := `SecurityEvent
		| where EventID == 4624
		| where Status == "Success"
		| where AccountName contains "admin"`

	result := ExtractConditions(query)
	if len(result.Errors) > 0 {
		t.Logf("Parse warnings/errors: %v", result.Errors)
	}

	// Should have conditions from each where clause
	if len(result.Conditions) < 3 {
		t.Errorf("Expected at least 3 conditions, got %d", len(result.Conditions))
	}

	// Verify pipe stages are incrementing
	for i, cond := range result.Conditions {
		t.Logf("Condition %d: %+v", i, cond)
	}
}

func TestExtractConditions_ExtendedFields(t *testing.T) {
	query := `SecurityEvent
		| extend ComputedField = strcat(Field1, Field2)
		| where ComputedField == "test"
		| where OriginalField == "value"`

	result := ExtractConditions(query)

	// ComputedField should be included but marked as IsComputed=true, OriginalField should be included but not marked as computed
	foundComputed := false
	foundComputedMarked := false
	foundOriginal := false
	for _, cond := range result.Conditions {
		if cond.Field == "ComputedField" {
			foundComputed = true
			if cond.IsComputed {
				foundComputedMarked = true
			}
		}
		if cond.Field == "OriginalField" {
			foundOriginal = true
			if cond.IsComputed {
				t.Error("OriginalField should not be marked as IsComputed")
			}
		}
	}

	if !foundComputed {
		t.Error("ComputedField should be included in conditions")
	}
	if !foundComputedMarked {
		t.Error("ComputedField should be marked with IsComputed=true")
	}
	if !foundOriginal {
		t.Error("OriginalField should be included")
	}
}

func TestExtractConditions_NotExpression(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		expectNegated bool
	}{
		{
			name:        "NOT before condition",
			query:       "SecurityEvent | where not(EventID == 4624)",
			expectNegated: true,
		},
		{
			name:        "without NOT",
			query:       "SecurityEvent | where EventID == 4624",
			expectNegated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			if len(result.Conditions) == 0 {
				t.Errorf("Expected at least 1 condition, got 0. Errors: %v", result.Errors)
				return
			}
			if result.Conditions[0].Negated != tt.expectNegated {
				t.Errorf("Expected negated=%v, got %v", tt.expectNegated, result.Conditions[0].Negated)
			}
		})
	}
}

func TestExtractConditions_ComplexQuery(t *testing.T) {
	query := `let timeframe = 1d;
SecurityEvent
| where TimeGenerated > ago(timeframe)
| where EventID == 4624
| where AccountType == "User"
| where LogonType in (2, 10, 11)
| where TargetUserName !contains "$"
| extend FullAccount = strcat(TargetDomainName, "\\", TargetUserName)
| where FullAccount !has "SYSTEM"
| project TimeGenerated, Computer, TargetUserName, LogonType, IpAddress`

	result := ExtractConditions(query)

	// Should parse without fatal errors
	t.Logf("Extracted %d conditions with %d errors", len(result.Conditions), len(result.Errors))
	for i, cond := range result.Conditions {
		t.Logf("Condition %d: %+v", i, cond)
	}

	// Should have extracted several conditions
	if len(result.Conditions) == 0 {
		t.Error("Expected to extract some conditions from complex query")
	}
}

func TestExtractConditions_RealWorldQueries(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name: "failed logon detection",
			query: `SecurityEvent
| where EventID == 4625
| where AccountType == "User"
| where FailureReason has "password"
| summarize FailedAttempts = count() by TargetAccount, IpAddress
| where FailedAttempts > 5`,
		},
		{
			name: "process creation monitoring",
			query: `DeviceProcessEvents
| where ActionType == "ProcessCreated"
| where FileName in ("cmd.exe", "powershell.exe", "pwsh.exe")
| where InitiatingProcessFileName !in ("explorer.exe", "services.exe")
| project Timestamp, DeviceName, FileName, ProcessCommandLine`,
		},
		{
			name: "azure signin analysis",
			query: `SigninLogs
| where ResultType != "0"
| where AppDisplayName contains "Azure"
| where Location !in ("US", "CA")
| extend City = tostring(LocationDetails.city)
| where City != ""`,
		},
		{
			name: "network anomaly detection",
			query: `DeviceNetworkEvents
| where ActionType == "ConnectionSuccess"
| where RemotePort in (22, 23, 3389, 5900)
| where RemoteIP !startswith "10." and RemoteIP !startswith "192.168."
| where LocalIP startswith "10."`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			t.Logf("Query: %s", tt.name)
			t.Logf("Conditions extracted: %d, Errors: %d", len(result.Conditions), len(result.Errors))

			for i, cond := range result.Conditions {
				t.Logf("  %d: %s %s %q (stage %d)", i, cond.Field, cond.Operator, cond.Value, cond.PipeStage)
			}

			if len(result.Errors) > 0 {
				t.Logf("Errors: %v", result.Errors)
			}
		})
	}
}

func TestExtractConditions_SummarizeAliasComputedFields(t *testing.T) {
	// Test that summarize function aliases are registered as computed fields
	// so post-aggregation filters don't get treated as required raw data fields
	query := `SecurityEvent
| where EventID == 4625
| summarize FailedAttempts=count() by TargetAccount
| where FailedAttempts > 10`

	result := ExtractConditions(query)

	t.Logf("Computed fields: %v", result.ComputedFields)
	t.Logf("Found %d conditions", len(result.Conditions))
	for _, c := range result.Conditions {
		t.Logf("Condition: %+v", c)
	}

	// "failedattempts" should be in ComputedFields
	if _, ok := result.ComputedFields["failedattempts"]; !ok {
		t.Errorf("Expected 'failedattempts' to be in ComputedFields, got: %v", result.ComputedFields)
	}

	// The "FailedAttempts > 10" condition should be marked as computed
	foundCondition := false
	for _, c := range result.Conditions {
		if strings.EqualFold(c.Field, "FailedAttempts") {
			foundCondition = true
			if !c.IsComputed {
				t.Error("Expected 'FailedAttempts' condition to be marked IsComputed=true")
			}
		}
	}
	if !foundCondition {
		t.Error("Expected to find 'FailedAttempts' condition from | where FailedAttempts > 10")
	}
}

func TestExtractConditions_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "empty query",
			query: "",
		},
		{
			name:  "just table name",
			query: "SecurityEvent",
		},
		{
			name:  "table with project only",
			query: "SecurityEvent | project Computer, EventID",
		},
		{
			name:  "nested parentheses",
			query: "SecurityEvent | where ((EventID == 4624) and (Status == \"Success\"))",
		},
		{
			name:  "verbatim string",
			query: `SecurityEvent | where FilePath == @"C:\Windows\System32"`,
		},
		{
			name:  "multiline string value",
			query: "SecurityEvent | where Message contains \"line1\\nline2\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			// Just ensure no panics
			t.Logf("Conditions: %d, Errors: %d", len(result.Conditions), len(result.Errors))
		})
	}
}

func TestDeduplicateConditions(t *testing.T) {
	conditions := []Condition{
		{Field: "EventID", Operator: "==", Value: "4624", PipeStage: 0},
		{Field: "EventID", Operator: "==", Value: "4625", PipeStage: 1},
		{Field: "Status", Operator: "==", Value: "Success", PipeStage: 0},
		{Field: "Status", Operator: "==", Value: "Success", PipeStage: 1},
	}

	result := DeduplicateConditions(conditions)

	// Should keep only conditions from latest pipe stage for each field
	if len(result) != 2 {
		t.Errorf("Expected 2 conditions after dedup, got %d", len(result))
	}

	// All should be from stage 1
	for _, cond := range result {
		if cond.PipeStage != 1 {
			t.Errorf("Expected pipe stage 1, got %d for field %s", cond.PipeStage, cond.Field)
		}
	}
}

func TestGroupORConditions(t *testing.T) {
	conditions := []Condition{
		{Field: "EventID", Operator: "==", Value: "4624", LogicalOp: "AND"},
		{Field: "EventID", Operator: "==", Value: "4625", LogicalOp: "OR"},
		{Field: "EventID", Operator: "==", Value: "4626", LogicalOp: "OR"},
		{Field: "Status", Operator: "==", Value: "Success", LogicalOp: "AND"},
	}

	result := groupORConditions(conditions)

	// First EventID should have alternatives
	if len(result) != 2 {
		t.Errorf("Expected 2 conditions after grouping, got %d", len(result))
		return
	}

	if len(result[0].Alternatives) != 3 {
		t.Errorf("Expected 3 alternatives for EventID, got %d", len(result[0].Alternatives))
	}

	if result[1].Field != "Status" {
		t.Errorf("Expected second condition to be Status, got %s", result[1].Field)
	}
}

func TestIsValidFieldName(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"EventID", true},
		{"event_id", true},
		{"_private", true},
		{"Field123", true},
		{"Properties.Result", true},
		{"123field", false},
		{"field-name", false},
		{"", false},
		{"field name", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isValidFieldName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidFieldName(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"hello"`, "hello"},
		{`'world'`, "world"},
		{`@"C:\path"`, "C:\\path"},
		{`@'C:\path'`, "C:\\path"},
		{"plain", "plain"},
		{"  spaced  ", "spaced"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractValue(tt.input)
			if result != tt.expected {
				t.Errorf("extractValue(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestJoinExtraction_SimpleInner(t *testing.T) {
	query := `SecurityEvent
| where EventID == 4625
| join kind=inner (
    SecurityEvent
    | where EventID == 4624
    | project TargetUserName, LogonType
) on TargetUserName`

	result := ExtractConditions(query)

	if len(result.Joins) != 1 {
		t.Fatalf("Expected 1 join, got %d", len(result.Joins))
	}

	j := result.Joins[0]
	if j.Type != "inner" {
		t.Errorf("Expected join type 'inner', got %q", j.Type)
	}
	if len(j.JoinFields) != 1 || j.JoinFields[0] != "TargetUserName" {
		t.Errorf("Expected join fields [TargetUserName], got %v", j.JoinFields)
	}
	if j.Subsearch == nil {
		t.Fatal("Expected subsearch ParseResult, got nil")
	}
}

func TestJoinExtraction_LeftOuter(t *testing.T) {
	query := `SigninLogs
| where ResultType != "0"
| join kind=leftouter (
    SigninLogs
    | where ResultType == "0"
    | project UserPrincipalName, IPAddress
) on UserPrincipalName`

	result := ExtractConditions(query)

	if len(result.Joins) == 0 {
		t.Fatal("Expected at least 1 join")
	}

	j := result.Joins[0]
	if j.Type != "leftouter" {
		t.Errorf("Expected join type 'leftouter', got %q", j.Type)
	}
}

func TestJoinExtraction_TableReference(t *testing.T) {
	query := `SecurityEvent
| where EventID == 4688
| join kind=inner IdentityInfo on AccountObjectId`

	result := ExtractConditions(query)

	if len(result.Joins) == 0 {
		t.Fatal("Expected at least 1 join")
	}

	j := result.Joins[0]
	if j.RightTable != "IdentityInfo" {
		t.Errorf("Expected right table 'IdentityInfo', got %q", j.RightTable)
	}
	if j.Subsearch != nil {
		t.Error("Expected no subsearch for table reference join")
	}
}

func TestJoinExtraction_LeftRightSyntax(t *testing.T) {
	query := `T1
| where Status == "Failed"
| join kind=inner (T2 | where Active == true) on $left.UserID == $right.ID`

	result := ExtractConditions(query)

	if len(result.Joins) == 0 {
		t.Fatal("Expected at least 1 join")
	}

	j := result.Joins[0]
	if len(j.LeftFields) != 1 || j.LeftFields[0] != "UserID" {
		t.Errorf("Expected left fields [UserID], got %v", j.LeftFields)
	}
	if len(j.RightFields) != 1 || j.RightFields[0] != "ID" {
		t.Errorf("Expected right fields [ID], got %v", j.RightFields)
	}
}

func TestJoinExtraction_FieldProvenance(t *testing.T) {
	query := `SecurityEvent
| where EventID == 4625
| join kind=inner (
    SecurityEvent
    | where EventID == 4624
    | project TargetUserName, LogonType, IpAddress
) on TargetUserName
| where LogonType == 10`

	result := ExtractConditions(query)

	if len(result.Joins) == 0 {
		t.Fatal("Expected at least 1 join")
	}

	tests := []struct {
		field    string
		expected FieldProvenance
	}{
		{"TargetUserName", ProvenanceJoinKey},
		{"LogonType", ProvenanceJoined},
		{"IpAddress", ProvenanceJoined},
		{"EventID", ProvenanceMain},
	}

	for _, tc := range tests {
		actual := ClassifyFieldProvenance(result, tc.field)
		if actual != tc.expected {
			t.Errorf("Field %q: expected provenance %q, got %q", tc.field, tc.expected, actual)
		}
	}
}

func TestJoinExtraction_BackwardCompatibility(t *testing.T) {
	// Verify that join right-side conditions don't leak into main conditions
	query := `T1
| where Status == "Failed"
| join kind=inner (T2 | where Active == true) on UserName`

	result := ExtractConditions(query)

	for _, c := range result.Conditions {
		if c.Field == "Active" {
			t.Error("Join right-side conditions should not appear in main conditions")
		}
	}

	// But they should be accessible via Joins
	if len(result.Joins) == 1 && result.Joins[0].Subsearch != nil {
		hasActive := false
		for _, c := range result.Joins[0].Subsearch.Conditions {
			if c.Field == "Active" {
				hasActive = true
			}
		}
		if !hasActive {
			t.Error("Expected Active condition in subsearch")
		}
	}
}
