package kql

import (
	"strings"
	"testing"
)

func TestKQLNormalizeEscapeSequencesEdges(t *testing.T) {
	input := "SecurityEvent\\n| where EventID == 4688\\tand Account == 'a\\nb'\\r| project Account"
	want := "SecurityEvent\n| where EventID == 4688\tand Account == 'a\\nb'\r| project Account"
	if got := normalizeEscapeSequences(input); got != want {
		t.Fatalf("normalizeEscapeSequences() = %q, want %q", got, want)
	}

	unicodeInput := `SecurityEvent\u00a0| where Message == \u201cblocked\u201d and User == \\u0041`
	unicodeWant := `SecurityEvent | where Message == "blocked" and User == A`
	if got := normalizeEscapeSequences(unicodeInput); got != unicodeWant {
		t.Fatalf("unicode escape normalization = %q, want %q", got, unicodeWant)
	}

	if isValidHex("123") {
		t.Fatalf("short hex sequence should be invalid")
	}
	if isValidHex("12x4") {
		t.Fatalf("non-hex sequence should be invalid")
	}

	var b strings.Builder
	handleUnicodeEscape("200b", &b)
	handleUnicodeEscape("202f", &b)
	handleUnicodeEscape("201c", &b)
	handleUnicodeEscape("2019", &b)
	handleUnicodeEscape("0041", &b)
	if got, want := b.String(), " \"'A"; got != want {
		t.Fatalf("handleUnicodeEscape() = %q, want %q", got, want)
	}
}

func TestKQLNormalizeUnicodeWhitespaceEdges(t *testing.T) {
	input := "SecurityEvent\u00a0| where Message == \"a\u00a0b\" and Account == 'x\u200by'\u200b"
	want := "SecurityEvent | where Message == \"a\u00a0b\" and Account == 'x\u200by'"
	if got := normalizeUnicodeWhitespace(input); got != want {
		t.Fatalf("normalizeUnicodeWhitespace() = %q, want %q", got, want)
	}

	commented := "SecurityEvent // keep\u00a0comment\n| where EventID == 4688"
	if got := normalizeUnicodeWhitespace(commented); got != commented {
		t.Fatalf("line comments should be preserved, got %q", got)
	}
}

func TestKQLStripReturnTypeAnnotationsEdges(t *testing.T) {
	input := `T | evaluate bag_unpack(Properties) : (*, Account:string, Count:int) | where Account == "x"`
	want := `T | evaluate bag_unpack(Properties) | where Account == "x"`
	if got := stripReturnTypeAnnotations(input); got != want {
		t.Fatalf("stripReturnTypeAnnotations() = %q, want %q", got, want)
	}

	unchanged := `T | evaluate bag_unpack(Properties) | where Account == "x"`
	if got := stripReturnTypeAnnotations(unchanged); got != unchanged {
		t.Fatalf("query without return annotation changed to %q", got)
	}

	incomplete := `T | evaluate bag_unpack(Properties) : (*, Account:string`
	if got := stripReturnTypeAnnotations(incomplete); got != incomplete {
		t.Fatalf("unterminated annotation changed to %q", got)
	}
}

