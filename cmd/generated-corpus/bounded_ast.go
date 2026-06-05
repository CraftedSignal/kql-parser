package main

import (
	crand "crypto/rand"
	"fmt"
	"math/big"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

const semanticMaxDepth = 3

type semanticPicker interface {
	Intn(int) int
	Int63() int64
}

type seededSemanticPicker struct {
	rng *rand.Rand
}

func (p seededSemanticPicker) Intn(n int) int {
	return p.rng.Intn(n)
}

func (p seededSemanticPicker) Int63() int64 {
	return p.rng.Int63()
}

type cryptoSemanticPicker struct{}

func (cryptoSemanticPicker) Intn(n int) int {
	if n <= 0 {
		panic("semantic picker called with non-positive bound")
	}
	value, err := crand.Int(crand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(fmt.Sprintf("crypto random read failed: %v", err))
	}
	return int(value.Int64())
}

func (p cryptoSemanticPicker) Int63() int64 {
	const maxInt63 = int64(1<<63 - 1)
	value, err := crand.Int(crand.Reader, big.NewInt(maxInt63))
	if err != nil {
		panic(fmt.Sprintf("crypto random read failed: %v", err))
	}
	return value.Int64()
}

type semanticExpr struct {
	kind         string
	condition    expectedCondition
	left         *semanticExpr
	right        *semanticExpr
	child        *semanticExpr
	parenthesize bool
}

func generateBoundedSemanticCase(seed, id int64) generatedCase {
	var picker semanticPicker
	caseSeed := seedForCase(seed, id)
	if seed < 0 {
		picker = cryptoSemanticPicker{}
		caseSeed = picker.Int63()
	} else {
		picker = seededSemanticPicker{rng: rand.New(rand.NewSource(caseSeed))}
	}

	query, expected := generateKQLSemanticQuery(picker)
	return generatedCase{
		ID:       id,
		Seed:     caseSeed,
		Query:    query,
		Expected: expected,
	}
}

func generateKQLSemanticQuery(p semanticPicker) (string, []expectedCondition) {
	table := semanticPickString(p, tables)
	expr := generateSemanticExpr(p, 1+p.Intn(semanticMaxDepth))
	rendered := renderKQLSemanticExpr(p, expr)
	expected := semanticExpected(expr, false)

	switch p.Intn(7) {
	case 0:
		return fmt.Sprintf("%s | where %s", table, rendered), expected
	case 1:
		return fmt.Sprintf("%s | where %s | project TimeGenerated, %s | take %d",
			table, rendered, strings.Join(semanticProjectionFields(expected), ", "), 1+p.Intn(500)), expected
	case 2:
		groupField := semanticPickString(p, []string{"AccountName", "TargetUserName", "DeviceName", "RemoteIP", "ActionType"})
		return fmt.Sprintf("%s | where %s | summarize EventCount=count() by %s | sort by EventCount desc | take %d",
			table, rendered, groupField, 1+p.Intn(250)), expected
	case 3:
		second := generateSemanticExpr(p, 1+p.Intn(semanticMaxDepth))
		query := fmt.Sprintf("%s | where %s | extend RiskScore = Count + %d | where %s | project TimeGenerated, %s | order by TimeGenerated desc",
			table, rendered, 1+p.Intn(50), renderKQLSemanticExpr(p, second), strings.Join(semanticProjectionFields(append(expected, semanticExpected(second, false)...)), ", "))
		return query, append(expected, semanticExpected(second, false)...)
	case 4:
		sub := generateSemanticExpr(p, 1+p.Intn(semanticMaxDepth))
		leftTable := semanticPickString(p, []string{"SecurityEvent", "DeviceNetworkEvents", "SigninLogs"})
		rightTable := semanticPickString(p, []string{"DeviceProcessEvents", "DeviceNetworkEvents", "AuditLogs"})
		query := fmt.Sprintf("%s | where %s | join kind=%s (%s | where %s | project DeviceId, %s) on DeviceId | project DeviceId, %s",
			leftTable,
			rendered,
			semanticPickString(p, []string{"inner", "leftouter", "innerunique"}),
			rightTable,
			renderKQLSemanticExpr(p, sub),
			semanticPickString(p, stringFields),
			semanticPickString(p, stringFields),
		)
		return query, append(expected, semanticExpected(sub, false)...)
	case 5:
		query := fmt.Sprintf("%s\n| where %s\n| project TimeGenerated, %s\n| sort by TimeGenerated desc\n| take %d",
			table, rendered, strings.Join(semanticProjectionFields(expected), ", "), 1+p.Intn(200))
		query = strings.ReplaceAll(query, " and ", "\n    and ")
		query = strings.ReplaceAll(query, " or ", "\n    or ")
		return query, expected
	default:
		return fmt.Sprintf("%s | where (%s) | distinct %s | take %d",
			table, rendered, semanticPickString(p, []string{"AccountName", "DeviceName", "RemoteIP", "FileName"}), 1+p.Intn(200)), expected
	}
}

func generateSemanticExpr(p semanticPicker, depth int) *semanticExpr {
	if depth <= 0 {
		return &semanticExpr{kind: "predicate", condition: randomSemanticCondition(p), parenthesize: p.Intn(2) == 0}
	}
	switch p.Intn(5) {
	case 0:
		return &semanticExpr{kind: "predicate", condition: randomSemanticCondition(p), parenthesize: p.Intn(2) == 0}
	case 1:
		return &semanticExpr{kind: "and", left: generateSemanticExpr(p, depth-1), right: generateSemanticExpr(p, depth-1), parenthesize: p.Intn(3) != 0}
	case 2:
		return &semanticExpr{kind: "or", left: generateSemanticExpr(p, depth-1), right: generateSemanticExpr(p, depth-1), parenthesize: p.Intn(3) != 0}
	case 3:
		return &semanticExpr{kind: "not", child: generateSemanticExpr(p, depth-1), parenthesize: true}
	default:
		leftDepth := p.Intn(depth + 1)
		rightDepth := p.Intn(depth + 1)
		return &semanticExpr{kind: "and", left: generateSemanticExpr(p, leftDepth), right: generateSemanticExpr(p, rightDepth), parenthesize: true}
	}
}

func randomSemanticCondition(p semanticPicker) expectedCondition {
	field := semanticPickString(p, fields)
	if contains(numericFields, field) {
		return expectedCondition{
			Field:    field,
			Operator: semanticPickString(p, []string{"==", "!=", ">", "<", ">=", "<="}),
			Value:    strconv.Itoa(1 + p.Intn(9000)),
		}
	}

	switch p.Intn(4) {
	case 0:
		return expectedCondition{
			Field:    field,
			Operator: semanticPickString(p, []string{"==", "!="}),
			Value:    semanticRandomValueForField(p, field),
		}
	case 1:
		return expectedCondition{
			Field:    field,
			Operator: semanticPickString(p, []string{"contains", "has", "startswith", "endswith"}),
			Value:    semanticRandomValueForField(p, field),
		}
	case 2:
		values := semanticUniqueValues(p, semanticValuesForField(field), 2+p.Intn(3))
		return expectedCondition{Field: field, Operator: "==", Value: values[0], Alternatives: values, Negated: p.Intn(4) == 0}
	default:
		return expectedCondition{
			Field:    field,
			Operator: "==",
			Value:    semanticRandomValueForField(p, field),
			Negated:  p.Intn(3) == 0,
		}
	}
}

func renderKQLSemanticExpr(p semanticPicker, expr *semanticExpr) string {
	switch expr.kind {
	case "predicate":
		return maybeSemanticParens(p, renderCondition(expr.condition), expr.parenthesize)
	case "and":
		return maybeSemanticParens(p, renderKQLSemanticExpr(p, expr.left)+" and "+renderKQLSemanticExpr(p, expr.right), expr.parenthesize)
	case "or":
		return maybeSemanticParens(p, renderKQLSemanticExpr(p, expr.left)+" or "+renderKQLSemanticExpr(p, expr.right), expr.parenthesize)
	case "not":
		return "not(" + renderKQLSemanticExpr(p, expr.child) + ")"
	default:
		panic("unknown semantic expression kind")
	}
}

func semanticExpected(expr *semanticExpr, negated bool) []expectedCondition {
	switch expr.kind {
	case "predicate":
		cond := expr.condition
		if negated {
			cond.Negated = !cond.Negated
		}
		return []expectedCondition{cond}
	case "and", "or":
		out := semanticExpected(expr.left, negated)
		out = append(out, semanticExpected(expr.right, negated)...)
		return out
	case "not":
		return semanticExpected(expr.child, !negated)
	default:
		panic("unknown semantic expression kind")
	}
}

func semanticProjectionFields(expected []expectedCondition) []string {
	seen := make(map[string]bool, len(expected))
	out := make([]string, 0, 4)
	for _, cond := range expected {
		if cond.Field == "" || seen[cond.Field] {
			continue
		}
		seen[cond.Field] = true
		out = append(out, cond.Field)
		if len(out) == 4 {
			return out
		}
	}
	for _, field := range []string{"AccountName", "DeviceName", "RemoteIP", "FileName"} {
		if !seen[field] {
			out = append(out, field)
		}
		if len(out) == 4 {
			return out
		}
	}
	return out
}

func maybeSemanticParens(_ semanticPicker, value string, enabled bool) string {
	if enabled {
		return "(" + value + ")"
	}
	return value
}

func semanticRandomValueForField(p semanticPicker, field string) string {
	values := semanticValuesForField(field)
	return semanticPickString(p, values)
}

func semanticValuesForField(field string) []string {
	switch field {
	case "FileName", "InitiatingProcessFileName":
		return []string{"cmd.exe", "powershell.exe", "pwsh.exe", "rundll32.exe", "mshta.exe", "svchost.exe"}
	case "CommandLine", "ProcessCommandLine":
		return []string{"-enc", "downloadstring", "whoami", "net user", " /c ", "Invoke-WebRequest"}
	case "RemoteIP", "LocalIP", "IPAddress":
		return []string{"10.0.0.5", "192.168.1.10", "172.16.1.20", "8.8.8.8", "203.0.113.10"}
	case "ActionType":
		return []string{"ProcessCreated", "ConnectionSuccess", "FileCreated", "LogonFailed", "LogonSuccess"}
	case "Status", "ResultType":
		return []string{"Success", "Failure", "Failed", "0", "50074", "Denied"}
	case "FolderPath":
		return []string{`C:\Windows\System32`, `C:\Users\Public`, `C:\ProgramData`, `C:\Temp`}
	case "Message":
		return []string{"error", "critical", "failed password", "mfa required", "admin login"}
	default:
		return []string{"admin", "root", "SYSTEM", "svc_app", "web01", "test"}
	}
}

func semanticUniqueValues(p semanticPicker, pool []string, count int) []string {
	if count > len(pool) {
		count = len(pool)
	}
	seen := make(map[string]bool, count)
	out := make([]string, 0, count)
	for len(out) < count {
		value := semanticPickString(p, pool)
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func semanticPickString(p semanticPicker, values []string) string {
	return values[p.Intn(len(values))]
}
