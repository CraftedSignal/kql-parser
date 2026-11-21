package kql

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

// These helpers use real budget failures, rather than a product allocation hook.
func reservationAbort(run func()) (reason string) {
	defer func() {
		if value := recover(); value != nil {
			reason = parseFailure(value).Error()
		}
	}()
	run()
	return ""
}

func TestCloneReservationsCountEachOwnedNodeOnce(t *testing.T) {
	source := &Operand{Kind: OperandList, Items: []Operand{
		{Kind: OperandString, Source: `"a"`, Value: "a"},
		{Kind: OperandString, Source: `"b"`, Value: "b"},
	}}
	for _, tc := range []struct {
		name                 string
		output, text, copies int
		want                 string
	}{
		{"exact", 3, 8, 1, ""}, {"one output short", 2, 8, 1, "output items"},
		{"one byte short", 3, 7, 1, "metadata bytes"}, {"two owned copies", 6, 16, 2, ""},
		{"second output short", 5, 16, 2, "output items"}, {"second text short", 6, 15, 2, "metadata bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := defaultParseLimits()
			limits.maxOutputItems = tc.output
			limits.maxMetadataBytes = tc.text
			budget := &parseBudget{ctx: context.Background(), limits: limits}
			var copied *Operand
			got := reservationAbort(func() {
				for range tc.copies {
					copied = cloneOperand(source, budget)
				}
			})
			if tc.want != "" {
				if !strings.Contains(got, tc.want) {
					t.Fatalf("want %s, got %q", tc.want, got)
				}
				return
			}
			if got != "" || budget.output != 3*tc.copies || budget.metadataBytes != 8*tc.copies || !reflect.DeepEqual(copied, source) {
				t.Fatalf("copy/cost changed: error=%q budget=%+v copy=%+v", got, budget, copied)
			}
			copied.Items[0].Value = "changed"
			if source.Items[0].Value != "a" {
				t.Fatal("operand child slice aliases source")
			}
		})
	}
}

func TestCloneReservationPrecedesBackingAllocation(t *testing.T) {
	// Thirty-two empty records are a small allocation-order fixture. A rejected
	// clone must not allocate even half of its otherwise-unused backing array.
	operands := &Operand{Kind: OperandList, Items: make([]Operand, 32)}
	conditions := make([]Condition, 32)
	for _, tc := range []struct {
		name  string
		bytes int64
		run   func(*parseBudget)
	}{
		{"operand children", int64(32 * unsafe.Sizeof(Operand{})), func(b *parseBudget) { cloneOperand(operands, b) }},
		{"semantic conditions", int64(32 * unsafe.Sizeof(Condition{})), func(b *parseBudget) { cloneSemanticConditions(conditions, b) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := defaultParseLimits()
			limits.maxOutputItems = 1
			var reason string
			measured := testing.Benchmark(func(b *testing.B) {
				for range b.N {
					budget := &parseBudget{ctx: context.Background(), limits: limits}
					reason = reservationAbort(func() { tc.run(budget) })
				}
			})
			if !strings.Contains(reason, "output items") {
				t.Fatalf("wrong failure: %q", reason)
			}
			if got := measured.AllocedBytesPerOp(); got >= tc.bytes/2 {
				t.Fatalf("rejected clone allocated %d bytes/op before reservation; backing array alone is %d bytes", got, tc.bytes)
			}
		})
	}
}

func TestDynamicArrayTemporaryReservation(t *testing.T) {
	operand := &Operand{Kind: OperandTyped, Type: "dynamic", Source: "dynamic([1,2,3])"}
	for _, limit := range []int{2, 3} {
		limits := defaultParseLimits()
		limits.maxWork = 13
		if limit == 2 {
			limits.maxWork = 7
		}
		budget := &parseBudget{ctx: context.Background(), limits: limits}
		var items []Operand
		reason := reservationAbort(func() { items, _ = dynamicArrayItems(operand, budget) })
		if limit == 2 {
			if reason != "parser limit exceeded: work budget" {
				t.Fatalf("tiny dynamic list grew without temporary reservation: items=%+v error=%q", items, reason)
			}
			continue
		}
		want := []Operand{{Kind: OperandNumber, Source: "1", Value: "1"}, {Kind: OperandNumber, Source: "2", Value: "2"}, {Kind: OperandNumber, Source: "3", Value: "3"}}
		if reason != "" || !reflect.DeepEqual(items, want) || budget.output != 0 {
			t.Fatalf("temporary parsing changed values or charged retained output: items=%+v output=%d error=%q", items, budget.output, reason)
		}
	}
}

