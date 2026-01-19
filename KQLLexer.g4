lexer grammar KQLLexer;

// =============================================================================
// KQL (Kusto Query Language) Lexer Grammar
// Production-ready lexer for Microsoft Sentinel / Azure Data Explorer queries
// =============================================================================

// -----------------------------------------------------------------------------
// TABULAR OPERATORS (case-sensitive in KQL)
// -----------------------------------------------------------------------------

// Data retrieval and filtering
WHERE           : 'where' ;
SEARCH          : 'search' ;
FIND            : 'find' ;
TAKE            : 'take' ;
LIMIT           : 'limit' ;
SAMPLE          : 'sample' ;
SAMPLE_DISTINCT : 'sample-distinct' ;
DISTINCT        : 'distinct' ;
COUNT           : 'count' ;
GETSCHEMA       : 'getschema' ;

// Projection and transformation
PROJECT         : 'project' ;
PROJECT_AWAY    : 'project-away' ;
PROJECT_KEEP    : 'project-keep' ;
PROJECT_RENAME  : 'project-rename' ;
PROJECT_REORDER : 'project-reorder' ;
EXTEND          : 'extend' ;
PARSE           : 'parse' ;
PARSE_WHERE     : 'parse-where' ;
PARSE_KV        : 'parse-kv' ;

// Sorting and limiting
SORT            : 'sort' ;
ORDER           : 'order' ;
TOP             : 'top' ;
TOP_NESTED      : 'top-nested' ;
TOP_HITTERS     : 'top-hitters' ;

// Aggregation
SUMMARIZE       : 'summarize' ;
MAKE_SERIES     : 'make-series' ;
MAKE_LIST       : 'make_list' ;
MAKE_SET        : 'make_set' ;

// Joining and combining
JOIN            : 'join' ;
LOOKUP          : 'lookup' ;
UNION           : 'union' ;
AS              : 'as' ;

// Multi-value expansion
MV_EXPAND       : 'mv-expand' ;
MV_APPLY        : 'mv-apply' ;

// Advanced operators
EVALUATE        : 'evaluate' ;
INVOKE          : 'invoke' ;
FACET           : 'facet' ;
FORK            : 'fork' ;
PARTITION       : 'partition' ;
SCAN            : 'scan' ;
SERIALIZE       : 'serialize' ;
RANGE           : 'range' ;
PRINT           : 'print' ;
RENDER          : 'render' ;
CONSUME         : 'consume' ;
EXTERNALDATA    : 'externaldata' ;

// Graph operators
MAKE_GRAPH      : 'make-graph' ;
GRAPH_MATCH     : 'graph-match' ;
GRAPH_SHORTEST_PATHS : 'graph-shortest-paths' ;
GRAPH_TO_TABLE  : 'graph-to-table' ;

// -----------------------------------------------------------------------------
// STATEMENT KEYWORDS
// -----------------------------------------------------------------------------

LET             : 'let' ;
SET             : 'set' ;
ALIAS           : 'alias' ;
DECLARE         : 'declare' ;
PATTERN         : 'pattern' ;
RESTRICT        : 'restrict' ;
ACCESS          : 'access' ;
MATERIALIZE     : 'materialize' ;

// -----------------------------------------------------------------------------
// JOIN KINDS AND HINTS
// -----------------------------------------------------------------------------

KIND            : 'kind' ;
HINT_DOT        : 'hint.' ;
INNER           : 'inner' ;
OUTER           : 'outer' ;
LEFT            : 'left' ;
RIGHT           : 'right' ;
FULL            : 'full' ;
LEFTSEMI        : 'leftsemi' ;
RIGHTSEMI       : 'rightsemi' ;
LEFTANTI        : 'leftanti' ;
RIGHTANTI       : 'rightanti' ;
LEFTOUTER       : 'leftouter' ;
RIGHTOUTER      : 'rightouter' ;
FULLOUTER       : 'fullouter' ;
ANTI            : 'anti' ;
SEMI            : 'semi' ;
INNERUNIQUE     : 'innerunique' ;

