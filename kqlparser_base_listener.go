// Code generated from KQLParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package kql // KQLParser
import "github.com/antlr4-go/antlr/v4"

// BaseKQLParserListener is a complete listener for a parse tree produced by KQLParser.
type BaseKQLParserListener struct{}

var _ KQLParserListener = &BaseKQLParserListener{}

// VisitTerminal is called when a terminal node is visited.
func (s *BaseKQLParserListener) VisitTerminal(node antlr.TerminalNode) {}

// VisitErrorNode is called when an error node is visited.
func (s *BaseKQLParserListener) VisitErrorNode(node antlr.ErrorNode) {}

// EnterEveryRule is called when any rule is entered.
func (s *BaseKQLParserListener) EnterEveryRule(ctx antlr.ParserRuleContext) {}

// ExitEveryRule is called when any rule is exited.
func (s *BaseKQLParserListener) ExitEveryRule(ctx antlr.ParserRuleContext) {}

// EnterQuery is called when production query is entered.
func (s *BaseKQLParserListener) EnterQuery(ctx *QueryContext) {}

// ExitQuery is called when production query is exited.
func (s *BaseKQLParserListener) ExitQuery(ctx *QueryContext) {}

// EnterStatement is called when production statement is entered.
func (s *BaseKQLParserListener) EnterStatement(ctx *StatementContext) {}

// ExitStatement is called when production statement is exited.
func (s *BaseKQLParserListener) ExitStatement(ctx *StatementContext) {}

// EnterLetStatement is called when production letStatement is entered.
func (s *BaseKQLParserListener) EnterLetStatement(ctx *LetStatementContext) {}

// ExitLetStatement is called when production letStatement is exited.
func (s *BaseKQLParserListener) ExitLetStatement(ctx *LetStatementContext) {}

// EnterSetStatement is called when production setStatement is entered.
func (s *BaseKQLParserListener) EnterSetStatement(ctx *SetStatementContext) {}

// ExitSetStatement is called when production setStatement is exited.
func (s *BaseKQLParserListener) ExitSetStatement(ctx *SetStatementContext) {}

// EnterAliasStatement is called when production aliasStatement is entered.
func (s *BaseKQLParserListener) EnterAliasStatement(ctx *AliasStatementContext) {}

// ExitAliasStatement is called when production aliasStatement is exited.
func (s *BaseKQLParserListener) ExitAliasStatement(ctx *AliasStatementContext) {}

// EnterDeclareStatement is called when production declareStatement is entered.
func (s *BaseKQLParserListener) EnterDeclareStatement(ctx *DeclareStatementContext) {}

// ExitDeclareStatement is called when production declareStatement is exited.
func (s *BaseKQLParserListener) ExitDeclareStatement(ctx *DeclareStatementContext) {}

// EnterPatternStatement is called when production patternStatement is entered.
func (s *BaseKQLParserListener) EnterPatternStatement(ctx *PatternStatementContext) {}

// ExitPatternStatement is called when production patternStatement is exited.
func (s *BaseKQLParserListener) ExitPatternStatement(ctx *PatternStatementContext) {}

// EnterRestrictStatement is called when production restrictStatement is entered.
func (s *BaseKQLParserListener) EnterRestrictStatement(ctx *RestrictStatementContext) {}

// ExitRestrictStatement is called when production restrictStatement is exited.
func (s *BaseKQLParserListener) ExitRestrictStatement(ctx *RestrictStatementContext) {}

// EnterViewExpression is called when production viewExpression is entered.
func (s *BaseKQLParserListener) EnterViewExpression(ctx *ViewExpressionContext) {}

// ExitViewExpression is called when production viewExpression is exited.
func (s *BaseKQLParserListener) ExitViewExpression(ctx *ViewExpressionContext) {}

// EnterPatternDefinition is called when production patternDefinition is entered.
func (s *BaseKQLParserListener) EnterPatternDefinition(ctx *PatternDefinitionContext) {}

// ExitPatternDefinition is called when production patternDefinition is exited.
func (s *BaseKQLParserListener) ExitPatternDefinition(ctx *PatternDefinitionContext) {}

// EnterPatternParam is called when production patternParam is entered.
func (s *BaseKQLParserListener) EnterPatternParam(ctx *PatternParamContext) {}

// ExitPatternParam is called when production patternParam is exited.
func (s *BaseKQLParserListener) ExitPatternParam(ctx *PatternParamContext) {}

// EnterTabularExpression is called when production tabularExpression is entered.
func (s *BaseKQLParserListener) EnterTabularExpression(ctx *TabularExpressionContext) {}

// ExitTabularExpression is called when production tabularExpression is exited.
func (s *BaseKQLParserListener) ExitTabularExpression(ctx *TabularExpressionContext) {}

// EnterTabularSource is called when production tabularSource is entered.
func (s *BaseKQLParserListener) EnterTabularSource(ctx *TabularSourceContext) {}

// ExitTabularSource is called when production tabularSource is exited.
func (s *BaseKQLParserListener) ExitTabularSource(ctx *TabularSourceContext) {}

// EnterTableName is called when production tableName is entered.
func (s *BaseKQLParserListener) EnterTableName(ctx *TableNameContext) {}

// ExitTableName is called when production tableName is exited.
func (s *BaseKQLParserListener) ExitTableName(ctx *TableNameContext) {}

// EnterDatabaseTableName is called when production databaseTableName is entered.
func (s *BaseKQLParserListener) EnterDatabaseTableName(ctx *DatabaseTableNameContext) {}

// ExitDatabaseTableName is called when production databaseTableName is exited.
func (s *BaseKQLParserListener) ExitDatabaseTableName(ctx *DatabaseTableNameContext) {}

// EnterMaterializeExpression is called when production materializeExpression is entered.
func (s *BaseKQLParserListener) EnterMaterializeExpression(ctx *MaterializeExpressionContext) {}

// ExitMaterializeExpression is called when production materializeExpression is exited.
func (s *BaseKQLParserListener) ExitMaterializeExpression(ctx *MaterializeExpressionContext) {}

// EnterTabularOperator is called when production tabularOperator is entered.
func (s *BaseKQLParserListener) EnterTabularOperator(ctx *TabularOperatorContext) {}

// ExitTabularOperator is called when production tabularOperator is exited.
func (s *BaseKQLParserListener) ExitTabularOperator(ctx *TabularOperatorContext) {}

// EnterWhereOperator is called when production whereOperator is entered.
func (s *BaseKQLParserListener) EnterWhereOperator(ctx *WhereOperatorContext) {}

// ExitWhereOperator is called when production whereOperator is exited.
func (s *BaseKQLParserListener) ExitWhereOperator(ctx *WhereOperatorContext) {}

// EnterSearchOperator is called when production searchOperator is entered.
func (s *BaseKQLParserListener) EnterSearchOperator(ctx *SearchOperatorContext) {}

// ExitSearchOperator is called when production searchOperator is exited.
func (s *BaseKQLParserListener) ExitSearchOperator(ctx *SearchOperatorContext) {}

// EnterSearchKind is called when production searchKind is entered.
func (s *BaseKQLParserListener) EnterSearchKind(ctx *SearchKindContext) {}

// ExitSearchKind is called when production searchKind is exited.
func (s *BaseKQLParserListener) ExitSearchKind(ctx *SearchKindContext) {}

