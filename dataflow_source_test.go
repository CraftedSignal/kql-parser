package kql

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"
)

func TestKQLSourceEditsKeepRepeatedOwnersAndUnicode(t *testing.T) {
	query := `T | extend x="é" | extend x="é"`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	src := newKQLMappedSource(query, budget)
	if len(src.Occurrences) != 5 { // query, two stages, two RHS
		t.Fatalf("occurrences: %#v", src.Occurrences)
	}
	first := src.Occurrences[1]
	second := src.Occurrences[3]
	if first.Span == second.Span || query[first.Span.Start:first.Span.End] != `extend x="é"` || query[second.Span.Start:second.Span.End] != `extend x="é"` {
		t.Fatalf("repeated stage owners: %#v", src.Occurrences)
	}
	// A replacement of only the first non-ASCII expression must not shift or
	// relabel the later identical stage.
	rhs := src.Occurrences[2]
	src = applyKQLSourceEdits(src, []kqlSourceEdit{{Start: rhs.Span.Start, End: rhs.Span.End, Parts: []kqlSourcePart{{Text: `"e"`, Origin: rhs.Span, OccurrenceID: rhs.ID}}}}, budget)
	if src.Text != `T | extend x="e" | extend x="é"` {
		t.Fatalf("mapped text: %q", src.Text)
	}
	if src.Occurrences[3].Span != second.Span {
		t.Fatalf("second stage origin changed: %#v", src.Occurrences[3])
	}
}

func TestKQLSourceAssociationUsesTokenOrigin(t *testing.T) {
	query := `T | extend x="é" | extend x="é"`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	src := newKQLMappedSource(query, budget)
	lexer := NewKQLLexer(antlr.NewInputStream(query))
	parser := NewKQLParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	owners := associateKQLSource(parser.Query(), src, budget)
	if len(owners.Stages) != 2 {
		t.Fatalf("associated stages: %d", len(owners.Stages))
	}
	seen := map[int]bool{}
	for ctx := range owners.Stages {
		id, source, ok := owners.stageSource(ctx)
		if !ok || source != `extend x="é"` || seen[id] {
			t.Fatalf("owner %d, source %q, ok %v", id, source, ok)
		}
		seen[id] = true
	}
}

func TestKQLNestedSourceOwnerStaysInsideJoin(t *testing.T) {
	query := `T | extend x=1 | join (U | extend x=1 | where x==1) on Key | where x==1`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	src := newKQLMappedSource(query, budget)
	var parent, child kqlSourceOccurrence
	for _, occurrence := range src.Occurrences {
		if occurrence.Kind != "stage" || occurrence.Command != "extend" {
			continue
		}
		if parent.ID == 0 {
			parent = occurrence
		} else {
			child = occurrence
		}
	}
	if parent.ID == 0 || child.ID == 0 || parent.OwnerID == child.OwnerID || query[parent.Span.Start:parent.Span.End] != `extend x=1` || query[child.Span.Start:child.Span.End] != `extend x=1` {
		t.Fatalf("parent/child occurrence leakage: %#v", src.Occurrences)
	}
}

func TestKQLChildSourceRebasesAuthoredJoin(t *testing.T) {
	query := `T | join (U | extend x=1 | where x==1) on Key`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	src := newKQLMappedSource(query, budget)
	lexer := NewKQLLexer(antlr.NewInputStream(query))
	parser := NewKQLParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	tree := parser.Query()
	owners := associateKQLSource(tree, src, budget)
	var joined *JoinOperatorContext
	var visit func(antlr.Tree)
	visit = func(node antlr.Tree) {
		if ctx, ok := node.(*JoinOperatorContext); ok {
			joined = ctx
			return
		}
		for i := 0; i < node.GetChildCount(); i++ {
			visit(node.GetChild(i))
		}
	}
	visit(tree)
	if joined == nil {
		t.Fatal("join context missing")
	}
	child, ok := owners.childSource(joined.TabularExpression(), budget)
	if !ok || child.Authored != `U | extend x=1 | where x==1` || child.Mapped.Text != child.Authored {
		t.Fatalf("child source: %#v, ok=%v", child, ok)
	}
	if len(child.Mapped.Occurrences) != 4 || child.Mapped.Occurrences[1].OwnerID != 0 {
		t.Fatalf("child occurrences: %#v", child.Mapped.Occurrences)
	}
}

