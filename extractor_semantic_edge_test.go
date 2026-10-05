package kql

import "testing"

func kqlExpressionListForTest(t *testing.T, expressionList string) IExpressionListContext {
	t.Helper()
	return kqlParserForTest(t, expressionList).ExpressionList()
}

func requireKQLCondition(t *testing.T, conditions []Condition, field, op, value string, negated bool) Condition {
	t.Helper()
	for _, condition := range conditions {
		if condition.Field == field && condition.Operator == op && condition.Value == value && condition.Negated == negated {
			return condition
		}
	}
	t.Fatalf("missing condition field=%q op=%q value=%q negated=%v in %#v", field, op, value, negated, conditions)
	return Condition{}
}

func TestKQLConditionHandlerSemanticEdges(t *testing.T) {
	extractor := &conditionExtractor{
		computedFields:      map[string]string{"derived": "RawField"},
		computedExpressions: map[string]string{},
		lastLogicalOp:       "OR",
		currentStage:        2,
	}

	extractor.handleComparison("derived", "=~", `"Admin"`)
	derived := requireKQLCondition(t, extractor.conditions, "derived", "=~", "Admin", false)
	if !derived.IsComputed || derived.SourceField != "RawField" || derived.LogicalOp != "OR" {
		t.Fatalf("computed comparison metadata not preserved: %#v", derived)
	}
	extractor.handleComparison("Account", "==", "OtherAccount")
	ref := requireKQLCondition(t, extractor.conditions, "Account", "==", "", false)
	if ref.ValueReference != "OtherAccount" {
		t.Fatalf("field reference comparison not preserved: %#v", ref)
	}
	before := len(extractor.conditions)
	extractor.handleComparison("where", "==", `"ignored"`)
	extractor.handleComparison("not-valid!", "==", `"ignored"`)
	if len(extractor.conditions) != before {
		t.Fatalf("invalid comparison fields were not ignored: %#v", extractor.conditions[before:])
	}

	inList := kqlExpressionListForTest(t, `"alice", "bob"`)
	extractor.handleInOperator("Account", inList, "in~", true)
	inCond := requireKQLCondition(t, extractor.conditions, "Account", "in~", "alice", true)
	if len(inCond.Alternatives) != 2 || inCond.Alternatives[1] != "bob" {
		t.Fatalf("IN alternatives not preserved: %#v", inCond)
	}
	extractor.handleInOperator("AccountRef", kqlExpressionListForTest(t, `OtherAccount`), "in", false)
	refIn := requireKQLCondition(t, extractor.conditions, "AccountRef", "in", "OtherAccount", false)
	if refIn.ValueReference != "OtherAccount" {
		t.Fatalf("single identifier IN did not record value reference: %#v", refIn)
	}

	extractor.handleBetweenOperator("Score", "10", "20", false)
	requireKQLCondition(t, extractor.conditions, "Score", ">=", "10", false)
	requireKQLCondition(t, extractor.conditions, "Score", "<=", "20", false)

	extractor.handleHasAnyAllOperator("CommandLine", "has_all", kqlExpressionListForTest(t, `"cmd", "powershell"`))
	first := requireKQLCondition(t, extractor.conditions, "CommandLine", "has", "cmd", false)
	second := requireKQLCondition(t, extractor.conditions, "CommandLine", "has", "powershell", false)
	if first.LogicalOp != "AND" || second.LogicalOp != "AND" {
		t.Fatalf("has_all should connect terms with AND: %#v %#v", first, second)
	}
}

func TestKQLSourceAndPortablePredicateEdges(t *testing.T) {
	query := `
let IOCs = dynamic(["bad.exe", "evil.exe"]);
let Derived = SecurityEvent | where EventID == 4688;
union withsource=TableName SecurityEvent, Derived, cluster("c").database("d").SigninLogs
| join kind=inner (DeviceEvents | where ActionType == "ProcessCreated") on Account
| where FileName in (IOCs) and set_has_element(CommandLine, "cmd") and Timestamp > ago(1d)
`
	sources := extractPortableDataSources(query, normalizeUnionParameters(query))
	for _, want := range []string{"SecurityEvent", "SigninLogs", "DeviceEvents"} {
		if !containsKQLString(sources, want) {
			t.Fatalf("source %q missing from %#v", want, sources)
		}
	}
	if containsKQLString(sources, "Derived") {
		t.Fatalf("let-bound table should not be reported as source: %#v", sources)
	}

	statements := extractLetStatements(query + "\nlet IOCs = dynamic([\"dupe\"]);")
	if len(statements) != 2 || statements[0].Name != "IOCs" || statements[1].Name != "Derived" {
		t.Fatalf("let statements not deduplicated/preserved: %#v", statements)
	}
	if !hasPortableFieldConditions([]Condition{{Field: "where"}, {Field: "Computed", IsComputed: true}, {Field: "Account"}}) {
		t.Fatalf("portable field condition was not detected")
	}
	if hasPortableFieldConditions([]Condition{{Field: "where"}, {Field: "Computed", IsComputed: true}}) {
		t.Fatalf("non-portable/computed-only conditions should not count")
	}

	cond, ok := parsePortablePredicate(`FileName in (IOCs)`, map[string][]string{"iocs": {"bad.exe", "evil.exe"}})
	if !ok || cond.Value != "bad.exe" || cond.ValueReference != "IOCs" || len(cond.Alternatives) != 2 {
		t.Fatalf("portable list predicate not preserved: %#v ok=%v", cond, ok)
	}
	funcCond, ok := parsePortablePredicate(`set_has_element(CommandLine, "cmd")`, nil)
	if !ok || funcCond.Field != "CommandLine" || funcCond.Operator != "has" || funcCond.Value != "cmd" {
		t.Fatalf("portable function predicate not preserved: %#v ok=%v", funcCond, ok)
	}
	if _, ok := parsePortablePredicate(`Timestamp > ago(1d)`, nil); ok {
		t.Fatalf("time window predicate should not become portable detection condition")
	}
}

