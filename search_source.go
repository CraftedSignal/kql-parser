package kql

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/antlr4-go/antlr/v4"
)

type protectedSearchClause struct{ marker, source string }
type searchProtection struct {
	clauses  []protectedSearchClause
	restored int
}
type searchSpan struct{ first, end int }

func searchTokens(query string, budget *parseBudget) []antlr.Token {
	budget.step()
	if budget != nil {
		return preflight(query, budget, false, nil).GetAllTokens()
	}
	lexer := NewKQLLexer(antlr.NewInputStream(query))
	lexer.RemoveErrorListeners()
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	stream.Fill()
	return stream.GetAllTokens()
}

// Find operators, not spelling inside literals/comments/arguments. This only
// identifies tabular boundaries; the generated search grammar validates syntax.
func searchClauseSpans(tokens []antlr.Token, budget *parseBudget) []searchSpan {
	budget.step()
	var spans []searchSpan
	source, operator, letHeader := true, false, false
	for i := 0; i < len(tokens); i++ {
		budget.step()
		t := tokens[i]
		if t.GetChannel() != antlr.TokenDefaultChannel {
			continue
		}
		kind := t.GetTokenType()
		if kind == KQLLexerSEARCH && (source || operator) {
			depth, j := 0, i+1
			for ; j < len(tokens); j++ {
				budget.step()
				if tokens[j].GetChannel() != antlr.TokenDefaultChannel {
					continue
				}
				k := tokens[j].GetTokenType()
				if depth == 0 && (k == KQLLexerPIPE || k == KQLLexerSEMICOLON || k == KQLLexerRPAREN || k == KQLLexerRBRACE || k == KQLLexerRBRACKET || k == antlr.TokenEOF) {
					break
				}
				switch k {
				case KQLLexerLPAREN, KQLLexerLBRACE, KQLLexerLBRACKET:
					depth++
				case KQLLexerRPAREN, KQLLexerRBRACE, KQLLexerRBRACKET:
					depth--
				}
			}
			budget.reserveOutput(1)
			spans = append(spans, searchSpan{i, j})
			i = j - 1
			source, operator = false, false
			continue
		}
		switch kind {
		case KQLLexerPIPE:
			source, operator = false, true
		case KQLLexerSEMICOLON:
			source, operator = true, false
			letHeader = false
		case KQLLexerLET:
			source, operator, letHeader = false, false, true
		case KQLLexerASSIGN:
			source, operator = letHeader, false
			letHeader = false
		case KQLLexerIN:
			// IN can start a tabular subquery after its opening parentheses.
			source, operator = true, false
		case KQLLexerJOIN, KQLLexerUNION, KQLLexerLOOKUP:
			source, operator = operator, false
		case KQLLexerLPAREN:
			operator = false
		case KQLLexerRPAREN:
			source, operator = false, false
		default:
			source, operator = false, false
		}
	}
	return spans
}

func searchRuneBytes(query string, budget *parseBudget) []int {
	budget.step()
	budget.input(query)
	budget.reserveOutput(1)
	offsets := make([]int, 0, utf8.RuneCountInString(query)+1)
	for index := range query {
		budget.step()
		offsets = append(offsets, index)
	}
	return append(offsets, len(query))
}

func protectSearchClauses(query string, budget *parseBudget) (string, *searchProtection) {
	budget.step()
	protection := &searchProtection{}
	tokens := searchTokens(query, budget)
	spans := searchClauseSpans(tokens, budget)
	if len(spans) == 0 {
		return query, protection
	}
	positions := searchRuneBytes(query, budget)
	var out strings.Builder
	last := 0
	for index, span := range spans {
		budget.step()
		start, end := positions[tokens[span.first].GetStart()], len(query)
		if span.end < len(tokens) && tokens[span.end].GetStart() >= 0 {
			end = positions[tokens[span.end].GetStart()]
		}
		marker := "__kql_search_" + strconv.Itoa(index) + "__"
		for strings.Contains(query, marker) {
			budget.step()
			marker += "_"
		}
		replacement := "search " + strconv.Quote(marker) + " "
		reserveSearchBytes(out.Len()+start-last+len(replacement)+len(query)-end, budget)
		budget.reserveOutput(1)
		protection.clauses = append(protection.clauses, protectedSearchClause{marker: marker, source: query[start:end]})
		out.WriteString(query[last:start])
		out.WriteString(replacement)
		last = end
	}
	out.WriteString(query[last:])
	return out.String(), protection
}

func reserveSearchBytes(size int, budget *parseBudget) {
	budget.step()
	if budget != nil && size > budget.limits.maxInputBytes {
		budget.reject("input bytes")
	}
}

func (p *searchProtection) restore(query string, budget *parseBudget) string {
	budget.step()
	if len(p.clauses) == 0 {
		return query
	}
	tokens := searchTokens(query, budget)
	spans := searchClauseSpans(tokens, budget)
	positions := searchRuneBytes(query, budget)
	used := make(map[string]bool)
	var out strings.Builder
	last := 0
	for _, span := range spans {
		budget.step()
		start, end := positions[tokens[span.first].GetStart()], len(query)
		if span.end < len(tokens) && tokens[span.end].GetStart() >= 0 {
			end = positions[tokens[span.end].GetStart()]
		}
		clause := strings.TrimSpace(query[start:end])
		var entry *protectedSearchClause
		for i := range p.clauses {
			budget.step()
			if clause == "search "+strconv.Quote(p.clauses[i].marker) {
				entry = &p.clauses[i]
				break
			}
		}
		if entry == nil || used[entry.marker] {
			panic(budgetAbort{fmt.Errorf("search source preservation failed")})
		}
		budget.reserveOutput(1)
		used[entry.marker] = true
		p.restored++
		reserveSearchBytes(out.Len()+start-last+len(entry.source)+len(query)-end, budget)
		out.WriteString(query[last:start])
		out.WriteString(entry.source)
		last = end
	}
	out.WriteString(query[last:])
	return out.String()
}
