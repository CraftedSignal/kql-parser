package kql

import (
	"sort"
	"strings"
)

type kqlEditSink struct {
	Source *kqlMappedSource
	Edits  []kqlSourceEdit
	Budget *parseBudget
}

type kqlSourceRecorder = kqlEditSink

func (recorder *kqlEditSink) Emit(edit kqlSourceEdit) {
	recorder.Budget.step()
	recorder.Budget.outputSize(len(recorder.Edits) + 1)
	recorder.Budget.outputSize(len(edit.Parts))
	checkedKQLSpan(kqlByteSpan{edit.Start, edit.End}, recorder.Source.Text)
	for _, part := range edit.Parts {
		recorder.Budget.step()
		if part.Copy == nil {
			checkedKQLSpan(part.Origin, recorder.Source.Original)
			if part.OccurrenceID < -1 || part.OccurrenceID >= len(recorder.Source.Occurrences) {
				sourceAbort("edit has invalid owner")
			}
		}
	}
	recorder.Edits = append(recorder.Edits, edit)
}

func (recorder *kqlEditSink) Owner(start, end int, kind string) (int, bool) {
	checkedKQLSpan(kqlByteSpan{start, end}, recorder.Source.Text)
	owner := -1
	at := 0
	for _, piece := range recorder.Source.Pieces {
		recorder.Budget.step()
		next := at + len(piece.Text)
		if next <= start || at >= end {
			at = next
			continue
		}
		id := piece.OccurrenceID
		for id >= 0 && id < len(recorder.Source.Occurrences) && recorder.Source.Occurrences[id].Kind != kind {
			recorder.Budget.step()
			id = recorder.Source.Occurrences[id].ParentID
		}
		if id < 0 || owner >= 0 && owner != id {
			return 0, false
		}
		owner = id
		at = next
	}
	return owner, owner >= 0
}

func (recorder *kqlEditSink) Replace(start, end int, replacement, rewrite string) {
	recorder.replace(start, end, replacement, rewrite)
}

func (recorder *kqlEditSink) ReplaceOwned(start, end int, replacement, rewrite string, occurrenceID int) {
	if occurrenceID < 0 || occurrenceID >= len(recorder.Source.Occurrences) {
		sourceAbort("explicit edit owner is invalid")
	}
	edit := kqlSourceEdit{Start: start, End: end, Rewrite: rewrite}
	if replacement != "" {
		recorder.Budget.outputSize(1)
		edit.Parts = []kqlSourcePart{{Text: replacement, Origin: recorder.Source.Occurrences[occurrenceID].Span, OccurrenceID: occurrenceID}}
	}
	recorder.Emit(edit)
}

func (recorder *kqlSourceRecorder) replace(start, end int, replacement, rewrite string) {
	recorder.Budget.step()
	edit := kqlSourceEdit{Start: start, End: end, Rewrite: rewrite}
	if replacement != "" {
		owner := kqlOccurrenceAt(recorder.Source, start, recorder.Budget)
		recorder.Budget.outputSize(1)
		edit.Parts = []kqlSourcePart{{Text: replacement, Origin: originOfEdit(recorder.Source, edit, recorder.Budget), OccurrenceID: owner}}
	}
	recorder.Emit(edit)
}

func replaceAllKQLMapped(query, old, replacement string, recorder *kqlSourceRecorder) string {
	if old == "" || !strings.Contains(query, old) {
		return query
	}
	var result strings.Builder
	last := 0
	for at := 0; at < len(query); {
		index := strings.Index(query[at:], old)
		if index < 0 {
			break
		}
		index += at
		result.WriteString(query[last:index])
		result.WriteString(replacement)
		if recorder != nil && recorder.Source.Text == query {
			recorder.replace(index, index+len(old), replacement, "")
		}
		at = index + len(old)
		last = at
	}
	result.WriteString(query[last:])
	text := result.String()
	if recorder != nil && len(recorder.Edits) > 0 {
		*recorder.Source = *applyKQLSourceEdits(recorder.Source, recorder.Edits, recorder.Budget)
		recorder.Edits = nil
		if recorder.Source.Text != text {
			sourceAbort("literal replacement edit mismatch")
		}
	}
	return text
}

