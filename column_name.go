package kql

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
)

// columnNameSpan reads complete bracketed compound strings without treating a
// literal's closing bracket as syntax. Each component is scanned once.
func columnNameSpan(text string, start int, budget *parseBudget) (end int, parts []compoundStringToken, ok bool) {
	if start >= len(text) || text[start] != '[' {
		return start, nil, false
	}
	i := start + 1
	for {
		for i < len(text) {
			budget.step()
			if strings.ContainsRune(" \t\r\n\f", rune(text[i])) {
				i++
				continue
			}
			if i+1 < len(text) && text[i:i+2] == "//" {
				for i < len(text) && text[i] != '\r' && text[i] != '\n' {
					budget.step()
					i++
				}
				continue
			}
			break
		}
		if i < len(text) && text[i] == ']' {
			return i + 1, parts, len(parts) > 0
		}
		token, found := compoundStringAt(text, i, budget)
		if !found || !token.valid {
			return start, nil, false
		}
		parts = appendTemporary(parts, budget, token)
		i = token.end
	}
}

func decodedColumnName(text string, budget *parseBudget) (string, bool) {
	// A complete standard/verbatim literal needs no compound inventories or
	// concatenation. Keep its existing scan/decode reservations intact.
	if len(text) >= 2 && text[0] == '[' && text[len(text)-1] == ']' {
		if value, _, complete := readIdentityLiteral(text[1:len(text)-1], budget, true); complete {
			return value, true
		}
	}
	end, parts, ok := columnNameSpan(text, 0, budget)
	if !ok || end != len(text) {
		return "", false
	}
	budget.reserveWork(len(parts))
	values := make([]string, 0, len(parts))
	size := 0
	for _, part := range parts {
		budget.step()
		value := decodeKQLStringBudgeted(text[part.start:part.end], budget)
		size = compoundSize(size, len(value), budget)
		values = append(values, value)
	}
	budget.reserveWork(size)
	var result strings.Builder
	result.Grow(size)
	for _, value := range values {
		budget.step()
		result.WriteString(value)
	}
	return result.String(), true
}

// Both already-completed preflights must belong to this request and source.
// Missing, foreign or incomplete lexer metadata cannot prove names absent.
func columnNamesAbsent(raw, normalized *antlr.CommonTokenStream, rawText, normalizedText string, budget *parseBudget) bool {
	for _, entry := range []struct {
		stream *antlr.CommonTokenStream
		text   string
	}{{raw, rawText}, {normalized, normalizedText}} {
		budget.step()
		if entry.stream == nil {
			return false
		}
		lexer, ok := entry.stream.GetTokenSource().(*budgetLexer)
		if !ok || lexer.budget != budget || lexer.source != entry.text || !lexer.complete || lexer.hasQuotedIdentifier {
			return false
		}
	}
	return true
}

func validateKQLColumnNames(tree antlr.Tree, source *kqlMappedSource, owners *kqlSourceOwners, budget *parseBudget) (map[*TabularOperatorContext]bool, string) {
	if owners == nil {
		owners = associateKQLSource(tree, source, budget)
	}
	var empty map[*TabularOperatorContext]bool
	markEmpty := func(node antlr.Tree) {
		for parent := node.GetParent(); parent != nil; parent = parent.GetParent() {
			budget.step()
			if stage, ok := parent.(*TabularOperatorContext); ok {
				if empty == nil {
					budget.reserveWork(1)
					empty = make(map[*TabularOperatorContext]bool)
				}
				budget.reserveWork(1)
				empty[stage] = true
				break
			}
		}
	}
	var walk func(antlr.Tree) string
	walk = func(node antlr.Tree) string {
		budget.step()
		if name, ok := node.(*ColumnNameContext); ok {
			if name.GetStart() == nil || name.GetStop() == nil || name.GetStart().GetTokenIndex() < 0 {
				return "missing column name"
			}
			authored, found := owners.authoredContext(name, budget)
			if !found {
				return "column name source ownership unavailable"
			}
			value := authored
			if strings.HasPrefix(authored, "[") {
				var valid bool
				value, valid = decodedColumnName(authored, budget)
				if !valid {
					return "invalid quoted column name"
				}
			}

			if value == "" {
				markEmpty(name)
			}
		}
		// Empty standalone references have the same unrepresentable identity
		// in passthrough/group/filter roles. Other legacy reference admission
		// remains independent of declaration validation.
		if primary, ok := node.(*PrimaryExpressionContext); ok && primary.QUOTED_IDENTIFIER() != nil {
			if value, valid := decodedColumnName(primary.QUOTED_IDENTIFIER().GetText(), budget); valid && value == "" {
				markEmpty(primary)
			}
		}

		for _, child := range node.GetChildren() {
			if diagnostic := walk(child); diagnostic != "" {
				return diagnostic
			}
		}
		return ""
	}
	diagnostic := walk(tree)
	return empty, diagnostic
}
