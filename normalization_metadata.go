package kql

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Track generated placeholders independently of user-authored strings. Private
// marker literals survive subsequent normalization; restore the public marker
// and record its final rune offsets before ANTLR sees the input.
type kqlNormalizationMetadata struct {
	subqueryMarker  string
	subqueryOffsets []int
}

func newKQLNormalizationMetadata(query string) *kqlNormalizationMetadata {
	for index := 0; ; index++ {
		marker := "__kql_in_subquery_" + strconv.Itoa(index) + "__"
		if !strings.Contains(query, marker) {
			return &kqlNormalizationMetadata{subqueryMarker: marker}
		}
	}
}

func (m *kqlNormalizationMetadata) restoreSubqueryMarkers(query string) string {
	marker, replacement := strconv.Quote(m.subqueryMarker), strconv.Quote(SubqueryPlaceholder)
	var restored strings.Builder
	runes := 0
	for {
		index := strings.Index(query, marker)
		if index < 0 {
			restored.WriteString(query)
			return restored.String()
		}
		restored.WriteString(query[:index])
		runes += utf8.RuneCountInString(query[:index])
		m.subqueryOffsets = append(m.subqueryOffsets, runes)
		restored.WriteString(replacement)
		runes += utf8.RuneCountInString(replacement)
		query = query[index+len(marker):]
	}
}
