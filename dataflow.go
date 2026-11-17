package kql

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
)

// QueryDataflow records rowset stages and the field environment visible at
// each stage. IDs are local to the containing parse result.
type QueryDataflow struct {
	Complete      bool                 `json:"complete"`
	OutputScopeID string               `json:"output_scope_id,omitempty"`
	Scopes        []FieldScope         `json:"scopes,omitempty"`
	Stages        []DataflowStage      `json:"stages,omitempty"`
	Bindings      []FieldBinding       `json:"bindings,omitempty"`
	Diagnostics   []DataflowDiagnostic `json:"diagnostics,omitempty"`
}

type FieldScope struct {
	ID            string   `json:"id"`
	Pipeline      int      `json:"pipeline"`
	Mode          string   `json:"mode"`
	ParentID      string   `json:"parent_id,omitempty"`
	Source        string   `json:"source,omitempty"`
	BindingIDs    []string `json:"binding_ids,omitempty"`
	RemovedFields []string `json:"removed_fields,omitempty"`
}

type ScopeReference struct {
	QueryPath string `json:"query_path,omitempty"`
	ScopeID   string `json:"scope_id"`
}

type DataflowStage struct {
	ID            string           `json:"id"`
	Pipeline      int              `json:"pipeline"`
	PipeStage     int              `json:"pipe_stage"`
	Command       string           `json:"command"`
	Source        string           `json:"source"`
	Effect        string           `json:"effect"`
	Inputs        []ScopeReference `json:"inputs,omitempty"`
	OutputScopeID string           `json:"output_scope_id"`
	BindingIDs    []string         `json:"binding_ids,omitempty"`
}

type FieldBinding struct {
	ID           string `json:"id"`
	StageID      string `json:"stage_id"`
	Ordinal      int    `json:"ordinal"`
	Field        string `json:"field"`
	Kind         string `json:"kind"`
	Expression   string `json:"expression"`
	InputScopeID string `json:"input_scope_id"`
}

type FieldBindingRef struct {
	ScopeID   string `json:"scope_id"`
	Kind      string `json:"kind"`
	BindingID string `json:"binding_id,omitempty"`
}

type DataflowDiagnostic struct {
	Code    string `json:"code"`
	StageID string `json:"stage_id,omitempty"`
	Message string `json:"message"`
}

type dataflowOwnerFrame struct {
	OwnerID        int
	Pipeline       int
	InputScopeID   string
	CurrentScopeID string
	Selected       bool
	ExternalChild  bool
}

type dataflowOperatorInfo struct{ PipeStage, Pipeline int }

type kqlDataflowBuilder struct {
	flow            *QueryDataflow
	extractor       *conditionExtractor
	owners          *kqlSourceOwners
	budget          *parseBudget
	current         map[int]string
	stageByPosition map[dataflowOperatorInfo]int
	stageBySearch   map[ISearchOperatorContext]int
	stageByOwner    map[int]int
	tombstones      map[int]bool
	ownerPipeline   map[int]int
	lastPipeStage   map[int]int
}

func buildKQLDataflow(tree antlr.Tree, owners *kqlSourceOwners, extractor *conditionExtractor, budget *parseBudget) *QueryDataflow {
	budget.reserveMetadata(1, 0)
	b := &kqlDataflowBuilder{flow: &QueryDataflow{Complete: true}, extractor: extractor, owners: owners, budget: budget, current: make(map[int]string), stageByPosition: make(map[dataflowOperatorInfo]int), stageBySearch: make(map[ISearchOperatorContext]int), stageByOwner: make(map[int]int), tombstones: make(map[int]bool), ownerPipeline: make(map[int]int), lastPipeStage: make(map[int]int)}
	b.ownerPipeline[owners.source.SelectedOwnerID] = extractor.queryPipeline
	var walk func(antlr.Tree)
	walk = func(node antlr.Tree) {
		budget.step()
		if query, ok := node.(*TabularExpressionContext); ok {
			owner, mapped := owners.Queries[query]
			pipeline, local := extractor.dataflowPipelines[query]
			if mapped && local {
				b.ownerPipeline[owner] = pipeline
			}
		}
		if operator, ok := node.(*TabularOperatorContext); ok {
			b.operator(operator)
		}
		if search, ok := node.(*SearchOperatorContext); ok {
			if _, leading := search.GetParent().(*TabularSourceContext); leading {
				b.leadingSearch(search)
			}
		}
		for i := 0; i < node.GetChildCount(); i++ {
			child := node.GetChild(i)
			// A join child is separately returned under Joins and has its own graph.
			if operator, ok := node.(*JoinOperatorContext); ok && child == operator.TabularExpression() {
				continue
			}
			walk(child)
		}
		if source, ok := node.(*TabularSourceContext); ok && source.TabularExpression() != nil {
			child, childOK := source.TabularExpression().(*TabularExpressionContext)
			parent, parentOK := source.GetParent().(*TabularExpressionContext)
			if !childOK || !parentOK {
				sourceAbort("unresolved local source owner")
			}
			childPipeline, childOK := extractor.dataflowPipelines[child]
			parentPipeline, parentOK := extractor.dataflowPipelines[parent]
			if !childOK || !parentOK {
				sourceAbort("local source has no output scope")
			}
			childOutput, outputOK := b.current[childPipeline]
			if !outputOK && len(child.AllTabularOperator()) == 0 && child.TabularSource() != nil && child.TabularSource().SearchOperator() == nil {
				childOutput = b.inputScope(childPipeline)
				outputOK = true
			}
			if !outputOK {
				sourceAbort("local source has no output scope")
			}
			b.current[parentPipeline] = childOutput
		}
		if query, ok := node.(*TabularExpressionContext); ok {
			if owner, found := owners.Queries[query]; found {
				if _, local := b.ownerPipeline[owner]; local {
					b.emitPriorTombstones(owner, owners.source.Occurrences[owner].Span.End+1)
				}
			}
		}
	}
	walk(tree)
	b.attachLocalUnionInputs()
	for _, occurrence := range owners.source.Occurrences {
		budget.step()
		if occurrence.Kind != "stage" {
			continue
		}
		if _, local := b.ownerPipeline[occurrence.OwnerID]; !local {
			continue
		}
		if _, found := b.stageByOwner[occurrence.ID]; !found {
			sourceAbort("authored stage lacks graph record")
		}
	}
	if selected, ok := b.current[extractor.queryPipeline]; ok {
		budget.reserveMetadataStrings(0, selected)
		b.flow.OutputScopeID = selected
	} else if extractor.queryPipeline >= 0 {
		selected := b.inputScope(extractor.queryPipeline)
		budget.reserveMetadataStrings(0, selected)
		b.flow.OutputScopeID = selected
	}
	b.annotate(extractor.conditions)
	b.annotateLeaves()
	return b.flow
}