// EnterTableList is called when production tableList is entered.
func (s *BaseKQLParserListener) EnterTableList(ctx *TableListContext) {}

// ExitTableList is called when production tableList is exited.
func (s *BaseKQLParserListener) ExitTableList(ctx *TableListContext) {}

// EnterProjectOperator is called when production projectOperator is entered.
func (s *BaseKQLParserListener) EnterProjectOperator(ctx *ProjectOperatorContext) {}

// ExitProjectOperator is called when production projectOperator is exited.
func (s *BaseKQLParserListener) ExitProjectOperator(ctx *ProjectOperatorContext) {}

// EnterProjectAwayOperator is called when production projectAwayOperator is entered.
func (s *BaseKQLParserListener) EnterProjectAwayOperator(ctx *ProjectAwayOperatorContext) {}

// ExitProjectAwayOperator is called when production projectAwayOperator is exited.
func (s *BaseKQLParserListener) ExitProjectAwayOperator(ctx *ProjectAwayOperatorContext) {}

// EnterProjectKeepOperator is called when production projectKeepOperator is entered.
func (s *BaseKQLParserListener) EnterProjectKeepOperator(ctx *ProjectKeepOperatorContext) {}

// ExitProjectKeepOperator is called when production projectKeepOperator is exited.
func (s *BaseKQLParserListener) ExitProjectKeepOperator(ctx *ProjectKeepOperatorContext) {}

// EnterProjectRenameOperator is called when production projectRenameOperator is entered.
func (s *BaseKQLParserListener) EnterProjectRenameOperator(ctx *ProjectRenameOperatorContext) {}

// ExitProjectRenameOperator is called when production projectRenameOperator is exited.
func (s *BaseKQLParserListener) ExitProjectRenameOperator(ctx *ProjectRenameOperatorContext) {}

// EnterProjectReorderOperator is called when production projectReorderOperator is entered.
func (s *BaseKQLParserListener) EnterProjectReorderOperator(ctx *ProjectReorderOperatorContext) {}

// ExitProjectReorderOperator is called when production projectReorderOperator is exited.
func (s *BaseKQLParserListener) ExitProjectReorderOperator(ctx *ProjectReorderOperatorContext) {}

// EnterProjectItemList is called when production projectItemList is entered.
func (s *BaseKQLParserListener) EnterProjectItemList(ctx *ProjectItemListContext) {}

// ExitProjectItemList is called when production projectItemList is exited.
func (s *BaseKQLParserListener) ExitProjectItemList(ctx *ProjectItemListContext) {}

// EnterProjectItem is called when production projectItem is entered.
func (s *BaseKQLParserListener) EnterProjectItem(ctx *ProjectItemContext) {}

// ExitProjectItem is called when production projectItem is exited.
func (s *BaseKQLParserListener) ExitProjectItem(ctx *ProjectItemContext) {}

// EnterIdentifierOrWildcardList is called when production identifierOrWildcardList is entered.
func (s *BaseKQLParserListener) EnterIdentifierOrWildcardList(ctx *IdentifierOrWildcardListContext) {}

// ExitIdentifierOrWildcardList is called when production identifierOrWildcardList is exited.
func (s *BaseKQLParserListener) ExitIdentifierOrWildcardList(ctx *IdentifierOrWildcardListContext) {}

// EnterIdentifierOrWildcard is called when production identifierOrWildcard is entered.
func (s *BaseKQLParserListener) EnterIdentifierOrWildcard(ctx *IdentifierOrWildcardContext) {}

// ExitIdentifierOrWildcard is called when production identifierOrWildcard is exited.
func (s *BaseKQLParserListener) ExitIdentifierOrWildcard(ctx *IdentifierOrWildcardContext) {}

// EnterRenameList is called when production renameList is entered.
func (s *BaseKQLParserListener) EnterRenameList(ctx *RenameListContext) {}

// ExitRenameList is called when production renameList is exited.
func (s *BaseKQLParserListener) ExitRenameList(ctx *RenameListContext) {}

// EnterRenameItem is called when production renameItem is entered.
func (s *BaseKQLParserListener) EnterRenameItem(ctx *RenameItemContext) {}

// ExitRenameItem is called when production renameItem is exited.
func (s *BaseKQLParserListener) ExitRenameItem(ctx *RenameItemContext) {}

// EnterExtendOperator is called when production extendOperator is entered.
func (s *BaseKQLParserListener) EnterExtendOperator(ctx *ExtendOperatorContext) {}

// ExitExtendOperator is called when production extendOperator is exited.
func (s *BaseKQLParserListener) ExitExtendOperator(ctx *ExtendOperatorContext) {}

// EnterExtendItemList is called when production extendItemList is entered.
func (s *BaseKQLParserListener) EnterExtendItemList(ctx *ExtendItemListContext) {}

// ExitExtendItemList is called when production extendItemList is exited.
func (s *BaseKQLParserListener) ExitExtendItemList(ctx *ExtendItemListContext) {}

// EnterExtendItem is called when production extendItem is entered.
func (s *BaseKQLParserListener) EnterExtendItem(ctx *ExtendItemContext) {}

// ExitExtendItem is called when production extendItem is exited.
func (s *BaseKQLParserListener) ExitExtendItem(ctx *ExtendItemContext) {}

// EnterSummarizeOperator is called when production summarizeOperator is entered.
func (s *BaseKQLParserListener) EnterSummarizeOperator(ctx *SummarizeOperatorContext) {}

// ExitSummarizeOperator is called when production summarizeOperator is exited.
func (s *BaseKQLParserListener) ExitSummarizeOperator(ctx *SummarizeOperatorContext) {}

// EnterSummarizeHints is called when production summarizeHints is entered.
func (s *BaseKQLParserListener) EnterSummarizeHints(ctx *SummarizeHintsContext) {}

// ExitSummarizeHints is called when production summarizeHints is exited.
func (s *BaseKQLParserListener) ExitSummarizeHints(ctx *SummarizeHintsContext) {}

// EnterAggregationList is called when production aggregationList is entered.
func (s *BaseKQLParserListener) EnterAggregationList(ctx *AggregationListContext) {}

// ExitAggregationList is called when production aggregationList is exited.
func (s *BaseKQLParserListener) ExitAggregationList(ctx *AggregationListContext) {}

// EnterAggregationItem is called when production aggregationItem is entered.
func (s *BaseKQLParserListener) EnterAggregationItem(ctx *AggregationItemContext) {}

// ExitAggregationItem is called when production aggregationItem is exited.
func (s *BaseKQLParserListener) ExitAggregationItem(ctx *AggregationItemContext) {}

// EnterAggregationFunction is called when production aggregationFunction is entered.
func (s *BaseKQLParserListener) EnterAggregationFunction(ctx *AggregationFunctionContext) {}

// ExitAggregationFunction is called when production aggregationFunction is exited.
func (s *BaseKQLParserListener) ExitAggregationFunction(ctx *AggregationFunctionContext) {}

// EnterGroupByList is called when production groupByList is entered.
func (s *BaseKQLParserListener) EnterGroupByList(ctx *GroupByListContext) {}

// ExitGroupByList is called when production groupByList is exited.
func (s *BaseKQLParserListener) ExitGroupByList(ctx *GroupByListContext) {}