func TestDynamicArrayPublicWorkFailureIsAtomic(t *testing.T) {
	limits := defaultParseLimits()
	limits.maxWork = 100
	result := extractConditionsContext(context.Background(), "T | where x in (dynamic([1,2,3]))", limits)
	if !reflect.DeepEqual(result.Errors, []string{"parser limit exceeded: work budget"}) || result.Expression != nil || result.Dataflow != nil || len(result.Conditions) != 0 {
		t.Fatalf("work rejection exposed partial output: %+v", result)
	}
}

func TestTemporaryOwnersAreSharedWithoutRetainedDoubleDebit(t *testing.T) {
	operand := &Operand{Kind: OperandTyped, Type: "dynamic", Source: "dynamic([1,2,3])"}
	// Each owner visits five source bytes twice and reserves three slots: 13 units.
	for _, limit := range []int{20, 26} {
		limits := defaultParseLimits()
		limits.maxWork = limit
		budget := &parseBudget{ctx: context.Background(), limits: limits}
		first, dynamic := dynamicArrayItems(operand, budget)
		if !dynamic || len(first) != 3 || budget.work != 13 {
			t.Fatalf("first owner changed: %+v %+v", first, budget)
		}
		reason := reservationAbort(func() { dynamicArrayItems(operand, budget) })
		if limit == 20 {
			if reason != "parser limit exceeded: work budget" || budget.work != 18 {
				t.Fatalf("second owner reset allowance: reason=%q budget=%+v", reason, budget)
			}
		} else if reason != "" || budget.work != 26 {
			t.Fatalf("two owners require26 work: reason=%q budget=%+v", reason, budget)
		}
		if budget.output != 0 || budget.metadataItems != 0 || budget.metadataBytes != 0 {
			t.Fatal("temporary arrays double-debited retained ownership")
		}
	}
	// Initial flat backing1 + dynamic13 + replacement backing3 =17 work units.
	limits := defaultParseLimits()
	limits.maxWork = 17
	budget := &parseBudget{ctx: context.Background(), limits: limits}
	flat := flattenDynamicArrays([]Operand{*operand}, budget)
	if len(flat) != 3 || budget.work != 17 || budget.output != 0 {
		t.Fatalf("flattened ownership changed: %+v %+v", flat, budget)
	}
	limits.maxWork = 16
	short := &parseBudget{ctx: context.Background(), limits: limits}
	metadataBudgetFailure(t, "work budget", func() { flattenDynamicArrays([]Operand{*operand}, short) })
	if constant, ok := parseLetConstant("dynamic([1,2,3])"); !ok || len(constant.items) != 3 {
		t.Fatal("optional-budget fixture lost support")
	}
	limits.maxWork = 7
	limited := &parseBudget{ctx: context.Background(), limits: limits}
	metadataBudgetFailure(t, "work budget", func() { parseLetConstant("dynamic([1,2,3])", limited) })
}

func TestRepeatedLetListRetainsExactTextOwnership(t *testing.T) {
	constant, ok := parseLetConstant(`dynamic(["a","b"])`)
	if !ok {
		t.Fatal("invalid fixture")
	}
	for _, limit := range []int{155, 156} {
		limits := defaultParseLimits()
		limits.maxMetadataBytes = limit
		budget := &parseBudget{ctx: context.Background(), limits: limits}
		query := "x in (h) or y in (h)"
		protection := newTypedProtection(query, budget)
		expanded := ""
		reason := reservationAbort(func() {
			expanded = inlineLetConstantOperands(query, map[string]letConstant{"h": constant}, budget, 0, protection, nil, 0)
		})
		if limit == 155 {
			if reason != "parser limit exceeded: metadata bytes" {
				t.Fatalf("shared let text did not reject: %q", reason)
			}
			continue
		}
		if reason != "" || budget.metadataBytes != 156 || budget.output != 4 || budget.work == 0 || len(protection.markers) != 4 {
			t.Fatalf("let ownership changed: budget=%+v markers=%+v error=%q", budget, protection.markers, reason)
		}
		if strings.Count(expanded, `"a"`) != 2 || strings.Count(expanded, `"b"`) != 2 {
			t.Fatalf("let expansions changed: %s", expanded)
		}
		for _, origin := range protection.markers {
			if origin.binding != "h" || origin.listSource != constant.source || origin.source != `"a"` && origin.source != `"b"` {
				t.Fatalf("lost list binding/source: %+v", origin)
			}
		}
	}
}

