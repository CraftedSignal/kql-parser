package kql

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// OperandKind describes the authored role of a condition's right operand.
type OperandKind string

const (
	OperandString OperandKind = "string"
	OperandNumber OperandKind = "number"
	OperandBool   OperandKind = "bool"
	OperandNull   OperandKind = "null"
	OperandField  OperandKind = "field"
	OperandList   OperandKind = "list"
	OperandTyped  OperandKind = "typed"
)

// Operand retains syntax and value separately. Typed constructor values are
// unevaluated; Items contains structurally recognized arguments or list items.
type Operand struct {
	FieldBinding *FieldBindingRef `json:"field_binding,omitempty"`
	Kind         OperandKind      `json:"kind"`
	Source       string           `json:"source,omitempty"`
	Value        string           `json:"value,omitempty"`
	Type         string           `json:"type,omitempty"`
	Items        []Operand        `json:"items,omitempty"`
	Binding      string           `json:"binding,omitempty"`
}

func literalKQLOperand(token int, source string) *Operand {
	switch token {
	case KQLLexerSTRING_LITERAL, KQLLexerVERBATIM_STRING, KQLLexerMULTILINE_STRING:
		return &Operand{Kind: OperandString, Source: source, Value: decodeKQLString(source)}
	case KQLLexerINT_NUMBER, KQLLexerLONG_NUMBER, KQLLexerREAL_NUMBER, KQLLexerDECIMAL_NUMBER, KQLLexerHEX_NUMBER:
		return &Operand{Kind: OperandNumber, Source: source, Value: source}
	case KQLLexerTRUE, KQLLexerFALSE:
		return &Operand{Kind: OperandBool, Source: source, Value: source}
	case KQLLexerNULL:
		return &Operand{Kind: OperandNull, Source: source}
	case KQLLexerDATETIME_LITERAL, KQLLexerTIMESPAN_LITERAL, KQLLexerGUID_LITERAL, KQLLexerDYNAMIC_LITERAL:
		open := strings.IndexByte(source, '(')
		if open > 0 {
			return &Operand{Kind: OperandTyped, Source: source, Type: strings.ToLower(strings.TrimSpace(source[:open]))}
		}
	case KQLLexerTIMESPAN_SHORT:
		if numericToken.MatchString(source) && strings.HasSuffix(source, "M") {
			return &Operand{Kind: OperandNumber, Source: source, Value: source}
		}
		return &Operand{Kind: OperandTyped, Source: source, Type: "timespan"}
	}
	return nil
}

// trimObfuscationPrefix removes the h or H that marks a standard or verbatim
// string literal as obfuscated; the literal keeps its value.
func trimObfuscationPrefix(s string) string {
	if len(s) > 2 && (s[0] == 'h' || s[0] == 'H') && (s[1] == '"' || s[1] == '\'' || s[1] == '@' && (s[2] == '"' || s[2] == '\'')) {
		return s[1:]
	}
	return s
}

func decodeKQLString(source string) string {
	return decodeKQLStringBudgeted(source, nil)
}