// EnterGroupByItem is called when production groupByItem is entered.
func (s *BaseKQLParserListener) EnterGroupByItem(ctx *GroupByItemContext) {}

// ExitGroupByItem is called when production groupByItem is exited.
func (s *BaseKQLParserListener) ExitGroupByItem(ctx *GroupByItemContext) {}

// EnterSortOperator is called when production sortOperator is entered.
func (s *BaseKQLParserListener) EnterSortOperator(ctx *SortOperatorContext) {}

// ExitSortOperator is called when production sortOperator is exited.
func (s *BaseKQLParserListener) ExitSortOperator(ctx *SortOperatorContext) {}

// EnterSortList is called when production sortList is entered.
func (s *BaseKQLParserListener) EnterSortList(ctx *SortListContext) {}

// ExitSortList is called when production sortList is exited.
func (s *BaseKQLParserListener) ExitSortList(ctx *SortListContext) {}

// EnterSortItem is called when production sortItem is entered.
func (s *BaseKQLParserListener) EnterSortItem(ctx *SortItemContext) {}

// ExitSortItem is called when production sortItem is exited.
func (s *BaseKQLParserListener) ExitSortItem(ctx *SortItemContext) {}

// EnterSortDirection is called when production sortDirection is entered.
func (s *BaseKQLParserListener) EnterSortDirection(ctx *SortDirectionContext) {}

// ExitSortDirection is called when production sortDirection is exited.
func (s *BaseKQLParserListener) ExitSortDirection(ctx *SortDirectionContext) {}

// EnterNullsPosition is called when production nullsPosition is entered.
func (s *BaseKQLParserListener) EnterNullsPosition(ctx *NullsPositionContext) {}

// ExitNullsPosition is called when production nullsPosition is exited.
func (s *BaseKQLParserListener) ExitNullsPosition(ctx *NullsPositionContext) {}

// EnterTopOperator is called when production topOperator is entered.
func (s *BaseKQLParserListener) EnterTopOperator(ctx *TopOperatorContext) {}

// ExitTopOperator is called when production topOperator is exited.
func (s *BaseKQLParserListener) ExitTopOperator(ctx *TopOperatorContext) {}

// EnterTopNestedOperator is called when production topNestedOperator is entered.
func (s *BaseKQLParserListener) EnterTopNestedOperator(ctx *TopNestedOperatorContext) {}

// ExitTopNestedOperator is called when production topNestedOperator is exited.
func (s *BaseKQLParserListener) ExitTopNestedOperator(ctx *TopNestedOperatorContext) {}

// EnterTopNestedClause is called when production topNestedClause is entered.
func (s *BaseKQLParserListener) EnterTopNestedClause(ctx *TopNestedClauseContext) {}

// ExitTopNestedClause is called when production topNestedClause is exited.
func (s *BaseKQLParserListener) ExitTopNestedClause(ctx *TopNestedClauseContext) {}

// EnterTakeOperator is called when production takeOperator is entered.
func (s *BaseKQLParserListener) EnterTakeOperator(ctx *TakeOperatorContext) {}

// ExitTakeOperator is called when production takeOperator is exited.
func (s *BaseKQLParserListener) ExitTakeOperator(ctx *TakeOperatorContext) {}

// EnterDistinctOperator is called when production distinctOperator is entered.
func (s *BaseKQLParserListener) EnterDistinctOperator(ctx *DistinctOperatorContext) {}

// ExitDistinctOperator is called when production distinctOperator is exited.
func (s *BaseKQLParserListener) ExitDistinctOperator(ctx *DistinctOperatorContext) {}

// EnterDistinctColumns is called when production distinctColumns is entered.
func (s *BaseKQLParserListener) EnterDistinctColumns(ctx *DistinctColumnsContext) {}

// ExitDistinctColumns is called when production distinctColumns is exited.
func (s *BaseKQLParserListener) ExitDistinctColumns(ctx *DistinctColumnsContext) {}

// EnterCountOperator is called when production countOperator is entered.
func (s *BaseKQLParserListener) EnterCountOperator(ctx *CountOperatorContext) {}

// ExitCountOperator is called when production countOperator is exited.
func (s *BaseKQLParserListener) ExitCountOperator(ctx *CountOperatorContext) {}

// EnterJoinOperator is called when production joinOperator is entered.
func (s *BaseKQLParserListener) EnterJoinOperator(ctx *JoinOperatorContext) {}

// ExitJoinOperator is called when production joinOperator is exited.
func (s *BaseKQLParserListener) ExitJoinOperator(ctx *JoinOperatorContext) {}

// EnterJoinKind is called when production joinKind is entered.
func (s *BaseKQLParserListener) EnterJoinKind(ctx *JoinKindContext) {}

// ExitJoinKind is called when production joinKind is exited.
func (s *BaseKQLParserListener) ExitJoinKind(ctx *JoinKindContext) {}

// EnterJoinFlavor is called when production joinFlavor is entered.
func (s *BaseKQLParserListener) EnterJoinFlavor(ctx *JoinFlavorContext) {}

// ExitJoinFlavor is called when production joinFlavor is exited.
func (s *BaseKQLParserListener) ExitJoinFlavor(ctx *JoinFlavorContext) {}

// EnterJoinHints is called when production joinHints is entered.
func (s *BaseKQLParserListener) EnterJoinHints(ctx *JoinHintsContext) {}

// ExitJoinHints is called when production joinHints is exited.
func (s *BaseKQLParserListener) ExitJoinHints(ctx *JoinHintsContext) {}

// EnterJoinHint is called when production joinHint is entered.
func (s *BaseKQLParserListener) EnterJoinHint(ctx *JoinHintContext) {}

// ExitJoinHint is called when production joinHint is exited.
func (s *BaseKQLParserListener) ExitJoinHint(ctx *JoinHintContext) {}

// EnterJoinCondition is called when production joinCondition is entered.
func (s *BaseKQLParserListener) EnterJoinCondition(ctx *JoinConditionContext) {}

// ExitJoinCondition is called when production joinCondition is exited.
func (s *BaseKQLParserListener) ExitJoinCondition(ctx *JoinConditionContext) {}

// EnterJoinAttribute is called when production joinAttribute is entered.
func (s *BaseKQLParserListener) EnterJoinAttribute(ctx *JoinAttributeContext) {}

// ExitJoinAttribute is called when production joinAttribute is exited.
func (s *BaseKQLParserListener) ExitJoinAttribute(ctx *JoinAttributeContext) {}

// EnterUnionOperator is called when production unionOperator is entered.
func (s *BaseKQLParserListener) EnterUnionOperator(ctx *UnionOperatorContext) {}

// ExitUnionOperator is called when production unionOperator is exited.
func (s *BaseKQLParserListener) ExitUnionOperator(ctx *UnionOperatorContext) {}

// EnterUnionParameters is called when production unionParameters is entered.
func (s *BaseKQLParserListener) EnterUnionParameters(ctx *UnionParametersContext) {}

// ExitUnionParameters is called when production unionParameters is exited.
func (s *BaseKQLParserListener) ExitUnionParameters(ctx *UnionParametersContext) {}

// EnterUnionParameter is called when production unionParameter is entered.
func (s *BaseKQLParserListener) EnterUnionParameter(ctx *UnionParameterContext) {}

