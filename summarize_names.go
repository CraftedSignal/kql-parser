package kql

import (
	"strconv"

	"github.com/antlr4-go/antlr/v4"
)

// Kusto's projection builder separates used names from declared names. A
// declaration records its allocated name, which may differ from its request.
// This state is temporary and belongs to exactly one summarize stage.
type summarizeNameAllocator struct {
	used     map[string]int
	declared map[string]bool
	budget   *parseBudget
}

func newSummarizeNameAllocator(capacity int, budget *parseBudget) *summarizeNameAllocator {
	budget.reserveWork(capacity)
	budget.reserveWork(capacity)
	return &summarizeNameAllocator{used: make(map[string]int, capacity), declared: make(map[string]bool, capacity), budget: budget}
}

func (a *summarizeNameAllocator) nameWork(name string) {
	a.budget.reserveWork(len(name))
	a.budget.step()
}

func (a *summarizeNameAllocator) add(name string) string {
	a.nameWork(name)
	suffix, exists := a.used[name]
	if !exists {
		a.nameWork(name)
		a.used[name] = 0
		return name
	}
	// Each rejected candidate is occupied, so a free complete name must occur
	// within occupied-count+1 attempts. Remember suffix state for each full base.
	for attempts := 0; attempts <= len(a.used); attempts++ {
		a.budget.step()
		if suffix == int(^uint(0)>>1) {
			a.budget.reject("summarize name suffix")
		}
		suffix++
		width := 1
		for n := suffix; n >= 10; n /= 10 {
			a.budget.step()
			width++
		}
		if len(name) > int(^uint(0)>>1)-width {
			a.budget.reject("work budget")
		}
		// Charge both the decimal string and the full candidate before allocation.
		a.budget.reserveWork(width)
		a.budget.reserveWork(len(name))
		a.budget.reserveWork(width)
		candidate := name + strconv.Itoa(suffix)
		a.nameWork(candidate)
		if _, occupied := a.used[candidate]; occupied {
			continue
		}
		// All reservations precede mutation, including both map-key hash scans.
		a.nameWork(name)
		a.nameWork(candidate)
		a.used[name] = suffix
		a.used[candidate] = 0
		return candidate
	}
	a.budget.reject("summarize name search")
	return ""
}

func (a *summarizeNameAllocator) declare(name string) (string, bool) {
	a.nameWork(name)
	if a.declared[name] {
		return "", false
	}
	// The allocated name may have at most the full decimal width of an int.
	// Reserve its future insertion separately after Add, before that insertion.
	allocated := a.add(name)
	a.nameWork(allocated)
	a.declared[allocated] = true
	return allocated, true
}

type summarizePlanStatus uint8

const (
	summarizeRepresented summarizePlanStatus = iota
	summarizeUnknown
	summarizeInvalidDeclaration
)

func (a *summarizeNameAllocator) declareReplacing(name string) (string, bool, bool) {
	a.nameWork(name)
	if a.declared[name] {
		return "", false, false
	}
	if _, occupied := a.used[name]; occupied {
		a.nameWork(name)
		a.declared[name] = true
		return name, true, true
	}
	allocated, valid := a.declare(name)
	return allocated, valid, false
}

type summarizeMetricName struct {
	member             int
	replace            bool
	index              int
	column, expression string
	declaration        bool
}
type summarizeGroupName struct{ column, expression string }
type summarizeNamePlan struct {
	status  summarizePlanStatus
	metrics []summarizeMetricName
	groups  []summarizeGroupName
	visible []int
}

func (b *kqlDataflowBuilder) indexAggregationOwners() {
	n := len(b.extractor.aggregations)
	if n != len(b.extractor.aggregationContexts) {
		sourceAbort("aggregate occurrence owners are not parallel")
	}
	b.budget.reserveWork(n)
	b.aggregateOwners = make(map[*AggregationItemContext]int, n)
	for i, owner := range b.extractor.aggregationContexts {
		b.budget.step()
		if owner == nil {
			sourceAbort("aggregate occurrence owner is nil")
		}
		b.budget.step()
		if _, exists := b.aggregateOwners[owner]; exists {
			sourceAbort("aggregate occurrence owner is repeated")
		}
		b.budget.step()
		b.aggregateOwners[owner] = i
	}
}