func (b *kqlDataflowBuilder) leadingSearch(ctx *SearchOperatorContext) {
	leaf := b.extractor.searchLeaves[ctx]
	if leaf == nil || leaf.Search == nil {
		return
	}
	info := dataflowOperatorInfo{PipeStage: leaf.Search.PipeStage, Pipeline: leaf.Search.Pipeline}
	occurrenceID, source, ok := b.owners.stageSource(ctx)
	if !ok {
		sourceAbort("leading search lacks authored occurrence")
	}
	occurrence := b.owners.source.Occurrences[occurrenceID]
	if index, exists := b.stageByOwner[occurrenceID]; exists {
		b.stageBySearch[ctx] = index
		b.stageByPosition[info] = index
		return
	}
	b.ownerPipeline[occurrence.OwnerID] = info.Pipeline
	b.emitPriorTombstones(occurrence.OwnerID, occurrence.Span.Start)
	input := b.inputScope(info.Pipeline)
	id := fmt.Sprintf("t%d", len(b.flow.Stages))
	b.budget.input(source)
	b.budget.reserveMetadataStrings(2, id, occurrence.Command, source, "filter", input)
	stage := DataflowStage{ID: id, Pipeline: info.Pipeline, PipeStage: info.PipeStage, Command: occurrence.Command, Source: source, Effect: "filter", Inputs: []ScopeReference{{ScopeID: input}}}
	if occurrence.Rewrite != "" {
		stage.Effect = "opaque"
		b.diagnostic(&stage, "unsupported_stage_dataflow", "rewritten search output")
	}
	mode := "inherit"
	if stage.Effect == "opaque" {
		mode = "opaque"
	}
	output := b.scope(info.Pipeline, mode, input, "", nil, nil)
	b.budget.reserveMetadataStrings(0, output)
	stage.OutputScopeID = output
	b.current[info.Pipeline] = stage.OutputScopeID
	b.stageByPosition[info] = len(b.flow.Stages)
	b.stageBySearch[ctx] = len(b.flow.Stages)
	b.stageByOwner[occurrenceID] = len(b.flow.Stages)
	b.flow.Stages = append(b.flow.Stages, stage)
	b.lastPipeStage[info.Pipeline] = info.PipeStage
}

func (b *kqlDataflowBuilder) attachLocalUnionInputs() {
	for stageIndex := range b.flow.Stages {
		b.budget.step()
		stage := &b.flow.Stages[stageIndex]
		if !strings.EqualFold(stage.Command, "union") {
			continue
		}
		for pipeline, legacy := range b.extractor.pipelines {
			b.budget.step()
			if legacy.Parent != stage.Pipeline || legacy.PipeStage != stage.PipeStage || legacy.Operator != "union" {
				continue
			}
			scope := b.inputScope(pipeline)
			b.budget.reserveMetadataStrings(1, scope)
			stage.Inputs = append(stage.Inputs, ScopeReference{ScopeID: scope})
		}
	}
}

func (b *kqlDataflowBuilder) inputScope(pipeline int) string {
	if id, ok := b.current[pipeline]; ok {
		return id
	}
	source := ""
	if pipeline >= 0 && pipeline < len(b.extractor.pipelines) {
		source = b.extractor.pipelines[pipeline].Source
	}
	if pipeline == b.extractor.queryPipeline && source == "DummyTable" {
		selected := b.owners.source.Occurrences[b.owners.source.SelectedOwnerID].Span
		authored := b.owners.source.Original[selected.Start:selected.End]
		if strings.HasPrefix(strings.TrimSpace(authored), "|") || isKQLLeadingOperator(authored, 0, len(authored)) {
			source = ""
		}
	}
	id := b.scope(pipeline, "input", "", source, nil, nil)
	b.current[pipeline] = id
	return id
}