// ExitUnionParameter is called when production unionParameter is exited.
func (s *BaseKQLParserListener) ExitUnionParameter(ctx *UnionParameterContext) {}

// EnterUnionTables is called when production unionTables is entered.
func (s *BaseKQLParserListener) EnterUnionTables(ctx *UnionTablesContext) {}

// ExitUnionTables is called when production unionTables is exited.
func (s *BaseKQLParserListener) ExitUnionTables(ctx *UnionTablesContext) {}

// EnterUnionTable is called when production unionTable is entered.
func (s *BaseKQLParserListener) EnterUnionTable(ctx *UnionTableContext) {}

// ExitUnionTable is called when production unionTable is exited.
func (s *BaseKQLParserListener) ExitUnionTable(ctx *UnionTableContext) {}

// EnterLookupOperator is called when production lookupOperator is entered.
func (s *BaseKQLParserListener) EnterLookupOperator(ctx *LookupOperatorContext) {}

// ExitLookupOperator is called when production lookupOperator is exited.
func (s *BaseKQLParserListener) ExitLookupOperator(ctx *LookupOperatorContext) {}

// EnterLookupKind is called when production lookupKind is entered.
func (s *BaseKQLParserListener) EnterLookupKind(ctx *LookupKindContext) {}

// ExitLookupKind is called when production lookupKind is exited.
func (s *BaseKQLParserListener) ExitLookupKind(ctx *LookupKindContext) {}

// EnterLookupCondition is called when production lookupCondition is entered.
func (s *BaseKQLParserListener) EnterLookupCondition(ctx *LookupConditionContext) {}

// ExitLookupCondition is called when production lookupCondition is exited.
func (s *BaseKQLParserListener) ExitLookupCondition(ctx *LookupConditionContext) {}

// EnterParseOperator is called when production parseOperator is entered.
func (s *BaseKQLParserListener) EnterParseOperator(ctx *ParseOperatorContext) {}

// ExitParseOperator is called when production parseOperator is exited.
func (s *BaseKQLParserListener) ExitParseOperator(ctx *ParseOperatorContext) {}

// EnterParseKind is called when production parseKind is entered.
func (s *BaseKQLParserListener) EnterParseKind(ctx *ParseKindContext) {}

// ExitParseKind is called when production parseKind is exited.
func (s *BaseKQLParserListener) ExitParseKind(ctx *ParseKindContext) {}

// EnterParsePattern is called when production parsePattern is entered.
func (s *BaseKQLParserListener) EnterParsePattern(ctx *ParsePatternContext) {}

// ExitParsePattern is called when production parsePattern is exited.
func (s *BaseKQLParserListener) ExitParsePattern(ctx *ParsePatternContext) {}

// EnterParsePatternItem is called when production parsePatternItem is entered.
func (s *BaseKQLParserListener) EnterParsePatternItem(ctx *ParsePatternItemContext) {}

// ExitParsePatternItem is called when production parsePatternItem is exited.
func (s *BaseKQLParserListener) ExitParsePatternItem(ctx *ParsePatternItemContext) {}

// EnterParseKvOperator is called when production parseKvOperator is entered.
func (s *BaseKQLParserListener) EnterParseKvOperator(ctx *ParseKvOperatorContext) {}

// ExitParseKvOperator is called when production parseKvOperator is exited.
func (s *BaseKQLParserListener) ExitParseKvOperator(ctx *ParseKvOperatorContext) {}

// EnterKvPairList is called when production kvPairList is entered.
func (s *BaseKQLParserListener) EnterKvPairList(ctx *KvPairListContext) {}

// ExitKvPairList is called when production kvPairList is exited.
func (s *BaseKQLParserListener) ExitKvPairList(ctx *KvPairListContext) {}

// EnterKvPair is called when production kvPair is entered.
func (s *BaseKQLParserListener) EnterKvPair(ctx *KvPairContext) {}

// ExitKvPair is called when production kvPair is exited.
func (s *BaseKQLParserListener) ExitKvPair(ctx *KvPairContext) {}

// EnterParseKvParameters is called when production parseKvParameters is entered.
func (s *BaseKQLParserListener) EnterParseKvParameters(ctx *ParseKvParametersContext) {}

// ExitParseKvParameters is called when production parseKvParameters is exited.
func (s *BaseKQLParserListener) ExitParseKvParameters(ctx *ParseKvParametersContext) {}

// EnterParseKvParam is called when production parseKvParam is entered.
func (s *BaseKQLParserListener) EnterParseKvParam(ctx *ParseKvParamContext) {}

// ExitParseKvParam is called when production parseKvParam is exited.
func (s *BaseKQLParserListener) ExitParseKvParam(ctx *ParseKvParamContext) {}

// EnterMvExpandOperator is called when production mvExpandOperator is entered.
func (s *BaseKQLParserListener) EnterMvExpandOperator(ctx *MvExpandOperatorContext) {}

// ExitMvExpandOperator is called when production mvExpandOperator is exited.
func (s *BaseKQLParserListener) ExitMvExpandOperator(ctx *MvExpandOperatorContext) {}

// EnterMvExpandKind is called when production mvExpandKind is entered.
func (s *BaseKQLParserListener) EnterMvExpandKind(ctx *MvExpandKindContext) {}

// ExitMvExpandKind is called when production mvExpandKind is exited.
func (s *BaseKQLParserListener) ExitMvExpandKind(ctx *MvExpandKindContext) {}

// EnterMvExpandParams is called when production mvExpandParams is entered.
func (s *BaseKQLParserListener) EnterMvExpandParams(ctx *MvExpandParamsContext) {}

// ExitMvExpandParams is called when production mvExpandParams is exited.
func (s *BaseKQLParserListener) ExitMvExpandParams(ctx *MvExpandParamsContext) {}

// EnterMvExpandItemList is called when production mvExpandItemList is entered.
func (s *BaseKQLParserListener) EnterMvExpandItemList(ctx *MvExpandItemListContext) {}

// ExitMvExpandItemList is called when production mvExpandItemList is exited.
func (s *BaseKQLParserListener) ExitMvExpandItemList(ctx *MvExpandItemListContext) {}

// EnterMvExpandItem is called when production mvExpandItem is entered.
func (s *BaseKQLParserListener) EnterMvExpandItem(ctx *MvExpandItemContext) {}

// ExitMvExpandItem is called when production mvExpandItem is exited.
func (s *BaseKQLParserListener) ExitMvExpandItem(ctx *MvExpandItemContext) {}

// EnterLimitClause is called when production limitClause is entered.
func (s *BaseKQLParserListener) EnterLimitClause(ctx *LimitClauseContext) {}

// ExitLimitClause is called when production limitClause is exited.
func (s *BaseKQLParserListener) ExitLimitClause(ctx *LimitClauseContext) {}

// EnterMvApplyOperator is called when production mvApplyOperator is entered.
func (s *BaseKQLParserListener) EnterMvApplyOperator(ctx *MvApplyOperatorContext) {}

// ExitMvApplyOperator is called when production mvApplyOperator is exited.
func (s *BaseKQLParserListener) ExitMvApplyOperator(ctx *MvApplyOperatorContext) {}

// EnterMvApplyItemList is called when production mvApplyItemList is entered.
func (s *BaseKQLParserListener) EnterMvApplyItemList(ctx *MvApplyItemListContext) {}

