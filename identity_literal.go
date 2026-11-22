package kql

import (
	"strings"
	"unicode"
)

// identityLiteral reads one complete standard/verbatim literal admitted by the
// lexer, retaining its exact tail. Legacy portable value readers intentionally
// keep their separate representation. Temporary scanning/decoding uses shared
// work; retained fields remain charged at their existing ownership sites.
func identityLiteral(raw string, budget *parseBudget) (value, tail string, ok bool) {
	budget.input(raw)
	budget.reserveWork(len(raw))
	raw = strings.TrimLeftFunc(raw, unicode.IsSpace)
	i := 0
	if len(raw) > 0 && (raw[0] == 'h' || raw[0] == 'H') {
		i++
	}
	verbatim := i < len(raw) && raw[i] == '@'
	if verbatim {
		i++
	}
	if i >= len(raw) || raw[i] != '\'' && raw[i] != '"' {
		return "", "", false
	}
	quote := raw[i]
	for i++; i < len(raw); i++ {
		ch := raw[i]
		if ch == quote {
			if verbatim && i+1 < len(raw) && raw[i+1] == quote {
				i++
				continue
			}
			literal := raw[:i+1]
			budget.reserveWork(len(literal)) // reserve decoder growth before allocation
			return decodeKQLString(literal), raw[i+1:], true
		}
		if !verbatim && (ch == '\r' || ch == '\n') {
			return "", "", false
		}
		if ch != '\\' || verbatim {
			continue
		}
		i++
		if i >= len(raw) {
			return "", "", false
		}
		digits := 0
		switch raw[i] {
		case 'b', 't', 'n', 'f', 'r', '"', '\'', '\\':
		case 'x':
			digits = 2
		case 'u':
			digits = 4
		case 'U':
			end, valid := unicodeScalarEscape(raw, i, budget)
			if !valid {
				return "", "", false
			}
			i = end - 1
		default:
			return "", "", false
		}
		if digits > len(raw)-i-1 {
			return "", "", false
		}
		for j := 0; j < digits; j++ {
			i++
			ch = raw[i]
			if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
				return "", "", false
			}
		}
	}
	return "", "", false
}

func quotedIdentityPrefix(text string) bool {
	text = strings.TrimSpace(text)
	text = trimObfuscationPrefix(text)
	text = strings.TrimPrefix(text, "@")
	return len(text) > 0 && (text[0] == '\'' || text[0] == '"')
}
