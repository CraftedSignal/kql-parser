// Code generated from KQLParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package kql

import "github.com/antlr4-go/antlr/v4"

type BaseKQLParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseKQLParserVisitor) VisitQuery(ctx *QueryContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitLetStatement(ctx *LetStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSetStatement(ctx *SetStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitAliasStatement(ctx *AliasStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitDeclareStatement(ctx *DeclareStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitPatternStatement(ctx *PatternStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitRestrictStatement(ctx *RestrictStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitViewExpression(ctx *ViewExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitPatternDefinition(ctx *PatternDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitPatternParam(ctx *PatternParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitTabularExpression(ctx *TabularExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitTabularSource(ctx *TabularSourceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitTableName(ctx *TableNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitDatabaseTableName(ctx *DatabaseTableNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMaterializeExpression(ctx *MaterializeExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitTabularOperator(ctx *TabularOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitWhereOperator(ctx *WhereOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSearchOperator(ctx *SearchOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSearchKind(ctx *SearchKindContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitTableList(ctx *TableListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitProjectOperator(ctx *ProjectOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitProjectAwayOperator(ctx *ProjectAwayOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitProjectKeepOperator(ctx *ProjectKeepOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitProjectRenameOperator(ctx *ProjectRenameOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitProjectReorderOperator(ctx *ProjectReorderOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitProjectItemList(ctx *ProjectItemListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitProjectItem(ctx *ProjectItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitIdentifierOrWildcardList(ctx *IdentifierOrWildcardListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitIdentifierOrWildcard(ctx *IdentifierOrWildcardContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitRenameList(ctx *RenameListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitRenameItem(ctx *RenameItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitExtendOperator(ctx *ExtendOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitExtendItemList(ctx *ExtendItemListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitExtendItem(ctx *ExtendItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSummarizeOperator(ctx *SummarizeOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSummarizeHints(ctx *SummarizeHintsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitAggregationList(ctx *AggregationListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitAggregationItem(ctx *AggregationItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitAggregationFunction(ctx *AggregationFunctionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGroupByList(ctx *GroupByListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGroupByItem(ctx *GroupByItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSortOperator(ctx *SortOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSortList(ctx *SortListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSortItem(ctx *SortItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSortDirection(ctx *SortDirectionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitNullsPosition(ctx *NullsPositionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitTopOperator(ctx *TopOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitTopNestedOperator(ctx *TopNestedOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitTopNestedClause(ctx *TopNestedClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitTakeOperator(ctx *TakeOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitDistinctOperator(ctx *DistinctOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitDistinctColumns(ctx *DistinctColumnsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitCountOperator(ctx *CountOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitJoinOperator(ctx *JoinOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitJoinKind(ctx *JoinKindContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitJoinFlavor(ctx *JoinFlavorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitJoinHints(ctx *JoinHintsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitJoinHint(ctx *JoinHintContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitJoinCondition(ctx *JoinConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitJoinAttribute(ctx *JoinAttributeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitUnionOperator(ctx *UnionOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitUnionParameters(ctx *UnionParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitUnionParameter(ctx *UnionParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitUnionTables(ctx *UnionTablesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitUnionTable(ctx *UnionTableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitLookupOperator(ctx *LookupOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitLookupKind(ctx *LookupKindContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitLookupCondition(ctx *LookupConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitParseOperator(ctx *ParseOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitParseKind(ctx *ParseKindContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitParsePattern(ctx *ParsePatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitParsePatternItem(ctx *ParsePatternItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitParseKvOperator(ctx *ParseKvOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitKvPairList(ctx *KvPairListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitKvPair(ctx *KvPairContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitParseKvParameters(ctx *ParseKvParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitParseKvParam(ctx *ParseKvParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMvExpandOperator(ctx *MvExpandOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMvExpandKind(ctx *MvExpandKindContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMvExpandParams(ctx *MvExpandParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMvExpandItemList(ctx *MvExpandItemListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMvExpandItem(ctx *MvExpandItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitLimitClause(ctx *LimitClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMvApplyOperator(ctx *MvApplyOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMvApplyItemList(ctx *MvApplyItemListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMvApplyItem(ctx *MvApplyItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMvApplyOnClause(ctx *MvApplyOnClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitEvaluateOperator(ctx *EvaluateOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitEvaluateHints(ctx *EvaluateHintsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitFacetOperator(ctx *FacetOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitForkOperator(ctx *ForkOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitForkBranch(ctx *ForkBranchContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitPartitionOperator(ctx *PartitionOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitPartitionHints(ctx *PartitionHintsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitScanOperator(ctx *ScanOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitScanParams(ctx *ScanParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitScanDeclare(ctx *ScanDeclareContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitScanDeclareItem(ctx *ScanDeclareItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitScanStepList(ctx *ScanStepListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitScanStep(ctx *ScanStepContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitScanAction(ctx *ScanActionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSerializeOperator(ctx *SerializeOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSampleOperator(ctx *SampleOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitSampleDistinctOperator(ctx *SampleDistinctOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMakeSeriesOperator(ctx *MakeSeriesOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMakeSeriesItemList(ctx *MakeSeriesItemListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMakeSeriesItem(ctx *MakeSeriesItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMakeSeriesOnClause(ctx *MakeSeriesOnClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMakeSeriesParams(ctx *MakeSeriesParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitFindOperator(ctx *FindOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitFindParams(ctx *FindParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGetschemaOperator(ctx *GetschemaOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitRenderOperator(ctx *RenderOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitRenderProperties(ctx *RenderPropertiesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitRenderProperty(ctx *RenderPropertyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitConsumeOperator(ctx *ConsumeOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitInvokeOperator(ctx *InvokeOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitAsOperator(ctx *AsOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGraphOperator(ctx *GraphOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMakeGraphOperator(ctx *MakeGraphOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGraphMatchOperator(ctx *GraphMatchOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGraphPattern(ctx *GraphPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGraphPatternElement(ctx *GraphPatternElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGraphEdge(ctx *GraphEdgeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGraphShortestPathsOperator(ctx *GraphShortestPathsOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGraphToTableOperator(ctx *GraphToTableOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitGraphToTableParams(ctx *GraphToTableParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitDatatable(ctx *DatatableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitDatatableSchema(ctx *DatatableSchemaContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitDatatableColumn(ctx *DatatableColumnContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitDatatableRows(ctx *DatatableRowsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitExternalData(ctx *ExternalDataContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitExternalDataUri(ctx *ExternalDataUriContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitExternalDataOptions(ctx *ExternalDataOptionsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitExternalDataOption(ctx *ExternalDataOptionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitPrintArgList(ctx *PrintArgListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitPrintArg(ctx *PrintArgContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitExpression(ctx *ExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitOrExpression(ctx *OrExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitAndExpression(ctx *AndExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitNotExpression(ctx *NotExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitComparisonExpression(ctx *ComparisonExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitComparisonOperator(ctx *ComparisonOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitStringOperator(ctx *StringOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitUnaryExpression(ctx *UnaryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitPostfixExpression(ctx *PostfixExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitPostfixOperator(ctx *PostfixOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitPrimaryExpression(ctx *PrimaryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitFunctionCall(ctx *FunctionCallContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitBuiltinFunction(ctx *BuiltinFunctionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitArgumentList(ctx *ArgumentListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitArgument(ctx *ArgumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitCaseExpression(ctx *CaseExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitCaseBranch(ctx *CaseBranchContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitIffExpression(ctx *IffExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitToScalarExpression(ctx *ToScalarExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitArrayExpression(ctx *ArrayExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitObjectExpression(ctx *ObjectExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitObjectPropertyList(ctx *ObjectPropertyListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitObjectProperty(ctx *ObjectPropertyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitFunctionParameters(ctx *FunctionParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitFunctionParameter(ctx *FunctionParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitTypeSpecifier(ctx *TypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitLiteral(ctx *LiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitBooleanLiteral(ctx *BooleanLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitIdentifier(ctx *IdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitIdentifierList(ctx *IdentifierListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKQLParserVisitor) VisitExpressionList(ctx *ExpressionListContext) interface{} {
	return v.VisitChildren(ctx)
}
