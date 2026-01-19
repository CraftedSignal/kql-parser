parser grammar KQLParser;

options { tokenVocab=KQLLexer; }

// =============================================================================
// KQL (Kusto Query Language) Parser Grammar
// Production-ready parser for Microsoft Sentinel / Azure Data Explorer queries
// =============================================================================

// -----------------------------------------------------------------------------
// ENTRY POINT
// A KQL query consists of optional statements followed by a tabular expression
// -----------------------------------------------------------------------------

query
    : statement* tabularExpression? EOF
    ;

// -----------------------------------------------------------------------------
// STATEMENTS
// Let statements, set statements, etc.
// -----------------------------------------------------------------------------

statement
    : letStatement SEMICOLON?
    | setStatement SEMICOLON?
    | aliasStatement SEMICOLON?
    | declareStatement SEMICOLON?
    | patternStatement SEMICOLON?
    | restrictStatement SEMICOLON?
    ;

letStatement
    : LET identifier ASSIGN expression
    | LET identifier ASSIGN LBRACE tabularExpression RBRACE
    | LET identifier ASSIGN LPAREN functionParameters? RPAREN LBRACE tabularExpression RBRACE
    | LET identifier ASSIGN viewExpression
    ;

setStatement
    : SET identifier (ASSIGN | EQ) expression
    ;

aliasStatement
    : ALIAS identifier ASSIGN expression
    ;

declareStatement
    : DECLARE PATTERN identifier ASSIGN patternDefinition
    | DECLARE identifier COLON typeSpecifier
    ;

patternStatement
    : PATTERN identifier patternDefinition
    ;

restrictStatement
    : RESTRICT ACCESS TO LPAREN identifierList RPAREN
    ;

viewExpression
    : VIEW LPAREN RPAREN LBRACE tabularExpression RBRACE
    ;

patternDefinition
    : LPAREN patternParam (COMMA patternParam)* RPAREN
    ;

patternParam
    : identifier (COLON typeSpecifier)?
    ;

// -----------------------------------------------------------------------------
// TABULAR EXPRESSION
// The core query structure: table | operator | operator ...
// -----------------------------------------------------------------------------

tabularExpression
    : tabularSource (PIPE tabularOperator)*
    ;

tabularSource
    : tableName
    | functionCall
    | LPAREN tabularExpression RPAREN
    | RANGE identifier FROM expression TO expression STEP expression
    | PRINT printArgList
    | datatable
    | externalData
    | materializeExpression
    ;

tableName
    : databaseTableName
    | identifier
    | QUOTED_IDENTIFIER
    ;

databaseTableName
    : identifier DOT identifier DOT identifier   // cluster.database.table
    | identifier DOT identifier                   // database.table
    ;

materializeExpression
    : MATERIALIZE LPAREN tabularExpression RPAREN
    ;

// -----------------------------------------------------------------------------
// TABULAR OPERATORS
// All KQL operators that transform tabular data
// -----------------------------------------------------------------------------

tabularOperator
    : whereOperator
    | searchOperator
    | projectOperator
    | projectAwayOperator
    | projectKeepOperator
    | projectRenameOperator
    | projectReorderOperator
    | extendOperator
    | summarizeOperator
    | sortOperator
    | topOperator
    | topNestedOperator
    | takeOperator
    | distinctOperator
    | countOperator
    | joinOperator
    | unionOperator
    | lookupOperator
    | parseOperator
    | parseKvOperator
    | mvExpandOperator
    | mvApplyOperator
    | evaluateOperator
    | facetOperator
    | forkOperator
    | partitionOperator
    | scanOperator
    | serializeOperator
    | sampleOperator
    | sampleDistinctOperator
    | makeSeriesOperator
    | findOperator
    | getschemaOperator
    | renderOperator
    | consumeOperator
    | invokeOperator
    | asOperator
    | graphOperator
    ;

// -----------------------------------------------------------------------------
// WHERE OPERATOR
// Filters rows based on predicate
// -----------------------------------------------------------------------------

