package kql

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDataflowSemanticCopyIsolatesGraphAndNestedRefs(t *testing.T) {
	parsed := ExtractConditions(`T | extend y=x | where x==1 or x==2`)
	if len(parsed.Errors) != 0 || len(parsed.Conditions) != 1 || parsed.Dataflow == nil || len(parsed.Dataflow.Bindings) != 1 || parsed.Conditions[0].FieldBinding == nil || parsed.Conditions[0].RHS == nil || len(parsed.Conditions[0].RHS.Items) != 2 {
		t.Fatalf("fixture lost graph or nested condition: %+v", parsed)
	}
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	copy := semanticFromExtraction(parsed, budget)
	if copy == nil || len(copy.Errors) != 0 || copy.Dataflow == nil || len(copy.Conditions) != 1 || copy.Expression == nil || copy.Expression.Condition == nil {
		t.Fatalf("semantic copy failed: %+v", copy)
	}
	copy.Dataflow.Scopes[0].ID = "mutated"
	copy.Dataflow.Stages[0].Inputs[0].ScopeID = "mutated"
	copy.Dataflow.Scopes[1].BindingIDs[0] = "mutated"
	copy.Dataflow.Stages[0].BindingIDs[0] = "mutated"
	copy.Dataflow.Bindings[0].Expression = "mutated"
	copy.Conditions[0].FieldBinding.ScopeID = "mutated"
	if copy.Expression.Condition.FieldBinding.ScopeID == "mutated" {
		t.Fatal("semantic condition occurrences share a field reference")
	}
	copy.Conditions[0].RHS.Items[0].Value = "mutated"
	copy.Expression.Condition.FieldBinding.ScopeID = "mutated"
	copy.Expression.Condition.RHS.Items[1].Value = "mutated"
	if copy.Conditions[0].RHS.Items[1].Value == "mutated" {
		t.Fatal("semantic condition occurrences share list items")
	}
	if parsed.Dataflow.Scopes[0].ID == "mutated" || parsed.Dataflow.Scopes[1].BindingIDs[0] == "mutated" || parsed.Dataflow.Stages[0].Inputs[0].ScopeID == "mutated" || parsed.Dataflow.Stages[0].BindingIDs[0] == "mutated" || parsed.Dataflow.Bindings[0].Expression == "mutated" || parsed.Conditions[0].FieldBinding.ScopeID == "mutated" || parsed.Conditions[0].RHS.Items[0].Value == "mutated" || parsed.Conditions[0].RHS.Items[1].Value == "mutated" {
		t.Fatalf("semantic copy aliases extraction: %+v", parsed)
	}
}

func TestDataflowConditionGroupRequiresExactReferenceIdentity(t *testing.T) {
	a := Condition{Field: "x", Operator: "==", StageID: "t0", InputScopeID: "s0", FieldBinding: &FieldBindingRef{ScopeID: "s0", Kind: "binding", BindingID: "b0"}, RHS: &Operand{Kind: OperandNumber, Value: "1"}}
	b := a
	b.RHS = &Operand{Kind: OperandNumber, Value: "2"}
	b.FieldBinding = &FieldBindingRef{ScopeID: "s0", Kind: "binding", BindingID: "b0"}
	if !sameConditionGroup(a, b) {
		t.Fatal("same-scope, same-binding OR alternatives were split")
	}
	for _, ref := range []*FieldBindingRef{{ScopeID: "s0", Kind: "binding", BindingID: "b1"}, {ScopeID: "s1", Kind: "binding", BindingID: "b0"}, nil} {
		b.FieldBinding = ref
		if sameConditionGroup(a, b) {
			t.Fatalf("different binding identity merged: %+v", ref)
		}
	}
}

func TestDataflowSemanticCopyIsolatesTypedFieldOperandRef(t *testing.T) {
	parsed := ExtractConditions(`T | where x==y`)
	if len(parsed.Errors) != 0 || len(parsed.Conditions) != 1 || parsed.Conditions[0].RHS == nil || parsed.Conditions[0].RHS.FieldBinding == nil {
		t.Fatalf("typed field reference missing: %+v", parsed)
	}
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	copy := semanticFromExtraction(parsed, budget)
	if copy == nil || len(copy.Conditions) != 1 || copy.Expression == nil || copy.Expression.Condition == nil || copy.Expression.Condition.RHS == nil || copy.Expression.Condition.RHS.FieldBinding == nil {
		t.Fatalf("semantic field reference missing: %+v", copy)
	}
	copy.Conditions[0].RHS.FieldBinding.ScopeID = "mutated"
	if copy.Expression.Condition.RHS.FieldBinding.ScopeID == "mutated" {
		t.Fatal("semantic operand occurrences share a field reference")
	}
	copy.Expression.Condition.RHS.FieldBinding.ScopeID = "mutated"
	if parsed.Conditions[0].RHS.FieldBinding.ScopeID == "mutated" || copy.Conditions[0].RHS.FieldBinding.ScopeID != "mutated" {
		t.Fatal("field operand reference aliases extraction")
	}
}

