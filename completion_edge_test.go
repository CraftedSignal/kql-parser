package kql

import (
	"strings"
	"testing"
)

func TestKQLCompletionReturnTypeTopNestedAndPortableEdges(t *testing.T) {
	evaluate := `T | evaluate plugin("a \" )", nested(1)) : (*, Slot:string, Count:long) | where Slot == "x"`
	evaluateWant := `T | evaluate plugin("a \" )", nested(1)) | where Slot == "x"`
	if got := stripReturnTypeAnnotations(evaluate); got != evaluateWant {
		t.Fatalf("stripReturnTypeAnnotations() = %q, want %q", got, evaluateWant)
	}
	nestedType := `T | evaluate plugin(1) : (*, Bag:dynamic(long)) | where Bag == "x"`
	nestedTypeWant := `T | evaluate plugin(1) | where Bag == "x"`
	if got := stripReturnTypeAnnotations(nestedType); got != nestedTypeWant {
		t.Fatalf("stripReturnTypeAnnotations(nested type) = %q, want %q", got, nestedTypeWant)
	}
	for _, malformed := range []string{
		`T | evaluate plugin("unterminated`,
		`T | evaluate plugin(1) : string | where x == 1`,
	} {
		if got := stripReturnTypeAnnotations(malformed); got != malformed {
			t.Fatalf("malformed return annotation changed to %q", got)
		}
	}

	standardTopNested := `T | top-nested Count by Field | where EventID == 4688`
	if got := normalizeTopNested(standardTopNested); got != standardTopNested {
		t.Fatalf("standard top-nested changed to %q", got)
	}

	commentedTopNested := "T | top-nested 5 of A by sum(X) | // comment before next clause\n top-nested 3 of B by count() | where EventID == 4688"
	normalizedTopNested := normalizeTopNested(commentedTopNested)
	if strings.Contains(strings.ToLower(normalizedTopNested), "top-nested") || !strings.Contains(normalizedTopNested, "where EventID == 4688") {
		t.Fatalf("top-nested chain not normalized/preserved: %q", normalizedTopNested)
	}
	trailingCommentTopNested := `T | top-nested 5 of A by count() | // trailing`
	if got := normalizeTopNested(trailingCommentTopNested); strings.Contains(strings.ToLower(got), "top-nested") {
		t.Fatalf("top-nested with trailing comment not normalized: %q", got)
	}

	if !hasPortableFieldConditions([]Condition{{Field: ""}, {Field: "Account"}}) {
		t.Fatalf("portable field condition after empty field was not detected")
	}
	if hasPortableFieldConditions([]Condition{{Field: ""}, {Field: "where"}}) {
		t.Fatalf("empty/non-portable-only conditions should not count")
	}
}

func TestKQLCompletionJoinSideFieldsAreJoinKeys(t *testing.T) {
	if got := ClassifyFieldProvenance(nil, "Account"); got != ProvenanceAmbiguous {
		t.Fatalf("nil result provenance = %q, want %q", got, ProvenanceAmbiguous)
	}

	result := &ParseResult{
		Joins: []JoinInfo{{
			PipeStage:   1,
			LeftFields:  []string{"LeftAccount"},
			RightFields: []string{"RightAccount"},
		}},
	}

	if got := ClassifyFieldProvenance(result, "LeftAccount"); got != ProvenanceJoinKey {
		t.Fatalf("left join field provenance = %q, want %q", got, ProvenanceJoinKey)
	}
	if got := ClassifyFieldProvenance(result, "RightAccount"); got != ProvenanceJoinKey {
		t.Fatalf("right join field provenance = %q, want %q", got, ProvenanceJoinKey)
	}
}