func (b *kqlDataflowBuilder) scope(pipeline int, mode, parent, source string, bindingIDs, removed []string) string {
	b.budget.step()
	b.budget.input(source)
	id := fmt.Sprintf("s%d", len(b.flow.Scopes))
	b.budget.reserveMetadataStrings(1+len(bindingIDs)+len(removed), id, mode, parent, source)
	for _, value := range bindingIDs {
		b.budget.reserveMetadataStrings(0, value)
	}
	for _, value := range removed {
		b.budget.reserveMetadataStrings(0, value)
	}
	b.flow.Scopes = append(b.flow.Scopes, FieldScope{ID: id, Pipeline: pipeline, Mode: mode, ParentID: parent, Source: source, BindingIDs: append([]string(nil), bindingIDs...), RemovedFields: append([]string(nil), removed...)})
	return id
}

func (b *kqlDataflowBuilder) addBinding(stage *DataflowStage, field, kind, expression, readScope string) string {
	b.budget.step()
	b.budget.input(expression)
	id := fmt.Sprintf("b%d", len(b.flow.Bindings))
	binding := FieldBinding{ID: id, StageID: stage.ID, Ordinal: len(stage.BindingIDs), Field: field, Kind: kind, Expression: expression, InputScopeID: readScope}
	b.budget.reserveMetadataStrings(2, id, stage.ID, field, kind, expression, readScope, id)
	b.flow.Bindings = append(b.flow.Bindings, binding)
	stage.BindingIDs = append(stage.BindingIDs, id)
	return id
}

func (b *kqlDataflowBuilder) authoredIdentifier(ctx antlr.ParserRuleContext) string {
	return b.extractor.authoredIdentifier(ctx)
}

func (b *kqlDataflowBuilder) diagnostic(stage *DataflowStage, code, message string) {
	b.budget.step()
	b.budget.reserveMetadataStrings(1, code, stage.ID, message)
	b.flow.Complete = false
	b.flow.Diagnostics = append(b.flow.Diagnostics, DataflowDiagnostic{Code: code, StageID: stage.ID, Message: message})
}

