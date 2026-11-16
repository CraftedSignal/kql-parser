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
	mapped          *kqlMappedSource
	subqueryMarker  string
	subqueryOffsets []int
	searchCount     int
	membership      *membershipProtection
	typed           *typedProtection
}

func newKQLNormalizationMetadata(query string, budgets ...*parseBudget) *kqlNormalizationMetadata {
	budget := suppliedBudget(budgets)
	budget.step()

	for index := 0; ; index++ {
		budget.step()
		marker := "__kql_in_subquery_" + strconv.Itoa(index) + "__"
		if !strings.Contains(query, marker) {
			return &kqlNormalizationMetadata{subqueryMarker: marker}
		}
	}
}

func (m *kqlNormalizationMetadata) restoreSubqueryMarkers(query string, budgets ...*parseBudget) string {
	return m.restoreSubqueryMarkersWithSource(query, suppliedBudget(budgets), nil)
}

func (m *kqlNormalizationMetadata) restoreSubqueryMarkersWithSource(query string, budget *parseBudget, recorder *kqlSourceRecorder) string {
	budget.step()

	marker, replacement := strconv.Quote(m.subqueryMarker), strconv.Quote(SubqueryPlaceholder)
	var restored strings.Builder
	runes := 0
	input := query
	position := 0
	for {
		budget.step()
		index := strings.Index(input[position:], marker)
		if index < 0 {
			restored.WriteString(input[position:])
			return finishKQLRecorded(input, restored.String(), recorder)
		}
		index += position
		restored.WriteString(input[position:index])
		runes += utf8.RuneCountInString(input[position:index])
		budget.outputSize(len(m.subqueryOffsets) + 1)
		m.subqueryOffsets = append(m.subqueryOffsets, runes)
		restored.WriteString(replacement)
		if recorder != nil {
			recorder.Replace(index, index+len(marker), replacement, "")
		}
		runes += utf8.RuneCountInString(replacement)
		position = index + len(marker)
	}
}
