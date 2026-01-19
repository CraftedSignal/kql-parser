// Code generated from KQLParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package kql // KQLParser
import "github.com/antlr4-go/antlr/v4"

// A complete Visitor for a parse tree produced by KQLParser.
type KQLParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by KQLParser#query.
	VisitQuery(ctx *QueryContext) interface{}

	// Visit a parse tree produced by KQLParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by KQLParser#letStatement.
	VisitLetStatement(ctx *LetStatementContext) interface{}

	// Visit a parse tree produced by KQLParser#setStatement.
	VisitSetStatement(ctx *SetStatementContext) interface{}

	// Visit a parse tree produced by KQLParser#aliasStatement.
	VisitAliasStatement(ctx *AliasStatementContext) interface{}

	// Visit a parse tree produced by KQLParser#declareStatement.
	VisitDeclareStatement(ctx *DeclareStatementContext) interface{}

	// Visit a parse tree produced by KQLParser#patternStatement.
	VisitPatternStatement(ctx *PatternStatementContext) interface{}

	// Visit a parse tree produced by KQLParser#restrictStatement.
	VisitRestrictStatement(ctx *RestrictStatementContext) interface{}

	// Visit a parse tree produced by KQLParser#viewExpression.
	VisitViewExpression(ctx *ViewExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#patternDefinition.
	VisitPatternDefinition(ctx *PatternDefinitionContext) interface{}

	// Visit a parse tree produced by KQLParser#patternParam.
	VisitPatternParam(ctx *PatternParamContext) interface{}

	// Visit a parse tree produced by KQLParser#tabularExpression.
	VisitTabularExpression(ctx *TabularExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#tabularSource.
	VisitTabularSource(ctx *TabularSourceContext) interface{}

	// Visit a parse tree produced by KQLParser#tableName.
	VisitTableName(ctx *TableNameContext) interface{}

	// Visit a parse tree produced by KQLParser#databaseTableName.
	VisitDatabaseTableName(ctx *DatabaseTableNameContext) interface{}

	// Visit a parse tree produced by KQLParser#materializeExpression.
	VisitMaterializeExpression(ctx *MaterializeExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#tabularOperator.
	VisitTabularOperator(ctx *TabularOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#whereOperator.
	VisitWhereOperator(ctx *WhereOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#searchOperator.
	VisitSearchOperator(ctx *SearchOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#searchKind.
	VisitSearchKind(ctx *SearchKindContext) interface{}

	// Visit a parse tree produced by KQLParser#tableList.
	VisitTableList(ctx *TableListContext) interface{}

	// Visit a parse tree produced by KQLParser#projectOperator.
	VisitProjectOperator(ctx *ProjectOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#projectAwayOperator.
	VisitProjectAwayOperator(ctx *ProjectAwayOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#projectKeepOperator.
	VisitProjectKeepOperator(ctx *ProjectKeepOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#projectRenameOperator.
	VisitProjectRenameOperator(ctx *ProjectRenameOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#projectReorderOperator.
	VisitProjectReorderOperator(ctx *ProjectReorderOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#projectItemList.
	VisitProjectItemList(ctx *ProjectItemListContext) interface{}

	// Visit a parse tree produced by KQLParser#projectItem.
	VisitProjectItem(ctx *ProjectItemContext) interface{}

	// Visit a parse tree produced by KQLParser#identifierOrWildcardList.
	VisitIdentifierOrWildcardList(ctx *IdentifierOrWildcardListContext) interface{}

	// Visit a parse tree produced by KQLParser#identifierOrWildcard.
	VisitIdentifierOrWildcard(ctx *IdentifierOrWildcardContext) interface{}

	// Visit a parse tree produced by KQLParser#renameList.
	VisitRenameList(ctx *RenameListContext) interface{}

	// Visit a parse tree produced by KQLParser#renameItem.
	VisitRenameItem(ctx *RenameItemContext) interface{}

	// Visit a parse tree produced by KQLParser#extendOperator.
	VisitExtendOperator(ctx *ExtendOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#extendItemList.
	VisitExtendItemList(ctx *ExtendItemListContext) interface{}

	// Visit a parse tree produced by KQLParser#extendItem.
	VisitExtendItem(ctx *ExtendItemContext) interface{}

	// Visit a parse tree produced by KQLParser#summarizeOperator.
	VisitSummarizeOperator(ctx *SummarizeOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#summarizeHints.
	VisitSummarizeHints(ctx *SummarizeHintsContext) interface{}

	// Visit a parse tree produced by KQLParser#aggregationList.
	VisitAggregationList(ctx *AggregationListContext) interface{}

	// Visit a parse tree produced by KQLParser#aggregationItem.
	VisitAggregationItem(ctx *AggregationItemContext) interface{}

	// Visit a parse tree produced by KQLParser#aggregationFunction.
	VisitAggregationFunction(ctx *AggregationFunctionContext) interface{}

	// Visit a parse tree produced by KQLParser#groupByList.
	VisitGroupByList(ctx *GroupByListContext) interface{}

	// Visit a parse tree produced by KQLParser#groupByItem.
	VisitGroupByItem(ctx *GroupByItemContext) interface{}

	// Visit a parse tree produced by KQLParser#sortOperator.
	VisitSortOperator(ctx *SortOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#sortList.
	VisitSortList(ctx *SortListContext) interface{}

	// Visit a parse tree produced by KQLParser#sortItem.
	VisitSortItem(ctx *SortItemContext) interface{}

	// Visit a parse tree produced by KQLParser#sortDirection.
	VisitSortDirection(ctx *SortDirectionContext) interface{}

	// Visit a parse tree produced by KQLParser#nullsPosition.
	VisitNullsPosition(ctx *NullsPositionContext) interface{}

	// Visit a parse tree produced by KQLParser#topOperator.
	VisitTopOperator(ctx *TopOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#topNestedOperator.
	VisitTopNestedOperator(ctx *TopNestedOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#topNestedClause.
	VisitTopNestedClause(ctx *TopNestedClauseContext) interface{}

	// Visit a parse tree produced by KQLParser#takeOperator.
	VisitTakeOperator(ctx *TakeOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#distinctOperator.
	VisitDistinctOperator(ctx *DistinctOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#distinctColumns.
	VisitDistinctColumns(ctx *DistinctColumnsContext) interface{}

	// Visit a parse tree produced by KQLParser#countOperator.
	VisitCountOperator(ctx *CountOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#joinOperator.
	VisitJoinOperator(ctx *JoinOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#joinKind.
	VisitJoinKind(ctx *JoinKindContext) interface{}

	// Visit a parse tree produced by KQLParser#joinFlavor.
	VisitJoinFlavor(ctx *JoinFlavorContext) interface{}

	// Visit a parse tree produced by KQLParser#joinHints.
	VisitJoinHints(ctx *JoinHintsContext) interface{}

	// Visit a parse tree produced by KQLParser#joinHint.
	VisitJoinHint(ctx *JoinHintContext) interface{}

	// Visit a parse tree produced by KQLParser#joinCondition.
	VisitJoinCondition(ctx *JoinConditionContext) interface{}

	// Visit a parse tree produced by KQLParser#joinAttribute.
	VisitJoinAttribute(ctx *JoinAttributeContext) interface{}

	// Visit a parse tree produced by KQLParser#unionOperator.
	VisitUnionOperator(ctx *UnionOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#unionParameters.
	VisitUnionParameters(ctx *UnionParametersContext) interface{}

	// Visit a parse tree produced by KQLParser#unionParameter.
	VisitUnionParameter(ctx *UnionParameterContext) interface{}

	// Visit a parse tree produced by KQLParser#unionTables.
	VisitUnionTables(ctx *UnionTablesContext) interface{}

	// Visit a parse tree produced by KQLParser#unionTable.
	VisitUnionTable(ctx *UnionTableContext) interface{}

	// Visit a parse tree produced by KQLParser#lookupOperator.
	VisitLookupOperator(ctx *LookupOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#lookupKind.
	VisitLookupKind(ctx *LookupKindContext) interface{}

	// Visit a parse tree produced by KQLParser#lookupCondition.
	VisitLookupCondition(ctx *LookupConditionContext) interface{}

	// Visit a parse tree produced by KQLParser#parseOperator.
	VisitParseOperator(ctx *ParseOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#parseKind.
	VisitParseKind(ctx *ParseKindContext) interface{}

	// Visit a parse tree produced by KQLParser#parsePattern.
	VisitParsePattern(ctx *ParsePatternContext) interface{}

	// Visit a parse tree produced by KQLParser#parsePatternItem.
	VisitParsePatternItem(ctx *ParsePatternItemContext) interface{}

	// Visit a parse tree produced by KQLParser#parseKvOperator.
	VisitParseKvOperator(ctx *ParseKvOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#kvPairList.
	VisitKvPairList(ctx *KvPairListContext) interface{}

	// Visit a parse tree produced by KQLParser#kvPair.
	VisitKvPair(ctx *KvPairContext) interface{}

	// Visit a parse tree produced by KQLParser#parseKvParameters.
	VisitParseKvParameters(ctx *ParseKvParametersContext) interface{}

	// Visit a parse tree produced by KQLParser#parseKvParam.
	VisitParseKvParam(ctx *ParseKvParamContext) interface{}

	// Visit a parse tree produced by KQLParser#mvExpandOperator.
	VisitMvExpandOperator(ctx *MvExpandOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#mvExpandKind.
	VisitMvExpandKind(ctx *MvExpandKindContext) interface{}

	// Visit a parse tree produced by KQLParser#mvExpandParams.
	VisitMvExpandParams(ctx *MvExpandParamsContext) interface{}

	// Visit a parse tree produced by KQLParser#mvExpandItemList.
	VisitMvExpandItemList(ctx *MvExpandItemListContext) interface{}

	// Visit a parse tree produced by KQLParser#mvExpandItem.
	VisitMvExpandItem(ctx *MvExpandItemContext) interface{}

	// Visit a parse tree produced by KQLParser#limitClause.
	VisitLimitClause(ctx *LimitClauseContext) interface{}

	// Visit a parse tree produced by KQLParser#mvApplyOperator.
	VisitMvApplyOperator(ctx *MvApplyOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#mvApplyItemList.
	VisitMvApplyItemList(ctx *MvApplyItemListContext) interface{}

	// Visit a parse tree produced by KQLParser#mvApplyItem.
	VisitMvApplyItem(ctx *MvApplyItemContext) interface{}

	// Visit a parse tree produced by KQLParser#mvApplyOnClause.
	VisitMvApplyOnClause(ctx *MvApplyOnClauseContext) interface{}

	// Visit a parse tree produced by KQLParser#evaluateOperator.
	VisitEvaluateOperator(ctx *EvaluateOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#evaluateHints.
	VisitEvaluateHints(ctx *EvaluateHintsContext) interface{}

	// Visit a parse tree produced by KQLParser#facetOperator.
	VisitFacetOperator(ctx *FacetOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#forkOperator.
	VisitForkOperator(ctx *ForkOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#forkBranch.
	VisitForkBranch(ctx *ForkBranchContext) interface{}

	// Visit a parse tree produced by KQLParser#partitionOperator.
	VisitPartitionOperator(ctx *PartitionOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#partitionHints.
	VisitPartitionHints(ctx *PartitionHintsContext) interface{}

	// Visit a parse tree produced by KQLParser#scanOperator.
	VisitScanOperator(ctx *ScanOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#scanParams.
	VisitScanParams(ctx *ScanParamsContext) interface{}

	// Visit a parse tree produced by KQLParser#scanDeclare.
	VisitScanDeclare(ctx *ScanDeclareContext) interface{}

	// Visit a parse tree produced by KQLParser#scanDeclareItem.
	VisitScanDeclareItem(ctx *ScanDeclareItemContext) interface{}

	// Visit a parse tree produced by KQLParser#scanStepList.
	VisitScanStepList(ctx *ScanStepListContext) interface{}

	// Visit a parse tree produced by KQLParser#scanStep.
	VisitScanStep(ctx *ScanStepContext) interface{}

	// Visit a parse tree produced by KQLParser#scanAction.
	VisitScanAction(ctx *ScanActionContext) interface{}

	// Visit a parse tree produced by KQLParser#serializeOperator.
	VisitSerializeOperator(ctx *SerializeOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#sampleOperator.
	VisitSampleOperator(ctx *SampleOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#sampleDistinctOperator.
	VisitSampleDistinctOperator(ctx *SampleDistinctOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#makeSeriesOperator.
	VisitMakeSeriesOperator(ctx *MakeSeriesOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#makeSeriesItemList.
	VisitMakeSeriesItemList(ctx *MakeSeriesItemListContext) interface{}

	// Visit a parse tree produced by KQLParser#makeSeriesItem.
	VisitMakeSeriesItem(ctx *MakeSeriesItemContext) interface{}

	// Visit a parse tree produced by KQLParser#makeSeriesOnClause.
	VisitMakeSeriesOnClause(ctx *MakeSeriesOnClauseContext) interface{}

	// Visit a parse tree produced by KQLParser#makeSeriesParams.
	VisitMakeSeriesParams(ctx *MakeSeriesParamsContext) interface{}

	// Visit a parse tree produced by KQLParser#findOperator.
	VisitFindOperator(ctx *FindOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#findParams.
	VisitFindParams(ctx *FindParamsContext) interface{}

	// Visit a parse tree produced by KQLParser#getschemaOperator.
	VisitGetschemaOperator(ctx *GetschemaOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#renderOperator.
	VisitRenderOperator(ctx *RenderOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#renderProperties.
	VisitRenderProperties(ctx *RenderPropertiesContext) interface{}

	// Visit a parse tree produced by KQLParser#renderProperty.
	VisitRenderProperty(ctx *RenderPropertyContext) interface{}

	// Visit a parse tree produced by KQLParser#consumeOperator.
	VisitConsumeOperator(ctx *ConsumeOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#invokeOperator.
	VisitInvokeOperator(ctx *InvokeOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#asOperator.
	VisitAsOperator(ctx *AsOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#graphOperator.
	VisitGraphOperator(ctx *GraphOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#makeGraphOperator.
	VisitMakeGraphOperator(ctx *MakeGraphOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#graphMatchOperator.
	VisitGraphMatchOperator(ctx *GraphMatchOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#graphPattern.
	VisitGraphPattern(ctx *GraphPatternContext) interface{}

	// Visit a parse tree produced by KQLParser#graphPatternElement.
	VisitGraphPatternElement(ctx *GraphPatternElementContext) interface{}

	// Visit a parse tree produced by KQLParser#graphEdge.
	VisitGraphEdge(ctx *GraphEdgeContext) interface{}

	// Visit a parse tree produced by KQLParser#graphShortestPathsOperator.
	VisitGraphShortestPathsOperator(ctx *GraphShortestPathsOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#graphToTableOperator.
	VisitGraphToTableOperator(ctx *GraphToTableOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#graphToTableParams.
	VisitGraphToTableParams(ctx *GraphToTableParamsContext) interface{}

	// Visit a parse tree produced by KQLParser#datatable.
	VisitDatatable(ctx *DatatableContext) interface{}

	// Visit a parse tree produced by KQLParser#datatableSchema.
	VisitDatatableSchema(ctx *DatatableSchemaContext) interface{}

	// Visit a parse tree produced by KQLParser#datatableColumn.
	VisitDatatableColumn(ctx *DatatableColumnContext) interface{}

	// Visit a parse tree produced by KQLParser#datatableRows.
	VisitDatatableRows(ctx *DatatableRowsContext) interface{}

	// Visit a parse tree produced by KQLParser#externalData.
	VisitExternalData(ctx *ExternalDataContext) interface{}

	// Visit a parse tree produced by KQLParser#externalDataUri.
	VisitExternalDataUri(ctx *ExternalDataUriContext) interface{}

	// Visit a parse tree produced by KQLParser#externalDataOptions.
	VisitExternalDataOptions(ctx *ExternalDataOptionsContext) interface{}

	// Visit a parse tree produced by KQLParser#externalDataOption.
	VisitExternalDataOption(ctx *ExternalDataOptionContext) interface{}

	// Visit a parse tree produced by KQLParser#printArgList.
	VisitPrintArgList(ctx *PrintArgListContext) interface{}

	// Visit a parse tree produced by KQLParser#printArg.
	VisitPrintArg(ctx *PrintArgContext) interface{}

	// Visit a parse tree produced by KQLParser#expression.
	VisitExpression(ctx *ExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#orExpression.
	VisitOrExpression(ctx *OrExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#andExpression.
	VisitAndExpression(ctx *AndExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#notExpression.
	VisitNotExpression(ctx *NotExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#comparisonExpression.
	VisitComparisonExpression(ctx *ComparisonExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#comparisonOperator.
	VisitComparisonOperator(ctx *ComparisonOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#stringOperator.
	VisitStringOperator(ctx *StringOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#additiveExpression.
	VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#multiplicativeExpression.
	VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#unaryExpression.
	VisitUnaryExpression(ctx *UnaryExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#postfixExpression.
	VisitPostfixExpression(ctx *PostfixExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#postfixOperator.
	VisitPostfixOperator(ctx *PostfixOperatorContext) interface{}

	// Visit a parse tree produced by KQLParser#primaryExpression.
	VisitPrimaryExpression(ctx *PrimaryExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#functionCall.
	VisitFunctionCall(ctx *FunctionCallContext) interface{}

	// Visit a parse tree produced by KQLParser#builtinFunction.
	VisitBuiltinFunction(ctx *BuiltinFunctionContext) interface{}

	// Visit a parse tree produced by KQLParser#argumentList.
	VisitArgumentList(ctx *ArgumentListContext) interface{}

	// Visit a parse tree produced by KQLParser#argument.
	VisitArgument(ctx *ArgumentContext) interface{}

	// Visit a parse tree produced by KQLParser#caseExpression.
	VisitCaseExpression(ctx *CaseExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#caseBranch.
	VisitCaseBranch(ctx *CaseBranchContext) interface{}

	// Visit a parse tree produced by KQLParser#iffExpression.
	VisitIffExpression(ctx *IffExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#toScalarExpression.
	VisitToScalarExpression(ctx *ToScalarExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#arrayExpression.
	VisitArrayExpression(ctx *ArrayExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#objectExpression.
	VisitObjectExpression(ctx *ObjectExpressionContext) interface{}

	// Visit a parse tree produced by KQLParser#objectPropertyList.
	VisitObjectPropertyList(ctx *ObjectPropertyListContext) interface{}

	// Visit a parse tree produced by KQLParser#objectProperty.
	VisitObjectProperty(ctx *ObjectPropertyContext) interface{}

	// Visit a parse tree produced by KQLParser#functionParameters.
	VisitFunctionParameters(ctx *FunctionParametersContext) interface{}

	// Visit a parse tree produced by KQLParser#functionParameter.
	VisitFunctionParameter(ctx *FunctionParameterContext) interface{}

	// Visit a parse tree produced by KQLParser#typeSpecifier.
	VisitTypeSpecifier(ctx *TypeSpecifierContext) interface{}

	// Visit a parse tree produced by KQLParser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}

	// Visit a parse tree produced by KQLParser#booleanLiteral.
	VisitBooleanLiteral(ctx *BooleanLiteralContext) interface{}

	// Visit a parse tree produced by KQLParser#identifier.
	VisitIdentifier(ctx *IdentifierContext) interface{}

	// Visit a parse tree produced by KQLParser#identifierList.
	VisitIdentifierList(ctx *IdentifierListContext) interface{}

	// Visit a parse tree produced by KQLParser#expressionList.
	VisitExpressionList(ctx *ExpressionListContext) interface{}
}
