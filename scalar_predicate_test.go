package kql

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestScalarPredicatesPreserveFilterBranches(t *testing.T) {
	for _, predicate := range []string{`true`, `false`, `5 > 10`, `"é" == "é"`, `1 + 2 > 5`, `Score + 1 > 5`, `not(false)`, `int(null) == long(null)`, `5`, `-1e-3 == 0x10`, `long(9007199254740993) == long(9007199254740994)`} {
		t.Run(predicate, func(t *testing.T) {
			result := ExtractConditions(`Events | where ` + predicate)
			if len(result.Conditions) != 0 || result.Expression == nil || result.Expression.Operator != "scalar" {
				t.Fatalf("lost scalar %s: %#v / %#v / %v", predicate, result.Conditions, result.Expression, result.Errors)
			}
			encoded, err := json.Marshal(result.Expression)
			if err != nil {
				t.Fatal(err)
			}
			var decoded ConditionExpression
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.Expression, &decoded) {
				t.Fatalf("round trip: %s", encoded)
			}
			if semantic := AnalyzeQuery(`Events | where ` + predicate); semantic.Expression == nil || semantic.Expression.Operator != "scalar" {
				t.Fatalf("semantic lost scalar: %#v", semantic)
			}
		})
	}
}

func TestComparisonOperandOrientation(t *testing.T) {
	inverse := map[string]string{"==": "==", "!=": "!=", "=~": "=~", "!~": "!~", "<": ">", "<=": ">=", ">": "<", ">=": "<="}
	for op, swapped := range inverse {
		for _, literal := range []string{`5`, `-5`, `(+5)`, `1e3`, `1e-3`, `0x10`, `9007199254740993`, `true`, `"value"`, `int(null)`, `datetime(2026-01-01)`} {
			for _, reverse := range []bool{false, true} {
				left, right, want := `Score`, literal, op
				if reverse {
					left, right, want = literal, `Score`, swapped
				}
				result := ExtractConditions(`Events | where ` + left + ` ` + op + ` ` + right)
				if len(result.Conditions) != 1 || result.Conditions[0].Field != "Score" || result.Conditions[0].Operator != want {
					t.Errorf("%s %s %s: %#v errors %v", left, op, right, result.Conditions, result.Errors)
				}
			}
		}
	}
}

func TestScalarBranchBarrierAndQuotedIdentifiers(t *testing.T) {
	for _, predicate := range []string{`a == 1 or 5 > 10 or a == 2`, `false and a == 1`, `a == 1 and false`, `not(a == 1 or false)`} {
		r := ExtractConditions(`Events | where ` + predicate)
		if r.Expression == nil || r.Expression.Operator == BooleanCondition {
			t.Errorf("lost branch %s: %#v", predicate, r)
		}
	}
	r := ExtractConditions(`Events | where ['5'] == 1 and _5_ == 2 and ['true'] == false`)
	if len(r.Conditions) != 3 || r.Conditions[0].Field != "5" || r.Conditions[1].Field != "_5_" || r.Conditions[2].Field != "true" {
		t.Fatalf("quoted collision: %#v %v", r.Conditions, r.Errors)
	}
}

func scalarLeaves(expression *ConditionExpression) []*ScalarPredicate {
	if expression == nil {
		return nil
	}
	if expression.Operator == BooleanScalar {
		return []*ScalarPredicate{expression.Scalar}
	}
	var result []*ScalarPredicate
	for _, child := range expression.Children {
		result = append(result, scalarLeaves(child)...)
	}
	return result
}

func TestScalarStagesTextNegationAndRemapping(t *testing.T) {
	r := ExtractConditions(`T | where not(a == 1 or "é" == "é") | extend n=1 | where false or a == 2 or a == 3`)
	leaves := scalarLeaves(r.Expression)
	if len(r.Conditions) != 2 || len(leaves) != 2 || leaves[0].Text != `"é" == "é"` || !leaves[0].Negated || leaves[0].PipeStage != 0 || leaves[1].Text != "false" || leaves[1].PipeStage != 2 || leaves[1].Pipeline != 0 || leaves[1].Negated {
		t.Fatalf("lost stages/text/negation: %#v %#v", r, leaves)
	}
	barrier := ExtractConditions(`T | where a == 1 or false or a == 2`)
	if len(barrier.Conditions) != 2 || len(barrier.Expression.Children) != 3 || len(barrier.Conditions[0].Alternatives) != 0 {
		t.Fatalf("scalar did not block alternatives: %#v", barrier)
	}
	semantic := semanticFromExtraction(r)
	encoded, _ := json.Marshal(semantic)
	var decoded SemanticQuery
	if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(semantic, &decoded) {
		t.Fatalf("semantic roundtrip: %s / %v", encoded, err)
	}
	var mutate func(*SemanticExpression)
	mutate = func(node *SemanticExpression) {
		if node.Scalar != nil {
			node.Scalar.Text = "changed"
		}
		for _, child := range node.Children {
			mutate(child)
		}
	}
	mutate(semantic.Expression)
	if leaves[0].Text == "changed" {
		t.Fatal("semantic scalar aliases parse scalar")
	}
}