func (b *kqlDataflowBuilder) operator(ctx *TabularOperatorContext) {
	info, ok := b.extractor.dataflowOperators[ctx]
	if !ok {
		return
	}
	occurrenceID, source, ok := b.owners.stageSource(ctx)
	if !ok {
		sourceAbort("operator lacks authored occurrence")
	}
	occurrence := b.owners.source.Occurrences[occurrenceID]
	if index, exists := b.stageByOwner[occurrenceID]; exists {
		b.stageByPosition[info] = index
		return
	}
	b.ownerPipeline[occurrence.OwnerID] = info.Pipeline
	b.emitPriorTombstones(occurrence.OwnerID, occurrence.Span.Start)
	input := b.inputScope(info.Pipeline)
	b.budget.reserveMetadataStrings(1, input)
	stage := DataflowStage{ID: fmt.Sprintf("t%d", len(b.flow.Stages)), Pipeline: info.Pipeline, PipeStage: info.PipeStage, Command: occurrence.Command, Source: source, Inputs: []ScopeReference{{ScopeID: input}}}
	if ctx.JoinOperator() != nil && ctx.JoinOperator().TabularExpression() != nil {
		for i := range b.extractor.joins {
			join := b.extractor.joins[i]
			if join.PipeStage != info.PipeStage {
				continue
			}
			if join.Subsearch == nil || join.Subsearch.Dataflow == nil || join.Subsearch.Dataflow.OutputScopeID == "" {
				sourceAbort("join child has no output scope")
			}
			path := "joins/" + strconv.Itoa(i)
			b.budget.reserveMetadataStrings(1, path, join.Subsearch.Dataflow.OutputScopeID)
			stage.Inputs = append(stage.Inputs, ScopeReference{QueryPath: path, ScopeID: join.Subsearch.Dataflow.OutputScopeID})
			break
		}
	}
	b.budget.input(source)
	if occurrence.Rewrite != "" {
		stage.Effect = "opaque"
		if occurrence.Rewrite == "normalizeSummarizeBy" || occurrence.Rewrite == "normalizeTopNested" {
			kept := b.extractor.aggregations[:0]
			keptContexts := b.extractor.aggregationContexts[:0]
			for i, aggregation := range b.extractor.aggregations {
				b.budget.step()
				if aggregation.PipeStage != info.PipeStage || aggregation.Pipeline != info.Pipeline {
					kept = append(kept, aggregation)
					keptContexts = append(keptContexts, b.extractor.aggregationContexts[i])
				}
			}
			b.extractor.aggregations = kept
			b.extractor.aggregationContexts = keptContexts
		}
	}
	switch {
	case stage.Effect == "opaque":
	case ctx.WhereOperator() != nil:
		stage.Effect = "filter"
	case ctx.SearchOperator() != nil:
		stage.Effect = "filter"
	case ctx.ExtendOperator() != nil:
		stage.Effect = "assign"
	case ctx.ProjectOperator() != nil:
		stage.Effect = "project"
	case ctx.ProjectRenameOperator() != nil:
		stage.Effect = "rename"
	case ctx.ProjectAwayOperator() != nil:
		stage.Effect = "project"
	case ctx.SummarizeOperator() != nil:
		stage.Effect = "aggregate"
	case isKQLRetainCommand(stage.Command):
		stage.Effect = "retain"
	default:
		stage.Effect = "opaque"
	}
	b.budget.reserveMetadataStrings(1, stage.ID, stage.Command, stage.Source, stage.Effect)
	stageIndex := len(b.flow.Stages)
	firstBinding := len(b.flow.Bindings)
	// Keep the record address stable while collecting its bindings.
	b.flow.Stages = append(b.flow.Stages, stage)
	stage = b.flow.Stages[stageIndex]
	mode := "inherit"
	var removed []string
	switch stage.Effect {
	case "assign":
		items := ctx.ExtendOperator().ExtendItemList()
		if items == nil {
			stage.Effect = "opaque"
			break
		}
		for _, item := range items.AllExtendItem() {
			b.budget.step()
			if item.Identifier() == nil || item.Expression() == nil {
				stage.Effect = "opaque"
				break
			}
			expression, ok := b.owners.rhsSource(item)
			if !ok {
				sourceAbort("extend RHS lacks authored occurrence")
			}
			b.addBinding(&stage, b.authoredIdentifier(item.Identifier()), "assignment", expression, input)
		}
	case "project":
		if projection := ctx.ProjectOperator(); projection != nil {
			mode = "closed"
			if projection.ProjectItemList() == nil {
				stage.Effect = "opaque"
				break
			}
			for _, item := range projection.ProjectItemList().AllProjectItem() {
				b.budget.step()
				if item.Expression() == nil {
					stage.Effect = "opaque"
					break
				}
				expression, ok := b.owners.rhsSource(item)
				if !ok {
					expression, ok = b.owners.authoredContext(item.Expression(), b.budget)
				}
				if !ok {
					sourceAbort("project expression lacks authored source")
				}
				if item.Identifier() != nil {
					b.addBinding(&stage, b.authoredIdentifier(item.Identifier()), "assignment", expression, input)
				} else if b.extractor.typed != nil && b.extractor.typed.hasOpaqueReference(item.Expression()) {
					stage.Effect = "opaque"
					break
				} else if text := item.Expression().GetText(); isValidFieldName(text) || decodeKQLIdentifier(text, b.budget) != text {
					b.addBinding(&stage, b.authoredIdentifier(item.Expression()), "passthrough", expression, input)
				} else {
					stage.Effect = "opaque"
					break
				}
			}
		} else if away := ctx.ProjectAwayOperator(); away != nil {
			if away.IdentifierOrWildcardList() == nil {
				stage.Effect = "opaque"
				break
			}
			for _, item := range away.IdentifierOrWildcardList().AllIdentifierOrWildcard() {
				name := item.GetText()
				if strings.ContainsAny(name, "*?") {
					stage.Effect = "opaque"
					break
				}
				removed = append(removed, b.authoredIdentifier(item))
			}
		}
	case "rename":
		if ctx.ProjectRenameOperator().RenameList() == nil {
			stage.Effect = "opaque"
			break
		}
		seen := map[string]bool{}
		for _, item := range ctx.ProjectRenameOperator().RenameList().AllRenameItem() {
			ids := item.AllIdentifier()
			if len(ids) != 2 {
				stage.Effect = "opaque"
				break
			}
			newName, oldName := b.authoredIdentifier(ids[0]), b.authoredIdentifier(ids[1])
			if seen[newName] || seen[oldName] || newName == oldName {
				stage.Effect = "opaque"
				break
			}
			seen[newName] = true
			seen[oldName] = true
			oldSource, ok := b.owners.authoredContext(ids[1], b.budget)
			if !ok {
				sourceAbort("rename source lacks authored span")
			}
			b.addBinding(&stage, newName, "alias", oldSource, input)
			removed = append(removed, oldName)
		}
	case "aggregate":
		mode = "closed"
		summary := ctx.SummarizeOperator()
		if summary.AggregationList() == nil {
			stage.Effect = "opaque"
			break
		}
		for _, item := range summary.AggregationList().AllAggregationItem() {
			alias := ""
			if item.Identifier() != nil {
				alias = b.authoredIdentifier(item.Identifier())
			}
			agg := newAggregation(item.AggregationFunction(), alias, info.PipeStage)
			expression, ok := b.owners.authoredContext(item.AggregationFunction(), b.budget)
			if !ok {
				sourceAbort("aggregate expression lacks authored span")
			}
			bindingID := b.addBinding(&stage, agg.Column, "aggregate", expression, input)
			paired := false
			for i, owner := range b.extractor.aggregationContexts {
				b.budget.step()
				if owner != item {
					continue
				}
				candidate := &b.extractor.aggregations[i]
				if candidate.PipeStage != info.PipeStage || candidate.Pipeline != info.Pipeline || candidate.Column != agg.Column || candidate.BindingID != "" {
					sourceAbort("aggregate occurrence owner is invalid")
				}
				b.budget.reserveMetadataStrings(0, stage.ID, bindingID)
				candidate.StageID, candidate.BindingID = stage.ID, bindingID
				paired = true
				break
			}
			if !paired {
				sourceAbort("represented aggregate lacks structured occurrence")
			}
		}
		if groups := summary.GroupByList(); groups != nil {
			for _, item := range groups.AllGroupByItem() {
				if item.Expression() == nil {
					stage.Effect = "opaque"
					break
				}
				if b.extractor.typed != nil && b.extractor.typed.hasOpaqueReference(item.Expression()) {
					stage.Effect = "opaque"
					break
				}
				expression, ok := b.owners.authoredContext(item.Expression(), b.budget)
				if !ok {
					sourceAbort("group expression lacks authored span")
				}
				field := item.Expression().GetText()
				if item.Identifier() != nil {
					field = b.authoredIdentifier(item.Identifier())
				} else if !isValidFieldName(field) && decodeKQLIdentifier(field, b.budget) == field {
					stage.Effect = "opaque"
					break
				} else {
					field = b.authoredIdentifier(item.Expression())
				}
				b.addBinding(&stage, field, "group", expression, input)
			}
		}
	}
	if stage.Effect == "opaque" {
		for i := range b.extractor.aggregations {
			if b.extractor.aggregations[i].StageID == stage.ID {
				b.extractor.aggregations[i].StageID = ""
				b.extractor.aggregations[i].BindingID = ""
			}
		}
		mode = "opaque"
		stage.BindingIDs = nil
		b.flow.Bindings = b.flow.Bindings[:firstBinding]
		b.diagnostic(&stage, "unsupported_stage_dataflow", "unrepresented "+stage.Command+" output")
	} else if stage.Effect == "project" && mode == "closed" || stage.Effect == "aggregate" {
		mode = "closed"
	}
	output := b.scope(info.Pipeline, mode, input, "", stage.BindingIDs, removed)
	b.budget.reserveMetadataStrings(0, output)
	stage.OutputScopeID = output
	b.current[info.Pipeline] = output
	b.flow.Stages[stageIndex] = stage
	b.stageByPosition[info] = stageIndex
	b.stageByOwner[occurrenceID] = stageIndex
	b.lastPipeStage[info.Pipeline] = info.PipeStage
}

