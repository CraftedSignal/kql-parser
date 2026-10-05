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