whereOperator
    : WHERE expression
    ;

// -----------------------------------------------------------------------------
// SEARCH OPERATOR
// Full-text search across columns
// -----------------------------------------------------------------------------

searchOperator
    : SEARCH searchKind? (IN LPAREN tableList RPAREN)? expression
    ;

searchKind
    : KIND ASSIGN identifier
    ;

tableList
    : tableName (COMMA tableName)*
    ;

// -----------------------------------------------------------------------------
// PROJECT OPERATORS
// Column selection and transformation
// -----------------------------------------------------------------------------

projectOperator
    : PROJECT projectItemList
    ;

projectAwayOperator
    : PROJECT_AWAY identifierOrWildcardList
    ;

projectKeepOperator
    : PROJECT_KEEP identifierOrWildcardList
    ;

projectRenameOperator
    : PROJECT_RENAME renameList
    ;

projectReorderOperator
    : PROJECT_REORDER identifierOrWildcardList (ASC | DESC)?
    ;

projectItemList
    : projectItem (COMMA projectItem)*
    ;

projectItem
    : expression (AS identifier)?
    | identifier ASSIGN expression
    ;

identifierOrWildcardList
    : identifierOrWildcard (COMMA identifierOrWildcard)*
    ;

identifierOrWildcard
    : identifier
    | STAR
    | identifier STAR        // prefix*
    | STAR identifier        // *suffix
    | identifier STAR identifier  // contains pattern
    ;

renameList
    : renameItem (COMMA renameItem)*
    ;

renameItem
    : identifier ASSIGN identifier
    ;

// -----------------------------------------------------------------------------
// EXTEND OPERATOR
// Add computed columns
// -----------------------------------------------------------------------------

extendOperator
    : EXTEND extendItemList
    ;

extendItemList
    : extendItem (COMMA extendItem)*
    ;

extendItem
    : identifier ASSIGN expression
    | expression (AS identifier)?
    ;

// -----------------------------------------------------------------------------
// SUMMARIZE OPERATOR
// Aggregation
// -----------------------------------------------------------------------------

summarizeOperator
    : SUMMARIZE summarizeHints? aggregationList (BY groupByList)?
    ;

summarizeHints
    : HINT_DOT identifier ASSIGN expression
    ;

aggregationList
    : aggregationItem (COMMA aggregationItem)*
    ;

aggregationItem
    : identifier ASSIGN aggregationFunction
    | aggregationFunction (AS identifier)?
    ;

aggregationFunction
    : functionCall
    | expression
    ;

groupByList
    : groupByItem (COMMA groupByItem)*
    ;

groupByItem
    : expression (AS identifier)?
    | identifier ASSIGN expression
    ;

// -----------------------------------------------------------------------------
// SORT OPERATOR
// Order rows
// -----------------------------------------------------------------------------

sortOperator
    : (SORT | ORDER) BY sortList
    ;

sortList
    : sortItem (COMMA sortItem)*
    ;

sortItem
    : expression sortDirection? nullsPosition?
    ;

sortDirection
    : ASC
    | DESC
    ;

nullsPosition
    : NULLS FIRST
    | NULLS LAST
    ;

// -----------------------------------------------------------------------------
// TOP OPERATOR
// Top N rows
// -----------------------------------------------------------------------------

topOperator
    : TOP expression BY sortList
    ;

topNestedOperator
    : TOP_NESTED topNestedClause (COMMA topNestedClause)*
    ;

topNestedClause
    : (OF identifier)? expression BY expression (ASC | DESC)? (WITH OTHERS ASSIGN expression)?
    ;


// -----------------------------------------------------------------------------
// TAKE / LIMIT OPERATOR
// Limit number of rows
// -----------------------------------------------------------------------------

takeOperator
    : (TAKE | LIMIT) expression
    ;

// -----------------------------------------------------------------------------
// DISTINCT OPERATOR
// Remove duplicate rows
// -----------------------------------------------------------------------------

