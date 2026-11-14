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

This parser change does not update or release the platform dependency. Before
adopting it, the platform adapter must handle scalar leaves or return an
unsupported-expression error. In particular, `kqlExpressionToDomain` must
reject unknown leaves instead of returning nil, `convertParserBooleanChildren`
must not skip nil AND children, and adapter fallback must not discard scalar
branches by rebuilding an expression from flat conditions.