func (b *kqlDataflowBuilder) emitPriorTombstones(ownerID, before int) {
	pipeline, ok := b.ownerPipeline[ownerID]
	if !ok {
		return
	}
	for _, occurrence := range b.owners.source.Occurrences {
		b.budget.step()
		if occurrence.Kind != "stage" || occurrence.OwnerID != ownerID || !occurrence.Removed || occurrence.Span.Start >= before || b.tombstones[occurrence.ID] {
			continue
		}
		input := b.inputScope(pipeline)
		b.budget.input(b.owners.source.Original[occurrence.Span.Start:occurrence.Span.End])
		id := fmt.Sprintf("t%d", len(b.flow.Stages))
		text := b.owners.source.Original[occurrence.Span.Start:occurrence.Span.End]
		b.budget.reserveMetadataStrings(2, id, occurrence.Command, text, "opaque", input)
		stage := DataflowStage{ID: id, Pipeline: pipeline, PipeStage: b.lastPipeStage[pipeline], Command: occurrence.Command, Source: text, Effect: "opaque", Inputs: []ScopeReference{{ScopeID: input}}}
		output := b.scope(pipeline, "opaque", input, "", nil, nil)
		b.budget.reserveMetadataStrings(0, output)
		stage.OutputScopeID = output
		b.flow.Stages = append(b.flow.Stages, stage)
		b.stageByOwner[occurrence.ID] = len(b.flow.Stages) - 1
		b.current[pipeline] = stage.OutputScopeID
		b.tombstones[occurrence.ID] = true
		b.diagnostic(&stage, "unsupported_stage_dataflow", "removed "+occurrence.Command+" stage has unrepresented output")
	}
}

func isKQLRetainCommand(command string) bool {
	switch strings.ToLower(command) {
	case "sort", "order", "take", "limit", "top":
		return true
	default:
		return false
	}
}

func (b *kqlDataflowBuilder) lookupValue(scopeID, field string) FieldBindingRef {
	ref := FieldBindingRef{ScopeID: scopeID, Kind: "unknown"}
	for current := scopeID; current != ""; {
		b.budget.step()
		index, err := strconv.Atoi(strings.TrimPrefix(current, "s"))
		if err != nil || index < 0 || index >= len(b.flow.Scopes) {
			sourceAbort("dangling scope during lookup")
		}
		scope := b.flow.Scopes[index]
		for _, id := range scope.BindingIDs {
			b.budget.step()
			index, err := strconv.Atoi(strings.TrimPrefix(id, "b"))
			if err != nil || index < 0 || index >= len(b.flow.Bindings) {
				sourceAbort("dangling binding during lookup")
			}
			if b.flow.Bindings[index].Field == field {
				ref.Kind = "binding"
				ref.BindingID = id
				return ref
			}
		}
		for _, removed := range scope.RemovedFields {
			b.budget.step()
			if removed == field {
				ref.Kind = "absent"
				return ref
			}
		}
		switch scope.Mode {
		case "input":
			ref.Kind = "input"
			return ref
		case "closed":
			ref.Kind = "absent"
			return ref
		case "opaque":
			return ref
		case "inherit":
			current = scope.ParentID
		default:
			sourceAbort("invalid scope mode")
		}
	}
	return ref
}

func (b *kqlDataflowBuilder) lookup(scopeID, field string) *FieldBindingRef {
	ref := b.lookupValue(scopeID, field)
	b.budget.reserveMetadataStrings(1, ref.ScopeID, ref.Kind, ref.BindingID)
	return &ref
}

func (b *kqlDataflowBuilder) annotate(conditions []Condition) {
	for i := range conditions {
		b.budget.step()
		condition := &conditions[i]
		stageIndex, ok := b.stageByPosition[dataflowOperatorInfo{PipeStage: condition.PipeStage, Pipeline: condition.Pipeline}]
		if !ok {
			continue
		}
		stage := b.flow.Stages[stageIndex]
		if len(stage.Inputs) == 0 {
			continue
		}
		b.budget.reserveMetadataStrings(0, stage.ID, stage.Inputs[0].ScopeID)
		condition.StageID, condition.InputScopeID = stage.ID, stage.Inputs[0].ScopeID
		condition.FieldBinding = b.lookup(condition.InputScopeID, condition.Field)
		b.annotateOperand(condition.RHS, condition.InputScopeID, &stage)
		if condition.FieldBinding.Kind == "binding" {
			index, _ := strconv.Atoi(strings.TrimPrefix(condition.FieldBinding.BindingID, "b"))
			binding := b.flow.Bindings[index]
			condition.IsComputed = binding.Kind != "passthrough" || b.lookupValue(binding.InputScopeID, binding.Field).Kind == "binding"
			if condition.IsComputed {
				condition.SourceField = sourceFieldForBinding(binding.Expression)
			} else {
				condition.SourceField = ""
			}
		} else {
			condition.IsComputed = false
			condition.SourceField = ""
		}
		if condition.FieldBinding.Kind == "unknown" || condition.FieldBinding.Kind == "absent" {
			code := "unresolved_field_binding"
			if condition.FieldBinding.Kind == "absent" {
				code = "field_not_in_scope"
			}
			b.diagnostic(&stage, code, "field "+condition.Field+" cannot be resolved in "+condition.InputScopeID)
		}
	}
}

