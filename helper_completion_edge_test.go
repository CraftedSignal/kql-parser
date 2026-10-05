package kql

import (
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"
)

type wrappedTabularExpressionContext struct {
	*TabularExpressionContext
	text string
}

func (w *wrappedTabularExpressionContext) GetText() string { return w.text }

func kqlTokenForTextRange(start, stop int) antlr.Token {
	return antlr.NewCommonToken(&antlr.TokenSourceCharStreamPair{}, 0, antlr.TokenDefaultChannel, start, stop)
}

func TestKQLPortableValueRemainingEdges(t *testing.T) {
	lists := map[string][]string{
		"iocs": {"bad.exe", "bad.exe", "evil.exe"},
	}

	if value, alternatives, ok := parsePortableValue(`false;`, lists); !ok || value != "false" || alternatives != nil {
		t.Fatalf("boolean portable value = value=%q alternatives=%#v ok=%v", value, alternatives, ok)
	}
	if value, alternatives, ok := parsePortableValue(`IOCs;`, lists); !ok || value != "bad.exe" || len(alternatives) != 3 {
		t.Fatalf("simple list reference not expanded: value=%q alternatives=%#v ok=%v", value, alternatives, ok)
	}
	if got := portableValueReference(`IOCs;`, lists); got != "IOCs" {
		t.Fatalf("simple portable value reference = %q", got)
	}
	if values, ok := parseListValue(`[IOCs, "evil.exe", 7, Unknown]`, lists); !ok || len(values) != 3 || values[0] != "bad.exe" || values[2] != "7" {
		t.Fatalf("list value dedupe/scalar handling = %#v ok=%v", values, ok)
	}
	if values, ok := parseListValue(`("first", , "second")`, lists); !ok || len(values) != 2 || values[1] != "second" {
		t.Fatalf("empty list item handling = %#v ok=%v", values, ok)
	}
	for _, raw := range []string{`dynamic([])`, `[]`, `(Unknown)`, `"unterminated`} {
		if value, alternatives, ok := parsePortableValue(raw, lists); ok {
			t.Fatalf("invalid portable value %q produced value=%q alternatives=%#v", raw, value, alternatives)
		}
	}
}

func TestKQLLiteralLetKeywordAndDedupRemainingEdges(t *testing.T) {
	for raw, want := range map[string]string{
		`"a\"b" rest`:     `a"b`,
		`'single\'quote'`: `single'quote`,
	} {
		value, _, ok := parseStringLiteral(raw)
		if !ok || value != want {
			t.Fatalf("parseStringLiteral(%q) = %q ok=%v, want %q", raw, value, ok, want)
		}
	}
	for _, raw := range []string{"", "bare", `"unterminated`} {
		if value, rest, ok := parseStringLiteral(raw); ok {
			t.Fatalf("invalid string literal %q produced value=%q rest=%q", raw, value, rest)
		}
	}

	name, rhs, ok := parseLetAssignment(`let Derived = iff(Message == "a=b", 1, 0) // comment`)
	if !ok || name != "Derived" || rhs != `iff(Message == "a=b", 1, 0)` {
		t.Fatalf("parseLetAssignment quoted equals = name=%q rhs=%q ok=%v", name, rhs, ok)
	}
	for _, line := range []string{`let = SecurityEvent`, `let bad-name = SecurityEvent`, `print x = 1`} {
		if name, rhs, ok := parseLetAssignment(line); ok {
			t.Fatalf("invalid let assignment %q produced name=%q rhs=%q", line, name, rhs)
		}
	}

	if got := extractPortableKeyword("none"); got != "" {
		t.Fatalf("trivial portable keyword = %q", got)
	}
	if got := compactKeyword(" \n\t "); got != "" {
		t.Fatalf("empty compact keyword = %q", got)
	}
	long := strings.Repeat("abcd ", 60)
	if got := compactKeyword(long); len(got) > 160 || strings.Contains(got, "\n") {
		t.Fatalf("long keyword not compacted/truncated correctly: len=%d value=%q", len(got), got)
	}
	if values := extractExpressionListValues(nil); values != nil {
		t.Fatalf("nil expression list values = %#v", values)
	}

	if got := stripOuterParens(`plain`); got != `plain` {
		t.Fatalf("stripOuterParens plain = %q", got)
	}
	if got := stripOuterParens(`(((Account == "a")))`); got != `Account == "a"` {
		t.Fatalf("stripOuterParens nested = %q", got)
	}
	if got := stripOuterParens(`("a\"b")`); got != `"a\"b"` {
		t.Fatalf("stripOuterParens escaped quote = %q", got)
	}

	if deduped := DeduplicateConditions(nil); deduped != nil {
		t.Fatalf("nil dedupe = %#v", deduped)
	}
	if deduped := DeduplicateConditions([]Condition{{Field: "Any", Operator: "==", Value: "*", PipeStage: 9}}); len(deduped) != 0 {
		t.Fatalf("wildcard-only dedupe = %#v", deduped)
	}
}

func TestKQLExtractTabularExpressionTextFallbackEdges(t *testing.T) {
	extractor := &conditionExtractor{originalQuery: `SecurityEvent | join (SigninLogs | where ResultType == 0) on Account`}
	if got := extractor.extractTabularExpressionText(nil); got != "" {
		t.Fatalf("nil tabular expression text = %q", got)
	}
	if got := extractor.extractTabularExpressionText(NewEmptyTabularExpressionContext()); got != "" {
		t.Fatalf("empty tabular expression text = %q", got)
	}
	wrapped := &wrappedTabularExpressionContext{
		TabularExpressionContext: NewEmptyTabularExpressionContext(),
		text:                     `SigninLogs | where ResultType == 0`,
	}
	if got := extractor.extractTabularExpressionText(wrapped); got != wrapped.text {
		t.Fatalf("wrapped tabular expression text = %q, want %q", got, wrapped.text)
	}

	query := `prefix SecurityEvent | where EventID == 4625 suffix`
	start := strings.Index(query, "SecurityEvent")
	subquery := `SecurityEvent | where EventID == 4625`
	boundedExtractor := &conditionExtractor{originalQuery: query}
	bounded := NewEmptyTabularExpressionContext()
	bounded.SetStart(kqlTokenForTextRange(start, start))
	bounded.SetStop(kqlTokenForTextRange(start+len(subquery)-1, start+len(subquery)-1))
	if got := boundedExtractor.extractTabularExpressionText(bounded); got != subquery {
		t.Fatalf("bounded tabular expression text = %q, want %q", got, subquery)
	}

	outOfRange := &wrappedTabularExpressionContext{
		TabularExpressionContext: NewEmptyTabularExpressionContext(),
		text:                     `fallback text`,
	}
	outOfRange.SetStart(kqlTokenForTextRange(len(query)+1, len(query)+1))
	outOfRange.SetStop(kqlTokenForTextRange(len(query)+2, len(query)+2))
	if got := boundedExtractor.extractTabularExpressionText(outOfRange); got != outOfRange.text {
		t.Fatalf("out-of-range tabular expression text = %q, want %q", got, outOfRange.text)
	}
}
