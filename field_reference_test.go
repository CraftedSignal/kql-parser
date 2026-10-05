package kql

import "testing"

func TestExtractConditions_FieldReferenceComparison(t *testing.T) {
	result := ExtractConditions(`SecurityEvent | where NewProcessName == ParentProcessName`)
	if len(result.Errors) > 0 {
		t.Fatalf("Unexpected errors: %v", result.Errors)
	}

	if len(result.Conditions) != 1 {
		t.Fatalf("expected one condition, got %+v", result.Conditions)
	}
	condition := result.Conditions[0]
	if condition.Field != "NewProcessName" || condition.ValueReference != "ParentProcessName" || condition.Value != "" {
		t.Fatalf("expected NewProcessName field reference to ParentProcessName, got %+v", condition)
	}
}

func TestExtractConditions_BooleanLiteralIsNotFieldReference(t *testing.T) {
	result := ExtractConditions(`SecurityEvent | where Enabled == true`)
	if len(result.Errors) > 0 {
		t.Fatalf("Unexpected errors: %v", result.Errors)
	}

	if len(result.Conditions) != 1 {
		t.Fatalf("expected one condition, got %+v", result.Conditions)
	}
	condition := result.Conditions[0]
	if condition.Value != "true" || condition.ValueReference != "" {
		t.Fatalf("expected true literal, got %+v", condition)
	}
}
