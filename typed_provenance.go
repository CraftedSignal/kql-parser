package kql

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/antlr4-go/antlr/v4"
)

type literalRewrite struct {
	start, end  int
	replacement string
}

func rewriteLiteralSources(query string, rewrites []literalRewrite, p *typedProtection, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	if len(rewrites) == 0 {
		return query
	}
	var out strings.Builder
	last := 0
	var recorder *kqlSourceRecorder
	if len(recorders) > 0 && recorders[0] != nil && recorders[0].Source.Text == query {
		recorder = recorders[0]
	}
	for _, rewrite := range rewrites {
		budget.step()
		if rewrite.start < last {
			continue
		}
		out.WriteString(query[last:rewrite.start])
		marker := p.mark(letOrigin{source: query[rewrite.start:rewrite.end], normalized: rewrite.replacement}, budget)
		out.WriteString(marker)
		out.WriteString(rewrite.replacement)
		if recorder != nil {
			recorder.replace(rewrite.start, rewrite.end, marker+rewrite.replacement, "")
		}
		last = rewrite.end
	}
	out.WriteString(query[last:])
	result := out.String()
	if recorder != nil && len(recorder.Edits) > 0 {
		*recorder.Source = *applyKQLSourceEdits(recorder.Source, recorder.Edits, budget)
		if recorder.Source.Text != result {
			sourceAbort("typed literal edit mismatch")
		}
	}
	return result
}

func protectedKQLBytes(query string, budget *parseBudget) []bool {
	protected := make([]bool, len(query))
	for i := 0; i < len(query); {
		budget.step()
		end := i
		switch {
		case isStringLiteralStart(query, i):
			end = stringLiteralEnd(query, i)
		case strings.HasPrefix(query[i:], "//"):
			end = i + 2
			for end < len(query) && query[end] != '\n' {
				end++
			}
		case strings.HasPrefix(query[i:], "/*"):
			if close := strings.Index(query[i+2:], "*/"); close >= 0 {
				end = i + 2 + close + 2
			}
		}
		if end <= i {
			i++
			continue
		}
		for j := i; j < end; j++ {
			protected[j] = true
		}
		i = end
	}
	return protected
}

func markedRegexLiterals(query string, pattern *regexp.Regexp, replacement string, p *typedProtection, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	protected := protectedKQLBytes(query, budget)
	var rewrites []literalRewrite
	for _, loc := range pattern.FindAllStringIndex(query, -1) {
		budget.step()
		if protected[loc[0]] {
			continue
		}
		old := query[loc[0]:loc[1]]
		next := pattern.ReplaceAllString(old, replacement)
		if next != old {
			rewrites = append(rewrites, literalRewrite{loc[0], loc[1], next})
		}
	}
	return rewriteLiteralSources(query, rewrites, p, budget, recorders...)
}

