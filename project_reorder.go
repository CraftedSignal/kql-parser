package kql

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
)

// Validate typed occurrences against the pre-normalization operator inventory.
// This includes authored occurrences that parser recovery never reached, so
// portable predicate extraction cannot promote a malformed reorder stage.
func validateKQLProjectReorders(tree antlr.Tree, owners *kqlSourceOwners, syntaxErrors bool, budget *parseBudget) string {
	budget.step()
	if owners == nil || owners.source == nil {
		return "project-reorder source ownership unavailable"
	}
	expected := 0
	for _, occurrence := range owners.source.Occurrences {
		budget.step()
		if occurrence.Kind == "stage" && len(occurrence.Command) == len("project-reorder") {
			budget.reserveWork(len(occurrence.Command))
			if strings.EqualFold(occurrence.Command, "project-reorder") {
				expected++
			}
		}
	}
	if expected > 0 && syntaxErrors {
		return "malformed project-reorder syntax"
	}
	var seen []bool
	if expected > 0 {
		budget.reserveWork(len(owners.source.Occurrences))
		seen = make([]bool, len(owners.source.Occurrences))
	}
	found := 0
	var walk func(antlr.Tree) string
	walk = func(node antlr.Tree) string {
		budget.step()
		if reorder, ok := node.(*ProjectReorderOperatorContext); ok {
			if syntaxErrors {
				return "malformed project-reorder syntax"
			}
			parent, ok := reorder.GetParent().(*TabularOperatorContext)
			if !ok {
				return "project-reorder operator ownership unavailable"
			}
			id, ok := owners.Stages[parent]
			if !ok || id < 0 || id >= len(seen) {
				return "project-reorder authored occurrence unavailable"
			}
			occurrence := owners.source.Occurrences[id]
			if occurrence.Kind != "stage" || occurrence.Command != "project-reorder" || occurrence.Removed || seen[id] {
				return "project-reorder authored occurrence mismatch"
			}
			query, ok := parent.GetParent().(*TabularExpressionContext)
			if !ok {
				return "project-reorder query ownership unavailable"
			}
			owner, ok := owners.Queries[query]
			if !ok || owner != occurrence.OwnerID {
				return "project-reorder query ownership mismatch"
			}
			if diagnostic := validateKQLProjectReorderTree(reorder, owners, budget); diagnostic != "" {
				return diagnostic
			}
			seen[id], found = true, found+1
			return ""
		}
		for _, child := range node.GetChildren() {
			if diagnostic := walk(child); diagnostic != "" {
				return diagnostic
			}
		}
		return ""
	}
	if diagnostic := walk(tree); diagnostic != "" {
		return diagnostic
	}
	if found != expected {
		return "project-reorder authored occurrence was not parsed completely"
	}
	return ""
}

func validateKQLProjectReorderTree(node antlr.Tree, owners *kqlSourceOwners, budget *parseBudget) string {
	budget.step()
	if _, bad := node.(antlr.ErrorNode); bad {
		return "recovered project-reorder syntax is unsupported"
	}
	if terminal, ok := node.(antlr.TerminalNode); ok {
		if terminal.GetSymbol().GetTokenIndex() < 0 {
			return "missing project-reorder token"
		}
		return ""
	}
	if ctx, ok := node.(antlr.ParserRuleContext); ok {
		if ctx.GetStart() == nil || ctx.GetStop() == nil || ctx.GetStart().GetTokenIndex() < 0 || ctx.GetStop().GetTokenIndex() < ctx.GetStart().GetTokenIndex() {
			return "incomplete project-reorder syntax"
		}
		if _, direction := ctx.(IProjectReorderDirectionContext); direction {
			authored, ok := owners.authoredContext(ctx, budget)
			if !ok {
				return "project-reorder direction source unavailable"
			}
			budget.reserveWork(len(authored))
			switch authored {
			case "asc", "desc", "granny-asc", "granny-desc":
			default:
				return "invalid project-reorder direction: expected asc, desc, granny-asc or granny-desc"
			}
		}
	}
	for _, child := range node.GetChildren() {
		if diagnostic := validateKQLProjectReorderTree(child, owners, budget); diagnostic != "" {
			return diagnostic
		}
	}
	return ""
}

// Reuse the mapped operator inventory instead of searching source interiors.
// The eventual typed validator still rejects malformed/recovered occurrences.
func projectReorderOwnsOffset(source *kqlMappedSource, offset int, budget *parseBudget) bool {
	id := kqlOccurrenceAt(source, offset, budget)
	for id >= 0 && id < len(source.Occurrences) {
		budget.step()
		occurrence := source.Occurrences[id]
		if occurrence.Kind == "stage" {
			return occurrence.Command == "project-reorder"
		}
		id = occurrence.ParentID
	}
	return false
}