func TestIOCReservationPreservesOrderedValuesAndRoles(t *testing.T) {
	for _, query := range []string{"DeviceFileEvents | where SHA256 in (" + iocTestHashes(20) + ")", "let h = dynamic([" + iocTestHashes(20) + "]); DeviceFileEvents | where SHA256 in (h)"} {
		got := ExtractConditions(query)
		if len(got.Errors) != 0 || len(got.Conditions) != 1 || got.Expression == nil || got.Dataflow == nil || !got.Dataflow.Complete {
			t.Fatalf("ordinary IOC support lost: %+v", got)
		}
		c := got.Conditions[0]
		if c.RHS == nil || c.RHS.Kind != OperandList || len(c.RHS.Items) != 20 || len(c.Alternatives) != 20 {
			t.Fatalf("IOC list roles lost: %+v", c)
		}
		for i, item := range c.RHS.Items {
			// The independent authored values are the integers1..20 rendered as64 hex digits.
			want := fmt.Sprintf("%064x", i+1)
			if c.Alternatives[i] != want || item.Kind != OperandString || item.Source != `"`+want+`"` || item.Value != want {
				t.Fatalf("IOC item%d changed: %+v alternative=%q", i, item, c.Alternatives[i])
			}
			if strings.HasPrefix(query, "let ") && item.Binding != "h" {
				t.Fatalf("let role lost: %+v", item)
			}
		}
	}
	query := `let h=dynamic(["one","two","three"]); T | where x has_any (h)`
	got := ExtractConditions(query)
	if len(got.Errors) != 0 || len(got.Conditions) != 1 {
		t.Fatalf("short has_any lost: %+v", got)
	}
	c := got.Conditions[0]
	if !reflect.DeepEqual(c.Alternatives, []string{"one", "two", "three"}) || c.RHS == nil || c.RHS.Kind != OperandList || len(c.RHS.Items) != 3 {
		t.Fatalf("has_any list role lost: %+v", c)
	}
	for i, want := range []string{"one", "two", "three"} {
		item := c.RHS.Items[i]
		if item.Kind != OperandString || item.Value != want || item.Source != `"`+want+`"` || item.Binding != "h" {
			t.Fatalf("has_any item ordering/role lost: %+v", item)
		}
	}
}

func TestReservationTokenBoundaryIsIndependent(t *testing.T) {
	for _, count := range []int{4096, 4097} {
		budget := &parseBudget{ctx: context.Background(), limits: defaultParseLimits()}
		reason := reservationAbort(func() { preflight(strings.Repeat("x ", count), budget, true, nil) })
		if count == 4096 {
			if reason != "" || budget.tokens != 4096 {
				t.Fatalf("exact token cap changed: tokens=%d error=%q", budget.tokens, reason)
			}
		} else if reason != "parser limit exceeded: token count" {
			t.Fatalf("token overflow changed: %q", reason)
		}
	}
}

