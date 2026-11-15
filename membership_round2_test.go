package kql_test

import (
	"testing"

	k "github.com/craftedsignal/kql-parser"
)

func TestMembershipRound2CompleteOwnerBothSides(t *testing.T) {
	for _, atom := range []string{
		`true == (a == 1 or Account in ((SigninLogs | project Account)))`,
		`false == (Account in ((SigninLogs | project Account)) or a == 1)`,
		`1 + toint((a == 1 and Account in ((SigninLogs | project Account)))) > 0`,
		`0 < 1 + toint((a == 1 and Account in ((SigninLogs | project Account))))`,
	} {
		query := `T | where ` + atom
		parsed, semantic := k.ExtractConditions(query), k.AnalyzeQuery(query)
		if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 0 || len(semantic.Conditions) != 0 ||
			parsed.Expression == nil || parsed.Expression.Operator != k.BooleanScalar || parsed.Expression.Scalar == nil || parsed.Expression.Scalar.Text != atom ||
			semantic.Expression == nil || semantic.Expression.Operator != k.BooleanScalar || semantic.Expression.Scalar == nil || semantic.Expression.Scalar.Text != atom {
			t.Errorf("owning atom was not preserved: %q; parse=%+v semantic=%+v", atom, parsed, semantic)
		}
	}
}

func TestMembershipRound2NestedConstructorValidation(t *testing.T) {
	valid := `Account in ((U | union (datatable(a:string)["x"]) | project a))`
	parsed, semantic := k.ExtractConditions(`T | where `+valid), k.AnalyzeQuery(`T | where `+valid)
	if len(parsed.Errors) != 0 || len(semantic.Errors) != 0 || len(parsed.Conditions) != 0 || len(semantic.Conditions) != 0 ||
		parsed.Expression == nil || parsed.Expression.Scalar == nil || parsed.Expression.Scalar.Text != valid ||
		semantic.Expression == nil || semantic.Expression.Scalar == nil || semantic.Expression.Scalar.Text != valid {
		t.Errorf("valid nested constructor lost source: parse=%+v semantic=%+v", parsed, semantic)
	}
	for _, query := range []string{
		`T | where Account in ((U | union (datatable(a:string)["x",]) | project a))`,
		`T | where Account in ((U | union (externaldata(a:string)[@"https://example.test/a.csv",]) | project a))`,
	} {
		parsed, semantic := k.ExtractConditions(query), k.AnalyzeQuery(query)
		if len(parsed.Errors) == 0 || len(semantic.Errors) == 0 || len(k.Validate(query)) == 0 ||
			len(parsed.Conditions) != 0 || len(semantic.Conditions) != 0 || parsed.Expression != nil || semantic.Expression != nil {
			t.Errorf("malformed nested constructor accepted or produced partial filters: %q; parse=%+v semantic=%+v", query, parsed, semantic)
		}
	}
}
