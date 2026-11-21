package kql

import (
	"reflect"
	"testing"
)

// A table in another scope is written workspace("w").T, app("a").T,
// database("d").T or cluster("c").database("d").T, or named by table("T")
// (Microsoft Learn: "Query across resources with Azure Monitor",
// "Cross-cluster and cross-database queries", "table()"). As the query's
// source it is that table.
func TestExtractConditions_QualifiedTableSource(t *testing.T) {
	tests := []struct {
		query, field, value, table string
	}{
		{`workspace("00000000-0000-0000-0000-000000000000").Update | where UpdateState == "Needed"`, "UpdateState", "Needed", "Update"},
		{`app("00000000-0000-0000-0000-000000000000").requests | where success == false`, "success", "false", "requests"},
		{`cluster('help').database('Samples').StormEvents | where State == "TEXAS"`, "State", "TEXAS", "StormEvents"},
		{`database('Samples').StormEvents | where State == "TEXAS"`, "State", "TEXAS", "StormEvents"},
		{`table('StormEvents') | where State == "TEXAS"`, "State", "TEXAS", "StormEvents"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			if len(result.Errors) != 0 || result.Expression == nil || len(result.Conditions) != 1 || result.Conditions[0].Field != tt.field || result.Conditions[0].Value != tt.value {
				t.Fatalf("conditions=%+v expression=%+v errors=%q; want [%s == %s] and no error", result.Conditions, result.Expression, result.Errors, tt.field, tt.value)
			}
			if !reflect.DeepEqual(result.DataSources, []string{tt.table}) {
				t.Fatalf("data_sources = %q, want [%s]", result.DataSources, tt.table)
			}
		})
	}
}

// The same qualified sources inside a join's right side and a union.
func TestExtractConditions_QualifiedTableInJoinAndUnion(t *testing.T) {
	join := ExtractConditions(`SecurityEvent | where EventID == 4624 | join kind=inner (workspace("w").SigninLogs | where ResultType == 0) on Account`)
	if len(join.Errors) != 0 || len(join.Conditions) != 1 || len(join.Joins) != 1 || join.Joins[0].Subsearch == nil {
		t.Fatalf("join: conditions=%+v joins=%+v errors=%q", join.Conditions, join.Joins, join.Errors)
	}
	sub := join.Joins[0].Subsearch
	if len(sub.Errors) != 0 || len(sub.Conditions) != 1 || sub.Conditions[0].Field != "ResultType" || !reflect.DeepEqual(sub.DataSources, []string{"SigninLogs"}) || !reflect.DeepEqual(join.DataSources, []string{"SecurityEvent", "SigninLogs"}) {
		t.Fatalf("join: subsearch=%+v data_sources=%q", sub, join.DataSources)
	}
	union := ExtractConditions("union \n(workspace(\"w\").Heartbeat | where Computer == \"myComputer\"),\n(app(\"a\").requests | where cloud_RoleInstance == \"myRoleInstance\")\n| count")
	if len(union.Errors) != 0 || len(union.Conditions) != 2 || !reflect.DeepEqual(union.DataSources, []string{"Heartbeat", "requests"}) {
		t.Fatalf("union: conditions=%+v data_sources=%q errors=%q", union.Conditions, union.DataSources, union.Errors)
	}
}
