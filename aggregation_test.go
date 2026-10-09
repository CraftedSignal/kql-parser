package kql

import (
	"reflect"
	"testing"
)

func TestExtractConditions_SummarizeAggregations(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []Aggregation
	}{
		{
			name:     "default count column",
			query:    `SigninLogs | where ResultType != 0 | summarize count() by UserPrincipalName | where count_ > 5`,
			expected: []Aggregation{{Function: "count", Column: "count_"}},
		},
		{
			name:     "assigned alias",
			query:    `SigninLogs | summarize Failed = count() by UserPrincipalName | where Failed > 5`,
			expected: []Aggregation{{Function: "count", Column: "Failed"}},
		},
		{
			name:     "as alias",
			query:    `SigninLogs | summarize count() as Failed by UserPrincipalName`,
			expected: []Aggregation{{Function: "count", Column: "Failed"}},
		},
		{
			name:     "default column of an aggregated field",
			query:    `SigninLogs | summarize dcount(AppDisplayName) by UserPrincipalName | where dcount_AppDisplayName > 3`,
			expected: []Aggregation{{Function: "dcount", Field: "AppDisplayName", Column: "dcount_AppDisplayName"}},
		},
		{
			name:  "several aggregations",
			query: `CommonSecurityLog | summarize Total = sum(SentBytes), avg(ReceivedBytes), make_set(DestinationIP) by SourceIP`,
			expected: []Aggregation{
				{Function: "sum", Field: "SentBytes", Column: "Total"},
				{Function: "avg", Field: "ReceivedBytes", Column: "avg_ReceivedBytes"},
				{Function: "make_set", Field: "DestinationIP", Column: "set_DestinationIP"},
			},
		},
		{
			name:     "predicate argument",
			query:    `SigninLogs | summarize countif(ResultType != 0) by UserPrincipalName`,
			expected: []Aggregation{{Function: "countif", Column: "countif_"}},
		},
		{
			name:     "join subquery aggregation belongs to the subquery",
			query:    `SigninLogs | join kind=inner (AuditLogs | summarize count() by UserPrincipalName) on UserPrincipalName`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			got := make([]Aggregation, len(result.Aggregations))
			for i, aggregation := range result.Aggregations {
				aggregation.PipeStage = 0
				got[i] = aggregation
			}
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("aggregations = %+v, want %+v", result.Aggregations, tt.expected)
			}
		})
	}
}

func TestExtractConditions_SummarizeAggregationPipeStage(t *testing.T) {
	result := ExtractConditions(`SigninLogs
| where ResultType != 0
| summarize count() by UserPrincipalName
| where count_ > 5`)
	if len(result.Aggregations) != 1 {
		t.Fatalf("expected one aggregation, got %+v", result.Aggregations)
	}
	stage := result.Aggregations[0].PipeStage
	for _, condition := range result.Conditions {
		switch condition.Field {
		case "ResultType":
			if condition.PipeStage >= stage {
				t.Errorf("filter before summarize has stage %d, aggregation stage %d", condition.PipeStage, stage)
			}
		case "count_":
			if condition.PipeStage <= stage {
				t.Errorf("filter after summarize has stage %d, aggregation stage %d", condition.PipeStage, stage)
			}
		}
	}
}

func TestExtractConditions_GroupByExpressions(t *testing.T) {
	result := ExtractConditions(`SigninLogs | summarize count() by UserPrincipalName, bin(TimeGenerated, 1h) | where count_ > 5`)
	if !reflect.DeepEqual(result.GroupByFields, []string{"UserPrincipalName"}) {
		t.Fatalf("group-by fields = %v, want only the plain column", result.GroupByFields)
	}
	if !reflect.DeepEqual(result.GroupByExpressions, []string{"bin(TimeGenerated,1h)"}) {
		t.Fatalf("group-by expressions = %v, want the bin expression", result.GroupByExpressions)
	}
}

