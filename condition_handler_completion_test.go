package kql

import "testing"

func TestKQLMultiValueHandlersSkipKeywordsAndEmptyLists(t *testing.T) {
	extractor := &conditionExtractor{
		computedFields:      map[string]string{},
		computedExpressions: map[string]string{},
		lastLogicalOp:       "AND",
	}
	values := kqlExpressionListForTest(t, `"alice", "bob"`)

	extractor.handleInOperator("where", values, "in", false)
	extractor.handleInOperator("Account", nil, "in", false)
	extractor.handleHasAnyAllOperator("project", "has_any", values)

	if len(extractor.conditions) != 0 {
		t.Fatalf("keyword/empty multi-value handlers emitted conditions: %#v", extractor.conditions)
	}
}

func TestKQLMultiValueHandlersPreserveComputedMetadata(t *testing.T) {
	extractor := &conditionExtractor{
		computedFields:      map[string]string{"derived": "RawField"},
		computedExpressions: map[string]string{},
		lastLogicalOp:       "OR",
		currentStage:        3,
	}

	extractor.handleInOperator("derived", kqlExpressionListForTest(t, `"a"`), "in", true)
	inCond := requireKQLCondition(t, extractor.conditions, "derived", "in", "a", true)
	if !inCond.IsComputed || inCond.SourceField != "RawField" || inCond.LogicalOp != "OR" || inCond.PipeStage != 3 {
		t.Fatalf("computed IN metadata not preserved: %#v", inCond)
	}

	extractor.handleHasAnyAllOperator("derived", "has_any", kqlExpressionListForTest(t, `"b", "c"`))
	first := requireKQLCondition(t, extractor.conditions, "derived", "has", "b", false)
	second := requireKQLCondition(t, extractor.conditions, "derived", "has", "c", false)
	if !first.IsComputed || first.SourceField != "RawField" || first.PipeStage != 3 {
		t.Fatalf("computed has_any metadata not preserved: %#v", first)
	}
	if second.LogicalOp != "OR" {
		t.Fatalf("has_any connector = %q, want OR", second.LogicalOp)
	}
}

func TestKQLCaseInsensitiveNotEqualOperatorPreserved(t *testing.T) {
	extractor := &conditionExtractor{
		computedFields:      map[string]string{},
		computedExpressions: map[string]string{},
		lastLogicalOp:       "AND",
	}

	extractor.handleComparison("Account", "!~", `"admin"`)
	condition := requireKQLCondition(t, extractor.conditions, "Account", "!~", "admin", false)
	if condition.ValueReference != "" {
		t.Fatalf("literal !~ comparison should not become field reference: %#v", condition)
	}
}
