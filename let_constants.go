package kql

import (
	"strings"
	"unicode/utf8"

	"github.com/antlr4-go/antlr/v4"
)

// letConstant is the literal a let statement binds, as written in the query:
// a scalar, or the elements of a dynamic([...]) array.
type letConstant struct {
	items []string
	list  bool
}

// inlineLetConstants replaces references to lets that bind a constant with
// the literal they bind, so a condition reads a let's value as it reads an
// inline literal: "x == name" becomes "x == <literal>", and "x in (name)" or
// "x has_all (name)" lists the array's elements. Only operands of comparison
// and list operators are replaced; lets that bind anything else, such as a
// tabular expression, stay names.
func inlineLetConstants(query string, lets []LetStatement, budget *parseBudget) string {
	budget.step()
	// Only substitute in a later top-level statement, after proving that
	// bindings are unique and no function scope or column assignment shadows
	// them. Ambiguous queries keep their bindings and scalar predicates.
	if len(lets) == 0 {
		return query
	}
	names := make(map[string]bool, len(lets))
	candidates := make(map[string]letConstant, len(lets))
	for _, binding := range lets {
		budget.step()
		if names[binding.Name] {
			return query
		}
		names[binding.Name] = true
		if constant, ok := parseLetConstant(binding.Expression); ok {
			candidates[binding.Name] = constant
		}
	}
	if len(candidates) == 0 {
		return query
	}
	input := &budgetCharStream{CharStream: antlr.NewInputStream(query), budget: budget}
	var lexer antlr.Lexer = NewKQLLexer(input)
	lexer.RemoveErrorListeners()
	if budget != nil {
		lexer = &budgetLexer{Lexer: lexer, budget: budget}
	}
	bytePosition, runePosition := 0, 0
	var previous, beforePrevious antlr.Token
	for token := lexer.NextToken(); token.GetTokenType() != antlr.TokenEOF; token = lexer.NextToken() {
		budget.step()
		if token.GetChannel() != antlr.TokenDefaultChannel {
			continue
		}
		if token.GetTokenType() == KQLLexerLBRACE {
			return query
		}
		if token.GetTokenType() == KQLLexerASSIGN && previous != nil && names[decodeKQLIdentifier(previous.GetText(), budget)] && (beforePrevious == nil || beforePrevious.GetTokenType() != KQLLexerLET) {
			return query
		}
		if constant, known := candidates[decodeKQLIdentifier(token.GetText(), budget)]; known && (previous == nil || previous.GetTokenType() != KQLLexerLET) {
			// An occurrence outside a supported RHS operand can introduce a
			// column (range, mv-expand, summarize BY, project, ...), or occur
			// in another scope. Without schema resolution its binding is unknown.
			for runePosition < token.GetStart() {
				budget.step()
				_, size := utf8.DecodeRuneInString(query[bytePosition:])
				bytePosition += size
				runePosition++
			}
			start := bytePosition
			end := start + len(token.GetText())
			if _, safe := letConstantOperand(query, start, end, constant); !safe {
				return query
			}
		}
		beforePrevious, previous = previous, token
	}
	constants := make(map[string]letConstant, len(lets))
	statements := splitStatements(query)
	normalizedBytes := len(statements) - 1
	for _, statement := range statements {
		normalizedBytes += len(statement)
	}
	for index, statement := range statements {
		budget.step()
		name, expression, isLet := parseLetAssignment(strings.TrimSpace(trimLeadingLineComments(statement)))
		if isLet {
			if constant, ok := parseLetConstant(expression); ok {
				constants[name] = constant
			}
			continue
		}
		// Account for every other statement before growing this one, so
		// splitting the query never resets the normalized-input byte cap.
		otherBytes := normalizedBytes - len(statement)
		statements[index] = inlineLetConstantOperands(statement, constants, budget, otherBytes)
		normalizedBytes = otherBytes + len(statements[index])
	}
	result := strings.Join(statements, ";")
	budget.input(result)
	return result
}

func inlineLetConstantOperands(query string, constants map[string]letConstant, budget *parseBudget, otherBytes int) string {
	if len(constants) == 0 {
		return query
	}

	var b strings.Builder
	write := func(value string) {
		budget.step()
		if budget != nil && len(value) > budget.limits.maxInputBytes-otherBytes-b.Len() {
			budget.reject("normalized input bytes")
		}
		b.WriteString(value)
	}
	last := 0
	for i := 0; i < len(query); {
		budget.step()
		ch := query[i]
		switch {
		case ch == '/' && i+1 < len(query) && query[i+1] == '/':
			for i < len(query) && query[i] != '\n' {
				i++
			}
		case isStringLiteralStart(query, i):
			i = stringLiteralEnd(query, i)
			if i < 0 {
				i = len(query)
			}
		case isIdentStart(ch) && (i == 0 || !isIdentChar(query[i-1]) && query[i-1] != '.'):
			end := i
			for end < len(query) && isIdentChar(query[end]) {
				end++
			}
			if constant, ok := constants[query[i:end]]; ok {
				if literal, ok := letConstantOperand(query, i, end, constant); ok {
					write(query[last:i])
					write(literal)
					last = end
				}
			}
			i = end
		default:
			i++
		}
	}
	if last == 0 {
		return query
	}
	write(query[last:])
	return b.String()
}