// ExitMvApplyItemList is called when production mvApplyItemList is exited.
func (s *BaseKQLParserListener) ExitMvApplyItemList(ctx *MvApplyItemListContext) {}

// EnterMvApplyItem is called when production mvApplyItem is entered.
func (s *BaseKQLParserListener) EnterMvApplyItem(ctx *MvApplyItemContext) {}

// ExitMvApplyItem is called when production mvApplyItem is exited.
func (s *BaseKQLParserListener) ExitMvApplyItem(ctx *MvApplyItemContext) {}

// EnterMvApplyOnClause is called when production mvApplyOnClause is entered.
func (s *BaseKQLParserListener) EnterMvApplyOnClause(ctx *MvApplyOnClauseContext) {}

// ExitMvApplyOnClause is called when production mvApplyOnClause is exited.
func (s *BaseKQLParserListener) ExitMvApplyOnClause(ctx *MvApplyOnClauseContext) {}

// EnterEvaluateOperator is called when production evaluateOperator is entered.
func (s *BaseKQLParserListener) EnterEvaluateOperator(ctx *EvaluateOperatorContext) {}

// ExitEvaluateOperator is called when production evaluateOperator is exited.
func (s *BaseKQLParserListener) ExitEvaluateOperator(ctx *EvaluateOperatorContext) {}

// EnterEvaluateHints is called when production evaluateHints is entered.
func (s *BaseKQLParserListener) EnterEvaluateHints(ctx *EvaluateHintsContext) {}

// ExitEvaluateHints is called when production evaluateHints is exited.
func (s *BaseKQLParserListener) ExitEvaluateHints(ctx *EvaluateHintsContext) {}

// EnterFacetOperator is called when production facetOperator is entered.
func (s *BaseKQLParserListener) EnterFacetOperator(ctx *FacetOperatorContext) {}

// ExitFacetOperator is called when production facetOperator is exited.
func (s *BaseKQLParserListener) ExitFacetOperator(ctx *FacetOperatorContext) {}

// EnterForkOperator is called when production forkOperator is entered.
func (s *BaseKQLParserListener) EnterForkOperator(ctx *ForkOperatorContext) {}

// ExitForkOperator is called when production forkOperator is exited.
func (s *BaseKQLParserListener) ExitForkOperator(ctx *ForkOperatorContext) {}

// EnterForkBranch is called when production forkBranch is entered.
func (s *BaseKQLParserListener) EnterForkBranch(ctx *ForkBranchContext) {}

// ExitForkBranch is called when production forkBranch is exited.
func (s *BaseKQLParserListener) ExitForkBranch(ctx *ForkBranchContext) {}

// EnterPartitionOperator is called when production partitionOperator is entered.
func (s *BaseKQLParserListener) EnterPartitionOperator(ctx *PartitionOperatorContext) {}

// ExitPartitionOperator is called when production partitionOperator is exited.
func (s *BaseKQLParserListener) ExitPartitionOperator(ctx *PartitionOperatorContext) {}

// EnterPartitionHints is called when production partitionHints is entered.
func (s *BaseKQLParserListener) EnterPartitionHints(ctx *PartitionHintsContext) {}

// ExitPartitionHints is called when production partitionHints is exited.
func (s *BaseKQLParserListener) ExitPartitionHints(ctx *PartitionHintsContext) {}

// EnterScanOperator is called when production scanOperator is entered.
func (s *BaseKQLParserListener) EnterScanOperator(ctx *ScanOperatorContext) {}

// ExitScanOperator is called when production scanOperator is exited.
func (s *BaseKQLParserListener) ExitScanOperator(ctx *ScanOperatorContext) {}

// EnterScanParams is called when production scanParams is entered.
func (s *BaseKQLParserListener) EnterScanParams(ctx *ScanParamsContext) {}

// ExitScanParams is called when production scanParams is exited.
func (s *BaseKQLParserListener) ExitScanParams(ctx *ScanParamsContext) {}

// EnterScanDeclare is called when production scanDeclare is entered.
func (s *BaseKQLParserListener) EnterScanDeclare(ctx *ScanDeclareContext) {}

// ExitScanDeclare is called when production scanDeclare is exited.
func (s *BaseKQLParserListener) ExitScanDeclare(ctx *ScanDeclareContext) {}

// EnterScanDeclareItem is called when production scanDeclareItem is entered.
func (s *BaseKQLParserListener) EnterScanDeclareItem(ctx *ScanDeclareItemContext) {}

// ExitScanDeclareItem is called when production scanDeclareItem is exited.
func (s *BaseKQLParserListener) ExitScanDeclareItem(ctx *ScanDeclareItemContext) {}

// EnterScanStepList is called when production scanStepList is entered.
func (s *BaseKQLParserListener) EnterScanStepList(ctx *ScanStepListContext) {}

// ExitScanStepList is called when production scanStepList is exited.
func (s *BaseKQLParserListener) ExitScanStepList(ctx *ScanStepListContext) {}

// EnterScanStep is called when production scanStep is entered.
func (s *BaseKQLParserListener) EnterScanStep(ctx *ScanStepContext) {}

// ExitScanStep is called when production scanStep is exited.
func (s *BaseKQLParserListener) ExitScanStep(ctx *ScanStepContext) {}

// EnterScanAction is called when production scanAction is entered.
func (s *BaseKQLParserListener) EnterScanAction(ctx *ScanActionContext) {}

// ExitScanAction is called when production scanAction is exited.
func (s *BaseKQLParserListener) ExitScanAction(ctx *ScanActionContext) {}

// EnterSerializeOperator is called when production serializeOperator is entered.
func (s *BaseKQLParserListener) EnterSerializeOperator(ctx *SerializeOperatorContext) {}

// ExitSerializeOperator is called when production serializeOperator is exited.
func (s *BaseKQLParserListener) ExitSerializeOperator(ctx *SerializeOperatorContext) {}

// EnterSampleOperator is called when production sampleOperator is entered.
func (s *BaseKQLParserListener) EnterSampleOperator(ctx *SampleOperatorContext) {}

// ExitSampleOperator is called when production sampleOperator is exited.
func (s *BaseKQLParserListener) ExitSampleOperator(ctx *SampleOperatorContext) {}

// EnterSampleDistinctOperator is called when production sampleDistinctOperator is entered.
func (s *BaseKQLParserListener) EnterSampleDistinctOperator(ctx *SampleDistinctOperatorContext) {}

// ExitSampleDistinctOperator is called when production sampleDistinctOperator is exited.
func (s *BaseKQLParserListener) ExitSampleDistinctOperator(ctx *SampleDistinctOperatorContext) {}

// EnterMakeSeriesOperator is called when production makeSeriesOperator is entered.
func (s *BaseKQLParserListener) EnterMakeSeriesOperator(ctx *MakeSeriesOperatorContext) {}

// ExitMakeSeriesOperator is called when production makeSeriesOperator is exited.
func (s *BaseKQLParserListener) ExitMakeSeriesOperator(ctx *MakeSeriesOperatorContext) {}

// EnterMakeSeriesItemList is called when production makeSeriesItemList is entered.
func (s *BaseKQLParserListener) EnterMakeSeriesItemList(ctx *MakeSeriesItemListContext) {}