// -----------------------------------------------------------------------------
// LOGICAL OPERATORS
// -----------------------------------------------------------------------------

AND             : 'and' ;
OR              : 'or' ;
NOT             : 'not' | '!' ;
BETWEEN         : 'between' ;
NOT_BETWEEN     : '!between' ;
IN              : 'in' ;
NOT_IN          : '!in' ;
IN_CS           : 'in~' ;
NOT_IN_CS       : '!in~' ;
HAS_ANY         : 'has_any' ;
HAS_ALL         : 'has_all' ;

// -----------------------------------------------------------------------------
// STRING OPERATORS (KQL-specific)
// -----------------------------------------------------------------------------

// Case-sensitive string operators
CONTAINS        : 'contains' ;
NOT_CONTAINS    : '!contains' ;
CONTAINS_CS     : 'contains_cs' ;
NOT_CONTAINS_CS : '!contains_cs' ;
HAS             : 'has' ;
NOT_HAS         : '!has' ;
HAS_CS          : 'has_cs' ;
NOT_HAS_CS      : '!has_cs' ;
HASPREFIX       : 'hasprefix' ;
NOT_HASPREFIX   : '!hasprefix' ;
HASPREFIX_CS    : 'hasprefix_cs' ;
NOT_HASPREFIX_CS: '!hasprefix_cs' ;
HASSUFFIX       : 'hassuffix' ;
NOT_HASSUFFIX   : '!hassuffix' ;
HASSUFFIX_CS    : 'hassuffix_cs' ;
NOT_HASSUFFIX_CS: '!hassuffix_cs' ;
STARTSWITH      : 'startswith' ;
NOT_STARTSWITH  : '!startswith' ;
STARTSWITH_CS   : 'startswith_cs' ;
NOT_STARTSWITH_CS: '!startswith_cs' ;
ENDSWITH        : 'endswith' ;
NOT_ENDSWITH    : '!endswith' ;
ENDSWITH_CS     : 'endswith_cs' ;
NOT_ENDSWITH_CS : '!endswith_cs' ;
MATCHES_REGEX   : 'matches' WS+ 'regex' ;
MATCHES         : 'matches' ;

// -----------------------------------------------------------------------------
// COMPARISON OPERATORS
// -----------------------------------------------------------------------------

EQ              : '==' ;
ASSIGN          : '=' ;
NEQ             : '!=' ;
LT              : '<' ;
GT              : '>' ;
LTE             : '<=' ;
GTE             : '>=' ;
EQTILDE         : '=~' ;    // Case-insensitive equality
NEQTILDE        : '!~' ;    // Case-insensitive inequality

// -----------------------------------------------------------------------------
// ARITHMETIC OPERATORS
// -----------------------------------------------------------------------------

PLUS            : '+' ;
MINUS           : '-' ;
STAR            : '*' ;
SLASH           : '/' ;
PERCENT         : '%' ;

// -----------------------------------------------------------------------------
// SPECIAL OPERATORS
// -----------------------------------------------------------------------------

DOTDOT          : '..' ;    // Range operator
ARROW           : '=>' ;    // Lambda arrow
QUESTION        : '?' ;     // Null-coalescing
QUESTIONDOT     : '?.' ;    // Safe navigation

// -----------------------------------------------------------------------------
// DELIMITERS
// -----------------------------------------------------------------------------

PIPE            : '|' ;
SEMICOLON       : ';' ;
COLON           : ':' ;
COMMA           : ',' ;
DOT             : '.' ;
LPAREN          : '(' ;
RPAREN          : ')' ;
LBRACKET        : '[' ;
RBRACKET        : ']' ;
LBRACE          : '{' ;
RBRACE          : '}' ;

// -----------------------------------------------------------------------------
// SORT DIRECTION
// -----------------------------------------------------------------------------

ASC             : 'asc' ;
DESC            : 'desc' ;
NULLS           : 'nulls' ;
FIRST           : 'first' ;
LAST            : 'last' ;

// -----------------------------------------------------------------------------
// COMMON KEYWORDS
// -----------------------------------------------------------------------------