func TestScalarNonFilterAndLetScope(t *testing.T) {
	for _, query := range []string{`T | extend n=5 > 10`, `T | project n=false`, `T | summarize n=countif(false)`, `print n=false`} {
		r := ExtractConditions(query)
		if r.Expression != nil || len(r.Conditions) != 0 {
			t.Errorf("computed expression became filter: %s: %#v", query, r)
		}
	}
	for _, query := range []string{`let threshold=5; let threshold=10; T | where Score > threshold`, `let flag=isnull(a); T | where flag or x == 1`, `let f=(threshold:long) {T | where Score > threshold}; let threshold=5; T | where Score > threshold`} {
		r := ExtractConditions(query)
		if len(scalarLeaves(r.Expression)) == 0 || len(r.LetStatements) == 0 {
			t.Errorf("let binding disappeared: %s: %#v", query, r)
		}
	}
}

func TestScalarListAndRangePredicates(t *testing.T) {
	for _, predicate := range []string{`5 in (1, 2)`, `5 between (1 .. 10)`, `"x" has_any ("x", "y")`, `a + 1 in (1, 2)`} {
		r := ExtractConditions(`T | where ` + predicate)
		if len(r.Conditions) != 0 || r.Expression == nil || r.Expression.Operator != BooleanScalar {
			t.Errorf("scalar list/range %s: %#v", predicate, r)
		}
	}
}

func TestRepeatedScalarPredicatesRemainDistinct(t *testing.T) {
	r := ExtractConditions(`T | where a == 1 and rand() > 0.5 and rand() > 0.5`)
	if leaves := scalarLeaves(r.Expression); len(leaves) != 2 {
		t.Fatalf("unevaluated scalar predicates were deduplicated: %#v", r.Expression)
	}
}

func TestOperandValuesAndReferenceMetadata(t *testing.T) {
	for _, literal := range []string{`-5`, `+5`, `1e3`, `1e-3`, `0x10`, `9007199254740993`, `long(9007199254740993)`, `real(null)`, `datetime(2026-01-01)`} {
		for _, predicate := range []string{`Score > ` + literal, literal + ` < Score`} {
			r := ExtractConditions(`T | where ` + predicate)
			if len(r.Conditions) != 1 || r.Conditions[0].Value != literal || r.Conditions[0].ValueReference != "" {
				t.Errorf("literal changed %s: %#v", predicate, r.Conditions)
			}
		}
	}
	r := ExtractConditions(`T | extend Score=OriginalScore | where not(5 < Score) | where ['where'] == ['5']`)
	if len(r.Conditions) != 2 {
		t.Fatalf("conditions: %#v", r)
	}
	first, second := r.Conditions[0], r.Conditions[1]
	if first.Field != "Score" || first.Operator != ">" || !first.Negated || !first.IsComputed || first.SourceField != "OriginalScore" || first.PipeStage != 1 || first.Pipeline != 0 {
		t.Errorf("swapped metadata: %#v", first)
	}
	if second.Field != "where" || second.ValueReference != "5" || second.Value != "" {
		t.Errorf("quoted reference changed: %#v", second)
	}
}

func TestLetShadowingAndForwardReferencesStayScalar(t *testing.T) {
	for _, query := range []string{`let threshold=5; T | extend threshold=10 | where Score > threshold`, `let threshold=5; let threshold=10; T | where (x == 1 and Score > threshold)`} {
		r := ExtractConditions(query)
		if len(scalarLeaves(r.Expression)) != 1 {
			t.Errorf("unsafe binding lost: %s: %#v", query, r)
		}
	}
	query := `let early=T | where Score > threshold; let threshold=5; T | where Score > threshold`
	inlined := inlineLetConstants(query, extractLetStatements(query))
	if inlined != `let early=T | where Score > threshold; let threshold=5; T | where Score > 5` {
		t.Errorf("forward reference changed: %s", inlined)
	}
	r := ExtractConditions(`let threshold=5; let threshold=10; T | where Score > threshold`)
	if len(r.LetStatements) != 2 || r.LetStatements[0].Expression != "5" || r.LetStatements[1].Expression != "10" {
		t.Errorf("shadowed binding lost: %#v", r.LetStatements)
	}
}

