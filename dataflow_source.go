package kql

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/antlr4-go/antlr/v4"
)

type kqlByteSpan struct{ Start, End int }

type kqlSourceOccurrence struct {
	ID, OwnerID, ParentID int
	Kind                  string
	Span                  kqlByteSpan
	Command               string
	Ordinal               int
	Removed               bool
	Rewrite               string
}

type kqlSourcePiece struct {
	Text         string
	Origin       kqlByteSpan
	OccurrenceID int
	Exact        bool
}

type kqlMappedSource struct {
	Original        string
	Text            string
	Pieces          []kqlSourcePiece
	Occurrences     []kqlSourceOccurrence
	SelectedOwnerID int
	Protected       map[string][]kqlSourcePiece
}

type kqlSourceEdit struct {
	Start, End int
	Parts      []kqlSourcePart
	Rewrite    string
}

type kqlSourcePart struct {
	Copy         *kqlByteSpan
	Text         string
	Origin       kqlByteSpan
	OccurrenceID int
}

type kqlSourceOwners struct {
	Stages    map[antlr.ParserRuleContext]int
	RHS       map[antlr.ParserRuleContext]int
	Queries   map[antlr.ParserRuleContext]int
	runeBytes []int
	source    *kqlMappedSource
}

type kqlExtractionSource struct {
	Authored  string
	Mapped    *kqlMappedSource
	OwnerID   int
	Inherited map[string]letOrigin
}

func associateKQLSource(tree antlr.Tree, src *kqlMappedSource, budget *parseBudget) *kqlSourceOwners {
	if src == nil {
		sourceAbort("missing mapped source")
	}
	owners := &kqlSourceOwners{
		Stages:    make(map[antlr.ParserRuleContext]int),
		RHS:       make(map[antlr.ParserRuleContext]int),
		Queries:   make(map[antlr.ParserRuleContext]int),
		runeBytes: newKQLRuneByteIndex(src.Text, budget), source: src,
	}
	var walk func(antlr.Tree)
	walk = func(node antlr.Tree) {
		budget.step()
		if ctx, ok := node.(antlr.ParserRuleContext); ok {
			if _, yes := ctx.(*TabularOperatorContext); yes {
				if id, ok := owners.ownerFor(ctx, "stage", budget); ok {
					owners.Stages[ctx] = id
				}
			}
			if search, yes := ctx.(*SearchOperatorContext); yes {
				if _, leading := search.GetParent().(*TabularSourceContext); leading {
					if id, ok := owners.ownerFor(ctx, "stage", budget); ok {
						owners.Stages[ctx] = id
					}
				}
			}
			if _, yes := ctx.(*ExtendItemContext); yes {
				if id, ok := owners.ownerFor(ctx, "rhs", budget); ok {
					owners.RHS[ctx] = id
				}
			}
			if _, yes := ctx.(*ProjectItemContext); yes {
				if id, ok := owners.ownerFor(ctx, "rhs", budget); ok {
					owners.RHS[ctx] = id
				}
			}
			if _, yes := ctx.(*AggregationItemContext); yes {
				if id, ok := owners.ownerFor(ctx, "rhs", budget); ok {
					owners.RHS[ctx] = id
				}
			}
			if _, yes := ctx.(*TabularExpressionContext); yes {
				if id, ok := owners.ownerFor(ctx, "query", budget); ok {
					owners.Queries[ctx] = id
				}
			}
		}
		for i := 0; i < node.GetChildCount(); i++ {
			walk(node.GetChild(i))
		}
	}
	walk(tree)
	return owners
}

func (owners *kqlSourceOwners) contextSpan(ctx antlr.ParserRuleContext) (kqlByteSpan, bool) {
	if ctx == nil || ctx.GetStart() == nil || ctx.GetStop() == nil {
		return kqlByteSpan{}, false
	}
	start, stop := ctx.GetStart().GetStart(), ctx.GetStop().GetStop()
	if start < 0 || stop < start || stop+1 >= len(owners.runeBytes) {
		return kqlByteSpan{}, false
	}
	return kqlByteSpan{owners.runeBytes[start], owners.runeBytes[stop+1]}, true
}