func TestKQLCompletionDatasourceBooleanDeclareAndSearchEdges(t *testing.T) {
	if isPortableDataSourceCandidate("NaN", nil) {
		t.Fatalf("numeric sentinel should not be a portable data source")
	}

	verbatimParts := splitBooleanConditions(`Field == @"a "" and "" b" and Other == "x"`)
	if len(verbatimParts) != 2 {
		t.Fatalf("verbatim string boolean split = %#v", verbatimParts)
	}
	escapedParts := splitBooleanConditions(`Field == "a \" and b" and Other == "x"`)
	if len(escapedParts) != 2 {
		t.Fatalf("escaped string boolean split = %#v", escapedParts)
	}

	declareInput := "declare query_parameters \n (x:dynamic = dynamic((1))) \n ;\nSecurityEvent | where EventID == 4688"
	if got, want := stripDeclareStatements(declareInput), "SecurityEvent | where EventID == 4688"; got != want {
		t.Fatalf("stripDeclareStatements(nested/whitespace) = %q, want %q", got, want)
	}
	malformedDeclare := "declare query_parameters"
	if got := stripDeclareStatements(malformedDeclare); got != malformedDeclare {
		t.Fatalf("malformed declare changed to %q", got)
	}

	malformedSearch := `search kind=case_sensitive`
	if got := normalizeSearchOperator(malformedSearch); got != malformedSearch {
		t.Fatalf("malformed search changed to %q", got)
	}
	if got, want := normalizeSearchOperator(`search in ((SecurityEvent)) | project Account`), "DummyTable | project Account"; got != want {
		t.Fatalf("nested search in normalization = %q, want %q", got, want)
	}
	if got := normalizeSearchOperator(`search kind=case_sensitive in (SecurityEvent)`); got != "DummyTable" {
		t.Fatalf("empty search pattern should collapse to DummyTable, got %q", got)
	}
}

func TestKQLCompletionNormalizerWrapperEdges(t *testing.T) {
	mvApplyWithoutOn := `T | mv-apply x to typeof(string) | where EventID == 4688`
	if got := stripMvApplySubquery(mvApplyWithoutOn); got != mvApplyWithoutOn {
		t.Fatalf("mv-apply without subquery changed to %q", got)
	}
	mvApplyWithoutBody := `T | mv-apply x on `
	if got := stripMvApplySubquery(mvApplyWithoutBody); got != mvApplyWithoutBody {
		t.Fatalf("mv-apply without body changed to %q", got)
	}
	mvApplyWithEscapedQuote := `T | mv-apply x on (print y="a \" )" | where z == 1) | where EventID == 4688`
	mvApplyStripped := stripMvApplySubquery(mvApplyWithEscapedQuote)
	if strings.Contains(strings.ToLower(mvApplyStripped), "mv-apply") || !strings.Contains(mvApplyStripped, "where EventID == 4688") {
		t.Fatalf("mv-apply subquery not stripped/preserved: %q", mvApplyStripped)
	}

	joinUnion := `T | join kind=inner ((union SecurityEvent, SigninLogs)) on Account`
	normalizedJoin := normalizeJoinUnion(joinUnion)
	if !strings.Contains(normalizedJoin, "DummyTable | union SecurityEvent") {
		t.Fatalf("join union was not normalized with dummy table: %q", normalizedJoin)
	}
	strippedHints := stripJoinHints(`T | join kind=inner hint.strategy=broadcast hint.shufflekey=(Account) ((union SecurityEvent, SigninLogs)) on Account`)
	if strings.Contains(strippedHints, "hint.strategy") || strings.Contains(strippedHints, "hint.shufflekey") {
		t.Fatalf("join hints not stripped: %q", strippedHints)
	}

	letStatements := extractLetStatements(`let IOCs = dynamic(["a"]); let IOCs = dynamic(["b"]); let Other = SecurityEvent;`)
	if len(letStatements) != 2 || letStatements[0].Name != "IOCs" || letStatements[0].Expression != `dynamic(["a"])` {
		t.Fatalf("duplicate let statements not deduped/preserved: %#v", letStatements)
	}
}

