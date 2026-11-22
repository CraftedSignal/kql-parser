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
	maxInputBytes    int
	maxNestingDepth  int
	maxTokens        int
	maxWork          int
	maxOutputItems   int
	maxMetadataItems int
	maxMetadataBytes int
}

func defaultParseLimits() parseLimits {
	return parseLimits{128 << 10, 16, 4096, 1000000, 4096, 4096, 1 << 20}
}

type parseBudget struct {
	ctx                          context.Context
	limits                       parseLimits
	work, tokens, output         int
	metadataItems, metadataBytes int
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

func (b *parseBudget) step() { b.reserveWork(1) }

// Reserve cooperative work before the operation. Temporary backing owners charge
// the full new capacity, including replacements, without refunds. This bounds
// cumulative guarded slot allocation and copying using the existing shared work
// allowance; retained output and metadata remain charged by their own owners.
func (b *parseBudget) reserveWork(units int) {
	if b == nil {
		return
	}
	b.check()
	if units < 0 || units > b.limits.maxWork-b.work {
		b.reject("work budget")
	}
	b.work += units
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

// Reserve the entire new backing array before allocation, including spare
// capacity. Fall back to exact capacity when geometric growth would not fit.
func appendTemporary[T any](values []T, budget *parseBudget, additions ...T) []T {
	if len(additions) > cap(values)-len(values) {
		maxInt := int(^uint(0) >> 1)
		if len(additions) > maxInt-len(values) {
			budget.reject("work budget")
		}
		required := len(values) + len(additions)
		capacity := required
		if cap(values) <= maxInt/2 {
			capacity = max(required, 2*cap(values))
		}
		if budget != nil && capacity > budget.limits.maxWork-budget.work {
			capacity = required
		}
		budget.reserveWork(capacity)
		grown := make([]T, len(values), capacity)
		copy(grown, values)
		values = grown
	}
	return append(values, additions...)
}

// Additive provenance has one request-shared allowance, independent of the
// legacy operand/condition output units. Both dimensions are reserved before
// growth, including every owned recursive and semantic copy.
func (b *parseBudget) reserveMetadata(items, textBytes int) {
	if b == nil {
		return
	}
	b.check()
	if items < 0 || items > b.limits.maxMetadataItems-b.metadataItems {
		b.reject("metadata items")
	}
	if textBytes < 0 || textBytes > b.limits.maxMetadataBytes-b.metadataBytes {
		b.reject("metadata bytes")
	}
	b.metadataItems += items
	b.metadataBytes += textBytes
}

// Sum string lengths using bounded subtraction so even a collection of large
// strings cannot wrap before the atomic two-dimensional reservation.
func (b *parseBudget) reserveMetadataStrings(items int, values ...string) {
	if b == nil {
		return
	}
	b.check()
	textBytes := 0
	for _, value := range values {
		if len(value) > b.limits.maxMetadataBytes-textBytes {
			b.reject("metadata bytes")
		}
		textBytes += len(value)
	}
	b.reserveMetadata(items, textBytes)
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
	source                        string
	complete, hasQuotedIdentifier bool
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
		l.complete = true
		return token
	}
	if l.tokens >= l.budget.limits.maxTokens {
		l.budget.reject("token count")
	}
	if kind == KQLLexerQUOTED_IDENTIFIER {
		l.hasQuotedIdentifier = true
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
	limited := &budgetLexer{Lexer: lexer, budget: b, charge: charge, source: query}
	stream := antlr.NewCommonTokenStream(limited, antlr.TokenDefaultChannel)
	stream.Fill()
	stream.Seek(0)
	b.check()
	return stream
}

func (e *conditionExtractor) appendCondition(condition Condition) {
	e.budget.reserveOutput(1 + len(condition.Alternatives) + operandOutputCost(condition.RHS, e.budget))
	e.conditions = append(e.conditions, condition)
}

func (e *conditionExtractor) checkOutput() {
	e.budget.outputSize(len(e.conditions))
}

func (e *conditionExtractor) EnterEveryRule(_ antlr.ParserRuleContext) {
	e.budget.step()
	e.checkOutput()
}

// Internal syntax helpers accept an optional budget so direct parser-tree
// fixtures remain usable. Production calls always carry the request budget.
func suppliedBudget(budgets []*parseBudget) *parseBudget {
	if len(budgets) == 0 {
		return nil
	}
	return budgets[0]
}