func kqlRecorder(metadata *kqlNormalizationMetadata, budget *parseBudget) *kqlSourceRecorder {
	if metadata == nil || metadata.mapped == nil {
		return nil
	}
	return &kqlEditSink{Source: metadata.mapped, Budget: budget}
}

func finishKQLRecorded(input, output string, recorder *kqlSourceRecorder) string {
	if recorder != nil && len(recorder.Edits) != 0 {
		if recorder.Source.Text != input {
			sourceAbort("source edit input mismatch")
		}
		sort.SliceStable(recorder.Edits, func(i, j int) bool { return recorder.Edits[i].Start < recorder.Edits[j].Start })
		*recorder.Source = *applyKQLSourceEdits(recorder.Source, recorder.Edits, recorder.Budget)
		recorder.Edits = nil
		if recorder.Source.Text != output {
			sourceAbort("source edit output mismatch")
		}
	}
	return output
}

func recordKQLInsertion(recorder *kqlSourceRecorder, at int, text, rewrite string) {
	if recorder == nil || recorder.Source.Text == "" {
		return
	}
	ownerAt := min(at, len(recorder.Source.Text)-1)
	owner := kqlOccurrenceAt(recorder.Source, ownerAt, recorder.Budget)
	if owner < 0 && at > 0 {
		owner = kqlOccurrenceAt(recorder.Source, at-1, recorder.Budget)
	}
	if owner < 0 {
		sourceAbort("insertion has no authored owner")
	}
	recorder.ReplaceOwned(at, at, text, rewrite, owner)
}

func kqlOccurrenceAt(src *kqlMappedSource, position int, budget *parseBudget) int {
	at := 0
	for _, piece := range src.Pieces {
		budget.step()
		next := at + len(piece.Text)
		if position >= at && position < next {
			return piece.OccurrenceID
		}
		at = next
	}
	return -1
}

func recordKQLLiteralEdits(src *kqlMappedSource, matches []kqlByteSpan, old, new string, budget *parseBudget) {
	if src == nil || len(matches) == 0 {
		return
	}
	prefix := 0
	for prefix < len(old) && prefix < len(new) && old[prefix] == new[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(new)-prefix && old[len(old)-1-suffix] == new[len(new)-1-suffix] {
		suffix++
	}
	budget.outputSize(len(matches))
	edits := make([]kqlSourceEdit, 0, len(matches))
	for _, match := range matches {
		budget.step()
		checkedKQLSpan(match, src.Text)
		edit := kqlSourceEdit{Start: match.Start, End: match.End}
		if prefix > 0 {
			copySpan := kqlByteSpan{match.Start, match.Start + prefix}
			budget.outputSize(len(edit.Parts) + 1)
			edit.Parts = append(edit.Parts, kqlSourcePart{Copy: &copySpan})
		}
		middle := new[prefix : len(new)-suffix]
		if middle != "" {
			originalMiddle := kqlByteSpan{match.Start + prefix, match.End - suffix}
			origin := originOfEdit(src, kqlSourceEdit{Start: originalMiddle.Start, End: originalMiddle.End}, budget)
			owner := kqlOccurrenceAt(src, originalMiddle.Start, budget)
			budget.outputSize(len(edit.Parts) + 1)
			edit.Parts = append(edit.Parts, kqlSourcePart{Text: middle, Origin: origin, OccurrenceID: owner})
		}
		if suffix > 0 {
			copySpan := kqlByteSpan{match.End - suffix, match.End}
			budget.outputSize(len(edit.Parts) + 1)
			edit.Parts = append(edit.Parts, kqlSourcePart{Copy: &copySpan})
		}
		edits = append(edits, edit)
	}
	*src = *applyKQLSourceEdits(src, edits, budget)
}
