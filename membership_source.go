package kql

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

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
		if tokens[j].GetTokenType() == KQLLexerIDENTIFIER && strings.EqualFold(tokens[j].GetText(), "filter") {
			return true
		}
	}
	return false
}

func membershipTabularRHS(tokens []antlr.Token, open, close int, pipe bool, budget *parseBudget) bool {
	budget.step()
	if pipe {
		return true
	}
	for j := open + 1; j < close; j++ {
		budget.step()
		if tokens[j].GetTokenType() == KQLLexerLPAREN {
			continue
		}
		switch tokens[j].GetTokenType() {
		case KQLLexerDATATABLE, KQLLexerEXTERNALDATA:
			return true
		}
		return false
	}
	return false
}

func membershipUnsupportedScalarSubquery(tokens []antlr.Token, depths []int, operator int, budget *parseBudget) bool {
	for j := operator - 1; j > 0; j-- {
		budget.step()
		if tokens[j].GetTokenType() != KQLLexerLPAREN || depths[j] >= depths[operator] {
			continue
		}
		close, _ := membershipParenEnd(tokens, j, budget)
		if close > operator && tokens[j-1].GetTokenType() == KQLLexerTOSCALAR {
			return true
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
			if j >= 0 && tokens[j].GetTokenType() == KQLLexerIDENTIFIER && strings.EqualFold(tokens[j].GetText(), "filter") {
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
		start++
	}
	// Walk every enclosing parenthesis. A Boolean group belongs to its parent
	// atom when it is a function argument or an operand of a comparison or
	// arithmetic expression, even when another Boolean branch precedes IN.
	for level := depths[operator] - 1; level >= 0; level-- {
		budget.step()
		open := -1
		for j := operator - 1; j >= 0; j-- {
			budget.step()
			if tokens[j].GetTokenType() == KQLLexerLPAREN && depths[j] == level {
				open = j
				break
			}
		}
		if open < 0 {
			continue
		}
		close, _ := membershipParenEnd(tokens, open, budget)
		if close < 0 {
			continue
		}
		if open > 0 && tokens[open-1].GetTokenType() == KQLLexerIDENTIFIER {
			start = open - 1
			continue
		}
		if open > 0 && membershipScalarOperator(tokens[open-1].GetTokenType()) ||
			close+1 < len(tokens) && membershipScalarOperator(tokens[close+1].GetTokenType()) {
			start = open
		}
	}
	// A promoted group/function can be the right operand of a comparison or
	// arithmetic chain. Include the complete left operand through the nearest
	// Boolean or pipeline boundary at this depth.
	if start > 0 && membershipScalarOperator(tokens[start-1].GetTokenType()) {
		base := depths[start]
		for j := start - 1; j >= 0; j-- {
			budget.step()
			if depths[j] < base || depths[j] == base && membershipAtomBoundary(tokens[j]) {
				start = j + 1
				break
			}
			if j == 0 {
				start = 0
			}
		}
	}
	return membershipAtomEnd(tokens, depths, start, rhsEnd, budget)
}

func membershipAtomBoundary(token antlr.Token) bool {
	switch token.GetTokenType() {
	case KQLLexerWHERE, KQLLexerAND, KQLLexerOR, KQLLexerNOT,
		KQLLexerPIPE, KQLLexerSEMICOLON:
		return true
	}
	return token.GetTokenType() == KQLLexerIDENTIFIER && strings.EqualFold(token.GetText(), "filter")
}

func membershipScalarOperator(kind int) bool {
	switch kind {
	case KQLLexerEQ, KQLLexerNEQ, KQLLexerLT, KQLLexerGT,
		KQLLexerLTE, KQLLexerGTE, KQLLexerEQTILDE, KQLLexerNEQTILDE,
		KQLLexerPLUS, KQLLexerMINUS, KQLLexerSTAR, KQLLexerSLASH,
		KQLLexerPERCENT, KQLLexerIN, KQLLexerNOT_IN,
		KQLLexerIN_CS, KQLLexerNOT_IN_CS:
		return true
	}
	return false
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
	return protectMembershipSourcesWithSource(query, budget, nil)
}

func protectMembershipSourcesWithSource(query string, budget *parseBudget, recorder *kqlSourceRecorder) (string, *membershipProtection) {
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
	var sourceEdits []kqlSourceEdit
	for i := 0; i+1 < len(tokens); i++ {
		budget.step()
		if !membershipToken(tokens[i].GetTokenType()) || tokens[i+1].GetTokenType() != KQLLexerLPAREN || !membershipWhere(tokens, depths, i, budget) {
			continue
		}
		close, pipe := membershipParenEnd(tokens, i+1, budget)
		if close < 0 {
			if pipe {
				membershipFailure("unmatched tabular RHS")
			}
			continue
		}
		if !membershipTabularRHS(tokens, i+1, close, pipe, budget) {
			continue
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
		if start >= selectedStart {
			if membershipUnsupportedScalarSubquery(tokens, depths, i, budget) {
				membershipFailure("unsupported membership inside scalar subquery")
			}
			validateMembershipOwner(query, tokens, positions, startToken, endToken, budget)
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
		if recorder != nil && recorder.Source.Text == query {
			recorder.Source.Protected[marker] = kqlPiecesInRange(recorder.Source, start, end, budget)
			owner := kqlOccurrenceAt(recorder.Source, start, budget)
			origin := originOfEdit(recorder.Source, kqlSourceEdit{Start: start, End: end}, budget)
			if owner >= 0 {
				origin = recorder.Source.Occurrences[owner].Span
			}
			sourceEdits = append(sourceEdits, kqlSourceEdit{Start: start, End: end, Parts: []kqlSourcePart{{Text: replacement, Origin: origin, OccurrenceID: owner}}})
		}
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
	result := out.String()
	if len(sourceEdits) > 0 {
		*recorder.Source = *applyKQLSourceEdits(recorder.Source, sourceEdits, budget)
		if recorder.Source.Text != result {
			sourceAbort("membership protection edit mismatch")
		}
	}
	return result, protection
}

func validateMembershipOwner(query string, tokens []antlr.Token, positions []int, startToken, endToken int, budget *parseBudget) {
	budget.step()
	start := positions[tokens[startToken].GetStart()]
	end := len(query)
	if endToken < len(tokens) && tokens[endToken].GetStart() >= 0 {
		end = positions[tokens[endToken].GetStart()]
	}
	var probe strings.Builder
	last := start
	for j := startToken; j+1 < endToken; j++ {
		budget.step()
		if !membershipToken(tokens[j].GetTokenType()) || tokens[j+1].GetTokenType() != KQLLexerLPAREN {
			continue
		}
		close, pipe := membershipParenEnd(tokens, j+1, budget)
		if close < 0 {
			membershipFailure("unmatched tabular RHS")
		}
		if !membershipTabularRHS(tokens, j+1, close, pipe, budget) {
			continue
		}
		rhsStart, rhsStop := positions[tokens[j+1].GetStop()+1], positions[tokens[close].GetStart()]
		if rhsStart > rhsStop || rhsStop > end {
			membershipFailure("invalid tabular RHS span")
		}
		validateMembershipRHS(query[rhsStart:rhsStop], budget)
		reserveSearchBytes(probe.Len()+rhsStart-last+len(`"membership"`)+end-rhsStop, budget)
		probe.WriteString(query[last:rhsStart])
		probe.WriteString(`"membership"`)
		last = rhsStop
		j = close
	}
	probe.WriteString(query[last:end])
	validateMembershipQuery("T | where "+probe.String(), budget)
}

func validateMembershipRHS(rhs string, budget *parseBudget) {
	budget.step()
	if strings.TrimSpace(rhs) == "" {
		membershipFailure("empty tabular RHS")
	}
	// These normalizers deliberately delete their subquery or parse body. Until
	// those bodies have their own validator, accepting them would launder errors.
	for _, token := range membershipTokens(rhs, budget) {
		budget.step()
		switch token.GetTokenType() {
		case KQLLexerMV_APPLY, KQLLexerPARSE, KQLLexerPARSE_WHERE, KQLLexerPARSE_KV:
			membershipFailure("unvalidated tabular stage")
		}
		if token.GetTokenType() == KQLLexerIDENTIFIER && strings.EqualFold(token.GetText(), "mvapply") {
			membershipFailure("unvalidated tabular stage")
		}
	}
	spans := validateMembershipConstructors(rhs, budget)
	// Only the private syntax-check buffer substitutes validated constructors.
	// Authored membership protection/ownership always retains the original rhs.
	syntax := membershipConstructorSyntax(rhs, spans, budget)
	protected, _ := protectMembershipSources(syntax, budget)
	validateMembershipQuery(protected, budget)
}

// Validate every constructor before normalizers can replace it with a dummy
// table name, including constructors nested in union/join branches.
func validateMembershipConstructors(rhs string, budget *parseBudget) []membershipConstructorSpan {
	var spans []membershipConstructorSpan
	for offset := 0; offset < len(rhs); {
		// Re-tokenize from a proven raw endpoint: an original token may cross
		// it because the lexer mistakes a ] inside a value for a data closer.
		tokens := membershipTokens(rhs[offset:], budget)
		positions := searchRuneBytes(rhs[offset:], budget)
		found := false
		for _, token := range tokens {
			budget.step()
			kind := token.GetTokenType()
			if kind != KQLLexerDATATABLE && kind != KQLLexerEXTERNALDATA {
				continue
			}
			start := offset + positions[token.GetStart()]
			end := start + validateMembershipConstructor(rhs[start:], budget)
			spans = appendTemporary(spans, budget, membershipConstructorSpan{start, end})
			offset, found = end, true
			break
		}
		if !found {
			break
		}
	}
	return spans
}

// The generated grammar cannot parse adjacent bracketed constructor data: its
// lexer emits the whole [..] as a quoted identifier. Validate that discarded
// source explicitly before the normalizer substitutes its table reference.
func validateMembershipConstructor(rhs string, budget *parseBudget) int {
	tokens := membershipTokens(rhs, budget)
	first := 0
	limit := len(tokens) - 1 // exclude EOF and enclosing grouping parentheses
	for first < len(tokens) && tokens[first].GetTokenType() == KQLLexerLPAREN {
		budget.step()
		close, _ := membershipParenEnd(tokens, first, budget)
		if close != limit-1 {
			break
		}
		first++
		limit--
	}
	if first >= len(tokens) {
		return 0
	}
	kind := tokens[first].GetTokenType()
	if kind != KQLLexerDATATABLE && kind != KQLLexerEXTERNALDATA {
		return 0
	}
	if first+1 >= len(tokens) || tokens[first+1].GetTokenType() != KQLLexerLPAREN {
		membershipFailure("invalid tabular constructor schema")
	}
	close, _ := membershipParenEnd(tokens, first+1, budget)
	if close < 0 || close+1 >= limit {
		membershipFailure("invalid tabular constructor schema")
	}
	columns := 0
	for j := first + 2; j < close; {
		budget.step()
		if j+2 >= close || !isSimpleIdentifier(tokens[j].GetText()) || tokens[j+1].GetTokenType() != KQLLexerCOLON || !isSimpleIdentifier(tokens[j+2].GetText()) {
			membershipFailure("invalid tabular constructor column")
		}
		columns++
		j += 3
		if j < close {
			if tokens[j].GetTokenType() != KQLLexerCOMMA {
				membershipFailure("invalid tabular constructor schema")
			}
			j++
		}
	}
	if columns == 0 {
		membershipFailure("empty tabular constructor schema")
	}
	positions := searchRuneBytes(rhs, budget)
	dataStart, dataEnd := membershipConstructorData(rhs, positions[tokens[close].GetStop()+1], budget)
	values := membershipSplitValues(rhs[dataStart+1:dataEnd-1], budget)
	if kind == KQLLexerDATATABLE && len(values)%columns != 0 {
		membershipFailure("incomplete datatable row")
	}
	if kind == KQLLexerEXTERNALDATA && len(values) == 0 {
		membershipFailure("externaldata needs a URI")
	}
	for _, value := range values {
		budget.step()
		if kind == KQLLexerEXTERNALDATA && !strings.HasPrefix(value, `@"`) && !strings.HasPrefix(value, `"`) && !strings.HasPrefix(value, `'`) {
			membershipFailure("invalid externaldata URI")
		}
		for _, token := range membershipTokens(value, budget) {
			budget.step()
			if token.GetTokenType() == KQLLexerPIPE || token.GetTokenType() == KQLLexerSEMICOLON {
				membershipFailure("invalid constructor value")
			}
		}
		validateMembershipQuery("T | where x == "+value, budget)
	}
	// Tokenize the suffix independently from the trusted raw endpoint. No
	// token that began inside the data can conceal bytes beyond that endpoint.
	suffix := rhs[dataEnd:]
	suffixTokens := membershipTokens(suffix, budget)
	suffixPositions := searchRuneBytes(suffix, budget)
	next, consumed := 0, dataEnd
	if kind == KQLLexerEXTERNALDATA && suffixTokens[next].GetTokenType() == KQLLexerWITH {
		if next+1 >= len(suffixTokens)-1 || suffixTokens[next+1].GetTokenType() != KQLLexerLPAREN {
			membershipFailure("invalid externaldata options")
		}
		optionEnd, _ := membershipParenEnd(suffixTokens, next+1, budget)
		if optionEnd < 0 || optionEnd == next+2 {
			membershipFailure("invalid externaldata options")
		}
		for j := next + 2; j < optionEnd; {
			budget.step()
			if j+2 >= optionEnd || !isSimpleIdentifier(suffixTokens[j].GetText()) || suffixTokens[j+1].GetTokenType() != KQLLexerASSIGN {
				membershipFailure("invalid externaldata option")
			}
			validateMembershipQuery("T | where x == "+suffixTokens[j+2].GetText(), budget)
			j += 3
			if j < optionEnd {
				if suffixTokens[j].GetTokenType() != KQLLexerCOMMA {
					membershipFailure("invalid externaldata option")
				}
				j++
			}
		}
		consumed = dataEnd + suffixPositions[suffixTokens[optionEnd].GetStop()+1]
		next = optionEnd + 1
	}
	if kind := suffixTokens[next].GetTokenType(); kind != antlr.TokenEOF && kind != KQLLexerPIPE && kind != KQLLexerRPAREN {
		membershipFailure("invalid tabular constructor suffix")
	}
	// Any following pipeline/group/suffix bytes remain in the syntax buffer.
	return consumed
}

// Only whitespace and // line comments are constructor trivia. Block comments
// remain visible to validation instead of being silently accepted as a tail.
func membershipSkipTrivia(text string, at int, budget *parseBudget) int {
	for at < len(text) {
		budget.step()
		if strings.HasPrefix(text[at:], "//") {
			for at < len(text) && text[at] != '\r' && text[at] != '\n' {
				budget.step()
				at++
			}
			continue
		}
		r, width := utf8.DecodeRuneInString(text[at:])
		if !unicode.IsSpace(r) {
			break
		}
		at += width
	}
	return at
}

// Read raw bytes because a multiline bracket body is not one lexer token.
// Whole string/comment regions may themselves contain commas and brackets.
func membershipConstructorData(text string, afterSchema int, budget *parseBudget) (start, end int) {
	start = membershipSkipTrivia(text, afterSchema, budget)
	if start >= len(text) || text[start] != '[' {
		membershipFailure("invalid tabular constructor data")
	}
	depth := 1
	for i := start + 1; i < len(text); {
		budget.step()
		if strings.HasPrefix(text[i:], "//") {
			for i < len(text) && text[i] != '\r' && text[i] != '\n' {
				budget.step()
				i++
			}
			continue
		}
		if token, ok := compoundStringAt(text, i, budget); ok {
			if !token.valid {
				membershipFailure("invalid constructor string")
			}
			i = token.end
			continue
		}
		switch text[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return start, i + 1
			}
		}
		i++
	}
	membershipFailure("unbalanced tabular constructor data")
	return 0, 0
}

type membershipConstructorSpan struct{ start, end int }

func membershipConstructorSyntax(rhs string, spans []membershipConstructorSpan, budget *parseBudget) string {
	if len(spans) == 0 {
		return rhs
	}
	var names []string
	size, last := 0, 0
	for i, span := range spans {
		budget.reserveWork(20) // Bounded decimal int temporary, before Itoa.
		number := strconv.Itoa(i)
		budget.reserveWork(len("__kql_constructor_") + len(number) + 2)
		name := "__kql_constructor_" + number + "__"
		for {
			budget.reserveWork(len(rhs))
			if !strings.Contains(rhs, name) {
				break
			}
			budget.reserveWork(len(name) + 1)
			name += "_"
		}
		names = appendTemporary(names, budget, name)
		size = compoundSize(size, span.start-last, budget)
		size = compoundSize(size, len(name)+2, budget)
		last = span.end
	}
	size = compoundSize(size, len(rhs)-last, budget)
	budget.reserveWork(size)
	var out strings.Builder
	out.Grow(size)
	last = 0
	for i, span := range spans {
		budget.step()
		out.WriteString(rhs[last:span.start])
		out.WriteByte(' ')
		out.WriteString(names[i])
		out.WriteByte(' ')
		last = span.end
	}
	out.WriteString(rhs[last:])
	return out.String()
}

func membershipSplitValues(data string, budget *parseBudget) []string {
	var values []string
	start := membershipSkipTrivia(data, 0, budget)
	depth, content := 0, false
	for i := start; i < len(data); {
		budget.step()
		if strings.HasPrefix(data[i:], "//") {
			for i < len(data) && data[i] != '\r' && data[i] != '\n' {
				budget.step()
				i++
			}
			continue
		}
		if token, ok := compoundStringAt(data, i, budget); ok {
			if !token.valid {
				membershipFailure("invalid constructor string")
			}
			content, i = true, token.end
			continue
		}
		ch := data[i]
		switch ch {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
			if depth < 0 {
				membershipFailure("unbalanced constructor value")
			}
		case ',':
			if depth == 0 {
				if !content {
					membershipFailure("empty constructor value")
				}
				budget.reserveOutput(1)
				values = appendTemporary(values, budget, strings.TrimSpace(data[start:i]))
				start = membershipSkipTrivia(data, i+1, budget)
				content, i = false, start
				continue
			}
		}
		if ch != ' ' && ch != '\t' && ch != '\r' && ch != '\n' {
			content = true
		}
		i++
	}
	if depth != 0 {
		membershipFailure("unbalanced constructor value")
	}
	if content {
		budget.reserveOutput(1)
		values = appendTemporary(values, budget, strings.TrimSpace(data[start:]))
	}
	return values
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
					if owner.GetText() != entry.marker {
						membershipFailure("protected atom does not own complete comparison")
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

func unprotectedMembershipFilter(tree antlr.Tree, offsets []int, budget *parseBudget) bool {
	if len(offsets) == 0 {
		return false
	}
	var visit func(antlr.Tree) bool
	visit = func(node antlr.Tree) bool {
		budget.step()
		if comparison, ok := node.(IComparisonExpressionContext); ok && isKQLFilterContext(comparison, budget) {
			start, stop := comparison.GetStart(), comparison.GetStop()
			if start != nil && stop != nil {
				for _, offset := range offsets {
					budget.step()
					if start.GetStart() <= offset && offset <= stop.GetStop() {
						return true
					}
				}
			}
		}
		for i := 0; i < node.GetChildCount(); i++ {
			if visit(node.GetChild(i)) {
				return true
			}
		}
		return false
	}
	return visit(tree)
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
