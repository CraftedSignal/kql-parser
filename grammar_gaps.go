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

// joinAdjacentStringLiterals joins adjacent string literals of the same kind,
// which KQL concatenates: "a" "b" is "ab".
func joinAdjacentStringLiterals(query string) string {
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
			for {
				next := end
				for next < len(query) && strings.IndexByte(" \t\r\n", query[next]) >= 0 {
					next++
				}
				if !isStringLiteralStart(query, next) || !sameStringLiteralKind(query, i, next) {
					break
				}
				nextEnd := stringLiteralEnd(query, next)
				if nextEnd < 0 {
					break
				}
				// Drop the closing quote, the space and the next opening quote.
				opening := next + 1
				if query[next] == '@' {
					opening++
				}
				b.WriteString(query[last : end-1])
				last = opening
				end = nextEnd
			}
			i = end
		default:
			i++
		}
	}
	if last == 0 {
		return query
	}
	b.WriteString(query[last:])
	return b.String()
}

// sameStringLiteralKind reports whether the literals starting at i and j are
// both verbatim or both regular, with the same quote character.
func sameStringLiteralKind(query string, i, j int) bool {
	verbatimI, verbatimJ := query[i] == '@', query[j] == '@'
	quoteI, quoteJ := query[i], query[j]
	if verbatimI {
		quoteI = query[i+1]
	}
	if verbatimJ {
		quoteJ = query[j+1]
	}
	return verbatimI == verbatimJ && quoteI == quoteJ
}

var makeGraphEdgeRe = regexp.MustCompile(`(?i)(\bmake-graph\s+[A-Za-z_][A-Za-z0-9_]*\s*)-->?(\s*[A-Za-z_])`)

// normalizeMakeGraphEdges writes the make-graph edge, "-->" (directed) or
// "--" (undirected), as the "=>" the grammar expects.
func normalizeMakeGraphEdges(query string) string {
	return makeGraphEdgeRe.ReplaceAllString(query, "${1}=>${2}")
}

var bracketedUnionRe = regexp.MustCompile(`(?i)\(\s*union\b`)

// normalizeBracketedUnion gives a union that opens a bracketed tabular
// expression, (union A, B), the placeholder table the grammar requires before
// union, as for a query that starts with union.
func normalizeBracketedUnion(query string) string {
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
	return b.String()
}

var datetimeLiteralRe = regexp.MustCompile(`(?i)^datetime\s*\([^)]*\)`)

// joinConditionAndRe matches "and" between join conditions, on the same line
// or the next: on $left.A == $right.B and $left.C == $right.D.
var joinConditionAndRe = regexp.MustCompile(`(?i)\s+and\s+\$(left|right)\.`)

var graphMatchStageRe = regexp.MustCompile(`(?i)\|\s*graph-match\b`)

// stripGraphMatchStages removes graph-match stages, whose patterns, such as
// (a)-[e*1..3]->(b), the grammar does not support. Their syntax is not
// validated.
func stripGraphMatchStages(query string) string {
	for {
		loc := graphMatchStageRe.FindStringIndex(query)
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
		query = query[:loc[0]] + query[end:]
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
	"or": true, "order": true, "pack": true, "partition": true, "pattern": true, "restrict": true,
	"sample": true, "scan": true, "serialize": true, "startswith": true, "toscalar": true,
	"typeof": true, "view": true, "withsource": true,
}
