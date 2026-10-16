package kql

import "testing"

func TestExtractConditions_BlankLinesInsideSubqueriesKeepTheQuery(t *testing.T) {
	result := ExtractConditions(`union
(

    DeviceProcessEvents
    | where FileName =~ "WerFaultSecure.exe"
),
(

    SecurityEvent
    | where EventID == 4688
)
| where isnotempty(CommandLine)`)

	for _, msg := range result.Errors {
		if msg != portablePredicateExtractionNote && msg != portableKeywordExtractionNote {
			t.Fatalf("unexpected syntax error %q", msg)
		}
	}
	for _, field := range []string{"FileName", "EventID"} {
		found := false
		for _, condition := range result.Conditions {
			found = found || condition.Field == field
		}
		if !found {
			t.Errorf("condition on %s missing: %+v", field, result.Conditions)
		}
	}
}

func TestExtractConditions_SecondTopLevelQueryIsIgnored(t *testing.T) {
	result := ExtractConditions(`SecurityEvent
| where EventID == 4625


SigninLogs
| where ResultType != 0`)

	for _, condition := range result.Conditions {
		if condition.Field == "ResultType" {
			t.Fatalf("condition from the second query extracted: %+v", result.Conditions)
		}
	}
}
