package kql

import (
	"regexp"
	"strings"
)

// These normalizations rewrite valid KQL the grammar does not accept into an
// equivalent form it does.

var datetimeWithSpaceRe = regexp.MustCompile(`(?i)\bdatetime\(\s*(\d{4}-\d{1,2}-\d{1,2})\s+(\d{1,2}:\d{2}(?::\d{2}(?:\.\d+)?)?)\s*\)`)

// normalizeDatetimeLiterals writes datetime(2026-02-04 10:00:00) as
// datetime(2026-02-04T10:00:00).
func normalizeDatetimeLiterals(query string) string {
	return datetimeWithSpaceRe.ReplaceAllString(query, "datetime(${1}T${2})")
}

// joinAdjacentStringLiterals applies the same compound-token plan as extraction.
func joinAdjacentStringLiterals(query string, budgets ...*parseBudget) string {
	budget := suppliedBudget(budgets)
	rewrites := planCompoundStrings(query, budget)
	return applyCompoundStrings(query, rewrites, nil, budget)
}

type compoundStringToken struct {
	start, end, opening, width        int
	quote                             byte
	verbatim, hidden, valid, boundary bool
}

// compoundStringAt consumes a whole token, including malformed escape text.
// Invalid tokens cannot expose a valid-looking suffix to the run planner.
func compoundStringAt(query string, start int, budget *parseBudget) (compoundStringToken, bool) {
	t := compoundStringToken{start: start, end: len(query), width: 1, valid: true}
	i := start
	if i >= len(query) {
		return t, false
	}
	if (query[i] == 'h' || query[i] == 'H') && strings.HasPrefix(query[i+1:], "```") {
		// Obfuscation prefixes are not admitted on multiline tokens. Consume
		// the whole spelling without repairing its otherwise valid suffix.
		whole, _ := compoundStringAt(query, i+1, budget)
		whole.start, whole.valid, whole.boundary = start, false, true
		return whole, true
	}
	if (query[i] == 'h' || query[i] == 'H') && i+1 < len(query) && isStringLiteralStart(query, i+1) {
		t.hidden = true
		i++
	}
	if strings.HasPrefix(query[i:], "```") || strings.HasPrefix(query[i:], `"""`) {
		t.quote, t.width, t.opening = query[i], 3, i+3
		t.boundary = query[i] == '"'
		for j := i + 3; j < len(query); j++ {
			budget.step()
			if len(query)-j >= 3 && query[j] == t.quote && query[j+1] == t.quote && query[j+2] == t.quote {
				t.end = j + 3
				return t, true
			}
		}
		t.valid = false
		return t, true
	}
	if !isStringLiteralStart(query, i) {
		return t, false
	}
	t.verbatim = query[i] == '@'
	if t.verbatim {
		i++
	}
	t.quote, t.opening = query[i], i+1
	for i++; i < len(query); i++ {
		budget.step()
		if query[i] == t.quote {
			if t.verbatim && i+1 < len(query) && query[i+1] == t.quote {
				i++
				continue
			}
			t.end = i + 1
			return t, true
		}
		if t.verbatim {
			continue
		}
		if query[i] == '\r' || query[i] == '\n' {
			t.valid = false
		}
		if query[i] != '\\' {
			continue
		}
		i++
		if i >= len(query) {
			break
		}
		switch query[i] {
		case 'b', 't', 'n', 'f', 'r', '"', '\'', '\\', '0':
		case 'x', 'u':
			n := 2
			if query[i] == 'u' {
				n = 4
			}
			for j := 0; j < n; j++ {
				if i+1 >= len(query) || !compoundHex(query[i+1]) {
					t.valid = false
					break
				}
				i++
				budget.step()
			}
		default:
			t.valid = false
		}
	}
	t.valid = false
	return t, true
}

func compoundHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func compoundSize(size, addition int, budget *parseBudget) int {
	ceiling := int(^uint(0) >> 1)
	if budget != nil {
		ceiling = budget.limits.maxInputBytes
	}
	if addition < 0 || addition > ceiling-size {
		budget.reject("input bytes")
	}
	return size + addition
}

func planCompoundStrings(query string, budget *parseBudget) []literalRewrite {
	var rewrites []literalRewrite
	for i := 0; i < len(query); {
		budget.step()
		if strings.HasPrefix(query[i:], "//") {
			for i < len(query) && query[i] != '\n' {
				budget.step()
				i++
			}
			continue
		}
		if strings.HasPrefix(query[i:], "/*") {
			i += 2
			for i < len(query) && !strings.HasPrefix(query[i:], "*/") {
				budget.step()
				i++
			}
			if i < len(query) {
				i += 2
			}
			continue
		}
		first, ok := compoundStringAt(query, i, budget)
		if !ok {
			i++
			continue
		}
		if first.boundary {
			i = first.end
			continue
		}
		tokens := appendTemporary([]compoundStringToken(nil), budget, first)
		end, valid, homogeneous := first.end, first.valid, true
		for {
			next := end
			for next < len(query) {
				if strings.IndexByte(" \t\r\n", query[next]) >= 0 {
					budget.step()
					next++
					continue
				}
				if strings.HasPrefix(query[next:], "//") {
					for next < len(query) && query[next] != '\r' && query[next] != '\n' {
						budget.step()
						next++
					}
					continue
				}
				break
			}
			token, found := compoundStringAt(query, next, budget)
			if !found || token.boundary {
				break
			}
			tokens = appendTemporary(tokens, budget, token)
			valid = valid && token.valid
			homogeneous = homogeneous && token.quote == first.quote && token.verbatim == first.verbatim && token.width == first.width && token.hidden == first.hidden
			end = token.end
		}
		if valid && len(tokens) > 1 {
			replacement := renderCompoundString(query, tokens, homogeneous, budget)
			rewrites = appendTemporary(rewrites, budget, literalRewrite{i, end, replacement})
		}
		i = end
	}
	return rewrites
}

func renderCompoundString(query string, tokens []compoundStringToken, homogeneous bool, budget *parseBudget) string {
	first, last := tokens[0], tokens[len(tokens)-1]
	var out strings.Builder
	if homogeneous {
		size := first.opening - first.start + last.width
		for _, token := range tokens {
			size = compoundSize(size, token.end-token.width-token.opening, budget)
		}
		budget.reserveWork(size)
		out.Grow(size)
		out.WriteString(query[first.start:first.opening])
		for _, token := range tokens {
			budget.step()
			out.WriteString(query[token.opening : token.end-token.width])
		}
		out.WriteString(query[last.end-last.width : last.end])
		return out.String()
	}
	var values []string
	hidden, size := false, 2
	for _, token := range tokens {
		budget.step()
		hidden = hidden || token.hidden
		source := query[token.start:token.end]
		value := decodeKQLStringBudgeted(source, budget)
		values = appendTemporary(values, budget, value)
		for i := 0; i < len(value); i++ {
			budget.step()
			width := 1
			switch value[i] {
			case '\\', '"', '\n', '\r', '\t':
				width = 2
			default:
				if value[i] < 0x20 {
					width = 6
				}
			}
			size = compoundSize(size, width, budget)
		}
	}
	if hidden {
		size = compoundSize(size, 1, budget)
	}
	budget.reserveWork(size)
	out.Grow(size)
	if hidden {
		out.WriteByte('h')
	}
	out.WriteByte('"')
	const hex = "0123456789abcdef"
	for _, value := range values {
		for i := 0; i < len(value); i++ {
			budget.step()
			c := value[i]
			switch c {
			case '\\', '"':
				out.WriteByte('\\')
				out.WriteByte(c)
			case '\n':
				out.WriteString(`\n`)
			case '\r':
				out.WriteString(`\r`)
			case '\t':
				out.WriteString(`\t`)
			default:
				if c < 0x20 {
					out.WriteString(`\u00`)
					out.WriteByte(hex[c>>4])
					out.WriteByte(hex[c&15])
				} else {
					out.WriteByte(c)
				}
			}
		}
	}
	out.WriteByte('"')
	return out.String()
}