// ExitMakeSeriesItemList is called when production makeSeriesItemList is exited.
func (s *BaseKQLParserListener) ExitMakeSeriesItemList(ctx *MakeSeriesItemListContext) {}

// EnterMakeSeriesItem is called when production makeSeriesItem is entered.
func (s *BaseKQLParserListener) EnterMakeSeriesItem(ctx *MakeSeriesItemContext) {}

// ExitMakeSeriesItem is called when production makeSeriesItem is exited.
func (s *BaseKQLParserListener) ExitMakeSeriesItem(ctx *MakeSeriesItemContext) {}

// EnterMakeSeriesOnClause is called when production makeSeriesOnClause is entered.
func (s *BaseKQLParserListener) EnterMakeSeriesOnClause(ctx *MakeSeriesOnClauseContext) {}

// ExitMakeSeriesOnClause is called when production makeSeriesOnClause is exited.
func (s *BaseKQLParserListener) ExitMakeSeriesOnClause(ctx *MakeSeriesOnClauseContext) {}

// EnterMakeSeriesParams is called when production makeSeriesParams is entered.
func (s *BaseKQLParserListener) EnterMakeSeriesParams(ctx *MakeSeriesParamsContext) {}

// ExitMakeSeriesParams is called when production makeSeriesParams is exited.
func (s *BaseKQLParserListener) ExitMakeSeriesParams(ctx *MakeSeriesParamsContext) {}

// EnterFindOperator is called when production findOperator is entered.
func (s *BaseKQLParserListener) EnterFindOperator(ctx *FindOperatorContext) {}

// ExitFindOperator is called when production findOperator is exited.
func (s *BaseKQLParserListener) ExitFindOperator(ctx *FindOperatorContext) {}

// EnterFindParams is called when production findParams is entered.
func (s *BaseKQLParserListener) EnterFindParams(ctx *FindParamsContext) {}

// ExitFindParams is called when production findParams is exited.
func (s *BaseKQLParserListener) ExitFindParams(ctx *FindParamsContext) {}

// EnterGetschemaOperator is called when production getschemaOperator is entered.
func (s *BaseKQLParserListener) EnterGetschemaOperator(ctx *GetschemaOperatorContext) {}

// ExitGetschemaOperator is called when production getschemaOperator is exited.
func (s *BaseKQLParserListener) ExitGetschemaOperator(ctx *GetschemaOperatorContext) {}

// EnterRenderOperator is called when production renderOperator is entered.
func (s *BaseKQLParserListener) EnterRenderOperator(ctx *RenderOperatorContext) {}

// ExitRenderOperator is called when production renderOperator is exited.
func (s *BaseKQLParserListener) ExitRenderOperator(ctx *RenderOperatorContext) {}

// EnterRenderProperties is called when production renderProperties is entered.
func (s *BaseKQLParserListener) EnterRenderProperties(ctx *RenderPropertiesContext) {}

// ExitRenderProperties is called when production renderProperties is exited.
func (s *BaseKQLParserListener) ExitRenderProperties(ctx *RenderPropertiesContext) {}

// EnterRenderProperty is called when production renderProperty is entered.
func (s *BaseKQLParserListener) EnterRenderProperty(ctx *RenderPropertyContext) {}

// ExitRenderProperty is called when production renderProperty is exited.
func (s *BaseKQLParserListener) ExitRenderProperty(ctx *RenderPropertyContext) {}

// EnterConsumeOperator is called when production consumeOperator is entered.
func (s *BaseKQLParserListener) EnterConsumeOperator(ctx *ConsumeOperatorContext) {}

// ExitConsumeOperator is called when production consumeOperator is exited.
func (s *BaseKQLParserListener) ExitConsumeOperator(ctx *ConsumeOperatorContext) {}

// EnterInvokeOperator is called when production invokeOperator is entered.
func (s *BaseKQLParserListener) EnterInvokeOperator(ctx *InvokeOperatorContext) {}

// ExitInvokeOperator is called when production invokeOperator is exited.
func (s *BaseKQLParserListener) ExitInvokeOperator(ctx *InvokeOperatorContext) {}

// EnterAsOperator is called when production asOperator is entered.
func (s *BaseKQLParserListener) EnterAsOperator(ctx *AsOperatorContext) {}

// ExitAsOperator is called when production asOperator is exited.
func (s *BaseKQLParserListener) ExitAsOperator(ctx *AsOperatorContext) {}

// EnterGraphOperator is called when production graphOperator is entered.
func (s *BaseKQLParserListener) EnterGraphOperator(ctx *GraphOperatorContext) {}

// ExitGraphOperator is called when production graphOperator is exited.
func (s *BaseKQLParserListener) ExitGraphOperator(ctx *GraphOperatorContext) {}

// EnterMakeGraphOperator is called when production makeGraphOperator is entered.
func (s *BaseKQLParserListener) EnterMakeGraphOperator(ctx *MakeGraphOperatorContext) {}

// ExitMakeGraphOperator is called when production makeGraphOperator is exited.
func (s *BaseKQLParserListener) ExitMakeGraphOperator(ctx *MakeGraphOperatorContext) {}

// EnterGraphMatchOperator is called when production graphMatchOperator is entered.
func (s *BaseKQLParserListener) EnterGraphMatchOperator(ctx *GraphMatchOperatorContext) {}

// ExitGraphMatchOperator is called when production graphMatchOperator is exited.
func (s *BaseKQLParserListener) ExitGraphMatchOperator(ctx *GraphMatchOperatorContext) {}

// EnterGraphPattern is called when production graphPattern is entered.
func (s *BaseKQLParserListener) EnterGraphPattern(ctx *GraphPatternContext) {}

// ExitGraphPattern is called when production graphPattern is exited.
func (s *BaseKQLParserListener) ExitGraphPattern(ctx *GraphPatternContext) {}

// EnterGraphPatternElement is called when production graphPatternElement is entered.
func (s *BaseKQLParserListener) EnterGraphPatternElement(ctx *GraphPatternElementContext) {}

// ExitGraphPatternElement is called when production graphPatternElement is exited.
func (s *BaseKQLParserListener) ExitGraphPatternElement(ctx *GraphPatternElementContext) {}

// EnterGraphEdge is called when production graphEdge is entered.
func (s *BaseKQLParserListener) EnterGraphEdge(ctx *GraphEdgeContext) {}

// ExitGraphEdge is called when production graphEdge is exited.
func (s *BaseKQLParserListener) ExitGraphEdge(ctx *GraphEdgeContext) {}

// EnterGraphShortestPathsOperator is called when production graphShortestPathsOperator is entered.
func (s *BaseKQLParserListener) EnterGraphShortestPathsOperator(ctx *GraphShortestPathsOperatorContext) {
}

// ExitGraphShortestPathsOperator is called when production graphShortestPathsOperator is exited.
func (s *BaseKQLParserListener) ExitGraphShortestPathsOperator(ctx *GraphShortestPathsOperatorContext) {
}

// EnterGraphToTableOperator is called when production graphToTableOperator is entered.
func (s *BaseKQLParserListener) EnterGraphToTableOperator(ctx *GraphToTableOperatorContext) {}

// ExitGraphToTableOperator is called when production graphToTableOperator is exited.
func (s *BaseKQLParserListener) ExitGraphToTableOperator(ctx *GraphToTableOperatorContext) {}