func TestDataflowCloneListItemFieldBindingPointer(t *testing.T) {
	// This directly exercises the recursive operand-copy seam. A field-valued
	// list is retained as scalar by the current public syntax path, but the
	// public Operand model and clone must still own every nested reference.
	parsed := ExtractConditions(`T | where x==y`)
	if len(parsed.Errors) != 0 || len(parsed.Conditions) != 1 || parsed.Conditions[0].RHS == nil || parsed.Conditions[0].RHS.FieldBinding == nil {
		t.Fatalf("field operand fixture missing: %+v", parsed)
	}
	parsed.Conditions[0].RHS = &Operand{Kind: OperandList, Items: []Operand{*parsed.Conditions[0].RHS}}
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	semantic := semanticFromExtraction(parsed, budget)
	if semantic == nil || len(semantic.Conditions) != 1 || semantic.Expression == nil || semantic.Expression.Condition == nil || semantic.Conditions[0].RHS == nil || semantic.Expression.Condition.RHS == nil || len(semantic.Conditions[0].RHS.Items) != 1 || len(semantic.Expression.Condition.RHS.Items) != 1 {
		t.Fatalf("semantic field-valued list missing: %+v", semantic)
	}
	top := semantic.Conditions[0].RHS.Items[0].FieldBinding
	leaf := semantic.Expression.Condition.RHS.Items[0].FieldBinding
	original := parsed.Conditions[0].RHS.Items[0].FieldBinding
	if top == nil || leaf == nil || original == nil || top == leaf || top == original || leaf == original {
		t.Fatalf("list-item refs share storage: top=%p leaf=%p original=%p", top, leaf, original)
	}
	top.ScopeID = "mutated"
	if leaf.ScopeID == "mutated" || original.ScopeID == "mutated" {
		t.Fatal("top-level list item aliases expression or extraction")
	}
	leaf.ScopeID = "other"
	if top.ScopeID != "mutated" || original.ScopeID == "other" {
		t.Fatal("expression list item aliases top-level or extraction")
	}
}

func TestDataflowRepeatedSummaryNamesKeepAggregateOwnersAcrossSemanticCopy(t *testing.T) {
	parsed := ExtractConditions(`T | summarize n=count() by x | summarize n=count() by n`)
	if len(parsed.Errors) != 0 || parsed.Dataflow == nil || !parsed.Dataflow.Complete || len(parsed.Dataflow.Bindings) != 4 || len(parsed.Aggregations) != 2 {
		t.Fatalf("two summaries missing: %+v", parsed)
	}
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	copy := semanticFromExtraction(parsed, budget)
	if copy == nil || len(copy.GroupByFields) != 2 || copy.GroupByFields[0] != "x" || copy.GroupByFields[1] != "n" {
		t.Fatalf("semantic group metadata lost: %+v", copy)
	}
	for _, result := range []struct {
		name string
		flow *QueryDataflow
		aggs []Aggregation
	}{{"extraction", parsed.Dataflow, parsed.Aggregations}, {"semantic", copy.Dataflow, copy.Aggregations}} {
		if result.flow.Bindings[0].Kind != "aggregate" || result.flow.Bindings[1].Kind != "group" || result.flow.Bindings[2].Kind != "aggregate" || result.flow.Bindings[3].Kind != "group" || result.flow.Bindings[0].Field != "n" || result.flow.Bindings[2].Field != "n" || result.flow.Bindings[3].Field != "n" {
			t.Fatalf("%s: group/aggregate ownership: %+v", result.name, result.flow.Bindings)
		}
		for i, agg := range result.aggs {
			binding := result.flow.Bindings[2*i]
			if agg.StageID != binding.StageID || agg.BindingID != binding.ID || agg.Column != binding.Field || agg.Function != "count" {
				t.Fatalf("%s: aggregation %d points to wrong owner: %+v vs %+v", result.name, i, agg, binding)
			}
		}
	}
	copy.GroupByFields[0] = "mutated"
	copy.Aggregations[0].Column = "mutated"
	if parsed.GroupByFields[0] == "mutated" || parsed.Aggregations[0].Column == "mutated" {
		t.Fatal("group or aggregate metadata aliases extraction")
	}
}