// Plan the complete normalized capacity, including provenance markers, before
// growing the builder. The markers retain their existing output/metadata owner.
func applyCompoundStrings(query string, rewrites []literalRewrite, p *typedProtection, budget *parseBudget, recorders ...*kqlSourceRecorder) string {
	if len(rewrites) == 0 {
		return query
	}
	var markers []string
	size := 0
	last := 0
	for _, rewrite := range rewrites {
		budget.step()
		size = compoundSize(size, rewrite.start-last, budget)
		if p != nil {
			// p.mark constructs this bounded decimal marker before retaining it.
			budget.reserveWork(len(p.prefix) + 32)
			marker := p.mark(letOrigin{source: query[rewrite.start:rewrite.end], normalized: rewrite.replacement}, budget)
			markers = appendTemporary(markers, budget, marker)
			size = compoundSize(size, len(marker), budget)
		}
		size = compoundSize(size, len(rewrite.replacement), budget)
		last = rewrite.end
	}
	size = compoundSize(size, len(query)-last, budget)
	budget.reserveWork(size)
	var out strings.Builder
	out.Grow(size)
	var recorder *kqlSourceRecorder
	if len(recorders) > 0 && recorders[0] != nil && recorders[0].Source.Text == query {
		recorder = recorders[0]
	}
	last = 0
	for i, rewrite := range rewrites {
		budget.step()
		out.WriteString(query[last:rewrite.start])
		if p != nil {
			out.WriteString(markers[i])
		}
		out.WriteString(rewrite.replacement)
		if recorder != nil {
			budget.reserveWork(len(markers[i]) + len(rewrite.replacement))
			recorder.replace(rewrite.start, rewrite.end, markers[i]+rewrite.replacement, "")
		}
		last = rewrite.end
	}
	out.WriteString(query[last:])
	result := out.String()
	if recorder != nil && len(recorder.Edits) > 0 {
		*recorder.Source = *applyKQLSourceEdits(recorder.Source, recorder.Edits, budget)
		if recorder.Source.Text != result {
			sourceAbort("compound literal edit mismatch")
		}
	}
	return result
}

var makeGraphEdgeRe = regexp.MustCompile(`(?i)(\bmake-graph\s+[A-Za-z_][A-Za-z0-9_]*\s*)-->?(\s*[A-Za-z_])`)

// normalizeMakeGraphEdges writes the make-graph edge, "-->" (directed) or
// "--" (undirected), as the "=>" the grammar expects.
func normalizeMakeGraphEdges(query string, recorders ...*kqlSourceRecorder) string {
	var recorder *kqlSourceRecorder
	if len(recorders) != 0 {
		recorder = recorders[0]
	}
	var b strings.Builder
	last := 0
	for _, match := range makeGraphEdgeRe.FindAllStringSubmatchIndex(kqlSyntaxView(query, recorder), -1) {
		replacement := string(makeGraphEdgeRe.ExpandString(nil, "${1}=>${2}", query, match))
		if recorder != nil {
			recorder.Replace(match[0], match[1], replacement, "normalizeMakeGraphEdges")
		}
		b.WriteString(query[last:match[0]])
		b.WriteString(replacement)
		last = match[1]
	}
	if last == 0 {
		return query
	}
	b.WriteString(query[last:])
	return finishKQLRecorded(query, b.String(), recorder)
}

var bracketedUnionRe = regexp.MustCompile(`(?i)\(\s*union\b`)