func TestUnrepresentableFilterNeverDisappears(t *testing.T) {
	for _, predicate := range []string{`a in ExternalTable`, `['true'] in (1, 2)`, `['true'] has_any ("x", "y")`} {
		r := ExtractConditions(`T | where ` + predicate)
		if r.Expression == nil {
			t.Errorf("filter disappeared: %s: %#v", predicate, r)
		}
	}
}

func TestLetColumnIntroductionsStayScalar(t *testing.T) {
	for _, query := range []string{`let threshold=5; range threshold from 1 to 10 step 1 | where Score > threshold`, `let threshold=5; T | mv-expand threshold | where Score > threshold`, `let threshold=5; T | summarize count() by threshold | where Score > threshold`} {
		r := ExtractConditions(query)
		if len(scalarLeaves(r.Expression)) != 1 {
			t.Errorf("column declaration was substituted: %s: %#v", query, r)
		}
	}
}

func TestScalarPredicatesBelongToNestedPipelines(t *testing.T) {
	r := ExtractConditions(`T | where false | union (U | where true) | where 1 > 2`)
	leaves := scalarLeaves(r.Expression)
	if len(r.Conditions) != 0 || len(leaves) != 3 || leaves[0].Pipeline != 0 || leaves[1].Pipeline != 1 || leaves[2].Pipeline != 0 || leaves[0].PipeStage != 0 || leaves[1].PipeStage != 1 || leaves[2].PipeStage != 3 {
		t.Fatalf("scalar pipeline metadata lost: %#v %#v", r, leaves)
	}
}

func TestQuotedLetReferencesRemainScalar(t *testing.T) {
	r := ExtractConditions(`let threshold=5; T | where Score > ['threshold']`)
	if len(r.Conditions) != 0 || len(scalarLeaves(r.Expression)) != 1 {
		t.Fatalf("quoted let reference lost its binding: %#v", r)
	}
}

func TestQuotedLetColumnIntroductionsStayScalar(t *testing.T) {
	for _, introduction := range []string{`mv-expand ['threshold']`, `project ['threshold']`, `summarize count() by ['threshold']`} {
		query := `let threshold=5; T | ` + introduction + ` | where Score > threshold`
		r := ExtractConditions(query)
		leaves := scalarLeaves(r.Expression)
		if len(r.Conditions) != 0 || len(leaves) != 1 || leaves[0].Text != `Score > threshold` || len(r.LetStatements) != 1 {
			t.Errorf("quoted declaration changed binding: %s: %#v / %#v", query, r, leaves)
		}
	}
}

func TestNullEmptyScalarArgumentsRemainCompletePredicates(t *testing.T) {
	// These functions accept scalar expressions, not only column names:
	// https://learn.microsoft.com/en-us/kusto/query/isnull-function
	for _, function := range []string{"isnull", "isnotnull", "isempty", "isnotempty"} {
		for _, argument := range []string{`5`, `""`, `int(null)`, `a + 1`, `tostring(rand())`} {
			for _, predicate := range []string{function + `(` + argument + `)`, function + `(` + argument + `) == false`} {
				t.Run(predicate, func(t *testing.T) {
					r := ExtractConditions(`T | where ` + predicate)
					leaves := scalarLeaves(r.Expression)
					if len(r.Conditions) != 0 || len(leaves) != 1 || leaves[0].Text != predicate || r.Expression.Operator != BooleanScalar {
						t.Fatalf("scalar argument fabricated a field: %#v / %#v", r, leaves)
					}
					semantic := AnalyzeQuery(`T | where ` + predicate)
					if semantic.Expression == nil || semantic.Expression.Scalar == nil || semantic.Expression.Scalar.Text != predicate {
						t.Fatalf("semantic scalar predicate lost: %#v", semantic)
					}
				})
			}
		}
		for _, argument := range []string{`['5']`, `(['5'])`} {
			r := ExtractConditions(`T | where ` + function + `(` + argument + `)`)
			if len(r.Conditions) != 1 || r.Conditions[0].Field != "5" || r.Conditions[0].Operator != function {
				t.Errorf("quoted argument not decoded: %s(%s): %#v", function, argument, r)
			}
		}
	}
}

