package kql_test

import (
	"testing"

	k "github.com/craftedsignal/kql-parser"
)

func TestWave12ConstructorSeparators(t *testing.T) {
	for _, atom := range []string{
		"Account in (externaldata(a:string,)[@\"https://example.test/a]b.csv\", // terminal , ] comment\n])",
		"Account in (externaldata(a:string,)[@\"https://example.test/a]b.csv\", // terminal , ] comment\r])",
		`Account in (datatable(a:string)["x]y",])`,
		`Account in (datatable(a:string,)["x",])`,
		`Account in ((__kql_constructor_0__ | union (datatable(a:string)["__kql_constructor_0__",]) | project a))`,
	} {
		query := "T | where " + atom
		if errs := k.Validate(query); len(errs) != 0 {
			t.Errorf("valid separators rejected: %q: %v", atom, errs)
		}
		p, s := k.ExtractConditions(query), k.AnalyzeQuery(query)
		if len(p.Errors) != 0 || len(s.Errors) != 0 || len(p.Conditions) != 0 || len(s.Conditions) != 0 || p.Expression == nil || s.Expression == nil || p.Expression.Scalar == nil || s.Expression.Scalar == nil || p.Expression.Scalar.Text != atom || s.Expression.Scalar.Text != atom {
			t.Errorf("authored constructor scalar lost: %q: %+v / %+v", atom, p, s)
			continue
		}
		for _, flow := range []*k.QueryDataflow{p.Dataflow, s.Dataflow} {
			if flow == nil || len(flow.Stages) != 1 || flow.Stages[0].Source != "where "+atom || p.Expression.Scalar.StageID != flow.Stages[0].ID || s.Expression.Scalar.StageID != flow.Stages[0].ID || p.Expression.Scalar.InputScopeID == "" || p.Expression.Scalar.InputScopeID != s.Expression.Scalar.InputScopeID {
				t.Errorf("authored constructor owner lost: %+v", flow)
			}
		}
	}
}

func TestWave12ConstructorSeparatorGuards(t *testing.T) {
	for _, constructor := range []string{
		`datatable(a:string)[,"x"]`,
		`datatable(a:string)["x]y",] garbage "`,
		`datatable(a:string)["x",,]`,
		`datatable(a:string,,)["x"]`,
		`datatable(a:string,b:long)["x",]`,
		`externaldata(a:string)[]`,
		`datatable(a:string)["x", /* tail */]`,
		`externaldata(a:string)["uri",] with (format=)`,
	} {
		query := "T | where Account in (" + constructor + ")"
		p, s := k.ExtractConditions(query), k.AnalyzeQuery(query)
		if len(k.Validate(query)) == 0 || len(p.Errors) == 0 || len(s.Errors) == 0 || len(p.Conditions) != 0 || len(s.Conditions) != 0 || p.Expression != nil || s.Expression != nil || p.Dataflow != nil || s.Dataflow != nil {
			t.Errorf("malformed constructor exposed a model: %q: %+v / %+v", constructor, p, s)
		}
	}
}