distinctOperator
    : DISTINCT distinctColumns?
    ;

distinctColumns
    : identifierOrWildcardList
    | STAR
    ;

// -----------------------------------------------------------------------------
// COUNT OPERATOR
// Count rows
// -----------------------------------------------------------------------------

countOperator
    : COUNT
    ;

// -----------------------------------------------------------------------------
// JOIN OPERATOR
// Combine tables
// -----------------------------------------------------------------------------

joinOperator
    : JOIN joinKind? joinHints? LPAREN tabularExpression RPAREN ON joinCondition
    | JOIN joinKind? joinHints? tableName ON joinCondition
    ;

joinKind
    : KIND ASSIGN joinFlavor
    ;

joinFlavor
    : INNER
    | OUTER
    | LEFT
    | RIGHT
    | FULL
    | LEFTSEMI
    | RIGHTSEMI
    | LEFTANTI
    | RIGHTANTI
    | LEFTOUTER
    | RIGHTOUTER
    | FULLOUTER
    | INNERUNIQUE
    | ANTI
    | SEMI
    ;

joinHints
    : joinHint+
    ;

joinHint
    : HINT_DOT identifier ASSIGN expression
    ;

joinCondition
    : joinAttribute (COMMA joinAttribute)*
    | expression
    ;

joinAttribute
    : identifier
    | DOLLAR LEFT DOT identifier EQ DOLLAR RIGHT DOT identifier
    ;


// -----------------------------------------------------------------------------
// UNION OPERATOR
// Combine multiple tables
// -----------------------------------------------------------------------------

unionOperator
    : UNION unionParameters? unionTables
    ;

unionParameters
    : unionParameter+
    ;

unionParameter
    : KIND ASSIGN identifier
    | WITH_SOURCE ASSIGN identifier
    | IS_FUZZY ASSIGN booleanLiteral
    ;


unionTables
    : unionTable (COMMA unionTable)*
    ;

unionTable
    : LPAREN tabularExpression RPAREN
    | tableName
    ;

// -----------------------------------------------------------------------------
// LOOKUP OPERATOR
// Enrich data from dimension table
// -----------------------------------------------------------------------------

lookupOperator
    : LOOKUP lookupKind? LPAREN tabularExpression RPAREN ON lookupCondition
    | LOOKUP lookupKind? tableName ON lookupCondition
    ;

lookupKind
    : KIND ASSIGN identifier
    ;

lookupCondition
    : joinCondition
    ;

// -----------------------------------------------------------------------------
// PARSE OPERATOR
// Extract data from strings
// -----------------------------------------------------------------------------

parseOperator
    : PARSE parseKind? expression WITH? parsePattern
    | PARSE_WHERE parseKind? expression WITH? parsePattern
    ;

parseKind
    : KIND ASSIGN identifier
    ;

parsePattern
    : parsePatternItem+
    ;

parsePatternItem
    : STRING_LITERAL
    | VERBATIM_STRING
    | STAR
    | identifier COLON typeSpecifier?
    ;

parseKvOperator
    : PARSE_KV expression AS LPAREN kvPairList RPAREN parseKvParameters?
    ;

kvPairList
    : kvPair (COMMA kvPair)*
    ;

kvPair
    : identifier COLON typeSpecifier
    ;

parseKvParameters
    : WITH LPAREN parseKvParam (COMMA parseKvParam)* RPAREN
    ;

parseKvParam
    : identifier ASSIGN expression
    ;

// -----------------------------------------------------------------------------
// MV-EXPAND OPERATOR
// Expand multi-value columns
// -----------------------------------------------------------------------------

mvExpandOperator
    : MV_EXPAND mvExpandKind? mvExpandParams? mvExpandItemList limitClause?
    ;

mvExpandKind
    : KIND ASSIGN identifier
    ;

mvExpandParams
    : BAG_EXPANSION ASSIGN identifier
    | WITH_ITEMINDEX ASSIGN identifier
    ;


mvExpandItemList
    : mvExpandItem (COMMA mvExpandItem)*
    ;

