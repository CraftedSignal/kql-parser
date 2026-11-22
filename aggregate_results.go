package kql

import (
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
)

// AggregationResult describes the ordered members of one authored call.
// Complete is committed-stage completeness, including historical replacements.
type AggregationResult struct {
	CallOrdinal int                 `json:"call_ordinal"`
	Expression  string              `json:"expression"`
	Complete    bool                `json:"complete"`
	Outputs     []AggregationOutput `json:"outputs,omitempty"`
}
type AggregationOutput struct {
	Ordinal   int    `json:"ordinal"`
	Column    string `json:"column"`
	BindingID string `json:"binding_id"`
}

func percentileFamily(name string) (rankStart int, singular bool, ok bool) {
	switch name {
	case "percentile":
		return 1, true, true
	case "percentiles":
		return 1, false, true
	case "percentilew":
		return 2, true, true
	case "percentilesw":
		return 2, false, true
	}
	return 0, false, false
}
func aggregateCallName(item IAggregationItemContext, budget *parseBudget) string {
	if item.AggregationFunction() == nil || item.AggregationFunction().FunctionCall() == nil {
		return ""
	}
	call := item.AggregationFunction().FunctionCall()
	if call.Identifier() == nil || call.Identifier().GetStart() == nil || call.Identifier().GetStop() == nil || call.Identifier().GetStart().GetTokenIndex() != call.Identifier().GetStop().GetTokenIndex() {
		return ""
	}
	token := call.Identifier().GetStart()
	budget.reserveWork(token.GetStop() - token.GetStart() + 1)
	return token.GetText()
}
func resultAggregate(item IAggregationItemContext, budget *parseBudget) bool {
	if item.AggregationNameList() != nil {
		return true
	}
	function := aggregateCallName(item, budget)
	if function == "take_any" {
		call := item.AggregationFunction().FunctionCall()
		if call.ArgumentList() == nil {
			return false
		}
		count := 0
		for _, child := range call.ArgumentList().GetChildren() {
			budget.step()
			if _, ok := child.(IArgumentContext); ok {
				count++
				if count >= 2 {
					return true
				}
			}
		}
		return false
	}
	_, _, ok := percentileFamily(function)
	return ok
}

// This walk is deliberately take_any-only; positional rank/scalar policies
// stay in aggregateArguments. Cardinality is independent of name inference.
func takeAnyResultArguments(item IAggregationItemContext, owners *kqlSourceOwners, budget *parseBudget) ([]IArgumentContext, bool) {
	if item.AggregationFunction() == nil || item.AggregationFunction().FunctionCall() == nil {
		return nil, false
	}
	call := item.AggregationFunction().FunctionCall()
	if call.ArgumentList() == nil {
		return nil, false
	}
	var complete func(antlr.Tree) bool
	complete = func(tree antlr.Tree) bool {
		budget.step()
		if _, bad := tree.(antlr.ErrorNode); bad {
			return false
		}
		if terminal, ok := tree.(antlr.TerminalNode); ok && terminal.GetSymbol().GetTokenIndex() < 0 {
			return false
		}
		if ctx, ok := tree.(antlr.ParserRuleContext); ok && (ctx.GetStart() == nil || ctx.GetStop() == nil || ctx.GetStart().GetTokenIndex() < 0 || ctx.GetStop().GetTokenIndex() < ctx.GetStart().GetTokenIndex()) {
			return false
		}
		for _, child := range tree.GetChildren() {
			if !complete(child) {
				return false
			}
		}
		return true
	}
	var args []IArgumentContext
	for _, child := range call.ArgumentList().GetChildren() {
		budget.step()
		argument, ok := child.(IArgumentContext)
		if !ok {
			continue
		}
		if argument.Expression() == nil || !complete(argument) || standaloneAggregateWildcard(argument, budget) {
			return nil, false
		}
		if argument.ASSIGN() != nil {
			authored, found := namedArgumentSource(argument, owners, budget)
			if !found {
				sourceAbort("take_any declaration source ownership unavailable")
			}
			if strings.HasPrefix(authored, "[") {
				value, valid := decodedColumnName(authored, budget)
				if !valid || value == "" {
					return nil, false
				}
			}
		}
		args = appendTemporary(args, budget, argument)
	}
	return args, len(args) > 0
}