func TestKQLLookupUnionASIMAndDeclareNormalizationEdges(t *testing.T) {
	lookupInput := `T | where x in (lookup (union A, B)) | project x`
	lookupWant := `T | where x in (lookup (LookupTable)) | project x`
	if got := normalizeLookupSubquery(lookupInput); got != lookupWant {
		t.Fatalf("normalizeLookupSubquery() = %q, want %q", got, lookupWant)
	}

	lookupUnchanged := `T | where x in (lookup (Watchlist))`
	if got := normalizeLookupSubquery(lookupUnchanged); got != lookupUnchanged {
		t.Fatalf("lookup without union changed to %q", got)
	}

	unionInput := `(union isfuzzy=true withsource=TableName kind=outer SecurityEvent, SigninLogs) | where EventID == 4688`
	union := normalizeUnionParameters(unionInput)
	unionLower := strings.ToLower(union)
	if !strings.HasPrefix(strings.TrimSpace(unionLower), "union ") {
		t.Fatalf("outer union parentheses were not removed: %q", union)
	}
	for _, removed := range []string{"isfuzzy", "withsource", "kind=outer"} {
		if strings.Contains(unionLower, removed) {
			t.Fatalf("union parameter %q was not removed from %q", removed, union)
		}
	}
	if !strings.Contains(union, "SecurityEvent") || !strings.Contains(union, "SigninLogs") {
		t.Fatalf("union sources were lost: %q", union)
	}

	asimCall := `_Im_Dns(responsecodename="NOERROR") | where SrcIpAddr == "1.2.3.4"`
	if got, want := replaceASIMFunctions(asimCall), `ASIMResult | where SrcIpAddr == "1.2.3.4"`; got != want {
		t.Fatalf("ASIM function call replacement = %q, want %q", got, want)
	}
	asimBare := `imAuthentication | where EventResult == "Success"`
	if got, want := replaceASIMFunctions(asimBare), `ASIMResult | where EventResult == "Success"`; got != want {
		t.Fatalf("bare ASIM replacement = %q, want %q", got, want)
	}
	partial := `my_imAuthentication_suffix | where EventResult == "Success"`
	if got := replaceASIMFunctions(partial); got != partial {
		t.Fatalf("partial ASIM identifier changed to %q", got)
	}

	declared := "declare query_parameters(account:string = 'x');\nSecurityEvent | where Account == account"
	if got, want := stripDeclareStatements(declared), "SecurityEvent | where Account == account"; got != want {
		t.Fatalf("stripDeclareStatements() = %q, want %q", got, want)
	}
	unterminatedDeclare := `declare query_parameters(account:string`
	if got := stripDeclareStatements(unterminatedDeclare); got != "" {
		t.Fatalf("unterminated declare changed to %q", got)
	}
}

func TestKQLTemplateParameterNormalizationEdges(t *testing.T) {
	doubleCurly := `SecurityEvent | where Account == {{AccountName}} and Message == "{{Literal}}" and Other == '{{AlsoLiteral}}'`
	doubleCurlyWant := `SecurityEvent | where Account == {AccountName} and Message == "{{Literal}}" and Other == '{{AlsoLiteral}}'`
	if got := collapseDoubleCurlyParametersOutsideStrings(doubleCurly); got != doubleCurlyWant {
		t.Fatalf("double-curly collapse = %q, want %q", got, doubleCurlyWant)
	}

	unterminated := `SecurityEvent | where Account == {{AccountName`
	if got := collapseDoubleCurlyParametersOutsideStrings(unterminated); got != unterminated {
		t.Fatalf("unterminated double-curly parameter changed to %q", got)
	}
	noParams := `SecurityEvent | where Message == "{{Literal}}"`
	if got := collapseDoubleCurlyParametersOutsideStrings(noParams); got != noParams {
		t.Fatalf("quoted double-curly parameter changed to %q", got)
	}

	generic := `SecurityEvent | where Account == {AccountName} and TimeGenerated {TimeRange} and Message == "{Literal}"`
	genericWant := `SecurityEvent | where Account == DummyParam and TimeGenerated > ago(1d) and Message == "{Literal}"`
	if got := replaceGenericParametersOutsideStrings(generic); got != genericWant {
		t.Fatalf("generic parameter replacement = %q, want %q", got, genericWant)
	}

	invalid := `SecurityEvent | where Account == {not valid}`
	if got := replaceGenericParametersOutsideStrings(invalid); got != invalid {
		t.Fatalf("invalid generic parameter changed to %q", got)
	}
}

