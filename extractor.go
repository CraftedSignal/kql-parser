package kql

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/antlr4-go/antlr/v4"
)

// MaxParseTime is the maximum time allowed for parsing a single query.
// Queries that exceed this are returned with an error.
var MaxParseTime = 5 * time.Second

const (
	portablePredicateExtractionNote = "parser-native portable predicate extraction emitted conditions"
	portableKeywordExtractionNote   = "parser-native keyword extraction emitted _keyword_ condition"
)

// Condition represents a field condition extracted from a KQL query
type Condition struct {
	Field          string   `json:"field"`
	Operator       string   `json:"operator"`
	Value          string   `json:"value"`
	ValueReference string   `json:"value_reference,omitempty"` // Variable or symbol referenced by the value expression
	Negated        bool     `json:"negated"`
	PipeStage      int      `json:"pipe_stage"`
	LogicalOp      string   `json:"logical_op"`             // "AND" or "OR" connecting to previous condition
	Alternatives   []string `json:"alternatives,omitempty"` // For OR conditions on same field
	IsComputed     bool     `json:"is_computed,omitempty"`  // True if field was created by extend/project
	SourceField    string   `json:"source_field,omitempty"` // Original field before transformation (for computed fields)
	Pipeline       int      `json:"pipeline"`               // Index in ParseResult.Pipelines of the pipeline the condition filters
}

// ParseResult contains all conditions extracted from the query
type ParseResult struct {
	Conditions          []Condition          `json:"conditions"`
	Expression          *ConditionExpression `json:"expression,omitempty"`
	DataSources         []string             `json:"data_sources,omitempty"`         // Table/source names referenced by the query
	LetStatements       []LetStatement       `json:"let_statements,omitempty"`       // KQL let variable definitions
	ComputedFields      map[string]string    `json:"computed_fields,omitempty"`      // Map of computed field name -> source field (from extend)
	ComputedExpressions map[string]string    `json:"computed_expressions,omitempty"` // Map of computed field name -> source expression
	GroupByFields       []string             `json:"group_by_fields,omitempty"`      // Fields from summarize BY clauses
	GroupByExpressions  []string             `json:"group_by_expressions,omitempty"` // Unaliased summarize BY expressions that are not plain columns, e.g. bin(TimeGenerated,1h)
	Aggregations        []Aggregation        `json:"aggregations,omitempty"`         // Aggregations computed by summarize, in query order
	Pipelines           []Pipeline           `json:"pipelines,omitempty"`            // Tabular pipelines of the query, in query order
	Commands            []string             `json:"commands,omitempty"`             // List of commands used in the query (summarize, extend, etc.)
	ProjectedFields     []string             `json:"projected_fields,omitempty"`     // Fields selected by project operators
	Joins               []JoinInfo           `json:"joins,omitempty"`
	Errors              []string             `json:"errors,omitempty"`
}

// Aggregation is one summarize aggregation and the column it outputs.
// Conditions with a higher PipeStage filter the summarized rows.
type Aggregation struct {
	Function  string `json:"function"`        // Lower-case aggregation function, e.g. count, dcount, sum
	Field     string `json:"field,omitempty"` // Aggregated column when the first argument is a plain column
	Column    string `json:"column"`          // Output column: the alias, or KQL's default name such as count_ or dcount_Field
	PipeStage int    `json:"pipe_stage"`      // Pipeline stage of the summarize operator
	Pipeline  int    `json:"pipeline"`        // Index in ParseResult.Pipelines of the pipeline it aggregates
}

// Pipeline is a tabular pipeline: the query, or a pipeline nested in
// another, such as a union input. Pipe stages count across the whole query,
// so a stage orders operators only along the chain of pipelines that feed
// each other.
type Pipeline struct {
	Parent    int    `json:"parent"`             // Enclosing pipeline, -1 for the query
	Operator  string `json:"operator,omitempty"` // How it nests: union, join, lookup or subquery; empty for the query and parenthesized sources
	PipeStage int    `json:"pipe_stage"`         // Stage of the nesting operator in Parent
	Operators int    `json:"operators"`          // Tabular operators the pipeline applies to its source
	Source    string `json:"source,omitempty"`   // Table or let name the pipeline reads, as written; empty when it reads an expression
}

// FieldProvenance indicates where a field originates relative to a join
type FieldProvenance string

const (
	ProvenanceMain      FieldProvenance = "main"
	ProvenanceJoined    FieldProvenance = "joined"
	ProvenanceJoinKey   FieldProvenance = "join_key"
	ProvenanceAmbiguous FieldProvenance = "ambiguous"
)

// JoinInfo captures the structured decomposition of a JOIN operator
type JoinInfo struct {
	Type          string       `json:"type"`                     // "inner", "leftouter", "leftanti", etc. (default: "innerunique")
	JoinFields    []string     `json:"join_fields,omitempty"`    // Fields from ON clause (simple identifiers)
	LeftFields    []string     `json:"left_fields,omitempty"`    // Left side of $left.X == $right.Y conditions
	RightFields   []string     `json:"right_fields,omitempty"`   // Right side of $left.X == $right.Y conditions
	RightTable    string       `json:"right_table,omitempty"`    // Table name if right side is a simple table reference
	Subsearch     *ParseResult `json:"subsearch,omitempty"`      // Recursively parsed right-side expression (if subquery)
	PipeStage     int          `json:"pipe_stage"`               // Pipeline stage where join appears
	ExposedFields []string     `json:"exposed_fields,omitempty"` // Fields the right side makes available
}

// LetStatement captures a KQL let variable definition.
type LetStatement struct {
	Name       string `json:"name"`
	Expression string `json:"expression"`
}

// KQL keywords that should be excluded from conditions
// These are metadata fields or table names, not actual data fields
// Note: We don't exclude aggregation function names like "count", "sum" etc.
// because they can also be valid field names (e.g., "Count > 5" after summarize)
var kqlKeywords = map[string]bool{
	// Common table names
	"securityevent": true, "signinlogs": true, "auditlogs": true,
	"deviceevents": true, "deviceprocessevents": true, "devicenetworkevents": true,
	"devicefileevents": true, "deviceregistryevents": true, "devicelogonevent": true,
	"commoncomputerenvironment": true, "aaboracletable": true,

	// Time-related (these are typically metadata, not user data)
	"timegenerated": true, "timestamp": true, "ingestiontime": true,

	// Operators/keywords (these shouldn't appear as field names in conditions)
	"where": true, "project": true, "extend": true, "summarize": true,
	"join": true, "union": true, "let": true, "datatable": true,
	"by": true, "on": true, "with": true, "and": true, "or": true, "not": true,
	"in": true, "between": true,
}

// conditionExtractor walks the parse tree to extract conditions
type conditionExtractor struct {
	*BaseKQLParserListener
	conditions          []Condition
	computedFields      map[string]string // Fields created by extend/project: computed field -> source field
	computedExpressions map[string]string // Fields created by extend/project: computed field -> expression
	groupByFields       []string          // Fields from summarize BY clauses
	groupByExpressions  []string          // Unaliased summarize BY expressions that are not plain columns
	aggregations        []Aggregation     // Aggregations computed by summarize
	pipelines           []Pipeline        // Tabular pipelines, in query order
	pipelineStack       []int             // Pipelines enclosing the current tree position
	operatorStages      []int             // Stages of the tabular operators enclosing the current tree position
	queryPipeline       int               // The query's own pipeline, or -1
	commands            []string          // Commands used in the query
	projectedFields     []string          // Fields selected by project operators
	joins               []JoinInfo
	currentStage        int
	inSubquery          int // depth of subquery nesting
	inFunctionCall      int // depth of function call nesting (countif, sumif, etc.)
	negated             bool
	lastLogicalOp       string
	errors              []string
	originalQuery       string // normalized query text for extracting subexpressions
}

// errorListener collects parse errors
type errorListener struct {
	*antlr.DefaultErrorListener
	errors []string
}

func (l *errorListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
	l.errors = append(l.errors, msg)
}

// normalizeQuery preprocesses a KQL query to normalize operators and functions that the parser doesn't handle well.
// This converts case-sensitive operators and special functions to their standard forms for parsing purposes.
func normalizeQuery(query string) string {
	return normalize(query, false)
}

// normalize rewrites constructs the grammar does not support into ones it
// does. Unless strict, it also rescues text that is not a single query: a
// write-up before the query, escaped newlines, workbook parameters, a second
// query after the first, or a query that starts with an operator.
func normalize(query string, strict bool) string {
	normalized := query

	// Remove line continuation backslashes (backslash followed by newline)
	// Common in PowerShell-style queries copied from scripts
	normalized = strings.ReplaceAll(normalized, "\\\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\\\r\n", "\r\n")

	// Fix escaped newlines from JSON extraction - but only outside of string literals
	// This handles queries extracted from YAML/JSON where \r\n became literal characters
	if !strict {
		normalized = normalizeEscapeSequences(normalized)
	}

	// Normalize actual UTF-8 whitespace runes that the lexer does not treat as WS.
	// Sentinel exports occasionally contain NBSP after query text.
	normalized = normalizeUnicodeWhitespace(normalized)

	// Strip documentation preambles like "Description:...Query:..."
	// These are common in documentation templates where the actual query follows "Query:"
	if !strict {
		normalized = stripDocumentationPreamble(normalized)
	}

	// Strip leading comments to get to the actual query
	normalized = stripLeadingComments(normalized, strict)

	// Strip declare query_parameters(...) statements
	// These define query parameters but aren't needed for condition extraction
	normalized = stripDeclareStatements(normalized)

	// Inline lets that bind constants before later steps rewrite their
	// literals, so a condition reads a let's value as an inline literal.
	normalized = inlineLetConstants(normalized, extractLetStatements(normalized))

	// Replace parameterized placeholders with dummy values
	// e.g., {TimeRange} -> 1d, {SourceTable} -> DummyTable
	if !strict {
		normalized = replaceParameters(normalized)
	}

	// Convert ASIM functions to dummy table names (these are parser functions that return tables)
	// Handle both simple calls (imAuthentication) and parameterized calls (_Im_Dns(param=value))
	normalized = replaceASIMFunctions(normalized)

	// Handle externaldata operator by replacing with dummy table
	normalized = replaceExternalData(normalized)

	// Handle datatable with inline data by replacing with dummy table
	normalized = replaceDatatableWithData(normalized)

	// Handle arg() cross-workspace function: arg("...").Table -> Table
	// Azure Resource Graph queries use arg("sub-id").Resources pattern
	normalized = replaceArgFunction(normalized)

	// Handle materialize by stripping it (handles both with and without space)
	normalized = strings.ReplaceAll(normalized, "materialize(", "(")
	normalized = strings.ReplaceAll(normalized, "materialize (", "(")
	normalized = strings.ReplaceAll(normalized, "materialize  (", "(")

	// Rename reserved keywords used as field names
	// "pattern" is a reserved word in the grammar but used as a field name in some queries
	normalized = renameReservedFieldNames(normalized)

	// Convert make_set and make_list aliases used by existing parser fixtures.
	// Handle both make_set( and make_set ( with space.
	normalized = strings.ReplaceAll(normalized, "make_set(", "makeset(")
	normalized = strings.ReplaceAll(normalized, "make_set (", "makeset(")
	normalized = strings.ReplaceAll(normalized, "make_list(", "makelist(")
	normalized = strings.ReplaceAll(normalized, "make_list (", "makelist(")

	// Normalize string operator aliases commonly found in Sentinel content.
	normalized = normalizeStringOperatorAliases(normalized)

	// Handle summarize without aggregation (summarize by x -> summarize count() by x)
	// The grammar requires aggregation before BY, but KQL allows just "summarize by"
	normalized = normalizeSummarizeBy(normalized)

	// Convert SQL-style <> to KQL != (handle all spacing variations)
	normalized = normalizeNotEqualOperator(normalized)

	// Normalize timespan literals without space: 0min -> 0 min, 30min -> 30 min
	// The lexer expects space or recognizes combined form only for certain patterns
	normalized = normalizeTimespanLiterals(normalized)
	normalized = normalizeDatetimeLiterals(normalized)
	normalized = joinAdjacentStringLiterals(normalized)
	normalized = normalizeMakeGraphEdges(normalized)
	normalized = stripGraphMatchStages(normalized)

	// Normalize trailing decimals: 1000. -> 1000.0
	// The grammar requires at least one digit after the decimal point
	normalized = normalizeTrailingDecimals(normalized)

	// Normalize \0 escape sequences to \x00 (null character)
	// The grammar only supports \x hex escapes, not bare \0
	normalized = normalizeNullEscapes(normalized)

	// Convert operator aliases to standard forms
	normalized = strings.ReplaceAll(normalized, "| mvexpand ", "| mv-expand ")
	normalized = strings.ReplaceAll(normalized, "\nmvexpand ", "\nmv-expand ")
	normalized = strings.ReplaceAll(normalized, "| mvapply ", "| mv-apply ")

	// Convert "filter" to "where" (filter is a legacy alias)
	normalized = strings.ReplaceAll(normalized, "| filter ", "| where ")
	normalized = strings.ReplaceAll(normalized, "\nfilter ", "\nwhere ")

	// Strip kind= from lookup operator (grammar doesn't support it)
	normalized = strings.ReplaceAll(normalized, "lookup kind=leftouter ", "lookup ")
	normalized = strings.ReplaceAll(normalized, "lookup kind=inner ", "lookup ")
	normalized = strings.ReplaceAll(normalized, "lookup kind=rightouter ", "lookup ")
	normalized = strings.ReplaceAll(normalized, "lookup kind=fullouter ", "lookup ")

	// Normalize lookup subqueries: lookup (union ...) -> lookup (LookupTable)
	// The grammar doesn't support complex expressions inside lookup
	normalized = normalizeLookupSubquery(normalized)

	// Normalize make-series: convert "in range(...)" to "from ... to ... step ..."
	// and add default step if missing
	normalized = normalizeMakeSeries(normalized)

	// Normalize join kind aliases (grammar supports leftanti/rightsemi but not combined forms)
	normalized = strings.ReplaceAll(normalized, "kind=leftantisemi", "kind=leftanti")
	normalized = strings.ReplaceAll(normalized, "kind=rightantisemi", "kind=rightanti")
	normalized = strings.ReplaceAll(normalized, "kind=leftsemijoin", "kind=leftsemi")
	normalized = strings.ReplaceAll(normalized, "kind=rightsemijoin", "kind=rightsemi")

	// Normalize join condition 'and' to comma (grammar expects comma-separated conditions)
	// on $left.A == $right.B and $left.C == $right.D -> on $left.A == $right.B, $left.C == $right.D
	normalized = joinConditionAndRe.ReplaceAllString(normalized, ", $$$1.")

	// Strip all hint.xxx=value patterns (hint.strategy=broadcast, hint.shufflekey=x, etc.)
	// The grammar doesn't support these join hints
	normalized = stripJoinHints(normalized)

	// Strip mv-apply subqueries: mv-apply x on (subquery) -> mv-apply x
	// The subquery isn't needed for condition extraction
	normalized = stripMvApplySubquery(normalized)

	// Strip parse statements: parse field with pattern -> (removed)
	// The parse operator extracts substrings but isn't needed for condition extraction
	normalized = stripParseStatements(normalized)

	// Strip return type annotations from evaluate: evaluate func(x) : (col:type) -> evaluate func(x)
	normalized = stripReturnTypeAnnotations(normalized)

	// Normalize dot-bracket pattern: obj.[0] -> obj[0], obj.["key"] -> obj["key"]
	// This unusual KQL syntax isn't handled by our grammar
	normalized = replaceAllOutsideStrings(normalized, ".[", "[")

	// Convert bracket property access to dot notation
	// obj['key'] -> obj._key_ (the lexer's QUOTED_IDENTIFIER conflicts with bracket access)
	normalized = convertBracketAccess(normalized)

	// Reduce "field in (Tabular | project X)" to a scalar placeholder so
	// expression parsing can continue. Portable extraction still sees the
	// original query for table and predicate discovery.
	normalized = normalizeInSubqueries(normalized)

	// Fix identifiers that start with numbers by prepending underscore
	// e.g., 3plogTime -> _3plogTime
	normalized = fixNumericIdentifiers(normalized)

	// Convert tuple unpacking to single assignment
	// (a, b, c) = func() -> _tuple_result = func()
	normalized = convertTupleUnpacking(normalized)

	// Extract main query from let statements (parse only the final query for conditions)
	// This MUST happen before union normalization so (union ...) after lets is handled
	if strict {
		normalized = normalized[skipLetStatements(normalized):]
	} else {
		normalized = extractMainQuery(normalized)
		normalized = rescueOperatorFirstQuery(normalized)
	}

	// Strip function parameters in union statements (grammar doesn't support them)
	// union Func('a'), Func2('b') -> union Func, Func2
	normalized = stripUnionFunctionParams(normalized)

	// Handle union with withsource and isfuzzy parameters
	// union withsource=... -> union
	// union isfuzzy=true -> union
	// Also handles (union ...) wrapped queries
	normalized = normalizeUnionParameters(normalized)

	// Handle queries that start with union (grammar requires union to follow a table reference)
	if hasKeywordPrefix(strings.TrimLeft(strings.ToLower(normalized), " \t\r\n"), "union") {
		normalized = "DummyTable | " + normalized
	}

	// Handle union inside join parentheses: join (union ...) -> join (DummyTable | union ...)
	// The grammar requires a tabularSource before union, even inside join
	normalized = normalizeJoinUnion(normalized)
	normalized = normalizeBracketedUnion(normalized)

	// Handle find operator: find in (Table1, Table2, ...) where ... -> extract conditions from where clause
	// The find operator isn't in the grammar, so convert to table + where
	normalized = normalizeFindOperator(normalized)

	// Handle search operator: search in (Table1, Table2) "pattern" -> simplified form
	// The search operator needs special handling
	normalized = normalizeSearchOperator(normalized)

	// Normalize top-nested operator: top-nested N of X by count() -> summarize count() by X
	// The grammar doesn't support the "of" keyword in top-nested
	normalized = normalizeTopNested(normalized)

	// Normalize partition bodies that start with an operator rather than a table
	// source, e.g. `partition by aid (top 1 by TimeGenerated desc)`.
	normalized = normalizePartitionOperator(normalized)

	// Strip named parameters from function calls: func(param=value) -> func()
	// These are common in user-defined function calls like parser(pack=true)
	normalized = stripNamedFunctionParams(normalized)

	// Normalize distinct clauses: distinct a, tostring(b) -> distinct a, b
	// The grammar doesn't support function calls in distinct column lists
	normalized = normalizeDistinctColumns(normalized)

	// Strip render operator parameters: render areachart kind=stacked -> render areachart
	// The grammar doesn't support all render parameter forms
	normalized = stripRenderParameters(normalized)

	// Strip trailing semicolons and comments (common in saved queries and function definitions)
	// Must handle cases like "| where x == 1; // comment" -> "| where x == 1"
	normalized = stripTrailingSemicolonsAndComments(normalized)

	return normalized
}

// rescueOperatorFirstQuery gives a query that starts with a pipe (workbook
// queries) or an operator (Azure Resource Graph queries start with "where",
// workbook queries with "extend") a placeholder table to filter.
func rescueOperatorFirstQuery(query string) string {
	query = strings.TrimLeft(query, " \t")
	if strings.HasPrefix(query, "|") {
		return "DummyTable " + query
	}
	lower := strings.ToLower(query)
	for _, operator := range []string{"where", "extend", "project", "summarize"} {
		if strings.HasPrefix(lower, operator) && len(lower) > len(operator) && strings.ContainsRune(" \t\n", rune(lower[len(operator)])) {
			return "DummyTable | " + query
		}
	}
	return query
}

// skipLetStatements returns the index where the query after its leading let
// statements and comments starts.
func skipLetStatements(query string) int {
	i := 0
	n := len(query)
	for i < n {
		// Skip whitespace and newlines
		for i < n && (query[i] == ' ' || query[i] == '\t' || query[i] == '\n' || query[i] == '\r') {
			i++
		}
		if i >= n {
			break
		}

		// Check for line comment
		if i+1 < n && query[i] == '/' && query[i+1] == '/' {
			// Skip to end of line
			for i < n && query[i] != '\n' {
				i++
			}
			continue
		}

		// Check for let statement
		if i+4 <= n && strings.ToLower(query[i:i+4]) == "let " {
			// Skip the let statement entirely
			i = skipLetStatement(query, i)
			continue
		}

		// Not a let statement, this is the start of the main query
		break
	}
	return i
}

// extractMainQuery extracts the main query from a let statement sequence
// For "let x = ...; let y = ...; Table | where ...", returns "Table | where ..."
func extractMainQuery(query string) string {
	n := len(query)
	i := skipLetStatements(query)
	if i >= n {
		return query // No main query found
	}

	// Find where this main query ends (before any subsequent query)
	mainQueryStart := i
	mainQueryEnd := n

	// Scan to find a new query starting after blank lines
	// A new query starts with: let statement, or table name (after optional comments)
	// outside any brackets: blank lines inside a subquery do not end the query.
	j := i
	depth := 0
	for j < n {
		// Skip whitespace
		wsStart := j
		for j < n && (query[j] == ' ' || query[j] == '\t') {
			j++
		}

		// Check for newlines (two or more blank lines indicate possible new query)
		nlCount := 0
		for j < n && (query[j] == '\n' || query[j] == '\r') {
			if query[j] == '\n' {
				nlCount++
			}
			j++
		}

		// After blank lines, check what comes next
		if nlCount >= 2 && depth == 0 {
			// Skip any comments and whitespace
			checkPos := j
			for checkPos < n {
				// Skip whitespace
				for checkPos < n && (query[checkPos] == ' ' || query[checkPos] == '\t' || query[checkPos] == '\n' || query[checkPos] == '\r') {
					checkPos++
				}
				// Check for comment
				if checkPos+1 < n && query[checkPos] == '/' && query[checkPos+1] == '/' {
					// Skip comment line
					for checkPos < n && query[checkPos] != '\n' {
						checkPos++
					}
					continue
				}
				break
			}

			// Check if it's a let statement
			if checkPos+4 <= n && strings.ToLower(query[checkPos:checkPos+4]) == "let " {
				mainQueryEnd = wsStart
				break
			}

			// Check if it's a table name (identifier starting a new query)
			// Table names start with a letter and are followed by pipe or newline
			if checkPos < n && (query[checkPos] >= 'A' && query[checkPos] <= 'Z' || query[checkPos] >= 'a' && query[checkPos] <= 'z') {
				// Read the identifier
				identEnd := checkPos
				for identEnd < n && isIdentChar(query[identEnd]) {
					identEnd++
				}
				// Skip whitespace
				for identEnd < n && (query[identEnd] == ' ' || query[identEnd] == '\t') {
					identEnd++
				}
				// Check if followed by | or newline (indicates table name starting query)
				if identEnd < n && (query[identEnd] == '|' || query[identEnd] == '\n' || query[identEnd] == '\r') {
					mainQueryEnd = wsStart
					break
				}
			}
		}

		// Skip to next line
		if nlCount == 0 {
			j = skipLineTrackingDepth(query, j, &depth)
		}
	}

	return query[mainQueryStart:mainQueryEnd]
}

// skipLineTrackingDepth advances from i to the end of the line, or past a
// string literal that spans lines, and updates depth for the brackets outside
// strings and comments. It returns the index of the newline or len(query).
func skipLineTrackingDepth(query string, i int, depth *int) int {
	for i < len(query) && query[i] != '\n' {
		switch c := query[i]; {
		case c == '/' && i+1 < len(query) && query[i+1] == '/':
			for i < len(query) && query[i] != '\n' {
				i++
			}
			return i
		case isStringLiteralStart(query, i):
			end := stringLiteralEnd(query, i)
			if end < 0 {
				for i < len(query) && query[i] != '\n' {
					i++
				}
				return i
			}
			i = end
			continue
		case c == '(' || c == '[' || c == '{':
			*depth++
		case (c == ')' || c == ']' || c == '}') && *depth > 0:
			*depth--
		}
		i++
	}
	return i
}

// skipLetStatement skips a let statement starting at position i and returns position after the semicolon
func skipLetStatement(query string, i int) int {
	n := len(query)

	// Skip past "let "
	i += 4

	// Find the semicolon that ends this let statement, tracking nesting
	parenDepth := 0
	braceDepth := 0
	inString := false
	stringChar := byte(0)
	isVerbatim := false // Tracks if current string is a verbatim string (@"..." or @'...')

	for i < n {
		c := query[i]

		// Handle string literals
		if !inString && (c == '"' || c == '\'') {
			// Check for verbatim string prefix (@)
			isVerbatim = i > 0 && query[i-1] == '@'
			inString = true
			stringChar = c
			i++
			continue
		}

		if inString {
			if c == stringChar {
				if isVerbatim {
					// Verbatim strings: backslash doesn't escape, so any closing quote ends the string
					inString = false
				} else if stringChar == '"' {
					// Regular double-quoted strings: check if escaped
					backslashes := 0
					for j := i - 1; j >= 0 && query[j] == '\\'; j-- {
						backslashes++
					}
					if backslashes%2 == 0 {
						inString = false
					}
				} else {
					// Single-quoted strings: no backslash escaping in KQL
					inString = false
				}
			}
			i++
			continue
		}

		// Handle line comments
		if c == '/' && i+1 < n && query[i+1] == '/' {
			// Skip to end of line
			for i < n && query[i] != '\n' {
				i++
			}
			continue
		}

		// Track nesting
		switch c {
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case ';':
			// Semicolon outside of all nesting ends the let statement
			if parenDepth <= 0 && braceDepth <= 0 {
				return i + 1
			}
		}

		i++
	}

	return n
}

