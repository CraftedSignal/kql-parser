package kql

import (
	"testing"
)

func TestFirstFieldInExpressionText(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`tostring(split(SenderMailFromAddress, "@")._idxn1)`, "SenderMailFromAddress"},
		{`tostring(split(SenderMailFromAddress, "@")[-1])`, "SenderMailFromAddress"},
		{`AccountName`, "AccountName"},
		{`tolower(CommandLine)`, "CommandLine"},
		{`coalesce(field1, field2)`, "field1"},
		{`FirstName + " " + LastName`, "FirstName"},
		{`strlen(AccountName)`, "AccountName"},
		{`123`, ""},
		{`"literal"`, ""},
	}
	for _, tc := range cases {
		got := firstFieldInExpressionText(tc.expr)
		if got != tc.want {
			t.Errorf("firstFieldInExpressionText(%q) = %q, want %q", tc.expr, got, tc.want)
		}
	}
}