func sourceFieldForBinding(expression string) string {
	expression = strings.TrimSpace(expression)
	if decoded := decodeKQLIdentifier(expression); decoded != expression {
		return decoded
	}
	if len(expression) > 1 && expression[0] >= '0' && expression[0] <= '9' {
		allName, hasLetter := true, false
		for i := 1; i < len(expression); i++ {
			if !isIdentChar(expression[i]) {
				allName = false
				break
			}
			if isIdentStart(expression[i]) {
				hasLetter = true
			}
		}
		if allName && hasLetter {
			return expression
		}
	}
	return firstFieldInExpressionText(expression)
}

func (b *kqlDataflowBuilder) annotateOperand(operand *Operand, scopeID string, stage *DataflowStage) {
	b.budget.step()
	if operand == nil {
		return
	}
	if operand.Kind == OperandField {
		operand.FieldBinding = b.lookup(scopeID, operand.Value)
		if operand.FieldBinding.Kind == "unknown" || operand.FieldBinding.Kind == "absent" {
			code := "unresolved_field_binding"
			if operand.FieldBinding.Kind == "absent" {
				code = "field_not_in_scope"
			}
			b.diagnostic(stage, code, "field "+operand.Value+" cannot be resolved in "+scopeID)
		}
	}
	for i := range operand.Items {
		b.annotateOperand(&operand.Items[i], scopeID, stage)
	}
}

func (b *kqlDataflowBuilder) annotateLeaves() {
	findStage := func(ctx antlr.ParserRuleContext) (DataflowStage, bool) {
		for parent := ctx.GetParent(); parent != nil; parent = parent.GetParent() {
			b.budget.step()
			operator, ok := parent.(*TabularOperatorContext)
			if !ok {
				continue
			}
			info, ok := b.extractor.dataflowOperators[operator]
			if !ok {
				break
			}
			index, ok := b.stageByPosition[info]
			if !ok {
				break
			}
			return b.flow.Stages[index], true
		}
		return DataflowStage{}, false
	}
	for ctx, leaf := range b.extractor.expressionLeaves {
		b.budget.step()
		if leaf == nil || leaf.Scalar == nil {
			continue
		}
		stage, ok := findStage(ctx)
		if !ok || len(stage.Inputs) == 0 {
			continue
		}
		b.budget.reserveMetadataStrings(0, stage.ID, stage.Inputs[0].ScopeID)
		leaf.Scalar.StageID, leaf.Scalar.InputScopeID = stage.ID, stage.Inputs[0].ScopeID
	}
	for ctx, leaf := range b.extractor.searchLeaves {
		b.budget.step()
		if leaf == nil || leaf.Search == nil {
			continue
		}
		stage, ok := findStage(ctx)
		if !ok {
			if index, found := b.stageBySearch[ctx]; found {
				stage, ok = b.flow.Stages[index], true
			}
		}
		if !ok || len(stage.Inputs) == 0 {
			continue
		}
		b.budget.reserveMetadataStrings(0, stage.ID, stage.Inputs[0].ScopeID)
		leaf.Search.StageID, leaf.Search.InputScopeID = stage.ID, stage.Inputs[0].ScopeID
	}
}

func fieldBindingIdentity(ref *FieldBindingRef) string {
	if ref == nil {
		return "0"
	}
	field := func(s string) string { return strconv.Itoa(len(s)) + ":" + s }
	return "1" + field(ref.ScopeID) + field(ref.Kind) + field(ref.BindingID)
}

func cloneFieldBindingRef(ref *FieldBindingRef, budget *parseBudget) *FieldBindingRef {
	budget.step()
	if ref == nil {
		return nil
	}
	budget.reserveMetadataStrings(1, ref.ScopeID, ref.Kind, ref.BindingID)
	copy := *ref
	return &copy
}