func TestComputedAndReferenceListOperandsStayScalar(t *testing.T) {
	// IN accepts scalar and tabular expressions; string alternatives cannot
	// represent those expressions or distinguish list references from literals.
	// https://learn.microsoft.com/en-us/kusto/query/in-cs-operator
	for _, operator := range []string{"in", "!in", "in~", "!in~", "has_any", "has_all"} {
		for _, values := range []string{`rand()`, `tostring(rand()), tostring(rand())`, `other`, `SubqueryResult`, `['5']`, `"literal", other`, `1 + 2`} {
			predicate := `a ` + operator + ` (` + values + `)`
			r := ExtractConditions(`T | where ` + predicate)
			leaves := scalarLeaves(r.Expression)
			if len(r.Conditions) != 0 || len(leaves) != 1 || leaves[0].Text != predicate {
				t.Errorf("list expressions became literals: %s: %#v / %#v", predicate, r, leaves)
			}
		}
	}
	query := `T | where a has_any (tostring(rand()), tostring(rand())) and a has_any (tostring(rand()), tostring(rand()))`
	if r := ExtractConditions(query); len(scalarLeaves(r.Expression)) != 2 {
		t.Fatalf("nondeterministic lists collapsed: %#v", r)
	}
	r := ExtractConditions(`T | extend x=1 | where not(a has_any (tostring(rand()), tostring(rand())) or a == 1)`)
	leaves := scalarLeaves(r.Expression)
	if len(leaves) != 1 || !leaves[0].Negated || leaves[0].Text != `a has_any (tostring(rand()), tostring(rand()))` || leaves[0].PipeStage != 1 || leaves[0].Pipeline != 0 || r.Expression.Operator != BooleanAnd || len(r.Conditions) != 1 || !r.Conditions[0].Negated {
		t.Fatalf("negated list scalar changed topology or metadata: %#v / %#v", r, leaves)
	}
	semantic := semanticFromExtraction(r)
	encoded, err := json.Marshal(semantic)
	if err != nil {
		t.Fatal(err)
	}
	var decoded SemanticQuery
	if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(semantic, &decoded) {
		t.Fatalf("list scalar semantic roundtrip failed: %s / %v", encoded, err)
	}
}

func TestLiteralListsDoNotFabricateReferences(t *testing.T) {
	for _, operator := range []string{"in", "!in", "in~", "!in~", "has_any", "has_all"} {
		r := ExtractConditions(`T | where a ` + operator + ` ("token")`)
		if len(r.Conditions) != 1 || r.Conditions[0].Value != "token" || r.Conditions[0].ValueReference != "" {
			t.Errorf("literal confused with reference: %s: %#v", operator, r)
		}
	}
}

func TestRangeBoundsKeepTypedReferences(t *testing.T) {
	for _, operator := range []string{"between", "!between"} {
		for _, bounds := range []string{`low .. high`, `['5'] .. 10`, `1 .. ['high']`} {
			r := ExtractConditions(`T | where a ` + operator + ` (` + bounds + `)`)
			if len(r.Conditions) != 2 {
				t.Fatalf("range lost: %s %s: %#v", operator, bounds, r)
			}
			wantRefs := map[string][2]string{`low .. high`: {"low", "high"}, `['5'] .. 10`: {"5", ""}, `1 .. ['high']`: {"", "high"}}[bounds]
			for index, condition := range r.Conditions {
				if condition.ValueReference != wantRefs[index] || wantRefs[index] != "" && condition.Value != "" || condition.Negated != (operator == "!between") {
					t.Errorf("bound typing changed: %s %s: %#v", operator, bounds, condition)
				}
			}
		}
	}
	for _, predicate := range []string{`a between (rand() .. 10)`, `a between (1 .. high + 1)`} {
		r := ExtractConditions(`T | where ` + predicate)
		if leaves := scalarLeaves(r.Expression); len(r.Conditions) != 0 || len(leaves) != 1 || leaves[0].Text != predicate {
			t.Errorf("computed range changed: %s: %#v", predicate, r)
		}
	}
}