func planTakeAnyMembers(item IAggregationItemContext, e *conditionExtractor, budget *parseBudget) ([]string, []bool, bool) {
	args, valid := takeAnyResultArguments(item, e.sourceOwners, budget)
	if !valid || item.AS() != nil && len(args) > 1 {
		return nil, nil, false
	}
	aliases := aggregateDeclarations(item, e, budget)
	if len(aliases) > len(args) {
		return nil, nil, false
	}
	budget.reserveWork(len(args))
	budget.reserveWork(len(args))
	names := make([]string, len(args))
	declared := make([]bool, len(args))
	for i, argument := range args {
		budget.step()
		if i < len(aliases) {
			if aliases[i] == "" {
				return nil, nil, false
			}
			names[i] = aliases[i]
			declared[i] = true
			continue
		}
		if argument.ASSIGN() != nil {
			authored, found := namedArgumentSource(argument, e.sourceOwners, budget)
			if !found {
				sourceAbort("take_any declaration source ownership unavailable")
			}
			name := authored
			if strings.HasPrefix(authored, "[") {
				var known bool
				name, known = decodedColumnName(authored, budget)
				if !known {
					return nil, nil, false
				}
			}
			if name == "" {
				return nil, nil, false
			}
			names[i] = name
			continue
		}
		name, kind := e.aggregateExpressionResultName(argument.Expression(), budget)
		switch kind {
		case aggregateNameKnown:
			if name == "" {
				return nil, nil, false
			}
			names[i] = name
		case aggregateNameUnnamed:
			width := 1
			for n := i; n >= 10; n /= 10 {
				budget.step()
				width++
			}
			budget.reserveWork(width)
			budget.reserveWork(len("any_arg") + width)
			names[i] = "any_arg" + strconv.Itoa(i)
		default:
			return nil, nil, false
		}
	}
	return names, declared, true
}

func aggregateArguments(item IAggregationItemContext, budget *parseBudget) ([]IExpressionContext, bool) {
	if item.AggregationFunction() == nil || item.AggregationFunction().FunctionCall() == nil {
		return nil, false
	}
	call := item.AggregationFunction().FunctionCall()
	var args []IExpressionContext
	if call.ArgumentList() == nil {
		return args, true
	}
	for _, child := range call.ArgumentList().GetChildren() {
		budget.step()
		arg, ok := child.(IArgumentContext)
		if !ok {
			continue
		}
		if arg.ASSIGN() != nil || arg.Expression() == nil || standaloneAggregateWildcard(arg, budget) {
			return nil, false
		}
		args = appendTemporary(args, budget, arg.Expression())
	}
	return args, true
}

// Literal rank recognition follows transparent typed wrappers, never evaluates
// expressions or substituted let bindings. Integer defaults are deliberately narrow.
func rankLiteral(tree antlr.Tree, e *conditionExtractor, budget *parseBudget) (raw string, integer bool, ok bool) {
	for tree != nil {
		budget.step()
		if ctx, yes := tree.(antlr.ParserRuleContext); yes && e != nil && e.typed != nil && ctx.GetStart() != nil {
			if _, present := e.typed.byStart[ctx.GetStart().GetStart()]; present {
				return "", false, false
			}
		}
		switch ctx := tree.(type) {
		case ILiteralContext:
			for _, child := range ctx.GetChildren() {
				budget.step()
				if token, yes := child.(antlr.TerminalNode); yes {
					kind := token.GetSymbol().GetTokenType()
					if kind != KQLParserINT_NUMBER && kind != KQLParserREAL_NUMBER && kind != KQLParserDECIMAL_NUMBER && kind != KQLParserLONG_NUMBER {
						return "", false, false
					}
					budget.reserveWork(token.GetSymbol().GetStop() - token.GetSymbol().GetStart() + 1)
					return token.GetText(), kind == KQLParserINT_NUMBER, true
				}
			}
		case IIdentifierContext:
			token := ctx.INT_NUMBER()
			if token != nil {
				budget.reserveWork(token.GetSymbol().GetStop() - token.GetSymbol().GetStart() + 1)
				return token.GetText(), true, true
			}
		}
		tree = kqlTransparentScalarChild(tree, budget)
	}
	return "", false, false
}
func aggregateDeclarations(item IAggregationItemContext, e *conditionExtractor, budget *parseBudget) []string {
	var names []string
	if list := item.AggregationNameList(); list != nil {
		for _, child := range list.GetChildren() {
			budget.step()
			if name, ok := child.(IColumnNameContext); ok {
				names = appendTemporary(names, budget, e.authoredIdentifier(name))
			}
		}
	} else if item.ColumnName() != nil {
		names = appendTemporary(names, budget, e.authoredIdentifier(item.ColumnName()))
	}
	return names
}