mvExpandItem
    : expression (TO TYPEOF LPAREN typeSpecifier RPAREN)?
    | identifier ASSIGN expression (TO TYPEOF LPAREN typeSpecifier RPAREN)?
    ;

limitClause
    : LIMIT expression
    ;

// -----------------------------------------------------------------------------
// MV-APPLY OPERATOR
// Apply subquery to each multi-value element
// -----------------------------------------------------------------------------

mvApplyOperator
    : MV_APPLY mvApplyItemList mvApplyOnClause? LPAREN tabularExpression RPAREN
    ;

mvApplyItemList
    : mvApplyItem (COMMA mvApplyItem)*
    ;

mvApplyItem
    : expression (TO TYPEOF LPAREN typeSpecifier RPAREN)? (AS identifier)?
    | identifier ASSIGN expression (TO TYPEOF LPAREN typeSpecifier RPAREN)?
    ;

mvApplyOnClause
    : ON identifierList
    ;

// -----------------------------------------------------------------------------
// EVALUATE OPERATOR
// Call plugin functions
// -----------------------------------------------------------------------------

evaluateOperator
    : EVALUATE evaluateHints? functionCall
    ;

evaluateHints
    : HINT_DOT identifier ASSIGN expression
    ;

// -----------------------------------------------------------------------------
// FACET OPERATOR
// Group and summarize by multiple columns
// -----------------------------------------------------------------------------

facetOperator
    : FACET BY identifierList (WITH LPAREN tabularExpression RPAREN)?
    ;

// -----------------------------------------------------------------------------
// FORK OPERATOR
// Split query into multiple branches
// -----------------------------------------------------------------------------

forkOperator
    : FORK forkBranch+
    ;

forkBranch
    : LPAREN tabularExpression RPAREN
    ;

// -----------------------------------------------------------------------------
// PARTITION OPERATOR
// Parallel processing by partition key
// -----------------------------------------------------------------------------

partitionOperator
    : PARTITION partitionHints? BY expression LPAREN tabularExpression RPAREN
    ;

partitionHints
    : HINT_DOT identifier ASSIGN expression
    ;

// -----------------------------------------------------------------------------
// SCAN OPERATOR
// Stateful row-by-row processing
// -----------------------------------------------------------------------------

scanOperator
    : SCAN scanParams? scanDeclare? WITH LPAREN scanStepList RPAREN
    ;

scanParams
    : WITH_MATCH_ID ASSIGN identifier
    ;


scanDeclare
    : DECLARE LPAREN scanDeclareItem (COMMA scanDeclareItem)* RPAREN
    ;

scanDeclareItem
    : identifier COLON typeSpecifier (ASSIGN expression)?
    ;

scanStepList
    : scanStep (COMMA scanStep)*
    ;

scanStep
    : STEP identifier (OUTPUT ASSIGN identifier)? COLON expression ARROW LBRACE scanAction+ RBRACE
    ;


scanAction
    : identifier ASSIGN expression SEMICOLON?
    ;

// -----------------------------------------------------------------------------
// SERIALIZE OPERATOR
// Mark for ordered processing
// -----------------------------------------------------------------------------

serializeOperator
    : SERIALIZE extendItemList?
    ;

// -----------------------------------------------------------------------------
// SAMPLE OPERATORS
// Random sampling
// -----------------------------------------------------------------------------

sampleOperator
    : SAMPLE expression
    ;

sampleDistinctOperator
    : SAMPLE_DISTINCT expression OF identifier
    ;

// -----------------------------------------------------------------------------
// MAKE-SERIES OPERATOR
// Create time series
// -----------------------------------------------------------------------------

makeSeriesOperator
    : MAKE_SERIES makeSeriesItemList makeSeriesOnClause makeSeriesParams?
    ;

makeSeriesItemList
    : makeSeriesItem (COMMA makeSeriesItem)*
    ;

makeSeriesItem
    : identifier? ASSIGN? aggregationFunction (DEFAULT ASSIGN expression)?
    ;