func (b *kqlDataflowBuilder) planSummarizeNames(summary ISummarizeOperatorContext, info dataflowOperatorInfo) summarizeNamePlan {
	plan := summarizeNamePlan{status: summarizeUnknown}
	if summary.AggregationList() == nil || b.unrepresentedSummarizeOutputs(summary.AggregationList()) || b.summarizeHasUnaliasedIntegerGroup(summary.GroupByList()) {
		return plan
	}
	metricCount, groupCount := 0, 0
	for _, child := range summary.AggregationList().GetChildren() {
		b.budget.step()
		if _, ok := child.(*AggregationItemContext); ok {
			metricCount++
		}
	}
	if groups := summary.GroupByList(); groups != nil {
		for _, child := range groups.GetChildren() {
			b.budget.step()
			if _, ok := child.(IGroupByItemContext); ok {
				groupCount++
			}
		}
	}
	b.budget.reserveWork(metricCount)
	plan.metrics = make([]summarizeMetricName, 0, metricCount)
	b.budget.reserveWork(groupCount)
	plan.groups = make([]summarizeGroupName, 0, groupCount)
	if groups := summary.GroupByList(); groups != nil {
		for _, child := range groups.GetChildren() {
			b.budget.step()
			item, ok := child.(IGroupByItemContext)
			if !ok {
				continue
			}
			if item.Expression() == nil || b.extractor.typed != nil && b.extractor.typed.hasOpaqueReference(item.Expression()) {
				return plan
			}
			expression, ok := b.owners.authoredContext(item.Expression(), b.budget)
			if !ok {
				sourceAbort("group expression lacks authored span")
			}
			var column string
			if item.ColumnName() != nil {
				column = b.authoredIdentifier(item.ColumnName())
			} else {
				column, ok = b.summarizeGroupReferenceName(item.Expression())
				if !ok {
					return plan
				}
			}
			plan.groups = append(plan.groups, summarizeGroupName{column, expression})
		}
	}
	for _, child := range summary.AggregationList().GetChildren() {
		b.budget.step()
		item, ok := child.(*AggregationItemContext)
		if !ok {
			continue
		}
		b.budget.step()
		index, found := b.aggregateOwners[item]
		if !found {
			sourceAbort("represented aggregate lacks structured occurrence")
		}
		candidate := b.extractor.aggregations[index]
		if candidate.PipeStage != info.PipeStage || candidate.Pipeline != info.Pipeline || candidate.BindingID != "" {
			sourceAbort("aggregate occurrence owner is invalid")
		}
		if item.AggregationFunction() == nil {
			return plan
		}
		expression, ok := b.owners.authoredContext(item.AggregationFunction(), b.budget)
		if !ok {
			sourceAbort("aggregate expression lacks authored span")
		}
		if candidate.Result != nil {
			names, declared, known := planAggregateMembers(item, b.extractor, b.budget)
			if !known {
				return plan
			}
			for ordinal, name := range names {
				b.budget.step()
				plan.metrics = appendTemporary(plan.metrics, b.budget, summarizeMetricName{index: index, column: name, expression: expression, declaration: declared[ordinal], member: ordinal, replace: ordinal == 0 && item.ASSIGN() != nil && item.ColumnName() != nil})
			}
		} else {
			if candidate.Column == "" && item.ColumnName() == nil {
				return plan
			}
			plan.metrics = appendTemporary(plan.metrics, b.budget, summarizeMetricName{index: index, column: candidate.Column, expression: expression, declaration: item.ColumnName() != nil})
		}
	}

	allocator := newSummarizeNameAllocator(len(plan.metrics)+groupCount, b.budget)
	for i := range plan.groups {
		b.budget.step()
		allocated, valid := allocator.declare(plan.groups[i].column)
		if !valid {
			plan.status = summarizeInvalidDeclaration
			return plan
		}
		plan.groups[i].column = allocated
	}
	for i := range plan.metrics {
		b.budget.step()
		metric := &plan.metrics[i]
		replaced := false
		if metric.declaration {
			var allocated string
			var valid bool
			if metric.replace {
				allocated, valid, replaced = allocator.declareReplacing(metric.column)
			} else {
				allocated, valid = allocator.declare(metric.column)
			}
			if !valid {
				plan.status = summarizeInvalidDeclaration
				return plan
			}
			metric.column = allocated
		} else {
			metric.column = allocator.add(metric.column)
		}
		if replaced {
			found := false
			for slot, prior := range plan.visible {
				b.budget.step()
				if plan.metrics[prior].column == metric.column {
					plan.visible[slot] = i
					found = true
					break
				}
			}
			if !found {
				sourceAbort("replacement lacks automatic projection slot")
			}
		} else {
			plan.visible = appendTemporary(plan.visible, b.budget, i)
		}
	}

	plan.status = summarizeRepresented
	return plan
}

// Only typed direct column references declare an unaliased BY output. A path
// requires unavailable left-side type information; a scalar let is not a column.
func (b *kqlDataflowBuilder) summarizeGroupReferenceName(tree antlr.Tree) (string, bool) {
	for tree != nil {
		b.budget.step()
		if ctx, ok := tree.(antlr.ParserRuleContext); ok && b.extractor.typed != nil && ctx.GetStart() != nil {
			if origin, present := b.extractor.typed.byStart[ctx.GetStart().GetStart()]; present {
				if origin.binding != "" || origin.bracketReference || origin.opaqueReference {
					return "", false
				}
			}
		}
		switch ctx := tree.(type) {
		case IIdentifierContext:
			b.budget.reserveWork(ctx.GetStop().GetStop() - ctx.GetStart().GetStart() + 1)
			text := ctx.GetText()
			if numericToken.MatchString(text) || text == "true" || text == "false" {
				return "", false
			}
			return b.authoredIdentifier(ctx), true
		case IPrimaryExpressionContext:
			if ctx.QUOTED_IDENTIFIER() != nil {
				return b.authoredIdentifier(ctx), true
			}
		case IPostfixExpressionContext:
			for _, child := range ctx.GetChildren() {
				b.budget.step()
				if _, postfix := child.(IPostfixOperatorContext); postfix {
					return "", false
				}
			}
		}
		tree = kqlTransparentScalarChild(tree, b.budget)
	}
	return "", false
}