func cloneQueryDataflow(flow *QueryDataflow, budget *parseBudget) *QueryDataflow {
	budget.step()
	if flow == nil {
		return nil
	}
	budget.reserveMetadataStrings(1, flow.OutputScopeID)
	copy := &QueryDataflow{Complete: flow.Complete, OutputScopeID: flow.OutputScopeID}
	if len(flow.Scopes) > 0 {
		budget.reserveMetadata(len(flow.Scopes), 0)
		copy.Scopes = make([]FieldScope, len(flow.Scopes))
		for i, scope := range flow.Scopes {
			budget.step()
			budget.input(scope.Source)
			budget.reserveMetadataStrings(0, scope.ID, scope.ParentID, scope.Mode, scope.Source)
			copy.Scopes[i] = scope
			copy.Scopes[i].BindingIDs = cloneDataflowStrings(scope.BindingIDs, budget)
			copy.Scopes[i].RemovedFields = cloneDataflowStrings(scope.RemovedFields, budget)
		}
	}
	if len(flow.Stages) > 0 {
		budget.reserveMetadata(len(flow.Stages), 0)
		copy.Stages = make([]DataflowStage, len(flow.Stages))
		for i, stage := range flow.Stages {
			budget.step()
			budget.input(stage.Source)
			budget.reserveMetadataStrings(0, stage.ID, stage.Command, stage.Source, stage.Effect, stage.OutputScopeID)
			copy.Stages[i] = stage
			copy.Stages[i].BindingIDs = cloneDataflowStrings(stage.BindingIDs, budget)
			if len(stage.Inputs) > 0 {
				budget.reserveMetadata(len(stage.Inputs), 0)
				for _, input := range stage.Inputs {
					budget.step()
					budget.reserveMetadataStrings(0, input.QueryPath, input.ScopeID)
				}
				copy.Stages[i].Inputs = append([]ScopeReference(nil), stage.Inputs...)
			}
		}
	}
	if len(flow.Bindings) > 0 {
		budget.reserveMetadata(len(flow.Bindings), 0)
		for _, binding := range flow.Bindings {
			budget.step()
			budget.input(binding.Expression)
			budget.reserveMetadataStrings(0, binding.ID, binding.StageID, binding.Field, binding.Kind, binding.Expression, binding.InputScopeID)
		}
		copy.Bindings = append([]FieldBinding(nil), flow.Bindings...)
	}
	if len(flow.Diagnostics) > 0 {
		budget.reserveMetadata(len(flow.Diagnostics), 0)
		for _, diagnostic := range flow.Diagnostics {
			budget.step()
			budget.reserveMetadataStrings(0, diagnostic.Code, diagnostic.StageID, diagnostic.Message)
		}
		copy.Diagnostics = append([]DataflowDiagnostic(nil), flow.Diagnostics...)
	}
	return copy
}

func cloneDataflowStrings(values []string, budget *parseBudget) []string {
	budget.step()
	if len(values) == 0 {
		return nil
	}
	budget.reserveMetadata(len(values), 0)
	for _, value := range values {
		budget.step()
		budget.reserveMetadataStrings(0, value)
	}
	return append([]string(nil), values...)
}

