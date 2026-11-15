package kql_test

import (
	"encoding/json"
	k "github.com/craftedsignal/kql-parser"
	"reflect"
	"strings"
	"testing"
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
		t.Errorf("fabricated condition: parse=%+v semantic=%+v", p.Conditions, s.Conditions)
	}
}
func TestReviewSecondRHSValidation(t *testing.T) {
	q := `T | where isnull(Account in ((SigninLogs | project Account)) or User in ((AuditLogs | project )))`
	p, s := k.ExtractConditions(q), k.AnalyzeQuery(q)
	if len(p.Errors) == 0 || len(s.Errors) == 0 || p.Expression != nil || s.Expression != nil || len(k.Validate(q)) == 0 {
		t.Errorf("malformed second RHS accepted: parseErrors=%v semanticErrors=%v validate=%v", p.Errors, s.Errors, k.Validate(q))
	}
}
func TestReviewRemovedSyntaxValidation(t *testing.T) {
	q := `T | where Account in ((SigninLogs | mv-apply x on (U | where x ==) | project Account))`
	p, s := k.ExtractConditions(q), k.AnalyzeQuery(q)
	if len(p.Errors) == 0 || len(s.Errors) == 0 || p.Expression != nil || s.Expression != nil || len(k.Validate(q)) == 0 {
		t.Errorf("malformed removed syntax accepted: parseErrors=%v semanticErrors=%v validate=%v", p.Errors, s.Errors, k.Validate(q))
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
}
func TestReviewNonfilterOwnership(t *testing.T) {
	q := `T | project x=toscalar(U | where Account in ((SigninLogs | project Account)) | count)`
	p, s := k.ExtractConditions(q), k.AnalyzeQuery(q)
	pj, _ := json.Marshal(p)
	sj, _ := json.Marshal(s)
	if len(p.Errors) != 0 || len(s.Errors) != 0 || p.Expression != nil || s.Expression != nil || strings.Contains(string(pj), "__kql_membership_") || strings.Contains(string(sj), "__kql_membership_") {
		t.Errorf("nonfilter marker escaped: parse=%s semantic=%s", pj, sj)
	}
}
func TestReviewOrdinaryControls(t *testing.T) {
	q := `T | where Account in ((SigninLogs | project Account)) and b == "__kql_member" "ship_0__"`
	r := k.AnalyzeQuery(q)
	if len(r.Errors) != 0 || len(r.Conditions) != 1 || r.Conditions[0].Value != "__kql_membership_0__" || r.Expression == nil || r.Expression.Operator != k.BooleanAnd || r.Expression.Children[0].Scalar.Text != `Account in ((SigninLogs | project Account))` {
		t.Fatalf("control failed: %+v", r)
	}
}