func TestKQLCompletionDataWrapperAndPartitionEdges(t *testing.T) {
	for _, unchanged := range []string{
		`datatable`,
		`datatable(x:string [ "a" ]`,
		`externaldata Field:string ["https://example.invalid/data.csv"]`,
		`externaldata(Field:string`,
	} {
		if got := replaceDatatableWithData(replaceExternalData(unchanged)); got != unchanged {
			t.Fatalf("malformed data wrapper changed to %q", got)
		}
	}

	datatableQuery := `datatable(x:string, y:dynamic(foo(bar))) [ "a [bracket]", dynamic({"v": ")"}) ] | where x == "a [bracket]"`
	if got := replaceDatatableWithData(datatableQuery); !strings.Contains(got, "DatatableResult | where x") {
		t.Fatalf("datatable wrapper not replaced/preserved: %q", got)
	}

	externalQuery := `externaldata(x:string)[@"https://example.invalid/[data].csv"] with(format="csv") | where x == "a"`
	if got := replaceExternalData(externalQuery); got != `ExtDataResult | where x == "a"` {
		t.Fatalf("externaldata wrapper = %q", got)
	}

	findQuery := "find where Account == 'a'\nproject Account"
	if got := normalizeFindOperator(findQuery); !strings.Contains(got, "DummyTable | where Account == 'a' |") || !strings.Contains(got, "project Account") {
		t.Fatalf("find newline project normalization = %q", got)
	}
	findTabProject := "find where Account == 'a'\tproject Account"
	if got := normalizeFindOperator(findTabProject); !strings.Contains(got, "DummyTable | where Account == 'a' |") || !strings.Contains(got, "project Account") {
		t.Fatalf("find tab project normalization = %q", got)
	}

	for _, unchanged := range []string{
		`T | partition hint.strategy=native (T | where EventID == 4688)`,
		`T | partition by (T | where EventID == 4688)`,
		`T | partition by Account`,
	} {
		if got := normalizePartitionOperator(unchanged); got != unchanged {
			t.Fatalf("malformed partition changed to %q", got)
		}
	}
	partitionQuery := `T | partition by Account (T | where EventID == 4688) | project Account`
	if got := normalizePartitionOperator(partitionQuery); got != `T | summarize count() by Account | project Account` {
		t.Fatalf("partition normalization = %q", got)
	}
}

func TestKQLCompletionQuotedPipeAndASIMEdges(t *testing.T) {
	asimNested := `_Im_Dns(pack("nested", dynamic({"v": ["x"]}))) | where SrcIpAddr == "1.2.3.4"`
	if got := replaceASIMFunctions(asimNested); got != `ASIMResult | where SrcIpAddr == "1.2.3.4"` {
		t.Fatalf("nested ASIM replacement = %q", got)
	}
	asimUnbalanced := `_Im_Dns(pack("nested", dynamic({"v": ["x"]})) | where SrcIpAddr == "1.2.3.4"`
	if got := replaceASIMFunctions(asimUnbalanced); got != asimUnbalanced {
		t.Fatalf("unbalanced ASIM call changed to %q", got)
	}

	segments := splitPipeSegments(`T | where Message == @"a ""|"" b" | where Other == "x\|y" | project Account`)
	if len(segments) != 4 {
		t.Fatalf("splitPipeSegments quoted/verbatim = %#v", segments)
	}
	topSegments := splitTopLevelPipeSegments(`T | join (U | where Message == @"a ""|"" b" and Other == "x\|y") on Id | project Id`)
	if len(topSegments) != 3 {
		t.Fatalf("splitTopLevelPipeSegments quoted/verbatim = %#v", topSegments)
	}

	if idx := indexOutsideQuotesAndParens(`a(@"=""", b) = c`, "="); idx != len(`a(@"=""", b) `) {
		t.Fatalf("indexOutsideQuotesAndParens verbatim = %d", idx)
	}
	if got := stripOuterParens(`(Account == "a") or (Account == "b")`); got != `(Account == "a") or (Account == "b")` {
		t.Fatalf("stripOuterParens unbalanced wrapper = %q", got)
	}
}
