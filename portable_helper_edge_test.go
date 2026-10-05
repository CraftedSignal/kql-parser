package kql

import "testing"

func TestKQLPortableHelperFailureAndReferenceEdges(t *testing.T) {
	lists := map[string][]string{
		"iocs":  {"bad.exe", "evil.exe"},
		"empty": {},
	}

	condition, ok := parsePortableFunctionPredicate(`set_has_element(CommandLine, (IOCs))`, lists)
	if !ok || condition.Field != "CommandLine" || condition.Operator != "has" || condition.Value != "bad.exe" || condition.ValueReference != "IOCs" || len(condition.Alternatives) != 2 {
		t.Fatalf("set_has_element list reference not preserved: %#v ok=%v", condition, ok)
	}

	for _, expr := range []string{
		`unknown(CommandLine, "cmd")`,
		`set_has_element(CommandLine)`,
		`set_has_element(CommandLine, OtherField)`,
		`set_has_element(tostring(CommandLine), "cmd")`,
		`set_has_element(CommandLine, "cmd", "extra")`,
		`set-has-element(CommandLine, "cmd")`,
		`set_has_element(CommandLine, "cmd") trailing`,
	} {
		if condition, ok := parsePortableFunctionPredicate(expr, lists); ok {
			t.Fatalf("unsupported function predicate %q produced %#v", expr, condition)
		}
	}

	for _, expr := range []string{
		``,
		`SecurityEvent | where Account == "admin"`,
		`Timestamp > ago(1d)`,
		`tolower(Account) == "admin"`,
		`Account == OtherField`,
	} {
		if condition, ok := parsePortablePredicate(expr, lists); ok {
			t.Fatalf("non-portable predicate %q produced %#v", expr, condition)
		}
	}

	boolCond, ok := parsePortablePredicate(`IsAdmin == true`, lists)
	if !ok || boolCond.Value != "true" {
		t.Fatalf("boolean portable predicate not parsed: %#v ok=%v", boolCond, ok)
	}
	numberCond, ok := parsePortablePredicate(`RiskScore >= 10.5`, lists)
	if !ok || numberCond.Value != "10.5" {
		t.Fatalf("numeric portable predicate not parsed: %#v ok=%v", numberCond, ok)
	}

	if value, alternatives, ok := parsePortableValue(`(IOCs)`, lists); !ok || value != "bad.exe" || len(alternatives) != 2 {
		t.Fatalf("parenthesized list reference value not expanded: value=%q alternatives=%#v ok=%v", value, alternatives, ok)
	}
	if value, alternatives, ok := parsePortableValue(`dynamic(["a,b", @'c''d', nested(1, 2), IOCs])`, lists); !ok || value != "a,b" || len(alternatives) != 4 || alternatives[1] != "c'd" || alternatives[3] != "evil.exe" {
		t.Fatalf("dynamic list values not preserved: value=%q alternatives=%#v ok=%v", value, alternatives, ok)
	}
	for _, raw := range []string{"", "Empty", "OtherField", "(OtherField)"} {
		if value, alternatives, ok := parsePortableValue(raw, lists); ok {
			t.Fatalf("invalid portable value %q produced value=%q alternatives=%#v", raw, value, alternatives)
		}
	}

	if got := portableValueReference(`(IOCs);`, lists); got != "IOCs" {
		t.Fatalf("parenthesized value reference = %q", got)
	}
	for _, raw := range []string{"", "OtherField", "(OtherField)", `"IOCs"`} {
		if got := portableValueReference(raw, lists); got != "" {
			t.Fatalf("unexpected value reference for %q: %q", raw, got)
		}
	}
}

func TestKQLStringAndDelimiterScannerEdges(t *testing.T) {
	if field := extractFirstFieldFromExpression(nil); field != "" {
		t.Fatalf("nil expression field = %q", field)
	}
	for expr, want := range map[string]string{
		`where`:                          "",
		`""`:                             "",
		`strcat("literal", AccountName)`: "AccountName",
		`tostring(split(SenderMailFromAddress, "@")[-1])`: "SenderMailFromAddress",
		`case(EventID == 4688, AccountName, "")`:          "EventID",
	} {
		if got := firstFieldInExpressionText(expr); got != want {
			t.Fatalf("firstFieldInExpressionText(%q) = %q, want %q", expr, got, want)
		}
	}

	args := splitCommaList(`@"a,b", strcat("c,d", Account), 'e\'f', nested(1, (2, 3))`)
	if len(args) != 4 || args[0] != `@"a,b"` || args[1] != ` strcat("c,d", Account)` || args[3] != ` nested(1, (2, 3))` {
		t.Fatalf("splitCommaList preserved wrong arguments: %#v", args)
	}
	if args := splitCommaList(`unterminated("a,b", second`); len(args) != 1 {
		t.Fatalf("unterminated nested expression should stay together: %#v", args)
	}

	if idx := indexOutsideQuotesAndParens(`a = @"not = here" and b = 2`, " and "); idx != len(`a = @"not = here"`) {
		t.Fatalf("verbatim string delimiter index = %d", idx)
	}
	if idx := indexOutsideQuotesAndParens(`fn(a = 1) = 2`, "="); idx != len(`fn(a = 1) `) {
		t.Fatalf("nested delimiter index = %d", idx)
	}
	if idx := indexOutsideQuotesAndParens(`a = "escaped \" = inside" and b = 2`, " and "); idx <= 0 {
		t.Fatalf("escaped quoted delimiter not skipped: %d", idx)
	}
	for _, expr := range []string{`@"unterminated =`, `fn(a = 1`} {
		if idx := indexOutsideQuotesAndParens(expr, "="); idx != -1 {
			t.Fatalf("unterminated scanner %q returned %d", expr, idx)
		}
	}
}