// Returns complete member bases and declaration flags without mutating summaries.
func planAggregateMembers(item IAggregationItemContext, e *conditionExtractor, budget *parseBudget) (names []string, declared []bool, known bool) {
	function := aggregateCallName(item, budget)
	if function == "take_any" {
		return planTakeAnyMembers(item, e, budget)
	}
	aliases := aggregateDeclarations(item, e, budget)
	args, valid := aggregateArguments(item, budget)
	if !valid {
		return nil, nil, false
	}
	rankStart, singular, percentile := percentileFamily(function)
	count := 1
	if percentile {
		if len(args) <= rankStart || singular && len(args) != rankStart+1 {
			return nil, nil, false
		}
		count = len(args) - rankStart
		if item.AS() != nil && count > 1 {
			return nil, nil, false
		}
		// Existing simple/AS single-output aliases already prove the only
		// name. Preserve their positional shape policy without evaluating a
		// nonliteral/let rank; only expanded/default paths need literal proof.
		legacyNamedSingle := count == 1 && item.ColumnName() != nil && len(aliases) == 1
		if !legacyNamedSingle {
			for _, rank := range args[rankStart:] {
				budget.step()
				if _, _, ok := rankLiteral(rank, e, budget); !ok {
					return nil, nil, false
				}
			}
		}
	} else {
		switch function {
		case "count":
			valid = len(args) == 0
		case "sum", "avg", "min", "max", "count_distinct":
			valid = len(args) == 1
		case "dcount":
			valid = len(args) >= 1 && len(args) <= 2
		default:
			return nil, nil, false
		}
		if !valid || item.AggregationNameList() == nil {
			return nil, nil, false
		}
	}
	if len(aliases) > count {
		return nil, nil, false
	}
	budget.reserveWork(count)
	budget.reserveWork(count)
	names = make([]string, count)
	declared = make([]bool, count)
	for i := 0; i < count; i++ {
		budget.step()
		if i < len(aliases) {
			if aliases[i] == "" {
				return nil, nil, false
			}
			names[i] = aliases[i]
			declared[i] = true
			continue
		}
		if !percentile {
			return nil, nil, false
		}
		input, kind := e.aggregateExpressionResultName(args[0], budget)
		if kind == aggregateNameUnknown {
			return nil, nil, false
		}
		rank, integer, valid := rankLiteral(args[rankStart+i], e, budget)
		if !valid || !integer {
			return nil, nil, false
		}
		budget.reserveWork(len(rank))
		rank = strings.TrimLeft(rank, "0")
		if rank == "" {
			rank = "0"
		}
		budget.reserveWork(len("percentile_") + len(input) + 1 + len(rank))
		names[i] = "percentile_" + input + "_" + rank
	}
	return names, declared, true
}

func validateAggregateTuples(tree antlr.Tree, source *kqlMappedSource, owners *kqlSourceOwners, syntaxErrors bool, budget *parseBudget) string {
	present := false
	for _, occ := range source.Occurrences {
		budget.step()
		if occ.Kind == "stage" && occ.Command == "summarize" {
			present = true
			break
		}
	}
	if !present {
		return ""
	}
	if owners == nil {
		owners = associateKQLSource(tree, source, budget)
	}
	for ctx := range owners.Stages {
		budget.step()
		operator, ok := ctx.(*TabularOperatorContext)
		if !ok || operator.SummarizeOperator() == nil || operator.SummarizeOperator().AggregationList() == nil {
			continue
		}
		for _, child := range operator.SummarizeOperator().AggregationList().GetChildren() {
			budget.step()
			item, ok := child.(IAggregationItemContext)
			if !ok || item.AggregationNameList() == nil {
				continue
			}
			if syntaxErrors || item.AggregationFunction() == nil {
				return "malformed aggregation declaration list"
			}
			count := 0
			for _, node := range item.AggregationNameList().GetChildren() {
				budget.step()
				if _, bad := node.(antlr.ErrorNode); bad {
					return "recovered aggregation declaration list"
				}
				if _, ok := node.(IColumnNameContext); ok {
					count++
				}
			}
			if count == 0 {
				return "empty aggregation declaration list"
			}
			function := aggregateCallName(item, budget)
			args, valid := aggregateArguments(item, budget)
			arity := 0
			if function == "take_any" {
				arguments, proven := takeAnyResultArguments(item, owners, budget)
				if proven {
					arity = len(arguments)
				}
			}
			if start, singular, percentile := percentileFamily(function); percentile && valid && len(args) > start && (!singular || len(args) == start+1) {
				rankProof := true
				for _, rank := range args[start:] {
					budget.step()
					if _, _, proven := rankLiteral(rank, nil, budget); !proven {
						rankProof = false
						break
					}
				}
				if rankProof {
					arity = len(args) - start
				}
			} else if valid {
				switch function {
				case "count":
					if len(args) == 0 {
						arity = 1
					}
				case "sum", "avg", "min", "max", "count_distinct":
					if len(args) == 1 {
						arity = 1
					}
				case "dcount":
					if len(args) >= 1 && len(args) <= 2 {
						arity = 1
					}
				}
			}
			if arity > 0 && count > arity {
				return "too many aggregation output declarations"
			}
		}
	}
	return ""
}

func cloneAggregationResult(result *AggregationResult, budget *parseBudget) *AggregationResult {
	if result == nil {
		return nil
	}
	budget.reserveMetadataStrings(1, result.Expression)
	budget.reserveMetadata(len(result.Outputs), 0)
	budget.reserveOutput(len(result.Outputs))
	copy := *result
	if result.Outputs != nil {
		copy.Outputs = make([]AggregationOutput, len(result.Outputs))
		for i, member := range result.Outputs {
			budget.step()
			budget.reserveMetadataStrings(0, member.Column, member.BindingID)
			copy.Outputs[i] = member
		}
	}
	return &copy
}
