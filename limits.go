package kql

import (
	"context"
	"errors"
	"fmt"

	"github.com/antlr4-go/antlr/v4"
)

// Limits are copied into each call and shared by its recursive subqueries.
// They bound accepted input and cooperative work, not ANTLR's peak memory or
// wall-clock time between token-stream callbacks.
type parseLimits struct {
	maxInputBytes   int
	maxNestingDepth int
	maxTokens       int
	maxWork         int
	maxOutputItems  int
}

func defaultParseLimits() parseLimits {
	return parseLimits{128 << 10, 16, 4096, 1000000, 4096}
}

type parseBudget struct {
	ctx                  context.Context
	limits               parseLimits
	work, tokens, output int
}

type budgetAbort struct{ err error }

func newParseBudget(ctx context.Context, limits parseLimits) (*parseBudget, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, MaxParseTime)
	return &parseBudget{ctx: ctx, limits: limits}, cancel
}

func parseFailure(value any) error {
	if abort, ok := value.(budgetAbort); ok {
		return abort.err
	}
	return fmt.Errorf("parser panic: %v", value)
}

func (b *parseBudget) reject(reason string) {
	panic(budgetAbort{fmt.Errorf("parser limit exceeded: %s", reason)})
}

func (b *parseBudget) check() {
	if b == nil {
		return
	}
	if err := b.ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("parser timeout: %w", err)
		}
		panic(budgetAbort{err})
	}
}

func (b *parseBudget) step() {
	if b == nil {
		return
	}
	b.check()
	if b.work >= b.limits.maxWork {
		b.reject("work budget")
	}
	b.work++
}

func (b *parseBudget) input(query string) {
	if b == nil {
		return
	}
	b.check()
	if len(query) > b.limits.maxInputBytes {
		b.reject("input bytes")
	}
}

func (b *parseBudget) reserveOutput(items int) {
	if b == nil {
		return
	}
	b.check()
	if items > b.limits.maxOutputItems-b.output {
		b.reject("output items")
	}
	b.output += items
}

func (b *parseBudget) outputSize(items int) {
	if b != nil && items > b.limits.maxOutputItems {
		b.reject("output items")
	}
	b.check()
}

// Stop before token-buffer growth. Complete preflight precedes parser.Query,
// so excessive nesting never reaches adaptive prediction. The real lexer
// keeps literal, identifier, regex and comment delimiters out of this count.
type budgetLexer struct {
	antlr.Lexer
	budget     *parseBudget
	charge     bool
	tokens     int
	delimiters []int
}

func (l *budgetLexer) NextToken() antlr.Token {
	l.budget.step()
	token := l.Lexer.NextToken()
	kind := token.GetTokenType()
	if kind == antlr.TokenEOF {
		return token
	}
	if l.tokens >= l.budget.limits.maxTokens {
		l.budget.reject("token count")
	}
	l.tokens++
	if l.charge {
		if l.budget.tokens >= l.budget.limits.maxTokens {
			l.budget.reject("token count")
		}
		l.budget.tokens++
	}
	switch kind {
	case KQLLexerLPAREN, KQLLexerLBRACKET, KQLLexerLBRACE:
		if len(l.delimiters) >= l.budget.limits.maxNestingDepth {
			l.budget.reject("nesting depth")
		}
		l.delimiters = append(l.delimiters, kind)
	case KQLLexerRPAREN:
		if len(l.delimiters) > 0 && l.delimiters[len(l.delimiters)-1] == KQLLexerLPAREN {
			l.delimiters = l.delimiters[:len(l.delimiters)-1]
		}
	case KQLLexerRBRACKET:
		if len(l.delimiters) > 0 && l.delimiters[len(l.delimiters)-1] == KQLLexerLBRACKET {
			l.delimiters = l.delimiters[:len(l.delimiters)-1]
		}
	case KQLLexerRBRACE:
		if len(l.delimiters) > 0 && l.delimiters[len(l.delimiters)-1] == KQLLexerLBRACE {
			l.delimiters = l.delimiters[:len(l.delimiters)-1]
		}
	}
	return token
}

type budgetCharStream struct {
	antlr.CharStream
	budget *parseBudget
}

func (s *budgetCharStream) LA(i int) int { s.budget.step(); return s.CharStream.LA(i) }
func (s *budgetCharStream) Consume()     { s.budget.step(); s.CharStream.Consume() }

type budgetTokenStream struct {
	*antlr.CommonTokenStream
	budget *parseBudget
}

func (s *budgetTokenStream) LA(i int) int         { s.budget.step(); return s.CommonTokenStream.LA(i) }
func (s *budgetTokenStream) LT(i int) antlr.Token { s.budget.step(); return s.CommonTokenStream.LT(i) }
func (s *budgetTokenStream) Consume()             { s.budget.step(); s.CommonTokenStream.Consume() }
func (s *budgetTokenStream) Get(i int) antlr.Token {
	s.budget.step()
	return s.CommonTokenStream.Get(i)
}

func preflight(query string, b *parseBudget, charge bool, listener *errorListener) *antlr.CommonTokenStream {
	b.input(query)
	input := &budgetCharStream{CharStream: antlr.NewInputStream(query), budget: b}
	lexer := NewKQLLexer(input)
	lexer.RemoveErrorListeners()
	if listener != nil {
		lexer.AddErrorListener(listener)
	}
	limited := &budgetLexer{Lexer: lexer, budget: b, charge: charge}
	stream := antlr.NewCommonTokenStream(limited, antlr.TokenDefaultChannel)
	stream.Fill()
	stream.Seek(0)
	b.check()
	return stream
}

func (e *conditionExtractor) appendCondition(condition Condition) {
	e.budget.reserveOutput(1 + len(condition.Alternatives))
	e.conditions = append(e.conditions, condition)
}

func (e *conditionExtractor) checkOutput() {
	e.budget.outputSize(len(e.conditions))
}

func (e *conditionExtractor) EnterEveryRule(_ antlr.ParserRuleContext) {
	e.budget.step()
	e.checkOutput()
}