// finalizeDataflow checks the published graph after recursive child results
// exist. An invalid link is an extraction failure, never an incomplete graph.
func finalizeDataflow(result *ParseResult, budget *parseBudget) {
	if result == nil || result.Dataflow == nil {
		return
	}
	seenResults := map[*ParseResult]bool{}
	var check func(*ParseResult)
	check = func(current *ParseResult) {
		budget.step()
		if current == nil || current.Dataflow == nil || seenResults[current] {
			sourceAbort("invalid or cyclic child dataflow")
		}
		seenResults[current] = true
		flow := current.Dataflow
		scopes := make(map[string]FieldScope, len(flow.Scopes))
		stages := make(map[string]DataflowStage, len(flow.Stages))
		aggregateStages := make(map[dataflowOperatorInfo]string)
		bindings := make(map[string]FieldBinding, len(flow.Bindings))
		for _, scope := range flow.Scopes {
			budget.step()
			if scope.ID == "" || scopes[scope.ID].ID != "" || scope.Mode != "input" && scope.Mode != "inherit" && scope.Mode != "closed" && scope.Mode != "opaque" {
				sourceAbort("invalid or duplicate field scope")
			}
			scopes[scope.ID] = scope
		}
		if _, ok := scopes[flow.OutputScopeID]; !ok {
			sourceAbort("missing selected output scope")
		}
		for _, stage := range flow.Stages {
			budget.step()
			if stage.ID == "" || stages[stage.ID].ID != "" {
				sourceAbort("invalid or duplicate stage")
			}
			if _, ok := scopes[stage.OutputScopeID]; !ok {
				sourceAbort("stage output scope is missing")
			}
			stages[stage.ID] = stage
			if stage.Effect == "aggregate" {
				aggregateStages[dataflowOperatorInfo{PipeStage: stage.PipeStage, Pipeline: stage.Pipeline}] = stage.ID
			}
		}
		for _, binding := range flow.Bindings {
			budget.step()
			if binding.ID == "" || bindings[binding.ID].ID != "" {
				sourceAbort("invalid or duplicate binding")
			}
			stage, ok := stages[binding.StageID]
			if !ok {
				sourceAbort("binding stage is missing")
			}
			if _, ok := scopes[binding.InputScopeID]; !ok {
				sourceAbort("binding input scope is missing")
			}
			if len(stage.Inputs) == 0 || stage.Inputs[0].QueryPath != "" || binding.InputScopeID != stage.Inputs[0].ScopeID || binding.InputScopeID == stage.OutputScopeID {
				sourceAbort("binding read scope is not its stage input")
			}
			bindings[binding.ID] = binding
		}
		checkReference := func(ref *FieldBindingRef) {
			budget.step()
			if ref == nil {
				return
			}
			if _, ok := scopes[ref.ScopeID]; !ok {
				sourceAbort("field reference scope is missing")
			}
			switch ref.Kind {
			case "binding":
				if _, ok := bindings[ref.BindingID]; !ok || ref.BindingID == "" {
					sourceAbort("field reference binding is missing")
				}
			case "input", "absent", "unknown":
				if ref.BindingID != "" {
					sourceAbort("unbound field reference has binding ID")
				}
			default:
				sourceAbort("invalid field reference kind")
			}
		}
		var checkOperand func(*Operand)
		checkOperand = func(operand *Operand) {
			budget.step()
			if operand == nil {
				return
			}
			checkReference(operand.FieldBinding)
			for i := range operand.Items {
				checkOperand(&operand.Items[i])
			}
		}
		for i := range current.Conditions {
			budget.step()
			condition := &current.Conditions[i]
			if condition.StageID != "" {
				stage, ok := stages[condition.StageID]
				if !ok || len(stage.Inputs) == 0 || stage.Inputs[0].ScopeID != condition.InputScopeID {
					sourceAbort("condition stage or read scope is invalid")
				}
			}
			checkReference(condition.FieldBinding)
			checkOperand(condition.RHS)
		}
		checkPayloadOwner := func(stageID, readScope string) {
			budget.step()
			stage, ok := stages[stageID]
			if !ok || stageID == "" || len(stage.Inputs) == 0 || stage.Inputs[0].ScopeID != readScope || stage.Inputs[0].QueryPath != "" {
				sourceAbort("predicate stage or read scope is invalid")
			}
		}
		var checkExpression func(*ConditionExpression)
		checkExpression = func(expression *ConditionExpression) {
			budget.step()
			if expression == nil {
				return
			}
			if expression.Scalar != nil {
				checkPayloadOwner(expression.Scalar.StageID, expression.Scalar.InputScopeID)
			}
			if expression.Search != nil {
				checkPayloadOwner(expression.Search.StageID, expression.Search.InputScopeID)
			}
			for _, child := range expression.Children {
				checkExpression(child)
			}
		}
		checkExpression(current.Expression)
		for _, aggregate := range current.Aggregations {
			budget.step()
			expectedStage := aggregateStages[dataflowOperatorInfo{PipeStage: aggregate.PipeStage, Pipeline: aggregate.Pipeline}]
			if aggregate.StageID == "" && aggregate.BindingID == "" {
				if expectedStage != "" {
					sourceAbort("represented aggregate lacks binding owner")
				}
				continue
			}
			binding, ok := bindings[aggregate.BindingID]
			if !ok || binding.Kind != "aggregate" || expectedStage != aggregate.StageID || binding.StageID != aggregate.StageID || binding.Field != aggregate.Column {
				sourceAbort("aggregate binding owner is invalid")
			}
		}
		for _, scope := range flow.Scopes {
			budget.step()
			if scope.Mode == "input" && scope.ParentID != "" || scope.Mode == "inherit" && scope.ParentID == "" {
				sourceAbort("invalid scope parent mode")
			}
			if scope.ParentID != "" {
				if _, ok := scopes[scope.ParentID]; !ok {
					sourceAbort("scope parent is missing")
				}
			}
			for _, id := range scope.BindingIDs {
				budget.step()
				if _, ok := bindings[id]; !ok {
					sourceAbort("scope binding is missing")
				}
			}
		}
		for _, stage := range flow.Stages {
			budget.step()
			for _, id := range stage.BindingIDs {
				budget.step()
				if binding, ok := bindings[id]; !ok || binding.StageID != stage.ID {
					sourceAbort("stage binding owner is invalid")
				}
			}
			for _, input := range stage.Inputs {
				budget.step()
				if input.QueryPath == "" {
					if _, ok := scopes[input.ScopeID]; !ok || input.ScopeID == stage.OutputScopeID {
						sourceAbort("local stage input is invalid")
					}
					continue
				}
				target := current
				parts := strings.Split(input.QueryPath, "/")
				if len(parts)%2 != 0 {
					sourceAbort("invalid child query path")
				}
				for i := 0; i < len(parts); i += 2 {
					budget.step()
					index, err := strconv.Atoi(parts[i+1])
					if err != nil || index < 0 || parts[i] != "joins" || index >= len(target.Joins) || target.Joins[index].Subsearch == nil {
						sourceAbort("unresolved child query path")
					}
					target = target.Joins[index].Subsearch
				}
				if target.Dataflow == nil || target.Dataflow.OutputScopeID != input.ScopeID {
					sourceAbort("child output scope is missing")
				}
			}
		}
		// Parent links and local stage input edges together must form a DAG.
		edges := make(map[string][]string, len(scopes))
		for _, scope := range flow.Scopes {
			budget.step()
			if scope.ParentID != "" {
				edges[scope.ID] = append(edges[scope.ID], scope.ParentID)
			}
		}
		for _, stage := range flow.Stages {
			budget.step()
			for _, input := range stage.Inputs {
				if input.QueryPath == "" {
					edges[stage.OutputScopeID] = append(edges[stage.OutputScopeID], input.ScopeID)
				}
			}
		}
		for _, binding := range flow.Bindings {
			budget.step()
			stage := stages[binding.StageID]
			edges[stage.OutputScopeID] = append(edges[stage.OutputScopeID], binding.InputScopeID)
		}
		color := make(map[string]uint8, len(scopes))
		var visit func(string)
		visit = func(id string) {
			budget.step()
			if color[id] == 1 {
				sourceAbort("cyclic local scope lineage")
			}
			if color[id] == 2 {
				return
			}
			color[id] = 1
			for _, parent := range edges[id] {
				visit(parent)
			}
			color[id] = 2
		}
		for id := range scopes {
			visit(id)
		}
		for _, join := range current.Joins {
			if join.Subsearch != nil {
				check(join.Subsearch)
			}
		}
		delete(seenResults, current)
	}
	check(result)
}