// EnterGraphToTableParams is called when production graphToTableParams is entered.
func (s *BaseKQLParserListener) EnterGraphToTableParams(ctx *GraphToTableParamsContext) {}

// ExitGraphToTableParams is called when production graphToTableParams is exited.
func (s *BaseKQLParserListener) ExitGraphToTableParams(ctx *GraphToTableParamsContext) {}

// EnterDatatable is called when production datatable is entered.
func (s *BaseKQLParserListener) EnterDatatable(ctx *DatatableContext) {}

// ExitDatatable is called when production datatable is exited.
func (s *BaseKQLParserListener) ExitDatatable(ctx *DatatableContext) {}

// EnterDatatableSchema is called when production datatableSchema is entered.
func (s *BaseKQLParserListener) EnterDatatableSchema(ctx *DatatableSchemaContext) {}

// ExitDatatableSchema is called when production datatableSchema is exited.
func (s *BaseKQLParserListener) ExitDatatableSchema(ctx *DatatableSchemaContext) {}

// EnterDatatableColumn is called when production datatableColumn is entered.
func (s *BaseKQLParserListener) EnterDatatableColumn(ctx *DatatableColumnContext) {}

// ExitDatatableColumn is called when production datatableColumn is exited.
func (s *BaseKQLParserListener) ExitDatatableColumn(ctx *DatatableColumnContext) {}

// EnterDatatableRows is called when production datatableRows is entered.
func (s *BaseKQLParserListener) EnterDatatableRows(ctx *DatatableRowsContext) {}

// ExitDatatableRows is called when production datatableRows is exited.
func (s *BaseKQLParserListener) ExitDatatableRows(ctx *DatatableRowsContext) {}

// EnterExternalData is called when production externalData is entered.
func (s *BaseKQLParserListener) EnterExternalData(ctx *ExternalDataContext) {}

// ExitExternalData is called when production externalData is exited.
func (s *BaseKQLParserListener) ExitExternalData(ctx *ExternalDataContext) {}

// EnterExternalDataUri is called when production externalDataUri is entered.
func (s *BaseKQLParserListener) EnterExternalDataUri(ctx *ExternalDataUriContext) {}

// ExitExternalDataUri is called when production externalDataUri is exited.
func (s *BaseKQLParserListener) ExitExternalDataUri(ctx *ExternalDataUriContext) {}

// EnterExternalDataOptions is called when production externalDataOptions is entered.
func (s *BaseKQLParserListener) EnterExternalDataOptions(ctx *ExternalDataOptionsContext) {}

// ExitExternalDataOptions is called when production externalDataOptions is exited.
func (s *BaseKQLParserListener) ExitExternalDataOptions(ctx *ExternalDataOptionsContext) {}

// EnterExternalDataOption is called when production externalDataOption is entered.
func (s *BaseKQLParserListener) EnterExternalDataOption(ctx *ExternalDataOptionContext) {}

// ExitExternalDataOption is called when production externalDataOption is exited.
func (s *BaseKQLParserListener) ExitExternalDataOption(ctx *ExternalDataOptionContext) {}

// EnterPrintArgList is called when production printArgList is entered.
func (s *BaseKQLParserListener) EnterPrintArgList(ctx *PrintArgListContext) {}

// ExitPrintArgList is called when production printArgList is exited.
func (s *BaseKQLParserListener) ExitPrintArgList(ctx *PrintArgListContext) {}

// EnterPrintArg is called when production printArg is entered.
func (s *BaseKQLParserListener) EnterPrintArg(ctx *PrintArgContext) {}

// ExitPrintArg is called when production printArg is exited.
func (s *BaseKQLParserListener) ExitPrintArg(ctx *PrintArgContext) {}

// EnterExpression is called when production expression is entered.
func (s *BaseKQLParserListener) EnterExpression(ctx *ExpressionContext) {}

// ExitExpression is called when production expression is exited.
func (s *BaseKQLParserListener) ExitExpression(ctx *ExpressionContext) {}

// EnterOrExpression is called when production orExpression is entered.
func (s *BaseKQLParserListener) EnterOrExpression(ctx *OrExpressionContext) {}

// ExitOrExpression is called when production orExpression is exited.
func (s *BaseKQLParserListener) ExitOrExpression(ctx *OrExpressionContext) {}

// EnterAndExpression is called when production andExpression is entered.
func (s *BaseKQLParserListener) EnterAndExpression(ctx *AndExpressionContext) {}

// ExitAndExpression is called when production andExpression is exited.
func (s *BaseKQLParserListener) ExitAndExpression(ctx *AndExpressionContext) {}

// EnterNotExpression is called when production notExpression is entered.
func (s *BaseKQLParserListener) EnterNotExpression(ctx *NotExpressionContext) {}

// ExitNotExpression is called when production notExpression is exited.
func (s *BaseKQLParserListener) ExitNotExpression(ctx *NotExpressionContext) {}

// EnterComparisonExpression is called when production comparisonExpression is entered.
func (s *BaseKQLParserListener) EnterComparisonExpression(ctx *ComparisonExpressionContext) {}

// ExitComparisonExpression is called when production comparisonExpression is exited.
func (s *BaseKQLParserListener) ExitComparisonExpression(ctx *ComparisonExpressionContext) {}

// EnterComparisonOperator is called when production comparisonOperator is entered.
func (s *BaseKQLParserListener) EnterComparisonOperator(ctx *ComparisonOperatorContext) {}

// ExitComparisonOperator is called when production comparisonOperator is exited.
func (s *BaseKQLParserListener) ExitComparisonOperator(ctx *ComparisonOperatorContext) {}

// EnterStringOperator is called when production stringOperator is entered.
func (s *BaseKQLParserListener) EnterStringOperator(ctx *StringOperatorContext) {}

// ExitStringOperator is called when production stringOperator is exited.
func (s *BaseKQLParserListener) ExitStringOperator(ctx *StringOperatorContext) {}

// EnterAdditiveExpression is called when production additiveExpression is entered.
func (s *BaseKQLParserListener) EnterAdditiveExpression(ctx *AdditiveExpressionContext) {}

// ExitAdditiveExpression is called when production additiveExpression is exited.
func (s *BaseKQLParserListener) ExitAdditiveExpression(ctx *AdditiveExpressionContext) {}

// EnterMultiplicativeExpression is called when production multiplicativeExpression is entered.
func (s *BaseKQLParserListener) EnterMultiplicativeExpression(ctx *MultiplicativeExpressionContext) {}

// ExitMultiplicativeExpression is called when production multiplicativeExpression is exited.
func (s *BaseKQLParserListener) ExitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) {}

// EnterUnaryExpression is called when production unaryExpression is entered.
func (s *BaseKQLParserListener) EnterUnaryExpression(ctx *UnaryExpressionContext) {}

// ExitUnaryExpression is called when production unaryExpression is exited.
func (s *BaseKQLParserListener) ExitUnaryExpression(ctx *UnaryExpressionContext) {}

// EnterPostfixExpression is called when production postfixExpression is entered.
func (s *BaseKQLParserListener) EnterPostfixExpression(ctx *PostfixExpressionContext) {}

// ExitPostfixExpression is called when production postfixExpression is exited.
func (s *BaseKQLParserListener) ExitPostfixExpression(ctx *PostfixExpressionContext) {}