// convertBracketAccess converts bracket notation to dot notation where possible
// Handles: obj['key'] -> obj.key, obj["key"] -> obj.key, array[0] -> array._0
// Also handles: ['Column Name'] = expr -> _Column_Name_ = expr (column aliases with special chars)
// This works around the lexer's QUOTED_IDENTIFIER rule that captures [...]
func convertBracketAccess(query string) string {
	var result strings.Builder
	i := 0
	inString := false
	stringChar := byte(0)
	isVerbatim := false

	for i < len(query) {
		c := query[i]

		// Skip line comments (// ...) - important to avoid false string detection in comments
		if !inString && c == '/' && i+1 < len(query) && query[i+1] == '/' {
			// Write the comment as-is until end of line
			for i < len(query) && query[i] != '\n' {
				result.WriteByte(query[i])
				i++
			}
			continue
		}

		// Track string literals to skip brackets inside them
		if !inString && (c == '"' || c == '\'') {
			// Check for verbatim string (@"..." or @'...')
			isVerbatim = i > 0 && query[i-1] == '@'
			inString = true
			stringChar = c
			result.WriteByte(c)
			i++
			continue
		}

		if inString {
			result.WriteByte(c)
			if c == stringChar {
				if isVerbatim {
					// Verbatim strings: no escape characters, closing quote ends the string
					inString = false
				} else if stringChar == '"' {
					// Regular double-quoted strings have backslash escaping
					backslashes := 0
					for j := i - 1; j >= 0 && query[j] == '\\'; j-- {
						backslashes++
					}
					if backslashes%2 == 0 {
						inString = false
					}
				} else {
					// Single-quoted strings don't use backslash escaping
					inString = false
				}
			}
			i++
			continue
		}

		// Look for [ pattern (only outside strings)
		if c == '[' && i+1 < len(query) {
			// Check if preceded by an identifier (property access) or standalone (column reference)
			precededByIdent := i > 0 && (isIdentChar(query[i-1]) || query[i-1] == ')')

			// Case 1: ['...'] or ["..."] - quoted property access or column alias
			if query[i+1] == '\'' || query[i+1] == '"' {
				quote := query[i+1]
				// Find the closing quote and bracket
				end := i + 2
				for end < len(query) && query[end] != quote {
					end++
				}
				if end < len(query) && end+1 < len(query) && query[end] == quote && query[end+1] == ']' {
					// Extract the key
					key := query[i+2 : end]
					// Check if key is a valid identifier (alphanumeric + underscore
					// only) the grammar does not read as a keyword
					if isValidPropertyKey(key) && !grammarReservedWords[strings.ToLower(key)] {
						if precededByIdent {
							result.WriteByte('.')
						}
						result.WriteString(key)
						i = end + 2
						continue
					}
					// Key has special characters (like $) - sanitize it
					// Check if this is a column alias pattern: ['special name'] = expr
					// Look ahead for = sign
					afterBracket := end + 2
					for afterBracket < len(query) && (query[afterBracket] == ' ' || query[afterBracket] == '\t') {
						afterBracket++
					}
					if afterBracket < len(query) && query[afterBracket] == '=' &&
						(afterBracket+1 >= len(query) || query[afterBracket+1] != '=') {
						// This is a column alias with special characters
						// Convert to valid identifier by replacing non-alphanumeric chars
						sanitized := sanitizeIdentifier(key)
						result.WriteString(sanitized)
						i = end + 2
						continue
					}
					// Property access with special characters in key (like obj['$kind'])
					// Sanitize the key name
					sanitized := sanitizeIdentifier(key)
					if precededByIdent {
						result.WriteByte('.')
					}
					result.WriteString(sanitized)
					i = end + 2
					continue
				}
			}

			// Case 2: [0], [1], [-1], [-2], etc. - numeric array index (positive or negative)
			startIdx := i + 1
			if startIdx < len(query) && query[startIdx] == '-' {
				startIdx++ // Skip the minus sign
			}
			if startIdx < len(query) && query[startIdx] >= '0' && query[startIdx] <= '9' {
				// Find the end of the number
				end := startIdx
				for end < len(query) && query[end] >= '0' && query[end] <= '9' {
					end++
				}
				// Check if it ends with ]
				if end < len(query) && query[end] == ']' {
					// Convert [0] to ._idx0 or _idx0 depending on context
					if precededByIdent {
						result.WriteByte('.')
					}
					result.WriteString("_idx")
					if query[i+1] == '-' {
						result.WriteByte('n') // n for negative
						result.WriteString(query[i+2 : end])
					} else {
						result.WriteString(query[i+1 : end])
					}
					i = end + 1
					continue
				}
			}

			// Case 3: [variable] or [function()] - dynamic index expression
			// Find the matching ] considering nested brackets and parens
			depth := 1
			end := i + 1
			inStr := false
			strCh := byte(0)
			for end < len(query) && depth > 0 {
				c := query[end]
				if !inStr {
					if c == '"' || c == '\'' {
						inStr = true
						strCh = c
					} else if c == '[' {
						depth++
					} else if c == ']' {
						depth--
					}
				} else {
					if c == strCh && (end == 0 || query[end-1] != '\\') {
						inStr = false
					}
				}
				end++
			}
			if depth == 0 && end > i+2 {
				// Replace entire [...] with ._dyn or _dyn depending on context
				if precededByIdent {
					result.WriteByte('.')
				}
				result.WriteString("_dyn")
				i = end
				continue
			}
		}
		result.WriteByte(query[i])
		i++
	}

	return result.String()
}

// fixNumericIdentifiers fixes identifiers that start with a digit by prepending underscore
// This handles cases like: 3plogTime -> _3plogTime, 3p_observed_Time -> _3p_observed_Time
// KQL grammar requires identifiers to start with letter or underscore
func fixNumericIdentifiers(query string) string {
	var result strings.Builder
	i := 0
	inString := false
	stringChar := byte(0)

	for i < len(query) {
		c := query[i]

		// Skip line comments
		if !inString && c == '/' && i+1 < len(query) && query[i+1] == '/' {
			for i < len(query) && query[i] != '\n' {
				result.WriteByte(query[i])
				i++
			}
			continue
		}

		// Track strings
		if !inString && (c == '"' || c == '\'') {
			inString = true
			stringChar = c
			result.WriteByte(c)
			i++
			continue
		}

		if inString {
			result.WriteByte(c)
			if c == stringChar {
				if stringChar == '"' {
					backslashes := 0
					for j := i - 1; j >= 0 && query[j] == '\\'; j-- {
						backslashes++
					}
					if backslashes%2 == 0 {
						inString = false
					}
				} else {
					inString = false
				}
			}
			i++
			continue
		}

		// Copy datetime literals, whose time part (2026-02-04T10:00) starts
		// with digits followed by letters, unchanged
		if loc := datetimeLiteralRe.FindStringIndex(query[i:]); loc != nil && loc[0] == 0 && (i == 0 || !isIdentChar(query[i-1])) {
			result.WriteString(query[i : i+loc[1]])
			i += loc[1]
			continue
		}

		// Check for identifier starting with digit
		// Only consider it an identifier if preceded by non-identifier char
		if c >= '0' && c <= '9' {
			// Check if this could be the start of an identifier
			// (preceded by space, newline, tab, =, comma, or operator char)
			if i == 0 || !isIdentChar(query[i-1]) {
				// Check if followed by letters (making it an identifier, not just a number)
				j := i + 1
				for j < len(query) && (query[j] >= '0' && query[j] <= '9') {
					j++
				}
				// Now check if there are letters after the digits
				if j < len(query) && ((query[j] >= 'a' && query[j] <= 'z') || (query[j] >= 'A' && query[j] <= 'Z') || query[j] == '_') {
					// Check if this is a time literal (e.g., 1d, 5m, 30s, 2h)
					// Time literals have exactly one letter at the end (d, h, m, s, ms, us, ns)
					endOfIdent := j
					for endOfIdent < len(query) && isIdentChar(query[endOfIdent]) {
						endOfIdent++
					}
					suffix := query[j:endOfIdent]
					isTimeLiteral := suffix == "d" || suffix == "h" || suffix == "m" || suffix == "s" ||
						suffix == "ms" || suffix == "us" || suffix == "ns" ||
						suffix == "tick" || suffix == "ticks" ||
						suffix == "minute" || suffix == "minutes" ||
						suffix == "hour" || suffix == "hours" ||
						suffix == "day" || suffix == "days" ||
						suffix == "second" || suffix == "seconds"

					if !isTimeLiteral {
						// This is an identifier starting with digits, prepend underscore
						result.WriteByte('_')
					}
				}
			}
		}

		result.WriteByte(c)
		i++
	}

	return result.String()
}

// convertTupleUnpacking converts tuple unpacking syntax to simple assignment
// (a, b, c) = func() -> _tuple_result = func()
// This handles patterns like: extend (Anomalies, Score, Baseline) = series_decompose_anomalies(...)
func convertTupleUnpacking(query string) string {
	var result strings.Builder
	i := 0

	for i < len(query) {
		// Look for pattern: whitespace or beginning + ( + identifiers with commas + ) + whitespace + =
		if query[i] == '(' {
			// Check if this looks like tuple unpacking
			// Find the closing paren
			end := i + 1
			parenDepth := 1
			for end < len(query) && parenDepth > 0 {
				if query[end] == '(' {
					parenDepth++
				} else if query[end] == ')' {
					parenDepth--
				}
				end++
			}

			if parenDepth == 0 && end < len(query) {
				// Check if what's inside looks like identifiers with commas
				inside := query[i+1 : end-1]
				if looksLikeTupleUnpacking(inside) {
					// Find the = sign after the closing paren
					eqIdx := end
					for eqIdx < len(query) && (query[eqIdx] == ' ' || query[eqIdx] == '\t') {
						eqIdx++
					}
					if eqIdx < len(query) && query[eqIdx] == '=' && (eqIdx+1 >= len(query) || query[eqIdx+1] != '=') {
						// This is tuple unpacking, replace with simple assignment
						result.WriteString("_tuple_result")
						i = end
						continue
					}
				}
			}
		}
		result.WriteByte(query[i])
		i++
	}

	return result.String()
}

// looksLikeTupleUnpacking checks if content looks like "identifier, identifier, ..."
func looksLikeTupleUnpacking(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return false
	}

	// Must contain at least one comma
	if !strings.Contains(s, ",") {
		return false
	}

	// Split by comma and check each part is an identifier
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 {
			return false
		}
		// Check if it's a valid identifier
		for i, c := range part {
			if i == 0 {
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
					return false
				}
			} else {
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
					return false
				}
			}
		}
	}
	return true
}

// isValidPropertyKey checks if a string can be used as a property key in dot notation
// Note: We don't allow $ because it's not valid in our KQL grammar identifiers
func isValidPropertyKey(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, c := range s {
		if i == 0 {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
				return false
			}
		} else {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}

// sanitizeIdentifier converts a string with special characters to a valid identifier
// e.g., "DataVolume(Bytes)" -> "_DataVolume_Bytes_", "Column Name" -> "_Column_Name_"
func sanitizeIdentifier(s string) string {
	var result strings.Builder
	result.WriteByte('_') // Start with underscore to ensure valid identifier
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result.WriteRune(c)
		} else {
			result.WriteByte('_')
		}
	}
	result.WriteByte('_') // End with underscore
	return result.String()
}

// normalizeEscapeSequences converts literal \r\n to actual newlines and \uXXXX to Unicode chars, but only outside strings
func normalizeEscapeSequences(query string) string {
	var result strings.Builder
	inString := false
	stringChar := byte(0)
	i := 0

	for i < len(query) {
		c := query[i]

		// Track string literal boundaries
		if !inString && (c == '"' || c == '\'') {
			// Check for verbatim string (@"..." or @'...')
			if i > 0 && query[i-1] == '@' {
				// Already wrote @, now in verbatim string
			}
			inString = true
			stringChar = c
			result.WriteByte(c)
			i++
			continue
		}

		if inString {
			if c == stringChar {
				// Check if it's escaped
				backslashes := 0
				for j := i - 1; j >= 0 && query[j] == '\\'; j-- {
					backslashes++
				}
				if backslashes%2 == 0 {
					// Not escaped, end of string
					inString = false
				}
			}
			result.WriteByte(c)
			i++
			continue
		}

		// Outside strings, convert escape sequences
		if c == '\\' && i+1 < len(query) {
			next := query[i+1]
			switch next {
			case 'n':
				result.WriteByte('\n')
				i += 2
				continue
			case 'r':
				result.WriteByte('\r')
				i += 2
				continue
			case 't':
				result.WriteByte('\t')
				i += 2
				continue
			case 'u':
				// Handle Unicode escape sequence: \uXXXX
				if i+5 < len(query) {
					hex := query[i+2 : i+6]
					if isValidHex(hex) {
						i += 6
						handleUnicodeEscape(hex, &result)
						continue
					}
				}
			case '\\':
				// Handle double backslash followed by uXXXX: \\uXXXX
				if i+6 < len(query) && query[i+2] == 'u' {
					hex := query[i+3 : i+7]
					if isValidHex(hex) {
						i += 7
						handleUnicodeEscape(hex, &result)
						continue
					}
				}
			}
		}

		result.WriteByte(c)
		i++
	}

	return result.String()
}

// normalizeUnicodeWhitespace converts non-ASCII whitespace outside string
// literals to regular spaces so the lexer does not emit ERROR_CHAR tokens.
func normalizeUnicodeWhitespace(query string) string {
	var result strings.Builder
	result.Grow(len(query))

	inString := false
	stringChar := byte(0)
	isVerbatim := false

	for i := 0; i < len(query); {
		c := query[i]

		if !inString && c == '/' && i+1 < len(query) && query[i+1] == '/' {
			for i < len(query) && query[i] != '\n' {
				result.WriteByte(query[i])
				i++
			}
			continue
		}

		if !inString && c == '/' && i+1 < len(query) && query[i+1] == '*' {
			result.WriteByte(c)
			result.WriteByte(query[i+1])
			i += 2
			for i < len(query) {
				result.WriteByte(query[i])
				if query[i] == '*' && i+1 < len(query) && query[i+1] == '/' {
					i++
					result.WriteByte(query[i])
					i++
					break
				}
				i++
			}
			continue
		}

		if !inString && (c == '"' || c == '\'') {
			isVerbatim = i > 0 && query[i-1] == '@'
			inString = true
			stringChar = c
			result.WriteByte(c)
			i++
			continue
		}

		if inString {
			result.WriteByte(c)
			if c == stringChar {
				if isVerbatim {
					if i+1 < len(query) && query[i+1] == c {
						i++
						result.WriteByte(query[i])
					} else {
						inString = false
						isVerbatim = false
					}
				} else if stringChar == '"' {
					backslashes := 0
					for j := i - 1; j >= 0 && query[j] == '\\'; j-- {
						backslashes++
					}
					if backslashes%2 == 0 {
						inString = false
					}
				} else {
					inString = false
				}
			}
			i++
			continue
		}

		if c < utf8.RuneSelf {
			result.WriteByte(c)
			i++
			continue
		}

		r, size := utf8.DecodeRuneInString(query[i:])
		if r == utf8.RuneError && size == 1 {
			result.WriteByte(c)
			i++
			continue
		}

		switch {
		case r == 0x200b || r == 0x200c || r == 0x200d || r == 0x2060 || r == 0xfeff:
			// Drop zero-width formatting characters outside string literals.
		case unicode.IsSpace(r):
			result.WriteByte(' ')
		default:
			result.WriteRune(r)
		}
		i += size
	}

	return result.String()
}

