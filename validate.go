package kql

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/antlr4-go/antlr/v4"
)

// Validate reports the syntax errors Kusto would raise for a query. Unlike
// ExtractConditions, which reads what it can from messy text, it rejects text
// that is not a single query: a write-up around the query, a second query after
// the first, a query that starts with an operator, or another language. The
// bodies of let statements are not validated.
func Validate(query string) []string {
	ch := make(chan []string, 1)
	go func() {
		ch <- validate(query)
	}()

	select {
	case errs := <-ch:
		return errs
	case <-time.After(MaxParseTime):
		return []string{fmt.Sprintf("parser timeout: query took longer than %s to parse", MaxParseTime)}
	}
}

func validate(query string) (errs []string) {
	defer func() {
		if r := recover(); r != nil {
			errs = []string{fmt.Sprintf("parser panic: %v", r)}
		}
	}()

	if strings.TrimSpace(query) == "" {
		return []string{"query is empty"}
	}

	lexer := NewKQLLexer(antlr.NewInputStream(normalize(query, true)))
	listener := &errorListener{}
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(listener)

	parser := NewKQLParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	parser.RemoveErrorListeners()
	parser.AddErrorListener(listener)
	parser.Query()

	for i, msg := range listener.errors {
		listener.errors[i] = shortenSyntaxError(msg)
	}
	return listener.errors
}

var expectedTokensRe = regexp.MustCompile(` expecting \{[^}]*\}$`)

// shortenSyntaxError drops a long list of expected tokens: where any operator
// or expression may follow, the parser names every token that can start one.
func shortenSyntaxError(msg string) string {
	if loc := expectedTokensRe.FindStringIndex(msg); loc != nil && strings.Count(msg[loc[0]:], ",") > 5 {
		return msg[:loc[0]]
	}
	return msg
}
