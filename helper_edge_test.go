package kql

import "testing"

func TestHelperNilInputsAreSafe(t *testing.T) {
	if IsStatisticalQuery(nil) {
		t.Fatal("nil parse result should not be statistical")
	}
	if HasUnmappedComputedFields(nil) {
		t.Fatal("nil parse result should not have unmapped computed fields")
	}
	if HasComplexWhereConditions(nil) {
		t.Fatal("nil parse result should not have complex where conditions")
	}
	if got := GetEventTypeFromConditions(nil); got != "" {
		t.Fatalf("nil parse result event type = %q, want empty", got)
	}
}

func TestKQLHelperClassificationEdges(t *testing.T) {
	statistical := &ParseResult{Commands: []string{"where", "summarize"}}
	if !IsStatisticalQuery(statistical) {
		t.Fatal("summarize command should be classified as statistical")
	}
	if IsStatisticalQuery(&ParseResult{Commands: []string{"where", "project"}}) {
		t.Fatal("non-aggregation commands should not be classified as statistical")
	}

	unmapped := &ParseResult{Conditions: []Condition{
		{Field: "derived", IsComputed: true},
	}}
	if !HasUnmappedComputedFields(unmapped) {
		t.Fatal("computed condition without source field should be unmapped")
	}
	mapped := &ParseResult{Conditions: []Condition{
		{Field: "derived", IsComputed: true, SourceField: "EventID"},
	}}
	if HasUnmappedComputedFields(mapped) {
		t.Fatal("computed condition with source field should be mapped")
	}

	complex := &ParseResult{
		Commands: []string{"where"},
		Conditions: []Condition{
			{Field: "RemoteIP", Operator: "ipv4_is_in_range", Value: "10.0.0.0/8", PipeStage: 1},
		},
	}
	if !HasComplexWhereConditions(complex) {
		t.Fatal("CIDR-style where operator should be complex")
	}
	negatedWhere := &ParseResult{
		Commands: []string{"where"},
		Conditions: []Condition{
			{Field: "AccountName", Operator: "==", Value: "SYSTEM", Negated: true, PipeStage: 1},
		},
	}
	if !HasComplexWhereConditions(negatedWhere) {
		t.Fatal("negated where condition should be complex")
	}
	notWhere := &ParseResult{
		Commands: []string{"project"},
		Conditions: []Condition{
			{Field: "CommandLine", Operator: "matches", Value: ".*", PipeStage: 1},
		},
	}
	if HasComplexWhereConditions(notWhere) {
		t.Fatal("complex operator outside a where command should not be complex where")
	}
	simpleWhere := &ParseResult{
		Commands: []string{"where"},
		Conditions: []Condition{
			{Field: "EventID", Operator: "==", Value: "4688", PipeStage: 1},
		},
	}
	if HasComplexWhereConditions(simpleWhere) {
		t.Fatal("simple where equality should not be complex")
	}

	eventTypes := map[string]string{
		"4688": "windows_4688",
		"4624": "windows_4624",
		"4625": "windows_4625",
		"1":    "sysmon_1",
		"3":    "sysmon_3",
	}
	for eventID, want := range eventTypes {
		got := GetEventTypeFromConditions(&ParseResult{Conditions: []Condition{
			{Field: "EventID", Operator: "==", Value: eventID},
		}})
		if got != want {
			t.Fatalf("EventID %s mapped to %q, want %q", eventID, got, want)
		}
	}
	if got := GetEventTypeFromConditions(&ParseResult{Conditions: []Condition{
		{Field: "EventCode", Operator: "==", Value: "4688"},
	}}); got != "" {
		t.Fatalf("KQL EventCode should not map event type, got %q", got)
	}
	if got := GetEventTypeFromConditions(&ParseResult{Conditions: []Condition{
		{Field: "EventID", Operator: "==", Value: "9999"},
	}}); got != "" {
		t.Fatalf("unknown EventID should not map event type, got %q", got)
	}
}