func TestKQLScannerAndValueHelperEdges(t *testing.T) {
	statements := splitStatements(`let A = dynamic(["a;b"]); let B = (T | where x == ";"); T | where y == "z"`)
	if len(statements) != 3 {
		t.Fatalf("splitStatements() = %#v", statements)
	}
	verbatimStatements := splitStatements(`let A = @"literal "";""; still string"; T | where y == "z"`)
	if len(verbatimStatements) != 2 {
		t.Fatalf("splitStatements() with verbatim semicolon = %#v", verbatimStatements)
	}
	if got := findMatchingParen(`(@"a)""b", (nested)) tail`, 0); got != len(`(@"a)""b", (nested))`)-1 {
		t.Fatalf("findMatchingParen with verbatim string = %d", got)
	}
	if got := findMatchingParen(`("a\")", (nested)) tail`, 0); got != len(`("a\")", (nested))`)-1 {
		t.Fatalf("findMatchingParen with escaped quote = %d", got)
	}
	if got := findMatchingParen(`not a group`, 0); got != -1 {
		t.Fatalf("invalid paren start returned %d", got)
	}

	if source := leadingDataSource(`where EventID == 4688`, nil); source != "" {
		t.Fatalf("operator should not be a data source: %q", source)
	}
	if source := cleanDataSourceToken(`cluster.database.SecurityEvent`, nil); source != "SecurityEvent" {
		t.Fatalf("clean data source token = %q", source)
	}
	letNames := map[string]bool{"derived": true}
	if isPortableDataSourceCandidate("Derived", letNames) || isPortableDataSourceCandidate("AllTables", nil) || isPortableDataSourceCandidate("123", nil) {
		t.Fatalf("non-portable data source candidate accepted")
	}

	parts := splitPipeSegments(`T | where Message == "a|b" | project Account`)
	if len(parts) != 3 {
		t.Fatalf("splitPipeSegments() = %#v", parts)
	}
	top := splitTopLevelPipeSegments(`T | join (U | where x == "a|b") on Id | project Id`)
	if len(top) != 3 || top[1] != ` join (U | where x == "a|b") on Id ` {
		t.Fatalf("splitTopLevelPipeSegments() = %#v", top)
	}
	booleanParts := splitBooleanConditions(`(Account == "a" and (CommandLine contains "cmd" or CommandLine contains "pwsh"))`)
	if len(booleanParts) != 3 {
		t.Fatalf("splitBooleanConditions() = %#v", booleanParts)
	}

	for raw, want := range map[string]string{
		`"quoted"`:   "quoted",
		`'single'`:   "single",
		`@"verb"`:    "verb",
		`@'vsingle'`: "vsingle",
		`bare`:       "bare",
	} {
		if got := extractValue(raw); got != want {
			t.Fatalf("extractValue(%q) = %q, want %q", raw, got, want)
		}
	}
	if !kqlValueIsFieldReference("OtherField", "OtherField") || kqlValueIsFieldReference(`"OtherField"`, "OtherField") || kqlValueIsFieldReference("true", "true") {
		t.Fatalf("field-reference detection failed")
	}
	if values, ok := parseListValue(`dynamic(["a", nested, 5])`, map[string][]string{"nested": {"b", "c"}}); !ok || len(values) != 4 || values[1] != "b" {
		t.Fatalf("parseListValue() = %#v ok=%v", values, ok)
	}
	if value, rest, ok := parseStringLiteral(`@"a""b" tail`); !ok || value != `a"b` || rest != " tail" {
		t.Fatalf("parseStringLiteral() value=%q rest=%q ok=%v", value, rest, ok)
	}
	if idx := indexOutsideQuotesAndParens(`a("=", b) = c`, "="); idx != len(`a("=", b) `) {
		t.Fatalf("indexOutsideQuotesAndParens() = %d", idx)
	}
	if got := stripOuterParens(`((Account == "a"))`); got != `Account == "a"` {
		t.Fatalf("stripOuterParens() = %q", got)
	}
	if got := compactKeyword("  one   two\nthree  "); got != "one two three" {
		t.Fatalf("compactKeyword() = %q", got)
	}
	if values := extractExpressionListValues(kqlExpressionListForTest(t, `"a", @"b", 3`)); len(values) != 3 || values[1] != "b" {
		t.Fatalf("extractExpressionListValues() = %#v", values)
	}
}

func containsKQLString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