var (
	letListOperators = map[string]bool{
		"in": true, "!in": true, "in~": true, "!in~": true, "has_any": true, "has_all": true,
	}
	letComparisonOperators = map[string]bool{
		"==": true, "!=": true, "=~": true, "!~": true, "<": true, "<=": true, ">": true, ">=": true,
		"has": true, "!has": true, "has_cs": true, "!has_cs": true,
		"contains": true, "!contains": true, "contains_cs": true, "!contains_cs": true,
		"startswith": true, "!startswith": true, "startswith_cs": true, "!startswith_cs": true,
		"endswith": true, "!endswith": true, "endswith_cs": true, "!endswith_cs": true,
		"hasprefix": true, "!hasprefix": true, "hasprefix_cs": true, "!hasprefix_cs": true,
		"hassuffix": true, "!hassuffix": true, "hassuffix_cs": true, "!hassuffix_cs": true,
	}
)

// letConstantOperand returns the literal that replaces the let name at
// query[start:end]: the array's elements, or the scalar, when the name is the
// whole list of a list operator; the scalar when the name follows a
// comparison operator.
func letConstantOperand(query string, start, end int, constant letConstant) (string, bool) {
	before := strings.TrimRight(query[:start], " \t\r\n")
	after := strings.TrimLeft(query[end:], " \t\r\n")
	if strings.HasSuffix(before, "(") && strings.HasPrefix(after, ")") {
		operator, _ := trailingOperator(before[:len(before)-1])
		if !letListOperators[operator] {
			return "", false
		}
		return strings.Join(constant.items, ", "), true
	}
	if constant.list {
		return "", false
	}
	operator, rest := trailingOperator(before)
	if operator == "regex" {
		operator, _ = trailingOperator(rest)
		if operator != "matches" {
			return "", false
		}
	} else if !letComparisonOperators[operator] {
		return "", false
	}
	return constant.items[0], true
}

// trailingOperator splits the operator that ends s, lower-cased, from the
// text before it: a word such as has or !in~, or a symbol such as >=.
func trailingOperator(s string) (string, string) {
	s = strings.TrimRight(s, " \t\r\n")
	end := len(s)
	i := end
	if i > 0 && s[i-1] == '~' && i > 1 && isIdentChar(s[i-2]) {
		i-- // in~, !in~
	}
	if i > 0 && isIdentChar(s[i-1]) {
		for i > 0 && isIdentChar(s[i-1]) {
			i--
		}
		if i > 0 && s[i-1] == '!' {
			i--
		}
	} else {
		for i > 0 && strings.IndexByte("=<>!~", s[i-1]) >= 0 {
			i--
		}
	}
	return strings.ToLower(s[i:end]), s[:i]
}

// parseLetConstant reads the literal a let binds: a string, number or
// boolean, or dynamic([...]) of those.
func parseLetConstant(expression string) (letConstant, bool) {
	expression = strings.TrimSpace(expression)
	if isScalarLiteral(expression) {
		return letConstant{items: []string{expression}}, true
	}
	if !strings.HasPrefix(strings.ToLower(expression), "dynamic") {
		return letConstant{}, false
	}
	body := strings.TrimSpace(expression[len("dynamic"):])
	if !strings.HasPrefix(body, "(") || !strings.HasSuffix(body, ")") {
		return letConstant{}, false
	}
	body = strings.TrimSpace(body[1 : len(body)-1])
	if !strings.HasPrefix(body, "[") || !strings.HasSuffix(body, "]") {
		return letConstant{}, false
	}
	var items []string
	for _, item := range splitListItems(body[1 : len(body)-1]) {
		if !isScalarLiteral(item) {
			return letConstant{}, false
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return letConstant{}, false
	}
	return letConstant{items: items, list: true}, true
}

// isScalarLiteral reports whether s is exactly one string, number or boolean
// literal.
func isScalarLiteral(s string) bool {
	if isStringLiteralStart(s, 0) {
		return stringLiteralEnd(s, 0) == len(s)
	}
	if numericToken.MatchString(strings.TrimLeft(s, "+-")) {
		return true
	}
	switch strings.ToLower(s) {
	case "true", "false":
		return true
	}
	return false
}

// splitListItems splits the comma-separated elements of an array literal,
// ignoring commas inside strings.
func splitListItems(s string) []string {
	var items []string
	start := 0
	for i := 0; i < len(s); {
		switch {
		case isStringLiteralStart(s, i):
			i = stringLiteralEnd(s, i)
			if i < 0 {
				return append(items, strings.TrimSpace(s[start:]))
			}
		case s[i] == ',':
			items = append(items, strings.TrimSpace(s[start:i]))
			i++
			start = i
		default:
			i++
		}
	}
	if rest := strings.TrimSpace(s[start:]); rest != "" || len(items) > 0 {
		items = append(items, rest)
	}
	return items
}

// isStringLiteralStart reports whether a string literal, regular or verbatim
// (@"..."), starts at s[i].
func isStringLiteralStart(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	if s[i] == '"' || s[i] == '\'' {
		return true
	}
	return s[i] == '@' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\'')
}

// stringLiteralEnd returns the index just past the string literal starting
// at s[start], or -1 when it is not closed. Regular strings escape with a
// backslash; verbatim strings double the quote.
func stringLiteralEnd(s string, start int) int {
	verbatim := s[start] == '@'
	i := start
	if verbatim {
		i++
	}
	quote := s[i]
	for i++; i < len(s); i++ {
		switch {
		case verbatim && s[i] == quote && i+1 < len(s) && s[i+1] == quote:
			i++
		case !verbatim && s[i] == '\\':
			i++
		case s[i] == quote:
			return i + 1
		}
	}
	return -1
}