// decodeKQLStringBudgeted reserves a single source-bounded backing buffer
// before decoding; the public internal helper retains its existing value API.
func decodeKQLStringBudgeted(source string, budget *parseBudget) string {
	s := source
	if strings.HasPrefix(s, "```") && strings.HasSuffix(s, "```") && len(s) >= 6 {
		return s[3 : len(s)-3]
	}
	s = trimObfuscationPrefix(s)
	verbatim := strings.HasPrefix(s, "@")
	if verbatim {
		s = s[1:]
	}
	if len(s) < 2 {
		return s
	}
	quote := s[0]
	if quote != '\'' && quote != '"' {
		return s
	}
	width := 1
	if len(s) >= 6 && s[0] == s[1] && s[1] == s[2] {
		width = 3
	}
	if len(s) < 2*width {
		return s
	}
	s = s[width : len(s)-width]
	budget.reserveWork(len(s)) // Precharge the complete decoding scan.
	var b strings.Builder
	if verbatim {
		size := len(s)
		for i := 0; i+1 < len(s); i++ {
			budget.check()
			if s[i] == quote && s[i+1] == quote {
				size--
				i++
			}
		}
		if size == len(s) {
			return s
		}
		budget.reserveWork(size)
		b.Grow(size)
		for i := 0; i < len(s); i++ {
			budget.check()
			b.WriteByte(s[i])
			if s[i] == quote && i+1 < len(s) && s[i+1] == quote {
				i++
			}
		}
		return b.String()
	}
	budget.reserveWork(len(s)) // One backing allocation; decoded bytes fit.
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		budget.check()
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '0':
			b.WriteByte(0)
		case 'u', 'U', 'x':
			n := 4
			switch s[i] {
			case 'U':
				n = 8
			case 'x':
				n = 2
			}
			if i+n < len(s) {
				if value, err := strconv.ParseUint(s[i+1:i+1+n], 16, 32); err == nil {
					if s[i] == 'u' && value >= 0xd800 && value <= 0xdbff && i+10 < len(s) && s[i+5] == '\\' && s[i+6] == 'u' {
						if low, err := strconv.ParseUint(s[i+7:i+11], 16, 16); err == nil && low >= 0xdc00 && low <= 0xdfff {
							b.WriteRune(rune(0x10000 + (value-0xd800)*0x400 + (low - 0xdc00)))
							i += 10
							continue
						}
					}
					if value > utf8.MaxRune {
						b.WriteRune(utf8.RuneError)
					} else {
						b.WriteRune(rune(value))
					}
					i += n
					continue
				}
			}
			b.WriteByte(s[i])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func cloneOperand(value *Operand, budget *parseBudget) *Operand {
	return cloneOperandReserved(value, budget, false)
}

func cloneOperandReserved(value *Operand, budget *parseBudget, reserved bool) *Operand {
	budget.step()
	if value == nil {
		return nil
	}
	if !reserved {
		budget.reserveOutput(1)
	}
	budget.reserveOutput(len(value.Items))
	budget.reserveMetadataStrings(0, value.Source, value.Value, value.Type, value.Binding)
	copy := *value
	copy.FieldBinding = cloneFieldBindingRef(value.FieldBinding, budget)
	if len(value.Items) != 0 {
		copy.Items = make([]Operand, len(value.Items))
		for i := range value.Items {
			child := cloneOperandReserved(&value.Items[i], budget, true)
			copy.Items[i] = *child
		}
	}
	return &copy
}

func operandOutputCost(value *Operand, budget *parseBudget) int {
	budget.step()
	if value == nil {
		return 0
	}
	cost := 1
	budget.reserveMetadataStrings(0, value.Source, value.Value, value.Type, value.Binding)
	if value.FieldBinding != nil {
		budget.reserveMetadataStrings(1, value.FieldBinding.ScopeID, value.FieldBinding.Kind, value.FieldBinding.BindingID)
	}
	for i := range value.Items {
		cost += operandOutputCost(&value.Items[i], budget)
	}
	return cost
}

func operandIdentity(value *Operand, budgets ...*parseBudget) string {
	identity, _ := operandIdentityParts(value, suppliedBudget(budgets))
	return identity
}

// Return the added reference tags/IDs separately so retained identity keys
// keep exactly their pre-wave2 legacy cost.
func operandIdentityParts(value *Operand, budget *parseBudget) (string, int) {
	var out strings.Builder
	metadataBytes := 0
	var visit func(*Operand)
	field := func(s string) {
		out.WriteString(strconv.Itoa(len(s)))
		out.WriteByte(':')
		out.WriteString(s)
	}
	visit = func(node *Operand) {
		budget.step()
		if node == nil {
			out.WriteByte('0')
			return
		}
		out.WriteByte('1')
		field(string(node.Kind))
		field(node.Source)
		field(node.Value)
		field(node.Type)
		field(node.Binding)
		metadataStart := out.Len()
		if node.FieldBinding == nil {
			out.WriteByte('0')
		} else {
			out.WriteByte('1')
			field(node.FieldBinding.ScopeID)
			field(node.FieldBinding.Kind)
			field(node.FieldBinding.BindingID)
		}
		metadataBytes += out.Len() - metadataStart
		out.WriteString(strconv.Itoa(len(node.Items)))
		out.WriteByte(':')
		for i := range node.Items {
			visit(&node.Items[i])
		}
	}
	visit(value)
	return out.String(), metadataBytes
}

func conditionOperandItems(condition Condition) []Operand {
	if condition.RHS == nil {
		return nil
	}
	if condition.RHS.Kind == OperandList {
		return append([]Operand(nil), condition.RHS.Items...)
	}
	return []Operand{*condition.RHS}
}

func deduplicateTypedAlternatives(values []string, items []Operand) ([]string, []Operand) {
	seen := make(map[string]bool, len(items))
	outValues := make([]string, 0, len(values))
	outItems := make([]Operand, 0, len(items))
	for i, item := range items {
		key := operandIdentity(&item)
		if seen[key] {
			continue
		}
		seen[key] = true
		outItems = append(outItems, item)
		if i < len(values) {
			outValues = append(outValues, values[i])
		}
	}
	return outValues, outItems
}

func portableTypedOperand(raw string) *Operand {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(raw), "dynamic(") && strings.HasSuffix(raw, ")") {
		return &Operand{Kind: OperandTyped, Source: raw, Type: "dynamic"}
	}
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
		parts := splitCommaList(raw[1 : len(raw)-1])
		if len(parts) == 1 {
			if child := portableTypedOperand(parts[0]); child != nil {
				copy := *child
				copy.Source = raw
				if copy.Kind == OperandNumber {
					copy.Value = raw
				}
				return &copy
			}
		}
		items := make([]Operand, 0, len(parts))
		for _, part := range parts {
			child := portableTypedOperand(part)
			if child == nil {
				return nil
			}
			items = append(items, *child)
		}
		return &Operand{Kind: OperandList, Source: raw, Items: items}
	}
	if _, rest, ok := parseStringLiteral(raw); ok && strings.TrimSpace(rest) == "" {
		return &Operand{Kind: OperandString, Source: raw, Value: decodeKQLString(raw)}
	}
	if raw == "true" || raw == "false" {
		return &Operand{Kind: OperandBool, Source: raw, Value: raw}
	}
	if raw == "null" {
		return &Operand{Kind: OperandNull, Source: raw}
	}
	if numericToken.MatchString(strings.TrimLeft(raw, "+-")) {
		return &Operand{Kind: OperandNumber, Source: raw, Value: raw}
	}
	return nil
}