func (owners *kqlSourceOwners) ownerFor(ctx antlr.ParserRuleContext, kind string, budget *parseBudget) (int, bool) {
	span, ok := owners.contextSpan(ctx)
	if !ok {
		return 0, false
	}
	if kind == "query" {
		// A containing operator may have child pipelines with other owners.
		span.End = min(span.End, span.Start+1)
	}
	at, found := 0, -1
	for _, piece := range owners.source.Pieces {
		budget.step()
		end := at + len(piece.Text)
		if end > span.Start && at < span.End && piece.OccurrenceID >= 0 {
			id := piece.OccurrenceID
			for id >= 0 && id < len(owners.source.Occurrences) {
				budget.step()
				occurrence := owners.source.Occurrences[id]
				if occurrence.Kind == kind {
					if kind == "stage" {
						return id, true
					}
					if found != -1 && found != id {
						return 0, false
					}
					found = id
					break
				}
				id = occurrence.ParentID
			}
		}
		at = end
	}
	return found, found >= 0
}

func (owners *kqlSourceOwners) stageSource(ctx antlr.ParserRuleContext) (int, string, bool) {
	id, ok := owners.Stages[ctx]
	if !ok || id < 0 || id >= len(owners.source.Occurrences) {
		return 0, "", false
	}
	span := owners.source.Occurrences[id].Span
	checkedKQLSpan(span, owners.source.Original)
	return id, owners.source.Original[span.Start:span.End], true
}

func (owners *kqlSourceOwners) rhsSource(ctx antlr.ParserRuleContext) (string, bool) {
	id, ok := owners.RHS[ctx]
	if !ok || id < 0 || id >= len(owners.source.Occurrences) {
		return "", false
	}
	span := owners.source.Occurrences[id].Span
	checkedKQLSpan(span, owners.source.Original)
	return owners.source.Original[span.Start:span.End], true
}

func (owners *kqlSourceOwners) authoredContext(ctx antlr.ParserRuleContext, budget *parseBudget) (string, bool) {
	span, ok := owners.contextSpan(ctx)
	if !ok {
		return "", false
	}
	at, start, end := 0, -1, -1
	for _, piece := range owners.source.Pieces {
		budget.step()
		next := at + len(piece.Text)
		if next > span.Start && at < span.End {
			if piece.OccurrenceID < 0 {
				return "", false
			}
			origin := piece.Origin
			if piece.Exact {
				lo, hi := max(at, span.Start)-at, min(next, span.End)-at
				origin.Start += lo
				origin.End = origin.Start + hi - lo
			}
			if start < 0 {
				start = origin.Start
			}
			end = origin.End
		}
		at = next
	}
	if start < 0 || end < start {
		return "", false
	}
	checkedKQLSpan(kqlByteSpan{start, end}, owners.source.Original)
	return owners.source.Original[start:end], true
}