func TestSyntheticSubqueryCannotBecomeCompleteScalar(t *testing.T) {
	for _, predicate := range []string{
		`tolower(Account) in ((SigninLogs | project Account))`,
		`5 in ((T2 | project n))`,
		`a == 1 and tolower(Account) in ((SigninLogs | project Account))`,
		`tolower(Account) in ((SigninLogs | project Account)) or a == 1`,
		`not(5 in ((T2 | project n)))`,
		`isnull(tolower(Account) in ((SigninLogs | project Account)))`,
		`"é" == "é" and 5 in ((T2 | project n))`,
		`Account in ((SigninLogs | project Account)) and 5 in ((T2 | project n))`,
	} {
		query := `T | where ` + predicate
		r := ExtractConditions(query)
		if len(r.Errors) != 1 || r.Errors[0] != "unsupported scalar predicate: unresolved IN subquery" || len(r.Conditions) != 0 || r.Expression != nil {
			t.Errorf("synthetic subquery looked complete: %s: %#v", query, r)
		}
		semantic := AnalyzeQuery(query)
		if len(semantic.Errors) != 1 || semantic.Errors[0] != "unsupported scalar predicate: unresolved IN subquery" || len(semantic.Conditions) != 0 || semantic.Expression != nil {
			t.Errorf("semantic extraction retained partial filters: %s: %#v", query, semantic)
		}
	}
}

func TestAuthoredSubqueryMarkerStringsRemainOrdinaryValues(t *testing.T) {
	for _, predicate := range []string{
		`tolower(Account) in ("SubqueryResult")`,
		`5 in ("SubqueryResult")`,
		`tolower(Account) in ("__kql_in_subquery_0__")`,
		`tolower(Account) in ("SubqueryResult") and Account in ((SigninLogs | project Account))`,
		`tolower(Account) in ("__kql_in_subquery_0__") and Account in ((SigninLogs | project Account))`,
	} {
		r := ExtractConditions(`T | where ` + predicate)
		if leaves := scalarLeaves(r.Expression); len(r.Errors) != 0 || len(leaves) != 1 {
			t.Errorf("authored string confused with synthetic marker: %s: %#v", predicate, r)
		}
	}
	r := ExtractConditions(`T | where Account in ("SubqueryResult")`)
	if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].Value != "SubqueryResult" || r.Conditions[0].ValueReference != "" {
		t.Fatalf("authored literal changed: %#v", r)
	}
}

func TestJoinedAuthoredSubqueryMarkersRemainUnchanged(t *testing.T) {
	r := ExtractConditions(`T | where a == "__kql_in_" "subquery_0__"`)
	if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].Value != "__kql_in_subquery_0__" {
		t.Errorf("joined authored value rewritten as synthetic marker: %#v", r)
	}
	r = ExtractConditions(`T | where tolower(Account) in ("__kql_in_" "subquery_0__")`)
	if leaves := scalarLeaves(r.Expression); len(r.Errors) != 0 || len(leaves) != 1 || leaves[0].Text != `tolower(Account) in ("__kql_in_subquery_0__")` {
		t.Fatalf("joined authored scalar rejected or rewritten: %#v", r)
	}
}

func TestSubqueryProvenanceAfterStringJoining(t *testing.T) {
	query := `T | where "é" == "é" and a == "__kql_in_" "subquery_0__" and b == "__kql_in_subquery_" "1__" and c == "SubqueryResult" and Account in ((SigninLogs | project Account)) and User in ((AuditLogs | project User))`
	metadata := newKQLNormalizationMetadata(query)
	normalized := normalizeWithMetadata(query, false, metadata)
	if normalized != NormalizeQueryForDebug(query) || len(metadata.subqueryOffsets) != 2 {
		t.Fatalf("normalization parity or generated provenance changed: %s / %#v", normalized, metadata)
	}
	for _, offset := range metadata.subqueryOffsets {
		if suffix := string([]rune(normalized)[offset:]); !strings.HasPrefix(suffix, `"SubqueryResult")`) {
			t.Errorf("generated marker rune offset points at authored source: %d: %s", offset, suffix)
		}
	}
	r := ExtractConditions(query)
	if len(r.Errors) != 0 || len(r.Conditions) != 5 || r.Conditions[0].Value != "__kql_in_subquery_0__" || r.Conditions[1].Value != "__kql_in_subquery_1__" || len(scalarLeaves(r.Expression)) != 1 {
		t.Fatalf("mixed generated/authored markers changed: %#v", r)
	}
	query += ` and 5 in ((Other | project n))`
	r = ExtractConditions(query)
	if len(r.Errors) != 1 || r.Errors[0] != "unsupported scalar predicate: unresolved IN subquery" || len(r.Conditions) != 0 || r.Expression != nil {
		t.Fatalf("actual synthetic scalar escaped guard: %#v", r)
	}
}