BY              : 'by' ;
ON              : 'on' ;
WITH            : 'with' ;
OF              : 'of' ;
TO              : 'to' ;
FROM            : 'from' ;
STEP            : 'step' ;
DEFAULT         : 'default' ;
TYPEOF          : 'typeof' ;
PACK            : 'pack' ;
PACK_ALL        : 'pack_all' ;
BAG_PACK        : 'bag_pack' ;

// Additional keywords used in parser
VIEW            : 'view' ;
OTHERS          : 'others' ;
DOLLAR          : '$' ;
WITH_SOURCE     : 'withsource' ;
IS_FUZZY        : 'isfuzzy' ;
BAG_EXPANSION   : 'bagexpansion' ;
WITH_ITEMINDEX  : 'with_itemindex' ;
WITH_MATCH_ID   : 'with_match_id' ;
OUTPUT          : 'output' ;
DATA_SCOPE      : 'datascope' ;
DECODEBLOCKS    : 'decodeblocks' ;
NODES           : 'nodes' ;
EDGES           : 'edges' ;
DATATABLE       : 'datatable' ;
CASE            : 'case' ;
IFF             : 'iff' ;
IIF             : 'iif' ;
TOSCALAR        : 'toscalar' ;
PACK_ARRAY      : 'pack_array' ;

// -----------------------------------------------------------------------------
// BOOLEAN LITERALS
// -----------------------------------------------------------------------------

TRUE            : 'true' ;
FALSE           : 'false' ;

// -----------------------------------------------------------------------------
// NULL LITERAL
// -----------------------------------------------------------------------------

NULL            : 'null' ;

// -----------------------------------------------------------------------------
// DATETIME LITERALS
// Supports: datetime(2024-01-15), datetime(2024-01-15T10:30:00Z)
// -----------------------------------------------------------------------------

DATETIME_LITERAL
    : 'datetime' WS* '(' WS* DATETIME_VALUE WS* ')'
    ;

fragment DATETIME_VALUE
    : DIGIT DIGIT DIGIT DIGIT '-' DIGIT DIGIT '-' DIGIT DIGIT
      ('T' DIGIT DIGIT ':' DIGIT DIGIT (':' DIGIT DIGIT ('.' DIGIT+)?)? 'Z'?)?
    | 'null'
    ;

// -----------------------------------------------------------------------------
// TIMESPAN LITERALS
// Supports: 1d, 2h, 30m, 45s, 100ms, time(1.02:03:04)
// -----------------------------------------------------------------------------

TIMESPAN_LITERAL
    : 'timespan' WS* '(' WS* TIMESPAN_VALUE WS* ')'
    | 'time' WS* '(' WS* TIMESPAN_VALUE WS* ')'
    ;

fragment TIMESPAN_VALUE
    : DIGIT+ '.' DIGIT+ ':' DIGIT+ ':' DIGIT+ ('.' DIGIT+)?
    | DIGIT+ ':' DIGIT+ ':' DIGIT+ ('.' DIGIT+)?
    | 'null'
    ;

// Short timespan format: 1d, 2h, 30m, etc.
TIMESPAN_SHORT
    : DIGIT+ ('.' DIGIT+)? [dhmsMy]
    | DIGIT+ 'ms'
    | DIGIT+ 'tick' 's'?
    | DIGIT+ 'microsecond' 's'?
    ;

// -----------------------------------------------------------------------------
// DYNAMIC LITERAL (JSON-like)
// -----------------------------------------------------------------------------

DYNAMIC_LITERAL
    : 'dynamic' WS* '(' WS* (DYNAMIC_NULL | DYNAMIC_ARRAY | DYNAMIC_OBJECT) WS* ')'
    ;

fragment DYNAMIC_NULL   : 'null' ;
fragment DYNAMIC_ARRAY  : '[' .*? ']' ;
fragment DYNAMIC_OBJECT : '{' .*? '}' ;

// -----------------------------------------------------------------------------
// GUID LITERAL
// -----------------------------------------------------------------------------