func portableTypedOperandWithReference(raw, reference string) *Operand {
	if value := portableTypedOperand(raw); value != nil {
		return value
	}
	if reference != "" {
		return &Operand{Kind: OperandField, Source: strings.TrimSpace(raw), Value: reference}
	}
	return nil
}

func portablePredicateOperand(raw, reference, operator string) *Operand {
	value := portableTypedOperandWithReference(raw, reference)
	if value == nil {
		return nil
	}
	listOperator := false
	switch operator {
	case "in", "in~", "!in", "!in~", "has_any", "has_all":
		listOperator = true
	}
	if listOperator && strings.HasPrefix(strings.TrimSpace(raw), "(") && value.Kind != OperandList && value.Kind != OperandField {
		source := strings.TrimSpace(raw)
		if child := portableTypedOperand(strings.TrimSpace(source[1 : len(source)-1])); child != nil {
			return &Operand{Kind: OperandList, Source: source, Items: []Operand{*child}}
		}
		return nil
	}
	return value
}

func portableListProjectionMatches(rhs *Operand, alternatives []string) bool {
	if rhs == nil || rhs.Kind != OperandList {
		return true
	}
	if len(rhs.Items) != len(alternatives) {
		return false
	}
	for i, item := range rhs.Items {
		if item.Kind != OperandString && item.Kind != OperandNumber && item.Kind != OperandBool || item.Value != alternatives[i] {
			return false
		}
	}
	return true
}