func TestInheritedLetTemporaryOwnersShareRecursiveBudget(t *testing.T) {
	child := `let n=dynamic([42,"42"]); U | where x in (n)`
	parent := `let n=dynamic([42,"42"]); T | where y in (n) | join (U | where x in (n)) on Key`
	parsed := ExtractConditions(parent)
	if len(parsed.Errors) != 0 || len(parsed.Joins) != 1 || parsed.Joins[0].Subsearch == nil {
		t.Fatalf("inherited let join lost: %+v", parsed)
	}
	rhs := parsed.Joins[0].Subsearch.Conditions[0].RHS
	if rhs == nil || len(rhs.Items) != 2 || rhs.Items[0].Binding != "n" || rhs.Items[0].Kind != OperandNumber || rhs.Items[1].Binding != "n" || rhs.Items[1].Kind != OperandString {
		t.Fatalf("inherited child let roles lost: %+v", rhs)
	}
	// The inherited join remains a supported public positive above. A small
	// shared work allowance admits a standalone child but rejects parent+child;
	// neither invocation receives a reset budget for its recursive extraction.
	limits := defaultParseLimits()
	limits.maxWork = 6000
	childBudget := &parseBudget{ctx: context.Background(), limits: limits}
	childResult := extractConditionsInternal(child, childBudget)
	if len(childResult.Errors) != 0 || len(childResult.Conditions) != 1 {
		t.Fatalf("child-only control must fit: %+v work=%d", childResult, childBudget.work)
	}
	shared := &parseBudget{ctx: context.Background(), limits: limits}
	var got *ParseResult
	reason := reservationAbort(func() { got = extractConditionsInternal(parent, shared) })
	if reason != "parser limit exceeded: work budget" || got != nil {
		t.Fatalf("recursive allowance reset or partial output escaped: reason=%q result=%+v work=%d", reason, got, shared.work)
	}
}

func TestReservationPreservesLargeSupportedDynamicLists(t *testing.T) {
	for _, tc := range []struct {
		count int
		let   bool
	}{{600, false}, {1000, false}, {450, true}} {
		t.Run(fmt.Sprintf("count%d/let%t", tc.count, tc.let), func(t *testing.T) {
			values := make([]string, tc.count)
			for i := range values {
				values[i] = fmt.Sprintf(`"v%04d"`, i)
			}
			literal := "dynamic([" + strings.Join(values, ",") + "])"
			query := "T | where F in (" + literal + ")"
			if tc.let {
				query = "let h=" + literal + "; T | where F in (h)"
			}
			got := ExtractConditions(query)
			if len(got.Errors) != 0 || got.Expression == nil || got.Dataflow == nil || !got.Dataflow.Complete || len(got.Conditions) != 1 {
				t.Fatalf("previously supported dynamic list lost: errors=%v expression=%+v conditions=%d", got.Errors, got.Expression, len(got.Conditions))
			}
			c := got.Conditions[0]
			if c.RHS == nil || c.RHS.Kind != OperandList || len(c.RHS.Items) != tc.count || len(c.Alternatives) != tc.count {
				t.Fatalf("list shape lost: %+v", c)
			}
			for i, item := range c.RHS.Items {
				want := fmt.Sprintf("v%04d", i)
				if item.Kind != OperandString || item.Source != values[i] || item.Value != want || c.Alternatives[i] != want || tc.let && item.Binding != "h" {
					t.Fatalf("ordered item%d changed: %+v", i, item)
				}
			}
		})
	}
}

func TestDynamicBooleanFallbackRemainsOpaque(t *testing.T) {
	for _, literal := range []string{"TRUE", "FALSE", "true", "false"} {
		t.Run(literal, func(t *testing.T) {
			got := ExtractConditions("T | where F in (dynamic([" + literal + "]))")
			if len(got.Errors) != 0 || got.Expression == nil || got.Dataflow == nil || !got.Dataflow.Complete {
				t.Fatalf("boolean parse changed: %+v", got)
			}
			if literal == strings.ToUpper(literal) {
				if len(got.Conditions) != 0 || got.Expression.Operator != BooleanScalar || got.Expression.Scalar == nil || got.Expression.Scalar.Text != "F in (dynamic(["+literal+"]))" {
					t.Fatalf("uppercase literal must retain opaque scalar: %+v", got)
				}
			} else {
				if len(got.Conditions) != 1 || got.Expression.Operator != BooleanCondition {
					t.Fatalf("lowercase literal lost predicate: %+v", got)
				}
				rhs := got.Conditions[0].RHS
				if rhs == nil || len(rhs.Items) != 1 || rhs.Items[0].Kind != OperandBool || rhs.Items[0].Value != literal {
					t.Fatalf("lowercase boolean changed: %+v", rhs)
				}
			}
		})
	}
}

