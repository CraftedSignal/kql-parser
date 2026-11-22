package kql

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestProjectReorderPublic(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, stage, field string
		binding                    bool
	}{
		{"all directions", "T", "project-reorder a* asc, b* desc, c* granny-asc, d* granny-desc", "a1", false},
		{"plain", "T", "project-reorder a*, b", "a1", false},
		{"empty", "T", "project-reorder", "a1", false},
		{"quoted and ordinary names", `T | extend Alias=src`, "project-reorder ['Alias'] desc, granny, ['granny-desc']", "Alias", true},
		{"authored trivia", "T", "project-reorder /*selectors*/ a* asc,\n b* granny-desc", "a1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := tc.prefix + " | " + tc.stage + " | where ['" + tc.field + `'] == "x"`
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			if errs := Validate(query); len(errs) != 0 {
				t.Fatalf("Validate: %v", errs)
			}
			if len(p.Errors)+len(s.Errors) != 0 || p.Dataflow == nil || s.Dataflow == nil || !p.Dataflow.Complete || !s.Dataflow.Complete || !reflect.DeepEqual(p.Dataflow, s.Dataflow) || !reflect.DeepEqual(p.Conditions, s.Conditions) {
				t.Fatalf("public reorder models: errors=%v/%v flow=%+v/%+v", p.Errors, s.Errors, p.Dataflow, s.Dataflow)
			}
			for _, flow := range []*QueryDataflow{p.Dataflow, s.Dataflow} {
				index := 0
				if tc.binding {
					index = 1
				}
				stage := flow.Stages[index]
				if stage.Command != "project-reorder" || stage.Source != tc.stage || stage.Effect != "retain" || len(stage.Inputs) != 1 || stage.Inputs[0].QueryPath != "" || len(stage.BindingIDs) != 0 || stage.OutputScopeID == stage.Inputs[0].ScopeID {
					t.Fatalf("not a same-owner retain: %+v", stage)
				}
				found := false
				for _, scope := range flow.Scopes {
					if scope.ID == stage.OutputScopeID {
						found = true
						if scope.Mode != "inherit" || scope.ParentID != stage.Inputs[0].ScopeID || len(scope.BindingIDs)+len(scope.RemovedFields) != 0 {
							t.Fatalf("reorder changed environment: %+v", scope)
						}
					}
				}
				if !found {
					t.Fatal("missing inherited output")
				}
				if len(p.Conditions) != 1 || p.Conditions[0].InputScopeID != stage.OutputScopeID || p.Conditions[0].Field != tc.field || p.Conditions[0].FieldBinding == nil {
					t.Fatalf("downstream ownership: %+v", p.Conditions)
				}
				wantBindings := 0
				if tc.binding {
					wantBindings = 1
					if p.Conditions[0].FieldBinding.Kind != "binding" || p.Conditions[0].FieldBinding.BindingID != flow.Bindings[0].ID {
						t.Fatal("prior alias binding lost")
					}
				}
				if len(flow.Bindings) != wantBindings {
					t.Fatalf("reorder invented bindings: %+v", flow.Bindings)
				}
			}
		})
	}
	closed := ExtractConditions(`T | project kept=src | project-reorder * desc | where missing == "x"`)
	if len(closed.Errors) != 0 || closed.Dataflow == nil || len(closed.Conditions) != 1 || closed.Conditions[0].FieldBinding == nil || closed.Conditions[0].FieldBinding.Kind != "absent" || closed.Dataflow.Stages[1].Effect != "retain" {
		t.Fatalf("closed-scope absence changed: %+v", closed)
	}
	control := ExtractConditions(`T | extend granny=src | where msg == "project-reorder a bogus-asc"`)
	if len(control.Errors) != 0 || control.Dataflow == nil || !control.Dataflow.Complete {
		t.Fatalf("ordinary name/string changed: %+v", control)
	}
}

func TestProjectReorderMalformed(t *testing.T) {
	for _, suffix := range []string{"a bogus-asc", "a granny -asc", "a granny- asc", "a granny/*comment*/-asc", "a Granny-asc", "a ASC", "a,", ",a", "a asc b desc", "a asc,,b desc", "a granny-", "a desc |"} {
		t.Run(suffix, func(t *testing.T) {
			query := `T | where x == "v" | project-reorder ` + suffix
			p, s := ExtractConditions(query), AnalyzeQuery(query)
			if len(Validate(query)) == 0 || len(p.Errors) == 0 || len(s.Errors) == 0 || p.Dataflow != nil || s.Dataflow != nil {
				t.Fatalf("malformed reorder promoted: errors=%v/%v flow=%+v/%+v", p.Errors, s.Errors, p.Dataflow, s.Dataflow)
			}
		})
	}
}

func TestProjectReorderBudgetAndClone(t *testing.T) {
	query := `T | extend Alias=src | project-reorder ['Alias'] granny-asc | where ['Alias']=="x"`
	for _, semantic := range []bool{false, true} {
		b, cancel := newParseBudget(context.Background(), defaultParseLimits())
		p := extractConditionsInternal(query, b)
		if len(p.Errors) != 0 || p.Dataflow == nil || !p.Dataflow.Complete {
			cancel()
			t.Fatalf("budget baseline: %+v", p)
		}
		if semantic {
			_ = semanticFromExtraction(p, b)
		}
		work := b.work
		cancel()
		limits := defaultParseLimits()
		limits.maxWork = work - 1
		if semantic {
			r := analyzeQueryContext(context.Background(), query, limits)
			if len(r.Errors) != 1 || !strings.Contains(r.Errors[0], "work budget") || r.Dataflow != nil || len(r.Conditions) != 0 || r.Expression != nil {
				t.Fatalf("partial semantic failure: %+v", r)
			}
		} else {
			r := extractConditionsContext(context.Background(), query, limits)
			if len(r.Errors) != 1 || !strings.Contains(r.Errors[0], "work budget") || r.Dataflow != nil || len(r.Conditions) != 0 || r.Expression != nil {
				t.Fatalf("partial extraction failure: %+v", r)
			}
		}
	}
	p := ExtractConditions(query)
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	s := semanticFromExtraction(p, b)
	s.Dataflow.Stages[1].Source = "changed"
	s.Dataflow.Stages[1].Inputs[0].ScopeID = "changed"
	s.Dataflow.Scopes[2].ParentID = "changed"
	if p.Dataflow.Stages[1].Source == "changed" || p.Dataflow.Stages[1].Inputs[0].ScopeID == "changed" || p.Dataflow.Scopes[2].ParentID == "changed" {
		t.Fatal("reorder graph clone aliases extraction")
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	p, s = ExtractConditionsContext(ctx, query), AnalyzeQueryContext(ctx, query)
	if len(ValidateContext(ctx, query)) == 0 || len(p.Errors) != 1 || len(s.Errors) != 1 || p.Dataflow != nil || s.Dataflow != nil || len(p.Conditions)+len(s.Conditions) != 0 {
		t.Fatal("cancellation published partial reorder")
	}
}

func TestProjectReorderNeighborWildcard(t *testing.T) {
	const query = `T | union U* | project-reorder a* | where msg == "project-reorder a*"`
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	mapped := newKQLMappedSource(query, b)
	metadata := &kqlNormalizationMetadata{mapped: mapped}
	normalized := normalizeBudgetedMetadata(query, false, b, metadata)
	const want = `T | union U | project-reorder a* | where msg == "project-reorder a*"`
	if normalized != want || mapped.Text != want {
		t.Fatalf("reorder bypass changed a neighbor: normalized=%q mapped=%q", normalized, mapped.Text)
	}
}

func TestProjectReorderAbsenceProofRetainsUnownedTypedCheck(t *testing.T) {
	const query = "T | project-reorder a"
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	source := newKQLMappedSource(query, b)
	stream := preflight(query, b, true, nil)
	parser := NewKQLParser(&budgetTokenStream{CommonTokenStream: stream, budget: b})
	parser.RemoveErrorListeners()
	tree := parser.Query()
	owners := associateKQLSource(tree, source, b)
	// Remove only the authored ownership evidence. A real typed reorder must
	// still be rejected even when the authored reorder count becomes zero.
	for _, id := range owners.Stages {
		source.Occurrences[id].Command = "where"
	}
	clear(owners.Stages)
	if diagnostic := validateKQLProjectReorders(tree, source, owners, stream.GetAllTokens(), false, b); diagnostic == "" {
		t.Fatal("zero authored count bypassed unowned typed reorder validation")
	}
}
