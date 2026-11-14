package kql

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
)

// SearchPredicate is an opaque, source-preserved tabular search. Consumers must
// handle the search operator and its scope, or explicitly report it unsupported.
type SearchPredicate struct {
	Text         string   `json:"text"`
	Kind         string   `json:"kind,omitempty"`
	Scope        string   `json:"scope"`
	TableSources []string `json:"table_sources,omitempty"`
	PipeStage    int      `json:"pipe_stage"`
	Pipeline     int      `json:"pipeline"`
}

func cloneSearchPredicate(search *SearchPredicate, budget *parseBudget) *SearchPredicate {
	budget.step()
	if search == nil {
		return nil
	}
	budget.reserveOutput(1)
	copy := *search
	copy.TableSources = cloneSemanticStrings(search.TableSources, budget)
	return &copy
}

func searchPredicateContext(ctx ISearchOperatorContext, budget *parseBudget) (*SearchPredicate, string) {
	budget.step()
	if ctx.SearchPredicate() == nil {
		return nil, "malformed search predicate"
	}
	budget.reserveOutput(1)
	search := &SearchPredicate{Text: kqlSourceText(ctx.SearchPredicate(), budget), Scope: "database"}
	if _, input := ctx.GetParent().(ITabularOperatorContext); input {
		search.Scope = "input"
	}
	if kind := ctx.SearchKind(); kind != nil {
		search.Kind = kind.Identifier().GetText()
		switch search.Kind {
		case "default", "case_insensitive", "case_sensitive":
		default:
			return nil, "unsupported search kind"
		}
	}
	if ctx.IN() != nil {
		if search.Scope == "input" {
			return nil, "search table scope cannot be combined with input"
		}
		search.Scope = "tables"
		if len(ctx.AllSearchTableSource()) == 0 {
			return nil, "search table scope is empty"
		}
		for _, selector := range ctx.AllSearchTableSource() {
			budget.step()
			qualifiers := selector.AllSearchQualifier()
			if len(qualifiers) > 2 {
				return nil, "unsupported search table selector"
			}
			for index, qualifier := range qualifiers {
				budget.step()
				name := qualifier.Identifier().GetText()
				if name != "database" && name != "cluster" || name == "cluster" && index != 0 || index == 1 && name != "database" {
					return nil, "unsupported search table selector"
				}
			}
			for _, component := range selector.AllSearchTableComponent() {
				budget.step()
				if kqlSourceText(component, budget) != component.GetText() {
					return nil, "unsupported search table selector"
				}
			}
			budget.reserveOutput(1)
			search.TableSources = append(search.TableSources, kqlSourceText(selector, budget))
		}
	}
	return search, ""
}

func validateKQLSearches(tree antlr.Tree, budget *parseBudget) (int, string) {
	budget.step()
	if search, ok := tree.(ISearchOperatorContext); ok {
		_, diagnostic := searchPredicateContext(search, budget)
		return 1, diagnostic
	}
	count := 0
	for i := 0; i < tree.GetChildCount(); i++ {
		budget.step()
		found, diagnostic := validateKQLSearches(tree.GetChild(i), budget)
		if diagnostic != "" {
			return count + found, diagnostic
		}
		count += found
	}
	return count, ""
}

func (e *conditionExtractor) EnterSearchOperator(ctx *SearchOperatorContext) {
	if e.inSubquery > 0 || e.inSearch > 0 {
		e.inSearch++
		return
	}
	e.inSearch++
	search, diagnostic := searchPredicateContext(ctx, e.budget)
	if diagnostic != "" {
		e.errors = append(e.errors, diagnostic)
		return
	}
	search.PipeStage, search.Pipeline = e.currentStage, e.currentPipeline()
	e.budget.reserveOutput(1)
	if e.searchLeaves == nil {
		e.searchLeaves = make(map[ISearchOperatorContext]*ConditionExpression)
	}
	e.searchLeaves[ctx] = &ConditionExpression{Operator: BooleanSearch, Search: search}
	e.budget.reserveOutput(1)
	e.searchOrder = append(e.searchOrder, ctx)
	e.commands = append(e.commands, "search")
}

func (e *conditionExtractor) ExitSearchOperator(ctx *SearchOperatorContext) {
	e.inSearch--
	if _, leading := ctx.GetParent().(ITabularSourceContext); leading && e.inSearch == 0 && e.inSubquery == 0 {
		e.currentStage++
		if pipeline := e.currentPipeline(); pipeline >= 0 {
			e.pipelines[pipeline].Operators++
		}
	}
}

func equalSearchPredicate(a, b *SearchPredicate, budget *parseBudget) bool {
	budget.step()
	if a == nil || b == nil {
		return a == b
	}
	if a.Text != b.Text || a.Kind != b.Kind || a.Scope != b.Scope || a.PipeStage != b.PipeStage || a.Pipeline != b.Pipeline || len(a.TableSources) != len(b.TableSources) {
		return false
	}
	for i := range a.TableSources {
		budget.step()
		if a.TableSources[i] != b.TableSources[i] {
			return false
		}
	}
	return true
}

func searchSources(extractor *conditionExtractor, budget *parseBudget) []string {
	budget.step()
	var sources []string
	for _, pipeline := range extractor.pipelines {
		budget.step()
		if pipeline.Source != "" && !strings.EqualFold(pipeline.Source, "search") {
			budget.reserveOutput(1)
			sources = appendUnique(sources, pipeline.Source)
		}
	}
	for _, context := range extractor.searchOrder {
		budget.step()
		leaf := extractor.searchLeaves[context]
		for _, source := range leaf.Search.TableSources {
			budget.reserveOutput(1)
			sources = appendUnique(sources, source)
		}
	}
	return sources
}