// A final computed-name map cannot distinguish which definition a prior
// filter read. The public JSON must retain each filter's ordered environment.
func TestDataflowPublicJSONKeepsRebindingHistory(t *testing.T) {
	query := `T | extend y=tolower(x) | where y == "a" | extend y=toupper(x) | where y == "A"`
	for name, value := range map[string]any{"extract": ExtractConditions(query), "analyze": AnalyzeQuery(query)} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		var got struct {
			Dataflow struct {
				Complete bool `json:"complete"`
				Stages   []struct {
					ID      string `json:"id"`
					Command string `json:"command"`
					Source  string `json:"source"`
					Inputs  []struct {
						ScopeID string `json:"scope_id"`
					} `json:"inputs"`
				} `json:"stages"`
				Bindings []struct {
					Field        string `json:"field"`
					Expression   string `json:"expression"`
					ID           string `json:"id"`
					InputScopeID string `json:"input_scope_id"`
				} `json:"bindings"`
			} `json:"dataflow"`
			Conditions []struct {
				StageID      string `json:"stage_id"`
				FieldBinding struct {
					Kind      string `json:"kind"`
					BindingID string `json:"binding_id"`
				} `json:"field_binding"`
			} `json:"conditions"`
		}
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatalf("%s: unmarshal: %v", name, err)
		}
		if !got.Dataflow.Complete {
			t.Errorf("%s: missing complete ordered dataflow in %s", name, encoded)
		}
		if len(got.Conditions) != 2 || got.Conditions[0].FieldBinding.BindingID == "" || got.Conditions[1].FieldBinding.BindingID == "" || got.Conditions[0].FieldBinding.BindingID == got.Conditions[1].FieldBinding.BindingID {
			t.Errorf("%s: filters lost distinct y definitions in %s", name, encoded)
		}
		if len(got.Dataflow.Stages) != 4 || got.Dataflow.Stages[0].Source != "extend y=tolower(x)" || got.Dataflow.Stages[2].Source != "extend y=toupper(x)" || len(got.Dataflow.Bindings) != 2 || got.Dataflow.Bindings[0].Expression != "tolower(x)" || got.Dataflow.Bindings[1].Expression != "toupper(x)" || got.Dataflow.Bindings[0].InputScopeID == got.Dataflow.Bindings[1].InputScopeID {
			t.Errorf("%s: authored stages/RHS or ordered read scopes lost in %s", name, encoded)
		}
	}
}

func TestDataflowFilterAliasKeepsAuthoredStage(t *testing.T) {
	query := `T | extend y=tolower(x) | filter y == "a"`
	result := ExtractConditions(query)
	if result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Stages) != 2 || result.Dataflow.Stages[1].Source != `filter y == "a"` || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "binding" {
		t.Fatalf("filter alias source/binding lost: %+v", result)
	}
}

func TestDataflowSelectedQueryAfterLetKeepsOnlyItsStages(t *testing.T) {
	query := "let threshold = 1; T | where x == 1\n\nU | where x == 2"
	result := ExtractConditions(query)
	if result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Stages) != 1 || result.Dataflow.Stages[0].Source != "where x == 1" || len(result.Conditions) != 1 || result.Conditions[0].Value != "1" {
		t.Fatalf("selected query dataflow: %+v", result)
	}
}

func TestDataflowSummarizeByDoesNotPublishSyntheticCount(t *testing.T) {
	result := ExtractConditions(`T | summarize by x | where x==1`)
	if result.Dataflow == nil || result.Dataflow.Complete || len(result.Dataflow.Stages) != 2 || result.Dataflow.Stages[0].Source != "summarize by x" || result.Dataflow.Stages[0].Effect != "opaque" || len(result.Aggregations) != 0 || len(result.Dataflow.Bindings) != 0 || len(result.Dataflow.Diagnostics) == 0 {
		t.Fatalf("synthetic count leaked or stage lost: %+v", result)
	}
}

func TestDataflowRemovedParseRetainsOpaqueStage(t *testing.T) {
	result := ExtractConditions(`T | parse raw with prefix ':' parsed | where parsed == "x"`)
	if len(result.Errors) > 0 || result.Dataflow == nil || result.Dataflow.Complete || len(result.Dataflow.Stages) != 2 {
		t.Fatalf("removed parse stage must survive as opaque dataflow: %+v", result)
	}
	parse, filter := result.Dataflow.Stages[0], result.Dataflow.Stages[1]
	if parse.Command != "parse" || parse.Source != `parse raw with prefix ':' parsed` || parse.Effect != "opaque" || len(filter.Inputs) != 1 || filter.Inputs[0].ScopeID != parse.OutputScopeID || len(result.Conditions) != 1 || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "unknown" {
		t.Fatalf("opaque parse ownership lost: %+v", result)
	}
}

func TestDataflowExtendSiblingReadsInputSnapshot(t *testing.T) {
	result := ExtractConditions(`T | extend x=y, z=x | where z=="a"`)
	if len(result.Errors) > 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Bindings) != 2 || len(result.Conditions) != 1 {
		t.Fatalf("sibling extend dataflow missing: %+v", result)
	}
	x, z := result.Dataflow.Bindings[0], result.Dataflow.Bindings[1]
	if x.Field != "x" || z.Field != "z" || x.InputScopeID != z.InputScopeID || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.BindingID != z.ID {
		t.Fatalf("same-stage RHS did not read one input scope: %+v", result)
	}
	separate := ExtractConditions(`T | extend x=y | extend z=x | where z=="a"`)
	if separate.Dataflow == nil || len(separate.Dataflow.Bindings) != 2 || separate.Dataflow.Bindings[0].InputScopeID == separate.Dataflow.Bindings[1].InputScopeID {
		t.Fatalf("separate stages did not advance scope: %+v", separate)
	}
}

