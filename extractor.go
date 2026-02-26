package kql

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/antlr4-go/antlr/v4"
)

// MaxParseTime is the maximum time allowed for parsing a single query.
// Queries that exceed this are returned with an error.
var MaxParseTime = 5 * time.Second

// Condition represents a field condition extracted from a KQL query
type Condition struct {
	Field        string   `json:"field"`
	Operator     string   `json:"operator"`
	Value        string   `json:"value"`
	Negated      bool     `json:"negated"`
	PipeStage    int      `json:"pipe_stage"`
	LogicalOp    string   `json:"logical_op"`              // "AND" or "OR" connecting to previous condition
	Alternatives []string `json:"alternatives,omitempty"`  // For OR conditions on same field
	IsComputed   bool     `json:"is_computed,omitempty"`   // True if field was created by extend/project
	SourceField  string   `json:"source_field,omitempty"`  // Original field before transformation (for computed fields)
}

// ParseResult contains all conditions extracted from the query
type ParseResult struct {
	Conditions      []Condition       `json:"conditions"`
	ComputedFields  map[string]string `json:"computed_fields,omitempty"`  // Map of computed field name -> source field (from extend)
	Commands        []string          `json:"commands,omitempty"`         // List of commands used in the query (summarize, extend, etc.)
	ProjectedFields []string          `json:"projected_fields,omitempty"` // Fields selected by project operators
	Joins           []JoinInfo        `json:"joins,omitempty"`
	Errors          []string          `json:"errors,omitempty"`
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
	conditions     []Condition
	computedFields map[string]string // Fields created by extend/project: computed field -> source field
	commands        []string          // Commands used in the query
	projectedFields []string          // Fields selected by project operators
	joins           []JoinInfo
	currentStage   int
	inSubquery     int // depth of subquery nesting
	inFunctionCall int // depth of function call nesting (countif, sumif, etc.)
	negated        bool
	lastLogicalOp  string
	errors         []string
	originalQuery  string // normalized query text for extracting subexpressions
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
	normalized := query

	// Remove line continuation backslashes (backslash followed by newline)
	// Common in PowerShell-style queries copied from scripts
	normalized = strings.ReplaceAll(normalized, "\\\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\\\r\n", "\r\n")

	// Fix escaped newlines from JSON extraction - but only outside of string literals
	// This handles queries extracted from YAML/JSON where \r\n became literal characters
	normalized = normalizeEscapeSequences(normalized)

	// Strip documentation preambles like "Description:...Query:..."
	// These are common in documentation templates where the actual query follows "Query:"
	normalized = stripDocumentationPreamble(normalized)

	// Strip leading comments to get to the actual query
	normalized = stripLeadingComments(normalized)

	// Strip declare query_parameters(...) statements
	// These define query parameters but aren't needed for condition extraction
	normalized = stripDeclareStatements(normalized)

	// Replace parameterized placeholders with dummy values
	// e.g., {TimeRange} -> 1d, {SourceTable} -> DummyTable
	normalized = replaceParameters(normalized)

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

	// Convert case-sensitive IN operators to standard forms
	// in~ -> in (case-insensitive IN becomes regular IN for parsing)
	// !in~ -> !in (case-insensitive NOT IN becomes regular NOT IN)
	normalized = strings.ReplaceAll(normalized, "!in~", "!in")
	normalized = strings.ReplaceAll(normalized, " in~ ", " in ")
	normalized = strings.ReplaceAll(normalized, "\tin~\t", "\tin\t")
	normalized = strings.ReplaceAll(normalized, "\tin~ ", "\tin ")
	normalized = strings.ReplaceAll(normalized, " in~\t", " in\t")
	normalized = strings.ReplaceAll(normalized, "\nin~", "\nin")
	normalized = strings.ReplaceAll(normalized, "in~(", "in(")

	// Convert make_set and make_list to generic function names that the parser recognizes
	// Handle both make_set( and make_set ( with space
	normalized = strings.ReplaceAll(normalized, "make_set(", "makeset(")
	normalized = strings.ReplaceAll(normalized, "make_set (", "makeset(")
	normalized = strings.ReplaceAll(normalized, "make_list(", "makelist(")
	normalized = strings.ReplaceAll(normalized, "make_list (", "makelist(")

	// Handle summarize without aggregation (summarize by x -> summarize count() by x)
	// The grammar requires aggregation before BY, but KQL allows just "summarize by"
	normalized = normalizeSummarizeBy(normalized)

	// Convert SQL-style <> to KQL != (handle all spacing variations)
	normalized = normalizeNotEqualOperator(normalized)

	// Normalize timespan literals without space: 0min -> 0 min, 30min -> 30 min
	// The lexer expects space or recognizes combined form only for certain patterns
	normalized = normalizeTimespanLiterals(normalized)

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
	normalized = strings.ReplaceAll(normalized, " and $left.", ", $left.")
	normalized = strings.ReplaceAll(normalized, " and $right.", ", $right.")

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
	normalized = strings.ReplaceAll(normalized, ".[", "[")

	// Convert bracket property access to dot notation
	// obj['key'] -> obj._key_ (the lexer's QUOTED_IDENTIFIER conflicts with bracket access)
	normalized = convertBracketAccess(normalized)

	// Fix identifiers that start with numbers by prepending underscore
	// e.g., 3plogTime -> _3plogTime
	normalized = fixNumericIdentifiers(normalized)

	// Convert tuple unpacking to single assignment
	// (a, b, c) = func() -> _tuple_result = func()
	normalized = convertTupleUnpacking(normalized)

	// Extract main query from let statements (parse only the final query for conditions)
	// This MUST happen before union normalization so (union ...) after lets is handled
	normalized = extractMainQuery(normalized)

	// Handle queries that start with a pipe (workbook queries)
	normalized = strings.TrimLeft(normalized, " \t")
	if strings.HasPrefix(normalized, "|") {
		normalized = "DummyTable " + normalized
	}

	// Handle queries that start with operators but no table name
	// Azure Resource Graph queries can start with "where", workbook queries with "extend", etc.
	lowerNorm := strings.ToLower(normalized)
	operatorPrefixes := []string{
		"where ", "where\t", "where\n",
		"extend ", "extend\t", "extend\n",
		"project ", "project\t", "project\n",
		"summarize ", "summarize\t", "summarize\n",
	}
	for _, prefix := range operatorPrefixes {
		if strings.HasPrefix(lowerNorm, prefix) {
			normalized = "DummyTable | " + normalized
			break
		}
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
	if strings.HasPrefix(strings.ToLower(normalized), "union") {
		normalized = "DummyTable | " + normalized
	}

	// Handle union inside join parentheses: join (union ...) -> join (DummyTable | union ...)
	// The grammar requires a tabularSource before union, even inside join
	normalized = normalizeJoinUnion(normalized)

	// Handle find operator: find in (Table1, Table2, ...) where ... -> extract conditions from where clause
	// The find operator isn't in the grammar, so convert to table + where
	normalized = normalizeFindOperator(normalized)

	// Handle search operator: search in (Table1, Table2) "pattern" -> simplified form
	// The search operator needs special handling
	normalized = normalizeSearchOperator(normalized)

	// Normalize top-nested operator: top-nested N of X by count() -> summarize count() by X
	// The grammar doesn't support the "of" keyword in top-nested
	normalized = normalizeTopNested(normalized)

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

// extractMainQuery extracts the main query from a let statement sequence
// For "let x = ...; let y = ...; Table | where ...", returns "Table | where ..."
func extractMainQuery(query string) string {
	// Find the position after all let statements
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

	if i >= n {
		return query // No main query found
	}

	// Find where this main query ends (before any subsequent query)
	mainQueryStart := i
	mainQueryEnd := n

	// Scan to find a new query starting after blank lines
	// A new query starts with: let statement, or table name (after optional comments)
	j := i
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
		if nlCount >= 2 {
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
			// No newline, advance to next newline or end
			for j < n && query[j] != '\n' {
				j++
			}
		}
	}

	return query[mainQueryStart:mainQueryEnd]
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
					// Check if key is a valid identifier (alphanumeric + underscore only)
					if isValidPropertyKey(key) {
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
	case 0x200b, 0x200c, 0x200d, 0xfeff: // zero-width chars
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
	// Look for patterns like "summarize by", "summarize  by", "summarize\nby"
	// and convert to "summarize count() by"
	result := query
	lowerResult := strings.ToLower(result)

	// Find all occurrences of "summarize" followed eventually by "by" without aggregation
	for {
		idx := strings.Index(lowerResult, "summarize")
		if idx == -1 {
			break
		}

		// Find where summarize ends (after whitespace)
		sumEnd := idx + 9 // length of "summarize"
		for sumEnd < len(result) && (result[sumEnd] == ' ' || result[sumEnd] == '\t' || result[sumEnd] == '\n') {
			sumEnd++
		}

		// Check if the next token is "by"
		if sumEnd+2 <= len(result) && strings.ToLower(result[sumEnd:sumEnd+2]) == "by" {
			// Check that "by" is followed by whitespace (not "byx" or similar)
			if sumEnd+2 == len(result) || result[sumEnd+2] == ' ' || result[sumEnd+2] == '\t' || result[sumEnd+2] == '\n' {
				// Insert "count() " before "by"
				result = result[:sumEnd] + "count() " + result[sumEnd:]
				lowerResult = strings.ToLower(result)
				continue
			}
		}

		// Move past this summarize
		lowerResult = lowerResult[idx+9:]
		if len(lowerResult) == 0 {
			break
		}
		// Adjust idx for next iteration - this won't work, need different approach
		break
	}

	// Use a regex-like approach for multiple occurrences
	result = query
	patterns := []string{
		"summarize by ",
		"summarize by\t",
		"summarize by\n",
		"summarize  by ",
		"summarize  by\t",
		"summarize  by\n",
		"summarize\nby ",
		"summarize\nby\t",
		"summarize\nby\n",
		"summarize\tby ",
		"summarize\tby\t",
		"summarize\tby\n",
	}

	for _, pattern := range patterns {
		replacement := strings.Replace(pattern, " by", " count() by", 1)
		replacement = strings.Replace(replacement, "\tby", " count() by", 1)
		replacement = strings.Replace(replacement, "\nby", " count() by", 1)
		result = strings.ReplaceAll(result, pattern, replacement)
		// Also handle case variations
		result = strings.ReplaceAll(result, strings.ToUpper(pattern[:1])+pattern[1:], replacement)
	}

	return result
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
		// Find "lookup " or "lookup("
		idx := strings.Index(lowerQuery[searchFrom:], "lookup ")
		idx2 := strings.Index(lowerQuery[searchFrom:], "lookup(")
		if idx == -1 && idx2 == -1 {
			break
		}
		// Pick the earliest match
		if idx == -1 || (idx2 != -1 && idx2 < idx) {
			idx = idx2
		}
		idx += searchFrom

		// Find the opening paren
		parenStart := idx + 6
		for parenStart < len(query) && (query[parenStart] == ' ' || query[parenStart] == '\t') {
			parenStart++
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
		if idx == -1 {
			idx = strings.Index(lowerQuery[searchFrom:], "| parse\t")
		}
		if idx == -1 {
			idx = strings.Index(lowerQuery[searchFrom:], "| parse\n")
		}
		if idx == -1 {
			break
		}
		idx += searchFrom

		// Find the end of this parse statement (next pipe, closing paren, or end of query)
		parseStart := idx + 2 // skip "| "
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
			// Skip the alias and = sign, read the actual field
			i++ // skip =
			// Skip whitespace after =
			for i < len(columns) && (columns[i] == ' ' || columns[i] == '\t') {
				i++
			}
			// Now read the actual field identifier
			fieldStart := i
			for i < len(columns) && (isIdentChar(columns[i]) || columns[i] == '.') {
				i++
			}
			fieldName := columns[fieldStart:i]
			result.WriteString(fieldName)
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
				// Get content inside parentheses
				content := columns[argStart : i-1]
				// Find first argument (before first comma if multi-arg)
				firstArg := content
				commaPos := strings.Index(content, ",")
				if commaPos >= 0 {
					firstArg = content[:commaPos]
				}
				firstArg = strings.TrimSpace(firstArg)

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

		// Check for trailing line comment (find last // not in a string)
		lastCommentIdx := -1
		inString := false
		stringChar := byte(0)
		for i := 0; i < len(result); i++ {
			ch := result[i]
			if inString {
				if ch == '\\' && i+1 < len(result) {
					i++ // Skip escaped char
					continue
				}
				if ch == stringChar {
					inString = false
				}
				continue
			}
			if ch == '"' || ch == '\'' {
				inString = true
				stringChar = ch
				continue
			}
			if i+1 < len(result) && ch == '/' && result[i+1] == '/' {
				lastCommentIdx = i
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

	for i := 0; i < len(query); i++ {
		c := query[i]

		// Track strings to avoid modifying decimals inside string literals
		if !inString && (c == '"' || c == '\'') {
			inString = true
			stringChar = c
			result.WriteByte(c)
			continue
		}
		if inString {
			if c == stringChar && (i == 0 || query[i-1] != '\\') {
				inString = false
			}
			result.WriteByte(c)
			continue
		}

		// Check for pattern: digit followed by . not followed by digit
		if c == '.' && i > 0 && isDigit(query[i-1]) {
			// Check what follows the dot
			nextIdx := i + 1
			if nextIdx >= len(query) || !isDigit(query[nextIdx]) {
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
	if strings.HasPrefix(trimmed, "(union") {
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

	// Find "union" keyword followed by any whitespace (space, tab, newline)
	idx := strings.Index(lowerResult, "union")
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
			isValueOrKeyword := afterEquals || lowerIdent == "true" || lowerIdent == "false"
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
			// Not at word boundary, skip this occurrence — replace "arg" with "___" in output
			b.WriteString(query[lastCopied:idx])
			b.WriteString("___")
			lastCopied = idx + 3
			searchFrom = idx + 4
			changed = true
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
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	b.Grow(len(query))
	lastCopied := 0
	changed := false
	searchFrom := 0

	for {
		idx := strings.Index(lowerQuery[searchFrom:], "externaldata")
		if idx == -1 {
			break
		}
		idx += searchFrom

		// Find the matching closing parenthesis after externaldata(
		openParenRel := strings.Index(query[idx:], "(")
		if openParenRel == -1 {
			break
		}
		openParen := idx + openParenRel

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

		// Now look for the data source block: [ ... ]
		end := closeParen
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
			if end+4 < len(query) && lowerQuery[end:end+4] == "with" {
				end += 4
				// Skip whitespace
				for end < len(query) && (query[end] == ' ' || query[end] == '\t' || query[end] == '\n' || query[end] == '\r') {
					end++
				}
				// Find ( ... )
				if end < len(query) && query[end] == '(' {
					withDepth := 1
					end++
					for end < len(query) && withDepth > 0 {
						if query[end] == '(' {
							withDepth++
						} else if query[end] == ')' {
							withDepth--
						}
						end++
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
	// Look for "Query:" or "Query:\n" pattern that separates description from actual query
	lowerQuery := strings.ToLower(query)

	// Check for common patterns like "Query:" or "KQL Query:" that precede the actual query
	patterns := []string{"query:", "kql query:", "kql:"}
	for _, pattern := range patterns {
		idx := strings.Index(lowerQuery, pattern)
		if idx != -1 {
			// Return everything after the pattern marker
			afterPattern := strings.TrimSpace(query[idx+len(pattern):])
			if len(afterPattern) > 0 {
				return afterPattern
			}
		}
	}

	// Process line by line, skipping documentation and finding where KQL starts
	lines := strings.Split(query, "\n")
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

		// Lines with many words (>6) that don't start with | are likely natural language
		if wordCount > 6 && !strings.HasPrefix(lineTrimmed, "|") {
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
		if (lastChar == ':' || lastChar == '.') && wordCount >= 4 {
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

		// Direct KQL keyword starters
		kqlStarters := []string{"let ", "union ", "where ", "search ", "find ", "range ", "print ", "datatable"}
		for _, starter := range kqlStarters {
			if strings.HasPrefix(lowerLine, starter) {
				isLikelyKQL = true
				break
			}
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

// stripLeadingComments removes leading // and # comments and URLs from a query
func stripLeadingComments(query string) string {
	lines := strings.Split(query, "\n")
	var result []string
	foundCode := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines, comment lines (// and #), and URL lines at the start
		if !foundCode {
			if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
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
		if strings.HasPrefix(trimmedLine, "/|") {
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
	replacements := map[string]string{
		"{SourceTable}":  "SecurityEvent",
		"{TotalE5Seats}": "100",
		"{Workspace}":    "workspace",
		"{Subscription}": "subscription",
	}

	result := query

	// Handle double curly braces {{param}} (Jinja2/Sentinel/Logic Apps style)
	// Convert to single curly braces first, then let regular handling take over
	{
		var b strings.Builder
		b.Grow(len(result))
		lastCopied := 0
		dcChanged := false
		searchFrom := 0
		for {
			start := strings.Index(result[searchFrom:], "{{")
			if start == -1 {
				break
			}
			start += searchFrom
			end := strings.Index(result[start+2:], "}}")
			if end == -1 {
				break
			}
			end += start + 2
			// Replace {{param}} with {param}
			b.WriteString(result[lastCopied:start])
			b.WriteByte('{')
			b.WriteString(result[start+2 : end])
			b.WriteByte('}')
			lastCopied = end + 2
			searchFrom = lastCopied
			dcChanged = true
		}
		if dcChanged {
			b.WriteString(result[lastCopied:])
			result = b.String()
		}
	}

	for param, value := range replacements {
		result = strings.ReplaceAll(result, param, value)
	}

	// Handle time range parameters with context awareness
	// {TimeRange} after a time field should become "> ago(1d)" not just "1d"
	result = replaceTimeRangeParams(result)

	// Generic replacement for any remaining {param} patterns
	// Replace {word} with a dummy identifier or expression depending on context
	for {
		start := strings.Index(result, "{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start
		// Get the entire {param}
		param := result[start : end+1]
		// Check if it looks like a parameter (contains only alphanumeric, underscore, colon)
		inner := result[start+1 : end]
		if isValidParamName(inner) {
			// Check context to determine replacement value
			replacement := "DummyParam"
			// If parameter name suggests time range and appears after a field (for filter context)
			lowerInner := strings.ToLower(inner)
			if strings.Contains(lowerInner, "time") || strings.Contains(lowerInner, "timer") ||
				strings.Contains(lowerInner, "date") || strings.Contains(lowerInner, "range") {
				// Check if preceded by a field name (time filter context)
				beforeParam := strings.TrimSpace(result[:start])
				lastWord := getLastWord(beforeParam)
				if strings.Contains(strings.ToLower(lastWord), "time") ||
					strings.Contains(strings.ToLower(lastWord), "timestamp") ||
					strings.Contains(strings.ToLower(lastWord), "date") {
					replacement = "> ago(1d)"
				}
			}
			result = strings.Replace(result, param, replacement, 1)
		} else {
			// Skip this one - might be a dynamic literal or regex
			break
		}
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
			idx := strings.Index(result[searchFrom:], param)
			if idx == -1 {
				break
			}
			idx += searchFrom

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
			Conditions: []Condition{},
			Commands:   []string{},
			Errors:     []string{fmt.Sprintf("parser timeout: query took longer than %s to parse", MaxParseTime)},
		}
	}
}

func extractConditionsInternal(query string) (result *ParseResult) {
	defer func() {
		if r := recover(); r != nil {
			result = &ParseResult{
				Conditions: []Condition{},
				Commands:   []string{},
				Errors:     []string{fmt.Sprintf("parser panic: %v", r)},
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
		conditions:     make([]Condition, 0),
		computedFields: make(map[string]string), // computed field -> source field
		commands:       make([]string, 0),
		joins:          make([]JoinInfo, 0),
		lastLogicalOp:  "AND", // default
		originalQuery:  normalizedQuery,
	}
	antlr.ParseTreeWalkerDefault.Walk(extractor, tree)

	// Combine errors
	allErrors := append(lexerErrors.errors, parserErrors.errors...)
	allErrors = append(allErrors, extractor.errors...)

	// Post-process to group OR conditions on same field
	conditions := groupORConditions(extractor.conditions)

	return &ParseResult{
		Conditions:      conditions,
		ComputedFields:  extractor.computedFields,
		Commands:        extractor.commands,
		ProjectedFields: extractor.projectedFields,
		Joins:           extractor.joins,
		Errors:          allErrors,
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

// ExitTabularOperator increments the stage counter after processing each operator
func (e *conditionExtractor) ExitTabularOperator(ctx *TabularOperatorContext) {
	e.currentStage++
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
	if ctx.Identifier() == nil {
		return
	}

	alias := ctx.Identifier().GetText()

	// Two grammar forms:
	//   identifier ASSIGN aggregationFunction   (e.g., EventCount = count())
	//   aggregationFunction AS identifier        (e.g., count() as EventCount)
	if ctx.ASSIGN() != nil || ctx.AS() != nil {
		sourceField := ""
		if ctx.AggregationFunction() != nil {
			// Try to extract source field from the aggregation function's expression
			if ctx.AggregationFunction().FunctionCall() != nil {
				fc := ctx.AggregationFunction().FunctionCall()
				if fc.Identifier() != nil {
					// The function name itself (count, sum, avg, etc.)
					funcName := strings.ToLower(fc.Identifier().GetText())
					sourceField = funcName
				}
			}
		}
		e.computedFields[strings.ToLower(alias)] = sourceField
	}
}

// EnterExtendItem tracks computed fields from extend
func (e *conditionExtractor) EnterExtendItem(ctx *ExtendItemContext) {
	// Track field assignments in extend with source field extraction
	if ctx.Identifier() != nil {
		field := ctx.Identifier().GetText()
		sourceField := ""

		// Try to extract the source field from the expression
		if ctx.Expression() != nil {
			sourceField = extractFirstFieldFromExpression(ctx.Expression())
		}

		e.computedFields[strings.ToLower(field)] = sourceField
	}
}

// extractFirstFieldFromExpression tries to extract the first field name from an expression
// This handles patterns like: tolower(CommandLine), coalesce(field1, field2), etc.
func extractFirstFieldFromExpression(ctx IExpressionContext) string {
	if ctx == nil {
		return ""
	}

	// Get the text and look for function call pattern: functionName(fieldName, ...)
	text := ctx.GetText()

	// Find the first identifier after an opening paren
	inParen := false
	start := -1
	for i, ch := range text {
		if ch == '(' {
			inParen = true
			start = i + 1
		} else if inParen && start == i {
			// Check if this is a valid field name character
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' {
				// Find the end of the field name
				end := i
				for j := i; j < len(text); j++ {
					ch := text[j]
					if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
						(ch >= '0' && ch <= '9') || ch == '_' || ch == '.' {
						end = j + 1
					} else {
						break
					}
				}
				return text[i:end]
			}
		}
	}
	return ""
}

// EnterProjectItem tracks renamed fields from project
func (e *conditionExtractor) EnterProjectItem(ctx *ProjectItemContext) {
	if e.inSubquery > 0 {
		return
	}
	if ctx.Identifier() != nil && (ctx.ASSIGN() != nil || ctx.AS() != nil) {
		// Aliased project item: project NewName = expr or project expr AS NewName
		field := ctx.Identifier().GetText()
		e.computedFields[strings.ToLower(field)] = ""
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
			isNegated := hasNOT_IN || hasNOT_IN_CS || e.negated
			e.handleInOperator(leftText, ctx.ExpressionList(), isNegated)
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
			isNegated := ctx.NOT_BETWEEN() != nil || e.negated
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

	cond := Condition{
		Field:       left,
		Operator:    normalizedOp,
		Value:       value,
		Negated:     e.negated,
		PipeStage:   e.currentStage,
		LogicalOp:   e.lastLogicalOp,
		IsComputed:  isComputed,
		SourceField: sourceField,
	}
	e.conditions = append(e.conditions, cond)
	e.lastLogicalOp = "AND" // reset to default
}

// handleInOperator processes IN operator conditions
func (e *conditionExtractor) handleInOperator(field string, exprList IExpressionListContext, negated bool) {
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
	for i, value := range values {
		logOp := e.lastLogicalOp
		if i > 0 {
			logOp = "OR"
		}
		cond := Condition{
			Field:       field,
			Operator:    "==",
			Value:       value,
			Negated:     negated,
			PipeStage:   e.currentStage,
			LogicalOp:   logOp,
			IsComputed:  isComputed,
			SourceField: sourceField,
		}
		e.conditions = append(e.conditions, cond)
	}
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

// ExitAndExpression sets logical op for next condition after AND expressions
func (e *conditionExtractor) ExitAndExpression(ctx *AndExpressionContext) {
	// Check parent to see if we're in an OR context
	parent := ctx.GetParent()
	if orCtx, ok := parent.(*OrExpressionContext); ok {
		if len(orCtx.AllAndExpression()) > 1 {
			e.lastLogicalOp = "OR"
		}
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

// extractExpressionListValues extracts values from an expression list
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

// groupORConditions groups consecutive OR conditions on the same field
func groupORConditions(conditions []Condition) []Condition {
	if len(conditions) == 0 {
		return conditions
	}

	result := make([]Condition, 0, len(conditions))

	for i := 0; i < len(conditions); i++ {
		cond := conditions[i]

		// Look ahead for OR conditions on the same field
		if i+1 < len(conditions) && conditions[i+1].LogicalOp == "OR" {
			fieldLower := strings.ToLower(cond.Field)
			alternatives := []string{cond.Value}

			j := i + 1
			for j < len(conditions) {
				next := conditions[j]
				if next.LogicalOp == "OR" && strings.ToLower(next.Field) == fieldLower && next.Operator == cond.Operator {
					alternatives = append(alternatives, next.Value)
					j++
				} else {
					break
				}
			}

			if len(alternatives) > 1 {
				cond.Alternatives = alternatives
				result = append(result, cond)
				i = j - 1 // skip the grouped conditions
				continue
			}
		}

		result = append(result, cond)
	}

	return result
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
				key := strings.ToLower(cond.Field) + "|" + cond.Operator + "|" + cond.Value
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
		"matches":            true, // regex matching
		"matches_regex":      true, // explicit regex
		"ipv4_is_in_range":   true, // CIDR matching (equivalent to SPL cidrmatch)
		"ipv6_is_in_range":   true, // IPv6 CIDR matching
		"has_any":            true, // dynamic list matching
		"has_all":            true, // dynamic list matching
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
