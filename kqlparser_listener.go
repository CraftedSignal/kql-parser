// Code generated from KQLParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package kql // KQLParser
import "github.com/antlr4-go/antlr/v4"

// KQLParserListener is a complete listener for a parse tree produced by KQLParser.
type KQLParserListener interface {
	antlr.ParseTreeListener

	// EnterQuery is called when entering the query production.
	EnterQuery(c *QueryContext)

	// EnterStatement is called when entering the statement production.
	EnterStatement(c *StatementContext)

	// EnterLetStatement is called when entering the letStatement production.
	EnterLetStatement(c *LetStatementContext)

	// EnterSetStatement is called when entering the setStatement production.
	EnterSetStatement(c *SetStatementContext)

	// EnterAliasStatement is called when entering the aliasStatement production.
	EnterAliasStatement(c *AliasStatementContext)

	// EnterDeclareStatement is called when entering the declareStatement production.
	EnterDeclareStatement(c *DeclareStatementContext)

	// EnterPatternStatement is called when entering the patternStatement production.
	EnterPatternStatement(c *PatternStatementContext)

	// EnterRestrictStatement is called when entering the restrictStatement production.
	EnterRestrictStatement(c *RestrictStatementContext)

	// EnterViewExpression is called when entering the viewExpression production.
	EnterViewExpression(c *ViewExpressionContext)

	// EnterPatternDefinition is called when entering the patternDefinition production.
	EnterPatternDefinition(c *PatternDefinitionContext)

	// EnterPatternParam is called when entering the patternParam production.
	EnterPatternParam(c *PatternParamContext)

	// EnterTabularExpression is called when entering the tabularExpression production.
	EnterTabularExpression(c *TabularExpressionContext)

	// EnterTabularSource is called when entering the tabularSource production.
	EnterTabularSource(c *TabularSourceContext)

	// EnterTableName is called when entering the tableName production.
	EnterTableName(c *TableNameContext)

	// EnterDatabaseTableName is called when entering the databaseTableName production.
	EnterDatabaseTableName(c *DatabaseTableNameContext)

	// EnterMaterializeExpression is called when entering the materializeExpression production.
	EnterMaterializeExpression(c *MaterializeExpressionContext)

	// EnterTabularOperator is called when entering the tabularOperator production.
	EnterTabularOperator(c *TabularOperatorContext)

	// EnterWhereOperator is called when entering the whereOperator production.
	EnterWhereOperator(c *WhereOperatorContext)

	// EnterSearchOperator is called when entering the searchOperator production.
	EnterSearchOperator(c *SearchOperatorContext)

	// EnterSearchKind is called when entering the searchKind production.
	EnterSearchKind(c *SearchKindContext)

	// EnterTableList is called when entering the tableList production.
	EnterTableList(c *TableListContext)

	// EnterProjectOperator is called when entering the projectOperator production.
	EnterProjectOperator(c *ProjectOperatorContext)

	// EnterProjectAwayOperator is called when entering the projectAwayOperator production.
	EnterProjectAwayOperator(c *ProjectAwayOperatorContext)

	// EnterProjectKeepOperator is called when entering the projectKeepOperator production.
	EnterProjectKeepOperator(c *ProjectKeepOperatorContext)

	// EnterProjectRenameOperator is called when entering the projectRenameOperator production.
	EnterProjectRenameOperator(c *ProjectRenameOperatorContext)

	// EnterProjectReorderOperator is called when entering the projectReorderOperator production.
	EnterProjectReorderOperator(c *ProjectReorderOperatorContext)

	// EnterProjectItemList is called when entering the projectItemList production.
	EnterProjectItemList(c *ProjectItemListContext)

	// EnterProjectItem is called when entering the projectItem production.
	EnterProjectItem(c *ProjectItemContext)

	// EnterIdentifierOrWildcardList is called when entering the identifierOrWildcardList production.
	EnterIdentifierOrWildcardList(c *IdentifierOrWildcardListContext)

	// EnterIdentifierOrWildcard is called when entering the identifierOrWildcard production.
	EnterIdentifierOrWildcard(c *IdentifierOrWildcardContext)

	// EnterRenameList is called when entering the renameList production.
	EnterRenameList(c *RenameListContext)

	// EnterRenameItem is called when entering the renameItem production.
	EnterRenameItem(c *RenameItemContext)

	// EnterExtendOperator is called when entering the extendOperator production.
	EnterExtendOperator(c *ExtendOperatorContext)

	// EnterExtendItemList is called when entering the extendItemList production.
	EnterExtendItemList(c *ExtendItemListContext)

	// EnterExtendItem is called when entering the extendItem production.
	EnterExtendItem(c *ExtendItemContext)

	// EnterSummarizeOperator is called when entering the summarizeOperator production.
	EnterSummarizeOperator(c *SummarizeOperatorContext)

	// EnterSummarizeHints is called when entering the summarizeHints production.
	EnterSummarizeHints(c *SummarizeHintsContext)

	// EnterAggregationList is called when entering the aggregationList production.
	EnterAggregationList(c *AggregationListContext)

	// EnterAggregationItem is called when entering the aggregationItem production.
	EnterAggregationItem(c *AggregationItemContext)

	// EnterAggregationFunction is called when entering the aggregationFunction production.
	EnterAggregationFunction(c *AggregationFunctionContext)

	// EnterGroupByList is called when entering the groupByList production.
	EnterGroupByList(c *GroupByListContext)

	// EnterGroupByItem is called when entering the groupByItem production.
	EnterGroupByItem(c *GroupByItemContext)

	// EnterSortOperator is called when entering the sortOperator production.
	EnterSortOperator(c *SortOperatorContext)

	// EnterSortList is called when entering the sortList production.
	EnterSortList(c *SortListContext)

	// EnterSortItem is called when entering the sortItem production.
	EnterSortItem(c *SortItemContext)

	// EnterSortDirection is called when entering the sortDirection production.
	EnterSortDirection(c *SortDirectionContext)

	// EnterNullsPosition is called when entering the nullsPosition production.
	EnterNullsPosition(c *NullsPositionContext)

	// EnterTopOperator is called when entering the topOperator production.
	EnterTopOperator(c *TopOperatorContext)

	// EnterTopNestedOperator is called when entering the topNestedOperator production.
	EnterTopNestedOperator(c *TopNestedOperatorContext)

	// EnterTopNestedClause is called when entering the topNestedClause production.
	EnterTopNestedClause(c *TopNestedClauseContext)

	// EnterTakeOperator is called when entering the takeOperator production.
	EnterTakeOperator(c *TakeOperatorContext)

	// EnterDistinctOperator is called when entering the distinctOperator production.
	EnterDistinctOperator(c *DistinctOperatorContext)

	// EnterDistinctColumns is called when entering the distinctColumns production.
	EnterDistinctColumns(c *DistinctColumnsContext)

	// EnterCountOperator is called when entering the countOperator production.
	EnterCountOperator(c *CountOperatorContext)

	// EnterJoinOperator is called when entering the joinOperator production.
	EnterJoinOperator(c *JoinOperatorContext)

	// EnterJoinKind is called when entering the joinKind production.
	EnterJoinKind(c *JoinKindContext)

	// EnterJoinFlavor is called when entering the joinFlavor production.
	EnterJoinFlavor(c *JoinFlavorContext)

	// EnterJoinHints is called when entering the joinHints production.
	EnterJoinHints(c *JoinHintsContext)

	// EnterJoinHint is called when entering the joinHint production.
	EnterJoinHint(c *JoinHintContext)

	// EnterJoinCondition is called when entering the joinCondition production.
	EnterJoinCondition(c *JoinConditionContext)

	// EnterJoinAttribute is called when entering the joinAttribute production.
	EnterJoinAttribute(c *JoinAttributeContext)

	// EnterUnionOperator is called when entering the unionOperator production.
	EnterUnionOperator(c *UnionOperatorContext)

	// EnterUnionParameters is called when entering the unionParameters production.
	EnterUnionParameters(c *UnionParametersContext)

	// EnterUnionParameter is called when entering the unionParameter production.
	EnterUnionParameter(c *UnionParameterContext)

	// EnterUnionTables is called when entering the unionTables production.
	EnterUnionTables(c *UnionTablesContext)

	// EnterUnionTable is called when entering the unionTable production.
	EnterUnionTable(c *UnionTableContext)

	// EnterLookupOperator is called when entering the lookupOperator production.
	EnterLookupOperator(c *LookupOperatorContext)

	// EnterLookupKind is called when entering the lookupKind production.
	EnterLookupKind(c *LookupKindContext)

	// EnterLookupCondition is called when entering the lookupCondition production.
	EnterLookupCondition(c *LookupConditionContext)

	// EnterParseOperator is called when entering the parseOperator production.
	EnterParseOperator(c *ParseOperatorContext)

	// EnterParseKind is called when entering the parseKind production.
	EnterParseKind(c *ParseKindContext)

	// EnterParsePattern is called when entering the parsePattern production.
	EnterParsePattern(c *ParsePatternContext)

	// EnterParsePatternItem is called when entering the parsePatternItem production.
	EnterParsePatternItem(c *ParsePatternItemContext)

	// EnterParseKvOperator is called when entering the parseKvOperator production.
	EnterParseKvOperator(c *ParseKvOperatorContext)

	// EnterKvPairList is called when entering the kvPairList production.
	EnterKvPairList(c *KvPairListContext)

	// EnterKvPair is called when entering the kvPair production.
	EnterKvPair(c *KvPairContext)

	// EnterParseKvParameters is called when entering the parseKvParameters production.
	EnterParseKvParameters(c *ParseKvParametersContext)

	// EnterParseKvParam is called when entering the parseKvParam production.
	EnterParseKvParam(c *ParseKvParamContext)

	// EnterMvExpandOperator is called when entering the mvExpandOperator production.
	EnterMvExpandOperator(c *MvExpandOperatorContext)

	// EnterMvExpandKind is called when entering the mvExpandKind production.
	EnterMvExpandKind(c *MvExpandKindContext)

	// EnterMvExpandParams is called when entering the mvExpandParams production.
	EnterMvExpandParams(c *MvExpandParamsContext)

	// EnterMvExpandItemList is called when entering the mvExpandItemList production.
	EnterMvExpandItemList(c *MvExpandItemListContext)

	// EnterMvExpandItem is called when entering the mvExpandItem production.
	EnterMvExpandItem(c *MvExpandItemContext)

	// EnterLimitClause is called when entering the limitClause production.
	EnterLimitClause(c *LimitClauseContext)

	// EnterMvApplyOperator is called when entering the mvApplyOperator production.
	EnterMvApplyOperator(c *MvApplyOperatorContext)

	// EnterMvApplyItemList is called when entering the mvApplyItemList production.
	EnterMvApplyItemList(c *MvApplyItemListContext)

	// EnterMvApplyItem is called when entering the mvApplyItem production.
	EnterMvApplyItem(c *MvApplyItemContext)

	// EnterMvApplyOnClause is called when entering the mvApplyOnClause production.
	EnterMvApplyOnClause(c *MvApplyOnClauseContext)

	// EnterEvaluateOperator is called when entering the evaluateOperator production.
	EnterEvaluateOperator(c *EvaluateOperatorContext)

	// EnterEvaluateHints is called when entering the evaluateHints production.
	EnterEvaluateHints(c *EvaluateHintsContext)

	// EnterFacetOperator is called when entering the facetOperator production.
	EnterFacetOperator(c *FacetOperatorContext)

	// EnterForkOperator is called when entering the forkOperator production.
	EnterForkOperator(c *ForkOperatorContext)

	// EnterForkBranch is called when entering the forkBranch production.
	EnterForkBranch(c *ForkBranchContext)

	// EnterPartitionOperator is called when entering the partitionOperator production.
	EnterPartitionOperator(c *PartitionOperatorContext)

	// EnterPartitionHints is called when entering the partitionHints production.
	EnterPartitionHints(c *PartitionHintsContext)

	// EnterScanOperator is called when entering the scanOperator production.
	EnterScanOperator(c *ScanOperatorContext)

	// EnterScanParams is called when entering the scanParams production.
	EnterScanParams(c *ScanParamsContext)

	// EnterScanDeclare is called when entering the scanDeclare production.
	EnterScanDeclare(c *ScanDeclareContext)

	// EnterScanDeclareItem is called when entering the scanDeclareItem production.
	EnterScanDeclareItem(c *ScanDeclareItemContext)

	// EnterScanStepList is called when entering the scanStepList production.
	EnterScanStepList(c *ScanStepListContext)

	// EnterScanStep is called when entering the scanStep production.
	EnterScanStep(c *ScanStepContext)

	// EnterScanAction is called when entering the scanAction production.
	EnterScanAction(c *ScanActionContext)

	// EnterSerializeOperator is called when entering the serializeOperator production.
	EnterSerializeOperator(c *SerializeOperatorContext)

	// EnterSampleOperator is called when entering the sampleOperator production.
	EnterSampleOperator(c *SampleOperatorContext)

	// EnterSampleDistinctOperator is called when entering the sampleDistinctOperator production.
	EnterSampleDistinctOperator(c *SampleDistinctOperatorContext)

	// EnterMakeSeriesOperator is called when entering the makeSeriesOperator production.
	EnterMakeSeriesOperator(c *MakeSeriesOperatorContext)

	// EnterMakeSeriesItemList is called when entering the makeSeriesItemList production.
	EnterMakeSeriesItemList(c *MakeSeriesItemListContext)

	// EnterMakeSeriesItem is called when entering the makeSeriesItem production.
	EnterMakeSeriesItem(c *MakeSeriesItemContext)

	// EnterMakeSeriesOnClause is called when entering the makeSeriesOnClause production.
	EnterMakeSeriesOnClause(c *MakeSeriesOnClauseContext)

	// EnterMakeSeriesParams is called when entering the makeSeriesParams production.
	EnterMakeSeriesParams(c *MakeSeriesParamsContext)

	// EnterFindOperator is called when entering the findOperator production.
	EnterFindOperator(c *FindOperatorContext)

	// EnterFindParams is called when entering the findParams production.
	EnterFindParams(c *FindParamsContext)

	// EnterGetschemaOperator is called when entering the getschemaOperator production.
	EnterGetschemaOperator(c *GetschemaOperatorContext)

	// EnterRenderOperator is called when entering the renderOperator production.
	EnterRenderOperator(c *RenderOperatorContext)

	// EnterRenderProperties is called when entering the renderProperties production.
	EnterRenderProperties(c *RenderPropertiesContext)

	// EnterRenderProperty is called when entering the renderProperty production.
	EnterRenderProperty(c *RenderPropertyContext)

	// EnterConsumeOperator is called when entering the consumeOperator production.
	EnterConsumeOperator(c *ConsumeOperatorContext)

	// EnterInvokeOperator is called when entering the invokeOperator production.
	EnterInvokeOperator(c *InvokeOperatorContext)

	// EnterAsOperator is called when entering the asOperator production.
	EnterAsOperator(c *AsOperatorContext)

	// EnterGraphOperator is called when entering the graphOperator production.
	EnterGraphOperator(c *GraphOperatorContext)

	// EnterMakeGraphOperator is called when entering the makeGraphOperator production.
	EnterMakeGraphOperator(c *MakeGraphOperatorContext)

	// EnterGraphMatchOperator is called when entering the graphMatchOperator production.
	EnterGraphMatchOperator(c *GraphMatchOperatorContext)

	// EnterGraphPattern is called when entering the graphPattern production.
	EnterGraphPattern(c *GraphPatternContext)

	// EnterGraphPatternElement is called when entering the graphPatternElement production.
	EnterGraphPatternElement(c *GraphPatternElementContext)

	// EnterGraphEdge is called when entering the graphEdge production.
	EnterGraphEdge(c *GraphEdgeContext)

	// EnterGraphShortestPathsOperator is called when entering the graphShortestPathsOperator production.
	EnterGraphShortestPathsOperator(c *GraphShortestPathsOperatorContext)

	// EnterGraphToTableOperator is called when entering the graphToTableOperator production.
	EnterGraphToTableOperator(c *GraphToTableOperatorContext)

	// EnterGraphToTableParams is called when entering the graphToTableParams production.
	EnterGraphToTableParams(c *GraphToTableParamsContext)

	// EnterDatatable is called when entering the datatable production.
	EnterDatatable(c *DatatableContext)

	// EnterDatatableSchema is called when entering the datatableSchema production.
	EnterDatatableSchema(c *DatatableSchemaContext)

	// EnterDatatableColumn is called when entering the datatableColumn production.
	EnterDatatableColumn(c *DatatableColumnContext)

	// EnterDatatableRows is called when entering the datatableRows production.
	EnterDatatableRows(c *DatatableRowsContext)

	// EnterExternalData is called when entering the externalData production.
	EnterExternalData(c *ExternalDataContext)

	// EnterExternalDataUri is called when entering the externalDataUri production.
	EnterExternalDataUri(c *ExternalDataUriContext)

	// EnterExternalDataOptions is called when entering the externalDataOptions production.
	EnterExternalDataOptions(c *ExternalDataOptionsContext)

	// EnterExternalDataOption is called when entering the externalDataOption production.
	EnterExternalDataOption(c *ExternalDataOptionContext)

	// EnterPrintArgList is called when entering the printArgList production.
	EnterPrintArgList(c *PrintArgListContext)

	// EnterPrintArg is called when entering the printArg production.
	EnterPrintArg(c *PrintArgContext)

	// EnterExpression is called when entering the expression production.
	EnterExpression(c *ExpressionContext)

	// EnterOrExpression is called when entering the orExpression production.
	EnterOrExpression(c *OrExpressionContext)

	// EnterAndExpression is called when entering the andExpression production.
	EnterAndExpression(c *AndExpressionContext)

	// EnterNotExpression is called when entering the notExpression production.
	EnterNotExpression(c *NotExpressionContext)

	// EnterComparisonExpression is called when entering the comparisonExpression production.
	EnterComparisonExpression(c *ComparisonExpressionContext)

	// EnterComparisonOperator is called when entering the comparisonOperator production.
	EnterComparisonOperator(c *ComparisonOperatorContext)

	// EnterStringOperator is called when entering the stringOperator production.
	EnterStringOperator(c *StringOperatorContext)

	// EnterAdditiveExpression is called when entering the additiveExpression production.
	EnterAdditiveExpression(c *AdditiveExpressionContext)

	// EnterMultiplicativeExpression is called when entering the multiplicativeExpression production.
	EnterMultiplicativeExpression(c *MultiplicativeExpressionContext)

	// EnterUnaryExpression is called when entering the unaryExpression production.
	EnterUnaryExpression(c *UnaryExpressionContext)

	// EnterPostfixExpression is called when entering the postfixExpression production.
	EnterPostfixExpression(c *PostfixExpressionContext)

	// EnterPostfixOperator is called when entering the postfixOperator production.
	EnterPostfixOperator(c *PostfixOperatorContext)

	// EnterPrimaryExpression is called when entering the primaryExpression production.
	EnterPrimaryExpression(c *PrimaryExpressionContext)

	// EnterFunctionCall is called when entering the functionCall production.
	EnterFunctionCall(c *FunctionCallContext)

	// EnterBuiltinFunction is called when entering the builtinFunction production.
	EnterBuiltinFunction(c *BuiltinFunctionContext)

	// EnterArgumentList is called when entering the argumentList production.
	EnterArgumentList(c *ArgumentListContext)

	// EnterArgument is called when entering the argument production.
	EnterArgument(c *ArgumentContext)

	// EnterCaseExpression is called when entering the caseExpression production.
	EnterCaseExpression(c *CaseExpressionContext)

	// EnterCaseBranch is called when entering the caseBranch production.
	EnterCaseBranch(c *CaseBranchContext)

	// EnterIffExpression is called when entering the iffExpression production.
	EnterIffExpression(c *IffExpressionContext)

	// EnterToScalarExpression is called when entering the toScalarExpression production.
	EnterToScalarExpression(c *ToScalarExpressionContext)

	// EnterArrayExpression is called when entering the arrayExpression production.
	EnterArrayExpression(c *ArrayExpressionContext)

	// EnterObjectExpression is called when entering the objectExpression production.
	EnterObjectExpression(c *ObjectExpressionContext)

	// EnterObjectPropertyList is called when entering the objectPropertyList production.
	EnterObjectPropertyList(c *ObjectPropertyListContext)

	// EnterObjectProperty is called when entering the objectProperty production.
	EnterObjectProperty(c *ObjectPropertyContext)

	// EnterFunctionParameters is called when entering the functionParameters production.
	EnterFunctionParameters(c *FunctionParametersContext)

	// EnterFunctionParameter is called when entering the functionParameter production.
	EnterFunctionParameter(c *FunctionParameterContext)

	// EnterTypeSpecifier is called when entering the typeSpecifier production.
	EnterTypeSpecifier(c *TypeSpecifierContext)

	// EnterLiteral is called when entering the literal production.
	EnterLiteral(c *LiteralContext)

	// EnterBooleanLiteral is called when entering the booleanLiteral production.
	EnterBooleanLiteral(c *BooleanLiteralContext)

	// EnterIdentifier is called when entering the identifier production.
	EnterIdentifier(c *IdentifierContext)

	// EnterIdentifierList is called when entering the identifierList production.
	EnterIdentifierList(c *IdentifierListContext)

	// EnterExpressionList is called when entering the expressionList production.
	EnterExpressionList(c *ExpressionListContext)

	// ExitQuery is called when exiting the query production.
	ExitQuery(c *QueryContext)

	// ExitStatement is called when exiting the statement production.
	ExitStatement(c *StatementContext)

	// ExitLetStatement is called when exiting the letStatement production.
	ExitLetStatement(c *LetStatementContext)

	// ExitSetStatement is called when exiting the setStatement production.
	ExitSetStatement(c *SetStatementContext)

	// ExitAliasStatement is called when exiting the aliasStatement production.
	ExitAliasStatement(c *AliasStatementContext)

	// ExitDeclareStatement is called when exiting the declareStatement production.
	ExitDeclareStatement(c *DeclareStatementContext)

	// ExitPatternStatement is called when exiting the patternStatement production.
	ExitPatternStatement(c *PatternStatementContext)

	// ExitRestrictStatement is called when exiting the restrictStatement production.
	ExitRestrictStatement(c *RestrictStatementContext)

	// ExitViewExpression is called when exiting the viewExpression production.
	ExitViewExpression(c *ViewExpressionContext)

	// ExitPatternDefinition is called when exiting the patternDefinition production.
	ExitPatternDefinition(c *PatternDefinitionContext)

	// ExitPatternParam is called when exiting the patternParam production.
	ExitPatternParam(c *PatternParamContext)

	// ExitTabularExpression is called when exiting the tabularExpression production.
	ExitTabularExpression(c *TabularExpressionContext)

	// ExitTabularSource is called when exiting the tabularSource production.
	ExitTabularSource(c *TabularSourceContext)

	// ExitTableName is called when exiting the tableName production.
	ExitTableName(c *TableNameContext)

	// ExitDatabaseTableName is called when exiting the databaseTableName production.
	ExitDatabaseTableName(c *DatabaseTableNameContext)

	// ExitMaterializeExpression is called when exiting the materializeExpression production.
	ExitMaterializeExpression(c *MaterializeExpressionContext)

	// ExitTabularOperator is called when exiting the tabularOperator production.
	ExitTabularOperator(c *TabularOperatorContext)

	// ExitWhereOperator is called when exiting the whereOperator production.
	ExitWhereOperator(c *WhereOperatorContext)

	// ExitSearchOperator is called when exiting the searchOperator production.
	ExitSearchOperator(c *SearchOperatorContext)

	// ExitSearchKind is called when exiting the searchKind production.
	ExitSearchKind(c *SearchKindContext)

	// ExitTableList is called when exiting the tableList production.
	ExitTableList(c *TableListContext)

	// ExitProjectOperator is called when exiting the projectOperator production.
	ExitProjectOperator(c *ProjectOperatorContext)

	// ExitProjectAwayOperator is called when exiting the projectAwayOperator production.
	ExitProjectAwayOperator(c *ProjectAwayOperatorContext)

	// ExitProjectKeepOperator is called when exiting the projectKeepOperator production.
	ExitProjectKeepOperator(c *ProjectKeepOperatorContext)

	// ExitProjectRenameOperator is called when exiting the projectRenameOperator production.
	ExitProjectRenameOperator(c *ProjectRenameOperatorContext)

	// ExitProjectReorderOperator is called when exiting the projectReorderOperator production.
	ExitProjectReorderOperator(c *ProjectReorderOperatorContext)

	// ExitProjectItemList is called when exiting the projectItemList production.
	ExitProjectItemList(c *ProjectItemListContext)

	// ExitProjectItem is called when exiting the projectItem production.
	ExitProjectItem(c *ProjectItemContext)

	// ExitIdentifierOrWildcardList is called when exiting the identifierOrWildcardList production.
	ExitIdentifierOrWildcardList(c *IdentifierOrWildcardListContext)

	// ExitIdentifierOrWildcard is called when exiting the identifierOrWildcard production.
	ExitIdentifierOrWildcard(c *IdentifierOrWildcardContext)

	// ExitRenameList is called when exiting the renameList production.
	ExitRenameList(c *RenameListContext)

	// ExitRenameItem is called when exiting the renameItem production.
	ExitRenameItem(c *RenameItemContext)

	// ExitExtendOperator is called when exiting the extendOperator production.
	ExitExtendOperator(c *ExtendOperatorContext)

	// ExitExtendItemList is called when exiting the extendItemList production.
	ExitExtendItemList(c *ExtendItemListContext)

	// ExitExtendItem is called when exiting the extendItem production.
	ExitExtendItem(c *ExtendItemContext)

	// ExitSummarizeOperator is called when exiting the summarizeOperator production.
	ExitSummarizeOperator(c *SummarizeOperatorContext)

	// ExitSummarizeHints is called when exiting the summarizeHints production.
	ExitSummarizeHints(c *SummarizeHintsContext)

	// ExitAggregationList is called when exiting the aggregationList production.
	ExitAggregationList(c *AggregationListContext)

	// ExitAggregationItem is called when exiting the aggregationItem production.
	ExitAggregationItem(c *AggregationItemContext)

	// ExitAggregationFunction is called when exiting the aggregationFunction production.
	ExitAggregationFunction(c *AggregationFunctionContext)

	// ExitGroupByList is called when exiting the groupByList production.
	ExitGroupByList(c *GroupByListContext)

	// ExitGroupByItem is called when exiting the groupByItem production.
	ExitGroupByItem(c *GroupByItemContext)

	// ExitSortOperator is called when exiting the sortOperator production.
	ExitSortOperator(c *SortOperatorContext)

	// ExitSortList is called when exiting the sortList production.
	ExitSortList(c *SortListContext)

	// ExitSortItem is called when exiting the sortItem production.
	ExitSortItem(c *SortItemContext)

	// ExitSortDirection is called when exiting the sortDirection production.
	ExitSortDirection(c *SortDirectionContext)

	// ExitNullsPosition is called when exiting the nullsPosition production.
	ExitNullsPosition(c *NullsPositionContext)

	// ExitTopOperator is called when exiting the topOperator production.
	ExitTopOperator(c *TopOperatorContext)

	// ExitTopNestedOperator is called when exiting the topNestedOperator production.
	ExitTopNestedOperator(c *TopNestedOperatorContext)

	// ExitTopNestedClause is called when exiting the topNestedClause production.
	ExitTopNestedClause(c *TopNestedClauseContext)

	// ExitTakeOperator is called when exiting the takeOperator production.
	ExitTakeOperator(c *TakeOperatorContext)

	// ExitDistinctOperator is called when exiting the distinctOperator production.
	ExitDistinctOperator(c *DistinctOperatorContext)

	// ExitDistinctColumns is called when exiting the distinctColumns production.
	ExitDistinctColumns(c *DistinctColumnsContext)

	// ExitCountOperator is called when exiting the countOperator production.
	ExitCountOperator(c *CountOperatorContext)

	// ExitJoinOperator is called when exiting the joinOperator production.
	ExitJoinOperator(c *JoinOperatorContext)

	// ExitJoinKind is called when exiting the joinKind production.
	ExitJoinKind(c *JoinKindContext)

	// ExitJoinFlavor is called when exiting the joinFlavor production.
	ExitJoinFlavor(c *JoinFlavorContext)

	// ExitJoinHints is called when exiting the joinHints production.
	ExitJoinHints(c *JoinHintsContext)

	// ExitJoinHint is called when exiting the joinHint production.
	ExitJoinHint(c *JoinHintContext)

	// ExitJoinCondition is called when exiting the joinCondition production.
	ExitJoinCondition(c *JoinConditionContext)

	// ExitJoinAttribute is called when exiting the joinAttribute production.
	ExitJoinAttribute(c *JoinAttributeContext)

	// ExitUnionOperator is called when exiting the unionOperator production.
	ExitUnionOperator(c *UnionOperatorContext)

	// ExitUnionParameters is called when exiting the unionParameters production.
	ExitUnionParameters(c *UnionParametersContext)

	// ExitUnionParameter is called when exiting the unionParameter production.
	ExitUnionParameter(c *UnionParameterContext)

	// ExitUnionTables is called when exiting the unionTables production.
	ExitUnionTables(c *UnionTablesContext)

	// ExitUnionTable is called when exiting the unionTable production.
	ExitUnionTable(c *UnionTableContext)

	// ExitLookupOperator is called when exiting the lookupOperator production.
	ExitLookupOperator(c *LookupOperatorContext)

	// ExitLookupKind is called when exiting the lookupKind production.
	ExitLookupKind(c *LookupKindContext)

	// ExitLookupCondition is called when exiting the lookupCondition production.
	ExitLookupCondition(c *LookupConditionContext)

	// ExitParseOperator is called when exiting the parseOperator production.
	ExitParseOperator(c *ParseOperatorContext)

	// ExitParseKind is called when exiting the parseKind production.
	ExitParseKind(c *ParseKindContext)

	// ExitParsePattern is called when exiting the parsePattern production.
	ExitParsePattern(c *ParsePatternContext)

	// ExitParsePatternItem is called when exiting the parsePatternItem production.
	ExitParsePatternItem(c *ParsePatternItemContext)

	// ExitParseKvOperator is called when exiting the parseKvOperator production.
	ExitParseKvOperator(c *ParseKvOperatorContext)

	// ExitKvPairList is called when exiting the kvPairList production.
	ExitKvPairList(c *KvPairListContext)

	// ExitKvPair is called when exiting the kvPair production.
	ExitKvPair(c *KvPairContext)

	// ExitParseKvParameters is called when exiting the parseKvParameters production.
	ExitParseKvParameters(c *ParseKvParametersContext)

	// ExitParseKvParam is called when exiting the parseKvParam production.
	ExitParseKvParam(c *ParseKvParamContext)

	// ExitMvExpandOperator is called when exiting the mvExpandOperator production.
	ExitMvExpandOperator(c *MvExpandOperatorContext)

	// ExitMvExpandKind is called when exiting the mvExpandKind production.
	ExitMvExpandKind(c *MvExpandKindContext)

	// ExitMvExpandParams is called when exiting the mvExpandParams production.
	ExitMvExpandParams(c *MvExpandParamsContext)

	// ExitMvExpandItemList is called when exiting the mvExpandItemList production.
	ExitMvExpandItemList(c *MvExpandItemListContext)

	// ExitMvExpandItem is called when exiting the mvExpandItem production.
	ExitMvExpandItem(c *MvExpandItemContext)

	// ExitLimitClause is called when exiting the limitClause production.
	ExitLimitClause(c *LimitClauseContext)

	// ExitMvApplyOperator is called when exiting the mvApplyOperator production.
	ExitMvApplyOperator(c *MvApplyOperatorContext)

	// ExitMvApplyItemList is called when exiting the mvApplyItemList production.
	ExitMvApplyItemList(c *MvApplyItemListContext)

	// ExitMvApplyItem is called when exiting the mvApplyItem production.
	ExitMvApplyItem(c *MvApplyItemContext)

	// ExitMvApplyOnClause is called when exiting the mvApplyOnClause production.
	ExitMvApplyOnClause(c *MvApplyOnClauseContext)

	// ExitEvaluateOperator is called when exiting the evaluateOperator production.
	ExitEvaluateOperator(c *EvaluateOperatorContext)

	// ExitEvaluateHints is called when exiting the evaluateHints production.
	ExitEvaluateHints(c *EvaluateHintsContext)

	// ExitFacetOperator is called when exiting the facetOperator production.
	ExitFacetOperator(c *FacetOperatorContext)

	// ExitForkOperator is called when exiting the forkOperator production.
	ExitForkOperator(c *ForkOperatorContext)

	// ExitForkBranch is called when exiting the forkBranch production.
	ExitForkBranch(c *ForkBranchContext)

	// ExitPartitionOperator is called when exiting the partitionOperator production.
	ExitPartitionOperator(c *PartitionOperatorContext)

	// ExitPartitionHints is called when exiting the partitionHints production.
	ExitPartitionHints(c *PartitionHintsContext)

	// ExitScanOperator is called when exiting the scanOperator production.
	ExitScanOperator(c *ScanOperatorContext)

	// ExitScanParams is called when exiting the scanParams production.
	ExitScanParams(c *ScanParamsContext)

	// ExitScanDeclare is called when exiting the scanDeclare production.
	ExitScanDeclare(c *ScanDeclareContext)

	// ExitScanDeclareItem is called when exiting the scanDeclareItem production.
	ExitScanDeclareItem(c *ScanDeclareItemContext)

	// ExitScanStepList is called when exiting the scanStepList production.
	ExitScanStepList(c *ScanStepListContext)

	// ExitScanStep is called when exiting the scanStep production.
	ExitScanStep(c *ScanStepContext)

	// ExitScanAction is called when exiting the scanAction production.
	ExitScanAction(c *ScanActionContext)

	// ExitSerializeOperator is called when exiting the serializeOperator production.
	ExitSerializeOperator(c *SerializeOperatorContext)

	// ExitSampleOperator is called when exiting the sampleOperator production.
	ExitSampleOperator(c *SampleOperatorContext)

	// ExitSampleDistinctOperator is called when exiting the sampleDistinctOperator production.
	ExitSampleDistinctOperator(c *SampleDistinctOperatorContext)

	// ExitMakeSeriesOperator is called when exiting the makeSeriesOperator production.
	ExitMakeSeriesOperator(c *MakeSeriesOperatorContext)

	// ExitMakeSeriesItemList is called when exiting the makeSeriesItemList production.
	ExitMakeSeriesItemList(c *MakeSeriesItemListContext)

	// ExitMakeSeriesItem is called when exiting the makeSeriesItem production.
	ExitMakeSeriesItem(c *MakeSeriesItemContext)

	// ExitMakeSeriesOnClause is called when exiting the makeSeriesOnClause production.
	ExitMakeSeriesOnClause(c *MakeSeriesOnClauseContext)

	// ExitMakeSeriesParams is called when exiting the makeSeriesParams production.
	ExitMakeSeriesParams(c *MakeSeriesParamsContext)

	// ExitFindOperator is called when exiting the findOperator production.
	ExitFindOperator(c *FindOperatorContext)

	// ExitFindParams is called when exiting the findParams production.
	ExitFindParams(c *FindParamsContext)

	// ExitGetschemaOperator is called when exiting the getschemaOperator production.
	ExitGetschemaOperator(c *GetschemaOperatorContext)

	// ExitRenderOperator is called when exiting the renderOperator production.
	ExitRenderOperator(c *RenderOperatorContext)

	// ExitRenderProperties is called when exiting the renderProperties production.
	ExitRenderProperties(c *RenderPropertiesContext)

	// ExitRenderProperty is called when exiting the renderProperty production.
	ExitRenderProperty(c *RenderPropertyContext)

	// ExitConsumeOperator is called when exiting the consumeOperator production.
	ExitConsumeOperator(c *ConsumeOperatorContext)

	// ExitInvokeOperator is called when exiting the invokeOperator production.
	ExitInvokeOperator(c *InvokeOperatorContext)

	// ExitAsOperator is called when exiting the asOperator production.
	ExitAsOperator(c *AsOperatorContext)

	// ExitGraphOperator is called when exiting the graphOperator production.
	ExitGraphOperator(c *GraphOperatorContext)

	// ExitMakeGraphOperator is called when exiting the makeGraphOperator production.
	ExitMakeGraphOperator(c *MakeGraphOperatorContext)

	// ExitGraphMatchOperator is called when exiting the graphMatchOperator production.
	ExitGraphMatchOperator(c *GraphMatchOperatorContext)

	// ExitGraphPattern is called when exiting the graphPattern production.
	ExitGraphPattern(c *GraphPatternContext)

	// ExitGraphPatternElement is called when exiting the graphPatternElement production.
	ExitGraphPatternElement(c *GraphPatternElementContext)

	// ExitGraphEdge is called when exiting the graphEdge production.
	ExitGraphEdge(c *GraphEdgeContext)

	// ExitGraphShortestPathsOperator is called when exiting the graphShortestPathsOperator production.
	ExitGraphShortestPathsOperator(c *GraphShortestPathsOperatorContext)

	// ExitGraphToTableOperator is called when exiting the graphToTableOperator production.
	ExitGraphToTableOperator(c *GraphToTableOperatorContext)

	// ExitGraphToTableParams is called when exiting the graphToTableParams production.
	ExitGraphToTableParams(c *GraphToTableParamsContext)

	// ExitDatatable is called when exiting the datatable production.
	ExitDatatable(c *DatatableContext)

	// ExitDatatableSchema is called when exiting the datatableSchema production.
	ExitDatatableSchema(c *DatatableSchemaContext)

	// ExitDatatableColumn is called when exiting the datatableColumn production.
	ExitDatatableColumn(c *DatatableColumnContext)

	// ExitDatatableRows is called when exiting the datatableRows production.
	ExitDatatableRows(c *DatatableRowsContext)

	// ExitExternalData is called when exiting the externalData production.
	ExitExternalData(c *ExternalDataContext)

	// ExitExternalDataUri is called when exiting the externalDataUri production.
	ExitExternalDataUri(c *ExternalDataUriContext)

	// ExitExternalDataOptions is called when exiting the externalDataOptions production.
	ExitExternalDataOptions(c *ExternalDataOptionsContext)

	// ExitExternalDataOption is called when exiting the externalDataOption production.
	ExitExternalDataOption(c *ExternalDataOptionContext)

	// ExitPrintArgList is called when exiting the printArgList production.
	ExitPrintArgList(c *PrintArgListContext)

	// ExitPrintArg is called when exiting the printArg production.
	ExitPrintArg(c *PrintArgContext)

	// ExitExpression is called when exiting the expression production.
	ExitExpression(c *ExpressionContext)

	// ExitOrExpression is called when exiting the orExpression production.
	ExitOrExpression(c *OrExpressionContext)

	// ExitAndExpression is called when exiting the andExpression production.
	ExitAndExpression(c *AndExpressionContext)

	// ExitNotExpression is called when exiting the notExpression production.
	ExitNotExpression(c *NotExpressionContext)

	// ExitComparisonExpression is called when exiting the comparisonExpression production.
	ExitComparisonExpression(c *ComparisonExpressionContext)

	// ExitComparisonOperator is called when exiting the comparisonOperator production.
	ExitComparisonOperator(c *ComparisonOperatorContext)

	// ExitStringOperator is called when exiting the stringOperator production.
	ExitStringOperator(c *StringOperatorContext)

	// ExitAdditiveExpression is called when exiting the additiveExpression production.
	ExitAdditiveExpression(c *AdditiveExpressionContext)

	// ExitMultiplicativeExpression is called when exiting the multiplicativeExpression production.
	ExitMultiplicativeExpression(c *MultiplicativeExpressionContext)

	// ExitUnaryExpression is called when exiting the unaryExpression production.
	ExitUnaryExpression(c *UnaryExpressionContext)

	// ExitPostfixExpression is called when exiting the postfixExpression production.
	ExitPostfixExpression(c *PostfixExpressionContext)

	// ExitPostfixOperator is called when exiting the postfixOperator production.
	ExitPostfixOperator(c *PostfixOperatorContext)

	// ExitPrimaryExpression is called when exiting the primaryExpression production.
	ExitPrimaryExpression(c *PrimaryExpressionContext)

	// ExitFunctionCall is called when exiting the functionCall production.
	ExitFunctionCall(c *FunctionCallContext)

	// ExitBuiltinFunction is called when exiting the builtinFunction production.
	ExitBuiltinFunction(c *BuiltinFunctionContext)

	// ExitArgumentList is called when exiting the argumentList production.
	ExitArgumentList(c *ArgumentListContext)

	// ExitArgument is called when exiting the argument production.
	ExitArgument(c *ArgumentContext)

	// ExitCaseExpression is called when exiting the caseExpression production.
	ExitCaseExpression(c *CaseExpressionContext)

	// ExitCaseBranch is called when exiting the caseBranch production.
	ExitCaseBranch(c *CaseBranchContext)

	// ExitIffExpression is called when exiting the iffExpression production.
	ExitIffExpression(c *IffExpressionContext)

	// ExitToScalarExpression is called when exiting the toScalarExpression production.
	ExitToScalarExpression(c *ToScalarExpressionContext)

	// ExitArrayExpression is called when exiting the arrayExpression production.
	ExitArrayExpression(c *ArrayExpressionContext)

	// ExitObjectExpression is called when exiting the objectExpression production.
	ExitObjectExpression(c *ObjectExpressionContext)

	// ExitObjectPropertyList is called when exiting the objectPropertyList production.
	ExitObjectPropertyList(c *ObjectPropertyListContext)

	// ExitObjectProperty is called when exiting the objectProperty production.
	ExitObjectProperty(c *ObjectPropertyContext)

	// ExitFunctionParameters is called when exiting the functionParameters production.
	ExitFunctionParameters(c *FunctionParametersContext)

	// ExitFunctionParameter is called when exiting the functionParameter production.
	ExitFunctionParameter(c *FunctionParameterContext)

	// ExitTypeSpecifier is called when exiting the typeSpecifier production.
	ExitTypeSpecifier(c *TypeSpecifierContext)

	// ExitLiteral is called when exiting the literal production.
	ExitLiteral(c *LiteralContext)

	// ExitBooleanLiteral is called when exiting the booleanLiteral production.
	ExitBooleanLiteral(c *BooleanLiteralContext)

	// ExitIdentifier is called when exiting the identifier production.
	ExitIdentifier(c *IdentifierContext)

	// ExitIdentifierList is called when exiting the identifierList production.
	ExitIdentifierList(c *IdentifierListContext)

	// ExitExpressionList is called when exiting the expressionList production.
	ExitExpressionList(c *ExpressionListContext)
}