makeSeriesOnClause
    : ON expression (FROM expression TO expression)? STEP expression (BY groupByList)?
    ;

makeSeriesParams
    : KIND ASSIGN identifier
    ;

// -----------------------------------------------------------------------------
// FIND OPERATOR
// Search across multiple tables
// -----------------------------------------------------------------------------

findOperator
    : FIND findParams? (IN LPAREN tableList RPAREN)? WHERE expression (PROJECT projectItemList)?
    ;

findParams
    : WITH_SOURCE ASSIGN identifier
    | DATA_SCOPE ASSIGN identifier
    ;


// -----------------------------------------------------------------------------
// GETSCHEMA OPERATOR
// Return schema information
// -----------------------------------------------------------------------------

getschemaOperator
    : GETSCHEMA
    ;

// -----------------------------------------------------------------------------
// RENDER OPERATOR
// Visualization hint
// -----------------------------------------------------------------------------

renderOperator
    : RENDER identifier renderProperties?
    ;

renderProperties
    : WITH LPAREN renderProperty (COMMA renderProperty)* RPAREN
    ;

renderProperty
    : identifier ASSIGN expression
    ;

// -----------------------------------------------------------------------------
// CONSUME OPERATOR
// Force execution without returning results
// -----------------------------------------------------------------------------

consumeOperator
    : CONSUME (DECODEBLOCKS ASSIGN booleanLiteral)?
    ;


// -----------------------------------------------------------------------------
// INVOKE OPERATOR
// Call stored function with current table
// -----------------------------------------------------------------------------

invokeOperator
    : INVOKE functionCall
    ;

// -----------------------------------------------------------------------------
// AS OPERATOR
// Name the result for subsequent use
// -----------------------------------------------------------------------------

asOperator
    : AS identifier
    ;

// -----------------------------------------------------------------------------
// GRAPH OPERATORS
// Graph query support
// -----------------------------------------------------------------------------

graphOperator
    : makeGraphOperator
    | graphMatchOperator
    | graphShortestPathsOperator
    | graphToTableOperator
    ;

makeGraphOperator
    : MAKE_GRAPH identifier ARROW identifier WITH tableName ON identifier
    ;

graphMatchOperator
    : GRAPH_MATCH graphPattern (PROJECT projectItemList)? (WHERE expression)?
    ;

graphPattern
    : graphPatternElement+
    ;

graphPatternElement
    : LPAREN identifier (COLON identifier)? RPAREN graphEdge?
    ;

graphEdge
    : MINUS LBRACKET identifier? (COLON identifier)? RBRACKET ARROW graphPatternElement
    | LT MINUS LBRACKET identifier? (COLON identifier)? RBRACKET MINUS graphPatternElement
    ;

graphShortestPathsOperator
    : GRAPH_SHORTEST_PATHS graphPattern (PROJECT projectItemList)?
    ;

graphToTableOperator
    : GRAPH_TO_TABLE graphToTableParams
    ;

graphToTableParams
    : (NODES | EDGES) (AS identifier)?
    ;


// -----------------------------------------------------------------------------
// DATATABLE
// Inline table definition
// -----------------------------------------------------------------------------

datatable
    : DATATABLE LPAREN datatableSchema RPAREN LBRACKET datatableRows RBRACKET
    ;


datatableSchema
    : datatableColumn (COMMA datatableColumn)*
    ;

datatableColumn
    : identifier COLON typeSpecifier
    ;

datatableRows
    : (literal (COMMA literal)*)?
    ;

// -----------------------------------------------------------------------------
// EXTERNAL DATA
// Query external data sources
// -----------------------------------------------------------------------------

externalData
    : EXTERNALDATA LPAREN datatableSchema RPAREN LBRACKET externalDataUri (COMMA externalDataUri)* RBRACKET externalDataOptions?
    ;

externalDataUri
    : STRING_LITERAL
    | VERBATIM_STRING
    ;