func TestExtractConditions_RenamedColumnsAreComputed(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "project-rename", query: `SigninLogs | where User == "before" | project-rename User = UserPrincipalName | where User == "admin@contoso.com"`},
		{name: "project alias", query: `SigninLogs | where User == "before" | project User = UserPrincipalName, ResultType | where User == "admin@contoso.com"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)
			if got := result.ComputedExpressions["user"]; got != "UserPrincipalName" {
				t.Fatalf("computed expression = %q, want UserPrincipalName", got)
			}
			var before, after *Condition
			for i := range result.Conditions {
				switch result.Conditions[i].Value {
				case "before":
					before = &result.Conditions[i]
				case "admin@contoso.com":
					after = &result.Conditions[i]
				}
			}
			if before == nil || before.IsComputed {
				t.Fatalf("filter before the rename should be on the raw column, got %+v", before)
			}
			if after == nil || !after.IsComputed || after.SourceField != "UserPrincipalName" {
				t.Fatalf("filter after the rename should be computed from UserPrincipalName, got %+v", after)
			}
		})
	}
}

func TestExtractConditions_Pipelines(t *testing.T) {
	result := ExtractConditions(`SecurityEvent
| where EventID == 4625
| union (WindowsEvent | where EventID == 4624 | summarize count() by Computer), AuditLogs
| where count_ > 5`)

	byValue := map[string]Condition{}
	for _, condition := range result.Conditions {
		byValue[condition.Field+condition.Value] = condition
	}
	query := byValue["EventID4625"].Pipeline
	if query < 0 || query >= len(result.Pipelines) {
		t.Fatalf("query pipeline %d out of range: %+v", query, result.Pipelines)
	}
	if got := result.Pipelines[query]; got.Parent != -1 || got.Operator != "" || got.Operators != 3 {
		t.Fatalf("query pipeline = %+v, want a top-level pipeline with three operators", got)
	}
	if got := byValue["count_5"].Pipeline; got != query {
		t.Fatalf("filter after the union is in pipeline %d, want %d", got, query)
	}

	var unionInputs []Pipeline
	for _, pipeline := range result.Pipelines {
		if pipeline.Operator == "union" {
			unionInputs = append(unionInputs, pipeline)
		}
	}
	if len(unionInputs) != 2 {
		t.Fatalf("expected the two union inputs, got %+v", result.Pipelines)
	}
	for _, input := range unionInputs {
		if input.Parent != query || input.PipeStage != byValue["EventID4625"].PipeStage+1 {
			t.Fatalf("union input %+v is not nested at the union operator of pipeline %d", input, query)
		}
	}
	if unionInputs[0].Operators != 2 || unionInputs[1].Operators != 0 {
		t.Fatalf("union inputs = %+v, want the filtered pipeline and the bare table", unionInputs)
	}
	if len(result.Aggregations) != 1 || result.Pipelines[result.Aggregations[0].Pipeline].Operator != "union" {
		t.Fatalf("aggregation should belong to the union input: %+v", result.Aggregations)
	}
}

func TestExtractConditions_InSubqueryUsesPlaceholder(t *testing.T) {
	result := ExtractConditions(`SecurityEvent | where Account in ((SigninLogs | where ResultType != 0 | project UserPrincipalName))`)
	for _, condition := range result.Conditions {
		if condition.Field == "Account" {
			if condition.Value != SubqueryPlaceholder {
				t.Fatalf("membership in a subquery = %+v, want value %q", condition, SubqueryPlaceholder)
			}
			return
		}
	}
	t.Fatalf("membership condition missing: %+v", result.Conditions)
}

func TestExtractConditions_PipelineSources(t *testing.T) {
	result := ExtractConditions(`Failures
| where AppDisplayName == "Azure Portal"
| union (SecurityEvent | where EventID == 4625), AuditLogs, (table("SigninLogs") | where ResultType != 0)`)
	var sources []string
	for _, pipeline := range result.Pipelines {
		sources = append(sources, pipeline.Source)
	}
	if !reflect.DeepEqual(sources, []string{"Failures", "SecurityEvent", "AuditLogs", ""}) {
		t.Fatalf("pipeline sources = %q, want the table each reads by name", sources)
	}
}

func TestExtractConditions_InlinesLetConstants(t *testing.T) {
	result := ExtractConditions(`let allowed = dynamic(['10.0.0.1', '10.0.0.2']);
let app = "Azure \"Portal\"";
let threshold = 5;
let tools = dynamic(["cmd.exe", "powershell.exe"]);
let failed = SigninLogs | where ResultType != 0 | project Account;
SigninLogs
| where AppDisplayName == app and IPAddress !in (allowed) and Account in (failed)
| where CommandLine has_all (tools)
| summarize count() by UserPrincipalName
| where count_ > threshold`)

	byField := map[string][]Condition{}
	for _, condition := range result.Conditions {
		byField[condition.Field] = append(byField[condition.Field], condition)
	}
	if got := byField["AppDisplayName"]; len(got) != 1 || got[0].Value != `Azure \"Portal\"` || got[0].ValueReference != "" {
		t.Errorf("scalar let = %+v, want the literal it binds", got)
	}
	if got := byField["IPAddress"]; len(got) != 1 || !got[0].Negated || !reflect.DeepEqual(got[0].Alternatives, []string{"10.0.0.1", "10.0.0.2"}) || got[0].ValueReference != "" {
		t.Errorf("list let in !in = %+v, want its values", got)
	}
	if got := byField["Account"]; len(got) != 1 || got[0].ValueReference != "failed" {
		t.Errorf("tabular let = %+v, want the reference kept", got)
	}
	if got := byField["CommandLine"]; len(got) != 2 || got[0].Value != "cmd.exe" || got[1].Value != "powershell.exe" || got[1].LogicalOp != "AND" {
		t.Errorf("list let in has_all = %+v, want one has per value, all required", got)
	}
	if got := byField["count_"]; len(got) != 1 || got[0].Value != "5" || got[0].ValueReference != "" {
		t.Errorf("scalar let threshold = %+v, want its value", got)
	}
	if len(result.LetStatements) != 5 {
		t.Errorf("let statements = %+v, want all five reported", result.LetStatements)
	}
}