func TestKQLFinalizationRejectsDanglingAndCyclicScope(t *testing.T) {
	for _, test := range []struct {
		name string
		flow QueryDataflow
	}{
		{"dangling", QueryDataflow{OutputScopeID: "s0", Scopes: []FieldScope{{ID: "s0", Mode: "inherit", ParentID: "missing"}}}},
		{"cycle", QueryDataflow{OutputScopeID: "s0", Scopes: []FieldScope{{ID: "s0", Mode: "inherit", ParentID: "s1"}, {ID: "s1", Mode: "inherit", ParentID: "s0"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
			defer cancel()
			defer func() {
				if recover() == nil {
					t.Fatal("invalid graph was published")
				}
			}()
			finalizeDataflow(&ParseResult{Dataflow: &test.flow}, budget)
		})
	}
}

func TestKQLInventoryRetainsUnnamedAndRenameRHS(t *testing.T) {
	query := `T | project 3plogTime | project-rename Fresh=Original | summarize n=count() by ['Fresh Name']`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	source := newKQLMappedSource(query, budget)
	var got []string
	for _, occurrence := range source.Occurrences {
		if occurrence.Kind == "rhs" {
			got = append(got, query[occurrence.Span.Start:occurrence.Span.End])
		}
	}
	want := []string{"3plogTime", "Original", "count()", "['Fresh Name']"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RHS inventory got %q, want %q", got, want)
	}
}

func TestKQLProjectAsAliasInventoryOwnsOnlyExpression(t *testing.T) {
	query := `T | project x as y | where y=="a"`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	source := newKQLMappedSource(query, budget)
	var rhs []string
	for _, occurrence := range source.Occurrences {
		if occurrence.Kind == "rhs" {
			rhs = append(rhs, query[occurrence.Span.Start:occurrence.Span.End])
		}
	}
	if !reflect.DeepEqual(rhs, []string{"x"}) {
		t.Fatalf("project alias RHS inventory includes alias: %q", rhs)
	}
}

func TestKQLDottedAsSegmentInventoryOwnsWholeExpression(t *testing.T) {
	query := `T | project obj.as.value | where obj.as.value=="a"`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	source := newKQLMappedSource(query, budget)
	var rhs []string
	for _, occurrence := range source.Occurrences {
		if occurrence.Kind == "rhs" {
			rhs = append(rhs, query[occurrence.Span.Start:occurrence.Span.End])
		}
	}
	if !reflect.DeepEqual(rhs, []string{"obj.as.value"}) {
		t.Fatalf("dotted project RHS inventory lost member access: %q", rhs)
	}
}

func TestKQLNumericDotProjectAliasInventoryOwnsOnlyExpression(t *testing.T) {
	query := `T | project 1. as y | where y==1`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	source := newKQLMappedSource(query, budget)
	var rhs []string
	for _, occurrence := range source.Occurrences {
		if occurrence.Kind == "rhs" {
			rhs = append(rhs, query[occurrence.Span.Start:occurrence.Span.End])
		}
	}
	if !reflect.DeepEqual(rhs, []string{"1."}) {
		t.Fatalf("numeric project alias RHS includes alias: %q", rhs)
	}
}

func TestKQLProjectAsAliasCommentsInventoryOwnsOnlyExpression(t *testing.T) {
	for _, tc := range []struct {
		name, query, expression string
	}{
		{"after alias", `T | project x as/*c*/y | where y=="a"`, "x"},
		{"before alias", `T | project x/*c*/as y | where y=="a"`, "x/*c*/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
			defer cancel()
			source := newKQLMappedSource(tc.query, budget)
			var rhs []string
			for _, occurrence := range source.Occurrences {
				if occurrence.Kind == "rhs" {
					rhs = append(rhs, tc.query[occurrence.Span.Start:occurrence.Span.End])
				}
			}
			if !reflect.DeepEqual(rhs, []string{tc.expression}) {
				t.Fatalf("project alias RHS inventory includes alias: %q, want %q", rhs, tc.expression)
			}
		})
	}
}

func TestKQLFinalizationRejectsSelfReadAndUnownedPayload(t *testing.T) {
	base := QueryDataflow{OutputScopeID: "s1", Scopes: []FieldScope{{ID: "s0", Mode: "input"}, {ID: "s1", Mode: "inherit", ParentID: "s0", BindingIDs: []string{"b0"}}}, Stages: []DataflowStage{{ID: "t0", Command: "extend", Effect: "assign", Inputs: []ScopeReference{{ScopeID: "s0"}}, OutputScopeID: "s1", BindingIDs: []string{"b0"}}}, Bindings: []FieldBinding{{ID: "b0", StageID: "t0", Field: "x", Kind: "assignment", Expression: "y", InputScopeID: "s1"}}}
	for _, tc := range []struct {
		name   string
		result ParseResult
	}{
		{"self read", ParseResult{Dataflow: &base}},
		{"unowned scalar", ParseResult{Dataflow: &QueryDataflow{OutputScopeID: "s0", Scopes: []FieldScope{{ID: "s0", Mode: "input"}}}, Expression: &ConditionExpression{Operator: BooleanScalar, Scalar: &ScalarPredicate{Text: "x > 0", StageID: "missing", InputScopeID: "s0"}}}},
		{"unowned search", ParseResult{Dataflow: &QueryDataflow{OutputScopeID: "s0", Scopes: []FieldScope{{ID: "s0", Mode: "input"}}}, Expression: &ConditionExpression{Operator: BooleanSearch, Search: &SearchPredicate{Text: "search x", StageID: "missing", InputScopeID: "s0"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
			defer cancel()
			defer func() {
				if recover() == nil {
					t.Fatal("invalid dataflow was accepted")
				}
			}()
			finalizeDataflow(&tc.result, budget)
		})
	}
}

func TestKQLTinyBudgetStopsInventoryAndRecursiveGraphClone(t *testing.T) {
	for _, tc := range []struct {
		name string
		cap  int
		run  func(*parseBudget)
	}{
		{"occurrence growth", 1, func(b *parseBudget) { newKQLMappedSource(`T | where x==1`, b) }},
		{"graph allocation", 0, func(b *parseBudget) { buildKQLDataflow(nil, nil, nil, b) }},
		{"graph clone", 1, func(b *parseBudget) {
			cloneQueryDataflow(&QueryDataflow{OutputScopeID: "s0", Scopes: []FieldScope{{ID: "s0", Mode: "input", Source: "T"}}, Stages: []DataflowStage{{ID: "t0", Command: "where", Effect: "filter", Inputs: []ScopeReference{{ScopeID: "s0"}}, OutputScopeID: "s0"}}}, b)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := defaultParseLimits()
			limits.maxMetadataItems = tc.cap
			budget, cancel := newParseBudget(context.Background(), limits)
			defer cancel()
			defer func() {
				if got, ok := recover().(budgetAbort); !ok || got.err == nil {
					t.Fatalf("expected budget failure before growth, got %v", got)
				}
			}()
			tc.run(budget)
		})
	}
}

func TestKQLFixedBudgetReachesRecursiveSemanticGraphClone(t *testing.T) {
	child := &ParseResult{Dataflow: &QueryDataflow{Complete: true, OutputScopeID: "s1", Scopes: []FieldScope{{ID: "s0", Mode: "input", Source: "U"}, {ID: "s1", Mode: "inherit", ParentID: "s0"}}, Stages: []DataflowStage{{ID: "t0", Command: "where", Effect: "filter", Source: "where x==1", Inputs: []ScopeReference{{ScopeID: "s0"}}, OutputScopeID: "s1"}}}}
	root := &ParseResult{Dataflow: &QueryDataflow{Complete: true, OutputScopeID: "s1", Scopes: []FieldScope{{ID: "s0", Mode: "input", Source: "T"}, {ID: "s1", Mode: "inherit", ParentID: "s0"}}, Stages: []DataflowStage{{ID: "t0", Command: "join", Effect: "retain", Source: "join (U)", Inputs: []ScopeReference{{ScopeID: "s0"}, {QueryPath: "joins/0", ScopeID: "s1"}}, OutputScopeID: "s1"}}}, Joins: []JoinInfo{{Subsearch: child}}}
	limits := defaultParseLimits()
	limits.maxMetadataBytes = 80
	rootOnly := *root
	rootOnly.Joins = nil
	first, cancelFirst := newParseBudget(context.Background(), limits)
	defer cancelFirst()
	if semantic := semanticFromExtraction(&rootOnly, first); semantic == nil || semantic.Dataflow == nil || semantic.Dataflow.OutputScopeID != "s1" {
		t.Fatalf("root graph should fit fixed budget: %+v", semantic)
	}
	second, cancelSecond := newParseBudget(context.Background(), limits)
	defer cancelSecond()
	defer func() {
		got, ok := recover().(budgetAbort)
		t.Logf("fixed cap=%d root-only=%d recursive-at-failure=%d", limits.maxMetadataBytes, first.metadataBytes, second.metadataBytes)
		// The root fits; recursive owned graph text consumes the remainder.
		if !ok || got.err == nil || !strings.Contains(got.err.Error(), "metadata bytes") || second.metadataBytes <= first.metadataBytes {
			t.Fatalf("budget did not fail inside recursive child graph clone: error=%v root=%d recursive=%d", got, first.metadataBytes, second.metadataBytes)
		}
	}()
	semanticFromExtraction(root, second)
}

func TestKQLSourceEditsRejectInvalidSpans(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edits []kqlSourceEdit
	}{
		{"overlap", []kqlSourceEdit{{Start: 0, End: 3}, {Start: 2, End: 4}}},
		{"out of range", []kqlSourceEdit{{Start: 0, End: 100}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
			defer cancel()
			source := newKQLMappedSource(`T | where x==1`, budget)
			defer func() {
				if recover() == nil {
					t.Fatal("invalid edit was accepted")
				}
			}()
			applyKQLSourceEdits(source, tc.edits, budget)
		})
	}
}

func TestKQLChildSourceUsesCanceledSharedBudget(t *testing.T) {
	query := `T | join (U | where x==1) on Key`
	budget, cancel := newParseBudget(context.Background(), defaultParseLimits())
	source := newKQLMappedSource(query, budget)
	lexer := NewKQLLexer(antlr.NewInputStream(query))
	parser := NewKQLParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	owners := associateKQLSource(parser.Query(), source, budget)
	var child antlr.ParserRuleContext
	for ctx, id := range owners.Queries {
		if id != source.SelectedOwnerID {
			child = ctx
			break
		}
	}
	if child == nil {
		t.Fatal("join child owner missing")
	}
	cancel()
	defer func() {
		if got, ok := recover().(budgetAbort); !ok || got.err == nil {
			t.Fatalf("child ignored canceled budget: %v", got)
		}
	}()
	owners.childSource(child, budget)
}