externalDataOptions
    : WITH LPAREN externalDataOption (COMMA externalDataOption)* RPAREN
    ;

externalDataOption
    : identifier ASSIGN expression
    ;

// -----------------------------------------------------------------------------
// PRINT
// Output literal values
// -----------------------------------------------------------------------------

printArgList
    : printArg (COMMA printArg)*
    ;

printArg
    : identifier ASSIGN expression
    | expression
    ;

// -----------------------------------------------------------------------------
// EXPRESSIONS
// Full expression support with proper precedence
// -----------------------------------------------------------------------------

expression
    : orExpression
    ;

orExpression
    : andExpression (OR andExpression)*
    ;

andExpression
    : notExpression (AND notExpression)*
    ;

notExpression
    : NOT notExpression
    | comparisonExpression
    ;

comparisonExpression
    : additiveExpression comparisonOperator additiveExpression
    | additiveExpression BETWEEN LPAREN additiveExpression DOTDOT additiveExpression RPAREN
    | additiveExpression NOT_BETWEEN LPAREN additiveExpression DOTDOT additiveExpression RPAREN
    | additiveExpression IN LPAREN expressionList RPAREN
    | additiveExpression IN tableName
    | additiveExpression NOT_IN LPAREN expressionList RPAREN
    | additiveExpression IN_CS LPAREN expressionList RPAREN
    | additiveExpression NOT_IN_CS LPAREN expressionList RPAREN
    | additiveExpression HAS_ANY LPAREN expressionList RPAREN
    | additiveExpression HAS_ALL LPAREN expressionList RPAREN
    | additiveExpression stringOperator additiveExpression
    | additiveExpression
    ;

comparisonOperator
    : EQ
    | NEQ
    | LT
    | GT
    | LTE
    | GTE
    | EQTILDE
    | NEQTILDE
    ;

stringOperator
    : CONTAINS
    | NOT_CONTAINS
    | CONTAINS_CS
    | NOT_CONTAINS_CS
    | HAS
    | NOT_HAS
    | HAS_CS
    | NOT_HAS_CS
    | HASPREFIX
    | NOT_HASPREFIX
    | HASPREFIX_CS
    | NOT_HASPREFIX_CS
    | HASSUFFIX
    | NOT_HASSUFFIX
    | HASSUFFIX_CS
    | NOT_HASSUFFIX_CS
    | STARTSWITH
    | NOT_STARTSWITH
    | STARTSWITH_CS
    | NOT_STARTSWITH_CS
    | ENDSWITH
    | NOT_ENDSWITH
    | ENDSWITH_CS
    | NOT_ENDSWITH_CS
    | MATCHES_REGEX
    | MATCHES
    ;

additiveExpression
    : multiplicativeExpression ((PLUS | MINUS) multiplicativeExpression)*
    ;

multiplicativeExpression
    : unaryExpression ((STAR | SLASH | PERCENT) unaryExpression)*
    ;

unaryExpression
    : MINUS unaryExpression
    | PLUS unaryExpression
    | postfixExpression
    ;

postfixExpression
    : primaryExpression postfixOperator*
    ;

postfixOperator
    : DOT identifier
    | DOT functionCall
    | LBRACKET expression RBRACKET
    | QUESTIONDOT identifier
    ;

primaryExpression
    : literal
    | identifier
    | QUOTED_IDENTIFIER
    | CLIENT_PARAMETER
    | functionCall
    | LPAREN expression RPAREN
    | LPAREN tabularExpression RPAREN
    | caseExpression
    | iffExpression
    | toScalarExpression
    | arrayExpression
    | objectExpression
    | STAR                          // For count(*) etc
    ;

// -----------------------------------------------------------------------------
// FUNCTION CALLS
// -----------------------------------------------------------------------------

functionCall
    : identifier LPAREN argumentList? RPAREN
    | builtinFunction
    ;