func TestDataflowProjectAndRenameScopes(t *testing.T) {
	project := ExtractConditions(`T | extend Fresh=src | project Fresh | where Fresh=="a" and src=="b"`)
	if project.Dataflow == nil || project.Dataflow.Complete || len(project.Conditions) != 2 || project.Conditions[0].FieldBinding == nil || project.Conditions[0].FieldBinding.Kind != "binding" || project.Conditions[1].FieldBinding == nil || project.Conditions[1].FieldBinding.Kind != "absent" {
		t.Fatalf("closed projection did not mask omitted field: %+v", project)
	}
	rename := ExtractConditions(`T | project-rename New=Old | where New=="a" and Old=="b"`)
	if rename.Dataflow == nil || rename.Dataflow.Complete || len(rename.Conditions) != 2 || rename.Conditions[0].FieldBinding == nil || rename.Conditions[0].FieldBinding.Kind != "binding" || rename.Conditions[1].FieldBinding == nil || rename.Conditions[1].FieldBinding.Kind != "absent" {
		t.Fatalf("rename did not install alias and mask source: %+v", rename)
	}
}

func TestDataflowJoinChildOwnsItsBindings(t *testing.T) {
	result := ExtractConditions(`T | extend x=parent | join (U | extend x=child | where x=="a") on Key | where x=="b"`)
	if len(result.Errors) > 0 || result.Dataflow == nil || len(result.Joins) != 1 || result.Joins[0].Subsearch == nil || result.Joins[0].Subsearch.Dataflow == nil {
		t.Fatalf("joined child graph missing: %+v", result)
	}
	if len(result.Dataflow.Bindings) != 1 || result.Dataflow.Bindings[0].Expression != "parent" || len(result.Joins[0].Subsearch.Dataflow.Bindings) != 1 || result.Joins[0].Subsearch.Dataflow.Bindings[0].Expression != "child" {
		t.Fatalf("join child bindings leaked into parent: %+v", result)
	}
	if len(result.Dataflow.Stages) != 3 || len(result.Dataflow.Stages[1].Inputs) != 2 || result.Dataflow.Stages[1].Inputs[1].QueryPath != "joins/0" || result.Dataflow.Stages[1].Inputs[1].ScopeID != result.Joins[0].Subsearch.Dataflow.OutputScopeID {
		t.Fatalf("join child path is not canonical: %+v", result.Dataflow)
	}
	if len(result.Conditions) != 1 || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "unknown" || len(result.Joins[0].Subsearch.Conditions) != 1 || result.Joins[0].Subsearch.Conditions[0].SourceField != "child" || result.ComputedFields["x"] != "parent" {
		t.Fatalf("join child condition provenance leaked: %+v", result)
	}
}

func TestDataflowExactCaseComputedName(t *testing.T) {
	result := ExtractConditions(`T | extend Fresh=src | where Fresh=="a" and fresh=="b"`)
	if result.Dataflow == nil || !result.Dataflow.Complete || len(result.Conditions) != 2 || result.ComputedFields["Fresh"] != "src" {
		t.Fatalf("exact-case field definitions missing: %+v", result)
	}
	if _, collision := result.ComputedFields["fresh"]; collision {
		t.Fatalf("lowercase compatibility key collided with raw field: %+v", result.ComputedFields)
	}
	if result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "binding" || !result.Conditions[0].IsComputed || result.Conditions[1].FieldBinding == nil || result.Conditions[1].FieldBinding.Kind != "input" || result.Conditions[1].IsComputed {
		t.Fatalf("exact-case condition refs collided: %+v", result.Conditions)
	}
}

func TestDataflowLetExpansionKeepsReferenceSiteSource(t *testing.T) {
	query := `let threshold=5; T | where x > threshold`
	result := ExtractConditions(query)
	if len(result.Errors) > 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Stages) != 1 || result.Dataflow.Stages[0].Source != `where x > threshold` || len(result.Conditions) != 1 || result.Conditions[0].Value != "5" || result.Conditions[0].RHS == nil || result.Conditions[0].RHS.Binding != "threshold" {
		t.Fatalf("let reference-site source/typed origin lost: %+v", result)
	}
}

func TestDataflowTopNestedRewriteStaysOpaque(t *testing.T) {
	result := ExtractConditions(`T | top-nested 5 of Account by count() | where Account=="a"`)
	if len(result.Errors) > 0 || result.Dataflow == nil || result.Dataflow.Complete || len(result.Dataflow.Stages) != 2 || result.Dataflow.Stages[0].Command != "top-nested" || result.Dataflow.Stages[0].Source != `top-nested 5 of Account by count()` || result.Dataflow.Stages[0].Effect != "opaque" || len(result.Aggregations) != 0 || len(result.Conditions) != 1 || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "unknown" {
		t.Fatalf("top-nested synthetic summarize leaked into graph: %+v", result)
	}
}

func TestDataflowTopNestedChainRetainsEachAuthoredStage(t *testing.T) {
	result := ExtractConditions(`T | top-nested 5 of A by count() | top-nested 3 of B by count() | where A=="a"`)
	if len(result.Errors) > 0 || result.Dataflow == nil || result.Dataflow.Complete || len(result.Dataflow.Stages) != 3 || result.Dataflow.Stages[0].Source != `top-nested 5 of A by count()` || result.Dataflow.Stages[1].Source != `top-nested 3 of B by count()` || len(result.Aggregations) != 0 || len(result.Conditions) != 1 || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "unknown" {
		t.Fatalf("top-nested chain lost authored stages: %+v", result)
	}
}