func replaceRegexOutsideProtected(query string, pattern *regexp.Regexp, replacement string, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	var recorder *kqlSourceRecorder
	if len(recorders) != 0 {
		recorder = recorders[0]
	}
	protected := protectedKQLBytes(query, budget)
	var out strings.Builder
	last := 0
	for _, loc := range pattern.FindAllStringSubmatchIndex(query, -1) {
		budget.step()
		blocked := false
		for i := loc[0]; i < loc[1]; i++ {
			if protected[i] {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		out.WriteString(query[last:loc[0]])
		rewritten := string(pattern.ExpandString(nil, replacement, query, loc))
		out.WriteString(rewritten)
		if recorder != nil {
			recorder.Replace(loc[0], loc[1], rewritten, "")
		}
		last = loc[1]
	}
	if last == 0 {
		return query
	}
	out.WriteString(query[last:])
	return finishKQLRecorded(query, out.String(), recorder)
}

var typedTimespanSuffix = regexp.MustCompile(`(?i)\b[0-9]+(?:minutes|minute|min|seconds|second|sec|hours|hour|days|day)\b`)
var typedTrailingDecimal = regexp.MustCompile(`[0-9]+\.`)
var typedBracketReference = regexp.MustCompile(`\b[0-9]*[A-Za-z_][A-Za-z0-9_]*(?:\.)?\[(?:'[^'\r\n]*'|"[^"\r\n]*"|[0-9]+)\]`)
var typedNumericReference = regexp.MustCompile(`\b[0-9]+[A-Za-z_][A-Za-z0-9_]*\b`)

func completeBracketReferenceEnd(query string, end int, budget *parseBudget) int {
	for end < len(query) {
		budget.step()
		open := end
		if query[open] == '.' {
			open++
		}
		if open >= len(query) || query[open] != '[' {
			break
		}
		depth := 1
		i := open + 1
		for i < len(query) && depth > 0 {
			budget.step()
			if isStringLiteralStart(query, i) {
				if next := stringLiteralEnd(query, i); next > i {
					i = next
					continue
				}
			}
			switch query[i] {
			case '[':
				depth++
			case ']':
				depth--
			}
			i++
		}
		if depth != 0 {
			break
		}
		end = i
	}
	return end
}

func (p *typedProtection) precedingBracketOrigin(query string, at int) bool {
	if at < 4 || query[at-1] != ' ' || query[at-3:at-1] != "*/" {
		return false
	}
	if start := strings.LastIndex(query[:at-1], "/*"); start >= 0 {
		origin, ok := p.markers[query[start:at-1]]
		return ok && origin.bracketReference
	}
	return false
}

func provenBracketReference(source, normalized string) string {
	open := strings.IndexByte(source, '[')
	if open < 0 || !strings.HasSuffix(source, "]") {
		return ""
	}
	base := strings.TrimSuffix(source[:open], ".")
	quoted := source[open+1 : len(source)-1]
	if len(quoted) < 2 || quoted[0] != quoted[len(quoted)-1] || quoted[0] != '\'' && quoted[0] != '"' {
		return ""
	}
	key := quoted[1 : len(quoted)-1]
	if !isValidPropertyKey(key) || grammarReservedWords[strings.ToLower(key)] || normalized != base+"."+key {
		return ""
	}
	return normalized
}

func markReferenceSources(query string, pattern *regexp.Regexp, transform func(string) string, p *typedProtection, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	protected := protectedKQLBytes(query, budget)
	var out strings.Builder
	last := 0
	var recorder *kqlSourceRecorder
	if len(recorders) > 0 && recorders[0] != nil && recorders[0].Source.Text == query {
		recorder = recorders[0]
	}
	for _, loc := range pattern.FindAllStringIndex(query, -1) {
		budget.step()
		if loc[0] < last {
			continue
		}
		if protected[loc[0]] {
			continue
		}
		if pattern == typedBracketReference {
			loc[1] = completeBracketReferenceEnd(query, loc[1], budget)
		}
		if pattern == typedNumericReference && p.precedingBracketOrigin(query, loc[0]) {
			continue
		}
		original := query[loc[0]:loc[1]]
		normalized := transform(original)
		if normalized == original {
			continue
		}
		origin := letOrigin{source: original, normalized: normalized}
		replacement := original
		switch pattern {
		case typedBracketReference:
			origin.referenceValue = provenBracketReference(original, normalized)
			origin.opaqueReference = origin.referenceValue == ""
			origin.bracketReference = true
			if origin.opaqueReference {
				replacement = p.prefix + "opaque_" + strconv.Itoa(p.next) + "__"
				origin.normalized = replacement
			} else {
				origin.normalized = fixNumericIdentifiers(normalized)
			}
		case typedNumericReference:
			origin.referenceValue = original
		}
		out.WriteString(query[last:loc[0]])
		marker := p.mark(origin, budget)
		out.WriteString(marker)
		out.WriteString(replacement)
		if recorder != nil {
			recorder.replace(loc[0], loc[1], marker+replacement, "")
		}
		last = loc[1]
	}
	if last == 0 {
		return query
	}
	out.WriteString(query[last:])
	result := out.String()
	if recorder != nil && len(recorder.Edits) > 0 {
		*recorder.Source = *applyKQLSourceEdits(recorder.Source, recorder.Edits, budget)
		if recorder.Source.Text != result {
			sourceAbort("typed reference edit mismatch")
		}
	}
	return result
}

func normalizeTimespanLiteralsTracked(query string, p *typedProtection, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	protected := protectedKQLBytes(query, budget)
	var rewrites []literalRewrite
	for _, loc := range typedTimespanSuffix.FindAllStringIndex(query, -1) {
		budget.step()
		if protected[loc[0]] {
			continue
		}
		old := query[loc[0]:loc[1]]
		next := normalizeTimespanLiterals(old)
		if next != old {
			rewrites = append(rewrites, literalRewrite{loc[0], loc[1], next})
		}
	}
	return rewriteLiteralSources(query, rewrites, p, budget, recorders...)
}

func normalizeTrailingDecimalsTracked(query string, p *typedProtection, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	protected := protectedKQLBytes(query, budget)
	var rewrites []literalRewrite
	for _, loc := range typedTrailingDecimal.FindAllStringIndex(query, -1) {
		budget.step()
		if protected[loc[0]] || loc[1] < len(query) && (isDigit(query[loc[1]]) || query[loc[1]] == '.') {
			continue
		}
		if !isStandaloneNumericLiteralBeforeDot(query, loc[1]-1) || !isTrailingDecimalBoundary(query, loc[1]) {
			continue
		}
		rewrites = append(rewrites, literalRewrite{loc[0], loc[1], query[loc[0]:loc[1]] + "0"})
	}
	return rewriteLiteralSources(query, rewrites, p, budget, recorders...)
}

func joinAdjacentStringLiteralsTracked(query string, p *typedProtection, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	var rewrites []literalRewrite
	for i := 0; i < len(query); {
		budget.step()
		if strings.HasPrefix(query[i:], "//") {
			for i < len(query) && query[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(query[i:], "/*") {
			if j := strings.Index(query[i+2:], "*/"); j >= 0 {
				i += j + 4
				continue
			}
		}
		if !isStringLiteralStart(query, i) {
			i++
			continue
		}
		start, end := i, stringLiteralEnd(query, i)
		if end <= i {
			i++
			continue
		}
		count := 1
		for {
			next := end
			for next < len(query) && strings.IndexByte(" \t\r\n", query[next]) >= 0 {
				next++
			}
			if !isStringLiteralStart(query, next) || !sameStringLiteralKind(query, start, next) {
				break
			}
			nextEnd := stringLiteralEnd(query, next)
			if nextEnd <= next {
				break
			}
			end = nextEnd
			count++
		}
		if count > 1 {
			rewrites = append(rewrites, literalRewrite{start, end, joinAdjacentStringLiterals(query[start:end])})
		}
		i = end
	}
	return rewriteLiteralSources(query, rewrites, p, budget, recorders...)
}

func markNullEscapeStrings(query string, p *typedProtection, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	var rewrites []literalRewrite
	for i := 0; i < len(query); {
		budget.step()
		if strings.HasPrefix(query[i:], "//") {
			for i < len(query) && query[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(query[i:], "/*") {
			if end := strings.Index(query[i+2:], "*/"); end >= 0 {
				i += end + 4
				continue
			}
		}
		if !isStringLiteralStart(query, i) {
			i++
			continue
		}
		end := stringLiteralEnd(query, i)
		if end <= i {
			i++
			continue
		}
		if strings.Contains(query[i:end], `\0`) {
			rewrites = append(rewrites, literalRewrite{i, end, query[i:end]})
		}
		i = end
	}
	return rewriteLiteralSources(query, rewrites, p, budget, recorders...)
}

// letOrigin is attached to a proven inlining occurrence, never recovered by
// comparing decoded values with let definitions.
type letOrigin struct {
	source, binding, listSource, normalized, referenceValue string
	opaqueReference, bracketReference                       bool
}

type typedProtection struct {
	markers       map[string]letOrigin
	shields       map[string]string
	byStart       map[int]letOrigin
	markerByStart map[int]string
	starts        []int
	prefix        string
	next          int
	budget        *parseBudget
}

func (p *typedProtection) normalizesField(authored, normalized string) bool {
	if p == nil {
		return false
	}
	for _, origin := range p.byStart {
		p.budget.step()
		if origin.referenceValue == authored && origin.normalized == normalized {
			return true
		}
	}
	return false
}

func newTypedProtection(query string, budgets ...*parseBudget) *typedProtection {
	prefix := "__kql_typed_origin_"
	for strings.Contains(query, prefix) {
		prefix += "_"
	}
	return &typedProtection{markers: make(map[string]letOrigin), shields: make(map[string]string), prefix: prefix, budget: suppliedBudget(budgets)}
}

func (p *typedProtection) shieldNative(query string, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	budget.step()
	selected := skipLetStatements(query)
	var stream *antlr.CommonTokenStream
	if budget != nil {
		stream = preflight(query, budget, false, nil)
	} else {
		lexer := NewKQLLexer(antlr.NewInputStream(query))
		lexer.RemoveErrorListeners()
		stream = antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
		stream.Fill()
	}
	positions := searchRuneBytes(query, budget)
	var out strings.Builder
	last := 0
	var sourceEdits []kqlSourceEdit
	for i := 0; i < stream.Size(); i++ {
		budget.step()
		token := stream.Get(i)
		if token == nil || token.GetStart() < 0 {
			continue
		}
		switch token.GetTokenType() {
		case KQLLexerGUID_LITERAL, KQLLexerDYNAMIC_LITERAL, KQLLexerDATETIME_LITERAL, KQLLexerTIMESPAN_LITERAL:
		default:
			continue
		}
		start, end := positions[token.GetStart()], positions[token.GetStop()+1]
		if start < selected || start < last {
			continue
		}
		marker := p.prefix + "shield_" + strconv.Itoa(p.next) + "__"
		p.next++
		budget.reserveOutput(1 + len(marker) + end - start)
		p.shields[marker] = query[start:end]
		if len(recorders) > 0 && recorders[0] != nil && recorders[0].Source.Text == query {
			recorder := recorders[0]
			recorder.Source.Protected[marker] = kqlPiecesInRange(recorder.Source, start, end, budget)
			owner := kqlOccurrenceAt(recorder.Source, start, budget)
			sourceEdits = append(sourceEdits, kqlSourceEdit{Start: start, End: end, Parts: []kqlSourcePart{{Text: marker, Origin: originOfEdit(recorder.Source, kqlSourceEdit{Start: start, End: end}, budget), OccurrenceID: owner}}})
		}
		out.WriteString(query[last:start])
		out.WriteString(marker)
		last = end
	}
	if last == 0 {
		return query
	}
	out.WriteString(query[last:])
	result := out.String()
	if len(sourceEdits) > 0 {
		recorder := recorders[0]
		*recorder.Source = *applyKQLSourceEdits(recorder.Source, sourceEdits, budget)
		if recorder.Source.Text != result {
			sourceAbort("typed shield edit mismatch")
		}
	}
	return result
}

func (p *typedProtection) restoreShields(query string, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	budget.step()
	if len(p.shields) == 0 {
		return query
	}
	prefix := p.prefix + "shield_"
	protected := protectedKQLBytes(query, budget)
	var out strings.Builder
	offset := 0
	originalInput := query
	var sourceEdits []kqlSourceEdit
	for len(query) > 0 {
		budget.step()
		index := strings.Index(query, prefix)
		if index < 0 {
			out.WriteString(query)
			break
		}
		out.WriteString(query[:index])
		query = query[index:]
		if protected[offset+index] {
			out.WriteString(prefix)
			query = query[len(prefix):]
			offset += index + len(prefix)
			continue
		}
		end := strings.Index(query[len(prefix):], "__")
		if end < 0 {
			out.WriteString(query[:len(prefix)])
			query = query[len(prefix):]
			offset += index + len(prefix)
			continue
		}
		end += len(prefix)
		marker := query[:end+2]
		if source, ok := p.shields[marker]; ok {
			out.WriteString(source)
			if len(recorders) > 0 && recorders[0] != nil && recorders[0].Source.Text == originalInput {
				var parts []kqlSourcePart
				for _, piece := range recorders[0].Source.Protected[marker] {
					budget.step()
					parts = append(parts, kqlSourcePart{Text: piece.Text, Origin: piece.Origin, OccurrenceID: piece.OccurrenceID})
				}
				if len(parts) == 0 {
					sourceAbort("typed shield lost protected source")
				}
				sourceEdits = append(sourceEdits, kqlSourceEdit{Start: offset + index, End: offset + index + end + 2, Parts: parts})
			}
		} else {
			out.WriteString(marker)
		}
		query = query[end+2:]
		offset += index + end + 2
	}
	result := out.String()
	if len(sourceEdits) > 0 {
		recorder := recorders[0]
		*recorder.Source = *applyKQLSourceEdits(recorder.Source, sourceEdits, budget)
		if recorder.Source.Text != result {
			sourceAbort("typed shield restoration edit mismatch")
		}
	}
	return result
}

func (p *typedProtection) mark(origin letOrigin, budget *parseBudget) string {
	budget.step()
	marker := "/*" + p.prefix + strconv.Itoa(p.next) + "__*/"
	p.next++
	budget.reserveOutput(1 + len(marker) + len(origin.source) + len(origin.binding) + len(origin.listSource))
	p.markers[marker] = origin
	// A separator prevents two adjacent block markers from forming */*/*,
	// whose middle // is misread as a line comment by legacy normalizers.
	return marker + " "
}

func (p *typedProtection) restore(query string, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	budget.step()
	if len(p.markers) == 0 {
		return query
	}
	p.byStart = make(map[int]letOrigin, len(p.markers))
	p.markerByStart = make(map[int]string, len(p.markers))
	p.starts = p.starts[:0]
	var out strings.Builder
	runes := 0
	var sourceEdits []kqlSourceEdit
	for i := 0; i < len(query); {
		budget.step()
		if isStringLiteralStart(query, i) {
			end := stringLiteralEnd(query, i)
			if end > i {
				out.WriteString(query[i:end])
				runes += utf8.RuneCountInString(query[i:end])
				i = end
				continue
			}
		}
		if strings.HasPrefix(query[i:], "/*") {
			if end := strings.Index(query[i+2:], "*/"); end >= 0 {
				end += i + 4
				marker := query[i:end]
				if origin, ok := p.markers[marker]; ok {
					markerStart := i
					if _, present := p.byStart[runes]; !present {
						budget.reserveOutput(1)
						p.byStart[runes] = origin
						p.markerByStart[runes] = marker
						p.starts = append(p.starts, runes)
					}
					i = end
					if i < len(query) && query[i] == ' ' {
						i++ // Consume the marker's guaranteed separator.
					}
					if len(recorders) > 0 && recorders[0] != nil {
						sourceEdits = append(sourceEdits, kqlSourceEdit{Start: markerStart, End: i})
					}
					continue
				}
				out.WriteString(marker)
				runes += utf8.RuneCountInString(marker)
				i = end
				continue
			}
		}
		out.WriteByte(query[i])
		if query[i]&0xc0 != 0x80 {
			runes++
		}
		i++
	}
	sort.Ints(p.starts)
	result := out.String()
	if len(sourceEdits) > 0 && recorders[0].Source.Text == query {
		recorder := recorders[0]
		*recorder.Source = *applyKQLSourceEdits(recorder.Source, sourceEdits, budget)
		if recorder.Source.Text != result {
			sourceAbort("typed marker restoration edit mismatch")
		}
	}
	return result
}

// reinsert carries only authenticated occurrences into a recursively parsed
// tabular child. The child normalizer knows the matching marker records, so a
// let's defining source and binding survive without matching decoded values.
func (p *typedProtection) reinsert(ctx antlr.ParserRuleContext) (string, map[string]letOrigin) {
	p.budget.step()
	text := kqlSourceText(ctx, p.budget)
	if len(p.starts) == 0 || ctx.GetStart() == nil || ctx.GetStop() == nil {
		return text, nil
	}
	start, stop := ctx.GetStart().GetStart(), ctx.GetStop().GetStop()
	positions := searchRuneBytes(text, p.budget)
	var out strings.Builder
	var inherited map[string]letOrigin
	last := 0
	for i := sort.SearchInts(p.starts, start); i < len(p.starts) && p.starts[i] <= stop; i++ {
		p.budget.step()
		relative := p.starts[i] - start
		if relative < 0 || relative >= len(positions) {
			continue
		}
		at := positions[relative]
		if at < last {
			continue
		}
		marker := p.markerByStart[p.starts[i]]
		if marker == "" {
			continue
		}
		if inherited == nil {
			inherited = make(map[string]letOrigin)
		}
		inherited[marker] = p.byStart[p.starts[i]]
		out.WriteString(text[last:at])
		out.WriteString(marker)
		out.WriteByte(' ')
		last = at
	}
	if inherited == nil {
		return text, nil
	}
	out.WriteString(text[last:])
	return out.String(), inherited
}

func (p *typedProtection) apply(operand *Operand, tree antlr.Tree) *Operand {
	if p == nil || operand == nil || tree == nil {
		return operand
	}
	p.budget.step()
	ctx, ok := tree.(antlr.ParserRuleContext)
	if !ok || ctx.GetStart() == nil {
		return operand
	}
	copy := *operand
	copy.Source = p.sourceOf(ctx)
	if origin, ok := p.byStart[ctx.GetStart().GetStart()]; ok {
		copy.Binding = origin.binding
	}
	if copy.Kind == OperandField {
		start, stop := ctx.GetStart().GetStart(), ctx.GetStop().GetStop()
		for i := sort.SearchInts(p.starts, start); i < len(p.starts) && p.starts[i] <= stop; i++ {
			p.budget.step()
			origin := p.byStart[p.starts[i]]
			if origin.referenceValue != "" {
				copy.Value = strings.Replace(copy.Value, origin.normalized, origin.referenceValue, 1)
			}
		}
	}
	if copy.Kind == OperandNumber {
		copy.Value = copy.Source
	}
	if copy.Kind == OperandTyped && len(copy.Items) == 1 {
		if child := p.constructorArgument(tree); child != nil {
			copy.Items = append([]Operand(nil), copy.Items...)
			copy.Items[0] = *p.apply(&copy.Items[0], child)
		}
	}
	return &copy
}

func (p *typedProtection) hasOpaqueReference(ctx antlr.ParserRuleContext) bool {
	if p == nil || ctx == nil || ctx.GetStart() == nil || ctx.GetStop() == nil {
		return false
	}
	start, stop := ctx.GetStart().GetStart(), ctx.GetStop().GetStop()
	for i := sort.SearchInts(p.starts, start); i < len(p.starts) && p.starts[i] <= stop; i++ {
		p.budget.step()
		if p.byStart[p.starts[i]].opaqueReference {
			return true
		}
	}
	return false
}

func (p *typedProtection) hasOrigin(ctx antlr.ParserRuleContext) bool {
	if p == nil || ctx == nil || ctx.GetStart() == nil || ctx.GetStop() == nil {
		return false
	}
	start, stop := ctx.GetStart().GetStart(), ctx.GetStop().GetStop()
	i := sort.SearchInts(p.starts, start)
	return i < len(p.starts) && p.starts[i] <= stop
}

func (p *typedProtection) opaqueValueIn(ctx antlr.ParserRuleContext, value string) bool {
	if p == nil || ctx == nil || ctx.GetStart() == nil || ctx.GetStop() == nil {
		return false
	}
	start, stop := ctx.GetStart().GetStart(), ctx.GetStop().GetStop()
	for i := sort.SearchInts(p.starts, start); i < len(p.starts) && p.starts[i] <= stop; i++ {
		p.budget.step()
		origin := p.byStart[p.starts[i]]
		if origin.opaqueReference && origin.normalized == value {
			return true
		}
	}
	return false
}

func (p *typedProtection) provesReferenceIn(tree antlr.ParserRuleContext, value string) bool {
	if p == nil || tree == nil || tree.GetStart() == nil || tree.GetStop() == nil {
		return false
	}
	start, stop := tree.GetStart().GetStart(), tree.GetStop().GetStop()
	for i := sort.SearchInts(p.starts, start); i < len(p.starts) && p.starts[i] <= stop; i++ {
		p.budget.step()
		origin := p.byStart[p.starts[i]]
		if !origin.opaqueReference && origin.referenceValue == value {
			return true
		}
	}
	return false
}

func (p *typedProtection) constructorArgument(tree antlr.Tree) antlr.Tree {
	p.budget.step()
	if call, ok := tree.(IFunctionCallContext); ok {
		if args := call.ArgumentList(); args != nil && len(args.AllArgument()) == 1 {
			return args.AllArgument()[0].Expression()
		}
	}
	for i := 0; i < tree.GetChildCount(); i++ {
		if child := p.constructorArgument(tree.GetChild(i)); child != nil {
			return child
		}
	}
	return nil
}

func (p *typedProtection) sourceOf(ctx antlr.ParserRuleContext) string {
	p.budget.step()
	text := kqlSourceText(ctx)
	if len(p.starts) == 0 || ctx.GetStart() == nil || ctx.GetStop() == nil {
		return text
	}
	start, stop := ctx.GetStart().GetStart(), ctx.GetStop().GetStop()
	first := sort.SearchInts(p.starts, start)
	if first == len(p.starts) || p.starts[first] > stop {
		return text
	}
	positions := searchRuneBytes(text, p.budget)
	var out strings.Builder
	last := 0
	for i := first; i < len(p.starts) && p.starts[i] <= stop; i++ {
		p.budget.step()
		relative := p.starts[i] - start
		if relative < 0 || relative >= len(positions) {
			continue
		}
		at := positions[relative]
		if at < last {
			continue
		}
		origin := p.byStart[p.starts[i]]
		end := at
		if origin.normalized != "" && strings.HasPrefix(text[at:], origin.normalized) {
			end += len(origin.normalized)
		} else if isStringLiteralStart(text, at) {
			end = stringLiteralEnd(text, at)
		} else {
			for end < len(text) && !strings.ContainsRune(" \t\r\n,)]}+-*/", rune(text[end])) {
				end++
			}
		}
		if end <= at {
			continue
		}
		out.WriteString(text[last:at])
		out.WriteString(origin.source)
		last = end
	}
	out.WriteString(text[last:])
	return out.String()
}

func (p *typedProtection) listOrigin(items []Operand) (string, string) {
	if p == nil || len(items) == 0 || items[0].Binding == "" {
		return "", ""
	}
	binding := items[0].Binding
	for _, item := range items {
		if item.Binding != binding {
			return "", ""
		}
	}
	for _, origin := range p.byStart {
		if origin.binding == binding && origin.listSource != "" {
			return origin.listSource, binding
		}
	}
	return "", ""
}