func (owners *kqlSourceOwners) childSource(ctx antlr.ParserRuleContext, budget *parseBudget) (kqlExtractionSource, bool) {
	id, ok := owners.ownerFor(ctx, "query", budget)
	if !ok || id == owners.source.SelectedOwnerID || id < 0 || id >= len(owners.source.Occurrences) {
		return kqlExtractionSource{}, false
	}
	originalSpan := owners.source.Occurrences[id].Span
	checkedKQLSpan(originalSpan, owners.source.Original)
	normalizedSpan, ok := owners.contextSpan(ctx)
	if !ok {
		return kqlExtractionSource{}, false
	}
	child := &kqlMappedSource{Original: owners.source.Original[originalSpan.Start:originalSpan.End], Text: owners.source.Text[normalizedSpan.Start:normalizedSpan.End], SelectedOwnerID: 0, Protected: make(map[string][]kqlSourcePiece)}
	indices := map[int]int{id: 0}
	budget.reserveMetadataStrings(1, "query")
	child.Occurrences = append(child.Occurrences, kqlSourceOccurrence{ID: 0, OwnerID: 0, ParentID: -1, Kind: "query", Span: kqlByteSpan{0, len(child.Original)}})
	for _, occurrence := range owners.source.Occurrences {
		budget.step()
		if occurrence.ID == id || occurrence.Span.Start < originalSpan.Start || occurrence.Span.End > originalSpan.End {
			continue
		}
		indices[occurrence.ID] = len(child.Occurrences)
		copy := occurrence
		copy.ID = len(child.Occurrences)
		copy.Span.Start -= originalSpan.Start
		copy.Span.End -= originalSpan.Start
		budget.reserveMetadataStrings(1, copy.Kind, copy.Command, copy.Rewrite)
		child.Occurrences = append(child.Occurrences, copy)
	}
	for i := 1; i < len(child.Occurrences); i++ {
		budget.step()
		occurrence := &child.Occurrences[i]
		parent, ok := indices[occurrence.ParentID]
		if !ok {
			sourceAbort("child occurrence escapes owner")
		}
		owner, ok := indices[occurrence.OwnerID]
		if !ok {
			sourceAbort("child owner escapes query")
		}
		occurrence.ParentID, occurrence.OwnerID = parent, owner
	}
	at := 0
	for _, piece := range owners.source.Pieces {
		budget.step()
		end := at + len(piece.Text)
		if end <= normalizedSpan.Start {
			at = end
			continue
		}
		if at >= normalizedSpan.End {
			break
		}
		lo, hi := max(at, normalizedSpan.Start)-at, min(end, normalizedSpan.End)-at
		copy := piece
		copy.Text = piece.Text[lo:hi]
		if piece.Exact {
			copy.Origin.Start += lo
			copy.Origin.End = copy.Origin.Start + hi - lo
		}
		if copy.Origin.Start < originalSpan.Start || copy.Origin.End > originalSpan.End {
			sourceAbort("child source crosses owner")
		}
		copy.Origin.Start -= originalSpan.Start
		copy.Origin.End -= originalSpan.Start
		if piece.OccurrenceID >= 0 {
			mapped, ok := indices[piece.OccurrenceID]
			if !ok {
				sourceAbort("child piece has foreign owner")
			}
			copy.OccurrenceID = mapped
		}
		budget.outputSize(len(child.Pieces) + 1)
		child.Pieces = append(child.Pieces, copy)
		at = end
	}
	for marker, saved := range owners.source.Protected {
		budget.step()
		if !strings.Contains(child.Text, marker) {
			continue
		}
		var rebased []kqlSourcePiece
		for _, piece := range saved {
			budget.step()
			if piece.Origin.Start < originalSpan.Start || piece.Origin.End > originalSpan.End {
				sourceAbort("child protected piece escapes owner")
			}
			copy := piece
			copy.Origin.Start -= originalSpan.Start
			copy.Origin.End -= originalSpan.Start
			if piece.OccurrenceID >= 0 {
				mapped, ok := indices[piece.OccurrenceID]
				if !ok {
					sourceAbort("child protected piece has foreign owner")
				}
				copy.OccurrenceID = mapped
			}
			budget.outputSize(len(rebased) + 1)
			rebased = append(rebased, copy)
		}
		child.Protected[marker] = rebased
	}
	return kqlExtractionSource{Authored: child.Original, Mapped: child, OwnerID: 0}, true
}

func restoreChildMembershipMarkers(src *kqlMappedSource, budget *parseBudget) {
	if src == nil || len(src.Protected) == 0 {
		return
	}
	var edits []kqlSourceEdit
	for marker, saved := range src.Protected {
		budget.step()
		if !strings.HasPrefix(marker, "__kql_membership_") {
			continue
		}
		needle := marker
		for at := 0; at < len(src.Text); {
			index := strings.Index(src.Text[at:], needle)
			if index < 0 {
				break
			}
			index += at
			start, end := index, index+len(needle)
			if start > 0 && src.Text[start-1] == ' ' {
				start--
			}
			if end < len(src.Text) && src.Text[end] == ' ' {
				end++
			}
			var parts []kqlSourcePart
			for _, piece := range saved {
				budget.step()
				budget.outputSize(len(parts) + 1)
				parts = append(parts, kqlSourcePart{Text: piece.Text, Origin: piece.Origin, OccurrenceID: piece.OccurrenceID})
			}
			budget.outputSize(len(edits) + 1)
			edits = append(edits, kqlSourceEdit{Start: start, End: end, Parts: parts})
			at = end
		}
	}
	if len(edits) > 0 {
		sort.Slice(edits, func(i, j int) bool { return edits[i].Start < edits[j].Start })
		*src = *applyKQLSourceEdits(src, edits, budget)
	}
}

func sourceAbort(reason string) {
	panic(budgetAbort{fmt.Errorf("dataflow source unavailable: %s", reason)})
}