func TestDataflowUnicodeWhitespaceKeepsAuthoredRHS(t *testing.T) {
	result := ExtractConditions("T | extend Fresh\u00a0=\u00a0src | where Fresh==\"a\"")
	if len(result.Errors) > 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Stages) != 2 || result.Dataflow.Stages[0].Source != "extend Fresh\u00a0=\u00a0src" || len(result.Dataflow.Bindings) != 1 || result.Dataflow.Bindings[0].Expression != "\u00a0src" || len(result.Conditions) != 1 || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "binding" {
		t.Fatalf("Unicode normalization lost original stage/RHS: stages=%+v bindings=%+v conditions=%+v", result.Dataflow.Stages, result.Dataflow.Bindings, result.Conditions)
	}
}

func TestDataflowAdjacentStringRHSKeepsAuthoredExpression(t *testing.T) {
	result := ExtractConditions(`T | extend note="a" "b" | where note=="ab"`)
	if len(result.Errors) > 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Bindings) != 1 || result.Dataflow.Bindings[0].Expression != `"a" "b"` || len(result.Dataflow.Stages) != 2 || result.Dataflow.Stages[0].Source != `extend note="a" "b"` || len(result.Conditions) != 1 || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "binding" {
		t.Fatalf("joined string literal lost authored RHS: stages=%+v bindings=%+v result=%+v", result.Dataflow.Stages, result.Dataflow.Bindings, result)
	}
}

func TestDataflowDocumentationPreambleSelectsOriginalStages(t *testing.T) {
	query := "Description: saved detection\nQuery:\nT | extend y=x | where y==\"a\""
	result := ExtractConditions(query)
	if len(result.Errors) > 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Stages) != 2 || result.Dataflow.Stages[0].Source != "extend y=x" || result.Dataflow.Bindings[0].Expression != "x" {
		t.Fatalf("documentation preamble changed selected stage origin: %+v", result)
	}
}

func TestDataflowParameterReplacementKeepsAuthoredRHS(t *testing.T) {
	result := ExtractConditions(`T | extend x={CustomValue} | where x=="a"`)
	if len(result.Errors) > 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Bindings) != 1 || result.Dataflow.Bindings[0].Expression != `{CustomValue}` || len(result.Conditions) != 1 || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "binding" {
		t.Fatalf("parameter replacement lost authored RHS: %+v", result)
	}
}

func TestDataflowSearchAfterAssignmentKeepsReadScope(t *testing.T) {
	result := ExtractConditions(`T | extend x=src | search "needle" | where x=="a"`)
	if len(result.Errors) != 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Stages) != 3 || result.Dataflow.Stages[1].Effect != "filter" || result.Dataflow.Stages[1].Source != `search "needle"` {
		t.Fatalf("search stage ownership changed: %+v", result)
	}
	if result.Expression == nil || len(result.Expression.Children) != 2 || result.Expression.Children[0].Search == nil || result.Expression.Children[0].Search.StageID != result.Dataflow.Stages[1].ID || result.Expression.Children[0].Search.InputScopeID != result.Dataflow.Stages[0].OutputScopeID {
		t.Fatalf("search predicate lost assignment input scope: %+v", result.Expression)
	}
}

func TestDataflowUnionNestedInputsStayLocal(t *testing.T) {
	result := ExtractConditions(`T | union (U | extend x=y), V | where x=="a"`)
	if len(result.Errors) != 0 || result.Dataflow == nil || len(result.Dataflow.Stages) != 3 || len(result.Dataflow.Stages[0].Inputs) != 3 || result.Dataflow.Stages[0].Inputs[1].QueryPath != "" || result.Dataflow.Stages[0].Inputs[1].ScopeID != result.Dataflow.Stages[1].OutputScopeID || result.Dataflow.Stages[1].Pipeline != 1 || result.Dataflow.OutputScopeID != result.Dataflow.Stages[2].OutputScopeID {
		t.Fatalf("union input/output ownership changed: %+v", result)
	}
}

func TestDataflowSingleTableJoinAndMaterializedSearchOwners(t *testing.T) {
	join := ExtractConditions(`SecurityEvent | join (SigninLogs) on Account`)
	if len(join.Errors) != 0 || join.Dataflow == nil || len(join.Dataflow.Stages) != 1 || len(join.Dataflow.Stages[0].Inputs) != 2 || len(join.Joins) != 1 || join.Joins[0].Subsearch == nil || join.Joins[0].Subsearch.Dataflow == nil || join.Dataflow.Stages[0].Inputs[1].ScopeID != join.Joins[0].Subsearch.Dataflow.OutputScopeID {
		t.Fatalf("single-table child lacks authored owner: %+v", join)
	}
	materialized := ExtractConditions(`materialize(search "needle") | where x == 1`)
	if len(materialized.Errors) != 0 || materialized.Dataflow == nil || len(materialized.Dataflow.Stages) != 2 || materialized.Dataflow.Stages[0].Source != `search "needle"` || materialized.Dataflow.Stages[0].Command != "search" || materialized.Dataflow.Stages[0].Pipeline == materialized.Dataflow.Stages[1].Pipeline {
		t.Fatalf("materialized search lacks its own authored stage: %+v", materialized)
	}
}