GUID_LITERAL
    : 'guid' WS* '(' WS* GUID_VALUE WS* ')'
    ;

fragment GUID_VALUE
    : HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT
      '-' HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT
      '-' HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT
      '-' HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT
      '-' HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT
    | 'null'
    ;

// -----------------------------------------------------------------------------
// TYPE KEYWORDS (for typeof and casts)
// -----------------------------------------------------------------------------

TYPE_BOOL       : 'bool' ;
TYPE_DATETIME   : 'datetime' ;
TYPE_DECIMAL    : 'decimal' ;
TYPE_DOUBLE     : 'double' ;
TYPE_DYNAMIC    : 'dynamic' ;
TYPE_GUID       : 'guid' ;
TYPE_INT        : 'int' ;
TYPE_LONG       : 'long' ;
TYPE_REAL       : 'real' ;
TYPE_STRING     : 'string' ;
TYPE_TIMESPAN   : 'timespan' ;

// -----------------------------------------------------------------------------
// STRING LITERALS
// Supports: "string", 'string', @"verbatim", @'verbatim', multi-line strings
// -----------------------------------------------------------------------------

STRING_LITERAL
    : '"' (ESC_SEQ | ~["\\\r\n])* '"'
    | '\'' (ESC_SEQ | ~['\\\r\n])* '\''
    ;

// Verbatim strings (@ prefix, no escape processing)
VERBATIM_STRING
    : '@"' (~["] | '""')* '"'
    | '@\'' (~['] | '\'\'')* '\''
    ;

// Multi-line string literals
MULTILINE_STRING
    : '```' .*? '```'
    | '"""' .*? '"""'
    ;

fragment ESC_SEQ
    : '\\' [btnfr"'\\]
    | '\\x' HEX_DIGIT HEX_DIGIT
    | '\\u' HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT
    ;

// -----------------------------------------------------------------------------
// NUMERIC LITERALS
// -----------------------------------------------------------------------------

// Hexadecimal
HEX_NUMBER
    : '0' [xX] HEX_DIGIT+
    ;

// Real/Double (must come before INT to properly match)
REAL_NUMBER
    : DIGIT+ '.' DIGIT+ ([eE] [+-]? DIGIT+)?
    | DIGIT+ [eE] [+-]? DIGIT+
    | '.' DIGIT+ ([eE] [+-]? DIGIT+)?
    ;

// Long literal
LONG_NUMBER
    : DIGIT+ [lL]
    ;

// Integer
INT_NUMBER
    : DIGIT+
    ;

// Decimal literal
DECIMAL_NUMBER
    : DIGIT+ '.' DIGIT+ [mM]
    | DIGIT+ [mM]
    ;

fragment DIGIT     : [0-9] ;
fragment HEX_DIGIT : [0-9a-fA-F] ;

// -----------------------------------------------------------------------------
// IDENTIFIERS
// -----------------------------------------------------------------------------

// Regular identifier
IDENTIFIER
    : [a-zA-Z_] [a-zA-Z0-9_]*
    ;

// Quoted identifier (for special characters in names)
QUOTED_IDENTIFIER
    : '[' ~[\]\r\n]+ ']'
    | '[\'' ~['\r\n]+ '\']'
    ;

// Client parameter (for parameterized queries)
CLIENT_PARAMETER
    : '{' [a-zA-Z_] [a-zA-Z0-9_]* '}'
    ;

// -----------------------------------------------------------------------------
// COMMENTS
// -----------------------------------------------------------------------------

LINE_COMMENT
    : '//' ~[\r\n]* -> channel(HIDDEN)
    ;

BLOCK_COMMENT
    : '/*' .*? '*/' -> channel(HIDDEN)
    ;

// -----------------------------------------------------------------------------
// WHITESPACE
// -----------------------------------------------------------------------------

WS
    : [ \t\r\n\u000C]+ -> skip
    ;

// -----------------------------------------------------------------------------
// ERROR FALLBACK
// Catch any unrecognized characters to prevent lexer errors
// -----------------------------------------------------------------------------

ERROR_CHAR
    : .
    ;