func checkedKQLSpan(span kqlByteSpan, text string) {
	if span.Start < 0 || span.End < span.Start || span.End > len(text) {
		sourceAbort("invalid byte span")
	}
	if span.Start < len(text) && !utf8.RuneStart(text[span.Start]) || span.End < len(text) && !utf8.RuneStart(text[span.End]) {
		sourceAbort("source span splits UTF-8 rune")
	}
}

func newKQLMappedSource(query string, budget *parseBudget) *kqlMappedSource {
	budget.input(query)
	selected := mainQuerySpan(query)
	checkedKQLSpan(selected, query)
	src := &kqlMappedSource{Original: query, Text: query, SelectedOwnerID: 0, Protected: make(map[string][]kqlSourcePiece)}
	budget.reserveMetadataStrings(1, "query")
	src.Occurrences = append(src.Occurrences, kqlSourceOccurrence{ID: 0, OwnerID: 0, ParentID: -1, Kind: "query", Span: selected})
	// This is a bounded delimiter inventory, not another grammar parse. It
	// records lexical operator/RHS spans before normalizers can remove them.
	start := selected.Start
	sourceEnd := selected.End
	depth := 0
	ordinal := 0
	for i := selected.Start; i < selected.End; {
		budget.step()
		if next, ok := skipKQLLexicalRegion(query, i, selected.End, budget); ok {
			i = next
			continue
		}
		switch query[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				depth = 0
			}
		case '|':
			if depth == 0 {
				if start == selected.Start {
					sourceEnd = i
				}
				if start != selected.Start || isKQLLeadingOperator(query, start, i) {
					appendKQLStageInventory(src, query, start, i, ordinal, 0, budget)
					ordinal++
				}
				start = i + 1
			}
		}
		i++
	}
	if start != selected.Start || isKQLLeadingOperator(query, start, selected.End) {
		appendKQLStageInventory(src, query, start, selected.End, ordinal, 0, budget)
	}
	// A materialized source owns its inner tabular expression even when that
	// expression has no pipe (notably a leading search).
	sourceSpan := trimKQLSpan(query, selected.Start, sourceEnd)
	sourceText := query[sourceSpan.Start:sourceSpan.End]
	if len(sourceText) >= len("materialize") && strings.EqualFold(sourceText[:len("materialize")], "materialize") && strings.HasPrefix(strings.TrimSpace(sourceText[len("materialize"):]), "(") {
		appendNestedKQLInventory(src, query, sourceSpan, 0, budget)
	}
	// Partition the immutable original once so every text byte has one owner.
	budget.outputSize(4 + 2*len(src.Occurrences))
	boundaries := []int{0, len(query), selected.Start, selected.End}
	for _, occurrence := range src.Occurrences {
		budget.step()
		boundaries = append(boundaries, occurrence.Span.Start, occurrence.Span.End)
	}
	sort.Ints(boundaries)
	for i := 1; i < len(boundaries); i++ {
		lo, hi := boundaries[i-1], boundaries[i]
		if lo == hi {
			continue
		}
		owner := -1
		if lo >= selected.Start && hi <= selected.End {
			owner = 0
			bestWidth := selected.End - selected.Start + 1
			for _, occurrence := range src.Occurrences[1:] {
				budget.step()
				width := occurrence.Span.End - occurrence.Span.Start
				if lo >= occurrence.Span.Start && hi <= occurrence.Span.End && width <= bestWidth {
					owner = occurrence.ID
					bestWidth = width
				}
			}
		}
		budget.outputSize(len(src.Pieces) + 1)
		src.Pieces = append(src.Pieces, kqlSourcePiece{Text: query[lo:hi], Origin: kqlByteSpan{lo, hi}, OccurrenceID: owner, Exact: true})
	}
	return src
}

func isKQLLeadingOperator(query string, lo, hi int) bool {
	span := trimKQLSpan(query, lo, hi)
	if span.Start == span.End {
		return false
	}
	end := span.Start
	for end < span.End && (isIdentChar(query[end]) || query[end] == '-') {
		end++
	}
	switch strings.ToLower(query[span.Start:end]) {
	case "union", "where", "extend", "project", "project-away", "project-rename", "summarize", "search", "find", "top-nested", "partition", "distinct", "render":
		return true
	}
	return false
}

