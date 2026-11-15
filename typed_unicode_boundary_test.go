package kql

import "testing"

func TestDecodeKQLStringUnicodeEscapeBoundaries(t *testing.T) {
	for _, fixture := range []struct {
		name, literal, value string
	}{
		{"bmp", `"\u00E9tail"`, "étail"},
		{"bmp_max", `"\uFFFFtail"`, "\uFFFFtail"},
		{"supplementary_min", `"\U00010000tail"`, "\U00010000tail"},
		{"supplementary", `"\U0001F600tail"`, "😀tail"},
		{"scalar_max", `"\U0010FFFFtail"`, "\U0010FFFFtail"},
		{"surrogate_pair", `"\uD83D\uDE00tail"`, "😀tail"},
		{"surrogate_pair_max", `"\uDBFF\uDFFFtail"`, "\U0010FFFFtail"},
		// These cases preserve this parser's existing replacement behavior;
		// they do not assert a Kusto contract for invalid scalar escapes.
		{"above_scalar_max", `"\U00110000tail"`, "\uFFFDtail"},
		{"signed_max", `"\U7FFFFFFFtail"`, "\uFFFDtail"},
		{"high_bit", `"\U80000000tail"`, "\uFFFDtail"},
		{"unsigned_max", `"\UFFFFFFFFtail"`, "\uFFFDtail"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if got := decodeKQLString(fixture.literal); got != fixture.value {
				t.Errorf("decodeKQLString(%q) = %q, want %q", fixture.literal, got, fixture.value)
			}
		})
	}
}

func TestTypedOperandUnicodeEscapeBoundaries(t *testing.T) {
	for _, fixture := range []struct {
		name, literal, value string
	}{
		{"bmp", `"\u00E9tail"`, "étail"},
		{"bmp_max", `"\uFFFFtail"`, "\uFFFFtail"},
		{"supplementary_min", `"\uD800\uDC00tail"`, "\U00010000tail"},
		{"surrogate_pair", `"\uD83D\uDE00tail"`, "😀tail"},
		{"surrogate_pair_max", `"\uDBFF\uDFFFtail"`, "\U0010FFFFtail"},
		{"raw_supplementary", `"😀tail"`, "😀tail"},
		{"raw_scalar_max", "\"\U0010FFFFtail\"", "\U0010FFFFtail"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			condition := requireTypedReviewCondition(t, `T | where x == `+fixture.literal)
			if condition.RHS.Kind != OperandString || condition.RHS.Source != fixture.literal || condition.RHS.Value != fixture.value {
				t.Errorf("RHS = %+v, want string source %q and value %q", condition.RHS, fixture.literal, fixture.value)
			}
		})
	}
}

func TestTypedOperandEightDigitUnicodeEscapesRemainDiagnosed(t *testing.T) {
	// The current grammar rejects uppercase-U escapes before decoding them,
	// including valid scalars. Keep this boundary while bounding the decoder.
	for _, literal := range []string{
		`"\U00010000tail"`, `"\U0001F600tail"`, `"\U0010FFFFtail"`,
		`"\U00110000tail"`, `"\U7FFFFFFFtail"`, `"\U80000000tail"`, `"\UFFFFFFFFtail"`,
	} {
		t.Run(literal, func(t *testing.T) {
			query := `T | where x == ` + literal
			parsed, semantic := ExtractConditions(query), AnalyzeQuery(query)
			if len(parsed.Errors) == 0 || len(semantic.Errors) == 0 || len(parsed.Conditions) != 0 || len(semantic.Conditions) != 0 {
				t.Fatalf("unsupported escape %q: parse=%#v semantic=%#v", literal, parsed, semantic)
			}
		})
	}
}
