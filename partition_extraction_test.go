package kql

import (
	"reflect"
	"testing"
)

// normalizePartitionOperator rewrites a partition stage so the query parses.
// The rewrite must stay owned by the authored partition stage: the filters
// around it are extracted and the stage is an opaque dataflow step.
func TestExtractConditions_PartitionKeepsSurroundingFilters(t *testing.T) {
	tests := []struct {
		name, query, partition string
		fields                 []string
	}{
		{
			name:      "operator body",
			query:     `SecurityEvent | where EventID == 4688 | partition by Computer (top 1 by TimeGenerated)`,
			partition: `partition by Computer (top 1 by TimeGenerated)`,
			fields:    []string{"EventID"},
		},
		{
			name:      "fuzz corpus query",
			query:     `SecurityEvent | partition by Computer (summarize count())`,
			partition: `partition by Computer (summarize count())`,
		},
		{
			name:      "filter after the partition",
			query:     `SecurityEvent | where EventID == 4688 | partition by Computer (top 1 by TimeGenerated) | where Account == "admin"`,
			partition: `partition by Computer (top 1 by TimeGenerated)`,
			fields:    []string{"EventID", "Account"},
		},
		{
			name: "Microsoft Learn example with a hint and a piped body",
			query: "SecurityEvent\n| where Computer startswith 'W'\n| partition hint.strategy=native by Computer\n    (\n" +
				"    summarize Events=count(), Accounts=dcount(Account) by EventID, Computer\n    | top 3 by Events\n    )",
			partition: "partition hint.strategy=native by Computer\n    (\n" +
				"    summarize Events=count(), Accounts=dcount(Account) by EventID, Computer\n    | top 3 by Events\n    )",
			fields: []string{"Computer"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			var fields []string
			for _, condition := range result.Conditions {
				fields = append(fields, condition.Field)
			}
			if len(result.Errors) != 0 || !reflect.DeepEqual(fields, tt.fields) || len(result.DataSources) == 0 || result.DataSources[0] != "SecurityEvent" {
				t.Fatalf("fields=%q data_sources=%q errors=%q; want fields %q, SecurityEvent first, no error", fields, result.DataSources, result.Errors, tt.fields)
			}
			if result.Dataflow == nil || result.Dataflow.Complete || len(result.Aggregations) != 0 {
				t.Fatalf("dataflow=%+v aggregations=%+v; want an incomplete graph and no synthetic aggregation", result.Dataflow, result.Aggregations)
			}
			var partition *DataflowStage
			for i := range result.Dataflow.Stages {
				if result.Dataflow.Stages[i].Command == "partition" {
					partition = &result.Dataflow.Stages[i]
				}
			}
			if partition == nil || partition.Source != tt.partition || partition.Effect != "opaque" {
				t.Fatalf("stages = %+v; want an opaque partition stage with its authored source", result.Dataflow.Stages)
			}
		})
	}
}