func skipKQLLexicalRegion(text string, at, end int, budget *parseBudget) (int, bool) {
	if at+1 < end && text[at] == '/' && text[at+1] == '/' {
		for at < end && text[at] != '\n' {
			budget.step()
			at++
		}
		return at, true
	}
	if at+1 < end && text[at] == '/' && text[at+1] == '*' {
		at += 2
		for at+1 < end && (text[at] != '*' || text[at+1] != '/') {
			budget.step()
			at++
		}
		if at+1 < end {
			at += 2
		}
		return at, true
	}
	if at+2 < end && text[at:at+3] == "```" {
		for i := at + 3; i < end; i++ {
			budget.step()
			if i+2 < end && text[i:i+3] == "```" {
				return i + 3, true
			}
		}
		return end, true
	}
	verbatim := text[at] == '@' && at+1 < end && (text[at+1] == '\'' || text[at+1] == '"')
	quoteAt := at
	if verbatim {
		quoteAt++
	}
	if text[quoteAt] == '\'' || text[quoteAt] == '"' || text[quoteAt] == '`' {
		quote := text[quoteAt]
		for i := quoteAt + 1; i < end; i++ {
			budget.step()
			if verbatim && text[i] == quote && i+1 < end && text[i+1] == quote {
				i++
				continue
			}
			if !verbatim && text[i] == '\\' && i+1 < end {
				i++
				continue
			}
			if text[i] == quote {
				return i + 1, true
			}
		}
		return end, true
	}
	return at, false
}

func isKQLSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func isKQLCommentStart(text string, at, end int) bool {
	return at+1 < end && text[at] == '/' && (text[at+1] == '/' || text[at+1] == '*')
}

func trimKQLSpan(text string, lo, hi int) kqlByteSpan {
	for lo < hi && isKQLSpaceByte(text[lo]) {
		lo++
	}
	for hi > lo && isKQLSpaceByte(text[hi-1]) {
		hi--
	}
	return kqlByteSpan{lo, hi}
}