func TestTemporaryCapacitySpendsExistingWork(t *testing.T) {
	for _, remaining := range []int{4, 5} {
		limits := defaultParseLimits()
		limits.maxWork = 3 + remaining
		budget := &parseBudget{ctx: context.Background(), limits: limits, work: 3}
		reason := reservationAbort(func() { budget.reserveWork(5) })
		if remaining == 4 {
			if reason != "parser limit exceeded: work budget" || budget.work != 3 {
				t.Fatalf("one-short work reservation changed: reason=%q work=%d", reason, budget.work)
			}
		} else if reason != "" || budget.work != 8 || budget.output != 0 || budget.metadataItems != 0 || budget.metadataBytes != 0 {
			t.Fatalf("temporary capacity must spend only existing work: reason=%q budget=%+v", reason, budget)
		}
	}
}

func TestWorkReservationRejectsInvalidAndCanceledRequests(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, units := range []int{-1, maxInt} {
		limits := defaultParseLimits()
		limits.maxWork = maxInt
		budget := &parseBudget{ctx: context.Background(), limits: limits, work: 3}
		reason := reservationAbort(func() { budget.reserveWork(units) })
		if reason != "parser limit exceeded: work budget" || budget.work != 3 {
			t.Fatalf("invalid/overflowing work mutated budget: units=%d reason=%q budget=%+v", units, reason, budget)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	budget := &parseBudget{ctx: ctx, limits: defaultParseLimits(), work: 3}
	values := []int{1, 2}
	reason := reservationAbort(func() { values = appendTemporary(values, budget, 3) })
	if reason != "context canceled" || budget.work != 3 || !reflect.DeepEqual(values, []int{1, 2}) {
		t.Fatalf("canceled growth mutated owner: reason=%q values=%v budget=%+v", reason, values, budget)
	}
	var optional *parseBudget
	optional.reserveWork(-1) // Existing optional-budget calling convention is no-op.
}

func TestTemporaryBackingCapacityAndCopyWork(t *testing.T) {
	for _, tc := range []struct {
		remaining, capacity int
		fail                bool
	}{{4, 4, false}, {3, 3, false}, {2, 2, true}} {
		limits := defaultParseLimits()
		limits.maxWork = 3 + tc.remaining
		budget := &parseBudget{ctx: context.Background(), limits: limits, work: 3}
		original := []int{1, 2}
		values := original
		reason := reservationAbort(func() { values = appendTemporary(values, budget, 3) })
		if tc.fail {
			if reason != "parser limit exceeded: work budget" || budget.work != 3 || !reflect.DeepEqual(values, original) {
				t.Fatalf("growth happened before work reservation: reason=%q values=%v budget=%+v", reason, values, budget)
			}
			continue
		}
		if reason != "" || cap(values) != tc.capacity || budget.work != 3+tc.capacity || !reflect.DeepEqual(values, []int{1, 2, 3}) {
			t.Fatalf("full backing/copy charge changed: reason=%q values=%v cap=%d budget=%+v", reason, values, cap(values), budget)
		}
		values[0] = 9
		if original[0] != 1 {
			t.Fatal("grown owner aliases previous backing")
		}
	}
	limits := defaultParseLimits()
	limits.maxWork = 7
	budget := &parseBudget{ctx: context.Background(), limits: limits}
	var values []int
	for _, item := range []int{1, 2, 3, 4} {
		values = appendTemporary(values, budget, item)
	}
	if budget.work != 7 || cap(values) != 4 {
		t.Fatalf("replacement backings1+2+4 were not charged in full: %+v cap=%d", budget, cap(values))
	}
	reason := reservationAbort(func() { values = appendTemporary(values, budget, 5) })
	if reason != "parser limit exceeded: work budget" || len(values) != 4 || budget.work != 7 {
		t.Fatalf("exhausted shared work grew backing: %q %+v", reason, budget)
	}
}

func TestTemporaryWorkReservationPrecedesBackingAllocation(t *testing.T) {
	additions := make([]Operand, 32)
	limits := defaultParseLimits()
	limits.maxWork = 31
	var reason string
	measured := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			budget := &parseBudget{ctx: context.Background(), limits: limits}
			reason = reservationAbort(func() { appendTemporary([]Operand(nil), budget, additions...) })
		}
	})
	if reason != "parser limit exceeded: work budget" {
		t.Fatalf("wrong preallocation failure: %q", reason)
	}
	if got, backing := measured.AllocedBytesPerOp(), int64(32*unsafe.Sizeof(Operand{})); got >= backing/2 {
		t.Fatalf("rejected work reservation allocated %d bytes/op; backing alone is %d", got, backing)
	}
}