// EnterPostfixOperator is called when production postfixOperator is entered.
func (s *BaseKQLParserListener) EnterPostfixOperator(ctx *PostfixOperatorContext) {}

// ExitPostfixOperator is called when production postfixOperator is exited.
func (s *BaseKQLParserListener) ExitPostfixOperator(ctx *PostfixOperatorContext) {}

// EnterPrimaryExpression is called when production primaryExpression is entered.
func (s *BaseKQLParserListener) EnterPrimaryExpression(ctx *PrimaryExpressionContext) {}

// ExitPrimaryExpression is called when production primaryExpression is exited.
func (s *BaseKQLParserListener) ExitPrimaryExpression(ctx *PrimaryExpressionContext) {}

// EnterFunctionCall is called when production functionCall is entered.
func (s *BaseKQLParserListener) EnterFunctionCall(ctx *FunctionCallContext) {}

// ExitFunctionCall is called when production functionCall is exited.
func (s *BaseKQLParserListener) ExitFunctionCall(ctx *FunctionCallContext) {}

// EnterBuiltinFunction is called when production builtinFunction is entered.
func (s *BaseKQLParserListener) EnterBuiltinFunction(ctx *BuiltinFunctionContext) {}

// ExitBuiltinFunction is called when production builtinFunction is exited.
func (s *BaseKQLParserListener) ExitBuiltinFunction(ctx *BuiltinFunctionContext) {}

// EnterArgumentList is called when production argumentList is entered.
func (s *BaseKQLParserListener) EnterArgumentList(ctx *ArgumentListContext) {}

// ExitArgumentList is called when production argumentList is exited.
func (s *BaseKQLParserListener) ExitArgumentList(ctx *ArgumentListContext) {}

// EnterArgument is called when production argument is entered.
func (s *BaseKQLParserListener) EnterArgument(ctx *ArgumentContext) {}

// ExitArgument is called when production argument is exited.
func (s *BaseKQLParserListener) ExitArgument(ctx *ArgumentContext) {}

// EnterCaseExpression is called when production caseExpression is entered.
func (s *BaseKQLParserListener) EnterCaseExpression(ctx *CaseExpressionContext) {}

// ExitCaseExpression is called when production caseExpression is exited.
func (s *BaseKQLParserListener) ExitCaseExpression(ctx *CaseExpressionContext) {}

// EnterCaseBranch is called when production caseBranch is entered.
func (s *BaseKQLParserListener) EnterCaseBranch(ctx *CaseBranchContext) {}

// ExitCaseBranch is called when production caseBranch is exited.
func (s *BaseKQLParserListener) ExitCaseBranch(ctx *CaseBranchContext) {}

// EnterIffExpression is called when production iffExpression is entered.
func (s *BaseKQLParserListener) EnterIffExpression(ctx *IffExpressionContext) {}

// ExitIffExpression is called when production iffExpression is exited.
func (s *BaseKQLParserListener) ExitIffExpression(ctx *IffExpressionContext) {}

// EnterToScalarExpression is called when production toScalarExpression is entered.
func (s *BaseKQLParserListener) EnterToScalarExpression(ctx *ToScalarExpressionContext) {}

// ExitToScalarExpression is called when production toScalarExpression is exited.
func (s *BaseKQLParserListener) ExitToScalarExpression(ctx *ToScalarExpressionContext) {}

// EnterArrayExpression is called when production arrayExpression is entered.
func (s *BaseKQLParserListener) EnterArrayExpression(ctx *ArrayExpressionContext) {}

// ExitArrayExpression is called when production arrayExpression is exited.
func (s *BaseKQLParserListener) ExitArrayExpression(ctx *ArrayExpressionContext) {}

// EnterObjectExpression is called when production objectExpression is entered.
func (s *BaseKQLParserListener) EnterObjectExpression(ctx *ObjectExpressionContext) {}

// ExitObjectExpression is called when production objectExpression is exited.
func (s *BaseKQLParserListener) ExitObjectExpression(ctx *ObjectExpressionContext) {}

// EnterObjectPropertyList is called when production objectPropertyList is entered.
func (s *BaseKQLParserListener) EnterObjectPropertyList(ctx *ObjectPropertyListContext) {}

// ExitObjectPropertyList is called when production objectPropertyList is exited.
func (s *BaseKQLParserListener) ExitObjectPropertyList(ctx *ObjectPropertyListContext) {}

// EnterObjectProperty is called when production objectProperty is entered.
func (s *BaseKQLParserListener) EnterObjectProperty(ctx *ObjectPropertyContext) {}

// ExitObjectProperty is called when production objectProperty is exited.
func (s *BaseKQLParserListener) ExitObjectProperty(ctx *ObjectPropertyContext) {}

// EnterFunctionParameters is called when production functionParameters is entered.
func (s *BaseKQLParserListener) EnterFunctionParameters(ctx *FunctionParametersContext) {}

// ExitFunctionParameters is called when production functionParameters is exited.
func (s *BaseKQLParserListener) ExitFunctionParameters(ctx *FunctionParametersContext) {}

// EnterFunctionParameter is called when production functionParameter is entered.
func (s *BaseKQLParserListener) EnterFunctionParameter(ctx *FunctionParameterContext) {}

// ExitFunctionParameter is called when production functionParameter is exited.
func (s *BaseKQLParserListener) ExitFunctionParameter(ctx *FunctionParameterContext) {}

// EnterTypeSpecifier is called when production typeSpecifier is entered.
func (s *BaseKQLParserListener) EnterTypeSpecifier(ctx *TypeSpecifierContext) {}

// ExitTypeSpecifier is called when production typeSpecifier is exited.
func (s *BaseKQLParserListener) ExitTypeSpecifier(ctx *TypeSpecifierContext) {}

// EnterLiteral is called when production literal is entered.
func (s *BaseKQLParserListener) EnterLiteral(ctx *LiteralContext) {}

// ExitLiteral is called when production literal is exited.
func (s *BaseKQLParserListener) ExitLiteral(ctx *LiteralContext) {}

// EnterBooleanLiteral is called when production booleanLiteral is entered.
func (s *BaseKQLParserListener) EnterBooleanLiteral(ctx *BooleanLiteralContext) {}

// ExitBooleanLiteral is called when production booleanLiteral is exited.
func (s *BaseKQLParserListener) ExitBooleanLiteral(ctx *BooleanLiteralContext) {}

// EnterIdentifier is called when production identifier is entered.
func (s *BaseKQLParserListener) EnterIdentifier(ctx *IdentifierContext) {}

// ExitIdentifier is called when production identifier is exited.
func (s *BaseKQLParserListener) ExitIdentifier(ctx *IdentifierContext) {}

// EnterIdentifierList is called when production identifierList is entered.
func (s *BaseKQLParserListener) EnterIdentifierList(ctx *IdentifierListContext) {}

// ExitIdentifierList is called when production identifierList is exited.
func (s *BaseKQLParserListener) ExitIdentifierList(ctx *IdentifierListContext) {}

// EnterExpressionList is called when production expressionList is entered.
func (s *BaseKQLParserListener) EnterExpressionList(ctx *ExpressionListContext) {}

// ExitExpressionList is called when production expressionList is exited.
func (s *BaseKQLParserListener) ExitExpressionList(ctx *ExpressionListContext) {}