func TestKQLStripNamedFunctionParamsEdges(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "empty query",
			query: "",
			want:  "",
		},
		{
			name:  "non identifier start",
			query: `| where Account == "alice"`,
			want:  `| where Account == "alice"`,
		},
		{
			name:  "no opening paren",
			query: `SecurityEvent | where Account == "alice"`,
			want:  `SecurityEvent | where Account == "alice"`,
		},
		{
			name:  "unnamed params stay intact",
			query: `parser("literal", iff(x == 1, "=", "")) | where Account == "alice"`,
			want:  `parser("literal", iff(x == 1, "=", "")) | where Account == "alice"`,
		},
		{
			name:  "unbalanced params stay intact",
			query: `parser(account="alice" | where Account == "alice"`,
			want:  `parser(account="alice" | where Account == "alice"`,
		},
		{
			name:  "named params stripped",
			query: `parser(pack=true, filter=iff(x == 1, "a=b", "c")) | where Account == "alice"`,
			want:  `parser | where Account == "alice"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripNamedFunctionParams(tc.query); got != tc.want {
				t.Fatalf("stripNamedFunctionParams() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestKQLAdditionalNormalizationEdges(t *testing.T) {
	distinct := normalizeDistinctColumnList(`User, tostring(Device.Name), Alias = Account, strcat(a,b), "literal"`)
	if want := `User, Name, Account, strcat(a,b), "literal"`; distinct != want {
		t.Fatalf("normalizeDistinctColumnList() = %q, want %q", distinct, want)
	}

	rendered := stripRenderParameters(`T | render areachart kind=stacked | where EventID == 4688`)
	if want := `T | render areachart| where EventID == 4688`; rendered != want {
		t.Fatalf("stripRenderParameters() = %q, want %q", rendered, want)
	}
	renderWithClause := `T | render timechart with (title="x")`
	if got := stripRenderParameters(renderWithClause); got != renderWithClause {
		t.Fatalf("render with-clause should be preserved, got %q", got)
	}

	decimals := normalizeTrailingDecimals(`T | where Amount > 1000. and Text == "1." and Version == 1..2 and Name == abc1.`)
	if want := `T | where Amount > 1000.0 and Text == "1." and Version == 1..2 and Name == abc1.`; decimals != want {
		t.Fatalf("normalizeTrailingDecimals() = %q, want %q", decimals, want)
	}
	if !isStandaloneNumericLiteralBeforeDot(`1000.`, 4) || isStandaloneNumericLiteralBeforeDot(`abc1000.`, 7) {
		t.Fatalf("standalone numeric literal boundary handling failed")
	}
	if !isTrailingDecimalBoundary(`1.|`, 2) || isTrailingDecimalBoundary(`1.a`, 2) {
		t.Fatalf("trailing decimal boundary handling failed")
	}

	nulls := normalizeNullEscapes(`T | where A == "\0" and B == @"\0" and C == "\012"`)
	if want := `T | where A == "\x00" and B == @"\0" and C == "\012"`; nulls != want {
		t.Fatalf("normalizeNullEscapes() = %q, want %q", nulls, want)
	}
}

func TestKQLSubqueryJoinUnionAndSourceNormalizationEdges(t *testing.T) {
	inSubquery := normalizeInSubqueries(`T | where Account in (SigninLogs | project Account) and Message == "a|b"`)
	if want := `T | where Account in (SubqueryResult) and Message == "a|b"`; inSubquery != want {
		t.Fatalf("normalizeInSubqueries() = %q, want %q", inSubquery, want)
	}
	verbatimInSubquery := normalizeInSubqueries(`T | where Message == @"literal "" in (SigninLogs | project Account)" and Account in (SigninLogs | project Account)`)
	if want := `T | where Message == @"literal "" in (SigninLogs | project Account)" and Account in (SubqueryResult)`; verbatimInSubquery != want {
		t.Fatalf("normalizeInSubqueries() with verbatim string = %q, want %q", verbatimInSubquery, want)
	}
	malformedInSubquery := `T | where Account in (SigninLogs | project Account`
	if got := normalizeInSubqueries(malformedInSubquery); got != malformedInSubquery {
		t.Fatalf("normalizeInSubqueries() changed malformed input to %q", got)
	}
	if containsPipeOutsideStrings(`"a|b"`) || !containsPipeOutsideStrings(`"a|b" | project x`) {
		t.Fatalf("containsPipeOutsideStrings did not respect quoted pipes")
	}
	operatorAlias := normalizeStringOperatorAliases(`T | where Message == @"literal "" containscs !containscs" and Text containscs "ERR" and Other !containscs "debug"`)
	if want := `T | where Message == @"literal "" containscs !containscs" and Text contains_cs "ERR" and Other !contains_cs "debug"`; operatorAlias != want {
		t.Fatalf("normalizeStringOperatorAliases() with verbatim string = %q, want %q", operatorAlias, want)
	}

	joinUnion := normalizeJoinUnion(`T | join kind=inner hint.strategy=broadcast (union A, B) on Id`)
	if want := `T | join kind=inner hint.strategy=broadcast (DummyTable | union A, B) on Id`; joinUnion != want {
		t.Fatalf("normalizeJoinUnion() = %q, want %q", joinUnion, want)
	}
	if got := stripJoinHints(`T | join hint.strategy=(broadcast) hint.remote=auto (U) on Id`); strings.Contains(got, "hint.") {
		t.Fatalf("stripJoinHints() left hints in %q", got)
	}
	malformedHint := `T | join hint.strategy (U) on Id`
	if got := stripJoinHints(malformedHint); got != malformedHint {
		t.Fatalf("stripJoinHints() changed malformed hint to %q", got)
	}
	if got := stripJoinHints(`T | join hint.strategy=   broadcast (U) on Id`); strings.Contains(got, "hint.") || strings.Contains(got, "broadcast") {
		t.Fatalf("stripJoinHints() left spaced hint value in %q", got)
	}
	if got := stripJoinHints(`T | join hint.shufflekey=(hash(Account, tostring(DeviceId))) (U) on Id`); strings.Contains(got, "hint.") || strings.Contains(got, "hash(") {
		t.Fatalf("stripJoinHints() left nested parenthesized hint in %q", got)
	}

	unionWildcard := normalizeUnionParameters(`union withsource=TableName kind=outer Device*, *_CL, * | where x == 1`)
	for _, notWant := range []string{"withsource", "kind=outer", "Device*", "*_CL", " * "} {
		if strings.Contains(unionWildcard, notWant) {
			t.Fatalf("normalizeUnionParameters() left %q in %q", notWant, unionWildcard)
		}
	}
	if !strings.Contains(unionWildcard, "AllTables") {
		t.Fatalf("union wildcard was not replaced: %q", unionWildcard)
	}
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "nested union context is ignored",
			input: `union (A, *) | where x == 1`,
			want:  `union (A, *) | where x == 1`,
		},
		{
			name:  "unmatched close paren breaks union context",
			input: `union A), * | where x == 1`,
			want:  `union A), * | where x == 1`,
		},
		{
			name:  "pipe before star breaks union context",
			input: `union A | where x == *`,
			want:  `union A | where x == *`,
		},
		{
			name:  "balanced parens before top-level wildcard",
			input: `union Func(x), * | where x == 1`,
			want:  `union Func(x), AllTables | where x == 1`,
		},
	} {
		t.Run("replace_union_wildcard_"+tt.name, func(t *testing.T) {
			if got := replaceUnionWildcard(tt.input); got != tt.want {
				t.Fatalf("replaceUnionWildcard() = %q, want %q", got, tt.want)
			}
		})
	}

	unionFuncs := stripUnionFunctionParams(`union Func("a,b"), Other(flag=true), Plain | where x == 1`)
	if want := `union Func, Other, Plain | where x == 1`; unionFuncs != want {
		t.Fatalf("stripUnionFunctionParams() = %q, want %q", unionFuncs, want)
	}
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "nested calls and quoted commas",
			input: `union Func(pack("k", strcat("a,b", "c"))), Other('x,y') | where x == 1`,
			want:  `union Func, Other | where x == 1`,
		},
		{
			name:  "union params are not function operands",
			input: `union isfuzzy=true, withsource=TableName Func("x"), Plain | where x == 1`,
			want:  `union isfuzzy=true, withsource=TableName Func, Plain | where x == 1`,
		},
		{
			name:  "no union keyword",
			input: `SecurityEvent | where Message has "Func(\"x\")"`,
			want:  `SecurityEvent | where Message has "Func(\"x\")"`,
		},
		{
			name:  "union-like identifier is ignored",
			input: `unionized Func("x") | where x == 1`,
			want:  `unionized Func("x") | where x == 1`,
		},
		{
			name:  "bare union keyword",
			input: `union`,
			want:  `union`,
		},
		{
			name:  "quoted union operands",
			input: `union "Table,\"Name", 'Other,Table', Func("x") | where x == 1`,
			want:  `union "Table,\"Name", 'Other,Table', Func | where x == 1`,
		},
	} {
		t.Run("strip_union_function_params_"+tt.name, func(t *testing.T) {
			if got := stripUnionFunctionParams(tt.input); got != tt.want {
				t.Fatalf("stripUnionFunctionParams() = %q, want %q", got, tt.want)
			}
		})
	}

	makeSeriesRange := normalizeMakeSeries(`T | make-series count() on TimeGenerated in range(ago(1d), now(), 1h) by Computer`)
	if !strings.Contains(makeSeriesRange, `from ago(1d) to now() step 1h`) {
		t.Fatalf("make-series range was not normalized: %q", makeSeriesRange)
	}
	makeSeriesDefault := normalizeMakeSeries(`T | make-series count() on TimeGenerated by Computer`)
	if !strings.Contains(makeSeriesDefault, `step 1h by Computer`) {
		t.Fatalf("make-series default step was not inserted: %q", makeSeriesDefault)
	}
	parts := splitByTopLevelComma(`ago(1d), iff(x, y, z), now()`)
	if len(parts) != 3 || strings.TrimSpace(parts[1]) != `iff(x, y, z)` {
		t.Fatalf("splitByTopLevelComma() = %#v", parts)
	}

	data := replaceDatatableWithData(`datatable(User:string, Count:int) ["alice", 1] | where User == "alice"`)
	if want := `DatatableResult | where User == "alice"`; data != want {
		t.Fatalf("replaceDatatableWithData() = %q, want %q", data, want)
	}
	argQualified := replaceArgFunction(`arg("sub").Resources | where type == "x"`)
	if want := `Resources | where type == "x"`; argQualified != want {
		t.Fatalf("replaceArgFunction qualified = %q, want %q", argQualified, want)
	}
	argBare := replaceArgFunction(`arg("sub") | where type == "x"`)
	if want := `ArgResult | where type == "x"`; argBare != want {
		t.Fatalf("replaceArgFunction bare = %q, want %q", argBare, want)
	}
	external := replaceExternalData(`externaldata (User:string) [@"https://example.invalid/users.csv"] with(format="csv") | where User == "alice"`)
	if want := `ExtDataResult | where User == "alice"`; external != want {
		t.Fatalf("replaceExternalData() = %q, want %q", external, want)
	}
}

func TestKQLPreambleCommentAndProvenanceEdges(t *testing.T) {
	doc := "Description: narrative text\nKQL Query: SecurityEvent\n| where EventID == 4688"
	if got, want := stripDocumentationPreamble(doc), "SecurityEvent\n| where EventID == 4688"; got != want {
		t.Fatalf("stripDocumentationPreamble() = %q, want %q", got, want)
	}
	inlineDoc := "KQL: SecurityEvent | where EventID == 4688"
	if got, want := stripDocumentationPreamble(inlineDoc), "SecurityEvent | where EventID == 4688"; got != want {
		t.Fatalf("stripDocumentationPreamble() inline = %q, want %q", got, want)
	}
	parenthesizedDoc := "Description: query exported from docs\n# source link\n(union SecurityEvent, DeviceEvents)\n| where EventID == 4688"
	if got, want := stripDocumentationPreamble(parenthesizedDoc), "(union SecurityEvent, DeviceEvents)\n| where EventID == 4688"; got != want {
		t.Fatalf("stripDocumentationPreamble() parenthesized = %q, want %q", got, want)
	}
	comments := "// heading\nhttps://example.invalid/doc\nSecurityEvent\n/| commented pipe\n| where EventID == 4688"
	if got, want := stripLeadingComments(comments, false), "SecurityEvent\n| where EventID == 4688"; got != want {
		t.Fatalf("stripLeadingComments() = %q, want %q", got, want)
	}

	result := &ParseResult{
		Conditions:     []Condition{{Field: "EventID", PipeStage: 0}, {Field: "JoinedField", PipeStage: 3}},
		ComputedFields: map[string]string{"extendedfield": "EventID"},
		Joins: []JoinInfo{{
			PipeStage:     2,
			JoinFields:    []string{"Account"},
			LeftFields:    []string{"Host"},
			RightFields:   []string{"Device"},
			ExposedFields: []string{"JoinedField"},
		}},
	}
	if got := ClassifyFieldProvenance(result, "Account"); got != ProvenanceJoinKey {
		t.Fatalf("join field provenance = %q", got)
	}
	if got := ClassifyFieldProvenance(result, "EventID"); got != ProvenanceMain {
		t.Fatalf("main field provenance = %q", got)
	}
	if got := ClassifyFieldProvenance(result, "ExtendedField"); got != ProvenanceMain {
		t.Fatalf("computed field provenance = %q", got)
	}
	if got := ClassifyFieldProvenance(result, "JoinedField"); got != ProvenanceJoined {
		t.Fatalf("joined field provenance = %q", got)
	}
	if got := ClassifyFieldProvenance(result, "Unknown"); got != ProvenanceAmbiguous {
		t.Fatalf("unknown field provenance = %q", got)
	}
}

func TestKQLOperatorNormalizationEdges(t *testing.T) {
	if got := getLastWord("where TimeGenerated"); got != "TimeGenerated" {
		t.Fatalf("getLastWord() = %q", got)
	}
	if got := getLastWord("   "); got != "" {
		t.Fatalf("empty getLastWord() = %q", got)
	}

	find := normalizeFindOperator(`find in (SecurityEvent, SigninLogs) where EventID == 4688 project Account`)
	if want := `DummyTable | where EventID == 4688 | project Account`; find != want {
		t.Fatalf("normalizeFindOperator() = %q, want %q", find, want)
	}
	if got := normalizeFindOperator(`find "needle"`); got != "DummyTable" {
		t.Fatalf("find without where = %q", got)
	}

	topNested := normalizeTopNested(`T | top-nested 5 of Account by count() | top-nested 3 of Computer by count() | where Account == "a"`)
	if strings.Contains(strings.ToLower(topNested), "top-nested") ||
		!strings.Contains(topNested, "summarize count()") ||
		!strings.Contains(topNested, `| where Account == "a"`) {
		t.Fatalf("normalizeTopNested() = %q", topNested)
	}

	partition := normalizePartitionOperator(`T | partition by Computer (where EventID == 4688 | summarize count()) | where Computer != ""`)
	if strings.Contains(strings.ToLower(partition), "partition") ||
		!strings.Contains(partition, `| summarize count() by Computer`) ||
		!strings.Contains(partition, `| where Computer != ""`) {
		t.Fatalf("normalizePartitionOperator() = %q", partition)
	}

	searchWithPipeline := normalizeSearchOperator(`search kind=case_sensitive in (SecurityEvent) | project Account`)
	if want := `DummyTable | project Account`; searchWithPipeline != want {
		t.Fatalf("normalizeSearchOperator() = %q, want %q", searchWithPipeline, want)
	}
	if got := normalizeSearchOperator(`search kind=case_sensitive in (SecurityEvent) "admin" | project Account`); got != "DummyTable" {
		t.Fatalf("search pattern should collapse to dummy table, got %q", got)
	}
	if got := normalizeSearchOperator(`search "admin"`); got != "DummyTable" {
		t.Fatalf("search without pipeline = %q", got)
	}
	if original := `SecurityEvent | search "admin"`; normalizeSearchOperator(original) != original {
		t.Fatalf("non-leading search operator should be preserved")
	}
}
