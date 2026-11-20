package kql

import (
	"reflect"
	"testing"
)

// The fields a join's right side exposes are collected from projections,
// conditions, computed columns and join keys. Identical calls must return
// them in the same order in both public APIs.
func TestJoinExposedFieldsOrderIsDeterministic(t *testing.T) {
	query := `SecurityEvent | join kind=inner (Heartbeat | where OSType == "Linux" | extend Up = 1 | project Computer, OSType, Version, Up) on Computer`
	want := []string{"Computer", "OSType", "Up", "Version"}
	for i := 0; i < 50; i++ {
		result := ExtractConditions(query)
		if len(result.Joins) != 1 || !reflect.DeepEqual(result.Joins[0].ExposedFields, want) {
			t.Fatalf("call %d: ExtractConditions joins = %+v, want ExposedFields %q", i, result.Joins, want)
		}
		semantic := AnalyzeQuery(query)
		if len(semantic.Joins) != 1 || !reflect.DeepEqual(semantic.Joins[0].ExposedFields, want) {
			t.Fatalf("call %d: AnalyzeQuery joins = %+v, want ExposedFields %q", i, semantic.Joins, want)
		}
	}
}
