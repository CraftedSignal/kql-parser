package kql

import (
	"strings"
	"testing"
)

func TestKQLUnicodeEscapeWordJoinerIsDroppedOutsideStrings(t *testing.T) {
	input := `SecurityEvent\u2060| where Account == "a\u2060b" and User == 'x\u2060y'`
	want := `SecurityEvent| where Account == "a\u2060b" and User == 'x\u2060y'`

	if got := normalizeEscapeSequences(input); got != want {
		t.Fatalf("normalizeEscapeSequences() = %q, want %q", got, want)
	}
}

func TestKQLUnicodeEscapeNormalizationPreservesLineComments(t *testing.T) {
	query := `SecurityEvent | where true // comment \u000a| where EventID == 4624`
	if got := normalizeEscapeSequences(query); got != query {
		t.Fatalf("line comment changed during escape normalization: got %q, want %q", got, query)
	}
	result := ExtractConditions(query)
	if len(result.Conditions) != 0 {
		t.Fatalf("extracted conditions from line comment: %+v", result.Conditions)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("valid query produced errors: %v", result.Errors)
	}
}

func TestKQLUnicodeEscapeNormalizationPreservesBlockComments(t *testing.T) {
	query := `SecurityEvent | where true /* comment \u002a/ | where EventID == 4624 */`
	result := ExtractConditions(query)
	if len(result.Errors) != 0 {
		t.Fatalf("valid query produced errors: %v", result.Errors)
	}
	if len(result.Conditions) != 0 {
		t.Fatalf("extracted conditions from block comment: %+v", result.Conditions)
	}
	if got := normalizeEscapeSequences(query); got != query {
		t.Fatalf("block comment changed during escape normalization: got %q, want %q", got, query)
	}
}

func TestKQLUnicodeEscapeNormalizationPreservesMultilineLiteral(t *testing.T) {
	for _, tc := range []struct{ literal, want string }{
		{"```\\u0041```", `\u0041`},
		{`"""one " quote \u2060"""`, "one \" quote \u2060"},
	} {
		query := "SecurityEvent | where Message == " + tc.literal
		t.Run(tc.literal, func(t *testing.T) {
			if got := normalizeEscapeSequences(query); got != query {
				t.Fatalf("multiline literal changed during escape normalization: got %q, want %q", got, query)
			}
			result := ExtractConditions(query)
			if len(result.Errors) != 0 || len(result.Conditions) != 1 {
				t.Fatalf("conditions=%+v errors=%q", result.Conditions, result.Errors)
			}
			if got := result.Conditions[0].RHS.Value; got != tc.want {
				t.Fatalf("multiline literal value = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStripKQLCommentsPreservesStringLiterals(t *testing.T) {
	query := `T | where Message == @"/* not a comment */ // still a literal"`
	if got := stripKQLComments(query); got != query {
		t.Fatalf("string literal changed while stripping comments: got %q, want %q", got, query)
	}
}

func TestStripKQLCommentsPreservesMultilineLiterals(t *testing.T) {
	for _, literal := range []string{
		"```first line\n// still literal\n/* still literal */```",
		"\"\"\"first line\n// still literal\n/* still literal */\"\"\"",
	} {
		query := "T | where Message == " + literal
		if got := stripKQLComments(query); got != query {
			t.Errorf("multiline literal changed while stripping comments: got %q, want %q", got, query)
		}
	}
}

func TestKQLHandleUnicodeEscapeZeroWidthAndUppercaseHex(t *testing.T) {
	var b strings.Builder
	handleUnicodeEscape("2060", &b)
	handleUnicodeEscape("FEFF", &b)
	handleUnicodeEscape("00A0", &b)
	handleUnicodeEscape("0041", &b)

	if got, want := b.String(), " A"; got != want {
		t.Fatalf("handleUnicodeEscape() = %q, want %q", got, want)
	}
}

func TestKQLNormalizeUnicodeWhitespacePreservesCommentsInvalidBytesAndUnicode(t *testing.T) {
	input := "SecurityEvent\u00a0/* keep\u00a0\u2060comment */\n| where Café == \"x\"" + string([]byte{0xff}) + " and Message == \"ok\""
	want := "SecurityEvent /* keep\u00a0\u2060comment */\n| where Café == \"x\"" + string([]byte{0xff}) + " and Message == \"ok\""

	if got := normalizeUnicodeWhitespace(input); got != want {
		t.Fatalf("normalizeUnicodeWhitespace() = %q, want %q", got, want)
	}
}

func TestKQLNormalizeUnicodeWhitespacePreservesVerbatimDoubledQuotes(t *testing.T) {
	input := "SecurityEvent | where Message == @\"a\"\"b\u00a0c\" and Account == 'alice'\u200b"
	want := "SecurityEvent | where Message == @\"a\"\"b\u00a0c\" and Account == 'alice'"

	if got := normalizeUnicodeWhitespace(input); got != want {
		t.Fatalf("normalizeUnicodeWhitespace() = %q, want %q", got, want)
	}
}