func TestDataflowVerbatimLiteralKeepsFollowingStage(t *testing.T) {
	query := `T | extend p=@"C:\" | where p=="a"`
	for name, result := range map[string]*ParseResult{"extract": ExtractConditions(query)} {
		if len(result.Errors) != 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Stages) != 2 || len(result.Dataflow.Bindings) != 1 || len(result.Conditions) != 1 {
			t.Fatalf("%s: verbatim stage boundary: %+v", name, result)
		}
		if result.Dataflow.Stages[0].Source != `extend p=@"C:\"` || result.Dataflow.Stages[1].Source != `where p=="a"` || result.Dataflow.Bindings[0].Expression != `@"C:\"` || result.Conditions[0].StageID != result.Dataflow.Stages[1].ID || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.BindingID != result.Dataflow.Bindings[0].ID {
			t.Fatalf("%s: verbatim source/read owner: %+v", name, result)
		}
	}
	semantic := AnalyzeQuery(query)
	if len(semantic.Errors) != 0 || semantic.Dataflow == nil || len(semantic.Dataflow.Stages) != 2 || len(semantic.Conditions) != 1 || semantic.Conditions[0].FieldBinding == nil || semantic.Conditions[0].FieldBinding.BindingID != semantic.Dataflow.Bindings[0].ID {
		t.Fatalf("analyze: verbatim owner: %+v", semantic)
	}
}

func TestDataflowTransportKeepsOrdinaryStages(t *testing.T) {
	for _, query := range []string{
		`extend Fresh=src | where Fresh=="a"`,
		`T | extend pattern=src | where pattern=="a"`,
		`T | extend pattern=src,nodes=other | where pattern=="a"`,
	} {
		result := ExtractConditions(query)
		if len(result.Errors) != 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Stages) != 2 || result.Dataflow.Stages[0].Effect != "assign" || result.Dataflow.Stages[1].Effect != "filter" || len(result.Conditions) != 1 || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "binding" {
			t.Fatalf("transport lost ordinary graph for %q: %+v", query, result)
		}
		if result.Dataflow.Scopes[0].Source == "DummyTable" || result.Conditions[0].Field != result.Dataflow.Bindings[0].Field {
			t.Fatalf("transport published synthetic name for %q: %+v", query, result)
		}
		semantic := AnalyzeQuery(query)
		if len(semantic.Errors) != 0 || semantic.Dataflow == nil || !semantic.Dataflow.Complete || len(semantic.Conditions) != 1 || semantic.Conditions[0].Field != result.Conditions[0].Field {
			t.Fatalf("Analyze transport mismatch for %q: %+v", query, semantic)
		}
	}
}

func TestDataflowAuthoredProjectionAndGroupNames(t *testing.T) {
	projection := ExtractConditions(`T | extend 3plogTime=src | project 3plogTime | where 3plogTime=="a"`)
	if len(projection.Errors) != 0 || projection.Dataflow == nil || !projection.Dataflow.Complete || len(projection.Dataflow.Bindings) != 2 || projection.Dataflow.Bindings[1].Field != "3plogTime" || projection.Dataflow.Bindings[1].Expression != "3plogTime" || len(projection.Conditions) != 1 || projection.Conditions[0].FieldBinding == nil || projection.Conditions[0].FieldBinding.BindingID != projection.Dataflow.Bindings[1].ID {
		t.Fatalf("projection normalized authored name/expression: %+v", projection)
	}
	group := ExtractConditions(`T | summarize n=count() by ['Fresh Name'] | where ['Fresh Name']=="a"`)
	if len(group.Errors) != 0 || group.Dataflow == nil || !group.Dataflow.Complete || len(group.Dataflow.Bindings) != 2 || group.Dataflow.Bindings[1].Field != "Fresh Name" || group.Dataflow.Bindings[1].Expression != "['Fresh Name']" || len(group.Conditions) != 1 || group.Conditions[0].FieldBinding == nil || group.Conditions[0].FieldBinding.BindingID != group.Dataflow.Bindings[1].ID {
		t.Fatalf("group normalized authored name/expression: %+v", group)
	}
}

func TestDataflowHistoricalSourceFieldFollowsVisibleBinding(t *testing.T) {
	result := ExtractConditions(`T | extend y=x | where y=="a" | extend y=z | where y=="b"`)
	if len(result.Errors) != 0 || result.Dataflow == nil || len(result.Conditions) != 2 || result.Conditions[0].SourceField != "x" || result.Conditions[1].SourceField != "z" || result.Conditions[0].FieldBinding == nil || result.Conditions[1].FieldBinding == nil || result.Conditions[0].FieldBinding.BindingID == result.Conditions[1].FieldBinding.BindingID {
		t.Fatalf("historical coarse provenance changed: %+v", result)
	}
	semantic := AnalyzeQuery(`T | extend y=x | where y=="a" | extend y=z | where y=="b"`)
	if len(semantic.Conditions) != 2 || semantic.Conditions[0].SourceField != "x" || semantic.Conditions[1].SourceField != "z" {
		t.Fatalf("semantic historical hints changed: %+v", semantic)
	}
}

