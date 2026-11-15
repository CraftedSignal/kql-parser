package kql

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
)

// A protected membership is one occurrence of an inline tabular membership.
// The parser sees only marker; source remains the complete authored filter atom.
type protectedMembership struct {
	marker   string
	source   string
	selected bool
	bound    bool
}

type membershipProtection struct {
	entries   []*protectedMembership
	byContext map[IComparisonExpressionContext]*protectedMembership
}

func membershipToken(kind int) bool {
	switch kind {
	case KQLLexerIN, KQLLexerNOT_IN, KQLLexerIN_CS, KQLLexerNOT_IN_CS:
		return true
	}
	return false
}

func membershipTokens(query string, budget *parseBudget) []antlr.Token {
	all := searchTokens(query, budget)
	budget.outputSize(len(all))
	var tokens []antlr.Token
	for _, token := range all {
		budget.step()
		if token.GetChannel() == antlr.TokenDefaultChannel {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func membershipDepths(tokens []antlr.Token, budget *parseBudget) []int {
	budget.outputSize(len(tokens))
	depths := make([]int, len(tokens))
	depth := 0
	for i, token := range tokens {
		budget.step()
		if token.GetTokenType() == KQLLexerRPAREN {
			depth--
		}
		depths[i] = depth
		if token.GetTokenType() == KQLLexerLPAREN {
			depth++
		}
	}
	return depths
}

func membershipWhere(tokens []antlr.Token, depths []int, index int, budget *parseBudget) bool {
	for j := index - 1; j >= 0; j-- {
		budget.step()
		if depths[j] > depths[index] {
			continue
		}
		switch tokens[j].GetTokenType() {
		case KQLLexerWHERE:
			return true
		case KQLLexerPIPE, KQLLexerSEMICOLON:
			return false
		}
	}
	return false
}

func membershipParenEnd(tokens []antlr.Token, open int, budget *parseBudget) (int, bool) {
	depth, pipe := 0, false
	for i := open; i < len(tokens); i++ {
		budget.step()
		switch tokens[i].GetTokenType() {
		case KQLLexerLPAREN:
			depth++
		case KQLLexerRPAREN:
			depth--
			if depth == 0 {
				return i, pipe
			}
		case KQLLexerPIPE:
			pipe = true
		}
	}
	return -1, pipe
}

func membershipAtomSpan(tokens []antlr.Token, depths []int, operator, rhsEnd int, budget *parseBudget) (int, int) {
	start := 0
	for j := operator - 1; j >= 0; j-- {
		budget.step()
		if depths[j] <= depths[operator] {
			switch tokens[j].GetTokenType() {
			case KQLLexerWHERE, KQLLexerAND, KQLLexerOR, KQLLexerPIPE, KQLLexerSEMICOLON:
				start = j + 1
				j = -1
			}
		}
	}
	// Parenthesized Boolean groups and outer NOT own structure, not source.
	for start < operator {
		budget.step()
		kind := tokens[start].GetTokenType()
		if kind != KQLLexerNOT && kind != KQLLexerLPAREN {
			break
		}
		if kind == KQLLexerLPAREN {
			close, _ := membershipParenEnd(tokens, start, budget)
			if close >= 0 && close+1 < len(tokens) {
				switch tokens[close+1].GetTokenType() {
				case KQLLexerAND, KQLLexerOR, KQLLexerPIPE, KQLLexerSEMICOLON, KQLLexerRPAREN, antlr.TokenEOF:
				default:
					// The group is an operand of a larger comparison or
					// arithmetic atom and belongs in its source.
					return membershipAtomEnd(tokens, depths, start, rhsEnd, budget)
				}
			}
		}
		start++
	}
	// If the membership is inside a function argument, the function call is
	// the complete atomic predicate represented by this one scalar leaf.
	for start > 1 && depths[start] > 0 {
		budget.step()
		open := -1
		for j := start - 1; j >= 0; j-- {
			budget.step()
			if tokens[j].GetTokenType() == KQLLexerLPAREN && depths[j] == depths[start]-1 {
				open = j
				break
			}
		}
		if open <= 0 || tokens[open-1].GetTokenType() != KQLLexerIDENTIFIER {
			break
		}
		start = open - 1
	}
	return membershipAtomEnd(tokens, depths, start, rhsEnd, budget)
}

func membershipAtomEnd(tokens []antlr.Token, depths []int, start, rhsEnd int, budget *parseBudget) (int, int) {
	base := depths[start]
	end := len(tokens) - 1
	for j := rhsEnd + 1; j < len(tokens); j++ {
		budget.step()
		kind := tokens[j].GetTokenType()
		if depths[j] < base ||
			depths[j] <= base && (kind == KQLLexerAND || kind == KQLLexerOR || kind == KQLLexerPIPE || kind == KQLLexerSEMICOLON || kind == antlr.TokenEOF) {
			end = j
			break
		}
	}
	return start, end
}

func membershipFailure(message string) {
	panic(budgetAbort{fmt.Errorf("membership source preservation failed: %s", message)})
}

// Run before the lossy normalizers, after selecting the same main-query
// boundary as search protection. Every occurrence has a distinct identifier.
func protectMembershipSources(query string, budget *parseBudget) (string, *membershipProtection) {
	if budget == nil {
		bounded, cancel := newParseBudget(context.Background(), defaultParseLimits())
		defer cancel()
		budget = bounded
	}
	budget.step()
	protection := &membershipProtection{}
	if !strings.Contains(strings.ToLower(query), "in") {
		return query, protection
	}
	tokens := membershipTokens(query, budget)
	depths := membershipDepths(tokens, budget)
	positions := searchRuneBytes(query, budget)
	selectedStart := skipLetStatements(query)
	var out strings.Builder
	last := 0
	for i := 0; i+1 < len(tokens); i++ {
		budget.step()
		if !membershipToken(tokens[i].GetTokenType()) || tokens[i+1].GetTokenType() != KQLLexerLPAREN || !membershipWhere(tokens, depths, i, budget) {
			continue
		}
		close, pipe := membershipParenEnd(tokens, i+1, budget)
		if !pipe {
			continue
		}
		if close < 0 {
			membershipFailure("unmatched tabular RHS")
		}
		startToken, endToken := membershipAtomSpan(tokens, depths, i, close, budget)
		start := positions[tokens[startToken].GetStart()]
		end := len(query)
		if endToken < len(tokens) && tokens[endToken].GetStart() >= 0 {
			end = positions[tokens[endToken].GetStart()]
		}
		if start < last || start >= end {
			membershipFailure("overlapping predicate atoms")
		}
		rhsStart := positions[tokens[i+1].GetStop()+1]
		rhsStop := positions[tokens[close].GetStart()]
		if rhsStart > rhsStop || rhsStop > len(query) {
			membershipFailure("invalid tabular RHS span")
		}
		if start >= selectedStart {
			validateMembershipRHS(query[rhsStart:rhsStop], budget)
			probe := query[start:rhsStart] + `"membership"` + query[rhsStop:end]
			validateMembershipQuery("T | where "+probe, budget)
		}
		marker := "__kql_membership_" + strconv.Itoa(len(protection.entries)) + "__"
		for strings.Contains(query, marker) {
			budget.step()
			marker += "_"
		}
		budget.reserveOutput(1)
		entry := &protectedMembership{marker: marker, source: strings.TrimRight(query[start:end], " \t\r\n"), selected: start >= selectedStart}
		protection.entries = append(protection.entries, entry)
		replacement := " " + marker + " "
		reserveSearchBytes(out.Len()+start-last+len(replacement)+len(query)-end, budget)
		out.WriteString(query[last:start])
		out.WriteString(replacement)
		last = end
		i = endToken - 1
	}
	if len(protection.entries) == 0 {
		return query, protection
	}
	out.WriteString(query[last:])
	return out.String(), protection
}

func validateMembershipRHS(rhs string, budget *parseBudget) {
	budget.step()
	if strings.TrimSpace(rhs) == "" {
		membershipFailure("empty tabular RHS")
	}
	protected, _ := protectMembershipSources(rhs, budget)
	validateMembershipQuery(protected, budget)
}

func validateMembershipQuery(query string, budget *parseBudget) {
	budget.step()
	normalized := normalizeBudgetedMetadata(query, true, budget, nil)
	listener := &errorListener{budget: budget}
	stream := preflight(normalized, budget, true, listener)
	parser := NewKQLParser(&budgetTokenStream{CommonTokenStream: stream, budget: budget})
	parser.RemoveErrorListeners()
	parser.AddErrorListener(listener)
	tree := parser.Query()
	budget.check()
	if len(listener.errors) != 0 {
		membershipFailure("invalid tabular predicate syntax: " + shortenSyntaxError(listener.errors[0]))
	}
	if _, diagnostic := validateKQLSearches(tree, budget); diagnostic != "" {
		membershipFailure("invalid tabular predicate syntax: " + diagnostic)
	}
}

func (p *membershipProtection) bind(tree antlr.Tree, normalized string, budget *parseBudget) {
	budget.step()
	if len(p.entries) == 0 {
		return
	}
	tokens := membershipTokens(normalized, budget)
	byMarker := make(map[string]*protectedMembership, len(p.entries))
	positions := make(map[string]int, len(p.entries))
	for _, entry := range p.entries {
		budget.step()
		byMarker[entry.marker] = entry
	}
	for _, token := range tokens {
		budget.step()
		if token.GetTokenType() != KQLLexerIDENTIFIER {
			continue
		}
		if entry := byMarker[token.GetText()]; entry != nil {
			if _, exists := positions[entry.marker]; exists {
				membershipFailure("duplicate private identity")
			}
			positions[entry.marker] = token.GetStart()
		}
	}
	for _, entry := range p.entries {
		budget.step()
		if entry.selected {
			if _, exists := positions[entry.marker]; !exists {
				membershipFailure("selected predicate was removed")
			}
		}
	}
	p.byContext = make(map[IComparisonExpressionContext]*protectedMembership)
	var visit func(antlr.Tree, IComparisonExpressionContext)
	visit = func(node antlr.Tree, owner IComparisonExpressionContext) {
		budget.step()
		if comparison, ok := node.(IComparisonExpressionContext); ok {
			if owner == nil || groupedKQLExpression(owner, budget) != nil {
				owner = comparison
			}
		}
		if terminal, ok := node.(antlr.TerminalNode); ok {
			if token := terminal.GetSymbol(); token != nil && token.GetTokenType() == KQLLexerIDENTIFIER {
				if entry := byMarker[token.GetText()]; entry != nil && positions[entry.marker] == token.GetStart() {
					if owner == nil || entry.bound {
						membershipFailure("unbound private identity")
					}
					if previous := p.byContext[owner]; previous != nil && previous != entry {
						membershipFailure("ambiguous predicate ownership")
					}
					entry.bound = true
					p.byContext[owner] = entry
				}
			}
		}
		for i := 0; i < node.GetChildCount(); i++ {
			visit(node.GetChild(i), owner)
		}
	}
	visit(tree, nil)
	for _, entry := range p.entries {
		budget.step()
		if entry.selected && !entry.bound {
			membershipFailure("selected predicate was not parsed")
		}
	}
}

// Recursive join extraction receives source text, never parser placeholders.
func (p *membershipProtection) originalSubquery(query string, budget *parseBudget) string {
	budget.step()
	if len(p.entries) == 0 {
		return query
	}
	tokens := membershipTokens(query, budget)
	positions := searchRuneBytes(query, budget)
	var out strings.Builder
	last := 0
	for _, token := range tokens {
		budget.step()
		if token.GetTokenType() != KQLLexerIDENTIFIER {
			continue
		}
		for _, entry := range p.entries {
			budget.step()
			if token.GetText() != entry.marker {
				continue
			}
			start, end := positions[token.GetStart()], positions[token.GetStop()+1]
			if start < last {
				membershipFailure("recursive source overlap")
			}
			reserveSearchBytes(out.Len()+start-last+len(entry.source)+len(query)-end, budget)
			out.WriteString(query[last:start])
			out.WriteString(entry.source)
			last = end
			break
		}
	}
	if last == 0 {
		return query
	}
	out.WriteString(query[last:])
	return out.String()
}