// isValidHex checks if a 4-character string is valid hexadecimal
func isValidHex(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// handleUnicodeEscape converts a 4-digit hex code to appropriate output
func handleUnicodeEscape(hex string, result *strings.Builder) {
	// Convert hex to rune
	var codepoint rune
	for _, h := range hex {
		codepoint *= 16
		if h >= '0' && h <= '9' {
			codepoint += rune(h - '0')
		} else if h >= 'a' && h <= 'f' {
			codepoint += rune(h - 'a' + 10)
		} else if h >= 'A' && h <= 'F' {
			codepoint += rune(h - 'A' + 10)
		}
	}
	// Replace certain problematic Unicode chars with space or skip them
	switch codepoint {
	case 0x200b, 0x200c, 0x200d, 0x2060, 0xfeff: // zero-width chars
		// Skip these entirely
	case 0x202f, 0x00a0: // narrow no-break space, no-break space
		result.WriteByte(' ')
	case 0x201c, 0x201d: // curly double quotes
		result.WriteByte('"')
	case 0x2018, 0x2019: // curly single quotes
		result.WriteByte('\'')
	default:
		// Write the actual Unicode character
		result.WriteRune(codepoint)
	}
}

// normalizeSummarizeBy adds a dummy aggregation when summarize has only BY clause
// KQL allows "summarize by x" but grammar requires aggregation before BY
func normalizeSummarizeBy(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query) + 32)
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		idx := indexKeyword(lowerQuery, "summarize", searchFrom)
		if idx == -1 {
			break
		}

		afterSummarize := idx + len("summarize")
		byStart := afterSummarize
		for byStart < len(query) && (query[byStart] == ' ' || query[byStart] == '\t' || query[byStart] == '\n' || query[byStart] == '\r') {
			byStart++
		}

		if hasKeywordPrefix(lowerQuery[byStart:], "by") {
			b.WriteString(query[lastCopied:byStart])
			b.WriteString("count() ")
			lastCopied = byStart
			changed = true
		}

		searchFrom = afterSummarize
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// stripReturnTypeAnnotations removes return type annotations from evaluate statements
// evaluate func(x) : (col:type, ...) -> evaluate func(x)
func stripReturnTypeAnnotations(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		// Find "evaluate"
		rel := strings.Index(lowerQuery[searchFrom:], "evaluate")
		if rel == -1 {
			break
		}
		idx := searchFrom + rel

		// Find the closing paren of the evaluate function call
		evalSearchStart := idx + 8
		parenStart := -1
		for i := evalSearchStart; i < len(query); i++ {
			if query[i] == '(' {
				parenStart = i
				break
			}
			if query[i] != ' ' && query[i] != '\t' && query[i] != '\n' &&
				!((query[i] >= 'a' && query[i] <= 'z') ||
					(query[i] >= 'A' && query[i] <= 'Z') ||
					query[i] == '_') {
				break
			}
		}

		if parenStart == -1 {
			searchFrom = idx + 8
			continue
		}

		// Find matching closing paren
		depth := 1
		parenEnd := parenStart + 1
		inString := false
		stringChar := byte(0)
		for parenEnd < len(query) && depth > 0 {
			c := query[parenEnd]
			if !inString {
				if c == '"' || c == '\'' {
					inString = true
					stringChar = c
				} else if c == '(' {
					depth++
				} else if c == ')' {
					depth--
				}
			} else {
				if c == stringChar {
					if stringChar == '"' && parenEnd > 0 && query[parenEnd-1] == '\\' {
						// escaped
					} else {
						inString = false
					}
				}
			}
			parenEnd++
		}

		if depth != 0 {
			break
		}

		// Now look for " : (" after the closing paren
		colonStart := parenEnd
		for colonStart < len(query) && (query[colonStart] == ' ' || query[colonStart] == '\t' || query[colonStart] == '\n') {
			colonStart++
		}

		if colonStart >= len(query) || query[colonStart] != ':' {
			// No colon after evaluate, continue
			searchFrom = idx + 8
			continue
		}

		// Find the opening paren after the colon
		typeStart := colonStart + 1
		for typeStart < len(query) && (query[typeStart] == ' ' || query[typeStart] == '\t' || query[typeStart] == '\n') {
			typeStart++
		}

		if typeStart >= len(query) || query[typeStart] != '(' {
			searchFrom = idx + 8
			continue
		}

		// Find the matching closing paren for the type specification
		depth = 1
		typeEnd := typeStart + 1
		for typeEnd < len(query) && depth > 0 {
			if query[typeEnd] == '(' {
				depth++
			} else if query[typeEnd] == ')' {
				depth--
			}
			typeEnd++
		}

		if depth != 0 {
			break
		}

		// Remove from colon to closing paren: "evaluate func(...) : (...)" -> "evaluate func(...)"
		// Copy everything up to parenEnd (keep the function call), skip parenEnd..typeEnd (the type annotation)
		b.WriteString(query[lastCopied:parenEnd])
		lastCopied = typeEnd
		searchFrom = typeEnd
		changed = true
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// normalizeLookupSubquery simplifies complex expressions inside lookup subqueries
// lookup (union T1, T2) -> lookup (LookupTable)
// The grammar only supports simple table references inside lookup
func normalizeLookupSubquery(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		idx := indexKeyword(lowerQuery, "lookup", searchFrom)
		if idx == -1 {
			break
		}

		// Find the opening paren
		parenStart := idx + 6
		for parenStart < len(query) && (query[parenStart] == ' ' || query[parenStart] == '\t' || query[parenStart] == '\n' || query[parenStart] == '\r') {
			parenStart++
		}

		if parenStart < len(query) && query[parenStart] != '(' {
			optionStart := parenStart
			for parenStart < len(query) && query[parenStart] != '(' && query[parenStart] != '|' && query[parenStart] != ' ' && query[parenStart] != '\t' && query[parenStart] != '\n' && query[parenStart] != '\r' {
				parenStart++
			}
			optionText := strings.TrimSpace(lowerQuery[optionStart:parenStart])
			if optionText == "" || !strings.HasPrefix(optionText, "kind=") {
				searchFrom = idx + 6
				continue
			}
			for parenStart < len(query) && (query[parenStart] == ' ' || query[parenStart] == '\t') {
				parenStart++
			}
		}

		if parenStart >= len(query) || query[parenStart] != '(' {
			// No paren, skip
			searchFrom = idx + 6
			continue
		}

		// Check if there's a union inside the paren
		// Find content between ( and matching )
		depth := 1
		parenEnd := parenStart + 1
		for parenEnd < len(query) && depth > 0 {
			if query[parenEnd] == '(' {
				depth++
			} else if query[parenEnd] == ')' {
				depth--
			}
			parenEnd++
		}

		if depth != 0 {
			break
		}

		// Check if content contains "union"
		content := lowerQuery[parenStart+1 : parenEnd-1]
		if strings.Contains(content, "union") {
			// Replace entire (union ...) with (LookupTable)
			// Keep the parens: copy up to parenStart+1, write "LookupTable", skip to parenEnd-1
			b.WriteString(query[lastCopied : parenStart+1])
			b.WriteString("LookupTable")
			lastCopied = parenEnd - 1
			searchFrom = parenEnd
			changed = true
		} else {
			// No union, advance
			searchFrom = parenEnd
		}
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// stripMvApplySubquery removes entire mv-apply statements that have on (subquery)
// | mv-apply x = col on (subquery) -> (removed entirely)
// The mv-apply with subquery isn't needed for condition extraction
func stripMvApplySubquery(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		// Find "| mv-apply" (the full operator with pipe)
		rel := strings.Index(lowerQuery[searchFrom:], "| mv-apply")
		if rel == -1 {
			break
		}
		idx := searchFrom + rel

		// Find " on " or " on(" after mv-apply
		mvSearchStart := idx + 10
		onIdx := -1
		for i := mvSearchStart; i < len(query)-3; i++ {
			if (query[i] == ' ' || query[i] == '\n' || query[i] == '\t') &&
				lowerQuery[i+1:i+3] == "on" &&
				(i+3 >= len(query) || query[i+3] == ' ' || query[i+3] == '\n' || query[i+3] == '\t' || query[i+3] == '(') {
				onIdx = i + 1
				break
			}
		}

		if onIdx == -1 {
			// No "on" found for this mv-apply, it's a simple mv-apply without subquery - keep it
			searchFrom = idx + 10
			continue
		}

		// Find the opening paren after "on"
		parenStart := onIdx + 2
		for parenStart < len(query) && (query[parenStart] == ' ' || query[parenStart] == '\t' || query[parenStart] == '\n') {
			parenStart++
		}

		if parenStart >= len(query) || query[parenStart] != '(' {
			// No paren after "on", just "on" keyword (edge case) - skip
			searchFrom = idx + 10
			continue
		}

		// Find matching closing paren
		depth := 1
		parenEnd := parenStart + 1
		inString := false
		stringChar := byte(0)
		for parenEnd < len(query) && depth > 0 {
			c := query[parenEnd]
			if !inString {
				if c == '"' || c == '\'' {
					inString = true
					stringChar = c
				} else if c == '(' {
					depth++
				} else if c == ')' {
					depth--
				}
			} else {
				// In KQL, only double-quoted strings have backslash escaping
				if c == stringChar {
					if stringChar == '"' && parenEnd > 0 && query[parenEnd-1] == '\\' {
						// Escaped quote, continue
					} else {
						inString = false
					}
				}
			}
			parenEnd++
		}

		if depth != 0 {
			// Unbalanced parens, skip
			break
		}

		// Remove entire mv-apply statement from "| mv-apply" to closing paren
		b.WriteString(query[lastCopied:idx])
		lastCopied = parenEnd
		searchFrom = parenEnd
		changed = true
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// stripParseStatements removes parse operator statements
// parse field with 'pattern' var1 'pattern2' var2 -> (entire statement removed)
// The parse operator extracts substrings but isn't needed for condition extraction
func stripParseStatements(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		// Find "| parse" or "\nparse" (parse operator at start of pipe stage)
		idx := strings.Index(lowerQuery[searchFrom:], "| parse ")
		parseKeywordLen := len("parse")
		if idx == -1 {
			idx = strings.Index(lowerQuery[searchFrom:], "| parse\t")
		}
		if idx == -1 {
			idx = strings.Index(lowerQuery[searchFrom:], "| parse\n")
		}
		if idx == -1 {
			idx = strings.Index(lowerQuery[searchFrom:], "| parse-where ")
			parseKeywordLen = len("parse-where")
		}
		if idx == -1 {
			idx = strings.Index(lowerQuery[searchFrom:], "| parse-where\t")
			parseKeywordLen = len("parse-where")
		}
		if idx == -1 {
			idx = strings.Index(lowerQuery[searchFrom:], "| parse-where\n")
			parseKeywordLen = len("parse-where")
		}
		if idx == -1 {
			break
		}
		idx += searchFrom

		// Find the end of this parse statement (next pipe, closing paren, or end of query)
		parseStart := idx + 2 + parseKeywordLen
		pipeOrEnd := parseStart

		// Track string literals and parentheses
		inString := false
		stringChar := byte(0)
		isVerbatim := false
		parenDepth := 0

		// Count starting paren depth from query beginning to idx (ignoring strings)
		inStrInit := false
		strCharInit := byte(0)
		isVerbatimInit := false
		for i := 0; i < idx; i++ {
			c := query[i]
			if !inStrInit {
				if c == '"' || c == '\'' {
					inStrInit = true
					strCharInit = c
					isVerbatimInit = i > 0 && query[i-1] == '@'
				} else if c == '(' {
					parenDepth++
				} else if c == ')' {
					parenDepth--
				}
			} else {
				if c == strCharInit {
					if isVerbatimInit {
						// Verbatim strings: any closing quote ends the string
						inStrInit = false
					} else if strCharInit == '"' {
						// Regular double-quoted: check backslash escaping
						if i == 0 || query[i-1] != '\\' {
							inStrInit = false
						}
					} else {
						// Single-quoted: no escaping
						inStrInit = false
					}
				}
			}
		}
		startingDepth := parenDepth

		for pipeOrEnd < len(query) {
			c := query[pipeOrEnd]
			if !inString {
				if c == '"' || c == '\'' {
					inString = true
					stringChar = c
					isVerbatim = pipeOrEnd > 0 && query[pipeOrEnd-1] == '@'
				} else if c == '|' {
					// Found next pipe at same paren level
					break
				} else if c == ';' {
					// Semicolon ends statements (like let statements that contain parse)
					if parenDepth <= startingDepth {
						break
					}
				} else if c == ')' {
					// Closing paren - stop here if we'd go below starting depth
					if parenDepth <= startingDepth {
						break
					}
					parenDepth--
				} else if c == '(' {
					parenDepth++
				} else if c == '\n' {
					// Check if next non-whitespace is a new operator
					nextNonSpace := pipeOrEnd + 1
					for nextNonSpace < len(query) && (query[nextNonSpace] == ' ' || query[nextNonSpace] == '\t') {
						nextNonSpace++
					}
					if nextNonSpace < len(query) && query[nextNonSpace] == '|' {
						pipeOrEnd = nextNonSpace
						break
					}
				}
			} else {
				if c == stringChar {
					if isVerbatim {
						// Verbatim strings (@"..."): any closing quote ends the string
						inString = false
					} else if stringChar == '"' {
						// Regular double-quoted strings: check backslash escaping
						backslashes := 0
						for j := pipeOrEnd - 1; j >= 0 && query[j] == '\\'; j-- {
							backslashes++
						}
						if backslashes%2 == 0 {
							inString = false
						}
					} else {
						// Single quote - no escaping, so this ends the string
						inString = false
					}
				}
			}
			pipeOrEnd++
		}

		// Remove the parse statement (keep the delimiter for the next statement if any)
		b.WriteString(query[lastCopied:idx])
		if pipeOrEnd < len(query) {
			delim := query[pipeOrEnd]
			if delim == '|' || delim == ')' || delim == ';' {
				// Keep the delimiter: "| parse ... | next" -> "| next"
				lastCopied = pipeOrEnd
			} else {
				lastCopied = pipeOrEnd
			}
		} else {
			// No following delimiter, just remove to end: "| parse ..." -> ""
			lastCopied = pipeOrEnd
		}
		searchFrom = pipeOrEnd
		changed = true
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// normalizeNotEqualOperator converts SQL-style <> to KQL !=
// Handles all spacing variations: x<>y, x<> y, x <> y, x<>"foo"
func normalizeNotEqualOperator(query string) string {
	var result strings.Builder
	result.Grow(len(query))

	i := 0
	for i < len(query) {
		// Check for <> pattern
		if i+1 < len(query) && query[i] == '<' && query[i+1] == '>' {
			result.WriteString("!=")
			i += 2
			continue
		}
		result.WriteByte(query[i])
		i++
	}
	return result.String()
}

// normalizeDistinctColumns strips function wrappers from distinct column lists
// distinct a, tostring(b), c -> distinct a, b, c
// The grammar only supports simple column names in distinct, not expressions
func normalizeDistinctColumns(query string) string {
	// Find each distinct clause
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		// Find "distinct " (must have space after to avoid "distinctive" etc)
		pos := strings.Index(lowerQuery[searchFrom:], "distinct ")
		if pos == -1 {
			break
		}
		distinctStart := searchFrom + pos + 9 // Position after "distinct "
		searchFrom = distinctStart

		// Find the end of the distinct clause (next pipe, newline, or end)
		endPos := len(query)
		for i := distinctStart; i < len(query); i++ {
			if query[i] == '|' || query[i] == '\n' {
				endPos = i
				break
			}
		}

		// Process the column list
		columnPart := query[distinctStart:endPos]
		normalizedCols := normalizeDistinctColumnList(columnPart)

		// Write everything up to distinctStart, then the normalized columns
		b.WriteString(query[lastCopied:distinctStart])
		b.WriteString(normalizedCols)
		lastCopied = endPos
		searchFrom = distinctStart + len(normalizedCols)
		// Adjust searchFrom relative to what's been written vs original
		// Since we're searching lowerQuery (original), we need to advance past endPos
		searchFrom = endPos
		changed = true
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// normalizeDistinctColumnList processes the column list portion of a distinct clause
// Unwraps single-argument function calls like tostring(x) -> x
// Keeps multi-argument or complex functions as-is to avoid breaking them
func normalizeDistinctColumnList(columns string) string {
	var result strings.Builder
	i := 0

	for i < len(columns) {
		// Skip whitespace
		if columns[i] == ' ' || columns[i] == '\t' {
			result.WriteByte(columns[i])
			i++
			continue
		}

		// Check for comma
		if columns[i] == ',' {
			result.WriteByte(columns[i])
			i++
			continue
		}

		// Handle string literals - pass through as-is
		if columns[i] == '"' || columns[i] == '\'' {
			quote := columns[i]
			result.WriteByte(columns[i])
			i++
			for i < len(columns) && columns[i] != quote {
				if columns[i] == '\\' && i+1 < len(columns) {
					result.WriteByte(columns[i])
					i++
				}
				if i < len(columns) {
					result.WriteByte(columns[i])
					i++
				}
			}
			if i < len(columns) {
				result.WriteByte(columns[i]) // closing quote
				i++
			}
			continue
		}

		// Handle other non-identifier characters - pass through
		if !isIdentChar(columns[i]) && columns[i] != '.' {
			result.WriteByte(columns[i])
			i++
			continue
		}

		// Read an identifier or function call
		start := i
		// Read the identifier
		for i < len(columns) && (isIdentChar(columns[i]) || columns[i] == '.') {
			i++
		}
		name := columns[start:i]

		// Skip whitespace
		wsStart := i
		for i < len(columns) && (columns[i] == ' ' || columns[i] == '\t') {
			i++
		}

		// Check if followed by = (alias assignment: Alias = Field)
		if i < len(columns) && columns[i] == '=' {
			// Skip the alias and = sign, read the actual expression until the
			// next top-level comma. The RHS can be a wrapper function such as
			// tostring(Device.Name), not only a bare identifier.
			i++ // skip =
			// Skip whitespace after =
			for i < len(columns) && (columns[i] == ' ' || columns[i] == '\t') {
				i++
			}
			exprStart := i
			depth := 0
			var quote byte
			for i < len(columns) {
				ch := columns[i]
				if quote != 0 {
					if ch == '\\' && i+1 < len(columns) {
						i += 2
						continue
					}
					if ch == quote {
						quote = 0
					}
					i++
					continue
				}
				if ch == '"' || ch == '\'' {
					quote = ch
					i++
					continue
				}
				switch ch {
				case '(':
					depth++
				case ')':
					if depth > 0 {
						depth--
					}
				case ',':
					if depth == 0 {
						goto writeAliasExpression
					}
				}
				i++
			}
		writeAliasExpression:
			expression := strings.TrimSpace(columns[exprStart:i])
			if expression == "" {
				result.WriteString("_distinct_col_")
			} else {
				result.WriteString(normalizeDistinctColumnList(expression))
			}
			continue
		}

		// Check if followed by (
		if i < len(columns) && columns[i] == '(' {
			// This is a function call - extract first argument
			parenDepth := 1
			argStart := i + 1
			i++
			for i < len(columns) && parenDepth > 0 {
				if columns[i] == '(' {
					parenDepth++
				} else if columns[i] == ')' {
					parenDepth--
				} else if columns[i] == '"' || columns[i] == '\'' {
					// Skip string literal
					quote := columns[i]
					i++
					for i < len(columns) && columns[i] != quote {
						if columns[i] == '\\' && i+1 < len(columns) {
							i++
						}
						i++
					}
				}
				i++
			}
			// Extract the first argument from any function call
			// For distinct, the grammar doesn't support function calls at all
			if parenDepth == 0 {
				// Get content inside parentheses. Only unwrap single-argument
				// wrappers; multi-argument calls carry semantics we must keep.
				content := columns[argStart : i-1]
				args := splitCommaList(content)
				if len(args) != 1 {
					result.WriteString(columns[start:i])
					continue
				}
				firstArg := strings.TrimSpace(stripOuterParens(args[0]))

				// Check if first arg is a simple identifier (optionally with dots)
				isSimple := len(firstArg) > 0
				for _, c := range firstArg {
					if !isIdentChar(byte(c)) && c != '.' {
						isSimple = false
						break
					}
				}
				if isSimple {
					// If there's a dot (property access), use only the last part
					// distinct doesn't support property access like x.y
					if dotIdx := strings.LastIndex(firstArg, "."); dotIdx != -1 {
						firstArg = firstArg[dotIdx+1:]
					}
					result.WriteString(firstArg)
				} else {
					// First arg is complex, use placeholder
					result.WriteString("_distinct_col_")
				}
			} else {
				// Malformed parens, use placeholder
				result.WriteString("_distinct_col_")
			}
		} else {
			// Plain identifier - restore any whitespace we skipped
			result.WriteString(name)
			i = wsStart
			for i < len(columns) && (columns[i] == ' ' || columns[i] == '\t') {
				result.WriteByte(columns[i])
				i++
			}
		}
	}

	return result.String()
}

// stripTrailingSemicolonsAndComments removes trailing semicolons and line comments
// Handles patterns like "| where x == 1; // comment" -> "| where x == 1"
func stripTrailingSemicolonsAndComments(query string) string {
	result := query

	for {
		// Strip trailing whitespace
		result = strings.TrimRight(result, " \t\n\r")
		if len(result) == 0 {
			break
		}

		// Check for trailing line comment (find last // not in a string or
		// in a comment)
		lastCommentIdx := -1
		for i := 0; i < len(result); {
			switch {
			case isStringLiteralStart(result, i):
				end := stringLiteralEnd(result, i)
				if end < 0 {
					end = len(result)
				}
				i = end
			case result[i] == '/' && i+1 < len(result) && result[i+1] == '/':
				lastCommentIdx = i
				for i < len(result) && result[i] != '\n' {
					i++
				}
			default:
				i++
			}
		}

		// If there's a trailing line comment on the last line, strip it
		if lastCommentIdx != -1 {
			// Check if there's a newline after this comment
			hasNewlineAfter := false
			for j := lastCommentIdx + 2; j < len(result); j++ {
				if result[j] == '\n' {
					hasNewlineAfter = true
					break
				}
			}
			if !hasNewlineAfter {
				// This comment is at the end - strip it
				result = strings.TrimRight(result[:lastCommentIdx], " \t")
				continue
			}
		}

		// Check for trailing semicolon
		if strings.HasSuffix(result, ";") {
			result = strings.TrimSuffix(result, ";")
			continue
		}

		// No more changes
		break
	}

	return result
}

// stripRenderParameters removes parameters from render operator
// render areachart kind=stacked -> render areachart
// The grammar doesn't support all render parameter forms (like kind=value without parens)
func stripRenderParameters(query string) string {
	// Find render operator
	lowerQuery := strings.ToLower(query)
	idx := strings.Index(lowerQuery, "| render ")
	if idx == -1 {
		idx = strings.Index(lowerQuery, "|render ")
	}
	if idx == -1 {
		return query
	}

	// Find start of render clause (after "| render ")
	renderStart := idx
	for renderStart < len(query) && query[renderStart] != 'r' && query[renderStart] != 'R' {
		renderStart++
	}

	// Skip "render "
	renderStart += 7
	for renderStart < len(query) && (query[renderStart] == ' ' || query[renderStart] == '\t') {
		renderStart++
	}

	// Read the chart type (first identifier)
	chartTypeStart := renderStart
	for renderStart < len(query) && isIdentChar(query[renderStart]) {
		renderStart++
	}

	// chartType ends at renderStart now
	// Skip any whitespace
	for renderStart < len(query) && (query[renderStart] == ' ' || query[renderStart] == '\t') {
		renderStart++
	}

	// If followed by "with" in parens, that's fine
	// If followed by identifier=value (like kind=stacked), strip it
	if renderStart < len(query) && query[renderStart] != '\n' && query[renderStart] != '|' {
		// Check if it's "with ("
		remaining := strings.ToLower(query[renderStart:])
		if strings.HasPrefix(remaining, "with ") || strings.HasPrefix(remaining, "with(") {
			// Keep "with (...)" - find the end
			return query
		}
		// Otherwise strip everything after chart type until newline or pipe or end
		end := renderStart
		for end < len(query) && query[end] != '\n' && query[end] != '|' {
			end++
		}
		return query[:chartTypeStart] + query[chartTypeStart:renderStart-1] + query[end:]
	}

	return query
}

// normalizeTimespanLiterals adds spaces between numbers and timespan suffixes where needed
// Converts: 0min -> 0min, 30min -> 30min (actually keeps them but in valid form)
// The issue is patterns like "0min" that get tokenized as number "0" + identifier "min"
// We convert them to the format the lexer recognizes: 0m, 30m, etc.
func normalizeTimespanLiterals(query string) string {
	// Map of full suffix to short suffix that the lexer recognizes
	suffixMap := map[string]string{
		"min":     "m",
		"minute":  "m",
		"minutes": "m",
		"sec":     "s",
		"second":  "s",
		"seconds": "s",
		"hour":    "h",
		"hours":   "h",
		"day":     "d",
		"days":    "d",
	}

	result := query
	for longSuffix, shortSuffix := range suffixMap {
		// Find number followed immediately by suffix — build output incrementally
		var b strings.Builder
		b.Grow(len(result))
		lastCopied := 0
		changed := false
		for i := 0; i < len(result); i++ {
			// Check if we're at a digit
			if result[i] >= '0' && result[i] <= '9' {
				// Find end of number
				numEnd := i + 1
				for numEnd < len(result) && result[numEnd] >= '0' && result[numEnd] <= '9' {
					numEnd++
				}
				// Check if followed by this suffix
				if numEnd+len(longSuffix) <= len(result) {
					potentialSuffix := result[numEnd : numEnd+len(longSuffix)]
					if strings.EqualFold(potentialSuffix, longSuffix) {
						// Make sure it's not part of a longer word
						afterSuffix := numEnd + len(longSuffix)
						if afterSuffix >= len(result) || !isIdentChar(result[afterSuffix]) {
							// Replace with short suffix
							b.WriteString(result[lastCopied:numEnd])
							b.WriteString(shortSuffix)
							lastCopied = afterSuffix
							i = afterSuffix - 1 // -1 because the for loop increments
							changed = true
							continue
						}
					}
				}
			}
		}
		if changed {
			b.WriteString(result[lastCopied:])
			result = b.String()
		}
	}
	return result
}

// normalizeTrailingDecimals adds a zero after trailing decimal points
// The grammar requires at least one digit after the decimal: 1000. -> 1000.0
func normalizeTrailingDecimals(query string) string {
	var result strings.Builder
	result.Grow(len(query))
	inString := false
	stringChar := byte(0)
	isVerbatim := false

	for i := 0; i < len(query); i++ {
		c := query[i]

		// Track strings to avoid modifying decimals inside string literals
		if !inString && (c == '"' || c == '\'') {
			inString = true
			stringChar = c
			isVerbatim = i > 0 && query[i-1] == '@'
			result.WriteByte(c)
			continue
		}
		if inString {
			if c == stringChar {
				if isVerbatim {
					if i+1 < len(query) && query[i+1] == stringChar {
						result.WriteByte(c)
						i++
						result.WriteByte(query[i])
						continue
					}
					inString = false
					isVerbatim = false
				} else if stringChar == '"' {
					backslashes := 0
					for j := i - 1; j >= 0 && query[j] == '\\'; j-- {
						backslashes++
					}
					if backslashes%2 == 0 {
						inString = false
					}
				} else {
					inString = false
				}
			}
			result.WriteByte(c)
			continue
		}

		// Check for pattern: digit followed by . not followed by digit
		if c == '.' && i > 0 && isDigit(query[i-1]) {
			// Check what follows the dot
			nextIdx := i + 1
			if (nextIdx >= len(query) || (!isDigit(query[nextIdx]) && query[nextIdx] != '.')) &&
				isStandaloneNumericLiteralBeforeDot(query, i) &&
				isTrailingDecimalBoundary(query, nextIdx) {
				// Trailing decimal point - add .0
				result.WriteByte('.')
				result.WriteByte('0')
				continue
			}
		}

		result.WriteByte(c)
	}

	return result.String()
}

func isStandaloneNumericLiteralBeforeDot(query string, dotIdx int) bool {
	start := dotIdx - 1
	for start >= 0 && isDigit(query[start]) {
		start--
	}
	if start < 0 {
		return true
	}
	return !isIdentChar(query[start]) && query[start] != '.'
}

func isTrailingDecimalBoundary(query string, idx int) bool {
	if idx >= len(query) {
		return true
	}
	switch query[idx] {
	case ' ', '\t', '\r', '\n', ',', ';', ')', ']', '}', '|', '+', '-', '*', '/', '%', '=', '!', '<', '>':
		return true
	default:
		return false
	}
}

// normalizeNullEscapes converts \0 to \x00 in regular strings
// The KQL grammar only supports \x hex escapes, not bare \0
func normalizeNullEscapes(query string) string {
	var result strings.Builder
	result.Grow(len(query) + 50) // Extra space for expanded escapes

	inSingleQuote := false
	inDoubleQuote := false
	inVerbatim := false // @"..." or @'...'

	for i := 0; i < len(query); i++ {
		c := query[i]

		// Check for verbatim string start
		if !inSingleQuote && !inDoubleQuote && !inVerbatim && c == '@' && i+1 < len(query) {
			if query[i+1] == '"' || query[i+1] == '\'' {
				result.WriteByte(c)
				result.WriteByte(query[i+1])
				inVerbatim = true
				if query[i+1] == '"' {
					inDoubleQuote = true
				} else {
					inSingleQuote = true
				}
				i++
				continue
			}
		}

		// Track string state
		if c == '"' && !inSingleQuote {
			if inVerbatim && inDoubleQuote {
				// Check for doubled quote (escape in verbatim)
				if i+1 < len(query) && query[i+1] == '"' {
					result.WriteByte(c)
					result.WriteByte(query[i+1])
					i++
					continue
				}
				inDoubleQuote = false
				inVerbatim = false
			} else if !inVerbatim {
				inDoubleQuote = !inDoubleQuote
			}
		} else if c == '\'' && !inDoubleQuote {
			if inVerbatim && inSingleQuote {
				// Check for doubled quote (escape in verbatim)
				if i+1 < len(query) && query[i+1] == '\'' {
					result.WriteByte(c)
					result.WriteByte(query[i+1])
					i++
					continue
				}
				inSingleQuote = false
				inVerbatim = false
			} else if !inVerbatim {
				inSingleQuote = !inSingleQuote
			}
		}

		// Check for \0 in regular strings (not verbatim)
		if (inSingleQuote || inDoubleQuote) && !inVerbatim && c == '\\' && i+1 < len(query) && query[i+1] == '0' {
			// Check that it's not followed by more digits (like \012 octal) or x (like \0x...)
			nextAfter := i + 2
			if nextAfter >= len(query) || (!isDigit(query[nextAfter]) && query[nextAfter] != 'x') {
				// Convert \0 to \x00
				result.WriteString("\\x00")
				i++ // Skip the '0'
				continue
			}
		}

		// Handle regular escapes - skip past the escaped character
		if (inSingleQuote || inDoubleQuote) && !inVerbatim && c == '\\' && i+1 < len(query) {
			result.WriteByte(c)
			i++
			result.WriteByte(query[i])
			continue
		}

		result.WriteByte(c)
	}

	return result.String()
}

// isDigit checks if a byte is a digit
func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// isIdentChar checks if a byte can be part of an identifier
func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func hasKeywordPrefix(s, keyword string) bool {
	if !strings.HasPrefix(s, keyword) {
		return false
	}
	return len(s) == len(keyword) || !isIdentChar(s[len(keyword)])
}

func indexKeyword(s, keyword string, start int) int {
	for start < len(s) {
		idx := strings.Index(s[start:], keyword)
		if idx == -1 {
			return -1
		}
		idx += start
		beforeOK := idx == 0 || !isIdentChar(s[idx-1])
		after := idx + len(keyword)
		afterOK := after >= len(s) || !isIdentChar(s[after])
		if beforeOK && afterOK {
			return idx
		}
		start = idx + len(keyword)
	}
	return -1
}

func indexKeywordOutsideStrings(s, keyword string, start int) int {
	for start < len(s) {
		idx := indexOutsideStrings(s, keyword, start, true)
		if idx == -1 {
			return -1
		}
		beforeOK := idx == 0 || !isIdentChar(s[idx-1])
		after := idx + len(keyword)
		afterOK := after >= len(s) || !isIdentChar(s[after])
		if beforeOK && afterOK {
			return idx
		}
		start = idx + len(keyword)
	}
	return -1
}

func indexOutsideStrings(s, needle string, start int, caseInsensitive bool) int {
	if needle == "" {
		return start
	}
	if start < 0 {
		start = 0
	}
	if len(needle) > len(s) || start > len(s)-len(needle) {
		return -1
	}

	var quote byte
	verbatim := false
	for i := start; i <= len(s)-len(needle); i++ {
		ch := s[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(s) && s[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(s) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}
		if ch == '@' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\'') {
			quote = s[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		candidate := s[i : i+len(needle)]
		if (!caseInsensitive && candidate == needle) || (caseInsensitive && strings.EqualFold(candidate, needle)) {
			return i
		}
	}
	return -1
}

func indexByteOutsideStrings(s string, needle byte, start int) int {
	return indexOutsideStrings(s, string(needle), start, false)
}

// SubqueryPlaceholder is the value of a membership condition whose values
// come from a subquery, as in Account in (SigninLogs | project Account): the
// parser does not evaluate the subquery.
const SubqueryPlaceholder = "SubqueryResult"

func normalizeInSubqueries(query string) string {
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	var quote byte
	verbatim := false

	for i := 0; i < len(query); i++ {
		ch := query[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(query) && query[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(query) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}

		if ch == '@' && i+1 < len(query) && (query[i+1] == '"' || query[i+1] == '\'') {
			quote = query[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}

		if ch != '(' || !precededByInOperator(query, i) {
			continue
		}

		close := findMatchingParen(query, i)
		if close <= i {
			continue
		}

		if !containsPipeOutsideStrings(query[i+1 : close]) {
			i = close
			continue
		}

		b.WriteString(query[lastCopied : i+1])
		b.WriteString(SubqueryPlaceholder)
		lastCopied = close
		changed = true
		i = close
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

func precededByInOperator(query string, openParen int) bool {
	i := openParen - 1
	for i >= 0 && (query[i] == ' ' || query[i] == '\t' || query[i] == '\r' || query[i] == '\n') {
		i--
	}
	end := i + 1
	for i >= 0 && (isIdentChar(query[i]) || query[i] == '!') {
		i--
	}
	token := strings.ToLower(query[i+1 : end])
	if token != "in" && token != "!in" {
		return false
	}
	return i < 0 || !isIdentChar(query[i])
}

func containsPipeOutsideStrings(s string) bool {
	var quote byte
	verbatim := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(s) && s[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(s) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}
		if ch == '@' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\'') {
			quote = s[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '|' {
			return true
		}
	}
	return false
}

func normalizeStringOperatorAliases(query string) string {
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	var quote byte
	verbatim := false

	for i := 0; i < len(query); i++ {
		ch := query[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(query) && query[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(query) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}

		if ch == '@' && i+1 < len(query) && (query[i+1] == '"' || query[i+1] == '\'') {
			quote = query[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}

		if alias, ok := legacyNegatedContains(query, i); ok {
			b.WriteString(query[lastCopied:i])
			b.WriteString(alias.replacement)
			i += len(alias.token) - 1
			lastCopied = i + 1
			changed = true
			continue
		}
		if hasOperatorTokenAt(query, i, "!containscs") {
			b.WriteString(query[lastCopied:i])
			b.WriteString("!contains_cs")
			i += len("!containscs") - 1
			lastCopied = i + 1
			changed = true
			continue
		}
		if hasOperatorTokenAt(query, i, "containscs") {
			b.WriteString(query[lastCopied:i])
			b.WriteString("contains_cs")
			i += len("containscs") - 1
			lastCopied = i + 1
			changed = true
		}
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// legacyNegatedContains matches notcontains and notcontainscs, the legacy
// spellings of !contains and !contains_cs, at query[i].
func legacyNegatedContains(query string, i int) (struct{ token, replacement string }, bool) {
	for _, alias := range []struct{ token, replacement string }{
		{"notcontainscs", "!contains_cs"},
		{"notcontains", "!contains"},
	} {
		if hasOperatorTokenAt(query, i, alias.token) {
			return alias, true
		}
	}
	return struct{ token, replacement string }{}, false
}

func hasOperatorTokenAt(query string, idx int, token string) bool {
	if idx+len(token) > len(query) {
		return false
	}
	if strings.ToLower(query[idx:idx+len(token)]) != token {
		return false
	}
	if idx > 0 && isIdentChar(query[idx-1]) {
		return false
	}
	after := idx + len(token)
	return after == len(query) || !isIdentChar(query[after])
}

// removeUnionParam removes all occurrences of a case-insensitive parameter (e.g., "isfuzzy")
// along with its =value and any trailing comma/whitespace, using a single-pass Builder.
func removeUnionParam(input string, paramLower string, paramLen int) string {
	lowerInput := strings.ToLower(input)
	var b strings.Builder
	b.Grow(len(input))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		idx := strings.Index(lowerInput[searchFrom:], paramLower)
		if idx == -1 {
			break
		}
		idx += searchFrom

		// Find the end of the parameter (next whitespace, comma, or newline)
		end := idx + paramLen
		// Skip any = and value
		for end < len(input) && (input[end] == '=' || input[end] == ' ' || input[end] == '\t') {
			end++
		}
		// Skip the value (true/false or identifier)
		for end < len(input) && input[end] != ' ' && input[end] != '\t' && input[end] != '\n' && input[end] != ',' && input[end] != '(' {
			end++
		}
		// Skip any trailing comma
		for end < len(input) && (input[end] == ',' || input[end] == ' ' || input[end] == '\t') {
			end++
		}
		b.WriteString(input[lastCopied:idx])
		lastCopied = end
		searchFrom = end
		changed = true
	}

	if !changed {
		return input
	}
	b.WriteString(input[lastCopied:])
	return b.String()
}

// normalizeUnionParameters removes union parameters like isfuzzy=true, withsource=...
// Also handles queries that start with (union ...) by stripping outer parens
func normalizeUnionParameters(query string) string {
	result := query

	// Handle outer parens around union: (union ...) | ... -> union ... | ...
	// or: (union ...) -> union ...
	// Also handles: (union ...) // comment -> union ... // comment
	trimmed := strings.TrimSpace(result)
	if strings.HasPrefix(strings.ToLower(trimmed), "(union") && (len(trimmed) == len("(union") || !isIdentChar(trimmed[len("(union")])) {
		// Find the matching closing paren
		depth := 0
		for i := 0; i < len(trimmed); i++ {
			if trimmed[i] == '(' {
				depth++
			} else if trimmed[i] == ')' {
				depth--
				if depth == 0 {
					// Check what comes after the closing paren
					afterClose := strings.TrimSpace(trimmed[i+1:])
					if len(afterClose) == 0 {
						// Entire query wrapped in parens: (union ...) -> union ...
						result = trimmed[1:i]
					} else if strings.HasPrefix(afterClose, "|") || strings.HasPrefix(afterClose, "//") {
						// Pattern: (union ...) | more... -> union ... | more...
						// or: (union ...) // comment... -> union ... // comment...
						result = trimmed[1:i] + " " + afterClose
					}
					// Otherwise don't modify (complex nesting)
					break
				}
			}
		}
	}

	// Remove isfuzzy=... parameter (up to next whitespace or comma)
	result = removeUnionParam(result, "isfuzzy", 7)

	// Remove withsource=... parameter
	result = removeUnionParam(result, "withsource", 10)
	result = removeUnionParam(result, "with_source", 11)

	// Also handle kind= parameter in union
	{
		lowerResult := strings.ToLower(result)
		var b strings.Builder
		b.Grow(len(result))
		lastCopied := 0
		changed := false
		searchFrom := 0
		for {
			// Search for "union kind=" or "union  kind=" in the remaining portion
			rel := strings.Index(lowerResult[searchFrom:], "union kind=")
			if rel == -1 {
				rel = strings.Index(lowerResult[searchFrom:], "union  kind=")
			}
			if rel == -1 {
				break
			}
			idx := searchFrom + rel
			// Find "kind=" and skip to end of value
			kindRel := strings.Index(lowerResult[idx:], "kind=")
			if kindRel == -1 {
				break
			}
			kindIdx := idx + kindRel
			end := kindIdx + 5
			// Skip the value
			for end < len(result) && result[end] != ' ' && result[end] != '\t' && result[end] != '\n' && result[end] != ',' && result[end] != '(' {
				end++
			}
			// Skip whitespace/comma after value
			for end < len(result) && (result[end] == ' ' || result[end] == '\t' || result[end] == ',') {
				end++
			}
			b.WriteString(result[lastCopied:kindIdx])
			lastCopied = end
			searchFrom = end
			changed = true
		}
		if changed {
			b.WriteString(result[lastCopied:])
			result = b.String()
		}
	}

	// Replace wildcard * in union with dummy table name
	// "union *" -> "union AllTables"
	// This handles patterns like: union *, union withsource=T *
	result = replaceUnionWildcard(result)

	// Strip * suffixes from table name patterns like Device*, Table_*
	// "union Device*" -> "union Device"
	result = stripTableWildcards(result)

	return result
}

// normalizeJoinUnion handles union inside join parentheses
// join (union T1, T2) -> join (DummyTable | union T1, T2)
// The grammar requires a tabularSource before union
func normalizeJoinUnion(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query) + 64)
	lastCopied := 0
	changed := false
	searchFrom := 0

	// Find all join operators
	for {
		joinRel := strings.Index(lowerQuery[searchFrom:], "join")
		if joinRel == -1 {
			break
		}
		joinIdx := searchFrom + joinRel
		searchFrom = joinIdx + 4

		// Skip to find the opening paren after join and optional kind=... hint=...
		i := joinIdx + 4 // after "join"
		for i < len(query) && (query[i] == ' ' || query[i] == '\t' || query[i] == '\n' || query[i] == '\r') {
			i++
		}

		// Skip kind=... and hint=... clauses
		for i < len(query) {
			// Check for kind=
			if i+5 < len(query) && lowerQuery[i:i+5] == "kind=" {
				i += 5
				// Skip the kind value
				for i < len(query) && query[i] != ' ' && query[i] != '\t' && query[i] != '\n' && query[i] != '(' {
					i++
				}
				// Skip whitespace
				for i < len(query) && (query[i] == ' ' || query[i] == '\t' || query[i] == '\n' || query[i] == '\r') {
					i++
				}
				continue
			}
			// Check for hint.xxx=
			if i+5 < len(query) && lowerQuery[i:i+5] == "hint." {
				// Skip hint.xxx=value
				for i < len(query) && query[i] != ' ' && query[i] != '\t' && query[i] != '\n' && query[i] != '(' {
					i++
				}
				// Skip whitespace
				for i < len(query) && (query[i] == ' ' || query[i] == '\t' || query[i] == '\n' || query[i] == '\r') {
					i++
				}
				continue
			}
			break
		}

		// Now we should be at the opening paren
		if i >= len(query) || query[i] != '(' {
			continue
		}
		parenStart := i

		// Check if content after ( starts with union (skip whitespace)
		j := i + 1
		for j < len(query) && (query[j] == ' ' || query[j] == '\t' || query[j] == '\n' || query[j] == '\r') {
			j++
		}

		// Handle double parens: ((union ...)) - find the innermost paren before union
		innerParenStart := -1
		for j < len(query) && query[j] == '(' {
			innerParenStart = j
			j++ // skip inner paren
			for j < len(query) && (query[j] == ' ' || query[j] == '\t' || query[j] == '\n' || query[j] == '\r') {
				j++
			}
		}

		if j+5 < len(query) && lowerQuery[j:j+5] == "union" {
			// Check it's a word boundary
			if j+5 < len(query) && !isIdentChar(query[j+5]) {
				// Insert DummyTable | after the innermost opening paren (or the main one if no inner)
				insertPoint := parenStart + 1
				if innerParenStart != -1 {
					insertPoint = innerParenStart + 1
				}
				b.WriteString(query[lastCopied:insertPoint])
				b.WriteString("DummyTable | ")
				lastCopied = insertPoint
				searchFrom = j + 5 // skip past "union"
				changed = true
			}
		}
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// stripJoinHints removes hint.xxx=value patterns from join statements
// Common hints: hint.strategy=broadcast, hint.shufflekey=x, hint.remote=auto
func stripJoinHints(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		hintIdx := strings.Index(lowerQuery[searchFrom:], "hint.")
		if hintIdx == -1 {
			break
		}
		hintIdx += searchFrom

		// Find the end of hint.xxx=value
		// First find the = sign
		eqIdx := strings.Index(query[hintIdx:], "=")
		if eqIdx == -1 {
			break // No = found, malformed
		}
		eqIdx += hintIdx

		// Find end of value - can be simple identifier or a function call with parens
		endIdx := eqIdx + 1
		// Skip whitespace after =
		for endIdx < len(query) && (query[endIdx] == ' ' || query[endIdx] == '\t') {
			endIdx++
		}

		// Check if value is a parenthesized expression
		if endIdx < len(query) && query[endIdx] == '(' {
			// Find matching close paren
			depth := 1
			endIdx++
			for endIdx < len(query) && depth > 0 {
				if query[endIdx] == '(' {
					depth++
				} else if query[endIdx] == ')' {
					depth--
				}
				endIdx++
			}
		} else {
			// Simple value - read until whitespace, comma, or open paren
			for endIdx < len(query) && query[endIdx] != ' ' && query[endIdx] != '\t' && query[endIdx] != '\n' && query[endIdx] != '(' && query[endIdx] != ')' && query[endIdx] != ',' {
				endIdx++
			}
		}

		// Remove the hint and any trailing whitespace
		trailing := endIdx
		for trailing < len(query) && (query[trailing] == ' ' || query[trailing] == '\t') {
			trailing++
		}

		b.WriteString(query[lastCopied:hintIdx])
		lastCopied = trailing
		searchFrom = trailing
		changed = true
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// replaceUnionWildcard replaces the * wildcard in union statements with a dummy table
func replaceUnionWildcard(query string) string {
	// Look for patterns like "union *" or "union  *" (with extra spaces)
	// Also handles after parameter stripping: "union *" from "union withsource=T *"

	// Find all occurrences of standalone * that represent table wildcards
	// Can appear as: "union *", "union Table, *", etc.
	// Single-pass: scan forward, collect replacement positions, then build output
	var b strings.Builder
	b.Grow(len(query) + 64)
	lastCopied := 0
	changed := false

	for i := 0; i < len(query); i++ {
		if query[i] == '*' {
			// Check if this is a standalone * (not part of *= or ** or inside a string)
			// It should be preceded by whitespace/comma and followed by whitespace/|/comment/end
			prevOk := i == 0 || query[i-1] == ' ' || query[i-1] == '\t' || query[i-1] == '\n' || query[i-1] == ','
			afterIdx := i + 1
			afterOk := afterIdx >= len(query) ||
				query[afterIdx] == ' ' || query[afterIdx] == '\t' || query[afterIdx] == '\n' || query[afterIdx] == '\r' ||
				query[afterIdx] == '|' || query[afterIdx] == ',' ||
				(afterIdx+1 < len(query) && query[afterIdx] == '/' && query[afterIdx+1] == '/')

			// Also reject if followed by a digit (multiplication like * 1.0)
			if afterOk && afterIdx < len(query) && isDigit(query[afterIdx]) {
				afterOk = false
			}
			// Reject if preceded by a digit (like 7 * something)
			if prevOk && i > 0 && isDigit(query[i-1]) {
				prevOk = false
			}

			if prevOk && afterOk {
				// Verify it's in a union context - look backwards for "union" keyword
				// without any closing parens or arithmetic operators between
				inUnionContext := false
				beforeStar := query[:i]
				lowerBefore := strings.ToLower(beforeStar)

				// Find the last occurrence of "union"
				lastUnionIdx := strings.LastIndex(lowerBefore, "union")
				if lastUnionIdx != -1 {
					// Check what's between "union" and the star
					// Should only be: whitespace, table names, commas, maybe some params
					// Should NOT have: closing parens without matching opens (arithmetic context)
					// Should NOT have: pipe operator | (which ends the union context)
					between := beforeStar[lastUnionIdx+5:]
					depth := 0
					validContext := true
					for _, ch := range between {
						if ch == '(' {
							depth++
						} else if ch == ')' {
							depth--
							if depth < 0 {
								// More closing parens than opening - we're in a different context
								validContext = false
								break
							}
						} else if ch == '|' && depth == 0 {
							// Pipe at depth 0 means we've left the union context
							validContext = false
							break
						}
					}
					// Also check we're not in a deep nesting
					if validContext && depth == 0 {
						inUnionContext = true
					}
				}

				if inUnionContext {
					b.WriteString(query[lastCopied:i])
					b.WriteString("AllTables")
					lastCopied = i + 1
					changed = true
				}
			}
		}
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// stripTableWildcards removes * from table name patterns like Device*, *_CL, Table_*
// These are valid KQL but the grammar doesn't support them
func stripTableWildcards(query string) string {
	// Look for patterns like "union TableName*", "union *_CL", or "union TableName*, OtherTable"
	// The * is attached to an identifier (no space on one side)
	// Single-pass: scan forward and skip * characters that are table wildcards
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false

	for i := 0; i < len(query); i++ {
		if query[i] == '*' {
			shouldRemove := false

			// Case 1: Suffix wildcard (Device*)
			// Check if preceded by an identifier character (letter, digit, underscore)
			if i > 0 && isIdentChar(query[i-1]) {
				// Check if followed by whitespace, comma, pipe, end, or closing paren
				afterOk := i+1 >= len(query) ||
					query[i+1] == ' ' || query[i+1] == '\t' || query[i+1] == '\n' ||
					query[i+1] == ',' || query[i+1] == '|' || query[i+1] == ')'
				if afterOk {
					shouldRemove = true
				}
			}

			// Case 2: Prefix wildcard (*_CL)
			// Check if followed by an identifier character
			if !shouldRemove && i+1 < len(query) && (isIdentChar(query[i+1]) || query[i+1] == '_') {
				// Check if preceded by whitespace, comma, or start
				prevOk := i == 0 ||
					query[i-1] == ' ' || query[i-1] == '\t' || query[i-1] == '\n' ||
					query[i-1] == ','
				if prevOk {
					shouldRemove = true
				}
			}

			if shouldRemove {
				b.WriteString(query[lastCopied:i])
				lastCopied = i + 1
				changed = true
			}
		}
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// stripNamedFunctionParams removes named parameter assignments from function calls
// that appear at the start of a query (user-defined function calls)
// Example: parser(pack=true) -> parser or parser (pack=pack) -> parser
func stripNamedFunctionParams(query string) string {
	trimmed := strings.TrimSpace(query)
	if len(trimmed) == 0 {
		return query
	}

	// Check if query starts with an identifier followed by optional space and (
	// Pattern: identifier(... or identifier (...)
	i := 0
	for i < len(trimmed) && (isIdentChar(trimmed[i]) || trimmed[i] == '_') {
		i++
	}
	if i == 0 {
		return query // doesn't start with identifier
	}

	// Skip optional whitespace
	j := i
	for j < len(trimmed) && (trimmed[j] == ' ' || trimmed[j] == '\t') {
		j++
	}

	// Check for (
	if j >= len(trimmed) || trimmed[j] != '(' {
		return query // no opening paren
	}

	// Find matching close paren
	parenDepth := 1
	k := j + 1
	inString := false
	stringChar := byte(0)
	hasNamedParam := false

	for k < len(trimmed) && parenDepth > 0 {
		c := trimmed[k]
		if !inString {
			if c == '"' || c == '\'' {
				inString = true
				stringChar = c
			} else if c == '(' {
				parenDepth++
			} else if c == ')' {
				parenDepth--
			} else if c == '=' && parenDepth == 1 {
				// Check if this is a named param (not ==)
				if k+1 < len(trimmed) && trimmed[k+1] != '=' && (k == 0 || trimmed[k-1] != '=' && trimmed[k-1] != '!' && trimmed[k-1] != '<' && trimmed[k-1] != '>') {
					hasNamedParam = true
				}
			}
		} else {
			if c == stringChar && (k == 0 || trimmed[k-1] != '\\') {
				inString = false
			}
		}
		k++
	}

	if parenDepth != 0 {
		return query // unbalanced parens
	}

	if !hasNamedParam {
		return query // no named params, leave as is
	}

	// Remove the parameter list entirely
	// identifier(named=params) | rest -> identifier | rest
	parenEnd := k
	afterParen := strings.TrimSpace(trimmed[parenEnd:])

	// Reconstruct: keep identifier, drop params, keep rest
	result := trimmed[:i] + " " + afterParen
	return strings.TrimSpace(result)
}

// normalizeMakeSeries handles make-series syntax variants:
// 1. "in range(start, end, step)" -> "from start to end step step"
// 2. Missing step -> add "step 1h"
func normalizeMakeSeries(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query) + 64)
	lastCopied := 0
	changed := false
	searchFrom := 0

	// Find make-series occurrences
	for {
		pos := strings.Index(lowerQuery[searchFrom:], "make-series")
		if pos == -1 {
			break
		}
		pos += searchFrom

		// Find the end of this make-series clause (next | or end of string)
		endPos := pos
		inString := false
		stringChar := byte(0)
		parenDepth := 0
		for endPos < len(query) {
			c := query[endPos]
			if !inString {
				if c == '"' || c == '\'' {
					inString = true
					stringChar = c
				} else if c == '(' {
					parenDepth++
				} else if c == ')' {
					parenDepth--
				} else if c == '|' && parenDepth == 0 {
					break
				}
			} else {
				if c == stringChar && (endPos == 0 || query[endPos-1] != '\\') {
					inString = false
				}
			}
			endPos++
		}

		makeSeriesClause := query[pos:endPos]
		lowerClause := strings.ToLower(makeSeriesClause)

		// Handle "in range(start, end, step)" pattern
		if strings.Contains(lowerClause, " in range(") {
			rangeIdx := strings.Index(lowerClause, " in range(")
			if rangeIdx != -1 {
				// Find the range(...) parameters
				actualRangeStart := pos + rangeIdx + 10 // after " in range("
				// Find matching close paren
				rpDepth := 1
				rangeEnd := actualRangeStart
				for rangeEnd < endPos && rpDepth > 0 {
					if query[rangeEnd] == '(' {
						rpDepth++
					} else if query[rangeEnd] == ')' {
						rpDepth--
					}
					rangeEnd++
				}
				if rpDepth == 0 {
					// Extract the parameters
					params := query[actualRangeStart : rangeEnd-1]
					// Split by comma (simple split - may need refinement for nested expressions)
					parts := splitByTopLevelComma(params)
					if len(parts) == 3 {
						// in range(start, end, step) -> from start to end step step
						b.WriteString(query[lastCopied : pos+rangeIdx])
						b.WriteString(" from ")
						b.WriteString(strings.TrimSpace(parts[0]))
						b.WriteString(" to ")
						b.WriteString(strings.TrimSpace(parts[1]))
						b.WriteString(" step ")
						b.WriteString(strings.TrimSpace(parts[2]))
						lastCopied = rangeEnd
						changed = true
					}
				}
			}
		} else if !strings.Contains(lowerClause, " step ") && !strings.Contains(lowerClause, "\nstep ") && !strings.Contains(lowerClause, "\tstep ") {
			// No step clause found, add default step
			// Find position after "on expression" to insert step
			// Simple approach: find where to insert before | or end
			insertPos := endPos
			// Check if there's a "by" clause - insert step before it
			byIdx := strings.LastIndex(lowerClause, " by ")
			if byIdx != -1 {
				insertPos = pos + byIdx
			}
			b.WriteString(query[lastCopied:insertPos])
			b.WriteString(" step 1h")
			lastCopied = insertPos
			changed = true
		}

		searchFrom = pos + 11 // move past "make-series"
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// splitByTopLevelComma splits a string by commas that are not inside parentheses
func splitByTopLevelComma(s string) []string {
	var parts []string
	var current strings.Builder
	parenDepth := 0

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '(' {
			parenDepth++
		} else if c == ')' {
			parenDepth--
		} else if c == ',' && parenDepth == 0 {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	parts = append(parts, current.String())
	return parts
}

// stripUnionFunctionParams removes function call parameters from union statements
// The grammar doesn't support: union Func('a'), Func2('b')
// Converts to: union Func, Func2
func stripUnionFunctionParams(query string) string {
	result := query
	lowerResult := strings.ToLower(result)

	// Find "union" keyword, outside strings, followed by any whitespace
	idx := indexKeywordOutsideStrings(lowerResult, "union", 0)
	if idx == -1 {
		return result
	}

	// Check that union is followed by whitespace
	afterUnion := idx + 5
	if afterUnion >= len(result) {
		return result
	}
	ws := result[afterUnion]
	if ws != ' ' && ws != '\t' && ws != '\n' && ws != '\r' {
		return result
	}

	// Start scanning after "union" + whitespace
	var newResult strings.Builder
	newResult.WriteString(result[:afterUnion+1]) // "union" + whitespace char

	i := afterUnion + 1
	inString := false
	stringChar := byte(0)
	afterEquals := false // Track if we just processed an = (next identifier is a value, not a function)

	for i < len(result) {
		c := result[i]

		// Track strings
		if !inString && (c == '"' || c == '\'') {
			inString = true
			stringChar = c
			newResult.WriteByte(c)
			i++
			afterEquals = false
			continue
		}

		if inString {
			newResult.WriteByte(c)
			if c == stringChar {
				if stringChar == '"' {
					backslashes := 0
					for j := i - 1; j >= 0 && result[j] == '\\'; j-- {
						backslashes++
					}
					if backslashes%2 == 0 {
						inString = false
					}
				} else {
					inString = false
				}
			}
			i++
			continue
		}

		// Track = for key=value pairs
		if c == '=' {
			newResult.WriteByte(c)
			i++
			afterEquals = true
			continue
		}

		// Stop when we hit a pipe (end of union operands)
		if c == '|' {
			newResult.WriteString(result[i:])
			return newResult.String()
		}

		// Look for function call pattern: identifier followed by (
		if isIdentChar(c) {
			// Scan the identifier
			identStart := i
			for i < len(result) && isIdentChar(result[i]) {
				i++
			}
			ident := result[identStart:i]
			lowerIdent := strings.ToLower(ident)

			// Remember position after identifier, before skipping whitespace
			afterIdent := i

			// Skip whitespace to check for ( - but NOT newlines
			// A newline before ( suggests a subquery, not a function call
			for i < len(result) && (result[i] == ' ' || result[i] == '\t') {
				i++
			}

			// Check if followed by ( (on same line - no newline was crossed)
			// But don't treat as function call if:
			// - It's right after = (it's a value in key=value, like isfuzzy=true)
			// - It's a KQL keyword like true/false
			isValueOrKeyword := afterEquals || lowerIdent == "true" || lowerIdent == "false" || lowerIdent == "union"
			if i < len(result) && result[i] == '(' && !isValueOrKeyword {
				// This is a function call - skip the parameters
				depth := 1
				i++ // skip opening (
				for i < len(result) && depth > 0 {
					ch := result[i]
					if !inString {
						if ch == '"' || ch == '\'' {
							inString = true
							stringChar = ch
						} else if ch == '(' {
							depth++
						} else if ch == ')' {
							depth--
						}
					} else {
						if ch == stringChar {
							if stringChar == '"' && i > 0 && result[i-1] == '\\' {
								// escaped
							} else {
								inString = false
							}
						}
					}
					i++
				}
				// Write just the function name (no params)
				newResult.WriteString(ident)
				afterEquals = false
				continue
			}

			// Not a function call, write the identifier and restore position
			// (so whitespace after identifier is preserved)
			newResult.WriteString(ident)
			i = afterIdent
			afterEquals = false
			continue
		}

		// Reset afterEquals for other characters (comma, whitespace, etc.)
		if c != ' ' && c != '\t' && c != '\n' {
			afterEquals = false
		}

		newResult.WriteByte(c)
		i++
	}

	return newResult.String()
}

// asimEntry holds a pre-computed lowercase ASIM function name and its length.
type asimEntry struct {
	lower string
	len   int
}

// asimByFirstByte groups ASIM function names by their lowercase first byte for
// O(1) lookup at each query position. Within each group entries are sorted
// longest-first so the greedy match is always correct.
var asimByFirstByte [256][]asimEntry

func init() {
	asimFunctions := []string{
		"imAuthentication", "imProcess", "imNetworkSession", "imDns",
		"imWebSession", "imFileEvent", "imProcessCreate", "imProcessTerminate",
		"imRegistry", "imAuditEvent", "imUserManagement",
		"_Im_Authentication", "_Im_Process", "_Im_NetworkSession", "_Im_Dns",
		"_Im_WebSession", "_Im_FileEvent", "_Im_Registry", "_Im_AuditEvent",
		"_Im_ProcessCreate", "_Im_ProcessTerminate", "_Im_UserManagement",
		// Sentinel watchlist and other underscore-prefixed functions
		"_GetWatchlist", "_GetWatchlistAlias",
	}

	for _, fn := range asimFunctions {
		lower := strings.ToLower(fn)
		fb := lower[0]
		asimByFirstByte[fb] = append(asimByFirstByte[fb], asimEntry{lower: lower, len: len(lower)})
	}
	// Sort each bucket longest-first so greedy matching picks the longest name.
	for i := range asimByFirstByte {
		bucket := asimByFirstByte[i]
		if len(bucket) > 1 {
			sort.Slice(bucket, func(a, b int) bool {
				return bucket[a].len > bucket[b].len
			})
		}
	}
}

// replaceASIMFunctions replaces ASIM function calls with dummy table references.
// Handles both simple calls (imAuthentication) and parameterised calls
// (_Im_Dns(param=value)) in a single pass over the query.
func replaceASIMFunctions(query string) string {
	lowerQuery := strings.ToLower(query)
	qLen := len(query)

	var b strings.Builder
	b.Grow(qLen)
	lastCopied := 0
	changed := false

	for i := 0; i < qLen; {
		// Word-boundary check before: the character preceding this position
		// must not be an identifier character.
		if i > 0 && isIdentChar(query[i-1]) {
			i++
			continue
		}

		// Look up candidate ASIM names by the lowercase byte at position i.
		bucket := asimByFirstByte[lowerQuery[i]]
		if len(bucket) == 0 {
			i++
			continue
		}

		matched := false
		for _, entry := range bucket {
			end := i + entry.len
			if end > qLen {
				continue
			}
			if lowerQuery[i:end] != entry.lower {
				continue
			}

			// We have a case-insensitive match.  Decide whether this is a
			// function-call form  func(...)  or a simple (bare) usage.

			// Skip optional whitespace after the function name to look for '('.
			parenPos := end
			for parenPos < qLen && (query[parenPos] == ' ' || query[parenPos] == '\t' || query[parenPos] == '\n') {
				parenPos++
			}

			if parenPos < qLen && query[parenPos] == '(' {
				// ── function-call form: replace func(...) ──
				depth := 1
				closeParen := parenPos + 1
				for closeParen < qLen && depth > 0 {
					if query[closeParen] == '(' {
						depth++
					} else if query[closeParen] == ')' {
						depth--
					}
					closeParen++
				}
				if depth != 0 {
					// Unbalanced parens — skip this match entirely.
					i = end
					matched = true
					break
				}

				b.WriteString(query[lastCopied:i])
				b.WriteString("ASIMResult")
				lastCopied = closeParen
				i = closeParen
				changed = true
				matched = true
				break
			}

			// ── simple (bare) form ──
			// Word-boundary check after: next char must not be an ident char
			// (and must not be '(' which was already handled above).
			if end < qLen && isIdentChar(query[end]) {
				continue // partial match of a longer identifier — try next entry
			}

			b.WriteString(query[lastCopied:i])
			b.WriteString("ASIMResult")
			lastCopied = end
			i = end
			changed = true
			matched = true
			break
		}

		if !matched {
			i++
		}
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// replaceDatatableWithData replaces datatable(...) [ data ] with a dummy table reference
// This handles inline data definitions that conflict with the lexer's QUOTED_IDENTIFIER rule
func replaceDatatableWithData(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		idx := strings.Index(lowerQuery[searchFrom:], "datatable")
		if idx == -1 {
			break
		}
		idx += searchFrom

		// Find the opening paren
		openParenRel := strings.Index(query[idx:], "(")
		if openParenRel == -1 {
			break
		}
		openParen := idx + openParenRel

		// Find the matching closing paren for the column definition
		depth := 1
		closeParen := openParen + 1
		for closeParen < len(query) && depth > 0 {
			if query[closeParen] == '(' {
				depth++
			} else if query[closeParen] == ')' {
				depth--
			}
			closeParen++
		}
		if depth != 0 {
			break
		}

		// Now look for the data block: [ ... ]
		// Skip whitespace after closing paren
		dataStart := closeParen
		for dataStart < len(query) && (query[dataStart] == ' ' || query[dataStart] == '\t' || query[dataStart] == '\n' || query[dataStart] == '\r') {
			dataStart++
		}

		// Check if there's a bracket data block
		if dataStart < len(query) && query[dataStart] == '[' {
			// Find the closing bracket
			bracketDepth := 1
			dataEnd := dataStart + 1
			inString := false
			stringChar := byte(0)
			for dataEnd < len(query) && bracketDepth > 0 {
				c := query[dataEnd]
				if !inString {
					if c == '"' || c == '\'' {
						inString = true
						stringChar = c
					} else if c == '[' {
						bracketDepth++
					} else if c == ']' {
						bracketDepth--
					}
				} else {
					if c == stringChar && (dataEnd == 0 || query[dataEnd-1] != '\\') {
						inString = false
					}
				}
				dataEnd++
			}

			if bracketDepth == 0 {
				// Replace datatable(...) [ data ] with DatatableResult
				b.WriteString(query[lastCopied:idx])
				b.WriteString("DatatableResult")
				lastCopied = dataEnd
				searchFrom = dataEnd
				changed = true
				continue
			}
		}

		// No bracket data, just move past this datatable
		break
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// replaceArgFunction replaces arg("...").Table patterns with just Table
// Azure Resource Graph uses arg("subscription-id").Resources for cross-workspace queries
func replaceArgFunction(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		// Find arg( at word boundary
		idx := strings.Index(lowerQuery[searchFrom:], "arg(")
		if idx == -1 {
			break
		}
		idx += searchFrom

		// Check it's at word boundary
		if idx > 0 && isIdentChar(query[idx-1]) {
			// Not a standalone arg(...) call; leave the identifier unchanged.
			searchFrom = idx + len("arg")
			continue
		}

		// Find the closing paren of arg(...)
		openParen := idx + 3 // position of (
		depth := 1
		closeParen := openParen + 1
		for closeParen < len(query) && depth > 0 {
			if query[closeParen] == '(' {
				depth++
			} else if query[closeParen] == ')' {
				depth--
			} else if query[closeParen] == '"' {
				// Skip string
				closeParen++
				for closeParen < len(query) && query[closeParen] != '"' {
					if query[closeParen] == '\\' {
						closeParen++
					}
					closeParen++
				}
			}
			closeParen++
		}
		if depth != 0 {
			break
		}

		// Check if followed by .
		if closeParen < len(query) && query[closeParen] == '.' {
			// Remove the entire arg(...). prefix — keep what's after the dot
			b.WriteString(query[lastCopied:idx])
			lastCopied = closeParen + 1
			searchFrom = closeParen + 1
		} else {
			// No dot after, replace with ArgResult
			b.WriteString(query[lastCopied:idx])
			b.WriteString("ArgResult")
			lastCopied = closeParen
			searchFrom = closeParen
		}
		changed = true
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// replaceExternalData replaces externaldata(...) [...] with (...) with a dummy table reference
// Full syntax: externaldata (Column:Type, ...) [ @"url" ] with (option=value)
func replaceExternalData(query string) string {
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		idx := indexKeywordOutsideStrings(query, "externaldata", searchFrom)
		if idx == -1 {
			break
		}

		openParen := idx + len("externaldata")
		for openParen < len(query) && (query[openParen] == ' ' || query[openParen] == '\t' || query[openParen] == '\n' || query[openParen] == '\r') {
			openParen++
		}
		if openParen >= len(query) || query[openParen] != '(' {
			searchFrom = idx + len("externaldata")
			continue
		}

		closeParen := findMatchingParen(query, openParen)
		if closeParen == -1 {
			break
		}

		// Now look for the data source block: [ ... ]
		end := closeParen + 1
		// Skip whitespace
		for end < len(query) && (query[end] == ' ' || query[end] == '\t' || query[end] == '\n' || query[end] == '\r') {
			end++
		}
		// Check for [ ... ] data source
		if end < len(query) && query[end] == '[' {
			bracketDepth := 1
			end++
			for end < len(query) && bracketDepth > 0 {
				if query[end] == '[' {
					bracketDepth++
				} else if query[end] == ']' {
					bracketDepth--
				}
				end++
			}
			// Skip whitespace after ]
			for end < len(query) && (query[end] == ' ' || query[end] == '\t' || query[end] == '\n' || query[end] == '\r') {
				end++
			}
			// Check for 'with' clause: with (...)
			if end+4 <= len(query) && strings.EqualFold(query[end:end+4], "with") {
				end += 4
				// Skip whitespace
				for end < len(query) && (query[end] == ' ' || query[end] == '\t' || query[end] == '\n' || query[end] == '\r') {
					end++
				}
				// Find ( ... )
				if end < len(query) && query[end] == '(' {
					if closeWith := findMatchingParen(query, end); closeWith != -1 {
						end = closeWith + 1
					}
				}
			}
		}

		// Replace entire externaldata expression with ExtDataResult
		b.WriteString(query[lastCopied:idx])
		b.WriteString("ExtDataResult")
		lastCopied = end
		searchFrom = end
		changed = true
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// stripDocumentationPreamble removes documentation wrappers from queries
// Some queries are wrapped with "Description:...Query:..." headers
func stripDocumentationPreamble(query string) string {
	// Process line by line, skipping documentation and finding where KQL starts
	lines := strings.Split(query, "\n")
	for i, line := range lines {
		lineTrimmed := strings.TrimSpace(line)
		lowerLine := strings.ToLower(lineTrimmed)
		for _, marker := range []string{"query:", "kql query:", "kql:"} {
			if lowerLine == marker {
				return strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			}
			if strings.HasPrefix(lowerLine, marker+" ") {
				markerStart := strings.Index(strings.ToLower(line), marker)
				if markerStart == -1 {
					continue
				}
				afterMarker := strings.TrimSpace(line[markerStart+len(marker):])
				rest := strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
				if rest == "" {
					return afterMarker
				}
				return strings.TrimSpace(afterMarker + "\n" + rest)
			}
		}
	}

	for i, line := range lines {
		lineTrimmed := strings.TrimSpace(line)
		lowerLine := strings.ToLower(lineTrimmed)

		// Skip empty lines
		if lineTrimmed == "" {
			continue
		}

		// Skip comment lines (// or #)
		if strings.HasPrefix(lineTrimmed, "//") || strings.HasPrefix(lineTrimmed, "#") {
			continue
		}

		// Parenthesized tabular expressions are common in KQL, especially
		// `(union ...)` and `(Table | where ...)` query roots.
		if strings.HasPrefix(lineTrimmed, "(") {
			return strings.Join(lines[i:], "\n")
		}

		// Skip documentation-style lines (Key: Value pattern)
		firstColonIdx := strings.Index(lineTrimmed, ":")
		if firstColonIdx > 0 {
			before := lineTrimmed[:firstColonIdx]
			// If the part before the FIRST colon is a simple word, it's likely documentation
			// This catches "Link: https://..." and "Description: some text" patterns
			if !strings.ContainsAny(before, " |()=<>\"'") {
				continue
			}
		}

		// Check if this line looks like natural language (multiple words that form a sentence)
		// Natural language sentences typically have many words and don't look like KQL
		isNaturalLanguage := false
		wordCount := strings.Count(lineTrimmed, " ") + 1

		// Lines with many words (>6) that don't start with a KQL operator are likely natural language.
		if wordCount > 6 && !strings.HasPrefix(lineTrimmed, "|") && !hasKQLStarterPrefix(lowerLine) {
			// Check it doesn't look like KQL with lots of conditions
			// KQL lines with many words typically have operators like ==, contains, in, etc.
			hasKQLOperator := strings.Contains(lineTrimmed, "==") ||
				strings.Contains(lineTrimmed, "!=") ||
				strings.Contains(lowerLine, " contains ") ||
				strings.Contains(lowerLine, " in ") ||
				strings.Contains(lowerLine, " startswith ") ||
				strings.Contains(lowerLine, " endswith ")
			if !hasKQLOperator {
				isNaturalLanguage = true
			}
		}

		// Also check for lines ending with punctuation that suggests natural language
		lastChar := lineTrimmed[len(lineTrimmed)-1]
		if (lastChar == ':' || lastChar == '.') && wordCount >= 4 && !hasKQLStarterPrefix(lowerLine) {
			isNaturalLanguage = true
		}

		if isNaturalLanguage {
			continue
		}

		// Check if this could be the start of KQL
		isLikelyKQL := false

		// Lines starting with | are definitely KQL pipeline operators
		if strings.HasPrefix(lineTrimmed, "|") || strings.HasPrefix(lineTrimmed, "| ") {
			isLikelyKQL = true
		}

		if hasKQLStarterPrefix(lowerLine) {
			isLikelyKQL = true
		}

		// Table name pattern: identifier possibly followed by | or newline
		// Table names are typically PascalCase or snake_case with no spaces before special chars
		if !isLikelyKQL && len(lineTrimmed) > 0 {
			firstChar := lineTrimmed[0]
			if (firstChar >= 'A' && firstChar <= 'Z') || (firstChar >= 'a' && firstChar <= 'z') || firstChar == '_' {
				// Check for table name pattern: identifier followed by whitespace/pipe/newline
				// and NOT followed by common English words that indicate it's a sentence
				words := strings.Fields(lineTrimmed)
				if len(words) >= 1 {
					// If first word looks like a table name (PascalCase, camelCase, contains underscore, or ends with common patterns)
					firstWord := words[0]
					looksLikeTable := false

					// Camel/Pascal case pattern (has at least one capital anywhere)
					hasCapital := false
					for _, c := range firstWord {
						if c >= 'A' && c <= 'Z' {
							hasCapital = true
							break
						}
					}
					if hasCapital && !strings.Contains(firstWord, " ") && len(firstWord) >= 3 {
						looksLikeTable = true
					}

					// Contains underscore (common in table names)
					if strings.Contains(firstWord, "_") {
						looksLikeTable = true
					}

					// Ends with common table name patterns
					tableSuffixes := []string{"Events", "Logs", "Data", "Table", "CL", "Info", "Records"}
					for _, suffix := range tableSuffixes {
						if strings.HasSuffix(firstWord, suffix) {
							looksLikeTable = true
							break
						}
					}

					// Check that the second word (if exists) isn't a common English word
					if looksLikeTable && len(words) >= 2 {
						commonEnglishStarters := []string{"using", "can", "to", "the", "is", "are", "for", "in", "on", "with", "and", "or", "but", "if", "this", "that", "these", "those", "will", "should", "could", "would", "may", "might"}
						secondWord := strings.ToLower(words[1])
						for _, eng := range commonEnglishStarters {
							if secondWord == eng {
								looksLikeTable = false
								break
							}
						}
					}

					if looksLikeTable {
						isLikelyKQL = true
					}
				}
			}
		}

		if isLikelyKQL {
			// Return from this line onwards
			return strings.Join(lines[i:], "\n")
		}
	}

	return query
}

func hasKQLStarterPrefix(lowerLine string) bool {
	kqlStarters := []string{"let", "union", "where", "search", "find", "range", "print", "datatable"}
	for _, starter := range kqlStarters {
		if lowerLine == starter || strings.HasPrefix(lowerLine, starter+" ") || strings.HasPrefix(lowerLine, starter+"\t") {
			return true
		}
	}
	return false
}

// stripDeclareStatements removes declare query_parameters(...) statements
// These define query parameters but aren't part of the actual query
func stripDeclareStatements(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	// Find "declare query_parameters"
	for {
		idx := strings.Index(lowerQuery[searchFrom:], "declare query_parameters")
		if idx == -1 {
			break
		}
		idx += searchFrom

		// Find the opening paren
		parenStart := idx + 24
		for parenStart < len(query) && query[parenStart] != '(' {
			parenStart++
		}
		if parenStart >= len(query) {
			break
		}

		// Find matching closing paren
		depth := 1
		parenEnd := parenStart + 1
		for parenEnd < len(query) && depth > 0 {
			if query[parenEnd] == '(' {
				depth++
			} else if query[parenEnd] == ')' {
				depth--
			}
			parenEnd++
		}

		// Find semicolon after close paren
		end := parenEnd
		for end < len(query) && (query[end] == ' ' || query[end] == '\t' || query[end] == '\n' || query[end] == '\r') {
			end++
		}
		if end < len(query) && query[end] == ';' {
			end++
		}

		// Remove the entire declare statement
		b.WriteString(query[lastCopied:idx])
		lastCopied = end
		searchFrom = end
		changed = true
	}

	if !changed {
		return strings.TrimLeft(query, " \t\n\r")
	}
	b.WriteString(query[lastCopied:])
	return strings.TrimLeft(b.String(), " \t\n\r")
}

// stripLeadingComments removes leading // comments from a query. Unless
// strict, it also removes what is not KQL: leading # comments and URL lines,
// and /| lines (a mistyped // commenting out a pipe stage).
func stripLeadingComments(query string, strict bool) string {
	lines := strings.Split(query, "\n")
	var result []string
	foundCode := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines, comment lines (// and #), and URL lines at the start
		if !foundCode {
			if trimmed == "" || strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strict {
				foundCode = true
				result = append(result, line)
				continue
			}
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			// Skip lines that are URLs (http:// or https://)
			lowerTrimmed := strings.ToLower(trimmed)
			if strings.HasPrefix(lowerTrimmed, "http://") || strings.HasPrefix(lowerTrimmed, "https://") {
				continue
			}
			foundCode = true
		}

		// After we've found code, also strip commented-out code lines
		// that start with /| (common mistake: / instead of // to comment out a pipe statement)
		trimmedLine := strings.TrimSpace(line)
		if !strict && strings.HasPrefix(trimmedLine, "/|") {
			// This is a commented-out pipe statement, skip it
			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// replaceParameters replaces {Parameter} placeholders with dummy values
func replaceParameters(query string) string {
	// Common parameter patterns (non-time-related)
	replacements := []struct {
		param string
		value string
	}{
		{param: "{SourceTable}", value: "SecurityEvent"},
		{param: "{TotalE5Seats}", value: "100"},
		{param: "{Workspace}", value: "workspace"},
		{param: "{Subscription}", value: "subscription"},
	}

	result := query

	// Handle double curly braces {{param}} (Jinja2/Sentinel/Logic Apps style)
	// Convert to single curly braces first, then let regular handling take over
	result = collapseDoubleCurlyParametersOutsideStrings(result)

	for _, replacement := range replacements {
		result = replaceAllOutsideStrings(result, replacement.param, replacement.value)
	}

	// Handle time range parameters with context awareness
	// {TimeRange} after a time field should become "> ago(1d)" not just "1d"
	result = replaceTimeRangeParams(result)

	// Generic replacement for any remaining {param} patterns
	// Replace {word} with a dummy identifier or expression depending on context
	result = replaceGenericParametersOutsideStrings(result)

	return result
}

func collapseDoubleCurlyParametersOutsideStrings(query string) string {
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0
	for {
		start := indexOutsideStrings(query, "{{", searchFrom, false)
		if start == -1 {
			break
		}
		end := indexOutsideStrings(query, "}}", start+2, false)
		if end == -1 {
			break
		}

		b.WriteString(query[lastCopied:start])
		b.WriteByte('{')
		b.WriteString(query[start+2 : end])
		b.WriteByte('}')
		lastCopied = end + 2
		searchFrom = lastCopied
		changed = true
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

func replaceAllOutsideStrings(query, old, new string) string {
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0
	for {
		idx := indexOutsideStrings(query, old, searchFrom, false)
		if idx == -1 {
			break
		}
		b.WriteString(query[lastCopied:idx])
		b.WriteString(new)
		lastCopied = idx + len(old)
		searchFrom = lastCopied
		changed = true
	}
	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

func replaceGenericParametersOutsideStrings(query string) string {
	result := query
	searchFrom := 0
	for {
		start := indexByteOutsideStrings(result, '{', searchFrom)
		if start == -1 {
			break
		}
		end := indexByteOutsideStrings(result, '}', start+1)
		if end == -1 {
			break
		}

		inner := result[start+1 : end]
		if !isValidParamName(inner) {
			searchFrom = start + 1
			continue
		}

		replacement := "DummyParam"
		lowerInner := strings.ToLower(inner)
		if strings.Contains(lowerInner, "time") || strings.Contains(lowerInner, "timer") ||
			strings.Contains(lowerInner, "date") || strings.Contains(lowerInner, "range") {
			beforeParam := strings.TrimSpace(result[:start])
			lastWord := getLastWord(beforeParam)
			if strings.Contains(strings.ToLower(lastWord), "time") ||
				strings.Contains(strings.ToLower(lastWord), "timestamp") ||
				strings.Contains(strings.ToLower(lastWord), "date") {
				replacement = "> ago(1d)"
			}
		}

		result = result[:start] + replacement + result[end+1:]
		searchFrom = start + len(replacement)
	}
	return result
}

// replaceTimeRangeParams handles {TimeRange} and similar time placeholders with context
func replaceTimeRangeParams(query string) string {
	result := query
	timeParams := []string{"{TimeRange}", "{timerange}", "{Timerange}", "{timeRange}"}

	for _, param := range timeParams {
		var b strings.Builder
		b.Grow(len(result))
		lastCopied := 0
		changed := false
		searchFrom := 0

		for {
			idx := indexOutsideStrings(result, param, searchFrom, false)
			if idx == -1 {
				break
			}

			// Check context - what's before the parameter?
			beforeParam := strings.TrimSpace(result[:idx])
			lastWord := getLastWord(beforeParam)
			lowerLastWord := strings.ToLower(lastWord)

			// If preceded by a time-related field name, use comparison operator
			var replacement string
			if strings.Contains(lowerLastWord, "time") ||
				strings.Contains(lowerLastWord, "timestamp") ||
				strings.Contains(lowerLastWord, "date") ||
				strings.HasSuffix(lowerLastWord, "generated") {
				replacement = "> ago(1d)"
			} else {
				// Generic replacement
				replacement = "1d"
			}

			b.WriteString(result[lastCopied:idx])
			b.WriteString(replacement)
			lastCopied = idx + len(param)
			searchFrom = lastCopied
			changed = true
		}

		if changed {
			b.WriteString(result[lastCopied:])
			result = b.String()
		}
	}

	return result
}

// renameReservedFieldNames replaces reserved keywords used as field names with safe alternatives
func renameReservedFieldNames(query string) string {
	// Reserved words that are sometimes used as field names
	reserved := map[string]string{
		"pattern": "_pattern_",
		"nodes":   "_nodes_",
	}

	result := query
	for word, replacement := range reserved {
		// Match word when preceded by whitespace, comma, ( and followed by operators or whitespace
		result = renameFieldWord(result, word, replacement)
	}
	return result
}

// renameFieldWord replaces a word when used as a field name (not as a keyword)
func renameFieldWord(query, word, replacement string) string {
	wordLen := len(word)
	var b strings.Builder
	b.Grow(len(query) + 32) // allow some growth for replacements
	lastCopied := 0
	changed := false

	// Scan through the query looking for the word
	i := 0
	for i < len(query)-wordLen {
		// Check if we have the word at position i (case-insensitive)
		if strings.EqualFold(query[i:i+wordLen], word) {
			// Check preceding character - should be whitespace, comma, or operator
			prevOk := i == 0 || query[i-1] == ' ' || query[i-1] == '\t' || query[i-1] == '\n' ||
				query[i-1] == ',' || query[i-1] == '(' || query[i-1] == '|'

			// Check following character - should be whitespace or operator (not alphanumeric)
			afterIdx := i + wordLen
			afterOk := afterIdx >= len(query) ||
				query[afterIdx] == ' ' || query[afterIdx] == '\t' || query[afterIdx] == '\n' ||
				query[afterIdx] == '!' || query[afterIdx] == '=' || query[afterIdx] == ',' ||
				query[afterIdx] == ')' || query[afterIdx] == '|' || query[afterIdx] == '.' ||
				query[afterIdx] == '[' || query[afterIdx] == '<' || query[afterIdx] == '>'

			if prevOk && afterOk {
				b.WriteString(query[lastCopied:i])
				b.WriteString(replacement)
				lastCopied = afterIdx
				i = afterIdx
				changed = true
				continue
			}
		}
		i++
	}
	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// getLastWord extracts the last identifier/word from a string
func getLastWord(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return ""
	}
	// Find the start of the last word
	end := len(s)
	for end > 0 && (s[end-1] >= 'a' && s[end-1] <= 'z' || s[end-1] >= 'A' && s[end-1] <= 'Z' ||
		s[end-1] >= '0' && s[end-1] <= '9' || s[end-1] == '_') {
		end--
	}
	return s[end:]
}

// normalizeFindOperator converts find statements to standard where clauses
// find in (T1, T2, ...) where condition | ... -> DummyTable | where condition | ...
// find in (T1, T2) where SHA1 == "x" -> DummyTable | where SHA1 == "x"
// find in (T1) where x project a, b -> DummyTable | where x | project a, b
func normalizeFindOperator(query string) string {
	lowerQuery := strings.ToLower(strings.TrimSpace(query))

	// Check if query starts with "find"
	if !strings.HasPrefix(lowerQuery, "find") {
		return query
	}

	// Find the position of "where" keyword
	whereIdx := strings.Index(lowerQuery, " where ")
	if whereIdx == -1 {
		whereIdx = strings.Index(lowerQuery, "\twhere ")
		if whereIdx == -1 {
			whereIdx = strings.Index(lowerQuery, "\nwhere ")
		}
	}

	if whereIdx == -1 {
		// No where clause, the find statement is just table searching
		// Convert to DummyTable with no conditions
		return "DummyTable"
	}

	// Extract everything from "where" onwards
	afterWhere := query[whereIdx+1:]

	// Handle "project" clause in find operator (no pipe needed in original)
	// find ... where condition project columns -> ... where condition | project columns
	afterWhereLower := strings.ToLower(afterWhere)
	projectIdx := strings.Index(afterWhereLower, " project ")
	if projectIdx == -1 {
		projectIdx = strings.Index(afterWhereLower, "\nproject ")
	}
	if projectIdx == -1 {
		projectIdx = strings.Index(afterWhereLower, "\tproject ")
	}

	// Check that project is not already preceded by a pipe
	if projectIdx != -1 {
		// Look back from projectIdx to see if there's a pipe
		hasPipe := false
		for i := projectIdx - 1; i >= 0; i-- {
			if afterWhere[i] == '|' {
				hasPipe = true
				break
			}
			if afterWhere[i] != ' ' && afterWhere[i] != '\t' && afterWhere[i] != '\n' {
				break
			}
		}
		if !hasPipe {
			// Insert a pipe before project
			afterWhere = afterWhere[:projectIdx] + " |" + afterWhere[projectIdx:]
		}
	}

	return "DummyTable | " + afterWhere
}

// normalizeTopNested converts top-nested clauses to simpler summarize form
// The top-nested operator has complex hierarchical semantics that aren't needed for condition extraction
// We simplify: find the entire top-nested chain and replace with summarize count()
func normalizeTopNested(query string) string {
	result := query

	// Keep processing until no more top-nested clauses with "of"
	for {
		lowerResult := strings.ToLower(result)
		idx := strings.Index(lowerResult, "top-nested")
		if idx == -1 {
			break
		}

		// Check if this has "of" keyword (the syntax we need to normalize)
		afterTopNested := lowerResult[idx+10:] // skip "top-nested"

		// Skip optional number and whitespace between "top-nested" and "of"
		j := 0
		for j < len(afterTopNested) && (afterTopNested[j] == ' ' || afterTopNested[j] == '\t' ||
			(afterTopNested[j] >= '0' && afterTopNested[j] <= '9')) {
			j++
		}

		if !strings.HasPrefix(afterTopNested[j:], "of ") {
			// No "of" found, this might be standard top-nested syntax - leave as is
			break
		}

		// Find the end of the entire top-nested chain
		// A top-nested chain ends at a pipe (|) that's followed by something other than top-nested
		// or at EOF
		endIdx := idx + 10 // start after "top-nested"
		depth := 0
		inComment := false

		for endIdx < len(result) {
			// Check for comment start
			if endIdx+1 < len(result) && result[endIdx] == '/' && result[endIdx+1] == '/' {
				// Skip to end of line
				for endIdx < len(result) && result[endIdx] != '\n' {
					endIdx++
				}
				continue
			}

			ch := result[endIdx]
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
			} else if ch == '|' && depth == 0 && !inComment {
				// Check if this pipe is followed by another top-nested
				afterPipe := strings.TrimSpace(result[endIdx+1:])
				lowerAfterPipe := strings.ToLower(afterPipe)

				// Skip comments at the start of the next part
				for strings.HasPrefix(lowerAfterPipe, "//") {
					nlIdx := strings.Index(afterPipe, "\n")
					if nlIdx == -1 {
						break
					}
					afterPipe = strings.TrimSpace(afterPipe[nlIdx+1:])
					lowerAfterPipe = strings.ToLower(afterPipe)
				}

				if !strings.HasPrefix(lowerAfterPipe, "top-nested") {
					// This pipe ends the top-nested chain
					break
				}
			}
			endIdx++
		}

		// Extract what's after the top-nested chain (including the pipe if present)
		afterChain := ""
		if endIdx < len(result) {
			afterChain = result[endIdx:]
		}

		// Check if the pipe was the end marker
		if len(afterChain) > 0 && afterChain[0] == '|' {
			// Keep the pipe and what follows
		} else {
			// No pipe found, nothing after
			afterChain = ""
		}

		// Build replacement: summarize count()
		// This is a simplified replacement that allows the query to parse
		replacement := "summarize count()"

		// Construct new result
		newResult := result[:idx] + replacement + afterChain

		// Verify we made progress to avoid infinite loop
		if newResult == result {
			break
		}
		result = newResult
	}

	return result
}

func normalizePartitionOperator(query string) string {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		rel := strings.Index(lowerQuery[searchFrom:], "| partition")
		if rel == -1 {
			break
		}
		idx := searchFrom + rel
		partitionStart := idx + len("| partition")
		i := partitionStart
		for i < len(query) && (query[i] == ' ' || query[i] == '\t' || query[i] == '\n' || query[i] == '\r') {
			i++
		}

		if !hasKeywordPrefix(lowerQuery[i:], "by") {
			searchFrom = partitionStart
			continue
		}
		i += len("by")
		for i < len(query) && (query[i] == ' ' || query[i] == '\t' || query[i] == '\n' || query[i] == '\r') {
			i++
		}

		keyStart := i
		for i < len(query) && query[i] != '(' && query[i] != '|' {
			i++
		}
		if i >= len(query) || query[i] != '(' {
			searchFrom = partitionStart
			continue
		}

		key := strings.TrimSpace(query[keyStart:i])
		if key == "" {
			searchFrom = partitionStart
			continue
		}

		close := findMatchingParen(query, i)
		if close <= i {
			searchFrom = partitionStart
			continue
		}

		b.WriteString(query[lastCopied:idx])
		b.WriteString("| summarize count() by ")
		b.WriteString(key)
		lastCopied = close + 1
		searchFrom = close + 1
		changed = true
	}

	if !changed {
		return query
	}
	b.WriteString(query[lastCopied:])
	return b.String()
}

// normalizeSearchOperator converts search statements to standard where clauses
// search in (T1, T2) "pattern" -> DummyTable | where * contains "pattern"
// search kind=case_sensitive in (T) "pattern" -> DummyTable | where * contains "pattern"
// search "pattern" -> DummyTable | where * contains "pattern"
func normalizeSearchOperator(query string) string {
	trimmed := strings.TrimSpace(query)
	lowerQuery := strings.ToLower(trimmed)

	// Check if query starts with "search"
	if !strings.HasPrefix(lowerQuery, "search") {
		return query
	}

	// Skip past "search" keyword
	i := 6

	// Skip any "kind=..." parameter
	rest := strings.TrimSpace(trimmed[i:])
	lowerRest := strings.ToLower(rest)
	if strings.HasPrefix(lowerRest, "kind=") {
		// Skip past kind=value
		spaceIdx := strings.IndexAny(rest, " \t\n")
		if spaceIdx == -1 {
			return query // malformed
		}
		rest = strings.TrimSpace(rest[spaceIdx:])
		lowerRest = strings.ToLower(rest)
	}

	// Skip "in (tables)" if present
	if strings.HasPrefix(lowerRest, "in ") || strings.HasPrefix(lowerRest, "in(") {
		// Find the closing paren
		openIdx := strings.Index(rest, "(")
		if openIdx != -1 {
			depth := 1
			closeIdx := openIdx + 1
			for closeIdx < len(rest) && depth > 0 {
				if rest[closeIdx] == '(' {
					depth++
				} else if rest[closeIdx] == ')' {
					depth--
				}
				closeIdx++
			}
			if depth == 0 {
				rest = strings.TrimSpace(rest[closeIdx:])
			}
		}
	}

	// Now rest should be the search pattern (e.g., "JIT", '*pattern*')
	// or a pipe followed by more operators
	if len(rest) == 0 {
		return "DummyTable"
	}

	// If there's a search pattern, the grammar expects "search expression"
	// but the grammar's search isn't being recognized, so just skip search entirely
	// and convert to a dummy table with any following pipe stages

	if strings.HasPrefix(rest, "|") {
		// There are more pipe stages, use them
		return "DummyTable " + rest
	}

	// The rest is a search expression - just use DummyTable
	// We can't easily extract conditions from search patterns
	return "DummyTable"
}

// isValidParamName checks if a string looks like a valid parameter name
// Allows alphanumeric, underscore, colon (for workbook params like {_TimeRange:value})
func isValidParamName(s string) bool {
	if len(s) == 0 || len(s) > 50 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == ':') {
			return false
		}
	}
	return true
}

// NormalizeQueryForDebug returns the normalized query for debugging purposes
func NormalizeQueryForDebug(query string) string {
	return normalizeQuery(query)
}

// ExtractConditions parses a KQL query and extracts all field conditions.
// Uses a timeout (MaxParseTime) to abort queries that cause the parser to hang
// on deeply nested expressions. Recovers from panics.
func ExtractConditions(query string) *ParseResult {
	ch := make(chan *ParseResult, 1)
	go func() {
		ch <- extractConditionsInternal(query)
	}()

	select {
	case result := <-ch:
		return result
	case <-time.After(MaxParseTime):
		return &ParseResult{
			Conditions:  []Condition{},
			DataSources: []string{},
			Commands:    []string{},
			Errors:      []string{fmt.Sprintf("parser timeout: query took longer than %s to parse", MaxParseTime)},
		}
	}
}

func extractConditionsInternal(query string) (result *ParseResult) {
	defer func() {
		if r := recover(); r != nil {
			result = &ParseResult{
				Conditions:  []Condition{},
				DataSources: []string{},
				Commands:    []string{},
				Errors:      []string{fmt.Sprintf("parser panic: %v", r)},
			}
		}
	}()

	// Normalize the query to handle operators the parser doesn't fully support
	normalizedQuery := normalizeQuery(query)
	input := antlr.NewInputStream(normalizedQuery)
	lexer := NewKQLLexer(input)

	// Remove default error listener and add our own
	lexer.RemoveErrorListeners()
	lexerErrors := &errorListener{}
	lexer.AddErrorListener(lexerErrors)

	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := NewKQLParser(stream)

	// Remove default error listener and add our own
	parser.RemoveErrorListeners()
	parserErrors := &errorListener{}
	parser.AddErrorListener(parserErrors)

	// Parse the query
	tree := parser.Query()

	// Walk the tree to extract conditions
	extractor := &conditionExtractor{
		conditions:          make([]Condition, 0),
		computedFields:      make(map[string]string), // computed field -> source field
		computedExpressions: make(map[string]string), // computed field -> expression
		commands:            make([]string, 0),
		joins:               make([]JoinInfo, 0),
		queryPipeline:       -1,
		lastLogicalOp:       "AND", // default
		originalQuery:       normalizedQuery,
	}
	antlr.ParseTreeWalkerDefault.Walk(extractor, tree)

	// Combine errors
	allErrors := append(lexerErrors.errors, parserErrors.errors...)
	allErrors = append(allErrors, extractor.errors...)

	// Post-process to group OR conditions on same field. The expression tree
	// knows the grouping; the flat operator sequence cannot tell
	// "x and (a or b)" from "(x and a) or b", so it is only a fallback.
	rawConditions := append([]Condition(nil), extractor.conditions...)
	rawExpression := buildConditionExpression(tree, rawConditions)
	conditions, conditionIndexMap, grouped := groupORConditionsByExpression(rawConditions, rawExpression)
	if !grouped {
		conditions, conditionIndexMap = groupORConditionsWithIndexMap(extractor.conditions)
	}
	expression := remapConditionExpression(rawExpression, conditionIndexMap)
	if !hasPortableFieldConditions(conditions) {
		var note string
		var extracted []Condition
		extracted, note = extractPortableConditions(query, normalizedQuery)
		portableStart := len(conditions)
		for _, condition := range extracted {
			// The tree walker already captured conditions on computed
			// columns, marked as computed; the text fallback cannot tell
			// them from raw columns.
			if _, computed := extractor.computedFields[strings.ToLower(condition.Field)]; computed {
				continue
			}
			condition.Pipeline = extractor.queryPipeline
			if !containsCondition(conditions, condition) {
				conditions = append(conditions, condition)
			}
		}
		if expression != nil && len(conditions) > portableStart {
			expression = appendConditionRangeExpression(expression, portableStart, len(conditions))
		}
		if note != "" && len(conditions) > portableStart {
			allErrors = append(allErrors, note)
		}
	}
	if keyword := extractLeadingSearchKeyword(query); keyword != "" {
		condition := portableKeywordCondition(keyword)
		condition.Pipeline = extractor.queryPipeline
		if !containsCondition(conditions, condition) {
			portableStart := len(conditions)
			conditions = append(conditions, condition)
			if expression != nil {
				expression = appendConditionRangeExpression(expression, portableStart, len(conditions))
			}
			allErrors = append(allErrors, portableKeywordExtractionNote)
		}
	}

	return &ParseResult{
		Conditions:          conditions,
		Expression:          expression,
		DataSources:         extractPortableDataSources(query, normalizedQuery),
		LetStatements:       extractLetStatements(query),
		ComputedFields:      extractor.computedFields,
		ComputedExpressions: extractor.computedExpressions,
		GroupByFields:       extractor.groupByFields,
		GroupByExpressions:  extractor.groupByExpressions,
		Aggregations:        extractor.aggregations,
		Pipelines:           extractor.pipelines,
		Commands:            extractor.commands,
		ProjectedFields:     extractor.projectedFields,
		Joins:               extractor.joins,
		Errors:              allErrors,
	}
}

// EnterJoinOperator extracts join metadata and recursively parses the right side
func (e *conditionExtractor) EnterJoinOperator(ctx *JoinOperatorContext) {
	e.commands = append(e.commands, "join")

	info := JoinInfo{
		Type:      "innerunique", // KQL default
		PipeStage: e.currentStage,
	}

	// Extract join kind (e.g., kind=inner, kind=leftouter)
	if ctx.JoinKind() != nil {
		kindCtx := ctx.JoinKind()
		if kindCtx.JoinFlavor() != nil {
			info.Type = strings.ToLower(kindCtx.JoinFlavor().GetText())
		}
	}

	// Extract join condition fields from ON clause
	if ctx.JoinCondition() != nil {
		condCtx := ctx.JoinCondition()
		for _, attr := range condCtx.AllJoinAttribute() {
			identifiers := attr.AllIdentifier()
			// $left.fieldA == $right.fieldB form: has DOLLAR tokens and 2 identifiers
			if len(attr.AllDOLLAR()) >= 2 && len(identifiers) >= 2 {
				info.LeftFields = append(info.LeftFields, identifiers[0].GetText())
				info.RightFields = append(info.RightFields, identifiers[1].GetText())
			} else if len(identifiers) == 1 {
				// Simple field name (same on both sides)
				info.JoinFields = append(info.JoinFields, identifiers[0].GetText())
			}
		}
	}

	// Extract right-side table or subquery
	if ctx.TableName() != nil && ctx.LPAREN() == nil {
		// Simple table reference: join kind=inner TableName on field
		info.RightTable = ctx.TableName().GetText()
	} else if ctx.TabularExpression() != nil {
		// Subquery: join kind=inner (SubQuery | where ...) on field
		subText := e.extractTabularExpressionText(ctx.TabularExpression())
		if subText != "" {
			info.Subsearch = ExtractConditions(subText)
			allJoinFields := append(info.JoinFields, info.LeftFields...)
			info.ExposedFields = deriveExposedFields(info.Subsearch, allJoinFields)
		}
	}

	e.joins = append(e.joins, info)

	// Increment inSubquery so the tree walker skips conditions inside the join's right side
	// (they are already captured via recursive ExtractConditions in the Subsearch field)
	if ctx.TabularExpression() != nil {
		e.inSubquery++
	}
}

// ExitJoinOperator decrements subquery depth when leaving a join with a subquery
func (e *conditionExtractor) ExitJoinOperator(ctx *JoinOperatorContext) {
	if ctx.TabularExpression() != nil {
		e.inSubquery--
	}
}

// extractTabularExpressionText extracts the original query text for a tabular expression
func (e *conditionExtractor) extractTabularExpressionText(ctx ITabularExpressionContext) string {
	if ctx == nil {
		return ""
	}
	start := ctx.GetStart()
	stop := ctx.GetStop()
	if start == nil || stop == nil {
		return ctx.GetText()
	}
	startPos := start.GetStart()
	stopPos := stop.GetStop()
	if startPos >= 0 && stopPos >= startPos && stopPos < len(e.originalQuery) {
		return e.originalQuery[startPos : stopPos+1]
	}
	return ctx.GetText()
}

// deriveExposedFields determines what fields the right side of a join makes available
func deriveExposedFields(subResult *ParseResult, joinFields []string) []string {
	if subResult == nil {
		return nil
	}

	fieldSet := make(map[string]bool)

	// Projected fields from project operators (most specific)
	for _, f := range subResult.ProjectedFields {
		fieldSet[f] = true
	}

	// Condition fields from the right side
	for _, c := range subResult.Conditions {
		if !kqlKeywords[strings.ToLower(c.Field)] {
			fieldSet[c.Field] = true
		}
	}

	// Computed fields from extend/project
	for computed := range subResult.ComputedFields {
		fieldSet[computed] = true
	}

	// Join fields exist on both sides
	for _, f := range joinFields {
		fieldSet[f] = true
	}

	result := make([]string, 0, len(fieldSet))
	for f := range fieldSet {
		result = append(result, f)
	}
	return result
}

// ClassifyFieldProvenance determines where a field originates relative to joins in the result
func ClassifyFieldProvenance(result *ParseResult, field string) FieldProvenance {
	if result == nil || len(result.Joins) == 0 {
		return ProvenanceAmbiguous
	}

	fieldLower := strings.ToLower(field)

	// Check join keys first (simple fields that exist on both sides)
	for _, j := range result.Joins {
		for _, jf := range j.JoinFields {
			if strings.ToLower(jf) == fieldLower {
				return ProvenanceJoinKey
			}
		}
		for _, lf := range j.LeftFields {
			if strings.ToLower(lf) == fieldLower {
				return ProvenanceJoinKey
			}
		}
		for _, rf := range j.RightFields {
			if strings.ToLower(rf) == fieldLower {
				return ProvenanceJoinKey
			}
		}
	}

	// Determine the first join stage
	firstJoinStage := -1
	for _, j := range result.Joins {
		if firstJoinStage == -1 || j.PipeStage < firstJoinStage {
			firstJoinStage = j.PipeStage
		}
	}

	// Check if field appears in main query conditions (before any join)
	// This takes priority over joined fields since the field was established in the main pipeline
	for _, c := range result.Conditions {
		if strings.ToLower(c.Field) == fieldLower && c.PipeStage < firstJoinStage {
			return ProvenanceMain
		}
	}

	if _, ok := result.ComputedFields[fieldLower]; ok {
		return ProvenanceMain
	}

	// Check if field is in exposed fields from any join's right side
	for _, j := range result.Joins {
		for _, ef := range j.ExposedFields {
			if strings.ToLower(ef) == fieldLower {
				return ProvenanceJoined
			}
		}
	}

	return ProvenanceAmbiguous
}

// EnterTabularOperator records the stage of the operator, which pipelines
// nested in it, such as union inputs, are fed into.
func (e *conditionExtractor) EnterTabularOperator(ctx *TabularOperatorContext) {
	e.operatorStages = append(e.operatorStages, e.currentStage)
}

// ExitTabularOperator increments the stage counter after processing each operator
func (e *conditionExtractor) ExitTabularOperator(ctx *TabularOperatorContext) {
	e.operatorStages = e.operatorStages[:len(e.operatorStages)-1]
	e.currentStage++
	if pipeline := e.currentPipeline(); pipeline >= 0 {
		e.pipelines[pipeline].Operators++
	}
}

// EnterTabularExpression starts a pipeline: the query, or one nested in the
// current pipeline.
func (e *conditionExtractor) EnterTabularExpression(ctx *TabularExpressionContext) {
	operator := "subquery"
	switch ctx.GetParent().(type) {
	case *QueryContext, *TabularSourceContext, *MaterializeExpressionContext:
		operator = ""
	case *UnionTableContext:
		operator = "union"
	case *JoinOperatorContext:
		operator = "join"
	case *LookupOperatorContext:
		operator = "lookup"
	}
	source := ""
	if ctx.TabularSource() != nil && ctx.TabularSource().TableName() != nil {
		source = ctx.TabularSource().TableName().GetText()
	}
	e.pushPipeline(operator, source)
	if _, ok := ctx.GetParent().(*QueryContext); ok {
		e.queryPipeline = e.currentPipeline()
	}
}

// ExitTabularExpression ends the current pipeline.
func (e *conditionExtractor) ExitTabularExpression(ctx *TabularExpressionContext) {
	e.pipelineStack = e.pipelineStack[:len(e.pipelineStack)-1]
}

// EnterUnionTable records a union input that names a table as a pipeline
// without operators; a parenthesized input starts its own pipeline.
func (e *conditionExtractor) EnterUnionTable(ctx *UnionTableContext) {
	if ctx.TableName() != nil {
		e.pushPipeline("union", ctx.TableName().GetText())
		e.pipelineStack = e.pipelineStack[:len(e.pipelineStack)-1]
	}
}

func (e *conditionExtractor) pushPipeline(operator, source string) {
	// A source feeds its pipeline before the first operator; other nested
	// pipelines feed the operator that contains them.
	stage := e.currentStage
	if n := len(e.operatorStages); operator != "" && n > 0 {
		stage = e.operatorStages[n-1]
	}
	e.pipelines = append(e.pipelines, Pipeline{Parent: e.currentPipeline(), Operator: operator, PipeStage: stage, Source: source})
	e.pipelineStack = append(e.pipelineStack, len(e.pipelines)-1)
}

// currentPipeline is the innermost pipeline at the current tree position, or
// -1 outside any.
func (e *conditionExtractor) currentPipeline() int {
	if len(e.pipelineStack) == 0 {
		return -1
	}
	return e.pipelineStack[len(e.pipelineStack)-1]
}

// EnterFunctionCall tracks when we enter a function call (countif, sumif, etc.)
// Conditions inside function calls are aggregation expressions, not filter conditions.
// Special handling for existence-check functions: isnotempty, isnotnull, isnull, isempty.
func (e *conditionExtractor) EnterFunctionCall(ctx *FunctionCallContext) {
	if ctx.Identifier() != nil {
		funcName := strings.ToLower(ctx.Identifier().GetText())
		switch funcName {
		case "isnotempty", "isnotnull":
			if ctx.ArgumentList() != nil {
				args := ctx.ArgumentList().AllArgument()
				if len(args) >= 1 {
					field := args[0].GetText()
					e.conditions = append(e.conditions, Condition{
						Field:    field,
						Operator: "isnotnull",
						Value:    "",
						Negated:  e.negated,
					})
				}
			}
			return // Don't increment inFunctionCall
		case "isnull", "isempty":
			if ctx.ArgumentList() != nil {
				args := ctx.ArgumentList().AllArgument()
				if len(args) >= 1 {
					field := args[0].GetText()
					e.conditions = append(e.conditions, Condition{
						Field:    field,
						Operator: "isnull",
						Value:    "",
						Negated:  e.negated,
					})
				}
			}
			return // Don't increment inFunctionCall
		}
	}
	e.inFunctionCall++
}

// ExitFunctionCall tracks when we exit a function call
func (e *conditionExtractor) ExitFunctionCall(ctx *FunctionCallContext) {
	e.inFunctionCall--
}

// EnterLetStatement tracks computed fields from let statements
func (e *conditionExtractor) EnterLetStatement(ctx *LetStatementContext) {
	if ctx.Identifier() != nil {
		field := ctx.Identifier().GetText()
		// Let statements can have complex expressions, so we don't track source field
		e.computedFields[strings.ToLower(field)] = ""
	}
}

// EnterAggregationItem tracks computed fields from summarize aggregation aliases.
// e.g., summarize FailedAttempts=count() registers "failedattempts" as computed.
// This prevents post-aggregation filters (| where FailedAttempts > 5) from being
// treated as required raw data fields in test validation.
func (e *conditionExtractor) EnterAggregationItem(ctx *AggregationItemContext) {
	// Two grammar forms:
	//   identifier ASSIGN aggregationFunction   (e.g., EventCount = count())
	//   aggregationFunction AS identifier        (e.g., count() as EventCount)
	alias := ""
	if ctx.Identifier() != nil && (ctx.ASSIGN() != nil || ctx.AS() != nil) {
		alias = ctx.Identifier().GetText()
		sourceField := ""
		if ctx.AggregationFunction() != nil {
			sourceField = aggregationFunctionText(ctx.AggregationFunction())
		}
		e.computedFields[strings.ToLower(alias)] = sourceField
	}

	if e.inSubquery > 0 {
		return
	}
	aggregation := newAggregation(ctx.AggregationFunction(), alias, e.currentStage)
	aggregation.Pipeline = e.currentPipeline()
	e.aggregations = append(e.aggregations, aggregation)
}

func aggregationFunctionText(ctx IAggregationFunctionContext) string {
	if ctx == nil || ctx.FunctionCall() == nil {
		return ""
	}
	return ctx.FunctionCall().GetText()
}

// newAggregation describes one summarize aggregation and the column it outputs.
func newAggregation(ctx IAggregationFunctionContext, alias string, stage int) Aggregation {
	aggregation := Aggregation{Column: alias, PipeStage: stage}
	if ctx == nil || ctx.FunctionCall() == nil || ctx.FunctionCall().Identifier() == nil {
		return aggregation
	}
	call := ctx.FunctionCall()
	aggregation.Function = strings.ToLower(call.Identifier().GetText())
	// normalizeQuery rewrites make_set and make_list to their legacy spellings.
	switch aggregation.Function {
	case "makeset":
		aggregation.Function = "make_set"
	case "makelist":
		aggregation.Function = "make_list"
	}
	if call.ArgumentList() != nil {
		if arguments := call.ArgumentList().AllArgument(); len(arguments) > 0 && arguments[0].ASSIGN() == nil && arguments[0].Expression() != nil {
			if text := arguments[0].Expression().GetText(); isValidFieldName(text) {
				aggregation.Field = text
			}
		}
	}
	if aggregation.Column == "" {
		aggregation.Column = defaultAggregationColumn(aggregation.Function, aggregation.Field)
	}
	return aggregation
}

// defaultAggregationColumn is the column KQL names an aggregation without an
// alias: count_ for count(), set_Field and list_Field for make_set and
// make_list, and function_Field otherwise (function_ when the first argument is
// not a plain column).
func defaultAggregationColumn(function, field string) string {
	switch function {
	case "count":
		return "count_"
	case "make_set":
		function = "set"
	case "make_list":
		function = "list"
	}
	return function + "_" + field
}

// EnterGroupByItem extracts field names from summarize BY clauses.
func (e *conditionExtractor) EnterGroupByItem(ctx *GroupByItemContext) {
	if e.inSubquery > 0 {
		return
	}

	// groupByItem: expression (AS identifier)? | identifier ASSIGN expression
	// Extract the field name — for simple identifiers, take the expression text;
	// for aliased items, take the alias (identifier) name.
	if ctx.Identifier() != nil && (ctx.ASSIGN() != nil || ctx.AS() != nil) {
		// Aliased: "Count = count()" or "count() AS Count" — the alias is the group-by result name
		alias := ctx.Identifier().GetText()
		e.groupByFields = append(e.groupByFields, alias)
		e.computedFields[strings.ToLower(alias)] = ""
	} else if ctx.Expression() != nil {
		text := ctx.Expression().GetText()
		if isValidFieldName(text) {
			e.groupByFields = append(e.groupByFields, text)
		} else {
			e.groupByExpressions = append(e.groupByExpressions, text)
		}
	}
}

// EnterExtendItem tracks computed fields from extend
func (e *conditionExtractor) EnterExtendItem(ctx *ExtendItemContext) {
	// Track field assignments in extend with source field extraction
	if ctx.Identifier() != nil {
		field := ctx.Identifier().GetText()
		sourceField := ""
		expression := ""

		// Try to extract the source field from the expression
		if ctx.Expression() != nil {
			expression = ctx.Expression().GetText()
			sourceField = extractFirstFieldFromExpression(ctx.Expression())
		}

		e.computedFields[strings.ToLower(field)] = sourceField
		e.computedExpressions[strings.ToLower(field)] = expression
	}
}

// extractFirstFieldFromExpression tries to extract the first raw event field name from an expression.
// It handles simple identifiers (AccountName), direct aliases (Account = AccountName), and
// nested function calls (tostring(split(SenderMailFromAddress, "@")[-1])).
func extractFirstFieldFromExpression(ctx IExpressionContext) string {
	if ctx == nil {
		return ""
	}
	return firstFieldInExpressionText(ctx.GetText())
}

// firstFieldInExpressionText returns the first raw field argument inside an expression,
// skipping function names, string literals, and numeric literals.
func firstFieldInExpressionText(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}

	// Simple identifier by itself (e.g., the right-hand side of "extend Account = AccountName").
	if isSimpleIdentifier(expr) && !kqlKeywords[strings.ToLower(expr)] {
		return expr
	}

	parenDepth := 0
	inString := byte(0)
	identStart := -1

	flushIdent := func(end int) string {
		if identStart < 0 {
			return ""
		}
		field := expr[identStart:end]
		identStart = -1
		if isSimpleIdentifier(field) && !kqlKeywords[strings.ToLower(field)] {
			return field
		}
		return ""
	}

	for i := 0; i < len(expr); {
		ch := expr[i]

		if inString != 0 {
			if ch == '\\' && i+1 < len(expr) {
				i += 2
				continue
			}
			if ch == inString {
				inString = 0
			}
			i++
			continue
		}

		switch ch {
		case '"', '\'', '`':
			inString = ch
			identStart = -1
		case '(':
			// Any identifier immediately before '(' is a function name, not a raw
			// event field, so discard it rather than returning it.
			parenDepth++
			identStart = -1
		case ')':
			if field := flushIdent(i); field != "" {
				return field
			}
			parenDepth--
			identStart = -1
		case ',':
			if field := flushIdent(i); field != "" {
				return field
			}
			identStart = -1
		default:
			if !isIdentChar(ch) {
				if field := flushIdent(i); field != "" {
					return field
				}
				identStart = -1
			} else if identStart < 0 && isIdentStart(ch) {
				identStart = i
			}
		}
		i++
	}

	if field := flushIdent(len(expr)); field != "" {
		return field
	}

	return ""
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

// EnterProjectItem tracks renamed fields from project
func (e *conditionExtractor) EnterProjectItem(ctx *ProjectItemContext) {
	if e.inSubquery > 0 {
		return
	}
	if ctx.Identifier() != nil && (ctx.ASSIGN() != nil || ctx.AS() != nil) {
		// Aliased project item: project NewName = expr or project expr AS NewName
		field := ctx.Identifier().GetText()
		sourceField, expression := "", ""
		if ctx.Expression() != nil {
			expression = ctx.Expression().GetText()
			sourceField = extractFirstFieldFromExpression(ctx.Expression())
		}
		e.computedFields[strings.ToLower(field)] = sourceField
		e.computedExpressions[strings.ToLower(field)] = expression
		e.projectedFields = append(e.projectedFields, field)
	} else if ctx.Expression() != nil {
		// Simple project item: project FieldName
		// The field name is the expression text (for simple identifiers)
		text := ctx.Expression().GetText()
		if isValidFieldName(text) {
			e.projectedFields = append(e.projectedFields, text)
		}
	}
}

// EnterRenameItem tracks project-rename columns as computed from the column
// they rename.
func (e *conditionExtractor) EnterRenameItem(ctx *RenameItemContext) {
	if e.inSubquery > 0 {
		return
	}
	identifiers := ctx.AllIdentifier()
	if len(identifiers) != 2 {
		return
	}
	field, source := identifiers[0].GetText(), identifiers[1].GetText()
	e.computedFields[strings.ToLower(field)] = source
	e.computedExpressions[strings.ToLower(field)] = source
}

// EnterComparisonExpression extracts field comparisons
func (e *conditionExtractor) EnterComparisonExpression(ctx *ComparisonExpressionContext) {
	// Skip conditions inside subqueries
	if e.inSubquery > 0 {
		return
	}

	// Skip conditions inside function calls (like countif(), sumif(), etc.)
	// These are aggregation expressions, not filter conditions
	if e.inFunctionCall > 0 {
		return
	}

	// Handle comparison operators: field == value, field != value, etc.
	if ctx.ComparisonOperator() != nil {
		addExprs := ctx.AllAdditiveExpression()
		if len(addExprs) >= 2 {
			e.handleComparison(addExprs[0].GetText(), ctx.ComparisonOperator().GetText(), addExprs[1].GetText())
		}
		return
	}

	// Handle string operators: field contains value, field has value, etc.
	if ctx.StringOperator() != nil {
		addExprs := ctx.AllAdditiveExpression()
		if len(addExprs) >= 2 {
			e.handleComparison(addExprs[0].GetText(), ctx.StringOperator().GetText(), addExprs[1].GetText())
		}
		return
	}

	// Handle IN operator: field in (values) - including case-sensitive variants (in~, !in~)
	// Note: IN_CS (in~) and NOT_IN_CS (!in~) tokens exist but generated parser doesn't have methods
	// So we check using GetToken directly for both regular and case-sensitive IN operators
	hasIN := ctx.IN() != nil
	hasNOT_IN := ctx.NOT_IN() != nil
	hasIN_CS := ctx.GetToken(KQLParserIN_CS, 0) != nil
	hasNOT_IN_CS := ctx.GetToken(KQLParserNOT_IN_CS, 0) != nil

	if hasIN || hasNOT_IN || hasIN_CS || hasNOT_IN_CS {
		addExprs := ctx.AllAdditiveExpression()
		if len(addExprs) >= 1 && ctx.ExpressionList() != nil {
			leftText := addExprs[0].GetText()
			isNegated := (hasNOT_IN || hasNOT_IN_CS) != e.negated
			operator := "in"
			if hasIN_CS || hasNOT_IN_CS || strings.Contains(strings.ToLower(ctx.GetText()), "in~") {
				operator = "in~"
			}
			e.handleInOperator(leftText, ctx.ExpressionList(), operator, isNegated)
		}
		return
	}

	// Handle BETWEEN operator: field between (low .. high)
	if ctx.BETWEEN() != nil || ctx.NOT_BETWEEN() != nil {
		addExprs := ctx.AllAdditiveExpression()
		if len(addExprs) >= 3 {
			leftText := addExprs[0].GetText()
			lowValue := addExprs[1].GetText()
			highValue := addExprs[2].GetText()
			isNegated := (ctx.NOT_BETWEEN() != nil) != e.negated
			e.handleBetweenOperator(leftText, lowValue, highValue, isNegated)
		}
		return
	}

	// Handle HAS_ANY and HAS_ALL operators
	if ctx.HAS_ANY() != nil || ctx.HAS_ALL() != nil {
		addExprs := ctx.AllAdditiveExpression()
		if len(addExprs) >= 1 && ctx.ExpressionList() != nil {
			leftText := addExprs[0].GetText()
			opName := "has_any"
			if ctx.HAS_ALL() != nil {
				opName = "has_all"
			}
			e.handleHasAnyAllOperator(leftText, opName, ctx.ExpressionList())
		}
		return
	}
}

// handleComparison processes a simple comparison (field op value)
func (e *conditionExtractor) handleComparison(left, op, right string) {
	// Check if left side looks like a field name
	if !isValidFieldName(left) {
		return
	}

	fieldLower := strings.ToLower(left)

	// Skip KQL keywords
	if kqlKeywords[fieldLower] {
		return
	}

	// Mark if this is a computed field (created by extend/project)
	sourceField, isComputed := e.computedFields[fieldLower]

	// Normalize the operator
	normalizedOp := normalizeOperator(op)

	// Extract value (remove quotes if present)
	value := extractValue(right)
	valueReference := ""
	if kqlValueIsFieldReference(right, value) {
		valueReference = value
		value = ""
	}

	cond := Condition{
		Field:          left,
		Operator:       normalizedOp,
		Value:          value,
		ValueReference: valueReference,
		Negated:        e.negated,
		PipeStage:      e.currentStage,
		Pipeline:       e.currentPipeline(),
		LogicalOp:      e.lastLogicalOp,
		IsComputed:     isComputed,
		SourceField:    sourceField,
	}
	e.conditions = append(e.conditions, cond)
	e.lastLogicalOp = "AND" // reset to default
}

// handleInOperator processes IN operator conditions
func (e *conditionExtractor) handleInOperator(field string, exprList IExpressionListContext, operator string, negated bool) {
	if !isValidFieldName(field) {
		return
	}

	fieldLower := strings.ToLower(field)
	if kqlKeywords[fieldLower] {
		return
	}

	// Mark if this is a computed field (created by extend/project)
	sourceField, isComputed := e.computedFields[fieldLower]

	values := extractExpressionListValues(exprList)
	if len(values) == 0 {
		return
	}
	cond := Condition{
		Field:        field,
		Operator:     operator,
		Value:        values[0],
		Negated:      negated,
		PipeStage:    e.currentStage,
		Pipeline:     e.currentPipeline(),
		LogicalOp:    e.lastLogicalOp,
		Alternatives: values,
		IsComputed:   isComputed,
		SourceField:  sourceField,
	}
	if len(values) == 1 && isSimpleIdentifier(values[0]) {
		cond.ValueReference = values[0]
	}
	e.conditions = append(e.conditions, cond)
	e.lastLogicalOp = "AND"
}

// handleBetweenOperator processes BETWEEN operator conditions
func (e *conditionExtractor) handleBetweenOperator(field, lowValue, highValue string, negated bool) {
	if !isValidFieldName(field) {
		return
	}

	fieldLower := strings.ToLower(field)
	if kqlKeywords[fieldLower] {
		return
	}

	// Mark if this is a computed field (created by extend/project)
	sourceField, isComputed := e.computedFields[fieldLower]

	// Add lower bound condition
	cond1 := Condition{
		Field:       field,
		Operator:    ">=",
		Value:       extractValue(lowValue),
		Negated:     negated,
		PipeStage:   e.currentStage,
		Pipeline:    e.currentPipeline(),
		LogicalOp:   e.lastLogicalOp,
		IsComputed:  isComputed,
		SourceField: sourceField,
	}
	e.conditions = append(e.conditions, cond1)

	// Add upper bound condition
	cond2 := Condition{
		Field:       field,
		Operator:    "<=",
		Value:       extractValue(highValue),
		Negated:     negated,
		PipeStage:   e.currentStage,
		Pipeline:    e.currentPipeline(),
		LogicalOp:   "AND",
		IsComputed:  isComputed,
		SourceField: sourceField,
	}
	e.conditions = append(e.conditions, cond2)
	e.lastLogicalOp = "AND"
}

// handleHasAnyAllOperator processes has_any and has_all operators
func (e *conditionExtractor) handleHasAnyAllOperator(field, op string, exprList IExpressionListContext) {
	if !isValidFieldName(field) {
		return
	}

	fieldLower := strings.ToLower(field)
	if kqlKeywords[fieldLower] {
		return
	}

	// Mark if this is a computed field (created by extend/project)
	sourceField, isComputed := e.computedFields[fieldLower]

	values := extractExpressionListValues(exprList)
	logicalConnector := "OR"
	if op == "has_all" {
		logicalConnector = "AND"
	}

	for i, value := range values {
		logOp := e.lastLogicalOp
		if i > 0 {
			logOp = logicalConnector
		}
		cond := Condition{
			Field:       field,
			Operator:    "has",
			Value:       value,
			Negated:     e.negated,
			PipeStage:   e.currentStage,
			Pipeline:    e.currentPipeline(),
			LogicalOp:   logOp,
			IsComputed:  isComputed,
			SourceField: sourceField,
		}
		e.conditions = append(e.conditions, cond)
	}
	e.lastLogicalOp = "AND"
}

// EnterNotExpression tracks negation
func (e *conditionExtractor) EnterNotExpression(ctx *NotExpressionContext) {
	if ctx.NOT() != nil {
		e.negated = !e.negated
	}
}

// ExitNotExpression resets negation
func (e *conditionExtractor) ExitNotExpression(ctx *NotExpressionContext) {
	if ctx.NOT() != nil {
		e.negated = !e.negated
	}
}

// EnterOrExpression handles OR in expressions
func (e *conditionExtractor) EnterOrExpression(ctx *OrExpressionContext) {
	// If there are multiple andExpressions, they're connected by OR
	if len(ctx.AllAndExpression()) > 1 {
		// The next condition after an OR will have OR logical op
	}
}

// ExitAndExpression links the next condition with OR when another OR branch
// follows this one. After the last branch the operator around the whole OR
// expression applies: in (a or b) and c, c is ANDed.
func (e *conditionExtractor) ExitAndExpression(ctx *AndExpressionContext) {
	orCtx, ok := ctx.GetParent().(*OrExpressionContext)
	if !ok {
		return
	}
	branches := orCtx.AllAndExpression()
	if len(branches) > 1 && branches[len(branches)-1] != ctx {
		e.lastLogicalOp = "OR"
	}
}

// isValidFieldName checks if a string could be a valid field name
func isValidFieldName(s string) bool {
	if len(s) == 0 {
		return false
	}

	// Allow purely numeric field names (e.g., Sysmon-style "1", "3", "22")
	// The grammar supports this via INT_NUMBER in the identifier rule.
	allDigits := true
	for _, c := range s {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return true
	}

	// Must start with letter or underscore
	first := s[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}

	// Check for nested field access (e.g., Properties.Result)
	for _, c := range s[1:] {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '.') {
			return false
		}
	}

	return true
}

// normalizeOperator converts KQL operators to standard form
func normalizeOperator(op string) string {
	switch op {
	case "==":
		return "=="
	case "!=":
		return "!="
	case "=~": // case-insensitive equal
		return "=~"
	case "!~": // case-insensitive not equal
		return "!~"
	case "<":
		return "<"
	case ">":
		return ">"
	case "<=":
		return "<="
	case ">=":
		return ">="
	default:
		return op
	}
}

// extractValue extracts the value, removing quotes if present
func extractValue(s string) string {
	s = strings.TrimSpace(s)

	// Remove double quotes
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}

	// Remove single quotes
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}

	// Remove verbatim string prefix (@"..." or @'...')
	if len(s) >= 3 && s[0] == '@' {
		if s[1] == '"' && s[len(s)-1] == '"' {
			return s[2 : len(s)-1]
		}
		if s[1] == '\'' && s[len(s)-1] == '\'' {
			return s[2 : len(s)-1]
		}
	}

	return s
}

func kqlValueIsFieldReference(raw, value string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, `"`) || strings.HasPrefix(raw, `'`) || strings.HasPrefix(raw, `@"`) || strings.HasPrefix(raw, `@'`) {
		return false
	}
	value = strings.TrimSpace(value)
	if !isSimpleIdentifier(value) {
		return false
	}
	switch strings.ToLower(value) {
	case "true", "false", "null":
		return false
	}
	return true
}

func extractPortableDataSources(originalQuery, normalizedQuery string) []string {
	letNames := extractLetNames(originalQuery)
	for name := range extractLetNames(normalizedQuery) {
		letNames[name] = true
	}

	var sources []string
	for _, query := range []string{originalQuery, normalizedQuery} {
		for _, source := range extractDataSourcesFromQuery(query, letNames) {
			sources = appendUnique(sources, source)
		}
	}
	return sources
}

func extractLetStatements(query string) []LetStatement {
	var statements []LetStatement
	seen := make(map[string]bool)
	for _, statement := range splitStatements(normalizeNewlines(query)) {
		cleaned := trimLeadingLineComments(statement)
		name, expression, ok := parseLetAssignment(strings.TrimSpace(cleaned))
		if !ok || expression == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		statements = append(statements, LetStatement{
			Name:       name,
			Expression: strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(expression), ";")),
		})
	}
	return statements
}

func trimLeadingLineComments(value string) string {
	lines := strings.Split(normalizeNewlines(value), "\n")
	for len(lines) > 0 {
		line := strings.TrimSpace(lines[0])
		if line == "" || strings.HasPrefix(line, "//") {
			lines = lines[1:]
			continue
		}
		break
	}
	return strings.Join(lines, "\n")
}

func hasPortableFieldConditions(conditions []Condition) bool {
	for _, condition := range conditions {
		if condition.IsComputed {
			continue
		}
		if condition.Field == "" {
			continue
		}
		if kqlKeywords[strings.ToLower(condition.Field)] {
			continue
		}
		return true
	}
	return false
}

func extractLetNames(query string) map[string]bool {
	names := make(map[string]bool)
	for _, statement := range splitStatements(normalizeNewlines(query)) {
		name, _, ok := parseLetAssignment(strings.TrimSpace(statement))
		if ok {
			names[strings.ToLower(name)] = true
		}
	}
	for _, line := range strings.Split(normalizeNewlines(query), "\n") {
		name, _, ok := parseLetAssignment(strings.TrimSpace(line))
		if ok {
			names[strings.ToLower(name)] = true
		}
	}
	return names
}

func extractDataSourcesFromQuery(query string, letNames map[string]bool) []string {
	query = normalizeNewlines(query)
	var sources []string
	for _, line := range strings.Split(query, "\n") {
		_, rhs, ok := parseLetAssignment(strings.TrimSpace(line))
		if !ok {
			continue
		}
		for _, source := range extractDataSourcesFromStatement(rhs, letNames, true) {
			sources = appendUnique(sources, source)
		}
	}
	segments := splitTopLevelPipeSegments(query)
	for i, segment := range segments {
		// The first segment names the data source; later segments are operators,
		// except the trailing one which never introduces a source. Requiring more
		// than one segment here used to discard bare table queries ("SecurityEvent"),
		// and — because a parenthesized join operand recurses through this function
		// as its own single-segment query — it also discarded the right-hand table
		// of every parenthesized join.
		allowLeadingSource := i == 0 || i < len(segments)-1
		for _, statement := range splitStatements(segment) {
			for _, source := range extractDataSourcesFromStatement(statement, letNames, allowLeadingSource) {
				sources = appendUnique(sources, source)
			}
		}
	}
	return sources
}

func splitStatements(query string) []string {
	var statements []string
	start := 0
	depth := 0
	var quote byte
	verbatim := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(query) && query[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(query) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}
		if ch == '@' && i+1 < len(query) && (query[i+1] == '"' || query[i+1] == '\'') {
			quote = query[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				statements = append(statements, query[start:i])
				start = i + 1
			}
		}
	}
	statements = append(statements, query[start:])
	return statements
}

func extractDataSourcesFromStatement(statement string, letNames map[string]bool, allowLeadingSource bool) []string {
	statement = strings.TrimSpace(trimLineComment(statement))
	statement = strings.TrimLeft(statement, " \t\r\n(),")
	statement = stripOuterParens(statement)
	if statement == "" {
		return nil
	}

	if _, rhs, ok := parseLetAssignment(statement); ok {
		return extractDataSourcesFromStatement(rhs, letNames, true)
	}

	lower := strings.ToLower(statement)
	switch {
	case strings.HasPrefix(lower, "search in"):
		return extractParenthesizedSources(statement, "search in", letNames)
	case strings.HasPrefix(lower, "find in"):
		return extractParenthesizedSources(statement, "find in", letNames)
	case strings.HasPrefix(lower, "union "):
		return extractUnionSources(statement, letNames)
	case strings.HasPrefix(lower, "join "):
		return extractJoinSources(statement, letNames)
	}

	if !allowLeadingSource {
		return nil
	}
	if source := leadingDataSource(statement, letNames); source != "" {
		return []string{source}
	}
	return nil
}

func extractParenthesizedSources(statement, keyword string, letNames map[string]bool) []string {
	rest := strings.TrimSpace(statement[len(keyword):])
	open := strings.Index(rest, "(")
	if open < 0 {
		return nil
	}
	open += len(statement) - len(rest)
	close := findMatchingParen(statement, open)
	if close < 0 {
		return nil
	}
	var sources []string
	for _, part := range splitCommaList(statement[open+1 : close]) {
		if source := cleanDataSourceToken(part, letNames); source != "" {
			sources = appendUnique(sources, source)
		}
	}
	return sources
}

func extractUnionSources(statement string, letNames map[string]bool) []string {
	rest := strings.TrimSpace(statement[len("union "):])
	var sources []string
	for _, part := range splitCommaList(rest) {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		for len(fields) > 0 && strings.Contains(fields[0], "=") {
			fields = fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		if source := cleanDataSourceToken(fields[0], letNames); source != "" {
			sources = appendUnique(sources, source)
		}
	}
	return sources
}

func extractJoinSources(statement string, letNames map[string]bool) []string {
	rest := strings.TrimSpace(statement[len("join "):])
	for {
		fields := strings.Fields(rest)
		if len(fields) == 0 || !strings.Contains(fields[0], "=") {
			break
		}
		rest = strings.TrimSpace(strings.TrimPrefix(rest, fields[0]))
	}
	if strings.HasPrefix(rest, "(") {
		close := findMatchingParen(rest, 0)
		if close > 0 {
			return extractDataSourcesFromQuery(rest[1:close], letNames)
		}
	}
	if source := leadingDataSource(rest, letNames); source != "" {
		return []string{source}
	}
	return nil
}

func leadingDataSource(statement string, letNames map[string]bool) string {
	statement = strings.TrimSpace(statement)
	lower := strings.ToLower(statement)
	if fields := strings.Fields(lower); len(fields) > 0 {
		for _, command := range []string{
			"where", "project", "project-away", "extend", "summarize", "order", "sort",
			"take", "top", "count", "render", "evaluate", "mv-expand", "mv-apply",
			"parse", "distinct", "lookup", "print", "range", "datatable",
			"externaldata", "dynamic", "pack_array", "pack", "bag_pack",
		} {
			if fields[0] == command || strings.HasPrefix(fields[0], command+"(") {
				return ""
			}
		}
	}
	end := len(statement)
	for i, ch := range statement {
		if ch == '|' || ch == ',' || ch == '(' || ch == ')' || ch == '[' || ch == ']' || ch == ';' || ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			end = i
			break
		}
	}
	return cleanDataSourceToken(statement[:end], letNames)
}

func cleanDataSourceToken(token string, letNames map[string]bool) string {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, " \t\r\n,;()[]")
	if idx := strings.LastIndex(token, "."); idx >= 0 && idx+1 < len(token) {
		token = token[idx+1:]
	}
	if !isPortableDataSourceCandidate(token, letNames) {
		return ""
	}
	return token
}

func isPortableDataSourceCandidate(token string, letNames map[string]bool) bool {
	if token == "" || !isSimpleIdentifier(token) {
		return false
	}
	lower := strings.ToLower(token)
	if letNames[lower] {
		return false
	}
	switch lower {
	case "dummytable", "lookuptable", "alltables", "true", "false", "t":
		return false
	}
	if _, err := strconv.ParseFloat(token, 64); err == nil {
		return false
	}
	return true
}

func findMatchingParen(s string, open int) int {
	if open < 0 || open >= len(s) || s[open] != '(' {
		return -1
	}
	depth := 0
	var quote byte
	verbatim := false
	for i := open; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(s) && s[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(s) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}
		if ch == '@' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\'') {
			quote = s[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '(' {
			depth++
			continue
		}
		if ch == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// extractPortableConditions extracts translatable conditions from KQL shapes
// that parse successfully but do not produce portable predicates in the tree walker.
func extractPortableConditions(originalQuery, normalizedQuery string) ([]Condition, string) {
	if conditions := extractPortablePredicates(originalQuery); len(conditions) > 0 {
		return conditions, portablePredicateExtractionNote
	}
	if conditions := extractPortablePredicates(normalizedQuery); len(conditions) > 0 {
		return conditions, portablePredicateExtractionNote
	}
	if containsExternalDataLet(originalQuery) {
		return nil, ""
	}
	keyword := extractPortableKeyword(originalQuery)
	if keyword == "" {
		keyword = extractPortableKeyword(normalizedQuery)
	}
	if keyword == "" {
		return nil, ""
	}
	return []Condition{portableKeywordCondition(keyword)}, portableKeywordExtractionNote
}

func portableKeywordCondition(keyword string) Condition {
	return Condition{
		Field:     "_keyword_",
		Operator:  "contains",
		Value:     keyword,
		PipeStage: 1,
		LogicalOp: "AND",
	}
}

func extractPortablePredicates(query string) []Condition {
	lists := extractLetLists(query)
	var conditions []Condition
	for _, expr := range extractWhereExpressions(query) {
		for _, part := range splitBooleanConditions(expr) {
			cond, ok := parsePortablePredicate(part, lists)
			if !ok {
				continue
			}
			if !containsCondition(conditions, cond) {
				conditions = append(conditions, cond)
			}
		}
	}
	return conditions
}

func extractLetLists(query string) map[string][]string {
	lists := make(map[string][]string)
	for _, line := range strings.Split(normalizeNewlines(query), "\n") {
		name, rhs, ok := parseLetAssignment(strings.TrimSpace(line))
		if !ok {
			continue
		}
		if values, ok := parseDynamicList(rhs); ok {
			lists[strings.ToLower(name)] = values
		}
	}
	return lists
}

func extractWhereExpressions(query string) []string {
	var expressions []string
	for _, segment := range splitPipeSegments(normalizeNewlines(query)) {
		segment = strings.TrimSpace(segment)
		if len(segment) < len("where ") || !strings.HasPrefix(strings.ToLower(segment), "where ") {
			continue
		}
		expr := strings.TrimSpace(segment[len("where "):])
		expr = strings.TrimSuffix(expr, ";")
		expr = strings.TrimSpace(expr)
		if expr != "" {
			expressions = append(expressions, expr)
		}
	}
	return expressions
}

// splitPipeSegments splits on every pipe regardless of nesting. Predicate
// extraction depends on this: a parenthesised let-subquery such as
// "let H = (T | where MD5 in (IOC));" must still yield its inner "where"
// segments. Use splitTopLevelPipeSegments when nesting must be respected.
func splitPipeSegments(query string) []string {
	var segments []string
	start := 0
	var quote byte
	verbatim := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(query) && query[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(query) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}
		if ch == '@' && i+1 < len(query) && (query[i+1] == '"' || query[i+1] == '\'') {
			quote = query[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '|' {
			segments = append(segments, query[start:i])
			start = i + 1
		}
	}
	segments = append(segments, query[start:])
	return segments
}

// splitTopLevelPipeSegments splits on pipes at paren depth zero only, so a
// parenthesised subquery stays intact. Data-source extraction needs this:
// "T | join (U | where x) on k" must keep "(U | where x)" together, otherwise
// the join operand is torn in half and U is never recognised as a source.
func splitTopLevelPipeSegments(query string) []string {
	var segments []string
	start := 0
	depth := 0
	var quote byte
	verbatim := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(query) && query[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(query) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}
		if ch == '@' && i+1 < len(query) && (query[i+1] == '"' || query[i+1] == '\'') {
			quote = query[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case '|':
			if depth == 0 {
				segments = append(segments, query[start:i])
				start = i + 1
			}
		}
	}
	segments = append(segments, query[start:])
	return segments
}

func splitBooleanConditions(expr string) []string {
	expr = stripOuterParens(strings.TrimSpace(expr))
	var parts []string
	start := 0
	depth := 0
	var quote byte
	verbatim := false
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(expr) && expr[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(expr) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}
		if ch == '@' && i+1 < len(expr) && (expr[i+1] == '"' || expr[i+1] == '\'') {
			quote = expr[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				if word := booleanAt(expr, i); word != "" {
					part := strings.TrimSpace(expr[start:i])
					if part != "" {
						parts = append(parts, splitBooleanConditions(part)...)
					}
					i += len(word) - 1
					start = i + 1
				}
			}
		}
	}
	tail := strings.TrimSpace(expr[start:])
	if tail != "" {
		tail = stripOuterParens(tail)
		tailLower := strings.ToLower(tail)
		if tail != expr && (strings.Contains(tailLower, " or ") || strings.Contains(tailLower, " and ")) {
			parts = append(parts, splitBooleanConditions(tail)...)
		} else {
			parts = append(parts, tail)
		}
	}
	return parts
}

func booleanAt(s string, idx int) string {
	for _, word := range []string{"and", "or"} {
		if idx+len(word) > len(s) || !strings.EqualFold(s[idx:idx+len(word)], word) {
			continue
		}
		if wordBoundary(s, idx-1) && wordBoundary(s, idx+len(word)) {
			return word
		}
	}
	return ""
}

func parsePortablePredicate(expr string, lists map[string][]string) (Condition, bool) {
	expr = strings.TrimSpace(stripOuterParens(expr))
	if expr == "" || strings.Contains(expr, "|") {
		return Condition{}, false
	}
	if condition, ok := parsePortableFunctionPredicate(expr, lists); ok {
		return condition, true
	}
	for _, op := range portablePredicateOperators() {
		idx := indexOperatorOutside(expr, op)
		if idx < 0 {
			continue
		}
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+len(op):])
		if !isSimpleIdentifier(left) || strings.EqualFold(left, "Timestamp") && strings.Contains(strings.ToLower(right), "ago(") {
			return Condition{}, false
		}
		valueReference := portableValueReference(right, lists)
		value, alternatives, ok := parsePortableValue(right, lists)
		if !ok {
			return Condition{}, false
		}
		return Condition{
			Field:          left,
			Operator:       op,
			Value:          value,
			ValueReference: valueReference,
			Alternatives:   alternatives,
			PipeStage:      1,
			LogicalOp:      "AND",
		}, true
	}
	return Condition{}, false
}

func parsePortableFunctionPredicate(expr string, lists map[string][]string) (Condition, bool) {
	name, args, ok := parseFunctionCall(expr)
	if !ok {
		return Condition{}, false
	}
	switch strings.ToLower(name) {
	case "set_has_element":
		if len(args) != 2 {
			return Condition{}, false
		}
		field := strings.TrimSpace(args[0])
		if !isSimpleIdentifier(field) {
			return Condition{}, false
		}
		valueReference := portableValueReference(args[1], lists)
		value, alternatives, ok := parsePortableValue(args[1], lists)
		if !ok {
			return Condition{}, false
		}
		return Condition{
			Field:          field,
			Operator:       "has",
			Value:          value,
			ValueReference: valueReference,
			Alternatives:   alternatives,
			PipeStage:      1,
			LogicalOp:      "AND",
		}, true
	}
	return Condition{}, false
}

func parseFunctionCall(expr string) (string, []string, bool) {
	open := strings.Index(expr, "(")
	if open <= 0 || !strings.HasSuffix(strings.TrimSpace(expr), ")") {
		return "", nil, false
	}
	name := strings.TrimSpace(expr[:open])
	if !isSimpleIdentifier(name) {
		return "", nil, false
	}
	close := findMatchingParen(expr, open)
	if close < 0 || strings.TrimSpace(expr[close+1:]) != "" {
		return "", nil, false
	}
	return name, splitCommaList(expr[open+1 : close]), true
}

func portablePredicateOperators() []string {
	return []string{
		"matches regex",
		"!contains",
		"!startswith",
		"!endswith",
		"!hasprefix",
		"!hassuffix",
		"!in~",
		"!in",
		"has_any",
		"has_all",
		"startswith",
		"endswith",
		"hasprefix",
		"hassuffix",
		"contains_cs",
		"contains",
		"has_cs",
		"has",
		"in~",
		"in",
		"=~",
		"!~",
		"==",
		"!=",
		">=",
		"<=",
		">",
		"<",
	}
}

func parsePortableValue(raw string, lists map[string][]string) (string, []string, bool) {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
	if raw == "" {
		return "", nil, false
	}
	if values, ok := parseListValue(raw, lists); ok {
		return values[0], values, true
	}
	if value, rest, ok := parseStringLiteral(raw); ok && strings.TrimSpace(rest) == "" {
		return value, nil, true
	}
	if values, ok := lists[strings.ToLower(raw)]; ok && len(values) > 0 {
		return values[0], values, true
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return raw, nil, true
	}
	lower := strings.ToLower(raw)
	if lower == "true" || lower == "false" {
		return lower, nil, true
	}
	return "", nil, false
}

func portableValueReference(raw string, lists map[string][]string) string {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
	if raw == "" {
		return ""
	}
	if isSimpleIdentifier(raw) {
		if _, ok := lists[strings.ToLower(raw)]; ok {
			return raw
		}
	}
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
		body := strings.TrimSpace(raw[1 : len(raw)-1])
		if isSimpleIdentifier(body) {
			if _, ok := lists[strings.ToLower(body)]; ok {
				return body
			}
		}
	}
	return ""
}

func parseDynamicList(raw string) ([]string, bool) {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
	return parseListValue(raw, nil)
}

func parseListValue(raw string, lists map[string][]string) ([]string, bool) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "dynamic(") && strings.HasSuffix(raw, ")") {
		raw = strings.TrimSpace(raw[len("dynamic(") : len(raw)-1])
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		raw = "(" + strings.TrimSpace(raw[1:len(raw)-1]) + ")"
	}
	if !strings.HasPrefix(raw, "(") || !strings.HasSuffix(raw, ")") {
		return nil, false
	}
	body := strings.TrimSpace(raw[1 : len(raw)-1])
	if body == "" {
		return nil, false
	}
	var values []string
	for _, item := range splitCommaList(body) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if nested, ok := lists[strings.ToLower(item)]; ok {
			for _, value := range nested {
				values = appendUnique(values, value)
			}
			continue
		}
		if value, rest, ok := parseStringLiteral(item); ok && strings.TrimSpace(rest) == "" {
			values = appendUnique(values, value)
			continue
		}
		if _, err := strconv.ParseFloat(item, 64); err == nil {
			values = appendUnique(values, item)
		}
	}
	if len(values) == 0 {
		return nil, false
	}
	return values, true
}

func splitCommaList(body string) []string {
	var parts []string
	start := 0
	depth := 0
	var quote byte
	verbatim := false
	for i := 0; i < len(body); i++ {
		ch := body[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(body) && body[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(body) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}
		if ch == '@' && i+1 < len(body) && (body[i+1] == '"' || body[i+1] == '\'') {
			quote = body[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, body[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, body[start:])
	return parts
}

func indexOperatorOutside(s, op string) int {
	for i := 0; i+len(op) <= len(s); i++ {
		idx := indexOutsideQuotesAndParens(s[i:], op)
		if idx < 0 {
			return -1
		}
		idx += i
		if !operatorHasWord(op) || wordBoundary(s, idx-1) && wordBoundary(s, idx+len(op)) {
			return idx
		}
		i = idx
	}
	return -1
}

func operatorHasWord(op string) bool {
	for i := 0; i < len(op); i++ {
		ch := op[i]
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' {
			return true
		}
	}
	return false
}

func wordBoundary(s string, idx int) bool {
	return idx < 0 || idx >= len(s) || !isIdentChar(s[idx])
}

func stripOuterParens(s string) string {
	for {
		s = strings.TrimSpace(s)
		if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
			return s
		}
		depth := 0
		balanced := true
		var quote byte
		for i := 0; i < len(s); i++ {
			ch := s[i]
			if quote != 0 {
				if ch == '\\' && i+1 < len(s) {
					i++
					continue
				}
				if ch == quote {
					quote = 0
				}
				continue
			}
			if ch == '"' || ch == '\'' {
				quote = ch
				continue
			}
			if ch == '(' {
				depth++
			}
			if ch == ')' {
				depth--
				if depth == 0 && i != len(s)-1 {
					balanced = false
					break
				}
				if depth < 0 {
					balanced = false
					break
				}
			}
		}
		if !balanced || depth != 0 {
			return s
		}
		s = s[1 : len(s)-1]
	}
}

func containsCondition(conditions []Condition, condition Condition) bool {
	key := conditionKey(condition)
	for _, existing := range conditions {
		if conditionKey(existing) == key {
			return true
		}
	}
	return false
}

func conditionKey(condition Condition) string {
	return fmt.Sprintf("%t|%s|%s|%s|%s|%s", condition.IsComputed, strings.ToLower(condition.Field), condition.Operator, condition.Value, condition.ValueReference, strings.Join(condition.Alternatives, "\x00"))
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func extractPortableKeyword(query string) string {
	query = strings.TrimSpace(query)
	if isTrivialPortableQuery(query) {
		return ""
	}
	if term := extractLeadingSearchKeyword(query); term != "" {
		return term
	}
	if term := extractSearchKeyword(query); term != "" {
		return term
	}
	if containsExternalDataLet(query) {
		return ""
	}
	if ident := extractFinalIdentifier(query); ident != "" {
		return ident
	}
	if literal := firstStringLiteral(query); literal != "" {
		if looksLikeURL(literal) {
			return ""
		}
		return literal
	}
	return compactKeyword(query)
}

func containsExternalDataLet(query string) bool {
	for _, statement := range splitStatements(normalizeNewlines(query)) {
		_, rhs, ok := parseLetAssignment(strings.TrimSpace(statement))
		if ok && strings.HasPrefix(strings.ToLower(strings.TrimSpace(rhs)), "externaldata") {
			return true
		}
	}
	return false
}

func isTrivialPortableQuery(query string) bool {
	switch strings.ToLower(strings.TrimSpace(query)) {
	case "", "n/a", "na", "none", "tbd", "todo", "-":
		return true
	}
	return false
}

func extractSearchKeyword(query string) string {
	lower := strings.ToLower(query)
	idx := strings.Index(lower, "search in")
	if idx < 0 {
		return ""
	}
	rest := query[idx+len("search in"):]
	if close := strings.Index(rest, ")"); close >= 0 {
		rest = rest[close+1:]
	}
	if value, _, ok := parseStringLiteral(strings.TrimSpace(rest)); ok {
		return value
	}
	return ""
}

func extractLeadingSearchKeyword(query string) string {
	trimmed := strings.TrimSpace(query)
	if len(trimmed) < len("search") {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "search") {
		return ""
	}
	if len(trimmed) > len("search") && isIdentChar(trimmed[len("search")]) {
		return ""
	}

	rest := strings.TrimSpace(trimmed[len("search"):])
	if rest == "" {
		return ""
	}
	lowerRest := strings.ToLower(rest)
	if strings.HasPrefix(lowerRest, "kind=") {
		end := strings.IndexAny(rest, " \t\r\n")
		if end < 0 {
			return ""
		}
		rest = strings.TrimSpace(rest[end:])
		lowerRest = strings.ToLower(rest)
	}

	if strings.HasPrefix(lowerRest, "in ") || strings.HasPrefix(lowerRest, "in(") {
		open := strings.Index(rest, "(")
		if open < 0 {
			return ""
		}
		close := findMatchingParen(rest, open)
		if close <= open {
			return ""
		}
		rest = strings.TrimSpace(rest[close+1:])
	}
	if rest == "" || strings.HasPrefix(rest, "|") {
		return ""
	}
	value, _, ok := parseStringLiteral(rest)
	if !ok || looksLikeURL(value) {
		return ""
	}
	return value
}

func parseLetAssignment(line string) (string, string, bool) {
	line = trimLineComment(line)
	if !strings.HasPrefix(strings.ToLower(line), "let ") {
		return "", "", false
	}
	rest := strings.TrimSpace(line[len("let "):])
	eq := indexOutsideQuotesAndParens(rest, "=")
	if eq <= 0 {
		return "", "", false
	}
	name := strings.TrimSpace(rest[:eq])
	if !isSimpleIdentifier(name) {
		return "", "", false
	}
	return name, strings.TrimSpace(rest[eq+1:]), true
}

func extractFinalIdentifier(query string) string {
	lines := strings.Split(normalizeNewlines(query), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := trimLineComment(strings.TrimSpace(lines[i]))
		line = strings.TrimSuffix(line, ";")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "|") {
			continue
		}
		if isSimpleIdentifier(line) {
			return line
		}
		break
	}
	return ""
}

func firstStringLiteral(query string) string {
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch != '"' && ch != '\'' && !(ch == '@' && i+1 < len(query) && (query[i+1] == '"' || query[i+1] == '\'')) {
			continue
		}
		if value, _, ok := parseStringLiteral(query[i:]); ok {
			return value
		}
	}
	return ""
}

func parseStringLiteral(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	verbatim := false
	if raw[0] == '@' && len(raw) > 1 && (raw[1] == '"' || raw[1] == '\'') {
		verbatim = true
		raw = raw[1:]
	}
	if raw[0] != '"' && raw[0] != '\'' {
		return "", "", false
	}
	quote := raw[0]
	var b strings.Builder
	for i := 1; i < len(raw); i++ {
		ch := raw[i]
		if verbatim && ch == quote && i+1 < len(raw) && raw[i+1] == quote {
			b.WriteByte(quote)
			i++
			continue
		}
		if !verbatim && ch == '\\' && i+1 < len(raw) {
			i++
			b.WriteByte(raw[i])
			continue
		}
		if ch == quote {
			return b.String(), raw[i+1:], true
		}
		b.WriteByte(ch)
	}
	return "", "", false
}

func indexOutsideQuotesAndParens(s, token string) int {
	depth := 0
	var quote byte
	verbatim := false
	for i := 0; i+len(token) <= len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if verbatim && ch == quote && i+1 < len(s) && s[i+1] == quote {
				i++
				continue
			}
			if ch == '\\' && !verbatim && i+1 < len(s) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				verbatim = false
			}
			continue
		}
		if ch == '@' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\'') {
			quote = s[i+1]
			verbatim = true
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			depth++
			continue
		case ')':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 && s[i:i+len(token)] == token {
			return i
		}
	}
	return -1
}

func trimLineComment(line string) string {
	var quote byte
	for i := 0; i+1 < len(line); i++ {
		ch := line[i]
		if quote != 0 {
			if ch == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '/' && line[i+1] == '/' {
			return strings.TrimSpace(line[:i])
		}
	}
	return line
}

func normalizeNewlines(query string) string {
	return strings.ReplaceAll(query, "\r\n", "\n")
}

func isSimpleIdentifier(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if i == 0 && (ch >= '0' && ch <= '9' || ch == '.') {
			return false
		}
		if !isIdentChar(ch) && ch != '.' {
			return false
		}
	}
	return true
}

func looksLikeURL(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func compactKeyword(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return ""
	}
	compact := strings.Join(fields, " ")
	if len(compact) > 160 {
		compact = strings.TrimSpace(compact[:160])
	}
	return compact
}

func extractExpressionListValues(ctx IExpressionListContext) []string {
	if ctx == nil {
		return nil
	}

	var values []string
	for _, expr := range ctx.AllExpression() {
		values = append(values, extractValue(expr.GetText()))
	}
	return values
}

// groupORConditionsByExpression merges same-field conditions that are
// adjacent children of one OR node in expression into a single condition
// with Alternatives, returning the merged list and a raw-to-merged index
// map. It returns false when there is no expression to group by.
func groupORConditionsByExpression(conditions []Condition, expression *ConditionExpression) ([]Condition, []int, bool) {
	if expression == nil {
		return nil, nil, false
	}
	groupStart := make([]int, len(conditions))
	for i := range groupStart {
		groupStart[i] = i
	}
	var visit func(*ConditionExpression)
	visit = func(node *ConditionExpression) {
		if node == nil {
			return
		}
		if node.Operator == BooleanOr {
			for i := 1; i < len(node.Children); i++ {
				prev, child := node.Children[i-1], node.Children[i]
				if prev.Operator != BooleanCondition || child.Operator != BooleanCondition {
					continue
				}
				first, index := groupStart[prev.ConditionIndex], child.ConditionIndex
				if index == prev.ConditionIndex+1 && index < len(conditions) && sameConditionGroup(conditions[first], conditions[index]) {
					groupStart[index] = first
				}
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(expression)

	result := make([]Condition, 0, len(conditions))
	indexMap := make([]int, len(conditions))
	for i, condition := range conditions {
		if start := groupStart[i]; start != i {
			merged := &result[indexMap[start]]
			merged.Alternatives = deduplicateConditionValues(append(conditionAlternatives(*merged), conditionAlternatives(condition)...))
			indexMap[i] = indexMap[start]
			continue
		}
		indexMap[i] = len(result)
		result = append(result, condition)
	}
	return result, indexMap, true
}

// groupORConditions groups consecutive OR conditions on the same field
func groupORConditions(conditions []Condition) []Condition {
	grouped, _ := groupORConditionsWithIndexMap(conditions)
	return grouped
}

func groupORConditionsWithIndexMap(conditions []Condition) ([]Condition, []int) {
	if len(conditions) == 0 {
		return conditions, nil
	}

	result := make([]Condition, 0, len(conditions))
	indexMap := make([]int, len(conditions))
	for i := range indexMap {
		indexMap[i] = -1
	}

	for i := 0; i < len(conditions); i++ {
		cond := conditions[i]

		// Look ahead for OR conditions on the same field
		if i+1 < len(conditions) && conditions[i+1].LogicalOp == "OR" && sameConditionGroup(cond, conditions[i+1]) && canMergeAlternativeGroup(conditions, i) {
			alternatives := conditionAlternatives(cond)

			j := i + 1
			for j < len(conditions) {
				next := conditions[j]
				if next.LogicalOp == "OR" && sameConditionGroup(cond, next) {
					alternatives = append(alternatives, conditionAlternatives(next)...)
					j++
				} else {
					break
				}
			}

			if len(alternatives) > 1 {
				cond.Alternatives = deduplicateConditionValues(alternatives)
				groupedIndex := len(result)
				result = append(result, cond)
				for rawIndex := i; rawIndex < j; rawIndex++ {
					indexMap[rawIndex] = groupedIndex
				}
				if j < len(conditions) && strings.EqualFold(conditions[j].LogicalOp, "OR") {
					conditions[j].LogicalOp = "AND"
				}
				i = j - 1 // skip the grouped conditions
				continue
			}
		}

		indexMap[i] = len(result)
		result = append(result, cond)
	}

	return result, indexMap
}

func canMergeAlternativeGroup(conditions []Condition, index int) bool {
	if index == 0 || strings.EqualFold(conditions[index].LogicalOp, "OR") {
		return true
	}
	j := index + 1
	for j < len(conditions) && strings.EqualFold(conditions[j].LogicalOp, "OR") && sameConditionGroup(conditions[index], conditions[j]) {
		j++
	}
	return j == len(conditions) || strings.EqualFold(conditions[j].LogicalOp, "OR")
}

func sameConditionGroup(a, b Condition) bool {
	return strings.EqualFold(a.Field, b.Field) &&
		a.Operator == b.Operator &&
		!a.Negated &&
		!b.Negated &&
		a.IsComputed == b.IsComputed &&
		strings.EqualFold(a.SourceField, b.SourceField)
}

func conditionAlternatives(cond Condition) []string {
	if len(cond.Alternatives) > 0 {
		return append([]string(nil), cond.Alternatives...)
	}
	return []string{cond.Value}
}

func deduplicateConditionValues(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func conditionDedupKey(cond Condition) string {
	return strings.ToLower(cond.Field) + "|" +
		cond.Operator + "|" +
		cond.Value + "|" +
		boolKey(cond.Negated) + "|" +
		boolKey(cond.IsComputed) + "|" +
		strings.ToLower(cond.SourceField) + "|" +
		strings.Join(cond.Alternatives, "\x00")
}

func boolKey(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

// DeduplicateConditions removes duplicate conditions, keeping the latest pipe stage
func DeduplicateConditions(conditions []Condition) []Condition {
	if len(conditions) == 0 {
		return conditions
	}

	// Group by field (case-insensitive)
	fieldConditions := make(map[string][]Condition)
	for _, cond := range conditions {
		// Skip pure wildcards
		if cond.Value == "*" {
			continue
		}
		fieldLower := strings.ToLower(cond.Field)
		fieldConditions[fieldLower] = append(fieldConditions[fieldLower], cond)
	}

	// Keep only conditions from the latest pipe stage for each field
	result := make([]Condition, 0)
	seen := make(map[string]bool)

	for _, conds := range fieldConditions {
		// Find max pipe stage
		maxStage := -1
		for _, c := range conds {
			if c.PipeStage > maxStage {
				maxStage = c.PipeStage
			}
		}

		// Keep only conditions from max stage
		for _, cond := range conds {
			if cond.PipeStage == maxStage {
				key := conditionDedupKey(cond)
				if !seen[key] {
					seen[key] = true
					result = append(result, cond)
				}
			}
		}
	}

	return result
}

// IsStatisticalQuery checks if the parse result contains aggregation commands
// (summarize) that create computed fields making static analysis unreliable
func IsStatisticalQuery(result *ParseResult) bool {
	if result == nil {
		return false
	}
	for _, cmd := range result.Commands {
		if cmd == "summarize" {
			return true
		}
	}
	return false
}

// HasUnmappedComputedFields checks if any computed field used in conditions
// could not be traced back to a source field
func HasUnmappedComputedFields(result *ParseResult) bool {
	if result == nil {
		return false
	}
	for _, cond := range result.Conditions {
		if cond.IsComputed && cond.SourceField == "" {
			return true
		}
	}
	return false
}

// HasComplexWhereConditions checks if the query has where clauses with complex functions
// (regex matching, CIDR matching, etc.) that can't be validated statically
func HasComplexWhereConditions(result *ParseResult) bool {
	if result == nil {
		return false
	}
	// Check if "where" command is used
	hasWhere := false
	for _, cmd := range result.Commands {
		if cmd == "where" {
			hasWhere = true
			break
		}
	}
	if !hasWhere {
		return false
	}

	// Check for conditions with complex operators
	// KQL complex operators: matches regex, ipv4_is_in_range, etc.
	complexOperators := map[string]bool{
		"matches":          true, // regex matching
		"matches_regex":    true, // explicit regex
		"ipv4_is_in_range": true, // CIDR matching (equivalent to SPL cidrmatch)
		"ipv6_is_in_range": true, // IPv6 CIDR matching
		"has_any":          true, // dynamic list matching
		"has_all":          true, // dynamic list matching
	}

	for _, cond := range result.Conditions {
		if complexOperators[cond.Operator] {
			return true
		}
		// Also check for negated conditions in where clauses
		if cond.Negated && cond.PipeStage > 0 {
			return true
		}
	}

	return false
}

// GetEventTypeFromConditions detects Windows Event types based on EventID conditions
// Returns event type strings like "windows_4688", "sysmon_1", etc.
func GetEventTypeFromConditions(result *ParseResult) string {
	if result == nil {
		return ""
	}
	var eventID string

	for _, cond := range result.Conditions {
		fieldLower := strings.ToLower(cond.Field)

		// Check for EventID (KQL uses EventID, not EventCode)
		if fieldLower == "eventid" {
			eventID = cond.Value
		}
	}

	if eventID == "" {
		return ""
	}

	// Map event IDs to event types
	switch eventID {
	case "4688":
		return "windows_4688"
	case "4624":
		return "windows_4624"
	case "4625":
		return "windows_4625"
	case "1":
		return "sysmon_1"
	case "3":
		return "sysmon_3"
	}

	return ""
}