func TestDataflowMaterializedSourceFeedsParentFilter(t *testing.T) {
	query := `materialize(T | extend x=y) | where x=="a"`
	result := ExtractConditions(query)
	if len(result.Errors) != 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Stages) != 2 || len(result.Dataflow.Bindings) != 1 || len(result.Conditions) != 1 || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.BindingID != result.Dataflow.Bindings[0].ID || result.Conditions[0].InputScopeID != result.Dataflow.Stages[0].OutputScopeID {
		t.Fatalf("materialized child output disconnected: %+v", result)
	}
	semantic := AnalyzeQuery(query)
	if len(semantic.Errors) != 0 || semantic.Dataflow == nil || len(semantic.Conditions) != 1 || semantic.Conditions[0].FieldBinding == nil || semantic.Conditions[0].FieldBinding.BindingID != semantic.Dataflow.Bindings[0].ID {
		t.Fatalf("semantic materialized child disconnected: %+v", semantic)
	}
}

func TestDataflowTableOnlyMaterializedSourceFeedsParentFilter(t *testing.T) {
	query := `materialize(T) | where x=="a"`
	result := ExtractConditions(query)
	if len(result.Errors) != 0 || result.Dataflow == nil || !result.Dataflow.Complete || len(result.Dataflow.Stages) != 1 || len(result.Dataflow.Scopes) != 2 || len(result.Conditions) != 1 || result.Dataflow.Scopes[0].Mode != "input" || result.Dataflow.Scopes[0].Source != "T" || result.Dataflow.Stages[0].Inputs[0].ScopeID != result.Dataflow.Scopes[0].ID || result.Conditions[0].InputScopeID != result.Dataflow.Scopes[0].ID || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "input" {
		t.Fatalf("extraction table-only child output lost: %+v", result)
	}
	semantic := AnalyzeQuery(query)
	if len(semantic.Errors) != 0 || semantic.Dataflow == nil || !semantic.Dataflow.Complete || len(semantic.Dataflow.Stages) != 1 || len(semantic.Dataflow.Scopes) != 2 || semantic.Dataflow.Scopes[0].Mode != "input" || semantic.Dataflow.Scopes[0].Source != "T" || len(semantic.Conditions) != 1 || semantic.Dataflow.Stages[0].Inputs[0].ScopeID != semantic.Dataflow.Scopes[0].ID || semantic.Conditions[0].InputScopeID != semantic.Dataflow.Scopes[0].ID || semantic.Conditions[0].FieldBinding == nil || semantic.Conditions[0].FieldBinding.Kind != "input" {
		t.Fatalf("semantic table-only child output lost: %+v", semantic)
	}
}

func TestDataflowProjectAsAliasRHSIsOnlyExpression(t *testing.T) {
	query := `T | project x as y | where y=="a"`
	parsed := ExtractConditions(query)
	if len(parsed.Errors) != 0 || parsed.Dataflow == nil || !parsed.Dataflow.Complete || len(parsed.Dataflow.Stages) != 2 || len(parsed.Dataflow.Bindings) != 1 || parsed.Dataflow.Bindings[0].Field != "y" || parsed.Dataflow.Bindings[0].Expression != "x" || len(parsed.Conditions) != 1 || parsed.Conditions[0].FieldBinding == nil || parsed.Conditions[0].FieldBinding.BindingID != parsed.Dataflow.Bindings[0].ID {
		t.Fatalf("project alias authored RHS: %+v", parsed)
	}
	semantic := AnalyzeQuery(query)
	if len(semantic.Errors) != 0 || semantic.Dataflow == nil || !semantic.Dataflow.Complete || len(semantic.Dataflow.Bindings) != 1 || semantic.Dataflow.Bindings[0].Expression != "x" || len(semantic.Conditions) != 1 || semantic.Conditions[0].FieldBinding == nil || semantic.Conditions[0].FieldBinding.BindingID != semantic.Dataflow.Bindings[0].ID {
		t.Fatalf("semantic project alias authored RHS: %+v", semantic)
	}
}

func TestDataflowDottedAsSegmentPreservesProjectExpression(t *testing.T) {
	query := `T | project obj.as.value | where obj.as.value=="a"`
	parsed := ExtractConditions(query)
	if len(parsed.Errors) != 0 || parsed.Dataflow == nil || !parsed.Dataflow.Complete || len(parsed.Dataflow.Bindings) != 1 || parsed.Dataflow.Bindings[0].Field != "obj.as.value" || parsed.Dataflow.Bindings[0].Expression != "obj.as.value" || len(parsed.Conditions) != 1 || parsed.Conditions[0].FieldBinding == nil || parsed.Conditions[0].FieldBinding.BindingID != parsed.Dataflow.Bindings[0].ID {
		t.Fatalf("dotted project expression in extraction: %+v", parsed)
	}
	semantic := AnalyzeQuery(query)
	if len(semantic.Errors) != 0 || semantic.Dataflow == nil || !semantic.Dataflow.Complete || len(semantic.Dataflow.Bindings) != 1 || semantic.Dataflow.Bindings[0].Field != "obj.as.value" || semantic.Dataflow.Bindings[0].Expression != "obj.as.value" || len(semantic.Conditions) != 1 || semantic.Conditions[0].FieldBinding == nil || semantic.Conditions[0].FieldBinding.BindingID != semantic.Dataflow.Bindings[0].ID {
		t.Fatalf("dotted project expression in semantic result: %+v", semantic)
	}
}

