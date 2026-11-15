package kql_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	k "github.com/craftedsignal/kql-parser"
)

func TestReviewOwningAtom(t *testing.T) {
	for _, atom := range []string{`(a == 1 or Account in ((SigninLogs | project Account))) == false`, `toint((a == 1 and Account in ((SigninLogs | project Account)))) > 0`} {
		q := `T | where ` + atom
		p, s := k.ExtractConditions(q), k.AnalyzeQuery(q)
		if len(p.Errors) != 0 || len(s.Errors) != 0 || p.Expression == nil || s.Expression == nil || p.Expression.Scalar == nil || s.Expression.Scalar == nil || p.Expression.Scalar.Text != atom || s.Expression.Scalar.Text != atom {
			t.Errorf("owning atom lost: %s; parse=%+v semantic=%+v", q, p.Expression.Scalar, s.Expression.Scalar)
		}
	}
}
func TestReviewFilterAlias(t *testing.T) {
	q := `T | filter Account in ((SigninLogs | project Account))`
	p, s := k.ExtractConditions(q), k.AnalyzeQuery(q)
	if len(p.Errors) != 0 || len(s.Errors) != 0 || len(p.Conditions) != 0 || len(s.Conditions) != 0 || p.Expression == nil || s.Expression == nil || p.Expression.Operator != k.BooleanScalar || s.Expression.Operator != k.BooleanScalar {
		t.Errorf("fabricated condition: parse=%+v semantic=%+v", p, s)
	}
	q = `T | join (U | filter Account in ((SigninLogs | project Account))) on Account`
	p, s = k.ExtractConditions(q), k.AnalyzeQuery(q)
	if len(p.Errors) != 0 || len(s.Errors) != 0 || len(s.Joins) != 1 || s.Joins[0].Subquery == nil || s.Joins[0].Subquery.Expression == nil || s.Joins[0].Subquery.Expression.Scalar == nil || s.Joins[0].Subquery.Expression.Scalar.Text != `Account in ((SigninLogs | project Account))` {
		t.Errorf("join filter alias lost source: parse=%+v semantic=%+v", p, s)
	}
}
func TestReviewSecondRHSValidation(t *testing.T) {
	atom := `isnull(Account in ((SigninLogs | project Account)) or User in ((AuditLogs | project User)))`
	valid := k.AnalyzeQuery(`T | where ` + atom)
	if len(valid.Errors) != 0 || len(valid.Conditions) != 0 || valid.Expression == nil || valid.Expression.Scalar == nil || valid.Expression.Scalar.Text != atom {
		t.Errorf("two valid RHS in one owner lost source: %+v", valid)
	}
	for _, q := range []string{
		`T | where isnull(Account in ((SigninLogs | project Account)) or User in ((AuditLogs | project )))`,
		`T | where isnull(Account in ((SigninLogs | project Account)) or User in ((AuditLogs | where x == | project User)))`,
	} {
		p, s := k.ExtractConditions(q), k.AnalyzeQuery(q)
		if len(p.Errors) == 0 || len(s.Errors) == 0 || p.Expression != nil || s.Expression != nil || len(k.Validate(q)) == 0 {
			t.Errorf("malformed second RHS accepted: %s; parseErrors=%v semanticErrors=%v validate=%v", q, p.Errors, s.Errors, k.Validate(q))
		}
	}
}
func TestReviewRemovedSyntaxValidation(t *testing.T) {
	for _, q := range []string{
		`T | where Account in ((SigninLogs | mv-apply x on (U | where x ==) | project Account))`,
		`T | where Account in ((SigninLogs | parse x with THIS IS INVALID ??? | project Account))`,
	} {
		p, s := k.ExtractConditions(q), k.AnalyzeQuery(q)
		if len(p.Errors) == 0 || len(s.Errors) == 0 || p.Expression != nil || s.Expression != nil || len(k.Validate(q)) == 0 {
			t.Errorf("malformed removed syntax accepted: %s; parseErrors=%v semanticErrors=%v validate=%v", q, p.Errors, s.Errors, k.Validate(q))
		}
	}
}
func TestReviewTabularWithoutPipe(t *testing.T) {
	a := `T | where Account in (datatable(a:string)["x"])`
	b := `T | where Account in (datatable(a:string)["y"])`
	pa, pb := k.ExtractConditions(a), k.ExtractConditions(b)
	sa, sb := k.AnalyzeQuery(a), k.AnalyzeQuery(b)
	if reflect.DeepEqual(pa, pb) || reflect.DeepEqual(sa, sb) {
		t.Errorf("different table contents conflated: parse=%+v semantic=%+v", pa.Expression.Scalar, sa.Expression.Scalar)
	}
	for _, atom := range []string{`Account in (datatable(a:string)["x"])`, `Account in (datatable(a:string)["y"])`, `Account in ((datatable(a:string)["x"]))`, `Account in (datatable(a:string,b:long)["x",1,"y",2])`, `Account in (externaldata(a:string)[@"https://example.test/a.csv"])`, `Account in (externaldata(a:string)[@"https://example.test/a.csv"] with (format="csv"))`} {
		q := `T | where ` + atom
		p, s := k.ExtractConditions(q), k.AnalyzeQuery(q)
		if len(p.Errors) != 0 || len(s.Errors) != 0 || len(p.Conditions) != 0 || len(s.Conditions) != 0 || p.Expression == nil || s.Expression == nil || p.Expression.Scalar == nil || s.Expression.Scalar == nil || p.Expression.Scalar.Text != atom || s.Expression.Scalar.Text != atom {
			t.Errorf("constructor source lost: %s: parse=%+v semantic=%+v", q, p, s)
		}
	}
	for _, q := range []string{
		`T | where Account in (datatable(a:string)["x",])`,
		`T | where Account in (datatable(a:string)["x")`,
		`T | where Account in (datatable(a:)["x"])`,
		`T | where Account in (externaldata(a:string)[@"https://example.test/a.csv",])`,
	} {
		p, s := k.ExtractConditions(q), k.AnalyzeQuery(q)
		if len(p.Errors) == 0 || len(s.Errors) == 0 || p.Expression != nil || s.Expression != nil || len(k.Validate(q)) == 0 {
			t.Errorf("malformed constructor accepted: %s; parse=%+v semantic=%+v", q, p, s)
		}
	}
}
func TestReviewNonfilterOwnership(t *testing.T) {
	for _, q := range []string{
		`T | project x=toscalar(U | where Account in ((SigninLogs | project Account)) | count)`,
		`T | extend x=toscalar(U | where Account in ((SigninLogs | project Account)) | count)`,
		`T | summarize x=countif(toscalar(U | where Account in ((SigninLogs | project Account)) | count))`,
	} {
		p, s := k.ExtractConditions(q), k.AnalyzeQuery(q)
		pj, _ := json.Marshal(p)
		sj, _ := json.Marshal(s)
		if len(p.Errors) == 0 || len(s.Errors) == 0 || p.Expression != nil || s.Expression != nil || len(p.Conditions) != 0 || len(s.Conditions) != 0 || strings.Contains(string(pj), "__kql_membership_") || strings.Contains(string(sj), "__kql_membership_") || len(k.Validate(q)) == 0 {
			t.Errorf("unsupported nested nonfilter membership did not fail cleanly: parse=%s semantic=%s", pj, sj)
		}
	}
}
func TestReviewOrdinaryControls(t *testing.T) {
	q := `T | where Account in ((SigninLogs | project Account)) and b == "__kql_member" "ship_0__"`
	r := k.AnalyzeQuery(q)
	if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].Value != "__kql_membership_0__" || r.Expression == nil || r.Expression.Operator != k.BooleanAnd || r.Expression.Children[0].Scalar.Text != `Account in ((SigninLogs | project Account))` {
		t.Fatalf("control failed: %+v", r)
	}
}
