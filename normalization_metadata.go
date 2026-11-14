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
	searchCount     int
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
	budget := suppliedBudget(budgets)
	budget.step()

	marker, replacement := strconv.Quote(m.subqueryMarker), strconv.Quote(SubqueryPlaceholder)
	var restored strings.Builder
	runes := 0
	for {
		budget.step()
		index := strings.Index(query, marker)
		if index < 0 {
			restored.WriteString(query)
			return restored.String()
		}
		restored.WriteString(query[:index])
		runes += utf8.RuneCountInString(query[:index])
		budget.outputSize(len(m.subqueryOffsets) + 1)
		m.subqueryOffsets = append(m.subqueryOffsets, runes)
		restored.WriteString(replacement)
		runes += utf8.RuneCountInString(replacement)
		query = query[index+len(marker):]
	}
}