builtinFunction
    : TYPEOF LPAREN typeSpecifier RPAREN
    | PACK LPAREN argumentList RPAREN
    | PACK_ALL LPAREN argumentList? RPAREN
    | BAG_PACK LPAREN argumentList RPAREN
    | MAKE_SET LPAREN argumentList RPAREN
    | MAKE_LIST LPAREN argumentList RPAREN
    ;

argumentList
    : argument (COMMA argument)*
    ;

argument
    : expression
    | identifier ASSIGN expression
    | STAR
    ;

// -----------------------------------------------------------------------------
// SPECIAL EXPRESSIONS
// -----------------------------------------------------------------------------

caseExpression
    : CASE LPAREN caseBranch (COMMA caseBranch)* (COMMA expression)? RPAREN
    ;


caseBranch
    : expression COMMA expression
    ;

iffExpression
    : IFF LPAREN expression COMMA expression COMMA expression RPAREN
    | IIF LPAREN expression COMMA expression COMMA expression RPAREN
    ;


toScalarExpression
    : TOSCALAR LPAREN tabularExpression RPAREN
    ;


arrayExpression
    : PACK_ARRAY LPAREN expressionList? RPAREN
    | LBRACKET expressionList? RBRACKET
    ;


objectExpression
    : LBRACE objectPropertyList? RBRACE
    ;

objectPropertyList
    : objectProperty (COMMA objectProperty)*
    ;

objectProperty
    : expression COLON expression
    | identifier COLON expression
    | STRING_LITERAL COLON expression
    ;

// -----------------------------------------------------------------------------
// FUNCTION PARAMETERS (for let statements)
// -----------------------------------------------------------------------------

functionParameters
    : functionParameter (COMMA functionParameter)*
    ;

functionParameter
    : identifier COLON typeSpecifier (ASSIGN expression)?
    ;

// -----------------------------------------------------------------------------
// TYPE SPECIFIERS
// -----------------------------------------------------------------------------

typeSpecifier
    : TYPE_BOOL
    | TYPE_DATETIME
    | TYPE_DECIMAL
    | TYPE_DOUBLE
    | TYPE_DYNAMIC
    | TYPE_GUID
    | TYPE_INT
    | TYPE_LONG
    | TYPE_REAL
    | TYPE_STRING
    | TYPE_TIMESPAN
    | identifier          // For custom types
    ;

// -----------------------------------------------------------------------------
// LITERALS
// -----------------------------------------------------------------------------

literal
    : STRING_LITERAL
    | VERBATIM_STRING
    | MULTILINE_STRING
    | INT_NUMBER
    | LONG_NUMBER
    | REAL_NUMBER
    | DECIMAL_NUMBER
    | HEX_NUMBER
    | booleanLiteral
    | NULL
    | DATETIME_LITERAL
    | TIMESPAN_LITERAL
    | TIMESPAN_SHORT
    | GUID_LITERAL
    | DYNAMIC_LITERAL
    ;

booleanLiteral
    : TRUE
    | FALSE
    ;

// -----------------------------------------------------------------------------
// IDENTIFIER HELPERS
// -----------------------------------------------------------------------------

identifier
    : IDENTIFIER
    // Allow keywords as identifiers in certain contexts
    | WHERE | PROJECT | EXTEND | SUMMARIZE | SORT | TOP | TAKE | JOIN
    | UNION | LOOKUP | COUNT | DISTINCT | SEARCH | PARSE | EVALUATE
    | RENDER | LET | SET | AS | BY | ON | WITH | OF | DEFAULT | STEP
    | RANGE | PRINT | ASC | DESC | INNER | OUTER | LEFT | RIGHT
    | TRUE | FALSE | TYPE_STRING | TYPE_INT | TYPE_LONG | TYPE_BOOL
    | TYPE_DATETIME | TYPE_TIMESPAN | TYPE_DYNAMIC | TYPE_REAL | TYPE_DOUBLE
    | TYPE_DECIMAL | TYPE_GUID | KIND | FROM | TO
    ;

identifierList
    : identifier (COMMA identifier)*
    ;

expressionList
    : expression (COMMA expression)*
    ;