// normalizeBracketedUnion gives a union that opens a bracketed tabular
// expression, (union A, B), the placeholder table the grammar requires before
// union, as for a query that starts with union.
func normalizeBracketedUnion(query string, recorders ...*kqlSourceRecorder) string {
	var recorder *kqlSourceRecorder
	if len(recorders) != 0 {
		recorder = recorders[0]
	}
	var b strings.Builder
	last := 0
	for i := 0; i < len(query); {
		switch {
		case query[i] == '/' && i+1 < len(query) && query[i+1] == '/':
			for i < len(query) && query[i] != '\n' {
				i++
			}
		case isStringLiteralStart(query, i):
			end := stringLiteralEnd(query, i)
			if end < 0 {
				i = len(query)
				continue
			}
			i = end
		case query[i] == '(':
			if loc := bracketedUnionRe.FindStringIndex(query[i:]); loc != nil && loc[0] == 0 {
				b.WriteString(query[last : i+1])
				b.WriteString("DummyTable | ")
				recordKQLInsertion(recorder, i+1, "DummyTable | ", "normalizeBracketedUnion")
				last = i + 1
			}
			i++
		default:
			i++
		}
	}
	if last == 0 {
		return query
	}
	b.WriteString(query[last:])
	return finishKQLRecorded(query, b.String(), recorder)
}

var datetimeLiteralRe = regexp.MustCompile(`(?i)^datetime\s*\([^)]*\)`)

// joinConditionAndRe matches "and" between join conditions, on the same line
// or the next: on $left.A == $right.B and $left.C == $right.D.
var joinConditionAndRe = regexp.MustCompile(`(?i)\s+and\s+\$(left|right)\.`)

var graphMatchStageRe = regexp.MustCompile(`(?i)\|\s*graph-match\b`)

// stripGraphMatchStages removes graph-match stages, whose patterns, such as
// (a)-[e*1..3]->(b), the grammar does not support. Their syntax is not
// validated.
func stripGraphMatchStages(query string, recorders ...*kqlSourceRecorder) string {
	var recorder *kqlSourceRecorder
	if len(recorders) != 0 {
		recorder = recorders[0]
	}
	for {
		loc := graphMatchStageRe.FindStringIndex(kqlSyntaxView(query, recorder))
		if loc == nil {
			return query
		}
		end := loc[1]
		depth := 0
		for end < len(query) {
			c := query[end]
			if isStringLiteralStart(query, end) {
				next := stringLiteralEnd(query, end)
				if next < 0 {
					end = len(query)
					break
				}
				end = next
				continue
			}
			if c == '(' || c == '[' || c == '{' {
				depth++
			} else if (c == ')' || c == ']' || c == '}') && depth > 0 {
				depth--
			} else if c == '|' && depth == 0 {
				break
			}
			end++
		}
		original := query
		if recorder != nil {
			recorder.Replace(loc[0], end, "", "stripGraphMatchStages")
		}
		query = finishKQLRecorded(original, query[:loc[0]]+query[end:], recorder)
		if recorder != nil {
			recorder.Edits = nil
		}
	}
}

// grammarReservedWords are names the grammar reads as keywords, so a column or
// property with that name, written ['limit'], cannot be read bare.
var grammarReservedWords = map[string]bool{
	"access": true, "alias": true, "and": true, "between": true, "case": true, "consume": true,
	"contains": true, "datatable": true, "declare": true, "endswith": true, "facet": true,
	"find": true, "first": true, "fork": true, "getschema": true, "has": true, "hint": true,
	"iff": true, "iif": true, "in": true, "invoke": true, "isfuzzy": true, "last": true,
	"limit": true, "materialize": true, "matches": true, "not": true, "null": true, "nulls": true,
	"or": true, "order": true, "output": true, "pack": true, "partition": true, "pattern": true, "restrict": true,
	"sample": true, "scan": true, "serialize": true, "startswith": true, "toscalar": true,
	"typeof": true, "view": true, "withsource": true,
}