// A trailing decimal point belongs to the accepted numeric expression, not
// member access; rejecting its alias used to invent a read of field "as".
func TestDataflowNumericDotProjectAliasKeepsExpressionOwnership(t *testing.T) {
	query := `T | project 1. as y | where y==1`
	parsed := ExtractConditions(query)
	t.Run("extract", func(t *testing.T) {
		if len(parsed.Errors) != 0 || parsed.Dataflow == nil || !parsed.Dataflow.Complete || len(parsed.Dataflow.Stages) != 2 || len(parsed.Dataflow.Bindings) != 1 || len(parsed.Conditions) != 1 {
			t.Fatalf("numeric alias extraction failed: %+v", parsed)
		}
		binding, condition := parsed.Dataflow.Bindings[0], parsed.Conditions[0]
		if binding.Field != "y" || binding.Expression != "1." || condition.Field != "y" || condition.SourceField != "" || condition.FieldBinding == nil || condition.FieldBinding.Kind != "binding" || condition.FieldBinding.BindingID != binding.ID {
			t.Fatalf("numeric alias expression/reference: binding=%+v condition=%+v", binding, condition)
		}
	})
	semantic := AnalyzeQuery(query)
	t.Run("analyze", func(t *testing.T) {
		if len(semantic.Errors) != 0 || semantic.Dataflow == nil || !semantic.Dataflow.Complete || len(semantic.Dataflow.Stages) != 2 || len(semantic.Dataflow.Bindings) != 1 || len(semantic.Conditions) != 1 {
			t.Fatalf("numeric alias analysis failed: %+v", semantic)
		}
		binding, condition := semantic.Dataflow.Bindings[0], semantic.Conditions[0]
		if binding.Field != "y" || binding.Expression != "1." || condition.Field != "y" || condition.SourceField != "" || condition.FieldBinding == nil || condition.FieldBinding.Kind != "binding" || condition.FieldBinding.BindingID != binding.ID {
			t.Fatalf("semantic numeric alias expression/reference: binding=%+v condition=%+v", binding, condition)
		}
	})
}

func TestDataflowProjectAsAliasCommentsKeepExpressionOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, query, expression string
	}{
		{"after alias", `T | project x as/*c*/y | where y=="a"`, "x"},
		{"before alias", `T | project x/*c*/as y | where y=="a"`, "x/*c*/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := ExtractConditions(tc.query)
			t.Run("extract", func(t *testing.T) {
				if len(parsed.Errors) != 0 || parsed.Dataflow == nil || !parsed.Dataflow.Complete || len(parsed.Dataflow.Stages) != 2 || len(parsed.Dataflow.Bindings) != 1 || len(parsed.Conditions) != 1 {
					t.Fatalf("project alias extraction failed: %+v", parsed)
				}
				binding := parsed.Dataflow.Bindings[0]
				if binding.Field != "y" || binding.Expression != tc.expression || parsed.Conditions[0].FieldBinding == nil || parsed.Conditions[0].FieldBinding.BindingID != binding.ID {
					t.Fatalf("project alias expression/reference: binding=%+v condition=%+v, want expression %q", binding, parsed.Conditions[0], tc.expression)
				}
			})
			semantic := AnalyzeQuery(tc.query)
			t.Run("analyze", func(t *testing.T) {
				if len(semantic.Errors) != 0 || semantic.Dataflow == nil || !semantic.Dataflow.Complete || len(semantic.Dataflow.Stages) != 2 || len(semantic.Dataflow.Bindings) != 1 || len(semantic.Conditions) != 1 {
					t.Fatalf("project alias analysis failed: %+v", semantic)
				}
				binding := semantic.Dataflow.Bindings[0]
				if binding.Field != "y" || binding.Expression != tc.expression || semantic.Conditions[0].FieldBinding == nil || semantic.Conditions[0].FieldBinding.BindingID != binding.ID {
					t.Fatalf("semantic alias expression/reference: binding=%+v condition=%+v, want expression %q", binding, semantic.Conditions[0], tc.expression)
				}
			})
		})
	}
}

func TestDataflowOverlappingRenameIsOpaqueInEitherOrder(t *testing.T) {
	for _, query := range []string{
		`T | project-rename a=b, b=c | where a=="x"`,
		`T | project-rename b=c, a=b | where a=="x"`,
	} {
		result := ExtractConditions(query)
		if len(result.Errors) != 0 || result.Dataflow == nil || result.Dataflow.Complete || len(result.Dataflow.Stages) != 2 || result.Dataflow.Stages[0].Effect != "opaque" || len(result.Dataflow.Diagnostics) == 0 || len(result.Dataflow.Bindings) != 0 || len(result.Conditions) != 1 || result.Conditions[0].FieldBinding == nil || result.Conditions[0].FieldBinding.Kind != "unknown" {
			t.Fatalf("overlapping rename claimed independent mapping for %q: %+v", query, result)
		}
	}
}