func appendKQLStageInventory(src *kqlMappedSource, query string, lo, hi, ordinal, ownerID int, budget *parseBudget) {
	stageSpan := trimKQLSpan(query, lo, hi)
	if stageSpan.Start == stageSpan.End {
		return
	}
	// Leading comments are lexical trivia, but remain part of the authored
	// stage span. Discover the command separately for inventory and publication.
	commandStart := stageSpan.Start
	for commandStart < stageSpan.End {
		budget.step()
		if isKQLSpaceByte(query[commandStart]) {
			commandStart++
			continue
		}
		if isKQLCommentStart(query, commandStart, stageSpan.End) {
			commandStart, _ = skipKQLLexicalRegion(query, commandStart, stageSpan.End, budget)
			continue
		}
		break
	}
	commandEnd := commandStart
	for commandEnd < stageSpan.End && (isIdentChar(query[commandEnd]) || query[commandEnd] == '-') {
		commandEnd++
	}
	stageID := len(src.Occurrences)
	budget.reserveMetadataStrings(1, "stage", query[commandStart:commandEnd])
	src.Occurrences = append(src.Occurrences, kqlSourceOccurrence{ID: stageID, OwnerID: ownerID, ParentID: ownerID, Kind: "stage", Span: stageSpan, Command: query[commandStart:commandEnd], Ordinal: ordinal})
	appendNestedKQLInventory(src, query, stageSpan, stageID, budget)
	command := strings.ToLower(query[commandStart:commandEnd])
	if command != "extend" && command != "project" && command != "project-rename" && command != "summarize" {
		return
	}
	// The delimiter inventory retains unnamed expressions as well as the RHS
	// of assignments. Summarize's BY boundary separates aggregate and group
	// items before comma/equality scanning.
	rhsOrdinal := 0
	scanItems := func(lo, hi int, unnamed bool) {
		itemStart, depth := lo, 0
		for i := lo; i <= hi; i++ {
			budget.step()
			if i < hi {
				if next, ok := skipKQLLexicalRegion(query, i, hi, budget); ok {
					i = next - 1
					continue
				}
				switch query[i] {
				case '(', '[', '{':
					depth++
				case ')', ']', '}':
					depth--
				}
			}
			if i != hi && (query[i] != ',' || depth != 0) {
				continue
			}
			part := trimKQLSpan(query, itemStart, i)
			rhs := kqlByteSpan{}
			aliasAt := -1
			nesting := 0
			triviaBefore := false
			previousMemberDot := false
			for j := part.Start; j < part.End; j++ {
				budget.step()
				if isKQLSpaceByte(query[j]) {
					triviaBefore = true
					continue
				}
				if next, ok := skipKQLLexicalRegion(query, j, part.End, budget); ok {
					triviaBefore = isKQLCommentStart(query, j, part.End)
					if !triviaBefore {
						previousMemberDot = false
					}
					j = next - 1
					continue
				}
				// Comments separate tokens just like whitespace. A dot blocks an
				// alias only when it is member access, not an accepted trailing
				// decimal point. Reuse the normalizer's numeric-token boundaries.
				aliasPrefix := triviaBefore && !previousMemberDot
				triviaBefore = false
				previousMemberDot = query[j] == '.'
				if previousMemberDot && j > part.Start && isDigit(query[j-1]) && isStandaloneNumericLiteralBeforeDot(query, j) && isTrailingDecimalBoundary(query, j+1) {
					previousMemberDot = false
				}
				switch query[j] {
				case '(', '[', '{':
					nesting++
				case ')', ']', '}':
					nesting--
				}
				if nesting != 0 {
					continue
				}
				if query[j] == '=' && (j+1 >= part.End || query[j+1] != '=') && (j == part.Start || query[j-1] != '=') {
					rhs = trimKQLSpan(query, j+1, part.End)
					break
				}
				if command == "project" && j > part.Start && j+2 < part.End && query[j:j+2] == "as" && aliasPrefix && (isKQLSpaceByte(query[j+2]) || isKQLCommentStart(query, j+2, part.End)) {
					aliasAt = j
				}
			}
			if rhs.Start == rhs.End && unnamed {
				rhs = part
				if aliasAt > part.Start && trimKQLSpan(query, aliasAt+2, part.End).Start < part.End {
					rhs = trimKQLSpan(query, part.Start, aliasAt)
				}
			}
			if rhs.Start < rhs.End {
				id := len(src.Occurrences)
				budget.reserveMetadataStrings(1, "rhs")
				src.Occurrences = append(src.Occurrences, kqlSourceOccurrence{ID: id, OwnerID: ownerID, ParentID: stageID, Kind: "rhs", Span: rhs, Ordinal: rhsOrdinal})
			}
			rhsOrdinal++
			itemStart = i + 1
		}
	}
	if command != "summarize" {
		scanItems(commandEnd, stageSpan.End, command == "project")
		return
	}
	byStart, byEnd := -1, -1
	depth := 0
	for i := commandEnd; i < stageSpan.End; i++ {
		budget.step()
		if next, ok := skipKQLLexicalRegion(query, i, stageSpan.End, budget); ok {
			i = next - 1
			continue
		}
		switch query[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
		if depth == 0 && i+2 <= stageSpan.End && strings.EqualFold(query[i:i+2], "by") && (i == commandEnd || !isIdentChar(query[i-1])) && (i+2 == stageSpan.End || !isIdentChar(query[i+2])) {
			byStart, byEnd = i, i+2
			break
		}
	}
	if byStart < 0 {
		scanItems(commandEnd, stageSpan.End, true)
		return
	}
	scanItems(commandEnd, byStart, true)
	scanItems(byEnd, stageSpan.End, true)
}

func appendNestedKQLInventory(src *kqlMappedSource, query string, span kqlByteSpan, parentStage int, budget *parseBudget) {
	var scan func(int, int)
	scan = func(lo, hi int) {
		for at := lo; at < hi; at++ {
			budget.step()
			if next, ok := skipKQLLexicalRegion(query, at, hi, budget); ok {
				at = next - 1
				continue
			}
			if query[at] != '(' {
				continue
			}
			depth, close := 1, at+1
			for close < hi && depth > 0 {
				budget.step()
				if next, ok := skipKQLLexicalRegion(query, close, hi, budget); ok {
					close = next
					continue
				}
				switch query[close] {
				case '(':
					depth++
				case ')':
					depth--
				}
				close++
			}
			if depth != 0 {
				return
			}
			body := trimKQLSpan(query, at+1, close-1)
			pipe := -1
			innerDepth := 0
			for i := body.Start; i < body.End; i++ {
				budget.step()
				if next, ok := skipKQLLexicalRegion(query, i, body.End, budget); ok {
					i = next - 1
					continue
				}
				switch query[i] {
				case '(', '[', '{':
					innerDepth++
				case ')', ']', '}':
					innerDepth--
				case '|':
					if innerDepth == 0 {
						pipe = i
						i = body.End
					}
				}
			}
			parentJoin := parentStage > 0 && strings.EqualFold(src.Occurrences[parentStage].Command, "join") && !strings.Contains(strings.ToLower(query[span.Start:at]), " on ")
			if pipe < 0 && parentStage != 0 && !parentJoin {
				scan(body.Start, body.End)
				at = close - 1
				continue
			}
			queryID := len(src.Occurrences)
			budget.reserveMetadataStrings(1, "query")
			src.Occurrences = append(src.Occurrences, kqlSourceOccurrence{ID: queryID, OwnerID: queryID, ParentID: parentStage, Kind: "query", Span: body})
			stageStart, ordinal, level := body.Start, 0, 0
			for i := body.Start; i <= body.End; i++ {
				budget.step()
				if i < body.End {
					if next, ok := skipKQLLexicalRegion(query, i, body.End, budget); ok {
						i = next - 1
						continue
					}
					switch query[i] {
					case '(', '[', '{':
						level++
					case ')', ']', '}':
						level--
					}
				}
				if i == body.End || query[i] == '|' && level == 0 {
					if stageStart != body.Start || isKQLLeadingOperator(query, stageStart, i) {
						appendKQLStageInventory(src, query, stageStart, i, ordinal, queryID, budget)
						ordinal++
					}
					stageStart = i + 1
				}
			}
			at = close - 1
		}
	}
	scan(span.Start, span.End)
}

func applyKQLSourceEdits(src *kqlMappedSource, edits []kqlSourceEdit, budget *parseBudget) *kqlMappedSource {
	if src == nil {
		sourceAbort("nil mapped source")
	}
	budget.input(src.Text)
	last := 0
	for _, edit := range edits {
		budget.step()
		checkedKQLSpan(kqlByteSpan{edit.Start, edit.End}, src.Text)
		if edit.Start < last {
			sourceAbort("overlapping source edits")
		}
		last = edit.End
	}
	budget.outputSize(len(src.Occurrences))
	out := &kqlMappedSource{Original: src.Original, Occurrences: append([]kqlSourceOccurrence(nil), src.Occurrences...), SelectedOwnerID: src.SelectedOwnerID, Protected: src.Protected}
	var pieces []kqlSourcePiece
	var builder strings.Builder
	appendRange := func(lo, hi int) {
		if lo == hi {
			return
		}
		at := 0
		for _, piece := range src.Pieces {
			budget.step()
			end := at + len(piece.Text)
			if end <= lo {
				at = end
				continue
			}
			if at >= hi {
				break
			}
			pLo, pHi := max(lo, at)-at, min(hi, end)-at
			part := piece
			part.Text = piece.Text[pLo:pHi]
			if piece.Exact {
				part.Origin = kqlByteSpan{piece.Origin.Start + pLo, piece.Origin.Start + pHi}
			}
			budget.outputSize(len(pieces) + 1)
			pieces = append(pieces, part)
			builder.WriteString(part.Text)
			at = end
		}
	}
	position := 0
	for _, edit := range edits {
		appendRange(position, edit.Start)
		for _, part := range edit.Parts {
			budget.step()
			if part.Copy != nil {
				checkedKQLSpan(*part.Copy, src.Text)
				if part.Copy.Start < edit.Start || part.Copy.End > edit.End {
					sourceAbort("copy outside edited span")
				}
				appendRange(part.Copy.Start, part.Copy.End)
				continue
			}
			checkedKQLSpan(part.Origin, src.Original)
			if part.OccurrenceID < -1 || part.OccurrenceID >= len(src.Occurrences) {
				sourceAbort("invalid occurrence")
			}
			if part.OccurrenceID >= 0 && part.Origin.Start != part.Origin.End {
				ownerSpan := src.Occurrences[part.OccurrenceID].Span
				if part.Origin.Start < ownerSpan.Start || part.Origin.End > ownerSpan.End {
					sourceAbort("replacement provenance escapes owner")
				}
			}
			if part.Text == "" {
				continue
			}
			budget.outputSize(len(pieces) + 1)
			pieces = append(pieces, kqlSourcePiece{Text: part.Text, Origin: part.Origin, OccurrenceID: part.OccurrenceID})
			builder.WriteString(part.Text)
		}
		if edit.Rewrite != "" {
			rewriteOwner := -1
			for _, part := range edit.Parts {
				if part.Copy == nil && part.OccurrenceID >= 0 {
					rewriteOwner = part.OccurrenceID
					break
				}
			}
			for i := range out.Occurrences {
				budget.step()
				if out.Occurrences[i].Kind == "stage" && (spansOverlap(out.Occurrences[i].Span, originOfEdit(src, edit, budget)) || isKQLAncestor(src.Occurrences, rewriteOwner, i)) {
					if out.Occurrences[i].Rewrite != edit.Rewrite {
						budget.reserveMetadataStrings(0, edit.Rewrite)
					}
					out.Occurrences[i].Rewrite = edit.Rewrite
				}
			}
		}
		position = edit.End
	}
	appendRange(position, len(src.Text))
	budget.outputSize(len(pieces))
	coalesced := make([]kqlSourcePiece, 0, len(pieces))
	for _, piece := range pieces {
		budget.step()
		if len(coalesced) > 0 {
			last := &coalesced[len(coalesced)-1]
			if last.Exact && piece.Exact && last.OccurrenceID == piece.OccurrenceID && last.Origin.End == piece.Origin.Start {
				last.Text += piece.Text
				last.Origin.End = piece.Origin.End
				continue
			}
		}
		budget.outputSize(len(coalesced) + 1)
		coalesced = append(coalesced, piece)
	}
	out.Pieces, out.Text = coalesced, builder.String()
	for i := range out.Occurrences {
		budget.step()
		if out.Occurrences[i].Kind != "stage" {
			continue
		}
		remaining := false
		for _, piece := range coalesced {
			budget.step()
			if piece.OccurrenceID >= 0 && isKQLAncestor(out.Occurrences, piece.OccurrenceID, i) {
				remaining = true
				break
			}
		}
		out.Occurrences[i].Removed = !remaining
	}
	budget.input(out.Text)
	return out
}

func isKQLAncestor(occurrences []kqlSourceOccurrence, descendant, ancestor int) bool {
	for descendant >= 0 && descendant < len(occurrences) {
		if descendant == ancestor {
			return true
		}
		descendant = occurrences[descendant].ParentID
	}
	return false
}

func spansOverlap(a, b kqlByteSpan) bool { return a.Start < b.End && b.Start < a.End }

func originOfEdit(src *kqlMappedSource, edit kqlSourceEdit, budget *parseBudget) kqlByteSpan {
	start, end := len(src.Original), 0
	at := 0
	for _, piece := range src.Pieces {
		budget.step()
		next := at + len(piece.Text)
		if next > edit.Start && at < edit.End {
			origin := piece.Origin
			if piece.Exact {
				lo, hi := max(at, edit.Start)-at, min(next, edit.End)-at
				origin = kqlByteSpan{piece.Origin.Start + lo, piece.Origin.Start + hi}
			}
			start = min(start, origin.Start)
			end = max(end, origin.End)
		}
		at = next
	}
	if start > end {
		return kqlByteSpan{}
	}
	return kqlByteSpan{start, end}
}

func kqlPiecesInRange(src *kqlMappedSource, lo, hi int, budget *parseBudget) []kqlSourcePiece {
	checkedKQLSpan(kqlByteSpan{lo, hi}, src.Text)
	var parts []kqlSourcePiece
	at := 0
	for _, piece := range src.Pieces {
		budget.step()
		next := at + len(piece.Text)
		if next > lo && at < hi {
			start, end := max(lo, at)-at, min(hi, next)-at
			copy := piece
			copy.Text = piece.Text[start:end]
			if piece.Exact {
				copy.Origin = kqlByteSpan{piece.Origin.Start + start, piece.Origin.Start + end}
			}
			budget.outputSize(len(parts) + 1)
			parts = append(parts, copy)
		}
		at = next
	}
	return parts
}

func newKQLRuneByteIndex(text string, budget *parseBudget) []int {
	runes := 0
	for range text {
		budget.step()
		runes++
	}
	budget.outputSize(runes + 1)
	index := make([]int, 0, runes+1)
	for at := range text {
		budget.step()
		index = append(index, at)
	}
	index = append(index, len(text))
	return index
}
