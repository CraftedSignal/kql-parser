# Scalar filter predicates

`ConditionExpression` and `SemanticExpression` now have a `BooleanScalar`
(`"scalar"`) leaf with a `Scalar *ScalarPredicate` payload. A scalar predicate
preserves a complete, unevaluated filter, including constant-only filters and
comparisons whose operands cannot be represented faithfully as a `Condition`.

For example, `T | where a == 1 or false` has an OR expression containing a
condition leaf and a scalar leaf with `Text: "false"`. `T | where 5 > 10` has
zero `Conditions` and a non-nil scalar `Expression`. Numeric literals are no
longer emitted as field names; numeric column names must be quoted, e.g.
`['5']`. Standalone quoted field names retain their decoded identity without
sanitization collisions. Literal-left comparisons with a reference on the
right now emit the reference as `Field` and reverse relational operators.

Consumers must walk `Expression`, handle `BooleanScalar`, and preserve its
`Text`, `Negated`, `PipeStage`, and `Pipeline`. `Text` is an exact slice of the
normalized parser input, not necessarily of the original query. The parser
does not evaluate or fold scalar predicates. Applying `Negated` means negating
the complete predicate. The semantic API clones scalar payloads.

A consumer that cannot represent or execute scalar leaves must explicitly
report them as unsupported. Never discard a scalar or unknown leaf, skip it
while converting an AND/OR node, or fall back to the flat `Conditions` list:
that can turn `a == 1 and false` into `a == 1` and change which rows match.
Zero conditions does not mean there is no filter. Repeated scalar occurrences
are preserved because their functions may be nondeterministic.

Known let references that have not been proven to bind a literal in the
applicable scope remain scalar predicates. `LetStatements` retains shadowed
bindings in source order. Constant substitution is limited to later top-level
statements with unambiguous bindings; function scopes and column assignments
that may shadow a binding disable it. Consumers need the retained bindings to
interpret unresolved scalar text.

Null/empty checks with constant or computed arguments, and list operators
with computed or reference operands, now remain complete scalar predicates.
Literal list elements remain condition values; quoted literals never become
references. Representable range bounds use `ValueReference` for columns and
`Value` for literals.

The existing IN-subquery placeholder is a legacy exception: normalization
still discards the subquery and projects membership as a condition whose
`Value` is `SubqueryPlaceholder` (`"SubqueryResult"`). This is an unresolved
subquery marker, not an executable literal membership test or preservation
of the original subquery source. Consumers must treat that marker as
unsupported unless they resolve the subquery independently. Normalized input
quotes this synthetic marker to distinguish it from a genuine unquoted column
reference named `SubqueryResult`, which remains scalar. A user-authored string
literal `"SubqueryResult"` still collides with the public legacy marker; this
schema has no provenance field to distinguish those cases.
If that generated marker would instead become part of a scalar predicate
(for example, `tolower(Account) in ((SigninLogs | project Account))`), extraction
fails with `unsupported scalar predicate: unresolved IN subquery`, no conditions,
and no expression. This also discards other filters in the query so callers
cannot accidentally execute partial AND/OR semantics. Private normalization
provenance distinguishes generated markers from user-authored strings; a scalar
that explicitly contains the string `"SubqueryResult"` remains supported.

This parser change does not update or release the platform dependency. Before
adopting it, the platform adapter must handle scalar leaves or return an
unsupported-expression error. In particular, `kqlExpressionToDomain` must
reject unknown leaves instead of returning nil, `convertParserBooleanChildren`
must not skip nil AND children, and adapter fallback must not discard scalar
branches by rebuilding an expression from flat conditions.
